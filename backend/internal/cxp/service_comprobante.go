package cxp

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// AdjuntarComprobante guarda el comprobante de pago (PDF) de una factura pagada/conciliada.
//
// Reemplazar el archivo borra la marca de «enviado» (lo hace el repositorio, en la misma
// transacción): el proveedor no tiene el PDF nuevo.
// `tipo` vacío = COMPROBANTE, que es lo que esta función significó siempre.
func (s *Service) AdjuntarComprobante(ctx context.Context, empresaID, docID, tipo, filename, mime string, contenido []byte, usuarioID string) error {
	t, ok := TipoAdjuntoValido(tipo)
	if !ok {
		return ErrTipoAdjuntoInvalido
	}
	if err := s.repo.GuardarComprobante(ctx, empresaID, docID, t, filename, mime, contenido, usuarioID); err != nil {
		return err
	}
	// La bitácora distingue los dos: «adjuntó un documento» no dice cuál, y el expediente de un
	// pago se audita por lo que tiene y lo que le falta.
	accion := "ADJUNTAR_COMPROBANTE"
	if t == AdjuntoFactura {
		accion = "ADJUNTAR_FACTURA"
	}
	s.auditarDoc(ctx, empresaID, docID, accion, usuarioID)
	return nil
}

// DescargarComprobante devuelve uno de los dos documentos adjuntos de una factura.
func (s *Service) DescargarComprobante(ctx context.Context, empresaID, docID, tipo string) (Comprobante, error) {
	t, ok := TipoAdjuntoValido(tipo)
	if !ok {
		return Comprobante{}, ErrTipoAdjuntoInvalido
	}
	return s.repo.ObtenerComprobante(ctx, empresaID, docID, t)
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
	return s.EnviarComprobanteA(ctx, empresaID, docID, "", usuarioID)
}

// EnviarComprobanteA manda el comprobante a UNA DIRECCIÓN ESCRITA A MANO.
//
// EL PEDIDO (Director Financiero, 19-oct-2026): poder cambiar el «Para» al reenviar. El contacto
// de Contabilidad del proveedor no siempre es el de la ficha, y hasta ahora la única forma de
// mandárselo a otra persona era editar la ficha, enviar y volver a dejarla como estaba.
//
// `destino` vacío usa el correo de la ficha, que es el camino normal. El que se escriba queda
// GUARDADO en la ficha si estaba vacía —misma decisión que el teléfono de WhatsApp— así que la
// próxima vez ya está. Si la ficha ya tenía correo NO se pisa: un envío puntual a otra persona no
// debería reemplazar el contacto oficial sin que nadie lo decida.
//
// A dónde se mandó queda en la bitácora igual que siempre: es lo que permite contestar, meses
// después, a qué dirección salió el comprobante de este pago y quién lo mandó.
func (s *Service) EnviarComprobanteA(ctx context.Context, empresaID, docID, destino, usuarioID string) (ResultadoEnvioComprobante, error) {
	envio, err := s.repo.ObtenerComprobanteEnvio(ctx, empresaID, docID)
	if err != nil {
		return ResultadoEnvioComprobante{}, err
	}
	if d := strings.TrimSpace(destino); d != "" {
		if !shared.EmailValido(d) {
			return ResultadoEnvioComprobante{}, ErrEmailDestinoInvalido
		}
		// El correo de la ficha se guarda ANTES de enviar: si el envío falla, el dato que la
		// persona ya escribió no se pierde y el reintento sale sin volver a escribirlo.
		if envio.ProveedorEmail == "" {
			if e := s.repo.GuardarEmailProveedorDeDocumento(ctx, empresaID, docID, d); e != nil {
				s.log.Warn("cxp: no se pudo guardar el correo del proveedor")
			}
		}
		envio.ProveedorEmail = d
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
