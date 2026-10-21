package cxp

import (
	"strings"
	"testing"
)

// El 25 de setiembre de 2026 el Director Financiero marcó tres facturas, dio «Generar lote + macro»
// y la pantalla le contestó:
//
//	no se creó el lote: ninguna de las 3 facturas seleccionadas se puede cortar
//
// Cierto, y completamente inútil: el servidor SABÍA que las tres estaban bloqueadas para pago —lo
// mandaba en `fuera`— pero el mensaje no lo decía, así que no había forma de saber qué hacer.
// Frenar el corte sin decir por qué es la mitad del trabajo.
//
// Estas pruebas fijan la otra mitad: el mensaje enumera los motivos con su conteo.
func TestElMensajeDelLoteVacioDiceLosMotivos(t *testing.T) {
	const bloqueo = "está bloqueada para pago: los viáticos no se pagan por banco"
	const enRevision = "está en REVISADO: solo se puede cortar lo aprobado o lo ya programado"

	casos := []struct {
		nombre   string
		fuera    []DocumentoFueraDelLote
		contiene []string
		ausente  []string
	}{
		{
			// El caso exacto de la captura: tres facturas, un solo motivo.
			nombre: "tres con el mismo motivo lo dicen una vez, con el conteo",
			fuera: []DocumentoFueraDelLote{
				{Proveedor: "Marcela Fernanda Quiros Vargas", Motivo: bloqueo},
				{Proveedor: "AR SISTEMAS SOCIEDAD ANONIMA", Motivo: bloqueo},
				{Proveedor: "Electritel Sociedad Anonima", Motivo: bloqueo},
			},
			contiene: []string{"ninguna de las 3", bloqueo, "(3)"},
		},
		{
			nombre: "motivos distintos: el más frecuente primero",
			fuera: []DocumentoFueraDelLote{
				{Motivo: bloqueo},
				{Motivo: enRevision},
				{Motivo: enRevision},
			},
			contiene: []string{enRevision + " (2)", bloqueo + " (1)"},
		},
		{
			// Una sola factura no necesita conteo: el motivo va tal cual, en singular.
			nombre:   "una sola trae su motivo sin conteo",
			fuera:    []DocumentoFueraDelLote{{Motivo: bloqueo}},
			contiene: []string{"la única factura", bloqueo},
			ausente:  []string{"(1)"},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			msg := (&LoteVacioError{Fuera: c.fuera}).Error()
			for _, quiero := range c.contiene {
				if !strings.Contains(msg, quiero) {
					t.Errorf("el mensaje no dice %q.\nMensaje: %s", quiero, msg)
				}
			}
			for _, noQuiero := range c.ausente {
				if strings.Contains(msg, noQuiero) {
					t.Errorf("el mensaje no debería decir %q.\nMensaje: %s", noQuiero, msg)
				}
			}
		})
	}
}

// El orden de los motivos más frecuentes primero tiene que ser ESTABLE: un mensaje que cambia de
// redacción entre dos corridas iguales es un mensaje que nadie puede citar en un correo ni buscar
// en el historial. Con `sort.Slice` (no estable) este test se pone rojo de a ratos, que es la
// forma más cara de descubrir un bug.
func TestElOrdenDeLosMotivosEsEstable(t *testing.T) {
	fuera := []DocumentoFueraDelLote{
		{Motivo: "ya está en el lote #14"},
		{Motivo: "no existe en esta empresa"},
		{Motivo: "está en REVISADO"},
	}
	primero := (&LoteVacioError{Fuera: fuera}).Error()
	for i := 0; i < 20; i++ {
		if otro := (&LoteVacioError{Fuera: fuera}).Error(); otro != primero {
			t.Fatalf("el mismo error dio dos mensajes distintos:\n%s\n%s", primero, otro)
		}
	}
}

// Con muchos lotes o muchos estados en juego los motivos se multiplican (cada lote y cada estado
// escriben uno distinto). El mensaje corta, pero DICE que cortó: un «y 3 motivo(s) más» avisa que
// hay que mirar la lista; una lista truncada en silencio se lee como completa.
func TestLosMotivosSeCortanPeroLoDicen(t *testing.T) {
	fuera := []DocumentoFueraDelLote{
		{Motivo: "ya está en el lote #10"},
		{Motivo: "ya está en el lote #11"},
		{Motivo: "ya está en el lote #12"},
		{Motivo: "ya está en el lote #13"},
		{Motivo: "ya está en el lote #14"},
		{Motivo: "ya está en el lote #15"},
		{Motivo: "ya está en el lote #16"},
	}
	msg := (&LoteVacioError{Fuera: fuera}).Error()

	if strings.Count(msg, "ya está en el lote") != topeMotivos {
		t.Errorf("esperaba %d motivos enumerados, el mensaje tiene otra cosa:\n%s", topeMotivos, msg)
	}
	if faltan := len(fuera) - topeMotivos; !strings.Contains(msg, "y 3 motivo(s) más") {
		t.Errorf("el mensaje no avisa que quedaron %d motivos sin mostrar:\n%s", faltan, msg)
	}
}
