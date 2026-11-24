package conta

// Consultas del generador de asientos (Fase 3B). El porqué está en generador.go.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// movimientosDelPeriodoSQL trae cada movimiento vivo del período con las DOS cuentas que necesita
// para ser un asiento, o con los campos en nulo que explican por qué no puede serlo.
//
// Los LEFT JOIN son a propósito: un INNER traería solo los contabilizables y la pantalla no
// podría decir qué falta ni cuánto. Lo que no se puede hacer importa tanto como lo que sí.
const movimientosDelPeriodoSQL = `
	SELECT m.id::text, m.fecha::text, COALESCE(m.descripcion,''), COALESCE(m.documento,''),
	       COALESCE(m.monto_crc,0)::text,
	       (m.debito > 0) AS es_debito,
	       COALESCE(cl.nombre,''),
	       mp.cuenta_id::text, mcb.cuenta_id::text,
	       COALESCE(cb.alias,''), COALESCE(mp.estado,''),
	       COALESCE(m.moneda_original,'CRC'),
	       (m.debito > 0 OR m.credito > 0) AS tiene_monto,
	       EXISTS (SELECT 1 FROM asiento a
	                WHERE a.empresa_id = m.empresa_id
	                  AND a.origen_tipo = 'MOVIMIENTO_BANCARIO'
	                  AND a.origen_id = m.id
	                  AND a.estado <> 'ANULADO') AS ya_tiene_asiento
	FROM movimiento_bancario m
	LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
	LEFT JOIN mapeo_contable mp
	       ON mp.clasificacion_id = m.clasificacion_id AND mp.empresa_id = m.empresa_id
	LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
	LEFT JOIN mapeo_cuenta_bancaria mcb
	       ON mcb.cuenta_bancaria_id = m.cuenta_bancaria_id AND mcb.empresa_id = m.empresa_id
	WHERE m.empresa_id = $1::uuid AND m.incluido
	  AND to_char(m.fecha, 'YYYY-MM') = $2
	ORDER BY m.fecha, m.id`

// MovimientosDelPeriodo lista los movimientos con su diagnóstico.
func (r *pgRepository) MovimientosDelPeriodo(ctx context.Context, empresaID, periodo string) ([]MovimientoAContabilizar, []bool, error) {
	rows, err := r.pool.Query(ctx, movimientosDelPeriodoSQL, empresaID, periodo)
	if err != nil {
		return nil, nil, fmt.Errorf("conta: leer los movimientos del período: %w", err)
	}
	defer rows.Close()

	out := make([]MovimientoAContabilizar, 0)
	yaTiene := make([]bool, 0)
	for rows.Next() {
		var m MovimientoAContabilizar
		var estadoMapeo, moneda string
		var tieneMonto, ya bool
		if err := rows.Scan(&m.MovimientoID, &m.Fecha, &m.Descripcion, &m.Documento,
			&m.MontoCRC, &m.EsDebito, &m.Partida,
			&m.CuentaPartida, &m.CuentaBanco, &m.CuentaBancaria, &estadoMapeo,
			&moneda, &tieneMonto, &ya); err != nil {
			return nil, nil, fmt.Errorf("conta: scan del movimiento: %w", err)
		}
		m.Motivo = motivoDeExclusion(m, estadoMapeo, moneda, tieneMonto)
		out = append(out, m)
		yaTiene = append(yaTiene, ya)
	}
	return out, yaTiene, rows.Err()
}

// motivoDeExclusion decide por qué un movimiento no puede ser asiento.
//
// EL ORDEN ES EL DE LA CADENA DE CAUSAS, no el de la conveniencia: sin partida no puede haber
// mapeo, y sin mapeo no importa si el banco está enlazado. Reportar la causa más profunda haría
// que alguien arregle el banco para descubrir que igual falta clasificar.
func motivoDeExclusion(m MovimientoAContabilizar, estadoMapeo, moneda string, tieneMonto bool) string {
	if !tieneMonto {
		return MontoCero
	}
	if m.Partida == "" {
		return FaltaClasificacion
	}
	if estadoMapeo == "" {
		return FaltaMapeo
	}
	if estadoMapeo == EstadoNoContabilizable {
		return NoContabilizable
	}
	if m.CuentaPartida == nil {
		// Mapeada pero sin cuenta: la base no lo permite, así que si pasa es que algo cambió
		// por debajo. Se trata como falta de mapeo en vez de romper la generación entera.
		return FaltaMapeo
	}
	if m.CuentaBanco == nil {
		return FaltaCuentaBanco
	}
	// Un movimiento en dólares sin tipo de cambio del mes queda en cero colones, y un asiento en
	// cero no es un asiento: la base lo rechazaría por la línea sin monto.
	if monto, err := decimal.NewFromString(m.MontoCRC); err != nil || !monto.IsPositive() {
		if moneda != "CRC" {
			return FaltaTipoCambio
		}
		return MontoCero
	}
	return ""
}

// ReservarConsecutivos toma un bloque de N números de una sola vez.
//
// UNO POR UNO SERÍAN N IDAS Y VUELTAS A LA BASE: un período de Valle de Paz son miles de
// movimientos, y pedir el número de cada asiento por separado convierte una generación en una
// espera. El bloque se reserva con el mismo UPDATE que serializa a los demás, así que nadie
// puede quedarse con un número del medio.
func (r *pgRepository) ReservarConsecutivos(ctx context.Context, tx pgx.Tx, empresaID string, n int) (int64, error) {
	const q = `
		INSERT INTO conta_consecutivo (empresa_id, ambito, siguiente)
		VALUES ($1::uuid, 'asiento', $2::bigint + 1)
		ON CONFLICT (empresa_id, ambito) DO UPDATE
		SET siguiente = conta_consecutivo.siguiente + $2::bigint
		RETURNING siguiente - $2::bigint`
	var primero int64
	if err := tx.QueryRow(ctx, q, empresaID, n).Scan(&primero); err != nil {
		return 0, fmt.Errorf("conta: reservar consecutivos: %w", err)
	}
	return primero, nil
}

// GenerarAsientos escribe los asientos de un período en UNA transacción.
//
// TODO O NADA: una contabilidad a medias es peor que ninguna, porque nadie sabe dónde se cortó.
// El disparador de partida doble es diferido y se evalúa al COMMIT, así que si una sola línea
// quedara descuadrada, no entra ninguna.
func (r *pgRepository) GenerarAsientos(ctx context.Context, empresaID string, movs []MovimientoAContabilizar, usuarioID string) (ResultadoGeneracion, error) {
	res := ResultadoGeneracion{}
	if len(movs) == 0 {
		return res, ErrNadaQueGenerar
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("conta: abrir transacción: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	primero, err := r.ReservarConsecutivos(ctx, tx, empresaID, len(movs))
	if err != nil {
		return res, err
	}

	const qCab = `
		INSERT INTO asiento (empresa_id, numero, fecha, glosa, origen_tipo, origen_id, estado, creado_por)
		VALUES ($1::uuid, $2, $3::date, $4, 'MOVIMIENTO_BANCARIO', $5::uuid, 'ASENTADO', NULLIF($6,'')::uuid)
		ON CONFLICT DO NOTHING
		RETURNING id::text`
	const qLin = `
		INSERT INTO asiento_linea (asiento_id, cuenta_id, debe, haber, detalle, orden)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)`

	numero := primero
	for _, m := range movs {
		monto, err := decimal.NewFromString(m.MontoCRC)
		if err != nil {
			return res, fmt.Errorf("conta: monto de %s: %w", m.MovimientoID, err)
		}
		var id string
		err = tx.QueryRow(ctx, qCab, empresaID, numero, m.Fecha,
			glosaDe(m.Partida, m.Descripcion), m.MovimientoID, usuarioID).Scan(&id)
		if err != nil {
			if err == pgx.ErrNoRows {
				// Ya existía un asiento vivo para este movimiento: el índice único lo impidió.
				// No es un error —es la idempotencia funcionando— y el número reservado se
				// salta, que es preferible a reusarlo y arriesgar un repetido.
				res.YaExistian++
				numero++
				continue
			}
			return res, traducirAsiento(err)
		}

		// LA REGLA: la dirección la da el signo del movimiento.
		//   débito  → salió plata: Debe la partida, Haber el banco
		//   crédito → entró plata: Debe el banco,  Haber la partida
		debePartida, haberPartida := monto, decimal.Zero
		debeBanco, haberBanco := decimal.Zero, monto
		if !m.EsDebito {
			debePartida, haberPartida = decimal.Zero, monto
			debeBanco, haberBanco = monto, decimal.Zero
		}
		if _, err := tx.Exec(ctx, qLin, id, *m.CuentaPartida, debePartida, haberPartida, m.Partida, 0); err != nil {
			return res, traducirAsiento(err)
		}
		if _, err := tx.Exec(ctx, qLin, id, *m.CuentaBanco, debeBanco, haberBanco, m.CuentaBancaria, 1); err != nil {
			return res, traducirAsiento(err)
		}

		if res.Creados == 0 {
			res.DesdeNumero = numero
		}
		res.HastaNumero = numero
		res.Creados++
		numero++
	}

	// Acá la base comprueba la partida doble de todos los asientos a la vez.
	if err := tx.Commit(ctx); err != nil {
		return res, traducirAsiento(err)
	}
	return res, nil
}

// RevertirGeneracion anula los asientos generados desde el banco en un período.
//
// ANULA, NO BORRA: el asiento queda en el libro, visible y sin efecto, con su motivo. Borrarlos
// dejaría un hueco en la numeración y un mayor que ya no se puede reconstruir. Y como el índice
// de idempotencia excluye los anulados, el período se puede volver a generar después.
func (r *pgRepository) RevertirGeneracion(ctx context.Context, empresaID, periodo, motivo, usuarioID string) (int, error) {
	const q = `
		UPDATE asiento SET estado = 'ANULADO', anulado_en = now(),
		       anulado_por = NULLIF($4,'')::uuid, anulado_motivo = $3
		WHERE empresa_id = $1::uuid
		  AND origen_tipo = 'MOVIMIENTO_BANCARIO'
		  AND estado <> 'ANULADO'
		  AND to_char(fecha, 'YYYY-MM') = $2`
	tag, err := r.pool.Exec(ctx, q, empresaID, periodo, motivo, usuarioID)
	if err != nil {
		// Se traduce: el disparador del cierre ya dice el mes y qué hacer, y envolverlo dejaba
		// esa frase entre «revertir la generación: ERROR: … (SQLSTATE 23514)».
		return 0, traducirAsiento(err)
	}
	return int(tag.RowsAffected()), nil
}
