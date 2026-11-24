package sistema

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository construye el repositorio.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

// Registrar inserta el incidente.
//
// Usa el pool directo y un INSERT sin lecturas previas: esto corre mientras se le responde a un
// usuario cuya petición ya falló, y cada viaje de más a la base es tiempo que el error tarda en
// llegarle.
func (r *pgRepository) Registrar(ctx context.Context, in Incidente) error {
	const q = `
		INSERT INTO incidente (referencia, empresa_id, usuario_id, metodo, ruta, estado,
		                       operacion, detalle, panico, traza)
		VALUES ($1, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10)`
	if _, err := r.pool.Exec(ctx, q, in.Referencia, in.EmpresaID, in.UsuarioID,
		in.Metodo, in.Ruta, in.Estado, in.Operacion, in.Detalle, in.Panico, in.Traza); err != nil {
		return fmt.Errorf("sistema: registrar incidente: %w", err)
	}
	return nil
}

// visibleDesde es el recorte por empresa, y es la pieza de seguridad de este paquete.
//
// Los incidentes de la empresa activa, MÁS los que no tienen ninguna. Esos últimos son los que
// ocurrieron antes de elegir empresa —un 500 en el login, en el refresh— y esconderlos dejaría
// sin ver justo el error que impide entrar. No llevan ningún dato de otra empresa: por definición
// ocurrieron cuando todavía no había una.
const visibleDesde = `(i.empresa_id = $1::uuid OR i.empresa_id IS NULL)`

func (r *pgRepository) Listar(ctx context.Context, empresaID string, f FiltrosIncidentes) ([]Incidente, error) {
	conds := []string{visibleDesde}
	args := []any{empresaID}
	addArg := func(v any) int { args = append(args, v); return len(args) }

	if f.Referencia != "" {
		conds = append(conds, fmt.Sprintf("i.referencia = $%d", addArg(f.Referencia)))
	}
	if f.Desde != "" {
		conds = append(conds, fmt.Sprintf("i.creado_en >= $%d::date", addArg(f.Desde)))
	}
	if f.Hasta != "" {
		// `< hasta + 1 día` y no `<= hasta`: la columna es timestamptz y un «hasta el 22» con
		// `<=` deja fuera todo lo que pasó ese 22 después de medianoche, que es todo.
		conds = append(conds, fmt.Sprintf("i.creado_en < ($%d::date + 1)", addArg(f.Hasta)))
	}
	if f.SoloPanico {
		conds = append(conds, "i.panico")
	}

	q := `
		SELECT i.id::text, i.referencia, i.empresa_id::text, i.usuario_id::text,
		       COALESCE(u.nombre, u.email, ''),
		       i.metodo, i.ruta, i.estado, i.operacion, i.detalle, i.panico, i.traza, i.creado_en
		FROM incidente i
		LEFT JOIN usuario u ON u.id = i.usuario_id
		WHERE ` + strings.Join(conds, " AND ") + `
		ORDER BY i.creado_en DESC
		LIMIT $` + fmt.Sprint(addArg(f.Limite))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("sistema: listar incidentes: %w", err)
	}
	defer rows.Close()

	out := make([]Incidente, 0)
	for rows.Next() {
		var in Incidente
		if err := rows.Scan(&in.ID, &in.Referencia, &in.EmpresaID, &in.UsuarioID, &in.Usuario,
			&in.Metodo, &in.Ruta, &in.Estado, &in.Operacion, &in.Detalle, &in.Panico, &in.Traza,
			&in.CreadoEn); err != nil {
			return nil, fmt.Errorf("sistema: scan incidente: %w", err)
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// Resumen cuenta por ventanas sobre EL MISMO conjunto que ve Listar.
//
// Una sola consulta con FILTER en vez de cuatro: si cada contador hiciera su viaje, los números
// de la pantalla podrían venir de instantes distintos y «1 en la última hora, 0 en el último día»
// es una contradicción que nadie puede explicar.
func (r *pgRepository) Resumen(ctx context.Context, empresaID string) (ResumenIncidentes, error) {
	const q = `
		SELECT COUNT(*) FILTER (WHERE i.creado_en >= now() - interval '1 hour')::int,
		       COUNT(*) FILTER (WHERE i.creado_en >= now() - interval '1 day')::int,
		       COUNT(*) FILTER (WHERE i.creado_en >= now() - interval '7 days')::int,
		       COUNT(*) FILTER (WHERE i.panico AND i.creado_en >= now() - interval '7 days')::int,
		       MAX(i.creado_en)
		FROM incidente i
		WHERE ` + visibleDesde

	var res ResumenIncidentes
	if err := r.pool.QueryRow(ctx, q, empresaID).Scan(
		&res.UltimaHora, &res.UltimoDia, &res.UltimaSemana, &res.Panicos, &res.Ultimo); err != nil {
		return ResumenIncidentes{}, fmt.Errorf("sistema: resumen de incidentes: %w", err)
	}
	return res, nil
}
