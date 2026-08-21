package bancos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// EstadoDuplicado clasifica cada línea del preview (RN-07/08).
type EstadoDuplicado string

const (
	DupNuevo    EstadoDuplicado = "NUEVO"
	DupReal     EstadoDuplicado = "DUPLICADO_REAL" // duplicada dentro del mismo archivo (BAC)
	DupReimport EstadoDuplicado = "REIMPORTACION"  // ya existe en la BD (idempotencia)
)

// Cuenta es la vista mínima de cuenta_bancaria que necesita el importador.
type Cuenta struct {
	ID      string
	BancoID string
	IBAN    string
	Moneda  string
	Alias   string
}

// CuentaListItem es una cuenta para el selector del importador / catálogo.
type CuentaListItem struct {
	ID     string `json:"id"`
	Alias  string `json:"alias"`
	Banco  string `json:"banco"`
	IBAN   string `json:"iban"`
	Moneda string `json:"moneda"`
	// Activo: una cuenta desactivada conserva sus movimientos y su historia, pero no
	// aparece en el importador ni en los filtros.
	Activo bool `json:"activo"`
}

// MovimientoPreview es una línea normalizada + su clasificación de duplicado, para la UI.
// Advertencias lista los problemas de integridad §19 de ESTA línea (vacío = línea sana).
// Una línea con advertencias NO se inserta al confirmar (gate duro).
type MovimientoPreview struct {
	NaturalKey       string          `json:"natural_key"`
	Fecha            string          `json:"fecha"`
	Documento        string          `json:"documento"`
	Descripcion      string          `json:"descripcion"`
	Debito           string          `json:"debito"`
	Credito          string          `json:"credito"`
	Moneda           string          `json:"moneda"`
	IndiceOcurrencia int             `json:"indice_ocurrencia"`
	EstadoDuplicado  EstadoDuplicado `json:"estado_duplicado"`
	Advertencias     []string        `json:"advertencias"`
}

// problemasLinea aplica las validaciones §19 de una línea. Devuelve los problemas
// que la hacen inválida (no se persiste). Hoy: monto negativo, débito y crédito
// simultáneos >0 (salvo [T1], sin adaptador que lo produzca) y línea sin monto.
func problemasLinea(m MovimientoParsed) []string {
	w := []string{} // no-nil: al serializar debe ser [] (no null) para el contrato del frontend
	if m.Debito.IsNegative() || m.Credito.IsNegative() {
		w = append(w, "Monto negativo en débito o crédito")
		return w // sin monto válido, las demás reglas no aplican
	}
	if m.Debito.IsPositive() && m.Credito.IsPositive() {
		w = append(w, "Débito y crédito simultáneos (>0) en la misma línea")
	}
	if !m.Debito.IsPositive() && !m.Credito.IsPositive() {
		w = append(w, "Línea sin monto (débito y crédito en cero)")
	}
	return w
}

// monedaMismatch: el archivo declara una moneda y la cuenta otra (choque definitivo).
// Si el archivo no declara moneda, no bloquea (nada con qué contrastar).
func monedaMismatch(cuentaMoneda, archivoMoneda string) bool {
	return cuentaMoneda != "" && archivoMoneda != "" && cuentaMoneda != archivoMoneda
}

// Resumen son los totales del preview.
type Resumen struct {
	Leidas           int `json:"leidas"`
	Nuevas           int `json:"nuevas"`
	DuplicadosReales int `json:"duplicados_reales"`
	Reimportacion    int `json:"reimportacion"`
	// Invalidas: líneas con problemas de integridad §19 que NO se insertan
	// (no cuentan en Nuevas/DuplicadosReales/Reimportacion).
	Invalidas int `json:"invalidas"`
}

// PreviewResult es el resultado de subir/previsualizar un archivo.
// Advertencias son avisos a nivel de archivo (p. ej. cuenta USD sin TC del mes) — §19.
type PreviewResult struct {
	ImportacionID string              `json:"importacion_id"`
	Banco         Banco               `json:"banco"`
	IBANArchivo   string              `json:"iban_archivo"`
	Resumen       Resumen             `json:"resumen"`
	Movimientos   []MovimientoPreview `json:"movimientos"`
	Advertencias  []string            `json:"advertencias"`
}

// MovimientoParaInsertar es una línea lista para persistir en movimiento_bancario.
type MovimientoParaInsertar struct {
	NaturalKey       string
	Fecha            time.Time
	Documento        string
	Descripcion      string
	Debito           decimal.Decimal
	Credito          decimal.Decimal
	MontoOriginal    decimal.Decimal
	MontoCRC         decimal.Decimal
	TCAplicado       *decimal.Decimal
	IndiceOcurrencia int
}

// Repository abstrae el acceso a datos del importador.
type Repository interface {
	ListarCuentas(ctx context.Context, empresaID string, incluirInactivas bool) ([]CuentaListItem, error)
	CuentaByID(ctx context.Context, empresaID, cuentaID string) (Cuenta, error)
	CrearImportacion(ctx context.Context, empresaID, cuentaID, hash, nombre string, banco Banco, archivo []byte, usuarioID string) (string, error)
	ImportacionArchivo(ctx context.Context, empresaID, importacionID string) (cuentaID string, archivo []byte, err error)
	NaturalKeysExistentes(ctx context.Context, empresaID string, keys []string) (map[string]bool, error)
	ConfirmarConMovimientos(ctx context.Context, empresaID, cuentaID, importacionID, moneda string, movs []MovimientoParaInsertar) (int, error)
	SetCuentaIBANSiVacio(ctx context.Context, empresaID, cuentaID, iban string) error

	// Administración de bancos y cuentas (catálogo)
	ListarBancos(ctx context.Context, empresaID string, incluirInactivos bool) ([]BancoItem, error)
	CrearBanco(ctx context.Context, empresaID, nombre string) (BancoItem, error)
	RenombrarBanco(ctx context.Context, empresaID, bancoID, nombre string) error
	CrearCuenta(ctx context.Context, empresaID, bancoID, alias, iban, moneda string) (CuentaListItem, error)
	RenombrarCuenta(ctx context.Context, empresaID, cuentaID, alias string) error
	// Corregir lo creado por error. Eliminar es físico solo si no hay nada colgando; si hay,
	// devuelve el detalle y queda desactivar. La moneda y el IBAN solo se cambian si la
	// cuenta no tiene movimientos (cambiarlos reinterpretaría montos ya importados).
	ActualizarCuenta(ctx context.Context, empresaID, cuentaID string, c CambioDeCuenta) error
	UsoDeCuenta(ctx context.Context, empresaID, cuentaID string) (UsoDeCuenta, error)
	EliminarCuenta(ctx context.Context, empresaID, cuentaID string) error
	CambiarActivoCuenta(ctx context.Context, empresaID, cuentaID string, activo bool) error
	EliminarBanco(ctx context.Context, empresaID, bancoID string) error
	CambiarActivoBanco(ctx context.Context, empresaID, bancoID string, activo bool) error

	// Clasificación, catálogo y movimientos (Fase 1)
	ListarReglas(ctx context.Context, empresaID string) ([]Regla, error)
	ListarMovimientos(ctx context.Context, empresaID string, f FiltrosMovimientos) (ListaMovimientos, error)
	ResumenFiltro(ctx context.Context, empresaID string, f FiltrosMovimientos, agrupar string) ([]ResumenFiltroRow, error)
	MovimientosDeImportacion(ctx context.Context, empresaID, importacionID string) ([]MovParaClasificar, error)
	MovimientosNoIdentificados(ctx context.Context, empresaID string) ([]MovParaClasificar, error)
	AplicarClasificaciones(ctx context.Context, empresaID string, updates []MovClasifUpdate) (int, error)
	ReclasificarMovimiento(ctx context.Context, empresaID, movID, conceptoID, clasificacionID string) error
	CrearRegla(ctx context.Context, empresaID string, r NuevaRegla) (string, error)
	ActualizarRegla(ctx context.Context, empresaID, reglaID string, cambios CambiosRegla) error
	EliminarRegla(ctx context.Context, empresaID, reglaID string) error
	MovimientoClasif(ctx context.Context, empresaID, movID string) (MovClasifActual, error)
	ContarNoIdentificadosConPalabra(ctx context.Context, empresaID, palabra, aplicaA string) (int, error)
	ExisteReglaConPalabra(ctx context.Context, empresaID, palabra string) (bool, error)
	ClasificarMasivo(ctx context.Context, empresaID string, movIDs []string, conceptoID, clasificacionID string) (int, error)
	ResumenClasificacion(ctx context.Context, empresaID, periodo string) (ResumenClasif, error)
	ListarConceptos(ctx context.Context, empresaID string, soloCxP bool) ([]Concepto, error)
	ListarClasificaciones(ctx context.Context, empresaID string, soloCxP bool) ([]ClasificacionItem, error)
	CrearConcepto(ctx context.Context, empresaID, nombre string, visibleCxP bool) (Concepto, error)
	CrearClasificacion(ctx context.Context, empresaID, conceptoID, nombre, cuentaContable string) (ClasificacionItem, error)
	RenombrarConcepto(ctx context.Context, empresaID, conceptoID, nombre string) error
	CambiarVisibilidadCxP(ctx context.Context, empresaID, conceptoID string, visible bool) error
	// CambiarNaturaleza declara si el concepto es INGRESO, GASTO o NEUTRO y devuelve el valor viejo.
	CambiarNaturaleza(ctx context.Context, empresaID, conceptoID, naturaleza string) (anterior string, err error)
	EliminarConcepto(ctx context.Context, empresaID, conceptoID string) error
	RenombrarClasificacion(ctx context.Context, empresaID, clasificacionID, nombre string) error
	ReasignarConceptoClasificacion(ctx context.Context, empresaID, clasificacionID, nuevoConceptoID string) error
	EliminarClasificacion(ctx context.Context, empresaID, clasificacionID string) error
	// Fusionar: mover TODO lo que usa una entrada del catálogo a otra y borrar el origen.
	// Es la única forma de limpiar un duplicado que ya tiene movimientos encima.
	FusionarConceptos(ctx context.Context, empresaID, origenID, destinoID string) (ResumenFusion, error)
	FusionarClasificaciones(ctx context.Context, empresaID, origenID, destinoID string, permitirOtroConcepto bool) (ResumenFusion, error)

	// Tipo de cambio (Fase 1)
	UpsertCotizacion(ctx context.Context, empresaID, fecha string, valor decimal.Decimal, fuente string) error
	CotizacionesMes(ctx context.Context, empresaID string, anio, mes int) ([]Cotizacion, error)
	EstadoTCMes(ctx context.Context, empresaID string, anio, mes int) (estado string, valorCongelado *string, err error)
	AplicarProvisional(ctx context.Context, empresaID string, anio, mes int, tcAntes15, tcDesde15 decimal.Decimal) (int, error)
	CongelarTC(ctx context.Context, empresaID string, anio, mes int, valor decimal.Decimal) (int, error)

	// Cuadre y dashboard (Fase 1)
	Cuadre(ctx context.Context, empresaID, periodo string) ([]CuadreRow, error)
	CuadreArbol(ctx context.Context, empresaID, periodo string) ([]CuadreArbolRow, error)
	// TotalesPeriodo: ingreso y gasto SEGÚN LA NATURALEZA declarada del concepto, más lo que quedó
	// fuera del EBITDA por no estar declarado (ver naturaleza.go).
	TotalesPeriodo(ctx context.Context, empresaID, periodo string) (TotalesEbitda, error)
	// ConceptosSinNaturaleza: cuántos conceptos EN USO siguen en NEUTRO (la acción pendiente).
	ConceptosSinNaturaleza(ctx context.Context, empresaID string) (int, error)

	// Análisis visual (Fase B)
	SerieMensual(ctx context.Context, empresaID, desde, hasta string) ([]SerieMensualPunto, error)
	// Análisis de partidas en el tiempo: tendencia y desvío contra su propio promedio.
	SaludMeses(ctx context.Context, empresaID, desde, hasta string) ([]SaludMes, error)
	// Clasificación en bloque desde Excel: hallar el movimiento por su tupla y asignar la partida
	// de cada fila (cada una con la suya, a diferencia del clasificar-masivo).
	BuscarMovimientosPorTupla(ctx context.Context, empresaID string, cuentas, fechas, debitos, creditos, documentos []string) ([]MovimientoCalzado, error)
	AplicarClasificacionesEnBloque(ctx context.Context, empresaID string, asigs []AsignacionClasif) (int, error)
	MovimientosPlantillaClasif(ctx context.Context, empresaID, desde, hasta string, soloSinClasificar bool, limite int) ([]MovimientosParaPlantilla, error)
	SeriePorPartida(ctx context.Context, empresaID, desde, hasta string) ([]TendenciaPartida, error)
	CalendarioDiario(ctx context.Context, empresaID, periodo string) ([]DiaCalendario, error)
	ResumenPorCuenta(ctx context.Context, empresaID, periodo string) ([]CuentaResumen, error)

	// Proyecciones (Fase C)
	SendaIngresos(ctx context.Context, empresaID, periodo string) ([]DiaMonto, error)
	SendasIngresosRango(ctx context.Context, empresaID string, periodos []string) ([]SendaMes, error)
	IngresosPorClasificacion(ctx context.Context, empresaID, periodo string) ([]LineaIngreso, error)
	GuardarEscenario(ctx context.Context, empresaID string, e EscenarioNuevo) (string, error)
	ListarEscenarios(ctx context.Context, empresaID, periodo string) ([]EscenarioGuardado, error)

	// Traslados/overnight y cierre de período (Fase 1)
	PropuestasTraslados(ctx context.Context, empresaID, periodo string, tolerancia decimal.Decimal) ([]PropuestaTraslado, error)
	MovimientoParaTraslado(ctx context.Context, empresaID, movID string) (MovTraslado, error)
	EmparejarTraslado(ctx context.Context, empresaID, debitoID, creditoID string) error
	DesemparejarTraslado(ctx context.Context, empresaID, movID string) error
	CerrarPeriodo(ctx context.Context, empresaID string, anio, mes, noIdentificados int, usuarioID string) error
	PeriodoCerrado(ctx context.Context, empresaID string, anio, mes int) (bool, error)

	// Saldos diarios y checklist de carga (Tanda 1: tesorería)
	SaldosDelDia(ctx context.Context, empresaID, fecha string) ([]SaldoDelDia, string, error)
	SerieSaldos(ctx context.Context, empresaID, fecha string, dias int) ([]PuntoSaldo, error)
	GuardarSaldos(ctx context.Context, empresaID, fecha string, saldos []SaldoInput, usuarioID string) (int, error)
	CargaDelPeriodo(ctx context.Context, empresaID, periodo string) ([]CargaCuenta, error)

	// Huella Bancos↔CxP (barrido de pagos)
	MovimientosConHuella(ctx context.Context, empresaID, prefijo, importacionID string) ([]MovimientoConHuella, error)
	EnlazarPagoCxP(ctx context.Context, empresaID, movimientoID, documentoID, conceptoID, clasificacionID string) (bool, bool, error)

	// Descubridor de patrones sin clasificar
	LineasSinClasificar(ctx context.Context, empresaID, periodo string) ([]LineaSinClasificar, error)
	DescripcionesEmpresa(ctx context.Context, empresaID string) ([]string, error)

	// Conciliación bancaria mensual (acta, partidas en tránsito y congelamiento del saldo)
	ActasDelMes(ctx context.Context, empresaID string, anio, mes int) ([]ActaConciliacion, error)
	PartidasDelMes(ctx context.Context, empresaID, cuentaID string, anio, mes int) ([]PartidaConciliacion, error)
	CrearPartida(ctx context.Context, empresaID string, in PartidaInput, signo int, usuarioID string) (string, error)
	AnularPartida(ctx context.Context, empresaID, partidaID, usuarioID string) error
	FirmarActa(ctx context.Context, empresaID, cuentaID string, anio, mes int, banco, libros, ajuste, usuarioID string) error
	RevisarSaldos(ctx context.Context, empresaID, fecha, usuarioID string, congelar bool) (int, error)

	// Parámetros por empresa (Fase D)
	ToleranciaTraslado(ctx context.Context, empresaID string) (decimal.Decimal, error)
	ActualizarTolerancia(ctx context.Context, empresaID string, pct decimal.Decimal) error

	// Análisis/exportación y proyecciones (Fase D — export)
	MovimientosParaExport(ctx context.Context, empresaID string, f FiltrosMovimientos) ([]MovimientoExport, error)
	// EncabezadoReporte trae la razón social y quién genera, para identificar el documento.
	EncabezadoReporte(ctx context.Context, empresaID, usuarioID string) (empresa, detalle, usuario string, err error)

	// Tipo de cambio — sync BCCR (Fase D)
	CotizacionExistente(ctx context.Context, empresaID, fecha string) (valor string, fuente string, existe bool, err error)
	UpsertCotizacionBCCR(ctx context.Context, empresaID, fecha string, valor decimal.Decimal) (escrito bool, err error)
	RegistrarSyncBCCR(ctx context.Context, l BCCRSyncLog) error
	UltimoSyncBCCR(ctx context.Context, empresaID string) (*BCCRSyncLog, error)
	EmpresasActivas(ctx context.Context) ([]string, error)
}

// Service orquesta la importación, clasificación, TC, cuadre, traslados y cierre.
type Service struct {
	repo             Repository
	audit            *shared.Audit
	log              *zap.Logger
	cierreBloqueante bool
	bccr             CotizacionFetcher // nil si BCCR no está configurado (fallback manual)
	conciliadorCxP   ConciliadorCxP    // nil si CxP no está conectado (no hay barrido de huellas)
}

// NewService construye el servicio del módulo Bancos.
func NewService(repo Repository, audit *shared.Audit, log *zap.Logger, cierreBloqueante bool) *Service {
	return &Service{repo: repo, audit: audit, log: log, cierreBloqueante: cierreBloqueante}
}

// SetBCCR inyecta el fetcher del BCCR (opcional). Sin él, el sync devuelve
// ErrBCCRNoConfigurado y el motor de TC sigue siendo 100% manual.
func (s *Service) SetBCCR(f CotizacionFetcher) { s.bccr = f }

// Cuentas lista las cuentas de la empresa activa (para el selector del importador).
func (s *Service) Cuentas(ctx context.Context, empresaID string, incluirInactivas bool) ([]CuentaListItem, error) {
	return s.repo.ListarCuentas(ctx, empresaID, incluirInactivas)
}

// Preview parsea y clasifica el archivo, y crea la importación (guarda el original, RN-06).
func (s *Service) Preview(ctx context.Context, empresaID, cuentaID, nombre string, archivo []byte, usuarioID string) (PreviewResult, error) {
	cuenta, err := s.repo.CuentaByID(ctx, empresaID, cuentaID)
	if err != nil {
		return PreviewResult{}, err
	}
	res, err := parseArchivo(archivo)
	if err != nil {
		return PreviewResult{}, err
	}
	if ibanMismatch(cuenta.IBAN, res.IBAN) {
		return PreviewResult{}, ErrIBANNoCoincide
	}
	if monedaMismatch(cuenta.Moneda, res.MonedaArchivo) {
		return PreviewResult{}, ErrMonedaNoCoincide
	}
	resumen, movs, err := s.clasificar(ctx, empresaID, cuentaID, cuenta.Moneda, res)
	if err != nil {
		return PreviewResult{}, err
	}
	impID, err := s.repo.CrearImportacion(ctx, empresaID, cuentaID, sha256hex(archivo), nombre, res.Banco, archivo, usuarioID)
	if err != nil {
		return PreviewResult{}, err
	}
	return PreviewResult{
		ImportacionID: impID,
		Banco:         res.Banco,
		IBANArchivo:   res.IBAN,
		Resumen:       resumen,
		Movimientos:   movs,
		Advertencias:  advertenciasArchivo(cuenta.Moneda, movs),
	}, nil
}

// PreviewExistente reconstruye el preview de una importación ya cargada (re-parsea el original).
func (s *Service) PreviewExistente(ctx context.Context, empresaID, importacionID string) (PreviewResult, error) {
	cuentaID, archivo, err := s.repo.ImportacionArchivo(ctx, empresaID, importacionID)
	if err != nil {
		return PreviewResult{}, err
	}
	cuenta, err := s.repo.CuentaByID(ctx, empresaID, cuentaID)
	if err != nil {
		return PreviewResult{}, err
	}
	res, err := parseArchivo(archivo)
	if err != nil {
		return PreviewResult{}, err
	}
	if ibanMismatch(cuenta.IBAN, res.IBAN) {
		return PreviewResult{}, ErrIBANNoCoincide
	}
	if monedaMismatch(cuenta.Moneda, res.MonedaArchivo) {
		return PreviewResult{}, ErrMonedaNoCoincide
	}
	resumen, movs, err := s.clasificar(ctx, empresaID, cuentaID, cuenta.Moneda, res)
	if err != nil {
		return PreviewResult{}, err
	}
	return PreviewResult{
		ImportacionID: importacionID,
		Banco:         res.Banco,
		IBANArchivo:   res.IBAN,
		Resumen:       resumen,
		Movimientos:   movs,
		Advertencias:  advertenciasArchivo(cuenta.Moneda, movs),
	}, nil
}

// Confirmar persiste los movimientos no excluidos y no reimportados; marca la importación confirmada.
func (s *Service) Confirmar(ctx context.Context, empresaID, importacionID string, excluir []string, usuarioID string) (int, error) {
	cuentaID, archivo, err := s.repo.ImportacionArchivo(ctx, empresaID, importacionID)
	if err != nil {
		return 0, err
	}
	cuenta, err := s.repo.CuentaByID(ctx, empresaID, cuentaID)
	if err != nil {
		return 0, err
	}
	res, err := parseArchivo(archivo)
	if err != nil {
		return 0, err
	}
	if ibanMismatch(cuenta.IBAN, res.IBAN) {
		return 0, ErrIBANNoCoincide
	}
	if monedaMismatch(cuenta.Moneda, res.MonedaArchivo) {
		return 0, ErrMonedaNoCoincide
	}

	keys := naturalKeys(cuentaID, res)
	existing, err := s.repo.NaturalKeysExistentes(ctx, empresaID, keys)
	if err != nil {
		return 0, err
	}
	movs := seleccionarParaInsertar(cuentaID, cuenta.Moneda, res, existing, toSet(excluir))

	inserted, err := s.repo.ConfirmarConMovimientos(ctx, empresaID, cuentaID, importacionID, cuenta.Moneda, movs)
	if err != nil {
		return 0, err
	}

	// Conversión a colones de lo recién importado (cuentas en otra moneda). Va ACÁ y no solo al
	// registrar el tipo de cambio: si el TC del mes ya existe, el movimiento no puede quedar en
	// cero por haber entrado después. Best-effort: no bloquea el confirm.
	s.AplicarTCImportado(ctx, empresaID, cuenta.Moneda, movs)

	// Auto-clasificación de lo recién importado (best-effort; no bloquea el confirm).
	if n, err := s.clasificarImportacion(ctx, empresaID, importacionID); err != nil {
		s.log.Warn("auto-clasificación tras importar falló", zap.String("importacion", importacionID), zap.Error(err))
	} else if n > 0 {
		s.log.Info("auto-clasificados", zap.String("importacion", importacionID), zap.Int("n", n))
	}

	// Huella Bancos↔CxP: los pagos hechos por la macro se emparejan solos con su factura.
	s.conciliarCxPImportacion(ctx, empresaID, importacionID, usuarioID)

	// Memoria IBAN (best-effort): si la cuenta no tenía IBAN y el archivo trae uno, se guarda.
	if res.IBAN != "" && cuenta.IBAN == "" {
		if err := s.repo.SetCuentaIBANSiVacio(ctx, empresaID, cuentaID, res.IBAN); err != nil {
			s.log.Warn("no se pudo memorizar IBAN", zap.String("cuenta", cuentaID), zap.Error(err))
		}
	}

	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "importacion", EntidadID: &importacionID,
		Accion: "CONFIRMAR_IMPORTACION", UsuarioID: &usuarioID,
		ValorNuevo: map[string]int{"insertados": inserted},
	})
	return inserted, nil
}

// clasificar arma el resumen y marca cada línea (nuevo / duplicado real / reimportación).
func (s *Service) clasificar(ctx context.Context, empresaID, cuentaID, moneda string, res ParseResult) (Resumen, []MovimientoPreview, error) {
	baseCount := make(map[string]int, len(res.Movimientos))
	keys := naturalKeys(cuentaID, res)
	for _, m := range res.Movimientos {
		baseCount[baseTuple(m)]++
	}
	existing, err := s.repo.NaturalKeysExistentes(ctx, empresaID, keys)
	if err != nil {
		return Resumen{}, nil, err
	}

	out := make([]MovimientoPreview, 0, len(res.Movimientos))
	var resumen Resumen
	for i, m := range res.Movimientos {
		nk := keys[i]
		problemas := problemasLinea(m)
		estado := DupNuevo
		switch {
		case existing[nk]:
			estado = DupReimport
		case baseCount[baseTuple(m)] > 1:
			estado = DupReal
		}
		// Las inválidas (§19) no cuentan en los buckets de inserción: van a Invalidas
		// para que la banda de resumen cuadre con lo que Confirmar realmente inserta.
		switch {
		case len(problemas) > 0:
			resumen.Invalidas++
		case estado == DupReimport:
			resumen.Reimportacion++
		case estado == DupReal:
			resumen.DuplicadosReales++
		default:
			resumen.Nuevas++
		}
		out = append(out, MovimientoPreview{
			NaturalKey:       nk,
			Fecha:            m.Fecha.Format("2006-01-02"),
			Documento:        m.Documento,
			Descripcion:      m.Descripcion,
			Debito:           m.Debito.String(),
			Credito:          m.Credito.String(),
			Moneda:           moneda,
			IndiceOcurrencia: m.IndiceOcurrencia,
			EstadoDuplicado:  estado,
			Advertencias:     problemas,
		})
	}
	resumen.Leidas = len(res.Movimientos)
	return resumen, out, nil
}

// advertenciasArchivo arma los avisos a nivel de archivo (§19): líneas inválidas que
// no se importarán y la marca provisional de una cuenta USD sin TC del mes congelado.
func advertenciasArchivo(cuentaMoneda string, movs []MovimientoPreview) []string {
	w := []string{} // no-nil: al serializar debe ser [] (no null) para el contrato del frontend
	invalidas := 0
	for _, m := range movs {
		if len(m.Advertencias) > 0 {
			invalidas++
		}
	}
	if invalidas > 0 {
		w = append(w, fmt.Sprintf("%d línea(s) con problemas de integridad no se importarán (débito/crédito simultáneos o sin monto).", invalidas))
	}
	if cuentaMoneda == "USD" {
		w = append(w, "Cuenta en USD: los montos en colones quedan provisionales hasta registrar o congelar el tipo de cambio del mes.")
	}
	return w
}

// seleccionarParaInsertar filtra reimportaciones y exclusiones, y calcula montos (función pura, testeable).
func seleccionarParaInsertar(cuentaID, moneda string, res ParseResult, existing, excluir map[string]bool) []MovimientoParaInsertar {
	out := make([]MovimientoParaInsertar, 0, len(res.Movimientos))
	for _, m := range res.Movimientos {
		nk := naturalKey(cuentaID, m)
		if existing[nk] || excluir[nk] {
			continue
		}
		// Gate §19: no se persisten líneas inválidas (débito⊕crédito, sin monto).
		if len(problemasLinea(m)) > 0 {
			continue
		}
		monto := m.Debito.Add(m.Credito) // válido: exactamente uno es > 0
		montoCRC := decimal.Zero
		if moneda == "CRC" {
			montoCRC = monto // USD queda pendiente hasta el motor de TC (Fase 1)
		}
		out = append(out, MovimientoParaInsertar{
			NaturalKey:       nk,
			Fecha:            m.Fecha,
			Documento:        m.Documento,
			Descripcion:      m.Descripcion,
			Debito:           m.Debito,
			Credito:          m.Credito,
			MontoOriginal:    monto,
			MontoCRC:         montoCRC,
			IndiceOcurrencia: m.IndiceOcurrencia,
		})
	}
	return out
}

func parseArchivo(archivo []byte) (ParseResult, error) {
	g, err := CargarGrid(bytes.NewReader(archivo))
	if err != nil {
		return ParseResult{}, err
	}
	a, err := Detectar(g)
	if err != nil {
		return ParseResult{}, err
	}
	return a.Parsea(g)
}

func naturalKeys(cuentaID string, res ParseResult) []string {
	keys := make([]string, len(res.Movimientos))
	for i, m := range res.Movimientos {
		keys[i] = naturalKey(cuentaID, m)
	}
	return keys
}

// naturalKey = hash(cuenta, fecha, débito, crédito, documento, indice_ocurrencia) — RN-08.
// La descripción NO entra (algunos bancos la cambian entre descargas).
func naturalKey(cuentaID string, m MovimientoParsed) string {
	base := strings.Join([]string{
		cuentaID,
		m.Fecha.Format("2006-01-02"),
		m.Debito.String(),
		m.Credito.String(),
		m.Documento,
		strconv.Itoa(m.IndiceOcurrencia),
	}, "|")
	return sha256hex([]byte(base))
}

func baseTuple(m MovimientoParsed) string {
	return strings.Join([]string{
		m.Fecha.Format("2006-01-02"), m.Debito.String(), m.Credito.String(), m.Documento,
	}, "|")
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func toSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, it := range items {
		set[it] = true
	}
	return set
}

// ibanMismatch indica un choque DEFINITIVO de IBAN: ambos presentes y distintos.
// Si el archivo no trae IBAN (p. ej. BN) o la cuenta aún no tiene IBAN memorizado,
// NO bloquea (no hay con qué contrastar). Compara normalizado (sin espacios ni guiones).
func ibanMismatch(cuentaIBAN, archivoIBAN string) bool {
	a := normIBAN(cuentaIBAN)
	b := normIBAN(archivoIBAN)
	return a != "" && b != "" && a != b
}

func normIBAN(s string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(s))
}
