package cxp

// Que el marcado EN LOTE deje el trabajo terminado (4-nov-2026).
//
// EL PEDIDO: «dale, igualalo» — el panel «Proveedores que declaran contado» ponía UNA de las dos
// condiciones del candado y avisaba «falta pasarlo a crédito» sin resolverlo. Con 573 de 649
// fichas en CONTADO, el camino dominante terminaba mandando a corregir a mano, ficha por ficha:
// el mismo trabajo tedioso que el panel venía a eliminar.
//
// El efecto en la base se prueba contra PostgreSQL (convenio_masivo_pg_test.go). Acá se prueba lo
// que decide el servicio: qué se le pide al repositorio y qué sale hacia el cliente.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// La decisión de la pantalla tiene que llegar al repositorio tal como se tomó, en los dos
// sentidos: desmarcar la casilla es una decisión, no una ausencia.
func TestElLotePasaLaDecisionDePasarACredito(t *testing.T) {
	for _, quiere := range []bool{true, false} {
		f := &fakeRepo{}
		_, err := svcConFake(f).MarcarConvenioMasivo(context.Background(), "emp",
			ConvenioMasivoInput{ProveedorIDs: []string{"p1"}, Convenio: "PROVEEDOR", PasarACredito: quiere},
			"usr")
		if err != nil {
			t.Fatalf("marcar: %v", err)
		}
		if f.convenioPasarACredito != quiere {
			t.Errorf("se pidió pasar a crédito = %v y al repositorio le llegó %v",
				quiere, f.convenioPasarACredito)
		}
	}
}

// LOS IDS SON PARA LA HUELLA, NO PARA LA PANTALLA.
//
// `PasadosACreditoIDs` existe para poder dejar el evento en la ficha de cada proveedor. Si viajara
// al cliente sería un dato interno expuesto sin que nadie lo use; el panel habla de nombres.
func TestLosIdsDeLasFichasNoViajanAlCliente(t *testing.T) {
	f := &fakeRepo{resultadoConvenio: ResultadoConvenioMasivo{
		Actualizados:       2,
		PasadosACredito:    []string{"FERRETERIA EL CLAVO"},
		PasadosACreditoIDs: []string{"11111111-1111-4111-8111-111111111111"},
	}}
	res, err := svcConFake(f).MarcarConvenioMasivo(context.Background(), "emp",
		ConvenioMasivoInput{ProveedorIDs: []string{"p1"}, Convenio: "PROVEEDOR", PasarACredito: true},
		"usr")
	if err != nil {
		t.Fatalf("marcar: %v", err)
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	cuerpo := string(b)
	if strings.Contains(cuerpo, "11111111-1111") {
		t.Errorf("el id de la ficha salió hacia el cliente: %s", cuerpo)
	}
	if !strings.Contains(cuerpo, "FERRETERIA EL CLAVO") {
		t.Errorf("no salió el nombre del proveedor que cambió de condición: %s", cuerpo)
	}
}

// Un pedido inválido no llega a la base. El tope existe para que un error de selección no
// reescriba el maestro entero de un clic.
func TestElLoteValidaAntesDeTocarNada(t *testing.T) {
	casos := []struct {
		nombre string
		in     ConvenioMasivoInput
		quiere error
	}{
		{"sin proveedores", ConvenioMasivoInput{Convenio: "PROVEEDOR"}, ErrSinProveedores},
		{"convenio inventado", ConvenioMasivoInput{ProveedorIDs: []string{"p1"}, Convenio: "CUALQUIERA"}, ErrConvenioInvalido},
		{"demasiados", ConvenioMasivoInput{ProveedorIDs: make([]string, topeProveedoresMasivo+1), Convenio: "XML"}, ErrDemasiadosProveedores},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := &fakeRepo{}
			if _, err := svcConFake(f).MarcarConvenioMasivo(context.Background(), "emp", c.in, "usr"); !errors.Is(err, c.quiere) {
				t.Errorf("error = %v, quería %v", err, c.quiere)
			}
			if f.convenioRecibido != "" {
				t.Error("se llamó al repositorio con un pedido inválido")
			}
		})
	}
}
