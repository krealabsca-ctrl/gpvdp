package cxp

// El .xlsx del cronograma, verificado ABRIENDO el archivo generado.
//
// LO QUE SE PROTEGE ES QUE LA CASCADA VIAJE EN EL ARCHIVO. La pantalla ya la muestra; el riesgo es
// que el archivo —que es lo que de verdad se manda por correo— salga con la lista y un total al
// pie. En el cuadro real que esto reemplaza eso comunicaría ₡52,1 M donde el cronograma es
// ₡10,9 M: una cifra cinco veces mayor, perfectamente bien formada, imposible de notar leyendo.
//
// Y que los montos queden como NÚMERO: lo primero que hace quien recibe el cuadro es sumar la
// columna, y un monto escrito como texto no suma ni ordena.

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// cronogramaDeMuestra: dos proveedores que entran, uno de reserva y uno por fuera. Los cuatro
// escalones dan números distintos entre sí, así que ninguno puede confundirse con otro.
func cronogramaDeMuestra() Cronograma {
	return Cronograma{
		LoteNumero: 43, FechaCorte: "2026-11-04",
		Lineas: []LineaCronograma{
			{Proveedor: "Gas Tomza De Costa Rica SA", Moneda: "CRC",
				Monto: "216174.09", MontoColonizado: "216174.09",
				Banco: "Banco Popular", Cuenta: "BP Valle de Paz Colones"},
			{Proveedor: "Daniel Esteban Chaves Arguedas", Moneda: "CRC",
				Monto: "72320.00", MontoColonizado: "72320.00",
				Banco: "BAC", Cuenta: "BAC Valle de Paz Colones"},
			{Proveedor: "Fondo De Mantenimiento", Moneda: "CRC",
				Monto: "100000.00", MontoColonizado: "100000.00",
				Banco: "Banco Popular", Cuenta: "BP Negocios", EsReserva: true},
			{Proveedor: "Municipalidad De Cartago", Moneda: "CRC",
				Monto: "500000.00", MontoColonizado: "500000.00",
				Banco: "BCR", Cuenta: "BCR Valle de Paz", PorFuera: true},
		},
		Totales: TotalesCronograma{
			TotalColones: "888494.09", TotalDolares: "0.00", DolaresColonizados: "0.00",
			TotalColonizado: "888494.09",
			Reserva:         "100000.00", FlujoNormal: "788494.09",
			PorFuera: "500000.00", CronogramaTotal: "288494.09",
			CronogramaEnColones: "288494.09", TipoCambio: "0.0000",
		},
	}
}

func abrirCronograma(t *testing.T, cr Cronograma) *excelize.File {
	t.Helper()
	datos, err := ConstruirCronogramaXLSX(cr, "Valle de Paz", "Administrador GPVDP", "04/11/2026 15:40")
	if err != nil {
		t.Fatalf("construir el cronograma: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(datos))
	if err != nil {
		t.Fatalf("abrir el cronograma: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// filaQueEmpiezaCon localiza una fila por su primera celda. Igual que en el aviso, las pruebas NO
// usan coordenadas fijas: lo que protegen es el contenido, no la maquetación.
//
// Lee los valores CRUDOS: con el formato aplicado, 888494.09 vuelve como «888,494.09» y la prueba
// pasaría a medir el formato de miles en vez del número, que es lo que importa acá.
func filaQueEmpiezaCon(t *testing.T, f *excelize.File, rotulo string) []string {
	t.Helper()
	filas, err := f.GetRows("Cronograma", excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatalf("leer filas: %v", err)
	}
	for _, fila := range filas {
		if len(fila) > 0 && fila[0] == rotulo {
			return fila
		}
	}
	t.Fatalf("el archivo no trae la línea %q", rotulo)
	return nil
}

// LA CASCADA ENTERA VIAJA EN EL ARCHIVO.
func TestElArchivoDelCronogramaTraeLaCascada(t *testing.T) {
	f := abrirCronograma(t, cronogramaDeMuestra())

	// Los cuatro escalones, con el número que le toca a cada uno. Se comprueban TODOS porque el
	// error que importa no es que falte la cascada: es que esté y cualquiera de los pasos traiga el
	// número de otro, y entonces el cuadro cierra y comunica la cifra equivocada.
	for _, caso := range []struct{ rotulo, monto string }{
		{"Total colones", "888494.09"},
		{"TOTAL COLONIZADO", "888494.09"},
		{"menos  Reserva", "100000"},
		{"TOTAL FLUJO NORMAL", "788494.09"},
		{"menos  Solicitudes por fuera", "500000"},
		{"CRONOGRAMA TOTAL FLUJO NORMAL", "288494.09"},
	} {
		fila := filaQueEmpiezaCon(t, f, caso.rotulo)
		// El rótulo ocupa A:C combinadas, así que el monto cae en la cuarta celda.
		if len(fila) < 4 {
			t.Fatalf("la línea %q salió sin monto: %q", caso.rotulo, fila)
		}
		if fila[3] != caso.monto {
			t.Errorf("%q = %q, se esperaba %q", caso.rotulo, fila[3], caso.monto)
		}
	}
}

// El número que se comunica NO es la suma de la columna.
func TestElTotalDelArchivoNoEsLaSumaDeLaLista(t *testing.T) {
	cr := cronogramaDeMuestra()
	f := abrirCronograma(t, cr)

	fila := filaQueEmpiezaCon(t, f, "CRONOGRAMA TOTAL FLUJO NORMAL")
	if fila[3] == "888494.09" {
		t.Fatal("el archivo comunica la suma de la lista (888.494,09) en vez del cronograma " +
			"(288.494,09): está anunciando el triple de lo que se va a girar")
	}
	if fila[3] != "288494.09" {
		t.Errorf("cronograma total = %q, se esperaba 288494.09", fila[3])
	}
}

// Las líneas que no entran tienen que poder distinguirse: si no, la diferencia entre la suma de la
// columna y el total de abajo parece un error de cálculo.
func TestElArchivoDiceCualesLineasNoEntran(t *testing.T) {
	f := abrirCronograma(t, cronogramaDeMuestra())
	filas, err := f.GetRows("Cronograma")
	if err != nil {
		t.Fatalf("leer filas: %v", err)
	}
	estados := map[string]string{}
	for _, fila := range filas {
		if len(fila) >= 7 {
			estados[fila[0]] = fila[6]
		}
	}
	for proveedor, esperado := range map[string]string{
		"Fondo De Mantenimiento":         "Reserva",
		"Municipalidad De Cartago":       "Por fuera",
		"Daniel Esteban Chaves Arguedas": "Entra",
	} {
		if estados[proveedor] != esperado {
			t.Errorf("%s = %q, se esperaba %q", proveedor, estados[proveedor], esperado)
		}
	}
}

// Los montos van como número, o quien recibe el cuadro no puede sumar la columna.
func TestLosMontosDelCronogramaSonNumeros(t *testing.T) {
	f := abrirCronograma(t, cronogramaDeMuestra())
	filas, err := f.GetRows("Cronograma")
	if err != nil {
		t.Fatalf("leer filas: %v", err)
	}
	fila := 0
	for i, fi := range filas {
		if len(fi) > 0 && fi[0] == "Gas Tomza De Costa Rica SA" {
			fila = i + 1
			break
		}
	}
	if fila == 0 {
		t.Fatal("no se encontró la fila del proveedor")
	}
	// Un número no lleva atributo `t` en el XML, así que excelize lo reporta como «unset»; lo que
	// delata al monto escrito como texto es que venga como cadena (suelta o compartida).
	celda := celdaC(fila)
	tipo, err := f.GetCellType("Cronograma", celda)
	if err != nil {
		t.Fatalf("tipo de celda: %v", err)
	}
	if tipo == excelize.CellTypeSharedString || tipo == excelize.CellTypeInlineString {
		t.Errorf("el monto quedó como texto (%v): la columna no suma ni ordena", tipo)
	}
	crudo, err := f.GetCellValue("Cronograma", celda, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatalf("leer %s: %v", celda, err)
	}
	if _, err := strconv.ParseFloat(crudo, 64); err != nil {
		t.Errorf("el monto escrito (%q) no es un número: %v", crudo, err)
	}
	// Y con formato de miles, o una columna angosta muestra «2,5E+05».
	estilo, err := f.GetCellStyle("Cronograma", celda)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.GetStyle(estilo)
	if err != nil {
		t.Fatal(err)
	}
	if st.CustomNumFmt == nil || *st.CustomNumFmt != "#,##0.00" {
		t.Errorf("el monto no tiene formato de miles: %+v", st.CustomNumFmt)
	}
}

// Sin dólares, las dos líneas de dólares NO salen: en cero invitan a buscar un error que no existe.
func TestSinDolaresElArchivoNoHablaDeDolares(t *testing.T) {
	f := abrirCronograma(t, cronogramaDeMuestra())
	filas, err := f.GetRows("Cronograma")
	if err != nil {
		t.Fatalf("leer filas: %v", err)
	}
	for _, fila := range filas {
		for _, celda := range fila {
			if strings.Contains(celda, "dólares colonizados") ||
				strings.Contains(celda, "Cronograma colones flujo normal") {
				t.Errorf("un corte sin dólares trae la línea %q", celda)
			}
		}
	}
}

func celdaC(fila int) string {
	nombre, _ := excelize.CoordinatesToCellName(3, fila)
	return nombre
}
