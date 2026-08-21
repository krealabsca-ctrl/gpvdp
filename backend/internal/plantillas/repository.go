package plantillas

// Acceso a datos de las plantillas. Filtra por empresa_id como todo el ERP.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository es el acceso a datos de las plantillas.
type Repository interface {
	// Listar trae las plantillas guardadas de la empresa (las que NO estén, van por defecto).
	Listar(ctx context.Context, empresaID string) (map[string]Plantilla, error)
	Guardar(ctx context.Context, empresaID, clave, asunto, cuerpo, usuarioID string) error
	Restablecer(ctx context.Context, empresaID, clave string) error
	// NombreEmpresa alimenta [NOMBRE_EMPRESA] sin que cada módulo tenga que acordarse.
	NombreEmpresa(ctx context.Context, empresaID string) (string, error)
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository construye el repositorio con el pool de conexiones.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

func (r *pgRepository) Listar(ctx context.Context, empresaID string) (map[string]Plantilla, error) {
	const q = `
		SELECT p.clave, p.asunto, p.cuerpo,
		       to_char(p.actualizado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       COALESCE(NULLIF(u.nombre, ''), u.email, '')
		FROM plantilla_correo p
		LEFT JOIN usuario u ON u.id = p.actualizado_por
		WHERE p.empresa_id = $1::uuid`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("plantillas: listar: %w", err)
	}
	defer rows.Close()
	out := map[string]Plantilla{}
	for rows.Next() {
		var p Plantilla
		if err := rows.Scan(&p.Clave, &p.Asunto, &p.Cuerpo, &p.ActualizadoEn, &p.ActualizadoPor); err != nil {
			return nil, fmt.Errorf("plantillas: scan: %w", err)
		}
		p.Personalizada = true
		out[p.Clave] = p
	}
	return out, rows.Err()
}

func (r *pgRepository) Guardar(ctx context.Context, empresaID, clave, asunto, cuerpo, usuarioID string) error {
	const q = `
		INSERT INTO plantilla_correo (empresa_id, clave, asunto, cuerpo, actualizado_por)
		VALUES ($1::uuid, $2, $3, $4, $5::uuid)
		ON CONFLICT (empresa_id, clave) DO UPDATE
		   SET asunto = EXCLUDED.asunto, cuerpo = EXCLUDED.cuerpo,
		       actualizado_por = EXCLUDED.actualizado_por, actualizado_en = now()`
	if _, err := r.pool.Exec(ctx, q, empresaID, clave, asunto, cuerpo, usuarioID); err != nil {
		return fmt.Errorf("plantillas: guardar: %w", err)
	}
	return nil
}

func (r *pgRepository) NombreEmpresa(ctx context.Context, empresaID string) (string, error) {
	var nombre string
	err := r.pool.QueryRow(ctx, `SELECT nombre FROM empresa WHERE id = $1::uuid`, empresaID).Scan(&nombre)
	if err != nil {
		return "", fmt.Errorf("plantillas: nombre de la empresa: %w", err)
	}
	return nombre, nil
}

// Restablecer borra la personalización: vuelve a regir el texto de fábrica.
func (r *pgRepository) Restablecer(ctx context.Context, empresaID, clave string) error {
	const q = `DELETE FROM plantilla_correo WHERE empresa_id = $1::uuid AND clave = $2`
	if _, err := r.pool.Exec(ctx, q, empresaID, clave); err != nil {
		return fmt.Errorf("plantillas: restablecer: %w", err)
	}
	return nil
}
