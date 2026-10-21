package precios

// Acceso a datos de las políticas de comisión.

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// Politicas lista las políticas de comisión de la empresa.
//
// Trae el nombre del plan y del vendedor resueltos: la pantalla muestra «Oro» y no un uuid, y
// hacerlo acá evita que el cliente tenga que cruzar tres listas para pintar una fila.
func (r *pgRepository) Politicas(ctx context.Context, empresaID string) ([]PoliticaComision, error) {
	const q = `
	SELECT pc.id::text, pc.plan_id::text, COALESCE(pl.nombre,''),
	       pc.vendedor_id::text, COALESCE(e.nombre,''),
	       COALESCE(pc.canal,''), pc.tipo, pc.base, pc.valor,
	       pc.vigente_desde::text, pc.vigente_hasta::text,
	       COALESCE(pc.notas,''), pc.activo
	FROM politica_comision pc
	LEFT JOIN plan_precio pl ON pl.id = pc.plan_id
	LEFT JOIN empleado e ON e.id = pc.vendedor_id
	WHERE pc.empresa_id = $1::uuid
	ORDER BY pc.activo DESC, pc.vigente_desde DESC`

	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("precios: listar políticas: %w", err)
	}
	defer rows.Close()

	out := []PoliticaComision{}
	for rows.Next() {
		var p PoliticaComision
		var valor decimal.Decimal
		if err := rows.Scan(&p.ID, &p.PlanID, &p.Plan, &p.VendedorID, &p.Vendedor,
			&p.Canal, &p.Tipo, &p.Base, &valor, &p.VigenteDesde, &p.VigenteHasta,
			&p.Notas, &p.Activo); err != nil {
			return nil, fmt.Errorf("precios: scan política: %w", err)
		}
		p.Valor = valor.String()
		p.Especificidad = especificidadDe(p.PlanID, p.VendedorID, p.Canal)
		out = append(out, p)
	}
	return out, rows.Err()
}

// CrearPolitica agrega una política.
func (r *pgRepository) CrearPolitica(ctx context.Context, empresaID string, in PoliticaInput, usuarioID string) (string, error) {
	const q = `INSERT INTO politica_comision (empresa_id, plan_id, vendedor_id, canal, tipo, base,
	                                          valor, vigente_desde, vigente_hasta, notas, creado_por)
	           VALUES ($1::uuid, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid, NULLIF($4,''), $5, $6,
	                   $7, $8::date, NULLIF($9,'')::date, $10, NULLIF($11,'')::uuid)
	           RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, textoDe(in.PlanID), textoDe(in.VendedorID), in.Canal,
		in.Tipo, in.Base, in.Valor, in.VigenteDesde, textoDe(in.VigenteHasta), in.Notas, usuarioID).Scan(&id)
	if esViolacionUnica(err) {
		return "", ErrCodigoDuplicado
	}
	if err != nil {
		return "", fmt.Errorf("precios: crear política: %w", err)
	}
	return id, nil
}

// ActualizarPolitica cambia una política.
func (r *pgRepository) ActualizarPolitica(ctx context.Context, empresaID, id string, in PoliticaInput) error {
	const q = `UPDATE politica_comision
	           SET plan_id = NULLIF($3,'')::uuid, vendedor_id = NULLIF($4,'')::uuid,
	               canal = NULLIF($5,''), tipo = $6, base = $7, valor = $8,
	               vigente_desde = $9::date, vigente_hasta = NULLIF($10,'')::date,
	               notas = $11, activo = COALESCE($12, activo), actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, textoDe(in.PlanID), textoDe(in.VendedorID),
		in.Canal, in.Tipo, in.Base, in.Valor, in.VigenteDesde, textoDe(in.VigenteHasta),
		in.Notas, in.Activo)
	if esViolacionUnica(err) {
		return ErrCodigoDuplicado
	}
	if err != nil {
		return fmt.Errorf("precios: actualizar política: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPoliticaNoEncontrada
	}
	return nil
}

// EliminarPolitica borra una política.
//
// Se puede borrar sin más porque una política no tiene historia colgando: el costeo de un
// período pasado se recalcula con las políticas vigentes ENTONCES, que es lo que guarda la
// fecha de vigencia. Lo que no hay que hacer es cambiarle la fecha a una vieja.
func (r *pgRepository) EliminarPolitica(ctx context.Context, empresaID, id string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM politica_comision WHERE empresa_id=$1::uuid AND id=$2::uuid`, empresaID, id)
	if err != nil {
		return fmt.Errorf("precios: eliminar política: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPoliticaNoEncontrada
	}
	return nil
}

// Vendedores lista los empleados que pueden tener política propia.
func (r *pgRepository) Vendedores(ctx context.Context, empresaID string) ([]Vendedor, error) {
	const q = `SELECT id::text, nombre, COALESCE(puesto,'')
	           FROM empleado WHERE empresa_id = $1::uuid AND activo
	           ORDER BY lower(nombre)`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("precios: listar vendedores: %w", err)
	}
	defer rows.Close()

	out := []Vendedor{}
	for rows.Next() {
		var v Vendedor
		if err := rows.Scan(&v.ID, &v.Nombre, &v.Puesto); err != nil {
			return nil, fmt.Errorf("precios: scan vendedor: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Vendedor es un empleado que puede tener política propia.
type Vendedor struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Puesto string `json:"puesto"`
}

func textoDe(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
