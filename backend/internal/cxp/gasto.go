package cxp

// Análisis de gasto por ítem: qué compramos, a quién y a qué precio.
//
// LA IDEA, QUE NO ES NUEVA. Es lo que en las empresas grandes se llama *spend analysis*: tomar el
// detalle de las facturas, agruparlo por una taxonomía común y mirar el mismo ítem a lo largo del
// tiempo y entre proveedores. En Costa Rica la taxonomía ya existe y es obligatoria: el CABYS que
// el emisor declara ante Hacienda en cada línea. No hay que inventar un catálogo interno ni
// clasificar a mano 4.533 facturas — ya vienen clasificadas por quien las emitió.
//
// LAS TRES PREGUNTAS QUE RESPONDE, EN ORDEN DE VALOR:
//
//  1. ¿En qué se nos va la plata? La lista de ítems ordenada por gasto. Casi siempre el 20 % de
//     los ítems explica el 80 % del monto, y es ahí donde vale la pena negociar.
//  2. ¿El mismo ítem nos cuesta siempre lo mismo? El precio unitario mínimo, el máximo y el
//     último. Una diferencia grande entre el mínimo y el máximo del MISMO código es la señal más
//     barata que existe de que hay algo que renegociar.
//  3. ¿A cuántos proveedores le compramos lo mismo? Comprar un ítem a seis proveedores distintos
//     es poder de negociación que se está dejando sobre la mesa.
//
// LO QUE ESTA PANTALLA NO HACE, A PROPÓSITO:
//
//   - No traduce el CABYS a un nombre. No hay catálogo oficial cargado en el repo, y una etiqueta
//     inventada se cree; un código crudo se puede ir a buscar. Lo que se muestra como nombre es la
//     DESCRIPCIÓN MÁS FRECUENTE que los propios proveedores le pusieron a ese código.
//   - No compara precios entre unidades de medida distintas. Si un CABYS se compró por kilo y por
//     unidad, el "precio mínimo" sería un número sin sentido: se dice cuántas unidades hay y no se
//     compara.
//   - No recomienda cambiar de proveedor. Muestra el precio; la decisión tiene contexto que el
//     sistema no ve (plazo, calidad, servicio).

// FiltrosGasto acota el análisis.
type FiltrosGasto struct {
	Desde       string
	Hasta       string
	ProveedorID string
	Texto       string
	Limite      int
}

// ItemGasto es una fila de la lista: un CABYS con todo lo que se le compró.
type ItemGasto struct {
	CABYS string `json:"cabys"`
	// Descripcion es la que más veces usaron los proveedores para este código, no una etiqueta
	// del sistema. Si hay varias, `Descripciones` dice cuántas.
	Descripcion   string `json:"descripcion"`
	Descripciones int    `json:"descripciones"`

	GastoCRC string `json:"gasto_crc"`

	Lineas      int `json:"lineas"`
	Facturas    int `json:"facturas"`
	Proveedores int `json:"proveedores"`

	// Unidad es la unidad de medida; `Unidades` cuántas distintas aparecieron. Con más de una,
	// los precios de abajo vienen vacíos: comparar el precio del kilo con el de la unidad daría
	// una "variación" enorme que no significa nada.
	Unidad   string `json:"unidad"`
	Unidades int    `json:"unidades"`

	Cantidad string `json:"cantidad"`

	PrecioMin    string `json:"precio_min"`
	PrecioMax    string `json:"precio_max"`
	PrecioUltimo string `json:"precio_ultimo"`
	// VariacionPct es (máx − mín) / mín × 100. Vacío cuando no se puede comparar.
	VariacionPct string `json:"variacion_pct"`

	UltimaCompra string `json:"ultima_compra"`

	// SinConvertir son las líneas de facturas en moneda extranjera SIN tipo de cambio, que no
	// entraron en el monto. Se cuentan y se muestran en vez de descartarlas en silencio: un
	// total que calla lo que dejó afuera es la clase de número que se cuadra mal durante meses.
	SinConvertir int `json:"sin_convertir"`
}

// CompraDeItem es una compra concreta de un ítem, para el detalle.
type CompraDeItem struct {
	Fecha       string `json:"fecha"`
	Proveedor   string `json:"proveedor"`
	ProveedorID string `json:"proveedor_id"`
	DocumentoID string `json:"documento_id"`
	Consecutivo string `json:"consecutivo"`
	Detalle     string `json:"detalle"`
	Moneda      string `json:"moneda"`

	Cantidad string `json:"cantidad"`
	Unidad   string `json:"unidad"`
	// PrecioUnitario y Total van CONVERTIDOS a colones; vacíos si la factura era en otra moneda
	// y no traía tipo de cambio.
	PrecioUnitario string `json:"precio_unitario"`
	Total          string `json:"total"`
}

// CoberturaGasto dice sobre QUÉ PARTE del gasto se está haciendo el análisis.
//
// ES LA PIEZA MÁS IMPORTANTE DE LA PANTALLA y por eso no es una nota al pie. El detalle sale del
// XML, y una factura sin XML cargado no aporta ni una línea. Si de 4.533 facturas hay detalle de
// 300, la lista de ítems es verdadera pero PARCIAL: "gastamos ₡3 M en papel" puede ser ₡30 M.
// Sin este bloque, el número se lee como si fuera el total y nadie tiene forma de notarlo.
type CoberturaGasto struct {
	Facturas        int    `json:"facturas"`
	FacturasConDet  int    `json:"facturas_con_detalle"`
	MontoTotal      string `json:"monto_total"`
	MontoConDetalle string `json:"monto_con_detalle"`
	PorcentajeMonto string `json:"porcentaje_monto"`
	// PendientesDeRelleno: facturas que entraron por el buzón, tienen su XML guardado en la base
	// y todavía no tienen detalle. Son las que se pueden rellenar SIN subir nada.
	PendientesDeRelleno int `json:"pendientes_de_relleno"`
	LineasSinCABYS      int `json:"lineas_sin_cabys"`
	LineasSinConvert    int `json:"lineas_sin_convertir"`
}

// AnalisisGasto es la respuesta completa de la pantalla.
type AnalisisGasto struct {
	Cobertura CoberturaGasto `json:"cobertura"`
	Items     []ItemGasto    `json:"items"`
	// Total es la suma de `GastoCRC` de TODOS los ítems del filtro, no solo de los que se
	// devuelven: la lista viene recortada y un total calculado sobre lo visible sería menor que
	// la realidad sin que nada lo diga.
	Total      string `json:"total"`
	ItemsTotal int    `json:"items_total"`
}
