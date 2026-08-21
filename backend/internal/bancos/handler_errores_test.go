package bancos

// El test que faltaba es el del BORDE, no el del lector.
//
// Los tres centinelas del archivo de clasificación estaban probados en el lector, pero nadie probaba
// qué hacía la capa HTTP con ellos: caían al `default` de responderError y salían como 500 «error
// interno». El mensaje redactado —«partilo por cuenta o por año»— nunca llegaba a la pantalla, y el
// usuario no podía distinguir un archivo rechazado de una caída del servidor.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestResponderErrorTraduceLosRechazosDeArchivo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, zap.NewNop())

	casos := []struct {
		nombre  string
		err     error
		enTexto string
	}{
		{"encabezado no reconocido", ErrClasifExcelSinEncabezado, "encabezado"},
		{"archivo sin filas útiles", ErrClasifExcelVacio, "no trae filas"},
		{"archivo demasiado grande", ErrClasifExcelDemasiadasFilas, "partilo por cuenta o por año"},
		{"diccionario sin encabezado", ErrDiccionarioSinEncabezado, "encabezado"},
		{"diccionario vacío", ErrDiccionarioVacio, "concepto"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/bancos/movimientos/clasificar-excel", nil)

			h.responderError(ctx, c.err, "test")

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, se esperaba 422 (un archivo rechazado no es una caída del servidor)", rec.Code)
			}
			var cuerpo struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("cuerpo ilegible: %v — %s", err, rec.Body.String())
			}
			if cuerpo.Code == "ERROR_INTERNO" {
				t.Fatalf("el error salió como interno: %s", rec.Body.String())
			}
			if !contieneTexto(cuerpo.Message, c.enTexto) {
				t.Errorf("el mensaje debería explicar qué hacer y decir %q; dice %q", c.enTexto, cuerpo.Message)
			}
		})
	}
}

// contieneTexto evita depender de strings en este archivo por una sola comprobación.
func contieneTexto(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexDe(s, sub) >= 0)
}

func indexDe(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
