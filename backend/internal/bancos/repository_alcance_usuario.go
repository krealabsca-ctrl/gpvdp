package bancos

// Las escrituras del alcance por persona. El porqué está en alcance_usuario.go.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// UsuarioDeLaEmpresa devuelve nombre y correo si la persona tiene acceso a esa empresa.
//
// ES LA GUARDA DE AISLAMIENTO de este archivo: `departamento_validador` no tiene `empresa_id`, así
// que lo único que impide darle un área de una empresa a alguien de otra es esta comprobación.
func (r *pgRepository) UsuarioDeLaEmpresa(ctx context.Context, empresaID, usuarioID string) (string, string, bool, error) {
	const q = `
		SELECT u.nombre, u.email
		  FROM usuario u
		  JOIN usuario_empresa_rol uer ON uer.usuario_id = u.id
		 WHERE u.id = $2::uuid
		   AND uer.empresa_id = $1::uuid
		 LIMIT 1`
	var nombre, email string
	err := r.pool.QueryRow(ctx, q, empresaID, usuarioID).Scan(&nombre, &email)
	if err == pgx.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("bancos: usuario de la empresa: %w", err)
	}
	return nombre, email, true, nil
}

// PermisosDeAlcanceDelUsuario dice si la persona puede abrir cada una de las dos pantallas.
//
// Mira los permisos EFECTIVOS —los que le llegan por sus roles en esta empresa—, no los de un rol
// suelto: alguien con dos roles ve la pantalla si cualquiera de los dos se la da.
func (r *pgRepository) PermisosDeAlcanceDelUsuario(ctx context.Context, empresaID, usuarioID string) (bool, bool, error) {
	const q = `
		SELECT
		  bool_or(p.codigo = 'bancos.ver_mi_presupuesto'),
		  bool_or(p.codigo = 'bancos.ver_mi_ingreso')
		  FROM usuario_empresa_rol uer
		  JOIN rol_permiso rp ON rp.rol_id = uer.rol_id AND rp.empresa_id = uer.empresa_id
		  JOIN permiso p ON p.id = rp.permiso_id
		 WHERE uer.usuario_id = $2::uuid
		   AND uer.empresa_id = $1::uuid`
	var presupuesto, ingreso *bool
	if err := r.pool.QueryRow(ctx, q, empresaID, usuarioID).Scan(&presupuesto, &ingreso); err != nil {
		return false, false, fmt.Errorf("bancos: permisos de alcance: %w", err)
	}
	// Sin ninguna fila, `bool_or` devuelve NULL: la persona no tiene rol con permisos acá.
	return presupuesto != nil && *presupuesto, ingreso != nil && *ingreso, nil
}

// GuardarAreasDelUsuario reemplaza las áreas de la persona, en una sola transacción.
//
// REEMPLAZA en vez de sumar porque la pantalla muestra la lista completa y la manda completa: un
// «agregar» dejaría sin forma de quitar, y dos llamadas (quitar + agregar) pueden partirse a la
// mitad y dejar a alguien sin su área.
//
// El DELETE filtra por empresa a través del departamento: si no, borraría las áreas que esa misma
// persona tiene en OTRA empresa, que no son asunto de esta pantalla.
func (r *pgRepository) GuardarAreasDelUsuario(ctx context.Context, empresaID, usuarioID string, areas []AreaAsignada) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bancos: abrir transacción de áreas: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const borrar = `
		DELETE FROM departamento_validador dv
		 USING departamento d
		 WHERE d.id = dv.departamento_id
		   AND d.empresa_id = $1::uuid
		   AND dv.usuario_id = $2::uuid`
	if _, err := tx.Exec(ctx, borrar, empresaID, usuarioID); err != nil {
		return fmt.Errorf("bancos: limpiar áreas del usuario: %w", err)
	}

	// El INSERT valida la empresa POR FILA con el SELECT: un departamento ajeno no inserta nada en
	// vez de insertar mal. El servicio ya lo comprobó; esto es la segunda llave de la misma puerta.
	const insertar = `
		INSERT INTO departamento_validador (departamento_id, usuario_id, rol)
		SELECT d.id, $2::uuid, $3
		  FROM departamento d
		 WHERE d.id = $4::uuid
		   AND d.empresa_id = $1::uuid
		ON CONFLICT (departamento_id, usuario_id) DO UPDATE SET rol = EXCLUDED.rol`
	for _, a := range areas {
		ct, err := tx.Exec(ctx, insertar, empresaID, usuarioID, a.Rol, a.DepartamentoID)
		if err != nil {
			return fmt.Errorf("bancos: asignar área: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return ErrAreaDesconocida
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("bancos: confirmar áreas del usuario: %w", err)
	}
	return nil
}

// GuardarPartidasDeIngresoDelUsuario reemplaza las partidas de ingreso de la persona.
//
// Misma forma que las áreas, con una diferencia: acá la tabla SÍ tiene `empresa_id`, así que el
// filtro es directo. Aun así el INSERT comprueba la empresa de la clasificación: la columna dice
// de quién es la fila, no de quién es la partida.
func (r *pgRepository) GuardarPartidasDeIngresoDelUsuario(ctx context.Context, empresaID, usuarioID string, clasificacionIDs []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bancos: abrir transacción de partidas: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const borrar = `
		DELETE FROM usuario_clasificacion_consulta
		 WHERE empresa_id = $1::uuid AND usuario_id = $2::uuid`
	if _, err := tx.Exec(ctx, borrar, empresaID, usuarioID); err != nil {
		return fmt.Errorf("bancos: limpiar partidas del usuario: %w", err)
	}

	const insertar = `
		INSERT INTO usuario_clasificacion_consulta (empresa_id, usuario_id, clasificacion_id)
		SELECT $1::uuid, $2::uuid, cl.id
		  FROM clasificacion cl
		 WHERE cl.id = $3::uuid
		   AND cl.empresa_id = $1::uuid
		ON CONFLICT DO NOTHING`
	for _, id := range clasificacionIDs {
		ct, err := tx.Exec(ctx, insertar, empresaID, usuarioID, id)
		if err != nil {
			return fmt.Errorf("bancos: asignar partida: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return ErrPartidaDesconocida
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("bancos: confirmar partidas del usuario: %w", err)
	}
	return nil
}

// NaturalezasDeClasificaciones devuelve la naturaleza del concepto de cada partida de la empresa.
//
// En un solo viaje: con 168 clasificaciones, preguntar una por una sería una consulta por fila, que
// es justo el patrón que ya costó 557 ms en el mapeo contable.
func (r *pgRepository) NaturalezasDeClasificaciones(ctx context.Context, empresaID string, ids []string) (map[string]string, error) {
	const q = `
		SELECT cl.id::text, co.naturaleza
		  FROM clasificacion cl
		  JOIN concepto co ON co.id = cl.concepto_id
		 WHERE cl.empresa_id = $1::uuid
		   AND cl.id = ANY($2::uuid[])`
	rows, err := r.pool.Query(ctx, q, empresaID, ids)
	if err != nil {
		return nil, fmt.Errorf("bancos: naturalezas de clasificaciones: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string, len(ids))
	for rows.Next() {
		var id, nat string
		if err := rows.Scan(&id, &nat); err != nil {
			return nil, fmt.Errorf("bancos: scan naturaleza: %w", err)
		}
		out[id] = nat
	}
	return out, rows.Err()
}
