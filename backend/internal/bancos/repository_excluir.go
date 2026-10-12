package bancos

// Sacar de los libros UN movimiento, y encontrar los que conviene sacar.
//
// `incluido` tenía veinte lectores y un solo escritor —la reversa de una importación entera—, así
// que un duplicado suelto obligaba a tirar abajo la carga completa. Acá está el segundo escritor.
//
// Nada de esto borra: `incluido = false` es la misma puerta que usa la reversa, y las dos conviven
// sin pisarse porque la reversa solo toca lo que está incluido y solo devuelve lo que ella misma
// marcó (ver repository_reversa.go).

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ExcluirMovimiento saca un movimiento de los libros dejando quién, cuándo y por qué.
//
// `excluido_por_reversa` se deja en false a propósito: es lo que distingue esta exclusión de la de
// una carga revertida, y lo que hace que deshacer una reversa NO devuelva este movimiento a los
// libros. Sin esa distinción, corregir un duplicado a mano y después deshacer una reversa
// cualquiera volvería a meter la plata duplicada.
func (r *pgRepository) ExcluirMovimiento(ctx context.Context, empresaID, movID, motivo, usuarioID string) error {
	const q = `
		UPDATE movimiento_bancario
		   SET incluido = false, excluido_por_reversa = false,
		       excluido_motivo = $3, excluido_por = $4::uuid, excluido_en = now(),
		       actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid AND incluido`
	tag, err := r.pool.Exec(ctx, q, empresaID, movID, motivo, usuarioID)
	if err != nil {
		return fmt.Errorf("bancos: excluir movimiento: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// O no existe en esta empresa, o ya estaba fuera de los libros. Las dos cosas significan
		// lo mismo para quien lo pidió: no había nada que sacar.
		return ErrMovimientoNoExcluible
	}
	return nil
}

// IncluirMovimiento lo devuelve a los libros.
//
// Solo los excluidos A MANO: `AND NOT excluido_por_reversa`. Devolver desde acá uno que salió con
// su carga lo dejaría incluido mientras su importación sigue marcada como revertida, y el libro
// tendría una fila que ningún estado de cuenta respalda.
func (r *pgRepository) IncluirMovimiento(ctx context.Context, empresaID, movID string) error {
	const q = `
		UPDATE movimiento_bancario
		   SET incluido = true, excluido_motivo = NULL, excluido_por = NULL, excluido_en = NULL,
		       actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid
		   AND NOT incluido AND NOT excluido_por_reversa`
	tag, err := r.pool.Exec(ctx, q, empresaID, movID)
	if err != nil {
		return fmt.Errorf("bancos: incluir movimiento: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMovimientoNoIncluible
	}
	return nil
}

// ExcluidosAMano lista lo que está fuera de los libros por decisión de alguien, con su rastro.
//
// Es la contraparte obligatoria de poder excluir: plata sacada de los libros que no se puede
// listar es plata perdida. Quien cierra el mes tiene que poder ver esta lista entera.
func (r *pgRepository) ExcluidosAMano(ctx context.Context, empresaID string) ([]MovimientoExcluido, error) {
	const q = `
		SELECT m.id::text, m.fecha::text, COALESCE(m.documento, ''),
		       COALESCE(m.debito, 0)::text, COALESCE(m.credito, 0)::text,
		       COALESCE(m.descripcion, ''), COALESCE(c.alias, ''), COALESCE(b.nombre, ''),
		       COALESCE(m.excluido_motivo, ''), COALESCE(u.nombre, ''),
		       COALESCE(m.excluido_en::text, '')
		FROM movimiento_bancario m
		JOIN cuenta_bancaria c ON c.id = m.cuenta_bancaria_id
		JOIN banco b ON b.id = c.banco_id
		LEFT JOIN usuario u ON u.id = m.excluido_por
		WHERE m.empresa_id = $1::uuid AND NOT m.incluido AND NOT m.excluido_por_reversa
		ORDER BY m.excluido_en DESC NULLS LAST, m.fecha DESC`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: listar excluidos: %w", err)
	}
	defer rows.Close()
	out := make([]MovimientoExcluido, 0, 16)
	for rows.Next() {
		var m MovimientoExcluido
		if err := rows.Scan(&m.ID, &m.Fecha, &m.Documento, &m.Debito, &m.Credito, &m.Descripcion,
			&m.Cuenta, &m.Banco, &m.Motivo, &m.ExcluidoPor, &m.ExcluidoEn); err != nil {
			return nil, fmt.Errorf("bancos: listar excluidos: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// duplicadosSQL busca pares de movimientos que podrían ser el mismo hecho cargado dos veces.
//
// EL NÚCLEO es misma cuenta + misma fecha + mismo monto, con consecutivos DISTINTOS. Sobre eso se
// calculan tres señales y NO se decide nada: se devuelven para que una persona mire.
//
// Que no decida solo es el punto. Probada la regla «mismos cuatro dígitos finales» a secas sobre
// los datos reales, devolvió 84 pares —todos de Popular y Davivienda— que eran comisiones y SINPE
// legítimos repetidos el mismo día, con la descripción idéntica: coincidían en los cuatro dígitos
// por casualidad. Excluirlos habría sacado plata real de los libros.
//
// Las tres señales, que es lo que separa el caso del BAC de esos 84:
//   - mismos_4: los últimos cuatro dígitos del consecutivo coinciden. Es la base que el banco
//     conserva cuando cambia el formato.
//   - largo_distinto: el consecutivo tiene distinta cantidad de dígitos. Es la huella de un cambio
//     de formato, no de dos transacciones.
//   - desc_distinta: la descripción cambió. Dos movimientos legítimos iguales el mismo día traen
//     la MISMA descripción; un duplicado por recarga trae otra.
//
// `a.id < b.id` evita que cada par salga dos veces, una por cada lado.
const duplicadosSQL = `
	SELECT a.id::text, a.fecha::text, COALESCE(a.documento,''), COALESCE(a.descripcion,''),
	       b.id::text, COALESCE(b.documento,''), COALESCE(b.descripcion,''),
	       (COALESCE(a.debito,0) + COALESCE(a.credito,0))::text,
	       COALESCE(a.debito,0) > 0,
	       COALESCE(c.alias,''), COALESCE(bk.nombre,''),
	       (right(regexp_replace(COALESCE(a.documento,''), '\D', '', 'g'), 4)
	          = right(regexp_replace(COALESCE(b.documento,''), '\D', '', 'g'), 4)
	        AND length(regexp_replace(COALESCE(a.documento,''), '\D', '', 'g')) >= 4) AS mismos_4,
	       (length(regexp_replace(COALESCE(a.documento,''), '\D', '', 'g'))
	          <> length(regexp_replace(COALESCE(b.documento,''), '\D', '', 'g'))) AS largo_distinto,
	       (COALESCE(a.descripcion,'') <> COALESCE(b.descripcion,'')) AS desc_distinta
	FROM movimiento_bancario a
	JOIN movimiento_bancario b
	  ON b.empresa_id = a.empresa_id
	 AND b.cuenta_bancaria_id = a.cuenta_bancaria_id
	 AND b.fecha = a.fecha
	 AND COALESCE(b.debito, 0)  = COALESCE(a.debito, 0)
	 AND COALESCE(b.credito, 0) = COALESCE(a.credito, 0)
	 AND COALESCE(b.documento, '') <> COALESCE(a.documento, '')
	 AND a.id < b.id
	 AND b.incluido
	JOIN cuenta_bancaria c ON c.id = a.cuenta_bancaria_id
	JOIN banco bk ON bk.id = c.banco_id
	WHERE a.empresa_id = $1::uuid AND a.incluido
	  AND (COALESCE(a.debito,0) + COALESCE(a.credito,0)) > 0
	  -- LOS CUATRO DÍGITOS FINALES SON OBLIGATORIOS, no una señal más.
	  --
	  -- Es la base que el banco conserva cuando le cambia el formato al resto del consecutivo, y
	  -- sin ella esto no es un detector: medido sobre setiembre de 2026, cruzar por fecha + monto
	  -- a secas devuelve 54.627 pares —las comisiones y los SINPE que se repiten dentro del mismo
	  -- día con el mismo importe— y ninguno es un duplicado. Una lista así no se revisa.
	  AND right(regexp_replace(COALESCE(a.documento,''), '\D', '', 'g'), 4)
	        = right(regexp_replace(COALESCE(b.documento,''), '\D', '', 'g'), 4)
	  AND length(regexp_replace(COALESCE(a.documento,''), '\D', '', 'g')) >= 4`

// DuplicadosSospechosos devuelve los pares candidatos de un rango de fechas.
//
// El rango es obligatorio en el service: cruzar treinta mil movimientos contra sí mismos sin acotar
// es una consulta que nadie quiere disparar desde una pantalla.
func (r *pgRepository) DuplicadosSospechosos(ctx context.Context, empresaID, desde, hasta string) ([]ParDuplicado, error) {
	q := duplicadosSQL + ` AND a.fecha >= $2::date AND a.fecha <= $3::date
		ORDER BY a.fecha DESC, (COALESCE(a.debito,0) + COALESCE(a.credito,0)) DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, desde, hasta)
	if err != nil {
		return nil, fmt.Errorf("bancos: buscar duplicados: %w", err)
	}
	defer rows.Close()
	out := make([]ParDuplicado, 0, 32)
	for rows.Next() {
		var p ParDuplicado
		if err := rows.Scan(&p.A.ID, &p.Fecha, &p.A.Documento, &p.A.Descripcion,
			&p.B.ID, &p.B.Documento, &p.B.Descripcion, &p.Monto, &p.EsDebito,
			&p.Cuenta, &p.Banco,
			&p.MismosUltimos4, &p.LargoDistinto, &p.DescripcionDistinta); err != nil {
			return nil, fmt.Errorf("bancos: buscar duplicados: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// MovimientoParaExcluir trae lo mínimo para confirmar antes de sacarlo: qué es y si ya está fuera.
func (r *pgRepository) MovimientoParaExcluir(ctx context.Context, empresaID, movID string) (MovimientoExcluido, error) {
	const q = `
		SELECT m.id::text, m.fecha::text, COALESCE(m.documento,''),
		       COALESCE(m.debito,0)::text, COALESCE(m.credito,0)::text,
		       COALESCE(m.descripcion,''), COALESCE(c.alias,''), COALESCE(b.nombre,''),
		       m.incluido, COALESCE(m.excluido_motivo,'')
		FROM movimiento_bancario m
		JOIN cuenta_bancaria c ON c.id = m.cuenta_bancaria_id
		JOIN banco b ON b.id = c.banco_id
		WHERE m.empresa_id = $1::uuid AND m.id = $2::uuid`
	var m MovimientoExcluido
	err := r.pool.QueryRow(ctx, q, empresaID, movID).
		Scan(&m.ID, &m.Fecha, &m.Documento, &m.Debito, &m.Credito, &m.Descripcion,
			&m.Cuenta, &m.Banco, &m.Incluido, &m.Motivo)
	if errors.Is(err, pgx.ErrNoRows) {
		return MovimientoExcluido{}, ErrMovimientoNoEncontrado
	}
	if err != nil {
		return MovimientoExcluido{}, fmt.Errorf("bancos: movimiento para excluir: %w", err)
	}
	return m, nil
}
