package bancos

// La mitad de ingreso, del lado del negocio. El porqué está en mi_ingreso.go.

import "context"

// MiIngresoDePartidas arma el control de ingreso recortado a las partidas de la persona.
//
// Las dos guardas, en el mismo orden que en la mitad de gasto: corte temprano sin alcance, y
// recorte explícito de las líneas. Y el MISMO cálculo que ve Finanzas —`ControlDeIngreso`—, no
// uno paralelo: dos cálculos para la misma pregunta terminan dando dos números.
func (s *Service) MiIngresoDePartidas(ctx context.Context, empresaID, usuarioID, desde, hasta string) (MiIngreso, error) {
	partidas, err := s.repo.PartidasDeIngresoDelUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return MiIngreso{}, err
	}
	if len(partidas) == 0 {
		return MiIngreso{}, ErrSinPartidasDeIngreso
	}

	ctrl, err := s.ControlDeIngreso(ctx, empresaID, desde, hasta)
	if err != nil {
		return MiIngreso{}, err
	}
	return RecortarAMisPartidas(ctrl, partidas), nil
}
