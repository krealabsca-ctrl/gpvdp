package sistema

// Lectura del contador de consultas de PostgreSQL. El porqué está en rendimiento.go.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgRepositorioRendimiento struct{ pool *pgxpool.Pool }

// NewRepositoryRendimiento construye el lector del contador.
func NewRepositoryRendimiento(pool *pgxpool.Pool) RepositoryRendimiento {
	return &pgRepositorioRendimiento{pool: pool}
}

// extensionActiva dice si `pg_stat_statements` está instalada EN ESTA BASE.
//
// Se pregunta antes de consultar y no se deduce del error: un fallo al leer la vista puede ser
// falta de permisos, un motor sin la librería o cualquier otra cosa, y los tres necesitan mensajes
// distintos. Preguntar primero deja el «no está instalada» como un hecho y no como una suposición
// sobre el texto de un error.
func (r *pgRepositorioRendimiento) extensionActiva(ctx context.Context) (bool, error) {
	const q = `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`
	var existe bool
	if err := r.pool.QueryRow(ctx, q).Scan(&existe); err != nil {
		return false, fmt.Errorf("sistema: comprobar pg_stat_statements: %w", err)
	}
	return existe, nil
}

// ConsultasLentas devuelve el top por TIEMPO TOTAL acumulado.
//
// Se excluyen las consultas de la propia extensión y las del healthcheck: aparecen entre las más
// llamadas de cualquier sistema y no se pueden arreglar, así que solo empujan hacia abajo lo que
// sí importa.
func (r *pgRepositorioRendimiento) ConsultasLentas(ctx context.Context, limite int) ([]ConsultaLenta, string, error) {
	activa, err := r.extensionActiva(ctx)
	if err != nil {
		return nil, "", err
	}
	if !activa {
		return nil, "", ErrRendimientoNoDisponible
	}

	// `stats_since` existe desde PostgreSQL 17; en 16 no está y la fecha se resuelve aparte. Se
	// consulta por separado para que la falta de esa columna no tumbe la lista entera.
	desde := r.midiendoDesde(ctx)

	const q = `
		SELECT left(regexp_replace(query, '\s+', ' ', 'g'), $2),
		       calls,
		       round(total_exec_time::numeric, 2)::float8,
		       round(mean_exec_time::numeric, 2)::float8,
		       round(max_exec_time::numeric, 2)::float8,
		       round((total_exec_time * 100 / NULLIF(SUM(total_exec_time) OVER (), 0))::numeric, 2)::float8,
		       round((rows::numeric / NULLIF(calls, 0)), 1)::float8
		  FROM pg_stat_statements
		 WHERE query NOT LIKE '%pg_stat_statements%'
		   AND query NOT LIKE '%pg_extension%'
		 ORDER BY total_exec_time DESC
		 LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limite, largoConsultaVisible)
	if err != nil {
		return nil, "", fmt.Errorf("sistema: leer consultas lentas: %w", err)
	}
	defer rows.Close()

	out := make([]ConsultaLenta, 0, limite)
	for rows.Next() {
		var c ConsultaLenta
		if err := rows.Scan(&c.Consulta, &c.Llamadas, &c.MsTotal, &c.MsMedia, &c.MsMax,
			&c.PorcentajeDelTotal, &c.FilasPorLlamada); err != nil {
			return nil, "", fmt.Errorf("sistema: scan consulta lenta: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	return out, desde, nil
}

// midiendoDesde resuelve desde cuándo se acumulan los contadores.
//
// Sin esa fecha los totales no se pueden interpretar: 40 segundos acumulados en una hora es un
// problema y en tres semanas no es nada. Devuelve "" si no se puede saber —no es motivo para
// esconder la lista—, y la pantalla lo dice.
func (r *pgRepositorioRendimiento) midiendoDesde(ctx context.Context) string {
	// `stats_reset` de pg_stat_database es la marca que sobrevive en PostgreSQL 16: cuándo se
	// reiniciaron las estadísticas de esta base. Es la mejor aproximación disponible y se mueve
	// junto con `pg_stat_statements_reset()`, que es cuando de verdad importa.
	const q = `SELECT to_char(stats_reset AT TIME ZONE 'America/Costa_Rica', 'YYYY-MM-DD HH24:MI')
	             FROM pg_stat_database WHERE datname = current_database()`
	var desde *string
	if err := r.pool.QueryRow(ctx, q).Scan(&desde); err != nil || desde == nil {
		return ""
	}
	return *desde
}

// ReiniciarContadores pone los acumulados en cero.
//
// Sirve para medir una ventana concreta: se reinicia, se reproduce lo que va lento y se mira. No
// borra ningún dato del ERP —los contadores viven en memoria del motor— pero sí se pierde el
// histórico acumulado, y por eso la pantalla lo pide con confirmación.
func (r *pgRepositorioRendimiento) ReiniciarContadores(ctx context.Context) error {
	activa, err := r.extensionActiva(ctx)
	if err != nil {
		return err
	}
	if !activa {
		return ErrRendimientoNoDisponible
	}
	if _, err := r.pool.Exec(ctx, `SELECT pg_stat_statements_reset()`); err != nil {
		return fmt.Errorf("sistema: reiniciar contadores: %w", err)
	}
	return nil
}
