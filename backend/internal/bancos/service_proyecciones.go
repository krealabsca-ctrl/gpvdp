package bancos

// Fase C — Proyecciones: el servicio arma el escenario (método pedido → método
// efectivo con respaldo RITMO), la meta, la senda para el gráfico y el desglose
// por línea de ingreso; y persiste escenarios con auditoría.

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// PuntoSenda es un punto (día, acumulado) para el gráfico de senda de cierre.
type PuntoSenda struct {
	Dia       int    `json:"dia"`
	Acumulado string `json:"acumulado"`
}

// LineaIngreso es el desglose por clasificación de ingreso del período.
type LineaIngreso struct {
	ClasificacionID string `json:"clasificacion_id"`
	Nombre          string `json:"nombre"`
	Real            string `json:"real"`
	Cierre          string `json:"cierre"`
	Meta            string `json:"meta"`
	Brecha          string `json:"brecha"`
}

// ProyeccionResultado es la respuesta completa para pintar la pantalla.
type ProyeccionResultado struct {
	Periodo            string         `json:"periodo"`
	DiasMes            int            `json:"dias_mes"`
	DiaCalculo         int            `json:"dia_calculo"`
	SinDatos           bool           `json:"sin_datos"`
	Metodo             string         `json:"metodo"`
	MetodoEfectivo     string         `json:"metodo_efectivo"`
	MesGemelo          string         `json:"mes_gemelo,omitempty"`
	MetodosDisponibles []string       `json:"metodos_disponibles"`
	RealAcumulado      string         `json:"real_acumulado"`
	CierreProyectado   string         `json:"cierre_proyectado"`
	MetaPct            string         `json:"meta_pct"`
	MetaMonto          string         `json:"meta_monto"`
	Brecha             string         `json:"brecha"`
	SendaReal          []PuntoSenda   `json:"senda_real"`
	SendaProyeccion    []PuntoSenda   `json:"senda_proyeccion"`
	PorLinea           []LineaIngreso `json:"por_linea"`
}

// EscenarioNuevo es la carga para persistir un escenario calculado.
type EscenarioNuevo struct {
	Periodo, Metodo, MetodoEfectivo string
	MetaPct                         decimal.Decimal
	LineasIngreso                   []string
	DiaCalculo                      int
	RealAcumulado                   decimal.Decimal
	CierreProyectado                decimal.Decimal
	MetaMonto                       decimal.Decimal
	CreadoPor                       string
}

// EscenarioGuardado es un escenario persistido, con el real del período para la precisión.
type EscenarioGuardado struct {
	ID               string `json:"id"`
	Periodo          string `json:"periodo"`
	Metodo           string `json:"metodo"`
	MetodoEfectivo   string `json:"metodo_efectivo"`
	MetaPct          string `json:"meta_pct"`
	DiaCalculo       int    `json:"dia_calculo"`
	RealAcumulado    string `json:"real_acumulado"`
	CierreProyectado string `json:"cierre_proyectado"`
	MetaMonto        string `json:"meta_monto"`
	RealCierre       string `json:"real_cierre"`
	CreadoEn         string `json:"creado_en"`
}

// CalcularProyeccion arma el escenario del período con el método pedido.
// Si el método necesita histórico y no lo hay, cae a RITMO (metodo_efectivo).
func (s *Service) CalcularProyeccion(ctx context.Context, empresaID, periodo, metodo string, metaPct decimal.Decimal) (ProyeccionResultado, error) {
	if !esPeriodoValido(periodo) {
		periodo = time.Now().Format("2006-01")
	}
	anio, mes := partesDePeriodo(periodo)
	diasMes := time.Date(anio, time.Month(mes)+1, 0, 0, 0, 0, 0, time.UTC).Day()

	actual, err := s.repo.SendaIngresos(ctx, empresaID, periodo)
	if err != nil {
		return ProyeccionResultado{}, err
	}
	res := ProyeccionResultado{
		Periodo: periodo, DiasMes: diasMes, Metodo: metodo,
		MetaPct:            metaPct.String(),
		MetodosDisponibles: []string{},
		RealAcumulado:      "0", CierreProyectado: "0", MetaMonto: "0", Brecha: "0",
		SendaReal: []PuntoSenda{}, SendaProyeccion: []PuntoSenda{}, PorLinea: []LineaIngreso{},
	}
	if len(actual) == 0 {
		res.SinDatos = true
		res.MetodoEfectivo = metodo
		return res, nil
	}

	diaCalculo := 0
	for _, dm := range actual {
		if dm.Dia > diaCalculo {
			diaCalculo = dm.Dia
		}
	}
	realAcum := acumuladoHasta(actual, diaCalculo)

	// Históricos: hasta 12 períodos previos (una sola query).
	previos := periodosPrevios(periodo, 12)
	sendas, err := s.repo.SendasIngresosRango(ctx, empresaID, previos)
	if err != nil {
		return ProyeccionResultado{}, err
	}
	porPeriodo := map[string]SendaMes{}
	for _, sm := range sendas {
		porPeriodo[sm.Periodo] = sm
	}
	mismoMesAnterior := porPeriodo[periodoMenosMeses(periodo, 12)]
	var mesesMismoAnio []SendaMes
	for _, p := range previos {
		if sm, ok := porPeriodo[p]; ok && strings.HasPrefix(p, periodo[:4]) {
			mesesMismoAnio = append(mesesMismoAnio, sm)
		}
	}

	// Disponibilidad + cálculo de cada método.
	cierres := map[string]decimal.Decimal{
		MetodoRitmo: proyectarRitmo(actual, diaCalculo, anio, mes, diasMes),
	}
	disponibles := []string{MetodoRitmo}
	if c, ok := proyectarHistorico(realAcum, mismoMesAnterior.Dias, diaCalculo); ok {
		cierres[MetodoHistorico] = c
		disponibles = append(disponibles, MetodoHistorico)
	}
	if c, ok := proyectarPromedio(realAcum, mesesMismoAnio, diaCalculo); ok {
		cierres[MetodoPromedio] = c
		disponibles = append(disponibles, MetodoPromedio)
	}
	gemelo := ""
	if c, g, ok := proyectarCoincidencia(actual, realAcum, sendas, diaCalculo); ok {
		cierres[MetodoCoincidencia] = c
		gemelo = g
		disponibles = append(disponibles, MetodoCoincidencia)
	}

	efectivo := metodo
	cierre, ok := cierres[metodo]
	if !ok {
		efectivo = MetodoRitmo
		cierre = cierres[MetodoRitmo]
	}

	// Meta sobre el cierre real del mes anterior.
	meta := metaCierre(totalMes(porPeriodo[periodoAnterior(periodo)].Dias), metaPct)

	res.DiaCalculo = diaCalculo
	res.MetodoEfectivo = efectivo
	if efectivo == MetodoCoincidencia {
		res.MesGemelo = gemelo
	}
	res.MetodosDisponibles = disponibles
	res.RealAcumulado = realAcum.String()
	res.CierreProyectado = cierre.String()
	res.MetaMonto = meta.String()
	res.Brecha = cierre.Sub(meta).String()
	res.SendaReal = sendaAcumulada(actual, diaCalculo)
	res.SendaProyeccion = sendaProyectada(realAcum, cierre, diaCalculo, diasMes, anio, mes)

	// Desglose por línea: participación real del período aplicada al cierre y la meta.
	lineas, err := s.repo.IngresosPorClasificacion(ctx, empresaID, periodo)
	if err != nil {
		return ProyeccionResultado{}, err
	}
	for _, l := range lineas {
		monto, _ := decimal.NewFromString(l.Real)
		if !realAcum.IsPositive() {
			continue
		}
		share := monto.Div(realAcum)
		cierreL := cierre.Mul(share)
		metaL := meta.Mul(share)
		l.Cierre = cierreL.String()
		l.Meta = metaL.String()
		l.Brecha = cierreL.Sub(metaL).String()
		res.PorLinea = append(res.PorLinea, l)
	}
	return res, nil
}

// GuardarEscenario calcula y persiste el escenario (spec: "Guardar escenario").
func (s *Service) GuardarEscenario(ctx context.Context, empresaID, periodo, metodo string, metaPct decimal.Decimal, lineas []string, usuarioID string) (ProyeccionResultado, string, error) {
	res, err := s.CalcularProyeccion(ctx, empresaID, periodo, metodo, metaPct)
	if err != nil {
		return ProyeccionResultado{}, "", err
	}
	if res.SinDatos {
		return res, "", nil
	}
	realAcum, _ := decimal.NewFromString(res.RealAcumulado)
	cierre, _ := decimal.NewFromString(res.CierreProyectado)
	meta, _ := decimal.NewFromString(res.MetaMonto)
	if lineas == nil {
		lineas = []string{}
	}
	id, err := s.repo.GuardarEscenario(ctx, empresaID, EscenarioNuevo{
		Periodo: res.Periodo, Metodo: metodo, MetodoEfectivo: res.MetodoEfectivo,
		MetaPct: metaPct, LineasIngreso: lineas, DiaCalculo: res.DiaCalculo,
		RealAcumulado: realAcum, CierreProyectado: cierre, MetaMonto: meta, CreadoPor: usuarioID,
	})
	if err != nil {
		return ProyeccionResultado{}, "", err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "proyeccion_escenario", EntidadID: &id,
		Accion: "GUARDAR_ESCENARIO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{
			"periodo": res.Periodo, "metodo": metodo, "metodo_efectivo": res.MetodoEfectivo,
			"meta_pct": metaPct.String(), "cierre_proyectado": res.CierreProyectado,
		},
	})
	return res, id, nil
}

// EscenariosGuardados lista los escenarios (con el real del período para la precisión).
func (s *Service) EscenariosGuardados(ctx context.Context, empresaID, periodo string) ([]EscenarioGuardado, error) {
	return s.repo.ListarEscenarios(ctx, empresaID, periodo)
}

// periodosPrevios devuelve los n períodos anteriores a `periodo` (más reciente primero).
func periodosPrevios(periodo string, n int) []string {
	out := make([]string, 0, n)
	p := periodo
	for i := 0; i < n; i++ {
		p = periodoAnterior(p)
		out = append(out, p)
	}
	return out
}

// periodoMenosMeses resta n meses a un período YYYY-MM.
func periodoMenosMeses(periodo string, n int) string {
	p := periodo
	for i := 0; i < n; i++ {
		p = periodoAnterior(p)
	}
	return p
}

// partesDePeriodo separa YYYY-MM en enteros (el período ya viene validado).
func partesDePeriodo(periodo string) (int, int) {
	parts := strings.SplitN(periodo, "-", 2)
	anio, _ := strconv.Atoi(parts[0])
	mes, _ := strconv.Atoi(parts[1])
	return anio, mes
}
