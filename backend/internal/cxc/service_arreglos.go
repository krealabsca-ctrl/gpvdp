package cxc

// La regla de los plazos, tal como la dio el negocio:
//
//	«lo ideal que se maneje en 1-3-6-9 máximo pero puede haber excepciones, los plazos largos
//	 los aprueba el supervisor de piso, y en el caso de incumplir aplica la regla de los 18
//	 meses operativos, pero pasa a una cartera morosa.»
//
// Traducido a software:
//
//   - 1, 3, 6 o 9 cuotas: un gestor con cxc.gestionar lo pacta solo. Es el camino normal y no
//     necesita autorización de nadie.
//   - Cualquier otro plazo (incluidos 2, 4, 5 y todo lo mayor que 9): es EXCEPCIÓN. Necesita
//     cxc.arreglos —el permiso del supervisor de piso— y queda marcado como excepción con quién
//     lo autorizó y por qué. Se cuenta en el resumen: con autorización sin tope, el acumulado
//     visible es el control.
//   - Los plazos estándar son un PARÁMETRO, no una constante: la regla la puso el negocio.
//
// Lo que NO se automatizó: quebrar el arreglo. Que el cliente esté en mora del plan lo calcula
// el sistema (es un hecho, derivado de los cobros); declarar el arreglo roto es una decisión de
// una persona con motivo, igual que la suspensión. Un arreglo quebrado manda el contrato a
// cartera morosa, y ahí la regla de los 18 meses sigue corriendo sobre los cargos originales
// —que el arreglo nunca tocó— así que no hay nada que recalcular.

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	arregloPlazoMaximoDefault = 60
	// diasContactoPreventivoDefault: una semana antes del vencimiento. Es el valor de arranque
	// y es editable en Parámetros.
	diasContactoPreventivoDefault = 7
)

// plazosEstandar lee ARREGLO_PLAZOS_ESTANDAR. Ante cualquier basura en el parámetro devuelve
// la lista que dio el negocio: un parámetro mal escrito no puede dejar sin trabajar al gestor.
func (s *Service) plazosEstandar(ctx context.Context, empresaID string) []int {
	def := []int{1, 3, 6, 9}
	p, err := s.repo.Parametros(ctx, empresaID)
	if err != nil {
		return def
	}
	out := []int{}
	for _, tok := range strings.Split(p["ARREGLO_PLAZOS_ESTANDAR"], ",") {
		if v, err := strconv.Atoi(strings.TrimSpace(tok)); err == nil && v >= 1 && v <= 600 {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func (s *Service) plazoMaximo(ctx context.Context, empresaID string) int {
	p, err := s.repo.Parametros(ctx, empresaID)
	if err != nil {
		return arregloPlazoMaximoDefault
	}
	if v, err := strconv.Atoi(strings.TrimSpace(p["ARREGLO_PLAZO_MAXIMO"])); err == nil && v >= 1 && v <= 600 {
		return v
	}
	return arregloPlazoMaximoDefault
}

// PlazosDeArreglo es lo que la pantalla necesita para armar el formulario sin adivinar.
type PlazosDeArreglo struct {
	Estandar []int `json:"estandar"`
	Maximo   int   `json:"maximo"`
	// PuedeExcepcion: si el usuario que pregunta tiene el permiso del supervisor. Sin esto la
	// pantalla ofrecería plazos que el servidor va a rechazar.
	PuedeExcepcion bool `json:"puede_excepcion"`
}

func (s *Service) PlazosDeArreglo(ctx context.Context, empresaID, rol string) PlazosDeArreglo {
	puede, _ := s.puedeAutorizarArreglos(ctx, empresaID, rol)
	return PlazosDeArreglo{
		Estandar:       s.plazosEstandar(ctx, empresaID),
		Maximo:         s.plazoMaximo(ctx, empresaID),
		PuedeExcepcion: puede,
	}
}

// puedeAutorizarArreglos resuelve cxc.arreglos —el permiso del supervisor de piso—. Sin
// checker (tests, arranque sin RBAC) devuelve true: la regla la impone el RBAC real, no un
// null que dejaría a todo el mundo trabado.
func (s *Service) puedeAutorizarArreglos(ctx context.Context, empresaID, rol string) (bool, error) {
	if s.perms == nil {
		return true, nil
	}
	return s.perms.Tiene(ctx, empresaID, rol, permisoArreglos)
}

// PactarArreglo valida la regla de plazos y escribe el arreglo.
//
// El permiso se resuelve ACÁ y no en el handler: la regla «un plazo fuera de los estándar lo
// autoriza el supervisor de piso» es de negocio, y tiene que vivir donde se prueba.
func (s *Service) PactarArreglo(ctx context.Context, empresaID, rol string, in ArregloInput, usuarioID string) (Arreglo, error) {
	in.Contrato = strings.TrimSpace(in.Contrato)
	if in.Plazo < 1 {
		return Arreglo{}, ErrPlazoInvalido
	}
	if tope := s.plazoMaximo(ctx, empresaID); in.Plazo > tope {
		return Arreglo{}, ErrPlazoExcedeTope
	}
	if in.Monto.Sign() < 0 {
		return Arreglo{}, ErrMontoArregloInvalido
	}

	esExcepcion := true
	for _, p := range s.plazosEstandar(ctx, empresaID) {
		if p == in.Plazo {
			esExcepcion = false
			break
		}
	}
	if esExcepcion {
		puede, err := s.puedeAutorizarArreglos(ctx, empresaID, rol)
		if err != nil {
			return Arreglo{}, err
		}
		if !puede {
			return Arreglo{}, ErrPlazoNoAutorizado
		}
		// Una excepción sin explicación es una excepción que nadie puede auditar.
		if !motivoUtil(strings.TrimSpace(in.MotivoAutoriza)) {
			return Arreglo{}, ErrMotivoRequerido
		}
	}

	// Las fechas las normaliza el servicio para que el repositorio no tenga que adivinar. El
	// día base es el de Costa Rica: a las 7 p. m. de un 4 acá, en UTC ya es el 5.
	if in.PrimeraCuota == "" {
		in.PrimeraCuota = hoyCR().AddDate(0, 0, 15).Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", in.PrimeraCuota); err != nil {
		return Arreglo{}, ErrFechaArregloInvalida
	}
	if in.Prima.Sign() > 0 {
		if in.PrimaFecha == "" {
			in.PrimaFecha = hoyCR().Format("2006-01-02")
		} else if _, err := time.Parse("2006-01-02", in.PrimaFecha); err != nil {
			return Arreglo{}, ErrFechaArregloInvalida
		}
	}
	in.MotivoAutoriza = strings.TrimSpace(in.MotivoAutoriza)
	in.Observaciones = strings.TrimSpace(in.Observaciones)

	a, err := s.repo.PactarArreglo(ctx, empresaID, in, esExcepcion, usuarioID, s.topeMeses(ctx, empresaID))
	if err != nil {
		return Arreglo{}, err
	}
	s.auditar(ctx, empresaID, "PACTAR_ARREGLO_CXC", usuarioID, map[string]any{
		"arreglo": a.Consecutivo, "contrato": a.Contrato, "monto": a.MontoArreglo,
		"plazo": a.Plazo, "prima": a.Prima, "es_excepcion": a.EsExcepcion,
		"motivo_autorizacion": in.MotivoAutoriza,
	})
	return a, nil
}

// ListarArreglos aplica el alcance por sede antes de listar, como el resto del módulo: sin
// cxc.ver_todas_sedes el operador solo ve los arreglos de su(s) sede(s).
func (s *Service) ListarArreglos(ctx context.Context, empresaID, rol, usuarioID string, f FiltrosArreglos) (ListaArreglos, error) {
	sedes, err := s.sedesVisibles(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return ListaArreglos{}, err
	}
	f.SedeIDs = sedes
	return s.repo.ListarArreglos(ctx, empresaID, f, s.parametrosCola(ctx, empresaID).TolPromesa)
}

func (s *Service) Arreglo(ctx context.Context, empresaID, id string) (Arreglo, error) {
	return s.repo.ArregloPorID(ctx, empresaID, id, s.parametrosCola(ctx, empresaID).TolPromesa)
}

// QuebrarArreglo declara el incumplimiento: el contrato pasa a cartera morosa.
func (s *Service) QuebrarArreglo(ctx context.Context, empresaID, id, motivo, usuarioID string) (Arreglo, error) {
	return s.cerrarArreglo(ctx, empresaID, id, motivo, usuarioID, true)
}

// AnularArreglo borra el compromiso sin marcar incumplimiento: es para el arreglo que se pactó
// mal o que el cliente nunca firmó. NO manda a cartera morosa.
func (s *Service) AnularArreglo(ctx context.Context, empresaID, id, motivo, usuarioID string) (Arreglo, error) {
	return s.cerrarArreglo(ctx, empresaID, id, motivo, usuarioID, false)
}

func (s *Service) cerrarArreglo(ctx context.Context, empresaID, id, motivo, usuarioID string, quebrar bool) (Arreglo, error) {
	motivo = strings.TrimSpace(motivo)
	if !motivoUtil(motivo) {
		return Arreglo{}, ErrMotivoRequerido
	}
	a, err := s.repo.CerrarArreglo(ctx, empresaID, id, motivo, usuarioID, quebrar,
		s.parametrosCola(ctx, empresaID).TolPromesa)
	if err != nil {
		return Arreglo{}, err
	}
	accion := "ANULAR_ARREGLO_CXC"
	if quebrar {
		accion = "QUEBRAR_ARREGLO_CXC"
	}
	s.auditar(ctx, empresaID, accion, usuarioID, map[string]any{
		"arreglo": a.Consecutivo, "contrato": a.Contrato, "pagado": a.Pagado,
		"atraso": a.Atraso, "motivo": motivo,
	})
	return a, nil
}

// montoArregloDesdeTexto parsea el monto opcional del request. Vacío significa «todo lo
// vencido», que es el caso normal.
func montoArregloDesdeTexto(v string) (decimal.Decimal, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(v)
}
