// Package inventario controla las existencias de producto físico —cofres, urnas y suministros—
// repartidas entre las sedes de la empresa.
//
// ── Las dos reglas que gobiernan todo el paquete ─────────────────────────────
//
//  1. **Dos formas de contar.** Un artículo se controla POR UNIDAD (cada objeto físico tiene su
//     ficha, su costo y su ubicación) o POR CANTIDAD (solo se cuenta). El cofre es lo primero
//     porque vale entre ₡70.000 y ₡423.750 y hay que saber cuál se usó en cuál servicio; la urna
//     es lo segundo porque se pide cada semana y no tiene identidad propia.
//
//  2. **La existencia se DERIVA del libro de movimientos.** Nunca se guarda un contador. Es el
//     mismo principio que el saldo diario en Bancos: un número mantenido a mano se desincroniza y
//     entonces hay dos verdades. Acá cada cifra se puede abrir y ver de qué movimientos está hecha.
package inventario

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// Modos de control de un artículo.
const (
	ModoUnidad   = "UNIDAD"
	ModoCantidad = "CANTIDAD"
)

// Estados de una unidad física.
const (
	EstadoDisponible = "DISPONIBLE"
	EstadoReservada  = "RESERVADA"
	EstadoEnTransito = "EN_TRANSITO"
	EstadoExhibicion = "EXHIBICION"
	EstadoUsada      = "USADA"
	EstadoDanada     = "DANADA"
	EstadoDevuelta   = "DEVUELTA"
	// El conteo no la encontró. Distinto de DANADA: no se sabe qué le pasó, y decir «daño»
	// afirmaría un hecho que nadie comprobó. El motivo que escribió quien contó queda en el
	// movimiento de baja.
	EstadoNoAparecio = "NO_APARECIO"
)

// Tipos de movimiento. El signo lo determina el tipo, nunca la cantidad —que siempre es positiva—:
// guardar negativos obliga a que cada consulta recuerde el signo, y basta que una se olvide para
// que el total mienta.
const (
	MovEntrada         = "ENTRADA"
	MovSalida          = "SALIDA"
	MovTrasladoSalida  = "TRASLADO_SALIDA"
	MovTrasladoEntrada = "TRASLADO_ENTRADA"
	// El ajuste son DOS tipos: sobraban o faltaban. Un solo «AJUSTE» obligaría a guardar
	// cantidades negativas y a que cada consulta se acordara del signo.
	MovAjusteMas   = "AJUSTE_MAS"
	MovAjusteMenos = "AJUSTE_MENOS"
	MovBaja        = "BAJA"
	MovDevolucion  = "DEVOLUCION"
)

// Estados de un traslado.
const (
	TrasladoEnTransito = "EN_TRANSITO"
	TrasladoRecibido   = "RECIBIDO"
	TrasladoCancelado  = "CANCELADO"
)

// sumaAlStock dice si un tipo de movimiento aumenta o disminuye la existencia de la sede que
// menciona. Está en un solo lugar a propósito: si cada consulta decidiera el signo por su cuenta,
// dos pantallas podrían mostrar existencias distintas del mismo artículo.
func sumaAlStock(tipo string) bool {
	switch tipo {
	case MovEntrada, MovTrasladoEntrada, MovAjusteMas:
		return true
	default:
		return false
	}
}

// sqlSignoMovimiento es la misma regla expresada en SQL, para que las consultas de existencia y el
// código de Go no puedan discrepar nunca.
const sqlSignoMovimiento = `CASE WHEN m.tipo IN ('ENTRADA','TRASLADO_ENTRADA','AJUSTE_MAS')
                                 THEN m.cantidad ELSE -m.cantidad END`

// sqlPromedioPonderado es el costo unitario de un artículo que se lleva por cantidad: el promedio
// ponderado de sus entradas. La fórmula se escribe UNA vez porque se lee de dos formas distintas
// —correlacionada dentro de una consulta grande, y suelta con parámetros dentro de una transacción—
// y si cada una llevara su propia copia, el costo con el que algo SALE del inventario podría dejar
// de ser el costo con el que se valoriza lo que QUEDA. Espera la tabla aliaseada como `e`.
const sqlPromedioPonderado = `SUM(e.cantidad * e.costo_unitario_crc) / NULLIF(SUM(e.cantidad), 0)`

// sqlSumaSalidasDelServicio es cuánto costó lo que un servicio sacó de bodega: la base sobre la que
// se mediría el margen. Vive acá porque la calculan dos lugares —el POST que registra el servicio y
// la pantalla que lista los servicios— y un servicio no puede valer dos cosas según dónde se mire.
// Espera la tabla aliaseada como `m`.
const sqlSumaSalidasDelServicio = `SUM(m.cantidad * m.costo_unitario_crc)`

// Errores de dominio.
var (
	// ErrArticuloNoEncontrado indica que el artículo no existe o no es de la empresa.
	ErrArticuloNoEncontrado = errors.New("inventario: artículo no encontrado")
	// ErrCategoriaNoEncontrada indica que la categoría no existe o no es de la empresa.
	ErrCategoriaNoEncontrada = errors.New("inventario: categoría no encontrada")
	// ErrUnidadNoEncontrada indica que la unidad no existe o no es de la empresa.
	ErrUnidadNoEncontrada = errors.New("inventario: unidad no encontrada")
	// ErrTrasladoNoEncontrado indica que el traslado no existe o no es de la empresa.
	ErrTrasladoNoEncontrado = errors.New("inventario: traslado no encontrado")
	// ErrServicioNoEncontrado indica que el servicio no existe o no es de la empresa.
	ErrServicioNoEncontrado = errors.New("inventario: servicio no encontrado")
	// ErrNombreRequerido indica que falta el nombre.
	ErrNombreRequerido = errors.New("inventario: el nombre es obligatorio")
	// ErrCodigoRequerido indica que falta el código del artículo.
	ErrCodigoRequerido = errors.New("inventario: el código del artículo es obligatorio")
	// ErrModoInvalido indica un modo de control que no existe.
	ErrModoInvalido = errors.New("inventario: el control tiene que ser por unidad o por cantidad")
	// ErrCantidadInvalida indica una cantidad que no tiene sentido.
	ErrCantidadInvalida = errors.New("inventario: la cantidad tiene que ser mayor que cero")
	// ErrCostoNegativo indica un costo imposible.
	ErrCostoNegativo = errors.New("inventario: el costo no puede ser negativo")
	// ErrSedeRequerida indica que falta decir en qué sede pasa el hecho.
	ErrSedeRequerida = errors.New("inventario: hace falta indicar la sede")
	// ErrDuplicado indica que ya existe algo con ese código o nombre.
	ErrDuplicado = errors.New("inventario: ya existe una entrada con ese código o nombre")
	// ErrMotivoRequerido indica que un ajuste o una baja llegó sin explicación. No es un formalismo:
	// un ajuste sin motivo es exactamente el movimiento que después nadie puede auditar.
	ErrMotivoRequerido = errors.New("inventario: un ajuste o una baja necesita motivo")
	// ErrMismaSede indica un traslado de una sede a sí misma.
	ErrMismaSede = errors.New("inventario: el origen y el destino tienen que ser sedes distintas")
	// ErrTrasladoYaRecibido indica que se quiso recibir dos veces el mismo traslado.
	ErrTrasladoYaRecibido = errors.New("inventario: ese traslado ya se recibió")
	// ErrFechaInvalida indica una fecha mal escrita.
	ErrFechaInvalida = errors.New("inventario: fecha inválida (se espera AAAA-MM-DD)")
	// ErrServicioSinConsumos indica un servicio que no dice qué sacó de bodega. Es una regla de
	// negocio, no un fallo: el servicio existe en este módulo para descargar el inventario, así que
	// uno sin consumos no tiene nada que registrar acá.
	ErrServicioSinConsumos = errors.New("inventario: un servicio tiene que decir qué consumió; " +
		"si no consumió nada de bodega, no hace falta registrarlo acá")
	// ErrProveedorConsignadaRequerido indica una unidad consignada sin decir de quién es. Sin el
	// proveedor, la cuenta por pagar que nace al usarla no tendría a quién pagarle.
	ErrProveedorConsignadaRequerido = errors.New("inventario: una unidad consignada tiene que decir " +
		"de qué proveedor es: es a quien hay que pagarle cuando se use")
	// ErrEstadoUnidadInvalido indica un estado de unidad que no existe.
	ErrEstadoUnidadInvalido = errors.New("inventario: ese no es un estado de unidad")
)

// ConsignacionSoloPorUnidadError indica que se quiso marcar como consignada una entrada de un
// artículo que se lleva por cantidad. No es un capricho: los artículos por cantidad no crean fichas
// de unidad, así que no hay dónde guardar de quién es cada objeto ni qué se le debe al proveedor.
// Antes el dato se aceptaba y se descartaba en silencio, que es peor: el usuario creía que había
// quedado marcado.
type ConsignacionSoloPorUnidadError struct {
	Articulo string
}

func (e *ConsignacionSoloPorUnidadError) Error() string {
	return "«" + e.Articulo + "» se lleva por cantidad, y la consignación se lleva por unidad: " +
		"para controlar de quién es cada objeto el artículo tiene que ser por unidad"
}

// SinExistenciaError indica que se quiso sacar más de lo que hay. Lleva los números para que el
// mensaje diga cuánto hay y cuánto se pidió: «no hay existencia» a secas obliga a ir a buscarlo.
type SinExistenciaError struct {
	Articulo string
	Sede     string
	Hay      int
	Pedido   int
}

func (e *SinExistenciaError) Error() string {
	return "en " + e.Sede + " hay " + itoa(e.Hay) + " de «" + e.Articulo + "» y se están sacando " +
		itoa(e.Pedido)
}

// UnidadNoDisponibleError indica que la unidad existe pero su estado no permite la operación.
type UnidadNoDisponibleError struct {
	Numero string
	Estado string
	Quiere string
}

func (e *UnidadNoDisponibleError) Error() string {
	return "la unidad " + e.Numero + " está " + EtiquetaEstado(e.Estado) + ", así que no se puede " + e.Quiere
}

// UnidadEnOtraSedeError indica que la unidad existe y está disponible, pero en otra plaza.
//
// Tiene su propio tipo porque el problema no es el estado sino la ubicación, y porque salía como
// «error interno»: una operación rechazada por una regla del negocio no puede verse igual que una
// caída del servidor. Dice DÓNDE está, que es lo que hace falta para resolverlo (trasladarla).
type UnidadEnOtraSedeError struct {
	Numero string
	Esta   string
	Pedida string
}

func (e *UnidadEnOtraSedeError) Error() string {
	donde := e.Esta
	if donde == "" {
		donde = "en tránsito entre sedes"
	} else {
		donde = "en " + donde
	}
	return "la unidad " + e.Numero + " está " + donde + ", no en " + e.Pedida
}

// EtiquetaEstado traduce el estado de una unidad a la palabra que usa la gente. Vive en el backend
// para que el mensaje de error y la pantalla digan lo mismo.
func EtiquetaEstado(estado string) string {
	switch estado {
	case EstadoDisponible:
		return "disponible"
	case EstadoReservada:
		return "reservada"
	case EstadoEnTransito:
		return "en tránsito"
	case EstadoExhibicion:
		return "en exhibición"
	case EstadoUsada:
		return "usada"
	case EstadoDanada:
		return "dada de baja por daño"
	case EstadoDevuelta:
		return "devuelta al proveedor"
	case EstadoNoAparecio:
		return "no apareció en el conteo"
	default:
		return strings.ToLower(estado)
	}
}

// modoValido valida el modo de control antes de que llegue a la base.
func modoValido(m string) bool { return m == ModoUnidad || m == ModoCantidad }

// reFecha valida AAAA-MM-DD en el borde, para que una fecha mal escrita no salga como error interno.
var reFecha = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// itoa evita importar strconv en los mensajes de error.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// ahoraCR es el día de operación en hora de Costa Rica (UTC−6).
//
// No se usa time.Now() a secas porque en el contenedor es UTC: entre las 18:00 y la medianoche de
// Costa Rica, un movimiento registrado hoy quedaría fechado mañana. Mismo criterio de día que ya
// usan Bancos, CxP y CxC.
func ahoraCR() time.Time { return time.Now().UTC().Add(-6 * time.Hour) }

// parseDia lee una fecha AAAA-MM-DD.
func parseDia(s string) (time.Time, error) { return time.Parse("2006-01-02", s) }

// deref devuelve el string de un puntero, o vacío si es nil. La sede de una unidad es NULL mientras
// va en tránsito, y ese caso hay que poder nombrarlo sin romper.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
