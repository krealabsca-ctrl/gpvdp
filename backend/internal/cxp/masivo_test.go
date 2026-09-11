package cxp

// Autorización de las acciones en lote: por PERMISO, no por código de rol.
//
// El test que importa es TestRolAMedidaConElPermisoPuedeLiquidar: reproduce lo que pasó en
// producción el 9 de setiembre de 2026 —un rol a medida con todo CxP marcado recibía «el rol no
// puede ejecutar esta acción» al liquidar viáticos— y falla si alguien vuelve a autorizar por
// nombre de rol.

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

// permisosFalsos implementa PermisoChecker con un conjunto fijo por rol.
type permisosFalsos struct {
	// porRol: rol → permisos que tiene. Un rol ausente no tiene ninguno.
	porRol map[string][]string
	// err: si no es nil, el verificador falla (simula la base caída).
	err error
	// consultas registra lo que se preguntó, para comprobar que se pide el permiso correcto.
	consultas []string
}

func (p *permisosFalsos) Tiene(_ context.Context, _, rol, permiso string) (bool, error) {
	p.consultas = append(p.consultas, permiso)
	if p.err != nil {
		return false, p.err
	}
	for _, x := range p.porRol[rol] {
		if x == permiso {
			return true, nil
		}
	}
	return false, nil
}

func servicioConPermisos(p *permisosFalsos) *Service {
	svc := NewService(&fakeRepo{}, nil, zap.NewNop())
	svc.SetPermisos(p)
	return svc
}

// EL CASO DE PRODUCCIÓN. Un rol creado desde la matriz, con `cxp.revisar` marcado, tiene que poder
// liquidar viáticos. Antes fallaba porque su código no estaba en una lista escrita a mano.
func TestRolAMedidaConElPermisoPuedeLiquidar(t *testing.T) {
	perms := &permisosFalsos{porRol: map[string][]string{
		"CUSTOM_SUPERVISOR_CONTABLE": {"cxp.revisar"},
	}}
	svc := servicioConPermisos(perms)

	// No falla con ErrRolNoAutorizado: pasa la autorización y llega a la transición por documento
	// (que con el repo falso reporta su propio error por documento, no un error del lote).
	res, err := svc.TransicionMasiva(context.Background(), "e", "u",
		"CUSTOM_SUPERVISOR_CONTABLE", AccLiquidar, []string{"x"}, "", "viáticos de setiembre")
	if errors.Is(err, ErrRolNoAutorizado) {
		t.Fatal("un rol a medida con cxp.revisar tiene que poder liquidar: volvió a autorizarse por nombre de rol")
	}
	if err != nil {
		t.Fatalf("no esperaba un error del lote: %v", err)
	}
	if len(res.Resultados) != 1 {
		t.Fatalf("esperaba el resultado del único documento, obtuve %+v", res)
	}
	// Y se preguntó por el permiso de la acción, no por otro.
	if len(perms.consultas) == 0 || perms.consultas[0] != "cxp.revisar" {
		t.Errorf("preguntó por %v, quería cxp.revisar", perms.consultas)
	}
}

func TestAccionSinElPermisoSeRechaza(t *testing.T) {
	// El mismo rol a medida, ahora sin `cxp.tesoreria`: no puede pagar en lote.
	perms := &permisosFalsos{porRol: map[string][]string{
		"CUSTOM_SUPERVISOR_CONTABLE": {"cxp.revisar"},
	}}
	svc := servicioConPermisos(perms)

	_, err := svc.TransicionMasiva(context.Background(), "e", "u",
		"CUSTOM_SUPERVISOR_CONTABLE", AccPagar, []string{"x"}, "", "")
	if !errors.Is(err, ErrRolNoAutorizado) {
		t.Errorf("pagar sin cxp.tesoreria => %v, quería ErrRolNoAutorizado", err)
	}
}

// Aprobar acepta el permiso general O el de las facturas «de Contabilidad»: sin el «o», el
// Supervisor perdería en lote lo que sí puede aprobar de a una.
func TestAprobarEnLoteAceptaElPermisoDeContabilidad(t *testing.T) {
	perms := &permisosFalsos{porRol: map[string][]string{
		"SUPERVISOR_FINANCIERO": {"cxp.revisar", "cxp.tesoreria", "cxp.aprobar_contabilidad"},
	}}
	svc := servicioConPermisos(perms)

	_, err := svc.TransicionMasiva(context.Background(), "e", "u",
		"SUPERVISOR_FINANCIERO", AccAprobar, []string{"x"}, "", "")
	if errors.Is(err, ErrRolNoAutorizado) {
		t.Error("con cxp.aprobar_contabilidad tiene que pasar la autorización del lote")
	}
}

// Sin verificador inyectado NO se autoriza nada. Deny-by-default: un servicio mal armado no puede
// terminar autorizando transiciones de dinero.
func TestSinVerificadorNoAutoriza(t *testing.T) {
	svc := NewService(&fakeRepo{}, nil, zap.NewNop()) // sin SetPermisos

	_, err := svc.TransicionMasiva(context.Background(), "e", "u", "ADMIN", AccRevisar, []string{"x"}, "", "")
	if !errors.Is(err, ErrRolNoAutorizado) {
		t.Errorf("sin verificador => %v, quería ErrRolNoAutorizado", err)
	}
}

// Un fallo del verificador se PROPAGA. Traducirlo a «no tenés permiso» mandaría al usuario a
// revisar la matriz por una base caída.
func TestFalloDelVerificadorNoEsUnNo(t *testing.T) {
	fallo := errors.New("base caída")
	svc := servicioConPermisos(&permisosFalsos{err: fallo})

	_, err := svc.TransicionMasiva(context.Background(), "e", "u", "ADMIN", AccRevisar, []string{"x"}, "", "")
	if !errors.Is(err, fallo) {
		t.Errorf("fallo del verificador => %v, quería que se propague", err)
	}
}

func TestTodaAccionTieneSuPermiso(t *testing.T) {
	// Si mañana se agrega una acción y se olvida su permiso, el mapa la dejaría con lista vacía y
	// `puedeAccion` devolvería false para todos: la acción quedaría muerta en silencio.
	acciones := []string{
		AccRevisar, AccAprobar, AccProgramar, AccPagar, AccConciliar,
		AccDenegar, AccAnular, AccLiquidar, AccRebotar, AccReintentar,
	}
	for _, a := range acciones {
		if len(permisosPorAccion[a]) == 0 {
			t.Errorf("la acción %q no declara ningún permiso", a)
		}
		if !accionValida(a) {
			t.Errorf("la acción %q no se reconoce como válida", a)
		}
	}
	if len(permisosPorAccion) != len(acciones) {
		t.Errorf("el mapa tiene %d acciones y la lista %d: alguna quedó sin probar",
			len(permisosPorAccion), len(acciones))
	}
}

func TestTransicionMasivaValidaciones(t *testing.T) {
	perms := &permisosFalsos{porRol: map[string][]string{
		"ADMIN": {"cxp.revisar", "cxp.tesoreria", "cxp.aprobar", "cxp.anular", "cxp.resultado_pago"},
	}}
	svc := servicioConPermisos(perms)
	ctx := context.Background()

	// La acción inválida se corta ANTES de consultar permisos: no hay permiso que consultar.
	if _, err := svc.TransicionMasiva(ctx, "e", "u", "ADMIN", "borrar", []string{"x"}, "", ""); err != ErrAccionInvalida {
		t.Errorf("acción inválida => %v, want ErrAccionInvalida", err)
	}
	if _, err := svc.TransicionMasiva(ctx, "e", "u", "ADMIN", AccRevisar, nil, "", ""); err != ErrSinDocumentos {
		t.Errorf("sin ids => %v, want ErrSinDocumentos", err)
	}
	if _, err := svc.TransicionMasiva(ctx, "e", "u", "ADMIN", AccProgramar, []string{"x"}, "", ""); err != ErrFechaPagoRequerida {
		t.Errorf("programar sin fecha => %v, want ErrFechaPagoRequerida", err)
	}
}

// Best-effort: con el fake (CambiarEstado devuelve 0 filas → ErrTransicionInvalida por doc),
// el lote no falla globalmente; cada documento reporta su error y se agregan los conteos.
func TestTransicionMasivaAgrega(t *testing.T) {
	perms := &permisosFalsos{porRol: map[string][]string{"ADMIN": {"cxp.revisar"}}}
	svc := servicioConPermisos(perms)

	res, err := svc.TransicionMasiva(context.Background(), "e", "u", "ADMIN", AccRevisar, []string{"a", "b", "c"}, "", "")
	if err != nil {
		t.Fatalf("TransicionMasiva error inesperado: %v", err)
	}
	if len(res.Resultados) != 3 || res.Fallidos != 3 || res.Exitosos != 0 {
		t.Errorf("agregación = %+v", res)
	}
	for _, r := range res.Resultados {
		if r.OK || r.Error == "" {
			t.Errorf("resultado por doc mal: %+v", r)
		}
	}
}
