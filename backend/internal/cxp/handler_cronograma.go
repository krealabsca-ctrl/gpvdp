package cxp

// Endpoints del cronograma de pago (mig 0122). El porqué está en cronograma.go.

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// CronogramaDeLote GET /v1/cxp/lotes/:id/cronograma (cxp.ver)
func (h *Handler) CronogramaDeLote(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	cr, err := h.svc.Cronograma(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "cronograma")
		return
	}
	c.JSON(http.StatusOK, cr)
}

// CronogramaXLSX GET /v1/cxp/lotes/:id/cronograma.xlsx (cxp.ver)
//
// El archivo que se manda por correo. La pantalla sirve para decidir; esto es lo que sale del
// sistema cuando ya está decidido.
func (h *Handler) CronogramaXLSX(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	cr, err := h.svc.Cronograma(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "cronograma-xlsx")
		return
	}
	if len(cr.Lineas) == 0 {
		// Un corte sin nada programado no baja un archivo vacío: se dice. Un .xlsx con solo el
		// encabezado se manda por correo sin que nadie note que no lleva ninguna línea.
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"este corte no tiene nada programado")
		return
	}
	empresa, usuario := h.svc.EncabezadoDelAviso(c.Request.Context(), empresaID, usuarioID)
	// La hora, en hora de Costa Rica: el contenedor corre en UTC y un cuadro emitido a las 14:23
	// diría «20:23» sin que quien lo recibe tenga cómo saberlo.
	datos, err := ConstruirCronogramaXLSX(cr, empresa, usuario,
		time.Now().UTC().Add(offsetCR).Format("02/01/2006 15:04"))
	if err != nil {
		h.responderError(c, err, "cronograma-xlsx")
		return
	}
	nombre := NombreCronogramaXLSX(cr.LoteNumero)
	const tipo = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	c.Header("Content-Type", tipo)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		nombre, url.PathEscape(nombre)))
	c.Data(http.StatusOK, tipo, datos)
}

// GuardarTCCronograma PUT /v1/cxp/lotes/:id/cronograma/tipo-cambio (cxp.tesoreria)
//
// Lo escribe tesorería, por decisión del Director Financiero: el del cuadro es un número elegido
// para proyectar, no la cotización del día.
func (h *Handler) GuardarTCCronograma(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		TipoCambio string `json:"tipo_cambio"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	err := h.svc.GuardarTCDelCronograma(c.Request.Context(), empresaID, c.Param("id"),
		req.TipoCambio, usuarioID)
	if err != nil {
		h.responderError(c, err, "tc-cronograma")
		return
	}
	c.Status(http.StatusNoContent)
}

// AsignarCuentaDePago PUT /v1/cxp/lotes/:id/cronograma/:proveedorId/cuenta (cxp.tesoreria)
func (h *Handler) AsignarCuentaDePago(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		// Vacío quita la asignación: hay que poder deshacer una cuenta puesta por error.
		CuentaID string `json:"cuenta_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	err := h.svc.AsignarCuentaDePago(c.Request.Context(), empresaID, c.Param("id"),
		c.Param("proveedorId"), req.CuentaID, usuarioID)
	if err != nil {
		h.responderError(c, err, "cuenta-de-pago")
		return
	}
	c.Status(http.StatusNoContent)
}

// MarcarPorFuera PUT /v1/cxp/lotes/:id/cronograma/:proveedorId/por-fuera (cxp.tesoreria)
func (h *Handler) MarcarPorFuera(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req struct {
		PorFuera bool `json:"por_fuera"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	err := h.svc.MarcarPorFuera(c.Request.Context(), empresaID, c.Param("id"),
		c.Param("proveedorId"), req.PorFuera, usuarioID)
	if err != nil {
		h.responderError(c, err, "por-fuera")
		return
	}
	c.Status(http.StatusNoContent)
}
