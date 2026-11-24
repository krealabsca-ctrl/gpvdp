package server

// Que «error interno» se pueda canjear (22-oct-2026, punto 4 de la revisión comercial).
//
// EL PEDIDO: «nadie se entera si se cae». La referencia que el usuario ve en pantalla existía
// desde el 12-oct, pero quedaba solo en el log del contenedor: un código que no se podía canjear
// por nada.
//
// LO QUE SE PRUEBA ACÁ ES LA CADENA COMPLETA, no sus partes. Que el servicio sepa guardar un
// incidente no prueba que el que se guarda sea EL MISMO que el usuario tiene en la mano. Si la
// referencia del cuerpo y la del registro no coinciden, el sistema funciona perfecto y el código
// sigue sin servir para nada.

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/httpx"
)

// registroFalso guarda lo que se le manda, para poder mirarlo.
type registroFalso struct{ guardados []IncidenteRegistrado }

func (r *registroFalso) Registrar(_ context.Context, in IncidenteRegistrado) {
	r.guardados = append(r.guardados, in)
}

// servir monta el middleware sobre un handler de prueba y devuelve la respuesta y lo registrado.
func servir(t *testing.T, handler gin.HandlerFunc) (*httptest.ResponseRecorder, *registroFalso) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	reg := &registroFalso{}
	r := gin.New()
	r.Use(incidentes(zap.NewNop(), reg))
	r.GET("/v1/prueba", handler)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/prueba", nil))
	return w, reg
}

// refDelCuerpo saca los seis caracteres del mensaje que ve el usuario.
func refDelCuerpo(t *testing.T, cuerpo []byte) string {
	t.Helper()
	var r httpx.ErrorResponse
	if err := json.Unmarshal(cuerpo, &r); err != nil {
		t.Fatalf("la respuesta no es el contrato de error: %q", string(cuerpo))
	}
	i := strings.LastIndex(r.Message, " ")
	if i < 0 {
		t.Fatalf("el mensaje no trae referencia: %q", r.Message)
	}
	return r.Message[i+1:]
}

// LA PRUEBA QUE IMPORTA: el código que el usuario lee es el que quedó guardado.
func TestLaReferenciaQueVeElUsuarioEsLaQueSeGuarda(t *testing.T) {
	w, reg := servir(t, func(c *gin.Context) {
		httpx.AbortInterno(c, zap.NewNop(), "prueba: guardar", errors.New("expected 17 arguments, got 16"))
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("código %d, esperaba 500", w.Code)
	}
	if len(reg.guardados) != 1 {
		t.Fatalf("se guardaron %d incidentes, esperaba 1", len(reg.guardados))
	}
	g := reg.guardados[0]
	if ref := refDelCuerpo(t, w.Body.Bytes()); g.Referencia != ref {
		t.Errorf("la referencia guardada (%s) no es la que el usuario tiene en pantalla (%s): el código no se puede canjear",
			g.Referencia, ref)
	}
	// Y con la causa real al lado, que es lo que hace que el canje sirva de algo.
	if !strings.Contains(g.Detalle, "expected 17 arguments") {
		t.Errorf("detalle = %q, esperaba el error real", g.Detalle)
	}
	if g.Operacion != "prueba: guardar" {
		t.Errorf("operacion = %q, esperaba la que se le pasó a AbortInterno", g.Operacion)
	}
	if g.Panico {
		t.Error("un 500 normal quedó marcado como panic")
	}
}

// EL PANIC: antes daba 500 SIN cuerpo. El usuario veía una pantalla rota y no tenía nada que
// dictar, así que el error era irrastreable por diseño.
func TestUnPanicRespondeConReferenciaYQuedaRegistrado(t *testing.T) {
	w, reg := servir(t, func(*gin.Context) {
		panic("índice fuera de rango")
	})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("código %d, esperaba 500", w.Code)
	}
	ref := refDelCuerpo(t, w.Body.Bytes())
	if len(reg.guardados) != 1 {
		t.Fatalf("se guardaron %d incidentes, esperaba 1", len(reg.guardados))
	}
	g := reg.guardados[0]
	if g.Referencia != ref {
		t.Errorf("referencia guardada %s ≠ la de la pantalla %s", g.Referencia, ref)
	}
	if !g.Panico {
		t.Error("el incidente no quedó marcado como panic: se mezcla con los 500 normales")
	}
	// El estado se fuerza a 500 y no se lee del writer: un panic a mitad del handler deja el
	// status en 200 y el incidente se escondería de cualquier filtro por error.
	if g.Estado != http.StatusInternalServerError {
		t.Errorf("estado = %d, esperaba 500", g.Estado)
	}
	if !strings.Contains(g.Traza, "incidentes_test.go") {
		t.Error("la traza no apunta al lugar del panic: sin eso el registro no dice dónde mirar")
	}
	if !strings.Contains(g.Detalle, "índice fuera de rango") {
		t.Errorf("detalle = %q, esperaba la causa del panic", g.Detalle)
	}
}

// La ruta se guarda como PATRÓN, no como URL: si guardara la URL real, cien errores del mismo
// defecto se verían como cien rutas distintas y ninguna se podría agrupar.
func TestSeGuardaElPatronDeLaRutaYNoLosIdentificadores(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := &registroFalso{}
	r := gin.New()
	r.Use(incidentes(zap.NewNop(), reg))
	r.GET("/v1/cxp/documentos/:id", func(c *gin.Context) {
		httpx.AbortInterno(c, zap.NewNop(), "cxp: documento", errors.New("boom"))
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/cxp/documentos/7f3a-2c91", nil))

	if len(reg.guardados) != 1 {
		t.Fatalf("se guardaron %d incidentes, esperaba 1", len(reg.guardados))
	}
	if got := reg.guardados[0].Ruta; got != "/v1/cxp/documentos/:id" {
		t.Errorf("ruta = %q, esperaba el patrón: con el identificador adentro no se agrupa nada", got)
	}
}

// Lo que NO es un incidente: el cliente que cierra la pestaña a mitad de una descarga. Contarlo
// llenaría la pantalla de alarmas falsas el día que alguien baje un reporte grande con mala señal.
func TestLaConexionCortadaPorElClienteNoEsUnIncidente(t *testing.T) {
	_, reg := servir(t, func(*gin.Context) {
		panic(&net.OpError{
			Op:  "write",
			Net: "tcp",
			Err: os.NewSyscallError("write", errors.New("broken pipe")),
		})
	})
	if len(reg.guardados) != 0 {
		t.Errorf("se registró %d incidente(s) por una conexión que el cliente cortó", len(reg.guardados))
	}
}

// Un 200 no se registra. Parece obvio, y es la diferencia entre una tabla que se lee y una que
// crece con todo el tráfico del sistema.
func TestLoQueSaleBienNoSeRegistra(t *testing.T) {
	_, reg := servir(t, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	if len(reg.guardados) != 0 {
		t.Errorf("se registraron %d incidentes de una respuesta exitosa", len(reg.guardados))
	}
}

// Y un 4xx tampoco: «no encontrado» o «sin permiso» son respuestas correctas del sistema, no
// fallas suyas. Mezclarlas haría que el contador de errores midiera el uso, no los problemas.
func TestUn4xxNoEsUnIncidente(t *testing.T) {
	_, reg := servir(t, func(c *gin.Context) {
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, "no existe")
	})
	if len(reg.guardados) != 0 {
		t.Errorf("se registraron %d incidentes de un 404", len(reg.guardados))
	}
}

// Sin registro inyectado el middleware sigue siendo un Recovery: atrapa el panic y responde. Se
// pierde la persistencia, no la respuesta —y sobre todo, no se cae el servidor—.
func TestSinRegistroSigueAtrapandoElPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(incidentes(zap.NewNop(), nil))
	r.GET("/v1/prueba", func(*gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/prueba", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("código %d, esperaba 500", w.Code)
	}
	if ref := refDelCuerpo(t, w.Body.Bytes()); len(ref) != 6 {
		t.Errorf("referencia %q: el usuario tiene que recibir una igual", ref)
	}
}
