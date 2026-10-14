package cxc

import (
	"context"
	"errors"
	"fmt"
)

// ErrConfirmacionNoCoincide: se pidió rehacer sin confirmar el número exacto del plan.
var ErrConfirmacionNoCoincide = errors.New("cxc: confirmá la cantidad exacta de cargos que dice el plan")

// RegeneracionPlan es lo que se va a hacer, ANTES de hacerlo.
type RegeneracionPlan struct {
	// Total: cuántos cargos hay hoy.
	Total int `json:"total"`
	// Borrables: cuántos se pueden rehacer (sin pagos, sin notas, sin factura).
	Borrables int `json:"borrables"`
	// SeConservan: los que NO se tocan porque algo cuelga de ellos. Van con nombre propio: es la
	// cifra que dice si la corrección va a quedar a medias.
	SeConservan   int    `json:"se_conservan"`
	MontoBorrable string `json:"monto_borrable"`
}

// RegeneracionHecha es el parte de lo que quedó.
type RegeneracionHecha struct {
	Borrados int        `json:"borrados"`
	Creados  int        `json:"creados"`
	Plan     PlanCargos `json:"plan"`
}

// PlanDeRegeneracion dice qué pasaría si se rehicieran los cargos. No toca nada.
func (s *Service) PlanDeRegeneracion(ctx context.Context, empresaID string) (RegeneracionPlan, error) {
	return s.repo.PlanDeRegeneracion(ctx, empresaID)
}

// RegenerarCargos borra los cargos generados que no tienen nada colgando y los vuelve a crear
// con la cartera que hay HOY.
//
// PARA QUÉ. Los cargos se calculan de la cartera; cuando la cartera se corrige, los cargos viejos
// quedan mal. El caso que lo motivó: al empezar a guardar la fecha de próximo pago, 18.450
// contratos pasaron a deber menos, ₡1.042.683.725 en total. Sin esta función había que borrarlos
// a mano en producción, que es la clase de operación que sale mal una sola vez.
//
// EXIGE CONFIRMAR EL NÚMERO EXACTO del plan, igual que la generación cuando el rango es grande.
// No es un trámite: entre mirar el plan y apretar el botón la cartera pudo cambiar —alguien
// registró un cobro— y entonces el número ya no es el mismo. Que no coincida es la señal de que
// hay que volver a mirar, no un capricho.
//
// NO ES UNA TRANSACCIÓN ÚNICA a propósito. Borrar 202.336 cargos y crear otros tantos dentro de
// una sola transacción mantiene bloqueada la tabla toda la corrida. El borrado es seguro por sí
// mismo —solo saca lo que no tiene pagos— y la generación es idempotente, así que si algo se
// corta a la mitad se vuelve a correr y termina. Lo que no puede pasar es perder un pago, y de
// eso se encarga el guardarraíl del DELETE.
func (s *Service) RegenerarCargos(ctx context.Context, empresaID, rol, usuarioID, desde, hasta string, confirmarBorrados int) (RegeneracionHecha, error) {
	plan, err := s.repo.PlanDeRegeneracion(ctx, empresaID)
	if err != nil {
		return RegeneracionHecha{}, err
	}
	if confirmarBorrados != plan.Borrables {
		return RegeneracionHecha{}, fmt.Errorf("%w: el plan son %d cargos y se confirmaron %d",
			ErrConfirmacionNoCoincide, plan.Borrables, confirmarBorrados)
	}

	borrados, err := s.repo.BorrarCargosRegenerables(ctx, empresaID)
	if err != nil {
		return RegeneracionHecha{}, err
	}
	// La auditoría del borrado va ANTES de generar: si la generación falla, tiene que quedar
	// escrito que los cargos se borraron igual, o nadie entendería por qué la cartera bajó.
	s.auditar(ctx, empresaID, "REGENERAR_CARGOS_CXC_BORRADO", usuarioID, map[string]any{
		"borrados": borrados, "se_conservan": plan.SeConservan, "monto": plan.MontoBorrable,
	})

	planCargos, creados, err := s.GenerarCargos(ctx, empresaID, rol, usuarioID, desde, hasta, 0)
	if err != nil {
		// El tope de la generación pide confirmar su propio total. Se reintenta una vez con el
		// número que ella misma calculó: quien confirmó el borrado ya aceptó rehacer la cartera,
		// y mandarlo a confirmar dos veces seguidas por el mismo gesto no agrega control.
		if errors.Is(err, ErrRangoDemasiadoAmplio) && planCargos.Cargos > 0 {
			planCargos, creados, err = s.GenerarCargos(ctx, empresaID, rol, usuarioID, desde, hasta, planCargos.Cargos)
		}
		if err != nil {
			return RegeneracionHecha{Borrados: borrados}, err
		}
	}
	s.auditar(ctx, empresaID, "REGENERAR_CARGOS_CXC", usuarioID, map[string]any{
		"borrados": borrados, "creados": creados, "desde": desde, "hasta": hasta,
	})
	return RegeneracionHecha{Borrados: borrados, Creados: creados, Plan: planCargos}, nil
}
