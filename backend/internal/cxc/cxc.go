package cxc

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// Errores de dominio.
var (
	ErrContratoNoEncontrado = errors.New("cxc: contrato no encontrado")
	ErrSinDesde             = errors.New("cxc: hay que indicar desde cuándo generar cargos")
	ErrRangoDemasiadoAmplio = errors.New("cxc: el rango pedido genera demasiados cargos; acotalo o confirmá el total")
	ErrSinPermisoSedes      = errors.New("cxc: no tenés permiso para ver la cartera de todas las sedes")
)

// permisoTodasSedes: quien lo tiene ve la cartera completa; sin él, el operador solo ve
// los contratos de la(s) sede(s) que le asignaron. Mismo patrón que el scoping por
// departamento de CxP: se resuelve en el SERVICIO, no en la pantalla.
const permisoTodasSedes = "cxc.ver_todas_sedes"

// permisoArreglos: el permiso del supervisor de piso. Los plazos estándar (1-3-6-9) los pacta
// cualquier gestor; todo lo demás es excepción y pasa por acá.
const permisoArreglos = "cxc.arreglos"

// TopeCargosPorCorrida es el freno del generador. Con 70 000 contratos, generar desde el
// primer cobro de los más viejos son millones de filas: el generador PREVISUALIZA y, si
// el total pasa este tope, exige que el usuario confirme explícitamente el número.
const TopeCargosPorCorrida = 250_000

// PermisoChecker resuelve si (empresa, rol) tiene un permiso. Lo implementa rbac.Service;
// se declara acá como interfaz local para no acoplar CxC al paquete rbac.
type PermisoChecker interface {
	Tiene(ctx context.Context, empresaID, rolCodigo, permiso string) (bool, error)
}

// Contrato es el EJE del módulo (decisión del Director Financiero: el cliente es dato
// del contrato, sin unicidad de cédula).
type Contrato struct {
	ID           string `json:"id"`
	Numero       string `json:"numero"`
	SedeID       string `json:"sede_id"`
	Sede         string `json:"sede"`
	Cliente      string `json:"cliente_nombre"`
	Documento    string `json:"documento"`
	Telefonos    string `json:"telefonos"`
	Correos      string `json:"correos"`
	Servicio     string `json:"servicio"`
	TipoServicio string `json:"tipo_servicio"`
	Modalidad    string `json:"modalidad"`
	FormaPago    string `json:"forma_pago"`
	Asociacion   string `json:"asociacion"`
	DiaPago      *int   `json:"dia_pago"`
	Cuota        string `json:"cuota_vigente"`
	FechaInicial string `json:"fecha_inicial"`
	PrimerCobro  string `json:"fecha_primer_cobro"`
	TarjetaVence string `json:"tarjeta_vence"`
	Estado       string `json:"estado"`
	// Cargos y saldo DERIVADOS: no se guardan. El saldo de un contrato es la suma de
	// los saldos de sus cargos abiertos, siempre calculada.
	Cargos            int    `json:"cargos_abiertos"`
	Saldo             string `json:"saldo"`
	DiasMoraMax       int    `json:"dias_mora_max"`
	Tramo             string `json:"tramo"`
	RevisionPendiente bool   `json:"revision_pendiente"`
	RevisionMotivo    string `json:"revision_motivo"`
	// Del sistema de origen, para la corrida en paralelo. Informativos.
	ScoreOrigen        *int    `json:"score_origen"`
	MorosidadOrigen    string  `json:"morosidad_origen"`
	DiasVencidosOrigen *int    `json:"dias_vencidos_origen"`
	SaldoOrigen        *string `json:"saldo_origen"`
}

// Cargo es la partida abierta: el período de un contrato con su vencimiento y su saldo.
type Cargo struct {
	ID       string `json:"id"`
	Periodo  string `json:"periodo"`
	VenceEn  string `json:"vence_en"`
	Monto    string `json:"monto"`
	Aplicado string `json:"monto_aplicado"`
	Saldo    string `json:"saldo"`
	Estado   string `json:"estado"`
	Origen   string `json:"origen"`
	DiasMora int    `json:"dias_mora"`
	Tramo    string `json:"tramo"`
	Etiqueta string `json:"tramo_etiqueta"`
}

// FiltrosContratos son los filtros de la lista de cartera. Se resuelven en el SERVIDOR:
// con 70 000 contratos, filtrar en el navegador no es una opción.
type FiltrosContratos struct {
	Q            string
	SedeID       string
	ModalidadID  string
	FormaPagoID  string
	AsociacionID string
	Estado       string
	// ConSaldo: solo los que deben algo.
	ConSaldo bool
	// EnRevision: solo los que quedaron en cuarentena al importar.
	EnRevision bool
	Orden      string
	Page       int
	PageSize   int
	// SedeIDs lo inyecta el servicio según el permiso: nil = ve todo; no-nil = solo esas.
	SedeIDs []string
}

// ListaContratos es la respuesta paginada con el resumen de LO FILTRADO (mismo patrón
// que la hoja de trabajo de Bancos: el encabezado mide lo que se está viendo).
type ListaContratos struct {
	Resumen  ResumenCartera `json:"resumen"`
	Items    []Contrato     `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// ResumenCartera son los totales del filtro activo.
type ResumenCartera struct {
	Contratos int    `json:"contratos"`
	ConSaldo  int    `json:"con_saldo"`
	Saldo     string `json:"saldo"`
	Vencido   string `json:"vencido"`
	PorVencer string `json:"por_vencer"`
	Cargos    int    `json:"cargos_abiertos"`
}

// Catalogos alimenta los selectores de la pantalla en una sola llamada.
type Catalogos struct {
	Sedes        []ItemCatalogo `json:"sedes"`
	Modalidades  []ItemCatalogo `json:"modalidades"`
	FormasPago   []ItemCatalogo `json:"formas_pago"`
	Asociaciones []ItemCatalogo `json:"asociaciones"`
	Tramos       []Tramo        `json:"tramos"`
}

type ItemCatalogo struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	// Contratos: cuántos contratos usan esta entrada (para que el selector diga algo).
	Contratos int `json:"contratos"`
}

// Tramo es un tramo de mora con su probabilidad de recuperación.
type Tramo struct {
	Codigo   string `json:"codigo"`
	Etiqueta string `json:"etiqueta"`
	DiasMin  int    `json:"dias_min"`
	DiasMax  int    `json:"dias_max"`
	Prob     string `json:"prob_recuperacion"`
}

// Service orquesta las reglas de CxC.
type Service struct {
	repo  Repository
	audit *shared.Audit
	log   *zap.Logger
	// perms es opcional: sin él no hay scoping por sede (se ve todo). En producción lo
	// inyecta main.go con el servicio de RBAC.
	perms PermisoChecker
}

// NewService construye el servicio de CxC.
func NewService(repo Repository, audit *shared.Audit, log *zap.Logger) *Service {
	return &Service{repo: repo, audit: audit, log: log}
}

// SetPermisos inyecta el verificador RBAC para el scoping por sede.
func (s *Service) SetPermisos(p PermisoChecker) { s.perms = p }

// sedesVisibles decide el alcance de datos del usuario:
//   - nil  => ve TODAS las sedes (tiene cxc.ver_todas_sedes, o no hay checker);
//   - no-nil (posiblemente vacío) => solo esas sedes.
//
// Vive en el servicio para que ninguna consulta pueda pedir una sede ajena, aunque
// alguien arme la URL a mano.
func (s *Service) sedesVisibles(ctx context.Context, empresaID, rol, usuarioID string) ([]string, error) {
	if s.perms == nil {
		return nil, nil
	}
	todas, err := s.perms.Tiene(ctx, empresaID, rol, permisoTodasSedes)
	if err != nil {
		return nil, err
	}
	if todas {
		return nil, nil
	}
	return s.repo.SedesDeUsuario(ctx, empresaID, usuarioID)
}
