package cxp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// ── LA PUERTA DE MÁQUINA ────────────────────────────────────────────────────────────────────
//
// Todo lo que llega del buzón pasa por acá. El orden importa y está pensado para que NADA se
// pierda y NADA entre en la empresa equivocada:
//
//  1. se registra el latido (siempre, incluso si después todo falla);
//  2. se guarda lo recibido ANTES de interpretarlo;
//  3. se coteja el buzón, el tipo de comprobante y el receptor;
//  4. recién entonces se crea la cuenta por pagar, reusando el camino del importador.
//
// Ver docs/GPVDP_Ingesta_Facturas_EspecTecnica_v1.0.md.

// VerificarTokenRecepcion resuelve el token de máquina: de qué empresa y de qué buzón es.
// Lo usa el middleware; el service no expone nada más de la credencial.
func (s *Service) VerificarTokenRecepcion(ctx context.Context, tokenEnClaro string) (TokenMaquina, error) {
	t := strings.TrimSpace(tokenEnClaro)
	if t == "" {
		return TokenMaquina{}, ErrTokenRecepcionInvalido
	}
	// Se hashea y se busca POR EL DIGEST: la comparación la hace el índice único de Postgres, no
	// Go, así que no hay canal de temporización.
	return s.repo.FuentePorTokenHash(ctx, hashTokenRecepcion(t))
}

// Latido registra que el script llamó, aunque no traiga facturas.
//
// Es el dato que distingue «hoy no hubo facturas» de «el script está muerto». Sin él, que Google
// desactive el trigger o que alguien revoque el token se ve en pantalla igual que un día tranquilo,
// y las facturas aparecen tres semanas después, varias ya vencidas.
func (s *Service) Latido(ctx context.Context, tok TokenMaquina) error {
	return s.repo.TocarFuente(ctx, tok.FuenteID)
}

// RecibirComprobante procesa UN comprobante que llega del buzón.
//
// Nunca devuelve error por algo que sea culpa del comprobante: eso queda en la recepción, con su
// motivo, y el resultado lo dice. Solo devuelve error cuando el problema es del sistema (la base
// caída, por ejemplo), porque eso sí tiene que hacer que el script REINTENTE.
func (s *Service) RecibirComprobante(ctx context.Context, tok TokenMaquina, in RecepcionInput) (ResultadoRecepcion, error) {
	// El latido primero: si el resto falla, igual quedó registrado que el script está vivo.
	if err := s.repo.TocarFuente(ctx, tok.FuenteID); err != nil {
		s.log.Warn("cxp: no se pudo registrar el latido de la fuente", zap.Error(err))
	}
	if len(in.XML) == 0 {
		return ResultadoRecepcion{}, errors.New("cxp: la recepción no trae el XML del comprobante")
	}

	// Se lee el comprobante para poder identificarlo (tipo, versión, clave, receptor) incluso si
	// después se descarta. `leerComprobantes` acepta un archivo con varios, pero el contrato de
	// esta puerta es un comprobante por llamada: se toma el primero y se informa si venían más.
	comprobantes, errLectura := leerComprobantes(in.XML)

	var c comprobanteXML
	var m mapeoComprobante
	if len(comprobantes) > 0 {
		c = comprobantes[0]
		m = mapearComprobante(c)
	}

	nueva := RecepcionNueva{
		FuenteID:      tok.FuenteID,
		Clave:         m.Fila.Clave,
		TipoDocumento: c.XMLName.Local,
		VersionSchema: versionDeEsquema(c.XMLName.Space),
		Receptor:      m.Fila.Receptor,
		XML:           in.XML,
		PDF:           in.PDF,
		PDFFilename:   in.PDFFilename,
		MessageID:     in.MessageID,
		Asunto:        in.Asunto,
		Remitente:     in.Remitente,
		Buzon:         in.Buzon,
		Estado:        RecPendiente,
	}
	nueva.LlaveIdem = llaveIdempotencia(in.MessageID, m.Fila.Clave, in.XML)

	// ── IDEMPOTENCIA ────────────────────────────────────────────────────────
	//
	// El script puede reenviar el mismo correo: el POST salió bien pero la respuesta se perdió, o
	// dos corridas se solaparon. Si esa llave ya se resolvió, se contesta LO MISMO que la primera
	// vez —con repetido = true— y no se crea nada. Contestar un error acá llenaría la cola en el
	// camino feliz y el hilo del correo nunca drenaría.
	if existente, err := s.repo.RecepcionPorLlave(ctx, tok.EmpresaID, nueva.LlaveIdem); err == nil {
		if existente.Estado != RecPendiente && existente.Estado != RecParqueada {
			return ResultadoRecepcion{
				RecepcionID: existente.ID, Estado: existente.Estado, Motivo: existente.Motivo,
				Repetido: true, Clave: existente.Clave, DocumentoID: existente.DocumentoID,
				Consecutivo: existente.Consecutivo,
			}, nil
		}
	} else if !errors.Is(err, ErrRecepcionNoEncontrada) {
		return ResultadoRecepcion{}, err
	}

	id, err := s.repo.GuardarRecepcion(ctx, tok.EmpresaID, nueva)
	if err != nil {
		return ResultadoRecepcion{}, err
	}

	// ── El juicio ───────────────────────────────────────────────────────────
	estado, motivo, docID, consecutivo := s.juzgarRecepcion(ctx, tok, in, comprobantes, m, errLectura)
	if err := s.repo.ResolverRecepcion(ctx, tok.EmpresaID, id, estado, motivo, docID); err != nil {
		return ResultadoRecepcion{}, err
	}
	s.auditarRecepcion(ctx, tok, id, estado, motivo)

	return ResultadoRecepcion{
		RecepcionID: id, Estado: estado, Motivo: motivo,
		Clave: m.Fila.Clave, DocumentoID: docID, Consecutivo: consecutivo,
	}, nil
}

// juzgarRecepcion decide qué pasa con el comprobante. Devuelve estado, motivo y el documento.
//
// El orden de los cortes NO es arbitrario: primero lo que descalifica la recepción completa (buzón
// equivocado, XML ilegible), después lo que descalifica al comprobante (tipo, receptor, clave
// repetida en otra empresa), y de último la creación.
func (s *Service) juzgarRecepcion(
	ctx context.Context, tok TokenMaquina, in RecepcionInput,
	comprobantes []comprobanteXML, m mapeoComprobante, errLectura error,
) (estado, motivo, documentoID, consecutivo string) {

	// 1. El buzón. Sin esta comparación, el campo «correo» de la pantalla de fuentes sería
	// decorativo: con el token de una empresa pegado en el script de otra —el copy-paste entre
	// scripts es el error más probable de todo el diseño— las facturas entrarían en la empresa
	// equivocada mientras la pantalla afirma lo contrario.
	if b := strings.TrimSpace(in.Buzon); b != "" && !strings.EqualFold(b, strings.TrimSpace(tok.Correo)) {
		return RecParqueada, fmt.Sprintf(
			"el token no corresponde a este buzón: la credencial es de «%s» y el correo llegó a «%s»",
			tok.Correo, b), "", ""
	}

	// 2. El XML.
	if len(comprobantes) == 0 {
		if errLectura != nil {
			return RecParqueada, "el XML no se pudo leer: " + errLectura.Error(), "", ""
		}
		return RecParqueada, "el archivo no trae ningún comprobante electrónico", "", ""
	}

	// 3. El tipo. Solo la factura electrónica genera cuenta por pagar (decisión del Director
	// Financiero, 2026-09-09). El resto NO se pierde: queda con su XML y su tipo, listo para
	// cuando se apruebe lo de las notas de crédito.
	tipo := comprobantes[0].XMLName.Local
	if tipo != TipoFacturaElectronica {
		if nombre, esDeHacienda := nombreDeTipo[tipo]; esDeHacienda {
			return RecDescartada, "es un comprobante de tipo «" + nombre +
				"», que no genera cuenta por pagar; se conserva para consulta", "", ""
		}
		// No es ninguno de los 7 tipos de Hacienda: es otro XML cualquiera (un aviso, una página
		// de error que el servidor de correo devolvió como adjunto). Se dice la raíz del archivo,
		// que es el dato con el que se entiende qué llegó.
		return RecParqueada, "el archivo no es un comprobante electrónico de Hacienda " +
			"(la raíz del XML dice «" + tipo + "»)", "", ""
	}

	// 4. La clave.
	if m.ClaveInvalida {
		return RecParqueada, "el comprobante no trae una clave de Hacienda válida (50 dígitos)", "", ""
	}
	if m.Fila.FechaEmision == "" {
		return RecParqueada, "no se pudo determinar la fecha de emisión del comprobante", "", ""
	}

	// 5. El RECEPTOR: de qué empresa es esta factura. Es el único guardarraíl que impide que una
	// factura de una empresa aterrice en otra, porque el UNIQUE de la base es (empresa_id, clave)
	// y la misma factura puede existir en dos empresas.
	cedulas, err := s.repo.CedulasDeEmpresa(ctx, tok.EmpresaID)
	if err != nil {
		s.log.Error("cxp: no se pudieron leer las cédulas de la empresa", zap.Error(err))
		// Fail-closed: si no se puede verificar, NO se acepta. Nadie está mirando esta puerta.
		return RecParqueada, "no se pudo verificar a qué empresa pertenece la factura", "", ""
	}
	if calza, porque := cotejarReceptor(m.Fila.Receptor, cedulas); !calza {
		return RecParqueada, porque, "", ""
	}

	// 6. ¿La misma factura ya entró en otra empresa del grupo? Si sí, se detiene: cada copia
	// recorrería su propio flujo y saldría en el archivo de pago de SU banco, y el proveedor
	// cobraría dos veces el mismo comprobante. No se dice en cuál empresa, para no filtrar
	// información entre empresas por el texto del error.
	if enOtra, err := s.repo.ClaveEnOtraEmpresa(ctx, tok.EmpresaID, m.Fila.Clave); err != nil {
		s.log.Error("cxp: no se pudo verificar la clave en el grupo", zap.Error(err))
		return RecParqueada, "no se pudo verificar si esta factura ya entró en otra empresa", "", ""
	} else if enOtra {
		return RecParqueada, "esta factura ya está registrada en otra empresa del grupo: " +
			"hay que definir a cuál corresponde antes de registrarla acá", "", ""
	}

	// 7. Crear. De acá en adelante es el MISMO camino del importador manual: resolver o dar de
	// alta el proveedor, armar el DocumentoInput y llamar a CrearDocumento.
	usuarioID, err := s.repo.UsuarioTecnicoRecepcion(ctx)
	if err != nil {
		s.log.Error("cxp: sin usuario técnico de recepción", zap.Error(err))
		return RecParqueada, "el sistema no tiene configurado el usuario de la integración", "", ""
	}

	var out ResultadoImportacion
	provID, err := s.resolverProveedor(ctx, tok.EmpresaID, m.Fila, map[string]string{}, &out, usuarioID)
	if err != nil {
		return RecParqueada, "no se pudo resolver el proveedor: " + err.Error(), "", ""
	}
	input, err := filaAInput(m.Fila, provID)
	if err != nil {
		return RecParqueada, err.Error(), "", ""
	}
	doc, err := s.CrearDocumento(ctx, tok.EmpresaID, input, usuarioID)
	if err != nil {
		if errors.Is(err, ErrDocumentoDuplicado) {
			// Ya estaba: se ENLAZA la que existe en vez de crear otra ni dejar la recepción
			// huérfana. Sin esto, cada reintento choca contra el mismo constraint para siempre.
			existente, e := s.repo.DocumentoPorClave(ctx, tok.EmpresaID, m.Fila.Clave)
			if e != nil {
				return RecParqueada, "la factura ya existe pero no se pudo enlazar: " + e.Error(), "", ""
			}
			return RecDuplicada, "esta factura ya estaba registrada", existente.ID, existente.Consecutivo
		}
		return RecParqueada, "no se pudo registrar la factura: " + err.Error(), "", ""
	}
	return RecProcesada, "", doc.ID, doc.Consecutivo
}

// ReintentarRecepcion vuelve a procesar una recepción parqueada, después de arreglar lo que faltaba.
//
// Solo tiene sentido sobre PARQUEADA: lo procesado ya tiene su documento y lo descartado no es una
// cuenta por pagar. Reutiliza el XML guardado, así que no hace falta volver a pedirle nada al correo
// —que es justo lo que hace que la cola de errores sirva de algo—.
func (s *Service) ReintentarRecepcion(ctx context.Context, empresaID, id, usuarioID string) (ResultadoRecepcion, error) {
	rec, err := s.repo.RecepcionPorID(ctx, empresaID, id)
	if err != nil {
		return ResultadoRecepcion{}, err
	}
	if rec.Estado != RecParqueada {
		return ResultadoRecepcion{}, ErrRecepcionNoReintentable
	}
	archivo, err := s.repo.ArchivoDeRecepcion(ctx, empresaID, id, "xml")
	if err != nil {
		// Si el XML no está —se borró porque la factura era de otra empresa— no hay nada que
		// reintentar acá: hay que reenviar el correo al buzón correcto.
		return ResultadoRecepcion{}, fmt.Errorf(
			"cxp: esta recepción no conserva el XML, así que no se puede reprocesar: %w", err)
	}

	comprobantes, errLectura := leerComprobantes(archivo.Contenido)
	var m mapeoComprobante
	if len(comprobantes) > 0 {
		m = mapearComprobante(comprobantes[0])
	}
	// El reintento corre con la fuente y el buzón que quedaron registrados, no con el token.
	tok := TokenMaquina{FuenteID: rec.FuenteID, EmpresaID: empresaID, Correo: rec.Buzon}
	estado, motivo, docID, consecutivo := s.juzgarRecepcion(ctx,
		tok, RecepcionInput{Buzon: rec.Buzon}, comprobantes, m, errLectura)

	if err := s.repo.ResolverRecepcion(ctx, empresaID, id, estado, motivo, docID); err != nil {
		return ResultadoRecepcion{}, err
	}
	s.auditarDocNota(ctx, empresaID, id, "REINTENTAR_RECEPCION", usuarioID, estado+": "+motivo)

	return ResultadoRecepcion{
		RecepcionID: id, Estado: estado, Motivo: motivo,
		Clave: m.Fila.Clave, DocumentoID: docID, Consecutivo: consecutivo,
	}, nil
}

// Recepciones lista la bandeja.
func (s *Service) Recepciones(ctx context.Context, empresaID string, f FiltrosRecepcion) ([]Recepcion, error) {
	return s.repo.ListarRecepciones(ctx, empresaID, f)
}

// ResumenRecepcion son los contadores de la bandeja (incluida la cola de errores).
func (s *Service) ResumenRecepcion(ctx context.Context, empresaID string) (ResumenRecepcion, error) {
	return s.repo.ResumenRecepcion(ctx, empresaID)
}

// ArchivoDeRecepcion devuelve el XML o el PDF originales para descargarlos.
func (s *Service) ArchivoDeRecepcion(ctx context.Context, empresaID, id, cual string) (ArchivoRecepcion, error) {
	return s.repo.ArchivoDeRecepcion(ctx, empresaID, id, cual)
}

// ── Fuentes de recepción ────────────────────────────────────────────────────

// CrearFuente da de alta un buzón y devuelve su token EN CLARO, una sola vez.
func (s *Service) CrearFuente(ctx context.Context, empresaID string, in FuenteInput, usuarioID string) (FuenteCreada, error) {
	in.Nombre = strings.TrimSpace(in.Nombre)
	in.Correo = strings.ToLower(strings.TrimSpace(in.Correo))
	if in.Nombre == "" || in.Correo == "" {
		return FuenteCreada{}, errors.New("cxp: la fuente necesita nombre y correo")
	}
	token, hash, err := generarTokenRecepcion()
	if err != nil {
		return FuenteCreada{}, fmt.Errorf("cxp: generar token: %w", err)
	}
	f, err := s.repo.CrearFuente(ctx, empresaID, in, hash, usuarioID)
	if err != nil {
		return FuenteCreada{}, err
	}
	// El token NO se audita (sería guardar la credencial en una tabla append-only): se audita que
	// se creó la fuente y para qué buzón.
	s.auditarDocNota(ctx, empresaID, f.ID, "CREAR_FUENTE_RECEPCION", usuarioID, f.Correo)
	return FuenteCreada{Fuente: f, Token: token}, nil
}

// Fuentes lista los buzones de la empresa, con su latido y sus contadores. Nunca el token.
func (s *Service) Fuentes(ctx context.Context, empresaID string) ([]FuenteRecepcion, error) {
	return s.repo.ListarFuentes(ctx, empresaID)
}

// RotarTokenFuente cambia la credencial del buzón: el token viejo deja de servir de inmediato.
func (s *Service) RotarTokenFuente(ctx context.Context, empresaID, fuenteID, usuarioID string) (string, error) {
	token, hash, err := generarTokenRecepcion()
	if err != nil {
		return "", fmt.Errorf("cxp: generar token: %w", err)
	}
	if err := s.repo.RotarTokenFuente(ctx, empresaID, fuenteID, hash); err != nil {
		return "", err
	}
	s.auditarDocNota(ctx, empresaID, fuenteID, "ROTAR_TOKEN_RECEPCION", usuarioID, "")
	return token, nil
}

// CambiarEstadoFuente activa o desactiva el buzón. Desactivada, su token deja de resolver.
func (s *Service) CambiarEstadoFuente(ctx context.Context, empresaID, fuenteID string, activo bool, usuarioID string) error {
	if err := s.repo.CambiarEstadoFuente(ctx, empresaID, fuenteID, activo); err != nil {
		return err
	}
	accion := "DESACTIVAR_FUENTE_RECEPCION"
	if activo {
		accion = "ACTIVAR_FUENTE_RECEPCION"
	}
	s.auditarDocNota(ctx, empresaID, fuenteID, accion, usuarioID, "")
	return nil
}

// CedulasDeEmpresa expone las cédulas jurídicas de la empresa, para que la pantalla pueda decir
// contra qué se está cotejando (y avisar si no hay ninguna).
func (s *Service) CedulasDeEmpresa(ctx context.Context, empresaID string) ([]string, error) {
	return s.repo.CedulasDeEmpresa(ctx, empresaID)
}

// auditarRecepcion deja el rastro de lo que hizo la MÁQUINA.
//
// `usuario_id` va en NULL a propósito: la columna es FK a usuario y admite nulo, y una recepción
// automática no tiene persona detrás. Quién la trajo queda en el valor del evento (la fuente).
// Pasar una cadena vacía como usuario haría fallar el INSERT en silencio, porque Registrar es
// best-effort y solo loguea.
func (s *Service) auditarRecepcion(ctx context.Context, tok TokenMaquina, recepcionID, estado, motivo string) {
	if s.audit == nil {
		return
	}
	emp := tok.EmpresaID
	ent := recepcionID
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &emp,
		Entidad:   "cxp_recepcion",
		EntidadID: &ent,
		Accion:    "RECIBIR_COMPROBANTE",
		UsuarioID: nil,
		ValorNuevo: map[string]any{
			"estado": estado, "motivo": motivo,
			"fuente_id": tok.FuenteID, "buzon": tok.Correo,
		},
	})
}
