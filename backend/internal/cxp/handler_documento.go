package cxp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

var errTCInvalido = errors.New("cxp: tc inválido para moneda USD")

type crearDocumentoRequest struct {
	ProveedorID  string `json:"proveedor_id" validate:"required,uuid"`
	Clave        string `json:"clave"`
	Consecutivo  string `json:"consecutivo"`
	FechaEmision string `json:"fecha_emision" validate:"required,datetime=2006-01-02"`
	Moneda       string `json:"moneda" validate:"required,oneof=CRC USD"`
	Subtotal     string `json:"subtotal"`
	IVA          string `json:"iva"`
	Retencion    string `json:"retencion"`
	Total        string `json:"total" validate:"required"`
	TC           string `json:"tc"`
	Descripcion  string `json:"descripcion"`
	Vencimiento  string `json:"fecha_vencimiento" validate:"omitempty,datetime=2006-01-02"`
	Tipo         string `json:"tipo" validate:"omitempty,oneof=CXP ANTICIPO VIATICOS REINTEGRO INTERNO"`
}

func decOrZero(s string) (decimal.Decimal, error) {
	if s == "" {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(s)
}

func (req crearDocumentoRequest) aInput() (DocumentoInput, error) {
	sub, err := decOrZero(req.Subtotal)
	if err != nil {
		return DocumentoInput{}, err
	}
	iva, err := decOrZero(req.IVA)
	if err != nil {
		return DocumentoInput{}, err
	}
	ret, err := decOrZero(req.Retencion)
	if err != nil {
		return DocumentoInput{}, err
	}
	total, err := decimal.NewFromString(req.Total)
	if err != nil {
		return DocumentoInput{}, err
	}
	tc := decimal.Zero
	if req.Moneda == "USD" {
		tc, err = decimal.NewFromString(req.TC)
		if err != nil || !tc.IsPositive() {
			return DocumentoInput{}, errTCInvalido
		}
	}
	return DocumentoInput{
		ProveedorID:  req.ProveedorID,
		Clave:        req.Clave,
		Consecutivo:  req.Consecutivo,
		FechaEmision: req.FechaEmision,
		Moneda:       req.Moneda,
		Subtotal:     sub,
		IVA:          iva,
		Retencion:    ret,
		Total:        total,
		TC:           tc,
		Descripcion:  req.Descripcion,
		Vencimiento:  req.Vencimiento,
		Tipo:         req.Tipo,
	}, nil
}

// CrearDocumento POST /v1/cxp/documentos
func (h *Handler) CrearDocumento(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req crearDocumentoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	in, err := req.aInput()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "montos/tc inválidos (para USD, tc > 0 es obligatorio)")
		return
	}
	d, err := h.svc.CrearDocumento(c.Request.Context(), empresaID, in, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-documento")
		return
	}
	c.JSON(http.StatusCreated, d)
}

// ListarDocumentos GET /v1/cxp/documentos?estado=&proveedor_id=&page=&page_size=
func (h *Handler) ListarDocumentos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var estados []string
	if v := c.Query("estados"); v != "" {
		estados = strings.Split(v, ",")
	}
	lista, err := h.svc.ListarDocumentos(c.Request.Context(), claims.EmpresaID, claims.Rol, claims.UsuarioID(), FiltrosDocumentos{
		Estado:             c.Query("estado"),
		Estados:            estados,
		Q:                  c.Query("q"),
		ProveedorID:        c.Query("proveedor_id"),
		ConceptoID:         c.Query("concepto_id"),
		ClasificacionID:    c.Query("clasificacion_id"),
		MontoMin:           c.Query("monto_min"),
		MontoMax:           c.Query("monto_max"),
		LoteID:             c.Query("lote_id"),
		LoteFiltro:         c.Query("lote"),
		Orden:              c.Query("orden"),
		Vencimiento:        c.Query("vencimiento"),
		Abierta:            c.Query("abierta") == "true",
		Contabilidad:       c.Query("contabilidad"),
		RequiereValidacion: c.Query("requiere_validacion"),
		Fase:               c.Query("fase"),
		Page:               atoiDefault(c.Query("page"), 1),
		PageSize:           atoiDefault(c.Query("page_size"), 100),
	})
	if err != nil {
		h.responderError(c, err, "listar-documentos")
		return
	}
	c.JSON(http.StatusOK, lista)
}

// DocumentoPorID GET /v1/cxp/documentos/:id
func (h *Handler) DocumentoPorID(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	d, err := h.svc.DocumentoPorID(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "documento-por-id")
		return
	}
	c.JSON(http.StatusOK, d)
}

// RevisarDocumento POST /v1/cxp/documentos/:id/revisar
func (h *Handler) RevisarDocumento(c *gin.Context) {
	h.transicion(c, h.svc.Revisar, "revisar")
}

// AprobarDocumento POST /v1/cxp/documentos/:id/aprobar
func (h *Handler) AprobarDocumento(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	d, err := h.svc.Aprobar(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID(), claims.Rol)
	if err != nil {
		h.responderError(c, err, "aprobar-documento")
		return
	}
	c.JSON(http.StatusOK, d)
}

type programarRequest struct {
	FechaPagoProgramada string `json:"fecha_pago_programada" validate:"required,datetime=2006-01-02"`
}

// ProgramarDocumento POST /v1/cxp/documentos/:id/programar
func (h *Handler) ProgramarDocumento(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req programarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	d, err := h.svc.Programar(c.Request.Context(), empresaID, c.Param("id"), req.FechaPagoProgramada, usuarioID)
	if err != nil {
		h.responderError(c, err, "programar-documento")
		return
	}
	c.JSON(http.StatusOK, d)
}

// PagarDocumento POST /v1/cxp/documentos/:id/pagar
func (h *Handler) PagarDocumento(c *gin.Context) {
	h.transicion(c, h.svc.MarcarPagado, "pagar")
}

// ConciliarDocumento POST /v1/cxp/documentos/:id/conciliar
func (h *Handler) ConciliarDocumento(c *gin.Context) {
	h.transicion(c, h.svc.MarcarConciliado, "conciliar")
}

// transicion es el patrón común de las acciones de estado sin cuerpo.
func (h *Handler) transicion(c *gin.Context, fn func(ctx context.Context, empresaID, id, usuarioID string) (Documento, error), op string) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	d, err := fn(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, op)
		return
	}
	c.JSON(http.StatusOK, d)
}
