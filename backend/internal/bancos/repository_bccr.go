package bancos

// Persistencia del sync BCCR: lectura de cotización existente (para respetar el
// override manual), bitácora de sincronización y enumeración de empresas activas
// (para el job automático multiempresa).

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// CotizacionExistente devuelve el valor y la fuente de la cotización de una fecha (si existe).
func (r *pgRepository) CotizacionExistente(ctx context.Context, empresaID, fecha string) (string, string, bool, error) {
	var valor, fuente string
	err := r.pool.QueryRow(ctx,
		`SELECT valor::text, fuente FROM tipo_cambio_cotizacion WHERE empresa_id = $1::uuid AND fecha = $2::date`,
		empresaID, fecha).Scan(&valor, &fuente)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("bancos: cotización existente: %w", err)
	}
	return valor, fuente, true, nil
}

// UpsertCotizacionBCCR inserta/actualiza una cotización con fuente BCCR SIN pisar
// jamás un override MANUAL: el WHERE del ON CONFLICT elimina la carrera
// check-then-write (si la fila existente es MANUAL, afecta 0 filas).
// Devuelve true si la cotización quedó escrita.
func (r *pgRepository) UpsertCotizacionBCCR(ctx context.Context, empresaID, fecha string, valor decimal.Decimal) (bool, error) {
	const q = `
		INSERT INTO tipo_cambio_cotizacion (empresa_id, fecha, valor, fuente)
		VALUES ($1::uuid, $2::date, $3, 'BCCR')
		ON CONFLICT (empresa_id, fecha) DO UPDATE
			SET valor = EXCLUDED.valor, fuente = 'BCCR'
			WHERE tipo_cambio_cotizacion.fuente <> 'MANUAL'`
	tag, err := r.pool.Exec(ctx, q, empresaID, fecha, valor)
	if err != nil {
		return false, fmt.Errorf("bancos: upsert cotización bccr: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *pgRepository) RegistrarSyncBCCR(ctx context.Context, l BCCRSyncLog) error {
	const q = `
		INSERT INTO bccr_sync_log (empresa_id, fecha, indicador, valor, exito, mensaje)
		VALUES ($1::uuid, $2::date, $3, NULLIF($4,'')::numeric, $5, NULLIF($6,''))`
	if _, err := r.pool.Exec(ctx, q, l.EmpresaID, l.Fecha, l.Indicador, l.Valor, l.Exito, l.Mensaje); err != nil {
		return fmt.Errorf("bancos: registrar sync bccr: %w", err)
	}
	return nil
}

func (r *pgRepository) UltimoSyncBCCR(ctx context.Context, empresaID string) (*BCCRSyncLog, error) {
	const q = `
		SELECT to_char(fecha,'YYYY-MM-DD'), indicador, COALESCE(valor::text,''), exito,
		       COALESCE(mensaje,''), to_char(creado_en,'YYYY-MM-DD"T"HH24:MI:SS')
		FROM bccr_sync_log WHERE empresa_id = $1::uuid ORDER BY creado_en DESC LIMIT 1`
	var l BCCRSyncLog
	l.EmpresaID = empresaID
	err := r.pool.QueryRow(ctx, q, empresaID).Scan(&l.Fecha, &l.Indicador, &l.Valor, &l.Exito, &l.Mensaje, &l.CreadoEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bancos: último sync bccr: %w", err)
	}
	return &l, nil
}

// EmpresasActivas lista los ids de empresas activas (para el job de sync multiempresa).
func (r *pgRepository) EmpresasActivas(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM empresa WHERE activo = true ORDER BY nombre`)
	if err != nil {
		return nil, fmt.Errorf("bancos: empresas activas: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("bancos: scan empresa activa: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
