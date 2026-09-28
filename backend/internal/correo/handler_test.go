package correo

// El BORDE del correo saliente: lo que sale por HTTP y con qué estado.
//
// Dos cosas se prueban acá y no en el service: que el CUERPO REAL de la respuesta no contenga la
// contraseña (el service puede ser perfecto y el handler filtrarla igual), y que ningún centinela
// salga como 500 «error interno» — defecto con el que este proyecto ya se quemó varias veces:
// el usuario no puede distinguir «te falta un dato» de «el servidor se cayó», así que reintenta lo
// mismo.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/shared"
)

// pedir corre un handler con un usuario autenticado en la empresa dada.
func pedir(t *testing.T, h *Handler, metodo, ruta, cuerpo string, correr func(*gin.Context)) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var body *strings.Reader
	if cuerpo == "" {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(cuerpo)
	}
	c.Request = httptest.NewRequest(metodo, ruta, body)
	c.Request.Header.Set("Content-Type", "application/json")
	auth.SetClaims(c, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "u1"},
		Email:            "ana@valledepazcr.com", EmpresaID: empresaVDP, Rol: "DIRECTOR_FINANCIERO",
	})
	correr(c)
	return w.Code, w.Body.String()
}

// TestElCuerpoDelGETNoTraeLaContrasena: la garantía, medida sobre los bytes que viajan.
func TestElCuerpoDelGETNoTraeLaContrasena(t *testing.T) {
	const password = "contrasena-del-buzon-corporativo"
	repo := &repoFalso{}
	svc := servicio(t, repo, cifradorDePrueba(t), globalDePrueba())
	if err := svc.Guardar(t.Context(), empresaVDP, entradaValida(password), "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	h := NewHandler(svc, zap.NewNop())

	codigo, cuerpo := pedir(t, h, http.MethodGet, "/v1/correo-saliente", "", h.Obtener)
	if codigo != http.StatusOK {
		t.Fatalf("estado = %d, cuerpo = %s", codigo, cuerpo)
	}
	if strings.Contains(cuerpo, password) {
		t.Fatalf("la contraseña viajó al navegador: %s", cuerpo)
	}
	if strings.Contains(cuerpo, repo.passwordGuardada) {
		t.Fatalf("el valor cifrado tampoco puede viajar: %s", cuerpo)
	}
	var mapa map[string]any
	if err := json.Unmarshal([]byte(cuerpo), &mapa); err != nil {
		t.Fatalf("la respuesta no es JSON válido: %v", err)
	}
	if _, existe := mapa["password"]; existe {
		t.Fatal("la respuesta NO puede tener una clave `password`")
	}
	if mapa["tiene_password"] != true {
		t.Fatalf("tiene_password tenía que ser true: %s", cuerpo)
	}
	// Y los campos que el frontend va a tipar tienen que estar, con estos nombres exactos.
	for _, campo := range []string{"configurado", "host", "puerto", "seguridad", "usuario",
		"tiene_password", "remitente", "remitente_nombre", "activo", "probado_en", "probado_error",
		"actualizado_por", "actualizado_en", "origen_vigente", "remitente_vigente", "cifrado_disponible"} {
		if _, existe := mapa[campo]; !existe {
			t.Fatalf("falta el campo %q en la respuesta: %s", campo, cuerpo)
		}
	}
}

// TestElPUTAceptaLaContrasenaPeroNoLaDevuelve: la asimetría del tipo `shared.Secreto` —entra por el
// binding, no sale por el marshal— medida de punta a punta.
func TestElPUTAceptaLaContrasenaPeroNoLaDevuelve(t *testing.T) {
	const password = "clave-que-entra-pero-no-sale"
	repo := &repoFalso{}
	h := NewHandler(servicio(t, repo, cifradorDePrueba(t), globalDePrueba()), zap.NewNop())

	cuerpoPUT := `{"host":"smtp.office365.com","puerto":587,"seguridad":"STARTTLS",
		"usuario":"cxp@valledepazcr.com","password":"` + password + `",
		"remitente":"cxp@valledepazcr.com","remitente_nombre":"Valle de Paz","activo":true}`
	codigo, cuerpo := pedir(t, h, http.MethodPut, "/v1/correo-saliente", cuerpoPUT, h.Guardar)
	if codigo != http.StatusOK {
		t.Fatalf("estado = %d, cuerpo = %s", codigo, cuerpo)
	}
	if strings.Contains(cuerpo, password) {
		t.Fatalf("la respuesta del PUT no puede repetir la contraseña: %s", cuerpo)
	}
	if !strings.Contains(cuerpo, `"tiene_password":true`) {
		t.Fatalf("el PUT tiene que confirmar que quedó guardada: %s", cuerpo)
	}
	// Llegó de verdad al service y se guardó cifrada.
	if !strings.HasPrefix(repo.passwordGuardada, "v1.") {
		t.Fatalf("lo guardado tenía que venir cifrado: %q", repo.passwordGuardada)
	}
	if strings.Contains(repo.passwordGuardada, password) {
		t.Fatal("se guardó en claro")
	}
}

// TestNingunCentinelaSaleComo500 es la red: agregar un centinela sin agregarlo al switch de
// `responder` deja este test rojo.
func TestNingunCentinelaSaleComo500(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		quiere int
	}{
		{"sin host", ErrHostRequerido, http.StatusBadRequest},
		{"puerto", ErrPuertoInvalido, http.StatusBadRequest},
		{"seguridad", ErrSeguridadInvalida, http.StatusBadRequest},
		{"remitente", ErrRemitenteInvalido, http.StatusBadRequest},
		{"falta la contraseña", ErrPasswordRequerida, http.StatusUnprocessableEntity},
		{"cambió el servidor", ErrPasswordRequeridaPorCambio, http.StatusUnprocessableEntity},
		{"host interno", ErrHostNoPermitido, http.StatusUnprocessableEntity},
		{"usuario sin correo", ErrSinCorreoDePrueba, http.StatusUnprocessableEntity},
		{"falta CIFRADO_SECRET", shared.ErrClaveAusente, http.StatusUnprocessableEntity},
		{"CIFRADO_SECRET corta", shared.ErrClaveCorta, http.StatusUnprocessableEntity},
		{"CIFRADO_SECRET débil", shared.ErrClaveDebil, http.StatusUnprocessableEntity},
		{"no descifra", shared.ErrNoDescifra, http.StatusUnprocessableEntity},
		{"formato del secreto", shared.ErrFormato, http.StatusUnprocessableEntity},
		{"sin correo configurado", shared.ErrSMTPNoConfigurado, http.StatusUnprocessableEntity},
		{"el servidor rechazó", &shared.ErrorSMTP{Categoria: shared.CategoriaAutenticacion, Codigo: 535, Servidor: "smtp.x:587"}, http.StatusUnprocessableEntity},
		{"prueba en ráfaga", ErrPruebaMuySeguida, http.StatusTooManyRequests},
		{"sin configuración", ErrConfigNoEncontrada, http.StatusNotFound},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPut, "/v1/correo-saliente", nil)
			(&Handler{}).responder(c, caso.err)
			if w.Code != caso.quiere {
				t.Fatalf("estado = %d, se esperaba %d (cuerpo: %s)", w.Code, caso.quiere, w.Body.String())
			}
			if w.Code == http.StatusInternalServerError {
				t.Fatal("un centinela de dominio NUNCA puede salir como «error interno»")
			}
			if strings.TrimSpace(w.Body.String()) == "" {
				t.Fatal("el error tiene que traer un mensaje que el usuario pueda leer")
			}
		})
	}
}

// TestElMensajeDeCifradoNombraLaVariable: el que lo lee tiene que saber qué poner.
func TestElMensajeDeCifradoNombraLaVariable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/v1/correo-saliente", nil)
	(&Handler{}).responder(c, shared.ErrClaveAusente)
	if !strings.Contains(w.Body.String(), "CIFRADO_SECRET") {
		t.Fatalf("el mensaje tiene que nombrar la variable: %s", w.Body.String())
	}
}

// TestSinClaimsNoSeResponde: la empresa sale del token, nunca del cuerpo.
func TestSinClaimsNoSeResponde(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(NewService(&repoFalso{}, nil, zap.NewNop(), nil, shared.SMTP{}, false), zap.NewNop())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/correo-saliente", nil)
	h.Obtener(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("estado = %d, se esperaba 401", w.Code)
	}
}

// La guarda que la lista de arriba NO es.
//
// El test anterior recorre casos ESCRITOS A MANO: hoy están todos, pero el día que alguien agregue
// un centinela y no toque la lista, sigue verde y el 500 vuelve — que es exactamente el defecto con
// el que este proyecto ya se quemó. Esta guarda enumera los centinelas del CÓDIGO FUENTE y exige
// que cada uno aparezca en el switch de `responder`. Agregar uno sin atenderlo deja esto rojo.
func TestTodoCentinelaDelPaqueteEstaEnElSwitch(t *testing.T) {
	fuente, err := os.ReadFile("correo.go")
	if err != nil {
		t.Fatalf("leer correo.go: %v", err)
	}
	handler, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("leer handler.go: %v", err)
	}

	// `ErrAlgo = errors.New(` en la declaración de centinelas del paquete.
	re := regexp.MustCompile(`(?m)^\s*(Err[A-Za-z0-9_]+)\s*=\s*errors\.New\(`)
	encontrados := re.FindAllStringSubmatch(string(fuente), -1)
	if len(encontrados) == 0 {
		t.Fatal("no se reconoció ningún centinela en correo.go: cambió la forma de declararlos y esta guarda quedó ciega")
	}

	switchDeResponder := string(handler)
	if i := strings.Index(switchDeResponder, "func (h *Handler) responder("); i >= 0 {
		switchDeResponder = switchDeResponder[i:]
	} else {
		t.Fatal("no se encontró `responder` en handler.go")
	}

	for _, m := range encontrados {
		nombre := m[1]
		if !strings.Contains(switchDeResponder, nombre) {
			t.Errorf("el centinela %s no está en el switch de responder: va a salir como 500 «error interno» "+
				"y el usuario no va a poder leer el motivo", nombre)
		}
	}
}
