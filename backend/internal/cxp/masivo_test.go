package cxp

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestRolPuedeAccion(t *testing.T) {
	casos := []struct {
		rol, accion string
		want        bool
	}{
		{"SUPERVISOR_FINANCIERO", AccRevisar, true},
		{"SUPERVISOR_FINANCIERO", AccPagar, false},  // pagar es solo DIRECTOR/ADMIN
		{"AUXILIAR_FINANCIERO", AccRevisar, true},   // revisar es su trabajo diario (2026-08-14)
		{"AUXILIAR_FINANCIERO", AccAprobar, false},  // pero no firma
		{"AUXILIAR_FINANCIERO", AccPagar, false},    // ni paga
		{"GERENCIA_GENERAL", AccAprobar, true},      // gerencia aprueba
		{"GERENCIA_GENERAL", AccProgramar, false},   // pero no programa
		{"DIRECTOR_FINANCIERO", AccConciliar, true}, //
		{"ADMIN", AccProgramar, true},               //
	}
	for _, c := range casos {
		if got := rolPuedeAccion(c.rol, c.accion); got != c.want {
			t.Errorf("rolPuedeAccion(%q,%q)=%v want %v", c.rol, c.accion, got, c.want)
		}
	}
}

func TestTransicionMasivaValidaciones(t *testing.T) {
	svc := NewService(&fakeRepo{}, nil, zap.NewNop())
	ctx := context.Background()

	if _, err := svc.TransicionMasiva(ctx, "e", "u", "ADMIN", "borrar", []string{"x"}, "", ""); err != ErrAccionInvalida {
		t.Errorf("acción inválida => %v, want ErrAccionInvalida", err)
	}
	if _, err := svc.TransicionMasiva(ctx, "e", "u", "ADMIN", AccRevisar, nil, "", ""); err != ErrSinDocumentos {
		t.Errorf("sin ids => %v, want ErrSinDocumentos", err)
	}
	if _, err := svc.TransicionMasiva(ctx, "e", "u", "SUPERVISOR_FINANCIERO", AccPagar, []string{"x"}, "", ""); err != ErrRolNoAutorizado {
		t.Errorf("rol no autorizado => %v, want ErrRolNoAutorizado", err)
	}
	if _, err := svc.TransicionMasiva(ctx, "e", "u", "ADMIN", AccProgramar, []string{"x"}, "", ""); err != ErrFechaPagoRequerida {
		t.Errorf("programar sin fecha => %v, want ErrFechaPagoRequerida", err)
	}
}

// Best-effort: con el fake (CambiarEstado devuelve 0 filas → ErrTransicionInvalida por doc),
// el lote no falla globalmente; cada documento reporta su error y se agregan los conteos.
func TestTransicionMasivaAgrega(t *testing.T) {
	svc := NewService(&fakeRepo{}, nil, zap.NewNop())
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
