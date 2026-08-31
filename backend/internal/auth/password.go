package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost es el costo de trabajo de bcrypt. 12 (≈250 ms) endurece la fuerza bruta
// offline frente al default 10 sin penalizar el login interactivo. Los hashes viejos
// (cost 10) siguen validando: bcrypt guarda el costo dentro del propio hash.
const bcryptCost = 12

// dummyHash es un hash bcrypt fijo (mismo costo que HashPassword). Se compara contra él
// cuando el usuario no existe o está inactivo, para que el login tarde lo mismo exista o
// no la cuenta: sin esto, la respuesta rápida cuando el email no existe (no corre bcrypt)
// vs. la lenta cuando sí (corre bcrypt) es un oráculo de enumeración de usuarios por timing.
var dummyHash []byte

func init() {
	// Se computa una vez al arrancar. Si fallara (no debería), VerifyDummy no paga costo,
	// pero el arranque igual continúa.
	dummyHash, _ = bcrypt.GenerateFromPassword([]byte("gpvdp-timing-equalizer"), bcryptCost)
}

// HashPassword genera un hash bcrypt de la contraseña en claro.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash de contraseña: %w", err)
	}
	return string(b), nil
}

// VerifyPassword compara una contraseña en claro contra su hash bcrypt.
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// VerifyDummy paga el costo de un bcrypt contra un hash fijo y descarta el resultado.
// Se llama en los caminos de login que NO deben revelar por qué fallaron (usuario
// inexistente o inactivo), para igualar el tiempo de respuesta con el de una contraseña
// incorrecta de un usuario real.
func VerifyDummy(plain string) {
	if dummyHash != nil {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(plain))
	}
}
