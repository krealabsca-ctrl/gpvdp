package cxp

// El arqueo que faltaba: lo que declaramos pagado y el banco nunca debitó.
//
// Hallazgo 3.1 de la auditoría Bancos↔CxP, el único CRÍTICO. Medido el 20-oct-2026, con el estado
// de cuenta cargado hasta el 14 de setiembre:
//
//     68 facturas en PAGADO sin ningún débito enlazado
//     62 de ellas con la fecha de pago YA cubierta por el estado de cuenta → ₡2.757.710,84
//     0 movimientos enlazados a una factura · 0 enlazados a una remesa (de 31.195)
//
// ── POR QUÉ ES LA PRIMERA CIFRA DE UN ARQUEO ────────────────────────────────────────────────
//
// La diferencia entre «lo que el sistema dice que pagamos» y «lo que el banco debitó» ES la
// partida conciliatoria de tesorería. Hoy el sistema no la calcula, no la muestra y no la alerta,
// y las dos lecturas posibles son incompatibles entre sí:
//
//   · El pago NUNCA se ejecutó —la macro no se subió, o el banco la rechazó en silencio—: hay 62
//     facturas marcadas como pagadas que se siguen debiendo, y el proveedor lo sabe aunque
//     nosotros no.
//   · El pago SÍ se ejecutó y lo que falló fue el reconocimiento: hay un defecto activo en el
//     barrido de huellas.
//
// Nadie puede distinguirlas sin este reporte, y por eso la severidad es crítica: el modo de falla
// es silencioso y acumulativo.
//
// ── LAS DOS DECISIONES DE DISEÑO ────────────────────────────────────────────────────────────
//
// 1 · SOLO LO QUE EL BANCO YA PUDO HABER DEBITADO. El corte es `UltimaFechaBanco`: la fecha más
//     alta de los movimientos incluidos. Una factura programada para el 30 de setiembre cuando el
//     estado de cuenta llega al 14 no es una excepción, es un estado de cuenta que falta cargar.
//     Mezclarlas convertiría el reporte en una lista que nadie puede terminar de revisar — que es
//     la forma segura de que deje de mirarse.
//
// 2 · PROPONE, NO DECIDE. Cada factura viaja con sus movimientos CANDIDATOS: débitos sin enlazar,
//     del mismo monto, cerca de la fecha. El sistema no elige: enlazar un pago contra el
//     movimiento equivocado deja dos errores donde había uno, y ninguno se vuelve a mirar porque
//     ambos quedan «conciliados». Es la misma regla del detector de duplicados de Bancos.

// VentanaCandidatosDias es cuántos días alrededor de la fecha de pago se buscan los candidatos.
//
// Nueve días a cada lado, y el número tiene motivo: una transferencia programada un viernes se
// debita el lunes, un feriado la corre dos días más, y las macros se suben con el corte de la
// semana. Una ventana más corta esconde el candidato correcto; una mucho más larga trae tantos
// débitos del mismo monto que la lista deja de ser una propuesta.
const VentanaCandidatosDias = 9

// MovimientoCandidato es un débito del banco que PODRÍA ser el pago de una factura.
//
// «Podría» es literal: la coincidencia es por monto y cercanía de fecha, no por huella. Si tuviera
// huella el barrido ya lo habría enlazado solo, y esta factura no estaría en el reporte.
type MovimientoCandidato struct {
	MovimientoID string `json:"movimiento_id"`
	Fecha        string `json:"fecha"`
	Descripcion  string `json:"descripcion"`
	MontoCRC     string `json:"monto_crc"`
	Cuenta       string `json:"cuenta"`
	// DiasDeDiferencia entre el débito y la fecha de pago programada. Negativo = el banco debitó
	// antes. Se muestra porque es lo que primero descarta un candidato a ojo.
	DiasDeDiferencia int `json:"dias_de_diferencia"`
	// Exacto: el débito calza ENTERO con el neto. Los que no, son el caso del pago parcial: el
	// banco debitó menos y la factura sigue debiendo el resto. Es un sí o un no, nunca un
	// «parecido» — la conciliación no afloja el monto, solo se le da camino a lo que no calza.
	Exacto bool `json:"exacto"`
}

// FacturaSinRespaldo es una factura declarada pagada que el banco nunca debitó.
type FacturaSinRespaldo struct {
	DocumentoID string `json:"documento_id"`
	Clave       string `json:"clave"`
	Consecutivo string `json:"consecutivo"`
	Proveedor   string `json:"proveedor"`
	// Huella es el código que debería venir en la descripción del débito. Se muestra para poder
	// buscarlo a mano en el estado de cuenta: si aparece ahí y el barrido no lo tomó, el problema
	// es el reconocimiento y no el pago.
	Huella    string `json:"huella"`
	Estado    string `json:"estado"`
	Moneda    string `json:"moneda"`
	NetoCRC   string `json:"neto_crc"`
	FechaPago string `json:"fecha_pago"`
	// DiasSinRespaldo desde la fecha de pago hasta el corte del estado de cuenta. Es la medida de
	// cuánto hace que esto debería haberse visto.
	DiasSinRespaldo int    `json:"dias_sin_respaldo"`
	LoteID          string `json:"lote_id,omitempty"`
	LoteNumero      int64  `json:"lote_numero,omitempty"`
	// PagadaPorRemesa dice que el pago salió agrupado. Cambia qué código buscar en el banco: el de
	// la remesa, no el de la factura — el banco del proveedor nunca va a traer el de la factura.
	PagadaPorRemesa  bool                  `json:"pagada_por_remesa"`
	ReferenciaRemesa string                `json:"referencia_remesa,omitempty"`
	Candidatos       []MovimientoCandidato `json:"candidatos"`
}

// DebitoSinFactura es la otra columna del arqueo: plata que salió del banco y no está aplicada a
// ninguna factura ni remesa.
//
// Se limita a los débitos del período que se está mirando y que NO están clasificados como algo
// que legítimamente no pasa por CxP (planilla, traslados). Sin ese filtro la columna trae miles de
// movimientos y deja de significar algo.
type DebitoSinFactura struct {
	MovimientoID  string `json:"movimiento_id"`
	Fecha         string `json:"fecha"`
	Descripcion   string `json:"descripcion"`
	MontoCRC      string `json:"monto_crc"`
	Cuenta        string `json:"cuenta"`
	Clasificacion string `json:"clasificacion,omitempty"`
}

// ReporteExcepcion es el arqueo de dos columnas.
type ReporteExcepcion struct {
	// CorteBanco es hasta qué fecha llega el estado de cuenta cargado. Todo el reporte se lee
	// contra esta fecha, así que va primero y siempre: sin ella los números no significan nada.
	CorteBanco string `json:"corte_banco"`
	// Desde/Hasta es el rango consultado para la columna de débitos.
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`

	SinRespaldo       []FacturaSinRespaldo `json:"sin_respaldo"`
	TotalSinRespaldo  string               `json:"total_sin_respaldo"`
	DebitosSinFactura []DebitoSinFactura   `json:"debitos_sin_factura"`
	TotalDebitos      string               `json:"total_debitos"`
	// DebitosEncontrados es cuántos débitos sin factura hay DE VERDAD en el rango; la lista trae
	// como mucho 500. Viaja siempre y la pantalla lo dice: sobre los datos reales son 2.388 en
	// noventa días, y mostrar 500 sin avisar se lee como «esto es todo», que es la peor forma de
	// recortar. El total de plata de arriba corresponde a lo MOSTRADO, no al universo.
	DebitosEncontrados int `json:"debitos_encontrados"`

	// AunNoVencidas son las facturas declaradas pagadas cuya fecha TODAVÍA no está cubierta por el
	// estado de cuenta. No son excepciones; se cuentan aparte para que nadie lea «0 excepciones»
	// como «todo conciliado» cuando lo que pasa es que falta cargar el banco.
	AunNoVencidas int `json:"aun_no_vencidas"`
}

// RespaldoDelLote es el mismo cálculo, acotado a un lote: lo que se pregunta al cerrarlo.
type RespaldoDelLote struct {
	LoteID      string               `json:"lote_id"`
	LoteNumero  int64                `json:"lote_numero"`
	CorteBanco  string               `json:"corte_banco"`
	Facturas    int                  `json:"facturas"`
	SinRespaldo []FacturaSinRespaldo `json:"sin_respaldo"`
}
