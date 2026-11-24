package bancos

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// fakeRepo implementa Repository en memoria para probar el servicio.
type fakeRepo struct {
	// areasDelUsuario: el alcance de «Mi presupuesto» (mig 0124). Nil por defecto a propósito.
	areasDelUsuario []AreaDelUsuario
	// Y el de la mitad de ingreso (mig 0125), por partida.
	partidasDeIngresoDelUsuario []PartidaDelUsuario
	periodosConMovimiento       []string
	// Alcance por persona. `usuarioDeLaEmpresa` arranca en false: ese es el caso a probar.
	usuarioDeLaEmpresa    bool
	puedeVerPresupuesto   bool
	puedeVerIngreso       bool
	naturalezas           map[string]string
	areasGuardadas        []AreaAsignada
	partidasGuardadas     []string
	pidioUsuarioDeEmpresa []string
	cuenta                Cuenta
	existing              map[string]bool
	// Reemisiones del banco (mig 0118). Lo que el servicio LEE para decidir si deja enlazar, y lo
	// que ESCRIBIÓ, para que la prueba pueda comprobar que no enlazó cuando no correspondía.
	candidatosReemision    []CandidatoReemision
	reemisiones            []Reemision
	vigenteReemision       datosDeMovimiento
	salienteReemision      datosDeMovimiento
	errDatosReemision      error
	dependenciasMovimiento []DependenciaDeMovimiento
	enlazoVigente          string
	enlazoSaliente         string
	enlazoMotivo           string
	enlazoMotivoExclusion  string
	enlazoHeredar          bool
	deshizoReemision       string
	// Formatos de banco configurados (mig 0108). Vacío = solo los seis del código.
	formatos []FormatoBanco
	// Tipo de cambio: en cero se comportan como los stubs originales.
	cotizaciones     []Cotizacion
	tcEstado         string
	tcValorCongelado *string
	tcAplicado       []tcAplicacion
	// Dashboard: totales por período (ingreso/gasto según la naturaleza del concepto) y cuántos
	// conceptos en uso siguen sin declararla.
	totales map[string]TotalesEbitda
	// Comparación a paridad de día: totales por TRAMO («2026-10:19»), hasta dónde está cargado
	// cada período, y los días que el servicio pidió (para ver que compara tramos iguales).
	totalesHastaDia        map[string]TotalesEbitda
	ultimoDiaCargado       map[string]int
	diasPedidos            []string
	conceptosSinNaturaleza int
	naturalezaActual       string
	// visibleCxP: si el rubro está dentro del alcance de `cxp.catalogo` (la puerta de Contabilidad).
	visibleCxP bool
	// Visibilidad por clasificación (mig 0080): id + valor guardado.
	visibleClasifGuardada [2]string
	// Dimensiones del gasto (departamento y sede) y presupuesto
	sedes         []Sede
	departamentos []Departamento
	gastoDim      []GastoDimension
	partidasDim   []GastoPartidaDimension
	presupuesto   []PresupuestoLinea
	movsDeClasif  int
	dimClasif     [3]string
	dimMov        [3]string
	agrupoPor     string
	pidioDimID    string
	presuGuardado [3]string
	// Presupuesto de INGRESO (mig 0116): la otra mitad de la meta.
	metasIngreso     []MetaIngreso
	metaGuardada     [3]string
	metaBorrada      [2]string
	ingresoReal      []FilaIngreso
	ingresoHistorico []FilaIngreso
	ingresoTotal     decimal.Decimal
	gastoTotal       decimal.Decimal
	naturalezaClasif string
	// Outlook del año (la tercera columna).
	realPorMes  map[string][2]decimal.Decimal
	metasPorMes map[string][2]decimal.Decimal
	// Subpresupuesto por partida y administración del catálogo de departamentos
	presuClasifID  string
	usoDepto       UsoDepartamento
	deptoActivo    bool
	deptoEliminado bool
	sedeEliminada  bool
	// Clasificación en bloque desde Excel
	cuentasLista  []CuentaListItem
	movsCalzados  []MovimientoCalzado
	asignados     []AsignacionClasif
	plantillaMovs []MovimientosParaPlantilla
	// Análisis de partidas en el tiempo (tendencia y desvío contra su propio promedio)
	saludMeses    []SaludMes
	seriePartidas []TendenciaPartida
	// Por rango «desde..hasta»: lo necesita la comparación anual, que pide DOS rangos distintos
	// en la misma llamada. Si está vacío se usan los campos de arriba, que es lo que esperan las
	// pruebas que ya existían.
	saludPorRango map[string][]SaludMes
	seriePorRango map[string][]TendenciaPartida
	// La cartera de CxP que ve el flujo proyectado.
	cxpVencido   decimal.Decimal
	cxpPorVencer decimal.Decimal
	// Día a día por partida: `serieDiaria` es lo que devuelve y `diarioPedido` guarda los ids con
	// los que se lo llamó, para poder verificar el recorte y el dedup que hace el service.
	serieDiaria  []SerieDiariaPartida
	diarioPedido []string
	// Resumen de la selección activa (hoja de trabajo)
	resumenFiltro []ResumenFiltroRow
	inserted      []MovimientoParaInsertar
	// Consulta por segmento (mig 0077). `filtroMovs` guarda el filtro con el que se llamó a
	// ListarMovimientos: es la única forma de comprobar que el alcance se FORZÓ y que el cliente
	// no pudo ensancharlo.
	filtroMovs       FiltrosMovimientos
	listaMovs        ListaMovimientos
	alcance          []string
	partidasAlcance  []PartidaDelSegmento
	cuentasAlcance   []CuentaDelSegmento
	cargadoHasta     string
	rolesConsulta    []RolDeConsulta
	asignConsulta    []AsignacionConsulta
	consultaGuardada struct {
		clasificacionID string
		rolIDs          []string
		llamado         bool
	}
	movEnAlcance bool
	// Búsqueda de un movimiento que no aparece: lo del alcance viene completo, lo de afuera solo
	// se cuenta (es todo lo que se divulga).
	busquedaMios     []MovimientoRow
	busquedaFuera    int
	enganche         string
	faltanteCreado   [6]string // usuario, fecha, monto, referencia, motivo, movimiento enganchado
	reportesAbiertos map[string]AvisoAbiertoDeFila
	reporteCreado    [3]string // movimiento, usuario, motivo
	reportes         []ReporteSegmentacion
	reporteResuelto  [4]string // reporte, usuario, resolución, respuesta
	// Lo sin partida de una cuenta del segmento (también se devuelve completo), y con qué usuario
	// se pidieron los avisos de las filas: el recorte del motivo depende de él.
	busquedaSinPartida  []MovimientoRow
	avisosPedidosPor    string
	resueltosPedidosPor string
	// «Mi partida» del 22-set-2026: avisos resueltos por movimiento, «Mis avisos» (con la marca de
	// si se consultó, para probar que un alcance vacío ni llega a la base) y la carga por cuenta.
	avisosResueltos    map[string]AvisoResuelto
	misAvisos          ListaMisAvisos
	misAvisosPedidos   bool
	misAvisosPagina    [2]int
	cargaCuentas       []CuentaCargadaHasta
	cargaCuentasPedida bool
	// Aviso de «está en otro mes»: lo que devuelve y con qué se pidió.
	otrosMeses           []CoincidenciaEnOtroMes
	capAlcanceOtrosMeses []string
	capQOtrosMeses       string
	// Sello «aplicado en el sistema anterior» (mig 0091): lo sembrado y lo que se le mandó escribir.
	debitosCandidatos []DebitoCandidato
	sellosEscritos    []SelloPagoExterno
	sellosVisibles    map[string]SelloVisible
	cargaRevertida    string
	quitadosEnReversa int
	resumenSellos     ResumenPagosExternos
	// Tesorería (saldos diarios y checklist de carga)
	saldosDia       []SaldoDelDia
	serieSaldos     []PuntoSaldo
	carga           []CargaCuenta
	hoyCR           string
	saldosGuardados []SaldoInput
	fechaGuardada   string
	// Conciliación bancaria mensual
	actas          []ActaConciliacion
	partidas       []PartidaConciliacion
	partidaCreada  PartidaInput
	signoUsado     int
	partidaAnulada string
	actaFirmada    string
	snapshotFirma  [3]string
	fechaRevisada  string
	congelo        bool
	cerrado        bool
	// Descubridor de patrones
	lineasSinClasif []LineaSinClasificar
	descripciones   []string
	// Huella Bancos↔CxP
	movsConHuella []MovimientoConHuella
	yaEnlazados   map[string]string
	// Diccionario del catálogo: foto actual y contadores de lo que se escribió.
	conceptosCat     []Concepto
	clasifsCat       []ClasificacionItem
	reglasCat        []Regla
	conceptosCreados int
	clasifsCreadas   int
	reglasCreadas    int
	ultimaRegla      NuevaRegla
}

func (f *fakeRepo) ListarCuentas(context.Context, string, bool) ([]CuentaListItem, error) {
	return f.cuentasLista, nil
}
func (f *fakeRepo) CuentaByID(context.Context, string, string) (Cuenta, error) { return f.cuenta, nil }
func (f *fakeRepo) CrearImportacion(context.Context, string, string, string, string, Banco, []byte, string) (string, error) {
	return "imp-1", nil
}
func (f *fakeRepo) ImportacionArchivo(context.Context, string, string) (string, []byte, error) {
	return f.cuenta.ID, nil, nil
}
func (f *fakeRepo) NaturalKeysExistentes(_ context.Context, _ string, keys []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, k := range keys {
		if f.existing[k] {
			out[k] = true
		}
	}
	return out, nil
}
func (f *fakeRepo) ConfirmarConMovimientos(_ context.Context, _, _, _, _ string, movs []MovimientoParaInsertar) (int, error) {
	f.inserted = movs
	return len(movs), nil
}
func (f *fakeRepo) SetCuentaIBANSiVacio(context.Context, string, string, string) error { return nil }

// Formatos de banco configurables (mig 0108). El doble devuelve LO QUE SE LE SIEMBRE: la lista
// vacía es el estado de toda instalación hasta que alguien configure el primero, y es la que hace
// que estas pruebas sigan midiendo exactamente lo que medían antes —los seis del código—.
func (f *fakeRepo) FormatosDeBanco(context.Context, string, bool) ([]FormatoBanco, error) {
	return f.formatos, nil
}

func (f *fakeRepo) GuardarFormatoBanco(_ context.Context, _ string, fb FormatoBanco, _ string) (FormatoBanco, error) {
	if fb.ID == "" {
		fb.ID = "formato-de-prueba"
	}
	f.formatos = append(f.formatos, fb)
	return fb, nil
}

func (f *fakeRepo) BorrarFormatoBanco(context.Context, string, string) error { return nil }

func mp(fecha, doc, deb, cred string, indice int) MovimientoParsed {
	t, _ := time.Parse("2006-01-02", fecha)
	return MovimientoParsed{
		Fecha: t, Documento: doc, Debito: dec(deb), Credito: dec(cred), IndiceOcurrencia: indice,
	}
}

func TestClasificar(t *testing.T) {
	const cuentaID = "cta-1"
	movA := mp("2026-06-01", "1", "0", "100.00", 1) // nuevo
	movB := mp("2026-06-02", "2", "50.00", "0", 1)  // dup real (misma tupla que C)
	movC := mp("2026-06-02", "2", "50.00", "0", 2)  // dup real
	movD := mp("2026-06-03", "3", "0", "200.00", 1) // reimportación
	res := ParseResult{Banco: BancoBN, Movimientos: []MovimientoParsed{movA, movB, movC, movD}}

	repo := &fakeRepo{existing: map[string]bool{naturalKey(cuentaID, movD): true}}
	svc := NewService(repo, nil, zap.NewNop(), true)

	resumen, movs, err := svc.clasificar(context.Background(), "emp", cuentaID, "CRC", res)
	if err != nil {
		t.Fatalf("clasificar: %v", err)
	}
	if resumen.Leidas != 4 || resumen.Nuevas != 1 || resumen.DuplicadosReales != 2 || resumen.Reimportacion != 1 {
		t.Fatalf("resumen = %+v", resumen)
	}
	if movs[0].EstadoDuplicado != DupNuevo {
		t.Errorf("mov A = %s, quería NUEVO", movs[0].EstadoDuplicado)
	}
	if movs[1].EstadoDuplicado != DupReal || movs[2].EstadoDuplicado != DupReal {
		t.Errorf("mov B/C = %s/%s, quería DUPLICADO_REAL", movs[1].EstadoDuplicado, movs[2].EstadoDuplicado)
	}
	if movs[3].EstadoDuplicado != DupReimport {
		t.Errorf("mov D = %s, quería REIMPORTACION", movs[3].EstadoDuplicado)
	}
}

func TestSeleccionarParaInsertar(t *testing.T) {
	const cuentaID = "cta-1"
	normal := mp("2026-06-01", "1", "0", "100.00", 1)
	reimport := mp("2026-06-02", "2", "50.00", "0", 1)
	excluido := mp("2026-06-03", "3", "0", "300.00", 1)
	res := ParseResult{Movimientos: []MovimientoParsed{normal, reimport, excluido}}

	existing := map[string]bool{naturalKey(cuentaID, reimport): true}
	excluir := map[string]bool{naturalKey(cuentaID, excluido): true}

	got := seleccionarParaInsertar(cuentaID, "CRC", res, existing, excluir)
	if len(got) != 1 {
		t.Fatalf("seleccionados = %d, quería 1 (solo el nuevo)", len(got))
	}
	if !got[0].MontoOriginal.Equal(dec("100.00")) || !got[0].MontoCRC.Equal(dec("100.00")) {
		t.Errorf("CRC: monto_original=%s monto_crc=%s, quería 100/100", got[0].MontoOriginal, got[0].MontoCRC)
	}

	// USD: monto_crc queda en 0 (pendiente del motor de TC).
	usd := seleccionarParaInsertar(cuentaID, "USD", ParseResult{Movimientos: []MovimientoParsed{normal}}, nil, nil)
	if !usd[0].MontoOriginal.Equal(dec("100.00")) || !usd[0].MontoCRC.Equal(dec("0")) {
		t.Errorf("USD: monto_original=%s monto_crc=%s, quería 100/0", usd[0].MontoOriginal, usd[0].MontoCRC)
	}
}

// MovimientoEnlazable: el débito que una persona elige desde el arqueo de excepción (mig 0104).
// Doble inerte: lo que hace es una consulta con sus guardas en el WHERE, y eso se prueba contra
// PostgreSQL de verdad; acá existe para que el fake siga cumpliendo la interfaz.
func (f *fakeRepo) MovimientoEnlazable(context.Context, string, string) (MovimientoEnlazable, error) {
	return MovimientoEnlazable{}, ErrMovimientoNoEnlazable
}

// Deshacer el enlace (mig 0104). Dobles inertes: son consultas con sus guardas en el WHERE y se
// prueban contra PostgreSQL de verdad.
func (f *fakeRepo) EnlaceDelMovimiento(context.Context, string, string) (EnlaceDelMovimiento, error) {
	return EnlaceDelMovimiento{}, ErrMovimientoSinEnlace
}
func (f *fakeRepo) LimpiarEnlacePago(context.Context, string, string) (int64, error) { return 0, nil }
