package nomina

// Tests de los reportes del cierre (Etapa 5): el finiquito del mes entra a la planilla CCSS
// —las vacaciones pagadas al cese son salario— sin duplicar la fila del trabajador.

import "testing"

// La carga patronal del finiquito se calcula sobre la porción AFECTA (vacaciones), con la
// misma fórmula que la colilla: es lo que la empresa le debe a la CCSS por ese salario.
func TestFiniquitoLlevaCargaPatronal(t *testing.T) {
	p := calcParams(t)
	r := CalcularFiniquito(EntradaFiniquito{
		Motivo:          MotivoDespido,
		FechaIngreso:    fecha("2016-06-15"),
		FechaSalida:     fecha("2026-07-31"),
		SalarioPromedio: dec("835000"),
		DiasVacaciones:  dec("8.5"),
	}, p)

	esperado, _ := CargasPatronales(r.BaseCCSS, p)
	exigir(t, "patronal del finiquito", r.Patronal, esperado.String())
	if !r.Patronal.IsPositive() {
		t.Fatalf("la carga patronal sobre %s debe ser positiva", r.BaseCCSS)
	}
	// Es costo del patrono: no toca lo que recibe la persona.
	bruto := r.Preaviso.Add(r.Cesantia).Add(r.Vacaciones).Add(r.Aguinaldo)
	exigir(t, "total (patronal no se resta)", r.Total,
		bruto.Sub(r.CCSSObrero).Sub(r.Renta).Sub(r.Descuentos).String())

	// Sin vacaciones pendientes no hay base afecta: nada que cotizar ni reportar.
	sinVac := CalcularFiniquito(EntradaFiniquito{
		Motivo:          MotivoRenuncia,
		FechaIngreso:    fecha("2025-01-15"),
		FechaSalida:     fecha("2026-07-31"),
		SalarioPromedio: dec("835000"),
	}, p)
	exigir(t, "patronal sin vacaciones", sinVac.Patronal, "0")
}

func TestFusionarPlanilla(t *testing.T) {
	colilla := LineaCorrida{Nombre: "Ana Solís", Identificacion: "1-1111-1111",
		BaseCCSS: "900000.00", CCSSObrero: "97470.00", Patronal: "232470.00"}
	finAna := FiniquitoDelMes{Nombre: "Ana Solís", Identificacion: "1-1111-1111",
		BaseCCSS: "180000.00", CCSSObrero: "19494.00", Patronal: "46494.00", Total: "500000.00"}
	finOtro := FiniquitoDelMes{Nombre: "Beto Mora", Identificacion: "2-2222-2222",
		BaseCCSS: "150000.00", CCSSObrero: "16245.00", Patronal: "38745.00", Total: "400000.00"}
	finExento := FiniquitoDelMes{Nombre: "Carla Ruiz", Identificacion: "3-3333-3333",
		BaseCCSS: "0.00", CCSSObrero: "0.00", Patronal: "0.00", Total: "300000.00"}

	t.Run("misma persona: una sola fila con las dos bases sumadas", func(t *testing.T) {
		filas, err := fusionarPlanilla([]LineaCorrida{colilla}, []FiniquitoDelMes{finAna})
		if err != nil {
			t.Fatalf("fusionarPlanilla: %v", err)
		}
		if len(filas) != 1 {
			t.Fatalf("filas = %d, quiere 1 (no se duplica el trabajador)", len(filas))
		}
		exigir(t, "base reportada", filas[0].Base, "1080000")
		exigir(t, "CCSS obrero", filas[0].Obrero, "116964")
		exigir(t, "patronal", filas[0].Patronal, "278964")
		if !filas[0].ConFiniquito || filas[0].Origen != "Salario del mes + vacaciones del finiquito" {
			t.Errorf("origen = %q (debe advertir que incluye el finiquito)", filas[0].Origen)
		}
	})

	t.Run("cese sin colilla del mes: fila propia", func(t *testing.T) {
		filas, err := fusionarPlanilla([]LineaCorrida{colilla}, []FiniquitoDelMes{finOtro})
		if err != nil {
			t.Fatalf("fusionarPlanilla: %v", err)
		}
		if len(filas) != 2 {
			t.Fatalf("filas = %d, quiere 2", len(filas))
		}
		exigir(t, "base del cese", filas[1].Base, "150000")
		if filas[1].Origen != "Vacaciones del finiquito (cese)" {
			t.Errorf("origen = %q", filas[1].Origen)
		}
	})

	t.Run("finiquito sin porción afecta: no agrega fila", func(t *testing.T) {
		filas, err := fusionarPlanilla(nil, []FiniquitoDelMes{finExento})
		if err != nil {
			t.Fatalf("fusionarPlanilla: %v", err)
		}
		if len(filas) != 0 {
			t.Fatalf("filas = %d, quiere 0 (cesantía y aguinaldo son exentos)", len(filas))
		}
	})

	t.Run("monto corrupto: error, nunca una planilla a medias", func(t *testing.T) {
		roto := finOtro
		roto.Patronal = "x"
		if _, err := fusionarPlanilla(nil, []FiniquitoDelMes{roto}); err == nil {
			t.Fatal("quiere error con la carga patronal corrupta")
		}
	})
}
