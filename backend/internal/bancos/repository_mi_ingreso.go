package bancos

// El alcance de la mitad de ingreso. El porqué está en mi_ingreso.go.

import (
	"context"
	"fmt"
)

// PartidasDeIngresoDelUsuario devuelve las partidas asignadas a la persona.
//
// Filtra por empresa en la propia tabla Y exige que la partida siga activa: una partida retirada
// del catálogo no tiene meta que mirar, y seguir mostrándola haría creer que el dato está vacío
// cuando en realidad ya no aplica.
func (r *pgRepository) PartidasDeIngresoDelUsuario(ctx context.Context, empresaID, usuarioID string) ([]PartidaDelUsuario, error) {
	const q = `
		SELECT cl.id::text, cl.nombre, co.nombre
		  FROM usuario_clasificacion_consulta ucc
		  JOIN clasificacion cl ON cl.id = ucc.clasificacion_id
		  JOIN concepto co ON co.id = cl.concepto_id
		 WHERE ucc.empresa_id = $1::uuid
		   AND ucc.usuario_id = $2::uuid
		   AND cl.activo
		 ORDER BY co.nombre, cl.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("bancos: partidas de ingreso del usuario: %w", err)
	}
	defer rows.Close()

	out := make([]PartidaDelUsuario, 0, 4)
	for rows.Next() {
		var p PartidaDelUsuario
		if err := rows.Scan(&p.ClasificacionID, &p.Clasificacion, &p.Concepto); err != nil {
			return nil, fmt.Errorf("bancos: scan partida del usuario: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
