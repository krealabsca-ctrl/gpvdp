package shared

import "testing"

// Los casos con nombre propio salen del catálogo REAL de proveedores (649 activos, 62 con tilde o
// eñe): no son inventados, son los que hoy viajarían crudos al banco.
func TestTextoParaBanco(t *testing.T) {
	casos := []struct{ entrada, quiero, porque string }{
		{"PETRÓLEOS DELTA S.A.", "PETROLEOS DELTA S.A.", "la tilde se translitera; el punto de «S.A.» se conserva"},
		{"AGRO UJARRÁS S.A.", "AGRO UJARRAS S.A.", "tilde en mayúscula"},
		{"SUPERMERCADO CADENA SANCARLEÑA DEL NORTE S.A", "SUPERMERCADO CADENA SANCARLENA DEL NORTE S.A", "la eñe"},
		{"Litografía e Imprenta Cora S.A.", "Litografia e Imprenta Cora S.A.", "se conserva la caja original"},
		{"CORPORACIÓN MEGASUPER, S.A.", "CORPORACION MEGASUPER S.A.", "la COMA se va: es el separador de la macro"},
		{
			"DISTRIBUIDORA X; S.A.", "DISTRIBUIDORA X S.A.",
			"el PUNTO Y COMA se va: partiría la línea en dos campos y el banco rechaza el archivo",
		},
		{
			"Asociación Solidarista Grupo Comeca & Afines", "Asociacion Solidarista Grupo Comeca Afines",
			"el ampersand se va y no deja doble espacio",
		},
		{"TRANSPORTES (CR) LIMITADA", "TRANSPORTES CR LIMITADA", "los paréntesis se van"},
		{"DANIEL ESTEBAN HERNÁNDEZ STERLING", "DANIEL ESTEBAN HERNANDEZ STERLING", "nombre de persona física"},
		{"FERRETERIA EL CLAVO", "FERRETERIA EL CLAVO", "lo que ya está limpio no se toca"},
		{"  espacios   de   sobra  ", "espacios de sobra", "se colapsan y se recortan"},
		{"", "", "vacío"},
		{",,,;;;", "", "solo separadores: queda vacío, no una línea de basura"},
		{"MAÑANA-TARDE", "MANANA-TARDE", "el guion se conserva"},
		{"CAFÉ ÑU 100% ARÁBICA", "CAFE NU 100 ARABICA", "el símbolo de porcentaje también se va"},
		{"COMILLAS \"RARAS\"", "COMILLAS RARAS", "las comillas romperían un CSV"},
		{"salto\nde\tlínea", "salto de linea", "los controles se vuelven espacio"},
	}
	for _, c := range casos {
		if got := TextoParaBanco(c.entrada); got != c.quiero {
			t.Errorf("TextoParaBanco(%q) = %q, quería %q — %s", c.entrada, got, c.quiero, c.porque)
		}
	}
}

// Lo que sale NO puede tener nada fuera del set seguro. Este test es la red: si mañana alguien
// agrega un carácter al `switch` sin pensarlo, se cae acá.
func TestTextoParaBancoNoDejaNadaRaro(t *testing.T) {
	entradas := []string{
		"PETRÓLEOS DELTA, S.A.; & (CR) 100% ÑOÑO \"x\"",
		"Ámbito Público – guion largo — y comillas «tipográficas»",
		"tabs\ty\nsaltos\r\n",
	}
	for _, e := range entradas {
		got := TextoParaBanco(e)
		for _, r := range got {
			ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
				r == ' ' || r == '.' || r == '-'
			if !ok {
				t.Errorf("TextoParaBanco(%q) = %q: dejó pasar %q (U+%04X)", e, got, r, r)
			}
		}
		if len(got) > 0 && (got[0] == ' ' || got[len(got)-1] == ' ') {
			t.Errorf("TextoParaBanco(%q) = %q: quedó con espacio en un extremo", e, got)
		}
	}
}
