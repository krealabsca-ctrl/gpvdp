package conta

// La lógica de la Fase 4: balance de comprobación, apertura y cierre de período.

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// Balance arma el balance de comprobación del rango, con sus totales y el cuadre.
func (s *Service) Balance(ctx context.Context, empresaID, desde, hasta string) (BalanceComprobacion, error) {
	if err := validarRango(desde, hasta); err != nil {
		return BalanceComprobacion{}, err
	}
	if desde == "" || hasta == "" {
		return BalanceComprobacion{}, ErrFechaAsientoRequerida
	}
	renglones, err := s.repo.Balance(ctx, empresaID, desde, hasta)
	if err != nil {
		return BalanceComprobacion{}, err
	}

	b := BalanceComprobacion{Desde: desde, Hasta: hasta, Renglones: renglones}
	var antD, antH, movD, movH, finD, finH decimal.Decimal
	for _, g := range renglones {
		for _, par := range []struct {
			valor string
			dest  *decimal.Decimal
		}{
			{g.SaldoAnteriorDebe, &antD}, {g.SaldoAnteriorHaber, &antH},
			{g.Debe, &movD}, {g.Haber, &movH},
			{g.SaldoFinalDebe, &finD}, {g.SaldoFinalHaber, &finH},
		} {
			v, err := decimal.NewFromString(par.valor)
			if err != nil {
				return BalanceComprobacion{}, fmt.Errorf("conta: monto de %s: %w", g.Codigo, err)
			}
			*par.dest = par.dest.Add(v)
		}
	}
	b.TotalAnteriorDebe, b.TotalAnteriorHaber = antD.StringFixed(2), antH.StringFixed(2)
	b.TotalDebe, b.TotalHaber = movD.StringFixed(2), movH.StringFixed(2)
	b.TotalFinalDebe, b.TotalFinalHaber = finD.StringFixed(2), finH.StringFixed(2)

	// LOS TRES PARES TIENEN QUE CUADRAR. Si uno solo no lo hace, el libro tiene un problema y
	// ningún estado financiero que salga de él sirve.
	b.Cuadra = antD.Equal(antH) && movD.Equal(movH) && finD.Equal(finH)
	if !b.Cuadra {
		// La diferencia más grande de las tres es la que hay que perseguir.
		dif := mayorDiferencia(antD.Sub(antH), movD.Sub(movH), finD.Sub(finH))
		b.Diferencia = dif.StringFixed(2)
	}

	hay, err := s.repo.HayApertura(ctx, empresaID)
	if err != nil {
		return BalanceComprobacion{}, err
	}
	b.SinApertura = !hay
	b.Aviso = avisoDeBalance(b)
	return b, nil
}

// mayorDiferencia devuelve la de mayor valor absoluto, conservando su signo.
func mayorDiferencia(ds ...decimal.Decimal) decimal.Decimal {
	mayor := decimal.Zero
	for _, d := range ds {
		if d.Abs().GreaterThan(mayor.Abs()) {
			mayor = d
		}
	}
	return mayor
}

func avisoDeBalance(b BalanceComprobacion) string {
	var partes []string
	if !b.Cuadra {
		partes = append(partes, fmt.Sprintf(
			"EL BALANCE NO CUADRA por %s. Mientras no cuadre, ningún estado financiero que salga de acá sirve.",
			montoLegible(b.Diferencia)))
	}
	if b.SinApertura {
		// Sin saldos iniciales el balance es correcto pero incompleto, y la diferencia entre las
		// dos cosas no se ve mirando los números.
		partes = append(partes,
			"No hay saldos iniciales cargados: esto muestra solo lo que se registró en el sistema, no el saldo real de cada cuenta.")
	}
	if len(partes) == 0 {
		return ""
	}
	return strings.Join(partes, " ")
}

// ── El período ──────────────────────────────────────────────────────────────────────────────

// Periodos lista los meses con su estado.
func (s *Service) Periodos(ctx context.Context, empresaID string) ([]PeriodoContable, error) {
	return s.repo.Periodos(ctx, empresaID)
}

// CerrarPeriodo congela el mes.
//
// NO SE CIERRA UN MES QUE NO CUADRA. Cerrarlo dejaría el error congelado y la única forma de
// arreglarlo sería reabrir, que es justo lo que el cierre existe para evitar.
func (s *Service) CerrarPeriodo(ctx context.Context, empresaID, periodo, usuarioID string) error {
	if !esPeriodo(periodo) {
		return ErrPeriodoInvalido
	}
	anio, mes := partesDePeriodo(periodo)
	debe, haber, n, err := s.repo.TotalesDelMes(ctx, empresaID, anio, mes)
	if err != nil {
		return err
	}
	if !debe.Equal(haber) {
		return ErrPeriodoNoCuadra
	}
	if err := s.repo.CerrarPeriodo(ctx, empresaID, anio, mes, debe, haber, n, usuarioID); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, periodo, "CERRAR_PERIODO", usuarioID, map[string]any{
		"periodo": periodo, "asientos": n,
		"debe": debe.StringFixed(2), "haber": haber.StringFixed(2),
	})
	return nil
}

// ReabrirPeriodo descongela el mes, con su motivo.
func (s *Service) ReabrirPeriodo(ctx context.Context, empresaID, periodo, motivo, usuarioID string) error {
	if !esPeriodo(periodo) {
		return ErrPeriodoInvalido
	}
	motivo, err := validarMotivoReapertura(motivo)
	if err != nil {
		return err
	}
	anio, mes := partesDePeriodo(periodo)
	if err := s.repo.ReabrirPeriodo(ctx, empresaID, anio, mes, motivo, usuarioID); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, periodo, "REABRIR_PERIODO", usuarioID, map[string]any{
		"periodo": periodo, "motivo": motivo,
	})
	return nil
}

// ── La apertura ─────────────────────────────────────────────────────────────────────────────

// PrevisualizarApertura dice qué saldos iniciales traería el Excel, sin escribir nada.
func (s *Service) PrevisualizarApertura(ctx context.Context, empresaID, periodo string, archivo []byte) (PrevisualizacionApertura, error) {
	if !esPeriodo(periodo) {
		return PrevisualizacionApertura{}, ErrPeriodoInvalido
	}
	filas, err := LeerAperturaDesdeExcel(archivo)
	if err != nil {
		return PrevisualizacionApertura{}, err
	}

	cuentas, err := s.repo.Cuentas(ctx, empresaID, true)
	if err != nil {
		return PrevisualizacionApertura{}, err
	}
	porCodigo := map[string]Cuenta{}
	for _, c := range cuentas {
		porCodigo[strings.TrimSpace(c.Codigo)] = c
	}

	prev := PrevisualizacionApertura{Fecha: finDeMesDe(periodo), Filas: filas}
	totD, totH := decimal.Zero, decimal.Zero
	for i := range prev.Filas {
		f := &prev.Filas[i]
		c, hay := porCodigo[f.Codigo]
		switch {
		case !hay:
			f.Problema = "no existe una cuenta con ese código en el plan"
		case !c.AceptaMovimiento:
			// La base lo rechazaría igual; decirlo acá evita que el archivo entero falle al final
			// por una fila.
			f.Problema = "esa cuenta agrupa a otras: no admite saldo"
		default:
			f.Cuenta = c.Nombre
		}
		if f.Problema != "" {
			prev.ConProblema++
			continue
		}
		d, _ := decimal.NewFromString(f.Debe)
		h, _ := decimal.NewFromString(f.Haber)
		totD, totH = totD.Add(d), totH.Add(h)
		prev.Cuentas++
	}
	prev.TotalDebe, prev.TotalHaber = totD.StringFixed(2), totH.StringFixed(2)
	prev.Cuadra = totD.Equal(totH) && prev.Cuentas > 0
	if !totD.Equal(totH) {
		prev.Diferencia = totD.Sub(totH).StringFixed(2)
	}

	hay, err := s.repo.HayApertura(ctx, empresaID)
	if err != nil {
		return PrevisualizacionApertura{}, err
	}
	prev.YaExiste = hay
	prev.Aviso = avisoDeApertura(prev)
	return prev, nil
}

func avisoDeApertura(p PrevisualizacionApertura) string {
	var partes []string
	if p.YaExiste {
		partes = append(partes,
			"Ya hay un asiento de apertura cargado. Para reemplazarlo hay que anular el anterior desde el libro diario; si no, quedarían dos puntos de partida.")
	}
	if p.Cuentas == 0 {
		partes = append(partes, "El archivo no trae ningún saldo que se pueda cargar.")
	} else if !p.Cuadra {
		partes = append(partes, fmt.Sprintf(
			"Los saldos no cuadran: sobran %s en el %s. Un asiento de apertura descuadrado no se puede escribir.",
			montoLegible(strings.TrimPrefix(p.Diferencia, "-")),
			ladoDeLaDiferencia(p.Diferencia)))
	}
	if p.ConProblema > 0 {
		partes = append(partes, fmt.Sprintf("%d fila(s) quedan fuera por un problema propio.", p.ConProblema))
	}
	return strings.Join(partes, " ")
}

// ladoDeLaDiferencia dice de qué lado sobra, que es lo que permite corregir sin volver a sumar.
func ladoDeLaDiferencia(dif string) string {
	if strings.HasPrefix(dif, "-") {
		return "haber"
	}
	return "debe"
}

// CargarApertura escribe el asiento de saldos iniciales.
func (s *Service) CargarApertura(ctx context.Context, empresaID, periodo string, archivo []byte, usuarioID string) (Asiento, error) {
	prev, err := s.PrevisualizarApertura(ctx, empresaID, periodo, archivo)
	if err != nil {
		return Asiento{}, err
	}
	if prev.YaExiste {
		return Asiento{}, ErrAperturaYaExiste
	}
	if prev.Cuentas == 0 {
		return Asiento{}, ErrAperturaVacia
	}
	if !prev.Cuadra {
		return Asiento{}, ErrAperturaDescuadrada
	}

	cuentas, err := s.repo.Cuentas(ctx, empresaID, true)
	if err != nil {
		return Asiento{}, err
	}
	porCodigo := map[string]Cuenta{}
	for _, c := range cuentas {
		porCodigo[strings.TrimSpace(c.Codigo)] = c
	}

	lineas := make([]LineaValidada, 0, prev.Cuentas)
	orden := 0
	for _, f := range prev.Filas {
		if f.Problema != "" {
			continue
		}
		d, _ := decimal.NewFromString(f.Debe)
		h, _ := decimal.NewFromString(f.Haber)
		lineas = append(lineas, LineaValidada{
			CuentaID: porCodigo[f.Codigo].ID,
			Debe:     d, Haber: h,
			Detalle: "saldo inicial", Orden: orden,
		})
		orden++
	}

	// La fecha es el ÚLTIMO DÍA del período: los saldos iniciales son los del cierre anterior,
	// no los de un día cualquiera del mes.
	a := Asiento{
		EmpresaID:  empresaID,
		Fecha:      prev.Fecha,
		Glosa:      "Saldos iniciales al " + prev.Fecha,
		OrigenTipo: OrigenApertura,
		Estado:     EstadoAsentado,
	}
	// El asiento de apertura necesita un origen: el CHECK de la 0111 exige que todo lo que no es
	// MANUAL apunte a un documento. Acá el «documento» es el período que se abre.
	origen := empresaID
	a.OrigenID = &origen

	creado, err := s.repo.CrearAsiento(ctx, a, lineas, usuarioID)
	if err != nil {
		return Asiento{}, err
	}
	s.auditar(ctx, empresaID, creado.ID, "CARGAR_APERTURA", usuarioID, map[string]any{
		"periodo": periodo, "cuentas": len(lineas), "total": prev.TotalDebe,
	})
	completo, err := s.repo.Asiento(ctx, empresaID, creado.ID)
	if err != nil {
		return creado, nil
	}
	return completo, nil
}
