package nomina

// El autoservicio de vacaciones, del lado del negocio.
//
// LA REGLA QUE GOBIERNA TODO ESTE ARCHIVO: el alcance lo pone el SERVIDOR a partir del usuario
// autenticado, nunca la petición. El cliente dice «quiero pedir vacaciones», no «quiero pedir
// vacaciones PARA el empleado X»; quién es X lo resuelve el servidor. Si el empleado viniera en el
// cuerpo, bastaría cambiarlo para pedirle —o aprobarle— días a cualquier otra persona.
//
// Y un alcance vacío CIERRA. Sin ficha ligada no hay «mis vacaciones»; sin jefatura cargada no hay
// equipo. Nunca al revés.

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// MisVacaciones devuelve el saldo y las solicitudes de quien pregunta. Nada de nadie más.
func (s *Service) MisVacaciones(ctx context.Context, empresaID, usuarioID string) (MiVacaciones, error) {
	empleadoID, err := s.repo.EmpleadoDeUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return MiVacaciones{}, err
	}
	diasPorMes, err := s.diasVacacionesDelAnio(ctx, empresaID)
	if err != nil {
		return MiVacaciones{}, err
	}
	saldo, err := s.repo.SaldoDeEmpleado(ctx, empresaID, empleadoID, diasPorMes)
	if err != nil {
		return MiVacaciones{}, err
	}
	sols, err := s.repo.MisVacaciones(ctx, empresaID, empleadoID)
	if err != nil {
		return MiVacaciones{}, err
	}
	return MiVacaciones{Saldo: saldo, Solicitudes: sols}, nil
}

// PedirVacaciones crea la solicitud PARA QUIEN LA PIDE.
//
// El input no trae empleado y no es un olvido: el empleado sale del usuario autenticado. La
// validación de saldo es la misma que usa RRHH —está en el propio INSERT—, así que pedir días que
// no se tienen falla acá y no en el escritorio de la jefatura.
func (s *Service) PedirVacaciones(ctx context.Context, empresaID, usuarioID string, in VacacionInput) (Vacacion, error) {
	empleadoID, err := s.repo.EmpleadoDeUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return Vacacion{}, err
	}
	in.EmpleadoID = empleadoID
	diasPorMes, err := s.diasVacacionesDelAnio(ctx, empresaID)
	if err != nil {
		return Vacacion{}, err
	}
	return s.repo.SolicitarVacacion(ctx, empresaID, in, diasPorMes, usuarioID)
}

// CancelarMiSolicitud retira una solicitud propia que todavía no se aprobó.
//
// Solo SOLICITADA: una vez que la jefatura dio el visto, retirarla es una decisión que ya no es
// únicamente del colaborador —el equipo se organizó contando con esa ausencia—, y pasa por quien
// la aprobó o por RRHH.
func (s *Service) CancelarMiSolicitud(ctx context.Context, empresaID, usuarioID, vacacionID string) error {
	empleadoID, err := s.repo.EmpleadoDeUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return err
	}
	dueno, estado, err := s.repo.VacacionDeEmpleado(ctx, empresaID, vacacionID)
	if err != nil {
		return err
	}
	if dueno != empleadoID {
		return ErrNoEsTuVacacion
	}
	if estado != VacacionSolicitada {
		return ErrTransicionVacacionInvalida
	}
	return s.repo.AnularVacacion(ctx, empresaID, vacacionID)
}

// VacacionesDelEquipo es lo que ve la jefatura: lo que espera su decisión, el calendario y los
// días disponibles de su gente. Ni un dato de dinero.
func (s *Service) VacacionesDelEquipo(ctx context.Context, empresaID, usuarioID string) (VacacionesEquipo, error) {
	deptos, err := s.repo.DepartamentosDeJefe(ctx, empresaID, usuarioID)
	if err != nil {
		return VacacionesEquipo{}, err
	}
	if len(deptos) == 0 {
		return VacacionesEquipo{}, ErrSinDepartamentosACargo
	}
	diasPorMes, err := s.diasVacacionesDelAnio(ctx, empresaID)
	if err != nil {
		return VacacionesEquipo{}, err
	}
	pendientes, err := s.repo.VacacionesDeDepartamentos(ctx, empresaID, deptos, true)
	if err != nil {
		return VacacionesEquipo{}, err
	}
	calendario, err := s.repo.VacacionesDeDepartamentos(ctx, empresaID, deptos, false)
	if err != nil {
		return VacacionesEquipo{}, err
	}
	saldos, err := s.repo.SaldosDeDepartamentos(ctx, empresaID, deptos, diasPorMes)
	if err != nil {
		return VacacionesEquipo{}, err
	}
	return VacacionesEquipo{Pendientes: pendientes, Calendario: calendario, Saldos: saldos}, nil
}

// ResolverSolicitud es la decisión de la jefatura: aprobar o rechazar.
//
// El recorte por departamento viaja hasta el UPDATE y no se comprueba antes: el id de la vacación
// lo manda el cliente, y una comprobación previa seguida de una escritura sin filtro deja la
// ventana para que entre medio cambie el departamento de la persona. El filtro va en la misma
// sentencia que escribe.
func (s *Service) ResolverSolicitud(ctx context.Context, empresaID, usuarioID, vacacionID string, aprobar bool, motivo string) error {
	deptos, err := s.repo.DepartamentosDeJefe(ctx, empresaID, usuarioID)
	if err != nil {
		return err
	}
	if len(deptos) == 0 {
		return ErrSinDepartamentosACargo
	}
	hacia := VacacionAprobada
	if !aprobar {
		if strings.TrimSpace(motivo) == "" {
			return ErrMotivoRechazoRequerido
		}
		hacia = VacacionRechazada
	}
	return s.repo.CambiarEstadoVacacion(ctx, empresaID, vacacionID,
		VacacionSolicitada, hacia, usuarioID, motivo, deptos)
}

// ConfirmarVacacion es el último paso, el de RRHH: los días se disfrutaron de verdad.
//
// Solo desde APROBADA, y sin recorte por departamento (`nil`): quien llega acá tiene
// `rrhh.ausencias`, que es el permiso de toda la planilla. Confirmar es lo que convierte la
// reserva en disfrute, y por eso no lo hace quien pidió ni quien aprobó.
func (s *Service) ConfirmarVacacion(ctx context.Context, empresaID, usuarioID, vacacionID string) error {
	return s.repo.CambiarEstadoVacacion(ctx, empresaID, vacacionID,
		VacacionAprobada, VacacionConfirmada, usuarioID, "", nil)
}

// ---- Jefaturas y vínculo con el usuario (administración) ----

// Jefaturas devuelve quién tiene a cargo cada departamento.
func (s *Service) Jefaturas(ctx context.Context, empresaID string) ([]Jefatura, error) {
	return s.repo.ListarJefaturas(ctx, empresaID)
}

// AsignarJefatura nombra a alguien jefatura de un departamento.
func (s *Service) AsignarJefatura(ctx context.Context, empresaID, departamentoID, usuarioID, rol, porUsuarioID string) error {
	if strings.TrimSpace(rol) == "" {
		rol = JefaturaTitular
	}
	if !RolJefaturaValido(rol) {
		return ErrRolJefaturaInvalido
	}
	return s.repo.AsignarJefatura(ctx, empresaID, departamentoID, usuarioID, rol, porUsuarioID)
}

// QuitarJefatura saca a alguien de un departamento.
func (s *Service) QuitarJefatura(ctx context.Context, empresaID, departamentoID, usuarioID string) error {
	return s.repo.QuitarJefatura(ctx, empresaID, departamentoID, usuarioID)
}

// LigarUsuario conecta una ficha de empleado con el usuario que esa persona usa para entrar.
// Con el usuario vacío, la desliga.
func (s *Service) LigarUsuario(ctx context.Context, empresaID, empleadoID, usuarioID string) error {
	return s.repo.LigarUsuarioAEmpleado(ctx, empresaID, empleadoID, strings.TrimSpace(usuarioID))
}

// diasVacacionesDelAnio lee el parámetro del año en curso llamando al MISMO helper que usa la
// pantalla de RRHH. El saldo de vacaciones es un derecho laboral: no puede depender de por qué
// puerta se pregunte, así que acá no hay una segunda fórmula ni un default propio.
func (s *Service) diasVacacionesDelAnio(ctx context.Context, empresaID string) (decimal.Decimal, error) {
	return s.diasVacacionesPorMes(ctx, empresaID, time.Now().Year())
}
