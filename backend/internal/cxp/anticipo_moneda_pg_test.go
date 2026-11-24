package cxp

// El anticipo en dólares SÍ se resta del neto (20-oct-2026, hallazgo 3.8).
//
// EL DEFECTO. `netoAPagarSQL` llevaba un `AND d.moneda = 'CRC'` dentro del subselect de los
// anticipos. Eso evitaba el error peor —restarle colones a una factura en dólares— pero dejaba
// otro, autodocumentado en el propio código: una factura en dólares con anticipo aplicado **se
// pagaba completa**, como si lo ya entregado no existiera. Al proveedor se le pagaba dos veces la
// parte anticipada.
//
// Inerte mientras las 4.542 facturas estuvieran en colones, pero la migración 0095 ya trajo macros
// por moneda: el sistema espera facturas en dólares.
//
// LA REGLA NUEVA ES «LA MISMA MONEDA», NO «SOLO COLONES» (decisión del Director Financiero). No se
// convierte entre monedas: con qué tipo de cambio es una decisión contable que no está tomada, y
// es la misma regla que ya aplica la conciliación por huella.

import (
	"context"
	"testing"
)

// facturaMoneda siembra un documento en la moneda indicada y devuelve su id.
//
// `total_crc` se llena a propósito con un valor distinto de `total` cuando es USD: así, si alguna
// consulta mezclara las dos columnas, el número saldría mal en vez de parecer correcto por
// casualidad.
func facturaMoneda(t *testing.T, repo *pgRepository, nombre, tipo, moneda, total, totalCRC string) string {
	t.Helper()
	var id string
	err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, tipo,
			 total, total_crc, retencion, moneda, tc_aplicado)
		VALUES ($1::uuid, $2::uuid, $3, $3, '2026-08-01'::date, 'PAGADO', $4,
		        $5, $6, 0, $7, 500)
		RETURNING id::text`,
		empresaLote, provLote, nombre, tipo, total, totalCRC, moneda).Scan(&id)
	if err != nil {
		t.Fatalf("sembrar factura %s: %v", nombre, err)
	}
	return id
}

func TestElAnticipoEnDolaresSeRestaDelNeto(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	// Factura de USD 1.000 (₡500.000 al TC 500) con un anticipo de USD 300 ya aplicado.
	factura := facturaMoneda(t, repo, "F-USD", "CXP", "USD", "1000", "500000")
	anticipo := facturaMoneda(t, repo, "A-USD", TipoAnticipo, "USD", "300", "150000")
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO anticipo_aplicacion (empresa_id, anticipo_id, factura_id, monto_crc, activo)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 300, true)`,
		empresaLote, anticipo, factura); err != nil {
		t.Fatalf("aplicar anticipo: %v", err)
	}

	neto, err := repo.NetoAPagar(ctx, empresaLote, factura)
	if err != nil {
		t.Fatalf("neto a pagar: %v", err)
	}
	if neto == "1000" || neto == "1000.00" {
		t.Fatal("el anticipo NO se restó: se le pagaría al proveedor la factura completa, pagando dos veces los USD 300 que ya recibió")
	}
	if neto != "700" && neto != "700.00" {
		t.Errorf("neto = %q, esperaba 700 (1000 − 300)", neto)
	}
}

// Y el neto de una factura en colones no cambia: lo que ya funcionaba sigue igual.
func TestElNetoEnColonesNoCambio(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	factura := facturaMoneda(t, repo, "F-CRC", "CXP", "CRC", "50000", "50000")
	anticipo := facturaMoneda(t, repo, "A-CRC", TipoAnticipo, "CRC", "20000", "20000")
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO anticipo_aplicacion (empresa_id, anticipo_id, factura_id, monto_crc, activo)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 20000, true)`,
		empresaLote, anticipo, factura); err != nil {
		t.Fatalf("aplicar anticipo: %v", err)
	}

	neto, err := repo.NetoAPagar(ctx, empresaLote, factura)
	if err != nil {
		t.Fatalf("neto a pagar: %v", err)
	}
	if neto != "30000" && neto != "30000.00" {
		t.Errorf("neto = %q, esperaba 30000 (50000 − 20000)", neto)
	}
}

// Un anticipo REVERSADO no se resta: `activo` es lo que distingue la aplicación viva de la
// deshecha, y contarla igual le descontaría al proveedor algo que se le devolvió.
func TestElAnticipoReversadoNoSeResta(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	factura := facturaMoneda(t, repo, "F-REV", "CXP", "CRC", "50000", "50000")
	anticipo := facturaMoneda(t, repo, "A-REV", TipoAnticipo, "CRC", "20000", "20000")
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO anticipo_aplicacion (empresa_id, anticipo_id, factura_id, monto_crc, activo)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 20000, false)`,
		empresaLote, anticipo, factura); err != nil {
		t.Fatalf("aplicar anticipo: %v", err)
	}

	neto, err := repo.NetoAPagar(ctx, empresaLote, factura)
	if err != nil {
		t.Fatalf("neto a pagar: %v", err)
	}
	if neto != "50000" && neto != "50000.00" {
		t.Errorf("neto = %q, esperaba 50000: una aplicación reversada no debe descontar nada", neto)
	}
}
