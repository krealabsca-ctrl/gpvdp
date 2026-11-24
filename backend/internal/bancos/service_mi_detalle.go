package bancos

// El detalle de las dos pantallas, del lado del negocio. El porqué está en mi_detalle.go.

import "context"

// MisPartidasDeArea devuelve el desglose por partida del área, con su serie mensual.
//
// Tres guardas, en orden: sin alcance corta, el área pedida tiene que ser suya, y recién entonces
// se reusa el cálculo de Finanzas. La segunda es la nueva y la que importa: acá el id del área
// viaja en el pedido, así que es lo único que impide pedir el desglose del área de otro.
func (s *Service) MisPartidasDeArea(ctx context.Context, empresaID, usuarioID, desde, hasta, departamentoID, umbral string) ([]GastoPartidaDimension, error) {
	areas, err := s.repo.AreasDelUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return nil, err
	}
	if len(areas) == 0 {
		return nil, ErrSinAlcanceDePresupuesto
	}
	if !EsAreaDeLaPersona(areas, departamentoID) {
		return nil, ErrAreaFueraDeMiAlcance
	}
	return s.PartidasDeDimension(ctx, empresaID, desde, hasta, "departamento", departamentoID, umbral)
}

// MiSerieDeIngreso devuelve el mes a mes de las partidas de la persona.
//
// Trae todas de una vez y no una por una: quien tiene este permiso maneja dos o tres canales de
// cobro, no treinta, y una llamada por partida convertiría abrir la pantalla en cinco viajes.
func (s *Service) MiSerieDeIngreso(ctx context.Context, empresaID, usuarioID, desde, hasta string) (MiIngresoSerie, error) {
	partidas, err := s.repo.PartidasDeIngresoDelUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return MiIngresoSerie{}, err
	}
	if len(partidas) == 0 {
		return MiIngresoSerie{}, ErrSinPartidasDeIngreso
	}

	series, err := s.repo.SeriePorPartida(ctx, empresaID, desde, hasta)
	if err != nil {
		return MiIngresoSerie{}, err
	}
	return MiIngresoSerie{
		Desde: desde, Hasta: hasta,
		Partidas: RecortarSerieAMisPartidas(series, partidas),
	}, nil
}
