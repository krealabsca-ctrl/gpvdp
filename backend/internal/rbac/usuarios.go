package rbac

import "errors"

var (
	// ErrEmailDuplicado indica que ya existe un usuario con ese correo.
	ErrEmailDuplicado = errors.New("rbac: ya existe un usuario con ese correo")
	// ErrUsuarioNoEncontrado indica que el usuario no existe o no tiene acceso a la empresa activa.
	ErrUsuarioNoEncontrado = errors.New("rbac: usuario no encontrado en esta empresa")
)

// UsuarioAdmin es la vista de un usuario para la administración, en el contexto de una empresa.
type UsuarioAdmin struct {
	ID          string `json:"id"`
	Nombre      string `json:"nombre"`
	Email       string `json:"email"`
	Activo      bool   `json:"activo"`
	DebeCambiar bool   `json:"debe_cambiar_password"`
	RolCodigo   string `json:"rol_codigo"`
	RolNombre   string `json:"rol_nombre"`
}
