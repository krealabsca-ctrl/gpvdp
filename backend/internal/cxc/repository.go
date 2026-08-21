package cxc

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Repository abstrae el acceso a datos de CxC.
type Repository interface {
	// Catálogos
	Catalogos(ctx context.Context, empresaID string) (Catalogos, error)
	Parametros(ctx context.Context, empresaID string) (map[string]string, error)
	TramoDe(ctx context.Context, empresaID string, dias int) (Tramo, error)
	SedesDeUsuario(ctx context.Context, empresaID, usuarioID string) ([]string, error)

	// Importación de cartera
	ResolverCatalogo(ctx context.Context, empresaID string, filas []FilaContrato) (Resolucion, error)
	GuardarContratos(ctx context.Context, empresaID string, filas []FilaContrato, res Resolucion) (Aplicado, error)
	// NumerosExistentes dice cuáles de esos números de contrato ya están en la cartera:
	// es lo que separa «carga inicial» de «actualización diaria» en el reporte.
	NumerosExistentes(ctx context.Context, empresaID string, numeros []string) (map[string]bool, error)
	CrearImportacion(ctx context.Context, empresaID, tipo, archivo, usuarioID string, r Conciliacion) (string, error)
	ConfirmarImportacion(ctx context.Context, empresaID, id string) error

	// Contratos y cargos
	ListarContratos(ctx context.Context, empresaID string, f FiltrosContratos) (ListaContratos, error)
	ContratoPorNumero(ctx context.Context, empresaID, numero string) (Contrato, error)
	CargosDeContrato(ctx context.Context, empresaID, contratoID string, soloAbiertos bool) ([]Cargo, error)
	// ContratosParaGenerar trae lo mínimo para correr el generador sobre toda la cartera.
	ContratosParaGenerar(ctx context.Context, empresaID string, sedeIDs []string) ([]ContratoGenerable, error)
	// InsertarCargos es idempotente: ON CONFLICT (contrato_id, periodo) DO NOTHING.
	InsertarCargos(ctx context.Context, empresaID string, cargos []CargoAInsertar) (int, error)

	// Cobros (fase 2). Registrar aplica en la MISMA transacción; reversar devuelve los
	// cargos a su saldo con la antigüedad original.
	RegistrarCobro(ctx context.Context, empresaID string, in CobroInput, usuarioID string) (CobroRegistrado, error)
	ReversarCobro(ctx context.Context, empresaID, cobroID, motivo, usuarioID string) error
	IdentificarCobro(ctx context.Context, empresaID, cobroID, numeroContrato, usuarioID string) (CobroRegistrado, error)
	ListarCobros(ctx context.Context, empresaID string, f FiltrosCobros) (ListaCobros, error)
	// PanoramaAsociaciones cruza los cargos que vencen contra los cobros que llegaron, por
	// asociación: es lo que separa «el cliente no pagó» de «la asociación no envió planilla».
	PanoramaAsociaciones(ctx context.Context, empresaID, periodo string, tolerancia decimal.Decimal) (PanoramaAsociaciones, error)

	// Gestión de cobro (fase 3). La cola se ordena por valor esperado y el cumplimiento de
	// las promesas se deriva de los cobros; de ahí los dos parámetros.
	ColaDeCobro(ctx context.Context, empresaID string, f FiltrosCola, p ParametrosCola) (ListaCola, error)
	CatalogosGestion(ctx context.Context, empresaID string) (CatalogosGestion, error)
	RegistrarGestion(ctx context.Context, empresaID string, in GestionInput, usuarioID string) (GestionRegistrada, error)
	GestionesDeContrato(ctx context.Context, empresaID, contratoID string, tolPromesa int) ([]GestionFila, error)

	// Planillas de asociación: el tercer contraste contra el depósito bancario. El monto NO
	// se captura: sale del movimiento que ya está en Bancos y que el operador vincula.
	AbrirPlanilla(ctx context.Context, empresaID, asociacionID, periodo, referencia, nota, usuarioID string) (string, error)
	VincularDeposito(ctx context.Context, empresaID, planillaID, movimientoID, usuarioID string) error
	DesvincularDeposito(ctx context.Context, empresaID, planillaID, movimientoID string) error
	CandidatosDeposito(ctx context.Context, empresaID, planillaID string, margenDias int) ([]CandidatoDeposito, error)
	PlanillaDeAsociacion(ctx context.Context, empresaID, asociacionID, periodo string, tolerancia decimal.Decimal) (PlanillaDetalle, error)
	DatosDePlanilla(ctx context.Context, empresaID, planillaID string) (asociacionID, periodo string, err error)

	// Notas de crédito: bajar deuda sin que entre plata. Se aplican con el MISMO motor FIFO
	// que los cobros, así que todo lo que deriva el saldo las toma en cuenta sin cambios.
	EmitirNotaCredito(ctx context.Context, empresaID string, in NotaCreditoInput, usuarioID string) (NotaCredito, error)
	AnularNotaCredito(ctx context.Context, empresaID, notaID, motivo, usuarioID string) error
	NotaCredito(ctx context.Context, empresaID, notaID string) (NotaCredito, error)
	ListarNotas(ctx context.Context, empresaID string, f FiltrosNotas) (ListaNotas, error)

	// Suspensión por mora: 18 MESES de mora, o su equivalencia en cuotas según la modalidad.
	// El contador se deriva de los cargos; el corte lo autoriza una persona, no el sistema.
	EstadoDeSuspension(ctx context.Context, empresaID, numero string, topeMeses int) (EstadoSuspension, error)
	Suspender(ctx context.Context, empresaID, numero, motivo, usuarioID string, topeMeses int) (EstadoSuspension, error)
	Reactivar(ctx context.Context, empresaID, numero, motivo, usuarioID string, topeMeses int) (EstadoSuspension, error)

	// Arreglos de pago: un plan ENCIMA de la deuda, sin reescribir los cargos. El cumplimiento
	// se deriva de los cobros; quebrarlo lo decide el supervisor de piso.
	PactarArreglo(ctx context.Context, empresaID string, in ArregloInput, esExcepcion bool, usuarioID string, topeMeses int) (Arreglo, error)
	ArregloPorID(ctx context.Context, empresaID, id string, tol int) (Arreglo, error)
	ListarArreglos(ctx context.Context, empresaID string, f FiltrosArreglos, tol int) (ListaArreglos, error)
	CerrarArreglo(ctx context.Context, empresaID, id, motivo, usuarioID string, quebrar bool, tol int) (Arreglo, error)

	// Contacto preventivo: el universo que la cola excluye a propósito.
	ListaPreventiva(ctx context.Context, empresaID string, f FiltrosPreventivo, dias, diasTarjeta int) (ListaPreventiva, error)

	// Configuración del módulo: parámetros, tramos, factores, sedes y la frontera de datos.
	// Sin esto todo eso solo se podía cambiar con un UPDATE a mano en la base.
	ConfigCxC(ctx context.Context, empresaID string) (ConfigCxC, error)
	GuardarParametros(ctx context.Context, empresaID string, valores map[string]string, usuarioID string) (int, error)
	ActualizarTramo(ctx context.Context, empresaID, codigo string, c CambioTramo) error
	ActualizarFormaPago(ctx context.Context, empresaID, id string, factor *string, activa *bool) error
	CrearSede(ctx context.Context, empresaID, nombre, razonSocial, plaza string) (SedeConfig, error)
	ActualizarSede(ctx context.Context, empresaID, id string, nombre *string, activa *bool) error
	AsignarSedes(ctx context.Context, empresaID, usuarioID string, sedeIDs []string) error
}

// Resolucion son los identificadores del catálogo que hicieron falta para una
// importación, y los que hubo que crear. Se muestra en la previsualización: el usuario
// ve qué sedes y asociaciones nuevas van a nacer ANTES de confirmar.
type Resolucion struct {
	Sedes        map[string]string `json:"sedes"`        // nombre → id
	Modalidades  map[string]string `json:"modalidades"`  // nombre → id
	FormasPago   map[string]string `json:"formas_pago"`  // nombre → id
	Asociaciones map[string]string `json:"asociaciones"` // nombre → id
	// Nuevas: las entradas que NO existían y se crearían.
	SedesNuevas        []string `json:"sedes_nuevas"`
	AsociacionesNuevas []string `json:"asociaciones_nuevas"`
	ModalidadesNuevas  []string `json:"modalidades_desconocidas"`
	FormasPagoNuevas   []string `json:"formas_pago_desconocidas"`
}

// Aplicado es el resultado de escribir los contratos.
type Aplicado struct {
	Nuevos       int `json:"nuevos"`
	Actualizados int `json:"actualizados"`
}

// ContratoGenerable es lo que el generador necesita de cada contrato.
type ContratoGenerable struct {
	ID          string
	Numero      string
	PrimerCobro string
	DiaPago     int
	Cuota       decimal.Decimal
	MesesCiclo  int
	Quincenal   bool
}

// CargoAInsertar es un cargo listo para escribirse.
type CargoAInsertar struct {
	ContratoID string
	Periodo    string
	VenceEn    string
	Monto      decimal.Decimal
	Origen     string
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository construye el repositorio PostgreSQL de CxC.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

// ---- Catálogos ----

func (r *pgRepository) Catalogos(ctx context.Context, empresaID string) (Catalogos, error) {
	out := Catalogos{
		Sedes: []ItemCatalogo{}, Modalidades: []ItemCatalogo{},
		FormasPago: []ItemCatalogo{}, Asociaciones: []ItemCatalogo{}, Tramos: []Tramo{},
	}
	// Cada catálogo viene con su conteo de contratos: un selector con «San José (31 240)»
	// dice más que uno con solo el nombre.
	type spec struct {
		tabla   string
		columna string
		dest    *[]ItemCatalogo
		activo  string
	}
	for _, sp := range []spec{
		{"cxc_sede", "sede_id", &out.Sedes, "activa"},
		{"cxc_modalidad", "modalidad_id", &out.Modalidades, "activa"},
		{"cxc_forma_pago", "forma_pago_id", &out.FormasPago, "activa"},
		{"cxc_asociacion", "asociacion_id", &out.Asociaciones, "activa"},
	} {
		q := fmt.Sprintf(`
			SELECT c.id::text, c.nombre, count(k.id)::int
			FROM %s c
			LEFT JOIN contrato_cxc k ON k.%s = c.id AND k.empresa_id = c.empresa_id
			WHERE c.empresa_id = $1::uuid AND c.%s = true
			GROUP BY c.id, c.nombre
			ORDER BY c.nombre`, sp.tabla, sp.columna, sp.activo)
		rows, err := r.pool.Query(ctx, q, empresaID)
		if err != nil {
			return Catalogos{}, fmt.Errorf("cxc: catálogo %s: %w", sp.tabla, err)
		}
		for rows.Next() {
			var it ItemCatalogo
			if err := rows.Scan(&it.ID, &it.Nombre, &it.Contratos); err != nil {
				rows.Close()
				return Catalogos{}, fmt.Errorf("cxc: scan %s: %w", sp.tabla, err)
			}
			*sp.dest = append(*sp.dest, it)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return Catalogos{}, err
		}
	}
	rows, err := r.pool.Query(ctx, `
		SELECT codigo, etiqueta, dias_min, dias_max, prob_recuperacion
		FROM cxc_tramo WHERE empresa_id = $1::uuid ORDER BY orden`, empresaID)
	if err != nil {
		return Catalogos{}, fmt.Errorf("cxc: tramos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t Tramo
		var prob decimal.Decimal
		if err := rows.Scan(&t.Codigo, &t.Etiqueta, &t.DiasMin, &t.DiasMax, &prob); err != nil {
			return Catalogos{}, fmt.Errorf("cxc: scan tramo: %w", err)
		}
		t.Prob = prob.String()
		out.Tramos = append(out.Tramos, t)
	}
	return out, rows.Err()
}

func (r *pgRepository) Parametros(ctx context.Context, empresaID string) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT clave, valor FROM cxc_parametro WHERE empresa_id = $1::uuid`, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxc: parámetros: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (r *pgRepository) TramoDe(ctx context.Context, empresaID string, dias int) (Tramo, error) {
	var t Tramo
	var prob decimal.Decimal
	err := r.pool.QueryRow(ctx, `
		SELECT codigo, etiqueta, dias_min, dias_max, prob_recuperacion
		FROM cxc_tramo
		WHERE empresa_id = $1::uuid AND $2::int BETWEEN dias_min AND dias_max
		LIMIT 1`, empresaID, dias).Scan(&t.Codigo, &t.Etiqueta, &t.DiasMin, &t.DiasMax, &prob)
	if err != nil {
		return Tramo{}, fmt.Errorf("cxc: tramo de %d días: %w", dias, err)
	}
	t.Prob = prob.String()
	return t, nil
}

func (r *pgRepository) SedesDeUsuario(ctx context.Context, empresaID, usuarioID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT sede_id::text FROM cxc_usuario_sede WHERE empresa_id = $1::uuid AND usuario_id = $2::uuid`,
		empresaID, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("cxc: sedes del usuario: %w", err)
	}
	defer rows.Close()
	// Se devuelve una lista NO nil aunque esté vacía: vacía significa «no ve nada», que
	// es distinto de nil («ve todo»). Confundirlas filtraría la cartera completa.
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ---- Importación ----

// ResolverCatalogo mira qué nombres del archivo ya existen en el catálogo de la empresa
// y cuáles habría que crear. NO escribe nada: es para la previsualización.
func (r *pgRepository) ResolverCatalogo(ctx context.Context, empresaID string, filas []FilaContrato) (Resolucion, error) {
	res := Resolucion{
		Sedes: map[string]string{}, Modalidades: map[string]string{},
		FormasPago: map[string]string{}, Asociaciones: map[string]string{},
	}
	// Nombres distintos que trae el archivo.
	sedes, modalidades, formas, asociaciones := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, f := range filas {
		if n := nombreSede(f); n != "" {
			sedes[n] = true
		}
		if f.Modalidad != "" {
			modalidades[f.Modalidad] = true
		}
		if f.FormaPago != "" {
			formas[f.FormaPago] = true
		}
		if f.Asociacion != "" {
			asociaciones[f.Asociacion] = true
		}
	}
	cargar := func(tabla string, nombres map[string]bool, dest map[string]string, nuevas *[]string) error {
		if len(nombres) == 0 {
			return nil
		}
		lista := claves(nombres)
		q := fmt.Sprintf(`SELECT nombre, id::text FROM %s WHERE empresa_id = $1::uuid AND lower(nombre) = ANY($2::text[])`, tabla)
		enMinuscula := make([]string, len(lista))
		for i, n := range lista {
			enMinuscula[i] = strings.ToLower(n)
		}
		rows, err := r.pool.Query(ctx, q, empresaID, enMinuscula)
		if err != nil {
			return fmt.Errorf("cxc: resolver %s: %w", tabla, err)
		}
		defer rows.Close()
		encontrados := map[string]string{}
		for rows.Next() {
			var nombre, id string
			if err := rows.Scan(&nombre, &id); err != nil {
				return err
			}
			encontrados[strings.ToLower(nombre)] = id
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, n := range lista {
			if id, ok := encontrados[strings.ToLower(n)]; ok {
				dest[n] = id
			} else {
				*nuevas = append(*nuevas, n)
			}
		}
		return nil
	}
	if err := cargar("cxc_sede", sedes, res.Sedes, &res.SedesNuevas); err != nil {
		return Resolucion{}, err
	}
	if err := cargar("cxc_modalidad", modalidades, res.Modalidades, &res.ModalidadesNuevas); err != nil {
		return Resolucion{}, err
	}
	if err := cargar("cxc_forma_pago", formas, res.FormasPago, &res.FormasPagoNuevas); err != nil {
		return Resolucion{}, err
	}
	if err := cargar("cxc_asociacion", asociaciones, res.Asociaciones, &res.AsociacionesNuevas); err != nil {
		return Resolucion{}, err
	}
	return res, nil
}

// GuardarContratos escribe la cartera en UNA transacción: o entra el archivo completo o
// no entra nada. A medio importar 70 000 contratos, dejar la mitad sería peor que fallar.
//
// Las sedes y asociaciones que no existían se CREAN (son nombres del negocio, no
// catálogos cerrados). Las modalidades y formas de pago desconocidas NO se crean: son
// las que gobiernan el ciclo de cobro y el factor de recuperación, así que la fila queda
// en revisión y alguien decide a qué modalidad equivale.
func (r *pgRepository) GuardarContratos(ctx context.Context, empresaID string, filas []FilaContrato, res Resolucion) (Aplicado, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Aplicado{}, fmt.Errorf("cxc: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	crear := func(tabla, nombre string, extra map[string]any) (string, error) {
		cols := []string{"empresa_id", "nombre"}
		vals := []any{empresaID, nombre}
		ph := []string{"$1::uuid", "$2"}
		for k, v := range extra {
			cols = append(cols, k)
			vals = append(vals, v)
			ph = append(ph, fmt.Sprintf("$%d", len(vals)))
		}
		q := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s)
			ON CONFLICT (empresa_id, nombre) DO UPDATE SET nombre = EXCLUDED.nombre
			RETURNING id::text`, tabla, strings.Join(cols, ", "), strings.Join(ph, ", "))
		var id string
		if err := tx.QueryRow(ctx, q, vals...).Scan(&id); err != nil {
			return "", fmt.Errorf("cxc: crear %s %q: %w", tabla, nombre, err)
		}
		return id, nil
	}

	// Sedes nuevas: se crean con su razón social y plaza ya separadas.
	for _, n := range res.SedesNuevas {
		var razon, plaza string
		for _, f := range filas {
			if nombreSede(f) == n {
				razon, plaza = f.RazonSocial, f.Plaza
				break
			}
		}
		id, err := crear("cxc_sede", n, map[string]any{"razon_social": razon, "plaza": plaza})
		if err != nil {
			return Aplicado{}, err
		}
		res.Sedes[n] = id
	}
	for _, n := range res.AsociacionesNuevas {
		id, err := crear("cxc_asociacion", n, nil)
		if err != nil {
			return Aplicado{}, err
		}
		res.Asociaciones[n] = id
	}

	var out Aplicado
	const upsert = `
		INSERT INTO contrato_cxc (
			empresa_id, numero, sede_id, cliente_nombre, documento, telefonos, correos,
			servicio, tipo_servicio, modalidad_id, forma_pago_id, asociacion_id,
			dia_pago, cuota_vigente, fecha_inicial, fecha_primer_cobro, tarjeta_vence,
			score_origen, estado_origen, morosidad_origen, dias_vencidos_origen, saldo_origen,
			revision_pendiente, revision_motivo
		) VALUES (
			$1::uuid, $2, NULLIF($3,'')::uuid, $4, $5, $6, $7,
			$8, $9, NULLIF($10,'')::uuid, NULLIF($11,'')::uuid, NULLIF($12,'')::uuid,
			NULLIF($13,'')::smallint, $14::numeric, NULLIF($15,'')::date, NULLIF($16,'')::date, NULLIF($17,'')::date,
			NULLIF($18,'')::int, $19, $20, NULLIF($21,'')::int, NULLIF($22,'')::numeric,
			$23, $24
		)
		ON CONFLICT (empresa_id, numero) DO UPDATE SET
			sede_id = COALESCE(EXCLUDED.sede_id, contrato_cxc.sede_id),
			cliente_nombre = EXCLUDED.cliente_nombre,
			documento = EXCLUDED.documento,
			telefonos = EXCLUDED.telefonos,
			correos = EXCLUDED.correos,
			servicio = EXCLUDED.servicio,
			tipo_servicio = EXCLUDED.tipo_servicio,
			modalidad_id = COALESCE(EXCLUDED.modalidad_id, contrato_cxc.modalidad_id),
			forma_pago_id = COALESCE(EXCLUDED.forma_pago_id, contrato_cxc.forma_pago_id),
			asociacion_id = COALESCE(EXCLUDED.asociacion_id, contrato_cxc.asociacion_id),
			dia_pago = COALESCE(EXCLUDED.dia_pago, contrato_cxc.dia_pago),
			cuota_vigente = EXCLUDED.cuota_vigente,
			fecha_inicial = COALESCE(EXCLUDED.fecha_inicial, contrato_cxc.fecha_inicial),
			-- La fecha de primer cobro NO se sobreescribe con nulo: es el ancla del ciclo
			-- de cargos y perderla desalinearía todos los períodos del contrato.
			fecha_primer_cobro = COALESCE(EXCLUDED.fecha_primer_cobro, contrato_cxc.fecha_primer_cobro),
			tarjeta_vence = COALESCE(EXCLUDED.tarjeta_vence, contrato_cxc.tarjeta_vence),
			score_origen = EXCLUDED.score_origen,
			estado_origen = EXCLUDED.estado_origen,
			morosidad_origen = EXCLUDED.morosidad_origen,
			dias_vencidos_origen = EXCLUDED.dias_vencidos_origen,
			saldo_origen = EXCLUDED.saldo_origen,
			revision_pendiente = EXCLUDED.revision_pendiente,
			revision_motivo = EXCLUDED.revision_motivo,
			actualizado_en = now()
		RETURNING (xmax = 0) AS insertado`

	for _, f := range filas {
		motivos := strings.Join(f.Motivos, " · ")
		// Una modalidad desconocida deja el contrato sin ciclo: no se le pueden generar
		// cargos, así que entra marcado aunque el resto del dato esté perfecto.
		modID := res.Modalidades[f.Modalidad]
		if f.Modalidad != "" && modID == "" {
			motivos = agregarMotivo(motivos, "modalidad desconocida: "+f.Modalidad)
		}
		formaID := res.FormasPago[f.FormaPago]
		if f.FormaPago != "" && formaID == "" {
			motivos = agregarMotivo(motivos, "forma de pago desconocida: "+f.FormaPago)
		}
		var insertado bool
		err := tx.QueryRow(ctx, upsert,
			empresaID, f.Numero, res.Sedes[nombreSede(f)], f.Cliente, f.Documento, f.Telefonos, f.Correos,
			f.Servicio, f.TipoServicio, modID, formaID, res.Asociaciones[f.Asociacion],
			textoODefecto(f.DiaPago), f.Cuota.String(), f.FechaInicial, f.PrimerCobro, f.TarjetaVence,
			punteroInt(f.ScoreOrigen), f.EstadoOrigen, f.MorosidadOrigen, punteroInt(f.DiasVencidosOrigen),
			punteroDecimal(f.SaldoOrigen), motivos != "", motivos,
		).Scan(&insertado)
		if err != nil {
			return Aplicado{}, fmt.Errorf("cxc: guardar contrato %s: %w", f.Numero, err)
		}
		if insertado {
			out.Nuevos++
		} else {
			out.Actualizados++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Aplicado{}, fmt.Errorf("cxc: commit: %w", err)
	}
	return out, nil
}

func (r *pgRepository) CrearImportacion(ctx context.Context, empresaID, tipo, archivo, usuarioID string, c Conciliacion) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO cxc_importacion (empresa_id, tipo, archivo, estado, filas, nuevos, actualizados, duplicados, cuarentena, reporte, creado_por)
		VALUES ($1::uuid, $2, $3, 'PREVISUALIZADA', $4, $5, $6, $7, $8, $9, NULLIF($10,'')::uuid)
		RETURNING id::text`,
		empresaID, tipo, archivo, c.Filas, c.Nuevos, c.Actualizados, c.Duplicados, c.Cuarentena, c, usuarioID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("cxc: registrar importación: %w", err)
	}
	return id, nil
}

func (r *pgRepository) ConfirmarImportacion(ctx context.Context, empresaID, id string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE cxc_importacion SET estado = 'CONFIRMADA', confirmado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND estado = 'PREVISUALIZADA'`, empresaID, id)
	if err != nil {
		return fmt.Errorf("cxc: confirmar importación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrImportacionAjena
	}
	return nil
}

// ---- Helpers de escritura ----

// nombreSede es el nombre con el que vive la sede en el catálogo. Se usa el campo crudo
// del archivo para que dos plazas de razones sociales distintas no se pisen.
func nombreSede(f FilaContrato) string { return f.SedeCruda }

func agregarMotivo(actual, nuevo string) string {
	if actual == "" {
		return nuevo
	}
	return actual + " · " + nuevo
}

func textoODefecto(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func punteroInt(p *int) string {
	if p == nil {
		return ""
	}
	return fmt.Sprint(*p)
}

func punteroDecimal(p *decimal.Decimal) string {
	if p == nil {
		return ""
	}
	return p.String()
}

func claves(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

var _ = pgx.ErrNoRows

func (r *pgRepository) NumerosExistentes(ctx context.Context, empresaID string, numeros []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(numeros) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT numero FROM contrato_cxc WHERE empresa_id = $1::uuid AND numero = ANY($2::text[])`,
		empresaID, numeros)
	if err != nil {
		return nil, fmt.Errorf("cxc: números existentes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}
