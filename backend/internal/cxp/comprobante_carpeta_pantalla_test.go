package cxp

// Que la pantalla sepa nombrar TODOS los veredictos que este paquete emite.
//
// El modo de falla no es cosmético: en el cliente, `VEREDICTO[estado]` devuelve `undefined` para un
// estado que falte y la fila revienta al leer `.tono`. El operador acaba de soltar cien archivos y
// se queda con una pantalla en blanco.
//
// La prueba vive del lado de Go porque es el único que ve los dos árboles: el contenedor del
// frontend monta solo `/app` y no tiene el fuente del backend. Mismo molde que
// catalogo_router_test.go, que lee el router para exigir que todo permiso pedido tenga su check —
// y que encontró cinco permisos que dejaban una función apagada para todo rol salvo ADMIN.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const pantallaComprobantes = "../../../frontend/src/features/cxp/pages/ComprobantesPage.tsx"

// reVeredictoTSX saca las claves del mapa VEREDICTO: líneas «  UNICO: {» con dos espacios de
// sangría, que es el nivel de las claves del objeto.
var reVeredictoTSX = regexp.MustCompile(`(?m)^  ([A-Z][A-Z_]+): \{`)

// estadosDeLaPantalla lee el mapa de la pantalla. Devuelve nil si el archivo no está, para que el
// backend siga siendo compilable y probable sin el frontend al lado.
func estadosDeLaPantalla(t *testing.T) []string {
	t.Helper()
	ruta := filepath.FromSlash(pantallaComprobantes)
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Skipf("sin el fuente de la pantalla (%v)", err)
	}
	src := string(b)
	i := strings.Index(src, "export const VEREDICTO")
	if i < 0 {
		t.Fatalf("no se encontró el mapa VEREDICTO en %s; si se renombró, actualizá esta prueba "+
			"en vez de borrarla: es lo que evita que un estado nuevo deje la pantalla en blanco", ruta)
	}
	fin := strings.Index(src[i:], "\n};")
	if fin < 0 {
		t.Fatalf("el mapa VEREDICTO no cierra en %s", ruta)
	}
	out := make([]string, 0, 8)
	for _, m := range reVeredictoTSX.FindAllStringSubmatch(src[i:i+fin], -1) {
		out = append(out, m[1])
	}
	return out
}

// estadosDeArchivo son los que este paquete puede devolver. Escritos a mano acá y comparados contra
// el uso real abajo: una lista a mano que nadie contrasta es justamente el defecto que se persigue.
var estadosDeArchivo = []string{
	ArchivoUnico, ArchivoRemesa, ArchivoSinCodigo, ArchivoVariosCodigos,
	ArchivoDuplicado, ArchivoNoEncontrado, ArchivoNoPagada,
}

// La lista de arriba tiene que cubrir todas las constantes `Archivo…` declaradas en el paquete. Sin
// esto, agregar una constante y olvidarse de sumarla dejaría la comparación con la pantalla
// midiendo de menos y la prueba pasaría sin probar nada.
func TestLaListaDeEstadosEstaCompleta(t *testing.T) {
	b, err := os.ReadFile("comprobante_carpeta.go")
	if err != nil {
		t.Fatalf("leer el fuente: %v", err)
	}
	re := regexp.MustCompile(`(?m)^\tArchivo[A-Za-z]+\s*=\s*"([A-Z_]+)"`)
	declarados := make([]string, 0, 8)
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		declarados = append(declarados, m[1])
	}
	if len(declarados) == 0 {
		t.Fatal("no se encontró ninguna constante Archivo… en el fuente")
	}
	tengo := make(map[string]bool, len(estadosDeArchivo))
	for _, e := range estadosDeArchivo {
		tengo[e] = true
	}
	for _, d := range declarados {
		if !tengo[d] {
			t.Errorf("la constante %q está declarada y no entró en estadosDeArchivo: la "+
				"comparación con la pantalla la ignoraría", d)
		}
	}
	if len(declarados) != len(estadosDeArchivo) {
		t.Errorf("el fuente declara %d estados y la lista tiene %d", len(declarados), len(estadosDeArchivo))
	}
}

func TestLaPantallaSabeNombrarTodosLosVeredictos(t *testing.T) {
	enPantalla := estadosDeLaPantalla(t)
	if len(enPantalla) == 0 {
		t.Fatal("la pantalla no declara ningún veredicto")
	}
	tiene := make(map[string]bool, len(enPantalla))
	for _, e := range enPantalla {
		tiene[e] = true
	}
	for _, e := range estadosDeArchivo {
		if !tiene[e] {
			t.Errorf("el servidor puede devolver %q y la pantalla no sabe nombrarlo: la fila "+
				"reventaría al leer el tono, justo después de soltar cien archivos", e)
		}
	}
	// Y al revés: una explicación de un estado que ya no se emite es texto muerto que confunde al
	// que mantiene la pantalla.
	emite := make(map[string]bool, len(estadosDeArchivo))
	for _, e := range estadosDeArchivo {
		emite[e] = true
	}
	for _, e := range enPantalla {
		if !emite[e] {
			t.Errorf("la pantalla explica %q, que el servidor ya no devuelve", e)
		}
	}
	sort.Strings(enPantalla)
	t.Logf("veredictos en la pantalla: %s", strings.Join(enPantalla, ", "))
}
