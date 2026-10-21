package precios

// Orquestación del costeo por actividad.

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// RepositorioABC es lo que el servicio necesita para el reparto.
type RepositorioABC interface {
	Drivers(ctx context.Context, empresaID, periodo string) ([]Driver, error)
	CrearDriver(ctx context.Context, empresaID string, in DriverInput) (string, error)
	EliminarDriver(ctx context.Context, empresaID, id string) error
	FijarBase(ctx context.Context, empresaID, driverID, periodo string, cantidad decimal.Decimal, nota, usuarioID string) error
	Repartos(ctx context.Context, empresaID, desde, hasta string) ([]RepartoDeGasto, error)
	FijarReparto(ctx context.Context, empresaID, clasificacionID, driverID string, pct decimal.Decimal, vigenteDesde, usuarioID string) error
	QuitarReparto(ctx context.Context, empresaID, id string) error
	ConsumosDelPlan(ctx context.Context, empresaID, planID string) ([]IndirectoDelPlan, error)
	FijarConsumo(ctx context.Context, empresaID, planID, driverID string, consumo decimal.Decimal, nota string) error
	QuitarConsumo(ctx context.Context, empresaID, planID, driverID string) error
	BaseDelDriver(ctx context.Context, empresaID, driverID, periodo string) (*decimal.Decimal, error)
}

func validarFecha(f string) (string, error) {
	if f == "" {
		return "", ErrPeriodoInvalido
	}
	if _, err := time.Parse("2006-01-02", f); err != nil {
		return "", ErrPeriodoInvalido
	}
	return f, nil
}

// Drivers lista los generadores de costo.
func (s *Service) Drivers(ctx context.Context, empresaID, periodo string) ([]Driver, error) {
	return s.repo.Drivers(ctx, empresaID, periodo)
}

// CrearDriver valida y crea.
func (s *Service) CrearDriver(ctx context.Context, empresaID string, in DriverInput) (string, error) {
	if err := in.Normalizar(); err != nil {
		return "", err
	}
	return s.repo.CrearDriver(ctx, empresaID, in)
}

// EliminarDriver quita un driver que no esté en uso.
func (s *Service) EliminarDriver(ctx context.Context, empresaID, id string) error {
	return s.repo.EliminarDriver(ctx, empresaID, id)
}

// FijarBase guarda la medición de un período.
func (s *Service) FijarBase(ctx context.Context, empresaID, driverID, periodo, cantidad, nota, usuarioID string) error {
	p, err := validarFecha(periodo)
	if err != nil {
		return err
	}
	c, err := decimal.NewFromString(cantidad)
	if err != nil || c.LessThanOrEqual(decimal.Zero) {
		return ErrBaseNoPositiva
	}
	return s.repo.FijarBase(ctx, empresaID, driverID, p, c, nota, usuarioID)
}

// FijarReparto asigna parte de un gasto a un driver.
func (s *Service) FijarReparto(ctx context.Context, empresaID, clasificacionID, driverID, porcentaje, vigenteDesde, usuarioID string) error {
	v, err := validarFecha(vigenteDesde)
	if err != nil {
		return err
	}
	p, err := decimal.NewFromString(porcentaje)
	if err != nil || p.LessThanOrEqual(decimal.Zero) || p.GreaterThan(cien) {
		return ErrPorcentajeInvalido
	}
	return s.repo.FijarReparto(ctx, empresaID, clasificacionID, driverID, p, v, usuarioID)
}

// QuitarReparto borra una línea.
func (s *Service) QuitarReparto(ctx context.Context, empresaID, id string) error {
	return s.repo.QuitarReparto(ctx, empresaID, id)
}

// FijarConsumo dice cuánto consume un plan de un driver.
func (s *Service) FijarConsumo(ctx context.Context, empresaID, planID, driverID, consumo, nota string) error {
	c, err := decimal.NewFromString(consumo)
	if err != nil || c.LessThanOrEqual(decimal.Zero) {
		return ErrConsumoNoPositivo
	}
	return s.repo.FijarConsumo(ctx, empresaID, planID, driverID, c, nota)
}

// QuitarConsumo saca un driver de un plan.
func (s *Service) QuitarConsumo(ctx context.Context, empresaID, planID, driverID string) error {
	return s.repo.QuitarConsumo(ctx, empresaID, planID, driverID)
}

// Repartir arma la foto del período: qué gasto entró a cada driver y a qué costo unitario.
//
// Un reparto que no suma 100 % NO se usa, y su gasto se informa aparte. Es la regla que impide
// repetir el 45 + 85 = 130 % del archivo de costeo: absorber de más infla el costo de todo lo
// que se vende, absorber de menos lo desinfla, y ninguna de las dos cosas se nota en el total.
func (s *Service) Repartir(ctx context.Context, empresaID, desde, hasta string) (RepartoDelPeriodo, error) {
	d, err := validarFecha(desde)
	if err != nil {
		return RepartoDelPeriodo{}, err
	}
	h, err := validarFecha(hasta)
	if err != nil {
		return RepartoDelPeriodo{}, err
	}

	repartos, err := s.repo.Repartos(ctx, empresaID, d, h)
	if err != nil {
		return RepartoDelPeriodo{}, err
	}

	rp := RepartoDelPeriodo{Desde: d, Hasta: h, Bolsas: []BolsaDeDriver{}, Problemas: []RepartoDeGasto{}}
	total, repartido, invalido := decimal.Zero, decimal.Zero, decimal.Zero
	bolsa := map[string]decimal.Decimal{}
	nombre := map[string]string{}
	unidad := map[string]string{}

	for _, g := range repartos {
		monto, err := decimal.NewFromString(g.GastoDelPeriodo)
		if err != nil {
			return RepartoDelPeriodo{}, fmt.Errorf("precios: gasto ilegible en %q: %w", g.Clasificacion, err)
		}
		total = total.Add(monto)

		if !g.Valido {
			// Hay DOS casos distintos y juntarlos miente sobre el estado del modelo:
			//
			//   · Sin ninguna línea → nadie le asignó driver todavía. Es trabajo pendiente, y su
			//     gasto queda «sin asignar».
			//   · Con líneas que no suman 100 % → está MAL repartido. Eso es un error a corregir,
			//     y es lo que la pantalla tiene que poner adelante.
			//
			// Medido contra el gasto real: 51 clasificaciones sin repartir y 1 mal repartida.
			// Contarlas juntas daba «52 problemas» y escondía el único que de verdad lo era.
			if len(g.Lineas) > 0 {
				rp.Problemas = append(rp.Problemas, g)
				invalido = invalido.Add(monto)
			}
			continue
		}
		for _, l := range g.Lineas {
			m, err := decimal.NewFromString(l.Monto)
			if err != nil {
				continue
			}
			bolsa[l.DriverID] = bolsa[l.DriverID].Add(m)
			nombre[l.DriverID] = l.Driver
			repartido = repartido.Add(m)
		}
	}

	// El costo unitario de cada bolsa necesita la base del período.
	drivers, err := s.repo.Drivers(ctx, empresaID, "")
	if err != nil {
		return RepartoDelPeriodo{}, err
	}
	for _, dr := range drivers {
		unidad[dr.ID] = dr.Unidad
		nombre[dr.ID] = dr.Nombre
	}

	for id, gasto := range bolsa {
		b := BolsaDeDriver{DriverID: id, Driver: nombre[id], Unidad: unidad[id], Gasto: gasto.StringFixed(2)}
		base, err := s.repo.BaseDelDriver(ctx, empresaID, id, h)
		if err != nil {
			return RepartoDelPeriodo{}, err
		}
		if base == nil || base.LessThanOrEqual(decimal.Zero) {
			b.Motivo = "falta la base del período: sin ella no hay costo unitario"
		} else {
			s := base.String()
			b.Base = &s
			u := gasto.Div(*base).Round(2).StringFixed(2)
			b.CostoUnitario = &u
		}
		rp.Bolsas = append(rp.Bolsas, b)
	}

	rp.GastoTotal = total.StringFixed(2)
	rp.GastoRepartido = repartido.StringFixed(2)
	rp.GastoEnRepartosInvalidos = invalido.StringFixed(2)
	rp.GastoSinAsignar = total.Sub(repartido).Sub(invalido).StringFixed(2)
	return rp, nil
}

// AbsorcionDelPlan calcula el indirecto que le toca a un plan en un período.
func (s *Service) AbsorcionDelPlan(ctx context.Context, empresaID, planID, desde, hasta string) (AbsorcionDelPlan, error) {
	plan, err := s.repo.PlanPorID(ctx, empresaID, planID)
	if err != nil {
		return AbsorcionDelPlan{}, err
	}
	consumos, err := s.repo.ConsumosDelPlan(ctx, empresaID, planID)
	if err != nil {
		return AbsorcionDelPlan{}, err
	}
	rp, err := s.Repartir(ctx, empresaID, desde, hasta)
	if err != nil {
		return AbsorcionDelPlan{}, err
	}
	return Absorber(planID, plan.Nombre, consumos, rp.Bolsas), nil
}
