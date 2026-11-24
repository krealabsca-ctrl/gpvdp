package cxp

// El neto de la cartera, cuando una factura tiene VARIAS rebajas del mismo tipo (9-nov-2026).
//
// El tablero calculaba el neto con tres subconsultas correlacionadas, una por factura y por tipo
// de rebaja. Eso era correcto pero carísimo: la base las resolvía fila por fila, y el tablero
// rearma la cartera siete veces por carga. Medido contra una copia de 200 000 documentos, una
// sola pasada tocaba 591 798 bloques.
//
// Al pasarlas a cruces con las rebajas YA AGRUPADAS por factura, el costo cayó a 8 051 bloques
// —73 veces menos— pero apareció un riesgo que antes no existía: un cruce puede MULTIPLICAR
// filas. Si las subconsultas de JoinsNetoSQL perdieran su GROUP BY, una factura con dos anticipos,
// dos notas de crédito y dos parciales se contaría 2×2×2 = OCHO veces, y el monto de la cartera
// se dispararía sin que ninguna pantalla se queje.
//
// Esa es exactamente la factura que siembran estas pruebas. Van contra Postgres de verdad porque
// lo que se protege ES el SQL: en Go esto compila igual de bien roto que bueno.

import (
	"context"
	"testing"
)

// sembrarRebajasRepetidas le pone a una factura DOS de cada rebaja, cada una con su propio origen
// (dos anticipos, dos notas, dos movimientos) porque los índices únicos vivos no dejan repetir el
// mismo par. Devuelve lo que las seis rebajas suman en total.
func sembrarRebajasRepetidas(t *testing.T, r *pgRepository, facturaID string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		// Dos anticipos de ₡100.000: juntos rebajan 200.000, no 100.000 ni 400.000.
		{`INSERT INTO anticipo_aplicacion (empresa_id, anticipo_id, factura_id, monto_crc)
		  VALUES ($1::uuid, gen_random_uuid(), $2::uuid, 100000),
		         ($1::uuid, gen_random_uuid(), $2::uuid, 100000)`, []any{empPC, facturaID}},
		// Dos notas de crédito de ₡50.000: juntas rebajan 100.000.
		{`INSERT INTO cxp_nota_credito_aplicacion (empresa_id, nota_id, factura_id, monto)
		  VALUES ($1::uuid, gen_random_uuid(), $2::uuid, 50000),
		         ($1::uuid, gen_random_uuid(), $2::uuid, 50000)`, []any{empPC, facturaID}},
		// Dos pagos parciales de ₡30.000: juntos rebajan 60.000. El motivo lleva al menos 20
		// caracteres porque la tabla lo exige: un parcial sin explicación no se puede auditar.
		{`INSERT INTO pago_parcial (empresa_id, documento_id, movimiento_id, monto, motivo)
		  VALUES ($1::uuid, $2::uuid, gen_random_uuid(), 30000, 'Abono parcial de prueba automatizada'),
		         ($1::uuid, $2::uuid, gen_random_uuid(), 30000, 'Segundo abono de prueba automatizada')`,
			[]any{empPC, facturaID}},
	} {
		if _, err := r.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("sembrar rebajas: %v", err)
		}
	}
}

// TestVariasRebajasDelMismoTipoNoMultiplicanLaFactura es la prueba que falsifica el cambio: si
// alguien quita el GROUP BY de JoinsNetoSQL, acá la cartera pasa de una factura a ocho.
func TestVariasRebajasDelMismoTipoNoMultiplicanLaFactura(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()

	// Una sola factura abierta de ₡1.000.000, sin retención.
	id := sembrarFacturaPC(t, repo, "900", EstAprobado, 1_000_000, -1)
	sembrarRebajasRepetidas(t, repo, id)

	var d DashboardCxP
	if err := repo.carteraCxP(ctx, empPC, nil, &d); err != nil {
		t.Fatalf("carteraCxP: %v", err)
	}

	// UNA factura. Con los cruces sin agrupar serían ocho, y nadie lo notaría mirando el tablero:
	// «8 facturas por pagar» se lee perfectamente normal.
	if d.Cartera.Abierta.Cantidad != 1 {
		t.Errorf("cantidad = %d, se esperaba 1: los cruces del neto están multiplicando filas",
			d.Cartera.Abierta.Cantidad)
	}
	// 1.000.000 − 200.000 (dos anticipos) − 100.000 (dos notas) − 60.000 (dos parciales).
	if d.Cartera.Abierta.Monto != "640000.00" {
		t.Errorf("neto = %q, se esperaba 640000.00", d.Cartera.Abierta.Monto)
	}
	// El bruto NO lleva rebajas: si también se hubiera multiplicado, serían ₡8.000.000.
	if d.Cartera.Bruto != "1000000.00" {
		t.Errorf("bruto = %q, se esperaba 1000000.00", d.Cartera.Bruto)
	}
	// La columna de anticipos sale del mismo cruce y antes era una CUARTA subconsulta correlacionada.
	if d.Cartera.Anticipos != "200000.00" {
		t.Errorf("anticipos = %q, se esperaba 200000.00", d.Cartera.Anticipos)
	}
}

// TestLasRebajasReversadasNoBajanElNeto protege el `WHERE activo` de las tres subconsultas.
//
// Es la otra mitad del mismo riesgo: una subconsulta que pierda el filtro seguiría dando UNA fila
// por factura —así que la prueba de arriba pasaría— y restaría de más en silencio. Reversar una
// aplicación tiene que devolver el neto a donde estaba.
func TestLasRebajasReversadasNoBajanElNeto(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()

	id := sembrarFacturaPC(t, repo, "901", EstAprobado, 1_000_000, -1)
	sembrarRebajasRepetidas(t, repo, id)

	// Se reversa UNA de cada tipo: queda la mitad de cada rebaja.
	for _, q := range []string{
		`UPDATE anticipo_aplicacion SET activo = false WHERE id = (
		   SELECT id FROM anticipo_aplicacion WHERE factura_id = $1::uuid LIMIT 1)`,
		`UPDATE cxp_nota_credito_aplicacion SET activo = false WHERE id = (
		   SELECT id FROM cxp_nota_credito_aplicacion WHERE factura_id = $1::uuid LIMIT 1)`,
		`UPDATE pago_parcial SET activo = false WHERE id = (
		   SELECT id FROM pago_parcial WHERE documento_id = $1::uuid LIMIT 1)`,
	} {
		if _, err := repo.pool.Exec(ctx, q, id); err != nil {
			t.Fatalf("reversar: %v", err)
		}
	}

	var d DashboardCxP
	if err := repo.carteraCxP(ctx, empPC, nil, &d); err != nil {
		t.Fatalf("carteraCxP: %v", err)
	}

	if d.Cartera.Abierta.Cantidad != 1 {
		t.Errorf("cantidad = %d, se esperaba 1", d.Cartera.Abierta.Cantidad)
	}
	// 1.000.000 − 100.000 − 50.000 − 30.000: la mitad de cada rebaja sigue viva.
	if d.Cartera.Abierta.Monto != "820000.00" {
		t.Errorf("neto = %q, se esperaba 820000.00: una rebaja reversada sigue restando",
			d.Cartera.Abierta.Monto)
	}
	if d.Cartera.Anticipos != "100000.00" {
		t.Errorf("anticipos = %q, se esperaba 100000.00", d.Cartera.Anticipos)
	}
}

// TestUnaFacturaSinRebajasMantieneSuTotal cubre el caso que es el 99 % de la cartera.
//
// Los tres cruces son LEFT JOIN y las tres subconsultas no devuelven nada para esa factura; si
// alguno se escribiera como cruce interno, la factura DESAPARECERÍA del tablero. Un neto de menos
// no se nota: la cartera simplemente marca menos plata y nadie va a buscar la que falta.
func TestUnaFacturaSinRebajasMantieneSuTotal(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()

	sembrarFacturaPC(t, repo, "902", EstAprobado, 750_000, -1)

	var d DashboardCxP
	if err := repo.carteraCxP(ctx, empPC, nil, &d); err != nil {
		t.Fatalf("carteraCxP: %v", err)
	}
	if d.Cartera.Abierta.Cantidad != 1 {
		t.Fatalf("cantidad = %d, se esperaba 1: la factura sin rebajas se cayó del tablero",
			d.Cartera.Abierta.Cantidad)
	}
	if d.Cartera.Abierta.Monto != "750000.00" {
		t.Errorf("neto = %q, se esperaba 750000.00", d.Cartera.Abierta.Monto)
	}
}
