package cxc

// Suspensión del servicio por mora: **18 MESES de mora, o su equivalencia en cuotas** (regla
// del negocio, precisada por el usuario).
//
// La distinción importa y no es cosmética:
//
//	Mensual      1 cuota = 1 mes    ⇒ 18 meses son 18 cuotas
//	Quincenal    1 cuota = 0,5 mes  ⇒ 18 meses son 36 cuotas
//	Trimestral   1 cuota = 3 meses  ⇒ 18 meses son 6 cuotas
//	Anual        1 cuota = 12 meses ⇒ 18 meses son 1,5 cuotas
//
// Contar cuotas habría cortado a un quincenal a los 9 meses de atraso y habría dejado a un
// anual acumular 18 años. La equivalencia sale del ciclo que ya está en el catálogo, así que
// no hubo que preguntar nada nuevo.
//
// Las dos medidas se muestran: los MESES deciden, las CUOTAS son el hecho concreto que el
// operador le dice al cliente («debe 36 cuotas»).

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// EstadoSuspension es lo que hace falta saber sobre la mora acumulada de un contrato.
type EstadoSuspension struct {
	Contrato       string `json:"contrato"`
	Estado         string `json:"estado"`
	CuotasVencidas int    `json:"cuotas_vencidas"`
	// MesesMora es la medida que decide: las cuotas vencidas convertidas a meses según la
	// modalidad del contrato.
	MesesMora string `json:"meses_mora"`
	Modalidad string `json:"modalidad"`
	// MesesPorCuota explica la conversión en pantalla («cada cuota son 0,5 meses»).
	MesesPorCuota string `json:"meses_por_cuota"`
	Tope          int    `json:"tope_meses"`
	// CuotasEquivalentes: a cuántas cuotas de ESTE contrato equivale el tope. Es el número
	// que el operador necesita para explicarle al cliente cuánto le falta.
	CuotasEquivalentes string `json:"cuotas_equivalentes"`
	// ParaSuspender: llegó o pasó el tope. NO se suspende solo: la regla dice cuándo se
	// puede, la decisión la toma una persona con permiso.
	ParaSuspender bool   `json:"para_suspender"`
	Saldo         string `json:"saldo"`
	// La suspensión vigente, si hay.
	SuspendidoEn      string `json:"suspendido_en"`
	SuspendidoPor     string `json:"suspendido_por"`
	MotivoSuspension  string `json:"motivo_suspension"`
	CuotasAlSuspender int    `json:"cuotas_al_suspender"`
	MesesAlSuspender  string `json:"meses_al_suspender"`
}

var (
	ErrContratoYaSuspendido = errors.New("cxc: el contrato ya está suspendido")
	ErrContratoNoSuspendido = errors.New("cxc: el contrato no está suspendido")
)

// sqlMesesPorCuota convierte una cuota en meses según el ciclo de la modalidad. Es la pieza
// que hace real «18 meses o su equivalencia». `%s` es el alias de cxc_modalidad.
const sqlMesesPorCuota = `(CASE WHEN %[1]s.quincenal THEN 0.5 ELSE COALESCE(%[1]s.meses_ciclo, 1)::numeric END)`

// sqlCuotasVencidas cuenta las CUOTAS vencidas sin pagar de un contrato. Un cargo pagado a
// medias cuenta: no está pagado. Es la misma expresión en los tres lugares que la usan
// (ficha, suspensión y cola) para que las tres cuenten igual.
const sqlCuotasVencidas = `(SELECT count(*)::int FROM cargo_cxc g
	WHERE g.contrato_id = %s AND g.estado IN ('ABIERTO','PARCIAL')
	  AND g.vence_en < $HOY AND g.monto > g.monto_aplicado)`

// EstadoDeSuspension calcula los meses de mora y dice si el contrato llegó al tope.
func (r *pgRepository) EstadoDeSuspension(ctx context.Context, empresaID, numero string, topeMeses int) (EstadoSuspension, error) {
	hoy := "(now() AT TIME ZONE 'America/Costa_Rica')::date"
	cuotasSQL := replaceHoy(fmt.Sprintf(sqlCuotasVencidas, "c.id"), hoy)
	porCuota := fmt.Sprintf(sqlMesesPorCuota, "mo")

	var e EstadoSuspension
	var saldo, meses, mesesPorCuota decimal.Decimal
	var susEn, susPor, susMotivo *string
	var cuotasAl *int
	var mesesAl *decimal.Decimal
	err := r.pool.QueryRow(ctx, `
		SELECT c.numero, c.estado, COALESCE(mo.nombre, ''),
		       `+cuotasSQL+`,
		       `+porCuota+`,
		       (`+cuotasSQL+` * `+porCuota+`),
		       COALESCE((SELECT sum(g.monto - g.monto_aplicado) FROM cargo_cxc g
		        WHERE g.contrato_id = c.id AND g.estado IN ('ABIERTO','PARCIAL')), 0),
		       to_char(s.suspendido_en AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD HH24:MI'),
		       u.nombre, s.motivo, s.cuotas_vencidas, s.meses_mora
		FROM contrato_cxc c
		LEFT JOIN cxc_modalidad mo ON mo.id = c.modalidad_id
		LEFT JOIN cxc_suspension s ON s.contrato_id = c.id AND s.reactivado_en IS NULL
		LEFT JOIN usuario u ON u.id = s.suspendido_por
		WHERE c.empresa_id = $1::uuid AND c.numero = $2`,
		empresaID, numero).Scan(&e.Contrato, &e.Estado, &e.Modalidad, &e.CuotasVencidas,
		&mesesPorCuota, &meses, &saldo, &susEn, &susPor, &susMotivo, &cuotasAl, &mesesAl)
	if errors.Is(err, pgx.ErrNoRows) {
		return EstadoSuspension{}, ErrContratoNoEncontrado
	}
	if err != nil {
		return EstadoSuspension{}, fmt.Errorf("cxc: estado de suspensión: %w", err)
	}
	e.Saldo = saldo.String()
	e.MesesMora = meses.String()
	e.MesesPorCuota = mesesPorCuota.String()
	e.Tope = topeMeses
	// A cuántas cuotas de ESTE contrato equivale el tope. Con mesesPorCuota = 0 (dato del
	// catálogo incompleto) no se divide: se deja vacío en vez de mentir.
	if mesesPorCuota.Sign() > 0 {
		e.CuotasEquivalentes = decimal.NewFromInt(int64(topeMeses)).Div(mesesPorCuota).String()
	}
	e.ParaSuspender = meses.GreaterThanOrEqual(decimal.NewFromInt(int64(topeMeses))) && e.Estado == "ACTIVO"
	e.SuspendidoEn = valorOVacio(susEn)
	e.SuspendidoPor = valorOVacio(susPor)
	e.MotivoSuspension = valorOVacio(susMotivo)
	if cuotasAl != nil {
		e.CuotasAlSuspender = *cuotasAl
	}
	if mesesAl != nil {
		e.MesesAlSuspender = mesesAl.String()
	}
	return e, nil
}

// Suspender corta el servicio y guarda la FOTO de la mora al momento —los meses Y las cuotas—
// porque «¿cuánto debía cuando le cortamos?» no se puede reconstruir después.
func (r *pgRepository) Suspender(ctx context.Context, empresaID, numero, motivo, usuarioID string, topeMeses int) (EstadoSuspension, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return EstadoSuspension{}, fmt.Errorf("cxc: begin suspender: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	contratoID, estado, err := contratoParaCambioDeEstado(ctx, tx, empresaID, numero)
	if err != nil {
		return EstadoSuspension{}, err
	}
	if estado == "SUSPENDIDO" {
		return EstadoSuspension{}, ErrContratoYaSuspendido
	}

	hoy := "(now() AT TIME ZONE 'America/Costa_Rica')::date"
	cuotasSQL := replaceHoy(fmt.Sprintf(sqlCuotasVencidas, "c.id"), hoy)
	porCuota := fmt.Sprintf(sqlMesesPorCuota, "mo")
	var cuotas int
	var meses, saldo decimal.Decimal
	if err := tx.QueryRow(ctx, `
		SELECT `+cuotasSQL+`, (`+cuotasSQL+` * `+porCuota+`),
		       COALESCE((SELECT sum(g.monto - g.monto_aplicado) FROM cargo_cxc g
		        WHERE g.contrato_id = c.id AND g.estado IN ('ABIERTO','PARCIAL')), 0)
		FROM contrato_cxc c
		LEFT JOIN cxc_modalidad mo ON mo.id = c.modalidad_id
		WHERE c.id = $1::uuid`,
		contratoID).Scan(&cuotas, &meses, &saldo); err != nil {
		return EstadoSuspension{}, fmt.Errorf("cxc: medir la mora: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE contrato_cxc SET estado = 'SUSPENDIDO', actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, contratoID); err != nil {
		return EstadoSuspension{}, fmt.Errorf("cxc: suspender contrato: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cxc_suspension (empresa_id, contrato_id, cuotas_vencidas, meses_mora,
		                            saldo_al_suspender, motivo, suspendido_por)
		VALUES ($1::uuid, $2::uuid, $3, $4::numeric, $5::numeric, $6, NULLIF($7,'')::uuid)`,
		empresaID, contratoID, cuotas, meses.String(), saldo.String(), motivo, usuarioID); err != nil {
		return EstadoSuspension{}, fmt.Errorf("cxc: registrar suspensión: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return EstadoSuspension{}, fmt.Errorf("cxc: commit suspender: %w", err)
	}
	return r.EstadoDeSuspension(ctx, empresaID, numero, topeMeses)
}

// Reactivar devuelve el contrato al servicio. La suspensión NO se borra: se cierra con quién la
// levantó y por qué, porque «¿este contrato estuvo cortado?» se pregunta después.
func (r *pgRepository) Reactivar(ctx context.Context, empresaID, numero, motivo, usuarioID string, topeMeses int) (EstadoSuspension, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return EstadoSuspension{}, fmt.Errorf("cxc: begin reactivar: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	contratoID, estado, err := contratoParaCambioDeEstado(ctx, tx, empresaID, numero)
	if err != nil {
		return EstadoSuspension{}, err
	}
	if estado != "SUSPENDIDO" {
		return EstadoSuspension{}, ErrContratoNoSuspendido
	}
	if _, err := tx.Exec(ctx,
		`UPDATE contrato_cxc SET estado = 'ACTIVO', actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, contratoID); err != nil {
		return EstadoSuspension{}, fmt.Errorf("cxc: reactivar contrato: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cxc_suspension
		SET reactivado_por = NULLIF($3,'')::uuid, reactivado_en = now(), reactivacion_motivo = $4
		WHERE empresa_id = $1::uuid AND contrato_id = $2::uuid AND reactivado_en IS NULL`,
		empresaID, contratoID, usuarioID, motivo); err != nil {
		return EstadoSuspension{}, fmt.Errorf("cxc: cerrar suspensión: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return EstadoSuspension{}, fmt.Errorf("cxc: commit reactivar: %w", err)
	}
	return r.EstadoDeSuspension(ctx, empresaID, numero, topeMeses)
}

func contratoParaCambioDeEstado(ctx context.Context, tx pgx.Tx, empresaID, numero string) (string, string, error) {
	var id, estado string
	err := tx.QueryRow(ctx,
		`SELECT id::text, estado FROM contrato_cxc
		 WHERE empresa_id = $1::uuid AND numero = $2 FOR UPDATE`,
		empresaID, numero).Scan(&id, &estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrContratoNoEncontrado
	}
	if err != nil {
		return "", "", fmt.Errorf("cxc: buscar contrato: %w", err)
	}
	return id, estado, nil
}
