package cxp

import (
	"strings"
	"testing"
)

// La factura REAL, no un fixture: `testdata/factura_real_4.4.xml` entró por el buzón de Memorial
// Pets. Si el extractor se rompe contra un comprobante de verdad, la tabla de líneas se llena de
// vacíos y el análisis de gasto queda en cero sin que nada falle.
func TestDetalleDeLaFacturaReal(t *testing.T) {
	cs, err := leerComprobantes(cargarFacturaReal(t))
	if err != nil || len(cs) == 0 {
		t.Fatalf("no se pudo leer la factura real: %v (%d comprobantes)", err, len(cs))
	}
	d := DetalleDeXML(cs[0])

	if len(d.Clave) != 50 {
		t.Errorf("clave = %q (%d dígitos), quería 50 — es la llave con que se empareja", d.Clave, len(d.Clave))
	}
	if d.Moneda == "" {
		t.Error("moneda vacía: sin ella no se puede convertir a colones para agregar")
	}
	if len(d.Lineas) == 0 {
		t.Fatal("cero líneas en una factura real: el detalle es justamente lo que se viene a guardar")
	}
	for _, l := range d.Lineas {
		if l.Detalle == "" {
			t.Errorf("línea %d sin descripción", l.Linea)
		}
		if l.DetalleNorm == "" {
			t.Errorf("línea %d sin forma normalizada: no se podría agrupar", l.Linea)
		}
		if l.CABYS == "" {
			t.Errorf("línea %d sin CABYS", l.Linea)
		}
		// Sin precio unitario no hay comparación de precios entre compras, que es la razón de ser
		// de la pantalla.
		if l.PrecioUnitario == nil {
			t.Errorf("línea %d sin precio unitario", l.Linea)
		}
	}
}

// EL ORDINAL MANDA, NO LO QUE EL EMISOR NUMERE.
//
// `linea` sostiene la clave única (documento_id, linea). Si se usara `NumeroLinea` del XML, este
// comprobante —dos líneas numeradas «1»— perdería una al guardar: el UPSERT de la segunda pisaría
// a la primera y la factura quedaría con la mitad del gasto, sin error en ninguna parte.
func TestElOrdinalNoLoDecideElEmisor(t *testing.T) {
	const xml = `<?xml version="1.0" encoding="UTF-8"?>
<FacturaElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/facturaElectronica">
  <Clave>50601011500010000000100100001010000000011999999999</Clave>
  <DetalleServicio>
    <LineaDetalle><NumeroLinea>1</NumeroLinea><CodigoCABYS>2399001000000</CodigoCABYS>
      <Detalle>CEMENTO GRIS</Detalle><Cantidad>2</Cantidad><PrecioUnitario>5000</PrecioUnitario>
      <MontoTotalLinea>10000</MontoTotalLinea></LineaDetalle>
    <LineaDetalle><NumeroLinea>1</NumeroLinea><CodigoCABYS>2399002000000</CodigoCABYS>
      <Detalle>ARENA</Detalle><Cantidad>3</Cantidad><PrecioUnitario>1000</PrecioUnitario>
      <MontoTotalLinea>3000</MontoTotalLinea></LineaDetalle>
  </DetalleServicio>
  <ResumenFactura>
    <CodigoTipoMoneda><CodigoMoneda>CRC</CodigoMoneda><TipoCambio>1</TipoCambio></CodigoTipoMoneda>
    <TotalComprobante>13000</TotalComprobante>
  </ResumenFactura>
</FacturaElectronica>`

	cs, err := leerComprobantes([]byte(xml))
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	d := DetalleDeXML(cs[0])

	if len(d.Lineas) != 2 {
		t.Fatalf("líneas = %d, quería 2", len(d.Lineas))
	}
	if d.Lineas[0].Linea != 1 || d.Lineas[1].Linea != 2 {
		t.Errorf("ordinales = %d y %d, quería 1 y 2 — el emisor numeró las dos como «1»",
			d.Lineas[0].Linea, d.Lineas[1].Linea)
	}
	// Lo que el documento declaró se conserva tal cual, para que se vea el disparate si alguien revisa.
	if d.Lineas[1].NumeroLinea != "1" {
		t.Errorf("numero_linea declarado = %q, quería conservar el «1» del emisor", d.Lineas[1].NumeroLinea)
	}
}

// LOS DESCUENTOS SE SUMAN: son repetibles en el esquema.
//
// Quedarse con el primero dejaría esta línea declarando 500 de rebaja en vez de 800, y el precio
// unitario efectivo que se calcule después saldría alto sin que nada lo delate.
func TestLosDescuentosDeUnaLineaSeSuman(t *testing.T) {
	const xml = `<?xml version="1.0" encoding="UTF-8"?>
<FacturaElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/facturaElectronica">
  <Clave>50601011500010000000100100001010000000011999999999</Clave>
  <DetalleServicio>
    <LineaDetalle><NumeroLinea>1</NumeroLinea><CodigoCABYS>2399001000000</CodigoCABYS>
      <Detalle>CEMENTO GRIS</Detalle><Cantidad>10</Cantidad><PrecioUnitario>1000</PrecioUnitario>
      <Descuento><MontoDescuento>500</MontoDescuento><NaturalezaDescuento>Volumen</NaturalezaDescuento></Descuento>
      <Descuento><MontoDescuento>300</MontoDescuento><NaturalezaDescuento>Pronto pago</NaturalezaDescuento></Descuento>
      <MontoTotalLinea>9200</MontoTotalLinea></LineaDetalle>
  </DetalleServicio>
  <ResumenFactura><CodigoTipoMoneda><CodigoMoneda>CRC</CodigoMoneda></CodigoTipoMoneda></ResumenFactura>
</FacturaElectronica>`

	cs, err := leerComprobantes([]byte(xml))
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	d := DetalleDeXML(cs[0])

	if len(d.Lineas) != 1 {
		t.Fatalf("líneas = %d", len(d.Lineas))
	}
	if d.Lineas[0].Descuento == nil {
		t.Fatal("descuento nil: la línea trae dos")
	}
	if got := d.Lineas[0].Descuento.String(); got != "800" {
		t.Errorf("descuento = %s, quería 800 (500 + 300)", got)
	}
}

// NULL NO ES CERO.
//
// Un precio que el comprobante no trajo no puede entrar en un promedio; uno que vino en cero sí,
// porque es un dato que el emisor declaró. Si `numeroOpcional` devolviera cero para el ausente,
// los dos casos caerían en el mismo promedio y el precio medio de un ítem bajaría solo, sin que
// nadie pueda ver por qué.
func TestAusenteYCeroNoSonLoMismo(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		nil_   bool
		valor  string
	}{
		{"vacío es ausente", "", true, ""},
		{"solo espacios es ausente", "   ", true, ""},
		{"ilegible es ausente, no cero", "N/D", true, ""},
		{"cero es un dato", "0", false, "0"},
		{"cero con decimales es un dato", "0.00", false, "0"},
		{"número normal", "1234.56", false, "1234.56"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := numeroOpcional(c.in)
			if c.nil_ {
				if got != nil {
					t.Fatalf("%q dio %s, quería nil", c.in, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("%q dio nil, quería %s", c.in, c.valor)
			}
			if got.String() != c.valor {
				t.Errorf("%q dio %s, quería %s", c.in, got, c.valor)
			}
		})
	}
}

func TestNormalizarDetalleAgrupaLoQueEsLoMismo(t *testing.T) {
	casos := []struct{ a, b string }{
		{"TORNILLO 1/2", "Tornillo 1/2"},
		{"CAFÉ MOLIDO", "cafe molido"},
		{"BOLSA  NEGRA", "bolsa negra"},
		{"  Papel higiénico  ", "PAPEL HIGIENICO"},
		{"GUANTES (PAR)", "guantes par"},
	}
	for _, c := range casos {
		if na, nb := normalizarDetalle(c.a), normalizarDetalle(c.b); na != nb {
			t.Errorf("%q → %q y %q → %q: deberían agrupar juntos", c.a, na, c.b, nb)
		}
	}

	// Y lo que NO es lo mismo sigue separado: la normalización no inventa sinónimos.
	if normalizarDetalle("CAJA DE CARTON") == normalizarDetalle("CAJA DE MADERA") {
		t.Error("juntó caja de cartón con caja de madera")
	}
	if normalizarDetalle("") != "" {
		t.Error("una descripción vacía tiene que quedar vacía, no producir un grupo fantasma")
	}
	if n := normalizarDetalle("ALCOHOL 70%"); strings.Contains(n, "%") {
		t.Errorf("%q: los signos tienen que salir", n)
	}
}

// Las dos formas del nodo de moneda conviven en el parque de proveedores (v4.2 contra v4.3/4.4).
// Leer el campo a mano funciona con una y devuelve vacío con la otra, y una línea sin moneda no se
// puede convertir a colones para sumarla con las demás.
func TestLaMonedaSaleDeLasDosFormasDelNodo(t *testing.T) {
	const v42 = `<?xml version="1.0" encoding="UTF-8"?>
<FacturaElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.2/facturaElectronica">
  <Clave>50601011500010000000100100001010000000011999999999</Clave>
  <DetalleServicio><LineaDetalle><NumeroLinea>1</NumeroLinea><Detalle>SERVICIO</Detalle></LineaDetalle></DetalleServicio>
  <ResumenFactura><CodigoMoneda>USD</CodigoMoneda><TotalComprobante>100</TotalComprobante></ResumenFactura>
</FacturaElectronica>`

	cs, err := leerComprobantes([]byte(v42))
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if d := DetalleDeXML(cs[0]); d.Moneda != "USD" {
		t.Errorf("moneda v4.2 = %q, quería USD", d.Moneda)
	}
}
