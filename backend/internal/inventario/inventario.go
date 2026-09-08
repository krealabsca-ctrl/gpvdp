package inventario

import (
	"context"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// Repository es el acceso a datos del inventario. Todas las operaciones reciben `empresaID` y lo
// aplican: un método que no lo filtre es un agujero de aislamiento entre empresas.
type Repository interface {
	// Catálogo
	ListarCategorias(ctx context.Context, empresaID string, incluirInactivas bool) ([]Categoria, error)
	CrearCategoria(ctx context.Context, empresaID, padreID, nombre string) (Categoria, error)
	ActualizarCategoria(ctx context.Context, empresaID, id, nombre string, activo bool) error
	ListarArticulos(ctx context.Context, empresaID string, f FiltroArticulos) ([]Articulo, error)
	ArticuloPorID(ctx context.Context, empresaID, id string) (Articulo, error)
	CrearArticulo(ctx context.Context, empresaID string, a ArticuloNuevo, usuarioID string) (string, error)
	ActualizarArticulo(ctx context.Context, empresaID, id string, a ArticuloNuevo) error
	FijarNivel(ctx context.Context, empresaID, articuloID, sedeID string, minimo, maximo int) error
	NivelesDeArticulo(ctx context.Context, empresaID, articuloID string) ([]NivelSede, error)

	// Existencias derivadas del libro de movimientos
	Existencias(ctx context.Context, empresaID string, f FiltroExistencias) ([]ExistenciaSede, error)
	ExistenciaDe(ctx context.Context, empresaID, articuloID, sedeID string) (int, error)
	ListarUnidades(ctx context.Context, empresaID string, f FiltroUnidades) ([]Unidad, error)
	UnidadPorNumero(ctx context.Context, empresaID, numero string) (Unidad, error)
	ListarMovimientos(ctx context.Context, empresaID string, f FiltroMovimientos) ([]Movimiento, error)

	// Entradas, salidas y ajustes
	RegistrarEntrada(ctx context.Context, empresaID string, e EntradaNueva, usuarioID string) (EntradaHecha, error)
	RegistrarServicio(ctx context.Context, empresaID string, s ServicioNuevo, usuarioID string) (Servicio, error)
	ListarServicios(ctx context.Context, empresaID, desde, hasta string) ([]Servicio, error)
	ServicioPorID(ctx context.Context, empresaID, id string) (Servicio, error)
	RegistrarAjuste(ctx context.Context, empresaID string, a AjusteNuevo, usuarioID string) error

	// Traslados
	CrearTraslado(ctx context.Context, empresaID string, t TrasladoNuevo, usuarioID string) (Traslado, error)
	RecibirTraslado(ctx context.Context, empresaID, id, fecha, usuarioID string) error
	ListarTraslados(ctx context.Context, empresaID, estado string) ([]Traslado, error)

	// Conteo cíclico (Fase 2)
	AbrirConteo(ctx context.Context, empresaID string, c ConteoNuevo, usuarioID string) (Conteo, error)
	ConteoPorID(ctx context.Context, empresaID, id string) (Conteo, error)
	ListarConteos(ctx context.Context, empresaID, estado, sedeID string) ([]Conteo, error)
	GuardarLineaConteo(ctx context.Context, empresaID, conteoID, lineaID string, contada int, motivo, usuarioID string) error
	CerrarConteo(ctx context.Context, empresaID, conteoID, fecha, usuarioID string) (CierreConteo, error)
	AnularConteo(ctx context.Context, empresaID, conteoID, motivo string) error
	PlanDeConteo(ctx context.Context, empresaID string) ([]PlanConteo, error)

	// Consignación (Fase 3)
	ConsignadasPendientes(ctx context.Context, empresaID string, f FiltroConsignacion) ([]ConsignadaSalida, error)
	ConsignadaPorUnidad(ctx context.Context, empresaID, unidadID string) (ConsignadaSalida, error)
	EnlazarCxPConsignacion(ctx context.Context, empresaID, unidadID, documentoID string) error
	// ReemplazarCxPConsignacion cambia la provisión por la factura real en UN solo UPDATE: sin eso
	// había un instante en que la unidad quedaba sin documento, y si el segundo paso fallaba quedaba
	// así para siempre.
	ReemplazarCxPConsignacion(ctx context.Context, empresaID, unidadID, provisionID, documentoRealID string) error
	UnidadConEsteDocumento(ctx context.Context, empresaID, documentoID, exceptoUnidadID string) (string, error)
	// FacturasCandidatas devuelve las facturas usables y el total (que puede ser mayor que el tope).
	FacturasCandidatas(ctx context.Context, empresaID, unidadID string, limite int) ([]FacturaCandidata, int, error)
	ResumenConsignacion(ctx context.Context, empresaID string) (ResumenConsignacion, error)

	// Análisis
	Reposicion(ctx context.Context, empresaID, sedeID string, semanas int) ([]SugerenciaPedido, error)
	Rotacion(ctx context.Context, empresaID, desde, hasta string) ([]RotacionArticulo, error)

	// Contexto para los avisos
	ContarSedes(ctx context.Context, empresaID string) (int, error)
	ContarServicios(ctx context.Context, empresaID, desde, hasta string) (int, error)
}

// Service es la lógica de negocio del inventario.
type Service struct {
	repo  Repository
	audit *shared.Audit
	log   *zap.Logger
	// facturador crea las cuentas por pagar de la consignación. Es opcional: si el servidor no lo
	// conecta, todo el módulo funciona y solo queda inhabilitado el botón de facturar. Ver
	// consignacion.go.
	facturador FacturadorCxP
}

// NewService construye el servicio.
func NewService(repo Repository, audit *shared.Audit, log *zap.Logger) *Service {
	return &Service{repo: repo, audit: audit, log: log}
}

// registrar deja el evento de auditoría si hay auditor (en tests puede no haberlo).
func (s *Service) registrar(ctx context.Context, e shared.Evento) {
	if s.audit == nil {
		return
	}
	s.audit.Registrar(ctx, e)
}

// ── Estructuras de entrada ──────────────────────────────────────────────────

// FiltroArticulos acota el catálogo.
type FiltroArticulos struct {
	CategoriaID      string
	ModoControl      string
	Q                string
	IncluirInactivos bool
}

// FiltroExistencias acota la pantalla de existencias.
type FiltroExistencias struct {
	SedeID      string
	CategoriaID string
	ModoControl string
	Q           string
	// SoloBajoMinimo deja únicamente lo que hay que reponer.
	SoloBajoMinimo bool
	// SoloConsignadas deja únicamente las filas donde hay mercadería del proveedor.
	SoloConsignadas bool
}

// FiltroUnidades acota las fichas de unidades.
type FiltroUnidades struct {
	ArticuloID string
	SedeID     string
	Estado     string
	Q          string
	// SoloQuietas: más de N días sin movimiento. 0 = no filtra.
	QuietasDesdeDias int
	Limite           int
	// Consignada es un puntero y no un bool porque hay TRES respuestas posibles: solo las del
	// proveedor, solo las propias, o no filtrar. Con un bool, «false» y «no me importa» serían lo
	// mismo y no habría forma de pedir solo las propias.
	Consignada *bool
}

// FiltroMovimientos acota el libro.
type FiltroMovimientos struct {
	ArticuloID string
	UnidadID   string
	SedeID     string
	Tipo       string
	Desde      string
	Hasta      string
	Limite     int
}

// ArticuloNuevo son los datos para crear o editar un artículo.
type ArticuloNuevo struct {
	Codigo          string `json:"codigo"`
	Nombre          string `json:"nombre"`
	CategoriaID     string `json:"categoria_id"`
	ModoControl     string `json:"modo_control"`
	UnidadMedida    string `json:"unidad_medida"`
	ProveedorID     string `json:"proveedor_id"`
	ClasificacionID string `json:"clasificacion_id"`
	Activo          bool   `json:"activo"`
	Nota            string `json:"nota"`
}

// NivelSede es el mínimo y máximo de un artículo en una sede.
type NivelSede struct {
	SedeID string `json:"sede_id"`
	Sede   string `json:"sede"`
	Minimo int    `json:"minimo"`
	Maximo int    `json:"maximo"`
}

// EntradaNueva es una remesa que llega.
type EntradaNueva struct {
	ArticuloID     string `json:"articulo_id"`
	SedeID         string `json:"sede_id"`
	Fecha          string `json:"fecha"`
	Cantidad       int    `json:"cantidad"`
	CostoUnitario  string `json:"costo_unitario"`
	ProveedorID    string `json:"proveedor_id"`
	DocumentoCxpID string `json:"documento_cxp_id"`
	EsConsignada   bool   `json:"es_consignada"`
	// Numeros son los números de las unidades cuando el artículo se controla por unidad. Si vienen
	// vacíos, el sistema los genera: hay funerarias que numeran con placa y otras que no, así que
	// se aceptan las dos formas en vez de imponer una.
	Numeros []string `json:"numeros"`
	Nota    string   `json:"nota"`
}

// EntradaHecha es el resultado de una entrada: dice qué se creó, para poder mostrarlo.
type EntradaHecha struct {
	MovimientoID   string   `json:"movimiento_id"`
	Cantidad       int      `json:"cantidad"`
	NumerosCreados []string `json:"numeros_creados"`
}

// ServicioNuevo es un funeral prestado y lo que consumió.
type ServicioNuevo struct {
	SedeID    string         `json:"sede_id"`
	Fecha     string         `json:"fecha"`
	ANombreDe string         `json:"a_nombre_de"`
	Nota      string         `json:"nota"`
	Consumos  []ConsumoNuevo `json:"consumos"`
}

// ConsumoNuevo es una línea de consumo. Para artículos por unidad se manda el número de la unidad;
// para los de cantidad, cuántos.
type ConsumoNuevo struct {
	ArticuloID   string `json:"articulo_id"`
	UnidadNumero string `json:"unidad_numero"`
	Cantidad     int    `json:"cantidad"`
}

// AjusteNuevo corrige una existencia o da de baja una unidad. Siempre con motivo.
type AjusteNuevo struct {
	ArticuloID   string `json:"articulo_id"`
	SedeID       string `json:"sede_id"`
	UnidadNumero string `json:"unidad_numero"`
	Fecha        string `json:"fecha"`
	// Diferencia con signo: +2 sobran dos, −3 faltan tres. Para una unidad se usa NuevoEstado.
	Diferencia  int    `json:"diferencia"`
	NuevoEstado string `json:"nuevo_estado"`
	Motivo      string `json:"motivo"`
}

// TrasladoNuevo es un envío entre sedes.
type TrasladoNuevo struct {
	SedeOrigenID  string               `json:"sede_origen_id"`
	SedeDestinoID string               `json:"sede_destino_id"`
	Fecha         string               `json:"fecha"`
	Nota          string               `json:"nota"`
	Lineas        []TrasladoLineaNueva `json:"lineas"`
}

// TrasladoLineaNueva es lo que va en el traslado.
type TrasladoLineaNueva struct {
	ArticuloID   string `json:"articulo_id"`
	UnidadNumero string `json:"unidad_numero"`
	Cantidad     int    `json:"cantidad"`
}
