package bancos

// SQL de «Cargar histórico» (mig 0087).

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Estados de una carga histórica (mig 0087).
const (
	EstadoCargaHistoricaPrevisualizada = "PREVISUALIZADA"
	EstadoCargaHistoricaConfirmada     = "CONFIRMADA"
)

// CargaHistoricaRow es el archivo en tránsito entre previsualizar y confirmar.
type CargaHistoricaRow struct {
	ID             string
	NombreArchivo  string
	SourceFileHash string
	Estado         string
	Archivo        []byte
}

func (r *pgRepository) CuentasHistorico(ctx context.Context, empresaID string) ([]CuentaHistorica, error) {
	// Se traen TAMBIÉN las desactivadas a propósito: si el archivo nombra una cuenta que existe
	// pero está apagada, el mensaje tiene que decir «está desactivada, activala» y no «no existe,
	// creala» — porque crearla de nuevo dejaría dos cuentas con el mismo dinero partido.
	const q = `
		SELECT c.id::text, COALESCE(c.alias, ''), b.nombre, COALESCE(c.iban, ''), c.moneda, c.activo
		FROM cuenta_bancaria c
		JOIN banco b ON b.id = c.banco_id
		WHERE c.empresa_id = $1::uuid
		ORDER BY b.nombre, c.alias`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("bancos: cuentas para histórico: %w", err)
	}
	defer rows.Close()
	out := []CuentaHistorica{}
	for rows.Next() {
		var c CuentaHistorica
		if err := rows.Scan(&c.ID, &c.Alias, &c.Banco, &c.IBAN, &c.Moneda, &c.Activo); err != nil {
			return nil, fmt.Errorf("bancos: scan cuenta histórico: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *pgRepository) CrearCargaHistorica(ctx context.Context, empresaID, nombre, hash string, archivo []byte, usuarioID string) (string, error) {
	const q = `
		INSERT INTO carga_historica (empresa_id, nombre_archivo, source_file_hash, archivo, estado, creado_por)
		VALUES ($1::uuid, $2, $3, $4, 'PREVISUALIZADA', NULLIF($5, '')::uuid)
		RETURNING id::text`
	var id string
	if err := r.pool.QueryRow(ctx, q, empresaID, nombre, hash, archivo, usuarioID).Scan(&id); err != nil {
		return "", fmt.Errorf("bancos: crear carga histórica: %w", err)
	}
	return id, nil
}

func (r *pgRepository) CargaHistorica(ctx context.Context, empresaID, cargaID string) (CargaHistoricaRow, error) {
	const q = `
		SELECT id::text, nombre_archivo, source_file_hash, estado, archivo
		FROM carga_historica
		WHERE empresa_id = $1::uuid AND id = $2::uuid`
	var c CargaHistoricaRow
	err := r.pool.QueryRow(ctx, q, empresaID, cargaID).Scan(
		&c.ID, &c.NombreArchivo, &c.SourceFileHash, &c.Estado, &c.Archivo)
	// El filtro por empresa está en el WHERE: una carga de OTRA empresa no existe para esta, y eso
	// es un 404, nunca un 403 (un 403 confirmaría que existe).
	if errors.Is(err, pgx.ErrNoRows) {
		return CargaHistoricaRow{}, ErrCargaHistoricaNoEncontrada
	}
	if err != nil {
		return CargaHistoricaRow{}, fmt.Errorf("bancos: carga histórica por id: %w", err)
	}
	return c, nil
}

// BloqueosDeCargaHistorica pregunta, para los pares (cuenta, año, mes) que toca el archivo, cuáles
// caen en un período CERRADO y cuáles en una cuenta/mes con acta de conciliación FIRMADA.
//
// Devuelve textos ya listos para el mensaje: «2025-03» y «Davivienda Colones 2025-03». El mes se
// arma con lpad y no con to_char porque el resultado es el mismo texto que ya usa el resto del
// módulo para un período.
func (r *pgRepository) BloqueosDeCargaHistorica(ctx context.Context, empresaID string, cuentas []string, anios, meses []int) ([]string, []string, error) {
	if len(cuentas) == 0 {
		return nil, nil, nil
	}
	const q = `
		WITH pares AS (
			SELECT * FROM unnest($2::uuid[], $3::int[], $4::int[]) AS t(cuenta, anio, mes)
		)
		SELECT
		  (SELECT COALESCE(array_agg(DISTINCT lpad(pc.anio::text, 4, '0') || '-' || lpad(pc.mes::text, 2, '0')), ARRAY[]::text[])
		     FROM periodo_cierre pc
		     JOIN pares p ON p.anio = pc.anio AND p.mes = pc.mes
		    WHERE pc.empresa_id = $1::uuid),
		  (SELECT COALESCE(array_agg(DISTINCT COALESCE(cb.alias, '') || ' ' || lpad(a.anio::text, 4, '0') || '-' || lpad(a.mes::text, 2, '0')), ARRAY[]::text[])
		     FROM acta_conciliacion a
		     JOIN pares p ON p.cuenta = a.cuenta_bancaria_id AND p.anio = a.anio AND p.mes = a.mes
		     JOIN cuenta_bancaria cb ON cb.id = a.cuenta_bancaria_id
		    WHERE a.empresa_id = $1::uuid AND a.firmado_en IS NOT NULL)`
	var periodos, actas []string
	if err := r.pool.QueryRow(ctx, q, empresaID, cuentas, anios, meses).Scan(&periodos, &actas); err != nil {
		return nil, nil, fmt.Errorf("bancos: bloqueos de carga histórica: %w", err)
	}
	return periodos, actas, nil
}

// ConfirmarCargaHistorica escribe TODO el archivo en UNA transacción: una importación por cuenta,
// sus movimientos y la marca de confirmada en la carga.
//
// Una sola transacción para las quince cuentas a propósito: un fallo a mitad de camino dejaría ocho
// cuentas cargadas y siete no, y el usuario no tendría forma de saber dónde se cortó. Que cada
// cuenta tenga su PROPIA importación es otra cosa, y es lo que le da la salida: si se equivocó en
// una sola cuenta, revierte esa carga y deja las otras catorce en paz.
func (r *pgRepository) ConfirmarCargaHistorica(ctx context.Context, empresaID, cargaID, nombreArchivo, hash, usuarioID string, lotes []LoteHistoricoCuenta) ([]ResultadoLoteHistorico, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("bancos: begin tx carga histórica: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// La carga PRIMERO: su fila es el candado. Dos confirmaciones simultáneas se serializan acá y
	// la segunda no afecta filas, así que se va con el centinela en vez de cargar todo dos veces.
	const qCarga = `
		UPDATE carga_historica
		   SET estado = 'CONFIRMADA', confirmada_en = now()
		 WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'PREVISUALIZADA'
		 RETURNING id::text`
	var confirmada string
	err = tx.QueryRow(ctx, qCarga, empresaID, cargaID).Scan(&confirmada)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCargaHistoricaYaConfirmada
	}
	if err != nil {
		return nil, fmt.Errorf("bancos: marcar carga histórica confirmada: %w", err)
	}

	// Todas las importaciones nacen con el MISMO nombre_archivo y el MISMO source_file_hash: eso es
	// lo que hace que «Cargas hechas» las muestre juntas como lo que son, una sola subida.
	const qImp = `
		INSERT INTO importacion (empresa_id, cuenta_bancaria_id, source_file_hash, nombre_archivo,
		                         estado, creado_por, banco)
		VALUES ($1::uuid, $2::uuid, $3, $4, 'CONFIRMADA', NULLIF($5, '')::uuid, $6)
		RETURNING id::text`

	// `origen_historico = true`: la columna existía desde la mig 0004 y nunca la escribió nadie.
	// Es exactamente para esto, y es lo que después permite distinguir «esto entró por la carga del
	// histórico» de «esto entró del estado de cuenta del banco».
	//
	// `estado_clasificacion`: REVISADO cuando el archivo trajo la partida. No es cosmético: el
	// motor de reglas solo toca los NO_IDENTIFICADO, así que REVISADO es lo que protege el trabajo
	// de segmentación que el Director ya hizo a mano.
	const qMov = `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, importacion_id, fecha, documento, descripcion,
			 debito, credito, moneda_original, monto_original, monto_crc,
			 concepto_id, clasificacion_id, estado_clasificacion,
			 natural_key, indice_ocurrencia, origen_historico)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11,
		        NULLIF($12, '')::uuid, NULLIF($13, '')::uuid, $14, $15, $16, true)
		ON CONFLICT (empresa_id, natural_key) WHERE NOT excluido_por_reversa DO NOTHING`

	out := make([]ResultadoLoteHistorico, 0, len(lotes))
	for _, lote := range lotes {
		var impID string
		if err := tx.QueryRow(ctx, qImp, empresaID, lote.CuentaID, hash, nombreArchivo, usuarioID, lote.Banco).
			Scan(&impID); err != nil {
			return nil, fmt.Errorf("bancos: crear importación histórica: %w", err)
		}
		insertados := 0
		for _, m := range lote.Movs {
			estado := "NO_IDENTIFICADO"
			if m.ClasificacionID != "" {
				estado = "REVISADO"
			}
			tag, err := tx.Exec(ctx, qMov,
				empresaID, lote.CuentaID, impID, m.Fecha, m.Documento, m.Descripcion,
				m.Debito, m.Credito, lote.Moneda, m.MontoOriginal, m.MontoCRC,
				m.ConceptoID, m.ClasificacionID, estado,
				m.NaturalKey, m.IndiceOcurrencia)
			if err != nil {
				return nil, fmt.Errorf("bancos: insertar movimiento histórico: %w", err)
			}
			insertados += int(tag.RowsAffected())
		}
		out = append(out, ResultadoLoteHistorico{
			CuentaID: lote.CuentaID, ImportacionID: impID, Insertados: insertados,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("bancos: commit carga histórica: %w", err)
	}
	return out, nil
}
