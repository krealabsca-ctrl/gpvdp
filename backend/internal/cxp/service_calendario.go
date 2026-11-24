package cxp

// El calendario de pagos, del lado del negocio. El porqué está en calendario.go.

import (
	"context"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// CalendarioDePagos arma los dos bloques.
//
// `hoy` se recibe en vez de leerse acá para que la ubicación de cada fecha —«esta semana», «la
// próxima»— se pueda probar sin depender del día en que corran las pruebas. El handler pasa la
// fecha de Costa Rica; el contenedor corre en UTC y a las 18:00 de un viernes ya sería sábado,
// con lo que «esta semana» se correría un día entero.
func (s *Service) CalendarioDePagos(ctx context.Context, empresaID string, hoy time.Time) (CalendarioPagos, error) {
	prog, err := s.repo.ProgramadoPorFecha(ctx, empresaID)
	if err != nil {
		return CalendarioPagos{}, err
	}
	venc, err := s.repo.VencidoPorEstado(ctx, empresaID)
	if err != nil {
		return CalendarioPagos{}, err
	}

	cal := CalendarioPagos{Hoy: hoy.Format("2006-01-02")}

	totalProg := decimal.Zero
	for _, f := range prog {
		fecha, err := time.Parse("2006-01-02", f.Fecha)
		if err != nil {
			continue // una fecha ilegible no puede tumbar el calendario entero
		}
		cuando, atrasado := UbicarFecha(fecha, hoy)
		monto, _ := decimal.NewFromString(f.TotalCRC)
		totalProg = totalProg.Add(monto)
		cal.Programado = append(cal.Programado, TramoProgramado{
			Fecha:    f.Fecha,
			Dia:      NombreDelDia(fecha),
			Cuando:   cuando,
			Atrasado: atrasado,
			Facturas: f.Facturas,
			TotalCRC: monto.StringFixed(2),
		})
	}

	// Los cuatro grupos del vencido se funden por SITUACIÓN: dos estados distintos pueden
	// significar lo mismo para quien mira (APROBADO y PROGRAMADO son los dos «listo para pagar»).
	porOrden := map[int]*GrupoVencido{}
	totalVenc := decimal.Zero
	for _, f := range venc {
		orden, situacion, queFalta := situacionDelVencido(f.Estado)
		g, ok := porOrden[orden]
		if !ok {
			g = &GrupoVencido{Orden: orden, Situacion: situacion, QueFalta: queFalta, TotalCRC: "0"}
			porOrden[orden] = g
		}
		monto, _ := decimal.NewFromString(f.TotalCRC)
		acumulado, _ := decimal.NewFromString(g.TotalCRC)
		g.TotalCRC = acumulado.Add(monto).String()
		g.Facturas += f.Facturas
		g.Estados = append(g.Estados, f.Estado)
		totalVenc = totalVenc.Add(monto)
	}
	for _, g := range porOrden {
		sort.Strings(g.Estados)
		monto, _ := decimal.NewFromString(g.TotalCRC)
		g.TotalCRC = monto.StringFixed(2)
		if g.Orden == 1 {
			cal.ListoParaPagar = g.TotalCRC
		}
		cal.Vencido = append(cal.Vencido, *g)
	}
	// En el orden del flujo: lo que se puede pagar hoy arriba, lo que está más lejos abajo.
	sort.Slice(cal.Vencido, func(i, j int) bool { return cal.Vencido[i].Orden < cal.Vencido[j].Orden })

	if cal.ListoParaPagar == "" {
		cal.ListoParaPagar = "0.00"
	}
	cal.TotalProgramado = totalProg.StringFixed(2)
	cal.TotalVencido = totalVenc.StringFixed(2)
	if cal.Programado == nil {
		cal.Programado = []TramoProgramado{}
	}
	if cal.Vencido == nil {
		cal.Vencido = []GrupoVencido{}
	}
	return cal, nil
}
