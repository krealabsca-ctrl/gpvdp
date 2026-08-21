package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims son los claims del access token. La empresa activa viaja DENTRO del token
// (claim empresa_id), nunca en el body ni en el query del cliente.
type Claims struct {
	jwt.RegisteredClaims
	Email     string `json:"email"`
	EmpresaID string `json:"empresa_id,omitempty"`
	Rol       string `json:"rol,omitempty"`
	Tipo      string `json:"tipo"`
}

// UsuarioID devuelve el id del usuario (claim sub).
func (c *Claims) UsuarioID() string { return c.Subject }

// MintAccessToken firma un access token HS256. empresaID/rol vacíos => token sin empresa.
func MintAccessToken(secret string, ttl time.Duration, usuarioID, email, empresaID, rol string) (string, error) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   usuarioID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Email:     email,
		EmpresaID: empresaID,
		Rol:       rol,
		Tipo:      "access",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("auth: firmar token: %w", err)
	}
	return signed, nil
}

// ParseAccessToken valida firma, expiración y tipo, y devuelve los claims.
func ParseAccessToken(secret, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrTokenInvalido
		}
		return []byte(secret), nil
	})
	if err != nil || !tok.Valid {
		return nil, ErrTokenInvalido
	}
	if claims.Tipo != "access" {
		return nil, ErrTokenInvalido
	}
	return claims, nil
}
