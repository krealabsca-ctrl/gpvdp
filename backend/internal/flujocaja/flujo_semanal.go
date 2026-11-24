package flujocaja

// FLUJO DE CAJA A 13 SEMANAS — el saldo proyectado del banco, semana por semana.
//
// ── QUÉ CONTESTA Y POR QUÉ NO LO CONTESTABA NADA ────────────────────────────────────────────
//
// «¿Cuánto voy a tener en el banco el viernes 14?» y «¿en qué semana me quedo corto?».
//
// El flujo proyectado que ya existía da el NETO de cada mes —cuánto entra y cuánto sale— y eso
// no es lo mismo: un mes puede cerrar positivo y aun así dejar la cuenta en rojo el día 12. Lo
// que decide si se puede pagar la planilla es el SALDO en una fecha, no el neto del período.
//
// ── POR QUÉ NO SE PROYECTA POR FECHA DE VENCIMIENTO ─────────────────────────────────────────
//
// Porque en esta empresa 4.465 de 4.469 facturas abiertas ya vencieron. Repartir por vencimiento
// pondría ₡936 millones en la semana 1 y cero en las otras doce: un gráfico que dice «el lunes
// te quedás sin plata», que es falso, y que nadie volvería a abrir.
//
// Lo que manda es el COMPORTAMIENTO: cuánto se paga de atrasos por semana (la política que el
// Director Financiero declara) y cuánto se cobra de verdad (la tasa medida del histórico).
//
// ── LAS TRES CAPAS, Y POR QUÉ VAN SEPARADAS ─────────────────────────────────────────────────
//
//	COMPROMETIDO — hay un documento con fecha: planilla, CxP programada, cargos ya generados.
//	               No se negocia.
//	ESPERADO     — cuota de los contratos activos por su tasa de cobro real. Es probable, no
//	               seguro.
//	REPRESA      — la cartera vencida, desagotada al ritmo que fija la política.
//
// Apiladas en una sola barra se vería un número; separadas se ve CUÁL de los tres hay que mover
// para que la semana 7 no quede en rojo. Esa es toda la diferencia entre un gráfico y una
// herramienta.

import (
	"time"

	"github.com/shopspring/decimal"
)

// SemanasHorizonte es el estándar de tesorería corporativa. No es arbitrario: trece semanas son
// un trimestre, el plazo en el que una decisión de cobro o de pago todavía alcanza a cambiar el
// resultado. Más allá, la incertidumbre supera a la precisión.
const SemanasHorizonte = 13

// Modos de la política de pago de atrasos.
const (
	TopeMontoFijo   = "MONTO_FIJO"
	TopePctIngresos = "PCT_INGRESOS"
)

// PoliticaCaja es la decisión de tesorería que hace proyectable la cartera atrasada.
type PoliticaCaja struct {
	Modo            string `json:"modo"`
	TopeSemanal     string `json:"tope_semanal"`
	TopePct         string `json:"tope_pct"`
	MinimoOperativo string `json:"minimo_operativo"`
	Moneda          string `json:"moneda"`
	Nota            string `json:"nota"`
	// Definida distingue «no hay política» de «la política es cero». Sin ella, una empresa que
	// todavía no declaró nada vería un flujo que afirma que no va a pagar ni un colón de atrasos.
	Definida bool `json:"definida"`
}

// MovimientoEsperado es una entrada o salida con fecha, antes de agruparse en semanas.
type MovimientoEsperado struct {
	Fecha   time.Time
	Monto   decimal.Decimal // siempre positivo; el signo lo da Entrada
	Origen  string          // PLANILLA | CXP_PROGRAMADA | CXP_ATRASO | CXC_CARGO | CXC_CUOTA
	Capa    string          // COMPROMETIDO | ESPERADO | REPRESA
	Entrada bool
}

// Capas del flujo.
const (
	CapaComprometido = "COMPROMETIDO"
	CapaEsperado     = "ESPERADO"
	CapaRepresa      = "REPRESA"
)

// Orígenes de cada línea. Se publican porque la pantalla deja abrir cada semana por origen: un
// déficit causado por la planilla y uno causado por proveedores se resuelven distinto.
const (
	OrigenPlanilla      = "PLANILLA"
	OrigenCxPProgramada = "CXP_PROGRAMADA"
	OrigenCxPAtraso     = "CXP_ATRASO"
	OrigenCxCCargo      = "CXC_CARGO"
	OrigenCxCCuota      = "CXC_CUOTA"
)

// SemanaCaja es una semana del horizonte con su saldo de cierre.
type SemanaCaja struct {
	Numero int    `json:"numero"` // 1..13
	Desde  string `json:"desde"`  // lunes
	Hasta  string `json:"hasta"`  // domingo

	SaldoInicial string `json:"saldo_inicial"`

	// Entradas y salidas, desglosadas por capa. El total es la suma de las tres.
	EntradasComprometidas string `json:"entradas_comprometidas"`
	EntradasEsperadas     string `json:"entradas_esperadas"`
	Entradas              string `json:"entradas"`

	SalidasComprometidas string `json:"salidas_comprometidas"`
	SalidasRepresa       string `json:"salidas_represa"`
	Salidas              string `json:"salidas"`

	Neto string `json:"neto"`
	// SaldoFinal es EL dato de la fila. Todo lo demás existe para explicarlo.
	SaldoFinal string `json:"saldo_final"`

	// BajoMinimo marca que esta semana cierra por debajo del mínimo operativo declarado.
	BajoMinimo bool `json:"bajo_minimo"`
	// EnRojo: el saldo final es negativo. Peor que bajo el mínimo, y vale la pena distinguirlo.
	EnRojo bool `json:"en_rojo"`

	// PorOrigen permite abrir la semana sin otra consulta.
	PorOrigen []AporteOrigen `json:"por_origen"`
}

// AporteOrigen es cuánto puso cada fuente en una semana.
type AporteOrigen struct {
	Origen  string `json:"origen"`
	Capa    string `json:"capa"`
	Monto   string `json:"monto"`
	Entrada bool   `json:"entrada"`
}

// FlujoSemanal es la proyección completa de una moneda.
type FlujoSemanal struct {
	Moneda string `json:"moneda"`
	Desde  string `json:"desde"`
	Hasta  string `json:"hasta"`

	SaldoInicial string `json:"saldo_inicial"`
	// FuenteSaldo dice de dónde salió el punto de partida: DECLARADO (lo capturó la tesorera) o
	// DERIVADO (sumado de los movimientos cargados). Un flujo que arranca mal está mal entero, y
	// esa diferencia no se ve mirando la curva.
	FuenteSaldo string `json:"fuente_saldo"`
	FechaSaldo  string `json:"fecha_saldo"`

	Semanas []SemanaCaja `json:"semanas"`

	// PrimeraSemanaBajoMinimo y PrimeraSemanaEnRojo: 0 si nunca pasa. Es lo que se mira primero.
	PrimeraSemanaBajoMinimo int `json:"primera_semana_bajo_minimo"`
	PrimeraSemanaEnRojo     int `json:"primera_semana_en_rojo"`

	SaldoFinalHorizonte string `json:"saldo_final_horizonte"`
	// PuntoMasBajo y en qué semana ocurre: el saldo mínimo del horizonte no siempre es el final.
	PuntoMasBajo       string `json:"punto_mas_bajo"`
	SemanaPuntoMasBajo int    `json:"semana_punto_mas_bajo"`

	Politica PoliticaCaja `json:"politica"`

	// RepresaTotal es la cartera atrasada completa, y RepresaDesagotada lo que cabe en el
	// horizonte según la política. La diferencia es lo que NO se paga en trece semanas, y es un
	// número que conviene ver: con un tope muy bajo, la represa no baja nunca.
	RepresaTotal      string `json:"represa_total"`
	RepresaDesagotada string `json:"represa_desagotada"`
	RepresaRemanente  string `json:"represa_remanente"`
	// SemanasParaVaciar estima cuántas semanas tomaría agotar la represa al ritmo actual. 0 si
	// no aplica, -1 si el tope es cero y por lo tanto nunca se vacía.
	SemanasParaVaciar int `json:"semanas_para_vaciar"`

	// TasaCobro es la proporción histórica de cobro aplicada a las cuotas. Se publica porque un
	// forecast de entrada que no dice qué supuso no se puede discutir.
	TasaCobro        string `json:"tasa_cobro"`
	TasaCobroMedida  bool   `json:"tasa_cobro_medida"`
	ContratosActivos int    `json:"contratos_activos"`

	// Cuántas cuentas sostienen el saldo de partida y cuántas quedaron fuera. Una cuenta sin
	// saldo capturado no entra como cero: entra como desconocida, y acá se dice cuántas son.
	CuentasConSaldo int `json:"cuentas_con_saldo"`
	CuentasSinSaldo int `json:"cuentas_sin_saldo"`

	Avisos []string `json:"avisos"`
}

// lunesDe devuelve el lunes de la semana de t. La semana de tesorería arranca el lunes porque es
// cuando se decide a quién se le paga.
func lunesDe(t time.Time) time.Time {
	dif := (int(t.Weekday()) + 6) % 7 // domingo=0 -> 6; lunes=1 -> 0
	return time.Date(t.Year(), t.Month(), t.Day()-dif, 0, 0, 0, 0, time.UTC)
}

// semanasDesde arma las N semanas del horizonte a partir del lunes de la fecha dada.
func semanasDesde(hoy time.Time, n int) []SemanaCaja {
	inicio := lunesDe(hoy)
	out := make([]SemanaCaja, 0, n)
	for i := 0; i < n; i++ {
		desde := inicio.AddDate(0, 0, i*7)
		out = append(out, SemanaCaja{
			Numero: i + 1,
			Desde:  desde.Format("2006-01-02"),
			Hasta:  desde.AddDate(0, 0, 6).Format("2006-01-02"),
		})
	}
	return out
}

// indiceDeSemana ubica una fecha en el horizonte. Devuelve -1 si cae fuera.
//
// LO QUE YA VENCIÓ NO SE DESCARTA: cae antes de la semana 1 y el llamador lo manda a la represa.
// Tratarlo como fuera del horizonte perdería ₡936 millones sin avisar.
func indiceDeSemana(fecha, lunesUno time.Time, n int) int {
	dias := int(fecha.Sub(lunesUno).Hours() / 24)
	if dias < 0 {
		return -1
	}
	i := dias / 7
	if i >= n {
		return -1
	}
	return i
}

// topeDeLaSemana calcula cuánto se puede destinar a atrasos esa semana según la política.
//
// En PCT_INGRESOS el tope depende de lo que se espera cobrar: es como se desagota de verdad
// cuando la entrada es irregular —se paga con lo que entra, no con lo que se desearía pagar—.
func topeDeLaSemana(p PoliticaCaja, entradasDeLaSemana decimal.Decimal) decimal.Decimal {
	if !p.Definida {
		return decimal.Zero
	}
	switch p.Modo {
	case TopePctIngresos:
		pct, err := decimal.NewFromString(p.TopePct)
		if err != nil || !pct.IsPositive() {
			return decimal.Zero
		}
		if entradasDeLaSemana.IsNegative() {
			return decimal.Zero
		}
		return entradasDeLaSemana.Mul(pct).Div(decimal.NewFromInt(100))
	default:
		tope, err := decimal.NewFromString(p.TopeSemanal)
		if err != nil || tope.IsNegative() {
			return decimal.Zero
		}
		return tope
	}
}

// ConstruirFlujoSemanal arma la proyección completa. Es PURA: recibe todo medido y no consulta
// nada, que es lo que la hace falsificable sin base de datos.
//
// `movs` son los comprometidos y esperados con su fecha. `represa` es la cartera atrasada total.
func ConstruirFlujoSemanal(
	hoy time.Time,
	moneda string,
	saldoInicial decimal.Decimal,
	fuenteSaldo, fechaSaldo string,
	movs []MovimientoEsperado,
	represa decimal.Decimal,
	pol PoliticaCaja,
) FlujoSemanal {
	semanas := semanasDesde(hoy, SemanasHorizonte)
	lunesUno, _ := time.Parse("2006-01-02", semanas[0].Desde)

	f := FlujoSemanal{
		Moneda:       moneda,
		Desde:        semanas[0].Desde,
		Hasta:        semanas[len(semanas)-1].Hasta,
		SaldoInicial: saldoInicial.StringFixed(2),
		FuenteSaldo:  fuenteSaldo,
		FechaSaldo:   fechaSaldo,
		Politica:     pol,
		RepresaTotal: represa.StringFixed(2),
	}

	// Acumuladores por semana. Se usan índices y no mapas para no depender del orden de
	// iteración al calcular el saldo arrastrado.
	entCom := make([]decimal.Decimal, SemanasHorizonte)
	entEsp := make([]decimal.Decimal, SemanasHorizonte)
	salCom := make([]decimal.Decimal, SemanasHorizonte)
	porOrigen := make([]map[string]*AporteOrigen, SemanasHorizonte)
	for i := range porOrigen {
		entCom[i], entEsp[i], salCom[i] = decimal.Zero, decimal.Zero, decimal.Zero
		porOrigen[i] = map[string]*AporteOrigen{}
	}

	vencidoSinFecha := decimal.Zero
	for _, m := range movs {
		i := indiceDeSemana(m.Fecha, lunesUno, SemanasHorizonte)
		if i < 0 {
			// Antes del horizonte: es atraso, y se suma a la represa en vez de perderse. Después
			// del horizonte: no es asunto de estas trece semanas.
			if m.Fecha.Before(lunesUno) && !m.Entrada {
				vencidoSinFecha = vencidoSinFecha.Add(m.Monto)
			}
			continue
		}
		switch {
		case m.Entrada && m.Capa == CapaComprometido:
			entCom[i] = entCom[i].Add(m.Monto)
		case m.Entrada:
			entEsp[i] = entEsp[i].Add(m.Monto)
		default:
			salCom[i] = salCom[i].Add(m.Monto)
		}

		clave := m.Origen + "|" + m.Capa
		if a, ok := porOrigen[i][clave]; ok {
			monto, _ := decimal.NewFromString(a.Monto)
			a.Monto = monto.Add(m.Monto).StringFixed(2)
		} else {
			porOrigen[i][clave] = &AporteOrigen{
				Origen: m.Origen, Capa: m.Capa,
				Monto: m.Monto.StringFixed(2), Entrada: m.Entrada,
			}
		}
	}
	represaPendiente := represa.Add(vencidoSinFecha)
	f.RepresaTotal = represaPendiente.StringFixed(2)

	minimo, err := decimal.NewFromString(pol.MinimoOperativo)
	if err != nil {
		minimo = decimal.Zero
	}

	saldo := saldoInicial
	desagotado := decimal.Zero
	// EL PUNTO MÁS BAJO SE MIDE SOBRE LAS SEMANAS, no incluyendo el saldo de hoy. La pregunta es
	// «¿en qué semana voy a estar peor?», y arrancar la comparación en el saldo inicial devolvía
	// «semana 0» cuando nada bajaba de él —una semana que no existe en la tabla—.
	masBajo := decimal.Zero
	semanaMasBaja := 0

	for i := range semanas {
		entradas := entCom[i].Add(entEsp[i])

		// La represa se desagota DESPUÉS de conocer las entradas de la semana, porque en modo
		// PCT_INGRESOS el tope depende de ellas.
		pago := decimal.Zero
		if represaPendiente.IsPositive() {
			pago = topeDeLaSemana(pol, entradas)
			if pago.GreaterThan(represaPendiente) {
				pago = represaPendiente
			}
			if pago.IsPositive() {
				represaPendiente = represaPendiente.Sub(pago)
				desagotado = desagotado.Add(pago)
				porOrigen[i][OrigenCxPAtraso+"|"+CapaRepresa] = &AporteOrigen{
					Origen: OrigenCxPAtraso, Capa: CapaRepresa,
					Monto: pago.StringFixed(2), Entrada: false,
				}
			}
		}

		salidas := salCom[i].Add(pago)
		neto := entradas.Sub(salidas)
		saldo = saldo.Add(neto)

		s := &semanas[i]
		s.SaldoInicial = saldo.Sub(neto).StringFixed(2)
		s.EntradasComprometidas = entCom[i].StringFixed(2)
		s.EntradasEsperadas = entEsp[i].StringFixed(2)
		s.Entradas = entradas.StringFixed(2)
		s.SalidasComprometidas = salCom[i].StringFixed(2)
		s.SalidasRepresa = pago.StringFixed(2)
		s.Salidas = salidas.StringFixed(2)
		s.Neto = neto.StringFixed(2)
		s.SaldoFinal = saldo.StringFixed(2)
		s.EnRojo = saldo.IsNegative()
		s.BajoMinimo = saldo.LessThan(minimo)
		s.PorOrigen = ordenarAportes(porOrigen[i])

		if s.EnRojo && f.PrimeraSemanaEnRojo == 0 {
			f.PrimeraSemanaEnRojo = s.Numero
		}
		if s.BajoMinimo && f.PrimeraSemanaBajoMinimo == 0 {
			f.PrimeraSemanaBajoMinimo = s.Numero
		}
		if semanaMasBaja == 0 || saldo.LessThan(masBajo) {
			masBajo, semanaMasBaja = saldo, s.Numero
		}
	}

	f.Semanas = semanas
	f.SaldoFinalHorizonte = saldo.StringFixed(2)
	f.PuntoMasBajo = masBajo.StringFixed(2)
	f.SemanaPuntoMasBajo = semanaMasBaja
	f.RepresaDesagotada = desagotado.StringFixed(2)
	f.RepresaRemanente = represaPendiente.StringFixed(2)
	f.SemanasParaVaciar = semanasParaVaciar(represa.Add(vencidoSinFecha), desagotado)
	return f
}

// semanasParaVaciar estima a cuántas semanas está el fondo de la represa al ritmo del horizonte.
//
// -1 significa NUNCA: con un tope de cero la cartera atrasada no baja, y decir «0 semanas» se
// leería como «ya está pagada», que es lo contrario.
func semanasParaVaciar(total, desagotadoEnHorizonte decimal.Decimal) int {
	if !total.IsPositive() {
		return 0
	}
	if !desagotadoEnHorizonte.IsPositive() {
		return -1
	}
	ritmo := desagotadoEnHorizonte.Div(decimal.NewFromInt(SemanasHorizonte))
	n := total.Div(ritmo).Ceil().IntPart()
	if n > 9999 {
		return 9999
	}
	return int(n)
}

// ordenarAportes deja los aportes de una semana en orden estable: entradas primero, y dentro de
// cada grupo por origen. Un mapa iterado al azar haría que la misma semana se dibuje distinta en
// cada recarga.
func ordenarAportes(m map[string]*AporteOrigen) []AporteOrigen {
	orden := []string{
		OrigenCxCCargo + "|" + CapaComprometido,
		OrigenCxCCuota + "|" + CapaEsperado,
		OrigenPlanilla + "|" + CapaComprometido,
		OrigenCxPProgramada + "|" + CapaComprometido,
		OrigenCxPAtraso + "|" + CapaRepresa,
	}
	out := make([]AporteOrigen, 0, len(m))
	for _, k := range orden {
		if a, ok := m[k]; ok {
			out = append(out, *a)
		}
	}
	return out
}
