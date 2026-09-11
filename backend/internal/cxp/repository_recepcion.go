package cxp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// RecepcionNueva es lo que se persiste al recibir un comprobante, antes de interpretarlo.
type RecepcionNueva struct {
	FuenteID      string
	LlaveIdem     string
	Clave         string
	TipoDocumento string
	VersionSchema string
	Receptor      string
	XML           []byte
	PDF           []byte
	PDFFilename   string
	MessageID     string
	Asunto        string
	Remitente     string
	Buzon         string
	Estado        string
	Motivo        string
}

// FiltrosRecepcion filtra la bandeja de recepción.
type FiltrosRecepcion struct {
	Estado string
	Q      string
	Limite int
}

// ArchivoRecepcion es el XML o el PDF originales, para descargarlos del expediente.
type ArchivoRecepcion struct {
	Filename  string
	Mime      string
	Contenido []byte
}

// ResumenRecepcion son los contadores de la bandeja.
type ResumenRecepcion struct {
	Pendientes  int    `json:"pendientes"`
	Procesadas  int    `json:"procesadas"`
	Duplicadas  int    `json:"duplicadas"`
	Parqueadas  int    `json:"parqueadas"`
	Descartadas int    `json:"descartadas"`
	UltimaEn    string `json:"ultima_en,omitempty"`
}

// CedulasDeEmpresa devuelve las cédulas jurídicas que responde la empresa.
//
// Devolver una lista VACÍA es significativo y el llamador tiene que tratarlo como «no se puede
// cotejar», nunca como «cotejó bien»: ver cotejarReceptor.
func (r *pgRepository) CedulasDeEmpresa(ctx context.Context, empresaID string) ([]string, error) {
	const q = `SELECT cedula FROM empresa_cedula WHERE empresa_id = $1::uuid ORDER BY principal DESC, cedula`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: cédulas de la empresa: %w", err)
	}
	defer rows.Close()
	out := []string{} // nunca nil: viaja a JSON
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("cxp: leer cédula: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const fuenteCols = `f.id::text, f.nombre, f.correo, f.activo,
	COALESCE(to_char(f.ultimo_contacto_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
	to_char(f.creado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF')`

func (r *pgRepository) CrearFuente(ctx context.Context, empresaID string, in FuenteInput, tokenHash, usuarioID string) (FuenteRecepcion, error) {
	const q = `
		INSERT INTO cxp_fuente_recepcion (empresa_id, nombre, correo, token_hash, creado_por)
		VALUES ($1::uuid, $2, $3, $4, NULLIF($5,'')::uuid)
		RETURNING id::text, nombre, correo, activo, '', to_char(creado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF')`
	var f FuenteRecepcion
	err := r.pool.QueryRow(ctx, q, empresaID, in.Nombre, in.Correo, tokenHash, usuarioID).
		Scan(&f.ID, &f.Nombre, &f.Correo, &f.Activo, &f.UltimoContacto, &f.CreadoEn)
	if err != nil {
		if esViolacionUnica(err) {
			return FuenteRecepcion{}, ErrFuenteDuplicada
		}
		return FuenteRecepcion{}, fmt.Errorf("cxp: crear fuente: %w", err)
	}
	return f, nil
}

// ListarFuentes trae los buzones de la empresa con su latido y sus contadores.
func (r *pgRepository) ListarFuentes(ctx context.Context, empresaID string) ([]FuenteRecepcion, error) {
	const q = `
		SELECT ` + fuenteCols + `,
		       COUNT(rc.id) FILTER (WHERE rc.id IS NOT NULL)::int,
		       COUNT(rc.id) FILTER (WHERE rc.estado = 'PARQUEADA')::int
		FROM cxp_fuente_recepcion f
		LEFT JOIN cxp_recepcion rc ON rc.fuente_id = f.id
		WHERE f.empresa_id = $1::uuid
		GROUP BY f.id, f.nombre, f.correo, f.activo, f.ultimo_contacto_en, f.creado_en
		ORDER BY f.activo DESC, f.nombre`
	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar fuentes: %w", err)
	}
	defer rows.Close()
	out := []FuenteRecepcion{}
	for rows.Next() {
		var f FuenteRecepcion
		if err := rows.Scan(&f.ID, &f.Nombre, &f.Correo, &f.Activo, &f.UltimoContacto,
			&f.CreadoEn, &f.Recibidas, &f.Parqueadas); err != nil {
			return nil, fmt.Errorf("cxp: leer fuente: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *pgRepository) RotarTokenFuente(ctx context.Context, empresaID, fuenteID, tokenHash string) error {
	const q = `UPDATE cxp_fuente_recepcion SET token_hash = $3
	           WHERE id = $2::uuid AND empresa_id = $1::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, fuenteID, tokenHash)
	if err != nil {
		return fmt.Errorf("cxp: rotar token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrFuenteNoEncontrada
	}
	return nil
}

func (r *pgRepository) CambiarEstadoFuente(ctx context.Context, empresaID, fuenteID string, activo bool) error {
	const q = `UPDATE cxp_fuente_recepcion SET activo = $3
	           WHERE id = $2::uuid AND empresa_id = $1::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, fuenteID, activo)
	if err != nil {
		return fmt.Errorf("cxp: cambiar estado de la fuente: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrFuenteNoEncontrada
	}
	return nil
}

// FuentePorTokenHash resuelve el token: de qué empresa y de qué buzón es.
//
// El `e.activo` del JOIN es el mismo filtro que lleva toda resolución de empresa en internal/auth,
// y acá es imprescindible: sin él, un token de una empresa desactivada seguiría escribiendo, porque
// más abajo nadie vuelve a mirar si la empresa está activa.
func (r *pgRepository) FuentePorTokenHash(ctx context.Context, tokenHash string) (TokenMaquina, error) {
	const q = `
		SELECT f.id::text, f.empresa_id::text, f.nombre, f.correo
		FROM cxp_fuente_recepcion f
		JOIN empresa e ON e.id = f.empresa_id AND e.activo = true
		WHERE f.token_hash = $1 AND f.activo = true`
	var t TokenMaquina
	err := r.pool.QueryRow(ctx, q, tokenHash).Scan(&t.FuenteID, &t.EmpresaID, &t.Nombre, &t.Correo)
	if errors.Is(err, pgx.ErrNoRows) {
		return TokenMaquina{}, ErrTokenRecepcionInvalido
	}
	if err != nil {
		return TokenMaquina{}, fmt.Errorf("cxp: resolver token de recepción: %w", err)
	}
	return t, nil
}

// TocarFuente registra el latido. Se llama en CADA llamada del script, incluso cuando no trajo
// facturas: es lo único que distingue «no hubo facturas» de «el script se murió».
func (r *pgRepository) TocarFuente(ctx context.Context, fuenteID string) error {
	const q = `UPDATE cxp_fuente_recepcion SET ultimo_contacto_en = now() WHERE id = $1::uuid`
	if _, err := r.pool.Exec(ctx, q, fuenteID); err != nil {
		return fmt.Errorf("cxp: registrar latido: %w", err)
	}
	return nil
}

const recepcionCols = `rc.id::text, COALESCE(rc.fuente_id::text,''), COALESCE(f.nombre,''),
	COALESCE(rc.clave,''), COALESCE(rc.tipo_documento,''), COALESCE(rc.version_schema,''),
	COALESCE(rc.receptor,''), COALESCE(rc.message_id,''), COALESCE(rc.asunto,''),
	COALESCE(rc.remitente,''), COALESCE(rc.buzon,''), rc.estado, COALESCE(rc.motivo,''),
	COALESCE(rc.documento_id::text,''), COALESCE(d.consecutivo,''), COALESCE(p.nombre,''),
	COALESCE(d.total::text,''), COALESCE(d.moneda,''), rc.intentos,
	(rc.xml_crudo IS NOT NULL), (rc.pdf IS NOT NULL),
	to_char(rc.creado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
	COALESCE(to_char(rc.procesado_en, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '')`

const recepcionFrom = `
	FROM cxp_recepcion rc
	LEFT JOIN cxp_fuente_recepcion f ON f.id = rc.fuente_id
	LEFT JOIN documento_cxp d ON d.id = rc.documento_id
	LEFT JOIN proveedor p ON p.id = d.proveedor_id`

func escanearRecepcion(row pgx.Row) (Recepcion, error) {
	var r Recepcion
	err := row.Scan(&r.ID, &r.FuenteID, &r.FuenteNombre, &r.Clave, &r.TipoDocumento,
		&r.VersionSchema, &r.Receptor, &r.MessageID, &r.Asunto, &r.Remitente, &r.Buzon,
		&r.Estado, &r.Motivo, &r.DocumentoID, &r.Consecutivo, &r.Proveedor, &r.Total,
		&r.Moneda, &r.Intentos, &r.TieneXML, &r.TienePDF, &r.CreadoEn, &r.ProcesadoEn)
	return r, err
}

// RecepcionPorLlave busca por la llave de idempotencia. `pgx.ErrNoRows` se traduce acá.
func (r *pgRepository) RecepcionPorLlave(ctx context.Context, empresaID, llave string) (Recepcion, error) {
	q := `SELECT ` + recepcionCols + recepcionFrom +
		` WHERE rc.empresa_id = $1::uuid AND rc.idempotency_key = $2`
	rec, err := escanearRecepcion(r.pool.QueryRow(ctx, q, empresaID, llave))
	if errors.Is(err, pgx.ErrNoRows) {
		return Recepcion{}, ErrRecepcionNoEncontrada
	}
	if err != nil {
		return Recepcion{}, fmt.Errorf("cxp: recepción por llave: %w", err)
	}
	return rec, nil
}

// GuardarRecepcion persiste lo recibido. Si la llave ya existe devuelve el id de la fila que ganó,
// que es lo que convierte una carrera entre dos envíos simultáneos en un «repetido» y no en un 500.
func (r *pgRepository) GuardarRecepcion(ctx context.Context, empresaID string, n RecepcionNueva) (string, error) {
	const q = `
		INSERT INTO cxp_recepcion (empresa_id, fuente_id, idempotency_key, clave, tipo_documento,
			version_schema, receptor, xml_crudo, pdf, pdf_filename, message_id, asunto,
			remitente, buzon, estado, motivo, intentos)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''),
			NULLIF($7,''), $8, $9, NULLIF($10,''), NULLIF($11,''), NULLIF($12,''),
			NULLIF($13,''), NULLIF($14,''), $15, NULLIF($16,''), 1)
		ON CONFLICT (empresa_id, idempotency_key)
			WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''
			DO NOTHING
		RETURNING id::text`
	var id string
	err := r.pool.QueryRow(ctx, q, empresaID, n.FuenteID, n.LlaveIdem, n.Clave, n.TipoDocumento,
		n.VersionSchema, n.Receptor, n.XML, n.PDF, n.PDFFilename, n.MessageID, n.Asunto,
		n.Remitente, n.Buzon, n.Estado, n.Motivo).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// El ON CONFLICT DO NOTHING no devuelve fila: ya existía. Se recupera la que ganó.
		existente, e := r.RecepcionPorLlave(ctx, empresaID, n.LlaveIdem)
		if e != nil {
			return "", e
		}
		return existente.ID, nil
	}
	if err != nil {
		return "", fmt.Errorf("cxp: guardar recepción: %w", err)
	}
	return id, nil
}

// ResolverRecepcion fija el resultado. El XML y el PDF se BORRAN cuando la recepción no es de esta
// empresa: dejarlos convertiría la cola de errores en un repositorio de documentos de otra empresa
// —razón social, proveedor, líneas, montos— visible para quien no debe verlos.
func (r *pgRepository) ResolverRecepcion(ctx context.Context, empresaID, id, estado, motivo, documentoID string) error {
	const q = `
		UPDATE cxp_recepcion
		SET estado = $3, motivo = NULLIF($4,''), documento_id = NULLIF($5,'')::uuid,
		    procesado_en = now(),
		    xml_crudo = CASE WHEN $6 THEN NULL ELSE xml_crudo END,
		    pdf       = CASE WHEN $6 THEN NULL ELSE pdf END
		WHERE id = $2::uuid AND empresa_id = $1::uuid`
	borrarContenido := estado == RecParqueada && strings.Contains(motivo, "no corresponde a esta empresa")
	tag, err := r.pool.Exec(ctx, q, empresaID, id, estado, motivo, documentoID, borrarContenido)
	if err != nil {
		return fmt.Errorf("cxp: resolver recepción: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRecepcionNoEncontrada
	}
	return nil
}

func (r *pgRepository) ListarRecepciones(ctx context.Context, empresaID string, f FiltrosRecepcion) ([]Recepcion, error) {
	args := []any{empresaID}
	conds := []string{"rc.empresa_id = $1::uuid"}
	if e := strings.TrimSpace(f.Estado); e != "" {
		args = append(args, e)
		conds = append(conds, fmt.Sprintf("rc.estado = $%d", len(args)))
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		args = append(args, "%"+q+"%")
		conds = append(conds, fmt.Sprintf(
			"(rc.clave ILIKE $%d OR rc.asunto ILIKE $%d OR rc.remitente ILIKE $%d OR p.nombre ILIKE $%d)",
			len(args), len(args), len(args), len(args)))
	}
	limite := f.Limite
	if limite <= 0 || limite > 500 {
		limite = 200
	}
	args = append(args, limite)
	sql := `SELECT ` + recepcionCols + recepcionFrom + ` WHERE ` + strings.Join(conds, " AND ") +
		fmt.Sprintf(` ORDER BY rc.creado_en DESC LIMIT $%d`, len(args))

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("cxp: listar recepciones: %w", err)
	}
	defer rows.Close()
	out := []Recepcion{}
	for rows.Next() {
		rec, err := escanearRecepcion(rows)
		if err != nil {
			return nil, fmt.Errorf("cxp: leer recepción: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *pgRepository) RecepcionPorID(ctx context.Context, empresaID, id string) (Recepcion, error) {
	q := `SELECT ` + recepcionCols + recepcionFrom + ` WHERE rc.empresa_id = $1::uuid AND rc.id = $2::uuid`
	rec, err := escanearRecepcion(r.pool.QueryRow(ctx, q, empresaID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Recepcion{}, ErrRecepcionNoEncontrada
	}
	if err != nil {
		return Recepcion{}, fmt.Errorf("cxp: recepción por id: %w", err)
	}
	return rec, nil
}

// ArchivoDeRecepcion devuelve el XML o el PDF originales. `cual` es "xml" o "pdf".
func (r *pgRepository) ArchivoDeRecepcion(ctx context.Context, empresaID, id, cual string) (ArchivoRecepcion, error) {
	const q = `SELECT COALESCE(clave,'sin-clave'), xml_crudo, pdf, COALESCE(pdf_filename,'')
	           FROM cxp_recepcion WHERE empresa_id = $1::uuid AND id = $2::uuid`
	var clave, pdfName string
	var xml, pdf []byte
	err := r.pool.QueryRow(ctx, q, empresaID, id).Scan(&clave, &xml, &pdf, &pdfName)
	if errors.Is(err, pgx.ErrNoRows) {
		return ArchivoRecepcion{}, ErrRecepcionNoEncontrada
	}
	if err != nil {
		return ArchivoRecepcion{}, fmt.Errorf("cxp: archivo de recepción: %w", err)
	}
	if cual == "pdf" {
		if len(pdf) == 0 {
			return ArchivoRecepcion{}, ErrRecepcionNoEncontrada
		}
		nombre := pdfName
		if nombre == "" {
			nombre = clave + ".pdf"
		}
		return ArchivoRecepcion{Filename: nombre, Mime: "application/pdf", Contenido: pdf}, nil
	}
	if len(xml) == 0 {
		return ArchivoRecepcion{}, ErrRecepcionNoEncontrada
	}
	return ArchivoRecepcion{Filename: clave + ".xml", Mime: "application/xml", Contenido: xml}, nil
}

func (r *pgRepository) ResumenRecepcion(ctx context.Context, empresaID string) (ResumenRecepcion, error) {
	const q = `
		SELECT COUNT(*) FILTER (WHERE estado = 'PENDIENTE')::int,
		       COUNT(*) FILTER (WHERE estado = 'PROCESADA')::int,
		       COUNT(*) FILTER (WHERE estado = 'DUPLICADA')::int,
		       COUNT(*) FILTER (WHERE estado = 'PARQUEADA')::int,
		       COUNT(*) FILTER (WHERE estado = 'DESCARTADA')::int,
		       COALESCE(to_char(max(creado_en), 'YYYY-MM-DD"T"HH24:MI:SSOF'), '')
		FROM cxp_recepcion WHERE empresa_id = $1::uuid`
	var res ResumenRecepcion
	err := r.pool.QueryRow(ctx, q, empresaID).Scan(&res.Pendientes, &res.Procesadas,
		&res.Duplicadas, &res.Parqueadas, &res.Descartadas, &res.UltimaEn)
	if err != nil {
		return ResumenRecepcion{}, fmt.Errorf("cxp: resumen de recepción: %w", err)
	}
	return res, nil
}

// ClaveEnOtraEmpresa dice si esa clave ya se registró en OTRA empresa del grupo.
//
// Es la única consulta de CxP que mira fuera de la empresa, y existe por una razón de dinero: el
// UNIQUE es (empresa_id, clave), así que la misma factura puede convertirse en dos cuentas por
// pagar —una por sociedad—, cada una con su huella y su archivo de banco, y el proveedor cobraría
// dos veces. Devuelve solo un booleano: NO dice en qué empresa, para no filtrar información entre
// empresas por el texto de un error.
func (r *pgRepository) ClaveEnOtraEmpresa(ctx context.Context, empresaID, clave string) (bool, error) {
	const q = `SELECT EXISTS (
		SELECT 1 FROM documento_cxp WHERE clave = $2 AND empresa_id <> $1::uuid)`
	var existe bool
	if err := r.pool.QueryRow(ctx, q, empresaID, clave).Scan(&existe); err != nil {
		return false, fmt.Errorf("cxp: clave en otra empresa: %w", err)
	}
	return existe, nil
}

// UsuarioTecnicoRecepcion devuelve el id del usuario con el que la máquina firma lo que crea.
//
// Hace falta porque `documento_cxp.creado_por` entra al INSERT como `$14::uuid` sin NULLIF —un
// usuario vacío hace fallar la creación— y porque `auditoria_evento.usuario_id` es FK a usuario:
// sin una fila real, el evento de auditoría se perdería en silencio.
func (r *pgRepository) UsuarioTecnicoRecepcion(ctx context.Context) (string, error) {
	const q = `SELECT id::text FROM usuario WHERE email = 'recepcion-facturas@sistema.local'`
	var id string
	err := r.pool.QueryRow(ctx, q).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("cxp: falta el usuario técnico de recepción (migración 0081)")
	}
	if err != nil {
		return "", fmt.Errorf("cxp: usuario técnico de recepción: %w", err)
	}
	return id, nil
}
