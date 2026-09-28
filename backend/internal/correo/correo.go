// Package correo resuelve DESDE QUÉ BUZÓN manda cada empresa.
//
// Hasta la migración 0084 el servidor de correo era uno solo para todo el grupo (SMTP_ADDR /
// SMTP_FROM / SMTP_USER / SMTP_PASS) y el mailer se armaba una vez en el arranque, así que
// Coopeprofa le escribía a sus proveedores desde el buzón de Valle de Paz. Acá vive la
// configuración por empresa: leerla, guardarla (con la contraseña cifrada), probarla, y —lo que
// consumen los demás módulos— resolverla en el MOMENTO del envío.
//
// El texto de los correos NO vive acá: eso es `internal/plantillas` y ya era configurable por
// empresa. Este paquete decide el sobre, no la carta.
package correo

import (
	"errors"
	"time"

	"github.com/gpvdp/erp/internal/shared"
)

// ─────────────────────────────────────────────────────────────────────────────
// Centinelas
// ─────────────────────────────────────────────────────────────────────────────
//
// Ninguno lleva jamás la contraseña ni el valor cifrado: el handler los traduce tal cual al
// mensaje que lee el usuario.

var (
	// ErrConfigNoEncontrada: la empresa no tiene fila propia. NO es un error de usuario en el GET
	// —significa «todavía manda por el global»—, pero sí lo es al leerla para guardar.
	ErrConfigNoEncontrada = errors.New("correo: la empresa no tiene configurado su correo saliente")
	// ErrHostRequerido / ErrRemitenteInvalido / ErrPuertoInvalido / ErrSeguridadInvalida: validación
	// de borde. Van como 400.
	ErrHostRequerido     = errors.New("correo: falta el servidor (host) del correo saliente")
	ErrRemitenteInvalido = errors.New("correo: el remitente tiene que ser una dirección de correo válida")
	ErrPuertoInvalido    = errors.New("correo: el puerto tiene que estar entre 1 y 65535")
	ErrSeguridadInvalida = errors.New("correo: la seguridad tiene que ser NINGUNO, STARTTLS o TLS")
	// ErrHostNoPermitido: en producción no se deja apuntar el envío a la red interna del servidor.
	// Ver service.validarHost.
	ErrHostNoPermitido = errors.New("correo: ese servidor apunta a la red interna del servidor; poné el host público de tu proveedor de correo")
	// ErrPasswordRequerida: hay usuario y no hay contraseña (ni guardada ni nueva). Con usuario y
	// sin contraseña el envío intenta AUTH PLAIN con la contraseña vacía, el servidor rechaza, el
	// correo no sale y nadie entiende por qué. Lo ataja también un CHECK de la base.
	ErrPasswordRequerida = errors.New("correo: falta la contraseña del buzón")
	// ErrPasswordRequeridaPorCambio es una DEFENSA DE SEGURIDAD, no una validación de formulario.
	//
	// La contraseña guardada queda atada a (empresa, host, puerto, usuario). Si se cambia el
	// servidor, lo guardado ya no descifra —y, sobre todo, no DEBE descifrar—: sin esta regla,
	// quien pueda editar esta pantalla se lleva la contraseña sin verla nunca, apuntando el host a
	// su propio servidor, dejando el campo de contraseña vacío («ausente = se conserva la
	// guardada») y apretando «probar». (Hallazgo S-1 de la revisión de seguridad del 2026-09-17.)
	ErrPasswordRequeridaPorCambio = errors.New("correo: cambió el servidor; hay que escribir de nuevo la contraseña del buzón")
	// ErrSinCorreoDePrueba: el usuario autenticado no tiene correo, así que la prueba no tiene a
	// dónde ir. La prueba SIEMPRE va al correo de quien la pide — ver Service.Probar.
	ErrSinCorreoDePrueba = errors.New("correo: tu usuario no tiene correo registrado, así que no hay a dónde mandar la prueba")
	// ErrPruebaMuySeguida: freno simple para que el endpoint no sirva de generador de tráfico.
	ErrPruebaMuySeguida = errors.New("correo: esperá unos segundos antes de volver a probar")
)

// EsperaEntrePruebas es el freno del endpoint de prueba, por empresa.
const EsperaEntrePruebas = 30 * time.Second

// ─────────────────────────────────────────────────────────────────────────────
// Lo que se guarda y lo que se muestra
// ─────────────────────────────────────────────────────────────────────────────

// Guardado es la fila tal cual está en `correo_saliente`. Es lo que devuelve el repositorio.
//
// `PasswordCifrada` es el blob, NO la contraseña: acá no hay nada legible. Sale del repositorio
// para que el servicio lo descifre con el empresa_id DEL CONTEXTO de la petición (nunca el que la
// propia fila trae: verificar un dato contra sí mismo no verifica nada — hallazgo S-7).
type Guardado struct {
	Host            string
	Puerto          int
	Seguridad       string
	Usuario         string
	PasswordCifrada string
	Remitente       string
	RemitenteNombre string
	Activo          bool
	ProbadoEn       *time.Time
	ProbadoError    string
	ActualizadoPor  string // nombre del usuario, para mostrar
	ActualizadoEn   time.Time
}

// Input es lo que llega del PUT.
type Input struct {
	Host            string
	Puerto          int
	Seguridad       string
	Usuario         string
	Remitente       string
	RemitenteNombre string
	Activo          bool
	// Password tiene TRES estados, y por eso es puntero: el GET no puede devolver la contraseña,
	// así que el formulario no la puede round-tripear y «vacío» tiene que poder significar dos
	// cosas distintas.
	//
	//   nil        → conservar la guardada
	//   apunta a "" → borrarla (queda sin autenticación)
	//   con valor   → cifrarla y reemplazar
	Password *shared.Secreto
}

// Config es lo que se le muestra a la pantalla. NO TIENE, NI VA A TENER, UN CAMPO CON LA
// CONTRASEÑA: ni vacío ni enmascarado. Lo único que se dice de ella es si existe.
type Config struct {
	Configurado     bool   `json:"configurado"`
	Host            string `json:"host"`
	Puerto          int    `json:"puerto"`
	Seguridad       string `json:"seguridad"`
	Usuario         string `json:"usuario"`
	TienePassword   bool   `json:"tiene_password"`
	Remitente       string `json:"remitente"`
	RemitenteNombre string `json:"remitente_nombre"`
	Activo          bool   `json:"activo"`
	// ProbadoEn / ProbadoError: resultado de la última prueba. `probado_en` con `probado_error`
	// nulo = la última prueba salió bien. Los dos nulos = nunca se probó, que no es lo mismo.
	//
	// Acá, y solo acá, va el DETALLE TÉCNICO del fallo (categoría, código SMTP y servidor). Esta
	// respuesta se lee con admin.correo, que hoy tiene un rol; la bitácora del comprobante la leen
	// siete. (Hallazgo S-3.)
	ProbadoEn      *time.Time `json:"probado_en"`
	ProbadoError   *string    `json:"probado_error"`
	ActualizadoPor string     `json:"actualizado_por"`
	ActualizadoEn  *time.Time `json:"actualizado_en"`
	// OrigenVigente dice desde dónde sale HOY el correo de esta empresa: EMPRESA (esta fila),
	// GLOBAL (las variables del servidor) o NINGUNO. Sin esto la pantalla diría «sin configurar»
	// mientras los correos sí están saliendo, que es mentir con buena intención.
	OrigenVigente    string `json:"origen_vigente"`
	RemitenteVigente string `json:"remitente_vigente"`
	// CifradoDisponible: si el servidor tiene CIFRADO_SECRET. En false, guardar una contraseña va a
	// responder 422 y la pantalla puede decirlo ANTES de que el usuario la escriba.
	CifradoDisponible bool `json:"cifrado_disponible"`
}

// Prueba es el resultado de probar la conexión.
type Prueba struct {
	OK          bool   `json:"ok"`
	Servidor    string `json:"servidor"`
	Remitente   string `json:"remitente"`
	Origen      string `json:"origen"`
	EnviadoA    string `json:"enviado_a"`
	ProbadoEn   string `json:"probado_en"`
	Descripcion string `json:"descripcion"`
}

// Orígenes posibles de la configuración con la que se manda.
const (
	// OrigenEmpresa: la fila de `correo_saliente` de esa empresa.
	OrigenEmpresa = "EMPRESA"
	// OrigenGlobal: las variables SMTP_* del proceso.
	OrigenGlobal = "GLOBAL"
	// OrigenNinguno: no hay ni lo uno ni lo otro. Solo se usa para informar en el GET; la bitácora
	// del comprobante no lo acepta (su CHECK es EMPRESA|GLOBAL).
	OrigenNinguno = "NINGUNO"
)
