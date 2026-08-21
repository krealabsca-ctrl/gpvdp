package bancos

// Fase A — motor que aprende: sugerencia de reglas, edición de reglas,
// clasificación masiva y resumen para el KPI de auto-clasificación.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *pgRepository) MovimientoClasif(ctx context.Context, empresaID, movID string) (MovClasifActual, error) {
	const q = `
		SELECT m.id::text, COALESCE(m.descripcion,''), (m.debito > 0),
		       COALESCE(m.concepto_id::text,''), COALESCE(co.nombre,''),
		       COALESCE(m.clasificacion_id::text,''), COALESCE(cl.nombre,'')
		FROM movimiento_bancario m
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		WHERE m.empresa_id = $1::uuid AND m.id = $2::uuid`
	var m MovClasifActual
	err := r.pool.QueryRow(ctx, q, empresaID, movID).Scan(&m.ID, &m.Descripcion, &m.EsDebito,
		&m.ConceptoID, &m.Concepto, &m.ClasificacionID, &m.Clasificacion)
	if errors.Is(err, pgx.ErrNoRows) {
		return MovClasifActual{}, ErrMovimientoNoEncontrado
	}
	if err != nil {
		return MovClasifActual{}, fmt.Errorf("bancos: movimiento para sugerencia: %w", err)
	}
	return m, nil
}

func (r *pgRepository) ContarNoIdentificadosConPalabra(ctx context.Context, empresaID, palabra, aplicaA string) (int, error) {
	// translate() quita tildes en ambos lados para contar igual que el matcher Go
	// (norm), que es insensible a acentos: el banner promete lo que la regla aplicará.
	const q = `
		SELECT COUNT(*) FROM movimiento_bancario
		WHERE empresa_id = $1::uuid AND estado_clasificacion = 'NO_IDENTIFICADO'
		  AND translate(upper(descripcion), 'ÁÉÍÓÚÑÜ', 'AEIOUNU')
		      LIKE '%' || translate(upper($2), 'ÁÉÍÓÚÑÜ', 'AEIOUNU') || '%'
		  AND (CASE $3 WHEN 'DEBITO' THEN debito > 0 WHEN 'CREDITO' THEN credito > 0 ELSE true END)`
	var n int
	if err := r.pool.QueryRow(ctx, q, empresaID, palabra, aplicaA).Scan(&n); err != nil {
		return 0, fmt.Errorf("bancos: contar similares: %w", err)
	}
	return n, nil
}

func (r *pgRepository) ExisteReglaConPalabra(ctx context.Context, empresaID, palabra string) (bool, error) {
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM palabra_clave p
			JOIN regla_clasificacion r ON r.id = p.regla_id
			WHERE r.empresa_id = $1::uuid AND lower(p.texto) = lower($2))`
	var existe bool
	if err := r.pool.QueryRow(ctx, q, empresaID, palabra).Scan(&existe); err != nil {
		return false, fmt.Errorf("bancos: regla con palabra: %w", err)
	}
	return existe, nil
}

func (r *pgRepository) ActualizarRegla(ctx context.Context, empresaID, reglaID string, cambios CambiosRegla) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bancos: begin tx editar regla: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existe bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM regla_clasificacion WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
		empresaID, reglaID).Scan(&existe); err != nil {
		return fmt.Errorf("bancos: verificar regla: %w", err)
	}
	if !existe {
		return ErrReglaNoEncontrada
	}

	if cambios.Prioridad != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE regla_clasificacion SET prioridad = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid`,
			empresaID, reglaID, *cambios.Prioridad); err != nil {
			return fmt.Errorf("bancos: cambiar prioridad: %w", err)
		}
	}
	if cambios.Activo != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE regla_clasificacion SET activo = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid`,
			empresaID, reglaID, *cambios.Activo); err != nil {
			return fmt.Errorf("bancos: pausar/activar regla: %w", err)
		}
	}
	for _, p := range cambios.AgregarPalabras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO palabra_clave (regla_id, texto)
			SELECT $1::uuid, $2
			WHERE NOT EXISTS (SELECT 1 FROM palabra_clave WHERE regla_id = $1::uuid AND lower(texto) = lower($2))`,
			reglaID, p); err != nil {
			return fmt.Errorf("bancos: agregar palabra: %w", err)
		}
	}
	if len(cambios.QuitarPalabras) > 0 {
		if _, err := tx.Exec(ctx,
			`DELETE FROM palabra_clave WHERE regla_id = $1::uuid AND lower(texto) = ANY(SELECT lower(unnest($2::text[])))`,
			reglaID, cambios.QuitarPalabras); err != nil {
			return fmt.Errorf("bancos: quitar palabras: %w", err)
		}
		// Una regla sin palabras clave no clasifica nada: se exige conservar al menos una.
		var quedan int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM palabra_clave WHERE regla_id = $1::uuid`, reglaID).Scan(&quedan); err != nil {
			return fmt.Errorf("bancos: contar palabras: %w", err)
		}
		if quedan == 0 {
			return ErrReglaSinPalabras
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("bancos: commit editar regla: %w", err)
	}
	return nil
}

func (r *pgRepository) EliminarRegla(ctx context.Context, empresaID, reglaID string) error {
	// Borra la regla y sus palabras clave (CASCADE). Los movimientos ya clasificados
	// por ella no se tocan (la clasificación aplicada es un hecho histórico).
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM regla_clasificacion WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, reglaID)
	if err != nil {
		return fmt.Errorf("bancos: eliminar regla: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrReglaNoEncontrada
	}
	return nil
}

func (r *pgRepository) ClasificarMasivo(ctx context.Context, empresaID string, movIDs []string, conceptoID, clasificacionID string) (int, error) {
	// Un solo UPDATE para todo el bloque; queda REVISADO (decisión humana, confianza 100).
	// EXISTS asegura que la clasificación pertenezca a la empresa y al concepto (tenant-safe).
	// es_traslado: un par emparejado SIEMPRE es traslado; sin par, decide el concepto.
	q := `
		UPDATE movimiento_bancario
		SET concepto_id = $3::uuid, clasificacion_id = $4::uuid,
		    estado_clasificacion = 'REVISADO', confianza = 100,
		    es_traslado = ` + sqlEsTrasladoDerivado("", "$3::uuid", "$1::uuid") + `,
		    actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = ANY($2::uuid[])
		  AND EXISTS (SELECT 1 FROM clasificacion
		              WHERE id = $4::uuid AND empresa_id = $1::uuid AND concepto_id = $3::uuid)`
	tag, err := r.pool.Exec(ctx, q, empresaID, movIDs, conceptoID, clasificacionID)
	if err != nil {
		return 0, fmt.Errorf("bancos: clasificar masivo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Distinguir clasificación inválida (422) de ids inexistentes (0 filas, sin error).
		var valida bool
		if e := r.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM clasificacion WHERE id = $2::uuid AND empresa_id = $1::uuid AND concepto_id = $3::uuid)`,
			empresaID, clasificacionID, conceptoID).Scan(&valida); e == nil && !valida {
			return 0, ErrClasificacionInvalida
		}
	}
	return int(tag.RowsAffected()), nil
}

func (r *pgRepository) ResumenClasificacion(ctx context.Context, empresaID, periodo string) (ResumenClasif, error) {
	conds := "empresa_id = $1::uuid"
	args := []any{empresaID}
	if periodo != "" {
		conds += " AND to_char(fecha, 'YYYY-MM') = $2"
		args = append(args, periodo)
	}
	q := `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE estado_clasificacion = 'NO_IDENTIFICADO'),
		       COUNT(*) FILTER (WHERE estado_clasificacion = 'AUTO'),
		       COUNT(*) FILTER (WHERE estado_clasificacion = 'REVISADO'),
		       COUNT(*) FILTER (WHERE es_traslado)
		FROM movimiento_bancario WHERE ` + conds
	var res ResumenClasif
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&res.Total, &res.NoIdentificados, &res.Auto, &res.Revisados, &res.Traslados); err != nil {
		return ResumenClasif{}, fmt.Errorf("bancos: resumen clasificación: %w", err)
	}
	return res, nil
}
