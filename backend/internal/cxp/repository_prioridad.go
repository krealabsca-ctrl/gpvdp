package cxp

// El catálogo de prioridades contra la base (mig 0121). El porqué está en prioridad.go.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Prioridades lista el catálogo de la empresa, de la más alta a la más baja.
//
// `en_uso` se cuenta en la misma consulta para que la pantalla pueda decir por qué una prioridad no
// se puede eliminar ANTES de que alguien lo intente. Es una subconsulta por fila sobre una tabla de
// a lo sumo una docena: el costo es irrelevante y evita un segundo viaje.
func (r *pgRepository) Prioridades(ctx context.Context, empresaID string, soloActivas bool) ([]Prioridad, error) {
	q := `
		SELECT p.id::text, p.codigo, p.nombre, p.orden, p.urgente, p.tono, p.activa,
		       (SELECT count(*)::int FROM documento_cxp d
		         WHERE d.empresa_id = p.empresa_id AND d.prioridad = p.codigo)
		  FROM prioridad_cxp p
		 WHERE p.empresa_id = $1::uuid`
	if soloActivas {
		q += ` AND p.activa`
	}
	q += ` ORDER BY p.orden DESC, p.codigo`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar prioridades: %w", err)
	}
	defer rows.Close()
	out := make([]Prioridad, 0, 8)
	for rows.Next() {
		var p Prioridad
		if err := rows.Scan(&p.ID, &p.Codigo, &p.Nombre, &p.Orden, &p.Urgente, &p.Tono,
			&p.Activa, &p.EnUso); err != nil {
			return nil, fmt.Errorf("cxp: scan prioridad: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CrearPrioridad agrega una al catálogo de la empresa.
func (r *pgRepository) CrearPrioridad(ctx context.Context, empresaID string, in PrioridadInput) (Prioridad, error) {
	const q = `
		INSERT INTO prioridad_cxp (empresa_id, codigo, nombre, orden, urgente, tono, activa)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, codigo, nombre, orden, urgente, tono, activa`
	var p Prioridad
	err := r.pool.QueryRow(ctx, q, empresaID, in.Codigo, in.Nombre, in.Orden, in.Urgente,
		in.Tono, in.Activa).
		Scan(&p.ID, &p.Codigo, &p.Nombre, &p.Orden, &p.Urgente, &p.Tono, &p.Activa)
	if err != nil {
		// 23505 es el índice único (empresa, código). Se traduce acá y no se comprueba antes con un
		// SELECT porque entre la comprobación y el INSERT cabe otra creación: la base es la única
		// que puede responderlo sin ventana.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Prioridad{}, ErrCodigoPrioridadRepetido
		}
		return Prioridad{}, fmt.Errorf("cxp: crear prioridad: %w", err)
	}
	return p, nil
}

// ActualizarPrioridad edita una existente.
//
// EL CÓDIGO NO SE CAMBIA ACÁ, a propósito: está escrito en las facturas que ya lo llevan, y
// renombrarlo las dejaría apuntando a un código que ya no existe en el catálogo. Si hace falta
// otro código, se crea una prioridad nueva y se desactiva la vieja.
func (r *pgRepository) ActualizarPrioridad(ctx context.Context, empresaID, id string, in PrioridadInput) (Prioridad, error) {
	const q = `
		UPDATE prioridad_cxp
		   SET nombre = $3, orden = $4, urgente = $5, tono = $6, activa = $7,
		       actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid
		RETURNING id::text, codigo, nombre, orden, urgente, tono, activa`
	var p Prioridad
	err := r.pool.QueryRow(ctx, q, empresaID, id, in.Nombre, in.Orden, in.Urgente, in.Tono, in.Activa).
		Scan(&p.ID, &p.Codigo, &p.Nombre, &p.Orden, &p.Urgente, &p.Tono, &p.Activa)
	if errors.Is(err, pgx.ErrNoRows) {
		return Prioridad{}, ErrPrioridadNoEncontrada
	}
	if err != nil {
		return Prioridad{}, fmt.Errorf("cxp: actualizar prioridad: %w", err)
	}
	return p, nil
}

// EliminarPrioridad la borra SOLO si no está puesta en ninguna factura.
//
// Si cuelga algo se desactiva, nunca se borra: es el mismo criterio del resto del catálogo. Una
// prioridad borrada dejaría facturas con un código que la pantalla ya no sabe nombrar ni colorear.
func (r *pgRepository) EliminarPrioridad(ctx context.Context, empresaID, id string) error {
	const q = `
		DELETE FROM prioridad_cxp p
		 WHERE p.empresa_id = $1::uuid AND p.id = $2::uuid
		   AND NOT EXISTS (SELECT 1 FROM documento_cxp d
		                    WHERE d.empresa_id = p.empresa_id AND d.prioridad = p.codigo)`
	tag, err := r.pool.Exec(ctx, q, empresaID, id)
	if err != nil {
		return fmt.Errorf("cxp: eliminar prioridad: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return r.porQueNoSeElimino(ctx, empresaID, id)
	}
	return nil
}

// porQueNoSeElimino distingue «no existe» de «está en uso»: el arreglo es distinto, y un «no se
// pudo» a secas manda a adivinar cuál de los dos es.
func (r *pgRepository) porQueNoSeElimino(ctx context.Context, empresaID, id string) error {
	const q = `
		SELECT EXISTS (SELECT 1 FROM documento_cxp d
		                JOIN prioridad_cxp p ON p.codigo = d.prioridad AND p.empresa_id = d.empresa_id
		               WHERE p.empresa_id = $1::uuid AND p.id = $2::uuid)`
	var enUso bool
	if err := r.pool.QueryRow(ctx, q, empresaID, id).Scan(&enUso); err == nil && enUso {
		return ErrPrioridadEnUso
	}
	return ErrPrioridadNoEncontrada
}

// CodigosDePrioridadValidos devuelve los códigos ACTIVOS de la empresa.
//
// Lo usa la validación al marcar una factura: con el CHECK fuera (mig 0121), es acá donde se
// comprueba que el código existe. Devolver la lista —y no solo un sí/no— deja que el error diga
// cuáles son las válidas, que es lo que un CHECK nunca pudo responder.
func (r *pgRepository) CodigosDePrioridadValidos(ctx context.Context, empresaID string) ([]string, error) {
	const q = `SELECT codigo FROM prioridad_cxp WHERE empresa_id = $1::uuid AND activa ORDER BY orden DESC`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: códigos de prioridad: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0, 8)
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("cxp: scan código de prioridad: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
