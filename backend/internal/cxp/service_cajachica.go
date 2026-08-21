package cxp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// puedeVerTodasLasCajas: Contabilidad/DF (cxp.ver_todo) y quien administra fondos ven las 13;
// un custodio (rol a medida sin esos permisos) solo ve/opera SU fondo.
func (s *Service) puedeVerTodasLasCajas(ctx context.Context, empresaID, rol string) (bool, error) {
	if s.perms == nil {
		return true, nil // sin checker (tests): sin scoping
	}
	if ok, err := s.perms.Tiene(ctx, empresaID, rol, permisoVerTodo); err != nil || ok {
		return ok, err
	}
	return s.perms.Tiene(ctx, empresaID, rol, "cxp.caja_administrar")
}

// ListarFondos devuelve los fondos visibles para el usuario (todos, o solo el suyo).
func (s *Service) ListarFondos(ctx context.Context, empresaID, rol, usuarioID string) ([]FondoCajaChica, error) {
	todas, err := s.puedeVerTodasLasCajas(ctx, empresaID, rol)
	if err != nil {
		return nil, err
	}
	custodio := ""
	if !todas {
		custodio = usuarioID
	}
	return s.repo.ListarFondos(ctx, empresaID, custodio)
}

// fondoOperable trae el fondo y verifica que el usuario pueda operarlo (custodio o Conta/DF).
func (s *Service) fondoOperable(ctx context.Context, empresaID, fondoID, rol, usuarioID string) (FondoCajaChica, error) {
	f, err := s.repo.FondoPorID(ctx, empresaID, fondoID)
	if err != nil {
		return FondoCajaChica{}, err
	}
	todas, err := s.puedeVerTodasLasCajas(ctx, empresaID, rol)
	if err != nil {
		return FondoCajaChica{}, err
	}
	if !todas && f.CustodioID != usuarioID {
		return FondoCajaChica{}, ErrNoEsCustodio
	}
	return f, nil
}

// CrearFondo constituye un fondo (decisión del DF: monto, umbral, límite, custodio).
func (s *Service) CrearFondo(ctx context.Context, empresaID string, in FondoInput, usuarioID string) (FondoCajaChica, error) {
	if strings.TrimSpace(in.Nombre) == "" || !in.MontoAsignado.IsPositive() {
		return FondoCajaChica{}, ErrFondoNoEncontrado // el handler valida antes; red de seguridad
	}
	f, err := s.repo.CrearFondo(ctx, empresaID, in)
	if err != nil {
		return FondoCajaChica{}, err
	}
	s.auditarCaja(ctx, empresaID, f.ID, "CREAR_FONDO_CAJA", usuarioID,
		fmt.Sprintf("%s · fondo ₡%s", f.Nombre, in.MontoAsignado.StringFixed(2)))
	return f, nil
}

// ActualizarFondo edita los parámetros del fondo.
func (s *Service) ActualizarFondo(ctx context.Context, empresaID, id string, in FondoInput, usuarioID string) (FondoCajaChica, error) {
	f, err := s.repo.ActualizarFondo(ctx, empresaID, id, in)
	if err != nil {
		return FondoCajaChica{}, err
	}
	s.auditarCaja(ctx, empresaID, id, "ACTUALIZAR_FONDO_CAJA", usuarioID, f.Nombre)
	return f, nil
}

// DesactivarFondo apaga el fondo (histórico intacto).
func (s *Service) DesactivarFondo(ctx context.Context, empresaID, id, usuarioID string) error {
	if err := s.repo.DesactivarFondo(ctx, empresaID, id); err != nil {
		return err
	}
	s.auditarCaja(ctx, empresaID, id, "DESACTIVAR_FONDO_CAJA", usuarioID, "")
	return nil
}

// ListarVales devuelve los vales del fondo (si el usuario puede ver ese fondo).
func (s *Service) ListarVales(ctx context.Context, empresaID, fondoID, rol, usuarioID string) ([]ValeCajaChica, error) {
	if _, err := s.fondoOperable(ctx, empresaID, fondoID, rol, usuarioID); err != nil {
		return nil, err
	}
	return s.repo.ListarVales(ctx, empresaID, fondoID)
}

// CrearVale registra un gasto menor contra el fondo, con las guardas de la maqueta:
// detalle y gasto obligatorios, límite por vale bloqueante y fondo suficiente.
func (s *Service) CrearVale(ctx context.Context, empresaID, fondoID string, in ValeInput, rol, usuarioID string) (string, error) {
	f, err := s.fondoOperable(ctx, empresaID, fondoID, rol, usuarioID)
	if err != nil {
		return "", err
	}
	if !f.Activo {
		return "", ErrFondoInactivo
	}
	if strings.TrimSpace(in.Detalle) == "" {
		return "", ErrValeDetalleRequerido
	}
	if in.ConceptoID == "" || in.ClasificacionID == "" {
		return "", ErrValeGastoRequerido
	}
	if !in.MontoCRC.IsPositive() {
		return "", ErrFondoInsuficiente
	}
	if in.Comprobante != "FE" {
		in.Comprobante = "RECIBO"
	}
	limite, _ := decimal.NewFromString(f.LimiteVale)
	if limite.IsPositive() && in.MontoCRC.GreaterThan(limite) {
		return "", ErrValeSobreLimite
	}
	disponible, _ := decimal.NewFromString(f.Disponible)
	if in.MontoCRC.GreaterThan(disponible) {
		return "", ErrFondoInsuficiente
	}
	id, err := s.repo.CrearVale(ctx, empresaID, fondoID, in, usuarioID)
	if err != nil {
		return "", err
	}
	s.auditarCaja(ctx, empresaID, fondoID, "REGISTRAR_VALE_CAJA", usuarioID,
		fmt.Sprintf("%s · ₡%s · %s", in.Detalle, in.MontoCRC.StringFixed(2),
			map[bool]string{true: "factura electrónica", false: "recibo manual (no deducible)"}[in.Comprobante == "FE"]))
	return id, nil
}

// AnularVale marca un vale como anulado (error de digitación); nunca borra.
func (s *Service) AnularVale(ctx context.Context, empresaID, fondoID, valeID, rol, usuarioID string) error {
	if _, err := s.fondoOperable(ctx, empresaID, fondoID, rol, usuarioID); err != nil {
		return err
	}
	if err := s.repo.AnularVale(ctx, empresaID, fondoID, valeID); err != nil {
		return err
	}
	s.auditarCaja(ctx, empresaID, fondoID, "ANULAR_VALE_CAJA", usuarioID, valeID)
	return nil
}

// GenerarReposicion agrupa los vales pendientes en UN documento CxP tipo REINTEGRO pagadero
// al custodio (proveedor interno del fondo). El documento viaja por el flujo normal
// (validación del jefe de área + matriz de firmas + corrida) y, al pagarse, los vales quedan
// repuestos y el fondo restaurado (estado derivado, ver repository_cajachica.go).
func (s *Service) GenerarReposicion(ctx context.Context, empresaID, fondoID, rol, usuarioID string) (Documento, error) {
	f, err := s.fondoOperable(ctx, empresaID, fondoID, rol, usuarioID)
	if err != nil {
		return Documento{}, err
	}
	if !f.Activo {
		return Documento{}, ErrFondoInactivo
	}
	if f.ProveedorID == "" {
		return Documento{}, ErrFondoSinProveedor
	}
	valeIDs, total, err := s.repo.ValesElegiblesReposicion(ctx, empresaID, fondoID)
	if err != nil {
		return Documento{}, err
	}
	if len(valeIDs) == 0 || !total.IsPositive() {
		return Documento{}, ErrSinValesPendientes
	}
	doc, err := s.CrearDocumento(ctx, empresaID, DocumentoInput{
		ProveedorID:  f.ProveedorID,
		Clave:        "", // sin factura electrónica: referencia interna autogenerada
		FechaEmision: time.Now().Format("2006-01-02"),
		Moneda:       "CRC",
		Total:        total,
		Tipo:         TipoReintegro,
		Descripcion:  fmt.Sprintf("Reposición caja chica «%s» — %d vales", f.Nombre, len(valeIDs)),
	}, usuarioID)
	if err != nil {
		return Documento{}, err
	}
	// El documento hereda el departamento del FONDO (para que valide el jefe de esa área).
	if f.DepartamentoID != "" {
		if _, err := s.repo.AsignarDepartamentoDoc(ctx, empresaID, doc.ID, f.DepartamentoID); err != nil {
			s.log.Warn("cxp: no se pudo asignar el departamento del fondo a la reposición")
		}
	}
	vinculados, err := s.repo.VincularValesAReposicion(ctx, empresaID, fondoID, doc.ID, valeIDs)
	if err != nil {
		return Documento{}, err
	}
	// Carrera de doble-generación: la guarda del UPDATE evita duplicar vales; si otro proceso
	// se adelantó, este documento queda sin vales y se reporta para que Conta lo anule.
	if vinculados == 0 {
		s.log.Warn("cxp: reposición sin vales vinculados (posible doble generación)")
	}
	s.auditarCaja(ctx, empresaID, fondoID, "GENERAR_REPOSICION_CAJA", usuarioID,
		fmt.Sprintf("%s · %d vales · ₡%s → %s", f.Nombre, vinculados, total.StringFixed(2), doc.Consecutivo+doc.Clave))
	s.auditarDocNota(ctx, empresaID, doc.ID, "CREAR_DOCUMENTO", usuarioID,
		fmt.Sprintf("Reposición de caja chica «%s»: %d vales por ₡%s", f.Nombre, vinculados, total.StringFixed(2)))
	return s.repo.DocumentoPorID(ctx, empresaID, doc.ID)
}

func (s *Service) auditarCaja(ctx context.Context, empresaID, fondoID, accion, usuarioID, nota string) {
	if s.audit == nil {
		return
	}
	ev := shared.Evento{EmpresaID: &empresaID, Entidad: "caja_chica_fondo", EntidadID: &fondoID, Accion: accion, UsuarioID: &usuarioID}
	if nota != "" {
		ev.ValorNuevo = map[string]string{"nota": nota}
	}
	s.audit.Registrar(ctx, ev)
}
