package cxp

import (
	"errors"
	"time"
)

var (
	// ErrComprobanteNoEncontrado indica que la factura no tiene comprobante adjunto.
	ErrComprobanteNoEncontrado = errors.New("cxp: la factura no tiene comprobante adjunto")
	// ErrDocNoPagado indica que el comprobante solo aplica a facturas pagadas/conciliadas.
	ErrDocNoPagado = errors.New("cxp: el comprobante solo aplica a facturas pagadas o conciliadas")
	// ErrProveedorSinEmail indica que el proveedor no tiene correo para enviarle el comprobante.
	ErrProveedorSinEmail = errors.New("cxp: el proveedor no tiene correo registrado")
	// ErrCifradoNoDisponible: la empresa tiene guardada la contraseña de su buzón y el servidor no
	// puede leerla porque falta CIFRADO_SECRET.
	//
	// Es 422 y nombra la variable, porque es lo único que le sirve a quien tiene que arreglarlo. Y
	// NO se cae al correo global: mandar igual por el buzón del grupo sería volver al defecto que
	// la migración 0084 arregla, ahora en silencio.
	ErrCifradoNoDisponible = errors.New("cxp: esta empresa tiene guardada la contraseña de su correo y el servidor no puede leerla: falta la variable de entorno CIFRADO_SECRET")
	// ErrSecretoCorreoIlegible: la contraseña guardada no descifra. Clave rotada sin volver a
	// escribir las contraseñas, o la fila fue manipulada.
	ErrSecretoCorreoIlegible = errors.New("cxp: la contraseña guardada del correo de esta empresa no se puede leer (¿cambió CIFRADO_SECRET?); volvé a escribirla en Configuración › Correo saliente")
)

// Comprobante es el archivo adjunto de pago (para descarga).
type Comprobante struct {
	Filename  string
	Mime      string
	Contenido []byte
}

// ComprobanteEnvio agrega los datos del proveedor y la factura para el correo.
type ComprobanteEnvio struct {
	Comprobante
	ProveedorEmail  string
	ProveedorNombre string
	Consecutivo     string
	TotalCRC        string
	// Datos que alimentan la plantilla del correo: el monto se informa en la MONEDA de la
	// factura (antes se decía «₡» aunque fuera en dólares).
	Moneda      string
	Total       string
	Huella      string
	Descripcion string
	// AprobadorEmail / AprobadorNombre: quien aprobó el pago. Va en COPIA OCULTA del comprobante
	// (decisión del Director Financiero, 17-set-2026).
	//
	// Vacío es un caso NORMAL, no un error: si la factura no tiene aprobación registrada o el
	// aprobador no tiene correo, el envío NO se cae — sale sin copia y queda escrito en la
	// bitácora, que es donde se audita de verdad.
	AprobadorEmail  string
	AprobadorNombre string
	// SubidoEn es cuándo se adjuntó ESTE archivo. Se guarda en la bitácora para poder distinguir
	// «se reenvió el mismo PDF» de «se mandó otro PDF».
	SubidoEn time.Time
}

// RegistroEnvio es lo que se escribe en la bitácora `comprobante_envio`: una fila por intento,
// salga bien o mal.
type RegistroEnvio struct {
	Destinatario string
	// Copia es la dirección del aprobador. Vacío = no había a quién copiar.
	Copia     string
	Remitente string
	// Origen: EMPRESA (correo_saliente) o GLOBAL (las variables del servidor). Es lo que permite
	// ver de un vistazo qué empresas siguen mandando desde el buzón equivocado.
	Origen              string
	Archivo             string
	ComprobanteSubidoEn *time.Time
	Resultado           string
	ErrorCategoria      string
	Error               string
	UsuarioID           string
}

// Resultados posibles de un envío.
const (
	EnvioOK    = "OK"
	EnvioError = "ERROR"
)

// EnvioComprobante es una fila de la bitácora, como la lee la pantalla.
type EnvioComprobante struct {
	ID           string    `json:"id"`
	EnviadoEn    time.Time `json:"enviado_en"`
	Destinatario string    `json:"destinatario"`
	Copia        string    `json:"copia"`
	Remitente    string    `json:"remitente"`
	Origen       string    `json:"origen"`
	Archivo      string    `json:"archivo"`
	Resultado    string    `json:"resultado"`
	// ErrorCategoria es la causa clasificada; `Error` es la frase para el operador de CxP. Ninguno
	// de los dos trae el host, el usuario del buzón ni el texto crudo del servidor: esta bitácora
	// se lee con cxp.ver, que hoy tienen siete roles, y el detalle técnico vive en
	// correo_saliente.probado_error, detrás de admin.correo. (Hallazgo S-3.)
	ErrorCategoria string  `json:"error_categoria"`
	Error          *string `json:"error"`
	EnviadoPor     string  `json:"enviado_por"`
	// Reenvio: ya había un envío OK ANTERIOR de este mismo documento. Se deriva de la bitácora, no
	// se guarda: el primer OK es el envío y los siguientes son reenvíos, y eso no puede
	// desincronizarse de los datos.
	Reenvio bool `json:"reenvio"`
	// MismoArchivo: lo que se mandó en esta fila es el PDF que está adjunto AHORA.
	MismoArchivo bool `json:"mismo_archivo"`
}

// ResultadoEnvioComprobante es lo que devuelve el POST de enviar: la pantalla necesita poder decir
// a quién se le mandó sin volver a preguntar.
type ResultadoEnvioComprobante struct {
	Destinatario string    `json:"destinatario"`
	Copia        string    `json:"copia"`
	Origen       string    `json:"origen"`
	Reenvio      bool      `json:"reenvio"`
	EnviadoEn    time.Time `json:"enviado_en"`
}
