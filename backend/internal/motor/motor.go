// Package motor ejecuta los respaldos (mysqldump → gzip → NAS/NFS) y las
// restauraciones (gunzip → mysql) contra los servidores MySQL.
package motor

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"

	"sirespaldos/internal/almacen"
	"sirespaldos/internal/eventos"
	"sirespaldos/internal/seguridad"
)

// Nombres de base aceptados. Evita rutas raras en disco y entrecomillado
// peligroso en CREATE DATABASE.
var nombreValido = regexp.MustCompile(`^[A-Za-z0-9_$-]{1,64}$`)

func NombreValido(n string) bool { return nombreValido.MatchString(n) }

type Motor struct {
	alm      *almacen.Almacen
	cif      *seguridad.Cifrador
	hub      *eventos.Hub
	dir      string
	timeout  time.Duration
	sinSSL   bool
	zona     *time.Location
	global   chan struct{}       // limite de trabajos simultaneos
	porServ  map[int]*sync.Mutex // un trabajo a la vez por servidor
	enCurso  map[string]struct{} // servidor/base corriendo ahora
	mu       sync.Mutex
	dumpBin  string
	mysqlBin string
}

type Opciones struct {
	Dir             string
	Timeout         time.Duration
	MaxConcurrencia int
	SinSSL          bool
	Zona            *time.Location
}

func Nuevo(alm *almacen.Almacen, cif *seguridad.Cifrador, hub *eventos.Hub, o Opciones) *Motor {
	return &Motor{
		alm: alm, cif: cif, hub: hub,
		dir: o.Dir, timeout: o.Timeout, sinSSL: o.SinSSL, zona: o.Zona,
		global:   make(chan struct{}, o.MaxConcurrencia),
		porServ:  map[int]*sync.Mutex{},
		enCurso:  map[string]struct{}{},
		dumpBin:  binario("MYSQLDUMP_BIN", "mysqldump"),
		mysqlBin: binario("MYSQL_BIN", "mysql"),
	}
}

func binario(env, def string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return def
}

// ---------------------------------------------------------------------------
// Conexion directa (pruebas y listado de bases)
// ---------------------------------------------------------------------------

func (m *Motor) abrir(s *almacen.Servidor, base string) (*sql.DB, error) {
	pass, err := m.cif.Descifrar(s.PasswordCifrado)
	if err != nil {
		return nil, err
	}
	if s.Usuario == "" {
		return nil, errors.New("el servidor no tiene usuario configurado")
	}
	cfg := mysql.NewConfig()
	cfg.User = s.Usuario
	cfg.Passwd = pass
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", s.Host, s.Puerto)
	cfg.DBName = base
	cfg.Timeout = 8 * time.Second
	cfg.ReadTimeout = 30 * time.Second
	cfg.AllowNativePasswords = true
	if !m.sinSSL {
		cfg.TLSConfig = "preferred"
	}
	return sql.Open("mysql", cfg.FormatDSN())
}

func (m *Motor) Probar(ctx context.Context, s *almacen.Servidor) (string, error) {
	db, err := m.abrir(s, "")
	if err != nil {
		return "", err
	}
	defer db.Close()
	var v string
	err = db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&v)
	return v, err
}

type InfoBase struct {
	Nombre string `json:"nombre"`
	Tablas int    `json:"tablas"`
	Bytes  int64  `json:"bytes"`
}

// Bases lista las bases del servidor con su tamaño aproximado, sin las excluidas.
func (m *Motor) Bases(ctx context.Context, s *almacen.Servidor) ([]InfoBase, error) {
	db, err := m.abrir(s, "")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT sc.SCHEMA_NAME, count(t.TABLE_NAME),
		coalesce(sum(t.DATA_LENGTH + t.INDEX_LENGTH), 0)
		FROM information_schema.SCHEMATA sc
		LEFT JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = sc.SCHEMA_NAME
		GROUP BY sc.SCHEMA_NAME ORDER BY sc.SCHEMA_NAME`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	excl := map[string]bool{}
	for _, e := range s.Excluir {
		excl[strings.ToLower(e)] = true
	}
	out := []InfoBase{}
	for rows.Next() {
		var b InfoBase
		if err := rows.Scan(&b.Nombre, &b.Tablas, &b.Bytes); err != nil {
			return nil, err
		}
		if !excl[strings.ToLower(b.Nombre)] {
			out = append(out, b)
		}
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Respaldos
// ---------------------------------------------------------------------------

type Solicitud struct {
	ServidorID     int
	Base           string // '*' = todas
	Origen         string // manual | programado
	ProgramacionID *int
	RetencionDias  int
	UsuarioID      *int
}

// Encolar resuelve las bases y lanza los respaldos en segundo plano.
// Devuelve las bases que se van a respaldar.
func (m *Motor) Encolar(ctx context.Context, sol Solicitud) ([]string, error) {
	s, err := m.alm.Servidor(ctx, sol.ServidorID)
	if err != nil {
		return nil, err
	}
	if !s.Activo {
		return nil, errors.New("el servidor está desactivado")
	}
	var bases []string
	if sol.Base == "*" {
		info, err := m.Bases(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("no se pudieron listar las bases de %s: %w", s.Nombre, err)
		}
		for _, b := range info {
			bases = append(bases, b.Nombre)
		}
	} else {
		if !NombreValido(sol.Base) {
			return nil, fmt.Errorf("nombre de base inválido: %q", sol.Base)
		}
		bases = []string{sol.Base}
	}
	if sol.RetencionDias < 0 {
		return nil, errors.New("retención inválida")
	}

	go func() {
		for _, b := range bases {
			if !NombreValido(b) {
				slog.Warn("base omitida por nombre no soportado", "servidor", s.Nombre, "base", b)
				continue
			}
			m.respaldar(s, b, sol)
		}
	}()
	return bases, nil
}

func (m *Motor) bloqueo(servidorID int) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.porServ[servidorID]
	if !ok {
		l = &sync.Mutex{}
		m.porServ[servidorID] = l
	}
	return l
}

func (m *Motor) respaldar(s *almacen.Servidor, base string, sol Solicitud) {
	clave := fmt.Sprintf("%d/%s", s.ID, base)
	m.mu.Lock()
	if _, ya := m.enCurso[clave]; ya {
		m.mu.Unlock()
		slog.Info("respaldo omitido: ya hay uno en curso", "servidor", s.Nombre, "base", base)
		return
	}
	m.enCurso[clave] = struct{}{}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.enCurso, clave)
		m.mu.Unlock()
	}()

	bg := context.Background()
	id, err := m.alm.IniciarRespaldo(bg, almacen.Respaldo{
		ServidorID: s.ID, ProgramacionID: sol.ProgramacionID, Base: base,
		Origen: sol.Origen, RetencionDias: sol.RetencionDias,
	}, sol.UsuarioID)
	if err != nil {
		slog.Error("no se pudo registrar el respaldo", "err", err)
		return
	}
	m.hub.Publicar(eventos.Evento{Tipo: "respaldo", ID: id, Estado: "en_cola", Mensaje: s.Nombre + " / " + base})

	m.global <- struct{}{}
	defer func() { <-m.global }()
	l := m.bloqueo(s.ID)
	l.Lock()
	defer l.Unlock()

	m.hub.Publicar(eventos.Evento{Tipo: "respaldo", ID: id, Estado: "en_curso", Mensaje: s.Nombre + " / " + base})
	ctx, cancel := context.WithTimeout(bg, m.timeout)
	defer cancel()

	inicio := time.Now()
	rel, sha, n, err := m.volcar(ctx, s, base)
	if err != nil {
		slog.Error("respaldo fallido", "servidor", s.Nombre, "base", base, "err", err)
		_ = m.alm.TerminarRespaldo(bg, id, "fallido", "", "", 0, err.Error())
		m.hub.Publicar(eventos.Evento{Tipo: "respaldo", ID: id, Estado: "fallido", Mensaje: s.Nombre + " / " + base + ": " + err.Error()})
		return
	}
	_ = m.alm.TerminarRespaldo(bg, id, "exitoso", rel, sha, n, "")
	slog.Info("respaldo exitoso", "servidor", s.Nombre, "base", base, "bytes", n, "duracion", time.Since(inicio).Round(time.Second))
	m.hub.Publicar(eventos.Evento{Tipo: "respaldo", ID: id, Estado: "exitoso", Mensaje: s.Nombre + " / " + base})
}

// volcar corre mysqldump y escribe <dir>/<clave>/<base>/<base>_<fecha>.sql.gz.
// Escribe primero a un .parcial y solo lo renombra si todo salio bien, asi un
// archivo con nombre definitivo siempre es un respaldo completo.
func (m *Motor) volcar(ctx context.Context, s *almacen.Servidor, base string) (rel, sha string, bytesEscritos int64, err error) {
	pass, err := m.cif.Descifrar(s.PasswordCifrado)
	if err != nil {
		return "", "", 0, err
	}
	if s.Usuario == "" {
		return "", "", 0, errors.New("el servidor no tiene usuario configurado")
	}

	// <servidor>/<AAAA-MM-DD>/<base>_<HHMMSS>.sql.gz: en el NAS/MSA se navega
	// igual que en el explorador del sistema (servidor → día → archivos).
	ahora := time.Now().In(m.zona)
	rel = filepath.Join(s.Clave, ahora.Format("2006-01-02"), fmt.Sprintf("%s_%s.sql.gz", base, ahora.Format("150405")))
	destino := filepath.Join(m.dir, rel)
	if err := os.MkdirAll(filepath.Dir(destino), 0o750); err != nil {
		return "", "", 0, fmt.Errorf("creando carpeta en almacenamiento: %w", err)
	}
	parcial := destino + ".parcial"
	f, err := os.OpenFile(parcial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return "", "", 0, err
	}
	limpiar := func() { f.Close(); os.Remove(parcial) }

	args := []string{
		"--host=" + s.Host, fmt.Sprintf("--port=%d", s.Puerto), "--user=" + s.Usuario,
		"--single-transaction", "--quick", "--routines", "--triggers", "--events",
		"--hex-blob", "--default-character-set=utf8mb4", "--add-drop-table",
	}
	if m.sinSSL {
		args = append(args, "--skip-ssl")
	}
	args = append(args, base)

	cmd := exec.CommandContext(ctx, m.dumpBin, args...)
	// La contraseña viaja por entorno, no por argumentos (ps la mostraria).
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+pass)
	var stderr limitado
	cmd.Stderr = &stderr

	h := sha256.New()
	contador := &contarEscritor{}
	gz, _ := gzip.NewWriterLevel(io.MultiWriter(f, h, contador), gzip.BestSpeed)
	cmd.Stdout = gz

	if err := cmd.Run(); err != nil {
		limpiar()
		msg := strings.TrimSpace(stderr.String())
		if ctx.Err() == context.DeadlineExceeded {
			msg = "se excedió el tiempo máximo del respaldo. " + msg
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", "", 0, errors.New(msg)
	}
	if err := gz.Close(); err != nil {
		limpiar()
		return "", "", 0, err
	}
	if err := f.Sync(); err != nil {
		limpiar()
		return "", "", 0, err
	}
	if err := f.Close(); err != nil {
		os.Remove(parcial)
		return "", "", 0, err
	}
	if err := os.Rename(parcial, destino); err != nil {
		os.Remove(parcial)
		return "", "", 0, err
	}
	return rel, hex.EncodeToString(h.Sum(nil)), contador.n, nil
}

// RutaArchivo valida que la ruta guardada no escape del directorio de respaldos.
func (m *Motor) RutaArchivo(rel string) (string, error) {
	if rel == "" {
		return "", errors.New("el respaldo no tiene archivo")
	}
	abs := filepath.Join(m.dir, filepath.Clean("/"+rel))
	base := filepath.Clean(m.dir) + string(os.PathSeparator)
	if !strings.HasPrefix(abs, base) {
		return "", errors.New("ruta fuera del almacenamiento")
	}
	return abs, nil
}

// ---------------------------------------------------------------------------
// Restauracion
// ---------------------------------------------------------------------------

type SolicitudRestauracion struct {
	RespaldoID   int64
	DestinoID    int
	BaseDestino  string
	Sobrescribir bool
	UsuarioID    *int
}

// Restaurar valida todo lo posible de forma sincrona (archivo, destino, base
// existente) y luego corre la carga en segundo plano.
func (m *Motor) Restaurar(ctx context.Context, sol SolicitudRestauracion) (int64, error) {
	r, err := m.alm.Respaldo(ctx, sol.RespaldoID)
	if err != nil {
		return 0, err
	}
	if r.Tipo == "archivo" {
		return 0, errors.New("las copias de archivos no se restauran desde aquí: descárguelas o tómelas de la MSA")
	}
	if r.Estado != "exitoso" {
		return 0, fmt.Errorf("solo se pueden restaurar respaldos exitosos vigentes (estado actual: %s)", r.Estado)
	}
	ruta, err := m.RutaArchivo(r.Archivo)
	if err != nil {
		return 0, err
	}
	if _, err := os.Stat(ruta); err != nil {
		return 0, fmt.Errorf("el archivo del respaldo no está disponible: %w", err)
	}
	if !NombreValido(sol.BaseDestino) {
		return 0, fmt.Errorf("nombre de base destino inválido: %q", sol.BaseDestino)
	}
	dest, err := m.alm.Servidor(ctx, sol.DestinoID)
	if err != nil {
		return 0, err
	}

	db, err := m.abrir(dest, "")
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var tablas int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ?`, sol.BaseDestino).Scan(&tablas); err != nil {
		return 0, fmt.Errorf("no se pudo consultar el destino: %w", err)
	}
	if tablas > 0 && !sol.Sobrescribir {
		return 0, fmt.Errorf("la base %s en %s ya tiene %d tablas; confirme sobrescribir o elija otro nombre", sol.BaseDestino, dest.Nombre, tablas)
	}

	id, err := m.alm.IniciarRestauracion(ctx, r.ID, dest.ID, sol.BaseDestino, sol.UsuarioID)
	if err != nil {
		return 0, err
	}
	go m.restaurar(id, r, ruta, dest, sol.BaseDestino)
	return id, nil
}

func (m *Motor) restaurar(id int64, r *almacen.Respaldo, ruta string, dest *almacen.Servidor, base string) {
	bg := context.Background()
	desc := fmt.Sprintf("%s/%s → %s/%s", r.Servidor, r.Base, dest.Nombre, base)
	m.hub.Publicar(eventos.Evento{Tipo: "restauracion", ID: id, Estado: "en_curso", Mensaje: desc})

	fallar := func(err error) {
		slog.Error("restauración fallida", "id", id, "err", err)
		_ = m.alm.TerminarRestauracion(bg, id, "fallido", err.Error())
		m.hub.Publicar(eventos.Evento{Tipo: "restauracion", ID: id, Estado: "fallido", Mensaje: desc + ": " + err.Error()})
	}

	m.global <- struct{}{}
	defer func() { <-m.global }()
	l := m.bloqueo(dest.ID)
	l.Lock()
	defer l.Unlock()

	ctx, cancel := context.WithTimeout(bg, m.timeout)
	defer cancel()

	// 1) Integridad: el SHA-256 debe coincidir con el registrado al respaldar.
	if err := verificarSHA(ruta, r.SHA256); err != nil {
		fallar(err)
		return
	}

	// 2) Crear la base destino si no existe.
	db, err := m.abrir(dest, "")
	if err != nil {
		fallar(err)
		return
	}
	_, err = db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+base+"` CHARACTER SET utf8mb4")
	db.Close()
	if err != nil {
		fallar(fmt.Errorf("creando base destino: %w", err))
		return
	}

	// 3) gunzip → mysql
	pass, err := m.cif.Descifrar(dest.PasswordCifrado)
	if err != nil {
		fallar(err)
		return
	}
	f, err := os.Open(ruta)
	if err != nil {
		fallar(err)
		return
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		fallar(err)
		return
	}
	defer gz.Close()

	args := []string{"--host=" + dest.Host, fmt.Sprintf("--port=%d", dest.Puerto), "--user=" + dest.Usuario,
		"--default-character-set=utf8mb4", "--max-allowed-packet=512M"}
	if m.sinSSL {
		args = append(args, "--skip-ssl")
	}
	args = append(args, base)
	cmd := exec.CommandContext(ctx, m.mysqlBin, args...)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+pass)
	cmd.Stdin = gz
	var stderr limitado
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		fallar(errors.New(msg))
		return
	}
	_ = m.alm.TerminarRestauracion(bg, id, "exitoso", "")
	m.hub.Publicar(eventos.Evento{Tipo: "restauracion", ID: id, Estado: "exitoso", Mensaje: desc})
}

func verificarSHA(ruta, esperado string) error {
	f, err := os.Open(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != esperado {
		return fmt.Errorf("el archivo está dañado: SHA-256 %s no coincide con %s", got[:12], esperado[:min(12, len(esperado))])
	}
	return nil
}

// ---------------------------------------------------------------------------
// Depuracion por retencion
// ---------------------------------------------------------------------------

func (m *Motor) Depurar(ctx context.Context) (int, error) {
	vencidos, err := m.alm.RespaldosVencidos(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range vencidos {
		// Copias de archivos dentro de una subcarpeta (p. ej. semana-44/): se
		// depura la carpeta completa cuando vence lo mas reciente, para no
		// dejar incrementales sin su respaldo total.
		if r.Tipo == "archivo" {
			partes := strings.Split(filepath.ToSlash(r.Archivo), "/")
			if len(partes) > 4 {
				vig, err := m.alm.VigentesEnCarpeta(ctx, path.Dir(filepath.ToSlash(r.Archivo)))
				if err != nil || vig > 0 {
					continue
				}
			}
		}
		ruta, err := m.RutaArchivo(r.Archivo)
		if err == nil {
			if err := os.Remove(ruta); err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Warn("no se pudo borrar respaldo vencido", "archivo", ruta, "err", err)
				continue
			}
		}
		if err := m.alm.MarcarDepurado(ctx, r.ID); err == nil {
			n++
		}
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// utilidades
// ---------------------------------------------------------------------------

type contarEscritor struct{ n int64 }

func (c *contarEscritor) Write(p []byte) (int, error) { c.n += int64(len(p)); return len(p), nil }

// limitado guarda solo los ultimos 4 KB de stderr.
type limitado struct{ b bytes.Buffer }

func (l *limitado) Write(p []byte) (int, error) {
	l.b.Write(p)
	if l.b.Len() > 4096 {
		resto := l.b.Bytes()[l.b.Len()-4096:]
		nuevo := append([]byte(nil), resto...)
		l.b.Reset()
		l.b.Write(nuevo)
	}
	return len(p), nil
}

func (l *limitado) String() string { return l.b.String() }

// Espacio informa la capacidad del almacenamiento de respaldos (NAS/MSA).
type Espacio struct {
	Ruta  string `json:"ruta"`
	Total uint64 `json:"total"`
	Libre uint64 `json:"libre"`
}

func (m *Motor) Espacio() Espacio {
	e := Espacio{Ruta: m.dir}
	var st syscall.Statfs_t
	if syscall.Statfs(m.dir, &st) == nil {
		e.Total = st.Blocks * uint64(st.Bsize)
		e.Libre = st.Bavail * uint64(st.Bsize)
	}
	return e
}
