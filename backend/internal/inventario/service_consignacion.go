package inventario

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// Consignadas lista la cola: lo del proveedor que salió de la bodega.
func (s *Service) Consignadas(ctx context.Context, empresaID string, f FiltroConsignacion) (ColaConsignacion, error) {
	if f.Estado != "" && !estadoUnidadValido(f.Estado) {
		return ColaConsignacion{}, fmt.Errorf("%w: %q", ErrEstadoUnidadInvalido, f.Estado)
	}
	switch f.Situacion {
	case "", SituacionPendiente, SituacionFacturada:
	default:
		return ColaConsignacion{}, fmt.Errorf("%w: %q", ErrSituacionInvalida, f.Situacion)
	}

	filas, err := s.repo.ConsignadasPendientes(ctx, empresaID, f)
	if err != nil {
		return ColaConsignacion{}, err
	}
	res, err := s.repo.ResumenConsignacion(ctx, empresaID)
	if err != nil {
		return ColaConsignacion{}, err
	}

	for i := range filas {
		filas[i] = conLecturaDeConsignada(filas[i])
	}
	res.Aviso = avisoConsignacion(res, s.facturador != nil)

	return ColaConsignacion{
		Resumen: res,
		Filas:   filas,
		// PuedeFacturar le dice a la pantalla si mostrar el botón. Sin esto tendría que adivinarlo
		// por la ausencia de un error, es decir, intentándolo.
		PuedeFacturar: s.facturador != nil,
	}, nil
}

// conLecturaDeConsignada agrega lo derivado: la situación, el estado en palabras y el aviso del caso.
// Se calcula en un solo lugar para que la lista y el detalle no puedan decir cosas distintas de la
// misma unidad.
func conLecturaDeConsignada(u ConsignadaSalida) ConsignadaSalida {
	u.EstadoLegible = EtiquetaEstado(u.Estado)
	// La situación se LEE de lo que dijo el SQL, no se vuelve a derivar acá. Antes se calculaba con
	// «¿tiene enlace?» y una provisión anulada dejaba la fila como FACTURADA mientras el resumen la
	// contaba como deuda vigente: la cabecera y la tabla se contradecían.
	u.Situacion = SituacionFacturada
	if u.DeudaVigente {
		u.Situacion = SituacionPendiente
	}
	u.Aviso, u.PuedeFacturarse = avisoDeSalida(u.Estado, u.TipoSalida)
	// Ya facturada no se vuelve a facturar, diga lo que diga el caso.
	if u.Situacion == SituacionFacturada {
		u.PuedeFacturarse = false
	}
	// Solo queda algo que conciliar si lo enlazado es la provisión que generó el sistema y todavía
	// SE PUEDE ANULAR. Antes la condición era «distinto de ANULADO», y eso dejaba pasar una provisión
	// PAGADA: conciliarla rompía el enlace, la anulación fallaba (CxP no anula lo pagado) y el aviso
	// mandaba a «anulala en CxP», algo que el sistema no permite. Quedaba el doble pago armado y sin
	// salida. Ahora el candado principal es que la provisión nace bloqueada para pago, y este es el
	// segundo: no se concilia lo que ya no se puede anular.
	u.PuedeConciliarse = u.DocumentoEsProvision && provisionAnulable(u.DocumentoEstado)
	if u.Situacion == SituacionFacturada && !u.DocumentoEsProvision {
		u.Aviso = "ya está enlazada a la factura real del proveedor: no queda nada por conciliar"
	}
	// Sin proveedor no hay a quién pagarle. Puede pasar con unidades cargadas antes de que el
	// proveedor fuera obligatorio: se dice en pantalla en vez de fallar al apretar el botón.
	if u.ProveedorID == "" {
		u.PuedeFacturarse = false
		u.Aviso = "esta unidad quedó consignada sin proveedor, así que no hay a quién facturarle: corregila en el catálogo de unidades"
	}
	return u
}

// avisoConsignacion explica en una frase qué mirar.
func avisoConsignacion(r ResumenConsignacion, hayFacturador bool) string {
	if !hayFacturador {
		return "el módulo de cuentas por pagar no está conectado en este servidor: la cola se ve, pero las provisiones hay que crearlas a mano en CxP."
	}
	if r.PorFacturar == 0 {
		return ""
	}
	return fmt.Sprintf("hay %d unidad(es) del proveedor que ya salieron de la bodega y todavía no tienen cuenta por pagar.", r.PorFacturar)
}

// topeCandidatas acota el desplegable de conciliar. Un proveedor real de esta base tiene decenas de
// facturas; más de esto no se elige de una lista, se busca. El total va aparte para poder avisar que
// se cortó.
const topeCandidatas = 50

// CandidatasParaConciliar son las facturas del proveedor que sirven para reemplazar la provisión.
func (s *Service) CandidatasParaConciliar(ctx context.Context, empresaID, unidadID string) (CandidatasConciliacion, error) {
	u, err := s.repo.ConsignadaPorUnidad(ctx, empresaID, unidadID)
	if err != nil {
		return CandidatasConciliacion{}, err
	}
	if u.ProveedorID == "" {
		return CandidatasConciliacion{}, ErrProveedorConsignadaRequerido
	}
	filas, total, err := s.repo.FacturasCandidatas(ctx, empresaID, unidadID, topeCandidatas)
	if err != nil {
		return CandidatasConciliacion{}, err
	}
	out := CandidatasConciliacion{Filas: filas, Total: total}
	switch {
	case total == 0:
		out.Aviso = "este proveedor no tiene ninguna factura cargada que sirva para esta unidad: cargá la factura electrónica en Cuentas por pagar y volvé acá."
	case total > len(filas):
		out.Aviso = fmt.Sprintf("hay %d facturas de este proveedor y se muestran las %d más recientes.", total, len(filas))
	}
	return out, nil
}

// FacturarConsignada crea la PROVISIÓN de la cuenta por pagar de una unidad consignada usada.
//
// Por qué la factura nace FUERA de la transacción del servicio prestado: CxP no expone una variante
// transaccional de «crear documento», así que meterla adentro obligaría a refactorizar su interfaz
// completa. Pero el orden importa más que la atomicidad acá: el hecho —el funeral se prestó y el
// cofre salió de la bodega— tiene que quedar registrado SIEMPRE, incluso si CxP está caído. Si la
// factura falla, la unidad simplemente sigue en la cola y la pantalla lo grita; si en cambio el
// registro del servicio dependiera de que CxP responda, la bodega dejaría de registrar y el
// inventario entero se cae. El hecho no depende de su consecuencia.
//
// Es explícito y no automático al registrar el servicio por decisión del usuario: la provisión se
// crea cuando alguien la mira. Así el número que se le va a deber al proveedor lo confirma una
// persona, no un efecto colateral.
func (s *Service) FacturarConsignada(ctx context.Context, empresaID, unidadID, usuarioID string, confirmarSalidaRara bool) (ConsignadaSalida, error) {
	if s.facturador == nil {
		return ConsignadaSalida{}, ErrSinFacturadorCxP
	}
	u, err := s.repo.ConsignadaPorUnidad(ctx, empresaID, unidadID)
	if err != nil {
		return ConsignadaSalida{}, err
	}
	u = conLecturaDeConsignada(u)
	if u.Situacion == SituacionFacturada {
		return ConsignadaSalida{}, ErrConsignadaYaFacturada
	}
	if u.ProveedorID == "" {
		return ConsignadaSalida{}, ErrProveedorConsignadaRequerido
	}
	// Una cuenta por pagar de cero no representa ninguna deuda, ocupa un consecutivo y saca la unidad
	// de la cola: el problema real es el costo, y hay que corregirlo antes.
	if esCeroOMenos(u.CostoCRC) {
		return ConsignadaSalida{}, ErrCostoCeroNoSeFactura
	}
	// La guarda que le faltaba al service: la pantalla ya escondía el botón para las salidas que no
	// son un uso, pero el endpoint las aceptaba igual, así que por API se le podía facturar al
	// proveedor un cofre DEVUELTO. Devuelta nunca; dañada o no encontrada solo confirmándolo, que es
	// el botón «facturar a mano» que el negocio pidió para esos casos.
	if u.Estado == EstadoDevuelta {
		return ConsignadaSalida{}, ErrDevueltaNoSeFactura
	}
	if !u.PuedeFacturarse && !confirmarSalidaRara {
		return ConsignadaSalida{}, ErrSalidaNoFacturableSola
	}

	// El monto es el costo de la unidad tal cual, sin IVA: decisión del usuario. Es el número que la
	// bodega tecleó al recibir el cofre, y el IVA lo trae la factura real del proveedor cuando llega.
	f := FacturaConsignacion{
		ProveedorID: u.ProveedorID,
		Fecha:       primeraFechaNoVacia(u.FechaSalida, ahoraCR().Format("2006-01-02")),
		TotalCRC:    u.CostoCRC,
		Clave:       claveDeConsignacion(u.UnidadID),
		// El número de la unidad es lo que la gente tiene en la mano: es el que va a buscar en la
		// bandeja de CxP para saber de qué es esta factura.
		Consecutivo: "CONSIG-" + u.UnidadNumero,
		Descripcion: descripcionProvision(u),
	}
	docID, consecutivo, err := s.facturador.CrearProvisionConsignacion(ctx, empresaID, f, usuarioID)

	// La clave ya existe. Antes esto era un callejón sin salida: el documento estaba creado, la
	// unidad sin enlazar, y cada reintento volvía a chocar contra el duplicado. Ahora se RECUPERA,
	// que es para lo que sirve una clave determinística.
	if errors.Is(err, ErrProvisionYaExisteEnCxP) {
		previa, errBusca := s.facturador.ProvisionPorClave(ctx, empresaID, f.Clave)
		if errBusca != nil {
			return ConsignadaSalida{}, err
		}
		if previa.Estado == EstadoDocAnulado {
			// La provisión de un intento anterior se anuló. No se puede reusar —no respalda deuda— y
			// la clave está tomada, así que hay que decidirlo a mano en CxP: el sistema lo dice en vez
			// de reintentar en círculos.
			return ConsignadaSalida{}, fmt.Errorf("%w: la provisión %s de esta unidad quedó anulada; para volver a facturarla hay que cargar la factura del proveedor y conciliarla desde acá", ErrProvisionYaExisteEnCxP, previa.Consecutivo)
		}
		s.log.Warn("inventario: la provisión ya existía y se enlaza en vez de crear otra",
			zap.String("unidad_id", unidadID), zap.String("documento_id", previa.ID))
		docID, consecutivo = previa.ID, previa.Consecutivo
	} else if err != nil {
		return ConsignadaSalida{}, err
	}

	if err := s.repo.EnlazarCxPConsignacion(ctx, empresaID, unidadID, docID); err != nil {
		// El documento quedó creado y la unidad sin enlazar. No se borra: la clave es determinística,
		// así que el reintento encuentra ESTA misma provisión por el camino de arriba y la enlaza.
		s.log.Error("inventario: provisión creada sin enlazar a la unidad",
			zap.String("unidad_id", unidadID), zap.String("documento_id", docID))
		return ConsignadaSalida{}, err
	}

	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_unidad", EntidadID: &unidadID,
		Accion: "FACTURAR_CONSIGNACION", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"documento_cxp_id": docID, "consecutivo": consecutivo,
			"proveedor_id": u.ProveedorID, "total_crc": u.CostoCRC,
			"unidad": u.UnidadNumero, "servicio": u.ServicioNumero,
		},
	})

	fresca, err := s.repo.ConsignadaPorUnidad(ctx, empresaID, unidadID)
	if err != nil {
		return ConsignadaSalida{}, err
	}
	return conLecturaDeConsignada(fresca), nil
}

// ConciliarConsignada reemplaza la provisión por la factura electrónica REAL del proveedor.
//
// Es la mitad que hace que la provisión no sea un riesgo: anula el documento que generó el sistema y
// suelta la unidad, que queda enlazada al documento verdadero. Sin este paso habría dos cuentas por
// pagar por el mismo cofre —la provisión y la FE que entra por el importador— y el riesgo es pagar
// dos veces.
func (s *Service) ConciliarConsignada(ctx context.Context, empresaID, unidadID, documentoRealID, usuarioID string) (ConsignadaSalida, error) {
	if s.facturador == nil {
		return ConsignadaSalida{}, ErrSinFacturadorCxP
	}
	if documentoRealID == "" {
		return ConsignadaSalida{}, ErrDocumentoRealRequerido
	}
	u, err := s.repo.ConsignadaPorUnidad(ctx, empresaID, unidadID)
	if err != nil {
		return ConsignadaSalida{}, err
	}
	if u.DocumentoID == "" {
		return ConsignadaSalida{}, ErrConsignadaSinFactura
	}
	// Guarda contra el peor caso posible de esta pantalla: si lo enlazado ya es la factura real del
	// proveedor, «conciliar» la anularía. La comprobación va en el service y no solo en la UI porque
	// el botón se puede evitar llamando al endpoint directo.
	if !u.DocumentoEsProvision {
		return ConsignadaSalida{}, ErrYaConciliada
	}
	// La guarda que faltaba: si la provisión ya no se puede anular, conciliar rompería el enlace y
	// dejaría dos deudas vivas por el mismo cofre. Se rechaza ANTES de tocar nada y el mensaje dice
	// qué hacer, en vez de pedir una anulación que CxP no permite.
	if !provisionAnulable(u.DocumentoEstado) {
		return ConsignadaSalida{}, fmt.Errorf("%w: la provisión %s está %s", ErrProvisionYaNoAnulable,
			u.DocumentoConsecutivo, strings.ToLower(u.DocumentoEstado))
	}
	if u.DocumentoID == documentoRealID {
		// Conciliar la provisión contra sí misma la anularía y la dejaría enlazada: el peor de los
		// dos mundos.
		return ConsignadaSalida{}, ErrDocumentoRealEsLaProvision
	}

	// ── Validar TODO antes de tocar nada ─────────────────────────────────────
	//
	// Este orden es el arreglo de un defecto grave: antes se anulaba la provisión y se desenlazaba la
	// unidad ANTES de saber si el documento destino servía. Con un id inexistente, de otra empresa, o
	// ya usado por otra unidad, el tercer paso fallaba y quedaba la provisión anulada, la unidad sin
	// documento y la deuda con el proveedor sin respaldo. Y refacturarla era imposible porque la clave
	// determinística ya estaba tomada.
	real, err := s.facturador.DocumentoPorID(ctx, empresaID, documentoRealID)
	if err != nil {
		return ConsignadaSalida{}, err
	}
	if real.Estado == EstadoDocAnulado {
		return ConsignadaSalida{}, ErrDocumentoRealAnulado
	}
	if real.ProveedorID != u.ProveedorID {
		return ConsignadaSalida{}, ErrDocumentoRealDeOtroProveedor
	}
	otra, err := s.repo.UnidadConEsteDocumento(ctx, empresaID, documentoRealID, unidadID)
	if err != nil {
		return ConsignadaSalida{}, err
	}
	if otra != "" {
		return ConsignadaSalida{}, fmt.Errorf("%w: la usa %s", ErrDocumentoRealYaEnlazado, otra)
	}

	// El intercambio va en UN solo UPDATE: no hay instante en que la unidad quede sin documento.
	if err := s.repo.ReemplazarCxPConsignacion(ctx, empresaID, unidadID, u.DocumentoID, documentoRealID); err != nil {
		return ConsignadaSalida{}, err
	}

	// La anulación va ÚLTIMA y a propósito. Si falla, la unidad ya quedó apuntando a la factura real
	// —que es el estado correcto— y la provisión sigue viva y visible en CxP para que alguien la
	// anule. El orden inverso perdía el enlace; este solo deja un documento de más, a la vista.
	motivo := fmt.Sprintf("provisión de consignación reemplazada por la factura real del proveedor (unidad %s)", u.UnidadNumero)
	avisoAnulacion := ""
	if err := s.facturador.AnularProvision(ctx, empresaID, u.DocumentoID, motivo, usuarioID); err != nil {
		s.log.Error("inventario: la unidad quedó conciliada pero la provisión no se pudo anular",
			zap.String("unidad_id", unidadID), zap.String("provision_id", u.DocumentoID), zap.Error(err))
		avisoAnulacion = "la unidad quedó enlazada a la factura real, pero la provisión " +
			u.DocumentoConsecutivo + " NO se pudo anular: anulala en Cuentas por pagar para no pagarla dos veces"
	}

	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_unidad", EntidadID: &unidadID,
		Accion: "CONCILIAR_CONSIGNACION", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"provision_anulada":   u.DocumentoID,
			"documento_cxp_id":    documentoRealID,
			"unidad":              u.UnidadNumero,
			"total_provision_crc": u.DocumentoTotalCRC,
			"total_real_crc":      real.TotalCRC,
			"anulacion_fallo":     avisoAnulacion != "",
		},
	})

	fresca, err := s.repo.ConsignadaPorUnidad(ctx, empresaID, unidadID)
	if err != nil {
		return ConsignadaSalida{}, err
	}
	salida := conLecturaDeConsignada(fresca)
	if avisoAnulacion != "" {
		salida.Aviso = avisoAnulacion
	}
	return salida, nil
}

// esCeroOMenos dice si un monto en texto decimal no da para facturar. Con decimal y no comparando
// strings: "0", "0.00" y "0.000" son el mismo número y hay que tratarlos igual.
func esCeroOMenos(monto string) bool {
	d, err := decimal.NewFromString(monto)
	if err != nil {
		return true
	}
	return d.LessThanOrEqual(decimal.Zero)
}

// Servicio trae un servicio prestado con lo que consumió.
func (s *Service) Servicio(ctx context.Context, empresaID, id string) (Servicio, error) {
	return s.repo.ServicioPorID(ctx, empresaID, id)
}

// Unidad trae la ficha de un objeto físico por su número, con el estado en palabras.
func (s *Service) Unidad(ctx context.Context, empresaID, numero string) (Unidad, error) {
	u, err := s.repo.UnidadPorNumero(ctx, empresaID, numero)
	if err != nil {
		return Unidad{}, err
	}
	u.EstadoLegible = EtiquetaEstado(u.Estado)
	return u, nil
}

// descripcionProvision escribe de dónde salió la factura, con el nombre de la unidad y el servicio.
// Es lo que va a leer quien la encuentre en la bandeja de CxP dentro de tres semanas.
func descripcionProvision(u ConsignadaSalida) string {
	d := "Provisión por consignación: " + u.Articulo + " " + u.UnidadNumero
	if u.ServicioNumero != "" {
		d += " usada en el servicio " + u.ServicioNumero
		if u.ServicioANombreDe != "" {
			d += " (" + u.ServicioANombreDe + ")"
		}
	}
	if u.Sede != "" {
		d += " · " + u.Sede
	}
	return d + ". Conciliar con la factura electrónica del proveedor cuando llegue."
}

func primeraFechaNoVacia(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
