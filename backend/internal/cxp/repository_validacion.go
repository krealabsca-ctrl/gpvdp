package cxp

import (
	"context"
	"fmt"
)

// AsignarDepartamentoDoc fija el departamento (centro de costo) de un documento.
// Solo antes de la validación de área (RECIBIDO/REVISADO) y solo si el departamento
// pertenece a la empresa y está activo. Devuelve filas afectadas (0 = doc/depto inválido o estado).
func (r *pgRepository) AsignarDepartamentoDoc(ctx context.Context, empresaID, docID, deptoID string) (int64, error) {
	const q = `
		UPDATE documento_cxp SET departamento_id = $3::uuid, actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado IN ('RECIBIDO', 'REVISADO')
		  AND EXISTS (SELECT 1 FROM departamento WHERE id = $3::uuid AND empresa_id = $1::uuid AND activo)`
	tag, err := r.pool.Exec(ctx, q, empresaID, docID, deptoID)
	if err != nil {
		return 0, fmt.Errorf("cxp: asignar departamento al documento: %w", err)
	}
	return tag.RowsAffected(), nil
}

// EsValidador indica si el usuario es validador (titular o suplente) del departamento,
// verificando además que el departamento sea de la empresa (tenant-safe).
func (r *pgRepository) EsValidador(ctx context.Context, empresaID, deptoID, usuarioID string) (bool, error) {
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM departamento_validador dv
			JOIN departamento d ON d.id = dv.departamento_id
			WHERE dv.departamento_id = $2::uuid AND dv.usuario_id = $3::uuid AND d.empresa_id = $1::uuid)`
	var ok bool
	if err := r.pool.QueryRow(ctx, q, empresaID, deptoID, usuarioID).Scan(&ok); err != nil {
		return false, fmt.Errorf("cxp: es validador: %w", err)
	}
	return ok, nil
}

// DepartamentosDeUsuario devuelve los IDs de departamento donde el usuario es validador
// (titular o suplente), limitado a departamentos activos de la empresa (tenant-safe).
func (r *pgRepository) DepartamentosDeUsuario(ctx context.Context, empresaID, usuarioID string) ([]string, error) {
	const q = `
		SELECT dv.departamento_id::text
		FROM departamento_validador dv
		JOIN departamento d ON d.id = dv.departamento_id
		WHERE dv.usuario_id = $2::uuid AND d.empresa_id = $1::uuid AND d.activo`
	rows, err := r.pool.Query(ctx, q, empresaID, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("cxp: departamentos de usuario: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0) // no-nil: "sin áreas" ≠ "ver todo"
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cxp: scan departamento de usuario: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ValidarDeptoDoc aplica la validación de área: REVISADO → VALIDADO_DEPTO, sellando quién,
// cuándo y con qué respaldo. Guard optimista por estado (0 filas = no estaba REVISADO).
func (r *pgRepository) ValidarDeptoDoc(ctx context.Context, empresaID, docID, usuarioID, respaldo, nota string) (int64, error) {
	const q = `
		UPDATE documento_cxp
		SET estado = 'VALIDADO_DEPTO', validado_depto_por = $3::uuid, validado_depto_en = now(),
		    validacion_respaldo = $4, validacion_nota = NULLIF($5, ''), actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'REVISADO'`
	tag, err := r.pool.Exec(ctx, q, empresaID, docID, usuarioID, respaldo, nota)
	if err != nil {
		return 0, fmt.Errorf("cxp: validar departamento: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DevolverDoc regresa una factura a Contabilidad para corregir/re-enrutar (REVISADO/VALIDADO_DEPTO
// → RECIBIDO), limpiando el sello de validación y registrando el motivo.
func (r *pgRepository) DevolverDoc(ctx context.Context, empresaID, docID, nota string) (int64, error) {
	const q = `
		UPDATE documento_cxp
		SET estado = 'RECIBIDO', validado_depto_por = NULL, validado_depto_en = NULL,
		    validacion_respaldo = NULL, nota_revision = NULLIF($3, ''), actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado IN ('REVISADO', 'VALIDADO_DEPTO')`
	tag, err := r.pool.Exec(ctx, q, empresaID, docID, nota)
	if err != nil {
		return 0, fmt.Errorf("cxp: devolver documento: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListarValidadores devuelve los validadores de un departamento (con nombre/email del usuario).
func (r *pgRepository) ListarValidadores(ctx context.Context, empresaID, deptoID string) ([]Validador, error) {
	const q = `
		SELECT u.id::text, u.nombre, u.email, dv.rol
		FROM departamento_validador dv
		JOIN departamento d ON d.id = dv.departamento_id
		JOIN usuario u ON u.id = dv.usuario_id
		WHERE dv.departamento_id = $2::uuid AND d.empresa_id = $1::uuid
		ORDER BY (dv.rol = 'TITULAR') DESC, u.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, deptoID)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar validadores: %w", err)
	}
	defer rows.Close()
	out := make([]Validador, 0)
	for rows.Next() {
		var v Validador
		if err := rows.Scan(&v.UsuarioID, &v.Nombre, &v.Email, &v.Rol); err != nil {
			return nil, fmt.Errorf("cxp: scan validador: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// AsignarValidador asigna (o reasigna el rol de) un validador a un departamento. Tenant-safe:
// el departamento debe ser de la empresa y el usuario debe pertenecer a la empresa.
func (r *pgRepository) AsignarValidador(ctx context.Context, empresaID, deptoID, usuarioID, rol string) error {
	const q = `
		INSERT INTO departamento_validador (departamento_id, usuario_id, rol)
		SELECT $2::uuid, $3::uuid, $4
		WHERE EXISTS (SELECT 1 FROM departamento WHERE id = $2::uuid AND empresa_id = $1::uuid)
		  AND EXISTS (SELECT 1 FROM usuario_empresa_rol WHERE usuario_id = $3::uuid AND empresa_id = $1::uuid)
		ON CONFLICT (departamento_id, usuario_id) DO UPDATE SET rol = EXCLUDED.rol`
	tag, err := r.pool.Exec(ctx, q, empresaID, deptoID, usuarioID, rol)
	if err != nil {
		return fmt.Errorf("cxp: asignar validador: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDepartamentoNoEncontrado // depto ajeno o usuario que no opera la empresa
	}
	return nil
}

// UsuariosEmpresa lista los usuarios que operan la empresa (para asignar validadores).
func (r *pgRepository) UsuariosEmpresa(ctx context.Context, empresaID string) ([]UsuarioRef, error) {
	const q = `
		SELECT u.id::text, u.nombre, u.email
		FROM usuario u
		JOIN usuario_empresa_rol uer ON uer.usuario_id = u.id
		WHERE uer.empresa_id = $1::uuid AND u.activo
		ORDER BY u.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: usuarios de empresa: %w", err)
	}
	defer rows.Close()
	out := make([]UsuarioRef, 0)
	for rows.Next() {
		var u UsuarioRef
		if err := rows.Scan(&u.ID, &u.Nombre, &u.Email); err != nil {
			return nil, fmt.Errorf("cxp: scan usuario: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// QuitarValidador desasigna un validador de un departamento (tenant-safe).
func (r *pgRepository) QuitarValidador(ctx context.Context, empresaID, deptoID, usuarioID string) error {
	const q = `
		DELETE FROM departamento_validador dv
		USING departamento d
		WHERE dv.departamento_id = d.id AND d.empresa_id = $1::uuid
		  AND dv.departamento_id = $2::uuid AND dv.usuario_id = $3::uuid`
	if _, err := r.pool.Exec(ctx, q, empresaID, deptoID, usuarioID); err != nil {
		return fmt.Errorf("cxp: quitar validador: %w", err)
	}
	return nil
}
