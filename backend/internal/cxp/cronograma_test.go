package cxp

// La cascada del cronograma (mig 0122).
//
// LA PRUEBA DE ACEPTACIÓN SON LAS CIFRAS DEL ARCHIVO REAL. El cuadro que esto reemplaza existe y
// tiene sus cuatro restas hechas; si el sistema no reproduce exactamente esos números, lo que
// produce no es el mismo documento y nadie lo va a poder usar para lo mismo.
//
// Las cifras salen del cronograma del 13-11-2026 que mostró el Director Financiero:
//
//	TOTAL COLONES                      51.782.243,73
//	TOTAL DOLARES COLONIZADO              328.491,00
//	TOTAL COLONIZADO                   52.110.734,73
//	TOTAL RESERVA BP NEGOCIOS             222.550,00
//	TOTAL FLUJO NORMAL VALLE           51.888.184,73
//	SOLICITUDES POR FUERA              40.955.356,26
//	CRONOGRAMA TOTAL FLUJO NORMAL      10.932.828,47
//	CRONOGRAMA COLONES FLUJO NORMAL    10.604.337,47
//
// El tipo de cambio implícito de ese archivo es 450,0000 exacto (328.491,00 / 730,00).

import (
	"testing"

	"github.com/shopspring/decimal"
)

func dec2(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Reproduce la cascada del archivo real, escalón por escalón.
func TestLaCascadaReproduceElCronogramaReal(t *testing.T) {
	// Se arma con cuatro líneas que suman las mismas cifras del archivo: el resto en colones de
	// flujo normal, la reserva, lo de por fuera, y los dólares.
	filas := []FilaCronogramaCruda{
		// Flujo normal en colones: lo que queda del cronograma menos los dólares que siguen en él.
		{ProveedorID: "p1", Proveedor: "Varios flujo normal", Moneda: "CRC",
			Monto: dec2("10604337.47"), Cuenta: "BAC Valle de Paz", TieneCuenta: true},
		// La reserva: sale de BP Negocios, que está marcada como cuenta de reserva.
		{ProveedorID: "p2", Proveedor: "Electritel Sociedad Anonima", Moneda: "CRC",
			Monto: dec2("222550.00"), Cuenta: "BP Negocios", EsReserva: true, TieneCuenta: true},
		// Las solicitudes por fuera: se pagan, pero no entran al cronograma que se comunica.
		{ProveedorID: "p3", Proveedor: "Solicitudes por fuera", Moneda: "CRC",
			Monto: dec2("40955356.26"), Cuenta: "BAC Valle de Paz", PorFuera: true, TieneCuenta: true},
		// Los dólares: 729,98 a 450,0000 dan los 328.491,00 exactos del archivo. (Leí 730,00 la primera
		// vez y la prueba lo atrapó: 730 × 450 da 328.500, nueve colones de más.)
		{ProveedorID: "p4", Proveedor: "Navegación Satelital de Costa Rica S.A", Moneda: "USD",
			Monto: dec2("729.98"), Cuenta: "BAC Dólares", TieneCuenta: true},
	}

	c, err := ConstruirCronograma(filas, dec2("450"))
	if err != nil {
		t.Fatalf("construir: %v", err)
	}

	casos := []struct {
		que    string
		got    string
		quiere string
	}{
		{"TOTAL COLONES", c.Totales.TotalColones, "51782243.73"},
		{"TOTAL DOLARES COLONIZADO", c.Totales.DolaresColonizados, "328491.00"},
		{"TOTAL COLONIZADO", c.Totales.TotalColonizado, "52110734.73"},
		{"TOTAL RESERVA", c.Totales.Reserva, "222550.00"},
		{"TOTAL FLUJO NORMAL", c.Totales.FlujoNormal, "51888184.73"},
		{"SOLICITUDES POR FUERA", c.Totales.PorFuera, "40955356.26"},
		{"CRONOGRAMA TOTAL FLUJO NORMAL", c.Totales.CronogramaTotal, "10932828.47"},
		{"CRONOGRAMA COLONES FLUJO NORMAL", c.Totales.CronogramaEnColones, "10604337.47"},
	}
	for _, k := range casos {
		if k.got != k.quiere {
			t.Errorf("%s = %s, el archivo real dice %s", k.que, k.got, k.quiere)
		}
	}
}

// Una línea que es reserva Y está marcada por fuera se cuenta UNA sola vez.
//
// Si se contara en las dos restas, la cascada descontaría dos veces el mismo dinero y el número
// que se comunica saldría más bajo de lo que es — sin que nada lo delate, porque cada resta por
// separado se ve razonable.
func TestUnaLineaNoSeDescuentaDosVeces(t *testing.T) {
	filas := []FilaCronogramaCruda{
		{ProveedorID: "p1", Proveedor: "Normal", Moneda: "CRC", Monto: dec2("1000"), TieneCuenta: true},
		{ProveedorID: "p2", Proveedor: "Las dos cosas", Moneda: "CRC", Monto: dec2("300"),
			EsReserva: true, PorFuera: true, TieneCuenta: true},
	}
	c, err := ConstruirCronograma(filas, decimal.Zero)
	if err != nil {
		t.Fatalf("construir: %v", err)
	}
	if c.Totales.Reserva != "300.00" {
		t.Errorf("reserva = %s, se esperaba 300.00", c.Totales.Reserva)
	}
	// NO vuelve a contarse acá: ya salió del flujo normal un escalón antes.
	if c.Totales.PorFuera != "0.00" {
		t.Errorf("por fuera = %s: la línea de reserva se está descontando dos veces", c.Totales.PorFuera)
	}
	if c.Totales.CronogramaTotal != "1000.00" {
		t.Errorf("cronograma = %s, se esperaba 1000.00", c.Totales.CronogramaTotal)
	}
}

// Los dólares que se fueron con la reserva o con lo de por fuera NO se restan otra vez al final.
func TestElUltimoEscalonSoloSacaLosDolaresQueSiguenEnElCronograma(t *testing.T) {
	filas := []FilaCronogramaCruda{
		{ProveedorID: "p1", Proveedor: "Colones normal", Moneda: "CRC", Monto: dec2("1000"), TieneCuenta: true},
		{ProveedorID: "p2", Proveedor: "Dólares normal", Moneda: "USD", Monto: dec2("10"), TieneCuenta: true},
		{ProveedorID: "p3", Proveedor: "Dólares por fuera", Moneda: "USD", Monto: dec2("20"),
			PorFuera: true, TieneCuenta: true},
	}
	c, err := ConstruirCronograma(filas, dec2("500"))
	if err != nil {
		t.Fatalf("construir: %v", err)
	}
	// Total colonizado: 1000 + 5000 + 10000 = 16000. Por fuera: 10000. Cronograma: 6000.
	if c.Totales.CronogramaTotal != "6000.00" {
		t.Fatalf("cronograma = %s, se esperaba 6000.00", c.Totales.CronogramaTotal)
	}
	// Y en colones queda 6000 − 5000 = 1000: solo se saca el dólar que SIGUE en el cronograma.
	// Restando los 15000 de todos los dólares daría −9000, que no significa nada.
	if c.Totales.CronogramaEnColones != "1000.00" {
		t.Errorf("cronograma en colones = %s, se esperaba 1000.00", c.Totales.CronogramaEnColones)
	}
}

// Sin tipo de cambio y con dólares en el corte, el cuadro NO se arma.
//
// La alternativa —colonizar a cero— haría desaparecer esos pagos del total sin que nada se queje,
// y el número que se comunica saldría corto.
func TestSinTipoDeCambioLosDolaresNoSeDanPorCero(t *testing.T) {
	filas := []FilaCronogramaCruda{
		{ProveedorID: "p1", Proveedor: "En dólares", Moneda: "USD", Monto: dec2("100"), TieneCuenta: true},
	}
	if _, err := ConstruirCronograma(filas, decimal.Zero); err != ErrTCCronogramaRequerido {
		t.Fatalf("error = %v, se esperaba ErrTCCronogramaRequerido", err)
	}
	// Pero un corte SIN dólares no necesita tipo de cambio: pedirlo sería un trámite inútil en el
	// caso normal, que es el 100 % de la operación en colones.
	soloColones := []FilaCronogramaCruda{
		{ProveedorID: "p1", Proveedor: "En colones", Moneda: "CRC", Monto: dec2("5000"), TieneCuenta: true},
	}
	if _, err := ConstruirCronograma(soloColones, decimal.Zero); err != nil {
		t.Errorf("un corte sin dólares no debería pedir tipo de cambio: %v", err)
	}
}

// Las líneas sin cuenta asignada se cuentan y se avisan: van al flujo normal, y un cronograma
// armado con la mitad sin asignar da un número que parece definitivo y no lo es.
func TestSeAvisaDeLasLineasSinCuentaAsignada(t *testing.T) {
	filas := []FilaCronogramaCruda{
		{ProveedorID: "p1", Proveedor: "Con cuenta", Moneda: "CRC", Monto: dec2("100"), TieneCuenta: true},
		{ProveedorID: "p2", Proveedor: "Sin cuenta", Moneda: "CRC", Monto: dec2("200")},
	}
	c, err := ConstruirCronograma(filas, decimal.Zero)
	if err != nil {
		t.Fatalf("construir: %v", err)
	}
	if c.SinCuenta != 1 {
		t.Errorf("sin cuenta = %d, se esperaba 1", c.SinCuenta)
	}
	// Y la que no tiene cuenta SÍ suma al cronograma: tratarla como reserva sacaría del cuadro
	// plata que sí se va a comunicar.
	if c.Totales.CronogramaTotal != "300.00" {
		t.Errorf("cronograma = %s, se esperaba 300.00", c.Totales.CronogramaTotal)
	}
}
