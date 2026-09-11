package cxp

import "errors"

// Errores de las acciones masivas del flujo CxP.
var (
	// ErrAccionInvalida indica una acción de transición no reconocida.
	ErrAccionInvalida = errors.New("cxp: acción de transición no válida")
	// ErrRolNoAutorizado indica que al rol le falta el permiso de la acción pedida.
	//
	// El mensaje dice DÓNDE se arregla: el error anterior («el rol no puede ejecutar esta acción»)
	// sonaba a límite del sistema y mandaba a buscar en el código, cuando la respuesta está en la
	// matriz de permisos.
	ErrRolNoAutorizado = errors.New("cxp: tu rol no tiene el permiso para esta acción; se marca en Configuración › Seguridad")
	// ErrFechaPagoRequerida indica que falta la fecha de pago al programar en lote.
	ErrFechaPagoRequerida = errors.New("cxp: la fecha de pago es obligatoria para programar")
	// ErrSinDocumentos indica que no se indicó ningún documento.
	ErrSinDocumentos = errors.New("cxp: no se indicaron documentos")
)

// Acciones válidas de transición del flujo (mismos verbos que las rutas por documento).
const (
	AccRevisar   = "revisar"
	AccAprobar   = "aprobar"
	AccProgramar = "programar"
	AccPagar     = "pagar"
	AccConciliar = "conciliar"
	// Acciones del ciclo de revisión (salen del flujo lineal).
	AccDenegar  = "denegar"
	AccAnular   = "anular"
	AccLiquidar = "liquidar"
	// Acciones del lote de pago (resultado del banco).
	AccRebotar    = "rebotar"
	AccReintentar = "reintentar"
)

// permisosPorAccion dice QUÉ PERMISO habilita cada acción del lote.
//
// ── POR QUÉ NO SON CÓDIGOS DE ROL ───────────────────────────────────────────
//
// Esto era una lista de códigos de rol escritos a mano, y ahí estaba el defecto. El sistema
// permite crear roles a medida y marcarles permisos desde la matriz —es una decisión del negocio,
// está en CLAUDE.md—, pero la lista solo conocía los seis roles base. Un rol nuevo con TODO CxP
// marcado recibía «el rol no puede ejecutar esta acción»: la matriz decía sí y el código decía no.
// Le pasó al rol «Supervisor Contable» en producción el 9 de setiembre de 2026 al liquidar unos
// viáticos, y la única salida era editar este archivo — que es exactamente lo que no puede pasar.
//
// La segunda verificación SIGUE siendo necesaria: la ruta masiva está detrás de un solo permiso
// (`cxp.revisar`) pero transporta diez acciones, así que sin esto quien puede revisar podría pagar.
// Lo que cambió es la pregunta: ahora es por el permiso de la acción concreta.
//
// Varios permisos en una lista = alcanza con tener UNO (igual que `PAlguno` en el router).
var permisosPorAccion = map[string][]string{
	// Los MISMOS permisos que la ruta individual de cada acción. Que el lote y el de a uno pidan
	// cosas distintas produce el peor síntoma posible —la misma factura, dos respuestas— y ya
	// estaba advertido en este archivo para `revisar`. Pasaba también, sin que nadie lo viera,
	// con `pagar` (se podía de a una y no en lote) y con `aprobar` (al revés: en lote sí y de a
	// una no, que es el que importa porque aprobar es firmar).
	AccRevisar:  {"cxp.revisar"},
	AccLiquidar: {"cxp.revisar"},
	// Denegar y anular no son «revisar»: sacan la factura del flujo. Tenían su propio recorte por
	// rol y ahora tienen su propio permiso, que la migración 0079 le da exactamente a quien ya
	// podía hacerlo — nadie gana ni pierde nada el día del despliegue.
	AccDenegar: {"cxp.anular"},
	AccAnular:  {"cxp.anular"},
	// Aprobar: el general, o el de las facturas «de Contabilidad». Alcanza con dejar pasar a
	// cualquiera de los dos porque el servicio de aprobación ya distingue el caso: si la factura
	// está marcada exige el permiso propio y aplica la segregación de funciones (quien la marcó a
	// mano no la firma). Sin el `o`, el Supervisor perdería el lote de las facturas de
	// Contabilidad, que sí puede aprobar de a una.
	AccAprobar: {"cxp.aprobar", "cxp.aprobar_contabilidad"},
	// Tesorería: los mismos de /programar, /pagar y /conciliar individuales.
	AccProgramar: {"cxp.tesoreria"},
	AccPagar:     {"cxp.tesoreria"},
	AccConciliar: {"cxp.tesoreria"},
	// El resultado del banco sobre un lote ya enviado. No tiene ruta individual, así que su
	// permiso propio conserva el recorte que tenía por rol (Dirección).
	AccRebotar:    {"cxp.resultado_pago"},
	AccReintentar: {"cxp.resultado_pago"},
}

func accionValida(accion string) bool {
	_, ok := permisosPorAccion[accion]
	return ok
}

// ResultadoTransicion es el resultado de aplicar la acción a un documento del lote.
type ResultadoTransicion struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Estado string `json:"estado,omitempty"` // estado resultante si OK
	Error  string `json:"error,omitempty"`  // motivo si falló
}

// ResultadoMasivo agrega el resultado de una transición masiva (best-effort por documento).
type ResultadoMasivo struct {
	Exitosos   int                   `json:"exitosos"`
	Fallidos   int                   `json:"fallidos"`
	Resultados []ResultadoTransicion `json:"resultados"`
}
