package cxp

import (
	"strings"
	"testing"
)

// El nombre del archivo lo escribe una PERSONA copiando la descripción de la macro. Entre lo que
// la macro emite y lo que termina siendo un nombre de archivo se pierden caracteres: el banco borra
// el guion, Windows no acepta algunos, y quien renombra usa el separador que tiene a mano.
func TestCodigosDelNombre(t *testing.T) {
	casos := []struct {
		nombre string
		quiere []string
		porque string
	}{
		{"A8 FC 000487 CXP-54A4281AE49A.pdf", []string{"CXP-54A4281AE49A"}, "la descripción tal cual sale de la macro"},
		{"A8 FC 000487 CXP54A4281AE49A.pdf", []string{"CXP-54A4281AE49A"}, "sin guion: así vuelve del banco y así lo copian"},
		{"FC_000487_CXP_54A4281AE49A.pdf", []string{"CXP-54A4281AE49A"}, "con guion bajo, que es como se renombra en Windows"},
		{"fc 000487 cxp54a4281ae49a.pdf", []string{"CXP-54A4281AE49A"}, "en minúscula"},
		{"CXP.54A4281AE49A.pdf", []string{"CXP-54A4281AE49A"}, "con punto de separador"},
		{"A9 REM 0043-07 CXPR-7K2M9P4XQ1BT.pdf", []string{"CXPR-7K2M9P4XQ1BT"}, "remesa con guion"},
		{"A9 REM 0043-07 CXPR7K2M9P4XQ1BT.pdf", []string{"CXPR-7K2M9P4XQ1BT"}, "remesa sin guion"},
		{"CXPR7K2M9P4XQ1B.pdf", []string{"CXP-R7K2M9P4XQ1B"}, "FACTURA cuyo código empieza con R: no es una remesa"},
		{"comprobante_8471263.pdf", nil, "el nombre que pone el banco no dice nada"},
		{"CXP-54A4281AE49A CXP-12B8CF71477D.pdf", []string{"CXP-54A4281AE49A", "CXP-12B8CF71477D"}, "dos pagos en un nombre"},
		{"CXP-54A4281AE49A y CXP54A4281AE49A.pdf", []string{"CXP-54A4281AE49A"}, "el mismo código dos veces es UNO"},
		{"CXP-54A4281AE49.pdf", nil, "once caracteres no es un código"},
		{"CXP-54A4281AE49A99.pdf", nil, "catorce tampoco: recortar a doce daría el código de OTRA factura"},
		{"sin codigo pero dice CXP en el texto.pdf", nil, "la palabra CXP suelta no es un código"},
		{"CXP-54A4281AE49A", []string{"CXP-54A4281AE49A"}, "sin extensión"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := CodigosDelNombre(c.nombre)
			if len(got) != len(c.quiere) {
				t.Fatalf("%s\n%q → %v, quería %v", c.porque, c.nombre, got, c.quiere)
			}
			for i := range got {
				if got[i] != c.quiere[i] {
					t.Fatalf("%s\n%q → %v, quería %v", c.porque, c.nombre, got, c.quiere)
				}
			}
		})
	}
}

// Los dos códigos miden 12 y el de remesa lleva una letra más de prefijo, así que sin guion se
// parecen. Lo que los separa es el borde: la lectura equivocada siempre deja un carácter colgando.
//
// Nota de lo que esta prueba NO demuestra: probé a invertir el orden de lectura en `codigoEn` y
// ningún caso cambió. En el nombre de un archivo el orden es redundante —el borde hace el trabajo—
// y por eso acá no hay una prueba que lo fije. La que sí lo fija es la del estado de cuenta, donde
// el texto sigue después del código y el borde no se puede exigir.
func TestRemesaYFacturaNoSeConfunden(t *testing.T) {
	cods := CodigosDelNombre("CXPR7K2M9P4XQ1BT.pdf")
	if len(cods) != 1 || cods[0] != "CXPR-7K2M9P4XQ1BT" {
		t.Fatalf("una remesa sin guion se leyó como %v; si se lee como factura («CXP-R7K2M9P4XQ1B»), "+
			"el recibo del pago agrupado no encuentra a quién pegarse", cods)
	}
	// Y al revés: una factura NUNCA se puede leer como remesa, porque le faltaría un carácter.
	cods = CodigosDelNombre("CXPR7K2M9P4XQ1B.pdf")
	if len(cods) != 1 || cods[0] != "CXP-R7K2M9P4XQ1B" {
		t.Fatalf("una factura cuyo código empieza con R se leyó como %v", cods)
	}
}

func idxDe(t *testing.T) map[string]HuellaDestino {
	t.Helper()
	return map[string]HuellaDestino{
		"CXP-54A4281AE49A": {Pagadas: []DestinoComprobante{{DocumentoID: "d1", Consecutivo: "000487"}}},
		"CXP-12B8CF71477D": {Pagadas: []DestinoComprobante{{DocumentoID: "d2", Consecutivo: "001170", YaTiene: true}}},
		"CXPR-7K2M9P4XQ1BT": {EsRemesa: true, Pagadas: []DestinoComprobante{
			{DocumentoID: "d3"}, {DocumentoID: "d4"}, {DocumentoID: "d5"},
		}},
		"CXP-AAAAAAAAAAAA": {SinPagar: 1},
	}
}

func porNombre(as []ArchivoCruzado, nombre string) ArchivoCruzado {
	for _, a := range as {
		if a.Nombre == nombre {
			return a
		}
	}
	return ArchivoCruzado{Nombre: "(no está)"}
}

func TestCruzarComprobantes(t *testing.T) {
	nombres := []string{
		"FC 000487 CXP54A4281AE49A.pdf",
		"FC 001170 CXP12B8CF71477D.pdf",
		"REM 0043-07 CXPR7K2M9P4XQ1BT.pdf",
		"CXP-AAAAAAAAAAAA.pdf",
		"CXP-BBBBBBBBBBBB.pdf",
		"escaneo.pdf",
		"CXP-54A4281AE49A CXP-12B8CF71477D.pdf",
	}
	as := CruzarComprobantes(nombres, idxDe(t))

	quiere := map[string]string{
		"FC 000487 CXP54A4281AE49A.pdf":         ArchivoUnico,
		"FC 001170 CXP12B8CF71477D.pdf":         ArchivoUnico,
		"REM 0043-07 CXPR7K2M9P4XQ1BT.pdf":      ArchivoRemesa,
		"CXP-AAAAAAAAAAAA.pdf":                  ArchivoNoPagada,
		"CXP-BBBBBBBBBBBB.pdf":                  ArchivoNoEncontrado,
		"escaneo.pdf":                           ArchivoSinCodigo,
		"CXP-54A4281AE49A CXP-12B8CF71477D.pdf": ArchivoVariosCodigos,
	}
	for n, e := range quiere {
		if got := porNombre(as, n).Estado; got != e {
			t.Errorf("%q → %s, quería %s (motivo dado: %q)", n, got, e, porNombre(as, n).Motivo)
		}
	}

	// El recibo de la remesa cubre las TRES facturas, no una.
	if d := porNombre(as, "REM 0043-07 CXPR7K2M9P4XQ1BT.pdf").Destinos; len(d) != 3 {
		t.Errorf("el recibo de la remesa quedó en %d facturas, quería 3", len(d))
	}

	// «No encontrado» y «no pagada» NO pueden decir lo mismo: son dos problemas con dos soluciones.
	noPagada := porNombre(as, "CXP-AAAAAAAAAAAA.pdf").Motivo
	noExiste := porNombre(as, "CXP-BBBBBBBBBBBB.pdf").Motivo
	if noPagada == noExiste {
		t.Errorf("un pago sin marcar como pagado y un código inexistente dan el mismo motivo (%q); "+
			"quien lee la pantalla no sabe cuál de las dos cosas tiene que arreglar", noPagada)
	}

	r := ResumirCarpeta(as)
	if r.SeAdjuntan != 3 {
		t.Errorf("se adjuntan %d archivos, quería 3", r.SeAdjuntan)
	}
	// Tres archivos, CINCO facturas: la remesa cubre tres. Es el número que se compara contra lo
	// que falta, y por eso no puede ser el de archivos.
	if r.Facturas != 5 {
		t.Errorf("tocan %d facturas, quería 5 (la remesa cubre 3)", r.Facturas)
	}
	if r.Reemplazan != 1 {
		t.Errorf("reemplazan %d, quería 1", r.Reemplazan)
	}
	if r.Problemas != 4 {
		t.Errorf("problemas %d, quería 4", r.Problemas)
	}
}

// Windows bautiza la segunda descarga «… (1).pdf» y el código de adentro no cambia. Los dos
// archivos tienen el mismo derecho a ser el bueno: no se elige ninguno.
func TestDosArchivosParaElMismoPagoNoSeAdjuntanNinguno(t *testing.T) {
	as := CruzarComprobantes([]string{
		"FC 000487 CXP54A4281AE49A.pdf",
		"FC 000487 CXP54A4281AE49A (1).pdf",
	}, idxDe(t))
	for _, a := range as {
		if a.Estado != ArchivoDuplicado {
			t.Errorf("%q quedó %s; quedarse con uno de los dos es jugárselo a la suerte, y una vez "+
				"adjunto nadie vuelve a revisarlo", a.Nombre, a.Estado)
		}
	}
	if r := ResumirCarpeta(as); r.SeAdjuntan != 0 {
		t.Errorf("se iban a adjuntar %d archivos duplicados", r.SeAdjuntan)
	}
}

// Un archivo descartado por su NOMBRE no se vuelve a juzgar por la cartera: diría dos cosas
// distintas del mismo problema.
func TestUnArchivoSinCodigoNoSeMarcaDuplicado(t *testing.T) {
	as := CruzarComprobantes([]string{"escaneo.pdf", "escaneo.pdf"}, idxDe(t))
	for _, a := range as {
		if a.Estado != ArchivoSinCodigo {
			t.Errorf("%q quedó %s, quería %s", a.Nombre, a.Estado, ArchivoSinCodigo)
		}
	}
}

// El duplicado también aplica ENTRE una remesa y una factura suya: si alguien sube el recibo del
// pago agrupado y además uno suelto para una de sus facturas, no hay forma de saber cuál vale.
func TestLaRemesaYUnaDeSusFacturasTambienChocan(t *testing.T) {
	idx := idxDe(t)
	idx["CXP-D3D3D3D3D3D3"] = HuellaDestino{Pagadas: []DestinoComprobante{{DocumentoID: "d3"}}}
	as := CruzarComprobantes([]string{
		"REM 0043-07 CXPR7K2M9P4XQ1BT.pdf",
		"FC 000001 CXPD3D3D3D3D3D3.pdf",
	}, idx)
	for _, a := range as {
		if a.Estado != ArchivoDuplicado {
			t.Errorf("%q quedó %s: la factura d3 recibiría dos recibos distintos", a.Nombre, a.Estado)
		}
	}
}

// El motivo es lo único que el operador va a leer para arreglar su carpeta: no puede venir vacío.
func TestTodoProblemaExplicaPorQue(t *testing.T) {
	as := CruzarComprobantes([]string{
		"escaneo.pdf", "CXP-BBBBBBBBBBBB.pdf", "CXP-AAAAAAAAAAAA.pdf",
		"CXP-54A4281AE49A CXP-12B8CF71477D.pdf",
		"FC 000487 CXP54A4281AE49A.pdf", "FC 000487 CXP54A4281AE49A (1).pdf",
	}, idxDe(t))
	for _, a := range as {
		if a.SeAdjunta() {
			continue
		}
		if strings.TrimSpace(a.Motivo) == "" {
			t.Errorf("%q quedó en %s sin decir por qué", a.Nombre, a.Estado)
		}
	}
}
