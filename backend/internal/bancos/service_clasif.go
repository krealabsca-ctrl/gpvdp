package bancos

import (
	"context"

	"github.com/gpvdp/erp/internal/shared"
)

// ---- Tipos de clasificación / movimientos / catálogo ----

// FiltrosMovimientos son los filtros de la hoja de trabajo (Bandeja de clasificación).
type FiltrosMovimientos struct {
	Desde, Hasta, Periodo       string
	ConceptoID, ClasificacionID string
	// Periodos / ConceptoIDs: las versiones en PLURAL, para pedir «uno, varios o todos».
	// Decisión del negocio sobre los reportes: «puede ser periodo o periodos, ambos puede ser;
	// puede ser un concepto o varios, o todos». Vacío = sin restricción (todos).
	//
	// Conviven con las singulares en vez de reemplazarlas: la pantalla de trabajo elige de a uno
	// y no había razón para romperla. Las dos se resuelven en el MISMO WHERE, así que el reporte
	// exportado y lo que se ve en pantalla no pueden discrepar.
	//
	// ClasificacionIDs es el nivel fino: hay 168 clasificaciones vivas en Valle de Paz (y van a
	// ser más), así que el reporte se pide por concepto cuando alcanza y por clasificación
	// cuando hay que bajar al detalle. Si se piden las dos, mandan las clasificaciones dentro
	// de los conceptos elegidos (las condiciones se suman con AND, como todo lo demás).
	Periodos         []string
	ConceptoIDs      []string
	ClasificacionIDs []string
	// BancoID / CuentaID: la hoja de trabajo muestra «Banco · Cuenta» y no se podía filtrar por
	// ahí. Con 7 bancos y 15 cuentas, revisar la conciliación de UNA cuenta obligaba a leer
	// todo. El banco filtra el grupo completo; la cuenta afina dentro de él.
	BancoID, CuentaID string
	Estado, Tipo, Q   string
	// Traslado: "" (todos) | "si" (solo traslados emparejados) | "no".
	Traslado string
	// Orden: "" (fecha desc) | fecha_asc | monto_desc | monto_asc.
	Orden          string
	Page, PageSize int
}

// ordenSQL traduce el orden pedido a un ORDER BY seguro (whitelist, sin inyección).
func ordenSQL(orden string) string {
	switch orden {
	case "fecha_asc":
		return "m.fecha ASC, m.id"
	case "monto_desc":
		return "m.monto_crc DESC, m.id"
	case "monto_asc":
		return "m.monto_crc ASC, m.id"
	default: // fecha_desc
		return "m.fecha DESC, m.id"
	}
}

// Totales del encabezado (según filtro). Expresados EN COLONES: salen de `monto_crc`, porque
// `debito`/`credito` vienen en la moneda de la cuenta y sumarlos mezclaría dólares con colones.
type Totales struct {
	TotalDebitos  string `json:"total_debitos"`
	TotalCreditos string `json:"total_creditos"`
	Diferencia    string `json:"diferencia"`
	// SinTipoCambio: cuántos movimientos del filtro son en otra moneda y todavía NO tienen tipo
	// de cambio, así que entran al total como CERO. Sin este dato el total en colones se queda
	// corto sin avisar.
	SinTipoCambio int `json:"sin_tipo_cambio"`
	// MontoSinConvertir es ese monto en su moneda original (no se puede sumar al total).
	MontoSinConvertir string `json:"monto_sin_convertir"`
}

// MovimientoRow es una fila de la hoja de trabajo.
type MovimientoRow struct {
	ID              string  `json:"id"`
	Fecha           string  `json:"fecha"`
	Documento       string  `json:"documento"`
	Descripcion     string  `json:"descripcion"`
	Banco           string  `json:"banco"`
	Cuenta          string  `json:"cuenta"`
	Debito          string  `json:"debito"`
	Credito         string  `json:"credito"`
	Moneda          string  `json:"moneda"`
	MontoCRC        string  `json:"monto_crc"`
	ConceptoID      *string `json:"concepto_id"`
	Concepto        string  `json:"concepto"`
	ClasificacionID *string `json:"clasificacion_id"`
	Clasificacion   string  `json:"clasificacion"`
	Estado          string  `json:"estado_clasificacion"`
	Confianza       *string `json:"confianza"`
	EsTraslado      bool    `json:"es_traslado"`
}

// ListaMovimientos es la respuesta paginada con totales.
type ListaMovimientos struct {
	Totales  Totales         `json:"totales"`
	Items    []MovimientoRow `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

// MovParaClasificar es la vista mínima de un movimiento para el motor de clasificación.
type MovParaClasificar struct {
	ID          string
	Descripcion string
	EsDebito    bool
}

// MovClasifUpdate es una asignación de clasificación a aplicar.
type MovClasifUpdate struct {
	MovID           string
	ConceptoID      string
	ClasificacionID string
	ReglaID         string
	Confianza       int
}

// Concepto y ClasificacionItem son entradas del catálogo.
// VisibleCxP controla si contabilidad lo ve desde el clasificador de gastos de CxP
// (el catálogo bancario tiene conceptos sensibles que no son de su interés).
type Concepto struct {
	ID         string `json:"id"`
	Nombre     string `json:"nombre"`
	VisibleCxP bool   `json:"visible_cxp"`
	// Naturaleza: qué es el concepto para el EBITDA — INGRESO, GASTO o NEUTRO (no cuenta).
	// La declara el usuario; el default es NEUTRO para que un concepto nuevo no entre al número
	// sin que nadie lo haya decidido. Ver naturaleza.go.
	Naturaleza string `json:"naturaleza"`
	// NaturalezaDeclarada: false = nadie la declaró y el valor viene del default. Separa la
	// decisión del silencio; sin esto «no entra al EBITDA a propósito» y «falta decidir» son el
	// mismo dato. Ver migración 0064.
	NaturalezaDeclarada bool `json:"naturaleza_declarada"`
}

type ClasificacionItem struct {
	ID         string `json:"id"`
	ConceptoID string `json:"concepto_id"`
	Concepto   string `json:"concepto"`
	Nombre     string `json:"nombre"`
}

// NuevaRegla es la carga para crear una regla (incluye creación "por bloque").
type NuevaRegla struct {
	Nombre          string
	AplicaA         string
	ConceptoID      string
	ClasificacionID string
	Prioridad       int
	Palabras        []string
}

// ---- Métodos de servicio ----

// ListarMovimientos devuelve la hoja de trabajo filtrada.
func (s *Service) ListarMovimientos(ctx context.Context, empresaID string, f FiltrosMovimientos) (ListaMovimientos, error) {
	return s.repo.ListarMovimientos(ctx, empresaID, f)
}

// Conceptos y Clasificaciones exponen el catálogo de la empresa.
// soloCxP limita la vista a los conceptos visibles para CxP (contabilidad).
func (s *Service) Conceptos(ctx context.Context, empresaID string, soloCxP bool) ([]Concepto, error) {
	return s.repo.ListarConceptos(ctx, empresaID, soloCxP)
}

func (s *Service) Clasificaciones(ctx context.Context, empresaID string, soloCxP bool) ([]ClasificacionItem, error) {
	return s.repo.ListarClasificaciones(ctx, empresaID, soloCxP)
}

// Reglas lista las reglas de clasificación activas (ordenadas por prioridad) para
// visibilidad y gestión del motor de segmentación.
func (s *Service) Reglas(ctx context.Context, empresaID string) ([]Regla, error) {
	return s.repo.ListarReglas(ctx, empresaID)
}

// CrearConcepto agrega un concepto al catálogo de la empresa.
func (s *Service) CrearConcepto(ctx context.Context, empresaID, nombre string, visibleCxP bool, usuarioID string) (Concepto, error) {
	c, err := s.repo.CrearConcepto(ctx, empresaID, nombre, visibleCxP)
	if err != nil {
		return Concepto{}, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "concepto", EntidadID: &c.ID, Accion: "CREAR_CONCEPTO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{"nombre": nombre, "visible_cxp": visibleCxP},
	})
	return c, nil
}

// CrearClasificacion agrega una clasificación bajo un concepto de la empresa.
func (s *Service) CrearClasificacion(ctx context.Context, empresaID, conceptoID, nombre, cuentaContable, usuarioID string) (ClasificacionItem, error) {
	ci, err := s.repo.CrearClasificacion(ctx, empresaID, conceptoID, nombre, cuentaContable)
	if err != nil {
		return ClasificacionItem{}, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "clasificacion", EntidadID: &ci.ID, Accion: "CREAR_CLASIFICACION", UsuarioID: &usuarioID,
	})
	return ci, nil
}

// ReclasificarManual asigna concepto/clasificación a un movimiento (queda REVISADO).
func (s *Service) ReclasificarManual(ctx context.Context, empresaID, movID, conceptoID, clasificacionID, usuarioID string) error {
	if err := s.repo.ReclasificarMovimiento(ctx, empresaID, movID, conceptoID, clasificacionID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "movimiento_bancario", EntidadID: &movID,
		Accion: "RECLASIFICAR", UsuarioID: &usuarioID,
	})
	return nil
}

// CrearRegla crea la regla y la aplica al bloque no identificado (aprendizaje en Revisar, RN-17).
// Devuelve el id de la regla y cuántos movimientos clasificó.
func (s *Service) CrearRegla(ctx context.Context, empresaID string, r NuevaRegla, usuarioID string) (string, int, error) {
	id, err := s.repo.CrearRegla(ctx, empresaID, r)
	if err != nil {
		return "", 0, err
	}
	clasificados := 0
	if movs, err := s.repo.MovimientosNoIdentificados(ctx, empresaID); err == nil {
		regla := Regla{
			ID: id, AplicaA: r.AplicaA, ConceptoID: r.ConceptoID,
			ClasificacionID: r.ClasificacionID, Prioridad: r.Prioridad, Palabras: r.Palabras,
		}
		clasificados, _ = s.repo.AplicarClasificaciones(ctx, empresaID, aplicarReglas(movs, []Regla{regla}))
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "regla_clasificacion", EntidadID: &id,
		Accion: "CREAR_REGLA", UsuarioID: &usuarioID,
	})
	return id, clasificados, nil
}

// clasificarImportacion aplica las reglas a los movimientos NO_IDENTIFICADO de una importación.
func (s *Service) clasificarImportacion(ctx context.Context, empresaID, importacionID string) (int, error) {
	reglas, err := s.repo.ListarReglas(ctx, empresaID)
	if err != nil {
		return 0, err
	}
	reglas = soloActivas(reglas) // las pausadas no clasifican
	if len(reglas) == 0 {
		return 0, nil
	}
	movs, err := s.repo.MovimientosDeImportacion(ctx, empresaID, importacionID)
	if err != nil {
		return 0, err
	}
	updates := aplicarReglas(movs, reglas)
	if len(updates) == 0 {
		return 0, nil
	}
	return s.repo.AplicarClasificaciones(ctx, empresaID, updates)
}

// aplicarReglas corre el matcher sobre cada movimiento y arma las asignaciones.
func aplicarReglas(movs []MovParaClasificar, reglas []Regla) []MovClasifUpdate {
	var out []MovClasifUpdate
	for _, m := range movs {
		if c, ok := Clasificar(m.Descripcion, m.EsDebito, reglas); ok {
			out = append(out, MovClasifUpdate{
				MovID: m.ID, ConceptoID: c.ConceptoID, ClasificacionID: c.ClasificacionID,
				ReglaID: c.ReglaID, Confianza: c.Confianza,
			})
		}
	}
	return out
}
