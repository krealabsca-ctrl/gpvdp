package cxp

import (
	"path/filepath"
	"strconv"
	"strings"
)

// Veredicto de cada archivo de la carpeta de comprobantes. Solo los dos primeros se adjuntan.
const (
	// ArchivoUnico: el nombre lleva el código de UNA factura pagada.
	ArchivoUnico = "UNICO"
	// ArchivoRemesa: el código es de un pago agrupado; el mismo recibo queda en todas sus facturas.
	ArchivoRemesa = "REMESA"
	// ArchivoSinCodigo: el nombre no trae ningún código reconocible.
	ArchivoSinCodigo = "SIN_CODIGO"
	// ArchivoVariosCodigos: el nombre trae más de un código distinto y no se puede decidir.
	ArchivoVariosCodigos = "VARIOS_CODIGOS"
	// ArchivoDuplicado: otro archivo de la misma carpeta apunta a la misma factura.
	ArchivoDuplicado = "DUPLICADO"
	// ArchivoNoEncontrado: el código es legible pero no hay factura ni remesa suya en esta empresa.
	ArchivoNoEncontrado = "NO_ENCONTRADO"
	// ArchivoNoPagada: la factura existe y todavía no está pagada, así que no admite comprobante.
	ArchivoNoPagada = "NO_PAGADA"
)

// separadorDeNombre dice si un carácter es de los que una persona (o Windows) mete entre el
// prefijo y el código al bautizar un archivo: «CXP-54A…», «CXP_54A…», «CXP 54A…», «CXP.54A…».
//
// Es más ancho que lo que acepta el lector del estado de cuenta, y a propósito: ahí el texto lo
// escribe el banco, acá lo escribe una persona con el teclado y el explorador de archivos de por
// medio. Exigir el guion en este lado sería repetir el defecto que costó no enganchar un solo pago
// en toda la vida del sistema, nada más que en la otra punta del proceso.
func separadorDeNombre(b byte) bool {
	return b == '-' || b == '_' || b == '.' || b == ' '
}

func alfanumerico(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// codigoEn intenta leer un código de CxP que empieza en la posición p de s (donde s[p:p+3] == "CXP").
// Devuelve la forma canónica, la posición donde termina y si se pudo.
//
// LO QUE SEPARA UNA REMESA DE UNA FACTURA ACÁ ES EL BORDE DERECHO, no el orden. Después de los 12
// caracteres no puede venir otro alfanumérico, y esa sola regla alcanza: una remesa
// («CXPR7K2M9P4XQ1BT») leída como factura daría «CXP» + «R7K2M9P4XQ1B» y dejaría una «T» colgando,
// así que se rechaza sola. Está medido: invertir el orden de abajo no cambia ni un caso de la
// tabla de pruebas.
//
// La remesa se prueba primero de todos modos, por coherencia con `extraerHuella` —que lee el
// estado de cuenta, donde el texto sigue después del código y el borde NO se puede exigir— y para
// que las dos puntas del proceso se lean igual. Acá es redundante; allá es obligatorio.
//
// El borde, en cambio, no es negociable: sin él «CXP54A4281AE49A99» recortaría los primeros doce y
// devolvería el código de OTRA factura. Un recibo pegado a la factura equivocada es peor que un
// recibo sin pegar, porque se ve resuelto.
func codigoEn(s string, p int) (string, int, bool) {
	leer := func(desde int) (string, int, bool) {
		i := desde
		if i < len(s) && separadorDeNombre(s[i]) {
			i++
		}
		if i+12 > len(s) {
			return "", 0, false
		}
		for k := i; k < i+12; k++ {
			if !alfanumerico(s[k]) {
				return "", 0, false
			}
		}
		fin := i + 12
		if fin < len(s) && alfanumerico(s[fin]) {
			return "", 0, false
		}
		return s[i:fin], fin, true
	}
	if strings.HasPrefix(s[p:], "CXPR") {
		if cod, fin, ok := leer(p + 4); ok {
			return "CXPR-" + cod, fin, true
		}
	}
	if cod, fin, ok := leer(p + 3); ok {
		return "CXP-" + cod, fin, true
	}
	return "", 0, false
}

// CodigosDelNombre saca del nombre de un archivo los códigos de pago que lleva, en forma canónica
// y sin repetir.
//
// La extensión se descarta antes de mirar: un «.pdf» pegado al código no debe estorbar, y tampoco
// debe contarse como parte de él.
//
// Devuelve más de uno cuando el nombre menciona dos pagos distintos. Eso NO es un dato de adorno:
// es la señal de que el archivo no se puede repartir solo, y quien lo reciba tiene que negarse a
// adjuntarlo en vez de elegir el primero.
func CodigosDelNombre(nombre string) []string {
	base := strings.TrimSpace(nombre)
	if ext := filepath.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}
	s := strings.ToUpper(base)
	out := make([]string, 0, 2)
	vistos := make(map[string]bool, 2)
	for i := 0; i+3 <= len(s); {
		j := strings.Index(s[i:], "CXP")
		if j < 0 {
			break
		}
		p := i + j
		if cod, fin, ok := codigoEn(s, p); ok {
			if !vistos[cod] {
				vistos[cod] = true
				out = append(out, cod)
			}
			i = fin
			continue
		}
		i = p + 3
	}
	return out
}

// DestinoComprobante es una factura a la que va a quedar pegado un archivo.
type DestinoComprobante struct {
	DocumentoID string `json:"documento_id"`
	Consecutivo string `json:"consecutivo"`
	Proveedor   string `json:"proveedor"`
	Total       string `json:"total"`
	Moneda      string `json:"moneda"`
	// YaTiene: esa factura ya tenía un comprobante adjunto y este lo reemplaza. No impide la carga
	// —volver a subir el archivo correcto es justamente cómo se corrige un error— pero se cuenta
	// aparte, porque reemplazar borra la marca de «enviado al proveedor».
	YaTiene bool `json:"ya_tiene"`
}

// HuellaDestino es lo que la base sabe de un código: a qué facturas lleva.
type HuellaDestino struct {
	EsRemesa bool
	// Pagadas son las facturas que admiten comprobante (PAGADO o CONCILIADO).
	Pagadas []DestinoComprobante
	// SinPagar es cuántas facturas lleva ese código que todavía no están pagadas. Permite
	// distinguir «ese código no existe» de «ese pago todavía no se marcó como pagado», que para
	// quien está viendo la pantalla son dos problemas con dos soluciones distintas.
	SinPagar int
}

// ArchivoCruzado es el veredicto de UN archivo de la carpeta.
type ArchivoCruzado struct {
	Nombre   string               `json:"nombre"`
	Estado   string               `json:"estado"`
	Codigo   string               `json:"codigo,omitempty"`
	Motivo   string               `json:"motivo,omitempty"`
	Destinos []DestinoComprobante `json:"destinos"`
}

// SeAdjunta dice si este archivo va a quedar pegado a alguna factura.
func (a ArchivoCruzado) SeAdjunta() bool {
	return a.Estado == ArchivoUnico || a.Estado == ArchivoRemesa
}

// CruzarComprobantes decide, para cada nombre de archivo, a qué factura corresponde.
//
// Es una función PURA: recibe los nombres y lo que la base contestó sobre los códigos que
// aparecen, y no toca nada. Así el criterio —que es lo delicado— se puede probar entero sin
// levantar una base, y la parte que escribe no tiene decisiones que tomar.
//
// EL ORDEN DE LAS NEGATIVAS IMPORTA. Primero las que son culpa del nombre (sin código, varios
// códigos), después las que son culpa de la cartera (no existe, no está pagada) y al final el
// duplicado, que solo se puede ver mirando TODA la carpeta junta. Un archivo que ya fue descartado
// por su nombre no se vuelve a juzgar: diría dos cosas distintas del mismo problema.
func CruzarComprobantes(nombres []string, idx map[string]HuellaDestino) []ArchivoCruzado {
	out := make([]ArchivoCruzado, 0, len(nombres))
	for _, n := range nombres {
		a := ArchivoCruzado{Nombre: n, Destinos: []DestinoComprobante{}}
		cods := CodigosDelNombre(n)
		switch {
		case len(cods) == 0:
			a.Estado = ArchivoSinCodigo
			a.Motivo = "el nombre no trae ningún código de pago"
		case len(cods) > 1:
			a.Estado = ArchivoVariosCodigos
			a.Codigo = strings.Join(cods, " · ")
			a.Motivo = "el nombre menciona " + strconv.Itoa(len(cods)) + " pagos distintos"
		default:
			a.Codigo = cods[0]
			d, hay := idx[cods[0]]
			switch {
			case !hay:
				a.Estado = ArchivoNoEncontrado
				a.Motivo = "no hay ninguna factura ni remesa con ese código en esta empresa"
			case len(d.Pagadas) == 0 && d.SinPagar > 0:
				a.Estado = ArchivoNoPagada
				a.Motivo = "el pago existe pero todavía no está marcado como pagado"
			case len(d.Pagadas) == 0:
				a.Estado = ArchivoNoEncontrado
				a.Motivo = "no hay ninguna factura ni remesa con ese código en esta empresa"
			default:
				a.Estado = ArchivoUnico
				if d.EsRemesa {
					a.Estado = ArchivoRemesa
				}
				a.Destinos = d.Pagadas
			}
		}
		out = append(out, a)
	}
	marcarDuplicados(out)
	return out
}

// marcarDuplicados anula los archivos que se pelean la misma factura.
//
// Pasa solo: el explorador de Windows bautiza la segunda descarga «… (1).pdf», y ese paréntesis no
// cambia el código que lleva adentro. Se anulan TODOS los que compiten, no se elige uno: los dos
// archivos tienen el mismo derecho a ser el bueno y no hay dato que lo decida. Quedarse con el
// primero sería jugárselo a la suerte, y una vez adjunto nadie vuelve a revisarlo.
func marcarDuplicados(as []ArchivoCruzado) {
	cuenta := make(map[string]int, len(as))
	for _, a := range as {
		if !a.SeAdjunta() {
			continue
		}
		for _, d := range a.Destinos {
			cuenta[d.DocumentoID]++
		}
	}
	for i := range as {
		if !as[i].SeAdjunta() {
			continue
		}
		for _, d := range as[i].Destinos {
			if cuenta[d.DocumentoID] > 1 {
				as[i].Estado = ArchivoDuplicado
				as[i].Motivo = "hay más de un archivo para este mismo pago"
				break
			}
		}
	}
}

// ItemCarpeta es un archivo ya decidido, listo para escribirse: el contenido y las facturas a las
// que va. Son varias cuando el pago fue agrupado.
type ItemCarpeta struct {
	Nombre      string
	Mime        string
	Contenido   []byte
	DocumentoID []string
}

// ResultadoCarpeta es lo que quedó escrito de verdad.
//
// `Rechazados` existe porque entre previsualizar y confirmar pasan minutos: una factura pudo
// cambiar de estado, y el INSERT la deja afuera. Callarlo mostraría «87 adjuntados» cuando fueron
// 85, y nadie volvería a mirar las dos que faltan.
type ResultadoCarpeta struct {
	Archivos   int      `json:"archivos"`
	Facturas   int      `json:"facturas"`
	Rechazados []string `json:"rechazados"`
}

// PagoConRecibo es una TRANSFERENCIA del banco y si ya tiene su comprobante guardado. Una fila por
// transferencia —la remesa cuenta una sola vez— porque es lo que corresponde uno a uno con un
// archivo bajado del banco.
type PagoConRecibo struct {
	Codigo     string `json:"codigo"`
	Referencia string `json:"referencia"`
	EsRemesa   bool   `json:"es_remesa"`
	Proveedor  string `json:"proveedor"`
	LoteID     string `json:"lote_id"`
	LoteNumero int64  `json:"lote_numero"`
	Facturas   int    `json:"facturas"`
	MontoCRC   string `json:"monto_crc"`
	// ConRecibo es cuántas de sus facturas ya tienen comprobante. Igual a Facturas = completa;
	// 0 = falta entera; en medio = alguien la adjuntó a mano a una sola y el pago quedó partido.
	ConRecibo      int    `json:"con_recibo"`
	NombreSugerido string `json:"nombre_sugerido"`
}

// NombreSugerido arma cómo conviene bautizar el archivo de ese pago.
//
// Es una SUGERENCIA, no un requisito, y la pantalla tiene que decirlo: lo único que el lector
// necesita es que el código aparezca en alguna parte del nombre. Lo que va delante es para que una
// persona reconozca el archivo sin abrirlo.
func NombreSugerido(referencia, huella string) string {
	r := strings.TrimSpace(referencia)
	if r == "" {
		return huella + ".pdf"
	}
	return r + " " + huella + ".pdf"
}

// ResumenCarpeta es el encabezado de la previsualización: lo que se contesta antes de confirmar.
type ResumenCarpeta struct {
	Archivos   int `json:"archivos"`
	SeAdjuntan int `json:"se_adjuntan"`
	Facturas   int `json:"facturas"`
	Reemplazan int `json:"reemplazan"`
	Problemas  int `json:"problemas"`
}

// ResumirCarpeta cuenta el cruce. `Facturas` no es igual a `SeAdjuntan`: un recibo de remesa cubre
// varias, y es el número de facturas —no el de archivos— el que se compara contra lo que falta.
func ResumirCarpeta(as []ArchivoCruzado) ResumenCarpeta {
	var r ResumenCarpeta
	r.Archivos = len(as)
	for _, a := range as {
		if !a.SeAdjunta() {
			r.Problemas++
			continue
		}
		r.SeAdjuntan++
		r.Facturas += len(a.Destinos)
		for _, d := range a.Destinos {
			if d.YaTiene {
				r.Reemplazan++
			}
		}
	}
	return r
}
