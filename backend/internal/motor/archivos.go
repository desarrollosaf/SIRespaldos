package motor

// Recoleccion de archivos: los scripts/cron de cada VM generan sus comprimidos
// (tar.xz, tar.zst, .snar...). SIRespaldos solo se conecta por SSH/SFTP, copia
// a la MSA lo que ya esta listo y lo verifica. Nunca borra nada en la VM.

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"sirespaldos/internal/almacen"
	"sirespaldos/internal/eventos"
)

// ---------------------------------------------------------------------------
// Conexion SSH
// ---------------------------------------------------------------------------

func (m *Motor) conectarSSH(ctx context.Context, s *almacen.Servidor) (*ssh.Client, string, error) {
	if s.SSHUsuario == "" {
		return nil, "", errors.New("el servidor no tiene usuario SSH configurado")
	}
	host := s.SSHHost
	if host == "" {
		host = s.Host
	}
	puerto := s.SSHPuerto
	if puerto == 0 {
		puerto = 22
	}

	var metodos []ssh.AuthMethod
	if s.SSHLlaveCifrada != "" {
		pem, err := m.cif.Descifrar(s.SSHLlaveCifrada)
		if err != nil {
			return nil, "", err
		}
		firma, err := ssh.ParsePrivateKey([]byte(pem))
		if err != nil {
			return nil, "", fmt.Errorf("llave SSH inválida (debe ser una llave privada sin frase de paso): %w", err)
		}
		metodos = append(metodos, ssh.PublicKeys(firma))
	}
	if s.SSHPasswordCifrado != "" {
		pass, err := m.cif.Descifrar(s.SSHPasswordCifrado)
		if err != nil {
			return nil, "", err
		}
		metodos = append(metodos, ssh.Password(pass),
			ssh.KeyboardInteractive(func(_, _ string, preguntas []string, _ []bool) ([]string, error) {
				r := make([]string, len(preguntas))
				for i := range r {
					r[i] = pass
				}
				return r, nil
			}))
	}
	if len(metodos) == 0 {
		return nil, "", errors.New("el servidor no tiene contraseña ni llave SSH")
	}

	// Huella del servidor: se acepta la primera vez y despues debe coincidir,
	// asi nadie puede hacerse pasar por la VM (TOFU, como known_hosts).
	var vista string
	cfg := &ssh.ClientConfig{
		User: s.SSHUsuario,
		Auth: metodos,
		HostKeyCallback: func(_ string, _ net.Addr, llave ssh.PublicKey) error {
			vista = ssh.FingerprintSHA256(llave)
			if s.SSHHuella != "" && s.SSHHuella != vista {
				return fmt.Errorf("la huella SSH de %s cambió (esperada %s, recibida %s). Si reinstalaron el servidor, restablezca la huella en Servidores", host, s.SSHHuella, vista)
			}
			return nil
		},
		Timeout: 15 * time.Second,
	}
	d := net.Dialer{Timeout: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(puerto)))
	if err != nil {
		return nil, "", err
	}
	c, canales, peticiones, err := ssh.NewClientConn(conn, conn.RemoteAddr().String(), cfg)
	if err != nil {
		conn.Close()
		return nil, "", err
	}
	if s.SSHHuella == "" && vista != "" {
		_ = m.alm.GuardarHuella(context.Background(), s.ID, vista)
	}
	return ssh.NewClient(c, canales, peticiones), vista, nil
}

func (m *Motor) abrirSFTP(ctx context.Context, s *almacen.Servidor) (*sftp.Client, func(), error) {
	sc, _, err := m.conectarSSH(ctx, s)
	if err != nil {
		return nil, nil, err
	}
	cli, err := sftp.NewClient(sc, sftp.UseConcurrentReads(true), sftp.MaxConcurrentRequestsPerFile(64))
	if err != nil {
		sc.Close()
		return nil, nil, fmt.Errorf("el servidor no acepta SFTP: %w", err)
	}
	return cli, func() { cli.Close(); sc.Close() }, nil
}

// ProbarSSH conecta y devuelve la huella del servidor y el usuario remoto.
func (m *Motor) ProbarSSH(ctx context.Context, s *almacen.Servidor) (string, error) {
	sc, huella, err := m.conectarSSH(ctx, s)
	if err != nil {
		return "", err
	}
	sc.Close()
	return huella, nil
}

// ---------------------------------------------------------------------------
// Listado remoto
// ---------------------------------------------------------------------------

type ArchivoRemoto struct {
	Ruta     string    `json:"ruta"`     // absoluta en la VM
	Relativa string    `json:"relativa"` // respecto a la carpeta del origen
	Bytes    int64     `json:"bytes"`
	Mtime    time.Time `json:"mtime"`
	Estado   string    `json:"estado"` // listo | escribiendo | copiado | ignorado
}

func coincide(nombre, patrones string) bool {
	for _, p := range strings.Split(patrones, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if ok, _ := path.Match(p, nombre); ok {
			return true
		}
	}
	return false
}

func temporal(nombre string) bool {
	n := strings.ToLower(nombre)
	for _, suf := range []string{".parcial", ".partial", ".part", ".tmp", ".temp", ".swp", ".lock"} {
		if strings.HasSuffix(n, suf) {
			return true
		}
	}
	return strings.HasPrefix(n, ".")
}

func (m *Motor) listarRemoto(ctx context.Context, cli *sftp.Client, o *almacen.OrigenArchivos) ([]ArchivoRemoto, error) {
	raiz := path.Clean(o.Ruta)
	if _, err := cli.Stat(raiz); err != nil {
		return nil, fmt.Errorf("no se puede leer %s en el servidor: %w", raiz, err)
	}
	limite := time.Now().Add(-time.Duration(o.EstabilidadMin) * time.Minute)
	var out []ArchivoRemoto
	w := cli.Walk(raiz)
	for w.Step() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if w.Err() != nil {
			continue // carpeta sin permiso: se omite
		}
		st := w.Stat()
		if !st.Mode().IsRegular() {
			continue
		}
		nombre := path.Base(w.Path())
		rel := strings.TrimPrefix(strings.TrimPrefix(w.Path(), raiz), "/")
		a := ArchivoRemoto{Ruta: w.Path(), Relativa: rel, Bytes: st.Size(), Mtime: st.ModTime()}
		switch {
		case temporal(nombre) || !coincide(nombre, o.Patrones):
			a.Estado = "ignorado"
		case st.ModTime().After(limite):
			a.Estado = "escribiendo"
		default:
			a.Estado = "listo"
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Relativa < out[j].Relativa })
	return out, nil
}

// VerRemoto lista la carpeta del origen indicando que se copiaria.
func (m *Motor) VerRemoto(ctx context.Context, origenID int) ([]ArchivoRemoto, error) {
	o, err := m.alm.Origen(ctx, origenID)
	if err != nil {
		return nil, err
	}
	s, err := m.alm.Servidor(ctx, o.ServidorID)
	if err != nil {
		return nil, err
	}
	cli, cerrar, err := m.abrirSFTP(ctx, s)
	if err != nil {
		return nil, err
	}
	defer cerrar()
	lista, err := m.listarRemoto(ctx, cli, o)
	if err != nil {
		return nil, err
	}
	for i, a := range lista {
		if a.Estado != "listo" {
			continue
		}
		if prev, _ := m.alm.UltimaCopia(ctx, o.ID, a.Ruta); yaCopiado(prev, a) {
			lista[i].Estado = "copiado"
		}
	}
	return lista, nil
}

func yaCopiado(prev *almacen.CopiaPrevia, a ArchivoRemoto) bool {
	return prev != nil && prev.Bytes == a.Bytes && prev.Mtime != nil &&
		prev.Mtime.Unix() == a.Mtime.Unix()
}

// ---------------------------------------------------------------------------
// Copia
// ---------------------------------------------------------------------------

type SolicitudCopia struct {
	OrigenID  int
	Origen    string // manual | programado
	UsuarioID *int
}

// EncolarCopia valida el origen y lanza la recoleccion en segundo plano.
func (m *Motor) EncolarCopia(ctx context.Context, sol SolicitudCopia) error {
	o, err := m.alm.Origen(ctx, sol.OrigenID)
	if err != nil {
		return err
	}
	if !o.Activo && sol.Origen == "programado" {
		return nil
	}
	s, err := m.alm.Servidor(ctx, o.ServidorID)
	if err != nil {
		return err
	}
	if !s.Activo {
		return errors.New("el servidor está desactivado")
	}
	clave := fmt.Sprintf("origen/%d", o.ID)
	m.mu.Lock()
	if _, ya := m.enCurso[clave]; ya {
		m.mu.Unlock()
		return errors.New("ya hay una copia en curso de este origen")
	}
	m.enCurso[clave] = struct{}{}
	m.mu.Unlock()

	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.enCurso, clave)
			m.mu.Unlock()
		}()
		m.recolectar(s, o, sol)
	}()
	return nil
}

func (m *Motor) recolectar(s *almacen.Servidor, o *almacen.OrigenArchivos, sol SolicitudCopia) {
	desc := s.Nombre + " / " + o.Nombre
	m.global <- struct{}{}
	defer func() { <-m.global }()
	l := m.bloqueo(s.ID)
	l.Lock()
	defer l.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	m.hub.Publicar(eventos.Evento{Tipo: "archivos", Estado: "en_curso", Mensaje: desc + ": revisando carpeta remota"})

	cli, cerrar, err := m.abrirSFTP(ctx, s)
	if err != nil {
		m.fallaOrigen(o, sol, desc, err)
		return
	}
	defer cerrar()
	lista, err := m.listarRemoto(ctx, cli, o)
	if err != nil {
		m.fallaOrigen(o, sol, desc, err)
		return
	}

	copiados, fallidos, omitidos := 0, 0, 0
	for _, a := range lista {
		if a.Estado != "listo" {
			continue
		}
		prev, err := m.alm.UltimaCopia(ctx, o.ID, a.Ruta)
		if err != nil {
			slog.Error("consultando copia previa", "err", err)
			continue
		}
		if yaCopiado(prev, a) {
			omitidos++
			continue
		}
		if m.copiarArchivo(ctx, cli, s, o, a, prev, sol) {
			copiados++
		} else {
			fallidos++
		}
		if ctx.Err() != nil {
			break
		}
	}
	estado := "exitoso"
	if fallidos > 0 {
		estado = "fallido"
	}
	msg := fmt.Sprintf("%s: %d copiados, %d ya estaban, %d con error", desc, copiados, omitidos, fallidos)
	slog.Info("recolección terminada", "origen", desc, "copiados", copiados, "omitidos", omitidos, "fallidos", fallidos)
	m.hub.Publicar(eventos.Evento{Tipo: "archivos", Estado: estado, Mensaje: msg})
}

// fallaOrigen registra en el historial que no se pudo ni siquiera listar el origen.
func (m *Motor) fallaOrigen(o *almacen.OrigenArchivos, sol SolicitudCopia, desc string, err error) {
	slog.Error("recolección fallida", "origen", desc, "err", err)
	bg := context.Background()
	if id, e := m.alm.IniciarCopia(bg, o, o.Ruta, time.Now(), sol.Origen, sol.UsuarioID); e == nil {
		_ = m.alm.TerminarRespaldo(bg, id, "fallido", "", "", 0, err.Error())
	}
	m.hub.Publicar(eventos.Evento{Tipo: "archivos", Estado: "fallido", Mensaje: desc + ": " + err.Error()})
}

func (m *Motor) copiarArchivo(ctx context.Context, cli *sftp.Client, s *almacen.Servidor, o *almacen.OrigenArchivos,
	a ArchivoRemoto, prev *almacen.CopiaPrevia, sol SolicitudCopia) bool {
	bg := context.Background()
	id, err := m.alm.IniciarCopia(bg, o, a.Ruta, a.Mtime, sol.Origen, sol.UsuarioID)
	if err != nil {
		slog.Error("no se pudo registrar la copia", "err", err)
		return false
	}
	m.hub.Publicar(eventos.Evento{Tipo: "respaldo", ID: id, Estado: "en_curso", Mensaje: s.Nombre + " / " + a.Relativa})

	rel, sha, n, err := m.descargar(ctx, cli, s, o, a)
	if err != nil {
		_ = m.alm.TerminarRespaldo(bg, id, "fallido", "", "", 0, err.Error())
		m.hub.Publicar(eventos.Evento{Tipo: "respaldo", ID: id, Estado: "fallido", Mensaje: s.Nombre + " / " + a.Relativa + ": " + err.Error()})
		return false
	}
	// Si el mismo archivo remoto ya se habia copiado (cambio, p. ej. el .snar),
	// la copia anterior se sobrescribio: se marca como reemplazada.
	if prev != nil && prev.Archivo == rel && prev.Estado == "exitoso" {
		_ = m.alm.MarcarReemplazado(bg, prev.ID)
	}
	_ = m.alm.TerminarRespaldo(bg, id, "exitoso", rel, sha, n, "")
	m.hub.Publicar(eventos.Evento{Tipo: "respaldo", ID: id, Estado: "exitoso", Mensaje: s.Nombre + " / " + a.Relativa})
	return true
}

// descargar copia el archivo a <dir>/<servidor>/archivos/<origen>/<relativa>.
func (m *Motor) descargar(ctx context.Context, cli *sftp.Client, s *almacen.Servidor, o *almacen.OrigenArchivos, a ArchivoRemoto) (string, string, int64, error) {
	limpio := path.Clean("/" + a.Relativa)[1:]
	if limpio == "" || strings.HasPrefix(limpio, "..") {
		return "", "", 0, fmt.Errorf("ruta remota no válida: %s", a.Relativa)
	}
	rel := filepath.Join(s.Clave, "archivos", o.Nombre, filepath.FromSlash(limpio))
	destino, err := m.RutaArchivo(rel)
	if err != nil {
		return "", "", 0, err
	}
	if err := os.MkdirAll(filepath.Dir(destino), 0o750); err != nil {
		return "", "", 0, fmt.Errorf("creando carpeta en almacenamiento: %w", err)
	}

	rf, err := cli.Open(a.Ruta)
	if err != nil {
		return "", "", 0, err
	}
	defer rf.Close()
	parcial := destino + ".parcial"
	f, err := os.OpenFile(parcial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return "", "", 0, err
	}
	limpiar := func() { f.Close(); os.Remove(parcial) }

	h := sha256.New()
	// Cancelable: si vence el tiempo maximo se cierra el archivo remoto.
	detener := context.AfterFunc(ctx, func() { rf.Close() })
	defer detener()
	n, err := rf.WriteTo(io.MultiWriter(f, h))
	if err != nil {
		limpiar()
		if ctx.Err() != nil {
			return "", "", 0, errors.New("se excedió el tiempo máximo de la copia")
		}
		return "", "", 0, err
	}
	if n != a.Bytes {
		limpiar()
		return "", "", 0, fmt.Errorf("copia incompleta: %d de %d bytes (¿el archivo cambió durante la copia?)", n, a.Bytes)
	}
	// Si el archivo cambio mientras se copiaba, la copia no es confiable.
	if st, err := cli.Stat(a.Ruta); err == nil && (st.Size() != a.Bytes || !st.ModTime().Equal(a.Mtime)) {
		limpiar()
		return "", "", 0, errors.New("el archivo cambió durante la copia; se reintentará en la siguiente ejecución")
	}
	sha := hex.EncodeToString(h.Sum(nil))

	// Si el script dejo un <archivo>.sha256, debe coincidir.
	if esperado := leerSHA(cli, a.Ruta+".sha256"); esperado != "" && !strings.EqualFold(esperado, sha) {
		limpiar()
		return "", "", 0, fmt.Errorf("SHA-256 no coincide con %s.sha256 (archivo dañado en la VM o en la red)", path.Base(a.Ruta))
	}
	if err := f.Sync(); err != nil {
		limpiar()
		return "", "", 0, err
	}
	if err := f.Close(); err != nil {
		os.Remove(parcial)
		return "", "", 0, err
	}
	_ = os.Chtimes(parcial, a.Mtime, a.Mtime)
	if err := os.Rename(parcial, destino); err != nil {
		os.Remove(parcial)
		return "", "", 0, err
	}
	return rel, sha, n, nil
}

// leerSHA lee la primera palabra de un archivo estilo `sha256sum`.
func leerSHA(cli *sftp.Client, ruta string) string {
	f, err := cli.Open(ruta)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(io.LimitReader(f, 4096))
	if sc.Scan() {
		if campos := strings.Fields(sc.Text()); len(campos) > 0 && len(campos[0]) == 64 {
			return campos[0]
		}
	}
	return ""
}
