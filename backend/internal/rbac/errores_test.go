package rbac

// El BORDE: que el error salga con el estado y el mensaje correctos.
//
// Probar que una función devuelve el error correcto NO prueba que el usuario lo lea. Este paquete ya
// se tropezó con eso: un centinela nuevo sin su caso en el switch del handler sale como 500 «error
// interno», y entonces una regla de negocio esperada se ve igual que una caída del servidor.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRolDeOtraEmpresaSaleComo422YExplicaQueHacer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handler{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rbac/usuarios", nil)

	h.error(c, &RolDeOtraEmpresaError{Codigo: "CUSTOM_NOMINA", Empresas: "Valle de Paz"})

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("estado = %d, se esperaba 422 (regla de negocio, no 404 ni 500)", w.Code)
	}
	cuerpo := w.Body.String()
	// El mensaje tiene que nombrar el rol, la empresa dueña y el camino a seguir: sin eso, «rol no
	// encontrado» se lee como un bug del sistema.
	for _, esperado := range []string{"CUSTOM_NOMINA", "Valle de Paz", "Seguridad", "roles base"} {
		if !strings.Contains(cuerpo, esperado) {
			t.Errorf("el mensaje no dice %q: %s", esperado, cuerpo)
		}
	}
}

func TestRolDeOtraEmpresaNoSeConfundeConNoEncontrado(t *testing.T) {
	// Son dos situaciones con caminos distintos: una se arregla creando el rol acá, la otra revisando
	// qué se escribió. Si `errors.Is` las igualara, el switch del handler tomaría el caso equivocado.
	err := error(&RolDeOtraEmpresaError{Codigo: "X", Empresas: "Y"})
	if errors.Is(err, ErrRolNoEncontrado) {
		t.Error("un rol de otra empresa no debe contarse como «rol no encontrado»")
	}
	var ajeno *RolDeOtraEmpresaError
	if !errors.As(err, &ajeno) {
		t.Fatal("errors.As no reconoce el error tipado")
	}
	if ajeno.Codigo != "X" {
		t.Errorf("se perdió el código del rol: %q", ajeno.Codigo)
	}
}

func TestRolNoEncontradoSigueSiendo404(t *testing.T) {
	// El caso viejo no se movió: agregar un caso al switch no debe cambiar los que ya andaban.
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rbac/usuarios", nil)

	h.error(c, ErrRolNoEncontrado)

	if w.Code != http.StatusNotFound {
		t.Errorf("estado = %d, se esperaba 404", w.Code)
	}
}
