package cxp

import "errors"

var (
	// ErrDepartamentoNoEncontrado indica que el departamento no existe o no es de la empresa.
	ErrDepartamentoNoEncontrado = errors.New("cxp: departamento no encontrado")
	// ErrDepartamentoDuplicado indica que ya existe un departamento con ese nombre en la empresa.
	ErrDepartamentoDuplicado = errors.New("cxp: ya existe un departamento con ese nombre")
)

// Departamento es un área/centro de costo de la empresa (catálogo administrable).
type Departamento struct {
	ID          string `json:"id"`
	Nombre      string `json:"nombre"`
	Codigo      string `json:"codigo"`
	CentroCosto string `json:"centro_costo"`
	Activo      bool   `json:"activo"`
}

// DepartamentoInput son los datos para crear/editar un departamento.
type DepartamentoInput struct {
	Nombre      string
	Codigo      string
	CentroCosto string
}

// Validador es un responsable (titular/suplente) de validar las facturas de un departamento.
type Validador struct {
	UsuarioID string `json:"usuario_id"`
	Nombre    string `json:"nombre"`
	Email     string `json:"email"`
	Rol       string `json:"rol"` // TITULAR | SUPLENTE
}

// UsuarioRef es un usuario que opera la empresa (para poblar el selector de validadores).
type UsuarioRef struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Email  string `json:"email"`
}

// DepartamentosBase es el set inicial que se siembra por empresa (editable luego en la UI).
var DepartamentosBase = []string{
	"Logística",
	"Operaciones",
	"Servicio / Sala",
	"Ventas",
	"Comercial",
	"Mercadeo",
	"Cobros",
	"Finanzas",
	"Administración",
	"Gerencia",
	"Tecnología",
	"Recursos Humanos",
	"Mantenimiento",
}
