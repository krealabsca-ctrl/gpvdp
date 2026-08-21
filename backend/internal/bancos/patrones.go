package bancos

// Descubrimiento de patrones en los movimientos sin clasificar.
//
// El problema real (Valle de Paz, julio 2026): 7 521 de 9 463 movimientos quedaron
// NO_IDENTIFICADO, y el período no cierra hasta clasificar el 100%. Pero no son 7 521 casos
// distintos: 5 077 son el mismo hecho repetido (pagos de planes por SINPE Móvil). Nadie
// descubre eso escribiendo búsquedas a mano.
//
// Esto agrupa las descripciones por su FORMA (los números se vuelven comodines), propone la
// palabra clave que identifica cada grupo y dice cuántos movimientos clasificaría. El motor no
// cambia: la palabra propuesta se le entrega a la misma `CrearRegla` que ya retro-aplica, y
// sigue vigente el ≥90%-o-nada (la regla asigna solo con coincidencia exacta de substring).

import (
	"regexp"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// tokensDeForma es cuántos tokens definen la forma de una descripción. Cuatro alcanza para
// separar los casos reales sin fundirlos:
//
//	dr/cr linea sinpe (smo-#   → SINPE Móvil de clientes (créditos)
//	dr/cr linea sinpe (#       → SINPE saliente a proveedores (débitos)
//	debito masivo sinpe (#     → pagos masivos
//	credito en cuenta #        → depósitos a la cuenta de la empresa
//
// Con tres se fundirían los dos primeros, que son hechos opuestos.
const tokensDeForma = 4

// minimoPatron es el tamaño mínimo de grupo que vale la pena proponer como regla.
const minimoPatron = 5

// largoMinimoPatron evita proponer palabras clave tan cortas que calcen con cualquier cosa.
const largoMinimoPatron = 4

// reDigitos son las corridas de dígitos: en la forma de la descripción son comodines.
var reDigitos = regexp.MustCompile(`\d+`)

// reAnio detecta un año dentro de la palabra clave propuesta. Una regla con año deja de calzar
// cuando cambia el año, así que se avisa en vez de silenciarlo.
var reAnio = regexp.MustCompile(`(19|20)\d{2}`)

// reReferencia detecta una corrida larga de dígitos: es un consecutivo o número de referencia,
// único por movimiento. Una palabra clave que lo arrastre clasificaría lo de hoy y NUNCA más;
// antes que proponer eso, el grupo se reporta sin palabra para que se revise a mano.
var reReferencia = regexp.MustCompile(`\d{5,}`)

// LineaSinClasificar es un movimiento pendiente de clasificar, visto por el agrupador.
type LineaSinClasificar struct {
	Descripcion string
	EsDebito    bool
	Monto       decimal.Decimal
}

// PatronSugerido es un grupo de movimientos que comparten forma, con la palabra clave que los
// identifica y el alcance que tendría la regla.
type PatronSugerido struct {
	// Patron es la palabra clave propuesta: la más corta que NO calza con nada fuera del grupo.
	// Vacío = el grupo existe pero no hay palabra segura (ver Motivo); se revisa a mano.
	Patron string `json:"patron"`
	// Motivo explica por qué no hay palabra propuesta. Vacío cuando sí hay.
	Motivo string `json:"motivo"`
	// Alterna es el otro candidato (más específico o más corto), por si el usuario lo prefiere.
	Alterna string `json:"alterna"`
	// AplicaA sale del signo del grupo: CREDITO, DEBITO o MIXTO si vienen mezclados.
	AplicaA     string `json:"aplica_a"`
	Movimientos int    `json:"movimientos"`
	Creditos    int    `json:"creditos"`
	Debitos     int    `json:"debitos"`
	Monto       string `json:"monto"`
	// Ejemplos son descripciones reales del grupo, para reconocer el hecho de un vistazo.
	Ejemplos []string `json:"ejemplos"`
	// AvisoAnio: el patrón contiene un año y dejaría de calzar cuando cambie.
	AvisoAnio bool `json:"aviso_anio"`
	// Alcance es cuántos movimientos de TODA la empresa contienen el patrón, clasificados
	// incluidos. Que sea mayor que Movimientos es NORMAL y buena señal: significa que el mismo
	// hecho ya se clasificó antes a mano.
	Alcance int `json:"alcance"`
	// Ajenos son los movimientos que calzan con el patrón pero son de OTRA forma. Cero es lo
	// esperado; más que cero significa que la palabra es demasiado genérica.
	Ajenos int `json:"ajenos"`
}

// formaDescripcion reduce una descripción a su forma: minúsculas sin acentos y con cada corrida
// de dígitos vuelta comodín. Es lo que permite ver que 5 077 líneas son el mismo hecho.
func formaDescripcion(descripcion string) string {
	tokens := strings.Fields(norm(descripcion))
	if len(tokens) > tokensDeForma {
		tokens = tokens[:tokensDeForma]
	}
	for i, t := range tokens {
		tokens[i] = reDigitos.ReplaceAllString(t, "#")
	}
	return strings.Join(tokens, " ")
}

// prefijoComun devuelve el prefijo compartido por todas las cadenas (ya normalizadas).
func prefijoComun(cadenas []string) string {
	if len(cadenas) == 0 {
		return ""
	}
	pref := cadenas[0]
	for _, s := range cadenas[1:] {
		i := 0
		for i < len(pref) && i < len(s) && pref[i] == s[i] {
			i++
		}
		pref = pref[:i]
		if pref == "" {
			return ""
		}
	}
	return pref
}

// recortarAntesDeDigitos corta el prefijo justo antes del primer dígito. Un prefijo que arrastra
// «2026» funciona hoy y falla en enero; sin los dígitos la regla sobrevive al cambio de año.
func recortarAntesDeDigitos(s string) string {
	if i := strings.IndexFunc(s, func(r rune) bool { return r >= '0' && r <= '9' }); i >= 0 {
		return s[:i]
	}
	return s
}

// AgruparPatrones agrupa los movimientos sin clasificar por forma y propone una palabra clave
// por grupo. `todas` son las descripciones de TODOS los movimientos incluidos de la empresa y
// sirven para medir el alcance real de cada palabra propuesta (incluidos los ya clasificados,
// porque una palabra demasiado genérica también atraparía los movimientos que entren mañana).
//
// Función pura: no toca base de datos ni reloj.
func AgruparPatrones(sinClasificar []LineaSinClasificar, todas []string, limite int) []PatronSugerido {
	type grupo struct {
		descsNorm []string
		ejemplos  []string
		creditos  int
		debitos   int
		monto     decimal.Decimal
	}
	grupos := map[string]*grupo{}
	orden := make([]string, 0, 64)
	for _, l := range sinClasificar {
		forma := formaDescripcion(l.Descripcion)
		if forma == "" {
			continue
		}
		g, ok := grupos[forma]
		if !ok {
			g = &grupo{monto: decimal.Zero}
			grupos[forma] = g
			orden = append(orden, forma)
		}
		g.descsNorm = append(g.descsNorm, norm(l.Descripcion))
		if len(g.ejemplos) < 2 {
			g.ejemplos = append(g.ejemplos, strings.TrimSpace(l.Descripcion))
		}
		if l.EsDebito {
			g.debitos++
		} else {
			g.creditos++
		}
		g.monto = g.monto.Add(l.Monto)
	}

	todasNorm := make([]string, len(todas))
	formasTodas := make([]string, len(todas))
	for i, d := range todas {
		todasNorm[i] = norm(d)
		formasTodas[i] = formaDescripcion(d)
	}
	// alcance cuenta en cuántas descripciones de la empresa aparece la palabra clave, y cuántas
	// de esas son de OTRA forma. Lo segundo es lo que descalifica una palabra: que calce con un
	// hecho distinto. Que calce con movimientos de la MISMA forma ya clasificados es correcto —
	// es el mismo hecho, clasificado antes a mano.
	// Sin palabra no hay nada que medir (un substring vacío calzaría con todo).
	alcance := func(patron, forma string) (total, ajenos int) {
		if patron == "" {
			return 0, 0
		}
		for i, d := range todasNorm {
			if !strings.Contains(d, patron) {
				continue
			}
			total++
			if formasTodas[i] != forma {
				ajenos++
			}
		}
		return total, ajenos
	}

	out := make([]PatronSugerido, 0, len(grupos))
	for _, forma := range orden {
		g := grupos[forma]
		total := g.creditos + g.debitos
		if total < minimoPatron {
			continue
		}
		largo := strings.TrimSpace(prefijoComun(g.descsNorm))
		corto := strings.TrimSpace(recortarAntesDeDigitos(largo))

		// Se prefiere el corto (sobrevive al cambio de año) si no se mete en otras formas.
		_, ajenosCorto := alcance(corto, forma)
		_, ajenosLargo := alcance(largo, forma)
		patron, alterna, motivo := "", "", ""
		switch {
		case len([]rune(corto)) >= largoMinimoPatron && ajenosCorto == 0:
			patron, alterna = corto, largo
		case reReferencia.MatchString(largo):
			// Lo único común es un consecutivo: una regla así clasificaría hoy y nunca más.
			motivo = "SOLO_REFERENCIAS"
		case len([]rune(largo)) < largoMinimoPatron || ajenosLargo > 0:
			// Ni el prefijo completo identifica al grupo sin invadir otros hechos.
			motivo = "SIN_PALABRA_SEGURA"
		default:
			patron, alterna = largo, corto
		}
		if alterna == patron {
			alterna = ""
		}

		aplica := "MIXTO"
		switch {
		case g.debitos == 0:
			aplica = "CREDITO"
		case g.creditos == 0:
			aplica = "DEBITO"
		}
		totalAlcance, ajenos := alcance(patron, forma)
		out = append(out, PatronSugerido{
			Patron:      patron,
			Motivo:      motivo,
			Alterna:     alterna,
			AplicaA:     aplica,
			Movimientos: total,
			Creditos:    g.creditos,
			Debitos:     g.debitos,
			Monto:       g.monto.StringFixed(2),
			Ejemplos:    g.ejemplos,
			AvisoAnio:   patron != "" && reAnio.MatchString(patron),
			Alcance:     totalAlcance,
			Ajenos:      ajenos,
		})
	}

	// Primero los grupos que más movimientos destraban; a igual cantidad, el de mayor monto.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Movimientos != out[j].Movimientos {
			return out[i].Movimientos > out[j].Movimientos
		}
		return decOrCero(out[i].Monto).GreaterThan(decOrCero(out[j].Monto))
	})
	if limite > 0 && len(out) > limite {
		out = out[:limite]
	}
	return out
}
