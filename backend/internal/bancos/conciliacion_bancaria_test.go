package bancos

// Tests de la conciliación bancaria: el acta es lo que habilita el cierre del mes, así que su
// aritmética y sus guardas quedan fijadas acá.

import (
	"context"
	"errors"
	"testing"
)

// actaBase es una cuenta con las dos puntas capturadas y movimientos del mes.
//
//	libros = 10 000 000 + 4 500 000 − 2 000 000 = 12 500 000
func actaBase() ActaConciliacion {
	return ActaConciliacion{
		CuentaID: "c1", Alias: "BN Jardines", Banco: "BN", Moneda: "CRC", Anio: 2026, Mes: 6,
		SaldoInicial: "10000000.00", FechaInicial: "2026-05-31",
		EntradasMes: "4500000.00", SalidasMes: "2000000.00",
		SaldoBanco: "12500000.00", FechaBanco: "2026-06-30",
	}
}

func TestDerivarActa(t *testing.T) {
	t.Run("cuadra cuando el banco coincide con los libros", func(t *testing.T) {
		a := actaBase()
		derivarActa(&a)
		if a.SaldoLibros != "12500000.00" {
			t.Errorf("libros = %s, quiere 12500000.00", a.SaldoLibros)
		}
		if !a.Cuadra || a.DiferenciaSinExplicar != "0.00" {
			t.Errorf("debería cuadrar: diferencia %s", a.DiferenciaSinExplicar)
		}
	})

	t.Run("una partida en tránsito cierra la diferencia", func(t *testing.T) {
		// El banco tiene ₡300 000 menos porque un depósito de libros no se acreditó.
		a := actaBase()
		a.SaldoBanco = "12200000.00"
		derivarActa(&a)
		if a.Cuadra || a.DiferenciaSinExplicar != "-300000.00" {
			t.Fatalf("sin la partida no debería cuadrar: %s", a.DiferenciaSinExplicar)
		}
		// Con el depósito no acreditado (+300 000) el acta cuadra.
		b := actaBase()
		b.SaldoBanco = "12200000.00"
		b.AjustePartidas = "300000.00"
		derivarActa(&b)
		if !b.Cuadra || b.DiferenciaSinExplicar != "0.00" {
			t.Errorf("con la partida debería cuadrar: %s", b.DiferenciaSinExplicar)
		}
	})

	t.Run("un signo equivocado NO cuadra (duplica la diferencia)", func(t *testing.T) {
		a := actaBase()
		a.SaldoBanco = "12200000.00"
		a.AjustePartidas = "-300000.00"
		derivarActa(&a)
		if a.Cuadra || a.DiferenciaSinExplicar != "-600000.00" {
			t.Errorf("diferencia = %s, quiere -600000.00", a.DiferenciaSinExplicar)
		}
	})

	t.Run("sin las dos puntas no se inventa veredicto", func(t *testing.T) {
		sinBanco := actaBase()
		sinBanco.SaldoBanco = ""
		derivarActa(&sinBanco)
		if sinBanco.Impedimento != ImpedimentoSinSaldoBanco || sinBanco.Cuadra {
			t.Errorf("sin saldo del banco: impedimento %q, cuadra %v", sinBanco.Impedimento, sinBanco.Cuadra)
		}
		sinInicial := actaBase()
		sinInicial.SaldoInicial = ""
		derivarActa(&sinInicial)
		if sinInicial.Impedimento != ImpedimentoSinSaldoInicial || sinInicial.DiferenciaSinExplicar != "" {
			t.Errorf("sin saldo inicial: impedimento %q, diferencia %q",
				sinInicial.Impedimento, sinInicial.DiferenciaSinExplicar)
		}
	})
}

func TestSignoPorTipo(t *testing.T) {
	casos := map[string]int{
		PartidaDepositoNoAcreditado:      1,
		PartidaCargoBancoNoRegistrado:    1,
		PartidaTransferenciaNoPresentada: -1,
		PartidaAbonoBancoNoRegistrado:    -1,
		PartidaOtra:                      0, // lo aporta el usuario
	}
	for tipo, quiere := range casos {
		if got := signoPorTipo(tipo); got != quiere {
			t.Errorf("signoPorTipo(%s) = %d, quiere %d", tipo, got, quiere)
		}
	}
}

func TestRegistrarPartidaValida(t *testing.T) {
	ctx := context.Background()
	base := PartidaInput{CuentaID: "c1", Anio: 2026, Mes: 6,
		Tipo: PartidaDepositoNoAcreditado, Descripcion: "Depósito del 30/06", Monto: "300000"}

	t.Run("el sistema fija el signo de los tipos conocidos", func(t *testing.T) {
		repo := &fakeRepo{}
		in := base
		in.Signo = -1 // aunque venga mal del cliente, manda el tipo
		if _, err := servicioSaldos(repo).RegistrarPartida(ctx, "e1", in, "u1"); err != nil {
			t.Fatalf("registrar: %v", err)
		}
		if repo.signoUsado != 1 {
			t.Errorf("signo guardado = %d, quiere 1 (lo fija el tipo, no el cliente)", repo.signoUsado)
		}
		if repo.partidaCreada.Monto != "300000.00" {
			t.Errorf("monto normalizado = %q", repo.partidaCreada.Monto)
		}
	})

	t.Run("«otra» exige el signo", func(t *testing.T) {
		in := base
		in.Tipo = PartidaOtra
		if _, err := servicioSaldos(&fakeRepo{}).RegistrarPartida(ctx, "e1", in, "u1"); !errors.Is(err, ErrSignoRequerido) {
			t.Errorf("err = %v, quiere ErrSignoRequerido", err)
		}
		in.Signo = -1
		repo := &fakeRepo{}
		if _, err := servicioSaldos(repo).RegistrarPartida(ctx, "e1", in, "u1"); err != nil {
			t.Fatalf("con signo debería pasar: %v", err)
		}
		if repo.signoUsado != -1 {
			t.Errorf("signo = %d, quiere -1", repo.signoUsado)
		}
	})

	t.Run("rechaza monto no positivo, tipo desconocido y descripción vacía", func(t *testing.T) {
		malos := []PartidaInput{
			func() PartidaInput { x := base; x.Monto = "0"; return x }(),
			func() PartidaInput { x := base; x.Monto = "-100"; return x }(),
			func() PartidaInput { x := base; x.Monto = "mil"; return x }(),
			func() PartidaInput { x := base; x.Tipo = "INVENTADO"; return x }(),
			func() PartidaInput { x := base; x.Descripcion = "   "; return x }(),
			func() PartidaInput { x := base; x.CuentaID = ""; return x }(),
			func() PartidaInput { x := base; x.Mes = 13; return x }(),
		}
		for i, in := range malos {
			if _, err := servicioSaldos(&fakeRepo{}).RegistrarPartida(ctx, "e1", in, "u1"); !errors.Is(err, ErrPartidaInvalida) {
				t.Errorf("caso %d: err = %v, quiere ErrPartidaInvalida", i, err)
			}
		}
	})
}

func TestFirmarActaSoloSiCuadra(t *testing.T) {
	ctx := context.Background()

	t.Run("no firma un acta con diferencia", func(t *testing.T) {
		a := actaBase()
		a.SaldoBanco = "12200000.00" // faltan 300 000 sin explicar
		repo := &fakeRepo{actas: []ActaConciliacion{a}}
		err := servicioSaldos(repo).FirmarActa(ctx, "e1", "c1", 2026, 6, "u1")
		if !errors.Is(err, ErrActaNoCuadra) {
			t.Errorf("err = %v, quiere ErrActaNoCuadra", err)
		}
		if repo.actaFirmada != "" {
			t.Error("no debería haber firmado nada")
		}
	})

	t.Run("firma con el snapshot de las cifras derivadas", func(t *testing.T) {
		repo := &fakeRepo{actas: []ActaConciliacion{actaBase()}}
		if err := servicioSaldos(repo).FirmarActa(ctx, "e1", "c1", 2026, 6, "u1"); err != nil {
			t.Fatalf("firmar: %v", err)
		}
		if repo.actaFirmada != "c1" {
			t.Errorf("acta firmada = %q", repo.actaFirmada)
		}
		if repo.snapshotFirma != [3]string{"12500000.00", "12500000.00", "0.00"} {
			t.Errorf("snapshot = %v", repo.snapshotFirma)
		}
	})

	t.Run("no firma si falta el saldo de cierre", func(t *testing.T) {
		a := actaBase()
		a.SaldoBanco = ""
		repo := &fakeRepo{actas: []ActaConciliacion{a}}
		if err := servicioSaldos(repo).FirmarActa(ctx, "e1", "c1", 2026, 6, "u1"); !errors.Is(err, ErrSaldoDelMesFaltante) {
			t.Errorf("err = %v, quiere ErrSaldoDelMesFaltante", err)
		}
	})

	t.Run("no firma en un período cerrado ni una cuenta ajena", func(t *testing.T) {
		repo := &fakeRepo{actas: []ActaConciliacion{actaBase()}, cerrado: true}
		if err := servicioSaldos(repo).FirmarActa(ctx, "e1", "c1", 2026, 6, "u1"); !errors.Is(err, ErrPeriodoYaCerrado) {
			t.Errorf("cerrado: err = %v", err)
		}
		abierto := &fakeRepo{actas: []ActaConciliacion{actaBase()}}
		if err := servicioSaldos(abierto).FirmarActa(ctx, "e1", "otra", 2026, 6, "u1"); !errors.Is(err, ErrCuentaNoEncontrada) {
			t.Errorf("cuenta ajena: err = %v", err)
		}
	})
}

func TestCierreExigeTodasLasActasFirmadas(t *testing.T) {
	ctx := context.Background()
	firmada := actaBase()
	firmada.FirmadoEn = "2026-07-01T09:00:00-06"
	pendiente := actaBase()
	pendiente.CuentaID, pendiente.Alias = "c2", "BAC Religiosa"

	t.Run("bloquea y dice qué cuentas faltan", func(t *testing.T) {
		repo := &fakeRepo{actas: []ActaConciliacion{firmada, pendiente}}
		_, err := servicioSaldos(repo).CerrarPeriodo(ctx, "e1", 2026, 6, "u1")
		if !errors.Is(err, ErrConciliacionPendiente) {
			t.Fatalf("err = %v, quiere ErrConciliacionPendiente", err)
		}
		var detalle *ErrorConciliacion
		if !errors.As(err, &detalle) {
			t.Fatal("el error debería traer el detalle de las cuentas")
		}
		if detalle.Pendientes != 1 || len(detalle.Cuentas) != 1 || detalle.Cuentas[0] != "BAC Religiosa" {
			t.Errorf("detalle = %+v", detalle)
		}
	})

	t.Run("con todas firmadas, cierra", func(t *testing.T) {
		otra := firmada
		otra.CuentaID = "c2"
		repo := &fakeRepo{actas: []ActaConciliacion{firmada, otra}}
		if _, err := servicioSaldos(repo).CerrarPeriodo(ctx, "e1", 2026, 6, "u1"); err != nil {
			t.Errorf("debería cerrar: %v", err)
		}
	})

	t.Run("sin cuentas activas no hay nada que conciliar", func(t *testing.T) {
		if _, err := servicioSaldos(&fakeRepo{}).CerrarPeriodo(ctx, "e1", 2026, 6, "u1"); err != nil {
			t.Errorf("no debería bloquear: %v", err)
		}
	})
}

func TestConciliacionSemaforo(t *testing.T) {
	firmada := actaBase()
	firmada.FirmadoEn = "2026-07-01T09:00:00-06"
	difiere := actaBase()
	difiere.CuentaID, difiere.SaldoBanco = "c2", "12200000.00"
	incompleta := actaBase()
	incompleta.CuentaID, incompleta.SaldoBanco = "c3", ""
	cuadra := actaBase()
	cuadra.CuentaID = "c4"

	repo := &fakeRepo{actas: []ActaConciliacion{firmada, difiere, incompleta, cuadra}}
	c, err := servicioSaldos(repo).Conciliacion(context.Background(), "e1", 2026, 6)
	if err != nil {
		t.Fatalf("conciliación: %v", err)
	}
	if c.Cuentas != 4 || c.Firmadas != 1 || c.ConDiferencia != 1 || c.Incompletas != 1 || c.Cuadran != 1 {
		t.Errorf("semáforo: %d cuentas, %d firmadas, %d con diferencia, %d incompletas, %d cuadran",
			c.Cuentas, c.Firmadas, c.ConDiferencia, c.Incompletas, c.Cuadran)
	}
	if c.PuedeCerrar {
		t.Error("no debería poder cerrar con 3 actas sin firmar")
	}
	if c.Periodo != "2026-06" {
		t.Errorf("período = %q", c.Periodo)
	}
	// Las partidas nunca llegan nil al cliente (la UI itera sin defensas).
	for _, a := range c.Actas {
		if a.Partidas == nil {
			t.Errorf("cuenta %s: partidas nil", a.CuentaID)
		}
	}
}

func TestRevisarSaldosValidaFecha(t *testing.T) {
	svc := servicioSaldos(&fakeRepo{})
	if _, err := svc.RevisarSaldos(context.Background(), "e1", "2026-02-31", true, "", "u1"); !errors.Is(err, ErrFechaInvalida) {
		t.Errorf("err = %v, quiere ErrFechaInvalida", err)
	}
	repo := &fakeRepo{}
	if _, err := servicioSaldos(repo).RevisarSaldos(context.Background(), "e1", "", true, "", "u1"); err != nil {
		t.Fatalf("sin fecha: %v", err)
	}
	if repo.fechaRevisada != HoyCR() || !repo.congelo {
		t.Errorf("fecha revisada = %q (quiere %q), congeló = %v", repo.fechaRevisada, HoyCR(), repo.congelo)
	}
}
