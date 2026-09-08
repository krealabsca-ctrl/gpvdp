package cxc

// Corrección de un contrato cargado con datos incompletos.
//
// ── POR QUÉ EXISTE ──────────────────────────────────────────────────────────
//
// El importador se niega a inventar una cuota: si el archivo del origen trae 0, aparta el contrato
// (`revision_pendiente`) en vez de fabricar deuda. Eso está bien —y hay que dejarlo así—, pero
// dejaba a esos contratos en un callejón sin salida: se veían con el filtro «Solo en revisión»,
// quedaban fuera de la cola de cobro, de las planillas y del preventivo, y **no había ninguna forma
// de arreglarlos desde el sistema**. Con la carga de Coopeprofa eso era el 20 % de la cartera
// (2.451 de 12.231) parada sin salida.
//
// El único camino era corregir el archivo y volver a importar, que sirve para una carga masiva pero
// no para el caso que aparece de a uno.
//
// ── QUÉ NO HACE ─────────────────────────────────────────────────────────────
//
// No toca cargos ya generados. Cambiar la cuota vigente afecta lo que se genere DE AQUÍ EN
// ADELANTE; los cargos que ya existen son deuda que alguien pudo haber cobrado, y reescribirlos
// hacia atrás cambiaría la historia. Es la misma regla que ya rige los arreglos de pago.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// Errores de la corrección.
//
// La cuota en cero se rechaza con el ErrCuotaInvalida que ya define el generador (periodo.go): es
// la misma condición, y tener dos errores para «la cuota no sirve» haría que el handler tradujera
// uno y se le escapara el otro.
var (
	// ErrDiaPagoInvalido: 1..31. El día se topa al fin de mes al generar, no acá.
	ErrDiaPagoInvalido = errors.New("cxc: el día de pago tiene que estar entre 1 y 31")
	// ErrModalidadNoEncontrada: la modalidad tiene que existir en el catálogo de la empresa.
	ErrModalidadNoEncontrada = errors.New("cxc: esa modalidad de pago no está en el catálogo")
	// ErrNadaQueCorregir evita un evento de auditoría que no cambió nada.
	ErrNadaQueCorregir = errors.New("cxc: no se indicó ningún cambio")
)

// CorreccionContrato es lo que se puede arreglar de un contrato apartado.
//
// Los campos son punteros para distinguir «no lo mandó» de «lo mandó en cero»: sin eso, un PATCH
// que solo cambia la modalidad pondría la cuota en 0 y volvería a apartar el contrato.
type CorreccionContrato struct {
	Cuota       *string `json:"cuota"`
	DiaPago     *int    `json:"dia_pago"`
	ModalidadID *string `json:"modalidad_id"`
	// Nota queda en la auditoría: por qué se corrigió y con qué respaldo.
	Nota string `json:"nota"`
}

// CorregirContrato arregla los datos que dejaron al contrato fuera de la generación de cargos y,
// si ya quedó completo, le levanta la marca de revisión.
//
// La marca NO se levanta a mano: se deriva de que el dato esté bueno. Si se pudiera desmarcar sin
// arreglar la cuota, el contrato entraría a la cola de cobro con cuota 0 y el cobrador llamaría a
// alguien para pedirle nada.
func (s *Service) CorregirContrato(ctx context.Context, empresaID, usuarioID, numero string, c CorreccionContrato) (Contrato, error) {
	// Se busca por (empresa, número), que es el mismo camino que usan las otras acciones sobre un
	// contrato puntual del módulo (suspender, reactivar, registrar gestión). El aislamiento que
	// importa —no ver ni tocar otra empresa— lo da el empresa_id del token.
	actual, err := s.repo.ContratoPorNumero(ctx, empresaID, numero)
	if err != nil {
		return Contrato{}, err
	}

	cambios, err := validarCorreccion(c)
	if err != nil {
		return Contrato{}, err
	}

	// Lo único que no se puede validar sin la base: que la modalidad exista en ESTE catálogo.
	if id, ok := cambios["modalidad_id"].(string); ok {
		existe, err := s.repo.ModalidadExiste(ctx, empresaID, id)
		if err != nil {
			return Contrato{}, err
		}
		if !existe {
			return Contrato{}, fmt.Errorf("%w: %q", ErrModalidadNoEncontrada, id)
		}
	}

	actualizado, err := s.repo.CorregirContrato(ctx, empresaID, actual.ID, cambios)
	if err != nil {
		return Contrato{}, err
	}

	// La auditoría guarda el ANTES y el DESPUÉS de lo que se tocó: es un cambio sobre la deuda de
	// una persona y tiene que poder explicarse meses después.
	s.auditar(ctx, empresaID, "CORREGIR_CONTRATO_CXC", usuarioID, map[string]any{
		"contrato":         actual.Numero,
		"nota":             c.Nota,
		"cuota_antes":      actual.Cuota,
		"cuota_despues":    actualizado.Cuota,
		"revision_antes":   actual.RevisionPendiente,
		"revision_despues": actualizado.RevisionPendiente,
	})
	return actualizado, nil
}

// validarCorreccion traduce el pedido a las columnas a actualizar, o falla.
//
// Es PURA a propósito: la interfaz del repositorio tiene 59 métodos, así que probar esta validación
// con un doble completo sería más ruido que prueba. Acá vive la regla que importa —una cuota en cero
// no «resuelve» nada— y se prueba sola.
func validarCorreccion(c CorreccionContrato) (map[string]any, error) {
	cambios := map[string]any{}

	if c.Cuota != nil {
		cuota, err := decimal.NewFromString(strings.TrimSpace(*c.Cuota))
		if err != nil || !cuota.IsPositive() {
			return nil, fmt.Errorf("%w: %q", ErrCuotaInvalida, *c.Cuota)
		}
		cambios["cuota_vigente"] = cuota
	}
	if c.DiaPago != nil {
		if *c.DiaPago < 1 || *c.DiaPago > 31 {
			return nil, fmt.Errorf("%w: %d", ErrDiaPagoInvalido, *c.DiaPago)
		}
		cambios["dia_pago"] = *c.DiaPago
	}
	if c.ModalidadID != nil {
		id := strings.TrimSpace(*c.ModalidadID)
		if id == "" {
			return nil, fmt.Errorf("%w: vacía", ErrModalidadNoEncontrada)
		}
		cambios["modalidad_id"] = id
	}

	// Sin nada que cambiar no se toca la base ni se escribe un evento de auditoría: un registro que
	// dice «alguien corrigió» sin ningún cambio ensucia la bitácora que después hay que leer.
	if len(cambios) == 0 {
		return nil, ErrNadaQueCorregir
	}
	return cambios, nil
}
