package bancos

// Qué cuenta como ingreso y qué como gasto: UNA definición para todo el módulo.
//
// Antes cada consulta la escribía por su cuenta como «crédito que no es traslado» / «débito que no
// es traslado». Eso metía al EBITDA el ahorro, las reservas, los préstamos, los aportes entre
// empresas y lo que todavía no está clasificado: en agosto 2026 inflaba los gastos de Valle de Paz
// en ₡35,3 millones. La naturaleza la declara el usuario en el CONCEPTO (migración 0060), y estas
// constantes son el único lugar donde eso se traduce a SQL — si cada panel lo resolviera, el KPI del
// encabezado y la tendencia del gráfico volverían a decir cosas distintas.

// Naturalezas posibles del concepto.
const (
	NaturalezaIngreso = "INGRESO"
	NaturalezaGasto   = "GASTO"
	NaturalezaNeutro  = "NEUTRO"
)

// Expresiones SQL de ingreso y gasto. Asumen que la consulta tiene `m` (movimiento_bancario) y `co`
// (concepto, con LEFT JOIN: un movimiento sin clasificar no tiene concepto y por eso no cuenta).
//
// Se toma el NETO del concepto, no solo el crédito o solo el débito: una devolución dentro de un
// concepto de ingreso baja el ingreso —no aparece como gasto—, y el reembolso de un gasto baja el
// gasto. Con `credito - debito` eso sale solo y sin casos especiales.
//
// `es_traslado` sigue excluido además de la naturaleza: un movimiento marcado como traslado
// emparejado no cuenta aunque su concepto diga otra cosa (decisión confirmada del DF sobre el
// EBITDA). Es un cinturón sobre el tirante, y es el que ya estaba.
const (
	sqlIngresoNeto = `COALESCE(SUM(CASE WHEN co.naturaleza = 'INGRESO' AND NOT m.es_traslado
	                                   THEN CASE WHEN m.credito > 0 THEN m.monto_crc ELSE -m.monto_crc END
	                                   ELSE 0 END), 0)`
	sqlGastoNeto = `COALESCE(SUM(CASE WHEN co.naturaleza = 'GASTO' AND NOT m.es_traslado
	                                 THEN CASE WHEN m.debito > 0 THEN m.monto_crc ELSE -m.monto_crc END
	                                 ELSE 0 END), 0)`

	// sqlFueraDelEbitda: lo que NO entró al EBITDA por naturaleza NEUTRO o por no tener concepto.
	// Va como aviso, no como corrección: el número tiene que ser el que el usuario declaró, y a la
	// vez tiene que verse qué quedó afuera para que un concepto sin declarar no pase inadvertido.
	sqlFueraDelEbitda = `COALESCE(SUM(CASE WHEN COALESCE(co.naturaleza, 'NEUTRO') = 'NEUTRO' AND NOT m.es_traslado
	                                      THEN m.monto_crc ELSE 0 END), 0)`
	sqlMovsFueraDelEbitda = `COUNT(*) FILTER (WHERE COALESCE(co.naturaleza, 'NEUTRO') = 'NEUTRO' AND NOT m.es_traslado)`

	// sqlMontoEnSuSentido es el monto de UNA partida expresado en su propio sentido: un gasto suma
	// como gasto y un ingreso como ingreso, los dos en positivo. Es la misma definición de arriba,
	// vista partida por partida en vez de sumada por naturaleza; sale de acá para que el análisis por
	// partida no pueda discrepar del EBITDA del tablero.
	//
	// Sin esto, un solo criterio de signo (por ejemplo «el débito suma») deja todos los ingresos en
	// negativo y la pantalla se lee al revés. NEUTRO no tiene sentido declarado: se muestra el monto
	// tal cual, igual que en `sqlFueraDelEbitda`.
	sqlMontoEnSuSentido = `SUM(CASE
	                            WHEN co.naturaleza = 'INGRESO'
	                                 THEN CASE WHEN m.credito > 0 THEN m.monto_crc ELSE -m.monto_crc END
	                            WHEN co.naturaleza = 'GASTO'
	                                 THEN CASE WHEN m.debito > 0 THEN m.monto_crc ELSE -m.monto_crc END
	                            ELSE m.monto_crc END)`
)

// joinConcepto es el LEFT JOIN que necesitan las expresiones de arriba.
const joinConcepto = `LEFT JOIN concepto co ON co.id = m.concepto_id`

// NaturalezaValida dice si el valor es una de las tres. El borde valida antes de escribir.
func NaturalezaValida(v string) bool {
	switch v {
	case NaturalezaIngreso, NaturalezaGasto, NaturalezaNeutro:
		return true
	default:
		return false
	}
}

// nombreNaturaleza es el nombre corto, para mensajes dentro de una frase.
func nombreNaturaleza(v string) string {
	switch v {
	case NaturalezaIngreso:
		return "Ingreso"
	case NaturalezaGasto:
		return "Gasto"
	default:
		return "No entra al EBITDA"
	}
}

// EtiquetaNaturaleza explica la naturaleza en una frase, para la pantalla.
func EtiquetaNaturaleza(v string) string {
	switch v {
	case NaturalezaIngreso:
		return "Suma a los ingresos del EBITDA"
	case NaturalezaGasto:
		return "Suma a los gastos del EBITDA"
	default:
		return "No entra al EBITDA (tesorería, ahorro, reservas, aportes)"
	}
}
