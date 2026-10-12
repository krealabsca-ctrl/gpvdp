package cxp

// Ordenar «Por pagar» por proveedor, fecha, monto o prioridad (1-oct-2026, punto 4).
//
// Dos cosas que probar, y la segunda es la que importa de verdad:
//
//  1. que el orden llegue al SQL — si se ordenara solo la página visible, la pantalla diría
//     «ordenado por monto» mostrando las 100 filas de siempre reacomodadas entre ellas, que es
//     mentir con 4.400 documentos detrás;
//  2. que el valor que llega del cliente NO termine dentro del ORDER BY. Ahí se concatena SQL.

import (
	"context"
	"strings"
	"testing"
)

func TestElOrdenSaleDeUnaListaCerrada(t *testing.T) {
	// Lo que un atacante (o un typo) podría mandar. Ninguno puede llegar al SQL: todos caen en el
	// default. Si alguien cambiara el mapa por una concatenación, esto se pone rojo.
	for _, malo := range []string{
		"d.total_crc; DROP TABLE documento_cxp",
		"proveedor; DELETE FROM lote_pago",
		"(SELECT 1)",
		"montoo",
		"MONTO", // la clave distingue mayúsculas: no se adivina
		"'",
	} {
		if got := ordenDocumentos(malo); got != ordenesDocumento[""] {
			t.Errorf("ordenDocumentos(%q) = %q — tiene que caer en el default", malo, got)
		}
	}

	// Y los buenos sí llegan.
	for _, bueno := range []string{"proveedor", "monto_desc", "prioridad", "vence", "departamento", "gasto"} {
		if got := ordenDocumentos(bueno); got == ordenesDocumento[""] {
			t.Errorf("ordenDocumentos(%q) cayó en el default: el orden no llegaría al SQL", bueno)
		}
	}
}

// Todo orden tiene que desempatar por `d.id`. Sin eso, dos facturas con el mismo monto (o la misma
// fecha) pueden cambiar de lugar entre una página y la siguiente: la misma factura sale dos veces,
// o ninguna. Con 4.400 documentos y páginas de 100, pasa todos los días.
func TestTodoOrdenTieneDesempateEstable(t *testing.T) {
	for clave, sql := range ordenesDocumento {
		if !strings.HasSuffix(sql, "d.id") {
			t.Errorf("el orden %q no termina en d.id: paginar con él puede repetir o saltar filas\n%s",
				clave, sql)
		}
	}
}

func TestElOrdenPedidoCambiaLasFilas(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	proveedorExtra(t, repo, provUno, "ZZZ ULTIMO ALFABETICAMENTE")
	proveedorExtra(t, repo, provDos, "AAA PRIMERO ALFABETICAMENTE")

	// El barato es del proveedor que va primero por nombre; el caro, del que va último. Así cada
	// orden tiene que devolver un primero DISTINTO y no se puede pasar por casualidad.
	facturaDe(t, repo, provDos, "F-BARATA", "1000")
	facturaDe(t, repo, provUno, "F-CARA", "900000")

	casos := []struct{ orden, primero, porque string }{
		{"proveedor", "AAA PRIMERO ALFABETICAMENTE", "por nombre ascendente"},
		{"proveedor_desc", "ZZZ ULTIMO ALFABETICAMENTE", "por nombre descendente"},
		{"monto_desc", "ZZZ ULTIMO ALFABETICAMENTE", "el monto mayor primero"},
		{"monto", "AAA PRIMERO ALFABETICAMENTE", "el monto menor primero"},
	}
	for _, c := range casos {
		t.Run(c.orden, func(t *testing.T) {
			lista, err := repo.ListarDocumentos(ctx, empresaLote, FiltrosDocumentos{
				Orden: c.orden, PageSize: 10,
			})
			if err != nil {
				t.Fatalf("listar: %v", err)
			}
			if len(lista.Items) < 2 {
				t.Fatalf("control: se esperaban 2 documentos, hay %d", len(lista.Items))
			}
			if lista.Items[0].Proveedor != c.primero {
				t.Errorf("con orden=%q el primero es %q y se esperaba %q (%s)",
					c.orden, lista.Items[0].Proveedor, c.primero, c.porque)
			}
		})
	}
}
