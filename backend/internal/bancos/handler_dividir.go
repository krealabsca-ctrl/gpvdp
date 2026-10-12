package bancos

// Endpoints para dividir un movimiento entre partidas.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/httpx"
)

// DividirMovimiento POST /v1/bancos/movimientos/:id/dividir (bancos.dividir_movimiento)
//
// Devuelve el REPARTO COMPLETO, no un «ok»: quien divide necesita ver en el acto que las partes
// siguen sumando lo que trajo el banco. Es la invariante del módulo, y mostrarla es más honesto
// que prometerla.
func (h *Handler) DividirMovimiento(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req struct {
		Monto           string `json:"monto"`
		ConceptoID      string `json:"concepto_id"`
		ClasificacionID string `json:"clasificacion_id"`
		DepartamentoID  string `json:"departamento_id"`
		SedeID          string `json:"sede_id"`
		Motivo          string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"decí cuánto separar y a qué clasificación va")
		return
	}
	monto, err := decimal.NewFromString(req.Monto)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el monto a separar no se entiende")
		return
	}
	in := DivisionInput{
		Monto:           monto,
		ConceptoID:      req.ConceptoID,
		ClasificacionID: req.ClasificacionID,
		DepartamentoID:  req.DepartamentoID,
		SedeID:          req.SedeID,
		Motivo:          req.Motivo,
	}
	d, err := h.svc.DividirMovimiento(c.Request.Context(), claims.EmpresaID, c.Param("id"), in, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "dividir-movimiento")
		return
	}
	c.JSON(http.StatusOK, respuestaDivision(d))
}

// DeshacerDivision POST /v1/bancos/movimientos/:id/deshacer-division (bancos.dividir_movimiento)
//
// El `:id` es el de la PARTE, no el del principal: se deshace una cara a la vez, porque un
// movimiento puede estar repartido en varias y devolverlas todas de golpe no siempre es lo que se
// quiere.
func (h *Handler) DeshacerDivision(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	err := h.svc.DeshacerDivision(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "deshacer-division")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PartesDeMovimiento GET /v1/bancos/movimientos/:id/partes (bancos.ver_clasificar)
//
// Sirve con el id del principal o con el de cualquier parte: desde cualquier cara se ve el reparto
// entero, porque quien abre una parte quiere saber de dónde salió.
func (h *Handler) PartesDeMovimiento(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	d, err := h.svc.PartesDeMovimiento(c.Request.Context(), claims.EmpresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "partes-movimiento")
		return
	}
	c.JSON(http.StatusOK, respuestaDivision(d))
}

// respuestaDivision agrega `cuadra`, que es la pregunta que la pantalla necesita contestar.
//
// Se calcula en el servidor y no en el cliente a propósito: la invariante es del dominio, y dos
// implementaciones de la misma comprobación son dos oportunidades de que una diga que cuadra
// cuando no.
func respuestaDivision(d Division) gin.H {
	return gin.H{
		"movimiento_id":          d.MovimientoID,
		"fecha":                  d.Fecha,
		"documento":              d.Documento,
		"descripcion":            d.Descripcion,
		"partes":                 d.Partes,
		"monto_antes_de_dividir": d.MontoAntesDeDividir,
		"suma_de_las_partes":     d.SumaDeLasPartes,
		"cuadra":                 d.Cuadra(),
	}
}
