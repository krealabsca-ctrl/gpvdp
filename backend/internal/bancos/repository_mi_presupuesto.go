package bancos

// El alcance de «Mi presupuesto». El porqué está en mi_presupuesto.go.

import (
	"context"
	"fmt"
)

// AreasDelUsuario devuelve las áreas de las que la persona es responsable.
//
// Lee `departamento_validador`, que ya existe y la usa CxP para la validación de área: una segunda
// tabla para lo mismo obligaría a mantener dos listas en sincronía.
//
// Filtra por empresa a través del departamento —`departamento_validador` no tiene `empresa_id`—,
// y también deja fuera los departamentos inactivos: un área que se cerró no tiene presupuesto que
// mirar, y seguir mostrándola haría creer que el dato está vacío cuando en realidad no aplica.
func (r *pgRepository) AreasDelUsuario(ctx context.Context, empresaID, usuarioID string) ([]AreaDelUsuario, error) {
	const q = `
		SELECT d.id::text, d.nombre, dv.rol
		  FROM departamento_validador dv
		  JOIN departamento d ON d.id = dv.departamento_id
		 WHERE d.empresa_id = $1::uuid AND d.activo
		   AND dv.usuario_id = $2::uuid
		 GROUP BY d.id, d.nombre, dv.rol
		 ORDER BY d.nombre, dv.rol`
	rows, err := r.pool.Query(ctx, q, empresaID, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("bancos: áreas del usuario: %w", err)
	}
	defer rows.Close()

	out := make([]AreaDelUsuario, 0, 4)
	for rows.Next() {
		var a AreaDelUsuario
		if err := rows.Scan(&a.DepartamentoID, &a.Departamento, &a.Rol); err != nil {
			return nil, fmt.Errorf("bancos: scan área del usuario: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
