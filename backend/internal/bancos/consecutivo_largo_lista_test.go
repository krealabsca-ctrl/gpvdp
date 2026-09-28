package bancos

// El «Consecutivo largo» llega a la FILA, no solo al .xlsx (23-set-2026).
//
// El Director Financiero lo pidió en «Mi partida»: es la referencia del SINPE con la que el equipo
// cruza su recibo. No es una columna de la base —Davivienda la esconde dentro de la descripción—,
// así que se deriva. Lo que esta prueba protege es que se derive UNA sola vez, con la misma función
// del exportador: si mañana alguien la reimplementa en la pantalla, el número del .xlsx y el de la
// tabla pueden separarse y nadie se entera hasta que un depósito no cuadra.
//
// La regla de Davivienda ya está probada aparte (TestConsecutivoLargo, con la posición y el largo).
// Acá se prueba el cableado: que `ListarMovimientos` la aplique y que el resto de los bancos queden
// en blanco de verdad, que es el caso de 1 de cada 4 filas.

import (
	"context"
	"testing"
)

const (
	bancoDavi  = "da71e17a-da71-4a71-8a71-da71da71da71"
	cuentaDavi = "dacc0000-dacc-4acc-8acc-dacc0000dacc"
)

func TestConsecutivoLargoLlegaALaFila(t *testing.T) {
	repo, svc, _ := baseMiPartida(t)
	ctx := context.Background()

	// Una cuenta de Davivienda en la MISMA partida del equipo, para que entre en «Mi partida».
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Davivienda')`,
			[]any{bancoDavi, empresaPrueba}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'Davivienda Colones')`,
			[]any{cuentaDavi, empresaPrueba, bancoDavi}},
	} {
		if _, err := repo.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("sembrar Davivienda: %v", err)
		}
	}

	// La descripción real de la base: la referencia arranca en la posición 24.
	const descDavi = "DR/CR LINEA SINPE (SMO-2026080115283000117909629 - PAGO DE FUNERARIA)"
	const refEsperada = "2026080115283000117909629"

	var idDavi string
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
			 estado_clasificacion, natural_key, incluido)
		VALUES ($1::uuid, $2::uuid, '2026-12-18'::date, 'DOC-davi', $3, 0, 7777.00,
		        'CRC', 7777.00, 7777.00, $4::uuid, $5::uuid, 'REVISADO', 'cl-davi', true)
		RETURNING id::text`,
		empresaPrueba, cuentaDavi, descDavi, conceptoID, partidaMia).Scan(&idDavi); err != nil {
		t.Fatalf("sembrar el movimiento de Davivienda: %v", err)
	}

	res, err := svc.MiSegmento(ctx, empresaPrueba, usuarioPrueba, VistaPartida,
		FiltrosMovimientos{Periodo: "2026-12", PageSize: 100})
	if err != nil {
		t.Fatalf("mi partida: %v", err)
	}

	porID := map[string]MovimientoRow{}
	for _, it := range res.Movimientos.Items {
		porID[it.ID] = it
	}

	davi, ok := porID[idDavi]
	if !ok {
		t.Fatal("control: el movimiento de Davivienda no está en la partida")
	}
	if davi.ConsecutivoLargo != refEsperada {
		t.Fatalf("consecutivo largo = %q, se esperaba %q", davi.ConsecutivoLargo, refEsperada)
	}
	// Y es EXACTAMENTE lo que el exportador escribiría para esa misma fila: una sola definición.
	if davi.ConsecutivoLargo != ConsecutivoLargo(davi.Banco, davi.Descripcion) {
		t.Fatalf("la fila y el exportador difieren: %q vs %q",
			davi.ConsecutivoLargo, ConsecutivoLargo(davi.Banco, davi.Descripcion))
	}

	t.Run("los demás bancos quedan vacíos, no con basura", func(t *testing.T) {
		otros := 0
		for _, it := range res.Movimientos.Items {
			if it.ID == idDavi {
				continue
			}
			otros++
			if it.ConsecutivoLargo != "" {
				t.Errorf("%s (%s) trae consecutivo largo %q y su banco no lo publica",
					it.ID, it.Banco, it.ConsecutivoLargo)
			}
		}
		if otros == 0 {
			t.Fatal("control: no había ninguna fila de otro banco con la que comparar")
		}
	})
}
