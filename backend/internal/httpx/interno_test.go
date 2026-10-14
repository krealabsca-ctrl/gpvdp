package httpx

// La referencia de incidente tiene que cumplir las dos mitades del trato: el usuario la VE y el
// log la TIENE. Si falla cualquiera de las dos, es peor que no tenerla — promete una trazabilidad
// que no existe y manda a soporte a buscar algo que no está escrito.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestAbortInternoDaLaMismaReferenciaAlUsuarioYAlLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	nucleo, grabado := observer.New(zapcore.ErrorLevel)
	log := zap.New(nucleo)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPatch, "/v1/cxp/proveedores/abc", nil)

	causa := errors.New("expected 17 arguments, got 16")
	AbortInterno(c, log, "cxp actualizar-proveedor", causa)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quería 500", rec.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if resp.Code != CodeErrorInterno {
		t.Errorf("code = %q", resp.Code)
	}

	// 1 · EL USUARIO VE UNA REFERENCIA. Sin esto vuelve a quedar «error interno» a secas.
	ref := referenciaDe(resp.Message)
	if ref == "" {
		t.Fatalf("el mensaje no trae referencia: %q", resp.Message)
	}

	// 2 · EL LOG TIENE ESA MISMA REFERENCIA Y LA CAUSA REAL.
	entradas := grabado.All()
	if len(entradas) != 1 {
		t.Fatalf("entradas de log = %d, quería 1", len(entradas))
	}
	e := entradas[0]
	if e.Message != "cxp actualizar-proveedor" {
		t.Errorf("operación logueada = %q", e.Message)
	}
	campos := e.ContextMap()
	if campos["ref"] != ref {
		t.Errorf("ref del log = %v, la que vio el usuario = %q — sin esto no se puede correlacionar",
			campos["ref"], ref)
	}
	if err, _ := campos["error"].(string); !strings.Contains(err, "expected 17 arguments") {
		t.Errorf("el log no guarda la causa real: %v", campos["error"])
	}

	// 3 · LA CAUSA NO SALE A LA PANTALLA. Un error interno trae nombres de tablas y fragmentos de
	// SQL; eso describe la base por dentro y no va al navegador.
	if strings.Contains(resp.Message, "17 arguments") {
		t.Errorf("el mensaje al usuario filtró el error interno: %q", resp.Message)
	}
}

// Sin logger se responde igual: un handler mal cableado no puede dejar al usuario sin respuesta.
func TestAbortInternoSinLoggerNoRevienta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/algo", nil)

	AbortInterno(c, nil, "prueba", errors.New("x"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quería 500", rec.Code)
	}
	var resp ErrorResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if referenciaDe(resp.Message) == "" {
		t.Errorf("sin logger tampoco puede faltar la referencia: %q", resp.Message)
	}
}

// Dos incidentes distintos no pueden compartir referencia: si todas fueran iguales, buscarla en
// el log devolvería cien líneas y no serviría para nada.
func TestLasReferenciasNoSeRepiten(t *testing.T) {
	vistas := map[string]bool{}
	for i := 0; i < 200; i++ {
		r := NuevaReferencia()
		if len(r) != 6 {
			t.Fatalf("referencia %q: largo %d, quería 6", r, len(r))
		}
		if r == "SINREF" {
			t.Fatal("se usó el valor de emergencia sin que el generador fallara")
		}
		if strings.ToUpper(r) != r {
			t.Errorf("referencia %q no está en mayúsculas: se dicta por teléfono", r)
		}
		vistas[r] = true
	}
	// Con 16,7 millones de combinaciones, 200 sorteos repetidos serían un generador roto.
	if len(vistas) < 195 {
		t.Errorf("de 200 referencias solo %d distintas", len(vistas))
	}
}

// referenciaDe saca el código del mensaje: son los últimos 6 caracteres tras el último espacio.
func referenciaDe(msg string) string {
	i := strings.LastIndex(msg, " ")
	if i < 0 {
		return ""
	}
	ref := msg[i+1:]
	if len(ref) != 6 {
		return ""
	}
	return ref
}
