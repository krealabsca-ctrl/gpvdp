// Package shared contiene utilidades transversales (auditoría, etc.).
package shared

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Evento describe un registro de auditoría (append-only). Los punteros nil => NULL.
type Evento struct {
	EmpresaID  *string
	Entidad    string
	EntidadID  *string
	Accion     string
	UsuarioID  *string
	ValorNuevo any
}

// Audit escribe eventos en auditoria_evento (tabla inmutable, spec §26).
type Audit struct {
	pool *pgxpool.Pool
	log  *zap.Logger
}

// NewAudit construye el escritor de auditoría.
func NewAudit(pool *pgxpool.Pool, log *zap.Logger) *Audit {
	return &Audit{pool: pool, log: log}
}

// Registrar inserta un evento. Es best-effort: si falla, loguea y no interrumpe la petición.
//
// Tolera un *Audit nil: la auditoría es un efecto de borde y su ausencia no debe tumbar la
// operación que la invoca (en producción siempre se inyecta; en pruebas no siempre).
func (a *Audit) Registrar(ctx context.Context, ev Evento) {
	if a == nil || a.pool == nil {
		return
	}
	var valorArg any
	if ev.ValorNuevo != nil {
		if b, err := json.Marshal(ev.ValorNuevo); err == nil {
			valorArg = string(b)
		}
	}

	const q = `
		INSERT INTO auditoria_evento (empresa_id, entidad, entidad_id, accion, valor_nuevo, usuario_id)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5::jsonb, $6::uuid)`
	if _, err := a.pool.Exec(ctx, q, ev.EmpresaID, ev.Entidad, ev.EntidadID, ev.Accion, valorArg, ev.UsuarioID); err != nil {
		a.log.Error("auditoria: no se pudo registrar evento",
			zap.String("accion", ev.Accion), zap.String("entidad", ev.Entidad), zap.Error(err))
	}
}
