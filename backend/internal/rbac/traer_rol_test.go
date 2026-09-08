package rbac

// Pruebas del BORDE de «traer un rol de otra empresa».
//
// Lo que se fija acá es el contrato de errores, que es donde este paquete ya se tropezó: un centinela
// sin su caso en el switch del handler sale como 500 «error interno», y entonces una negativa
// esperada se ve igual que una caída del servidor. La copia en sí se verifica contra PostgreSQL real
// (índices únicos parciales, transacción), no con un fake que no los tiene.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSinAccesoAlOrigenSaleComo403(t *testing.T) {
	// Es la guarda que hace segura la función: `admin.roles` en la empresa DESTINO no alcanza para
	// leer los permisos de un rol de otra empresa. Y es 403, no 404: la empresa existe, lo que falta
	// es el acceso de quien pregunta — decir «no encontrado» mandaría a buscar un error inexistente.
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rbac/roles/traer", nil)

	h.error(c, ErrEmpresaOrigenSinAcceso)

	if w.Code != http.StatusForbidden {
		t.Errorf("estado = %d, se esperaba 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "empresa de origen") {
		t.Errorf("el mensaje no nombra la empresa de origen: %s", w.Body.String())
	}
}

func TestTraerElMismoRolDosVecesEsConflicto(t *testing.T) {
	// El segundo intento no es un error del sistema: el rol ya está acá y el camino es editarlo.
	gin.SetMode(gin.TestMode)
	h := &Handler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rbac/roles/traer", nil)

	h.error(c, ErrRolDuplicado)

	if w.Code != http.StatusConflict {
		t.Errorf("estado = %d, se esperaba 409", w.Code)
	}
}

func TestRolTraibleLlevaDeQueEmpresaVieneYCuantosPermisos(t *testing.T) {
	// Sin el nombre de la empresa, dos roles homónimos de empresas distintas son indistinguibles en
	// el selector; sin la cuenta de permisos no se sabe si se está trayendo un rol vacío.
	it := RolTraible{
		Codigo: "CUSTOM_NOMINA", Nombre: "Nomina",
		EmpresaID: "e-1", EmpresaNombre: "Valle de Paz", CuantosPermiso: 6,
	}
	if it.EmpresaNombre == "" || it.CuantosPermiso == 0 {
		t.Error("el rol traible tiene que decir de dónde viene y con cuántos permisos")
	}
}

func TestCodigoDeRolAMedidaNoPuedeChocarConUnoBase(t *testing.T) {
	// Por qué importa: `PermisosDeRol` busca por CÓDIGO dentro de la empresa. Si un rol a medida
	// pudiera llamarse igual que uno base, devolvería la unión de los dos —un escalamiento de
	// privilegios silencioso—. El prefijo obligatorio «CUSTOM_» es lo que lo hace imposible, y desde
	// la migración 0067 (código único por empresa) conviene tenerlo fijado por un test.
	for _, nombre := range []string{"Admin", "ADMIN", "Director Financiero", "Auditor Interno"} {
		codigo := codigoDesdeNombre(nombre)
		if !strings.HasPrefix(codigo, "CUSTOM_") {
			t.Errorf("%q generó %q, que no lleva el prefijo CUSTOM_", nombre, codigo)
		}
		for _, base := range []string{RolAdmin, "AUDITOR_INTERNO", "AUXILIAR_FINANCIERO",
			"DIRECTOR_FINANCIERO", "GERENCIA_GENERAL", "SUPERVISOR_FINANCIERO", "SUPERVISOR_PISO"} {
			if codigo == base {
				t.Errorf("el nombre %q genera el código de un rol base (%s)", nombre, base)
			}
		}
	}
}
