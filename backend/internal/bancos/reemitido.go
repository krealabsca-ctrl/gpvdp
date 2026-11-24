package bancos

// El banco REEMITE un movimiento: lo manda otra vez, otro día, con otra descripción y el mismo
// consecutivo (mig 0118). El porqué del diseño está en esa migración.
//
// No lo agarra el detector de duplicados —ese busca el MISMO día con consecutivos DISTINTOS, que
// es exactamente lo contrario— y no se puede detectar solo: la huella «mismo consecutivo, mismo
// monto, misma cuenta, fechas distintas» da sobre todo pagos RECURRENTES legítimos. Acá el sistema
// propone y una persona decide, igual que con los duplicados.

import (
	"errors"
	"strings"
)

var (
	// ErrReemisionMismoMovimiento: se pidió enlazar un movimiento consigo mismo.
	ErrReemisionMismoMovimiento = errors.New("bancos: un movimiento no se puede reemplazar a sí mismo")
	// ErrReemisionNoEncontrada: alguno de los dos no existe en esta empresa.
	ErrReemisionNoEncontrada = errors.New("bancos: alguno de esos dos movimientos no existe en esta empresa")
	// ErrReemisionDistintaCuenta: el banco no reemite de una cuenta a otra.
	//
	// Si fueran cuentas distintas no es una reemisión: es un TRASLADO, y para eso está el
	// emparejamiento de traslados, que conserva las dos patas en los libros. Enlazarlos acá sacaría
	// una de las dos y el traslado dejaría de cuadrar.
	ErrReemisionDistintaCuenta = errors.New("bancos: esos dos movimientos son de cuentas distintas; si es un traslado se empareja como traslado, no como reemisión")
	// ErrReemisionDistintoMonto: montos o lados distintos.
	//
	// El mismo hecho económico no cambia de monto al reemitirse. Si el banco mandó otra cifra, no es
	// la misma transacción y enlazarlas haría desaparecer la diferencia de los libros sin rastro.
	ErrReemisionDistintoMonto = errors.New("bancos: esos dos movimientos no tienen el mismo monto; una reemisión del banco conserva la cifra")
	// ErrReemisionYaEnlazada: alguno de los dos ya participa de otro enlace.
	ErrReemisionYaEnlazada = errors.New("bancos: alguno de esos dos movimientos ya está enlazado a otro; deshacé ese enlace primero")
	// ErrReemisionFueraDeLibros: alguno ya no cuenta.
	//
	// Enlazar algo que ya está fuera de los libros no cambia ningún número y deja un enlace que
	// miente sobre lo que pasó: diría que se sacó por reemisión cuando salió por otra cosa.
	ErrReemisionFueraDeLibros = errors.New("bancos: los dos movimientos tienen que estar en los libros para enlazarlos")
	// ErrMotivoReemisionRequerido: el motivo es obligatorio y la base exige diez caracteres.
	ErrMotivoReemisionRequerido = errors.New("bancos: decí por qué estos dos movimientos son el mismo hecho (al menos 10 caracteres)")
	// ErrReemisionPeriodoCerrado: el movimiento que saldría está en un mes ya cerrado.
	//
	// Cerrar un período es declarar que esos números son definitivos. Sacar plata de un mes cerrado
	// cambia un saldo que ya se reportó, y quien lea el cierre de ese mes va a ver otra cifra que la
	// que firmó.
	ErrReemisionPeriodoCerrado = errors.New("bancos: el movimiento que saldría de los libros está en un período ya cerrado")
	// ErrReemisionConDependencias: algo cuelga del movimiento que saldría.
	//
	// El mensaje lo arma el servicio nombrando QUÉ cuelga: el arreglo es distinto según sea un cobro
	// de CxC, el pago de una factura o una pata de traslado, y «tiene dependencias» a secas manda a
	// buscar el problema a ciegas.
	ErrReemisionConDependencias = errors.New("bancos: hay registros colgando del movimiento que saldría de los libros")
	// ErrReemisionNoExiste: se pidió deshacer un enlace que no está.
	ErrReemisionNoExiste = errors.New("bancos: ese movimiento no reemplaza a ninguno")
)

// largoMinimoMotivoReemision es el mismo tope que el CHECK de la mig 0118. Se comprueba acá para
// devolver un error explicado en vez del 500 de una violación de restricción.
const largoMinimoMotivoReemision = 10

// MotivoReemisionValido exige un motivo con sustancia, por la misma razón que el de excluir: quien
// audite el mes tiene que poder leer por qué esa plata no está.
func MotivoReemisionValido(m string) bool {
	return len([]rune(strings.TrimSpace(m))) >= largoMinimoMotivoReemision
}

// LadoReemision es uno de los dos movimientos del par, con lo que hace falta para decidir.
//
// Lleva la clasificación porque es el dato que define cuál conviene dejar vigente: si uno ya está
// clasificado y revisado y el otro no, enlazarlos al revés obliga a reclasificar a mano.
type LadoReemision struct {
	ID          string `json:"id"`
	Fecha       string `json:"fecha"`
	Documento   string `json:"documento"`
	Descripcion string `json:"descripcion"`
	// Clasificacion es «Concepto › Clasificación» ya armado, o vacío si no está clasificado.
	Clasificacion string `json:"clasificacion"`
	Estado        string `json:"estado_clasificacion"`
	Incluido      bool   `json:"incluido"`
}

// CandidatoReemision es un par que PODRÍA ser el mismo hecho reemitido.
//
// `DiasEntre` es la señal que más separa el grano de la paja y por eso viaja a la pantalla: las
// recurrencias legítimas caen a 7, 14, 28 o 30 días; una reemisión del banco cae al día siguiente.
// El sistema no decide con ese número, lo muestra.
type CandidatoReemision struct {
	Monto     string        `json:"monto"`
	EsDebito  bool          `json:"es_debito"`
	Cuenta    string        `json:"cuenta"`
	Banco     string        `json:"banco"`
	DiasEntre int           `json:"dias_entre"`
	Anterior  LadoReemision `json:"anterior"`
	Posterior LadoReemision `json:"posterior"`
}

// Reemision es un enlace ya hecho: el que cuenta y el que salió.
type Reemision struct {
	Monto    string `json:"monto"`
	EsDebito bool   `json:"es_debito"`
	Cuenta   string `json:"cuenta"`
	Banco    string `json:"banco"`
	// Vigente es el que cuenta para los libros.
	Vigente LadoReemision `json:"vigente"`
	// Reemplazado salió de los libros: sigue existiendo con su fecha y su descripción.
	Reemplazado LadoReemision `json:"reemplazado"`
	Motivo      string        `json:"motivo"`
	EnlazadoPor string        `json:"enlazado_por"`
	EnlazadoEn  string        `json:"enlazado_en"`
	// MismaClasificacion dice que los dos terminaron con la misma partida.
	//
	// NO DICE «la heredó». Se deriva comparando las dos filas, así que también da verdadero cuando
	// los dos ya venían clasificados igual —que es lo normal cuando el motor automático acertó en
	// ambos—. Llamarlo «heredada» le atribuía al sistema un trabajo que muchas veces no hizo, y eso
	// importa: quien revisa necesita saber si una clasificación la puso una persona.
	//
	// Si alguna vez hace falta el dato exacto de la herencia, está en la bitácora del enlace
	// (`ENLAZAR_REEMISION`, campo `heredo_clasificacion`), que lo registra en el momento.
	MismaClasificacion bool `json:"misma_clasificacion"`
}

// DependenciaDeMovimiento nombra algo que cuelga de un movimiento y cuántos son.
//
// Viaja a la pantalla para que el aviso diga QUÉ hay que deshacer. El usuario eligió que el
// sistema FRENE en vez de avisar y seguir: un cobro de CxC apuntando a plata que ya no cuenta es
// un descuadre que nadie ve hasta el cierre.
type DependenciaDeMovimiento struct {
	Que      string `json:"que"`
	Cantidad int    `json:"cantidad"`
}
