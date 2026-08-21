package bancos

// Exportación (Fase D, §30): tipos y el Consecutivo Largo de Davivienda.

// MovimientoExport es una fila lista para exportar (montos como string decimal).
type MovimientoExport struct {
	Fecha         string
	Documento     string
	Descripcion   string
	Banco         string
	Cuenta        string
	Debito        string
	Credito       string
	Moneda        string
	MontoCRC      string
	Concepto      string
	Clasificacion string
	Estado        string
	EsTraslado    bool
}

// ConsecutivoLargo deriva el "Consecutivo Largo" de Davivienda desde la descripción
// (§30 / Formatos §6): EXTRAE(desc; 24; 25) — 25 caracteres a partir de la posición 24.
// EXTRAE es 1-indexado (Excel), así que en Go es el rango de runas [23:48].
// Solo aplica cuando el banco es Davivienda; para el resto devuelve "".
// Si la descripción es más corta que la posición pedida, devuelve lo que haya (como EXTRAE).
func ConsecutivoLargo(banco, descripcion string) string {
	if !esDavivienda(banco) {
		return ""
	}
	r := []rune(descripcion)
	const inicio = 23 // posición 24 en base 1
	const largo = 25
	if inicio >= len(r) {
		return ""
	}
	fin := inicio + largo
	if fin > len(r) {
		fin = len(r)
	}
	return string(r[inicio:fin])
}

func esDavivienda(banco string) bool {
	return normalizarBanco(banco) == "davivienda"
}

// normalizarBanco reutiliza norm() (minúsculas + sin acentos + trim) para comparar nombres.
func normalizarBanco(s string) string { return norm(s) }
