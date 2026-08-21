package bancos

// Fase A — motor que aprende: endpoints de sugerencia, edición de reglas,
// clasificación masiva y resumen del KPI.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// SugerenciaRegla GET /v1/bancos/reglas/sugerencia?movimiento_id= — propuesta de aprendizaje
// tras clasificar a mano (banner "¿Crear regla?" con conteo de similares).
func (h *Handler) SugerenciaRegla(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	movID := c.Query("movimiento_id")
	if movID == "" {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "movimiento_id es requerido")
		return
	}
	sug, err := h.svc.SugerenciaRegla(c.Request.Context(), claims.EmpresaID, movID)
	if err != nil {
		h.responderError(c, err, "sugerencia-regla")
		return
	}
	c.JSON(http.StatusOK, sug)
}

type actualizarReglaRequest struct {
	Prioridad       *int     `json:"prioridad"`
	Activo          *bool    `json:"activo"`
	AgregarPalabras []string `json:"agregar_palabras"`
	QuitarPalabras  []string `json:"quitar_palabras"`
}

// ActualizarRegla PATCH /v1/bancos/reglas/:id — prioridad, pausar/activar, palabras clave.
func (h *Handler) ActualizarRegla(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req actualizarReglaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if req.Prioridad == nil && req.Activo == nil && len(req.AgregarPalabras) == 0 && len(req.QuitarPalabras) == 0 {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "nada que actualizar")
		return
	}
	err := h.svc.ActualizarRegla(c.Request.Context(), claims.EmpresaID, c.Param("id"), CambiosRegla{
		Prioridad: req.Prioridad, Activo: req.Activo,
		AgregarPalabras: req.AgregarPalabras, QuitarPalabras: req.QuitarPalabras,
	}, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "actualizar-regla")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// EliminarRegla DELETE /v1/bancos/reglas/:id
func (h *Handler) EliminarRegla(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.EliminarRegla(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID()); err != nil {
		h.responderError(c, err, "eliminar-regla")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type clasificarMasivoRequest struct {
	MovimientoIDs   []string `json:"movimiento_ids" validate:"required,min=1,dive,uuid"`
	ConceptoID      string   `json:"concepto_id" validate:"required,uuid"`
	ClasificacionID string   `json:"clasificacion_id" validate:"required,uuid"`
}

// ClasificarMasivo POST /v1/bancos/movimientos/clasificar-masivo — un bloque, un golpe.
func (h *Handler) ClasificarMasivo(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req clasificarMasivoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	n, err := h.svc.ClasificarMasivo(c.Request.Context(), claims.EmpresaID,
		req.MovimientoIDs, req.ConceptoID, req.ClasificacionID, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "clasificar-masivo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"clasificados": n})
}

// ResumenClasificacion GET /v1/bancos/clasificacion/resumen?periodo=YYYY-MM — KPI % auto.
func (h *Handler) ResumenClasificacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	res, err := h.svc.ResumenClasificacion(c.Request.Context(), claims.EmpresaID, c.Query("periodo"))
	if err != nil {
		h.responderError(c, err, "resumen-clasificacion")
		return
	}
	c.JSON(http.StatusOK, res)
}
