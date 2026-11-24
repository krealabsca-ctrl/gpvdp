package bancos

// Mediana, recurrencia y estacionalidad.
//
// Los tres existen por lo mismo: el promedio simple no distingue un ritmo de un accidente. Los
// casos de abajo usan la forma real de Memorial Pets, medida el 28-oct-2026.

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

func montoDec(s string) decimal.Decimal {
	v, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestMediana(t *testing.T) {
	casos := []struct {
		nombre string
		in     []string
		quiero string
	}{
		{"impares toma el del medio", []string{"10", "30", "20"}, "20"},
		// Con dos centrales se promedian: si no, la mediana saltaría según la paridad del
		// conteo y agregar un mes la movería sin que cambie nada.
		{"pares promedia los dos centrales", []string{"10", "20", "30", "40"}, "25"},
		{"uno solo es él mismo", []string{"7"}, "7"},
		{"vacío es cero", nil, "0"},
		// EL CASO QUE MOTIVA TODO: once meses de ₡150.000 y uno de ₡2.500.000. El promedio se va
		// a ₡345.833 —más del doble de la mediana— y a partir de ahí cada mes normal marca una
		// caída del 57 % que no ocurrió. Es la forma de «Synergia» en Memorial (+126 %).
		{"un pago grande no mueve la mediana", []string{
			"150000", "150000", "150000", "150000", "150000", "150000",
			"150000", "150000", "150000", "150000", "150000", "2500000"}, "150000"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			in := make([]decimal.Decimal, 0, len(c.in))
			for _, s := range c.in {
				in = append(in, montoDec(s))
			}
			if got := mediana(in); !got.Equal(montoDec(c.quiero)) {
				t.Fatalf("mediana = %s, se esperaba %s", got, c.quiero)
			}
		})
	}

	t.Run("no reordena la lista que recibe", func(t *testing.T) {
		// La serie llega en orden cronológico y otros cálculos dependen de eso: ordenarla en
		// sitio para la mediana rompería el «último mes» sin que nada avise.
		in := []decimal.Decimal{montoDec("30"), montoDec("10"), montoDec("20")}
		mediana(in)
		if !in[0].Equal(montoDec("30")) {
			t.Errorf("la mediana reordenó la serie: %v", in)
		}
	})
}

func TestRecurrenciaDe(t *testing.T) {
	// Sobre 17 meses comparables, que es lo que tiene Memorial.
	casos := []struct {
		conGasto int
		quiero   string
	}{
		{17, RecurrenciaFija},         // todos los meses
		{14, RecurrenciaFija},         // 82 %: una mensual que se saltó dos pagos sigue siendo fija
		{13, RecurrenciaIntermitente}, // 76 %
		{7, RecurrenciaIntermitente},  // 41 %
		{6, RecurrenciaEsporadica},    // 35 %: ya no hay serie que promediar
		{1, RecurrenciaEsporadica},
		{0, RecurrenciaEsporadica},
	}
	for _, c := range casos {
		if got := recurrenciaDe(c.conGasto, 17); got != c.quiero {
			t.Errorf("con %d de 17 meses dio %q, se esperaba %q", c.conGasto, got, c.quiero)
		}
	}
	// Sin meses comparables no se puede afirmar un ritmo.
	if got := recurrenciaDe(0, 0); got != RecurrenciaEsporadica {
		t.Errorf("sin meses comparables dio %q", got)
	}
}

// LA MEDIANA CAMBIA LA CONCLUSIÓN, no solo el número. Un mes normal después de un pago grande:
// contra el promedio parece una caída del 57 %; contra la mediana, está en su sitio.
func TestElPagoGrandeNoVuelveAnomaliaAlMesSiguiente(t *testing.T) {
	serie := map[string]string{}
	for i := 1; i <= 11; i++ {
		serie[periodoDeIndice(i)] = "150000"
	}
	serie["2025-12"] = "2500000" // el pago grande
	serie["2026-01"] = "155000"  // un mes normal: el que se juzga

	salud := make([]SaludMes, 0, 13)
	for p := range serie {
		salud = append(salud, mesSano(p))
	}
	ordenarSalud(salud)

	svc := servicioConRangos(
		map[string][]SaludMes{"2025-01..2026-01": salud},
		map[string][]TendenciaPartida{"2025-01..2026-01": {partida("c1", "Synergia", serie)}},
	)
	res, err := svc.AnalisisPartidas(context.Background(), "e1", "2025-01", "2026-01")
	if err != nil {
		t.Fatalf("analizar: %v", err)
	}
	p := res.Partidas[0]

	// Promedio de los 12 anteriores: (11×150.000 + 2.500.000) / 12 = 345.833,33
	if p.Promedio != "345833.33" {
		t.Fatalf("promedio %q, se esperaba 345833.33", p.Promedio)
	}
	if p.Mediana != "150000.00" {
		t.Fatalf("mediana %q, se esperaba 150000.00", p.Mediana)
	}
	// Contra el promedio, el mes normal parece un desplome.
	if p.DesvioPct != "-55.2" {
		t.Errorf("desvío contra el promedio %q, se esperaba -55.2", p.DesvioPct)
	}
	// Contra la mediana, está donde siempre estuvo.
	if p.DesvioMedianaPct != "3.3" {
		t.Errorf("desvío contra la mediana %q, se esperaba 3.3", p.DesvioMedianaPct)
	}
}

func periodoDeIndice(i int) string {
	return "2025-" + dosDigitos(i)
}

func dosDigitos(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func ordenarSalud(s []SaludMes) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j].Periodo < s[i].Periodo {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

// LA RECURRENCIA ES UNA PROPIEDAD DE LA PARTIDA, no un juicio sobre el último mes: cuenta todos
// los meses comparables, el último incluido.
func TestLaRecurrenciaSeCalculaSobreElRango(t *testing.T) {
	// Una partida que solo se paga dos veces en doce meses: su «promedio mensual» no significa
	// nada, y la pantalla tiene que poder decirlo.
	esporadica := map[string]string{}
	fija := map[string]string{}
	salud := make([]SaludMes, 0, 12)
	for i := 1; i <= 12; i++ {
		p := "2026-" + dosDigitos(i)
		salud = append(salud, mesSano(p))
		fija[p] = "100000"
		if i == 3 || i == 9 {
			esporadica[p] = "500000"
		} else {
			esporadica[p] = "0"
		}
	}

	svc := servicioConRangos(
		map[string][]SaludMes{"2026-01..2026-12": salud},
		map[string][]TendenciaPartida{"2026-01..2026-12": {
			partida("c1", "Planilla", fija),
			partida("c2", "Liquidación Laboral", esporadica),
		}},
	)
	res, err := svc.AnalisisPartidas(context.Background(), "e1", "2026-01", "2026-12")
	if err != nil {
		t.Fatalf("analizar: %v", err)
	}
	porNombre := map[string]TendenciaPartida{}
	for _, p := range res.Partidas {
		porNombre[p.Clasificacion] = p
	}
	if got := porNombre["Planilla"]; got.Recurrencia != RecurrenciaFija || got.MesesConGasto != 12 {
		t.Errorf("Planilla: %q con %d meses; se esperaba FIJO con 12", got.Recurrencia, got.MesesConGasto)
	}
	if got := porNombre["Liquidación Laboral"]; got.Recurrencia != RecurrenciaEsporadica || got.MesesConGasto != 2 {
		t.Errorf("Liquidación: %q con %d meses; se esperaba ESPORADICO con 2", got.Recurrencia, got.MesesConGasto)
	}
}

// ── Estacionalidad ──────────────────────────────────────────────────────────────────────────

// CON UN SOLO AÑO NO HAY ESTACIONALIDAD: es el dato de ese mes. Presentarlo como patrón sería
// inventar, y es exactamente lo que pasa hoy en Memorial, donde solo 5 de 12 meses tienen dos
// observaciones.
func TestLaEstacionalidadDeclaraCuantosAniosLaRespaldan(t *testing.T) {
	// Dos eneros y un julio.
	serie := map[string]string{
		"2025-01": "1000", "2025-07": "2000", "2026-01": "1000",
	}
	salud := []SaludMes{mesSano("2025-01"), mesSano("2025-07"), mesSano("2026-01")}

	est, err := Estacionalidad(salud, []TendenciaPartida{partida("c1", "Gastos", serie)})
	if err != nil {
		t.Fatalf("estacionalidad: %v", err)
	}
	porMes := map[int]MesEstacional{}
	for _, e := range est {
		porMes[e.Mes] = e
	}
	if porMes[1].Anios != 2 || !porMes[1].Confiable {
		t.Errorf("enero: %d año(s), confiable=%v; se esperaba 2 y true", porMes[1].Anios, porMes[1].Confiable)
	}
	if porMes[7].Anios != 1 || porMes[7].Confiable {
		t.Errorf("julio: %d año(s), confiable=%v; se esperaba 1 y false", porMes[7].Anios, porMes[7].Confiable)
	}
	// El índice se calcula igual, pero julio queda marcado como no confiable: el promedio de los
	// tres períodos es 1.333,33, así que julio da 150 y enero 75.
	if porMes[7].Indice != "150.0" {
		t.Errorf("índice de julio %q, se esperaba 150.0", porMes[7].Indice)
	}
	if porMes[1].Indice != "75.0" {
		t.Errorf("índice de enero %q, se esperaba 75.0", porMes[1].Indice)
	}
}

// Un mes a medio clasificar no entra: mostraría menos gasto del que tuvo y bajaría su índice.
func TestLaEstacionalidadIgnoraLosMesesIncompletos(t *testing.T) {
	serie := map[string]string{"2025-01": "1000", "2025-02": "100", "2026-01": "1000"}
	salud := []SaludMes{mesSano("2025-01"), mesFlojo("2025-02"), mesSano("2026-01")}

	est, err := Estacionalidad(salud, []TendenciaPartida{partida("c1", "Gastos", serie)})
	if err != nil {
		t.Fatalf("estacionalidad: %v", err)
	}
	for _, e := range est {
		if e.Mes == 2 {
			t.Fatal("febrero está al 40 % clasificado y entró a la estacionalidad")
		}
	}
	if len(est) != 1 || est[0].Mes != 1 || est[0].Anios != 2 {
		t.Errorf("se esperaba solo enero con dos años: %+v", est)
	}
}

// Con un trimestre no hay patrón anual que mostrar, y es correcto que no lo haya.
func TestUnRangoCortoNoProduceEstacionalidad(t *testing.T) {
	svc := servicioConRangos(
		map[string][]SaludMes{"2026-01..2026-03": {mesSano("2026-01"), mesSano("2026-02"), mesSano("2026-03")}},
		map[string][]TendenciaPartida{"2026-01..2026-03": {partida("c1", "Planilla", map[string]string{
			"2026-01": "100", "2026-02": "100", "2026-03": "100"})}},
	)
	res, err := svc.AnalisisPartidas(context.Background(), "e1", "2026-01", "2026-03")
	if err != nil {
		t.Fatalf("analizar: %v", err)
	}
	// Hay tres meses del calendario, pero ninguno con dos años: nada es confiable.
	if res.MesesConDosAnios != 0 {
		t.Errorf("declaró %d mes(es) con dos años sobre un trimestre", res.MesesConDosAnios)
	}
	for _, e := range res.Estacionalidad {
		if e.Confiable {
			t.Errorf("el mes %d salió confiable con un solo año", e.Mes)
		}
	}
}
