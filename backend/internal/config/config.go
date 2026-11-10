// Package config lee la configuracion del entorno. Todo lo sensible (claves,
// contraseñas) llega por variables de entorno, nunca por codigo.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Puerto           string
	DB               DBConfig
	JWTSecreto       string
	JWTDuracion      time.Duration
	ClaveCifrado     string // 32 bytes en base64 o texto; cifra las contraseñas de los servidores MySQL
	DirRespaldos     string // punto de montaje del NAS/NFS
	Zona             *time.Location
	MaxConcurrencia  int
	TimeoutRespaldo  time.Duration
	AdminUsuario     string
	AdminPassword    string
	OrigenesCORS     []string
	MySQLSinSSL      bool
	RetencionDefecto int // dias; 0 = permanente
}

// DBConfig son las credenciales de la base administrativa (MySQL/MariaDB).
type DBConfig struct {
	Host     string
	Puerto   int
	Usuario  string
	Password string
	Nombre   string
}

func Cargar() (*Config, error) {
	zonaNombre := env("TZ", "America/Mexico_City")
	zona, err := time.LoadLocation(zonaNombre)
	if err != nil {
		return nil, fmt.Errorf("zona horaria %q: %w", zonaNombre, err)
	}

	c := &Config{
		Puerto: env("PORT", "3060"),
		DB: DBConfig{
			Host:     env("DB_HOST", "db"),
			Puerto:   entero("DB_PUERTO", 3306),
			Usuario:  env("DB_USUARIO", "sirespaldos"),
			Password: os.Getenv("DB_PASSWORD"),
			Nombre:   env("DB_NOMBRE", "sirespaldos"),
		},
		JWTSecreto:       os.Getenv("JWT_SECRETO"),
		JWTDuracion:      duracion("JWT_DURACION", 10*time.Hour),
		ClaveCifrado:     os.Getenv("CLAVE_CIFRADO"),
		DirRespaldos:     env("DIR_RESPALDOS", "/respaldos"),
		Zona:             zona,
		MaxConcurrencia:  entero("MAX_CONCURRENCIA", 2),
		TimeoutRespaldo:  duracion("TIMEOUT_RESPALDO", 6*time.Hour),
		AdminUsuario:     env("ADMIN_USUARIO", "admin"),
		AdminPassword:    os.Getenv("ADMIN_PASSWORD"),
		OrigenesCORS:     []string{env("CORS_ORIGEN", "http://localhost:4200")},
		MySQLSinSSL:      env("MYSQL_SIN_SSL", "true") == "true",
		RetencionDefecto: retencion("RETENCION_DEFECTO_DIAS", 730),
	}

	if c.DB.Password == "" {
		return nil, fmt.Errorf("falta DB_PASSWORD (credenciales de la base administrativa MySQL)")
	}
	if len(c.JWTSecreto) < 32 {
		return nil, fmt.Errorf("JWT_SECRETO debe tener al menos 32 caracteres")
	}
	if c.ClaveCifrado == "" {
		return nil, fmt.Errorf("falta CLAVE_CIFRADO")
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func entero(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}

func duracion(k string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}

// retencion admite 0 (permanente), a diferencia de entero().
func retencion(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v >= 0 {
		return v
	}
	return def
}
