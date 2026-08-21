package cxp

import (
	"regexp"
	"strings"
)

// reHuella reconoce la huella que CxP embebe en la descripción del pago (ver generarHuella).
var reHuella = regexp.MustCompile(`CXP-[A-Z0-9]{12}`)

// extraerHuella busca la huella CxP dentro de la descripción de un movimiento bancario.
func extraerHuella(descripcion string) (string, bool) {
	m := reHuella.FindString(strings.ToUpper(descripcion))
	if m == "" {
		return "", false
	}
	return m, true
}

// PagoRow es una línea del archivo de pago (formato SINPE: cédula, nombre, IBAN, monto neto).
type PagoRow struct {
	Cedula      string `json:"cedula"`
	Nombre      string `json:"nombre"`
	IBAN        string `json:"iban"`
	Moneda      string `json:"moneda"`
	MontoNeto   string `json:"monto_neto"`  // total − retención (lo que recibe el proveedor)
	Descripcion string `json:"descripcion"` // la huella, para conciliar contra el banco
	Consecutivo string `json:"consecutivo"`
	DocumentoID string `json:"documento_id"`
}
