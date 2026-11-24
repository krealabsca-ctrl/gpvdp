package conta

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository construye el repositorio.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

const camposCuenta = `id::text, empresa_id::text, codigo, nombre, padre_id::text, clase,
	naturaleza, acepta_movimiento, nivel, codigo_softland, activo, creado_en`

func escanear(row pgx.Row) (Cuenta, error) {
	var c Cuenta
	err := row.Scan(&c.ID, &c.EmpresaID, &c.Codigo, &c.Nombre, &c.PadreID, &c.Clase,
		&c.Naturaleza, &c.AceptaMovimiento, &c.Nivel, &c.CodigoSoftland, &c.Activo, &c.CreadoEn)
	return c, err
}

// visibleDesde es el recorte por empresa.
//
// Las cuentas de la empresa MÁS las del grupo (`empresa_id IS NULL`). Esa segunda mitad no es una
// fuga: una cuenta del grupo existe justamente para que las tres empresas la usen, y es la que
// permite un plan común sin decidirlo en el modelo. Una cuenta de OTRA empresa nunca se ve.
const visibleDesde = `(c.empresa_id = $1::uuid OR c.empresa_id IS NULL)`

func (r *pgRepository) Cuentas(ctx context.Context, empresaID string, soloActivas bool) ([]Cuenta, error) {
	q := `SELECT ` + camposCuenta + ` FROM cuenta_contable c WHERE ` + visibleDesde
	if soloActivas {
		q += ` AND c.activo`
	}
	q += ` ORDER BY c.codigo`

	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("conta: listar cuentas: %w", err)
	}
	defer rows.Close()

	out := make([]Cuenta, 0)
	for rows.Next() {
		c, err := escanear(rows)
		if err != nil {
			return nil, fmt.Errorf("conta: scan cuenta: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *pgRepository) CuentaPorCodigo(ctx context.Context, empresaID, codigo string) (Cuenta, error) {
	q := `SELECT ` + camposCuenta + ` FROM cuenta_contable c WHERE ` + visibleDesde + ` AND c.codigo = $2`
	c, err := escanear(r.pool.QueryRow(ctx, q, empresaID, codigo))
	if errors.Is(err, pgx.ErrNoRows) {
		return Cuenta{}, ErrCuentaNoEncontrada
	}
	if err != nil {
		return Cuenta{}, fmt.Errorf("conta: cuenta por código: %w", err)
	}
	return c, nil
}

func (r *pgRepository) CrearCuenta(ctx context.Context, c Cuenta, usuarioID string) (Cuenta, error) {
	// La madre llega como CÓDIGO desde el importador y como id desde la pantalla. Acá se resuelve
	// a id: es lo que la llave foránea necesita.
	padreID, err := r.resolverPadre(ctx, r.pool, c)
	if err != nil {
		return Cuenta{}, err
	}
	const q = `
		INSERT INTO cuenta_contable (empresa_id, codigo, nombre, padre_id, clase, naturaleza,
		                             acepta_movimiento, nivel, codigo_softland, activo, creado_por)
		VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6, $7, $8, $9, $10, NULLIF($11,'')::uuid)
		RETURNING ` + camposCuenta
	nueva, err := escanear(r.pool.QueryRow(ctx, q, c.EmpresaID, c.Codigo, c.Nombre, padreID,
		c.Clase, c.Naturaleza, c.AceptaMovimiento, c.Nivel, c.CodigoSoftland, true, usuarioID))
	if err != nil {
		return Cuenta{}, traducir(err)
	}
	return nueva, nil
}

// resolverPadre convierte el padre que venga —id o código— en un id de la base.
func (r *pgRepository) resolverPadre(ctx context.Context, q consultante, c Cuenta) (*string, error) {
	if c.PadreID == nil || *c.PadreID == "" {
		return nil, nil
	}
	p := *c.PadreID
	// Un código trae guiones; un uuid no. Es la forma más barata de distinguirlos y no hay
	// ambigüedad posible: ningún uuid contiene «-» en las posiciones de un código de cuenta.
	if !esUUID(p) {
		var id string
		err := q.QueryRow(ctx,
			`SELECT id::text FROM cuenta_contable
			 WHERE codigo = $1 AND (empresa_id = $2::uuid OR ($2::uuid IS NULL AND empresa_id IS NULL))`,
			p, c.EmpresaID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: la cuenta madre %q no existe", ErrCuentaNoEncontrada, p)
		}
		if err != nil {
			return nil, fmt.Errorf("conta: resolver cuenta madre: %w", err)
		}
		return &id, nil
	}
	return &p, nil
}

// consultante es lo mínimo que `resolverPadre` necesita, para servir igual al pool y a una
// transacción.
type consultante interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func esUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	return s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-'
}

func (r *pgRepository) ActualizarCuenta(ctx context.Context, empresaID string, c Cuenta) (Cuenta, error) {
	// El código NO se actualiza: mover una cuenta de lugar es otra operación. Ver el servicio.
	const q = `
		UPDATE cuenta_contable c
		SET nombre = $3, naturaleza = $4, acepta_movimiento = $5, activo = $6, actualizado_en = now()
		WHERE c.id = $2::uuid AND ` + visibleDesde + `
		RETURNING ` + camposCuenta
	act, err := escanear(r.pool.QueryRow(ctx, q, empresaID, c.ID, c.Nombre, c.Naturaleza,
		c.AceptaMovimiento, c.Activo))
	if errors.Is(err, pgx.ErrNoRows) {
		return Cuenta{}, ErrCuentaNoEncontrada
	}
	if err != nil {
		return Cuenta{}, traducir(err)
	}
	return act, nil
}

func (r *pgRepository) BorrarCuenta(ctx context.Context, empresaID, id string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM cuenta_contable c WHERE c.id = $2::uuid AND `+visibleDesde, empresaID, id)
	if err != nil {
		if idInvalido(err) {
			return ErrCuentaNoEncontrada
		}
		// Una hija apuntando a esta cuenta la retiene: borrar la madre dejaría el árbol partido.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return fmt.Errorf("conta: la cuenta tiene cuentas hijas; borralas o movelas primero")
		}
		return fmt.Errorf("conta: borrar cuenta: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCuentaNoEncontrada
	}
	return nil
}

// ImportarCuentas escribe el plan entero en UNA transacción.
//
// POR QUÉ UNA SOLA TRANSACCIÓN: un plan de cuentas a medias es peor que ninguno. Si la fila 180
// falla, las 179 anteriores dejarían un árbol trunco donde faltan ramas, y nadie sabría cuáles
// sin comparar contra el Excel a mano. O entra entero, o no entra nada.
//
// Las cuentas llegan ORDENADAS DE LA RAÍZ HACIA ABAJO (ver ArmarPlan): la madre se inserta antes
// que la hija, que es lo que la llave foránea exige.
func (r *pgRepository) ImportarCuentas(ctx context.Context, destino *string, cuentas []Cuenta, usuarioID string) (ResultadoImportacion, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ResultadoImportacion{}, fmt.Errorf("conta: abrir transacción: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var res ResultadoImportacion
	for _, c := range cuentas {
		c.EmpresaID = destino
		padreID, err := r.resolverPadre(ctx, tx, c)
		if err != nil {
			return ResultadoImportacion{}, err
		}

		// ON CONFLICT sobre el código: reimportar el mismo archivo actualiza en vez de reventar,
		// que es lo que hace falta mientras el contador corrige el catálogo y vuelve a subirlo.
		//
		// `acepta_movimiento` se actualiza también: si una cuenta que era hoja gana hijas en el
		// archivo nuevo, tiene que dejar de aceptar movimiento, y el disparador de la 0109 se
		// encarga de que el orden sea el correcto.
		conflicto := `(empresa_id, codigo) WHERE empresa_id IS NOT NULL`
		if destino == nil {
			conflicto = `(codigo) WHERE empresa_id IS NULL`
		}
		q := `
			INSERT INTO cuenta_contable (empresa_id, codigo, nombre, padre_id, clase, naturaleza,
			                             acepta_movimiento, nivel, codigo_softland, creado_por)
			VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6, $7, $8, $9, NULLIF($10,'')::uuid)
			ON CONFLICT ` + conflicto + ` DO UPDATE
			SET nombre = EXCLUDED.nombre, padre_id = EXCLUDED.padre_id, clase = EXCLUDED.clase,
			    naturaleza = EXCLUDED.naturaleza, acepta_movimiento = EXCLUDED.acepta_movimiento,
			    nivel = EXCLUDED.nivel, codigo_softland = EXCLUDED.codigo_softland,
			    actualizado_en = now()
			RETURNING (xmax = 0) AS insertada`
		var insertada bool
		if err := tx.QueryRow(ctx, q, destino, c.Codigo, c.Nombre, padreID, c.Clase,
			c.Naturaleza, c.AceptaMovimiento, c.Nivel, c.CodigoSoftland, usuarioID).Scan(&insertada); err != nil {
			return ResultadoImportacion{}, fmt.Errorf("conta: importar la cuenta %s: %w", c.Codigo, traducir(err))
		}
		if insertada {
			res.Creadas++
		} else {
			res.Actualizadas++
		}
		if c.AceptaMovimiento {
			res.DeMovimiento++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ResultadoImportacion{}, fmt.Errorf("conta: confirmar la importación: %w", err)
	}
	res.Descartadas = []FilaImportada{}
	return res, nil
}

// traducir convierte los errores de Postgres en errores de dominio, para que el handler pueda
// contestar algo que se entienda en vez de «error interno».
func traducir(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505":
		return ErrCodigoDuplicado
	case "23514":
		// Lo levanta el disparador de la 0109 con su propio mensaje, que ya explica el porqué.
		if pgErr.Message != "" {
			return fmt.Errorf("conta: %s", pgErr.Message)
		}
		return ErrTieneHijas
	}
	return err
}

func idInvalido(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02"
}

// errorDePostgres devuelve el mensaje del servidor, o "" si el error no viene de Postgres.
//
// Los disparadores de las migraciones 0109 y 0110 levantan excepciones con un texto escrito para
// que lo lea una persona («la cuenta X agrupa a otras, así que no admite asientos»). Ese texto es
// mejor que cualquier mensaje genérico que se pueda poner en su lugar, así que se recupera.
func errorDePostgres(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Message
	}
	return ""
}
