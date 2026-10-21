package precios

// Pruebas del costeo. Cada una existe por un defecto concreto que ya nos costó caro en otro
// módulo, no por cubrir líneas.

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// repoFalso permite armar escenarios de costeo sin base de datos. Las pruebas que tocan SQL de
// verdad van aparte, contra PostgreSQL: un repositorio probado solo detrás de un fake no está
// probado.
type repoFalso struct {
	plan   Plan
	lineas []LineaReceta
	base   string
}

func (r *repoFalso) PlanPorID(context.Context, string, string) (Plan, error) { return r.plan, nil }
func (r *repoFalso) LineasDeReceta(context.Context, string, string, string) ([]LineaReceta, error) {
	return r.lineas, nil
}
func (r *repoFalso) BaseIVADelInsumo(context.Context, string, string) (string, error) {
	return r.base, nil
}

func (r *repoFalso) Insumos(context.Context, string, string, string, string, bool) ([]Insumo, error) {
	return nil, nil
}
func (r *repoFalso) InsumoPorID(context.Context, string, string, string) (Insumo, error) {
	return Insumo{}, nil
}
func (r *repoFalso) CrearInsumo(context.Context, string, InsumoInput) (string, error) { return "", nil }
func (r *repoFalso) ActualizarInsumo(context.Context, string, string, InsumoInput) error {
	return nil
}
func (r *repoFalso) EliminarInsumo(context.Context, string, string) error { return nil }
func (r *repoFalso) GuardarCosto(context.Context, string, string, decimal.Decimal, CostoInput, string) error {
	return nil
}
func (r *repoFalso) HistorialDeCosto(context.Context, string, string) ([]CapturaCosto, error) {
	return nil, nil
}
func (r *repoFalso) Planes(context.Context, string) ([]Plan, error)                  { return nil, nil }
func (r *repoFalso) CrearPlan(context.Context, string, PlanInput) (string, error)    { return "", nil }
func (r *repoFalso) ActualizarPlan(context.Context, string, string, PlanInput) error { return nil }
func (r *repoFalso) AgregarLinea(context.Context, string, string, string, decimal.Decimal, string) error {
	return nil
}
func (r *repoFalso) CambiarCantidad(context.Context, string, string, decimal.Decimal) error {
	return nil
}
func (r *repoFalso) QuitarLinea(context.Context, string, string) error { return nil }
func (r *repoFalso) ImportarCatalogo(context.Context, string, PlanDeImportacion, string, string) (ResultadoImportacion, error) {
	return ResultadoImportacion{}, nil
}
func (r *repoFalso) ImportarRecetas(context.Context, string, PlanDeRecetas, string, string) (ResultadoRecetas, error) {
	return ResultadoRecetas{}, nil
}

func str(s string) *string { return &s }
func ent(i int) *int       { return &i }

func linea(nombre, tipo, cantidad string, costo *string) LineaReceta {
	l := LineaReceta{Nombre: nombre, TipoCosto: tipo, Cantidad: cantidad}
	if costo != nil {
		c := decimal.RequireFromString(*costo)
		q := decimal.RequireFromString(cantidad)
		cu := c.StringFixed(2)
		ext := c.Mul(q).Round(2).StringFixed(2)
		l.CostoUnitario, l.Extendido = &cu, &ext
	}
	return l
}

// Este es EL defecto que el módulo existe para evitar: un costeo al que le faltan insumos se lee
// igual que uno completo, porque todos sus números son correctos —solo que de menos líneas—.
func TestUnCosteoIncompletoNoPublicaMargen(t *testing.T) {
	t.Parallel()
	svc := NewService(&repoFalso{
		plan: Plan{Nombre: "Oro", Modalidad: ModalidadDirecto, PrecioVenta: str("1350000")},
		lineas: []LineaReceta{
			linea("Cofre", TipoVariable, "1", str("135000")),
			linea("Sala", TipoVariable, "1", str("275000")),
			linea("Cremación de adulto", TipoVariable, "1", nil), // sin costo capturado
		},
	})

	c, err := svc.Costear(context.Background(), "e", "p", "2026-10-14")
	if err != nil {
		t.Fatalf("Costear: %v", err)
	}

	if c.Margen != nil {
		t.Errorf("publicó margen %q con un insumo sin costo: ese margen sale MÁS ALTO que el real", *c.Margen)
	}
	if c.MargenPct != nil {
		t.Errorf("publicó porcentaje de margen %q sin cobertura completa", *c.MargenPct)
	}
	if c.LineasSinCosto != 1 {
		t.Errorf("líneas sin costo = %d, se esperaba 1", c.LineasSinCosto)
	}
	if c.CoberturaPct != "66.7" {
		t.Errorf("cobertura = %q, se esperaba 66.7", c.CoberturaPct)
	}
	if len(c.InsumosSinCosto) != 1 || c.InsumosSinCosto[0] != "Cremación de adulto" {
		t.Errorf("no nombró el insumo que falta: %v", c.InsumosSinCosto)
	}
	// El costo que sí se pudo sumar se muestra igual: lo que no se publica es el MARGEN.
	if c.CostoVariable != "410000.00" {
		t.Errorf("costo variable = %q, se esperaba 410000.00", c.CostoVariable)
	}
}

func TestUnCosteoCompletoSiPublicaMargen(t *testing.T) {
	t.Parallel()
	svc := NewService(&repoFalso{
		plan: Plan{Nombre: "Oro", Modalidad: ModalidadDirecto, PrecioVenta: str("1000000")},
		lineas: []LineaReceta{
			linea("Cofre", TipoVariable, "1", str("400000")),
			linea("Indirecto", TipoFijo, "1", str("100000")),
		},
	})

	c, err := svc.Costear(context.Background(), "e", "p", "2026-10-14")
	if err != nil {
		t.Fatalf("Costear: %v", err)
	}
	if !c.EstaCompleto() {
		t.Fatal("debería estar completo")
	}
	if c.Margen == nil || *c.Margen != "500000.00" {
		t.Errorf("margen = %v, se esperaba 500000.00", c.Margen)
	}
	if c.MargenPct == nil || *c.MargenPct != "50.0" {
		t.Errorf("margen %% = %v, se esperaba 50.0", c.MargenPct)
	}
}

// Variable y fijo no se pueden sumar sin decirlo: el piso de precio solo mira lo variable, porque
// lo fijo ya está pagado véndase o no. Mezclarlos da un piso inflado y mata ventas que convenían.
func TestElPisoDePrecioEsSoloLoVariable(t *testing.T) {
	t.Parallel()
	svc := NewService(&repoFalso{
		plan: Plan{Nombre: "Jade", Modalidad: ModalidadDirecto, PrecioVenta: str("500000")},
		lineas: []LineaReceta{
			linea("Cofre", TipoVariable, "1", str("200000")),
			linea("Flores", TipoVariable, "6", str("5000")),
			linea("Indirecto absorbido", TipoFijo, "1", str("100000")),
		},
	})

	c, err := svc.Costear(context.Background(), "e", "p", "2026-10-14")
	if err != nil {
		t.Fatalf("Costear: %v", err)
	}
	// 200.000 + 6 × 5.000 = 230.000 variable; el fijo NO entra al piso.
	if c.PisoDePrecio != "230000.00" {
		t.Errorf("piso = %q, se esperaba 230000.00 (sin el fijo)", c.PisoDePrecio)
	}
	if c.CostoTotal != "330000.00" {
		t.Errorf("costo total = %q, se esperaba 330000.00", c.CostoTotal)
	}
	if c.ContribucionMarginal == nil || *c.ContribucionMarginal != "270000.00" {
		t.Errorf("contribución marginal = %v, se esperaba 270000.00", c.ContribucionMarginal)
	}
}

// La contribución marginal SÍ se publica aunque falte un costo fijo, porque el piso no depende de
// lo fijo. Pero si falta un VARIABLE, el piso está mal y no se puede publicar.
func TestLaContribucionSePublicaSiSoloFaltaUnFijo(t *testing.T) {
	t.Parallel()
	conFijoSinCosto := NewService(&repoFalso{
		plan: Plan{Modalidad: ModalidadDirecto, PrecioVenta: str("500000")},
		lineas: []LineaReceta{
			linea("Cofre", TipoVariable, "1", str("200000")),
			linea("Indirecto", TipoFijo, "1", nil),
		},
	})
	c, _ := conFijoSinCosto.Costear(context.Background(), "e", "p", "2026-10-14")
	if c.ContribucionMarginal == nil {
		t.Error("faltando solo un FIJO, la contribución marginal sí se puede calcular")
	}
	if c.Margen != nil {
		t.Error("el margen total NO se puede publicar: falta un costo")
	}

	conVariableSinCosto := NewService(&repoFalso{
		plan: Plan{Modalidad: ModalidadDirecto, PrecioVenta: str("500000")},
		lineas: []LineaReceta{
			linea("Cofre", TipoVariable, "1", nil),
			linea("Indirecto", TipoFijo, "1", str("100000")),
		},
	})
	c2, _ := conVariableSinCosto.Costear(context.Background(), "e", "p", "2026-10-14")
	if c2.ContribucionMarginal != nil {
		t.Errorf("faltando un VARIABLE el piso está mal y no se debe publicar: %v", c2.ContribucionMarginal)
	}
}

// En suscripción no se compara la cuota contra el costo de un servicio completo: diría que el
// plan pierde ₡1,7 millones cada mes, que es absurdo. Se compara lo que el contrato paga en toda
// su vida contra lo que va a costar.
func TestEnSuscripcionElPrecioEsCuotaPorPermanencia(t *testing.T) {
	t.Parallel()
	svc := NewService(&repoFalso{
		plan: Plan{
			Nombre: "Gardenia", Modalidad: ModalidadSuscripcion,
			CuotaMensual: str("4950"), PermanenciaMeses: ent(36), Beneficiarios: ent(9),
		},
		lineas: []LineaReceta{linea("Servicio completo", TipoVariable, "1", str("150000"))},
	})

	c, err := svc.Costear(context.Background(), "e", "p", "2026-10-14")
	if err != nil {
		t.Fatalf("Costear: %v", err)
	}
	// 4.950 × 36 = 178.200
	if c.PrecioReferencia == nil || *c.PrecioReferencia != "178200.00" {
		t.Fatalf("precio de referencia = %v, se esperaba 178200.00 (cuota × permanencia)", c.PrecioReferencia)
	}
	if c.Margen == nil || *c.Margen != "28200.00" {
		t.Errorf("margen = %v, se esperaba 28200.00", c.Margen)
	}
}

// Un plan de suscripción sin permanencia no tiene contra qué medirse. Inventarle 12 meses sería
// una regla de negocio que nadie pidió.
func TestSuscripcionSinPermanenciaNoTienePrecioDeReferencia(t *testing.T) {
	t.Parallel()
	svc := NewService(&repoFalso{
		plan:   Plan{Modalidad: ModalidadSuscripcion, CuotaMensual: str("4950")},
		lineas: []LineaReceta{linea("Servicio", TipoVariable, "1", str("100"))},
	})
	c, _ := svc.Costear(context.Background(), "e", "p", "2026-10-14")
	if c.PrecioReferencia != nil {
		t.Errorf("no debería inventar un precio de referencia: %v", c.PrecioReferencia)
	}
	if c.Margen != nil {
		t.Errorf("sin precio no hay margen: %v", c.Margen)
	}
}

// El IVA soportado es crédito fiscal, así que el costo que entra al modelo va SIN IVA. La
// conversión ocurre una sola vez, al capturar, y no se reparte entre las consultas.
func TestUnCostoCapturadoConIVASeGuardaSinIVA(t *testing.T) {
	t.Parallel()
	in := CostoInput{Costo: "113000", VigenteDesde: "2026-10-14"}
	got, err := in.ACostoSinIVA(BaseConIVA)
	if err != nil {
		t.Fatalf("ACostoSinIVA: %v", err)
	}
	if got.StringFixed(2) != "100000.00" {
		t.Errorf("₡113.000 con IVA = %s, se esperaba 100000.00", got.StringFixed(2))
	}

	igual := CostoInput{Costo: "100000", VigenteDesde: "2026-10-14"}
	got2, _ := igual.ACostoSinIVA(BaseSinIVA)
	if got2.StringFixed(2) != "100000.00" {
		t.Errorf("declarado SIN IVA no se debe tocar: %s", got2.StringFixed(2))
	}
}

func TestLaCapturaDeCostoRechazaLoInvalido(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre string
		in     CostoInput
		quiere error
	}{
		{"sin fecha", CostoInput{Costo: "100"}, ErrFechaVigenciaVacia},
		{"fecha ilegible", CostoInput{Costo: "100", VigenteDesde: "14/10/2026"}, ErrFechaVigenciaVacia},
		{"costo negativo", CostoInput{Costo: "-5", VigenteDesde: "2026-10-14"}, ErrCostoNegativo},
		{"costo no numérico", CostoInput{Costo: "mil", VigenteDesde: "2026-10-14"}, ErrCostoNegativo},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := c.in.ACostoSinIVA(BaseSinIVA); !errors.Is(err, c.quiere) {
				t.Errorf("error = %v, se esperaba %v", err, c.quiere)
			}
		})
	}
}

func TestLaValidacionDelInsumoNormalizaYRechaza(t *testing.T) {
	t.Parallel()
	t.Run("pone los valores por defecto", func(t *testing.T) {
		in := InsumoInput{Codigo: " COF-24 ", Nombre: "  Cofre #24  "}
		if err := in.Normalizar(); err != nil {
			t.Fatalf("Normalizar: %v", err)
		}
		if in.Codigo != "COF-24" || in.Nombre != "Cofre #24" {
			t.Errorf("no recortó: %q / %q", in.Codigo, in.Nombre)
		}
		if in.TipoCosto != TipoVariable {
			t.Errorf("tipo por defecto = %q, se esperaba VARIABLE", in.TipoCosto)
		}
		if in.BaseIVA != BaseSinIVA {
			t.Errorf("base por defecto = %q, se esperaba SIN_IVA", in.BaseIVA)
		}
	})
	t.Run("rechaza lo inválido", func(t *testing.T) {
		casos := []struct {
			in     InsumoInput
			quiere error
		}{
			{InsumoInput{Nombre: "x"}, ErrCodigoVacio},
			{InsumoInput{Codigo: "x"}, ErrNombreVacio},
			{InsumoInput{Codigo: "x", Nombre: "y", TipoCosto: "OTRO"}, ErrTipoCostoInvalido},
			{InsumoInput{Codigo: "x", Nombre: "y", BaseIVA: "QUIZAS"}, ErrBaseIVAInvalida},
		}
		for _, c := range casos {
			in := c.in
			if err := in.Normalizar(); !errors.Is(err, c.quiere) {
				t.Errorf("%+v → %v, se esperaba %v", c.in, err, c.quiere)
			}
		}
	})
}

// ── El repoFalso también satisface la parte ABC, para que el servicio se pueda construir en las
// pruebas de costeo. Los escenarios del reparto se prueban aparte, en abc_test.go.
func (r *repoFalso) Drivers(context.Context, string, string) ([]Driver, error) { return nil, nil }
func (r *repoFalso) CrearDriver(context.Context, string, DriverInput) (string, error) {
	return "", nil
}
func (r *repoFalso) EliminarDriver(context.Context, string, string) error { return nil }
func (r *repoFalso) FijarBase(context.Context, string, string, string, decimal.Decimal, string, string) error {
	return nil
}
func (r *repoFalso) Repartos(context.Context, string, string, string) ([]RepartoDeGasto, error) {
	return nil, nil
}
func (r *repoFalso) FijarReparto(context.Context, string, string, string, decimal.Decimal, string, string) error {
	return nil
}
func (r *repoFalso) QuitarReparto(context.Context, string, string) error { return nil }
func (r *repoFalso) ConsumosDelPlan(context.Context, string, string) ([]IndirectoDelPlan, error) {
	return nil, nil
}
func (r *repoFalso) FijarConsumo(context.Context, string, string, string, decimal.Decimal, string) error {
	return nil
}
func (r *repoFalso) QuitarConsumo(context.Context, string, string, string) error { return nil }
func (r *repoFalso) BaseDelDriver(context.Context, string, string, string) (*decimal.Decimal, error) {
	return nil, nil
}

// El repoFalso también satisface la parte de comisiones; los escenarios van en comisiones_test.go.
func (r *repoFalso) Politicas(context.Context, string) ([]PoliticaComision, error) { return nil, nil }
func (r *repoFalso) CrearPolitica(context.Context, string, PoliticaInput, string) (string, error) {
	return "", nil
}
func (r *repoFalso) ActualizarPolitica(context.Context, string, string, PoliticaInput) error {
	return nil
}
func (r *repoFalso) EliminarPolitica(context.Context, string, string) error { return nil }
func (r *repoFalso) Vendedores(context.Context, string) ([]Vendedor, error) { return nil, nil }
