package cxp

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type fakeRepo struct {
	creado      *ProveedorInput
	doc         Documento            // lo que devuelve DocumentoPorID por defecto (para probar guards)
	docs        map[string]Documento // por id (anticipos: factura y anticipo distintos)
	esValidador bool                 // lo que devuelve EsValidador
	// Scoping por área: qué devuelve DepartamentosDeUsuario y qué filtro se capturó.
	deptsDeUsuario  []string
	capListDepts    []string
	capListSet      bool
	capResumenDepts []string
	capResumenSet   bool
	// Huella Bancos↔CxP.
	docPorHuella Documento
	errPorHuella error
	netoAPagar   string
	capA         string // último estado destino pasado a CambiarEstado
	filasCambio  int64  // lo que devuelve CambiarEstado (0 = conflicto, por defecto)
	// Anticipos.
	saldoAnticipo  decimal.Decimal
	aplicarLlamado bool
	loteAplicado   int
	// Aprobación: qué roles ya aprobaron y a qué estado transicionó CambiarEstadoMulti.
	rolesAprobaciones []string
	capMultiA         string
	// Marca «de Contabilidad»: lo capturado y el modo «el UPDATE no tocó nada».
	contaMarcaDoc    *bool
	contaMotivoDoc   string
	contaFilasCero   bool
	contaSellado     bool
	contaSelloMotivo string
	// Validación por riesgo.
	motivoValidacion string
	parametros       []ParametroCxP
	paramGuardado    string
	paramFilasCero   bool
	efecto           EfectoValidacion
	// Carga de IBAN.
	provsPorCedula map[string]ProveedorIBAN
	ibanGuardado   map[string]string
	// Caja chica.
	fondo          FondoCajaChica
	valesElegibles []string
	valesTotal     decimal.Decimal
	valeCreado     bool
}

func (f *fakeRepo) Crear(_ context.Context, _ string, p ProveedorInput) (Proveedor, error) {
	f.creado = &p
	return Proveedor{ID: "prov-1", Nombre: p.Nombre, RetencionRentaPct: p.RetencionRentaPct.String(), Activo: true}, nil
}
func (f *fakeRepo) Listar(context.Context, string, FiltrosProveedor, int, int) (ListaProveedores, error) {
	return ListaProveedores{Items: []Proveedor{}, Page: 1, PageSize: 100}, nil
}
func (f *fakeRepo) PorID(context.Context, string, string) (Proveedor, error) {
	return Proveedor{ID: "prov-1"}, nil
}
func (f *fakeRepo) Actualizar(context.Context, string, string, ProveedorInput) (Proveedor, error) {
	return Proveedor{ID: "prov-1"}, nil
}
func (f *fakeRepo) Desactivar(context.Context, string, string) error { return nil }

func (f *fakeRepo) CrearDocumento(context.Context, string, DocumentoInput, decimal.Decimal, *decimal.Decimal, string) (Documento, error) {
	return Documento{}, nil
}
func (f *fakeRepo) ListarDocumentos(_ context.Context, _ string, filtros FiltrosDocumentos) (ListaDocumentos, error) {
	f.capListDepts = filtros.DepartamentoIDs
	f.capListSet = true
	return ListaDocumentos{}, nil
}
func (f *fakeRepo) DocumentoPorID(_ context.Context, _ string, id string) (Documento, error) {
	if d, ok := f.docs[id]; ok {
		return d, nil
	}
	return f.doc, nil
}
func (f *fakeRepo) AnticiposDisponibles(context.Context, string, string) ([]AnticipoSaldo, error) {
	return nil, nil
}
func (f *fakeRepo) ListarFondos(context.Context, string, string) ([]FondoCajaChica, error) {
	return nil, nil
}
func (f *fakeRepo) FondoPorID(context.Context, string, string) (FondoCajaChica, error) {
	return f.fondo, nil
}
func (f *fakeRepo) CrearFondo(_ context.Context, _ string, in FondoInput) (FondoCajaChica, error) {
	return FondoCajaChica{ID: "fondo-1", Nombre: in.Nombre, Activo: true}, nil
}
func (f *fakeRepo) ActualizarFondo(context.Context, string, string, FondoInput) (FondoCajaChica, error) {
	return f.fondo, nil
}
func (f *fakeRepo) DesactivarFondo(context.Context, string, string) error { return nil }
func (f *fakeRepo) ListarVales(context.Context, string, string) ([]ValeCajaChica, error) {
	return nil, nil
}
func (f *fakeRepo) CrearVale(context.Context, string, string, ValeInput, string) (string, error) {
	f.valeCreado = true
	return "vale-1", nil
}
func (f *fakeRepo) AnularVale(context.Context, string, string, string) error { return nil }
func (f *fakeRepo) ValesElegiblesReposicion(context.Context, string, string) ([]string, decimal.Decimal, error) {
	return f.valesElegibles, f.valesTotal, nil
}
func (f *fakeRepo) VincularValesAReposicion(context.Context, string, string, string, []string) (int64, error) {
	return int64(len(f.valesElegibles)), nil
}
func (f *fakeRepo) AnticiposEmpresa(context.Context, string) ([]AnticipoSaldo, error) {
	return nil, nil
}
func (f *fakeRepo) SaldoAnticipo(context.Context, string, string) (decimal.Decimal, error) {
	return f.saldoAnticipo, nil
}
func (f *fakeRepo) AplicarAnticipo(context.Context, string, string, string, decimal.Decimal, string) (string, error) {
	f.aplicarLlamado = true
	return "aa-1", nil
}
func (f *fakeRepo) AplicarAnticiposLote(_ context.Context, _ string, _ string, apps []AplicacionInput, _ string) error {
	f.aplicarLlamado = true
	f.loteAplicado = len(apps)
	return nil
}
func (f *fakeRepo) ReversarAplicacion(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeRepo) AplicacionesDeFactura(context.Context, string, string) ([]AplicacionAnticipo, error) {
	return nil, nil
}

// CambiarEstado devuelve filasCambio (0 por defecto, para los tests de conflicto) y anota el
// estado destino, que es lo que verifica la conciliación por huella.
func (f *fakeRepo) CambiarEstado(_ context.Context, _, _, _, a string) (int64, error) {
	f.capA = a
	return f.filasCambio, nil
}
func (f *fakeRepo) Programar(context.Context, string, string, string, string) (int64, error) {
	return 0, nil
}
func (f *fakeRepo) Clasificar(context.Context, string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (f *fakeRepo) ListarSubclasificaciones(context.Context, string, string) ([]Subclasificacion, error) {
	return nil, nil
}
func (f *fakeRepo) CrearSubclasificacion(context.Context, string, string, string) (Subclasificacion, error) {
	return Subclasificacion{}, nil
}
func (f *fakeRepo) ListarDepartamentos(context.Context, string, bool) ([]Departamento, error) {
	return nil, nil
}
func (f *fakeRepo) CrearDepartamento(context.Context, string, DepartamentoInput) (Departamento, error) {
	return Departamento{}, nil
}
func (f *fakeRepo) ActualizarDepartamento(context.Context, string, string, DepartamentoInput) (Departamento, error) {
	return Departamento{}, nil
}
func (f *fakeRepo) DesactivarDepartamento(context.Context, string, string) error { return nil }
func (f *fakeRepo) EnsureDepartamentos(context.Context) error                    { return nil }
func (f *fakeRepo) AsignarDepartamentoDoc(context.Context, string, string, string) (int64, error) {
	return 1, nil
}
func (f *fakeRepo) EsValidador(context.Context, string, string, string) (bool, error) {
	return f.esValidador, nil
}
func (f *fakeRepo) DepartamentosDeUsuario(context.Context, string, string) ([]string, error) {
	return f.deptsDeUsuario, nil
}
func (f *fakeRepo) ValidarDeptoDoc(context.Context, string, string, string, string, string) (int64, error) {
	return 1, nil
}
func (f *fakeRepo) DevolverDoc(context.Context, string, string, string) (int64, error) { return 1, nil }

// Marca «de Contabilidad». `contaMarcaDoc` guarda la última marca aplicada a un documento para
// poder afirmar CON QUÉ valor se llamó (que es la regla de los tres estados).
func (f *fakeRepo) MarcarDocumentoContabilidad(_ context.Context, _, _ string, valor *bool, motivo, _ string) (int64, error) {
	f.contaMarcaDoc = valor
	f.contaMotivoDoc = motivo
	if f.contaFilasCero {
		return 0, nil
	}
	return 1, nil
}
func (f *fakeRepo) MarcarProveedorContabilidad(context.Context, string, string, bool) (int64, error) {
	if f.contaFilasCero {
		return 0, nil
	}
	return 1, nil
}
func (f *fakeRepo) MarcarConceptoContabilidad(context.Context, string, string, bool) (int64, error) {
	if f.contaFilasCero {
		return 0, nil
	}
	return 1, nil
}
func (f *fakeRepo) MarcarClasificacionContabilidad(context.Context, string, string, bool) (int64, error) {
	if f.contaFilasCero {
		return 0, nil
	}
	return 1, nil
}
func (f *fakeRepo) MarcasContabilidad(context.Context, string) (MarcasContabilidad, error) {
	return MarcasContabilidad{}, nil
}

// EvaluarValidacion devuelve el motivo que se le ponga (por defecto "": no requiere validación).
func (f *fakeRepo) EvaluarValidacion(context.Context, string, string) (string, error) {
	return f.motivoValidacion, nil
}
func (f *fakeRepo) ParametrosValidacion(context.Context, string) ([]ParametroCxP, error) {
	return f.parametros, nil
}

// Carga de IBAN: el fake guarda lo que se le pidió actualizar, para poder afirmar QUÉ se guardó.
func (f *fakeRepo) ProveedoresPorIdentificacion(context.Context, string) (map[string]ProveedorIBAN, error) {
	return f.provsPorCedula, nil
}
func (f *fakeRepo) ActualizarIBANProveedor(_ context.Context, _, proveedorID, iban string) error {
	if f.ibanGuardado == nil {
		f.ibanGuardado = map[string]string{}
	}
	f.ibanGuardado[proveedorID] = iban
	return nil
}

func (f *fakeRepo) EfectoValidacion(context.Context, string) (EfectoValidacion, error) {
	return f.efecto, nil
}
func (f *fakeRepo) GuardarParametroValidacion(_ context.Context, _, clave, valor, _ string) (int64, error) {
	f.paramGuardado = clave + "=" + valor
	if f.paramFilasCero {
		return 0, nil
	}
	return 1, nil
}

// contaSellado registra si se congeló la marca heredada al aprobar (y con qué motivo).
func (f *fakeRepo) SellarContabilidad(_ context.Context, _, _, motivo string) error {
	f.contaSellado = true
	f.contaSelloMotivo = motivo
	return nil
}
func (f *fakeRepo) ListarValidadores(context.Context, string, string) ([]Validador, error) {
	return nil, nil
}
func (f *fakeRepo) AsignarValidador(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeRepo) QuitarValidador(context.Context, string, string, string) error { return nil }
func (f *fakeRepo) UsuariosEmpresa(context.Context, string) ([]UsuarioRef, error) {
	return nil, nil
}
func (f *fakeRepo) CambiarEstadoMulti(_ context.Context, _ string, _ string, _ []string, a string) (int64, error) {
	f.capMultiA = a
	return 1, nil
}
func (f *fakeRepo) AsignarTipo(context.Context, string, string, string) (int64, error) {
	return 0, nil
}
func (f *fakeRepo) GuardarGastoDefault(context.Context, string, string, string, string, string) error {
	return nil
}
func (f *fakeRepo) ResumenBandeja(_ context.Context, _ string, deptIDs []string) ([]FaseBandeja, error) {
	f.capResumenDepts = deptIDs
	f.capResumenSet = true
	return nil, nil
}
func (f *fakeRepo) ProgramarAprobados(context.Context, string, []string, string) (int64, error) {
	return 0, nil
}
func (f *fakeRepo) AsignarPrioridad(context.Context, string, string, string) (int64, error) {
	return 0, nil
}
func (f *fakeRepo) GuardarNotaRevision(context.Context, string, string, string) error { return nil }
func (f *fakeRepo) RegistrarGastoProveedor(context.Context, string, string, string, string, string) error {
	return nil
}
func (f *fakeRepo) AprenderCondicionPago(context.Context, string, string, string, int) error {
	return nil
}
func (f *fakeRepo) GastosDeProveedor(context.Context, string, string) ([]GastoFrecuente, error) {
	return nil, nil
}
func (f *fakeRepo) RegistrarAprobacion(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeRepo) RolesAprobaciones(context.Context, string, string) ([]string, error) {
	return f.rolesAprobaciones, nil
}
func (f *fakeRepo) DocumentosParaPago(context.Context, string, string) ([]PagoRow, error) {
	return nil, nil
}
func (f *fakeRepo) DocumentosParaPagoPorIDs(context.Context, string, []string) ([]PagoRow, error) {
	return nil, nil
}
func (f *fakeRepo) DocumentoPorHuella(context.Context, string, string) (Documento, error) {
	return f.docPorHuella, f.errPorHuella
}
func (f *fakeRepo) NetoAPagar(context.Context, string, string) (string, error) {
	return f.netoAPagar, nil
}
func (f *fakeRepo) ClavesExistentes(context.Context, string, []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}
func (f *fakeRepo) ProveedorIDPorIdentificacion(context.Context, string, string) (string, bool, error) {
	return "", false, nil
}
func (f *fakeRepo) HistorialDocumento(context.Context, string, string) ([]EventoHistorial, error) {
	return nil, nil
}
func (f *fakeRepo) DashboardCxP(_ context.Context, _, periodo string, deptIDs []string) (DashboardCxP, error) {
	// Devuelve lo recibido para poder afirmar que el servicio propaga período y alcance.
	return DashboardCxP{Periodo: periodo, AlcanceLimitado: deptIDs != nil}, nil
}
func (f *fakeRepo) CrearLote(context.Context, string, string, []string, string) (LotePago, error) {
	return LotePago{}, nil
}
func (f *fakeRepo) ListarLotes(context.Context, string) ([]LotePago, error) {
	return nil, nil
}
func (f *fakeRepo) DocumentosParaPagoPorLote(context.Context, string, string) ([]PagoRow, error) {
	return nil, nil
}
func (f *fakeRepo) Reintentar(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (f *fakeRepo) GuardarComprobante(context.Context, string, string, string, string, []byte, string) error {
	return nil
}
func (f *fakeRepo) ObtenerComprobante(context.Context, string, string) (Comprobante, error) {
	return Comprobante{}, nil
}
func (f *fakeRepo) ObtenerComprobanteEnvio(context.Context, string, string) (ComprobanteEnvio, error) {
	return ComprobanteEnvio{}, nil
}
func (f *fakeRepo) MarcarComprobanteEnviado(context.Context, string, string) error {
	return nil
}

func TestCrearProveedor(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, nil, zap.NewNop()) // audit nil => auditar() no-op

	p, err := svc.Crear(context.Background(), "emp", ProveedorInput{Nombre: "ACME S.A.", RetencionRentaPct: dec("2")}, "u1")
	if err != nil {
		t.Fatalf("Crear: %v", err)
	}
	if p.ID != "prov-1" || p.Nombre != "ACME S.A." {
		t.Fatalf("proveedor = %+v", p)
	}
	if repo.creado == nil || repo.creado.Nombre != "ACME S.A." || !repo.creado.RetencionRentaPct.Equal(dec("2")) {
		t.Errorf("el input no llegó al repo: %+v", repo.creado)
	}
}

func TestValidarDeptoGuards(t *testing.T) {
	ctx := context.Background()
	// No está REVISADO → transición inválida.
	svc := NewService(&fakeRepo{doc: Documento{ID: "d1", Estado: EstRecibido}}, nil, zap.NewNop())
	if _, err := svc.ValidarDepto(ctx, "emp", "d1", "u1", "remision.pdf", ""); err != ErrTransicionInvalida {
		t.Errorf("estado != REVISADO => %v, quiere ErrTransicionInvalida", err)
	}
	// REVISADO pero sin departamento → ErrDeptoRequerido.
	svc = NewService(&fakeRepo{doc: Documento{ID: "d1", Estado: EstRevisado}}, nil, zap.NewNop())
	if _, err := svc.ValidarDepto(ctx, "emp", "d1", "u1", "remision.pdf", ""); err != ErrDeptoRequerido {
		t.Errorf("sin depto => %v, quiere ErrDeptoRequerido", err)
	}
	// Con depto pero sin respaldo → ErrRespaldoRequerido.
	svc = NewService(&fakeRepo{doc: Documento{ID: "d1", Estado: EstRevisado, DepartamentoID: "dep1"}}, nil, zap.NewNop())
	if _, err := svc.ValidarDepto(ctx, "emp", "d1", "u1", "  ", ""); err != ErrRespaldoRequerido {
		t.Errorf("sin respaldo => %v, quiere ErrRespaldoRequerido", err)
	}
	// Con depto y respaldo pero el usuario NO es validador → ErrNoEsValidador.
	svc = NewService(&fakeRepo{doc: Documento{ID: "d1", Estado: EstRevisado, DepartamentoID: "dep1"}, esValidador: false}, nil, zap.NewNop())
	if _, err := svc.ValidarDepto(ctx, "emp", "d1", "u1", "remision.pdf", ""); err != ErrNoEsValidador {
		t.Errorf("no validador => %v, quiere ErrNoEsValidador", err)
	}
	// Todo en orden → sin error (fake ValidarDeptoDoc devuelve 1).
	svc = NewService(&fakeRepo{doc: Documento{ID: "d1", Estado: EstRevisado, DepartamentoID: "dep1"}, esValidador: true}, nil, zap.NewNop())
	if _, err := svc.ValidarDepto(ctx, "emp", "d1", "u1", "remision.pdf", "ok"); err != nil {
		t.Errorf("validación válida => error inesperado %v", err)
	}
}

// fakePerms simula el checker RBAC: solo responde al permiso cxp.ver_todo.
type fakePerms struct{ verTodo bool }

func (p fakePerms) Tiene(_ context.Context, _, _, permiso string) (bool, error) {
	if permiso == permisoVerTodo {
		return p.verTodo, nil
	}
	return false, nil
}

// El validador de área (sin cxp.ver_todo) solo ve las facturas de su(s) departamento(s);
// quien tiene cxp.ver_todo (o sin checker) ve todo (filtro nil).
func TestScopingPorAreaEnBandejaYListado(t *testing.T) {
	ctx := context.Background()

	// (1) Con cxp.ver_todo => sin filtro (nil) tanto en bandeja como en listado.
	repo := &fakeRepo{deptsDeUsuario: []string{"depA"}}
	svc := NewService(repo, nil, zap.NewNop())
	svc.SetPermisos(fakePerms{verTodo: true})
	if _, err := svc.Bandeja(ctx, "emp", "SUPERVISOR_FINANCIERO", "u1"); err != nil {
		t.Fatalf("Bandeja ver_todo: %v", err)
	}
	if !repo.capResumenSet || repo.capResumenDepts != nil {
		t.Errorf("ver_todo: resumen debe recibir deptIDs nil, recibió %#v", repo.capResumenDepts)
	}
	if _, err := svc.ListarDocumentos(ctx, "emp", "SUPERVISOR_FINANCIERO", "u1", FiltrosDocumentos{}); err != nil {
		t.Fatalf("Listar ver_todo: %v", err)
	}
	if !repo.capListSet || repo.capListDepts != nil {
		t.Errorf("ver_todo: listado debe recibir deptIDs nil, recibió %#v", repo.capListDepts)
	}

	// (2) Sin cxp.ver_todo (validador de área) => filtra a sus departamentos.
	repo2 := &fakeRepo{deptsDeUsuario: []string{"depA", "depB"}}
	svc2 := NewService(repo2, nil, zap.NewNop())
	svc2.SetPermisos(fakePerms{verTodo: false})
	if _, err := svc2.Bandeja(ctx, "emp", "CUSTOM_VALIDADOR", "u1"); err != nil {
		t.Fatalf("Bandeja validador: %v", err)
	}
	if len(repo2.capResumenDepts) != 2 || repo2.capResumenDepts[0] != "depA" {
		t.Errorf("validador: resumen debe filtrar a [depA depB], recibió %#v", repo2.capResumenDepts)
	}

	// (3) Validador SIN áreas asignadas => filtro no-nil pero vacío (no ve nada; ≠ ver todo).
	repo3 := &fakeRepo{deptsDeUsuario: []string{}}
	svc3 := NewService(repo3, nil, zap.NewNop())
	svc3.SetPermisos(fakePerms{verTodo: false})
	if _, err := svc3.ListarDocumentos(ctx, "emp", "CUSTOM_VALIDADOR", "u1", FiltrosDocumentos{}); err != nil {
		t.Fatalf("Listar validador sin áreas: %v", err)
	}
	if repo3.capListDepts == nil || len(repo3.capListDepts) != 0 {
		t.Errorf("validador sin áreas: filtro debe ser no-nil y vacío, recibió %#v", repo3.capListDepts)
	}
}

func TestAplicarAnticipoGuards(t *testing.T) {
	ctx := context.Background()
	base := func() *fakeRepo {
		return &fakeRepo{
			saldoAnticipo: dec("500000"),
			docs: map[string]Documento{
				"fact": {ID: "fact", Tipo: TipoCxP, Estado: EstRevisado, ProveedorID: "prov-1", Moneda: "CRC", TotalCRC: "1200000.00", NetoCRC: "1200000.00"},
				"ant":  {ID: "ant", Tipo: TipoAnticipo, Estado: EstPagado, ProveedorID: "prov-1", Moneda: "CRC", TotalCRC: "500000.00", NetoCRC: "500000.00"},
			},
		}
	}
	// Feliz: aplica ₡400.000 (≤ saldo y ≤ neto) → llama al repo.
	repo := base()
	svc := NewService(repo, nil, zap.NewNop())
	if _, err := svc.AplicarAnticipo(ctx, "emp", "fact", "ant", "400000", "u1"); err != nil {
		t.Fatalf("aplicación válida => %v", err)
	}
	if !repo.aplicarLlamado {
		t.Error("no se llamó al repo en la aplicación válida")
	}
	// El documento destino no es factura sino un anticipo → ErrFacturaNoNeteable.
	repo = base()
	repo.docs["fact"] = Documento{ID: "fact", Tipo: TipoAnticipo, Estado: EstRecibido, ProveedorID: "prov-1", Moneda: "CRC", NetoCRC: "1000.00"}
	if _, err := NewService(repo, nil, zap.NewNop()).AplicarAnticipo(ctx, "emp", "fact", "ant", "1000", "u1"); err != ErrFacturaNoNeteable {
		t.Errorf("factura=anticipo => %v, quiere ErrFacturaNoNeteable", err)
	}
	// El "anticipo" no es de tipo ANTICIPO → ErrNoEsAnticipo.
	repo = base()
	repo.docs["ant"] = Documento{ID: "ant", Tipo: TipoCxP, Estado: EstPagado, ProveedorID: "prov-1", Moneda: "CRC"}
	if _, err := NewService(repo, nil, zap.NewNop()).AplicarAnticipo(ctx, "emp", "fact", "ant", "1000", "u1"); err != ErrNoEsAnticipo {
		t.Errorf("no es anticipo => %v, quiere ErrNoEsAnticipo", err)
	}
	// Proveedor distinto → ErrProveedorDistinto.
	repo = base()
	repo.docs["ant"] = Documento{ID: "ant", Tipo: TipoAnticipo, Estado: EstPagado, ProveedorID: "prov-2", Moneda: "CRC", NetoCRC: "500000.00"}
	if _, err := NewService(repo, nil, zap.NewNop()).AplicarAnticipo(ctx, "emp", "fact", "ant", "1000", "u1"); err != ErrProveedorDistinto {
		t.Errorf("proveedor distinto => %v, quiere ErrProveedorDistinto", err)
	}
	// Moneda USD → ErrMonedaNoNeteable.
	repo = base()
	repo.docs["fact"] = Documento{ID: "fact", Tipo: TipoCxP, Estado: EstRevisado, ProveedorID: "prov-1", Moneda: "USD", NetoCRC: "1200000.00"}
	if _, err := NewService(repo, nil, zap.NewNop()).AplicarAnticipo(ctx, "emp", "fact", "ant", "1000", "u1"); err != ErrMonedaNoNeteable {
		t.Errorf("moneda USD => %v, quiere ErrMonedaNoNeteable", err)
	}
	// Monto mayor al saldo del anticipo → ErrMontoAplicacionInvalido.
	repo = base()
	if _, err := NewService(repo, nil, zap.NewNop()).AplicarAnticipo(ctx, "emp", "fact", "ant", "600000", "u1"); err != ErrMontoAplicacionInvalido {
		t.Errorf("monto>saldo => %v, quiere ErrMontoAplicacionInvalido", err)
	}
}

// Aplicar varios anticipos a la vez: la suma no puede exceder el neto de la factura, ni
// repetirse el mismo anticipo en el lote.
func TestAplicarAnticiposLoteGuards(t *testing.T) {
	ctx := context.Background()
	base := func() *fakeRepo {
		return &fakeRepo{
			saldoAnticipo: dec("500000"),
			docs: map[string]Documento{
				"fact": {ID: "fact", Tipo: TipoCxP, Estado: EstRevisado, ProveedorID: "p1", Moneda: "CRC", TotalCRC: "600000.00", NetoCRC: "600000.00"},
				"a1":   {ID: "a1", Tipo: TipoAnticipo, Estado: EstPagado, ProveedorID: "p1", Moneda: "CRC", TotalCRC: "500000.00", NetoCRC: "500000.00"},
				"a2":   {ID: "a2", Tipo: TipoAnticipo, Estado: EstPagado, ProveedorID: "p1", Moneda: "CRC", TotalCRC: "500000.00", NetoCRC: "500000.00"},
			},
		}
	}
	// Dos anticipos que suman 550.000 ≤ neto 600.000 → OK, un solo lote.
	repo := base()
	svc := NewService(repo, nil, zap.NewNop())
	if _, err := svc.AplicarAnticiposLote(ctx, "emp", "fact",
		[]AplicacionInput{{AnticipoID: "a1", Monto: dec("300000")}, {AnticipoID: "a2", Monto: dec("250000")}}, "u1"); err != nil {
		t.Fatalf("lote válido => %v", err)
	}
	if repo.loteAplicado != 2 {
		t.Errorf("se esperaban 2 líneas aplicadas, hubo %d", repo.loteAplicado)
	}
	// La suma (700.000) excede el neto (600.000) → rechazo, sin aplicar nada.
	repo = base()
	if _, err := NewService(repo, nil, zap.NewNop()).AplicarAnticiposLote(ctx, "emp", "fact",
		[]AplicacionInput{{AnticipoID: "a1", Monto: dec("400000")}, {AnticipoID: "a2", Monto: dec("300000")}}, "u1"); err != ErrMontoAplicacionInvalido {
		t.Errorf("suma > neto => %v, quiere ErrMontoAplicacionInvalido", err)
	}
	if repo.loteAplicado != 0 {
		t.Error("no debió aplicarse ninguna línea si el lote es inválido")
	}
	// El mismo anticipo repetido en el lote → rechazo.
	repo = base()
	if _, err := NewService(repo, nil, zap.NewNop()).AplicarAnticiposLote(ctx, "emp", "fact",
		[]AplicacionInput{{AnticipoID: "a1", Monto: dec("100000")}, {AnticipoID: "a1", Monto: dec("100000")}}, "u1"); err != ErrMontoAplicacionInvalido {
		t.Errorf("anticipo repetido => %v, quiere ErrMontoAplicacionInvalido", err)
	}
	// Lote vacío → rechazo.
	if _, err := NewService(base(), nil, zap.NewNop()).AplicarAnticiposLote(ctx, "emp", "fact", nil, "u1"); err != ErrMontoAplicacionInvalido {
		t.Errorf("lote vacío => %v, quiere ErrMontoAplicacionInvalido", err)
	}
}

// Vía expresa: los documentos internos de Contabilidad (ANTICIPO, REINTEGRO de caja chica,
// INTERNO) aprueban directo desde RECIBIDO/REVISADO/VALIDADO_DEPTO — sin exigir validación de
// área — con la matriz de firmas; una factura normal sigue exigiendo VALIDADO_DEPTO.
func TestAprobarViaExpresa(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		docs: map[string]Documento{
			"ant": {ID: "ant", Tipo: TipoAnticipo, Estado: EstRecibido, TotalCRC: "500000.00", NetoCRC: "500000.00"},
			// El caso real del atasco: la reposición de caja chica trabada en «Por validar (área)».
			"rei": {ID: "rei", Tipo: TipoReintegro, Estado: EstRevisado, TotalCRC: "150000.00", NetoCRC: "150000.00"},
			"int": {ID: "int", Tipo: TipoInterno, Estado: EstRecibido, TotalCRC: "250000.00", NetoCRC: "250000.00"},
			"fac": {ID: "fac", Tipo: TipoCxP, Estado: EstRevisado, TotalCRC: "500000.00", NetoCRC: "500000.00"},
		},
		rolesAprobaciones: []string{"DIRECTOR_FINANCIERO"}, // ≤₡1M ⇒ 1 firma completa la matriz
	}
	svc := NewService(repo, nil, zap.NewNop())
	for _, id := range []string{"ant", "rei", "int"} {
		repo.capMultiA = ""
		if _, err := svc.Aprobar(ctx, "emp", id, "u1", "DIRECTOR_FINANCIERO"); err != nil {
			t.Fatalf("%s debería aprobar directo (vía expresa): %v", id, err)
		}
		if repo.capMultiA != EstAprobado {
			t.Errorf("%s: debió transicionar a APROBADO vía CambiarEstadoMulti, fue %q", id, repo.capMultiA)
		}
	}
	if _, err := svc.Aprobar(ctx, "emp", "fac", "u1", "DIRECTOR_FINANCIERO"); err != ErrTransicionInvalida {
		t.Errorf("factura normal en REVISADO => %v, quiere ErrTransicionInvalida (sigue exigiendo validación de área)", err)
	}
}

// La solicitud de anticipo exige motivo/respaldo (sustituye a la validación de área que no recorre).
func TestCrearAnticipoExigeMotivo(t *testing.T) {
	svc := NewService(&fakeRepo{}, nil, zap.NewNop())
	_, err := svc.CrearDocumento(context.Background(), "emp",
		DocumentoInput{ProveedorID: "p1", Tipo: TipoAnticipo, Moneda: "CRC", Total: dec("100000")}, "u1")
	if err != ErrMotivoAnticipoRequerido {
		t.Errorf("anticipo sin motivo => %v, quiere ErrMotivoAnticipoRequerido", err)
	}
}

// Guardas de la maqueta de caja chica: límite por vale, fondo suficiente, detalle y gasto.
func TestCrearValeGuards(t *testing.T) {
	ctx := context.Background()
	fondoOK := FondoCajaChica{ID: "f1", Nombre: "Sede Central", Activo: true, CustodioID: "u1",
		MontoAsignado: "200000.00", LimiteVale: "40000.00", Disponible: "50000.00"}
	valeOK := ValeInput{Detalle: "taxi trámite", MontoCRC: dec("15000"), ConceptoID: "c1", ClasificacionID: "cl1", Comprobante: "RECIBO"}

	// Feliz: dentro del límite y del disponible.
	repo := &fakeRepo{fondo: fondoOK}
	if _, err := NewService(repo, nil, zap.NewNop()).CrearVale(ctx, "emp", "f1", valeOK, "ROL", "u1"); err != nil {
		t.Fatalf("vale válido => %v", err)
	}
	if !repo.valeCreado {
		t.Error("no llegó al repo el vale válido")
	}
	// Sobre el límite por vale → bloqueado (va por CxP normal).
	v := valeOK
	v.MontoCRC = dec("45000")
	if _, err := NewService(&fakeRepo{fondo: fondoOK}, nil, zap.NewNop()).CrearVale(ctx, "emp", "f1", v, "ROL", "u1"); err != ErrValeSobreLimite {
		t.Errorf("vale > límite => %v, quiere ErrValeSobreLimite", err)
	}
	// El fondo no alcanza (disponible 50.000, vale 60.000; sin límite para aislar la guarda).
	fSinLimite := fondoOK
	fSinLimite.LimiteVale = "0.00"
	v.MontoCRC = dec("60000")
	if _, err := NewService(&fakeRepo{fondo: fSinLimite}, nil, zap.NewNop()).CrearVale(ctx, "emp", "f1", v, "ROL", "u1"); err != ErrFondoInsuficiente {
		t.Errorf("vale > disponible => %v, quiere ErrFondoInsuficiente", err)
	}
	// Sin detalle / sin gasto.
	v = valeOK
	v.Detalle = "  "
	if _, err := NewService(&fakeRepo{fondo: fondoOK}, nil, zap.NewNop()).CrearVale(ctx, "emp", "f1", v, "ROL", "u1"); err != ErrValeDetalleRequerido {
		t.Errorf("sin detalle => %v, quiere ErrValeDetalleRequerido", err)
	}
	v = valeOK
	v.ClasificacionID = ""
	if _, err := NewService(&fakeRepo{fondo: fondoOK}, nil, zap.NewNop()).CrearVale(ctx, "emp", "f1", v, "ROL", "u1"); err != ErrValeGastoRequerido {
		t.Errorf("sin gasto => %v, quiere ErrValeGastoRequerido", err)
	}
	// Fondo desactivado.
	fInactivo := fondoOK
	fInactivo.Activo = false
	if _, err := NewService(&fakeRepo{fondo: fInactivo}, nil, zap.NewNop()).CrearVale(ctx, "emp", "f1", valeOK, "ROL", "u1"); err != ErrFondoInactivo {
		t.Errorf("fondo inactivo => %v, quiere ErrFondoInactivo", err)
	}
}

// La reposición exige proveedor interno (custodio) y vales pendientes.
func TestGenerarReposicionGuards(t *testing.T) {
	ctx := context.Background()
	base := FondoCajaChica{ID: "f1", Nombre: "Sede Central", Activo: true, CustodioID: "u1",
		MontoAsignado: "200000.00", Disponible: "50000.00"}

	// Sin proveedor interno → 422 con mensaje claro.
	if _, err := NewService(&fakeRepo{fondo: base}, nil, zap.NewNop()).GenerarReposicion(ctx, "emp", "f1", "ROL", "u1"); err != ErrFondoSinProveedor {
		t.Errorf("sin proveedor => %v, quiere ErrFondoSinProveedor", err)
	}
	// Con proveedor pero sin vales pendientes.
	conProv := base
	conProv.ProveedorID = "prov-1"
	if _, err := NewService(&fakeRepo{fondo: conProv}, nil, zap.NewNop()).GenerarReposicion(ctx, "emp", "f1", "ROL", "u1"); err != ErrSinValesPendientes {
		t.Errorf("sin vales => %v, quiere ErrSinValesPendientes", err)
	}
	// Feliz: 2 vales por ₡60.000 → crea el documento de reposición sin error.
	repo := &fakeRepo{fondo: conProv, valesElegibles: []string{"v1", "v2"}, valesTotal: dec("60000")}
	if _, err := NewService(repo, nil, zap.NewNop()).GenerarReposicion(ctx, "emp", "f1", "ROL", "u1"); err != nil {
		t.Fatalf("reposición válida => %v", err)
	}
}

func TestAprobarSegregacion(t *testing.T) {
	ctx := context.Background()
	// Aprobar antes de validar el área → transición inválida.
	svc := NewService(&fakeRepo{doc: Documento{ID: "d1", Estado: EstRevisado, TotalCRC: "100"}}, nil, zap.NewNop())
	if _, err := svc.Aprobar(ctx, "emp", "d1", "u1", "DIRECTOR_FINANCIERO"); err != ErrTransicionInvalida {
		t.Errorf("aprobar sin validar => %v, quiere ErrTransicionInvalida", err)
	}
	// Quien validó no puede aprobar la misma factura (segregación de funciones).
	svc = NewService(&fakeRepo{doc: Documento{ID: "d1", Estado: EstValidadoDepto, ValidadoDeptoPor: "u1", TotalCRC: "100"}}, nil, zap.NewNop())
	if _, err := svc.Aprobar(ctx, "emp", "d1", "u1", "DIRECTOR_FINANCIERO"); err != ErrValidadorNoAprueba {
		t.Errorf("validador aprueba => %v, quiere ErrValidadorNoAprueba", err)
	}
}

func TestProveedorRequestAInput(t *testing.T) {
	// porcentaje válido
	in, err := proveedorRequest{Nombre: "X", RetencionRentaPct: "2.5"}.aInput()
	if err != nil || !in.RetencionRentaPct.Equal(dec("2.5")) {
		t.Errorf("2.5 => %s, err=%v", in.RetencionRentaPct, err)
	}
	// vacío => 0
	in, err = proveedorRequest{Nombre: "X"}.aInput()
	if err != nil || !in.RetencionRentaPct.IsZero() {
		t.Errorf("vacío => %s, err=%v", in.RetencionRentaPct, err)
	}
	// inválido => error
	if _, err := (proveedorRequest{Nombre: "X", RetencionRentaPct: "abc"}).aInput(); err == nil {
		t.Error("retención inválida debería dar error")
	}
}
