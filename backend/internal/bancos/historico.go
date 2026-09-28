package bancos

// CARGAR HISTÓRICO — el lector del archivo (funciones puras, sin base de datos).
//
// ── POR QUÉ EXISTE ──────────────────────────────────────────────────────────
//
// Textual del Director Financiero: «lo que no se puede hacer es buscar todos los bancos de estos
// años e ir de uno en uno cargandolo». Tiene 2025 y 2026 en Excel, ya segmentados por él, y quiere
// subir UN archivo con VARIAS CUENTAS y VARIOS MESES adentro que CREE los movimientos con su
// partida ya puesta.
//
// Su primer intento fue con «Traer la clasificación desde Excel», que SOLO pinta la partida sobre
// movimientos ya cargados y nunca los crea: como el sistema no tenía nada anterior al 01/07/2026,
// todas sus filas salieron «ese movimiento no está cargado».
//
// ── QUÉ HACE ESTE LECTOR DISTINTO DEL OTRO ──────────────────────────────────
//
// Tres cosas, y las tres salen de medir archivos .xlsx de verdad:
//
//  1. FECHAS. El otro lector usa `GetRows`, que devuelve el valor FORMATEADO. Una celda que Excel
//     guarda como fecha de verdad sale como «07-03-26» y no se entiende; «15/1/2025» (día de un
//     dígito) tampoco. Acá se lee TAMBIÉN el valor CRUDO (`RawCellValue`), que para una fecha real
//     es el serial, y el texto se parsea con día y mes de uno o dos dígitos.
//
//  2. DECORACIÓN. El reporte que exporta el sistema intercala la banda de cada partida, los
//     «Subtotal …», el «TOTAL · N movimiento(s)» y el pie «Generado por GPVDP ERP…». Esas filas se
//     SALTAN y se cuentan aparte. Reportarlas como errores le mostraría al usuario media docena de
//     errores falsos en cada archivo que el propio sistema le dio.
//
//  3. PARÉNTESIS. `parseMontoTolerante` borra los paréntesis, así que «(1.500,00)» —el negativo
//     contable de Excel— se lee como +1500 EN SILENCIO. En un lector que solo pinta etiquetas eso
//     es un movimiento que no calza; en uno que CREA movimientos es plata con el signo cambiado.

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

var (
	// ErrHistoricoSinEncabezado indica que no se encontró la fila de encabezado.
	ErrHistoricoSinEncabezado = errors.New(
		"bancos: no se encontró el encabezado (se esperan al menos las columnas «Fecha» y «Débito»/«Crédito»)")
	// ErrHistoricoVacio indica que el archivo no trae ni una fila de movimiento.
	ErrHistoricoVacio = errors.New("bancos: el archivo no trae ninguna fila de movimiento debajo del encabezado")
	// ErrHistoricoDemasiadasFilas indica que el archivo pasa el tope y hay que partirlo.
	//
	// Es un ERROR y no un recorte: un archivo de dos años cargado «hasta donde se pudo» deja un
	// faltante que no se nota hasta que no cuadra el año.
	ErrHistoricoDemasiadasFilas = fmt.Errorf(
		"bancos: el archivo trae más de %d filas de movimiento, que es el tope de una carga histórica; partilo por año o por cuenta y subí los pedazos",
		maxFilasHistorico)
	// ErrHistoricoSinNadaQueCargar frena un confirmar que no escribiría ni una fila.
	ErrHistoricoSinNadaQueCargar = errors.New(
		"bancos: no quedó ninguna fila para cargar: revisá las cuentas que no se resolvieron y los errores del archivo")
	// ErrCargaHistoricaNoEncontrada: la carga no existe o es de otra empresa.
	ErrCargaHistoricaNoEncontrada = errors.New("bancos: esa carga histórica no existe")
	// ErrCargaHistoricaYaConfirmada frena el doble clic en «Confirmar».
	ErrCargaHistoricaYaConfirmada = errors.New(
		"bancos: esa carga histórica ya se confirmó; si te equivocaste, revertí las cargas desde «Cargas hechas»")
	// ErrHistoricoArchivoIlegible: lo que se subió no es un .xlsx que se pueda abrir.
	//
	// Sin este centinela el error de excelize caía en el `default` de responderError y salía como
	// 500 «error interno» (medido contra la API real subiendo un .csv y un archivo cualquiera con
	// nombre .xlsx). El usuario no puede distinguir «me equivoqué de archivo» de «se cayó el
	// servidor», así que reintenta el mismo archivo y vuelve a fallar.
	ErrHistoricoArchivoIlegible = errors.New(
		"bancos: no se pudo leer el archivo: tiene que ser un .xlsx. Si lo exportaste como .csv o .xls, " +
			"abrilo en Excel y guardalo como «Libro de Excel (.xlsx)» antes de volver a subirlo")
)

// maxFilasHistorico es el tope de filas de MOVIMIENTO por archivo (la decoración no cuenta).
// Es el mismo del otro lector: una cuenta activa de Valle de Paz pasa las nueve mil filas en mes y
// medio, y dos años de quince cuentas no caben en una sola subida.
const maxFilasHistorico = 50000

// maxErroresHistorico es cuántos errores de fila viajan al navegador. El CONTADOR siempre es del
// total; lo que se recorta es la lista, y cuando se recorta la respuesta lo dice.
const maxErroresHistorico = 300

// FilaHistorico es una línea de movimiento leída del archivo, antes de resolver nada contra la base.
type FilaHistorico struct {
	Linea int
	// Cuenta es el texto TAL CUAL venía en la celda: es lo que se le muestra al usuario cuando no
	// se puede resolver, para que cree esa cuenta con ese nombre y vuelva a subir.
	Cuenta        string
	Fecha         time.Time
	Documento     string
	Descripcion   string
	Debito        decimal.Decimal
	Credito       decimal.Decimal
	Concepto      string
	Clasificacion string
}

// ErrorFilaHistorico es una fila que no se pudo leer, con el texto original que no se entendió.
//
// El texto va en la respuesta a propósito: «no se entiende la fecha» sin decir QUÉ decía la celda
// obliga a abrir el Excel y contar filas a mano.
type ErrorFilaHistorico struct {
	Linea  int    `json:"linea"`
	Motivo string `json:"motivo"`
	Texto  string `json:"texto"`
}

// LecturaHistorico es todo lo que salió de leer el archivo.
type LecturaHistorico struct {
	Filas   []FilaHistorico
	Errores []ErrorFilaHistorico
	// LineasDeFormato son las filas de decoración del reporte (bandas, subtotales, total y pie).
	// Se cuentan y se nombran para que el usuario sepa que no se perdió nada.
	LineasDeFormato int
	// Hoja leída y todas las hojas del libro.
	Hoja  string
	Hojas []string
}

// columnasHistorico busca la fila de encabezado y devuelve papel→índice.
//
// El encabezado NO está en la fila 1 cuando el archivo es el reporte que exporta el sistema: ahí
// van dos filas de título antes y el encabezado arranca en la 3. Por eso se busca, igual que hace
// columnasClasifExcel.
//
// Se reconoce por tener FECHA y al menos una de las dos columnas de monto: acá una fila ES un
// movimiento, y sin monto no hay movimiento que crear. (El otro lector pide fecha + partida porque
// su trabajo es otro: pintar la etiqueta sobre algo que ya existe.)
func columnasHistorico(g Grid) (map[string]int, int) {
	for i := 0; i < len(g) && i < 20; i++ {
		col := map[string]int{}
		for j, c := range g[i] {
			if papel, ok := encabezadosMovimientos[norm(c)]; ok {
				if _, repetido := col[papel]; !repetido {
					col[papel] = j
				}
			}
		}
		_, hayFecha := col["fecha"]
		_, hayDebito := col["debito"]
		_, hayCredito := col["credito"]
		if hayFecha && (hayDebito || hayCredito) {
			return col, i
		}
	}
	return nil, 0
}

// reFechaNumerica acepta día y mes de UNO o dos dígitos: «15/1/2025» es lo que escribe una persona
// en Excel y el otro lector no lo entiende porque el layout «02/01/2006» de Go exige dos dígitos.
var reFechaNumerica = regexp.MustCompile(`^(\d{1,4})[/-](\d{1,2})[/-](\d{1,4})$`)

// reFechaConMes acepta «15-ene-2025» y «15 ENE 2025»: es como Excel formatea con NumFmt 15/16.
var reFechaConMes = regexp.MustCompile(`^(\d{1,2})[/ .-]([A-Za-zÁÉÍÓÚÑáéíóúñ]{3,})[/ .-](\d{2,4})$`)

// reNumeroCrudo reconoce el valor CRUDO de una celda numérica de Excel (siempre punto decimal y
// sin separador de miles). Cuando la celda es un número de verdad, ese valor es la fuente de la
// verdad y no hay que adivinar si la coma era decimal o de miles.
var reNumeroCrudo = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// reCrudoAmbiguo es la ÚNICA forma en que el valor crudo no alcanza para decidir: un punto seguido
// de EXACTAMENTE tres dígitos.
//
// En una celda NUMÉRICA «25.000» son veinticinco (con tres decimales). En una celda de TEXTO
// —como quedan los montos que alguien pega desde el banco o teclea a mano— «25.000» son
// VEINTICINCO MIL, que es la notación de Costa Rica y la que ya lee `parseMontoTolerante`.
// Medido contra la API real antes de este arreglo: un débito escrito como texto «25.000» entraba
// a los libros como ₡25,00, sin un solo error en pantalla. Mil veces menos plata, en silencio.
//
// No se puede decidir por la forma del texto —el número 1,005 y el texto «1.005» se escriben
// igual—, así que para estas celdas, y SOLO para estas, se le pregunta a Excel el tipo de la celda
// (ver gridDeArchivoHistorico). Cualquier otra forma da lo mismo por los dos caminos.
var reCrudoAmbiguo = regexp.MustCompile(`^-?\d+\.\d{3}$`)

// fechaDeSerial entiende el número de días de Excel.
//
// `date1904` es la ÉPOCA del libro, y hay que respetarla o las fechas salen CUATRO AÑOS corridas.
// Un libro guardado con el sistema de fechas 1904 —el viejo default de Excel para Mac, y una
// casilla que cualquiera puede marcar en Opciones— cuenta los días desde 1904-01-01 en vez de
// 1899-12-30. Medido: el 07/03/2025 de un libro 1904 se guarda con serial 44261, que leído como
// 1900 da 2021-03-06. La celda SE VE bien en Excel y el movimiento entra en el año equivocado sin
// un solo aviso, que es la peor forma de perder plata acá.
//
// El rango 20000–80000 son los años 1954 a 2119 (1958 a 2123 en la época 1904): afuera de ahí es
// más probable que sea un monto o un número de documento que una fecha, y tomarlo por fecha sería
// peor que no entenderlo.
func fechaDeSerial(v string, date1904 bool) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" || strings.ContainsAny(v, "/-") {
		return time.Time{}, false
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
	if err != nil || n < 20000 || n > 80000 {
		return time.Time{}, false
	}
	// ExcelDateToTime de excelize y no una resta a mano: maneja el año 1900 bisiesto que Excel
	// inventó y que corre todas las fechas anteriores a marzo de 1900 un día.
	t, err := excelize.ExcelDateToTime(n, date1904)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
}

// anioDeDosCifras completa «26» → 2026 y «99» → 1999. El corte en 70 es la convención de Windows y
// de Excel; acá no hay estados de cuenta anteriores a 1970 ni posteriores a 2069.
func anioDeDosCifras(n int, digitos int) int {
	if digitos > 2 {
		return n
	}
	if n < 70 {
		return 2000 + n
	}
	return 1900 + n
}

// fechaEscrita entiende una fecha escrita como TEXTO, con día y mes de uno o dos dígitos.
//
// La convención es dd/mm/aaaa (Costa Rica, y lo que escribe el reporte del sistema). Solo se lee
// al revés cuando el primer grupo tiene cuatro dígitos, que es el ISO «2026-01-02».
func fechaEscrita(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	if m := reFechaNumerica.FindStringSubmatch(v); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		c, _ := strconv.Atoi(m[3])
		dia, mes, anio := a, b, anioDeDosCifras(c, len(m[3]))
		if len(m[1]) == 4 {
			anio, mes, dia = a, b, c
		}
		return fechaDeCalendario(anio, mes, dia)
	}
	if m := reFechaConMes.FindStringSubmatch(v); m != nil {
		dia, _ := strconv.Atoi(m[1])
		nombre := strings.ToUpper(acentos.Replace(m[2]))
		if len(nombre) < 3 {
			return time.Time{}, false
		}
		mes, ok := mesesAbrev[nombre[:3]]
		if !ok {
			return time.Time{}, false
		}
		anio, _ := strconv.Atoi(m[3])
		return fechaDeCalendario(anioDeDosCifras(anio, len(m[3])), int(mes), dia)
	}
	return time.Time{}, false
}

// fechaDeCalendario rechaza el 31 de febrero en vez de dejar que time.Date lo corra al 3 de marzo,
// que es un movimiento con fecha inventada y nadie se entera.
func fechaDeCalendario(anio, mes, dia int) (time.Time, bool) {
	if mes < 1 || mes > 12 || dia < 1 || dia > 31 || anio < 1900 || anio > 2200 {
		return time.Time{}, false
	}
	t := time.Date(anio, time.Month(mes), dia, 0, 0, 0, 0, time.UTC)
	if t.Day() != dia || int(t.Month()) != mes || t.Year() != anio {
		return time.Time{}, false
	}
	return t, true
}

// fechaDeCeldaHistorico resuelve la fecha mirando PRIMERO el valor crudo.
//
// Este es EL caso que rompió el intento del Director: una celda que Excel guarda como fecha de
// verdad se formatea como «07-03-26», y con el formateado no hay forma de saber si el 07 es el día
// o el mes. El crudo es el serial y no tiene ambigüedad.
func fechaDeCeldaHistorico(crudo, formateado string, date1904 bool) (time.Time, bool) {
	if t, ok := fechaDeSerial(crudo, date1904); ok {
		return t, true
	}
	if t, ok := fechaEscrita(crudo); ok {
		return t, true
	}
	if t, ok := fechaEscrita(formateado); ok {
		return t, true
	}
	return fechaDeSerial(formateado, date1904)
}

// parseMontoHistorico lee un monto tratando el PARÉNTESIS como negativo.
//
// `parseMontoTolerante` borra todo lo que no sea dígito o separador, así que «(1.500,00)» le sale
// +1500. Para el lector que solo pinta etiquetas eso era una fila que no calzaba; acá crearía un
// movimiento con el signo al revés, y eso no se nota hasta que no cuadra el mes.
func parseMontoHistorico(s string) (decimal.Decimal, error) {
	t := strings.TrimSpace(s)
	negativo := strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")")
	if negativo {
		t = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(t, "("), ")"))
	}
	d, err := parseMontoTolerante(t)
	if err != nil {
		return decimal.Zero, err
	}
	if negativo {
		return d.Neg(), nil
	}
	return d, nil
}

// montoDeCelda prefiere el valor CRUDO cuando la celda es un número de verdad: ahí no hay que
// adivinar si la coma era decimal o de miles, porque Excel guarda siempre con punto.
func montoDeCelda(crudo, formateado string) (decimal.Decimal, error) {
	if c := strings.TrimSpace(crudo); c != "" && reNumeroCrudo.MatchString(c) {
		return decimal.NewFromString(c)
	}
	return parseMontoHistorico(formateado)
}

// decimalesDeLosLibros es la precisión con la que la base guarda la plata: numeric(16,2).
const decimalesDeLosLibros = 2

// maxMontoDeLosLibros es el monto más grande que entra en numeric(16,2): catorce enteros y dos
// decimales.
//
// Sin este freno, una celda con ceros de más no rompe la fila: rompe el INSERT de TODO el archivo,
// la transacción se va entera y el usuario recibe un «error interno» que no nombra ni una línea.
// Medido: un débito de 99999999999999999,99 devolvía 500 en `confirmar` y nada quedaba cargado.
var maxMontoDeLosLibros = decimal.RequireFromString("99999999999999.99")

// aLaPrecisionDeLosLibros redondea a dos decimales ANTES de contar nada.
//
// Postgres redondea igual al insertar en numeric(16,2), así que si acá no se redondea, el resumen
// que el usuario aprueba y la plata que termina en la base son DOS NÚMEROS DISTINTOS. Medido: tres
// filas de 1,005 daban «1,005 × 3 = 3,02» en la previsualización y 1,01 × 3 = 3,03 en la base, y
// dos filas de 0,004 entraban como movimientos de ¢0,00 sin decir nada. Redondeando acá, la huella,
// el resumen, el evento de auditoría y la base cuentan todos la misma plata, y una fila que se
// vuelve cero sale como «no trae monto», que es lo que de verdad es.
//
// El redondeo es medio-arriba (`Round`), el mismo de numeric en Postgres.
func aLaPrecisionDeLosLibros(d decimal.Decimal) decimal.Decimal {
	return d.Round(decimalesDeLosLibros)
}

func noEntraEnLosLibros(d decimal.Decimal) bool {
	return d.Abs().GreaterThan(maxMontoDeLosLibros)
}

// esDecoracionDeReporte reconoce las filas que el reporte del sistema intercala entre los
// movimientos: la banda de cada partida, «Subtotal <partida>», «TOTAL · N movimiento(s)» y el pie.
//
// Sin esto el usuario ve media docena de errores FALSOS en cada archivo que el propio sistema le
// exportó, que es la forma más rápida de que deje de confiar en el resumen.
//
// La etiqueta cae en la PRIMERA columna, que en el reporte es justo la de «Fecha» (el reporte la
// escribe combinada de A a J). Por eso se mira ahí y también en la columna de fecha cuando no es la
// primera: el archivo lo puede editar gente que mueve columnas.
//
// La banda de partida («Ingresos › Depósitos», o «Sin clasificar») no tiene palabra clave que la
// delate: se reconoce porque va SOLA en la fila, y solo cuenta como banda si ese único texto está
// en la primera columna o en la de fecha. Una fila rota con texto suelto en otra columna sigue
// siendo un error, no decoración.
func esDecoracionDeReporte(fila []string, idxFecha int) bool {
	etiquetas := []string{cell(fila, 0)}
	if idxFecha != 0 {
		etiquetas = append(etiquetas, cell(fila, idxFecha))
	}
	for _, txt := range etiquetas {
		n := norm(txt)
		switch {
		case n == "":
		case strings.HasPrefix(n, "subtotal "):
			return true
		case strings.HasPrefix(n, "total") && strings.Contains(n, "movimiento"):
			return true
		case strings.HasPrefix(n, "generado por gpvdp erp"):
			return true
		}
	}
	conTexto, donde := 0, -1
	for j, c := range fila {
		if strings.TrimSpace(c) != "" {
			conTexto++
			donde = j
		}
	}
	return conTexto == 1 && (donde == 0 || donde == idxFecha)
}

func filaVacia(fila []string) bool {
	for _, c := range fila {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// sinClasificar es el texto que el reporte escribe cuando el movimiento no tiene partida. Leerlo
// de vuelta como el NOMBRE de una partida haría que el archivo que el sistema exportó reportara
// «no existe la clasificación Sin clasificar» en cada fila sin clasificar.
const sinClasificar = "sin clasificar"

func nombreDePartida(v string) string {
	if norm(v) == sinClasificar {
		return ""
	}
	return strings.TrimSpace(v)
}

// LeerHistorico interpreta las dos grillas (la formateada y la cruda) y devuelve las filas de
// movimiento, los errores y cuánta decoración se saltó. Función pura: no toca la base.
//
// `date1904` es la época del libro (ver fechaDeSerial): viene del propio archivo y no se adivina.
func LeerHistorico(formateada, cruda Grid, date1904 bool) (LecturaHistorico, error) {
	col, filaEnc := columnasHistorico(formateada)
	if col == nil {
		return LecturaHistorico{}, ErrHistoricoSinEncabezado
	}

	lec := LecturaHistorico{Filas: make([]FilaHistorico, 0, 512), Errores: []ErrorFilaHistorico{}}
	for i := filaEnc + 1; i < len(formateada); i++ {
		fila := formateada[i]
		if filaVacia(fila) {
			continue
		}
		celda := func(papel string) string {
			idx, ok := col[papel]
			if !ok {
				return ""
			}
			return strings.TrimSpace(cell(fila, idx))
		}
		crudo := func(papel string) string {
			idx, ok := col[papel]
			if !ok || i >= len(cruda) {
				return ""
			}
			return strings.TrimSpace(cell(cruda[i], idx))
		}

		// La decoración se descarta ANTES de exigir una fecha, y no después: en el reporte la
		// etiqueta del subtotal cae DENTRO de la columna «Fecha», así que preguntar primero «¿tiene
		// fecha?» la dejaría entrar como fila rota.
		fechaTxt := celda("fecha")
		fecha, ok := fechaDeCeldaHistorico(crudo("fecha"), fechaTxt, date1904)
		if !ok {
			if esDecoracionDeReporte(fila, col["fecha"]) {
				lec.LineasDeFormato++
				continue
			}
			motivo := "no se entiende la fecha (se espera dd/mm/aaaa)"
			texto := fechaTxt
			if fechaTxt == "" {
				motivo = "la fila no trae fecha y no es una línea de formato del reporte"
				texto = primerTextoDeFila(fila)
			}
			lec.Errores = append(lec.Errores, ErrorFilaHistorico{Linea: i + 1, Motivo: motivo, Texto: texto})
			continue
		}

		if len(lec.Filas) >= maxFilasHistorico {
			return LecturaHistorico{}, ErrHistoricoDemasiadasFilas
		}

		deb, errD := montoDeCelda(crudo("debito"), celda("debito"))
		cre, errC := montoDeCelda(crudo("credito"), celda("credito"))
		textoMontos := strings.TrimSpace(celda("debito") + " / " + celda("credito"))
		if errD == nil && errC == nil {
			// A la precisión de los libros ANTES de sumar, de armar la huella y de contar: ver
			// aLaPrecisionDeLosLibros. Lo que se muestra tiene que ser lo que se escribe.
			deb, cre = aLaPrecisionDeLosLibros(deb), aLaPrecisionDeLosLibros(cre)
		}
		switch {
		case errD != nil || errC != nil:
			lec.Errores = append(lec.Errores, ErrorFilaHistorico{
				Linea: i + 1, Motivo: "el débito o el crédito no se entienden como monto", Texto: textoMontos,
			})
			continue
		case deb.IsNegative() || cre.IsNegative():
			// Acá cae el «(1.500,00)». El sentido lo da la COLUMNA, no el signo: un negativo en la
			// columna de débito querría decir «en realidad fue un crédito», y adivinar eso por el
			// usuario es meter plata al revés en la mitad de los casos.
			lec.Errores = append(lec.Errores, ErrorFilaHistorico{
				Linea: i + 1,
				Motivo: "monto negativo (los paréntesis de Excel también son negativo): el sentido lo da " +
					"la columna, pasalo a la otra columna en positivo",
				Texto: textoMontos,
			})
			continue
		case noEntraEnLosLibros(deb) || noEntraEnLosLibros(cre):
			// Se frena ACÁ y no en el INSERT: allá el error se lleva puesta la transacción entera y
			// el archivo completo se cae con un «error interno» que no nombra la fila.
			lec.Errores = append(lec.Errores, ErrorFilaHistorico{
				Linea: i + 1,
				Motivo: "el monto no entra en los libros (el máximo es " + maxMontoDeLosLibros.StringFixed(2) +
					"): fijate si le sobran ceros",
				Texto: textoMontos,
			})
			continue
		case deb.IsPositive() && cre.IsPositive():
			lec.Errores = append(lec.Errores, ErrorFilaHistorico{
				Linea: i + 1, Motivo: "débito y crédito a la vez en la misma fila", Texto: textoMontos,
			})
			continue
		case deb.IsZero() && cre.IsZero():
			lec.Errores = append(lec.Errores, ErrorFilaHistorico{
				Linea: i + 1, Motivo: "la fila no trae monto ni en débito ni en crédito", Texto: textoMontos,
			})
			continue
		}

		lec.Filas = append(lec.Filas, FilaHistorico{
			Linea:         i + 1,
			Cuenta:        celda("cuenta"),
			Fecha:         fecha,
			Documento:     celda("documento"),
			Descripcion:   celda("descripcion"),
			Debito:        deb,
			Credito:       cre,
			Concepto:      nombreDePartida(celda("concepto")),
			Clasificacion: nombreDePartida(celda("clasificacion")),
		})
	}

	if len(lec.Filas) == 0 && len(lec.Errores) == 0 {
		return LecturaHistorico{}, ErrHistoricoVacio
	}
	return lec, nil
}

// primerTextoDeFila devuelve algo reconocible de una fila que no se pudo leer, para que el usuario
// la encuentre en su Excel sin contar líneas.
func primerTextoDeFila(fila []string) string {
	for _, c := range fila {
		if t := strings.TrimSpace(c); t != "" {
			if len(t) > 120 {
				return t[:120] + "…"
			}
			return t
		}
	}
	return ""
}

// taparCrudosAmbiguos borra de la grilla CRUDA los valores que no alcanzan para decidir solos, para
// que el lector caiga en el texto formateado y lo lea con la convención de Costa Rica.
//
// El caso es uno solo: un punto seguido de tres dígitos (ver reCrudoAmbiguo). Ahí «25.000» puede
// ser el número veinticinco-con-tres-decimales o el TEXTO «veinticinco mil», y las dos cosas se
// escriben igual. Lo único que las distingue es el TIPO de la celda, así que se le pregunta a
// Excel —y solo para esas celdas, que en el reporte del propio sistema son cero, así que no cuesta
// nada—. Si la celda es texto, el valor crudo no es un número: se tapa y decide parseMontoTolerante.
//
// Se tapa en vez de convertir acá porque la conversión ya existe y está probada en un solo lugar;
// duplicarla sería dejar dos reglas de miles que pueden separarse.
func taparCrudosAmbiguos(f *excelize.File, hoja string, raw [][]string) {
	for i := range raw {
		for j, v := range raw[i] {
			if !reCrudoAmbiguo.MatchString(strings.TrimSpace(v)) {
				continue
			}
			celda, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				continue
			}
			t, err := f.GetCellType(hoja, celda)
			if err != nil {
				continue
			}
			if t == excelize.CellTypeSharedString || t == excelize.CellTypeInlineString {
				raw[i][j] = ""
			}
		}
	}
}

// gridDeArchivoHistorico abre el libro y devuelve la hoja preferida DOS veces: formateada (lo que
// se ve en pantalla) y cruda (lo que Excel guarda).
//
// Las dos hacen falta: el texto de la descripción y de la partida solo sirve formateado, y la
// fecha y los montos solo son inequívocos en crudo.
// Devuelve además la ÉPOCA del libro (`date1904`), que es la que le da sentido a los seriales de
// la grilla cruda: sin ella un libro guardado con el sistema de fechas 1904 entra con todo corrido
// cuatro años (ver fechaDeSerial).
func gridDeArchivoHistorico(archivo []byte, preferida string) (formateada, cruda Grid, hoja string, hojas []string, date1904 bool, err error) {
	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		// El error de excelize se conserva envuelto para el log, pero el centinela es el que le da
		// al usuario un 400 con qué hacer en vez de un 500 «error interno».
		return nil, nil, "", nil, false, fmt.Errorf("%w (%v)", ErrHistoricoArchivoIlegible, err)
	}
	defer func() { _ = f.Close() }()

	hojas = f.GetSheetList()
	if len(hojas) == 0 {
		return nil, nil, "", nil, false, fmt.Errorf("%w: el libro no tiene ninguna hoja", ErrHistoricoArchivoIlegible)
	}
	hoja = hojas[0]
	for _, h := range hojas {
		if strings.EqualFold(strings.TrimSpace(h), preferida) {
			hoja = h
			break
		}
	}
	// Si no se puede leer la propiedad, se asume 1900, que es el default de Excel: es el mismo
	// supuesto de antes y no empeora nada.
	if props, e := f.GetWorkbookProps(); e == nil && props.Date1904 != nil {
		date1904 = *props.Date1904
	}
	rows, err := f.GetRows(hoja)
	if err != nil {
		return nil, nil, "", nil, false, fmt.Errorf("bancos: leer filas: %w", err)
	}
	raw, err := f.GetRows(hoja, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, nil, "", nil, false, fmt.Errorf("bancos: leer filas sin formato: %w", err)
	}
	taparCrudosAmbiguos(f, hoja, raw)
	return Grid(rows), Grid(raw), hoja, hojas, date1904, nil
}
