package bancos

// La capa HTTP de los formatos de banco configurables (mig 0108).

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// archivoDeFormulario lee el Excel del multipart. Mismo campo («archivo») que el importador: es
// el mismo archivo, subido desde otra pantalla.
func archivoDeFormulario(c *gin.Context) ([]byte, error) {
	fh, err := c.FormFile("archivo")
	if err != nil {
		return nil, errors.New("subí el estado de cuenta con el que querés probar")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, errors.New("no se pudo leer el archivo")
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, errors.New("no se pudo leer el archivo")
	}
	return data, nil
}

// FormatosDeBancoHandler GET /v1/bancos/formatos (bancos.formatos)
//
// Devuelve también la lista de formatos de fecha disponibles: es una lista CERRADA del servidor, y
// escribirla otra vez en la pantalla es la forma de que un día ofrezca una opción que el backend
// no sabe traducir.
func (h *Handler) FormatosDeBancoHandler(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	items, err := h.svc.FormatosDeBanco(c.Request.Context(), cl.EmpresaID, c.Query("activos") == "1")
	if err != nil {
		h.responderError(c, err, "formatos")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":             items,
		"formatos_fecha":    FormatosDeFecha,
		"bancos_del_codigo": bancosDelCodigo(),
	})
}

// GuardarFormatoBancoHandler POST /v1/bancos/formatos (bancos.formatos)
func (h *Handler) GuardarFormatoBancoHandler(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var f FormatoBanco
	if err := c.ShouldBindJSON(&f); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el formato no se pudo leer")
		return
	}
	guardado, err := h.svc.GuardarFormatoBanco(c.Request.Context(), cl.EmpresaID, f, cl.UsuarioID())
	if err != nil {
		h.responderError(c, err, "guardar-formato")
		return
	}
	c.JSON(http.StatusOK, guardado)
}

// BorrarFormatoBancoHandler DELETE /v1/bancos/formatos/:id (bancos.formatos)
func (h *Handler) BorrarFormatoBancoHandler(c *gin.Context) {
	cl, ok := auth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	if err := h.svc.BorrarFormatoBanco(c.Request.Context(), cl.EmpresaID, c.Param("id"), cl.UsuarioID()); err != nil {
		h.responderError(c, err, "borrar-formato")
		return
	}
	c.Status(http.StatusNoContent)
}

// ProbarFormatoBancoHandler POST /v1/bancos/formatos/probar (bancos.formatos)
//
// Multipart: el archivo de verdad más el formato en JSON. NO guarda nada; corre el formato contra
// el archivo y devuelve lo que entendió, para poder corregir las columnas antes de que exista.
func (h *Handler) ProbarFormatoBancoHandler(c *gin.Context) {
	if _, ok := auth.ClaimsFromContext(c); !ok {
		httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
		return
	}
	var f FormatoBanco
	if err := json.Unmarshal([]byte(c.PostForm("formato")), &f); err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, "el formato no se pudo leer")
		return
	}
	archivo, err := archivoDeFormulario(c)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, httpx.CodeValidacion, err.Error())
		return
	}
	prueba, err := h.svc.ProbarFormato(c.Request.Context(), f, archivo)
	if err != nil {
		h.responderError(c, err, "probar-formato")
		return
	}
	c.JSON(http.StatusOK, prueba)
}

// bancosDelCodigo nombra los seis que vienen de fábrica.
//
// La pantalla los muestra para que nadie configure uno que ya existe —y para que se entienda que
// un formato propio NO reemplaza a estos: se prueba antes, y si no calza, estos siguen ahí.
func bancosDelCodigo() []string {
	out := make([]string, 0, len(Adapters()))
	for _, a := range Adapters() {
		out = append(out, string(a.Banco()))
	}
	return out
}
