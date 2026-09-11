package shared

import "strings"

// TextoParaBanco deja un texto en la forma que aceptan las plataformas bancarias: sin tildes, sin
// eñes y sin caracteres especiales.
//
// ── POR QUÉ EXISTE ──────────────────────────────────────────────────────────
//
// El nombre del proveedor (y el del empleado) viaja tal cual al archivo que se sube al banco. Los
// nombres reales del catálogo traen tildes y eñes —«PETRÓLEOS DELTA S.A.», «AGRO UJARRÁS S.A.»,
// «SUPERMERCADO CADENA SANCARLEÑA»— y también separadores que el propio formato usa: la coma y el
// PUNTO Y COMA. Una línea con un punto y coma en el nombre puede partirse en dos campos y el banco
// rechaza el archivo entero, o peor: lo acepta con los campos corridos.
//
// El requisito es del Director Financiero (10 de setiembre de 2026): «el formato de las macros no
// debe incluir tildes ni caracteres especiales que reboten en las plataformas bancarias».
//
// ── LA REGLA ────────────────────────────────────────────────────────────────
//
//  1. Las vocales acentuadas, la eñe y la diéresis se transliteran a su letra base. NO se borran:
//     borrarlas convertiría «PETRÓLEOS» en «PETRLEOS», que es un nombre que el proveedor no
//     reconoce en su estado de cuenta.
//  2. Todo lo que no sea letra, dígito, espacio, punto o guion se vuelve espacio. Eso saca la coma,
//     el punto y coma, el ampersand y los paréntesis. El punto se conserva porque «S.A.» es parte
//     del nombre legal y es inofensivo; el guion también.
//  3. Los espacios repetidos se colapsan y se recortan los extremos, para que sacar un carácter no
//     deje un hueco doble en el archivo.
//
// Se conserva MAYÚSCULA/minúscula tal como está en el catálogo: cambiarla no lo pidió nadie y el
// nombre lo lee una persona del otro lado.
func TextoParaBanco(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if base, ok := sinTilde[r]; ok {
			b.WriteRune(base)
			continue
		}
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-':
			b.WriteRune(r)
		default:
			// Coma, punto y coma, ampersand, paréntesis, comillas, cualquier símbolo y cualquier
			// letra que no se pudo transliterar: espacio. Nunca se elimina sin dejar la separación,
			// o dos palabras quedarían pegadas.
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// sinTilde son las letras con diacrítico que aparecen en nombres del catálogo (y las vecinas
// razonables), con su letra base. Cubre el español y los diacríticos que arrastran los nombres
// importados de otros sistemas.
var sinTilde = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a', 'ã': 'a', 'å': 'a',
	'Á': 'A', 'À': 'A', 'Ä': 'A', 'Â': 'A', 'Ã': 'A', 'Å': 'A',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'É': 'E', 'È': 'E', 'Ë': 'E', 'Ê': 'E',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'Í': 'I', 'Ì': 'I', 'Ï': 'I', 'Î': 'I',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o', 'õ': 'o',
	'Ó': 'O', 'Ò': 'O', 'Ö': 'O', 'Ô': 'O', 'Õ': 'O',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
	'Ú': 'U', 'Ù': 'U', 'Ü': 'U', 'Û': 'U',
	'ñ': 'n', 'Ñ': 'N',
	'ç': 'c', 'Ç': 'C',
}
