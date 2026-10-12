package cxp

import (
	"fmt"
	"sort"
	"strings"
)

// LoteVacioError frena la creación de un lote al que no entró NI UNA factura.
//
// Es un error TIPADO y no un centinela porque la mitad útil es el detalle: cuál factura y por qué.
// «No se pudo crear el lote» a secas deja al usuario mirando la misma pantalla sin saber si el
// problema es el estado, un bloqueo o que ya las cortó ayer.
type LoteVacioError struct {
	Fuera []DocumentoFueraDelLote
}

func (e *LoteVacioError) Error() string {
	if len(e.Fuera) == 1 {
		return fmt.Sprintf("no se creó el lote: la única factura seleccionada no se puede cortar (%s)", e.Fuera[0].Motivo)
	}
	return fmt.Sprintf("no se creó el lote: ninguna de las %d facturas seleccionadas se puede cortar. Motivos: %s",
		len(e.Fuera), resumirMotivos(e.Fuera))
}

// topeMotivos corta la enumeración: son cinco CLASES de motivo, pero dos de ellas llevan un dato
// adentro («está en REVISADO», «ya está en el lote #14»), así que un corte de decenas de facturas
// puede producir decenas de motivos distintos. El detalle completo viaja igual en `fuera`.
const topeMotivos = 4

// resumirMotivos agrupa las facturas por razón: «está en RECIBIDO: … (2); está bloqueada… (1)».
//
// Agrupa en vez de enumerar porque el caso real es un corte de decenas de facturas con dos o tres
// razones: noventa líneas iguales no se leen, y lo accionable es la razón, no cuál factura la tiene
// (para eso está la lista `fuera`, que la pantalla muestra completa).
//
// El conteo va al final entre paréntesis y no adelante a propósito: los motivos vienen redactados
// en singular desde la consulta («está en…», «ya está en…», «no existe…») y anteponerles un número
// obligaría a conjugarlos en plural, que en español es una tabla de irregulares esperando fallar.
func resumirMotivos(fuera []DocumentoFueraDelLote) string {
	orden := make([]string, 0, len(fuera))
	cuenta := make(map[string]int, len(fuera))
	for _, f := range fuera {
		if _, visto := cuenta[f.Motivo]; !visto {
			orden = append(orden, f.Motivo)
		}
		cuenta[f.Motivo]++
	}
	// El motivo más frecuente primero: es el que explica el corte. Orden estable en el empate, así
	// el mismo error da siempre el mismo texto (un mensaje que baila es un mensaje que no se cita).
	sort.SliceStable(orden, func(i, j int) bool { return cuenta[orden[i]] > cuenta[orden[j]] })

	partes := make([]string, 0, len(orden))
	for _, m := range orden[:min(len(orden), topeMotivos)] {
		partes = append(partes, fmt.Sprintf("%s (%d)", m, cuenta[m]))
	}
	texto := strings.Join(partes, "; ")
	if sobran := len(orden) - topeMotivos; sobran > 0 {
		texto += fmt.Sprintf("; y %d motivo(s) más", sobran)
	}
	return texto
}

// LotePago es un lote/corte de pago: agrupa facturas PROGRAMADAS a pagar en el banco.
// `numero` es consecutivo por empresa (el "ID de lo que se va a pagar").
type LotePago struct {
	ID         string `json:"id"`
	Numero     int64  `json:"numero"`
	FechaCorte string `json:"fecha_corte"`
	Estado     string `json:"estado"`
	Cantidad   int    `json:"cantidad"`
	TotalCRC   string `json:"total_crc"`
	CreadoEn   string `json:"creado_en"`
	// ModoPago dice cuántas transferencias salieron: FACTURA (una por factura) o PROVEEDOR (una
	// por proveedor, cubriendo varias). Se guarda y no se infiere de si hay remesas: un corte por
	// proveedor donde cada uno trajo una sola factura no agrupa nada y aun así se armó así.
	ModoPago string `json:"modo_pago"`
	// Remesas son los pagos agrupados de ESTE lote (vacío en el modo por factura). Solo lo llena
	// quien lo necesita —la pantalla del lote y la macro—, no el listado.
	Remesas []Remesa `json:"remesas,omitempty"`
	// Desglose del resultado (histórico/auditoría del corte).
	Pagadas    int `json:"pagadas"`
	Rebotadas  int `json:"rebotadas"`
	Pendientes int `json:"pendientes"`
	// Fuera son las facturas que se pidieron y NO entraron a ESTE lote, con el porqué de cada una.
	// Vacío en el caso normal. Viaja en la respuesta de crear el lote —no se persiste— porque es
	// justo el momento en que hay que decirlo: si el corte salió de 9 y se pidieron 10, la décima no
	// se paga y sin esta lista nadie se entera hasta que el proveedor reclama.
	Fuera []DocumentoFueraDelLote `json:"fuera,omitempty"`
}

// DocumentoFueraDelLote es una factura que se pidió cortar y quedó afuera, con su razón.
type DocumentoFueraDelLote struct {
	DocumentoID string `json:"documento_id"`
	Consecutivo string `json:"consecutivo"`
	Proveedor   string `json:"proveedor"`
	Estado      string `json:"estado"`
	// Motivo está escrito para leerse en pantalla, no para programar contra él.
	Motivo string `json:"motivo"`
}
