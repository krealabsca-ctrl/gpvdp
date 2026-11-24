package conta

// Todo error de dominio de este paquete tiene que tener su traducción HTTP.
//
// Es el mismo centinela que vigila CxP y el módulo de sistema. Acá el costo de olvidarse es que
// quien está cargando un plan de 258 cuentas, o mapeando 332 partidas, recibe «error interno» en
// vez de la frase que le dice qué falta —y no tiene ninguna otra forma de averiguarlo—.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var reCentinelaConta = regexp.MustCompile(`(?m)^\s*(?:var\s+)?(Err[A-Za-z0-9]+)\s*=\s*errors\.New\(`)

func TestTodoCentinelaDeContaTieneTraduccionHTTP(t *testing.T) {
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
		for _, m := range reCentinelaConta.FindAllStringSubmatch(string(src), -1) {
			encontrados++
			if !strings.Contains(textoHandler, m[1]) {
				t.Errorf("%s (declarado en %s) no está en el switch de responder: va a salir como "+
					"500 «error interno» y quien lo reciba no va a saber qué corregir", m[1], nombre)
			}
		}
	}

	// Control: si el patrón dejara de calzar, la prueba pasaría sin mirar nada.
	if encontrados < 10 {
		t.Fatalf("solo se encontraron %d centinelas; el patrón dejó de reconocerlos y esta prueba "+
			"estaría pasando en falso", encontrados)
	}
}

// Las rutas del mapeo existen y van con su permiso. El router es un archivo de 700 líneas: una
// ruta que se queda sin registrar deja la pantalla entera devolviendo 404, y una sin `P(...)` la
// deja abierta a cualquiera que tenga sesión.
func TestLasRutasDelMapeoEstanRegistradasConSuPermiso(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "server", "router.go"))
	if err != nil {
		t.Fatalf("leer el router: %v", err)
	}
	texto := string(src)
	for _, ruta := range []string{
		`scoped.GET("/conta/mapeo", P("conta.mapeo"), contaH.Mapeo)`,
		`scoped.POST("/conta/mapeo", P("conta.mapeo"), contaH.Decidir)`,
		`scoped.DELETE("/conta/mapeo/:id", P("conta.mapeo"), contaH.Deshacer)`,
	} {
		if !strings.Contains(texto, ruta) {
			t.Errorf("falta en el router, o perdió su permiso: %s", ruta)
		}
	}
}
