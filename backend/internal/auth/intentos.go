package auth

// Límite de intentos de ingreso (21-oct-2026, punto 5 de la revisión comercial).
//
// Hasta hoy el login no tenía ninguna barrera: un script podía probar contraseñas tan rápido como
// el servidor contestara. Mientras el sistema vivió dentro de una oficina eso era un detalle; con
// el ERP en internet —que es lo que habilita el instalador de VPS— es lo primero que pregunta
// cualquier revisión de seguridad.
//
// ── POR QUÉ NO SE BLOQUEA LA CUENTA ─────────────────────────────────────────────────────────
//
// La forma obvia —«cinco fallos y se bloquea el usuario»— convierte el control en un ataque: a
// cualquiera le basta escribir mal la contraseña de la tesorera cinco veces para dejarla afuera
// el día del pago de planilla. El control contra la fuerza bruta se vuelve una herramienta de
// denegación de servicio contra la gente que trabaja.
//
// Por eso lo que se cuenta es la pareja (CORREO, ORIGEN), no el correo solo. Quien ataca desde una
// IP se queda sin esa IP; la persona de verdad, que entra desde la oficina o desde su casa, no se
// entera. Y como además se cuenta el ORIGEN solo, un atacante no puede rotar correos para seguir
// probando desde el mismo lugar.
//
// ── POR QUÉ EN MEMORIA Y NO EN LA BASE ──────────────────────────────────────────────────────
//
// Un contador en la base escribiría una fila en cada intento fallido —exactamente lo que busca
// quien ataca— y convertiría el ataque de contraseñas en un ataque de escrituras. En memoria el
// costo por intento es nulo.
//
// Lo que se pierde es que los contadores se van al reiniciar. No importa: quien ataca no puede
// reiniciar el servidor, y el administrador que lo reinicia está justamente arreglando algo.
//
// ── LO QUE ESTO NO ES ───────────────────────────────────────────────────────────────────────
//
// No reemplaza contraseñas fuertes ni la bitácora. Hace que probar contraseñas a ciegas deje de
// ser gratis, que es un piso, no un techo.

import (
	"sync"
	"time"
)

// Los tres números que gobiernan el control. Son defaults de seguridad, no reglas del negocio, y
// están acá juntos para que cambiarlos sea una decisión y no una búsqueda.
const (
	// VentanaIntentos es cuánto se recuerda un fallo. Quince minutos es suficiente para frenar un
	// script y lo bastante corto para que una persona que se equivocó pueda reintentar sin llamar
	// a nadie.
	VentanaIntentos = 15 * time.Minute

	// MaxPorCorreoYOrigen: fallos de UNA cuenta desde UN origen. Cinco es lo que alguien se
	// equivoca de verdad; el sexto ya es otra cosa.
	MaxPorCorreoYOrigen = 5

	// MaxPorOrigen: fallos de CUALQUIER cuenta desde un mismo origen. Más alto a propósito,
	// porque detrás de una sola IP puede haber una oficina entera: el día que se cae el proveedor
	// de correo, varias personas se equivocan a la vez y ninguna debería quedar afuera.
	MaxPorOrigen = 30
)

// Intentos cuenta fallos de ingreso por clave y por ventana.
//
// Es una ventana deslizante simple: se guardan las marcas de tiempo de los fallos vivos y se
// descartan al leer. Con un puñado de claves activas eso es más barato —y mucho más fácil de
// razonar— que cualquier estructura con expiración propia.
type Intentos struct {
	mu     sync.Mutex
	fallos map[string][]time.Time
	// ahora se inyecta para poder probar el paso del tiempo sin dormir: un test que espera quince
	// minutos no es un test, es una pausa.
	ahora func() time.Time
}

// NuevoIntentos crea el contador.
func NuevoIntentos() *Intentos {
	return &Intentos{fallos: map[string][]time.Time{}, ahora: time.Now}
}

// vivos devuelve los fallos de la clave dentro de la ventana, limpiando los viejos.
//
// La limpieza ocurre al LEER y no con una rutina aparte: sin tráfico no hay nada que limpiar, y
// una goroutine barriendo un mapa vacío es trabajo que nadie pidió.
func (i *Intentos) vivos(clave string, ahora time.Time) []time.Time {
	corte := ahora.Add(-VentanaIntentos)
	marcas := i.fallos[clave]
	k := 0
	for _, t := range marcas {
		if t.After(corte) {
			marcas[k] = t
			k++
		}
	}
	marcas = marcas[:k]
	if len(marcas) == 0 {
		delete(i.fallos, clave)
	} else {
		i.fallos[clave] = marcas
	}
	return marcas
}

// Bloqueado dice si esa pareja (correo, origen) ya no puede intentar, y cuánto falta.
//
// Devuelve la espera para poder decirla: «probá de nuevo» sin un cuándo obliga a reintentar a
// ciegas, que es la forma de que alguien crea que el sistema se rompió.
func (i *Intentos) Bloqueado(correo, origen string) (bool, time.Duration) {
	i.mu.Lock()
	defer i.mu.Unlock()
	ahora := i.ahora()

	if marcas := i.vivos(claveCorreoOrigen(correo, origen), ahora); len(marcas) >= MaxPorCorreoYOrigen {
		return true, VentanaIntentos - ahora.Sub(marcas[0])
	}
	if marcas := i.vivos(claveOrigen(origen), ahora); len(marcas) >= MaxPorOrigen {
		return true, VentanaIntentos - ahora.Sub(marcas[0])
	}
	return false, 0
}

// Fallo registra un intento fallido en las DOS claves.
//
// En las dos a propósito: la de la pareja frena el ataque dirigido a una cuenta, y la del origen
// frena al que rota correos para esquivar la primera.
func (i *Intentos) Fallo(correo, origen string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	ahora := i.ahora()
	for _, c := range []string{claveCorreoOrigen(correo, origen), claveOrigen(origen)} {
		i.fallos[c] = append(i.vivos(c, ahora), ahora)
	}
}

// Exito limpia el contador de esa pareja.
//
// SOLO el de la pareja, nunca el del origen: si entrar con una cuenta buena borrara el contador
// del lugar, quien tiene una credencial válida podría usarla para seguir probando otras a
// voluntad, que es justo lo que el segundo contador existe para impedir.
func (i *Intentos) Exito(correo, origen string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.fallos, claveCorreoOrigen(correo, origen))
}

func claveCorreoOrigen(correo, origen string) string { return "par\x00" + correo + "\x00" + origen }
func claveOrigen(origen string) string               { return "ip\x00" + origen }
