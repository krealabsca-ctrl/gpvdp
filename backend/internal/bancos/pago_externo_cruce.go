package bancos

// El cruce entre el reporte del otro ERP y los movimientos del banco.
//
// Función PURA: recibe las filas del reporte y los débitos candidatos, y dice qué calza con qué.
// No toca la base. Así se puede probar el criterio —que es lo delicado— sin montar un esquema, y
// el servicio se queda con una sola responsabilidad: traer los datos y escribir el resultado.
//
// ── EL CRITERIO ────────────────────────────────────────────────────────────────────────────────
//
// Dos caminos, en este orden:
//
//  1. REFERENCIA. Si el reporte trae una referencia y aparece en el documento o en la descripción
//     del movimiento, calza. Es el camino bueno: la referencia la escribió un humano de los dos
//     lados y no depende de que los montos cuadren al céntimo.
//  2. FECHA + MONTO exactos. El de respaldo, para los reportes que no numeran sus pagos.
//
// Nunca por un dato solo. Un monto repetido calza con decenas de débitos y una fecha con cientos:
// emparejar así no es conciliar, es inventar. Y cuando hay MÁS DE UN candidato no se elige ninguno
// —se dice que hay varios—, porque acertar por casualidad es peor que no marcar: deja una marca
// que nadie va a volver a revisar.

import (
	"strings"

	"github.com/shopspring/decimal"
)

// DebitoCandidato es un movimiento de salida que todavía no tiene sello ni enlace.
type DebitoCandidato struct {
	ID          string
	Fecha       string // YYYY-MM-DD
	Documento   string
	Descripcion string
	Monto       decimal.Decimal // el débito, positivo
}

// Veredicto de una fila del reporte.
const (
	// CruceUnico: calzó con exactamente un movimiento. Es el único que se marca.
	CruceUnico = "UNICO"
	// CruceVarios: calzó con más de uno. No se marca ninguno: elegir sería adivinar.
	CruceVarios = "VARIOS"
	// CruceSinMovimiento: no calzó con nada. El pago puede ser de una cuenta que no se cargó, de
	// otra empresa, o de un período que el ERP todavía no tiene.
	CruceSinMovimiento = "SIN_MOVIMIENTO"
	// CruceIlegible: la fila no se pudo leer o no trae con qué cruzar.
	CruceIlegible = "ILEGIBLE"
)

// Cómo se encontró el movimiento, para que el reporte del cruce lo diga.
const (
	PorReferencia  = "REFERENCIA"
	PorFechaYMonto = "FECHA_Y_MONTO"
)

// ResultadoCruce es el veredicto de UNA fila del reporte.
type ResultadoCruce struct {
	Fila FilaReportePago `json:"fila"`
	// Veredicto: UNICO | VARIOS | SIN_MOVIMIENTO | ILEGIBLE.
	Veredicto string `json:"veredicto"`
	// Por: REFERENCIA | FECHA_Y_MONTO. Vacío si no calzó.
	Por string `json:"por,omitempty"`
	// MovimientoID del único candidato. Vacío salvo en UNICO.
	MovimientoID string `json:"movimiento_id,omitempty"`
	// Candidatos es cuántos movimientos calzaron (0, 1 o más).
	Candidatos int `json:"candidatos"`
	// DiferenciaMonto: lo que el reporte dice menos lo que el banco debitó, cuando calzó por
	// REFERENCIA y los montos no coinciden. Vacío cuando son iguales o no se puede comparar.
	//
	// No impide el sello: que difieran es un HALLAZGO —una comisión, un redondeo, un pago parcial—
	// y esconderlo detrás de un rechazo haría perder la fila entera.
	DiferenciaMonto string `json:"diferencia_monto,omitempty"`
}

// CruzarReporte empareja cada fila del reporte contra los débitos candidatos.
//
// El cruce se hace en memoria y no fila por fila contra la base: con 3.437 débitos sin marcar el
// conjunto entra holgado, y 50.000 consultas sueltas tardarían minutos.
func CruzarReporte(filas []FilaReportePago, candidatos []DebitoCandidato) []ResultadoCruce {
	// Índice por fecha+monto para el camino 2. La clave junta los dos: por separado no sirven.
	porFechaMonto := map[string][]int{}
	for i, c := range candidatos {
		porFechaMonto[c.Fecha+"|"+c.Monto.StringFixed(2)] = append(porFechaMonto[c.Fecha+"|"+c.Monto.StringFixed(2)], i)
	}

	out := make([]ResultadoCruce, 0, len(filas))
	// Un movimiento no se puede marcar dos veces en la MISMA corrida: si dos filas del reporte
	// calzan con el mismo débito, la segunda queda sin él. Sin esto, un reporte con duplicados
	// marcaría el mismo movimiento dos veces y el conteo mentiría.
	tomados := map[string]bool{}

	for _, f := range filas {
		r := ResultadoCruce{Fila: f}
		if f.Motivo != "" || !f.SePuedeCruzar() {
			r.Veredicto = CruceIlegible
			out = append(out, r)
			continue
		}

		// ── Camino 1: la referencia
		if f.Referencia != "" {
			ref := normalizarRef(f.Referencia)
			var encontrados []int
			for i, c := range candidatos {
				if tomados[c.ID] {
					continue
				}
				if refAparece(ref, c) {
					encontrados = append(encontrados, i)
				}
			}
			if len(encontrados) > 0 {
				decidir(&r, encontrados, candidatos, PorReferencia, tomados)
				out = append(out, r)
				continue
			}
		}

		// ── Camino 2: fecha + monto exactos
		if f.Fecha != "" && f.Monto.IsPositive() {
			var libres []int
			for _, i := range porFechaMonto[f.Fecha+"|"+f.Monto.StringFixed(2)] {
				if !tomados[candidatos[i].ID] {
					libres = append(libres, i)
				}
			}
			if len(libres) > 0 {
				decidir(&r, libres, candidatos, PorFechaYMonto, tomados)
				out = append(out, r)
				continue
			}
		}

		r.Veredicto = CruceSinMovimiento
		out = append(out, r)
	}
	return out
}

// decidir cierra el veredicto de una fila que tuvo candidatos.
func decidir(r *ResultadoCruce, idx []int, candidatos []DebitoCandidato, por string, tomados map[string]bool) {
	r.Candidatos = len(idx)
	r.Por = por
	if len(idx) != 1 {
		// Más de uno: no se elige. Acertar por casualidad deja una marca que nadie vuelve a mirar.
		r.Veredicto = CruceVarios
		r.Por = ""
		return
	}
	c := candidatos[idx[0]]
	r.Veredicto = CruceUnico
	r.MovimientoID = c.ID
	tomados[c.ID] = true
	// Por fecha+monto los montos son iguales por definición; solo tiene sentido comparar cuando se
	// encontró por referencia.
	if por == PorReferencia && r.Fila.Monto.IsPositive() && !r.Fila.Monto.Equal(c.Monto) {
		r.DiferenciaMonto = r.Fila.Monto.Sub(c.Monto).StringFixed(2)
	}
}

// refAparece dice si la referencia del reporte está en el movimiento.
//
// Se compara contra el documento (igualdad) y contra la descripción (contenida). El banco mete la
// referencia dentro de un texto más largo —«TEF 123456 A FAVOR DE…»—, así que exigir igualdad en la
// descripción no encontraría nada; y exigir solo «contenida» en el documento haría que la
// referencia «1» calzara con todos.
func refAparece(ref string, c DebitoCandidato) bool {
	if ref == "" {
		return false
	}
	if normalizarRef(c.Documento) == ref {
		return true
	}
	// Mínimo cuatro caracteres para buscar dentro del texto: con menos, cualquier descripción la
	// contiene por casualidad y el cruce empieza a marcar movimientos al azar.
	if len(ref) >= 4 && strings.Contains(normalizarRef(c.Descripcion), ref) {
		return true
	}
	return false
}

// normalizarRef deja la referencia comparable: sin espacios, guiones ni ceros a la izquierda, y en
// mayúsculas. Los dos sistemas escriben el mismo número de formas distintas —«REF-00123», «ref
// 123», «123»— y compararlas crudas no encontraría nada.
func normalizarRef(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '-', '_', '.', '/', '#':
			continue
		default:
			b.WriteRune(r)
		}
	}
	limpio := b.String()
	// Los ceros de relleno se quitan SOLO si lo que queda sigue teniendo algo: «000» no se vuelve "".
	recortado := strings.TrimLeft(limpio, "0")
	if recortado == "" {
		return limpio
	}
	return recortado
}
