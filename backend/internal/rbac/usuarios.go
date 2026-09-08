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
	// OtrasEmpresas son las OTRAS empresas del grupo a las que este usuario entra, separadas por
	// « · ». Vacío = solo esta. El acceso a cada empresa es una MEMBRESÍA (una fila en
	// usuario_empresa_rol), no un permiso: por eso no aparece en la matriz de permisos, y por eso
	// hace falta verlo acá.
	OtrasEmpresas string `json:"otras_empresas"`
}
