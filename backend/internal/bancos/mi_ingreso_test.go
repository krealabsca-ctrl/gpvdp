package bancos

// «Mi presupuesto», mitad de INGRESO: el recorte por partida (mig 0125, 16-nov-2026).
//
// MISMA FUNCIÓN DE SEGURIDAD QUE EN EL GASTO, y mismo defecto a evitar: un recorte que falla
// entrega el ingreso de TODA la empresa con el rótulo de la partida de quien abrió la pantalla, y
// no da ningún error. Acá la plata ajena es grande: «Asociaciones / Cooperativas» sola mueve
// ₡251,4 M y «Depósito de Clientes» ₡90,3 M.
//
// Y LO SIN CLASIFICAR NO ES DE NADIE. Un ingreso que nadie clasificó es justo el que un equipo
// busca cuando espera un depósito; mostrárselo al primero que abra la pantalla sería divulgarlo.
// Es la misma decisión que ya se cerró en «Mi partida» el 23-set-2026.

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func controlIngresoDeMuestra() ControlIngreso {
	return ControlIngreso{
		Desde: "2026-10", Hasta: "2026-10",
		Lineas: []FilaIngreso{
			{ClasificacionID: "c-aso", Clasificacion: "Asociaciones / Cooperativas", Concepto: "Ingresos",
				Meta: "200000000.00", Real: "251438874.00", Movs: 649, CumplidoPct: "125.7"},
			{ClasificacionID: "c-dep", Clasificacion: "Deposito de Clientes", Concepto: "Ingresos",
				Meta: "", Real: "90283274.00", Movs: 7960},
			{ClasificacionID: "c-dat", Clasificacion: "Datafonos", Concepto: "Ingresos",
				Meta: "80000000.00", Real: "79606689.00", Movs: 1397, CumplidoPct: "99.5"},
			// El ingreso que nadie clasificó: sin id, y no es de nadie.
			{ClasificacionID: "", Clasificacion: "(sin clasificar)", Real: "141000000.00", Movs: 563},
		},
		RealTotal: "562328837.00", MetaTotal: "280000000.00",
		// El aviso que arma Finanzas sobre TODA la empresa: trae un conteo y un monto ajenos.
		Aviso: "El desglose por línea suma ₡45000000.00 MÁS que la meta del rango: una de las dos está mal. " +
			"17 línea(s) tuvieron ingreso sin meta propia: entran en el total pero no se juzgan.",
	}
}

// EL AVISO DE LA EMPRESA NO SE COPIA.
//
// Medido el 16-nov-2026 contra el sistema: al jefe de cobros de prueba, con 2 partidas en
// pantalla, le salía «17 línea(s) tuvieron ingreso sin meta propia». El 17 es de la empresa.
func TestMiIngresoNoCopiaElAvisoDeLaEmpresa(t *testing.T) {
	mi := RecortarAMisPartidas(controlIngresoDeMuestra(), []PartidaDelUsuario{
		{ClasificacionID: "c-dep"}, // sin meta
		{ClasificacionID: "c-dat"}, // con meta
	})

	if strings.Contains(mi.Aviso, "17") {
		t.Errorf("el aviso trae el conteo de la empresa: %q", mi.Aviso)
	}
	// Y tampoco el monto del descuadre global: es plata ajena.
	if strings.Contains(mi.Aviso, "45000000") {
		t.Errorf("el aviso trae el descuadre del presupuesto global: %q", mi.Aviso)
	}
	// Lo que sí tiene que decir es lo suyo: 1 de sus 2 partidas no tiene meta.
	if !strings.Contains(mi.Aviso, "1 de tus partidas") {
		t.Errorf("el aviso no cuenta SUS partidas sin meta: %q", mi.Aviso)
	}
}

// SIN NINGUNA META PROPIA, el aviso lo dice sin mencionar a la empresa.
func TestMiIngresoAvisaCuandoNingunaDeSusPartidasTieneMeta(t *testing.T) {
	mi := RecortarAMisPartidas(controlIngresoDeMuestra(), []PartidaDelUsuario{
		{ClasificacionID: "c-dep"}, // la única sin meta del control
	})
	if mi.MetaTotal != "0.00" {
		t.Fatalf("meta = %s; esta partida no tiene meta", mi.MetaTotal)
	}
	if !strings.Contains(mi.Aviso, "Ninguna de tus partidas") {
		t.Errorf("aviso = %q", mi.Aviso)
	}
}

// SIN LÍNEAS NO HAY AVISO: la pantalla ya lo dice en el lugar de la tabla, y decirlo dos veces
// hace ver un problema donde solo hay un mes sin movimiento.
func TestMiIngresoNoAvisaDosVecesQueNoHuboIngreso(t *testing.T) {
	mi := RecortarAMisPartidas(controlIngresoDeMuestra(), []PartidaDelUsuario{
		{ClasificacionID: "c-nueva", Clasificacion: "Reintegro Regalias"},
	})
	if len(mi.Lineas) != 0 {
		t.Fatalf("líneas = %d; esa partida no facturó", len(mi.Lineas))
	}
	if mi.Aviso != "" {
		t.Errorf("aviso = %q; sin líneas la pantalla ya lo dice en el estado vacío", mi.Aviso)
	}
}

// SOLO SALEN LAS PARTIDAS DE LA PERSONA.
func TestMiIngresoNoDejaPasarPartidasAjenas(t *testing.T) {
	mi := RecortarAMisPartidas(controlIngresoDeMuestra(), []PartidaDelUsuario{
		{ClasificacionID: "c-dat", Clasificacion: "Datafonos", Concepto: "Ingresos"},
	})

	if len(mi.Lineas) != 1 {
		t.Fatalf("líneas = %d; se esperaba solo Datáfonos. Las ajenas: %+v", len(mi.Lineas), mi.Lineas)
	}
	if mi.Lineas[0].Clasificacion != "Datafonos" {
		t.Errorf("salió %q", mi.Lineas[0].Clasificacion)
	}
	// Los totales son de SU partida: ₡79,6 M, no los ₡562,3 M de la empresa.
	if mi.RealTotal != "79606689.00" {
		t.Errorf("real = %s; se esperaba el de Datáfonos, no el de la empresa", mi.RealTotal)
	}
	if mi.MetaTotal != "80000000.00" {
		t.Errorf("meta = %s", mi.MetaTotal)
	}
}

// EL INGRESO SIN CLASIFICAR NO VIAJA, ni como línea ni dentro del total.
func TestMiIngresoNoMuestraLoSinClasificar(t *testing.T) {
	// Incluso pidiendo TODAS las partidas reales, la línea sin id queda afuera.
	mi := RecortarAMisPartidas(controlIngresoDeMuestra(), []PartidaDelUsuario{
		{ClasificacionID: "c-aso"}, {ClasificacionID: "c-dep"}, {ClasificacionID: "c-dat"},
	})
	for _, l := range mi.Lineas {
		if l.ClasificacionID == "" {
			t.Errorf("se coló el ingreso sin clasificar: %+v", l)
		}
	}
	// 251.438.874 + 90.283.274 + 79.606.689 = 421.328.837, SIN los 141 M sin clasificar.
	if mi.RealTotal != "421328837.00" {
		t.Errorf("real = %s; se esperaba 421328837.00 (sin lo que nadie clasificó)", mi.RealTotal)
	}
}

// POSITIVO ES BUENO: la diferencia se calcula real − meta, al revés del gasto.
func TestMiIngresoCalculaLaDiferenciaAFavor(t *testing.T) {
	mi := RecortarAMisPartidas(controlIngresoDeMuestra(), []PartidaDelUsuario{
		{ClasificacionID: "c-aso"},
	})
	// 251.438.874 − 200.000.000 = +51.438.874. Superó la meta.
	if mi.Diferencia != "51438874.00" {
		t.Errorf("diferencia = %s; se esperaba +51438874.00 (real − meta)", mi.Diferencia)
	}
}

// Una partida SIN meta se cuenta: sin meta no hay semáforo y hay que decirlo.
func TestMiIngresoCuentaLasPartidasSinMeta(t *testing.T) {
	mi := RecortarAMisPartidas(controlIngresoDeMuestra(), []PartidaDelUsuario{
		{ClasificacionID: "c-dep"}, // Depósito de Clientes no tiene meta
		{ClasificacionID: "c-dat"}, // Datáfonos sí
	})
	if mi.SinMeta != 1 {
		t.Errorf("sin_meta = %d; «Depósito de Clientes» no tiene meta propia", mi.SinMeta)
	}
	// Y la meta total solo suma lo que SÍ está declarado.
	if mi.MetaTotal != "80000000.00" {
		t.Errorf("meta = %s; la partida sin meta no aporta", mi.MetaTotal)
	}
}

// Las partidas viajan SIEMPRE, aunque no hayan facturado.
func TestMiIngresoNombraMisPartidasAunqueNoFacturaran(t *testing.T) {
	mi := RecortarAMisPartidas(controlIngresoDeMuestra(), []PartidaDelUsuario{
		{ClasificacionID: "c-nueva", Clasificacion: "Reintegro Regalias", Concepto: "Ingresos"},
	})
	if len(mi.Lineas) != 0 {
		t.Errorf("líneas = %d; esa partida no facturó", len(mi.Lineas))
	}
	if len(mi.Partidas) != 1 || mi.Partidas[0].Clasificacion != "Reintegro Regalias" {
		t.Errorf("las partidas no viajaron: %+v", mi.Partidas)
	}
	if mi.RealTotal != "0.00" {
		t.Errorf("real = %s; sin líneas tiene que ser 0,00 y no vacío", mi.RealTotal)
	}
}

// Con un alcance vacío NO sale ni una línea. Segunda guarda.
func TestMiIngresoConAlcanceVacioNoDevuelveNada(t *testing.T) {
	mi := RecortarAMisPartidas(controlIngresoDeMuestra(), nil)
	if len(mi.Lineas) != 0 {
		t.Fatalf("con alcance vacío salieron %d línea(s): el recorte se abrió en vez de cerrarse", len(mi.Lineas))
	}
	if mi.RealTotal != "0.00" {
		t.Errorf("real = %s con alcance vacío", mi.RealTotal)
	}
}

// SIN PARTIDAS, EL SERVICIO CORTA — y el doble SÍ tiene ingreso para dar.
func TestMiIngresoSinPartidasNoDevuelveNada(t *testing.T) {
	repo := &fakeRepo{
		// El doble TIENE ingreso. Si la guarda fallara, esto es lo que saldría.
		ingresoReal: []FilaIngreso{
			{ClasificacionID: "c-aso", Clasificacion: "Asociaciones / Cooperativas", Real: "251438874.00", Movs: 649},
			{ClasificacionID: "", Clasificacion: "(sin clasificar)", Real: "141000000.00", Movs: 563},
		},
		// Y sin partidas para el usuario: ese es el escenario.
		partidasDeIngresoDelUsuario: nil,
	}
	svc := NewService(repo, nil, zap.NewNop(), false)

	mi, err := svc.MiIngresoDePartidas(context.Background(), "e1", "u-sin-partidas", "2026-10", "2026-10")
	if err != ErrSinPartidasDeIngreso {
		t.Fatalf("err = %v; se esperaba ErrSinPartidasDeIngreso. Con la guarda rota, esta llamada "+
			"devuelve el ingreso de TODA la empresa", err)
	}
	if len(mi.Lineas) != 0 || mi.RealTotal != "" {
		t.Errorf("salió información igual: %+v", mi)
	}
}
