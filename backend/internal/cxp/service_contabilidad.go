package cxp

// Facturas «de Contabilidad»: el gasto que no tiene área operativa que lo valide.
//
// Por qué existe: la validación de área (§0027) exige que un validador del departamento asignado
// confirme la factura antes de la aprobación financiera. Pero los honorarios del contador, los
// timbres, las comisiones bancarias, Hacienda o la auditoría no son de ningún área: Contabilidad
// ES quien sabe. Esas facturas se quedaban trancadas, porque el escalamiento solo procede si el
// departamento no tiene validador o la factura está vencida.
//
// Qué se salta y qué NO: se salta la validación OPERATIVA de área. La matriz de firmas por monto
// se aplica igual, y marcar no es aprobar (son dos permisos distintos a propósito: si fueran uno,
// quien decide «esta no necesita validación» sería automáticamente quien la firma).

import (
	"context"
	"strings"
)

// MarcaContabilidad es una entrada marcada del catálogo o del maestro de proveedores.
type MarcaContabilidad struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	// Concepto acompaña a la clasificación: el mismo nombre puede existir en dos rubros.
	Concepto string `json:"concepto,omitempty"`
	// Activo: desactivar un proveedor o un rubro NO le quita la marca. Se listan igual, señalados,
	// porque la excepción sigue vigente para sus facturas abiertas.
	Activo bool `json:"activo"`
}

// MarcasContabilidad es el cuadro completo de lo que hoy está marcado.
type MarcasContabilidad struct {
	Proveedores     []MarcaContabilidad `json:"proveedores"`
	Conceptos       []MarcaContabilidad `json:"conceptos"`
	Clasificaciones []MarcaContabilidad `json:"clasificaciones"`
}

// MarcarDocumentoContabilidad fija el override de UNA factura.
//
//	valor = &true  → es de Contabilidad (motivo obligatorio: queda en auditoría)
//	valor = &false → la valida el área, aunque el proveedor o el rubro estén marcados
//	valor = nil    → vuelve a heredar de proveedor/concepto/clasificación
func (s *Service) MarcarDocumentoContabilidad(ctx context.Context, empresaID, id string, valor *bool, motivo, usuarioID string) (Documento, error) {
	motivo = strings.TrimSpace(motivo)
	// El motivo es obligatorio solo al marcar a mano: es la excepción que hay que poder explicar.
	// Desmarcar o volver a heredar no necesita justificación —devuelve el control al flujo normal.
	if valor != nil && *valor && motivo == "" {
		return Documento{}, ErrMotivoContabilidadRequerido
	}
	if valor == nil || !*valor {
		motivo = "" // no se guarda un motivo que ya no explica nada
	}
	n, err := s.repo.MarcarDocumentoContabilidad(ctx, empresaID, id, valor, motivo, usuarioID)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		// Puede ser que no exista, que sea de otra empresa, o que ya esté aprobada/pagada.
		doc, errDoc := s.repo.DocumentoPorID(ctx, empresaID, id)
		if errDoc != nil {
			return Documento{}, errDoc
		}
		_ = doc
		return Documento{}, ErrContabilidadNoModificable
	}
	s.auditarDocNota(ctx, empresaID, id, "MARCAR_CONTABILIDAD", usuarioID, descripcionMarca(valor, motivo))
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// descripcionMarca redacta lo que queda en la auditoría. Un evento que solo dice «MARCAR» obliga a
// abrir la fila para saber qué pasó.
func descripcionMarca(valor *bool, motivo string) string {
	switch {
	case valor == nil:
		return "vuelve a heredar la marca del proveedor/rubro"
	case *valor:
		return "de Contabilidad (sin validación de área) — " + motivo
	default:
		return "NO es de Contabilidad: la valida el área, aunque el proveedor o el rubro estén marcados"
	}
}

// MarcarProveedorContabilidad marca al proveedor: es la marca que captura el «siempre».
// Retroactiva por diseño — las facturas que ya existen y todavía no se aprobaron pasan a heredarla,
// porque son exactamente las que están trancadas.
func (s *Service) MarcarProveedorContabilidad(ctx context.Context, empresaID, proveedorID string, valor bool, usuarioID string) error {
	n, err := s.repo.MarcarProveedorContabilidad(ctx, empresaID, proveedorID, valor)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrProveedorNoEncontrado
	}
	s.auditar(ctx, empresaID, proveedorID, accionMarca("MARCAR_PROVEEDOR_CONTABILIDAD", valor), usuarioID)
	return nil
}

// MarcarConceptoContabilidad marca un rubro completo.
func (s *Service) MarcarConceptoContabilidad(ctx context.Context, empresaID, conceptoID string, valor bool, usuarioID string) error {
	n, err := s.repo.MarcarConceptoContabilidad(ctx, empresaID, conceptoID, valor)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCatalogoInvalido
	}
	s.auditarEntidad(ctx, empresaID, "concepto", conceptoID, accionMarca("MARCAR_CONCEPTO_CONTABILIDAD", valor), usuarioID)
	return nil
}

// MarcarClasificacionContabilidad marca el nivel fino del catálogo.
func (s *Service) MarcarClasificacionContabilidad(ctx context.Context, empresaID, clasificacionID string, valor bool, usuarioID string) error {
	n, err := s.repo.MarcarClasificacionContabilidad(ctx, empresaID, clasificacionID, valor)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCatalogoInvalido
	}
	s.auditarEntidad(ctx, empresaID, "clasificacion", clasificacionID, accionMarca("MARCAR_CLASIFICACION_CONTABILIDAD", valor), usuarioID)
	return nil
}

// accionMarca separa marcar de desmarcar en la auditoría: son decisiones opuestas y en el histórico
// tienen que poder filtrarse aparte.
func accionMarca(base string, valor bool) string {
	if valor {
		return base
	}
	return "DES" + base
}

// MarcasContabilidad devuelve el cuadro completo de lo marcado hoy.
func (s *Service) MarcasContabilidad(ctx context.Context, empresaID string) (MarcasContabilidad, error) {
	return s.repo.MarcasContabilidad(ctx, empresaID)
}

// AprobarContabilidad es la vía propia para aprobar una factura «de Contabilidad».
//
// Existe como endpoint aparte porque la ruta normal de aprobar está detrás de `cxp.aprobar`, que el
// Supervisor Financiero no tiene —no firma el gasto de las áreas— y el middleware lo bloquearía
// antes de llegar a la regla. Acá el candado es `cxp.aprobar_contabilidad`.
//
// Y verifica que la factura ESTÉ marcada: si no lo hiciera, el permiso de la excepción serviría
// para aprobar cualquier factura salteándose la validación de área, que es justo lo que no se
// quiere. Delega en Aprobar, que vuelve a comprobar el permiso y aplica la matriz de firmas.
func (s *Service) AprobarContabilidad(ctx context.Context, empresaID, id, usuarioID, rol string) (Documento, error) {
	doc, err := s.repo.DocumentoPorID(ctx, empresaID, id)
	if err != nil {
		return Documento{}, err
	}
	if !doc.EsContabilidad {
		return Documento{}, ErrNoEsDeContabilidad
	}
	return s.Aprobar(ctx, empresaID, id, usuarioID, rol)
}
