package bancos

// La huella CxP→Bancos muestra también EL TIPO DE GASTO (13-oct-2026).
//
// EL PEDIDO del Director Financiero: «estaba revisando la huella que va desde CxP hacia Bancos
// para identificar a qué corresponde la partida de la macro, y creo que sería bueno que en esa
// huella también se visualice el tipo de gasto si ya está cargado; en el caso de que no, sin
// problema».
//
// Sin esto la huella dice a QUIÉN se le pagó y hay que abrir la factura para saber de QUÉ era —
// que es justo lo que uno necesita para clasificar el movimiento.
//
// LA REGLA DEL PAGO AGRUPADO es la que no se puede probar con un doble. Un pago por proveedor
// cubre varias facturas; si mezclan gastos distintos no hay UNO que describa el pago, y mostrar
// cualquiera de ellos atribuiría la plata al rubro equivocado. Es la misma regla que ya usa el
// cierre de remesa en CxP, y acá se verifica contra el SQL de verdad: la trampa está en que
// `count(DISTINCT x)` ignora los NULL, así que una factura clasificada junto a otra sin clasificar
// se vería como «todas iguales».

import (
	"context"
	"testing"
)

const (
	provHue   = "e1e1e1e1-f1f1-4a1a-b1b1-c1c1c1c1c1c1"
	concHue   = "e2e2e2e2-f2f2-4a2a-b2b2-c2c2c2c2c2c2"
	clasifHue = "e3e3e3e3-f3f3-4a3a-b3b3-c3c3c3c3c3c3"
	clasOtra  = "e4e4e4e4-f4f4-4a4a-b4b4-c4c4c4c4c4c4"
	remHue    = "e5e5e5e5-f5f5-4a5a-b5b5-c5c5c5c5c5c5"
	loteHue   = "e6e6e6e6-f6f6-4a6a-b6b6-c6c6c6c6c6c6"
)

// baseDeHuella extiende el escenario de exclusión con lo que hace falta para enlazar pagos.
func baseDeHuella(t *testing.T) *pgRepository {
	t.Helper()
	repo := baseDeExclusion(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := repo.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	exec(`INSERT INTO proveedor (id, empresa_id, nombre) VALUES ($1::uuid,$2::uuid,'VOLCANO 506 S.A')`,
		provHue, empExc)
	exec(`INSERT INTO concepto (id, empresa_id, nombre) VALUES ($1::uuid,$2::uuid,'Gastos')`, concHue, empExc)
	exec(`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
	      VALUES ($1::uuid,$2::uuid,$3::uuid,'Suministros de oficina')`, clasifHue, empExc, concHue)
	exec(`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
	      VALUES ($1::uuid,$2::uuid,$3::uuid,'Combustible')`, clasOtra, empExc, concHue)
	return repo
}

// facturaCxP crea una factura, opcionalmente clasificada y opcionalmente dentro de una remesa.
func facturaCxP(t *testing.T, r *pgRepository, clave, clasifID, remesaID string) string {
	t.Helper()
	var id string
	err := r.pool.QueryRow(context.Background(), `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
			 total, total_crc, concepto_id, clasificacion_id, remesa_id, huella)
		VALUES ($1::uuid,$2::uuid,$3,$3,'2026-10-09','PAGADO','CRC',41930.37,41930.37,
		        CASE WHEN $4 <> '' THEN $5::uuid END, NULLIF($4,'')::uuid, NULLIF($6,'')::uuid, $7)
		RETURNING id::text`,
		empExc, provHue, clave, clasifID, concHue, remesaID, "CXP-"+clave[len(clave)-8:]).Scan(&id)
	if err != nil {
		t.Fatalf("sembrar factura %s: %v", clave, err)
	}
	return id
}

// huellaDe busca el movimiento en la lista y devuelve su enlace a CxP.
func huellaDe(t *testing.T, r *pgRepository, movID string) *PagoCxPDeMovimiento {
	t.Helper()
	lista, err := r.ListarMovimientos(context.Background(), empExc, FiltrosMovimientos{Page: 1, PageSize: 200})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	for _, it := range lista.Items {
		if it.ID == movID {
			return it.PagoCxP
		}
	}
	t.Fatalf("el movimiento %s no apareció en la lista", movID)
	return nil
}

// UNA FACTURA CLASIFICADA: la huella dice de qué era el pago.
func TestLaHuellaMuestraElGastoDeLaFactura(t *testing.T) {
	repo := baseDeHuella(t)
	docID := facturaCxP(t, repo, "00100001010000023998", clasifHue, "")
	mov := sembrarMovDebito(t, repo, "2026-10-09", "520385390", "DEBITO MASIVO SINPE CXPB79BEE30BAB0", 41930.37)
	if _, err := repo.pool.Exec(context.Background(),
		`UPDATE movimiento_bancario SET documento_cxp_id = $1::uuid WHERE id = $2::uuid`, docID, mov); err != nil {
		t.Fatalf("enlazar: %v", err)
	}

	p := huellaDe(t, repo, mov)
	if p == nil {
		t.Fatal("el movimiento perdió su enlace a CxP")
	}
	if p.Clasificacion != "Suministros de oficina" {
		t.Errorf("clasificación = %q, quería «Suministros de oficina»", p.Clasificacion)
	}
	if p.Concepto != "Gastos" {
		t.Errorf("concepto = %q, quería «Gastos»", p.Concepto)
	}
}

// UNA FACTURA SIN CLASIFICAR: viene vacío, y está bien. No hay nada que decir todavía.
func TestLaHuellaDeUnaFacturaSinClasificarVieneVacia(t *testing.T) {
	repo := baseDeHuella(t)
	docID := facturaCxP(t, repo, "00100001010000023999", "", "")
	mov := sembrarMovDebito(t, repo, "2026-10-10", "520385391", "DEBITO MASIVO SINPE", 41930.37)
	if _, err := repo.pool.Exec(context.Background(),
		`UPDATE movimiento_bancario SET documento_cxp_id = $1::uuid WHERE id = $2::uuid`, docID, mov); err != nil {
		t.Fatalf("enlazar: %v", err)
	}

	p := huellaDe(t, repo, mov)
	if p == nil {
		t.Fatal("el movimiento perdió su enlace a CxP")
	}
	if p.Clasificacion != "" || p.Concepto != "" {
		t.Errorf("gasto = (%q, %q), quería vacío: la factura no está clasificada", p.Concepto, p.Clasificacion)
	}
	// Pero el enlace sigue ahí: el proveedor se muestra igual.
	if p.Proveedor != "VOLCANO 506 S.A" {
		t.Errorf("proveedor = %q: el enlace no depende de que haya gasto", p.Proveedor)
	}
}

// PAGO AGRUPADO CON EL MISMO GASTO: se muestra, porque uno solo describe el pago entero.
func TestLaHuellaDeUnaRemesaConUnSoloGastoLoMuestra(t *testing.T) {
	repo := baseDeHuella(t)
	sembrarRemesa(t, repo)
	facturaCxP(t, repo, "00100001010000024001", clasifHue, remHue)
	facturaCxP(t, repo, "00100001010000024002", clasifHue, remHue)

	mov := sembrarMovDebito(t, repo, "2026-10-11", "520385392", "DEBITO MASIVO SINPE REM", 83860.74)
	if _, err := repo.pool.Exec(context.Background(),
		`UPDATE movimiento_bancario SET remesa_id = $1::uuid WHERE id = $2::uuid`, remHue, mov); err != nil {
		t.Fatalf("enlazar remesa: %v", err)
	}

	p := huellaDe(t, repo, mov)
	if p == nil {
		t.Fatal("el movimiento perdió su enlace a la remesa")
	}
	if p.Facturas != 2 {
		t.Errorf("facturas = %d, quería 2", p.Facturas)
	}
	if p.Clasificacion != "Suministros de oficina" {
		t.Errorf("clasificación = %q: las dos facturas comparten gasto, así que hay uno que describe el pago", p.Clasificacion)
	}
}

// PAGO AGRUPADO CON GASTOS MEZCLADOS: NO se muestra ninguno.
func TestLaHuellaDeUnaRemesaConGastosMezcladosNoMuestraNinguno(t *testing.T) {
	repo := baseDeHuella(t)
	sembrarRemesa(t, repo)
	facturaCxP(t, repo, "00100001010000024011", clasifHue, remHue)
	facturaCxP(t, repo, "00100001010000024012", clasOtra, remHue)

	mov := sembrarMovDebito(t, repo, "2026-10-12", "520385393", "DEBITO MASIVO SINPE REM MIXTA", 83860.74)
	if _, err := repo.pool.Exec(context.Background(),
		`UPDATE movimiento_bancario SET remesa_id = $1::uuid WHERE id = $2::uuid`, remHue, mov); err != nil {
		t.Fatalf("enlazar remesa: %v", err)
	}

	p := huellaDe(t, repo, mov)
	if p == nil {
		t.Fatal("el movimiento perdió su enlace a la remesa")
	}
	if p.Clasificacion != "" {
		t.Errorf("clasificación = %q: el pago mezcla dos gastos, mostrar uno atribuiría la plata al rubro equivocado", p.Clasificacion)
	}
}

// LA TRAMPA DEL NULL: una clasificada y otra sin clasificar NO son «todas iguales».
//
// `count(DISTINCT clasificacion_id)` ignora los NULL, así que sin el FILTER este caso daría
// «distintos = 1» y mostraría el gasto de la primera como si describiera las dos.
func TestLaHuellaNoCuentaLaFacturaSinClasificarComoIgual(t *testing.T) {
	repo := baseDeHuella(t)
	sembrarRemesa(t, repo)
	facturaCxP(t, repo, "00100001010000024021", clasifHue, remHue)
	facturaCxP(t, repo, "00100001010000024022", "", remHue)

	mov := sembrarMovDebito(t, repo, "2026-10-13", "520385394", "DEBITO MASIVO SINPE REM PARCIAL", 83860.74)
	if _, err := repo.pool.Exec(context.Background(),
		`UPDATE movimiento_bancario SET remesa_id = $1::uuid WHERE id = $2::uuid`, remHue, mov); err != nil {
		t.Fatalf("enlazar remesa: %v", err)
	}

	p := huellaDe(t, repo, mov)
	if p == nil {
		t.Fatal("el movimiento perdió su enlace a la remesa")
	}
	if p.Clasificacion != "" {
		t.Errorf("clasificación = %q: una de las dos facturas no está clasificada, así que NO hay "+
			"un gasto que describa el pago entero", p.Clasificacion)
	}
}

func sembrarRemesa(t *testing.T, r *pgRepository) {
	t.Helper()
	ctx := context.Background()
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO lote_pago (id, empresa_id, numero, fecha_corte, modo_pago)
		 VALUES ($1::uuid,$2::uuid,29,'2026-10-09','PROVEEDOR') ON CONFLICT DO NOTHING`,
		loteHue, empExc); err != nil {
		t.Fatalf("sembrar lote: %v", err)
	}
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO remesa_pago (id, empresa_id, lote_id, proveedor_id, numero, huella, monto_crc)
		 VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,1,'CXPR-AAAA1111BBBB',83860.74)
		 ON CONFLICT DO NOTHING`,
		remHue, empExc, loteHue, provHue); err != nil {
		t.Fatalf("sembrar remesa: %v", err)
	}
}
