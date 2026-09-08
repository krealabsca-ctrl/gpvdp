// Package grupo es la vista consolidada de las empresas del grupo.
//
// ── QUÉ ES Y QUÉ NO ES ──────────────────────────────────────────────────────
//
// Es una VISTA: solo lee. No tiene una sola escritura, no cambia ninguna consulta de los otros
// módulos y no toca el middleware de tenant. Todo lo que ya funcionaba sigue funcionando igual; si
// este paquete se borrara, el sistema quedaría exactamente como antes.
//
// ── POR QUÉ ES UN PAQUETE APARTE ────────────────────────────────────────────
//
// Todo el ERP está aislado por empresa: cada consulta filtra por el `empresa_id` del token, y eso es
// la regla que protege los datos. Esta vista es la ÚNICA excepción deliberada, así que vive separada
// para que se vea: nadie va a agregar por accidente una consulta multiempresa dentro de bancos.
//
// ── CÓMO NO OTORGA ACCESO NUEVO ─────────────────────────────────────────────
//
// El consolidado suma únicamente las empresas donde el usuario YA podía ver ese dato: se cruzan sus
// membresías (`usuario_empresa_rol`) con el permiso de lectura del módulo en CADA empresa. Es una
// re-presentación de lo que ya podía mirar de a una, no una llave nueva.
//
// Y dice qué incluyó y qué dejó afuera. Un total que omite una empresa en silencio es peor que no
// tener total: se lee como el número del grupo cuando es el de dos tercios del grupo.
//
// ── LO QUE LOS DATOS PERMITEN HOY (medido el 2026-08-27) ────────────────────
//
// Solo Bancos existe en las tres empresas. CxP (4.542 facturas), nómina, inventario y CxC son solo de
// Valle de Paz, así que un «consolidado de CxP» sería una empresa más dos ceros presentados como
// total del grupo. No se construye hasta que haya datos en más de una.
package grupo

import (
	"context"

	"go.uber.org/zap"
)

// Repository es el acceso a datos de la vista. Todas las consultas reciben la LISTA de empresas
// permitidas y filtran por ella: no hay ninguna que lea «todas las empresas».
type Repository interface {
	// EmpresasVisibles resuelve, para un usuario, las empresas donde tiene membresía Y el permiso
	// indicado. Es el corazón de la seguridad de este paquete.
	EmpresasVisibles(ctx context.Context, usuarioID, permiso string, esAdmin bool) ([]EmpresaVisible, error)
	// ResumenPorEmpresa da ingresos, gastos y lo que quedó sin clasificar en el período.
	ResumenPorEmpresa(ctx context.Context, empresaIDs []string, periodo string) ([]FilaEmpresa, error)
	// OperacionesEntreEmpresas son los movimientos que cruzan empresas del grupo. Se muestran
	// APARTE y no se eliminan del total: decisión del Director Financiero (2026-08-27).
	OperacionesEntreEmpresas(ctx context.Context, empresaIDs []string, periodo string) ([]OperacionInterna, error)
	// PartidasDelGrupo son las partidas más grandes sumando todas las empresas visibles.
	PartidasDelGrupo(ctx context.Context, empresaIDs []string, periodo string, limite int) ([]PartidaGrupo, error)
	// ContarEmpresas dice cuántas hay en el sistema. Es lo único que permite confesar cuántas
	// quedaron afuera del consolidado: sin ese número, un total parcial se lee como el del grupo.
	ContarEmpresas(ctx context.Context) (int, error)
}

// Service es la lógica de la vista: resuelve el alcance, arma los totales y explica los límites.
type Service struct {
	repo Repository
	log  *zap.Logger
}

// NewService construye el servicio.
func NewService(repo Repository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log}
}
