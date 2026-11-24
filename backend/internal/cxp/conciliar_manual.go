package cxp

// Enlazar A MANO un débito con su factura (20-oct-2026, hallazgo 3.1).
//
// ── POR QUÉ HACÍA FALTA OTRO CAMINO ─────────────────────────────────────────────────────────
//
// Ya existía `ConciliarMatch`, pero empareja POR HUELLA: recibe una descripción, le saca el código
// y busca la factura. Eso no sirve para lo que el arqueo encuentra, que es exactamente el caso
// contrario — el débito existe, el pago salió, pero su descripción NO trae la huella (el banco la
// recortó, el pago se hizo por fuera de la macro, alguien la tecleó distinta). Si la huella
// estuviera, el barrido ya lo habría enlazado solo y la factura no estaría en el reporte.
//
// Por eso este camino arranca al revés: una persona eligió ESTE movimiento para ESTA factura.
//
// ── LO QUE SIGUE VERIFICÁNDOSE, Y POR QUÉ ───────────────────────────────────────────────────
//
// Que lo elija una persona no relaja la moneda ni el monto. Son las mismas dos comprobaciones del
// barrido automático y en el mismo orden:
//
//   · La MONEDA primero. Hay cinco cuentas en dólares; dar por buena una factura de USD 1.500
//     contra un débito de ₡1.500 es comparar números, no plata.
//   · El MONTO exacto. En colones no hay diferencial que justifique tolerancia, y si el banco
//     debitó otra cifra eso es un hecho DISTINTO —pago parcial, retención no aplicada, otra
//     factura—, que es el hallazgo 3.5 y todavía no tiene salida. Enlazarlo igual cerraría la
//     factura por una cifra que nadie mandó pagar y nadie lo volvería a mirar, porque quedaría
//     «conciliada».
//
// Aflojar acá convertiría la herramienta que destapa el problema en la que lo tapa.
//
// ── EL MOTIVO ───────────────────────────────────────────────────────────────────────────────
//
// Obligatorio, como en liberar y en cerrar por otro medio. Enlazar a mano es afirmar «este débito
// ES el pago de esta factura» sin que el sistema lo pueda comprobar por la huella. Dentro de seis
// meses, lo único que permite reconstruir por qué se emparejaron es quién lo dijo y con qué
// argumento.

import (
	"context"
	"errors"
	"strings"

	"github.com/shopspring/decimal"
)

// EL MONTO Y LA MONEDA QUE NO CALZAN NO SON ERRORES: SON VEREDICTOS.
//
// Es la misma decisión que ya tomó `ConciliarHuella`, y acá importa el doble. Este resultado cruza
// el PUERTO hacia Bancos, y Bancos no puede importar CxP —para eso existe el puerto—, así que un
// error de este paquete llegaría allá como un error opaco y saldría por el `default` como 500
// «error interno»: el mensaje redactado con cuidado no llegaría a la pantalla. Es exactamente el
// modo de falla que el centinela de traducción vigila.
//
// Viajando como veredicto, Bancos lo traduce mirando un string que ya conoce, y el reporte puede
// decir «el banco debitó ₡120.000 y la factura espera ₡150.000», que es lo que alguien necesita
// leer para decidir qué hacer.
var (
	// ErrEnlaceMotivoRequerido: sin motivo no se enlaza a mano.
	//
	// Es el ÚNICO centinela de este camino, y sobrevive como error —no como veredicto— porque es
	// una falla de la petición, no del emparejamiento: no hay nada que informar sobre la factura.
	// Bancos valida el motivo antes de llamar, así que en la práctica no cruza el puerto; queda
	// acá como red por si alguien llama al servicio desde otro lado.
	ErrEnlaceMotivoRequerido = errors.New("cxp: decí por qué este débito es el pago de esta factura")
)

// ConciliarPorDocumento cierra una factura contra un débito elegido por una persona.
//
// Devuelve el mismo `ResultadoHuella` que el barrido automático a propósito: Bancos enlaza el
// movimiento con lo que venga en ese resultado, y dos formas distintas de decir lo mismo son dos
// formas de que una se quede atrás.
func (s *Service) ConciliarPorDocumento(ctx context.Context, empresaID, documentoID, montoBanco, monedaBanco, usuarioID, motivo string) (ResultadoHuella, error) {
	// El mismo mínimo que liberar y que reabrir un lote: veinte caracteres descartan la respuesta
	// refleja («ok», «ya») que deja el expediente sin nada que leer. Un solo número para la misma
	// clase de huella, o se vuelven tres reglas que alguien tiene que recordar.
	if len([]rune(strings.TrimSpace(motivo))) < motivoLiberacionMinimo {
		return ResultadoHuella{}, ErrEnlaceMotivoRequerido
	}
	doc, err := s.repo.DocumentoPorID(ctx, empresaID, documentoID)
	if err != nil {
		return ResultadoHuella{}, err
	}
	// Solo se concilia lo que YA se declaró pagado. Conciliar una factura que nadie mandó pagar
	// saltearía el flujo entero —aprobación incluida— por una pantalla de cuadre.
	if doc.Estado != EstPagado && doc.Estado != EstProgramado {
		return ResultadoHuella{Veredicto: HuellaEstadoInvalido, DocumentoID: doc.ID,
			Consecutivo: doc.Consecutivo, Proveedor: doc.Proveedor}, nil
	}

	esperado, err := s.repo.NetoAPagar(ctx, empresaID, doc.ID)
	if err != nil {
		return ResultadoHuella{}, err
	}
	res := ResultadoHuella{
		Huella:          doc.Huella,
		DocumentoID:     doc.ID,
		Consecutivo:     doc.Consecutivo,
		Proveedor:       doc.Proveedor,
		ConceptoID:      doc.ConceptoID,
		ClasificacionID: doc.ClasificacionID,
		MontoEsperado:   esperado,
		MontoBanco:      montoBanco,
		MonedaEsperada:  doc.Moneda,
	}
	if distintaMoneda(monedaBanco, doc.Moneda) {
		res.Veredicto = HuellaMonedaDiferente
		return res, nil
	}
	banco, err := decimal.NewFromString(montoBanco)
	if err != nil {
		return ResultadoHuella{}, err
	}
	if !banco.Equal(decOrCero(esperado)) {
		res.Veredicto = HuellaMontoDiferente
		return res, nil
	}

	if doc.Estado == EstProgramado {
		if _, err := s.repo.CambiarEstado(ctx, empresaID, doc.ID, EstProgramado, EstPagado); err != nil {
			return ResultadoHuella{}, err
		}
	}
	n, err := s.repo.CambiarEstado(ctx, empresaID, doc.ID, EstPagado, EstConciliado)
	if err != nil {
		return ResultadoHuella{}, err
	}
	if n == 0 {
		// Alguien la movió entremedio. No se declara conciliado lo que no cambió, y va como
		// veredicto por la misma razón que el resto: esto cruza el puerto.
		res.Veredicto = HuellaEstadoInvalido
		return res, nil
	}
	// Acción PROPIA y no CONCILIAR_AUTO: en la bitácora tiene que poder distinguirse lo que el
	// barrido emparejó solo de lo que una persona afirmó. Son dos niveles de evidencia distintos.
	s.auditarDocNota(ctx, empresaID, doc.ID, "CONCILIAR_MANUAL", usuarioID, motivo)
	res.Veredicto = HuellaConciliado
	res.Facturas = 1
	return res, nil
}
