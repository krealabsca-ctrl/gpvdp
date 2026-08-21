package bancos

// Conciliación bancaria mensual. El saldo de libros y la diferencia sin explicar se DERIVAN
// acá: si entra un movimiento que faltaba, el acta se corrige sola mientras no esté firmada.
// Al firmar se guarda el snapshot, porque a partir de ahí es un documento.

import (
	"context"
	"fmt"
	"strings"

	"github.com/gpvdp/erp/internal/shared"
	"github.com/shopspring/decimal"
)

// Conciliacion arma las actas del mes de todas las cuentas activas.
func (s *Service) Conciliacion(ctx context.Context, empresaID string, anio, mes int) (Conciliacion, error) {
	if !periodoValidoBancos(anio, mes) {
		return Conciliacion{}, ErrFechaInvalida
	}
	actas, err := s.repo.ActasDelMes(ctx, empresaID, anio, mes)
	if err != nil {
		return Conciliacion{}, err
	}
	partidas, err := s.repo.PartidasDelMes(ctx, empresaID, "", anio, mes)
	if err != nil {
		return Conciliacion{}, err
	}
	porCuenta := map[string][]PartidaConciliacion{}
	for _, p := range partidas {
		porCuenta[p.CuentaID] = append(porCuenta[p.CuentaID], p)
	}

	c := Conciliacion{Anio: anio, Mes: mes, Periodo: periodoTexto(anio, mes), Actas: actas}
	for i := range c.Actas {
		a := &c.Actas[i]
		a.Partidas = porCuenta[a.CuentaID]
		if a.Partidas == nil {
			a.Partidas = []PartidaConciliacion{}
		}
		derivarActa(a)
	}
	c.Cuentas = len(c.Actas)
	for _, a := range c.Actas {
		switch {
		case a.FirmadoEn != "":
			c.Firmadas++
		case a.Impedimento != "":
			c.Incompletas++
		case a.Cuadra:
			c.Cuadran++
		default:
			c.ConDiferencia++
		}
	}
	// «Cerrar todo e identificar todo»: el mes cierra solo si TODAS las actas están firmadas.
	// Una empresa sin cuentas activas no tiene nada que conciliar, así que no bloquea.
	c.PuedeCerrar = c.Firmadas == c.Cuentas
	if cerrado, err := s.repo.PeriodoCerrado(ctx, empresaID, anio, mes); err == nil {
		c.Cerrado = cerrado
	} else {
		return Conciliacion{}, err
	}
	return c, nil
}

// derivarActa calcula el saldo de libros, el ajuste y la diferencia sin explicar.
//
//	libros    = saldo inicial + entradas − salidas (movimientos ya cargados)
//	diferencia = (banco + ajuste de partidas) − libros
//
// Cero = el acta cuadra. Un acta ya firmada conserva su veredicto: cuadró cuando se firmó.
func derivarActa(a *ActaConciliacion) {
	switch {
	case a.SaldoBanco == "":
		a.Impedimento = ImpedimentoSinSaldoBanco
	case a.SaldoInicial == "":
		a.Impedimento = ImpedimentoSinSaldoInicial
	}
	if a.AjustePartidas == "" {
		a.AjustePartidas = "0.00"
	}
	if a.Impedimento != "" {
		// Sin las dos puntas no hay conciliación que hacer: no se inventa un veredicto.
		return
	}
	libros := decOrCero(a.SaldoInicial).
		Add(decOrCero(a.EntradasMes)).
		Sub(decOrCero(a.SalidasMes))
	a.SaldoLibros = libros.StringFixed(2)
	a.AjustePartidas = decOrCero(a.AjustePartidas).StringFixed(2)
	dif := decOrCero(a.SaldoBanco).Add(decOrCero(a.AjustePartidas)).Sub(libros)
	a.DiferenciaSinExplicar = dif.StringFixed(2)
	a.Cuadra = dif.IsZero()
}

// RegistrarPartida agrega una partida en tránsito que explica parte de la diferencia.
func (s *Service) RegistrarPartida(ctx context.Context, empresaID string, in PartidaInput, usuarioID string) (string, error) {
	in.Descripcion = strings.TrimSpace(in.Descripcion)
	if in.CuentaID == "" || in.Descripcion == "" || !tipoPartidaValido(in.Tipo) || !periodoValidoBancos(in.Anio, in.Mes) {
		return "", ErrPartidaInvalida
	}
	monto, err := decimal.NewFromString(in.Monto)
	if err != nil || monto.LessThanOrEqual(decimal.Zero) {
		// El monto siempre es positivo: la dirección la da el signo, no el número.
		return "", ErrPartidaInvalida
	}
	in.Monto = monto.StringFixed(2)

	signo := signoPorTipo(in.Tipo)
	if signo == 0 {
		// Solo «OTRA» pide el signo; los tipos conocidos no lo dejan a criterio de nadie.
		if in.Signo != 1 && in.Signo != -1 {
			return "", ErrSignoRequerido
		}
		signo = in.Signo
	}

	id, err := s.repo.CrearPartida(ctx, empresaID, in, signo, usuarioID)
	if err != nil {
		return "", err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "partida_conciliacion", EntidadID: &id,
		Accion: "REGISTRAR_PARTIDA", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"cuenta_id": in.CuentaID, "periodo": periodoTexto(in.Anio, in.Mes),
			"tipo": in.Tipo, "monto": in.Monto, "signo": signo, "descripcion": in.Descripcion,
		},
	})
	return id, nil
}

// AnularPartida la deja sin efecto (nunca se borra: tabla financiera).
func (s *Service) AnularPartida(ctx context.Context, empresaID, partidaID, usuarioID string) error {
	if partidaID == "" {
		return ErrPartidaNoEncontrada
	}
	if err := s.repo.AnularPartida(ctx, empresaID, partidaID, usuarioID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "partida_conciliacion", EntidadID: &partidaID,
		Accion: "ANULAR_PARTIDA", UsuarioID: &usuarioID,
	})
	return nil
}

// FirmarActa cierra la conciliación de una cuenta. Solo firma si la diferencia sin explicar es
// CERO: firmar un acta que no cuadra sería declarar conforme algo que no lo está.
func (s *Service) FirmarActa(ctx context.Context, empresaID, cuentaID string, anio, mes int, usuarioID string) error {
	if cuentaID == "" {
		return ErrCuentaNoEncontrada
	}
	c, err := s.Conciliacion(ctx, empresaID, anio, mes)
	if err != nil {
		return err
	}
	if c.Cerrado {
		return ErrPeriodoYaCerrado
	}
	for _, a := range c.Actas {
		if a.CuentaID != cuentaID {
			continue
		}
		if a.Impedimento != "" {
			return ErrSaldoDelMesFaltante
		}
		if !a.Cuadra {
			return ErrActaNoCuadra
		}
		if err := s.repo.FirmarActa(ctx, empresaID, cuentaID, anio, mes,
			a.SaldoBanco, a.SaldoLibros, a.AjustePartidas, usuarioID); err != nil {
			return err
		}
		s.audit.Registrar(ctx, shared.Evento{
			EmpresaID: &empresaID, Entidad: "acta_conciliacion", EntidadID: &cuentaID,
			Accion: "FIRMAR_ACTA", UsuarioID: &usuarioID,
			ValorNuevo: map[string]any{
				"periodo": c.Periodo, "alias": a.Alias,
				"saldo_banco": a.SaldoBanco, "saldo_libros": a.SaldoLibros,
				"ajuste_partidas": a.AjustePartidas,
			},
		})
		return nil
	}
	return ErrCuentaNoEncontrada
}

// RevisarSaldos congela (o descongela) los saldos capturados de una fecha. Congelar es el acto
// de Dirección Financiera que vuelve el saldo del día un hecho no editable; descongelar es
// excepcional y queda auditado con su motivo.
func (s *Service) RevisarSaldos(ctx context.Context, empresaID, fecha string, congelar bool, motivo, usuarioID string) (int, error) {
	if fecha == "" {
		fecha = HoyCR()
	}
	if !fechaValida(fecha) {
		return 0, ErrFechaInvalida
	}
	n, err := s.repo.RevisarSaldos(ctx, empresaID, fecha, usuarioID, congelar)
	if err != nil {
		return 0, err
	}
	accion := "CONGELAR_SALDOS"
	if !congelar {
		accion = "DESCONGELAR_SALDOS"
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "saldo_cuenta_diario", Accion: accion, UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{"fecha": fecha, "cuentas": n, "motivo": strings.TrimSpace(motivo)},
	})
	return n, nil
}

// periodoValidoBancos acota el período a algo operable (mismo criterio que el resto del módulo).
func periodoValidoBancos(anio, mes int) bool {
	return anio >= 2024 && anio <= 2100 && mes >= 1 && mes <= 12
}

// periodoTexto formatea el período como AAAA-MM.
func periodoTexto(anio, mes int) string {
	return fmt.Sprintf("%04d-%02d", anio, mes)
}
