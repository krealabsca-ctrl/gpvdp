package server

// `/readyz` tiene que GRITAR cuando el sistema está vivo pero no sirve (21-oct-2026).
//
// Cada caso de acá corresponde a algo que PASÓ en este proyecto y que desde afuera se veía igual
// que un sistema sano. Esa es la razón de ser del endpoint: `/healthz` devuelve «ok» sin mirar
// nada, así que un monitor apuntado ahí confirmaba que el proceso respiraba mientras nadie podía
// entrar.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type estadoFalso struct {
	pingErr error
	version int64
	sucio   bool
	esqErr  error
	hay     bool
	hayErr  error
}

func (e estadoFalso) Ping(context.Context) error { return e.pingErr }
func (e estadoFalso) Esquema(context.Context) (int64, bool, error) {
	return e.version, e.sucio, e.esqErr
}
func (e estadoFalso) HayEmpresas(context.Context) (bool, error) { return e.hay, e.hayErr }

func pedir(t *testing.T, estado EstadoDelSistema) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/readyz", readyz(estado))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	var cuerpo map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	return w.Code, cuerpo
}

func TestListoCuandoTodoEstaBien(t *testing.T) {
	code, cuerpo := pedir(t, estadoFalso{version: 106, hay: true})
	if code != http.StatusOK {
		t.Errorf("código %d, esperaba 200", code)
	}
	if cuerpo["listo"] != true {
		t.Errorf("listo = %v, esperaba true", cuerpo["listo"])
	}
}

// La base caída: lo más obvio, y lo que `/healthz` no ve.
func TestNoEstaListoSiLaBaseNoResponde(t *testing.T) {
	code, _ := pedir(t, estadoFalso{pingErr: errors.New("timeout"), hay: true})
	if code != http.StatusServiceUnavailable {
		t.Errorf("código %d con la base caída, esperaba 503", code)
	}
}

// LA MIGRACIÓN A MEDIAS. Pasó el 21-oct-2026 con la 0106: el esquema queda incompleto y el
// problema aparece días después, en una pantalla cualquiera, sin que nadie lo relacione.
func TestNoEstaListoConMigracionAMedias(t *testing.T) {
	code, cuerpo := pedir(t, estadoFalso{version: 106, sucio: true, hay: true})
	if code != http.StatusServiceUnavailable {
		t.Errorf("código %d con el esquema sucio, esperaba 503", code)
	}
	if cuerpo["sucio"] != true {
		t.Error("la respuesta no dice que el esquema quedó a medias: sin eso hay que ir a buscarlo a la base")
	}
}

// SIN EMPRESAS. Pasó de verdad: la siembra fallaba en silencio desde la migración 0067, el
// arranque seguía adelante y quedaba un sistema sano donde NADIE puede entrar —el ingreso exige
// elegir una empresa—. Desde afuera se veía perfecto.
func TestNoEstaListoSinEmpresas(t *testing.T) {
	code, cuerpo := pedir(t, estadoFalso{version: 106, hay: false})
	if code != http.StatusServiceUnavailable {
		t.Errorf("código %d sin ninguna empresa, esperaba 503: el sistema está instalado y nadie puede entrar", code)
	}
	if d, _ := cuerpo["detalle"].(string); d == "" {
		t.Error("no se explica el motivo: un 503 sin detalle obliga a adivinar qué revisar")
	}
}

// Sin verificador no se afirma nada. Afirmar «listo» sin haber mirado es peor que no tener el
// endpoint: un monitor en verde sobre un sistema que nadie comprobó.
func TestSinVerificadorNoSeAfirmaQueEstaListo(t *testing.T) {
	code, cuerpo := pedir(t, nil)
	if code != http.StatusServiceUnavailable {
		t.Errorf("código %d sin verificador, esperaba 503", code)
	}
	if cuerpo["listo"] != false {
		t.Error("se declaró listo sin haber comprobado nada")
	}
}
