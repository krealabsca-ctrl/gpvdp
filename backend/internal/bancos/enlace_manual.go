package bancos

// Enlazar a mano un débito con su factura (20-oct-2026, hallazgo 3.1).
//
// El barrido automático empareja por huella. Cuando la descripción no la trae —el banco la cortó,
// el pago salió por fuera de la macro, alguien la tecleó distinta—, el débito y la factura existen
// los dos y no hay nada que los una. Eso es exactamente lo que el arqueo de excepción encuentra:
// sobre los datos reales, 62 facturas declaradas pagadas sin respaldo, 17 de ellas con un débito
// candidato del mismo monto esperando a pocos días.
//
// Acá se cierra ese caso: una persona elige el movimiento, CxP verifica que la plata cuadre y el
// movimiento queda apuntando a la factura igual que si lo hubiera enlazado el barrido.
//
// EL ORDEN IMPORTA Y ES AL REVÉS DEL QUE PARECE. Primero se le pide a CxP que concilie —ahí se
// verifican moneda y monto, y ahí puede fallar—; solo si eso salió bien se escribe el enlace en el
// movimiento. Hacerlo al revés dejaría el movimiento apuntando a una factura que CxP rechazó, que
// es un huérfano que nadie vuelve a mirar porque el movimiento ya «tiene» su documento.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/gpvdp/erp/internal/shared"
)

var (
	// ErrMovimientoNoEnlazable: el movimiento no existe, no es un débito, está excluido de los
	// libros o ya está enlazado a otra factura o remesa.
	ErrMovimientoNoEnlazable = errors.New("bancos: ese movimiento no se puede enlazar: tiene que ser un débito activo y todavía sin factura")
	// ErrSinConciliadorCxP: CxP no está conectado en este armado.
	ErrSinConciliadorCxP = errors.New("bancos: cuentas por pagar no está disponible")
	// ErrMotivoEnlaceRequerido: enlazar a mano sin decir por qué. Es una afirmación con
	// consecuencias —«este débito ES el pago de esta factura»— que el sistema no puede comprobar
	// por la huella; el motivo es la única evidencia que queda.
	ErrMotivoEnlaceRequerido = errors.New("bancos: decí por qué este débito es el pago de esa factura")
)

// MovimientoEnlazable es lo mínimo que hace falta saber del débito elegido.
type MovimientoEnlazable struct {
	ID     string
	Debito string
	// Moneda es la de la CUENTA, que es en la que está expresado el débito. Viaja siempre: un
	// importe sin su moneda no se puede comparar con el neto de la factura, y suponer colones es
	// la forma silenciosa de equivocarse (hay cinco cuentas en dólares).
	Moneda string
}

// motivoEnlaceMinimo es el largo mínimo del motivo, el mismo que exige excluir un movimiento.
//
// Veinte caracteres no garantizan una explicación, pero descartan la respuesta refleja —«ok»,
// «ya»— que deja el expediente sin nada que leer seis meses después, que es justo para lo que
// existe el campo.
const motivoEnlaceMinimo = 20

// EnlazarPagoAMano cierra una factura contra un débito que eligió una persona.
func (s *Service) EnlazarPagoAMano(ctx context.Context, empresaID, movimientoID, documentoID, usuarioID, motivo string) (ResultadoHuella, error) {
	if s.conciliadorCxP == nil {
		return ResultadoHuella{}, ErrSinConciliadorCxP
	}
	// El motivo se valida ACÁ y no solo del otro lado del puerto: CxP lo rechazaría con un error
	// de su paquete, que llegaría opaco y saldría como 500.
	if len([]rune(strings.TrimSpace(motivo))) < motivoEnlaceMinimo {
		return ResultadoHuella{}, ErrMotivoEnlaceRequerido
	}
	mov, err := s.repo.MovimientoEnlazable(ctx, empresaID, movimientoID)
	if err != nil {
		return ResultadoHuella{}, err
	}

	// CxP primero: acá se verifican moneda y monto.
	res, err := s.conciliadorCxP.ConciliarDocumento(ctx, empresaID, documentoID, mov.Debito, mov.Moneda, usuarioID, motivo)
	if err != nil {
		return res, err
	}
	// Un veredicto distinto de CONCILIADO NO es un fallo del sistema: es la respuesta. El
	// movimiento NO se enlaza y el resultado viaja entero —lo esperado y lo que trajo el banco—
	// para que la pantalla pueda decir por qué. Enlazarlo igual cerraría la factura por una cifra
	// que nadie mandó pagar, y nadie la volvería a mirar porque quedaría «conciliada».
	if res.Veredicto != HuellaConciliado {
		return res, nil
	}

	clasificado, enlazado, err := s.repo.EnlazarPagoCxP(ctx, empresaID, mov.ID,
		res.DocumentoID, res.ConceptoID, res.ClasificacionID)
	if err != nil {
		return ResultadoHuella{}, err
	}
	if !enlazado {
		// Alguien lo tomó entremedio. La factura ya quedó conciliada por CxP, así que esto NO se
		// silencia: se dice, porque el movimiento quedó sin apuntar a nada y hace falta revisarlo.
		return res, ErrMovimientoNoEnlazable
	}

	// Acción PROPIA, no CONCILIAR_CXP: en la bitácora tiene que poder distinguirse lo que el
	// barrido emparejó por huella de lo que una persona afirmó, porque son dos niveles de
	// evidencia distintos. El motivo va adentro: es lo único que explica el emparejamiento.
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "movimiento_bancario", EntidadID: &mov.ID,
		Accion: "CONCILIAR_MANUAL", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"documento_id": res.DocumentoID, "consecutivo": res.Consecutivo,
			"monto": mov.Debito, "moneda": mov.Moneda, "clasificado": clasificado,
			"nota": motivo,
		},
	})
	return res, nil
}

// MovimientoEnlazable lee el débito elegido y comprueba que se pueda enlazar.
//
// Las condiciones van EN EL WHERE y no en una lectura previa seguida de una decisión en Go: entre
// las dos consultas otro barrido puede enlazar el mismo movimiento, y entonces se escribiría
// encima. Es la misma razón por la que la guarda de excluir un movimiento vive en su WHERE.
func (r *pgRepository) MovimientoEnlazable(ctx context.Context, empresaID, movimientoID string) (MovimientoEnlazable, error) {
	var m MovimientoEnlazable
	err := r.pool.QueryRow(ctx, `
		SELECT m.id::text, m.debito::text, COALESCE(cb.moneda, 'CRC')
		FROM movimiento_bancario m
		LEFT JOIN cuenta_bancaria cb ON cb.id = m.cuenta_bancaria_id
		WHERE m.empresa_id = $1::uuid AND m.id = $2::uuid
		  AND m.incluido
		  AND m.debito > 0
		  AND m.documento_cxp_id IS NULL AND m.remesa_id IS NULL`,
		empresaID, movimientoID).Scan(&m.ID, &m.Debito, &m.Moneda)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MovimientoEnlazable{}, ErrMovimientoNoEnlazable
		}
		return MovimientoEnlazable{}, fmt.Errorf("bancos: movimiento enlazable: %w", err)
	}
	return m, nil
}

// ── Deshacer el enlace ──────────────────────────────────────────────────────────────────────

// ErrMovimientoSinEnlace: se pidió desenlazar un movimiento que no apunta a ninguna factura.
var ErrMovimientoSinEnlace = errors.New("bancos: ese movimiento no está enlazado a ninguna factura")

// ErrNadaQueReabrir: el movimiento apunta a una factura que NO está conciliada. Es un estado
// inconsistente —alguien movió la factura por otro camino— y se dice en vez de taparlo limpiando
// el enlace: lo que hay que revisar es por qué quedaron desalineados.
var ErrNadaQueReabrir = errors.New("bancos: ese movimiento apunta a una factura que no está conciliada; revisá el estado de la factura antes de desenlazar")

// EnlaceDelMovimiento dice a qué apunta hoy un movimiento.
type EnlaceDelMovimiento struct {
	DocumentoID string
	RemesaID    string
}

// DesenlazarPago deshace una conciliación: el movimiento deja de apuntar a la factura y la factura
// vuelve a PAGADO, o sea a estar esperando su débito.
//
// EL ORDEN ES EL INVERSO AL DE ENLAZAR, y por la misma razón. Primero se le pide a CxP que abra la
// factura —ahí puede decir que no hay nada que abrir— y recién entonces se limpia el movimiento.
// Al revés quedaría un movimiento suelto apuntando a una factura que sigue conciliada: un pago sin
// respaldo que el arqueo no detecta, porque la factura se ve cerrada.
//
// La CLASIFICACIÓN del movimiento no se toca. El enlace solo clasifica cuando el movimiento venía
// sin identificar, y esa partida puede seguir siendo correcta aunque la factura no lo fuera;
// devolverlo a «no identificado» a ciegas desharía trabajo que alguien ya hizo bien.
func (s *Service) DesenlazarPago(ctx context.Context, empresaID, movimientoID, usuarioID, motivo string) (int, error) {
	if s.conciliadorCxP == nil {
		return 0, ErrSinConciliadorCxP
	}
	if len([]rune(strings.TrimSpace(motivo))) < motivoEnlaceMinimo {
		return 0, ErrMotivoEnlaceRequerido
	}
	enl, err := s.repo.EnlaceDelMovimiento(ctx, empresaID, movimientoID)
	if err != nil {
		return 0, err
	}

	vueltas, err := s.conciliadorCxP.DesconciliarPago(ctx, empresaID, enl.DocumentoID, enl.RemesaID, usuarioID, motivo)
	if err != nil {
		return 0, err
	}
	// Cero facturas reabiertas con el movimiento SÍ enlazado es un estado inconsistente: el
	// movimiento apunta a algo que no está conciliado. No se limpia el enlace a ciegas —sería
	// tapar la inconsistencia— y se dice, que es lo que permite ir a mirarla.
	if vueltas == 0 {
		return 0, ErrNadaQueReabrir
	}
	if _, err := s.repo.LimpiarEnlacePago(ctx, empresaID, movimientoID); err != nil {
		return 0, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "movimiento_bancario", EntidadID: &movimientoID,
		Accion: "DESCONCILIAR_MANUAL", UsuarioID: &usuarioID,
		// `shared.Evento` solo lleva valor nuevo, así que el enlace que se deshizo va adentro: sin
		// él, el evento diría que algo se desenlazó sin decir de qué, y eso es justo lo que hay que
		// poder reconstruir después.
		ValorNuevo: map[string]any{
			"documento_id_anterior": enl.DocumentoID, "remesa_id_anterior": enl.RemesaID,
			"facturas_reabiertas": vueltas, "nota": motivo,
		},
	})
	return vueltas, nil
}

// EnlaceDelMovimiento lee a qué apunta el movimiento, exigiendo que apunte a algo.
func (r *pgRepository) EnlaceDelMovimiento(ctx context.Context, empresaID, movimientoID string) (EnlaceDelMovimiento, error) {
	var e EnlaceDelMovimiento
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(documento_cxp_id::text,''), COALESCE(remesa_id::text,'')
		FROM movimiento_bancario
		WHERE empresa_id = $1::uuid AND id = $2::uuid
		  AND (documento_cxp_id IS NOT NULL OR remesa_id IS NOT NULL)`,
		empresaID, movimientoID).Scan(&e.DocumentoID, &e.RemesaID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EnlaceDelMovimiento{}, ErrMovimientoSinEnlace
		}
		return EnlaceDelMovimiento{}, fmt.Errorf("bancos: enlace del movimiento: %w", err)
	}
	return e, nil
}

// LimpiarEnlacePago borra las DOS columnas de enlace.
//
// Las dos y no solo la que venía puesta: son excluyentes por el CHECK de la 0089, así que limpiar
// ambas es seguro y deja el movimiento en un estado que no depende de cuál estaba escrita.
func (r *pgRepository) LimpiarEnlacePago(ctx context.Context, empresaID, movimientoID string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE movimiento_bancario
		   SET documento_cxp_id = NULL, remesa_id = NULL, actualizado_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, movimientoID)
	if err != nil {
		return 0, fmt.Errorf("bancos: limpiar enlace: %w", err)
	}
	return tag.RowsAffected(), nil
}
