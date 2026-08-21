package cxp

// Puerto de la huella Bancos↔CxP.
//
// El dominio define la huella como la terna (descripción única + monto + fecha): con ella, al
// importar el estado de cuenta, el movimiento se empareja con el pago de CxP. Acá vive el lado
// de CxP: reconocer la huella en una descripción y decidir si ese movimiento ES el pago.
//
// La verificación del MONTO no es un lujo: si el banco debitó una cifra distinta a la que la
// factura mandaba pagar, eso es un hallazgo, no una conciliación. En ese caso NO se concilia y
// se devuelve el veredicto con las dos cifras para que alguien lo mire.

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
)

// Veredictos del emparejamiento por huella.
const (
	// HuellaSinHuella: la descripción no trae huella; el movimiento no es un pago de CxP.
	HuellaSinHuella = "SIN_HUELLA"
	// HuellaSinDocumento: hay huella pero ningún documento pagable la reclama (ya conciliado,
	// anulado, o de otra empresa).
	HuellaSinDocumento = "SIN_DOCUMENTO"
	// HuellaConciliado: emparejado y el documento quedó CONCILIADO.
	HuellaConciliado = "CONCILIADO"
	// HuellaMontoDiferente: el documento existe pero el banco debitó otro monto. No se concilia.
	HuellaMontoDiferente = "MONTO_DIFERENTE"
)

// ResultadoHuella es lo que CxP le contesta a Bancos por cada movimiento examinado.
type ResultadoHuella struct {
	Veredicto string
	Huella    string
	// Datos del documento (vacíos si no se encontró).
	DocumentoID     string
	Consecutivo     string
	Proveedor       string
	ConceptoID      string
	ClasificacionID string
	// MontoEsperado es el neto que la factura mandaba pagar; MontoBanco lo que el banco debitó.
	MontoEsperado string
	MontoBanco    string
}

// PrefijoHuella es el literal por el que Bancos puede filtrar en SQL sin conocer el formato
// completo de la huella (que es asunto de CxP).
func (s *Service) PrefijoHuella() string { return "CXP-" }

// HuellaEnDescripcion extrae la huella de una descripción bancaria.
func (s *Service) HuellaEnDescripcion(descripcion string) (string, bool) {
	return extraerHuella(descripcion)
}

// ConciliarHuella empareja un movimiento bancario con su pago de CxP.
//
// montoBanco es el débito del movimiento en CRC ("" = no verificar, para el emparejamiento
// manual que ya existía). Solo concilia cuando el monto coincide con el neto de la factura.
func (s *Service) ConciliarHuella(ctx context.Context, empresaID, descripcion, montoBanco, usuarioID string) (ResultadoHuella, error) {
	huella, ok := extraerHuella(descripcion)
	if !ok {
		return ResultadoHuella{Veredicto: HuellaSinHuella}, nil
	}
	doc, err := s.repo.DocumentoPorHuella(ctx, empresaID, huella)
	if err != nil {
		if errors.Is(err, ErrDocumentoNoEncontrado) {
			return ResultadoHuella{Veredicto: HuellaSinDocumento, Huella: huella}, nil
		}
		return ResultadoHuella{}, err
	}
	// El monto esperado se pide con la MISMA expresión que arma el archivo de pago, para que
	// nunca haya dos definiciones del neto que puedan divergir.
	esperado, err := s.repo.NetoAPagar(ctx, empresaID, doc.ID)
	if err != nil {
		return ResultadoHuella{}, err
	}
	res := ResultadoHuella{
		Huella:          huella,
		DocumentoID:     doc.ID,
		Consecutivo:     doc.Consecutivo,
		Proveedor:       doc.Proveedor,
		ConceptoID:      doc.ConceptoID,
		ClasificacionID: doc.ClasificacionID,
		MontoEsperado:   esperado,
		MontoBanco:      montoBanco,
	}

	if montoBanco != "" {
		banco, errB := decimal.NewFromString(montoBanco)
		if errB != nil {
			return ResultadoHuella{}, errB
		}
		// Exacto: en colones no hay diferencial que justifique tolerancia. Si difiere, es un
		// hecho distinto (pago parcial, retención no aplicada, otra factura) y lo ve una persona.
		if !banco.Equal(decOrCero(esperado)) {
			res.Veredicto = HuellaMontoDiferente
			return res, nil
		}
	}

	// El movimiento bancario ES el pago: se lleva a CONCILIADO respetando el flujo.
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
		// Alguien movió el documento entremedio: no se declara conciliado lo que no cambió.
		res.Veredicto = HuellaSinDocumento
		return res, nil
	}
	s.auditarDoc(ctx, empresaID, doc.ID, "CONCILIAR_AUTO", usuarioID)
	res.Veredicto = HuellaConciliado
	return res, nil
}

// decOrCero parsea un monto tolerando vacío. Nunca usa float64.
func decOrCero(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}
