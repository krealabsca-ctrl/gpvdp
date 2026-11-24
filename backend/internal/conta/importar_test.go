package conta

// Armar el árbol desde los códigos (Fase 1, mig 0109).
//
// EL ARCHIVO REAL que esto tiene que leer es `CATALOGO DF.1.xlsx`, 258 cuentas del Grupo Valle de
// Paz. Los casos de acá salen de él: la máscara de cinco segmentos, los niveles que se saltan y
// la segunda columna de códigos (Softland) que hay que conservar porque es el puente a los saldos
// iniciales.
//
// LA REGLA QUE SE PRUEBA MÁS DE UNA VEZ, desde ángulos distintos: solo las HOJAS aceptan
// movimiento. Si una cuenta con hijas lo aceptara, el mismo colón se sumaría dos veces al
// totalizar el árbol —una en la hija y otra en la madre— y ningún balance cuadraría.

import (
	"testing"
)

func TestElNivelSaleDeLosSegmentosSignificativos(t *testing.T) {
	casos := []struct {
		codigo string
		quiere int
	}{
		{"1-00-00-000-000", 1}, // ACTIVOS
		{"1-01-00-000-000", 2}, // ACTIVOS CORRIENTES
		{"1-01-01-000-000", 3}, // Cajas Chicas
		{"1-01-01-001-000", 4}, // CAJAS CHICAS
		{"1-01-01-001-002", 5}, // Caja Chica Cartago
		{"6-01-01-021-000", 4}, // Servicio de Electricidad
	}
	for _, c := range casos {
		if got := nivelDelCodigo(c.codigo); got != c.quiere {
			t.Errorf("nivel de %s = %d, se esperaba %d", c.codigo, got, c.quiere)
		}
	}
}

func TestLaMadreSaleDePonerEnCerosElUltimoSegmento(t *testing.T) {
	casos := []struct{ codigo, quiere string }{
		{"1-01-01-001-002", "1-01-01-001-000"},
		{"1-01-01-001-000", "1-01-01-000-000"},
		{"1-01-01-000-000", "1-01-00-000-000"},
		{"1-01-00-000-000", "1-00-00-000-000"},
		// La raíz de la clase no tiene madre: el primer segmento nunca se pone en ceros.
		{"1-00-00-000-000", ""},
	}
	for _, c := range casos {
		if got := codigoDelPadre(c.codigo); got != c.quiere {
			t.Errorf("madre de %s = %q, se esperaba %q", c.codigo, got, c.quiere)
		}
	}
}

// LA PRUEBA CENTRAL: con el molde real del catálogo, cada cuenta cuelga de la correcta y solo las
// hojas aceptan movimiento.
func TestArmarPlanCuelgaCadaCuentaYSoloLasHojasAceptanMovimiento(t *testing.T) {
	filas := []FilaImportada{
		{Fila: 3, Codigo: "1-00-00-000-000", Nombre: "ACTIVOS"},
		{Fila: 4, Codigo: "1-01-00-000-000", Nombre: "ACTIVOS CORRIENTES"},
		{Fila: 5, Codigo: "1-01-01-000-000", Nombre: "Cajas Chicas"},
		{Fila: 6, Codigo: "1-01-01-001-000", Nombre: "CAJAS CHICAS"},
		{Fila: 7, Codigo: "1-01-01-001-001", Nombre: "Caja Chica Sabana", CodigoSoftland: "1-01-01-002-000"},
		{Fila: 8, Codigo: "1-01-01-001-002", Nombre: "Caja Chica Cartago", CodigoSoftland: "1-01-01-003-000"},
		{Fila: 9, Codigo: "4-00-00-000-000", Nombre: "INGRESOS"},
		{Fila: 10, Codigo: "4-01-00-000-000", Nombre: "INGRESO POR SERVICIOS"},
		{Fila: 11, Codigo: "4-01-01-000-000", Nombre: "Servicios Directos"},
	}
	plan := ArmarPlan(filas)
	if len(plan) != len(filas) {
		t.Fatalf("plan de %d cuentas, se esperaban %d", len(plan), len(filas))
	}

	porCodigo := map[string]Cuenta{}
	for _, c := range plan {
		porCodigo[c.Codigo] = c
	}

	t.Run("cada cuenta cuelga de la correcta", func(t *testing.T) {
		esperado := map[string]string{
			"1-00-00-000-000": "",
			"1-01-00-000-000": "1-00-00-000-000",
			"1-01-01-000-000": "1-01-00-000-000",
			"1-01-01-001-000": "1-01-01-000-000",
			"1-01-01-001-001": "1-01-01-001-000",
			"1-01-01-001-002": "1-01-01-001-000",
			"4-01-01-000-000": "4-01-00-000-000",
		}
		for cod, madre := range esperado {
			c := porCodigo[cod]
			got := ""
			if c.PadreID != nil {
				got = *c.PadreID
			}
			if got != madre {
				t.Errorf("%s cuelga de %q, se esperaba %q", cod, got, madre)
			}
		}
	})

	t.Run("solo las hojas aceptan movimiento", func(t *testing.T) {
		hojas := map[string]bool{
			"1-01-01-001-001": true, // Caja Chica Sabana
			"1-01-01-001-002": true, // Caja Chica Cartago
			"4-01-01-000-000": true, // Servicios Directos: nadie cuelga de ella
		}
		for cod, c := range porCodigo {
			if c.AceptaMovimiento != hojas[cod] {
				t.Errorf("%s (%s): acepta_movimiento = %v, se esperaba %v",
					cod, c.Nombre, c.AceptaMovimiento, hojas[cod])
			}
		}
	})

	t.Run("la madre viene SIEMPRE antes que la hija", func(t *testing.T) {
		// No es cosmética: la llave foránea rechaza una hija cuya madre todavía no existe.
		visto := map[string]bool{}
		for _, c := range plan {
			if c.PadreID != nil && !visto[*c.PadreID] {
				t.Errorf("%s llega antes que su madre %s", c.Codigo, *c.PadreID)
			}
			visto[c.Codigo] = true
		}
	})

	t.Run("la clase y la naturaleza salen del código", func(t *testing.T) {
		if c := porCodigo["1-01-01-001-001"]; c.Clase != ClaseActivo || c.Naturaleza != Deudora {
			t.Errorf("caja chica: clase %s, naturaleza %s; se esperaba ACTIVO/DEUDORA", c.Clase, c.Naturaleza)
		}
		if c := porCodigo["4-01-01-000-000"]; c.Clase != ClaseIngreso || c.Naturaleza != Acreedora {
			t.Errorf("ingreso: clase %s, naturaleza %s; se esperaba INGRESO/ACREEDORA", c.Clase, c.Naturaleza)
		}
	})

	t.Run("el código de Softland se conserva", func(t *testing.T) {
		// Es el puente a los saldos iniciales (Fase 4). Perderlo obliga a rehacer el cruce a mano.
		if got := porCodigo["1-01-01-001-002"].CodigoSoftland; got != "1-01-01-003-000" {
			t.Errorf("código Softland = %q, se esperaba 1-01-01-003-000", got)
		}
	})
}

// Un plan que salta niveles: la madre que falta se busca un escalón más arriba.
//
// Pasa en los catálogos reales todo el tiempo —una cuenta de nivel 5 colgando directo de una de
// nivel 3 porque el nivel 4 no existe— y si no se tolerara, esas cuentas quedarían como raíces
// sueltas de la clase.
func TestUnPlanQueSaltaNivelesCuelgaDeLaMadreQueSiExiste(t *testing.T) {
	plan := ArmarPlan([]FilaImportada{
		{Codigo: "6-00-00-000-000", Nombre: "GASTOS"},
		{Codigo: "6-01-00-000-000", Nombre: "GASTOS GENERALES"},
		// No existe «6-01-01-000-000»: esta tiene que colgar de GASTOS GENERALES.
		{Codigo: "6-01-01-021-000", Nombre: "Servicio de Electricidad"},
	})
	for _, c := range plan {
		if c.Codigo != "6-01-01-021-000" {
			continue
		}
		if c.PadreID == nil || *c.PadreID != "6-01-00-000-000" {
			got := "(ninguna)"
			if c.PadreID != nil {
				got = *c.PadreID
			}
			t.Errorf("cuelga de %s, se esperaba 6-01-00-000-000", got)
		}
		return
	}
	t.Fatal("la cuenta no quedó en el plan")
}

// Las filas que no son cuentas se descartan CON SU MOTIVO, no en silencio: un catálogo trae
// títulos y subtotales, y quien importa tiene que poder ver qué quedó afuera y por qué.
func TestLasFilasQueNoSonCuentasSeDescartanConMotivo(t *testing.T) {
	filas := []FilaImportada{
		{Fila: 1, Codigo: "ACTUAL", Nombre: ""},
		{Fila: 2, Codigo: "", Nombre: "Un subtítulo suelto"},
		{Fila: 3, Codigo: "9-99-99-999-999", Nombre: "Clase que no existe"},
		{Fila: 4, Codigo: "1-01-00-000-000", Nombre: "ACTIVOS CORRIENTES"},
	}
	// Se marcan como las marcaría el lector del Excel.
	for i := range filas {
		switch {
		case filas[i].Codigo == "":
			filas[i].Problema = "sin código"
		case filas[i].Nombre == "":
			filas[i].Problema = "sin nombre"
		case filas[i].Codigo == "9-99-99-999-999":
			filas[i].Problema = "el código no empieza por una clase contable (1 a 7)"
		}
	}
	plan := ArmarPlan(filas)
	if len(plan) != 1 {
		t.Fatalf("entraron %d cuentas, se esperaba 1 (solo la válida)", len(plan))
	}
	if plan[0].Codigo != "1-01-00-000-000" {
		t.Errorf("entró %s, se esperaba la única cuenta válida", plan[0].Codigo)
	}
}

// Un código repetido dentro del mismo archivo no puede reventar la importación a mitad de camino.
func TestUnCodigoRepetidoEnElArchivoNoDuplica(t *testing.T) {
	plan := ArmarPlan([]FilaImportada{
		{Codigo: "1-00-00-000-000", Nombre: "ACTIVOS"},
		{Codigo: "1-00-00-000-000", Nombre: "ACTIVOS otra vez"},
	})
	if len(plan) != 1 {
		t.Fatalf("plan de %d cuentas, se esperaba 1", len(plan))
	}
	if plan[0].Nombre != "ACTIVOS" {
		t.Errorf("gana la segunda (%q); tiene que ganar la primera", plan[0].Nombre)
	}
}
