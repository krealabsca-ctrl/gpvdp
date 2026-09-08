package inventario

// Consignación: la mercadería del proveedor que ya salió de la bodega.
//
// El estado de esta pantalla es DERIVADO, como la existencia: no hay ninguna columna «pagado» ni
// ningún contador. Una unidad consignada está pendiente cuando salió de la bodega y todavía no tiene
// una cuenta por pagar enlazada, y deja de estarlo cuando la tiene. Si mañana alguien anula esa
// factura en CxP, la unidad vuelve sola a la cola sin que este módulo tenga que enterarse.
//
// La alternativa —marcar la unidad como «pagada»— se descartó por lo mismo de siempre en este
// módulo: dos lugares guardando el mismo hecho terminan discrepando, y el que miente es justo el
// que nadie mira.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// sqlSalidaDeLaUnidad es el movimiento con el que la unidad dejó la bodega. Se elige el ÚLTIMO por
// fecha porque una unidad puede tener varios movimientos (entró, se trasladó, se usó) y el que
// interesa es el que la sacó definitivamente. Espera la unidad aliaseada como `u`.
const sqlSalidaDeLaUnidad = `
	(SELECT m.id FROM inv_movimiento m
	 WHERE m.unidad_id = u.id AND m.tipo IN ('SALIDA','BAJA')
	 ORDER BY m.fecha DESC, m.creado_en DESC LIMIT 1)`

// sqlDeudaVigente es la condición de «todavía se le debe al proveedor», y es la razón de ser de esta
// pantalla. Vive en una constante porque la usan la LISTA y el RESUMEN: cuando cada una llevaba su
// propia condición, la cabecera y la tabla se contradecían.
//
// La sutileza que costó un defecto: NO alcanza con `cxp_consignacion_id IS NULL`. Si alguien anula la
// provisión desde CxP, la columna sigue apuntando a un documento sin efecto y la unidad quedaba fuera
// de la cola: la deuda desaparecía del número principal y la unidad no se podía volver a facturar. El
// estado se DERIVA del documento, no de la presencia del enlace.
const sqlDeudaVigente = `
	(u.cxp_consignacion_id IS NULL
	 OR NOT EXISTS (SELECT 1 FROM documento_cxp dv
	                WHERE dv.id = u.cxp_consignacion_id AND dv.empresa_id = u.empresa_id
	                  AND dv.estado <> 'ANULADO'))`

// sqlSalioDeLaBodega son los estados en los que la unidad ya no está para vender. EN_TRANSITO queda
// afuera porque mudarla de sede no es usarla.
const sqlSalioDeLaBodega = `(u.estado NOT IN ` + estadosQueCuentan + ` AND u.estado <> 'EN_TRANSITO')`

// ConsignadasPendientes lista lo consignado que salió de bodega, con qué lo sacó y si ya se facturó.
//
// Incluye a propósito los cuatro destinos posibles y no solo el uso: una unidad del proveedor que se
// dañó, que el conteo no encontró o que se devolvió también salió de la bodega, y alguien tiene que
// decidir qué se hace con ella. Dejar afuera los casos incómodos es lo que hace que se olviden.
func (r *pgRepository) ConsignadasPendientes(ctx context.Context, empresaID string, f FiltroConsignacion) ([]ConsignadaSalida, error) {
	q := `
		SELECT u.id::text, u.numero, a.nombre,
		       CASE WHEN cp.nombre IS NULL THEN c.nombre ELSE cp.nombre || ' › ' || c.nombre END,
		       u.estado, u.costo_crc::text,
		       COALESCE(u.proveedor_id::text, ''), COALESCE(p.nombre, ''),
		       COALESCE(s.nombre, ''),
		       COALESCE(to_char(mov.fecha, 'YYYY-MM-DD'), ''),
		       COALESCE(mov.tipo, ''), COALESCE(mov.motivo, ''),
		       COALESCE(sv.numero, ''), COALESCE(sv.a_nombre_de, ''),
		       COALESCE(u.cxp_consignacion_id::text, ''),
		       COALESCE(d.consecutivo, ''), COALESCE(d.estado, ''),
		       COALESCE(d.total_crc::text, ''),
		       COALESCE(u.documento_cxp_id::text, ''),
		       -- La deuda sale de la MISMA expresión que usan el filtro y el resumen. Derivarla otra
		       -- vez en Go fue el defecto de siempre: el SQL contaba la unidad como deuda vigente y
		       -- la pantalla la mostraba como «facturada» porque miraba solo si había un enlace.
		       ` + sqlDeudaVigente + `,
		       -- ¿Lo enlazado es la provisión que generó el sistema, o ya es la factura real?
		       -- Se reconoce por la clave determinística con la que se creó. Sin esta distinción la
		       -- pantalla ofrecía «conciliar» sobre una unidad YA conciliada, y el segundo intento
		       -- habría anulado la factura verdadera del proveedor.
		       COALESCE(d.clave = 'INVC-' || replace(u.id::text, '-', ''), false)
		FROM inv_unidad u
		JOIN inv_articulo a ON a.id = u.articulo_id
		JOIN inv_categoria c ON c.id = a.categoria_id
		LEFT JOIN inv_categoria cp ON cp.id = c.padre_id
		LEFT JOIN proveedor p ON p.id = u.proveedor_id
		LEFT JOIN inv_movimiento mov ON mov.id = ` + sqlSalidaDeLaUnidad + `
		LEFT JOIN sede s ON s.id = mov.sede_id
		LEFT JOIN inv_servicio sv ON sv.id = mov.servicio_id
		-- El JOIN lleva empresa_id aunque la FK sea de una sola columna: si un enlace quedara
		-- apuntando a un documento de otra empresa, esta consulta NO va a mostrar sus datos.
		LEFT JOIN documento_cxp d ON d.id = u.cxp_consignacion_id AND d.empresa_id = u.empresa_id
		WHERE u.empresa_id = $1::uuid AND u.es_consignada
		  AND ` + sqlSalioDeLaBodega

	args := []any{empresaID}
	if f.ProveedorID != "" {
		args = append(args, f.ProveedorID)
		q += fmt.Sprintf(` AND u.proveedor_id = $%d::uuid`, len(args))
	}
	if f.Estado != "" {
		args = append(args, f.Estado)
		q += fmt.Sprintf(` AND u.estado = $%d`, len(args))
	}
	switch f.Situacion {
	case SituacionPendiente:
		q += ` AND ` + sqlDeudaVigente
	case SituacionFacturada:
		q += ` AND NOT ` + sqlDeudaVigente
	}
	q += ` ORDER BY (NOT ` + sqlDeudaVigente + `), mov.fecha DESC NULLS LAST, u.numero`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("inventario: consignadas pendientes: %w", err)
	}
	defer rows.Close()

	out := []ConsignadaSalida{}
	for rows.Next() {
		var u ConsignadaSalida
		if err := rows.Scan(&u.UnidadID, &u.UnidadNumero, &u.Articulo, &u.Categoria,
			&u.Estado, &u.CostoCRC, &u.ProveedorID, &u.Proveedor, &u.Sede,
			&u.FechaSalida, &u.TipoSalida, &u.MotivoSalida,
			&u.ServicioNumero, &u.ServicioANombreDe,
			&u.DocumentoID, &u.DocumentoConsecutivo, &u.DocumentoEstado, &u.DocumentoTotalCRC,
			&u.DocumentoCompraID, &u.DeudaVigente, &u.DocumentoEsProvision); err != nil {
			return nil, fmt.Errorf("inventario: scan consignada: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ConsignadaPorUnidad trae una sola, con lo que hace falta para facturarla.
//
// Cuando no está en la cola, el error dice POR QUÉ y no «no encontrada»: una unidad puede faltar de
// esta lista por tres razones distintas —no existe, no es consignada, o sigue en la bodega— y las
// tres se arreglan de forma distinta. Responder «unidad no encontrada» a alguien que la está viendo
// en pantalla es hacerle perder el rato buscándola.
func (r *pgRepository) ConsignadaPorUnidad(ctx context.Context, empresaID, unidadID string) (ConsignadaSalida, error) {
	list, err := r.ConsignadasPendientes(ctx, empresaID, FiltroConsignacion{})
	if err != nil {
		return ConsignadaSalida{}, err
	}
	for _, u := range list {
		if u.UnidadID == unidadID {
			return u, nil
		}
	}

	// No está en la cola: averiguar cuál de los tres casos es.
	const q = `
		SELECT u.es_consignada, u.estado
		FROM inv_unidad u
		WHERE u.empresa_id = $1::uuid AND u.id = $2::uuid`
	var esConsignada bool
	var estado string
	if err := r.pool.QueryRow(ctx, q, empresaID, unidadID).Scan(&esConsignada, &estado); err != nil {
		return ConsignadaSalida{}, ErrUnidadNoEncontrada
	}
	switch {
	case !esConsignada:
		return ConsignadaSalida{}, ErrNoEsConsignada
	case estado == EstadoEnTransito:
		return ConsignadaSalida{}, ErrConsignadaEnTransito
	default:
		return ConsignadaSalida{}, ErrConsignadaEnBodega
	}
}

// EnlazarCxPConsignacion pega la factura a la unidad, y es el guardarraíl contra el doble pago.
//
// El UPDATE exige `cxp_consignacion_id IS NULL`: si dos personas facturan la misma unidad a la vez,
// la segunda no escribe nada y se lleva el error. La comprobación va DENTRO del UPDATE y no en un
// SELECT previo a propósito —leer y después escribir deja una ventana entre las dos operaciones— y
// además hay un índice único parcial en la base como última línea de defensa.
func (r *pgRepository) EnlazarCxPConsignacion(ctx context.Context, empresaID, unidadID, documentoID string) error {
	const q = `
		UPDATE inv_unidad SET cxp_consignacion_id = $3::uuid, actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND es_consignada
		  AND cxp_consignacion_id IS NULL`
	tag, err := r.pool.Exec(ctx, q, empresaID, unidadID, documentoID)
	if err != nil {
		return fmt.Errorf("inventario: enlazar cxp de consignación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConsignadaYaFacturada
	}
	return nil
}

// ReemplazarCxPConsignacion cambia la provisión por la factura real EN UN SOLO UPDATE.
//
// Es un compare-and-swap: solo escribe si la unidad todavía apunta a la provisión que se leyó. Antes
// esto eran dos operaciones —desenlazar y después enlazar— y entre las dos había una ventana en la
// que la unidad quedaba sin documento; si la segunda fallaba, quedaba así para siempre, con la
// provisión ya anulada y sin forma de rehacerla. Un solo UPDATE no tiene ventana: o queda enlazada a
// la factura real, o queda como estaba.
func (r *pgRepository) ReemplazarCxPConsignacion(ctx context.Context, empresaID, unidadID, provisionID, documentoRealID string) error {
	const q = `
		UPDATE inv_unidad SET cxp_consignacion_id = $4::uuid, actualizado_en = now()
		WHERE empresa_id = $1::uuid AND id = $2::uuid AND es_consignada
		  AND cxp_consignacion_id = $3::uuid`
	tag, err := r.pool.Exec(ctx, q, empresaID, unidadID, provisionID, documentoRealID)
	if err != nil {
		return fmt.Errorf("inventario: reemplazar cxp de consignación: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Alguien más movió la unidad entre la lectura y la escritura.
		return ErrConsignadaYaFacturada
	}
	return nil
}

// UnidadConEsteDocumento dice qué unidad ya usa esa factura, si alguna. Una factura electrónica del
// proveedor puede cubrir varios cofres, pero el enlace es uno a uno: hay que avisar con el número de
// la otra unidad, porque «ya está enlazada» a secas obliga a buscarla a mano.
func (r *pgRepository) UnidadConEsteDocumento(ctx context.Context, empresaID, documentoID, exceptoUnidadID string) (string, error) {
	const q = `
		SELECT u.numero FROM inv_unidad u
		WHERE u.empresa_id = $1::uuid AND u.cxp_consignacion_id = $2::uuid AND u.id <> $3::uuid
		LIMIT 1`
	var numero string
	err := r.pool.QueryRow(ctx, q, empresaID, documentoID, exceptoUnidadID).Scan(&numero)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inventario: unidad con ese documento: %w", err)
	}
	return numero, nil
}

// FacturasCandidatas son las facturas del proveedor que SÍ se pueden usar para conciliar esta unidad.
//
// Existe porque ofrecer todas y dejar que el servidor rechace es una trampa: la pantalla mostraba las
// provisiones que el propio módulo había generado para otras unidades, y las facturas ya enlazadas.
// Elegir cualquiera de esas terminaba en un rechazo después de haber abierto el diálogo, y en la
// versión anterior del código incluso anulaba la provisión buena antes de fallar.
//
// Los cuatro descartes son exactamente las cuatro guardas del service, escritas una sola vez acá para
// que la lista y la validación no puedan discrepar.
func (r *pgRepository) FacturasCandidatas(ctx context.Context, empresaID, unidadID string, limite int) ([]FacturaCandidata, int, error) {
	const q = `
		WITH uni AS (
		  SELECT u.proveedor_id, u.cxp_consignacion_id
		  FROM inv_unidad u WHERE u.empresa_id = $1::uuid AND u.id = $2::uuid
		),
		posibles AS (
		  SELECT d.id, d.consecutivo, d.clave, d.total_crc, d.estado,
		         to_char(d.fecha_emision, 'YYYY-MM-DD') AS fecha
		  FROM documento_cxp d, uni
		  WHERE d.empresa_id = $1::uuid
		    -- 1. del MISMO proveedor que la mercadería
		    AND d.proveedor_id = uni.proveedor_id
		    -- 2. que no esté anulada: no respaldaría ninguna deuda
		    AND d.estado <> 'ANULADO'
		    -- 3. que no sea la provisión que se está reemplazando
		    AND d.id IS DISTINCT FROM uni.cxp_consignacion_id
		    -- 4. que no la esté usando otra unidad (el enlace es uno a uno)
		    AND NOT EXISTS (SELECT 1 FROM inv_unidad o
		                    WHERE o.empresa_id = $1::uuid AND o.cxp_consignacion_id = d.id)
		    -- Y que no sea una provisión de este mismo módulo: ofrecer una como «factura real» sería
		    -- cambiar una provisión por otra.
		    AND d.clave NOT LIKE 'INVC-%'
		)
		SELECT id::text, COALESCE(consecutivo, ''), clave, total_crc::text, fecha,
		       count(*) OVER () AS total
		FROM posibles
		ORDER BY fecha DESC, consecutivo DESC
		LIMIT $3`

	rows, err := r.pool.Query(ctx, q, empresaID, unidadID, limite)
	if err != nil {
		return nil, 0, fmt.Errorf("inventario: facturas candidatas: %w", err)
	}
	defer rows.Close()

	out := []FacturaCandidata{}
	total := 0
	for rows.Next() {
		var c FacturaCandidata
		if err := rows.Scan(&c.ID, &c.Consecutivo, &c.Clave, &c.TotalCRC, &c.Fecha, &total); err != nil {
			return nil, 0, fmt.Errorf("inventario: scan candidata: %w", err)
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// ResumenConsignacion cuenta la cola en plata y en unidades, para la cabecera de la pantalla.
func (r *pgRepository) ResumenConsignacion(ctx context.Context, empresaID string) (ResumenConsignacion, error) {
	// Las condiciones son LAS MISMAS constantes que usa la lista. Es la única forma de que la
	// cabecera y la tabla no se contradigan, y en este proyecto ya pasó cuatro veces que dos capas
	// calculando el mismo número dejaran de cuadrar.
	//
	// «Por facturar» se parte en dos: lo que salió por un USO —eso se le debe— y lo que salió por
	// otra razón —eso hay que decidirlo—. Meterlos en el mismo total decía que se le debe al
	// proveedor un cofre que se devolvió.
	const q = `
		WITH salidas AS (
		  SELECT u.*, ` + sqlSalioDeLaBodega + ` AS salio, ` + sqlDeudaVigente + ` AS se_debe
		  FROM inv_unidad u
		  WHERE u.empresa_id = $1::uuid AND u.es_consignada
		)
		SELECT
		  count(*) FILTER (WHERE NOT salio)::int,
		  COALESCE(SUM(costo_crc) FILTER (WHERE NOT salio), 0)::text,
		  count(*) FILTER (WHERE salio AND se_debe AND estado = 'USADA')::int,
		  COALESCE(SUM(costo_crc) FILTER (WHERE salio AND se_debe AND estado = 'USADA'), 0)::text,
		  count(*) FILTER (WHERE salio AND se_debe AND estado <> 'USADA')::int,
		  COALESCE(SUM(costo_crc) FILTER (WHERE salio AND se_debe AND estado <> 'USADA'), 0)::text,
		  count(*) FILTER (WHERE salio AND NOT se_debe)::int,
		  COALESCE(SUM(costo_crc) FILTER (WHERE salio AND NOT se_debe), 0)::text,
		  count(DISTINCT proveedor_id)::int
		FROM salidas`
	var r2 ResumenConsignacion
	err := r.pool.QueryRow(ctx, q, empresaID).Scan(
		&r2.EnBodega, &r2.EnBodegaCRC,
		&r2.PorFacturar, &r2.PorFacturarCRC,
		&r2.ADecidir, &r2.ADecidirCRC,
		&r2.Facturadas, &r2.FacturadasCRC,
		&r2.Proveedores)
	if err != nil {
		return ResumenConsignacion{}, fmt.Errorf("inventario: resumen de consignación: %w", err)
	}
	return r2, nil
}
