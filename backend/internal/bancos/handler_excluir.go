package bancos

// Endpoints para sacar un movimiento de los libros y para encontrar los duplicados.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// ExcluirMovimiento POST /v1/bancos/movimientos/:id/excluir (bancos.excluir_movimiento)
//
// El motivo va en el cuerpo y es obligatorio: sin él, el movimiento desaparece de los libros y
// nadie puede distinguir una corrección de un error del sistema.
func (h *Handler) ExcluirMovimiento(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	var req struct {
		Motivo string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "decí por qué se saca este movimiento")
		return
	}
	err := h.svc.ExcluirMovimiento(c.Request.Context(), claims.EmpresaID, c.Param("id"),
		req.Motivo, claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "excluir-movimiento")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// IncluirMovimiento POST /v1/bancos/movimientos/:id/incluir (bancos.excluir_movimiento)
//
// Mismo permiso que excluir: es la misma autoridad —mover plata dentro y fuera de los libros— y
// quien puede sacar tiene que poder deshacer lo suyo sin pedirle a otro.
func (h *Handler) IncluirMovimiento(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	err := h.svc.IncluirMovimiento(c.Request.Context(), claims.EmpresaID, c.Param("id"), claims.UsuarioID())
	if err != nil {
		h.responderError(c, err, "incluir-movimiento")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ExcluidosAMano GET /v1/bancos/movimientos/excluidos (bancos.ver_clasificar)
//
// Se LEE con el permiso de ver, no con el de excluir: plata fuera de los libros que solo puede
// mirar quien la sacó no es auditable. Quien cierra el mes tiene que ver esta lista entera.
func (h *Handler) ExcluidosAMano(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	items, err := h.svc.ExcluidosAMano(c.Request.Context(), claims.EmpresaID)
	if err != nil {
		h.responderError(c, err, "excluidos-a-mano")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

// DuplicadosSospechosos GET /v1/bancos/movimientos/duplicados?desde=&hasta= (bancos.ver_clasificar)
//
// Devuelve PARES con sus señales, no un veredicto. La respuesta trae `senales` (0 a 3) ya contado
// por el servidor para que la pantalla no invente su propio criterio.
func (h *Handler) DuplicadosSospechosos(c *gin.Context) {
	claims, ok := claimsDe(c)
	if !ok {
		return
	}
	pares, err := h.svc.DuplicadosSospechosos(c.Request.Context(), claims.EmpresaID,
		c.Query("desde"), c.Query("hasta"))
	if err != nil {
		h.responderError(c, err, "duplicados-sospechosos")
		return
	}
	// `con_las_tres` es el número por el que se pregunta al abrir la pantalla: cuántos pares
	// cumplen las tres señales. Se cuenta acá y no en el cliente para que no haya dos criterios.
	conLasTres := 0
	for _, p := range pares {
		if p.Senales == 3 {
			conLasTres++
		}
	}
	c.JSON(http.StatusOK, gin.H{"pares": pares, "total": len(pares), "con_las_tres": conLasTres})
}
