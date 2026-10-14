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
	// MontoMin / MontoMax: rango en COLONES y en valor absoluto (ver condicionesMovimientos).
	// Llegan como texto y se castean en SQL: son dinero, y pasar por float64 para validarlos
	// perdería centavos justo en el filtro con el que alguien busca un movimiento exacto.
	MontoMin, MontoMax string
	Estado, Tipo, Q    string
	// Traslado: "" (todos) | "si" (solo traslados emparejados) | "no".
	Traslado string
	// Orden: "" (fecha desc) | fecha_asc | monto_desc | monto_asc.
	Orden          string
	Page, PageSize int
	// Alcance es el RECORTE por segmento: las clasificaciones que el rol del usuario puede
	// consultar (mig 0077). No se parece a `ClasificacionIDs` aunque filtre lo mismo:
	//
	//   · ClasificacionIDs lo pide el cliente y puede venir vacío = «todas».
	//   · Alcance lo pone el SERVIDOR desde el rol, y vacío significa «ninguna», nunca «todas».
	//
	// Por eso viven separados y se suman con AND: dentro de su alcance el usuario puede afinar,
	// pero no puede ensancharlo pidiendo otra clasificación. Ver `condicionesMovimientos`.
	Alcance []string
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
	// Excluidos: cuántos movimientos del filtro están marcados `incluido = false` (un duplicado
	// que alguien corrigió). Siguen contados en `Total` y siguen en la lista —esconderlos le
	// quitaría a quien corrigió la forma de verificar qué marcó—, pero NO suman en los totales.
	// Este número es lo que hace legible esa diferencia.
	Excluidos int `json:"excluidos"`
}

// MovimientoRow es una fila de la hoja de trabajo.
type MovimientoRow struct {
	ID          string `json:"id"`
	Fecha       string `json:"fecha"`
	Documento   string `json:"documento"`
	Descripcion string `json:"descripcion"`
	// ConsecutivoLargo es la referencia larga que Davivienda esconde DENTRO de la descripción
	// (la del SINPE). No es una columna de la base: se DERIVA con la misma función que usa el
	// exportador (ver ConsecutivoLargo en export.go), para que el número que el equipo cruza
	// contra su recibo sea el mismo en el .xlsx y en la pantalla.
	//
	// Viene VACÍO en todo lo que no es Davivienda —BN, BAC, BCR, Banco Popular, Promerica no
	// publican esa referencia—, así que la pantalla que lo muestre tiene que explicar el blanco
	// en vez de dejarlo parecer una falla.
	ConsecutivoLargo string  `json:"consecutivo_largo"`
	Banco            string  `json:"banco"`
	Cuenta           string  `json:"cuenta"`
	Debito           string  `json:"debito"`
	Credito          string  `json:"credito"`
	Moneda           string  `json:"moneda"`
	MontoCRC         string  `json:"monto_crc"`
	ConceptoID       *string `json:"concepto_id"`
	Concepto         string  `json:"concepto"`
	ClasificacionID  *string `json:"clasificacion_id"`
	Clasificacion    string  `json:"clasificacion"`
	Estado           string  `json:"estado_clasificacion"`
	Confianza        *string `json:"confianza"`
	EsTraslado       bool    `json:"es_traslado"`
	// Incluido = false: el movimiento está excluido (p. ej. el mismo estado de cuenta importado
	// en dos cuentas y corregido después). La fila se sigue mostrando —una fila que se evapora no
	// explica por qué bajó un total—, pero NO suma. Sin `omitempty` a propósito: `false` es
	// justamente el valor que la pantalla tiene que pintar.
	Incluido bool `json:"incluido"`
	// ReporteAbierto: no vacío = este movimiento tiene un aviso sin resolver (vacío = ninguno). Lo
	// llena solo la consulta por segmento (mig 0077); la hoja de trabajo no lo pide y no paga el costo
	// de buscarlo. Lleva el MOTIVO solo si el aviso es de quien pregunta y no es un faltante; si no,
	// TextoAvisoAbiertoDeOtro (ver AvisoAbiertoDeFila).
	ReporteAbierto string `json:"reporte_abierto,omitempty"`
	// ReporteAbiertoPropio distingue los dos casos de ReporteAbierto sin texto mágico: true = es el
	// motivo que escribió quien pregunta; false = es el texto genérico. Nil (ausente) sin aviso abierto.
	ReporteAbiertoPropio *bool `json:"reporte_abierto_propio,omitempty"`
	// AvisoResuelto: el último aviso RESUELTO del movimiento hecho por QUIEN PREGUNTA (no faltante),
	// con su respuesta y cuándo. Solo viene cuando NO hay uno abierto (el abierto es lo vigente). Lo
	// llena solo la consulta por segmento.
	AvisoResuelto *AvisoResuelto `json:"aviso_resuelto,omitempty"`
	// PagoCxP identifica la FACTURA que este movimiento pagó, cuando la conciliación por huella
	// los enlazó. El enlace vivía en `movimiento_bancario.documento_cxp_id` desde la migración 0043
	// y NINGUNA pantalla lo mostraba: en Bancos se veía un débito con un código ilegible y en CxP
	// una factura «pagada» sin decir con cuál movimiento. Mirar las dos pantallas no alcanzaba para
	// responder «¿a qué factura corresponde este pago?», que es la pregunta de todos los cierres.
	// Vacío (ausente) = este movimiento no está enlazado a ninguna factura.
	PagoCxP *PagoCxPDeMovimiento `json:"pago_cxp,omitempty"`
	// SelloExterno: este pago YA estaba aplicado en el sistema anterior (mig 0091).
	//
	// Ausente en la enorme mayoría. No es lo mismo que `PagoCxP`: aquel dice QUÉ FACTURA del ERP
	// cerró el movimiento, y esto dice «alguien ya lo resolvió afuera» — una afirmación más débil y
	// más honesta, que no inventa un enlace que nadie verificó.
	//
	// El movimiento sigue visible: decisión del Director Financiero (6-oct-2026). La plata salió
	// del banco igual, y lo que desaparece de la lista no se revisa más.
	SelloExterno *SelloVisible `json:"sello_externo,omitempty"`
}

// PagoCxPDeMovimiento es la factura que un movimiento bancario pagó, con lo mínimo para
// reconocerla sin salir de la pantalla: a quién y por cuál documento.
type PagoCxPDeMovimiento struct {
	DocumentoID string `json:"documento_id"`
	Consecutivo string `json:"consecutivo"`
	Proveedor   string `json:"proveedor"`
	// Huella es el código interno del pago («CXP-54A4281AE49A»): el mismo que viaja en la
	// descripción del banco y el que permite seguir el pago de punta a punta.
	Huella string `json:"huella"`
	// Facturas: cuántas cubre este movimiento. Es 1 en el pago de una factura suelta y más en un
	// pago AGRUPADO por proveedor (remesa), donde `Consecutivo` trae la referencia del grupo
	// («REM 0029-01») en vez del número de una factura — porque no hay una sola que nombrar.
	Facturas int `json:"facturas"`
	// Concepto y Clasificacion: el TIPO DE GASTO de lo que se pagó, tal como está clasificado en
	// CxP. Vacíos si la factura todavía no se clasificó, y eso es correcto: no hay nada que decir.
	//
	// En un pago AGRUPADO solo vienen con valor si TODAS las facturas comparten el mismo gasto.
	// Si mezclan, van vacíos: es la misma regla que usa el cierre de remesa, y por la misma razón
	// —elegir uno de varios atribuiría la plata al rubro equivocado—.
	Concepto      string `json:"concepto"`
	Clasificacion string `json:"clasificacion"`
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
	// VisibleCxP: si Contabilidad la ve en el selector de gasto (mig 0080).
	//
	// El corte fino dentro de un concepto que sí es de CxP. Antes la visibilidad vivía solo en el
	// concepto y la clasificación la heredaba: con 4 conceptos visibles quedaban 124
	// clasificaciones expuestas, y entre ellas gasto confidencial.
	VisibleCxP bool `json:"visible_cxp"`
	// VisibleConcepto: si el CONCEPTO padre es visible para CxP. Va en la respuesta para que la
	// pantalla pueda explicar por qué una clasificación tildada igual no aparece en CxP —el
	// concepto la está tapando— en vez de dejar al usuario peleando con un interruptor que no
	// tiene efecto.
	VisibleConcepto bool `json:"visible_concepto"`
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
	lista, err := s.repo.ListarMovimientos(ctx, empresaID, f)
	if err != nil {
		return lista, err
	}
	if err := s.anotarSelloExterno(ctx, empresaID, lista.Items); err != nil {
		return ListaMovimientos{}, err
	}
	return lista, nil
}

// anotarSelloExterno marca los movimientos que el sistema ANTERIOR ya tenía aplicados.
//
// Se anota DESPUÉS y no con un JOIN en el listado: la enorme mayoría de los movimientos no tiene
// sello, y una consulta aparte por los ids de la página cuesta menos que colgarle otra tabla a la
// consulta más caliente del módulo. Es el mismo criterio que ya se usa con los avisos del segmento.
func (s *Service) anotarSelloExterno(ctx context.Context, empresaID string, items []MovimientoRow) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, m := range items {
		ids = append(ids, m.ID)
	}
	sellos, err := s.repo.SellosDeMovimientos(ctx, empresaID, ids)
	if err != nil {
		return err
	}
	if len(sellos) == 0 {
		return nil
	}
	for i := range items {
		if s, ok := sellos[items[i].ID]; ok {
			sello := s
			items[i].SelloExterno = &sello
		}
	}
	return nil
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

// ConceptoEsVisibleCxP / ClasificacionEsVisibleCxP son las guardas de alcance de la puerta de
// Contabilidad al catálogo (`cxp.catalogo`). Pasan derecho al repositorio: no hay regla que aplicar,
// solo la pregunta.
func (s *Service) ConceptoEsVisibleCxP(ctx context.Context, empresaID, conceptoID string) (bool, error) {
	return s.repo.ConceptoEsVisibleCxP(ctx, empresaID, conceptoID)
}

func (s *Service) ClasificacionEsVisibleCxP(ctx context.Context, empresaID, clasificacionID string) (bool, error) {
	return s.repo.ClasificacionEsVisibleCxP(ctx, empresaID, clasificacionID)
}
