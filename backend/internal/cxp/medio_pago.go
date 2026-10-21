package cxp

// Cerrar una factura que se pagó por fuera del banco, diciendo con qué (mig 0102).
//
// EL PEDIDO (Director Financiero, 16-oct-2026): «hay facturas que ya se pagaron pero tampoco las
// deja liberarlas, y algunas sí se pagaron por caja chica pero solo deja anularlas cuando debería
// de haber un botón de Caja chica o viáticos».
//
// LA CAUSA era una asimetría entre dos transiciones: `Anular` llega hasta PROGRAMADO y `Liquidar`
// se quedaba en REVISADO. En la pestaña «Por pagar» —donde aparece el candado del contado— la
// factura ya está APROBADA o PROGRAMADA, así que la única salida ofrecida era anular.
//
// Y anular una factura que SÍ se pagó es falsear el registro: dice que no existió cuando el dinero
// salió de verdad. El gasto desaparece de los libros y el proveedor queda sin su cuenta saldada.
//
// EL ESTADO ES LIQUIDADA, no PAGADO. Si quedaran en PAGADO entrarían en la partida conciliatoria
// del tablero —lo declarado como pagado que el banco no confirmó— y nunca conciliarían, porque no
// hay débito con huella que las respalde. Inflarían esa cifra para siempre.

import (
	"errors"
	"strings"
)

// Los cuatro medios por los que una factura puede haberse pagado sin pasar por el banco.
// Espejan el CHECK de la migración 0102: agregar uno acá sin la migración lo rechaza la base.
const (
	MedioCajaChica = "CAJA_CHICA"
	MedioViaticos  = "VIATICOS"
	MedioTarjeta   = "TARJETA"
	MedioEfectivo  = "EFECTIVO"
)

var (
	// ErrMedioPagoInvalido: el medio no es uno de los cuatro.
	ErrMedioPagoInvalido = errors.New("cxp: el medio de pago tiene que ser caja chica, viáticos, tarjeta o efectivo")
	// ErrMotivoMedioPagoRequerido: sin motivo no se cierra.
	//
	// Mismo criterio que liberar una factura de contado: es una afirmación con consecuencias y es
	// la única barrera que queda contra el doble pago, porque el vale de caja chica no lleva la
	// clave del comprobante. Si en seis meses aparece un pago duplicado, lo único que permite
	// reconstruir qué pasó es quién lo dijo, cuándo y con qué argumento.
	ErrMotivoMedioPagoRequerido = errors.New("cxp: decí cómo y cuándo se pagó esta factura (por ejemplo: vale 214 de caja chica del 3 de setiembre)")
	// ErrValeSoloCajaChica: el vale pertenece al fondo fijo.
	ErrValeSoloCajaChica = errors.New("cxp: el vale es del fondo de caja chica; no se puede ligar a un pago con tarjeta, efectivo o viáticos")
)

// El vale que no existe usa `ErrValeNoEncontrado`, que ya declara cajachica.go: es el mismo hecho
// y un centinela propio solo haría que la misma causa llegue con dos mensajes distintos según por
// qué puerta se entró.

// motivoMedioPagoMinimo es el largo mínimo del motivo.
//
// Veinte caracteres no garantizan una explicación, pero sí descartan la respuesta refleja —«ok»,
// «ya», «se pagó»— que es la que deja el expediente sin nada que leer. Es el mismo mínimo que
// liberar, por la misma razón y para que las dos se sientan igual.
const motivoMedioPagoMinimo = 20

// MedioPagoValido dice si el medio es uno de los cuatro.
func MedioPagoValido(m string) bool {
	switch m {
	case MedioCajaChica, MedioViaticos, MedioTarjeta, MedioEfectivo:
		return true
	}
	return false
}

// MedioPagoInput es lo que hace falta para cerrar una factura pagada por fuera del banco.
type MedioPagoInput struct {
	Medio  string `json:"medio"`
	Motivo string `json:"motivo"`
	// ValeID es el vale del fondo fijo, si existe. OPCIONAL a propósito (decisión del Director
	// Financiero): exigirlo dejaría la factura trancada cuando nadie cargó el vale, que es el
	// mismo problema que esto viene a resolver.
	ValeID string `json:"vale_id"`
}

// Normalizar limpia y valida la entrada.
func (in *MedioPagoInput) Normalizar() error {
	in.Medio = strings.ToUpper(strings.TrimSpace(in.Medio))
	in.Motivo = strings.TrimSpace(in.Motivo)
	in.ValeID = strings.TrimSpace(in.ValeID)

	if !MedioPagoValido(in.Medio) {
		return ErrMedioPagoInvalido
	}
	if len([]rune(in.Motivo)) < motivoMedioPagoMinimo {
		return ErrMotivoMedioPagoRequerido
	}
	if in.ValeID != "" && in.Medio != MedioCajaChica {
		return ErrValeSoloCajaChica
	}
	return nil
}

// MedioPagoDelDocumento es cómo se cerró una factura que no salió por el banco. Viaja al cliente
// dentro del documento: sin esto, una LIQUIDADA se ve igual que cualquier otra y nadie puede
// saber si fue un viático, la caja chica o una tarjeta.
type MedioPagoDelDocumento struct {
	Medio  string `json:"medio"`
	Motivo string `json:"motivo"`
	// Vale es el detalle del vale ligado, cuando hay uno. Vacío = se declaró sin vale.
	ValeID        string `json:"vale_id,omitempty"`
	ValeDetalle   string `json:"vale_detalle,omitempty"`
	ValeMonto     string `json:"vale_monto,omitempty"`
	RegistradoEn  string `json:"registrado_en"`
	RegistradoPor string `json:"registrado_por"`
}

// NotaDeCierre arma lo que queda escrito en la línea de tiempo del documento.
//
// Es donde alguien va a buscar «¿por qué esta factura está liquidada?», así que lleva el medio
// ADELANTE: es el dato que distingue este cierre de un viático archivado. Y cuando la factura
// salió de un corte lo dice, porque es un efecto sobre otro objeto —el corte de alguien más
// perdió una línea— y callarlo sería la clase de cambio silencioso que después nadie explica.
//
// Función aparte para poder fijarla sin infraestructura: lo que se prueba es el texto.
func NotaDeCierre(medio, motivo string, salioDeUnLote bool) string {
	nota := "pagada por " + EtiquetaMedioPago(medio) + " — " + strings.TrimSpace(motivo)
	if salioDeUnLote {
		nota += " · se sacó del corte en el que estaba"
	}
	return nota
}

// EtiquetaMedioPago traduce el medio para mostrarlo. Vive acá y no en el cliente porque el
// historial del documento lo escribe el servidor.
func EtiquetaMedioPago(m string) string {
	switch m {
	case MedioCajaChica:
		return "caja chica"
	case MedioViaticos:
		return "viáticos"
	case MedioTarjeta:
		return "tarjeta"
	case MedioEfectivo:
		return "efectivo"
	}
	return m
}
