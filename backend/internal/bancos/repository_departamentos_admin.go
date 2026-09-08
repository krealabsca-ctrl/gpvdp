package bancos

// Administración del catálogo de departamentos DESDE Bancos.
//
// La tabla `departamento` es de la EMPRESA, no de un módulo: vive en la migración 0026 (que llegó con
// CxP) y ya la usan las facturas, los empleados y los fondos de caja chica. Acá se abre una segunda
// puerta para administrarla desde la pantalla donde se usa, que es lo que pidió el usuario. **Es la
// misma tabla**: no hay dos catálogos que puedan contradecirse, solo dos lugares desde donde editarlo.

import (
	"context"
	"fmt"
)

// CrearDepartamento agrega un departamento al catálogo de la empresa.
func (r *pgRepository) CrearDepartamento(ctx context.Context, empresaID, nombre, codigo string) (Departamento, error) {
	const q = `INSERT INTO departamento (empresa_id, nombre, codigo, orden)
	           VALUES ($1::uuid, $2, NULLIF($3, ''),
	                   COALESCE((SELECT MAX(orden) + 1 FROM departamento WHERE empresa_id = $1::uuid), 0))
	           RETURNING id::text, nombre, COALESCE(codigo, ''), activo`
	var d Departamento
	err := r.pool.QueryRow(ctx, q, empresaID, nombre, codigo).
		Scan(&d.ID, &d.Nombre, &d.Codigo, &d.Activo)
	if esViolacionUnica(err) {
		return Departamento{}, ErrCatalogoDuplicado
	}
	if err != nil {
		return Departamento{}, fmt.Errorf("bancos: crear departamento: %w", err)
	}
	return d, nil
}

// ActualizarDepartamento renombra o cambia el código.
func (r *pgRepository) ActualizarDepartamento(ctx context.Context, empresaID, deptoID, nombre, codigo string) error {
	const q = `UPDATE departamento SET nombre = $3, codigo = NULLIF($4, ''), actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, deptoID, nombre, codigo)
	if esViolacionUnica(err) {
		return ErrCatalogoDuplicado
	}
	if err != nil {
		return fmt.Errorf("bancos: actualizar departamento: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDepartamentoNoEncontrado
	}
	return nil
}

// CambiarActivoDepartamento da de baja o revive un departamento.
func (r *pgRepository) CambiarActivoDepartamento(ctx context.Context, empresaID, deptoID string, activo bool) error {
	const q = `UPDATE departamento SET activo = $3, actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, deptoID, activo)
	if err != nil {
		return fmt.Errorf("bancos: cambiar activo de departamento: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDepartamentoNoEncontrado
	}
	return nil
}

// UsoDeDepartamento cuenta de qué cuelga un departamento, en las SIETE tablas que lo referencian.
//
// Es lo que decide si se puede borrar de verdad o solo desactivar. Contar de menos sería peor que no
// contar: dejaría borrar algo que sí tiene historia y el borrado fallaría con un error de clave
// foránea que nadie sabría interpretar.
func (r *pgRepository) UsoDeDepartamento(ctx context.Context, empresaID, deptoID string) (UsoDepartamento, error) {
	const q = `
		SELECT (SELECT count(*) FROM documento_cxp WHERE empresa_id = $1::uuid AND departamento_id = $2::uuid),
		       (SELECT count(*) FROM empleado WHERE empresa_id = $1::uuid AND departamento_id = $2::uuid),
		       (SELECT count(*) FROM caja_chica_fondo WHERE empresa_id = $1::uuid AND departamento_id = $2::uuid),
		       (SELECT count(*) FROM departamento_validador WHERE departamento_id = $2::uuid),
		       (SELECT count(*) FROM clasificacion WHERE empresa_id = $1::uuid AND departamento_id = $2::uuid),
		       (SELECT count(*) FROM movimiento_bancario WHERE empresa_id = $1::uuid AND departamento_id = $2::uuid),
		       (SELECT count(*) FROM presupuesto_departamento WHERE empresa_id = $1::uuid AND departamento_id = $2::uuid)`
	var u UsoDepartamento
	err := r.pool.QueryRow(ctx, q, empresaID, deptoID).Scan(
		&u.Facturas, &u.Empleados, &u.Fondos, &u.Validadores,
		&u.Partidas, &u.Movimientos, &u.LineasPresupuesto)
	if err != nil {
		return UsoDepartamento{}, fmt.Errorf("bancos: uso del departamento: %w", err)
	}
	return u, nil
}

// EliminarDepartamento lo borra FÍSICAMENTE. Solo se llama cuando el uso está en cero: quien decide
// eso es el servicio, que ya consultó `UsoDeDepartamento`.
func (r *pgRepository) EliminarDepartamento(ctx context.Context, empresaID, deptoID string) error {
	const q = `DELETE FROM departamento WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, deptoID)
	if err != nil {
		return fmt.Errorf("bancos: eliminar departamento: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDepartamentoNoEncontrado
	}
	return nil
}

// UsoDeSede es el equivalente para una sede: hoy solo la referencian las partidas y los movimientos.
func (r *pgRepository) UsoDeSede(ctx context.Context, empresaID, sedeID string) (UsoDepartamento, error) {
	const q = `
		SELECT (SELECT count(*) FROM clasificacion WHERE empresa_id = $1::uuid AND sede_id = $2::uuid),
		       (SELECT count(*) FROM movimiento_bancario WHERE empresa_id = $1::uuid AND sede_id = $2::uuid)`
	var u UsoDepartamento
	if err := r.pool.QueryRow(ctx, q, empresaID, sedeID).Scan(&u.Partidas, &u.Movimientos); err != nil {
		return UsoDepartamento{}, fmt.Errorf("bancos: uso de la sede: %w", err)
	}
	return u, nil
}

// EliminarSede borra una sede sin uso.
func (r *pgRepository) EliminarSede(ctx context.Context, empresaID, sedeID string) error {
	const q = `DELETE FROM sede WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, sedeID)
	if err != nil {
		return fmt.Errorf("bancos: eliminar sede: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSedeNoEncontrada
	}
	return nil
}
