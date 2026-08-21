package bancos

// Fase B — análisis visual: serie mensual, calendario diario y resumen por cuenta.
// Misma semántica que el Dashboard (§13): ingresos/gastos EXCLUYEN traslados
// emparejados (es_traslado) y solo cuentan movimientos incluidos.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SerieMensualPunto es un mes de la tendencia (ingresos/gastos/EBITDA + pendientes).
type SerieMensualPunto struct {
	Periodo         string `json:"periodo"` // YYYY-MM
	Ingresos        string `json:"ingresos"`
	Gastos          string `json:"gastos"`
	EBITDA          string `json:"ebitda"`
	Movimientos     int    `json:"movimientos"`
	NoIdentificados int    `json:"no_identificados"`
}

// DiaCalendario es un día del calendario de flujo (créditos − débitos, sin traslados).
type DiaCalendario struct {
	Fecha       string `json:"fecha"` // YYYY-MM-DD
	Creditos    string `json:"creditos"`
	Debitos     string `json:"debitos"`
	Neto        string `json:"neto"`
	Movimientos int    `json:"movimientos"`
}

// CuentaResumen son los totales del período de una cuenta bancaria.
type CuentaResumen struct {
	CuentaID    string `json:"cuenta_id"`
	Banco       string `json:"banco"`
	Alias       string `json:"alias"`
	Creditos    string `json:"creditos"`
	Debitos     string `json:"debitos"`
	Movimientos int    `json:"movimientos"`
}

// SerieMensual devuelve los últimos `meses` períodos hasta `hasta` (inclusive),
// SIN huecos: los meses sin movimientos salen en cero para que la gráfica no salte.
func (s *Service) SerieMensual(ctx context.Context, empresaID, hasta string, meses int) ([]SerieMensualPunto, error) {
	if meses <= 0 || meses > 36 {
		meses = 12
	}
	if !esPeriodoValido(hasta) {
		hasta = time.Now().Format("2006-01")
	}
	periodos := periodosHasta(hasta, meses)
	desde := periodos[0]
	puntos, err := s.repo.SerieMensual(ctx, empresaID, desde, hasta)
	if err != nil {
		return nil, err
	}
	return rellenarSerie(puntos, periodos), nil
}

// CalendarioDiario devuelve los totales por día del período (solo días con movimientos).
func (s *Service) CalendarioDiario(ctx context.Context, empresaID, periodo string) ([]DiaCalendario, error) {
	return s.repo.CalendarioDiario(ctx, empresaID, periodo)
}

// ResumenPorCuenta devuelve créditos/débitos del período por cuenta bancaria.
func (s *Service) ResumenPorCuenta(ctx context.Context, empresaID, periodo string) ([]CuentaResumen, error) {
	return s.repo.ResumenPorCuenta(ctx, empresaID, periodo)
}

// periodosHasta genera n períodos YYYY-MM consecutivos que terminan en `hasta`.
func periodosHasta(hasta string, n int) []string {
	parts := strings.SplitN(hasta, "-", 2)
	anio, _ := strconv.Atoi(parts[0])
	mes, _ := strconv.Atoi(parts[1])
	out := make([]string, n)
	for i := n - 1; i >= 0; i-- {
		out[i] = fmt.Sprintf("%04d-%02d", anio, mes)
		mes--
		if mes < 1 {
			mes = 12
			anio--
		}
	}
	return out
}

// rellenarSerie alinea los puntos de la BD con la lista completa de períodos,
// completando con ceros los meses sin datos.
func rellenarSerie(puntos []SerieMensualPunto, periodos []string) []SerieMensualPunto {
	porPeriodo := make(map[string]SerieMensualPunto, len(puntos))
	for _, p := range puntos {
		porPeriodo[p.Periodo] = p
	}
	out := make([]SerieMensualPunto, 0, len(periodos))
	for _, per := range periodos {
		if p, ok := porPeriodo[per]; ok {
			out = append(out, p)
			continue
		}
		out = append(out, SerieMensualPunto{Periodo: per, Ingresos: "0", Gastos: "0", EBITDA: "0"})
	}
	return out
}

func esPeriodoValido(p string) bool {
	if len(p) != 7 || p[4] != '-' {
		return false
	}
	anio, err1 := strconv.Atoi(p[:4])
	mes, err2 := strconv.Atoi(p[5:])
	return err1 == nil && err2 == nil && anio >= 2000 && anio <= 2100 && mes >= 1 && mes <= 12
}
