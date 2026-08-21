package bancos

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// Cotizacion es una cotización puntual del TC (día 1 / 15 / último) — RN-10.
type Cotizacion struct {
	Fecha  string `json:"fecha"` // YYYY-MM-DD
	Valor  string `json:"valor"`
	Fuente string `json:"fuente"` // BCCR | MANUAL
}

// TCMes es el estado del tipo de cambio de un mes.
type TCMes struct {
	Anio           int          `json:"anio"`
	Mes            int          `json:"mes"`
	Estado         string       `json:"estado"` // PROVISIONAL | CONGELADO
	ValorCongelado *string      `json:"valor_congelado"`
	Cotizaciones   []Cotizacion `json:"cotizaciones"`
}

// EstadoMes devuelve el estado del TC del mes con sus cotizaciones.
func (s *Service) EstadoMes(ctx context.Context, empresaID string, anio, mes int) (TCMes, error) {
	estado, valorCong, err := s.repo.EstadoTCMes(ctx, empresaID, anio, mes)
	if err != nil {
		return TCMes{}, err
	}
	if estado == "" {
		estado = "PROVISIONAL"
	}
	cots, err := s.repo.CotizacionesMes(ctx, empresaID, anio, mes)
	if err != nil {
		return TCMes{}, err
	}
	if cots == nil {
		cots = []Cotizacion{}
	}
	return TCMes{Anio: anio, Mes: mes, Estado: estado, ValorCongelado: valorCong, Cotizaciones: cots}, nil
}

// RegistrarCotizacion guarda una cotización y recalcula el provisional del mes (si no está congelado).
func (s *Service) RegistrarCotizacion(ctx context.Context, empresaID, fecha string, valor decimal.Decimal, fuente string) error {
	if err := s.repo.UpsertCotizacion(ctx, empresaID, fecha, valor, fuente); err != nil {
		return err
	}
	anio, mes, ok := anioMes(fecha)
	if !ok {
		return nil
	}
	if estado, _, err := s.repo.EstadoTCMes(ctx, empresaID, anio, mes); err == nil && estado != "CONGELADO" {
		s.recalcularProvisional(ctx, empresaID, anio, mes)
	}
	return nil
}

// Congelar fija el TC del mes = promedio(día1, 15, último) y lo aplica a los USD del mes (RN-12/13).
func (s *Service) Congelar(ctx context.Context, empresaID string, anio, mes int, usuarioID string) (int, error) {
	cots, err := s.repo.CotizacionesMes(ctx, empresaID, anio, mes)
	if err != nil {
		return 0, err
	}
	d1, d15, dUlt, ok := extraerTres(cots)
	if !ok {
		return 0, ErrCotizacionesIncompletas
	}
	valor := TCCongelado(d1, d15, dUlt)
	n, err := s.repo.CongelarTC(ctx, empresaID, anio, mes, valor)
	if err != nil {
		return 0, err
	}
	empID := empresaID
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empID, Entidad: "tipo_cambio_mes", Accion: "CONGELAR_TC", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{"anio": anio, "mes": mes, "valor": valor.String(), "movimientos": n},
	})
	return n, nil
}

// AplicarTCImportado convierte a colones lo que se acaba de importar en una cuenta que no es en
// colones.
//
// Por qué hace falta: hasta ahora la conversión se disparaba SOLO al tocar el tipo de cambio
// (registrar una cotización o congelar el mes). Si los movimientos entraban DESPUÉS de eso,
// nadie los convertía y se quedaban con monto_crc = 0 para siempre. Pasó exactamente así: la
// cotización del 1 de agosto se registró a las 16:54 y el estado de cuenta se importó a las
// 16:59 — cinco minutos después, y los 16 movimientos en dólares quedaron en cero.
//
// El criterio es el mismo de siempre (RN-11/RN-12) y no se reimplementa: si el mes está
// CONGELADO se aplica ese valor —es inmutable y va retroactivo a todo el mes—; si no, se usa el
// provisional escalonado. Un movimiento no puede aterrizar en cero cuando el TC de su mes ya
// se conoce.
func (s *Service) AplicarTCImportado(ctx context.Context, empresaID, moneda string, movs []MovimientoParaInsertar) {
	if moneda == "CRC" || len(movs) == 0 {
		return
	}
	// Los meses tocados por esta importación (normalmente uno, pero un archivo puede cruzar dos).
	type ym struct{ anio, mes int }
	meses := map[ym]bool{}
	for _, m := range movs {
		meses[ym{m.Fecha.Year(), int(m.Fecha.Month())}] = true
	}
	for k := range meses {
		estado, valorCong, err := s.repo.EstadoTCMes(ctx, empresaID, k.anio, k.mes)
		if err != nil {
			continue
		}
		if estado == "CONGELADO" && valorCong != nil {
			v, err := decimal.NewFromString(*valorCong)
			if err != nil {
				continue
			}
			// Mismo valor a las dos mitades del mes = aplicar el congelado a todo el mes.
			_, _ = s.repo.AplicarProvisional(ctx, empresaID, k.anio, k.mes, v, v)
			continue
		}
		s.recalcularProvisional(ctx, empresaID, k.anio, k.mes)
	}
}

// recalcularProvisional aplica el TC provisional escalonado a los USD no congelados del mes (RN-11).
func (s *Service) recalcularProvisional(ctx context.Context, empresaID string, anio, mes int) {
	cots, err := s.repo.CotizacionesMes(ctx, empresaID, anio, mes)
	if err != nil {
		return
	}
	d1, tieneD1 := valorDia(cots, 1)
	if !tieneD1 {
		return // sin día 1 no hay base provisional
	}
	d15, tieneD15 := valorDia(cots, 15)
	tcAntes15 := d1.Round(4)
	tcDesde15 := d1.Round(4)
	if tieneD15 {
		tcDesde15 = promedioDecimales(d1, d15).Round(4)
	}
	_, _ = s.repo.AplicarProvisional(ctx, empresaID, anio, mes, tcAntes15, tcDesde15)
}

// ---- helpers de cotizaciones ----

func anioMes(fecha string) (int, int, bool) {
	t, err := time.Parse("2006-01-02", fecha)
	if err != nil {
		return 0, 0, false
	}
	return t.Year(), int(t.Month()), true
}

func diaDe(fecha string) int {
	t, err := time.Parse("2006-01-02", fecha)
	if err != nil {
		return 0
	}
	return t.Day()
}

func valorDia(cots []Cotizacion, dia int) (decimal.Decimal, bool) {
	for _, c := range cots {
		if diaDe(c.Fecha) == dia {
			if v, err := decimal.NewFromString(c.Valor); err == nil {
				return v, true
			}
		}
	}
	return decimal.Zero, false
}

// extraerTres obtiene las cotizaciones de día 1, día 15 y último (mayor día > 15).
func extraerTres(cots []Cotizacion) (d1, d15, dUlt decimal.Decimal, ok bool) {
	v1, ok1 := valorDia(cots, 1)
	v15, ok15 := valorDia(cots, 15)
	maxDia := 0
	var vUlt decimal.Decimal
	okU := false
	for _, c := range cots {
		if d := diaDe(c.Fecha); d > 15 && d > maxDia {
			if v, err := decimal.NewFromString(c.Valor); err == nil {
				maxDia, vUlt, okU = d, v, true
			}
		}
	}
	if ok1 && ok15 && okU {
		return v1, v15, vUlt, true
	}
	return decimal.Zero, decimal.Zero, decimal.Zero, false
}
