package nomina

// Consultas de las notificaciones de RRHH. Filtran por empresa_id.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CorreosEmpleados devuelve empleado_id → correo de la empresa. Solo los que tienen correo:
// así el servicio distingue «no tiene» de «falló el envío».
func (r *pgRepository) CorreosEmpleados(ctx context.Context, empresaID string) (map[string]string, error) {
	const q = `
		SELECT id::text, email FROM empleado
		WHERE empresa_id = $1::uuid AND COALESCE(email, '') <> ''`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("nomina: correos de empleados: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, email string
		if err := rows.Scan(&id, &email); err != nil {
			return nil, fmt.Errorf("nomina: scan correo: %w", err)
		}
		out[id] = email
	}
	return out, rows.Err()
}

// VacacionParaAviso trae el disfrute con los datos del empleado para el correo.
//
// NO calcula el saldo: eso lo pone el servicio con `SaldoVacacionesEmpleado`, que ya deriva el
// acumulado con los días por mes de los parámetros del año. Una segunda fórmula del saldo (que
// es un derecho laboral) sería una fuente de verdad paralela.
func (r *pgRepository) VacacionParaAviso(ctx context.Context, empresaID, vacacionID string) (VacacionAviso, error) {
	const q = `
		SELECT e.id::text, e.nombre, COALESCE(e.email, ''),
		       v.dias::text,
		       to_char(v.fecha_inicio, 'YYYY-MM-DD'),
		       to_char(v.fecha_inicio + ((v.dias - 1) || ' day')::interval, 'YYYY-MM-DD'),
		       COALESCE(v.observaciones, '')
		FROM vacacion v
		JOIN empleado e ON e.id = v.empleado_id
		WHERE v.empresa_id = $1::uuid AND v.id = $2::uuid AND NOT v.anulada`
	var a VacacionAviso
	err := r.pool.QueryRow(ctx, q, empresaID, vacacionID).Scan(
		&a.EmpleadoID, &a.Nombre, &a.Email, &a.Dias, &a.FechaInicio, &a.FechaFin, &a.Observaciones)
	if errors.Is(err, pgx.ErrNoRows) {
		return VacacionAviso{}, ErrVacacionNoEncontrada
	}
	if err != nil {
		return VacacionAviso{}, fmt.Errorf("nomina: vacación para aviso: %w", err)
	}
	return a, nil
}
