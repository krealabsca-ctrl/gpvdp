package conta

// El mapeo contable en .xlsx: sacarlo para llenarlo afuera y volverlo a entrar (12-nov-2026).
//
// ── POR QUÉ EXISTE ───────────────────────────────────────────────────────────────────────────
//
// Quien sabe a qué cuenta va cada partida es el contador, y hasta hoy no había forma de sacarle
// la lista: el mapeo solo se podía decidir adentro del sistema, pantalla por pantalla. Es el mismo
// problema que ya resolvió el diccionario del catálogo de Bancos, y la misma solución: el trabajo
// se arma en Excel —donde la gente trabaja— y se sube de una vez.
//
// ── POR QUÉ NO HAY SUGERENCIA AUTOMÁTICA ─────────────────────────────────────────────────────
//
// Lo primero que se intentó fue proponer la cuenta comparando nombres, como `SugerirPorIBAN` hace
// con las cuentas bancarias. Medido contra el plan real de Valle de Paz: de 131 partidas con
// movimiento, solo SEIS calzan exacto. De los 29 parciales, «Alquiler» devuelve NUEVE candidatos
// —y entre ellos hay cuentas de ingreso (Alquiler de Capillas) y de activo (Depósito en Garantía)
// mezcladas con las de gasto—. Una sugerencia así se acepta en lote y produce un error contable
// que nadie revisa. Se descartó: el archivo sale con la columna VACÍA.
//
// ── LA LLAVE SON LOS NOMBRES, Y ESO ES SEGURO ────────────────────────────────────────────────
//
// Cada fila se reconoce por (Concepto, Partida) y no por el id: un UUID de 36 caracteres en una
// hoja que alguien edita a mano se corrompe o se pierde. La pareja es única POR CONSTRUCCIÓN, no
// por casualidad: la base tiene `concepto_empresa_id_nombre_key` sobre (empresa, nombre) y
// `clasificacion_empresa_id_concepto_id_nombre_key` sobre (empresa, concepto_id, nombre).
//
// ── EL PLAN VIAJA EN LA SEGUNDA HOJA ─────────────────────────────────────────────────────────
//
// Sin él, llenar la columna de código obliga a tener el plan abierto en otro lado y a transcribir
// códigos de 15 caracteres de memoria. Con 211 cuentas imputables, eso es el error esperándote.

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

const (
	hojaMapeo = "Mapeo"
	hojaPlan  = "Plan de cuentas"
)

// columnasMapeoXLSX son los títulos de la hoja de trabajo, en orden.
//
// El MONTO va tercero, antes que la cantidad de movimientos, por la misma razón que ordena la
// pantalla: es el número con el que se decide a qué partidas dedicarles tiempo.
var columnasMapeoXLSX = []string{
	"Concepto", "Partida", "Colones", "Movimientos", "Naturaleza en el ERP",
	"Código de cuenta", "Motivo si no se contabiliza", "Estado actual",
}

// NombreMapeoXLSX nombra el archivo.
func NombreMapeoXLSX() string { return "mapeo-contable.xlsx" }

// ConstruirMapeoXLSX arma el libro: la hoja de trabajo y el plan de cuentas al lado.
//
// El archivo NO dice de qué empresa es, y es una decisión: el claim del token no trae el nombre y
// agregarlo al repositorio solo para rotularlo costaría más de lo que vale. El riesgo —subir el
// archivo de una empresa estando en otra— ya está cubierto donde importa: la importación resuelve
// cada fila contra las partidas de la empresa activa y rechaza las que no existen, nombrándolas.
// Un archivo cruzado se rechaza entero y con motivo, no se aplica a medias.
func ConstruirMapeoXLSX(filas []FilaDelMapeo, cuentas []Cuenta) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	if err := f.SetSheetName("Sheet1", hojaMapeo); err != nil {
		return nil, fmt.Errorf("conta: nombrar hoja: %w", err)
	}

	encabezado, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#1F3864"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
	})
	if err != nil {
		return nil, fmt.Errorf("conta: estilo de encabezado: %w", err)
	}
	// Las dos columnas que se llenan van resaltadas: en una hoja de ocho columnas, cuál hay que
	// escribir no puede depender de que alguien lea el instructivo.
	porLlenar, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#FFF2CC"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
	})
	if err != nil {
		return nil, fmt.Errorf("conta: estilo de columna a llenar: %w", err)
	}
	fmtMonto := "#,##0.00"
	monto, err := f.NewStyle(&excelize.Style{
		CustomNumFmt: &fmtMonto,
		Alignment:    &excelize.Alignment{Horizontal: "right"},
	})
	if err != nil {
		return nil, fmt.Errorf("conta: estilo de monto: %w", err)
	}

	anchos := []struct {
		col string
		w   float64
	}{{"A", 22}, {"B", 34}, {"C", 16}, {"D", 13}, {"E", 20}, {"F", 20}, {"G", 36}, {"H", 18}}
	for _, a := range anchos {
		if err := f.SetColWidth(hojaMapeo, a.col, a.col, a.w); err != nil {
			return nil, fmt.Errorf("conta: ancho de columna: %w", err)
		}
	}

	for i, c := range columnasMapeoXLSX {
		celda, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return nil, fmt.Errorf("conta: celda de cabecera: %w", err)
		}
		if err := f.SetCellValue(hojaMapeo, celda, c); err != nil {
			return nil, err
		}
		estilo := encabezado
		if i == 5 || i == 6 { // «Código de cuenta» y «Motivo…»
			estilo = porLlenar
		}
		if err := f.SetCellStyle(hojaMapeo, celda, celda, estilo); err != nil {
			return nil, err
		}
	}
	_ = f.SetRowHeight(hojaMapeo, 1, 30)

	for i, fi := range filas {
		fila := i + 2
		valores := []any{
			fi.Concepto, fi.Clasificacion, nil, fi.Movimientos,
			naturalezaLegible(fi), fi.CuentaCodigo, fi.Motivo, estadoLegible(fi.Estado),
		}
		for j, v := range valores {
			celda, err := excelize.CoordinatesToCellName(j+1, fila)
			if err != nil {
				return nil, fmt.Errorf("conta: celda de fila: %w", err)
			}
			if j == 2 {
				// El monto va como NÚMERO: quien recibe la hoja la ordena por esta columna para
				// decidir por dónde empezar, y en texto no ordena.
				if v, ok := floatDeTexto(fi.MontoCRC); ok {
					if err := f.SetCellValue(hojaMapeo, celda, v); err != nil {
						return nil, err
					}
				}
				if err := f.SetCellStyle(hojaMapeo, celda, celda, monto); err != nil {
					return nil, err
				}
				continue
			}
			if v != nil && v != "" {
				if err := f.SetCellValue(hojaMapeo, celda, v); err != nil {
					return nil, err
				}
			}
		}
	}

	if err := construirHojaPlan(f, cuentas, encabezado); err != nil {
		return nil, err
	}

	// La cabecera queda fija: la hoja tiene cientos de filas y sin esto se pierde de vista cuál
	// columna es cuál justo cuando hay que escribir en la sexta.
	_ = f.SetPanes(hojaMapeo, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	})
	_ = f.SetSheetView(hojaMapeo, 0, &excelize.ViewOptions{ShowGridLines: &[]bool{true}[0]})

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("conta: serializar el mapeo: %w", err)
	}
	return buf.Bytes(), nil
}

// construirHojaPlan deja el plan a la vista, solo con las cuentas que ADMITEN movimiento.
//
// Las de agrupación se dejan fuera a propósito: no se pueden usar en un mapeo —la base lo rechaza
// con un disparador— así que ofrecerlas solo produce intentos que van a fallar.
func construirHojaPlan(f *excelize.File, cuentas []Cuenta, encabezado int) error {
	if _, err := f.NewSheet(hojaPlan); err != nil {
		return fmt.Errorf("conta: crear hoja del plan: %w", err)
	}
	for _, a := range []struct {
		col string
		w   float64
	}{{"A", 20}, {"B", 50}, {"C", 14}} {
		if err := f.SetColWidth(hojaPlan, a.col, a.col, a.w); err != nil {
			return fmt.Errorf("conta: ancho del plan: %w", err)
		}
	}
	for i, c := range []string{"Código de cuenta", "Nombre", "Clase"} {
		celda, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(hojaPlan, celda, c); err != nil {
			return err
		}
		if err := f.SetCellStyle(hojaPlan, celda, celda, encabezado); err != nil {
			return err
		}
	}
	fila := 2
	for _, c := range cuentas {
		if !c.AceptaMovimiento || !c.Activo {
			continue
		}
		for j, v := range []string{c.Codigo, c.Nombre, c.Clase} {
			celda, _ := excelize.CoordinatesToCellName(j+1, fila)
			if err := f.SetCellValue(hojaPlan, celda, v); err != nil {
				return err
			}
		}
		fila++
	}
	_ = f.SetPanes(hojaPlan, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	})
	return nil
}

// naturalezaLegible dice lo que el ERP declara, distinguiendo «es neutro» de «nadie lo decidió».
//
// La columna `naturaleza` nunca viene vacía —su default es NEUTRO— así que escribir «NEUTRO» a
// secas haría creer que alguien lo eligió. En 58 de 64 conceptos nadie lo hizo.
func naturalezaLegible(f FilaDelMapeo) string {
	if !f.NaturalezaDeclarada {
		return "sin declarar"
	}
	return strings.ToLower(f.NaturalezaConcepto)
}

func estadoLegible(e string) string {
	switch e {
	case EstadoMapeada:
		return "mapeada"
	case EstadoNoContabilizable:
		return "no se contabiliza"
	default:
		return "falta decidir"
	}
}

// floatDeTexto convierte el monto que viaja como string. Devuelve false si no es un número, para
// dejar la celda vacía en vez de escribir un cero que se lee como «esta partida no movió nada».
func floatDeTexto(s string) (float64, bool) {
	var v float64
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%g", &v); err != nil {
		return 0, false
	}
	return v, true
}

// FilaMapeoLeida es una fila del archivo, ya normalizada.
type FilaMapeoLeida struct {
	Fila          int    `json:"fila"`
	Concepto      string `json:"concepto"`
	Clasificacion string `json:"clasificacion"`
	CuentaCodigo  string `json:"cuenta_codigo"`
	Motivo        string `json:"motivo"`
}

// LeerMapeoDesdeExcel saca las filas utilizables del archivo.
//
// Lee por NOMBRE DE COLUMNA y no por posición: quien llena la hoja en Excel reordena, esconde o
// agrega columnas sin pensarlo, y una lectura posicional convertiría eso en datos mal puestos sin
// un solo error. Es la misma lección que el importador de cartera de CxC.
func LeerMapeoDesdeExcel(archivo []byte) ([]FilaMapeoLeida, error) {
	f, err := excelize.OpenReader(bytes.NewReader(archivo))
	if err != nil {
		return nil, ErrMapeoArchivoIlegible
	}
	defer func() { _ = f.Close() }()

	// Si no está la hoja «Mapeo» se usa la primera: alguien pudo renombrarla o pegar los datos en
	// una hoja nueva, y eso no debería costar el archivo entero.
	hoja := hojaMapeo
	if idx, err := f.GetSheetIndex(hojaMapeo); err != nil || idx < 0 {
		nombres := f.GetSheetList()
		if len(nombres) == 0 {
			return nil, ErrMapeoArchivoIlegible
		}
		hoja = nombres[0]
	}
	filas, err := f.GetRows(hoja)
	if err != nil || len(filas) < 2 {
		return nil, ErrMapeoSinFilas
	}

	col := map[string]int{}
	for i, c := range filas[0] {
		col[normalizarTitulo(c)] = i
	}
	iConcepto, okC := col["concepto"]
	iPartida, okP := col["partida"]
	if !okC || !okP {
		return nil, ErrMapeoSinEncabezado
	}
	iCuenta, hayCuenta := col["codigo de cuenta"]
	iMotivo, hayMotivo := col["motivo si no se contabiliza"]

	out := make([]FilaMapeoLeida, 0, len(filas))
	for n, fila := range filas[1:] {
		leida := FilaMapeoLeida{
			Fila:          n + 2, // base 1 + la cabecera
			Concepto:      celdaLimpia(fila, iConcepto),
			Clasificacion: celdaLimpia(fila, iPartida),
		}
		if hayCuenta {
			leida.CuentaCodigo = celdaLimpia(fila, iCuenta)
		}
		if hayMotivo {
			leida.Motivo = celdaLimpia(fila, iMotivo)
		}
		if leida.Concepto == "" || leida.Clasificacion == "" {
			continue // fila en blanco o de relleno
		}
		// Sin cuenta Y sin motivo no hay decisión: la fila se dejó como estaba, a propósito.
		// Importarla como «vacía» borraría un mapeo ya hecho.
		if leida.CuentaCodigo == "" && leida.Motivo == "" {
			continue
		}
		out = append(out, leida)
	}
	if len(out) == 0 {
		return nil, ErrMapeoSinFilas
	}
	return out, nil
}

// normalizarTitulo deja el encabezado comparable: sin tildes, sin mayúsculas y sin espacios de más.
func normalizarTitulo(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", " ", " ")
	s = r.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// celdaLimpia reusa `celdaDe` (importar_apertura.go), que ya protege del recorte de filas que hace
// excelize, y le quita los espacios: una celda escrita a mano trae espacios de sobra, y un código
// de cuenta con un espacio al final no calza contra el plan.
func celdaLimpia(fila []string, i int) string {
	return strings.TrimSpace(celdaDe(fila, i))
}
