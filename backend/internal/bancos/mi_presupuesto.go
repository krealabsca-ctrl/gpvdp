package bancos

// «Mi presupuesto»: el gasto del área contra su presupuesto, para quien NO entra a Bancos.
//
// ── LAS DOS CAPAS, COMO EN «MI PARTIDA» ──────────────────────────────────────────────────────
//
//   · El PERMISO `bancos.ver_mi_presupuesto` dice qué PANTALLA abre.
//   · El ALCANCE —las filas de `departamento_validador` de esa persona— dice CUÁLES FILAS ve.
//
// Son dos cosas distintas a propósito: el permiso lo da Seguridad una vez, y el alcance lo mueve
// quien administra las áreas, sin tocar roles.
//
// ── UN ALCANCE VACÍO CIERRA, NUNCA ABRE ──────────────────────────────────────────────────────
//
// Es el error clásico del recorte por lista y ya mordió una vez en este sistema: se arma el filtro
// con `if len(ids) > 0`, la lista viene vacía, la condición no se agrega y el resultado queda
// ABIERTO. No falla ruidosamente — entrega los datos de toda la empresa. Por eso hay dos guardas:
// el corte temprano con `ErrSinAlcanceDePresupuesto` y el recorte explícito de las filas.
//
// ── POR QUÉ REUSA EL CÁLCULO DEL CONTROL ─────────────────────────────────────────────────────
//
// No calcula el gasto por su cuenta: pide el MISMO `ControlPresupuestario` que ve Finanzas y lo
// recorta. Dos cálculos para la misma pregunta producen tarde o temprano dos números, y el día que
// un jefe diga «yo veo ₡5 M» y Finanzas vea ₡8 M, la pantalla deja de usarse.
//
// ── LO QUE SE QUITA AL RECORTAR, Y POR QUÉ ───────────────────────────────────────────────────
//
// El gasto SIN DUEÑO (hoy ₡127,5 M, el 37,9 % del total) NO viaja: es plata de toda la empresa y
// mostrársela a un jefe de área sería mostrarle lo ajeno. Los totales se recalculan sobre lo suyo,
// o dirían el total de la empresa bajo el rótulo de su área.
//
// La SALUD DE CLASIFICACIÓN sí viaja: dice qué porcentaje del mes está clasificado, no revela
// ningún movimiento, y sin ella un mes a medio clasificar se lee como un mes de bajo gasto.

import (
	"errors"

	"github.com/shopspring/decimal"
)

// ErrSinAlcanceDePresupuesto: la persona tiene el permiso pero no es responsable de ninguna área.
//
// Es un error y no una respuesta vacía: con una lista vacía, quien abre la pantalla no sabe si su
// área no gastó nada este mes o si nadie lo declaró responsable, y son dos problemas con dueños
// distintos —el primero es suyo, el segundo es de quien administra las áreas—.
var ErrSinAlcanceDePresupuesto = errors.New("bancos: todavía no estás declarado como responsable de ninguna área; pedíselo a quien administra los departamentos")

// AreaDelUsuario es un área de la que la persona es responsable.
type AreaDelUsuario struct {
	DepartamentoID string `json:"departamento_id"`
	Departamento   string `json:"departamento"`
	// Rol es TITULAR o SUPLENTE. Viaja porque un suplente que ve el mismo número tiene que saber
	// que lo está viendo como suplente.
	Rol string `json:"rol"`
}

// MiPresupuesto es lo que ve un líder de área: su control, sin nada de las otras.
type MiPresupuesto struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`
	// Areas son las que la persona tiene a cargo. Van siempre, aunque alguna no tenga gasto:
	// «mi área no aparece» y «mi área no gastó» son cosas distintas.
	Areas []AreaDelUsuario `json:"areas"`
	Filas []FilaControl    `json:"filas"`
	// Totales de SUS áreas, no de la empresa.
	TotalPresupuesto string `json:"total_presupuesto"`
	TotalGasto       string `json:"total_gasto"`
	// Meses y Aviso se heredan del control: son sobre la calidad del dato, no sobre plata ajena.
	Meses []SaludMes `json:"meses"`
	Aviso string     `json:"aviso"`
	// SinPresupuesto cuenta cuántas de sus áreas no tienen monto definido. Va aparte porque es lo
	// primero que va a ver casi todo el mundo: medido el 16-nov-2026, de 9 áreas con gasto solo 2
	// tienen presupuesto cargado, y una pantalla sin semáforo necesita explicar por qué.
	SinPresupuesto int `json:"sin_presupuesto"`
}

// RecortarAMisAreas deja solo las filas de las áreas de la persona y recalcula los totales.
//
// Está separada del servicio para poder probarla sin base: es LA función de seguridad de esta
// pantalla, y lo que tiene que resistir es que le pasen un control con áreas ajenas adentro.
func RecortarAMisAreas(ctrl ControlPresupuestario, areas []AreaDelUsuario) MiPresupuesto {
	mias := make(map[string]bool, len(areas))
	for _, a := range areas {
		mias[a.DepartamentoID] = true
	}

	mp := MiPresupuesto{
		Desde: ctrl.Desde, Hasta: ctrl.Hasta,
		Areas: areas,
		Filas: make([]FilaControl, 0, len(areas)),
		Meses: ctrl.Meses, Aviso: ctrl.Aviso,
	}
	presu, gasto := decimal.Zero, decimal.Zero
	for _, f := range ctrl.Filas {
		// La fila «(sin asignar)» no tiene id y nunca es de nadie: el `mias[""]` daría false de
		// todos modos, pero se dice para que quede escrito que no es un descuido.
		if f.DepartamentoID == "" || !mias[f.DepartamentoID] {
			continue
		}
		if p, err := decimal.NewFromString(f.Presupuesto); err == nil {
			presu = presu.Add(p)
		}
		if g, err := decimal.NewFromString(f.Gasto); err == nil {
			gasto = gasto.Add(g)
		}
		if f.Estado == CtrlSinPresupuesto {
			mp.SinPresupuesto++
		}
		mp.Filas = append(mp.Filas, f)
	}
	mp.TotalPresupuesto = presu.StringFixed(2)
	mp.TotalGasto = gasto.StringFixed(2)
	if mp.Meses == nil {
		mp.Meses = []SaludMes{}
	}
	return mp
}
