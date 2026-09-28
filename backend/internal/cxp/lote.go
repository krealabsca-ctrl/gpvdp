package cxp

import "fmt"

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
	return fmt.Sprintf("no se creó el lote: ninguna de las %d facturas seleccionadas se puede cortar", len(e.Fuera))
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
