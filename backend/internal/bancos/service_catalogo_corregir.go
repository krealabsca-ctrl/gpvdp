package bancos

// Corregir el catálogo y las cuentas: eliminar o desactivar un banco o una cuenta creados
// por error, arreglar la moneda de una cuenta, y fusionar dos conceptos o dos
// clasificaciones para que el catálogo quede limpio.
//
// Todo lo de acá deja evento de auditoría: son operaciones que reescriben o borran datos de
// catálogo compartido con CxP y caja chica, y varias son irreversibles.

import (
	"context"

	"github.com/gpvdp/erp/internal/shared"
)

// EliminarBanco borra un banco sin cuentas (en uso → CatalogoEnUsoError con el detalle).
func (s *Service) EliminarBanco(ctx context.Context, empresaID, bancoID, usuarioID string) error {
	if err := s.repo.EliminarBanco(ctx, empresaID, bancoID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "banco", EntidadID: &bancoID,
		Accion: "ELIMINAR_BANCO", UsuarioID: &usuarioID,
	})
	return nil
}

// CambiarActivoBanco desactiva o reactiva un banco.
func (s *Service) CambiarActivoBanco(ctx context.Context, empresaID, bancoID string, activo bool, usuarioID string) error {
	if err := s.repo.CambiarActivoBanco(ctx, empresaID, bancoID, activo); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "banco", EntidadID: &bancoID,
		Accion: "CAMBIAR_ACTIVO_BANCO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]bool{"activo": activo},
	})
	return nil
}

// UsoDeCuenta dice qué cuelga de una cuenta, para avisar ANTES de intentar eliminarla.
func (s *Service) UsoDeCuenta(ctx context.Context, empresaID, cuentaID string) (UsoDeCuenta, error) {
	return s.repo.UsoDeCuenta(ctx, empresaID, cuentaID)
}

// EliminarCuenta borra una cuenta sin movimientos ni historia (en uso → detalle).
func (s *Service) EliminarCuenta(ctx context.Context, empresaID, cuentaID, usuarioID string) error {
	if err := s.repo.EliminarCuenta(ctx, empresaID, cuentaID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "cuenta_bancaria", EntidadID: &cuentaID,
		Accion: "ELIMINAR_CUENTA", UsuarioID: &usuarioID,
	})
	return nil
}

// CambiarActivoCuenta desactiva o reactiva una cuenta.
func (s *Service) CambiarActivoCuenta(ctx context.Context, empresaID, cuentaID string, activo bool, usuarioID string) error {
	if err := s.repo.CambiarActivoCuenta(ctx, empresaID, cuentaID, activo); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "cuenta_bancaria", EntidadID: &cuentaID,
		Accion: "CAMBIAR_ACTIVO_CUENTA", UsuarioID: &usuarioID,
		ValorNuevo: map[string]bool{"activo": activo},
	})
	return nil
}

// ActualizarCuenta corrige alias, banco, IBAN o moneda. La moneda y el IBAN solo si la
// cuenta no tiene movimientos: el repositorio lo hace cumplir.
func (s *Service) ActualizarCuenta(ctx context.Context, empresaID, cuentaID string, c CambioDeCuenta, usuarioID string) error {
	if err := s.repo.ActualizarCuenta(ctx, empresaID, cuentaID, c); err != nil {
		return err
	}
	detalle := map[string]string{}
	if c.Alias != nil {
		detalle["alias"] = *c.Alias
	}
	if c.IBAN != nil {
		detalle["iban"] = *c.IBAN
	}
	if c.Moneda != nil {
		detalle["moneda"] = *c.Moneda
	}
	if c.BancoID != nil {
		detalle["banco_id"] = *c.BancoID
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "cuenta_bancaria", EntidadID: &cuentaID,
		Accion: "ACTUALIZAR_CUENTA", UsuarioID: &usuarioID, ValorNuevo: detalle,
	})
	return nil
}

// FusionarConceptos mueve todo del concepto origen al destino y borra el origen.
// Es irreversible: el resumen de lo que arrastró queda en la auditoría.
func (s *Service) FusionarConceptos(ctx context.Context, empresaID, origenID, destinoID, usuarioID string) (ResumenFusion, error) {
	res, err := s.repo.FusionarConceptos(ctx, empresaID, origenID, destinoID)
	if err != nil {
		return ResumenFusion{}, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "concepto", EntidadID: &origenID,
		Accion: "FUSIONAR_CONCEPTO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"destino_id": destinoID, "origen": res.Origen, "destino": res.Destino,
			"movimientos": res.Movimientos, "reglas": res.Reglas,
			"documentos_cxp": res.DocumentosCxP, "gastos_proveedor": res.GastosProveedor,
			"vales_caja_chica": res.ValesCajaChica, "proveedores": res.Proveedores,
			"clasificaciones": res.Clasificaciones, "subclasificaciones": res.Subclasificaciones,
		},
	})
	return res, nil
}

// FusionarClasificaciones mueve todo de una clasificación a otra (que puede estar en otro
// concepto, si se confirma) y borra la de origen.
func (s *Service) FusionarClasificaciones(ctx context.Context, empresaID, origenID, destinoID string, permitirOtroConcepto bool, usuarioID string) (ResumenFusion, error) {
	res, err := s.repo.FusionarClasificaciones(ctx, empresaID, origenID, destinoID, permitirOtroConcepto)
	if err != nil {
		return ResumenFusion{}, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "clasificacion", EntidadID: &origenID,
		Accion: "FUSIONAR_CLASIFICACION", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"destino_id": destinoID, "origen": res.Origen, "destino": res.Destino,
			"cambio_de_concepto": permitirOtroConcepto,
			"movimientos":        res.Movimientos, "reglas": res.Reglas,
			"documentos_cxp": res.DocumentosCxP, "gastos_proveedor": res.GastosProveedor,
			"vales_caja_chica": res.ValesCajaChica, "proveedores": res.Proveedores,
			"subclasificaciones": res.Subclasificaciones,
		},
	})
	return res, nil
}
