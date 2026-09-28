package bancos

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Un identificador mal escrito en el query string tiene que salir por 400, no por 500: antes
// llegaba al cast `::uuid[]` de Postgres y el cliente recibía «error interno» por un typo.
func TestValidarFiltros(t *testing.T) {
	t.Parallel()
	const bueno = "7341057e-c233-4ae2-8a21-84f366fb46b2"

	casos := []struct {
		nombre    string
		f         FiltrosMovimientos
		valido    bool
		paramMalo string
	}{
		{"vacío es válido (sin restricción)", FiltrosMovimientos{}, true, ""},
		{"uuid bueno singular", FiltrosMovimientos{ConceptoID: bueno}, true, ""},
		{"uuid bueno en lista", FiltrosMovimientos{ClasificacionIDs: []string{bueno, bueno}}, true, ""},
		{"clasificación no-uuid", FiltrosMovimientos{ClasificacionIDs: []string{"no-es-uuid"}}, false, "clasificaciones"},
		{"una buena y una mala en la lista", FiltrosMovimientos{ConceptoIDs: []string{bueno, "xx"}}, false, "conceptos"},
		{"concepto singular mal", FiltrosMovimientos{ConceptoID: "123"}, false, "concepto_id"},
		{"cuenta singular mal", FiltrosMovimientos{CuentaID: "'; DROP TABLE movimiento_bancario; --"}, false, "cuenta_bancaria_id"},
		{"largo correcto pero con letra no hex", FiltrosMovimientos{BancoID: "7341057e-c233-4ae2-8a21-84f366fb46bZ"}, false, "banco_id"},
		{"guion fuera de lugar", FiltrosMovimientos{BancoID: "7341057ec-233-4ae2-8a21-84f366fb46b2"}, false, "banco_id"},
		{"período bueno", FiltrosMovimientos{Periodos: []string{"2026-08"}}, true, ""},
		{"período mal formado", FiltrosMovimientos{Periodos: []string{"agosto"}}, false, "periodos"},
		{"período singular bueno", FiltrosMovimientos{Periodo: "2026-09"}, true, ""},
		{"período singular mal formado", FiltrosMovimientos{Periodo: "setiembre"}, false, "periodo"},
		// Tenía la forma (siete caracteres y un guion) pero no es un mes: pasaba, llegaba al SQL y
		// devolvía cero filas sin avisar, que se lee como «ese mes no entró nada».
		{"período con mes 13", FiltrosMovimientos{Periodo: "2026-13"}, false, "periodo"},
		{"período con mes 00", FiltrosMovimientos{Periodo: "2026-00"}, false, "periodo"},
		{"lista con un mes 13", FiltrosMovimientos{Periodos: []string{"2026-08", "2026-13"}}, false, "periodos"},
		{"período de año cero", FiltrosMovimientos{Periodo: "0000-09"}, false, "periodo"},

		// Las fechas del rango. Cada caso es un 500 «error interno» reproducido contra el módulo
		// (o, peor, un filtro que filtraba otra cosa en silencio).
		{"rango bueno", FiltrosMovimientos{Desde: "2026-09-01", Hasta: "2026-09-30"}, true, ""},
		{"solo desde", FiltrosMovimientos{Desde: "2026-09-01"}, true, ""},
		{"solo hasta", FiltrosMovimientos{Hasta: "2026-09-30"}, true, ""},
		// El año a medio teclear del selector de fecha del navegador: Go lo parsea (año cero) y
		// Postgres lo rechaza con «date/time field value out of range» → 500.
		{"año cero", FiltrosMovimientos{Desde: "0000-09-01"}, false, "desde"},
		// Estos tres Postgres SÍ los acepta, así que no dan 500: dan el HISTÓRICO COMPLETO en
		// silencio, como si fueran un filtro. Son los estados intermedios del selector de fecha
		// mientras se teclea «2026». Con el piso de año en 1 pasaban.
		{"año de tres cifras con cero adelante", FiltrosMovimientos{Desde: "0202-09-01"}, false, "desde"},
		{"año de dos cifras con ceros adelante", FiltrosMovimientos{Desde: "0020-09-01"}, false, "desde"},
		{"año de una cifra con ceros adelante", FiltrosMovimientos{Hasta: "0002-09-30"}, false, "hasta"},
		{"el primer año que se acepta", FiltrosMovimientos{Desde: "1000-01-01"}, true, ""},
		{"31 de febrero", FiltrosMovimientos{Desde: "2026-02-31"}, false, "desde"},
		{"mes 13", FiltrosMovimientos{Desde: "2026-13-01"}, false, "desde"},
		// Postgres SÍ entiende esto, y con DateStyle=MDY lo lee como el 9 de enero: el filtro no
		// filtra lo que el usuario cree y nadie se entera.
		{"fecha con barras", FiltrosMovimientos{Desde: "01/09/2026"}, false, "desde"},
		{"mes sin día", FiltrosMovimientos{Hasta: "2026-09"}, false, "hasta"},
		{"solo el año", FiltrosMovimientos{Desde: "2026"}, false, "desde"},
		{"texto libre", FiltrosMovimientos{Hasta: "abc"}, false, "hasta"},
		{"fecha con hora", FiltrosMovimientos{Desde: "2026-09-01T00:00:00"}, false, "desde"},
	}

	for _, c := range casos {
		c := c
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			param, ok := validarFiltros(c.f)
			if ok != c.valido {
				t.Fatalf("validarFiltros() válido = %v, se esperaba %v (param %q)", ok, c.valido, param)
			}
			if !c.valido && param != c.paramMalo {
				t.Fatalf("señaló el parámetro %q, se esperaba %q", param, c.paramMalo)
			}
		})
	}
}

// Un rango al revés NO es una lista vacía. Antes salía 200 con cero filas y la pantalla lo
// explicaba como «ningún movimiento de tu partida coincide con lo que filtraste»: el equipo leía
// «esa plata no entró» cuando lo único que pasaba era que las dos fechas estaban invertidas.
func TestRangoDeFechasAlRevesSeExplica(t *testing.T) {
	t.Parallel()

	motivo := motivoFiltrosInvalidos(FiltrosMovimientos{Desde: "2026-09-30", Hasta: "2026-09-01"})
	if motivo == "" {
		t.Fatal("el rango invertido pasó como válido: la respuesta sería una lista vacía sin explicación")
	}
	// El mensaje tiene que nombrar las dos fechas: «revisá el rango» no dice cuál hay que mover.
	for _, texto := range []string{"hasta", "desde", "2026-09-01", "2026-09-30"} {
		if !contieneTexto(motivo, texto) {
			t.Errorf("el mensaje %q no menciona %q", motivo, texto)
		}
	}
	// El mismo día en las dos puntas es un rango legítimo de un día, no un error.
	if m := motivoFiltrosInvalidos(FiltrosMovimientos{Desde: "2026-09-11", Hasta: "2026-09-11"}); m != "" {
		t.Errorf("un solo día no puede ser inválido: %q", m)
	}
	if m := motivoFiltrosInvalidos(FiltrosMovimientos{Desde: "2026-09-01", Hasta: "2026-09-30"}); m != "" {
		t.Errorf("un rango normal no puede ser inválido: %q", m)
	}
}

// El borde HTTP: lo que el Director vio fue «error interno». Estos son los valores exactos que lo
// producían, y ahora tienen que salir por 400 con un mensaje que se pueda leer y corregir.
func TestFiltrosDeFechaRespondenCuatrocientosYNoQuinientos(t *testing.T) {
	gin.SetMode(gin.TestMode)

	casos := []struct {
		nombre  string
		query   string
		enTexto string
	}{
		{"año a medio teclear", "desde=0000-09-01", "desde"},
		{"día que no existe", "desde=2026-02-31", "desde"},
		{"mes que no existe", "hasta=2026-13-01", "hasta"},
		{"fecha con barras", "desde=01/09/2026", "desde"},
		{"rango al revés", "desde=2026-09-30&hasta=2026-09-01", "al revés"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequest(http.MethodGet,
				"/v1/bancos/mi-segmento/movimientos?"+c.query, nil)

			if abortarSiFiltrosInvalidos(ctx, filtrosDeQuery(ctx)) {
				t.Fatal("el filtro pasó: la cadena cruda llegaría al ::date de Postgres y saldría 500")
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba 400 (escribir mal una fecha no es una caída del servidor)", rec.Code)
			}
			var cuerpo struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("cuerpo ilegible: %v — %s", err, rec.Body.String())
			}
			if cuerpo.Code == "ERROR_INTERNO" {
				t.Fatalf("salió como error interno: %s", rec.Body.String())
			}
			if !contieneTexto(cuerpo.Message, c.enTexto) {
				t.Errorf("el mensaje tiene que decir qué corregir y mencionar %q; dice %q", c.enTexto, cuerpo.Message)
			}
		})
	}

	// Y el contrario: una fecha bien escrita no puede quedar bloqueada por la validación nueva.
	t.Run("un rango correcto pasa", func(t *testing.T) {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodGet,
			"/v1/bancos/mi-segmento/movimientos?desde=2026-09-01&hasta=2026-09-30&periodo=2026-09&page=2&page_size=50", nil)
		if !abortarSiFiltrosInvalidos(ctx, filtrosDeQuery(ctx)) {
			t.Fatalf("un rango correcto fue rechazado: %s", rec.Body.String())
		}
	})
}

// «Falta un movimiento» tiene su propia puerta de fecha y ahí también mordía el año cero: el
// time.Parse suelto lo dejaba pasar y Postgres lo rechazaba con 500 al guardar el aviso. Ahora usa
// el mismo validador que los filtros, así que las dos puertas no pueden volver a separarse.
func TestFaltaUnMovimientoRechazaLasFechasQueLosFiltrosRechazan(t *testing.T) {
	t.Parallel()
	for _, fecha := range []string{"0000-09-01", "0202-09-01", "0020-09-01", "2026-02-31", "2026-13-01", "01/09/2026", "2026-09", ""} {
		fecha := fecha
		t.Run(fecha, func(t *testing.T) {
			t.Parallel()
			if _, _, err := validarFechaYMonto(fecha, "5000"); !errors.Is(err, ErrFechaInvalida) {
				t.Fatalf("validarFechaYMonto(%q) err = %v, se esperaba ErrFechaInvalida", fecha, err)
			}
			// Y la regla es la MISMA que la de los filtros: si una acepta y la otra rechaza, la
			// pantalla deja buscar algo que después no deja avisar (o al revés).
			if fechaISOValida(fecha) {
				t.Fatalf("fechaISOValida(%q) = true, pero «falta un movimiento» la rechaza", fecha)
			}
		})
	}
	if _, _, err := validarFechaYMonto("2026-09-17", "4 950,50"); err != nil {
		t.Fatalf("una fecha y un monto buenos no pueden fallar: %v", err)
	}
}
