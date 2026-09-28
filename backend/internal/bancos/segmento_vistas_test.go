package bancos

// «Mi partida en bancos» del lado del SERVICIO y del BORDE HTTP (con dobles del repositorio). Lo
// que hace el SQL se prueba contra Postgres en segmento_sin_partida_test.go.
//
//   1. La pantalla tiene UNA vista. `sin_clasificar` —la pestaña «Todavía sin partida» que el
//      Director Financiero mandó quitar el 23-set-2026— se RECHAZA, no se sirve la partida en su
//      lugar: un cliente viejo tiene que enterarse de que esa lista ya no existe.
//   2. La respuesta de un aviso resuelto se ve en la fila, y «Mis avisos» existe aunque el
//      movimiento haya salido del alcance.
//   3. «Cargado hasta» es la cuenta del segmento más atrasada, nombrada.
//
// Y en todos: un alcance vacío CIERRA. El doble del repositorio devuelve datos a propósito —son
// la trampa—, así que si el servicio llegara a consultar, la prueba lo vería.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
)

func TestMiSegmentoSinVistaEsLaDeLaPartida(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{alcance: []string{"clasif-depositos"}}
	svc := servicioSegmento(repo)

	// El cliente no manda vista e intenta además los débitos: lo segundo no se respeta.
	res, err := svc.MiSegmento(context.Background(), "emp-1", "usr-1", "", FiltrosMovimientos{Tipo: "DEBITO"})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if res.Vista != VistaPartida {
		t.Fatalf("vista = %q, se esperaba %q", res.Vista, VistaPartida)
	}
	// El recorte por partida es lo ÚNICO que arma la lista: si el alcance no llegara al repositorio,
	// la pantalla se abriría a toda la empresa.
	if len(repo.filtroMovs.Alcance) != 1 || repo.filtroMovs.Alcance[0] != "clasif-depositos" {
		t.Fatalf("el alcance tiene que salir del rol, llegó %v", repo.filtroMovs.Alcance)
	}
	if repo.filtroMovs.Tipo != "CREDITO" {
		t.Fatalf("la pantalla es de ingresos: esperaba CREDITO, llegó %q", repo.filtroMovs.Tipo)
	}
}

// Una vista que no es «partida» NO cae en la principal, y `sin_clasificar` es el caso que importa:
// era la pestaña «Todavía sin partida» hasta el 23-set-2026 («esto no debe ser visible por ningún
// motivo a los consultores»). Si cayera en la principal, un cliente viejo pintaría los créditos de
// la partida bajo el rótulo de la lista que ya no existe, y las dos afirman cosas distintas.
func TestMiSegmentoVistaDesconocidaEsUnError(t *testing.T) {
	t.Parallel()
	for _, vista := range []string{"sin_clasificar", " SIN_CLASIFICAR ", "todas"} {
		t.Run(vista, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{
				alcance:   []string{"clasif-1"},
				listaMovs: ListaMovimientos{Items: []MovimientoRow{{ID: "no-tenia-que-salir"}}, Total: 1},
			}
			svc := servicioSegmento(repo)

			res, err := svc.MiSegmento(context.Background(), "emp-1", "usr-1", vista, FiltrosMovimientos{})
			if !errors.Is(err, ErrVistaInvalida) {
				t.Fatalf("esperaba ErrVistaInvalida, obtuve %v", err)
			}
			if len(res.Movimientos.Items) != 0 || repo.filtroMovs.Alcance != nil {
				t.Fatal("con una vista inválida no debería ni consultar los movimientos")
			}
		})
	}
}

// La respuesta de un aviso resuelto llega a la fila, pero solo cuando no hay uno abierto: el
// abierto es lo vigente, y mostrar las dos cosas diría «resuelto» y «en revisión» a la vez.
func TestMiSegmentoMuestraLaRespuestaDelAvisoResuelto(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		alcance: []string{"clasif-1"},
		listaMovs: ListaMovimientos{Items: []MovimientoRow{
			{ID: "solo-resuelto"}, {ID: "resuelto-y-reabierto"}, {ID: "sin-avisos"},
		}},
		reportesAbiertos: map[string]AvisoAbiertoDeFila{"resuelto-y-reabierto": {Propio: true, Motivo: "sigue sin ser nuestro"}},
		avisosResueltos: map[string]AvisoResuelto{
			"solo-resuelto": {Motivo: "no es de Depósitos", Resolucion: ResolucionSinCambio,
				Respuesta: "Es de ustedes: es la planilla de setiembre", ResueltoEn: "2026-09-20T15:04:05Z"},
			"resuelto-y-reabierto": {Motivo: "no es nuestro", Resolucion: ResolucionSinCambio,
				Respuesta: "Sí es", ResueltoEn: "2026-09-19T10:00:00Z"},
		},
	}
	svc := servicioSegmento(repo)

	res, err := svc.MiSegmento(context.Background(), "emp-1", "usr-1", VistaPartida, FiltrosMovimientos{})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	porID := map[string]MovimientoRow{}
	for _, it := range res.Movimientos.Items {
		porID[it.ID] = it
	}
	if a := porID["solo-resuelto"].AvisoResuelto; a == nil ||
		a.Respuesta != "Es de ustedes: es la planilla de setiembre" || a.ResueltoEn == "" {
		t.Fatalf("la fila con un aviso resuelto tiene que traer la respuesta y cuándo, trajo %+v", a)
	}
	if it := porID["resuelto-y-reabierto"]; it.ReporteAbierto != "sigue sin ser nuestro" || it.AvisoResuelto != nil {
		t.Fatalf("con un aviso abierto manda el abierto: reporte_abierto=%q aviso_resuelto=%+v",
			it.ReporteAbierto, it.AvisoResuelto)
	}
	if it := porID["sin-avisos"]; it.ReporteAbierto != "" || it.AvisoResuelto != nil {
		t.Fatalf("una fila sin avisos no puede traer ninguno: %+v", it)
	}
}

func TestCuentaMasAtrasadaEsElMinimoDeLosMaximos(t *testing.T) {
	t.Parallel()
	davi := CuentaCargadaHasta{ID: "davi", Banco: "Davivienda", Cuenta: "Davivienda Colones", CargadoHasta: "2026-09-11"}
	bn := CuentaCargadaHasta{ID: "bn", Banco: "BN", Cuenta: "BN Privado de Cartago", CargadoHasta: "2026-09-09"}
	bac := CuentaCargadaHasta{ID: "bac", Banco: "BAC", Cuenta: "BAC Religiosa", CargadoHasta: "2026-09-09"}

	casos := []struct {
		nombre string
		cargas []CuentaCargadaHasta
		fecha  string
		cuenta string
	}{
		{"sin cuentas no hay fecha que afirmar", nil, "", ""},
		{"una sola cuenta", []CuentaCargadaHasta{davi}, "2026-09-11", "davi"},
		// El orden de la lista NO decide: la más atrasada viene al final y tiene que ganar igual.
		{"la más atrasada aunque venga al final", []CuentaCargadaHasta{davi, bn}, "2026-09-09", "bn"},
		{"en un empate se nombra la primera", []CuentaCargadaHasta{davi, bac, bn}, "2026-09-09", "bac"},
		{"una cuenta sin fecha no gana", []CuentaCargadaHasta{{ID: "vacia"}, davi}, "2026-09-11", "davi"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			fecha, cuenta := cuentaMasAtrasada(c.cargas)
			if fecha != c.fecha {
				t.Fatalf("fecha = %q, se esperaba %q", fecha, c.fecha)
			}
			if c.cuenta == "" {
				if cuenta != nil {
					t.Fatalf("sin fecha no puede nombrar una cuenta: %+v", cuenta)
				}
				return
			}
			if cuenta == nil || cuenta.ID != c.cuenta || cuenta.CargadoHasta != c.fecha {
				t.Fatalf("cuenta = %+v, se esperaba %q con %q", cuenta, c.cuenta, c.fecha)
			}
		})
	}
}

// El encabezado y el diálogo de «Falta un movimiento» usan la MISMA fecha: si no, uno dice «cargado
// hasta el 11» y el otro «conviene esperar, está cargado hasta el 9».
func TestCargadoHastaEsElMismoEnElEncabezadoYEnFaltaUnMovimiento(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{alcance: []string{"clasif-1"}, cargaCuentas: []CuentaCargadaHasta{
		{ID: "bn", Banco: "BN", Cuenta: "BN Privado de Cartago", CargadoHasta: "2026-09-09"},
		{ID: "davi", Banco: "Davivienda", Cuenta: "Davivienda Colones", CargadoHasta: "2026-09-11"},
	}}
	svc := servicioSegmento(repo)

	seg, err := svc.MiSegmento(context.Background(), "emp-1", "usr-1", VistaPartida, FiltrosMovimientos{})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	busq, err := svc.BuscarFaltante(context.Background(), "emp-1", "usr-1", "2026-09-10", "4950")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if seg.CargadoHasta != "2026-09-09" || seg.CuentaMasAtrasada == nil || seg.CuentaMasAtrasada.ID != "bn" {
		t.Fatalf("encabezado: %q / %+v, se esperaba el BN al 2026-09-09", seg.CargadoHasta, seg.CuentaMasAtrasada)
	}
	if busq.CargadoHasta != seg.CargadoHasta || busq.CargadoHastaCuenta == nil ||
		busq.CargadoHastaCuenta.ID != seg.CuentaMasAtrasada.ID {
		t.Fatalf("el diálogo dice %q (%+v) y el encabezado %q: se contradicen",
			busq.CargadoHasta, busq.CargadoHastaCuenta, seg.CargadoHasta)
	}
	if len(seg.CargaPorCuenta) != 2 {
		t.Fatalf("carga por cuenta = %d, se esperaban las 2 cuentas del segmento", len(seg.CargaPorCuenta))
	}
}

func TestMisAvisosConAlcanceVacioNoConsulta(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		alcance:   nil,
		misAvisos: ListaMisAvisos{Items: []MiAviso{{ID: "aviso-que-no-debe-verse"}}, Total: 1},
	}
	svc := servicioSegmento(repo)

	res, err := svc.MisAvisos(context.Background(), "emp-1", "usr-1", 1, 50)
	if !errors.Is(err, ErrSinAlcance) {
		t.Fatalf("esperaba ErrSinAlcance, obtuve %v", err)
	}
	if repo.misAvisosPedidos {
		t.Fatal("un alcance vacío cierra: ni siquiera se consultan los avisos")
	}
	if !res.SinAlcance || len(res.Items) != 0 || res.Items == nil {
		t.Fatalf("la respuesta tiene que venir marcada y con la lista vacía (no nula): %+v", res)
	}
}

func TestMisAvisosNormalizaLaPagina(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre         string
		page, pageSize int
		quierePage     int
		quierePageSize int
	}{
		{"por defecto", 0, 0, 1, 50},
		{"un tamaño que el paginador ofrece", 2, 200, 2, 200},
		{"demasiado grande vuelve al defecto", 1, 5000, 1, 50},
		{"negativo vuelve al defecto", -3, -1, 1, 50},
		// (page-1)·pageSize desbordaba a un OFFSET negativo: 500 de Postgres. Igual que el tamaño,
		// una página fuera de rango vuelve al defecto.
		{"una página absurda vuelve al defecto", 200000000000000000, 50, 1, 50},
		{"la página máxima se respeta", paginaMaxima, 200, paginaMaxima, 200},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{alcance: []string{"clasif-1"}}
			svc := servicioSegmento(repo)
			if _, err := svc.MisAvisos(context.Background(), "emp-1", "usr-1", c.page, c.pageSize); err != nil {
				t.Fatalf("no esperaba error: %v", err)
			}
			if repo.misAvisosPagina != [2]int{c.quierePage, c.quierePageSize} {
				t.Fatalf("página al repositorio = %v, se esperaba [%d %d]",
					repo.misAvisosPagina, c.quierePage, c.quierePageSize)
			}
		})
	}
}

// ── El borde HTTP ────────────────────────────────────────────────────────────

// El centinela nuevo tiene que estar en el switch del handler: si no, una vista mal escrita sale
// como 500 «error interno» (ya nos pasó con los rechazos del archivo de clasificación).
func TestResponderErrorVistaInvalidaEsCuatrocientos(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, zap.NewNop())
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/bancos/mi-segmento/movimientos?vista=x", nil)

	h.responderError(ctx, ErrVistaInvalida, "test")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba 400", rec.Code)
	}
	var cuerpo struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo ilegible: %v — %s", err, rec.Body.String())
	}
	if cuerpo.Code == "ERROR_INTERNO" || !contieneTexto(cuerpo.Message, "partida") {
		t.Fatalf("el mensaje tiene que decir qué vista existe: %s", rec.Body.String())
	}
}

// routerDeSegmento monta las dos rutas con un usuario ya autenticado (lo que en producción pone el
// middleware), para probar la forma EXACTA de la respuesta que va a leer la pantalla.
func routerDeSegmento(repo *fakeRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewHandler(servicioSegmento(repo), zap.NewNop())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		auth.SetClaims(c, &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "usr-1"},
			EmpresaID:        "emp-1",
		})
		c.Next()
	})
	r.GET("/v1/bancos/mi-segmento/movimientos", h.MiSegmento)
	r.GET("/v1/bancos/mi-segmento/mis-avisos", h.MisAvisos)
	return r
}

func pedirJSON(t *testing.T, r *gin.Engine, url string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo ilegible: %v — %s", err, rec.Body.String())
	}
	return rec.Code, cuerpo
}

func TestHTTPMiSegmentoVistas(t *testing.T) {
	t.Run("la vista de la partida responde con su forma y los campos de carga", func(t *testing.T) {
		repo := &fakeRepo{
			alcance: []string{"clasif-1"},
			cargaCuentas: []CuentaCargadaHasta{
				{ID: "bn", Banco: "BN", Cuenta: "BN Privado de Cartago", CargadoHasta: "2026-09-09"},
			},
		}
		code, cuerpo := pedirJSON(t, routerDeSegmento(repo), "/v1/bancos/mi-segmento/movimientos?periodo=2026-09")
		if code != http.StatusOK {
			t.Fatalf("status = %d: %v", code, cuerpo)
		}
		if cuerpo["vista"] != VistaPartida {
			t.Fatalf("vista = %v", cuerpo["vista"])
		}
		for _, k := range []string{"partidas", "cuentas", "movimientos", "cargado_hasta", "cargado_hasta_cuenta", "carga_por_cuenta", "sin_alcance"} {
			if _, ok := cuerpo[k]; !ok {
				t.Errorf("falta la clave %q en la respuesta", k)
			}
		}
		if cuerpo["cargado_hasta"] != "2026-09-09" {
			t.Errorf("cargado_hasta = %v", cuerpo["cargado_hasta"])
		}
		cta, _ := cuerpo["cargado_hasta_cuenta"].(map[string]any)
		if cta["cuenta"] != "BN Privado de Cartago" {
			t.Errorf("cargado_hasta_cuenta = %v, tiene que nombrar la cuenta", cuerpo["cargado_hasta_cuenta"])
		}
	})

	// La prueba que importa del 23-set-2026: la URL de la pestaña que se quitó no sirve nada. Un 200
	// acá —aunque fuera la partida— significaría que el endpoint sigue abierto por ese lado.
	t.Run("la vista «sin_clasificar» ya no existe: 400, no la vista principal", func(t *testing.T) {
		for _, vista := range []string{"sin_clasificar", "todo"} {
			repo := &fakeRepo{
				alcance:   []string{"clasif-1"},
				listaMovs: ListaMovimientos{Items: []MovimientoRow{{ID: "no-tenia-que-salir"}}, Total: 1},
			}
			code, cuerpo := pedirJSON(t, routerDeSegmento(repo), "/v1/bancos/mi-segmento/movimientos?vista="+vista)
			if code != http.StatusBadRequest {
				t.Fatalf("vista=%s: status = %d, se esperaba 400: %v", vista, code, cuerpo)
			}
			if _, hay := cuerpo["movimientos"]; hay {
				t.Fatalf("vista=%s: la respuesta trae movimientos: %v", vista, cuerpo)
			}
		}
	})

	t.Run("sin alcance la respuesta vacía trae las claves nuevas", func(t *testing.T) {
		repo := &fakeRepo{alcance: nil}
		code, cuerpo := pedirJSON(t, routerDeSegmento(repo), "/v1/bancos/mi-segmento/movimientos")
		if code != http.StatusOK || cuerpo["sin_alcance"] != true {
			t.Fatalf("status = %d, cuerpo = %v", code, cuerpo)
		}
		if cuerpo["vista"] != VistaPartida || cuerpo["cargado_hasta_cuenta"] != nil {
			t.Fatalf("vista = %v, cargado_hasta_cuenta = %v", cuerpo["vista"], cuerpo["cargado_hasta_cuenta"])
		}
		if lista, ok := cuerpo["carga_por_cuenta"].([]any); !ok || len(lista) != 0 {
			t.Fatalf("carga_por_cuenta tiene que ser [] y no null: %v", cuerpo["carga_por_cuenta"])
		}
	})
}

func TestHTTPMisAvisos(t *testing.T) {
	t.Run("devuelve los avisos con su paginado", func(t *testing.T) {
		repo := &fakeRepo{
			alcance: []string{"clasif-1"},
			misAvisos: ListaMisAvisos{
				Items: []MiAviso{{ID: "av-1", Motivo: "no es nuestro", Estado: AvisoResueltoEstado,
					Resolucion: ResolucionReclasificado, Respuesta: "Lo pasé a Asociaciones"}},
				Total: 1, Resueltos: 1, Page: 1, PageSize: 50,
			},
		}
		code, cuerpo := pedirJSON(t, routerDeSegmento(repo), "/v1/bancos/mi-segmento/mis-avisos")
		if code != http.StatusOK {
			t.Fatalf("status = %d: %v", code, cuerpo)
		}
		for _, k := range []string{"items", "total", "abiertos", "resueltos", "page", "page_size", "sin_alcance"} {
			if _, ok := cuerpo[k]; !ok {
				t.Errorf("falta la clave %q en la respuesta", k)
			}
		}
		items, _ := cuerpo["items"].([]any)
		if len(items) != 1 || cuerpo["sin_alcance"] != false {
			t.Fatalf("items = %v, sin_alcance = %v", cuerpo["items"], cuerpo["sin_alcance"])
		}
	})

	t.Run("sin alcance: 200, vacío y marcado", func(t *testing.T) {
		repo := &fakeRepo{alcance: nil, misAvisos: ListaMisAvisos{Items: []MiAviso{{ID: "no"}}, Total: 1}}
		code, cuerpo := pedirJSON(t, routerDeSegmento(repo), "/v1/bancos/mi-segmento/mis-avisos")
		if code != http.StatusOK || cuerpo["sin_alcance"] != true {
			t.Fatalf("status = %d, cuerpo = %v", code, cuerpo)
		}
		if items, ok := cuerpo["items"].([]any); !ok || len(items) != 0 {
			t.Fatalf("items tiene que ser [] y no null: %v", cuerpo["items"])
		}
	})
}
