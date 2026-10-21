package cxp

// GET /v1/cxp/documentos/:id/comprobante/whatsapp — el texto para mandarlo por WhatsApp.

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MensajeWhatsAppComprobante devuelve el número normalizado y el MISMO texto que iría por correo.
//
// No envía nada: el envío lo hace la persona desde WhatsApp Web, porque el canal no admite
// adjuntar el PDF por enlace. La pantalla abre el chat con este texto y descarga el comprobante
// para que lo arrastre.
func (h *Handler) MensajeWhatsAppComprobante(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	msj, err := h.svc.MensajeDeComprobante(c.Request.Context(), empresaID,
		c.Param("id"), c.Query("telefono"))
	if err != nil {
		h.responderError(c, err, "comprobante-whatsapp")
		return
	}
	c.JSON(http.StatusOK, msj)
}
