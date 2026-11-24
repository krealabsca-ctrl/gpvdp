package cxp

// El catálogo de prioridades, del lado del negocio. El porqué está en prioridad.go.

import (
	"context"
	"fmt"
	"strings"
)

// Prioridades lista el catálogo de la empresa.
func (s *Service) Prioridades(ctx context.Context, empresaID string, soloActivas bool) ([]Prioridad, error) {
	return s.repo.Prioridades(ctx, empresaID, soloActivas)
}

// CrearPrioridad agrega una prioridad al catálogo.
func (s *Service) CrearPrioridad(ctx context.Context, empresaID string, in PrioridadInput, usuarioID string) (Prioridad, error) {
	if err := in.limpiar(); err != nil {
		return Prioridad{}, err
	}
	p, err := s.repo.CrearPrioridad(ctx, empresaID, in)
	if err != nil {
		return Prioridad{}, err
	}
	s.auditarPrioridad(ctx, empresaID, p, "CREAR_PRIORIDAD", usuarioID)
	return p, nil
}

// ActualizarPrioridad edita una existente. El código no cambia (ver repositorio).
func (s *Service) ActualizarPrioridad(ctx context.Context, empresaID, id string, in PrioridadInput, usuarioID string) (Prioridad, error) {
	if err := in.limpiar(); err != nil {
		return Prioridad{}, err
	}
	p, err := s.repo.ActualizarPrioridad(ctx, empresaID, id, in)
	if err != nil {
		return Prioridad{}, err
	}
	s.auditarPrioridad(ctx, empresaID, p, "EDITAR_PRIORIDAD", usuarioID)
	return p, nil
}

// EliminarPrioridad la borra si no cuelga nada; si cuelga, el repositorio lo dice.
func (s *Service) EliminarPrioridad(ctx context.Context, empresaID, id, usuarioID string) error {
	if err := s.repo.EliminarPrioridad(ctx, empresaID, id); err != nil {
		return err
	}
	s.auditarPrioridad(ctx, empresaID, Prioridad{ID: id}, "ELIMINAR_PRIORIDAD", usuarioID)
	return nil
}

// validarPrioridad comprueba que el código exista en el catálogo ACTIVO de la empresa.
//
// ESTO REEMPLAZA AL CHECK QUE LA MIG 0121 QUITÓ. Sin esta comprobación, marcar una factura con un
// código inventado la dejaría con una prioridad que ninguna pantalla sabe nombrar ni colorear, y
// que el orden de la Bandeja manda al fondo junto con las que nadie priorizó.
//
// El vacío SIEMPRE es válido: es «sin prioridad», y quitarle la prioridad a una factura tiene que
// poder hacerse aunque el catálogo quede sin ninguna activa.
func (s *Service) validarPrioridad(ctx context.Context, empresaID, codigo string) error {
	if strings.TrimSpace(codigo) == "" {
		return nil
	}
	validos, err := s.repo.CodigosDePrioridadValidos(ctx, empresaID)
	if err != nil {
		return err
	}
	for _, v := range validos {
		if v == codigo {
			return nil
		}
	}
	// El error NOMBRA las válidas: es lo que un CHECK nunca pudo responder, y lo que convierte un
	// «valor inválido» en algo que se puede arreglar sin preguntarle a nadie.
	if len(validos) == 0 {
		return fmt.Errorf("%w: esta empresa no tiene prioridades configuradas", ErrPrioridadNoEncontrada)
	}
	return fmt.Errorf("%w: las de esta empresa son %s", ErrPrioridadNoEncontrada, strings.Join(validos, ", "))
}

func (s *Service) auditarPrioridad(ctx context.Context, empresaID string, p Prioridad, accion, usuarioID string) {
	s.auditarDoc(ctx, empresaID, p.ID, accion, usuarioID)
}
