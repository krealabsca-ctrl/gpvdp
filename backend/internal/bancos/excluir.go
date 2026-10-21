package bancos

// Excluir un movimiento suelto y detectar los duplicados que conviene excluir (mig 0093).

import (
	"errors"
	"strings"
)

var (
	// ErrMovimientoNoExcluible: no existe en esta empresa, o ya estaba fuera de los libros. Las dos
	// cosas significan lo mismo para quien lo pidió —no había nada que sacar— y distinguirlas le
	// diría a quien prueba ids ajenos cuáles existen.
	ErrMovimientoNoExcluible = errors.New("bancos: ese movimiento no existe o ya está fuera de los libros")
	// ErrMovimientoNoIncluible: no está excluido a mano. Lo que salió con la reversa de su carga
	// vuelve deshaciendo esa reversa, no desde acá: devolverlo suelto dejaría una fila en los
	// libros que ningún estado de cuenta respalda.
	ErrMovimientoNoIncluible = errors.New("bancos: ese movimiento no está excluido a mano; si salió al revertir una carga, se devuelve deshaciendo esa reversa")
	// ErrMotivoExclusionRequerido: sacar plata de los libros sin decir por qué deja un faltante que
	// nadie puede distinguir de un error del sistema.
	ErrMotivoExclusionRequerido = errors.New("bancos: decí por qué se saca este movimiento de los libros")
	// ErrExclusionConCxP: el movimiento es el pago de una factura o de una remesa.
	//
	// Sacarlo de los libros deja a la factura CONCILIADA contra plata que ya no está, y el barrido
	// de huellas no puede volver a mirarla: filtra por `incluido` Y por `documento_cxp_id IS NULL`,
	// así que el enlace queda huérfano para siempre y no hay forma de repararlo desde la aplicación.
	//
	// La reversa de la carga entera ya bloquea por esto (ver BloqueosReversa.FacturasCxP) y dividir
	// también (ErrDivisionConCxP). Excluir suelto es el tercer camino hacia `incluido = false` y
	// durante un tiempo fue el único sin la guarda.
	ErrExclusionConCxP = errors.New("bancos: ese movimiento es el pago de una factura de CxP; desenlazalo antes de sacarlo de los libros")
	// ErrRangoDuplicadosRequerido: el detector cruza la tabla contra sí misma y necesita un rango.
	ErrRangoDuplicadosRequerido = errors.New("bancos: decí desde y hasta qué fecha buscar duplicados")
)

// MovimientoExcluido es un movimiento fuera de los libros, con su rastro.
type MovimientoExcluido struct {
	ID          string `json:"id"`
	Fecha       string `json:"fecha"`
	Documento   string `json:"documento"`
	Debito      string `json:"debito"`
	Credito     string `json:"credito"`
	Descripcion string `json:"descripcion"`
	Cuenta      string `json:"cuenta"`
	Banco       string `json:"banco"`
	Motivo      string `json:"motivo"`
	ExcluidoPor string `json:"excluido_por"`
	ExcluidoEn  string `json:"excluido_en"`
	// Incluido dice si HOY está en los libros. Lo usa la confirmación previa, no la lista.
	Incluido bool `json:"incluido"`
}

// LadoDuplicado es uno de los dos movimientos de un par.
type LadoDuplicado struct {
	ID          string `json:"id"`
	Documento   string `json:"documento"`
	Descripcion string `json:"descripcion"`
}

// ParDuplicado son dos movimientos que podrían ser el mismo hecho cargado dos veces.
//
// Trae las SEÑALES y no un veredicto. El sistema no decide cuál sacar —ni si hay que sacar
// alguno—: propone y una persona mira. Está medido por qué: la regla «mismos cuatro dígitos» a
// secas devolvió 84 pares sobre los datos reales que eran comisiones legítimas repetidas el mismo
// día, y excluirlas habría sacado plata real de los libros.
type ParDuplicado struct {
	Fecha    string        `json:"fecha"`
	Monto    string        `json:"monto"`
	EsDebito bool          `json:"es_debito"`
	Cuenta   string        `json:"cuenta"`
	Banco    string        `json:"banco"`
	A        LadoDuplicado `json:"a"`
	B        LadoDuplicado `json:"b"`
	// MismosUltimos4: los últimos cuatro dígitos del consecutivo coinciden. Es la base que el
	// banco conserva cuando le cambia el formato al resto.
	MismosUltimos4 bool `json:"mismos_ultimos_4"`
	// LargoDistinto: el consecutivo tiene distinta cantidad de dígitos. Huella de un cambio de
	// formato, no de dos transacciones.
	LargoDistinto bool `json:"largo_distinto"`
	// DescripcionDistinta: dos movimientos legítimos iguales el mismo día traen la MISMA
	// descripción; un duplicado por recarga trae otra.
	DescripcionDistinta bool `json:"descripcion_distinta"`
	// Senales es cuántas de las tres se cumplen (0 a 3). Ordena la lista: lo que tiene las tres es
	// lo que hay que mirar primero.
	Senales int `json:"senales"`
}

// ContarSenales llena `Senales` y ordena el par para la pantalla.
//
// Se calcula en Go y no en SQL por una sola razón: así la prueba puede fijarlo sin base, y la
// pantalla recibe el mismo número que el que se probó.
func ContarSenales(p *ParDuplicado) {
	p.Senales = 0
	for _, s := range []bool{p.MismosUltimos4, p.LargoDistinto, p.DescripcionDistinta} {
		if s {
			p.Senales++
		}
	}
}

// MotivoExclusionValido exige un motivo con sustancia.
//
// Tres caracteres no son un motivo, y «x» tampoco. El que audite el mes tiene que poder leer por
// qué falta esa plata; si el campo admite cualquier cosa, termina lleno de puntos.
func MotivoExclusionValido(m string) bool {
	return len([]rune(strings.TrimSpace(m))) >= 5
}
