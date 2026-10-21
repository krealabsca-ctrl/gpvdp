package cxp

// Filtrar por CUÁNDO SE PAGÓ DE VERDAD (19-oct-2026).
//
// EL PEDIDO: «en el flujo de lo pagado esté una fecha de cuando se pagó y que también lo pueda
// validar y filtrar».
//
// Ya existía un filtro «Pago», pero era la fecha PROGRAMADA: cuándo se pensaba pagar. Una factura
// programada para el 15 se paga el 22, y preguntarle a la programada «qué pagamos en agosto»
// devuelve lo que se planeó, no lo que salió del banco. Esa diferencia es justamente lo que se
// audita, así que son dos filtros y no uno.
//
// LA FECHA SALE DEL EVENTO, no de una columna. No existe una: `actualizado_en` se reescribe con
// cualquier edición posterior —adjuntar el comprobante, reclasificar— y arrastraría la factura a
// otro mes. Es la misma fuente que el ciclo de pago del tablero, y por eso no pueden discrepar.

import "testing"

func TestLaColumnaDeFechaDistingueProgramadoDePagado(t *testing.T) {
	t.Parallel()
	// La PROGRAMADA es una columna de la factura.
	if got := columnaFecha(FechaPago); got != "d.fecha_pago_programada" {
		t.Errorf("«pago» = %q, quiere la columna programada", got)
	}
	// La REAL sale del evento: si apuntara a una columna, estaría leyendo otra cosa.
	real := columnaFecha(FechaPagado)
	if real == "d.fecha_pago_programada" {
		t.Fatal("«pagado» devuelve la fecha PROGRAMADA: son dos cosas distintas")
	}
	if !contieneSub(real, "auditoria_evento") {
		t.Errorf("«pagado» no sale del evento: %q", real)
	}
	if contieneSub(real, "actualizado_en") {
		t.Error("«pagado» usa actualizado_en, que se reescribe con cualquier edición posterior")
	}
	// Y en día de Costa Rica: un pago de las 18:30 de acá es de ese día, no del siguiente.
	if !contieneSub(real, "America/Costa_Rica") {
		t.Errorf("«pagado» no convierte a día de Costa Rica: %q", real)
	}
}

// Las acciones que fechan un pago son las tres, no solo «PAGAR_DOCUMENTO».
//
// Una factura que se concilió automáticamente por la huella nunca pasó por el botón «Pagada»: su
// único evento es CONCILIAR_AUTO. Si la expresión mirara solo el primero, esas facturas no
// tendrían fecha y desaparecerían de cualquier filtro por rango.
func TestLaFechaDePagoCuentaLasTresFormasDePagar(t *testing.T) {
	t.Parallel()
	for _, accion := range []string{"PAGAR_DOCUMENTO", "CONCILIAR_AUTO", "CONCILIAR_DOCUMENTO"} {
		if !contieneSub(sqlFechaPagada, accion) {
			t.Errorf("%s no cuenta como pago: esas facturas quedarían sin fecha", accion)
		}
	}
	// MIN y no MAX: la fecha del pago es la del PRIMER evento. Con MAX, conciliar en octubre un
	// pago de agosto lo movería a octubre y el mes cerrado cambiaría solo.
	if !contieneSub(sqlFechaPagada, "MIN(") {
		t.Error("usa algo distinto de MIN: conciliar después movería la factura de mes")
	}
}

func contieneSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
