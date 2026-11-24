package cxp

// El catálogo de prioridades contra PostgreSQL de verdad (mig 0121).
//
// «Que se pueda agregar nuevas prioridades.» Hasta la 0121 eran tres valores en un CHECK, así que
// una cuarta exigía migración y despliegue.
//
// Lo que se protege acá no lo puede demostrar un doble:
//
//  1. QUE UNA PRIORIDAD NUEVA SE USE DE VERDAD. Crearla es lo fácil; lo que importa es que el
//     orden de la Bandeja la ponga donde corresponde y que el filtro de urgentes la tome. Las dos
//     cosas son SQL contra el catálogo, no lógica de Go.
//  2. QUE NO SE PUEDA BORRAR UNA QUE YA ESTÁ PUESTA EN FACTURAS. Borrarla dejaría facturas con un
//     código que ninguna pantalla sabe nombrar ni colorear. La condición vive en el DELETE.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestUnaPrioridadNuevaOrdenaYCuentaComoUrgente(t *testing.T) {
	repo := baseDeLotes(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	// El catálogo nace vacío en el esquema temporal (la copia no trae datos), así que se siembran
	// las dos de siempre más una por encima: es el caso del usuario —«si mañana creás una AAA».
	for _, p := range []PrioridadInput{
		{Codigo: "A", Nombre: "Puede esperar", Orden: 10, Urgente: false, Tono: "pendiente", Activa: true},
		{Codigo: "AA", Nombre: "Se paga sí o sí", Orden: 20, Urgente: true, Tono: "negativo", Activa: true},
		{Codigo: "AAA", Nombre: "Urgencia absoluta", Orden: 30, Urgente: true, Tono: "negativo", Activa: true},
	} {
		if _, err := svc.CrearPrioridad(ctx, empresaLote, p, usuarioLote); err != nil {
			t.Fatalf("crear prioridad %s: %v", p.Codigo, err)
		}
	}

	lista, err := svc.Prioridades(ctx, empresaLote, true)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(lista) != 3 {
		t.Fatalf("prioridades = %d, se esperaban 3", len(lista))
	}
	// De la más alta a la más baja: es el orden con el que la pantalla dibuja el selector.
	if lista[0].Codigo != "AAA" || lista[1].Codigo != "AA" || lista[2].Codigo != "A" {
		t.Errorf("orden del catálogo = %s, %s, %s; se esperaba AAA, AA, A",
			lista[0].Codigo, lista[1].Codigo, lista[2].Codigo)
	}
}

// Marcar una factura con un código que no existe tiene que fallar, y el error tiene que NOMBRAR
// las válidas: es justo lo que un CHECK de la base nunca pudo responder.
func TestNoSePuedeMarcarUnaPrioridadQueNoExiste(t *testing.T) {
	repo := baseDeLotes(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	if _, err := svc.CrearPrioridad(ctx, empresaLote,
		PrioridadInput{Codigo: "AA", Nombre: "Se paga sí o sí", Orden: 20, Urgente: true,
			Tono: "negativo", Activa: true}, usuarioLote); err != nil {
		t.Fatalf("sembrar catálogo: %v", err)
	}

	err := svc.validarPrioridad(ctx, empresaLote, "ZZZ")
	if !errors.Is(err, ErrPrioridadNoEncontrada) {
		t.Fatalf("error = %v, se esperaba ErrPrioridadNoEncontrada", err)
	}
	// El mensaje dice CUÁLES son las válidas: sin eso, quien lo recibe tiene que ir a buscarlas.
	if !strings.Contains(err.Error(), "AA") {
		t.Errorf("el error no nombra las prioridades válidas: %q", err)
	}

	// Y el vacío SIEMPRE vale: quitarle la prioridad a una factura tiene que poder hacerse aunque
	// el catálogo quede sin ninguna activa.
	if err := svc.validarPrioridad(ctx, empresaLote, ""); err != nil {
		t.Errorf("quitar la prioridad falló: %v", err)
	}
}

// Dos prioridades con el mismo código serían indistinguibles en las facturas ya marcadas, porque
// el documento guarda el CÓDIGO y no un id.
func TestNoSePuedenRepetirLosCodigosDePrioridad(t *testing.T) {
	repo := baseDeLotes(t)
	svc := &Service{repo: repo}
	ctx := context.Background()

	in := PrioridadInput{Codigo: "AA", Nombre: "Se paga sí o sí", Orden: 20, Urgente: true,
		Tono: "negativo", Activa: true}
	if _, err := svc.CrearPrioridad(ctx, empresaLote, in, usuarioLote); err != nil {
		t.Fatalf("crear la primera: %v", err)
	}
	// El código se normaliza a mayúsculas, así que «aa» es la MISMA: si no lo fuera, la tabla
	// tendría dos insignias que nadie puede distinguir de un vistazo.
	in.Codigo = "aa"
	in.Nombre = "Otra cosa"
	if _, err := svc.CrearPrioridad(ctx, empresaLote, in, usuarioLote); !errors.Is(err, ErrCodigoPrioridadRepetido) {
		t.Fatalf("error = %v, se esperaba ErrCodigoPrioridadRepetido", err)
	}
}

func TestLaValidacionDeLaPrioridadPideNombreYColorDelSistema(t *testing.T) {
	casos := []struct {
		nombre   string
		in       PrioridadInput
		esperado error
	}{
		{"sin código", PrioridadInput{Nombre: "Algo", Tono: "negativo"}, ErrCodigoPrioridadRequerido},
		{"código larguísimo", PrioridadInput{Codigo: "DEMASIADO", Nombre: "Algo", Tono: "negativo"}, ErrCodigoPrioridadRequerido},
		{"nombre de una letra", PrioridadInput{Codigo: "X", Nombre: "a", Tono: "negativo"}, ErrNombrePrioridadRequerido},
		// Un hexadecimal elegido a mano se ve bien en un tema y desaparece en el otro.
		{"color inventado", PrioridadInput{Codigo: "X", Nombre: "Algo", Tono: "#ff0000"}, ErrTonoPrioridadInvalido},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			in := c.in
			if err := in.limpiar(); !errors.Is(err, c.esperado) {
				t.Errorf("error = %v, se esperaba %v", err, c.esperado)
			}
		})
	}
}
