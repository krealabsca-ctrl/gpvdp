package seed

import (
	"os"
	"strings"
	"testing"
)

// codigoDeSeedCatalogo devuelve el cuerpo de `seedCatalogo` leyendo el archivo fuente.
//
// Leer el propio código es inusual, pero acá es lo correcto: la propiedad que hay que fijar no es
// un valor de retorno, es que la función NO ESCRIBA cuando la empresa ya tiene catálogo. Probar eso
// de verdad pide un PostgreSQL, y este paquete no lo levanta; sin esta red, la guarda se puede
// borrar en un refactor y nadie se enteraría hasta que el catálogo del usuario vuelva a revivir.
func codigoDeSeedCatalogo(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("seed.go")
	if err != nil {
		t.Fatalf("leer seed.go: %v", err)
	}
	src := string(b)
	i := strings.Index(src, "func seedCatalogo(")
	if i < 0 {
		t.Fatal("no se encontró seedCatalogo en seed.go")
	}
	resto := src[i:]
	if j := strings.Index(resto, "\nfunc "); j > 0 {
		resto = resto[:j]
	}
	return resto
}

func contiene(s, sub string) bool { return strings.Contains(s, sub) }
