package cxc

// HTTP de los arreglos de pago y de la lista de contacto preventivo.

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// Arreglos GET /v1/cxc/arreglos — la lista y, con ella, los plazos que este usuario puede
// pactar. Van juntos a propósito: si la pantalla tuviera que pedir la configuración por
// separado, podría ofrecer un plazo que el servidor va a rechazar.
func (h *Handler) Arreglos(c *gin.Context) {
	empresaID, rol, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	lista, err := h.svc.ListarArreglos(c.Request.Context(), empresaID, rol, usuarioID, FiltrosArreglos{
		Contrato:        c.Query("contrato"),
		Estado:          c.Query("estado"),
		SoloExcepciones: c.Query("excepciones") == "true",
		Page:            page,
		PageSize:        pageSize,
	})
	if err != nil {
		h.error(c, err, "arreglos")
		return
	}
	lista.Plazos = h.svc.PlazosDeArreglo(c.Request.Context(), empresaID, rol)
	c.JSON(http.StatusOK, lista)
}

// Arreglo GET /v1/cxc/arreglos/:id
func (h *Handler) Arreglo(c *gin.Context) {
	empresaID, _, _, ok := h.claims(c)
	if !ok {
		return
	}
	a, err := h.svc.Arreglo(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.error(c, err, "arreglo")
		return
	}
	c.JSON(http.StatusOK, a)
}

type arregloRequest struct {
	Contrato string `json:"contrato" binding:"required"`
	// Monto vacío significa «todo lo vencido», que es el caso normal.
	Monto          string `json:"monto"`
	Prima          string `json:"prima"`
	PrimaFecha     string `json:"prima_fecha"`
	Plazo          int    `json:"plazo_cuotas" binding:"required"`
	PrimeraCuota   string `json:"primera_cuota"`
	Observaciones  string `json:"observaciones"`
	MotivoAutoriza string `json:"motivo_autorizacion"`
}

// PactarArreglo POST /v1/cxc/arreglos
func (h *Handler) PactarArreglo(c *gin.Context) {
	empresaID, rol, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req arregloRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"hace falta el contrato y el plazo en cuotas")
		return
	}
	monto, err := montoArregloDesdeTexto(req.Monto)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el monto del arreglo no es un número")
		return
	}
	prima, err := montoArregloDesdeTexto(req.Prima)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "la prima no es un número")
		return
	}
	a, err := h.svc.PactarArreglo(c.Request.Context(), empresaID, rol, ArregloInput{
		Contrato:       req.Contrato,
		Monto:          monto,
		Prima:          prima,
		PrimaFecha:     req.PrimaFecha,
		Plazo:          req.Plazo,
		PrimeraCuota:   req.PrimeraCuota,
		Observaciones:  req.Observaciones,
		MotivoAutoriza: req.MotivoAutoriza,
	}, usuarioID)
	if err != nil {
		h.error(c, err, "pactar-arreglo")
		return
	}
	c.JSON(http.StatusCreated, a)
}

// QuebrarArreglo POST /v1/cxc/arreglos/:id/quebrar — declara el incumplimiento; el contrato
// pasa a cartera morosa.
func (h *Handler) QuebrarArreglo(c *gin.Context) {
	h.cerrarArregloHTTP(c, true)
}

// AnularArreglo POST /v1/cxc/arreglos/:id/anular — el arreglo que no debió existir. NO marca
// incumplimiento ni manda a cartera morosa.
func (h *Handler) AnularArreglo(c *gin.Context) {
	h.cerrarArregloHTTP(c, false)
}

func (h *Handler) cerrarArregloHTTP(c *gin.Context, quebrar bool) {
	empresaID, _, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	var req motivoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "hace falta el motivo")
		return
	}
	var a Arreglo
	var err error
	if quebrar {
		a, err = h.svc.QuebrarArreglo(c.Request.Context(), empresaID, c.Param("id"), req.Motivo, usuarioID)
	} else {
		a, err = h.svc.AnularArreglo(c.Request.Context(), empresaID, c.Param("id"), req.Motivo, usuarioID)
	}
	if err != nil {
		h.error(c, err, "cerrar-arreglo")
		return
	}
	c.JSON(http.StatusOK, a)
}

// Preventivo GET /v1/cxc/preventivo — la lista de avisos ANTES del vencimiento. Es el universo
// que la cola excluye a propósito, y tiene su propio permiso porque es otra actividad.
func (h *Handler) Preventivo(c *gin.Context) {
	empresaID, rol, usuarioID, ok := h.claims(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	lista, err := h.svc.ListaPreventiva(c.Request.Context(), empresaID, rol, usuarioID, FiltrosPreventivo{
		SedeID:   c.Query("sede_id"),
		Motivo:   c.Query("motivo"),
		Q:        c.Query("q"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		h.error(c, err, "preventivo")
		return
	}
	c.JSON(http.StatusOK, lista)
}
