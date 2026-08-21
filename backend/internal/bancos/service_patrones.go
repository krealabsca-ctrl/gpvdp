package bancos

// Servicio del descubridor de patrones: junta los datos y delega el criterio a la función pura.

import "context"

// limitePatronesDefault es cuántos grupos se devuelven; más que esto no se atiende de una
// sentada y la lista deja de ser accionable.
const limitePatronesDefault = 25

// Patrones devuelve los grupos de movimientos sin clasificar que valen una regla, del más
// grande al más chico. Período vacío = toda la empresa.
func (s *Service) Patrones(ctx context.Context, empresaID, periodo string, limite int) ([]PatronSugerido, error) {
	if limite <= 0 {
		limite = limitePatronesDefault
	}
	lineas, err := s.repo.LineasSinClasificar(ctx, empresaID, periodo)
	if err != nil {
		return nil, err
	}
	if len(lineas) == 0 {
		return []PatronSugerido{}, nil
	}
	todas, err := s.repo.DescripcionesEmpresa(ctx, empresaID)
	if err != nil {
		return nil, err
	}
	return AgruparPatrones(lineas, todas, limite), nil
}
