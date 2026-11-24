package bancos

// Diagnóstico del caso Scotiabank (23-oct-2026). Archivo real del Director Financiero.
//
// Reproduce la estructura exacta que devolvió el sistema en su diagnóstico: cuatro líneas de
// preámbulo (cliente, cuenta, tipo, moneda) y el encabezado recién en la fila 5. El formato
// configurado es el que se ve en su captura.

import (
	"strings"
	"testing"
)

// gridScotiabank es la estructura tal como la describió el diagnóstico del sistema.
var gridScotiabank = Grid{
	{"CLIENTE :", "", "VALLE DE PAZ SERVICIOS FUNERARIOS SOCIEDAD AN"},
	{"Nº DE CUENTA :", "", "CR26012300130016581036"},
	{"TIPO DE CUENTA :", "", "Universal específica"},
	{"MONEDA :", "", "COLON DE COSTA RICA"},
	{"Fecha y Hora", "Descripcion", "Documento", "Debito", "Creditos", "Saldo"},
	{"01/09/2026", "1752529810", "Intereses generados en su cuenta", "0", "25.83", "1000.00"},
	{"02/09/2026", "1752946737", "Intereses generados en su cuenta", "0", "25.83", "1025.83"},
}

// formatoScotiabank es exactamente lo que el Director configuró en la pantalla.
func formatoScotiabank() FormatoBanco {
	return FormatoBanco{
		Nombre:         "Scotiabank",
		Activo:         true,
		ColFecha:       0,
		ColDescripcion: 1,
		ColDocumento:   2,
		ColDebito:      3,
		ColCredito:     4,
		FormatoFecha:   FechaDDMMAAAA,
		Senales: []SenalEncabezado{
			{Columna: 0, Texto: "fecha y hora"},
			{Columna: 3, Texto: "debito"},
		},
	}
}

// Las señales TAL COMO quedaron guardadas en la base del Director Financiero.
//
// Las dos primeras son las que la pantalla propuso sola y calzan. Las tres de abajo las agregó a
// mano, y dos están CRUZADAS: dicen que «Descripcion» está en la columna 2 y «Documento» en la 1,
// cuando en la fila de títulos es al revés. Se entiende por qué: en este archivo los títulos de
// Scotiabank están corridos respecto de los datos —la columna 1 se titula «Descripcion» pero trae
// el número de documento—, así que al mirar los DATOS esas dos columnas parecen intercambiadas.
//
// Las señales no se comparan contra los datos: se comparan contra la fila de TÍTULOS. Y como
// todas se unen con Y, dos que nunca se cumplen hacen que el formato no reconozca nada.
func senalesGuardadasDelDirector() []SenalEncabezado {
	return []SenalEncabezado{
		{Columna: 0, Texto: "fecha y hora"},
		{Columna: 3, Texto: "debito"},
		{Columna: 2, Texto: "Descripcion"},
		{Columna: 1, Texto: "Documento"},
		{Columna: 4, Texto: "Creditos"},
	}
}

// EL ARREGLO: el sistema tiene que NOMBRAR la señal que no calza.
//
// Sabía perfectamente que la señal de la columna 2 pedía «Descripcion» y que ahí decía
// «Documento», y respondía «no se reconoció el archivo». Con eso, quien configura no tiene por
// dónde empezar: las cinco señales se ven igual de plausibles.
func TestProbarDiceCualSenalNoCalza(t *testing.T) {
	f := formatoScotiabank()
	f.Senales = senalesGuardadasDelDirector()

	fallan := SenalesQueNoCalzan(f, gridScotiabank)
	if len(fallan) != 2 {
		t.Fatalf("señales que fallan = %d, se esperaban 2: %+v", len(fallan), fallan)
	}
	// Y dice QUÉ hay de verdad en esa columna, que es lo que permite corregirlo sin volver al Excel.
	porColumna := map[int]SenalQueNoCalza{}
	for _, s := range fallan {
		porColumna[s.Columna] = s
	}
	if g := porColumna[2]; g.Pedido != "Descripcion" || g.Encontrado != "Documento" {
		t.Errorf("columna 2: pedido %q, encontrado %q", g.Pedido, g.Encontrado)
	}
	if g := porColumna[1]; g.Pedido != "Documento" || g.Encontrado != "Descripcion" {
		t.Errorf("columna 1: pedido %q, encontrado %q", g.Pedido, g.Encontrado)
	}
	// Contra qué fila se comparó: sin eso, «en los títulos dice X» no se puede verificar.
	if fallan[0].FilaTitulos != 5 {
		t.Errorf("fila de títulos = %d, se esperaba 5", fallan[0].FilaTitulos)
	}

	// Con las señales correctas no falla ninguna.
	buenas := formatoScotiabank()
	if n := len(SenalesQueNoCalzan(buenas, gridScotiabank)); n != 0 {
		t.Errorf("un formato que SÍ detecta reportó %d señales fallando", n)
	}
}

// Sin una fila de títulos reconocible NO se señalan señales: entonces el problema es otro —la hoja
// equivocada, el tramo equivocado— y mandar a corregir las señales sería mandar a tocar lo que
// está bien.
func TestSinFilaDeTitulosNoSeCulpaALasSenales(t *testing.T) {
	soloDatos := Grid{
		{"01/09/2026", "1752529810", "algo", "0", "25.83"},
		{"02/09/2026", "1752946737", "algo", "0", "25.83"},
	}
	if n := len(SenalesQueNoCalzan(formatoScotiabank(), soloDatos)); n != 0 {
		t.Errorf("se culpó a %d señales en un archivo sin fila de títulos", n)
	}
}

func TestLasSenalesCruzadasImpidenLaDeteccion(t *testing.T) {
	f := formatoScotiabank()
	f.Senales = senalesGuardadasDelDirector()

	if err := f.Validar(); err != nil {
		t.Fatalf("validar: %v", err)
	}
	a, err := f.Adaptador()
	if err != nil {
		t.Fatalf("adaptador: %v", err)
	}
	if a.Detecta(gridScotiabank) {
		t.Fatal("detectó: entonces las señales cruzadas NO son la causa y hay que seguir buscando")
	}

	// Cuál falla, exactamente. Es lo que el sistema tendría que haberle dicho.
	titulos := gridScotiabank[4]
	for _, s := range f.Senales {
		hay := cellHas(titulos, s.Columna, norm(s.Texto))
		estado := "CALZA"
		if !hay {
			estado = "NO CALZA"
		}
		t.Logf("%-9s columna %d: se pidió «%s» · en los títulos dice «%s»",
			estado, s.Columna, s.Texto, cell(titulos, s.Columna))
	}
}

func TestDiagnosticoScotiabank(t *testing.T) {
	f := formatoScotiabank()
	if err := f.Validar(); err != nil {
		t.Fatalf("el formato configurado no pasa la validación: %v", err)
	}
	a, err := f.Adaptador()
	if err != nil {
		t.Fatalf("no se pudo construir el adaptador: %v", err)
	}

	t.Run("detecta el archivo", func(t *testing.T) {
		if !a.Detecta(gridScotiabank) {
			t.Error("NO lo detecta")
		}
	})

	t.Run("lo parsea", func(t *testing.T) {
		res, err := a.Parsea(gridScotiabank)
		if err != nil {
			t.Fatalf("parsear: %v", err)
		}
		t.Logf("movimientos leídos: %d", len(res.Movimientos))
		for _, m := range res.Movimientos {
			t.Logf("  %s | doc %q | desc %q | deb %s | cred %s",
				m.Fecha.Format("2006-01-02"), m.Documento, m.Descripcion, m.Debito, m.Credito)
		}
	})

	// Y lo que el usuario VIO: el diagnóstico no llega a mostrar la fila del encabezado.
	t.Run("el diagnóstico muestra la fila de los títulos", func(t *testing.T) {
		diag := DiagnosticarEncabezado(gridScotiabank, "ESTADO DE MOVIMIENTOS", nil)
		msg := diag.Error()
		t.Logf("diagnóstico: %s", msg)
		if !strings.Contains(msg, "Fecha y Hora") {
			t.Error("el diagnóstico NO muestra la fila del encabezado: es justo la que hace falta " +
				"para configurar las columnas, y se queda fuera cuando el banco trae varias líneas de preámbulo")
		}
	})
}
