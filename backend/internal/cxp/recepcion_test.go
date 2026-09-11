package cxp

import (
	"strings"
	"testing"
)

// Las cédulas reales del grupo, declaradas por el Director Financiero el 10 de setiembre de 2026
// y sembradas por la migración 0081.
const (
	cedVDP        = "3101318985" // VALLE DE PAZ SERVICIOS FUNERARIOS S.A.
	cedMemorial   = "3101794025" // MEMORIAL PETS DE COSTA RICA S.A.
	cedCoopeprofa = "3004275336" // COOPERATIVA DE SERVICIOS MÚLTIPLES DE PROTECCIÓN FAMILIAR R.L
)

// EL COTEJO DEL RECEPTOR ES EL ÚNICO GUARDARRAÍL contra que una factura entre en la empresa
// equivocada, porque el índice único de la base es (empresa_id, clave) y la misma factura PUEDE
// existir en dos empresas.
//
// Los dos primeros casos son la razón por la que esta función no es un `==`: con el receptor y la
// cédula vacíos, `"" == ""` calzaría y el comprobante entraría sin que nada lo cuestione. Es la
// misma lección de bancos/repository_clasif.go: un alcance vacío tiene que CERRAR, no abrir.
func TestCotejarReceptorCierraCuandoNoPuedeVerificar(t *testing.T) {
	casos := []struct {
		nombre      string
		receptor    string
		cedulas     []string
		quieroCalza bool
		motivoDice  string
	}{
		{
			nombre: "LOS DOS VACÍOS: el caso que un == dejaría pasar",
			// Pasa de verdad: un tiquete electrónico no trae nodo Receptor, y una empresa sin
			// cédulas cargadas escanea a "".
			receptor: "", cedulas: nil,
			quieroCalza: false, motivoDice: "no tiene cédulas",
		},
		{
			nombre:   "empresa sin cédulas, receptor presente: NI SE COMPARA",
			receptor: cedVDP, cedulas: []string{},
			quieroCalza: false, motivoDice: "no tiene cédulas",
		},
		{
			nombre:   "el comprobante no dice a quién va",
			receptor: "", cedulas: []string{cedVDP},
			quieroCalza: false, motivoDice: "no dice a quién",
		},
		{
			nombre:   "calza",
			receptor: cedVDP, cedulas: []string{cedVDP},
			quieroCalza: true,
		},
		{
			nombre:   "calza con guiones, como se escribe a mano",
			receptor: "3-101-318985", cedulas: []string{cedVDP},
			quieroCalza: true,
		},
		{
			nombre:   "calza con espacios de la sangría del XML",
			receptor: "  " + cedVDP + "  ", cedulas: []string{cedVDP},
			quieroCalza: true,
		},
		{
			nombre:   "calza con la segunda cédula de la lista",
			receptor: cedMemorial, cedulas: []string{cedVDP, cedMemorial},
			quieroCalza: true,
		},
		{
			// EL CASO DEL DINERO: la factura es de Coopeprofa y llegó al buzón de Valle de Paz.
			// Si esto calzara, la factura entraría como cuenta por pagar de la empresa equivocada.
			nombre:   "una factura de OTRA empresa del grupo NO calza",
			receptor: cedCoopeprofa, cedulas: []string{cedVDP},
			quieroCalza: false, motivoDice: "no corresponde a esta empresa",
		},
		{
			nombre:   "un tercero desconocido no calza",
			receptor: "3101999999", cedulas: []string{cedVDP},
			quieroCalza: false, motivoDice: "no corresponde a esta empresa",
		},
		{
			nombre:   "una cédula que solo COMPARTE el prefijo no calza",
			receptor: "3101318", cedulas: []string{cedVDP},
			quieroCalza: false, motivoDice: "no corresponde a esta empresa",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			calza, motivo := cotejarReceptor(c.receptor, c.cedulas)
			if calza != c.quieroCalza {
				t.Fatalf("cotejarReceptor(%q, %v) = %v, quería %v — motivo: %s",
					c.receptor, c.cedulas, calza, c.quieroCalza, motivo)
			}
			if !calza {
				if motivo == "" {
					t.Error("cuando no calza SIEMPRE tiene que haber motivo: es lo que la persona lee en la cola de errores")
				}
				if c.motivoDice != "" && !strings.Contains(motivo, c.motivoDice) {
					t.Errorf("el motivo dice %q y tiene que mencionar %q", motivo, c.motivoDice)
				}
			}
		})
	}
}

// El motivo del rechazo NO puede filtrar información de otra empresa: es un texto que va a leer
// alguien que solo tiene acceso a la suya.
func TestCotejarReceptorNoFiltraDatosDeOtraEmpresa(t *testing.T) {
	_, motivo := cotejarReceptor(cedCoopeprofa, []string{cedVDP})
	// Puede decir qué cédula vino y cuál se esperaba (las dos son datos de la propia operación),
	// pero no el nombre ni la razón social de la otra empresa, que no se conocen acá.
	for _, prohibido := range []string{"Coopeprofa", "COOPERATIVA", "Memorial", "Valle de Paz"} {
		if strings.Contains(motivo, prohibido) {
			t.Errorf("el motivo menciona %q: %s", prohibido, motivo)
		}
	}
}

// LA LLAVE DE IDEMPOTENCIA. La unidad es el ADJUNTO, no el correo: el script reenvía correos
// enteros y un reenvío puede traer una factura MÁS.
func TestLlaveIdempotencia(t *testing.T) {
	const xmlA = "<FacturaElectronica><Clave>A</Clave></FacturaElectronica>"
	const xmlB = "<FacturaElectronica><Clave>B</Clave></FacturaElectronica>"

	t.Run("el mismo correo con el mismo comprobante da la misma llave", func(t *testing.T) {
		a := llaveIdempotencia("msg-1", claveAgosto13, []byte(xmlA))
		b := llaveIdempotencia("msg-1", claveAgosto13, []byte(xmlA))
		if a != b {
			t.Errorf("%q != %q: un reenvío se registraría dos veces", a, b)
		}
	})

	t.Run("el mismo correo con OTRO comprobante da otra llave", func(t *testing.T) {
		// Es el caso que se perdería si la llave fuera solo el message_id: el proveedor reenvía el
		// correo agregando una factura, y esa factura nueva tiene que entrar.
		a := llaveIdempotencia("msg-1", claveAgosto13, []byte(xmlA))
		b := llaveIdempotencia("msg-1", claveAgosto05, []byte(xmlB))
		if a == b {
			t.Error("la factura nueva de un reenvío se perdería en silencio")
		}
	})

	t.Run("el mismo comprobante en dos correos distintos da llaves distintas", func(t *testing.T) {
		// A propósito: son dos hechos de recepción distintos y los dos quedan registrados. La
		// factura no se duplica igual, porque de eso se encarga el UNIQUE (empresa_id, clave)
		// del documento y la recepción se resuelve como DUPLICADA.
		a := llaveIdempotencia("msg-1", claveAgosto13, []byte(xmlA))
		b := llaveIdempotencia("msg-2", claveAgosto13, []byte(xmlA))
		if a == b {
			t.Error("se perdería el rastro de que la factura llegó por dos correos")
		}
	})

	t.Run("la clave sucia y la limpia dan la MISMA llave", func(t *testing.T) {
		// El XML viene indentado, así que la clave llega con espacios. Si la llave no normalizara,
		// el mismo comprobante entraría dos veces.
		a := llaveIdempotencia("msg-1", claveAgosto13, []byte(xmlA))
		b := llaveIdempotencia("msg-1", "\n   "+claveAgosto13+"\n  ", []byte(xmlA))
		if a != b {
			t.Errorf("%q != %q: la clave sin recortar duplicaría la recepción", a, b)
		}
	})

	t.Run("sin clave usable se cae al hash del contenido", func(t *testing.T) {
		a := llaveIdempotencia("msg-1", "", []byte(xmlA))
		b := llaveIdempotencia("msg-1", "", []byte(xmlA))
		c := llaveIdempotencia("msg-1", "", []byte(xmlB))
		if a != b {
			t.Error("el mismo XML ilegible reenviado llenaría la cola de copias")
		}
		if a == c {
			t.Error("dos XML distintos sin clave no pueden compartir llave")
		}
		if !strings.Contains(a, "sha:") {
			t.Errorf("la llave por contenido tiene que ser reconocible: %q", a)
		}
	})

	t.Run("nunca queda vacía", func(t *testing.T) {
		// Una llave vacía cae afuera del índice único parcial (que exige <> ''), así que dejaría
		// de deduplicar sin que nada avise.
		for _, k := range []string{
			llaveIdempotencia("", "", []byte("")),
			llaveIdempotencia("", claveAgosto13, nil),
			llaveIdempotencia("msg", "", nil),
		} {
			if strings.TrimSpace(k) == "" {
				t.Error("llave vacía: el índice único parcial no la ataja")
			}
		}
	})
}

// El token: entropía y hash. No se compara el secreto en Go, se busca por el digest.
func TestTokenDeRecepcion(t *testing.T) {
	t1, h1, err := generarTokenRecepcion()
	if err != nil {
		t.Fatalf("generar: %v", err)
	}
	t2, h2, err := generarTokenRecepcion()
	if err != nil {
		t.Fatalf("generar: %v", err)
	}
	if t1 == t2 || h1 == h2 {
		t.Fatal("dos tokens seguidos salieron iguales")
	}
	// 32 bytes en base64 sin relleno = 43 caracteres. La entropía es la ÚNICA defensa: el backend
	// no tiene rate limiting en ninguna ruta.
	if len(t1) != 43 {
		t.Errorf("largo del token = %d, quería 43 (32 bytes de crypto/rand)", len(t1))
	}
	if len(h1) != 64 {
		t.Errorf("largo del hash = %d, quería 64 (sha256 en hex)", len(h1))
	}
	if hashTokenRecepcion(t1) != h1 {
		t.Error("el hash no es reproducible: la verificación por digest no funcionaría")
	}
	if strings.Contains(h1, t1) {
		t.Error("el hash contiene el token en claro")
	}
}
