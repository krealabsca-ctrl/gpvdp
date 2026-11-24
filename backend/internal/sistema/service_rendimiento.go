package sistema

// Las consultas lentas, del lado del negocio. El porqué está en rendimiento.go.

import (
	"context"
	"errors"
)

// ServiceRendimiento lee el contador de consultas del motor.
//
// Va aparte de `Service` por la misma razón que su repositorio: los incidentes son una tabla de
// este sistema y esto es una extensión de PostgreSQL. Lo único que comparten es la pantalla donde
// se miran.
type ServiceRendimiento struct {
	repo RepositoryRendimiento
}

// NewServiceRendimiento construye el servicio.
func NewServiceRendimiento(repo RepositoryRendimiento) *ServiceRendimiento {
	return &ServiceRendimiento{repo: repo}
}

// motivoNoDisponible es lo que la pantalla le muestra a quien administra el servidor.
//
// Dice QUÉ hacer, no solo que no se puede: quien abre esta pantalla es justamente quien tiene
// acceso al servidor para arreglarlo, y un «no disponible» a secas lo deja sin camino.
const motivoNoDisponible = "El contador de consultas no está activo en este servidor. " +
	"Se enciende arrancando PostgreSQL con shared_preload_libraries=pg_stat_statements " +
	"(ya viene así en deploy/docker-compose.prod.yml) y reiniciando la base."

// ConsultasLentas devuelve el top por tiempo total.
//
// NO devuelve error cuando la extensión no está: devuelve `Disponible: false` con el motivo. Un
// error ahí pintaría la pantalla de rojo como si algo se hubiera roto, cuando lo que pasa es que
// una herramienta opcional no está encendida.
func (s *ServiceRendimiento) ConsultasLentas(ctx context.Context, limite int) (RendimientoConsultas, error) {
	consultas, desde, err := s.repo.ConsultasLentas(ctx, acotarTop(limite))
	if errors.Is(err, ErrRendimientoNoDisponible) {
		return RendimientoConsultas{
			Disponible: false,
			Motivo:     motivoNoDisponible,
			Consultas:  []ConsultaLenta{},
		}, nil
	}
	if err != nil {
		return RendimientoConsultas{}, err
	}
	if consultas == nil {
		consultas = []ConsultaLenta{}
	}
	return RendimientoConsultas{
		Disponible:    true,
		Consultas:     consultas,
		MidiendoDesde: desde,
	}, nil
}

// ReiniciarContadores pone los acumulados en cero para medir una ventana concreta.
func (s *ServiceRendimiento) ReiniciarContadores(ctx context.Context) error {
	return s.repo.ReiniciarContadores(ctx)
}
