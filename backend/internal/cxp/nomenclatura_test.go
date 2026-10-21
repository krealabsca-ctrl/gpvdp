package cxp

// Nomenclatura interna del pago (1-oct-2026): «A8 = comisiones», «A9 = planilla».
//
// Lo que estas pruebas protegen no es el guardado del código —eso es un UPDATE— sino las tres
// decisiones que lo hacen servir para algo:
//
//  1. que se HEREDE del concepto, porque con 124 clasificaciones bajo «Gastos» nadie va a cargar
//     124 códigos, y lo que hay que cargar a mano termina sin cargarse;
//  2. que la clasificación pise al concepto cuando necesita distinguirse;
//  3. que viaje al banco PRIMERO, y que un pago agrupado que mezcla rubros NO lleve ninguno.

import (
	"context"
	"strings"
	"testing"
)

func TestElCodigoSeHeredaDelConceptoYLaClasificacionLoPisa(t *testing.T) {
	casos := []struct {
		nombre, clasificacion, concepto, quiere string
	}{
		{"la clasificación no tiene: hereda del concepto", "", "A8", "A8"},
		{"la clasificación tiene el suyo: manda sobre el concepto", "A9", "A8", "A9"},
		{"ninguno tiene: sin código, y el pago sale igual", "", "", ""},
		{"solo la clasificación", "B12", "", "B12"},
		// Lo que escribe una persona no siempre viene limpio.
		{"minúsculas y espacios se normalizan", " a8 ", "", "A8"},
		{"el del concepto también se normaliza", "", "a9", "A9"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := CodigoDePago(c.clasificacion, c.concepto); got != c.quiere {
				t.Errorf("CodigoDePago(%q, %q) = %q, quiere %q", c.clasificacion, c.concepto, got, c.quiere)
			}
		})
	}
}

func TestLaFormaDelCodigoSeValida(t *testing.T) {
	buenos := []struct{ entra, queda string }{
		{"A8", "A8"}, {"A9", "A9"}, {"B12", "B12"}, {"PLA", "PLA"}, {"01", "01"}, {"ABCD", "ABCD"},
		{" a8 ", "A8"}, // se limpia
		{"", ""},       // vacío = quitar el código, es legítimo
		{"   ", ""},    // solo espacios, idem
	}
	for _, c := range buenos {
		got, err := ValidarCodigoPago(c.entra)
		if err != nil {
			t.Errorf("ValidarCodigoPago(%q) dio error: %v", c.entra, err)
		}
		if got != c.queda {
			t.Errorf("ValidarCodigoPago(%q) = %q, quiere %q", c.entra, got, c.queda)
		}
	}

	// Demasiado largo no entra: el código comparte el campo del banco con la referencia y el
	// código interno. Y los separadores romperían la lectura de la descripción.
	for _, malo := range []string{"ABCDE", "A-8", "A 8", "A8!", "ÑA", "A.8", "12345"} {
		if _, err := ValidarCodigoPago(malo); err == nil {
			t.Errorf("ValidarCodigoPago(%q) lo aceptó", malo)
		}
	}
}

// Lo que de verdad viaja al banco. Si el código no llega acá, el catálogo entero no sirve de nada.
func TestLaNomenclaturaViajaAlBancoAdelante(t *testing.T) {
	casos := []struct {
		nombre string
		row    PagoRow
		quiere string
	}{
		{
			"una factura con su rubro codificado",
			PagoRow{CodigoPago: "A8", Consecutivo: "00100001010000000487", Descripcion: "CXP-54A4281AE49A"},
			"A8 FC 000487 CXP54A4281AE49A",
		},
		{
			// Sin código el pago sale igual: la nomenclatura ayuda a leer, no es un requisito.
			"la misma factura sin código",
			PagoRow{Consecutivo: "00100001010000000487", Descripcion: "CXP-54A4281AE49A"},
			"FC 000487 CXP54A4281AE49A",
		},
		{
			"un pago agrupado, todas del mismo rubro",
			PagoRow{CodigoPago: "A9", Referencia: "REM 004307", Descripcion: "CXPR-7K2M9P4XQ1BT"},
			"A9 REM 004307 CXPR7K2M9P4XQ1BT",
		},
		{
			// El repositorio devuelve vacío cuando la remesa mezcla rubros: etiquetarla con el de
			// la primera diría que todo el débito es comisiones cuando la mitad es planilla.
			"un pago agrupado que mezcla rubros va sin código",
			PagoRow{Referencia: "REM 004307", Descripcion: "CXPR-7K2M9P4XQ1BT"},
			"REM 004307 CXPR7K2M9P4XQ1BT",
		},
		{
			"minúsculas: al banco va en mayúsculas, como vuelve",
			PagoRow{CodigoPago: "a8", Consecutivo: "1000", Descripcion: "CXP-0123456789AB"},
			"A8 FC 001000 CXP0123456789AB",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := descripcionDePago(c.row)
			if got != c.quiere {
				t.Errorf("descripción = %q, quiere %q", got, c.quiere)
			}
			// Y lo que importa de verdad: agregar la nomenclatura adelante NO puede romper la
			// lectura del código interno, de la que depende toda la conciliación.
			if c.row.Descripcion != "" {
				h, ok := extraerHuella(got)
				if !ok || h != c.row.Descripcion {
					t.Errorf("con la nomenclatura adelante ya no se reconoce el código interno: (%q, %v)", h, ok)
				}
			}
			// LO QUE SE EMITE TIENE QUE SOBREVIVIR AL CANAL (19-oct-2026). El banco borra los
			// guiones y conserva los espacios: si la descripción trae un carácter que no va a
			// volver, lo que el sistema guarda y lo que el estado de cuenta trae son dos cadenas
			// distintas, y copiar una para buscar la otra no encuentra nada.
			if got != ParaElBanco(got) {
				t.Errorf("la descripción lleva algo que el banco va a borrar: emitido %q, vuelve %q",
					got, ParaElBanco(got))
			}
		})
	}
}

// El código se guarda NORMALIZADO. Si llegara «a8» a la base, lo emitido y lo que devuelve el
// banco (que viene en mayúsculas) no se verían iguales y nadie podría cruzarlos a ojo.
func TestElServicioGuardaElCodigoNormalizado(t *testing.T) {
	repo := &fakeRepo{}
	err := servicioHuella(repo).AsignarCodigoPago(context.Background(), "e1", NivelConcepto, "con-1", "  a8 ", "u1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if repo.capCodigo != "A8" {
		t.Errorf("se guardó %q, quiere A8", repo.capCodigo)
	}

	t.Run("un nivel desconocido no escribe nada", func(t *testing.T) {
		repo := &fakeRepo{}
		// `nivel` elige la TABLA. Un valor que no se entiende no puede caer en un default.
		if err := servicioHuella(repo).AsignarCodigoPago(context.Background(), "e1", "PROVEEDOR", "x", "A8", "u1"); err == nil {
			t.Error("aceptó un nivel desconocido")
		}
		if repo.capCodigo != "" {
			t.Errorf("escribió %q pese al nivel inválido", repo.capCodigo)
		}
	})

	t.Run("una forma inválida no llega al repositorio", func(t *testing.T) {
		repo := &fakeRepo{}
		if err := servicioHuella(repo).AsignarCodigoPago(context.Background(), "e1", NivelConcepto, "con-1", "A-8", "u1"); err == nil {
			t.Error("aceptó un código con guion")
		}
		if repo.capCodigo != "" {
			t.Errorf("escribió %q pese a la forma inválida", repo.capCodigo)
		}
	})
}

func TestElMensajeDelCodigoInvalidoDiceComoEs(t *testing.T) {
	_, err := ValidarCodigoPago("A-8")
	if err == nil {
		t.Fatal("no rechazó el código")
	}
	// El mensaje tiene que traer un ejemplo: «forma inválida» no le dice a nadie qué escribir.
	for _, q := range []string{"1 a 4", "A8"} {
		if !strings.Contains(err.Error(), q) {
			t.Errorf("el mensaje no dice %q: %s", q, err.Error())
		}
	}
}
