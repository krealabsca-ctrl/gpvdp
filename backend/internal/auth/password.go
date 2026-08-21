package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword genera un hash bcrypt de la contraseña en claro.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash de contraseña: %w", err)
	}
	return string(b), nil
}

// VerifyPassword compara una contraseña en claro contra su hash bcrypt.
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
