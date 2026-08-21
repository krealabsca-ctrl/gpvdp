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

// condicionesContratos arma el WHERE de la cartera. Igual que en la hoja de trabajo de
// Bancos, lo comparten la LISTA y su RESUMEN: si cada uno armara su filtro, el encabezado
// terminaría diciendo un número y la tabla mostrando otro.
//
// El alcance por sede llega en f.SedeIDs y NO es negociable desde el cliente: lo inyecta
// el servicio según el permiso.
func condicionesContratos(empresaID string, f FiltrosContratos) (string, []any) {
	conds := []string{"c.empresa_id = $1::uuid"}
	args := []any{empresaID}
	add := func(v any) int { args = append(args, v); return len(args) }

	if f.SedeIDs != nil {
		// Lista vacía = no ve nada. Un `IN ()` vacío es inválido en SQL, así que se
		// fuerza el falso explícito en vez de dejar pasar todo por descuido.
		if len(f.SedeIDs) == 0 {
			conds = append(conds, "false")
		} else {
			conds = append(conds, fmt.Sprintf("c.sede_id = ANY($%d::uuid[])", add(f.SedeIDs)))
		}
	}
	if f.SedeID != "" {
		conds = append(conds, fmt.Sprintf("c.sede_id = $%d::uuid", add(f.SedeID)))
	}
	if f.ModalidadID != "" {
		conds = append(conds, fmt.Sprintf("c.modalidad_id = $%d::uuid", add(f.ModalidadID)))
	}
	if f.FormaPagoID != "" {
		conds = append(conds, fmt.Sprintf("c.forma_pago_id = $%d::uuid", add(f.FormaPagoID)))
	}
	if f.AsociacionID != "" {
		conds = append(conds, fmt.Sprintf("c.asociacion_id = $%d::uuid", add(f.AsociacionID)))
	}
	if f.Estado != "" {
		conds = append(conds, fmt.Sprintf("c.estado = $%d", add(f.Estado)))
	}
	if f.EnRevision {
		conds = append(conds, "c.revision_pendiente = true")
	}
	if f.Q != "" {
		// Busca por número, cédula o nombre. El nombre va con ILIKE %…% porque la gente
		// busca «vargas mora», no el prefijo (el índice trigram sostiene eso).
		n := add("%" + f.Q + "%")
		conds = append(conds, fmt.Sprintf(
			"(c.numero ILIKE $%d OR c.documento ILIKE $%d OR c.cliente_nombre ILIKE $%d)", n, n, n))
	}
	return strings.Join(conds, " AND "), args
}

// saldoPorContrato es la subconsulta que DERIVA el estado de cobro de cada contrato a
// partir de sus cargos abiertos. No se guarda nada: el saldo de un contrato es siempre la
// suma de lo que le falta a sus cargos, y los días de mora los del más viejo sin pagar.
const saldoPorContrato = `
	LEFT JOIN (
		SELECT g.contrato_id,
		       count(*)::int                                   AS cargos,
		       sum(g.monto - g.monto_aplicado)                  AS saldo,
		       sum(CASE WHEN g.vence_en < $HOY THEN g.monto - g.monto_aplicado ELSE 0 END) AS vencido,
		       sum(CASE WHEN g.vence_en >= $HOY THEN g.monto - g.monto_aplicado ELSE 0 END) AS por_vencer,
		       min(g.vence_en)                                  AS mas_viejo
		FROM cargo_cxc g
		WHERE g.empresa_id = $1::uuid AND g.estado IN ('ABIERTO','PARCIAL')
		GROUP BY g.contrato_id
	) s ON s.contrato_id = c.id`

func (r *pgRepository) ListarContratos(ctx context.Context, empresaID string, f FiltrosContratos) (ListaContratos, error) {
	where, args := condicionesContratos(empresaID, f)
	// El «hoy» es el día de operación de Costa Rica, no el UTC del servidor: a las 6 p. m.
	// de un 31 en Costa Rica ya es día 1 en UTC y la mora saltaría un día.
	hoy := "(now() AT TIME ZONE 'America/Costa_Rica')::date"
	join := strings.ReplaceAll(saldoPorContrato, "$HOY", hoy)

	// ── Resumen del filtro (sobre TODO el conjunto, no la página)
	var res ResumenCartera
	var saldo, vencido, porVencer decimal.Decimal
	err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int,
		       count(*) FILTER (WHERE COALESCE(s.saldo,0) > 0)::int,
		       COALESCE(sum(s.saldo),0), COALESCE(sum(s.vencido),0), COALESCE(sum(s.por_vencer),0),
		       COALESCE(sum(s.cargos),0)::int
		FROM contrato_cxc c`+join+` WHERE `+where, args...).
		Scan(&res.Contratos, &res.ConSaldo, &saldo, &vencido, &porVencer, &res.Cargos)
	if err != nil {
		return ListaContratos{}, fmt.Errorf("cxc: resumen de cartera: %w", err)
	}
	res.Saldo, res.Vencido, res.PorVencer = saldo.String(), vencido.String(), porVencer.String()

	if f.ConSaldo {
		where += " AND COALESCE(s.saldo,0) > 0"
	}
	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*)::int FROM contrato_cxc c`+join+` WHERE `+where, args...).Scan(&total); err != nil {
		return ListaContratos{}, fmt.Errorf("cxc: contar contratos: %w", err)
	}

	args = append(args, pageSize, (page-1)*pageSize)
	q := `
		SELECT c.id::text, c.numero, COALESCE(c.sede_id::text,''), COALESCE(sd.nombre,''),
		       c.cliente_nombre, c.documento, c.telefonos, c.correos,
		       c.servicio, c.tipo_servicio, COALESCE(m.nombre,''), COALESCE(fp.nombre,''), COALESCE(a.nombre,''),
		       c.dia_pago, c.cuota_vigente, c.fecha_inicial, c.fecha_primer_cobro, c.tarjeta_vence, c.estado,
		       COALESCE(s.cargos,0), COALESCE(s.saldo,0),
		       CASE WHEN s.mas_viejo IS NULL THEN 0 ELSE (` + hoy + ` - s.mas_viejo) END,
		       c.revision_pendiente, c.revision_motivo,
		       c.score_origen, c.morosidad_origen, c.dias_vencidos_origen, c.saldo_origen
		FROM contrato_cxc c` + join + `
		LEFT JOIN cxc_sede sd ON sd.id = c.sede_id
		LEFT JOIN cxc_modalidad m ON m.id = c.modalidad_id
		LEFT JOIN cxc_forma_pago fp ON fp.id = c.forma_pago_id
		LEFT JOIN cxc_asociacion a ON a.id = c.asociacion_id
		WHERE ` + where + `
		ORDER BY ` + ordenContratos(f.Orden) + `
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return ListaContratos{}, fmt.Errorf("cxc: listar contratos: %w", err)
	}
	defer rows.Close()

	items := make([]Contrato, 0, pageSize)
	for rows.Next() {
		c, err := scanContrato(rows)
		if err != nil {
			return ListaContratos{}, err
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return ListaContratos{}, err
	}
	return ListaContratos{Resumen: res, Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func ordenContratos(orden string) string {
	switch orden {
	case "saldo_desc":
		return "COALESCE(s.saldo,0) DESC, c.numero"
	case "mora_desc":
		return "s.mas_viejo ASC NULLS LAST, c.numero"
	case "numero":
		return "c.numero"
	case "cliente":
		return "c.cliente_nombre, c.numero"
	default:
		// Por omisión, lo que más pesa primero: es la pantalla de trabajo de cobros.
		return "COALESCE(s.saldo,0) DESC, c.numero"
	}
}

func scanContrato(rows pgx.Rows) (Contrato, error) {
	var (
		c                Contrato
		diaPago          *int16
		cuota            decimal.Decimal
		fIni, fPri, fTar *time.Time
		saldo            decimal.Decimal
		saldoOrigen      *decimal.Decimal
	)
	if err := rows.Scan(&c.ID, &c.Numero, &c.SedeID, &c.Sede,
		&c.Cliente, &c.Documento, &c.Telefonos, &c.Correos,
		&c.Servicio, &c.TipoServicio, &c.Modalidad, &c.FormaPago, &c.Asociacion,
		&diaPago, &cuota, &fIni, &fPri, &fTar, &c.Estado,
		&c.Cargos, &saldo, &c.DiasMoraMax,
		&c.RevisionPendiente, &c.RevisionMotivo,
		&c.ScoreOrigen, &c.MorosidadOrigen, &c.DiasVencidosOrigen, &saldoOrigen); err != nil {
		return Contrato{}, fmt.Errorf("cxc: scan contrato: %w", err)
	}
	if diaPago != nil {
		n := int(*diaPago)
		c.DiaPago = &n
	}
	c.Cuota = cuota.String()
	c.Saldo = saldo.String()
	c.FechaInicial, c.PrimerCobro, c.TarjetaVence = fechaOVacio(fIni), fechaOVacio(fPri), fechaOVacio(fTar)
	if saldoOrigen != nil {
		s := saldoOrigen.String()
		c.SaldoOrigen = &s
	}
	return c, nil
}

func fechaOVacio(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func (r *pgRepository) ContratoPorNumero(ctx context.Context, empresaID, numero string) (Contrato, error) {
	l, err := r.ListarContratos(ctx, empresaID, FiltrosContratos{Q: numero, PageSize: 5})
	if err != nil {
		return Contrato{}, err
	}
	for _, c := range l.Items {
		if strings.EqualFold(c.Numero, numero) {
			return c, nil
		}
	}
	return Contrato{}, ErrContratoNoEncontrado
}

func (r *pgRepository) CargosDeContrato(ctx context.Context, empresaID, contratoID string, soloAbiertos bool) ([]Cargo, error) {
	cond := ""
	if soloAbiertos {
		cond = " AND g.estado IN ('ABIERTO','PARCIAL')"
	}
	// El tramo se resuelve en la misma consulta con un LATERAL contra el catálogo: así la
	// antigüedad y su etiqueta salen del mismo lugar que las de la cola.
	q := `
		SELECT g.id::text, g.periodo, g.vence_en, g.monto, g.monto_aplicado, g.estado, g.origen,
		       ((now() AT TIME ZONE 'America/Costa_Rica')::date - g.vence_en) AS dias,
		       COALESCE(t.codigo,''), COALESCE(t.etiqueta,'')
		FROM cargo_cxc g
		LEFT JOIN LATERAL (
			SELECT codigo, etiqueta FROM cxc_tramo
			WHERE empresa_id = g.empresa_id
			  AND ((now() AT TIME ZONE 'America/Costa_Rica')::date - g.vence_en) BETWEEN dias_min AND dias_max
			LIMIT 1
		) t ON true
		WHERE g.empresa_id = $1::uuid AND g.contrato_id = $2::uuid` + cond + `
		ORDER BY g.vence_en`
	rows, err := r.pool.Query(ctx, q, empresaID, contratoID)
	if err != nil {
		return nil, fmt.Errorf("cxc: cargos del contrato: %w", err)
	}
	defer rows.Close()
	out := []Cargo{}
	for rows.Next() {
		var (
			g            Cargo
			vence        time.Time
			monto, aplic decimal.Decimal
		)
		if err := rows.Scan(&g.ID, &g.Periodo, &vence, &monto, &aplic, &g.Estado, &g.Origen,
			&g.DiasMora, &g.Tramo, &g.Etiqueta); err != nil {
			return nil, fmt.Errorf("cxc: scan cargo: %w", err)
		}
		g.VenceEn = vence.Format("2006-01-02")
		g.Monto, g.Aplicado = monto.String(), aplic.String()
		g.Saldo = monto.Sub(aplic).String()
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *pgRepository) ContratosParaGenerar(ctx context.Context, empresaID string, sedeIDs []string) ([]ContratoGenerable, error) {
	conds := []string{
		"c.empresa_id = $1::uuid",
		"c.estado = 'ACTIVO'",
		// Un contrato en cuarentena NO genera cargos: sería fabricar deuda sobre un dato
		// que ya sabemos que está mal.
		"c.revision_pendiente = false",
		"c.fecha_primer_cobro IS NOT NULL",
		"c.cuota_vigente > 0",
		"c.modalidad_id IS NOT NULL",
	}
	args := []any{empresaID}
	if sedeIDs != nil {
		if len(sedeIDs) == 0 {
			return []ContratoGenerable{}, nil
		}
		args = append(args, sedeIDs)
		conds = append(conds, fmt.Sprintf("c.sede_id = ANY($%d::uuid[])", len(args)))
	}
	q := `
		SELECT c.id::text, c.numero, c.fecha_primer_cobro, COALESCE(c.dia_pago,0), c.cuota_vigente,
		       m.meses_ciclo, m.quincenal
		FROM contrato_cxc c
		JOIN cxc_modalidad m ON m.id = c.modalidad_id
		WHERE ` + strings.Join(conds, " AND ") + `
		ORDER BY c.numero`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("cxc: contratos para generar: %w", err)
	}
	defer rows.Close()
	out := []ContratoGenerable{}
	for rows.Next() {
		var (
			g       ContratoGenerable
			primer  time.Time
			dia     int16
			ciclo   int16
			cuota   decimal.Decimal
			quincen bool
		)
		if err := rows.Scan(&g.ID, &g.Numero, &primer, &dia, &cuota, &ciclo, &quincen); err != nil {
			return nil, fmt.Errorf("cxc: scan generable: %w", err)
		}
		g.PrimerCobro = primer.Format("2006-01-02")
		g.DiaPago, g.Cuota, g.MesesCiclo, g.Quincenal = int(dia), cuota, int(ciclo), quincen
		out = append(out, g)
	}
	return out, rows.Err()
}

// InsertarCargos escribe los cargos en lote y devuelve cuántos entraron DE VERDAD.
// El ON CONFLICT DO NOTHING sobre (contrato_id, periodo) es lo que hace idempotente al
// generador: correrlo dos veces no duplica y el segundo pase reporta 0 nuevos.
func (r *pgRepository) InsertarCargos(ctx context.Context, empresaID string, cargos []CargoAInsertar) (int, error) {
	if len(cargos) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("cxc: begin cargos: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const lote = 1000
	insertados := 0
	for i := 0; i < len(cargos); i += lote {
		fin := i + lote
		if fin > len(cargos) {
			fin = len(cargos)
		}
		var sb strings.Builder
		sb.WriteString(`INSERT INTO cargo_cxc (empresa_id, contrato_id, periodo, vence_en, monto, origen) VALUES `)
		args := make([]any, 0, (fin-i)*5+1)
		args = append(args, empresaID)
		for j, c := range cargos[i:fin] {
			if j > 0 {
				sb.WriteString(", ")
			}
			b := len(args)
			args = append(args, c.ContratoID, c.Periodo, c.VenceEn, c.Monto.String(), c.Origen)
			fmt.Fprintf(&sb, "($1::uuid, $%d::uuid, $%d, $%d::date, $%d::numeric, $%d)",
				b+1, b+2, b+3, b+4, b+5)
		}
		sb.WriteString(" ON CONFLICT (contrato_id, periodo) DO NOTHING")
		tag, err := tx.Exec(ctx, sb.String(), args...)
		if err != nil {
			return 0, fmt.Errorf("cxc: insertar cargos: %w", err)
		}
		insertados += int(tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("cxc: commit cargos: %w", err)
	}
	return insertados, nil
}

var _ = errors.Is
