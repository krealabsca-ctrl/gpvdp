package bancos

// Cuando NO se reconoce el encabezado, decir qué se encontró (21-oct-2026, punto 2 de la revisión
// comercial).
//
// ── EL PROBLEMA ────────────────────────────────────────────────────────────────────────────
//
// El lector no tiene un formato por banco: reconoce las columnas por una tabla de SINÓNIMOS de
// encabezado (`encabezadosMovimientos`). Eso es mucho mejor de lo que parece —agregar un banco es
// agregar sinónimos, no escribir un adaptador— pero tenía un borde malo.
//
// Cuando el archivo trae un encabezado que la tabla no conoce, la respuesta era una frase fija:
//
//     «no se encontró el encabezado (se esperan al menos las columnas Fecha y Débito/Crédito)»
//
// Esa frase dice qué hacía falta y NO dice qué había. Con eso, quien sube el archivo no puede
// distinguir entre cuatro cosas distintas: que eligió la hoja equivocada, que el archivo trae
// filas de título de más, que el banco escribe «F. Movimiento» en vez de «Fecha», o que el archivo
// está vacío. Las cuatro se ven igual, y las cuatro tienen arreglos distintos.
//
// Y para un sistema que va a venderse, es la diferencia entre «el cliente nuevo manda su archivo y
// alguien ve en dos minutos qué sinónimo falta» y «hay que pedirle el archivo, abrirlo y leer
// código».
//
// ── POR QUÉ SE DEVUELVEN LAS FILAS Y NO SOLO UNA ───────────────────────────────────────────
//
// El encabezado no siempre está en la fila 1 —el reporte que exporta el propio sistema trae dos
// filas de título antes—, así que el lector busca en las primeras 20. Mostrar solo la fila 1
// confundiría más: lo que hay que ver es qué miró el lector y qué entendió de cada fila.
//
// Se muestran las primeras pocas filas NO VACÍAS. Una lista de veinte, casi todas en blanco, es
// ruido donde hace falta señal.

import (
	"fmt"
	"strings"
)

// filasDeDiagnostico es cuántas filas con contenido se muestran al no reconocer el encabezado.
//
// Cuatro de CONTEXTO, más la que más parece el encabezado si no entró entre ellas (ver
// `mejorCandidata`). Eso último no es un adorno: el estado de cuenta de Scotiabank trae cuatro
// líneas de preámbulo —cliente, cuenta, tipo y moneda— y sus títulos recién aparecen en la quinta.
// El diagnóstico listaba las cuatro de preámbulo y cortaba justo antes de la única fila que sirve
// para configurar las columnas (23-oct-2026, reportado por el Director Financiero).
const filasDeDiagnostico = 4

// filasQueSeMiran acota hasta dónde se busca la fila de títulos. El mismo tope que usa `detect`
// para reconocer un banco: lo que el detector no mira, el diagnóstico no tiene por qué explicar.
const filasQueSeMiran = 25

// ErrorDeEncabezado explica por qué no se reconoció el encabezado, mostrando lo que sí había.
//
// Es un error tipado y no una cadena armada en el handler porque el mensaje tiene que viajar igual
// desde los tres lectores (histórico, clasificación y diccionario) y construirlo tres veces es la
// forma de que los tres terminen diciendo cosas distintas.
type ErrorDeEncabezado struct {
	// Faltan nombra los papeles que no se encontraron, en palabras del usuario («Fecha»,
	// «Débito o Crédito»), nunca las claves internas.
	Faltan []string
	// Vistas son las primeras filas con contenido, tal como venían.
	Vistas []FilaVista
	// Hoja es la hoja que se leyó. Elegir la hoja equivocada es la causa más común y la más fácil
	// de descartar cuando se dice cuál se miró.
	Hoja string
	// Hojas son TODAS las del libro. Decir «leí la hoja A, el archivo trae A, B y C» convierte la
	// causa más común en algo que se ve de un vistazo; decir solo cuál se leyó obliga a abrir el
	// Excel para saber si había otra (22-oct-2026).
	Hojas []string
	// Salida es qué hacer, cuando el lector sabe decirlo. El importador de estados de cuenta manda
	// a «Formatos de banco»; los demás lectores lo dejan vacío porque su arreglo está en el archivo.
	Salida string
	// Base es el error de dominio que este diagnóstico detalla, para que `errors.Is` lo siga
	// encontrando. Nil en los lectores que no envuelven ninguno: `Unwrap` devuelve nil y todo se
	// comporta como antes.
	Base error
}

// Unwrap expone el error de dominio que este diagnóstico detalla (p. ej. ErrNoReconocido), para
// que quien lo compare con `errors.Is` lo siga encontrando aunque el mensaje sea el rico.
func (e *ErrorDeEncabezado) Unwrap() error { return e.Base }

// FilaVista es una fila del archivo con lo que el lector entendió de ella.
type FilaVista struct {
	Numero int      `json:"numero"`
	Celdas []string `json:"celdas"`
	// Reconocidas son los papeles que SÍ se entendieron en esa fila. Es lo que convierte el
	// mensaje en un diagnóstico: «entendí Fecha y Descripción, me faltó el monto» dice dónde
	// mirar; «no se reconoció el encabezado» no dice nada.
	Reconocidas []string `json:"reconocidas"`
}

func (e *ErrorDeEncabezado) Error() string {
	var b strings.Builder
	b.WriteString("bancos: no se reconoció el encabezado del archivo")
	if e.Hoja != "" {
		b.WriteString(" (hoja «" + e.Hoja + "»")
		// Las OTRAS hojas del libro. Es la causa más común del rechazo y la única que se descarta
		// sin abrir el archivo: si el libro trae una sola hoja, no se dice nada.
		if otras := sinLaLeida(e.Hojas, e.Hoja); len(otras) > 0 {
			b.WriteString("; el archivo también trae " + enumerar(enComillasCada(otras)))
		}
		b.WriteString(")")
	}
	if len(e.Faltan) > 0 {
		b.WriteString(". Hace falta " + enumerar(e.Faltan))
	}
	if len(e.Vistas) == 0 {
		b.WriteString(". El archivo no trae ninguna fila con contenido.")
		return conSalida(b.String(), e.Salida)
	}
	b.WriteString(". Esto es lo que se encontró:")
	for _, f := range e.Vistas {
		b.WriteString(fmt.Sprintf(" fila %d: %s", f.Numero, enComillas(f.Celdas)))
		if len(f.Reconocidas) > 0 {
			b.WriteString(" (se entendió " + enumerar(f.Reconocidas) + ")")
		} else {
			b.WriteString(" (no se entendió ninguna columna)")
		}
		b.WriteString(";")
	}
	return conSalida(strings.TrimSuffix(b.String(), ";"), e.Salida)
}

// conSalida pega al final qué hacer. Un diagnóstico que describe el problema y no dice el
// siguiente paso deja a alguien mirando un mensaje largo sin saber a dónde ir.
func conSalida(msg, salida string) string {
	if salida == "" {
		return msg
	}
	return msg + ". " + salida
}

// sinLaLeida devuelve las hojas del libro menos la que se leyó.
func sinLaLeida(hojas []string, leida string) []string {
	out := make([]string, 0, len(hojas))
	for _, h := range hojas {
		if !strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(leida)) {
			out = append(out, h)
		}
	}
	return out
}

// enComillasCada entrecomilla cada nombre para que una hoja llamada «Movimientos 2026» no se lea
// como dos.
func enComillasCada(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, "«"+x+"»")
	}
	return out
}

// DiagnosticarEncabezado arma el error mirando las primeras filas con contenido.
//
// `requeridos` son los papeles que el lector necesitaba, en palabras del usuario. Cada lector pasa
// los suyos: el histórico pide fecha y monto, el de clasificación pide fecha y partida, y el del
// diccionario pide concepto. No se infieren acá porque son tres trabajos distintos.
func DiagnosticarEncabezado(g Grid, hoja string, requeridos []string) *ErrorDeEncabezado {
	e := &ErrorDeEncabezado{Faltan: requeridos, Hoja: hoja, Vistas: []FilaVista{}}

	// Primero se miran TODAS las filas del tramo, no solo las cuatro que se van a mostrar: hay que
	// saber cuál parece el encabezado antes de decidir cuáles enseñar.
	todas := filasConContenido(g)
	if len(todas) == 0 {
		return e
	}

	e.Vistas = todas
	if len(todas) <= filasDeDiagnostico {
		return e
	}

	// Las primeras de contexto, más la que más columnas reconoció si se quedó afuera. Esa es la
	// fila de títulos, y es la única con la que alguien puede configurar un formato.
	e.Vistas = todas[:filasDeDiagnostico]
	if mejor := mejorCandidata(todas); mejor != nil && !estaEntre(e.Vistas, mejor.Numero) {
		// Se inserta al final y el orden queda por número de fila: un diagnóstico que salta de la
		// fila 4 a la 12 y vuelve a la 5 se lee como si el archivo estuviera desordenado.
		e.Vistas = append(e.Vistas, *mejor)
		ordenarPorNumero(e.Vistas)
	}
	return e
}

// filasConContenido arma una FilaVista por cada fila no vacía del tramo que se mira.
func filasConContenido(g Grid) []FilaVista {
	out := []FilaVista{}
	for i := 0; i < len(g) && i < filasQueSeMiran; i++ {
		celdas := recortarVacias(g[i])
		if len(celdas) == 0 {
			continue
		}
		papeles := map[string]bool{}
		for _, c := range celdas {
			if papel, ok := encabezadosMovimientos[norm(c)]; ok {
				papeles[papel] = true
			}
		}
		out = append(out, FilaVista{Numero: i + 1, Celdas: celdas, Reconocidas: ordenados(papeles)})
	}
	return out
}

// mejorCandidata devuelve la fila que más parece el encabezado: la que reconoció más columnas.
//
// Devuelve nil si ninguna reconoció al menos dos. Con una sola coincidencia no se puede afirmar
// nada —cualquier fila de datos puede traer la palabra «fecha»—, y señalar una fila cualquiera
// como «los títulos» mandaría a configurar las columnas contra el lugar equivocado.
func mejorCandidata(filas []FilaVista) *FilaVista {
	var mejor *FilaVista
	for i := range filas {
		if len(filas[i].Reconocidas) < 2 {
			continue
		}
		if mejor == nil || len(filas[i].Reconocidas) > len(mejor.Reconocidas) {
			mejor = &filas[i]
		}
	}
	return mejor
}

func estaEntre(filas []FilaVista, numero int) bool {
	for _, f := range filas {
		if f.Numero == numero {
			return true
		}
	}
	return false
}

// ordenarPorNumero deja las filas en el orden en que están en el archivo. Son pocas (cinco), así
// que una inserción simple alcanza y se lee de un vistazo.
func ordenarPorNumero(filas []FilaVista) {
	for i := 1; i < len(filas); i++ {
		for j := i; j > 0 && filas[j-1].Numero > filas[j].Numero; j-- {
			filas[j-1], filas[j] = filas[j], filas[j-1]
		}
	}
}

// recortarVacias quita las celdas en blanco del final: una fila de Excel suele venir con decenas
// de columnas vacías a la derecha y mostrarlas tapa lo que importa.
func recortarVacias(fila []string) []string {
	fin := len(fila)
	for fin > 0 && strings.TrimSpace(fila[fin-1]) == "" {
		fin--
	}
	if fin == 0 {
		return nil
	}
	out := make([]string, 0, fin)
	for _, c := range fila[:fin] {
		out = append(out, strings.TrimSpace(c))
	}
	return out
}

// ordenados devuelve los papeles en un orden estable: un mensaje que cambia de orden entre dos
// lecturas del mismo archivo parece dos problemas distintos.
func ordenados(m map[string]bool) []string {
	orden := []string{"fecha", "documento", "debito", "credito", "descripcion", "cuenta", "concepto", "clasificacion"}
	out := make([]string, 0, len(m))
	for _, p := range orden {
		if m[p] {
			out = append(out, p)
		}
	}
	return out
}

func enComillas(v []string) string {
	q := make([]string, len(v))
	for i, s := range v {
		q[i] = "«" + s + "»"
	}
	return strings.Join(q, ", ")
}

func enumerar(v []string) string {
	switch len(v) {
	case 0:
		return ""
	case 1:
		return v[0]
	default:
		return strings.Join(v[:len(v)-1], ", ") + " y " + v[len(v)-1]
	}
}
