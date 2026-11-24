package cxp

// Consultas del arqueo de excepción (hallazgo 3.1). El porqué está en excepcion.go.

import (
	"context"
	"fmt"
)

// sinRespaldoBase es el corazón del reporte y se usa en los DOS lugares que preguntan lo mismo:
// el arqueo general y el respaldo de un lote al cerrarlo. Una sola definición, para que no puedan
// discrepar.
//
// EL ENLACE TIENE DOS CAMINOS Y HAY QUE MIRAR LOS DOS. Un pago individual deja el movimiento
// apuntando a la factura (`documento_cxp_id`); un pago agrupado apunta a la REMESA
// (`remesa_id`), y la factura cuelga de esa remesa. Mirar solo el primero marcaría como «sin
// respaldo» a toda factura pagada en grupo, que es precisamente la forma en que se paga a los
// proveedores con varias facturas.
//
// `m.incluido` en las dos: un movimiento revertido o excluido no respalda nada. Es la misma regla
// que el resto del módulo —el `AND incluido` va en el FILTER, nunca solo en el WHERE del padre—.
//
// Lleva pegado JoinsNetoSQL aunque el conteo de `ContarAunNoVencidas` no use el neto: partir la
// constante en dos para ahorrarle tres cruces a una consulta que devuelve un número sería volver
// a tener dos definiciones de lo mismo, que es justo lo que este comentario previene. Los tres
// cruces agrupan tablas de excepción —chicas— y no multiplican filas, así que el conteo no cambia.
const sinRespaldoBase = `
	FROM documento_cxp d
	LEFT JOIN proveedor p ON p.id = d.proveedor_id
	LEFT JOIN lote_pago l ON l.id = d.lote_id
	LEFT JOIN remesa_pago r ON r.id = d.remesa_id` + JoinsNetoSQL + `
	WHERE d.empresa_id = $1::uuid
	  AND d.estado = 'PAGADO'
	  AND NOT EXISTS (SELECT 1 FROM movimiento_bancario m
	                  WHERE m.documento_cxp_id = d.id AND m.incluido)
	  AND (d.remesa_id IS NULL OR NOT EXISTS (SELECT 1 FROM movimiento_bancario m
	                  WHERE m.remesa_id = d.remesa_id AND m.incluido))`

// El monto contra el que se compara es `netoCRCSQL`, el que ya define el tablero: total en
// colones − retención (convertida) − anticipos aplicados. Es la misma fórmula del archivo de pago.
//
// ESTA CONSTANTE SE ESCRIBIÓ DOS VECES Y LAS DOS VERSIONES NO DECÍAN LO MISMO. La primera versión
// de este arqueo copió la fórmula del LISTADO de lotes, que no resta la retención —el listado
// muestra un total, no lo que sale del banco—. Con eso, una factura con retención nunca habría
// encontrado su candidato: se buscaba un débito por el total y el banco debitó el total menos la
// retención. Hoy es una sola factura y ₡13, pero el defecto no depende del monto.
//
// Se reusa la definición existente en vez de arreglar la copia: dos expresiones del mismo neto
// divergen en cuanto alguien toca una, que es exactamente lo que acababa de pasar.

// FacturasSinRespaldo lista lo declarado pagado que el banco no debitó, hasta el corte indicado.
//
// `soloLote` acota a un lote (vacío = todas). `hastaFecha` es el corte del estado de cuenta: solo
// entran las facturas que el banco YA pudo haber debitado.
func (r *pgRepository) FacturasSinRespaldo(ctx context.Context, empresaID, hastaFecha, soloLote string) ([]FacturaSinRespaldo, error) {
	q := `
		SELECT d.id::text, COALESCE(d.clave,''), COALESCE(d.consecutivo,''), COALESCE(p.nombre,''),
		       COALESCE(d.huella,''), d.estado, COALESCE(d.moneda,'CRC'),
		       ` + netoCRCSQL + `::text,
		       COALESCE(to_char(d.fecha_pago_programada,'YYYY-MM-DD'),''),
		       COALESCE(($2::date - d.fecha_pago_programada), 0)::int,
		       COALESCE(l.id::text,''), COALESCE(l.numero, 0),
		       (d.remesa_id IS NOT NULL), COALESCE(r.huella,'')
		` + sinRespaldoBase + `
		  AND d.fecha_pago_programada <= $2::date
		  AND ($3 = '' OR d.lote_id = NULLIF($3,'')::uuid)
		ORDER BY d.fecha_pago_programada, p.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, hastaFecha, soloLote)
	if err != nil {
		return nil, fmt.Errorf("cxp: facturas sin respaldo: %w", err)
	}
	defer rows.Close()
	out := make([]FacturaSinRespaldo, 0)
	for rows.Next() {
		var f FacturaSinRespaldo
		if err := rows.Scan(&f.DocumentoID, &f.Clave, &f.Consecutivo, &f.Proveedor, &f.Huella,
			&f.Estado, &f.Moneda, &f.NetoCRC, &f.FechaPago, &f.DiasSinRespaldo,
			&f.LoteID, &f.LoteNumero, &f.PagadaPorRemesa, &f.ReferenciaRemesa); err != nil {
			return nil, fmt.Errorf("cxp: scan sin respaldo: %w", err)
		}
		f.Candidatos = make([]MovimientoCandidato, 0)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ContarAunNoVencidas cuenta lo declarado pagado que el banco TODAVÍA no pudo debitar.
//
// Existe para que «0 excepciones» no se lea como «todo cuadra» cuando lo que pasa es que falta
// cargar el estado de cuenta. Es la misma lección del Dashboard: un número sin su aviso de días
// sin cargar miente sin decir nada falso.
func (r *pgRepository) ContarAunNoVencidas(ctx context.Context, empresaID, hastaFecha string) (int, error) {
	var n int
	q := `SELECT count(*)::int ` + sinRespaldoBase + `
		  AND (d.fecha_pago_programada IS NULL OR d.fecha_pago_programada > $2::date)`
	if err := r.pool.QueryRow(ctx, q, empresaID, hastaFecha).Scan(&n); err != nil {
		return 0, fmt.Errorf("cxp: contar aún no vencidas: %w", err)
	}
	return n, nil
}

// CandidatosPara busca débitos que PODRÍAN ser el pago de esa factura: sin enlazar, dentro de la
// ventana de días, y marca cuáles calzan EXACTO con el neto.
//
// EL MONTO NO SE AFLOJA, SE ETIQUETA. La conciliación sigue exigiendo coincidencia exacta —en
// colones no hay diferencial que justifique tolerancia— y por eso `Exacto` viaja como un sí o un
// no, no como un «parecido». Lo que cambió el 21-oct-2026 es que los que NO calzan también se
// devuelven, porque son justamente el caso del pago parcial (hallazgo 3.5): el banco debitó menos
// y eso ahora tiene camino.
//
// Sin esto el parcial no tenía por dónde empezar: el arqueo solo ofrecía los exactos, así que una
// factura pagada de menos se veía igual que una que nunca se pagó.
//
// LOS EXACTOS VAN PRIMERO. Un débito que calza entero es casi siempre la respuesta correcta, y
// mezclarlo con los demás obligaría a buscarlo entre ocho líneas.
func (r *pgRepository) CandidatosPara(ctx context.Context, empresaID, docID string, dias int) ([]MovimientoCandidato, error) {
	const q = `
		SELECT m.id::text, to_char(m.fecha,'YYYY-MM-DD'), COALESCE(m.descripcion,''),
		       m.monto_crc::text, COALESCE(cb.alias,''),
		       (m.fecha - d.fecha_pago_programada)::int,
		       (m.monto_crc = ` + netoCRCSQL + `) AS exacto
		FROM documento_cxp d` + JoinsNetoSQL + `
		JOIN movimiento_bancario m ON m.empresa_id = d.empresa_id
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid
		  AND m.incluido
		  AND m.debito > 0
		  AND m.documento_cxp_id IS NULL AND m.remesa_id IS NULL
		  AND m.fecha BETWEEN d.fecha_pago_programada - $3::int AND d.fecha_pago_programada + $3::int
		  -- O CALZA ENTERO, O ES MENOR QUE EL NETO. Un débito MAYOR no puede ser el pago parcial
		  -- de esta factura: por definición, un parcial es menos de lo que se debe.
		  --
		  -- Sin esta línea la lista traía 465 candidatos sobre 62 facturas —ocho por cada una,
		  -- casi todos débitos ajenos que caían cerca de la fecha—. Proponer eso no es ayudar: es
		  -- la forma de que alguien empareje el equivocado, y entonces hay dos errores donde había
		  -- uno y ninguno se vuelve a mirar porque los dos quedan «conciliados».
		  AND m.monto_crc <= ` + netoCRCSQL + `
		-- EL DESEMPATE FINAL POR id NO ES ADORNO (9-nov-2026). Las tres claves de arriba empatan
		-- masivamente: hay facturas con 115 débitos del MISMO día dentro de la ventana, y sin una
		-- clave única al final, cuáles 8 de esos 115 se ofrecen lo decidía el plan de ejecución.
		-- Eso significa que la lista de candidatos cambiaba sola —al crecer la tabla, al correr un
		-- ANALYZE, al tocar un índice— sin que nadie moviera un dato. Se descubrió porque reescribir
		-- el neto cambió el plan y con él los candidatos ofrecidos, con los montos idénticos.
		--
		-- Esto hace la lista REPRODUCIBLE, no la hace buena: ofrecer 8 de 115 empatados sigue siendo
		-- poco donde el comentario de arriba advierte que emparejar el equivocado deja dos errores
		-- conciliados. Acotar de verdad esa ventana es decisión del Director Financiero.
		ORDER BY (m.monto_crc = ` + netoCRCSQL + `) DESC,
		         abs(m.fecha - d.fecha_pago_programada), m.fecha, m.id
		LIMIT 8`
	rows, err := r.pool.Query(ctx, q, empresaID, docID, dias)
	if err != nil {
		return nil, fmt.Errorf("cxp: candidatos: %w", err)
	}
	defer rows.Close()
	out := make([]MovimientoCandidato, 0)
	for rows.Next() {
		var c MovimientoCandidato
		if err := rows.Scan(&c.MovimientoID, &c.Fecha, &c.Descripcion, &c.MontoCRC, &c.Cuenta,
			&c.DiasDeDiferencia, &c.Exacto); err != nil {
			return nil, fmt.Errorf("cxp: scan candidato: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// topeDebitos es cuántos débitos sin factura se devuelven como MUCHO.
//
// Existe porque son 2.388 en noventa días sobre los datos reales: la mayoría son planilla,
// traslados y comisiones, que legítimamente nunca pasan por CxP. Mandarlos todos vuelve la
// respuesta pesada y la columna ilegible.
//
// PERO EL TOTAL SE DEVUELVE SIEMPRE, y por eso `DebitosSinFactura` devuelve también cuántos hay
// de verdad: un tope silencioso se lee como «esto es todo», que es la peor forma de recortar. La
// pantalla dice «500 de 2.388» y quien necesite el resto acota las fechas.
const topeDebitos = 500

// DebitosSinFactura es la otra columna: plata que salió y no está aplicada a nada de CxP.
// Devuelve la página y el total real de coincidencias.
func (r *pgRepository) DebitosSinFactura(ctx context.Context, empresaID, desde, hasta string) ([]DebitoSinFactura, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int FROM movimiento_bancario m
		WHERE m.empresa_id = $1::uuid AND m.incluido AND m.debito > 0
		  AND m.documento_cxp_id IS NULL AND m.remesa_id IS NULL
		  AND m.fecha BETWEEN $2::date AND $3::date`,
		empresaID, desde, hasta).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("cxp: contar débitos sin factura: %w", err)
	}
	const q = `
		SELECT m.id::text, to_char(m.fecha,'YYYY-MM-DD'), COALESCE(m.descripcion,''),
		       m.monto_crc::text, COALESCE(cb.alias,''), COALESCE(cl.nombre,'')
		FROM movimiento_bancario m
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		WHERE m.empresa_id = $1::uuid
		  AND m.incluido
		  AND m.debito > 0
		  AND m.documento_cxp_id IS NULL AND m.remesa_id IS NULL
		  AND m.fecha BETWEEN $2::date AND $3::date
		ORDER BY m.fecha DESC, m.monto_crc DESC
		LIMIT $4::int`
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta, topeDebitos)
	if err != nil {
		return nil, 0, fmt.Errorf("cxp: débitos sin factura: %w", err)
	}
	defer rows.Close()
	out := make([]DebitoSinFactura, 0)
	for rows.Next() {
		var d DebitoSinFactura
		if err := rows.Scan(&d.MovimientoID, &d.Fecha, &d.Descripcion, &d.MontoCRC, &d.Cuenta,
			&d.Clasificacion); err != nil {
			return nil, 0, fmt.Errorf("cxp: scan débito: %w", err)
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}
