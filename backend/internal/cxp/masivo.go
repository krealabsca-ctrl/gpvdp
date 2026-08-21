package cxp

import "errors"

// Errores de las acciones masivas del flujo CxP.
var (
	// ErrAccionInvalida indica una acción de transición no reconocida.
	ErrAccionInvalida = errors.New("cxp: acción de transición no válida")
	// ErrRolNoAutorizado indica que el rol del usuario no puede ejecutar la acción.
	ErrRolNoAutorizado = errors.New("cxp: el rol no puede ejecutar esta acción")
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

// rolesPorAccion refleja el RBAC de router.go para cada transición. Es la autoridad
// del backend: aunque la ruta masiva se abre a la unión de roles, aquí se verifica
// que el rol pueda ejecutar la acción concreta del lote.
var rolesPorAccion = map[string][]string{
	// El auxiliar revisa igual que en la acción de a una: la ruta masiva y la individual están
	// gateadas por el MISMO permiso (cxp.revisar en router.go), así que si esta lista no lo
	// incluye, el sistema le dice que sí de a una y que no en lote — la misma factura, dos
	// respuestas. Aprobar/programar/pagar siguen siendo de supervisor para arriba.
	AccRevisar:    {"AUXILIAR_FINANCIERO", "SUPERVISOR_FINANCIERO", "DIRECTOR_FINANCIERO", "ADMIN"},
	AccAprobar:    {"SUPERVISOR_FINANCIERO", "DIRECTOR_FINANCIERO", "GERENCIA_GENERAL", "ADMIN"},
	AccProgramar:  {"DIRECTOR_FINANCIERO", "ADMIN"},
	AccPagar:      {"DIRECTOR_FINANCIERO", "ADMIN"},
	AccConciliar:  {"DIRECTOR_FINANCIERO", "ADMIN"},
	AccDenegar:    {"SUPERVISOR_FINANCIERO", "DIRECTOR_FINANCIERO", "ADMIN"},
	AccAnular:     {"SUPERVISOR_FINANCIERO", "DIRECTOR_FINANCIERO", "ADMIN"},
	AccLiquidar:   {"AUXILIAR_FINANCIERO", "SUPERVISOR_FINANCIERO", "DIRECTOR_FINANCIERO", "ADMIN"},
	AccRebotar:    {"DIRECTOR_FINANCIERO", "ADMIN"},
	AccReintentar: {"DIRECTOR_FINANCIERO", "ADMIN"},
}

func accionValida(accion string) bool {
	_, ok := rolesPorAccion[accion]
	return ok
}

func rolPuedeAccion(rol, accion string) bool {
	for _, r := range rolesPorAccion[accion] {
		if r == rol {
			return true
		}
	}
	return false
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
