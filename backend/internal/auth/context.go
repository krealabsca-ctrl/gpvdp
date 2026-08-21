package auth

import "github.com/gin-gonic/gin"

// ctxClaimsKey es la clave bajo la cual el middleware guarda los claims validados.
const ctxClaimsKey = "gpvdp_claims"

// SetClaims guarda los claims autenticados en el contexto de la petición.
func SetClaims(c *gin.Context, claims *Claims) {
	c.Set(ctxClaimsKey, claims)
}

// ClaimsFromContext recupera los claims autenticados de la petición.
func ClaimsFromContext(c *gin.Context) (*Claims, bool) {
	v, ok := c.Get(ctxClaimsKey)
	if !ok {
		return nil, false
	}
	claims, ok := v.(*Claims)
	return claims, ok
}
