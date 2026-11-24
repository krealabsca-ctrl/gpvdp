package ratios

// Las consultas de los ratios.
//
// Como `grupo` y `flujocaja`, este paquete es una VISTA: cruza CxC, CxP e Inventario y no escribe
// una sola fila. Vive aparte para que se vea que cruza módulos.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/cxp"
)

// Repository es lo que el servicio necesita de los datos.
type Repository interface {
	CarteraPorCobrar(ctx context.Context, empresaID string) (decimal.Decimal, int, error)
	FacturacionPorMes(ctx context.Context, empresaID, desde, hasta string) ([]MesDeBase, error)

	CarteraPorPagar(ctx context.Context, empresaID string) (decimal.Decimal, int, error)
	ComprasPorMes(ctx context.Context, empresaID, desde, hasta string) ([]MesDeBase, error)

	// El DIO queda fuera por ahora, a propósito. Hace falta el valor del inventario y el costo de
	// lo que sale, y hoy `inv_nivel` guarda mínimos y máximos de reposición —no existencias—, así
	// que la consulta habría que derivarla de los movimientos. Sin un solo artículo cargado,
	// construirla sería escribir contra un esquema que nadie ejercitó. El ciclo se calcula como
	// DSO − DPO y la pantalla dice qué le falta para ser el CCC completo.

	// Drivers de la cartera: el negocio por lo que lo mueve —contratos, tipo de servicio,
	// modalidad, sede— en vez de por partida contable.
	DriversPorDimension(ctx context.Context, empresaID, dim, periodo string) ([]FilaDriver, error)
	TotalesDeCartera(ctx context.Context, empresaID, periodo string) (int, decimal.Decimal, int, decimal.Decimal, int, error)
	AltasPorMes(ctx context.Context, empresaID, desde, hasta string) ([]AltaMensual, error)

	// CobrosAplicados dice si los cobros se están registrando contra los cargos. Sin eso el DSO
	// mide «lo que falta cobrar» contra una cartera que nunca baja, y da un número enorme que
	// habla del registro, no del negocio.
	CobrosAplicados(ctx context.Context, empresaID string) (int, error)
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository construye el repositorio sobre PostgreSQL.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

// ── El cobro (DSO) ──────────────────────────────────────────────────────────────────────────

// CarteraPorCobrar es lo que falta cobrar hoy: monto menos lo ya aplicado, de los cargos vivos.
func (r *pgRepository) CarteraPorCobrar(ctx context.Context, empresaID string) (decimal.Decimal, int, error) {
	const q = `
		SELECT COALESCE(SUM(cc.monto - cc.monto_aplicado), 0)::text, COUNT(*)::int
		FROM cargo_cxc cc
		WHERE cc.empresa_id = $1::uuid
		  AND cc.estado IN ('ABIERTO', 'PARCIAL')
		  AND cc.monto > cc.monto_aplicado`
	return r.montoYConteo(ctx, q, "cartera por cobrar", empresaID)
}

// facturacionPorMesSQL agrupa lo facturado por el mes en que VENCE el cargo.
//
// Por `vence_en` y no por la fecha de creación: un cargo se genera cuando se corre el proceso del
// período, a veces días después, y agrupar por eso correría toda la serie. El vencimiento es la
// fecha que el contrato define y la que el cliente conoce.
const facturacionPorMesSQL = `
	SELECT to_char(cc.vence_en, 'YYYY-MM') AS periodo,
	       SUM(cc.monto)::text,
	       EXTRACT(DAY FROM (date_trunc('month', cc.vence_en) + interval '1 month - 1 day'))::int
	FROM cargo_cxc cc
	WHERE cc.empresa_id = $1::uuid
	  AND cc.vence_en >= $2::date AND cc.vence_en <= $3::date
	GROUP BY 1, 3
	ORDER BY 1 DESC`

// FacturacionPorMes trae la base del DSO, del mes más reciente al más viejo.
func (r *pgRepository) FacturacionPorMes(ctx context.Context, empresaID, desde, hasta string) ([]MesDeBase, error) {
	return r.mesesDeBase(ctx, facturacionPorMesSQL, "facturación por mes", empresaID, desde, hasta)
}

// CobrosAplicados cuenta los cargos que recibieron algo. Cero sobre una cartera grande no
// significa que no se cobre: significa que los cobros no están llegando al sistema.
func (r *pgRepository) CobrosAplicados(ctx context.Context, empresaID string) (int, error) {
	const q = `SELECT COUNT(*)::int FROM cargo_cxc
	            WHERE empresa_id = $1::uuid AND monto_aplicado > 0`
	var n int
	if err := r.pool.QueryRow(ctx, q, empresaID).Scan(&n); err != nil {
		return 0, fmt.Errorf("ratios: contar cobros aplicados: %w", err)
	}
	return n, nil
}

// ── El pago (DPO) ───────────────────────────────────────────────────────────────────────────

// carteraPorPagarSQL usa el MISMO neto que el tablero de CxP y el flujo de caja.
//
// Se importa de `cxp` en vez de copiarse: el total no sale nunca del banco —va menos retención,
// anticipos, notas de crédito y parciales— y tres definiciones del mismo número terminan dando
// tres DPO distintos.
var carteraPorPagarSQL = `
	SELECT COALESCE(SUM(` + cxp.NetoCRCSQL + `), 0)::text, COUNT(*)::int
	FROM documento_cxp d` + cxp.JoinsNetoSQL + `
	WHERE d.empresa_id = $1::uuid
	  AND d.estado NOT IN ` + cxp.EstadosCerradosSQL

// CarteraPorPagar es lo que falta pagar hoy.
func (r *pgRepository) CarteraPorPagar(ctx context.Context, empresaID string) (decimal.Decimal, int, error) {
	return r.montoYConteo(ctx, carteraPorPagarSQL, "cartera por pagar", empresaID)
}

// comprasPorMesSQL agrupa lo comprado por fecha de EMISIÓN de la factura.
//
// Es la fecha del hecho —cuándo el proveedor facturó— y no cuándo se recibió en el ERP, que puede
// ser semanas después y correría la serie entera hacia adelante.
//
// Acá va el total NETO también: el DPO compara lo que falta pagar contra lo que se compró, y las
// dos puntas tienen que medirse igual o el ratio no significa nada.
var comprasPorMesSQL = `
	SELECT to_char(d.fecha_emision, 'YYYY-MM') AS periodo,
	       SUM(` + cxp.NetoCRCSQL + `)::text,
	       EXTRACT(DAY FROM (date_trunc('month', d.fecha_emision) + interval '1 month - 1 day'))::int
	FROM documento_cxp d` + cxp.JoinsNetoSQL + `
	WHERE d.empresa_id = $1::uuid
	  AND d.fecha_emision >= $2::date AND d.fecha_emision <= $3::date
	  AND d.estado <> 'ANULADO'
	GROUP BY 1, 3
	ORDER BY 1 DESC`

// ComprasPorMes trae la base del DPO.
func (r *pgRepository) ComprasPorMes(ctx context.Context, empresaID, desde, hasta string) ([]MesDeBase, error) {
	return r.mesesDeBase(ctx, comprasPorMesSQL, "compras por mes", empresaID, desde, hasta)
}

// ── Lectura común ───────────────────────────────────────────────────────────────────────────

func (r *pgRepository) montoYConteo(ctx context.Context, q, que, empresaID string) (decimal.Decimal, int, error) {
	var monto string
	var n int
	if err := r.pool.QueryRow(ctx, q, empresaID).Scan(&monto, &n); err != nil {
		return decimal.Zero, 0, fmt.Errorf("ratios: %s: %w", que, err)
	}
	d, err := decimal.NewFromString(monto)
	if err != nil {
		return decimal.Zero, 0, fmt.Errorf("ratios: %s inválida: %w", que, err)
	}
	return d, n, nil
}

func (r *pgRepository) mesesDeBase(ctx context.Context, q, que, empresaID, desde, hasta string) ([]MesDeBase, error) {
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("ratios: %s: %w", que, err)
	}
	defer rows.Close()

	out := make([]MesDeBase, 0, 12)
	for rows.Next() {
		var m MesDeBase
		var monto string
		if err := rows.Scan(&m.Periodo, &monto, &m.Dias); err != nil {
			return nil, fmt.Errorf("ratios: scan de %s: %w", que, err)
		}
		d, err := decimal.NewFromString(monto)
		if err != nil {
			// Un mes ilegible no vale abortar el ratio entero: entra en cero y el agotamiento lo
			// salta, que es lo mismo que un mes sin actividad.
			d = decimal.Zero
		}
		m.Monto = d
		out = append(out, m)
	}
	return out, rows.Err()
}

// ventana devuelve el rango de los últimos n meses completos más el corriente.
func ventana(hoy time.Time, meses int) (string, string) {
	hasta := hoy
	desde := time.Date(hoy.Year(), hoy.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(meses - 1), 0)
	return desde.Format("2006-01-02"), hasta.Format("2006-01-02")
}
