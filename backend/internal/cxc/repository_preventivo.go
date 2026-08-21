package cxc

// Contacto preventivo: el universo que la cola de cobro deja FUERA a propósito.
//
// La cola solo trae lo VENCIDO, porque a nadie se le cobra una cuota que todavía no vence —el
// propio catálogo lo dice: los tramos ADELANTADO y AL_DÍA traen canal sugerido «Ninguno»—. Pero
// llamar antes del vencimiento no es cobrar: es evitar que la cuota se venza. El negocio lo
// pidió como lista aparte y con su propio permiso, y eso es exactamente lo correcto: es otra
// actividad, con otro guion y otro indicador de éxito.
//
// Dos poblaciones entran:
//
//  1. Contratos al día cuya próxima cuota vence dentro de DIAS_CONTACTO_PREVENTIVO.
//  2. Domiciliados cuya tarjeta caduca pronto. Renovarla ANTES es más barato que cobrar el
//     débito rechazado después, y hasta ahora ese dato solo salía como una marca en la cola.
//
// Se ordena por la fecha de vencimiento y no por valor esperado: acá el criterio no es cuánto
// se recupera, es a quién hay que avisarle antes de que sea tarde.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// FilaPreventiva es un contrato al que hay que avisarle antes del vencimiento.
type FilaPreventiva struct {
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
	// ProximaCuota y su monto: es el dato del aviso («su cuota de ₡12.500 vence el 12»).
	ProximaCuota   string `json:"proxima_cuota"`
	ProximoMonto   string `json:"proximo_monto"`
	DiasParaVencer int    `json:"dias_para_vencer"`
	// Motivo: POR_VENCER | TARJETA. Dice qué hay que decirle al cliente.
	Motivo        string `json:"motivo"`
	TarjetaVence  string `json:"tarjeta_vence"`
	UltimaGestion string `json:"ultima_gestion"`
	Gestiones     int    `json:"gestiones"`
	// Domiciliado: a un domiciliado el aviso es distinto (que tenga fondos, no que pague).
	Domiciliado bool `json:"domiciliado"`
}

// ResumenPreventivo mide la lista.
type ResumenPreventivo struct {
	Contratos int    `json:"contratos"`
	Monto     string `json:"monto"`
	PorVencer int    `json:"por_vencer"`
	Tarjetas  int    `json:"tarjetas"`
	// SinContactar: cuántos no tienen ni teléfono ni correo. Un aviso preventivo sin canal
	// no se puede dar, y eso es un dato de calidad de cartera que conviene ver.
	SinContactar int `json:"sin_contactar"`
	// Dias es la ventana que se está usando, para que la pantalla no la adivine.
	Dias int `json:"dias"`
}

type ListaPreventiva struct {
	Resumen ResumenPreventivo `json:"resumen"`
	Items   []FilaPreventiva  `json:"items"`
	Total   int               `json:"total"`
}

// FiltrosPreventivo filtra la lista.
type FiltrosPreventivo struct {
	SedeIDs []string
	SedeID  string
	// Motivo: POR_VENCER | TARJETA. Vacío trae las dos poblaciones.
	Motivo         string
	Q              string
	Page, PageSize int
}

// preventivoBase: contratos ACTIVOS, sin nada vencido, con la próxima cuota a la vista. El
// `NOT EXISTS` sobre lo vencido es lo que hace que esta lista y la cola sean universos
// disjuntos: ningún contrato puede salir en las dos, así que nadie recibe dos llamadas
// contradictorias el mismo día.
const preventivoBase = `
	FROM contrato_cxc c
	JOIN (
		SELECT g.contrato_id,
		       sum(g.monto - g.monto_aplicado) AS saldo,
		       min(g.vence_en) AS proxima,
		       min(g.vence_en) FILTER (WHERE g.vence_en < $HOY) AS vencida
		FROM cargo_cxc g
		WHERE g.empresa_id = $1::uuid AND g.estado IN ('ABIERTO','PARCIAL')
		GROUP BY g.contrato_id
		HAVING sum(g.monto - g.monto_aplicado) > 0
	) s ON s.contrato_id = c.id
	LEFT JOIN cxc_sede sd ON sd.id = c.sede_id
	LEFT JOIN cxc_modalidad m ON m.id = c.modalidad_id
	LEFT JOIN cxc_forma_pago fp ON fp.id = c.forma_pago_id
	LEFT JOIN cxc_asociacion a ON a.id = c.asociacion_id
	LEFT JOIN (
		SELECT DISTINCT ON (ge.contrato_id) ge.contrato_id, ge.gestionada_en
		FROM gestion_cxc ge WHERE ge.empresa_id = $1::uuid
		ORDER BY ge.contrato_id, ge.gestionada_en DESC
	) ug ON ug.contrato_id = c.id
	LEFT JOIN (
		SELECT contrato_id, count(*)::int AS n FROM gestion_cxc
		WHERE empresa_id = $1::uuid GROUP BY contrato_id
	) cg ON cg.contrato_id = c.id`

// ListaPreventiva arma la lista.
func (r *pgRepository) ListaPreventiva(ctx context.Context, empresaID string, f FiltrosPreventivo, dias, diasTarjeta int) (ListaPreventiva, error) {
	hoy := "(now() AT TIME ZONE 'America/Costa_Rica')::date"
	rep := func(s string) string { return strings.ReplaceAll(s, "$HOY", hoy) }
	base := rep(preventivoBase)

	// Las dos razones de estar en la lista.
	porVencer := fmt.Sprintf("(s.vencida IS NULL AND s.proxima <= %s + '%d days'::interval)", hoy, dias)
	tarjeta := fmt.Sprintf(
		"(fp.es_domiciliado = true AND c.tarjeta_vence IS NOT NULL AND c.tarjeta_vence >= %s"+
			" AND c.tarjeta_vence <= %s + '%d days'::interval)", hoy, hoy, diasTarjeta)

	conds := []string{
		"c.empresa_id = $1::uuid",
		"c.estado = 'ACTIVO'",
		"c.revision_pendiente = false",
		// Nada vencido: si ya se venció, el contrato es de la cola, no de esta lista.
		"s.vencida IS NULL",
	}
	args := []any{empresaID}
	add := func(v any) int { args = append(args, v); return len(args) }

	switch f.Motivo {
	case "POR_VENCER":
		conds = append(conds, porVencer)
	case "TARJETA":
		conds = append(conds, tarjeta)
	default:
		conds = append(conds, "("+porVencer+" OR "+tarjeta+")")
	}

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
	if f.Q != "" {
		n := add("%" + f.Q + "%")
		conds = append(conds, fmt.Sprintf(
			"(c.numero ILIKE $%d OR c.documento ILIKE $%d OR c.cliente_nombre ILIKE $%d)", n, n, n))
	}
	where := strings.Join(conds, " AND ")

	var res ResumenPreventivo
	var monto decimal.Decimal
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int, COALESCE(sum(s.saldo),0),
		       count(*) FILTER (WHERE `+porVencer+`)::int,
		       count(*) FILTER (WHERE `+tarjeta+`)::int,
		       count(*) FILTER (WHERE COALESCE(btrim(c.telefonos), '') = ''
		                          AND COALESCE(btrim(c.correos), '') = '')::int
		`+base+` WHERE `+where, args...).
		Scan(&res.Contratos, &monto, &res.PorVencer, &res.Tarjetas, &res.SinContactar); err != nil {
		return ListaPreventiva{}, fmt.Errorf("cxc: resumen preventivo: %w", err)
	}
	res.Monto, res.Dias = monto.String(), dias

	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, `
		SELECT c.id::text, c.numero, c.cliente_nombre, COALESCE(c.documento,''),
		       COALESCE(c.telefonos,''), COALESCE(c.correos,''),
		       COALESCE(sd.nombre,''), COALESCE(fp.nombre,''), COALESCE(a.nombre,''), COALESCE(m.nombre,''),
		       s.saldo, s.proxima,
		       COALESCE((SELECT sum(g.monto - g.monto_aplicado) FROM cargo_cxc g
		        WHERE g.contrato_id = c.id AND g.estado IN ('ABIERTO','PARCIAL')
		          AND g.vence_en = s.proxima), 0),
		       (s.proxima - `+hoy+`),
		       CASE WHEN `+porVencer+` THEN 'POR_VENCER' ELSE 'TARJETA' END,
		       c.tarjeta_vence, COALESCE(fp.es_domiciliado, false),
		       (ug.gestionada_en AT TIME ZONE 'America/Costa_Rica'), COALESCE(cg.n,0)
		`+base+`
		WHERE `+where+`
		-- Por fecha de vencimiento: acá el criterio no es cuánto se recupera, es a quién se le
		-- avisa antes de que sea tarde.
		ORDER BY s.proxima ASC, s.saldo DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return ListaPreventiva{}, fmt.Errorf("cxc: lista preventiva: %w", err)
	}
	defer rows.Close()

	items := make([]FilaPreventiva, 0, pageSize)
	for rows.Next() {
		var (
			fp           FilaPreventiva
			saldo, prox  decimal.Decimal
			proxima      time.Time
			tarj, ultima *time.Time
		)
		if err := rows.Scan(&fp.ContratoID, &fp.Numero, &fp.Cliente, &fp.Documento,
			&fp.Telefonos, &fp.Correos, &fp.Sede, &fp.FormaPago, &fp.Asociacion, &fp.Modalidad,
			&saldo, &proxima, &prox, &fp.DiasParaVencer, &fp.Motivo,
			&tarj, &fp.Domiciliado, &ultima, &fp.Gestiones); err != nil {
			return ListaPreventiva{}, fmt.Errorf("cxc: scan fila preventiva: %w", err)
		}
		fp.Saldo, fp.ProximoMonto = saldo.String(), prox.String()
		fp.ProximaCuota = proxima.Format("2006-01-02")
		fp.TarjetaVence = fechaOVacio(tarj)
		if ultima != nil {
			fp.UltimaGestion = ultima.Format("2006-01-02")
		}
		items = append(items, fp)
	}
	if err := rows.Err(); err != nil {
		return ListaPreventiva{}, fmt.Errorf("cxc: leer lista preventiva: %w", err)
	}
	return ListaPreventiva{Resumen: res, Items: items, Total: res.Contratos}, nil
}
