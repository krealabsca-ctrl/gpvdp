package sistema

// Las consultas lentas (mig 0120).
//
// Dos cosas se protegen acá, y son de naturaleza distinta:
//
//  1. QUE UN SERVIDOR SIN LA EXTENSIÓN NO SE VEA COMO UN SISTEMA ROTO. Es el estado normal de
//     cualquier instalación que no actualizó su docker-compose, y la pantalla tiene que decir qué
//     hacer en vez de pintarse de rojo o, peor, mostrar una lista vacía que se lee como «no hay
//     consultas lentas». Eso se prueba con un doble: es lógica del servicio.
//
//  2. QUE NO SE ESCAPE NI UN DATO DE NINGUNA EMPRESA. Esta pantalla la mira quien administra, pero
//     el ERP guarda montos, cédulas y nombres de proveedores. La garantía es que PostgreSQL
//     normaliza las consultas antes de contarlas —los valores se vuelven $1, $2— y eso NO se puede
//     comprobar con un doble: hay que preguntárselo a Postgres. Está en rendimiento_pg_test.go.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// repoFingido devuelve lo que la prueba le diga.
type repoFingido struct {
	consultas []ConsultaLenta
	desde     string
	err       error
	limite    int // el que recibió, para comprobar el acotado
	reinicios int
}

func (r *repoFingido) ConsultasLentas(_ context.Context, limite int) ([]ConsultaLenta, string, error) {
	r.limite = limite
	return r.consultas, r.desde, r.err
}

func (r *repoFingido) ReiniciarContadores(context.Context) error {
	r.reinicios++
	return r.err
}

// Un servidor sin la extensión NO es un error: es una herramienta opcional apagada.
func TestSinLaExtensionLaPantallaExplicaComoEncenderla(t *testing.T) {
	repo := &repoFingido{err: ErrRendimientoNoDisponible}
	svc := NewServiceRendimiento(repo)

	res, err := svc.ConsultasLentas(context.Background(), 0)
	if err != nil {
		t.Fatalf("devolvió error: %v — un servidor sin la extensión no está roto", err)
	}
	if res.Disponible {
		t.Error("se declaró disponible sin extensión")
	}
	// El motivo tiene que decir QUÉ HACER: quien abre esta pantalla es quien puede arreglarlo, y
	// un «no disponible» a secas lo deja sin camino.
	if !strings.Contains(res.Motivo, "shared_preload_libraries") {
		t.Errorf("el motivo no dice cómo encenderlo: %q", res.Motivo)
	}
	// Lista vacía, NUNCA nil: un `null` en el JSON obliga a la pantalla a defenderse de dos formas
	// distintas del vacío.
	if res.Consultas == nil {
		t.Error("consultas = nil; debe ser lista vacía")
	}
}

// Un fallo de verdad SÍ es un error: no se puede disfrazar de «no disponible», porque entonces
// una base caída se leería como una extensión apagada y se buscaría el problema donde no está.
func TestUnFalloDeVerdadSiSePropaga(t *testing.T) {
	repo := &repoFingido{err: errors.New("se cayó la base")}
	svc := NewServiceRendimiento(repo)

	if _, err := svc.ConsultasLentas(context.Background(), 0); err == nil {
		t.Fatal("un fallo real debería propagarse, no disfrazarse de «no disponible»")
	}
}

func TestElLimiteSeAcota(t *testing.T) {
	casos := []struct {
		nombre string
		pedido int
		quiere int
	}{
		{"sin pedir nada usa el de por defecto", 0, topConsultasDefecto},
		{"negativo usa el de por defecto", -5, topConsultasDefecto},
		{"un valor razonable se respeta", 30, 30},
		{"un valor enorme se corta", 100000, topConsultasMaximo},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo := &repoFingido{}
			svc := NewServiceRendimiento(repo)
			if _, err := svc.ConsultasLentas(context.Background(), c.pedido); err != nil {
				t.Fatalf("error: %v", err)
			}
			if repo.limite != c.quiere {
				t.Errorf("límite = %d, se esperaba %d", repo.limite, c.quiere)
			}
		})
	}
}

func TestReiniciarSinExtensionDevuelveElMotivo(t *testing.T) {
	repo := &repoFingido{err: ErrRendimientoNoDisponible}
	svc := NewServiceRendimiento(repo)

	err := svc.ReiniciarContadores(context.Background())
	if !errors.Is(err, ErrRendimientoNoDisponible) {
		t.Fatalf("error = %v, se esperaba ErrRendimientoNoDisponible", err)
	}
}
