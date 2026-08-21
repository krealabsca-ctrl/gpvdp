package cxp

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// AnticiposDisponibles: billetera de anticipos con saldo del proveedor.
func (s *Service) AnticiposDisponibles(ctx context.Context, empresaID, proveedorID string) ([]AnticipoSaldo, error) {
	return s.repo.AnticiposDisponibles(ctx, empresaID, proveedorID)
}

// AnticiposEmpresa: billetera global (todos los anticipos con saldo de la empresa).
func (s *Service) AnticiposEmpresa(ctx context.Context, empresaID string) ([]AnticipoSaldo, error) {
	return s.repo.AnticiposEmpresa(ctx, empresaID)
}

// AplicacionesDeFactura: anticipos aplicados (activos) a una factura.
func (s *Service) AplicacionesDeFactura(ctx context.Context, empresaID, facturaID string) ([]AplicacionAnticipo, error) {
	return s.repo.AplicacionesDeFactura(ctx, empresaID, facturaID)
}

// AplicarAnticipo netea un anticipo pagado del proveedor contra una factura, ANTES de aprobarla.
// El neto (total − anticipos) es lo que luego se aprueba y se paga. v1: solo colones (CRC).
func (s *Service) AplicarAnticipo(ctx context.Context, empresaID, facturaID, anticipoID, montoStr, usuarioID string) (Documento, error) {
	monto, err := decimal.NewFromString(montoStr)
	if err != nil || monto.LessThanOrEqual(decimal.Zero) {
		return Documento{}, ErrMontoAplicacionInvalido
	}
	factura, err := s.repo.DocumentoPorID(ctx, empresaID, facturaID)
	if err != nil {
		return Documento{}, err
	}
	anticipo, err := s.repo.DocumentoPorID(ctx, empresaID, anticipoID)
	if err != nil {
		return Documento{}, err
	}
	if anticipo.Tipo != TipoAnticipo {
		return Documento{}, ErrNoEsAnticipo
	}
	if anticipo.Estado != EstPagado && anticipo.Estado != EstConciliado {
		return Documento{}, ErrAnticipoNoPagado
	}
	// La factura receptora no puede ser a su vez un anticipo, y solo se netea antes de aprobar.
	if factura.Tipo == TipoAnticipo {
		return Documento{}, ErrFacturaNoNeteable
	}
	switch factura.Estado {
	case EstRecibido, EstRevisado, EstValidadoDepto:
		// ok: aún no aprobada
	default:
		return Documento{}, ErrFacturaNoNeteable
	}
	if factura.ProveedorID != anticipo.ProveedorID {
		return Documento{}, ErrProveedorDistinto
	}
	if factura.Moneda != "CRC" || anticipo.Moneda != "CRC" {
		return Documento{}, ErrMonedaNoNeteable
	}
	// Pre-chequeo con mensaje específico (la guarda atómica definitiva vive en el repo).
	saldo, err := s.repo.SaldoAnticipo(ctx, empresaID, anticipoID)
	if err != nil {
		return Documento{}, err
	}
	neto, _ := decimal.NewFromString(factura.NetoCRC)
	if monto.GreaterThan(saldo) || monto.GreaterThan(neto) {
		return Documento{}, ErrMontoAplicacionInvalido
	}
	if _, err := s.repo.AplicarAnticipo(ctx, empresaID, anticipoID, facturaID, monto, usuarioID); err != nil {
		return Documento{}, err
	}
	cons := anticipo.Consecutivo
	if cons == "" {
		cons = "anticipo"
	}
	s.auditarDocNota(ctx, empresaID, facturaID, "APLICAR_ANTICIPO", usuarioID,
		fmt.Sprintf("%s aplicado: ₡%s", cons, monto.StringFixed(2)))
	return s.repo.DocumentoPorID(ctx, empresaID, facturaID)
}

// AplicarAnticiposLote netea VARIOS anticipos contra la misma factura en una sola operación
// (todo-o-nada). Valida las mismas reglas que la aplicación individual para cada línea, y que
// la suma no exceda el neto de la factura.
func (s *Service) AplicarAnticiposLote(ctx context.Context, empresaID, facturaID string, lineas []AplicacionInput, usuarioID string) (Documento, error) {
	if len(lineas) == 0 {
		return Documento{}, ErrMontoAplicacionInvalido
	}
	factura, err := s.repo.DocumentoPorID(ctx, empresaID, facturaID)
	if err != nil {
		return Documento{}, err
	}
	if factura.Tipo == TipoAnticipo {
		return Documento{}, ErrFacturaNoNeteable
	}
	switch factura.Estado {
	case EstRecibido, EstRevisado, EstValidadoDepto:
	default:
		return Documento{}, ErrFacturaNoNeteable
	}
	if factura.Moneda != "CRC" {
		return Documento{}, ErrMonedaNoNeteable
	}
	neto, _ := decimal.NewFromString(factura.NetoCRC)
	suma := decimal.Zero
	vistos := map[string]bool{}
	for _, l := range lineas {
		if l.Monto.LessThanOrEqual(decimal.Zero) {
			return Documento{}, ErrMontoAplicacionInvalido
		}
		if vistos[l.AnticipoID] { // el mismo anticipo dos veces en el lote
			return Documento{}, ErrMontoAplicacionInvalido
		}
		vistos[l.AnticipoID] = true

		anticipo, err := s.repo.DocumentoPorID(ctx, empresaID, l.AnticipoID)
		if err != nil {
			return Documento{}, err
		}
		if anticipo.Tipo != TipoAnticipo {
			return Documento{}, ErrNoEsAnticipo
		}
		if anticipo.Estado != EstPagado && anticipo.Estado != EstConciliado {
			return Documento{}, ErrAnticipoNoPagado
		}
		if anticipo.ProveedorID != factura.ProveedorID {
			return Documento{}, ErrProveedorDistinto
		}
		if anticipo.Moneda != "CRC" {
			return Documento{}, ErrMonedaNoNeteable
		}
		saldo, err := s.repo.SaldoAnticipo(ctx, empresaID, l.AnticipoID)
		if err != nil {
			return Documento{}, err
		}
		if l.Monto.GreaterThan(saldo) {
			return Documento{}, ErrMontoAplicacionInvalido
		}
		suma = suma.Add(l.Monto)
	}
	if suma.GreaterThan(neto) {
		return Documento{}, ErrMontoAplicacionInvalido
	}
	if err := s.repo.AplicarAnticiposLote(ctx, empresaID, facturaID, lineas, usuarioID); err != nil {
		return Documento{}, err
	}
	s.auditarDocNota(ctx, empresaID, facturaID, "APLICAR_ANTICIPO", usuarioID,
		fmt.Sprintf("%d anticipo(s) aplicados: ₡%s", len(lineas), suma.StringFixed(2)))
	return s.repo.DocumentoPorID(ctx, empresaID, facturaID)
}

// ReversarAplicacion deshace una aplicación de anticipo (solo si la factura no fue pagada).
func (s *Service) ReversarAplicacion(ctx context.Context, empresaID, facturaID, aplicacionID, usuarioID string) (Documento, error) {
	if err := s.repo.ReversarAplicacion(ctx, empresaID, facturaID, aplicacionID, usuarioID); err != nil {
		return Documento{}, err
	}
	s.auditarDocNota(ctx, empresaID, facturaID, "REVERSAR_ANTICIPO", usuarioID, "Aplicación de anticipo reversada")
	return s.repo.DocumentoPorID(ctx, empresaID, facturaID)
}
