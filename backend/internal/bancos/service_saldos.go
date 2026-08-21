package bancos

// Tesorería: arma la foto del día a partir de los saldos capturados y de los movimientos ya
// cargados. El juicio (esperado, diferencia, cuadre) vive acá y es derivado, no almacenado.

import (
	"context"
	"sort"
	"time"

	"github.com/gpvdp/erp/internal/shared"
	"github.com/shopspring/decimal"
)

// diasSerieSaldos es el largo de la serie del disponible (la maqueta aprobada muestra 7 días).
const diasSerieSaldos = 7

// fechaValida acepta solo YYYY-MM-DD real (rechaza 2026-02-31).
func fechaValida(fecha string) bool {
	_, err := time.Parse("2006-01-02", fecha)
	return err == nil
}

// HoyCR devuelve el día de operación de Costa Rica (UTC−6, sin horario de verano).
func HoyCR() string {
	return time.Now().UTC().Add(-6 * time.Hour).Format("2006-01-02")
}

// Tesoreria arma las pantallas «Saldos del día» y «Posición de tesorería».
func (s *Service) Tesoreria(ctx context.Context, empresaID, fecha string) (Tesoreria, error) {
	if fecha == "" {
		fecha = HoyCR()
	}
	if !fechaValida(fecha) {
		return Tesoreria{}, ErrFechaInvalida
	}
	filas, hoy, err := s.repo.SaldosDelDia(ctx, empresaID, fecha)
	if err != nil {
		return Tesoreria{}, err
	}
	t := Tesoreria{Fecha: fecha, Hoy: hoy, Saldos: filas, Cuentas: len(filas)}

	// Derivar esperado, diferencia y cuadre; y acumular los totales de lo capturado.
	porMoneda := map[string]*TotalMoneda{}
	porBanco := map[string]*TotalBanco{}
	for i := range t.Saldos {
		f := &t.Saldos[i]

		m, ok := porMoneda[f.Moneda]
		if !ok {
			m = &TotalMoneda{Moneda: f.Moneda, Monto: "0"}
			porMoneda[f.Moneda] = m
		}
		m.Cuentas++
		b, ok := porBanco[f.Banco]
		if !ok {
			b = &TotalBanco{Banco: f.Banco, MontoCRC: "0", MontoUSD: "0"}
			porBanco[f.Banco] = b
		}
		b.Cuentas++

		// Mismo clasificador que el checklist: si allá dice «rezagada», acá también.
		// Una cuenta sin movimientos del mes no se cuenta como atraso de carga acá: eso lo
		// reporta el checklist, que es donde se ve el mes completo.
		switch estadoCarga(1, f.DiasSinCargar) {
		case CargaRezagada:
			t.Rezagadas++
		case CargaAtrasada:
			t.Atrasadas++
		}

		// Saldo esperado: solo tiene sentido si hay un saldo anterior contra el que acumular.
		hayAnterior := f.SaldoAnterior != ""
		if hayAnterior {
			esperado := decOrCero(f.SaldoAnterior).
				Add(decOrCero(f.EntradasDia)).
				Sub(decOrCero(f.SalidasDia))
			f.SaldoEsperado = esperado.StringFixed(2)
		}

		if f.Saldo == "" {
			t.SinCapturar++
			b.SinCapturar++
			f.Cuadre = CuadreSinCaptura
			continue
		}

		saldo := decOrCero(f.Saldo)
		m.Capturadas++
		if f.Congelado {
			t.Congeladas++
		}
		m.Monto = decOrCero(m.Monto).Add(saldo).StringFixed(2)
		if f.Moneda == "USD" {
			b.MontoUSD = decOrCero(b.MontoUSD).Add(saldo).StringFixed(2)
		} else {
			b.MontoCRC = decOrCero(b.MontoCRC).Add(saldo).StringFixed(2)
		}

		switch {
		case !hayAnterior:
			// Primer día de la cuenta: se guarda el saldo, pero no hay nada que cuadrar.
			f.Cuadre = CuadreSinAnterior
		default:
			dif := saldo.Sub(decOrCero(f.SaldoEsperado))
			f.Diferencia = dif.StringFixed(2)
			if dif.IsZero() {
				f.Cuadre = CuadreOK
			} else {
				f.Cuadre = CuadreDifiere
				t.NoCuadran++
			}
		}
	}

	// El día se considera revisado cuando TODO lo capturado está congelado (y hay algo).
	t.DiaRevisado = t.Congeladas > 0 && t.Congeladas == t.Cuentas-t.SinCapturar

	for _, m := range porMoneda {
		t.Totales = append(t.Totales, *m)
	}
	// CRC primero, después el resto por código: orden estable para la UI.
	sort.SliceStable(t.Totales, func(i, j int) bool {
		if t.Totales[i].Moneda == "CRC" {
			return true
		}
		if t.Totales[j].Moneda == "CRC" {
			return false
		}
		return t.Totales[i].Moneda < t.Totales[j].Moneda
	})
	for _, b := range porBanco {
		t.Bancos = append(t.Bancos, *b)
	}
	// De mayor a menor disponible: la concentración se lee de arriba hacia abajo.
	sort.SliceStable(t.Bancos, func(i, j int) bool {
		return decOrCero(t.Bancos[i].MontoCRC).GreaterThan(decOrCero(t.Bancos[j].MontoCRC))
	})

	if t.Serie, err = s.repo.SerieSaldos(ctx, empresaID, fecha, diasSerieSaldos); err != nil {
		return Tesoreria{}, err
	}
	if t.Serie == nil {
		t.Serie = []PuntoSaldo{}
	}
	return t, nil
}

// GuardarSaldos registra la captura del día. Valida los montos ANTES de tocar la base para no
// dejar la captura a medias por un dato mal escrito.
func (s *Service) GuardarSaldos(ctx context.Context, empresaID, fecha string, saldos []SaldoInput, usuarioID string) (int, error) {
	if fecha == "" {
		fecha = HoyCR()
	}
	if !fechaValida(fecha) {
		return 0, ErrFechaInvalida
	}
	if len(saldos) == 0 {
		return 0, ErrSinCuentas
	}
	for _, x := range saldos {
		if x.CuentaID == "" {
			return 0, ErrCuentaNoEncontrada
		}
		// El saldo puede ser negativo (un sobregiro existe), pero tiene que ser un número.
		if _, err := decimal.NewFromString(x.Saldo); err != nil {
			return 0, ErrSaldoInvalido
		}
	}
	n, err := s.repo.GuardarSaldos(ctx, empresaID, fecha, saldos, usuarioID)
	if err != nil {
		return 0, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "saldo_cuenta_diario", Accion: "CAPTURAR_SALDOS",
		UsuarioID:  &usuarioID,
		ValorNuevo: map[string]any{"fecha": fecha, "cuentas": n},
	})
	return n, nil
}

// CargaDelPeriodo devuelve el checklist de carga de estados de cuenta del mes.
func (s *Service) CargaDelPeriodo(ctx context.Context, empresaID, periodo string) ([]CargaCuenta, error) {
	if periodo == "" {
		periodo = HoyCR()[:7]
	}
	return s.repo.CargaDelPeriodo(ctx, empresaID, periodo)
}
