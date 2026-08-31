package auth

import "errors"

// Errores de dominio del paquete auth (tipados, comparables con errors.Is).
var (
	ErrCredenciales    = errors.New("auth: credenciales inválidas")
	ErrUsuarioInactivo = errors.New("auth: usuario inactivo")
	ErrSinAcceso       = errors.New("auth: el usuario no tiene acceso a la empresa")
	ErrTokenInvalido   = errors.New("auth: token inválido o expirado")
	ErrRefreshInvalido = errors.New("auth: refresh token inválido, revocado o expirado")
	ErrPasswordDebil   = errors.New("auth: la contraseña debe tener al menos 8 caracteres")
	// ErrCuentaBloqueada: demasiados intentos fallidos seguidos; la cuenta queda bloqueada
	// por una ventana de tiempo. Frena la fuerza bruta por cuenta (complementa el límite por IP).
	ErrCuentaBloqueada = errors.New("auth: cuenta bloqueada temporalmente por intentos fallidos")
)
