package bancos

// Conciliación automática Bancos↔CxP por huella.
//
// El dominio lo define así: al pagar, CxP embebe una descripción única (la huella) en la
// instrucción que va al banco; al importar el estado de cuenta, esa huella empareja el
// movimiento con el pago y lo clasifica en la cuenta correspondiente.
//
// Bancos es dueño del movimiento; CxP es dueño de la huella y del estado del documento. Por eso
// esto habla con CxP por un puerto (igual que el fetcher del BCCR): sin ese puerto inyectado,
// el barrido simplemente no hace nada.

import "context"

// Veredictos que devuelve CxP por cada movimiento examinado (los define CxP; se reflejan acá
// para poder reportarlos sin importar el paquete).
const (
	HuellaConciliado     = "CONCILIADO"
	HuellaMontoDiferente = "MONTO_DIFERENTE"
	HuellaSinDocumento   = "SIN_DOCUMENTO"
	HuellaSinHuella      = "SIN_HUELLA"
)

// ResultadoHuella es la respuesta de CxP para un movimiento.
type ResultadoHuella struct {
	Veredicto       string
	Huella          string
	DocumentoID     string
	Consecutivo     string
	Proveedor       string
	ConceptoID      string
	ClasificacionID string
	MontoEsperado   string
	MontoBanco      string
}

// ConciliadorCxP es el puerto hacia CxP.
type ConciliadorCxP interface {
	// PrefijoHuella permite filtrar en SQL sin conocer el formato completo de la huella.
	PrefijoHuella() string
	// ConciliarHuella empareja el movimiento con su pago. montoBanco en la moneda del movimiento.
	ConciliarHuella(ctx context.Context, empresaID, descripcion, montoBanco, usuarioID string) (ResultadoHuella, error)
}

// MovimientoConHuella es un movimiento bancario candidato a ser el pago de una factura.
type MovimientoConHuella struct {
	ID          string `json:"id"`
	Fecha       string `json:"fecha"`
	Descripcion string `json:"descripcion"`
	Debito      string `json:"debito"`
	Cuenta      string `json:"cuenta"`
}

// PagoConciliado es una línea del reporte del barrido.
type PagoConciliado struct {
	MovimientoID  string `json:"movimiento_id"`
	Fecha         string `json:"fecha"`
	Cuenta        string `json:"cuenta"`
	Huella        string `json:"huella"`
	Veredicto     string `json:"veredicto"`
	Consecutivo   string `json:"consecutivo"`
	Proveedor     string `json:"proveedor"`
	MontoBanco    string `json:"monto_banco"`
	MontoEsperado string `json:"monto_esperado"`
	// Clasificado: además de conciliar la factura, el movimiento quedó clasificado con el
	// concepto del documento (deja de contar como No identificado).
	Clasificado bool `json:"clasificado"`
}

// ReporteConciliacionCxP es el resultado del barrido.
type ReporteConciliacionCxP struct {
	// Examinados: movimientos con huella que todavía no estaban enlazados a una factura.
	Examinados     int              `json:"examinados"`
	Conciliados    int              `json:"conciliados"`
	MontoDiferente int              `json:"monto_diferente"`
	SinDocumento   int              `json:"sin_documento"`
	Detalle        []PagoConciliado `json:"detalle"`
	// Disponible: false cuando CxP no está conectado (nada que hacer, no es un error).
	Disponible bool `json:"disponible"`
}

// SetConciliadorCxP conecta CxP. Sin esto, el barrido no hace nada.
func (s *Service) SetConciliadorCxP(c ConciliadorCxP) { s.conciliadorCxP = c }
