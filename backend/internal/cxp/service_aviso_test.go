package cxp

// Guardar el texto del aviso: lo que decide el servicio.
//
// El riesgo de esta operación no es que falle, es que BORRE: el INSERT escribe las dos columnas
// siempre, así que un pedido que trae solo el concepto dejaría la glosa en NULL y el cuadro
// volvería a la propuesta sin que nadie entienda por qué.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// GUARDAR SOLO UN CAMPO NO BORRA EL OTRO.
func TestGuardarSoloElConceptoNoBorraLaGlosa(t *testing.T) {
	f := &fakeRepo{
		avisoProveedorEnCorte: true,
		textosAviso: map[string]TextoAviso{
			"p1": {Concepto: ptr("viejo"), Documentos: ptr("FC 87 88 URNAS ZOAART")},
		},
	}
	err := svcConFake(f).GuardarTextoDeAviso(context.Background(), "emp", "lote", "p1",
		TextoAviso{Concepto: ptr("Urnas Personalizadas")}, "usr")
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if f.textoAvisoGuardado == nil {
		t.Fatal("no se llamó al repositorio")
	}
	if f.textoAvisoGuardado.Documentos == nil {
		t.Fatal("la glosa se iba a guardar como NULL: el cuadro perdería lo que alguien escribió")
	}
	if *f.textoAvisoGuardado.Documentos != "FC 87 88 URNAS ZOAART" {
		t.Errorf("la glosa quedó en %q, quería la que ya estaba", *f.textoAvisoGuardado.Documentos)
	}
	if *f.textoAvisoGuardado.Concepto != "Urnas Personalizadas" {
		t.Errorf("el concepto quedó en %q", *f.textoAvisoGuardado.Concepto)
	}
}

// Sin texto previo, lo que no venga se guarda como NULL: no hay nada que conservar y NULL es
// justamente «usá lo que el sistema propone».
func TestSinTextoPrevioLoQueNoVieneQuedaEnNulo(t *testing.T) {
	f := &fakeRepo{avisoProveedorEnCorte: true}
	if err := svcConFake(f).GuardarTextoDeAviso(context.Background(), "emp", "lote", "p1",
		TextoAviso{Concepto: ptr("Urnas")}, "usr"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if f.textoAvisoGuardado.Documentos != nil {
		t.Errorf("se guardó una glosa que nadie escribió: %q", *f.textoAvisoGuardado.Documentos)
	}
}

// EL PROVEEDOR TIENE QUE ESTAR EN EL CORTE. Un texto de un proveedor ajeno no se vería en ninguna
// pantalla y aparecería solo el día que ese proveedor entre a un corte.
func TestNoSeGuardaTextoDeUnProveedorAjenoAlCorte(t *testing.T) {
	f := &fakeRepo{avisoProveedorEnCorte: false}
	err := svcConFake(f).GuardarTextoDeAviso(context.Background(), "emp", "lote", "p9",
		TextoAviso{Concepto: ptr("Lo que sea")}, "usr")
	if !errors.Is(err, ErrProveedorNoEstaEnElCorte) {
		t.Errorf("error = %v, quería ErrProveedorNoEstaEnElCorte", err)
	}
}

// EL TOPE SE COMPRUEBA ANTES DE LA BASE: así el usuario lee una explicación en vez del 500 de una
// violación de CHECK. Los topes son los mismos de la migración 0117.
func TestElTextoDelAvisoTieneTope(t *testing.T) {
	casos := []struct {
		nombre string
		in     TextoAviso
		falla  bool
	}{
		{"concepto normal", TextoAviso{Concepto: ptr("Urnas Personalizadas")}, false},
		{"concepto larguísimo", TextoAviso{Concepto: ptr(strings.Repeat("a", largoMaximoConcepto+1))}, true},
		{"concepto justo en el tope", TextoAviso{Concepto: ptr(strings.Repeat("a", largoMaximoConcepto))}, false},
		{"glosa larguísima", TextoAviso{Documentos: ptr(strings.Repeat("b", largoMaximoDocumentos+1))}, true},
		// Las tildes cuentan como UN carácter, no como dos bytes: con un tope por bytes, un
		// concepto en español entraría en el límite antes de llegar al largo real.
		{"tildes al borde", TextoAviso{Concepto: ptr(strings.Repeat("á", largoMaximoConcepto))}, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := &fakeRepo{avisoProveedorEnCorte: true}
			err := svcConFake(f).GuardarTextoDeAviso(context.Background(), "emp", "lote", "p1", c.in, "usr")
			if c.falla && !errors.Is(err, ErrTextoDeAvisoLargo) {
				t.Errorf("se aceptó un texto de más: %v", err)
			}
			if !c.falla && err != nil {
				t.Errorf("se rechazó un texto válido: %v", err)
			}
		})
	}
}

// El texto se guarda SIN espacios de sobra: «  Urnas  » y «Urnas» son el mismo concepto, y dejar
// el espacio hace que el cuadro se vea desalineado sin que se entienda por qué.
func TestElTextoDelAvisoSeGuardaLimpio(t *testing.T) {
	f := &fakeRepo{avisoProveedorEnCorte: true}
	if err := svcConFake(f).GuardarTextoDeAviso(context.Background(), "emp", "lote", "p1",
		TextoAviso{Concepto: ptr("   Urnas Personalizadas  ")}, "usr"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if got := *f.textoAvisoGuardado.Concepto; got != "Urnas Personalizadas" {
		t.Errorf("se guardó %q", got)
	}
}
