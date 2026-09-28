package cxp

import (
	"os"
	"strings"
	"testing"
)

// UltimaFechaBanco es lo que separa «esto está VENCIDA» de «SIN DATO, todavía no puedo saberlo» en
// el calendario de obligaciones. Si cuenta movimientos excluidos, una importación duplicada ya
// revertida sostendría sola la afirmación «el banco llega hasta el 14» y el semáforo daría por
// vencidas responsabilidades que en realidad todavía no se pueden juzgar.
//
// Su gemela es bancos.UltimaFechaCargada, que sí filtra. Las dos contestan la misma pregunta: o
// filtran las dos o el mismo concepto da dos respuestas distintas según quién pregunte.
//
// La guarda lee el SQL porque lo que hay que fijar es una condición DENTRO de la consulta, no un
// valor de retorno. Mismo criterio que grupo/incluido_test.go y seed/fuente_test.go.
func TestUltimaFechaBancoSoloMiraLoIncluido(t *testing.T) {
	b, err := os.ReadFile("repository_responsabilidad.go")
	if err != nil {
		t.Fatalf("leer repository_responsabilidad.go: %v", err)
	}
	src := string(b)

	const firma = "func (r *pgRepository) UltimaFechaBanco("
	i := strings.Index(src, firma)
	if i < 0 {
		t.Fatalf("no se encontró %s", firma)
	}
	cuerpo := src[i:]
	if j := strings.Index(cuerpo, "\nfunc "); j > 0 {
		cuerpo = cuerpo[:j]
	}

	if !strings.Contains(cuerpo, "FROM movimiento_bancario") {
		t.Fatal("cambió la forma de la consulta: ya no se reconoce el FROM movimiento_bancario")
	}
	if !strings.Contains(cuerpo, "AND incluido") {
		t.Error("UltimaFechaBanco no filtra `incluido`: una importación duplicada revertida seguiría " +
			"sosteniendo la fecha hasta la que llega el banco, y el semáforo marcaría VENCIDA una " +
			"responsabilidad que todavía no se puede juzgar")
	}
}
