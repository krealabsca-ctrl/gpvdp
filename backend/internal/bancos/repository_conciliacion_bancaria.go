package bancos

// Consultas de la conciliación bancaria mensual. Todas filtran por empresa_id.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ActasDelMes arma, por cuenta activa, las cifras de la conciliación del mes: el saldo que
// dice el banco (el capturado el último día del mes), el saldo inicial (el último capturado
// del mes anterior), los movimientos del mes y la firma si ya existe.
//
// El «saldo de libros» no se guarda: se calcula acá con los movimientos cargados, así que si
// entra un movimiento que faltaba el acta se actualiza sola mientras no esté firmada.
func (r *pgRepository) ActasDelMes(ctx context.Context, empresaID string, anio, mes int) ([]ActaConciliacion, error) {
	const q = `
		WITH cuentas AS (
			SELECT cb.id, cb.alias, cb.moneda, b.nombre AS banco
			FROM cuenta_bancaria cb JOIN banco b ON b.id = cb.banco_id
			WHERE cb.empresa_id = $1::uuid AND cb.activo
		),
		mes AS (SELECT make_date($2, $3, 1) AS inicio,
		               (make_date($2, $3, 1) + INTERVAL '1 month' - INTERVAL '1 day')::date AS fin),
		-- Saldo del banco: el último capturado DENTRO del mes.
		cierre AS (
			SELECT DISTINCT ON (s.cuenta_bancaria_id) s.cuenta_bancaria_id, s.saldo, s.fecha
			FROM saldo_cuenta_diario s, mes
			WHERE s.empresa_id = $1::uuid AND s.fecha BETWEEN mes.inicio AND mes.fin
			ORDER BY s.cuenta_bancaria_id, s.fecha DESC
		),
		-- Saldo inicial: el último capturado ANTES del mes.
		inicial AS (
			SELECT DISTINCT ON (s.cuenta_bancaria_id) s.cuenta_bancaria_id, s.saldo, s.fecha
			FROM saldo_cuenta_diario s, mes
			WHERE s.empresa_id = $1::uuid AND s.fecha < mes.inicio
			ORDER BY s.cuenta_bancaria_id, s.fecha DESC
		),
		-- Movimientos desde el día del saldo inicial (excluido) hasta el del saldo de cierre.
		movs AS (
			SELECT c.id AS cuenta_id,
			       COALESCE(SUM(m.credito), 0) AS entradas,
			       COALESCE(SUM(m.debito), 0) AS salidas
			FROM cuentas c
			LEFT JOIN movimiento_bancario m
			       ON m.cuenta_bancaria_id = c.id AND m.empresa_id = $1::uuid AND m.incluido
			      AND m.fecha > COALESCE((SELECT i.fecha FROM inicial i WHERE i.cuenta_bancaria_id = c.id),
			                             (SELECT inicio - INTERVAL '1 day' FROM mes))
			      AND m.fecha <= COALESCE((SELECT cr.fecha FROM cierre cr WHERE cr.cuenta_bancaria_id = c.id),
			                              (SELECT fin FROM mes))
			GROUP BY c.id
		),
		ajuste AS (
			SELECT p.cuenta_bancaria_id, COALESCE(SUM(p.signo * p.monto), 0) AS total
			FROM partida_conciliacion p
			WHERE p.empresa_id = $1::uuid AND p.anio = $2 AND p.mes = $3 AND NOT p.anulada
			GROUP BY p.cuenta_bancaria_id
		)
		SELECT c.id::text, c.alias, c.banco, c.moneda,
		       COALESCE(cr.saldo::text, ''), COALESCE(to_char(cr.fecha, 'YYYY-MM-DD'), ''),
		       COALESCE(i.saldo::text, ''), COALESCE(to_char(i.fecha, 'YYYY-MM-DD'), ''),
		       mv.entradas::text, mv.salidas::text,
		       COALESCE(aj.total, 0)::text,
		       COALESCE(to_char(ac.firmado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
		       COALESCE(NULLIF(uf.nombre, ''), uf.email, '')
		FROM cuentas c
		JOIN movs mv ON mv.cuenta_id = c.id
		LEFT JOIN cierre cr ON cr.cuenta_bancaria_id = c.id
		LEFT JOIN inicial i ON i.cuenta_bancaria_id = c.id
		LEFT JOIN ajuste aj ON aj.cuenta_bancaria_id = c.id
		LEFT JOIN acta_conciliacion ac
		       ON ac.cuenta_bancaria_id = c.id AND ac.empresa_id = $1::uuid
		      AND ac.anio = $2 AND ac.mes = $3
		LEFT JOIN usuario uf ON uf.id = ac.firmado_por
		ORDER BY c.banco, c.alias`
	rows, err := r.pool.Query(ctx, q, empresaID, anio, mes)
	if err != nil {
		return nil, fmt.Errorf("bancos: actas del mes: %w", err)
	}
	defer rows.Close()
	out := make([]ActaConciliacion, 0, 16)
	for rows.Next() {
		a := ActaConciliacion{Anio: anio, Mes: mes}
		if err := rows.Scan(&a.CuentaID, &a.Alias, &a.Banco, &a.Moneda,
			&a.SaldoBanco, &a.FechaBanco, &a.SaldoInicial, &a.FechaInicial,
			&a.EntradasMes, &a.SalidasMes, &a.AjustePartidas,
			&a.FirmadoEn, &a.FirmadoPor); err != nil {
			return nil, fmt.Errorf("bancos: scan acta: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PartidasDelMes devuelve las partidas vivas del mes: de TODAS las cuentas (una sola consulta
// para las 13 cuentas) o de una sola si se pasa cuentaID.
func (r *pgRepository) PartidasDelMes(ctx context.Context, empresaID, cuentaID string, anio, mes int) ([]PartidaConciliacion, error) {
	const q = `
		SELECT p.id::text, p.cuenta_bancaria_id::text, p.tipo, p.descripcion, p.monto::text, p.signo,
		       to_char(p.registrado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       COALESCE(NULLIF(u.nombre, ''), u.email, '')
		FROM partida_conciliacion p
		LEFT JOIN usuario u ON u.id = p.registrado_por
		WHERE p.empresa_id = $1::uuid AND p.anio = $3 AND p.mes = $4 AND NOT p.anulada
		  AND ($2 = '' OR p.cuenta_bancaria_id = NULLIF($2, '')::uuid)
		ORDER BY p.cuenta_bancaria_id, p.registrado_en`
	rows, err := r.pool.Query(ctx, q, empresaID, cuentaID, anio, mes)
	if err != nil {
		return nil, fmt.Errorf("bancos: partidas del mes: %w", err)
	}
	defer rows.Close()
	out := make([]PartidaConciliacion, 0, 8)
	for rows.Next() {
		var p PartidaConciliacion
		if err := rows.Scan(&p.ID, &p.CuentaID, &p.Tipo, &p.Descripcion, &p.Monto, &p.Signo,
			&p.RegistradoEn, &p.RegistradoPor); err != nil {
			return nil, fmt.Errorf("bancos: scan partida: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CrearPartida registra una partida en tránsito. Valida la cuenta contra la empresa DENTRO del
// INSERT (tenant-safe) y contra un acta ya firmada (un documento firmado no cambia).
func (r *pgRepository) CrearPartida(ctx context.Context, empresaID string, in PartidaInput, signo int, usuarioID string) (string, error) {
	const q = `
		INSERT INTO partida_conciliacion
			(empresa_id, cuenta_bancaria_id, anio, mes, tipo, descripcion, monto, signo, registrado_por)
		SELECT $1::uuid, cb.id, $3, $4, $5, $6, $7::numeric, $8, $9::uuid
		FROM cuenta_bancaria cb
		WHERE cb.id = $2::uuid AND cb.empresa_id = $1::uuid AND cb.activo
		  AND NOT EXISTS (
			SELECT 1 FROM acta_conciliacion ac
			WHERE ac.empresa_id = $1::uuid AND ac.cuenta_bancaria_id = cb.id
			  AND ac.anio = $3 AND ac.mes = $4 AND ac.firmado_en IS NOT NULL)
		RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, in.CuentaID, in.Anio, in.Mes,
		in.Tipo, in.Descripcion, in.Monto, signo, usuarioID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// La cuenta no es de la empresa, no está activa, o el acta del mes ya se firmó.
		return "", ErrPartidaNoEncontrada
	}
	if err != nil {
		return "", fmt.Errorf("bancos: crear partida: %w", err)
	}
	return id, nil
}

// AnularPartida marca la partida como anulada (nunca se borra). Rechaza si el acta ya se firmó.
func (r *pgRepository) AnularPartida(ctx context.Context, empresaID, partidaID, usuarioID string) error {
	const q = `
		UPDATE partida_conciliacion p
		SET anulada = true, anulada_por = $3::uuid, anulada_en = now()
		WHERE p.empresa_id = $1::uuid AND p.id = $2::uuid AND NOT p.anulada
		  AND NOT EXISTS (
			SELECT 1 FROM acta_conciliacion ac
			WHERE ac.empresa_id = p.empresa_id AND ac.cuenta_bancaria_id = p.cuenta_bancaria_id
			  AND ac.anio = p.anio AND ac.mes = p.mes AND ac.firmado_en IS NOT NULL)`
	tag, err := r.pool.Exec(ctx, q, empresaID, partidaID, usuarioID)
	if err != nil {
		return fmt.Errorf("bancos: anular partida: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPartidaNoEncontrada
	}
	return nil
}

// FirmarActa congela el acta con el snapshot de sus cifras. Es idempotente por cuenta y mes.
func (r *pgRepository) FirmarActa(ctx context.Context, empresaID, cuentaID string, anio, mes int, banco, libros, ajuste, usuarioID string) error {
	const q = `
		INSERT INTO acta_conciliacion
			(empresa_id, cuenta_bancaria_id, anio, mes, saldo_banco, saldo_libros, ajuste_partidas,
			 preparado_por, firmado_por, firmado_en)
		SELECT $1::uuid, cb.id, $3, $4, $5::numeric, $6::numeric, $7::numeric, $8::uuid, $8::uuid, now()
		FROM cuenta_bancaria cb
		WHERE cb.id = $2::uuid AND cb.empresa_id = $1::uuid
		ON CONFLICT (empresa_id, cuenta_bancaria_id, anio, mes) DO UPDATE
		   SET saldo_banco = EXCLUDED.saldo_banco, saldo_libros = EXCLUDED.saldo_libros,
		       ajuste_partidas = EXCLUDED.ajuste_partidas,
		       firmado_por = EXCLUDED.firmado_por, firmado_en = now()
		   WHERE acta_conciliacion.firmado_en IS NULL`
	tag, err := r.pool.Exec(ctx, q, empresaID, cuentaID, anio, mes, banco, libros, ajuste, usuarioID)
	if err != nil {
		return fmt.Errorf("bancos: firmar acta: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// O la cuenta no es de la empresa, o el acta ya estaba firmada (idempotente).
		return ErrPartidaNoEncontrada
	}
	return nil
}

// RevisarSaldos congela los saldos de una fecha (Dirección Financiera los revisó).
func (r *pgRepository) RevisarSaldos(ctx context.Context, empresaID, fecha, usuarioID string, congelar bool) (int, error) {
	// Descongelar limpia la marca; quién lo hizo queda en auditoria_evento, no en la fila.
	q := `
		UPDATE saldo_cuenta_diario
		SET revisado_por = NULL, revisado_en = NULL, actualizado_en = now()
		WHERE empresa_id = $1::uuid AND fecha = $2::date AND revisado_en IS NOT NULL`
	args := []any{empresaID, fecha}
	if congelar {
		q = `
		UPDATE saldo_cuenta_diario
		SET revisado_por = $3::uuid, revisado_en = now(), actualizado_en = now()
		WHERE empresa_id = $1::uuid AND fecha = $2::date AND revisado_en IS NULL`
		args = append(args, usuarioID)
	}
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("bancos: revisar saldos: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
