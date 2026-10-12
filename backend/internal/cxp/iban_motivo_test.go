package cxp

// «Ya tiene cuenta IBAN» — 1 de octubre de 2026.
//
// El lote #30 no dejó bajar la macro con el mensaje «Hay pagos sin cuenta IBAN», y el proveedor SÍ
// tenía una cargada: `cr123456789874563210236`, con 21 dígitos en vez de 20. El guardarraíl estaba
// en lo correcto —el banco la habría rechazado— pero el mensaje decía que FALTABA, así que mandaba
// a mirar una ficha donde el dato está, y ahí se acababa la pista.
//
// La diferencia no es de redacción: «falta» se arregla pidiéndosela al proveedor y «está mal
// escrita» se arregla en la ficha en treinta segundos.

import (
	"strings"
	"testing"
)

func TestElMotivoDistingueFaltarDeEstarMalEscrito(t *testing.T) {
	casos := []struct {
		nombre, iban string
		contiene     []string
		valido       bool
	}{
		{
			nombre: "el caso real: un dígito de más",
			iban:   "cr123456789874563210236",
			// Dice qué tiene, cuánto mide y cuánto debería medir: con eso se corrige sin preguntar.
			contiene: []string{"23 caracteres", "22", "CR + 20 dígitos", "CR123456789874563210236"},
		},
		{
			nombre:   "un dígito de menos",
			iban:     "CR1234567896587412302",
			contiene: []string{"21 caracteres", "22"},
		},
		{
			nombre:   "vacío: eso sí es que falta",
			iban:     "",
			contiene: []string{"no tiene IBAN registrado"},
		},
		{
			nombre:   "no empieza con CR",
			iban:     "US12345678965874123026",
			contiene: []string{"no empieza con CR"},
		},
		{
			nombre:   "una letra donde van los dígitos",
			iban:     "CR1234567896587412302X",
			contiene: []string{"no es un dígito"},
		},
		{
			// El del otro proveedor del mismo lote, que SÍ estaba bien y no tenía por qué aparecer
			// en la lista de problemas.
			nombre: "un IBAN bien formado no tiene motivo",
			iban:   "CR12345678965874123026",
			valido: true,
		},
		{
			// Como lo pega la gente desde el banco. La normalización ya existía; esto la fija.
			nombre: "con espacios sigue siendo válido",
			iban:   "CR 1234 5678 9658 7412 3026",
			valido: true,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			motivo := MotivoIBANInvalido(c.iban)
			if c.valido {
				if motivo != "" {
					t.Fatalf("un IBAN válido no debería tener motivo, dio %q", motivo)
				}
				return
			}
			if motivo == "" {
				t.Fatalf("no explicó por qué %q no sirve", c.iban)
			}
			for _, q := range c.contiene {
				if !strings.Contains(motivo, q) {
					t.Errorf("el motivo no dice %q.\nMotivo: %s", q, motivo)
				}
			}
		})
	}
}

// El aviso general también tiene que distinguir, porque es lo primero que se lee.
func TestElAvisoDeIBANNoDiceQueFaltaCuandoEstaMalEscrito(t *testing.T) {
	malEscrito := []PagoRow{{Nombre: "INVERSIONES DEL PACIFICO", IBAN: "cr123456789874563210236"}}
	msg := mensajeIBAN(malEscrito)
	if strings.Contains(msg, "sin cuenta IBAN") {
		t.Errorf("sigue diciendo que FALTA la cuenta y lo que pasa es que está mal escrita:\n%s", msg)
	}
	if !strings.Contains(msg, "MAL ESCRITO") {
		t.Errorf("el aviso no dice cuál es el problema:\n%s", msg)
	}

	t.Run("cuando de verdad falta, lo dice así", func(t *testing.T) {
		msg := mensajeIBAN([]PagoRow{{Nombre: "SIN CUENTA", IBAN: ""}})
		if !strings.Contains(msg, "sin cuenta IBAN") {
			t.Errorf("con el campo vacío sí corresponde decir que falta:\n%s", msg)
		}
	})

	t.Run("mezclados: los cuenta por separado", func(t *testing.T) {
		msg := mensajeIBAN([]PagoRow{
			{Nombre: "A", IBAN: ""},
			{Nombre: "B", IBAN: "cr123456789874563210236"},
			{Nombre: "C", IBAN: "cr123456789874563210239"},
		})
		// Se arreglan en lugares distintos, así que el aviso no puede juntarlos en un solo número.
		for _, q := range []string{"1 proveedor(es) sin IBAN", "2 con el IBAN mal escrito"} {
			if !strings.Contains(msg, q) {
				t.Errorf("el aviso no dice %q.\nAviso: %s", q, msg)
			}
		}
	})
}
