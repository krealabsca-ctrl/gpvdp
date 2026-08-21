package cxp

// Tests de los helpers puros del tablero: el período que manda sobre el MOVIMIENTO (el
// selector global de la barra) y la serie de meses de la gráfica.

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

func TestPeriodoValido(t *testing.T) {
	casos := []struct {
		periodo string
		quiere  bool
	}{
		{"2026-07", true},
		{"2026-01", true},
		{"2026-12", true},
		{"2026-13", false}, // mes fuera de rango
		{"2026-00", false},
		{"2026-7", false},  // sin cero a la izquierda
		{"2026", false},    // incompleto
		{"julio", false},   // basura
		{"", false},        // vacío: el servicio lo resuelve al mes en curso antes de validar
		{"1999-12", false}, // año irreal
		{"2101-01", false}, // año irreal
		{"2026-07-15", false},
	}
	for _, c := range casos {
		if got := periodoValido(c.periodo); got != c.quiere {
			t.Errorf("periodoValido(%q) = %v, quiere %v", c.periodo, got, c.quiere)
		}
	}
}

func TestUltimosPeriodos(t *testing.T) {
	t.Run("cruza el año hacia atrás", func(t *testing.T) {
		got, err := ultimosPeriodos("2026-02", 7)
		if err != nil {
			t.Fatalf("ultimosPeriodos: %v", err)
		}
		quiere := []string{"2025-08", "2025-09", "2025-10", "2025-11", "2025-12", "2026-01", "2026-02"}
		if len(got) != len(quiere) {
			t.Fatalf("largo = %d, quiere %d (%v)", len(got), len(quiere), got)
		}
		for i := range quiere {
			if got[i] != quiere[i] {
				t.Errorf("posición %d = %q, quiere %q", i, got[i], quiere[i])
			}
		}
	})

	t.Run("el último siempre es el período pedido", func(t *testing.T) {
		got, err := ultimosPeriodos("2026-07", 7)
		if err != nil {
			t.Fatalf("ultimosPeriodos: %v", err)
		}
		if got[len(got)-1] != "2026-07" {
			t.Errorf("último = %q, quiere 2026-07", got[len(got)-1])
		}
		if got[0] != "2026-01" {
			t.Errorf("primero = %q, quiere 2026-01", got[0])
		}
	})

	t.Run("n inválido devuelve solo el período", func(t *testing.T) {
		got, err := ultimosPeriodos("2026-07", 0)
		if err != nil {
			t.Fatalf("ultimosPeriodos: %v", err)
		}
		if len(got) != 1 || got[0] != "2026-07" {
			t.Errorf("got = %v, quiere [2026-07]", got)
		}
	})

	t.Run("período basura da error tipado", func(t *testing.T) {
		if _, err := ultimosPeriodos("nada", 6); !errors.Is(err, ErrPeriodoInvalido) {
			t.Errorf("err = %v, quiere ErrPeriodoInvalido", err)
		}
	})
}

// El servicio propaga el período al repositorio, resuelve el vacío al mes en curso y
// rechaza la basura antes de tocar la base.
func TestServicioDashboardPeriodo(t *testing.T) {
	svc := NewService(&fakeRepo{}, nil, zap.NewNop())
	ctx := context.Background()

	d, err := svc.Dashboard(ctx, "e1", "2026-06", "ADMIN", "u1")
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if d.Periodo != "2026-06" {
		t.Errorf("periodo = %q, quiere 2026-06 (el selector de la barra debe llegar al repo)", d.Periodo)
	}
	if d.Cola == nil {
		t.Error("cola = nil; debe ser [] para que el JSON no lleve null")
	}

	// Sin período: el mes en curso de Costa Rica, no un error.
	d, err = svc.Dashboard(ctx, "e1", "", "ADMIN", "u1")
	if err != nil {
		t.Fatalf("dashboard sin período: %v", err)
	}
	if d.Periodo != PeriodoActualCR() {
		t.Errorf("periodo por defecto = %q, quiere %q", d.Periodo, PeriodoActualCR())
	}

	if _, err := svc.Dashboard(ctx, "e1", "2026-13", "ADMIN", "u1"); !errors.Is(err, ErrPeriodoInvalido) {
		t.Errorf("período inválido: err = %v, quiere ErrPeriodoInvalido", err)
	}
}

// El filtro de vencimiento del listado usa una LISTA CERRADA: nada de lo que escriba el
// usuario llega al SQL, y cada tramo del tablero tiene su condición.
func TestCondVencimiento(t *testing.T) {
	tramos := []string{"vencido", TramoV90, TramoV61, TramoV31, TramoV1, TramoSemana, TramoMes, TramoFuturo, TramoSinFecha}
	vistas := map[string]string{}
	for _, tr := range tramos {
		cond := condVencimiento(tr)
		if cond == "" {
			t.Errorf("condVencimiento(%q) vino vacía; el tramo del tablero no sería navegable", tr)
			continue
		}
		if otro, dup := vistas[cond]; dup {
			t.Errorf("condVencimiento(%q) repite la condición de %q", tr, otro)
		}
		vistas[cond] = tr
	}
	// Cualquier otra cosa (incluido un intento de inyección) no produce filtro.
	for _, basura := range []string{"", "todos", "v9", "1; DROP TABLE documento_cxp", "' OR 1=1 --"} {
		if cond := condVencimiento(basura); cond != "" {
			t.Errorf("condVencimiento(%q) = %q, quiere cadena vacía", basura, cond)
		}
	}
}

func TestPeriodoActualCR(t *testing.T) {
	// No se puede fijar el reloj, pero el formato y la validez sí se exigen: el default del
	// endpoint nunca puede producir un período que su propia validación rechace.
	p := PeriodoActualCR()
	if !periodoValido(p) {
		t.Fatalf("PeriodoActualCR() = %q, no pasa periodoValido", p)
	}
}
