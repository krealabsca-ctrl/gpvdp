package bancos

// Tests del diccionario del catálogo. Lo que importa: que lea lo que la gente escribe en Excel,
// que no adivine cuando falta el destino, y que importar dos veces no duplique nada.

import (
	"context"
	"errors"
	"testing"
)

func gridDicc(filas ...[]string) Grid {
	g := Grid{{"Concepto", "Clasificación", "Visible en CxP", "Palabras clave", "Aplica a", "Prioridad"}}
	return append(g, filas...)
}

func TestLeerDiccionario(t *testing.T) {
	g := gridDicc(
		[]string{"Gastos", "Electricidad", "Sí", "ICE; CNFL", "Débito", "120"},
		[]string{"Ingresos", "Planes", "", "linea sinpe (smo-", "crédito", ""},
		[]string{"Gastos", "Agua", "no", "", "", ""},
		[]string{"", "", "", "", "", ""}, // fila en blanco: se ignora
	)
	filas, err := LeerDiccionario(g)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if len(filas) != 3 {
		t.Fatalf("filas = %d, quiere 3 (la vacía se ignora): %+v", len(filas), filas)
	}

	t.Run("parte las palabras por punto y coma", func(t *testing.T) {
		if len(filas[0].Palabras) != 2 || filas[0].Palabras[0] != "ICE" || filas[0].Palabras[1] != "CNFL" {
			t.Errorf("palabras = %+v", filas[0].Palabras)
		}
	})

	t.Run("normaliza aplica_a en cualquier grafía", func(t *testing.T) {
		if filas[0].AplicaA != "DEBITO" || filas[1].AplicaA != "CREDITO" {
			t.Errorf("aplica_a = %s / %s", filas[0].AplicaA, filas[1].AplicaA)
		}
		if filas[2].AplicaA != "MIXTO" {
			t.Errorf("sin aplica_a debería quedar MIXTO, quedó %s", filas[2].AplicaA)
		}
	})

	t.Run("lee visible en CxP y la prioridad", func(t *testing.T) {
		if !filas[0].VisibleCxP || filas[2].VisibleCxP {
			t.Errorf("visible_cxp = %v / %v", filas[0].VisibleCxP, filas[2].VisibleCxP)
		}
		if filas[0].Prioridad != 120 {
			t.Errorf("prioridad = %d, quiere 120", filas[0].Prioridad)
		}
		if filas[1].Prioridad != prioridadDiccionario {
			t.Errorf("sin prioridad debería usar el default %d, usó %d", prioridadDiccionario, filas[1].Prioridad)
		}
	})

	t.Run("conserva el número de línea del archivo", func(t *testing.T) {
		if filas[0].Linea != 2 {
			t.Errorf("línea = %d, quiere 2 (encabezado en la 1)", filas[0].Linea)
		}
	})
}

func TestLeerDiccionarioNoAdivina(t *testing.T) {
	t.Run("palabras clave sin clasificación se omiten", func(t *testing.T) {
		// Una regla asigna Concepto Y Clasificación: sin destino completo no se inventa.
		filas, err := LeerDiccionario(gridDicc([]string{"Gastos", "", "", "ICE", "", ""}))
		if err != nil {
			t.Fatalf("leer: %v", err)
		}
		if filas[0].Problema == "" {
			t.Error("debería marcar el problema en vez de crear una regla a medias")
		}
	})

	t.Run("palabras de menos de 3 letras se descartan", func(t *testing.T) {
		filas, _ := LeerDiccionario(gridDicc([]string{"Gastos", "Agua", "", "AyA; xy; a", "", ""}))
		if len(filas[0].Palabras) != 1 || filas[0].Palabras[0] != "AyA" {
			t.Errorf("palabras = %+v, quiere solo «AyA»", filas[0].Palabras)
		}
	})

	t.Run("sin encabezado no procesa nada", func(t *testing.T) {
		g := Grid{{"cosa", "otra"}, {"x", "y"}}
		if _, err := LeerDiccionario(g); !errors.Is(err, ErrDiccionarioSinEncabezado) {
			t.Errorf("err = %v, quiere ErrDiccionarioSinEncabezado", err)
		}
	})

	t.Run("encabezado sin filas útiles", func(t *testing.T) {
		if _, err := LeerDiccionario(gridDicc()); !errors.Is(err, ErrDiccionarioVacio) {
			t.Errorf("err = %v, quiere ErrDiccionarioVacio", err)
		}
	})

	t.Run("encuentra el encabezado aunque no esté en la primera fila", func(t *testing.T) {
		g := Grid{
			{"Diccionario del catálogo"}, {""},
			{"Concepto", "Clasificación", "", "Palabras clave"},
			{"Gastos", "Agua", "", "AyA"},
		}
		filas, err := LeerDiccionario(g)
		if err != nil {
			t.Fatalf("leer: %v", err)
		}
		if len(filas) != 1 || filas[0].Concepto != "Gastos" {
			t.Errorf("filas = %+v", filas)
		}
	})
}

func TestClaveReglaIgnoraElOrden(t *testing.T) {
	// Dos filas con las mismas palabras en otro orden son LA MISMA regla: si no, reimportar
	// duplicaría reglas.
	a := claveRegla("Gastos", "Electricidad", "DEBITO", []string{"ICE", "CNFL"})
	b := claveRegla("gastos", "electricidad", "DEBITO", []string{"cnfl", "ice"})
	if a != b {
		t.Errorf("las claves deberían coincidir:\n%s\n%s", a, b)
	}
	if claveRegla("Gastos", "Electricidad", "CREDITO", []string{"ICE"}) == claveRegla("Gastos", "Electricidad", "DEBITO", []string{"ICE"}) {
		t.Error("distinto aplica_a debería dar distinta regla")
	}
}

func TestImportarDiccionarioPreview(t *testing.T) {
	ctx := context.Background()
	// Catálogo actual: Gastos › Agua ya existe, con una regla «AyA».
	repo := &fakeRepo{
		conceptosCat: []Concepto{{ID: "con-g", Nombre: "Gastos"}},
		clasifsCat:   []ClasificacionItem{{ID: "cl-agua", ConceptoID: "con-g", Concepto: "Gastos", Nombre: "Agua"}},
		reglasCat: []Regla{{ID: "r1", AplicaA: "DEBITO", ConceptoID: "con-g",
			ClasificacionID: "cl-agua", Palabras: []string{"AyA"}}},
	}
	archivo := gridDicc(
		[]string{"Gastos", "Agua", "", "AyA", "Débito", ""},          // ya existe todo
		[]string{"Gastos", "Electricidad", "", "ICE; CNFL", "D", ""}, // clasificación + regla nuevas
		[]string{"Ingresos", "Planes", "", "smo-", "C", ""},          // concepto + clasif + regla nuevas
		[]string{"", "", "", "ICE", "", ""},                          // omitida: sin concepto
	)
	filas, err := LeerDiccionario(archivo)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	svc := servicioSaldos(repo)

	plan, err := svc.aplicarDiccionario(ctx, "e1", filas, false, "u1")
	if err != nil {
		t.Fatalf("previsualizar: %v", err)
	}
	if plan.Aplicado {
		t.Error("la previsualización no debería marcarse como aplicada")
	}
	if plan.ConceptosNuevos != 1 || plan.ClasificacionesNuevas != 2 || plan.ReglasNuevas != 2 {
		t.Errorf("plan: %d conceptos, %d clasificaciones, %d reglas nuevas",
			plan.ConceptosNuevos, plan.ClasificacionesNuevas, plan.ReglasNuevas)
	}
	if plan.SinCambios != 1 {
		t.Errorf("sin cambios = %d, quiere 1 (Gastos › Agua con su regla ya existía)", plan.SinCambios)
	}
	if plan.Omitidas != 1 {
		t.Errorf("omitidas = %d, quiere 1 (la fila sin concepto)", plan.Omitidas)
	}
	if repo.conceptosCreados != 0 || repo.clasifsCreadas != 0 || repo.reglasCreadas != 0 {
		t.Errorf("la previsualización NO debe escribir: %d conceptos, %d clasif, %d reglas",
			repo.conceptosCreados, repo.clasifsCreadas, repo.reglasCreadas)
	}
}

func TestImportarDiccionarioAplica(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		conceptosCat: []Concepto{{ID: "con-g", Nombre: "Gastos"}},
		clasifsCat:   []ClasificacionItem{{ID: "cl-agua", ConceptoID: "con-g", Concepto: "Gastos", Nombre: "Agua"}},
	}
	svc := servicioSaldos(repo)
	filas, err := LeerDiccionario(gridDicc(
		[]string{"Gastos", "Electricidad", "", "ICE", "Débito", "120"},
		[]string{"gastos", "agua", "", "", "", ""}, // minúsculas: es el mismo que ya existe
	))
	if err != nil {
		t.Fatalf("leer: %v", err)
	}

	plan, err := svc.aplicarDiccionario(ctx, "e1", filas, true, "u1")
	if err != nil {
		t.Fatalf("aplicar: %v", err)
	}
	if !plan.Aplicado {
		t.Error("debería marcarse como aplicado")
	}
	if repo.clasifsCreadas != 1 || repo.reglasCreadas != 1 || repo.conceptosCreados != 0 {
		t.Errorf("escrituras: %d conceptos, %d clasif, %d reglas (quiere 0/1/1)",
			repo.conceptosCreados, repo.clasifsCreadas, repo.reglasCreadas)
	}
	if plan.SinCambios != 1 {
		t.Errorf("«gastos › agua» en minúsculas debería reconocerse como existente; sin cambios = %d", plan.SinCambios)
	}
	if repo.ultimaRegla.Prioridad != 120 || repo.ultimaRegla.AplicaA != "DEBITO" {
		t.Errorf("la regla debería respetar prioridad y aplica_a del archivo: %+v", repo.ultimaRegla)
	}
}
