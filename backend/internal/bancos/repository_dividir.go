package bancos

// Dividir un movimiento entre dos partidas (mig 0094).
//
// LA INVARIANTE, y todo este archivo existe para sostenerla:
//
//	padre + hijos = lo que trajo el banco, SIEMPRE
//
// Por eso las dos escrituras —restarle al padre y crear el hijo— van en UNA transacción, y por eso
// el UPDATE del padre lleva su propia comprobación de saldo: si entre que la pantalla calculó y
// confirmó alguien dividió el mismo movimiento, la segunda operación no encuentra la fila y no
// escribe nada, en vez de dejar un padre en negativo.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// DividirMovimiento aparta `monto` del movimiento y lo pone en una parte nueva con su propia
// clasificación.
//
// Devuelve el id de la parte creada.
func (r *pgRepository) DividirMovimiento(ctx context.Context, empresaID, movID string, in DivisionInput, usuarioID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("bancos: dividir movimiento: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Restarle al padre, con la comprobación del saldo DENTRO del UPDATE.
	//
	// `esDebito` decide de qué columna se resta. Un movimiento tiene débito o crédito, nunca los
	// dos, y la parte apartada va del mismo lado: un recibo de luz no genera un ingreso al
	// dividirse.
	//
	// El `>` y no `>=` es deliberado: apartar TODO el monto dejaría un padre en cero, que no es
	// una división sino una reclasificación disfrazada —y para eso ya está clasificar—.
	const qPadre = `
		UPDATE movimiento_bancario m
		   SET debito  = CASE WHEN COALESCE(m.debito,0)  > 0 THEN m.debito  - $3::numeric ELSE m.debito  END,
		       credito = CASE WHEN COALESCE(m.credito,0) > 0 THEN m.credito - $3::numeric ELSE m.credito END,
		       monto_crc = COALESCE(m.monto_crc,0) - $4::numeric,
		       monto_antes_de_dividir = COALESCE(m.monto_antes_de_dividir,
		                                         COALESCE(m.debito,0) + COALESCE(m.credito,0)),
		       dividido_en = now(), dividido_por = $5::uuid,
		       actualizado_en = now()
		 WHERE m.empresa_id = $1::uuid AND m.id = $2::uuid AND m.incluido
		   AND m.dividido_de IS NULL
		   AND m.documento_cxp_id IS NULL AND m.remesa_id IS NULL
		   AND (COALESCE(m.debito,0) + COALESCE(m.credito,0)) > $3::numeric
		RETURNING m.cuenta_bancaria_id::text, m.importacion_id::text, m.fecha::text,
		          COALESCE(m.documento,''), COALESCE(m.descripcion,''),
		          COALESCE(m.debito,0) > 0, m.natural_key, m.moneda_original`
	var cuentaID, impID, fecha, documento, descripcion, naturalKey, monedaOrig string
	var esDebito bool
	err = tx.QueryRow(ctx, qPadre, empresaID, movID, in.Monto, in.MontoCRC, usuarioID).
		Scan(&cuentaID, &impID, &fecha, &documento, &descripcion, &esDebito, &naturalKey, &monedaOrig)
	if errors.Is(err, pgx.ErrNoRows) {
		// Una sola respuesta para varias causas —no existe, ya está fuera de los libros, ya es una
		// parte, está enlazado a una factura, o el monto no alcanza—, y el service ya las
		// distinguió antes con un mensaje propio. Acá llega como la carrera que no se pudo ganar.
		return "", ErrMovimientoNoDivisible
	}
	if err != nil {
		return "", fmt.Errorf("bancos: dividir movimiento: %w", err)
	}

	// 2. Crear la parte, heredando el HECHO BANCARIO.
	//
	// Fecha, consecutivo y descripción se copian tal cual: es el mismo movimiento del banco visto
	// por otra partida, y quien lo busque por su consecutivo tiene que encontrar las dos caras.
	//
	// `natural_key` NO se puede copiar: es única por empresa (`ux_mov_natural_key_vigente`) y es
	// la llave anti-duplicado. La parte lleva la del padre con un sufijo, de modo que una recarga
	// del mismo archivo sigue chocando con el padre y no entra dos veces.
	const qHijo = `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, importacion_id, fecha, documento, descripcion,
			 debito, credito, moneda_original, monto_original, monto_crc,
			 concepto_id, clasificacion_id, departamento_id, sede_id,
			 natural_key, incluido, dividido_de, dividido_en, dividido_por, dividido_motivo)
		VALUES ($1::uuid, $2::uuid, NULLIF($3,'')::uuid, $4::date, $5, $6,
		        CASE WHEN $7 THEN $8::numeric ELSE 0 END,
		        CASE WHEN $7 THEN 0 ELSE $8::numeric END,
		        $9, $8::numeric, $10::numeric,
		        NULLIF($11,'')::uuid, NULLIF($12,'')::uuid, NULLIF($13,'')::uuid, NULLIF($14,'')::uuid,
		        $15, true, $16::uuid, now(), $17::uuid, NULLIF($18,''))
		RETURNING id::text`
	var hijoID string
	err = tx.QueryRow(ctx, qHijo, empresaID, cuentaID, impID, fecha, documento, descripcion,
		esDebito, in.Monto, monedaOrig, in.MontoCRC,
		in.ConceptoID, in.ClasificacionID, in.DepartamentoID, in.SedeID,
		naturalKey+"|div|"+in.Monto.String(), movID, usuarioID, in.Motivo).Scan(&hijoID)
	if err != nil {
		return "", fmt.Errorf("bancos: crear la parte: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("bancos: dividir movimiento: %w", err)
	}
	return hijoID, nil
}

// DeshacerDivision devuelve el monto de UNA parte a su padre y saca la parte de los libros.
//
// Las dos escrituras van en la misma transacción por la invariante: devolverle al padre sin sacar
// la parte contaría la plata dos veces, y sacar la parte sin devolver dejaría al mes corto.
func (r *pgRepository) DeshacerDivision(ctx context.Context, empresaID, parteID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bancos: deshacer división: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const qParte = `
		UPDATE movimiento_bancario
		   SET incluido = false, actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid AND incluido AND dividido_de IS NOT NULL
		 RETURNING dividido_de::text, COALESCE(debito,0)::text, COALESCE(credito,0)::text,
		           COALESCE(monto_crc,0)::text`
	var padreID, debito, credito, montoCRC string
	err = tx.QueryRow(ctx, qParte, empresaID, parteID).Scan(&padreID, &debito, &credito, &montoCRC)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrParteNoEncontrada
	}
	if err != nil {
		return fmt.Errorf("bancos: deshacer división: %w", err)
	}

	// Al padre se le devuelve EXACTAMENTE lo que tenía la parte, no lo que alguien diga.
	const qPadre = `
		UPDATE movimiento_bancario
		   SET debito    = COALESCE(debito,0)  + $3::numeric,
		       credito   = COALESCE(credito,0) + $4::numeric,
		       monto_crc = COALESCE(monto_crc,0) + $5::numeric,
		       actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := tx.Exec(ctx, qPadre, empresaID, padreID, debito, credito, montoCRC)
	if err != nil {
		return fmt.Errorf("bancos: devolver el monto al movimiento: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrParteNoEncontrada
	}

	// Si al padre ya no le queda ninguna parte viva, deja de estar dividido.
	const qLimpiar = `
		UPDATE movimiento_bancario p
		   SET monto_antes_de_dividir = NULL, dividido_en = NULL, dividido_por = NULL
		 WHERE p.empresa_id = $1::uuid AND p.id = $2::uuid
		   AND NOT EXISTS (SELECT 1 FROM movimiento_bancario h
		                    WHERE h.dividido_de = p.id AND h.incluido)`
	if _, err := tx.Exec(ctx, qLimpiar, empresaID, padreID); err != nil {
		return fmt.Errorf("bancos: limpiar la marca de dividido: %w", err)
	}
	return tx.Commit(ctx)
}

// PartesDeMovimiento devuelve el movimiento y sus partes, para mostrar el reparto completo.
func (r *pgRepository) PartesDeMovimiento(ctx context.Context, empresaID, movID string) (Division, error) {
	// El id que llega puede ser el del padre o el de una parte: desde cualquiera se ve el reparto
	// entero, porque quien abre una parte quiere saber de dónde salió.
	const qRaiz = `
		SELECT COALESCE(dividido_de::text, id::text)
		FROM movimiento_bancario WHERE empresa_id = $1::uuid AND id = $2::uuid`
	var raizID string
	err := r.pool.QueryRow(ctx, qRaiz, empresaID, movID).Scan(&raizID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Division{}, ErrMovimientoNoEncontrado
	}
	if err != nil {
		return Division{}, fmt.Errorf("bancos: partes del movimiento: %w", err)
	}

	const q = `
		SELECT m.id::text, (m.dividido_de IS NULL) AS es_principal,
		       m.fecha::text, COALESCE(m.documento,''), COALESCE(m.descripcion,''),
		       (COALESCE(m.debito,0) + COALESCE(m.credito,0))::text,
		       COALESCE(m.debito,0) > 0,
		       COALESCE(con.nombre,''), COALESCE(cl.nombre,''),
		       COALESCE(m.dividido_motivo,''), m.incluido,
		       COALESCE(m.monto_antes_de_dividir,0)::text
		FROM movimiento_bancario m
		LEFT JOIN concepto con ON con.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		WHERE m.empresa_id = $1::uuid AND (m.id = $2::uuid OR m.dividido_de = $2::uuid)
		  AND m.incluido
		ORDER BY (m.dividido_de IS NOT NULL), m.id`
	rows, err := r.pool.Query(ctx, q, empresaID, raizID)
	if err != nil {
		return Division{}, fmt.Errorf("bancos: partes del movimiento: %w", err)
	}
	defer rows.Close()

	var d Division
	total := decimal.Zero
	for rows.Next() {
		var p ParteMovimiento
		var antes string
		if err := rows.Scan(&p.ID, &p.EsPrincipal, &d.Fecha, &d.Documento, &d.Descripcion,
			&p.Monto, &p.EsDebito, &p.Concepto, &p.Clasificacion, &p.Motivo, &p.Incluido,
			&antes); err != nil {
			return Division{}, fmt.Errorf("bancos: partes del movimiento: %w", err)
		}
		if p.EsPrincipal {
			d.MontoAntesDeDividir = antes
		}
		total = total.Add(decimal.RequireFromString(p.Monto))
		d.Partes = append(d.Partes, p)
	}
	if err := rows.Err(); err != nil {
		return Division{}, fmt.Errorf("bancos: partes del movimiento: %w", err)
	}
	if len(d.Partes) == 0 {
		return Division{}, ErrMovimientoNoEncontrado
	}
	d.MovimientoID = raizID
	d.SumaDeLasPartes = total.StringFixed(2)
	return d, nil
}

// MovimientoParaDividir trae lo que hace falta para decidir si se puede dividir y por cuánto.
func (r *pgRepository) MovimientoParaDividir(ctx context.Context, empresaID, movID string) (MovimientoDivisible, error) {
	const q = `
		SELECT m.id::text, (COALESCE(m.debito,0) + COALESCE(m.credito,0))::text,
		       COALESCE(m.monto_crc,0)::text, m.incluido,
		       (m.dividido_de IS NOT NULL) AS es_parte,
		       (m.documento_cxp_id IS NOT NULL OR m.remesa_id IS NOT NULL) AS enlazado_a_cxp,
		       COALESCE(m.descripcion,'')
		FROM movimiento_bancario m
		WHERE m.empresa_id = $1::uuid AND m.id = $2::uuid`
	var m MovimientoDivisible
	err := r.pool.QueryRow(ctx, q, empresaID, movID).
		Scan(&m.ID, &m.Monto, &m.MontoCRC, &m.Incluido, &m.EsParte, &m.EnlazadoACxP, &m.Descripcion)
	if errors.Is(err, pgx.ErrNoRows) {
		return MovimientoDivisible{}, ErrMovimientoNoEncontrado
	}
	if err != nil {
		return MovimientoDivisible{}, fmt.Errorf("bancos: movimiento para dividir: %w", err)
	}
	return m, nil
}
