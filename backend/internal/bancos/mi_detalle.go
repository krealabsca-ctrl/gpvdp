package bancos

// «¿En qué se me fue?» y «¿en qué mes?»: el detalle de «Mi presupuesto» y «Mi ingreso».
//
// ── POR QUÉ HACÍA FALTA ──────────────────────────────────────────────────────────────────────
//
// Las dos pantallas daban UN número por área (o por partida) y nada más. «Mi área gastó ₡4,2 M»
// no se puede usar para nada: la primera pregunta de cualquier jefe es en qué se fue, y la
// segunda es en qué mes. Sin las dos, la única salida era llamar a Finanzas, que es justo la
// llamada que estas pantallas vienen a evitar.
//
// ── NO SE CALCULA NADA NUEVO ─────────────────────────────────────────────────────────────────
//
// El desglose de gasto reusa `PartidasDeDimension` —el mismo que ve Finanzas en Control y
// presupuesto— y la serie de ingreso reusa `SeriePorPartida` —la misma de Análisis y tendencias—.
// Dos cálculos para la misma pregunta terminan dando dos números, y entonces el jefe y Finanzas
// discuten sobre cuál está bien en vez de sobre el gasto.
//
// ── LA GUARDA ES EL DEPARTAMENTO PEDIDO ──────────────────────────────────────────────────────
//
// Acá el usuario ELIGE qué área mirar, así que el id viaja en el pedido. Sin comprobar que esa
// área es suya, cualquier jefe podría pedir el desglose de cualquier otra con solo cambiar un
// parámetro: es el camino de fuga que no existía mientras la pantalla no tenía detalle.

import "errors"

// ErrAreaFueraDeMiAlcance: se pidió el detalle de un área que no es de la persona.
//
// Es 404 y no 403 por la misma razón que `ErrFueraDeAlcance`: para esa persona esa área no
// existe, y un 403 confirmaría que existe.
var ErrAreaFueraDeMiAlcance = errors.New("bancos: esa área no está en tu alcance")

// MiIngresoSerie es el mes a mes de las partidas de la persona.
type MiIngresoSerie struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`
	// Partidas, cada una con su serie. Van las asignadas que tuvieron movimiento; una partida sin
	// nada en el rango no aparece acá, y la pantalla ya la nombra arriba de todos modos.
	Partidas []TendenciaPartida `json:"partidas"`
}

// RecortarSerieAMisPartidas deja solo las series de las partidas de la persona.
//
// Separada del servicio para poder probarla sin base, igual que `RecortarAMisPartidas`: lo que
// tiene que resistir es que le entreguen la serie de las 131 partidas de la empresa —que es lo
// que le llega, porque reusa el cálculo de Análisis— y deje salir solo las propias.
func RecortarSerieAMisPartidas(series []TendenciaPartida, partidas []PartidaDelUsuario) []TendenciaPartida {
	mias := make(map[string]bool, len(partidas))
	for _, p := range partidas {
		mias[p.ClasificacionID] = true
	}
	out := make([]TendenciaPartida, 0, len(partidas))
	for _, s := range series {
		// Igual que en el recorte del control: lo sin clasificar no es de nadie, y se dice en vez
		// de depender de que el mapa no tenga la clave vacía.
		if s.ClasificacionID == "" || !mias[s.ClasificacionID] {
			continue
		}
		out = append(out, s)
	}
	return out
}

// EsAreaDeLaPersona dice si ese departamento está entre los suyos.
func EsAreaDeLaPersona(areas []AreaDelUsuario, departamentoID string) bool {
	// Un id vacío NO vale como «todas»: sin esto, omitir el parámetro devolvería el desglose de la
	// empresa entera, que es exactamente lo que estas pantallas no muestran.
	if departamentoID == "" {
		return false
	}
	for _, a := range areas {
		if a.DepartamentoID == departamentoID {
			return true
		}
	}
	return false
}
