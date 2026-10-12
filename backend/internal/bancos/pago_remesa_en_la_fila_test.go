package bancos

// Un débito que pagó VARIAS facturas de un proveedor tiene que decirlo en la fila (1-oct-2026).
//
// Cuando el corte se agrupa por proveedor, el movimiento no apunta a una factura sino a la remesa
// (mig 0089). Si la fila solo mirara `documento_cxp_id`, el pago agrupado quedaría enlazado en la
// base y MUDO en la pantalla — exactamente el defecto que se venía de arreglar para el pago suelto.
//
// Y hay una trampa propia: con cinco facturas adentro no hay UN número de factura que mostrar.
// Mostrar el de la primera haría creer que ese débito pagó esa factura y nada más.

import (
	"context"
	"testing"
)

const (
	provRemesa = "c0cc0000-c0cc-4ccc-8ccc-c0cc0000c0cc"
	loteRemesa = "10100000-1010-4010-8010-101000001010"
	remesaUna  = "ee000000-ee00-4e00-8e00-ee000000ee00"
)

func TestElMovimientoDiceQueRemesaPago(t *testing.T) {
	repo, _, _ := baseMiPartida(t)
	ctx := context.Background()

	// Un corte agrupado: una remesa de ALFA que cubre tres facturas.
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO proveedor (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'ALFA SOCIEDAD ANONIMA')`,
			[]any{provRemesa, empresaPrueba}},
		{`INSERT INTO lote_pago (id, empresa_id, numero, fecha_corte, modo_pago)
		  VALUES ($1::uuid, $2::uuid, 29, '2026-10-02'::date, 'PROVEEDOR')`,
			[]any{loteRemesa, empresaPrueba}},
		{`INSERT INTO remesa_pago (id, empresa_id, lote_id, proveedor_id, numero, huella, monto_crc)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 1, 'CXPR-1658A5302608', 186450.00)`,
			[]any{remesaUna, empresaPrueba, loteRemesa, provRemesa}},
	} {
		if _, err := repo.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("sembrar el corte agrupado: %v", err)
		}
	}
	for i, consec := range []string{"00100001010000000668", "00100001010000000669", "00100001010000000670"} {
		if _, err := repo.pool.Exec(ctx, `
			INSERT INTO documento_cxp
				(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
				 total, total_crc, lote_id, remesa_id)
			VALUES ($1::uuid, $2::uuid, $3, $3, '2026-09-01'::date, 'CONCILIADO', 'CRC',
			        62150.00, 62150.00, $4::uuid, $5::uuid)`,
			empresaPrueba, provRemesa, consec, loteRemesa, remesaUna); err != nil {
			t.Fatalf("sembrar factura %d: %v", i, err)
		}
	}

	// El débito: UNO solo, por el total de las tres.
	const descBanco = "DEBITO MASIVO SINPE ( - 1060746795 REM 0029-01 CXPR1658A5302608 Lote 888607 Linea 4)"
	var idPago string
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
			 estado_clasificacion, natural_key, incluido, remesa_id)
		VALUES ($1::uuid, $2::uuid, '2026-12-18'::date, 'DOC-rem', $3, 186450.00, 0,
		        'CRC', 186450.00, 186450.00, $4::uuid, $5::uuid, 'REVISADO', 'cl-rem', true, $6::uuid)
		RETURNING id::text`,
		empresaPrueba, cuentaBuena, descBanco, conceptoID, partidaMia, remesaUna).Scan(&idPago); err != nil {
		t.Fatalf("sembrar el pago agrupado: %v", err)
	}

	res, err := repo.ListarMovimientos(ctx, empresaPrueba, FiltrosMovimientos{PageSize: 200})
	if err != nil {
		t.Fatalf("listar movimientos: %v", err)
	}
	var fila *MovimientoRow
	for i := range res.Items {
		if res.Items[i].ID == idPago {
			fila = &res.Items[i]
			break
		}
	}
	if fila == nil {
		t.Fatal("control: el pago agrupado no está en la lista")
	}
	if fila.PagoCxP == nil {
		t.Fatal("el movimiento está enlazado a una remesa y la fila no lo dice: el pago agrupado " +
			"quedaría tan mudo como estaba el suelto")
	}
	casos := []struct{ campo, got, quiere string }{
		{"proveedor", fila.PagoCxP.Proveedor, "ALFA SOCIEDAD ANONIMA"},
		{"huella", fila.PagoCxP.Huella, "CXPR-1658A5302608"},
		// La referencia del GRUPO, no el número de una de las tres facturas.
		{"referencia", fila.PagoCxP.Consecutivo, "REM 0029-01"},
	}
	for _, c := range casos {
		if c.got != c.quiere {
			t.Errorf("%s = %q, quiere %q", c.campo, c.got, c.quiere)
		}
	}
	if fila.PagoCxP.Facturas != 3 {
		t.Errorf("facturas = %d, quiere 3: es lo que distingue un pago agrupado de uno suelto",
			fila.PagoCxP.Facturas)
	}
}
