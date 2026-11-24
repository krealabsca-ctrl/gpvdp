package conta

// El asiento contra Postgres (mig 0111, Fase 3A).
//
// LO QUE SE PRUEBA ACÁ es lo que un doble no puede: la partida doble la garantiza un disparador
// DIFERIDO de la base, no el código de Go. Y la prueba tiene que ser diferida también — las
// líneas entran una por una, así que inmediatamente después de la primera el asiento está
// descuadrado por construcción. Una prueba que no abra transacción estaría midiendo otra cosa.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var tablasDelAsiento = []string{
	"empresa", "usuario", "cuenta_contable",
	"banco", "cuenta_bancaria", "movimiento_bancario",
	"mapeo_cuenta_bancaria", "conta_consecutivo", "asiento", "asiento_linea",
}

const (
	disparadorLineaCuenta = `
		CREATE TRIGGER trg_linea_cuenta_de_movimiento
		BEFORE INSERT OR UPDATE ON %s.asiento_linea
		FOR EACH ROW EXECUTE FUNCTION public.fn_linea_cuenta_de_movimiento()`

	disparadorMapeoCB = `
		CREATE TRIGGER trg_mapeo_cb_cuenta_de_movimiento
		BEFORE INSERT OR UPDATE ON %s.mapeo_cuenta_bancaria
		FOR EACH ROW EXECUTE FUNCTION public.fn_mapeo_cuenta_de_movimiento()`

	// Los dos CONSTRAINT TRIGGER, con su DEFERRABLE INITIALLY DEFERRED. Copiarlos sin el
	// `DEFERRABLE` haría que las pruebas de abajo fallaran por una razón equivocada: rechazarían
	// el asiento bueno en la primera línea.
	disparadorCuadraCab = `
		CREATE CONSTRAINT TRIGGER trg_asiento_cuadra_cab
		AFTER INSERT OR UPDATE ON %s.asiento
		DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW EXECUTE FUNCTION public.fn_asiento_cuadra_cab()`

	disparadorCuadraLin = `
		CREATE CONSTRAINT TRIGGER trg_asiento_cuadra_lin
		AFTER INSERT OR UPDATE OR DELETE ON %s.asiento_linea
		DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW EXECUTE FUNCTION public.fn_asiento_cuadra_lin()`
)

func baseAsiento(t *testing.T) *pgRepository {
	t.Helper()
	return baseConta(t, tablasDelAsiento, []string{
		disparadorDelArbol, disparadorMapeoCB, disparadorCuadraCab, disparadorCuadraLin,
		disparadorLineaCuenta,
	})
}

// dosCuentas siembra una de gasto y una de banco, que es el par mínimo para un asiento.
func dosCuentas(t *testing.T, r *pgRepository) (gasto, banco Cuenta) {
	t.Helper()
	ctx := context.Background()
	var err error
	gasto, err = r.CrearCuenta(ctx, cuenta("6-01-00-000-000", "Gastos generales", idDe(empUno), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear cuenta de gasto: %v", err)
	}
	banco, err = r.CrearCuenta(ctx, cuenta("1-01-02-001-001", "Banco Nacional Colones CR15015108710010019890", idDe(empUno), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear cuenta de banco: %v", err)
	}
	return gasto, banco
}

func dec(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}

// LA INVARIANTE DEL LIBRO, hecha cumplir por la base.
//
// Un asiento descuadrado no es un asiento con un error: es dinero que aparece o desaparece. Si
// entra, el balance deja de cuadrar y nadie sabe desde cuándo ni por cuánto.
func TestLaBaseRechazaUnAsientoDescuadrado(t *testing.T) {
	repo := baseAsiento(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)

	base := Asiento{EmpresaID: empUno, Fecha: "2026-10-27", Glosa: "prueba", OrigenTipo: OrigenManual, Estado: EstadoAsentado}

	t.Run("cuadrado entra", func(t *testing.T) {
		_, err := repo.CrearAsiento(ctx, base, []LineaValidada{
			{CuentaID: gasto.ID, Debe: dec("1000.00")},
			{CuentaID: banco.ID, Haber: dec("1000.00")},
		}, usrPrueba)
		if err != nil {
			t.Fatalf("el asiento bueno no entró: %v", err)
		}
	})

	t.Run("descuadrado por cien colones", func(t *testing.T) {
		_, err := repo.CrearAsiento(ctx, base, []LineaValidada{
			{CuentaID: gasto.ID, Debe: dec("1000.00")},
			{CuentaID: banco.ID, Haber: dec("900.00")},
		}, usrPrueba)
		if !errors.Is(err, ErrAsientoDescuadrado) {
			t.Fatalf("LA BASE ACEPTÓ un asiento descuadrado: %v", err)
		}
		// Y dice de cuánto es la diferencia. «No cuadra» a secas obliga a sumar a mano.
		if !strings.Contains(err.Error(), "100") {
			t.Errorf("el error no dice la diferencia: %v", err)
		}
	})

	t.Run("una sola línea", func(t *testing.T) {
		_, err := repo.CrearAsiento(ctx, base, []LineaValidada{
			{CuentaID: gasto.ID, Debe: dec("1000.00")},
		}, usrPrueba)
		if !errors.Is(err, ErrAsientoSinLineas) {
			t.Fatalf("LA BASE ACEPTÓ un asiento de una sola línea: %v", err)
		}
	})

	t.Run("sin líneas", func(t *testing.T) {
		_, err := repo.CrearAsiento(ctx, base, nil, usrPrueba)
		if !errors.Is(err, ErrAsientoSinLineas) {
			t.Fatalf("LA BASE ACEPTÓ un asiento sin líneas: %v", err)
		}
	})

	t.Run("y lo rechazado no quedó a medias", func(t *testing.T) {
		// La transacción entera se revierte: lo contrario dejaría cabeceras huérfanas y huecos
		// en la numeración, que es justo lo que el consecutivo existe para evitar.
		vivos, err := repo.Diario(ctx, empUno, FiltroDiario{})
		if err != nil {
			t.Fatalf("leer el diario: %v", err)
		}
		if len(vivos) != 1 {
			t.Errorf("quedaron %d asientos; solo el bueno tenía que entrar", len(vivos))
		}
	})
}

// UN ASIENTO NO SE ESCRIBE CONTRA UNA CUENTA DE AGRUPACIÓN.
//
// Es la misma regla que vigila el mapeo (mig 0110) y acá importa más: el mapeo dice a qué cuenta
// IRÍA una partida, pero la línea de asiento es la que de verdad mueve el saldo. Contra una
// cuenta madre, el monto se cuenta dos veces al totalizar el árbol —una en la madre, otra en la
// rama— y el balance deja de cuadrar sin que nada avise.
//
// SE ESCAPÓ EN LA PRIMERA PASADA: el disparador estaba en las dos tablas de mapeo y no en la
// línea. Lo encontró una prueba contra el sistema corriendo, no esta suite.
func TestNoSeAsientaContraUnaCuentaDeAgrupacion(t *testing.T) {
	repo := baseAsiento(t)
	ctx := context.Background()
	gasto, _ := dosCuentas(t, repo)

	madre, err := repo.CrearCuenta(ctx, cuenta("1-00-00-000-000", "ACTIVOS", idDe(empUno), false, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear la madre: %v", err)
	}
	if _, err := repo.CrearCuenta(ctx, cuenta("1-01-00-000-000", "ACTIVOS CORRIENTES", idDe(empUno), true, madre.Codigo), usrPrueba); err != nil {
		t.Fatalf("crear la hija: %v", err)
	}
	ajena, err := repo.CrearCuenta(ctx, cuenta("6-09-00-000-000", "Gastos de la otra", idDe(empDos), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear la ajena: %v", err)
	}

	base := Asiento{EmpresaID: empUno, Fecha: "2026-10-27", Glosa: "prueba", OrigenTipo: OrigenManual, Estado: EstadoAsentado}

	t.Run("cuenta de agrupación", func(t *testing.T) {
		_, err := repo.CrearAsiento(ctx, base, []LineaValidada{
			{CuentaID: madre.ID, Debe: dec("1000.00")},
			{CuentaID: gasto.ID, Haber: dec("1000.00")},
		}, usrPrueba)
		if !errors.Is(err, ErrCuentaDeAgrupacion) {
			t.Fatalf("SE ASENTÓ contra una cuenta de agrupación: %v", err)
		}
		if !strings.Contains(err.Error(), madre.Codigo) {
			t.Errorf("el error no dice qué cuenta: %v", err)
		}
	})

	t.Run("cuenta de otra empresa", func(t *testing.T) {
		_, err := repo.CrearAsiento(ctx, base, []LineaValidada{
			{CuentaID: ajena.ID, Debe: dec("1000.00")},
			{CuentaID: gasto.ID, Haber: dec("1000.00")},
		}, usrPrueba)
		if err == nil {
			t.Fatal("se asentó contra la cuenta de OTRA empresa: cruzaría dos contabilidades")
		}
		if !strings.Contains(err.Error(), "otra empresa") {
			t.Errorf("el error no dice qué pasó: %v", err)
		}
	})
}

// Borrar o cambiar líneas DESPUÉS, en otra transacción, también tiene que cuadrar. Sin el
// disparador sobre la línea, un asiento bueno se podría descuadrar al día siguiente.
func TestNoSePuedeDescuadrarUnAsientoYaEscrito(t *testing.T) {
	repo := baseAsiento(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)

	a, err := repo.CrearAsiento(ctx, Asiento{
		EmpresaID: empUno, Fecha: "2026-10-27", Glosa: "prueba", OrigenTipo: OrigenManual, Estado: EstadoAsentado,
	}, []LineaValidada{
		{CuentaID: gasto.ID, Debe: dec("1000.00")},
		{CuentaID: banco.ID, Haber: dec("1000.00")},
	}, usrPrueba)
	if err != nil {
		t.Fatalf("crear: %v", err)
	}

	t.Run("borrar una línea", func(t *testing.T) {
		_, err := repo.pool.Exec(ctx,
			`DELETE FROM asiento_linea WHERE asiento_id = $1::uuid AND haber > 0`, a.ID)
		if err == nil {
			t.Fatal("se borró una línea y el asiento quedó descuadrado")
		}
	})

	t.Run("cambiarle el monto a una línea", func(t *testing.T) {
		_, err := repo.pool.Exec(ctx,
			`UPDATE asiento_linea SET haber = 1 WHERE asiento_id = $1::uuid AND haber > 0`, a.ID)
		if err == nil {
			t.Fatal("se cambió un monto y el asiento quedó descuadrado")
		}
	})

	t.Run("borrar el asiento entero SÍ se puede", func(t *testing.T) {
		// Se lleva sus líneas por CASCADE y no hay nada que cuadrar. Si el disparador no tuviera
		// la salida por «el asiento ya no existe», esto fallaría al commit.
		if _, err := repo.pool.Exec(ctx, `DELETE FROM asiento WHERE id = $1::uuid`, a.ID); err != nil {
			t.Fatalf("no se pudo borrar un asiento entero: %v", err)
		}
	})
}

// IDEMPOTENCIA: un documento tiene UN asiento vivo. Regenerar desde el mismo documento no puede
// duplicar el gasto; pero después de anular, SÍ hay que poder volver a generarlo.
func TestUnDocumentoTieneUnAsientoVivo(t *testing.T) {
	repo := baseAsiento(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)

	doc := "44444444-4444-4444-8444-444444444444"
	desdeDoc := Asiento{
		EmpresaID: empUno, Fecha: "2026-10-27", Glosa: "desde el movimiento",
		OrigenTipo: OrigenMovimiento, OrigenID: &doc, Estado: EstadoAsentado,
	}
	lineas := []LineaValidada{
		{CuentaID: gasto.ID, Debe: dec("500.00")},
		{CuentaID: banco.ID, Haber: dec("500.00")},
	}

	primero, err := repo.CrearAsiento(ctx, desdeDoc, lineas, usrPrueba)
	if err != nil {
		t.Fatalf("primera generación: %v", err)
	}
	if _, err := repo.CrearAsiento(ctx, desdeDoc, lineas, usrPrueba); err == nil {
		t.Fatal("el mismo documento generó DOS asientos: el gasto entraría dos veces")
	}

	// Anulado, el documento queda libre para regenerarse. Sin esto, un asiento mal generado
	// bloquearía su documento para siempre.
	if err := repo.AnularAsiento(ctx, empUno, primero.ID, "se generó con la cuenta equivocada", usrPrueba); err != nil {
		t.Fatalf("anular: %v", err)
	}
	if _, err := repo.CrearAsiento(ctx, desdeDoc, lineas, usrPrueba); err != nil {
		t.Errorf("después de anular no se pudo regenerar: %v", err)
	}

	// Anular dos veces no es anular más.
	if err := repo.AnularAsiento(ctx, empUno, primero.ID, "otra vez por si acaso", usrPrueba); !errors.Is(err, ErrAsientoYaAnulado) {
		t.Errorf("anular dos veces no avisó: %v", err)
	}
	// Y el asiento sigue en el libro: no se borra.
	todos, err := repo.Diario(ctx, empUno, FiltroDiario{})
	if err != nil {
		t.Fatalf("diario: %v", err)
	}
	if len(todos) != 2 {
		t.Errorf("hay %d asientos en el libro; el anulado tiene que seguir visible", len(todos))
	}
}

// El consecutivo no repite. Un libro diario se cita por número.
func TestElConsecutivoNoRepite(t *testing.T) {
	repo := baseAsiento(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)

	vistos := map[int64]bool{}
	for i := 0; i < 5; i++ {
		a, err := repo.CrearAsiento(ctx, Asiento{
			EmpresaID: empUno, Fecha: "2026-10-27", Glosa: "prueba", OrigenTipo: OrigenManual, Estado: EstadoAsentado,
		}, []LineaValidada{
			{CuentaID: gasto.ID, Debe: dec("10.00")},
			{CuentaID: banco.ID, Haber: dec("10.00")},
		}, usrPrueba)
		if err != nil {
			t.Fatalf("crear %d: %v", i, err)
		}
		if vistos[a.Numero] {
			t.Fatalf("el número %d salió dos veces", a.Numero)
		}
		vistos[a.Numero] = true
	}
	if !vistos[1] || !vistos[5] {
		t.Errorf("la numeración no arrancó en 1 o tiene huecos: %v", vistos)
	}
}

// EL MAYOR TIENE QUE ARRANCAR DEL SALDO ANTERIOR. Sin eso, el mayor de un mes arranca en cero y
// su saldo final es el del mes, no el de la cuenta: el número que alguien copie a un balance
// estaría mal y parecería bien.
func TestElMayorArrastraElSaldoAnterior(t *testing.T) {
	repo := baseAsiento(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)

	asentar := func(fecha, monto string) {
		t.Helper()
		if _, err := repo.CrearAsiento(ctx, Asiento{
			EmpresaID: empUno, Fecha: fecha, Glosa: "compra", OrigenTipo: OrigenManual, Estado: EstadoAsentado,
		}, []LineaValidada{
			{CuentaID: gasto.ID, Debe: dec(monto)},
			{CuentaID: banco.ID, Haber: dec(monto)},
		}, usrPrueba); err != nil {
			t.Fatalf("asentar %s: %v", fecha, err)
		}
	}
	asentar("2026-09-10", "300.00")
	asentar("2026-10-05", "100.00")
	asentar("2026-10-20", "200.00")

	m, err := repo.Mayor(ctx, empUno, FiltroMayor{CuentaID: gasto.ID, Desde: "2026-10-01", Hasta: "2026-10-31"})
	if err != nil {
		t.Fatalf("mayor: %v", err)
	}
	if m.SaldoInicial != "300.00" {
		t.Errorf("saldo inicial %q, se esperaba 300.00 (lo de setiembre)", m.SaldoInicial)
	}
	if len(m.Renglones) != 2 {
		t.Fatalf("vinieron %d renglones en octubre, se esperaban 2", len(m.Renglones))
	}
	if m.Renglones[0].Saldo != "400.00" || m.Renglones[1].Saldo != "600.00" {
		t.Errorf("el saldo no corre: %q y %q", m.Renglones[0].Saldo, m.Renglones[1].Saldo)
	}
	if m.SaldoFinal != "600.00" {
		t.Errorf("saldo final %q, se esperaba 600.00", m.SaldoFinal)
	}

	// EL SIGNO SIGUE A LA NATURALEZA. El banco es activo (deudora) y acá solo recibió créditos,
	// así que su saldo baja; el gasto es deudora y sube. Si el mayor restara siempre
	// debe-menos-haber, todos los pasivos saldrían en negativo y nadie los leería así.
	mb, err := repo.Mayor(ctx, empUno, FiltroMayor{CuentaID: banco.ID})
	if err != nil {
		t.Fatalf("mayor del banco: %v", err)
	}
	if mb.SaldoFinal != "-600.00" {
		t.Errorf("saldo del banco %q, se esperaba -600.00", mb.SaldoFinal)
	}
}

// Un asiento ANULADO no mueve saldo. Está en el libro para que se vea que existió.
func TestElAnuladoNoMueveSaldo(t *testing.T) {
	repo := baseAsiento(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)

	a, err := repo.CrearAsiento(ctx, Asiento{
		EmpresaID: empUno, Fecha: "2026-10-27", Glosa: "se va a anular", OrigenTipo: OrigenManual, Estado: EstadoAsentado,
	}, []LineaValidada{
		{CuentaID: gasto.ID, Debe: dec("750.00")},
		{CuentaID: banco.ID, Haber: dec("750.00")},
	}, usrPrueba)
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	antes, err := repo.Mayor(ctx, empUno, FiltroMayor{CuentaID: gasto.ID})
	if err != nil || antes.SaldoFinal != "750.00" {
		t.Fatalf("saldo antes de anular %q (%v)", antes.SaldoFinal, err)
	}

	if err := repo.AnularAsiento(ctx, empUno, a.ID, "la factura venía duplicada del proveedor", usrPrueba); err != nil {
		t.Fatalf("anular: %v", err)
	}
	despues, err := repo.Mayor(ctx, empUno, FiltroMayor{CuentaID: gasto.ID})
	if err != nil {
		t.Fatalf("mayor: %v", err)
	}
	if despues.SaldoFinal != "0.00" {
		t.Errorf("el asiento anulado sigue moviendo saldo: %q", despues.SaldoFinal)
	}
	if len(despues.Renglones) != 0 {
		t.Errorf("el anulado sigue apareciendo en el mayor: %d renglones", len(despues.Renglones))
	}
}

// Un asiento de OTRA empresa no se ve, no se lee y no se anula.
func TestElAsientoNoCruzaEmpresas(t *testing.T) {
	repo := baseAsiento(t)
	ctx := context.Background()
	dosCuentas(t, repo) // las de empUno, para que el diario de empUno tenga con qué compararse

	// La otra empresa necesita SU propio par de cuentas: desde que la línea exige que la cuenta
	// sea visible para la empresa del asiento, no se puede asentar con las ajenas.
	gastoDos, err := repo.CrearCuenta(ctx, cuenta("6-01-00-000-000", "Gastos generales", idDe(empDos), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear gasto de la otra: %v", err)
	}
	bancoDos, err := repo.CrearCuenta(ctx, cuenta("1-01-02-001-001", "Banco de la otra", idDe(empDos), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear banco de la otra: %v", err)
	}

	ajeno, err := repo.CrearAsiento(ctx, Asiento{
		EmpresaID: empDos, Fecha: "2026-10-27", Glosa: "de la otra", OrigenTipo: OrigenManual, Estado: EstadoAsentado,
	}, []LineaValidada{
		{CuentaID: gastoDos.ID, Debe: dec("100.00")},
		{CuentaID: bancoDos.ID, Haber: dec("100.00")},
	}, usrPrueba)
	if err != nil {
		t.Fatalf("crear en la otra empresa: %v", err)
	}

	mios, err := repo.Diario(ctx, empUno, FiltroDiario{})
	if err != nil {
		t.Fatalf("diario: %v", err)
	}
	if len(mios) != 0 {
		t.Errorf("se vieron %d asientos de OTRA empresa", len(mios))
	}
	if _, err := repo.Asiento(ctx, empUno, ajeno.ID); !errors.Is(err, ErrAsientoNoEncontrado) {
		t.Errorf("se leyó el asiento de otra empresa: %v", err)
	}
	if err := repo.AnularAsiento(ctx, empUno, ajeno.ID, "no debería poder anularlo nunca", usrPrueba); !errors.Is(err, ErrAsientoNoEncontrado) {
		t.Errorf("se anuló el asiento de otra empresa: %v", err)
	}
}

// Una fecha mal escrita no puede salir como 500. Reventaría en Postgres y el sistema se
// registraría un incidente a sí mismo; ya pasó una vez, en el módulo de incidentes.
func TestUnaFechaMalEscritaNoRevienta(t *testing.T) {
	repo := baseAsiento(t)
	svc := NewService(repo, nil, zap.NewNop())
	ctx := context.Background()

	if _, err := svc.Diario(ctx, empUno, FiltroDiario{Desde: "hola"}); !errors.Is(err, ErrFechaInvalida) {
		t.Errorf("diario con fecha basura: %v", err)
	}
	if _, err := svc.Mayor(ctx, empUno, FiltroMayor{CuentaID: "x", Hasta: "27/10/2026"}); !errors.Is(err, ErrFechaInvalida) {
		t.Errorf("mayor con fecha en formato de pantalla: %v", err)
	}
}
