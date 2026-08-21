package cxp

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
}
