package precios

// Costeo por actividad (ABC): el indirecto se reparte por lo que lo causa.
//
// ── LA CADENA, EN UNA LÍNEA ─────────────────────────────────────────────────────────────────
//
//	gasto de Bancos --[% al driver]--> bolsa del driver --[÷ base]--> costo unitario
//	                                                      --[× consumo del plan]--> indirecto
//
// ── LAS DOS COSAS QUE ESTE ARCHIVO SE NIEGA A HACER ─────────────────────────────────────────
//
//  1. **Repartir un gasto cuyos porcentajes no suman 100 %.** En el archivo de costeo las dos
//     hojas reparten 45 % y 85 %: se absorbe más gasto del que existe. Acá un reparto incompleto
//     o excedido se informa y NO se usa, porque absorber 130 % del gasto infla el costo de todo
//     lo que se vende y absorber 70 % lo desinfla, y ninguna de las dos cosas se nota mirando el
//     total.
//
//  2. **Dividir entre una base presupuestada.** La base es la medición del período. Con un
//     divisor fijo, si baja la venta sube el costo unitario y el modelo pide subir el precio
//     justo cuando el mercado se enfría.

import (
	"errors"
	"strings"

	"github.com/shopspring/decimal"
)

// Errores del reparto.
var (
	ErrDriverNoEncontrado = errors.New("precios: driver no encontrado")
	ErrPorcentajeInvalido = errors.New("precios: el porcentaje tiene que estar entre 0 y 100")
	ErrConsumoNoPositivo  = errors.New("precios: el consumo del driver debe ser mayor que cero")
	ErrBaseNoPositiva     = errors.New("precios: la base del período debe ser mayor que cero")
	ErrPeriodoInvalido    = errors.New("precios: el período tiene que ser una fecha (AAAA-MM-DD)")
	ErrDriverEnUso        = errors.New("precios: el driver está en uso por algún gasto o plan; quitalo de ahí antes de eliminarlo")
)

// Driver es un generador de costo.
type Driver struct {
	ID          string `json:"id"`
	Codigo      string `json:"codigo"`
	Nombre      string `json:"nombre"`
	Unidad      string `json:"unidad"`
	Descripcion string `json:"descripcion"`
	Activo      bool   `json:"activo"`

	// Gastos y planes que lo usan, para no borrar algo del que cuelga un costeo.
	Gastos int `json:"gastos"`
	Planes int `json:"planes"`
	// BaseDelPeriodo es la medición vigente, nil cuando no se cargó.
	BaseDelPeriodo *string `json:"base_del_periodo"`
}

// RepartoDeGasto es una clasificación de gasto con su reparto entre drivers.
type RepartoDeGasto struct {
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	Concepto        string `json:"concepto"`
	// GastoDelPeriodo es lo que de verdad salió por esa clasificación, tomado de Bancos.
	GastoDelPeriodo string           `json:"gasto_del_periodo"`
	Lineas          []LineaDeReparto `json:"lineas"`
	// SumaPct es cuánto suman los porcentajes. Distinto de 100 ⇒ el gasto NO se reparte.
	SumaPct string `json:"suma_pct"`
	Valido  bool   `json:"valido"`
	Motivo  string `json:"motivo"`
}

// LineaDeReparto es la porción de un gasto que va a un driver.
type LineaDeReparto struct {
	ID         string `json:"id"`
	DriverID   string `json:"driver_id"`
	Driver     string `json:"driver"`
	Porcentaje string `json:"porcentaje"`
	Monto      string `json:"monto"`
}

// BolsaDeDriver es lo que un driver acumuló en el período y su costo unitario.
type BolsaDeDriver struct {
	DriverID string `json:"driver_id"`
	Driver   string `json:"driver"`
	Unidad   string `json:"unidad"`
	Gasto    string `json:"gasto"`
	// Base es la medición del período. nil ⇒ no se puede calcular el unitario.
	Base          *string `json:"base"`
	CostoUnitario *string `json:"costo_unitario"`
	Motivo        string  `json:"motivo"`
}

// RepartoDelPeriodo es la foto completa: qué se repartió, a qué driver y a qué costo unitario.
type RepartoDelPeriodo struct {
	Desde string `json:"desde"`
	Hasta string `json:"hasta"`

	GastoTotal string `json:"gasto_total"`
	// GastoRepartido es lo que efectivamente entró a un driver. La diferencia con el total es
	// gasto sin asignar, y se muestra: un indirecto que no se reparte no desaparece, queda sin
	// absorber y el margen sale más alto de lo que es.
	GastoRepartido  string `json:"gasto_repartido"`
	GastoSinAsignar string `json:"gasto_sin_asignar"`
	// GastoEnRepartosInvalidos es lo que NO se repartió por no sumar 100 %.
	GastoEnRepartosInvalidos string `json:"gasto_en_repartos_invalidos"`

	Bolsas    []BolsaDeDriver  `json:"bolsas"`
	Problemas []RepartoDeGasto `json:"problemas"`
}

// cien es el total que tiene que dar un reparto.
var cien = decimal.NewFromInt(100)

// toleranciaPct absorbe el redondeo de porcentajes con decimales (33,33 × 3 = 99,99). Sin ella,
// un reparto correcto en tres partes quedaría marcado como inválido para siempre.
var toleranciaPct = decimal.RequireFromString("0.05")

// ValidarReparto dice si los porcentajes de una clasificación suman 100 %.
//
// Devuelve la suma y el motivo cuando no cierra, porque «no suma 100» sin decir cuánto suma
// obliga a sumarlo a mano para saber si falta o sobra.
func ValidarReparto(lineas []LineaDeReparto) (decimal.Decimal, bool, string) {
	suma := decimal.Zero
	for _, l := range lineas {
		p, err := decimal.NewFromString(l.Porcentaje)
		if err != nil {
			return suma, false, "hay un porcentaje ilegible"
		}
		suma = suma.Add(p)
	}
	if len(lineas) == 0 {
		return suma, false, "sin repartir"
	}
	dif := suma.Sub(cien).Abs()
	if dif.LessThanOrEqual(toleranciaPct) {
		return suma, true, ""
	}
	if suma.LessThan(cien) {
		return suma, false, "falta " + cien.Sub(suma).StringFixed(2) + " % por repartir"
	}
	return suma, false, "sobra " + suma.Sub(cien).StringFixed(2) + " %: se absorbería más gasto del que existe"
}

// IndirectoDelPlan es lo que un plan absorbe de cada driver.
type IndirectoDelPlan struct {
	DriverID      string  `json:"driver_id"`
	Driver        string  `json:"driver"`
	Unidad        string  `json:"unidad"`
	Consumo       string  `json:"consumo"`
	CostoUnitario *string `json:"costo_unitario"`
	Absorbido     *string `json:"absorbido"`
	Motivo        string  `json:"motivo"`
}

// AbsorcionDelPlan es el indirecto total que le toca a un plan.
type AbsorcionDelPlan struct {
	PlanID string             `json:"plan_id"`
	Plan   string             `json:"plan"`
	Lineas []IndirectoDelPlan `json:"lineas"`
	Total  string             `json:"total"`
	// Completo es falso cuando algún driver que el plan consume no tiene costo unitario. Ahí el
	// total es PARCIAL, y publicarlo como si fuera el indirecto completo repetiría el defecto que
	// el módulo entero existe para evitar.
	Completo       bool     `json:"completo"`
	DriversSinBase []string `json:"drivers_sin_base"`
}

// Absorber calcula lo que un plan toma de cada bolsa.
func Absorber(planID, plan string, consumos []IndirectoDelPlan, bolsas []BolsaDeDriver) AbsorcionDelPlan {
	unitario := map[string]*string{}
	motivo := map[string]string{}
	for _, b := range bolsas {
		unitario[b.DriverID] = b.CostoUnitario
		motivo[b.DriverID] = b.Motivo
	}

	a := AbsorcionDelPlan{PlanID: planID, Plan: plan, Completo: true, DriversSinBase: []string{}, Lineas: []IndirectoDelPlan{}}
	total := decimal.Zero
	for _, c := range consumos {
		cu, hay := unitario[c.DriverID]
		if !hay || cu == nil {
			c.Motivo = motivo[c.DriverID]
			if c.Motivo == "" {
				c.Motivo = "el driver no tiene costo unitario en el período"
			}
			a.Completo = false
			a.DriversSinBase = append(a.DriversSinBase, c.Driver)
			a.Lineas = append(a.Lineas, c)
			continue
		}
		q, err1 := decimal.NewFromString(c.Consumo)
		u, err2 := decimal.NewFromString(*cu)
		if err1 != nil || err2 != nil {
			c.Motivo = "consumo o costo unitario ilegible"
			a.Completo = false
			a.Lineas = append(a.Lineas, c)
			continue
		}
		abs := q.Mul(u).Round(2)
		s := abs.StringFixed(2)
		c.CostoUnitario, c.Absorbido = cu, &s
		total = total.Add(abs)
		a.Lineas = append(a.Lineas, c)
	}
	a.Total = total.StringFixed(2)
	return a
}

// DriverInput es lo que llega para crear o editar un driver.
type DriverInput struct {
	Codigo      string `json:"codigo"`
	Nombre      string `json:"nombre"`
	Unidad      string `json:"unidad"`
	Descripcion string `json:"descripcion"`
	Activo      *bool  `json:"activo"`
}

// Normalizar limpia y valida.
func (in *DriverInput) Normalizar() error {
	in.Codigo = strings.TrimSpace(in.Codigo)
	in.Nombre = strings.TrimSpace(in.Nombre)
	in.Unidad = strings.TrimSpace(in.Unidad)
	in.Descripcion = strings.TrimSpace(in.Descripcion)
	if in.Codigo == "" {
		return ErrCodigoVacio
	}
	if in.Nombre == "" {
		return ErrNombreVacio
	}
	if in.Unidad == "" {
		in.Unidad = "UNIDAD"
	}
	return nil
}
