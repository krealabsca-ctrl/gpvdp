package server

// `/readyz`: si el sistema SIRVE, no solo si el proceso vive (21-oct-2026, punto 4 de la revisión
// comercial).
//
// ── POR QUÉ NO ALCANZABA `/healthz` ─────────────────────────────────────────────────────────
//
// `/healthz` devuelve «ok» sin mirar nada: es un `return` constante. Eso está BIEN para lo que
// hace —es el chequeo de vida que usa Docker para decidir si reinicia el contenedor, y tiene que
// ser barato y no depender de la base, o una caída de PostgreSQL reiniciaría el backend en bucle
// sin arreglar nada—. Por eso no se toca: lo consumen el `healthcheck` del compose, el instalador
// y `actualizar.sh`, los tres buscando la palabra «ok».
//
// Lo que faltaba es el otro chequeo. El sistema puede estar perfectamente «vivo» y no servir:
//
//   · La base dejó de responder y toda pantalla da error.
//   · Una migración quedó a MEDIAS (`dirty`). El esquema está incompleto y el problema aparece
//     días después, en una pantalla cualquiera, sin que nadie lo relacione. Pasó el 21-oct-2026
//     con la 0106.
//   · La base está vacía porque la siembra falló en silencio. Pasó: el arranque registraba el
//     error y continuaba, y quedaba un sistema sano SIN empresas, donde nadie puede entrar.
//
// Los tres se ven desde adentro y ninguno se veía desde afuera. Un monitor externo apuntado acá
// —que es lo que la guía del VPS recomienda— ahora avisa de cosas que pasan de verdad, en vez de
// confirmar que el proceso respira.
//
// ── POR QUÉ 503 Y NO 200 CON UN CAMPO ──────────────────────────────────────────────────────
//
// Porque lo va a leer una máquina. Un monitor de disponibilidad mira el código de estado; si todo
// contestara 200 habría que configurarlo para que además lea el cuerpo, y eso es exactamente lo
// que nadie hace. El detalle va en el cuerpo igual, para quien entre a mirar.

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// EstadoDelSistema es lo que `/readyz` necesita saber. Lo declara el servidor y lo satisface el
// armado, igual que los demás puertos del proyecto: así `server` no depende de pgx.
type EstadoDelSistema interface {
	// Ping comprueba que la base responda.
	Ping(ctx context.Context) error
	// Esquema devuelve la versión aplicada y si quedó a medias.
	Esquema(ctx context.Context) (version int64, sucio bool, err error)
	// HayEmpresas dice si el sistema tiene al menos una empresa. Sin ninguna, entrar es imposible
	// —el ingreso exige elegir una— y el sistema está instalado pero inservible.
	HayEmpresas(ctx context.Context) (bool, error)
}

// tiempoDeChequeo acota cuánto puede tardar la verificación.
//
// Tres segundos: un monitor que pregunta cada minuto no puede quedarse colgado esperando a una
// base que no contesta, porque entonces el monitor deja de avisar justo cuando hay algo que avisar.
const tiempoDeChequeo = 3 * time.Second

func readyz(estado EstadoDelSistema) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Sin verificador inyectado no se puede afirmar nada, y afirmar «listo» sin haber mirado
		// sería peor que no tener el endpoint.
		if estado == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"listo": false, "detalle": "el verificador de estado no está configurado",
			})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), tiempoDeChequeo)
		defer cancel()

		if err := estado.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"listo": false, "detalle": "la base de datos no responde",
			})
			return
		}
		version, sucio, err := estado.Esquema(ctx)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"listo": false, "detalle": "no se pudo leer la versión del esquema",
			})
			return
		}
		if sucio {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"listo": false, "esquema": version, "sucio": true,
				"detalle": "una migración quedó a medias; el esquema está incompleto",
			})
			return
		}
		hay, err := estado.HayEmpresas(ctx)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"listo": false, "detalle": "no se pudo comprobar si hay empresas",
			})
			return
		}
		if !hay {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"listo": false, "esquema": version, "empresas": 0,
				"detalle": "no hay ninguna empresa: la siembra inicial no se completó y nadie puede entrar",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"listo": true, "esquema": version})
	}
}
