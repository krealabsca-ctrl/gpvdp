package correo

// Los tres endpoints del correo saliente. Cero SQL y cero reglas: solo bind, llamada y traducción
// del error a su estado HTTP.

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
	"github.com/gpvdp/erp/internal/shared"
)

// Handler expone la configuración del correo saliente (permiso admin.correo).
type Handler struct {
	svc *Service
	// log es lo que hace visible del lado del servidor un fallo que el usuario ve como «error
	// interno». Sin él, el reporte «guardé el correo y se rompió» no tiene NADA que mirar: el
	// `default` de responder devuelve 500 y el motivo real se pierde.
	log *zap.Logger
}

// NewHandler construye el handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler { return &Handler{svc: svc, log: log} }

// Obtener GET /v1/correo-saliente — la configuración de la empresa activa.
//
// NO DEVUELVE LA CONTRASEÑA. No existe el campo en el contrato: si no está en el tipo, nadie la
// puede pintar por error.
func (h *Handler) Obtener(c *gin.Context) {
	empresaID, _, ok := contexto(c)
	if !ok {
		return
	}
	cfg, err := h.svc.Obtener(c.Request.Context(), empresaID)
	if err != nil {
		h.responder(c, err)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// guardarRequest es el cuerpo del PUT.
//
// `Password` es *shared.Secreto: entra normal desde el JSON (el tipo es un string por debajo y no
// define UnmarshalJSON) y NO PUEDE SALIR, porque su MarshalJSON imprime `***`. Así, si alguien
// pasa esta struct entera a la auditoría o a un log, la contraseña no viaja. (Hallazgos S-2/S-6.)
type guardarRequest struct {
	Host            string          `json:"host"`
	Puerto          int             `json:"puerto"`
	Seguridad       string          `json:"seguridad"`
	Usuario         string          `json:"usuario"`
	Password        *shared.Secreto `json:"password"`
	Remitente       string          `json:"remitente"`
	RemitenteNombre string          `json:"remitente_nombre"`
	Activo          *bool           `json:"activo"`
}

// Guardar PUT /v1/correo-saliente — crea o reemplaza la configuración de la empresa activa.
func (h *Handler) Guardar(c *gin.Context) {
	empresaID, usuarioID, ok := contexto(c)
	if !ok {
		return
	}
	var req guardarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido")
		return
	}
	activo := true // omitido = activo: guardar una configuración para dejarla apagada es el caso raro
	if req.Activo != nil {
		activo = *req.Activo
	}
	in := Input{
		Host: req.Host, Puerto: req.Puerto, Seguridad: req.Seguridad, Usuario: req.Usuario,
		Password: req.Password, Remitente: req.Remitente, RemitenteNombre: req.RemitenteNombre,
		Activo: activo,
	}
	if err := h.svc.Guardar(c.Request.Context(), empresaID, in, usuarioID); err != nil {
		h.responder(c, err)
		return
	}
	cfg, err := h.svc.Obtener(c.Request.Context(), empresaID)
	if err != nil {
		h.responder(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"guardada": true, "tiene_password": cfg.TienePassword})
}

// Probar POST /v1/correo-saliente/probar — manda un correo de prueba al correo del propio usuario.
//
// Sin cuerpo: el destino NO se elige (ver Service.Probar).
func (h *Handler) Probar(c *gin.Context) {
	empresaID, usuarioID, ok := contexto(c)
	if !ok {
		return
	}
	p, err := h.svc.Probar(c.Request.Context(), empresaID, usuarioID)
	if err != nil {
		h.responder(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func contexto(c *gin.Context) (empresaID, usuarioID string, ok bool) {
	claims, existe := auth.ClaimsFromContext(c)
	if !existe {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return "", "", false
	}
	// La empresa sale de los claims (el token la acuñó contra la membresía), nunca del cuerpo.
	return claims.EmpresaID, claims.UsuarioID(), true
}

// responder traduce cada centinela a su estado HTTP.
//
// La lista de abajo es la red contra el defecto que ya mordió a este proyecto varias veces: se
// agrega un centinela, se olvida el caso, y una regla de negocio sale como 500 «error interno»,
// con lo cual el usuario no lee el motivo y reintenta lo mismo. Todo centinela nuevo de este
// paquete va acá Y en el test.
func (h *Handler) responder(c *gin.Context, err error) {
	var smtpErr *shared.ErrorSMTP
	switch {
	// Validación de borde: el cuerpo está mal formado.
	case errors.Is(err, ErrHostRequerido), errors.Is(err, ErrPuertoInvalido),
		errors.Is(err, ErrSeguridadInvalida), errors.Is(err, ErrRemitenteInvalido):
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
	// Reglas: el pedido está bien formado pero no se puede cumplir. Cada mensaje dice qué hacer.
	case errors.Is(err, ErrPasswordRequerida), errors.Is(err, ErrPasswordRequeridaPorCambio),
		errors.Is(err, ErrHostNoPermitido), errors.Is(err, ErrSinCorreoDePrueba):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	// Falta CIFRADO_SECRET (o no sirve): 422 NOMBRANDO LA VARIABLE, nunca 500 ni panic, y nada
	// quedó guardado en claro.
	case errors.Is(err, shared.ErrClaveAusente), errors.Is(err, shared.ErrClaveCorta),
		errors.Is(err, shared.ErrClaveDebil):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	// La contraseña guardada no descifra: clave rotada, dato manipulado o servidor cambiado.
	case errors.Is(err, shared.ErrNoDescifra), errors.Is(err, shared.ErrFormato):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, shared.ErrSMTPNoConfigurado):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio,
			"el correo saliente no está configurado: guardá el servidor de esta empresa, o definí SMTP_ADDR y SMTP_FROM en el servidor")
	// El servidor de correo rechazó algo. El mensaje YA viene clasificado y sin el texto crudo del
	// servidor; el detalle técnico queda en probado_error, que se lee con este mismo permiso.
	case errors.As(err, &smtpErr):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, smtpErr.Detalle())
	case errors.Is(err, ErrPruebaMuySeguida):
		httpx.Abort(c, http.StatusTooManyRequests, httpx.CodeReglaNegocio, err.Error())
	case errors.Is(err, ErrConfigNoEncontrada):
		httpx.Abort(c, http.StatusNotFound, httpx.CodeNoEncontrado, err.Error())
	// Falta el destinatario. Hoy no se alcanza desde esta pantalla, pero sin el caso saldría como
	// 500 y el usuario leería «error interno» sobre algo que sí se puede explicar.
	case errors.Is(err, shared.ErrSinDestinatario):
		httpx.Abort(c, http.StatusUnprocessableEntity, httpx.CodeReglaNegocio, err.Error())
	default:
		// Lo único que llega acá es un fallo de infraestructura (base caída, bug). Se loguea con el
		// error real porque del lado del cliente sale «error interno» a propósito: si tampoco queda
		// del lado del servidor, el reporte «guardé y se rompió» no tiene nada que mirar.
		if h != nil && h.log != nil {
			h.log.Error("correo: error interno", zap.Error(err))
		}
		httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error interno")
	}
}
