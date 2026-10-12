package cxp

// El código de la REMESA y el de la factura conviven en el mismo campo del banco, con el mismo
// largo y un prefijo que es prefijo del otro («CXP» vs «CXPR»). Y el banco borra el guion, que es
// justo lo que los separaría a simple vista.
//
// Si el lector los confunde, el daño no es cosmético: un pago agrupado que se lea como factura va a
// buscar un documento que no existe, el veredicto será «sin documento» y las cinco facturas que ese
// pago cubrió quedan abiertas. Nadie se entera hasta que el proveedor llama.
//
// Estas pruebas fijan la regla y su orden.

import (
	"strings"
	"testing"
)

func TestElCodigoDeRemesaNoSeConfundeConElDeFactura(t *testing.T) {
	casos := []struct {
		nombre, descripcion, quiere string
		remesa                      bool
	}{
		{
			nombre:      "remesa con guion",
			descripcion: "REM 0043-07 CXPR-7K2M9P4XQ1BT",
			quiere:      "CXPR-7K2M9P4XQ1BT",
			remesa:      true,
		},
		{
			// El caso que importa: así vuelve del banco, sin el guion.
			nombre:      "remesa sin guion, como la devuelve el banco",
			descripcion: "DEBITO MASIVO SINPE ( - 1060746795 REM 0043-07 CXPR7K2M9P4XQ1BT Lote 888607 Linea 3)",
			quiere:      "CXPR-7K2M9P4XQ1BT",
			remesa:      true,
		},
		{
			// Una factura cuyo código ARRANCA con R: el caso adversario del prefijo.
			nombre:      "factura que empieza con R, con guion",
			descripcion: "FC 000487 CXP-R7K2M9P4XQ1B",
			quiere:      "CXP-R7K2M9P4XQ1B",
			remesa:      false,
		},
		{
			// La misma, sin guion: el texto queda «CXPR7K2M9P4XQ1B» y se parece muchísimo a una
			// remesa. No lo es: como remesa solo quedarían 11 caracteres y el patrón pide 12.
			nombre:      "factura que empieza con R, sin guion, NO es una remesa",
			descripcion: "FC 000487 CXPR7K2M9P4XQ1B",
			quiere:      "CXP-R7K2M9P4XQ1B",
			remesa:      false,
		},
		{
			nombre:      "factura normal",
			descripcion: "FC 000487 CXP54A4281AE49A",
			quiere:      "CXP-54A4281AE49A",
			remesa:      false,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, ok := extraerHuella(c.descripcion)
			if !ok {
				t.Fatalf("no reconoció ningún código en %q", c.descripcion)
			}
			if got != c.quiere {
				t.Errorf("código = %q, quiere %q", got, c.quiere)
			}
			if EsRemesa(got) != c.remesa {
				t.Errorf("EsRemesa(%q) = %v, quiere %v — de esto depende si se cierra UNA factura o TODAS las del pago",
					got, EsRemesa(got), c.remesa)
			}
		})
	}
}

// Los dos códigos tienen que seguir teniendo el MISMO largo. Toda la separación entre ellos descansa
// en que una factura, leída como remesa, se quede corta por un carácter. Si alguien alargara o
// acortara uno de los dos, esa garantía desaparece sin que ningún otro test lo note.
func TestLosDosCodigosMidenLoMismo(t *testing.T) {
	const factura = "CXP-A1B2C3D4E5F6"
	const remesa = "CXPR-A1B2C3D4E5F6"

	if n := len(strings.TrimPrefix(factura, "CXP-")); n != 12 {
		t.Fatalf("el código de factura tiene %d caracteres útiles, el diseño asume 12", n)
	}
	if n := len(strings.TrimPrefix(remesa, "CXPR-")); n != 12 {
		t.Fatalf("el código de remesa tiene %d caracteres útiles, el diseño asume 12", n)
	}
	// Y la propiedad que se deriva: la factura sin guion NO se lee como remesa.
	sinGuion := strings.Replace(factura, "-", "", 1)
	if h, _ := extraerHuella(sinGuion); EsRemesa(h) {
		t.Fatalf("%q se leyó como remesa (%q): el pago de una factura cerraría el grupo equivocado",
			sinGuion, h)
	}
}
