package cxp

// Fase de la Bandeja: en QUÉ cola de trabajo está una factura.
//
// Una sola expresión, usada por el RESUMEN (los números del encabezado) y por el LISTADO (las
// filas). Mientras las dos consultas la compartan, el número de una pestaña y lo que se abre al
// hacerle clic no pueden discrepar: es el mismo CASE. Cuando estaban escritas por separado,
// discreparon —el encabezado contaba una factura en dos pestañas— y el validador terminaba
// abriendo trabajo que no le tocaba.
//
// El orden de los WHEN importa: cada documento cae en UNA sola fase, la primera que calza.
//
//	cnt  «de Contabilidad»: no depende de ningún área para avanzar, así que sale de la cola de
//	     área y forma su propia fila. Se evalúa PRIMERO por eso.
//	rec  recibida, sin revisar.
//	val  cola del ÁREA. Es la excepción, no el default: solo cae acá lo que disparó un criterio
//	     de riesgo al revisarse (monto, proveedor esporádico, desvío contra su histórico).
//	apr  lista para firma: o el área ya dio conformidad (VALIDADO_DEPTO), o la factura nunca
//	     necesitó pasar por el área. Las dos llegan al mismo lugar.
//
// `requiere_validacion` NULL (documento anterior a la regla, o que no se pudo evaluar) cuenta
// como que SÍ requiere validación: lo conservador es que alguien la mire, no que se pague sin
// que nadie la haya decidido.
const faseBandejaSQL = `CASE
		WHEN d.estado IN ('RECIBIDO', 'REVISADO', 'VALIDADO_DEPTO')
		     AND (` + contabilidadOrigenSQL + `) <> '' THEN 'cnt'
		WHEN d.estado = 'RECIBIDO' THEN 'rec'
		WHEN d.estado = 'REVISADO' AND COALESCE(d.requiere_validacion, true) THEN 'val'
		WHEN d.estado = 'VALIDADO_DEPTO' OR d.estado = 'REVISADO' THEN 'apr'
		WHEN d.estado = 'APROBADO' OR (d.estado = 'PROGRAMADO' AND d.lote_id IS NULL) THEN 'pag'
		WHEN (d.estado = 'PROGRAMADO' AND d.lote_id IS NOT NULL) OR d.estado = 'REBOTADA' THEN 'bco'
		WHEN d.estado IN ('PAGADO', 'CONCILIADO') THEN 'pgd'
		ELSE 'arc' END`

// fasesBandeja son las claves válidas del filtro `fase` (mismas que produce faseBandejaSQL).
var fasesBandeja = map[string]bool{
	"cnt": true, "rec": true, "val": true, "apr": true,
	"pag": true, "bco": true, "pgd": true, "arc": true,
}
