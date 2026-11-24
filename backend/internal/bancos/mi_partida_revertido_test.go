package bancos

// Lo revertido no llega a «Mi partida» (22-oct-2026).
//
// EL PEDIDO (Director Financiero, sobre la leyenda al pie de la lista): «lo que se invierte es un
// tema interno del admin o los que tengan permisos para manipularlo, pero esto no puede quedar
// visible a la operativa, genera demasiada confusión».
//
// LO QUE ESTABA PASANDO: la fila se mostraba y no sumaba, así que el paginador decía «3.845
// movimientos» y la suma de dinero contaba 3.811. Para que esos dos números no se contradijeran en
// silencio había una leyenda al pie —«34 de esos vienen de una importación revertida»— que encima
// podía mentir: el mismo contador incluye lo que la tesorera saca a mano por duplicado, que no
// viene de ninguna reversa.
//
// Ese razonamiento —«no puede desaparecer de la lista donde la tesorera acaba de marcarlo»— vale
// en la HOJA DE TRABAJO, donde quien mira es quien excluyó. Acá no: el equipo que consulta su
// partida no excluyó nada y no puede hacer nada con esa fila.
//
// Por eso esta prueba tiene DOS mitades, y la segunda importa tanto como la primera: que lo
// revertido salga de «Mi partida» NO puede sacarlo también de la hoja de trabajo.

import (
	"context"
	"testing"
)

// sembrarCreditoRevertido mete un crédito de la partida del equipo YA fuera de los libros, como
// queda un movimiento después de revertir su importación.
func sembrarCreditoRevertido(t *testing.T, repo *pgRepository, doc, monto string) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
			 estado_clasificacion, natural_key, incluido, excluido_por_reversa)
		VALUES ($1::uuid, $2::uuid, '2026-12-11'::date, $3, 'TEF DE:CARGA REVERTIDA', 0, $4::numeric,
		        'CRC', $4::numeric, $4::numeric, $5::uuid, $6::uuid, 'REVISADO', $3, false, true)`,
		empresaPrueba, cuentaBuena, doc, monto, conceptoID, partidaMia); err != nil {
		t.Fatalf("sembrar crédito revertido: %v", err)
	}
}

func TestLoRevertidoNoSaleDeMiPartida(t *testing.T) {
	repo, svc, _ := baseMiPartida(t)
	ctx := context.Background()
	diciembre := FiltrosMovimientos{Periodo: "2026-12", PageSize: 100}

	// El escenario base de diciembre son 3 créditos de la partida por ₡2.500 (ver
	// TestLoSinPartidaNoSaleDeMiPartida). Se le agrega uno revertido, por un monto bien distinto
	// para que un total mal sumado se note.
	sembrarCreditoRevertido(t, repo, "rev-1", "900000")

	partida, err := svc.MiSegmento(ctx, empresaPrueba, usuarioPrueba, VistaPartida, diciembre)
	if err != nil {
		t.Fatalf("mi segmento: %v", err)
	}

	// El conteo. Antes del cambio esto daba 4: la fila se contaba aunque no sumara.
	if partida.Movimientos.Total != 3 {
		t.Errorf("filas = %d, se esperaban 3: lo revertido no se le muestra a quien consulta su partida",
			partida.Movimientos.Total)
	}
	// Y el contador que alimentaba la leyenda al pie queda en cero, que es lo que la hace
	// innecesaria: no hay nada que explicar porque no hay nada que se cuente sin sumar.
	if partida.Movimientos.Totales.Excluidos != 0 {
		t.Errorf("excluidos = %d, se esperaba 0: con esto la pantalla volvía a pedir una leyenda",
			partida.Movimientos.Totales.Excluidos)
	}
	if !igualDecimal(t, partida.Movimientos.Totales.TotalCreditos, "2500.00") {
		t.Errorf("total = %s, se esperaba 2500.00 (el revertido nunca sumó, y ahora tampoco se cuenta)",
			partida.Movimientos.Totales.TotalCreditos)
	}
	// Ninguna fila puede venir marcada: si llegara una, la pantalla —que ya no la pinta distinto—
	// la mostraría como plata que entró.
	for _, it := range partida.Movimientos.Items {
		if !it.Incluido {
			t.Errorf("llegó la fila %s fuera de los libros: la pantalla la dibuja como una más", it.ID)
		}
	}

	// LA OTRA MITAD. La hoja de trabajo la sigue viendo: ahí mirar lleva a una acción —deshacer la
	// reversa, revisar el duplicado— y una fila que se evapora no explica por qué bajó un total.
	t.Run("la hoja de trabajo la sigue mostrando", func(t *testing.T) {
		hoja, err := repo.ListarMovimientos(ctx, empresaPrueba, FiltrosMovimientos{
			Q: "CARGA REVERTIDA", PageSize: 50,
		})
		if err != nil {
			t.Fatalf("listar: %v", err)
		}
		if hoja.Total != 1 {
			t.Fatalf("la hoja de trabajo trajo %d, se esperaba 1: sacarlo de «Mi partida» no puede sacarlo de acá", hoja.Total)
		}
		if hoja.Totales.Excluidos != 1 {
			t.Errorf("excluidos en la hoja = %d, se esperaba 1", hoja.Totales.Excluidos)
		}
		if hoja.Items[0].Incluido {
			t.Errorf("la fila de la hoja de trabajo viene marcada como incluida: ahí sí se pinta distinta")
		}
	})

	// El aviso de «también aparece en otro mes» cuenta sobre el mismo conjunto. Si contara lo
	// revertido, mandaría a alguien a un mes donde después no encontraría nada.
	t.Run("el aviso de otros meses tampoco lo cuenta", func(t *testing.T) {
		sembrarCreditoRevertido(t, repo, "rev-2", "900000")
		if _, err := repo.pool.Exec(ctx,
			`UPDATE movimiento_bancario SET fecha = '2026-10-11'::date WHERE natural_key = 'rev-2'`); err != nil {
			t.Fatalf("mover el revertido a octubre: %v", err)
		}
		otros, err := repo.CoincidenciasEnOtrosMeses(ctx, empresaPrueba, FiltrosMovimientos{
			Q: "CARGA REVERTIDA", Periodo: "2026-12", Alcance: []string{partidaMia}, SoloIncluidos: true,
		})
		if err != nil {
			t.Fatalf("coincidencias: %v", err)
		}
		for _, o := range otros {
			t.Errorf("avisó de %s (%d): ahí solo hay una carga revertida que el consultor no va a ver",
				o.Periodo, o.Cuantos)
		}
	})
}
