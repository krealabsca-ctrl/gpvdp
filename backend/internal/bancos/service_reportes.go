package bancos

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// CuadreRow es una fila del cuadre por concepto (RN-21).
type CuadreRow struct {
	ConceptoID    string `json:"concepto_id"`
	Concepto      string `json:"concepto"`
	TotalCreditos string `json:"total_creditos"`
	TotalDebitos  string `json:"total_debitos"`
}

// Comparativo son los totales del período anterior.
type Comparativo struct {
	PeriodoAnterior string `json:"periodo_anterior"`
	Ingresos        string `json:"ingresos"`
	Gastos          string `json:"gastos"`
	EBITDA          string `json:"ebitda"`
}

// DashboardData son los KPIs del período (§13). EBITDA = Ingresos − Gastos (decisión del DF).
//
// Ingresos y Gastos son SOLO lo que el usuario declaró como tal en la naturaleza del concepto
// (ver naturaleza.go). Lo demás —ahorro, reservas, préstamos, aportes entre empresas, lo que
// todavía no está clasificado— queda fuera y se informa aparte en vez de sumarse en silencio.
type DashboardData struct {
	Periodo         string      `json:"periodo"`
	Ingresos        string      `json:"ingresos"`
	Gastos          string      `json:"gastos"`
	EBITDA          string      `json:"ebitda"`
	NoIdentificados int         `json:"no_identificados"`
	Comparativo     Comparativo `json:"comparativo"`
	// FueraDelEbitda: cuánto y cuántos movimientos NO entraron porque su concepto está en NEUTRO o
	// no tienen concepto. Es el aviso que evita que un concepto sin declarar deje el número corto
	// sin que nadie se entere.
	FueraDelEbitda       string `json:"fuera_del_ebitda"`
	MovsFueraDelEbitda   int    `json:"movs_fuera_del_ebitda"`
	ConceptosSinDeclarar int    `json:"conceptos_sin_declarar"`
}

// Cuadre agrupa créditos/débitos por concepto en el período.
func (s *Service) Cuadre(ctx context.Context, empresaID, periodo string) ([]CuadreRow, error) {
	return s.repo.Cuadre(ctx, empresaID, periodo)
}

// Dashboard calcula Ingresos/Gastos/EBITDA del período y el comparativo con el anterior.
// Los traslados/overnight emparejados (es_traslado) se excluyen (§13).
func (s *Service) Dashboard(ctx context.Context, empresaID, periodo string) (DashboardData, error) {
	t, err := s.repo.TotalesPeriodo(ctx, empresaID, periodo)
	if err != nil {
		return DashboardData{}, err
	}
	prev := periodoAnterior(periodo)
	p, err := s.repo.TotalesPeriodo(ctx, empresaID, prev)
	if err != nil {
		return DashboardData{}, err
	}
	// Cuántos conceptos EN USO siguen sin declarar su naturaleza: es la acción concreta que hay que
	// hacer para que el número esté completo, así que se cuenta y se muestra.
	sinDeclarar, err := s.repo.ConceptosSinNaturaleza(ctx, empresaID)
	if err != nil {
		return DashboardData{}, err
	}
	return DashboardData{
		Periodo:         periodo,
		Ingresos:        t.Ingresos.String(),
		Gastos:          t.Gastos.String(),
		EBITDA:          t.Ingresos.Sub(t.Gastos).String(),
		NoIdentificados: t.NoIdentificados,
		Comparativo: Comparativo{
			PeriodoAnterior: prev,
			Ingresos:        p.Ingresos.String(),
			Gastos:          p.Gastos.String(),
			EBITDA:          p.Ingresos.Sub(p.Gastos).String(),
		},
		FueraDelEbitda:       t.MontoFueraEbitda.String(),
		MovsFueraDelEbitda:   t.MovsFueraEbitda,
		ConceptosSinDeclarar: sinDeclarar,
	}, nil
}

// periodoAnterior devuelve el período (YYYY-MM) previo.
func periodoAnterior(periodo string) string {
	parts := strings.SplitN(periodo, "-", 2)
	if len(parts) != 2 {
		return ""
	}
	anio, err1 := strconv.Atoi(parts[0])
	mes, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return ""
	}
	mes--
	if mes < 1 {
		mes = 12
		anio--
	}
	return fmt.Sprintf("%04d-%02d", anio, mes)
}
