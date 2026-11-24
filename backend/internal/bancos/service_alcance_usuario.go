package bancos

// El alcance por persona, del lado del negocio. El porqué está en alcance_usuario.go.

import "context"

// AlcanceDeUnUsuario arma la ficha «qué ve» de una persona.
func (s *Service) AlcanceDeUnUsuario(ctx context.Context, empresaID, usuarioID string) (AlcanceDelUsuario, error) {
	nombre, email, existe, err := s.repo.UsuarioDeLaEmpresa(ctx, empresaID, usuarioID)
	if err != nil {
		return AlcanceDelUsuario{}, err
	}
	if !existe {
		return AlcanceDelUsuario{}, ErrUsuarioFueraDeLaEmpresa
	}

	areas, err := s.repo.AreasDelUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return AlcanceDelUsuario{}, err
	}
	partidas, err := s.repo.PartidasDeIngresoDelUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return AlcanceDelUsuario{}, err
	}
	vePresupuesto, veIngreso, err := s.repo.PermisosDeAlcanceDelUsuario(ctx, empresaID, usuarioID)
	if err != nil {
		return AlcanceDelUsuario{}, err
	}

	return AlcanceDelUsuario{
		UsuarioID: usuarioID, Nombre: nombre, Email: email,
		Areas: areas, Partidas: partidas,
		PuedeVerPresupuesto: vePresupuesto, PuedeVerIngreso: veIngreso,
	}, nil
}

// GuardarAreasDeUnUsuario reemplaza las áreas de gasto de la persona.
//
// Comprueba la empresa ANTES de escribir, no después: el repositorio vuelve a comprobarla por
// fila, pero una escritura que llega hasta la base con un usuario ajeno ya es una escritura de
// más, y el orden de las dos llaves importa cuando alguien cambia una de las dos.
func (s *Service) GuardarAreasDeUnUsuario(ctx context.Context, empresaID, usuarioID string, areas []AreaAsignada) error {
	if _, _, existe, err := s.repo.UsuarioDeLaEmpresa(ctx, empresaID, usuarioID); err != nil {
		return err
	} else if !existe {
		return ErrUsuarioFueraDeLaEmpresa
	}

	limpias, err := NormalizarAreas(areas)
	if err != nil {
		return err
	}
	return s.repo.GuardarAreasDelUsuario(ctx, empresaID, usuarioID, limpias)
}

// GuardarPartidasDeIngresoDeUnUsuario reemplaza las partidas de ingreso de la persona.
//
// Rechaza las partidas que NO son de ingreso. Sin esta comprobación, asignar una partida de gasto
// sería aceptado y nunca produciría una línea —el control de ingreso solo mira los conceptos de
// naturaleza INGRESO—, así que la persona vería su partida nombrada arriba y una tabla vacía
// debajo, sin ningún error que explique por qué.
func (s *Service) GuardarPartidasDeIngresoDeUnUsuario(ctx context.Context, empresaID, usuarioID string, ids []string) error {
	if _, _, existe, err := s.repo.UsuarioDeLaEmpresa(ctx, empresaID, usuarioID); err != nil {
		return err
	} else if !existe {
		return ErrUsuarioFueraDeLaEmpresa
	}

	limpias := SinRepetidos(ids)
	if len(limpias) > 0 {
		naturalezas, err := s.repo.NaturalezasDeClasificaciones(ctx, empresaID, limpias)
		if err != nil {
			return err
		}
		for _, id := range limpias {
			nat, conocida := naturalezas[id]
			if !conocida {
				return ErrPartidaDesconocida
			}
			if nat != "INGRESO" {
				return ErrPartidaNoEsDeIngreso
			}
		}
	}
	return s.repo.GuardarPartidasDeIngresoDelUsuario(ctx, empresaID, usuarioID, limpias)
}
