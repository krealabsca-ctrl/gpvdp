package bancos

// Lo que se prueba acá es que el control presupuestario no mienta: que el gasto sin dueño no se
// reparta entre los departamentos, que un mes a medio clasificar lo diga, y que no aparezca un
// semáforo verde sobre un presupuesto que nadie definió.

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func servicioControl(repo *fakeRepo) *Service {
	return NewService(repo, nil, zap.NewNop(), true)
}

func TestControlNoRepartteElGastoSinDueno(t *testing.T) {
	t.Parallel()
	// El gasto sin departamento va en su propia fila y NO se suma a los que sí tienen dueño:
	// repartirlo haría que cada departamento pareciera más chico de lo que es.
	repo := &fakeRepo{
		saludMeses: []SaludMes{{Periodo: "2026-08", Movs: 100, PctClasificado: "100.0"}},
		gastoDim: []GastoDimension{
			{ID: "d1", Nombre: "Logística", Movs: 10, Gasto: "300000.00"},
			{ID: "", Nombre: "(sin asignar)", Movs: 40, Gasto: "900000.00"},
		},
		presupuesto: []PresupuestoLinea{
			{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-08", Monto: "500000.00"},
		},
	}
	got, err := servicioControl(repo).ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "")
	if err != nil {
		t.Fatalf("ControlPresupuestario: %v", err)
	}
	if len(got.Filas) != 1 {
		t.Fatalf("filas = %d, se esperaba 1 (el sin asignar NO es un departamento)", len(got.Filas))
	}
	if got.SinAsignar != "900000.00" || got.SinAsignarMovs != 40 {
		t.Errorf("sin asignar = %s en %d movs; se esperaba 900000.00 en 40", got.SinAsignar, got.SinAsignarMovs)
	}
	f := got.Filas[0]
	if f.Gasto != "300000.00" || f.Presupuesto != "500000.00" || f.Disponible != "200000.00" {
		t.Errorf("fila = %+v", f)
	}
	if f.ConsumidoPct != "60.0" || f.Estado != CtrlEnRango {
		t.Errorf("consumido = %s, estado = %s; se esperaba 60.0 y EN_RANGO", f.ConsumidoPct, f.Estado)
	}
	// El total del gasto sí incluye TODO: es la plata que salió del banco.
	if got.TotalGasto != "1200000.00" {
		t.Errorf("total del gasto = %s, se esperaba 1200000.00 (incluye el sin asignar)", got.TotalGasto)
	}
	if !strings.Contains(got.Aviso, "no tienen departamento asignado") {
		t.Errorf("con gasto sin dueño el control debe avisarlo; dijo %q", got.Aviso)
	}
}

func TestControlSinPresupuestoNoPintaSemaforo(t *testing.T) {
	t.Parallel()
	// Un departamento sin presupuesto definido NO está «en rango»: no hay contra qué compararlo.
	repo := &fakeRepo{
		saludMeses: []SaludMes{{Periodo: "2026-08", Movs: 100, PctClasificado: "100.0"}},
		gastoDim:   []GastoDimension{{ID: "d9", Nombre: "Mercadeo", Movs: 3, Gasto: "80000.00"}},
	}
	got, err := servicioControl(repo).ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "")
	if err != nil {
		t.Fatalf("ControlPresupuestario: %v", err)
	}
	f := got.Filas[0]
	if f.Estado != CtrlSinPresupuesto {
		t.Fatalf("estado = %s, se esperaba SIN_PRESUPUESTO", f.Estado)
	}
	if f.Presupuesto != "" || f.Disponible != "" || f.ConsumidoPct != "" {
		t.Errorf("sin presupuesto no se inventan números: %+v", f)
	}
}

func TestControlMarcaAlertaYExcedidoConElUmbralDelUsuario(t *testing.T) {
	t.Parallel()
	repo := func() *fakeRepo {
		return &fakeRepo{
			saludMeses: []SaludMes{{Periodo: "2026-08", Movs: 100, PctClasificado: "100.0"}},
			gastoDim: []GastoDimension{
				{ID: "d1", Nombre: "Casi", Movs: 1, Gasto: "800000.00"},    // 80 %
				{ID: "d2", Nombre: "Pasado", Movs: 1, Gasto: "1100000.00"}, // 110 %
			},
			presupuesto: []PresupuestoLinea{
				{DepartamentoID: "d1", Departamento: "Casi", Periodo: "2026-08", Monto: "1000000.00"},
				{DepartamentoID: "d2", Departamento: "Pasado", Periodo: "2026-08", Monto: "1000000.00"},
			},
		}
	}
	// Con el default (90 %), el de 80 % está en rango.
	got, err := servicioControl(repo()).ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "")
	if err != nil {
		t.Fatalf("ControlPresupuestario: %v", err)
	}
	porNombre := map[string]FilaControl{}
	for _, f := range got.Filas {
		porNombre[f.Departamento] = f
	}
	if porNombre["Casi"].Estado != CtrlEnRango {
		t.Errorf("con umbral 90, el 80 %% debe estar EN_RANGO; quedó %s", porNombre["Casi"].Estado)
	}
	if porNombre["Pasado"].Estado != CtrlExcedido || porNombre["Pasado"].Disponible != "-100000.00" {
		t.Errorf("el que se pasó debe quedar EXCEDIDO con disponible negativo: %+v", porNombre["Pasado"])
	}

	// Bajando el umbral a 75, el mismo 80 % pasa a ALERTA. El criterio es del usuario.
	got2, err := servicioControl(repo()).ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "75")
	if err != nil {
		t.Fatalf("ControlPresupuestario con umbral 75: %v", err)
	}
	for _, f := range got2.Filas {
		if f.Departamento == "Casi" && f.Estado != CtrlAlerta {
			t.Errorf("con umbral 75 el 80 %% debe ser ALERTA; quedó %s", f.Estado)
		}
	}
	if got2.UmbralAlertaPct != "75" {
		t.Errorf("el umbral aplicado debe volver en la respuesta para poder mostrarlo; volvió %q", got2.UmbralAlertaPct)
	}
}

func TestControlAvisaCuandoElMesEstaAMedioClasificar(t *testing.T) {
	t.Parallel()
	// Es el guardarraíl que importa: si el mes está al 31 %, el gasto que se ve es MENOR que el real,
	// así que el presupuesto parece menos consumido de lo que está y eso invita a seguir gastando.
	repo := &fakeRepo{
		saludMeses: []SaludMes{{Periodo: "2026-07", Movs: 8167, PctClasificado: "31.8"}},
		gastoDim:   []GastoDimension{{ID: "d1", Nombre: "Logística", Movs: 5, Gasto: "100000.00"}},
		presupuesto: []PresupuestoLinea{
			{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-07", Monto: "1000000.00"},
		},
	}
	got, err := servicioControl(repo).ControlPresupuestario(context.Background(), "emp", "2026-07", "2026-07", AgruparPorDepartamento, "")
	if err != nil {
		t.Fatalf("ControlPresupuestario: %v", err)
	}
	if !strings.Contains(got.Aviso, "medio clasificar") || !strings.Contains(got.Aviso, "subestimado") {
		t.Fatalf("el aviso debe decir que el consumo está subestimado; dijo %q", got.Aviso)
	}
	// Y el número igual se muestra: el usuario pidió el análisis, con aviso, no en vez del análisis.
	if got.Filas[0].ConsumidoPct != "10.0" {
		t.Errorf("consumido = %s, se esperaba 10.0", got.Filas[0].ConsumidoPct)
	}
}

func TestControlMuestraElPresupuestoSinUsar(t *testing.T) {
	t.Parallel()
	// Un departamento con presupuesto y CERO gasto no aparece en el gasto real, y es justo el que hay
	// que ver: tiene plata autorizada sin tocar.
	repo := &fakeRepo{
		saludMeses: []SaludMes{{Periodo: "2026-08", Movs: 10, PctClasificado: "100.0"}},
		gastoDim:   []GastoDimension{},
		presupuesto: []PresupuestoLinea{
			{DepartamentoID: "d7", Departamento: "Mantenimiento", Periodo: "2026-08", Monto: "400000.00"},
		},
	}
	got, err := servicioControl(repo).ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "")
	if err != nil {
		t.Fatalf("ControlPresupuestario: %v", err)
	}
	if len(got.Filas) != 1 {
		t.Fatalf("filas = %d; el departamento con presupuesto sin usar tiene que aparecer", len(got.Filas))
	}
	f := got.Filas[0]
	if f.Gasto != "0.00" || f.Disponible != "400000.00" || f.ConsumidoPct != "0.0" {
		t.Errorf("fila = %+v", f)
	}
	if got.TotalPresupuesto != "400000.00" {
		t.Errorf("el total presupuestado debe incluirlo; dio %s", got.TotalPresupuesto)
	}
}

func TestControlPorSedeNoInventaPresupuesto(t *testing.T) {
	t.Parallel()
	// El presupuesto se definió por departamento. Agrupando por sede se muestra el gasto real y se
	// dice que no hay presupuesto que comparar, en vez de repartir el del departamento.
	repo := &fakeRepo{
		saludMeses: []SaludMes{{Periodo: "2026-08", Movs: 10, PctClasificado: "100.0"}},
		gastoDim:   []GastoDimension{{ID: "s1", Nombre: "Limón", Movs: 4, Gasto: "250000.00"}},
		presupuesto: []PresupuestoLinea{
			{DepartamentoID: "d1", Departamento: "Logística", Periodo: "2026-08", Monto: "999999.00"},
		},
	}
	got, err := servicioControl(repo).ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", AgruparPorSede, "")
	if err != nil {
		t.Fatalf("ControlPresupuestario por sede: %v", err)
	}
	if repo.agrupoPor != AgruparPorSede {
		t.Errorf("el repositorio recibió agrupar_por = %q", repo.agrupoPor)
	}
	if got.TotalPresupuesto != "0.00" {
		t.Errorf("por sede no hay presupuesto: dio %s", got.TotalPresupuesto)
	}
	f := got.Filas[0]
	if f.Estado != CtrlSinPresupuesto || f.Presupuesto != "" {
		t.Errorf("por sede ninguna fila puede traer presupuesto: %+v", f)
	}
	if !strings.Contains(got.Aviso, "por departamento, no por sede") {
		t.Errorf("hay que decir que el presupuesto es por departamento; dijo %q", got.Aviso)
	}
}

func TestDimensionInvalidaNoLlegaAlSQL(t *testing.T) {
	t.Parallel()
	// `agrupar_por` se concatena en el SQL, así que validarlo en el servicio es lo que cierra la
	// puerta a que se pueda inyectar algo por ese camino.
	repo := &fakeRepo{}
	svc := servicioControl(repo)
	for _, malo := range []string{"", "departamento; DROP TABLE movimiento_bancario", "cuenta", "DEPARTAMENTO"} {
		if _, err := svc.ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", malo, ""); err != ErrDimensionInvalida {
			t.Errorf("agrupar_por %q devolvió %v, se esperaba ErrDimensionInvalida", malo, err)
		}
		if _, err := svc.PartidasDeDimension(context.Background(), "emp", "2026-08", "2026-08", malo, "", ""); err != ErrDimensionInvalida {
			t.Errorf("partidas con agrupar_por %q devolvió %v", malo, err)
		}
	}
	if repo.agrupoPor != "" {
		t.Errorf("una dimensión inválida no debe llegar al repositorio; llegó %q", repo.agrupoPor)
	}
}

func TestAsignarDimensionesDicecuantosMovimientosAbarca(t *testing.T) {
	t.Parallel()
	// Poner el default en una partida atribuye toda su historia sin tocar un movimiento. El número
	// es lo que hace visible el alcance del clic.
	repo := &fakeRepo{movsDeClasif: 245}
	n, err := servicioControl(repo).AsignarDimensionesClasificacion(context.Background(), "emp", "cl-1", "d-1", "s-1", "u1")
	if err != nil {
		t.Fatalf("AsignarDimensionesClasificacion: %v", err)
	}
	if n != 245 {
		t.Errorf("movimientos afectados = %d, se esperaban 245", n)
	}
	if repo.dimClasif != [3]string{"cl-1", "d-1", "s-1"} {
		t.Errorf("el repositorio recibió %+v", repo.dimClasif)
	}
}

func TestPresupuestoRechazaMontoImposible(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := servicioControl(repo)
	for _, malo := range []string{"-1", "-500000.00", "mil pesos", ""} {
		if err := svc.GuardarPresupuesto(context.Background(), "emp", "d1", "", "2026-08", malo, "", "u1"); err == nil {
			t.Errorf("monto %q se aceptó y no debía", malo)
		}
	}
	if repo.presuGuardado != [3]string{} {
		t.Errorf("no se debió guardar nada; se guardó %+v", repo.presuGuardado)
	}
	// Cero SÍ es válido: significa «este departamento no tiene presupuesto este mes», que es una
	// decisión, distinta de no haberlo definido.
	if err := svc.GuardarPresupuesto(context.Background(), "emp", "d1", "", "2026-08", "0", "sin asignación este mes", "u1"); err != nil {
		t.Errorf("un presupuesto de cero es legítimo: %v", err)
	}
	if repo.presuGuardado != [3]string{"d1", "2026-08", "0.00"} {
		t.Errorf("se guardó %+v", repo.presuGuardado)
	}
}

func TestPartidasDeDimensionExplicaDeDondeSalioLaAtribucion(t *testing.T) {
	t.Parallel()
	// Ver «Logística» sin saber si lo escribió alguien o si lo heredó de la partida no alcanza para
	// poder corregirlo en el lugar correcto.
	repo := &fakeRepo{partidasDim: []GastoPartidaDimension{
		{Concepto: "Gastos", Clasificacion: "Combustible", Movs: 3, Gasto: "1000.00", Origen: OrigenDimPartida},
		{Concepto: "Gastos", Clasificacion: "Viaticos", Movs: 1, Gasto: "500.00", Origen: OrigenDimMovimiento},
		{Concepto: "Gastos", Clasificacion: "Flores", Movs: 2, Gasto: "200.00", Origen: OrigenDimFactura},
	}}
	filas, err := servicioControl(repo).PartidasDeDimension(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "d1", "")
	if err != nil {
		t.Fatalf("PartidasDeDimension: %v", err)
	}
	esperado := []string{"heredado de la partida", "asignado a este movimiento", "heredado de la factura de CxP"}
	for i, e := range esperado {
		if filas[i].OrigenLegible != e {
			t.Errorf("fila %d: origen legible = %q, se esperaba %q", i, filas[i].OrigenLegible, e)
		}
	}
	if repo.pidioDimID != "d1" {
		t.Errorf("el repositorio recibió id = %q", repo.pidioDimID)
	}

	// Y el id vacío pide justamente el gasto SIN atribuir, que es donde arranca el trabajo.
	if _, err := servicioControl(repo).PartidasDeDimension(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "", ""); err != nil {
		t.Fatalf("PartidasDeDimension sin id: %v", err)
	}
	if repo.pidioDimID != "" {
		t.Errorf("con id vacío se debe pedir el sin asignar; llegó %q", repo.pidioDimID)
	}
}

func TestControlDevuelveFilasVaciasYNoNull(t *testing.T) {
	t.Parallel()
	// Un slice nil de Go se serializa como `null`, y en el navegador `null.map()` rompe la pantalla
	// completa. Pasó de verdad al abrir el control con cero departamentos atribuidos, que es el
	// estado inicial de cualquier empresa: el caso más común, no el raro.
	repo := &fakeRepo{saludMeses: []SaludMes{{Periodo: "2026-08", Movs: 10, PctClasificado: "100.0"}}}
	got, err := servicioControl(repo).ControlPresupuestario(context.Background(), "emp", "2026-08", "2026-08", AgruparPorDepartamento, "")
	if err != nil {
		t.Fatalf("ControlPresupuestario: %v", err)
	}
	if got.Filas == nil {
		t.Fatal("Filas nunca llega como null: tiene que ser un arreglo vacío")
	}
	if len(got.Filas) != 0 {
		t.Errorf("filas = %d, se esperaban 0", len(got.Filas))
	}
}
