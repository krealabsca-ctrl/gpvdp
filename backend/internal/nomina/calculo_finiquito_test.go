package nomina

// Tests del motor de finiquito: escalas EXACTAS del Código de Trabajo (arts. 28 y 29)
// sobre el caso Carlos de la maqueta aprobada (salario promedio real 835 000 con bono y
// extras incluidos — nunca base reducida).

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func fecha(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// Carlos (maqueta #2): ingreso 15/06/2016, salida 31/07/2026 → 10 años. Despido con
// responsabilidad patronal: preaviso 1 mes + cesantía 21.5 días/año (escala CT para 10
// años) × tope 8 + vacaciones 8.5 días + aguinaldo proporcional dic–jul (8 meses).
func TestFiniquitoDespidoCarlos(t *testing.T) {
	p := calcParams(t)
	r := CalcularFiniquito(EntradaFiniquito{
		Motivo:          MotivoDespido,
		FechaIngreso:    fecha("2016-06-15"),
		FechaSalida:     fecha("2026-07-31"),
		SalarioPromedio: dec("835000"),
		DiasVacaciones:  dec("8.5"),
	}, p)

	if r.AniosServicio != 10 {
		t.Fatalf("años de servicio = %d, quiere 10", r.AniosServicio)
	}
	exigir(t, "salario diario", r.SalarioDiario, "27833") // 835000/30
	exigir(t, "preaviso", r.Preaviso, "835000")           // 30 días (≥1 año)
	exigir(t, "cesantía", r.Cesantia, "4787276")          // 27833 × 21.5 × 8 (tope)
	exigir(t, "vacaciones", r.Vacaciones, "236581")       // 27833 × 8.5 (236580.5 ↑)
	exigir(t, "aguinaldo", r.Aguinaldo, "556667")         // 835000 × 8/12
	// Solo las vacaciones cotizan: preaviso, cesantía y aguinaldo son exentos.
	exigir(t, "base afecta", r.BaseCCSS, "236581")
	exigir(t, "CCSS obrero", r.CCSSObrero, "25622") // 13012 + 10244 + 2366
	exigir(t, "renta", r.Renta, "0")                // bajo el tramo exento
	exigir(t, "total", r.Total, "6389902")          // 6415524 − 25622
}

// Renuncia: sin preaviso ni cesantía — solo vacaciones y aguinaldo proporcionales.
func TestFiniquitoRenuncia(t *testing.T) {
	p := calcParams(t)
	r := CalcularFiniquito(EntradaFiniquito{
		Motivo:          MotivoRenuncia,
		FechaIngreso:    fecha("2016-06-15"),
		FechaSalida:     fecha("2026-07-31"),
		SalarioPromedio: dec("835000"),
		DiasVacaciones:  dec("8.5"),
	}, p)
	exigir(t, "preaviso", r.Preaviso, "0")
	exigir(t, "cesantía", r.Cesantia, "0")
	exigir(t, "CCSS obrero sobre vacaciones", r.CCSSObrero, "25622")
	exigir(t, "total", r.Total, "767626") // 236581 + 556667 − 25622
}

// Menos de 1 año (7 meses): preaviso 15 días y cesantía 14 días TOTALES (no por año);
// el aguinaldo corre desde el INGRESO (posterior al 1 de diciembre).
func TestFiniquitoMenorUnAnio(t *testing.T) {
	p := calcParams(t)
	r := CalcularFiniquito(EntradaFiniquito{
		Motivo:          MotivoDespido,
		FechaIngreso:    fecha("2026-01-10"),
		FechaSalida:     fecha("2026-08-31"),
		SalarioPromedio: dec("400000"),
	}, p)
	if r.AniosServicio != 0 {
		t.Fatalf("años de servicio = %d, quiere 0", r.AniosServicio)
	}
	exigir(t, "preaviso 15 días", r.Preaviso, "200000")
	exigir(t, "cesantía 14 días totales", r.Cesantia, "186662") // 13333 × 14
	exigir(t, "aguinaldo desde el ingreso", r.Aguinaldo, "257778")
}

// Descuentos con prelación y piso 0: el adelanto pagado sin descontar cobra primero,
// luego el saldo del préstamo topado al disponible — el total jamás queda negativo.
func TestFiniquitoDescuentos(t *testing.T) {
	p := calcParams(t)
	saldo := dec("315000")
	r := CalcularFiniquito(EntradaFiniquito{
		Motivo:            MotivoRenuncia,
		FechaIngreso:      fecha("2019-03-01"),
		FechaSalida:       fecha("2026-03-31"),
		SalarioPromedio:   dec("480000"),
		DiasVacaciones:    dec("10"),
		AdelantoPendiente: dec("200000"),
		SaldosDeducciones: []DeduccionCalc{
			{ID: "d1", Etiqueta: "Préstamo Asociación", Cuota: dec("45000"), SaldoRestante: &saldo, Prioridad: 100},
		},
	}, p)
	exigir(t, "vacaciones", r.Vacaciones, "160000") // 16000 × 10
	exigir(t, "aguinaldo dic–mar", r.Aguinaldo, "160000")
	exigir(t, "CCSS sobre vacaciones", r.CCSSObrero, "17328") // 8800 + 6928 + 1600
	// Bruto 320 000 − CCSS 17 328 = 302 672 disponible: adelanto 200 000 + préstamo topado.
	exigir(t, "descuentos", r.Descuentos, "302672")
	exigir(t, "total piso 0", r.Total, "0")
	var prestamo string
	for _, d := range r.Detalle {
		if d.DeduccionID == "d1" {
			prestamo = d.Monto
		}
	}
	if prestamo != "102672.00" {
		t.Fatalf("préstamo topado = %q, quiere 102672.00", prestamo)
	}
}

// La escala de cesantía del CT art. 29 (días por año según antigüedad total).
func TestEscalaCesantia(t *testing.T) {
	casos := map[int]string{1: "19.5", 2: "20", 3: "20.5", 4: "21", 5: "21.24",
		6: "21.5", 7: "22", 9: "22", 10: "21.5", 12: "20.5", 13: "20", 20: "20"}
	for anios, want := range casos {
		if got := diasCesantiaPorAnio(anios, 0); !got.Equal(decimal.RequireFromString(want)) {
			t.Errorf("diasCesantia(%d años) = %s, quiere %s", anios, got.String(), want)
		}
	}
	if got := diasCesantiaPorAnio(0, 4); !got.Equal(decimal.NewFromInt(7)) {
		t.Errorf("3-6 meses = %s, quiere 7", got.String())
	}
	if got := diasCesantiaPorAnio(0, 2); !got.IsZero() {
		t.Errorf("<3 meses = %s, quiere 0", got.String())
	}
}

// El aguinaldo de un año completo (dic→nov) es exactamente 12/12 del promedio.
func TestMesesAguinaldoAnioCompleto(t *testing.T) {
	m := mesesTrabajados(fecha("2025-12-01"), fecha("2026-11-30"))
	if !m.Equal(decimal.NewFromInt(12)) {
		t.Fatalf("meses dic–nov = %s, quiere 12", m.String())
	}
}
