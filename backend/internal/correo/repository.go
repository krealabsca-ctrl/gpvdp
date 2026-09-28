package correo

// Acceso a datos de `correo_saliente` (mig 0084). Una fila por empresa, y TODA consulta filtra por
// el empresa_id que le pasan —que es siempre el del contexto de la petición, nunca uno del cuerpo—.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository abstrae el acceso a datos del correo saliente.
type Repository interface {
	// Obtener devuelve la fila de la empresa. ErrConfigNoEncontrada si no tiene.
	Obtener(ctx context.Context, empresaID string) (Guardado, error)
	// Guardar hace el upsert (el UNIQUE por empresa_id es lo que lo vuelve un solo camino).
	// `passwordCifrada` ya viene cifrada por el servicio: acá no se cifra nada.
	Guardar(ctx context.Context, empresaID string, g Guardado, passwordCifrada, usuarioID string) error
	// MarcarPrueba guarda el resultado de la última prueba. `detalle` vacío = salió bien.
	MarcarPrueba(ctx context.Context, empresaID, detalle string) error
	// CorreoUsuario devuelve el nombre y el correo de un usuario (destino de la prueba).
	CorreoUsuario(ctx context.Context, usuarioID string) (nombre, email string, err error)
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository crea el repositorio respaldado por PostgreSQL.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

func (r *pgRepository) Obtener(ctx context.Context, empresaID string) (Guardado, error) {
	const q = `
		SELECT cs.host, cs.puerto, cs.seguridad, cs.usuario, cs.password_cifrada,
		       cs.remitente, cs.remitente_nombre, cs.activo,
		       cs.probado_en, COALESCE(cs.probado_error, ''),
		       COALESCE(u.nombre, ''), cs.actualizado_en
		FROM correo_saliente cs
		LEFT JOIN usuario u ON u.id = cs.actualizado_por
		WHERE cs.empresa_id = $1::uuid`
	var g Guardado
	err := r.pool.QueryRow(ctx, q, empresaID).Scan(&g.Host, &g.Puerto, &g.Seguridad, &g.Usuario,
		&g.PasswordCifrada, &g.Remitente, &g.RemitenteNombre, &g.Activo,
		&g.ProbadoEn, &g.ProbadoError, &g.ActualizadoPor, &g.ActualizadoEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return Guardado{}, ErrConfigNoEncontrada
	}
	if err != nil {
		return Guardado{}, fmt.Errorf("correo: obtener configuración: %w", err)
	}
	return g, nil
}

// Guardar escribe la fila completa.
//
// `probado_en` y `probado_error` se ponen en NULL a propósito: la prueba anterior describía OTRA
// configuración, y dejarla puesta haría que la pantalla mostrara un «probado y funcionando» que ya
// no significa nada. Después de guardar hay que volver a probar.
func (r *pgRepository) Guardar(ctx context.Context, empresaID string, g Guardado, passwordCifrada, usuarioID string) error {
	const q = `
		INSERT INTO correo_saliente (empresa_id, host, puerto, seguridad, usuario, password_cifrada,
			remitente, remitente_nombre, activo, actualizado_por, actualizado_en)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, '')::uuid, now())
		ON CONFLICT (empresa_id) DO UPDATE SET
			host = EXCLUDED.host, puerto = EXCLUDED.puerto, seguridad = EXCLUDED.seguridad,
			usuario = EXCLUDED.usuario, password_cifrada = EXCLUDED.password_cifrada,
			remitente = EXCLUDED.remitente, remitente_nombre = EXCLUDED.remitente_nombre,
			activo = EXCLUDED.activo, actualizado_por = EXCLUDED.actualizado_por,
			actualizado_en = now(), probado_en = NULL, probado_error = NULL`
	_, err := r.pool.Exec(ctx, q, empresaID, g.Host, g.Puerto, g.Seguridad, g.Usuario, passwordCifrada,
		g.Remitente, g.RemitenteNombre, g.Activo, usuarioID)
	if err != nil {
		return fmt.Errorf("correo: guardar configuración: %w", err)
	}
	return nil
}

func (r *pgRepository) MarcarPrueba(ctx context.Context, empresaID, detalle string) error {
	const q = `UPDATE correo_saliente SET probado_en = now(), probado_error = NULLIF($2, '')
	           WHERE empresa_id = $1::uuid`
	if _, err := r.pool.Exec(ctx, q, empresaID, detalle); err != nil {
		return fmt.Errorf("correo: marcar prueba: %w", err)
	}
	return nil
}

func (r *pgRepository) CorreoUsuario(ctx context.Context, usuarioID string) (string, string, error) {
	const q = `SELECT nombre, email FROM usuario WHERE id = $1::uuid`
	var nombre, email string
	err := r.pool.QueryRow(ctx, q, usuarioID).Scan(&nombre, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrSinCorreoDePrueba
	}
	if err != nil {
		return "", "", fmt.Errorf("correo: usuario de la prueba: %w", err)
	}
	return nombre, email, nil
}
