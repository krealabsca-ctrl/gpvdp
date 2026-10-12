package bancos

// Dividir un movimiento entre partidas (mig 0094).
//
// «Del recibo de luz este mes se pagó 1.400.000, pero de luz normal eran 1.000.000, y esos 400.000
// corresponden a un proyecto especial.» El hecho bancario es uno; la atribución, dos.

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	// ErrMovimientoNoDivisible: el movimiento no está en condiciones de dividirse. Lo devuelve el
	// repositorio cuando el UPDATE no encuentra la fila, que a esa altura solo puede ser una
	// carrera: el service ya descartó antes las causas conocidas con su mensaje propio.
	ErrMovimientoNoDivisible = errors.New("bancos: ese movimiento ya no se puede dividir; recargá y volvé a intentar")
	// ErrMontoDivisionInvalido: lo que se aparta tiene que ser mayor que cero.
	ErrMontoDivisionInvalido = errors.New("bancos: el monto a separar tiene que ser mayor que cero")
	// ErrMontoDivisionExcede: apartar todo o más de lo que hay no es dividir.
	//
	// Apartar el monto COMPLETO dejaría el movimiento principal en cero, que no es una división
	// sino una reclasificación disfrazada —y para eso ya está clasificar—.
	ErrMontoDivisionExcede = errors.New("bancos: el monto a separar tiene que ser menor que el del movimiento; si querés moverlo entero, cambiale la clasificación")
	// ErrParteNoDivisible: una parte no se vuelve a dividir.
	//
	// Es una restricción de un nivel a propósito: con partes de partes, «padre + hijos» deja de
	// ser una suma y pasa a ser un árbol que hay que recorrer, y la invariante —que es lo único
	// que protege la plata— se vuelve mucho más difícil de comprobar. Para repartir en tres, se
	// divide dos veces el principal.
	ErrParteNoDivisible = errors.New("bancos: esto ya es una parte de otro movimiento; dividí el movimiento principal otra vez")
	// ErrDivisionConCxP: el movimiento está enlazado a una factura o a una remesa.
	//
	// El enlace afirma «este débito pagó esta factura por este monto». Partirlo haría que el monto
	// deje de calzar y la conciliación pasaría a mentir, sin que nada falle.
	ErrDivisionConCxP = errors.New("bancos: ese movimiento está enlazado a una factura de CxP; desenlazalo antes de dividirlo")
	// ErrDivisionSinClasificacion: la parte que se aparta necesita su propia partida, que es toda
	// la razón de separarla.
	ErrDivisionSinClasificacion = errors.New("bancos: decí a qué clasificación va la parte que separás")
	// ErrParteNoEncontrada: no existe esa parte, o ya se deshizo.
	ErrParteNoEncontrada = errors.New("bancos: no se encontró esa parte")
	// ErrMovimientoExcluidoNoSeDivide: lo que está fuera de los libros no se reparte.
	ErrMovimientoExcluidoNoSeDivide = errors.New("bancos: ese movimiento está fuera de los libros; devolvelo antes de dividirlo")
)

// DivisionInput es lo que se aparta y a qué partida va.
type DivisionInput struct {
	// Monto en la moneda del movimiento, y MontoCRC su equivalente en colones. Los dos porque el
	// movimiento lleva los dos, y recalcular uno a partir del otro acá sería una segunda fórmula
	// de conversión que puede discrepar de la del ERP.
	Monto    decimal.Decimal
	MontoCRC decimal.Decimal

	ConceptoID      string
	ClasificacionID string
	DepartamentoID  string
	SedeID          string
	Motivo          string
}

// ParteMovimiento es una de las caras de un movimiento dividido.
type ParteMovimiento struct {
	ID string `json:"id"`
	// EsPrincipal distingue el movimiento original de las partes que se le apartaron.
	EsPrincipal   bool   `json:"es_principal"`
	Monto         string `json:"monto"`
	EsDebito      bool   `json:"es_debito"`
	Concepto      string `json:"concepto"`
	Clasificacion string `json:"clasificacion"`
	Motivo        string `json:"motivo"`
	Incluido      bool   `json:"incluido"`
}

// Division es el reparto completo de un movimiento: el hecho bancario y sus partes.
type Division struct {
	MovimientoID string `json:"movimiento_id"`
	// El hecho bancario, que es UNO y no se reparte: la misma fecha, el mismo consecutivo y la
	// misma descripción para todas las partes.
	Fecha       string `json:"fecha"`
	Documento   string `json:"documento"`
	Descripcion string `json:"descripcion"`

	Partes []ParteMovimiento `json:"partes"`
	// MontoAntesDeDividir es lo que trajo el banco; SumaDeLasPartes, lo que suman las caras hoy.
	// Los dos viajan JUNTOS para que la pantalla pueda mostrar la invariante en vez de prometerla:
	// si alguna vez difieren, se ve.
	MontoAntesDeDividir string `json:"monto_antes_de_dividir"`
	SumaDeLasPartes     string `json:"suma_de_las_partes"`
}

// Cuadra dice si el reparto sigue sumando lo que trajo el banco.
//
// Es la invariante del módulo y por eso se contesta, no se asume. Un reparto que no cuadra es
// plata inventada o perdida, y tiene que poder verse en la pantalla.
func (d Division) Cuadra() bool {
	if d.MontoAntesDeDividir == "" || d.SumaDeLasPartes == "" {
		return true // sin dividir: no hay nada que cuadrar
	}
	antes, err1 := decimal.NewFromString(d.MontoAntesDeDividir)
	suma, err2 := decimal.NewFromString(d.SumaDeLasPartes)
	if err1 != nil || err2 != nil {
		return false
	}
	if antes.IsZero() {
		return true
	}
	return antes.Equal(suma)
}

// MovimientoDivisible es lo que hace falta saber antes de dividir.
type MovimientoDivisible struct {
	ID           string `json:"id"`
	Monto        string `json:"monto"`
	MontoCRC     string `json:"monto_crc"`
	Descripcion  string `json:"descripcion"`
	Incluido     bool   `json:"incluido"`
	EsParte      bool   `json:"es_parte"`
	EnlazadoACxP bool   `json:"enlazado_a_cxp"`
}

// PuedeDividirse devuelve el motivo por el que NO se puede, o nil.
//
// Cada causa tiene su propio error y su propia frase: «no se puede» a secas obliga a adivinar, y
// las cuatro causas tienen arreglos distintos —devolverlo a los libros, desenlazarlo de CxP,
// dividir el principal, o bajar el monto—.
func (m MovimientoDivisible) PuedeDividirse() error {
	switch {
	case !m.Incluido:
		return ErrMovimientoExcluidoNoSeDivide
	case m.EsParte:
		return ErrParteNoDivisible
	case m.EnlazadoACxP:
		return ErrDivisionConCxP
	}
	return nil
}
