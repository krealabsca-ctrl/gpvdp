package nomina

// Los endpoints del autoservicio de vacaciones.
//
// NINGUNO recibe a quién corresponde la acción. «Mis vacaciones» no lleva empleado y «aprobar» no
// lleva departamento: los dos salen del usuario autenticado, en el service. Un endpoint que
// aceptara el empleado por parámetro dejaría que cualquiera con el permiso pidiera o aprobara días
// a nombre de otro, y el permiso está pensado justamente para darse a mucha gente.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// MisVacaciones GET /v1/rrhh/vacaciones/mias — saldo propio y solicitudes propias.
func (h *Handler) MisVacaciones(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	res, err := h.svc.MisVacaciones(c.Request.Context(), empresaID, usuarioID)
	if err != nil {
		h.responderError(c, err, "mis-vacaciones")
		return
	}
	c.JSON(http.StatusOK, res)
}

// PedirVacaciones POST /v1/rrhh/vacaciones/mias — crea la solicitud propia.
func (h *Handler) PedirVacaciones(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		FechaInicio   string `json:"fecha_inicio" validate:"required"`
		Dias          string `json:"dias" validate:"required"`
		Observaciones string `json:"observaciones"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí la fecha de inicio y cuántos días")
		return
	}
	in := VacacionInput{
		FechaInicio:   req.FechaInicio,
		Dias:          req.Dias,
		Observaciones: req.Observaciones,
	}
	v, err := h.svc.PedirVacaciones(c.Request.Context(), empresaID, usuarioID, in)
	if err != nil {
		h.responderError(c, err, "pedir-vacaciones")
		return
	}
	c.JSON(http.StatusCreated, v)
}

// CancelarMiSolicitud POST /v1/rrhh/vacaciones/mias/:id/cancelar — retira una solicitud propia
// que todavía no se aprobó.
func (h *Handler) CancelarMiSolicitud(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.CancelarMiSolicitud(c.Request.Context(), empresaID, usuarioID, c.Param("id")); err != nil {
		h.responderError(c, err, "cancelar-mi-solicitud")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// VacacionesDelEquipo GET /v1/rrhh/vacaciones/equipo — pendientes, calendario y saldos del
// departamento a cargo.
func (h *Handler) VacacionesDelEquipo(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	res, err := h.svc.VacacionesDelEquipo(c.Request.Context(), empresaID, usuarioID)
	if err != nil {
		h.responderError(c, err, "vacaciones-equipo")
		return
	}
	c.JSON(http.StatusOK, res)
}

// ResolverSolicitud POST /v1/rrhh/vacaciones/equipo/:id/resolver — la jefatura aprueba o rechaza.
func (h *Handler) ResolverSolicitud(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		Aprobar bool   `json:"aprobar"`
		Motivo  string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí si se aprueba o se rechaza")
		return
	}
	err := h.svc.ResolverSolicitud(c.Request.Context(), empresaID, usuarioID, c.Param("id"), req.Aprobar, req.Motivo)
	if err != nil {
		h.responderError(c, err, "resolver-solicitud")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ConfirmarVacacion POST /v1/rrhh/vacaciones/:id/confirmar — RRHH da por disfrutados los días.
func (h *Handler) ConfirmarVacacion(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.ConfirmarVacacion(c.Request.Context(), empresaID, usuarioID, c.Param("id")); err != nil {
		h.responderError(c, err, "confirmar-vacacion")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Administración: jefaturas y vínculo con el usuario ----

// Jefaturas GET /v1/rrhh/jefaturas — quién tiene a cargo cada departamento.
func (h *Handler) Jefaturas(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	js, err := h.svc.Jefaturas(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "jefaturas")
		return
	}
	c.JSON(http.StatusOK, js)
}

// AsignarJefatura POST /v1/rrhh/jefaturas — nombra una jefatura.
func (h *Handler) AsignarJefatura(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		DepartamentoID string `json:"departamento_id" validate:"required"`
		UsuarioID      string `json:"usuario_id" validate:"required"`
		Rol            string `json:"rol"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.DepartamentoID == "" || req.UsuarioID == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí el departamento y el usuario")
		return
	}
	err := h.svc.AsignarJefatura(c.Request.Context(), empresaID, req.DepartamentoID, req.UsuarioID, req.Rol, usuarioID)
	if err != nil {
		h.responderError(c, err, "asignar-jefatura")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// QuitarJefatura DELETE /v1/rrhh/jefaturas — saca a alguien de un departamento.
func (h *Handler) QuitarJefatura(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	dep, usr := c.Query("departamento_id"), c.Query("usuario_id")
	if dep == "" || usr == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí el departamento y el usuario")
		return
	}
	if err := h.svc.QuitarJefatura(c.Request.Context(), empresaID, dep, usr); err != nil {
		h.responderError(c, err, "quitar-jefatura")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// LigarUsuario PUT /v1/rrhh/empleados/:id/usuario — conecta la ficha con el usuario del sistema.
// Mandar el usuario vacío la desliga.
func (h *Handler) LigarUsuario(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		UsuarioID string `json:"usuario_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "mandá el usuario (vacío para desligar)")
		return
	}
	if err := h.svc.LigarUsuario(c.Request.Context(), empresaID, c.Param("id"), req.UsuarioID); err != nil {
		h.responderError(c, err, "ligar-usuario")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
