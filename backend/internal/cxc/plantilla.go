package cxc

// La plantilla de importación de cartera: el machote que se baja, se llena y se sube.
//
// Existe porque el archivo que trae el sistema de origen no siempre tiene las columnas con los
// nombres que el lector reconoce, y cuando no las tiene NO FALLA: las filas entran con los campos
// vacíos y caen en cuarentena en bloque. Una corrida real entró con 23.144 filas, 23.144 en
// cuarentena y 23.144 sin sede — todas, porque la columna de sede no se llamaba como ninguno de sus
// alias y «sin sede» es uno de los motivos que marcan la fila.
//
// El libro lleva TRES hojas y cada una contesta una pregunta distinta:
//   · «Contratos»     — los encabezados exactos, listos para pegar los datos debajo.
//   · «Cómo llenarla» — qué significa cada columna, si es obligatoria y qué pasa si falta.
//   · «Valores válidos» — las sedes, modalidades y formas de pago de ESTA empresa, leídas del
//     catálogo. No se inventan: escribir una modalidad que no existe deja el contrato sin cobrar.

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// Las tres hojas del libro.
const (
	hojaContratos = "Contratos"
	hojaAyuda     = "Cómo llenarla"
	hojaValores   = "Valores válidos"
)

// columnaPlantilla describe una columna del machote para la hoja de ayuda.
type columnaPlantilla struct {
	// Encabezado es el nombre que SE ESCRIBE en la plantilla. Sale de `aliasContrato`, así que lo
	// que la plantilla propone es exactamente lo que el lector reconoce.
	Encabezado  string
	Campo       string // la clave en aliasContrato, para poder cruzarlo en la prueba
	Obligatoria string
	Formato     string
	SiFalta     string
}

// columnasDeLaPlantilla es el contrato de la importación, en el orden en que conviene leerlo: lo
// que identifica al contrato, lo que decide cuánto y cuándo se cobra, y al final lo informativo.
//
// El orden NO importa para el importador —resuelve por encabezado, no por posición— pero sí para
// quien llena el archivo a mano.
var columnasDeLaPlantilla = []columnaPlantilla{
	{"Contrato", "contrato", "SÍ", "Texto tal cual el sistema de origen: «CO198456», «CD-0000000561»",
		"La fila SE SALTA sin avisar: sin número no hay contrato que crear ni actualizar"},
	{"Cliente", "cliente", "Sí", "Nombre completo", "El contrato entra sin nombre y no se puede llamar a nadie"},
	{"Documento", "documento", "Sí", "Cédula, sin formato obligatorio", "Entra vacío; es el dato con que se cruza contra otros sistemas"},
	{"Teléfonos", "telefonos", "Sí", "Uno o varios, separados por coma", "Entra vacío y el contrato no es gestionable por teléfono"},
	{"Otro teléfono", "otro_telefono", "No", "Se junta con el anterior", "Nada"},
	{"Correos", "correos", "No", "Uno o varios, separados por coma", "Nada"},
	{"Sede", "sede", "SÍ", "«Razón Social - Plaza» (ej. «Valle de Paz S.A. - Cartago»), o solo la plaza",
		"CUARENTENA: queda sin sede operativa y nadie lo tiene asignado"},
	{"Cuota servicio", "cuota", "SÍ", "Número. Punto o coma decimal, con o sin separador de miles",
		"CUARENTENA si está ilegible o pasa el máximo configurado en Parámetros"},
	{"Modalidad de cobro", "modalidad", "SÍ", "Ver la hoja «Valores válidos»",
		"CUARENTENA: sin modalidad no se sabe cada cuánto cobrar y no se genera ningún cargo"},
	{"Contrato fecha primer cobro", "primer_cobro", "SÍ", "d/m/aaaa",
		"CUARENTENA si tampoco hay «Fecha próximo pago»: sin una de las dos no se genera ni un cargo"},
	{"Fecha próximo pago", "proximo_pago", "No", "d/m/aaaa", "Se usa como primer cobro si ese falta (queda marcado)"},
	{"Días de pagos", "dia_pago", "Sí", "Número del 1 al 31", "Entra sin día fijo; si se escribe fuera de rango va a CUARENTENA"},
	{"Forma de pago", "forma_pago", "Sí", "Ver la hoja «Valores válidos»", "Entra vacío: no se sabe por dónde cobrarle"},
	{"Contrato fecha inicial", "fecha_inicial", "No", "d/m/aaaa", "Nada; es informativa"},
	{"Servicios", "servicio", "No", "Texto", "Nada"},
	{"Tipo de servicio", "tipo_servicio", "No", "Texto", "Nada"},
	{"Asociación", "asociacion", "No", "Nombre de la asociación solidarista", "Nada; hace falta si la forma de pago es por asociación"},
	{"Fecha vencimiento tarjeta", "tarjeta_vence", "No", "mes-año («oct-28»); se toma el último día del mes", "Nada"},
	{"Estado", "estado", "No", "Texto del sistema de origen", "Nada; es informativo"},
	{"Estatus de morosidad", "morosidad", "No", "Texto del sistema de origen", "Nada; el ERP calcula su propio tramo"},
	{"Días vencidos", "dias_vencidos", "No", "Número", "Nada; el ERP calcula los suyos con los cargos reales"},
	{"Score", "score", "No", "Número", "Nada; informativo para comparar contra el sistema viejo"},
	{"Saldo pendiente", "saldo", "No", "Número", "Nada; informativo. El saldo del ERP sale de sus propios cargos"},
}

// filaEjemplo es una muestra de cada columna, en el mismo orden. Va en la hoja de AYUDA y no debajo
// del encabezado de «Contratos», aunque ahí se vería mejor: una fila de ejemplo en la hoja de datos
// es un contrato más para el importador, y quien olvide borrarla mete «CO198456» en la cartera. Lo
// detectó la prueba que relee la plantilla con el lector de verdad: leyó DOS filas donde había una.
var filaEjemplo = []string{
	"CO198456", "MARÍA RODRÍGUEZ SOLÍS", "1-0888-0777", "8888-7777, 2222-3333", "", "maria@ejemplo.com",
	"Valle de Paz S.A. - Cartago", "15000.00", "Mensual", "15/1/2026", "", "15", "Débito Automático",
	"1/1/2026", "Plan Familiar", "Prepago", "", "oct-28", "Activo", "Al día", "0", "750", "0.00",
}

// PlantillaCartera arma el .xlsx del machote con los catálogos de la empresa.
func PlantillaCartera(cat Catalogos) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	if err := hojaDeContratos(f); err != nil {
		return nil, err
	}
	if err := hojaDeAyuda(f); err != nil {
		return nil, err
	}
	if err := hojaDeValores(f, cat); err != nil {
		return nil, err
	}
	f.DeleteSheet("Sheet1")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("cxc: escribir plantilla: %w", err)
	}
	return buf.Bytes(), nil
}

func hojaDeContratos(f *excelize.File) error {
	idx, err := f.NewSheet(hojaContratos)
	if err != nil {
		return fmt.Errorf("cxc: hoja de contratos: %w", err)
	}
	f.SetActiveSheet(idx)

	negrita, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"0F6E4E"}},
	})

	for i, c := range columnasDeLaPlantilla {
		celda, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellStr(hojaContratos, celda, c.Encabezado)
		// Ancho a ojo del encabezado: no hace falta ser exacto, alcanza con que no salga cortado.
		col, _ := excelize.ColumnNumberToName(i + 1)
		ancho := float64(len(c.Encabezado)) + 4
		if ancho < 12 {
			ancho = 12
		}
		_ = f.SetColWidth(hojaContratos, col, col, ancho)
	}
	_ = f.SetRowStyle(hojaContratos, 1, 1, negrita)

	// La hoja queda SOLO con el encabezado: los datos se pegan desde la fila 2 y lo que haya debajo
	// es un contrato. Ni ejemplos ni avisos acá —cualquier texto en la columna A entra como el
	// número de un contrato—; eso va en «Cómo llenarla».

	// Encabezado congelado: con 23 000 filas, perder de vista los nombres es perder el archivo.
	_ = f.SetPanes(hojaContratos, &excelize.Panes{
		Freeze: true, Split: false, XSplit: 0, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	})
	return nil
}

func hojaDeAyuda(f *excelize.File) error {
	if _, err := f.NewSheet(hojaAyuda); err != nil {
		return fmt.Errorf("cxc: hoja de ayuda: %w", err)
	}
	negrita, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"0F6E4E"}},
	})
	ajuste, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})

	_ = f.SetCellStr(hojaAyuda, "A1", "Columna")
	_ = f.SetCellStr(hojaAyuda, "B1", "¿Obligatoria?")
	_ = f.SetCellStr(hojaAyuda, "C1", "Ejemplo")
	_ = f.SetCellStr(hojaAyuda, "D1", "Cómo se escribe")
	_ = f.SetCellStr(hojaAyuda, "E1", "Qué pasa si falta")
	_ = f.SetRowStyle(hojaAyuda, 1, 1, negrita)

	for i, c := range columnasDeLaPlantilla {
		fila := i + 2
		_ = f.SetCellStr(hojaAyuda, fmt.Sprintf("A%d", fila), c.Encabezado)
		_ = f.SetCellStr(hojaAyuda, fmt.Sprintf("B%d", fila), c.Obligatoria)
		if i < len(filaEjemplo) {
			_ = f.SetCellStr(hojaAyuda, fmt.Sprintf("C%d", fila), filaEjemplo[i])
		}
		_ = f.SetCellStr(hojaAyuda, fmt.Sprintf("D%d", fila), c.Formato)
		_ = f.SetCellStr(hojaAyuda, fmt.Sprintf("E%d", fila), c.SiFalta)
	}
	_ = f.SetColWidth(hojaAyuda, "A", "A", 30)
	_ = f.SetColWidth(hojaAyuda, "B", "B", 14)
	_ = f.SetColWidth(hojaAyuda, "C", "C", 28)
	_ = f.SetColWidth(hojaAyuda, "D", "D", 55)
	_ = f.SetColWidth(hojaAyuda, "E", "E", 60)
	_ = f.SetCellStyle(hojaAyuda, "D2", fmt.Sprintf("E%d", len(columnasDeLaPlantilla)+1), ajuste)

	// Las tres cosas que hay que saber antes de llenar nada.
	notas := []string{
		"",
		"LO QUE CONVIENE SABER ANTES DE LLENARLA",
		"· Pegá tus datos en la hoja «Contratos» desde la fila 2, debajo del encabezado. La columna «Ejemplo» de acá es solo de muestra: NO la copies a la hoja de datos, porque cualquier fila con número de contrato entra como un contrato más.",
		"· Las columnas se resuelven por ENCABEZADO, no por posición: podés reordenarlas o agregar columnas de más, que se ignoran.",
		"· Podés dejar el encabezado que ya trae tu archivo si significa lo mismo (por ejemplo «Sucursal» en vez de «Sede»): el importador acepta varios nombres por columna.",
		"· Ninguna fila se descarta en silencio, con UNA excepción: la que viene sin número de contrato.",
		"· «Cuarentena» no es un rechazo: el contrato ENTRA, marcado, y queda en una lista para revisar.",
		"· El ERP no importa saldos ni días vencidos para calcular: los GENERA de los cargos. Esas columnas viajan solo para comparar contra el sistema de origen.",
	}
	base := len(columnasDeLaPlantilla) + 3
	for i, n := range notas {
		_ = f.SetCellStr(hojaAyuda, fmt.Sprintf("A%d", base+i), n)
	}
	tituloNota, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	_ = f.SetCellStyle(hojaAyuda, fmt.Sprintf("A%d", base+1), fmt.Sprintf("A%d", base+1), tituloNota)
	return nil
}

func hojaDeValores(f *excelize.File, cat Catalogos) error {
	if _, err := f.NewSheet(hojaValores); err != nil {
		return fmt.Errorf("cxc: hoja de valores: %w", err)
	}
	negrita, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"0F6E4E"}},
	})

	_ = f.SetCellStr(hojaValores, "A1", "Sedes")
	_ = f.SetCellStr(hojaValores, "B1", "Modalidades de cobro")
	_ = f.SetCellStr(hojaValores, "C1", "Formas de pago")
	_ = f.SetCellStr(hojaValores, "D1", "Asociaciones")
	_ = f.SetRowStyle(hojaValores, 1, 1, negrita)

	columnas := []struct {
		letra string
		items []ItemCatalogo
	}{
		{"A", cat.Sedes},
		{"B", cat.Modalidades},
		{"C", cat.FormasPago},
		{"D", cat.Asociaciones},
	}
	for _, col := range columnas {
		for i, it := range col.items {
			_ = f.SetCellStr(hojaValores, fmt.Sprintf("%s%d", col.letra, i+2), it.Nombre)
		}
		_ = f.SetColWidth(hojaValores, col.letra, col.letra, 34)
	}

	// Una empresa recién abierta no tiene catálogo todavía, y una hoja vacía se lee como «no hay
	// que llenar esto». Se dice de dónde salen.
	fin := 2
	for _, col := range columnas {
		if n := len(col.items) + 2; n > fin {
			fin = n
		}
	}
	_ = f.SetCellStr(hojaValores, fmt.Sprintf("A%d", fin+1),
		"Estos valores salen del catálogo de CxC de esta empresa. Si el que necesitás no está, cargalo primero en el ERP: un valor que no existe deja el contrato sin cobrar.")
	return nil
}
