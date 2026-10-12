package cxp

// Acceso a datos de la nomenclatura: listar el catálogo con sus códigos y asignarlos.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// codigoPagoSQL resuelve el código de una factura con la precedencia del dominio: la clasificación
// pisa al concepto. Se define UNA vez y se usa en el archivo de pago y en el listado, para que la
// pantalla no pueda mostrar un código distinto del que viaja al banco.
const codigoPagoSQL = `COALESCE(NULLIF(cl.codigo_pago, ''), NULLIF(c.codigo_pago, ''), '')`

// CatalogoConCodigos lista los rubros del catálogo con su código, incluidos los que no tienen.
//
// Trae TODO el catálogo y no solo lo codificado: la pantalla sirve para cargar los que faltan, y
// una lista que esconda los vacíos haría imposible encontrarlos. El conteo de facturas dice por
// dónde empezar — codificar un rubro con 800 facturas rinde más que uno con dos.
func (r *pgRepository) CatalogoConCodigos(ctx context.Context, empresaID string) ([]RubroConCodigo, error) {
	const q = `
		SELECT c.id::text, c.nombre, 'CONCEPTO', '', COALESCE(c.codigo_pago, ''), false,
		       (SELECT COUNT(*) FROM documento_cxp d WHERE d.concepto_id = c.id)
		FROM concepto c
		WHERE c.empresa_id = $1::uuid AND c.activo
		UNION ALL
		SELECT cl.id::text, cl.nombre, 'CLASIFICACION', c.nombre,
		       -- El código EFECTIVO: el propio, o el que hereda del concepto.
		       ` + codigoPagoSQL + `,
		       -- Heredado cuando no tiene propio pero el concepto sí.
		       (NULLIF(cl.codigo_pago, '') IS NULL AND NULLIF(c.codigo_pago, '') IS NOT NULL),
		       (SELECT COUNT(*) FROM documento_cxp d WHERE d.clasificacion_id = cl.id)
		FROM clasificacion cl
		JOIN concepto c ON c.id = cl.concepto_id
		WHERE cl.empresa_id = $1::uuid AND cl.activo
		ORDER BY 4, 3 DESC, 2`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: catálogo con códigos: %w", err)
	}
	defer rows.Close()
	out := make([]RubroConCodigo, 0)
	for rows.Next() {
		var x RubroConCodigo
		if err := rows.Scan(&x.ID, &x.Nombre, &x.Nivel, &x.Concepto, &x.Codigo, &x.Heredado, &x.Facturas); err != nil {
			return nil, fmt.Errorf("cxp: scan rubro con código: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// AsignarCodigoPago pone (o quita, con "") el código de un rubro del catálogo.
//
// `nivel` decide la TABLA y por eso sale de una whitelist del propio paquete, nunca del cliente:
// acá se concatena el nombre de una tabla.
func (r *pgRepository) AsignarCodigoPago(ctx context.Context, empresaID, nivel, id, codigo string) error {
	tabla := "concepto"
	if nivel == NivelClasificacion {
		tabla = "clasificacion"
	} else if nivel != NivelConcepto {
		return fmt.Errorf("cxp: nivel de catálogo desconocido: %q", nivel)
	}
	// NULLIF para guardar NULL y no la cadena vacía: el índice único es parcial sobre NOT NULL, y
	// con cadenas vacías dos rubros «sin código» chocarían entre sí.
	q := `UPDATE ` + tabla + ` SET codigo_pago = NULLIF($3, '')
	      WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, codigo)
	if err != nil {
		// 23505 = el código ya identifica a otro rubro. Se traduce al error del dominio en vez de
		// devolver un 500: no es una falla del sistema, es una regla.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrCodigoPagoDuplicado
		}
		return fmt.Errorf("cxp: asignar código de pago: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCatalogoInvalido
	}
	return nil
}

// CodigoPagoDeDocumento resuelve el código de UNA factura, con la precedencia del dominio.
func (r *pgRepository) CodigoPagoDeDocumento(ctx context.Context, empresaID, documentoID string) (string, error) {
	const q = `
		SELECT ` + codigoPagoSQL + `
		FROM documento_cxp d
		LEFT JOIN concepto c ON c.id = d.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = d.clasificacion_id
		WHERE d.empresa_id = $1::uuid AND d.id = $2::uuid`
	var codigo string
	err := r.pool.QueryRow(ctx, q, empresaID, documentoID).Scan(&codigo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrDocumentoNoEncontrado
	}
	if err != nil {
		return "", fmt.Errorf("cxp: código de pago del documento: %w", err)
	}
	return codigo, nil
}
