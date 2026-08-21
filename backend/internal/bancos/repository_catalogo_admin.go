package bancos

// Administración del catálogo (renombrar / eliminar conceptos y clasificaciones).
// Eliminar es FÍSICO solo cuando la entrada no tiene NINGUNA referencia (catálogo,
// no tabla financiera); si está en uso se devuelve el detalle para que el usuario
// reclasifique primero (p. ej. una clasificación duplicada por error).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *pgRepository) RenombrarConcepto(ctx context.Context, empresaID, conceptoID, nombre string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE concepto SET nombre = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, conceptoID, nombre)
	if esViolacionUnica(err) {
		return ErrCatalogoDuplicado
	}
	if err != nil {
		return fmt.Errorf("bancos: renombrar concepto: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConceptoNoEncontrado
	}
	return nil
}

// CambiarNaturaleza declara la naturaleza del concepto y devuelve la que tenía, para que la
// auditoría pueda registrar el ANTES: mover un concepto de NEUTRO a GASTO cambia el EBITDA de todos
// los períodos, y sin el valor anterior no se puede reconstruir por qué el número cambió.
func (r *pgRepository) CambiarNaturaleza(ctx context.Context, empresaID, conceptoID, naturaleza string) (string, error) {
	// El CTE `previo` se materializa ANTES del UPDATE, así que devuelve el valor viejo sin
	// ambigüedad. Un `RETURNING naturaleza` a secas devolvería el nuevo.
	//
	// `naturaleza_declarada = true` también cuando se elige NEUTRO: elegir «no entra al EBITDA» ES
	// una decisión y tiene que poder distinguirse del silencio (migración 0064). Es lo que hace que
	// el aviso del tablero se pueda apagar respondiendo bien.
	var anterior string
	err := r.pool.QueryRow(ctx,
		`WITH previo AS (
		     SELECT id, naturaleza FROM concepto WHERE empresa_id = $1::uuid AND id = $2::uuid
		 )
		 UPDATE concepto c SET naturaleza = $3, naturaleza_declarada = true
		 FROM previo p WHERE c.id = p.id
		 RETURNING p.naturaleza`,
		empresaID, conceptoID, naturaleza).Scan(&anterior)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrConceptoNoEncontrado
	}
	if err != nil {
		return "", fmt.Errorf("bancos: cambiar naturaleza: %w", err)
	}
	return anterior, nil
}

func (r *pgRepository) CambiarVisibilidadCxP(ctx context.Context, empresaID, conceptoID string, visible bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE concepto SET visible_cxp = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, conceptoID, visible)
	if err != nil {
		return fmt.Errorf("bancos: cambiar visibilidad cxp: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConceptoNoEncontrado
	}
	return nil
}

func (r *pgRepository) EliminarConcepto(ctx context.Context, empresaID, conceptoID string) error {
	// Tenant-safe: primero se verifica que el concepto pertenezca a la empresa.
	var pertenece bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM concepto WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
		empresaID, conceptoID).Scan(&pertenece); err != nil {
		return fmt.Errorf("bancos: verificar concepto: %w", err)
	}
	if !pertenece {
		return ErrConceptoNoEncontrado
	}
	// Referencias que impiden el borrado (incluye las de CxP, que comparte catálogo).
	const qRefs = `
		SELECT
			(SELECT COUNT(*) FROM clasificacion WHERE concepto_id = $1::uuid),
			(SELECT COUNT(*) FROM movimiento_bancario WHERE concepto_id = $1::uuid),
			(SELECT COUNT(*) FROM regla_clasificacion WHERE concepto_id = $1::uuid),
			(SELECT COUNT(*) FROM documento_cxp WHERE concepto_id = $1::uuid),
			(SELECT COUNT(*) FROM proveedor_gasto WHERE concepto_id = $1::uuid),
			(SELECT COUNT(*) FROM caja_chica_vale WHERE concepto_id = $1::uuid),
			(SELECT COUNT(*) FROM proveedor WHERE gasto_concepto_id = $1::uuid)`
	var clasifs, movs, reglas, docs, provs, vales, proveedores int
	if err := r.pool.QueryRow(ctx, qRefs, conceptoID).
		Scan(&clasifs, &movs, &reglas, &docs, &provs, &vales, &proveedores); err != nil {
		return fmt.Errorf("bancos: referencias de concepto: %w", err)
	}
	if detalle := detalleEnUso(clasifs, movs, reglas, docs, provs, vales, proveedores); detalle != "" {
		return &CatalogoEnUsoError{Detalle: detalle}
	}
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM concepto WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, conceptoID)
	if err != nil {
		return fmt.Errorf("bancos: eliminar concepto: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConceptoNoEncontrado
	}
	return nil
}

func (r *pgRepository) RenombrarClasificacion(ctx context.Context, empresaID, clasificacionID, nombre string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE clasificacion SET nombre = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, clasificacionID, nombre)
	if esViolacionUnica(err) {
		return ErrCatalogoDuplicado
	}
	if err != nil {
		return fmt.Errorf("bancos: renombrar clasificación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrClasificacionNoEncontrada
	}
	return nil
}

// ReasignarConceptoClasificacion mueve una clasificación a otro concepto. Solo se permite
// cuando la clasificación NO tiene referencias (mismo guard que Eliminar): así se corrige una
// mal creada aún sin uso sin romper los FK compuestos (movimiento/regla/documento apuntan a
// (clasificacion_id, concepto_id)). Tenant-safe en clasificación y concepto destino.
func (r *pgRepository) ReasignarConceptoClasificacion(ctx context.Context, empresaID, clasificacionID, nuevoConceptoID string) error {
	var pertenece bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM clasificacion WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
		empresaID, clasificacionID).Scan(&pertenece); err != nil {
		return fmt.Errorf("bancos: verificar clasificación: %w", err)
	}
	if !pertenece {
		return ErrClasificacionNoEncontrada
	}
	var conceptoOk bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM concepto WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
		empresaID, nuevoConceptoID).Scan(&conceptoOk); err != nil {
		return fmt.Errorf("bancos: verificar concepto: %w", err)
	}
	if !conceptoOk {
		return ErrConceptoNoEncontrado
	}
	const qRefs = `
		SELECT
			0,
			(SELECT COUNT(*) FROM movimiento_bancario WHERE clasificacion_id = $1::uuid),
			(SELECT COUNT(*) FROM regla_clasificacion WHERE clasificacion_id = $1::uuid),
			(SELECT COUNT(*) FROM documento_cxp d
			  WHERE d.clasificacion_id = $1::uuid
			     OR d.subclasificacion_id IN (SELECT id FROM subclasificacion WHERE clasificacion_id = $1::uuid)),
			(SELECT COUNT(*) FROM proveedor_gasto WHERE clasificacion_id = $1::uuid),
			(SELECT COUNT(*) FROM caja_chica_vale v
			  WHERE v.clasificacion_id = $1::uuid
			     OR v.subclasificacion_id IN (SELECT id FROM subclasificacion WHERE clasificacion_id = $1::uuid)),
			(SELECT COUNT(*) FROM proveedor p
			  WHERE p.gasto_clasificacion_id = $1::uuid
			     OR p.gasto_subclasificacion_id IN (SELECT id FROM subclasificacion WHERE clasificacion_id = $1::uuid))`
	var cero, movs, reglas, docs, provs, vales, proveedores int
	if err := r.pool.QueryRow(ctx, qRefs, clasificacionID).
		Scan(&cero, &movs, &reglas, &docs, &provs, &vales, &proveedores); err != nil {
		return fmt.Errorf("bancos: referencias de clasificación: %w", err)
	}
	if detalle := detalleEnUso(cero, movs, reglas, docs, provs, vales, proveedores); detalle != "" {
		return &CatalogoEnUsoError{Detalle: detalle}
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE clasificacion SET concepto_id = $3::uuid WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, clasificacionID, nuevoConceptoID)
	if esViolacionUnica(err) {
		return ErrCatalogoDuplicado // el concepto destino ya tiene una clasificación con ese nombre
	}
	if err != nil {
		return fmt.Errorf("bancos: reasignar concepto de clasificación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrClasificacionNoEncontrada
	}
	return nil
}

func (r *pgRepository) EliminarClasificacion(ctx context.Context, empresaID, clasificacionID string) error {
	// Tenant-safe: primero se verifica que la clasificación pertenezca a la empresa.
	var pertenece bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM clasificacion WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
		empresaID, clasificacionID).Scan(&pertenece); err != nil {
		return fmt.Errorf("bancos: verificar clasificación: %w", err)
	}
	if !pertenece {
		return ErrClasificacionNoEncontrada
	}
	// Las subclasificaciones sin uso caen en cascada; las usadas por CxP bloquean.
	const qRefs = `
		SELECT
			0,
			(SELECT COUNT(*) FROM movimiento_bancario WHERE clasificacion_id = $1::uuid),
			(SELECT COUNT(*) FROM regla_clasificacion WHERE clasificacion_id = $1::uuid),
			(SELECT COUNT(*) FROM documento_cxp d
			  WHERE d.clasificacion_id = $1::uuid
			     OR d.subclasificacion_id IN (SELECT id FROM subclasificacion WHERE clasificacion_id = $1::uuid)),
			(SELECT COUNT(*) FROM proveedor_gasto WHERE clasificacion_id = $1::uuid),
			(SELECT COUNT(*) FROM caja_chica_vale v
			  WHERE v.clasificacion_id = $1::uuid
			     OR v.subclasificacion_id IN (SELECT id FROM subclasificacion WHERE clasificacion_id = $1::uuid)),
			(SELECT COUNT(*) FROM proveedor p
			  WHERE p.gasto_clasificacion_id = $1::uuid
			     OR p.gasto_subclasificacion_id IN (SELECT id FROM subclasificacion WHERE clasificacion_id = $1::uuid))`
	var cero, movs, reglas, docs, provs, vales, proveedores int
	if err := r.pool.QueryRow(ctx, qRefs, clasificacionID).
		Scan(&cero, &movs, &reglas, &docs, &provs, &vales, &proveedores); err != nil {
		return fmt.Errorf("bancos: referencias de clasificación: %w", err)
	}
	if detalle := detalleEnUso(cero, movs, reglas, docs, provs, vales, proveedores); detalle != "" {
		return &CatalogoEnUsoError{Detalle: detalle}
	}
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM clasificacion WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, clasificacionID)
	if err != nil {
		return fmt.Errorf("bancos: eliminar clasificación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrClasificacionNoEncontrada
	}
	return nil
}

// detalleEnUso arma el mensaje humano de por qué no se puede eliminar ("" = libre).
//
// Cuenta TODAS las tablas que referencian al catálogo. Si faltara una, el guard dejaría
// pasar el borrado y el FK lo rechazaría después como «error interno»: por eso se listan
// los vales de caja chica y el gasto por omisión del proveedor, que se agregaron cuando
// esos módulos nacieron.
func detalleEnUso(clasifs, movs, reglas, docs, provs, vales, proveedores int) string {
	var partes []string
	if clasifs > 0 {
		partes = append(partes, fmt.Sprintf("%d clasificación(es) — eliminálas primero", clasifs))
	}
	if movs > 0 {
		partes = append(partes, fmt.Sprintf("%d movimiento(s) bancario(s) — reclasificalos primero", movs))
	}
	if reglas > 0 {
		partes = append(partes, fmt.Sprintf("%d regla(s) del motor", reglas))
	}
	if docs > 0 {
		partes = append(partes, fmt.Sprintf("%d documento(s) de CxP", docs))
	}
	if provs > 0 {
		partes = append(partes, fmt.Sprintf("%d gasto(s) de proveedor", provs))
	}
	if vales > 0 {
		partes = append(partes, fmt.Sprintf("%d vale(s) de caja chica", vales))
	}
	if proveedores > 0 {
		partes = append(partes, fmt.Sprintf("%d proveedor(es) que lo usan por omisión", proveedores))
	}
	return strings.Join(partes, ", ")
}
