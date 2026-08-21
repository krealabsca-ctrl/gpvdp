package bancos

// Tests del cuadre diario: es el control que detecta movimientos faltantes, así que su
// comportamiento queda fijado acá.

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

func servicioSaldos(repo *fakeRepo) *Service {
	return NewService(repo, nil, zap.NewNop(), true)
}

func TestTesoreriaCuadre(t *testing.T) {
	repo := &fakeRepo{
		hoyCR: "2026-07-30",
		saldosDia: []SaldoDelDia{
			// Cuadra: 10 000 000 + 1 200 000 − 700 000 = 10 500 000.
			{CuentaID: "c1", Alias: "BN Colones", Banco: "BN", Moneda: "CRC",
				SaldoAnterior: "10000000.00", FechaAnterior: "2026-07-29",
				EntradasDia: "1200000.00", SalidasDia: "700000.00", Saldo: "10500000.00",
				UltimoMovimiento: "2026-07-30", DiasSinCargar: 0},
			// No cuadra: faltan ₡148 500 de movimientos.
			{CuentaID: "c2", Alias: "BAC Religiosa", Banco: "BAC", Moneda: "CRC",
				SaldoAnterior: "5000000.00", FechaAnterior: "2026-07-29",
				EntradasDia: "300000.00", SalidasDia: "100000.00", Saldo: "5051500.00",
				UltimoMovimiento: "2026-07-30", DiasSinCargar: 0},
			// Sin capturar: no suma al disponible y se cuenta como pendiente.
			{CuentaID: "c3", Alias: "BP Colones", Banco: "Banco Popular", Moneda: "CRC",
				SaldoAnterior: "2000000.00", FechaAnterior: "2026-07-29",
				EntradasDia: "0", SalidasDia: "0", Saldo: "",
				UltimoMovimiento: "2026-07-10", DiasSinCargar: 20},
			// Primer día de la cuenta: se guarda el saldo pero no hay nada que cuadrar.
			{CuentaID: "c4", Alias: "Davivienda Dólares", Banco: "Davivienda", Moneda: "USD",
				SaldoAnterior: "", EntradasDia: "0", SalidasDia: "0", Saldo: "18400.00",
				UltimoMovimiento: "2026-07-28", DiasSinCargar: 2},
		},
	}
	t.Run("deriva esperado, diferencia y veredicto", func(t *testing.T) {
		tes, err := servicioSaldos(repo).Tesoreria(context.Background(), "e1", "2026-07-30")
		if err != nil {
			t.Fatalf("tesorería: %v", err)
		}
		if tes.Saldos[0].SaldoEsperado != "10500000.00" || tes.Saldos[0].Cuadre != CuadreOK {
			t.Errorf("c1: esperado %s, cuadre %s", tes.Saldos[0].SaldoEsperado, tes.Saldos[0].Cuadre)
		}
		if tes.Saldos[1].Cuadre != CuadreDifiere || tes.Saldos[1].Diferencia != "-148500.00" {
			t.Errorf("c2: cuadre %s, diferencia %s (quiere DIFIERE y -148500.00)",
				tes.Saldos[1].Cuadre, tes.Saldos[1].Diferencia)
		}
		if tes.Saldos[2].Cuadre != CuadreSinCaptura {
			t.Errorf("c3 sin capturar: cuadre %s", tes.Saldos[2].Cuadre)
		}
		if tes.Saldos[3].Cuadre != CuadreSinAnterior || tes.Saldos[3].SaldoEsperado != "" {
			t.Errorf("c4 primer día: cuadre %s, esperado %q", tes.Saldos[3].Cuadre, tes.Saldos[3].SaldoEsperado)
		}
		if tes.SinCapturar != 1 || tes.NoCuadran != 1 || tes.Cuentas != 4 {
			t.Errorf("resumen: %d cuentas, %d sin capturar, %d no cuadran", tes.Cuentas, tes.SinCapturar, tes.NoCuadran)
		}
		// Mismos cortes que el checklist: 20 días es REZAGADA (>14), no «atrasada».
		if tes.Rezagadas != 1 || tes.Atrasadas != 0 {
			t.Errorf("carga: %d rezagadas y %d atrasadas, quiere 1 y 0 (la cuenta de 20 días)",
				tes.Rezagadas, tes.Atrasadas)
		}
	})

	t.Run("los dólares NO se suman a los colones", func(t *testing.T) {
		tes, _ := servicioSaldos(repo).Tesoreria(context.Background(), "e1", "2026-07-30")
		porMoneda := map[string]TotalMoneda{}
		for _, m := range tes.Totales {
			porMoneda[m.Moneda] = m
		}
		// CRC: solo las dos capturadas (10 500 000 + 5 051 500). La sin capturar no suma.
		if porMoneda["CRC"].Monto != "15551500.00" {
			t.Errorf("total CRC = %s, quiere 15551500.00", porMoneda["CRC"].Monto)
		}
		if porMoneda["USD"].Monto != "18400.00" {
			t.Errorf("total USD = %s, quiere 18400.00", porMoneda["USD"].Monto)
		}
		if porMoneda["CRC"].Capturadas != 2 || porMoneda["CRC"].Cuentas != 3 {
			t.Errorf("CRC: %d capturadas de %d", porMoneda["CRC"].Capturadas, porMoneda["CRC"].Cuentas)
		}
		if tes.Totales[0].Moneda != "CRC" {
			t.Errorf("los colones van primero, vino %s", tes.Totales[0].Moneda)
		}
	})

	t.Run("la concentración por banco se ordena de mayor a menor", func(t *testing.T) {
		tes, _ := servicioSaldos(repo).Tesoreria(context.Background(), "e1", "2026-07-30")
		if len(tes.Bancos) != 4 || tes.Bancos[0].Banco != "BN" {
			t.Errorf("bancos = %+v", tes.Bancos)
		}
		// El banco con la cuenta sin capturar lo declara.
		for _, b := range tes.Bancos {
			if b.Banco == "Banco Popular" && b.SinCapturar != 1 {
				t.Errorf("Banco Popular debería declarar 1 cuenta sin capturar, tiene %d", b.SinCapturar)
			}
		}
	})
}

func TestGuardarSaldosValida(t *testing.T) {
	svc := servicioSaldos(&fakeRepo{})
	ctx := context.Background()

	if _, err := svc.GuardarSaldos(ctx, "e1", "2026-13-01", []SaldoInput{{CuentaID: "c1", Saldo: "1"}}, "u1"); !errors.Is(err, ErrFechaInvalida) {
		t.Errorf("fecha inválida: err = %v", err)
	}
	if _, err := svc.GuardarSaldos(ctx, "e1", "2026-02-31", []SaldoInput{{CuentaID: "c1", Saldo: "1"}}, "u1"); !errors.Is(err, ErrFechaInvalida) {
		t.Errorf("31 de febrero: err = %v, quiere ErrFechaInvalida", err)
	}
	if _, err := svc.GuardarSaldos(ctx, "e1", "2026-07-30", nil, "u1"); !errors.Is(err, ErrSinCuentas) {
		t.Errorf("sin saldos: err = %v", err)
	}
	if _, err := svc.GuardarSaldos(ctx, "e1", "2026-07-30", []SaldoInput{{CuentaID: "c1", Saldo: "mil"}}, "u1"); !errors.Is(err, ErrSaldoInvalido) {
		t.Errorf("saldo no numérico: err = %v", err)
	}
	// Un sobregiro es un saldo válido.
	if _, err := svc.GuardarSaldos(ctx, "e1", "2026-07-30", []SaldoInput{{CuentaID: "c1", Saldo: "-45000.50"}}, "u1"); err != nil {
		t.Errorf("saldo negativo (sobregiro) debería aceptarse: %v", err)
	}
	// Sin fecha se asume el día de Costa Rica, no un error.
	repo := &fakeRepo{}
	if _, err := servicioSaldos(repo).GuardarSaldos(ctx, "e1", "", []SaldoInput{{CuentaID: "c1", Saldo: "10"}}, "u1"); err != nil {
		t.Fatalf("sin fecha: %v", err)
	}
	if repo.fechaGuardada != HoyCR() {
		t.Errorf("fecha por defecto = %q, quiere %q", repo.fechaGuardada, HoyCR())
	}
}

func TestEstadoCarga(t *testing.T) {
	casos := []struct {
		movs, dias int
		quiere     string
	}{
		{0, 0, CargaSinCarga},
		{100, 0, CargaAlDia},
		{100, 7, CargaAlDia},
		{100, 8, CargaAtrasada},
		{100, 14, CargaAtrasada},
		{100, 15, CargaRezagada},
		{2, 21, CargaRezagada}, // el caso real: BN Jardines Dólares
	}
	for _, c := range casos {
		if got := estadoCarga(c.movs, c.dias); got != c.quiere {
			t.Errorf("estadoCarga(%d movs, %d días) = %s, quiere %s", c.movs, c.dias, got, c.quiere)
		}
	}
}
