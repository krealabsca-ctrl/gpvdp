package bancos

import (
	"context"

	"github.com/shopspring/decimal"
)

// Stubs de fakeRepo para las dimensiones del gasto y el presupuesto. Devuelven lo que el test siembre.

func (f *fakeRepo) ListarSedes(context.Context, string, bool) ([]Sede, error) {
	return f.sedes, nil
}

func (f *fakeRepo) CrearSede(_ context.Context, _, nombre, codigo string) (Sede, error) {
	s := Sede{ID: "sede-" + nombre, Nombre: nombre, Codigo: codigo, Activo: true}
	f.sedes = append(f.sedes, s)
	return s, nil
}

func (f *fakeRepo) ActualizarSede(context.Context, string, string, string, string) error { return nil }

func (f *fakeRepo) CambiarActivoSede(context.Context, string, string, bool) error { return nil }

func (f *fakeRepo) DepartamentosActivos(context.Context, string) ([]Departamento, error) {
	return f.departamentos, nil
}

func (f *fakeRepo) AsignarDimensionesClasificacion(_ context.Context, _, clasifID, deptoID, sedeID string) (int, error) {
	f.dimClasif = [3]string{clasifID, deptoID, sedeID}
	return f.movsDeClasif, nil
}

func (f *fakeRepo) AsignarDimensionesMovimiento(_ context.Context, _, movID, deptoID, sedeID string) error {
	f.dimMov = [3]string{movID, deptoID, sedeID}
	return nil
}

func (f *fakeRepo) GastoPorDimension(_ context.Context, _, _, _, agruparPor string) ([]GastoDimension, error) {
	f.agrupoPor = agruparPor
	return f.gastoDim, nil
}

func (f *fakeRepo) PartidasDeDimension(_ context.Context, _, _, _, _, dimID string) ([]GastoPartidaDimension, error) {
	f.pidioDimID = dimID
	return f.partidasDim, nil
}

func (f *fakeRepo) PresupuestoDelRango(context.Context, string, string, string) ([]PresupuestoLinea, error) {
	return f.presupuesto, nil
}

func (f *fakeRepo) GuardarPresupuesto(_ context.Context, _, deptoID, clasifID, periodo, monto, _, _ string) error {
	f.presuGuardado = [3]string{deptoID, periodo, monto}
	f.presuClasifID = clasifID
	return nil
}

func (f *fakeRepo) BorrarPresupuesto(_ context.Context, _, _, clasifID, _ string) error {
	f.presuClasifID = clasifID
	return nil
}

// ── Administración del catálogo de departamentos ─────────────────────────────

func (f *fakeRepo) CrearDepartamento(_ context.Context, _, nombre, codigo string) (Departamento, error) {
	d := Departamento{ID: "dep-" + nombre, Nombre: nombre, Codigo: codigo, Activo: true}
	f.departamentos = append(f.departamentos, d)
	return d, nil
}

func (f *fakeRepo) ActualizarDepartamento(context.Context, string, string, string, string) error {
	return nil
}

func (f *fakeRepo) CambiarActivoDepartamento(_ context.Context, _, _ string, activo bool) error {
	f.deptoActivo = activo
	return nil
}

func (f *fakeRepo) UsoDeDepartamento(context.Context, string, string) (UsoDepartamento, error) {
	return f.usoDepto, nil
}

func (f *fakeRepo) EliminarDepartamento(context.Context, string, string) error {
	f.deptoEliminado = true
	return nil
}

func (f *fakeRepo) UsoDeSede(context.Context, string, string) (UsoDepartamento, error) {
	return f.usoDepto, nil
}

func (f *fakeRepo) EliminarSede(context.Context, string, string) error {
	f.sedeEliminada = true
	return nil
}

// ── Presupuesto de INGRESO (mig 0116) ───────────────────────────────────────────────────────
//
// Los stubs devuelven lo que el test siembre. La lógica que de verdad importa —la asimetría del
// semáforo y el resultado presupuestado— es PURA y se prueba sin repositorio, en
// presupuesto_ingreso_test.go.

func (f *fakeRepo) MetasDeIngreso(context.Context, string, string, string) ([]MetaIngreso, error) {
	return f.metasIngreso, nil
}

func (f *fakeRepo) GuardarMetaIngreso(_ context.Context, _, clasifID, periodo, monto, _, _ string) error {
	f.metaGuardada = [3]string{clasifID, periodo, monto}
	return nil
}

func (f *fakeRepo) BorrarMetaIngreso(_ context.Context, _, clasifID, periodo string) error {
	f.metaBorrada = [2]string{clasifID, periodo}
	return nil
}

func (f *fakeRepo) IngresoRealPorLinea(context.Context, string, string, string) ([]FilaIngreso, error) {
	return f.ingresoReal, nil
}

func (f *fakeRepo) IngresoYGastoReal(context.Context, string, string, string) (decimal.Decimal, decimal.Decimal, error) {
	return f.ingresoTotal, f.gastoTotal, nil
}

func (f *fakeRepo) IngresoHistorico(context.Context, string, string, string) ([]FilaIngreso, error) {
	return f.ingresoHistorico, nil
}

func (f *fakeRepo) NaturalezaDeClasificacion(context.Context, string, string) (string, error) {
	if f.naturalezaClasif == "" {
		return NaturalezaIngreso, nil
	}
	return f.naturalezaClasif, nil
}

func (f *fakeRepo) RealPorMesDelAnio(context.Context, string, int) (map[string][2]decimal.Decimal, error) {
	return f.realPorMes, nil
}

func (f *fakeRepo) MetasPorMesDelAnio(context.Context, string, int) (map[string][2]decimal.Decimal, error) {
	return f.metasPorMes, nil
}

// El alcance de «Mi presupuesto» (mig 0124). `areasDelUsuario` es nil por defecto, que es el caso
// que importa: un doble que devolviera áreas por accidente haría pasar una prueba de recorte que
// en realidad no recorta nada.
func (f *fakeRepo) AreasDelUsuario(context.Context, string, string) ([]AreaDelUsuario, error) {
	return f.areasDelUsuario, nil
}

// El alcance de la mitad de ingreso (mig 0125). Nil por defecto, igual que el de gasto: un doble
// que devolviera partidas por accidente haría pasar una prueba de recorte que no recorta nada.
func (f *fakeRepo) PartidasDeIngresoDelUsuario(context.Context, string, string) ([]PartidaDelUsuario, error) {
	return f.partidasDeIngresoDelUsuario, nil
}

// El calendario real de la empresa. Nil por defecto: una empresa recién instalada no tiene nada
// cargado, y es el caso en que el selector tiene que seguir funcionando.
func (f *fakeRepo) PeriodosConMovimiento(context.Context, string) ([]string, error) {
	return f.periodosConMovimiento, nil
}

// ── El alcance por persona, desde la ficha del usuario ───────────────────────────────────────
//
// `usuarioDeLaEmpresa` arranca en FALSE a propósito: «esa persona no es de esta empresa» es el
// caso que hay que probar, y un doble que dijera que sí por omisión dejaría pasar una prueba de
// aislamiento que en realidad no aísla nada.

func (f *fakeRepo) UsuarioDeLaEmpresa(_ context.Context, empresaID, usuarioID string) (string, string, bool, error) {
	f.pidioUsuarioDeEmpresa = append(f.pidioUsuarioDeEmpresa, empresaID+"/"+usuarioID)
	if !f.usuarioDeLaEmpresa {
		return "", "", false, nil
	}
	return "Jefe de prueba", "jefe@prueba.local", true, nil
}

func (f *fakeRepo) PermisosDeAlcanceDelUsuario(context.Context, string, string) (bool, bool, error) {
	return f.puedeVerPresupuesto, f.puedeVerIngreso, nil
}

func (f *fakeRepo) GuardarAreasDelUsuario(_ context.Context, _, _ string, areas []AreaAsignada) error {
	f.areasGuardadas = areas
	return nil
}

func (f *fakeRepo) GuardarPartidasDeIngresoDelUsuario(_ context.Context, _, _ string, ids []string) error {
	f.partidasGuardadas = ids
	return nil
}

func (f *fakeRepo) NaturalezasDeClasificaciones(context.Context, string, []string) (map[string]string, error) {
	return f.naturalezas, nil
}
