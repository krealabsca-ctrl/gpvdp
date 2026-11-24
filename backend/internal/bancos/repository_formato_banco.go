package bancos

// Consultas de los formatos de banco configurables (mig 0108). El porqué está en formato_banco.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrFormatoNoEncontrado: el formato no existe en esta empresa.
var ErrFormatoNoEncontrado = errors.New("bancos: ese formato de banco no existe")

// ErrFormatoDuplicado: ya hay uno con ese nombre en la empresa.
var ErrFormatoDuplicado = errors.New("bancos: ya hay un formato con ese nombre")

const camposFormato = `id::text, nombre, activo, texto_identificador,
	col_fecha, col_documento, col_descripcion, col_debito, col_credito,
	formato_fecha, senales, creado_en`

func escanearFormato(row pgx.Row) (FormatoBanco, error) {
	var f FormatoBanco
	var crudas []byte
	err := row.Scan(&f.ID, &f.Nombre, &f.Activo, &f.TextoIdentificador,
		&f.ColFecha, &f.ColDocumento, &f.ColDescripcion, &f.ColDebito, &f.ColCredito,
		&f.FormatoFecha, &crudas, &f.CreadoEn)
	if err != nil {
		return FormatoBanco{}, err
	}
	// Las señales se guardan como JSON. Si una fila quedara ilegible se devuelve con la lista
	// vacía en vez de romper el listado entero: `isHeader` no reconoce nada sin señales, así que
	// el formato queda inerte y visible para que alguien lo corrija —en vez de invisible—.
	if len(crudas) > 0 {
		_ = json.Unmarshal(crudas, &f.Senales)
	}
	if f.Senales == nil {
		f.Senales = []SenalEncabezado{}
	}
	return f, nil
}

// FormatosDeBanco lista los formatos de la empresa. `soloActivos` es lo que usa el importador.
func (r *pgRepository) FormatosDeBanco(ctx context.Context, empresaID string, soloActivos bool) ([]FormatoBanco, error) {
	q := `SELECT ` + camposFormato + ` FROM formato_banco WHERE empresa_id = $1::uuid`
	if soloActivos {
		q += ` AND activo`
	}
	// Por nombre: es una lista para leer, y el orden de creación no significa nada para quien la
	// mira. El importador los prueba a todos, así que el orden no cambia qué se detecta.
	q += ` ORDER BY nombre`

	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar formatos: %w", err)
	}
	defer rows.Close()

	out := make([]FormatoBanco, 0)
	for rows.Next() {
		f, err := escanearFormato(rows)
		if err != nil {
			return nil, fmt.Errorf("bancos: scan formato: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GuardarFormatoBanco crea o actualiza. Con `f.ID` vacío crea.
func (r *pgRepository) GuardarFormatoBanco(ctx context.Context, empresaID string, f FormatoBanco, usuarioID string) (FormatoBanco, error) {
	senales, err := json.Marshal(f.Senales)
	if err != nil {
		return FormatoBanco{}, fmt.Errorf("bancos: serializar señales: %w", err)
	}

	if f.ID == "" {
		const q = `
			INSERT INTO formato_banco (empresa_id, nombre, activo, texto_identificador,
				col_fecha, col_documento, col_descripcion, col_debito, col_credito,
				formato_fecha, senales, creado_por)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, NULLIF($12,'')::uuid)
			RETURNING ` + camposFormato
		nuevo, err := escanearFormato(r.pool.QueryRow(ctx, q, empresaID, f.Nombre, f.Activo,
			f.TextoIdentificador, f.ColFecha, f.ColDocumento, f.ColDescripcion, f.ColDebito,
			f.ColCredito, f.FormatoFecha, string(senales), usuarioID))
		if err != nil {
			if esViolacionUnica(err) {
				return FormatoBanco{}, ErrFormatoDuplicado
			}
			return FormatoBanco{}, fmt.Errorf("bancos: crear formato: %w", err)
		}
		return nuevo, nil
	}

	const q = `
		UPDATE formato_banco SET nombre = $3, activo = $4, texto_identificador = $5,
			col_fecha = $6, col_documento = $7, col_descripcion = $8, col_debito = $9,
			col_credito = $10, formato_fecha = $11, senales = $12::jsonb, actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid
		RETURNING ` + camposFormato
	act, err := escanearFormato(r.pool.QueryRow(ctx, q, empresaID, f.ID, f.Nombre, f.Activo,
		f.TextoIdentificador, f.ColFecha, f.ColDocumento, f.ColDescripcion, f.ColDebito,
		f.ColCredito, f.FormatoFecha, string(senales)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || idInvalido(err) {
			return FormatoBanco{}, ErrFormatoNoEncontrado
		}
		if esViolacionUnica(err) {
			return FormatoBanco{}, ErrFormatoDuplicado
		}
		return FormatoBanco{}, fmt.Errorf("bancos: actualizar formato: %w", err)
	}
	return act, nil
}

// BorrarFormatoBanco lo elimina.
//
// Borrado FÍSICO y no lógico, que es la excepción a la regla del proyecto: esto es configuración
// de LECTURA, no un asiento. Nada cuelga de esta fila —`importacion.banco` guarda el nombre como
// texto, sin llave foránea—, así que borrarlo no deja huérfano nada ni cambia una sola cifra de
// lo ya importado. Y `activo = false` existe aparte para apagarlo sin perderlo.
func (r *pgRepository) BorrarFormatoBanco(ctx context.Context, empresaID, id string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM formato_banco WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, id)
	if err != nil {
		if idInvalido(err) {
			return ErrFormatoNoEncontrado
		}
		return fmt.Errorf("bancos: borrar formato: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrFormatoNoEncontrado
	}
	return nil
}
