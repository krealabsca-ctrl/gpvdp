package cxp

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// Topes del cuerpo de la recepción. El XML lo emite un tercero y el PDF puede ser cualquier cosa,
// así que se acotan con LimitReader en vez de confiar en el tamaño declarado.
const (
	topeXML = 4 << 20  // 4 MB: un comprobante real pesa entre 5 y 60 KB
	topePDF = 12 << 20 // 12 MB: una representación gráfica con logos pesa cientos de KB
)

// ── La puerta de máquina ────────────────────────────────────────────────────

// Recepcion POST /v1/cxp/recepcion — recibe un comprobante desde el buzón de una empresa.
//
// Autenticada por TOKEN DE MÁQUINA (no por JWT de usuario): el `empresa_id` sale de la fila del
// token, nunca del cuerpo. La ruta cuelga de `v1` con su propio middleware y NO usa `P(...)`,
// porque RequirePermiso necesita un código de rol y esta llamada no tiene usuario: con rol vacío
// daría 403 siempre, y con empresa vacía daría 500. El middleware ES el autorizador de esta ruta.
//
// El cuerpo es multipart: `xml` (obligatorio), `pdf` (opcional) y los metadatos del correo.
func (h *Handler) Recepcion(c *gin.Context) {
	tok, ok := tokenMaquinaDeContexto(c)
	if !ok {
		return
	}

	xml, ok := h.leerParteAcotada(c, "xml", topeXML, true)
	if !ok {
		return
	}
	pdf, ok := h.leerParteAcotada(c, "pdf", topePDF, false)
	if !ok {
		return
	}
	var pdfName string
	if fh, err := c.FormFile("pdf"); err == nil {
		pdfName = fh.Filename
	}

	res, err := h.svc.RecibirComprobante(c.Request.Context(), tok, RecepcionInput{
		XML:         xml,
		PDF:         pdf,
		PDFFilename: pdfName,
		MessageID:   c.PostForm("message_id"),
		Asunto:      c.PostForm("asunto"),
		Remitente:   c.PostForm("remitente"),
		Buzon:       c.PostForm("buzon"),
	})
	if err != nil {
		// Un error acá es del SISTEMA, no del comprobante: tiene que salir con 5xx para que el
		// script NO etiquete el hilo y lo reintente en la corrida siguiente.
		h.responderError(c, err, "recepcion")
		return
	}
	// Siempre 200, incluso cuando la recepción quedó parqueada o descartada: el comprobante llegó
	// y está guardado, así que el hilo del correo ya se puede etiquetar. Con un 4xx el script
	// volvería a mandarlo para siempre y la cola se llenaría de copias.
	c.JSON(http.StatusOK, res)
}

// LatidoRecepcion POST /v1/cxp/recepcion/latido — el script avisa que corrió, aunque no haya
// facturas nuevas.
//
// Sin esto, que la integración se apague se ve en pantalla igual que un día tranquilo.
func (h *Handler) LatidoRecepcion(c *gin.Context) {
	tok, ok := tokenMaquinaDeContexto(c)
	if !ok {
		return
	}
	if err := h.svc.Latido(c.Request.Context(), tok); err != nil {
		h.responderError(c, err, "recepcion-latido")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "buzon": tok.Correo})
}

// leerParteAcotada lee una parte del multipart con tope de tamaño.
func (h *Handler) leerParteAcotada(c *gin.Context, campo string, tope int64, obligatoria bool) ([]byte, bool) {
	fh, err := c.FormFile(campo)
	if err != nil {
		if obligatoria {
			httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
				"falta el archivo «"+campo+"»")
			return nil, false
		}
		return nil, true
	}
	f, err := fh.Open()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer «"+campo+"»")
		return nil, false
	}
	defer func() { _ = f.Close() }()
	// LimitReader y no ReadAll a secas: el tamaño declarado por el cliente no es de fiar.
	b, err := io.ReadAll(io.LimitReader(f, tope+1))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "no se pudo leer «"+campo+"»")
		return nil, false
	}
	if int64(len(b)) > tope {
		httpx.Abort(c, http.StatusRequestEntityTooLarge, httpx.CodeValidacion,
			"el archivo «"+campo+"» excede el tamaño permitido")
		return nil, false
	}
	return b, true
}

// ── La bandeja de recepción (usuarios) ──────────────────────────────────────

// Recepciones GET /v1/cxp/recepciones — la bandeja y la cola de errores.
func (h *Handler) Recepciones(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	limite, _ := strconv.Atoi(c.Query("limite"))
	lista, err := h.svc.Recepciones(c.Request.Context(), empresaID, FiltrosRecepcion{
		Estado: c.Query("estado"), Q: c.Query("q"), Limite: limite,
	})
	if err != nil {
		h.responderError(c, err, "recepciones")
		return
	}
	resumen, err := h.svc.ResumenRecepcion(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "recepciones-resumen")
		return
	}
	cedulas, err := h.svc.CedulasDeEmpresa(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "recepciones-cedulas")
		return
	}
	// Las cédulas viajan para que la pantalla pueda decir contra qué se coteja —y avisar cuando no
	// hay ninguna, que es el caso en que el guardarraíl no puede funcionar—.
	c.JSON(http.StatusOK, gin.H{"resumen": resumen, "recepciones": lista, "cedulas": cedulas})
}

// ReintentarRecepcion POST /v1/cxp/recepciones/:id/reintentar — reprocesa una parqueada.
func (h *Handler) ReintentarRecepcion(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	res, err := h.svc.ReintentarRecepcion(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "reintentar-recepcion")
		return
	}
	c.JSON(http.StatusOK, res)
}

// ArchivoRecepcion GET /v1/cxp/recepciones/:id/archivo?cual=xml|pdf — descarga el original.
func (h *Handler) ArchivoRecepcion(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	cual := c.Query("cual")
	if cual != "pdf" {
		cual = "xml"
	}
	a, err := h.svc.ArchivoDeRecepcion(c.Request.Context(), empresaID, c.Param("id"), cual)
	if err != nil {
		h.responderError(c, err, "archivo-recepcion")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+a.Filename+`"`)
	c.Data(http.StatusOK, a.Mime, a.Contenido)
}

// ── Las fuentes de recepción (configuración) ────────────────────────────────

type fuenteRequest struct {
	Nombre string `json:"nombre" validate:"required,min=2,max=80"`
	Correo string `json:"correo" validate:"required,email"`
}

// CrearFuente POST /v1/cxp/fuentes — da de alta un buzón y devuelve su token UNA sola vez.
func (h *Handler) CrearFuente(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req fuenteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := httpx.Validate.Struct(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	out, err := h.svc.CrearFuente(c.Request.Context(), empresaID,
		FuenteInput{Nombre: req.Nombre, Correo: req.Correo}, usuarioID)
	if err != nil {
		h.responderError(c, err, "crear-fuente")
		return
	}
	c.JSON(http.StatusCreated, out)
}

// Fuentes GET /v1/cxp/fuentes — los buzones de la empresa (sin el token).
func (h *Handler) Fuentes(c *gin.Context) {
	empresaID, _, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	lista, err := h.svc.Fuentes(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "fuentes")
		return
	}
	cedulas, err := h.svc.CedulasDeEmpresa(c.Request.Context(), empresaID)
	if err != nil {
		h.responderError(c, err, "fuentes-cedulas")
		return
	}
	c.JSON(http.StatusOK, gin.H{"fuentes": lista, "cedulas": cedulas})
}

// RotarTokenFuente POST /v1/cxp/fuentes/:id/rotar — nueva credencial; la vieja deja de servir.
func (h *Handler) RotarTokenFuente(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	token, err := h.svc.RotarTokenFuente(c.Request.Context(), empresaID, c.Param("id"), usuarioID)
	if err != nil {
		h.responderError(c, err, "rotar-token")
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

type estadoFuenteRequest struct {
	Activo *bool `json:"activo" validate:"required"`
}

// CambiarEstadoFuente PATCH /v1/cxp/fuentes/:id — activa o desactiva el buzón.
func (h *Handler) CambiarEstadoFuente(c *gin.Context) {
	empresaID, usuarioID, ok := ctxEmpresa(c)
	if !ok {
		return
	}
	var req estadoFuenteRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Activo == nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	if err := h.svc.CambiarEstadoFuente(c.Request.Context(), empresaID, c.Param("id"),
		*req.Activo, usuarioID); err != nil {
		h.responderError(c, err, "estado-fuente")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// tokenMaquinaDeContexto recupera lo que resolvió el middleware de token de máquina.
func tokenMaquinaDeContexto(c *gin.Context) (TokenMaquina, bool) {
	v, existe := c.Get(ctxTokenMaquina)
	if !existe {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "token inválido o expirado")
		return TokenMaquina{}, false
	}
	tok, ok := v.(TokenMaquina)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "token inválido o expirado")
		return TokenMaquina{}, false
	}
	return tok, true
}

// ctxTokenMaquina es la clave con la que el middleware deja el token resuelto en el contexto.
const ctxTokenMaquina = "gpvdp_token_maquina"

// RequireTokenRecepcion es el middleware de la puerta de máquina.
//
// Vive acá y no en `internal/tenant` porque la credencial ES la fuente de recepción, que es un
// concepto de CxP: meterlo en tenant obligaría a que tenant conozca el dominio de CxP.
//
// NO sintetiza un `auth.Claims`. Sería lo más cómodo —todos los handlers existentes leen la
// empresa de ahí— pero dejaría a RequireEmpresa y RequirePermiso «funcionando» con un usuario y un
// rol inventados, que es exactamente el bug de autorización por código de rol que este repo ya
// cometió tres veces. Esta ruta se autoriza acá y en ningún otro lado.
func (h *Handler) RequireTokenRecepcion() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		const prefijo = "Bearer "
		if len(header) <= len(prefijo) || header[:len(prefijo)] != prefijo {
			httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado,
				"falta el token de recepción")
			return
		}
		tok, err := h.svc.VerificarTokenRecepcion(c.Request.Context(), header[len(prefijo):])
		if err != nil {
			if errors.Is(err, ErrTokenRecepcionInvalido) {
				httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado,
					"token inválido o expirado")
				return
			}
			// Un error de infraestructura NO se traduce a «sin permiso»: si la base está caída, el
			// script tiene que reintentar, no dar por bueno que la credencial dejó de valer.
			httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno,
				"no se pudo verificar el token")
			return
		}
		c.Set(ctxTokenMaquina, tok)
		c.Next()
	}
}
