package cxp

// Nomenclatura interna del pago: el código con el que la empresa reconoce, mirando el estado de
// cuenta, a qué corresponde cada débito («A8» comisiones, «A9» planilla).
//
// Vive en el catálogo de gasto (migración 0090) y se HEREDA: la factura ya tiene su gasto, así que
// el código sale solo. Nadie lo teclea factura por factura, que es justo lo que haría que la mitad
// de los pagos salieran sin él.

import (
	"errors"
	"regexp"
	"strings"
)

// ErrCodigoPagoInvalido: la forma del código no sirve para viajar al banco.
var ErrCodigoPagoInvalido = errors.New(
	"cxp: el código tiene que ser de 1 a 4 letras o números (por ejemplo «A8», «A9» o «PLA»)")

// ErrCodigoPagoDuplicado: ese código ya identifica a otro rubro en esta empresa.
var ErrCodigoPagoDuplicado = errors.New(
	"cxp: ese código ya está usado por otro rubro; un código significa UNA sola cosa")

// reCodigoPago: 1 a 4 alfanuméricos. Corto porque comparte el campo del banco con la referencia y
// el código interno, y ese campo no es infinito.
var reCodigoPago = regexp.MustCompile(`^[A-Z0-9]{1,4}$`)

// NormalizarCodigoPago limpia lo que escribió una persona: espacios de más y minúsculas.
//
// Mayúsculas SIEMPRE, y no por estética: el código viaja dentro de la descripción del banco, que
// la devuelve en mayúsculas. Si se guardara «a8», lo emitido y lo que vuelve no se verían iguales
// y nadie podría cruzarlos a ojo.
func NormalizarCodigoPago(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// ValidarCodigoPago acepta el vacío (quitar el código) y exige la forma en lo demás.
func ValidarCodigoPago(s string) (string, error) {
	n := NormalizarCodigoPago(s)
	if n == "" {
		return "", nil
	}
	if !reCodigoPago.MatchString(n) {
		return "", ErrCodigoPagoInvalido
	}
	return n, nil
}

// CodigoDePago resuelve qué código le corresponde a una factura.
//
// Lo MÁS ESPECÍFICO gana: si la clasificación tiene el suyo, manda sobre el del concepto. Es la
// misma precedencia que ya usa la marca «de Contabilidad», así que no hay una regla nueva que
// aprender. Vacío = este gasto todavía no tiene código, y el pago sale igual: la nomenclatura
// sirve para leer más rápido, no es un requisito que trabe la operación.
func CodigoDePago(codigoClasificacion, codigoConcepto string) string {
	if c := NormalizarCodigoPago(codigoClasificacion); c != "" {
		return c
	}
	return NormalizarCodigoPago(codigoConcepto)
}

// RubroConCodigo es una línea del catálogo de nomenclaturas: qué significa cada código.
//
// Sirve para la pantalla que los administra y para imprimir la tabla de equivalencias, que es lo
// que alguien necesita al lado cuando lee un estado de cuenta.
type RubroConCodigo struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	// Nivel: "CONCEPTO" o "CLASIFICACION". El de la clasificación pisa al de su concepto.
	Nivel string `json:"nivel"`
	// Concepto es el padre cuando Nivel es CLASIFICACION (vacío si es el concepto mismo).
	Concepto string `json:"concepto"`
	Codigo   string `json:"codigo"`
	// Heredado: true cuando la clasificación NO tiene código propio y usa el de su concepto. La
	// pantalla tiene que distinguirlo: borrar un código heredado no hace nada, y sin la marca eso
	// se lee como que el sistema ignoró el cambio.
	Heredado bool `json:"heredado"`
	// Facturas es cuántas usan este rubro hoy: dice qué códigos conviene cargar primero.
	Facturas int `json:"facturas"`
}

// Niveles del catálogo donde puede vivir un código.
const (
	NivelConcepto      = "CONCEPTO"
	NivelClasificacion = "CLASIFICACION"
)

// NivelCatalogoValido acota lo que llega del cliente: decide en qué TABLA se escribe.
func NivelCatalogoValido(n string) bool {
	return n == NivelConcepto || n == NivelClasificacion
}
