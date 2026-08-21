package bancos

// Regresión del importador del Banco Popular (agosto 2026).
//
// Qué pasó: el portal del BP exporta los meses en INGLÉS («01 AUG 2026 03:37») y el parser solo
// conocía las abreviaturas en español. Ocho de los doce meses se escriben igual en los dos
// idiomas, así que el adaptador —construido con un archivo de JUNIO— funcionó durante meses y
// se rompió en agosto. Peor: falló en silencio, porque descartar filas sin fecha es el
// comportamiento correcto para el saldo inicial y los subtotales, y el importador reportó
// «0 movimientos», que es idéntico a un mes sin actividad.
//
// El archivo real de julio parseaba 171 movimientos; el de agosto, 0.

import (
	"errors"
	"testing"
	"time"
)

func TestFechaBPAceptaMesesEnEspanolYEnIngles(t *testing.T) {
	t.Parallel()
	casos := []struct {
		entrada string
		quiere  time.Time
	}{
		// Los cuatro que DIFIEREN entre idiomas: son los que rompen el importador.
		{"01 ENE 2026 03:23", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"01 JAN 2026 03:23", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"15 ABR 2026 10:00", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)},
		{"15 APR 2026 10:00", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)},
		{"01 AGO 2026 03:37", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
		{"01 AUG 2026 03:37", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}, // el caso real
		{"31 DIC 2026 23:59", time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
		{"31 DEC 2026 23:59", time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
		// Los que coinciden en ambos idiomas y por eso nunca fallaron.
		{"01 JUN 2026 03:23", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
		{"01 JUL 2026 03:38", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		{"09 SEP 2026 08:00", time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)},
		{"09 SET 2026 08:00", time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)}, // abreviatura de CR
		// Con la sangría con la que viene en el Excel real.
		{"    01 AUG 2026 03:37", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
		// Minúsculas, por si el portal cambia de estilo.
		{"01 aug 2026 03:37", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range casos {
		got, err := fechaBP(c.entrada)
		if err != nil {
			t.Errorf("fechaBP(%q) devolvió error %v; se esperaba %s", c.entrada, err, c.quiere.Format("2006-01-02"))
			continue
		}
		if !got.Equal(c.quiere) {
			t.Errorf("fechaBP(%q) = %s, quiere %s", c.entrada, got.Format("2006-01-02"), c.quiere.Format("2006-01-02"))
		}
	}
}

func TestFechaBPRechazaLoQueNoEsFecha(t *testing.T) {
	t.Parallel()
	// Estas SÍ se tienen que descartar: son el saldo inicial, el pie y basura.
	for _, s := range []string{"", "    ", "Saldo Inicial", "01 XXX 2026", "AUG 2026", "aa AUG 2026"} {
		if _, err := fechaBP(s); err == nil {
			t.Errorf("fechaBP(%q) no debería parsear", s)
		}
	}
}

// gridBPAgosto reproduce el archivo real de agosto: encabezado y columnas idénticos al de
// junio, y la única diferencia es el mes en inglés.
var gridBPAgosto = Grid{
	{"Movimiento de cuenta", " Banco Popular"},
	{"    Titular             ", " VALLE DE PAZ SERVICIOS FUNERARIOS S", "Cuenta IBAN", "      CR62016101008810244232"},
	{"    Rango de fecha      ", " 01/08/2026 - 07/08/2026        ", "Moneda", "           Colon Costa Rica"},
	{"Fecha y Hora", "Descripcion", "Documento", "Debito", "Creditos", "Saldo"},
	{"    ", "Saldo Inicial", "", "CRC ", "CRC ", "CRC 1,485,296.57"},
	{"    01 AUG 2026 03:37", "Orden Fija VALLE DE PAZ", "FT26213R2THG\\C36", "CRC 0.00", "CRC 6,500.00", "CRC 1,491,796.57"},
	{"    03 AUG 2026 12:41", "Pago Cuotas Prestamo PAGO 0180246385892", "FT26215S22VW\\BNK", "CRC 580,638.50", "CRC 0.00", "CRC 996,351.50"},
}

func TestBPParseaElArchivoDeAgosto(t *testing.T) {
	t.Parallel()
	a, err := Detectar(gridBPAgosto)
	if err != nil {
		t.Fatalf("Detectar: %v", err)
	}
	if a.Banco() != BancoBP {
		t.Fatalf("banco = %s, quiere %s", a.Banco(), BancoBP)
	}
	res, err := a.Parsea(gridBPAgosto)
	if err != nil {
		t.Fatalf("Parsea: %v", err)
	}
	if len(res.Movimientos) != 2 {
		t.Fatalf("movimientos = %d, quiere 2 (el saldo inicial SÍ se descarta)", len(res.Movimientos))
	}
	if got := res.Movimientos[0].Fecha.Format("2006-01-02"); got != "2026-08-01" {
		t.Errorf("fecha[0] = %s, quiere 2026-08-01", got)
	}
	if got := res.Movimientos[1].Debito.String(); got != "580638.5" {
		t.Errorf("débito[1] = %s, quiere 580638.5", got)
	}
	if res.IBAN != "CR62016101008810244232" {
		t.Errorf("IBAN = %q", res.IBAN)
	}
}

// TestParseaGritaSiNoEntiendeNingunaFecha es el guardarraíl que faltaba: sin esto, un cambio
// de formato del banco se ve exactamente igual que un mes sin movimientos.
func TestParseaGritaSiNoEntiendeNingunaFecha(t *testing.T) {
	t.Parallel()
	roto := Grid{
		{"Movimiento de cuenta", " Banco Popular"},
		{"Fecha y Hora", "Descripcion", "Documento", "Debito", "Creditos", "Saldo"},
		{"    ", "Saldo Inicial", "", "CRC ", "CRC ", "CRC 1,485,296.57"},
		{"    2026-08-01T03:37:00Z", "Orden Fija", "FT1", "CRC 0.00", "CRC 6,500.00", "CRC 1"},
		{"    2026-08-03T12:41:00Z", "Pago", "FT2", "CRC 580,638.50", "CRC 0.00", "CRC 2"},
	}
	_, err := bp.Parsea(roto)
	var ilegibles *FechasIlegiblesError
	if !errors.As(err, &ilegibles) {
		t.Fatalf("Parsea devolvió %v; se esperaba FechasIlegiblesError", err)
	}
	if ilegibles.Filas != 2 {
		t.Errorf("Filas = %d, quiere 2 (el saldo inicial no cuenta: su celda de fecha está vacía)", ilegibles.Filas)
	}
	if ilegibles.Muestra != "2026-08-01T03:37:00Z" {
		t.Errorf("Muestra = %q, quiere la primera fecha cruda que no se entendió", ilegibles.Muestra)
	}
	if ilegibles.Banco != BancoBP {
		t.Errorf("Banco = %s", ilegibles.Banco)
	}
}

// Un archivo legítimo SIN movimientos (mes sin actividad) NO puede confundirse con el error
// de arriba: se distingue por tener las celdas de fecha vacías, no ilegibles.
func TestParseaAceptaUnMesSinMovimientos(t *testing.T) {
	t.Parallel()
	vacio := Grid{
		{"Movimiento de cuenta", " Banco Popular"},
		{"Fecha y Hora", "Descripcion", "Documento", "Debito", "Creditos", "Saldo"},
		{"    ", "Saldo Inicial", "", "CRC ", "CRC ", "CRC 1,485,296.57"},
		{"    ", "Saldo Final", "", "CRC ", "CRC ", "CRC 1,485,296.57"},
	}
	res, err := bp.Parsea(vacio)
	if err != nil {
		t.Fatalf("Parsea de un mes sin movimientos no debe fallar: %v", err)
	}
	if len(res.Movimientos) != 0 {
		t.Errorf("movimientos = %d, quiere 0", len(res.Movimientos))
	}
}
