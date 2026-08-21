package plantillas

// Servicio de plantillas: el texto vigente de cada notificación, su edición y el armado del
// correo final que los módulos van a enviar.

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gpvdp/erp/internal/shared"
)

// Service es la lógica de las plantillas.
type Service struct {
	repo  Repository
	audit *shared.Audit
}

// NewService construye el servicio.
func NewService(repo Repository, audit *shared.Audit) *Service {
	return &Service{repo: repo, audit: audit}
}

// Listar devuelve todos los tipos con su texto vigente (personalizado o de fábrica).
func (s *Service) Listar(ctx context.Context, empresaID string) ([]TipoConPlantilla, error) {
	guardadas, err := s.repo.Listar(ctx, empresaID)
	if err != nil {
		return nil, err
	}
	out := make([]TipoConPlantilla, 0, len(Catalogo))
	for _, t := range Catalogo {
		out = append(out, TipoConPlantilla{Tipo: t, Vigente: vigente(t, guardadas)})
	}
	return out, nil
}

// vigente resuelve qué texto rige: el guardado por la empresa o el de fábrica.
func vigente(t Tipo, guardadas map[string]Plantilla) Plantilla {
	if p, ok := guardadas[t.Clave]; ok {
		return p
	}
	return Plantilla{Clave: t.Clave, Asunto: t.AsuntoDefault, Cuerpo: t.CuerpoDefault}
}

// Vigente devuelve el texto que rige para un tipo (lo usan CxP y Nómina al enviar).
func (s *Service) Vigente(ctx context.Context, empresaID, clave string) (Tipo, Plantilla, error) {
	t, ok := TipoPorClave(clave)
	if !ok {
		return Tipo{}, Plantilla{}, ErrTipoDesconocido
	}
	guardadas, err := s.repo.Listar(ctx, empresaID)
	if err != nil {
		return Tipo{}, Plantilla{}, err
	}
	return t, vigente(t, guardadas), nil
}

// Armar devuelve el asunto y el cuerpo listos para enviar, con las variables reemplazadas.
//
// Es LO ÚNICO que usan los módulos al enviar: si la empresa no personalizó nada, sale el texto
// de fábrica; si lo personalizó, sale el suyo. Un valor que falte queda vacío, nunca «[VAR]».
func (s *Service) Armar(ctx context.Context, empresaID, clave string, valores map[string]string) (string, string, error) {
	_, p, err := s.Vigente(ctx, empresaID, clave)
	if err != nil {
		return "", "", err
	}
	s.completarComunes(ctx, empresaID, valores)
	return Render(p.Asunto, valores), Render(p.Cuerpo, valores), nil
}

// completarComunes llena las variables que traen todos los tipos (empresa y año) para que
// ningún módulo tenga que acordarse de ellas. No pisa lo que el módulo ya haya puesto.
func (s *Service) completarComunes(ctx context.Context, empresaID string, valores map[string]string) {
	if valores["NOMBRE_EMPRESA"] == "" {
		if nombre, err := s.repo.NombreEmpresa(ctx, empresaID); err == nil {
			valores["NOMBRE_EMPRESA"] = nombre
		}
	}
	if valores["ANIO"] == "" {
		valores["ANIO"] = strconv.Itoa(time.Now().UTC().Add(-6 * time.Hour).Year())
	}
}

// Guardar valida y persiste la plantilla de la empresa.
//
// Rechaza variables que el sistema no sabe llenar: es mejor un error al guardar que un correo
// con «[FOO]» en la cara del proveedor.
func (s *Service) Guardar(ctx context.Context, empresaID, clave, asunto, cuerpo, usuarioID string) ([]string, error) {
	t, ok := TipoPorClave(clave)
	if !ok {
		return nil, ErrTipoDesconocido
	}
	asunto, cuerpo = strings.TrimSpace(asunto), strings.TrimSpace(cuerpo)
	if asunto == "" {
		return nil, ErrAsuntoVacio
	}
	if cuerpo == "" {
		return nil, ErrCuerpoVacio
	}
	if desconocidas := t.Desconocidas(asunto, cuerpo); len(desconocidas) > 0 {
		return desconocidas, ErrVariablesDesconocidas
	}
	if err := s.repo.Guardar(ctx, empresaID, clave, asunto, cuerpo, usuarioID); err != nil {
		return nil, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "plantilla_correo", Accion: "GUARDAR_PLANTILLA",
		UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"clave": clave, "asunto": asunto, "variables": VariablesUsadas(asunto, cuerpo),
		},
	})
	return nil, nil
}

// Restablecer devuelve la plantilla al texto de fábrica.
func (s *Service) Restablecer(ctx context.Context, empresaID, clave, usuarioID string) error {
	if _, ok := TipoPorClave(clave); !ok {
		return ErrTipoDesconocido
	}
	if err := s.repo.Restablecer(ctx, empresaID, clave); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "plantilla_correo", Accion: "RESTABLECER_PLANTILLA",
		UsuarioID: &usuarioID, ValorNuevo: map[string]string{"clave": clave},
	})
	return nil
}

// VistaPrevia arma el correo con valores de ejemplo, sin guardar nada. Recibe el texto que el
// usuario tiene en pantalla (todavía sin guardar), que es justo lo que quiere ver.
func (s *Service) VistaPrevia(ctx context.Context, empresaID, clave, asunto, cuerpo string) (string, string, []string, error) {
	t, ok := TipoPorClave(clave)
	if !ok {
		return "", "", nil, ErrTipoDesconocido
	}
	if strings.TrimSpace(asunto) == "" {
		asunto = t.AsuntoDefault
	}
	if strings.TrimSpace(cuerpo) == "" {
		cuerpo = t.CuerpoDefault
	}
	// La vista previa usa el nombre REAL de la empresa activa: así se ve el correo que va a
	// salir, no uno de mentira.
	valores := t.Ejemplos()
	valores["NOMBRE_EMPRESA"] = ""
	s.completarComunes(ctx, empresaID, valores)
	return Render(asunto, valores), Render(cuerpo, valores), t.Desconocidas(asunto, cuerpo), nil
}
