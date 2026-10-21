package precios

// Lógica de negocio del costeo.
//
// ── LA REGLA QUE GOBIERNA ESTE ARCHIVO ──────────────────────────────────────────────────────
//
// Un costeo incompleto NO publica margen.
//
// Es el mismo defecto que ya nos costó caro en «Gasto por ítem»: una lista armada con el 2 % de
// las facturas se lee idéntica a una armada con el 100 %, porque todos sus números son correctos
// —solo que de menos filas—. Acá es peor, porque el número alimenta una decisión de precio: un
// margen calculado sobre un costo al que le faltan insumos sale MÁS ALTO que el real, y empuja a
// vender barato justo lo que menos conviene.
//
// Por eso `Costear` siempre devuelve la cobertura, y deja `Margen` en nil cuando falta un costo.

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Repository es el acceso a datos que necesita el servicio.
type Repository interface {
	Insumos(ctx context.Context, empresaID, fecha, busca, tipo string, soloSinCosto bool) ([]Insumo, error)
	InsumoPorID(ctx context.Context, empresaID, id, fecha string) (Insumo, error)
	CrearInsumo(ctx context.Context, empresaID string, in InsumoInput) (string, error)
	ActualizarInsumo(ctx context.Context, empresaID, id string, in InsumoInput) error
	EliminarInsumo(ctx context.Context, empresaID, id string) error
	BaseIVADelInsumo(ctx context.Context, empresaID, insumoID string) (string, error)
	GuardarCosto(ctx context.Context, empresaID, insumoID string, costo decimal.Decimal, in CostoInput, usuarioID string) error
	HistorialDeCosto(ctx context.Context, empresaID, insumoID string) ([]CapturaCosto, error)

	Planes(ctx context.Context, empresaID string) ([]Plan, error)
	PlanPorID(ctx context.Context, empresaID, id string) (Plan, error)
	CrearPlan(ctx context.Context, empresaID string, in PlanInput) (string, error)
	ActualizarPlan(ctx context.Context, empresaID, id string, in PlanInput) error

	LineasDeReceta(ctx context.Context, empresaID, planID, fecha string) ([]LineaReceta, error)
	AgregarLinea(ctx context.Context, empresaID, planID, insumoID string, cantidad decimal.Decimal, nota string) error
	CambiarCantidad(ctx context.Context, empresaID, lineaID string, cantidad decimal.Decimal) error
	QuitarLinea(ctx context.Context, empresaID, lineaID string) error

	ImportarCatalogo(ctx context.Context, empresaID string, plan PlanDeImportacion, vigenteDesde, usuarioID string) (ResultadoImportacion, error)
	ImportarRecetas(ctx context.Context, empresaID string, plan PlanDeRecetas, vigenteDesde, usuarioID string) (ResultadoRecetas, error)

	RepositorioABC
	RepositorioComisiones
}

// Service expone las operaciones del módulo.
type Service struct {
	repo Repository
	// cargas es el puerto hacia nómina; nil = el costo patronal de las comisiones no se calcula.
	cargas FuenteDeCargasSociales
}

// NewService construye el servicio.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

func hoy() string { return time.Now().Format("2006-01-02") }

func fechaOHoy(f string) string {
	if f == "" {
		return hoy()
	}
	if _, err := time.Parse("2006-01-02", f); err != nil {
		return hoy()
	}
	return f
}

// Insumos lista el catálogo.
func (s *Service) Insumos(ctx context.Context, empresaID, fecha, busca, tipo string, soloSinCosto bool) ([]Insumo, error) {
	return s.repo.Insumos(ctx, empresaID, fechaOHoy(fecha), busca, tipo, soloSinCosto)
}

// CrearInsumo valida y crea.
func (s *Service) CrearInsumo(ctx context.Context, empresaID string, in InsumoInput) (string, error) {
	if err := in.Normalizar(); err != nil {
		return "", err
	}
	return s.repo.CrearInsumo(ctx, empresaID, in)
}

// ActualizarInsumo valida y actualiza.
func (s *Service) ActualizarInsumo(ctx context.Context, empresaID, id string, in InsumoInput) error {
	if err := in.Normalizar(); err != nil {
		return err
	}
	return s.repo.ActualizarInsumo(ctx, empresaID, id, in)
}

// EliminarInsumo quita del catálogo lo que no esté en uso.
func (s *Service) EliminarInsumo(ctx context.Context, empresaID, id string) error {
	return s.repo.EliminarInsumo(ctx, empresaID, id)
}

// CapturarCosto guarda un costo convirtiéndolo a la base sin IVA según lo declarado en el insumo.
//
// La conversión ocurre ACÁ y no en cada consulta: si cada lectura tuviera que acordarse de
// convertir, basta que una se olvide para que el costeo mezcle bases y nadie lo note.
func (s *Service) CapturarCosto(ctx context.Context, empresaID, insumoID string, in CostoInput, usuarioID string) error {
	base, err := s.repo.BaseIVADelInsumo(ctx, empresaID, insumoID)
	if err != nil {
		return err
	}
	costo, err := in.ACostoSinIVA(base)
	if err != nil {
		return err
	}
	return s.repo.GuardarCosto(ctx, empresaID, insumoID, costo, in, usuarioID)
}

// HistorialDeCosto devuelve las capturas de un insumo.
func (s *Service) HistorialDeCosto(ctx context.Context, empresaID, insumoID string) ([]CapturaCosto, error) {
	return s.repo.HistorialDeCosto(ctx, empresaID, insumoID)
}

// Planes lista los planes.
func (s *Service) Planes(ctx context.Context, empresaID string) ([]Plan, error) {
	return s.repo.Planes(ctx, empresaID)
}

// CrearPlan valida y crea.
func (s *Service) CrearPlan(ctx context.Context, empresaID string, in PlanInput) (string, error) {
	if err := in.Normalizar(); err != nil {
		return "", err
	}
	return s.repo.CrearPlan(ctx, empresaID, in)
}

// ActualizarPlan valida y actualiza.
func (s *Service) ActualizarPlan(ctx context.Context, empresaID, id string, in PlanInput) error {
	if err := in.Normalizar(); err != nil {
		return err
	}
	return s.repo.ActualizarPlan(ctx, empresaID, id, in)
}

// AgregarALaReceta pone un insumo en un plan.
func (s *Service) AgregarALaReceta(ctx context.Context, empresaID, planID, insumoID, cantidad, nota string) error {
	c, err := decimal.NewFromString(cantidad)
	if err != nil || c.LessThanOrEqual(decimal.Zero) {
		return ErrCantidadNoPositiva
	}
	return s.repo.AgregarLinea(ctx, empresaID, planID, insumoID, c, nota)
}

// CambiarCantidad ajusta una línea.
func (s *Service) CambiarCantidad(ctx context.Context, empresaID, lineaID, cantidad string) error {
	c, err := decimal.NewFromString(cantidad)
	if err != nil || c.LessThanOrEqual(decimal.Zero) {
		return ErrCantidadNoPositiva
	}
	return s.repo.CambiarCantidad(ctx, empresaID, lineaID, c)
}

// QuitarDeLaReceta saca una línea.
func (s *Service) QuitarDeLaReceta(ctx context.Context, empresaID, lineaID string) error {
	return s.repo.QuitarLinea(ctx, empresaID, lineaID)
}

// Costear arma el costeo completo de un plan a una fecha.
//
// Esta función es el módulo. Todo lo demás existe para alimentarla.
func (s *Service) Costear(ctx context.Context, empresaID, planID, fecha string) (Costeo, error) {
	f := fechaOHoy(fecha)

	plan, err := s.repo.PlanPorID(ctx, empresaID, planID)
	if err != nil {
		return Costeo{}, err
	}
	lineas, err := s.repo.LineasDeReceta(ctx, empresaID, planID, f)
	if err != nil {
		return Costeo{}, fmt.Errorf("precios: costear: %w", err)
	}

	c := Costeo{Plan: plan, Fecha: f, Lineas: lineas, LineasTotal: len(lineas), InsumosSinCosto: []string{}}

	variable, fijo := decimal.Zero, decimal.Zero
	for _, l := range lineas {
		if l.Extendido == nil {
			c.LineasSinCosto++
			c.InsumosSinCosto = append(c.InsumosSinCosto, l.Nombre)
			continue
		}
		c.LineasConCosto++
		monto, err := decimal.NewFromString(*l.Extendido)
		if err != nil {
			return Costeo{}, fmt.Errorf("precios: extendido ilegible en %q: %w", l.Nombre, err)
		}
		if l.TipoCosto == TipoFijo {
			fijo = fijo.Add(monto)
		} else {
			variable = variable.Add(monto)
		}
	}

	total := variable.Add(fijo)
	c.CostoVariable = variable.StringFixed(2)
	c.CostoFijo = fijo.StringFixed(2)
	c.CostoTotal = total.StringFixed(2)
	c.PisoDePrecio = variable.StringFixed(2)

	if c.LineasTotal == 0 {
		c.CoberturaPct = "0.0"
	} else {
		cob := decimal.NewFromInt(int64(c.LineasConCosto)).
			Div(decimal.NewFromInt(int64(c.LineasTotal))).
			Mul(decimal.NewFromInt(100))
		c.CoberturaPct = cob.StringFixed(1)
	}

	precio := precioDeReferencia(plan)
	if precio == nil {
		return c, nil
	}
	p := precio.StringFixed(2)
	c.PrecioReferencia = &p

	// La contribución marginal se publica aunque falten costos FIJOS: el piso de precio solo
	// depende de lo variable. Pero si falta un costo VARIABLE, el piso está mal y no se publica.
	faltaVariable := false
	for _, l := range lineas {
		if l.Extendido == nil && l.TipoCosto == TipoVariable {
			faltaVariable = true
			break
		}
	}
	if !faltaVariable {
		cm := precio.Sub(variable).Round(2).StringFixed(2)
		c.ContribucionMarginal = &cm
	}

	// El margen exige cobertura total: ver el comentario de cabecera.
	if c.EstaCompleto() {
		margen := precio.Sub(total).Round(2)
		m := margen.StringFixed(2)
		c.Margen = &m
		if precio.GreaterThan(decimal.Zero) {
			pct := margen.Div(*precio).Mul(decimal.NewFromInt(100)).Round(1).StringFixed(1)
			c.MargenPct = &pct
		}
	}
	return c, nil
}

// precioDeReferencia es el precio contra el que se mide el costo: el de venta en un plan directo,
// o cuota × permanencia en uno de suscripción.
//
// En suscripción NO se usa la cuota sola: compararla contra el costo de un servicio completo
// diría que el plan pierde ₡1,7 millones cada mes, que es absurdo. Lo que se compara es todo lo
// que el contrato va a pagar en su vida contra lo que va a costar.
func precioDeReferencia(p Plan) *decimal.Decimal {
	if p.Modalidad == ModalidadSuscripcion {
		if p.CuotaMensual == nil || p.PermanenciaMeses == nil || *p.PermanenciaMeses <= 0 {
			return nil
		}
		cuota, err := decimal.NewFromString(*p.CuotaMensual)
		if err != nil {
			return nil
		}
		total := cuota.Mul(decimal.NewFromInt(int64(*p.PermanenciaMeses)))
		return &total
	}
	if p.PrecioVenta == nil {
		return nil
	}
	v, err := decimal.NewFromString(*p.PrecioVenta)
	if err != nil {
		return nil
	}
	return &v
}
