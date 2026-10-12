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
	// RemesaID: el pago cubre VARIAS facturas de un proveedor. En ese caso DocumentoID viene vacío
	// —no hay UNA factura a la que enlazar el movimiento— y el enlace se escribe contra la remesa.
	RemesaID string
	// Facturas es cuántas cerró este pago (1 en el pago suelto). Va al reporte: «un movimiento,
	// cinco facturas» es lo que alguien quiere ver al cuadrar.
	Facturas int
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
	// Examinados: CANDIDATOS que trajo el pre-filtro (descripciones que contienen «CXP» y todavía
	// no están enlazadas). No todos son pagos nuestros: ver SinHuella.
	Examinados  int `json:"examinados"`
	Conciliados int `json:"conciliados"`
	// MontoDiferente: el código calzó pero el banco debitó otra cifra. NO se concilia a propósito.
	MontoDiferente int `json:"monto_diferente"`
	// SinDocumento: hay código de CxP pero ninguna factura pagable lo reclama.
	SinDocumento int `json:"sin_documento"`
	// SinHuella: candidatos que NO son pagos de CxP (el pre-filtro trae de más a propósito, porque
	// el banco borra el guion del código y un filtro estricto no traería ninguno de los buenos).
	// Se informan para que el total cuadre, pero no son un problema que nadie tenga que mirar.
	SinHuella int              `json:"sin_huella"`
	Detalle   []PagoConciliado `json:"detalle"`
	// Disponible: false cuando CxP no está conectado (nada que hacer, no es un error).
	Disponible bool `json:"disponible"`
}

// SetConciliadorCxP conecta CxP. Sin esto, el barrido no hace nada.
func (s *Service) SetConciliadorCxP(c ConciliadorCxP) { s.conciliadorCxP = c }
