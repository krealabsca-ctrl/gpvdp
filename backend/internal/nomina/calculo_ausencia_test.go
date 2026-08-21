package nomina

// Tests del efecto de las incapacidades: política de ley confirmada por el DF —
// CCSS paga desde el día 4 (los 3 primeros los cubre la empresa al 50%) e INS desde el
// día siguiente al accidente (el día del accidente lo paga la empresa completo).

import (
	"testing"

	"github.com/shopspring/decimal"
)

// CCSS de 5 días con salario ₡900 000 (diario ₡30 000): la empresa paga 3 días al 50%
// (= 1,5 días) y no paga 2 días → se descuentan 3,5 días = ₡105 000. El empleado recibe
// de la empresa ₡795 000 (25 días trabajados + 1,5 días de incapacidad) y la CCSS le gira
// su subsidio por los días 4 y 5.
func TestIncapacidadCCSSCincoDias(t *testing.T) {
	p := calcParams(t)
	inc := IncapacidadCalc{Entidad: EntidadCCSS, FechaInicio: fecha("2026-07-08"), Dias: 5}

	ef := CalcularEfectoIncapacidad(inc, 2026, 7)
	if ef.DiasEnMes != 5 || ef.DiasCubreEntidad != 2 {
		t.Fatalf("efecto: díasEnMes=%d cubreCCSS=%d (quiere 5/2)", ef.DiasEnMes, ef.DiasCubreEntidad)
	}
	exigir(t, "días que paga la empresa", ef.DiasPagaEmpresa, "1.5")

	descuento, renglones := AjusteIncapacidades([]IncapacidadCalc{inc}, dec("900000"), 2026, 7, p)
	exigir(t, "descuento del salario", descuento, "105000")
	if len(renglones) != 1 || renglones[0].Tipo != "INCAPACIDAD" {
		t.Fatalf("renglones inesperados: %+v", renglones)
	}
	t.Logf("colilla dice: %s", renglones[0].Nombre)
}

// INS de 4 días: el día del accidente lo paga la empresa completo, los otros 3 los cubre
// el INS → se descuentan 3 días = ₡90 000.
func TestIncapacidadINSCuatroDias(t *testing.T) {
	p := calcParams(t)
	inc := IncapacidadCalc{Entidad: EntidadINS, FechaInicio: fecha("2026-07-21"), Dias: 4}

	ef := CalcularEfectoIncapacidad(inc, 2026, 7)
	exigir(t, "días que paga la empresa", ef.DiasPagaEmpresa, "1")
	if ef.DiasCubreEntidad != 3 {
		t.Fatalf("días que cubre el INS = %d, quiere 3", ef.DiasCubreEntidad)
	}
	descuento, _ := AjusteIncapacidades([]IncapacidadCalc{inc}, dec("900000"), 2026, 7, p)
	exigir(t, "descuento del salario", descuento, "90000")
}

// Una incapacidad de 2 días o menos (CCSS) la paga toda la empresa al 50%: nada de la CCSS.
func TestIncapacidadCortaSinSubsidio(t *testing.T) {
	p := calcParams(t)
	inc := IncapacidadCalc{Entidad: EntidadCCSS, FechaInicio: fecha("2026-07-10"), Dias: 2}
	ef := CalcularEfectoIncapacidad(inc, 2026, 7)
	if ef.DiasCubreEntidad != 0 {
		t.Fatalf("una incapacidad de 2 días no genera subsidio CCSS, tiene %d", ef.DiasCubreEntidad)
	}
	// 2 días − 1 día equivalente (2 × 50%) = 1 día descontado.
	descuento, _ := AjusteIncapacidades([]IncapacidadCalc{inc}, dec("900000"), 2026, 7, p)
	exigir(t, "descuento", descuento, "30000")
}

// La incapacidad que CRUZA de mes: el conteo de los primeros días corre desde su inicio,
// así que los días que caen en el mes siguiente ya no llevan el 50% de la empresa.
func TestIncapacidadCruzaDeMes(t *testing.T) {
	p := calcParams(t)
	// Empieza el 30 de junio, 5 días: 30-jun (día 1), 1..4-jul (días 2, 3, 4 y 5).
	inc := IncapacidadCalc{Entidad: EntidadCCSS, FechaInicio: fecha("2026-06-30"), Dias: 5}

	junio := CalcularEfectoIncapacidad(inc, 2026, 6)
	if junio.DiasEnMes != 1 {
		t.Fatalf("junio: díasEnMes = %d, quiere 1", junio.DiasEnMes)
	}
	exigir(t, "junio paga la empresa", junio.DiasPagaEmpresa, "0.5")

	julio := CalcularEfectoIncapacidad(inc, 2026, 7)
	if julio.DiasEnMes != 4 || julio.DiasCubreEntidad != 2 {
		t.Fatalf("julio: díasEnMes=%d cubreCCSS=%d (quiere 4/2)", julio.DiasEnMes, julio.DiasCubreEntidad)
	}
	// En julio caen los días 2 y 3 (al 50% = 1 día) y los días 4 y 5 (subsidio CCSS).
	exigir(t, "julio paga la empresa", julio.DiasPagaEmpresa, "1")
	descuento, _ := AjusteIncapacidades([]IncapacidadCalc{inc}, dec("900000"), 2026, 7, p)
	exigir(t, "descuento de julio", descuento, "90000") // (4 − 1) × 30 000
}

// Dos incapacidades distintas en el mismo mes: cada una tiene su propio conteo de
// primeros días (son eventos independientes) y ambas aparecen en la colilla.
func TestDosIncapacidadesEnElMes(t *testing.T) {
	p := calcParams(t)
	incs := []IncapacidadCalc{
		{Entidad: EntidadCCSS, FechaInicio: fecha("2026-07-06"), Dias: 2}, // 2 × 50% → descuenta 1 día
		{Entidad: EntidadCCSS, FechaInicio: fecha("2026-07-20"), Dias: 4}, // 3 × 50% + 1 → descuenta 2,5 días
	}
	descuento, renglones := AjusteIncapacidades(incs, dec("900000"), 2026, 7, p)
	if len(renglones) != 2 {
		t.Fatalf("quiere 2 renglones en la colilla, hay %d", len(renglones))
	}
	exigir(t, "descuento total", descuento, "105000") // (1 + 2,5) × 30 000
}

// Una incapacidad de otro mes no toca la corrida del mes en curso.
func TestIncapacidadDeOtroMesNoAfecta(t *testing.T) {
	p := calcParams(t)
	inc := IncapacidadCalc{Entidad: EntidadCCSS, FechaInicio: fecha("2026-05-10"), Dias: 3}
	descuento, renglones := AjusteIncapacidades([]IncapacidadCalc{inc}, dec("900000"), 2026, 7, p)
	if !descuento.IsZero() || len(renglones) != 0 {
		t.Fatalf("no debería afectar julio: descuento=%s renglones=%d", descuento, len(renglones))
	}
}

// El texto del subsidio explica quién paga qué (va en la colilla y en la pantalla).
func TestDescribirSubsidio(t *testing.T) {
	casos := []struct {
		entidad           string
		dias, entidadDias int
		contiene          string
	}{
		{EntidadCCSS, 5, 2, "subsidio de la CCSS"},
		{EntidadCCSS, 2, 0, "al 50% por cuenta de la empresa"},
		{EntidadINS, 4, 3, "lo paga la empresa"},
	}
	for _, c := range casos {
		got := DescribirSubsidio(c.entidad, c.dias, c.entidadDias)
		if !contiene(got, c.contiene) {
			t.Errorf("DescribirSubsidio(%s, %d, %d) = %q, quiere que contenga %q",
				c.entidad, c.dias, c.entidadDias, got, c.contiene)
		}
	}
}

func contiene(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

var _ = decimal.Zero // el paquete decimal se usa en los helpers compartidos
