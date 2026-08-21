package nomina

// Tests de las notificaciones de RRHH. Acá viajan salarios, así que lo que se prueba es a quién
// NO se le manda y que un fallo suelto no deje a los demás sin su boleta.

import (
	"context"
	"errors"
	"go.uber.org/zap"
	"strings"
	"testing"
)

// correoFake registra lo enviado y puede fallar para una dirección concreta.
type correoFake struct {
	enviados []string // "to|asunto"
	cuerpos  map[string]string
	fallaA   string
}

func (c *correoFake) Enviar(to, asunto, cuerpo string) error {
	if c.fallaA != "" && to == c.fallaA {
		return errors.New("servidor rechazó el destinatario")
	}
	if c.cuerpos == nil {
		c.cuerpos = map[string]string{}
	}
	c.enviados = append(c.enviados, to+"|"+asunto)
	c.cuerpos[to] = cuerpo
	return nil
}

func TestEnviarBoletasSinCorreoConfigurado(t *testing.T) {
	// Sin servidor de correo se dice claramente, no se falla de forma rara.
	_, err := NewService(newFakeRepo(), nil, zap.NewNop()).EnviarBoletas(context.Background(), "e1", "c1", "u1")
	if !errors.Is(err, ErrCorreoNoConfigurado) {
		t.Errorf("err = %v, quiere ErrCorreoNoConfigurado", err)
	}
}

func TestEnviarBoletas(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	st := repo.corridaStore()
	st.corridas["c1"] = Corrida{ID: "c1", Anio: 2026, Mes: 7, FechaPago: "2026-07-31", Estado: "APROBADA"}
	st.lineas["c1"] = []LineaCorrida{
		{ID: "l1", EmpleadoID: "e-1", Nombre: "María Fernández", Identificacion: "1-1234-5678",
			Puesto: "Asesora", Bruto: "650000.00", CCSSObrero: "70395.00", Renta: "0.00",
			Deducciones: "25000.00", Adelanto: "0.00", Neto: "554605.00"},
		{ID: "l2", EmpleadoID: "e-2", Nombre: "Juan Pérez", Neto: "300000.00"},
		{ID: "l3", EmpleadoID: "e-3", Nombre: "Ana Solís", Neto: "400000.00"},
	}
	// e-2 NO tiene correo en su ficha; a e-3 el servidor lo rechaza.
	repo.correos = map[string]string{"e-1": "maria@vdp.cr", "e-3": "ana@vdp.cr"}
	correo := &correoFake{fallaA: "ana@vdp.cr"}

	svc := NewService(repo, nil, zap.NewNop())
	svc.SetNotificaciones(nil, correo) // sin plantillero: usa el texto de fábrica

	res, err := svc.EnviarBoletas(ctx, "e1", "c1", "u1")
	if err != nil {
		t.Fatalf("enviar: %v", err)
	}

	t.Run("manda solo a quien tiene correo y reporta el resto", func(t *testing.T) {
		if res.Enviados != 1 {
			t.Errorf("enviados = %d, quiere 1", res.Enviados)
		}
		if len(res.SinCorreo) != 1 || res.SinCorreo[0] != "Juan Pérez" {
			t.Errorf("sin correo = %v, quiere [Juan Pérez]", res.SinCorreo)
		}
		if len(res.Fallidos) != 1 || res.Fallidos[0] != "Ana Solís" {
			t.Errorf("fallidos = %v, quiere [Ana Solís]", res.Fallidos)
		}
	})

	t.Run("un fallo no detiene a los demás", func(t *testing.T) {
		// María va primero y Ana falla después: el reporte los distingue y el envío siguió.
		if len(correo.enviados) != 1 || !strings.HasPrefix(correo.enviados[0], "maria@vdp.cr|") {
			t.Errorf("enviados = %v", correo.enviados)
		}
	})

	t.Run("el correo lleva el período legible y las cifras del empleado", func(t *testing.T) {
		if !strings.Contains(correo.enviados[0], "Julio 2026") {
			t.Errorf("el asunto debería decir «Julio 2026»: %q", correo.enviados[0])
		}
		cuerpo := correo.cuerpos["maria@vdp.cr"]
		for _, esperado := range []string{"María Fernández", "₡650 000,00", "₡554 605,00", "31/07/2026", "Asesora"} {
			if !strings.Contains(cuerpo, esperado) {
				t.Errorf("falta %q en el cuerpo:\n%s", esperado, cuerpo)
			}
		}
		if strings.Contains(cuerpo, "[") {
			t.Errorf("quedó un marcador sin llenar:\n%s", cuerpo)
		}
	})
}

func TestEnviarAvisoVacacionesSinCorreoEnLaFicha(t *testing.T) {
	repo := newFakeRepo()
	repo.vacacionAviso = VacacionAviso{EmpleadoID: "e-1", Nombre: "Juan Pérez", Email: ""}
	svc := NewService(repo, nil, zap.NewNop())
	svc.SetNotificaciones(nil, &correoFake{})
	err := svc.EnviarAvisoVacaciones(context.Background(), "e1", "v1", "u1")
	if !errors.Is(err, ErrEmpleadoSinCorreo) {
		t.Errorf("err = %v, quiere ErrEmpleadoSinCorreo (no se adivina una dirección)", err)
	}
}

func TestMilesYFechaLegible(t *testing.T) {
	casos := map[string]string{
		"650000.00":  "650 000,00",
		"1234567.89": "1 234 567,89",
		"999.50":     "999,50",
		"0.00":       "0,00",
		"-45000.50":  "-45 000,50",
	}
	for entrada, quiere := range casos {
		if got := miles(entrada); got != quiere {
			t.Errorf("miles(%s) = %q, quiere %q", entrada, got, quiere)
		}
	}
	if got := fechaLegible("2026-07-31"); got != "31/07/2026" {
		t.Errorf("fechaLegible = %q", got)
	}
	if got := fechaLegible(""); got != "" {
		t.Errorf("fecha vacía debería quedar vacía, dio %q", got)
	}
	if got := periodoTexto(2026, 7); got != "Julio 2026" {
		t.Errorf("periodoTexto = %q", got)
	}
	if got := periodoTexto(2026, 13); got != "2026-13" {
		t.Errorf("mes inválido debería caer al formato numérico, dio %q", got)
	}
}
