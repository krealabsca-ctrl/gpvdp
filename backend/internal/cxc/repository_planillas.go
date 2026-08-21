package cxc

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// FilaAsociacion es el estado de UNA asociación en un período: lo que debía mandar contra
// lo que mandó.
//
// Por qué existe esta pantalla: el descuento por asociación solidarista es el canal
// dominante (11 de 11 pagos de la muestra real). La asociación deduce de la planilla del
// trabajador, manda UN depósito y un detalle con cientos de contratos. Si no lo manda,
// cientos de clientes caen en mora **sin haber hecho nada**, y tratarlos como morosos sería
// un error de gestión y de trato.
type FilaAsociacion struct {
	AsociacionID string `json:"asociacion_id"`
	Asociacion   string `json:"asociacion"`
	Patrono      string `json:"patrono"`
	// Contratos activos de esa asociación (sin los que están en revisión).
	Contratos int `json:"contratos"`
	// Esperado: suma de los cargos que VENCEN en el período para esos contratos. Es una
	// derivación, no un acuerdo: dice cuánto debería traer la planilla según la cartera.
	Esperado string `json:"esperado"`
	Cargos   int    `json:"cargos_del_periodo"`
	// Cobrado: lo que se registró de esa asociación en el período.
	Cobrado string `json:"cobrado"`
	Cobros  int    `json:"cobros"`
	// Diferencia = cobrado − esperado. Negativa = faltó plata.
	Diferencia string `json:"diferencia"`
	// FechasBancarias: cuándo entró. Pueden ser VARIAS: el dato real trae planillas que
	// llegaron en dos transferencias.
	FechasBancarias []string `json:"fechas_bancarias"`
	// SinPlanilla: tiene contratos y cargos del período pero NO llegó ningún cobro. Es el
	// hallazgo que separa «el cliente no pagó» de «la asociación no envió».
	SinPlanilla bool `json:"sin_planilla"`
	// Depositado: lo que de verdad entró al banco, de los movimientos VINCULADOS a la
	// planilla. Es el tercer contraste y el único que no depende de lo que diga la
	// asociación: el depósito ya está en Bancos.
	Depositado string `json:"depositado"`
	// DiferenciaDeposito = depositado − cobrado. Si entró plata que el detalle no explica
	// (o al revés), es un hallazgo que hay que mirar, no un error del sistema.
	DiferenciaDeposito string `json:"diferencia_deposito"`
	// PlanillaID existe cuando ya se abrió la planilla del período (vacío = todavía no).
	PlanillaID string `json:"planilla_id"`
	Referencia string `json:"referencia"`
	Depositos  int    `json:"depositos"`
	// Estado derivado: SIN_CARGOS · NO_ENVIO · SIN_DEPOSITO · CONCILIADA · CON_DIFERENCIA.
	Estado string `json:"estado"`
}

// PanoramaAsociaciones es el estado del canal completo en un período.
type PanoramaAsociaciones struct {
	Periodo string `json:"periodo"`
	// Totales del canal.
	Asociaciones   int    `json:"asociaciones"`
	ConPlanilla    int    `json:"con_planilla"`
	SinPlanillaCnt int    `json:"sin_planilla"`
	Esperado       string `json:"esperado"`
	Cobrado        string `json:"cobrado"`
	// EnRiesgo: el esperado de las asociaciones que no enviaron nada. Es plata que no entró
	// por un tercero, no por el cliente.
	EnRiesgo          string `json:"en_riesgo"`
	ContratosEnRiesgo int    `json:"contratos_en_riesgo"`
	// Depositado y su desglose: cuántas planillas cuadran contra el banco, cuántas trajeron
	// el detalle sin depósito vinculado todavía y cuántas no coinciden.
	Depositado    string           `json:"depositado"`
	Conciliadas   int              `json:"conciliadas"`
	SinDeposito   int              `json:"sin_deposito"`
	ConDiferencia int              `json:"con_diferencia"`
	Filas         []FilaAsociacion `json:"filas"`
}

// PanoramaAsociaciones cruza, para el período pedido, los cargos que vencen (lo que la
// planilla debería traer) contra los cobros registrados de cada asociación.
//
// Se calcula VIVO, no se guarda: si se materializara, una asociación que manda la planilla
// tarde seguiría apareciendo como incumplida hasta el siguiente refresco.
func (r *pgRepository) PanoramaAsociaciones(ctx context.Context, empresaID, periodo string, tolerancia decimal.Decimal) (PanoramaAsociaciones, error) {
	if periodo == "" {
		periodo = hoyCR().Format("2006-01")
	}
	// El período se interpreta como mes: los cargos que vencen dentro de él y los cobros
	// cuya fecha BANCARIA cae en él (la que dice cuándo entró la plata de verdad).
	const q = `
		WITH asoc AS (
			SELECT a.id, a.nombre, COALESCE(a.patrono, '') AS patrono
			FROM cxc_asociacion a
			WHERE a.empresa_id = $1::uuid AND a.activa = true
		),
		contratos AS (
			SELECT c.asociacion_id, count(*)::int AS contratos
			FROM contrato_cxc c
			WHERE c.empresa_id = $1::uuid AND c.asociacion_id IS NOT NULL
			  AND c.estado = 'ACTIVO' AND c.revision_pendiente = false
			GROUP BY c.asociacion_id
		),
		cargos AS (
			SELECT c.asociacion_id,
			       count(*)::int AS cargos,
			       COALESCE(sum(g.monto), 0) AS esperado
			FROM cargo_cxc g
			JOIN contrato_cxc c ON c.id = g.contrato_id
			WHERE g.empresa_id = $1::uuid AND c.asociacion_id IS NOT NULL
			  AND g.estado <> 'ANULADO'
			  AND to_char(g.vence_en, 'YYYY-MM') = $2
			GROUP BY c.asociacion_id
		),
		cobros AS (
			SELECT k.asociacion_id,
			       count(*)::int AS cobros,
			       COALESCE(sum(k.monto), 0) AS cobrado,
			       array_remove(array_agg(DISTINCT k.fecha_bancaria), NULL) AS fechas
			FROM cobro_cxc k
			WHERE k.empresa_id = $1::uuid AND k.asociacion_id IS NOT NULL
			  AND k.estado <> 'REVERSADO'
			  AND to_char(COALESCE(k.fecha_bancaria, k.fecha_pago), 'YYYY-MM') = $2
			GROUP BY k.asociacion_id
		),
		-- La planilla del período y lo que de verdad se depositó: la suma de los movimientos
		-- bancarios que el operador vinculó. Es el dato que no depende de la asociación.
		planilla AS (
			SELECT p.asociacion_id, p.id, p.referencia,
			       COALESCE(sum(m.monto_crc), 0) AS depositado,
			       count(m.id)::int AS depositos
			FROM cxc_planilla p
			LEFT JOIN cxc_planilla_movimiento pm ON pm.planilla_id = p.id
			LEFT JOIN movimiento_bancario m ON m.id = pm.movimiento_bancario_id
			WHERE p.empresa_id = $1::uuid AND left(p.periodo, 7) = $2
			GROUP BY p.asociacion_id, p.id, p.referencia
		)
		SELECT a.id::text, a.nombre, a.patrono,
		       COALESCE(ct.contratos, 0), COALESCE(cg.cargos, 0), COALESCE(cg.esperado, 0),
		       COALESCE(cb.cobros, 0), COALESCE(cb.cobrado, 0), COALESCE(cb.fechas, '{}'),
		       COALESCE(pl.id::text, ''), COALESCE(pl.referencia, ''),
		       COALESCE(pl.depositado, 0), COALESCE(pl.depositos, 0)
		FROM asoc a
		LEFT JOIN contratos ct ON ct.asociacion_id = a.id
		LEFT JOIN cargos cg ON cg.asociacion_id = a.id
		LEFT JOIN cobros cb ON cb.asociacion_id = a.id
		LEFT JOIN planilla pl ON pl.asociacion_id = a.id
		ORDER BY COALESCE(cg.esperado, 0) DESC, a.nombre`

	rows, err := r.pool.Query(ctx, q, empresaID, periodo)
	if err != nil {
		return PanoramaAsociaciones{}, fmt.Errorf("cxc: panorama de asociaciones: %w", err)
	}
	defer rows.Close()

	out := PanoramaAsociaciones{Periodo: periodo, Filas: []FilaAsociacion{}}
	totalEsperado, totalCobrado, enRiesgo := decimal.Zero, decimal.Zero, decimal.Zero
	totalDepositado := decimal.Zero
	for rows.Next() {
		var (
			f                 FilaAsociacion
			esperado, cobrado decimal.Decimal
			depositado        decimal.Decimal
			fechas            []time.Time
		)
		if err := rows.Scan(&f.AsociacionID, &f.Asociacion, &f.Patrono,
			&f.Contratos, &f.Cargos, &esperado, &f.Cobros, &cobrado, &fechas,
			&f.PlanillaID, &f.Referencia, &depositado, &f.Depositos); err != nil {
			return PanoramaAsociaciones{}, fmt.Errorf("cxc: scan asociación: %w", err)
		}
		f.Esperado, f.Cobrado = esperado.String(), cobrado.String()
		f.Diferencia = cobrado.Sub(esperado).String()
		f.FechasBancarias = []string{}
		for _, t := range fechas {
			f.FechasBancarias = append(f.FechasBancarias, t.Format("2006-01-02"))
		}
		// «No envió» solo tiene sentido si había algo que enviar: una asociación sin cargos
		// del período no está incumpliendo nada.
		f.SinPlanilla = f.Cobros == 0 && f.Cargos > 0
		f.Depositado = depositado.String()
		f.DiferenciaDeposito = depositado.Sub(cobrado).String()
		// El estado sale de los TRES montos, con la misma función que usa el detalle de la
		// planilla: el panorama y la ficha no pueden decir cosas distintas.
		f.Estado = estadoPlanilla(f.PlanillaID != "", esperado, cobrado, depositado, tolerancia)
		switch f.Estado {
		case "CONCILIADA":
			out.Conciliadas++
		case "SIN_DEPOSITO":
			out.SinDeposito++
		case "CON_DIFERENCIA":
			out.ConDiferencia++
		}
		totalDepositado = totalDepositado.Add(depositado)

		out.Asociaciones++
		if f.Cobros > 0 {
			out.ConPlanilla++
		}
		if f.SinPlanilla {
			out.SinPlanillaCnt++
			enRiesgo = enRiesgo.Add(esperado)
			out.ContratosEnRiesgo += f.Contratos
		}
		totalEsperado = totalEsperado.Add(esperado)
		totalCobrado = totalCobrado.Add(cobrado)
		out.Filas = append(out.Filas, f)
	}
	if err := rows.Err(); err != nil {
		return PanoramaAsociaciones{}, err
	}
	out.Esperado, out.Cobrado, out.EnRiesgo = totalEsperado.String(), totalCobrado.String(), enRiesgo.String()
	out.Depositado = totalDepositado.String()
	return out, nil
}
