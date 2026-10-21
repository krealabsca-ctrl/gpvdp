package precios

// Planes y costeo.
//
// ── POR QUÉ EL COSTEO SE ARMA EN GO Y NO EN SQL ─────────────────────────────────────────────
//
// Las líneas SÍ se traen con SQL (una consulta, con el costo vigente resuelto por LATERAL), pero
// la suma se hace en Go con decimal. La razón es la regla que más importa: hay que distinguir
// «este insumo cuesta cero» de «este insumo no tiene costo capturado». Un SUM() de SQL trata el
// NULL como cero y devuelve un total que parece completo; acá cada línea sin costo se cuenta
// aparte y el total sale acompañado de su cobertura.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// CapturaCosto es una fila del historial de costos de un insumo.
type CapturaCosto struct {
	Costo        string `json:"costo"`
	Moneda       string `json:"moneda"`
	VigenteDesde string `json:"vigente_desde"`
	Origen       string `json:"origen"`
	Nota         string `json:"nota"`
	CapturadoPor string `json:"capturado_por"`
}

// PlanInput es lo que llega para crear o editar un plan.
type PlanInput struct {
	Codigo           string  `json:"codigo"`
	Nombre           string  `json:"nombre"`
	Familia          string  `json:"familia"`
	Modalidad        string  `json:"modalidad"`
	PrecioVenta      *string `json:"precio_venta"`
	CuotaMensual     *string `json:"cuota_mensual"`
	Moneda           string  `json:"moneda"`
	Beneficiarios    *int    `json:"beneficiarios"`
	PermanenciaMeses *int    `json:"permanencia_meses"`
	Notas            string  `json:"notas"`
	Activo           *bool   `json:"activo"`
}

// Normalizar limpia y valida la entrada de un plan.
func (in *PlanInput) Normalizar() error {
	in.Codigo = strings.TrimSpace(in.Codigo)
	in.Nombre = strings.TrimSpace(in.Nombre)
	in.Familia = strings.TrimSpace(in.Familia)
	in.Notas = strings.TrimSpace(in.Notas)

	if in.Codigo == "" {
		return ErrCodigoVacio
	}
	if in.Nombre == "" {
		return ErrNombreVacio
	}
	if in.Familia == "" {
		in.Familia = "GENERAL"
	}
	if in.Modalidad == "" {
		in.Modalidad = ModalidadDirecto
	}
	in.Modalidad = strings.ToUpper(strings.TrimSpace(in.Modalidad))
	if in.Modalidad != ModalidadDirecto && in.Modalidad != ModalidadSuscripcion {
		return ErrModalidadInvalida
	}
	in.Moneda = strings.ToUpper(strings.TrimSpace(in.Moneda))
	if in.Moneda == "" {
		in.Moneda = "CRC"
	}
	for _, m := range []*string{in.PrecioVenta, in.CuotaMensual} {
		if m == nil || strings.TrimSpace(*m) == "" {
			continue
		}
		v, err := decimal.NewFromString(strings.TrimSpace(*m))
		if err != nil || v.IsNegative() {
			return ErrPrecioNegativo
		}
	}
	return nil
}

const basePlanSQL = `
SELECT p.id::text, p.codigo, p.nombre, p.familia, p.modalidad,
       p.precio_venta, p.cuota_mensual, p.moneda,
       p.beneficiarios, p.permanencia_meses, COALESCE(p.notas,''), p.activo
FROM plan_precio p
WHERE p.empresa_id = $1::uuid`

func scanPlan(row pgx.Row) (Plan, error) {
	var p Plan
	var precio, cuota *decimal.Decimal
	err := row.Scan(&p.ID, &p.Codigo, &p.Nombre, &p.Familia, &p.Modalidad,
		&precio, &cuota, &p.Moneda, &p.Beneficiarios, &p.PermanenciaMeses, &p.Notas, &p.Activo)
	if err != nil {
		return Plan{}, err
	}
	if precio != nil {
		s := precio.StringFixed(2)
		p.PrecioVenta = &s
	}
	if cuota != nil {
		s := cuota.StringFixed(2)
		p.CuotaMensual = &s
	}
	return p, nil
}

// Planes lista los planes de la empresa.
func (r *pgRepository) Planes(ctx context.Context, empresaID string) ([]Plan, error) {
	rows, err := r.pool.Query(ctx, basePlanSQL+" ORDER BY p.activo DESC, p.familia, lower(p.nombre)", empresaID)
	if err != nil {
		return nil, fmt.Errorf("precios: listar planes: %w", err)
	}
	defer rows.Close()

	out := []Plan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("precios: scan plan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PlanPorID trae un plan.
func (r *pgRepository) PlanPorID(ctx context.Context, empresaID, id string) (Plan, error) {
	p, err := scanPlan(r.pool.QueryRow(ctx, basePlanSQL+" AND p.id = $2::uuid", empresaID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrPlanNoEncontrado
	}
	if err != nil {
		return Plan{}, fmt.Errorf("precios: plan por id: %w", err)
	}
	return p, nil
}

// CrearPlan agrega un plan.
func (r *pgRepository) CrearPlan(ctx context.Context, empresaID string, in PlanInput) (string, error) {
	const q = `INSERT INTO plan_precio (empresa_id, codigo, nombre, familia, modalidad,
	                                    precio_venta, cuota_mensual, moneda, beneficiarios,
	                                    permanencia_meses, notas)
	           VALUES ($1::uuid, $2, $3, $4, $5, NULLIF($6,'')::numeric, NULLIF($7,'')::numeric,
	                   $8, $9, $10, $11)
	           RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, in.Codigo, in.Nombre, in.Familia, in.Modalidad,
		texto(in.PrecioVenta), texto(in.CuotaMensual), in.Moneda, in.Beneficiarios,
		in.PermanenciaMeses, in.Notas).Scan(&id)
	if esViolacionUnica(err) {
		return "", ErrCodigoDuplicado
	}
	if err != nil {
		return "", fmt.Errorf("precios: crear plan: %w", err)
	}
	return id, nil
}

// ActualizarPlan cambia un plan.
func (r *pgRepository) ActualizarPlan(ctx context.Context, empresaID, id string, in PlanInput) error {
	const q = `UPDATE plan_precio SET codigo=$3, nombre=$4, familia=$5, modalidad=$6,
	                  precio_venta=NULLIF($7,'')::numeric, cuota_mensual=NULLIF($8,'')::numeric,
	                  moneda=$9, beneficiarios=$10, permanencia_meses=$11, notas=$12,
	                  activo=COALESCE($13, activo), actualizado_en=now()
	           WHERE empresa_id=$1::uuid AND id=$2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, in.Codigo, in.Nombre, in.Familia, in.Modalidad,
		texto(in.PrecioVenta), texto(in.CuotaMensual), in.Moneda, in.Beneficiarios,
		in.PermanenciaMeses, in.Notas, in.Activo)
	if esViolacionUnica(err) {
		return ErrCodigoDuplicado
	}
	if err != nil {
		return fmt.Errorf("precios: actualizar plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPlanNoEncontrado
	}
	return nil
}

func texto(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// LineasDeReceta trae la receta de un plan con el costo vigente de cada insumo a la fecha dada.
//
// El LEFT JOIN LATERAL es lo que hace que el costo sea derivado y no guardado. Cuando un insumo
// no tiene captura, `costo` viene NULL y eso se propaga: la línea sale sin costo, no en cero.
func (r *pgRepository) LineasDeReceta(ctx context.Context, empresaID, planID, fecha string) ([]LineaReceta, error) {
	const q = `
	SELECT rl.id::text, i.id::text, i.codigo, i.nombre, i.unidad, i.tipo_costo,
	       rl.cantidad, rl.ordinal, COALESCE(rl.nota,''), c.costo
	FROM receta_linea rl
	JOIN insumo i ON i.id = rl.insumo_id AND i.empresa_id = rl.empresa_id
	LEFT JOIN LATERAL (
	    SELECT ic.costo
	    FROM insumo_costo ic
	    WHERE ic.insumo_id = i.id AND ic.empresa_id = i.empresa_id
	      AND ic.vigente_desde <= $3::date
	    ORDER BY ic.vigente_desde DESC
	    LIMIT 1
	) c ON true
	WHERE rl.empresa_id = $1::uuid AND rl.plan_id = $2::uuid
	ORDER BY rl.ordinal, lower(i.nombre)`

	rows, err := r.pool.Query(ctx, q, empresaID, planID, fecha)
	if err != nil {
		return nil, fmt.Errorf("precios: lineas de receta: %w", err)
	}
	defer rows.Close()

	out := []LineaReceta{}
	for rows.Next() {
		var l LineaReceta
		var cantidad decimal.Decimal
		var costo *decimal.Decimal
		if err := rows.Scan(&l.ID, &l.InsumoID, &l.Codigo, &l.Nombre, &l.Unidad, &l.TipoCosto,
			&cantidad, &l.Ordinal, &l.Nota, &costo); err != nil {
			return nil, fmt.Errorf("precios: scan linea de receta: %w", err)
		}
		l.Cantidad = cantidad.String()
		if costo != nil {
			cu := costo.StringFixed(2)
			ext := costo.Mul(cantidad).Round(2).StringFixed(2)
			l.CostoUnitario, l.Extendido = &cu, &ext
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// AgregarLinea pone un insumo en la receta de un plan.
func (r *pgRepository) AgregarLinea(ctx context.Context, empresaID, planID, insumoID string, cantidad decimal.Decimal, nota string) error {
	// El insumo tiene que ser de la misma empresa. Sin esta verificación, un id de otra empresa
	// entraría por la FK —que no mira empresa_id— y cruzaría datos entre empresas.
	const dueno = `SELECT EXISTS (SELECT 1 FROM insumo WHERE id=$1::uuid AND empresa_id=$2::uuid)`
	var ok bool
	if err := r.pool.QueryRow(ctx, dueno, insumoID, empresaID).Scan(&ok); err != nil {
		return fmt.Errorf("precios: verificar insumo: %w", err)
	}
	if !ok {
		return ErrInsumoDeOtraEmpresa
	}

	const q = `INSERT INTO receta_linea (empresa_id, plan_id, insumo_id, cantidad, ordinal, nota)
	           VALUES ($1::uuid, $2::uuid, $3::uuid, $4,
	                   COALESCE((SELECT max(ordinal)+1 FROM receta_linea
	                             WHERE empresa_id=$1::uuid AND plan_id=$2::uuid), 0),
	                   $5)`
	_, err := r.pool.Exec(ctx, q, empresaID, planID, insumoID, cantidad, nota)
	if esViolacionUnica(err) {
		return ErrInsumoRepetido
	}
	if err != nil {
		return fmt.Errorf("precios: agregar linea: %w", err)
	}
	return nil
}

// CambiarCantidad ajusta la cantidad de una línea.
func (r *pgRepository) CambiarCantidad(ctx context.Context, empresaID, lineaID string, cantidad decimal.Decimal) error {
	const q = `UPDATE receta_linea SET cantidad=$3 WHERE empresa_id=$1::uuid AND id=$2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, lineaID, cantidad)
	if err != nil {
		return fmt.Errorf("precios: cambiar cantidad: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInsumoNoEncontrado
	}
	return nil
}

// QuitarLinea saca un insumo de la receta.
func (r *pgRepository) QuitarLinea(ctx context.Context, empresaID, lineaID string) error {
	const q = `DELETE FROM receta_linea WHERE empresa_id=$1::uuid AND id=$2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, lineaID)
	if err != nil {
		return fmt.Errorf("precios: quitar linea: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInsumoNoEncontrado
	}
	return nil
}
