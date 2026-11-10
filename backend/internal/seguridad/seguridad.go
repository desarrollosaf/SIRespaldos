// Package seguridad agrupa JWT, hashes de contraseña y el cifrado de las
// credenciales de los servidores MySQL.
package seguridad

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------------------------
// Contraseñas de usuarios
// ---------------------------------------------------------------------------

func HashPassword(p string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(h), err
}

func VerificarPassword(hash, p string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

// ---------------------------------------------------------------------------
// Cifrado AES-256-GCM para contraseñas de MySQL guardadas en la base administrativa
// ---------------------------------------------------------------------------

type Cifrador struct{ aead cipher.AEAD }

// NuevoCifrador deriva una clave de 32 bytes con SHA-256 de la clave dada,
// asi CLAVE_CIFRADO puede ser cualquier cadena larga y aleatoria.
func NuevoCifrador(clave string) (*Cifrador, error) {
	k := sha256.Sum256([]byte(clave))
	bloque, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(bloque)
	if err != nil {
		return nil, err
	}
	return &Cifrador{aead: aead}, nil
}

func (c *Cifrador) Cifrar(texto string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := c.aead.Seal(nonce, nonce, []byte(texto), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

func (c *Cifrador) Descifrar(b64 string) (string, error) {
	if b64 == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	n := c.aead.NonceSize()
	if len(data) < n {
		return "", errors.New("texto cifrado invalido")
	}
	plano, err := c.aead.Open(nil, data[:n], data[n:], nil)
	if err != nil {
		return "", fmt.Errorf("no se pudo descifrar (¿cambio CLAVE_CIFRADO?): %w", err)
	}
	return string(plano), nil
}

// ---------------------------------------------------------------------------
// JWT
// ---------------------------------------------------------------------------

type Claims struct {
	UsuarioID int    `json:"uid"`
	Rol       string `json:"rol"`
	Nombre    string `json:"nom"`
	jwt.RegisteredClaims
}

type Tokens struct {
	secreto  []byte
	duracion time.Duration
}

func NuevosTokens(secreto string, duracion time.Duration) *Tokens {
	return &Tokens{secreto: []byte(secreto), duracion: duracion}
}

func (t *Tokens) Emitir(uid int, rol, nombre string) (string, error) {
	ahora := time.Now()
	c := Claims{
		UsuarioID: uid, Rol: rol, Nombre: nombre,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(ahora),
			ExpiresAt: jwt.NewNumericDate(ahora.Add(t.duracion)),
			Issuer:    "sirespaldos",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secreto)
}

func (t *Tokens) Validar(token string) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(token, c, func(*jwt.Token) (any, error) { return t.secreto, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("sirespaldos"))
	if err != nil {
		return nil, err
	}
	return c, nil
}
