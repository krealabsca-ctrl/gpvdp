package cxp

import "context"

// Subclasificaciones lista el 3er nivel del catálogo de gasto (opcional: filtrado por clasificación).
func (s *Service) Subclasificaciones(ctx context.Context, empresaID, clasificacionID string) ([]Subclasificacion, error) {
	return s.repo.ListarSubclasificaciones(ctx, empresaID, clasificacionID)
}

// CrearSubclasificacion da de alta una subclasificación en una clasificación de la empresa.
func (s *Service) CrearSubclasificacion(ctx context.Context, empresaID, clasificacionID, nombre string) (Subclasificacion, error) {
	return s.repo.CrearSubclasificacion(ctx, empresaID, clasificacionID, nombre)
}
