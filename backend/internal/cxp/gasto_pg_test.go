package cxp

// El análisis de gasto contra PostgreSQL de verdad (9-oct-2026).
//
// Casi todo lo de esta función es SQL de agregación: `mode() WITHIN GROUP`, el `array_remove` que
// saca el último precio, la multiplicación por un tipo de cambio que puede ser NULL. Nada de eso
// se puede probar detrás de un doble —un fake devuelve lo que se le programe y el GROUP BY nunca
// corre—, y en este proyecto ya hubo un `MIN(uuid)` que pasó catorce suites y reventó con un 500
// contra la base.
//
// LO QUE SE PRUEBA ACÁ, EN UNA FRASE: que un total nunca mienta. Ni sumando una línea que no se
// pudo convertir como si valiera cero, ni comparando el precio del kilo con el de la unidad, ni
// dejando entrar el gasto de otra empresa.

import (
	"context"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

const (
	provGasto  = "a1a1a1a1-a1a1-4a1a-9a1a-a1a1a1a1a1a1"
	provGasto2 = "a2a2a2a2-a2a2-4a2a-9a2a-a2a2a2a2a2a2"
)

// baseDeGasto siembra un escenario con las trampas adentro:
//
//	2399001 CEMENTO  — dos proveedores, precios 5.000 y 6.500 (variación real del 30 %)
//	2399002 ARENA    — comprada por saco y por metro cúbico (NO se debe comparar)
//	2399003 TORNILLO — en una factura USD CON tipo de cambio (sí convierte)
//	2399004 PINTURA  — en una factura USD SIN tipo de cambio (no se puede convertir)
//	2399005 ATAÚD    — en OTRA empresa (nunca puede aparecer)
//	2399006 FLORES   — en una factura ANULADA (no es gasto)
func baseDeGasto(t *testing.T) *pgRepository {
	t.Helper()
	repo := baseDeLotes(t)
	ctx := context.Background()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := repo.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	exec(`INSERT INTO proveedor (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'FERRETERIA UNO')`,
		provGasto, empresaLote)
	exec(`INSERT INTO proveedor (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'FERRETERIA DOS')`,
		provGasto2, empresaLote)

	// doc devuelve el id para poder colgarle líneas.
	doc := func(empresa, prov, clave, estado, moneda string, tc any, fecha string, total int) string {
		t.Helper()
		var id string
		err := repo.pool.QueryRow(ctx, `INSERT INTO documento_cxp
			  (empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
			   tc_aplicado, total, total_crc)
		      VALUES ($1::uuid, $2::uuid, $3, $3, $4::date, $5, $6, $7::numeric, $8, $8)
		      RETURNING id::text`,
			empresa, prov, clave, fecha, estado, moneda, tc, total).Scan(&id)
		if err != nil {
			t.Fatalf("sembrar documento %s: %v", clave, err)
		}
		return id
	}

	dec := func(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
	lin := func(empresa, docID, cabys, detalle, unidad, cant, precio, total string) LineaDetalle {
		return LineaDetalle{
			Linea: 1, NumeroLinea: "1", CABYS: cabys, Detalle: detalle,
			DetalleNorm: normalizarDetalle(detalle), UnidadMedida: unidad,
			Cantidad: dec(cant), PrecioUnitario: dec(precio), MontoTotalLinea: dec(total),
		}
	}

	// CEMENTO: la misma bolsa a dos precios. Es el hallazgo que la pantalla viene a hacer visible.
	d1 := doc(empresaLote, provGasto, "50601000000000000000000000000000000000000000000001",
		"RECIBIDO", "CRC", nil, "2026-03-10", 50000)
	_ = repo.GuardarDetalle(ctx, empresaLote, d1,
		[]LineaDetalle{lin(empresaLote, d1, "2399001", "CEMENTO GRIS 50KG", "Sacos", "10", "5000", "50000")})

	d2 := doc(empresaLote, provGasto2, "50601000000000000000000000000000000000000000000002",
		"PAGADO", "CRC", nil, "2026-07-20", 65000)
	_ = repo.GuardarDetalle(ctx, empresaLote, d2,
		[]LineaDetalle{lin(empresaLote, d2, "2399001", "Cemento gris 50kg", "Sacos", "10", "6500", "65000")})

	// La tercera compra de cemento es la MÁS RECIENTE y NO es la más cara (se renegoció a 5.500).
	// Sin esta fila, "último precio" y "precio máximo" coincidirían y la prueba no distinguiría
	// entre sacar el último por fecha o sacarlo por monto: se verificó por mutación que no lo hacía.
	d2b := doc(empresaLote, provGasto2, "50601000000000000000000000000000000000000000000012",
		"PAGADO", "CRC", nil, "2026-09-15", 55000)
	_ = repo.GuardarDetalle(ctx, empresaLote, d2b,
		[]LineaDetalle{lin(empresaLote, d2b, "2399001", "CEMENTO GRIS 50KG", "Sacos", "10", "5500", "55000")})

	// ARENA: dos unidades de medida distintas bajo el mismo CABYS.
	d3 := doc(empresaLote, provGasto, "50601000000000000000000000000000000000000000000003",
		"RECIBIDO", "CRC", nil, "2026-04-01", 9000)
	_ = repo.GuardarDetalle(ctx, empresaLote, d3, []LineaDetalle{
		lin(empresaLote, d3, "2399002", "ARENA", "Sacos", "3", "1000", "3000"),
		func() LineaDetalle {
			l := lin(empresaLote, d3, "2399002", "ARENA", "m3", "1", "6000", "6000")
			l.Linea, l.NumeroLinea = 2, "2"
			return l
		}(),
	})

	// TORNILLO: factura en dólares CON tipo de cambio. $20 × 520 = ₡10.400.
	d4 := doc(empresaLote, provGasto, "50601000000000000000000000000000000000000000000004",
		"RECIBIDO", "USD", "520", "2026-05-05", 20)
	_ = repo.GuardarDetalle(ctx, empresaLote, d4,
		[]LineaDetalle{lin(empresaLote, d4, "2399003", "TORNILLO 1/2", "Unid", "20", "1", "20")})

	// PINTURA: factura en dólares SIN tipo de cambio. No hay con qué convertirla.
	d5 := doc(empresaLote, provGasto, "50601000000000000000000000000000000000000000000005",
		"RECIBIDO", "USD", nil, "2026-05-06", 300)
	_ = repo.GuardarDetalle(ctx, empresaLote, d5,
		[]LineaDetalle{lin(empresaLote, d5, "2399004", "PINTURA BLANCA", "Galon", "3", "100", "300")})

	// ATAÚD: otra empresa.
	d6 := doc(otraEmpLote, provGasto, "50601000000000000000000000000000000000000000000006",
		"RECIBIDO", "CRC", nil, "2026-05-07", 400000)
	_ = repo.GuardarDetalle(ctx, otraEmpLote, d6,
		[]LineaDetalle{lin(otraEmpLote, d6, "2399005", "ATAUD METALICO", "Unid", "1", "400000", "400000")})

	// FLORES: anulada.
	d7 := doc(empresaLote, provGasto, "50601000000000000000000000000000000000000000000007",
		"ANULADO", "CRC", nil, "2026-05-08", 25000)
	_ = repo.GuardarDetalle(ctx, empresaLote, d7,
		[]LineaDetalle{lin(empresaLote, d7, "2399006", "ARREGLO FLORAL", "Unid", "1", "25000", "25000")})

	return repo
}

func itemPorCABYS(items []ItemGasto, cabys string) *ItemGasto {
	for i := range items {
		if items[i].CABYS == cabys {
			return &items[i]
		}
	}
	return nil
}

// EL HALLAZGO: el mismo ítem a dos precios. Es la razón de ser de la pantalla.
func TestGastoMuestraLaVariacionDePrecioDelMismoItem(t *testing.T) {
	repo := baseDeGasto(t)
	items, _, _, err := repo.ItemsDeGasto(context.Background(), empresaLote, FiltrosGasto{})
	if err != nil {
		t.Fatalf("items: %v", err)
	}

	cem := itemPorCABYS(items, "2399001")
	if cem == nil {
		t.Fatal("el cemento no aparece en la lista")
	}
	if cem.Proveedores != 2 {
		t.Errorf("proveedores = %d, quería 2", cem.Proveedores)
	}
	if cem.Facturas != 3 {
		t.Errorf("facturas = %d, quería 3", cem.Facturas)
	}
	if !montoIgual(cem.GastoCRC, "170000") {
		t.Errorf("gasto = %s, quería 170000 (50.000 + 65.000 + 55.000)", cem.GastoCRC)
	}
	if !montoIgual(cem.PrecioMin, "5000") || !montoIgual(cem.PrecioMax, "6500") {
		t.Errorf("precios = [%s, %s], quería [5000, 6500]", cem.PrecioMin, cem.PrecioMax)
	}
	// EL ÚLTIMO PRECIO ES EL DE LA COMPRA MÁS RECIENTE, NO EL MÁS ALTO.
	//
	// En setiembre se compró a 5.500 tras renegociar; el máximo sigue siendo el 6.500 de julio.
	// Un "último precio" sacado por monto en vez de por fecha mostraría 6.500 y escondería justo
	// lo que se quiere ver: que el precio ya bajó. Antes de agregar esta tercera compra, la
	// prueba no distinguía los dos casos —se comprobó por mutación que no fallaba—.
	if !montoIgual(cem.PrecioUltimo, "5500") {
		t.Errorf("último precio = %s, quería 5500 (la compra de setiembre, la más reciente)", cem.PrecioUltimo)
	}
	if cem.VariacionPct != "30.0" {
		t.Errorf("variación = %s%%, quería 30.0", cem.VariacionPct)
	}
	// Las dos descripciones difieren solo en mayúsculas: la normalización las junta en una.
	if cem.Descripciones != 1 {
		t.Errorf("descripciones distintas = %d, quería 1 («CEMENTO GRIS 50KG» y «Cemento gris 50kg»)",
			cem.Descripciones)
	}
}

// DOS UNIDADES DE MEDIDA APAGAN LA COMPARACIÓN.
//
// La arena se compró por saco (₡1.000) y por metro cúbico (₡6.000). Comparar esos dos precios
// daría una "variación del 500 %" y mandaría a renegociar un ítem que nunca tuvo problema. El
// gasto sí se suma —son colones en los dos casos—; lo que se calla es el precio.
func TestGastoNoComparaPreciosDeUnidadesDistintas(t *testing.T) {
	repo := baseDeGasto(t)
	items, _, _, err := repo.ItemsDeGasto(context.Background(), empresaLote, FiltrosGasto{})
	if err != nil {
		t.Fatalf("items: %v", err)
	}

	arena := itemPorCABYS(items, "2399002")
	if arena == nil {
		t.Fatal("la arena no aparece")
	}
	if arena.Unidades != 2 {
		t.Errorf("unidades = %d, quería 2 (Sacos y m3)", arena.Unidades)
	}
	if arena.PrecioMin != "" || arena.PrecioMax != "" || arena.VariacionPct != "" {
		t.Errorf("comparó precios de unidades distintas: min=%q max=%q var=%q",
			arena.PrecioMin, arena.PrecioMax, arena.VariacionPct)
	}
	if !montoIgual(arena.GastoCRC, "9000") {
		t.Errorf("gasto = %s, quería 9000: el MONTO sí se suma, lo que no se compara es el precio",
			arena.GastoCRC)
	}
}

// LOS DÓLARES SE CONVIERTEN CON EL TC DE LA FACTURA.
func TestGastoConvierteLaFacturaEnDolares(t *testing.T) {
	repo := baseDeGasto(t)
	items, _, _, err := repo.ItemsDeGasto(context.Background(), empresaLote, FiltrosGasto{})
	if err != nil {
		t.Fatalf("items: %v", err)
	}

	torn := itemPorCABYS(items, "2399003")
	if torn == nil {
		t.Fatal("el tornillo no aparece")
	}
	if !montoIgual(torn.GastoCRC, "10400") {
		t.Errorf("gasto = %s, quería 10400 ($20 × 520)", torn.GastoCRC)
	}
	if !montoIgual(torn.PrecioUltimo, "520") {
		t.Errorf("precio unitario = %s, quería 520 ($1 × 520): comparar un precio en dólares "+
			"contra uno en colones sin convertir daría una variación inventada", torn.PrecioUltimo)
	}
}

// LA LÍNEA QUE NO SE PUEDE CONVERTIR NO VALE CERO: VALE DESCONOCIDO, Y SE DICE.
//
// La pintura está en una factura USD sin tipo de cambio. Si el SQL hiciera COALESCE(tc, 0), el
// ítem aparecería con un gasto de ₡0 —como si la pintura fuera gratis— y nadie tendría forma de
// notarlo. El monto queda en cero PERO el contador lo delata.
func TestGastoNoTrataLoNoConvertibleComoCero(t *testing.T) {
	repo := baseDeGasto(t)
	items, _, _, err := repo.ItemsDeGasto(context.Background(), empresaLote, FiltrosGasto{})
	if err != nil {
		t.Fatalf("items: %v", err)
	}

	pint := itemPorCABYS(items, "2399004")
	if pint == nil {
		t.Fatal("la pintura no aparece: una línea que no se pudo convertir NO se descarta, se avisa")
	}
	if pint.SinConvertir != 1 {
		t.Errorf("sin_convertir = %d, quería 1 — es lo único que delata que el monto está incompleto",
			pint.SinConvertir)
	}

	cob, err := repo.CoberturaDeGasto(context.Background(), empresaLote, FiltrosGasto{})
	if err != nil {
		t.Fatalf("cobertura: %v", err)
	}
	if cob.LineasSinConvert != 1 {
		t.Errorf("líneas sin convertir en la cobertura = %d, quería 1", cob.LineasSinConvert)
	}
}

// NI OTRA EMPRESA NI UNA FACTURA ANULADA.
func TestGastoNoCruzaEmpresasNiCuentaAnuladas(t *testing.T) {
	repo := baseDeGasto(t)
	items, total, _, err := repo.ItemsDeGasto(context.Background(), empresaLote, FiltrosGasto{})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	if itemPorCABYS(items, "2399005") != nil {
		t.Error("apareció el ataúd de la OTRA empresa")
	}
	if itemPorCABYS(items, "2399006") != nil {
		t.Error("apareció el arreglo floral de una factura ANULADA: no es gasto")
	}
	// 170.000 (cemento) + 9.000 (arena) + 10.400 (tornillo) + 0 (pintura sin TC) = 189.400
	if !montoIgual(total, "189400") {
		t.Errorf("total = %s, quería 189400", total)
	}

	// Y desde la otra empresa se ve lo suyo y solo lo suyo.
	otros, _, _, err := repo.ItemsDeGasto(context.Background(), otraEmpLote, FiltrosGasto{})
	if err != nil {
		t.Fatalf("items otra empresa: %v", err)
	}
	if len(otros) != 1 || otros[0].CABYS != "2399005" {
		t.Errorf("la otra empresa ve %d ítems, quería solo el suyo", len(otros))
	}
}

// LA COBERTURA DICE SOBRE QUÉ PARTE DEL GASTO SE ESTÁ HABLANDO.
//
// Se agrega una factura SIN detalle cargado: tiene que contar en el denominador. Sin eso, la
// pantalla diría «100 % cubierto» mirando solo las facturas que ya tienen XML, que es exactamente
// la mentira que esta sección existe para evitar.
func TestCoberturaCuentaLasFacturasSinDetalle(t *testing.T) {
	repo := baseDeGasto(t)
	ctx := context.Background()

	if _, err := repo.pool.Exec(ctx, `INSERT INTO documento_cxp
		  (empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda, total, total_crc)
	      VALUES ($1::uuid, $2::uuid, $3, $3, '2026-06-01'::date, 'RECIBIDO', 'CRC', 1000000, 1000000)`,
		empresaLote, provGasto, "50601000000000000000000000000000000000000000000099"); err != nil {
		t.Fatalf("sembrar factura sin detalle: %v", err)
	}

	cob, err := repo.CoberturaDeGasto(ctx, empresaLote, FiltrosGasto{})
	if err != nil {
		t.Fatalf("cobertura: %v", err)
	}
	// 7 facturas de la empresa que no están anuladas (6 con detalle + la nueva sin detalle).
	if cob.Facturas != 7 {
		t.Errorf("facturas = %d, quería 7", cob.Facturas)
	}
	if cob.FacturasConDet != 6 {
		t.Errorf("facturas con detalle = %d, quería 6", cob.FacturasConDet)
	}
	// La de ₡1.000.000 no tiene detalle, así que la cobertura por monto tiene que ser BAJA.
	if cob.PorcentajeMonto == "100" || cob.PorcentajeMonto == "100.0" {
		t.Errorf("cobertura = %s %%: hay ₡1.000.000 sin detalle que no se está contando",
			cob.PorcentajeMonto)
	}
}

// GUARDAR DOS VECES EL MISMO XML DEJA LA FACTURA IGUAL, NO CON EL DETALLE DUPLICADO.
func TestGuardarDetalleReemplazaEnVezDeAcumular(t *testing.T) {
	repo := baseDeGasto(t)
	ctx := context.Background()

	var docID string
	if err := repo.pool.QueryRow(ctx,
		`SELECT id::text FROM documento_cxp WHERE empresa_id = $1::uuid AND clave = $2`,
		empresaLote, "50601000000000000000000000000000000000000000000001").Scan(&docID); err != nil {
		t.Fatalf("buscar documento: %v", err)
	}

	d := decimal.RequireFromString("5000")
	linea := []LineaDetalle{{
		Linea: 1, NumeroLinea: "1", CABYS: "2399001", Detalle: "CEMENTO GRIS 50KG",
		DetalleNorm: normalizarDetalle("CEMENTO GRIS 50KG"), UnidadMedida: "Sacos",
		PrecioUnitario: &d, MontoTotalLinea: &d,
	}}
	for i := 0; i < 3; i++ {
		if err := repo.GuardarDetalle(ctx, empresaLote, docID, linea); err != nil {
			t.Fatalf("guardar (vuelta %d): %v", i+1, err)
		}
	}

	lineas, err := repo.LineasDeDocumento(ctx, empresaLote, docID)
	if err != nil {
		t.Fatalf("leer líneas: %v", err)
	}
	if len(lineas) != 1 {
		t.Fatalf("líneas = %d tras cargar 3 veces el mismo XML, quería 1", len(lineas))
	}

	var marca int
	if err := repo.pool.QueryRow(ctx,
		`SELECT detalle_lineas FROM documento_cxp WHERE id = $1::uuid`, docID).Scan(&marca); err != nil {
		t.Fatalf("leer marca: %v", err)
	}
	if marca != 1 {
		t.Errorf("detalle_lineas = %d, quería 1", marca)
	}
}

// El detalle de un ítem: cada compra con su fecha, su proveedor y su precio ya convertido.
func TestComprasDeItemListaCadaCompra(t *testing.T) {
	repo := baseDeGasto(t)
	compras, err := repo.ComprasDeItem(context.Background(), empresaLote, "2399001", FiltrosGasto{})
	if err != nil {
		t.Fatalf("compras: %v", err)
	}
	if len(compras) != 3 {
		t.Fatalf("compras = %d, quería 3", len(compras))
	}
	// De la más reciente a la más vieja: la primera fila es la compra de setiembre.
	if !strings.HasPrefix(compras[0].Fecha, "2026-09") {
		t.Errorf("primera compra = %s, quería la de setiembre (orden descendente por fecha)", compras[0].Fecha)
	}
	if compras[0].Proveedor != "FERRETERIA DOS" {
		t.Errorf("proveedor = %q, quería FERRETERIA DOS", compras[0].Proveedor)
	}
	if !montoIgual(compras[0].PrecioUnitario, "5500") {
		t.Errorf("precio = %s, quería 5500", compras[0].PrecioUnitario)
	}
}

// El filtro por fecha recorta de verdad: la compra de julio queda afuera de un corte a junio.
func TestGastoFiltraPorFecha(t *testing.T) {
	repo := baseDeGasto(t)
	items, _, _, err := repo.ItemsDeGasto(context.Background(), empresaLote,
		FiltrosGasto{Desde: "2026-01-01", Hasta: "2026-06-30"})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	cem := itemPorCABYS(items, "2399001")
	if cem == nil {
		t.Fatal("el cemento no aparece en el rango")
	}
	if cem.Facturas != 1 || !montoIgual(cem.GastoCRC, "50000") {
		t.Errorf("en el corte a junio: facturas=%d gasto=%s, quería 1 y 50000",
			cem.Facturas, cem.GastoCRC)
	}
}

// montoIgual compara dos montos por VALOR, no por texto: `numeric` vuelve de la base con los
// decimales que tenga la columna ("50000.00000"), y compararlo como string contra "50000" fallaría
// por el formato aunque el número esté bien.
func montoIgual(got, want string) bool {
	g, err1 := decimal.NewFromString(strings.TrimSpace(got))
	w, err2 := decimal.NewFromString(want)
	if err1 != nil || err2 != nil {
		return false
	}
	return g.Equal(w)
}
