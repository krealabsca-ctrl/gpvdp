package server

// El único punto donde un error del sistema queda escrito (mig 0107).
//
// ── POR QUÉ UN SOLO MIDDLEWARE Y NO DOS ─────────────────────────────────────────────────────
//
// Un 500 y un panic terminan igual para quien los sufre —una pantalla que no hizo lo que pedía—
// y tienen que terminar igual para quien los investiga: una referencia que se canjea por la
// causa. Separarlos en dos middlewares significa dos lugares que arman el mismo registro, y el
// día que uno gane un campo el otro se queda atrás.
//
// La diferencia real es que el 500 ya pasó por `httpx.AbortInterno` —que generó la referencia, la
// logueó y se la devolvió al usuario— y el panic no pasó por nada: `gin.Recovery()` respondía 500
// SIN cuerpo, así que el usuario veía una pantalla rota y no tenía nada que dictar.
//
// ── ESTE MIDDLEWARE REEMPLAZA A gin.Recovery() ──────────────────────────────────────────────
//
// Hace lo mismo —atrapar el panic para que el servidor no se caiga— y además responde el contrato
// de error del proyecto. Se conserva lo único no obvio que hacía gin: cuando el panic viene de
// una conexión que el cliente cortó, no se registra ni se intenta responder. Eso no es un error
// del sistema; es alguien que cerró la pestaña, y contarlo como incidente llenaría la pantalla de
// alarmas falsas justo el día que alguien descargue un reporte grande con mala señal.

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/httpx"
)

// IncidenteRegistrado es lo que se guarda de un error interno.
//
// Lo declara el servidor —que es quien lo observa— y lo satisface el armado, igual que
// EstadoDelSistema. Así `server` no depende de pgx ni del paquete `sistema`.
type IncidenteRegistrado struct {
	Referencia string
	EmpresaID  *string
	UsuarioID  *string
	Metodo     string
	Ruta       string
	Estado     int
	Operacion  string
	Detalle    string
	Panico     bool
	Traza      string
}

// RegistroDeIncidentes es el puerto de escritura.
//
// No devuelve error a propósito: esto corre sobre una petición que YA falló, y un fallo al
// guardar no puede convertirse en un segundo error que tape al primero. Quien implemente deja la
// falla en el log.
type RegistroDeIncidentes interface {
	Registrar(ctx context.Context, in IncidenteRegistrado)
}

// incidentes atrapa los panics y registra todo lo que termine en 5xx.
//
// `reg` puede ser nil: sin registro inyectado el middleware sigue atrapando panics y respondiendo
// el contrato de error. Se pierde la persistencia, no la respuesta.
func incidentes(log *zap.Logger, reg RegistroDeIncidentes) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				manejarPanico(c, log, reg, r)
				return
			}
			// Un 5xx que ya respondió: la referencia la dejó AbortInterno. Si no hay ninguna, el
			// 500 salió por una de las pocas salidas que no pasan por ahí (y se registra igual,
			// con una referencia nueva: un error sin código es justo el que no se puede rastrear).
			if c.Writer.Status() >= http.StatusInternalServerError {
				ref := httpx.DelContexto(c, httpx.ClaveRef)
				if ref == "" {
					ref = httpx.NuevaReferencia()
				}
				registrar(c, reg, IncidenteRegistrado{
					Referencia: ref,
					Metodo:     c.Request.Method,
					Ruta:       rutaDe(c),
					Estado:     c.Writer.Status(),
					Operacion:  httpx.DelContexto(c, httpx.ClaveOp),
					Detalle:    httpx.DelContexto(c, httpx.ClaveDetalle),
				})
			}
		}()
		c.Next()
	}
}

// manejarPanico responde el contrato de error y deja el incidente con su traza.
func manejarPanico(c *gin.Context, log *zap.Logger, reg RegistroDeIncidentes, r any) {
	traza := string(debug.Stack())

	// La conexión que el cliente cortó no es un error del sistema. Se aborta y se sale sin
	// registrar: no hay a quién responderle y no hay nada que investigar.
	if esConexionRota(r) {
		if log != nil {
			log.Warn("conexión interrumpida por el cliente", zap.String("ruta", rutaDe(c)))
		}
		c.Abort()
		return
	}

	ref := httpx.NuevaReferencia()
	if log != nil {
		log.Error("panic",
			zap.String("ref", ref),
			zap.Any("causa", r),
			zap.String("metodo", c.Request.Method),
			zap.String("ruta", rutaDe(c)),
			zap.String("traza", traza),
		)
	}
	registrar(c, reg, IncidenteRegistrado{
		Referencia: ref,
		Metodo:     c.Request.Method,
		Ruta:       rutaDe(c),
		// 500 y no `c.Writer.Status()`: un panic a mitad del handler deja el status en lo que
		// hubiera puesto antes —a menudo 200— y registrar un incidente como «200» lo escondería
		// de cualquier filtro por error.
		Estado:    http.StatusInternalServerError,
		Operacion: "panic",
		Detalle:   textoDeLaCausa(r),
		Panico:    true,
		Traza:     traza,
	})

	// Si ya se escribió la respuesta no se puede cambiar el estado ni el cuerpo: lo único honesto
	// es cortar. El incidente ya quedó guardado, que es lo que permite investigarlo después.
	if c.Writer.Written() {
		c.Abort()
		return
	}
	httpx.Abort(c, http.StatusInternalServerError, httpx.CodeErrorInterno,
		"Error interno del sistema. Pasá esta referencia a soporte para encontrar la causa exacta: "+ref)
}

// registrar completa los datos del token y escribe. El token puede no existir —un 500 en el login
// ocurre antes de que haya empresa— y eso es válido: la columna admite nulo.
func registrar(c *gin.Context, reg RegistroDeIncidentes, in IncidenteRegistrado) {
	if reg == nil {
		return
	}
	if cl, ok := auth.ClaimsFromContext(c); ok {
		if cl.EmpresaID != "" {
			empresa := cl.EmpresaID
			in.EmpresaID = &empresa
		}
		if u := cl.UsuarioID(); u != "" {
			usuario := u
			in.UsuarioID = &usuario
		}
	}
	// El contexto de la petición ya puede estar cancelado (el cliente se fue, o el handler
	// canceló): escribir con él perdería el registro justo en los casos raros. Se usa uno limpio.
	reg.Registrar(context.WithoutCancel(c.Request.Context()), in)
}

// rutaDe prefiere el patrón de la ruta («/v1/cxp/documentos/:id») sobre la URL real, que lleva
// identificadores. Si no hay patrón —una 404, una ruta no registrada— cae a la URL.
func rutaDe(c *gin.Context) string {
	if p := c.FullPath(); p != "" {
		return p
	}
	return c.Request.URL.Path
}

func textoDeLaCausa(r any) string {
	if err, ok := r.(error); ok {
		return err.Error()
	}
	if s, ok := r.(string); ok {
		return s
	}
	return "panic sin mensaje"
}

// esConexionRota reconoce el panic que viene de escribirle a un cliente que ya se fue.
//
// Es el mismo criterio que usa gin.Recovery: un error de red del sistema operativo cuyo texto
// dice que la tubería se rompió o que el par reinició la conexión.
func esConexionRota(r any) bool {
	ne, ok := r.(*net.OpError)
	if !ok {
		return false
	}
	var se *os.SyscallError
	if !errors.As(ne.Err, &se) {
		return false
	}
	msg := strings.ToLower(se.Error())
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "forcibly closed")
}
