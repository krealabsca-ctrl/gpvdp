package precios

// Pruebas del importador de planes y recetas.
//
// El escenario reproduce la forma de la matriz real: dos bloques de columnas con los mismos
// nombres de plan —uno de CONSUMO y otro de COSTEO— y una fila de precios abajo. Esa duplicación
// es lo que más fácil se lee mal, y por eso se prueba en vez de confiar en la inspección.

import (
	"testing"
)

// matrizDeCosteo arma una hoja con la forma del archivo real.
func matrizDeCosteo(t *testing.T) []byte {
	t.Helper()
	return libro(t, "Costos", [][]string{
		// fila 1: vacía
		{},
		// fila 2: los rótulos de bloque. CONSUMO en F (índice 5), COSTEO en K (índice 10).
		{"", "", "", "", "", "CONSUMO", "", "", "", "", "COSTEO POR SERVICIO"},
		// fila 3: vacía
		{},
		// fila 4: familias
		{"", "", "", "", "", "SERVICIOS FUNERARIOS", "", "", "CORPORATIVO", "", "SERVICIOS FUNERARIOS", "", "", "CORPORATIVO"},
		// fila 5: el encabezado de planes, en los DOS bloques
		{"PRODUCTOS", "COSTO", "Precio Incluye IVA", "Proveedor", "",
			"Jade", "Oro", "", "Gardenia", "",
			"Jade", "Oro", "", "Gardenia"},
		// fila 6: vacía
		{},
		// filas 7+: insumos con su costo y las cantidades por plan
		{"Cofre #24", "135000", "", "", "", "1", "1", "", "1"},
		{"Arreglo Floral", "5000", "", "", "", "2", "6", "", "4"},
		{"Solo para Oro", "80000", "", "", "", "", "1", "", ""},
		{"Sin costo conocido", "", "", "Averiguar costos", "", "1", "1", "", "1"},
		// fila 11: la de precios
		{"PRECIO DE VENTA", "", "", "", "", "", "", "", "", "",
			"500000", "1350000", "", "4950"},
	})
}

func TestLaMatrizDaLosPlanesConSuPrecioYModalidad(t *testing.T) {
	t.Parallel()
	a, err := AnalizarRecetas(matrizDeCosteo(t), "Costos")
	if err != nil {
		t.Fatalf("AnalizarRecetas: %v", err)
	}
	if a.FilaEncabezado != 5 {
		t.Errorf("fila de encabezado = %d, se esperaba 5", a.FilaEncabezado)
	}
	if len(a.Planes) != 3 {
		t.Fatalf("planes = %d, se esperaban 3 (Jade, Oro, Gardenia): %+v", len(a.Planes), a.Planes)
	}

	porCodigo := map[string]PlanDetectado{}
	for _, p := range a.Planes {
		porCodigo[p.Codigo] = p
	}

	t.Run("un precio de venta alto es modalidad DIRECTO", func(t *testing.T) {
		j := porCodigo["JADE"]
		if j.Precio == nil || *j.Precio != "500000.00" {
			t.Errorf("precio de Jade = %v, se esperaba 500000.00", j.Precio)
		}
		if j.Modalidad != ModalidadDirecto {
			t.Errorf("modalidad de Jade = %q, se esperaba DIRECTO", j.Modalidad)
		}
		if j.Familia != "SERVICIOS FUNERARIOS" {
			t.Errorf("familia de Jade = %q", j.Familia)
		}
	})

	// El número que separa un precio de una cuota es la diferencia entre comparar el costo contra
	// una venta o contra lo que el contrato paga en toda su vida. Leerlo mal da un plan que
	// aparenta perder millones cada mes.
	t.Run("un monto chico es una CUOTA, no un precio", func(t *testing.T) {
		g := porCodigo["GARDENIA"]
		if g.Precio == nil || *g.Precio != "4950.00" {
			t.Errorf("cuota de Gardenia = %v, se esperaba 4950.00", g.Precio)
		}
		if g.Modalidad != ModalidadSuscripcion {
			t.Errorf("modalidad de Gardenia = %q, se esperaba SUSCRIPCION", g.Modalidad)
		}
	})

	// El segundo bloque repite los nombres de plan. Sin el corte, cada plan entraría dos veces y
	// las cantidades se leerían de las columnas equivocadas.
	t.Run("el bloque de COSTEO no duplica los planes", func(t *testing.T) {
		vistos := map[string]int{}
		for _, p := range a.Planes {
			vistos[p.Codigo]++
		}
		for cod, n := range vistos {
			if n > 1 {
				t.Errorf("%s aparece %d veces: se está leyendo el bloque de costeo como si fueran planes", cod, n)
			}
		}
	})
}

func TestLasCeldasDeLaMatrizSeVuelvenLineasDeReceta(t *testing.T) {
	t.Parallel()
	p, err := PlanificarRecetas(matrizDeCosteo(t), "Costos", nil)
	if err != nil {
		t.Fatalf("PlanificarRecetas: %v", err)
	}

	// Jade: cofre, flores y el sin costo = 3. Oro: esos tres más el suyo = 4. Gardenia: 3.
	porPlan := map[string]int{}
	for _, l := range p.Lineas {
		porPlan[l.PlanCodigo]++
	}
	if porPlan["JADE"] != 3 || porPlan["ORO"] != 4 || porPlan["GARDENIA"] != 3 {
		t.Errorf("líneas por plan = %v, se esperaban Jade 3, Oro 4, Gardenia 3", porPlan)
	}

	t.Run("una celda vacía NO es una línea de receta", func(t *testing.T) {
		for _, l := range p.Lineas {
			if l.PlanCodigo == "JADE" && l.InsumoNombre == "Solo para Oro" {
				t.Error("Jade no lleva ese insumo: una celda vacía se leyó como cantidad")
			}
		}
	})

	t.Run("la cantidad se conserva tal cual", func(t *testing.T) {
		for _, l := range p.Lineas {
			if l.PlanCodigo == "ORO" && l.InsumoNombre == "Arreglo Floral" && l.Cantidad != "6" {
				t.Errorf("cantidad = %q, se esperaba 6", l.Cantidad)
			}
		}
	})

	t.Run("el insumo sin costo entra igual, sin inventarle cero", func(t *testing.T) {
		var sinCosto *InsumoAImportar
		for i := range p.Insumos {
			if p.Insumos[i].Nombre == "Sin costo conocido" {
				sinCosto = &p.Insumos[i]
			}
		}
		if sinCosto == nil {
			t.Fatal("el insumo sin costo no entró al plan de importación")
		}
		if sinCosto.Costo != nil {
			t.Errorf("se le inventó un costo: %v — tiene que entrar sin costo y quedar contado aparte", sinCosto.Costo)
		}
	})

	t.Run("cada insumo aparece UNA sola vez aunque esté en varios planes", func(t *testing.T) {
		vistos := map[string]int{}
		for _, i := range p.Insumos {
			vistos[i.Codigo]++
		}
		for cod, n := range vistos {
			if n > 1 {
				t.Errorf("%s está %d veces en el plan de insumos", cod, n)
			}
		}
	})
}

// La modalidad que confirma el usuario manda sobre la inferida por monto: el umbral es una
// heurística, y hay planes cuyo precio podría caer del lado equivocado.
func TestLaModalidadConfirmadaMandaSobreLaInferida(t *testing.T) {
	t.Parallel()
	p, err := PlanificarRecetas(matrizDeCosteo(t), "Costos", map[string]string{
		"GARDENIA": ModalidadDirecto,
	})
	if err != nil {
		t.Fatalf("PlanificarRecetas: %v", err)
	}
	for _, pl := range p.Planes {
		if pl.Codigo == "GARDENIA" && pl.Modalidad != ModalidadDirecto {
			t.Errorf("modalidad = %q: la confirmación del usuario no se respetó", pl.Modalidad)
		}
	}
}

func TestUnaHojaQueNoEsLaMatrizSeRechaza(t *testing.T) {
	t.Parallel()
	b := libro(t, "Otra", [][]string{{"a", "b"}, {"1", "2"}})
	if _, err := AnalizarRecetas(b, "Otra"); err != ErrEncabezadoDePlanesNoHallado {
		t.Errorf("error = %v, se esperaba ErrEncabezadoDePlanesNoHallado", err)
	}
}
