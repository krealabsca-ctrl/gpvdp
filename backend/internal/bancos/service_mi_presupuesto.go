package bancos

// «Mi presupuesto», del lado del negocio. El porqué está en mi_presupuesto.go.

import "context"

// MiPresupuestoDeArea arma el control recortado a las áreas de la persona.
//
// LAS DOS GUARDAS, en este orden:
//
//  1. Sin áreas, CORTA con error antes de tocar ningún número. Es lo que impide que un alcance
//     vacío se convierta en «sin filtro».
//  2. `RecortarAMisAreas` deja fuera todo lo que no sea suyo, incluido el gasto sin dueño.
//
// La segunda sola bastaría hoy, pero la primera es la que sobrevive a que alguien reordene esto
// dentro de seis meses: un corte temprano es difícil de borrar por accidente, un filtro en medio
// de una función larga no.
func (s *Service) MiPresupuestoDeArea(ctx context.Context, empresaID, usuarioID, desde, hasta, umbral string) (MiPresupuesto, error) {
	areas, err := s.repo.AreasDelUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return MiPresupuesto{}, err
	}
	if len(areas) == 0 {
		return MiPresupuesto{}, ErrSinAlcanceDePresupuesto
	}

	// EL MISMO cálculo que ve Finanzas, no uno paralelo: dos cálculos para la misma pregunta
	// terminan dando dos números distintos, y ahí la pantalla deja de usarse.
	ctrl, err := s.ControlPresupuestario(ctx, empresaID, desde, hasta, AgruparPorDepartamento, umbral)
	if err != nil {
		return MiPresupuesto{}, err
	}
	return RecortarAMisAreas(ctrl, areas), nil
}
