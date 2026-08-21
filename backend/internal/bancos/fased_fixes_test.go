package bancos

// Tests de los defectos corregidos tras la revisión adversarial de la Fase D.

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestProblemasLinea(t *testing.T) {
	linea := func(deb, cred string) MovimientoParsed {
		return MovimientoParsed{Debito: dec(deb), Credito: dec(cred)}
	}
	casos := []struct {
		nombre    string
		m         MovimientoParsed
		problemas int
	}{
		{"débito normal", linea("100", "0"), 0},
		{"crédito normal", linea("0", "250.50"), 0},
		{"ambos > 0 (§19 regla 1)", linea("100", "50"), 1},
		{"sin monto", linea("0", "0"), 1},
		{"débito negativo", linea("-100", "0"), 1},
		{"crédito negativo", linea("0", "-5"), 1},
		{"negativo + positivo (antes se colaba y corrompía el monto)", linea("-100", "50"), 1},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := len(problemasLinea(c.m)); got != c.problemas {
				t.Errorf("problemasLinea = %d problema(s), quiere %d: %v", got, c.problemas, problemasLinea(c.m))
			}
		})
	}
}

// Contrato con el frontend: una línea sana debe devolver un slice NO nil (serializa []),
// no nil (serializaría null y rompía el importador al leer .length). Regresión.
func TestProblemasLineaSanaNoEsNil(t *testing.T) {
	if problemasLinea(MovimientoParsed{Debito: dec("100"), Credito: dec("0")}) == nil {
		t.Error("línea sana debe devolver [] (no nil): el frontend espera un array en 'advertencias'")
	}
}

func TestToleranciaEfectiva(t *testing.T) {
	// 0 explícito = emparejamiento exacto (NO cae al default).
	if got := toleranciaEfectiva(decimal.Zero); !got.IsZero() {
		t.Errorf("tolerancia 0 debe respetarse, dio %s", got)
	}
	// Valor configurado se respeta.
	if got := toleranciaEfectiva(dec("0.015")); !got.Equal(dec("0.015")) {
		t.Errorf("tolerancia 1.5%% = %s", got)
	}
	// Negativo (defensivo) cae al default 1%.
	if got := toleranciaEfectiva(dec("-0.01")); !got.Equal(ToleranciaTrasladoDefault) {
		t.Errorf("negativo debe caer al default, dio %s", got)
	}
	// Con tolerancia 0, solo montos exactos emparejan.
	if dentroDeTolerancia(dec("100.00"), dec("100.90"), toleranciaEfectiva(decimal.Zero)) {
		t.Error("con tolerancia 0, 100.00 vs 100.90 NO debe emparejar")
	}
	if !dentroDeTolerancia(dec("100.00"), dec("100.00"), toleranciaEfectiva(decimal.Zero)) {
		t.Error("con tolerancia 0, montos exactos SÍ deben emparejar")
	}
}
