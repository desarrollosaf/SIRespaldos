// Package almacen es el acceso a la base administrativa (MySQL / MariaDB).
//
// Todas las fechas se guardan en UTC: la conexion fija time_zone='+00:00' y el
// driver devuelve time.Time en UTC. Para agrupar por dia local se usa el
// desfase de la zona configurada (CONVERT_TZ con desfase numerico, que no
// requiere tener cargadas las tablas de zonas horarias de MySQL).
package almacen

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

//go:embed migraciones/*.sql
var migracionesFS embed.FS

var ErrNoEncontrado = errors.New("no encontrado")

type Almacen struct {
	DB *sql.DB
}

type Conexion struct {
	Host     string
	Puerto   int
	Usuario  string
	Password string
	Base     string
}

func Abrir(ctx context.Context, c Conexion) (*Almacen, error) {
	cfg := mysql.NewConfig()
	cfg.User = c.Usuario
	cfg.Passwd = c.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", c.Host, c.Puerto)
	cfg.DBName = c.Base
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Collation = "utf8mb4_unicode_ci"
	cfg.Timeout = 8 * time.Second
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	// MySQL puede tardar en aceptar conexiones al levantar docker compose.
	for intento := 0; intento < 45; intento++ {
		if err = db.PingContext(ctx); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("conectando a MySQL %s/%s: %w", cfg.Addr, c.Base, err)
	}
	a := &Almacen{DB: db}
	if err := a.migrar(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

// migrar aplica migraciones/*.sql en orden. MySQL no tiene DDL transaccional,
// asi que cada archivo se ejecuta sentencia por sentencia.
func (a *Almacen) migrar(ctx context.Context) error {
	if _, err := a.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS migraciones (
		nombre VARCHAR(255) PRIMARY KEY, aplicada DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}
	nombres, err := fs.Glob(migracionesFS, "migraciones/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(nombres)
	for _, n := range nombres {
		var existe int
		if err := a.DB.QueryRowContext(ctx, `SELECT count(*) FROM migraciones WHERE nombre=?`, n).Scan(&existe); err != nil {
			return err
		}
		if existe > 0 {
			continue
		}
		contenido, _ := migracionesFS.ReadFile(n)
		for _, sentencia := range sentencias(string(contenido)) {
			if _, err := a.DB.ExecContext(ctx, sentencia); err != nil {
				return fmt.Errorf("migración %s: %w\n%s", n, err, sentencia)
			}
		}
		if _, err := a.DB.ExecContext(ctx, `INSERT INTO migraciones (nombre) VALUES (?)`, n); err != nil {
			return err
		}
	}
	return nil
}

// sentencias quita comentarios "--" y separa por ';' al final de linea.
func sentencias(sqlTexto string) []string {
	var b strings.Builder
	for _, l := range strings.Split(sqlTexto, "\n") {
		if i := strings.Index(l, "--"); i >= 0 && !strings.Contains(l[:i], "'") {
			l = l[:i]
		}
		b.WriteString(strings.TrimRight(l, " \t") + "\n")
	}
	var out []string
	for _, s := range strings.Split(b.String(), ";\n") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, strings.TrimSuffix(s, ";"))
		}
	}
	return out
}

// desfase devuelve el desfase actual de la zona como '+HH:MM'.
func desfase(zona string) string {
	loc, err := time.LoadLocation(zona)
	if err != nil {
		loc = time.UTC
	}
	_, seg := time.Now().In(loc).Zone()
	signo := '+'
	if seg < 0 {
		signo = '-'
		seg = -seg
	}
	return fmt.Sprintf("%c%02d:%02d", signo, seg/3600, (seg%3600)/60)
}

type fila interface{ Scan(dest ...any) error }

// consultar ejecuta la consulta y escanea cada fila con scan.
func consultar[T any](ctx context.Context, db *sql.DB, scan func(fila) (*T, error), q string, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		x, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

func idInsertado(res sql.Result, err error) (int, error) {
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

// ---------------------------------------------------------------------------
// Modelos
// ---------------------------------------------------------------------------

type Usuario struct {
	ID      int       `json:"id"`
	Usuario string    `json:"usuario"`
	Nombre  string    `json:"nombre"`
	Rol     string    `json:"rol"`
	Activo  bool      `json:"activo"`
	Creado  time.Time `json:"creado"`
	Hash    string    `json:"-"`
}

type Servidor struct {
	ID              int       `json:"id"`
	Clave           string    `json:"clave"`
	Nombre          string    `json:"nombre"`
	Host            string    `json:"host"`
	Puerto          int       `json:"puerto"`
	Usuario         string    `json:"usuario"`
	PasswordCifrado string    `json:"-"`
	TienePassword   bool      `json:"tienePassword"`
	Excluir         []string  `json:"excluir"`
	Activo          bool      `json:"activo"`
	Actualizado     time.Time `json:"actualizado"`
	Logo            string    `json:"logo"`

	SSHHost            string `json:"sshHost"`
	SSHPuerto          int    `json:"sshPuerto"`
	SSHUsuario         string `json:"sshUsuario"`
	SSHPasswordCifrado string `json:"-"`
	SSHLlaveCifrada    string `json:"-"`
	TieneSSHPassword   bool   `json:"tieneSshPassword"`
	TieneSSHLlave      bool   `json:"tieneSshLlave"`
	SSHHuella          string `json:"sshHuella"`
}

type Programacion struct {
	ID            int       `json:"id"`
	ServidorID    int       `json:"servidorId"`
	Servidor      string    `json:"servidor"`
	Base          string    `json:"base"`
	Cron          string    `json:"cron"`
	RetencionDias int       `json:"retencionDias"`
	Descripcion   string    `json:"descripcion"`
	Activo        bool      `json:"activo"`
	Creado        time.Time `json:"creado"`
}

type Respaldo struct {
	ID             int64      `json:"id"`
	ServidorID     int        `json:"servidorId"`
	Servidor       string     `json:"servidor"`
	ProgramacionID *int       `json:"programacionId"`
	Base           string     `json:"base"`
	Origen         string     `json:"origen"`
	Estado         string     `json:"estado"`
	Inicio         time.Time  `json:"inicio"`
	Fin            *time.Time `json:"fin"`
	Bytes          int64      `json:"bytes"`
	Archivo        string     `json:"archivo"`
	SHA256         string     `json:"sha256"`
	Error          string     `json:"error"`
	RetencionDias  int        `json:"retencionDias"`
	SolicitadoPor  *string    `json:"solicitadoPor"`
	Tipo           string     `json:"tipo"` // base | archivo
	RutaOrigen     string     `json:"rutaOrigen"`
}

type Restauracion struct {
	ID              int64      `json:"id"`
	RespaldoID      int64      `json:"respaldoId"`
	BaseOrigen      string     `json:"baseOrigen"`
	ServidorOrigen  string     `json:"servidorOrigen"`
	ServidorDestino string     `json:"servidorDestino"`
	BaseDestino     string     `json:"baseDestino"`
	Estado          string     `json:"estado"`
	Inicio          time.Time  `json:"inicio"`
	Fin             *time.Time `json:"fin"`
	Error           string     `json:"error"`
	SolicitadoPor   *string    `json:"solicitadoPor"`
}

// ---------------------------------------------------------------------------
// Usuarios
// ---------------------------------------------------------------------------

const colsUsuario = `id, usuario, nombre, rol, activo, creado, hash`

func scanUsuario(r fila) (*Usuario, error) {
	u := &Usuario{}
	err := r.Scan(&u.ID, &u.Usuario, &u.Nombre, &u.Rol, &u.Activo, &u.Creado, &u.Hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoEncontrado
	}
	return u, err
}

func (a *Almacen) UsuarioPorNombre(ctx context.Context, usuario string) (*Usuario, error) {
	return scanUsuario(a.DB.QueryRowContext(ctx, `SELECT `+colsUsuario+` FROM usuarios WHERE usuario=?`, usuario))
}

func (a *Almacen) UsuarioPorID(ctx context.Context, id int) (*Usuario, error) {
	return scanUsuario(a.DB.QueryRowContext(ctx, `SELECT `+colsUsuario+` FROM usuarios WHERE id=?`, id))
}

func (a *Almacen) Usuarios(ctx context.Context) ([]Usuario, error) {
	return consultar(ctx, a.DB, scanUsuario, `SELECT `+colsUsuario+` FROM usuarios ORDER BY nombre`)
}

func (a *Almacen) ContarUsuarios(ctx context.Context) (int, error) {
	var n int
	err := a.DB.QueryRowContext(ctx, `SELECT count(*) FROM usuarios`).Scan(&n)
	return n, err
}

func (a *Almacen) CrearUsuario(ctx context.Context, u Usuario) (int, error) {
	return idInsertado(a.DB.ExecContext(ctx,
		`INSERT INTO usuarios (usuario, nombre, hash, rol, activo) VALUES (?,?,?,?,?)`,
		u.Usuario, u.Nombre, u.Hash, u.Rol, u.Activo))
}

func (a *Almacen) ActualizarUsuario(ctx context.Context, u Usuario) error {
	if u.Hash != "" {
		_, err := a.DB.ExecContext(ctx, `UPDATE usuarios SET nombre=?, rol=?, activo=?, hash=? WHERE id=?`,
			u.Nombre, u.Rol, u.Activo, u.Hash, u.ID)
		return err
	}
	_, err := a.DB.ExecContext(ctx, `UPDATE usuarios SET nombre=?, rol=?, activo=? WHERE id=?`,
		u.Nombre, u.Rol, u.Activo, u.ID)
	return err
}

// ---------------------------------------------------------------------------
// Servidores
// ---------------------------------------------------------------------------

const colsServidor = `id, clave, nombre, host, puerto, usuario, password_cifrado, excluir, activo, actualizado, logo,
	ssh_host, ssh_puerto, ssh_usuario, COALESCE(ssh_password_cifrado, ''), COALESCE(ssh_llave_cifrada, ''), ssh_huella`

func scanServidor(r fila) (*Servidor, error) {
	s := &Servidor{}
	var excluir string
	err := r.Scan(&s.ID, &s.Clave, &s.Nombre, &s.Host, &s.Puerto, &s.Usuario, &s.PasswordCifrado, &excluir, &s.Activo, &s.Actualizado, &s.Logo,
		&s.SSHHost, &s.SSHPuerto, &s.SSHUsuario, &s.SSHPasswordCifrado, &s.SSHLlaveCifrada, &s.SSHHuella)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoEncontrado
	}
	s.Excluir = []string{}
	for _, e := range strings.Split(excluir, ",") {
		if e = strings.TrimSpace(e); e != "" {
			s.Excluir = append(s.Excluir, e)
		}
	}
	s.TienePassword = s.PasswordCifrado != ""
	s.TieneSSHPassword = s.SSHPasswordCifrado != ""
	s.TieneSSHLlave = s.SSHLlaveCifrada != ""
	return s, err
}

func (a *Almacen) Servidores(ctx context.Context) ([]Servidor, error) {
	return consultar(ctx, a.DB, scanServidor, `SELECT `+colsServidor+` FROM servidores ORDER BY nombre`)
}

func (a *Almacen) Servidor(ctx context.Context, id int) (*Servidor, error) {
	return scanServidor(a.DB.QueryRowContext(ctx, `SELECT `+colsServidor+` FROM servidores WHERE id=?`, id))
}

func (a *Almacen) CrearServidor(ctx context.Context, s Servidor) (int, error) {
	return idInsertado(a.DB.ExecContext(ctx,
		`INSERT INTO servidores (clave, nombre, host, puerto, usuario, password_cifrado, excluir, logo, activo,
		   ssh_host, ssh_puerto, ssh_usuario, ssh_password_cifrado, ssh_llave_cifrada)
		 VALUES (?,?,?,?,?,?,?,'',?,?,?,?,?,?)`,
		s.Clave, s.Nombre, s.Host, s.Puerto, s.Usuario, s.PasswordCifrado, strings.Join(s.Excluir, ","), s.Activo,
		s.SSHHost, s.SSHPuerto, s.SSHUsuario, s.SSHPasswordCifrado, s.SSHLlaveCifrada))
}

// ActualizarServidor conserva contraseñas/llave guardadas cuando llegan vacias.
// Con quitarSSH se borran las credenciales SSH. Si cambia el host o puerto SSH
// (o se pide), se olvida la huella para volver a aceptarla en la siguiente conexion.
func (a *Almacen) ActualizarServidor(ctx context.Context, s Servidor, quitarSSH, olvidarHuella bool) error {
	_, err := a.DB.ExecContext(ctx,
		`UPDATE servidores SET
		   ssh_huella = IF(? OR ssh_host <> ? OR ssh_puerto <> ?, '', ssh_huella),
		   nombre=?, host=?, puerto=?, usuario=?,
		   password_cifrado = IF(? = '', password_cifrado, ?),
		   excluir=?, activo=?,
		   ssh_host=?, ssh_puerto=?, ssh_usuario=?,
		   ssh_password_cifrado = IF(?, NULL, IF(? = '', ssh_password_cifrado, ?)),
		   ssh_llave_cifrada    = IF(?, NULL, IF(? = '', ssh_llave_cifrada, ?)),
		   actualizado=UTC_TIMESTAMP(6)
		 WHERE id=?`,
		olvidarHuella, s.SSHHost, s.SSHPuerto,
		s.Nombre, s.Host, s.Puerto, s.Usuario, s.PasswordCifrado, s.PasswordCifrado,
		strings.Join(s.Excluir, ","), s.Activo,
		s.SSHHost, s.SSHPuerto, s.SSHUsuario,
		quitarSSH, s.SSHPasswordCifrado, s.SSHPasswordCifrado,
		quitarSSH, s.SSHLlaveCifrada, s.SSHLlaveCifrada,
		s.ID)
	return err
}

// GuardarHuella fija la huella SSH aceptada la primera vez (TOFU).
func (a *Almacen) GuardarHuella(ctx context.Context, id int, huella string) error {
	_, err := a.DB.ExecContext(ctx, `UPDATE servidores SET ssh_huella=? WHERE id=? AND ssh_huella=''`, huella, id)
	return err
}

func (a *Almacen) GuardarLogo(ctx context.Context, id int, logo string) error {
	if _, err := a.Servidor(ctx, id); err != nil {
		return err
	}
	_, err := a.DB.ExecContext(ctx, `UPDATE servidores SET logo=?, actualizado=UTC_TIMESTAMP(6) WHERE id=?`, logo, id)
	return err
}

// ---------------------------------------------------------------------------
// Programaciones
// ---------------------------------------------------------------------------

const selProgramacion = `SELECT p.id, p.servidor_id, s.nombre, p.base, p.cron, p.retencion_dias, p.descripcion, p.activo, p.creado
	FROM programaciones p JOIN servidores s ON s.id = p.servidor_id`

func scanProgramacion(r fila) (*Programacion, error) {
	p := &Programacion{}
	err := r.Scan(&p.ID, &p.ServidorID, &p.Servidor, &p.Base, &p.Cron, &p.RetencionDias, &p.Descripcion, &p.Activo, &p.Creado)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoEncontrado
	}
	return p, err
}

func (a *Almacen) Programaciones(ctx context.Context) ([]Programacion, error) {
	return consultar(ctx, a.DB, scanProgramacion, selProgramacion+` ORDER BY s.nombre, p.base`)
}

func (a *Almacen) Programacion(ctx context.Context, id int) (*Programacion, error) {
	return scanProgramacion(a.DB.QueryRowContext(ctx, selProgramacion+` WHERE p.id=?`, id))
}

func (a *Almacen) GuardarProgramacion(ctx context.Context, p Programacion) (int, error) {
	if p.ID == 0 {
		return idInsertado(a.DB.ExecContext(ctx,
			`INSERT INTO programaciones (servidor_id, base, cron, retencion_dias, descripcion, activo) VALUES (?,?,?,?,?,?)`,
			p.ServidorID, p.Base, p.Cron, p.RetencionDias, p.Descripcion, p.Activo))
	}
	if _, err := a.Programacion(ctx, p.ID); err != nil {
		return 0, err
	}
	_, err := a.DB.ExecContext(ctx,
		`UPDATE programaciones SET servidor_id=?, base=?, cron=?, retencion_dias=?, descripcion=?, activo=? WHERE id=?`,
		p.ServidorID, p.Base, p.Cron, p.RetencionDias, p.Descripcion, p.Activo, p.ID)
	return p.ID, err
}

func (a *Almacen) EliminarProgramacion(ctx context.Context, id int) error {
	_, err := a.DB.ExecContext(ctx, `DELETE FROM programaciones WHERE id=?`, id)
	return err
}

// ---------------------------------------------------------------------------
// Respaldos
// ---------------------------------------------------------------------------

const selRespaldo = `SELECT r.id, r.servidor_id, s.nombre, r.programacion_id, r.base, r.origen, r.estado,
	r.inicio, r.fin, r.bytes, r.archivo, r.sha256, COALESCE(r.error, ''), r.retencion_dias, u.nombre, r.tipo, r.ruta_origen
	FROM respaldos r JOIN servidores s ON s.id = r.servidor_id LEFT JOIN usuarios u ON u.id = r.solicitado_por`

func scanRespaldo(r fila) (*Respaldo, error) {
	x := &Respaldo{}
	err := r.Scan(&x.ID, &x.ServidorID, &x.Servidor, &x.ProgramacionID, &x.Base, &x.Origen, &x.Estado,
		&x.Inicio, &x.Fin, &x.Bytes, &x.Archivo, &x.SHA256, &x.Error, &x.RetencionDias, &x.SolicitadoPor, &x.Tipo, &x.RutaOrigen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoEncontrado
	}
	return x, err
}

func (a *Almacen) IniciarRespaldo(ctx context.Context, r Respaldo, usuarioID *int) (int64, error) {
	res, err := a.DB.ExecContext(ctx,
		`INSERT INTO respaldos (servidor_id, programacion_id, base, origen, estado, inicio, retencion_dias, solicitado_por, error)
		 VALUES (?,?,?,?,'en_curso',UTC_TIMESTAMP(6),?,?,'')`,
		r.ServidorID, r.ProgramacionID, r.Base, r.Origen, r.RetencionDias, usuarioID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (a *Almacen) TerminarRespaldo(ctx context.Context, id int64, estado, archivo, sha string, bytes int64, msgError string) error {
	_, err := a.DB.ExecContext(ctx,
		`UPDATE respaldos SET estado=?, archivo=?, sha256=?, bytes=?, error=?, fin=UTC_TIMESTAMP(6) WHERE id=?`,
		estado, archivo, sha, bytes, msgError, id)
	return err
}

// MarcarInterrumpidos cierra respaldos que quedaron "en_curso" si el proceso se reinicio.
func (a *Almacen) MarcarInterrumpidos(ctx context.Context) error {
	if _, err := a.DB.ExecContext(ctx, `UPDATE respaldos SET estado='fallido', error='Interrumpido por reinicio del servicio', fin=UTC_TIMESTAMP(6) WHERE estado='en_curso'`); err != nil {
		return err
	}
	_, err := a.DB.ExecContext(ctx, `UPDATE restauraciones SET estado='fallido', error='Interrumpida por reinicio del servicio', fin=UTC_TIMESTAMP(6) WHERE estado='en_curso'`)
	return err
}

func (a *Almacen) Respaldo(ctx context.Context, id int64) (*Respaldo, error) {
	return scanRespaldo(a.DB.QueryRowContext(ctx, selRespaldo+` WHERE r.id=?`, id))
}

type FiltroRespaldos struct {
	ServidorID int
	Base       string
	Estado     string
	Desde      *time.Time
	Hasta      *time.Time
	Limite     int
	Desplazar  int
}

func (a *Almacen) Respaldos(ctx context.Context, f FiltroRespaldos) ([]Respaldo, int, error) {
	var cond []string
	var args []any
	if f.ServidorID > 0 {
		cond, args = append(cond, "r.servidor_id = ?"), append(args, f.ServidorID)
	}
	if f.Base != "" {
		cond, args = append(cond, "r.base LIKE ?"), append(args, "%"+f.Base+"%")
	}
	if f.Estado != "" {
		cond, args = append(cond, "r.estado = ?"), append(args, f.Estado)
	}
	if f.Desde != nil {
		cond, args = append(cond, "r.inicio >= ?"), append(args, f.Desde.UTC())
	}
	if f.Hasta != nil {
		cond, args = append(cond, "r.inicio < ?"), append(args, f.Hasta.UTC())
	}
	where := ""
	if len(cond) > 0 {
		where = " WHERE " + strings.Join(cond, " AND ")
	}

	var total int
	if err := a.DB.QueryRowContext(ctx, `SELECT count(*) FROM respaldos r`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.Limite <= 0 || f.Limite > 500 {
		f.Limite = 50
	}
	if f.Desplazar < 0 {
		f.Desplazar = 0
	}
	lista, err := consultar(ctx, a.DB, scanRespaldo, selRespaldo+where+` ORDER BY r.inicio DESC LIMIT ? OFFSET ?`,
		append(args, f.Limite, f.Desplazar)...)
	return lista, total, err
}

// RespaldosVencidos devuelve respaldos exitosos cuya retencion ya paso (0 = permanente).
func (a *Almacen) RespaldosVencidos(ctx context.Context) ([]Respaldo, error) {
	return consultar(ctx, a.DB, scanRespaldo, selRespaldo+` WHERE r.estado='exitoso' AND r.retencion_dias > 0
		AND DATE_ADD(r.inicio, INTERVAL r.retencion_dias DAY) < UTC_TIMESTAMP(6)`)
}

func (a *Almacen) MarcarDepurado(ctx context.Context, id int64) error {
	_, err := a.DB.ExecContext(ctx, `UPDATE respaldos SET estado='depurado' WHERE id=?`, id)
	return err
}

// ---------------------------------------------------------------------------
// Restauraciones
// ---------------------------------------------------------------------------

func (a *Almacen) IniciarRestauracion(ctx context.Context, respaldoID int64, destinoID int, base string, usuarioID *int) (int64, error) {
	res, err := a.DB.ExecContext(ctx,
		`INSERT INTO restauraciones (respaldo_id, servidor_destino_id, base_destino, estado, inicio, solicitado_por, error)
		 VALUES (?,?,?,'en_curso',UTC_TIMESTAMP(6),?,'')`, respaldoID, destinoID, base, usuarioID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (a *Almacen) TerminarRestauracion(ctx context.Context, id int64, estado, msgError string) error {
	_, err := a.DB.ExecContext(ctx, `UPDATE restauraciones SET estado=?, error=?, fin=UTC_TIMESTAMP(6) WHERE id=?`, estado, msgError, id)
	return err
}

func scanRestauracion(r fila) (*Restauracion, error) {
	x := &Restauracion{}
	err := r.Scan(&x.ID, &x.RespaldoID, &x.BaseOrigen, &x.ServidorOrigen, &x.ServidorDestino, &x.BaseDestino,
		&x.Estado, &x.Inicio, &x.Fin, &x.Error, &x.SolicitadoPor)
	return x, err
}

func (a *Almacen) Restauraciones(ctx context.Context, limite int) ([]Restauracion, error) {
	if limite <= 0 || limite > 500 {
		limite = 100
	}
	return consultar(ctx, a.DB, scanRestauracion, `SELECT t.id, t.respaldo_id, r.base, so.nombre, sd.nombre, t.base_destino, t.estado,
		t.inicio, t.fin, COALESCE(t.error, ''), u.nombre
		FROM restauraciones t
		JOIN respaldos r ON r.id = t.respaldo_id
		JOIN servidores so ON so.id = r.servidor_id
		JOIN servidores sd ON sd.id = t.servidor_destino_id
		LEFT JOIN usuarios u ON u.id = t.solicitado_por
		ORDER BY t.inicio DESC LIMIT ?`, limite)
}

// ---------------------------------------------------------------------------
// Tablero
// ---------------------------------------------------------------------------

type ResumenServidor struct {
	ServidorID       int        `json:"servidorId"`
	Nombre           string     `json:"nombre"`
	Host             string     `json:"host"`
	Activo           bool       `json:"activo"`
	UltimoExito      *time.Time `json:"ultimoExito"`
	UltimoEstado     *string    `json:"ultimoEstado"`
	Exitosos7d       int        `json:"exitosos7d"`
	Fallidos7d       int        `json:"fallidos7d"`
	EnCurso          int        `json:"enCurso"`
	BytesVigentes    int64      `json:"bytesVigentes"`
	ArchivosVigentes int        `json:"archivosVigentes"`
}

func scanResumen(r fila) (*ResumenServidor, error) {
	x := &ResumenServidor{}
	err := r.Scan(&x.ServidorID, &x.Nombre, &x.Host, &x.Activo, &x.UltimoExito, &x.UltimoEstado,
		&x.Exitosos7d, &x.Fallidos7d, &x.EnCurso, &x.BytesVigentes, &x.ArchivosVigentes)
	return x, err
}

func (a *Almacen) Resumen(ctx context.Context) ([]ResumenServidor, error) {
	return consultar(ctx, a.DB, scanResumen, `SELECT s.id, s.nombre, s.host, s.activo,
		(SELECT max(fin) FROM respaldos WHERE servidor_id=s.id AND estado='exitoso'),
		(SELECT estado FROM respaldos WHERE servidor_id=s.id ORDER BY inicio DESC LIMIT 1),
		(SELECT count(*) FROM respaldos WHERE servidor_id=s.id AND estado IN ('exitoso','depurado') AND inicio > UTC_TIMESTAMP(6) - INTERVAL 7 DAY),
		(SELECT count(*) FROM respaldos WHERE servidor_id=s.id AND estado='fallido' AND inicio > UTC_TIMESTAMP(6) - INTERVAL 7 DAY),
		(SELECT count(*) FROM respaldos WHERE servidor_id=s.id AND estado='en_curso'),
		(SELECT COALESCE(sum(bytes),0) FROM respaldos WHERE servidor_id=s.id AND estado='exitoso'),
		(SELECT count(*) FROM respaldos WHERE servidor_id=s.id AND estado='exitoso')
		FROM servidores s ORDER BY s.nombre`)
}

type PuntoDiario struct {
	Dia      string `json:"dia"`
	Exitosos int    `json:"exitosos"`
	Fallidos int    `json:"fallidos"`
	Bytes    int64  `json:"bytes"`
}

func scanPunto(r fila) (*PuntoDiario, error) {
	p := &PuntoDiario{}
	return p, r.Scan(&p.Dia, &p.Exitosos, &p.Fallidos, &p.Bytes)
}

// Tendencia devuelve los ultimos `dias` dias (hora local), incluidos los dias sin respaldos.
func (a *Almacen) Tendencia(ctx context.Context, dias int, zona string) ([]PuntoDiario, error) {
	loc, err := time.LoadLocation(zona)
	if err != nil {
		loc = time.UTC
	}
	hoy := time.Now().In(loc)
	inicio := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(dias - 1))

	filas, err := consultar(ctx, a.DB, scanPunto, `SELECT DATE_FORMAT(CONVERT_TZ(inicio, '+00:00', ?), '%Y-%m-%d') d,
		CAST(SUM(estado IN ('exitoso','depurado')) AS SIGNED), CAST(SUM(estado = 'fallido') AS SIGNED),
		CAST(COALESCE(SUM(IF(estado IN ('exitoso','depurado'), bytes, 0)), 0) AS SIGNED)
		FROM respaldos WHERE inicio >= ? GROUP BY d`, desfase(zona), inicio.UTC())
	if err != nil {
		return nil, err
	}
	porDia := map[string]PuntoDiario{}
	for _, p := range filas {
		porDia[p.Dia] = p
	}
	out := make([]PuntoDiario, 0, dias)
	for i := 0; i < dias; i++ {
		d := inicio.AddDate(0, 0, i).Format("2006-01-02")
		p := porDia[d]
		p.Dia = d
		out = append(out, p)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Explorador: servidor → dias → archivos
// ---------------------------------------------------------------------------

type DiaRespaldo struct {
	Dia      string `json:"dia"` // AAAA-MM-DD en hora local
	Archivos int    `json:"archivos"`
	Bytes    int64  `json:"bytes"`
	Exitosos int    `json:"exitosos"`
	Fallidos int    `json:"fallidos"`
	Vigentes int    `json:"vigentes"`
}

func scanDia(r fila) (*DiaRespaldo, error) {
	d := &DiaRespaldo{}
	return d, r.Scan(&d.Dia, &d.Archivos, &d.Bytes, &d.Exitosos, &d.Fallidos, &d.Vigentes)
}

func (a *Almacen) DiasServidor(ctx context.Context, servidorID int, zona string) ([]DiaRespaldo, error) {
	return consultar(ctx, a.DB, scanDia, `SELECT DATE_FORMAT(CONVERT_TZ(inicio, '+00:00', ?), '%Y-%m-%d') d,
		count(*), CAST(COALESCE(SUM(IF(estado='exitoso', bytes, 0)), 0) AS SIGNED),
		CAST(SUM(estado IN ('exitoso','depurado')) AS SIGNED), CAST(SUM(estado='fallido') AS SIGNED),
		CAST(SUM(estado='exitoso') AS SIGNED)
		FROM respaldos WHERE servidor_id=? GROUP BY d ORDER BY d DESC`, desfase(zona), servidorID)
}

func (a *Almacen) RespaldosDelDia(ctx context.Context, servidorID int, dia, zona string) ([]Respaldo, error) {
	loc, err := time.LoadLocation(zona)
	if err != nil {
		loc = time.UTC
	}
	ini, err := time.ParseInLocation("2006-01-02", dia, loc)
	if err != nil {
		return nil, err
	}
	return consultar(ctx, a.DB, scanRespaldo, selRespaldo+` WHERE r.servidor_id=? AND r.inicio >= ? AND r.inicio < ? ORDER BY r.base, r.inicio`,
		servidorID, ini.UTC(), ini.AddDate(0, 0, 1).UTC())
}

// ---------------------------------------------------------------------------
// Origenes de archivos (recoleccion por SFTP)
// ---------------------------------------------------------------------------

type OrigenArchivos struct {
	ID             int        `json:"id"`
	ServidorID     int        `json:"servidorId"`
	Servidor       string     `json:"servidor"`
	Nombre         string     `json:"nombre"`
	Ruta           string     `json:"ruta"`
	Patrones       string     `json:"patrones"`
	EstabilidadMin int        `json:"estabilidadMin"`
	Cron           string     `json:"cron"`
	RetencionDias  int        `json:"retencionDias"`
	Descripcion    string     `json:"descripcion"`
	Activo         bool       `json:"activo"`
	UltimaCopia    *time.Time `json:"ultimaCopia"`
	Archivos       int        `json:"archivos"`
	Bytes          int64      `json:"bytes"`
}

const selOrigen = `SELECT o.id, o.servidor_id, s.nombre, o.nombre, o.ruta, o.patrones, o.estabilidad_min, o.cron,
	o.retencion_dias, o.descripcion, o.activo,
	(SELECT max(fin) FROM respaldos WHERE origen_archivo_id=o.id AND estado='exitoso'),
	(SELECT count(*) FROM respaldos WHERE origen_archivo_id=o.id AND estado='exitoso'),
	(SELECT COALESCE(sum(bytes),0) FROM respaldos WHERE origen_archivo_id=o.id AND estado='exitoso')
	FROM origenes_archivos o JOIN servidores s ON s.id = o.servidor_id`

func scanOrigen(r fila) (*OrigenArchivos, error) {
	o := &OrigenArchivos{}
	err := r.Scan(&o.ID, &o.ServidorID, &o.Servidor, &o.Nombre, &o.Ruta, &o.Patrones, &o.EstabilidadMin, &o.Cron,
		&o.RetencionDias, &o.Descripcion, &o.Activo, &o.UltimaCopia, &o.Archivos, &o.Bytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoEncontrado
	}
	return o, err
}

func (a *Almacen) Origenes(ctx context.Context) ([]OrigenArchivos, error) {
	return consultar(ctx, a.DB, scanOrigen, selOrigen+` ORDER BY s.nombre, o.nombre`)
}

func (a *Almacen) Origen(ctx context.Context, id int) (*OrigenArchivos, error) {
	return scanOrigen(a.DB.QueryRowContext(ctx, selOrigen+` WHERE o.id=?`, id))
}

func (a *Almacen) GuardarOrigen(ctx context.Context, o OrigenArchivos) (int, error) {
	if o.ID == 0 {
		return idInsertado(a.DB.ExecContext(ctx,
			`INSERT INTO origenes_archivos (servidor_id, nombre, ruta, patrones, estabilidad_min, cron, retencion_dias, descripcion, activo)
			 VALUES (?,?,?,?,?,?,?,?,?)`,
			o.ServidorID, o.Nombre, o.Ruta, o.Patrones, o.EstabilidadMin, o.Cron, o.RetencionDias, o.Descripcion, o.Activo))
	}
	if _, err := a.Origen(ctx, o.ID); err != nil {
		return 0, err
	}
	// El nombre no cambia: es la carpeta en la MSA donde ya hay copias.
	_, err := a.DB.ExecContext(ctx,
		`UPDATE origenes_archivos SET ruta=?, patrones=?, estabilidad_min=?, cron=?, retencion_dias=?, descripcion=?, activo=? WHERE id=?`,
		o.Ruta, o.Patrones, o.EstabilidadMin, o.Cron, o.RetencionDias, o.Descripcion, o.Activo, o.ID)
	return o.ID, err
}

func (a *Almacen) EliminarOrigen(ctx context.Context, id int) error {
	_, err := a.DB.ExecContext(ctx, `DELETE FROM origenes_archivos WHERE id=?`, id)
	return err
}

// CopiaPrevia es la ultima copia registrada de un archivo remoto.
type CopiaPrevia struct {
	ID      int64
	Bytes   int64
	Mtime   *time.Time
	Estado  string
	Archivo string
}

func (a *Almacen) UltimaCopia(ctx context.Context, origenID int, rutaOrigen string) (*CopiaPrevia, error) {
	c := &CopiaPrevia{}
	err := a.DB.QueryRowContext(ctx, `SELECT id, bytes, mtime_origen, estado, archivo FROM respaldos
		WHERE origen_archivo_id=? AND ruta_origen=? AND estado IN ('exitoso','en_curso')
		ORDER BY inicio DESC LIMIT 1`, origenID, rutaOrigen).Scan(&c.ID, &c.Bytes, &c.Mtime, &c.Estado, &c.Archivo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

func (a *Almacen) IniciarCopia(ctx context.Context, o *OrigenArchivos, rutaOrigen string, mtime time.Time, origen string, usuarioID *int) (int64, error) {
	res, err := a.DB.ExecContext(ctx,
		`INSERT INTO respaldos (servidor_id, base, origen, estado, inicio, retencion_dias, solicitado_por, error,
		   tipo, origen_archivo_id, ruta_origen, mtime_origen)
		 VALUES (?,?,?,'en_curso',UTC_TIMESTAMP(6),?,?,'','archivo',?,?,?)`,
		o.ServidorID, o.Nombre, origen, o.RetencionDias, usuarioID, o.ID, rutaOrigen, mtime.UTC())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MarcarReemplazado deja como depurada una copia anterior cuyo archivo local
// fue sobrescrito por una version mas reciente (p. ej. snapshot.snar).
func (a *Almacen) MarcarReemplazado(ctx context.Context, id int64) error {
	_, err := a.DB.ExecContext(ctx, `UPDATE respaldos SET estado='depurado', error='Reemplazado por una versión más reciente' WHERE id=?`, id)
	return err
}

// VigentesEnCarpeta cuenta copias exitosas no vencidas dentro de la carpeta
// local dada (p. ej. camara/archivos/congreso/semana-44).
func (a *Almacen) VigentesEnCarpeta(ctx context.Context, carpeta string) (int, error) {
	var n int
	err := a.DB.QueryRowContext(ctx, `SELECT count(*) FROM respaldos
		WHERE tipo='archivo' AND estado='exitoso' AND LEFT(archivo, CHAR_LENGTH(?)+1) = CONCAT(?, '/')
		AND (retencion_dias = 0 OR DATE_ADD(inicio, INTERVAL retencion_dias DAY) >= UTC_TIMESTAMP(6))`,
		carpeta, carpeta).Scan(&n)
	return n, err
}
