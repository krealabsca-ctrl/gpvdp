// Package tenant provee el middleware de autenticación y aislamiento por empresa.
package tenant

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// PermisoChecker resuelve si (empresa, rol) tiene un permiso (lo implementa rbac.Service).
type PermisoChecker interface {
	Tiene(ctx context.Context, empresaID, rolCodigo, permiso string) (bool, error)
}

// RequireAuth valida el Bearer JWT y coloca los claims en el contexto.
// El empresa_id activo proviene SIEMPRE del token (claim), nunca del body/query.
func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "falta el token de acceso")
			return
		}
		tokenStr := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		claims, err := auth.ParseAccessToken(secret, tokenStr)
		if err != nil {
			httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "token inválido o expirado")
			return
		}
		auth.SetClaims(c, claims)
		c.Next()
	}
}

// RequireEmpresa exige que el token esté scopeado a una empresa (empresa seleccionada).
func RequireEmpresa() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := auth.ClaimsFromContext(c)
		if !ok || claims.EmpresaID == "" {
			httpx.Abort(c, http.StatusForbidden, httpx.CodeEmpresaNoSel, "seleccione una empresa para continuar")
			return
		}
		c.Next()
	}
}

// RequirePermiso exige que (empresa, rol) del token tenga el permiso indicado según
// la matriz RBAC configurable (permiso × rol × empresa). ADMIN tiene bypass (chk lo
// resuelve). Deny-by-default: si el checker dice que no, se responde 403.
func RequirePermiso(chk PermisoChecker, permiso string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := auth.ClaimsFromContext(c)
		if !ok {
			httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
			return
		}
		tiene, err := chk.Tiene(c.Request.Context(), claims.EmpresaID, claims.Rol, permiso)
		if err != nil {
			httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error al verificar permisos")
			return
		}
		if !tiene {
			httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, "no tenés permiso para esta acción")
			return
		}
		c.Next()
	}
}

// RequireAlgunPermiso exige AL MENOS UNO de los permisos indicados (OR, no AND).
//
// Hace falta porque hay rutas que sirven a más de una pantalla, y al partir el permiso de lectura de
// Bancos en uno por pantalla dejaron de tener un dueño único. Los dos casos reales:
//
//   - `/bancos/movimientos` lo consumen Clasificar, el árbol del Dashboard, Exportar y —de otro
//     módulo— Inventario › Entradas, para ligar una entrada con el movimiento que la pagó.
//   - `/bancos/catalogo/sedes` es el catálogo de sedes COMPARTIDO: lo leen seis pantallas de
//     Inventario además de Control y presupuesto.
//
// La alternativa era dejar esas rutas con un permiso grueso, que es justo lo que se vino a arreglar:
// obligaría a dar acceso de lectura a todo Bancos para que funcione una pantalla de bodega.
//
// Se corta en el PRIMER permiso que da true, así que conviene poner primero el más común. Un fallo
// del checker se responde 500 y no se sigue probando: no se puede distinguir «no tiene» de «no se
// pudo averiguar», y tratar un error como una negativa esconde una base caída detrás de un 403.
func RequireAlgunPermiso(chk PermisoChecker, permisos ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := auth.ClaimsFromContext(c)
		if !ok {
			httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
			return
		}
		for _, permiso := range permisos {
			tiene, err := chk.Tiene(c.Request.Context(), claims.EmpresaID, claims.Rol, permiso)
			if err != nil {
				httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno, "error al verificar permisos")
				return
			}
			if tiene {
				c.Next()
				return
			}
		}
		httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, "no tenés permiso para esta acción")
	}
}

// RequireRol exige que el rol del token esté dentro de los códigos permitidos (RBAC básico).
// NOTA: reemplazado por RequirePermiso (matriz configurable); se conserva por compatibilidad.
func RequireRol(codigos ...string) gin.HandlerFunc {
	permitidos := make(map[string]struct{}, len(codigos))
	for _, cod := range codigos {
		permitidos[cod] = struct{}{}
	}
	return func(c *gin.Context) {
		claims, ok := auth.ClaimsFromContext(c)
		if !ok {
			httpx.Abort(c, http.StatusUnauthorized, httpx.CodeNoAutenticado, "no autenticado")
			return
		}
		if _, allowed := permitidos[claims.Rol]; !allowed {
			httpx.Abort(c, http.StatusForbidden, httpx.CodeSinPermiso, "rol no autorizado para esta acción")
			return
		}
		c.Next()
	}
}
