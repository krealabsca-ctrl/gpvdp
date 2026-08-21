package cxp

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// Handler expone los endpoints de CxP (bajo RequireEmpresa).
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler construye el handler de CxP.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

type proveedorRequest struct {
	Nombre             string `json:"nombre" validate:"required"`
	TipoIdentificacion string `json:"tipo_identificacion" validate:"omitempty,oneof=FISICA JURIDICA DIMEX NITE"`
	Identificacion     string `json:"identificacion"`
	Email              string `json:"email" validate:"omitempty,email"`
	Telefono           string `json:"telefono"`
	IBAN               string `json:"iban"`
	RetencionRentaPct  string `json:"retencion_renta_pct"`
	ExentoIVA          bool   `json:"exento_iva"`
	CondicionPago      string `json:"condicion_pago" validate:"omitempty,oneof=CONTADO CREDITO"`
	PlazoCreditoDias   int    `json:"plazo_credito_dias" validate:"gte=0,lte=365"`
	// Gasto predeterminado (alimenta el AUTO de sus facturas).
	GastoConceptoID         string `json:"gasto_concepto_id" validate:"omitempty,uuid"`
	GastoClasificacionID    string `json:"gasto_clasificacion_id" validate:"omitempty,uuid"`
	GastoSubclasificacionID string `json:"gasto_subclasificacion_id" validate:"omitempty,uuid"`
	// Departamento: área de la empresa (vocabulario controlado desde el frontend).
	Departamento string `json:"departamento" validate:"omitempty,max=40"`
}

func (req proveedorRequest) aInput() (ProveedorInput, error) {
	ret := decimal.Zero
	if s := req.RetencionRentaPct; s != "" {
		v, err := decimal.NewFromString(s)
		if err != nil {
			return ProveedorInput{}, err
		}
		ret = v
	}
	return ProveedorInput{
		Nombre:                  req.Nombre,
		TipoIdentificacion:      req.TipoIdentificacion,
		Identificacion:          req.Identificacion,
		Email:                   req.Email,
		Telefono:                req.Telefono,
		IBAN:                    req.IBAN,
		RetencionRentaPct:       ret,
		ExentoIVA:               req.ExentoIVA,
		CondicionPago:           req.CondicionPago,
		PlazoCreditoDias:        req.PlazoCreditoDias,
		GastoConceptoID:         req.GastoConceptoID,
		GastoClasificacionID:    req.GastoClasificacionID,
		GastoSubclasificacionID: req.GastoSubclasificacionID,
		Departamento:            req.Departamento,
	}, nil
}

// CrearProveedor POST /v1/cxp/proveedores
func (h *Handler) CrearProveedor(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindProveedor(c)
	if !ok {
		return
	}
	p, err := h.svc.Crear(c.Request.Context(), empresaID, in, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-proveedor")
		return
	}
	c.JSON(http.StatusCreated, p)
}

// ListarProveedores GET /v1/cxp/proveedores?q=&estado=&iva=&condicion=&retencion=&iban=&gasto=&departamento=&page=&page_size=
func (h *Handler) ListarProveedores(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	f := FiltrosProveedor{
		Q:            c.Query("q"),
		Estado:       c.Query("estado"),
		IVA:          c.Query("iva"),
		Condicion:    c.Query("condicion"),
		Retencion:    c.Query("retencion"),
		IBAN:         c.Query("iban"),
		Gasto:        c.Query("gasto"),
		Departamento: c.Query("departamento"),
	}
	lista, err := h.svc.Listar(c.Request.Context(), empresaID, f,
		atoiDefault(c.Query("page"), 1), atoiDefault(c.Query("page_size"), 100))
	if err != nil {
		h.responderError(c, err, "listar-proveedores")
		return
	}
	c.JSON(http.StatusOK, lista)
}

// ProveedorPorID GET /v1/cxp/proveedores/:id
func (h *Handler) ProveedorPorID(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	p, err := h.svc.PorID(c.Request.Context(), empresaID, c.Param("id"))
	if err != nil {
		h.responderError(c, err, "proveedor-por-id")
		return
	}
	c.JSON(http.StatusOK, p)
}

// ActualizarProveedor PATCH /v1/cxp/proveedores/:id
func (h *Handler) ActualizarProveedor(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	in, ok := bindProveedor(c)
	if !ok {
		return
	}
	p, err := h.svc.Actualizar(c.Request.Context(), empresaID, c.Param("id"), in, usuarioID)
	if err != nil {
		h.responderError(c, err, "actualizar-proveedor")
		return
	}
	c.JSON(http.StatusOK, p)
}

// DesactivarProveedor POST /v1/cxp/proveedores/:id/desactivar
func (h *Handler) DesactivarProveedor(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	if err := h.svc.Desactivar(c.Request.Context(), empresaID, c.Param("id"), usuarioID); err != nil {
		h.responderError(c, err, "desactivar-proveedor")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- helpers ----

func ctxEmpresa(c *gin.Context) (empresaID, usuarioID string, ok bool) {
	claims, exists := auth.ClaimsFromContext(c)
	if !exists {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return "", "", false
	}
	return claims.EmpresaID, claims.UsuarioID(), true
}

func bindProveedor(c *gin.Context) (ProveedorInput, bool) {
	var req proveedorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return ProveedorInput{}, false
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return ProveedorInput{}, false
	}
	in, err := req.aInput()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "retencion_renta_pct inválido")
		return ProveedorInput{}, false
	}
	return in, true
}

func (h *Handler) responderError(c *gin.Context, err error, op string) {
	switch {
	case errors.Is(err, ErrProveedorNoEncontrado), errors.Is(err, ErrDocumentoNoEncontrado), errors.Is(err, ErrComprobanteNoEncontrado), errors.Is(err, ErrDepartamentoNoEncontrado), errors.Is(err, ErrAplicacionNoEncontrada),
		errors.Is(err, ErrFondoNoEncontrado), errors.Is(err, ErrValeNoEncontrado):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, err.Error())
	case errors.Is(err, ErrFondoDuplicado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, err.Error())
	case errors.Is(err, ErrNoEsCustodio), errors.Is(err, ErrSinPermisoCartera):
		httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, err.Error())
	case errors.Is(err, ErrValeSobreLimite), errors.Is(err, ErrFondoInsuficiente), errors.Is(err, ErrFondoSinProveedor),
		errors.Is(err, ErrSinValesPendientes), errors.Is(err, ErrValeYaEnReposicion), errors.Is(err, ErrFondoInactivo),
		errors.Is(err, ErrValeGastoRequerido), errors.Is(err, ErrValeDetalleRequerido):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrDocNoPagado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, err.Error())
	case errors.Is(err, ErrProveedorSinEmail):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrProveedorDuplicado), errors.Is(err, ErrDocumentoDuplicado), errors.Is(err, ErrDepartamentoDuplicado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, err.Error())
	case errors.Is(err, ErrTransicionInvalida), errors.Is(err, ErrYaAprobado):
		httpx.Abort(c, http.StatusConflict, httpx.CodeConflicto, err.Error())
	case errors.Is(err, ErrArchivoVacio), errors.Is(err, ErrFormatoImportacion),
		errors.Is(err, ErrAccionInvalida), errors.Is(err, ErrFechaPagoRequerida), errors.Is(err, ErrSinDocumentos),
		errors.Is(err, ErrPeriodoInvalido):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
	case errors.Is(err, ErrRolNoAutorizado), errors.Is(err, ErrNoEsValidador), errors.Is(err, ErrValidadorNoAprueba),
		errors.Is(err, ErrNoAprobadorContabilidad), errors.Is(err, ErrMarcadorNoAprueba):
		httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, err.Error())
	case errors.Is(err, ErrCatalogoInvalido):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
	case errors.Is(err, ErrDeptoRequerido), errors.Is(err, ErrRespaldoRequerido), errors.Is(err, ErrEscalamientoNoAplica),
		errors.Is(err, ErrClaveRequerida), errors.Is(err, ErrMotivoAnticipoRequerido),
		errors.Is(err, ErrMotivoContabilidadRequerido), errors.Is(err, ErrContabilidadNoModificable),
		errors.Is(err, ErrNoEsDeContabilidad), errors.Is(err, ErrParametroInvalido):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrNoEsAnticipo), errors.Is(err, ErrAnticipoNoPagado), errors.Is(err, ErrProveedorDistinto),
		errors.Is(err, ErrMonedaNoNeteable), errors.Is(err, ErrFacturaNoNeteable), errors.Is(err, ErrMontoAplicacionInvalido),
		errors.Is(err, ErrReversaNoPermitida):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	default:
		// Red de seguridad: id UUID inválido (22P02) o fecha inválida (22007/22008) → 400, no 500.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "22P02" || pgErr.Code == "22007" || pgErr.Code == "22008") {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "dato inválido (id o fecha con formato incorrecto)")
			return
		}
		h.log.Error("cxp "+op, zap.Error(err))
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
	}
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
