package bancos

// Fase A — motor que aprende: el servicio orquesta la sugerencia de reglas tras una
// corrección manual, la gestión de reglas y la clasificación masiva.

import (
	"context"

	"github.com/gpvdp/erp/internal/shared"
)

// MovClasifActual es un movimiento con su clasificación vigente (para sugerir una regla).
type MovClasifActual struct {
	ID              string
	Descripcion     string
	EsDebito        bool
	ConceptoID      string
	Concepto        string
	ClasificacionID string
	Clasificacion   string
}

// SugerenciaRegla es la propuesta de aprendizaje tras clasificar a mano un movimiento:
// "¿Crear la regla «palabra» → concepto/clasificación? Clasificaría N similares."
type SugerenciaRegla struct {
	Sugerible       bool   `json:"sugerible"`
	Motivo          string `json:"motivo,omitempty"` // por qué no hay sugerencia
	PalabraClave    string `json:"palabra_clave"`
	AplicaA         string `json:"aplica_a"`
	ConceptoID      string `json:"concepto_id"`
	Concepto        string `json:"concepto"`
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	Similares       int    `json:"similares"`
	NombreSugerido  string `json:"nombre_sugerido"`
}

// CambiosRegla es la edición parcial de una regla (solo lo presente se toca).
type CambiosRegla struct {
	Prioridad       *int
	Activo          *bool
	AgregarPalabras []string
	QuitarPalabras  []string
}

// ResumenClasif alimenta el KPI de auto-clasificación (meta: ≥90% sin tocar).
type ResumenClasif struct {
	Total           int `json:"total"`
	NoIdentificados int `json:"no_identificados"`
	Auto            int `json:"auto"`
	Revisados       int `json:"revisados"`
	Traslados       int `json:"traslados"`
}

// SugerenciaRegla propone una regla a partir de un movimiento ya clasificado a mano:
// extrae la palabra clave específica de la descripción y cuenta cuántos NO_IDENTIFICADO
// similares clasificaría. La regla solo se crea si el usuario acepta (1 clic en la UI).
func (s *Service) SugerenciaRegla(ctx context.Context, empresaID, movID string) (SugerenciaRegla, error) {
	m, err := s.repo.MovimientoClasif(ctx, empresaID, movID)
	if err != nil {
		return SugerenciaRegla{}, err
	}
	if m.ConceptoID == "" || m.ClasificacionID == "" {
		return SugerenciaRegla{Sugerible: false, Motivo: "el movimiento aún no tiene clasificación"}, nil
	}
	palabra := ExtraerPalabraClave(m.Descripcion)
	if palabra == "" {
		return SugerenciaRegla{Sugerible: false, Motivo: "la descripción no tiene una palabra clave útil (solo códigos o vocabulario genérico)"}, nil
	}
	if existe, err := s.repo.ExisteReglaConPalabra(ctx, empresaID, palabra); err != nil {
		return SugerenciaRegla{}, err
	} else if existe {
		return SugerenciaRegla{Sugerible: false, Motivo: "ya existe una regla con la palabra «" + palabra + "»"}, nil
	}
	aplicaA := "CREDITO"
	if m.EsDebito {
		aplicaA = "DEBITO"
	}
	similares, err := s.repo.ContarNoIdentificadosConPalabra(ctx, empresaID, palabra, aplicaA)
	if err != nil {
		return SugerenciaRegla{}, err
	}
	return SugerenciaRegla{
		Sugerible:       true,
		PalabraClave:    palabra,
		AplicaA:         aplicaA,
		ConceptoID:      m.ConceptoID,
		Concepto:        m.Concepto,
		ClasificacionID: m.ClasificacionID,
		Clasificacion:   m.Clasificacion,
		Similares:       similares,
		NombreSugerido:  palabra + " → " + m.Clasificacion,
	}, nil
}

// ActualizarRegla edita prioridad/activo/palabras de una regla del motor.
func (s *Service) ActualizarRegla(ctx context.Context, empresaID, reglaID string, cambios CambiosRegla, usuarioID string) error {
	if err := s.repo.ActualizarRegla(ctx, empresaID, reglaID, cambios); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "regla_clasificacion", EntidadID: &reglaID,
		Accion: "EDITAR_REGLA", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"prioridad": cambios.Prioridad, "activo": cambios.Activo,
			"agregar": cambios.AgregarPalabras, "quitar": cambios.QuitarPalabras,
		},
	})
	return nil
}

// EliminarRegla borra una regla del motor (los movimientos ya clasificados no se tocan).
func (s *Service) EliminarRegla(ctx context.Context, empresaID, reglaID, usuarioID string) error {
	if err := s.repo.EliminarRegla(ctx, empresaID, reglaID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "regla_clasificacion", EntidadID: &reglaID,
		Accion: "ELIMINAR_REGLA", UsuarioID: &usuarioID,
	})
	return nil
}

// ClasificarMasivo asigna concepto/clasificación a un bloque de movimientos en un solo golpe.
func (s *Service) ClasificarMasivo(ctx context.Context, empresaID string, movIDs []string, conceptoID, clasificacionID, usuarioID string) (int, error) {
	n, err := s.repo.ClasificarMasivo(ctx, empresaID, movIDs, conceptoID, clasificacionID)
	if err != nil {
		return 0, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "movimiento_bancario",
		Accion: "CLASIFICAR_MASIVO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{"movimientos": len(movIDs), "clasificados": n, "clasificacion_id": clasificacionID},
	})
	return n, nil
}

// ResumenClasificacion devuelve los conteos por estado para el KPI de auto-clasificación.
func (s *Service) ResumenClasificacion(ctx context.Context, empresaID, periodo string) (ResumenClasif, error) {
	return s.repo.ResumenClasificacion(ctx, empresaID, periodo)
}
