package bancos

import "context"

// Agrupamientos válidos del resumen de la selección.
const (
	AgruparConcepto = "concepto"
	AgruparCuenta   = "cuenta"
)

// ResumenSeleccion es el resumen de LO QUE ESTÁS VIENDO en la hoja de trabajo:
// cuántos movimientos, cuánto en débitos, cuánto en créditos y el neto, con su
// desglose de dos niveles.
//
// Comparte la forma del árbol del Cuadre (`conceptos` → `clasificaciones`) para que la
// pantalla lo pinte con el mismo componente; `agrupar` dice qué representan esos dos
// niveles, porque en «Por clasificar» agrupar por concepto no diría nada (todo es
// «sin concepto») y ahí lo útil es banco → cuenta.
type ResumenSeleccion struct {
	Agrupar      string               `json:"agrupar"`
	Movs         int                  `json:"movs"`
	TotalDebito  string               `json:"total_debito"`
	TotalCredito string               `json:"total_credito"`
	Neto         string               `json:"neto"`
	Conceptos    []CuadreConceptoNodo `json:"conceptos"`
}

// ResumenFiltro arma el resumen de la selección activa. El filtro es el MISMO que el de
// la lista (mismo constructor de condiciones en el repositorio), así que los números del
// encabezado siempre son los de las filas que se están viendo.
func (s *Service) ResumenFiltro(ctx context.Context, empresaID string, f FiltrosMovimientos, agrupar string) (ResumenSeleccion, error) {
	if agrupar != AgruparCuenta {
		agrupar = AgruparConcepto
	}
	filas, err := s.repo.ResumenFiltro(ctx, empresaID, f, agrupar)
	if err != nil {
		return ResumenSeleccion{}, err
	}
	b := newCuadreBuilder()
	for _, r := range filas {
		b.add(r.PadreID, r.Padre, r.HijoID, r.Hijo, r.DebitoSum, r.CreditoSum, r.Movs)
	}
	arbol := b.build("")
	return ResumenSeleccion{
		Agrupar:      agrupar,
		Movs:         arbol.Movs,
		TotalDebito:  arbol.TotalDebito,
		TotalCredito: arbol.TotalCredito,
		// El neto se deriva acá y no se guarda: crédito − débito, el mismo criterio del
		// Cuadre (entra menos sale).
		Neto:      b.totalCred.Sub(b.totalDeb).String(),
		Conceptos: arbol.Conceptos,
	}, nil
}
