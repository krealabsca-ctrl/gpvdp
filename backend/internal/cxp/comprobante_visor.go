package cxp

import (
	"strconv"
	"strings"
)

// ── EL VISOR DEL COMPROBANTE (14 de setiembre de 2026) ──────────────────────────────────────────
//
// EL PROBLEMA, MEDIDO. Hasta hoy, la única forma de mirar una factura recibida era descargar el XML
// crudo. Y no había alternativa: `documento_cxp.descripcion` está cortada a 45 caracteres en 4.526
// de 4.542 filas (99,6 %) y no existe ninguna tabla de líneas. O sea que del 99,6 % de las facturas
// del sistema NO SE PUEDE SABER QUÉ SE COMPRÓ.
//
// LOS VARIOS IVAS NO SON HIPOTÉTICOS. 414 de 3.455 facturas con IVA (12,0 %) tienen una tasa
// efectiva que no es ninguna tarifa legal de Costa Rica: 12,9 %, 11,2 %, 9,3 %, 7,1 %… Son facturas
// con líneas a tarifas distintas, y el ERP las aplasta en un solo campo `iva`. Una factura real del
// Director lo muestra: tres líneas al 13 % y una bolsa al 1 % dan una tasa efectiva de 11,2 %.
//
// CÓMO. Se parsea `cxp_recepcion.xml_crudo` AL VUELO, con el MISMO parser que ya está en
// producción: sin migración, funcionando hacia atrás con todo lo ya recibido, y garantizando que
// lo que se ve ES el XML y no una copia que puede desincronizarse.
//
// QUÉ NO HACE. No calcula ni corrige ningún número: los muestra como vinieron. Y no toca el camino
// de ingesta: el importador sigue usando el resumen, igual que siempre.

// Modos de respuesta del visor.
const (
	// VisorComprobante: el XML es un comprobante y se puede pintar como factura.
	VisorComprobante = "COMPROBANTE"
	// VisorDesconocido: el archivo no se pudo interpretar. Queda la descarga del original.
	VisorDesconocido = "DESCONOCIDO"
	// VisorSinXML: la recepción existe pero su XML se borró a propósito, porque la factura no era
	// de esta empresa. Es un caso legítimo y hay que explicarlo, no devolverlo como error.
	VisorSinXML = "SIN_XML"
)

// etiquetaCondicionVenta traduce SOLO los códigos confirmados contra el propio código del ERP
// (`esCredito`) y sus pruebas. El resto se devuelve crudo.
//
// Una etiqueta equivocada es peor que el código crudo: el código se puede buscar, la etiqueta se
// cree. Por eso acá no hay ninguna tabla copiada de memoria.
var etiquetaCondicionVenta = map[string]string{
	"01": "Contado",
	"02": "Crédito",
	"03": "Consignación",
	"10": "Crédito con IVA a 90 días",
}

// ImpuestoVisor es un impuesto, de una línea o del desglose del comprobante.
type ImpuestoVisor struct {
	Codigo string `json:"codigo"`
	// Tarifa es el PORCENTAJE y sale del propio XML (<Tarifa>13</Tarifa>). No se deduce del código.
	Tarifa string `json:"tarifa"`
	// CodigoTarifa va CRUDO: no hay catálogo oficial en el repo para traducirlo.
	CodigoTarifa string `json:"codigo_tarifa"`
	Monto        string `json:"monto"`
}

// DescuentoVisor es un descuento de una línea.
type DescuentoVisor struct {
	Monto string `json:"monto"`
	// Codigo crudo, y Naturaleza tal como la escribió el emisor en el XML.
	Codigo     string `json:"codigo"`
	Naturaleza string `json:"naturaleza"`
}

// LineaVisor es una línea del detalle: lo que se compró.
type LineaVisor struct {
	Numero          string           `json:"numero"`
	Detalle         string           `json:"detalle"`
	CABYS           string           `json:"cabys"`
	CodigoComercial string           `json:"codigo_comercial"`
	Cantidad        string           `json:"cantidad"`
	UnidadMedida    string           `json:"unidad_medida"`
	PrecioUnitario  string           `json:"precio_unitario"`
	MontoTotal      string           `json:"monto_total"`
	Descuentos      []DescuentoVisor `json:"descuentos"`
	SubTotal        string           `json:"subtotal"`
	BaseImponible   string           `json:"base_imponible"`
	Impuestos       []ImpuestoVisor  `json:"impuestos"`
	MontoTotalLinea string           `json:"monto_total_linea"`
}

// MedioPagoVisor es un medio de pago declarado (repetible: puede haber pago mixto).
type MedioPagoVisor struct {
	Tipo  string `json:"tipo"`
	Monto string `json:"monto"`
}

// ParteVisor es el emisor o el receptor.
type ParteVisor struct {
	Nombre             string `json:"nombre"`
	NombreComercial    string `json:"nombre_comercial,omitempty"`
	TipoIdentificacion string `json:"tipo_identificacion,omitempty"`
	Identificacion     string `json:"identificacion"`
}

// ComprobanteVisor es la factura lista para pintar.
//
// TODO monto es string, tal como vino del XML. Cadena vacía significa EL ELEMENTO NO VINO, y es
// distinto de "0": el frontend tiene que mirar el string y no convertirlo a número, porque un
// `toNumber` devuelve 0 para los dos casos y no se podrían distinguir en pantalla.
type ComprobanteVisor struct {
	Tipo       string `json:"tipo"`
	TipoNombre string `json:"tipo_nombre"`
	Version    string `json:"version"`

	Clave string `json:"clave"`
	// ClaveValida: 50 dígitos exactos. 16 de 4.542 claves del sistema no los tienen, así que la
	// pantalla no puede cortarla por posiciones fijas sin comprobarlo antes.
	ClaveValida  bool   `json:"clave_valida"`
	Consecutivo  string `json:"consecutivo"`
	FechaEmision string `json:"fecha_emision"`

	CondicionVenta         string `json:"condicion_venta"`
	CondicionVentaEtiqueta string `json:"condicion_venta_etiqueta"`
	PlazoCredito           string `json:"plazo_credito"`

	Emisor   ParteVisor  `json:"emisor"`
	Receptor *ParteVisor `json:"receptor"`

	Moneda     string `json:"moneda"`
	TipoCambio string `json:"tipo_cambio"`

	Lineas []LineaVisor `json:"lineas"`

	// Desglose es el resumen por tarifa: LA respuesta a los varios IVAs.
	Desglose   []ImpuestoVisor  `json:"desglose"`
	MediosPago []MedioPagoVisor `json:"medios_pago"`

	TotalVenta       string `json:"total_venta"`
	TotalDescuentos  string `json:"total_descuentos"`
	TotalVentaNeta   string `json:"total_venta_neta"`
	TotalImpuesto    string `json:"total_impuesto"`
	TotalOtrosCargos string `json:"total_otros_cargos"`
	TotalIVADevuelto string `json:"total_iva_devuelto"`
	TotalComprobante string `json:"total_comprobante"`

	TotalGravado   string `json:"total_gravado"`
	TotalExento    string `json:"total_exento"`
	TotalExonerado string `json:"total_exonerado"`

	// Descuadre es el texto de la MISMA verificación que ya usa la ingesta, reusada tal cual.
	// Vacío = los números cuadran. No se recalcula ni se corrige nada.
	Descuadre string `json:"descuadre"`
}

// RespuestaVisor es lo que devuelve el endpoint.
type RespuestaVisor struct {
	Modo string `json:"modo"`
	// Avisos dice EN PALABRAS lo que el visor no pudo leer. Nunca nil: `null` en JSON revienta un
	// `.map()` en el frontend, y acá el caso vacío es el normal.
	Avisos []string `json:"avisos"`
	// OtrosEnElArchivo: cuántos comprobantes MÁS traía el mismo archivo. Normal es 0; si no lo es,
	// el visor muestra el primero y lo dice, en vez de callar los otros.
	OtrosEnElArchivo int               `json:"otros_en_el_archivo"`
	Comprobante      *ComprobanteVisor `json:"comprobante"`
}

// VerComprobante arma la vista a partir del XML crudo de una recepción.
//
// Nunca devuelve error por contenido: un archivo ilegible es un MODO de respuesta, no un fallo. El
// usuario abre el visor justamente cuando algo salió raro, y un 500 ahí no le dice nada.
func VerComprobante(xmlCrudo []byte) RespuestaVisor {
	r := RespuestaVisor{Avisos: []string{}}

	if len(xmlCrudo) == 0 {
		r.Modo = VisorSinXML
		r.Avisos = append(r.Avisos,
			"Esta recepción ya no conserva el XML. Se elimina a propósito cuando el comprobante no era de esta empresa, para no guardar documentos ajenos.")
		return r
	}

	// Se reusa el parser de producción: resuelve ISO-8859-1, el BOM y los archivos con varios
	// comprobantes pegados. Parsear en el navegador perdería las tres cosas en silencio.
	comps, err := leerComprobantes(xmlCrudo)
	if err != nil || len(comps) == 0 {
		r.Modo = VisorDesconocido
		r.Avisos = append(r.Avisos,
			"Este archivo no se pudo interpretar como un comprobante electrónico. Se puede descargar el original para revisarlo.")
		return r
	}

	// SE DESPACHA POR EL NOMBRE DE LA RAÍZ, ANTES DE PINTAR NADA.
	//
	// El struct de lectura acepta CUALQUIER raíz —por diseño, para poder reconocer lo que llegó y
	// rechazarlo con nombre—, así que sin este filtro un acuse de Hacienda (`MensajeHacienda`), que
	// es XML perfectamente válido del mismo namespace base, se pintaría como una factura con TODOS
	// LOS MONTOS EN CERO y sin un solo error. Nadie se enteraría de que está mirando otra cosa.
	raiz := comps[0].XMLName.Local
	if nombreDeTipo[raiz] == "" {
		r.Modo = VisorDesconocido
		r.Avisos = append(r.Avisos, avisoDeRaizDesconocida(raiz))
		return r
	}

	r.Modo = VisorComprobante
	r.OtrosEnElArchivo = len(comps) - 1
	if r.OtrosEnElArchivo > 0 {
		r.Avisos = append(r.Avisos,
			plural(r.OtrosEnElArchivo,
				"El archivo traía 1 comprobante más; acá se muestra el primero.",
				"El archivo traía %d comprobantes más; acá se muestra el primero."))
	}

	c := comps[0]
	v := armarComprobante(c)
	r.Comprobante = &v
	r.Avisos = append(r.Avisos, avisosDe(c, v)...)
	return r
}

// armarComprobante traduce el XML al tipo de pantalla. No calcula: copia y recorta espacios.
func armarComprobante(c comprobanteXML) ComprobanteVisor {
	res := c.resumen()
	moneda, tc := res.moneda()
	clave := strings.TrimSpace(c.Clave)

	v := ComprobanteVisor{
		Tipo:       c.XMLName.Local,
		TipoNombre: nombreDeTipo[c.XMLName.Local],
		Version:    versionDeEsquema(c.XMLName.Space),

		Clave:        clave,
		ClaveValida:  len(clave) == 50,
		Consecutivo:  strings.TrimSpace(c.NumeroConsecutivo),
		FechaEmision: strings.TrimSpace(c.FechaEmision),

		CondicionVenta: strings.TrimSpace(c.CondicionVenta),
		PlazoCredito:   strings.TrimSpace(c.PlazoCredito),

		Emisor: ParteVisor{
			Nombre:             strings.TrimSpace(c.Emisor.Nombre),
			NombreComercial:    strings.TrimSpace(c.Emisor.NombreComercial),
			TipoIdentificacion: strings.TrimSpace(c.Emisor.Identificacion.Tipo),
			Identificacion:     strings.TrimSpace(c.Emisor.Identificacion.Numero),
		},

		Moneda:     moneda,
		TipoCambio: tc,

		Lineas:     []LineaVisor{},
		Desglose:   []ImpuestoVisor{},
		MediosPago: []MedioPagoVisor{},

		TotalVenta:       strings.TrimSpace(res.TotalVenta),
		TotalDescuentos:  strings.TrimSpace(res.TotalDescuentos),
		TotalVentaNeta:   strings.TrimSpace(res.TotalVentaNeta),
		TotalImpuesto:    strings.TrimSpace(res.TotalImpuesto),
		TotalOtrosCargos: strings.TrimSpace(res.TotalOtrosCargos),
		TotalIVADevuelto: strings.TrimSpace(res.TotalIVADevuelto),
		TotalComprobante: strings.TrimSpace(res.TotalComprobante),

		TotalGravado:   strings.TrimSpace(res.TotalGravado),
		TotalExento:    strings.TrimSpace(res.TotalExento),
		TotalExonerado: strings.TrimSpace(res.TotalExonerado),

		Descuadre: descuadreDe(res),
	}
	v.CondicionVentaEtiqueta = etiquetaCondicionVenta[v.CondicionVenta]

	// El receptor es puntero: un tiquete electrónico normalmente no lo trae, y un bloque vacío sin
	// explicación se lee como un error del sistema.
	if n := strings.TrimSpace(c.Receptor.Nombre); n != "" || strings.TrimSpace(c.Receptor.Identificacion.Numero) != "" {
		v.Receptor = &ParteVisor{
			Nombre:         n,
			Identificacion: strings.TrimSpace(c.Receptor.Identificacion.Numero),
		}
	}

	for _, l := range c.Lineas {
		lv := LineaVisor{
			Numero:          strings.TrimSpace(l.NumeroLinea),
			Detalle:         strings.TrimSpace(l.Detalle),
			CABYS:           strings.TrimSpace(l.CodigoCABYS),
			CodigoComercial: strings.TrimSpace(l.CodigoComercial.Codigo),
			Cantidad:        strings.TrimSpace(l.Cantidad),
			UnidadMedida:    strings.TrimSpace(l.UnidadMedida),
			PrecioUnitario:  strings.TrimSpace(l.PrecioUnitario),
			MontoTotal:      strings.TrimSpace(l.MontoTotal),
			SubTotal:        strings.TrimSpace(l.SubTotal),
			BaseImponible:   strings.TrimSpace(l.BaseImponible),
			MontoTotalLinea: strings.TrimSpace(l.MontoTotalLinea),
			Descuentos:      []DescuentoVisor{},
			Impuestos:       []ImpuestoVisor{},
		}
		for _, d := range l.Descuentos {
			lv.Descuentos = append(lv.Descuentos, DescuentoVisor{
				Monto:      strings.TrimSpace(d.MontoDescuento),
				Codigo:     strings.TrimSpace(d.CodigoDescuento),
				Naturaleza: strings.TrimSpace(d.NaturalezaDescuento),
			})
		}
		for _, i := range l.Impuestos {
			lv.Impuestos = append(lv.Impuestos, ImpuestoVisor{
				Codigo:       strings.TrimSpace(i.Codigo),
				Tarifa:       strings.TrimSpace(i.Tarifa),
				CodigoTarifa: i.codigoTarifa(),
				Monto:        strings.TrimSpace(i.Monto),
			})
		}
		v.Lineas = append(v.Lineas, lv)
	}

	// El desglose no trae la tarifa: se toma de las líneas que comparten el mismo código, que es de
	// donde el propio documento la declara. Si ninguna línea lo dice, queda vacía y la pantalla
	// muestra un guion — nunca un porcentaje deducido del código.
	tarifaPorCodigo := map[string]string{}
	for _, l := range c.Lineas {
		for _, i := range l.Impuestos {
			if t := strings.TrimSpace(i.Tarifa); t != "" {
				tarifaPorCodigo[i.codigoTarifa()] = t
			}
		}
	}
	for _, d := range res.Desglose {
		cod := d.codigoTarifa()
		v.Desglose = append(v.Desglose, ImpuestoVisor{
			Codigo:       strings.TrimSpace(d.Codigo),
			Tarifa:       tarifaPorCodigo[cod],
			CodigoTarifa: cod,
			Monto:        strings.TrimSpace(d.TotalMontoImpuesto),
		})
	}

	for _, m := range res.MediosPago {
		v.MediosPago = append(v.MediosPago, MedioPagoVisor{
			Tipo:  strings.TrimSpace(m.TipoMedioPago),
			Monto: strings.TrimSpace(m.TotalMedioPago),
		})
	}

	return v
}

// avisosDe dice en palabras lo que el visor NO pudo mostrar.
//
// Callar lo desconocido es peor que mostrarlo: quien mira cree que vio la factura entera.
func avisosDe(c comprobanteXML, v ComprobanteVisor) []string {
	var out []string

	if v.Descuadre != "" {
		out = append(out, "Los números de este comprobante no cuadran entre sí: "+v.Descuadre+
			". No se corrigió nada; se muestran tal como vinieron.")
	}
	if len(v.Lineas) == 0 {
		out = append(out, "Este comprobante no trae detalle de líneas, así que no se puede ver qué se compró.")
	}
	if v.Moneda == "" {
		out = append(out, "El comprobante no declara la moneda.")
	}
	if e := strings.TrimSpace(v.TotalExonerado); e != "" && e != "0" && !soloCeros(e) {
		out = append(out, "Esta factura tiene monto exonerado. El detalle del documento de exoneración todavía no se lee.")
	}
	if c.XMLName.Local == TipoNotaCreditoElectronica || c.XMLName.Local == TipoNotaDebitoElectronica {
		out = append(out, "Es una nota de "+
			map[string]string{TipoNotaCreditoElectronica: "crédito", TipoNotaDebitoElectronica: "débito"}[c.XMLName.Local]+
			". Todavía no se lee a qué factura hace referencia.")
	}
	// Un impuesto sin tarifa deja una fila del desglose con guion: vale decir por qué.
	for _, d := range v.Desglose {
		if d.Tarifa == "" {
			out = append(out, "Hay impuestos cuyo porcentaje el comprobante no declara; se muestra solo el código y el monto.")
			break
		}
	}
	return out
}

// avisoDeRaizDesconocida explica, con el nombre del elemento a la vista, por qué no se pinta.
//
// Los acuses se nombran aparte porque son el caso FRECUENTE y tienen explicación operativa: el
// correo del proveedor suele traer la factura y su acuse, y si el conector guardó el acuse como si
// fuera la factura, esto es lo que lo delata.
func avisoDeRaizDesconocida(raiz string) string {
	switch raiz {
	case "MensajeHacienda":
		return "Este archivo es el ACUSE DE HACIENDA, no la factura. Es la respuesta de Hacienda al comprobante; la factura viene en otro archivo del mismo correo."
	case "MensajeReceptor":
		return "Este archivo es un MENSAJE DEL RECEPTOR (la aceptación o rechazo de una factura), no una factura."
	case "":
		return "Este archivo no se pudo interpretar como un comprobante electrónico. Se puede descargar el original para revisarlo."
	}
	return "Este archivo es un «" + raiz + "», que no es un comprobante electrónico de los que el sistema sabe leer. Se puede descargar el original para revisarlo."
}

// soloCeros dice si un monto es cero en cualquiera de sus escrituras ("0", "0.00", "0.00000").
func soloCeros(s string) bool {
	for _, r := range s {
		if r != '0' && r != '.' && r != ',' && r != ' ' {
			return false
		}
	}
	return true
}

// plural elige el texto según la cantidad, y le pone el número si hace falta.
func plural(n int, uno, varios string) string {
	if n == 1 {
		return uno
	}
	return strings.Replace(varios, "%d", strconv.Itoa(n), 1)
}
