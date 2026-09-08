package inventario

// Tipos de dominio que viajan al cliente. Los montos son strings decimales (nunca float64) y las
// existencias enteros: no hay medio cofre.

// Categoria es una categoría del catálogo, opcionalmente hija de otra.
type Categoria struct {
	ID        string `json:"id"`
	PadreID   string `json:"padre_id"`
	Padre     string `json:"padre"`
	Nombre    string `json:"nombre"`
	Activo    bool   `json:"activo"`
	Articulos int    `json:"articulos"`
}

// Articulo es una línea del catálogo.
type Articulo struct {
	ID          string `json:"id"`
	Codigo      string `json:"codigo"`
	Nombre      string `json:"nombre"`
	CategoriaID string `json:"categoria_id"`
	Categoria   string `json:"categoria"`
	// ModoControl: UNIDAD (cada objeto es una ficha) o CANTIDAD (solo se cuenta).
	ModoControl  string `json:"modo_control"`
	UnidadMedida string `json:"unidad_medida"`
	ProveedorID  string `json:"proveedor_id"`
	Proveedor    string `json:"proveedor"`
	// ClasificacionID enlaza el artículo con la partida de gasto con la que se compra.
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	Activo          bool   `json:"activo"`
	Nota            string `json:"nota"`
}

// ExistenciaSede es cuánto hay de un artículo en una sede, con su nivel y su valor.
type ExistenciaSede struct {
	ArticuloID  string `json:"articulo_id"`
	Codigo      string `json:"codigo"`
	Articulo    string `json:"articulo"`
	Categoria   string `json:"categoria"`
	ModoControl string `json:"modo_control"`
	SedeID      string `json:"sede_id"`
	Sede        string `json:"sede"`
	Cantidad    int    `json:"cantidad"`
	// Minimo y Maximo en 0 = no se definió nivel para esta sede: entonces no hay semáforo.
	Minimo int `json:"minimo"`
	Maximo int `json:"maximo"`
	// Estado: SIN_NIVEL | EN_RANGO | CERCA_DEL_MINIMO | BAJO_MINIMO | SOBRE_MAXIMO.
	Estado string `json:"estado"`
	// ValorCRC es la cantidad por el costo. En modo UNIDAD suma el costo real de cada unidad; en
	// modo CANTIDAD usa el promedio ponderado de las entradas.
	ValorCRC string `json:"valor_crc"`
	// CostoUnitarioCRC es el costo con el que se valorizó (promedio en modo CANTIDAD).
	CostoUnitarioCRC string `json:"costo_unitario_crc"`
	// Consignadas: de las unidades presentes, cuántas son del proveedor y no capital propio.
	Consignadas int `json:"consignadas"`
	// ValorConsignadoCRC es cuánto de ValorCRC es del proveedor. Se calcula en SQL sobre el costo
	// real de cada unidad: estimarlo con el promedio de la fila daba un número equivocado cuando las
	// unidades del mismo artículo tienen costos distintos.
	ValorConsignadoCRC string `json:"valor_consignado_crc"`
}

// Estados del semáforo de existencia.
const (
	NivelSinNivel = "SIN_NIVEL"
	NivelEnRango  = "EN_RANGO"
	NivelCerca    = "CERCA_DEL_MINIMO"
	NivelBajo     = "BAJO_MINIMO"
	NivelSobre    = "SOBRE_MAXIMO"
)

// ResumenExistencias es la cabecera de la pantalla de existencias.
type ResumenExistencias struct {
	Filas []ExistenciaSede `json:"filas"`
	// Unidades y Cantidades se cuentan aparte: sumar cofres con urnas daría un número sin sentido.
	UnidadesTotales   int `json:"unidades_totales"`
	CantidadesTotales int `json:"cantidades_totales"`
	// ValorTotalCRC es TODO lo que hay físicamente en bodega, propio y consignado. No se le cambió
	// el significado al separar el capital: un número que ya se lee en pantallas y reportes no puede
	// empezar a medir otra cosa con el mismo nombre.
	ValorTotalCRC string `json:"valor_total_crc"`
	// ValorPropioCRC es la plata que la empresa tiene invertida: el total menos lo consignado. Es el
	// número que importa para decidir cuánto capital está inmovilizado.
	ValorPropioCRC string `json:"valor_propio_crc"`
	// ConsignadasCRC es lo que está en bodega pero es del proveedor.
	ConsignadasCRC string `json:"consignadas_crc"`
	// UnidadesConsignadas es cuántas de las unidades presentes son del proveedor.
	UnidadesConsignadas int `json:"unidades_consignadas"`
	BajoMinimo          int `json:"bajo_minimo"`
	SedesConStock       int `json:"sedes_con_stock"`
	// Aviso explica en una frase por qué el número puede no ser confiable (vacío = está bien).
	Aviso string `json:"aviso"`
}

// Unidad es la ficha de un objeto físico.
type Unidad struct {
	ID            string `json:"id"`
	Numero        string `json:"numero"`
	ArticuloID    string `json:"articulo_id"`
	Articulo      string `json:"articulo"`
	Categoria     string `json:"categoria"`
	SedeID        string `json:"sede_id"`
	Sede          string `json:"sede"`
	SedeDestinoID string `json:"sede_destino_id"`
	SedeDestino   string `json:"sede_destino"`
	Estado        string `json:"estado"`
	EstadoLegible string `json:"estado_legible"`
	CostoCRC      string `json:"costo_crc"`
	EsConsignada  bool   `json:"es_consignada"`
	ProveedorID   string `json:"proveedor_id"`
	Proveedor     string `json:"proveedor"`
	IngresadaEn   string `json:"ingresada_en"`
	// ServicioNumero: en qué servicio se usó (vacío si todavía no se usó).
	ServicioNumero string `json:"servicio_numero"`
	// DiasQuieta son los días desde su último movimiento: es lo que delata el capital detenido.
	DiasQuieta int `json:"dias_quieta"`
}

// Movimiento es una línea del libro de inventario.
type Movimiento struct {
	ID          string `json:"id"`
	Fecha       string `json:"fecha"`
	Tipo        string `json:"tipo"`
	TipoLegible string `json:"tipo_legible"`
	ArticuloID  string `json:"articulo_id"`
	Articulo    string `json:"articulo"`
	UnidadID    string `json:"unidad_id"`
	UnidadNum   string `json:"unidad_numero"`
	// Cantidad siempre positiva; Signo dice si sumó o restó.
	Cantidad         int    `json:"cantidad"`
	Signo            int    `json:"signo"`
	Sede             string `json:"sede"`
	SedeContra       string `json:"sede_contra"`
	CostoUnitarioCRC string `json:"costo_unitario_crc"`
	Servicio         string `json:"servicio"`
	Proveedor        string `json:"proveedor"`
	Motivo           string `json:"motivo"`
	Usuario          string `json:"usuario"`
}

// EtiquetaTipo traduce el tipo de movimiento a la frase que usa la gente.
func EtiquetaTipo(t string) string {
	switch t {
	case MovEntrada:
		return "entrada por compra"
	case MovSalida:
		return "salida por servicio"
	case MovTrasladoSalida:
		return "salió a otra sede"
	case MovTrasladoEntrada:
		return "llegó de otra sede"
	case MovAjusteMas:
		return "ajuste: sobraban"
	case MovAjusteMenos:
		return "ajuste: faltaban"
	case MovBaja:
		return "baja"
	case MovDevolucion:
		return "devolución al proveedor"
	default:
		return t
	}
}

// Servicio es un funeral prestado: el hecho que descarga el inventario.
type Servicio struct {
	ID        string `json:"id"`
	Numero    string `json:"numero"`
	SedeID    string `json:"sede_id"`
	Sede      string `json:"sede"`
	Fecha     string `json:"fecha"`
	ANombreDe string `json:"a_nombre_de"`
	Nota      string `json:"nota"`
	// CostoProductoCRC es lo que costó el producto que consumió: la base del margen del servicio.
	CostoProductoCRC string         `json:"costo_producto_crc"`
	Consumos         []ConsumoLinea `json:"consumos"`
}

// ConsumoLinea es un artículo que consumió un servicio.
type ConsumoLinea struct {
	ArticuloID string `json:"articulo_id"`
	Articulo   string `json:"articulo"`
	UnidadNum  string `json:"unidad_numero"`
	Cantidad   int    `json:"cantidad"`
	CostoCRC   string `json:"costo_crc"`
	// EsConsignada avisa, en el momento de registrar el servicio, que lo que se acaba de usar era
	// del proveedor y va a generar una cuenta por pagar. Se ve en la respuesta para que quien
	// registra el funeral lo sepa ahí mismo y no dos semanas después.
	EsConsignada bool `json:"es_consignada"`
}

// Traslado es un envío entre sedes.
type Traslado struct {
	ID            string `json:"id"`
	Numero        string `json:"numero"`
	SedeOrigenID  string `json:"sede_origen_id"`
	SedeOrigen    string `json:"sede_origen"`
	SedeDestinoID string `json:"sede_destino_id"`
	SedeDestino   string `json:"sede_destino"`
	Estado        string `json:"estado"`
	EnviadoEn     string `json:"enviado_en"`
	RecibidoEn    string `json:"recibido_en"`
	EnviadoPor    string `json:"enviado_por"`
	RecibidoPor   string `json:"recibido_por"`
	Nota          string `json:"nota"`
	// DiasEnCamino es lo que lleva sin recibirse. Es el número que delata lo extraviado.
	DiasEnCamino int             `json:"dias_en_camino"`
	Lineas       []TrasladoLinea `json:"lineas"`
}

// TrasladoLinea es lo que va en un traslado.
type TrasladoLinea struct {
	ArticuloID string `json:"articulo_id"`
	Articulo   string `json:"articulo"`
	UnidadID   string `json:"unidad_id"`
	UnidadNum  string `json:"unidad_numero"`
	Cantidad   int    `json:"cantidad"`
}

// SugerenciaPedido es una línea de la pantalla de reposición.
type SugerenciaPedido struct {
	ArticuloID  string `json:"articulo_id"`
	Codigo      string `json:"codigo"`
	Articulo    string `json:"articulo"`
	SedeID      string `json:"sede_id"`
	Sede        string `json:"sede"`
	ProveedorID string `json:"proveedor_id"`
	Proveedor   string `json:"proveedor"`
	Hay         int    `json:"hay"`
	Minimo      int    `json:"minimo"`
	Maximo      int    `json:"maximo"`
	// ConsumoSemanal es el consumo medido de las últimas semanas. -1 = todavía no hay historia.
	ConsumoSemanal string `json:"consumo_semanal"`
	Sugerido       int    `json:"sugerido"`
	// CostoUnitarioCRC es el costo promedio conocido del artículo. Viaja para que la pantalla pueda
	// explicar de dónde sale el costo estimado.
	CostoUnitarioCRC string `json:"costo_unitario_crc"`
	// DeDondeSale explica el número en una frase. Sin esto, una cantidad sugerida es una orden
	// que nadie puede discutir.
	DeDondeSale      string `json:"de_donde_sale"`
	CostoEstimadoCRC string `json:"costo_estimado_crc"`
}

// RotacionArticulo mide si un artículo se mueve o está detenido.
type RotacionArticulo struct {
	ArticuloID string `json:"articulo_id"`
	Codigo     string `json:"codigo"`
	Articulo   string `json:"articulo"`
	Categoria  string `json:"categoria"`
	EnStock    int    `json:"en_stock"`
	Salidas    int    `json:"salidas"`
	// Rotacion = salidas del período ÷ existencia actual. Vacío si no hay existencia.
	Rotacion string `json:"rotacion"`
	// DiasDeStock: a este ritmo, cuántos días alcanza lo que hay. Vacío si no hubo salidas.
	DiasDeStock string `json:"dias_de_stock"`
	// CapitalCRC es plata PROPIA detenida. Lo consignado va aparte porque no es capital de la
	// empresa: mezclarlos pondría a la cabeza de «lo que está inmovilizado» algo que nadie pagó.
	CapitalCRC           string `json:"capital_crc"`
	CapitalConsignadoCRC string `json:"capital_consignado_crc"`
	// Lectura: SANO | LENTO | DETENIDO | SIN_SALIDAS.
	Lectura string `json:"lectura"`
}

// Lecturas de rotación.
const (
	RotacionSana       = "SANO"
	RotacionLenta      = "LENTO"
	RotacionDetenida   = "DETENIDO"
	RotacionSinSalidas = "SIN_SALIDAS"
)
