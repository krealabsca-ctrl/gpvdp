package bancos

// El tipo de cambio guarda SEIS decimales, contra PostgreSQL de verdad (10-nov-2026, mig 0119).
//
// «Debería permitir hasta 6 decimales, para que la ecuación nos quede bien; aunque del BCCR usa dos
// decimales, al usar el promedio debe permitir hasta 6 decimales si el promedio lo tiene.»
//
// Esto NO lo puede demostrar una prueba en Go. `decimal.Round(6)` siempre devuelve seis decimales;
// lo que decide si sobreviven es la ESCALA DE LA COLUMNA, y PostgreSQL recorta para entrar en ella
// sin dar error ni aviso. Con numeric(14,4), guardar 456,673333 y leerlo devuelve 456,6733 y todo
// parece correcto: el defecto solo aparece al cuadrar los montos del mes contra el banco.
//
// Se prueban las CUATRO columnas porque el valor las recorre todas: se calcula en tipo_cambio_mes,
// viaja al movimiento y a la factura. Una sola en cuatro decimales vuelve a redondearlo y anula el
// resto.

import (
	"context"
	"testing"
)

// unTCConSeisDecimales es el promedio real de octubre 2026: (460,73 + 453,25 + 456,04) / 3. Los
// dos últimos decimales son justamente los que se perdían.
const unTCConSeisDecimales = "456.673333"

func TestLasColumnasDeTipoDeCambioGuardanSeisDecimales(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()

	// Un movimiento al que aplicarle el TC, y una factura.
	movID := sembrarMov(t, repo, "2026-10-31", "900001", "CREDITO EN DOLARES", 880000)

	casos := []struct {
		nombre  string
		sembrar string
		leer    string
	}{
		{
			"la cotización del día",
			`INSERT INTO tipo_cambio_cotizacion (empresa_id, fecha, valor, fuente)
			 VALUES ($1::uuid, '2026-10-01', ` + unTCConSeisDecimales + `, 'MANUAL')`,
			`SELECT valor::text FROM tipo_cambio_cotizacion WHERE empresa_id = $1::uuid`,
		},
		{
			"el valor congelado del mes",
			`INSERT INTO tipo_cambio_mes (empresa_id, anio, mes, valor_congelado, estado)
			 VALUES ($1::uuid, 2026, 10, ` + unTCConSeisDecimales + `, 'CONGELADO')`,
			`SELECT valor_congelado::text FROM tipo_cambio_mes WHERE empresa_id = $1::uuid`,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := repo.pool.Exec(ctx, c.sembrar, empExc); err != nil {
				t.Fatalf("sembrar: %v", err)
			}
			var leido string
			if err := repo.pool.QueryRow(ctx, c.leer, empExc).Scan(&leido); err != nil {
				t.Fatalf("leer: %v", err)
			}
			// La comparación es de TEXTO a propósito: con numeric(14,4) la base devuelve
			// «456.6733» y una comparación numérica contra 456.673333 fallaría igual, pero el
			// mensaje no diría que lo que se perdió son decimales.
			if leido != unTCConSeisDecimales {
				t.Errorf("guardado = %q, se esperaba %q: la columna está recortando decimales",
					leido, unTCConSeisDecimales)
			}
		})
	}

	t.Run("el TC aplicado al movimiento", func(t *testing.T) {
		if _, err := repo.pool.Exec(ctx,
			`UPDATE movimiento_bancario SET tc_aplicado = `+unTCConSeisDecimales+
				` WHERE id = $1::uuid`, movID); err != nil {
			t.Fatalf("aplicar TC: %v", err)
		}
		var leido string
		if err := repo.pool.QueryRow(ctx,
			`SELECT tc_aplicado::text FROM movimiento_bancario WHERE id = $1::uuid`,
			movID).Scan(&leido); err != nil {
			t.Fatalf("leer: %v", err)
		}
		if leido != unTCConSeisDecimales {
			t.Errorf("tc_aplicado = %q, se esperaba %q: el movimiento recorta el tipo de cambio",
				leido, unTCConSeisDecimales)
		}
	})

	t.Run("el TC aplicado a la factura de CxP", func(t *testing.T) {
		// La factura exige proveedor, así que se siembra uno: saltarse la prueba por eso dejaría la
		// columna de CxP sin proteger, que es justo donde el TC vuelve a multiplicar un monto.
		var provID string
		if err := repo.pool.QueryRow(ctx, `
			INSERT INTO proveedor (empresa_id, nombre) VALUES ($1::uuid, 'Proveedor en dólares')
			RETURNING id::text`, empExc).Scan(&provID); err != nil {
			t.Fatalf("sembrar proveedor: %v", err)
		}
		var leido string
		if err := repo.pool.QueryRow(ctx, `
			INSERT INTO documento_cxp
				(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
				 total, total_crc, retencion, tc_aplicado)
			VALUES ($1::uuid, $2::uuid, '900002', '900002', '2026-10-31', 'APROBADO', 'USD',
			        1000, 456673.33, 0, `+unTCConSeisDecimales+`)
			RETURNING tc_aplicado::text`, empExc, provID).Scan(&leido); err != nil {
			t.Fatalf("sembrar factura en dólares: %v", err)
		}
		if leido != unTCConSeisDecimales {
			t.Errorf("tc_aplicado = %q, se esperaba %q: la factura recorta el tipo de cambio",
				leido, unTCConSeisDecimales)
		}
	})
}
