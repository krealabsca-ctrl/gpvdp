package bancos

// El SQL del sello, contra Postgres de verdad.
//
// Las pruebas del cruce prueban el CRITERIO con un doble; estas prueban la consulta, que es lo que
// decide qué movimientos se le ofrecen al cruce y qué se escribe de verdad. Un repositorio probado
// solo detrás de un fake no está probado: `MIN(uuid)` pasó catorce suites y reventó con un 500 la
// primera vez que corrió contra la base.

import (
	"context"
	"testing"
)

const (
	peCuenta = "d1000000-0000-4000-8000-000000000001"
	peBanco  = "d1000000-0000-4000-8000-000000000002"
)

// baseSello siembra cuatro débitos: uno libre, uno ya enlazado a una factura, uno excluido de la
// carga y uno que es un CRÉDITO. Solo el primero puede sellarse.
func baseSello(t *testing.T) *pgRepository {
	t.Helper()
	repo, _, _ := baseMiPartida(t)
	ctx := context.Background()

	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Banco del Sello')`,
			[]any{peBanco, empresaPrueba}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'Cuenta del Sello')`,
			[]any{peCuenta, empresaPrueba, peBanco}},
	} {
		if _, err := repo.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("sembrar catálogo: %v", err)
		}
	}

	movs := []struct{ nk, fecha, monto, desc string }{
		{"pe-libre", "2026-03-10", "125000.00", "TEF 998877 PROVEEDOR X"},
		{"pe-enlazado", "2026-03-11", "50000.00", "PAGO YA ENLAZADO"},
		{"pe-excluido", "2026-03-12", "70000.00", "PAGO DE CARGA REVERTIDA"},
	}
	for _, m := range movs {
		incluido := m.nk != "pe-excluido"
		if _, err := repo.pool.Exec(ctx, `
			INSERT INTO movimiento_bancario
				(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
				 moneda_original, monto_original, monto_crc, estado_clasificacion, natural_key, incluido)
			VALUES ($1::uuid, $2::uuid, $3::date, $4, $5, $6::numeric, 0,
			        'CRC', $6::numeric, -$6::numeric, 'NO_IDENTIFICADO', $4, $7)`,
			empresaPrueba, peCuenta, m.fecha, m.nk, m.desc, m.monto, incluido); err != nil {
			t.Fatalf("sembrar %s: %v", m.nk, err)
		}
	}
	// Un crédito: no es una salida de plata y nunca puede explicarse con un pago aplicado.
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, estado_clasificacion, natural_key, incluido)
		VALUES ($1::uuid, $2::uuid, '2026-03-13'::date, 'pe-credito', 'DEPOSITO', 0, 90000,
		        'CRC', 90000, 90000, 'NO_IDENTIFICADO', 'pe-credito', true)`,
		empresaPrueba, peCuenta); err != nil {
		t.Fatalf("sembrar crédito: %v", err)
	}
	return repo
}

// idDe devuelve el id del movimiento con esa natural_key.
func idDe(t *testing.T, repo *pgRepository, nk string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(),
		`SELECT id::text FROM movimiento_bancario WHERE natural_key = $1`, nk).Scan(&id); err != nil {
		t.Fatalf("buscar %s: %v", nk, err)
	}
	return id
}

func TestSoloSeOfrecenLosDebitosQueNadieExplicaTodavia(t *testing.T) {
	repo := baseSello(t)
	ctx := context.Background()

	// Al «enlazado» se le pone una factura: ese movimiento ya tiene dueño en el ERP.
	var docID string
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO proveedor (empresa_id, nombre) VALUES ($1::uuid, 'PROV') RETURNING id`,
		empresaPrueba).Scan(&docID); err != nil {
		t.Fatalf("sembrar proveedor: %v", err)
	}
	var facturaID string
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO documento_cxp (empresa_id, proveedor_id, clave, consecutivo, fecha_emision,
		                           estado, moneda, total, total_crc)
		VALUES ($1::uuid, $2::uuid, 'F1', 'F1', '2026-03-01'::date, 'PAGADO', 'CRC', 50000, 50000)
		RETURNING id`, empresaPrueba, docID).Scan(&facturaID); err != nil {
		t.Fatalf("sembrar factura: %v", err)
	}
	if _, err := repo.pool.Exec(ctx,
		`UPDATE movimiento_bancario SET documento_cxp_id = $1::uuid WHERE natural_key = 'pe-enlazado'`,
		facturaID); err != nil {
		t.Fatalf("enlazar: %v", err)
	}

	cands, err := repo.DebitosParaCruzar(ctx, empresaPrueba)
	if err != nil {
		t.Fatalf("débitos para cruzar: %v", err)
	}
	porDoc := map[string]bool{}
	for _, c := range cands {
		porDoc[c.Documento] = true
	}
	if !porDoc["pe-libre"] {
		t.Error("el débito libre NO se ofreció: es el único que se puede explicar desde afuera")
	}
	for _, no := range []struct{ nk, porque string }{
		{"pe-enlazado", "ya tiene factura en el ERP"},
		{"pe-excluido", "está excluido de la carga (incluido = false)"},
		{"pe-credito", "es un crédito, no una salida de plata"},
	} {
		if porDoc[no.nk] {
			t.Errorf("se ofreció %s y %s", no.nk, no.porque)
		}
	}
}

func TestSellarEscribeYElMovimientoDejaDeOfrecerse(t *testing.T) {
	repo := baseSello(t)
	ctx := context.Background()
	libre := idDe(t, repo, "pe-libre")

	carga, n, err := repo.SellarPagosExternos(ctx, empresaPrueba, usuarioPrueba, []SelloPagoExterno{{
		MovimientoID: libre, Referencia: "998877", Documento: "FAC-100",
		Tercero: "PROVEEDOR X", FechaAplicado: "2026-03-10", MontoReportado: "125000.00",
		Nota: "del sistema anterior",
	}})
	if err != nil {
		t.Fatalf("sellar: %v", err)
	}
	if n != 1 {
		t.Fatalf("sellados = %d, se esperaba 1", n)
	}
	if carga == "" {
		t.Error("no devolvió el id de la carga: sin él no se puede revertir")
	}

	// Ya no se ofrece: un movimiento no se sella dos veces.
	cands, _ := repo.DebitosParaCruzar(ctx, empresaPrueba)
	for _, c := range cands {
		if c.ID == libre {
			t.Error("el movimiento sellado se sigue ofreciendo para cruzar")
		}
	}

	// Y el sello se lee para pintarlo en la lista.
	sellos, err := repo.SellosDeMovimientos(ctx, empresaPrueba, []string{libre})
	if err != nil {
		t.Fatalf("leer sellos: %v", err)
	}
	s, ok := sellos[libre]
	if !ok {
		t.Fatal("el sello no se lee de vuelta")
	}
	if s.Referencia != "998877" || s.Tercero != "PROVEEDOR X" || s.FechaAplicado != "2026-03-10" {
		t.Errorf("sello = %+v", s)
	}
}

// Entre previsualizar y confirmar pueden pasar minutos. Si en ese rato alguien enlaza el
// movimiento a una factura, el sello NO se escribe: el INSERT lo vuelve a verificar.
func TestNoSeSellaUnMovimientoQueMientrasTantoConsiguioDuenno(t *testing.T) {
	repo := baseSello(t)
	ctx := context.Background()
	libre := idDe(t, repo, "pe-libre")

	var provID, facturaID string
	_ = repo.pool.QueryRow(ctx, `INSERT INTO proveedor (empresa_id, nombre) VALUES ($1::uuid,'P') RETURNING id`,
		empresaPrueba).Scan(&provID)
	_ = repo.pool.QueryRow(ctx, `
		INSERT INTO documento_cxp (empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda, total, total_crc)
		VALUES ($1::uuid,$2::uuid,'F9','F9','2026-03-01'::date,'PAGADO','CRC',1,1) RETURNING id`,
		empresaPrueba, provID).Scan(&facturaID)
	if _, err := repo.pool.Exec(ctx,
		`UPDATE movimiento_bancario SET documento_cxp_id=$1::uuid WHERE id=$2::uuid`, facturaID, libre); err != nil {
		t.Fatalf("enlazar: %v", err)
	}

	_, n, err := repo.SellarPagosExternos(ctx, empresaPrueba, usuarioPrueba,
		[]SelloPagoExterno{{MovimientoID: libre, Referencia: "998877"}})
	if err != nil {
		t.Fatalf("sellar: %v", err)
	}
	if n != 0 {
		t.Errorf("selló %d movimiento(s) que ya tenían factura: el sello diría «resuelto afuera» sobre un pago que el ERP ya explica", n)
	}
}

func TestLaReversaQuitaLaCargaEnteraYDevuelveElMovimiento(t *testing.T) {
	repo := baseSello(t)
	ctx := context.Background()
	libre := idDe(t, repo, "pe-libre")

	carga, _, err := repo.SellarPagosExternos(ctx, empresaPrueba, usuarioPrueba,
		[]SelloPagoExterno{{MovimientoID: libre, Referencia: "998877", MontoReportado: "125000.00"}})
	if err != nil {
		t.Fatalf("sellar: %v", err)
	}

	n, err := repo.RevertirCargaDeSellos(ctx, empresaPrueba, carga)
	if err != nil {
		t.Fatalf("revertir: %v", err)
	}
	if n != 1 {
		t.Errorf("quitó %d, se esperaba 1", n)
	}
	// El movimiento vuelve a estar disponible: la reversa tiene que devolverlo del todo.
	cands, _ := repo.DebitosParaCruzar(ctx, empresaPrueba)
	volvio := false
	for _, c := range cands {
		if c.ID == libre {
			volvio = true
		}
	}
	if !volvio {
		t.Error("después de revertir, el movimiento no volvió a ofrecerse")
	}
}

func TestElResumenSeparaLoSelladoDeLoPendiente(t *testing.T) {
	repo := baseSello(t)
	ctx := context.Background()

	antes, err := repo.ResumenSellos(ctx, empresaPrueba)
	if err != nil {
		t.Fatalf("resumen: %v", err)
	}
	if antes.Sellados != 0 {
		t.Errorf("sellados antes = %d, se esperaba 0", antes.Sellados)
	}
	pendientesAntes := antes.Pendientes

	if _, _, err := repo.SellarPagosExternos(ctx, empresaPrueba, usuarioPrueba,
		[]SelloPagoExterno{{MovimientoID: idDe(t, repo, "pe-libre"), Referencia: "998877"}}); err != nil {
		t.Fatalf("sellar: %v", err)
	}

	despues, err := repo.ResumenSellos(ctx, empresaPrueba)
	if err != nil {
		t.Fatalf("resumen: %v", err)
	}
	if despues.Sellados != 1 {
		t.Errorf("sellados = %d, se esperaba 1", despues.Sellados)
	}
	// LO QUE IMPORTA: el pendiente baja en uno. Es el número que el Director quiere ver bajar.
	if despues.Pendientes != pendientesAntes-1 {
		t.Errorf("pendientes pasó de %d a %d, se esperaba %d", pendientesAntes, despues.Pendientes, pendientesAntes-1)
	}
	if despues.Cargas != 1 {
		t.Errorf("cargas = %d, se esperaba 1", despues.Cargas)
	}
}

// El recorte por empresa llega también a lo nuevo.
func TestUnaEmpresaNoSellaElMovimientoDeOtra(t *testing.T) {
	repo := baseSello(t)
	ctx := context.Background()

	_, n, err := repo.SellarPagosExternos(ctx, otraEmpresa, usuarioPrueba,
		[]SelloPagoExterno{{MovimientoID: idDe(t, repo, "pe-libre"), Referencia: "998877"}})
	if err != nil {
		t.Fatalf("sellar: %v", err)
	}
	if n != 0 {
		t.Errorf("otra empresa selló %d movimiento(s) ajeno(s)", n)
	}
}
