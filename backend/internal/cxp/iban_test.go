package cxp

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestValidarIBANCR(t *testing.T) {
	casos := []struct {
		nombre, entra, normalizado string
		valido                     bool
	}{
		// Cómo lo pega la gente desde el estado de cuenta del banco.
		{"con espacios cada 4", "CR21 0151 0001 0026 2841 12", "CR21015100010026284112", true},
		{"con guiones", "CR21-0151-0001-0026-2841-12", "CR21015100010026284112", true},
		{"en minúsculas", "cr21015100010026284112", "CR21015100010026284112", true},
		{"limpio", "CR21015100010026284112", "CR21015100010026284112", true},
		// Los errores que de verdad ocurren al cargar 648 filas a mano.
		{"sin el prefijo CR", "21015100010026284112", "21015100010026284112", false},
		{"le falta un dígito", "CR2101510001002628411", "CR2101510001002628411", false},
		{"tiene uno de más", "CR210151000100262841123", "CR210151000100262841123", false},
		{"celda vacía", "", "", false},
		{"solo espacios", "   ", "", false},
		{"letras en el número", "CR2101510001002628411X", "CR2101510001002628411X", false},
		{"es otro país", "ES9121000418450200051332", "ES9121000418450200051332", false},
	}
	for _, c := range casos {
		got, ok := ValidarIBANCR(c.entra)
		if ok != c.valido {
			t.Errorf("%s: ValidarIBANCR(%q) válido = %v, quiere %v", c.nombre, c.entra, ok, c.valido)
		}
		if got != c.normalizado {
			t.Errorf("%s: normalizado = %q, quiere %q", c.nombre, got, c.normalizado)
		}
	}
}

func TestPrevisualizarIBANClasificaCadaFila(t *testing.T) {
	repo := &fakeRepo{provsPorCedula: map[string]ProveedorIBAN{
		"3101402954": {ID: "p1", Nombre: "AMERICAN DATA NETWORKS S.A.", IBAN: ""},
		"402310892":  {ID: "p2", Nombre: "FRANCELA FALLAS VARGAS", IBAN: "CR21015100010026284112"},
	}}
	svc := NewService(repo, nil, zap.NewNop())

	res, err := svc.PrevisualizarIBAN(context.Background(), "emp", []FilaIBAN{
		{Fila: 2, Identificacion: "3-101-402954", IBAN: "CR21 0151 0001 0026 2841 13"}, // se carga
		{Fila: 3, Identificacion: "402310892", IBAN: "CR21015100010026284112"},         // ya lo tenía
		{Fila: 4, Identificacion: "999999999", IBAN: "CR21015100010026284114"},         // no existe
		{Fila: 5, Identificacion: "3101402954", IBAN: "CR21015100010026284115"},        // repetida
		{Fila: 6, Identificacion: "402310892", IBAN: "123"},                            // inválido
		{Fila: 7, Identificacion: "", IBAN: "CR21015100010026284116"},                  // sin cédula
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.ACargar != 1 || res.SinCambio != 1 || res.NoHallados != 1 || res.Duplicados != 1 || res.Invalidos != 2 {
		t.Errorf("resumen inesperado: %+v", res)
	}
	// La fila que se carga tiene que traer el nombre del proveedor y el IBAN normalizado.
	f := res.Filas[0]
	if f.Estado != IBANOK || f.ProveedorID != "p1" || f.IBAN != "CR21015100010026284113" {
		t.Errorf("fila 2: %+v", f)
	}
	if f.Nombre != "AMERICAN DATA NETWORKS S.A." {
		t.Errorf("la previsualización tiene que decir de QUIÉN es la cuenta: %+v", f)
	}
	// La que reemplaza un IBAN existente debe mostrar el anterior, para no cambiarlo a ciegas.
	// (Acá la fila 3 tenía el mismo, así que va como SIN_CAMBIO con su anterior visible.)
	if res.Filas[1].Estado != IBANIgual || res.Filas[1].IBANAnterior == "" {
		t.Errorf("fila 3: %+v", res.Filas[1])
	}
}

func TestCargarIBANSoloEscribeLasValidas(t *testing.T) {
	repo := &fakeRepo{provsPorCedula: map[string]ProveedorIBAN{
		"3101402954": {ID: "p1", Nombre: "Proveedor Uno", IBAN: ""},
	}}
	svc := NewService(repo, nil, zap.NewNop())

	n, err := svc.CargarIBAN(context.Background(), "emp", []FilaIBAN{
		{Fila: 2, Identificacion: "3101402954", IBAN: "CR21 0151 0001 0026 2841 12"},
		{Fila: 3, Identificacion: "999999999", IBAN: "CR21015100010026284113"}, // no existe: se ignora
		{Fila: 4, Identificacion: "3101402954", IBAN: "no es un iban"},         // inválido: se ignora
	}, "u1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if n != 1 {
		t.Errorf("actualizados = %d, quiere 1", n)
	}
	// Y lo que se guardó es el IBAN NORMALIZADO, no el texto con espacios del Excel.
	if got := repo.ibanGuardado["p1"]; got != "CR21015100010026284112" {
		t.Errorf("se guardó %q", got)
	}
	if len(repo.ibanGuardado) != 1 {
		t.Errorf("no debería haber escrito nada más: %v", repo.ibanGuardado)
	}
}

// El guardarraíl: la macro no puede salir con líneas que el banco va a rechazar.
func TestFaltanIBAN(t *testing.T) {
	rows := []PagoRow{
		{Nombre: "Con cuenta", IBAN: "CR21015100010026284112"},
		{Nombre: "Sin cuenta", IBAN: ""},
		{Nombre: "Cuenta mal cargada", IBAN: "0151-0001-0026"},
	}
	sin := FaltanIBAN(rows)
	if len(sin) != 2 {
		t.Fatalf("faltantes = %d, quiere 2: %+v", len(sin), sin)
	}
	if sin[0].Nombre != "Sin cuenta" || sin[1].Nombre != "Cuenta mal cargada" {
		t.Errorf("tiene que decir QUIÉNES son: %+v", sin)
	}
}
