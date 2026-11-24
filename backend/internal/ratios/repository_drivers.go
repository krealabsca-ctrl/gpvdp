package ratios

// Consultas de los drivers de cartera. El porqué está en drivers.go.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// columnaDimension traduce la dimensión pedida a su expresión SQL.
//
// LA LISTA ES CERRADA y el valor nunca se interpola desde el pedido: `DimensionValida` lo
// comprueba antes en el servicio, y acá el `switch` solo puede devolver una de tres constantes.
// Interpolar el nombre de una columna que viene del cliente es la forma más corta de abrir una
// inyección en una consulta que por lo demás usa parámetros.
func columnaDimension(dim string) (clave, nombre string) {
	switch dim {
	case PorModalidad:
		return "COALESCE(mo.id::text, '')", "COALESCE(mo.nombre, '(sin modalidad)')"
	case PorSede:
		return "COALESCE(se.id::text, '')", "COALESCE(se.nombre, '(sin sede)')"
	default: // PorTipoServicio
		return "COALESCE(NULLIF(c.tipo_servicio, ''), '(sin tipo)')",
			"COALESCE(NULLIF(c.tipo_servicio, ''), '(sin tipo)')"
	}
}

// DriversPorDimension arma el corte de la cartera activa.
//
// El LEFT JOIN contra los cargos del período mide la COBERTURA —cuántos contratos de cada corte
// generaron cargo— que es lo que explica la brecha entre lo contratado y lo facturado. Va por
// `periodo` y no por fecha: es la misma llave con la que se generan los cargos.
func (r *pgRepository) DriversPorDimension(ctx context.Context, empresaID, dim, periodo string) ([]FilaDriver, error) {
	clave, nombre := columnaDimension(dim)
	q := `
		SELECT ` + clave + ` AS clave, ` + nombre + ` AS nombre,
		       COUNT(*)::int,
		       COALESCE(SUM(c.cuota_vigente), 0)::text,
		       COUNT(*) FILTER (WHERE cc.contrato_id IS NOT NULL)::int,
		       COALESCE(SUM(cc.monto) FILTER (WHERE cc.contrato_id IS NOT NULL), 0)::text
		FROM contrato_cxc c
		LEFT JOIN cxc_modalidad mo ON mo.id = c.modalidad_id
		LEFT JOIN cxc_sede se ON se.id = c.sede_id
		LEFT JOIN LATERAL (
		    SELECT cc.contrato_id, SUM(cc.monto) AS monto
		    FROM cargo_cxc cc
		    WHERE cc.contrato_id = c.id AND cc.periodo = $2
		    GROUP BY cc.contrato_id
		) cc ON true
		WHERE c.empresa_id = $1::uuid AND c.estado = 'ACTIVO'
		GROUP BY 1, 2
		ORDER BY 4 DESC`

	rows, err := r.pool.Query(ctx, q, empresaID, periodo)
	if err != nil {
		return nil, fmt.Errorf("ratios: drivers por %s: %w", dim, err)
	}
	defer rows.Close()

	out := []FilaDriver{}
	for rows.Next() {
		var f FilaDriver
		if err := rows.Scan(&f.Clave, &f.Nombre, &f.Contratos,
			&f.CuotaContratada, &f.ContratosFacturados, &f.CuotaFacturada); err != nil {
			return nil, fmt.Errorf("ratios: scan driver: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// TotalesDeCartera trae el total activo y cuánto de eso facturó en el período.
func (r *pgRepository) TotalesDeCartera(ctx context.Context, empresaID, periodo string) (int, decimal.Decimal, int, decimal.Decimal, int, error) {
	const q = `
		SELECT COUNT(*)::int,
		       COALESCE(SUM(c.cuota_vigente), 0)::text,
		       COUNT(*) FILTER (WHERE EXISTS (
		           SELECT 1 FROM cargo_cxc cc
		           WHERE cc.contrato_id = c.id AND cc.periodo = $2))::int,
		       COALESCE(SUM((SELECT SUM(cc.monto) FROM cargo_cxc cc
		                     WHERE cc.contrato_id = c.id AND cc.periodo = $2)), 0)::text,
		       COUNT(*) FILTER (WHERE c.fecha_inicial IS NULL)::int
		FROM contrato_cxc c
		WHERE c.empresa_id = $1::uuid AND c.estado = 'ACTIVO'`

	var activos, facturados, sinFecha int
	var contratada, facturada string
	if err := r.pool.QueryRow(ctx, q, empresaID, periodo).
		Scan(&activos, &contratada, &facturados, &facturada, &sinFecha); err != nil {
		return 0, decimal.Zero, 0, decimal.Zero, 0, fmt.Errorf("ratios: totales de cartera: %w", err)
	}
	c, err := decimal.NewFromString(contratada)
	if err != nil {
		return 0, decimal.Zero, 0, decimal.Zero, 0, fmt.Errorf("ratios: cuota contratada inválida: %w", err)
	}
	f, err := decimal.NewFromString(facturada)
	if err != nil {
		return 0, decimal.Zero, 0, decimal.Zero, 0, fmt.Errorf("ratios: cuota facturada inválida: %w", err)
	}
	return activos, c, facturados, f, sinFecha, nil
}

// AltasPorMes cuenta los contratos que entraron en cada mes del rango.
//
// Solo los que tienen `fecha_inicial`: los migrados del sistema anterior sin esa fecha no son de
// ningún mes, y repartirlos adivinando inventaría una serie. Cuántos son se informa aparte.
func (r *pgRepository) AltasPorMes(ctx context.Context, empresaID, desde, hasta string) ([]AltaMensual, error) {
	const q = `
		SELECT to_char(c.fecha_inicial, 'YYYY-MM') AS periodo,
		       COUNT(*)::int,
		       COALESCE(SUM(c.cuota_vigente), 0)::text
		FROM contrato_cxc c
		WHERE c.empresa_id = $1::uuid
		  AND c.fecha_inicial IS NOT NULL
		  AND to_char(c.fecha_inicial, 'YYYY-MM') BETWEEN $2 AND $3
		GROUP BY 1
		ORDER BY 1 DESC`

	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("ratios: altas por mes: %w", err)
	}
	defer rows.Close()

	out := []AltaMensual{}
	for rows.Next() {
		var a AltaMensual
		var cuota string
		if err := rows.Scan(&a.Periodo, &a.Altas, &cuota); err != nil {
			return nil, fmt.Errorf("ratios: scan alta: %w", err)
		}
		c, err := decimal.NewFromString(cuota)
		if err != nil {
			c = decimal.Zero
		}
		a.CuotaIncorporada = c.StringFixed(2)
		a.CuotaPromedio = promedio(c, a.Altas).StringFixed(2)
		out = append(out, a)
	}
	return out, rows.Err()
}
