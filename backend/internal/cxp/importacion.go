package cxp

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

var (
	// ErrArchivoVacio indica que el Excel no trae filas de datos.
	ErrArchivoVacio = errors.New("cxp: el archivo no tiene filas de datos")
	// ErrFormatoImportacion indica que no se reconoció el formato (falta la columna Clave).
	ErrFormatoImportacion = errors.New("cxp: formato no reconocido (falta la columna Clave)")
)

// ImportEstado clasifica una fila del preview de importación.
type ImportEstado string

const (
	ImpNuevo     ImportEstado = "NUEVO"
	ImpDuplicado ImportEstado = "DUPLICADO"
)

// FilaImportada es una factura leída del Excel de facturación.
type FilaImportada struct {
	Clave        string `json:"clave"`
	Consecutivo  string `json:"consecutivo"`
	FechaEmision string `json:"fecha_emision"`
	Proveedor    string `json:"proveedor"`
	Cedula       string `json:"cedula"`
	Moneda       string `json:"moneda"`
	Subtotal     string `json:"subtotal"`
	IVA          string `json:"iva"`
	Total        string `json:"total"`
	Condicion    string `json:"condicion"`
	Vencimiento  string `json:"vencimiento"`
	// TC: el tipo de cambio DE LA FACTURA (columna «Tipo Cambio»). Solo aplica a moneda extranjera.
	TC             string       `json:"tc"`
	Estado         ImportEstado `json:"estado"`
	ProveedorNuevo bool         `json:"proveedor_nuevo"`

	// ── Solo del camino XML (el .xlsx no trae nada de esto) ──
	//
	// TipoDocumento es la raíz del comprobante (FacturaElectronica, NotaCreditoElectronica, …).
	TipoDocumento string `json:"tipo_documento,omitempty"`
	// Receptor/ReceptorNombre: a quién se le facturó. Es el único dato del comprobante que puede
	// decir de qué empresa es la factura, y el Excel lo perdía por completo.
	Receptor       string `json:"receptor,omitempty"`
	ReceptorNombre string `json:"receptor_nombre,omitempty"`
	// Descuadre explica, en palabras, por qué la aritmética del comprobante no cierra. Vacío = cierra.
	Descuadre string `json:"descuadre,omitempty"`
}

// ResumenImportacion son los totales del preview.
type ResumenImportacion struct {
	Leidas            int `json:"leidas"`
	Nuevas            int `json:"nuevas"`
	Duplicadas        int `json:"duplicadas"`
	ProveedoresNuevos int `json:"proveedores_nuevos"`
	// ── De dónde salieron, para que un faltante se pueda EXPLICAR ──
	//
	// Sin esto, un archivo de 500 facturas del que se leen 85 se ve igual que un archivo de 85: el
	// parser descartaba las filas sin clave en silencio y nadie podía saber si el problema era el
	// archivo, la hoja equivocada o el sistema. Ahora la pantalla puede decir «la hoja tiene 500
	// filas, 415 sin clave».
	Hoja        string   `json:"hoja"`       // hoja que se leyó
	Hojas       []string `json:"hojas"`      // todas las del archivo, por si se leyó la que no era
	FilasEnHoja int      `json:"filas_hoja"` // filas de datos que trae la hoja (sin el encabezado)
	SinClave    int      `json:"sin_clave"`  // descartadas por no tener clave
	SinFecha    int      `json:"sin_fecha"`  // con la fecha de emisión ilegible
	// FechaCorregida: filas donde la columna del Excel no coincidía con la fecha que trae la clave
	// numérica y se tomó la de la clave. Se cuenta para que se vea: si el número es alto, el archivo
	// viene con las fechas mal y conviene mirar el script que lo genera.
	FechaCorregida int `json:"fecha_corregida"`

	// ── Solo del camino XML ──
	//
	// Versiones del esquema que se encontraron (v4.2 / v4.3 / v4.4). Si algún día aparece una
	// versión nueva, se ve acá en vez de descubrirse por un total en cero.
	Versiones []string `json:"versiones,omitempty"`
	// Descartados: cuántos comprobantes de cada tipo se leyeron pero NO generan cuenta por pagar
	// (nota de crédito, tiquete, recibo de pago…). Se informan para que nada parezca perdido.
	Descartados map[string]int `json:"descartados,omitempty"`
	// XMLIlegibles: archivos que no se pudieron decodificar como XML.
	XMLIlegibles int `json:"xml_ilegibles,omitempty"`
	// RepetidasEnArchivo: el mismo comprobante venía dos veces en la misma entrega.
	RepetidasEnArchivo int `json:"repetidas_en_archivo,omitempty"`
	// Descuadres: comprobantes cuya aritmética no cierra. Ver FilaImportada.Descuadre.
	Descuadres int `json:"descuadres,omitempty"`
	// SinReceptor: comprobantes que no dicen a quién se le facturó, así que no se puede cotejar
	// contra la empresa.
	SinReceptor int `json:"sin_receptor,omitempty"`
}

// PreviewImportacion es el resultado de subir el archivo (sin crear nada aún).
type PreviewImportacion struct {
	Resumen ResumenImportacion `json:"resumen"`
	Filas   []FilaImportada    `json:"filas"`
}

// ResultadoImportacion es el resultado de confirmar la importación.
type ResultadoImportacion struct {
	Creados            int      `json:"creados"`
	OmitidosDuplicados int      `json:"omitidos_duplicados"`
	ProveedoresCreados int      `json:"proveedores_creados"`
	Errores            []string `json:"errores"`
}

// parsearFacturas lee el .xlsx de facturación (una hoja: encabezado + una fila por factura).
// Mapea columnas por nombre de encabezado (tolerante a orden, acentos y mayúsculas).
//
// Devuelve además la traza de lectura: qué hoja se usó, cuántas filas traía y cuántas se
// descartaron. Sin eso, un archivo del que se leen menos facturas de las que tiene no se puede
// diagnosticar — y con el descarte silencioso de las filas sin clave era imposible saber si
// faltaban 400 facturas o si el archivo solo tenía las que se leyeron.
func parsearFacturas(data []byte) ([]FilaImportada, ResumenImportacion, error) {
	var traza ResumenImportacion
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, traza, ErrFormatoImportacion
	}
	defer func() { _ = f.Close() }()

	hojas := f.GetSheetList()
	if len(hojas) == 0 {
		return nil, traza, ErrArchivoVacio
	}
	traza.Hojas = hojas
	// Se prefiere la hoja con la columna Clave, no la primera del archivo: el libro que genera el
	// script de facturación trae también «Catalogo Proveedores», y si algún día quedara primera se
	// leería la hoja equivocada y el resultado serían «0 facturas» sin explicación.
	hoja, rows := hojaDeFacturas(f, hojas)
	if hoja == "" {
		return nil, traza, ErrFormatoImportacion
	}
	traza.Hoja = hoja
	traza.FilasEnHoja = len(rows) - 1
	if len(rows) < 2 {
		return nil, traza, ErrArchivoVacio
	}
	col := mapearColumnas(rows[0])

	out := make([]FilaImportada, 0, len(rows)-1)
	for _, r := range rows[1:] {
		clave := strings.TrimSpace(celda(r, col["clave"]))
		if clave == "" {
			traza.SinClave++
			continue
		}
		// La fecha de emisión sale de la CLAVE cuando la clave la trae; la columna del Excel es el
		// respaldo. Ver fechaDeEmision.
		emision, corregida := fechaDeEmision(clave, celda(r, col["emision"]))
		if corregida {
			traza.FechaCorregida++
		}
		out = append(out, FilaImportada{
			Clave:       clave,
			Consecutivo: strings.TrimSpace(celda(r, col["consecutivo"])),
			// Normalizada ACÁ, en el borde: así ninguna capa de abajo puede recibir «13/08/2026».
			FechaEmision: emision,
			Proveedor:    strings.TrimSpace(celda(r, col["proveedor"])),
			Cedula:       strings.TrimSpace(celda(r, col["cedula"])),
			Moneda:       strings.ToUpper(strings.TrimSpace(celda(r, col["moneda"]))),
			Subtotal:     limpiarNumero(celda(r, col["subtotal"])),
			IVA:          limpiarNumero(celda(r, col["impuestos"])),
			Total:        limpiarNumero(celda(r, col["total"])),
			Condicion:    strings.TrimSpace(celda(r, col["condicion"])),
			Vencimiento:  fechaISO(celda(r, col["vencimiento"])),
			// El tipo de cambio de LA FACTURA. Es el que manda para lo que se le debe al proveedor
			// (decisión del usuario, 2026-09-07): el TC congelado del mes es de Bancos.
			TC: limpiarNumero(celda(r, col["tc"])),
		})
	}
	for i := range out {
		if out[i].FechaEmision == "" {
			traza.SinFecha++
		}
	}
	if len(out) == 0 {
		return nil, traza, ErrArchivoVacio
	}
	return out, traza, nil
}

// hojaDeFacturas elige la hoja que tenga la columna Clave y devuelve sus filas.
//
// Recorre en orden y se queda con la primera que califique: así el libro puede traer otras hojas
// —el catálogo de proveedores, por ejemplo— sin que el importador lea la que no era.
func hojaDeFacturas(f *excelize.File, hojas []string) (string, [][]string) {
	for _, h := range hojas {
		rows, err := f.GetRows(h)
		if err != nil || len(rows) == 0 {
			continue
		}
		if mapearColumnas(rows[0])["clave"] >= 0 {
			return h, rows
		}
	}
	return "", nil
}

func celda(r []string, i int) string {
	if i < 0 || i >= len(r) {
		return ""
	}
	return r[i]
}

// condicionDeFila deriva las condiciones de pago de la factura: "Crédito" en la columna
// Condición + plazo = vencimiento − emisión (en días). Contado => plazo 0.
func condicionDeFila(f FilaImportada) (condicion string, plazoDias int) {
	if !strings.Contains(normEnc(f.Condicion), "credito") {
		return "CONTADO", 0
	}
	emi, err1 := time.Parse("2006-01-02", strings.TrimSpace(f.FechaEmision))
	ven, err2 := time.Parse("2006-01-02", strings.TrimSpace(f.Vencimiento))
	if err1 != nil || err2 != nil {
		return "CREDITO", 0
	}
	d := int(ven.Sub(emi).Hours() / 24)
	if d < 0 {
		d = 0
	}
	return "CREDITO", d
}

// fechaISO normaliza a YYYY-MM-DD lo que traiga el Excel; "" si no se entiende.
//
// ── POR QUÉ NO ALCANZA CON time.Parse("2006-01-02") ─────────────────────────
//
// Excel guarda las fechas como número de serie, pero `excelize.GetRows` devuelve el texto YA
// FORMATEADO según el formato de la celda. El archivo que genera el script de facturación formatea
// la emisión como `dd/mm/yyyy`, así que llega «13/08/2026» y no «2026-08-13».
//
// Eso rompió la importación en producción el 9 de setiembre de 2026, y de la peor manera posible:
// la fecha de emisión se pasaba CRUDA al `::date` de Postgres, que está en `DateStyle = MDY`.
// Resultado:
//
//   - día 13 al 31 → «date/time field value out of range» y la fila se rechazaba con ruido;
//   - día 1 al 12  → **entraba con el mes y el día invertidos, en silencio**. «05/08/2026» se
//     guardaba como 8 de mayo en vez de 5 de agosto, y con eso se corría el vencimiento, el aging
//     y el tablero, sin un solo error a la vista.
//
// El error ruidoso era el síntoma menor. Por eso ahora TODA fecha pasa por acá y nunca se manda
// una cadena sin normalizar a la base.
//
// El orden de los formatos importa: primero el ISO (sin ambigüedad), después el día-primero, que
// es el que usa Costa Rica. No se intenta `mm/dd/yyyy` a propósito — con «05/08» las dos lecturas
// son válidas y adivinar es justo lo que produjo el error silencioso.
func fechaISO(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Excel puede devolver el número de serie si la celda no tiene formato de fecha.
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 0 {
		// 1899-12-30 es el origen de Excel; el desfase de 2 días viene del bug del año 1900 que
		// Excel arrastra a propósito por compatibilidad con Lotus 1-2-3.
		if n >= 1 && n < 100000 {
			return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).
				AddDate(0, 0, int(n)).Format("2006-01-02")
		}
		return ""
	}
	// La hora sobra: la emisión es un día, no un instante.
	if i := strings.IndexAny(s, " T"); i > 0 {
		s = s[:i]
	}
	for _, layout := range []string{
		"2006-01-02", // ISO
		"2006/01/02",
		"02/01/2006", // día-primero: el formato de Costa Rica
		"02-01-2006",
		"2/1/2006", // sin ceros a la izquierda
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// fechaDeEmision decide la fecha de emisión de la factura y dice si hubo que corregir el Excel.
//
// ── POR QUÉ LA CLAVE MANDA SOBRE LA COLUMNA ─────────────────────────────────
//
// La clave numérica de 50 dígitos de Hacienda LLEVA ADENTRO la fecha de emisión, en posiciones
// fijas y en un solo formato posible (`ddmmaa`). No hay nada que interpretar: «1308» son el día 13
// y el mes 08, y no existe la otra lectura. La columna del Excel, en cambio, llega formateada por
// quien generó el archivo, y con «05/08» las dos lecturas son legítimas — que es exactamente lo que
// produjo el error silencioso de producción.
//
// Así que si la clave trae fecha, esa gana. Contra los 4.526 documentos ya importados la fecha de
// la clave coincidió con la registrada en 4.526 casos: cero discrepancias. Es un dato confiable.
//
// La columna queda como respaldo para lo que no tenga clave de 50 dígitos (una nota interna, un
// archivo armado a mano). Y cuando las dos existen y no coinciden, se toma la de la clave y se
// CUENTA, porque un archivo con las fechas mal es un problema del script que lo genera y hay que
// poder verlo.
func fechaDeEmision(clave, celdaExcel string) (fecha string, corregida bool) {
	delExcel := fechaISO(celdaExcel)
	deLaClave := fechaDeClave(clave)
	if deLaClave == "" {
		return delExcel, false
	}
	return deLaClave, delExcel != "" && delExcel != deLaClave
}

// fechaDeClave extrae la fecha de emisión de la clave numérica de 50 dígitos; "" si no se puede.
//
// Estructura de la clave (Hacienda, comprobante electrónico): 3 país + 2 día + 2 mes + 2 año +
// 12 identificación + 20 consecutivo + 1 situación + 8 código de seguridad.
//
// El año viene en dos dígitos y se asume 20xx: la factura electrónica en Costa Rica arrancó en 2018,
// así que no hay ambigüedad de siglo para ningún valor plausible.
func fechaDeClave(clave string) string {
	clave = strings.TrimSpace(clave)
	if len(clave) != 50 {
		return ""
	}
	for _, r := range clave {
		if r < '0' || r > '9' {
			return ""
		}
	}
	// time.Parse valida el calendario: un «3102» (31 de febrero) no pasa y se cae al respaldo.
	t, err := time.Parse("020106", clave[3:9])
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// limpiarNumero quita separadores de miles (coma) y espacios; deja el punto decimal.
func limpiarNumero(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func mapearColumnas(header []string) map[string]int {
	idx := map[string]int{
		"clave": -1, "consecutivo": -1, "emision": -1, "proveedor": -1, "cedula": -1,
		"moneda": -1, "subtotal": -1, "impuestos": -1, "total": -1, "condicion": -1,
		"vencimiento": -1, "tc": -1,
	}
	for i, h := range header {
		n := normEnc(h)
		switch {
		// «Tipo Cambio» va ANTES de «total»: si no, «Total Comprobante» no se confunde pero
		// cualquier encabezado que contenga las dos palabras sí, y el orden acá es la única
		// defensa. Los casos van del más específico al más general.
		case strings.Contains(n, "tipo cambio") || strings.Contains(n, "tipo de cambio"):
			idx["tc"] = i
		case strings.Contains(n, "clave"):
			idx["clave"] = i
		case strings.Contains(n, "consecutivo"):
			idx["consecutivo"] = i
		case strings.Contains(n, "emision"):
			idx["emision"] = i
		case strings.Contains(n, "vencim"):
			idx["vencimiento"] = i
		case strings.Contains(n, "proveedor"):
			idx["proveedor"] = i
		case strings.Contains(n, "cedula"):
			idx["cedula"] = i
		case strings.Contains(n, "moneda"):
			idx["moneda"] = i
		case strings.Contains(n, "subtotal"):
			idx["subtotal"] = i
		case strings.Contains(n, "impuesto"):
			idx["impuestos"] = i
		case strings.Contains(n, "comprobante"):
			idx["total"] = i
		case strings.Contains(n, "condici"):
			idx["condicion"] = i
		}
	}
	return idx
}

// normEnc normaliza un encabezado: minúsculas, sin acentos.
func normEnc(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "ü", "u").Replace(s)
}
