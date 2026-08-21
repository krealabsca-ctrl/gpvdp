package cxc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// CobroInput es un cobro a registrar. Lo usan por igual el archivo, la API y la caja.
type CobroInput struct {
	// Contrato vacío = cobro SIN IDENTIFICAR: entra y espera en su bandeja.
	Contrato       string
	Consecutivo    string
	FechaPago      string
	FechaBancaria  string
	FechaRegistro  string
	Monto          decimal.Decimal
	FormaPago      string
	Asociacion     string
	PlanillaID     string
	Referencia     string
	Concepto       string
	Origen         string
	IdempotencyKey string
	// Destinos: si viene, se aplica a ESOS cargos en ese orden (vía de excepción).
	// Si viene vacío, se aplica FIFO al más viejo primero.
	Destinos []string
}

// CobroRegistrado es el resultado de registrar un cobro.
type CobroRegistrado struct {
	ID           string       `json:"id"`
	Contrato     string       `json:"contrato"`
	Consecutivo  string       `json:"consecutivo"`
	Monto        string       `json:"monto"`
	Estado       string       `json:"estado"`
	Aplicaciones []Aplicacion `json:"aplicaciones"`
	SaldoAFavor  string       `json:"saldo_a_favor"`
	// Repetido: el cobro ya existía (por consecutivo o por llave de idempotencia) y se
	// devuelve el que ya estaba en vez de crear otro.
	Repetido bool `json:"repetido"`
	aplicado decimal.Decimal
}

var (
	ErrCobroNoEncontrado = errors.New("cxc: cobro no encontrado")
	ErrContratoAjeno     = errors.New("cxc: el contrato no existe en esta empresa")
	// ErrCobroYaIdentificado: identificar uno ya aplicado movería plata de un contrato a
	// otro sin rastro. Es una regla de negocio (422), no una falla del servidor.
	ErrCobroYaIdentificado = errors.New("cxc: el cobro ya está identificado; para corregirlo hay que reversarlo y volver a registrarlo")
)

// RegistrarCobro escribe el cobro, lo aplica a los cargos y actualiza sus estados, TODO en
// una transacción. Si algo falla, no queda ni el cobro ni media aplicación: en cuentas por
// cobrar, un cobro registrado sin aplicar es plata que nadie encuentra.
//
// Es IDEMPOTENTE por dos caminos: la llave de idempotencia (API) y el par
// (contrato, consecutivo) del archivo. Reenviar el mismo cobro devuelve el mismo resultado
// con `repetido: true`, no un duplicado.
func (r *pgRepository) RegistrarCobro(ctx context.Context, empresaID string, in CobroInput, usuarioID string) (CobroRegistrado, error) {
	if in.Monto.Sign() <= 0 {
		return CobroRegistrado{}, ErrMontoInvalido
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CobroRegistrado{}, fmt.Errorf("cxc: begin cobro: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// ── ¿Ya existe? Se pregunta ANTES de escribir para poder devolver el mismo resultado.
	if ya, err := cobroExistente(ctx, tx, empresaID, in); err != nil {
		return CobroRegistrado{}, err
	} else if ya != nil {
		return *ya, nil
	}

	// ── Contrato (si viene). Sin contrato el cobro entra sin identificar.
	var contratoID string
	estado := CobroSinIdentificar
	if in.Contrato != "" {
		err := tx.QueryRow(ctx,
			`SELECT id::text FROM contrato_cxc WHERE empresa_id = $1::uuid AND numero = $2`,
			empresaID, in.Contrato).Scan(&contratoID)
		if errors.Is(err, pgx.ErrNoRows) {
			// El contrato del archivo no está en la cartera: NO se descarta el cobro. La
			// plata entró de verdad, así que entra sin identificar con su referencia para
			// que alguien lo resuelva.
			contratoID = ""
		} else if err != nil {
			return CobroRegistrado{}, fmt.Errorf("cxc: buscar contrato %s: %w", in.Contrato, err)
		} else {
			estado = CobroAplicado
		}
	}

	// ── Catálogos por nombre (el archivo trae texto, no ids).
	formaID, err := idPorNombre(ctx, tx, "cxc_forma_pago", empresaID, in.FormaPago)
	if err != nil {
		return CobroRegistrado{}, err
	}
	// La asociación SÍ se crea si no está en el catálogo, igual que en el importador de
	// cartera: es un nombre del negocio, no una regla. Sin esto los cobros del canal
	// dominante quedaban sin asociación y el panorama mostraba ₡0 cobrado para todas —
	// exactamente lo que pasó con los archivos reales del usuario: la cartera nombraba
	// ASEPAN y los pagos nombraban ADEPSA, ASEPRO, ANASSAS, COOPEANDE, ASETAMESIS…
	//
	// La forma de pago NO se crea a propósito: gobierna el factor de recuperación de la cola
	// (decisión de la fase 1), así que una desconocida tiene que pasar por revisión humana.
	asocID, err := idPorNombreCreando(ctx, tx, "cxc_asociacion", empresaID, in.Asociacion)
	if err != nil {
		return CobroRegistrado{}, err
	}

	// ── Aplicación. Los cargos se leen con FOR UPDATE: dos cobros simultáneos del mismo
	// contrato no pueden aplicarse ambos al mismo saldo.
	var res ResultadoAplicacion
	if contratoID != "" {
		cargos, err := cargosAbiertosParaAplicar(ctx, tx, empresaID, contratoID)
		if err != nil {
			return CobroRegistrado{}, err
		}
		if len(in.Destinos) > 0 {
			res, err = AplicarADestino(in.Monto, cargos, in.Destinos)
		} else {
			res, err = AplicarFIFO(in.Monto, cargos)
		}
		if err != nil {
			return CobroRegistrado{}, err
		}
	} else {
		// Sin contrato no se aplica nada: el monto entero queda pendiente de identificar.
		res = ResultadoAplicacion{Aplicaciones: []Aplicacion{}, Aplicado: decimal.Zero, SaldoAFavor: in.Monto}
	}

	// ── Cabecera del cobro
	var cobroID string
	err = tx.QueryRow(ctx, `
		INSERT INTO cobro_cxc (
			empresa_id, contrato_id, consecutivo, fecha_pago, fecha_bancaria, fecha_registro,
			monto, saldo_a_favor, forma_pago_id, asociacion_id, planilla_id, referencia,
			concepto_origen, contrato_origen, origen, estado, idempotency_key, creado_por
		) VALUES (
			$1::uuid, NULLIF($2,'')::uuid, $3, $4::date, NULLIF($5,'')::date, NULLIF($6,'')::date,
			$7::numeric, $8::numeric, NULLIF($9,'')::uuid, NULLIF($10,'')::uuid, NULLIF($11,'')::uuid, $12,
			$13, $14, $15, $16, NULLIF($17,''), NULLIF($18,'')::uuid
		) RETURNING id::text`,
		empresaID, contratoID, in.Consecutivo, in.FechaPago, in.FechaBancaria, in.FechaRegistro,
		in.Monto.String(), res.SaldoAFavor.String(), formaID, asocID, in.PlanillaID, in.Referencia,
		in.Concepto, in.Contrato, origenODefecto(in.Origen), estado, in.IdempotencyKey, usuarioID,
	).Scan(&cobroID)
	if err != nil {
		return CobroRegistrado{}, fmt.Errorf("cxc: insertar cobro: %w", err)
	}

	// ── Aplicaciones + estado de cada cargo, en la misma transacción.
	for _, a := range res.Aplicaciones {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cobro_aplicacion (empresa_id, cobro_id, cargo_id, monto, parcial)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::numeric, $5)`,
			empresaID, cobroID, a.CargoID, a.Monto.String(), a.Parcial); err != nil {
			return CobroRegistrado{}, fmt.Errorf("cxc: insertar aplicación: %w", err)
		}
		// El estado se deriva de los montos con la MISMA función que usa la reversa, para
		// que ir y volver llegue al mismo lugar. El CHECK de la tabla impide pasarse.
		if _, err := tx.Exec(ctx, `
			UPDATE cargo_cxc
			SET monto_aplicado = monto_aplicado + $3::numeric,
			    estado = CASE
			        WHEN monto_aplicado + $3::numeric >= monto THEN 'SALDADO'
			        WHEN monto_aplicado + $3::numeric > 0 THEN 'PARCIAL'
			        ELSE 'ABIERTO' END,
			    actualizado_en = now()
			WHERE empresa_id = $1::uuid AND id = $2::uuid`,
			empresaID, a.CargoID, a.Monto.String()); err != nil {
			return CobroRegistrado{}, fmt.Errorf("cxc: actualizar cargo: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return CobroRegistrado{}, fmt.Errorf("cxc: commit cobro: %w", err)
	}
	return CobroRegistrado{
		ID: cobroID, Contrato: in.Contrato, Consecutivo: in.Consecutivo,
		Monto: in.Monto.String(), Estado: estado,
		Aplicaciones: res.Aplicaciones, SaldoAFavor: res.SaldoAFavor.String(),
		aplicado: res.Aplicado,
	}, nil
}

// cobroExistente busca un cobro ya registrado por llave de idempotencia o por
// (contrato, consecutivo), y devuelve su resultado para poder responder lo mismo.
func cobroExistente(ctx context.Context, tx pgx.Tx, empresaID string, in CobroInput) (*CobroRegistrado, error) {
	var (
		id, estado   string
		monto, favor decimal.Decimal
	)
	buscar := func(q string, args ...any) (bool, error) {
		err := tx.QueryRow(ctx, q, args...).Scan(&id, &estado, &monto, &favor)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("cxc: buscar cobro existente: %w", err)
		}
		return true, nil
	}
	hallado := false
	var err error
	if in.IdempotencyKey != "" {
		hallado, err = buscar(`
			SELECT id::text, estado, monto, saldo_a_favor FROM cobro_cxc
			WHERE empresa_id = $1::uuid AND idempotency_key = $2`, empresaID, in.IdempotencyKey)
		if err != nil {
			return nil, err
		}
	}
	if !hallado && in.Contrato != "" && in.Consecutivo != "" {
		hallado, err = buscar(`
			SELECT k.id::text, k.estado, k.monto, k.saldo_a_favor
			FROM cobro_cxc k
			JOIN contrato_cxc c ON c.id = k.contrato_id
			WHERE k.empresa_id = $1::uuid AND c.numero = $2 AND k.consecutivo = $3`,
			empresaID, in.Contrato, in.Consecutivo)
		if err != nil {
			return nil, err
		}
	}
	if !hallado {
		return nil, nil
	}
	// Se devuelven también sus aplicaciones: la respuesta de un reenvío tiene que ser
	// idéntica a la del primer envío, no un «ok» pelado.
	aps, err := aplicacionesDe(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return &CobroRegistrado{
		ID: id, Contrato: in.Contrato, Consecutivo: in.Consecutivo,
		Monto: monto.String(), Estado: estado, Aplicaciones: aps,
		SaldoAFavor: favor.String(), Repetido: true,
	}, nil
}

func aplicacionesDe(ctx context.Context, tx pgx.Tx, cobroID string) ([]Aplicacion, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.cargo_id::text, g.periodo, a.monto, a.parcial, g.estado
		FROM cobro_aplicacion a JOIN cargo_cxc g ON g.id = a.cargo_id
		WHERE a.cobro_id = $1::uuid ORDER BY g.vence_en`, cobroID)
	if err != nil {
		return nil, fmt.Errorf("cxc: aplicaciones del cobro: %w", err)
	}
	defer rows.Close()
	out := []Aplicacion{}
	for rows.Next() {
		var a Aplicacion
		if err := rows.Scan(&a.CargoID, &a.Periodo, &a.Monto, &a.Parcial, &a.EstadoFinal); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// cargosAbiertosParaAplicar lee los cargos con saldo BLOQUEÁNDOLOS. Sin el FOR UPDATE, dos
// cobros simultáneos del mismo contrato podrían aplicarse los dos al mismo saldo y dejar el
// cargo sobre-aplicado (el CHECK lo rechazaría, pero con un error feo en vez de una espera).
func cargosAbiertosParaAplicar(ctx context.Context, tx pgx.Tx, empresaID, contratoID string) ([]CargoParaAplicar, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, periodo, vence_en, monto, monto_aplicado
		FROM cargo_cxc
		WHERE empresa_id = $1::uuid AND contrato_id = $2::uuid
		  AND estado IN ('ABIERTO','PARCIAL') AND monto > monto_aplicado
		ORDER BY vence_en, periodo
		FOR UPDATE`, empresaID, contratoID)
	if err != nil {
		return nil, fmt.Errorf("cxc: cargos para aplicar: %w", err)
	}
	defer rows.Close()
	out := []CargoParaAplicar{}
	for rows.Next() {
		var (
			c     CargoParaAplicar
			vence time.Time
		)
		if err := rows.Scan(&c.ID, &c.Periodo, &vence, &c.Monto, &c.Aplicado); err != nil {
			return nil, fmt.Errorf("cxc: scan cargo para aplicar: %w", err)
		}
		c.VenceEn = vence.Format("2006-01-02")
		out = append(out, c)
	}
	return out, rows.Err()
}

// ReversarCobro deshace un cobro (cheque devuelto, débito rechazado, contracargo): NO borra
// nada, marca el cobro como reversado y devuelve sus cargos al saldo que tenían. Los cargos
// vuelven a abrirse CON SU ANTIGÜEDAD ORIGINAL, que es lo que hace que la mora no se
// «limpie» por un pago que en realidad no entró.
func (r *pgRepository) ReversarCobro(ctx context.Context, empresaID, cobroID, motivo, usuarioID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cxc: begin reversa: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var estado string
	err = tx.QueryRow(ctx,
		`SELECT estado FROM cobro_cxc WHERE empresa_id = $1::uuid AND id = $2::uuid FOR UPDATE`,
		empresaID, cobroID).Scan(&estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCobroNoEncontrado
	}
	if err != nil {
		return fmt.Errorf("cxc: leer cobro a reversar: %w", err)
	}
	if estado == CobroReversado {
		return ErrCobroYaReversado
	}

	// Devolver lo aplicado a cada cargo y recalcular su estado con el mismo criterio.
	if _, err := tx.Exec(ctx, `
		UPDATE cargo_cxc g
		SET monto_aplicado = g.monto_aplicado - a.monto,
		    estado = CASE
		        WHEN g.monto_aplicado - a.monto >= g.monto THEN 'SALDADO'
		        WHEN g.monto_aplicado - a.monto > 0 THEN 'PARCIAL'
		        ELSE 'ABIERTO' END,
		    actualizado_en = now()
		FROM cobro_aplicacion a
		WHERE a.cargo_id = g.id AND a.cobro_id = $2::uuid AND g.empresa_id = $1::uuid`,
		empresaID, cobroID); err != nil {
		return fmt.Errorf("cxc: desaplicar cargos: %w", err)
	}
	// Las aplicaciones se borran: el rastro de que existieron queda en el cobro reversado y
	// en auditoría. Dejarlas colgando haría que el cargo pareciera pagado en los reportes.
	if _, err := tx.Exec(ctx,
		`DELETE FROM cobro_aplicacion WHERE cobro_id = $1::uuid AND empresa_id = $2::uuid`,
		cobroID, empresaID); err != nil {
		return fmt.Errorf("cxc: borrar aplicaciones: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cobro_cxc
		SET estado = 'REVERSADO', saldo_a_favor = 0, reversado_por = NULLIF($3,'')::uuid,
		    reversado_en = now(), reversa_motivo = $4, actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, cobroID, usuarioID, motivo); err != nil {
		return fmt.Errorf("cxc: marcar cobro reversado: %w", err)
	}
	return tx.Commit(ctx)
}

// IdentificarCobro asigna un contrato a un cobro que entró sin identificar y lo aplica.
func (r *pgRepository) IdentificarCobro(ctx context.Context, empresaID, cobroID, numeroContrato, usuarioID string) (CobroRegistrado, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CobroRegistrado{}, fmt.Errorf("cxc: begin identificar: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var monto decimal.Decimal
	var estado string
	err = tx.QueryRow(ctx,
		`SELECT monto, estado FROM cobro_cxc WHERE empresa_id = $1::uuid AND id = $2::uuid FOR UPDATE`,
		empresaID, cobroID).Scan(&monto, &estado)
	if errors.Is(err, pgx.ErrNoRows) {
		return CobroRegistrado{}, ErrCobroNoEncontrado
	}
	if err != nil {
		return CobroRegistrado{}, fmt.Errorf("cxc: leer cobro: %w", err)
	}
	if estado != CobroSinIdentificar {
		return CobroRegistrado{}, fmt.Errorf("%w (está %s)", ErrCobroYaIdentificado, estado)
	}

	var contratoID string
	err = tx.QueryRow(ctx,
		`SELECT id::text FROM contrato_cxc WHERE empresa_id = $1::uuid AND numero = $2`,
		empresaID, numeroContrato).Scan(&contratoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return CobroRegistrado{}, ErrContratoAjeno
	}
	if err != nil {
		return CobroRegistrado{}, fmt.Errorf("cxc: buscar contrato: %w", err)
	}

	cargos, err := cargosAbiertosParaAplicar(ctx, tx, empresaID, contratoID)
	if err != nil {
		return CobroRegistrado{}, err
	}
	res, err := AplicarFIFO(monto, cargos)
	if err != nil {
		return CobroRegistrado{}, err
	}
	for _, a := range res.Aplicaciones {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cobro_aplicacion (empresa_id, cobro_id, cargo_id, monto, parcial)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::numeric, $5)`,
			empresaID, cobroID, a.CargoID, a.Monto.String(), a.Parcial); err != nil {
			return CobroRegistrado{}, fmt.Errorf("cxc: aplicar al identificar: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cargo_cxc
			SET monto_aplicado = monto_aplicado + $3::numeric,
			    estado = CASE
			        WHEN monto_aplicado + $3::numeric >= monto THEN 'SALDADO'
			        WHEN monto_aplicado + $3::numeric > 0 THEN 'PARCIAL'
			        ELSE 'ABIERTO' END,
			    actualizado_en = now()
			WHERE empresa_id = $1::uuid AND id = $2::uuid`,
			empresaID, a.CargoID, a.Monto.String()); err != nil {
			return CobroRegistrado{}, fmt.Errorf("cxc: actualizar cargo al identificar: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cobro_cxc SET contrato_id = $2::uuid, estado = 'APLICADO',
		    saldo_a_favor = $3::numeric, actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $4::uuid`,
		empresaID, contratoID, res.SaldoAFavor.String(), cobroID); err != nil {
		return CobroRegistrado{}, fmt.Errorf("cxc: asignar contrato al cobro: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CobroRegistrado{}, fmt.Errorf("cxc: commit identificar: %w", err)
	}
	return CobroRegistrado{
		ID: cobroID, Contrato: numeroContrato, Monto: monto.String(), Estado: CobroAplicado,
		Aplicaciones: res.Aplicaciones, SaldoAFavor: res.SaldoAFavor.String(),
	}, nil
}

// ---- Helpers ----

func idPorNombre(ctx context.Context, tx pgx.Tx, tabla, empresaID, nombre string) (string, error) {
	if nombre == "" {
		return "", nil
	}
	var id string
	q := fmt.Sprintf(`SELECT id::text FROM %s WHERE empresa_id = $1::uuid AND lower(nombre) = lower($2)`, tabla)
	err := tx.QueryRow(ctx, q, empresaID, nombre).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// Un nombre que no está en el catálogo NO detiene el cobro: la plata entró igual.
		// Queda sin ese dato y el reporte lo muestra.
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cxc: resolver %s %q: %w", tabla, nombre, err)
	}
	return id, nil
}

func origenODefecto(o string) string {
	switch o {
	case "ARCHIVO", "API", "CAJA", "BANCO", "PLANILLA":
		return o
	default:
		return "ARCHIVO"
	}
}

// ---- Consulta de cobros ----

// FiltrosCobros son los filtros de la lista de cobros y de la bandeja de sin identificar.
type FiltrosCobros struct {
	Q            string
	Contrato     string
	AsociacionID string
	Estado       string
	Desde, Hasta string
	// SinIdentificar acota a la bandeja del dinero que entró y no se sabe de quién.
	SinIdentificar bool
	Page, PageSize int
}

// CobroFila es una fila de la lista de cobros.
type CobroFila struct {
	ID       string `json:"id"`
	Contrato string `json:"contrato"`
	// ContratoOrigen: lo que decía el archivo. Cuando no resolvió, es la pista del operador.
	ContratoOrigen string `json:"contrato_origen"`
	Cliente        string `json:"cliente"`
	Consecutivo    string `json:"consecutivo"`
	FechaPago      string `json:"fecha_pago"`
	FechaBancaria  string `json:"fecha_bancaria"`
	Monto          string `json:"monto"`
	Aplicado       string `json:"aplicado"`
	SaldoAFavor    string `json:"saldo_a_favor"`
	FormaPago      string `json:"forma_pago"`
	Asociacion     string `json:"asociacion"`
	Referencia     string `json:"referencia"`
	Concepto       string `json:"concepto_origen"`
	Estado         string `json:"estado"`
	Origen         string `json:"origen"`
	Periodos       string `json:"periodos"`
}

// ResumenCobros son los totales del filtro activo (mismo patrón que el resto del ERP: el
// encabezado mide lo que se está viendo).
type ResumenCobros struct {
	Cobros         int    `json:"cobros"`
	Monto          string `json:"monto"`
	Aplicado       string `json:"aplicado"`
	SaldoAFavor    string `json:"saldo_a_favor"`
	SinIdentificar int    `json:"sin_identificar"`
	Reversados     int    `json:"reversados"`
}

type ListaCobros struct {
	Resumen  ResumenCobros `json:"resumen"`
	Items    []CobroFila   `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

func (r *pgRepository) ListarCobros(ctx context.Context, empresaID string, f FiltrosCobros) (ListaCobros, error) {
	conds := []string{"k.empresa_id = $1::uuid"}
	args := []any{empresaID}
	add := func(v any) int { args = append(args, v); return len(args) }

	if f.SinIdentificar {
		conds = append(conds, "k.estado = 'SIN_IDENTIFICAR'")
	} else if f.Estado != "" {
		conds = append(conds, fmt.Sprintf("k.estado = $%d", add(f.Estado)))
	}
	if f.Contrato != "" {
		conds = append(conds, fmt.Sprintf("c.numero = $%d", add(f.Contrato)))
	}
	if f.AsociacionID != "" {
		conds = append(conds, fmt.Sprintf("k.asociacion_id = $%d::uuid", add(f.AsociacionID)))
	}
	// El rango se filtra por la fecha BANCARIA: es la que concilia contra Bancos y la que
	// responde «cuándo entró la plata». Las otras dos se muestran pero no filtran.
	if f.Desde != "" {
		conds = append(conds, fmt.Sprintf("COALESCE(k.fecha_bancaria, k.fecha_pago) >= $%d::date", add(f.Desde)))
	}
	if f.Hasta != "" {
		conds = append(conds, fmt.Sprintf("COALESCE(k.fecha_bancaria, k.fecha_pago) <= $%d::date", add(f.Hasta)))
	}
	if f.Q != "" {
		n := add("%" + f.Q + "%")
		conds = append(conds, fmt.Sprintf(
			"(k.consecutivo ILIKE $%d OR k.referencia ILIKE $%d OR c.numero ILIKE $%d OR c.cliente_nombre ILIKE $%d OR k.contrato_origen ILIKE $%d)", n, n, n, n, n))
	}
	where := strings.Join(conds, " AND ")
	const desde = `
		FROM cobro_cxc k
		LEFT JOIN contrato_cxc c ON c.id = k.contrato_id
		LEFT JOIN cxc_forma_pago fp ON fp.id = k.forma_pago_id
		LEFT JOIN cxc_asociacion a ON a.id = k.asociacion_id`

	var res ResumenCobros
	var monto, aplicado, favor decimal.Decimal
	err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int, COALESCE(sum(k.monto),0),
		       COALESCE(sum((SELECT COALESCE(sum(x.monto),0) FROM cobro_aplicacion x WHERE x.cobro_id = k.id)),0),
		       COALESCE(sum(k.saldo_a_favor),0),
		       count(*) FILTER (WHERE k.estado = 'SIN_IDENTIFICAR')::int,
		       count(*) FILTER (WHERE k.estado = 'REVERSADO')::int`+desde+` WHERE `+where, args...).
		Scan(&res.Cobros, &monto, &aplicado, &favor, &res.SinIdentificar, &res.Reversados)
	if err != nil {
		return ListaCobros{}, fmt.Errorf("cxc: resumen de cobros: %w", err)
	}
	res.Monto, res.Aplicado, res.SaldoAFavor = monto.String(), aplicado.String(), favor.String()

	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	args = append(args, pageSize, (page-1)*pageSize)
	q := `
		SELECT k.id::text, COALESCE(c.numero,''), k.contrato_origen, COALESCE(c.cliente_nombre,''), k.consecutivo,
		       k.fecha_pago, k.fecha_bancaria, k.monto,
		       (SELECT COALESCE(sum(x.monto),0) FROM cobro_aplicacion x WHERE x.cobro_id = k.id),
		       k.saldo_a_favor, COALESCE(fp.nombre,''), COALESCE(a.nombre,''), k.referencia,
		       k.concepto_origen, k.estado, k.origen,
		       COALESCE((SELECT string_agg(g.periodo, ' + ' ORDER BY g.vence_en)
		                 FROM cobro_aplicacion x JOIN cargo_cxc g ON g.id = x.cargo_id
		                 WHERE x.cobro_id = k.id), '')` + desde + `
		WHERE ` + where + `
		ORDER BY COALESCE(k.fecha_bancaria, k.fecha_pago) DESC, k.creado_en DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return ListaCobros{}, fmt.Errorf("cxc: listar cobros: %w", err)
	}
	defer rows.Close()
	items := make([]CobroFila, 0, pageSize)
	for rows.Next() {
		var (
			k             CobroFila
			fPago         time.Time
			fBanco        *time.Time
			mon, apl, fav decimal.Decimal
		)
		if err := rows.Scan(&k.ID, &k.Contrato, &k.ContratoOrigen, &k.Cliente, &k.Consecutivo, &fPago, &fBanco, &mon,
			&apl, &fav, &k.FormaPago, &k.Asociacion, &k.Referencia, &k.Concepto, &k.Estado, &k.Origen,
			&k.Periodos); err != nil {
			return ListaCobros{}, fmt.Errorf("cxc: scan cobro: %w", err)
		}
		k.FechaPago = fPago.Format("2006-01-02")
		k.FechaBancaria = fechaOVacio(fBanco)
		k.Monto, k.Aplicado, k.SaldoAFavor = mon.String(), apl.String(), fav.String()
		items = append(items, k)
	}
	if err := rows.Err(); err != nil {
		return ListaCobros{}, err
	}
	return ListaCobros{Resumen: res, Items: items, Total: res.Cobros, Page: page, PageSize: pageSize}, nil
}

// idPorNombreCreando resuelve el catálogo por nombre y lo CREA si no existe.
//
// Se usa solo para la asociación: es un nombre del negocio (la asociación existe, la conozca
// o no el catálogo) y perder el dato hace inútil el panorama del canal. Es idempotente: dos
// cobros de la misma asociación nueva crean una sola entrada.
func idPorNombreCreando(ctx context.Context, tx pgx.Tx, tabla, empresaID, nombre string) (string, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return "", nil
	}
	id, err := idPorNombre(ctx, tx, tabla, empresaID, nombre)
	if err != nil || id != "" {
		return id, err
	}
	q := fmt.Sprintf(`
		INSERT INTO %s (empresa_id, nombre) VALUES ($1::uuid, $2)
		ON CONFLICT (empresa_id, nombre) DO UPDATE SET nombre = EXCLUDED.nombre
		RETURNING id::text`, tabla)
	if err := tx.QueryRow(ctx, q, empresaID, nombre).Scan(&id); err != nil {
		return "", fmt.Errorf("cxc: crear %s %q: %w", tabla, nombre, err)
	}
	return id, nil
}
