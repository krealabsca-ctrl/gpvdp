package bancos

// Enlazar dos movimientos que son el mismo hecho reemitido por el banco (mig 0118).
//
// Este es el TERCER escritor de `incluido`, después de la reversa de una carga (repository_reversa.go)
// y de excluir suelto (repository_excluir.go). Conviven porque cada uno solo devuelve a los libros lo
// que él mismo sacó: la reversa mira `excluido_por_reversa`, el de a mano mira que NO lo sea, y este
// mira el enlace. Sin esa separación, deshacer cualquiera de los tres devolvería plata que otro había
// sacado por una razón distinta.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CandidatosReemision propone pares que PODRÍAN ser el mismo hecho reemitido.
//
// La huella es: misma cuenta, mismo consecutivo, mismo monto y mismo lado, en fechas distintas pero
// CERCANAS. La cercanía es lo que hace útil la lista: medido sobre los datos reales, la huella sin
// el límite de días devuelve sobre todo pagos recurrentes —un ahorro de ₡1.000.000 que el BCR debita
// cada semana con el mismo consecutivo, cuotas mensuales del BN— que caen a 7, 14, 28 o 30 días. Una
// reemisión del banco cae al día siguiente.
//
// Excluye los que ya están enlazados por cualquiera de los dos lados, y los que ya salieron de los
// libros: proponer un par cuyo movimiento ya no cuenta mandaría a corregir algo que ya está resuelto.
func (r *pgRepository) CandidatosReemision(ctx context.Context, empresaID string, diasMax int) ([]CandidatoReemision, error) {
	const q = `
		SELECT COALESCE(NULLIF(a.debito, 0), a.credito, 0)::text,
		       COALESCE(a.debito, 0) > 0,
		       COALESCE(cb.alias, ''), COALESCE(bk.nombre, ''),
		       (b.fecha - a.fecha)::int,
		       ` + ladoReemisionSQL_a + `,
		       ` + ladoReemisionSQL_b + `
		FROM movimiento_bancario a
		JOIN movimiento_bancario b
		  ON b.empresa_id = a.empresa_id
		 AND b.cuenta_bancaria_id = a.cuenta_bancaria_id
		 AND b.documento = a.documento
		 AND COALESCE(b.debito, 0) = COALESCE(a.debito, 0)
		 AND COALESCE(b.credito, 0) = COALESCE(a.credito, 0)
		 AND b.fecha > a.fecha
		 AND b.fecha <= a.fecha + $2::int
		JOIN cuenta_bancaria cb ON cb.id = a.cuenta_bancaria_id
		JOIN banco bk ON bk.id = cb.banco_id
		LEFT JOIN concepto co_a ON co_a.id = a.concepto_id
		LEFT JOIN clasificacion cl_a ON cl_a.id = a.clasificacion_id
		LEFT JOIN concepto co_b ON co_b.id = b.concepto_id
		LEFT JOIN clasificacion cl_b ON cl_b.id = b.clasificacion_id
		WHERE a.empresa_id = $1::uuid
		  AND COALESCE(a.documento, '') <> ''
		  AND a.incluido AND b.incluido
		  -- Ninguno de los dos puede estar ya en un enlace, ni como el que cuenta ni como el que salió.
		  AND a.reemplaza_a IS NULL AND b.reemplaza_a IS NULL
		  AND NOT EXISTS (SELECT 1 FROM movimiento_bancario x
		                   WHERE x.reemplaza_a IN (a.id, b.id))
		ORDER BY (b.fecha - a.fecha), a.fecha DESC
		LIMIT 50`
	rows, err := r.pool.Query(ctx, q, empresaID, diasMax)
	if err != nil {
		return nil, fmt.Errorf("bancos: candidatos de reemisión: %w", err)
	}
	defer rows.Close()
	out := make([]CandidatoReemision, 0, 8)
	for rows.Next() {
		var c CandidatoReemision
		if err := rows.Scan(&c.Monto, &c.EsDebito, &c.Cuenta, &c.Banco, &c.DiasEntre,
			&c.Anterior.ID, &c.Anterior.Fecha, &c.Anterior.Documento, &c.Anterior.Descripcion,
			&c.Anterior.Clasificacion, &c.Anterior.Estado, &c.Anterior.Incluido,
			&c.Posterior.ID, &c.Posterior.Fecha, &c.Posterior.Documento, &c.Posterior.Descripcion,
			&c.Posterior.Clasificacion, &c.Posterior.Estado, &c.Posterior.Incluido); err != nil {
			return nil, fmt.Errorf("bancos: scan candidato de reemisión: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ladoReemisionSQL_a y _b son ladoReemisionSQL con los alias del lado que toca. Se escriben aparte
// porque la consulta cruza la tabla contra sí misma y los dos lados necesitan sus propios joins.
const ladoReemisionSQL_a = `
	a.id::text, a.fecha::text, COALESCE(a.documento, ''), COALESCE(a.descripcion, ''),
	CASE WHEN co_a.nombre IS NULL AND cl_a.nombre IS NULL THEN ''
	     ELSE concat_ws(' › ', NULLIF(co_a.nombre, ''), NULLIF(cl_a.nombre, '')) END,
	a.estado_clasificacion, a.incluido`

const ladoReemisionSQL_b = `
	b.id::text, b.fecha::text, COALESCE(b.documento, ''), COALESCE(b.descripcion, ''),
	CASE WHEN co_b.nombre IS NULL AND cl_b.nombre IS NULL THEN ''
	     ELSE concat_ws(' › ', NULLIF(co_b.nombre, ''), NULLIF(cl_b.nombre, '')) END,
	b.estado_clasificacion, b.incluido`

// datosDeMovimiento es lo que hace falta para validar un enlace antes de escribirlo.
type datosDeMovimiento struct {
	ID       string
	Cuenta   string
	Debito   string
	Credito  string
	Fecha    string
	Anio     int
	Mes      int
	Incluido bool
	// YaEnlazado: este movimiento ya reemplaza a otro.
	YaEnlazado bool
	// YaReemplazado: otro movimiento ya lo reemplazó a él.
	YaReemplazado   bool
	ConceptoID      *string
	ClasificacionID *string
	DepartamentoID  *string
	SedeID          *string
	Estado          string
}

// DatosParaReemision lee los dos lados en UNA consulta para validarlos juntos.
//
// Van juntos a propósito: leerlos por separado deja una ventana en la que el segundo cambia después
// de haber validado el primero, y el enlace se escribiría contra un estado que ya no es.
func (r *pgRepository) DatosParaReemision(ctx context.Context, empresaID, vigenteID, salienteID string) (datosDeMovimiento, datosDeMovimiento, error) {
	const q = `
		SELECT m.id::text, m.cuenta_bancaria_id::text,
		       COALESCE(m.debito, 0)::text, COALESCE(m.credito, 0)::text,
		       m.fecha::text, EXTRACT(YEAR FROM m.fecha)::int, EXTRACT(MONTH FROM m.fecha)::int,
		       m.incluido,
		       m.reemplaza_a IS NOT NULL,
		       EXISTS (SELECT 1 FROM movimiento_bancario x WHERE x.reemplaza_a = m.id),
		       m.concepto_id::text, m.clasificacion_id::text,
		       m.departamento_id::text, m.sede_id::text, m.estado_clasificacion
		FROM movimiento_bancario m
		WHERE m.empresa_id = $1::uuid AND m.id = ANY($2::uuid[])`
	rows, err := r.pool.Query(ctx, q, empresaID, []string{vigenteID, salienteID})
	if err != nil {
		return datosDeMovimiento{}, datosDeMovimiento{}, fmt.Errorf("bancos: datos para reemisión: %w", err)
	}
	defer rows.Close()
	hallados := map[string]datosDeMovimiento{}
	for rows.Next() {
		var d datosDeMovimiento
		if err := rows.Scan(&d.ID, &d.Cuenta, &d.Debito, &d.Credito, &d.Fecha, &d.Anio, &d.Mes,
			&d.Incluido, &d.YaEnlazado, &d.YaReemplazado, &d.ConceptoID, &d.ClasificacionID,
			&d.DepartamentoID, &d.SedeID, &d.Estado); err != nil {
			return datosDeMovimiento{}, datosDeMovimiento{}, fmt.Errorf("bancos: scan datos para reemisión: %w", err)
		}
		hallados[d.ID] = d
	}
	if err := rows.Err(); err != nil {
		return datosDeMovimiento{}, datosDeMovimiento{}, err
	}
	v, okV := hallados[vigenteID]
	s, okS := hallados[salienteID]
	if !okV || !okS {
		return datosDeMovimiento{}, datosDeMovimiento{}, ErrReemisionNoEncontrada
	}
	return v, s, nil
}

// El candado del período cerrado reusa `PeriodoCerrado` (repository_traslados.go): es la misma
// pregunta que ya se hace al emparejar traslados, y dos consultas del mismo cierre terminarían
// dando respuestas distintas el día que alguien cambie una.

// DependenciasDelMovimiento nombra lo que cuelga de un movimiento y cuántos son.
//
// NO incluye `movimiento_reporte_segmentacion`: esa tabla solo registra que el movimiento se le
// mostró a un equipo de partida, no mueve plata, y frenar por ella bloquearía la corrección sin
// proteger ningún número.
func (r *pgRepository) DependenciasDelMovimiento(ctx context.Context, empresaID, movID string) ([]DependenciaDeMovimiento, error) {
	// Todas las tablas de acá llevan `empresa_id` y todas lo filtran. No es ceremonia: sin ese
	// filtro, un id de otra empresa contaría sus dependencias y el freno hablaría de registros que
	// quien pregunta no puede ni ver.
	const q = `
		SELECT 'el pago de una factura de CxP',
		       (SELECT count(*)::int FROM movimiento_bancario m
		         WHERE m.empresa_id = $1::uuid AND m.id = $2::uuid
		           AND (m.documento_cxp_id IS NOT NULL OR m.remesa_id IS NOT NULL))
		UNION ALL SELECT 'un cobro de CxC',
		       (SELECT count(*)::int FROM cobro_cxc
		         WHERE empresa_id = $1::uuid AND movimiento_bancario_id = $2::uuid)
		UNION ALL SELECT 'una planilla de CxC',
		       (SELECT count(*)::int FROM cxc_planilla_movimiento
		         WHERE empresa_id = $1::uuid AND movimiento_bancario_id = $2::uuid)
		UNION ALL SELECT 'un abono parcial a una factura',
		       (SELECT count(*)::int FROM pago_parcial
		         WHERE empresa_id = $1::uuid AND movimiento_id = $2::uuid)
		UNION ALL SELECT 'un pago aplicado en el sistema anterior',
		       (SELECT count(*)::int FROM pago_aplicado_externo
		         WHERE empresa_id = $1::uuid AND movimiento_bancario_id = $2::uuid)
		UNION ALL SELECT 'una responsabilidad del mes',
		       (SELECT count(*)::int FROM responsabilidad_periodo
		         WHERE empresa_id = $1::uuid AND movimiento_id = $2::uuid)
		UNION ALL SELECT 'la otra pata de un traslado',
		       (SELECT count(*)::int FROM movimiento_bancario m
		         WHERE m.empresa_id = $1::uuid AND m.id = $2::uuid AND m.par_traslado_id IS NOT NULL)
		UNION ALL SELECT 'las partes en que se dividió',
		       (SELECT count(*)::int FROM movimiento_bancario
		         WHERE empresa_id = $1::uuid AND dividido_de = $2::uuid)
		UNION ALL SELECT 'el movimiento del que es una parte',
		       (SELECT count(*)::int FROM movimiento_bancario m
		         WHERE m.empresa_id = $1::uuid AND m.id = $2::uuid AND m.dividido_de IS NOT NULL)`
	rows, err := r.pool.Query(ctx, q, empresaID, movID)
	if err != nil {
		return nil, fmt.Errorf("bancos: dependencias del movimiento: %w", err)
	}
	defer rows.Close()
	out := make([]DependenciaDeMovimiento, 0, 4)
	for rows.Next() {
		var d DependenciaDeMovimiento
		if err := rows.Scan(&d.Que, &d.Cantidad); err != nil {
			return nil, fmt.Errorf("bancos: scan dependencia: %w", err)
		}
		if d.Cantidad > 0 {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

// EnlazarReemision escribe el enlace: el vigente apunta al que sale, el que sale deja los libros y,
// si el vigente no estaba clasificado, hereda la clasificación del otro.
//
// Todo en UNA transacción. Si el enlace se escribiera sin que el otro saliera de los libros, el monto
// estaría dos veces en la cartera y nada lo delataría: las dos filas son legítimas por separado.
//
// Devuelve si hubo herencia, para que la pantalla pueda decirlo: una clasificación que apareció sola
// se ve igual que una que alguien puso a mano, y quien revise el mes tiene derecho a saber cuál es.
//
// `motivoExclusion` lo arma el servicio nombrando la fecha del que queda vigente: quien abra la
// lista de fuera de los libros tiene que poder leer ahí por qué salió sin ir a buscar el enlace.
func (r *pgRepository) EnlazarReemision(ctx context.Context, empresaID, vigenteID, salienteID, motivo, motivoExclusion, usuarioID string, heredar bool) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("bancos: abrir transacción de reemisión: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// El que sale deja los libros por la MISMA puerta que excluir a mano, con su motivo y su autor.
	// `excluido_por_reversa = false` lo distingue de lo que salió con una carga revertida: deshacer
	// esa reversa no debe devolver este movimiento.
	const sacar = `
		UPDATE movimiento_bancario
		   SET incluido = false, excluido_por_reversa = false,
		       excluido_motivo = $3, excluido_por = $4::uuid, excluido_en = now(),
		       actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid AND incluido`
	tag, err := tx.Exec(ctx, sacar, empresaID, salienteID, motivoExclusion, usuarioID)
	if err != nil {
		return false, fmt.Errorf("bancos: sacar el reemplazado: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, ErrReemisionFueraDeLibros
	}

	// El vigente apunta al que salió. La herencia va en el mismo UPDATE y SOLO si el vigente no
	// tenía clasificación: pisar una clasificación puesta a mano con la del otro destruiría trabajo
	// sin avisar, y el caso que motivó esto es justamente el contrario —el nuevo entra en blanco—.
	const enlazar = `
		UPDATE movimiento_bancario v
		   SET reemplaza_a = $3::uuid, reemplazo_en = now(),
		       reemplazo_por = $4::uuid, reemplazo_motivo = $5,
		       concepto_id = CASE WHEN $6::bool THEN s.concepto_id ELSE v.concepto_id END,
		       clasificacion_id = CASE WHEN $6::bool THEN s.clasificacion_id ELSE v.clasificacion_id END,
		       departamento_id = CASE WHEN $6::bool THEN s.departamento_id ELSE v.departamento_id END,
		       sede_id = CASE WHEN $6::bool THEN s.sede_id ELSE v.sede_id END,
		       estado_clasificacion = CASE WHEN $6::bool THEN s.estado_clasificacion
		                                   ELSE v.estado_clasificacion END,
		       actualizado_en = now()
		  FROM movimiento_bancario s
		 WHERE v.empresa_id = $1::uuid AND v.id = $2::uuid
		   AND s.empresa_id = $1::uuid AND s.id = $3::uuid
		   AND v.incluido AND v.reemplaza_a IS NULL`
	tag, err = tx.Exec(ctx, enlazar, empresaID, vigenteID, salienteID, usuarioID, motivo, heredar)
	if err != nil {
		return false, fmt.Errorf("bancos: enlazar la reemisión: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, ErrReemisionYaEnlazada
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("bancos: confirmar la reemisión: %w", err)
	}
	return heredar, nil
}

// DeshacerReemision rompe el enlace y devuelve a los libros el que había salido.
//
// Las dos cosas juntas, por la misma razón que al enlazar: romper el enlace sin devolver el
// movimiento lo dejaría fuera de los libros con el motivo de un enlace que ya no existe, y nadie
// podría saber si eso fue a propósito.
//
// NO revierte la herencia de clasificación. Si el vigente heredó y después alguien afinó esa
// clasificación, devolverla al estado anterior borraría ese trabajo; y si nadie la tocó, la
// clasificación heredada sigue siendo la correcta para ese movimiento.
func (r *pgRepository) DeshacerReemision(ctx context.Context, empresaID, vigenteID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bancos: abrir transacción de deshacer: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var salienteID string
	const leer = `
		SELECT reemplaza_a::text FROM movimiento_bancario
		 WHERE empresa_id = $1::uuid AND id = $2::uuid AND reemplaza_a IS NOT NULL
		 FOR UPDATE`
	if err := tx.QueryRow(ctx, leer, empresaID, vigenteID).Scan(&salienteID); err != nil {
		if err == pgx.ErrNoRows {
			return ErrReemisionNoExiste
		}
		return fmt.Errorf("bancos: leer la reemisión: %w", err)
	}

	const soltar = `
		UPDATE movimiento_bancario
		   SET reemplaza_a = NULL, reemplazo_en = NULL, reemplazo_por = NULL,
		       reemplazo_motivo = NULL, actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid`
	if _, err := tx.Exec(ctx, soltar, empresaID, vigenteID); err != nil {
		return fmt.Errorf("bancos: soltar la reemisión: %w", err)
	}

	// Solo se devuelve si salió a mano: si entretanto se revirtió su carga, devolverlo acá dejaría
	// una fila en los libros que ningún estado de cuenta respalda.
	const devolver = `
		UPDATE movimiento_bancario
		   SET incluido = true, excluido_motivo = NULL, excluido_por = NULL, excluido_en = NULL,
		       actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid
		   AND NOT incluido AND NOT excluido_por_reversa`
	if _, err := tx.Exec(ctx, devolver, empresaID, salienteID); err != nil {
		return fmt.Errorf("bancos: devolver el reemplazado: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("bancos: confirmar deshacer: %w", err)
	}
	return nil
}

// Reemisiones lista los enlaces ya hechos, el más reciente primero.
func (r *pgRepository) Reemisiones(ctx context.Context, empresaID string) ([]Reemision, error) {
	const q = `
		SELECT COALESCE(NULLIF(v.debito, 0), v.credito, 0)::text,
		       COALESCE(v.debito, 0) > 0,
		       COALESCE(cb.alias, ''), COALESCE(bk.nombre, ''),
		       COALESCE(v.reemplazo_motivo, ''), COALESCE(u.nombre, ''),
		       COALESCE(v.reemplazo_en::text, ''),
		       v.id::text, v.fecha::text, COALESCE(v.documento, ''), COALESCE(v.descripcion, ''),
		       CASE WHEN co_v.nombre IS NULL AND cl_v.nombre IS NULL THEN ''
		            ELSE concat_ws(' › ', NULLIF(co_v.nombre, ''), NULLIF(cl_v.nombre, '')) END,
		       v.estado_clasificacion, v.incluido,
		       s.id::text, s.fecha::text, COALESCE(s.documento, ''), COALESCE(s.descripcion, ''),
		       CASE WHEN co_s.nombre IS NULL AND cl_s.nombre IS NULL THEN ''
		            ELSE concat_ws(' › ', NULLIF(co_s.nombre, ''), NULLIF(cl_s.nombre, '')) END,
		       s.estado_clasificacion, s.incluido,
		       -- Los dos terminaron con la misma partida. Se deriva en vez de guardarse para que no
		       -- pueda desincronizarse con la clasificación real si alguien la cambia después; el
		       -- precio es que no distingue «la heredó» de «ya venían iguales», y por eso el campo
		       -- se llama misma_clasificacion y no «heredada».
		       (v.concepto_id IS NOT DISTINCT FROM s.concepto_id
		        AND v.clasificacion_id IS NOT DISTINCT FROM s.clasificacion_id
		        AND v.clasificacion_id IS NOT NULL)
		FROM movimiento_bancario v
		JOIN movimiento_bancario s ON s.id = v.reemplaza_a
		JOIN cuenta_bancaria cb ON cb.id = v.cuenta_bancaria_id
		JOIN banco bk ON bk.id = cb.banco_id
		LEFT JOIN usuario u ON u.id = v.reemplazo_por
		LEFT JOIN concepto co_v ON co_v.id = v.concepto_id
		LEFT JOIN clasificacion cl_v ON cl_v.id = v.clasificacion_id
		LEFT JOIN concepto co_s ON co_s.id = s.concepto_id
		LEFT JOIN clasificacion cl_s ON cl_s.id = s.clasificacion_id
		WHERE v.empresa_id = $1::uuid AND v.reemplaza_a IS NOT NULL
		ORDER BY v.reemplazo_en DESC NULLS LAST`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar reemisiones: %w", err)
	}
	defer rows.Close()
	out := make([]Reemision, 0, 8)
	for rows.Next() {
		var x Reemision
		if err := rows.Scan(&x.Monto, &x.EsDebito, &x.Cuenta, &x.Banco, &x.Motivo,
			&x.EnlazadoPor, &x.EnlazadoEn,
			&x.Vigente.ID, &x.Vigente.Fecha, &x.Vigente.Documento, &x.Vigente.Descripcion,
			&x.Vigente.Clasificacion, &x.Vigente.Estado, &x.Vigente.Incluido,
			&x.Reemplazado.ID, &x.Reemplazado.Fecha, &x.Reemplazado.Documento,
			&x.Reemplazado.Descripcion, &x.Reemplazado.Clasificacion, &x.Reemplazado.Estado,
			&x.Reemplazado.Incluido, &x.MismaClasificacion); err != nil {
			return nil, fmt.Errorf("bancos: scan reemisión: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
