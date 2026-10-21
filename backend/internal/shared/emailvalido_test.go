package shared

// El «Para» editable al reenviar un comprobante (19-oct-2026) deja escribir una dirección a mano,
// y una dirección mal escrita tiene que decirse ACÁ. Si se dejara fallar al servidor de correo, el
// error llega como «error del servidor» y manda a revisar la configuración SMTP cuando lo que pasó
// es que alguien se comió una letra.

import "testing"

func TestEmailValido(t *testing.T) {
	t.Parallel()
	validos := []string{
		"info@wisomti.com",
		"contabilidad@valledepazcr.com",
		// El «+» de las etiquetas es legítimo y un validador casero suele rechazarlo, lo que
		// obliga a la gente a buscar otra vía.
		"pagos+vdp@empresa.co.cr",
		"a@b.cr",
		"nombre.apellido@sub.dominio.com",
	}
	for _, e := range validos {
		if !EmailValido(e) {
			t.Errorf("EmailValido(%q) = false, es una dirección legítima", e)
		}
	}

	invalidos := []string{
		"",
		"   ",
		"sin-arroba.com",
		"@sindominio.com",
		"dos@@arrobas.com",
		"con espacio@dominio.com",
		// La forma con nombre se rechaza a propósito: acá se escribe UNA dirección, y aceptar el
		// formato largo dejaría pasar texto que después se ve raro en la bitácora.
		"Juan <juan@x.com>",
		// Varias direcciones en un campo que manda a una sola.
		"uno@x.com, dos@y.com",
		"uno@x.com; dos@y.com",
	}
	for _, e := range invalidos {
		if EmailValido(e) {
			t.Errorf("EmailValido(%q) = true, no sirve como destinatario", e)
		}
	}
}

// Los espacios de más al pegar no invalidan una dirección buena: es lo que pasa copiando de un
// correo o de un Excel.
func TestEmailValidoToleraEspaciosAlPegar(t *testing.T) {
	t.Parallel()
	if !EmailValido("  info@wisomti.com  ") {
		t.Error("una dirección pegada con espacios alrededor es válida")
	}
}
