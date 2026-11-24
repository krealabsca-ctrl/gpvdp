package bancos

// El detalle de «Mi presupuesto» y «Mi ingreso» (17-nov-2026).
//
// EL RIESGO NUEVO: hasta acá, el alcance de estas pantallas salía ENTERO del token y la persona no
// elegía nada. El detalle cambia eso —el área a mirar viaja en el pedido—, así que aparece un
// camino que antes no existía: cambiar un id en la barra de direcciones y pedir el desglose del
// área de otro. La comprobación de que el área es suya es lo único que lo cierra.

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

// ── La guarda del área pedida ────────────────────────────────────────────────────────────────

// PEDIR EL ÁREA DE OTRO NO DEVUELVE NADA, aunque la persona sí tenga alcance.
func TestNoSeVeElDesgloseDeUnAreaAjena(t *testing.T) {
	repo := &fakeRepo{
		// Es jefe de Mercadeo...
		areasDelUsuario: []AreaDelUsuario{{DepartamentoID: "d-mercadeo", Departamento: "Mercadeo", Rol: AreaTitular}},
	}
	svc := NewService(repo, nil, zap.NewNop(), false)

	// ...y pide el desglose de Gerencia.
	_, err := svc.MisPartidasDeArea(context.Background(), "e1", "u1", "2026-01", "2026-10",
		"d-gerencia", "")
	if err != ErrAreaFueraDeMiAlcance {
		t.Fatalf("err = %v; se esperaba ErrAreaFueraDeMiAlcance. Con la guarda rota, cualquier jefe "+
			"lee el gasto de cualquier área cambiando un parámetro", err)
	}
}

// UN ÁREA VACÍA NO VALE COMO «TODAS».
//
// Es el descuido clásico: omitir el parámetro y que el filtro se abra. Devolvería el desglose de
// la empresa entera, que es exactamente lo que esta pantalla no muestra.
func TestElAreaVaciaNoAbreElDesgloseDeTodo(t *testing.T) {
	repo := &fakeRepo{
		areasDelUsuario: []AreaDelUsuario{{DepartamentoID: "d-mercadeo", Departamento: "Mercadeo"}},
	}
	svc := NewService(repo, nil, zap.NewNop(), false)

	if _, err := svc.MisPartidasDeArea(context.Background(), "e1", "u1", "2026-01", "2026-10",
		"", ""); err != ErrAreaFueraDeMiAlcance {
		t.Fatalf("err = %v; un área vacía tiene que cerrar, no abrir", err)
	}
}

// SIN ALCANCE CORTA ANTES, sin siquiera mirar qué área pidió.
func TestSinAreasNoHayDesglose(t *testing.T) {
	svc := NewService(&fakeRepo{}, nil, zap.NewNop(), false)

	if _, err := svc.MisPartidasDeArea(context.Background(), "e1", "u1", "2026-01", "2026-10",
		"d-cualquiera", ""); err != ErrSinAlcanceDePresupuesto {
		t.Fatalf("err = %v", err)
	}
}

// Y LA PROPIA SÍ PASA la guarda (el cálculo de abajo ya tiene sus propias pruebas).
func TestElAreaPropiaPasaLaGuarda(t *testing.T) {
	if !EsAreaDeLaPersona(
		[]AreaDelUsuario{{DepartamentoID: "d-a"}, {DepartamentoID: "d-b"}}, "d-b") {
		t.Error("el área propia no pasó la guarda")
	}
}

// ── El recorte de la serie de ingreso ────────────────────────────────────────────────────────

func seriesDeMuestra() []TendenciaPartida {
	return []TendenciaPartida{
		{ClasificacionID: "c-aso", Clasificacion: "Asociaciones / Cooperativas", Total: "251438874.00",
			Serie: []PuntoPartida{{Periodo: "2026-08", Monto: "125000000.00", Movs: 300}}},
		{ClasificacionID: "c-dat", Clasificacion: "Datafonos", Total: "79606689.00",
			Serie: []PuntoPartida{{Periodo: "2026-08", Monto: "40000000.00", Movs: 700}}},
		// Lo que nadie clasificó: sin id.
		{ClasificacionID: "", Clasificacion: "(sin clasificar)", Total: "141000000.00"},
	}
}

// SOLO LAS SERIES DE SUS PARTIDAS.
func TestLaSerieNoDejaPasarPartidasAjenas(t *testing.T) {
	out := RecortarSerieAMisPartidas(seriesDeMuestra(), []PartidaDelUsuario{{ClasificacionID: "c-dat"}})

	if len(out) != 1 {
		t.Fatalf("series = %d; se esperaba solo Datáfonos. Las ajenas: %+v", len(out), out)
	}
	if out[0].Clasificacion != "Datafonos" {
		t.Errorf("salió %q", out[0].Clasificacion)
	}
}

// LO SIN CLASIFICAR NO VIAJA, ni pidiendo todas las partidas reales.
func TestLaSerieNoMuestraLoSinClasificar(t *testing.T) {
	out := RecortarSerieAMisPartidas(seriesDeMuestra(), []PartidaDelUsuario{
		{ClasificacionID: "c-aso"}, {ClasificacionID: "c-dat"},
	})
	for _, s := range out {
		if s.ClasificacionID == "" {
			t.Errorf("se coló la serie de lo sin clasificar: %+v", s)
		}
	}
	if len(out) != 2 {
		t.Errorf("series = %d; se esperaban 2", len(out))
	}
}

// CON ALCANCE VACÍO NO SALE NI UNA SERIE.
func TestLaSerieConAlcanceVacioNoDevuelveNada(t *testing.T) {
	if out := RecortarSerieAMisPartidas(seriesDeMuestra(), nil); len(out) != 0 {
		t.Fatalf("salieron %d serie(s): el recorte se abrió en vez de cerrarse", len(out))
	}
}

// SIN PARTIDAS, EL SERVICIO CORTA — y el doble SÍ tiene series para dar.
func TestLaSerieSinPartidasNoDevuelveNada(t *testing.T) {
	repo := &fakeRepo{
		seriePartidas:               seriesDeMuestra(), // si la guarda fallara, esto es lo que saldría
		partidasDeIngresoDelUsuario: nil,
	}
	svc := NewService(repo, nil, zap.NewNop(), false)

	res, err := svc.MiSerieDeIngreso(context.Background(), "e1", "u-sin-partidas", "2026-01", "2026-10")
	if err != ErrSinPartidasDeIngreso {
		t.Fatalf("err = %v; se esperaba ErrSinPartidasDeIngreso. Con la guarda rota, esta llamada "+
			"devuelve la serie de TODA la empresa", err)
	}
	if len(res.Partidas) != 0 {
		t.Errorf("salió información igual: %+v", res.Partidas)
	}
}
