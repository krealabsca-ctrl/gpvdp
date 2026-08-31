package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gpvdp/erp/internal/httpx"
)

// rateLimiter es un limitador por clave (IP) con ventana deslizante simple, en memoria.
// Suficiente para una sola instancia del backend detrás de Caddy; si algún día se escala a
// varias réplicas, el conteo debería moverse a un store compartido (Redis).
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]int64 // clave -> timestamps (unix nanos) dentro de la ventana
	max    int
	window time.Duration
	now    func() time.Time // inyectable para tests
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		hits:   make(map[string][]int64),
		max:    max,
		window: window,
		now:    time.Now,
	}
	return rl
}

// permitir registra un golpe de `clave` y devuelve si está dentro del límite, junto con los
// segundos que faltan para que se libere un cupo (Retry-After) cuando se excede.
func (rl *rateLimiter) permitir(clave string) (ok bool, retryAfter int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	ahora := rl.now()
	limite := ahora.Add(-rl.window).UnixNano()

	// Descarta los golpes fuera de la ventana.
	prev := rl.hits[clave]
	vivos := prev[:0]
	for _, t := range prev {
		if t > limite {
			vivos = append(vivos, t)
		}
	}

	if len(vivos) >= rl.max {
		// Cuándo expira el más antiguo ⇒ cuándo se libera un cupo.
		libera := time.Unix(0, vivos[0]).Add(rl.window).Sub(ahora)
		rl.hits[clave] = vivos
		secs := int(libera.Seconds())
		if secs < 1 {
			secs = 1
		}
		return false, secs
	}

	rl.hits[clave] = append(vivos, ahora.UnixNano())
	return true, 0
}

// limpiar borra las claves sin golpes vigentes (evita que el mapa crezca sin fin).
func (rl *rateLimiter) limpiar() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	limite := rl.now().Add(-rl.window).UnixNano()
	for k, ts := range rl.hits {
		vivo := false
		for _, t := range ts {
			if t > limite {
				vivo = true
				break
			}
		}
		if !vivo {
			delete(rl.hits, k)
		}
	}
}

// rateLimit devuelve un middleware que limita por IP de cliente. Al exceder responde 429 con
// Retry-After. Se usa en los endpoints públicos de autenticación (login/refresh) para frenar
// la fuerza bruta y el credential stuffing.
func rateLimit(max int, window time.Duration) gin.HandlerFunc {
	rl := newRateLimiter(max, window)

	// Limpieza periódica en segundo plano (barata; una goroutine por limitador).
	go func() {
		t := time.NewTicker(window)
		defer t.Stop()
		for range t.C {
			rl.limpiar()
		}
	}()

	return func(c *gin.Context) {
		ok, retryAfter := rl.permitir(c.ClientIP())
		if !ok {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			httpx.Abort(c, http.StatusTooManyRequests, httpx.CodeDemasiadosIntentos,
				"demasiados intentos, probá de nuevo en unos minutos")
			return
		}
		c.Next()
	}
}
