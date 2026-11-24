package sistema

// Todo error de dominio de este paquete tiene que tener su traducción HTTP.
//
// Es el mismo centinela que vigila CxP, y acá importa el doble: este paquete ES la pantalla de
// errores del sistema. Un error suyo que salga como 500 «error interno» se registra a sí mismo
// como incidente, y la pantalla que existe para explicar los errores empieza a producirlos.
//
// Ya pasó una vez, el mismo día que se construyó: `?desde=hola` reventaba en Postgres y salía
// como 500 en vez de 400. Se descubrió provocándolo a propósito contra el sistema corriendo.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// reCentinelaSistema captura los errores declarados con `var ErrX = errors.New(`, sueltos o
// dentro de un bloque `var ( … )`.
var reCentinelaSistema = regexp.MustCompile(`(?m)^\s*(?:var\s+)?(Err[A-Za-z0-9]+)\s*=\s*errors\.New\(`)

func TestTodoCentinelaDelSistemaTieneTraduccionHTTP(t *testing.T) {
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
		for _, m := range reCentinelaSistema.FindAllStringSubmatch(string(src), -1) {
			encontrados++
			if !strings.Contains(textoHandler, m[1]) {
				t.Errorf("%s (declarado en %s) no está en el switch de responder: va a salir como "+
					"500 y se va a registrar como un incidente del propio sistema", m[1], nombre)
			}
		}
	}

	// Control: si el patrón dejara de calzar, la prueba pasaría sin mirar nada.
	if encontrados < 2 {
		t.Fatalf("solo se encontraron %d centinelas; el patrón dejó de reconocerlos y esta prueba "+
			"estaría pasando en falso", encontrados)
	}
}
