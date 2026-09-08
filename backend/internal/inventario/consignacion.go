package inventario

// Consignación: tipos, reglas y el puerto hacia CxP.
//
// El hecho de negocio, tal como lo aprobó el Director Financiero: el proveedor deja el cofre en la
// funeraria y cobra CUANDO SE USA. Hasta ese momento el capital es suyo, así que la unidad está en
// la bodega —se puede vender— pero no es plata invertida por la empresa.
//
// Tres decisiones tomadas por el usuario que este archivo implementa, y que conviene tener escritas
// para que nadie las cambie por conveniencia:
//
//  1. La factura que genera el sistema es una PROVISIÓN, no el documento definitivo. Cuando entra la
//     factura electrónica real del proveedor por el importador de CxP, se concilia: se anula la
//     provisión y la unidad queda enlazada a la real. Sin eso, el día que llegue la FE del proveedor
//     habría DOS documentos por el mismo cofre y el riesgo es pagar dos veces.
//  2. El monto es el costo de la unidad TAL CUAL, sin IVA. Se le paga el número que la bodega tecleó
//     al recibirla; el IVA lo trae la factura real cuando llega.
//  3. Solo el USO factura automáticamente. Una unidad dañada, devuelta o que el conteo no encontró
//     queda en la cola con su caso a la vista, para que alguien decida. El sistema no adivina que
//     hay que pagarle al proveedor por un cofre que se rompió.

import (
	"context"
	"errors"
)

// Situaciones de la cola de consignación.
const (
	// SituacionPendiente: salió de bodega y nadie le facturó al proveedor.
	SituacionPendiente = "PENDIENTE"
	// SituacionFacturada: ya tiene su cuenta por pagar enlazada.
	SituacionFacturada = "FACTURADA"
)

// Errores de consignación.
var (
	// ErrConsignadaYaFacturada indica que la unidad ya tiene su cuenta por pagar. Es el guardarraíl
	// contra el doble pago, y salta también si dos personas facturan a la vez.
	ErrConsignadaYaFacturada = errors.New("inventario: esa unidad ya tiene una cuenta por pagar generada")
	// ErrNoEsConsignada indica que se quiso facturar al proveedor una unidad que es de la empresa.
	ErrNoEsConsignada = errors.New("inventario: esa unidad no es consignada: es capital propio y no hay nada que pagarle a nadie")
	// ErrConsignadaEnBodega indica que la unidad todavía está disponible. No se le paga al proveedor
	// por mercadería que sigue en la bodega: eso es justamente lo que la consignación evita.
	ErrConsignadaEnBodega = errors.New("inventario: esa unidad todavía está en la bodega; al proveedor se le paga cuando se usa")
	// ErrConsignadaEnTransito indica que la unidad va entre sedes. Todavía no salió del inventario:
	// mover algo de plaza no es usarlo.
	ErrConsignadaEnTransito = errors.New("inventario: esa unidad va en tránsito entre sedes; mudarla no es usarla")
	// ErrSinFacturadorCxP indica que el sistema no tiene cómo crear cuentas por pagar. Es un error de
	// configuración del servidor, no del usuario, pero se responde con un mensaje entendible.
	ErrSinFacturadorCxP = errors.New("inventario: este servidor no tiene el módulo de cuentas por pagar conectado")
	// ErrConsignadaSinFactura indica que se quiso conciliar una unidad que no tiene provisión.
	ErrConsignadaSinFactura = errors.New("inventario: esa unidad no tiene una provisión que conciliar")
	// ErrSituacionInvalida indica un valor que no es PENDIENTE ni FACTURADA.
	ErrSituacionInvalida = errors.New("inventario: la situación tiene que ser PENDIENTE o FACTURADA")
	// ErrDocumentoRealRequerido indica que falta decir contra qué factura se concilia.
	ErrDocumentoRealRequerido = errors.New("inventario: hace falta indicar la factura del proveedor contra la que se concilia")
	// ErrDocumentoRealEsLaProvision indica que se pasó la propia provisión como factura real:
	// anularla y reenlazarla dejaría la unidad apuntando a un documento anulado.
	ErrDocumentoRealEsLaProvision = errors.New("inventario: esa es la misma provisión que generó el sistema, no la factura del proveedor")
	// ErrYaConciliada indica que la unidad ya está enlazada a la factura real. Conciliar de nuevo
	// anularía el documento verdadero del proveedor, que es el peor error posible de esta pantalla.
	ErrYaConciliada = errors.New("inventario: esa unidad ya está enlazada a la factura real del proveedor; no queda ninguna provisión que conciliar")
	// ErrCostoCeroNoSeFactura indica que la unidad entró con costo 0. Una cuenta por pagar de cero no
	// le sirve a nadie: ocupa un consecutivo, saca la unidad de la cola y no representa ninguna deuda.
	// El costo se corrige con un ajuste antes de facturar.
	ErrCostoCeroNoSeFactura = errors.New("inventario: esa unidad entró con costo 0, así que no hay monto que facturarle al proveedor: corregí el costo antes de generar la cuenta por pagar")
	// ErrSalidaNoFacturableSola indica que la unidad no salió por un uso. Se puede facturar igual,
	// pero solo pidiéndolo de forma explícita: el sistema no decide por nadie si al proveedor se le
	// paga un cofre que se rompió o que el conteo no encontró.
	ErrSalidaNoFacturableSola = errors.New("inventario: esa unidad no salió por un servicio, así que hay que confirmar a mano que corresponde pagarle al proveedor")
	// ErrDevueltaNoSeFactura indica que la unidad volvió al proveedor. Ese es el caso normal de la
	// consignación —lo que no se usó se devuelve— y no genera deuda ni confirmándolo a mano.
	ErrDevueltaNoSeFactura = errors.New("inventario: esa unidad se devolvió al proveedor, así que no hay nada que pagarle")

	// ── Errores que nacen en CxP y el adaptador traduce ──────────────────────
	//
	// Existen para que el borde HTTP pueda contestar algo entendible. Sin ellos, cualquier rechazo de
	// CxP salía como 500, incluido el rechazo por duplicado en el que se apoya toda la idempotencia.

	// ErrProvisionYaExisteEnCxP indica que ya hay un documento con esa clave determinística.
	ErrProvisionYaExisteEnCxP = errors.New("inventario: ya existe una cuenta por pagar generada por esta unidad")
	// ErrDocumentoRealNoEncontrado indica que el documento no existe en esta empresa. Es lo que
	// impide enlazar una unidad a una factura de otra empresa.
	ErrDocumentoRealNoEncontrado = errors.New("inventario: esa factura no existe en esta empresa")
	// ErrDocumentoRealDeOtroProveedor indica que la factura es de un proveedor distinto del dueño de
	// la mercadería. Enlazarla haría que la unidad quedara respaldada por una deuda con un tercero.
	ErrDocumentoRealDeOtroProveedor = errors.New("inventario: esa factura es de otro proveedor, no del dueño de la mercadería")
	// ErrDocumentoRealAnulado indica que la factura elegida está anulada: enlazarla dejaría la unidad
	// apuntando a un documento sin efecto y la deuda desaparecería de la cola.
	ErrDocumentoRealAnulado = errors.New("inventario: esa factura está anulada, así que no respalda ninguna deuda")
	// ErrDocumentoRealYaEnlazado indica que otra unidad ya usa esa factura. Una factura electrónica
	// puede cubrir varios cofres, pero el enlace es uno a uno: si cubre dos, hay que decidir a cuál
	// se le atribuye y resolver el otro a mano.
	ErrDocumentoRealYaEnlazado = errors.New("inventario: esa factura ya está enlazada a otra unidad")
	// ErrCxPRechazo envuelve cualquier otro rechazo de CxP con su propio mensaje.
	ErrCxPRechazo = errors.New("inventario: cuentas por pagar rechazó la operación")
	// ErrProvisionYaNoAnulable indica que la provisión salió del rango de estados que se pueden
	// anular —típicamente porque ya se pagó—. Conciliar entonces dejaría dos deudas vivas por el
	// mismo cofre, así que se rechaza y se dice qué hacer.
	ErrProvisionYaNoAnulable = errors.New("inventario: la provisión ya no se puede anular, así que " +
		"conciliar dejaría dos deudas por el mismo cofre: resolvelo en Cuentas por pagar (nota de " +
		"crédito del proveedor, o aplicando lo pagado como anticipo contra la factura real)")
)

// FacturaCandidata es una factura del proveedor que sirve para conciliar. Lleva lo justo para que la
// persona la reconozca en un desplegable: el consecutivo, el monto y la fecha.
type FacturaCandidata struct {
	ID          string `json:"id"`
	Consecutivo string `json:"consecutivo"`
	// Clave es el respaldo cuando el consecutivo viene vacío: en esta base conviven facturas con
	// consecutivo y sin él.
	Clave    string `json:"clave"`
	TotalCRC string `json:"total_crc"`
	Fecha    string `json:"fecha"`
}

// CandidatasConciliacion es la lista que alimenta el desplegable de conciliar.
type CandidatasConciliacion struct {
	Filas []FacturaCandidata `json:"filas"`
	// Total es cuántas hay en TOTAL, aunque la lista venga cortada. Un tope que no avisa esconde
	// justamente la factura que se está buscando.
	Total int `json:"total"`
	// Aviso explica por qué la lista puede estar vacía o cortada.
	Aviso string `json:"aviso"`
}

// ColaConsignacion es lo que ve la pantalla: el resumen arriba y las unidades abajo.
type ColaConsignacion struct {
	Resumen ResumenConsignacion `json:"resumen"`
	Filas   []ConsignadaSalida  `json:"filas"`
	// PuedeFacturar dice si este servidor tiene CxP conectado. La pantalla lo necesita para no
	// ofrecer un botón que va a fallar: sin esto tendría que descubrirlo intentándolo.
	PuedeFacturar bool `json:"puede_facturar"`
}

// FiltroConsignacion acota la cola.
type FiltroConsignacion struct {
	ProveedorID string
	Estado      string
	// Situacion: PENDIENTE | FACTURADA | "" (las dos).
	Situacion string
}

// ConsignadaSalida es una unidad del proveedor que salió de la bodega, con el hecho que la sacó.
//
// Lleva el servicio y el motivo porque la pregunta que contesta esta pantalla no es «cuántas»: es
// «por qué salió esta y qué corresponde hacer». Un listado de números de unidad sin el hecho obliga
// a ir a buscarlo a otras tres pantallas.
type ConsignadaSalida struct {
	UnidadID     string `json:"unidad_id"`
	UnidadNumero string `json:"unidad_numero"`
	Articulo     string `json:"articulo"`
	Categoria    string `json:"categoria"`
	Estado       string `json:"estado"`
	// EstadoLegible es el estado en palabras: «DANADA» no es lenguaje de nadie.
	EstadoLegible string `json:"estado_legible"`
	CostoCRC      string `json:"costo_crc"`
	ProveedorID   string `json:"proveedor_id"`
	Proveedor     string `json:"proveedor"`
	Sede          string `json:"sede"`
	FechaSalida   string `json:"fecha_salida"`
	// TipoSalida es SALIDA (se usó en un servicio) o BAJA (se dañó, o el conteo no la encontró).
	TipoSalida        string `json:"tipo_salida"`
	MotivoSalida      string `json:"motivo_salida"`
	ServicioNumero    string `json:"servicio_numero"`
	ServicioANombreDe string `json:"servicio_a_nombre_de"`
	// DocumentoID vacío = todavía no se le facturó al proveedor.
	DocumentoID          string `json:"documento_id"`
	DocumentoConsecutivo string `json:"documento_consecutivo"`
	DocumentoEstado      string `json:"documento_estado"`
	DocumentoTotalCRC    string `json:"documento_total_crc"`
	// DocumentoCompraID es la factura con la que se recibió la unidad, si se registró. No es lo
	// mismo que DocumentoID y por eso son dos campos.
	DocumentoCompraID string `json:"documento_compra_id"`
	// DeudaVigente dice si todavía se le debe al proveedor por esta unidad. Lo calcula el SQL con la
	// misma expresión que el filtro y el resumen: no alcanza con «tiene enlace», porque un documento
	// ANULADO deja el enlace puesto y la deuda viva.
	DeudaVigente bool `json:"deuda_vigente"`
	// DocumentoEsProvision distingue lo que generó el sistema de la factura real del proveedor. Es
	// lo que decide si todavía queda algo por conciliar: sin este dato la pantalla ofrecía conciliar
	// una unidad ya conciliada, y el segundo intento habría anulado la factura verdadera.
	DocumentoEsProvision bool `json:"documento_es_provision"`
	// PuedeConciliarse: hay una provisión vigente esperando la factura real del proveedor.
	PuedeConciliarse bool `json:"puede_conciliarse"`
	// Situacion: PENDIENTE | FACTURADA. Derivada, no guardada.
	Situacion string `json:"situacion"`
	// PuedeFacturarse: solo el uso factura automáticamente; los demás casos los decide una persona.
	PuedeFacturarse bool `json:"puede_facturarse"`
	// Aviso explica el caso cuando no es un uso normal (se dañó, no apareció, se devolvió).
	Aviso string `json:"aviso"`
}

// ResumenConsignacion es la cabecera de la pantalla: de quién es lo que hay y qué se debe.
type ResumenConsignacion struct {
	// EnBodega es lo del proveedor que todavía está y se puede vender: NO se le debe nada por eso.
	EnBodega    int    `json:"en_bodega"`
	EnBodegaCRC string `json:"en_bodega_crc"`
	// PorFacturar es lo que se USÓ y nadie facturó: plata que se le debe al proveedor y que todavía
	// no está en ninguna cuenta por pagar. Es el número que importa.
	PorFacturar    int    `json:"por_facturar"`
	PorFacturarCRC string `json:"por_facturar_crc"`
	// ADecidir es lo que salió de la bodega por otra razón —se dañó, el conteo no lo encontró, se
	// devolvió—. Va SEPARADO de PorFacturar porque no es deuda: es un caso que alguien tiene que
	// resolver. Sumarlos decía que se le debe al proveedor un cofre que se le devolvió.
	ADecidir      int    `json:"a_decidir"`
	ADecidirCRC   string `json:"a_decidir_crc"`
	Facturadas    int    `json:"facturadas"`
	FacturadasCRC string `json:"facturadas_crc"`
	Proveedores   int    `json:"proveedores"`
	// Aviso explica por qué el número puede no ser confiable.
	Aviso string `json:"aviso"`
}

// FacturaConsignacion es lo que se le manda a CxP para crear la provisión.
type FacturaConsignacion struct {
	ProveedorID string
	Fecha       string
	TotalCRC    string
	// Clave es determinística (deriva del id de la unidad) y ahí está toda la idempotencia: si el
	// pedido se repite, CxP rechaza por duplicado en vez de crear una segunda factura. Con una clave
	// aleatoria, cada reintento fabricaría una cuenta por pagar nueva por el mismo cofre.
	Clave string
	// Consecutivo es el identificador que va a LEER una persona en la bandeja de CxP. La clave sirve
	// para la máquina —es un uuid sin guiones— y no se puede buscar de memoria; sin consecutivo el
	// documento aparecía sin nombre y no había forma de encontrarlo desde inventario.
	Consecutivo string
	Descripcion string
}

// DocumentoCxP es lo que inventario necesita saber de una cuenta por pagar ajena: de quién es y cómo
// está. No es el documento completo de CxP: solo los campos con los que se decide.
type DocumentoCxP struct {
	ID          string
	Consecutivo string
	ProveedorID string
	Estado      string
	TotalCRC    string
}

// EstadoDocAnulado es el estado que deja un documento sin efecto en CxP. Vive acá y no importado de
// cxp porque el puerto existe justamente para que inventario no dependa de ese paquete.
const EstadoDocAnulado = "ANULADO"

// provisionAnulable dice si la provisión todavía se puede dejar sin efecto en CxP.
//
// Espeja los estados que CxP acepta para anular. Está duplicado a propósito —el puerto existe para
// que inventario no importe cxp— y por eso el service NO se apoya solo en esto: la provisión nace
// bloqueada para pago (migración 0072), así que en la práctica nunca debería llegar a PAGADO. Esta
// función es el segundo candado, para que conciliar no rompa el enlace cuando la anulación va a
// fallar.
func provisionAnulable(estado string) bool {
	switch estado {
	case "RECIBIDO", "REVISADO", "VALIDADO_DEPTO", "APROBADO", "PROGRAMADO":
		return true
	}
	return false
}

// FacturadorCxP es lo único que inventario necesita saber de CxP.
//
// Es un PUERTO y no una llamada directa al paquete cxp, siguiendo el molde de bancos.ConciliadorCxP:
// el adaptador vive en main.go. Así inventario no depende de CxP para compilar ni para testearse, y
// el día que la provisión se genere de otra forma solo cambia el adaptador.
//
// **El adaptador tiene que traducir los errores de CxP a los centinelas de ESTE archivo.** Si deja
// pasar un error del paquete cxp, el switch de responder() no lo reconoce y sale como 500 «error
// interno»: el usuario no puede distinguir «esa factura ya existe» de «el servidor se cayó». Es la
// quinta vez que este proyecto se topa con la misma clase de defecto.
type FacturadorCxP interface {
	// CrearProvisionConsignacion devuelve el id y el consecutivo del documento creado.
	// Si la clave ya existe devuelve ErrProvisionYaExisteEnCxP.
	CrearProvisionConsignacion(ctx context.Context, empresaID string, f FacturaConsignacion, usuarioID string) (id, consecutivo string, err error)
	// AnularProvision la deja sin efecto en CxP. Se usa al conciliar contra la factura real.
	AnularProvision(ctx context.Context, empresaID, documentoID, motivo, usuarioID string) error
	// DocumentoPorID trae un documento de ESTA empresa. Devuelve ErrDocumentoRealNoEncontrado si no
	// existe o es de otra: es lo que impide enlazar una unidad a la factura de otra empresa.
	DocumentoPorID(ctx context.Context, empresaID, documentoID string) (DocumentoCxP, error)
	// ProvisionPorClave busca la provisión que este módulo ya creó, para poder recuperarse de un
	// duplicado en vez de quedar sin salida.
	ProvisionPorClave(ctx context.Context, empresaID, clave string) (DocumentoCxP, error)
}

// SetFacturadorCxP conecta el módulo de cuentas por pagar. Si no se llama, la cola de consignación
// funciona igual y solo queda inhabilitado el botón de facturar: la pantalla que muestra qué se le
// debe al proveedor vale por sí sola.
func (s *Service) SetFacturadorCxP(f FacturadorCxP) { s.facturador = f }

// claveDeConsignacion fabrica la clave del documento a partir de la unidad.
//
// Es lo que hace que facturar dos veces la misma unidad sea imposible incluso si el enlace en
// inv_unidad fallara: CxP tiene un UNIQUE por (empresa, clave), así que el segundo intento choca
// contra la base. El prefijo dice de dónde salió el documento, que es lo que alguien va a querer
// saber cuando lo encuentre en la bandeja.
func claveDeConsignacion(unidadID string) string {
	limpio := ""
	for _, r := range unidadID {
		if r != '-' {
			limpio += string(r)
		}
	}
	return "INVC-" + limpio
}

// avisoDeSalida explica el caso cuando la unidad no salió por un uso normal, y decide si el sistema
// puede facturarla solo.
//
// La regla la fijó el usuario: automático SOLO el uso. Los otros tres casos son decisiones —¿el
// proveedor cobra un cofre que se rompió en la bodega?— y el sistema no las toma por nadie; las
// muestra con el motivo escrito para que alguien las resuelva.
func avisoDeSalida(estado, tipoSalida string) (aviso string, puedeFacturarse bool) {
	switch estado {
	case EstadoUsada:
		if tipoSalida == MovSalida {
			return "", true
		}
		// Usada pero sin salida en el libro: algo quedó a medias y no se factura a ciegas.
		return "la unidad está marcada como usada pero no tiene la salida en el libro: revisá el movimiento antes de facturar", false
	case EstadoDanada:
		return "se dañó: si el proveedor asume el daño no hay nada que pagarle, y si no, generá la factura a mano acá", false
	case EstadoNoAparecio:
		return "el conteo no la encontró: lo más probable es que se haya usado sin registrar el servicio, y el proveedor va a cobrarla igual", false
	case EstadoDevuelta:
		return "se devolvió al proveedor: no corresponde pagarla", false
	default:
		return "", false
	}
}
