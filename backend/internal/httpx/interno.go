package httpx

// La referencia de incidente: que «error interno» deje de ser un callejón sin salida.
//
// EL PROBLEMA, en palabras del Director Financiero (12-oct-2026): «cuando dice error, debería
// detallar qué es el error para no suponer, o al menos un código».
//
// Tenía razón y el caso que lo destapó lo demuestra: al guardar un proveedor salía «error
// interno» y nada más. La causa real —«expected 17 arguments, got 16», un argumento que faltaba
// en el UPDATE— estaba en el log del servidor, al que el usuario no tiene acceso. Con cientos de
// líneas de log por minuto, encontrar CUÁL de todas corresponde al clic que acaba de fallar es
// adivinar.
//
// POR QUÉ NO SE MUESTRA EL ERROR CRUDO. Un error interno trae nombres de tablas y columnas,
// fragmentos de SQL y a veces datos de la fila. Eso no va a la pantalla: describe la base por
// dentro y es exactamente lo que sirve para atacarla. Los errores que SÍ se pueden explicar ya
// tienen su mensaje propio (422, 409, 404…); lo que cae acá es lo que nadie previó.
//
// LA SALIDA: una REFERENCIA corta. Se genera una por incidente, viaja al usuario en el mensaje y
// queda escrita en el log junto al error real. El usuario dice «me dio error, ref 3F8A2C» y esa
// referencia aparece una sola vez en el log, con la causa al lado. Deja de suponerse.

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NuevaReferencia devuelve un identificador corto de incidente, en mayúsculas.
//
// Seis hex (16,7 millones de combinaciones) alcanzan de sobra: no es un identificador único
// global, es algo que una persona dicta por teléfono y que se busca en el log de la última hora.
// Doce caracteres se transcriben mal —ya pasó con los códigos de pago— y aquí el que lee el
// mensaje lo está copiando a mano.
func NuevaReferencia() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Si hasta el generador de aleatorios falla, se responde igual. Una referencia repetida
		// es un problema menor; no contestar el error es uno grande.
		return "SINREF"
	}
	s := hex.EncodeToString(b[:])
	return string([]byte{
		mayus(s[0]), mayus(s[1]), mayus(s[2]), mayus(s[3]), mayus(s[4]), mayus(s[5]),
	})
}

func mayus(c byte) byte {
	if c >= 'a' && c <= 'f' {
		return c - 'a' + 'A'
	}
	return c
}

// AbortInterno responde un 500 con una referencia y deja esa misma referencia en el log, al lado
// del error de verdad.
//
// `op` es la operación («cxp actualizar-proveedor»), que es lo que permite buscar en el log sin
// depender de la referencia cuando el usuario no la anotó.
//
// El logger puede ser nil: en ese caso se responde igual, con referencia. Un handler sin logger
// es un bug de cableado, pero no es motivo para dejar al usuario sin respuesta.
func AbortInterno(c *gin.Context, log *zap.Logger, op string, err error) {
	ref := NuevaReferencia()
	if log != nil {
		log.Error(op,
			zap.String("ref", ref),
			zap.Error(err),
			zap.String("metodo", c.Request.Method),
			zap.String("ruta", c.FullPath()),
		)
	}
	// El mensaje dice qué hacer con la referencia. «error interno · ref 3F8A2C» a secas se lee
	// como ruido técnico y se cierra sin copiarlo, que es justo lo que no puede pasar.
	Abort(c, http.StatusInternalServerError, CodeErrorInterno,
		"Error interno del sistema. Pasá esta referencia a soporte para encontrar la causa exacta: "+ref)
}
