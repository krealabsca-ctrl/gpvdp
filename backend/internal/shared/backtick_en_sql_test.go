package shared

// El acento grave dentro de un raw string de Go: el error que ya costó SIETE veces.
//
// QUÉ PASA. Las consultas SQL del proyecto viven en raw strings delimitados por acento grave. Un
// comentario adentro que cite un nombre de columna entre acentos graves —la forma natural de
// escribirlo en Markdown, y la que usan los comentarios del resto del código— CIERRA el string a
// mitad de la consulta. El compilador no dice «hay un backtick de más»: dice «expected ';', found
// count» o «illegal rune literal» quince líneas después, y hay que ir a buscarlo.
//
// Ocurrió en bancos, en cxp (dos veces), en el plazo del proveedor, en el convenio y en el gasto
// de la huella. Siempre igual: se escribe el comentario, se compila, se pierde un minuto leyendo
// un error que no habla del problema.
//
// Esta prueba lo nombra. No evita escribirlo, pero convierte un error de sintaxis desconcertante
// en un mensaje que dice exactamente qué pasó y en qué línea.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNingunComentarioSQLUsaAcentoGrave(t *testing.T) {
	raiz := filepath.Join("..", "..", "internal")

	var hallazgos []string
	err := filepath.Walk(raiz, func(ruta string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(ruta, ".go") {
			return err
		}
		data, err := os.ReadFile(ruta)
		if err != nil {
			return err
		}

		// Se recorre el archivo contando acentos graves: los de número IMPAR abren un raw string
		// y los pares lo cierran. Un comentario SQL (una línea que empieza con «--») que aparezca
		// mientras hay un raw string abierto y contenga un acento grave es el defecto.
		dentro := false
		for i, linea := range strings.Split(string(data), "\n") {
			recortada := strings.TrimSpace(linea)
			esComentarioSQL := strings.HasPrefix(recortada, "--")

			if dentro && esComentarioSQL && strings.Contains(linea, "`") {
				hallazgos = append(hallazgos,
					filepath.ToSlash(ruta)+":"+itoa(i+1)+" → "+recortada)
			}
			if strings.Count(linea, "`")%2 == 1 {
				dentro = !dentro
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("recorrer el árbol: %v", err)
	}

	if len(hallazgos) > 0 {
		t.Errorf("hay %d comentario(s) SQL con acento grave dentro de un raw string: cierran la "+
			"consulta y el compilador falla en otra línea con un mensaje que no habla del problema. "+
			"Escribí el nombre sin acentos graves.\n  %s",
			len(hallazgos), strings.Join(hallazgos, "\n  "))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
