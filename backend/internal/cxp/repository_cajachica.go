package cxp

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Condición SQL: el vale sigue contando contra el saldo mientras su reposición no esté pagada.
// (pendiente, o en una reposición que aún no llegó a PAGADO/CONCILIADO — incluye anuladas:
// si la reposición se anuló, la plata sigue gastada y sin reponer.)
const valeCuentaContraSaldo = `NOT v.anulado AND (v.reposicion_id IS NULL OR NOT EXISTS (
	SELECT 1 FROM documento_cxp r WHERE r.id = v.reposicion_id AND r.estado IN ('PAGADO', 'CONCILIADO')))`

// Condición SQL: el vale es elegible para una NUEVA reposición (nunca vinculado, o su
// reposición anterior murió en el archivo: anulada/denegada/liquidada).
const valeElegibleReposicion = `NOT v.anulado AND (v.reposicion_id IS NULL OR EXISTS (
	SELECT 1 FROM documento_cxp r WHERE r.id = v.reposicion_id AND r.estado IN ('ANULADO', 'DENEGADO', 'LIQUIDADA')))`

const fondoCols = `f.id::text, f.nombre,
	COALESCE(f.custodio_id::text, ''), COALESCE(NULLIF(u.nombre, ''), u.email, ''),
	COALESCE(f.departamento_id::text, ''), COALESCE(d.nombre, ''),
	COALESCE(f.proveedor_id::text, ''), COALESCE(p.nombre, ''),
	f.monto_asignado::text, f.umbral_pct::text, f.limite_vale::text, f.activo,
	COALESCE((SELECT SUM(v.monto_crc) FROM caja_chica_vale v WHERE v.fondo_id = f.id AND ` + valeCuentaContraSaldo + `), 0)::numeric(14,2)::text,
	COALESCE((SELECT COUNT(*) FROM caja_chica_vale v WHERE v.fondo_id = f.id AND ` + valeElegibleReposicion + `), 0)::int,
	COALESCE((SELECT SUM(v.monto_crc) FROM caja_chica_vale v WHERE v.fondo_id = f.id AND ` + valeElegibleReposicion + `), 0)::numeric(14,2)::text`

const fondoFrom = `FROM caja_chica_fondo f
	LEFT JOIN usuario u ON u.id = f.custodio_id
	LEFT JOIN departamento d ON d.id = f.departamento_id
	LEFT JOIN proveedor p ON p.id = f.proveedor_id`

func scanFondo(row scanner) (FondoCajaChica, error) {
	var f FondoCajaChica
	err := row.Scan(&f.ID, &f.Nombre, &f.CustodioID, &f.Custodio, &f.DepartamentoID, &f.Departamento,
		&f.ProveedorID, &f.Proveedor, &f.MontoAsignado, &f.UmbralPct, &f.LimiteVale, &f.Activo,
		&f.EnVales, &f.ValesPendientes, &f.MontoPendiente)
	if err != nil {
		return f, err
	}
	m, _ := decimal.NewFromString(f.MontoAsignado)
	v, _ := decimal.NewFromString(f.EnVales)
	f.Disponible = m.Sub(v).StringFixed(2)
	return f, nil
}

// ListarFondos devuelve los fondos de la empresa con su saldo derivado. custodioID != "" limita
// a los fondos de ese custodio (scoping del rol Custodio).
func (r *pgRepository) ListarFondos(ctx context.Context, empresaID, custodioID string) ([]FondoCajaChica, error) {
	conds := "f.empresa_id = $1::uuid"
	args := []any{empresaID}
	if custodioID != "" {
		args = append(args, custodioID)
		conds += " AND f.custodio_id = $2::uuid"
	}
	q := "SELECT " + fondoCols + " " + fondoFrom + " WHERE " + conds + " ORDER BY f.activo DESC, f.nombre"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar fondos: %w", err)
	}
	defer rows.Close()
	out := make([]FondoCajaChica, 0)
	for rows.Next() {
		f, err := scanFondo(rows)
		if err != nil {
			return nil, fmt.Errorf("cxp: scan fondo: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FondoPorID trae un fondo (tenant-safe) con su saldo derivado.
func (r *pgRepository) FondoPorID(ctx context.Context, empresaID, id string) (FondoCajaChica, error) {
	q := "SELECT " + fondoCols + " " + fondoFrom + " WHERE f.empresa_id = $1::uuid AND f.id = $2::uuid"
	f, err := scanFondo(r.pool.QueryRow(ctx, q, empresaID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return FondoCajaChica{}, ErrFondoNoEncontrado
	}
	if err != nil {
		return FondoCajaChica{}, fmt.Errorf("cxp: fondo por id: %w", err)
	}
	return f, nil
}

// CrearFondo registra un fondo nuevo (lo constituye el DF).
func (r *pgRepository) CrearFondo(ctx context.Context, empresaID string, in FondoInput) (FondoCajaChica, error) {
	const q = `
		INSERT INTO caja_chica_fondo (empresa_id, nombre, custodio_id, departamento_id, proveedor_id, monto_asignado, umbral_pct, limite_vale)
		VALUES ($1::uuid, $2, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid, $6, $7, $8)
		RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, in.Nombre, in.CustodioID, in.DepartamentoID, in.ProveedorID,
		in.MontoAsignado, in.UmbralPct, in.LimiteVale).Scan(&id)
	if esViolacionUnica(err) {
		return FondoCajaChica{}, ErrFondoDuplicado
	}
	if err != nil {
		return FondoCajaChica{}, fmt.Errorf("cxp: crear fondo: %w", err)
	}
	return r.FondoPorID(ctx, empresaID, id)
}

// ActualizarFondo edita los parámetros del fondo (tenant-safe).
func (r *pgRepository) ActualizarFondo(ctx context.Context, empresaID, id string, in FondoInput) (FondoCajaChica, error) {
	const q = `
		UPDATE caja_chica_fondo
		SET nombre = $3, custodio_id = NULLIF($4, '')::uuid, departamento_id = NULLIF($5, '')::uuid,
		    proveedor_id = NULLIF($6, '')::uuid, monto_asignado = $7, umbral_pct = $8, limite_vale = $9
		WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, id, in.Nombre, in.CustodioID, in.DepartamentoID, in.ProveedorID,
		in.MontoAsignado, in.UmbralPct, in.LimiteVale)
	if esViolacionUnica(err) {
		return FondoCajaChica{}, ErrFondoDuplicado
	}
	if err != nil {
		return FondoCajaChica{}, fmt.Errorf("cxp: actualizar fondo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return FondoCajaChica{}, ErrFondoNoEncontrado
	}
	return r.FondoPorID(ctx, empresaID, id)
}

// DesactivarFondo apaga el fondo (soft; el histórico de vales queda).
func (r *pgRepository) DesactivarFondo(ctx context.Context, empresaID, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE caja_chica_fondo SET activo = false WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, id)
	if err != nil {
		return fmt.Errorf("cxp: desactivar fondo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrFondoNoEncontrado
	}
	return nil
}

// ListarVales trae los vales del fondo (más recientes primero) con su estado derivado.
func (r *pgRepository) ListarVales(ctx context.Context, empresaID, fondoID string) ([]ValeCajaChica, error) {
	q := `
		SELECT v.id::text, v.fondo_id::text, to_char(v.fecha, 'YYYY-MM-DD'), v.detalle, v.monto_crc::text,
		       COALESCE(v.concepto_id::text, ''), COALESCE(c.nombre, ''),
		       COALESCE(v.clasificacion_id::text, ''), COALESCE(cl.nombre, ''),
		       v.comprobante, COALESCE(NULLIF(u.nombre, ''), u.email, ''),
		       COALESCE(v.reposicion_id::text, ''), v.anulado,
		       CASE
		         WHEN v.anulado THEN 'ANULADO'
		         WHEN v.reposicion_id IS NULL THEN 'PENDIENTE'
		         WHEN EXISTS (SELECT 1 FROM documento_cxp r WHERE r.id = v.reposicion_id AND r.estado IN ('PAGADO', 'CONCILIADO')) THEN 'REPUESTO'
		         WHEN EXISTS (SELECT 1 FROM documento_cxp r WHERE r.id = v.reposicion_id AND r.estado IN ('ANULADO', 'DENEGADO', 'LIQUIDADA')) THEN 'PENDIENTE'
		         ELSE 'EN_REPOSICION' END
		FROM caja_chica_vale v
		LEFT JOIN concepto c ON c.id = v.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = v.clasificacion_id
		LEFT JOIN usuario u ON u.id = v.registrado_por
		WHERE v.empresa_id = $1::uuid AND v.fondo_id = $2::uuid
		ORDER BY v.fecha DESC, v.creado_en DESC`
	rows, err := r.pool.Query(ctx, q, empresaID, fondoID)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar vales: %w", err)
	}
	defer rows.Close()
	out := make([]ValeCajaChica, 0)
	for rows.Next() {
		var v ValeCajaChica
		if err := rows.Scan(&v.ID, &v.FondoID, &v.Fecha, &v.Detalle, &v.MontoCRC,
			&v.ConceptoID, &v.Concepto, &v.ClasificacionID, &v.Clasificacion,
			&v.Comprobante, &v.RegistradoPor, &v.ReposicionID, &v.Anulado, &v.Estado); err != nil {
			return nil, fmt.Errorf("cxp: scan vale: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CrearVale registra un vale contra el fondo.
func (r *pgRepository) CrearVale(ctx context.Context, empresaID, fondoID string, in ValeInput, usuarioID string) (string, error) {
	const q = `
		INSERT INTO caja_chica_vale (empresa_id, fondo_id, fecha, detalle, monto_crc, concepto_id, clasificacion_id, subclasificacion_id, comprobante, registrado_por)
		VALUES ($1::uuid, $2::uuid, COALESCE(NULLIF($3, '')::date, CURRENT_DATE), $4, $5,
		        NULLIF($6, '')::uuid, NULLIF($7, '')::uuid, NULLIF($8, '')::uuid, $9, $10::uuid)
		RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, fondoID, in.Fecha, in.Detalle, in.MontoCRC,
		in.ConceptoID, in.ClasificacionID, in.SubclasificacionID, in.Comprobante, usuarioID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("cxp: crear vale: %w", err)
	}
	return id, nil
}

// AnularVale marca el vale como anulado (solo si aún no fue vinculado a una reposición viva).
func (r *pgRepository) AnularVale(ctx context.Context, empresaID, fondoID, valeID string) error {
	q := `
		UPDATE caja_chica_vale v SET anulado = true
		WHERE v.empresa_id = $1::uuid AND v.fondo_id = $2::uuid AND v.id = $3::uuid AND NOT v.anulado
		  AND ` + valeElegibleReposicion
	tag, err := r.pool.Exec(ctx, q, empresaID, fondoID, valeID)
	if err != nil {
		return fmt.Errorf("cxp: anular vale: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var existe bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM caja_chica_vale WHERE empresa_id = $1::uuid AND fondo_id = $2::uuid AND id = $3::uuid AND NOT anulado)`,
		empresaID, fondoID, valeID).Scan(&existe); err != nil {
		return fmt.Errorf("cxp: verificar vale: %w", err)
	}
	if existe {
		return ErrValeYaEnReposicion
	}
	return ErrValeNoEncontrado
}

// ValesElegiblesReposicion suma e identifica los vales listos para agrupar en una reposición.
func (r *pgRepository) ValesElegiblesReposicion(ctx context.Context, empresaID, fondoID string) ([]string, decimal.Decimal, error) {
	q := `
		SELECT v.id::text, v.monto_crc::text FROM caja_chica_vale v
		WHERE v.empresa_id = $1::uuid AND v.fondo_id = $2::uuid AND ` + valeElegibleReposicion
	rows, err := r.pool.Query(ctx, q, empresaID, fondoID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("cxp: vales elegibles: %w", err)
	}
	defer rows.Close()
	var ids []string
	total := decimal.Zero
	for rows.Next() {
		var id, monto string
		if err := rows.Scan(&id, &monto); err != nil {
			return nil, decimal.Zero, fmt.Errorf("cxp: scan vale elegible: %w", err)
		}
		m, err := decimal.NewFromString(monto)
		if err != nil {
			return nil, decimal.Zero, fmt.Errorf("cxp: monto de vale inválido: %w", err)
		}
		ids = append(ids, id)
		total = total.Add(m)
	}
	return ids, total, rows.Err()
}

// VincularValesAReposicion amarra los vales al documento de reposición. El WHERE re-verifica
// elegibilidad para que un vale nunca quede en dos reposiciones vivas (guarda anti-carrera).
func (r *pgRepository) VincularValesAReposicion(ctx context.Context, empresaID, fondoID, docID string, valeIDs []string) (int64, error) {
	q := `
		UPDATE caja_chica_vale v SET reposicion_id = $3::uuid
		WHERE v.empresa_id = $1::uuid AND v.fondo_id = $2::uuid AND v.id = ANY($4::uuid[]) AND ` + valeElegibleReposicion
	tag, err := r.pool.Exec(ctx, q, empresaID, fondoID, docID, valeIDs)
	if err != nil {
		return 0, fmt.Errorf("cxp: vincular vales: %w", err)
	}
	return tag.RowsAffected(), nil
}
