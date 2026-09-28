package cxp

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// AdjuntarComprobante guarda el comprobante de pago (PDF) de una factura pagada/conciliada.
//
// Reemplazar el archivo borra la marca de «enviado» (lo hace el repositorio, en la misma
// transacción): el proveedor no tiene el PDF nuevo.
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

// EnviosComprobante devuelve la bitácora de envíos de una factura.
func (s *Service) EnviosComprobante(ctx context.Context, empresaID, docID string) ([]EnvioComprobante, error) {
	return s.repo.ListarEnvios(ctx, empresaID, docID)
}

// EnviarComprobante manda el comprobante al proveedor, con copia oculta a quien aprobó el pago, y
// DEJA CONSTANCIA del intento salga como salga.
//
// Tres cosas que antes no pasaban:
//
//  1. El servidor de correo se resuelve por EMPRESA en este momento, no en el arranque.
//  2. Va con copia oculta al aprobador (decisión del Director Financiero, 17-set-2026). Si la
//     factura no tiene aprobador o el aprobador no tiene correo, el envío NO se cae: sale sin
//     copia y eso queda escrito.
//  3. Queda una fila en la bitácora TAMBIÉN CUANDO FALLA. Hasta hoy el error se le mostraba al
//     usuario en pantalla y se perdía al recargar, así que nadie podía contestar «¿cuándo y a qué
//     dirección se le mandó el comprobante a este proveedor, y por qué no llegó?».
//
// Reenviar es esta misma llamada: cada una deja su propia fila, y la bitácora dice cuál fue el
// primer envío y cuáles fueron reenvíos.
func (s *Service) EnviarComprobante(ctx context.Context, empresaID, docID, usuarioID string) (ResultadoEnvioComprobante, error) {
	envio, err := s.repo.ObtenerComprobanteEnvio(ctx, empresaID, docID)
	if err != nil {
		return ResultadoEnvioComprobante{}, err
	}
	if envio.ProveedorEmail == "" {
		// No se registra en la bitácora: no hubo intento de envío. La fila exigiría un destinatario
		// que no existe, y una causa que no es del servidor de correo sino de la ficha del
		// proveedor. Hoy esto aplica a 44 de las 62 facturas pagadas de Valle de Paz: solo 6 de
		// 649 proveedores tienen correo registrado.
		return ResultadoEnvioComprobante{}, ErrProveedorSinEmail
	}
	if s.mailer == nil {
		return ResultadoEnvioComprobante{}, ErrCorreoNoConfigurado
	}
	// El texto sale de la plantilla de la empresa (editable en Configuración → Notificaciones).
	asunto, cuerpo, err := s.textoComprobante(ctx, empresaID, envio)
	if err != nil {
		return ResultadoEnvioComprobante{}, err
	}

	sobre := shared.Sobre{
		Para:   envio.ProveedorEmail,
		Asunto: asunto,
		Cuerpo: cuerpo,
		// OCULTA: el proveedor no ve la dirección de quien autoriza los pagos. Publicársela a 649
		// terceros, factura por factura, es entregar el blanco exacto de un fraude de cambio de
		// cuenta bancaria. La constancia de la copia queda en la bitácora, que es donde se audita.
		CopiaOculta: envio.AprobadorEmail,
		Adjunto:     &shared.Adjunto{Filename: envio.Filename, Mime: envio.Mime, Contenido: envio.Contenido},
	}
	origen, remitente, errEnvio := s.mailer.EnviarConAdjunto(ctx, empresaID, sobre)

	subido := envio.SubidoEn
	reg := RegistroEnvio{
		Destinatario:        envio.ProveedorEmail,
		Copia:               envio.AprobadorEmail,
		Remitente:           remitente,
		Origen:              origenValido(origen),
		Archivo:             envio.Filename,
		ComprobanteSubidoEn: &subido,
		Resultado:           EnvioOK,
		UsuarioID:           usuarioID,
	}
	if errEnvio != nil {
		reg.Resultado = EnvioError
		reg.ErrorCategoria = categoriaDeEnvio(errEnvio)
		// La frase para el operador, NO el texto crudo del servidor: esta columna es permanente y
		// la leen siete roles. El detalle técnico queda en correo_saliente.probado_error.
		reg.Error = errEnvio.Error()
	}

	reenvio, enviadoEn, errReg := s.repo.RegistrarEnvio(ctx, empresaID, docID, reg)
	if errEnvio != nil {
		if errReg != nil && s.log != nil {
			// La bitácora no pudo escribirse, pero el usuario tiene que leer POR QUÉ no salió el
			// correo: ese error gana. El de la bitácora se registra en el log del servidor.
			s.log.Error("cxp: no se pudo registrar el envío fallido del comprobante", zap.Error(errReg))
		}
		return ResultadoEnvioComprobante{}, errEnvio
	}
	if errReg != nil {
		// El correo SALIÓ y no quedó constancia: eso sí es un fallo del sistema, no del usuario.
		return ResultadoEnvioComprobante{}, errReg
	}

	s.auditarDoc(ctx, empresaID, docID, "ENVIAR_COMPROBANTE", usuarioID)
	return ResultadoEnvioComprobante{
		Destinatario: envio.ProveedorEmail,
		Copia:        envio.AprobadorEmail,
		Origen:       reg.Origen,
		Reenvio:      reenvio,
		EnviadoEn:    enviadoEn,
	}, nil
}

// origenValido protege el CHECK de la bitácora (`origen IN ('EMPRESA','GLOBAL')`): si la
// resolución no alcanzó a decir de dónde salía, se anota GLOBAL, que es lo que el sistema habría
// usado. Un INSERT rebotado acá borraría la única evidencia justo cuando el envío falló.
func origenValido(origen string) string {
	if origen == "EMPRESA" {
		return "EMPRESA"
	}
	return "GLOBAL"
}

// categoriaDeEnvio clasifica el fallo con los valores que acepta el CHECK de
// `comprobante_envio.error_categoria`.
func categoriaDeEnvio(err error) string {
	switch {
	case errors.Is(err, ErrCorreoNoConfigurado):
		return "CORREO_NO_CONFIGURADO"
	case errors.Is(err, ErrCifradoNoDisponible), errors.Is(err, ErrSecretoCorreoIlegible):
		return "SECRETO_ILEGIBLE"
	default:
		// shared.CategoriaDe devuelve la categoría del transporte, u OTRO si el error no es suyo.
		return shared.CategoriaDe(err)
	}
}
