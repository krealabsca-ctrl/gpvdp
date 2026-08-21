// Package httpx contiene utilidades HTTP compartidas: contrato de error y validación.
package httpx

import (
	"github.com/gin-gonic/gin"
)

// ErrorResponse es el contrato uniforme de error hacia el cliente.
// Coincide con el schema ErrorResponse del OpenAPI.
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Códigos de error estables (los consume el frontend para decidir la UI).
const (
	CodeValidacion    = "VALIDACION"
	CodeNoAutenticado = "NO_AUTENTICADO"
	CodeSinPermiso    = "SIN_PERMISO"
	CodeNoEncontrado  = "NO_ENCONTRADO"
	CodeConflicto     = "CONFLICTO"
	CodeReglaNegocio  = "REGLA_NEGOCIO"
	CodeCredenciales  = "CREDENCIALES_INVALIDAS"
	CodeEmpresaNoSel  = "EMPRESA_NO_SELECCIONADA"
	CodeErrorInterno  = "ERROR_INTERNO"
)

// Abort responde con el contrato de error y detiene la cadena de handlers.
func Abort(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{Code: code, Message: message})
}
