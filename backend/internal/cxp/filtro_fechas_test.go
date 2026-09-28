package cxp

// Filtrar por fechas en la Bandeja y en Recepción (25-set-2026).
//
// El Director pidió «que se pueda filtrar por fechas» en CxP y «todos los filtros que hacen falta»
// en Recepción. Lo que protegen estas pruebas no es que el filtro exista, sino las dos formas en
// que un filtro de fechas miente:
//
//   · la columna: «qué facturó el proveedor en agosto» NO es «qué se vence en agosto». Si el campo
//     se elige mal —o si el selector se pudiera manipular—, la lista contesta otra pregunta;
//   · el borde: una fecha a medio teclear («0002-08-01», lo que emite el navegador mientras se
//     escribe el año) tiene que rebotar. Si llega al `::date` es un 500, y si llega como «0202»
//     devuelve una lista vacía que se lee como un filtro que funcionó. Ese defecto ya nos costó
//     una vez en Bancos.

import "testing"

func TestColumnaFechaSoloDeLaWhitelist(t *testing.T) {
	t.Parallel()
	casos := []struct{ campo, quiere, porque string }{
		{FechaEmision, "d.fecha_emision", "la de siempre"},
		{FechaVencimiento, "d.fecha_vencimiento", "la que ordena «Por pagar»"},
		{FechaPago, "d.fecha_pago_programada", "la del corte"},
		{"", "d.fecha_emision", "sin campo manda el defecto de la pantalla"},
		{"inventado", "d.fecha_emision", "un campo desconocido NO puede abrir otra columna"},
		{"d.total_crc", "d.fecha_emision", "un nombre de columna real tampoco pasa"},
		{"fecha_emision; DROP TABLE documento_cxp", "d.fecha_emision", "esto se concatena al SQL"},
	}
	for _, c := range casos {
		if got := columnaFecha(c.campo); got != c.quiere {
			t.Errorf("columnaFecha(%q) = %q, se esperaba %q (%s)", c.campo, got, c.quiere, c.porque)
		}
	}
}

func TestFechaISOValidaCortaLoQueElNavegadorEmiteMientrasSeEscribe(t *testing.T) {
	t.Parallel()
	validas := []string{"2026-08-01", "2025-01-15", "2026-02-28", "1000-01-01"}
	for _, v := range validas {
		if !FechaISOValida(v) {
			t.Errorf("FechaISOValida(%q) = false, tenía que aceptarla", v)
		}
	}
	// Las tres primeras son literalmente lo que Chrome manda tecleando «2026» en el campo de año.
	invalidas := map[string]string{
		"0002-08-01": "año a medio teclear: reventaba en Postgres",
		"0202-08-01": "año a medio teclear: devolvía el histórico y parecía un filtro",
		"0000-08-01": "año cero",
		"2026-02-31": "día que no existe en el calendario",
		"2026-13-01": "mes 13",
		"01/08/2026": "formato del usuario, no ISO",
		"2026-8-1":   "sin ceros a la izquierda no es aaaa-mm-dd",
		"2026-08":    "mes sin día",
		"":           "vacío no es una fecha (el llamador lo trata como «sin filtro»)",
		"hoy":        "texto",
	}
	for v, porque := range invalidas {
		if FechaISOValida(v) {
			t.Errorf("FechaISOValida(%q) = true, tenía que rechazarla (%s)", v, porque)
		}
	}
}

// Contra Postgres de verdad: las tres fechas de un documento son DISTINTAS a propósito, así que
// cada filtro tiene que devolver un conjunto distinto. Si el selector no llegara al SQL, los tres
// darían lo mismo y la prueba no lo notaría con fechas iguales.
func TestElRangoDeFechasFiltraPorLaColumnaElegida(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := t.Context()

	// Emitida en JULIO, vence en AGOSTO, programada para SETIEMBRE.
	desfasada := documentoConFechas(t, repo, "F-DESFASADA", "2026-07-15", "2026-08-15", "2026-09-15")
	// Todo en agosto: el control que tiene que aparecer siempre.
	agosto := documentoConFechas(t, repo, "F-AGOSTO", "2026-08-10", "2026-08-20", "2026-08-25")

	casos := []struct {
		nombre string
		campo  string
		quiere []string
	}{
		{"emisión (por defecto)", "", []string{agosto}},
		{"emisión explícita", FechaEmision, []string{agosto}},
		{"vencimiento", FechaVencimiento, []string{agosto, desfasada}},
		{"pago programado", FechaPago, []string{agosto}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			lista, err := repo.ListarDocumentos(ctx, empresaLote, FiltrosDocumentos{
				Desde: "2026-08-01", Hasta: "2026-08-31", CampoFecha: c.campo,
			})
			if err != nil {
				t.Fatalf("listar: %v", err)
			}
			got := map[string]bool{}
			for _, d := range lista.Items {
				got[d.ID] = true
			}
			if len(got) != len(c.quiere) {
				t.Fatalf("devolvió %d documento(s), se esperaban %d", len(got), len(c.quiere))
			}
			for _, id := range c.quiere {
				if !got[id] {
					t.Errorf("falta el documento %s en el filtro por %s", id, c.nombre)
				}
			}
			// El total del paginador tiene que contar lo MISMO que la lista, o el pie miente.
			if lista.Total != len(c.quiere) {
				t.Errorf("total = %d, items = %d: el conteo no aplica el filtro", lista.Total, len(c.quiere))
			}
		})
	}

	t.Run("sin fechas devuelve todo", func(t *testing.T) {
		lista, err := repo.ListarDocumentos(ctx, empresaLote, FiltrosDocumentos{CampoFecha: FechaVencimiento})
		if err != nil {
			t.Fatalf("listar: %v", err)
		}
		if lista.Total != 2 {
			t.Fatalf("total = %d, se esperaban 2: el rango vacío no filtra", lista.Total)
		}
	})

	t.Run("el rango es inclusivo en las dos puntas", func(t *testing.T) {
		lista, err := repo.ListarDocumentos(ctx, empresaLote, FiltrosDocumentos{
			Desde: "2026-08-10", Hasta: "2026-08-10",
		})
		if err != nil {
			t.Fatalf("listar: %v", err)
		}
		if lista.Total != 1 || lista.Items[0].ID != agosto {
			t.Fatalf("total = %d: el día exacto de emisión tiene que entrar", lista.Total)
		}
	})
}

// documentoConFechas siembra un documento con sus tres fechas distintas y devuelve su id.
func documentoConFechas(t *testing.T, repo *pgRepository, nombre, emision, vencimiento, pago string) string {
	t.Helper()
	var id string
	err := repo.pool.QueryRow(t.Context(), `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, fecha_vencimiento,
			 fecha_pago_programada, estado, total, total_crc, moneda)
		VALUES ($1::uuid, $2::uuid, $3, $3, $4::date, $5::date, $6::date, 'APROBADO', 1000, 1000, 'CRC')
		RETURNING id::text`,
		empresaLote, provLote, nombre, emision, vencimiento, pago).Scan(&id)
	if err != nil {
		t.Fatalf("sembrar %s: %v", nombre, err)
	}
	return id
}
