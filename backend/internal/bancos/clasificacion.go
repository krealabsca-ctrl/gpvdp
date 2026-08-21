package bancos

import "strings"

// Regla es una regla de clasificación con sus palabras clave (RN-15/17).
// Los campos de nombre (Nombre/Concepto/Clasificacion) se llenan al listar para la
// UI de gestión; el motor de clasificación solo usa AplicaA/Palabras/IDs/Prioridad.
type Regla struct {
	ID              string   `json:"id"`
	Nombre          string   `json:"nombre"`
	AplicaA         string   `json:"aplica_a"` // DEBITO | CREDITO | MIXTO
	ConceptoID      string   `json:"concepto_id"`
	Concepto        string   `json:"concepto"`
	ClasificacionID string   `json:"clasificacion_id"`
	Clasificacion   string   `json:"clasificacion"`
	Prioridad       int      `json:"prioridad"`
	Palabras        []string `json:"palabras_clave"`
	Activo          bool     `json:"activo"`
	Aciertos        int      `json:"aciertos"`
}

// Clasificado es el resultado de aplicar una regla a un movimiento.
type Clasificado struct {
	ConceptoID      string
	ClasificacionID string
	ReglaID         string
	Confianza       int // 0..100
}

// Clasificar aplica las reglas (deben venir ordenadas por prioridad desc) a un movimiento.
// Modelo determinista por palabra clave: coincide si alguna palabra clave aparece en la
// descripción y el aplica_a corresponde al signo del movimiento. Confianza = 100 en coincidencia
// (el umbral ≥90% de RN-16 se cumple; queda como gate para un futuro matcher difuso/ML).
func Clasificar(descripcion string, esDebito bool, reglas []Regla) (Clasificado, bool) {
	descN := norm(descripcion)
	for _, r := range reglas {
		if !aplicaCorresponde(r.AplicaA, esDebito) {
			continue
		}
		for _, p := range r.Palabras {
			pn := norm(p)
			if pn != "" && strings.Contains(descN, pn) {
				return Clasificado{
					ConceptoID:      r.ConceptoID,
					ClasificacionID: r.ClasificacionID,
					ReglaID:         r.ID,
					Confianza:       100,
				}, true
			}
		}
	}
	return Clasificado{}, false
}

func aplicaCorresponde(aplicaA string, esDebito bool) bool {
	switch aplicaA {
	case "DEBITO":
		return esDebito
	case "CREDITO":
		return !esDebito
	default: // MIXTO
		return true
	}
}

// sqlEsTrasladoDerivado es la ÚNICA definición de cómo se deriva `es_traslado` al clasificar a mano.
//
// Vive acá porque la necesitan dos caminos distintos —la clasificación masiva y la clasificación en
// bloque desde Excel— y si divergieran, el mismo movimiento entraría al EBITDA o no según por cuál de
// los dos se lo clasificó.
//
// Un par ya emparejado SIEMPRE es traslado; sin par, decide el nombre del concepto destino.
//
//	movAlias:    prefijo de la tabla de movimientos en la consulta ("m." o "").
//	conceptoExpr: expresión SQL que da el concepto destino.
//	empresaExpr:  expresión SQL que da la empresa.
func sqlEsTrasladoDerivado(movAlias, conceptoExpr, empresaExpr string) string {
	return `(` + movAlias + `par_traslado_id IS NOT NULL)
	        OR EXISTS (SELECT 1 FROM concepto
	                   WHERE id = ` + conceptoExpr + ` AND empresa_id = ` + empresaExpr + `
	                     AND (nombre ILIKE '%traslado%' OR nombre ILIKE '%overnight%'))`
}
