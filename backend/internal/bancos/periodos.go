package bancos

// Qué meses tiene cargados la empresa, para que los selectores de período dejen de inventarlos.
//
// ── EL DEFECTO QUE ARREGLA ───────────────────────────────────────────────────────────────────
//
// Los dos selectores de período del sistema —el de la barra superior y la parrilla de Exportar—
// armaban su lista contando 18 meses hacia atrás desde el reloj, SIN mirar la base. Eso hacía dos
// daños opuestos, medidos el 16-nov-2026:
//
//   · ESCONDÍA MESES QUE EXISTEN. Memorial Pets tiene 21 meses cargados desde enero 2025; la
//     ventana de 18 meses desde noviembre 2026 empieza en junio 2025, así que enero a mayo 2025
//     eran inalcanzables desde la pantalla. El reporte «salía con cap».
//   · OFRECÍA MESES QUE NO EXISTEN. Valle de Paz tiene 3 meses (julio a setiembre 2026) y el
//     selector ofrecía 18: elegir noviembre y no ver nada parecía una falla del sistema.
//
// ── POR QUÉ DOS LISTAS Y NO UNA ──────────────────────────────────────────────────────────────
//
// `ConMovimiento` son los meses que de verdad tienen movimiento bancario. Sirve para EXPORTAR:
// bajar el Excel de un mes vacío es perder el viaje, así que ahí no se ofrece.
//
// El selector de la barra NO puede usar esa lista: gobierna 16 pantallas, y varias trabajan
// justamente sobre meses sin movimiento todavía —cargar el tipo de cambio del mes que arranca,
// cerrar un período contable, ver la nómina—. Si se limitara a los meses con movimiento, cargar
// el tipo de cambio de un mes nuevo sería imposible. Por eso van también `Primero` y `Ultimo`: la
// barra arma con ellos un rango CONTINUO desde el primer mes con dato hasta hoy.

import "context"

// PeriodosDeLaEmpresa es el calendario real de la empresa.
type PeriodosDeLaEmpresa struct {
	// ConMovimiento: los meses con al menos un movimiento bancario, del más viejo al más nuevo.
	// Puede tener huecos, y el hueco es información: significa que ese mes no se cargó.
	ConMovimiento []string `json:"con_movimiento"`
	// Primero y Ultimo son los extremos. Vacíos si la empresa no tiene nada cargado todavía.
	Primero string `json:"primero"`
	Ultimo  string `json:"ultimo"`
}

// Periodos devuelve los meses cargados de la empresa.
//
// Sin cifras: solo etiquetas «YYYY-MM». Es lo que permite servirlo a cualquiera que entró al
// sistema —el selector está en la barra de TODAS las pantallas, también las de quien solo tiene
// «Mi ingreso»— sin abrirle nada del módulo.
func (s *Service) Periodos(ctx context.Context, empresaID string) (PeriodosDeLaEmpresa, error) {
	meses, err := s.repo.PeriodosConMovimiento(ctx, empresaID)
	if err != nil {
		return PeriodosDeLaEmpresa{}, err
	}
	p := PeriodosDeLaEmpresa{ConMovimiento: meses}
	if len(meses) > 0 {
		p.Primero = meses[0]
		p.Ultimo = meses[len(meses)-1]
	}
	return p, nil
}
