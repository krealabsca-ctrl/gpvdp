package cxc

import (
	"context"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// Valores por omisión de los parámetros de la cola. Si la empresa no los tiene definidos
// (o los tiene con basura), la cola sigue funcionando con estos.
const (
	diasSinGestionarDefault  = 30
	tolPromesaDefault        = 3
	diasAlertaTarjetaDefault = 60
)

// ParametrosCola son los tres números que gobiernan la cola de cobro.
type ParametrosCola struct {
	DiasSinGestionar  int
	TolPromesa        int
	DiasAlertaTarjeta int
	// MesesParaSuspender: la regla del negocio — 18 MESES de mora, o su equivalencia en
	// cuotas según la modalidad del contrato.
	MesesParaSuspender int
}

// parametrosCola los lee de la configuración de la empresa. No falla si faltan: un
// parámetro ausente no puede dejar sin cola al operador.
func (s *Service) parametrosCola(ctx context.Context, empresaID string) ParametrosCola {
	out := ParametrosCola{
		DiasSinGestionar:   diasSinGestionarDefault,
		TolPromesa:         tolPromesaDefault,
		DiasAlertaTarjeta:  diasAlertaTarjetaDefault,
		MesesParaSuspender: mesesParaSuspenderDefault,
	}
	p, err := s.repo.Parametros(ctx, empresaID)
	if err != nil {
		return out
	}
	if v, err := strconv.Atoi(p["DIAS_SIN_GESTIONAR"]); err == nil && v > 0 && v <= 3650 {
		out.DiasSinGestionar = v
	}
	// La tolerancia sí puede ser 0 (promesa para hoy es para hoy).
	if v, err := strconv.Atoi(p["PROMESA_TOLERANCIA_DIAS"]); err == nil && v >= 0 && v <= 60 {
		out.TolPromesa = v
	}
	if v, err := strconv.Atoi(p["DIAS_ALERTA_TARJETA"]); err == nil && v >= 0 && v <= 365 {
		out.DiasAlertaTarjeta = v
	}
	if v, err := strconv.Atoi(p["MESES_PARA_SUSPENDER"]); err == nil && v > 0 && v <= 600 {
		out.MesesParaSuspender = v
	}
	return out
}

// ColaDeCobro es la pantalla de trabajo del gestor: los contratos que deben algo, ordenados
// por VALOR ESPERADO (saldo × probabilidad del tramo × factor de la forma de pago).
//
// El alcance por sede lo pone el servicio, nunca el cliente: sin cxc.ver_todas_sedes el
// operador solo trabaja la cartera de su(s) sede(s), aunque arme la URL a mano.
func (s *Service) ColaDeCobro(ctx context.Context, empresaID, rol, usuarioID string, f FiltrosCola) (ListaCola, error) {
	sedes, err := s.sedesVisibles(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return ListaCola{}, err
	}
	f.SedeIDs = sedes
	// Pedir una sede que no es suya no devuelve una lista vacía: devuelve 403. Vaciar la
	// lista escondería que la pantalla está pidiendo algo que no le corresponde.
	if sedes != nil && f.SedeID != "" && !contiene(sedes, f.SedeID) {
		return ListaCola{}, ErrSinPermisoSedes
	}
	p := s.parametrosCola(ctx, empresaID)
	return s.repo.ColaDeCobro(ctx, empresaID, f, p)
}

// CatalogosGestion son los canales y resultados para el formulario de gestión.
func (s *Service) CatalogosGestion(ctx context.Context, empresaID string) (CatalogosGestion, error) {
	return s.repo.CatalogosGestion(ctx, empresaID)
}

// RegistrarGestion anota lo que pasó con un contrato: quién lo trabajó, por qué canal, con
// qué resultado y qué prometió el cliente.
//
// Sin esto la cola no sirve: el operador volvería a llamar tres veces al mismo cliente y
// nunca a otros mil.
func (s *Service) RegistrarGestion(ctx context.Context, empresaID, rol, usuarioID string, in GestionInput) (GestionRegistrada, error) {
	if err := validarPromesa(in); err != nil {
		return GestionRegistrada{}, err
	}
	sedes, err := s.sedesVisibles(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return GestionRegistrada{}, err
	}
	if sedes != nil {
		c, err := s.repo.ContratoPorNumero(ctx, empresaID, in.Contrato)
		if err != nil {
			return GestionRegistrada{}, err
		}
		if !contiene(sedes, c.SedeID) {
			return GestionRegistrada{}, ErrSinPermisoSedes
		}
	}
	res, err := s.repo.RegistrarGestion(ctx, empresaID, in, usuarioID)
	if err != nil {
		return GestionRegistrada{}, err
	}
	s.auditar(ctx, empresaID, "REGISTRAR_GESTION_CXC", usuarioID, map[string]any{
		"gestion": res.ID, "contrato": in.Contrato, "resultado": res.Resultado,
		"saldo_al_gestionar": res.SaldoAlGestionar, "tramo": res.Tramo,
		"promesa": in.PromesaFecha,
	})
	return res, nil
}

// GestionesDeContrato es el historial: qué se le dijo a este cliente y qué contestó.
func (s *Service) GestionesDeContrato(ctx context.Context, empresaID, numero string) ([]GestionFila, error) {
	c, err := s.repo.ContratoPorNumero(ctx, empresaID, numero)
	if err != nil {
		return nil, err
	}
	p := s.parametrosCola(ctx, empresaID)
	return s.repo.GestionesDeContrato(ctx, empresaID, c.ID, p.TolPromesa)
}

// validarPromesa revisa la promesa antes de escribirla.
//
// Una promesa con fecha PASADA no es una promesa: nadie puede comprometerse a pagar ayer.
// Además rompería la medición, porque el cumplimiento se deriva de los pagos que entran
// entre el día de la promesa y la fecha prometida.
func validarPromesa(in GestionInput) error {
	if in.PromesaFecha == "" {
		if in.PromesaMonto != "" {
			return ErrPromesaRequerida
		}
		return nil
	}
	f, err := time.Parse("2006-01-02", in.PromesaFecha)
	if err != nil {
		return ErrPromesaFechaInvalida
	}
	if f.Before(hoyCR()) {
		return ErrPromesaEnElPasado
	}
	if in.PromesaMonto != "" {
		m, err := decimal.NewFromString(in.PromesaMonto)
		if err != nil || m.Sign() <= 0 {
			return ErrPromesaMontoInvalido
		}
	}
	return nil
}

func contiene(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
