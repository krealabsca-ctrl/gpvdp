package conta

// Fase 4: balance de comprobación, apertura y cierre de período.
//
// LA INVARIANTE DEL CIERRE la hace cumplir un disparador, no el servicio: si viviera en el
// código, un script de corrección o un `psql` a mano podrían mover el saldo de un mes ya
// publicado, y dos personas con el mismo reporte verían números distintos.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var tablasDelPeriodo = []string{
	"empresa", "usuario", "cuenta_contable",
	"conta_consecutivo", "asiento", "asiento_linea", "periodo_contable",
}

const disparadorPeriodoAsiento = `
	CREATE TRIGGER trg_asiento_periodo_abierto
	BEFORE INSERT OR UPDATE OR DELETE ON %s.asiento
	FOR EACH ROW EXECUTE FUNCTION public.fn_periodo_no_cerrado()`

const disparadorPeriodoLinea = `
	CREATE TRIGGER trg_linea_periodo_abierto
	BEFORE INSERT OR UPDATE OR DELETE ON %s.asiento_linea
	FOR EACH ROW EXECUTE FUNCTION public.fn_periodo_no_cerrado()`

func basePeriodo(t *testing.T) (*pgRepository, *Service) {
	t.Helper()
	repo := baseConta(t, tablasDelPeriodo, []string{
		disparadorDelArbol, disparadorCuadraCab, disparadorCuadraLin, disparadorLineaCuenta,
		disparadorPeriodoAsiento, disparadorPeriodoLinea,
	})
	return repo, NewService(repo, nil, zap.NewNop())
}

// asentar crea un asiento que cuadra, en la fecha dada.
func asentar(t *testing.T, repo *pgRepository, fecha string, monto string, gasto, banco Cuenta) Asiento {
	t.Helper()
	a, err := repo.CrearAsiento(context.Background(), Asiento{
		EmpresaID: empUno, Fecha: fecha, Glosa: "prueba",
		OrigenTipo: OrigenManual, Estado: EstadoAsentado,
	}, []LineaValidada{
		{CuentaID: gasto.ID, Debe: dec(monto)},
		{CuentaID: banco.ID, Haber: dec(monto)},
	}, usrPrueba)
	if err != nil {
		t.Fatalf("asentar %s: %v", fecha, err)
	}
	return a
}

// UN PERÍODO CERRADO NO RECIBE NADA: ni asientos nuevos, ni líneas, ni anulaciones.
//
// La anulación es la que más se escapa al pensarlo: no agrega plata, pero la saca, y mueve el
// saldo de un mes publicado exactamente igual que un asiento nuevo.
func TestUnPeriodoCerradoNoRecibeAsientos(t *testing.T) {
	repo, svc := basePeriodo(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)

	a := asentar(t, repo, "2026-08-15", "500.00", gasto, banco)
	if err := svc.CerrarPeriodo(ctx, empUno, "2026-08", usrPrueba); err != nil {
		t.Fatalf("cerrar: %v", err)
	}

	t.Run("un asiento nuevo con fecha de agosto", func(t *testing.T) {
		_, err := repo.CrearAsiento(ctx, Asiento{
			EmpresaID: empUno, Fecha: "2026-08-20", Glosa: "no debería entrar",
			OrigenTipo: OrigenManual, Estado: EstadoAsentado,
		}, []LineaValidada{
			{CuentaID: gasto.ID, Debe: dec("100.00")},
			{CuentaID: banco.ID, Haber: dec("100.00")},
		}, usrPrueba)
		if err == nil {
			t.Fatal("entró un asiento en un período cerrado")
		}
		if !strings.Contains(err.Error(), "cerrado") {
			t.Errorf("el error no dice que el período está cerrado: %v", err)
		}
		// Y dice qué hacer, que es la mitad útil del mensaje.
		if !strings.Contains(err.Error(), "reabrir") {
			t.Errorf("el error no dice cómo corregirlo: %v", err)
		}
	})

	t.Run("anular un asiento de agosto", func(t *testing.T) {
		err := repo.AnularAsiento(ctx, empUno, a.ID, "intento en período cerrado", usrPrueba)
		if err == nil {
			t.Fatal("se anuló un asiento de un período cerrado: eso mueve el saldo publicado")
		}
	})

	t.Run("setiembre sigue abierto", func(t *testing.T) {
		asentar(t, repo, "2026-09-10", "700.00", gasto, banco)
	})
}

// NO SE CIERRA UN MES QUE NO CUADRA: cerrarlo congelaría el error, y la única salida sería
// reabrir —que es justo lo que el cierre existe para evitar—.
func TestNoSeCierraUnPeriodoQueNoCuadra(t *testing.T) {
	repo, svc := basePeriodo(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)
	a := asentar(t, repo, "2026-08-15", "500.00", gasto, banco)

	// Se descuadra por abajo, saltándose el servicio: es el estado en que podría quedar el libro
	// si algo externo lo tocara.
	if _, err := repo.pool.Exec(ctx,
		`ALTER TABLE asiento_linea DISABLE TRIGGER trg_asiento_cuadra_lin`); err != nil {
		t.Fatalf("apagar el disparador: %v", err)
	}
	if _, err := repo.pool.Exec(ctx,
		`UPDATE asiento_linea SET haber = 400 WHERE asiento_id = $1::uuid AND haber > 0`, a.ID); err != nil {
		t.Fatalf("descuadrar: %v", err)
	}

	if err := svc.CerrarPeriodo(ctx, empUno, "2026-08", usrPrueba); !errors.Is(err, ErrPeriodoNoCuadra) {
		t.Fatalf("cerró un período descuadrado: %v", err)
	}
}

// Reabrir exige motivo, queda registrado, y devuelve la capacidad de escribir.
func TestReabrirExigeMotivoYQuedaRegistrado(t *testing.T) {
	repo, svc := basePeriodo(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)
	asentar(t, repo, "2026-08-15", "500.00", gasto, banco)

	if err := svc.CerrarPeriodo(ctx, empUno, "2026-08", usrPrueba); err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	// Cerrar dos veces no cierra más.
	if err := svc.CerrarPeriodo(ctx, empUno, "2026-08", usrPrueba); !errors.Is(err, ErrPeriodoCerradoYa) {
		t.Errorf("cerró dos veces: %v", err)
	}
	// El «ok» reflejo no explica nada a quien pregunte dentro de seis meses.
	if err := svc.ReabrirPeriodo(ctx, empUno, "2026-08", "ok", usrPrueba); !errors.Is(err, ErrMotivoReaperturaRequerido) {
		t.Errorf("reabrió sin explicación: %v", err)
	}

	const motivo = "faltaba cargar la factura del proveedor de agosto"
	if err := svc.ReabrirPeriodo(ctx, empUno, "2026-08", motivo, usrPrueba); err != nil {
		t.Fatalf("reabrir: %v", err)
	}
	// Reabrir dos veces tampoco.
	if err := svc.ReabrirPeriodo(ctx, empUno, "2026-08", motivo, usrPrueba); !errors.Is(err, ErrPeriodoNoCerrado) {
		t.Errorf("reabrió un período abierto: %v", err)
	}

	ps, err := svc.Periodos(ctx, empUno)
	if err != nil {
		t.Fatalf("listar períodos: %v", err)
	}
	var agosto *PeriodoContable
	for i := range ps {
		if ps[i].Anio == 2026 && ps[i].Mes == 8 {
			agosto = &ps[i]
		}
	}
	if agosto == nil {
		t.Fatalf("no apareció agosto: %+v", ps)
	}
	if agosto.Estado != PeriodoAbierto {
		t.Errorf("estado %q, se esperaba ABIERTO", agosto.Estado)
	}
	if agosto.ReabiertoMotivo != motivo {
		t.Errorf("el motivo no quedó registrado: %q", agosto.ReabiertoMotivo)
	}
	if agosto.ReabiertoEn == nil {
		t.Error("no quedó cuándo se reabrió")
	}

	// Y ya se puede volver a escribir.
	asentar(t, repo, "2026-08-25", "300.00", gasto, banco)
}

// ── Balance de comprobación ─────────────────────────────────────────────────────────────────

// EL BALANCE ARRASTRA EL SALDO ANTERIOR. Sin esa columna, el balance de un mes arranca en cero y
// su saldo final es el del mes, no el de la cuenta: el número que alguien copie a un estado
// financiero estaría mal y parecería bien.
func TestElBalanceArrastraElSaldoAnteriorYCuadra(t *testing.T) {
	repo, svc := basePeriodo(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)

	asentar(t, repo, "2026-07-10", "1000.00", gasto, banco) // antes del rango
	asentar(t, repo, "2026-08-05", "300.00", gasto, banco)
	asentar(t, repo, "2026-08-20", "200.00", gasto, banco)

	b, err := svc.Balance(ctx, empUno, "2026-08-01", "2026-08-31")
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if !b.Cuadra {
		t.Fatalf("el balance no cuadra: diferencia %q", b.Diferencia)
	}

	porCodigo := map[string]RenglonBalance{}
	for _, g := range b.Renglones {
		porCodigo[g.Codigo] = g
	}
	g := porCodigo[gasto.Codigo]
	if g.SaldoAnteriorDebe != "1000.00" {
		t.Errorf("saldo anterior del gasto %q, se esperaba 1000.00 (lo de julio)", g.SaldoAnteriorDebe)
	}
	if g.Debe != "500.00" {
		t.Errorf("movimientos del gasto %q, se esperaba 500.00", g.Debe)
	}
	if g.SaldoFinalDebe != "1500.00" {
		t.Errorf("saldo final del gasto %q, se esperaba 1500.00", g.SaldoFinalDebe)
	}

	// EL SALDO VA EN LA COLUMNA DE SU SIGNO. El banco solo recibió créditos, así que su saldo es
	// acreedor: mostrarlo como «debe −1500» obliga a traducir cada fila mentalmente.
	bc := porCodigo[banco.Codigo]
	if bc.SaldoFinalHaber != "1500.00" || bc.SaldoFinalDebe != "0.00" {
		t.Errorf("saldo del banco: debe %q, haber %q; se esperaba 0.00 y 1500.00",
			bc.SaldoFinalDebe, bc.SaldoFinalHaber)
	}

	// Sin apertura cargada, el balance lo dice: es correcto pero incompleto, y la diferencia
	// entre las dos cosas no se ve mirando los números.
	if !b.SinApertura {
		t.Error("no avisó que faltan los saldos iniciales")
	}
	if !strings.Contains(b.Aviso, "saldos iniciales") {
		t.Errorf("el aviso no menciona los saldos iniciales: %q", b.Aviso)
	}
}

// EL BALANCE NO NOMBRA UNA CUENTA QUE ESTA EMPRESA NO MOVIÓ.
//
// Las cuentas del GRUPO las comparten las tres empresas, así que la misma fila puede tener líneas
// de todas. El filtro por empresa vive en el join del asiento, y la condición que decide qué
// cuentas listar tiene que mirar por ahí: si suma las líneas directamente, una cuenta aparece en
// este balance porque OTRA empresa la usó. Sale con las seis columnas en blanco —los montos sí
// están filtrados—, pero su sola presencia ya cuenta que allá hubo movimiento.
//
// Lo mismo pasa con un asiento ANULADO de esta empresa: deja la fila en el balance sin un solo
// número, que es como el defecto se ve antes de que alguien entienda de dónde viene.
func TestElBalanceNoListaCuentasMovidasPorOtraEmpresaNiPorAsientosAnulados(t *testing.T) {
	repo, svc := basePeriodo(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)

	// Dos cuentas DEL GRUPO: sin empresa, las ven las tres. Hacen falta las dos porque la base ya
	// impide que una empresa use la cuenta propia de otra —ese camino está cerrado—, y lo que se
	// prueba acá es el que queda abierto: las que de verdad comparten.
	compartida, err := repo.CrearCuenta(ctx, cuenta("6-02-00-000-000", "Servicios compartidos", nil, true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear la cuenta del grupo: %v", err)
	}
	bancoGrupo, err := repo.CrearCuenta(ctx, cuenta("1-01-02-009-001", "Banco del grupo", nil, true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear el banco del grupo: %v", err)
	}

	// Coopeprofa las mueve. Valle de Paz no.
	if _, err := repo.CrearAsiento(ctx, Asiento{
		EmpresaID: empDos, Fecha: "2026-08-10", Glosa: "de la otra empresa",
		OrigenTipo: OrigenManual, Estado: EstadoAsentado,
	}, []LineaValidada{
		{CuentaID: compartida.ID, Debe: dec("7777.00")},
		{CuentaID: bancoGrupo.ID, Haber: dec("7777.00")},
	}, usrPrueba); err != nil {
		t.Fatalf("asiento de la otra empresa: %v", err)
	}

	// Y Valle de Paz tiene uno propio, pero anulado.
	anulado := asentar(t, repo, "2026-08-15", "500.00", gasto, banco)
	if err := repo.AnularAsiento(ctx, empUno, anulado.ID, "prueba", usrPrueba); err != nil {
		t.Fatalf("anular: %v", err)
	}

	b, err := svc.Balance(ctx, empUno, "2026-08-01", "2026-08-31")
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	for _, g := range b.Renglones {
		if g.Codigo == compartida.Codigo || g.Codigo == bancoGrupo.Codigo {
			t.Errorf("el balance de Valle de Paz lista %q, que solo movió la otra empresa", g.Codigo)
		}
		if g.Codigo == gasto.Codigo {
			t.Errorf("el balance lista %q, cuyo único asiento está anulado", g.Codigo)
		}
	}
	// Con todo anulado o ajeno, no queda nada que mostrar —y eso también tiene que cuadrar—.
	if len(b.Renglones) != 0 {
		t.Errorf("quedaron %d renglón(es): %+v", len(b.Renglones), b.Renglones)
	}
	if !b.Cuadra {
		t.Errorf("un balance vacío no cuadra: diferencia %q", b.Diferencia)
	}
}

// Una fecha mal escrita no llega a la base: saldría como 500 y el sistema se registraría un
// incidente a sí mismo.
func TestElBalanceRechazaFechasMalEscritas(t *testing.T) {
	_, svc := basePeriodo(t)
	ctx := context.Background()
	if _, err := svc.Balance(ctx, empUno, "hola", "2026-08-31"); !errors.Is(err, ErrFechaInvalida) {
		t.Errorf("desde basura: %v", err)
	}
	if _, err := svc.Balance(ctx, empUno, "2026-08-01", "31/08/2026"); !errors.Is(err, ErrFechaInvalida) {
		t.Errorf("hasta en formato de pantalla: %v", err)
	}
}

// ── La apertura ─────────────────────────────────────────────────────────────────────────────

func TestMontoDeCelda(t *testing.T) {
	// LOS MILES SIN DECIMALES SON EL CASO QUE ROMPÍA. «1.234.567» tomaba el último punto como
	// decimal, quedaba con dos puntos, `decimal` fallaba y el saldo entraba en CERO en silencio
	// —sobre el archivo de apertura, que es donde menos se puede—.
	casos := map[string]string{
		"1000":         "1000",
		"1.234,56":     "1234.56", // coma decimal, puntos de miles
		"1,234.56":     "1234.56", // punto decimal, comas de miles
		"₡ 1.234.567":  "1234567", // miles con punto, SIN decimales
		"1,234,567":    "1234567", // miles con coma, sin decimales
		"1.234":        "1234",    // tres dígitos detrás = miles
		"1,5":          "1.5",     // un dígito detrás = decimal
		"1,25":         "1.25",    // dos dígitos detrás = decimal
		"(500,25)":     "-500.25", // paréntesis = negativo en contabilidad
		"-500":         "-500",
		"":             "0",
		"  ":           "0",
		"no es numero": "0",
	}
	for in, quiero := range casos {
		got := montoDeCelda(in)
		if !got.Equal(dec(quiero)) {
			t.Errorf("montoDeCelda(%q) = %s, se esperaba %s", in, got, quiero)
		}
	}
}

func TestMontoLegible(t *testing.T) {
	casos := map[string]string{
		"1234567.89": "₡1.234.567,89",
		"500.00":     "₡500,00",
		"0.00":       "₡0,00",
		"-1000.50":   "-₡1.000,50",
	}
	for in, quiero := range casos {
		if got := montoLegible(in); got != quiero {
			t.Errorf("montoLegible(%q) = %q, se esperaba %q", in, got, quiero)
		}
	}
}

func TestFinDeMesDe(t *testing.T) {
	casos := map[string]string{
		"2026-01": "2026-01-31",
		"2026-02": "2026-02-28",
		"2024-02": "2024-02-29", // bisiesto
		"2026-04": "2026-04-30",
		"2026-12": "2026-12-31",
	}
	for in, quiero := range casos {
		if got := finDeMesDe(in); got != quiero {
			t.Errorf("finDeMesDe(%q) = %q, se esperaba %q", in, got, quiero)
		}
	}
}

func TestRepartirPorSigno(t *testing.T) {
	d, h := repartirPorSigno(dec("500"))
	if d != "500.00" || h != "0.00" {
		t.Errorf("positivo: debe %q, haber %q", d, h)
	}
	d, h = repartirPorSigno(dec("-500"))
	if d != "0.00" || h != "500.00" {
		t.Errorf("negativo: debe %q, haber %q", d, h)
	}
	d, h = repartirPorSigno(decimal.Zero)
	if d != "0.00" || h != "0.00" {
		t.Errorf("cero: debe %q, haber %q", d, h)
	}
}
