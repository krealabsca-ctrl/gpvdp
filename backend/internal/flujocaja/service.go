package flujocaja

// La lógica del flujo a 13 semanas: juntar las cuatro fuentes, aplicar la política y —sobre
// todo— DECIR QUÉ NO ESTÁ VIENDO.
//
// Un flujo de caja que omite una fuente en silencio es peor que no tener flujo: se lee como la
// proyección completa cuando es la de tres cuartos, y alguien decide sobre eso.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

var (
	// ErrMonedaInvalida: solo colones y dólares. Las dos se proyectan por separado.
	ErrMonedaInvalida = errors.New("flujocaja: moneda inválida: solo CRC o USD")
	// ErrModoInvalido: la política solo admite los dos modos que la tabla declara.
	ErrModoInvalido = errors.New("flujocaja: el modo tiene que ser MONTO_FIJO o PCT_INGRESOS")
	// ErrTopeInvalido: un tope negativo pagaría atrasos al revés.
	ErrTopeInvalido = errors.New("flujocaja: el tope no puede ser negativo")
	// ErrPctInvalido: fuera de 0..100 no es un porcentaje.
	ErrPctInvalido = errors.New("flujocaja: el porcentaje tiene que estar entre 0 y 100")
	// ErrMinimoInvalido: el saldo mínimo operativo no puede ser negativo.
	ErrMinimoInvalido = errors.New("flujocaja: el saldo mínimo no puede ser negativo")
)

// Service arma el flujo.
type Service struct {
	repo  Repository
	audit *shared.Audit
	log   *zap.Logger
	// ahora se inyecta para que las pruebas puedan fijar el día. time.Now() dentro del servicio
	// haría que una prueba de la semana 1 pase o falle según el día en que se corra.
	ahora func() time.Time
}

// NewService construye el servicio.
func NewService(repo Repository, audit *shared.Audit, log *zap.Logger) *Service {
	return &Service{repo: repo, audit: audit, log: log, ahora: time.Now}
}

// SetReloj fija el reloj. Solo para pruebas.
func (s *Service) SetReloj(f func() time.Time) { s.ahora = f }

// monedasValidas son las dos que el sistema maneja.
func monedaValida(m string) bool { return m == "CRC" || m == "USD" }

// Flujo arma la proyección a 13 semanas de una moneda.
func (s *Service) Flujo(ctx context.Context, empresaID, moneda string) (FlujoSemanal, error) {
	if !monedaValida(moneda) {
		return FlujoSemanal{}, ErrMonedaInvalida
	}

	hoy := s.ahora().UTC()
	semanas := semanasDesde(hoy, SemanasHorizonte)
	desde, hasta := semanas[0].Desde, semanas[len(semanas)-1].Hasta

	partida, err := s.repo.SaldoDePartida(ctx, empresaID, moneda)
	if err != nil {
		return FlujoSemanal{}, err
	}
	pol, err := s.repo.Politica(ctx, empresaID, moneda)
	if err != nil {
		return FlujoSemanal{}, err
	}
	tasa, medida, err := s.repo.TasaDeCobro(ctx, empresaID)
	if err != nil {
		return FlujoSemanal{}, err
	}

	movs := make([]MovimientoEsperado, 0, 256)
	for _, traer := range []func() ([]MovimientoEsperado, error){
		func() ([]MovimientoEsperado, error) {
			return s.repo.PagosProgramados(ctx, empresaID, moneda, desde, hasta)
		},
		func() ([]MovimientoEsperado, error) {
			return s.repo.Planilla(ctx, empresaID, moneda, desde, hasta)
		},
		func() ([]MovimientoEsperado, error) {
			return s.repo.CargosPorCobrar(ctx, empresaID, moneda, desde, hasta)
		},
	} {
		parte, err := traer()
		if err != nil {
			return FlujoSemanal{}, err
		}
		movs = append(movs, parte...)
	}

	cuotas, activos, err := s.repo.CuotasProyectadas(ctx, empresaID, moneda, desde, hasta, tasa)
	if err != nil {
		return FlujoSemanal{}, err
	}
	movs = append(movs, cuotas...)

	represa, err := s.repo.CarteraAtrasada(ctx, empresaID, moneda)
	if err != nil {
		return FlujoSemanal{}, err
	}

	f := ConstruirFlujoSemanal(hoy, moneda, partida.Monto, partida.Fuente, partida.Fecha,
		movs, represa, pol)
	f.TasaCobro = tasa.StringFixed(4)
	f.TasaCobroMedida = medida
	f.ContratosActivos = activos
	f.CuentasConSaldo = partida.CuentasConSaldo
	f.CuentasSinSaldo = partida.CuentasSinSaldo
	f.Avisos = avisosDelFlujo(f)
	return f, nil
}

// avisosDelFlujo dice en voz alta lo que el número no muestra.
//
// EL ORDEN ES EL DE LA GRAVEDAD: primero lo que invalida la curva entera (de dónde salió el
// saldo), después lo que la sesga, y al final lo que falta por declarar. Quien lee tres avisos
// solo procesa el primero.
func avisosDelFlujo(f FlujoSemanal) []string {
	var av []string

	if f.FuenteSaldo == SaldoSinDatos {
		av = append(av, fmt.Sprintf(
			"No hay saldo de partida: ninguna de las %d cuenta(s) tiene un saldo capturado en Tesorería. "+
				"La curva arranca en cero, así que dice cuánto vas a MOVER, no cuánto vas a TENER. "+
				"Con capturar el saldo de hoy en Bancos › Tesorería, esta proyección pasa a ser real.",
			f.CuentasSinSaldo))
	} else if f.CuentasSinSaldo > 0 {
		av = append(av, fmt.Sprintf(
			"%d cuenta(s) no tienen saldo capturado y quedaron fuera del punto de partida: el saldo real "+
				"es más alto que el que muestra esta curva.", f.CuentasSinSaldo))
	}

	if !f.Politica.Definida {
		av = append(av, "No hay política de caja declarada, así que la cartera atrasada no se paga en esta proyección. "+
			"El flujo se ve mejor de lo que va a ser.")
	} else if f.SemanasParaVaciar < 0 {
		av = append(av, "El tope semanal es cero: la cartera atrasada no baja nunca a este ritmo.")
	} else if f.SemanasParaVaciar > SemanasHorizonte {
		av = append(av, fmt.Sprintf(
			"Al ritmo actual la cartera atrasada tarda %d semanas en salir, más que las %d que se proyectan acá.",
			f.SemanasParaVaciar, SemanasHorizonte))
	}

	if !f.TasaCobroMedida {
		av = append(av, "No hay histórico utilizable para medir cuánto se cobra de verdad —no hay cobros "+
			"aplicados contra los cargos vencidos—, así que las cuotas se proyectan al 100 %. "+
			"Es el supuesto más optimista posible.")
	}

	// LA LIMITACIÓN QUE MÁS IMPORTA, y va siempre: el flujo cubre tres circuitos, no todo el
	// banco. El gasto que no pasa por CxP ni por planilla —servicios, impuestos, compras de
	// contado— no está acá. Omitirlo en silencio haría que la curva se lea como completa.
	av = append(av, "Esta proyección cubre proveedores (CxP), planilla y cobros de cartera (CxC). "+
		"Lo que sale del banco sin pasar por esos tres circuitos no está incluido.")

	return av
}

// GuardarPolitica valida y guarda la decisión de tesorería.
func (s *Service) GuardarPolitica(ctx context.Context, empresaID, usuarioID string, p PoliticaCaja) error {
	if !monedaValida(p.Moneda) {
		return ErrMonedaInvalida
	}
	if p.Modo != TopeMontoFijo && p.Modo != TopePctIngresos {
		return ErrModoInvalido
	}
	for _, v := range []struct {
		valor string
		err   error
	}{
		{p.TopeSemanal, ErrTopeInvalido},
		{p.MinimoOperativo, ErrMinimoInvalido},
	} {
		d, err := decimal.NewFromString(strings.TrimSpace(v.valor))
		if err != nil || d.IsNegative() {
			return v.err
		}
	}
	pct, err := decimal.NewFromString(strings.TrimSpace(p.TopePct))
	if err != nil || pct.IsNegative() || pct.GreaterThan(decimal.NewFromInt(100)) {
		return ErrPctInvalido
	}

	if err := s.repo.GuardarPolitica(ctx, empresaID, usuarioID, p); err != nil {
		return err
	}
	if s.audit != nil {
		// Queda registrado porque la política DECIDE la curva: si mañana el flujo se ve distinto,
		// la primera pregunta es quién movió el tope y cuándo.
		moneda, usuario := p.Moneda, usuarioID
		s.audit.Registrar(ctx, shared.Evento{
			EmpresaID: &empresaID, UsuarioID: &usuario,
			Entidad: "politica_caja", EntidadID: &moneda, Accion: "GUARDAR_POLITICA_CAJA",
			ValorNuevo: map[string]any{
				"moneda": p.Moneda, "modo": p.Modo,
				"tope_semanal": p.TopeSemanal, "tope_pct": p.TopePct,
				"minimo_operativo": p.MinimoOperativo,
			},
		})
	}
	return nil
}

// Politica lee la vigente.
func (s *Service) Politica(ctx context.Context, empresaID, moneda string) (PoliticaCaja, error) {
	if !monedaValida(moneda) {
		return PoliticaCaja{}, ErrMonedaInvalida
	}
	return s.repo.Politica(ctx, empresaID, moneda)
}
