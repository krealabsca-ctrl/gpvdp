package bancos

// «Mi partida en bancos» — revisión adversarial del 22-set-2026. Cada prueba de acá estaba roja
// antes del arreglo que la acompaña:
//
//  1. «Falta un movimiento» daba 500 con un crédito SIN clasificar (el caso más común: 563 en
//     setiembre), porque `clasificacion_id = ANY(...)` da NULL y el scan a bool revienta. Ahora la
//     búsqueda separa dos grupos con un CASE —de mi partida, y el resto— y lo que no es de la partida
//     se CUENTA sin traer ni un dato. (El 23-set-2026 lo sin clasificar pasó al segundo grupo: tuvo
//     veredicto propio mientras existió la pestaña «Todavía sin partida», que el Director Financiero
//     mandó quitar.)
//  2. La fila traía el MOTIVO del aviso abierto de cualquier persona (y de faltantes que el servidor
//     enganchó), y la RESPUESTA de avisos ajenos. Ahora: que hay un aviso abierto se dice siempre; el
//     motivo solo si es propio y no es faltante; el resuelto, solo el propio.
//  3. `page=200000000000000000` desbordaba el OFFSET a negativo: 500 en las dos listas.
//  4. La mutación `if len(f.Alcance) > 0 || …` en condicionesMovimientos dejaba toda la suite verde.
//
// Contra Postgres en el mismo esquema temporal que segmento_sin_partida_test.go (no se escribe en
// public). Sin base de datos se OMITEN; las pruebas con dobles del repositorio corren siempre.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
)

// sembrarCredito agrega un crédito al escenario de «Mi partida» y devuelve su id. `partida` vacía =
// sin clasificar.
func sembrarCredito(t *testing.T, repo *pgRepository, nombre, cuenta, fecha, credito, partida string) string {
	t.Helper()
	concepto, estado := "", "NO_IDENTIFICADO"
	if partida != "" {
		concepto, estado = conceptoID, "REVISADO"
	}
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
			 estado_clasificacion, natural_key, incluido)
		VALUES ($1::uuid, $2::uuid, $3::date, $4, $5, 0, $6, 'CRC', $6, $6,
		        NULLIF($7,'')::uuid, NULLIF($8,'')::uuid, $9, $10, true)
		RETURNING id::text`,
		empresaPrueba, cuenta, fecha, "DOC-"+nombre, "DEPOSITO "+strings.ToUpper(nombre), credito,
		concepto, partida, estado, "rev-"+nombre).Scan(&id); err != nil {
		t.Fatalf("sembrar %s: %v", nombre, err)
	}
	return id
}

// routerReal monta las rutas del equipo sobre el SERVICIO REAL (y por lo tanto el SQL real), con el
// usuario ya autenticado como lo deja el middleware. Es la forma de reproducir un 500 que nace en el
// repositorio: con el doble, la consulta rota ni se ejecuta.
func routerReal(svc *Service, usuarioID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(svc, zap.NewNop())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		auth.SetClaims(c, &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: usuarioID},
			EmpresaID:        empresaPrueba,
		})
		c.Next()
	})
	r.GET("/v1/bancos/mi-segmento/movimientos", h.MiSegmento)
	r.GET("/v1/bancos/mi-segmento/mis-avisos", h.MisAvisos)
	r.POST("/v1/bancos/mi-segmento/buscar", h.BuscarFaltante)
	return r
}

func pedir(t *testing.T, r *gin.Engine, metodo, url, cuerpo string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(metodo, url, strings.NewReader(cuerpo))
	if cuerpo != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// ── 1. «Falta un movimiento» con un crédito sin partida ─────────────────────────────────────────────

func TestFaltaUnMovimientoSinPartidaEnMisCuentas(t *testing.T) {
	repo, svc, ids := baseMiPartida(t)
	ctx := context.Background()
	alcance := []string{partidaMia}

	t.Run("por HTTP: el crédito sin partida de mi cuenta ya no es un 500, y no se muestra", func(t *testing.T) {
		// sp-01: ₡100 del 01/12 en Promerica VDP, sin partida. Antes: «cannot scan NULL into *bool».
		// Desde el 23-set-2026 tampoco se enseña: el veredicto dice que existe, y nada más.
		code, cuerpo := pedir(t, routerReal(svc, usuarioPrueba), http.MethodPost,
			"/v1/bancos/mi-segmento/buscar", `{"fecha":"2026-12-01","monto":"100"}`)
		if code != http.StatusOK {
			t.Fatalf("status = %d, se esperaba 200: %s", code, cuerpo)
		}
		var res ResultadoFaltante
		if err := json.Unmarshal([]byte(cuerpo), &res); err != nil {
			t.Fatalf("cuerpo ilegible: %v — %s", err, cuerpo)
		}
		if res.Veredicto != FaltanteFueraDeMiPartida {
			t.Fatalf("veredicto = %q, se esperaba %q", res.Veredicto, FaltanteFueraDeMiPartida)
		}
		if len(res.Movimientos) != 0 {
			t.Fatalf("movimientos = %+v: lo que nadie clasificó no se muestra", res.Movimientos)
		}
		// Ni por el cuerpo crudo: el id, la cuenta y la referencia del sp-01 no pueden salir de acá.
		for _, prohibido := range []string{ids["sp-01"], "DOC-sp-01", "Promerica VDP"} {
			if strings.Contains(cuerpo, prohibido) {
				t.Fatalf("la respuesta filtra %q: %s", prohibido, cuerpo)
			}
		}
	})

	casos := []struct {
		nombre, fecha, monto string
		mios                 []string
		fuera                int
		porque               string
	}{
		{"de mi partida", "2026-12-14", "1000", []string{"mio-1"}, 0, "mio-1 es de la partida"},
		{"sin partida en una cuenta del segmento", "2026-12-01", "100", nil, 1,
			"nadie lo clasificó: existe, y el usuario no lo ve en ningún lado"},
		{"sin partida en la OTRA cuenta del segmento", "2026-12-02", "200", nil, 1,
			"que la cuenta sea del segmento ya no lo hace visible"},
		{"sin partida en una cuenta que NO es del segmento", "2026-12-01", "999", nil, 1,
			"Promerica Colinas no es del segmento: se cuenta y no se devuelve"},
		{"sin partida en la cuenta ajena", "2026-12-01", "555", nil, 1, "BN Ajena no es del segmento"},
		{"de otra partida en una cuenta del segmento", "2026-12-14", "30000", nil, 1,
			"tiene partida, y no es la mía"},
		{"excluido", "2026-12-13", "50000", nil, 0, "un duplicado revertido no entró a los libros"},
	}
	for _, c := range casos {
		t.Run("repositorio: "+c.nombre, func(t *testing.T) {
			mios, fuera, err := repo.BuscarPorFechaYMonto(ctx, empresaPrueba, c.fecha, dec(c.monto), alcance)
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			idsDe := func(rows []MovimientoRow) []string {
				out := []string{}
				for _, r := range rows {
					out = append(out, r.ID)
				}
				return out
			}
			esperar := func(nombres []string) []string {
				out := []string{}
				for _, n := range nombres {
					out = append(out, ids[n])
				}
				return out
			}
			if strings.Join(idsDe(mios), ",") != strings.Join(esperar(c.mios), ",") || fuera != c.fuera {
				t.Fatalf("mios=%v fuera=%d; se esperaba mios=%v fuera=%d (%s)",
					idsDe(mios), fuera, c.mios, c.fuera, c.porque)
			}
		})
	}

	t.Run("precedencia: de mi partida, y todo lo demás solo se cuenta", func(t *testing.T) {
		// El 20: uno de cada clase. El 21: sin partida en una cuenta del segmento, otra partida, y sin
		// partida en otra cuenta — ninguno se ve. El 22: solo lo que no ve.
		mio := sembrarCredito(t, repo, "prec-mio", cuentaBuena, "2026-12-20", "4321.00", partidaMia)
		sembrarCredito(t, repo, "prec-sp", cuentaSegunda, "2026-12-20", "4321.00", "")
		sembrarCredito(t, repo, "prec-ajeno", cuentaAjena, "2026-12-20", "4321.00", partidaAjena)
		sembrarCredito(t, repo, "prec2-sp", cuentaBuena, "2026-12-21", "4321.00", "")
		sembrarCredito(t, repo, "prec2-ajeno", cuentaAjena, "2026-12-21", "4321.00", partidaAjena)
		sembrarCredito(t, repo, "prec2-mala", cuentaMala, "2026-12-21", "4321.00", "")
		sembrarCredito(t, repo, "prec3-ajeno", cuentaBuena, "2026-12-22", "4321.00", partidaAjena)
		sembrarCredito(t, repo, "prec3-mala", cuentaMala, "2026-12-22", "4321.00", "")

		esperado := []struct {
			fecha, veredicto string
			ids              []string
		}{
			// Gana la partida, y viaja SOLO el de la partida: los otros dos del mismo día no se mezclan.
			{"2026-12-20", FaltanteEnMiPartida, []string{mio}},
			// Tres que el usuario no ve, uno de ellos sin clasificar en una cuenta suya: existe y basta.
			{"2026-12-21", FaltanteFueraDeMiPartida, nil},
			{"2026-12-22", FaltanteFueraDeMiPartida, nil},
			{"2026-12-23", FaltanteNoExiste, nil},
		}
		for _, e := range esperado {
			res, err := svc.BuscarFaltante(ctx, empresaPrueba, usuarioPrueba, e.fecha, "4.321,00")
			if err != nil {
				t.Fatalf("%s: error inesperado: %v", e.fecha, err)
			}
			got := []string{}
			for _, m := range res.Movimientos {
				got = append(got, m.ID)
			}
			if res.Veredicto != e.veredicto || strings.Join(got, ",") != strings.Join(e.ids, ",") {
				t.Errorf("%s: veredicto=%q movimientos=%v; se esperaba %q con %v", e.fecha, res.Veredicto, got,
					e.veredicto, e.ids)
			}
		}
	})

	// La otra mitad del cierre del 23-set-2026: no basta con no mostrarlo. Si la guarda del aviso
	// siguiera aceptando un crédito sin clasificar, mandando ids a mano se podría confirmar que
	// existe —y quedaría un aviso sobre un movimiento que la pantalla no enseña.
	t.Run("tampoco se puede avisar sobre un crédito que nadie clasificó", func(t *testing.T) {
		err := svc.ReportarSegmentacion(ctx, empresaPrueba, usuarioPrueba, ids["sp-01"],
			"es el depósito de ventanilla de Rojas")
		if !errors.Is(err, ErrFueraDeAlcance) {
			t.Fatalf("esperaba ErrFueraDeAlcance, obtuve %v", err)
		}
		// Y el de la partida sí, para que la prueba no pase por estar todo cerrado.
		if err := svc.ReportarSegmentacion(ctx, empresaPrueba, usuarioPrueba, ids["mio-1"],
			"esto es de Emergencias"); err != nil {
			t.Fatalf("avisar sobre un crédito de mi partida: %v", err)
		}
	})
}

// Con el doble: la precedencia y la forma de la respuesta, sin base de datos.
func TestBuscarFaltanteDosGruposPrecedencia(t *testing.T) {
	t.Parallel()
	mio := MovimientoRow{ID: "mio", Credito: "4950"}
	mio2 := MovimientoRow{ID: "mio-2", Credito: "4950"}
	casos := []struct {
		nombre    string
		mios      []MovimientoRow
		fuera     int
		veredicto string
		ids       string
	}{
		{"de mi partida gana a todo", []MovimientoRow{mio}, 2, FaltanteEnMiPartida, "mio"},
		{"varios de mi partida viajan todos", []MovimientoRow{mio, mio2}, 0, FaltanteEnMiPartida, "mio,mio-2"},
		{"solo fuera: el veredicto y nada más", nil, 1, FaltanteFueraDeMiPartida, ""},
		{"nada", nil, 0, FaltanteNoExiste, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{alcance: []string{"clasif-1"}, busquedaMios: c.mios, busquedaFuera: c.fuera}
			res, err := servicioSegmento(repo).BuscarFaltante(context.Background(), "emp-1", "usr-1", "2026-09-11", "15000")
			if err != nil {
				t.Fatalf("no esperaba error: %v", err)
			}
			got := []string{}
			for _, m := range res.Movimientos {
				got = append(got, m.ID)
			}
			if res.Veredicto != c.veredicto || strings.Join(got, ",") != c.ids {
				t.Fatalf("veredicto=%q movimientos=%v; se esperaba %q con [%s]", res.Veredicto, got, c.veredicto, c.ids)
			}
			// La fecha de carga acompaña SOLO a NO_EXISTE.
			if repo.cargaCuentasPedida != (c.veredicto == FaltanteNoExiste) {
				t.Fatalf("carga pedida = %v con el veredicto %q", repo.cargaCuentasPedida, res.Veredicto)
			}
		})
	}

	t.Run("una fila de mi partida que ya está en revisión llega marcada", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			alcance:          []string{"clasif-1"},
			busquedaMios:     []MovimientoRow{mio},
			reportesAbiertos: map[string]AvisoAbiertoDeFila{"mio": {Propio: false}},
		}
		res, err := servicioSegmento(repo).BuscarFaltante(context.Background(), "emp-1", "usr-1", "2026-09-11", "15000")
		if err != nil {
			t.Fatalf("no esperaba error: %v", err)
		}
		m := res.Movimientos[0]
		if m.ReporteAbierto != TextoAvisoAbiertoDeOtro || m.ReporteAbiertoPropio == nil || *m.ReporteAbiertoPropio {
			t.Fatalf("reporte_abierto = %q, propio = %s", m.ReporteAbierto, valorPropio(m.ReporteAbiertoPropio))
		}
	})
}

// ── 2. La fila no lleva motivos ni respuestas ajenas ────────────────────────────────────────────────

func TestLaFilaNoLlevaMotivosNiRespuestasAjenas(t *testing.T) {
	repo, svc, ids := baseMiPartida(t)
	ctx := context.Background()
	diciembre := FiltrosMovimientos{Periodo: "2026-12", PageSize: 100}

	abrir := func(mov, usuario, motivo string) string {
		t.Helper()
		id, err := repo.CrearReporteSegmentacion(ctx, empresaPrueba, ids[mov], usuario, motivo)
		if err != nil {
			t.Fatalf("aviso sobre %s: %v", mov, err)
		}
		return id
	}
	faltante := func(mov, usuario, fecha, monto, motivo string) string {
		t.Helper()
		id, err := repo.CrearReporteFaltante(ctx, empresaPrueba, usuario, fecha, dec(monto), "REC-9", motivo, ids[mov])
		if err != nil {
			t.Fatalf("faltante enganchado a %s: %v", mov, err)
		}
		return id
	}
	resolver := func(repID, respuesta string) {
		t.Helper()
		if err := repo.ResolverReporteSegmentacion(ctx, empresaPrueba, repID, usuarioOtro, ResolucionSinCambio, respuesta); err != nil {
			t.Fatalf("resolver: %v", err)
		}
	}

	// Dos créditos más de MI partida, para los casos de aviso ya resuelto: el escenario base solo
	// trae tres, y los tres se usan con avisos abiertos.
	ids["mio-3"] = sembrarCredito(t, repo, "mio-3", cuentaBuena, "2026-12-16", "1234.00", partidaMia)
	ids["mio-4"] = sembrarCredito(t, repo, "mio-4", cuentaBuena, "2026-12-17", "1234.00", partidaMia)

	// Abiertos.
	abrir("mio-1", usuarioOtro, "MOTIVO-AJENO-abierto")                           // de otra persona
	faltante("mio-2", usuarioPrueba, "2026-12-15", "1000", "MOTIVO-FALTANTE-mio") // faltante MÍO, enganchado
	abrir("mio-segunda", usuarioPrueba, "MOTIVO-PROPIO")                          // el único que es mío
	// Resueltos.
	resolver(abrir("mio-3", usuarioOtro, "MOTIVO-AJENO-resuelto"), "RESPUESTA-AJENA")
	resolver(faltante("mio-4", usuarioPrueba, "2026-12-17", "1234", "MOTIVO-FALTANTE-resuelto"), "RESPUESTA-FALTANTE")

	res, err := svc.MiSegmento(ctx, empresaPrueba, usuarioPrueba, VistaPartida, diciembre)
	if err != nil {
		t.Fatalf("mi partida: %v", err)
	}
	porID := map[string]MovimientoRow{}
	for _, it := range res.Movimientos.Items {
		porID[it.ID] = it
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("serializar: %v", err)
	}
	crudo := string(b)
	fila := func(mov string) MovimientoRow {
		t.Helper()
		it, ok := porID[ids[mov]]
		if !ok {
			t.Fatalf("%s no está en «Mi partida»", mov)
		}
		return it
	}

	t.Run("el aviso abierto se dice siempre; el motivo, solo si es mío y no es faltante", func(t *testing.T) {
		casos := []struct {
			mov    string
			texto  string
			propio bool
			porque string
		}{
			{"mio-1", TextoAvisoAbiertoDeOtro, false, "es de otra persona"},
			{"mio-2", TextoAvisoAbiertoDeOtro, false, "es un faltante (aunque sea mío)"},
			{"mio-segunda", "MOTIVO-PROPIO", true, "es mío"},
		}
		for _, c := range casos {
			it := fila(c.mov)
			if it.ReporteAbierto != c.texto || it.ReporteAbiertoPropio == nil || *it.ReporteAbiertoPropio != c.propio {
				t.Errorf("%s: reporte_abierto=%q propio=%s; se esperaba %q/%v (%s)",
					c.mov, it.ReporteAbierto, valorPropio(it.ReporteAbiertoPropio), c.texto, c.propio, c.porque)
			}
		}
	})

	t.Run("el resuelto de otra persona y el faltante resuelto no llegan a mi fila", func(t *testing.T) {
		for _, mov := range []string{"mio-3", "mio-4"} {
			if it := fila(mov); it.AvisoResuelto != nil || it.ReporteAbierto != "" {
				t.Errorf("%s trae %+v / %q", mov, it.AvisoResuelto, it.ReporteAbierto)
			}
		}
	})

	t.Run("ningún texto ajeno viaja en la respuesta", func(t *testing.T) {
		for _, prohibido := range []string{"MOTIVO-AJENO", "MOTIVO-FALTANTE", "RESPUESTA-AJENA", "RESPUESTA-FALTANTE"} {
			if strings.Contains(crudo, prohibido) {
				t.Errorf("la respuesta de «Mi partida» contiene %q", prohibido)
			}
		}
		// Los dos casos se distinguen por un CAMPO, no por el texto.
		if !strings.Contains(crudo, `"reporte_abierto_propio":false`) || !strings.Contains(crudo, `"reporte_abierto_propio":true`) {
			t.Error("falta reporte_abierto_propio en el JSON: la pantalla no puede distinguir el motivo propio del texto genérico")
		}
	})

	t.Run("sigue sin poder avisar otra vez sobre un movimiento con aviso abierto ajeno", func(t *testing.T) {
		// Por eso la fila tiene que seguir diciendo que hay uno: si el botón reapareciera, daría 409.
		err := svc.ReportarSegmentacion(ctx, empresaPrueba, usuarioPrueba, ids["mio-1"], "no es nuestro")
		if !errors.Is(err, ErrReporteYaAbierto) {
			t.Fatalf("esperaba ErrReporteYaAbierto, obtuve %v", err)
		}
	})

	t.Run("quien avisó ve su propio motivo", func(t *testing.T) {
		res, err := svc.MiSegmento(ctx, empresaPrueba, usuarioOtro, VistaPartida, diciembre)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		visto := false
		for _, it := range res.Movimientos.Items {
			if it.ID != ids["mio-1"] {
				continue
			}
			visto = true
			if it.ReporteAbierto != "MOTIVO-AJENO-abierto" || it.ReporteAbiertoPropio == nil || !*it.ReporteAbiertoPropio {
				t.Fatalf("a quien avisó: %q / propio=%s", it.ReporteAbierto, valorPropio(it.ReporteAbiertoPropio))
			}
		}
		if !visto {
			t.Fatal("control: mio-1 no está en la partida de quien avisó")
		}
	})
}

// valorPropio imprime ReporteAbiertoPropio legible en un mensaje de error (nil, true o false).
func valorPropio(p *bool) string {
	if p == nil {
		return "nil"
	}
	if *p {
		return "true"
	}
	return "false"
}

// ── 3. Una página absurda no desborda el OFFSET ─────────────────────────────────────────────────────

func TestPaginaFueraDeRangoNoDesbordaElOffset(t *testing.T) {
	repo, svc, _ := baseMiPartida(t)
	ctx := context.Background()

	// Con 100 por página, 1e17 da (1e17-1)·100 ≈ 1e19: pasa de 2^63 y vuelve NEGATIVO. Con 50, 2e17
	// también. Antes: «OFFSET must not be negative», un 500.
	for _, c := range []struct{ page, pageSize int }{{100000000000000000, 100}, {200000000000000000, 50}, {paginaMaxima + 1, 500}} {
		got, err := repo.ListarMovimientos(ctx, empresaPrueba, FiltrosMovimientos{
			Periodo: "2026-12", Tipo: "CREDITO", Alcance: []string{partidaMia}, Page: c.page, PageSize: c.pageSize})
		if err != nil {
			t.Fatalf("page=%d page_size=%d: %v", c.page, c.pageSize, err)
		}
		// Igual que un page_size fuera de rango: vuelve al defecto, y la respuesta lo dice.
		if got.Page != 1 || len(got.Items) != got.Total || got.Total == 0 {
			t.Fatalf("page=%d: página=%d items=%d total=%d; se esperaba la página 1 completa", c.page, got.Page, len(got.Items), got.Total)
		}
	}

	// La última página válida sigue siendo válida (vacía, con el total real).
	got, err := repo.ListarMovimientos(ctx, empresaPrueba, FiltrosMovimientos{
		Periodo: "2026-12", Tipo: "CREDITO", Alcance: []string{partidaMia}, Page: paginaMaxima, PageSize: 500})
	if err != nil || got.Page != paginaMaxima || len(got.Items) != 0 || got.Total == 0 {
		t.Fatalf("página máxima: page=%d items=%d total=%d (%v)", got.Page, len(got.Items), got.Total, err)
	}

	avisos, err := svc.MisAvisos(ctx, empresaPrueba, usuarioPrueba, 200000000000000000, 50)
	if err != nil || avisos.Page != 1 {
		t.Fatalf("mis avisos: page=%d (%v)", avisos.Page, err)
	}

	t.Run("por HTTP, las dos listas responden 200", func(t *testing.T) {
		r := routerReal(svc, usuarioPrueba)
		for _, url := range []string{
			"/v1/bancos/mi-segmento/movimientos?page=200000000000000000&page_size=50",
			"/v1/bancos/mi-segmento/movimientos?page=100000000000000000",
			"/v1/bancos/mi-segmento/mis-avisos?page=200000000000000000",
		} {
			if code, cuerpo := pedir(t, r, http.MethodGet, url, ""); code != http.StatusOK {
				t.Errorf("%s: status = %d: %s", url, code, cuerpo)
			}
		}
	})
}

// ── 4. La vista principal con alcance vacío cierra EN EL REPOSITORIO ────────────────────────────────

// El servicio corta antes (ErrSinAlcance), así que sin esta prueba el `false` de
// condicionesMovimientos no lo protegía nadie: la mutación `if len(f.Alcance) > 0 || …` dejaba la
// suite entera verde y abría la vista principal a toda la empresa.
func TestVistaPrincipalAlcanceVacioCierraEnElRepositorio(t *testing.T) {
	repo, _, _ := baseMiPartida(t)
	ctx := context.Background()

	con, err := repo.ListarMovimientos(ctx, empresaPrueba, FiltrosMovimientos{Tipo: "CREDITO", Alcance: []string{partidaMia}})
	if err != nil || con.Total == 0 {
		t.Fatalf("control: con el alcance de verdad hay %d filas (%v)", con.Total, err)
	}

	got, err := repo.ListarMovimientos(ctx, empresaPrueba, FiltrosMovimientos{Tipo: "CREDITO", Alcance: []string{}})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if got.Total != 0 || len(got.Items) != 0 || !igualDecimal(t, got.Totales.TotalCreditos, "0") {
		t.Fatalf("con alcance vacío la vista principal devolvió %d filas por %s: un alcance vacío tiene que CERRAR",
			got.Total, got.Totales.TotalCreditos)
	}

	// `nil` NO es un alcance: es la hoja de trabajo de Clasificar, que no recorta (y por eso no puede
	// cerrar). Lo que impide que la consulta por segmento llegue con nil es que AlcanceDeUsuario
	// devuelve SIEMPRE un slice no nulo; si algún día devolviera nil para un rol sin partidas, la
	// guarda de arriba no se aplicaría.
	t.Run("un rol sin partidas recibe []string{}, nunca nil", func(t *testing.T) {
		alcance, err := repo.AlcanceDeUsuario(ctx, empresaPrueba, usuarioSinAlcance)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if alcance == nil || len(alcance) != 0 {
			t.Fatalf("alcance = %#v, se esperaba []string{} (no nil)", alcance)
		}
	})
}
