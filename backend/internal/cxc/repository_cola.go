package cxc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// FiltrosCola son los filtros del puesto de trabajo del operador.
type FiltrosCola struct {
	Q            string
	SedeID       string
	FormaPagoID  string
	AsociacionID string
	Tramo        string
	// SinGestionar: solo los que nadie tocó en los últimos N días (parámetro).
	SinGestionar bool
	// PromesaIncumplida: prometieron y no pagaron. Es la peor señal y la más gestionable.
	PromesaIncumplida bool
	// ParaSuspender: solo los que llegaron al tope de meses de mora acumulada.
	ParaSuspender bool
	// Morosa: cartera morosa — quebró su arreglo de pago o llegó al tope de meses.
	Morosa bool
	// Arreglo: AL_DIA | EN_MORA | CON | SIN.
	Arreglo string
	// TarjetaVencida: débito automático con tarjeta caducada. La plata no va a salir sola.
	TarjetaVencida bool
	// TarjetaPorVencer: caduca pronto (DIAS_ALERTA_TARJETA). Se gestiona distinto: se pide
	// la tarjeta nueva, no el pago.
	TarjetaPorVencer bool
	Orden            string
	Page, PageSize   int
	// SedeIDs lo inyecta el servicio según el permiso; el cliente no lo controla.
	SedeIDs []string
}

// FilaCola es un contrato en la cola de cobro, listo para trabajar.
type FilaCola struct {
	ContratoID string `json:"contrato_id"`
	Numero     string `json:"numero"`
	Cliente    string `json:"cliente"`
	Documento  string `json:"documento"`
	Telefonos  string `json:"telefonos"`
	Correos    string `json:"correos"`
	Sede       string `json:"sede"`
	FormaPago  string `json:"forma_pago"`
	Asociacion string `json:"asociacion"`
	Modalidad  string `json:"modalidad"`
	Saldo      string `json:"saldo"`
	// Vencido es la parte del saldo que YA se puede cobrar. Es la base del valor esperado.
	Vencido       string `json:"vencido"`
	Cargos        int    `json:"cargos_abiertos"`
	DiasMora      int    `json:"dias_mora"`
	Tramo         string `json:"tramo"`
	TramoEtiqueta string `json:"tramo_etiqueta"`
	Estrategia    string `json:"estrategia"`
	CanalSugerido string `json:"canal_sugerido"`
	// ValorEsperado: el criterio de orden. Saldo × probabilidad × factor.
	ValorEsperado string `json:"valor_esperado"`
	// Última gestión: para no llamar dos veces al mismo y saber qué se dijo.
	UltimaGestion   string `json:"ultima_gestion"`
	UltimoResultado string `json:"ultimo_resultado"`
	DiasSinGestion  *int   `json:"dias_sin_gestion"`
	Gestiones       int    `json:"gestiones"`
	// Promesa vigente o incumplida.
	PromesaFecha string `json:"promesa_fecha"`
	PromesaMonto string `json:"promesa_monto"`
	// PromesaIncumplida: la fecha pasó (con su tolerancia) y no entró el pago.
	PromesaIncumplida bool `json:"promesa_incumplida"`
	// PromesaVigente: prometió y la fecha todavía no llega. Es razón para NO llamar hoy.
	PromesaVigente bool `json:"promesa_vigente"`
	// CuotasVencidas: cuántas cuotas vencieron sin pagarse. Es el hecho concreto que el
	// operador le dice al cliente.
	CuotasVencidas int `json:"cuotas_vencidas"`
	// MesesMora: esas cuotas convertidas a meses según la modalidad. Es la medida que DECIDE,
	// porque la regla del negocio son 18 meses de mora «o su equivalencia»: 18 cuotas de un
	// quincenal son 9 meses, la mitad de lo que manda la regla.
	MesesMora     string `json:"meses_mora"`
	ParaSuspender bool   `json:"para_suspender"`
	// ArregloEstado: '' si no tiene arreglo, o AL_DIA | EN_MORA | CUMPLIDO | QUEBRADO |
	// ANULADO. Un arreglo AL_DÍA es razón para NO llamar hoy; uno EN_MORA es lo contrario.
	ArregloEstado string `json:"arreglo_estado"`
	// EnCarteraMorosa: quebró el arreglo o llegó al tope de meses de mora.
	EnCarteraMorosa bool `json:"en_cartera_morosa"`
	// Suspendido: ya se le cortó el servicio, pero sigue debiendo y sigue en la cola.
	Suspendido     bool   `json:"suspendido"`
	TarjetaVence   string `json:"tarjeta_vence"`
	TarjetaVencida bool   `json:"tarjeta_vencida"`
	// TarjetaPorVencer: domiciliado cuya tarjeta caduca dentro de DIAS_ALERTA_TARJETA.
	// Renovarla antes es más barato que cobrar el débito rechazado después.
	TarjetaPorVencer bool `json:"tarjeta_por_vencer"`
}

// Expresiones del estado de la promesa. Se comparten entre el resumen, los filtros y las
// filas para que las tres midan exactamente lo mismo.
//
// `cumplida` va dentro de un CASE y no de un AND por una razón de rendimiento medida:
// Postgres garantiza que las ramas de un CASE se evalúan de forma perezosa, así que la
// subconsulta sobre los cobros NO se corre para los contratos que no tienen promesa (la
// enorme mayoría). Con un AND no hay esa garantía.
const (
	sqlPromesaPasada   = "((prb.fecha_promesa + ($TOL || ' days')::interval)::date < $HOY)"
	sqlPromesaCumplida = `(CASE WHEN prb.fecha_promesa IS NULL THEN NULL ELSE
		COALESCE((
			SELECT sum(co.monto) FROM cobro_cxc co
			WHERE co.contrato_id = c.id AND co.estado <> 'REVERSADO'
			  -- LEAST porque la promesa se registra ANTES de su fecha; si un dato histórico
			  -- viniera al revés, la ventana no se invierte y queda vacía.
			  AND COALESCE(co.fecha_bancaria, co.fecha_pago)
			      BETWEEN LEAST(prb.creado_en::date, prb.fecha_promesa)
			      AND (prb.fecha_promesa + ($TOL || ' days')::interval)::date
		), 0) >= GREATEST(COALESCE(prb.monto, 0), 0.01) END)`
	// Con NULL (contrato sin promesa) las dos dan NULL, que se coalesce a false: el mismo
	// resultado que cuando esto era un LATERAL que no devolvía fila.
	sqlPromesaIncumplida = "(" + sqlPromesaPasada + " AND NOT " + sqlPromesaCumplida + ")"
	sqlPromesaVigente    = "(NOT " + sqlPromesaPasada + " AND NOT " + sqlPromesaCumplida + ")"
)

// Estado de la tarjeta de un domiciliado. `$ALERTA` es DIAS_ALERTA_TARJETA: el aviso previo
// convierte un parámetro que nadie leía en una señal accionable — renovar la tarjeta ANTES
// de que el débito falle es más barato que cobrar después.
const (
	sqlTarjetaVencida   = "(fp.es_domiciliado = true AND c.tarjeta_vence IS NOT NULL AND c.tarjeta_vence < $HOY)"
	sqlTarjetaPorVencer = "(fp.es_domiciliado = true AND c.tarjeta_vence IS NOT NULL AND c.tarjeta_vence >= $HOY" +
		" AND c.tarjeta_vence <= $HOY + $ALERTA)"
)

// ResumenCola son los totales del filtro activo.
type ResumenCola struct {
	Contratos            int    `json:"contratos"`
	Saldo                string `json:"saldo"`
	Vencido              string `json:"vencido"`
	ValorEsperado        string `json:"valor_esperado"`
	SinGestionar         int    `json:"sin_gestionar"`
	ConPromesaIncumplida int    `json:"con_promesa_incumplida"`
	ConPromesaVigente    int    `json:"con_promesa_vigente"`
	TarjetasVencidas     int    `json:"tarjetas_vencidas"`
	// ParaSuspender: cuántos llegaron al tope de meses de mora. Es la cola de decisión
	// del supervisor: ninguno se suspende solo.
	ParaSuspender     int `json:"para_suspender"`
	Suspendidos       int `json:"suspendidos"`
	TarjetasPorVencer int `json:"tarjetas_por_vencer"`
	// Arreglos: cuántos tienen un plan al día (no hay que llamarlos) y cuántos lo rompieron.
	ArregloAlDia  int `json:"arreglo_al_dia"`
	ArregloEnMora int `json:"arreglo_en_mora"`
	// CarteraMorosa: quebraron el arreglo o llegaron al tope de meses de mora.
	CarteraMorosa int `json:"cartera_morosa"`
	// El universo que la cola deja FUERA a propósito: contratos que deben pero cuya cuota
	// todavía no vence. Se reporta para que la exclusión sea visible y no un misterio.
	PorVencerContratos int    `json:"por_vencer_contratos"`
	PorVencerMonto     string `json:"por_vencer_monto"`
}

type ListaCola struct {
	Resumen  ResumenCola `json:"resumen"`
	Items    []FilaCola  `json:"items"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

// colaBase es el cuerpo de la consulta de la cola. Se comparte entre el resumen y la lista
// para que el encabezado y las filas midan lo mismo, como en el resto del ERP.
//
// El valor esperado se calcula EN SQL (mismo criterio que ValorEsperado en Go, que es la
// versión probada) porque es el ORDER BY: traerlo a Go obligaría a leer los 70 000
// contratos para ordenar 50.
const colaBase = `
	FROM contrato_cxc c
	JOIN (
		SELECT g.contrato_id,
		       count(*)::int AS cargos,
		       sum(g.monto - g.monto_aplicado) AS saldo,
		       sum(CASE WHEN g.vence_en < $HOY THEN g.monto - g.monto_aplicado ELSE 0 END) AS vencido,
		       min(g.vence_en) AS mas_viejo,
		       -- Las cuotas vencidas sin pagar salen de ESTE agregado y no de una subconsulta
		       -- correlacionada. Es el mismo criterio que la ficha del contrato, pero acá se
		       -- calcula en el escaneo que ya se estaba haciendo: medido con 70 000 contratos,
		       -- la versión con subconsulta costaba 2 700 ms porque se evaluaba cuatro veces por
		       -- fila (en la fila, en los meses, en el tope y en la cartera morosa). Así, cero
		       -- veces.
		       count(*) FILTER (WHERE g.vence_en < $HOY AND g.monto > g.monto_aplicado)::int AS cuotas_vencidas
		FROM cargo_cxc g
		WHERE g.empresa_id = $1::uuid AND g.estado IN ('ABIERTO','PARCIAL')
		GROUP BY g.contrato_id
		HAVING sum(g.monto - g.monto_aplicado) > 0
	) s ON s.contrato_id = c.id
	LEFT JOIN cxc_sede sd ON sd.id = c.sede_id
	LEFT JOIN cxc_modalidad m ON m.id = c.modalidad_id
	LEFT JOIN cxc_forma_pago fp ON fp.id = c.forma_pago_id
	LEFT JOIN cxc_asociacion a ON a.id = c.asociacion_id
	-- El tramo sale de los días de mora del cargo más viejo sin pagar.
	LEFT JOIN LATERAL (
		SELECT t.codigo, t.etiqueta, t.prob_recuperacion, t.estrategia, t.canal_sugerido
		FROM cxc_tramo t
		WHERE t.empresa_id = c.empresa_id
		  AND ($HOY - s.mas_viejo) BETWEEN t.dias_min AND t.dias_max
		LIMIT 1
	) t ON true
	-- Última gestión y conteo. Van como UNIONES a tablas chicas y no como LATERAL por
	-- contrato: medido con 70 000 contratos, el LATERAL costaba 200 ms porque se evaluaba
	-- 50 922 veces para devolver 50 filas. Filtradas por empresa a propósito — sin el
	-- filtro se agregaban las gestiones de TODAS las empresas.
	LEFT JOIN (
		SELECT DISTINCT ON (ge.contrato_id) ge.contrato_id, ge.gestionada_en, rg.etiqueta AS resultado
		FROM gestion_cxc ge
		JOIN cxc_resultado_gestion rg ON rg.id = ge.resultado_id
		WHERE ge.empresa_id = $1::uuid
		ORDER BY ge.contrato_id, ge.gestionada_en DESC
	) ug ON ug.contrato_id = c.id
	LEFT JOIN (
		SELECT contrato_id, count(*)::int AS n FROM gestion_cxc
		WHERE empresa_id = $1::uuid GROUP BY contrato_id
	) cg ON cg.contrato_id = c.id
	-- Promesa más reciente del contrato. Si se cumplió NO se guarda: se deriva de los
	-- cobros (ver sqlPromesaCumplida), para que una promesa pagada deje de aparecer como
	-- incumplida en el momento, sin depender de ningún job.
	LEFT JOIN (
		SELECT DISTINCT ON (pp.contrato_id) pp.contrato_id, pp.fecha_promesa, pp.monto, pp.creado_en
		FROM promesa_pago_cxc pp
		WHERE pp.empresa_id = $1::uuid
		ORDER BY pp.contrato_id, pp.fecha_promesa DESC
	) prb ON prb.contrato_id = c.id
	-- Arreglo de pago más reciente. El último gana: si el cliente quebró uno y pactó otro, el
	-- que manda es el nuevo. Sin arreglo, todas las señales quedan en NULL → false.
	LEFT JOIN (
		SELECT DISTINCT ON (ar.contrato_id) ar.contrato_id, ar.id, ar.creado_en,
		       ar.monto_arreglo, ar.quebrado_en, ar.anulado_en
		FROM arreglo_pago_cxc ar
		WHERE ar.empresa_id = $1::uuid
		ORDER BY ar.contrato_id, ar.creado_en DESC
	) arr ON arr.contrato_id = c.id
	LEFT JOIN (
		SELECT q.arreglo_id,
		       sum(q.monto) FILTER (WHERE (q.vence_en + ($TOL || ' days')::interval)::date < $HOY) AS esperado
		FROM arreglo_cuota_cxc q
		WHERE q.empresa_id = $1::uuid
		GROUP BY q.arreglo_id
	) arq ON arq.arreglo_id = arr.id`

func (r *pgRepository) ColaDeCobro(ctx context.Context, empresaID string, f FiltrosCola, p ParametrosCola) (ListaCola, error) {
	diasSinGestionar, tolPromesa := p.DiasSinGestionar, p.TolPromesa
	hoy := "(now() AT TIME ZONE 'America/Costa_Rica')::date"
	// Las mismas dos sustituciones para el cuerpo y para las expresiones de la promesa: si
	// solo se hicieran en el cuerpo, el resumen y los filtros compararían contra otra
	// tolerancia que las filas.
	rep := func(s string) string {
		s = strings.ReplaceAll(s, "$HOY", hoy)
		s = strings.ReplaceAll(s, "$TOL", fmt.Sprint(tolPromesa))
		return strings.ReplaceAll(s, "$ALERTA", fmt.Sprintf("'%d days'::interval", p.DiasAlertaTarjeta))
	}
	base := rep(colaBase)
	promIncumplida := rep(sqlPromesaIncumplida)
	promVigente := rep(sqlPromesaVigente)
	tarjetaVencida := rep(sqlTarjetaVencida)
	// Las cuotas vencidas ya vienen contadas en el agregado `s` con el MISMO criterio que la
	// ficha del contrato y la suspensión: los tres lugares cuentan igual, pero acá sin pagar
	// una subconsulta por fila.
	cuotas := "s.cuotas_vencidas"
	// La regla del negocio son MESES de mora, no cuotas: un quincenal con 18 cuotas vencidas
	// lleva 9 meses de atraso. La equivalencia sale del ciclo de la modalidad (join `m`).
	mesesMora := "(" + cuotas + " * " + fmt.Sprintf(sqlMesesPorCuota, "m") + ")"
	paraSuspender := "(" + mesesMora + " >= " + fmt.Sprint(p.MesesParaSuspender) + " AND c.estado = 'ACTIVO')"
	tarjetaPorVencer := rep(sqlTarjetaPorVencer)

	// ── Arreglo de pago ─────────────────────────────────────────────────────────
	// TODO el estado del arreglo sale de UNA sola expresión CASE, y no de varias condiciones
	// booleanas encadenadas, por la misma razón medida que se documentó para las promesas:
	// Postgres garantiza que las ramas de un CASE se evalúan de forma perezosa, así que la
	// subconsulta sobre los cobros no se corre para los contratos sin arreglo —la enorme
	// mayoría—. Con un AND no hay esa garantía y se pagaría en las 70 000 filas.
	arrPagado := "COALESCE((SELECT sum(co.monto) FROM cobro_cxc co" +
		" WHERE co.contrato_id = c.id AND co.estado <> 'REVERSADO'" +
		" AND COALESCE(co.fecha_bancaria, co.fecha_pago) >= arr.creado_en::date), 0)"
	arrEstado := `(CASE
		WHEN arr.id IS NULL THEN ''
		WHEN arr.anulado_en IS NOT NULL THEN 'ANULADO'
		WHEN arr.quebrado_en IS NOT NULL THEN 'QUEBRADO'
		WHEN ` + arrPagado + ` >= arr.monto_arreglo THEN 'CUMPLIDO'
		WHEN COALESCE(arq.esperado, 0) > ` + arrPagado + ` THEN 'EN_MORA'
		ELSE 'AL_DIA' END)`
	arrEnMora := "(" + arrEstado + " = 'EN_MORA')"
	arrAlDia := "(" + arrEstado + " = 'AL_DIA')"
	// CARTERA MOROSA, tal como la definió el negocio: «en el caso de incumplir aplica la regla
	// de los 18 meses, pero pasa a una cartera morosa». Es decir, se llega por dos caminos:
	// romper un arreglo, o llegar al tope de meses de mora. Va DERIVADA y no como bandera
	// guardada: así no puede quedar desactualizada nunca.
	morosa := "((arr.id IS NOT NULL AND arr.quebrado_en IS NOT NULL) OR " + paraSuspender + ")"

	// Los SUSPENDIDOS siguen en la cola: cortarles el servicio no borra la deuda, y son
	// justamente los casos que el catálogo manda a cobro judicial. La fila los marca.
	conds := []string{"c.empresa_id = $1::uuid", "c.estado IN ('ACTIVO','SUSPENDIDO')"}
	args := []any{empresaID}
	add := func(v any) int { args = append(args, v); return len(args) }

	// Un contrato en revisión NO entra a la cola: no se llama a nadie por un dato que ya
	// sabemos que está mal.
	conds = append(conds, "c.revision_pendiente = false")

	// La cola es lo VENCIDO. Un contrato cuya cuota todavía no vence no se cobra hoy, y el
	// propio catálogo lo dice: los tramos ADELANTADO y AL_DÍA traen canal sugerido
	// «Ninguno». Sin este filtro, un contrato que paga por adelantado (probabilidad 1,00)
	// se pondría delante de uno con 186 días de mora.
	conds = append(conds, "s.vencido > 0")

	if f.SedeIDs != nil {
		if len(f.SedeIDs) == 0 {
			conds = append(conds, "false")
		} else {
			conds = append(conds, fmt.Sprintf("c.sede_id = ANY($%d::uuid[])", add(f.SedeIDs)))
		}
	}
	if f.SedeID != "" {
		conds = append(conds, fmt.Sprintf("c.sede_id = $%d::uuid", add(f.SedeID)))
	}
	if f.FormaPagoID != "" {
		conds = append(conds, fmt.Sprintf("c.forma_pago_id = $%d::uuid", add(f.FormaPagoID)))
	}
	if f.AsociacionID != "" {
		conds = append(conds, fmt.Sprintf("c.asociacion_id = $%d::uuid", add(f.AsociacionID)))
	}
	if f.Tramo != "" {
		conds = append(conds, fmt.Sprintf("t.codigo = $%d", add(f.Tramo)))
	}
	if f.SinGestionar {
		conds = append(conds, fmt.Sprintf(
			"(ug.gestionada_en IS NULL OR ug.gestionada_en < now() - ($%d || ' days')::interval)",
			add(fmt.Sprint(diasSinGestionar))))
	}
	if f.PromesaIncumplida {
		conds = append(conds, promIncumplida)
	}
	if f.TarjetaVencida {
		conds = append(conds, tarjetaVencida)
	}
	if f.ParaSuspender {
		conds = append(conds, paraSuspender)
	}
	if f.TarjetaPorVencer {
		conds = append(conds, tarjetaPorVencer)
	}
	// Cartera morosa: quebró su arreglo o llegó al tope de meses. Es la lista que el negocio
	// pidió que se separara.
	if f.Morosa {
		conds = append(conds, morosa)
	}
	switch f.Arreglo {
	case "AL_DIA":
		conds = append(conds, arrAlDia)
	case "EN_MORA":
		conds = append(conds, arrEnMora)
	case "SIN":
		conds = append(conds, "arr.id IS NULL")
	case "CON":
		conds = append(conds, "arr.id IS NOT NULL")
	}
	if f.Q != "" {
		n := add("%" + f.Q + "%")
		conds = append(conds, fmt.Sprintf(
			"(c.numero ILIKE $%d OR c.documento ILIKE $%d OR c.cliente_nombre ILIKE $%d)", n, n, n))
	}
	where := strings.Join(conds, " AND ")

	// El valor esperado se calcula sobre lo VENCIDO, no sobre el saldo total: la cuota que
	// todavía no vence no se puede cobrar hoy, y meterla inflaría la expectativa del día.
	// Los mismos topes que la versión en Go: probabilidad y factor acotados, y el resultado
	// nunca mayor que el monto cobrable.
	const valorEsperado = `
		round(LEAST(
			s.vencido,
			s.vencido
			  * LEAST(GREATEST(COALESCE(t.prob_recuperacion, 0), 0), 1)
			  * LEAST(GREATEST(COALESCE(fp.factor_recuperacion, 1), 0.10), 2)
		), 2)`

	var res ResumenCola
	var saldo, vencido, valor decimal.Decimal
	err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int, COALESCE(sum(s.saldo),0), COALESCE(sum(s.vencido),0),
		       COALESCE(sum(`+valorEsperado+`),0),
		       count(*) FILTER (WHERE ug.gestionada_en IS NULL OR ug.gestionada_en < now() - ('`+fmt.Sprint(diasSinGestionar)+`' || ' days')::interval)::int,
		       count(*) FILTER (WHERE `+promIncumplida+`)::int,
		       count(*) FILTER (WHERE `+promVigente+`)::int,
		       count(*) FILTER (WHERE `+tarjetaVencida+`)::int,
		       count(*) FILTER (WHERE `+tarjetaPorVencer+`)::int,
		       count(*) FILTER (WHERE `+paraSuspender+`)::int,
		       count(*) FILTER (WHERE c.estado = 'SUSPENDIDO')::int,
		       count(*) FILTER (WHERE `+arrAlDia+`)::int,
		       count(*) FILTER (WHERE `+arrEnMora+`)::int,
		       count(*) FILTER (WHERE `+morosa+`)::int
		`+base+` WHERE `+where, args...).
		Scan(&res.Contratos, &saldo, &vencido, &valor, &res.SinGestionar,
			&res.ConPromesaIncumplida, &res.ConPromesaVigente, &res.TarjetasVencidas, &res.TarjetasPorVencer,
			&res.ParaSuspender, &res.Suspendidos,
			&res.ArregloAlDia, &res.ArregloEnMora, &res.CarteraMorosa)
	if err != nil {
		return ListaCola{}, fmt.Errorf("cxc: resumen de la cola: %w", err)
	}
	res.Saldo, res.Vencido, res.ValorEsperado = saldo.String(), vencido.String(), valor.String()

	// El universo por vencer, con el mismo alcance de sede: no entra a la cola, pero el
	// operador tiene que poder ver que existe.
	pvArgs := []any{empresaID}
	pvCond := ""
	if f.SedeIDs != nil {
		if len(f.SedeIDs) == 0 {
			pvCond = " AND false"
		} else {
			pvArgs = append(pvArgs, f.SedeIDs)
			pvCond = " AND c.sede_id = ANY($2::uuid[])"
		}
	}
	var pvMonto decimal.Decimal
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int, COALESCE(sum(s.saldo),0)
		FROM contrato_cxc c
		JOIN (
			SELECT g.contrato_id,
			       sum(g.monto - g.monto_aplicado) AS saldo,
			       sum(CASE WHEN g.vence_en < `+hoy+` THEN g.monto - g.monto_aplicado ELSE 0 END) AS vencido
			FROM cargo_cxc g
			WHERE g.empresa_id = $1::uuid AND g.estado IN ('ABIERTO','PARCIAL')
			GROUP BY g.contrato_id
		) s ON s.contrato_id = c.id
		WHERE c.empresa_id = $1::uuid AND c.estado = 'ACTIVO' AND c.revision_pendiente = false
		  AND s.saldo > 0 AND s.vencido = 0`+pvCond, pvArgs...).Scan(&res.PorVencerContratos, &pvMonto); err != nil {
		return ListaCola{}, fmt.Errorf("cxc: universo por vencer: %w", err)
	}
	res.PorVencerMonto = pvMonto.String()

	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	args = append(args, pageSize, (page-1)*pageSize)

	q := `
		SELECT c.id::text, c.numero, c.cliente_nombre, c.documento, c.telefonos, c.correos,
		       COALESCE(sd.nombre,''), COALESCE(fp.nombre,''), COALESCE(a.nombre,''), COALESCE(m.nombre,''),
		       s.saldo, s.vencido, s.cargos, (` + hoy + ` - s.mas_viejo),
		       COALESCE(t.codigo,''), COALESCE(t.etiqueta,''), COALESCE(t.estrategia,''), COALESCE(t.canal_sugerido,''),
		       ` + valorEsperado + `,
		       -- La fecha de la gestión se muestra en el día de Costa Rica: a las 7 p. m. de
		       -- un 4 en Costa Rica, en UTC ya es el 5, y la pantalla diría «mañana».
		       (ug.gestionada_en AT TIME ZONE 'America/Costa_Rica'),
		       (` + hoy + ` - (ug.gestionada_en AT TIME ZONE 'America/Costa_Rica')::date),
		       COALESCE(ug.resultado,''), COALESCE(cg.n,0),
		       prb.fecha_promesa, prb.monto,
		       COALESCE(` + promIncumplida + `, false), COALESCE(` + promVigente + `, false),
		       c.tarjeta_vence,
		       ` + cuotas + `, ` + mesesMora + `, COALESCE(` + paraSuspender + `, false), (c.estado = 'SUSPENDIDO'),
		       COALESCE(` + tarjetaVencida + `, false), COALESCE(` + tarjetaPorVencer + `, false),
		       ` + arrEstado + `, COALESCE(` + morosa + `, false)
		` + base + `
		WHERE ` + where + `
		ORDER BY ` + ordenCola(f.Orden, valorEsperado) + `
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return ListaCola{}, fmt.Errorf("cxc: cola de cobro: %w", err)
	}
	defer rows.Close()

	items := make([]FilaCola, 0, pageSize)
	for rows.Next() {
		var (
			fc                 FilaCola
			sal, ven, val      decimal.Decimal
			meses              decimal.Decimal
			ultima             *time.Time
			diasSin            *int
			promFecha, tarjeta *time.Time
			promMonto          *decimal.Decimal
		)
		if err := rows.Scan(&fc.ContratoID, &fc.Numero, &fc.Cliente, &fc.Documento, &fc.Telefonos, &fc.Correos,
			&fc.Sede, &fc.FormaPago, &fc.Asociacion, &fc.Modalidad,
			&sal, &ven, &fc.Cargos, &fc.DiasMora,
			&fc.Tramo, &fc.TramoEtiqueta, &fc.Estrategia, &fc.CanalSugerido,
			&val, &ultima, &diasSin, &fc.UltimoResultado, &fc.Gestiones,
			&promFecha, &promMonto, &fc.PromesaIncumplida, &fc.PromesaVigente,
			&tarjeta,
			&fc.CuotasVencidas, &meses, &fc.ParaSuspender, &fc.Suspendido,
			&fc.TarjetaVencida, &fc.TarjetaPorVencer,
			&fc.ArregloEstado, &fc.EnCarteraMorosa); err != nil {
			return ListaCola{}, fmt.Errorf("cxc: scan fila de cola: %w", err)
		}
		fc.Saldo, fc.Vencido, fc.ValorEsperado = sal.String(), ven.String(), val.String()
		fc.MesesMora = meses.String()
		if ultima != nil {
			fc.UltimaGestion = ultima.Format("2006-01-02")
			fc.DiasSinGestion = diasSin
		}
		fc.PromesaFecha = fechaOVacio(promFecha)
		if promMonto != nil {
			fc.PromesaMonto = promMonto.String()
		}
		fc.TarjetaVence = fechaOVacio(tarjeta)
		items = append(items, fc)
	}
	if err := rows.Err(); err != nil {
		return ListaCola{}, err
	}
	return ListaCola{Resumen: res, Items: items, Total: res.Contratos, Page: page, PageSize: pageSize}, nil
}

// ordenCola: por omisión el VALOR ESPERADO, que es el punto de la pantalla. Los otros
// órdenes existen porque a veces el supervisor quiere ver la cartera de otra manera.
func ordenCola(orden, valorEsperado string) string {
	switch orden {
	case "saldo":
		return "s.saldo DESC, c.numero"
	case "mora":
		return "s.mas_viejo ASC, c.numero"
	case "sin_gestion":
		// Los nunca gestionados primero, después los más olvidados.
		return "ug.gestionada_en ASC NULLS FIRST, " + valorEsperado + " DESC"
	default:
		return valorEsperado + " DESC, s.saldo DESC, c.numero"
	}
}

// ---- Gestión ----

// GestionInput es una gestión a registrar.
type GestionInput struct {
	Contrato    string
	CanalID     string
	ResultadoID string
	Notas       string
	// Promesa: solo si el resultado la exige.
	PromesaFecha string
	PromesaMonto string
}

// GestionRegistrada devuelve lo que quedó guardado, con la foto del contrato.
type GestionRegistrada struct {
	ID               string `json:"id"`
	Contrato         string `json:"contrato"`
	Resultado        string `json:"resultado"`
	EsContacto       bool   `json:"es_contacto"`
	SaldoAlGestionar string `json:"saldo_al_gestionar"`
	Tramo            string `json:"tramo_al_gestionar"`
	PromesaID        string `json:"promesa_id"`
}

var (
	ErrResultadoInvalido    = errors.New("cxc: el resultado de gestión no existe en esta empresa")
	ErrCanalInvalido        = errors.New("cxc: el canal de gestión no existe en esta empresa")
	ErrPromesaRequerida     = errors.New("cxc: ese resultado exige una fecha de promesa")
	ErrPromesaFechaInvalida = errors.New("cxc: la fecha prometida no tiene el formato AAAA-MM-DD")
	ErrPromesaEnElPasado    = errors.New("cxc: la fecha prometida no puede ser anterior a hoy")
	ErrPromesaMontoInvalido = errors.New("cxc: el monto prometido no es un número válido")
)

// RegistrarGestion guarda la gestión con la FOTO del estado del contrato en ese momento
// (saldo, días de mora, tramo). Se guarda porque «¿cuánto debía cuando lo llamamos?» no se
// puede reconstruir después: el saldo de hoy ya cambió.
func (r *pgRepository) RegistrarGestion(ctx context.Context, empresaID string, in GestionInput, usuarioID string) (GestionRegistrada, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return GestionRegistrada{}, fmt.Errorf("cxc: begin gestión: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var contratoID string
	err = tx.QueryRow(ctx,
		`SELECT id::text FROM contrato_cxc WHERE empresa_id = $1::uuid AND numero = $2`,
		empresaID, in.Contrato).Scan(&contratoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return GestionRegistrada{}, ErrContratoNoEncontrado
	}
	if err != nil {
		return GestionRegistrada{}, fmt.Errorf("cxc: buscar contrato: %w", err)
	}

	// El canal se valida antes de escribir: un id inventado tiene que dar 422 con su
	// motivo, no una violación de llave foránea traducida a «error interno».
	var canalOK bool
	err = tx.QueryRow(ctx,
		`SELECT true FROM cxc_canal_gestion WHERE empresa_id = $1::uuid AND id = $2::uuid AND activo = true`,
		empresaID, in.CanalID).Scan(&canalOK)
	if errors.Is(err, pgx.ErrNoRows) {
		return GestionRegistrada{}, ErrCanalInvalido
	}
	if err != nil {
		return GestionRegistrada{}, fmt.Errorf("cxc: leer canal: %w", err)
	}

	var (
		resultado          string
		esContacto, exigeP bool
	)
	err = tx.QueryRow(ctx, `
		SELECT etiqueta, es_contacto, exige_promesa FROM cxc_resultado_gestion
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND activo = true`,
		empresaID, in.ResultadoID).Scan(&resultado, &esContacto, &exigeP)
	if errors.Is(err, pgx.ErrNoRows) {
		return GestionRegistrada{}, ErrResultadoInvalido
	}
	if err != nil {
		return GestionRegistrada{}, fmt.Errorf("cxc: leer resultado: %w", err)
	}
	// Una «promesa de pago» sin fecha no es una promesa: es una nota. Se rechaza en vez de
	// guardar un compromiso que nadie puede evaluar después.
	if exigeP && in.PromesaFecha == "" {
		return GestionRegistrada{}, ErrPromesaRequerida
	}

	// Foto del estado actual, derivada de los cargos abiertos.
	var (
		saldo decimal.Decimal
		dias  int
		tramo string
	)
	err = tx.QueryRow(ctx, `
		WITH s AS (
			SELECT COALESCE(sum(monto - monto_aplicado), 0) AS saldo, min(vence_en) AS mas_viejo
			FROM cargo_cxc
			WHERE empresa_id = $1::uuid AND contrato_id = $2::uuid AND estado IN ('ABIERTO','PARCIAL')
		)
		SELECT s.saldo,
		       COALESCE((now() AT TIME ZONE 'America/Costa_Rica')::date - s.mas_viejo, 0),
		       COALESCE((SELECT codigo FROM cxc_tramo
		                 WHERE empresa_id = $1::uuid
		                   AND COALESCE((now() AT TIME ZONE 'America/Costa_Rica')::date - s.mas_viejo, 0)
		                       BETWEEN dias_min AND dias_max
		                 LIMIT 1), '')
		FROM s`, empresaID, contratoID).Scan(&saldo, &dias, &tramo)
	if err != nil {
		return GestionRegistrada{}, fmt.Errorf("cxc: foto del contrato: %w", err)
	}

	var gestionID string
	err = tx.QueryRow(ctx, `
		INSERT INTO gestion_cxc (empresa_id, contrato_id, usuario_id, canal_id, resultado_id, notas,
		                         saldo_al_gestionar, dias_mora_al_gestionar, tramo_al_gestionar)
		VALUES ($1::uuid, $2::uuid, NULLIF($3,'')::uuid, $4::uuid, $5::uuid, $6, $7::numeric, $8, $9)
		RETURNING id::text`,
		empresaID, contratoID, usuarioID, in.CanalID, in.ResultadoID, in.Notas,
		saldo.String(), dias, tramo).Scan(&gestionID)
	if err != nil {
		return GestionRegistrada{}, fmt.Errorf("cxc: insertar gestión: %w", err)
	}

	out := GestionRegistrada{
		ID: gestionID, Contrato: in.Contrato, Resultado: resultado, EsContacto: esContacto,
		SaldoAlGestionar: saldo.String(), Tramo: tramo,
	}
	if in.PromesaFecha != "" {
		err = tx.QueryRow(ctx, `
			INSERT INTO promesa_pago_cxc (empresa_id, gestion_id, contrato_id, fecha_promesa, monto)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::date, NULLIF($5,'')::numeric)
			RETURNING id::text`,
			empresaID, gestionID, contratoID, in.PromesaFecha, in.PromesaMonto).Scan(&out.PromesaID)
		if err != nil {
			return GestionRegistrada{}, fmt.Errorf("cxc: insertar promesa: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return GestionRegistrada{}, fmt.Errorf("cxc: commit gestión: %w", err)
	}
	return out, nil
}

// GestionFila es una gestión en el historial del contrato.
type GestionFila struct {
	ID               string `json:"id"`
	Fecha            string `json:"fecha"`
	Canal            string `json:"canal"`
	Resultado        string `json:"resultado"`
	EsContacto       bool   `json:"es_contacto"`
	Notas            string `json:"notas"`
	Usuario          string `json:"usuario"`
	SaldoEntonces    string `json:"saldo_entonces"`
	TramoEntonces    string `json:"tramo_entonces"`
	DiasMoraEntonces int    `json:"dias_mora_entonces"`
	PromesaFecha     string `json:"promesa_fecha"`
	PromesaMonto     string `json:"promesa_monto"`
	// PromesaCumplida es null mientras la fecha (más su tolerancia) no haya pasado: todavía
	// no se puede juzgar.
	PromesaCumplida *bool `json:"promesa_cumplida"`
}

func (r *pgRepository) GestionesDeContrato(ctx context.Context, empresaID, contratoID string, tolPromesa int) ([]GestionFila, error) {
	// El cumplimiento de cada promesa se deriva igual que en la cola, y queda NULL mientras
	// la fecha (más su tolerancia) no haya pasado: todavía no se puede juzgar.
	rows, err := r.pool.Query(ctx, `
		SELECT ge.id::text, (ge.gestionada_en AT TIME ZONE 'America/Costa_Rica'),
		       cn.nombre, rg.etiqueta, rg.es_contacto, ge.notas,
		       COALESCE(u.nombre,''), ge.saldo_al_gestionar, ge.tramo_al_gestionar, ge.dias_mora_al_gestionar,
		       pp.fecha_promesa, pp.monto,
		       CASE
		         WHEN pp.id IS NULL THEN NULL
		         WHEN COALESCE((
		           SELECT sum(co.monto) FROM cobro_cxc co
		           WHERE co.contrato_id = ge.contrato_id AND co.estado <> 'REVERSADO'
		             AND COALESCE(co.fecha_bancaria, co.fecha_pago) BETWEEN pp.creado_en::date
		                 AND (pp.fecha_promesa + ($3 || ' days')::interval)::date
		         ), 0) >= GREATEST(COALESCE(pp.monto, 0), 0.01) THEN true
		         WHEN (pp.fecha_promesa + ($3 || ' days')::interval)::date
		              < (now() AT TIME ZONE 'America/Costa_Rica')::date THEN false
		         ELSE NULL
		       END AS cumplida
		FROM gestion_cxc ge
		JOIN cxc_canal_gestion cn ON cn.id = ge.canal_id
		JOIN cxc_resultado_gestion rg ON rg.id = ge.resultado_id
		LEFT JOIN usuario u ON u.id = ge.usuario_id
		LEFT JOIN promesa_pago_cxc pp ON pp.gestion_id = ge.id
		WHERE ge.empresa_id = $1::uuid AND ge.contrato_id = $2::uuid
		ORDER BY ge.gestionada_en DESC
		LIMIT 100`, empresaID, contratoID, fmt.Sprint(tolPromesa))
	if err != nil {
		return nil, fmt.Errorf("cxc: gestiones del contrato: %w", err)
	}
	defer rows.Close()
	out := []GestionFila{}
	for rows.Next() {
		var (
			g         GestionFila
			fecha     time.Time
			saldo     decimal.Decimal
			promFecha *time.Time
			promMonto *decimal.Decimal
		)
		if err := rows.Scan(&g.ID, &fecha, &g.Canal, &g.Resultado, &g.EsContacto, &g.Notas,
			&g.Usuario, &saldo, &g.TramoEntonces, &g.DiasMoraEntonces,
			&promFecha, &promMonto, &g.PromesaCumplida); err != nil {
			return nil, fmt.Errorf("cxc: scan gestión: %w", err)
		}
		g.Fecha = fecha.Format("2006-01-02 15:04")
		g.SaldoEntonces = saldo.String()
		g.PromesaFecha = fechaOVacio(promFecha)
		if promMonto != nil {
			g.PromesaMonto = promMonto.String()
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// CatalogosGestion son los canales y resultados para el formulario.
type CatalogosGestion struct {
	Canales    []ItemCatalogo  `json:"canales"`
	Resultados []ResultadoItem `json:"resultados"`
}

type ResultadoItem struct {
	ID           string `json:"id"`
	Codigo       string `json:"codigo"`
	Etiqueta     string `json:"etiqueta"`
	EsContacto   bool   `json:"es_contacto"`
	ExigePromesa bool   `json:"exige_promesa"`
}

func (r *pgRepository) CatalogosGestion(ctx context.Context, empresaID string) (CatalogosGestion, error) {
	out := CatalogosGestion{Canales: []ItemCatalogo{}, Resultados: []ResultadoItem{}}
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, nombre FROM cxc_canal_gestion WHERE empresa_id = $1::uuid AND activo = true ORDER BY orden, nombre`,
		empresaID)
	if err != nil {
		return CatalogosGestion{}, fmt.Errorf("cxc: canales: %w", err)
	}
	for rows.Next() {
		var it ItemCatalogo
		if err := rows.Scan(&it.ID, &it.Nombre); err != nil {
			rows.Close()
			return CatalogosGestion{}, err
		}
		out.Canales = append(out.Canales, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return CatalogosGestion{}, err
	}
	rows, err = r.pool.Query(ctx, `
		SELECT id::text, codigo, etiqueta, es_contacto, exige_promesa
		FROM cxc_resultado_gestion WHERE empresa_id = $1::uuid AND activo = true ORDER BY orden, etiqueta`,
		empresaID)
	if err != nil {
		return CatalogosGestion{}, fmt.Errorf("cxc: resultados: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var it ResultadoItem
		if err := rows.Scan(&it.ID, &it.Codigo, &it.Etiqueta, &it.EsContacto, &it.ExigePromesa); err != nil {
			return CatalogosGestion{}, err
		}
		out.Resultados = append(out.Resultados, it)
	}
	return out, rows.Err()
}
