package cxp

import (
	"context"

	"go.uber.org/zap"
)

// ClasificarDocumento asigna concepto/clasificación de gasto a un documento (paso "clasifican por
// gastos" del proceso). Reusa el catálogo Concepto/Clasificación; el repo valida pertenencia a la
// empresa. Pasar "" en un campo lo deja sin asignar.
func (s *Service) ClasificarDocumento(ctx context.Context, empresaID, id, conceptoID, clasificacionID, subclasificacionID, usuarioID string) (Documento, error) {
	n, err := s.repo.Clasificar(ctx, empresaID, id, conceptoID, clasificacionID, subclasificacionID)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		// 0 filas: el documento no existe, o el concepto/clasificación no es de la empresa.
		if _, e := s.repo.DocumentoPorID(ctx, empresaID, id); e != nil {
			return Documento{}, e // ErrDocumentoNoEncontrado
		}
		return Documento{}, ErrCatalogoInvalido
	}
	s.auditarDoc(ctx, empresaID, id, "CLASIFICAR_DOCUMENTO", usuarioID)
	d, err := s.repo.DocumentoPorID(ctx, empresaID, id)
	if err != nil {
		return Documento{}, err
	}
	// Memoria del proveedor: esta categoría queda como predeterminada (AUTO) y se acumula en
	// sus gastos frecuentes (algunos proveedores usan varias). Best-effort: no bloquea.
	if conceptoID != "" {
		if e := s.repo.GuardarGastoDefault(ctx, empresaID, d.ProveedorID, conceptoID, clasificacionID, subclasificacionID); e != nil {
			s.log.Warn("cxp: no se pudo guardar la memoria de gasto del proveedor", zap.Error(e))
		}
		if e := s.repo.RegistrarGastoProveedor(ctx, empresaID, d.ProveedorID, conceptoID, clasificacionID, subclasificacionID); e != nil {
			s.log.Warn("cxp: no se pudo registrar el gasto frecuente", zap.Error(e))
		}
	}
	return d, nil
}

// GastosFrecuentes lista las categorías usadas históricamente con un proveedor.
func (s *Service) GastosFrecuentes(ctx context.Context, empresaID, proveedorID string) ([]GastoFrecuente, error) {
	return s.repo.GastosDeProveedor(ctx, empresaID, proveedorID)
}

// AsignarPrioridadMasivo fija la prioridad interna (AA/A/"") de un lote de facturas.
func (s *Service) AsignarPrioridadMasivo(ctx context.Context, empresaID, usuarioID string, ids []string, prioridad string) (ResultadoMasivo, error) {
	if len(ids) == 0 {
		return ResultadoMasivo{}, ErrSinDocumentos
	}
	res := ResultadoMasivo{Resultados: make([]ResultadoTransicion, 0, len(ids))}
	for _, id := range ids {
		n, err := s.repo.AsignarPrioridad(ctx, empresaID, id, prioridad)
		rt := ResultadoTransicion{ID: id}
		if err != nil || n == 0 {
			rt.Error = "no se pudo asignar la prioridad"
			res.Fallidos++
		} else {
			rt.OK = true
			res.Exitosos++
			s.auditarDoc(ctx, empresaID, id, "PRIORIDAD_DOCUMENTO", usuarioID)
		}
		res.Resultados = append(res.Resultados, rt)
	}
	return res, nil
}

// ClasificarMasivo aplica la misma clasificación de gasto a varios documentos (best-effort por
// documento: el error de uno no frena a los demás).
func (s *Service) ClasificarMasivo(ctx context.Context, empresaID, usuarioID string, ids []string, conceptoID, clasificacionID, subclasificacionID string) (ResultadoMasivo, error) {
	if len(ids) == 0 {
		return ResultadoMasivo{}, ErrSinDocumentos
	}
	res := ResultadoMasivo{Resultados: make([]ResultadoTransicion, 0, len(ids))}
	for _, id := range ids {
		doc, err := s.ClasificarDocumento(ctx, empresaID, id, conceptoID, clasificacionID, subclasificacionID, usuarioID)
		rt := ResultadoTransicion{ID: id}
		if err != nil {
			rt.Error = err.Error()
			res.Fallidos++
		} else {
			rt.OK = true
			rt.Estado = doc.Estado
			res.Exitosos++
		}
		res.Resultados = append(res.Resultados, rt)
	}
	return res, nil
}

// AsignarTipoMasivo marca el tipo de factura de un lote (best-effort por documento).
func (s *Service) AsignarTipoMasivo(ctx context.Context, empresaID, usuarioID string, ids []string, tipo string) (ResultadoMasivo, error) {
	if len(ids) == 0 {
		return ResultadoMasivo{}, ErrSinDocumentos
	}
	res := ResultadoMasivo{Resultados: make([]ResultadoTransicion, 0, len(ids))}
	for _, id := range ids {
		doc, err := s.AsignarTipo(ctx, empresaID, id, tipo, usuarioID)
		rt := ResultadoTransicion{ID: id}
		if err != nil {
			rt.Error = err.Error()
			res.Fallidos++
		} else {
			rt.OK = true
			rt.Estado = doc.Estado
			res.Exitosos++
		}
		res.Resultados = append(res.Resultados, rt)
	}
	return res, nil
}
