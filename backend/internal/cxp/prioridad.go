package cxp

// Las prioridades de pago, configurables por empresa (mig 0121).
//
// «Que se pueda agregar nuevas prioridades.» Hasta la 0121 eran tres valores escritos en un CHECK
// de la base, así que una cuarta exigía migración y despliegue: una decisión del negocio que
// necesitaba un programador, igual que los formatos de banco antes de la 0108.
//
// EL ORDEN Y LA URGENCIA SON COSAS DISTINTAS. `Orden` manda en cómo se ordenan las listas; `Urgente`
// dice si esa prioridad cuenta en «lo que se paga sí o sí» —el tablero, el filtro y la premarca del
// corte—. Derivar la segunda de la primera («la más alta es la urgente») haría que crear una
// prioridad nueva por encima dejara de contar la anterior en silencio.

import (
	"errors"
	"strings"
)

var (
	// ErrPrioridadNoEncontrada: no existe en esta empresa.
	ErrPrioridadNoEncontrada = errors.New("cxp: esa prioridad no existe en esta empresa")
	// ErrCodigoPrioridadRequerido: el código es lo que queda escrito en la factura.
	ErrCodigoPrioridadRequerido = errors.New("cxp: el código de la prioridad va de 1 a 6 caracteres")
	// ErrNombrePrioridadRequerido: el código solo no explica nada. «AA» no dice si se paga ya o si
	// puede esperar; el nombre es lo que lee quien prioriza una factura.
	ErrNombrePrioridadRequerido = errors.New("cxp: ponele un nombre que explique cuándo usarla (al menos 3 caracteres)")
	// ErrCodigoPrioridadRepetido: el código ya está en uso en esta empresa.
	//
	// Importa más que en otros catálogos porque el documento guarda el CÓDIGO: dos prioridades con
	// el mismo código serían indistinguibles en las facturas ya marcadas.
	ErrCodigoPrioridadRepetido = errors.New("cxp: ya hay una prioridad con ese código en esta empresa")
	// ErrTonoPrioridadInvalido: el tono sale de la paleta del sistema.
	ErrTonoPrioridadInvalido = errors.New("cxp: el color no es uno de los del sistema")
	// ErrPrioridadEnUso: se quiso eliminar una que ya está puesta en facturas.
	//
	// Se desactiva en vez de borrarse, igual que el resto del catálogo: borrarla dejaría facturas
	// con un código que ya no se puede explicar.
	ErrPrioridadEnUso = errors.New("cxp: esa prioridad ya está puesta en facturas; desactivala en vez de eliminarla")
)

// tonosPrioridad son los tonos de la paleta del sistema.
//
// Se limita a estos y no se admite un color libre para que una prioridad nueva se vea como el
// resto del sistema y siga el tema claro/oscuro. Un hexadecimal elegido a mano se ve bien en un
// tema y desaparece en el otro.
var tonosPrioridad = map[string]bool{
	"neutral": true, "accent": true, "positivo": true, "negativo": true, "pendiente": true,
}

// Prioridad es una prioridad de pago del catálogo de una empresa.
type Prioridad struct {
	ID     string `json:"id"`
	Codigo string `json:"codigo"`
	Nombre string `json:"nombre"`
	// Orden: mayor = más arriba en las listas.
	Orden int `json:"orden"`
	// Urgente: cuenta en «lo que se paga sí o sí».
	Urgente bool   `json:"urgente"`
	Tono    string `json:"tono"`
	Activa  bool   `json:"activa"`
	// EnUso dice en cuántas facturas está puesta. La pantalla la usa para explicar por qué una
	// prioridad no se puede eliminar, antes de que alguien lo intente.
	EnUso int `json:"en_uso"`
}

// PrioridadInput es lo que llega al crear o editar.
type PrioridadInput struct {
	Codigo  string `json:"codigo"`
	Nombre  string `json:"nombre"`
	Orden   int    `json:"orden"`
	Urgente bool   `json:"urgente"`
	Tono    string `json:"tono"`
	Activa  bool   `json:"activa"`
}

// limpiar normaliza y valida. El código va en MAYÚSCULAS porque es una insignia de dos letras en
// una tabla: «aa» y «AA» serían dos prioridades distintas que nadie puede distinguir de un vistazo.
func (in *PrioridadInput) limpiar() error {
	in.Codigo = strings.ToUpper(strings.TrimSpace(in.Codigo))
	in.Nombre = strings.TrimSpace(in.Nombre)
	in.Tono = strings.TrimSpace(in.Tono)
	if in.Tono == "" {
		in.Tono = "pendiente"
	}
	if n := len([]rune(in.Codigo)); n < 1 || n > 6 {
		return ErrCodigoPrioridadRequerido
	}
	if len([]rune(in.Nombre)) < 3 {
		return ErrNombrePrioridadRequerido
	}
	if !tonosPrioridad[in.Tono] {
		return ErrTonoPrioridadInvalido
	}
	return nil
}
