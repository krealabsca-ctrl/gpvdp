package cxp

import (
	"bytes"
	"errors"
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
	Clave          string       `json:"clave"`
	Consecutivo    string       `json:"consecutivo"`
	FechaEmision   string       `json:"fecha_emision"`
	Proveedor      string       `json:"proveedor"`
	Cedula         string       `json:"cedula"`
	Moneda         string       `json:"moneda"`
	Subtotal       string       `json:"subtotal"`
	IVA            string       `json:"iva"`
	Total          string       `json:"total"`
	Condicion      string       `json:"condicion"`
	Vencimiento    string       `json:"vencimiento"`
	Estado         ImportEstado `json:"estado"`
	ProveedorNuevo bool         `json:"proveedor_nuevo"`
}

// ResumenImportacion son los totales del preview.
type ResumenImportacion struct {
	Leidas            int `json:"leidas"`
	Nuevas            int `json:"nuevas"`
	Duplicadas        int `json:"duplicadas"`
	ProveedoresNuevos int `json:"proveedores_nuevos"`
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
func parsearFacturas(data []byte) ([]FilaImportada, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, ErrFormatoImportacion
	}
	defer func() { _ = f.Close() }()

	hojas := f.GetSheetList()
	if len(hojas) == 0 {
		return nil, ErrArchivoVacio
	}
	rows, err := f.GetRows(hojas[0])
	if err != nil || len(rows) < 2 {
		return nil, ErrArchivoVacio
	}
	col := mapearColumnas(rows[0])
	if col["clave"] < 0 {
		return nil, ErrFormatoImportacion
	}

	out := make([]FilaImportada, 0, len(rows)-1)
	for _, r := range rows[1:] {
		clave := strings.TrimSpace(celda(r, col["clave"]))
		if clave == "" {
			continue
		}
		out = append(out, FilaImportada{
			Clave:        clave,
			Consecutivo:  strings.TrimSpace(celda(r, col["consecutivo"])),
			FechaEmision: strings.TrimSpace(celda(r, col["emision"])),
			Proveedor:    strings.TrimSpace(celda(r, col["proveedor"])),
			Cedula:       strings.TrimSpace(celda(r, col["cedula"])),
			Moneda:       strings.ToUpper(strings.TrimSpace(celda(r, col["moneda"]))),
			Subtotal:     limpiarNumero(celda(r, col["subtotal"])),
			IVA:          limpiarNumero(celda(r, col["impuestos"])),
			Total:        limpiarNumero(celda(r, col["total"])),
			Condicion:    strings.TrimSpace(celda(r, col["condicion"])),
			Vencimiento:  strings.TrimSpace(celda(r, col["vencimiento"])),
		})
	}
	if len(out) == 0 {
		return nil, ErrArchivoVacio
	}
	return out, nil
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

// fechaISO devuelve s si es una fecha válida YYYY-MM-DD; si no, "" (para no romper el INSERT ::date).
func fechaISO(s string) string {
	s = strings.TrimSpace(s)
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return ""
	}
	return s
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
		"moneda": -1, "subtotal": -1, "impuestos": -1, "total": -1, "condicion": -1, "vencimiento": -1,
	}
	for i, h := range header {
		n := normEnc(h)
		switch {
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
