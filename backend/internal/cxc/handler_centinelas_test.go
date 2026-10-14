package cxc

// Todo error de dominio de CxC tiene que tener su traducción HTTP.
//
// Es la misma prueba que ya vigila CxP, traída acá el 13 de octubre de 2026 después de que un
// «error interno» en el importador de cobros obligara a salir a buscar la causa a mano. CxC tiene
// 51 centinelas declarados y ninguna prueba que exigiera traducirlos: cualquiera sin su `case`
// sale como 500, el mensaje redactado con cuidado no llega a la pantalla, y desde el otro lado un
// dato mal escrito se ve igual que un servidor caído.
//
// NO LLEVA UNA LISTA A MANO. Lee el fuente del paquete, saca cada centinela declarado y comprueba
// que `handler.go` lo nombre. Un centinela nuevo la pone roja sola.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Captura los errores declaradas en las dos formas del paquete: dentro de un bloque `var ( … )`
// y sueltos con `var ErrX = errors.New(`.
var reCentinelaCxc = regexp.MustCompile(`(?m)^\s*(?:var\s+)?(Err[A-Za-z0-9]+)\s*=\s*errors\.New\(`)

// exentosCxc son los centinelas que NO viajan por HTTP, con la razón al lado. La lista es corta a
// propósito: si crece, es señal de que algo se está escondiendo detrás de un 500.
var exentosCxc = map[string]string{}

func TestTodoCentinelaDeCxcTieneTraduccionHTTP(t *testing.T) {
	entradas, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("leer el paquete: %v", err)
	}
	handler, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("leer handler.go: %v", err)
	}
	textoHandler := string(handler)

	revisados := 0
	for _, e := range entradas {
		nombre := e.Name()
		if e.IsDir() || !strings.HasSuffix(nombre, ".go") || strings.HasSuffix(nombre, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", nombre))
		if err != nil {
			t.Fatalf("leer %s: %v", nombre, err)
		}
		for _, m := range reCentinelaCxc.FindAllStringSubmatch(string(src), -1) {
			centinela := m[1]
			if razon, ok := exentosCxc[centinela]; ok {
				t.Logf("%s exento: %s", centinela, razon)
				continue
			}
			revisados++
			if !strings.Contains(textoHandler, centinela) {
				t.Errorf("%s (declarado en %s) no está en el switch de responderError: va a salir "+
					"como 500 «error interno» y su mensaje nunca llega a la pantalla",
					centinela, nombre)
			}
		}
	}
	if revisados == 0 {
		t.Fatal("no se encontró ningún centinela: la expresión dejó de calzar y la prueba no vigila nada")
	}
	t.Logf("%d centinelas revisados", revisados)
}
