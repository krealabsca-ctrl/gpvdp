package bancos

// La guarda del alcance vacío, en el servicio (mig 0124).
//
// ES LA PRIMERA DE LAS DOS, y existe por un defecto concreto que ya mordió en este sistema: se
// arma el filtro con `if len(ids) > 0`, la lista viene vacía, la condición no se agrega y el
// resultado queda ABIERTO. No falla ruidosamente —entrega los datos de toda la empresa— y por eso
// la prueba SIEMBRA GASTO EN EL DOBLE: comprobar que «no devuelve nada» contra un repositorio que
// tampoco tiene nada no prueba absolutamente nada.

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

// SIN ÁREAS, NO SALE NI UN NÚMERO — y el repositorio sí tiene gasto para dar.
func TestMiPresupuestoSinAlcanceNoDevuelveNada(t *testing.T) {
	repo := &fakeRepo{
		// El doble TIENE gasto de tres áreas. Si la guarda fallara, esto es lo que saldría.
		gastoDim: []GastoDimension{
			{ID: "d-ope", Nombre: "Operaciones", Gasto: "48115442.00", Movs: 104},
			{ID: "d-fin", Nombre: "Finanzas", Gasto: "119414681.00", Movs: 239},
			{ID: "", Nombre: "(sin asignar)", Gasto: "127526463.00", Movs: 651},
		},
		// Y sin áreas para el usuario: ese es el escenario.
		areasDelUsuario: nil,
	}
	svc := NewService(repo, nil, zap.NewNop(), false)

	mp, err := svc.MiPresupuestoDeArea(context.Background(), "e1", "u-sin-areas", "2026-10", "2026-10", "")
	if !errors.Is(err, ErrSinAlcanceDePresupuesto) {
		t.Fatalf("err = %v; se esperaba ErrSinAlcanceDePresupuesto. Con la guarda rota, esta "+
			"llamada devuelve el gasto de TODA la empresa", err)
	}
	if len(mp.Filas) != 0 || mp.TotalGasto != "" {
		t.Errorf("salió información igual: %+v", mp)
	}
}

// El mensaje dice QUÉ HACER y a quién pedírselo, no solo que no hay nada.
func TestElErrorDeAlcanceDicePedirselo(t *testing.T) {
	msg := ErrSinAlcanceDePresupuesto.Error()
	for _, pista := range []string{"responsable", "administra"} {
		if !contiene(msg, pista) {
			t.Errorf("%q no menciona %q: quien lo lea no sabe a quién pedirle el acceso", msg, pista)
		}
	}
}

func contiene(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
