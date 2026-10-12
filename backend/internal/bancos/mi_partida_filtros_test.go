package bancos

// Filtros de «Mi partida» (2-oct-2026): monto, y el aviso de «está en otro mes».
//
// El segundo es el que importa. Buscar y no encontrar era AMBIGUO: «no existe ningún depósito de
// SOLIDARIS» y «no existe EN ESTE MES» se veían igual —una lista vacía— y la segunda es la
// respuesta correcta casi siempre, porque quien consulta su partida busca un depósito que le
// avisaron y no sabe de qué mes es. La pantalla lo dejaba concluir que no entró.

import (
	"context"
	"testing"
)

func TestElFiltroDeMontoRecortaPorElValorEnColones(t *testing.T) {
	repo, _, _ := baseMiPartida(t)
	ctx := context.Background()

	// Tres montos bien separados, todos en la partida del equipo.
	for i, monto := range []string{"5000", "50000", "500000"} {
		if _, err := repo.pool.Exec(ctx, `
			INSERT INTO movimiento_bancario
				(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
				 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
				 estado_clasificacion, natural_key, incluido)
			VALUES ($1::uuid, $2::uuid, '2026-12-05'::date, $3, 'DEPOSITO DE PRUEBA', 0, $4::numeric,
			        'CRC', $4::numeric, $4::numeric, $5::uuid, $6::uuid, 'REVISADO', $3, true)`,
			empresaPrueba, cuentaBuena, "mnt-"+string(rune('a'+i)), monto, conceptoID, partidaMia); err != nil {
			t.Fatalf("sembrar %s: %v", monto, err)
		}
	}

	casos := []struct {
		nombre, min, max string
		quiere           int
	}{
		{"sin rango los trae todos", "", "", 3},
		{"desde 50.000 deja afuera el chico", "50000", "", 2},
		{"hasta 50.000 deja afuera el grande", "", "50000", 2},
		{"rango cerrado deja solo el del medio", "20000", "100000", 1},
		{"los extremos son INCLUSIVOS", "50000", "50000", 1},
		{"un rango sin nada adentro devuelve vacío", "600000", "700000", 0},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			res, err := repo.ListarMovimientos(ctx, empresaPrueba, FiltrosMovimientos{
				Q: "DEPOSITO DE PRUEBA", MontoMin: c.min, MontoMax: c.max, PageSize: 50,
			})
			if err != nil {
				t.Fatalf("listar: %v", err)
			}
			if len(res.Items) != c.quiere {
				t.Errorf("trajo %d, quiere %d (min=%q max=%q)", len(res.Items), c.quiere, c.min, c.max)
			}
		})
	}
}

func TestElAvisoDiceEnQueOtroMesEstaLoQueSeBusca(t *testing.T) {
	repo, _, _ := baseMiPartida(t)
	ctx := context.Background()

	// El mismo depósito, en tres meses. El equipo mira diciembre y lo busca sin saber de cuándo es.
	for i, fecha := range []string{"2026-12-05", "2026-10-20", "2026-10-21", "2026-08-15"} {
		if _, err := repo.pool.Exec(ctx, `
			INSERT INTO movimiento_bancario
				(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
				 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
				 estado_clasificacion, natural_key, incluido)
			VALUES ($1::uuid, $2::uuid, $3::date, $4, 'SINPE SOLIDARISTA ABC', 0, 1000,
			        'CRC', 1000, 1000, $5::uuid, $6::uuid, 'REVISADO', $4, true)`,
			empresaPrueba, cuentaBuena, fecha, "soli-"+string(rune('a'+i)), conceptoID, partidaMia); err != nil {
			t.Fatalf("sembrar %s: %v", fecha, err)
		}
	}

	t.Run("buscando en diciembre avisa de octubre y agosto", func(t *testing.T) {
		otros, err := repo.CoincidenciasEnOtrosMeses(ctx, empresaPrueba, FiltrosMovimientos{
			Q: "SOLIDARISTA", Periodo: "2026-12", Alcance: []string{partidaMia},
		})
		if err != nil {
			t.Fatalf("coincidencias: %v", err)
		}
		porMes := map[string]int{}
		for _, o := range otros {
			porMes[o.Periodo] = o.Cuantos
		}
		if porMes["2026-10"] != 2 {
			t.Errorf("octubre = %d, quiere 2", porMes["2026-10"])
		}
		if porMes["2026-08"] != 1 {
			t.Errorf("agosto = %d, quiere 1", porMes["2026-08"])
		}
		// Y NO vuelve a nombrar el mes que se está mirando: eso no es un hallazgo.
		if _, hay := porMes["2026-12"]; hay {
			t.Error("nombró el mes que el usuario ya tiene a la vista")
		}
	})

	t.Run("un rango avisa de los dos lados, no solo de antes", func(t *testing.T) {
		// Mirando octubre: agosto queda ANTES y diciembre DESPUÉS. Con las condiciones unidas por
		// AND en vez de OR, «antes Y después» no es nadie y el aviso callaría los dos.
		otros, err := repo.CoincidenciasEnOtrosMeses(ctx, empresaPrueba, FiltrosMovimientos{
			Q: "SOLIDARISTA", Desde: "2026-10-01", Hasta: "2026-10-31", Alcance: []string{partidaMia},
		})
		if err != nil {
			t.Fatalf("coincidencias: %v", err)
		}
		porMes := map[string]int{}
		for _, o := range otros {
			porMes[o.Periodo] = o.Cuantos
		}
		if porMes["2026-08"] != 1 || porMes["2026-12"] != 1 {
			t.Errorf("agosto=%d diciembre=%d; se esperaba 1 y 1 — el aviso tiene que mirar los dos lados de un rango",
				porMes["2026-08"], porMes["2026-12"])
		}
	})

	t.Run("sin búsqueda no avisa nada", func(t *testing.T) {
		// Un aviso que aparece siempre deja de leerse.
		otros, err := repo.CoincidenciasEnOtrosMeses(ctx, empresaPrueba, FiltrosMovimientos{
			Periodo: "2026-12", Alcance: []string{partidaMia},
		})
		if err != nil {
			t.Fatalf("coincidencias: %v", err)
		}
		if len(otros) != 0 {
			t.Errorf("avisó %d meses sin que nadie buscara nada", len(otros))
		}
	})

	t.Run("sin filtro de fecha tampoco: ya se está viendo todo", func(t *testing.T) {
		otros, err := repo.CoincidenciasEnOtrosMeses(ctx, empresaPrueba, FiltrosMovimientos{
			Q: "SOLIDARISTA", Alcance: []string{partidaMia},
		})
		if err != nil {
			t.Fatalf("coincidencias: %v", err)
		}
		if len(otros) != 0 {
			t.Errorf("avisó de %d meses que el usuario ya tiene delante", len(otros))
		}
	})

	// LA prueba de seguridad: el aviso corre con el MISMO recorte por segmento que la lista. Si
	// contara sobre toda la empresa, le diría a un consultor que «existe algo» en una partida que
	// no puede ver — justo lo que el alcance impide.
	t.Run("un alcance vacío CIERRA también acá", func(t *testing.T) {
		otros, err := repo.CoincidenciasEnOtrosMeses(ctx, empresaPrueba, FiltrosMovimientos{
			Q: "SOLIDARISTA", Periodo: "2026-12", Alcance: []string{},
		})
		if err != nil {
			t.Fatalf("coincidencias: %v", err)
		}
		if len(otros) != 0 {
			t.Fatalf("con alcance vacío avisó de %d meses: el recorte no llegó a esta consulta", len(otros))
		}
	})

	t.Run("una partida ajena no se delata", func(t *testing.T) {
		otros, err := repo.CoincidenciasEnOtrosMeses(ctx, empresaPrueba, FiltrosMovimientos{
			Q: "SOLIDARISTA", Periodo: "2026-12", Alcance: []string{partidaAjena},
		})
		if err != nil {
			t.Fatalf("coincidencias: %v", err)
		}
		if len(otros) != 0 {
			t.Errorf("avisó de movimientos de otra partida: %+v", otros)
		}
	})
}
