package cxp

import (
	"context"
	"fmt"
)

// AdjuntarComprobante guarda el comprobante de pago (PDF) de una factura pagada/conciliada.
func (s *Service) AdjuntarComprobante(ctx context.Context, empresaID, docID, filename, mime string, contenido []byte, usuarioID string) error {
	if err := s.repo.GuardarComprobante(ctx, empresaID, docID, filename, mime, contenido, usuarioID); err != nil {
		return err
	}
	s.auditarDoc(ctx, empresaID, docID, "ADJUNTAR_COMPROBANTE", usuarioID)
	return nil
}

// DescargarComprobante devuelve el comprobante adjunto de una factura.
func (s *Service) DescargarComprobante(ctx context.Context, empresaID, docID string) (Comprobante, error) {
	return s.repo.ObtenerComprobante(ctx, empresaID, docID)
}

// EnviarComprobante manda el comprobante al correo del proveedor y marca la factura como enviada.
func (s *Service) EnviarComprobante(ctx context.Context, empresaID, docID, usuarioID string) error {
	envio, err := s.repo.ObtenerComprobanteEnvio(ctx, empresaID, docID)
	if err != nil {
		return err
	}
	if envio.ProveedorEmail == "" {
		return ErrProveedorSinEmail
	}
	// El texto sale de la plantilla de la empresa (editable en Configuración → Notificaciones).
	// Antes estaba escrito acá, con la firma de otra empresa y ₡ aunque la factura fuera en USD.
	asunto, cuerpo, err := s.textoComprobante(ctx, empresaID, envio)
	if err != nil {
		return err
	}
	if err := s.mailer.EnviarConAdjunto(envio.ProveedorEmail, asunto, cuerpo, envio.Filename, envio.Mime, envio.Contenido); err != nil {
		return fmt.Errorf("cxp: enviar comprobante: %w", err)
	}
	if err := s.repo.MarcarComprobanteEnviado(ctx, empresaID, docID); err != nil {
		return err
	}
	s.auditarDoc(ctx, empresaID, docID, "ENVIAR_COMPROBANTE", usuarioID)
	return nil
}
