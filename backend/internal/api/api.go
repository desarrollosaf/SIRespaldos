// Package api expone la API REST (Gin) consumida por el frontend Angular.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"sirespaldos/internal/almacen"
	"sirespaldos/internal/eventos"
	"sirespaldos/internal/motor"
	"sirespaldos/internal/programador"
	"sirespaldos/internal/seguridad"
)

type API struct {
	Alm              *almacen.Almacen
	Mot              *motor.Motor
	Prog             *programador.Programador
	Hub              *eventos.Hub
	Tokens           *seguridad.Tokens
	Cif              *seguridad.Cifrador
	Zona             *time.Location
	CORS             []string
	RetencionDefecto int
}

func (a *API) Rutas() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger(), a.cors())

	g := r.Group("/api")
	g.GET("/salud", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	g.POST("/auth/login", a.login)

	// SSE: EventSource no permite cabeceras, el token llega por query.
	g.GET("/eventos", a.autenticarQuery(), a.eventos)

	p := g.Group("", a.autenticar())
	p.GET("/auth/yo", a.yo)

	p.GET("/tablero", a.tablero)
	p.GET("/calendario", a.calendario)

	p.GET("/servidores", a.listarServidores)
	p.POST("/servidores", rol("admin"), a.crearServidor)
	p.PUT("/servidores/:id", rol("admin"), a.actualizarServidor)
	p.POST("/servidores/:id/probar", rol("admin", "operador"), a.probarServidor)
	p.GET("/servidores/:id/bases", a.basesServidor)
	p.PUT("/servidores/:id/logo", rol("admin"), a.guardarLogo)
	p.POST("/servidores/:id/probar-ssh", rol("admin", "operador"), a.probarSSH)

	p.GET("/origenes", a.listarOrigenes)
	p.POST("/origenes", rol("admin"), a.guardarOrigen)
	p.PUT("/origenes/:id", rol("admin"), a.guardarOrigen)
	p.DELETE("/origenes/:id", rol("admin"), a.eliminarOrigen)
	p.GET("/origenes/:id/remoto", rol("admin", "operador"), a.verRemoto)
	p.POST("/origenes/:id/copiar", rol("admin", "operador"), a.copiarAhora)

	p.GET("/explorador", a.explorador)
	p.GET("/explorador/:id/dias", a.diasServidor)
	p.GET("/explorador/:id/dias/:dia", a.respaldosDelDia)

	p.GET("/programaciones", a.listarProgramaciones)
	p.POST("/programaciones", rol("admin", "operador"), a.guardarProgramacion)
	p.PUT("/programaciones/:id", rol("admin", "operador"), a.guardarProgramacion)
	p.DELETE("/programaciones/:id", rol("admin", "operador"), a.eliminarProgramacion)

	p.GET("/respaldos", a.listarRespaldos)
	p.POST("/respaldos", rol("admin", "operador"), a.respaldarAhora)
	p.GET("/respaldos/:id/descargar", rol("admin"), a.descargar)
	p.POST("/respaldos/:id/restaurar", rol("admin"), a.restaurar)
	p.GET("/restauraciones", a.listarRestauraciones)

	p.GET("/usuarios", rol("admin"), a.listarUsuarios)
	p.POST("/usuarios", rol("admin"), a.guardarUsuario)
	p.PUT("/usuarios/:id", rol("admin"), a.guardarUsuario)
	return r
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

func (a *API) cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origen := c.GetHeader("Origin")
		for _, o := range a.CORS {
			if o == origen {
				c.Header("Access-Control-Allow-Origin", origen)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				c.Header("Access-Control-Expose-Headers", "Content-Disposition")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (a *API) validar(c *gin.Context, token string) bool {
	claims, err := a.Tokens.Validar(token)
	if err != nil {
		c.AbortWithStatusJSON(401, gin.H{"mensaje": "Sesión inválida o vencida"})
		return false
	}
	// El rol y el estado se revisan contra la base en cada peticion: desactivar
	// una cuenta o cambiarle el rol surte efecto de inmediato.
	u, err := a.Alm.UsuarioPorID(c, claims.UsuarioID)
	if err != nil || !u.Activo {
		c.AbortWithStatusJSON(401, gin.H{"mensaje": "Cuenta inactiva"})
		return false
	}
	c.Set("usuario", u)
	return true
}

func (a *API) autenticar() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(401, gin.H{"mensaje": "Falta el token"})
			return
		}
		if a.validar(c, strings.TrimPrefix(h, "Bearer ")) {
			c.Next()
		}
	}
}

func (a *API) autenticarQuery() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.validar(c, c.Query("token")) {
			c.Next()
		}
	}
}

func rol(permitidos ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := usuario(c)
		for _, p := range permitidos {
			if u.Rol == p {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(403, gin.H{"mensaje": "No tiene permiso para esta acción"})
	}
}

func usuario(c *gin.Context) *almacen.Usuario {
	return c.MustGet("usuario").(*almacen.Usuario)
}

func idParam(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"mensaje": "Identificador inválido"})
		return 0, false
	}
	return id, true
}

func fallo(c *gin.Context, err error) {
	if errors.Is(err, almacen.ErrNoEncontrado) {
		c.JSON(404, gin.H{"mensaje": "No encontrado"})
		return
	}
	c.JSON(500, gin.H{"mensaje": err.Error()})
}

// ---------------------------------------------------------------------------
// Autenticacion
// ---------------------------------------------------------------------------

func (a *API) login(c *gin.Context) {
	var in struct {
		Usuario  string `json:"usuario" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"mensaje": "Capture usuario y contraseña"})
		return
	}
	u, err := a.Alm.UsuarioPorNombre(c, strings.TrimSpace(in.Usuario))
	if err != nil || !u.Activo || !seguridad.VerificarPassword(u.Hash, in.Password) {
		time.Sleep(400 * time.Millisecond)
		c.JSON(401, gin.H{"mensaje": "Usuario o contraseña incorrectos"})
		return
	}
	tok, err := a.Tokens.Emitir(u.ID, u.Rol, u.Nombre)
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"token": tok, "usuario": u})
}

func (a *API) yo(c *gin.Context) { c.JSON(200, usuario(c)) }

// ---------------------------------------------------------------------------
// Tablero y calendario
// ---------------------------------------------------------------------------

func (a *API) tablero(c *gin.Context) {
	resumen, err := a.Alm.Resumen(c)
	if err != nil {
		fallo(c, err)
		return
	}
	tend, err := a.Alm.Tendencia(c, 14, a.Zona.String())
	if err != nil {
		fallo(c, err)
		return
	}
	ahora := time.Now()
	prox, err := a.Prog.Proximas(c, ahora, ahora.Add(48*time.Hour))
	if err != nil {
		fallo(c, err)
		return
	}
	if len(prox) > 10 {
		ordenarOcurrencias(prox)
		prox = prox[:10]
	} else {
		ordenarOcurrencias(prox)
	}
	recientes, _, err := a.Alm.Respaldos(c, almacen.FiltroRespaldos{Limite: 10})
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"servidores": resumen, "tendencia": tend, "proximas": prox, "recientes": recientes})
}

func (a *API) calendario(c *gin.Context) {
	desde, err1 := time.Parse(time.RFC3339, c.Query("desde"))
	hasta, err2 := time.Parse(time.RFC3339, c.Query("hasta"))
	if err1 != nil || err2 != nil || !hasta.After(desde) || hasta.Sub(desde) > 62*24*time.Hour {
		c.JSON(400, gin.H{"mensaje": "Rango inválido (desde/hasta en RFC3339, máximo 62 días)"})
		return
	}
	prox, err := a.Prog.Proximas(c, desde, hasta)
	if err != nil {
		fallo(c, err)
		return
	}
	ordenarOcurrencias(prox)
	hechos, _, err := a.Alm.Respaldos(c, almacen.FiltroRespaldos{Desde: &desde, Hasta: &hasta, Limite: 500})
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"previstos": prox, "realizados": hechos})
}

func ordenarOcurrencias(o []programador.Ocurrencia) {
	for i := 1; i < len(o); i++ {
		for j := i; j > 0 && o[j].Momento.Before(o[j-1].Momento); j-- {
			o[j], o[j-1] = o[j-1], o[j]
		}
	}
}

// ---------------------------------------------------------------------------
// Servidores
// ---------------------------------------------------------------------------

type entradaServidor struct {
	Clave    string   `json:"clave"`
	Nombre   string   `json:"nombre" binding:"required"`
	Host     string   `json:"host" binding:"required"`
	Puerto   int      `json:"puerto"`
	Usuario  string   `json:"usuario"`
	Password string   `json:"password"`
	Excluir  []string `json:"excluir"`
	Activo   bool     `json:"activo"`

	SSHHost           string `json:"sshHost"`
	SSHPuerto         int    `json:"sshPuerto"`
	SSHUsuario        string `json:"sshUsuario"`
	SSHPassword       string `json:"sshPassword"`
	SSHLlave          string `json:"sshLlave"`
	QuitarSSH         bool   `json:"quitarSsh"`
	RestablecerHuella bool   `json:"restablecerHuella"`
}

func (a *API) aServidor(in entradaServidor) (almacen.Servidor, error) {
	if in.Puerto == 0 {
		in.Puerto = 3306
	}
	if in.Excluir == nil {
		in.Excluir = []string{"information_schema", "performance_schema", "sys"}
	}
	if in.SSHPuerto == 0 {
		in.SSHPuerto = 22
	}
	s := almacen.Servidor{Clave: in.Clave, Nombre: in.Nombre, Host: in.Host, Puerto: in.Puerto,
		Usuario: in.Usuario, Excluir: in.Excluir, Activo: in.Activo,
		SSHHost: strings.TrimSpace(in.SSHHost), SSHPuerto: in.SSHPuerto, SSHUsuario: strings.TrimSpace(in.SSHUsuario)}
	for _, x := range []struct {
		plano   string
		destino *string
	}{{in.SSHPassword, &s.SSHPasswordCifrado}, {strings.TrimSpace(in.SSHLlave), &s.SSHLlaveCifrada}} {
		if x.plano == "" {
			continue
		}
		cif, err := a.Cif.Cifrar(x.plano)
		if err != nil {
			return s, err
		}
		*x.destino = cif
	}
	if in.Password != "" {
		cif, err := a.Cif.Cifrar(in.Password)
		if err != nil {
			return s, err
		}
		s.PasswordCifrado = cif
	}
	return s, nil
}

func (a *API) listarServidores(c *gin.Context) {
	s, err := a.Alm.Servidores(c)
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, s)
}

func (a *API) crearServidor(c *gin.Context) {
	var in entradaServidor
	if err := c.ShouldBindJSON(&in); err != nil || !motor.NombreValido(in.Clave) {
		c.JSON(400, gin.H{"mensaje": "Datos incompletos (la clave sólo admite letras, números, guion y guion bajo)"})
		return
	}
	s, err := a.aServidor(in)
	if err != nil {
		fallo(c, err)
		return
	}
	id, err := a.Alm.CrearServidor(c, s)
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(201, gin.H{"id": id})
}

func (a *API) actualizarServidor(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var in entradaServidor
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"mensaje": "Datos incompletos"})
		return
	}
	s, err := a.aServidor(in)
	if err != nil {
		fallo(c, err)
		return
	}
	s.ID = id
	if err := a.Alm.ActualizarServidor(c, s, in.QuitarSSH, in.RestablecerHuella); err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (a *API) probarServidor(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	s, err := a.Alm.Servidor(c, id)
	if err != nil {
		fallo(c, err)
		return
	}
	v, err := a.Mot.Probar(c, s)
	if err != nil {
		c.JSON(200, gin.H{"ok": false, "mensaje": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "version": v})
}

func (a *API) basesServidor(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	s, err := a.Alm.Servidor(c, id)
	if err != nil {
		fallo(c, err)
		return
	}
	b, err := a.Mot.Bases(c, s)
	if err != nil {
		c.JSON(502, gin.H{"mensaje": "No se pudo conectar a " + s.Nombre + ": " + err.Error()})
		return
	}
	c.JSON(200, b)
}

// ---------------------------------------------------------------------------
// Programaciones
// ---------------------------------------------------------------------------

func (a *API) listarProgramaciones(c *gin.Context) {
	p, err := a.Alm.Programaciones(c)
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, p)
}

func (a *API) guardarProgramacion(c *gin.Context) {
	var in almacen.Programacion
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"mensaje": "Datos inválidos"})
		return
	}
	if c.Param("id") != "" {
		id, ok := idParam(c)
		if !ok {
			return
		}
		in.ID = id
	} else {
		in.ID = 0
	}
	in.Cron = strings.TrimSpace(in.Cron)
	if err := programador.ValidarCron(in.Cron); err != nil {
		c.JSON(400, gin.H{"mensaje": "Expresión cron inválida: " + err.Error()})
		return
	}
	if in.Base != "*" && !motor.NombreValido(in.Base) {
		c.JSON(400, gin.H{"mensaje": "Base inválida"})
		return
	}
	if in.RetencionDias < 0 {
		c.JSON(400, gin.H{"mensaje": "Retención inválida (0 = permanente)"})
		return
	}
	id, err := a.Alm.GuardarProgramacion(c, in)
	if err != nil {
		fallo(c, err)
		return
	}
	if err := a.Prog.Recargar(c); err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"id": id})
}

func (a *API) eliminarProgramacion(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := a.Alm.EliminarProgramacion(c, id); err != nil {
		fallo(c, err)
		return
	}
	_ = a.Prog.Recargar(c)
	c.JSON(200, gin.H{"ok": true})
}

// ---------------------------------------------------------------------------
// Respaldos y restauraciones
// ---------------------------------------------------------------------------

func (a *API) listarRespaldos(c *gin.Context) {
	f := almacen.FiltroRespaldos{Base: c.Query("base"), Estado: c.Query("estado")}
	f.ServidorID, _ = strconv.Atoi(c.Query("servidor"))
	f.Limite, _ = strconv.Atoi(c.Query("limite"))
	f.Desplazar, _ = strconv.Atoi(c.Query("desplazar"))
	if t, err := time.Parse(time.RFC3339, c.Query("desde")); err == nil {
		f.Desde = &t
	}
	if t, err := time.Parse(time.RFC3339, c.Query("hasta")); err == nil {
		f.Hasta = &t
	}
	lista, total, err := a.Alm.Respaldos(c, f)
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"datos": lista, "total": total})
}

func (a *API) respaldarAhora(c *gin.Context) {
	var in struct {
		ServidorID    int    `json:"servidorId" binding:"required"`
		Base          string `json:"base" binding:"required"`
		RetencionDias *int   `json:"retencionDias"` // ausente = valor por defecto; 0 = permanente
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"mensaje": "Indique servidor y base"})
		return
	}
	ret := a.RetencionDefecto
	if in.RetencionDias != nil {
		ret = *in.RetencionDias
	}
	uid := usuario(c).ID
	bases, err := a.Mot.Encolar(c, motor.Solicitud{
		ServidorID: in.ServidorID, Base: in.Base, Origen: "manual",
		RetencionDias: ret, UsuarioID: &uid,
	})
	if err != nil {
		c.JSON(400, gin.H{"mensaje": err.Error()})
		return
	}
	c.JSON(202, gin.H{"bases": bases})
}

func (a *API) descargar(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	r, err := a.Alm.Respaldo(c, int64(id))
	if err != nil {
		fallo(c, err)
		return
	}
	ruta, err := a.Mot.RutaArchivo(r.Archivo)
	if err != nil {
		c.JSON(404, gin.H{"mensaje": err.Error()})
		return
	}
	f, err := os.Open(ruta)
	if err != nil {
		c.JSON(404, gin.H{"mensaje": "Archivo no disponible"})
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(ruta)))
	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Length", strconv.FormatInt(st.Size(), 10))
	c.Status(200)
	_, _ = io.Copy(c.Writer, f)
}

func (a *API) restaurar(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var in struct {
		ServidorDestinoID int    `json:"servidorDestinoId" binding:"required"`
		BaseDestino       string `json:"baseDestino" binding:"required"`
		Sobrescribir      bool   `json:"sobrescribir"`
		Confirmacion      string `json:"confirmacion"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"mensaje": "Indique servidor y base destino"})
		return
	}
	// Sobrescribir exige teclear el nombre de la base, como confirmacion explicita.
	if in.Sobrescribir && in.Confirmacion != in.BaseDestino {
		c.JSON(400, gin.H{"mensaje": "Para sobrescribir escriba el nombre exacto de la base destino"})
		return
	}
	uid := usuario(c).ID
	rid, err := a.Mot.Restaurar(c, motor.SolicitudRestauracion{
		RespaldoID: int64(id), DestinoID: in.ServidorDestinoID, BaseDestino: in.BaseDestino,
		Sobrescribir: in.Sobrescribir, UsuarioID: &uid,
	})
	if err != nil {
		c.JSON(400, gin.H{"mensaje": err.Error()})
		return
	}
	c.JSON(202, gin.H{"id": rid})
}

func (a *API) listarRestauraciones(c *gin.Context) {
	r, err := a.Alm.Restauraciones(c, 200)
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, r)
}

// ---------------------------------------------------------------------------
// Usuarios
// ---------------------------------------------------------------------------

func (a *API) listarUsuarios(c *gin.Context) {
	u, err := a.Alm.Usuarios(c)
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, u)
}

func (a *API) guardarUsuario(c *gin.Context) {
	var in struct {
		Usuario  string `json:"usuario"`
		Nombre   string `json:"nombre" binding:"required"`
		Rol      string `json:"rol" binding:"required,oneof=admin operador consulta"`
		Activo   bool   `json:"activo"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"mensaje": "Datos incompletos"})
		return
	}
	u := almacen.Usuario{Usuario: strings.TrimSpace(in.Usuario), Nombre: in.Nombre, Rol: in.Rol, Activo: in.Activo}
	if in.Password != "" {
		if len(in.Password) < 8 {
			c.JSON(400, gin.H{"mensaje": "La contraseña debe tener al menos 8 caracteres"})
			return
		}
		h, err := seguridad.HashPassword(in.Password)
		if err != nil {
			fallo(c, err)
			return
		}
		u.Hash = h
	}
	if c.Param("id") == "" {
		if u.Usuario == "" || u.Hash == "" {
			c.JSON(400, gin.H{"mensaje": "Usuario y contraseña son obligatorios"})
			return
		}
		id, err := a.Alm.CrearUsuario(c, u)
		if err != nil {
			c.JSON(400, gin.H{"mensaje": "No se pudo crear (¿usuario repetido?)"})
			return
		}
		c.JSON(201, gin.H{"id": id})
		return
	}
	id, ok := idParam(c)
	if !ok {
		return
	}
	u.ID = id
	if id == usuario(c).ID && (u.Rol != "admin" || !u.Activo) {
		c.JSON(400, gin.H{"mensaje": "No puede quitarse a sí mismo el rol de administrador ni desactivarse"})
		return
	}
	if err := a.Alm.ActualizarUsuario(c, u); err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// ---------------------------------------------------------------------------
// Eventos en tiempo real (SSE)
// ---------------------------------------------------------------------------

func (a *API) eventos(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ch := a.Hub.Suscribir()
	defer a.Hub.Cancelar(ch)
	latido := time.NewTicker(25 * time.Second)
	defer latido.Stop()

	c.Writer.WriteString(": conectado\n\n")
	c.Writer.Flush()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-latido.C:
			c.Writer.WriteString(": latido\n\n")
			c.Writer.Flush()
		case e, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(e)
			fmt.Fprintf(c.Writer, "data: %s\n\n", b)
			c.Writer.Flush()
		}
	}
}

// ---------------------------------------------------------------------------
// Logo y explorador
// ---------------------------------------------------------------------------

var logoValido = regexp.MustCompile(`^data:image/(png|jpeg|webp);base64,[A-Za-z0-9+/=]+$`)

func (a *API) guardarLogo(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var in struct {
		Logo string `json:"logo"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || (in.Logo != "" && (!logoValido.MatchString(in.Logo) || len(in.Logo) > 300_000)) {
		c.JSON(400, gin.H{"mensaje": "Logo inválido (PNG, JPG o WEBP de hasta ~200 KB)"})
		return
	}
	if err := a.Alm.GuardarLogo(c, id, in.Logo); err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (a *API) explorador(c *gin.Context) {
	res, err := a.Alm.Resumen(c)
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"almacenamiento": a.Mot.Espacio(), "servidores": res, "retencionDefecto": a.RetencionDefecto})
}

func (a *API) diasServidor(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	d, err := a.Alm.DiasServidor(c, id, a.Zona.String())
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, d)
}

var diaValido = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func (a *API) respaldosDelDia(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	dia := c.Param("dia")
	if !diaValido.MatchString(dia) {
		c.JSON(400, gin.H{"mensaje": "Día inválido"})
		return
	}
	r, err := a.Alm.RespaldosDelDia(c, id, dia, a.Zona.String())
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, r)
}

// ---------------------------------------------------------------------------
// SSH y origenes de archivos
// ---------------------------------------------------------------------------

func (a *API) probarSSH(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	s, err := a.Alm.Servidor(c, id)
	if err != nil {
		fallo(c, err)
		return
	}
	huella, err := a.Mot.ProbarSSH(c, s)
	if err != nil {
		c.JSON(200, gin.H{"ok": false, "mensaje": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "huella": huella, "nueva": s.SSHHuella == ""})
}

func (a *API) listarOrigenes(c *gin.Context) {
	o, err := a.Alm.Origenes(c)
	if err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, o)
}

func (a *API) guardarOrigen(c *gin.Context) {
	var in almacen.OrigenArchivos
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"mensaje": "Datos inválidos"})
		return
	}
	in.ID = 0
	if c.Param("id") != "" {
		id, ok := idParam(c)
		if !ok {
			return
		}
		in.ID = id
	}
	in.Nombre = strings.TrimSpace(in.Nombre)
	in.Ruta = strings.TrimSpace(in.Ruta)
	in.Cron = strings.TrimSpace(in.Cron)
	if in.ID == 0 && !motor.NombreValido(in.Nombre) {
		c.JSON(400, gin.H{"mensaje": "Nombre inválido: sólo letras, números, guion y guion bajo (es la carpeta en la MSA)"})
		return
	}
	if !strings.HasPrefix(in.Ruta, "/") || strings.Contains(in.Ruta, "..") || in.Ruta == "/" {
		c.JSON(400, gin.H{"mensaje": "La ruta remota debe ser absoluta, p. ej. /home/hyper/CONGRESO-COMPRIMIDOS/descarga_congreso"})
		return
	}
	if in.Cron != "" {
		if err := programador.ValidarCron(in.Cron); err != nil {
			c.JSON(400, gin.H{"mensaje": "Expresión cron inválida: " + err.Error()})
			return
		}
	}
	if strings.TrimSpace(in.Patrones) == "" {
		in.Patrones = "*.tar.xz,*.tar.zst,*.tar.gz,*.tgz,*.snar,*.sha256"
	}
	if in.EstabilidadMin <= 0 {
		in.EstabilidadMin = 15
	}
	if in.RetencionDias < 0 {
		c.JSON(400, gin.H{"mensaje": "Retención inválida (0 = permanente)"})
		return
	}
	id, err := a.Alm.GuardarOrigen(c, in)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			c.JSON(400, gin.H{"mensaje": "Ya existe un origen con ese nombre en el servidor"})
			return
		}
		fallo(c, err)
		return
	}
	if err := a.Prog.Recargar(c); err != nil {
		fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"id": id})
}

func (a *API) eliminarOrigen(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := a.Alm.EliminarOrigen(c, id); err != nil {
		fallo(c, err)
		return
	}
	_ = a.Prog.Recargar(c)
	c.JSON(200, gin.H{"ok": true})
}

func (a *API) verRemoto(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	l, err := a.Mot.VerRemoto(c, id)
	if err != nil {
		c.JSON(502, gin.H{"mensaje": err.Error()})
		return
	}
	c.JSON(200, l)
}

func (a *API) copiarAhora(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	uid := usuario(c).ID
	if err := a.Mot.EncolarCopia(c, motor.SolicitudCopia{OrigenID: id, Origen: "manual", UsuarioID: &uid}); err != nil {
		c.JSON(400, gin.H{"mensaje": err.Error()})
		return
	}
	c.JSON(202, gin.H{"ok": true})
}
