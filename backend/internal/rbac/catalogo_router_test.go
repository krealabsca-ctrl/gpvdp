package rbac

// Todo permiso que el ROUTER exige tiene que estar en el catálogo (5-oct-2026).
//
// El Director: «hay algunos permisos que parece que están a nivel de código pero a nivel de check
// no está completo». Tenía razón y eran cinco: los de `cxp.responsabilidades.*`. La migración los
// sembró en la tabla `permiso`, el router los exige en nueve rutas, y nunca se escribieron en
// `Catalogo`. Como la matriz de Seguridad se dibuja desde ESTE catálogo y no desde la tabla, no
// había ningún check que marcar: el calendario de responsabilidades quedaba inalcanzable para todo
// rol que no fuera ADMIN, que saltea la matriz y por eso nadie lo notó al probarlo.
//
// Las pruebas que ya existían comparaban el catálogo CONSIGO MISMO y con la matriz por defecto, así
// que ninguna podía ver lo que faltaba. Esta lee el router —el fuente, igual que la de centinelas de
// CxP— porque la única fuente de verdad de «qué permisos existen de verdad» son las rutas que los
// piden. Un permiso que el router pide y el catálogo no nombra es una función construida y apagada.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Las dos formas de exigir permiso en el router: P("x") para uno, PAlguno("x", "y") para
// «cualquiera de estos». PAlguno se escribe en una sola llamada, así que basta con sacarle todos
// los literales de adentro.
var (
	reP       = regexp.MustCompile(`\bP\("([a-z_]+\.[a-z_.]+)"\)`)
	rePAlguno = regexp.MustCompile(`\bPAlguno\(([^)]*)\)`)
	reLiteral = regexp.MustCompile(`"([a-z_]+\.[a-z_.]+)"`)
)

// permisosDelRouter lee router.go y devuelve todos los permisos que exige.
func permisosDelRouter(t *testing.T) []string {
	t.Helper()
	ruta := filepath.Join("..", "server", "router.go")
	src, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leer %s: %v", ruta, err)
	}
	texto := string(src)

	set := map[string]bool{}
	for _, m := range reP.FindAllStringSubmatch(texto, -1) {
		set[m[1]] = true
	}
	for _, m := range rePAlguno.FindAllStringSubmatch(texto, -1) {
		for _, l := range reLiteral.FindAllStringSubmatch(m[1], -1) {
			set[l[1]] = true
		}
	}

	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func TestTodoPermisoDelRouterEstaEnElCatalogo(t *testing.T) {
	exigidos := permisosDelRouter(t)
	// Si el regex deja de calzar (alguien renombra el helper), la prueba pasaría sin revisar nada.
	if len(exigidos) < 50 {
		t.Fatalf("solo se leyeron %d permisos del router: el lector dejó de funcionar y la prueba no está midiendo nada", len(exigidos))
	}

	enCatalogo := map[string]bool{}
	for _, p := range Catalogo {
		enCatalogo[p.Codigo] = true
	}

	var faltan []string
	for _, p := range exigidos {
		if !enCatalogo[p] {
			faltan = append(faltan, p)
		}
	}
	if len(faltan) > 0 {
		t.Errorf("el router exige %d permiso(s) que el catálogo no nombra, así que no hay check que marcar y la función queda apagada para todo rol que no sea ADMIN:\n  %s",
			len(faltan), strings.Join(faltan, "\n  "))
	}
}

// El otro lado: un permiso del catálogo que nadie exige es un check que no hace nada. No es un
// error —puede ser una ruta por construir— pero el catálogo es lo que el Director lee para repartir
// accesos, y un check que no abre nada le hace creer que dio un acceso que no dio.
func TestTodoPermisoDelCatalogoLoExigeAlguienOEstaJustificado(t *testing.T) {
	exigidos := map[string]bool{}
	for _, p := range permisosDelRouter(t) {
		exigidos[p] = true
	}

	// Permisos que NO se exigen en una ruta y por qué es correcto. Cada excepción se nombra: si
	// mañana sobra otro, hay que decidirlo a conciencia y no dejarlo pasar.
	//
	// Los cinco son de los dos tipos que legítimamente no viven en el router: ALCANCE DE DATOS (la
	// ruta se abre igual, lo que cambia es cuántas filas se ven) y permiso POR ACCIÓN dentro de un
	// endpoint que sirve varias (el masivo de la bandeja).
	exentos := map[string]string{
		"cxp.ver_todo":        "alcance: sin él solo se ven las facturas propias (cxp/service.go)",
		"cxp.cartera_abierta": "alcance: abre la pestaña de cartera viva y su panel del tablero (cxp/service.go)",
		"cxp.anular":          "por acción: lo exige el endpoint masivo para Denegar y Anular (cxp/masivo.go)",
		"cxp.resultado_pago":  "por acción: lo exige el endpoint masivo para Rebotar y Reintentar (cxp/masivo.go)",
		"cxc.ver_todas_sedes": "alcance: sin él solo se ve la cartera de la(s) sede(s) asignada(s) (cxc/cxc.go)",
	}

	var sueltos []string
	for _, p := range Catalogo {
		if !exigidos[p.Codigo] && exentos[p.Codigo] == "" {
			sueltos = append(sueltos, p.Codigo)
		}
	}
	if len(sueltos) > 0 {
		t.Errorf("el catálogo ofrece %d check(s) que ninguna ruta pide: marcarlos no abre nada.\n  %s\nSi alguno es legítimo (se usa fuera del router), agregalo a `exentos` con el motivo.",
			len(sueltos), strings.Join(sueltos, "\n  "))
	}
}
