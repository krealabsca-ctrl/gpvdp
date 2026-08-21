package cxp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Repository abstrae el acceso a datos de proveedores.
type Repository interface {
	Crear(ctx context.Context, empresaID string, p ProveedorInput) (Proveedor, error)
	Listar(ctx context.Context, empresaID string, f FiltrosProveedor, page, pageSize int) (ListaProveedores, error)
	PorID(ctx context.Context, empresaID, id string) (Proveedor, error)
	Actualizar(ctx context.Context, empresaID, id string, p ProveedorInput) (Proveedor, error)
	Desactivar(ctx context.Context, empresaID, id string) error

	// Documentos CxP
	CrearDocumento(ctx context.Context, empresaID string, in DocumentoInput, totalCRC decimal.Decimal, tc *decimal.Decimal, usuarioID string) (Documento, error)
	ListarDocumentos(ctx context.Context, empresaID string, f FiltrosDocumentos) (ListaDocumentos, error)
	DocumentoPorID(ctx context.Context, empresaID, id string) (Documento, error)
	CambiarEstado(ctx context.Context, empresaID, id, de, a string) (int64, error)
	CambiarEstadoMulti(ctx context.Context, empresaID, id string, de []string, a string) (int64, error)
	Programar(ctx context.Context, empresaID, id, fecha, huella string) (int64, error)
	Clasificar(ctx context.Context, empresaID, id, conceptoID, clasificacionID, subclasificacionID string) (int64, error)
	AsignarTipo(ctx context.Context, empresaID, id, tipo string) (int64, error)
	GuardarGastoDefault(ctx context.Context, empresaID, proveedorID, conceptoID, clasificacionID, subclasificacionID string) error
	// ResumenBandeja: conteo/monto por fase. deptIDs nil = todas las áreas; no-nil (aun vacío)
	// = solo esos departamentos (scoping del validador de área).
	ResumenBandeja(ctx context.Context, empresaID string, deptIDs []string) ([]FaseBandeja, error)
	ProgramarAprobados(ctx context.Context, empresaID string, ids []string, fecha string) (int64, error)
	AsignarPrioridad(ctx context.Context, empresaID, id, prioridad string) (int64, error)
	GuardarNotaRevision(ctx context.Context, empresaID, id, nota string) error
	RegistrarGastoProveedor(ctx context.Context, empresaID, proveedorID, conceptoID, clasificacionID, subclasificacionID string) error
	AprenderCondicionPago(ctx context.Context, empresaID, proveedorID, condicion string, plazoDias int) error
	GastosDeProveedor(ctx context.Context, empresaID, proveedorID string) ([]GastoFrecuente, error)

	// Catálogo de gasto — 3er nivel (Subclasificación), exclusivo de CxP.
	ListarSubclasificaciones(ctx context.Context, empresaID, clasificacionID string) ([]Subclasificacion, error)
	CrearSubclasificacion(ctx context.Context, empresaID, clasificacionID, nombre string) (Subclasificacion, error)

	// Catálogo de departamentos (centros de costo) por empresa.
	ListarDepartamentos(ctx context.Context, empresaID string, soloActivos bool) ([]Departamento, error)
	CrearDepartamento(ctx context.Context, empresaID string, in DepartamentoInput) (Departamento, error)
	ActualizarDepartamento(ctx context.Context, empresaID, id string, in DepartamentoInput) (Departamento, error)
	DesactivarDepartamento(ctx context.Context, empresaID, id string) error
	EnsureDepartamentos(ctx context.Context) error

	// Validación por departamento (control operativo de área).
	AsignarDepartamentoDoc(ctx context.Context, empresaID, docID, deptoID string) (int64, error)
	EsValidador(ctx context.Context, empresaID, deptoID, usuarioID string) (bool, error)
	// DepartamentosDeUsuario devuelve los IDs de departamento donde el usuario es validador
	// (titular o suplente) en la empresa. Slice no-nil (posiblemente vacío) para poder distinguir
	// "sin áreas" (no ve nada) de "ver todo" (nil).
	DepartamentosDeUsuario(ctx context.Context, empresaID, usuarioID string) ([]string, error)
	ValidarDeptoDoc(ctx context.Context, empresaID, docID, usuarioID, respaldo, nota string) (int64, error)

	// Marca «de Contabilidad» (facturas sin área operativa que las valide).
	MarcarDocumentoContabilidad(ctx context.Context, empresaID, docID string, valor *bool, motivo, usuarioID string) (int64, error)
	MarcarProveedorContabilidad(ctx context.Context, empresaID, proveedorID string, valor bool) (int64, error)
	MarcarConceptoContabilidad(ctx context.Context, empresaID, conceptoID string, valor bool) (int64, error)
	MarcarClasificacionContabilidad(ctx context.Context, empresaID, clasificacionID string, valor bool) (int64, error)
	MarcasContabilidad(ctx context.Context, empresaID string) (MarcasContabilidad, error)
	SellarContabilidad(ctx context.Context, empresaID, docID, motivo string) error

	// Validación de área POR RIESGO (0061/0062): el veredicto se calcula al revisar y se guarda.
	EvaluarValidacion(ctx context.Context, empresaID, docID string) (motivo string, err error)
	ParametrosValidacion(ctx context.Context, empresaID string) ([]ParametroCxP, error)
	EfectoValidacion(ctx context.Context, empresaID string) (EfectoValidacion, error)
	GuardarParametroValidacion(ctx context.Context, empresaID, clave, valor, usuarioID string) (int64, error)
	DevolverDoc(ctx context.Context, empresaID, docID, nota string) (int64, error)
	ListarValidadores(ctx context.Context, empresaID, deptoID string) ([]Validador, error)
	AsignarValidador(ctx context.Context, empresaID, deptoID, usuarioID, rol string) error
	QuitarValidador(ctx context.Context, empresaID, deptoID, usuarioID string) error
	UsuariosEmpresa(ctx context.Context, empresaID string) ([]UsuarioRef, error)
	RegistrarAprobacion(ctx context.Context, empresaID, docID, usuarioID, rol string) error
	RolesAprobaciones(ctx context.Context, empresaID, docID string) ([]string, error)

	// Caja chica (fondo fijo): fondos, vales y reposición.
	ListarFondos(ctx context.Context, empresaID, custodioID string) ([]FondoCajaChica, error)
	FondoPorID(ctx context.Context, empresaID, id string) (FondoCajaChica, error)
	CrearFondo(ctx context.Context, empresaID string, in FondoInput) (FondoCajaChica, error)
	ActualizarFondo(ctx context.Context, empresaID, id string, in FondoInput) (FondoCajaChica, error)
	DesactivarFondo(ctx context.Context, empresaID, id string) error
	ListarVales(ctx context.Context, empresaID, fondoID string) ([]ValeCajaChica, error)
	CrearVale(ctx context.Context, empresaID, fondoID string, in ValeInput, usuarioID string) (string, error)
	AnularVale(ctx context.Context, empresaID, fondoID, valeID string) error
	ValesElegiblesReposicion(ctx context.Context, empresaID, fondoID string) ([]string, decimal.Decimal, error)
	VincularValesAReposicion(ctx context.Context, empresaID, fondoID, docID string, valeIDs []string) (int64, error)

	// Anticipos (netting): billetera del proveedor, aplicar/reversar y neto de la factura.
	AnticiposDisponibles(ctx context.Context, empresaID, proveedorID string) ([]AnticipoSaldo, error)
	AnticiposEmpresa(ctx context.Context, empresaID string) ([]AnticipoSaldo, error)
	SaldoAnticipo(ctx context.Context, empresaID, anticipoID string) (decimal.Decimal, error)
	AplicarAnticipo(ctx context.Context, empresaID, anticipoID, facturaID string, monto decimal.Decimal, usuarioID string) (string, error)
	// AplicarAnticiposLote: varios anticipos a la misma factura, todo-o-nada (una transacción).
	AplicarAnticiposLote(ctx context.Context, empresaID, facturaID string, apps []AplicacionInput, usuarioID string) error
	ReversarAplicacion(ctx context.Context, empresaID, facturaID, aplicacionID, usuarioID string) error
	AplicacionesDeFactura(ctx context.Context, empresaID, facturaID string) ([]AplicacionAnticipo, error)

	// Pagos y conciliación (huella Bancos↔CxP)
	DocumentosParaPago(ctx context.Context, empresaID, fecha string) ([]PagoRow, error)
	ProveedoresPorIdentificacion(ctx context.Context, empresaID string) (map[string]ProveedorIBAN, error)
	ActualizarIBANProveedor(ctx context.Context, empresaID, proveedorID, iban string) error
	DocumentosParaPagoPorIDs(ctx context.Context, empresaID string, ids []string) ([]PagoRow, error)
	DocumentoPorHuella(ctx context.Context, empresaID, huella string) (Documento, error)
	// NetoAPagar es lo que sale del banco por esa factura (mismo cálculo que el archivo de pago).
	NetoAPagar(ctx context.Context, empresaID, docID string) (string, error)

	// Importación de facturación (Excel)
	ClavesExistentes(ctx context.Context, empresaID string, claves []string) (map[string]bool, error)
	ProveedorIDPorIdentificacion(ctx context.Context, empresaID, identificacion string) (string, bool, error)

	// Trazabilidad y dashboard ejecutivo
	HistorialDocumento(ctx context.Context, empresaID, docID string) ([]EventoHistorial, error)
	// DashboardCxP: cartera a hoy + movimiento del período (YYYY-MM). deptIDs recorta al
	// área del usuario (nil = toda la empresa), igual que ResumenBandeja.
	DashboardCxP(ctx context.Context, empresaID, periodo string, deptIDs []string) (DashboardCxP, error)

	// Lotes de pago (corte)
	CrearLote(ctx context.Context, empresaID, fechaCorte string, ids []string, usuarioID string) (LotePago, error)
	ListarLotes(ctx context.Context, empresaID string) ([]LotePago, error)
	DocumentosParaPagoPorLote(ctx context.Context, empresaID, loteID string) ([]PagoRow, error)
	Reintentar(ctx context.Context, empresaID, id string) (int64, error)

	// Comprobante de pago (adjunto + envío)
	GuardarComprobante(ctx context.Context, empresaID, docID, filename, mime string, contenido []byte, usuarioID string) error
	ObtenerComprobante(ctx context.Context, empresaID, docID string) (Comprobante, error)
	ObtenerComprobanteEnvio(ctx context.Context, empresaID, docID string) (ComprobanteEnvio, error)
	MarcarComprobanteEnviado(ctx context.Context, empresaID, docID string) error
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository crea el repository de CxP respaldado por PostgreSQL.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

const proveedorCols = `id::text, nombre, COALESCE(tipo_identificacion, ''), COALESCE(identificacion, ''),
	COALESCE(email, ''), COALESCE(telefono, ''), COALESCE(iban, ''), retencion_renta_pct::text, exento_iva, activo,
	condicion_pago, plazo_credito_dias,
	COALESCE(gasto_concepto_id::text, ''), COALESCE(gasto_clasificacion_id::text, ''), COALESCE(gasto_subclasificacion_id::text, ''),
	COALESCE(departamento, ''), es_contabilidad`

type scanner interface{ Scan(dest ...any) error }

func scanProveedor(row scanner) (Proveedor, error) {
	var p Proveedor
	err := row.Scan(&p.ID, &p.Nombre, &p.TipoIdentificacion, &p.Identificacion, &p.Email,
		&p.Telefono, &p.IBAN, &p.RetencionRentaPct, &p.ExentoIVA, &p.Activo,
		&p.CondicionPago, &p.PlazoCreditoDias,
		&p.GastoConceptoID, &p.GastoClasificacionID, &p.GastoSubclasificacionID,
		&p.Departamento, &p.EsContabilidad)
	return p, err
}

func (r *pgRepository) Crear(ctx context.Context, empresaID string, in ProveedorInput) (Proveedor, error) {
	// El gasto predeterminado se valida contra la empresa (subconsultas): un id ajeno queda NULL.
	const q = `
		INSERT INTO proveedor (empresa_id, nombre, tipo_identificacion, identificacion, email, telefono, iban, retencion_renta_pct, exento_iva, condicion_pago, plazo_credito_dias,
			departamento, gasto_concepto_id, gasto_clasificacion_id, gasto_subclasificacion_id)
		VALUES ($1::uuid, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8, $9, COALESCE(NULLIF($10, ''), 'CONTADO'), $11,
			NULLIF($15, ''),
			(SELECT id FROM concepto WHERE id = NULLIF($12, '')::uuid AND empresa_id = $1::uuid AND visible_cxp),
			(SELECT id FROM clasificacion WHERE id = NULLIF($13, '')::uuid AND empresa_id = $1::uuid),
			(SELECT id FROM subclasificacion WHERE id = NULLIF($14, '')::uuid AND empresa_id = $1::uuid))
		RETURNING ` + proveedorCols
	p, err := scanProveedor(r.pool.QueryRow(ctx, q, empresaID, in.Nombre, in.TipoIdentificacion, in.Identificacion,
		in.Email, in.Telefono, in.IBAN, in.RetencionRentaPct, in.ExentoIVA, in.CondicionPago, in.PlazoCreditoDias,
		in.GastoConceptoID, in.GastoClasificacionID, in.GastoSubclasificacionID, in.Departamento))
	if esViolacionUnica(err) {
		return Proveedor{}, ErrProveedorDuplicado
	}
	if err != nil {
		return Proveedor{}, fmt.Errorf("cxp: crear proveedor: %w", err)
	}
	return p, nil
}

func (r *pgRepository) Listar(ctx context.Context, empresaID string, f FiltrosProveedor, page, pageSize int) (ListaProveedores, error) {
	conds := []string{"empresa_id = $1::uuid"}
	args := []any{empresaID}
	// addArg agrega un parámetro y devuelve su índice ($N) para incrustar en la condición.
	addArg := func(v any) int { args = append(args, v); return len(args) }

	if f.Q != "" {
		n := addArg("%" + f.Q + "%")
		conds = append(conds, fmt.Sprintf("(nombre ILIKE $%d OR identificacion ILIKE $%d)", n, n))
	}
	switch f.Estado {
	case "activo":
		conds = append(conds, "activo = true")
	case "inactivo":
		conds = append(conds, "activo = false")
	}
	switch f.IVA {
	case "grava":
		conds = append(conds, "exento_iva = false")
	case "exento":
		conds = append(conds, "exento_iva = true")
	}
	if f.Condicion == "CONTADO" || f.Condicion == "CREDITO" {
		conds = append(conds, fmt.Sprintf("condicion_pago = $%d", addArg(f.Condicion)))
	}
	switch f.Retencion {
	case "con":
		conds = append(conds, "retencion_renta_pct > 0")
	case "sin":
		conds = append(conds, "retencion_renta_pct = 0")
	}
	switch f.IBAN {
	case "con":
		conds = append(conds, "iban IS NOT NULL AND iban <> ''")
	case "sin":
		conds = append(conds, "(iban IS NULL OR iban = '')")
	}
	switch f.Gasto {
	case "con":
		conds = append(conds, "gasto_concepto_id IS NOT NULL")
	case "sin":
		conds = append(conds, "gasto_concepto_id IS NULL")
	}
	if f.Departamento != "" {
		conds = append(conds, fmt.Sprintf("departamento = $%d", addArg(f.Departamento)))
	}
	where := strings.Join(conds, " AND ")

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM proveedor WHERE "+where, args...).Scan(&total); err != nil {
		return ListaProveedores{}, fmt.Errorf("cxp: contar proveedores: %w", err)
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 100
	}
	if page <= 0 {
		page = 1
	}
	args = append(args, pageSize, (page-1)*pageSize)
	listQ := "SELECT " + proveedorCols + " FROM proveedor WHERE " + where +
		fmt.Sprintf(" ORDER BY nombre LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, listQ, args...)
	if err != nil {
		return ListaProveedores{}, fmt.Errorf("cxp: listar proveedores: %w", err)
	}
	defer rows.Close()
	items := make([]Proveedor, 0, pageSize)
	for rows.Next() {
		p, err := scanProveedor(rows)
		if err != nil {
			return ListaProveedores{}, fmt.Errorf("cxp: scan proveedor: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return ListaProveedores{}, fmt.Errorf("cxp: iterar proveedores: %w", err)
	}
	return ListaProveedores{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r *pgRepository) PorID(ctx context.Context, empresaID, id string) (Proveedor, error) {
	const q = `SELECT ` + proveedorCols + ` FROM proveedor WHERE empresa_id = $1::uuid AND id = $2::uuid`
	p, err := scanProveedor(r.pool.QueryRow(ctx, q, empresaID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Proveedor{}, ErrProveedorNoEncontrado
	}
	if err != nil {
		return Proveedor{}, fmt.Errorf("cxp: proveedor por id: %w", err)
	}
	return p, nil
}

func (r *pgRepository) Actualizar(ctx context.Context, empresaID, id string, in ProveedorInput) (Proveedor, error) {
	const q = `
		UPDATE proveedor
		SET nombre = $3, tipo_identificacion = NULLIF($4, ''), identificacion = NULLIF($5, ''),
		    email = NULLIF($6, ''), telefono = NULLIF($7, ''), iban = NULLIF($8, ''),
		    retencion_renta_pct = $9, exento_iva = $10,
		    condicion_pago = COALESCE(NULLIF($11, ''), 'CONTADO'), plazo_credito_dias = $12,
		    gasto_concepto_id = (SELECT c.id FROM concepto c WHERE c.id = NULLIF($13, '')::uuid AND c.empresa_id = $1::uuid AND c.visible_cxp),
		    gasto_clasificacion_id = (SELECT cl.id FROM clasificacion cl WHERE cl.id = NULLIF($14, '')::uuid AND cl.empresa_id = $1::uuid),
		    gasto_subclasificacion_id = (SELECT sc.id FROM subclasificacion sc WHERE sc.id = NULLIF($15, '')::uuid AND sc.empresa_id = $1::uuid),
		    departamento = NULLIF($16, ''),
		    actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid
		RETURNING ` + proveedorCols
	p, err := scanProveedor(r.pool.QueryRow(ctx, q, empresaID, id, in.Nombre, in.TipoIdentificacion, in.Identificacion,
		in.Email, in.Telefono, in.IBAN, in.RetencionRentaPct, in.ExentoIVA, in.CondicionPago, in.PlazoCreditoDias,
		in.GastoConceptoID, in.GastoClasificacionID, in.GastoSubclasificacionID, in.Departamento))
	if errors.Is(err, pgx.ErrNoRows) {
		return Proveedor{}, ErrProveedorNoEncontrado
	}
	if esViolacionUnica(err) {
		return Proveedor{}, ErrProveedorDuplicado
	}
	if err != nil {
		return Proveedor{}, fmt.Errorf("cxp: actualizar proveedor: %w", err)
	}
	return p, nil
}

func (r *pgRepository) Desactivar(ctx context.Context, empresaID, id string) error {
	const q = `UPDATE proveedor SET activo = false, actualizado_en = now() WHERE empresa_id = $1::uuid AND id = $2::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, id)
	if err != nil {
		return fmt.Errorf("cxp: desactivar proveedor: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProveedorNoEncontrado
	}
	return nil
}

func esViolacionUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
