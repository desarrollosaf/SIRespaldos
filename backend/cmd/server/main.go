package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sirespaldos/internal/almacen"
	"sirespaldos/internal/api"
	"sirespaldos/internal/config"
	"sirespaldos/internal/eventos"
	"sirespaldos/internal/motor"
	"sirespaldos/internal/programador"
	"sirespaldos/internal/seguridad"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := ejecutar(); err != nil {
		slog.Error("el servicio terminó con error", "err", err)
		os.Exit(1)
	}
}

func ejecutar() error {
	cfg, err := config.Cargar()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	alm, err := almacen.Abrir(ctx, almacen.Conexion{
		Host: cfg.DB.Host, Puerto: cfg.DB.Puerto, Usuario: cfg.DB.Usuario, Password: cfg.DB.Password, Base: cfg.DB.Nombre,
	})
	if err != nil {
		return err
	}
	defer alm.DB.Close()

	if err := alm.MarcarInterrumpidos(ctx); err != nil {
		return err
	}
	if err := sembrarAdmin(ctx, alm, cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DirRespaldos, 0o750); err != nil {
		return err
	}

	cif, err := seguridad.NuevoCifrador(cfg.ClaveCifrado)
	if err != nil {
		return err
	}
	hub := eventos.NuevoHub()
	mot := motor.Nuevo(alm, cif, hub, motor.Opciones{
		Dir: cfg.DirRespaldos, Timeout: cfg.TimeoutRespaldo, MaxConcurrencia: cfg.MaxConcurrencia,
		SinSSL: cfg.MySQLSinSSL, Zona: cfg.Zona,
	})
	prog := programador.Nuevo(alm, mot, cfg.Zona)
	if err := prog.Iniciar(ctx); err != nil {
		return err
	}

	a := &api.API{Alm: alm, Mot: mot, Prog: prog, Hub: hub, Cif: cif, Zona: cfg.Zona, CORS: cfg.OrigenesCORS, RetencionDefecto: cfg.RetencionDefecto,
		Tokens: seguridad.NuevosTokens(cfg.JWTSecreto, cfg.JWTDuracion)}
	srv := &http.Server{Addr: ":" + cfg.Puerto, Handler: a.Rutas(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		slog.Info("SIRespaldos escuchando", "puerto", cfg.Puerto, "respaldos", cfg.DirRespaldos)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("servidor http", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.Info("deteniendo…")
	<-prog.Detener().Done()
	apagar, fin := context.WithTimeout(context.Background(), 10*time.Second)
	defer fin()
	return srv.Shutdown(apagar)
}

// sembrarAdmin crea el primer administrador si la tabla de usuarios esta vacia.
func sembrarAdmin(ctx context.Context, alm *almacen.Almacen, cfg *config.Config) error {
	n, err := alm.ContarUsuarios(ctx)
	if err != nil || n > 0 {
		return err
	}
	if len(cfg.AdminPassword) < 8 {
		return errors.New("no hay usuarios: defina ADMIN_PASSWORD (mínimo 8 caracteres) para crear el administrador inicial")
	}
	h, err := seguridad.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	_, err = alm.CrearUsuario(ctx, almacen.Usuario{Usuario: cfg.AdminUsuario, Nombre: "Administrador", Hash: h, Rol: "admin", Activo: true})
	if err == nil {
		slog.Info("administrador inicial creado", "usuario", cfg.AdminUsuario)
	}
	return err
}
