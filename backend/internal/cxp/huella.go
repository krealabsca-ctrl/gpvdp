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
	"strings"

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
	// HuellaMonedaDiferente: el documento existe pero está en otra moneda que el débito.
	//
	// Es un veredicto aparte y no un caso de MONTO_DIFERENTE porque el problema no es la cifra:
	// una factura de USD 1.500 y un débito de ₡1.500 son números iguales y hechos incomparables.
	// Comparar importes sin mirar la moneda los daría por conciliados.
	HuellaMonedaDiferente = "MONEDA_DIFERENTE"
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
	// MonedaEsperada es la de la factura (o CRC en una remesa, que se declara siempre en colones).
	// Sin ella, MontoEsperado es un número sin unidad.
	MonedaEsperada string
	// RemesaID no está vacío cuando el pago cubre VARIAS facturas de un proveedor. En ese caso
	// DocumentoID queda vacío a propósito: no hay UNA factura a la que enlazar el movimiento, y
	// elegir la primera diría que esa factura costó lo que costó el grupo entero.
	RemesaID string
	// Facturas es cuántas cerró este pago (1 en el pago suelto). Va al reporte del barrido: «un
	// movimiento, cinco facturas» es justo lo que alguien quiere ver al cuadrar.
	Facturas int
}

// PrefijoHuella es el literal por el que Bancos puede filtrar en SQL sin conocer el formato
// completo de la huella (que es asunto de CxP).
//
// Va SIN el guion: el banco lo borra en el camino (ver reHuella), y un filtro con guion descarta
// justo los movimientos que hay que examinar. Es un pre-filtro barato, no el reconocedor: trae de
// más —una descripción ajena como «FIDE 1937 CXP 430581» entra— y el patrón decide después. Traer
// de más cuesta unas filas; traer de menos cuesta la conciliación entera, en silencio.
func (s *Service) PrefijoHuella() string { return "CXP" }

// HuellaEnDescripcion extrae la huella de una descripción bancaria.
func (s *Service) HuellaEnDescripcion(descripcion string) (string, bool) {
	return extraerHuella(descripcion)
}

// ConciliarHuella empareja un movimiento bancario con su pago de CxP.
//
// montoBanco es el débito del movimiento y monedaBanco la moneda en que está expresado ("" = no
// verificar, para el emparejamiento manual que ya existía). Solo concilia cuando la MONEDA y el
// monto coinciden con los de la factura, en ese orden: comparar importes de monedas distintas es
// comparar números, no plata.
func (s *Service) ConciliarHuella(ctx context.Context, empresaID, descripcion, montoBanco, monedaBanco, usuarioID string) (ResultadoHuella, error) {
	huella, ok := extraerHuella(descripcion)
	if !ok {
		return ResultadoHuella{Veredicto: HuellaSinHuella}, nil
	}
	// Un pago agrupado cierra VARIAS facturas y se verifica contra el monto de la remesa, no contra
	// el de ninguna de ellas. Es otro camino desde el primer paso: buscar una factura con este
	// código no encontraría nada y el veredicto sería «sin documento» con el pago bien hecho.
	if EsRemesa(huella) {
		return s.conciliarRemesa(ctx, empresaID, huella, montoBanco, monedaBanco, usuarioID)
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
		MonedaEsperada:  doc.Moneda,
	}

	if montoBanco != "" {
		// La MONEDA primero. Hay cinco cuentas bancarias en dólares, y el neto de la factura está
		// en la moneda de la factura: comparar los dos importes sin mirar la unidad daría por
		// conciliada una factura de USD 1.500 contra un débito de ₡1.500.
		//
		// Cuando difieren NO se convierte: con qué tipo de cambio —el del día del débito, compra o
		// venta— es una regla que no está definida, y elegir una acá la inventaría. Se reporta
		// para que una persona la resuelva.
		if distintaMoneda(monedaBanco, doc.Moneda) {
			res.Veredicto = HuellaMonedaDiferente
			return res, nil
		}
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
	res.Facturas = 1
	return res, nil
}

// conciliarRemesa empareja un débito con un pago agrupado y cierra TODAS sus facturas.
//
// La verificación del monto es contra lo que declaró la remesa, que es lo que se le mandó pagar al
// banco. Si difiere, no se concilia nada: con cinco facturas adentro, conciliar «casi» sería dejar
// un grupo cerrado por una cifra que nadie mandó pagar.
func (s *Service) conciliarRemesa(ctx context.Context, empresaID, huella, montoBanco, monedaBanco, usuarioID string) (ResultadoHuella, error) {
	rem, err := s.repo.RemesaPorHuella(ctx, empresaID, huella)
	if err != nil {
		if errors.Is(err, ErrRemesaNoEncontrada) {
			return ResultadoHuella{Veredicto: HuellaSinDocumento, Huella: huella}, nil
		}
		return ResultadoHuella{}, err
	}
	res := ResultadoHuella{
		Huella:    huella,
		RemesaID:  rem.ID,
		Proveedor: rem.Proveedor,
		// `remesa_pago` guarda `monto_crc`: una remesa se declara SIEMPRE en colones, cualquiera
		// sea la moneda de las facturas que agrupa. Por eso acá la moneda esperada es literal y no
		// sale de la fila.
		MontoEsperado:  rem.MontoCRC,
		MontoBanco:     montoBanco,
		MonedaEsperada: MonedaCRC,
		// El consecutivo de UNA factura no describe el pago. Lo que lo describe es su referencia,
		// con el número REAL del lote: es lo que trae el estado de cuenta y con lo que se cruza.
		Consecutivo: ReferenciaRemesa(rem.LoteNumero, rem.Numero),
	}
	if montoBanco != "" {
		// Una remesa está en colones, así que un débito de una cuenta en dólares no la paga —no
		// con estos números—. Sin esta puerta, un débito de USD 125.000 cerraría una remesa de
		// ₡125.000 y con ella TODAS sus facturas de una vez.
		if distintaMoneda(monedaBanco, MonedaCRC) {
			res.Veredicto = HuellaMonedaDiferente
			return res, nil
		}
		banco, errB := decimal.NewFromString(montoBanco)
		if errB != nil {
			return ResultadoHuella{}, errB
		}
		if !banco.Equal(decOrCero(rem.MontoCRC)) {
			res.Veredicto = HuellaMontoDiferente
			return res, nil
		}
	}

	cerradas, conceptoID, clasificacionID, err := s.repo.CerrarRemesa(ctx, empresaID, rem.ID)
	if err != nil {
		return ResultadoHuella{}, err
	}
	if cerradas == 0 {
		// Ninguna factura cambió: ya estaban cerradas, o se anularon entre el corte y hoy. No se
		// declara conciliado lo que no se movió.
		res.Veredicto = HuellaSinDocumento
		return res, nil
	}
	res.ConceptoID = conceptoID
	res.ClasificacionID = clasificacionID
	res.Facturas = cerradas
	res.Veredicto = HuellaConciliado
	s.auditarDoc(ctx, empresaID, rem.ID, "CONCILIAR_REMESA", usuarioID)
	return res, nil
}

// MonedaCRC es la moneda en que se declaran las remesas, cualquiera sea la de sus facturas.
const MonedaCRC = "CRC"

// distintaMoneda compara dos monedas tolerando mayúsculas y espacios.
//
// Una moneda VACÍA significa «no se puede verificar», y entonces no bloquea: es la misma semántica
// que `montoBanco == ""`, que ya existía para el emparejamiento manual. No es un agujero: desde
// Bancos la moneda sale de `cuenta_bancaria.moneda`, que es NOT NULL con CHECK (CRC|USD), así que
// el barrido automático siempre la trae.
func distintaMoneda(a, b string) bool {
	na := strings.ToUpper(strings.TrimSpace(a))
	nb := strings.ToUpper(strings.TrimSpace(b))
	if na == "" || nb == "" {
		return false
	}
	return na != nb
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
