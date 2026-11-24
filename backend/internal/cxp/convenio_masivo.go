package cxp

// Marcar «le creemos al convenio» a varios proveedores de una vez (21-oct-2026).
//
// ── EL PEDIDO ──────────────────────────────────────────────────────────────────────────────
//
// Textual del Director Financiero: «hay proveedores que son así y es tedioso cada factura
// ejecutar el mismo proceso de limpia; lo mejor es que exista un botón del proveedor que a pesar
// de que dice contado es a crédito».
//
// ── LA FUNCIÓN YA EXISTÍA, Y NADIE PODÍA ENCONTRARLA ───────────────────────────────────────
//
// La regla está en el INSERT del documento desde la migración 0097: con `convenio_manda =
// 'PROVEEDOR'` y `condicion_pago = 'CREDITO'`, la factura NO nace bloqueada aunque el comprobante
// diga contado. Funciona. Pero al medirlo: **0 de 649 proveedores** lo tenían puesto.
//
// No es que esté mal hecho: es que el campo se llama «El vencimiento lo manda» y en ninguna parte
// dice que también gobierna el candado del contado. Es el mismo patrón que ya apareció con las
// horas extra y con el orden de la Bandeja — construido, probado y sin puerta.
//
// Lo que falta, entonces, no es la regla: es poder aplicarla a muchos proveedores sin entrar 649
// veces, y saber a cuáles conviene.
//
// ── POR QUÉ HACEN FALTA LAS DOS CONDICIONES ────────────────────────────────────────────────
//
// `convenio_manda = PROVEEDOR` dice «le creemos a la ficha»; `condicion_pago = CREDITO` es lo que
// la ficha dice. Marcar lo primero sobre una ficha que dice CONTADO no desactiva nada —y se vería
// como que el sistema ignoró lo que se le pidió—, así que la respuesta lo informa fila por fila
// en vez de dejarlo pasar.

import "errors"

var (
	// ErrSinProveedores: no se mandó ninguno.
	ErrSinProveedores = errors.New("cxp: no se indicó ningún proveedor")
	// ErrDemasiadosProveedores: el tope existe para que un clic no reescriba el maestro entero.
	ErrDemasiadosProveedores = errors.New("cxp: demasiados proveedores en una sola operación (máximo 500)")
	// ErrConvenioInvalido: el valor no es ninguno de los dos conocidos.
	ErrConvenioInvalido = errors.New("cxp: el convenio tiene que ser «XML» o «PROVEEDOR»")
)

// topeProveedoresMasivo acota la operación.
//
// 500 cubre con holgura los 649 proveedores de la empresa más grande repartidos en dos tandas, y
// evita que un error de selección reescriba el maestro completo de un clic.
const topeProveedoresMasivo = 500

// CandidatoConvenio es un proveedor que declara contado en sus comprobantes, con lo que hace falta
// para decidir si marcarlo.
type CandidatoConvenio struct {
	ProveedorID string `json:"proveedor_id"`
	Nombre      string `json:"nombre"`
	// CondicionPago es lo que dice la FICHA (CREDITO o CONTADO). Si dice CONTADO, marcar el
	// convenio no levanta el candado: hacen falta las dos cosas.
	CondicionPago string `json:"condicion_pago"`
	ConvenioManda string `json:"convenio_manda"`
	// FacturasContado es cuántas de sus facturas declararon contado. Es la medida de cuánto
	// trabajo ahorra marcarlo.
	FacturasContado int `json:"facturas_contado"`
	// Bloqueadas y MontoBloqueado son lo que hoy está trancado: lo que se liberaría.
	Bloqueadas     int    `json:"bloqueadas"`
	MontoBloqueado string `json:"monto_bloqueado"`
	// YaListo: tiene las dos condiciones puestas, así que sus facturas nuevas ya no se bloquean.
	YaListo bool `json:"ya_listo"`
	// FaltaCredito: se marcó el convenio pero la ficha dice CONTADO, así que la regla no se
	// activa. Es la trampa que el usuario no puede ver solo.
	FaltaCredito bool `json:"falta_credito"`
}

// ResultadoConvenioMasivo es lo que cambió y lo que quedó a medias.
type ResultadoConvenioMasivo struct {
	Actualizados int `json:"actualizados"`
	// SinCredito son los proveedores que quedaron con el convenio marcado y la ficha en CONTADO:
	// la regla NO se les activa. Se devuelven con nombre para poder decirlo, en vez de informar un
	// éxito que no lo es.
	//
	// Solo se llena cuando NO se pidió pasarlos a crédito: con PasarACredito ya no queda nadie a
	// medias, y el aviso sería sobre un problema que se acaba de resolver.
	SinCredito []string `json:"sin_credito"`
	// PasadosACredito son los que estaban en CONTADO y quedaron en CRÉDITO para que la regla se
	// active. Se nombran porque es un cambio en la CONDICIÓN COMERCIAL de la ficha, no solo en a
	// quién se le cree: callarlo sería cambiar en silencio lo que el proveedor «es».
	PasadosACredito []string `json:"pasados_a_credito"`
	// PasadosACreditoIDs son los mismos, por id, para poder dejar la huella en la ficha de cada
	// uno. No viaja al cliente: la pantalla habla de nombres, la auditoría de identidades.
	PasadosACreditoIDs []string `json:"-"`
}

// ConvenioMasivoInput es el pedido.
type ConvenioMasivoInput struct {
	ProveedorIDs []string
	Convenio     string
	// PasarACredito completa la segunda condición en los que están en CONTADO.
	//
	// SIN ESTO EL BOTÓN DEJABA EL TRABAJO A MEDIAS. Marcar solo el convenio sobre una ficha que
	// dice CONTADO no desactiva nada —hacen falta las dos— y 573 de 649 proveedores están en
	// CONTADO: el camino dominante terminaba con un aviso de «pasalos a crédito» que había que ir
	// a hacer a mano, ficha por ficha, que es el trabajo tedioso que el panel venía a eliminar.
	//
	// Es una opción y no un automatismo porque entre los que declaran contado hay dos clases
	// distintas: el que lo declara por costumbre y es a crédito, y el que de verdad se paga en el
	// mostrador. Para el segundo el candado corresponde, y pasarlo a crédito abriría la puerta al
	// doble pago que el candado existe para cerrar.
	PasarACredito bool
}

// Validar revisa el pedido antes de tocar la base.
func (in ConvenioMasivoInput) Validar() error {
	if len(in.ProveedorIDs) == 0 {
		return ErrSinProveedores
	}
	if len(in.ProveedorIDs) > topeProveedoresMasivo {
		return ErrDemasiadosProveedores
	}
	if in.Convenio != "XML" && in.Convenio != "PROVEEDOR" {
		return ErrConvenioInvalido
	}
	return nil
}
