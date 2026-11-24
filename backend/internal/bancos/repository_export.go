package bancos

// Datos para exportación (Fase D): todos los movimientos del período (sin paginar),
// con el nombre del banco para derivar el Consecutivo Largo de Davivienda.

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// EncabezadoReporte trae la identificación de la empresa y de quien genera el reporte.
// Un reporte sin decir de quién es y quién lo emitió no se puede llevar a una reunión.
//
// `detalle` es la identificación secundaria. Hoy solo hay `tipo_legal`: la tabla `empresa` NO
// tiene cédula jurídica, y en un reporte financiero debería. Cuando se agregue, se suma acá.
func (r *pgRepository) EncabezadoReporte(ctx context.Context, empresaID, usuarioID string) (string, string, string, error) {
	const q = `
		SELECT e.nombre, COALESCE(e.tipo_legal, ''),
		       COALESCE((SELECT u.nombre FROM usuario u WHERE u.id = NULLIF($2,'')::uuid), '')
		FROM empresa e WHERE e.id = $1::uuid`
	var empresa, tipoLegal, usuario string
	if err := r.pool.QueryRow(ctx, q, empresaID, usuarioID).Scan(&empresa, &tipoLegal, &usuario); err != nil {
		return "", "", "", fmt.Errorf("bancos: encabezado de reporte: %w", err)
	}
	return empresa, tipoLegal, usuario, nil
}

// MovimientosParaExport usa el MISMO WHERE que la hoja de trabajo (`condicionesMovimientos`).
// Así lo que se exporta es exactamente lo que se ve en pantalla con esos filtros: si cada uno
// armara su propio filtro, el reporte y la pantalla dirían cosas distintas y nadie sabría cuál
// creer. La única condición extra es `incluido = true`: un duplicado excluido a mano no se
// exporta.
func (r *pgRepository) MovimientosParaExport(ctx context.Context, empresaID string, f FiltrosMovimientos) ([]MovimientoExport, error) {
	where, args := condicionesMovimientos(empresaID, f)
	q := `
		SELECT m.fecha, COALESCE(m.documento,''), COALESCE(m.descripcion,''),
		       COALESCE(b.nombre,''), COALESCE(cb.alias,''),
		       m.debito, m.credito, m.moneda_original, m.monto_crc,
		       COALESCE(co.nombre,''), COALESCE(cl.nombre,''),
		       m.estado_clasificacion, m.es_traslado,
		       -- El id de la clasificación no se muestra en ninguna hoja: sirve para nombrar el
		       -- archivo respetando el ORDEN en que el usuario eligió las clasificaciones en el
		       -- filtro. Por nombre no se puede: dos conceptos pueden tener la misma clasificación.
		       COALESCE(m.clasificacion_id::text,''),
		       -- EL REPARTO (mig 0094). Sin esto, dos caras del mismo movimiento salen con la
		       -- misma fecha, el mismo consecutivo y la misma descripción, y contra el estado de
		       -- cuenta se leen como un duplicado. El id del PRINCIPAL es lo que las agrupa: el
		       -- mismo COALESCE sirve para el original y para sus partes.
		       CASE WHEN m.dividido_de IS NOT NULL OR m.monto_antes_de_dividir IS NOT NULL
		            THEN COALESCE(m.dividido_de::text, m.id::text) ELSE '' END
		FROM movimiento_bancario m
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		LEFT JOIN banco b ON b.id = cb.banco_id
		WHERE ` + where + ` AND m.incluido = true
		ORDER BY m.fecha, m.id`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("bancos: movimientos para export: %w", err)
	}
	defer rows.Close()
	var out []MovimientoExport
	for rows.Next() {
		var (
			m         MovimientoExport
			fecha     time.Time
			deb, cred decimal.Decimal
			mcrc      decimal.Decimal
		)
		var principalDelReparto string
		if err := rows.Scan(&fecha, &m.Documento, &m.Descripcion, &m.Banco, &m.Cuenta,
			&deb, &cred, &m.Moneda, &mcrc, &m.Concepto, &m.Clasificacion, &m.Estado, &m.EsTraslado,
			&m.ClasificacionID, &principalDelReparto); err != nil {
			return nil, fmt.Errorf("bancos: scan export: %w", err)
		}
		// El código se deriva acá con la MISMA función que usa la pantalla: el que alguien lee en
		// la parrilla y el que sale en el .xlsx tienen que ser el mismo, o no sirven para cruzar.
		m.Reparto = CodigoDeReparto(principalDelReparto)
		m.Fecha = fecha.Format("2006-01-02")
		m.Debito = deb.String()
		m.Credito = cred.String()
		m.MontoCRC = mcrc.String()
		out = append(out, m)
	}
	return out, rows.Err()
}
