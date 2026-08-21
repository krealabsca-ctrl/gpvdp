package nomina

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Métodos de corrida del Repository (implementados por pgRepository).

const corridaCols = `c.id::text, c.anio, c.mes, c.tipo, c.estado, c.fecha_pago::text,
	(SELECT COUNT(*) FROM corrida_linea cl WHERE cl.corrida_id = c.id),
	c.total_bruto::text, c.total_ccss_obrero::text, c.total_renta::text, c.total_deducciones::text,
	c.total_adelanto::text, c.total_neto::text, c.total_patronal::text, c.total_provisiones::text,
	c.creado_en::text, COALESCE(c.aprobado_en::text, ''), COALESCE(c.pagado_en::text, '')`

func scanCorrida(row scanner) (Corrida, error) {
	var c Corrida
	err := row.Scan(&c.ID, &c.Anio, &c.Mes, &c.Tipo, &c.Estado, &c.FechaPago, &c.Empleados,
		&c.TotalBruto, &c.TotalCCSS, &c.TotalRenta, &c.TotalDeduc, &c.TotalAdel, &c.TotalNeto,
		&c.TotalPatr, &c.TotalProv, &c.CreadoEn, &c.AprobadoEn, &c.PagadoEn)
	return c, err
}

func (r *pgRepository) ListarCorridas(ctx context.Context, empresaID string, anio int) ([]Corrida, error) {
	q := "SELECT " + corridaCols + ` FROM corrida_nomina c
		WHERE c.empresa_id = $1::uuid AND c.anio = $2
		ORDER BY c.mes DESC, c.tipo`
	rows, err := r.pool.Query(ctx, q, empresaID, anio)
	if err != nil {
		return nil, fmt.Errorf("nomina: listar corridas: %w", err)
	}
	defer rows.Close()
	items := make([]Corrida, 0, 24)
	for rows.Next() {
		c, err := scanCorrida(rows)
		if err != nil {
			return nil, fmt.Errorf("nomina: scan corrida: %w", err)
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (r *pgRepository) CorridaPorID(ctx context.Context, empresaID, id string) (Corrida, error) {
	q := "SELECT " + corridaCols + " FROM corrida_nomina c WHERE c.empresa_id = $1::uuid AND c.id = $2::uuid"
	c, err := scanCorrida(r.pool.QueryRow(ctx, q, empresaID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Corrida{}, ErrCorridaNoEncontrada
	}
	if err != nil {
		return Corrida{}, fmt.Errorf("nomina: corrida por id: %w", err)
	}
	return c, nil
}

func (r *pgRepository) CrearCorrida(ctx context.Context, empresaID string, anio, mes int, tipo, fechaPago string, parametros []byte, usuarioID string) (Corrida, error) {
	q := `
		WITH ins AS (
			INSERT INTO corrida_nomina (empresa_id, anio, mes, tipo, fecha_pago, parametros, creado_por)
			VALUES ($1::uuid, $2, $3, $4, $5::date, $6::jsonb, $7::uuid)
			RETURNING *
		)
		SELECT ` + corridaCols + ` FROM ins c`
	c, err := scanCorrida(r.pool.QueryRow(ctx, q, empresaID, anio, mes, tipo, fechaPago, parametros, usuarioID))
	if esViolacionUnica(err) {
		return Corrida{}, ErrCorridaDuplicada
	}
	if err != nil {
		return Corrida{}, fmt.Errorf("nomina: crear corrida: %w", err)
	}
	return c, nil
}

const lineaCols = `cl.id::text, cl.empleado_id::text, cl.nombre, cl.identificacion, COALESCE(cl.iban, ''),
	COALESCE(cl.departamento, ''), COALESCE(cl.puesto, ''), cl.salario_base::text, cl.hijos, cl.conyuge,
	cl.bruto::text, cl.base_ccss::text, cl.base_renta::text, cl.ccss_obrero::text, cl.renta::text,
	cl.deducciones::text, cl.adelanto::text, cl.neto::text, cl.patronal::text,
	cl.prov_aguinaldo::text, cl.prov_vacaciones::text, cl.prov_cesantia::text,
	cl.tratamiento, cl.detalle`

func (r *pgRepository) LineasCorrida(ctx context.Context, empresaID, corridaID string) ([]LineaCorrida, error) {
	q := "SELECT " + lineaCols + ` FROM corrida_linea cl
		WHERE cl.empresa_id = $1::uuid AND cl.corrida_id = $2::uuid ORDER BY cl.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, corridaID)
	if err != nil {
		return nil, fmt.Errorf("nomina: listar líneas: %w", err)
	}
	defer rows.Close()
	items := make([]LineaCorrida, 0, 32)
	for rows.Next() {
		var l LineaCorrida
		var detalle []byte
		if err := rows.Scan(&l.ID, &l.EmpleadoID, &l.Nombre, &l.Identificacion, &l.IBAN,
			&l.Departamento, &l.Puesto, &l.SalarioBase, &l.Hijos, &l.Conyuge,
			&l.Bruto, &l.BaseCCSS, &l.BaseRenta, &l.CCSSObrero, &l.Renta,
			&l.Deducciones, &l.Adelanto, &l.Neto, &l.Patronal,
			&l.ProvAguinaldo, &l.ProvVacaciones, &l.ProvCesantia, &l.Tratamiento, &detalle); err != nil {
			return nil, fmt.Errorf("nomina: scan línea: %w", err)
		}
		if err := json.Unmarshal(detalle, &l.Detalle); err != nil {
			return nil, fmt.Errorf("nomina: detalle corrupto: %w", err)
		}
		items = append(items, l)
	}
	return items, rows.Err()
}

// GuardarLineas reemplaza las colillas de una corrida EN BORRADOR y actualiza los totales
// y el snapshot de parámetros de la cabecera — todo o nada (una transacción).
func (r *pgRepository) GuardarLineas(ctx context.Context, empresaID, corridaID string, lineas []LineaCorrida, totales TotalesCorrida, parametros []byte) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("nomina: tx líneas: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`DELETE FROM corrida_linea WHERE empresa_id = $1::uuid AND corrida_id = $2::uuid`,
		empresaID, corridaID); err != nil {
		return fmt.Errorf("nomina: limpiar líneas: %w", err)
	}
	const insQ = `
		INSERT INTO corrida_linea (empresa_id, corrida_id, empleado_id, nombre, identificacion, iban,
			departamento, puesto, salario_base, hijos, conyuge, bruto, base_ccss, base_renta, ccss_obrero,
			renta, deducciones, adelanto, neto, patronal, prov_aguinaldo, prov_vacaciones, prov_cesantia,
			tratamiento, detalle)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
			$9::numeric, $10, $11, $12::numeric, $13::numeric, $14::numeric, $15::numeric, $16::numeric,
			$17::numeric, $18::numeric, $19::numeric, $20::numeric, $21::numeric, $22::numeric, $23::numeric,
			$24, $25::jsonb)`
	for _, l := range lineas {
		detalle, err := json.Marshal(l.Detalle)
		if err != nil {
			return fmt.Errorf("nomina: serializar detalle: %w", err)
		}
		if _, err := tx.Exec(ctx, insQ, empresaID, corridaID, l.EmpleadoID, l.Nombre, l.Identificacion,
			l.IBAN, l.Departamento, l.Puesto, l.SalarioBase, l.Hijos, l.Conyuge, l.Bruto, l.BaseCCSS,
			l.BaseRenta, l.CCSSObrero, l.Renta, l.Deducciones, l.Adelanto, l.Neto, l.Patronal,
			l.ProvAguinaldo, l.ProvVacaciones, l.ProvCesantia, l.Tratamiento, detalle); err != nil {
			return fmt.Errorf("nomina: insertar línea: %w", err)
		}
	}
	// El WHERE re-verifica BORRADOR: si otra sesión aprobó en paralelo, nada se pisa.
	tag, err := tx.Exec(ctx, `
		UPDATE corrida_nomina SET parametros = $3::jsonb,
			total_bruto = $4, total_ccss_obrero = $5, total_renta = $6, total_deducciones = $7,
			total_adelanto = $8, total_neto = $9, total_patronal = $10, total_provisiones = $11
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'BORRADOR'`,
		empresaID, corridaID, parametros, totales.Bruto, totales.CCSSObrero, totales.Renta,
		totales.Deducciones, totales.Adelanto, totales.Neto, totales.Patronal, totales.Provisiones)
	if err != nil {
		return fmt.Errorf("nomina: totales corrida: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCorridaNoEditable
	}
	return tx.Commit(ctx)
}

// TotalesCorrida agrega los resultados de todas las líneas (decimal, nunca float).
type TotalesCorrida struct {
	Bruto, CCSSObrero, Renta, Deducciones, Adelanto, Neto, Patronal, Provisiones decimal.Decimal
}

// ---- Novedades ----

func (r *pgRepository) NovedadesCorrida(ctx context.Context, empresaID, corridaID string) ([]NovedadCorrida, error) {
	const q = `
		SELECT n.empleado_id::text, n.concepto_id::text, c.nombre, n.monto::text,
		       COALESCE(n.cantidad, 0)::text
		FROM corrida_novedad n JOIN concepto_nomina c ON c.id = n.concepto_id
		WHERE n.empresa_id = $1::uuid AND n.corrida_id = $2::uuid
		ORDER BY c.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, corridaID)
	if err != nil {
		return nil, fmt.Errorf("nomina: novedades: %w", err)
	}
	defer rows.Close()
	items := make([]NovedadCorrida, 0, 32)
	for rows.Next() {
		var n NovedadCorrida
		if err := rows.Scan(&n.EmpleadoID, &n.ConceptoID, &n.ConceptoNombre, &n.Monto, &n.Cantidad); err != nil {
			return nil, fmt.Errorf("nomina: scan novedad: %w", err)
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

// ReemplazarNovedades sustituye el set completo de novedades de la corrida (una transacción).
// Cada inserción re-valida contra la empresa: empleado activo y concepto INGRESO activo.
func (r *pgRepository) ReemplazarNovedades(ctx context.Context, empresaID, corridaID string, novedades []novedadValidada) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("nomina: tx novedades: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock de la cabecera + re-verificación de BORRADOR dentro de la MISMA tx: una
	// aprobación concurrente espera el lock o llega antes — nunca se mutan las novedades
	// de una corrida ya congelada (espejo de la guarda de GuardarLineas).
	var estado string
	err = tx.QueryRow(ctx,
		`SELECT estado FROM corrida_nomina WHERE empresa_id = $1::uuid AND id = $2::uuid FOR UPDATE`,
		empresaID, corridaID).Scan(&estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCorridaNoEncontrada
	}
	if err != nil {
		return fmt.Errorf("nomina: lock corrida: %w", err)
	}
	if estado != EstBorrador {
		return ErrCorridaNoEditable
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM corrida_novedad WHERE empresa_id = $1::uuid AND corrida_id = $2::uuid`,
		empresaID, corridaID); err != nil {
		return fmt.Errorf("nomina: limpiar novedades: %w", err)
	}
	const insQ = `
		INSERT INTO corrida_novedad (empresa_id, corrida_id, empleado_id, concepto_id, monto, cantidad)
		SELECT $1::uuid, $2::uuid, e.id, c.id, $5::numeric, NULLIF($6, 0)::numeric
		FROM empleado e
		JOIN concepto_nomina c ON c.id = $4::uuid AND c.empresa_id = $1::uuid AND c.tipo = 'INGRESO' AND c.activo
		WHERE e.id = $3::uuid AND e.empresa_id = $1::uuid AND e.activo`
	for _, n := range novedades {
		tag, err := tx.Exec(ctx, insQ, empresaID, corridaID, n.EmpleadoID, n.ConceptoID, n.Monto, n.Cantidad.String())
		if err != nil {
			return fmt.Errorf("nomina: insertar novedad: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNovedadInvalida
		}
	}
	return tx.Commit(ctx)
}

// novedadValidada es una novedad con el monto ya parseado (interna al paquete).
type novedadValidada struct {
	// Cantidad > 0 = la novedad se registró por HORAS (extra); el monto se deriva al calcular.
	Cantidad   decimal.Decimal
	EmpleadoID string
	ConceptoID string
	Monto      decimal.Decimal
}

// NovedadesParaCalc devuelve las novedades con las banderas de afectación de su concepto
// (el guardarraíl viaja con el concepto: las banderas de los de sistema están bloqueadas).
func (r *pgRepository) NovedadesParaCalc(ctx context.Context, empresaID, corridaID string) (map[string][]IngresoCalc, error) {
	const q = `
		SELECT n.empleado_id::text, c.nombre, n.monto::text, COALESCE(n.cantidad, 0)::text,
		       c.afecta_ccss, c.afecta_renta, c.afecta_aguinaldo
		FROM corrida_novedad n JOIN concepto_nomina c ON c.id = n.concepto_id
		WHERE n.empresa_id = $1::uuid AND n.corrida_id = $2::uuid
		ORDER BY c.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, corridaID)
	if err != nil {
		return nil, fmt.Errorf("nomina: novedades para cálculo: %w", err)
	}
	defer rows.Close()
	out := map[string][]IngresoCalc{}
	for rows.Next() {
		var empleadoID, monto, cantidad string
		var ing IngresoCalc
		if err := rows.Scan(&empleadoID, &ing.Nombre, &monto, &cantidad,
			&ing.AfectaCCSS, &ing.AfectaRenta, &ing.AfectaAguinaldo); err != nil {
			return nil, fmt.Errorf("nomina: scan novedad calc: %w", err)
		}
		if ing.Monto, err = decimal.NewFromString(monto); err != nil {
			return nil, fmt.Errorf("nomina: monto de novedad corrupto: %w", err)
		}
		if ing.Horas, err = decimal.NewFromString(cantidad); err != nil {
			return nil, fmt.Errorf("nomina: cantidad de novedad corrupta: %w", err)
		}
		out[empleadoID] = append(out[empleadoID], ing)
	}
	return out, rows.Err()
}

// DeduccionesParaCalc devuelve las deducciones recurrentes vigentes de toda la empresa
// (activas y con saldo pendiente), agrupadas por empleado, para la liquidación.
// El saldo disponible RESTA lo ya comprometido en documentos APROBADOS sin pagar —
// corridas Y finiquitos: dos documentos aprobados jamás retienen más que la deuda real
// (el saldo en DB solo se decrementa al pagar, pero el compromiso cuenta al aprobar).
func (r *pgRepository) DeduccionesParaCalc(ctx context.Context, empresaID string) (map[string][]DeduccionCalc, error) {
	const q = `
		SELECT de.empleado_id::text, de.id::text, de.etiqueta, de.cuota::text,
			CASE WHEN de.saldo_restante IS NULL THEN ''
				ELSE (de.saldo_restante - COALESCE(comp.monto, 0))::text END,
			de.prioridad, de.frecuencia
		FROM deduccion_empleado de
		LEFT JOIN (
			SELECT ded_id, SUM(monto) AS monto FROM (
				SELECT (elem->>'deduccion_id')::uuid AS ded_id, (elem->>'monto')::numeric AS monto
				FROM corrida_linea cl
				JOIN corrida_nomina co ON co.id = cl.corrida_id
				CROSS JOIN LATERAL jsonb_array_elements(cl.detalle) elem
				WHERE co.empresa_id = $1::uuid AND co.estado = 'APROBADA'
					AND elem->>'tipo' = 'DEDUCCION' AND COALESCE(elem->>'deduccion_id', '') <> ''
				UNION ALL
				SELECT (elem->>'deduccion_id')::uuid AS ded_id, (elem->>'monto')::numeric AS monto
				FROM finiquito fq
				CROSS JOIN LATERAL jsonb_array_elements(fq.detalle) elem
				WHERE fq.empresa_id = $1::uuid AND fq.estado = 'APROBADO'
					AND elem->>'tipo' = 'DEDUCCION' AND COALESCE(elem->>'deduccion_id', '') <> ''
			) t GROUP BY ded_id
		) comp ON comp.ded_id = de.id
		WHERE de.empresa_id = $1::uuid AND de.activo
			AND (de.saldo_restante IS NULL OR de.saldo_restante - COALESCE(comp.monto, 0) > 0)`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("nomina: deducciones para cálculo: %w", err)
	}
	defer rows.Close()
	out := map[string][]DeduccionCalc{}
	for rows.Next() {
		var empleadoID, cuota, saldo string
		var d DeduccionCalc
		if err := rows.Scan(&empleadoID, &d.ID, &d.Etiqueta, &cuota, &saldo, &d.Prioridad, &d.Frecuencia); err != nil {
			return nil, fmt.Errorf("nomina: scan deducción calc: %w", err)
		}
		if d.Cuota, err = decimal.NewFromString(cuota); err != nil {
			return nil, fmt.Errorf("nomina: cuota corrupta: %w", err)
		}
		if saldo != "" {
			s, err := decimal.NewFromString(saldo)
			if err != nil {
				return nil, fmt.Errorf("nomina: saldo corrupto: %w", err)
			}
			d.SaldoRestante = &s
		}
		out[empleadoID] = append(out[empleadoID], d)
	}
	return out, rows.Err()
}

// AdelantosPagadosDelMes devuelve, por empleado, el neto de la corrida ADELANTO del mes
// en estado APROBADA o PAGADA que la liquidación debe descontar, MENOS lo que un finiquito
// APROBADO/PAGADO del mismo mes ya retuvo por ese adelanto (sin esta resta, el empleado
// pagaría el adelanto dos veces: en el finiquito y en la liquidación).
//
// Solo cuenta lo que se pagó como ADELANTO: el pago del día 15 de un empleado QUINCENAL
// (tratamiento QUINCENA_1) es un SALARIO por su mitad del mes, no un anticipo, y por eso
// jamás se le descuenta en la segunda quincena.
func (r *pgRepository) AdelantosPagadosDelMes(ctx context.Context, empresaID string, anio, mes int) (map[string]decimal.Decimal, error) {
	const q = `
		SELECT cl.empleado_id::text,
			GREATEST(0, cl.neto - COALESCE((
				SELECT SUM((elem->>'monto')::numeric)
				FROM finiquito fq CROSS JOIN LATERAL jsonb_array_elements(fq.detalle) elem
				WHERE fq.empresa_id = $1::uuid AND fq.empleado_id = cl.empleado_id
					AND fq.estado IN ('APROBADO', 'PAGADO')
					AND EXTRACT(YEAR FROM fq.fecha_salida) = $2 AND EXTRACT(MONTH FROM fq.fecha_salida) = $3
					AND elem->>'tipo' = 'ADELANTO'), 0))::numeric(14,2)::text
		FROM corrida_linea cl JOIN corrida_nomina c ON c.id = cl.corrida_id
		WHERE c.empresa_id = $1::uuid AND c.anio = $2 AND c.mes = $3
			AND c.tipo = 'ADELANTO' AND c.estado IN ('APROBADA', 'PAGADA')
			AND cl.tratamiento = 'ADELANTO'`
	rows, err := r.pool.Query(ctx, q, empresaID, anio, mes)
	if err != nil {
		return nil, fmt.Errorf("nomina: adelantos del mes: %w", err)
	}
	defer rows.Close()
	out := map[string]decimal.Decimal{}
	for rows.Next() {
		var empleadoID, neto string
		if err := rows.Scan(&empleadoID, &neto); err != nil {
			return nil, fmt.Errorf("nomina: scan adelanto: %w", err)
		}
		d, err := decimal.NewFromString(neto)
		if err != nil {
			return nil, fmt.Errorf("nomina: adelanto corrupto: %w", err)
		}
		out[empleadoID] = d
	}
	return out, rows.Err()
}

// LineasPlanillaDelMes agrega, por empleado, las bases y cargas de TODAS las colillas
// congeladas (APROBADA/PAGADA) del mes: así la planilla CCSS reporta el salario mensual
// íntegro también cuando se pagó en dos quincenas.
func (r *pgRepository) LineasPlanillaDelMes(ctx context.Context, empresaID string, anio, mes int) ([]LineaCorrida, error) {
	// Se agrupa por empleado (NO por nombre): la colilla congela el nombre del momento, así
	// que si la ficha se corrigió entre las dos quincenas el mismo trabajador aparecería dos
	// veces en la planilla. Se reporta el último nombre registrado.
	const q = `
		SELECT MAX(cl.nombre), cl.identificacion,
			SUM(cl.base_ccss)::text, SUM(cl.ccss_obrero)::text, SUM(cl.patronal)::text
		FROM corrida_linea cl JOIN corrida_nomina c ON c.id = cl.corrida_id
		WHERE cl.empresa_id = $1::uuid AND c.anio = $2 AND c.mes = $3
			AND c.estado IN ('APROBADA', 'PAGADA')
		GROUP BY cl.empleado_id, cl.identificacion
		HAVING SUM(cl.base_ccss) > 0
		ORDER BY MAX(cl.nombre)`
	rows, err := r.pool.Query(ctx, q, empresaID, anio, mes)
	if err != nil {
		return nil, fmt.Errorf("nomina: líneas de planilla del mes: %w", err)
	}
	defer rows.Close()
	items := make([]LineaCorrida, 0, 32)
	for rows.Next() {
		var l LineaCorrida
		if err := rows.Scan(&l.Nombre, &l.Identificacion, &l.BaseCCSS, &l.CCSSObrero, &l.Patronal); err != nil {
			return nil, fmt.Errorf("nomina: scan planilla: %w", err)
		}
		items = append(items, l)
	}
	return items, rows.Err()
}

// RentaRetenidaPrimeraQuincena devuelve, por empleado, el impuesto al salario que ya se
// retuvo en la 1ª quincena APROBADA/PAGADA del mes: la 2ª lo descuenta del impuesto del
// mes real (los tramos son mensuales, así que el ajuste se hace al cierre).
func (r *pgRepository) RentaRetenidaPrimeraQuincena(ctx context.Context, empresaID string, anio, mes int) (map[string]decimal.Decimal, error) {
	const q = `
		SELECT cl.empleado_id::text, cl.renta::text
		FROM corrida_linea cl JOIN corrida_nomina c ON c.id = cl.corrida_id
		WHERE c.empresa_id = $1::uuid AND c.anio = $2 AND c.mes = $3
			AND c.tipo = 'ADELANTO' AND c.estado IN ('APROBADA', 'PAGADA')
			AND cl.tratamiento = 'QUINCENA_1' AND cl.renta > 0`
	rows, err := r.pool.Query(ctx, q, empresaID, anio, mes)
	if err != nil {
		return nil, fmt.Errorf("nomina: renta retenida en la 1ª quincena: %w", err)
	}
	defer rows.Close()
	out := map[string]decimal.Decimal{}
	for rows.Next() {
		var empleadoID, renta string
		if err := rows.Scan(&empleadoID, &renta); err != nil {
			return nil, fmt.Errorf("nomina: scan renta retenida: %w", err)
		}
		d, err := decimal.NewFromString(renta)
		if err != nil {
			return nil, fmt.Errorf("nomina: renta retenida corrupta: %w", err)
		}
		out[empleadoID] = d
	}
	return out, rows.Err()
}

// ---- Transiciones de estado ----

// AprobarCorrida congela el BORRADOR con la guarda cruzada ADELANTO↔LIQUIDACIÓN evaluada
// ATÓMICAMENTE en el mismo UPDATE (sin ventana de carrera): una liquidación no se aprueba
// con el adelanto del mes en borrador, y un adelanto no se aprueba con la liquidación del
// mes ya aprobada/pagada (jamás se descontaría: el mes se pagaría 1.5 veces).
func (r *pgRepository) AprobarCorrida(ctx context.Context, empresaID, id, usuarioID string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE corrida_nomina AS c SET estado = 'APROBADA', aprobado_por = $3::uuid, aprobado_en = now()
		WHERE c.empresa_id = $1::uuid AND c.id = $2::uuid AND c.estado = 'BORRADOR'
			AND NOT EXISTS (
				SELECT 1 FROM corrida_nomina h
				WHERE h.empresa_id = c.empresa_id AND h.anio = c.anio AND h.mes = c.mes AND h.id <> c.id
					AND ((c.tipo = 'LIQUIDACION' AND h.tipo = 'ADELANTO' AND h.estado = 'BORRADOR')
						OR (c.tipo = 'ADELANTO' AND h.tipo = 'LIQUIDACION' AND h.estado IN ('APROBADA', 'PAGADA'))))`,
		empresaID, id, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("nomina: aprobar corrida: %w", err)
	}
	return tag.RowsAffected(), nil
}

// LiquidacionCerradaDelMes indica si la liquidación del mes ya está APROBADA o PAGADA
// (bloquea crear/aprobar un adelanto de ese mes: jamás se descontaría).
func (r *pgRepository) LiquidacionCerradaDelMes(ctx context.Context, empresaID string, anio, mes int) (bool, error) {
	var existe bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM corrida_nomina
			WHERE empresa_id = $1::uuid AND anio = $2 AND mes = $3
				AND tipo = 'LIQUIDACION' AND estado IN ('APROBADA', 'PAGADA'))`,
		empresaID, anio, mes).Scan(&existe)
	if err != nil {
		return false, fmt.Errorf("nomina: liquidación cerrada del mes: %w", err)
	}
	return existe, nil
}

// TieneNetoNegativo indica si alguna colilla de la corrida quedó con neto < 0 (se corrige
// en borrador; jamás se congela un depósito negativo).
func (r *pgRepository) TieneNetoNegativo(ctx context.Context, empresaID, corridaID string) (bool, error) {
	var existe bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM corrida_linea
			WHERE empresa_id = $1::uuid AND corrida_id = $2::uuid AND neto < 0)`,
		empresaID, corridaID).Scan(&existe)
	if err != nil {
		return false, fmt.Errorf("nomina: neto negativo: %w", err)
	}
	return existe, nil
}

// AdelantosSinColilla indica si hay adelantos APROBADOS/PAGADOS del mes cuyos empleados no
// aparecen en la liquidación: ese salario pagado quedaría sin cotizar CCSS ni descontarse —
// la aprobación se bloquea hasta resolverlo. Solo exime un FINIQUITO APROBADO/PAGADO cuya
// fecha de salida cae en el MISMO mes del adelanto: únicamente ese finiquito recupera este
// adelanto (AdelantoPendienteEmpleado lo busca por año y mes de la salida). Un finiquito de
// otro mes no lo descuenta y por tanto no exime.
func (r *pgRepository) AdelantosSinColilla(ctx context.Context, empresaID string, anio, mes int, liquidacionID string) (bool, error) {
	var existe bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM corrida_linea cl
			JOIN corrida_nomina a ON a.id = cl.corrida_id
			WHERE a.empresa_id = $1::uuid AND a.anio = $2 AND a.mes = $3
				AND a.tipo = 'ADELANTO' AND a.estado IN ('APROBADA', 'PAGADA')
				-- Solo los adelantos verdaderos quedan por recuperar: una 1ª quincena de
				-- pago quincenal ya cotizó CCSS y renta por sí misma.
				AND cl.tratamiento = 'ADELANTO'
				AND NOT EXISTS (SELECT 1 FROM corrida_linea l2
					WHERE l2.corrida_id = $4::uuid AND l2.empleado_id = cl.empleado_id)
				AND NOT EXISTS (SELECT 1 FROM finiquito fq
					WHERE fq.empresa_id = $1::uuid AND fq.empleado_id = cl.empleado_id
						AND fq.estado IN ('APROBADO', 'PAGADO')
						AND EXTRACT(YEAR FROM fq.fecha_salida) = $2
						AND EXTRACT(MONTH FROM fq.fecha_salida) = $3))`,
		empresaID, anio, mes, liquidacionID).Scan(&existe)
	if err != nil {
		return false, fmt.Errorf("nomina: adelantos sin colilla: %w", err)
	}
	return existe, nil
}

// EmpleadoCorrida es un empleado elegible para la corrida con la proporción del mes que
// efectivamente laboró (base 30 días): 1 = mes completo.
type EmpleadoCorrida struct {
	Empleado
	FraccionMes string
}

// EmpleadosParaCorrida devuelve los empleados que entran a la corrida:
//   - LIQUIDACION: los activos MÁS los dados de baja cuyo último día cae en el mes (su
//     salario devengado debe pagarse y cotizar CCSS — sin esto quedaría sin declarar).
//   - ADELANTO (día 15): activos. Se excluye a quien ya tiene finiquito aprobado/pagado
//     SOLO si su jornada no es quincenal — a un mensual no se le adelanta salario porque
//     ese anticipo no tendría de dónde descontarse, pero a un quincenal se le debe su
//     salario de la primera mitad del mes, que es un pago definitivo, no un anticipo.
//
// La fracción prorratea el mes cuando el empleado ingresó o salió a mitad de período.
func (r *pgRepository) EmpleadosParaCorrida(ctx context.Context, empresaID string, anio, mes int, tipo string) ([]EmpleadoCorrida, error) {
	q := `
		WITH rango AS (
			SELECT make_date($2, $3, 1) AS ini,
				(make_date($2, $3, 1) + INTERVAL '1 month - 1 day')::date AS fin
		)
		SELECT ` + empleadoCols + `,
			(LEAST(30, GREATEST(0,
				(LEAST(r.fin, COALESCE(e.fecha_salida, r.fin)) - GREATEST(r.ini, e.fecha_ingreso)) + 1
			))::numeric / 30)::numeric(8,6)::text
		FROM empleado e
		LEFT JOIN departamento d ON d.id = e.departamento_id
		CROSS JOIN rango r
		WHERE e.empresa_id = $1::uuid
			AND e.fecha_ingreso <= r.fin
			AND (e.fecha_salida IS NULL OR e.fecha_salida >= r.ini)
			AND (e.activo OR ($4 = 'LIQUIDACION' AND e.fecha_salida BETWEEN r.ini AND r.fin))
			AND ($4 <> 'ADELANTO' OR e.jornada = 'QUINCENAL' OR NOT EXISTS (
				SELECT 1 FROM finiquito fq
				WHERE fq.empresa_id = e.empresa_id AND fq.empleado_id = e.id
					AND fq.estado IN ('APROBADO', 'PAGADO')))
		ORDER BY e.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID, anio, mes, tipo)
	if err != nil {
		return nil, fmt.Errorf("nomina: empleados para corrida: %w", err)
	}
	defer rows.Close()
	items := make([]EmpleadoCorrida, 0, 32)
	for rows.Next() {
		var ec EmpleadoCorrida
		e := &ec.Empleado
		if err := rows.Scan(&e.ID, &e.Nombre, &e.TipoIdentificacion, &e.Identificacion, &e.Email,
			&e.Telefono, &e.IBAN, &e.DepartamentoID, &e.DepartamentoNombre, &e.Puesto,
			&e.FechaIngreso, &e.FechaSalida, &e.SalarioBase, &e.Jornada,
			&e.Hijos, &e.Conyuge, &e.Activo, &e.DeduccionesActivas, &ec.FraccionMes); err != nil {
			return nil, fmt.Errorf("nomina: scan empleado de corrida: %w", err)
		}
		items = append(items, ec)
	}
	return items, rows.Err()
}

// PagarCorrida marca la corrida PAGADA y, en la MISMA transacción, descuenta el saldo de
// las deducciones recurrentes aplicadas en las colillas (GREATEST(0, saldo − monto)).
func (r *pgRepository) PagarCorrida(ctx context.Context, empresaID, id, usuarioID string) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("nomina: tx pagar: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE corrida_nomina SET estado = 'PAGADA', pagado_por = $3::uuid, pagado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'APROBADA'`,
		empresaID, id, usuarioID)
	if err != nil {
		return 0, fmt.Errorf("nomina: pagar corrida: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return 0, nil // el servicio traduce a ErrCorridaNoPagable
	}
	// Descuento de saldos desde el detalle de las colillas (solo deducciones con tope).
	if _, err := tx.Exec(ctx, `
		UPDATE deduccion_empleado de
		SET saldo_restante = GREATEST(0, de.saldo_restante - d.monto)
		FROM (
			SELECT (elem->>'deduccion_id')::uuid AS ded_id, SUM((elem->>'monto')::numeric) AS monto
			FROM corrida_linea cl, jsonb_array_elements(cl.detalle) elem
			WHERE cl.empresa_id = $1::uuid AND cl.corrida_id = $2::uuid
				AND elem->>'tipo' = 'DEDUCCION' AND COALESCE(elem->>'deduccion_id', '') <> ''
			GROUP BY 1
		) d
		WHERE de.id = d.ded_id AND de.empresa_id = $1::uuid AND de.saldo_restante IS NOT NULL`,
		empresaID, id); err != nil {
		return 0, fmt.Errorf("nomina: descontar saldos: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("nomina: commit pagar: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ExisteAdelantoBorrador indica si el adelanto del mes sigue en BORRADOR (bloquea aprobar
// la liquidación: se pagaría el mes 1.5 veces).
func (r *pgRepository) ExisteAdelantoBorrador(ctx context.Context, empresaID string, anio, mes int) (bool, error) {
	var existe bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM corrida_nomina
			WHERE empresa_id = $1::uuid AND anio = $2 AND mes = $3 AND tipo = 'ADELANTO' AND estado = 'BORRADOR')`,
		empresaID, anio, mes).Scan(&existe)
	if err != nil {
		return false, fmt.Errorf("nomina: adelanto en borrador: %w", err)
	}
	return existe, nil
}

// AnularCorrida anula un BORRADOR o una APROBADA — salvo un ADELANTO aprobado que una
// liquidación APROBADA/PAGADA ya descontó (anularlo dejaría al empleado sin ese monto);
// la guarda va en el propio UPDATE (atómica, sin ventana de carrera).
func (r *pgRepository) AnularCorrida(ctx context.Context, empresaID, id string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE corrida_nomina AS c SET estado = 'ANULADA'
		WHERE c.empresa_id = $1::uuid AND c.id = $2::uuid AND c.estado IN ('BORRADOR', 'APROBADA')
			AND NOT (c.tipo = 'ADELANTO' AND c.estado = 'APROBADA' AND EXISTS (
				SELECT 1 FROM corrida_nomina h
				WHERE h.empresa_id = c.empresa_id AND h.anio = c.anio AND h.mes = c.mes
					AND h.tipo = 'LIQUIDACION' AND h.estado IN ('APROBADA', 'PAGADA')))`,
		empresaID, id)
	if err != nil {
		return 0, fmt.Errorf("nomina: anular corrida: %w", err)
	}
	return tag.RowsAffected(), nil
}
