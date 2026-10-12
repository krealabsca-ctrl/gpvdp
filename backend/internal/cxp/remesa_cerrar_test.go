package cxp

// CerrarRemesa contra PostgreSQL de verdad.
//
// Esta prueba existe porque faltaba. Las del servicio corren contra un fake, así que la consulta
// nunca se ejecutaba: `MIN(uuid)` —que en PostgreSQL no existe— pasó todas las pruebas y reventó
// con un 500 la primera vez que se conció un pago agrupado contra la base real. Un repositorio
// probado solo detrás de un fake no está probado.

import (
	"context"
	"testing"
)

const (
	provCerrar  = "77777777-7777-4777-8777-777777777777"
	loteCerrar  = "78787878-7878-4878-8878-787878787878"
	remCerrar   = "79797979-7979-4979-8979-797979797979"
	conceptoUno = "7a7a7a7a-7a7a-4a7a-8a7a-7a7a7a7a7a7a"
	clasifUno   = "7b7b7b7b-7b7b-4b7b-8b7b-7b7b7b7b7b7b"
	clasifDos   = "7c7c7c7c-7c7c-4c7c-8c7c-7c7c7c7c7c7c"
)

// remesaSembrada arma una remesa con N facturas PROGRAMADAS, cada una con la clasificación dada
// ("" = sin clasificar). Devuelve el repositorio listo.
func remesaSembrada(t *testing.T, clasifs []string) *pgRepository {
	t.Helper()
	repo := baseDeLotes(t)
	ctx := context.Background()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO proveedor (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'ALFA')`,
			[]any{provCerrar, empresaLote}},
		{`INSERT INTO concepto (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Gastos')`,
			[]any{conceptoUno, empresaLote}},
		{`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'Agua')`,
			[]any{clasifUno, empresaLote, conceptoUno}},
		{`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'Luz')`,
			[]any{clasifDos, empresaLote, conceptoUno}},
		{`INSERT INTO lote_pago (id, empresa_id, numero, fecha_corte, modo_pago)
		  VALUES ($1::uuid, $2::uuid, 1, '2026-10-02'::date, 'PROVEEDOR')`,
			[]any{loteCerrar, empresaLote}},
		{`INSERT INTO remesa_pago (id, empresa_id, lote_id, proveedor_id, numero, huella, monto_crc)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 1, 'CXPR-AAAAAAAAAAAA', 300000)`,
			[]any{remCerrar, empresaLote, loteCerrar, provCerrar}},
	} {
		if _, err := repo.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("sembrar: %v", err)
		}
	}
	for i, cl := range clasifs {
		if _, err := repo.pool.Exec(ctx, `
			INSERT INTO documento_cxp
				(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
				 total, total_crc, lote_id, remesa_id, concepto_id, clasificacion_id)
			VALUES ($1::uuid, $2::uuid, $3, $3, '2026-09-01'::date, 'PROGRAMADO', 'CRC',
			        100000, 100000, $4::uuid, $5::uuid,
			        CASE WHEN $6 = '' THEN NULL ELSE $7::uuid END,
			        NULLIF($6, '')::uuid)`,
			empresaLote, provCerrar, "F-"+string(rune('A'+i)), loteCerrar, remCerrar, cl, conceptoUno); err != nil {
			t.Fatalf("sembrar factura %d: %v", i, err)
		}
	}
	return repo
}

func TestCerrarRemesaLlevaTodasSusFacturasAConciliado(t *testing.T) {
	repo := remesaSembrada(t, []string{clasifUno, clasifUno, clasifUno})
	ctx := context.Background()

	cerradas, conceptoID, clasifID, err := repo.CerrarRemesa(ctx, empresaLote, remCerrar)
	if err != nil {
		t.Fatalf("cerrar remesa: %v", err)
	}
	if cerradas != 3 {
		t.Errorf("cerró %d facturas, la remesa cubría 3", cerradas)
	}
	// Las tres pasaron por PAGADO antes de CONCILIADO: saltarse el estado dejaría sin registrar
	// que la plata salió.
	var conciliadas int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM documento_cxp WHERE remesa_id = $1::uuid AND estado = 'CONCILIADO'`,
		remCerrar).Scan(&conciliadas); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if conciliadas != 3 {
		t.Errorf("quedaron %d conciliadas de 3", conciliadas)
	}
	// Comparten gasto: el movimiento bancario se puede clasificar con él.
	if clasifID != clasifUno || conceptoID != conceptoUno {
		t.Errorf("gasto común = (%q, %q), quiere (%q, %q)", conceptoID, clasifID, conceptoUno, clasifUno)
	}
}

func TestUnaRemesaConGastosMezcladosNoDevuelveGastoComun(t *testing.T) {
	repo := remesaSembrada(t, []string{clasifUno, clasifDos})
	ctx := context.Background()

	cerradas, conceptoID, clasifID, err := repo.CerrarRemesa(ctx, empresaLote, remCerrar)
	if err != nil {
		t.Fatalf("cerrar remesa: %v", err)
	}
	if cerradas != 2 {
		t.Errorf("cerró %d facturas, quiere 2", cerradas)
	}
	// Dos gastos distintos: no hay UNO que describa el pago, y el movimiento queda sin clasificar.
	// Elegir cualquiera de los dos atribuiría plata al gasto equivocado.
	if conceptoID != "" || clasifID != "" {
		t.Errorf("devolvió el gasto (%q, %q) para un pago que mezcla dos: tiene que venir vacío",
			conceptoID, clasifID)
	}
}

func TestUnaRemesaConUnaFacturaSinClasificarNoDevuelveGastoComun(t *testing.T) {
	// Dos con el mismo gasto y una sin clasificar. «Todas iguales» no se cumple: una no tiene
	// gasto, así que el del pago no se conoce.
	repo := remesaSembrada(t, []string{clasifUno, clasifUno, ""})
	ctx := context.Background()

	_, conceptoID, clasifID, err := repo.CerrarRemesa(ctx, empresaLote, remCerrar)
	if err != nil {
		t.Fatalf("cerrar remesa: %v", err)
	}
	if conceptoID != "" || clasifID != "" {
		t.Errorf("devolvió (%q, %q) con una factura sin clasificar", conceptoID, clasifID)
	}
}
