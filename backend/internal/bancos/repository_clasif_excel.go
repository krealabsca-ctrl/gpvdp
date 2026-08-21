package bancos

// Consultas de la clasificación en bloque desde Excel.

import (
	"context"
	"fmt"
)

// MovimientoCalzado es un movimiento que calzó con una fila del archivo.
type MovimientoCalzado struct {
	// Clave: la tupla (cuenta|fecha|débito|crédito|documento) con la que se buscó.
	Clave         string
	ID            string
	Descripcion   string
	Concepto      string
	Clasificacion string
	ClasifID      string
	Estado        string
}

// BuscarMovimientosPorTupla devuelve los movimientos que calzan con las tuplas pedidas.
//
// La comparación es en SQL con `date` y `numeric`, no comparando textos: así da igual cómo Excel
// haya escrito «1 234,56». La descripción NO entra en la búsqueda (algunos bancos la cambian entre
// descargas) pero SÍ se devuelve, porque es la prueba de que se encontró el movimiento correcto.
//
// Un pedido puede traer varios movimientos: son los duplicados legítimos que el importador conserva
// con `indice_ocurrencia`. Vienen ordenados por id para que el emparejamiento sea estable entre dos
// corridas del mismo archivo.
func (r *pgRepository) BuscarMovimientosPorTupla(ctx context.Context, empresaID string, cuentas []string, fechas []string, debitos, creditos, documentos []string) ([]MovimientoCalzado, error) {
	const q = `
		WITH pedidos AS (
			SELECT * FROM unnest($2::uuid[], $3::date[], $4::numeric[], $5::numeric[], $6::text[])
			         AS t(cuenta_id, fecha, debito, credito, documento)
		)
		SELECT p.cuenta_id::text, to_char(p.fecha, 'YYYY-MM-DD'),
		       to_char(p.debito, 'FM9999999999999990.00'), to_char(p.credito, 'FM9999999999999990.00'),
		       p.documento,
		       m.id::text, COALESCE(m.descripcion, ''),
		       COALESCE(co.nombre, ''), COALESCE(cl.nombre, ''),
		       COALESCE(m.clasificacion_id::text, ''), m.estado_clasificacion
		FROM pedidos p
		JOIN movimiento_bancario m
		  ON m.empresa_id = $1::uuid
		 AND m.cuenta_bancaria_id = p.cuenta_id
		 AND m.fecha = p.fecha
		 AND m.debito = p.debito
		 AND m.credito = p.credito
		 AND COALESCE(m.documento, '') = p.documento
		 AND m.incluido
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		ORDER BY 1, 2, 3, 4, 5, m.id`
	rows, err := r.pool.Query(ctx, q, empresaID, cuentas, fechas, debitos, creditos, documentos)
	if err != nil {
		return nil, fmt.Errorf("bancos: buscar movimientos por tupla: %w", err)
	}
	defer rows.Close()
	out := []MovimientoCalzado{}
	for rows.Next() {
		var cuenta, fecha, debito, credito, documento string
		var m MovimientoCalzado
		if err := rows.Scan(&cuenta, &fecha, &debito, &credito, &documento,
			&m.ID, &m.Descripcion, &m.Concepto, &m.Clasificacion, &m.ClasifID, &m.Estado); err != nil {
			return nil, fmt.Errorf("bancos: scan movimiento calzado: %w", err)
		}
		m.Clave = cuenta + "|" + fecha + "|" + debito + "|" + credito + "|" + documento
		out = append(out, m)
	}
	return out, rows.Err()
}

// AsignacionClasif es un movimiento con la partida que le toca.
type AsignacionClasif struct {
	MovimientoID    string
	ConceptoID      string
	ClasificacionID string
}

// AplicarClasificacionesEnBloque asigna a cada movimiento SU propia partida, en una transacción.
//
// No se puede reusar el `clasificar-masivo` existente: ese aplica UNA partida a muchos movimientos, y
// acá cada fila del archivo trae la suya. El UPDATE va con `unnest` para que sea una sola ida a la
// base aunque vengan miles de filas.
//
// Deja `estado_clasificacion = 'REVISADO'` y `confianza = 100`: lo trae una persona en un archivo, no
// el motor, así que no es una sugerencia automática. `es_traslado` se deriva de la clasificación, con
// la misma expresión que usa la clasificación manual — si acá se olvidara, un traslado importado por
// archivo entraría al EBITDA y el número dejaría de cuadrar con el de la pantalla.
func (r *pgRepository) AplicarClasificacionesEnBloque(ctx context.Context, empresaID string, asigs []AsignacionClasif) (int, error) {
	if len(asigs) == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(asigs))
	conceptos := make([]string, 0, len(asigs))
	clasifs := make([]string, 0, len(asigs))
	for _, a := range asigs {
		ids = append(ids, a.MovimientoID)
		conceptos = append(conceptos, a.ConceptoID)
		clasifs = append(clasifs, a.ClasificacionID)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("bancos: abrir transacción de clasificación en bloque: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// El JOIN con `clasificacion` es el guardarraíl de empresa y de coherencia: una clasificación de
	// otra empresa, o que no pertenezca al concepto indicado, no calza y esa fila no se escribe.
	q := `
		WITH asig AS (
			SELECT * FROM unnest($2::uuid[], $3::uuid[], $4::uuid[])
			         AS t(mov_id, concepto_id, clasificacion_id)
		)
		UPDATE movimiento_bancario m
		   SET concepto_id = a.concepto_id,
		       clasificacion_id = a.clasificacion_id,
		       estado_clasificacion = 'REVISADO',
		       confianza = 100,
		       es_traslado = ` + sqlEsTrasladoDerivado("m.", "a.concepto_id", "$1::uuid") + `,
		       actualizado_en = now()
		  FROM asig a
		  JOIN clasificacion cl ON cl.id = a.clasificacion_id
		                       AND cl.empresa_id = $1::uuid
		                       AND cl.concepto_id = a.concepto_id
		 WHERE m.id = a.mov_id AND m.empresa_id = $1::uuid`
	tag, err := tx.Exec(ctx, q, empresaID, ids, conceptos, clasifs)
	if err != nil {
		return 0, fmt.Errorf("bancos: aplicar clasificaciones en bloque: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("bancos: confirmar clasificación en bloque: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// MovimientosParaPlantilla es una fila de la plantilla de clasificación.
type MovimientosParaPlantilla struct {
	Fecha         string
	Banco         string
	Cuenta        string
	Documento     string
	Descripcion   string
	Moneda        string
	Debito        string
	Credito       string
	Concepto      string
	Clasificacion string
}

// MovimientosPlantillaClasif devuelve los movimientos del rango para armar la plantilla.
//
// `soloSinClasificar` es el caso normal: la plantilla es la lista de trabajo. Con false trae todo,
// que sirve para revisar o corregir lo ya clasificado.
func (r *pgRepository) MovimientosPlantillaClasif(ctx context.Context, empresaID, desde, hasta string, soloSinClasificar bool, limite int) ([]MovimientosParaPlantilla, error) {
	const q = `
		SELECT to_char(m.fecha, 'DD/MM/YYYY'), b.nombre, c.alias, COALESCE(m.documento, ''),
		       COALESCE(m.descripcion, ''), m.moneda_original,
		       to_char(m.debito, 'FM9999999999999990.00'), to_char(m.credito, 'FM9999999999999990.00'),
		       COALESCE(co.nombre, ''), COALESCE(cl.nombre, '')
		FROM movimiento_bancario m
		JOIN cuenta_bancaria c ON c.id = m.cuenta_bancaria_id
		JOIN banco b ON b.id = c.banco_id
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		WHERE m.empresa_id = $1::uuid AND m.incluido
		  AND m.fecha BETWEEN to_date($2, 'YYYY-MM-DD') AND to_date($3, 'YYYY-MM-DD')
		  AND (NOT $4::bool OR m.clasificacion_id IS NULL)
		ORDER BY c.alias, m.fecha, m.id
		LIMIT $5`
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta, soloSinClasificar, limite)
	if err != nil {
		return nil, fmt.Errorf("bancos: movimientos para plantilla: %w", err)
	}
	defer rows.Close()
	out := []MovimientosParaPlantilla{}
	for rows.Next() {
		var m MovimientosParaPlantilla
		if err := rows.Scan(&m.Fecha, &m.Banco, &m.Cuenta, &m.Documento, &m.Descripcion,
			&m.Moneda, &m.Debito, &m.Credito, &m.Concepto, &m.Clasificacion); err != nil {
			return nil, fmt.Errorf("bancos: scan fila de plantilla: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
