package bancos

// El calendario real de la empresa (16-nov-2026).
//
// LO QUE ESTO VIENE A ARREGLAR, medido contra el sistema: Memorial Pets tiene 21 meses cargados
// desde enero 2025, y la pantalla ofrecía 18 contados hacia atrás desde el reloj, así que enero a
// mayo 2025 eran inalcanzables. Un tope en el servidor repondría el mismo defecto un piso más
// abajo, donde además nadie lo ve.

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

// Los 21 meses de Memorial Pets, el caso que el cap de 18 cortaba.
// (`dosDigitos` ya vive en analisis_robusto_test.go.)
func veintiunMeses() []string {
	out := make([]string, 0, 21)
	for m := 1; m <= 12; m++ {
		out = append(out, "2025-"+dosDigitos(m))
	}
	for m := 1; m <= 9; m++ {
		out = append(out, "2026-"+dosDigitos(m))
	}
	return out
}

// NINGÚN MES SE PIERDE. Es la prueba del defecto reportado.
func TestPeriodosNoRecortaLosMesesCargados(t *testing.T) {
	svc := NewService(&fakeRepo{periodosConMovimiento: veintiunMeses()}, nil, zap.NewNop(), false)

	p, err := svc.Periodos(context.Background(), "e1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(p.ConMovimiento) != 21 {
		t.Fatalf("meses = %d; se esperaban los 21 cargados. Con un tope de 18, enero a mayo 2025 "+
			"quedan afuera y no hay forma de llegar a ellos desde la pantalla", len(p.ConMovimiento))
	}
	// El más viejo es el que el cap escondía.
	if p.Primero != "2025-01" {
		t.Errorf("primero = %q; se esperaba 2025-01", p.Primero)
	}
	if p.Ultimo != "2026-09" {
		t.Errorf("último = %q; se esperaba 2026-09", p.Ultimo)
	}
}

// UNA EMPRESA SIN NADA CARGADO no rompe el selector: devuelve vacío, no un error.
//
// Es el estado de una instalación nueva, y el selector de período está en la barra de todas las
// pantallas: un error acá dejaría el sistema sin barra desde el primer arranque.
func TestPeriodosDeUnaEmpresaVaciaNoFalla(t *testing.T) {
	svc := NewService(&fakeRepo{}, nil, zap.NewNop(), false)

	p, err := svc.Periodos(context.Background(), "e-nueva")
	if err != nil {
		t.Fatalf("err = %v; una empresa sin movimientos no es un error", err)
	}
	if len(p.ConMovimiento) != 0 || p.Primero != "" || p.Ultimo != "" {
		t.Errorf("se esperaba vacío: %+v", p)
	}
}

// LOS HUECOS SE CONSERVAN. Un mes sin cargar es información, no algo que rellenar.
//
// Si el servicio completara los meses faltantes, la parrilla de Exportar ofrecería un mes que da
// un Excel vacío, que es justo lo que esta pantalla evita.
func TestPeriodosConservaLosHuecos(t *testing.T) {
	// Falta marzo: se cargó enero, febrero y abril.
	svc := NewService(&fakeRepo{periodosConMovimiento: []string{"2026-01", "2026-02", "2026-04"}},
		nil, zap.NewNop(), false)

	p, err := svc.Periodos(context.Background(), "e1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(p.ConMovimiento) != 3 {
		t.Fatalf("meses = %v; el hueco de marzo no se rellena", p.ConMovimiento)
	}
	for _, m := range p.ConMovimiento {
		if m == "2026-03" {
			t.Errorf("se inventó marzo, que no tiene movimiento")
		}
	}
	// Y los extremos siguen siendo los reales, con hueco y todo.
	if p.Primero != "2026-01" || p.Ultimo != "2026-04" {
		t.Errorf("extremos = %q..%q", p.Primero, p.Ultimo)
	}
}

// UN SOLO MES: primero y último son el mismo, y no se cae.
func TestPeriodosConUnSoloMes(t *testing.T) {
	svc := NewService(&fakeRepo{periodosConMovimiento: []string{"2026-07"}}, nil, zap.NewNop(), false)

	p, err := svc.Periodos(context.Background(), "e1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if p.Primero != "2026-07" || p.Ultimo != "2026-07" {
		t.Errorf("extremos = %q..%q; con un solo mes los dos son ese", p.Primero, p.Ultimo)
	}
}
