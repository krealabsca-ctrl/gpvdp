package cxc

// Conciliación de la planilla de asociación contra el DEPÓSITO bancario.
//
// El tercer contraste, el que faltaba:
//
//	ESPERADO    los cargos que vencen en el período para los contratos de esa asociación
//	REGISTRADO  los cobros del detalle que mandó la asociación (ya importados)
//	DEPOSITADO  el movimiento bancario que de verdad entró  ← esto es lo nuevo
//
// Los tres se DERIVAN. La planilla solo guarda que existe, su referencia (el comprobante
// que la asociación manda por correo) y a qué movimientos bancarios está vinculada.
//
// Por qué el operador vincula y el sistema no adivina: en los datos reales de la empresa,
// la descripción del banco casi nunca dice de qué asociación es el depósito («TEF
// DE:ASOCIACION SOLIDARISTA», 31 veces). Emparejar por nombre daría falsos positivos que
// darían por conciliada a la asociación equivocada.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// MovimientoVinculado es un depósito ya asociado a una planilla.
type MovimientoVinculado struct {
	ID          string `json:"id"`
	Fecha       string `json:"fecha"`
	Descripcion string `json:"descripcion"`
	Monto       string `json:"monto"`
	Cuenta      string `json:"cuenta"`
	Banco       string `json:"banco"`
}

// CandidatoDeposito es un crédito de Bancos que PODRÍA ser el depósito de esta planilla.
type CandidatoDeposito struct {
	MovimientoVinculado
	Clasificacion string `json:"clasificacion"`
	// CalzaMonto: el crédito es igual a lo que falta por conciliar. Es la señal más fuerte.
	CalzaMonto bool `json:"calza_monto"`
	// NombraLaAsociacion: la descripción del banco menciona el nombre (o su raíz). Solo
	// algunos bancos lo traen; cuando aparece, es una pista fuerte.
	NombraLaAsociacion bool `json:"nombra_la_asociacion"`
	// Diferencia contra lo que falta: negativa = el depósito es menor.
	Diferencia string `json:"diferencia"`
}

// PlanillaDetalle es una planilla con sus tres montos y sus depósitos.
type PlanillaDetalle struct {
	ID           string                `json:"id"`
	AsociacionID string                `json:"asociacion_id"`
	Asociacion   string                `json:"asociacion"`
	Periodo      string                `json:"periodo"`
	Referencia   string                `json:"referencia"`
	Nota         string                `json:"nota"`
	Esperado     string                `json:"esperado"`
	Registrado   string                `json:"registrado"`
	Depositado   string                `json:"depositado"`
	Estado       string                `json:"estado"`
	Movimientos  []MovimientoVinculado `json:"movimientos"`
	CreadoEn     string                `json:"creado_en"`
}

var (
	ErrPlanillaNoEncontrada   = errors.New("cxc: la planilla no existe en esta empresa")
	ErrMovimientoAjeno        = errors.New("cxc: el movimiento bancario no existe en esta empresa")
	ErrMovimientoNoEsCredito  = errors.New("cxc: solo se puede vincular un crédito: un débito no es un depósito recibido")
	ErrMovimientoYaVinculado  = errors.New("cxc: ese depósito ya está vinculado a otra planilla")
	ErrAsociacionNoEncontrada = errors.New("cxc: la asociación no existe en esta empresa")
)

// AbrirPlanilla crea (o encuentra) la planilla de una asociación en un período. Es
// idempotente: registrar el comprobante dos veces no crea dos planillas.
func (r *pgRepository) AbrirPlanilla(ctx context.Context, empresaID, asociacionID, periodo, referencia, nota, usuarioID string) (string, error) {
	var ok bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM cxc_asociacion WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
		empresaID, asociacionID).Scan(&ok); err != nil {
		return "", fmt.Errorf("cxc: verificar asociación: %w", err)
	}
	if !ok {
		return "", ErrAsociacionNoEncontrada
	}
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO cxc_planilla (empresa_id, asociacion_id, periodo, referencia, nota, creado_por)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, NULLIF($6,'')::uuid)
		ON CONFLICT (empresa_id, asociacion_id, periodo) DO UPDATE
		SET referencia = CASE WHEN EXCLUDED.referencia <> '' THEN EXCLUDED.referencia ELSE cxc_planilla.referencia END,
		    nota = CASE WHEN EXCLUDED.nota <> '' THEN EXCLUDED.nota ELSE cxc_planilla.nota END,
		    actualizado_en = now()
		RETURNING id::text`,
		empresaID, asociacionID, periodo, referencia, nota, usuarioID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("cxc: abrir planilla: %w", err)
	}
	return id, nil
}

// VincularDeposito ata un movimiento bancario a la planilla.
func (r *pgRepository) VincularDeposito(ctx context.Context, empresaID, planillaID, movimientoID, usuarioID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cxc: begin vincular depósito: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existe bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM cxc_planilla WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
		empresaID, planillaID).Scan(&existe); err != nil {
		return fmt.Errorf("cxc: verificar planilla: %w", err)
	}
	if !existe {
		return ErrPlanillaNoEncontrada
	}

	// El movimiento tiene que ser de ESTA empresa y ser un crédito: vincular un débito
	// daría por recibida una plata que en realidad salió.
	var credito decimal.Decimal
	err = tx.QueryRow(ctx,
		`SELECT credito FROM movimiento_bancario WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, movimientoID).Scan(&credito)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMovimientoAjeno
	}
	if err != nil {
		return fmt.Errorf("cxc: leer movimiento: %w", err)
	}
	if credito.Sign() <= 0 {
		return ErrMovimientoNoEsCredito
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO cxc_planilla_movimiento (empresa_id, planilla_id, movimiento_bancario_id, vinculado_por)
		VALUES ($1::uuid, $2::uuid, $3::uuid, NULLIF($4,'')::uuid)`,
		empresaID, planillaID, movimientoID, usuarioID)
	// El UNIQUE del movimiento es el guardarraíl: un mismo depósito no puede dar por
	// conciliadas dos asociaciones.
	if esViolacionUnicaPG(err) {
		return ErrMovimientoYaVinculado
	}
	if err != nil {
		return fmt.Errorf("cxc: vincular depósito: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cxc: commit vincular depósito: %w", err)
	}
	return nil
}

// DesvincularDeposito deshace el vínculo. No toca el movimiento: sigue en Bancos con su
// clasificación intacta, solo deja de contar como depósito de esta planilla.
func (r *pgRepository) DesvincularDeposito(ctx context.Context, empresaID, planillaID, movimientoID string) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM cxc_planilla_movimiento
		WHERE empresa_id = $1::uuid AND planilla_id = $2::uuid AND movimiento_bancario_id = $3::uuid`,
		empresaID, planillaID, movimientoID)
	if err != nil {
		return fmt.Errorf("cxc: desvincular depósito: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPlanillaNoEncontrada
	}
	return nil
}

// CandidatosDeposito propone los créditos de Bancos que podrían ser el depósito.
//
// El criterio, en este orden: no estar vinculado a ninguna planilla, ser un crédito del
// período (con unos días de margen porque la plata suele entrar después del corte), y
// ordenados por si el monto calza con lo que falta y si la descripción nombra la asociación.
func (r *pgRepository) CandidatosDeposito(ctx context.Context, empresaID, planillaID string, margenDias int) ([]CandidatoDeposito, error) {
	// Lo que falta por conciliar sale de la propia planilla: es contra eso que se compara.
	var asociacion, periodo string
	var registrado, depositado decimal.Decimal
	err := r.pool.QueryRow(ctx, `
		SELECT a.nombre, p.periodo,
		       COALESCE((
		         SELECT sum(co.monto) FROM cobro_cxc co
		         WHERE co.empresa_id = p.empresa_id AND co.asociacion_id = p.asociacion_id
		           AND co.estado <> 'REVERSADO'
		           AND to_char(COALESCE(co.fecha_bancaria, co.fecha_pago), 'YYYY-MM') = left(p.periodo, 7)
		       ), 0),
		       COALESCE((
		         SELECT sum(m.monto_crc) FROM cxc_planilla_movimiento pm
		         JOIN movimiento_bancario m ON m.id = pm.movimiento_bancario_id
		         WHERE pm.planilla_id = p.id
		       ), 0)
		FROM cxc_planilla p
		JOIN cxc_asociacion a ON a.id = p.asociacion_id
		WHERE p.empresa_id = $1::uuid AND p.id = $2::uuid`,
		empresaID, planillaID).Scan(&asociacion, &periodo, &registrado, &depositado)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPlanillaNoEncontrada
	}
	if err != nil {
		return nil, fmt.Errorf("cxc: datos de la planilla: %w", err)
	}
	falta := registrado.Sub(depositado)

	// La raíz del nombre para la pista textual: «ASEPANDUIT SOLIDARISTA» → «ASEPANDUIT».
	// Se usa solo como PISTA visible, nunca para vincular solo.
	rows, err := r.pool.Query(ctx, `
		WITH raiz AS (SELECT upper(split_part($3, ' ', 1)) AS token)
		SELECT m.id::text, m.fecha::text, COALESCE(m.descripcion,''), m.monto_crc::text,
		       COALESCE(c.alias,''), COALESCE(b.nombre,''), COALESCE(l.nombre,''),
		       (m.monto_crc = $4::numeric) AS calza,
		       (length((SELECT token FROM raiz)) >= 4 AND upper(COALESCE(m.descripcion,'')) LIKE '%' || (SELECT token FROM raiz) || '%') AS nombra
		FROM movimiento_bancario m
		JOIN cuenta_bancaria c ON c.id = m.cuenta_bancaria_id
		JOIN banco b ON b.id = c.banco_id
		LEFT JOIN clasificacion l ON l.id = m.clasificacion_id
		WHERE m.empresa_id = $1::uuid
		  AND m.credito > 0
		  AND m.fecha BETWEEN (to_date(left($2,7), 'YYYY-MM'))
		      AND (to_date(left($2,7), 'YYYY-MM') + interval '1 month' + ($5 || ' days')::interval)::date
		  AND NOT EXISTS (SELECT 1 FROM cxc_planilla_movimiento pm WHERE pm.movimiento_bancario_id = m.id)
		ORDER BY calza DESC, nombra DESC, abs(m.monto_crc - $4::numeric), m.fecha
		LIMIT 60`,
		empresaID, periodo, asociacion, falta.String(), fmt.Sprint(margenDias))
	if err != nil {
		return nil, fmt.Errorf("cxc: candidatos de depósito: %w", err)
	}
	defer rows.Close()
	out := []CandidatoDeposito{}
	for rows.Next() {
		var c CandidatoDeposito
		var monto decimal.Decimal
		if err := rows.Scan(&c.ID, &c.Fecha, &c.Descripcion, &monto, &c.Cuenta, &c.Banco,
			&c.Clasificacion, &c.CalzaMonto, &c.NombraLaAsociacion); err != nil {
			return nil, fmt.Errorf("cxc: scan candidato: %w", err)
		}
		c.Monto = monto.String()
		c.Diferencia = monto.Sub(falta).String()
		out = append(out, c)
	}
	return out, rows.Err()
}

// PlanillaDeAsociacion devuelve la planilla de una asociación en un período con los tres
// montos derivados y sus depósitos. Si no existe todavía, devuelve una vacía «PENDIENTE».
func (r *pgRepository) PlanillaDeAsociacion(ctx context.Context, empresaID, asociacionID, periodo string, tolerancia decimal.Decimal) (PlanillaDetalle, error) {
	hoy := "(now() AT TIME ZONE 'America/Costa_Rica')::date"
	var d PlanillaDetalle
	var esperado, registrado, depositado decimal.Decimal
	var planillaID, referencia, nota, creado *string

	err := r.pool.QueryRow(ctx, `
		SELECT a.id::text, a.nombre,
		       p.id::text, p.referencia, p.nota,
		       to_char(p.creado_en AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD HH24:MI'),
		       -- Esperado: los cargos que VENCEN en el período para los contratos de la
		       -- asociación. No es un acuerdo con ella: sale de la cartera.
		       COALESCE((
		         SELECT sum(g.monto) FROM cargo_cxc g
		         JOIN contrato_cxc k ON k.id = g.contrato_id
		         WHERE k.empresa_id = $1::uuid AND k.asociacion_id = a.id
		           AND to_char(g.vence_en, 'YYYY-MM') = left($3, 7)
		           AND g.estado <> 'ANULADO'
		       ), 0),
		       -- Registrado: los cobros del detalle, por fecha bancaria (la que dice cuándo
		       -- entró la plata), sin los reversados.
		       COALESCE((
		         SELECT sum(co.monto) FROM cobro_cxc co
		         WHERE co.empresa_id = $1::uuid AND co.asociacion_id = a.id
		           AND co.estado <> 'REVERSADO'
		           AND to_char(COALESCE(co.fecha_bancaria, co.fecha_pago), 'YYYY-MM') = left($3, 7)
		       ), 0),
		       -- Depositado: la suma de los movimientos bancarios vinculados.
		       COALESCE((
		         SELECT sum(m.monto_crc) FROM cxc_planilla_movimiento pm
		         JOIN movimiento_bancario m ON m.id = pm.movimiento_bancario_id
		         WHERE pm.planilla_id = p.id
		       ), 0)
		FROM cxc_asociacion a
		LEFT JOIN cxc_planilla p
		       ON p.empresa_id = a.empresa_id AND p.asociacion_id = a.id AND p.periodo = $3
		WHERE a.empresa_id = $1::uuid AND a.id = $2::uuid`,
		empresaID, asociacionID, periodo).
		Scan(&d.AsociacionID, &d.Asociacion, &planillaID, &referencia, &nota, &creado,
			&esperado, &registrado, &depositado)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlanillaDetalle{}, ErrAsociacionNoEncontrada
	}
	if err != nil {
		return PlanillaDetalle{}, fmt.Errorf("cxc: planilla de la asociación: %w", err)
	}
	_ = hoy
	d.Periodo = periodo
	d.Esperado, d.Registrado, d.Depositado = esperado.String(), registrado.String(), depositado.String()
	if planillaID != nil {
		d.ID = *planillaID
		d.Referencia, d.Nota, d.CreadoEn = valorOVacio(referencia), valorOVacio(nota), valorOVacio(creado)
	}
	d.Estado = estadoPlanilla(d.ID != "", esperado, registrado, depositado, tolerancia)
	d.Movimientos = []MovimientoVinculado{}
	if d.ID == "" {
		return d, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT m.id::text, m.fecha::text, COALESCE(m.descripcion,''), m.monto_crc::text,
		       COALESCE(c.alias,''), COALESCE(b.nombre,'')
		FROM cxc_planilla_movimiento pm
		JOIN movimiento_bancario m ON m.id = pm.movimiento_bancario_id
		JOIN cuenta_bancaria c ON c.id = m.cuenta_bancaria_id
		JOIN banco b ON b.id = c.banco_id
		WHERE pm.empresa_id = $1::uuid AND pm.planilla_id = $2::uuid
		ORDER BY m.fecha, m.id`, empresaID, d.ID)
	if err != nil {
		return PlanillaDetalle{}, fmt.Errorf("cxc: depósitos de la planilla: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var mv MovimientoVinculado
		if err := rows.Scan(&mv.ID, &mv.Fecha, &mv.Descripcion, &mv.Monto, &mv.Cuenta, &mv.Banco); err != nil {
			return PlanillaDetalle{}, fmt.Errorf("cxc: scan depósito: %w", err)
		}
		d.Movimientos = append(d.Movimientos, mv)
	}
	return d, rows.Err()
}

// estadoPlanilla deriva el estado de los tres montos. El orden de las preguntas ES la regla
// de negocio:
//
//	SIN_CARGOS      la asociación no tenía nada que cobrar este período: no incumple nada
//	NO_ENVIO        había cargos y no llegó ni un cobro ni un depósito
//	SIN_DEPOSITO    mandó el detalle pero el depósito todavía no se vinculó
//	CONCILIADA      lo depositado calza con lo registrado (dentro de la tolerancia)
//	CON_DIFERENCIA  entró plata pero no coincide con el detalle: es un hallazgo, no un error
func estadoPlanilla(existe bool, esperado, registrado, depositado, tolerancia decimal.Decimal) string {
	if esperado.Sign() <= 0 && registrado.Sign() <= 0 && depositado.Sign() <= 0 {
		return "SIN_CARGOS"
	}
	if registrado.Sign() <= 0 && depositado.Sign() <= 0 {
		return "NO_ENVIO"
	}
	if depositado.Sign() <= 0 {
		return "SIN_DEPOSITO"
	}
	if depositado.Sub(registrado).Abs().LessThanOrEqual(tolerancia) {
		return "CONCILIADA"
	}
	return "CON_DIFERENCIA"
}

// DatosDePlanilla devuelve (asociacion_id, periodo) de una planilla: lo que hace falta para
// recargar su ficha después de vincular o desvincular.
func (r *pgRepository) DatosDePlanilla(ctx context.Context, empresaID, planillaID string) (string, string, error) {
	var asociacionID, periodo string
	err := r.pool.QueryRow(ctx,
		`SELECT asociacion_id::text, periodo FROM cxc_planilla WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, planillaID).Scan(&asociacionID, &periodo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrPlanillaNoEncontrada
	}
	if err != nil {
		return "", "", fmt.Errorf("cxc: datos de planilla: %w", err)
	}
	return asociacionID, periodo, nil
}
