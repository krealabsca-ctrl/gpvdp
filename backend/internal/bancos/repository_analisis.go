package bancos

// Fase B — análisis visual: queries agregadas para la tendencia, el calendario
// diario y el desglose por cuenta. Mismos criterios que TotalesPeriodo (§13):
// ingresos/gastos excluyen es_traslado; solo movimientos incluido = true.

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

func (r *pgRepository) SerieMensual(ctx context.Context, empresaID, desde, hasta string) ([]SerieMensualPunto, error) {
	// Misma definición de ingreso/gasto que el KPI del encabezado (naturaleza.go): si la tendencia
	// usara otra, el gráfico y el número de arriba dirían cosas distintas del mismo mes.
	q := `
		SELECT to_char(m.fecha, 'YYYY-MM') AS periodo,
		       ` + sqlIngresoNeto + `, ` + sqlGastoNeto + `,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE m.estado_clasificacion = 'NO_IDENTIFICADO')
		FROM movimiento_bancario m
		` + joinConcepto + `
		WHERE m.empresa_id = $1::uuid AND m.incluido = true
		  AND to_char(m.fecha, 'YYYY-MM') BETWEEN $2 AND $3
		GROUP BY 1
		ORDER BY 1`
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("bancos: serie mensual: %w", err)
	}
	defer rows.Close()
	var out []SerieMensualPunto
	for rows.Next() {
		var (
			p        SerieMensualPunto
			ing, gas decimal.Decimal
		)
		if err := rows.Scan(&p.Periodo, &ing, &gas, &p.Movimientos, &p.NoIdentificados); err != nil {
			return nil, fmt.Errorf("bancos: scan serie mensual: %w", err)
		}
		p.Ingresos = ing.String()
		p.Gastos = gas.String()
		p.EBITDA = ing.Sub(gas).String()
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *pgRepository) CalendarioDiario(ctx context.Context, empresaID, periodo string) ([]DiaCalendario, error) {
	q := `
		SELECT m.fecha,
		       ` + sqlIngresoNeto + `, ` + sqlGastoNeto + `,
		       COUNT(*)
		FROM movimiento_bancario m
		` + joinConcepto + `
		WHERE m.empresa_id = $1::uuid AND m.incluido = true AND to_char(m.fecha, 'YYYY-MM') = $2
		GROUP BY m.fecha
		ORDER BY m.fecha`
	rows, err := r.pool.Query(ctx, q, empresaID, periodo)
	if err != nil {
		return nil, fmt.Errorf("bancos: calendario diario: %w", err)
	}
	defer rows.Close()
	var out []DiaCalendario
	for rows.Next() {
		var (
			d         DiaCalendario
			fecha     time.Time
			cred, deb decimal.Decimal
		)
		if err := rows.Scan(&fecha, &cred, &deb, &d.Movimientos); err != nil {
			return nil, fmt.Errorf("bancos: scan calendario: %w", err)
		}
		d.Fecha = fecha.Format("2006-01-02")
		d.Creditos = cred.String()
		d.Debitos = deb.String()
		d.Neto = cred.Sub(deb).String()
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *pgRepository) ResumenPorCuenta(ctx context.Context, empresaID, periodo string) ([]CuentaResumen, error) {
	const q = `
		SELECT cb.id::text, b.nombre, COALESCE(cb.alias, ''),
		       COALESCE(SUM(CASE WHEN m.credito > 0 AND NOT m.es_traslado THEN m.monto_crc ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN m.debito  > 0 AND NOT m.es_traslado THEN m.monto_crc ELSE 0 END), 0),
		       COUNT(*)
		FROM movimiento_bancario m
		JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		JOIN banco b ON b.id = cb.banco_id
		WHERE m.empresa_id = $1::uuid AND m.incluido = true AND to_char(m.fecha, 'YYYY-MM') = $2
		GROUP BY cb.id, b.nombre, cb.alias
		ORDER BY 4 DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, periodo)
	if err != nil {
		return nil, fmt.Errorf("bancos: resumen por cuenta: %w", err)
	}
	defer rows.Close()
	var out []CuentaResumen
	for rows.Next() {
		var (
			c         CuentaResumen
			cred, deb decimal.Decimal
		)
		if err := rows.Scan(&c.CuentaID, &c.Banco, &c.Alias, &cred, &deb, &c.Movimientos); err != nil {
			return nil, fmt.Errorf("bancos: scan resumen cuenta: %w", err)
		}
		c.Creditos = cred.String()
		c.Debitos = deb.String()
		out = append(out, c)
	}
	return out, rows.Err()
}
