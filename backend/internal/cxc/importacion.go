package cxc

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// Errores del importador.
var (
	ErrArchivoVacio     = errors.New("cxc: el archivo está vacío")
	ErrSinEncabezado    = errors.New("cxc: no se encontró la fila de encabezados (falta la columna Contrato)")
	ErrSinFilas         = errors.New("cxc: el archivo trae encabezado pero ninguna fila de datos")
	ErrImportacionAjena = errors.New("cxc: la importación no existe o es de otra empresa")
)

// Campos del archivo de cartera. Los nombres son los del sistema de origen; los alias
// existen porque el mismo reporte se exporta con encabezados distintos según quién lo
// saque, y porque la API expondrá la misma información con otro nombre.
//
// La resolución es POR ENCABEZADO, nunca por posición: la cicatriz del portal de Apps
// Script fue justamente que una columna corrida desplazó todos los datos en silencio.
var aliasContrato = map[string][]string{
	// LOS ALIAS SE ESCRIBEN COMO SE COMPARAN: sin tildes y con espacios, porque el encabezado
	// se normaliza así antes de buscarlo (ver normalizarEncabezado). Un alias con tilde o con
	// guion bajo no puede calzar nunca; había quince acá —«cédula», «teléfonos», «día de
	// pago»…— que parecían cubrir un caso y no cubrían ninguno, todos duplicando a su gemelo
	// sin tilde, que es el que hacía el trabajo. Hay una prueba que lo exige.
	"contrato":      {"contrato", "no. contrato", "numero de contrato", "num contrato"},
	"cliente":       {"cliente", "nombre del cliente", "cliente id", "nombre"},
	"documento":     {"documento", "cedula", "identificacion", "cedula cliente"},
	"telefonos":     {"telefonos", "telefono"},
	"otro_telefono": {"otro telefono", "telefono 2"},
	"correos":       {"correos", "correo", "email", "correo electronico"},
	"sede":          {"sede", "sucursal", "sucursal contrato", "sede operativa", "sede contrato"},
	"dia_pago":      {"dias de pagos", "dia de pago", "dia pago"},
	"servicio":      {"servicios", "servicio", "plan"},
	"tipo_servicio": {"tipo de servicio", "tipo servicio"},
	// «asocaicion» NO es un error de tipeo de acá: así viene escrito el encabezado en el
	// archivo que exporta el sistema de origen. Se reconoce tal cual porque corregirlo allá
	// no está en nuestras manos, y sin esta línea la columna entra vacía.
	"asociacion":    {"asociacion", "asociacion siglas", "asocaicion"},
	"cuota":         {"cuota servicio", "cuota", "monto cuota", "cuota del servicio"},
	"modalidad":     {"modalidad de cobro", "modalidad", "periodicidad"},
	"forma_pago":    {"forma de pago", "forma pago", "medio de pago"},
	"dias_vencidos": {"dias vencidos"},
	"score":         {"score", "puntaje"},
	"estado":        {"estado", "estado del contrato", "contrato estado"},
	"morosidad":     {"estatus de morosidad", "estatus morosidad", "morosidad"},
	"saldo":         {"saldo pendiente", "saldo"},
	"fecha_inicial": {"contrato fecha inicial", "fecha inicial", "fecha de inicio", "fecha inicio contrato"},
	"primer_cobro":  {"contrato fecha primer cobro", "fecha primer cobro", "primer cobro"},
	"proximo_pago":  {"fecha proximo pago", "fecha del proximo pago", "proximo pago"},
	"tarjeta_vence": {"fecha vencimiento tarjeta", "vencimiento tarjeta", "tarjeta vence"},
	"mes_cobro":     {"mes cobro/ano", "mes cobro", "mes de cobro"},
}

// FilaContrato es una fila del archivo ya interpretada, con los motivos por los que
// debería quedar en cuarentena. La fila NUNCA se descarta en silencio: entra marcada.
type FilaContrato struct {
	Linea int `json:"linea"` // línea del archivo, para que el usuario la encuentre

	Numero       string          `json:"numero"`
	Cliente      string          `json:"cliente"`
	Documento    string          `json:"documento"`
	Telefonos    string          `json:"telefonos"`
	Correos      string          `json:"correos"`
	SedeCruda    string          `json:"sede_cruda"`
	RazonSocial  string          `json:"razon_social"`
	Plaza        string          `json:"plaza"`
	Servicio     string          `json:"servicio"`
	TipoServicio string          `json:"tipo_servicio"`
	Asociacion   string          `json:"asociacion"`
	Modalidad    string          `json:"modalidad"`
	FormaPago    string          `json:"forma_pago"`
	DiaPago      int             `json:"dia_pago"`
	Cuota        decimal.Decimal `json:"cuota"`
	FechaInicial string          `json:"fecha_inicial"`
	PrimerCobro  string          `json:"primer_cobro"`
	TarjetaVence string          `json:"tarjeta_vence"`
	Estado       string          `json:"estado"`
	// Datos del origen: informativos. El ERP calcula su propio tramo con la antigüedad
	// real de los cargos; estos sirven para comparar contra el sistema viejo durante la
	// corrida en paralelo.
	ScoreOrigen        *int             `json:"score_origen"`
	EstadoOrigen       string           `json:"estado_origen"`
	MorosidadOrigen    string           `json:"morosidad_origen"`
	DiasVencidosOrigen *int             `json:"dias_vencidos_origen"`
	SaldoOrigen        *decimal.Decimal `json:"saldo_origen"`

	Motivos []string `json:"motivos"`
}

// EnCuarentena indica si la fila entra marcada para revisión.
func (f FilaContrato) EnCuarentena() bool { return len(f.Motivos) > 0 }

// ReglasImportacion son los umbrales configurables que deciden qué va a cuarentena.
// Vienen de cxc_parametro: el negocio los mueve sin tocar código.
type ReglasImportacion struct {
	CuotaMaxima decimal.Decimal
}

// LeerContratos interpreta el archivo de cartera. No toca la base y no decide nada
// irreversible: devuelve lo que entendió y por qué dudaría de cada fila, para que la
// pantalla lo muestre antes de confirmar.
func LeerContratos(h Hoja, r ReglasImportacion) ([]FilaContrato, error) {
	g := h.Valores
	if len(g) == 0 {
		return nil, ErrArchivoVacio
	}
	cols, encabezado := columnasContrato(g)
	if cols == nil {
		return nil, ErrSinEncabezado
	}
	if encabezado+1 >= len(g) {
		return nil, ErrSinFilas
	}

	out := make([]FilaContrato, 0, len(g)-encabezado-1)
	for i := encabezado + 1; i < len(g); i++ {
		fila := g[i]
		// La MISMA fila sin el formato del libro, de donde salen las fechas. Se busca por
		// número de fila y no se asume que las dos lecturas tengan el mismo largo: `cruda`
		// puede venir vacía, y `celda` ya devuelve "" fuera de rango.
		cruda := filaCruda(h.Crudos, i)
		// fecha lee una columna por su nombre usando las dos versiones de la celda.
		fecha := func(col string) string {
			return fechaDeCelda(celda(fila, cols[col]), celda(cruda, cols[col]))
		}
		numero := celda(fila, cols["contrato"])
		if numero == "" {
			continue // fila de relleno o total: no es un contrato
		}
		f := FilaContrato{
			Linea: i + 1,
			// El número se guarda TAL CUAL: los datos reales traen «CO198456» y
			// «CD-0000000561». Normalizarlo rompería el cruce con el sistema de origen.
			Numero:          numero,
			Cliente:         celda(fila, cols["cliente"]),
			Documento:       celda(fila, cols["documento"]),
			Telefonos:       juntarTelefonos(celda(fila, cols["telefonos"]), celda(fila, cols["otro_telefono"])),
			Correos:         celda(fila, cols["correos"]),
			SedeCruda:       celda(fila, cols["sede"]),
			Servicio:        celda(fila, cols["servicio"]),
			TipoServicio:    celda(fila, cols["tipo_servicio"]),
			Asociacion:      celda(fila, cols["asociacion"]),
			Modalidad:       celda(fila, cols["modalidad"]),
			FormaPago:       celda(fila, cols["forma_pago"]),
			EstadoOrigen:    celda(fila, cols["estado"]),
			MorosidadOrigen: celda(fila, cols["morosidad"]),
		}
		f.RazonSocial, f.Plaza = partirSede(f.SedeCruda)

		// ── Cuota
		cuota, err := montoDe(celda(fila, cols["cuota"]))
		switch {
		case err != nil:
			f.Motivos = append(f.Motivos, fmt.Sprintf("cuota ilegible: %q", celda(fila, cols["cuota"])))
		case cuota.Sign() <= 0:
			f.Motivos = append(f.Motivos, "cuota en cero o negativa")
		case r.CuotaMaxima.Sign() > 0 && cuota.GreaterThan(r.CuotaMaxima):
			// El caso real del portal: un SALDO pegado en el campo de cuota. Cuotas de
			// miles de millones existieron de verdad.
			f.Motivos = append(f.Motivos, fmt.Sprintf("cuota sobre el máximo razonable (%s)", r.CuotaMaxima.StringFixed(2)))
		}
		f.Cuota = cuota

		// ── Día de pago
		if dp := celda(fila, cols["dia_pago"]); dp != "" {
			n, err := strconv.Atoi(soloDigitos(dp))
			if err != nil || n < 1 || n > 31 {
				f.Motivos = append(f.Motivos, fmt.Sprintf("día de pago inválido: %q", dp))
			} else {
				f.DiaPago = n
			}
		}

		// ── Fechas: del número de serie si la celda es una fecha de Excel, y del texto
		// (d/m/aaaa) si vino escrita como tal. Ver fechaDeCelda.
		f.FechaInicial = fecha("fecha_inicial")
		f.PrimerCobro = fecha("primer_cobro")
		if f.PrimerCobro == "" {
			// Sin fecha de primer cobro no se puede generar ni un cargo, y no se
			// inventa: la fila entra marcada y no estorba a las demás.
			if pp := fecha("proximo_pago"); pp != "" {
				f.PrimerCobro = pp
				f.Motivos = append(f.Motivos, "sin fecha de primer cobro: se tomó la de próximo pago")
			} else {
				f.Motivos = append(f.Motivos, "sin fecha de primer cobro ni de próximo pago")
			}
		}
		// La tarjeta viene como mes-año («oct-28», «nov-29») y vale hasta el ÚLTIMO día de ese
		// mes. Ver finDeMesDeCelda: Excel convierte «ago-23» en el 1 de agosto, y quedarse con
		// ese día le quitaría a la tarjeta el mes de vigencia que la regla le da.
		f.TarjetaVence = finDeMesDeCelda(celda(fila, cols["tarjeta_vence"]), celda(cruda, cols["tarjeta_vence"]))

		// ── Datos informativos del origen
		if s := celda(fila, cols["score"]); s != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				f.ScoreOrigen = &n
			}
		}
		if s := celda(fila, cols["dias_vencidos"]); s != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				f.DiasVencidosOrigen = &n
			}
		}
		if s := celda(fila, cols["saldo"]); s != "" {
			if v, err := montoDe(s); err == nil {
				f.SaldoOrigen = &v
			}
		}

		if f.Modalidad == "" {
			f.Motivos = append(f.Motivos, "sin modalidad de cobro")
		}
		if f.SedeCruda == "" {
			// No es cuarentena: el contrato existe y se puede cobrar. Queda sin sede
			// operativa y aparece en el reporte para que alguien lo asigne.
			f.Motivos = append(f.Motivos, "sin sede")
		}

		out = append(out, f)
	}
	if len(out) == 0 {
		return nil, ErrSinFilas
	}
	return out, nil
}

// columnasContrato busca la fila de encabezados en las primeras filas del archivo
// (algunos exportadores ponen título y fecha arriba) y resuelve cada campo por alias.
func columnasContrato(g Grid) (map[string]int, int) {
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
			for campo, alias := range aliasContrato {
				if _, ya := cols[campo]; ya {
					continue
				}
				for _, a := range alias {
					if enc == normalizarEncabezado(a) {
						cols[campo] = i
						break
					}
				}
			}
		}
		// «Contrato» es el mínimo indispensable: sin él no hay nada que importar.
		if _, ok := cols["contrato"]; ok {
			for campo := range aliasContrato {
				if _, ok := cols[campo]; !ok {
					cols[campo] = -1 // ausente: celda() devolverá ""
				}
			}
			return cols, fila
		}
	}
	return nil, 0
}

func normalizarEncabezado(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, bom)))
	// El GUION BAJO y el guion son separadores de palabra, no letras. El sistema de origen
	// exporta los encabezados en snake_case —«fecha_primer_cobro», «sede_contrato»,
	// «modalidad_de_cobro»— y sin esto ninguno calza con su alias, que está escrito con
	// espacios. Medido el 8-oct-2026 sobre el archivo real: de 31 columnas, las de fecha,
	// sede, modalidad, forma de pago y día de pago entraban todas vacías, y las 23.144 filas
	// caían en cuarentena por datos que el archivo traía puestos.
	s = strings.Map(func(r rune) rune {
		if r == '_' || r == '-' {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ") // colapsa espacios dobles
	return sinTildes(s)
}

// sinTildes quita los acentos para que «Cédula» y «Cedula» sean el mismo encabezado.
func sinTildes(s string) string {
	r := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
		"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "Ü", "u", "Ñ", "n",
	)
	return r.Replace(s)
}

// partirSede separa el campo «Sede», que el origen trae con la plaza y la razón social
// pegadas con un guion y en dos órdenes distintos:
//
//	«SAN JOSÉ - VALLE DE PAZ DE COSTA RICA SA»
//	«Sucursal de San Carlos - Jardines Tropicales y Forestales La Paz S.A-San carlos»
//
// La razón social se reconoce por el sufijo societario (SA, S.A., LTDA…). Si no se
// puede decidir, se devuelve todo como plaza y nada como razón social: es mejor que la
// pantalla de mapeo lo muestre en blanco que adivinar mal quién es el dueño del contrato.
func partirSede(s string) (razonSocial, plaza string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	partes := strings.SplitN(s, " - ", 2)
	if len(partes) != 2 {
		return "", s
	}
	a, b := strings.TrimSpace(partes[0]), strings.TrimSpace(partes[1])
	switch {
	case pareceRazonSocial(b):
		return b, a
	case pareceRazonSocial(a):
		return a, b
	default:
		return "", s
	}
}

func pareceRazonSocial(s string) bool {
	t := sinTildes(strings.ToUpper(s))
	for _, suf := range []string{" SA", " S.A", " S. A", " LTDA", " LIMITADA", " SRL", " S.R.L"} {
		if strings.Contains(t, suf) {
			return true
		}
	}
	return false
}

func juntarTelefonos(a, b string) string {
	switch {
	case a != "" && b != "" && a != b:
		return a + " / " + b
	case a != "":
		return a
	default:
		return b
	}
}

// montoDe lee un monto tolerando las formas en que los exportadores escriben dinero:
// «5600.00», «5 600,00», «₡5,600.00». Devuelve error en vez de cero cuando no entiende:
// un cero silencioso se volvería un contrato sin cuota que nadie cobra.
func montoDe(s string) (decimal.Decimal, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return decimal.Zero, nil
	}
	t = strings.NewReplacer("₡", "", "$", "", "CRC", "", "USD", "", " ", "", " ", "").Replace(t)
	// Si hay coma y punto, el ÚLTIMO separador es el decimal.
	ultimaComa, ultimoPunto := strings.LastIndex(t, ","), strings.LastIndex(t, ".")
	switch {
	case ultimaComa >= 0 && ultimoPunto >= 0:
		if ultimaComa > ultimoPunto {
			t = strings.ReplaceAll(t, ".", "")
			t = strings.Replace(t, ",", ".", 1)
		} else {
			t = strings.ReplaceAll(t, ",", "")
		}
	case ultimaComa >= 0:
		// Solo coma: decimal si deja 1 o 2 dígitos detrás; si no, es de miles.
		if len(t)-ultimaComa-1 <= 2 {
			t = strings.Replace(t, ",", ".", 1)
		} else {
			t = strings.ReplaceAll(t, ",", "")
		}
	}
	v, err := decimal.NewFromString(t)
	if err != nil {
		return decimal.Zero, fmt.Errorf("cxc: monto ilegible %q: %w", s, err)
	}
	return v, nil
}

// Rango en el que un número de serie de Excel se acepta como fecha: 1990-01-01 a
// 2200-01-01.
//
// El tope de abajo es lo que importa. Sin él, un «4» —el día de pago de una columna mal
// mapeada, o una celda corrida— se convertiría en el 4 de enero de 1900 y entraría como una
// fecha perfectamente válida. Con él, ese número no se parece a una fecha, cae al camino del
// texto y, si tampoco se entiende ahí, la fila queda marcada, que es lo correcto. Una
// cartera cuyos cobros arrancan antes de 1990 no existe, y si existiera, el valor
// formateado todavía puede rescatarla.
const (
	serieMinima = 32874.0  // 1990-01-01
	serieMaxima = 109574.0 // 2200-01-01
)

// filaCruda devuelve la fila `i` de la lectura sin formato, o nil si no está.
//
// Devuelve nil en vez de fallar porque las dos lecturas pueden no tener el mismo largo (una
// hoja con filas finales vacías las recorta distinto según se aplique o no el formato). Sin
// la fila cruda, las fechas de esa línea se leen del texto, que es el comportamiento de
// antes: se pierde la mejora, no el dato.
func filaCruda(crudos Grid, i int) []string {
	if i < 0 || i >= len(crudos) {
		return nil
	}
	return crudos[i]
}

// fechaDeCelda lee una fecha mirando PRIMERO el valor crudo.
//
// Si el crudo es un número de serie de Excel, esa es la fecha y no hay nada que interpretar:
// 44593 es el 1 de febrero de 2022 en cualquier equipo, con cualquier configuración
// regional y cualquier formato de celda. Solo cuando el crudo no es una serie —una fecha
// escrita como texto, o un CSV, donde crudo y formateado son lo mismo— se cae al texto, que
// es donde sí hay que saber que el origen escribe d/m/aaaa.
//
// Ese orden es el arreglo: al leer solo el texto formateado, una fecha real de Excel llegaba
// como «02-01-22» y se perdía entera (7-oct-2026). Devolver el texto a la primera
// posición volvería a poner a adivinar si el 02 es el mes o el día.
func fechaDeCelda(formateado, crudo string) string {
	if t := strings.TrimSpace(crudo); t != "" {
		if serie, err := strconv.ParseFloat(t, 64); err == nil {
			if serie >= serieMinima && serie <= serieMaxima {
				if v, err := excelize.ExcelDateToTime(serie, false); err == nil {
					return v.Format("2006-01-02")
				}
			}
			// Es un número y no es una serie plausible: NO se intenta leerlo como texto.
			// «4» no es el 4 de enero de 1900 ni ninguna otra fecha, y dejarlo seguir sería
			// inventar un dato que el archivo no trae.
			return ""
		}
	}
	return fechaDe(formateado)
}

// fechaDe lee las fechas del origen escritas como TEXTO. El formato real es d/m/aaaa
// (3/8/2026 = 3 de AGOSTO), así que el día va primero: interpretarlo al revés movería de mes
// miles de vencimientos. Devuelve "" si no entiende, para que el llamador decida.
//
// Para leer una celda de Excel usar fechaDeCelda, que mira el número de serie antes que
// esto: el texto de una fecha real de Excel lo genera el formato del libro y puede venir en
// cualquier orden.
func fechaDe(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	// Un mismo campo puede traer dos fechas separadas por «|» (la planilla de una
	// asociación que llegó en dos transferencias): se toma la primera.
	if i := strings.Index(t, "|"); i > 0 {
		t = strings.TrimSpace(t[:i])
	}
	for _, layout := range []string{"2/1/2006", "02/01/2006", "2006-01-02", "2/1/06", "02/01/06", "2006/01/02"} {
		if v, err := time.Parse(layout, t); err == nil {
			return v.Format("2006-01-02")
		}
	}
	return ""
}

// finDeMesDeCelda lee el vencimiento de una tarjeta de una celda de Excel.
//
// Cuando el crudo es un número de serie, la fecha se lleva al ÚLTIMO día de su mes, que es la
// regla que este campo ya tenía escrita: una tarjeta que vence en octubre sirve todo octubre.
// Hace falta porque el origen escribe «ago-23» y Excel lo guarda como el 1 de agosto: quedarse
// con ese día le quitaría a la tarjeta el mes de vigencia que la regla le da, y el cobro se
// cortaría antes de tiempo. Medido sobre el archivo real: los 12 contratos que traen
// vencimiento caen los 12 en día 1, que es la firma de un mes-año convertido.
//
// El camino del TEXTO se deja como estaba —finDeMesDe respeta una fecha completa tal cual— para
// no cambiar lo que ya venía funcionando en los archivos CSV.
func finDeMesDeCelda(formateado, crudo string) string {
	if v := fechaDeCelda("", crudo); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).
				AddDate(0, 1, -1).Format("2006-01-02")
		}
	}
	return finDeMesDe(formateado)
}

// finDeMesDe lee un «mes-año» como los del vencimiento de tarjeta («oct-28»,
// «nov-29», «sept-26») y devuelve el ÚLTIMO día de ese mes: una tarjeta que vence en
// octubre sirve todo octubre.
func finDeMesDe(s string) string {
	t := strings.ToUpper(strings.TrimSpace(s))
	if t == "" {
		return ""
	}
	if f := fechaDe(s); f != "" {
		return f
	}
	partes := strings.FieldsFunc(t, func(r rune) bool { return r == '-' || r == '/' || r == ' ' })
	if len(partes) != 2 {
		return ""
	}
	mes, ok := mesesES[sinTildes(partes[0])]
	if !ok {
		return ""
	}
	anio, err := strconv.Atoi(partes[1])
	if err != nil {
		return ""
	}
	if anio < 100 {
		anio += 2000
	}
	primero := time.Date(anio, mes, 1, 0, 0, 0, 0, time.UTC)
	return primero.AddDate(0, 1, -1).Format("2006-01-02")
}

func soloDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
