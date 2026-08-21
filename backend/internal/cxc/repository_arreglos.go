package cxc

// Arreglos de pago: el plan que el cliente se compromete a cumplir sobre una deuda ya vencida.
//
// Cuatro decisiones de diseño, y la razón de cada una:
//
//  1. El arreglo NO reescribe los cargos. Los cargos vencidos siguen vencidos con su fecha
//     original, así que la mora, el tramo, el aging y la regla de los 18 meses no se borran por
//     firmar un arreglo. El arreglo es un plan ENCIMA de la deuda. Los cobros se siguen
//     aplicando FIFO: el motor de aplicación no cambió ni una línea.
//
//  2. El cumplimiento NO se guarda: se deriva de los cobros, igual que las promesas. Y se mide
//     ACUMULADO —«a hoy debía haber pagado ₡X, pagó ₡Y»—, no cuota por cuota, porque quien
//     adelanta la cuota 3 no tiene por qué aparecer en mora en la 2.
//
//  3. Solo un arreglo vivo por contrato (índice único parcial). Dos planes simultáneos sobre la
//     misma deuda no se pueden juzgar: ¿de cuál era la cuota que pagó?
//
//  4. «En mora» lo calcula el sistema (es un hecho). «Quebrado» lo declara una persona con
//     motivo (es una consecuencia), igual que la suspensión. Un arreglo quebrado manda el
//     contrato a cartera morosa.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// ArregloInput es un arreglo a pactar.
type ArregloInput struct {
	Contrato string
	// Monto es lo que se pacta pagar. Por omisión el servicio usa el vencido del contrato.
	Monto decimal.Decimal
	// Prima es el abono de entrada. Opcional: en cero no cambia nada.
	Prima      decimal.Decimal
	PrimaFecha string
	Plazo      int
	// PrimeraCuota es la fecha de la cuota 1. Las demás salen cada mes desde ahí.
	PrimeraCuota   string
	Observaciones  string
	MotivoAutoriza string
}

// CuotaArreglo es una cuota del plan. La 0 es la prima.
type CuotaArreglo struct {
	Numero   int    `json:"numero"`
	VenceEn  string `json:"vence_en"`
	Monto    string `json:"monto"`
	Vencida  bool   `json:"vencida"`
	Cubierta bool   `json:"cubierta"`
}

// Arreglo es un arreglo con su estado derivado.
type Arreglo struct {
	ID          string `json:"id"`
	Consecutivo string `json:"consecutivo"`
	Contrato    string `json:"contrato"`
	Cliente     string `json:"cliente"`
	Sede        string `json:"sede"`
	// Estado derivado: AL_DIA | EN_MORA | CUMPLIDO | QUEBRADO | ANULADO.
	Estado       string `json:"estado"`
	MontoArreglo string `json:"monto_arreglo"`
	Prima        string `json:"prima"`
	Plazo        int    `json:"plazo_cuotas"`
	EsExcepcion  bool   `json:"es_excepcion"`
	// Pagado: lo cobrado desde que se pactó. Es la vara con la que se juzga el arreglo.
	Pagado string `json:"pagado"`
	// EsperadoAHoy: la suma de las cuotas cuya fecha ya pasó (con la tolerancia).
	EsperadoAHoy string `json:"esperado_a_hoy"`
	// Atraso: cuánto le falta a hoy. Cero si va al día.
	Atraso string `json:"atraso"`
	// Falta: lo que resta del arreglo completo.
	Falta           string `json:"falta"`
	CuotasCubiertas int    `json:"cuotas_cubiertas"`
	ProximaCuota    string `json:"proxima_cuota"`
	ProximoMonto    string `json:"proximo_monto"`
	// La foto de la deuda al pactar.
	SaldoAlPactar     string `json:"saldo_al_pactar"`
	VencidoAlPactar   string `json:"vencido_al_pactar"`
	MesesMoraAlPactar string `json:"meses_mora_al_pactar"`

	Cuotas []CuotaArreglo `json:"cuotas"`

	Observaciones      string `json:"observaciones"`
	AutorizadoPor      string `json:"autorizado_por"`
	AutorizacionMotivo string `json:"autorizacion_motivo"`
	CreadoPor          string `json:"creado_por"`
	CreadoEn           string `json:"creado_en"`
	QuebradoPor        string `json:"quebrado_por"`
	QuebradoEn         string `json:"quebrado_en"`
	QuebrantoMotivo    string `json:"quebranto_motivo"`
	AnuladoPor         string `json:"anulado_por"`
	AnuladoEn          string `json:"anulado_en"`
	AnulacionMotivo    string `json:"anulacion_motivo"`
}

// ResumenArreglos mide la cartera bajo arreglo. La pregunta que contesta es la del supervisor:
// ¿cuánta plata depende de que estos planes se cumplan, y cuántos ya se rompieron?
type ResumenArreglos struct {
	Arreglos  int    `json:"arreglos"`
	Pactado   string `json:"pactado"`
	Pagado    string `json:"pagado"`
	AlDia     int    `json:"al_dia"`
	EnMora    int    `json:"en_mora"`
	Cumplidos int    `json:"cumplidos"`
	Quebrados int    `json:"quebrados"`
	Anulados  int    `json:"anulados"`
	// Excepciones: cuántos se pactaron fuera de los plazos estándar. Con autorización sin
	// tope, el acumulado ES el control.
	Excepciones int    `json:"excepciones"`
	AtrasoTotal string `json:"atraso_total"`
}

type ListaArreglos struct {
	Resumen ResumenArreglos `json:"resumen"`
	Items   []Arreglo       `json:"items"`
	Total   int             `json:"total"`
	// Plazos viaja con la lista para que la pantalla no ofrezca plazos que el servidor va a
	// rechazar. Lo llena el handler, que es quien conoce el rol.
	Plazos PlazosDeArreglo `json:"plazos"`
}

// FiltrosArreglos filtra el listado.
type FiltrosArreglos struct {
	Contrato string
	// Estado: AL_DIA | EN_MORA | CUMPLIDO | QUEBRADO | ANULADO | VIVOS.
	Estado          string
	SedeIDs         []string
	SoloExcepciones bool
	Page, PageSize  int
}

var (
	ErrArregloNoEncontrado  = errors.New("cxc: el arreglo de pago no existe en esta empresa")
	ErrArregloVigente       = errors.New("cxc: el contrato ya tiene un arreglo de pago vigente; hay que quebrarlo o anularlo antes de pactar otro")
	ErrArregloCerrado       = errors.New("cxc: el arreglo ya está quebrado o anulado")
	ErrPlazoInvalido        = errors.New("cxc: el plazo del arreglo tiene que ser al menos 1 cuota")
	ErrPlazoExcedeTope      = errors.New("cxc: el plazo excede el tope de cuotas configurado")
	ErrPlazoNoAutorizado    = errors.New("cxc: ese plazo no está entre los estándar y requiere autorización del supervisor de piso")
	ErrMontoArregloInvalido = errors.New("cxc: el monto del arreglo tiene que ser mayor que cero")
	ErrPrimaExcedeMonto     = errors.New("cxc: la prima no puede ser mayor o igual que el monto del arreglo")
	ErrSinVencido           = errors.New("cxc: el contrato no tiene nada vencido: un arreglo de pago se hace sobre deuda vencida")
	ErrFechaArregloInvalida = errors.New("cxc: la fecha de la primera cuota no es válida (YYYY-MM-DD)")
)

// sqlPagadoDesdeArreglo suma lo cobrado desde que se pactó el arreglo. Va dentro de un CASE
// para que Postgres NO la evalúe en los contratos sin arreglo (la enorme mayoría): el mismo
// truco medido que se usa para las promesas.
const sqlPagadoDesdeArreglo = `(CASE WHEN %[1]s.id IS NULL THEN NULL ELSE
	COALESCE((SELECT sum(co.monto) FROM cobro_cxc co
		WHERE co.contrato_id = %[2]s AND co.estado <> 'REVERSADO'
		  AND COALESCE(co.fecha_bancaria, co.fecha_pago) >= %[1]s.creado_en::date
	), 0) END)`

// PactarArreglo escribe el arreglo y su plan de cuotas en una transacción.
func (r *pgRepository) PactarArreglo(ctx context.Context, empresaID string, in ArregloInput, esExcepcion bool, usuarioID string, topeMeses int) (Arreglo, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Arreglo{}, fmt.Errorf("cxc: begin arreglo: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	hoy := "(now() AT TIME ZONE 'America/Costa_Rica')::date"
	cuotasSQL := replaceHoy(fmt.Sprintf(sqlCuotasVencidas, "c.id"), hoy)
	porCuota := fmt.Sprintf(sqlMesesPorCuota, "mo")

	var contratoID string
	var saldo, vencido, meses decimal.Decimal
	var cuotasVenc int
	err = tx.QueryRow(ctx, `
		SELECT c.id::text,
		       COALESCE((SELECT sum(g.monto - g.monto_aplicado) FROM cargo_cxc g
		        WHERE g.contrato_id = c.id AND g.estado IN ('ABIERTO','PARCIAL')), 0),
		       COALESCE((SELECT sum(g.monto - g.monto_aplicado) FROM cargo_cxc g
		        WHERE g.contrato_id = c.id AND g.estado IN ('ABIERTO','PARCIAL')
		          AND g.vence_en < `+hoy+`), 0),
		       `+cuotasSQL+`, (`+cuotasSQL+` * `+porCuota+`)
		FROM contrato_cxc c
		LEFT JOIN cxc_modalidad mo ON mo.id = c.modalidad_id
		WHERE c.empresa_id = $1::uuid AND c.numero = $2
		FOR UPDATE OF c`,
		empresaID, in.Contrato).Scan(&contratoID, &saldo, &vencido, &cuotasVenc, &meses)
	if errors.Is(err, pgx.ErrNoRows) {
		return Arreglo{}, ErrContratoNoEncontrado
	}
	if err != nil {
		return Arreglo{}, fmt.Errorf("cxc: medir la deuda para el arreglo: %w", err)
	}
	// Un arreglo de pago se hace sobre deuda VENCIDA. Sin nada vencido no hay nada que
	// reprogramar, y el contrato pertenece a la lista preventiva, no a esta.
	if vencido.Sign() <= 0 {
		return Arreglo{}, ErrSinVencido
	}

	monto := in.Monto
	if monto.Sign() <= 0 {
		monto = vencido
	}
	if in.Prima.Sign() > 0 && in.Prima.GreaterThanOrEqual(monto) {
		return Arreglo{}, ErrPrimaExcedeMonto
	}

	// Un solo arreglo vivo por contrato. Se chequea explícito para dar un error entendible en
	// vez de dejar que reviente el índice único.
	var vivos int
	if err := tx.QueryRow(ctx,
		`SELECT count(*)::int FROM arreglo_pago_cxc
		 WHERE contrato_id = $1::uuid AND quebrado_en IS NULL AND anulado_en IS NULL`,
		contratoID).Scan(&vivos); err != nil {
		return Arreglo{}, fmt.Errorf("cxc: buscar arreglo vigente: %w", err)
	}
	if vivos > 0 {
		return Arreglo{}, ErrArregloVigente
	}

	// Consecutivo por empresa y sin huecos, bajo advisory lock. Un arreglo es un compromiso
	// que se le entrega firmado al cliente: la serie tiene que poder citarse.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('arreglo_cxc:' || $1))`, empresaID); err != nil {
		return Arreglo{}, fmt.Errorf("cxc: lock del consecutivo: %w", err)
	}
	var consecutivo int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(max(consecutivo), 0) + 1 FROM arreglo_pago_cxc WHERE empresa_id = $1::uuid`,
		empresaID).Scan(&consecutivo); err != nil {
		return Arreglo{}, fmt.Errorf("cxc: consecutivo del arreglo: %w", err)
	}

	var arregloID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO arreglo_pago_cxc (empresa_id, contrato_id, consecutivo,
		    saldo_al_pactar, vencido_al_pactar, cuotas_vencidas_al_pactar, meses_mora_al_pactar,
		    monto_arreglo, plazo_cuotas, prima, es_excepcion, autorizado_por, autorizacion_motivo,
		    observaciones, creado_por)
		VALUES ($1::uuid, $2::uuid, $3, $4::numeric, $5::numeric, $6, $7::numeric,
		        $8::numeric, $9, $10::numeric, $11,
		        CASE WHEN $11 THEN NULLIF($12,'')::uuid ELSE NULL END, $13,
		        $14, NULLIF($12,'')::uuid)
		RETURNING id::text`,
		empresaID, contratoID, consecutivo, saldo.String(), vencido.String(), cuotasVenc,
		meses.String(), monto.String(), in.Plazo, in.Prima.String(), esExcepcion,
		usuarioID, in.MotivoAutoriza, in.Observaciones).Scan(&arregloID); err != nil {
		return Arreglo{}, fmt.Errorf("cxc: insertar arreglo: %w", err)
	}

	for _, q := range planDeCuotas(monto, in.Prima, in.Plazo, in.PrimaFecha, in.PrimeraCuota) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO arreglo_cuota_cxc (empresa_id, arreglo_id, numero, vence_en, monto)
			VALUES ($1::uuid, $2::uuid, $3, $4::date, $5::numeric)`,
			empresaID, arregloID, q.Numero, q.VenceEn, q.Monto); err != nil {
			return Arreglo{}, fmt.Errorf("cxc: insertar cuota %d del arreglo: %w", q.Numero, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Arreglo{}, fmt.Errorf("cxc: commit arreglo: %w", err)
	}
	return r.ArregloPorID(ctx, empresaID, arregloID, 0)
}

// planDeCuotas reparte el monto en cuotas iguales y le deja el redondeo a la ÚLTIMA, para que
// la suma del plan sea exactamente el monto pactado. Si sobrara un colón, el arreglo nunca
// podría cumplirse (o se cumpliría de más), y eso es justo lo que se le entrega firmado al
// cliente.
func planDeCuotas(monto, prima decimal.Decimal, plazo int, primaFecha, primera string) []CuotaArreglo {
	out := make([]CuotaArreglo, 0, plazo+1)
	base := hoyCR()
	if d, err := time.Parse("2006-01-02", primera); err == nil {
		base = d
	}
	if prima.Sign() > 0 {
		f := primaFecha
		if f == "" {
			f = hoyCR().Format("2006-01-02")
		}
		out = append(out, CuotaArreglo{Numero: 0, VenceEn: f, Monto: prima.StringFixed(2)})
	}
	resto := monto.Sub(prima)
	if plazo < 1 {
		plazo = 1
	}
	cuota := resto.Div(decimal.NewFromInt(int64(plazo))).Round(2)
	acum := decimal.Zero
	for i := 1; i <= plazo; i++ {
		m := cuota
		if i == plazo {
			m = resto.Sub(acum)
		}
		acum = acum.Add(m)
		if m.Sign() <= 0 {
			// Un plan con más cuotas que colones: se corta acá en vez de escribir cuotas de 0
			// (que el CHECK de la tabla rechazaría).
			break
		}
		out = append(out, CuotaArreglo{
			Numero:  i,
			VenceEn: base.AddDate(0, i-1, 0).Format("2006-01-02"),
			Monto:   m.StringFixed(2),
		})
	}
	return out
}

// arregloSelect es el cuerpo compartido entre el listado y la ficha, para que las dos deriven
// el estado igual.
const arregloSelect = `
	SELECT ar.id::text, ar.consecutivo, c.numero, c.cliente_nombre, COALESCE(sd.nombre,''),
	       ar.monto_arreglo, ar.prima, ar.plazo_cuotas, ar.es_excepcion,
	       ar.saldo_al_pactar, ar.vencido_al_pactar, ar.meses_mora_al_pactar,
	       COALESCE(PAGADO, 0),
	       COALESCE(esp.esperado, 0),
	       ar.observaciones, COALESCE(ua.nombre,''), ar.autorizacion_motivo,
	       COALESCE(uc.nombre,''),
	       to_char(ar.creado_en AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD HH24:MI'),
	       COALESCE(uq.nombre,''),
	       to_char(ar.quebrado_en AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD HH24:MI'),
	       ar.quebranto_motivo,
	       COALESCE(un.nombre,''),
	       to_char(ar.anulado_en AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD HH24:MI'),
	       ar.anulacion_motivo,
	       (ar.quebrado_en IS NOT NULL), (ar.anulado_en IS NOT NULL)
	FROM arreglo_pago_cxc ar
	JOIN contrato_cxc c ON c.id = ar.contrato_id
	LEFT JOIN cxc_sede sd ON sd.id = c.sede_id
	LEFT JOIN usuario ua ON ua.id = ar.autorizado_por
	LEFT JOIN usuario uc ON uc.id = ar.creado_por
	LEFT JOIN usuario uq ON uq.id = ar.quebrado_por
	LEFT JOIN usuario un ON un.id = ar.anulado_por
	LEFT JOIN (
		SELECT q.arreglo_id,
		       sum(q.monto) FILTER (WHERE (q.vence_en + ($TOL || ' days')::interval)::date < $HOY) AS esperado
		FROM arreglo_cuota_cxc q
		WHERE q.empresa_id = $1::uuid
		GROUP BY q.arreglo_id
	) esp ON esp.arreglo_id = ar.id`

func (r *pgRepository) arregloQuery(tol int) string {
	hoy := "(now() AT TIME ZONE 'America/Costa_Rica')::date"
	q := strings.ReplaceAll(arregloSelect, "PAGADO", fmt.Sprintf(sqlPagadoDesdeArreglo, "ar", "c.id"))
	q = strings.ReplaceAll(q, "$TOL", fmt.Sprint(tol))
	return strings.ReplaceAll(q, "$HOY", hoy)
}

func (r *pgRepository) scanArreglo(row pgx.Row) (Arreglo, error) {
	var a Arreglo
	var monto, prima, saldoP, vencP, mesesP, pagado, esperado decimal.Decimal
	var quebEn, anulEn *string
	var quebrado, anulado bool
	err := row.Scan(&a.ID, &a.Consecutivo, &a.Contrato, &a.Cliente, &a.Sede,
		&monto, &prima, &a.Plazo, &a.EsExcepcion,
		&saldoP, &vencP, &mesesP, &pagado, &esperado,
		&a.Observaciones, &a.AutorizadoPor, &a.AutorizacionMotivo,
		&a.CreadoPor, &a.CreadoEn,
		&a.QuebradoPor, &quebEn, &a.QuebrantoMotivo,
		&a.AnuladoPor, &anulEn, &a.AnulacionMotivo,
		&quebrado, &anulado)
	if err != nil {
		return Arreglo{}, err
	}
	a.MontoArreglo, a.Prima = monto.String(), prima.String()
	a.SaldoAlPactar, a.VencidoAlPactar, a.MesesMoraAlPactar = saldoP.String(), vencP.String(), mesesP.String()
	a.Pagado, a.EsperadoAHoy = pagado.String(), esperado.String()
	a.QuebradoEn, a.AnuladoEn = valorOVacio(quebEn), valorOVacio(anulEn)

	// El atraso es la diferencia acumulada, nunca negativa: quien va adelantado no tiene
	// atraso «negativo», va al día.
	atraso := esperado.Sub(pagado)
	if atraso.Sign() < 0 {
		atraso = decimal.Zero
	}
	a.Atraso = atraso.String()
	falta := monto.Sub(pagado)
	if falta.Sign() < 0 {
		falta = decimal.Zero
	}
	a.Falta = falta.String()

	switch {
	case anulado:
		a.Estado = "ANULADO"
	case quebrado:
		a.Estado = "QUEBRADO"
	case pagado.GreaterThanOrEqual(monto):
		a.Estado = "CUMPLIDO"
	case atraso.Sign() > 0:
		a.Estado = "EN_MORA"
	default:
		a.Estado = "AL_DIA"
	}
	return a, nil
}

// cuotasDelArreglo trae el plan y marca, con el acumulado pagado, hasta dónde llegó.
func (r *pgRepository) cuotasDelArreglo(ctx context.Context, empresaID, arregloID string, pagado decimal.Decimal, tol int) ([]CuotaArreglo, int, string, string, error) {
	hoy := "(now() AT TIME ZONE 'America/Costa_Rica')::date"
	rows, err := r.pool.Query(ctx, `
		SELECT q.numero, q.vence_en, q.monto,
		       ((q.vence_en + ($3 || ' days')::interval)::date < `+hoy+`)
		FROM arreglo_cuota_cxc q
		WHERE q.empresa_id = $1::uuid AND q.arreglo_id = $2::uuid
		ORDER BY q.numero`, empresaID, arregloID, fmt.Sprint(tol))
	if err != nil {
		return nil, 0, "", "", fmt.Errorf("cxc: cuotas del arreglo: %w", err)
	}
	defer rows.Close()

	out := []CuotaArreglo{}
	restante := pagado
	cubiertas := 0
	proxFecha, proxMonto := "", ""
	for rows.Next() {
		var q CuotaArreglo
		var vence time.Time
		var monto decimal.Decimal
		if err := rows.Scan(&q.Numero, &vence, &monto, &q.Vencida); err != nil {
			return nil, 0, "", "", fmt.Errorf("cxc: scan cuota de arreglo: %w", err)
		}
		q.VenceEn, q.Monto = vence.Format("2006-01-02"), monto.String()
		// El pago se consume en orden: la cuota está cubierta si lo que quedaba del acumulado
		// alcanzaba para pagarla completa.
		if restante.GreaterThanOrEqual(monto) {
			q.Cubierta = true
			cubiertas++
			restante = restante.Sub(monto)
		} else {
			restante = decimal.Zero
			if proxFecha == "" {
				proxFecha, proxMonto = q.VenceEn, q.Monto
			}
		}
		out = append(out, q)
	}
	return out, cubiertas, proxFecha, proxMonto, rows.Err()
}

// ArregloPorID trae un arreglo con su plan.
func (r *pgRepository) ArregloPorID(ctx context.Context, empresaID, id string, tol int) (Arreglo, error) {
	a, err := r.scanArreglo(r.pool.QueryRow(ctx,
		r.arregloQuery(tol)+` WHERE ar.empresa_id = $1::uuid AND ar.id = $2::uuid`, empresaID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Arreglo{}, ErrArregloNoEncontrado
	}
	if err != nil {
		return Arreglo{}, fmt.Errorf("cxc: arreglo por id: %w", err)
	}
	pagado, _ := decimal.NewFromString(a.Pagado)
	cuotas, cubiertas, pf, pm, err := r.cuotasDelArreglo(ctx, empresaID, a.ID, pagado, tol)
	if err != nil {
		return Arreglo{}, err
	}
	a.Cuotas, a.CuotasCubiertas, a.ProximaCuota, a.ProximoMonto = cuotas, cubiertas, pf, pm
	return a, nil
}

// ListarArreglos lista con su resumen. El estado se filtra en Go y no en SQL porque es
// derivado de dos sumas: replicarlo como condición SQL sería una segunda definición del
// mismo criterio, y tarde o temprano las dos dejarían de coincidir.
func (r *pgRepository) ListarArreglos(ctx context.Context, empresaID string, f FiltrosArreglos, tol int) (ListaArreglos, error) {
	conds := []string{"ar.empresa_id = $1::uuid"}
	args := []any{empresaID}
	add := func(v any) int { args = append(args, v); return len(args) }

	if f.Contrato != "" {
		conds = append(conds, fmt.Sprintf("c.numero = $%d", add(f.Contrato)))
	}
	if f.SedeIDs != nil {
		if len(f.SedeIDs) == 0 {
			conds = append(conds, "false")
		} else {
			conds = append(conds, fmt.Sprintf("c.sede_id = ANY($%d::uuid[])", add(f.SedeIDs)))
		}
	}
	if f.SoloExcepciones {
		conds = append(conds, "ar.es_excepcion = true")
	}

	rows, err := r.pool.Query(ctx,
		r.arregloQuery(tol)+" WHERE "+strings.Join(conds, " AND ")+" ORDER BY ar.creado_en DESC", args...)
	if err != nil {
		return ListaArreglos{}, fmt.Errorf("cxc: listar arreglos: %w", err)
	}
	defer rows.Close()

	todos := []Arreglo{}
	for rows.Next() {
		a, err := r.scanArreglo(rows)
		if err != nil {
			return ListaArreglos{}, fmt.Errorf("cxc: scan arreglo: %w", err)
		}
		todos = append(todos, a)
	}
	if err := rows.Err(); err != nil {
		return ListaArreglos{}, fmt.Errorf("cxc: leer arreglos: %w", err)
	}

	// El resumen mide TODO lo filtrado, antes del filtro por estado: es la misma regla que en
	// el resto del ERP —el encabezado mide lo que el usuario está viendo—, y el filtro por
	// estado se aplica después para que los contadores sigan sirviendo de navegación.
	var res ResumenArreglos
	pactado, pagadoT, atraso := decimal.Zero, decimal.Zero, decimal.Zero
	for _, a := range todos {
		res.Arreglos++
		m, _ := decimal.NewFromString(a.MontoArreglo)
		p, _ := decimal.NewFromString(a.Pagado)
		at, _ := decimal.NewFromString(a.Atraso)
		pactado, pagadoT, atraso = pactado.Add(m), pagadoT.Add(p), atraso.Add(at)
		if a.EsExcepcion {
			res.Excepciones++
		}
		switch a.Estado {
		case "AL_DIA":
			res.AlDia++
		case "EN_MORA":
			res.EnMora++
		case "CUMPLIDO":
			res.Cumplidos++
		case "QUEBRADO":
			res.Quebrados++
		case "ANULADO":
			res.Anulados++
		}
	}
	res.Pactado, res.Pagado, res.AtrasoTotal = pactado.String(), pagadoT.String(), atraso.String()

	filtrados := todos
	if f.Estado != "" {
		filtrados = make([]Arreglo, 0, len(todos))
		for _, a := range todos {
			if a.Estado == f.Estado ||
				(f.Estado == "VIVOS" && (a.Estado == "AL_DIA" || a.Estado == "EN_MORA")) {
				filtrados = append(filtrados, a)
			}
		}
	}

	total := len(filtrados)
	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	desde := (page - 1) * pageSize
	if desde > total {
		desde = total
	}
	hasta := desde + pageSize
	if hasta > total {
		hasta = total
	}
	pagina := filtrados[desde:hasta]

	// El plan completo solo para la página que se devuelve: traerlo para todos sería una
	// consulta por arreglo sin que nadie lo vea.
	for i := range pagina {
		pagado, _ := decimal.NewFromString(pagina[i].Pagado)
		cuotas, cub, pf, pm, err := r.cuotasDelArreglo(ctx, empresaID, pagina[i].ID, pagado, tol)
		if err != nil {
			return ListaArreglos{}, err
		}
		pagina[i].Cuotas, pagina[i].CuotasCubiertas = cuotas, cub
		pagina[i].ProximaCuota, pagina[i].ProximoMonto = pf, pm
	}

	return ListaArreglos{Resumen: res, Items: pagina, Total: total}, nil
}

// CerrarArreglo quiebra o anula. La diferencia importa: ANULADO es «este arreglo no debió
// existir» (se pactó mal, el cliente no firmó) y no deja rastro de incumplimiento; QUEBRADO es
// «el cliente no cumplió» y manda el contrato a cartera morosa.
func (r *pgRepository) CerrarArreglo(ctx context.Context, empresaID, id, motivo, usuarioID string, quebrar bool, tol int) (Arreglo, error) {
	var cerrado bool
	err := r.pool.QueryRow(ctx,
		`SELECT (quebrado_en IS NOT NULL OR anulado_en IS NOT NULL)
		 FROM arreglo_pago_cxc WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, id).Scan(&cerrado)
	if errors.Is(err, pgx.ErrNoRows) {
		return Arreglo{}, ErrArregloNoEncontrado
	}
	if err != nil {
		return Arreglo{}, fmt.Errorf("cxc: buscar arreglo: %w", err)
	}
	if cerrado {
		return Arreglo{}, ErrArregloCerrado
	}

	sql := `UPDATE arreglo_pago_cxc
		SET anulado_en = now(), anulado_por = NULLIF($3,'')::uuid, anulacion_motivo = $4
		WHERE empresa_id = $1::uuid AND id = $2::uuid`
	if quebrar {
		sql = `UPDATE arreglo_pago_cxc
			SET quebrado_en = now(), quebrado_por = NULLIF($3,'')::uuid, quebranto_motivo = $4
			WHERE empresa_id = $1::uuid AND id = $2::uuid`
	}
	if _, err := r.pool.Exec(ctx, sql, empresaID, id, usuarioID, motivo); err != nil {
		return Arreglo{}, fmt.Errorf("cxc: cerrar arreglo: %w", err)
	}
	return r.ArregloPorID(ctx, empresaID, id, tol)
}
