package cxc

import "testing"

// El plan de cargos tiene que CONFESAR los contratos que no van a generar nada.
//
// Historia del defecto: la consulta descartaba en el WHERE los contratos en revisión, con cuota en
// cero o sin modalidad, así que nunca llegaban al servicio y el mapa `Excluidos` —que existe
// justo para que «un contrato que nunca cobra no pase inadvertido»— salía SIEMPRE vacío. Con la
// carga real de Coopeprofa (12.231 contratos), la vista previa decía «9.779 contratos» y no
// mencionaba los 2.451 apartados. El usuario reportó «muchos contratos salen sin cuotas» porque el
// sistema no tenía forma de decírselo.
func TestPlanDeExcluyeConElMotivoDelSQL(t *testing.T) {
	casos := []struct {
		nombre string
		motivo string
	}{
		{"en revisión", "en revisión: el dato del origen quedó incompleto"},
		{"cuota en cero", "la cuota está en cero"},
		{"sin modalidad", "sin modalidad de pago en el catálogo"},
		{"sin primer cobro", "sin fecha de primer cobro"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			// Un contrato que por lo demás generaría de sobra: lo único que lo detiene es el motivo.
			g := ContratoGenerable{
				Numero: "CO-1", PrimerCobro: "2026-01-01", DiaPago: 1,
				Cuota: d("5000"), MesesCiclo: 1, Motivo: c.motivo,
			}
			cargos, motivo := planDe(g, f("2026-08-01"), f("2026-09-30"))
			if motivo != c.motivo {
				t.Errorf("motivo = %q, quiere %q", motivo, c.motivo)
			}
			if len(cargos) != 0 {
				t.Errorf("un contrato excluido no puede generar cargos, generó %d", len(cargos))
			}
		})
	}
}

// Sin motivo, el contrato genera normalmente: la guarda nueva no puede haber apagado el generador.
func TestPlanDeSinMotivoSigueGenerando(t *testing.T) {
	g := ContratoGenerable{
		Numero: "CO-2", PrimerCobro: "2026-01-01", DiaPago: 1,
		Cuota: d("5000"), MesesCiclo: 1,
	}
	cargos, motivo := planDe(g, f("2026-08-01"), f("2026-09-30"))
	if motivo != "" {
		t.Fatalf("motivo inesperado: %q", motivo)
	}
	if len(cargos) != 2 { // agosto y septiembre
		t.Errorf("cargos = %d, quiere 2", len(cargos))
	}
}

// Un contrato sano cuyo primer cobro cae DESPUÉS del rango no es un error: simplemente no le toca
// cobrar todavía. Se cuenta aparte (FueraDelRango) y no como excluido, pero se cuenta: es el caso
// real de CO24763 (primer cobro 2026-10-01), el único de los 12.231 que no cerraba la resta.
func TestContratoPosteriorAlRangoNoEsExcluido(t *testing.T) {
	g := ContratoGenerable{
		Numero: "CO24763", PrimerCobro: "2026-10-01", DiaPago: 1,
		Cuota: d("5000"), MesesCiclo: 1,
	}
	cargos, motivo := planDe(g, f("2026-08-01"), f("2026-09-30"))
	if motivo != "" {
		t.Errorf("no le toca cobrar en el rango; eso no es un motivo de exclusión: %q", motivo)
	}
	if len(cargos) != 0 {
		t.Errorf("no debería generar nada en el rango, generó %d", len(cargos))
	}
}
