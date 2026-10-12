package cxp

// Todo error de dominio tiene que tener su traducción HTTP.
//
// El 1 de octubre de 2026 agregué dos centinelas nuevos (`ErrCodigoPagoInvalido` y
// `ErrCodigoPagoDuplicado`), escribí sus mensajes con cuidado —«tiene que ser de 1 a 4 letras o
// números, por ejemplo A8»— y olvidé agregarlos al switch de `responderError`. Los dos salieron
// como 500 «error interno»: el mensaje redactado no llegó a la pantalla, y desde el otro lado un
// código mal escrito se ve igual que un servidor caído. Toda la suite estaba verde.
//
// Es un modo de falla conocido del proyecto y se repite porque la lista a mantener está en un
// archivo distinto del que declara los errores. Por eso esta prueba NO lleva una lista: LEE EL
// FUENTE del paquete, saca cada centinela declarado y comprueba que `handler.go` lo nombre. Un
// centinela nuevo la pone roja sola, sin que nadie se acuerde de venir a agregarlo acá.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// reCentinela captura el nombre de los errores de dominio, en las DOS formas en que el paquete los
// declara: dentro de un bloque `var ( … )` y sueltos con `var ErrX = errors.New(`.
//
// El `(?:var\s+)?` no es un detalle: la primera versión de esta prueba lo omitía y por eso no veía
// los dos centinelas sueltos que motivaron escribirla. Pasaba en verde mientras el defecto seguía
// ahí — una prueba que no falla cuando rompés lo que vigila es peor que no tenerla, porque además
// da confianza. Se descubrió falsificándola a propósito.
var reCentinela = regexp.MustCompile(`(?m)^\s*(?:var\s+)?(Err[A-Za-z0-9]+)\s*=\s*errors\.New\(`)

// exentos son los centinelas que NO viajan por HTTP, con la razón. Cualquier otro tiene que estar
// en el switch. La lista es corta a propósito: si crece, es señal de que algo se está escondiendo.
var exentos = map[string]string{
	// Se traduce en el handler del lote con su propio 422 y la lista de facturas, no por el switch.
	"ErrSinDocumentos": "lo responde CrearLote con el detalle factura por factura",
}

func TestTodoCentinelaTieneTraduccionHTTP(t *testing.T) {
	entradas, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("leer el paquete: %v", err)
	}

	handler, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("leer handler.go: %v", err)
	}
	textoHandler := string(handler)

	encontrados := 0
	for _, e := range entradas {
		nombre := e.Name()
		if e.IsDir() || !strings.HasSuffix(nombre, ".go") || strings.HasSuffix(nombre, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", nombre))
		if err != nil {
			t.Fatalf("leer %s: %v", nombre, err)
		}
		for _, m := range reCentinela.FindAllStringSubmatch(string(src), -1) {
			centinela := m[1]
			encontrados++
			if _, ok := exentos[centinela]; ok {
				continue
			}
			if !strings.Contains(textoHandler, centinela) {
				t.Errorf("%s (declarado en %s) no está en el switch de responderError: "+
					"va a salir como 500 «error interno» y su mensaje nunca llega a la pantalla",
					centinela, nombre)
			}
		}
	}

	// Control: si el regex dejara de calzar, la prueba pasaría sin mirar nada.
	if encontrados < 10 {
		t.Fatalf("solo se encontraron %d centinelas en el paquete; el patrón dejó de reconocerlos "+
			"y esta prueba estaría pasando en falso", encontrados)
	}
	t.Logf("%d centinelas revisados", encontrados)
}
