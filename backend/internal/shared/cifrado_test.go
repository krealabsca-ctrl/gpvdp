package shared

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// claveDePrueba imita lo que genera `secreto 64 64` en deploy/instalar-vps.sh: 64 caracteres al
// azar del alfabeto base64 sin los símbolos.
const claveDePrueba = "Kf7QmZ2xTb9LrWvA4nJcHs1YpEdGu0Ri6XoNB3kMaVzQwT5yPlCeSgUhIjDfOb8"

const (
	empresaVDP  = "427f8851-1e94-4c13-abbd-3f7bb9f456e9"
	empresaCoop = "f151fabc-f575-4871-817a-8f7bb193d04b"
)

func nuevoCifrador(t *testing.T) *Cifrador {
	t.Helper()
	c, err := NewCifrador(claveDePrueba)
	if err != nil {
		t.Fatalf("NewCifrador: %v", err)
	}
	return c
}

func TestCifrarDescifrarIdaYVuelta(t *testing.T) {
	c := nuevoCifrador(t)
	original := Secreto("cl4ve-del-buzón-de-CxP · ñ")

	guardado, err := c.Cifrar(original, empresaVDP, "smtp.office365.com", "587", "cxp@valledepazcr.com")
	if err != nil {
		t.Fatalf("Cifrar: %v", err)
	}
	if !strings.HasPrefix(guardado, "v1.") {
		t.Fatalf("lo guardado tiene que empezar con la marca de versión (el CHECK de la base lo exige): %q", guardado)
	}
	if strings.Contains(guardado, original.Revelar()) {
		t.Fatal("el texto en claro aparece dentro de lo guardado")
	}

	vuelto, err := c.Descifrar(guardado, empresaVDP, "smtp.office365.com", "587", "cxp@valledepazcr.com")
	if err != nil {
		t.Fatalf("Descifrar: %v", err)
	}
	if vuelto != original {
		t.Fatalf("no volvió el mismo valor")
	}
}

func TestCifrarDosVecesDaValoresDistintos(t *testing.T) {
	c := nuevoCifrador(t)
	uno, err := c.Cifrar("misma-contraseña", empresaVDP)
	if err != nil {
		t.Fatalf("Cifrar: %v", err)
	}
	dos, err := c.Cifrar("misma-contraseña", empresaVDP)
	if err != nil {
		t.Fatalf("Cifrar: %v", err)
	}
	if uno == dos {
		t.Fatal("dos cifrados del mismo valor dieron lo mismo: el nonce no está cambiando, y así dos empresas con la misma contraseña se delatan")
	}
	for _, g := range []string{uno, dos} {
		if v, err := c.Descifrar(g, empresaVDP); err != nil || v != "misma-contraseña" {
			t.Fatalf("los dos tienen que descifrar: %v / %q", err, v)
		}
	}
}

func TestDescifrarDetectaManipulacion(t *testing.T) {
	c := nuevoCifrador(t)
	guardado, err := c.Cifrar("secreto", empresaVDP)
	if err != nil {
		t.Fatalf("Cifrar: %v", err)
	}

	// Un byte cambiado en el texto cifrado (no en el prefijo): el tag de GCM tiene que rebotarlo.
	crudo, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(guardado, "v1."))
	if err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	crudo[len(crudo)-1] ^= 0x01
	tocado := "v1." + base64.RawURLEncoding.EncodeToString(crudo)

	v, err := c.Descifrar(tocado, empresaVDP)
	if !errors.Is(err, ErrNoDescifra) {
		t.Fatalf("un dato manipulado tiene que dar ErrNoDescifra, dio %v", err)
	}
	if v != "" {
		t.Fatal("no puede devolver nada cuando el tag no cuadra")
	}
}

func TestDescifrarNoCruzaEmpresas(t *testing.T) {
	c := nuevoCifrador(t)
	guardado, err := c.Cifrar("clave-de-coopeprofa", empresaCoop)
	if err != nil {
		t.Fatalf("Cifrar: %v", err)
	}
	// La fila de una empresa pegada en la de otra NO descifra: el empresa_id del CONTEXTO de la
	// petición es parte de los datos autenticados.
	if _, err := c.Descifrar(guardado, empresaVDP); !errors.Is(err, ErrNoDescifra) {
		t.Fatalf("el secreto de una empresa no puede descifrar bajo otra; dio %v", err)
	}
}

// El control del hallazgo S-1: la contraseña queda atada al servidor para el que se guardó.
//
// Sin esto, quien puede editar la configuración se lleva la contraseña sin verla nunca: cambia el
// host al suyo sin tocar el campo de contraseña (ausente = se conserva la guardada), aprieta
// «probar», y el servidor le presenta la contraseña real a su propio SMTP.
func TestDescifrarNoSirveConOtroServidor(t *testing.T) {
	c := nuevoCifrador(t)
	const (
		host    = "smtp.office365.com"
		puerto  = "587"
		usuario = "cxp@valledepazcr.com"
	)
	guardado, err := c.Cifrar("clave-del-buzón", empresaVDP, host, puerto, usuario)
	if err != nil {
		t.Fatalf("Cifrar: %v", err)
	}

	casos := []struct {
		nombre              string
		host, puerto, buzon string
	}{
		{"host del atacante", "smtp.atacante.com", puerto, usuario},
		{"otro puerto", host, "465", usuario},
		{"otro usuario", host, puerto, "otro@valledepazcr.com"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			v, err := c.Descifrar(guardado, empresaVDP, caso.host, caso.puerto, caso.buzon)
			if !errors.Is(err, ErrNoDescifra) {
				t.Fatalf("cambiar el servidor tiene que volver el secreto indescifrable; dio %v", err)
			}
			if v != "" {
				t.Fatal("no puede entregar la contraseña para un servidor distinto del que se guardó")
			}
		})
	}
}

// El largo de cada parte va en el AAD, así que ("ab","c") y ("a","bc") no son lo mismo.
func TestContextoNoEsAmbiguo(t *testing.T) {
	c := nuevoCifrador(t)
	guardado, err := c.Cifrar("secreto", "ab", "c")
	if err != nil {
		t.Fatalf("Cifrar: %v", err)
	}
	if _, err := c.Descifrar(guardado, "a", "bc"); !errors.Is(err, ErrNoDescifra) {
		t.Fatalf("partir el contexto distinto no puede descifrar; dio %v", err)
	}
}

func TestCifrarExigeContexto(t *testing.T) {
	c := nuevoCifrador(t)
	if _, err := c.Cifrar("secreto"); !errors.Is(err, ErrContextoVacio) {
		t.Fatalf("cifrar sin contexto tiene que fallar; dio %v", err)
	}
	if _, err := c.Descifrar("v1.loquesea"); !errors.Is(err, ErrContextoVacio) {
		t.Fatalf("descifrar sin contexto tiene que fallar; dio %v", err)
	}
}

func TestSinClaveNoSeGuardaEnClaro(t *testing.T) {
	casos := []struct {
		nombre    string
		valor     string
		centinela error
	}{
		{"ausente", "", ErrClaveAusente},
		{"solo espacios", "        ", ErrClaveAusente},
		{"corta", "apenas-31-caracteres-aqui-1234", ErrClaveCorta},
		{"larga pero repetida", strings.Repeat("ab", 40), ErrClaveDebil},
		{"larga pero una sola letra", strings.Repeat("x", 64), ErrClaveDebil},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			c, err := NewCifrador(caso.valor)
			if !errors.Is(err, caso.centinela) {
				t.Fatalf("se esperaba %v, dio %v", caso.centinela, err)
			}
			if c.Configurado() {
				t.Fatal("un cifrador que no se pudo construir no puede decir que está configurado")
			}
			// Lo que importa de verdad: con el cifrador inservible, cifrar NO devuelve el texto
			// en claro para que alguien lo guarde igual.
			guardado, err := c.Cifrar("contraseña-real", empresaVDP)
			if !errors.Is(err, ErrClaveAusente) {
				t.Fatalf("cifrar sin clave tiene que dar ErrClaveAusente, dio %v", err)
			}
			if guardado != "" {
				t.Fatalf("cifrar sin clave devolvió algo (%q): jamás puede devolver el valor para guardarlo", guardado)
			}
		})
	}
}

func TestMensajeDeClaveAusenteNombraLaVariable(t *testing.T) {
	// Es 422 con el nombre de la variable, no un 500: quien lo lee tiene que saber qué poner.
	if !strings.Contains(ErrClaveAusente.Error(), "CIFRADO_SECRET") {
		t.Fatal("el error tiene que nombrar CIFRADO_SECRET")
	}
	if !strings.Contains(ErrClaveCorta.Error(), "CIFRADO_SECRET") {
		t.Fatal("el error tiene que nombrar CIFRADO_SECRET")
	}
}

func TestDescifrarRechazaFormatoDesconocido(t *testing.T) {
	c := nuevoCifrador(t)
	for _, malo := range []string{
		"",
		"contraseña-en-claro",
		"v2.AAAA",
		"v1.no-es-base64-válido-ñ",
		"v1." + base64.RawURLEncoding.EncodeToString([]byte("corto")),
	} {
		if _, err := c.Descifrar(malo, empresaVDP); !errors.Is(err, ErrFormato) {
			t.Fatalf("%q tenía que dar ErrFormato, dio %v", malo, err)
		}
	}
}

func TestClaveRotadaNoDescifra(t *testing.T) {
	viejo := nuevoCifrador(t)
	guardado, err := viejo.Cifrar("clave-del-buzón", empresaVDP)
	if err != nil {
		t.Fatalf("Cifrar: %v", err)
	}
	nuevo, err := NewCifrador("OtRa9ClAvE7dIsTiNtA2qUe6NaDiE4aDiViNa8XyZwVuTsRqPoNmLkJhGfDcBa")
	if err != nil {
		t.Fatalf("NewCifrador: %v", err)
	}
	// Rotar CIFRADO_SECRET sin re-cifrar deja lo guardado ilegible. No hay corrupción ni envío con
	// contraseña vacía: se responde 422 y hay que volver a escribirla.
	if _, err := nuevo.Descifrar(guardado, empresaVDP); !errors.Is(err, ErrNoDescifra) {
		t.Fatalf("con otra clave tiene que dar ErrNoDescifra, dio %v", err)
	}
}

func TestCifradorNilNoRevienta(t *testing.T) {
	var c *Cifrador
	if c.Configurado() {
		t.Fatal("un cifrador nil no está configurado")
	}
	if _, err := c.Cifrar("x", empresaVDP); !errors.Is(err, ErrClaveAusente) {
		t.Fatalf("dio %v", err)
	}
	if _, err := c.Descifrar("v1.x", empresaVDP); !errors.Is(err, ErrClaveAusente) {
		t.Fatalf("dio %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// El tipo Secreto: que no se imprima por accidente
// ─────────────────────────────────────────────────────────────────────────────

func TestSecretoNoSeImprime(t *testing.T) {
	const valor = "L4-contraseña-del-buzón"
	s := Secreto(valor)

	for _, formato := range []string{"%v", "%s", "%q", "%#v", "%+v"} {
		salida := fmt.Sprintf(formato, s)
		if strings.Contains(salida, valor) {
			t.Fatalf("%s filtró la contraseña: %s", formato, salida)
		}
		if !strings.Contains(salida, "***") {
			t.Fatalf("%s tenía que mostrar ***, mostró %s", formato, salida)
		}
	}
	if s.Revelar() != valor {
		t.Fatal("Revelar tiene que devolver el valor real")
	}
	if s.Vacio() {
		t.Fatal("no está vacío")
	}
	if !Secreto("").Vacio() {
		t.Fatal("el vacío sí está vacío")
	}
}

// La auditoría hace json.Marshal del struct que le pasen y lo escribe en `auditoria_evento`, que
// es append-only: una contraseña ahí no se puede ni borrar. Con este tipo, pasar la struct entera
// del request es seguro.
func TestSecretoNoLlegaAlJSONNiALaAuditoria(t *testing.T) {
	const valor = "L4-contraseña-del-buzón"
	entrada := struct {
		Host     string  `json:"host"`
		Usuario  string  `json:"usuario"`
		Password Secreto `json:"password"`
	}{Host: "smtp.office365.com", Usuario: "cxp@valledepazcr.com", Password: Secreto(valor)}

	b, err := json.Marshal(entrada)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), valor) {
		t.Fatalf("la contraseña salió en el JSON: %s", b)
	}
	if !strings.Contains(string(b), `"password":"***"`) {
		t.Fatalf("se esperaba la contraseña tapada: %s", b)
	}

	// Y la asimetría: el JSON del request ENTRA normal (el binding del PUT lo necesita).
	var vuelta struct {
		Password Secreto `json:"password"`
	}
	if err := json.Unmarshal([]byte(`{"password":"`+valor+`"}`), &vuelta); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if vuelta.Password.Revelar() != valor {
		t.Fatalf("el binding tiene que recibir el valor real, recibió %q", vuelta.Password.Revelar())
	}
}
