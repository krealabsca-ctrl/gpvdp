package bancos

import (
	"testing"

	"github.com/shopspring/decimal"
)

func d(v int64) decimal.Decimal { return decimal.NewFromInt(v) }

func senda(pares ...int64) []DiaMonto {
	out := make([]DiaMonto, 0, len(pares)/2)
	for i := 0; i+1 < len(pares); i += 2 {
		out = append(out, DiaMonto{Dia: int(pares[i]), Monto: decimal.NewFromInt(pares[i+1])})
	}
	return out
}

func TestProyectarRitmo(t *testing.T) {
	// Julio 2026: 10 días con actividad hasta el día 14 (acum 1100, media 110).
	// Días activos restantes 15..31 sin domingos (19 y 26): 15 → 1100 + 110×15.
	actual := senda(1, 100, 2, 100, 3, 100, 4, 100, 6, 100, 7, 100, 8, 100, 9, 100, 10, 100, 13, 200)
	got := proyectarRitmo(actual, 14, 2026, 7, 31)
	quiere := d(1100).Add(d(110).Mul(d(15))) // acum 1100, media 110, 15 activos
	if !got.Equal(quiere) {
		t.Errorf("ritmo = %s, quiere %s", got, quiere)
	}
	// Sin actividad: devuelve el acumulado tal cual (cero).
	if got := proyectarRitmo(nil, 14, 2026, 7, 31); !got.IsZero() {
		t.Errorf("ritmo sin datos = %s, quiere 0", got)
	}
}

func TestProyectarHistorico(t *testing.T) {
	// El histórico llevaba 400 de 1000 al día 14 (40%); real 200 → cierre 500.
	hist := senda(5, 400, 20, 600)
	got, ok := proyectarHistorico(d(200), hist, 14)
	if !ok || !got.Equal(d(500)) {
		t.Errorf("historico = %s ok=%v, quiere 500 true", got, ok)
	}
	// Histórico sin avance al día → no disponible.
	if _, ok := proyectarHistorico(d(200), senda(20, 600), 14); ok {
		t.Error("historico sin avance debe ser no disponible")
	}
	if _, ok := proyectarHistorico(d(200), nil, 14); ok {
		t.Error("historico vacío debe ser no disponible")
	}
}

func TestProyectarPromedio(t *testing.T) {
	// Mes A: resto tras el día 14 = 300; mes B: resto = 500 → promedio 400; real 200 → 600.
	meses := []SendaMes{
		{Periodo: "2026-05", Dias: senda(10, 700, 20, 300)},
		{Periodo: "2026-06", Dias: senda(10, 500, 20, 500)},
	}
	got, ok := proyectarPromedio(d(200), meses, 14)
	if !ok || !got.Equal(d(600)) {
		t.Errorf("promedio = %s ok=%v, quiere 600 true", got, ok)
	}
	if _, ok := proyectarPromedio(d(200), nil, 14); ok {
		t.Error("promedio sin meses debe ser no disponible")
	}
}

func TestProyectarCoincidencia(t *testing.T) {
	// Actual: fuerte al inicio (todo al día 2). Gemelo esperado: mayo (misma forma),
	// no junio (todo al final). Mayo: avance al 14 = 800/1000 → cierre = 400/0.8 = 500.
	actual := senda(2, 400)
	meses := []SendaMes{
		{Periodo: "2026-05", Dias: senda(2, 800, 20, 200)},
		{Periodo: "2026-06", Dias: senda(13, 100, 20, 900)},
	}
	got, gemelo, ok := proyectarCoincidencia(actual, d(400), meses, 14)
	if !ok || gemelo != "2026-05" || !got.Equal(d(500)) {
		t.Errorf("coincidencia = %s gemelo=%s ok=%v, quiere 500 2026-05 true", got, gemelo, ok)
	}
	if _, _, ok := proyectarCoincidencia(actual, d(400), nil, 14); ok {
		t.Error("coincidencia sin meses debe ser no disponible")
	}
}

func TestMetaCierre(t *testing.T) {
	if got := metaCierre(d(1000), d(6)); !got.Equal(d(1060)) {
		t.Errorf("meta = %s, quiere 1060", got)
	}
	if got := metaCierre(decimal.Zero, d(6)); !got.IsZero() {
		t.Errorf("meta sin base = %s, quiere 0", got)
	}
	// Meta negativa (decrecimiento) también vale.
	if got := metaCierre(d(1000), d(-10)); !got.Equal(d(900)) {
		t.Errorf("meta -10%% = %s, quiere 900", got)
	}
}

func TestSendas(t *testing.T) {
	acum := sendaAcumulada(senda(1, 100, 3, 50), 4)
	if len(acum) != 4 || acum[0].Acumulado != "100" || acum[1].Acumulado != "100" ||
		acum[2].Acumulado != "150" || acum[3].Acumulado != "150" {
		t.Errorf("sendaAcumulada = %+v", acum)
	}
	// Proyectada: termina exactamente en el cierre.
	proy := sendaProyectada(d(150), d(450), 14, 31, 2026, 7)
	ultimo := proy[len(proy)-1]
	if ultimo.Dia != 31 || ultimo.Acumulado != "450" {
		t.Errorf("sendaProyectada termina en %+v, quiere día 31 = 450", ultimo)
	}
}
