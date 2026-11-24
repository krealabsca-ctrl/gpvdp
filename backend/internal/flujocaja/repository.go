package flujocaja

// Las consultas del flujo a 13 semanas.
//
// ── POR QUÉ ESTE PAQUETE CONSULTA CUATRO MÓDULOS ────────────────────────────────────────────
//
// Porque el saldo del banco no sale de ninguno solo: entra lo que cobra CxC, sale lo que paga
// CxP y lo que cuesta la planilla, y el punto de partida lo pone Bancos. Es el mismo criterio
// que `internal/grupo`: una VISTA que solo lee, en un paquete aparte para que se vea que cruza
// módulos y nadie meta por accidente una consulta así dentro de bancos.
//
// No tiene una sola escritura salvo la política de caja, que es su propia decisión y su propia
// tabla. Si este paquete se borrara, todo lo demás seguiría igual.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/cxp"
)

// esSinFilas distingue «no hay política» de «la consulta falló». Confundirlos haría que una
// empresa sin política vea un error en vez de su flujo.
func esSinFilas(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// Repository es lo que el servicio necesita de los datos.
type Repository interface {
	SaldoDePartida(ctx context.Context, empresaID, moneda string) (SaldoPartida, error)
	PagosProgramados(ctx context.Context, empresaID, moneda, desde, hasta string) ([]MovimientoEsperado, error)
	CarteraAtrasada(ctx context.Context, empresaID, moneda string) (decimal.Decimal, error)
	Planilla(ctx context.Context, empresaID, moneda, desde, hasta string) ([]MovimientoEsperado, error)
	CargosPorCobrar(ctx context.Context, empresaID, moneda, desde, hasta string) ([]MovimientoEsperado, error)
	CuotasProyectadas(ctx context.Context, empresaID, moneda, desde, hasta string, tasa decimal.Decimal) ([]MovimientoEsperado, int, error)
	TasaDeCobro(ctx context.Context, empresaID string) (decimal.Decimal, bool, error)

	Politica(ctx context.Context, empresaID, moneda string) (PoliticaCaja, error)
	GuardarPolitica(ctx context.Context, empresaID, usuarioID string, p PoliticaCaja) error
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository construye el repositorio sobre PostgreSQL.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

// hoyCR es el día calendario de Costa Rica. Tomar UTC correría el corte seis horas y, de noche,
// un día entero: la semana 1 arrancaría donde no es.
const hoyCR = `(now() AT TIME ZONE 'America/Costa_Rica')::date`

// ── El punto de partida ─────────────────────────────────────────────────────────────────────

// saldoDePartidaSQL calcula el saldo real de cada cuenta activa de la moneda.
//
// ── POR QUÉ NO ALCANZA CON SUMAR LOS MOVIMIENTOS ────────────────────────────────────────────
//
// Porque SUM(crédito − débito) NO es un saldo: es el NETO del período que los movimientos
// cubren. Las cargas de esta empresa arrancan en julio de 2026, así que esa suma da el neto de
// cuatro meses —medido: −₡27,9 millones— y eso se lee como «la cuenta está sobregirada», que es
// falso. Solo sería un saldo si los movimientos cubrieran la cuenta desde que se abrió.
//
// Lo que SÍ es un saldo es el que la tesorera declara contra el estado del banco. Y como puede
// ser de hace unos días, se le suman los movimientos POSTERIORES a esa fecha: eso lo trae al
// día sin inventar nada.
//
// Una cuenta que nunca tuvo un saldo declarado no tiene saldo conocido, y entra como tal —no
// como cero—: un cero silencioso en una cuenta con plata corre toda la curva hacia abajo.
const saldoDePartidaSQL = `
	WITH ultimo AS (
		SELECT DISTINCT ON (sd.cuenta_bancaria_id)
		       sd.cuenta_bancaria_id, sd.saldo, sd.fecha
		FROM saldo_cuenta_diario sd
		JOIN cuenta_bancaria cb ON cb.id = sd.cuenta_bancaria_id
		WHERE sd.empresa_id = $1::uuid AND cb.moneda = $2 AND cb.activo
		ORDER BY sd.cuenta_bancaria_id, sd.fecha DESC
	)
	SELECT COALESCE(SUM(u.saldo + COALESCE(post.neto, 0)), 0)::text,
	       COALESCE(MIN(u.fecha)::text, ''),
	       COUNT(u.cuenta_bancaria_id)::int,
	       (SELECT COUNT(*) FROM cuenta_bancaria cb2
	         WHERE cb2.empresa_id = $1::uuid AND cb2.moneda = $2 AND cb2.activo
	           AND cb2.id NOT IN (SELECT cuenta_bancaria_id FROM ultimo))::int
	FROM ultimo u
	LEFT JOIN LATERAL (
		SELECT SUM(m.credito - m.debito) AS neto
		FROM movimiento_bancario m
		WHERE m.cuenta_bancaria_id = u.cuenta_bancaria_id
		  AND m.incluido AND m.fecha > u.fecha
	) post ON true`

// cuentasDeLaMonedaSQL cuenta las cuentas activas, para distinguir «no hay cuentas» de «hay
// cuentas y ninguna tiene saldo».
const cuentasDeLaMonedaSQL = `
	SELECT COUNT(*)::int FROM cuenta_bancaria
	 WHERE empresa_id = $1::uuid AND moneda = $2 AND activo`

// Fuentes del saldo de partida.
const (
	// SaldoDeclarado: salió de un saldo que alguien comparó contra el banco.
	SaldoDeclarado = "DECLARADO"
	// SaldoSinDatos: ninguna cuenta tiene saldo declarado. La curva arranca en cero y lo dice.
	SaldoSinDatos = "SIN_DATOS"
)

// SaldoPartida es el punto donde arranca la curva, con lo que hace falta para juzgarlo.
type SaldoPartida struct {
	Monto           decimal.Decimal
	Fuente          string
	Fecha           string
	CuentasConSaldo int
	CuentasSinSaldo int
}

// SaldoDePartida devuelve el saldo real y cuántas cuentas quedaron sin medir.
func (r *pgRepository) SaldoDePartida(ctx context.Context, empresaID, moneda string) (SaldoPartida, error) {
	var monto, fecha string
	var conSaldo, sinSaldo int
	err := r.pool.QueryRow(ctx, saldoDePartidaSQL, empresaID, moneda).
		Scan(&monto, &fecha, &conSaldo, &sinSaldo)
	if err != nil {
		return SaldoPartida{}, fmt.Errorf("flujocaja: saldo de partida: %w", err)
	}
	if conSaldo == 0 {
		// Sin ningún saldo declarado no hay punto de partida. El total de cuentas sí se informa,
		// para que la pantalla pueda decir cuántas faltan capturar.
		var total int
		if err := r.pool.QueryRow(ctx, cuentasDeLaMonedaSQL, empresaID, moneda).Scan(&total); err != nil {
			return SaldoPartida{}, fmt.Errorf("flujocaja: contar cuentas: %w", err)
		}
		return SaldoPartida{Fuente: SaldoSinDatos, CuentasSinSaldo: total}, nil
	}
	d, err := decimal.NewFromString(monto)
	if err != nil {
		return SaldoPartida{}, fmt.Errorf("flujocaja: saldo de partida inválido: %w", err)
	}
	return SaldoPartida{
		Monto: d, Fuente: SaldoDeclarado, Fecha: fecha,
		CuentasConSaldo: conSaldo, CuentasSinSaldo: sinSaldo,
	}, nil
}

// ── Las salidas ─────────────────────────────────────────────────────────────────────────────

// pagosProgramadosSQL trae lo que alguien DECIDIÓ pagar en el rango.
//
// Por `fecha_pago_programada` y no por vencimiento: lo que sale del banco es lo que se programó.
// Proyectar por vencimiento con el 99,9 % de la cartera vencida pondría todo en la semana 1.
var pagosProgramadosSQL = `
	SELECT d.fecha_pago_programada::text,
	       CASE WHEN $2 = 'USD' THEN ROUND(` + cxp.NetoCRCSQL + ` / NULLIF(COALESCE(d.tc_aplicado, 1), 0), 2)
	            ELSE ` + cxp.NetoCRCSQL + ` END::text
	FROM documento_cxp d` + cxp.JoinsNetoSQL + `
	WHERE d.empresa_id = $1::uuid
	  AND d.moneda = $2
	  AND d.estado NOT IN ` + cxp.EstadosCerradosSQL + `
	  AND d.fecha_pago_programada BETWEEN $3::date AND $4::date`

// PagosProgramados lista las salidas comprometidas con proveedores.
func (r *pgRepository) PagosProgramados(ctx context.Context, empresaID, moneda, desde, hasta string) ([]MovimientoEsperado, error) {
	return r.movimientos(ctx, pagosProgramadosSQL, OrigenCxPProgramada, CapaComprometido, false,
		empresaID, moneda, desde, hasta)
}

// carteraAtrasadaSQL suma lo vencido que NO tiene fecha de pago programada.
//
// Lo que ya se programó se cuenta en su semana; contarlo acá también lo pagaría dos veces, y la
// curva mostraría una salida que no existe.
var carteraAtrasadaSQL = `
	SELECT COALESCE(SUM(
	       CASE WHEN $2 = 'USD' THEN ROUND(` + cxp.NetoCRCSQL + ` / NULLIF(COALESCE(d.tc_aplicado, 1), 0), 2)
	            ELSE ` + cxp.NetoCRCSQL + ` END), 0)::text
	FROM documento_cxp d` + cxp.JoinsNetoSQL + `
	WHERE d.empresa_id = $1::uuid
	  AND d.moneda = $2
	  AND d.estado NOT IN ` + cxp.EstadosCerradosSQL + `
	  AND d.fecha_pago_programada IS NULL
	  AND (d.fecha_vencimiento IS NULL OR d.fecha_vencimiento < ` + hoyCR + `)`

// CarteraAtrasada es la represa: lo vencido sin fecha de pago decidida.
func (r *pgRepository) CarteraAtrasada(ctx context.Context, empresaID, moneda string) (decimal.Decimal, error) {
	var s string
	if err := r.pool.QueryRow(ctx, carteraAtrasadaSQL, empresaID, moneda).Scan(&s); err != nil {
		return decimal.Zero, fmt.Errorf("flujocaja: cartera atrasada: %w", err)
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, fmt.Errorf("flujocaja: cartera atrasada inválida: %w", err)
	}
	return d, nil
}

// planillaSQL trae las corridas de nómina con fecha de pago en el rango.
//
// NETO + PATRONAL: del banco sale lo que recibe el empleado y además lo que se entera a la CCSS
// y demás instituciones. Proyectar solo el neto subestimaría la salida en más de un cuarto.
//
// Las corridas en BORRADOR no entran: todavía no son un compromiso.
const planillaSQL = `
	SELECT cn.fecha_pago::text, (cn.total_neto + cn.total_patronal)::text
	FROM corrida_nomina cn
	WHERE cn.empresa_id = $1::uuid
	  AND $2 = 'CRC'
	  AND cn.estado <> 'BORRADOR'
	  AND cn.fecha_pago BETWEEN $3::date AND $4::date`

// Planilla lista las salidas comprometidas de nómina. Solo en colones: la planilla costarricense
// se paga en colones y no hay corridas en otra moneda.
func (r *pgRepository) Planilla(ctx context.Context, empresaID, moneda, desde, hasta string) ([]MovimientoEsperado, error) {
	return r.movimientos(ctx, planillaSQL, OrigenPlanilla, CapaComprometido, false,
		empresaID, moneda, desde, hasta)
}

// ── Las entradas ────────────────────────────────────────────────────────────────────────────

// cargosPorCobrarSQL trae los cargos de CxC ya generados que vencen en el rango.
//
// Usa el índice parcial `idx_cargo_cxc_abierto (empresa_id, vence_en) WHERE estado IN (...)`,
// que ya existía. El saldo es monto − aplicado: cobrar de nuevo lo ya aplicado sería contar dos
// veces la misma plata.
const cargosPorCobrarSQL = `
	SELECT cc.vence_en::text, (cc.monto - cc.monto_aplicado)::text
	FROM cargo_cxc cc
	WHERE cc.empresa_id = $1::uuid
	  AND $2 = 'CRC'
	  AND cc.estado IN ('ABIERTO', 'PARCIAL')
	  AND cc.vence_en BETWEEN $3::date AND $4::date
	  AND cc.monto > cc.monto_aplicado`

// CargosPorCobrar lista las entradas comprometidas de cartera.
func (r *pgRepository) CargosPorCobrar(ctx context.Context, empresaID, moneda, desde, hasta string) ([]MovimientoEsperado, error) {
	return r.movimientos(ctx, cargosPorCobrarSQL, OrigenCxCCargo, CapaComprometido, true,
		empresaID, moneda, desde, hasta)
}

// cuotasProyectadasSQL proyecta la cuota de los contratos activos en su día de pago.
//
// ── POR QUÉ NO ALCANZA CON LOS CARGOS YA GENERADOS ──────────────────────────────────────────
//
// Porque los cargos se GENERAN por período: los de las próximas semanas todavía no existen como
// registro. Proyectar solo lo generado dejaría las semanas futuras en cero y haría creer que no
// va a entrar nada.
//
// Se excluyen los contratos que YA tienen cargo generado para ese período, o la misma cuota se
// contaría dos veces: una como cargo comprometido y otra como cuota esperada.
//
// `dia_pago` puede exceder los días del mes (un 31 en febrero): LEAST contra el último día del
// mes lo corrige, que es como lo cobra cualquier domiciliación.
const cuotasProyectadasSQL = `
	WITH meses AS (
		SELECT generate_series(
			date_trunc('month', $3::date),
			date_trunc('month', $4::date),
			interval '1 month')::date AS mes
	),
	activos AS (
		SELECT c.id, c.cuota_vigente, COALESCE(c.dia_pago, 1) AS dia_pago
		FROM contrato_cxc c
		WHERE c.empresa_id = $1::uuid AND c.estado = 'ACTIVO' AND c.cuota_vigente > 0
	)
	SELECT fecha::text, ROUND(SUM(cuota) * $5, 2)::text
	FROM (
		SELECT (m.mes + (LEAST(a.dia_pago,
		                       EXTRACT(DAY FROM (m.mes + interval '1 month - 1 day'))::int) - 1)
		                * interval '1 day')::date AS fecha,
		       a.cuota_vigente AS cuota,
		       a.id AS contrato_id,
		       to_char(m.mes, 'YYYY-MM') AS periodo
		FROM meses m CROSS JOIN activos a
	) q
	WHERE fecha BETWEEN $3::date AND $4::date
	  AND NOT EXISTS (
		SELECT 1 FROM cargo_cxc cc
		WHERE cc.contrato_id = q.contrato_id AND cc.periodo = q.periodo)
	  AND $2 = 'CRC'
	GROUP BY fecha`

// CuotasProyectadas estima lo que entraría de los contratos activos, ya castigado por la tasa de
// cobro. Devuelve además cuántos contratos activos la sostienen.
func (r *pgRepository) CuotasProyectadas(ctx context.Context, empresaID, moneda, desde, hasta string, tasa decimal.Decimal) ([]MovimientoEsperado, int, error) {
	rows, err := r.pool.Query(ctx, cuotasProyectadasSQL, empresaID, moneda, desde, hasta, tasa)
	if err != nil {
		return nil, 0, fmt.Errorf("flujocaja: cuotas proyectadas: %w", err)
	}
	defer rows.Close()

	out := make([]MovimientoEsperado, 0)
	for rows.Next() {
		m, err := escanearMovimiento(rows, OrigenCxCCuota, CapaEsperado, true)
		if err != nil {
			return nil, 0, err
		}
		if m != nil {
			out = append(out, *m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var activos int
	const qa = `SELECT count(*) FROM contrato_cxc
	             WHERE empresa_id = $1::uuid AND estado = 'ACTIVO' AND cuota_vigente > 0`
	if err := r.pool.QueryRow(ctx, qa, empresaID).Scan(&activos); err != nil {
		return nil, 0, fmt.Errorf("flujocaja: contar contratos activos: %w", err)
	}
	return out, activos, nil
}

// tasaDeCobroSQL mide qué proporción de lo facturado se cobró de verdad.
//
// ── POR QUÉ MEDIRLA Y NO SUPONER EL 100 % ───────────────────────────────────────────────────
//
// Suponer que todos pagan puntual es el error clásico de un forecast de caja: la curva se ve
// bien hasta la semana en que no alcanza. Se mide sobre cargos con al menos 60 días de vida
// —antes, un cargo sin cobrar no está impago, está en plazo— y sobre los últimos 12 meses.
const tasaDeCobroSQL = `
	SELECT COALESCE(SUM(cc.monto_aplicado), 0)::text, COALESCE(SUM(cc.monto), 0)::text
	FROM cargo_cxc cc
	WHERE cc.empresa_id = $1::uuid
	  AND cc.vence_en <= ` + hoyCR + ` - 60
	  AND cc.vence_en >= ` + hoyCR + ` - 425`

// TasaDeCobro devuelve la proporción histórica cobrada y si se pudo medir.
//
// Cuando no hay base suficiente devuelve 1 y medida=false: el servicio lo avisa en pantalla. Es
// preferible a inventar un castigo, pero hay que decirlo —un flujo al 100 % es optimista—.
func (r *pgRepository) TasaDeCobro(ctx context.Context, empresaID string) (decimal.Decimal, bool, error) {
	var cobrado, facturado string
	if err := r.pool.QueryRow(ctx, tasaDeCobroSQL, empresaID).Scan(&cobrado, &facturado); err != nil {
		return decimal.Zero, false, fmt.Errorf("flujocaja: tasa de cobro: %w", err)
	}
	fac, err := decimal.NewFromString(facturado)
	if err != nil || !fac.IsPositive() {
		return decimal.NewFromInt(1), false, nil
	}
	cob, err := decimal.NewFromString(cobrado)
	if err != nil {
		return decimal.NewFromInt(1), false, nil
	}
	tasa := cob.Div(fac).Round(4)
	// Una tasa mayor que 1 significa que se cobró más de lo facturado en la ventana —pagos
	// adelantados de cargos posteriores—. Para proyectar se acota a 1: cobrar el 112 % de lo que
	// se va a facturar no es una expectativa, es un artefacto de la ventana.
	if tasa.GreaterThan(decimal.NewFromInt(1)) {
		tasa = decimal.NewFromInt(1)
	}
	// UNA TASA DE CERO NO SE APLICA: multiplicaría por cero TODAS las cuotas proyectadas y el
	// flujo mostraría una caída libre sin que nada en pantalla diga por qué. Medido acá: 56.372
	// cargos por ₡425,9 millones con ₡0,00 aplicados. Una empresa que no cobra un colón en un
	// año no existe; lo que existe es un enlace de cobros que todavía no se alimenta. Se trata
	// como no medible —y el servicio lo avisa— en vez de borrar los ingresos en silencio.
	if !tasa.IsPositive() {
		return decimal.NewFromInt(1), false, nil
	}
	return tasa, true, nil
}

// ── La política ─────────────────────────────────────────────────────────────────────────────

// Politica lee la política vigente de esa moneda.
func (r *pgRepository) Politica(ctx context.Context, empresaID, moneda string) (PoliticaCaja, error) {
	const q = `SELECT modo, tope_semanal::text, tope_pct::text, minimo_operativo::text, moneda, nota
	             FROM politica_caja WHERE empresa_id = $1::uuid AND moneda = $2`
	var p PoliticaCaja
	err := r.pool.QueryRow(ctx, q, empresaID, moneda).Scan(
		&p.Modo, &p.TopeSemanal, &p.TopePct, &p.MinimoOperativo, &p.Moneda, &p.Nota)
	if err != nil {
		if esSinFilas(err) {
			// Sin política NO es un error: es el estado inicial. El flujo se arma igual y avisa
			// que la represa no baja.
			return PoliticaCaja{Moneda: moneda, Modo: TopeMontoFijo,
				TopeSemanal: "0", TopePct: "0", MinimoOperativo: "0"}, nil
		}
		return PoliticaCaja{}, fmt.Errorf("flujocaja: leer la política: %w", err)
	}
	p.Definida = true
	return p, nil
}

// GuardarPolitica reemplaza la política de esa moneda.
func (r *pgRepository) GuardarPolitica(ctx context.Context, empresaID, usuarioID string, p PoliticaCaja) error {
	const q = `
		INSERT INTO politica_caja
		  (empresa_id, modo, tope_semanal, tope_pct, minimo_operativo, moneda, nota, creado_por)
		VALUES ($1::uuid, $2, $3::numeric, $4::numeric, $5::numeric, $6, $7, NULLIF($8,'')::uuid)
		ON CONFLICT (empresa_id, moneda) DO UPDATE
		SET modo = EXCLUDED.modo, tope_semanal = EXCLUDED.tope_semanal,
		    tope_pct = EXCLUDED.tope_pct, minimo_operativo = EXCLUDED.minimo_operativo,
		    nota = EXCLUDED.nota, creado_por = EXCLUDED.creado_por, creado_en = now()`
	if _, err := r.pool.Exec(ctx, q, empresaID, p.Modo, p.TopeSemanal, p.TopePct,
		p.MinimoOperativo, p.Moneda, p.Nota, usuarioID); err != nil {
		return fmt.Errorf("flujocaja: guardar la política: %w", err)
	}
	return nil
}

// ── Lectura común ───────────────────────────────────────────────────────────────────────────

// filaMovimiento es lo que devuelven las consultas de movimientos: una fecha y un monto.
type filaMovimiento interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// movimientos corre una consulta de (fecha, monto) y la convierte en movimientos esperados.
func (r *pgRepository) movimientos(ctx context.Context, q, origen, capa string, entrada bool,
	empresaID, moneda, desde, hasta string) ([]MovimientoEsperado, error) {
	rows, err := r.pool.Query(ctx, q, empresaID, moneda, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("flujocaja: %s: %w", origen, err)
	}
	defer rows.Close()

	out := make([]MovimientoEsperado, 0)
	for rows.Next() {
		m, err := escanearMovimiento(rows, origen, capa, entrada)
		if err != nil {
			return nil, err
		}
		if m != nil {
			out = append(out, *m)
		}
	}
	return out, rows.Err()
}

// escanearMovimiento lee una fila (fecha, monto).
//
// Devuelve nil —en vez de un error— para una fila sin fecha o con monto cero: no son un fallo de
// la consulta, son filas que no aportan al flujo, y abortar por una de ellas dejaría la pantalla
// sin reporte por un dato irrelevante.
func escanearMovimiento(rows filaMovimiento, origen, capa string, entrada bool) (*MovimientoEsperado, error) {
	var fechaStr, montoStr string
	if err := rows.Scan(&fechaStr, &montoStr); err != nil {
		return nil, fmt.Errorf("flujocaja: scan de %s: %w", origen, err)
	}
	if fechaStr == "" {
		return nil, nil
	}
	fecha, err := time.Parse("2006-01-02", fechaStr)
	if err != nil {
		return nil, nil
	}
	monto, err := decimal.NewFromString(montoStr)
	if err != nil || !monto.IsPositive() {
		return nil, nil
	}
	return &MovimientoEsperado{
		Fecha: fecha, Monto: monto, Origen: origen, Capa: capa, Entrada: entrada,
	}, nil
}
