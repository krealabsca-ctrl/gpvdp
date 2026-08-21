package tenant

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
)

const secret = "secreto-de-prueba"

func init() { gin.SetMode(gin.TestMode) }

func nuevoEngine() *gin.Engine {
	r := gin.New()
	g := r.Group("")
	g.Use(RequireAuth(secret))
	g.GET("/protegido", func(c *gin.Context) { c.Status(http.StatusOK) })

	emp := r.Group("")
	emp.Use(RequireAuth(secret), RequireEmpresa())
	emp.GET("/con-empresa", func(c *gin.Context) { c.Status(http.StatusOK) })

	admin := r.Group("")
	admin.Use(RequireAuth(secret), RequireRol("ADMIN"))
	admin.GET("/solo-admin", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func token(t *testing.T, empresa, rol string) string {
	t.Helper()
	tok, err := auth.MintAccessToken(secret, time.Hour, "u1", "a@b.com", empresa, rol)
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}
	return tok
}

func do(r *gin.Engine, method, path, bearer string) int {
	req := httptest.NewRequest(method, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRequireAuth(t *testing.T) {
	r := nuevoEngine()

	if code := do(r, http.MethodGet, "/protegido", ""); code != http.StatusUnauthorized {
		t.Errorf("sin token: code = %d, quería 401", code)
	}
	if code := do(r, http.MethodGet, "/protegido", "token.basura"); code != http.StatusUnauthorized {
		t.Errorf("token inválido: code = %d, quería 401", code)
	}
	if code := do(r, http.MethodGet, "/protegido", token(t, "", "")); code != http.StatusOK {
		t.Errorf("token válido: code = %d, quería 200", code)
	}
}

func TestRequireEmpresa(t *testing.T) {
	r := nuevoEngine()

	if code := do(r, http.MethodGet, "/con-empresa", token(t, "", "")); code != http.StatusForbidden {
		t.Errorf("token sin empresa: code = %d, quería 403", code)
	}
	if code := do(r, http.MethodGet, "/con-empresa", token(t, "emp-1", "ADMIN")); code != http.StatusOK {
		t.Errorf("token con empresa: code = %d, quería 200", code)
	}
}

func TestRequireRol(t *testing.T) {
	r := nuevoEngine()

	if code := do(r, http.MethodGet, "/solo-admin", token(t, "emp-1", "AUXILIAR_FINANCIERO")); code != http.StatusForbidden {
		t.Errorf("rol no autorizado: code = %d, quería 403", code)
	}
	if code := do(r, http.MethodGet, "/solo-admin", token(t, "emp-1", "ADMIN")); code != http.StatusOK {
		t.Errorf("rol autorizado: code = %d, quería 200", code)
	}
}
