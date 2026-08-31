package auth

import "time"

// Usuario representa la identidad de acceso. PasswordHash nunca se serializa a HTTP.
type Usuario struct {
	ID           string
	Nombre       string
	Email        string
	PasswordHash string
	Activo       bool
	// DebeCambiarPassword: contraseña temporal pendiente de cambio (primer ingreso / reset).
	DebeCambiarPassword bool
	// IntentosFallidos y BloqueadoHasta implementan el bloqueo por cuenta ante fuerza bruta.
	// BloqueadoHasta nil o en el pasado ⇒ la cuenta no está bloqueada.
	IntentosFallidos int
	BloqueadoHasta   *time.Time
}

// Membership es la pertenencia de un usuario a una empresa con un rol.
type Membership struct {
	EmpresaID     string
	EmpresaNombre string
	RolID         string
	RolCodigo     string
}

// LoginResult es el resultado del login: tokens + identidad + empresas accesibles.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	Usuario      Usuario
	Empresas     []Membership
}

// MeResult describe el estado de la sesión actual.
type MeResult struct {
	Usuario             Usuario
	Empresas            []Membership
	EmpresaActivaID     string
	Rol                 string
	DebeCambiarPassword bool
}
