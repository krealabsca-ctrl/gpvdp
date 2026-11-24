package ratios

// La lógica de los ratios: juntar cartera y base, y —sobre todo— DECIR CUÁNDO EL NÚMERO NO
// SIGNIFICA LO QUE PARECE.
//
// Un DSO de 290 días puede querer decir «cobramos tardísimo» o «los cobros no se están
// registrando», y las dos cosas se ven idénticas mirando la cifra. Un ratio sin su advertencia es
// peor que ningún ratio: el primero se lleva a una junta.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ErrVentanaInvalida: la ventana son meses, y tiene que ser una cantidad razonable.
var ErrVentanaInvalida = errors.New("ratios: la ventana tiene que ser de 3, 6, 12 o 24 meses")

// ventanasValidas son las que el negocio pide. 12 es el estándar de reporte; 3 sirve para ver un
// cambio reciente; 24 para una cartera muy vieja que no cabe en un año.
var ventanasValidas = map[int]bool{3: true, 6: true, 12: true, 24: true}

// VentanaPorDefecto es la que usa cualquier reporte financiero cuando no se dice otra cosa.
const VentanaPorDefecto = 12

// Service calcula los ratios.
type Service struct {
	repo Repository
	log  *zap.Logger
	// ahora se inyecta para que una prueba pueda fijar el día: con time.Now() adentro, un test de
	// la ventana pasa o falla según el mes en que se corra.
	ahora func() time.Time
}

// NewService construye el servicio.
func NewService(repo Repository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log, ahora: time.Now}
}

// SetReloj fija el reloj. Solo para pruebas.
func (s *Service) SetReloj(f func() time.Time) { s.ahora = f }

// Ciclo calcula DSO, DPO y el ciclo de efectivo de la ventana pedida.
func (s *Service) Ciclo(ctx context.Context, empresaID string, meses int) (CicloDeEfectivo, error) {
	if meses == 0 {
		meses = VentanaPorDefecto
	}
	if !ventanasValidas[meses] {
		return CicloDeEfectivo{}, ErrVentanaInvalida
	}
	desde, hasta := ventana(s.ahora().UTC(), meses)

	porCobrar, cargosAbiertos, err := s.repo.CarteraPorCobrar(ctx, empresaID)
	if err != nil {
		return CicloDeEfectivo{}, err
	}
	facturado, err := s.repo.FacturacionPorMes(ctx, empresaID, desde, hasta)
	if err != nil {
		return CicloDeEfectivo{}, err
	}
	porPagar, facturasAbiertas, err := s.repo.CarteraPorPagar(ctx, empresaID)
	if err != nil {
		return CicloDeEfectivo{}, err
	}
	comprado, err := s.repo.ComprasPorMes(ctx, empresaID, desde, hasta)
	if err != nil {
		return CicloDeEfectivo{}, err
	}
	cobrosAplicados, err := s.repo.CobrosAplicados(ctx, empresaID)
	if err != nil {
		return CicloDeEfectivo{}, err
	}

	c := CicloDeEfectivo{
		Desde: desde, Hasta: hasta, Meses: meses,
		DSO: CalcularRatio(porCobrar, facturado, "facturación"),
		DPO: CalcularRatio(porPagar, comprado, "compras"),
		// El DIO no se calcula todavía: ver el comentario del repositorio.
		DIO: Ratio{Calculable: false, Aviso: "El inventario no tiene movimientos cargados, así que el DIO no se puede medir."},
	}
	c.Ciclo, c.CicloCalculable = CalcularCiclo(c.DSO, c.DPO, c.DIO)
	c.Avisos = avisosDelCiclo(c, cobrosAplicados, cargosAbiertos, facturasAbiertas)
	return c, nil
}

// avisosDelCiclo dice en voz alta lo que las cifras no muestran.
//
// EL ORDEN ES EL DE LA GRAVEDAD: primero lo que invalida un ratio entero, después lo que lo sesga,
// al final lo que falta. Quien lee tres avisos solo procesa el primero.
func avisosDelCiclo(c CicloDeEfectivo, cobrosAplicados, cargosAbiertos, facturasAbiertas int) []string {
	var av []string

	// EL AVISO QUE MÁS IMPORTA. Con cartera abierta y CERO cobros aplicados, el DSO no mide el
	// cobro: mide que los cobros no llegan al sistema. Sin esto, el número se lee como un
	// diagnóstico del negocio y es un diagnóstico del registro.
	if cobrosAplicados == 0 && cargosAbiertos > 0 {
		av = append(av, fmt.Sprintf(
			"EL DSO NO ES CONFIABLE todavía: hay %d cargos abiertos y NINGUNO con un cobro aplicado. "+
				"Mientras los cobros no se registren contra el cargo, la cartera nunca baja y el ratio "+
				"mide el registro, no la cobranza.", cargosAbiertos))
	}

	if !c.DSO.Calculable {
		av = append(av, "DSO: "+c.DSO.Aviso)
	} else if c.DSO.Aviso != "" {
		av = append(av, "DSO: "+c.DSO.Aviso)
	}
	if !c.DPO.Calculable {
		av = append(av, "DPO: "+c.DPO.Aviso)
	} else if c.DPO.Aviso != "" {
		av = append(av, "DPO: "+c.DPO.Aviso)
	}

	// El ciclo sin inventario no es el CCC de manual, y conviene decirlo antes de que alguien lo
	// compare contra el de otra empresa.
	if c.CicloCalculable {
		av = append(av, "El ciclo es DSO − DPO: le falta el inventario (DIO) para ser el ciclo de conversión completo.")
	}

	return av
}

// ── Drivers de la cartera ───────────────────────────────────────────────────────────────────

// ErrDimensionInvalida: la dimensión es una lista cerrada porque su valor entra en la consulta.
var ErrDimensionInvalida = errors.New("ratios: la dimensión tiene que ser tipo_servicio, modalidad o sede")

// Drivers arma el panel de la cartera por la dimensión pedida.
//
// `periodo` es el mes contra el que se mide la COBERTURA —cuántos contratos generaron cargo—, que
// es lo que explica la brecha entre lo contratado y lo facturado. Vacío = el mes anterior
// completo: el mes en curso todavía no terminó de generar sus cargos y mediría una cobertura
// artificialmente baja.
func (s *Service) Drivers(ctx context.Context, empresaID, dim, periodo string, mesesAltas int) (DriversDeCartera, error) {
	if dim == "" {
		dim = PorTipoServicio
	}
	if !DimensionValida(dim) {
		return DriversDeCartera{}, ErrDimensionInvalida
	}
	if periodo == "" {
		periodo = mesAnterior(s.ahora().UTC())
	}
	if mesesAltas <= 0 || mesesAltas > 36 {
		mesesAltas = 12
	}

	filas, err := s.repo.DriversPorDimension(ctx, empresaID, dim, periodo)
	if err != nil {
		return DriversDeCartera{}, err
	}
	activos, contratada, facturados, facturada, sinFecha, err := s.repo.TotalesDeCartera(ctx, empresaID, periodo)
	if err != nil {
		return DriversDeCartera{}, err
	}
	desde, hasta := ventanaDeMeses(s.ahora().UTC(), mesesAltas)
	altas, err := s.repo.AltasPorMes(ctx, empresaID, desde, hasta)
	if err != nil {
		return DriversDeCartera{}, err
	}

	d := DriversDeCartera{
		Dimension:             dim,
		PeriodoFacturado:      periodo,
		ContratosActivos:      activos,
		CuotaContratada:       contratada.StringFixed(2),
		ContratosFacturados:   facturados,
		CuotaFacturada:        facturada.StringFixed(2),
		Filas:                 filas,
		Altas:                 altas,
		ContratosSinFechaAlta: sinFecha,
	}
	CompletarDrivers(&d)
	return d, nil
}

// mesAnterior devuelve el YYYY-MM del mes pasado.
func mesAnterior(hoy time.Time) string {
	return time.Date(hoy.Year(), hoy.Month(), 1, 0, 0, 0, 0, time.UTC).
		AddDate(0, -1, 0).Format("2006-01")
}

// ventanaDeMeses devuelve los últimos n meses como YYYY-MM.
func ventanaDeMeses(hoy time.Time, n int) (string, string) {
	primero := time.Date(hoy.Year(), hoy.Month(), 1, 0, 0, 0, 0, time.UTC)
	return primero.AddDate(0, -(n - 1), 0).Format("2006-01"), primero.Format("2006-01")
}
