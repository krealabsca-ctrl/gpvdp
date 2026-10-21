package precios

// Pruebas del costeo por actividad.
//
// La prueba central es la del 100 %: en el archivo de costeo las dos hojas reparten 45 % y 85 %,
// o sea que se absorbe un 130 % del gasto. Ese defecto no se nota mirando el total —los números
// son correctos, la suma de las partes no—, así que tiene que fallar una prueba y no el ojo de
// quien revise.

import (
	"testing"
)

func lineas(pares ...string) []LineaDeReparto {
	out := []LineaDeReparto{}
	for i := 0; i+1 < len(pares); i += 2 {
		out = append(out, LineaDeReparto{DriverID: pares[i], Driver: pares[i], Porcentaje: pares[i+1]})
	}
	return out
}

func TestUnRepartoTieneQueSumarCienPorCiento(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre string
		lineas []LineaDeReparto
		valido bool
		motivo string
	}{
		{"exacto en dos partes", lineas("servicios", "30", "contratos", "70"), true, ""},
		{"exacto en una sola", lineas("servicios", "100"), true, ""},
		{"tres partes con redondeo", lineas("a", "33.33", "b", "33.33", "c", "33.34"), true, ""},
		// El caso del archivo real: 45 % a servicios y 85 % a planes.
		{"el 130 % del archivo", lineas("servicios", "45", "planes", "85"), false,
			"sobra 30.00 %: se absorbería más gasto del que existe"},
		{"falta repartir", lineas("servicios", "60"), false, "falta 40.00 % por repartir"},
		{"sin repartir", nil, false, "sin repartir"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			suma, valido, motivo := ValidarReparto(c.lineas)
			if valido != c.valido {
				t.Errorf("válido = %v (suma %s), se esperaba %v", valido, suma.StringFixed(2), c.valido)
			}
			if motivo != c.motivo {
				t.Errorf("motivo = %q, se esperaba %q", motivo, c.motivo)
			}
		})
	}
}

// Un redondeo de porcentajes no puede invalidar un reparto correcto: 33,33 × 3 da 99,99 y eso es
// un reparto en tres partes iguales, no un error.
func TestElRedondeoDeTresPartesNoInvalidaElReparto(t *testing.T) {
	t.Parallel()
	_, valido, motivo := ValidarReparto(lineas("a", "33.33", "b", "33.33", "c", "33.33"))
	if !valido {
		t.Errorf("99.99 %% debería aceptarse por tolerancia de redondeo; motivo: %q", motivo)
	}
	// Pero un faltante de verdad sí tiene que fallar.
	if _, v, _ := ValidarReparto(lineas("a", "33", "b", "33", "c", "33")); v {
		t.Error("99.00 %% no es redondeo: falta un punto entero y debe marcarse")
	}
}

func str2(s string) *string { return &s }

func TestElPlanAbsorbeConsumoPorCostoUnitario(t *testing.T) {
	t.Parallel()
	bolsas := []BolsaDeDriver{
		{DriverID: "serv", Driver: "Servicios atendidos", Gasto: "36000000", Base: str2("360"), CostoUnitario: str2("100000.00")},
		{DriverID: "km", Driver: "Kilómetros", Gasto: "4000000", Base: str2("20000"), CostoUnitario: str2("200.00")},
	}
	consumos := []IndirectoDelPlan{
		{DriverID: "serv", Driver: "Servicios atendidos", Consumo: "1"},
		{DriverID: "km", Driver: "Kilómetros", Consumo: "150"},
	}

	a := Absorber("p1", "Oro", consumos, bolsas)
	if !a.Completo {
		t.Fatalf("debería estar completo: %+v", a.DriversSinBase)
	}
	// 1 × 100.000 + 150 × 200 = 130.000
	if a.Total != "130000.00" {
		t.Errorf("total = %q, se esperaba 130000.00", a.Total)
	}
}

// Si un driver que el plan consume NO tiene costo unitario, el total es PARCIAL. Publicarlo como
// si fuera el indirecto completo es el mismo defecto que el módulo entero existe para evitar.
func TestUnDriverSinBaseDejaLaAbsorcionIncompleta(t *testing.T) {
	t.Parallel()
	bolsas := []BolsaDeDriver{
		{DriverID: "serv", Driver: "Servicios", CostoUnitario: str2("100000.00")},
		{DriverID: "m2", Driver: "Metros de sala", Motivo: "falta la base del período: sin ella no hay costo unitario"},
	}
	consumos := []IndirectoDelPlan{
		{DriverID: "serv", Driver: "Servicios", Consumo: "1"},
		{DriverID: "m2", Driver: "Metros de sala", Consumo: "24"},
	}

	a := Absorber("p1", "Oro", consumos, bolsas)
	if a.Completo {
		t.Error("no puede estar completo: un driver no tiene costo unitario")
	}
	if len(a.DriversSinBase) != 1 || a.DriversSinBase[0] != "Metros de sala" {
		t.Errorf("no nombró el driver que falta: %v", a.DriversSinBase)
	}
	// Lo que sí se pudo calcular se muestra; lo que falta queda nombrado.
	if a.Total != "100000.00" {
		t.Errorf("total parcial = %q, se esperaba 100000.00", a.Total)
	}
	// Y la línea que falta lleva su motivo, para que no haya que adivinar por qué no sumó.
	for _, l := range a.Lineas {
		if l.DriverID == "m2" && l.Motivo == "" {
			t.Error("la línea sin costo unitario no dice por qué")
		}
	}
}

// El driver que el plan NO consume no le suma nada, aunque tenga bolsa: eso es justamente lo que
// distingue al ABC del reparto plano que tiene hoy el archivo.
func TestUnPlanSoloAbsorbeLoQueConsume(t *testing.T) {
	t.Parallel()
	bolsas := []BolsaDeDriver{
		{DriverID: "serv", Driver: "Servicios", CostoUnitario: str2("100000.00")},
		{DriverID: "contratos", Driver: "Contratos vigentes", CostoUnitario: str2("1195.00")},
	}
	// Un plan directo consume servicios, no contratos-mes.
	a := Absorber("p1", "Oro", []IndirectoDelPlan{
		{DriverID: "serv", Driver: "Servicios", Consumo: "1"},
	}, bolsas)

	if a.Total != "100000.00" {
		t.Errorf("total = %q: absorbió un driver que el plan no consume", a.Total)
	}
	if len(a.Lineas) != 1 {
		t.Errorf("líneas = %d, se esperaba 1", len(a.Lineas))
	}
}

func TestLaValidacionDelDriverRechazaLoInvalido(t *testing.T) {
	t.Parallel()
	t.Run("pone la unidad por defecto", func(t *testing.T) {
		in := DriverInput{Codigo: " SERV ", Nombre: "  Servicios  "}
		if err := in.Normalizar(); err != nil {
			t.Fatalf("Normalizar: %v", err)
		}
		if in.Codigo != "SERV" || in.Nombre != "Servicios" || in.Unidad != "UNIDAD" {
			t.Errorf("normalización = %+v", in)
		}
	})
	t.Run("rechaza lo vacío", func(t *testing.T) {
		if err := (&DriverInput{Nombre: "x"}).Normalizar(); err != ErrCodigoVacio {
			t.Errorf("sin código debería fallar")
		}
		if err := (&DriverInput{Codigo: "x"}).Normalizar(); err != ErrNombreVacio {
			t.Errorf("sin nombre debería fallar")
		}
	})
}

// «Sin repartir» y «mal repartido» son dos cosas distintas, y el reparto tiene que distinguirlas.
//
// Medido contra el gasto real de septiembre: 51 clasificaciones sin asignar y 1 mal repartida
// (la que reproducía el 45 + 85 del archivo). Contarlas juntas daba «52 problemas» y escondía el
// único que de verdad había que corregir.
func TestSinRepartirNoEsLoMismoQueMalRepartido(t *testing.T) {
	t.Parallel()
	t.Run("sin líneas es trabajo pendiente, no un error", func(t *testing.T) {
		_, valido, motivo := ValidarReparto(nil)
		if valido {
			t.Error("sin repartir no puede darse por válido")
		}
		if motivo != "sin repartir" {
			t.Errorf("motivo = %q, se esperaba «sin repartir»", motivo)
		}
	})
	t.Run("con líneas que no cierran es un error a corregir", func(t *testing.T) {
		_, valido, motivo := ValidarReparto(lineas("a", "45", "b", "85"))
		if valido {
			t.Error("130 %% no puede darse por válido")
		}
		if motivo == "sin repartir" {
			t.Error("un reparto excedido no puede reportarse como «sin repartir»: son problemas distintos")
		}
	})
}
