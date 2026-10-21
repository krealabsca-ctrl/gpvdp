package bancos

// Barrido de huellas: recorre los movimientos que traen la huella de CxP y empareja cada uno con
// su pago. Corre solo al confirmar una importación y también a pedido sobre lo ya cargado.

import (
	"context"

	"github.com/gpvdp/erp/internal/shared"
	"go.uber.org/zap"
)

// ConciliarCxP barre los movimientos con huella. importacionID vacío = toda la empresa.
//
// Solo concilia cuando el monto del banco coincide con el neto de la factura; si difiere, lo
// reporta y NO toca nada (es un hallazgo, no una conciliación).
func (s *Service) ConciliarCxP(ctx context.Context, empresaID, importacionID, usuarioID string) (ReporteConciliacionCxP, error) {
	rep := ReporteConciliacionCxP{Detalle: []PagoConciliado{}}
	if s.conciliadorCxP == nil {
		// CxP no está conectado: no hay nada que conciliar y no es un error.
		return rep, nil
	}
	rep.Disponible = true

	movs, err := s.repo.MovimientosConHuella(ctx, empresaID, s.conciliadorCxP.PrefijoHuella(), importacionID)
	if err != nil {
		return ReporteConciliacionCxP{}, err
	}
	rep.Examinados = len(movs)

	for _, m := range movs {
		res, err := s.conciliadorCxP.ConciliarHuella(ctx, empresaID, m.Descripcion, m.Debito, m.Moneda, usuarioID)
		if err != nil {
			return ReporteConciliacionCxP{}, err
		}
		linea := PagoConciliado{
			MovimientoID: m.ID, Fecha: m.Fecha, Cuenta: m.Cuenta,
			Huella: res.Huella, Veredicto: res.Veredicto,
			Consecutivo: res.Consecutivo, Proveedor: res.Proveedor,
			MontoBanco: m.Debito, MontoEsperado: res.MontoEsperado,
			MonedaBanco: m.Moneda, MonedaEsperada: res.MonedaEsperada,
		}

		switch res.Veredicto {
		case HuellaConciliado:
			// Un pago agrupado se enlaza contra la REMESA, no contra una de sus facturas: CxP
			// devuelve DocumentoID vacío justamente para que acá no se elija una al azar.
			var (
				clasificado, enlazado bool
				err                   error
			)
			if res.RemesaID != "" {
				clasificado, enlazado, err = s.repo.EnlazarPagoRemesa(ctx, empresaID, m.ID,
					res.RemesaID, res.ConceptoID, res.ClasificacionID)
			} else {
				clasificado, enlazado, err = s.repo.EnlazarPagoCxP(ctx, empresaID, m.ID,
					res.DocumentoID, res.ConceptoID, res.ClasificacionID)
			}
			if err != nil {
				return ReporteConciliacionCxP{}, err
			}
			if !enlazado {
				// Otro barrido lo tomó primero; no se cuenta dos veces.
				continue
			}
			linea.Clasificado = clasificado
			rep.Conciliados++
			s.audit.Registrar(ctx, shared.Evento{
				EmpresaID: &empresaID, Entidad: "movimiento_bancario", EntidadID: &m.ID,
				Accion: "CONCILIAR_CXP", UsuarioID: &usuarioID,
				ValorNuevo: map[string]any{
					"documento_id": res.DocumentoID, "consecutivo": res.Consecutivo,
					"huella": res.Huella, "monto": m.Debito, "clasificado": clasificado,
				},
			})
		case HuellaMontoDiferente:
			rep.MontoDiferente++
		case HuellaMonedaDiferente:
			rep.MonedaDiferente++
		case HuellaSinDocumento:
			rep.SinDocumento++
		case HuellaSinHuella:
			// NO es un pago nuestro: el pre-filtro busca «CXP» a secas (el banco borra el guion del
			// código, así que un filtro más estricto no traería ni uno de los verdaderos), y eso
			// arrastra descripciones ajenas como «FIDE 1937 CXP 430581 JUNIO 26/BANCO IMPROSA».
			//
			// Se cuenta aparte y NO va al detalle. Mezclarlo con «sin factura que lo reclame»
			// convertía cinco movimientos de otro banco en cinco problemas de CxP que no existen,
			// y una alarma falsa que se repite todos los meses termina ignorándose entera.
			rep.SinHuella++
			continue
		}
		rep.Detalle = append(rep.Detalle, linea)
	}
	return rep, nil
}

// conciliarCxPImportacion corre el barrido sobre lo recién importado. Best-effort: si falla,
// el confirm no se cae (igual que la auto-clasificación); el barrido se puede repetir a pedido.
func (s *Service) conciliarCxPImportacion(ctx context.Context, empresaID, importacionID, usuarioID string) {
	if s.conciliadorCxP == nil {
		return
	}
	rep, err := s.ConciliarCxP(ctx, empresaID, importacionID, usuarioID)
	if err != nil {
		s.log.Warn("conciliación CxP tras importar falló",
			zap.String("importacion", importacionID), zap.Error(err))
		return
	}
	if rep.Examinados > 0 {
		s.log.Info("conciliación CxP tras importar",
			zap.String("importacion", importacionID),
			zap.Int("examinados", rep.Examinados),
			zap.Int("conciliados", rep.Conciliados),
			zap.Int("monto_diferente", rep.MontoDiferente))
	}
}
