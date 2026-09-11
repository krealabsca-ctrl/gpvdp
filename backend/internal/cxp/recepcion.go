package cxp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// ── RECEPCIÓN DE FACTURAS ELECTRÓNICAS POR BUZÓN DE CORREO ──────────────────────────────────
//
// El comprobante entra directo al ERP: el script del buzón hace POST del XML y del PDF, y acá se
// coteja, se interpreta y —si es una factura electrónica de esta empresa— se crea la cuenta por
// pagar reusando el mismo camino que el importador manual.
//
// Ver docs/GPVDP_Ingesta_Facturas_EspecTecnica_v1.0.md.

// Estados de una recepción. Nada se borra: lo que no se pudo procesar queda con el motivo escrito.
const (
	// RecPendiente: llegó y todavía no se interpretó.
	RecPendiente = "PENDIENTE"
	// RecProcesada: se creó (o se enlazó) el documento de CxP.
	RecProcesada = "PROCESADA"
	// RecDuplicada: esa clave ya estaba registrada; se enlaza el documento existente.
	RecDuplicada = "DUPLICADA"
	// RecParqueada: hay algo que arreglar y se puede reintentar. Es la cola de errores.
	RecParqueada = "PARQUEADA"
	// RecDescartada: legítimamente no es una cuenta por pagar (nota de crédito, recibo de pago…).
	// Conserva el XML para que la etapa de notas de crédito tenga su materia prima.
	RecDescartada = "DESCARTADA"
)

var (
	// ErrFuenteNoEncontrada indica que la fuente de recepción no existe en esta empresa.
	ErrFuenteNoEncontrada = errors.New("cxp: la fuente de recepción no existe")
	// ErrTokenRecepcionInvalido indica un token de recepción desconocido, revocado o de una
	// empresa inactiva. Los tres casos dan el MISMO error a propósito: distinguirlos le diría a
	// quien prueba tokens cuál de ellos existió alguna vez.
	ErrTokenRecepcionInvalido = errors.New("cxp: token de recepción inválido")
	// ErrRecepcionNoEncontrada indica que la recepción no existe en esta empresa.
	ErrRecepcionNoEncontrada = errors.New("cxp: la recepción no existe")
	// ErrFuenteDuplicada indica que ya hay una fuente para ese buzón en esta empresa.
	ErrFuenteDuplicada = errors.New("cxp: ya existe una fuente para ese correo")
	// ErrRecepcionNoReintentable indica que la recepción ya se resolvió y no hay nada que reintentar.
	ErrRecepcionNoReintentable = errors.New("cxp: solo se puede reintentar una recepción parqueada")
)

// FuenteRecepcion es un buzón de correo dado de alta para una empresa. La fuente ES la credencial.
type FuenteRecepcion struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Correo string `json:"correo"`
	Activo bool   `json:"activo"`
	// UltimoContacto es el LATIDO: la última vez que el script llamó, aunque no trajera facturas.
	// Es lo que distingue «hoy no hubo facturas» de «esto está muerto».
	UltimoContacto string `json:"ultimo_contacto,omitempty"`
	CreadoEn       string `json:"creado_en"`
	// Recibidas/Parqueadas: el conteo por fuente, para que la pantalla diga si algo está trancado.
	Recibidas  int `json:"recibidas"`
	Parqueadas int `json:"parqueadas"`
}

// FuenteInput es el alta de una fuente.
type FuenteInput struct {
	Nombre string
	Correo string
}

// FuenteCreada devuelve el token EN CLARO, y es la única vez que se puede ver: la base solo guarda
// su hash. Si se pierde, se rota.
type FuenteCreada struct {
	Fuente FuenteRecepcion `json:"fuente"`
	Token  string          `json:"token"`
}

// Recepcion es una fila de la tabla de aterrizaje.
type Recepcion struct {
	ID            string `json:"id"`
	FuenteID      string `json:"fuente_id,omitempty"`
	FuenteNombre  string `json:"fuente_nombre,omitempty"`
	Clave         string `json:"clave,omitempty"`
	TipoDocumento string `json:"tipo_documento,omitempty"`
	VersionSchema string `json:"version_schema,omitempty"`
	Receptor      string `json:"receptor,omitempty"`
	MessageID     string `json:"message_id,omitempty"`
	Asunto        string `json:"asunto,omitempty"`
	Remitente     string `json:"remitente,omitempty"`
	Buzon         string `json:"buzon,omitempty"`
	Estado        string `json:"estado"`
	Motivo        string `json:"motivo,omitempty"`
	DocumentoID   string `json:"documento_id,omitempty"`
	Consecutivo   string `json:"consecutivo,omitempty"`
	Proveedor     string `json:"proveedor,omitempty"`
	Total         string `json:"total,omitempty"`
	Moneda        string `json:"moneda,omitempty"`
	Intentos      int    `json:"intentos"`
	TieneXML      bool   `json:"tiene_xml"`
	TienePDF      bool   `json:"tiene_pdf"`
	CreadoEn      string `json:"creado_en"`
	ProcesadoEn   string `json:"procesado_en,omitempty"`
}

// RecepcionInput es un comprobante que llega del buzón.
type RecepcionInput struct {
	// XML es el comprobante crudo. Es obligatorio: sin XML no hay nada que interpretar.
	XML []byte
	// PDF es la representación gráfica, si el correo la traía. Opcional.
	PDF         []byte
	PDFFilename string
	// Metadatos del correo. `Buzon` se compara contra el correo de la fuente del token: sin esa
	// comparación, un token pegado en el script equivocado manda las facturas a la otra empresa.
	MessageID string
	Asunto    string
	Remitente string
	Buzon     string
}

// ResultadoRecepcion es lo que contesta el endpoint por cada comprobante recibido.
//
// «Ya existe» NO es un error: se contesta 200 con Repetido = true. Con un 409 el script no puede
// etiquetar el hilo como procesado y lo vuelve a mandar para siempre, así que la cola de errores se
// llenaría en el camino feliz.
type ResultadoRecepcion struct {
	RecepcionID string `json:"recepcion_id"`
	Estado      string `json:"estado"`
	Motivo      string `json:"motivo,omitempty"`
	Repetido    bool   `json:"repetido"`
	Clave       string `json:"clave,omitempty"`
	DocumentoID string `json:"documento_id,omitempty"`
	Consecutivo string `json:"consecutivo,omitempty"`
}

// TokenMaquina es lo que el middleware resuelve a partir del token: de qué empresa y de qué buzón.
type TokenMaquina struct {
	FuenteID  string
	EmpresaID string
	Nombre    string
	Correo    string
}

// generarTokenRecepcion crea el secreto del buzón y su hash.
//
// Mismo algoritmo que el refresh token de `internal/auth`: 32 bytes de crypto/rand en
// base64 URL-safe. La entropía es la ÚNICA defensa contra la fuerza bruta —el backend no tiene
// rate limiting en ninguna ruta—, así que el token no se acorta ni se le pone un prefijo legible.
func generarTokenRecepcion() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashTokenRecepcion(token), nil
}

// hashTokenRecepcion es sha256 en hexadecimal, igual que en `internal/auth`.
//
// La verificación NO compara secretos en Go: hashea lo que llegó y BUSCA POR EL DIGEST en el índice
// único. Así no hay canal de temporización y la fila de la base es el único punto de revocación.
func hashTokenRecepcion(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ── EL COTEJO DEL RECEPTOR ──────────────────────────────────────────────────
//
// `UNIQUE (empresa_id, clave)` significa que la misma factura PUEDE existir en dos empresas. Este
// cotejo no es validación de lujo: es lo único que impide que una factura de Coopeprofa aterrice
// en Valle de Paz.
//
// Se escribe con ramas EXPLÍCITAS y nunca con un `==` sobre cadenas que pueden venir vacías. La
// razón es concreta: un tiquete electrónico no trae nodo Receptor y una empresa sin cédulas
// cargadas escanea a "", así que `receptor == cedula` daría `"" == ""` ⇒ CALZA, y el comprobante
// entraría en esa empresa sin que nada lo cuestione. Es la misma lección que ya está escrita en
// bancos/repository_clasif.go: un alcance vacío tiene que CERRAR, no abrir.
//
// Devuelve el motivo en palabras cuando no calza, porque ese texto es lo que la persona va a leer
// en la cola de errores para saber qué arreglar.
func cotejarReceptor(receptor string, cedulas []string) (calza bool, motivo string) {
	rec := soloDigitos(strings.TrimSpace(receptor))

	if len(cedulas) == 0 {
		return false, "esta empresa no tiene cédulas jurídicas configuradas, así que no se puede " +
			"verificar que la factura sea suya"
	}
	if rec == "" {
		return false, "el comprobante no dice a quién se le facturó (no trae receptor)"
	}
	for _, c := range cedulas {
		if soloDigitos(c) == rec {
			return true, ""
		}
	}
	return false, "el receptor del comprobante (" + rec + ") no corresponde a esta empresa; " +
		"se esperaba " + strings.Join(cedulas, " o ")
}

// llaveIdempotencia deriva la llave de una recepción del ORIGEN, nunca al azar.
//
// La unidad es el ADJUNTO, no el correo: así un reenvío que agrega una factura nueva entra, y el
// reenvío que no aporta nada se reconoce como repetido. Con la llave puesta solo en el message_id,
// el adjunto nuevo de un reenvío se perdería en silencio.
//
// Cuando el XML no trae clave usable se cae al hash del contenido, que es lo único estable que
// queda: sin eso, un comprobante ilegible reenviado llenaría la cola de copias.
func llaveIdempotencia(messageID, clave string, xml []byte) string {
	msg := strings.TrimSpace(messageID)
	cl := claveNormalizada(clave)
	if cl == "" {
		sum := sha256.Sum256(xml)
		cl = "sha:" + hex.EncodeToString(sum[:])[:32]
	}
	if msg == "" {
		// Sin message_id la llave es solo el comprobante: dos correos distintos con la misma
		// factura se reconocen como el mismo hecho, que es lo correcto.
		return cl
	}
	return msg + "|" + cl
}
