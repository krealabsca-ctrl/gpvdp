package cxc

import (
	"strings"

	"github.com/shopspring/decimal"
)

// Alias del archivo de pagos del sistema de origen (28 columnas en la muestra real).
var aliasCobro = map[string][]string{
	"contrato":     {"contrato", "no. contrato", "numero de contrato"},
	"documento":    {"cedula cliente", "cédula cliente", "documento", "cedula", "cédula"},
	"cliente":      {"cliente id", "cliente", "nombre del cliente"},
	"sucursal":     {"sucursal_contrato", "sucursal", "sede"},
	"consecutivo":  {"consecutivo", "recibo", "no. recibo"},
	"forma_pago":   {"forma pago", "forma de pago", "forma pago contrato"},
	"medio_pago":   {"medio de pago", "medio pago"},
	"asociacion":   {"asociacion", "asociación"},
	"concepto":     {"concepto"},
	"valor":        {"valor", "monto", "monto pagado"},
	"fecha_pago":   {"fecha de pago", "fecha pago"},
	"fecha_banco":  {"fecha de pago bancario de la asociacion", "fecha de pago bancario de la asociación", "fecha bancaria", "fecha de pago bancario"},
	"fecha_creado": {"fecha creacion", "fecha creación", "fecha de creacion", "fecha de creación"},
	"referencia":   {"pago referencia", "referencia"},
	"estado":       {"estado"},
	"observacion":  {"observaciones", "observacion", "observación"},
}

// AplicacionLeida es un tramo de la aplicación que el sistema de origen ya había hecho,
// leído del campo `Concepto`. La usa la migración para reconstruir la historia real en vez
// de aplicar FIFO a ciegas sobre cargos viejos.
type AplicacionLeida struct {
	Periodo string          `json:"periodo"`
	Monto   decimal.Decimal `json:"monto"`
	Parcial bool            `json:"parcial"`
}

// FilaCobro es una fila del archivo de pagos ya interpretada.
type FilaCobro struct {
	Linea int `json:"linea"`

	Contrato    string          `json:"contrato"`
	Documento   string          `json:"documento"`
	Cliente     string          `json:"cliente"`
	Consecutivo string          `json:"consecutivo"`
	FormaPago   string          `json:"forma_pago"`
	Asociacion  string          `json:"asociacion"`
	Referencia  string          `json:"referencia"`
	Monto       decimal.Decimal `json:"monto"`
	// Las TRES fechas del origen, cada una con su significado.
	FechaPago   string `json:"fecha_pago"`
	FechaBanco  string `json:"fecha_bancaria"`
	FechaCreado string `json:"fecha_registro"`
	Estado      string `json:"estado"`
	Concepto    string `json:"concepto"`
	Observacion string `json:"observacion"`
	// Aplicaciones que el propio origen ya había hecho, leídas del Concepto.
	Aplicaciones []AplicacionLeida `json:"aplicaciones"`
	Motivos      []string          `json:"motivos"`
}

// EnCuarentena indica si la fila entra marcada para revisión.
func (f FilaCobro) EnCuarentena() bool { return len(f.Motivos) > 0 }

// Anulado: el origen marca así los pagos que ya no valen. No se importan como cobros
// vivos; entran anotados para que el conteo cuadre con el sistema viejo.
func (f FilaCobro) Anulado() bool {
	return strings.EqualFold(strings.TrimSpace(f.Estado), "Anulado")
}

// ReglasCobros son los umbrales configurables del importador de cobros.
type ReglasCobros struct {
	CobroMaximo decimal.Decimal
}

// LeerCobros interpreta el archivo de pagos. No escribe nada.
func LeerCobros(g Grid, r ReglasCobros) ([]FilaCobro, error) {
	if len(g) == 0 {
		return nil, ErrArchivoVacio
	}
	cols, encabezado := columnasPor(g, aliasCobro, "contrato")
	if cols == nil {
		return nil, ErrSinEncabezado
	}
	if encabezado+1 >= len(g) {
		return nil, ErrSinFilas
	}

	out := make([]FilaCobro, 0, len(g)-encabezado-1)
	for i := encabezado + 1; i < len(g); i++ {
		fila := g[i]
		contrato := celda(fila, cols["contrato"])
		consecutivo := celda(fila, cols["consecutivo"])
		if contrato == "" && consecutivo == "" {
			continue // fila de relleno
		}
		f := FilaCobro{
			Linea:       i + 1,
			Contrato:    contrato,
			Documento:   celda(fila, cols["documento"]),
			Cliente:     celda(fila, cols["cliente"]),
			Consecutivo: consecutivo,
			FormaPago:   primeroNoVacio(celda(fila, cols["forma_pago"]), celda(fila, cols["medio_pago"])),
			Asociacion:  celda(fila, cols["asociacion"]),
			Referencia:  celda(fila, cols["referencia"]),
			Estado:      celda(fila, cols["estado"]),
			Concepto:    celda(fila, cols["concepto"]),
			Observacion: celda(fila, cols["observacion"]),
		}
		f.FechaPago = fechaDe(celda(fila, cols["fecha_pago"]))
		// La fecha bancaria puede traer DOS valores separados por «|» cuando la planilla de
		// la asociación llegó en dos transferencias. `fechaDe` toma la primera; la segunda
		// se conserva en la planilla, que es donde la conciliación del lote la necesita.
		f.FechaBanco = fechaDe(celda(fila, cols["fecha_banco"]))
		f.FechaCreado = fechaDe(celda(fila, cols["fecha_creado"]))

		monto, err := montoDe(celda(fila, cols["valor"]))
		switch {
		case err != nil:
			f.Motivos = append(f.Motivos, "monto ilegible: "+celda(fila, cols["valor"]))
		case monto.Sign() <= 0:
			f.Motivos = append(f.Motivos, "monto en cero o negativo")
		case r.CobroMaximo.Sign() > 0 && monto.GreaterThan(r.CobroMaximo):
			// El caso real del sistema viejo: cédulas pegadas en el campo del monto.
			f.Motivos = append(f.Motivos, "monto sobre el máximo razonable ("+r.CobroMaximo.StringFixed(2)+")")
		}
		f.Monto = monto

		if f.Contrato == "" {
			// Sin contrato no se puede aplicar: es un cobro SIN IDENTIFICAR, no un error.
			f.Motivos = append(f.Motivos, "sin contrato: entra como cobro sin identificar")
		}
		if f.FechaPago == "" {
			f.Motivos = append(f.Motivos, "sin fecha de pago legible")
		}

		// El año no viene en el Concepto: se toma del de la fecha de pago.
		anio := anioDe(f.FechaPago)
		f.Aplicaciones = LeerAplicacionesDelConcepto(f.Concepto, anio)
		// Si el origen decía a qué períodos iba y la suma no cuadra con el monto, hay que
		// mirarlo: no se corrige en silencio ni se descarta la información.
		if len(f.Aplicaciones) > 0 {
			suma := decimal.Zero
			for _, a := range f.Aplicaciones {
				suma = suma.Add(a.Monto)
			}
			if !suma.Equal(monto) {
				f.Motivos = append(f.Motivos,
					"el detalle del concepto suma "+suma.StringFixed(2)+" y el valor es "+monto.StringFixed(2))
			}
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return nil, ErrSinFilas
	}
	return out, nil
}

// LeerAplicacionesDelConcepto parte el campo `Concepto` del sistema de origen en los
// períodos que pagó, con su monto.
//
// Los datos reales tienen esta forma (el monto va PEGADO al nombre del plan):
//
//	"M/JULIO - Ice Corporacion Brunca Zafiro5000.00"
//	"1Q/JULIO - Adepsa Zafiro2500.00, PAGO PARCIAL - 2Q/JULIO - … cuota250.00"
//
// Es la pieza que permite reconstruir la historia de cargos en la migración: el sistema de
// origen YA sabía a qué período iba cada cobro, solo que lo guardaba en un texto donde
// nadie podía sumarlo ni auditarlo.
func LeerAplicacionesDelConcepto(concepto string, anio int) []AplicacionLeida {
	if strings.TrimSpace(concepto) == "" || anio == 0 {
		return nil
	}
	out := []AplicacionLeida{}
	// Cada tramo va separado por coma. La coma no aparece dentro de los montos del origen
	// (usan punto decimal), así que es un separador seguro.
	for _, tramo := range strings.Split(concepto, ",") {
		t := strings.TrimSpace(tramo)
		if t == "" {
			continue
		}
		parcial := strings.Contains(strings.ToUpper(t), "PAGO PARCIAL")
		// El período puede venir después de «PAGO PARCIAL - ».
		periodo, ok := PeriodoDesdeConcepto(quitarPrefijoParcial(t), anio)
		if !ok {
			continue
		}
		monto, ok := montoAlFinal(t)
		if !ok {
			continue
		}
		out = append(out, AplicacionLeida{Periodo: periodo, Monto: monto, Parcial: parcial})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// quitarPrefijoParcial deja el tramo empezando en el período: «PAGO PARCIAL - 2Q/JULIO …»
// se vuelve «2Q/JULIO …».
func quitarPrefijoParcial(t string) string {
	u := strings.ToUpper(t)
	if i := strings.Index(u, "PAGO PARCIAL"); i >= 0 {
		resto := t[i+len("PAGO PARCIAL"):]
		resto = strings.TrimLeft(resto, " -–")
		return resto
	}
	return t
}

// montoAlFinal saca el número pegado al final del texto («…Zafiro2500.00» → 2500.00).
//
// Toma la corrida MÁXIMA de dígitos y separadores al final y la interpreta con `montoDe`,
// que ya sabe de puntos de miles y comas decimales. Es a propósito: una versión anterior
// leía de derecha a izquierda aceptando un solo separador y con «1.084.200,50» devolvía
// **200,50** — un monto aplicado 5 400 veces menor, en silencio. Preferible no leer nada y
// que la fila quede sin detalle, que leer mal.
func montoAlFinal(t string) (decimal.Decimal, bool) {
	fin := len(t)
	// Un punto de cierre de oración no es parte del monto («…de cuota250.00.»).
	for fin > 0 && (t[fin-1] == ' ' || t[fin-1] == '.') {
		if t[fin-1] == '.' && fin >= 2 && esDigito(t[fin-2]) {
			break
		}
		fin--
	}
	i := fin
	for i > 0 {
		c := t[i-1]
		if esDigito(c) || c == '.' || c == ',' {
			i--
			continue
		}
		break
	}
	// La corrida tiene que empezar en un dígito: «.00» no es un monto.
	for i < fin && !esDigito(t[i]) {
		i++
	}
	if i >= fin {
		return decimal.Zero, false
	}
	v, err := montoDe(t[i:fin])
	if err != nil || v.Sign() <= 0 {
		return decimal.Zero, false
	}
	return v, true
}

func esDigito(c byte) bool { return c >= '0' && c <= '9' }

func anioDe(fecha string) int {
	if len(fecha) < 4 {
		return 0
	}
	n := 0
	for i := 0; i < 4; i++ {
		if !esDigito(fecha[i]) {
			return 0
		}
		n = n*10 + int(fecha[i]-'0')
	}
	return n
}

func primeroNoVacio(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// columnasPor resuelve columnas por encabezado con alias, exigiendo que exista `obligatoria`.
// Lo comparten los dos importadores (cartera y cobros) para que agregar un alias sirva en
// los dos y para que ninguno vuelva a leer por posición.
func columnasPor(g Grid, alias map[string][]string, obligatoria string) (map[string]int, int) {
	limite := 10
	if len(g) < limite {
		limite = len(g)
	}
	for fila := 0; fila < limite; fila++ {
		cols := map[string]int{}
		for i, c := range g[fila] {
			enc := normalizarEncabezado(c)
			if enc == "" {
				continue
			}
			for campo, lista := range alias {
				if _, ya := cols[campo]; ya {
					continue
				}
				for _, a := range lista {
					if enc == a {
						cols[campo] = i
						break
					}
				}
			}
		}
		if _, ok := cols[obligatoria]; ok {
			for campo := range alias {
				if _, ok := cols[campo]; !ok {
					cols[campo] = -1
				}
			}
			return cols, fila
		}
	}
	return nil, 0
}
