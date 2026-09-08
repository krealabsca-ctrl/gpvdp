package inventario

// Reglas del conteo cíclico.

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// ConteoNuevo son los datos para abrir una hoja.
type ConteoNuevo struct {
	SedeID      string `json:"sede_id"`
	CategoriaID string `json:"categoria_id"`
	Fecha       string `json:"fecha"`
	Nota        string `json:"nota"`
}

// CierreConteo es el resultado de cerrar una hoja: dice qué se hizo, no solo que salió bien.
type CierreConteo struct {
	Numero           string `json:"numero"`
	Lineas           int    `json:"lineas"`
	AjustesQueSuman  int    `json:"ajustes_que_suman"`
	AjustesQueRestan int    `json:"ajustes_que_restan"`
	Bajas            int    `json:"bajas"`
	// ImpactoCRC es cuánto cambió el valor del inventario. Negativo = faltaba.
	ImpactoCRC string `json:"impacto_crc"`
	// impacto acumula en decimal mientras se recorre; no viaja al cliente.
	impacto decimal.Decimal
}

// AbrirConteo crea la hoja con la foto del sistema congelada.
func (s *Service) AbrirConteo(ctx context.Context, empresaID string, c ConteoNuevo, usuarioID string) (Conteo, error) {
	if c.SedeID == "" {
		return Conteo{}, ErrSedeRequerida
	}
	if c.Fecha == "" {
		c.Fecha = ahoraCR().Format("2006-01-02")
	}
	if !reFecha.MatchString(c.Fecha) {
		return Conteo{}, ErrFechaInvalida
	}
	hecho, err := s.repo.AbrirConteo(ctx, empresaID, c, usuarioID)
	if err != nil {
		return Conteo{}, err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_conteo", EntidadID: &hecho.ID,
		Accion: "ABRIR_CONTEO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"numero": hecho.Numero, "sede_id": c.SedeID, "categoria_id": c.CategoriaID,
			"lineas": hecho.Lineas,
		},
	})
	return hecho, nil
}

// Conteo trae una hoja con sus líneas y el resumen que decide si se puede cerrar.
func (s *Service) Conteo(ctx context.Context, empresaID, id string) (Conteo, error) {
	c, err := s.repo.ConteoPorID(ctx, empresaID, id)
	if err != nil {
		return Conteo{}, err
	}
	// El resumen se calcula a partir de las MISMAS filas que ve la pantalla: si viniera de una
	// consulta aparte, el encabezado podría decir «se puede cerrar» sobre filas que dicen otra cosa.
	for _, f := range c.Filas {
		switch f.Estado {
		case LineaSinContar:
		case LineaCuadra:
			c.Contadas++
		case LineaExplicada:
			c.Contadas++
			c.ConDiferencia++
		case LineaSinExplicar:
			c.Contadas++
			c.ConDiferencia++
			c.SinExplicar++
		}
	}
	c.Lineas = len(c.Filas)
	c.PuedeCerrarse = c.Estado == ConteoAbierto && c.Contadas == c.Lineas && c.SinExplicar == 0
	return c, nil
}

// Conteos lista las hojas.
func (s *Service) Conteos(ctx context.Context, empresaID, estado, sedeID string) ([]Conteo, error) {
	if estado != "" && estado != ConteoAbierto && estado != ConteoCerrado && estado != ConteoAnulado {
		return nil, fmt.Errorf("inventario: «%s» no es un estado de conteo", estado)
	}
	return s.repo.ListarConteos(ctx, empresaID, estado, sedeID)
}

// GuardarLineaConteo anota lo contado.
//
// Una cantidad negativa no es un dato: es un error de tipeo, y aceptarlo produciría una diferencia
// imposible que después habría que explicar.
func (s *Service) GuardarLineaConteo(ctx context.Context, empresaID, conteoID, lineaID string, contada int, motivo, usuarioID string) error {
	if contada < 0 {
		return fmt.Errorf("%w: lo contado no puede ser negativo", ErrCantidadInvalida)
	}
	return s.repo.GuardarLineaConteo(ctx, empresaID, conteoID, lineaID, contada,
		strings.TrimSpace(motivo), usuarioID)
}

// CerrarConteo cierra la hoja y convierte las diferencias explicadas en ajustes.
func (s *Service) CerrarConteo(ctx context.Context, empresaID, conteoID, fecha, usuarioID string) (CierreConteo, error) {
	if fecha == "" {
		fecha = ahoraCR().Format("2006-01-02")
	}
	if !reFecha.MatchString(fecha) {
		return CierreConteo{}, ErrFechaInvalida
	}
	res, err := s.repo.CerrarConteo(ctx, empresaID, conteoID, fecha, usuarioID)
	if err != nil {
		return CierreConteo{}, err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_conteo", EntidadID: &conteoID,
		Accion: "CERRAR_CONTEO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"numero": res.Numero, "lineas": res.Lineas,
			"ajustes_que_suman": res.AjustesQueSuman, "ajustes_que_restan": res.AjustesQueRestan,
			"bajas": res.Bajas, "impacto_crc": res.ImpactoCRC,
		},
	})
	return res, nil
}

// AnularConteo descarta una hoja sin generar ajustes. Exige motivo: una hoja anulada sin
// explicación deja la duda de si se contó y no cuadró, o si nunca se contó.
func (s *Service) AnularConteo(ctx context.Context, empresaID, conteoID, motivo, usuarioID string) error {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		// Centinela propio y no un wrap de ErrMotivoRequerido: envolverlo producía «un ajuste o una
		// baja necesita motivo: anular un conteo necesita motivo», que dice dos veces lo mismo con
		// palabras que no corresponden a lo que se estaba haciendo.
		return ErrMotivoAnularRequerido
	}
	if err := s.repo.AnularConteo(ctx, empresaID, conteoID, motivo); err != nil {
		return err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_conteo", EntidadID: &conteoID,
		Accion: "ANULAR_CONTEO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"motivo": motivo},
	})
	return nil
}

// PlanDeConteo dice qué toca contar y con cuánta urgencia.
func (s *Service) PlanDeConteo(ctx context.Context, empresaID string) ([]PlanConteo, error) {
	return s.repo.PlanDeConteo(ctx, empresaID)
}
