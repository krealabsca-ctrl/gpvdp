package cxc

import (
	"errors"
	"strings"
	"testing"
)

func ptrS(s string) *string { return &s }
func ptrI(i int) *int       { return &i }

// La regla que da sentido a toda la pantalla: una cuota en cero NO resuelve la revisión.
//
// Si se aceptara, el contrato saldría de la cola de apartados y entraría a la cola de cobro con
// cuota 0: el cobrador llamaría a una persona a pedirle nada.
func TestCorreccionRechazaCuotaQueNoSirve(t *testing.T) {
	for _, v := range []string{"0", "0.00", "-1", "-4700", "", "  ", "abc", "4.700,00"} {
		t.Run(v, func(t *testing.T) {
			_, err := validarCorreccion(CorreccionContrato{Cuota: ptrS(v)})
			if !errors.Is(err, ErrCuotaInvalida) {
				t.Errorf("cuota %q: err = %v, quiere ErrCuotaInvalida", v, err)
			}
		})
	}
}

func TestCorreccionAceptaCuotaBuena(t *testing.T) {
	for _, v := range []string{"4700", "4700.50", " 12000 ", "0.01"} {
		cambios, err := validarCorreccion(CorreccionContrato{Cuota: ptrS(v)})
		if err != nil {
			t.Fatalf("cuota %q: %v", v, err)
		}
		if _, ok := cambios["cuota_vigente"]; !ok {
			t.Errorf("cuota %q: no se registró el cambio", v)
		}
	}
}

func TestCorreccionValidaDiaDePago(t *testing.T) {
	for _, d := range []int{0, -1, 32, 99} {
		if _, err := validarCorreccion(CorreccionContrato{DiaPago: ptrI(d)}); !errors.Is(err, ErrDiaPagoInvalido) {
			t.Errorf("día %d: err = %v, quiere ErrDiaPagoInvalido", d, err)
		}
	}
	// 31 se acepta: el generador ya topa el día al fin de mes, no es trabajo de esta validación.
	for _, d := range []int{1, 15, 28, 30, 31} {
		if _, err := validarCorreccion(CorreccionContrato{DiaPago: ptrI(d)}); err != nil {
			t.Errorf("día %d debería aceptarse: %v", d, err)
		}
	}
}

// Un PATCH sin ningún campo no toca la base ni escribe auditoría: un evento que dice «alguien
// corrigió» sin ningún cambio ensucia la bitácora que después hay que leer.
func TestCorreccionSinCambiosNoHaceNada(t *testing.T) {
	if _, err := validarCorreccion(CorreccionContrato{Nota: "solo una nota"}); !errors.Is(err, ErrNadaQueCorregir) {
		t.Errorf("err = %v, quiere ErrNadaQueCorregir", err)
	}
}

// Es PATCH y no PUT: lo que no se manda no se toca. Con PUT, corregir solo la modalidad pondría la
// cuota en cero y volvería a apartar el contrato que se venía a arreglar.
func TestCorreccionSoloTocaLoQueLlega(t *testing.T) {
	cambios, err := validarCorreccion(CorreccionContrato{ModalidadID: ptrS("0a15795d-7229-41e6-95b8-4b70bfbf74b2")})
	if err != nil {
		t.Fatalf("validar: %v", err)
	}
	if len(cambios) != 1 {
		t.Fatalf("cambios = %v, quiere solo la modalidad", cambios)
	}
	if _, ok := cambios["cuota_vigente"]; ok {
		t.Error("no se mandó cuota: no puede aparecer en los cambios")
	}
	if _, ok := cambios["dia_pago"]; ok {
		t.Error("no se mandó día de pago: no puede aparecer en los cambios")
	}
}

func TestCorreccionModalidadVaciaSeRechaza(t *testing.T) {
	if _, err := validarCorreccion(CorreccionContrato{ModalidadID: ptrS("   ")}); !errors.Is(err, ErrModalidadNoEncontrada) {
		t.Errorf("err = %v, quiere ErrModalidadNoEncontrada", err)
	}
}

// La marca de revisión se DERIVA del dato, y la regla que la deriva tiene que ser la misma que usa
// el generador de cargos para excluir. Si se separan, un contrato puede quedar «sin revisión» y
// aun así no generar nada —invisible en las dos pantallas—.
func TestReglaDeRevisionCoincideConLaDelGenerador(t *testing.T) {
	// sqlMotivoNoGenerable tiene que estar construido SOBRE la parte objetiva, no ser una copia.
	if !strings.Contains(sqlMotivoNoGenerable, sqlMotivoDatoIncompleto) {
		t.Error("sqlMotivoNoGenerable dejó de componerse con sqlMotivoDatoIncompleto: " +
			"son dos copias de la misma regla y van a divergir")
	}
	// Y la parte objetiva NO puede mirar la marca: derivarla de sí misma dejaría al contrato
	// apartado para siempre por más que se le arregle la cuota.
	if strings.Contains(sqlMotivoDatoIncompleto, "revision_pendiente") {
		t.Error("sqlMotivoDatoIncompleto no puede mirar revision_pendiente: la derivación sería circular")
	}
}
