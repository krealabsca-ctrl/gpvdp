package cxp

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

func svcResp(repo *fakeRepo, perms *permisosFalsos) *Service {
	s := NewService(repo, nil, zap.NewNop())
	if perms != nil {
		s.SetPermisos(perms)
	}
	return s
}

// ── EL ALCANCE: quién ve qué ───────────────────────────────────────────────────────────────────
//
// Esto es lo que el Director pidió textualmente: «no todo mundo debe tener acceso a ver todas las
// responsabilidades». El riesgo acá no es un error visible: es que un recorte mal resuelto deje
// ver TODO en silencio, y nadie se entere nunca.

func TestSinPermisoNoVeNingunaResponsabilidad(t *testing.T) {
	repo := &fakeRepo{respLista: []Responsabilidad{{ID: "a"}, {ID: "b"}}}
	perms := &permisosFalsos{porRol: map[string][]string{"CURIOSO": {"cxp.ver"}}}
	s := svcResp(repo, perms)

	if _, err := s.ListarResponsabilidades(context.Background(), "emp", "CURIOSO", "u1", FiltrosResponsabilidad{}); err != nil {
		t.Fatalf("no debía fallar: %v", err)
	}
	if !repo.capFiltrosRespSet {
		t.Fatal("el service no llamó al repositorio")
	}
	a := repo.capFiltrosResp.Alcance
	// LO QUE IMPORTA: `Todo` en false. Si saliera true, quien no tiene permiso vería la empresa
	// entera y la pantalla se vería perfectamente normal.
	if a.Todo {
		t.Error("sin permiso el alcance NO puede ser Todo: eso es acceso total en silencio")
	}
	if len(a.ClasificacionIDs) != 0 || a.UsuarioID != "" {
		t.Errorf("sin permiso el alcance tiene que quedar vacío, quedó %+v", a)
	}
}

func TestConPermisoDeVerTodasElAlcanceEsTodo(t *testing.T) {
	repo := &fakeRepo{}
	perms := &permisosFalsos{porRol: map[string][]string{"DIRECTOR": {permisoRespVer}}}
	s := svcResp(repo, perms)

	if _, err := s.ListarResponsabilidades(context.Background(), "emp", "DIRECTOR", "u1", FiltrosResponsabilidad{}); err != nil {
		t.Fatalf("no debía fallar: %v", err)
	}
	if !repo.capFiltrosResp.Alcance.Todo {
		t.Error("con cxp.responsabilidades.ver el alcance tiene que ser Todo")
	}
}

// «VER SOLO LAS MÍAS» = mis partidas O lo que llevo yo. Con Y sería la intersección —casi nada— y
// el encargado de servicios públicos no vería el alquiler del que es titular.
func TestVerMiasCombinaPartidasYTitularidad(t *testing.T) {
	repo := &fakeRepo{partidasDelRol: []string{"part-agua", "part-luz"}}
	perms := &permisosFalsos{porRol: map[string][]string{"ENCARGADO": {permisoRespVerMias}}}
	s := svcResp(repo, perms)

	if _, err := s.ListarResponsabilidades(context.Background(), "emp", "ENCARGADO", "u-encargado", FiltrosResponsabilidad{}); err != nil {
		t.Fatalf("no debía fallar: %v", err)
	}
	a := repo.capFiltrosResp.Alcance
	if a.Todo {
		t.Fatal("«ver solo las mías» no puede resolver a Todo")
	}
	if len(a.ClasificacionIDs) != 2 {
		t.Errorf("tenía que traer las 2 partidas del rol, trajo %v", a.ClasificacionIDs)
	}
	if a.UsuarioID != "u-encargado" {
		t.Errorf("tenía que incluir al usuario para lo que lleva a su nombre, trajo %q", a.UsuarioID)
	}
}

// El detalle también comprueba el alcance: si no, bastaría con adivinar un id.
func TestDetalleDeOtraResponsabilidadDa404(t *testing.T) {
	repo := &fakeRepo{
		respLista:      []Responsabilidad{{ID: "mia"}}, // lo que SÍ ve con su alcance
		partidasDelRol: []string{"part-agua"},
	}
	perms := &permisosFalsos{porRol: map[string][]string{"ENCARGADO": {permisoRespVerMias}}}
	s := svcResp(repo, perms)

	if _, err := s.ResponsabilidadPorID(context.Background(), "emp", "ENCARGADO", "u1", "ajena"); !errors.Is(err, ErrResponsabilidadNoEncontrada) {
		t.Errorf("una responsabilidad fuera del alcance tiene que dar «no existe», dio %v", err)
	}
	if _, err := s.ResponsabilidadPorID(context.Background(), "emp", "ENCARGADO", "u1", "mia"); err != nil {
		t.Errorf("la propia sí se tiene que poder leer, dio %v", err)
	}
}

// Sin checker de RBAC configurado (arranque sin permisos), ve todo: si no, una instalación así
// quedaría inutilizable sin que nada lo explique.
func TestSinCheckerDePermisosVeTodo(t *testing.T) {
	repo := &fakeRepo{}
	s := svcResp(repo, nil)
	if _, err := s.ListarResponsabilidades(context.Background(), "emp", "", "u1", FiltrosResponsabilidad{}); err != nil {
		t.Fatalf("no debía fallar: %v", err)
	}
	if !repo.capFiltrosResp.Alcance.Todo {
		t.Error("sin checker configurado el alcance tiene que ser Todo")
	}
}

// ── ABRIR EL MES ───────────────────────────────────────────────────────────────────────────────

func TestPlanDelMesRespetaPeriodicidadYLoQueYaEstaba(t *testing.T) {
	repo := &fakeRepo{
		abrirMesCreadas: -1,
		respActivas: []Responsabilidad{
			{ID: "alq", Nombre: "Alquiler", Periodicidad: "MENSUAL", DiaVencimiento: 1, MontoEsperado: "1000000", Moneda: "CRC"},
			{ID: "pol", Nombre: "Póliza", Periodicidad: "TRIMESTRAL", MesAncla: 3, DiaVencimiento: 15, MontoEsperado: "500000", Moneda: "CRC"},
			{ID: "mun", Nombre: "Patente", Periodicidad: "ANUAL", MesAncla: 1, DiaVencimiento: 31, MontoEsperado: "250000", Moneda: "CRC"},
			{ID: "int", Nombre: "Internet", Periodicidad: "MENSUAL", DiaVencimiento: 5, MontoEsperado: "80000", Moneda: "CRC"},
		},
		// Internet ya tiene su fila de setiembre: no se vuelve a crear.
		conFilaEnElMes: map[string]bool{"int": true},
	}
	s := svcResp(repo, nil)

	plan, err := s.PrevisualizarMes(context.Background(), "emp", "2026-09")
	if err != nil {
		t.Fatalf("previsualizar: %v", err)
	}
	// Setiembre: alquiler sí (mensual). Póliza sí (trimestral desde marzo: 3-6-9-12).
	// Patente no (anual en enero). Internet ya estaba.
	if plan.VaACrear != 2 {
		t.Errorf("VaACrear = %d, quería 2 (alquiler y póliza)", plan.VaACrear)
	}
	if plan.YaEstaban != 1 {
		t.Errorf("YaEstaban = %d, quería 1 (internet)", plan.YaEstaban)
	}
	if plan.MontoEsperado != "1500000.00" {
		t.Errorf("MontoEsperado = %s, quería 1500000.00", plan.MontoEsperado)
	}
	// Lo que queda afuera se explica; el silencio es lo que hace que nadie confíe en el número.
	if len(plan.Afuera) == 0 {
		t.Fatal("la patente anual tenía que aparecer explicada en Afuera")
	}
	hallado := false
	for _, a := range plan.Afuera {
		for _, n := range a.Nombres {
			if n == "Patente" {
				hallado = true
			}
		}
	}
	if !hallado {
		t.Errorf("la Patente tenía que estar en Afuera, quedó: %+v", plan.Afuera)
	}
}

// La previsualización y la apertura tienen que mirar EXACTAMENTE lo mismo, o el total que la
// persona confirma no sería el que se ejecuta.
func TestAbrirMesCalculaElVencimientoYCongelaElMonto(t *testing.T) {
	repo := &fakeRepo{
		abrirMesCreadas: -1,
		respActivas: []Responsabilidad{
			// Vence «el 31» en un mes de 30: tiene que caer al 30, no irse a octubre.
			{ID: "x", Nombre: "Alquiler", Periodicidad: "MENSUAL", DiaVencimiento: 31, MontoEsperado: "1234.5", Moneda: "CRC"},
		},
	}
	s := svcResp(repo, nil)

	if _, err := s.AbrirMes(context.Background(), "emp", "2026-09", -1, "u1"); err != nil {
		t.Fatalf("abrir mes: %v", err)
	}
	if !repo.capAbrirMesSet || len(repo.capAbrirMes) != 1 {
		t.Fatalf("tenía que mandar 1 fila, mandó %+v", repo.capAbrirMes)
	}
	f := repo.capAbrirMes[0]
	if f.VenceEn != "2026-09-30" {
		t.Errorf("VenceEn = %s, quería 2026-09-30 (el 31 de setiembre no existe)", f.VenceEn)
	}
	if f.MontoEsperado != "1234.50" {
		t.Errorf("MontoEsperado = %s, quería 1234.50 con dos decimales", f.MontoEsperado)
	}
	if f.Periodo != "2026-09" {
		t.Errorf("Periodo = %s", f.Periodo)
	}
}

// EL SEGUNDO CLIC. El UNIQUE de la base no deja crear nada y el service tiene que decirlo, no
// fingir que hizo algo.
func TestAbrirMesDosVecesInformaCero(t *testing.T) {
	repo := &fakeRepo{
		abrirMesCreadas: 0, // el repo dice: no creé ninguna, ya estaban
		respActivas: []Responsabilidad{
			{ID: "x", Nombre: "Alquiler", Periodicidad: "MENSUAL", DiaVencimiento: 1, MontoEsperado: "1000", Moneda: "CRC"},
		},
	}
	s := svcResp(repo, nil)
	plan, err := s.AbrirMes(context.Background(), "emp", "2026-09", -1, "u1")
	if err != nil {
		t.Fatalf("abrir mes: %v", err)
	}
	if plan.VaACrear != 0 {
		t.Errorf("el segundo clic tiene que reportar 0 creadas, reportó %d", plan.VaACrear)
	}
}

// Uno confirma un TOTAL, no una intención: si el plan cambió mientras la persona miraba, se frena.
func TestAbrirMesSeDetieneSiElPlanCambio(t *testing.T) {
	repo := &fakeRepo{
		abrirMesCreadas: -1,
		respActivas: []Responsabilidad{
			{ID: "a", Nombre: "A", Periodicidad: "MENSUAL", DiaVencimiento: 1, MontoEsperado: "1", Moneda: "CRC"},
			{ID: "b", Nombre: "B", Periodicidad: "MENSUAL", DiaVencimiento: 1, MontoEsperado: "1", Moneda: "CRC"},
		},
	}
	s := svcResp(repo, nil)
	// La persona vio 1, pero ahora son 2.
	if _, err := s.AbrirMes(context.Background(), "emp", "2026-09", 1, "u1"); err == nil {
		t.Error("tenía que detenerse: la persona confirmó un total distinto del actual")
	}
	if repo.capAbrirMesSet {
		t.Error("no podía escribir nada si el plan cambió")
	}
}

func TestAbrirMesRechazaPeriodoInvalido(t *testing.T) {
	s := svcResp(&fakeRepo{abrirMesCreadas: -1}, nil)
	for _, p := range []string{"", "2026", "2026-13", "basura"} {
		if _, err := s.AbrirMes(context.Background(), "emp", p, -1, "u1"); !errors.Is(err, ErrPeriodoInvalido) {
			t.Errorf("período %q tenía que rechazarse, dio %v", p, err)
		}
	}
}

// ── CERRAR UN MES ──────────────────────────────────────────────────────────────────────────────

func TestCerrarPeriodoExigeMotivoYPrueba(t *testing.T) {
	casos := []struct {
		nombre string
		c      CierrePeriodo
		quiero error
	}{
		{"no aplica sin motivo", CierrePeriodo{Estado: PerNoAplica}, ErrMotivoObligatorio},
		{"cumplida sin decir cómo", CierrePeriodo{Estado: PerCumplida}, ErrPruebaObligatoria},
		{"cumplida con factura pero sin documento", CierrePeriodo{Estado: PerCumplida, CumplidaCon: PruebaFactura}, ErrPruebaObligatoria},
		{"cumplida con movimiento pero sin movimiento", CierrePeriodo{Estado: PerCumplida, CumplidaCon: PruebaMovimiento}, ErrPruebaObligatoria},
		{"cumplida con acuse pero sin archivo", CierrePeriodo{Estado: PerCumplida, CumplidaCon: PruebaAcuse, AcuseArchivo: "   "}, ErrPruebaObligatoria},
	}
	for _, c := range casos {
		repo := &fakeRepo{abrirMesCreadas: -1}
		s := svcResp(repo, nil)
		err := s.CerrarPeriodo(context.Background(), "emp", "per-1", c.c, "u1")
		if !errors.Is(err, c.quiero) {
			t.Errorf("%s: dio %v, quería %v", c.nombre, err, c.quiero)
		}
		if repo.capCierre != nil {
			t.Errorf("%s: no podía llegar al repositorio", c.nombre)
		}
	}
}

// Un «no aplica» no puede arrastrar pruebas: la fila diría a la vez que no correspondía y que se pagó.
func TestNoAplicaLimpiaLasPruebas(t *testing.T) {
	repo := &fakeRepo{abrirMesCreadas: -1}
	s := svcResp(repo, nil)
	err := s.CerrarPeriodo(context.Background(), "emp", "per-1", CierrePeriodo{
		Estado: PerNoAplica, Motivo: "el local estuvo cerrado", CumplidaCon: PruebaFactura, DocumentoID: "doc-1",
	}, "u1")
	if err != nil {
		t.Fatalf("no debía fallar: %v", err)
	}
	if repo.capCierre.DocumentoID != "" || repo.capCierre.CumplidaCon != "" {
		t.Errorf("un «no aplica» no puede llevar prueba: %+v", repo.capCierre)
	}
}

// Y una cumplida lleva UNA sola prueba: si llegan dos, se queda la que la persona eligió.
func TestCumplidaDejaSoloLaPruebaElegida(t *testing.T) {
	repo := &fakeRepo{abrirMesCreadas: -1}
	s := svcResp(repo, nil)
	err := s.CerrarPeriodo(context.Background(), "emp", "per-1", CierrePeriodo{
		Estado: PerCumplida, CumplidaCon: PruebaMovimiento,
		MovimientoID: "mov-1", DocumentoID: "doc-1", AcuseArchivo: "acuse.pdf",
	}, "u1")
	if err != nil {
		t.Fatalf("no debía fallar: %v", err)
	}
	if repo.capCierre.MovimientoID != "mov-1" {
		t.Errorf("tenía que conservar el movimiento: %+v", repo.capCierre)
	}
	if repo.capCierre.DocumentoID != "" || repo.capCierre.AcuseArchivo != "" {
		t.Errorf("tenía que descartar las otras pruebas: %+v", repo.capCierre)
	}
}

func TestReabrirExigeMotivo(t *testing.T) {
	s := svcResp(&fakeRepo{abrirMesCreadas: -1}, nil)
	if err := s.ReabrirPeriodo(context.Background(), "emp", "per-1", "  ", "u1"); err == nil {
		t.Error("reabrir sin motivo tenía que fallar")
	}
}

// ── LA PANTALLA DEL MES ────────────────────────────────────────────────────────────────────────

// La guarda de honestidad tiene que llegar hasta la pantalla, no quedarse en la función pura.
func TestMesNoDiceVencidaSiElBancoNoAlcanza(t *testing.T) {
	repo := &fakeRepo{
		abrirMesCreadas: -1,
		periodos: []PeriodoResponsabilidad{
			{ID: "p1", ResponsabilidadID: "r1", Periodo: "2026-09", VenceEn: "2026-09-01",
				MontoEsperado: "1000.00", Estado: PerPendiente},
		},
		bancoHasta: "", // no hay banco importado
	}
	s := svcResp(repo, nil)
	v, err := s.MesDeResponsabilidades(context.Background(), "emp", "", "u1", "2026-09")
	if err != nil {
		t.Fatalf("mes: %v", err)
	}
	if len(v.Filas) != 1 {
		t.Fatalf("filas = %d", len(v.Filas))
	}
	if v.Filas[0].Semaforo != SemSinDato {
		t.Errorf("semáforo = %s, quería %s: sin banco no se puede afirmar que esté vencida",
			v.Filas[0].Semaforo, SemSinDato)
	}
	if v.Resumen.BancoHasta != "" {
		t.Errorf("BancoHasta tenía que viajar vacío para que la pantalla lo explique")
	}
}

// SIN ABRIR: la responsabilidad existe y el mes no tiene fila. Sin esto, un mes que nadie abrió se
// ve idéntico a un mes sin nada pendiente — el olvido del olvido.
func TestMesDelataLoQueNadieAbrio(t *testing.T) {
	repo := &fakeRepo{
		abrirMesCreadas: -1,
		respLista: []Responsabilidad{
			{ID: "alq", Nombre: "Alquiler", Periodicidad: "MENSUAL", DiaVencimiento: 1},
			{ID: "pat", Nombre: "Patente", Periodicidad: "ANUAL", MesAncla: 1, DiaVencimiento: 31},
		},
		periodos:       []PeriodoResponsabilidad{},
		conFilaEnElMes: map[string]bool{},
	}
	s := svcResp(repo, nil)
	v, err := s.MesDeResponsabilidades(context.Background(), "emp", "", "u1", "2026-09")
	if err != nil {
		t.Fatalf("mes: %v", err)
	}
	// El alquiler (mensual) sí toca setiembre; la patente (anual en enero) no.
	if len(v.SinAbrir) != 1 || v.SinAbrir[0].ID != "alq" {
		t.Errorf("SinAbrir = %+v, quería solo el alquiler", v.SinAbrir)
	}
	if v.Resumen.SinAbrir != 1 {
		t.Errorf("el resumen tiene que contar 1 sin abrir, contó %d", v.Resumen.SinAbrir)
	}
}

// «Mis responsabilidades» no depende de permisos: lo que te asignaron, lo ves.
func TestMisResponsabilidadesFiltraPorElUsuario(t *testing.T) {
	repo := &fakeRepo{abrirMesCreadas: -1}
	perms := &permisosFalsos{porRol: map[string][]string{}} // sin ningún permiso
	s := svcResp(repo, perms)

	if _, err := s.MisResponsabilidades(context.Background(), "emp", "u-titular", "2026-09"); err != nil {
		t.Fatalf("no debía fallar: %v", err)
	}
	a := repo.capFiltrosPeriodo.Alcance
	if a.Todo {
		t.Error("«mis responsabilidades» nunca puede ser Todo")
	}
	if a.UsuarioID != "u-titular" {
		t.Errorf("tenía que filtrar por el usuario, filtró por %q", a.UsuarioID)
	}
}

// ── DECLARAR UN ACUERDO ────────────────────────────────────────────────────────────────────────

func TestCrearResponsabilidadAplicaLosValoresDeFabrica(t *testing.T) {
	repo := &fakeRepo{abrirMesCreadas: -1}
	s := svcResp(repo, nil)
	_, err := s.CrearResponsabilidad(context.Background(), "emp", ResponsabilidadInput{
		Nombre: "  Alquiler sede  ", Contraparte: " Don Fulano ", DiaVencimiento: 1,
	}, "u1")
	if err != nil {
		t.Fatalf("no debía fallar: %v", err)
	}
	in := repo.respCreada
	if in.Nombre != "Alquiler sede" || in.Contraparte != "Don Fulano" {
		t.Errorf("tenía que recortar los espacios: %q / %q", in.Nombre, in.Contraparte)
	}
	if in.Tipo != "PAGO" || in.Periodicidad != "MENSUAL" || in.Moneda != "CRC" ||
		in.MontoTipo != "FIJO" || in.RespaldoTipo != "NINGUNO" || in.MontoEsperado != "0" {
		t.Errorf("valores de fábrica mal aplicados: %+v", in)
	}
}

func TestCrearResponsabilidadRechazaLoQueNoSeSostiene(t *testing.T) {
	casos := []struct {
		nombre string
		in     ResponsabilidadInput
	}{
		{"sin nombre", ResponsabilidadInput{Contraparte: "X", DiaVencimiento: 1}},
		{"sin contraparte", ResponsabilidadInput{Nombre: "X", DiaVencimiento: 1}},
		{"día 0", ResponsabilidadInput{Nombre: "X", Contraparte: "Y", DiaVencimiento: 0}},
		{"día 32", ResponsabilidadInput{Nombre: "X", Contraparte: "Y", DiaVencimiento: 32}},
		{"monto ilegible", ResponsabilidadInput{Nombre: "X", Contraparte: "Y", DiaVencimiento: 1, MontoEsperado: "mil"}},
		{"periodicidad inventada", ResponsabilidadInput{Nombre: "X", Contraparte: "Y", DiaVencimiento: 1, Periodicidad: "QUINCENAL"}},
		{"trimestral sin ancla", ResponsabilidadInput{Nombre: "X", Contraparte: "Y", DiaVencimiento: 1, Periodicidad: "TRIMESTRAL"}},
		// La regla fiscal: de palabra no puede ser deducible.
		{"verbal y deducible", ResponsabilidadInput{Nombre: "X", Contraparte: "Y", DiaVencimiento: 1, RespaldoTipo: "VERBAL", Deducible: true}},
		{"contrato sin adjunto", ResponsabilidadInput{Nombre: "X", Contraparte: "Y", DiaVencimiento: 1, RespaldoTipo: "CONTRATO"}},
	}
	for _, c := range casos {
		repo := &fakeRepo{abrirMesCreadas: -1}
		s := svcResp(repo, nil)
		if _, err := s.CrearResponsabilidad(context.Background(), "emp", c.in, "u1"); err == nil {
			t.Errorf("%s: tenía que fallar", c.nombre)
		}
		if repo.respCreada != nil {
			t.Errorf("%s: no podía llegar al repositorio", c.nombre)
		}
	}
}

// Suspender saca la responsabilidad del calendario: permite TAPAR el olvido en vez de cumplirlo.
// Por eso exige motivo escrito.
func TestSuspenderExigeMotivo(t *testing.T) {
	repo := &fakeRepo{abrirMesCreadas: -1}
	s := svcResp(repo, nil)
	if err := s.CambiarEstadoResponsabilidad(context.Background(), "emp", "r1", RespSuspendida, "   ", "u1"); err == nil {
		t.Error("suspender sin motivo tenía que fallar")
	}
	if repo.respEstado != "" {
		t.Error("no podía llegar al repositorio")
	}
	if err := s.CambiarEstadoResponsabilidad(context.Background(), "emp", "r1", RespSuspendida, "se cerró la sede", "u1"); err != nil {
		t.Errorf("con motivo tenía que pasar: %v", err)
	}
	// Reactivar no exige motivo: volver a cumplir no necesita justificación.
	if err := s.CambiarEstadoResponsabilidad(context.Background(), "emp", "r1", RespActiva, "", "u1"); err != nil {
		t.Errorf("reactivar sin motivo tenía que pasar: %v", err)
	}
	if err := s.CambiarEstadoResponsabilidad(context.Background(), "emp", "r1", "INVENTADO", "x", "u1"); err == nil {
		t.Error("un estado inventado tenía que fallar")
	}
}
