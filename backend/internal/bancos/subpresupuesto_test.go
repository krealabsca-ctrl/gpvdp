package bancos

// Pruebas del subpresupuesto por partida y de la administración del catálogo de departamentos.
//
// Lo que más importa acá no es que los números salgan: es que las DOS CLASES de línea de presupuesto
// no se mezclen. El total del departamento y el desglose por partida viven en la misma tabla, y
// sumarlos juntos haría aparecer presupuesto que nadie autorizó.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestElTotalYLosSubpresupuestosNoSeSuman(t *testing.T) {
	t.Parallel()
	// Logística tiene ₡2.000.000 autorizados y los reparte en dos partidas. El presupuesto del
	// departamento sigue siendo 2.000.000, NO 3.500.000.
	repo := &fakeRepo{
		gastoDim: []GastoDimension{{ID: "d1", Nombre: "Logística", Movs: 10, Gasto: "1500000.00"}},
		presupuesto: []PresupuestoLinea{
			{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-08", Monto: "2000000.00"},
			{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-08", Monto: "1200000.00",
				ClasificacionID: "cl-comb", Clasificacion: "Combustible"},
			{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-08", Monto: "300000.00",
				ClasificacionID: "cl-mant", Clasificacion: "Mantenimiento"},
		},
	}
	res, err := servicioControl(repo).ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "")
	if err != nil {
		t.Fatalf("ControlPresupuestario: %v", err)
	}
	if len(res.Filas) != 1 {
		t.Fatalf("filas = %d, se esperaba 1", len(res.Filas))
	}
	f := res.Filas[0]
	if f.Presupuesto != "2000000.00" {
		t.Errorf("presupuesto = %q, se esperaba 2000000.00 (el total, no la suma con el desglose)", f.Presupuesto)
	}
	if res.TotalPresupuesto != "2000000.00" {
		t.Errorf("total presupuesto = %q, se esperaba 2000000.00", res.TotalPresupuesto)
	}
	if f.Subpresupuestos != 2 {
		t.Errorf("subpresupuestos = %d, se esperaban 2", f.Subpresupuestos)
	}
	if f.SumaSubpresupuestos != "1500000.00" {
		t.Errorf("suma de subpresupuestos = %q, se esperaba 1500000.00", f.SumaSubpresupuestos)
	}
	// 2.000.000 − 1.500.000 = 500.000 todavía sin repartir por partida.
	if f.SinRepartir != "500000.00" {
		t.Errorf("sin repartir = %q, se esperaba 500000.00", f.SinRepartir)
	}
	// Y el semáforo del departamento se calcula contra su TOTAL: 1.500.000 de 2.000.000 = 75 %.
	if f.ConsumidoPct != "75.0" || f.Estado != CtrlEnRango {
		t.Errorf("consumido = %q estado = %q, se esperaba 75.0 / EN_RANGO", f.ConsumidoPct, f.Estado)
	}
}

func TestUnaMismaPartidaEnVariosMesesEsUnSoloSubpresupuesto(t *testing.T) {
	t.Parallel()
	// Contar líneas en vez de partidas diría «3 subpresupuestos» cuando hay uno solo cargado en tres
	// meses del rango, y el usuario buscaría dos partidas que no existen.
	repo := &fakeRepo{
		gastoDim: []GastoDimension{{ID: "d1", Nombre: "Logística", Movs: 3, Gasto: "100.00"}},
		presupuesto: []PresupuestoLinea{
			{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-06", Monto: "1000.00"},
			{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-07", Monto: "1000.00"},
			{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-08", Monto: "1000.00"},
			{DepartamentoID: "d1", Periodo: "2026-06", Monto: "400.00", ClasificacionID: "cl-1", Clasificacion: "Combustible"},
			{DepartamentoID: "d1", Periodo: "2026-07", Monto: "400.00", ClasificacionID: "cl-1", Clasificacion: "Combustible"},
			{DepartamentoID: "d1", Periodo: "2026-08", Monto: "400.00", ClasificacionID: "cl-1", Clasificacion: "Combustible"},
		},
	}
	res, err := servicioControl(repo).ControlPresupuestario(context.Background(), "emp", "2026-06", "2026-08", AgruparPorDepartamento, "")
	if err != nil {
		t.Fatalf("ControlPresupuestario: %v", err)
	}
	f := res.Filas[0]
	if f.Subpresupuestos != 1 {
		t.Errorf("subpresupuestos = %d, se esperaba 1 (una partida en tres meses)", f.Subpresupuestos)
	}
	// Los MONTOS sí se suman por los tres meses: 400 × 3.
	if f.SumaSubpresupuestos != "1200.00" {
		t.Errorf("suma = %q, se esperaba 1200.00", f.SumaSubpresupuestos)
	}
	if f.MesesConPresupuesto != 3 {
		t.Errorf("meses con presupuesto = %d, se esperaban 3", f.MesesConPresupuesto)
	}
}

func TestElDesgloseQueRepartemMasDeLoAutorizadoSeAvisa(t *testing.T) {
	t.Parallel()
	// El departamento está EN VERDE (gastó poco) y sin embargo su presupuesto está mal repartido: las
	// partidas suman más de lo autorizado. Sin este aviso, el problema aparece recién cuando la última
	// partida se queda sin plata.
	repo := &fakeRepo{
		gastoDim: []GastoDimension{{ID: "d1", Nombre: "Logística", Movs: 2, Gasto: "100000.00"}},
		presupuesto: []PresupuestoLinea{
			{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-08", Monto: "1000000.00"},
			{DepartamentoID: "d1", Periodo: "2026-08", Monto: "800000.00", ClasificacionID: "cl-1"},
			{DepartamentoID: "d1", Periodo: "2026-08", Monto: "700000.00", ClasificacionID: "cl-2"},
		},
	}
	res, err := servicioControl(repo).ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "")
	if err != nil {
		t.Fatalf("ControlPresupuestario: %v", err)
	}
	if res.Filas[0].Estado != CtrlEnRango {
		t.Fatalf("el departamento gastó el 10 %%: estado = %q", res.Filas[0].Estado)
	}
	if res.Filas[0].SinRepartir != "-500000.00" {
		t.Errorf("sin repartir = %q, se esperaba -500000.00", res.Filas[0].SinRepartir)
	}
	if !strings.Contains(res.Aviso, "reparten MÁS de lo autorizado") {
		t.Errorf("el aviso no menciona el sobregiro del desglose: %q", res.Aviso)
	}
}

func TestSemaforoPorPartidaUsaSuPropioSubpresupuesto(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{partidasDim: []GastoPartidaDimension{
		// Excedida: gastó 2.300.000 de 2.000.000.
		{ClasificacionID: "cl-1", Clasificacion: "Combustible", Gasto: "2300000.00", Subpresupuesto: "2000000.00"},
		// En alerta: 920.000 de 1.000.000 = 92 % (umbral 90).
		{ClasificacionID: "cl-2", Clasificacion: "Mantenimiento", Gasto: "920000.00", Subpresupuesto: "1000000.00"},
		// En rango: la mitad.
		{ClasificacionID: "cl-3", Clasificacion: "Viáticos", Gasto: "250000.00", Subpresupuesto: "500000.00"},
		// Sin subpresupuesto definido: NO es «en rango», es que no hay con qué comparar.
		{ClasificacionID: "cl-4", Clasificacion: "Peajes", Gasto: "40000.00", Subpresupuesto: "0.00"},
	}}
	filas, err := servicioControl(repo).PartidasDeDimension(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "d1", "")
	if err != nil {
		t.Fatalf("PartidasDeDimension: %v", err)
	}
	esperado := []string{CtrlExcedido, CtrlAlerta, CtrlEnRango, CtrlSinPresupuesto}
	for i, e := range esperado {
		if filas[i].Estado != e {
			t.Errorf("%s: estado = %q, se esperaba %q", filas[i].Clasificacion, filas[i].Estado, e)
		}
	}
	if filas[0].Disponible != "-300000.00" {
		t.Errorf("disponible de la excedida = %q, se esperaba -300000.00", filas[0].Disponible)
	}
	if filas[1].ConsumidoPct != "92.0" {
		t.Errorf("consumido de la de alerta = %q, se esperaba 92.0", filas[1].ConsumidoPct)
	}
	// La partida sin subpresupuesto no debe mostrar un "0.00" que se lea como «tiene cero autorizado».
	if filas[3].Subpresupuesto != "" || filas[3].ConsumidoPct != "" || filas[3].Disponible != "" {
		t.Errorf("la partida sin subpresupuesto trae números inventados: %+v", filas[3])
	}
}

func TestElUmbralEsElMismoEnElDepartamentoYEnLaPartida(t *testing.T) {
	t.Parallel()
	// Dos umbrales distintos harían que la misma plata estuviera «en rango» en una pantalla y «en
	// alerta» en la otra. Con umbral 80, el 85 % es ALERTA en las dos.
	repoDep := &fakeRepo{
		gastoDim:    []GastoDimension{{ID: "d1", Nombre: "Logística", Movs: 1, Gasto: "850.00"}},
		presupuesto: []PresupuestoLinea{{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-08", Monto: "1000.00"}},
	}
	res, err := servicioControl(repoDep).ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "80")
	if err != nil {
		t.Fatalf("ControlPresupuestario: %v", err)
	}
	if res.Filas[0].Estado != CtrlAlerta {
		t.Errorf("departamento con umbral 80 y 85 %% consumido: estado = %q", res.Filas[0].Estado)
	}
	if res.UmbralAlertaPct != "80" {
		t.Errorf("umbral aplicado = %q, se esperaba 80", res.UmbralAlertaPct)
	}

	repoPar := &fakeRepo{partidasDim: []GastoPartidaDimension{
		{ClasificacionID: "cl-1", Clasificacion: "Combustible", Gasto: "850.00", Subpresupuesto: "1000.00"},
	}}
	filas, err := servicioControl(repoPar).PartidasDeDimension(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "d1", "80")
	if err != nil {
		t.Fatalf("PartidasDeDimension: %v", err)
	}
	if filas[0].Estado != CtrlAlerta {
		t.Errorf("partida con umbral 80 y 85 %% consumido: estado = %q", filas[0].Estado)
	}
}

func TestSubpresupuestoViajaAlRepositorio(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := servicioControl(repo)
	if err := svc.GuardarPresupuesto(context.Background(), "emp", "d1", "cl-9", "2026-08", "500000", "combustible del mes", "u1"); err != nil {
		t.Fatalf("GuardarPresupuesto: %v", err)
	}
	if repo.presuClasifID != "cl-9" {
		t.Errorf("el repositorio recibió clasificacion_id = %q", repo.presuClasifID)
	}
	// Y sin partida, el repositorio tiene que ver el vacío: es lo que lo manda al índice del TOTAL.
	if err := svc.GuardarPresupuesto(context.Background(), "emp", "d1", "", "2026-08", "2000000", "", "u1"); err != nil {
		t.Fatalf("GuardarPresupuesto total: %v", err)
	}
	if repo.presuClasifID != "" {
		t.Errorf("sin partida el repositorio debe recibir vacío; recibió %q", repo.presuClasifID)
	}
}

// ── Administración del catálogo de departamentos ─────────────────────────────

func TestNoSeBorraUnDepartamentoConHistoria(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{usoDepto: UsoDepartamento{Facturas: 3, Movimientos: 128}}
	err := servicioControl(repo).EliminarDepartamento(context.Background(), "emp", "d1", "u1")
	if err == nil {
		t.Fatal("se borró un departamento con 3 facturas y 128 movimientos")
	}
	var enUso *CatalogoEnUsoError
	if !errors.As(err, &enUso) {
		t.Fatalf("el error no es CatalogoEnUsoError: %T %v", err, err)
	}
	// El mensaje tiene que nombrar QUÉ cuelga: un «no se puede» pelado obliga a adivinar.
	for _, esperado := range []string{"3 facturas", "128 movimientos bancarios", "desactivalo"} {
		if !strings.Contains(enUso.Detalle, esperado) {
			t.Errorf("el detalle no dice %q: %q", esperado, enUso.Detalle)
		}
	}
	if repo.deptoEliminado {
		t.Error("se llamó al borrado físico igual")
	}
}

func TestSeBorraElDepartamentoDelQueNoCuelgaNada(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	if err := servicioControl(repo).EliminarDepartamento(context.Background(), "emp", "d1", "u1"); err != nil {
		t.Fatalf("EliminarDepartamento: %v", err)
	}
	if !repo.deptoEliminado {
		t.Error("no se llamó al borrado")
	}
}

func TestUsoCuentaLasSieteTablasQueReferencianElDepartamento(t *testing.T) {
	t.Parallel()
	// Contar de menos dejaría borrar algo que sí tiene historia, y el borrado fallaría con un error de
	// clave foránea que nadie sabe interpretar. Este test fija el total en las siete.
	u := UsoDepartamento{Facturas: 1, Empleados: 1, Fondos: 1, Validadores: 1,
		Partidas: 1, Movimientos: 1, LineasPresupuesto: 1}
	if u.Total() != 7 {
		t.Errorf("Total() = %d, se esperaban 7 (una por tabla que referencia el departamento)", u.Total())
	}
	if (UsoDepartamento{}).Total() != 0 {
		t.Error("un departamento sin uso debe dar cero")
	}
	// El detalle nombra solo lo que sí tiene y usa singular/plural.
	d := UsoDepartamento{Facturas: 1, Movimientos: 5}.Detalle()
	if d != "1 factura y 5 movimientos bancarios" {
		t.Errorf("detalle = %q", d)
	}
	if (UsoDepartamento{}).Detalle() != "" {
		t.Error("sin uso el detalle debe ser vacío")
	}
}

func TestUnDepartamentoSinNombreNoSeCrea(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	for _, malo := range []string{"", "   ", "\t"} {
		if _, err := servicioControl(repo).CrearDepartamento(context.Background(), "emp", malo, "", "u1"); !errors.Is(err, ErrNombreRequerido) {
			t.Errorf("nombre %q devolvió %v, se esperaba ErrNombreRequerido", malo, err)
		}
	}
	if len(repo.departamentos) != 0 {
		t.Errorf("se creó algo igual: %+v", repo.departamentos)
	}
}
