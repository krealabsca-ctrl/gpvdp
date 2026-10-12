package cxp

// Administración de la nomenclatura interna del pago.

import "context"

// CatalogoNomenclaturas lista el catálogo de gasto con el código de cada rubro.
func (s *Service) CatalogoNomenclaturas(ctx context.Context, empresaID string) ([]RubroConCodigo, error) {
	return s.repo.CatalogoConCodigos(ctx, empresaID)
}

// AsignarCodigoPago pone o quita el código de un rubro del catálogo.
//
// Valida la FORMA acá y deja la unicidad a la base: el índice único es la única garantía real
// contra dos personas asignando el mismo código al mismo tiempo. Verificar antes con un SELECT
// daría una ventana entre la consulta y el UPDATE, y el código de un rubro no es un dato que
// convenga tener duplicado ni por un instante.
func (s *Service) AsignarCodigoPago(ctx context.Context, empresaID, nivel, id, codigo, usuarioID string) error {
	if !NivelCatalogoValido(nivel) {
		return ErrCatalogoInvalido
	}
	limpio, err := ValidarCodigoPago(codigo)
	if err != nil {
		return err
	}
	if err := s.repo.AsignarCodigoPago(ctx, empresaID, nivel, id, limpio); err != nil {
		return err
	}
	// Queda en auditoría: el código es un acuerdo interno que después se usa para leer el banco,
	// así que quién lo cambió y cuándo importa tanto como el valor.
	s.auditarDoc(ctx, empresaID, id, "ASIGNAR_CODIGO_PAGO", usuarioID)
	return nil
}
