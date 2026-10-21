package precios

// Todo error de dominio de Precios tiene que tener su traducción HTTP.
//
// Es la misma prueba que ya vigila CxP y CxC. Existe porque un centinela sin su `case` sale como
// 500 «error interno»: el mensaje redactado con cuidado no llega a la pantalla, y desde el otro
// lado un dato mal escrito se ve igual que un servidor caído. En CxC atrapó dos casos reales el
// mismo día en que se escribió.
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

var reCentinelaPrecios = regexp.MustCompile(`(?m)^\s*(?:var\s+)?(Err[A-Za-z0-9]+)\s*=\s*errors\.New\(`)

// exentosPrecios son los centinelas que NO viajan por HTTP, con la razón al lado. Vacío a
// propósito: si crece, es señal de que algo se está escondiendo detrás de un 500.
var exentosPrecios = map[string]string{}

func TestTodoCentinelaDePreciosTieneTraduccionHTTP(t *testing.T) {
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
		for _, m := range reCentinelaPrecios.FindAllStringSubmatch(string(src), -1) {
			centinela := m[1]
			if razon, ok := exentosPrecios[centinela]; ok {
				t.Logf("%s exento: %s", centinela, razon)
				continue
			}
			revisados++
			if !strings.Contains(textoHandler, centinela) {
				t.Errorf("%s (declarado en %s) no está en el switch de responder: va a salir "+
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

// El router tiene que exigir permisos que el catálogo RBAC liste. Un permiso que el router exige
// y el catálogo no declara queda sin casilla en la matriz, y entonces la función está apagada
// para todo rol salvo ADMIN, que saltea la matriz. Ya pasó con cinco permisos en este proyecto.
func TestLosPermisosDePreciosEstanEnElCatalogo(t *testing.T) {
	catalogo, err := os.ReadFile(filepath.Join("..", "rbac", "catalogo.go"))
	if err != nil {
		t.Fatalf("leer el catálogo RBAC: %v", err)
	}
	router, err := os.ReadFile(filepath.Join("..", "server", "router.go"))
	if err != nil {
		t.Fatalf("leer el router: %v", err)
	}

	exigidos := regexp.MustCompile(`P\("(precios\.[a-z_]+)"\)`).FindAllStringSubmatch(string(router), -1)
	if len(exigidos) == 0 {
		t.Fatal("el router no exige ningún permiso de precios: la expresión dejó de calzar")
	}
	vistos := map[string]bool{}
	for _, m := range exigidos {
		p := m[1]
		if vistos[p] {
			continue
		}
		vistos[p] = true
		if !strings.Contains(string(catalogo), `"`+p+`"`) {
			t.Errorf("el router exige %q pero el catálogo RBAC no lo declara: la función queda "+
				"apagada para todo rol salvo ADMIN", p)
		}
	}
	t.Logf("%d permisos de precios verificados", len(vistos))
}
