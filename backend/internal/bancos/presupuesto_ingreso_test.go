package bancos

// El presupuesto de INGRESO.
//
// Lo que se prueba acá es la ASIMETRÍA con el gasto, que es donde se equivoca todo el mundo:
// consumir el 95 % del gasto es una alerta y llegar al 95 % de la meta de ingreso es casi un
// logro. Un semáforo copiado del gasto pinta de rojo un mes bueno, y nadie lo nota porque el
// número de abajo es correcto.

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func pi(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// PASARSE DE LA META ES LA BUENA NOTICIA, al revés del gasto.
func TestCumplirDeMasEsCumplido(t *testing.T) {
	estado, pct := estadoDeIngreso(pi("1000000"), pi("1200000"))
	if estado != IngresoCumplido {
		t.Errorf("120 %% de la meta dio %q, se esperaba CUMPLIDO", estado)
	}
	if pct != "120.0" {
		t.Errorf("porcentaje %q", pct)
	}
	// Y justo en la meta también se cumple: el 100 % no es «casi».
	if e, _ := estadoDeIngreso(pi("1000000"), pi("1000000")); e != IngresoCumplido {
		t.Errorf("exactamente la meta dio %q", e)
	}
}

// Los tres escalones, con el umbral en su borde.
func TestLosEscalonesDelCumplimiento(t *testing.T) {
	casos := []struct {
		nombre, meta, real, quiere string
	}{
		{"muy por debajo", "1000", "400", IngresoCorto},
		{"justo bajo el umbral", "1000", "849", IngresoCorto},
		{"en el umbral es en camino", "1000", "850", IngresoEnCamino},
		{"casi la meta", "1000", "990", IngresoEnCamino},
		{"la meta", "1000", "1000", IngresoCumplido},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if e, _ := estadoDeIngreso(pi(c.meta), pi(c.real)); e != c.quiere {
				t.Errorf("meta %s real %s dio %q, se esperaba %q", c.meta, c.real, e, c.quiere)
			}
		})
	}
}

// SIN META NO ES CERO POR CIENTO. Un mes sin meta declarada no incumplió nada: nadie dijo qué se
// esperaba. Pintarlo en rojo haría que el semáforo mienta justo sobre el período que no tiene
// nada que juzgar.
func TestSinMetaNoSeJuzga(t *testing.T) {
	for _, meta := range []string{"0", "-5000"} {
		estado, pct := estadoDeIngreso(pi(meta), pi("3000000"))
		if estado != IngresoSinMeta {
			t.Errorf("con meta %s el estado fue %q, se esperaba SIN_META", meta, estado)
		}
		if pct != "" {
			t.Errorf("con meta %s devolvió un porcentaje (%q) contra una meta inexistente", meta, pct)
		}
	}
}

// EL RESULTADO PRESUPUESTADO es la razón de existir de esta mitad: con una sola meta no se podía
// calcular.
func TestElResultadoPresupuestadoCruzaLasDosMitades(t *testing.T) {
	r := CalcularResultado(pi("10000000"), pi("9000000"), pi("7000000"), pi("7500000"))

	if r.ResultadoMeta != "3000000.00" {
		t.Errorf("resultado meta %q, se esperaba 3000000.00", r.ResultadoMeta)
	}
	if r.ResultadoReal != "1500000.00" {
		t.Errorf("resultado real %q, se esperaba 1500000.00", r.ResultadoReal)
	}
	// Se cobró menos y se gastó más: el desvío es negativo por los dos lados.
	if r.Desvio != "-1500000.00" {
		t.Errorf("desvío %q, se esperaba -1500000.00", r.Desvio)
	}
	if !r.Completo {
		t.Error("con las dos metas declaradas, el resultado tiene que estar completo")
	}
	if r.Aviso != "" {
		t.Errorf("avisó algo sin hacer falta: %q", r.Aviso)
	}
}

// CON MEDIA META EL RESULTADO ENGAÑA, Y HAY QUE DECIR HACIA QUÉ LADO.
//
// Sin meta de ingreso, el resultado presupuestado sale artificialmente malo; sin presupuesto de
// gasto, artificialmente bueno. Las dos cifras se ven igual de plausibles, así que el aviso tiene
// que nombrar la dirección del sesgo y no solo decir «faltan datos».
func TestConMediaMetaElAvisoDiceHaciaQueLadoEngana(t *testing.T) {
	t.Run("falta la meta de ingreso", func(t *testing.T) {
		r := CalcularResultado(decimal.Zero, pi("9000000"), pi("7000000"), pi("7500000"))
		if r.Completo {
			t.Error("se declaró completo sin meta de ingreso")
		}
		if !strings.Contains(r.Aviso, "INGRESO") || !strings.Contains(r.Aviso, "peor") {
			t.Errorf("el aviso no dice que sale PEOR de lo que debería: %q", r.Aviso)
		}
	})

	t.Run("falta el presupuesto de gasto", func(t *testing.T) {
		r := CalcularResultado(pi("10000000"), pi("9000000"), decimal.Zero, pi("7500000"))
		if !strings.Contains(r.Aviso, "GASTO") || !strings.Contains(r.Aviso, "mejor") {
			t.Errorf("el aviso no dice que sale MEJOR de lo que debería: %q", r.Aviso)
		}
	})

	t.Run("no hay ninguna de las dos", func(t *testing.T) {
		r := CalcularResultado(decimal.Zero, pi("9000000"), decimal.Zero, pi("7500000"))
		if !strings.Contains(r.Aviso, "sin nada contra qué compararlo") {
			t.Errorf("aviso: %q", r.Aviso)
		}
		// El resultado real se calcula igual: es un dato válido aunque no haya con qué compararlo.
		if r.ResultadoReal != "1500000.00" {
			t.Errorf("resultado real %q", r.ResultadoReal)
		}
	})
}

// El total del mes y el desglose por línea NO se suman entre sí: son dos vistas del mismo dinero.
func TestElTotalDelMesSeDistingueDelDesglose(t *testing.T) {
	total := MetaIngreso{Periodo: "2026-11", Monto: "50000000"}
	linea := MetaIngreso{Periodo: "2026-11", Monto: "20000000", ClasificacionID: "abc", Clasificacion: "Datafonos"}

	if !total.EsTotalDelMes() {
		t.Error("la meta sin clasificación tiene que ser el total del mes")
	}
	if linea.EsTotalDelMes() {
		t.Error("la meta de una línea se tomó como el total del mes: sumarlas contaría dos veces")
	}
}
