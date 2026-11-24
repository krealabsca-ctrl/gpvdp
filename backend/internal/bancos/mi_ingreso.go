package bancos

// «Mi presupuesto», mitad de INGRESO: lo que entró en MIS partidas (mig 0125).
//
// ── POR QUÉ LA PARTIDA Y NO EL DEPARTAMENTO ──────────────────────────────────────────────────
//
// Las 18 partidas de ingreso de Valle de Paz son CANALES DE COBRO, no áreas: «Depósito de
// Clientes» (₡90,3 M), «Datáfonos» (₡79,6 M), «Cargo BAC», «Cobrador Limón». Ninguna tiene
// departamento asignado y forzarlo inventaría un dueño que el negocio no tiene. Además el
// presupuesto de ingreso ya está modelado por clasificación (mig 0116): el eje de la plata y el
// eje de la meta tienen que ser el mismo, o el porcentaje de cumplimiento no significa nada.
//
// ── POSITIVO ES BUENO, AL REVÉS DEL GASTO ────────────────────────────────────────────────────
//
// En el gasto, pasarse del presupuesto es malo. Acá pasarse de la meta es lo que se busca, y un
// 95 % de cumplimiento es casi un logro, no una alerta. El estado viene calculado del control de
// ingreso y se respeta tal cual: recalcularlo acá con la lógica del gasto invertiría el semáforo.
//
// ── UN ALCANCE VACÍO CIERRA ──────────────────────────────────────────────────────────────────
//
// Mismo riesgo que en la mitad de gasto, mismas dos guardas: corte temprano en el servicio y
// recorte explícito acá.
//
// ── EL AVISO SE REESCRIBE, NO SE COPIA ───────────────────────────────────────────────────────
//
// Medido el 16-nov-2026 con un jefe de cobros de prueba: copiar el aviso del control de Finanzas
// le decía «17 línea(s) tuvieron ingreso sin meta propia» cuando en su pantalla hay 2. El número
// era el de la empresa, así que además de contradecir lo que veía le contaba cuántas partidas de
// ingreso tiene Valle de Paz. Otra de las partes del aviso —«el desglose suma ₡X MÁS que la
// meta»— es un error de configuración del presupuesto global: plata ajena y nada que él pueda
// hacer. El aviso de esta pantalla sale de SUS cifras.

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// ErrSinPartidasDeIngreso: la persona tiene el permiso pero no le asignaron ninguna partida.
var ErrSinPartidasDeIngreso = errors.New("bancos: todavía no te asignaron ninguna partida de ingreso; pedíselo a quien administra el catálogo")

// PartidaDelUsuario es una partida de ingreso asignada a la persona.
type PartidaDelUsuario struct {
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	Concepto        string `json:"concepto"`
}

// MiIngreso es lo que ve quien tiene partidas de ingreso asignadas.
type MiIngreso struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`
	// Partidas son las asignadas. Van siempre, aunque alguna no haya tenido ingreso: «mi partida
	// no aparece» y «mi partida no facturó» son dos cosas distintas.
	Partidas []PartidaDelUsuario `json:"partidas"`
	Lineas   []FilaIngreso       `json:"lineas"`
	// Totales de SUS partidas, no de la empresa.
	MetaTotal string `json:"meta_total"`
	RealTotal string `json:"real_total"`
	// Diferencia = real − meta. POSITIVO ES BUENO.
	Diferencia string `json:"diferencia"`
	// SinMeta cuenta cuántas de sus partidas facturaron sin tener meta propia. Es lo primero que
	// va a ver casi todo el mundo: medido el 16-nov-2026 hay CERO líneas de meta en el sistema.
	SinMeta int    `json:"sin_meta"`
	Aviso   string `json:"aviso,omitempty"`
}

// RecortarAMisPartidas deja solo las líneas de las partidas de la persona y recalcula los totales.
//
// Es la función de seguridad de esta sección y está separada del servicio para poder probarla sin
// base: lo que tiene que resistir es que le entreguen el control completo —que es lo que le
// llega, porque reusa el cálculo de Finanzas— y deje salir solo lo propio.
func RecortarAMisPartidas(ctrl ControlIngreso, partidas []PartidaDelUsuario) MiIngreso {
	mias := make(map[string]bool, len(partidas))
	for _, p := range partidas {
		mias[p.ClasificacionID] = true
	}

	mi := MiIngreso{
		Desde: ctrl.Desde, Hasta: ctrl.Hasta,
		Partidas: partidas,
		Lineas:   make([]FilaIngreso, 0, len(partidas)),
	}
	meta, real := decimal.Zero, decimal.Zero
	for _, l := range ctrl.Lineas {
		// La línea sin clasificación —el ingreso que nadie clasificó— no es de nadie. El
		// `mias[""]` daría false igual, pero se dice para que quede escrito que no es un descuido:
		// un ingreso sin partida es justo el que alguien busca, y no se le muestra a nadie.
		if l.ClasificacionID == "" || !mias[l.ClasificacionID] {
			continue
		}
		if m, err := decimal.NewFromString(l.Meta); err == nil {
			meta = meta.Add(m)
		} else {
			// Sin meta propia: se cuenta, porque sin meta no hay semáforo y hay que decirlo.
			mi.SinMeta++
		}
		if r, err := decimal.NewFromString(l.Real); err == nil {
			real = real.Add(r)
		}
		mi.Lineas = append(mi.Lineas, l)
	}
	mi.MetaTotal = meta.StringFixed(2)
	mi.RealTotal = real.StringFixed(2)
	mi.Diferencia = real.Sub(meta).StringFixed(2)
	mi.Aviso = avisoDeMiIngreso(mi, meta)
	return mi
}

// avisoDeMiIngreso explica lo que la persona tiene que saber de SUS partidas, y solo de ellas.
func avisoDeMiIngreso(mi MiIngreso, meta decimal.Decimal) string {
	switch {
	case len(mi.Lineas) == 0:
		// SIN LÍNEAS NO HAY AVISO: la pantalla ya dice «tus partidas no registran ingreso en este
		// período» en el lugar de la tabla. Decirlo dos veces, una arriba en amarillo y otra abajo,
		// hace ver un problema donde solo hay un mes sin movimiento. (Visto en pantalla el
		// 16-nov-2026.) Y «no facturaron» ≠ «no te asignaron partidas»: eso corta antes, en el
		// servicio, con ErrSinPartidasDeIngreso.
		return ""
	case meta.IsZero():
		return "Ninguna de tus partidas tiene meta declarada en este rango: lo que ves es lo que entró, " +
			"sin nada contra qué compararlo."
	case mi.SinMeta > 0:
		return fmt.Sprintf("%d de tus partidas registró ingreso sin meta propia: entra en el total pero "+
			"no se juzga.", mi.SinMeta)
	}
	return ""
}
