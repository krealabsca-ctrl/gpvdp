package cxp

// Endpoints del aviso de pago. El porqué está en aviso_pago.go.

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// AvisoDeLoteHandler GET /v1/cxp/lotes/:id/aviso — el cuadro de pago, partido por moneda.
//
// Devuelve JSON para que la pantalla lo muestre y deje corregir el texto ANTES de descargarlo:
// bajar el .xlsx, abrirlo, ver que el concepto está mal y volver al sistema es el viaje que este
// endpoint evita.
func (h *Handler) AvisoDeLoteHandler(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	av, err := h.svc.AvisoDeLote(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "aviso-lote")
		return
	}
	c.JSON(http.StatusOK, av)
}

// AvisoDeLoteXLSX GET /v1/cxp/lotes/:id/aviso.xlsx?moneda=CRC — el papel que se manda.
//
// La moneda es obligatoria en los hechos: cada archivo corresponde a UNA, igual que la macro. Si
// no se indica, se baja la primera (colones), que es la del 100 % de la operación.
func (h *Handler) AvisoDeLoteXLSX(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	av, err := h.svc.AvisoDeLote(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "aviso-lote-xlsx")
		return
	}
	if len(av.Monedas) == 0 {
		// Un corte sin nada que pagar no baja un archivo vacío: se dice. Un .xlsx con solo el
		// encabezado se manda por correo sin que nadie note que no lleva ninguna línea.
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"este corte no tiene nada que pagar")
		return
	}
	pedida := strings.ToUpper(strings.TrimSpace(c.Query("moneda")))
	elegida := av.Monedas[0]
	if pedida != "" {
		hallada := false
		for _, m := range av.Monedas {
			if m.Moneda == pedida {
				elegida, hallada = m, true
				break
			}
		}
		if !hallada {
			httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado,
				fmt.Sprintf("este corte no tiene pagos en %s", pedida))
			return
		}
	}

	empresa, usuario := h.svc.EncabezadoDelAviso(c.Request.Context(), empresaID, usuarioID)
	// La hora, en hora de Costa Rica. El contenedor corre en UTC: un aviso emitido a las 14:23
	// diría «20:23» y quien lo recibe no tendría cómo saberlo. Mismo `offsetCR` que ya usa el
	// resto del paquete.
	datos, err := ConstruirAvisoXLSX(av, elegida, empresa, usuario,
		time.Now().UTC().Add(offsetCR).Format("02/01/2006 15:04"))
	if err != nil {
		h.responderError(c, err, "aviso-lote-xlsx")
		return
	}
	nombre := NombreAvisoXLSX(av.LoteNumero, elegida.Moneda)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	// El nombre lleva tildes en el título pero no en el archivo; aun así se manda en las dos
	// formas, porque un navegador que no entienda `filename*` guarda el archivo con el nombre
	// crudo y un `Ñ` mal codificado ahí rompe la descarga entera.
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		nombre, url.PathEscape(nombre)))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", datos)
}

type textoAvisoRequest struct {
	// Punteros: lo que no venga NO se toca. Mandar solo el concepto no puede borrar la glosa.
	Concepto   *string `json:"concepto"`
	Documentos *string `json:"documentos"`
}

// GuardarTextoDeAvisoHandler PUT /v1/cxp/lotes/:id/aviso/:proveedorId
func (h *Handler) GuardarTextoDeAvisoHandler(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req textoAvisoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if req.Concepto == nil && req.Documentos == nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"no se indicó ningún texto que cambiar")
		return
	}
	err := h.svc.GuardarTextoDeAviso(c.Request.Context(), empresaID, c.Param("id"),
		c.Param("proveedorId"), TextoAviso{Concepto: req.Concepto, Documentos: req.Documentos}, usuarioID)
	if err != nil {
		h.responderError(c, err, "aviso-texto")
		return
	}
	c.Status(http.StatusNoContent)
}
