package grupo

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// fakeRepo registra con qué empresas se consultó, que es justo lo que hay que poder afirmar: ninguna
// consulta de la vista puede tocar una empresa que no salió de EmpresasVisibles.
type fakeRepo struct {
	visibles []EmpresaVisible
	errVis   error
	filas    []FilaEmpresa
	internas []OperacionInterna
	partidas []PartidaGrupo
	total    int
	errTotal error

	// Lo que efectivamente se consultó.
	pedidoResumen  []string
	pedidoInternas []string
	pedidoPartidas []string
	pedidoAdmin    bool
	pedidoPermiso  string
	pedidoUsuario  string
}

func (f *fakeRepo) EmpresasVisibles(_ context.Context, usuarioID, permiso string, esAdmin bool) ([]EmpresaVisible, error) {
	f.pedidoUsuario, f.pedidoPermiso, f.pedidoAdmin = usuarioID, permiso, esAdmin
	return f.visibles, f.errVis
}

func (f *fakeRepo) ResumenPorEmpresa(_ context.Context, ids []string, _ string) ([]FilaEmpresa, error) {
	f.pedidoResumen = ids
	return f.filas, nil
}

func (f *fakeRepo) OperacionesEntreEmpresas(_ context.Context, ids []string, _ string) ([]OperacionInterna, error) {
	f.pedidoInternas = ids
	return f.internas, nil
}

func (f *fakeRepo) PartidasDelGrupo(_ context.Context, ids []string, _ string, _ int) ([]PartidaGrupo, error) {
	f.pedidoPartidas = ids
	return f.partidas, nil
}

func (f *fakeRepo) ContarEmpresas(context.Context) (int, error) { return f.total, f.errTotal }

func nuevo(r Repository) *Service { return NewService(r, zap.NewNop()) }

// Las tres empresas reales del grupo, con los números de agosto 2026 medidos en la base.
func tresEmpresas() *fakeRepo {
	return &fakeRepo{
		total: 3,
		visibles: []EmpresaVisible{
			{ID: "e-cop", Nombre: "Coopeprofa", Rol: "DIRECTOR_FINANCIERO"},
			{ID: "e-mp", Nombre: "Memorial Pets", Rol: "DIRECTOR_FINANCIERO"},
			{ID: "e-vdp", Nombre: "Valle de Paz", Rol: "DIRECTOR_FINANCIERO"},
		},
		filas: []FilaEmpresa{
			{EmpresaID: "e-cop", Empresa: "Coopeprofa", IngresosCRC: "100.00", GastosCRC: "40.00",
				NeutroCRC: "10.00", Movimientos: 10, SinClasificarCRC: "0.00", PctClasificado: "100.0"},
			{EmpresaID: "e-mp", Empresa: "Memorial Pets", IngresosCRC: "50.00", GastosCRC: "30.00",
				NeutroCRC: "0.00", Movimientos: 5, SinClasificarCRC: "0.00", PctClasificado: "100.0"},
			{EmpresaID: "e-vdp", Empresa: "Valle de Paz", IngresosCRC: "300.00", GastosCRC: "200.00",
				NeutroCRC: "25.00", Movimientos: 100, SinClasificarCRC: "0.00", PctClasificado: "100.0"},
		},
	}
}

// El total del grupo es la SUMA de las filas, sin eliminar lo intercompañía: decisión del Director
// Financiero. Si alguien "mejora" esto restando las internas, este test se pone rojo.
func TestTotalesSonLaSumaDeLasFilas(t *testing.T) {
	repo := tresEmpresas()
	res, err := nuevo(repo).Resumen(context.Background(), "u-1", "DIRECTOR_FINANCIERO", "2026-08")
	if err != nil {
		t.Fatalf("Resumen: %v", err)
	}
	if res.IngresosCRC != "450.00" || res.GastosCRC != "270.00" || res.EbitdaCRC != "180.00" {
		t.Errorf("totales = %s / %s / %s, quiere 450.00 / 270.00 / 180.00",
			res.IngresosCRC, res.GastosCRC, res.EbitdaCRC)
	}
	if res.NeutroCRC != "35.00" {
		t.Errorf("neutro = %s, quiere 35.00", res.NeutroCRC)
	}
	if res.Movimientos != 115 {
		t.Errorf("movimientos = %d, quiere 115", res.Movimientos)
	}
	// El EBITDA de cada fila lo deriva el servicio, no viene del SQL: una sola fórmula.
	if res.Empresas[0].EbitdaCRC != "60.00" {
		t.Errorf("ebitda Coopeprofa = %s, quiere 60.00", res.Empresas[0].EbitdaCRC)
	}
	if !res.Completo || res.Aviso != "" {
		t.Errorf("con las 3 empresas al 100 %% debería ser completo y sin aviso; aviso=%q", res.Aviso)
	}
}

// El alcance sale del token y ninguna consulta puede salirse de él.
func TestNingunaConsultaVeMasQueLasEmpresasVisibles(t *testing.T) {
	repo := tresEmpresas()
	repo.visibles = repo.visibles[:1] // el usuario solo ve Coopeprofa
	repo.filas = repo.filas[:1]

	if _, err := nuevo(repo).Resumen(context.Background(), "u-1", "AUXILIAR_FINANCIERO", "2026-08"); err != nil {
		t.Fatalf("Resumen: %v", err)
	}
	for nombre, ids := range map[string][]string{
		"ResumenPorEmpresa":        repo.pedidoResumen,
		"OperacionesEntreEmpresas": repo.pedidoInternas,
		"PartidasDelGrupo":         repo.pedidoPartidas,
	} {
		if len(ids) != 1 || ids[0] != "e-cop" {
			t.Errorf("%s se consultó con %v, quiere solo [e-cop]", nombre, ids)
		}
	}
	if repo.pedidoUsuario != "u-1" || repo.pedidoPermiso != PermisoLecturaBancos {
		t.Errorf("EmpresasVisibles se llamó con (%q, %q)", repo.pedidoUsuario, repo.pedidoPermiso)
	}
	if repo.pedidoAdmin {
		t.Error("un AUXILIAR_FINANCIERO no puede pedir el bypass de admin")
	}
}

// ADMIN es bypass, igual que en rbac.Tiene: si acá se resolviera distinto, un admin vería MENOS
// empresas en el consolidado que entrando de a una.
func TestAdminPideBypass(t *testing.T) {
	repo := tresEmpresas()
	if _, err := nuevo(repo).Resumen(context.Background(), "u-1", rolAdmin, "2026-08"); err != nil {
		t.Fatalf("Resumen: %v", err)
	}
	if !repo.pedidoAdmin {
		t.Error("ADMIN debería consultar con esAdmin=true")
	}
}

// Un total que omite una empresa en silencio se lee como el número del grupo. Tiene que confesarlo, y
// sin nombrar la empresa que el usuario no puede ver.
func TestTotalParcialLoConfiesa(t *testing.T) {
	repo := tresEmpresas()
	repo.visibles = repo.visibles[:2]
	repo.filas = repo.filas[:2]

	res, err := nuevo(repo).Resumen(context.Background(), "u-1", "AUXILIAR_FINANCIERO", "2026-08")
	if err != nil {
		t.Fatalf("Resumen: %v", err)
	}
	if res.Completo {
		t.Error("con 2 de 3 empresas el resumen no puede decir que está completo")
	}
	if len(res.Excluidas) != 1 || !strings.Contains(res.Excluidas[0], "1 empresa") {
		t.Errorf("excluidas = %v, quiere avisar de 1 empresa", res.Excluidas)
	}
	if !strings.Contains(res.Aviso, "no tenés acceso") {
		t.Errorf("el aviso no explica la exclusión: %q", res.Aviso)
	}
	// Y no filtra el nombre de la que no puede ver.
	if strings.Contains(res.Aviso, "Valle de Paz") {
		t.Errorf("el aviso nombra una empresa que el usuario no puede ver: %q", res.Aviso)
	}
}

// Sin acceso a ninguna empresa NO es un error del servidor: es que no hay acceso.
func TestSinEmpresasVisiblesEsErrorPropio(t *testing.T) {
	repo := &fakeRepo{total: 3, visibles: []EmpresaVisible{}}
	_, err := nuevo(repo).Resumen(context.Background(), "u-1", "AUXILIAR_FINANCIERO", "2026-08")
	if !errors.Is(err, ErrSinEmpresasVisibles) {
		t.Errorf("err = %v, quiere ErrSinEmpresasVisibles", err)
	}
}

func TestPeriodoInvalido(t *testing.T) {
	casos := []string{"", "2026", "2026-13", "2026-00", "agosto", "2026-8", "2026-08-01"}
	for _, p := range casos {
		t.Run(p, func(t *testing.T) {
			repo := tresEmpresas()
			if _, err := nuevo(repo).Resumen(context.Background(), "u-1", rolAdmin, p); !errors.Is(err, ErrPeriodoInvalido) {
				t.Errorf("periodo %q: err = %v, quiere ErrPeriodoInvalido", p, err)
			}
			if repo.pedidoUsuario != "" {
				t.Error("un período inválido no debería llegar a consultar la base")
			}
		})
	}
}

// Una empresa floja arrastra la confianza del total, y el aviso tiene que decir cuál y cuánto.
func TestEmpresaBajoElUmbralAvisa(t *testing.T) {
	repo := tresEmpresas()
	repo.filas[1].PctClasificado = "62.4"
	repo.filas[1].SinClasificarCRC = "30.00"

	res, err := nuevo(repo).Resumen(context.Background(), "u-1", rolAdmin, "2026-08")
	if err != nil {
		t.Fatalf("Resumen: %v", err)
	}
	if res.Empresas[1].Confiable {
		t.Error("62,4 % no llega al 90 %: no puede ser confiable")
	}
	if res.Completo {
		t.Error("con una empresa bajo el umbral el resumen no está completo")
	}
	if !strings.Contains(res.Aviso, "Memorial Pets al 62.4 %") {
		t.Errorf("el aviso no nombra la empresa floja: %q", res.Aviso)
	}
	if res.SinClasificarCRC != "30.00" {
		t.Errorf("sin clasificar del grupo = %s, quiere 30.00", res.SinClasificarCRC)
	}
}

// Lo intercompañía se informa, no se elimina; y solo distorsiona el resultado lo que la naturaleza
// cuenta como ingreso o gasto.
func TestEntreEmpresasSeInformaSinRestar(t *testing.T) {
	repo := tresEmpresas()
	repo.internas = []OperacionInterna{
		{Empresa: "Valle de Paz", Contraparte: "Memorial Pets", Partida: "Regalias Memorial Pets",
			Naturaleza: "GASTO", MontoCRC: "12.00", Movimientos: 1},
		{Empresa: "Coopeprofa", Contraparte: "Valle de Paz", Partida: "Traslado a Valle de Paz",
			Naturaleza: "NEUTRO", MontoCRC: "500.00", Movimientos: 3},
	}
	res, err := nuevo(repo).Resumen(context.Background(), "u-1", rolAdmin, "2026-08")
	if err != nil {
		t.Fatalf("Resumen: %v", err)
	}
	if res.EntreEmpresasCRC != "512.00" {
		t.Errorf("entre empresas = %s, quiere 512.00", res.EntreEmpresasCRC)
	}
	if res.EntreEmpresasEbitdaCRC != "12.00" {
		t.Errorf("entre empresas que afecta EBITDA = %s, quiere 12.00 (el traslado NEUTRO no cuenta)",
			res.EntreEmpresasEbitdaCRC)
	}
	// El total no se toca.
	if res.EbitdaCRC != "180.00" {
		t.Errorf("ebitda = %s: lo intercompañía no se resta del total", res.EbitdaCRC)
	}
	// El aviso NO lleva montos: el cliente los formatea. Si alguien vuelve a meter plata en la frase,
	// se separa del formato del resto del ERP (pasó: «₡1290000» arriba de una tabla con «₡1 290 000,00»).
	if strings.Contains(res.Aviso, "₡") {
		t.Errorf("el aviso no debe traer montos formateados: %q", res.Aviso)
	}
}

// Un mes sin un solo movimiento NO es un mes perfectamente clasificado. El porcentaje de un conjunto
// vacío da 100 % y pintaba de verde un período en el que nadie cargó nada.
func TestPeriodoVacioNoSeDeclaraCompleto(t *testing.T) {
	repo := tresEmpresas()
	for i := range repo.filas {
		repo.filas[i] = FilaEmpresa{
			EmpresaID: repo.filas[i].EmpresaID, Empresa: repo.filas[i].Empresa,
			IngresosCRC: "0.00", GastosCRC: "0.00", NeutroCRC: "0.00",
			Movimientos: 0, SinClasificarCRC: "0.00", PctClasificado: "100.0",
		}
	}

	res, err := nuevo(repo).Resumen(context.Background(), "u-1", rolAdmin, "2026-09")
	if err != nil {
		t.Fatalf("Resumen: %v", err)
	}
	if res.Completo {
		t.Error("un período sin movimientos no puede declararse completo")
	}
	for _, f := range res.Empresas {
		if !f.SinDatos {
			t.Errorf("%s tiene 0 movimientos: sin_datos debería ser true", f.Empresa)
		}
		if f.Confiable {
			t.Errorf("%s no tiene datos: no puede ser confiable", f.Empresa)
		}
	}
	// Con TODAS vacías se dice en una frase, sin nombrarlas una por una.
	if !strings.Contains(res.Aviso, "no tiene movimientos cargados en ninguna empresa") {
		t.Errorf("el aviso no explica que el período está vacío: %q", res.Aviso)
	}
	// Y no se cuela como si fueran empresas mal clasificadas.
	if strings.Contains(res.Aviso, "90 %") {
		t.Errorf("un período vacío no es un problema de clasificación: %q", res.Aviso)
	}
}

// Un enumerado en español lleva comas y una sola «y». Con Join(" y ") el aviso decía
// «Coopeprofa y Memorial Pets y Valle de Paz», que se lee como un error de la máquina.
func TestEnumerar(t *testing.T) {
	casos := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"A"}, "A"},
		{[]string{"A", "B"}, "A y B"},
		{[]string{"A", "B", "C"}, "A, B y C"},
		{[]string{"A", "B", "C", "D"}, "A, B, C y D"},
	}
	for _, c := range casos {
		if got := enumerar(c.in); got != c.want {
			t.Errorf("enumerar(%v) = %q, quiere %q", c.in, got, c.want)
		}
	}
}

// Una empresa vacía y otra floja son dos avisos distintos y tienen que convivir.
func TestVaciaYFlojaSeAvisanPorSeparado(t *testing.T) {
	repo := tresEmpresas()
	repo.filas[0] = FilaEmpresa{ // Coopeprofa sin datos
		EmpresaID: "e-cop", Empresa: "Coopeprofa", IngresosCRC: "0.00", GastosCRC: "0.00",
		NeutroCRC: "0.00", Movimientos: 0, SinClasificarCRC: "0.00", PctClasificado: "100.0",
	}
	repo.filas[1].PctClasificado = "71.5" // Memorial Pets floja

	res, err := nuevo(repo).Resumen(context.Background(), "u-1", rolAdmin, "2026-08")
	if err != nil {
		t.Fatalf("Resumen: %v", err)
	}
	if !strings.Contains(res.Aviso, "Coopeprofa no tiene ningún movimiento") {
		t.Errorf("falta el aviso de la empresa vacía: %q", res.Aviso)
	}
	if !strings.Contains(res.Aviso, "Memorial Pets al 71.5 %") {
		t.Errorf("falta el aviso de la empresa floja: %q", res.Aviso)
	}
	// La vacía no debe aparecer también en la lista de flojas.
	if strings.Contains(res.Aviso, "Coopeprofa al") {
		t.Errorf("la empresa vacía se está reportando como mal clasificada: %q", res.Aviso)
	}
}

// Contar las empresas del sistema es solo para el aviso: si falla, la vista igual se entrega.
func TestContarEmpresasQueFallaNoTumbaLaVista(t *testing.T) {
	repo := tresEmpresas()
	repo.errTotal = errors.New("boom")
	res, err := nuevo(repo).Resumen(context.Background(), "u-1", rolAdmin, "2026-08")
	if err != nil {
		t.Fatalf("un fallo al contar empresas no debería negar la vista: %v", err)
	}
	if res.IngresosCRC != "450.00" {
		t.Errorf("ingresos = %s, quiere 450.00", res.IngresosCRC)
	}
	if len(res.Excluidas) != 0 {
		t.Errorf("sin el total del sistema no se puede afirmar que falte alguna: %v", res.Excluidas)
	}
}

// El % clasificado se mide por MONTO, no por cantidad: 155 movimientos chicos sin partida no dicen lo
// mismo que ₡32,6M sin partida.
func TestPctClasificadoPorMonto(t *testing.T) {
	casos := []struct{ total, sin, quiere string }{
		{"100.00", "0.00", "100.0"},
		{"100.00", "10.00", "90.0"},
		{"100.00", "100.00", "0.0"},
		{"0.00", "0.00", "100.0"}, // sin movimientos no hay nada sin clasificar
		{"basura", "1.00", "100.0"},
	}
	for _, c := range casos {
		if got := pctClasificado(c.total, c.sin); got != c.quiere {
			t.Errorf("pctClasificado(%s, %s) = %s, quiere %s", c.total, c.sin, got, c.quiere)
		}
	}
}
