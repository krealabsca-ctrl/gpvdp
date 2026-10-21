package bancos

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
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

// PromedioDelAnio son los totales promedio de los meses YA TRANSCURRIDOS del año en curso, cada uno
// medido hasta el mismo día del mes (pedido de Gerencia General, 19-oct-2026).
//
// No incluye el mes actual: compararlo parcialmente contra sí mismo diluye la señal justo en la
// cifra que se quiere leer.
type PromedioDelAnio struct {
	// Meses es sobre cuántos se promedió. 0 = enero, o un año sin datos previos: entonces no hay
	// promedio y la pantalla no debe inventar uno.
	Meses    int    `json:"meses"`
	Ingresos string `json:"ingresos"`
	Gastos   string `json:"gastos"`
	EBITDA   string `json:"ebitda"`
}

// CorteDelPeriodo dice HASTA QUÉ DÍA se midió todo lo que trae el tablero, y cuánto de eso tiene
// respaldo cargado.
//
// El corte es el día de HOY cuando el mes está en curso (decisión del Director Financiero: «sería
// al 19, porque si algo no se ha cargado se termine de cargar»). Esa decisión usa el tablero como
// empuje para mantener la carga al día, y para que funcione hace falta `UltimoDiaCargado`: sin él
// los días sin estado de cuenta entran como ceros y un atraso administrativo se lee igual que una
// caída del negocio.
type CorteDelPeriodo struct {
	// Dia es hasta qué día se sumó. 0 = el mes entero (mes ya cerrado).
	Dia int `json:"dia"`
	// EnCurso dice si el mes todavía no terminó, que es cuando la comparación va a paridad.
	EnCurso bool `json:"en_curso"`
	// UltimoDiaCargado es el día del último movimiento que hay en el período. Si es menor que
	// `Dia`, los días del medio están vacíos porque falta cargar, no porque no haya pasado nada.
	UltimoDiaCargado int `json:"ultimo_dia_cargado"`
	// DiasSinCargar es la diferencia, ya calculada para que la pantalla no la derive mal.
	DiasSinCargar int `json:"dias_sin_cargar"`
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
	// Corte y PromedioAnio: la comparación A PARIDAD DE DÍA que pidió Gerencia General. Cuando el
	// mes está en curso, `Comparativo` también sale cortado al mismo día, así que las dos cifras
	// miden tramos iguales.
	Corte        CorteDelPeriodo `json:"corte"`
	PromedioAnio PromedioDelAnio `json:"promedio_anio"`
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

// DiaDeCorte decide hasta qué día se mide un período, comparado contra la fecha de hoy.
//
// Un mes YA CERRADO se mide entero (devuelve 0): cortarlo por un día dejaría afuera plata que
// ocurrió. Un mes EN CURSO se corta al día de hoy —decisión del Director Financiero— y entonces el
// mes anterior y el promedio del año se cortan al mismo día, que es lo que vuelve comparables las
// tres cifras.
//
// Un mes FUTURO también da 0: no hay nada que cortar, y tratarlo como en curso pondría un día de
// corte mayor que cualquier dato.
//
// Función aparte para poder fijar el comportamiento sin base ni reloj.
func DiaDeCorte(periodo string, hoy time.Time) (dia int, enCurso bool) {
	if periodo != hoy.Format("2006-01") {
		return 0, false
	}
	return hoy.Day(), true
}

// mesesPreviosDelAnio lista los períodos del MISMO año anteriores al dado («2026-10» → enero a
// setiembre). Vacío en enero: no hay con qué promediar y no se inventa nada.
func mesesPreviosDelAnio(periodo string) []string {
	parts := strings.SplitN(periodo, "-", 2)
	if len(parts) != 2 {
		return nil
	}
	anio, err1 := strconv.Atoi(parts[0])
	mes, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || mes < 1 {
		return nil
	}
	out := make([]string, 0, mes-1)
	for m := 1; m < mes; m++ {
		out = append(out, fmt.Sprintf("%04d-%02d", anio, m))
	}
	return out
}

// Dashboard calcula Ingresos/Gastos/EBITDA del período y el comparativo con el anterior.
// Los traslados/overnight emparejados (es_traslado) se excluyen (§13).
//
// COMPARACIÓN A PARIDAD DE DÍA (pedido de Gerencia General, 19-oct-2026). Con el mes en curso, los
// tres números —este mes, el anterior y el promedio del año— se miden hasta el MISMO día. Antes el
// mes a medias se comparaba contra meses enteros, así que el porcentaje medía cuántos días habían
// pasado y no el negocio: setiembre salía «−62 % vs agosto» cuando a paridad la caída real era del
// 35 % y, contra el promedio del año, venía 3 % ARRIBA.
func (s *Service) Dashboard(ctx context.Context, empresaID, periodo string) (DashboardData, error) {
	// AhoraCR y no time.Now(): en el contenedor el reloj es UTC, y entre las 18:00 y la
	// medianoche de Costa Rica ya es el día siguiente allá. El corte saltaría un día antes de
	// tiempo y la comparación incluiría un día que todavía no ocurrió.
	dia, enCurso := DiaDeCorte(periodo, AhoraCR())

	t, err := s.repo.TotalesPeriodoHastaDia(ctx, empresaID, periodo, dia)
	if err != nil {
		return DashboardData{}, err
	}
	prev := periodoAnterior(periodo)
	// El mes anterior se corta al MISMO día: ahí está toda la corrección.
	p, err := s.repo.TotalesPeriodoHastaDia(ctx, empresaID, prev, dia)
	if err != nil {
		return DashboardData{}, err
	}
	promedio, err := s.promedioDelAnio(ctx, empresaID, periodo, dia)
	if err != nil {
		return DashboardData{}, err
	}
	ultimoCargado, err := s.repo.UltimoDiaCargado(ctx, empresaID, periodo)
	if err != nil {
		return DashboardData{}, err
	}
	corte := CorteDelPeriodo{Dia: dia, EnCurso: enCurso, UltimoDiaCargado: ultimoCargado}
	// Solo tiene sentido en un mes en curso: en uno cerrado no se espera que entre nada más.
	if enCurso && dia > ultimoCargado {
		corte.DiasSinCargar = dia - ultimoCargado
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
		Corte:                corte,
		PromedioAnio:         promedio,
	}, nil
}

// promedioDelAnio promedia los meses ya transcurridos del año, cada uno cortado al mismo día.
//
// SOLO CUENTAN LOS MESES CON MOVIMIENTOS. Un mes del que todavía no se cargó nada entraría como
// cero y arrastraría el promedio hacia abajo, haciendo que el mes en curso luzca mejor de lo que
// va: el promedio diría más sobre qué se cargó que sobre cómo vendimos.
func (s *Service) promedioDelAnio(ctx context.Context, empresaID, periodo string, dia int) (PromedioDelAnio, error) {
	meses := mesesPreviosDelAnio(periodo)
	if len(meses) == 0 {
		return PromedioDelAnio{}, nil // enero: no hay con qué comparar y no se inventa
	}
	var sumaIng, sumaGas decimal.Decimal
	con := 0
	for _, m := range meses {
		t, err := s.repo.TotalesPeriodoHastaDia(ctx, empresaID, m, dia)
		if err != nil {
			return PromedioDelAnio{}, err
		}
		if t.Ingresos.IsZero() && t.Gastos.IsZero() {
			continue
		}
		sumaIng = sumaIng.Add(t.Ingresos)
		sumaGas = sumaGas.Add(t.Gastos)
		con++
	}
	if con == 0 {
		return PromedioDelAnio{}, nil
	}
	n := decimal.NewFromInt(int64(con))
	ing := sumaIng.Div(n).Round(2)
	gas := sumaGas.Div(n).Round(2)
	return PromedioDelAnio{
		Meses:    con,
		Ingresos: ing.String(),
		Gastos:   gas.String(),
		EBITDA:   ing.Sub(gas).String(),
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
