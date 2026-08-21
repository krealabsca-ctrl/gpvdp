package cxp

// Validación de área POR RIESGO: quién pasa derecho a aprobación y quién espera al área.

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func boolPtr(b bool) *bool { return &b }

// La regla nueva, en una tabla: desde REVISADO, aprobar depende del veredicto de riesgo.
func TestAprobarSegunRiesgo(t *testing.T) {
	ctx := context.Background()
	casos := []struct {
		nombre   string
		requiere *bool
		quiere   error
	}{
		{"sin riesgo: pasa derecho a aprobación", boolPtr(false), nil},
		{"con riesgo: tiene que esperar al área", boolPtr(true), ErrTransicionInvalida},
		// Sin evaluar es el caso conservador: mejor que alguien la mire a que se pague sin
		// que nadie la haya decidido. Cubre los documentos anteriores a la regla.
		{"sin evaluar: se comporta como si requiriera validación", nil, ErrTransicionInvalida},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo := &fakeRepo{
				docs: map[string]Documento{"fac": {
					ID: "fac", Tipo: TipoCxP, Estado: EstRevisado,
					TotalCRC: "500000.00", NetoCRC: "500000.00",
					RequiereValidacion: c.requiere,
				}},
				rolesAprobaciones: []string{"DIRECTOR_FINANCIERO"}, // ≤₡1M ⇒ 1 firma completa
			}
			svc := NewService(repo, nil, zap.NewNop())
			_, err := svc.Aprobar(ctx, "emp", "fac", "u1", "DIRECTOR_FINANCIERO")
			if err != c.quiere {
				t.Fatalf("Aprobar => %v, quiere %v", err, c.quiere)
			}
			if c.quiere == nil && repo.capMultiA != EstAprobado {
				t.Errorf("debió transicionar a APROBADO, fue %q", repo.capMultiA)
			}
		})
	}
}

// Revisar evalúa el riesgo: es el momento en que Contabilidad ya le puso concepto y departamento.
func TestRevisarEvaluaElRiesgo(t *testing.T) {
	repo := &fakeRepo{
		doc:         Documento{ID: "fac", Tipo: TipoCxP, Estado: EstRecibido, ConceptoID: "c1", ClasificacionID: "cl1"},
		filasCambio: 1,
		// El repo devuelve que disparó el criterio de monto.
		motivoValidacion: MotivoMonto,
	}
	if _, err := NewService(repo, nil, zap.NewNop()).Revisar(context.Background(), "emp", "fac", "u1"); err != nil {
		t.Fatalf("Revisar => %v", err)
	}
	if repo.capA != EstRevisado {
		t.Errorf("debió pasar a REVISADO, fue %q", repo.capA)
	}
}

// Los umbrales: clave del catálogo y valor numérico no negativo. Nada más entra.
func TestGuardarParametroValidacionGuards(t *testing.T) {
	ctx := context.Background()
	casos := []struct {
		nombre, clave, valor string
		quiere               error
	}{
		{"clave y valor válidos", "VALIDACION_UMBRAL_MONTO", "250000", nil},
		{"la clave se normaliza a mayúsculas", "validacion_umbral_monto", "250000", nil},
		{"decimal válido", "VALIDACION_DESVIO_PCT", "12.5", nil},
		{"clave inventada", "AHORRO_CCSS", "1", ErrParametroInvalido},
		{"valor negativo", "VALIDACION_UMBRAL_MONTO", "-1", ErrParametroInvalido},
		{"valor no numérico", "VALIDACION_UMBRAL_MONTO", "mucho", ErrParametroInvalido},
		{"valor vacío", "VALIDACION_UMBRAL_MONTO", "  ", ErrParametroInvalido},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo := &fakeRepo{}
			err := NewService(repo, nil, zap.NewNop()).
				GuardarParametroValidacion(ctx, "emp", c.clave, c.valor, "u1")
			if err != c.quiere {
				t.Fatalf("GuardarParametroValidacion(%q, %q) => %v, quiere %v", c.clave, c.valor, err, c.quiere)
			}
		})
	}

	// Una clave válida que el UPDATE no encuentra (empresa sin ese parámetro) NO es un éxito
	// silencioso: si no se guardó, la pantalla tiene que enterarse.
	repo := &fakeRepo{paramFilasCero: true}
	err := NewService(repo, nil, zap.NewNop()).
		GuardarParametroValidacion(ctx, "emp", "VALIDACION_UMBRAL_MONTO", "1000", "u1")
	if err != ErrParametroInvalido {
		t.Errorf("0 filas afectadas => %v, quiere ErrParametroInvalido", err)
	}
}
