package bancos

// Corregir un banco o una cuenta mal creados.
//
// Antes solo se podía crear y renombrar: un banco agregado por error quedaba para siempre, y
// una cuenta con la moneda equivocada tampoco se podía arreglar.
//
// Las dos reglas del módulo que gobiernan esto:
//
//  1. Eliminar es FÍSICO solo cuando no hay NADA colgando (banco y cuenta son catálogo, no
//     tablas financieras). Con movimientos, saldos, actas o importaciones encima se rechaza
//     con el detalle y queda DESACTIVAR, que es lo que corresponde a un dato que sí existió.
//  2. La moneda y el IBAN solo se cambian si la cuenta NO tiene movimientos. Cambiar la
//     moneda de una cuenta con movimientos reinterpretaría cada monto ya importado (y su
//     conversión a colones, y todos los cuadres) sin que el archivo original haya cambiado.

import (
	"context"
	"fmt"
	"strings"
)

// EliminarBanco borra el banco solo si no tiene cuentas. Con cuentas devuelve el detalle:
// la salida es borrar las cuentas primero, o desactivar el banco.
func (r *pgRepository) EliminarBanco(ctx context.Context, empresaID, bancoID string) error {
	var pertenece bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM banco WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
		empresaID, bancoID).Scan(&pertenece); err != nil {
		return fmt.Errorf("bancos: verificar banco: %w", err)
	}
	if !pertenece {
		return ErrBancoNoEncontrado
	}
	var cuentas int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM cuenta_bancaria WHERE banco_id = $1::uuid`, bancoID).Scan(&cuentas); err != nil {
		return fmt.Errorf("bancos: cuentas del banco: %w", err)
	}
	if cuentas > 0 {
		return &CatalogoEnUsoError{Detalle: fmt.Sprintf(
			"%d cuenta(s) — eliminá o desactivá las cuentas primero, o desactivá el banco", cuentas)}
	}
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM banco WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, bancoID)
	if err != nil {
		return fmt.Errorf("bancos: eliminar banco: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBancoNoEncontrado
	}
	return nil
}

// CambiarActivoBanco desactiva o reactiva un banco. Desactivar NO toca sus cuentas ni sus
// movimientos: solo lo saca de los selectores para que nadie lo siga usando.
func (r *pgRepository) CambiarActivoBanco(ctx context.Context, empresaID, bancoID string, activo bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE banco SET activo = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, bancoID, activo)
	if err != nil {
		return fmt.Errorf("bancos: cambiar activo del banco: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBancoNoEncontrado
	}
	return nil
}

// UsoDeCuenta cuenta lo que cuelga de una cuenta. Se expone para que la pantalla pueda
// avisar ANTES de intentar el borrado, en vez de dejar al usuario chocar con el error.
type UsoDeCuenta struct {
	Movimientos   int `json:"movimientos"`
	Importaciones int `json:"importaciones"`
	Saldos        int `json:"saldos"`
	Actas         int `json:"actas"`
	Partidas      int `json:"partidas"`
}

func (u UsoDeCuenta) libre() bool {
	return u.Movimientos == 0 && u.Importaciones == 0 && u.Saldos == 0 && u.Actas == 0 && u.Partidas == 0
}

func (u UsoDeCuenta) detalle() string {
	var partes []string
	if u.Movimientos > 0 {
		partes = append(partes, fmt.Sprintf("%d movimiento(s)", u.Movimientos))
	}
	if u.Importaciones > 0 {
		partes = append(partes, fmt.Sprintf("%d importación(es)", u.Importaciones))
	}
	if u.Saldos > 0 {
		partes = append(partes, fmt.Sprintf("%d saldo(s) diario(s)", u.Saldos))
	}
	if u.Actas > 0 {
		partes = append(partes, fmt.Sprintf("%d acta(s) de conciliación", u.Actas))
	}
	if u.Partidas > 0 {
		partes = append(partes, fmt.Sprintf("%d partida(s) de conciliación", u.Partidas))
	}
	return strings.Join(partes, ", ")
}

func (r *pgRepository) UsoDeCuenta(ctx context.Context, empresaID, cuentaID string) (UsoDeCuenta, error) {
	var pertenece bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM cuenta_bancaria WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
		empresaID, cuentaID).Scan(&pertenece); err != nil {
		return UsoDeCuenta{}, fmt.Errorf("bancos: verificar cuenta: %w", err)
	}
	if !pertenece {
		return UsoDeCuenta{}, ErrCuentaNoEncontrada
	}
	const q = `
		SELECT
			(SELECT COUNT(*) FROM movimiento_bancario WHERE cuenta_bancaria_id = $1::uuid),
			(SELECT COUNT(*) FROM importacion WHERE cuenta_bancaria_id = $1::uuid),
			(SELECT COUNT(*) FROM saldo_cuenta_diario WHERE cuenta_bancaria_id = $1::uuid),
			(SELECT COUNT(*) FROM acta_conciliacion WHERE cuenta_bancaria_id = $1::uuid),
			(SELECT COUNT(*) FROM partida_conciliacion WHERE cuenta_bancaria_id = $1::uuid)`
	var u UsoDeCuenta
	if err := r.pool.QueryRow(ctx, q, cuentaID).
		Scan(&u.Movimientos, &u.Importaciones, &u.Saldos, &u.Actas, &u.Partidas); err != nil {
		return UsoDeCuenta{}, fmt.Errorf("bancos: uso de la cuenta: %w", err)
	}
	return u, nil
}

// EliminarCuenta borra la cuenta solo si no tiene nada colgando.
func (r *pgRepository) EliminarCuenta(ctx context.Context, empresaID, cuentaID string) error {
	uso, err := r.UsoDeCuenta(ctx, empresaID, cuentaID)
	if err != nil {
		return err
	}
	if !uso.libre() {
		return &CatalogoEnUsoError{Detalle: uso.detalle() + " — desactivá la cuenta en vez de eliminarla"}
	}
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM cuenta_bancaria WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, cuentaID)
	if err != nil {
		return fmt.Errorf("bancos: eliminar cuenta: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCuentaNoEncontrada
	}
	return nil
}

// CambiarActivoCuenta desactiva o reactiva una cuenta. Una cuenta desactivada desaparece de
// los selectores y del importador, pero sus movimientos siguen en la historia y en el cuadre.
func (r *pgRepository) CambiarActivoCuenta(ctx context.Context, empresaID, cuentaID string, activo bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE cuenta_bancaria SET activo = $3 WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, cuentaID, activo)
	if err != nil {
		return fmt.Errorf("bancos: cambiar activo de la cuenta: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCuentaNoEncontrada
	}
	return nil
}

// CambioDeCuenta son los campos a corregir. Cada uno es opcional: nil = no se toca.
type CambioDeCuenta struct {
	Alias   *string
	IBAN    *string
	Moneda  *string
	BancoID *string
}

// ActualizarCuenta corrige los datos de una cuenta.
//
// El alias y el banco se pueden cambiar siempre (son etiquetas). La MONEDA y el IBAN solo si
// la cuenta no tiene movimientos: la moneda porque reinterpretaría montos ya importados, y
// el IBAN porque es la identidad con la que el importador reconoce el archivo — cambiarlo
// con movimientos encima haría que dos cuentas distintas terminen mezcladas.
func (r *pgRepository) ActualizarCuenta(ctx context.Context, empresaID, cuentaID string, c CambioDeCuenta) error {
	uso, err := r.UsoDeCuenta(ctx, empresaID, cuentaID)
	if err != nil {
		return err
	}
	if (c.Moneda != nil || c.IBAN != nil) && uso.Movimientos > 0 {
		return &CambioNoPermitidoError{Motivo: fmt.Sprintf(
			"la cuenta ya tiene %d movimiento(s) importados: cambiarle la moneda o el IBAN "+
				"reinterpretaría montos que ya están en el cuadre. Creá la cuenta correcta y desactivá esta",
			uso.Movimientos)}
	}

	sets := []string{}
	args := []any{empresaID, cuentaID}
	add := func(expr string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf(expr, len(args)))
	}
	if c.Alias != nil {
		add("alias = NULLIF($%d, '')", *c.Alias)
	}
	if c.IBAN != nil {
		add("iban = NULLIF($%d, '')", *c.IBAN)
	}
	if c.Moneda != nil {
		add("moneda = $%d", *c.Moneda)
	}
	if c.BancoID != nil {
		// El banco destino tiene que ser de la misma empresa: si no, la cuenta saltaría de
		// empresa por la puerta de atrás.
		var ok bool
		if err := r.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM banco WHERE empresa_id = $1::uuid AND id = $2::uuid)`,
			empresaID, *c.BancoID).Scan(&ok); err != nil {
			return fmt.Errorf("bancos: verificar banco destino: %w", err)
		}
		if !ok {
			return ErrBancoNoEncontrado
		}
		add("banco_id = $%d::uuid", *c.BancoID)
	}
	if len(sets) == 0 {
		return nil
	}
	q := `UPDATE cuenta_bancaria SET ` + strings.Join(sets, ", ") +
		` WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, args...)
	if esViolacionUnica(err) {
		return ErrCatalogoDuplicado // otra cuenta de la empresa ya tiene ese IBAN
	}
	if err != nil {
		return fmt.Errorf("bancos: actualizar cuenta: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCuentaNoEncontrada
	}
	return nil
}
