package ratios

// DRIVERS DEL NEGOCIO: la cartera por lo que la mueve, no por partida contable.
//
// ── POR QUÉ HACEN FALTA ─────────────────────────────────────────────────────────────────────
//
// Todo el análisis del sistema es por partida contable: cuánto entró por Datáfonos, cuánto por
// Asociaciones. Eso dice CÓMO entró la plata, no POR QUÉ. Sin drivers, presupuestar es «el año
// pasado más un diez por ciento», porque no hay con qué explicar de dónde saldría ese diez.
//
// El driver de este negocio es la CARTERA: 35.375 contratos de prepago, cada uno con su cuota,
// su tipo de servicio, su modalidad y su sede. Cuántos hay, cuánto paga cada uno y cómo se
// reparten es lo que de verdad determina el ingreso recurrente.
//
// ── LA DISTINCIÓN QUE SOSTIENE TODA LA PANTALLA ─────────────────────────────────────────────
//
// CONTRATADO ≠ FACTURADO. La suma de las cuotas de los contratos activos es lo que DEBERÍA
// entrar; lo que generó cargo este mes es lo que de verdad se puso a cobrar. Medido: ₡227,2
// millones contratados contra 10.468 de 35.375 contratos con cargo en el mes.
//
// Publicar solo el contratado daría un ingreso recurrente que no corresponde a nada, y se vería
// perfectamente razonable. Van los dos, y la brecha es el dato.
//
// ── LO QUE ESTE PAQUETE NO PUEDE MEDIR ──────────────────────────────────────────────────────
//
// El servicio PRESTADO —el funeral o la cremación ejecutada—, su ticket y su mix. La tabla
// `inv_servicio` existe con la estructura correcta y está vacía: nadie registra servicios. Sin
// eso no hay unit economics de la operación, solo de la cartera. Se dice en vez de aproximarlo
// con movimientos bancarios, que contaría depósitos y no servicios.

import "github.com/shopspring/decimal"

// Dimensiones por las que se puede abrir la cartera.
const (
	// PorTipoServicio es el MIX del negocio: CORPORATIVO, PLANES COOPEPROFA, SERVICIO FUNERARIO
	// A PLAZO, ESCAPADITAS…
	PorTipoServicio = "tipo_servicio"
	PorModalidad    = "modalidad"
	PorSede         = "sede"
)

// DimensionValida dice si se puede agrupar por ahí. Lista cerrada: el valor entra en la consulta.
func DimensionValida(d string) bool {
	switch d {
	case PorTipoServicio, PorModalidad, PorSede:
		return true
	}
	return false
}

// FilaDriver es un corte de la cartera.
type FilaDriver struct {
	Clave  string `json:"clave"`
	Nombre string `json:"nombre"`

	Contratos int `json:"contratos"`
	// CuotaContratada es la suma de las cuotas vigentes: lo que DEBERÍA entrar al mes.
	CuotaContratada string `json:"cuota_contratada"`
	// CuotaPromedio es el ticket recurrente de este corte. Es lo que distingue un plan caro de
	// uno barato, y lo que un «+10 %» tendría que mover.
	CuotaPromedio string `json:"cuota_promedio"`

	// ContratosFacturados y CuotaFacturada: los que de verdad generaron cargo en el mes medido.
	ContratosFacturados int    `json:"contratos_facturados"`
	CuotaFacturada      string `json:"cuota_facturada"`
	// CoberturaPct: qué proporción de los contratos de este corte facturó. Es el número que
	// explica la brecha entre lo contratado y lo facturado.
	CoberturaPct string `json:"cobertura_pct"`

	// AportePct sobre la cuota contratada total. Dice cuánto pesa este corte en el recurrente.
	AportePct string `json:"aporte_pct"`
}

// AltaMensual es cuántos contratos entraron en un mes.
type AltaMensual struct {
	Periodo       string `json:"periodo"`
	Altas         int    `json:"altas"`
	CuotaPromedio string `json:"cuota_promedio"`
	// CuotaIncorporada: cuánto recurrente mensual sumaron esas altas.
	CuotaIncorporada string `json:"cuota_incorporada"`
}

// DriversDeCartera es el panel completo.
type DriversDeCartera struct {
	Dimension string `json:"dimension"`
	// PeriodoFacturado es el mes contra el que se midió la cobertura.
	PeriodoFacturado string `json:"periodo_facturado"`

	ContratosActivos int    `json:"contratos_activos"`
	CuotaContratada  string `json:"cuota_contratada"`
	CuotaPromedio    string `json:"cuota_promedio"`

	ContratosFacturados int    `json:"contratos_facturados"`
	CuotaFacturada      string `json:"cuota_facturada"`
	CoberturaPct        string `json:"cobertura_pct"`

	Filas []FilaDriver  `json:"filas"`
	Altas []AltaMensual `json:"altas"`

	// ContratosSinFechaAlta: no se les puede medir la antigüedad ni contar como alta de ningún
	// mes. Son los que vinieron migrados del sistema anterior sin esa fecha.
	ContratosSinFechaAlta int      `json:"contratos_sin_fecha_alta"`
	Avisos                []string `json:"avisos"`
}

// porcentaje devuelve parte/total × 100 con un decimal, o "" si el total no es positivo.
//
// Vacío y no "0.0": un porcentaje sobre un total de cero no es cero por ciento, es una división
// que no se puede hacer — y mostrarlo como 0 % diría que ese corte no aporta nada.
func porcentaje(parte, total decimal.Decimal) string {
	if !total.IsPositive() {
		return ""
	}
	return parte.Div(total).Mul(decimal.NewFromInt(100)).Round(1).StringFixed(1)
}

// promedio devuelve total/n, o cero si no hay con qué dividir.
func promedio(total decimal.Decimal, n int) decimal.Decimal {
	if n <= 0 {
		return decimal.Zero
	}
	return total.Div(decimal.NewFromInt(int64(n))).Round(2)
}

// CompletarDrivers llena los derivados de cada fila y los avisos del panel.
//
// Se hace acá y no en SQL para que la regla de «sin total no hay porcentaje» viva en un solo
// lugar: repartida en las consultas, cada corte podría resolverla distinto.
func CompletarDrivers(d *DriversDeCartera) {
	totalContratada, err := decimal.NewFromString(d.CuotaContratada)
	if err != nil {
		totalContratada = decimal.Zero
	}
	for i := range d.Filas {
		f := &d.Filas[i]
		cuota, err := decimal.NewFromString(f.CuotaContratada)
		if err != nil {
			cuota = decimal.Zero
		}
		f.CuotaPromedio = promedio(cuota, f.Contratos).StringFixed(2)
		f.AportePct = porcentaje(cuota, totalContratada)
		f.CoberturaPct = porcentaje(
			decimal.NewFromInt(int64(f.ContratosFacturados)),
			decimal.NewFromInt(int64(f.Contratos)))
	}
	d.CuotaPromedio = promedio(totalContratada, d.ContratosActivos).StringFixed(2)
	d.CoberturaPct = porcentaje(
		decimal.NewFromInt(int64(d.ContratosFacturados)),
		decimal.NewFromInt(int64(d.ContratosActivos)))
	d.Avisos = avisosDeDrivers(*d)
}

func avisosDeDrivers(d DriversDeCartera) []string {
	var av []string

	// SIN CARTERA, UNA PANTALLA DE CEROS NO SE EXPLICA SOLA. Esta empresa puede no tener
	// contratos de prepago —el negocio de otra puede ser todo de contado— y el lector no tiene
	// cómo distinguir eso de una consulta que falló. Va primero y corta: los demás avisos hablan
	// de una cartera que no existe.
	if d.ContratosActivos == 0 {
		return []string{
			"Esta empresa no tiene contratos de cartera activos, así que no hay drivers que medir acá. " +
				"Los contratos de prepago viven en las empresas que los venden; probá cambiando la empresa activa.",
		}
	}

	// EL AVISO QUE MÁS IMPORTA: la brecha entre lo contratado y lo facturado. Sin él, la cuota
	// contratada se lee como el ingreso recurrente del mes y no lo es.
	if d.ContratosActivos > 0 && d.ContratosFacturados < d.ContratosActivos {
		av = append(av, "Lo CONTRATADO no es lo FACTURADO: de "+
			miles(d.ContratosActivos)+" contratos activos, "+miles(d.ContratosFacturados)+
			" generaron cargo en "+d.PeriodoFacturado+". La cuota contratada es lo que debería entrar al mes; "+
			"la facturada, lo que se puso a cobrar.")
	}
	if d.ContratosSinFechaAlta > 0 {
		av = append(av, miles(d.ContratosSinFechaAlta)+
			" contrato(s) no tienen fecha de inicio, así que no entran en las altas por mes ni se les puede medir la antigüedad.")
	}
	// La limitación de fondo, siempre visible: esto mide la CARTERA, no la operación.
	av = append(av, "Esto mide la cartera de contratos. El servicio PRESTADO —cuántos funerales o "+
		"cremaciones se ejecutaron, su ticket y su mix— no se registra todavía, así que no hay "+
		"unit economics de la operación.")
	return av
}

// miles pone separador de miles a un entero, para que los avisos se lean.
func miles(n int) string {
	s := decimal.NewFromInt(int64(n)).String()
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	return string(out)
}
