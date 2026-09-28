package bancos

// «Mi partida en bancos» mostraba 200 filas de 2.108 y no lo decía en ninguna parte: el pie seguía
// diciendo «Mostrando Septiembre 2026» mientras el 90 % de los créditos del equipo quedaba fuera.
// Quien concilia con esa lista da por FALTANTE plata que sí entró.
//
// El recorte lo ponía el cliente, pero lo que hace posible arreglarlo es una promesa del servidor:
// que `total` mide el CONJUNTO FILTRADO y no la página que se devolvió. Sin esa promesa el
// paginador nuevo mentiría igual, solo que con más confianza. Estas pruebas la fijan.
//
// Se corren contra Postgres de verdad (mismo esquema temporal que `excluido_no_suma_test.go`)
// porque lo que hay que probar es el SQL: un doble del repositorio probaría el doble. Sin base de
// datos se OMITEN.

import (
	"context"
	"fmt"
	"testing"
)

const (
	partidaAjena = "88888888-8888-4888-8888-888888888888"
)

// sembrarOctubre deja en el mes 2026-10 —vacío en el incidente— los créditos con los que se prueba
// el paginado: 25 del alcance, 3 del alcance EXCLUIDOS y 5 de una partida ajena.
//
// Cada fila lleva `documento`: es la referencia bancaria que el equipo compara contra su recibo, y
// una prueba que no la mire no notaría que el SELECT dejó de traerla.
func sembrarOctubre(t *testing.T, repo *pgRepository) {
	t.Helper()
	ctx := context.Background()

	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
		 VALUES ($1::uuid, $2::uuid, $3::uuid, 'Servicios de Emergencias')`,
		partidaAjena, empresaPrueba, conceptoID); err != nil {
		t.Fatalf("sembrar la partida ajena: %v", err)
	}

	const q = `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
			 estado_clasificacion, natural_key, incluido)
		VALUES ($1::uuid, $2::uuid, $3::date, $4, $5, 0, $6, 'CRC', $6, $6,
		        $7::uuid, $8::uuid, 'REVISADO', $9, $10)`

	sembrar := func(nombre, partida, monto string, dia int, incluido bool) {
		fecha := fmt.Sprintf("2026-10-%02d", dia)
		if _, err := repo.pool.Exec(ctx, q,
			empresaPrueba, cuentaBuena, fecha,
			fmt.Sprintf("REF-%s-%02d", nombre, dia), "DEPOSITO CLIENTE "+nombre,
			monto, conceptoID, partida, fmt.Sprintf("%s-%02d", nombre, dia), incluido); err != nil {
			t.Fatalf("sembrar %s del día %d: %v", nombre, dia, err)
		}
	}

	for dia := 1; dia <= 25; dia++ {
		sembrar("mio", partidaMia, "1000.00", dia, true)
	}
	// Duplicados revertidos: siguen en la lista, marcados, y NO suman (cambio del 18-set).
	for dia := 26; dia <= 28; dia++ {
		sembrar("mio-excluido", partidaMia, "1000000.00", dia, false)
	}
	// De otra partida: el paginado no puede ser la puerta por donde se cuelan.
	for dia := 1; dia <= 5; dia++ {
		sembrar("ajeno", partidaAjena, "500000.00", dia, true)
	}
}

// filtroDeOctubre es lo que arma el servicio para «Mi partida»: el alcance y el tipo los pone el
// SERVIDOR (ver segmento_test.go), acá se fija qué hace la consulta con ellos al paginar.
func filtroDeOctubre(page, pageSize int) FiltrosMovimientos {
	return FiltrosMovimientos{
		Periodo:  "2026-10",
		Tipo:     "CREDITO",
		Alcance:  []string{partidaMia},
		Page:     page,
		PageSize: pageSize,
	}
}

func TestMiSegmentoTotalNoSeCalculaSobreLaPagina(t *testing.T) {
	repo := baseDelIncidente(t)
	sembrarIncidente(t, repo)
	sembrarOctubre(t, repo)
	ctx := context.Background()

	// 28 filas del alcance en octubre: 25 que suman y 3 excluidas que se muestran marcadas.
	const filasDelAlcance = 28

	t.Run("la primera página dice cuántas hay en total", func(t *testing.T) {
		got, err := repo.ListarMovimientos(ctx, empresaPrueba, filtroDeOctubre(1, 10))
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		// ESTO es lo que la pantalla necesita para poder escribir «1-10 de 28». Sin este número el
		// cliente solo puede contar lo que recibió —10— y el recorte queda invisible.
		if got.Total != filasDelAlcance {
			t.Errorf("total = %d, se esperaban %d: el total mide el conjunto filtrado, no la página", got.Total, filasDelAlcance)
		}
		if len(got.Items) != 10 {
			t.Errorf("items = %d, se esperaban 10", len(got.Items))
		}
		if got.Page != 1 || got.PageSize != 10 {
			t.Errorf("la respuesta no repite la página pedida: page=%d page_size=%d", got.Page, got.PageSize)
		}
	})

	t.Run("la última página está incompleta y el total no cambia", func(t *testing.T) {
		got, err := repo.ListarMovimientos(ctx, empresaPrueba, filtroDeOctubre(3, 10))
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if len(got.Items) != 8 {
			t.Errorf("items = %d, se esperaban 8 (28 − 20)", len(got.Items))
		}
		if got.Total != filasDelAlcance {
			t.Errorf("total = %d, se esperaban %d en toda página", got.Total, filasDelAlcance)
		}
	})

	t.Run("una página fuera de rango sigue diciendo cuántas hay", func(t *testing.T) {
		got, err := repo.ListarMovimientos(ctx, empresaPrueba, filtroDeOctubre(9, 10))
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if len(got.Items) != 0 {
			t.Errorf("items = %d, se esperaban 0", len(got.Items))
		}
		// Si el conteo viajara con las filas (un COUNT(*) OVER ()), acá no volvería ninguno y la
		// pantalla no podría decir «esta página quedó vacía: hay 28 movimientos».
		if got.Total != filasDelAlcance {
			t.Errorf("total = %d, se esperaban %d: sin él la página vacía no se puede explicar", got.Total, filasDelAlcance)
		}
	})

	t.Run("recorriendo las páginas se ven todas las filas, una sola vez", func(t *testing.T) {
		vistos := map[string]int{}
		for page := 1; page <= 3; page++ {
			got, err := repo.ListarMovimientos(ctx, empresaPrueba, filtroDeOctubre(page, 10))
			if err != nil {
				t.Fatalf("página %d: %v", page, err)
			}
			for _, it := range got.Items {
				vistos[it.ID]++
			}
		}
		if len(vistos) != filasDelAlcance {
			t.Errorf("filas distintas = %d, se esperaban %d: el paginado salta o repite", len(vistos), filasDelAlcance)
		}
		for id, veces := range vistos {
			if veces != 1 {
				t.Errorf("el movimiento %s apareció %d veces", id, veces)
			}
		}
	})

	t.Run("los totales de dinero no dependen de la página", func(t *testing.T) {
		for _, page := range []int{1, 2, 3} {
			got, err := repo.ListarMovimientos(ctx, empresaPrueba, filtroDeOctubre(page, 10))
			if err != nil {
				t.Fatalf("página %d: %v", page, err)
			}
			// 25 × ₡1.000. Los 3 excluidos (₡1.000.000 cada uno) NO suman: es la marca del 18-set,
			// y una página no puede cambiarla.
			if !igualDecimal(t, got.Totales.TotalCreditos, "25000.00") {
				t.Errorf("página %d: créditos = %s, se esperaba 25000.00", page, got.Totales.TotalCreditos)
			}
			if got.Totales.Excluidos != 3 {
				t.Errorf("página %d: excluidos = %d, se esperaban 3", page, got.Totales.Excluidos)
			}
		}
	})

	t.Run("el recorte por alcance se mantiene en todas las páginas", func(t *testing.T) {
		for page := 1; page <= 3; page++ {
			got, err := repo.ListarMovimientos(ctx, empresaPrueba, filtroDeOctubre(page, 10))
			if err != nil {
				t.Fatalf("página %d: %v", page, err)
			}
			for _, it := range got.Items {
				if it.Clasificacion != "Depósito de Clientes" {
					t.Fatalf("página %d: se coló una fila de %q — el alcance no puede aflojarse al paginar",
						page, it.Clasificacion)
				}
			}
		}
	})
}

// Las dos columnas que pidió el Director ya viajan en la respuesta: la REFERENCIA BANCARIA
// (`documento`, la que trae cada banco en su propia columna) y el SEGMENTO asignado
// (`clasificacion`). Esta prueba es la que se pone roja si alguien las saca del SELECT.
func TestMiSegmentoTraeReferenciaYSegmentoEnCadaFila(t *testing.T) {
	repo := baseDelIncidente(t)
	sembrarIncidente(t, repo)
	sembrarOctubre(t, repo)
	ctx := context.Background()

	got, err := repo.ListarMovimientos(ctx, empresaPrueba, filtroDeOctubre(1, 50))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(got.Items) != 28 {
		t.Fatalf("items = %d, se esperaban 28", len(got.Items))
	}
	var excluidasVistas int
	for _, it := range got.Items {
		if it.Documento == "" {
			t.Errorf("movimiento %s sin referencia bancaria: la fila no se puede cotejar con el recibo", it.ID)
		}
		if it.Clasificacion == "" {
			t.Errorf("movimiento %s sin segmento: no se puede saber contra qué partida quedó marcado", it.ID)
		}
		if it.ClasificacionID == nil || *it.ClasificacionID != partidaMia {
			t.Errorf("movimiento %s con partida %v, se esperaba la del alcance", it.ID, it.ClasificacionID)
		}
		if !it.Incluido {
			excluidasVistas++
		}
	}
	// El excluido se sigue mostrando y llega MARCADO: una fila que se evapora no explica por qué
	// bajó un total, y una que llega sin marca se lee como plata buena.
	if excluidasVistas != 3 {
		t.Errorf("filas marcadas como excluidas = %d, se esperaban 3", excluidasVistas)
	}
}

// El paginado con EMPATES, que es el caso real y el que la prueba de arriba no cubre: ahí cada fila
// tiene una fecha distinta, así que el orden sale único aunque el ORDER BY no desempate.
//
// En el alcance real hay hasta 1.184 créditos del mismo día y 2.009 del mismo monto. Si el ORDER BY
// no termina en una columna única, Postgres puede devolver los empatados en cualquier orden —y en
// uno DISTINTO en cada página, porque con LIMIT/OFFSET ordena con un montículo que no es estable—.
// Resultado: al pasar de página una fila se repite y otra no aparece NUNCA. Medido contra la base,
// sin el desempate por `m.id` se pierden 113 filas ordenando por fecha y 9 ordenando por monto.
//
// Por eso se prueban las CUATRO opciones de «Ordenar por», no solo la de por defecto.
func TestMiSegmentoPaginadoConEmpatesNoPierdeNiRepite(t *testing.T) {
	repo := baseDelIncidente(t)
	sembrarIncidente(t, repo)
	ctx := context.Background()

	// 120 créditos del MISMO día y del MISMO monto: empate total en las dos claves de orden.
	const empatados = 120
	const q = `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
			 estado_clasificacion, natural_key, incluido)
		VALUES ($1::uuid, $2::uuid, DATE '2026-11-05', $3, 'DEPOSITO EMPATADO', 0, 777.00,
		        'CRC', 777.00, 777.00, $4::uuid, $5::uuid, 'REVISADO', $6, true)`
	for i := 0; i < empatados; i++ {
		if _, err := repo.pool.Exec(ctx, q, empresaPrueba, cuentaBuena,
			fmt.Sprintf("EMP-%03d", i), conceptoID, partidaMia, fmt.Sprintf("empate-%03d", i)); err != nil {
			t.Fatalf("sembrar el empatado %d: %v", i, err)
		}
	}

	for _, orden := range []string{"fecha_desc", "fecha_asc", "monto_desc", "monto_asc"} {
		orden := orden
		t.Run(orden, func(t *testing.T) {
			vistos := map[string]int{}
			const pageSize = 7 // no divide a 120: la última página queda incompleta, el borde más frágil
			paginas := (empatados + pageSize - 1) / pageSize
			for page := 1; page <= paginas; page++ {
				f := FiltrosMovimientos{
					Periodo: "2026-11", Tipo: "CREDITO", Alcance: []string{partidaMia},
					Orden: orden, Page: page, PageSize: pageSize,
				}
				got, err := repo.ListarMovimientos(ctx, empresaPrueba, f)
				if err != nil {
					t.Fatalf("página %d: %v", page, err)
				}
				if got.Total != empatados {
					t.Fatalf("página %d: total = %d, se esperaban %d", page, got.Total, empatados)
				}
				for _, it := range got.Items {
					vistos[it.ID]++
				}
			}
			repetidas := 0
			for _, veces := range vistos {
				if veces > 1 {
					repetidas++
				}
			}
			if len(vistos) != empatados || repetidas > 0 {
				t.Errorf("con el orden %q se vieron %d filas distintas de %d (%d repetidas): al pasar de página "+
					"una fila salta y otra no aparece nunca — el ORDER BY no desempata", orden, len(vistos), empatados, repetidas)
			}
		})
	}
}
