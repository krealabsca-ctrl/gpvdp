package cxp

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// CrearDocumento registra un documento CxP (estado inicial RECIBIDO).
// total_crc = total (CRC) o total × TC (USD) — base para los umbrales de aprobación.
func (s *Service) CrearDocumento(ctx context.Context, empresaID string, in DocumentoInput, usuarioID string) (Documento, error) {
	// La clave de Hacienda es obligatoria SOLO para la factura electrónica (CXP). Los documentos
	// sin comprobante electrónico (Anticipo, Interno, Viáticos, Reintegro) generan referencia interna.
	tipo := in.Tipo
	if tipo == "" {
		tipo = TipoCxP
	}
	if tipo == TipoCxP && strings.TrimSpace(in.Clave) == "" {
		return Documento{}, ErrClaveRequerida
	}
	// La solicitud de anticipo lleva motivo + respaldo obligatorios (vía expresa: es lo que
	// sustituye a la validación de área, que el anticipo no recorre).
	if tipo == TipoAnticipo && strings.TrimSpace(in.Descripcion) == "" {
		return Documento{}, ErrMotivoAnticipoRequerido
	}
	totalCRC := in.Total
	var tc *decimal.Decimal
	if in.Moneda == "USD" {
		totalCRC = in.Total.Mul(in.TC).Round(2)
		t := in.TC
		tc = &t
	}
	d, err := s.repo.CrearDocumento(ctx, empresaID, in, totalCRC, tc, usuarioID)
	if err != nil {
		return Documento{}, err
	}
	s.auditarDoc(ctx, empresaID, d.ID, "CREAR_DOCUMENTO", usuarioID)
	return d, nil
}

// ListarDocumentos y DocumentoPorID exponen la hoja de documentos. Aplica scoping por área:
// si el usuario no tiene cxp.ver_todo, solo ve las facturas de su(s) departamento(s).
func (s *Service) ListarDocumentos(ctx context.Context, empresaID, rol, usuarioID string, f FiltrosDocumentos) (ListaDocumentos, error) {
	// La cartera abierta es la deuda total de la empresa: se gatea acá y no solo en la UI,
	// porque el filtro es alcanzable a mano (?abierta=true).
	if f.Abierta && s.perms != nil {
		puede, err := s.perms.Tiene(ctx, empresaID, rol, permisoCarteraAbierta)
		if err != nil {
			return ListaDocumentos{}, err
		}
		if !puede {
			return ListaDocumentos{}, ErrSinPermisoCartera
		}
	}
	deptIDs, err := s.departamentosVisibles(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return ListaDocumentos{}, err
	}
	f.DepartamentoIDs = deptIDs // nil = ve todo; no-nil = solo esas áreas
	return s.repo.ListarDocumentos(ctx, empresaID, f)
}

func (s *Service) DocumentoPorID(ctx context.Context, empresaID, id string) (Documento, error) {
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// Revisar: RECIBIDO → REVISADO.
// Revisar pasa la factura de RECIBIDO a REVISADO y, en el mismo acto, decide si el ÁREA tiene que
// confirmarla: es el momento en que Contabilidad ya le puso concepto y departamento, o sea el
// primero en que hay con qué decidir. El veredicto queda escrito en la factura (ver
// service_validacion_riesgo.go); no se recalcula después.
func (s *Service) Revisar(ctx context.Context, empresaID, id, usuarioID string) (Documento, error) {
	if _, err := s.transicionar(ctx, empresaID, id, EstRecibido, EstRevisado, "REVISAR_DOCUMENTO", usuarioID); err != nil {
		return Documento{}, err
	}
	s.evaluarValidacion(ctx, empresaID, id)
	// Se relee: el veredicto acaba de escribirse y el llamador tiene que verlo (la Bandeja decide
	// con él a qué cola va la factura).
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// AsignarDepartamentoDoc fija el departamento (centro de costo) del documento durante la
// segmentación de Contabilidad. Necesario para poder enrutar la validación de área.
func (s *Service) AsignarDepartamentoDoc(ctx context.Context, empresaID, id, deptoID, usuarioID string) (Documento, error) {
	n, err := s.repo.AsignarDepartamentoDoc(ctx, empresaID, id, deptoID)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		return s.conflictoOTransicion(ctx, empresaID, id)
	}
	s.auditarDoc(ctx, empresaID, id, "ASIGNAR_DEPARTAMENTO", usuarioID)
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// ValidarDepto: REVISADO → VALIDADO_DEPTO (control operativo de área). Reglas duras:
//   - el documento debe tener departamento asignado;
//   - el usuario debe ser validador (titular/suplente) de ese departamento;
//   - el respaldo es obligatorio (sin él la factura no avanza al pago).
func (s *Service) ValidarDepto(ctx context.Context, empresaID, id, usuarioID, respaldo, nota string) (Documento, error) {
	doc, err := s.repo.DocumentoPorID(ctx, empresaID, id)
	if err != nil {
		return Documento{}, err
	}
	if doc.Estado != EstRevisado {
		return Documento{}, ErrTransicionInvalida
	}
	if doc.DepartamentoID == "" {
		return Documento{}, ErrDeptoRequerido
	}
	if strings.TrimSpace(respaldo) == "" {
		return Documento{}, ErrRespaldoRequerido
	}
	ok, err := s.repo.EsValidador(ctx, empresaID, doc.DepartamentoID, usuarioID)
	if err != nil {
		return Documento{}, err
	}
	if !ok {
		return Documento{}, ErrNoEsValidador
	}
	n, err := s.repo.ValidarDeptoDoc(ctx, empresaID, id, usuarioID, respaldo, nota)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		return s.conflictoOTransicion(ctx, empresaID, id)
	}
	s.auditarDocNota(ctx, empresaID, id, "VALIDAR_DEPTO", usuarioID, nota)
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// ValidarEscalado: validación de área por ESCALAMIENTO (Dirección/Gerencia) cuando la factura
// está trancada — el departamento no tiene validador asignado, o la factura ya está vencida.
// Deja el mismo sello (REVISADO → VALIDADO_DEPTO) pero marcado como escalamiento en la nota y
// la auditoría. NO es un bypass general: si el área tiene validador y la factura no está vencida,
// se rechaza (usá la validación normal). El motivo del escalamiento es obligatorio.
func (s *Service) ValidarEscalado(ctx context.Context, empresaID, id, usuarioID, respaldo, motivo string) (Documento, error) {
	doc, err := s.repo.DocumentoPorID(ctx, empresaID, id)
	if err != nil {
		return Documento{}, err
	}
	if doc.Estado != EstRevisado {
		return Documento{}, ErrTransicionInvalida
	}
	if doc.DepartamentoID == "" {
		return Documento{}, ErrDeptoRequerido
	}
	if strings.TrimSpace(motivo) == "" {
		return Documento{}, ErrRespaldoRequerido // el motivo del escalamiento es obligatorio
	}
	// El escalamiento SOLO procede si la factura está trancada.
	vals, err := s.repo.ListarValidadores(ctx, empresaID, doc.DepartamentoID)
	if err != nil {
		return Documento{}, err
	}
	sinValidador := len(vals) == 0
	hoy := time.Now().Format("2006-01-02")
	vencida := doc.FechaVencimiento != nil && *doc.FechaVencimiento < hoy
	if !sinValidador && !vencida {
		return Documento{}, ErrEscalamientoNoAplica
	}
	nota := "[ESCALAMIENTO] " + strings.TrimSpace(motivo)
	n, err := s.repo.ValidarDeptoDoc(ctx, empresaID, id, usuarioID, respaldo, nota)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		return s.conflictoOTransicion(ctx, empresaID, id)
	}
	s.auditarDocNota(ctx, empresaID, id, "VALIDAR_ESCALADO", usuarioID, nota)
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// Devolver: REVISADO/VALIDADO_DEPTO → RECIBIDO. El validador (o Contabilidad) regresa la
// factura para corregir/re-enrutar, con motivo; se limpia el sello de validación previo.
func (s *Service) Devolver(ctx context.Context, empresaID, id, nota, usuarioID string) (Documento, error) {
	n, err := s.repo.DevolverDoc(ctx, empresaID, id, nota)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		return s.conflictoOTransicion(ctx, empresaID, id)
	}
	s.auditarDocNota(ctx, empresaID, id, "DEVOLVER_DOCUMENTO", usuarioID, nota)
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// MarcarPagado: PROGRAMADO → PAGADO.
func (s *Service) MarcarPagado(ctx context.Context, empresaID, id, usuarioID string) (Documento, error) {
	return s.transicionar(ctx, empresaID, id, EstProgramado, EstPagado, "PAGAR_DOCUMENTO", usuarioID)
}

// MarcarConciliado: PAGADO → CONCILIADO (luego lo automatiza la huella Bancos↔CxP).
func (s *Service) MarcarConciliado(ctx context.Context, empresaID, id, usuarioID string) (Documento, error) {
	return s.transicionar(ctx, empresaID, id, EstPagado, EstConciliado, "CONCILIAR_DOCUMENTO", usuarioID)
}

// Programar: APROBADO → PROGRAMADO, fija fecha de pago y genera la huella para el banco.
func (s *Service) Programar(ctx context.Context, empresaID, id, fechaPago, usuarioID string) (Documento, error) {
	huella := generarHuella(id)
	n, err := s.repo.Programar(ctx, empresaID, id, fechaPago, huella)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		return s.conflictoOTransicion(ctx, empresaID, id)
	}
	s.auditarDoc(ctx, empresaID, id, "PROGRAMAR_DOCUMENTO", usuarioID)
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// Aprobar registra la aprobación (financiera, por monto) del usuario; si el monto ya reúne
// las firmas requeridas, pasa VALIDADO_DEPTO → APROBADO (matriz por monto, ver aprobacion.go).
// Solo aprueba lo que el área YA validó, y quien validó no puede aprobar (segregación).
//
// VÍA EXPRESA (decisión del DF 2026-07-27, estándar SAP/Oracle): el ANTICIPO no pasa por la
// validación de área — todavía no hay gasto que validar; su respaldo es la cotización/contrato
// de la solicitud. Se aprueba directo desde RECIBIDO/REVISADO con la MISMA matriz de firmas.
// El control fuerte queda en la factura final, que sí valida el área y firma sobre el neto.
func (s *Service) Aprobar(ctx context.Context, empresaID, id, usuarioID, rol string) (Documento, error) {
	doc, err := s.repo.DocumentoPorID(ctx, empresaID, id)
	if err != nil {
		return Documento{}, err
	}
	// «De Contabilidad»: gasto que no tiene área operativa que lo valide (honorarios contables,
	// timbres, comisiones bancarias, Hacienda, auditoría). Se salta la validación de área —no la
	// firma— y solo lo puede aprobar quien tenga el permiso propio.
	deContabilidad := doc.EsContabilidad
	if deContabilidad {
		puede, err := s.puedeAprobarContabilidad(ctx, empresaID, rol)
		if err != nil {
			return Documento{}, err
		}
		if !puede {
			return Documento{}, ErrNoAprobadorContabilidad
		}
		// Segregación de funciones, misma lógica que validador≠aprobador: quien marcó A MANO que
		// esta factura se salta la validación de área NO puede además firmarla. Sin esto un solo
		// usuario con los dos permisos —y el SUPERVISOR_FINANCIERO los tiene— cerraría el ciclo
		// completo: marcar, aprobar y quedar listo para pago. La marca heredada del proveedor o
		// del rubro no bloquea a nadie: no es una decisión sobre ESTA factura.
		if doc.ContabilidadOrigen == ContaOrigenFactura && doc.ContabilidadMarcadoPor == usuarioID {
			return Documento{}, ErrMarcadorNoAprueba
		}
	}
	// «No requiere validación de área» es ahora la regla, no la excepción (decisión 2026-08-13):
	// una factura que no disparó ningún criterio de riesgo —monto, proveedor nuevo, desvío— avanza
	// del circuito contable directo a la firma por monto, sin esperar a que un área la confirme.
	sinValidacionDeArea := doc.RequiereValidacion != nil && !*doc.RequiereValidacion
	switch {
	case esViaExpresa(doc.Tipo), deContabilidad, sinValidacionDeArea:
		// Anticipo / Reintegro (caja chica) / Interno, las de Contabilidad y las que no requieren
		// validación de área: aprobables desde cualquier punto previo (una validación que sí se
		// haya hecho no estorba, suma).
		if doc.Estado != EstRecibido && doc.Estado != EstRevisado && doc.Estado != EstValidadoDepto {
			return Documento{}, ErrTransicionInvalida
		}
	default:
		if doc.Estado != EstValidadoDepto {
			return Documento{}, ErrTransicionInvalida
		}
	}
	// Regla dura de segregación de funciones: quien validó por el área no aprueba la misma factura.
	if doc.ValidadoDeptoPor != "" && doc.ValidadoDeptoPor == usuarioID {
		return Documento{}, ErrValidadorNoAprueba
	}
	if err := s.repo.RegistrarAprobacion(ctx, empresaID, id, usuarioID, rol); err != nil {
		return Documento{}, err
	}
	roles, err := s.repo.RolesAprobaciones(ctx, empresaID, id)
	if err != nil {
		return Documento{}, err
	}
	// La aprobación financiera (matriz de firmas por monto) se evalúa sobre el NETO a pagar
	// (total − anticipos aplicados), no sobre el total bruto de la factura.
	netoCRC, _ := decimal.NewFromString(doc.NetoCRC)
	quedaAprobada := aprobacionCompleta(netoCRC, roles)
	if quedaAprobada {
		// La transición tiene que aceptar los MISMOS estados de origen que aceptó la guarda de
		// arriba. Si no, el UPDATE no encuentra la fila, no falla, y la factura queda con la firma
		// registrada pero atascada en su estado anterior: el atasco silencioso de siempre.
		if esViaExpresa(doc.Tipo) || deContabilidad || sinValidacionDeArea {
			if _, err := s.repo.CambiarEstadoMulti(ctx, empresaID, id, []string{EstRecibido, EstRevisado, EstValidadoDepto}, EstAprobado); err != nil {
				return Documento{}, err
			}
		} else if _, err := s.repo.CambiarEstado(ctx, empresaID, id, EstValidadoDepto, EstAprobado); err != nil {
			return Documento{}, err
		}
	}
	// La auditoría distingue las dos aprobaciones y dice POR QUÉ se saltó el área: si quedaran
	// iguales, en el histórico no habría forma de encontrar las que no pasaron por validación.
	if deContabilidad {
		// Sellar la marca HEREDADA convierte «hoy el proveedor está marcado» en «esta factura se
		// aprobó por esta vía». Si no se sellara, desmarcar el proveedor mañana borraría del
		// documento la razón por la que se aprobó sin validación de área.
		//
		// Solo cuando la factura QUEDÓ aprobada: con firmas a medias todavía no pasó nada que
		// haya que congelar, y sellar antes le quitaría al catálogo la última chance de cambiar
		// de opinión.
		if quedaAprobada && doc.ContabilidadOrigen != ContaOrigenFactura {
			if err := s.repo.SellarContabilidad(ctx, empresaID, id, EtiquetaOrigenContabilidad(doc.ContabilidadOrigen)); err != nil {
				return Documento{}, err
			}
		}
		s.auditarDocNota(ctx, empresaID, id, "APROBAR_DOCUMENTO_CONTABILIDAD", usuarioID,
			"sin validación de área — "+EtiquetaOrigenContabilidad(doc.ContabilidadOrigen))
	} else {
		s.auditarDoc(ctx, empresaID, id, "APROBAR_DOCUMENTO", usuarioID)
	}
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// puedeAprobarContabilidad resuelve quién puede firmar la excepción.
//
// Vale CUALQUIERA de los dos permisos: el propio (`cxp.aprobar_contabilidad`, que existe para que
// el Supervisor —sin `cxp.aprobar`— pueda resolver este gasto) o el general de aprobación. Si solo
// contara el propio, marcar un rubro dejaría a los aprobadores normales sin poder aprobar facturas
// que antes sí aprobaban, y eso sería un retroceso, no un control.
//
// Sin checker configurado (tests) no bloquea: el candado real es el middleware de la ruta más este
// chequeo, y en los tests de servicio no hay RBAC que consultar.
func (s *Service) puedeAprobarContabilidad(ctx context.Context, empresaID, rol string) (bool, error) {
	if s.perms == nil {
		return true, nil
	}
	propio, err := s.perms.Tiene(ctx, empresaID, rol, permisoAprobarContabilidad)
	if err != nil {
		return false, err
	}
	if propio {
		return true, nil
	}
	return s.perms.Tiene(ctx, empresaID, rol, permisoAprobar)
}

// Denegar: RECIBIDO/REVISADO/VALIDADO_DEPTO → DENEGADO (se rechaza la factura en firme).
func (s *Service) Denegar(ctx context.Context, empresaID, id, usuarioID string) (Documento, error) {
	return s.transicionarMulti(ctx, empresaID, id, []string{EstRecibido, EstRevisado, EstValidadoDepto}, EstDenegado, "DENEGAR_DOCUMENTO", usuarioID)
}

// Anular: RECIBIDO/REVISADO/VALIDADO_DEPTO/APROBADO/PROGRAMADO → ANULADO (antes de pagar).
func (s *Service) Anular(ctx context.Context, empresaID, id, usuarioID string) (Documento, error) {
	return s.transicionarMulti(ctx, empresaID, id,
		[]string{EstRecibido, EstRevisado, EstValidadoDepto, EstAprobado, EstProgramado}, EstAnulado, "ANULAR_DOCUMENTO", usuarioID)
}

// Liquidar: RECIBIDO/REVISADO → LIQUIDADA (viáticos/almuerzos ya pagados: se archivan sin pago).
func (s *Service) Liquidar(ctx context.Context, empresaID, id, usuarioID string) (Documento, error) {
	return s.transicionarMulti(ctx, empresaID, id, []string{EstRecibido, EstRevisado}, EstLiquidada, "LIQUIDAR_DOCUMENTO", usuarioID)
}

// Rebotar: PROGRAMADO → REBOTADA (el banco rechazó el pago dentro de un lote).
func (s *Service) Rebotar(ctx context.Context, empresaID, id, usuarioID string) (Documento, error) {
	return s.transicionarMulti(ctx, empresaID, id, []string{EstProgramado}, EstRebotada, "REBOTAR_DOCUMENTO", usuarioID)
}

// Reintentar: REBOTADA → PROGRAMADO (la saca del lote para reincluirla en un nuevo corte).
func (s *Service) Reintentar(ctx context.Context, empresaID, id, usuarioID string) (Documento, error) {
	n, err := s.repo.Reintentar(ctx, empresaID, id)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		return s.conflictoOTransicion(ctx, empresaID, id)
	}
	s.auditarDoc(ctx, empresaID, id, "REINTENTAR_DOCUMENTO", usuarioID)
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// AsignarTipo fija el tipo de factura de un documento.
func (s *Service) AsignarTipo(ctx context.Context, empresaID, id, tipo, usuarioID string) (Documento, error) {
	n, err := s.repo.AsignarTipo(ctx, empresaID, id, tipo)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		return Documento{}, ErrDocumentoNoEncontrado
	}
	s.auditarDoc(ctx, empresaID, id, "TIPO_DOCUMENTO", usuarioID)
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// transicionarMulti cambia el estado si el actual está en alguno de `de`.
func (s *Service) transicionarMulti(ctx context.Context, empresaID, id string, de []string, a, accion, usuarioID string) (Documento, error) {
	n, err := s.repo.CambiarEstadoMulti(ctx, empresaID, id, de, a)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		return s.conflictoOTransicion(ctx, empresaID, id)
	}
	s.auditarDoc(ctx, empresaID, id, accion, usuarioID)
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// transicionar aplica un cambio de estado guardado por el estado esperado.
func (s *Service) transicionar(ctx context.Context, empresaID, id, de, a, accion, usuarioID string) (Documento, error) {
	n, err := s.repo.CambiarEstado(ctx, empresaID, id, de, a)
	if err != nil {
		return Documento{}, err
	}
	if n == 0 {
		return s.conflictoOTransicion(ctx, empresaID, id)
	}
	s.auditarDoc(ctx, empresaID, id, accion, usuarioID)
	return s.repo.DocumentoPorID(ctx, empresaID, id)
}

// conflictoOTransicion distingue documento inexistente de estado incorrecto (0 filas afectadas).
func (s *Service) conflictoOTransicion(ctx context.Context, empresaID, id string) (Documento, error) {
	if _, err := s.repo.DocumentoPorID(ctx, empresaID, id); err != nil {
		return Documento{}, err // ErrDocumentoNoEncontrado
	}
	return Documento{}, ErrTransicionInvalida
}

func (s *Service) auditarDoc(ctx context.Context, empresaID, id, accion, usuarioID string) {
	s.auditarDocNota(ctx, empresaID, id, accion, usuarioID, "")
}

// auditarDocNota registra el evento y, si viene una nota (motivo/comentario), la conserva en
// valor_nuevo->>'nota' para poder mostrarla en la línea de tiempo del documento. Antes el
// comentario de devolución solo vivía en documento_cxp.nota_revision (un único campo que se
// sobreescribe), así que no quedaba trazado por evento; ahora sí.
func (s *Service) auditarDocNota(ctx context.Context, empresaID, id, accion, usuarioID, nota string) {
	if s.audit == nil {
		return
	}
	ev := shared.Evento{
		EmpresaID: &empresaID, Entidad: "documento_cxp", EntidadID: &id, Accion: accion, UsuarioID: &usuarioID,
	}
	if strings.TrimSpace(nota) != "" {
		ev.ValorNuevo = map[string]string{"nota": strings.TrimSpace(nota)}
	}
	s.audit.Registrar(ctx, ev)
}

// generarHuella crea una descripción única para el banco (huella Bancos↔CxP).
func generarHuella(docID string) string {
	limpio := strings.ReplaceAll(docID, "-", "")
	if len(limpio) > 12 {
		limpio = limpio[:12]
	}
	return "CXP-" + strings.ToUpper(limpio)
}
