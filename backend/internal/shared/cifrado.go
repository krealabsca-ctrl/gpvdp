package shared

// Cifrado de los secretos que el sistema tiene que volver a USAR.
//
// Hoy hay UNO: la contraseña del correo saliente de cada empresa (mig 0084). El sistema se la
// tiene que presentar al servidor SMTP en cada envío, así que no sirve el precedente que ya
// existía en el proyecto —`cxp_fuente_recepcion.token_hash` es sha256, irreversible: sirve para
// COMPARAR lo que llega, no para volver a usarlo—.
//
// La clave vive FUERA de la base, en la variable de entorno CIFRADO_SECRET. Esa es toda la idea:
// un respaldo de Postgres que alguien se lleve trae texto inútil.
//
// ⚠ SI CIFRADO_SECRET SE PIERDE, LO GUARDADO ES IRRECUPERABLE. No hay puerta de atrás —ese es el
// punto—. La recuperación es escribir de nuevo la contraseña de cada empresa en la pantalla de
// Correo saliente. El aviso está también en el `.env` que genera deploy/instalar-vps.sh y en
// deploy/LEEME-VPS.md, que es donde lo va a leer quien instala.

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// El secreto en memoria
// ─────────────────────────────────────────────────────────────────────────────

// Secreto es una contraseña en claro que NO se puede imprimir por accidente.
//
// Es un tipo y no un `string` a propósito. Una contraseña de correo pasa por un binding de Gin,
// por un struct de auditoría que termina en `json.Marshal` (shared/audit.go → tabla INMUTABLE),
// por un `zap.Any` y por cualquier `%v` que alguien agregue el año que viene para depurar. Con un
// `string`, cada uno de esos caminos la escribe en claro y hay que acordarse de taparlos de a uno.
// Con este tipo, `%v`, `%s`, `%q`, `%#v`, `json.Marshal` y el codificador de zap imprimen `***`
// POR CONSTRUCCIÓN, y el único camino al valor real es `Revelar()`, que se puede buscar con grep.
//
// Ojo con lo que NO hace: `string(s)` y `s.Revelar()` sí devuelven el valor. La protección es
// contra el descuido, no contra quien la quiera sacar a propósito.
//
// No define `UnmarshalJSON` a propósito: la asimetría es el diseño. El JSON del request ENTRA
// normal (es un tipo de base string) y nunca SALE.
type Secreto string

// secretoOculto es lo único que este tipo deja ver.
const secretoOculto = "***"

// String tapa el valor en `%v`, `%s` y `%q`, y en cualquier log que formatee la struct.
func (Secreto) String() string { return secretoOculto }

// GoString tapa el valor en `%#v`, que no pasa por String.
func (Secreto) GoString() string { return `shared.Secreto("` + secretoOculto + `")` }

// MarshalJSON tapa el valor en `json.Marshal` y en el codificador de zap.
//
// Es lo que hace que pasar la struct entera del request como `ValorNuevo` de la auditoría sea
// seguro: `auditoria_evento` es append-only y una contraseña escrita ahí no se puede ni borrar.
func (Secreto) MarshalJSON() ([]byte, error) { return []byte(`"` + secretoOculto + `"`), nil }

// MarshalText tapa el valor en los codificadores que prefieren texto a JSON.
func (Secreto) MarshalText() ([]byte, error) { return []byte(secretoOculto), nil }

// Revelar devuelve el valor real. Es el ÚNICO camino, y existe para poder buscarlo con grep:
// toda línea del proyecto que exponga una contraseña dice `Revelar()`.
func (s Secreto) Revelar() string { return string(s) }

// Vacio dice si no hay secreto (sin revelarlo).
func (s Secreto) Vacio() bool { return s == "" }

// ─────────────────────────────────────────────────────────────────────────────
// Centinelas
// ─────────────────────────────────────────────────────────────────────────────
//
// Ninguno lleva jamás el valor cifrado ni el valor en claro en el mensaje: los traduce el handler
// y terminan en la pantalla del usuario.

var (
	// ErrClaveAusente: no hay CIFRADO_SECRET. Se nombra la variable a propósito: es lo único que
	// le sirve a quien tiene que arreglarlo.
	ErrClaveAusente = errors.New("shared: falta la variable de entorno CIFRADO_SECRET; sin ella no se puede guardar la contraseña del correo")
	// ErrClaveCorta: la clave mide menos de LargoMinimoClave.
	ErrClaveCorta = errors.New("shared: CIFRADO_SECRET tiene que medir al menos 32 caracteres")
	// ErrClaveDebil: mide lo suficiente pero se repite demasiado ("aaaa…", "12121212…", una
	// palabra pegada cuatro veces). El largo solo no dice nada: SHA-256 es de una pasada y barato
	// de atacar por fuerza bruta contra un respaldo robado si la clave es adivinable. La clave la
	// genera el instalador al azar; esto ataja al operador que la escribe a mano.
	ErrClaveDebil = errors.New("shared: CIFRADO_SECRET se repite demasiado; usá el valor al azar que genera el instalador, no una frase")
	// ErrContextoVacio: se intentó cifrar sin atar el secreto a nada. Ver Cifrar.
	ErrContextoVacio = errors.New("shared: hay que cifrar atando el secreto a su contexto (al menos la empresa)")
	// ErrFormato: lo guardado no tiene la marca de versión.
	ErrFormato = errors.New("shared: el secreto guardado no tiene el formato esperado")
	// ErrNoDescifra: el tag no cuadra. Clave cambiada, dato manipulado, o el contexto no es el
	// mismo con el que se cifró (otra empresa, otro servidor de correo).
	ErrNoDescifra = errors.New("shared: el secreto guardado no se puede descifrar con la clave actual (¿cambió CIFRADO_SECRET, o el servidor de correo?)")
)

const (
	// LargoMinimoClave es el piso de largo de CIFRADO_SECRET.
	LargoMinimoClave = 32
	// minDistintosClave es el piso de caracteres DISTINTOS. Ver ErrClaveDebil.
	minDistintosClave = 12

	// prefijoV1 va ADENTRO del valor guardado para poder cambiar de algoritmo más adelante
	// distinguiendo lo viejo de lo nuevo sin adivinar. Es además lo que chequea la base
	// (`CHECK (password_cifrada = '' OR password_cifrada LIKE 'v1.%')`, mig 0084): un INSERT con
	// la contraseña en claro rebota.
	prefijoV1 = "v1."

	// Etiquetas de dominio de la derivación. Cambiarlas invalida todo lo cifrado: son parte del
	// formato, no un comentario.
	saltClave = "gpvdp-cifrado-salt-v1"
	infoClave = "gpvdp/secretos-reversibles/aes-256-gcm/v1"
	// versionAAD entra a los datos autenticados para que un blob de "v1" no se pueda replantar
	// bajo otra versión del formato el día que exista.
	versionAAD = "gpvdp-aad-v1"
)

// ─────────────────────────────────────────────────────────────────────────────
// El cifrador
// ─────────────────────────────────────────────────────────────────────────────

// Cifrador cifra y descifra con AES-256-GCM.
//
// Por qué GCM y no un cifrado a secas: GCM es AEAD, o sea que lleva un tag de autenticación de 16
// bytes. Si alguien edita un byte del texto cifrado en la base, el descifrado FALLA en vez de
// devolver basura —y esa basura, acá, sería una contraseña que el servidor le manda a un servidor
// SMTP ajeno—. Y el AAD (ver Cifrar) es lo que ata el secreto a las condiciones bajo las que se
// puede volver a usar.
type Cifrador struct{ aead cipher.AEAD }

// NewCifrador construye el cifrador desde el valor de CIFRADO_SECRET.
//
// NUNCA hace panic y NUNCA es fatal para el arranque: hay instalaciones corriendo sin esta
// variable, y tumbar el proceso convertiría una mejora del correo en una caída del ERP entero. El
// fallo se cobra donde se puede leer —al guardar una contraseña y al enviar—, no al arrancar.
func NewCifrador(secreto string) (*Cifrador, error) {
	secreto = strings.TrimSpace(secreto)
	if secreto == "" {
		return nil, ErrClaveAusente
	}
	if len([]rune(secreto)) < LargoMinimoClave {
		return nil, ErrClaveCorta
	}
	if claveDebil(secreto) {
		return nil, ErrClaveDebil
	}
	// HKDF y no un sha256 de una pasada: la variable puede traer cualquier formato (el instalador
	// escribe 64 caracteres al azar, pero nadie garantiza que mañana no sea otra cosa) y HKDF es
	// la función pensada justamente para convertir eso en una clave de 32 bytes.
	clave, err := hkdf.Key(sha256.New, []byte(secreto), []byte(saltClave), infoClave, 32)
	if err != nil {
		return nil, err
	}
	bloque, err := aes.NewCipher(clave)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(bloque)
	if err != nil {
		return nil, err
	}
	return &Cifrador{aead: aead}, nil
}

// Configurado dice si se puede cifrar (mismo patrón que Mailer.Configurado): sirve para responder
// «falta configurar X» en vez de un error interno.
func (c *Cifrador) Configurado() bool { return c != nil && c.aead != nil }

// Cifrar devuelve "v1.<base64url(nonce‖ciphertext‖tag)>".
//
// EL CONTEXTO NO ES OPCIONAL Y NO ES DECORATIVO. Son los datos autenticados (AAD): quedan ligados
// al valor cifrado sin viajar adentro de él, y Descifrar solo funciona si le pasan exactamente los
// mismos, en el mismo orden. Es un control de seguridad, no una validación de formulario:
//
//   - El primer elemento SIEMPRE es el empresa_id, y SIEMPRE el del contexto de la petición (el
//     que el token acuñó contra la membresía), nunca el que el repositorio leyó de la propia fila:
//     verificar un dato contra sí mismo no verifica nada. Con esto, una fila de correo_saliente
//     copiada de una empresa a otra NO descifra.
//   - Para la contraseña del correo van además host, puerto y usuario. Sin eso, quien pueda editar
//     la configuración se lleva la contraseña sin verla nunca: cambia el host al suyo dejando el
//     campo de contraseña vacío —«ausente = se conserva la guardada»—, aprieta «probar», y el
//     servidor le presenta la contraseña real a su propio SMTP. Atado al host, cambiar el servidor
//     vuelve el blob indescifrable: no hay nada que entregar. (Hallazgo S-1 de la revisión de
//     seguridad del 2026-09-17.)
//
// Dos cifrados del mismo valor dan resultados distintos: el nonce son 12 bytes nuevos de
// crypto/rand en cada llamada. Así dos empresas con la misma contraseña no se delatan.
func (c *Cifrador) Cifrar(claro Secreto, contexto ...string) (string, error) {
	if !c.Configurado() {
		// Con el cifrador sin configurar se devuelve error, NUNCA el texto en claro: es lo que
		// impide que una instalación sin CIFRADO_SECRET guarde la contraseña legible.
		return "", ErrClaveAusente
	}
	if len(contexto) == 0 {
		return "", ErrContextoVacio
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sellado := c.aead.Seal(nil, nonce, []byte(claro), datosAutenticados(contexto))
	return prefijoV1 + base64.RawURLEncoding.EncodeToString(append(nonce, sellado...)), nil
}

// Descifrar es la inversa de Cifrar. El contexto tiene que ser EL MISMO, en el mismo orden.
//
// Devuelve ErrFormato si el valor no trae la marca de versión, y ErrNoDescifra si el tag no cuadra
// —clave rotada, dato manipulado, u otra empresa/otro servidor—. Nunca devuelve un valor a medias:
// GCM verifica antes de entregar nada.
func (c *Cifrador) Descifrar(guardado string, contexto ...string) (Secreto, error) {
	if !c.Configurado() {
		return "", ErrClaveAusente
	}
	if len(contexto) == 0 {
		return "", ErrContextoVacio
	}
	if !strings.HasPrefix(guardado, prefijoV1) {
		return "", ErrFormato
	}
	crudo, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(guardado, prefijoV1))
	if err != nil {
		return "", ErrFormato
	}
	n := c.aead.NonceSize()
	if len(crudo) < n+c.aead.Overhead() {
		return "", ErrFormato
	}
	claro, err := c.aead.Open(nil, crudo[:n], crudo[n:], datosAutenticados(contexto))
	if err != nil {
		// El error de GCM no se envuelve: no dice nada útil y lo que sí tiene valor —qué contexto
		// se probó— es justamente lo que no queremos repetirle a nadie.
		return "", ErrNoDescifra
	}
	return Secreto(claro), nil
}

// datosAutenticados arma el AAD con el largo de cada parte por delante.
//
// El largo no es paja: concatenar a secas haría que ("ab","c") y ("a","bc") produjeran el mismo
// AAD, y entonces un host cuidadosamente elegido podría hacerse pasar por otra combinación válida.
func datosAutenticados(contexto []string) []byte {
	var b bytes.Buffer
	b.WriteString(versionAAD)
	var largo [8]byte
	for _, parte := range contexto {
		binary.BigEndian.PutUint64(largo[:], uint64(len(parte)))
		b.Write(largo[:])
		b.WriteString(parte)
	}
	return b.Bytes()
}

// claveDebil mide si la clave se repite demasiado. Ver ErrClaveDebil.
func claveDebil(secreto string) bool {
	distintos := make(map[rune]struct{}, minDistintosClave)
	for _, r := range secreto {
		distintos[r] = struct{}{}
		if len(distintos) >= minDistintosClave {
			return false
		}
	}
	return true
}
