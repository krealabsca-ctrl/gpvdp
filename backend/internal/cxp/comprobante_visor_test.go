package cxp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// LA PRUEBA CONTRA LA FACTURA REAL.
//
// `testdata/factura_real_4.4.xml` es una factura de verdad, firmada, que entró por el buzón de
// Memorial Pets el 14 de setiembre de 2026. Los 6 fixtures sintéticos que ya tiene el proyecto
// NO TIENEN NI UNA LÍNEA DE DETALLE, así que la suite entera podría pasar sin haber renderizado
// jamás una línea. Por eso esta prueba existe y por eso el archivo está versionado.
func cargarFacturaReal(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/factura_real_4.4.xml")
	if err != nil {
		t.Fatalf("no se pudo leer la factura real: %v", err)
	}
	return data
}

func TestVisorLeeLaFacturaReal(t *testing.T) {
	r := VerComprobante(cargarFacturaReal(t))

	if r.Modo != VisorComprobante {
		t.Fatalf("modo = %s, quería %s", r.Modo, VisorComprobante)
	}
	if r.Comprobante == nil {
		t.Fatal("no devolvió comprobante")
	}
	c := r.Comprobante

	casos := []struct{ campo, got, quiero string }{
		{"tipo", c.Tipo, "FacturaElectronica"},
		{"tipo legible", c.TipoNombre, "Factura electrónica"},
		{"clave", c.Clave, "50614092600310163508700200001010000215398101008166"},
		{"consecutivo", c.Consecutivo, "00200001010000215398"},
		{"emisor", c.Emisor.Nombre, "ENGRANES SOFT S.A"},
		{"cédula del emisor", c.Emisor.Identificacion, "3101635087"},
		{"receptor", c.Receptor.Nombre, "Memorial Pets"},
		{"cédula del receptor", c.Receptor.Identificacion, "3101794025"},
		{"moneda", c.Moneda, "CRC"},
		{"condición de venta", c.CondicionVenta, "01"},
		{"condición legible", c.CondicionVentaEtiqueta, "Contado"},
		// EL NÚMERO QUE IMPORTA: es el que el ERP registró y el que el Director validó con Factun.
		{"total del comprobante", c.TotalComprobante, "140251.08"},
		{"total impuesto", c.TotalImpuesto, "16135.08"},
		{"total descuentos", c.TotalDescuentos, "21903"},
		{"venta neta", c.TotalVentaNeta, "124116"},
	}
	for _, x := range casos {
		if x.got != x.quiero {
			t.Errorf("%s = %q, quería %q", x.campo, x.got, x.quiero)
		}
	}
	if !c.ClaveValida {
		t.Error("la clave de 50 dígitos tenía que dar válida")
	}
	if c.Descuadre != "" {
		t.Errorf("esta factura cuadra; el visor dijo: %s", c.Descuadre)
	}
}

// LA LÍNEA: lo que hoy no se puede ver por ningún lado.
func TestVisorLeeLaLineaDeDetalle(t *testing.T) {
	r := VerComprobante(cargarFacturaReal(t))
	if len(r.Comprobante.Lineas) != 1 {
		t.Fatalf("líneas = %d, quería 1", len(r.Comprobante.Lineas))
	}
	l := r.Comprobante.Lineas[0]

	// La descripción completa. En `documento_cxp` esta misma factura guarda 45 caracteres.
	const detalle = "PLAQUITA BLANCA DE ALUMINIO MILITAR GRANDE SUBLIMABLE 1664"
	if l.Detalle != detalle {
		t.Errorf("detalle = %q, quería %q", l.Detalle, detalle)
	}
	if len(l.Detalle) <= 45 {
		t.Errorf("el detalle tiene %d caracteres: esta prueba pierde sentido si no supera los 45 que guarda documento_cxp", len(l.Detalle))
	}

	casos := []struct{ campo, got, quiero string }{
		{"número de línea", l.Numero, "1"},
		{"cantidad", l.Cantidad, "300"},
		{"unidad", l.UnidadMedida, "Unid"},
		{"precio unitario", l.PrecioUnitario, "486.73"},
		{"monto total", l.MontoTotal, "146019"},
		{"subtotal", l.SubTotal, "124116"},
		{"base imponible", l.BaseImponible, "124116"},
		{"total de la línea", l.MontoTotalLinea, "140251.08"},
		{"CABYS", l.CABYS, "4299905999900"},
		// El código del proveedor: es el que sirve para cotejar contra inventario.
		{"código comercial", l.CodigoComercial, "PALM004"},
	}
	for _, x := range casos {
		if x.got != x.quiero {
			t.Errorf("%s = %q, quería %q", x.campo, x.got, x.quiero)
		}
	}

	if len(l.Descuentos) != 1 {
		t.Fatalf("descuentos = %d, quería 1", len(l.Descuentos))
	}
	if l.Descuentos[0].Monto != "21903" {
		t.Errorf("monto del descuento = %q", l.Descuentos[0].Monto)
	}
	// La naturaleza sale del propio XML: no es una traducción nuestra.
	if l.Descuentos[0].Naturaleza != "Descuento al cliente" {
		t.Errorf("naturaleza del descuento = %q", l.Descuentos[0].Naturaleza)
	}

	if len(l.Impuestos) != 1 {
		t.Fatalf("impuestos = %d, quería 1", len(l.Impuestos))
	}
	i := l.Impuestos[0]
	// EL PORCENTAJE SALE DEL XML, no de una tabla de códigos inventada.
	if i.Tarifa != "13" {
		t.Errorf("tarifa = %q, quería 13 (el <Tarifa> del XML)", i.Tarifa)
	}
	if i.CodigoTarifa != "08" {
		t.Errorf("código de tarifa = %q, quería 08 crudo", i.CodigoTarifa)
	}
	if i.Monto != "16135.08" {
		t.Errorf("monto del impuesto = %q", i.Monto)
	}
}

// EL DESGLOSE POR TARIFA: la respuesta a los «varios IVAs».
func TestVisorLeeElDesglosePorTarifa(t *testing.T) {
	r := VerComprobante(cargarFacturaReal(t))
	d := r.Comprobante.Desglose
	if len(d) != 1 {
		t.Fatalf("desglose = %d filas, quería 1", len(d))
	}
	if d[0].Monto != "16135.08" {
		t.Errorf("monto del desglose = %q", d[0].Monto)
	}
	if d[0].CodigoTarifa != "08" {
		t.Errorf("código de tarifa del desglose = %q", d[0].CodigoTarifa)
	}
	// El desglose NO trae <Tarifa>: se toma de la línea que comparte el código. Sin esto, la fila
	// del desglose saldría sin porcentaje y el usuario no sabría a qué tasa corresponde el monto.
	if d[0].Tarifa != "13" {
		t.Errorf("tarifa del desglose = %q, quería 13 tomada de la línea con el mismo código", d[0].Tarifa)
	}
}

// El medio de pago es COMPUESTO en 4.4: leerlo como string daría cadena vacía sin error.
func TestVisorLeeElMedioDePagoCompuesto(t *testing.T) {
	r := VerComprobante(cargarFacturaReal(t))
	m := r.Comprobante.MediosPago
	if len(m) != 1 {
		t.Fatalf("medios de pago = %d, quería 1", len(m))
	}
	if m[0].Tipo != "01" || m[0].Monto != "140251.08" {
		t.Errorf("medio de pago = %+v, quería tipo 01 por 140251.08", m[0])
	}
}

// DOS LÍNEAS CON TARIFAS DISTINTAS: el caso que el Director pidió explícitamente.
//
// No es hipotético: 414 de 3.455 facturas con IVA (12,0 %) tienen una tasa efectiva que no es
// ninguna tarifa legal, o sea que mezclan tarifas. Una factura real del Director tiene tres líneas
// al 13 % y una bolsa al 1 %, lo que da 11,2 % efectivo — un número que no existe en la ley.
func TestVisorConVariasTarifas(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?>
<FacturaElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/facturaElectronica">
  <Clave>50614092600310163508700200001010000215398101008167</Clave>
  <NumeroConsecutivo>00100001010000063274</NumeroConsecutivo>
  <FechaEmision>2026-09-07T11:00:44-06:00</FechaEmision>
  <Emisor><Nombre>MAYCSOLUCIONESCR SOCIEDAD ANONIMA</Nombre>
    <Identificacion><Tipo>02</Tipo><Numero>3101749605</Numero></Identificacion></Emisor>
  <Receptor><Nombre>VALLE DE PAZ SERVICIOS FUNERARIOS S.A.</Nombre>
    <Identificacion><Tipo>02</Tipo><Numero>3101318985</Numero></Identificacion></Receptor>
  <CondicionVenta>02</CondicionVenta>
  <DetalleServicio>
    <LineaDetalle><NumeroLinea>1</NumeroLinea><Detalle>DESODORANTE AMBIENTAL GLADE</Detalle>
      <Cantidad>6</Cantidad><SubTotal>16164.00</SubTotal>
      <Impuesto><Codigo>01</Codigo><CodigoTarifaIVA>08</CodigoTarifaIVA><Tarifa>13</Tarifa><Monto>2101.32</Monto></Impuesto>
      <MontoTotalLinea>18265.32</MontoTotalLinea></LineaDetalle>
    <LineaDetalle><NumeroLinea>2</NumeroLinea><Detalle>BOLSA EN ROLLO HYP GRANDE NEGRA</Detalle>
      <Cantidad>5</Cantidad><SubTotal>7125.00</SubTotal>
      <Impuesto><Codigo>01</Codigo><CodigoTarifaIVA>04</CodigoTarifaIVA><Tarifa>1</Tarifa><Monto>71.25</Monto></Impuesto>
      <MontoTotalLinea>7196.25</MontoTotalLinea></LineaDetalle>
  </DetalleServicio>
  <ResumenFactura>
    <CodigoTipoMoneda><CodigoMoneda>CRC</CodigoMoneda><TipoCambio>1</TipoCambio></CodigoTipoMoneda>
    <TotalVentaNeta>23289.00</TotalVentaNeta>
    <TotalDesgloseImpuesto><Codigo>01</Codigo><CodigoTarifaIVA>08</CodigoTarifaIVA><TotalMontoImpuesto>2101.32</TotalMontoImpuesto></TotalDesgloseImpuesto>
    <TotalDesgloseImpuesto><Codigo>01</Codigo><CodigoTarifaIVA>04</CodigoTarifaIVA><TotalMontoImpuesto>71.25</TotalMontoImpuesto></TotalDesgloseImpuesto>
    <TotalImpuesto>2172.57</TotalImpuesto>
    <TotalComprobante>25461.57</TotalComprobante>
  </ResumenFactura>
</FacturaElectronica>`

	r := VerComprobante([]byte(xml))
	if r.Modo != VisorComprobante {
		t.Fatalf("modo = %s", r.Modo)
	}
	c := r.Comprobante

	// LAS DOS LÍNEAS. Si `Lineas` fuera un campo escalar, encoding/xml se quedaría con la ÚLTIMA
	// con err=nil, y la pantalla mostraría la bolsa como si fuera toda la factura.
	if len(c.Lineas) != 2 {
		t.Fatalf("líneas = %d, quería 2 — ojo con declarar LineaDetalle como campo escalar", len(c.Lineas))
	}
	if c.Lineas[0].Impuestos[0].Tarifa != "13" {
		t.Errorf("tarifa de la línea 1 = %q, quería 13", c.Lineas[0].Impuestos[0].Tarifa)
	}
	if c.Lineas[1].Impuestos[0].Tarifa != "1" {
		t.Errorf("tarifa de la línea 2 = %q, quería 1", c.Lineas[1].Impuestos[0].Tarifa)
	}

	// EL DESGLOSE CON DOS TARIFAS: es lo que vuelve explicable una tasa efectiva de 9,3 %.
	if len(c.Desglose) != 2 {
		t.Fatalf("desglose = %d filas, quería 2 (13 %% y 1 %%)", len(c.Desglose))
	}
	porCodigo := map[string]ImpuestoVisor{}
	for _, d := range c.Desglose {
		porCodigo[d.CodigoTarifa] = d
	}
	if porCodigo["08"].Tarifa != "13" || porCodigo["08"].Monto != "2101.32" {
		t.Errorf("desglose del 13 %% = %+v", porCodigo["08"])
	}
	if porCodigo["04"].Tarifa != "1" || porCodigo["04"].Monto != "71.25" {
		t.Errorf("desglose del 1 %% = %+v", porCodigo["04"])
	}
	if c.CondicionVentaEtiqueta != "Crédito" {
		t.Errorf("condición = %q, quería Crédito", c.CondicionVentaEtiqueta)
	}
}

// NINGÚN SLICE SALE NIL. `null` en el JSON revienta un .map() en el frontend, y acá el caso vacío
// es el NORMAL: el visor se abre justamente en las recepciones raras.
func TestVisorNuncaSerializaNull(t *testing.T) {
	// Un comprobante sin detalle, sin desglose y sin medios de pago: el peor caso.
	xml := `<FacturaElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/facturaElectronica">
	  <Clave>506</Clave><ResumenFactura><TotalComprobante>100</TotalComprobante></ResumenFactura>
	</FacturaElectronica>`
	r := VerComprobante([]byte(xml))

	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("no serializa: %v", err)
	}
	for _, campo := range []string{`"lineas":null`, `"desglose":null`, `"medios_pago":null`, `"avisos":null`} {
		if strings.Contains(string(b), campo) {
			t.Errorf("el JSON trae %s — un .map() sobre eso revienta la pantalla", campo)
		}
	}
	// Y una clave que no mide 50 no puede darse por válida: 16 de 4.542 del sistema no los tienen.
	if r.Comprobante.ClaveValida {
		t.Error("una clave de 3 dígitos no puede salir como válida")
	}
}

// Un archivo ilegible es un MODO, no un error 500: el visor se abre justo cuando algo salió raro.
func TestVisorConArchivoIlegible(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		data   []byte
		modo   string
	}{
		{"vacío", nil, VisorSinXML},
		{"no es XML", []byte("esto no es un comprobante"), VisorDesconocido},
		{"XML de otra cosa", []byte(`<hola><mundo/></hola>`), VisorDesconocido},
	} {
		r := VerComprobante(caso.data)
		if r.Modo != caso.modo {
			t.Errorf("%s: modo = %s, quería %s", caso.nombre, r.Modo, caso.modo)
		}
		if len(r.Avisos) == 0 {
			t.Errorf("%s: tenía que explicar en palabras qué pasó", caso.nombre)
		}
		if r.Avisos == nil {
			t.Errorf("%s: avisos nil", caso.nombre)
		}
	}
}

// EL ACUSE DE HACIENDA NO SE PINTA COMO FACTURA.
//
// Es la trampa más peligrosa del visor: `MensajeHacienda` es XML perfectamente válido del mismo
// namespace base, y sin despachar por la raíz se renderiza como una factura con TODOS LOS MONTOS
// EN CERO y sin un solo error. El correo del proveedor suele traer la factura Y su acuse, así que
// el caso no es raro: es el de todos los días.
func TestVisorNoPintaUnAcuseComoFactura(t *testing.T) {
	acuse := `<MensajeHacienda xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/mensajeHacienda">
	  <Clave>50614092600310163508700200001010000215398101008166</Clave>
	  <NombreEmisor>ENGRANES SOFT S.A</NombreEmisor>
	  <Mensaje>1</Mensaje><DetalleMensaje>Comprobante aceptado</DetalleMensaje>
	  <MontoTotalImpuesto>16135.08</MontoTotalImpuesto><TotalFactura>140251.08</TotalFactura>
	</MensajeHacienda>`

	r := VerComprobante([]byte(acuse))
	if r.Modo != VisorDesconocido {
		t.Fatalf("modo = %s, quería %s: un acuse NO es una factura", r.Modo, VisorDesconocido)
	}
	if r.Comprobante != nil {
		t.Error("no puede devolver comprobante: se pintaría todo en cero como si fuera la factura")
	}
	// Y tiene que decir QUÉ es, no solo que no se pudo: el usuario necesita saber que la factura
	// está en otro archivo del mismo correo.
	junto := strings.Join(r.Avisos, " ")
	if !strings.Contains(junto, "ACUSE DE HACIENDA") {
		t.Errorf("tenía que nombrar el acuse: %q", junto)
	}
}

// El visor avisa lo que NO pudo leer, en vez de callarlo.
func TestVisorAvisaLoQueNoLee(t *testing.T) {
	// Nota de crédito: no se lee a qué factura corrige, y eso hay que decirlo.
	xml := `<NotaCreditoElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/notaCreditoElectronica">
	  <Clave>50614092600310163508700200001010000215398101008168</Clave>
	  <ResumenFactura><TotalComprobante>5000</TotalComprobante>
	    <CodigoTipoMoneda><CodigoMoneda>CRC</CodigoMoneda></CodigoTipoMoneda></ResumenFactura>
	</NotaCreditoElectronica>`
	r := VerComprobante([]byte(xml))

	junto := strings.Join(r.Avisos, " | ")
	if !strings.Contains(junto, "nota de crédito") {
		t.Errorf("tenía que avisar que es nota de crédito y que no se lee la referencia: %q", junto)
	}
	if !strings.Contains(junto, "no trae detalle de líneas") {
		t.Errorf("tenía que avisar que no hay líneas: %q", junto)
	}
	if r.Comprobante.TipoNombre != "Nota de crédito" {
		t.Errorf("tipo legible = %q", r.Comprobante.TipoNombre)
	}
}

// Un descuadre real se muestra; no se corrige ni se esconde.
func TestVisorMuestraElDescuadreSinCorregirlo(t *testing.T) {
	xml := `<FacturaElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/facturaElectronica">
	  <Clave>50614092600310163508700200001010000215398101008169</Clave>
	  <ResumenFactura>
	    <CodigoTipoMoneda><CodigoMoneda>CRC</CodigoMoneda></CodigoTipoMoneda>
	    <TotalVentaNeta>1000</TotalVentaNeta><TotalImpuesto>130</TotalImpuesto>
	    <TotalComprobante>9999</TotalComprobante>
	  </ResumenFactura>
	</FacturaElectronica>`
	r := VerComprobante([]byte(xml))

	if r.Comprobante.Descuadre == "" {
		t.Fatal("1000 + 130 no da 9999: tenía que marcar el descuadre")
	}
	// Y los montos quedan TAL COMO VINIERON: el visor no arregla la factura del proveedor.
	if r.Comprobante.TotalComprobante != "9999" {
		t.Errorf("el total se alteró: %q", r.Comprobante.TotalComprobante)
	}
	if !strings.Contains(strings.Join(r.Avisos, " "), "no cuadran") {
		t.Error("el descuadre tenía que salir también como aviso arriba")
	}
}

// El tipo de cambio solo importa cuando la moneda no es colones, pero tiene que LEERSE siempre.
func TestVisorLeeMonedaExtranjera(t *testing.T) {
	xml := `<FacturaElectronica xmlns="https://cdn.comprobanteselectronicos.go.cr/xml-schemas/v4.4/facturaElectronica">
	  <Clave>50614092600310163508700200001010000215398101008170</Clave>
	  <ResumenFactura>
	    <CodigoTipoMoneda><CodigoMoneda>USD</CodigoMoneda><TipoCambio>512.35</TipoCambio></CodigoTipoMoneda>
	    <TotalComprobante>100.00</TotalComprobante>
	  </ResumenFactura>
	</FacturaElectronica>`
	r := VerComprobante([]byte(xml))
	if r.Comprobante.Moneda != "USD" || r.Comprobante.TipoCambio != "512.35" {
		t.Errorf("moneda = %q, tc = %q", r.Comprobante.Moneda, r.Comprobante.TipoCambio)
	}
}
