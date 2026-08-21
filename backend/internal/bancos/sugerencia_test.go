package bancos

import "testing"

func TestExtraerPalabraClave(t *testing.T) {
	casos := []struct {
		desc   string
		quiere string
	}{
		// proveedor tras vocabulario bancario genérico
		{"TEF DE COMAPAN SA 84512", "COMAPAN"},
		{"PAGO FACTURA COOPEPROFA R.L.", "COOPEPROFA"},
		{"COMPRA WALMART ESCAZU", "WALMART"},
		{"TRANSFERENCIA SINPE MOVIL FLORISTERIA LILIANA", "FLORISTERIA"},
		// acentos y minúsculas se normalizan para comparar, se devuelve la grafía original
		{"pago Panadería Móvil", "Panadería"},
		// solo códigos y genéricos: sin candidato
		{"SINPE MOVIL 8899-1234", ""},
		{"TEF 004512 REF 99", ""},
		{"", ""},
		// meses no cuentan como palabra clave
		{"PAGO PLANILLA JUN 2026", "PLANILLA"},
		// tokens cortos y alfanuméricos se saltan; gana el primer candidato válido
		{"ND AB12 KIA MOTORS", "KIA"},
	}
	for _, c := range casos {
		if got := ExtraerPalabraClave(c.desc); got != c.quiere {
			t.Errorf("ExtraerPalabraClave(%q) = %q, quiere %q", c.desc, got, c.quiere)
		}
	}
}

func TestSoloActivas(t *testing.T) {
	reglas := []Regla{
		{ID: "a", Activo: true},
		{ID: "b", Activo: false},
		{ID: "c", Activo: true},
	}
	activas := soloActivas(reglas)
	if len(activas) != 2 || activas[0].ID != "a" || activas[1].ID != "c" {
		t.Fatalf("soloActivas devolvió %+v, quiere solo a y c", activas)
	}
}
