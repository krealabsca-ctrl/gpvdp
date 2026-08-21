package bancos

// Endpoints de la conciliación bancaria mensual: actas, partidas en tránsito, firma del acta y
// congelamiento de los saldos capturados.

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// periodoDeQuery lee ?periodo=YYYY-MM. Vacío = mes en curso de Costa Rica.
func periodoDeQuery(c *gin.Context) (int, int, bool) {
	p := strings.TrimSpace(c.Query("periodo"))
	if p == "" {
		p = HoyCR()[:7]
	}
	return parsePeriodoTexto(p)
}

// Conciliacion GET /v1/bancos/conciliacion?periodo=YYYY-MM (bancos.ver)
// Las actas del mes de todas las cuentas activas, con su diferencia sin explicar.
func (h *Handler) Conciliacion(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	anio, mes, ok := periodoDeQuery(c)
	if !ok {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "período inválido (se espera YYYY-MM)")
		return
	}
	conc, err := h.svc.Conciliacion(c.Request.Context(), claims.EmpresaID, anio, mes)
	if err != nil {
		h.responderError(c, err, "conciliacion")
		return
	}
	c.JSON(http.StatusOK, conc)
}

// registrarPartidaRequest es la captura de una partida en tránsito.
type registrarPartidaRequest struct {
	CuentaID    string `json:"cuenta_id" binding:"required"`
	Periodo     string `json:"periodo" binding:"required"`
	Tipo        string `json:"tipo" binding:"required"`
	Descripcion string `json:"descripcion" binding:"required"`
	Monto       string `json:"monto" binding:"required"`
	// Signo solo se exige cuando el tipo es OTRA (los demás los fija el sistema).
	Signo int `json:"signo"`
}

// RegistrarPartida POST /v1/bancos/conciliacion/partidas (bancos.conciliar)
func (h *Handler) RegistrarPartida(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req registrarPartidaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion,
			"cuerpo inválido: se esperan cuenta, período, tipo, descripción y monto")
		return
	}
	anio, mes, ok := parsePeriodoTexto(req.Periodo)
	if !ok {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "período inválido (se espera YYYY-MM)")
		return
	}
	id, err := h.svc.RegistrarPartida(c.Request.Context(), claims.EmpresaID, PartidaInput{
		CuentaID: req.CuentaID, Anio: anio, Mes: mes, Tipo: req.Tipo,
		Descripcion: req.Descripcion, Monto: req.Monto, Signo: req.Signo,
	}, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "registrar-partida")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// AnularPartida DELETE /v1/bancos/conciliacion/partidas/:id (bancos.conciliar)
// No borra: anula y deja el rastro.
func (h *Handler) AnularPartida(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.AnularPartida(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID()); err != nil {
		h.responderError(c, err, "anular-partida")
		return
	}
	c.JSON(http.StatusOK, gin.H{"anulada": true})
}

// firmarActaRequest identifica el acta a firmar.
type firmarActaRequest struct {
	CuentaID string `json:"cuenta_id" binding:"required"`
	Periodo  string `json:"periodo" binding:"required"`
}

// FirmarActa POST /v1/bancos/conciliacion/firmar (bancos.conciliar)
// Solo firma si la diferencia sin explicar es cero.
func (h *Handler) FirmarActa(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req firmarActaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido: se esperan cuenta y período")
		return
	}
	anio, mes, ok := parsePeriodoTexto(req.Periodo)
	if !ok {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "período inválido (se espera YYYY-MM)")
		return
	}
	if err := h.svc.FirmarActa(c.Request.Context(), claims.EmpresaID, req.CuentaID, anio, mes, claims.UsuarioID()); err != nil {
		h.responderError(c, err, "firmar-acta")
		return
	}
	c.JSON(http.StatusOK, gin.H{"firmada": true})
}

// revisarSaldosRequest congela o descongela los saldos de un día.
type revisarSaldosRequest struct {
	Fecha    string `json:"fecha"`
	Congelar *bool  `json:"congelar" binding:"required"`
	Motivo   string `json:"motivo"`
}

// RevisarSaldos POST /v1/bancos/saldos/revisar (bancos.saldos_revisar)
// Congelar = Dirección Financiera revisó el día; después nadie edita esos saldos sin
// descongelarlos primero, y eso queda auditado.
func (h *Handler) RevisarSaldos(c *gin.Context) {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var req revisarSaldosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "cuerpo inválido: se espera congelar true/false")
		return
	}
	n, err := h.svc.RevisarSaldos(c.Request.Context(), claims.EmpresaID, req.Fecha, *req.Congelar, req.Motivo, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "revisar-saldos")
		return
	}
	c.JSON(http.StatusOK, gin.H{"cuentas": n, "congelados": *req.Congelar})
}

// parsePeriodoTexto valida YYYY-MM del cuerpo de un request.
func parsePeriodoTexto(p string) (int, int, bool) {
	p = strings.TrimSpace(p)
	if len(p) != 7 || p[4] != '-' {
		return 0, 0, false
	}
	anio, err1 := strconv.Atoi(p[:4])
	mes, err2 := strconv.Atoi(p[5:])
	if err1 != nil || err2 != nil || !periodoValidoBancos(anio, mes) {
		return 0, 0, false
	}
	return anio, mes, true
}
