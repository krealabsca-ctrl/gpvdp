package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/httpx"
	"github.com/gpvdp/erp/internal/shared"
)

// Handler expone los endpoints de autenticación y selección de empresa.
type Handler struct {
	svc   *Service
	audit *shared.Audit
	log   *zap.Logger
}

// NewHandler construye el handler de auth.
func NewHandler(svc *Service, audit *shared.Audit, log *zap.Logger) *Handler {
	return &Handler{svc: svc, audit: audit, log: log}
}

// ---- DTOs (contrato HTTP) ----

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
	EmpresaID    string `json:"empresa_id" validate:"omitempty,uuid"`
}

type selectEmpresaRequest struct {
	EmpresaID string `json:"empresa_id" validate:"required,uuid"`
}

type usuarioDTO struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Email  string `json:"email"`
}

type empresaDTO struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Rol    string `json:"rol"`
}

type loginResponse struct {
	AccessToken         string       `json:"access_token"`
	RefreshToken        string       `json:"refresh_token"`
	Usuario             usuarioDTO   `json:"user"`
	Empresas            []empresaDTO `json:"empresas"`
	DebeCambiarPassword bool         `json:"debe_cambiar_password"`
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type selectEmpresaResponse struct {
	AccessToken string `json:"access_token"`
}

type empresaActivaDTO struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
}

type meResponse struct {
	Usuario             usuarioDTO        `json:"user"`
	Empresas            []empresaDTO      `json:"empresas"`
	EmpresaActiva       *empresaActivaDTO `json:"empresa_activa"`
	Rol                 *string           `json:"rol"`
	DebeCambiarPassword bool              `json:"debe_cambiar_password"`
}

// ---- Handlers ----

// Login POST /v1/auth/login
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}

	res, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, ErrCuentaBloqueada):
			// Mismo 429/mensaje que el límite por IP: no se confirma que la cuenta exista.
			httpx.Abort(c, http.StatusTooManyRequests, httpx.CodeDemasiadosIntentos, "demasiados intentos, probá de nuevo en unos minutos")
		case errors.Is(err, ErrCredenciales):
			httpx.Abort(c, http.StatusUnauthorized, httpx.CodeCredenciales, "credenciales inválidas")
		case errors.Is(err, ErrUsuarioInactivo):
			httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, "usuario inactivo")
		default:
			h.log.Error("login", zap.Error(err))
			httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
		}
		return
	}

	uid := res.Usuario.ID
	h.audit.Registrar(c.Request.Context(), shared.Evento{
		Entidad: "usuario", EntidadID: &uid, Accion: "LOGIN", UsuarioID: &uid,
	})

	c.JSON(http.StatusOK, loginResponse{
		AccessToken:         res.AccessToken,
		RefreshToken:        res.RefreshToken,
		Usuario:             toUsuarioDTO(res.Usuario),
		Empresas:            toEmpresaDTOs(res.Empresas),
		DebeCambiarPassword: res.Usuario.DebeCambiarPassword,
	})
}

// Refresh POST /v1/auth/refresh
func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}

	access, refresh, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken, req.EmpresaID)
	if err != nil {
		switch {
		case errors.Is(err, ErrRefreshInvalido):
			httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "sesión inválida, iniciá sesión de nuevo")
		case errors.Is(err, ErrSinAcceso):
			httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, "sin acceso a la empresa")
		case errors.Is(err, ErrUsuarioInactivo):
			httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, "usuario inactivo")
		default:
			h.log.Error("refresh", zap.Error(err))
			httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
		}
		return
	}

	c.JSON(http.StatusOK, refreshResponse{AccessToken: access, RefreshToken: refresh})
}

// SelectEmpresa POST /v1/auth/select-empresa (requiere access token)
func (h *Handler) SelectEmpresa(c *gin.Context) {
	claims, ok := ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}

	var req selectEmpresaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}

	access, m, err := h.svc.SelectEmpresa(c.Request.Context(), claims.UsuarioID(), req.EmpresaID)
	if err != nil {
		switch {
		case errors.Is(err, ErrSinAcceso):
			httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, "sin acceso a la empresa")
		case errors.Is(err, ErrUsuarioInactivo):
			httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, "usuario inactivo")
		default:
			h.log.Error("select-empresa", zap.Error(err))
			httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
		}
		return
	}

	uid := claims.UsuarioID()
	empID := m.EmpresaID
	h.audit.Registrar(c.Request.Context(), shared.Evento{
		EmpresaID: &empID, Entidad: "sesion", Accion: "SELECT_EMPRESA", UsuarioID: &uid,
	})

	c.JSON(http.StatusOK, selectEmpresaResponse{AccessToken: access})
}

// Me GET /v1/me (requiere access token)
func (h *Handler) Me(c *gin.Context) {
	claims, ok := ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	res, err := h.svc.Me(c.Request.Context(), claims)
	if err != nil {
		h.log.Error("me", zap.Error(err))
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
		return
	}

	resp := meResponse{
		Usuario:             toUsuarioDTO(res.Usuario),
		Empresas:            toEmpresaDTOs(res.Empresas),
		DebeCambiarPassword: res.DebeCambiarPassword,
	}
	if res.EmpresaActivaID != "" {
		for _, m := range res.Empresas {
			if m.EmpresaID == res.EmpresaActivaID {
				resp.EmpresaActiva = &empresaActivaDTO{ID: m.EmpresaID, Nombre: m.EmpresaNombre}
				break
			}
		}
		rol := res.Rol
		resp.Rol = &rol
	}
	c.JSON(http.StatusOK, resp)
}

// Empresas GET /v1/empresas (requiere access token) — para el selector.
func (h *Handler) Empresas(c *gin.Context) {
	claims, ok := ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	res, err := h.svc.Me(c.Request.Context(), claims)
	if err != nil {
		h.log.Error("empresas", zap.Error(err))
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
		return
	}
	c.JSON(http.StatusOK, toEmpresaDTOs(res.Empresas))
}

// EmpresaActual GET /v1/empresas/actual (requiere empresa seleccionada) — demuestra el scoping por tenant.
func (h *Handler) EmpresaActual(c *gin.Context) {
	claims, ok := ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	c.JSON(http.StatusOK, gin.H{"empresa_id": claims.EmpresaID, "rol": claims.Rol})
}

type cambiarPasswordRequest struct {
	Actual string `json:"actual" validate:"required"`
	Nueva  string `json:"nueva" validate:"required,min=8"`
}

// CambiarPassword POST /v1/auth/cambiar-password (requiere access token)
func (h *Handler) CambiarPassword(c *gin.Context) {
	claims, ok := ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req cambiarPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	if err := h.svc.CambiarPassword(c.Request.Context(), claims.UsuarioID(), req.Actual, req.Nueva); err != nil {
		switch {
		case errors.Is(err, ErrCredenciales):
			httpx.Abort(c, http.StatusUnauthorized, httpx.CodeCredenciales, "la contraseña actual no es correcta")
		case errors.Is(err, ErrPasswordDebil):
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		default:
			h.log.Error("cambiar-password", zap.Error(err))
			httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
		}
		return
	}
	uid := claims.UsuarioID()
	h.audit.Registrar(c.Request.Context(), shared.Evento{Entidad: "usuario", EntidadID: &uid, Accion: "CAMBIAR_PASSWORD", UsuarioID: &uid})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- mapeos ----

func toUsuarioDTO(u Usuario) usuarioDTO {
	return usuarioDTO{ID: u.ID, Nombre: u.Nombre, Email: u.Email}
}

func toEmpresaDTOs(ms []Membership) []empresaDTO {
	out := make([]empresaDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, empresaDTO{ID: m.EmpresaID, Nombre: m.EmpresaNombre, Rol: m.RolCodigo})
	}
	return out
}
