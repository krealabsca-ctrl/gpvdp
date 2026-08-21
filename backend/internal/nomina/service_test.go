package nomina

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// fakeRepo implementa Repository en memoria para probar las guardas del servicio.
type fakeRepo struct {
	// Notificaciones (boleta y aviso de vacaciones).
	correos          map[string]string
	vacacionAviso    VacacionAviso
	errVacacionAviso error

	empleados     map[string]Empleado
	conceptos     map[string]ConceptoNomina
	parametros    map[int]Parametros
	corr          *fakeCorrida
	finiquitos    map[string]Finiquito
	consecutivos  int
	dedsCalc      map[string][]DeduccionCalc
	incaps        []Incapacidad
	vacs          []Vacacion
	mesesServicio int // meses de servicio que simula el saldo de vacaciones
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		empleados:  map[string]Empleado{},
		conceptos:  map[string]ConceptoNomina{},
		parametros: map[int]Parametros{},
	}
}

func (f *fakeRepo) ListarEmpleados(_ context.Context, _ string, _ FiltrosEmpleado) ([]Empleado, error) {
	out := make([]Empleado, 0, len(f.empleados))
	for _, e := range f.empleados {
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeRepo) EmpleadoPorID(_ context.Context, _, id string) (Empleado, error) {
	e, ok := f.empleados[id]
	if !ok {
		return Empleado{}, ErrEmpleadoNoEncontrado
	}
	return e, nil
}

func (f *fakeRepo) CrearEmpleado(_ context.Context, _ string, in EmpleadoInput) (Empleado, error) {
	e := Empleado{ID: "emp-" + in.Identificacion, Nombre: in.Nombre, Identificacion: in.Identificacion,
		SalarioBase: in.SalarioBase.StringFixed(2), Activo: true}
	f.empleados[e.ID] = e
	return e, nil
}

func (f *fakeRepo) ActualizarEmpleado(_ context.Context, _, id string, in EmpleadoInput) (Empleado, error) {
	e, ok := f.empleados[id]
	if !ok {
		return Empleado{}, ErrEmpleadoNoEncontrado
	}
	e.Nombre, e.SalarioBase = in.Nombre, in.SalarioBase.StringFixed(2)
	f.empleados[id] = e
	return e, nil
}

func (f *fakeRepo) DesactivarEmpleado(_ context.Context, _, id, _ string) error {
	e, ok := f.empleados[id]
	if !ok {
		return ErrEmpleadoNoEncontrado
	}
	e.Activo = false
	f.empleados[id] = e
	return nil
}

func (f *fakeRepo) ParametrosPorAnio(_ context.Context, _ string, anio int) (Parametros, error) {
	p, ok := f.parametros[anio]
	if !ok {
		return Parametros{}, ErrParametrosNoEncontrados
	}
	return p, nil
}

func (f *fakeRepo) GuardarParametros(_ context.Context, _ string, anio int, in ParametrosInput) (Parametros, error) {
	p := Parametros{ID: "par-1", Anio: anio, Cargas: in.Cargas, Renta: in.Renta, Origen: "EMPRESA"}
	f.parametros[anio] = p
	return p, nil
}

func (f *fakeRepo) ListarConceptos(_ context.Context, _ string) ([]ConceptoNomina, error) {
	out := make([]ConceptoNomina, 0, len(f.conceptos))
	for _, c := range f.conceptos {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeRepo) ConceptoPorID(_ context.Context, _, id string) (ConceptoNomina, error) {
	c, ok := f.conceptos[id]
	if !ok {
		return ConceptoNomina{}, ErrConceptoNoEncontrado
	}
	return c, nil
}

func (f *fakeRepo) CrearConcepto(_ context.Context, _ string, in ConceptoInput) (ConceptoNomina, error) {
	c := ConceptoNomina{ID: "con-" + in.Nombre, Nombre: in.Nombre, Tipo: in.Tipo,
		AfectaCCSS: in.AfectaCCSS, AfectaRenta: in.AfectaRenta, AfectaAguinaldo: in.AfectaAguinaldo,
		BaseLegal: in.BaseLegal, Activo: true}
	f.conceptos[c.ID] = c
	return c, nil
}

func (f *fakeRepo) ActualizarConcepto(_ context.Context, _, id string, in ConceptoInput) (ConceptoNomina, error) {
	c, ok := f.conceptos[id]
	if !ok || c.DeSistema {
		return ConceptoNomina{}, ErrConceptoNoEncontrado
	}
	c.Nombre, c.AfectaCCSS = in.Nombre, in.AfectaCCSS
	f.conceptos[id] = c
	return c, nil
}

func (f *fakeRepo) DesactivarConcepto(_ context.Context, _, id string) error {
	c, ok := f.conceptos[id]
	if !ok || c.DeSistema {
		return ErrConceptoNoEncontrado
	}
	c.Activo = false
	f.conceptos[id] = c
	return nil
}

func (f *fakeRepo) EnsureConceptos(_ context.Context) error { return nil }

func (f *fakeRepo) ListarDeducciones(_ context.Context, _, _ string) ([]DeduccionEmpleado, error) {
	return nil, nil
}

func (f *fakeRepo) CrearDeduccion(_ context.Context, _, empleadoID string, in DeduccionInput) (DeduccionEmpleado, error) {
	if _, ok := f.empleados[empleadoID]; !ok {
		return DeduccionEmpleado{}, ErrEmpleadoNoEncontrado
	}
	return DeduccionEmpleado{ID: "ded-1", EmpleadoID: empleadoID, Etiqueta: in.Etiqueta,
		Cuota: in.Cuota.StringFixed(2), Activo: true}, nil
}

func (f *fakeRepo) ActualizarDeduccion(_ context.Context, _, _, id string, in DeduccionInput) (DeduccionEmpleado, error) {
	return DeduccionEmpleado{ID: id, Etiqueta: in.Etiqueta, Cuota: in.Cuota.StringFixed(2), Activo: true}, nil
}

func (f *fakeRepo) DesactivarDeduccion(_ context.Context, _, _, _ string) error { return nil }

// ---- Corridas (Etapa 2): el fake guarda una corrida en memoria para probar el ciclo ----

type fakeCorrida struct {
	corridas  map[string]Corrida
	lineas    map[string][]LineaCorrida
	novedades map[string][]novedadValidada
}

func (f *fakeRepo) corridaStore() *fakeCorrida {
	if f.corr == nil {
		f.corr = &fakeCorrida{corridas: map[string]Corrida{}, lineas: map[string][]LineaCorrida{}, novedades: map[string][]novedadValidada{}}
	}
	return f.corr
}

func (f *fakeRepo) ListarCorridas(_ context.Context, _ string, _ int) ([]Corrida, error) {
	st := f.corridaStore()
	out := make([]Corrida, 0, len(st.corridas))
	for _, c := range st.corridas {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeRepo) CorridaPorID(_ context.Context, _, id string) (Corrida, error) {
	c, ok := f.corridaStore().corridas[id]
	if !ok {
		return Corrida{}, ErrCorridaNoEncontrada
	}
	return c, nil
}

func (f *fakeRepo) CrearCorrida(_ context.Context, _ string, anio, mes int, tipo, fechaPago string, _ []byte, _ string) (Corrida, error) {
	st := f.corridaStore()
	for _, c := range st.corridas {
		if c.Anio == anio && c.Mes == mes && c.Tipo == tipo && c.Estado != EstAnulada {
			return Corrida{}, ErrCorridaDuplicada
		}
	}
	c := Corrida{ID: fmt.Sprintf("cor-%d", len(st.corridas)+1), Anio: anio, Mes: mes, Tipo: tipo,
		Estado: EstBorrador, FechaPago: fechaPago}
	st.corridas[c.ID] = c
	return c, nil
}

func (f *fakeRepo) LineasCorrida(_ context.Context, _, id string) ([]LineaCorrida, error) {
	return f.corridaStore().lineas[id], nil
}

func (f *fakeRepo) GuardarLineas(_ context.Context, _, id string, lineas []LineaCorrida, tot TotalesCorrida, _ []byte) error {
	st := f.corridaStore()
	c, ok := st.corridas[id]
	if !ok {
		return ErrCorridaNoEncontrada
	}
	if c.Estado != EstBorrador {
		return ErrCorridaNoEditable
	}
	st.lineas[id] = lineas
	c.Empleados = len(lineas)
	c.TotalBruto, c.TotalNeto = tot.Bruto.StringFixed(2), tot.Neto.StringFixed(2)
	c.TotalCCSS, c.TotalRenta = tot.CCSSObrero.StringFixed(2), tot.Renta.StringFixed(2)
	c.TotalDeduc, c.TotalAdel = tot.Deducciones.StringFixed(2), tot.Adelanto.StringFixed(2)
	st.corridas[id] = c
	return nil
}

func (f *fakeRepo) NovedadesCorrida(_ context.Context, _, id string) ([]NovedadCorrida, error) {
	out := []NovedadCorrida{}
	for _, n := range f.corridaStore().novedades[id] {
		out = append(out, NovedadCorrida{EmpleadoID: n.EmpleadoID, ConceptoID: n.ConceptoID, Monto: n.Monto.StringFixed(2)})
	}
	return out, nil
}

func (f *fakeRepo) ReemplazarNovedades(_ context.Context, _, id string, novedades []novedadValidada) error {
	for _, n := range novedades {
		c, ok := f.conceptos[n.ConceptoID]
		if !ok || c.Tipo != ConceptoIngreso || !c.Activo {
			return ErrNovedadInvalida
		}
		if _, ok := f.empleados[n.EmpleadoID]; !ok {
			return ErrNovedadInvalida
		}
	}
	f.corridaStore().novedades[id] = novedades
	return nil
}

func (f *fakeRepo) NovedadesParaCalc(_ context.Context, _, id string) (map[string][]IngresoCalc, error) {
	out := map[string][]IngresoCalc{}
	for _, n := range f.corridaStore().novedades[id] {
		c := f.conceptos[n.ConceptoID]
		out[n.EmpleadoID] = append(out[n.EmpleadoID], IngresoCalc{Nombre: c.Nombre, Monto: n.Monto,
			AfectaCCSS: c.AfectaCCSS, AfectaRenta: c.AfectaRenta, AfectaAguinaldo: c.AfectaAguinaldo})
	}
	return out, nil
}

func (f *fakeRepo) DeduccionesParaCalc(_ context.Context, _ string) (map[string][]DeduccionCalc, error) {
	if f.dedsCalc == nil {
		return map[string][]DeduccionCalc{}, nil
	}
	return f.dedsCalc, nil
}

func (f *fakeRepo) RentaRetenidaPrimeraQuincena(_ context.Context, _ string, anio, mes int) (map[string]decimal.Decimal, error) {
	st := f.corridaStore()
	out := map[string]decimal.Decimal{}
	for _, c := range st.corridas {
		if c.Anio == anio && c.Mes == mes && c.Tipo == CorridaAdelanto &&
			(c.Estado == EstAprobada || c.Estado == EstPagada) {
			for _, l := range st.lineas[c.ID] {
				if l.Tratamiento == TratQuincena1 {
					out[l.EmpleadoID] = decimal.RequireFromString(l.Renta)
				}
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) AdelantosPagadosDelMes(_ context.Context, _ string, anio, mes int) (map[string]decimal.Decimal, error) {
	st := f.corridaStore()
	out := map[string]decimal.Decimal{}
	for _, c := range st.corridas {
		if c.Anio == anio && c.Mes == mes && c.Tipo == CorridaAdelanto && (c.Estado == EstAprobada || c.Estado == EstPagada) {
			for _, l := range st.lineas[c.ID] {
				// Solo los adelantos verdaderos se descuentan (una 1ª quincena es salario).
				if l.Tratamiento == TratAdelanto {
					out[l.EmpleadoID] = decimal.RequireFromString(l.Neto)
				}
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) cambiarEstadoCorrida(id, de, a string) (int64, error) {
	st := f.corridaStore()
	c, ok := st.corridas[id]
	if !ok || c.Estado != de {
		return 0, nil
	}
	c.Estado = a
	st.corridas[id] = c
	return 1, nil
}

func (f *fakeRepo) ExisteAdelantoBorrador(_ context.Context, _ string, anio, mes int) (bool, error) {
	for _, c := range f.corridaStore().corridas {
		if c.Anio == anio && c.Mes == mes && c.Tipo == CorridaAdelanto && c.Estado == EstBorrador {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) LiquidacionCerradaDelMes(_ context.Context, _ string, anio, mes int) (bool, error) {
	for _, c := range f.corridaStore().corridas {
		if c.Anio == anio && c.Mes == mes && c.Tipo == CorridaLiquidacion &&
			(c.Estado == EstAprobada || c.Estado == EstPagada) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) TieneNetoNegativo(_ context.Context, _, corridaID string) (bool, error) {
	for _, l := range f.corridaStore().lineas[corridaID] {
		if decimal.RequireFromString(l.Neto).IsNegative() {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) AdelantosSinColilla(_ context.Context, _ string, anio, mes int, liquidacionID string) (bool, error) {
	st := f.corridaStore()
	enLiq := map[string]bool{}
	for _, l := range st.lineas[liquidacionID] {
		enLiq[l.EmpleadoID] = true
	}
	for _, c := range st.corridas {
		if c.Anio == anio && c.Mes == mes && c.Tipo == CorridaAdelanto && (c.Estado == EstAprobada || c.Estado == EstPagada) {
			for _, l := range st.lineas[c.ID] {
				if !enLiq[l.EmpleadoID] {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (f *fakeRepo) AprobarCorrida(ctx context.Context, empresaID, id, _ string) (int64, error) {
	// Espejo de la guarda SQL cruzada ADELANTO↔LIQUIDACIÓN.
	c, ok := f.corridaStore().corridas[id]
	if !ok || c.Estado != EstBorrador {
		return 0, nil
	}
	if c.Tipo == CorridaLiquidacion {
		if pendiente, _ := f.ExisteAdelantoBorrador(ctx, empresaID, c.Anio, c.Mes); pendiente {
			return 0, nil
		}
	} else {
		if cerrada, _ := f.LiquidacionCerradaDelMes(ctx, empresaID, c.Anio, c.Mes); cerrada {
			return 0, nil
		}
	}
	return f.cambiarEstadoCorrida(id, EstBorrador, EstAprobada)
}

func (f *fakeRepo) PagarCorrida(_ context.Context, _, id, _ string) (int64, error) {
	return f.cambiarEstadoCorrida(id, EstAprobada, EstPagada)
}

func (f *fakeRepo) AnularCorrida(ctx context.Context, empresaID, id string) (int64, error) {
	c, ok := f.corridaStore().corridas[id]
	if !ok {
		return 0, nil
	}
	// Espejo de la guarda SQL: un ADELANTO aprobado ya descontado no se anula.
	if c.Tipo == CorridaAdelanto && c.Estado == EstAprobada {
		if cerrada, _ := f.LiquidacionCerradaDelMes(ctx, empresaID, c.Anio, c.Mes); cerrada {
			return 0, nil
		}
	}
	if n, _ := f.cambiarEstadoCorrida(id, EstBorrador, EstAnulada); n == 1 {
		return 1, nil
	}
	return f.cambiarEstadoCorrida(id, EstAprobada, EstAnulada)
}

// ---- Finiquitos (Etapa 3): fake en memoria ----

func (f *fakeRepo) finiquitoStore() map[string]Finiquito {
	if f.finiquitos == nil {
		f.finiquitos = map[string]Finiquito{}
	}
	return f.finiquitos
}

func (f *fakeRepo) ListarFiniquitos(_ context.Context, _ string) ([]Finiquito, error) {
	st := f.finiquitoStore()
	out := make([]Finiquito, 0, len(st))
	for _, fi := range st {
		out = append(out, fi)
	}
	return out, nil
}

func (f *fakeRepo) FiniquitoPorID(_ context.Context, _, id string) (Finiquito, error) {
	fi, ok := f.finiquitoStore()[id]
	if !ok {
		return Finiquito{}, ErrFiniquitoNoEncontrado
	}
	return fi, nil
}

func (f *fakeRepo) EmpleadosParaCorrida(_ context.Context, _ string, _, _ int, tipo string) ([]EmpleadoCorrida, error) {
	out := make([]EmpleadoCorrida, 0, len(f.empleados))
	for _, e := range f.empleados {
		if !e.Activo {
			continue
		}
		if tipo == CorridaAdelanto {
			omitir := false
			for _, fi := range f.finiquitoStore() {
				if fi.EmpleadoID == e.ID && (fi.Estado == FinAprobado || fi.Estado == FinPagado) {
					omitir = true
				}
			}
			if omitir {
				continue
			}
		}
		out = append(out, EmpleadoCorrida{Empleado: e, FraccionMes: "1"})
	}
	return out, nil
}

func (f *fakeRepo) GuardarFiniquito(_ context.Context, _, id string, in FiniquitoInput, res ResultadoFiniquito, promedio, dias decimal.Decimal, _ string) (Finiquito, error) {
	st := f.finiquitoStore()
	if id == "" {
		for _, fi := range st {
			if fi.EmpleadoID == in.EmpleadoID && fi.Estado != FinAnulado {
				return Finiquito{}, ErrFiniquitoDuplicado
			}
		}
		id = fmt.Sprintf("fin-%d", len(st)+1)
		st[id] = Finiquito{ID: id, EmpleadoID: in.EmpleadoID, Estado: FinBorrador}
	}
	fi := st[id]
	if fi.Estado != FinBorrador {
		return Finiquito{}, ErrFiniquitoNoEditable
	}
	emp := f.empleados[in.EmpleadoID]
	fi.EmpleadoNombre, fi.Identificacion, fi.FechaIngreso = emp.Nombre, emp.Identificacion, emp.FechaIngreso
	fi.Motivo, fi.FechaSalida, fi.DiasVacaciones = in.Motivo, in.FechaSalida, dias.StringFixed(2)
	fi.DiasManual = in.DiasManual
	fi.SalarioPromedio, fi.SalarioDiario = promedio.StringFixed(2), res.SalarioDiario.StringFixed(2)
	fi.AniosServicio = res.AniosServicio
	fi.Preaviso, fi.Cesantia = res.Preaviso.StringFixed(2), res.Cesantia.StringFixed(2)
	fi.Vacaciones, fi.Aguinaldo = res.Vacaciones.StringFixed(2), res.Aguinaldo.StringFixed(2)
	fi.BaseCCSS, fi.CCSSObrero, fi.Renta = res.BaseCCSS.StringFixed(2), res.CCSSObrero.StringFixed(2), res.Renta.StringFixed(2)
	fi.Descuentos, fi.Total = res.Descuentos.StringFixed(2), res.Total.StringFixed(2)
	fi.Detalle = res.Detalle
	st[id] = fi
	return fi, nil
}

func (f *fakeRepo) cambiarEstadoFiniquito(id, de, a string) (int64, error) {
	st := f.finiquitoStore()
	fi, ok := st[id]
	if !ok || fi.Estado != de {
		return 0, nil
	}
	fi.Estado = a
	st[id] = fi
	return 1, nil
}

func (f *fakeRepo) AprobarFiniquito(_ context.Context, _, id, _ string, motivo, fechaSalida, dias string) (int64, error) {
	// Espejo del locking optimista del UPDATE real.
	fi, ok := f.finiquitoStore()[id]
	if !ok || fi.Motivo != motivo || fi.FechaSalida != fechaSalida ||
		!decimal.RequireFromString(fi.DiasVacaciones).Equal(decimal.RequireFromString(dias)) {
		return 0, nil
	}
	return f.cambiarEstadoFiniquito(id, FinBorrador, FinAprobado)
}

func (f *fakeRepo) PagarFiniquito(_ context.Context, _, id, _ string) (int64, error) {
	n, _ := f.cambiarEstadoFiniquito(id, FinAprobado, FinPagado)
	if n == 1 {
		fi := f.finiquitoStore()[id]
		if e, ok := f.empleados[fi.EmpleadoID]; ok {
			e.Activo = false
			e.FechaSalida = fi.FechaSalida
			f.empleados[fi.EmpleadoID] = e
		}
	}
	return n, nil
}

func (f *fakeRepo) AnularFiniquito(_ context.Context, _, id string) (int64, error) {
	if n, _ := f.cambiarEstadoFiniquito(id, FinBorrador, FinAnulado); n == 1 {
		return 1, nil
	}
	return f.cambiarEstadoFiniquito(id, FinAprobado, FinAnulado)
}

func (f *fakeRepo) SalarioPromedioEmpleado(_ context.Context, _, empleadoID string) (decimal.Decimal, error) {
	e, ok := f.empleados[empleadoID]
	if !ok {
		return decimal.Zero, ErrEmpleadoNoEncontrado
	}
	return decimal.NewFromString(e.SalarioBase)
}

func (f *fakeRepo) AdelantoPendienteEmpleado(_ context.Context, _, _ string, _, _ int) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (f *fakeRepo) ProvisionesEmpleado(_ context.Context, _, _ string) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (f *fakeRepo) ProvisionesAnio(_ context.Context, _ string, _ int) ([]ProvisionEmpleadoAnio, error) {
	return nil, nil
}

func (f *fakeRepo) LineasParaArchivo(_ context.Context, _, corridaID string) ([]LineaArchivoPago, int, error) {
	out := []LineaArchivoPago{}
	sinIBAN := 0
	for _, l := range f.corridaStore().lineas[corridaID] {
		if l.IBAN == "" {
			sinIBAN++
			continue
		}
		out = append(out, LineaArchivoPago{TipoIdentificacion: "CEDULA", Identificacion: l.Identificacion,
			Nombre: l.Nombre, IBAN: l.IBAN, Neto: l.Neto})
	}
	return out, sinIBAN, nil
}

func (f *fakeRepo) FiniquitosDelMes(_ context.Context, _ string, anio, mes int) ([]FiniquitoDelMes, error) {
	out := []FiniquitoDelMes{}
	prefijo := fmt.Sprintf("%d-%02d", anio, mes)
	for _, fi := range f.finiquitoStore() {
		if fi.Estado != FinAprobado && fi.Estado != FinPagado {
			continue
		}
		if !strings.HasPrefix(fi.FechaSalida, prefijo) {
			continue
		}
		out = append(out, FiniquitoDelMes{ID: fi.ID, Nombre: fi.EmpleadoNombre,
			TipoIdentificacion: "CEDULA", Identificacion: fi.Identificacion,
			IBAN: f.empleados[fi.EmpleadoID].IBAN, BaseCCSS: fi.BaseCCSS,
			CCSSObrero: fi.CCSSObrero, Patronal: fi.Patronal, Total: fi.Total, Estado: fi.Estado})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nombre < out[j].Nombre })
	return out, nil
}

// ---- Dashboard (Etapa 5): agregados en memoria sobre las corridas vivas del mes ----

// lineasVivasDelMes devuelve las colillas de las corridas no anuladas del mes.
func (f *fakeRepo) lineasVivasDelMes(anio, mes int) []LineaCorrida {
	st := f.corridaStore()
	out := []LineaCorrida{}
	for id, c := range st.corridas {
		if c.Anio != anio || c.Mes != mes || c.Estado == EstAnulada {
			continue
		}
		out = append(out, st.lineas[id]...)
	}
	return out
}

func (f *fakeRepo) ResumenNominaMes(_ context.Context, _ string, anio, mes int) (ResumenMes, error) {
	st := f.corridaStore()
	bruto, base, patronal := decimal.Zero, decimal.Zero, decimal.Zero
	agui, vac, ces := decimal.Zero, decimal.Zero, decimal.Zero
	neto, netoLiq := decimal.Zero, decimal.Zero
	empleados := map[string]bool{}
	for id, c := range st.corridas {
		if c.Anio != anio || c.Mes != mes || c.Estado == EstAnulada {
			continue
		}
		for _, l := range st.lineas[id] {
			if l.Tratamiento != TratAdelanto {
				bruto = bruto.Add(dec(l.Bruto))
			}
			base = base.Add(dec(l.BaseCCSS))
			patronal = patronal.Add(dec(l.Patronal))
			agui, vac, ces = agui.Add(dec(l.ProvAguinaldo)), vac.Add(dec(l.ProvVacaciones)), ces.Add(dec(l.ProvCesantia))
			neto = neto.Add(dec(l.Neto))
			if c.Tipo == CorridaLiquidacion {
				netoLiq = netoLiq.Add(dec(l.Neto))
			}
			empleados[l.EmpleadoID] = true
		}
	}
	return ResumenMes{Bruto: bruto.StringFixed(2), BaseCCSS: base.StringFixed(2),
		Patronal: patronal.StringFixed(2), ProvAguinaldo: agui.StringFixed(2),
		ProvVacaciones: vac.StringFixed(2), ProvCesantia: ces.StringFixed(2),
		Neto: neto.StringFixed(2), NetoLiquidacion: netoLiq.StringFixed(2),
		Empleados: len(empleados)}, nil
}

func (f *fakeRepo) TendenciaCostoNomina(_ context.Context, _, desde, hasta string) ([]CostoMes, error) {
	st := f.corridaStore()
	porMes := map[int]decimal.Decimal{}
	for id, c := range st.corridas {
		if c.Estado == EstAnulada {
			continue
		}
		clave := fmt.Sprintf("%04d-%02d-01", c.Anio, c.Mes)
		if clave < desde || clave > hasta {
			continue
		}
		total := porMes[c.Anio*100+c.Mes]
		for _, l := range st.lineas[id] {
			if l.Tratamiento != TratAdelanto {
				total = total.Add(dec(l.Bruto))
			}
			total = total.Add(dec(l.Patronal)).Add(dec(l.ProvAguinaldo)).
				Add(dec(l.ProvVacaciones)).Add(dec(l.ProvCesantia))
		}
		porMes[c.Anio*100+c.Mes] = total
	}
	out := make([]CostoMes, 0, len(porMes))
	for clave, total := range porMes {
		out = append(out, CostoMes{Anio: clave / 100, Mes: clave % 100, Costo: total.StringFixed(2)})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Anio*100+out[i].Mes < out[j].Anio*100+out[j].Mes
	})
	return out, nil
}

func (f *fakeRepo) CorridasVivasDelMes(_ context.Context, _ string, anio, mes int) ([]EstadoCorridaMes, error) {
	out := []EstadoCorridaMes{}
	for _, c := range f.corridaStore().corridas {
		if c.Anio == anio && c.Mes == mes && c.Estado != EstAnulada {
			out = append(out, EstadoCorridaMes{ID: c.ID, Tipo: c.Tipo, Estado: c.Estado})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tipo < out[j].Tipo })
	return out, nil
}

func (f *fakeRepo) HeadcountPorDepartamento(_ context.Context, _ string) ([]DashboardDepto, error) {
	conteo := map[string]int{}
	for _, e := range f.empleados {
		if !e.Activo {
			continue
		}
		depto := e.DepartamentoNombre
		if depto == "" {
			depto = "Sin departamento"
		}
		conteo[depto]++
	}
	out := make([]DashboardDepto, 0, len(conteo))
	for d, n := range conteo {
		out = append(out, DashboardDepto{Departamento: d, Empleados: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Empleados != out[j].Empleados {
			return out[i].Empleados > out[j].Empleados
		}
		return out[i].Departamento < out[j].Departamento
	})
	return out, nil
}

func (f *fakeRepo) AvisosNominaMes(_ context.Context, _ string, _, _ int) (AvisosNomina, error) {
	a := AvisosNomina{NombresSinIBAN: []string{}, SaldoDeducciones: "0.00"}
	for _, e := range f.empleados {
		if e.Activo && e.IBAN == "" {
			a.SinIBAN++
			a.NombresSinIBAN = append(a.NombresSinIBAN, e.Nombre)
		}
	}
	sort.Strings(a.NombresSinIBAN)
	saldo := decimal.Zero
	for _, deds := range f.dedsCalc {
		for _, d := range deds {
			if d.SaldoRestante != nil && d.SaldoRestante.IsPositive() {
				a.DeduccionesActivas++
				saldo = saldo.Add(*d.SaldoRestante)
			}
		}
	}
	a.SaldoDeducciones = saldo.StringFixed(2)
	for _, c := range f.conceptos {
		if c.Activo && c.Tipo == "INGRESO" && !c.AfectaCCSS {
			a.ConceptosNoAfectos++
			if c.BaseLegal == "" {
				a.SinBaseLegal++
			}
		}
	}
	for _, i := range f.incaps {
		if !i.Anulada {
			a.IncapacidadesMes++
		}
	}
	return a, nil
}

func (f *fakeRepo) RegistrarArchivoPago(_ context.Context, _, _ string, _ int, _ decimal.Decimal, _ string) (int, error) {
	f.consecutivos++
	return f.consecutivos, nil
}

// ---- Incapacidades y vacaciones (Etapa 4): fake en memoria ----

func (f *fakeRepo) ListarIncapacidades(_ context.Context, _ string, _, _ int) ([]Incapacidad, error) {
	return f.incaps, nil
}

func (f *fakeRepo) IncapacidadPorID(_ context.Context, _, id string) (Incapacidad, error) {
	for _, i := range f.incaps {
		if i.ID == id {
			return i, nil
		}
	}
	return Incapacidad{}, ErrIncapacidadNoEncontrada
}

func (f *fakeRepo) CrearIncapacidad(_ context.Context, _ string, in IncapacidadInput, _ string) (Incapacidad, error) {
	e, ok := f.empleados[in.EmpleadoID]
	if !ok {
		return Incapacidad{}, ErrEmpleadoNoEncontrado
	}
	i := Incapacidad{ID: fmt.Sprintf("inc-%d", len(f.incaps)+1), EmpleadoID: in.EmpleadoID,
		EmpleadoNombre: e.Nombre, Entidad: in.Entidad, FechaInicio: in.FechaInicio, Dias: in.Dias}
	f.incaps = append(f.incaps, i)
	return i, nil
}

func (f *fakeRepo) AnularIncapacidad(_ context.Context, _, id string) error {
	for k, i := range f.incaps {
		if i.ID == id && !i.Anulada {
			f.incaps[k].Anulada = true
			return nil
		}
	}
	return ErrIncapacidadNoEncontrada
}

func (f *fakeRepo) IncapacidadesParaCalc(_ context.Context, _ string, anio, mes int) (map[string][]IncapacidadCalc, error) {
	out := map[string][]IncapacidadCalc{}
	for _, i := range f.incaps {
		if i.Anulada {
			continue
		}
		inicio, err := time.Parse("2006-01-02", i.FechaInicio)
		if err != nil {
			continue
		}
		out[i.EmpleadoID] = append(out[i.EmpleadoID], IncapacidadCalc{
			ID: i.ID, Entidad: i.Entidad, FechaInicio: inicio, Dias: i.Dias})
	}
	return out, nil
}

func (f *fakeRepo) ListarVacaciones(_ context.Context, _, _ string) ([]Vacacion, error) {
	return f.vacs, nil
}

func (f *fakeRepo) CrearVacacion(_ context.Context, _ string, in VacacionInput, _ decimal.Decimal, _ string) (Vacacion, error) {
	e, ok := f.empleados[in.EmpleadoID]
	if !ok {
		return Vacacion{}, ErrEmpleadoNoEncontrado
	}
	v := Vacacion{ID: fmt.Sprintf("vac-%d", len(f.vacs)+1), EmpleadoID: in.EmpleadoID,
		EmpleadoNombre: e.Nombre, FechaInicio: in.FechaInicio, Dias: in.Dias}
	f.vacs = append(f.vacs, v)
	return v, nil
}

func (f *fakeRepo) AnularVacacion(_ context.Context, _, id string) error {
	for k, v := range f.vacs {
		if v.ID == id && !v.Anulada {
			f.vacs[k].Anulada = true
			return nil
		}
	}
	return ErrVacacionNoEncontrada
}

func (f *fakeRepo) SaldosVacaciones(ctx context.Context, empresaID string, diasPorMes decimal.Decimal) ([]SaldoVacaciones, error) {
	out := make([]SaldoVacaciones, 0, len(f.empleados))
	for id := range f.empleados {
		s, err := f.SaldoVacacionesEmpleado(ctx, empresaID, id, diasPorMes)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeRepo) SaldoVacacionesEmpleado(_ context.Context, _, empleadoID string, diasPorMes decimal.Decimal) (SaldoVacaciones, error) {
	e, ok := f.empleados[empleadoID]
	if !ok {
		return SaldoVacaciones{}, ErrEmpleadoNoEncontrado
	}
	meses := f.mesesServicio
	acumulado := diasPorMes.Mul(decimal.NewFromInt(int64(meses)))
	disfrutado := decimal.Zero
	for _, v := range f.vacs {
		if v.EmpleadoID == empleadoID && !v.Anulada {
			disfrutado = disfrutado.Add(decimal.RequireFromString(v.Dias))
		}
	}
	return SaldoVacaciones{EmpleadoID: empleadoID, Nombre: e.Nombre, Identificacion: e.Identificacion,
		FechaIngreso: e.FechaIngreso, MesesServicio: meses,
		Acumulado: acumulado.StringFixed(2), Disfrutado: disfrutado.StringFixed(2),
		Pendiente: restarPiso0(acumulado, disfrutado).StringFixed(2)}, nil
}

func (f *fakeRepo) CorridaCerradaDelMes(_ context.Context, _ string, anio, mes int, tipo string) (bool, error) {
	for _, c := range f.corridaStore().corridas {
		if c.Anio == anio && c.Mes == mes && (tipo == "" || c.Tipo == tipo) &&
			(c.Estado == EstAprobada || c.Estado == EstPagada) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) LineasPlanillaDelMes(_ context.Context, _ string, anio, mes int) ([]LineaCorrida, error) {
	st := f.corridaStore()
	porEmpleado := map[string]LineaCorrida{}
	for _, c := range st.corridas {
		if c.Anio != anio || c.Mes != mes || (c.Estado != EstAprobada && c.Estado != EstPagada) {
			continue
		}
		for _, l := range st.lineas[c.ID] {
			acc, ok := porEmpleado[l.EmpleadoID]
			if !ok {
				porEmpleado[l.EmpleadoID] = LineaCorrida{Nombre: l.Nombre, Identificacion: l.Identificacion,
					BaseCCSS: l.BaseCCSS, CCSSObrero: l.CCSSObrero, Patronal: l.Patronal}
				continue
			}
			suma := func(a, b string) string {
				return decimal.RequireFromString(a).Add(decimal.RequireFromString(b)).StringFixed(2)
			}
			acc.BaseCCSS = suma(acc.BaseCCSS, l.BaseCCSS)
			acc.CCSSObrero = suma(acc.CCSSObrero, l.CCSSObrero)
			acc.Patronal = suma(acc.Patronal, l.Patronal)
			porEmpleado[l.EmpleadoID] = acc
		}
	}
	out := make([]LineaCorrida, 0, len(porEmpleado))
	for _, l := range porEmpleado {
		out = append(out, l)
	}
	return out, nil
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// GUARDARRAÍL: los conceptos de sistema (comisiones, bonos habituales…) no se editan ni
// desactivan — sus banderas de afectación CCSS están bloqueadas por ley.
func TestGuardarrailConceptoDeSistema(t *testing.T) {
	repo := newFakeRepo()
	repo.conceptos["sys-1"] = ConceptoNomina{ID: "sys-1", Nombre: "Comisiones", Tipo: ConceptoIngreso,
		AfectaCCSS: true, AfectaRenta: true, AfectaAguinaldo: true, DeSistema: true, Activo: true}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	// Intento de apagar afecta_ccss en Comisiones (subdeclaración): rechazado.
	_, err := svc.ActualizarConcepto(ctx, "e1", "sys-1", ConceptoInput{
		Nombre: "Comisiones", Tipo: ConceptoIngreso, AfectaCCSS: false, BaseLegal: "ninguna"}, "u1")
	if !errors.Is(err, ErrConceptoDeSistema) {
		t.Fatalf("editar concepto de sistema: err = %v, quiere ErrConceptoDeSistema", err)
	}
	if err := svc.DesactivarConcepto(ctx, "e1", "sys-1", "u1"); !errors.Is(err, ErrConceptoDeSistema) {
		t.Fatalf("desactivar concepto de sistema: err = %v, quiere ErrConceptoDeSistema", err)
	}
}

// GUARDARRAÍL: un INGRESO no afecto a CCSS exige base legal (viáticos, reembolsos…).
func TestConceptoNoSalarialExigeBaseLegal(t *testing.T) {
	svc := NewService(newFakeRepo(), nil, nil)
	ctx := context.Background()

	_, err := svc.CrearConcepto(ctx, "e1", ConceptoInput{
		Nombre: "Plus discrecional", Tipo: ConceptoIngreso, AfectaCCSS: false}, "u1")
	if !errors.Is(err, ErrBaseLegalRequerida) {
		t.Fatalf("ingreso sin CCSS y sin base legal: err = %v, quiere ErrBaseLegalRequerida", err)
	}
	// Con base legal citada, sí procede.
	if _, err := svc.CrearConcepto(ctx, "e1", ConceptoInput{
		Nombre: "Viáticos liquidados", Tipo: ConceptoIngreso, AfectaCCSS: false,
		BaseLegal: "Reglamento de gastos de viaje CGR — no salarial con liquidación"}, "u1"); err != nil {
		t.Fatalf("ingreso no salarial con base legal: err = %v, quiere nil", err)
	}
	// Un ingreso salarial normal no exige base legal.
	if _, err := svc.CrearConcepto(ctx, "e1", ConceptoInput{
		Nombre: "Recargo nocturno", Tipo: ConceptoIngreso, AfectaCCSS: true, AfectaRenta: true, AfectaAguinaldo: true}, "u1"); err != nil {
		t.Fatalf("ingreso salarial: err = %v, quiere nil", err)
	}
}

func TestCrearEmpleadoExigeSalarioPositivo(t *testing.T) {
	svc := NewService(newFakeRepo(), nil, nil)
	ctx := context.Background()

	_, err := svc.CrearEmpleado(ctx, "e1", EmpleadoInput{Nombre: "Ana", Identificacion: "1-111", SalarioBase: decimal.Zero}, "u1")
	if !errors.Is(err, ErrSalarioInvalido) {
		t.Fatalf("salario 0: err = %v, quiere ErrSalarioInvalido", err)
	}
	if _, err := svc.CrearEmpleado(ctx, "e1", EmpleadoInput{Nombre: "Ana", Identificacion: "1-111", SalarioBase: dec("480000")}, "u1"); err != nil {
		t.Fatalf("salario válido: err = %v, quiere nil", err)
	}
}

func TestValidarParametros(t *testing.T) {
	base := ParametrosDefault2026(2026)
	valido := ParametrosInput{Cargas: base.Cargas, Renta: base.Renta,
		INSRiesgosPct: dec("1"), AdelantoPct: dec("50"),
		AdelantoBase: "SALARIO_BASE", Redondeo: "COLON", ProvisionBase: "REMUNERACION_TOTAL"}
	if err := validarParametros(valido); err != nil {
		t.Fatalf("defaults CR 2026: err = %v, quiere nil", err)
	}

	// Sin cargas patronales: incompleto.
	soloObreras := valido
	soloObreras.Cargas = []Carga{{Codigo: "SEM_OBR", Nombre: "SEM", Tipo: CargaObrero, Pct: "5.50"}}
	if err := validarParametros(soloObreras); !errors.Is(err, ErrCargasIncompletas) {
		t.Fatalf("solo obreras: err = %v, quiere ErrCargasIncompletas", err)
	}

	// Porcentaje no numérico.
	malPct := valido
	malPct.Cargas = append([]Carga{}, valido.Cargas...)
	malPct.Cargas[0] = Carga{Codigo: "X", Nombre: "X", Tipo: CargaObrero, Pct: "cinco"}
	if err := validarParametros(malPct); !errors.Is(err, ErrCargaInvalida) {
		t.Fatalf("pct no numérico: err = %v, quiere ErrCargaInvalida", err)
	}

	// Tramos desordenados (el segundo termina antes que el primero).
	h1, h2 := "918000", "500000"
	malTramos := valido
	malTramos.Renta = RentaConfig{Tramos: []TramoRenta{{Hasta: &h1, Pct: "0"}, {Hasta: &h2, Pct: "10"}, {Hasta: nil, Pct: "15"}}}
	if err := validarParametros(malTramos); !errors.Is(err, ErrTramosInvalidos) {
		t.Fatalf("tramos desordenados: err = %v, quiere ErrTramosInvalidos", err)
	}

	// El último tramo debe ser abierto.
	cerrado := valido
	cerrado.Renta = RentaConfig{Tramos: []TramoRenta{{Hasta: &h1, Pct: "0"}}}
	if err := validarParametros(cerrado); !errors.Is(err, ErrTramosInvalidos) {
		t.Fatalf("sin tramo abierto: err = %v, quiere ErrTramosInvalidos", err)
	}
}

// Sin parámetros guardados, el servicio devuelve los legales de referencia (DEFAULT);
// tras guardar, devuelve los de la empresa.
func TestParametrosDefaultHastaGuardar(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	p, err := svc.Parametros(ctx, "e1", 2026)
	if err != nil || p.Origen != "DEFAULT" {
		t.Fatalf("sin guardar: origen = %q err = %v, quiere DEFAULT", p.Origen, err)
	}
	base := ParametrosDefault2026(2026)
	if _, err := svc.GuardarParametros(ctx, "e1", 2026, ParametrosInput{Cargas: base.Cargas, Renta: base.Renta,
		INSRiesgosPct: dec("1"), AdelantoPct: dec("50"), AdelantoBase: "SALARIO_BASE",
		Redondeo: "COLON", ProvisionBase: "REMUNERACION_TOTAL"}, "u1"); err != nil {
		t.Fatalf("guardar: err = %v", err)
	}
	p, err = svc.Parametros(ctx, "e1", 2026)
	if err != nil || p.Origen != "EMPRESA" {
		t.Fatalf("tras guardar: origen = %q err = %v, quiere EMPRESA", p.Origen, err)
	}
}

func TestDeduccionGuards(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "Ana", Activo: true}
	repo.conceptos["ded-prestamo"] = ConceptoNomina{ID: "ded-prestamo", Nombre: "Préstamo", Tipo: ConceptoDeduccion, Activo: true}
	repo.conceptos["ing-salario"] = ConceptoNomina{ID: "ing-salario", Nombre: "Salario", Tipo: ConceptoIngreso, Activo: true}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	// Cuota cero: inválida.
	_, err := svc.CrearDeduccion(ctx, "e1", "emp-1", DeduccionInput{ConceptoID: "ded-prestamo", Etiqueta: "Préstamo Asociación", Cuota: decimal.Zero}, "u1")
	if !errors.Is(err, ErrDeduccionInvalida) {
		t.Fatalf("cuota 0: err = %v, quiere ErrDeduccionInvalida", err)
	}
	// Concepto de tipo INGRESO: no sirve como deducción.
	_, err = svc.CrearDeduccion(ctx, "e1", "emp-1", DeduccionInput{ConceptoID: "ing-salario", Etiqueta: "X", Cuota: dec("1000")}, "u1")
	if !errors.Is(err, ErrConceptoNoEsDeduccion) {
		t.Fatalf("concepto ingreso: err = %v, quiere ErrConceptoNoEsDeduccion", err)
	}
	// Válida, con saldo total (préstamo con tope).
	saldo := dec("315000")
	if _, err := svc.CrearDeduccion(ctx, "e1", "emp-1", DeduccionInput{
		ConceptoID: "ded-prestamo", Etiqueta: "Préstamo Asociación", Cuota: dec("45000"), SaldoTotal: &saldo}, "u1"); err != nil {
		t.Fatalf("deducción válida: err = %v, quiere nil", err)
	}
}

// Ciclo completo de corrida: adelanto (día 15) → aprobar → liquidación que descuenta el
// adelanto REAL → aprobar → pagar; con las guardas de duplicado, novedades y estados.
func TestCicloCorrida(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "María", Identificacion: "1-111",
		SalarioBase: "480000.00", Hijos: 2, Activo: true}
	repo.conceptos["com"] = ConceptoNomina{ID: "com", Nombre: "Comisiones", Tipo: ConceptoIngreso,
		AfectaCCSS: true, AfectaRenta: true, AfectaAguinaldo: true, DeSistema: true, Activo: true}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	// Adelanto del mes: 50% del salario base.
	adel, err := svc.CrearCorrida(ctx, "e1", 2026, 8, CorridaAdelanto, "2026-08-15", "u1")
	if err != nil {
		t.Fatalf("crear adelanto: %v", err)
	}
	if adel.Lineas[0].Neto != "240000.00" {
		t.Fatalf("neto del adelanto = %s, quiere 240000.00", adel.Lineas[0].Neto)
	}
	// Duplicado del mismo mes/tipo: rechazado.
	if _, err := svc.CrearCorrida(ctx, "e1", 2026, 8, CorridaAdelanto, "2026-08-15", "u1"); !errors.Is(err, ErrCorridaDuplicada) {
		t.Fatalf("adelanto duplicado: err = %v, quiere ErrCorridaDuplicada", err)
	}
	// Novedades en un adelanto: no aplica.
	if _, err := svc.GuardarNovedades(ctx, "e1", adel.ID, nil, "u1"); !errors.Is(err, ErrNovedadSoloLiquidacion) {
		t.Fatalf("novedades en adelanto: err = %v, quiere ErrNovedadSoloLiquidacion", err)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", adel.ID, "u1"); err != nil {
		t.Fatalf("aprobar adelanto: %v", err)
	}

	// Liquidación: mes completo con comisiones (SON salario) y descuento del adelanto real.
	liq, err := svc.CrearCorrida(ctx, "e1", 2026, 8, CorridaLiquidacion, "2026-08-30", "u1")
	if err != nil {
		t.Fatalf("crear liquidación: %v", err)
	}
	liq, err = svc.GuardarNovedades(ctx, "e1", liq.ID, []NovedadInput{
		{EmpleadoID: "emp-1", ConceptoID: "com", Monto: "420000"},
	}, "u1")
	if err != nil {
		t.Fatalf("novedades: %v", err)
	}
	l := liq.Lineas[0]
	if l.Bruto != "900000.00" || l.CCSSObrero != "97470.00" || l.Adelanto != "240000.00" || l.Neto != "562530.00" {
		t.Fatalf("liquidación: bruto=%s ccss=%s adelanto=%s neto=%s (quiere 900000/97470/240000/562530)",
			l.Bruto, l.CCSSObrero, l.Adelanto, l.Neto)
	}

	// Pagar exige APROBADA; anular una PAGADA no procede.
	if _, err := svc.PagarCorrida(ctx, "e1", liq.ID, "u1"); !errors.Is(err, ErrCorridaNoPagable) {
		t.Fatalf("pagar borrador: err = %v, quiere ErrCorridaNoPagable", err)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", liq.ID, "u1"); err != nil {
		t.Fatalf("aprobar liquidación: %v", err)
	}
	if _, err := svc.RecalcularCorrida(ctx, "e1", liq.ID, "u1"); !errors.Is(err, ErrCorridaNoEditable) {
		t.Fatalf("recalcular aprobada: err = %v, quiere ErrCorridaNoEditable", err)
	}
	if _, err := svc.PagarCorrida(ctx, "e1", liq.ID, "u1"); err != nil {
		t.Fatalf("pagar liquidación: %v", err)
	}
	if _, err := svc.AnularCorrida(ctx, "e1", liq.ID, "u1"); !errors.Is(err, ErrCorridaNoAnulable) {
		t.Fatalf("anular pagada: err = %v, quiere ErrCorridaNoAnulable", err)
	}
}

// Guarda anti-doble-pago: la liquidación no se aprueba mientras el adelanto del MISMO mes
// siga en borrador (se pagaría el mes 1.5 veces); aprobar el adelanto desbloquea.
func TestAprobarLiquidacionConAdelantoBorrador(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "Ana", Identificacion: "1-111",
		SalarioBase: "400000.00", Activo: true}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	adel, err := svc.CrearCorrida(ctx, "e1", 2026, 9, CorridaAdelanto, "2026-09-15", "u1")
	if err != nil {
		t.Fatalf("crear adelanto: %v", err)
	}
	liq, err := svc.CrearCorrida(ctx, "e1", 2026, 9, CorridaLiquidacion, "2026-09-30", "u1")
	if err != nil {
		t.Fatalf("crear liquidación: %v", err)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", liq.ID, "u1"); !errors.Is(err, ErrAdelantoPendiente) {
		t.Fatalf("aprobar liquidación con adelanto en borrador: err = %v, quiere ErrAdelantoPendiente", err)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", adel.ID, "u1"); err != nil {
		t.Fatalf("aprobar adelanto: %v", err)
	}
	// Al aprobar, la liquidación se RECALCULA y absorbe el adelanto recién aprobado.
	aprobada, err := svc.AprobarCorrida(ctx, "e1", liq.ID, "u1")
	if err != nil {
		t.Fatalf("aprobar liquidación: %v", err)
	}
	// 400 000 − CCSS 43 320 − adelanto 200 000 = 156 680.
	if aprobada.TotalNeto != "156680.00" || aprobada.TotalAdel != "200000.00" {
		t.Fatalf("liquidación aprobada: neto=%s adelanto=%s (quiere 156680.00/200000.00)",
			aprobada.TotalNeto, aprobada.TotalAdel)
	}
}

// Guarda recíproca anti-doble-pago: con la liquidación del mes APROBADA/PAGADA no se puede
// crear (ni aprobar) un adelanto de ese mes — jamás se descontaría.
func TestAdelantoTrasLiquidacionCerrada(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "Ana", Identificacion: "1-111",
		SalarioBase: "400000.00", Activo: true}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	liq, err := svc.CrearCorrida(ctx, "e1", 2026, 10, CorridaLiquidacion, "2026-10-31", "u1")
	if err != nil {
		t.Fatalf("crear liquidación: %v", err)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", liq.ID, "u1"); err != nil {
		t.Fatalf("aprobar liquidación: %v", err)
	}
	if _, err := svc.CrearCorrida(ctx, "e1", 2026, 10, CorridaAdelanto, "2026-10-15", "u1"); !errors.Is(err, ErrLiquidacionCerrada) {
		t.Fatalf("crear adelanto tras liquidación cerrada: err = %v, quiere ErrLiquidacionCerrada", err)
	}
}

// Un ADELANTO aprobado que la liquidación cerrada ya descontó NO se anula (el empleado ya
// fue descontado: anularlo lo dejaría sin ese monto).
func TestAnularAdelantoDescontado(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "Ana", Identificacion: "1-111",
		SalarioBase: "400000.00", Activo: true}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	adel, err := svc.CrearCorrida(ctx, "e1", 2026, 11, CorridaAdelanto, "2026-11-15", "u1")
	if err != nil {
		t.Fatalf("crear adelanto: %v", err)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", adel.ID, "u1"); err != nil {
		t.Fatalf("aprobar adelanto: %v", err)
	}
	liq, err := svc.CrearCorrida(ctx, "e1", 2026, 11, CorridaLiquidacion, "2026-11-30", "u1")
	if err != nil {
		t.Fatalf("crear liquidación: %v", err)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", liq.ID, "u1"); err != nil {
		t.Fatalf("aprobar liquidación: %v", err)
	}
	if _, err := svc.AnularCorrida(ctx, "e1", adel.ID, "u1"); !errors.Is(err, ErrAdelantoDescontado) {
		t.Fatalf("anular adelanto descontado: err = %v, quiere ErrAdelantoDescontado", err)
	}
	// Antes de que la liquidación se apruebe sí es anulable (el recálculo lo corrige).
	adel2, err := svc.CrearCorrida(ctx, "e1", 2026, 12, CorridaAdelanto, "2026-12-15", "u1")
	if err != nil {
		t.Fatalf("crear adelanto diciembre: %v", err)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", adel2.ID, "u1"); err != nil {
		t.Fatalf("aprobar adelanto diciembre: %v", err)
	}
	if _, err := svc.AnularCorrida(ctx, "e1", adel2.ID, "u1"); err != nil {
		t.Fatalf("anular adelanto sin liquidación cerrada: err = %v, quiere nil", err)
	}
}

// Nunca se congela un depósito negativo: con adelanto_pct 90% el neto de la liquidación
// queda bajo cero y la aprobación se bloquea (se corrige en borrador).
func TestNetoNegativoNoAprueba(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "Ana", Identificacion: "1-111",
		SalarioBase: "400000.00", Activo: true}
	p := ParametrosDefault2026(2026)
	p.AdelantoPct = "90"
	p.Origen = "EMPRESA"
	repo.parametros[2026] = p
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	adel, err := svc.CrearCorrida(ctx, "e1", 2026, 7, CorridaAdelanto, "2026-07-15", "u1")
	if err != nil {
		t.Fatalf("crear adelanto: %v", err)
	}
	if adel.Lineas[0].Neto != "360000.00" {
		t.Fatalf("adelanto 90%% = %s, quiere 360000.00", adel.Lineas[0].Neto)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", adel.ID, "u1"); err != nil {
		t.Fatalf("aprobar adelanto: %v", err)
	}
	liq, err := svc.CrearCorrida(ctx, "e1", 2026, 7, CorridaLiquidacion, "2026-07-31", "u1")
	if err != nil {
		t.Fatalf("crear liquidación: %v", err)
	}
	// 400 000 − 43 320 (CCSS) − 360 000 (adelanto) = −3 320: se ve en el borrador…
	if liq.Lineas[0].Neto != "-3320.00" {
		t.Fatalf("neto del borrador = %s, quiere -3320.00", liq.Lineas[0].Neto)
	}
	// …pero jamás se congela.
	if _, err := svc.AprobarCorrida(ctx, "e1", liq.ID, "u1"); !errors.Is(err, ErrNetoNegativo) {
		t.Fatalf("aprobar con neto negativo: err = %v, quiere ErrNetoNegativo", err)
	}
}

// Las dos quincenas de un empleado QUINCENAL, extremo a extremo por el servicio: la 1ª
// retiene su CCSS, sus deducciones y la mitad del impuesto; la 2ª cobra el ajuste real y
// NO le descuenta el pago del día 15 (fueron dos salarios, no un adelanto).
func TestCicloQuincenal(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["ana"] = Empleado{ID: "ana", Nombre: "Ana Lucía Castro", Identificacion: "1-1503",
		SalarioBase: "1000000.00", Hijos: 1, Jornada: JornadaQuincenal, Activo: true}
	repo.conceptos["com"] = ConceptoNomina{ID: "com", Nombre: "Comisiones", Tipo: ConceptoIngreso,
		AfectaCCSS: true, AfectaRenta: true, AfectaAguinaldo: true, DeSistema: true, Activo: true}
	// Préstamo cada quincena y ahorro solo en la segunda.
	repo.dedsCalc = map[string][]DeduccionCalc{"ana": {
		{ID: "d1", Etiqueta: "Préstamo Asociación", Cuota: dec("40000"), SaldoRestante: decPtr("315000"), Prioridad: 100, Frecuencia: FrecAmbas},
		{ID: "d2", Etiqueta: "Ahorro navideño", Cuota: dec("20000"), Prioridad: 100, Frecuencia: FrecSegunda},
	}}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	q1, err := svc.CrearCorrida(ctx, "e1", 2026, 7, CorridaAdelanto, "2026-07-15", "u1")
	if err != nil {
		t.Fatalf("crear 1ª quincena: %v", err)
	}
	l1 := q1.Lineas[0]
	if l1.Tratamiento != TratQuincena1 {
		t.Fatalf("tratamiento = %s, quiere QUINCENA_1", l1.Tratamiento)
	}
	if l1.Bruto != "500000.00" || l1.CCSSObrero != "54150.00" || l1.Renta != "3245.00" ||
		l1.Deducciones != "40000.00" || l1.Neto != "402605.00" {
		t.Fatalf("1ª quincena: bruto=%s ccss=%s renta=%s deducc=%s neto=%s (quiere 500000/54150/3245/40000/402605)",
			l1.Bruto, l1.CCSSObrero, l1.Renta, l1.Deducciones, l1.Neto)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", q1.ID, "u1"); err != nil {
		t.Fatalf("aprobar 1ª quincena: %v", err)
	}

	q2, err := svc.CrearCorrida(ctx, "e1", 2026, 7, CorridaLiquidacion, "2026-07-31", "u1")
	if err != nil {
		t.Fatalf("crear 2ª quincena: %v", err)
	}
	q2, err = svc.GuardarNovedades(ctx, "e1", q2.ID, []NovedadInput{
		{EmpleadoID: "ana", ConceptoID: "com", Monto: "200000"},
	}, "u1")
	if err != nil {
		t.Fatalf("novedades: %v", err)
	}
	l2 := q2.Lineas[0]
	if l2.Tratamiento != TratQuincena2 {
		t.Fatalf("tratamiento = %s, quiere QUINCENA_2", l2.Tratamiento)
	}
	if l2.Bruto != "700000.00" || l2.CCSSObrero != "75810.00" || l2.Renta != "23245.00" ||
		l2.Deducciones != "60000.00" || l2.Neto != "540945.00" {
		t.Fatalf("2ª quincena: bruto=%s ccss=%s renta=%s deducc=%s neto=%s (quiere 700000/75810/23245/60000/540945)",
			l2.Bruto, l2.CCSSObrero, l2.Renta, l2.Deducciones, l2.Neto)
	}
	// Lo crítico: su pago del día 15 fue salario, así que NO se le descuenta.
	if l2.Adelanto != "0.00" {
		t.Fatalf("a un quincenal no se le descuenta la 1ª quincena: adelanto = %s, quiere 0.00", l2.Adelanto)
	}
	// El mes cuadra: CCSS 129 960 (10,83% de 1 200 000) y renta 26 490.
	ccssMes := dec(l1.CCSSObrero).Add(dec(l2.CCSSObrero))
	rentaMes := dec(l1.Renta).Add(dec(l2.Renta))
	if !ccssMes.Equal(dec("129960")) || !rentaMes.Equal(dec("26490")) {
		t.Fatalf("cuadre del mes: ccss=%s renta=%s (quiere 129960/26490)", ccssMes, rentaMes)
	}
}

// Un empleado de jornada MENSUAL sigue exactamente como antes: adelanto sin deducciones
// el día 15 y liquidación que lo descuenta el 30.
func TestJornadaMensualNoCambia(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["carlos"] = Empleado{ID: "carlos", Nombre: "Carlos Fernández", Identificacion: "3-0389",
		SalarioBase: "800000.00", Jornada: "MENSUAL", Activo: true}
	repo.dedsCalc = map[string][]DeduccionCalc{"carlos": {
		{ID: "d1", Etiqueta: "Ahorro", Cuota: dec("30000"), Prioridad: 100, Frecuencia: FrecMensual},
	}}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	adel, err := svc.CrearCorrida(ctx, "e1", 2026, 7, CorridaAdelanto, "2026-07-15", "u1")
	if err != nil {
		t.Fatalf("crear adelanto: %v", err)
	}
	if adel.Lineas[0].Tratamiento != TratAdelanto || adel.Lineas[0].Neto != "400000.00" ||
		adel.Lineas[0].Deducciones != "0.00" {
		t.Fatalf("adelanto: trat=%s neto=%s deducc=%s (quiere ADELANTO/400000/0)",
			adel.Lineas[0].Tratamiento, adel.Lineas[0].Neto, adel.Lineas[0].Deducciones)
	}
	if _, err := svc.AprobarCorrida(ctx, "e1", adel.ID, "u1"); err != nil {
		t.Fatalf("aprobar adelanto: %v", err)
	}
	liq, err := svc.CrearCorrida(ctx, "e1", 2026, 7, CorridaLiquidacion, "2026-07-31", "u1")
	if err != nil {
		t.Fatalf("crear liquidación: %v", err)
	}
	l := liq.Lineas[0]
	// 800 000 − CCSS 86 640 − ahorro 30 000 − adelanto 400 000 = 283 360.
	if l.Tratamiento != TratMensual || l.Adelanto != "400000.00" || l.Neto != "283360.00" {
		t.Fatalf("liquidación mensual: trat=%s adelanto=%s neto=%s (quiere MENSUAL/400000/283360)",
			l.Tratamiento, l.Adelanto, l.Neto)
	}
}

// La corrida descuenta los días de incapacidad que la empresa no paga y lo explica en la
// colilla; la base CCSS baja porque ese salario no se devengó (no es subdeclaración).
func TestCorridaConIncapacidad(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "Luis Vargas", Identificacion: "2-0641",
		SalarioBase: "900000.00", Jornada: "MENSUAL", Activo: true}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	// CCSS de 5 días: 3 al 50% de la empresa (1,5 días) y 2 con subsidio → descuenta 3,5
	// días de salario = ₡105 000 sobre un diario de ₡30 000.
	if _, err := svc.RegistrarIncapacidad(ctx, "e1", IncapacidadInput{
		EmpleadoID: "emp-1", Entidad: EntidadCCSS, FechaInicio: "2026-07-08", Dias: 5}, "u1"); err != nil {
		t.Fatalf("registrar incapacidad: %v", err)
	}
	liq, err := svc.CrearCorrida(ctx, "e1", 2026, 7, CorridaLiquidacion, "2026-07-31", "u1")
	if err != nil {
		t.Fatalf("crear liquidación: %v", err)
	}
	l := liq.Lineas[0]
	if l.Bruto != "795000.00" || l.BaseCCSS != "795000.00" {
		t.Fatalf("con incapacidad: bruto=%s baseCCSS=%s (quiere 795000.00 ambos)", l.Bruto, l.BaseCCSS)
	}
	var explica string
	for _, d := range l.Detalle {
		if d.Tipo == "INCAPACIDAD" {
			explica = d.Nombre
		}
	}
	if explica == "" {
		t.Fatalf("la colilla debe explicar la incapacidad; detalle: %+v", l.Detalle)
	}
	t.Logf("colilla: %s", explica)

	// Con la corrida ya aprobada, no se pueden tocar las ausencias del período.
	if _, err := svc.AprobarCorrida(ctx, "e1", liq.ID, "u1"); err != nil {
		t.Fatalf("aprobar: %v", err)
	}
	if _, err := svc.RegistrarIncapacidad(ctx, "e1", IncapacidadInput{
		EmpleadoID: "emp-1", Entidad: EntidadCCSS, FechaInicio: "2026-07-20", Dias: 2,
	}, "u1"); !errors.Is(err, ErrAusenciaCorridaCerrada) {
		t.Fatalf("registrar en mes cerrado: err = %v, quiere ErrAusenciaCorridaCerrada", err)
	}
}

// Saldo de vacaciones: se acumula 1 día por mes, el disfrute lo descuenta y no se puede
// registrar más de lo acumulado. El finiquito toma el pendiente si no se le indica otro.
func TestSaldoVacacionesYFiniquito(t *testing.T) {
	repo := newFakeRepo()
	repo.mesesServicio = 10 // 10 días acumulados con 1 día por mes
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "Ana", Identificacion: "1-111",
		SalarioBase: "600000.00", FechaIngreso: "2025-09-01", Activo: true}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	saldos, err := svc.SaldosVacaciones(ctx, "e1", 2026)
	if err != nil || len(saldos) != 1 {
		t.Fatalf("saldos: %v (n=%d)", err, len(saldos))
	}
	if saldos[0].Acumulado != "10.00" || saldos[0].Pendiente != "10.00" {
		t.Fatalf("saldo inicial: acumulado=%s pendiente=%s (quiere 10.00/10.00)",
			saldos[0].Acumulado, saldos[0].Pendiente)
	}
	// Disfruta 4 días → quedan 6.
	if _, err := svc.RegistrarVacacion(ctx, "e1", VacacionInput{
		EmpleadoID: "emp-1", FechaInicio: "2026-07-06", Dias: "4"}, "u1"); err != nil {
		t.Fatalf("registrar vacación: %v", err)
	}
	saldos, _ = svc.SaldosVacaciones(ctx, "e1", 2026)
	if saldos[0].Pendiente != "6.00" {
		t.Fatalf("pendiente tras disfrutar 4: %s (quiere 6.00)", saldos[0].Pendiente)
	}
	// Más días que el saldo: rechazado.
	if _, err := svc.RegistrarVacacion(ctx, "e1", VacacionInput{
		EmpleadoID: "emp-1", FechaInicio: "2026-08-03", Dias: "9",
	}, "u1"); !errors.Is(err, ErrSinSaldoVacaciones) {
		t.Fatalf("sin saldo: err = %v, quiere ErrSinSaldoVacaciones", err)
	}
	// El finiquito sin días indicados toma el pendiente (6 × ₡20 000 diarios = ₡120 000).
	fi, err := svc.CrearFiniquito(ctx, "e1", FiniquitoInput{
		EmpleadoID: "emp-1", Motivo: MotivoRenuncia, FechaSalida: "2026-07-31"}, "u1")
	if err != nil {
		t.Fatalf("crear finiquito: %v", err)
	}
	if fi.DiasVacaciones != "6.00" || fi.Vacaciones != "120000.00" {
		t.Fatalf("finiquito precargado: días=%s vacaciones=%s (quiere 6.00/120000.00)",
			fi.DiasVacaciones, fi.Vacaciones)
	}
}

// Ciclo del finiquito: crear (calculado) → duplicado bloqueado → aprobar → pagar (da de
// baja la ficha) → anular pagado bloqueado.
func TestCicloFiniquito(t *testing.T) {
	repo := newFakeRepo()
	repo.empleados["emp-1"] = Empleado{ID: "emp-1", Nombre: "Carlos", Identificacion: "3-0389",
		SalarioBase: "835000.00", FechaIngreso: "2016-06-15", Activo: true}
	svc := NewService(repo, nil, nil)
	ctx := context.Background()

	fi, err := svc.CrearFiniquito(ctx, "e1", FiniquitoInput{
		EmpleadoID: "emp-1", Motivo: MotivoDespido, FechaSalida: "2026-07-31", DiasVacaciones: "8.5",
	}, "u1")
	if err != nil {
		t.Fatalf("crear finiquito: %v", err)
	}
	if fi.Total != "6389902.00" || fi.AniosServicio != 10 {
		t.Fatalf("finiquito Carlos: total=%s años=%d (quiere 6389902.00/10)", fi.Total, fi.AniosServicio)
	}
	if _, err := svc.CrearFiniquito(ctx, "e1", FiniquitoInput{
		EmpleadoID: "emp-1", Motivo: MotivoRenuncia, FechaSalida: "2026-08-31",
	}, "u1"); !errors.Is(err, ErrFiniquitoDuplicado) {
		t.Fatalf("finiquito duplicado: err = %v, quiere ErrFiniquitoDuplicado", err)
	}
	// Motivo inválido y fecha anterior al ingreso: rechazados.
	if _, err := svc.ActualizarFiniquito(ctx, "e1", fi.ID, FiniquitoInput{
		Motivo: "OTRO", FechaSalida: "2026-07-31"}, "u1"); !errors.Is(err, ErrMotivoInvalido) {
		t.Fatalf("motivo inválido: err = %v, quiere ErrMotivoInvalido", err)
	}
	if _, err := svc.ActualizarFiniquito(ctx, "e1", fi.ID, FiniquitoInput{
		Motivo: MotivoRenuncia, FechaSalida: "2015-01-01"}, "u1"); !errors.Is(err, ErrFechaSalidaInvalida) {
		t.Fatalf("salida antes del ingreso: err = %v, quiere ErrFechaSalidaInvalida", err)
	}
	// Cambiar a renuncia recalcula (sin preaviso ni cesantía).
	fi, err = svc.ActualizarFiniquito(ctx, "e1", fi.ID, FiniquitoInput{
		Motivo: MotivoRenuncia, FechaSalida: "2026-07-31", DiasVacaciones: "8.5"}, "u1")
	if err != nil || fi.Total != "767626.00" {
		t.Fatalf("finiquito renuncia: total=%s err=%v (quiere 767626.00)", fi.Total, err)
	}
	// Días de vacaciones absurdos: 422 en vez de desbordar numeric(6,2).
	if _, err := svc.ActualizarFiniquito(ctx, "e1", fi.ID, FiniquitoInput{
		Motivo: MotivoRenuncia, FechaSalida: "2026-07-31", DiasVacaciones: "99999",
	}, "u1"); !errors.Is(err, ErrDiasVacacionesInvalidos) {
		t.Fatalf("días absurdos: err = %v, quiere ErrDiasVacacionesInvalidos", err)
	}
	if _, err := svc.PagarFiniquito(ctx, "e1", fi.ID, "u1"); !errors.Is(err, ErrFiniquitoNoPagable) {
		t.Fatalf("pagar borrador: err = %v, quiere ErrFiniquitoNoPagable", err)
	}
	if _, err := svc.AprobarFiniquito(ctx, "e1", fi.ID, "u1"); err != nil {
		t.Fatalf("aprobar finiquito: %v", err)
	}
	if _, err := svc.ActualizarFiniquito(ctx, "e1", fi.ID, FiniquitoInput{
		Motivo: MotivoRenuncia, FechaSalida: "2026-07-31"}, "u1"); !errors.Is(err, ErrFiniquitoNoEditable) {
		t.Fatalf("editar aprobado: err = %v, quiere ErrFiniquitoNoEditable", err)
	}
	if _, err := svc.PagarFiniquito(ctx, "e1", fi.ID, "u1"); err != nil {
		t.Fatalf("pagar finiquito: %v", err)
	}
	if repo.empleados["emp-1"].Activo {
		t.Fatalf("el empleado debió quedar de baja al pagar el finiquito")
	}
	if _, err := svc.AnularFiniquito(ctx, "e1", fi.ID, "u1"); !errors.Is(err, ErrFiniquitoNoAnulable) {
		t.Fatalf("anular pagado: err = %v, quiere ErrFiniquitoNoAnulable", err)
	}
}
