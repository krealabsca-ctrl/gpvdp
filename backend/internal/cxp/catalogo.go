package cxp

// Subclasificacion es el 3er nivel del catálogo de gasto (cuelga de una Clasificación).
// Exclusivo de CxP: el módulo Bancos usa solo Concepto › Clasificación.
type Subclasificacion struct {
	ID              string `json:"id"`
	ClasificacionID string `json:"clasificacion_id"`
	Nombre          string `json:"nombre"`
}
