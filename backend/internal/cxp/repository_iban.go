package cxp

// Acceso a datos de la carga de IBAN.

import (
	"context"
	"fmt"
	"strings"
)

// ProveedorIBAN es lo mínimo que necesita la carga para decidir sobre cada fila.
type ProveedorIBAN struct {
	ID     string
	Nombre string
	IBAN   string
}

// ProveedoresPorIdentificacion indexa los proveedores ACTIVOS de la empresa por su identificación.
//
// Se indexa por identificación y no por nombre porque el nombre viene escrito de mil formas en los
// Excel («FRANCELA FALLAS VARGAS», «Francela Fallas», con y sin S.A.), mientras que la cédula es
// el dato que el banco y Hacienda usan como llave. Se normaliza a solo dígitos y letras para que
// «3-101-402954» y «3101402954» sean la misma.
func (r *pgRepository) ProveedoresPorIdentificacion(ctx context.Context, empresaID string) (map[string]ProveedorIBAN, error) {
	const q = `SELECT id::text, nombre, COALESCE(identificacion,''), COALESCE(iban,'')
	           FROM proveedor WHERE empresa_id = $1::uuid AND activo`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: proveedores por identificación: %w", err)
	}
	defer rows.Close()
	out := make(map[string]ProveedorIBAN)
	for rows.Next() {
		var p ProveedorIBAN
		var ident string
		if err := rows.Scan(&p.ID, &p.Nombre, &ident, &p.IBAN); err != nil {
			return nil, fmt.Errorf("cxp: scan proveedor: %w", err)
		}
		if k := clave(ident); k != "" {
			out[k] = p
		}
	}
	return out, rows.Err()
}

// ActualizarIBANProveedor guarda la cuenta. No borra nada: si el proveedor ya tenía IBAN, queda
// reemplazado y el cambio va a la auditoría con el valor anterior visible en la previsualización.
func (r *pgRepository) ActualizarIBANProveedor(ctx context.Context, empresaID, proveedorID, iban string) error {
	const q = `UPDATE proveedor SET iban = $3, actualizado_en = now()
	           WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, proveedorID, iban)
	if err != nil {
		return fmt.Errorf("cxp: actualizar IBAN: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProveedorNoEncontrado
	}
	return nil
}

// clave normaliza una identificación para comparar: solo letras y dígitos, en mayúsculas.
// Así «3-101-402954», «3101402954» y «3 101 402954» son la misma llave.
func clave(ident string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(ident)) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
