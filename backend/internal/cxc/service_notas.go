package cxc

// Notas de crédito. Las reglas del negocio, tal como las definió el usuario:
// las autoriza el supervisor de piso y NO tienen tope de monto.
//
// Sin tope, el sistema no puede protegerse con un límite, así que se protege con lo único que
// queda: exigir un motivo con contenido, dejar rastro de quién la emitió, y no permitir
// borrarla (solo anularla, devolviendo los cargos a su saldo). El permiso `cxc.notas_credito`
// es lo que representa la autorización, y viene asignado al rol SUPERVISOR_PISO.

import (
	"context"
	"strings"
	"unicode"
)

// motivoMinimo: un motivo de menos de esto no explica nada. No es burocracia — es el único
// dato que va a quedar cuando alguien pregunte, meses después, por qué se bajó esa deuda.
const motivoMinimo = 10

// EmitirNotaCredito valida y emite. El monto no se topa (decisión del usuario), pero el
// motivo sí se exige.
func (s *Service) EmitirNotaCredito(ctx context.Context, empresaID string, in NotaCreditoInput, usuarioID string) (NotaCredito, error) {
	if in.Monto.Sign() <= 0 {
		return NotaCredito{}, ErrMontoInvalido
	}
	in.Motivo = strings.TrimSpace(in.Motivo)
	if !motivoUtil(in.Motivo) {
		return NotaCredito{}, ErrMotivoRequerido
	}
	if in.Fecha == "" {
		in.Fecha = hoyCR().Format("2006-01-02")
	}
	nota, err := s.repo.EmitirNotaCredito(ctx, empresaID, in, usuarioID)
	if err != nil {
		return NotaCredito{}, err
	}
	// La auditoría guarda el monto, el motivo y a qué cargos fue: es el expediente de una
	// operación que baja deuda sin que entre plata.
	s.auditar(ctx, empresaID, "EMITIR_NOTA_CREDITO_CXC", usuarioID, map[string]any{
		"nota": nota.ID, "consecutivo": nota.Consecutivo, "contrato": in.Contrato,
		"monto": in.Monto.String(), "motivo": in.Motivo,
		"aplicaciones": len(nota.Aplicaciones), "sin_aplicar": nota.SinAplicar,
	})
	return nota, nil
}

// AnularNotaCredito deshace la nota y devuelve los cargos a su saldo original.
func (s *Service) AnularNotaCredito(ctx context.Context, empresaID, notaID, motivo, usuarioID string) error {
	motivo = strings.TrimSpace(motivo)
	if !motivoUtil(motivo) {
		return ErrMotivoRequerido
	}
	if err := s.repo.AnularNotaCredito(ctx, empresaID, notaID, motivo, usuarioID); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "ANULAR_NOTA_CREDITO_CXC", usuarioID, map[string]any{
		"nota": notaID, "motivo": motivo,
	})
	return nil
}

// ListarNotas trae las notas del filtro con el resumen de lo condonado y por quién.
func (s *Service) ListarNotas(ctx context.Context, empresaID string, f FiltrosNotas) (ListaNotas, error) {
	return s.repo.ListarNotas(ctx, empresaID, f)
}

// NotaCredito devuelve una nota con sus aplicaciones.
func (s *Service) NotaCredito(ctx context.Context, empresaID, notaID string) (NotaCredito, error) {
	return s.repo.NotaCredito(ctx, empresaID, notaID)
}

// motivoUtil rechaza los motivos que no dicen nada: cortos, o de puro relleno («....», «xxx»).
// Un campo obligatorio que se puede llenar con basura no es un control.
func motivoUtil(m string) bool {
	if len([]rune(m)) < motivoMinimo {
		return false
	}
	distintas := map[rune]bool{}
	letras := 0
	for _, c := range strings.ToLower(m) {
		if unicode.IsLetter(c) {
			letras++
			distintas[c] = true
		}
	}
	// Al menos cinco letras distintas: descarta «aaaaaaaaaa» y «xxxxxxxxxx» sin caer en
	// exigencias absurdas para un texto en español.
	return letras >= 8 && len(distintas) >= 5
}
