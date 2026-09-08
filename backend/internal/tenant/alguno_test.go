package tenant

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// checkerFalso responde por lista de permisos concedidos y cuenta las consultas.
type checkerFalso struct {
	concedidos map[string]bool
	err        error
	consultas  []string
}

func (c *checkerFalso) Tiene(_ context.Context, _, _, permiso string) (bool, error) {
	c.consultas = append(c.consultas, permiso)
	if c.err != nil {
		return false, c.err
	}
	return c.concedidos[permiso], nil
}

func pedir(t *testing.T, chk PermisoChecker, permisos ...string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	g := r.Group("")
	g.Use(RequireAuth(secret), RequireAlgunPermiso(chk, permisos...))
	g.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token(t, "e1", "AUXILIAR_FINANCIERO"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Con CUALQUIERA de los permisos alcanza: es un OR, no un AND.
func TestAlgunPermisoAlcanzaConUno(t *testing.T) {
	casos := []struct {
		nombre    string
		concedido string
	}{
		{"el primero", "bancos.ver_clasificar"},
		{"el del medio", "bancos.ver_dashboard"},
		{"el último", "inventario.entrada"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			chk := &checkerFalso{concedidos: map[string]bool{c.concedido: true}}
			w := pedir(t, chk, "bancos.ver_clasificar", "bancos.ver_dashboard", "bancos.exportar", "inventario.entrada")
			if w.Code != http.StatusOK {
				t.Errorf("con %q concedido: status = %d, quiere 200", c.concedido, w.Code)
			}
		})
	}
}

// Sin ninguno, 403. Deny-by-default.
func TestAlgunPermisoSinNingunoEs403(t *testing.T) {
	chk := &checkerFalso{concedidos: map[string]bool{"otra.cosa": true}}
	w := pedir(t, chk, "bancos.ver_clasificar", "bancos.ver_dashboard")
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, quiere 403", w.Code)
	}
}

// Se corta en el primero que da true: no tiene sentido seguir consultando la matriz.
func TestAlgunPermisoCortaEnElPrimeroQueSirve(t *testing.T) {
	chk := &checkerFalso{concedidos: map[string]bool{"bancos.ver_clasificar": true}}
	if w := pedir(t, chk, "bancos.ver_clasificar", "bancos.ver_dashboard", "bancos.exportar"); w.Code != http.StatusOK {
		t.Fatalf("status = %d, quiere 200", w.Code)
	}
	if len(chk.consultas) != 1 {
		t.Errorf("consultó %v; debería haber cortado en el primero", chk.consultas)
	}
}

// Un fallo del checker es 500, NO 403.
//
// Importa la diferencia: «no tiene permiso» y «no se pudo averiguar» son cosas distintas, y
// devolver 403 cuando la base está caída manda a todo el mundo a revisar permisos que están bien.
func TestAlgunPermisoConErrorEs500YNoSigue(t *testing.T) {
	chk := &checkerFalso{err: errors.New("base caída")}
	w := pedir(t, chk, "bancos.ver_clasificar", "bancos.ver_dashboard")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, quiere 500", w.Code)
	}
	if len(chk.consultas) != 1 {
		t.Errorf("consultó %v; con un error no debe seguir probando los demás", chk.consultas)
	}
}

// Sin permisos en la lista nadie pasa: una llamada mal escrita no puede quedar abierta.
func TestAlgunPermisoSinListaNiegaTodo(t *testing.T) {
	chk := &checkerFalso{concedidos: map[string]bool{"bancos.ver": true}}
	if w := pedir(t, chk); w.Code != http.StatusForbidden {
		t.Errorf("status = %d, quiere 403: una lista vacía no autoriza a nadie", w.Code)
	}
}
