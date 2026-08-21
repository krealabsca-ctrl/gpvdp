package bancos

// Fusión del catálogo: mover TODO lo que apunta a un concepto o a una clasificación hacia
// otro y borrar el origen.
//
// Por qué existe: eliminar solo funcionaba con entradas sin uso, así que un concepto
// duplicado o mal nombrado con miles de movimientos encima quedaba para siempre. La única
// forma de dejar el catálogo limpio es poder decir «esto en realidad era aquello».
//
// Dos reglas que gobiernan la implementación:
//
//  1. NUNCA se reparenta una clasificación que tenga hijos. `movimiento_bancario` y
//     `regla_clasificacion` apuntan a la llave COMPUESTA (clasificacion_id, concepto_id),
//     así que cambiarle el concepto a la clasificación rompería el FK de sus hijos. En vez
//     de eso, los hijos se mueven a la clasificación gemela del destino (que se crea si no
//     existe) y la del origen se borra.
//  2. Se toca TODO lo que referencia al catálogo, no solo Bancos: el catálogo es compartido
//     con CxP (documentos, gastos de proveedor) y con caja chica (vales). Dejar una sola
//     referencia sin mover hace fallar el DELETE final y aborta la transacción entera —
//     que es lo correcto, pero no sirve de nada al usuario.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ResumenFusion cuenta qué se movió. Se devuelve al usuario y se guarda en la auditoría:
// una fusión es irreversible, así que tiene que quedar dicho qué arrastró.
type ResumenFusion struct {
	Origen             string `json:"origen"`
	Destino            string `json:"destino"`
	Movimientos        int    `json:"movimientos"`
	Reglas             int    `json:"reglas"`
	DocumentosCxP      int    `json:"documentos_cxp"`
	GastosProveedor    int    `json:"gastos_proveedor"`
	ValesCajaChica     int    `json:"vales_caja_chica"`
	Proveedores        int    `json:"proveedores"`
	Clasificaciones    int    `json:"clasificaciones"`
	Subclasificaciones int    `json:"subclasificaciones"`
}

func (r *ResumenFusion) sumar(o ResumenFusion) {
	r.Movimientos += o.Movimientos
	r.Reglas += o.Reglas
	r.DocumentosCxP += o.DocumentosCxP
	r.GastosProveedor += o.GastosProveedor
	r.ValesCajaChica += o.ValesCajaChica
	r.Proveedores += o.Proveedores
	r.Subclasificaciones += o.Subclasificaciones
}

var (
	// ErrFusionMismaEntrada: fusionar algo consigo mismo no es una operación, es un error
	// de la pantalla; se rechaza en vez de borrar el origen y dejar todo huérfano.
	ErrFusionMismaEntrada = errors.New("bancos: el origen y el destino de la fusión son el mismo")
	// ErrFusionOtroConcepto se usa al fusionar clasificaciones de conceptos distintos sin
	// que el usuario lo haya confirmado (el movimiento cambia de concepto, no solo de
	// clasificación: cambia el cuadre).
	ErrFusionOtroConcepto = errors.New("bancos: la clasificación destino pertenece a otro concepto; confirmá el cambio de concepto")
)

// FusionarClasificaciones mueve todo lo de `origenID` a `destinoID` y borra el origen.
// El destino puede vivir en otro concepto: eso es justamente «pasar estos movimientos a
// otro concepto», y por eso `permitirOtroConcepto` es una confirmación explícita.
func (r *pgRepository) FusionarClasificaciones(ctx context.Context, empresaID, origenID, destinoID string, permitirOtroConcepto bool) (ResumenFusion, error) {
	if origenID == destinoID {
		return ResumenFusion{}, ErrFusionMismaEntrada
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ResumenFusion{}, fmt.Errorf("bancos: begin fusión de clasificación: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	oNombre, oConcepto, err := datosClasificacion(ctx, tx, empresaID, origenID)
	if err != nil {
		return ResumenFusion{}, err
	}
	dNombre, dConcepto, err := datosClasificacion(ctx, tx, empresaID, destinoID)
	if err != nil {
		return ResumenFusion{}, err
	}
	if oConcepto != dConcepto && !permitirOtroConcepto {
		return ResumenFusion{}, ErrFusionOtroConcepto
	}

	res, err := moverClasificacion(ctx, tx, empresaID, origenID, destinoID, dConcepto)
	if err != nil {
		return ResumenFusion{}, err
	}
	if err := heredarMarcaContabilidad(ctx, tx, "clasificacion", empresaID, origenID, destinoID); err != nil {
		return ResumenFusion{}, err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM clasificacion WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, origenID); err != nil {
		return ResumenFusion{}, fmt.Errorf("bancos: borrar clasificación origen: %w", err)
	}
	res.Origen, res.Destino = oNombre, dNombre
	res.Clasificaciones = 1
	if err := tx.Commit(ctx); err != nil {
		return ResumenFusion{}, fmt.Errorf("bancos: commit fusión de clasificación: %w", err)
	}
	return res, nil
}

// FusionarConceptos mueve todas las clasificaciones del concepto origen al destino
// (fusionando las que ya existen ahí con el mismo nombre) y borra el origen.
func (r *pgRepository) FusionarConceptos(ctx context.Context, empresaID, origenID, destinoID string) (ResumenFusion, error) {
	if origenID == destinoID {
		return ResumenFusion{}, ErrFusionMismaEntrada
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ResumenFusion{}, fmt.Errorf("bancos: begin fusión de concepto: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	oNombre, err := nombreConcepto(ctx, tx, empresaID, origenID)
	if err != nil {
		return ResumenFusion{}, err
	}
	dNombre, err := nombreConcepto(ctx, tx, empresaID, destinoID)
	if err != nil {
		return ResumenFusion{}, err
	}

	// Las clasificaciones del origen, una por una.
	rows, err := tx.Query(ctx,
		`SELECT id::text, nombre, COALESCE(cuenta_contable_futura,'') FROM clasificacion
		 WHERE empresa_id = $1::uuid AND concepto_id = $2::uuid ORDER BY nombre`,
		empresaID, origenID)
	if err != nil {
		return ResumenFusion{}, fmt.Errorf("bancos: clasificaciones del concepto origen: %w", err)
	}
	type clasif struct{ id, nombre, cuenta string }
	var clasifs []clasif
	for rows.Next() {
		var c clasif
		if err := rows.Scan(&c.id, &c.nombre, &c.cuenta); err != nil {
			rows.Close()
			return ResumenFusion{}, fmt.Errorf("bancos: scan clasificación: %w", err)
		}
		clasifs = append(clasifs, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ResumenFusion{}, err
	}

	res := ResumenFusion{Origen: oNombre, Destino: dNombre}
	for _, c := range clasifs {
		// ¿El destino ya tiene una clasificación con ese nombre? Se fusionan; si no, se crea
		// la gemela y se le mueve todo (no se reparenta: rompería el FK compuesto).
		var destinoClasif string
		err := tx.QueryRow(ctx,
			`SELECT id::text FROM clasificacion
			 WHERE empresa_id = $1::uuid AND concepto_id = $2::uuid AND lower(nombre) = lower($3)`,
			empresaID, destinoID, c.nombre).Scan(&destinoClasif)
		if errors.Is(err, pgx.ErrNoRows) {
			if err := tx.QueryRow(ctx,
				`INSERT INTO clasificacion (empresa_id, concepto_id, nombre, cuenta_contable_futura)
				 VALUES ($1::uuid, $2::uuid, $3, NULLIF($4,''))
				 RETURNING id::text`,
				empresaID, destinoID, c.nombre, c.cuenta).Scan(&destinoClasif); err != nil {
				return ResumenFusion{}, fmt.Errorf("bancos: crear clasificación gemela: %w", err)
			}
		} else if err != nil {
			return ResumenFusion{}, fmt.Errorf("bancos: buscar clasificación gemela: %w", err)
		}
		parcial, err := moverClasificacion(ctx, tx, empresaID, c.id, destinoClasif, destinoID)
		if err != nil {
			return ResumenFusion{}, err
		}
		if err := heredarMarcaContabilidad(ctx, tx, "clasificacion", empresaID, c.id, destinoClasif); err != nil {
			return ResumenFusion{}, err
		}
		res.sumar(parcial)
		res.Clasificaciones++
		if _, err := tx.Exec(ctx,
			`DELETE FROM clasificacion WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, c.id); err != nil {
			return ResumenFusion{}, fmt.Errorf("bancos: borrar clasificación del origen: %w", err)
		}
	}

	// Lo que apunta al concepto SIN pasar por una clasificación (un movimiento puede tener
	// concepto y no clasificación: el FK compuesto no se exige si clasificacion_id es null).
	sueltos, err := moverConceptoSuelto(ctx, tx, empresaID, origenID, destinoID)
	if err != nil {
		return ResumenFusion{}, err
	}
	res.sumar(sueltos)

	if err := heredarMarcaContabilidad(ctx, tx, "concepto", empresaID, origenID, destinoID); err != nil {
		return ResumenFusion{}, err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM concepto WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, origenID); err != nil {
		return ResumenFusion{}, fmt.Errorf("bancos: borrar concepto origen: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ResumenFusion{}, fmt.Errorf("bancos: commit fusión de concepto: %w", err)
	}
	return res, nil
}

// ---- Piezas compartidas ----

// heredarMarcaContabilidad arrastra al destino la marca «de Contabilidad» (CxP) del origen.
//
// Fusionar mueve movimientos, reglas, CxP, vales y proveedores, pero la marca vive en una columna
// del catálogo y se perdía en silencio al borrar el origen: un rubro marcado como «de Contabilidad»
// dejaba de estarlo por fusionarlo, y sus facturas volvían a esperar un validador de área que no
// existe. Nunca DESMARCA el destino: si el destino ya estaba marcado, sigue marcado (es un OR).
//
// `tabla` es un literal del propio código (concepto | clasificacion), no entrada del usuario.
func heredarMarcaContabilidad(ctx context.Context, tx pgx.Tx, tabla, empresaID, origenID, destinoID string) error {
	q := `UPDATE ` + tabla + ` SET es_contabilidad = true
	      WHERE empresa_id = $1::uuid AND id = $2::uuid
	        AND EXISTS (SELECT 1 FROM ` + tabla + ` o
	                    WHERE o.empresa_id = $1::uuid AND o.id = $3::uuid AND o.es_contabilidad)`
	if _, err := tx.Exec(ctx, q, empresaID, destinoID, origenID); err != nil {
		return fmt.Errorf("bancos: heredar marca de contabilidad en %s: %w", tabla, err)
	}
	return nil
}

func datosClasificacion(ctx context.Context, tx pgx.Tx, empresaID, id string) (nombre, conceptoID string, err error) {
	err = tx.QueryRow(ctx,
		`SELECT nombre, concepto_id::text FROM clasificacion WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaID, id).Scan(&nombre, &conceptoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrClasificacionNoEncontrada
	}
	if err != nil {
		return "", "", fmt.Errorf("bancos: leer clasificación: %w", err)
	}
	return nombre, conceptoID, nil
}

func nombreConcepto(ctx context.Context, tx pgx.Tx, empresaID, id string) (string, error) {
	var nombre string
	err := tx.QueryRow(ctx,
		`SELECT nombre FROM concepto WHERE empresa_id = $1::uuid AND id = $2::uuid`, empresaID, id).Scan(&nombre)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrConceptoNoEncontrado
	}
	if err != nil {
		return "", fmt.Errorf("bancos: leer concepto: %w", err)
	}
	return nombre, nil
}

// moverClasificacion reapunta todos los hijos de `origen` a (`destino`, `destinoConcepto`).
// No borra el origen: eso lo decide quien llama.
func moverClasificacion(ctx context.Context, tx pgx.Tx, empresaID, origen, destino, destinoConcepto string) (ResumenFusion, error) {
	var res ResumenFusion

	// Las subclasificaciones van primero: los vales y documentos que apuntan a ellas no
	// deben quedar apuntando a una subclasificación de una clasificación ya borrada.
	if err := moverSubclasificaciones(ctx, tx, empresaID, origen, destino, &res); err != nil {
		return ResumenFusion{}, err
	}

	// Pares (tabla, columnas) donde el movimiento es un UPDATE directo.
	pasos := []struct {
		nombre  string
		sql     string
		destino *int
	}{
		{"movimiento_bancario", `UPDATE movimiento_bancario SET clasificacion_id = $2::uuid, concepto_id = $3::uuid
			WHERE empresa_id = $1::uuid AND clasificacion_id = $4::uuid`, &res.Movimientos},
		{"regla_clasificacion", `UPDATE regla_clasificacion SET clasificacion_id = $2::uuid, concepto_id = $3::uuid
			WHERE empresa_id = $1::uuid AND clasificacion_id = $4::uuid`, &res.Reglas},
		{"documento_cxp", `UPDATE documento_cxp SET clasificacion_id = $2::uuid, concepto_id = $3::uuid
			WHERE empresa_id = $1::uuid AND clasificacion_id = $4::uuid`, &res.DocumentosCxP},
		{"caja_chica_vale", `UPDATE caja_chica_vale SET clasificacion_id = $2::uuid, concepto_id = $3::uuid
			WHERE empresa_id = $1::uuid AND clasificacion_id = $4::uuid`, &res.ValesCajaChica},
		{"proveedor", `UPDATE proveedor SET gasto_clasificacion_id = $2::uuid, gasto_concepto_id = $3::uuid
			WHERE empresa_id = $1::uuid AND gasto_clasificacion_id = $4::uuid`, &res.Proveedores},
	}
	for _, p := range pasos {
		tag, err := tx.Exec(ctx, p.sql, empresaID, destino, destinoConcepto, origen)
		if err != nil {
			return ResumenFusion{}, fmt.Errorf("bancos: mover %s: %w", p.nombre, err)
		}
		*p.destino += int(tag.RowsAffected())
	}

	// proveedor_gasto tiene UNIQUE (proveedor_id, concepto_id, clasificacion_id,
	// subclasificacion_id): si el proveedor ya tenía el gasto del destino, mover el del
	// origen duplicaría la fila. Se borra el del origen en vez de reventar.
	if _, err := tx.Exec(ctx, `
		DELETE FROM proveedor_gasto o
		WHERE o.empresa_id = $1::uuid AND o.clasificacion_id = $4::uuid
		  AND EXISTS (
			SELECT 1 FROM proveedor_gasto d
			WHERE d.proveedor_id = o.proveedor_id
			  AND d.clasificacion_id = $2::uuid AND d.concepto_id = $3::uuid
			  AND d.subclasificacion_id IS NOT DISTINCT FROM o.subclasificacion_id
		  )`, empresaID, destino, destinoConcepto, origen); err != nil {
		return ResumenFusion{}, fmt.Errorf("bancos: quitar gastos de proveedor duplicados: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE proveedor_gasto SET clasificacion_id = $2::uuid, concepto_id = $3::uuid
		WHERE empresa_id = $1::uuid AND clasificacion_id = $4::uuid`,
		empresaID, destino, destinoConcepto, origen)
	if err != nil {
		return ResumenFusion{}, fmt.Errorf("bancos: mover gastos de proveedor: %w", err)
	}
	res.GastosProveedor += int(tag.RowsAffected())
	return res, nil
}

// moverSubclasificaciones lleva las subclasificaciones del origen a la clasificación
// destino. Si el destino ya tiene una con el mismo nombre (UNIQUE), se fusionan.
func moverSubclasificaciones(ctx context.Context, tx pgx.Tx, empresaID, origen, destino string, res *ResumenFusion) error {
	rows, err := tx.Query(ctx,
		`SELECT id::text, nombre FROM subclasificacion WHERE clasificacion_id = $1::uuid ORDER BY nombre`, origen)
	if err != nil {
		return fmt.Errorf("bancos: subclasificaciones del origen: %w", err)
	}
	type sub struct{ id, nombre string }
	var subs []sub
	for rows.Next() {
		var s sub
		if err := rows.Scan(&s.id, &s.nombre); err != nil {
			rows.Close()
			return fmt.Errorf("bancos: scan subclasificación: %w", err)
		}
		subs = append(subs, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, s := range subs {
		var gemela string
		err := tx.QueryRow(ctx,
			`SELECT id::text FROM subclasificacion WHERE clasificacion_id = $1::uuid AND lower(nombre) = lower($2)`,
			destino, s.nombre).Scan(&gemela)
		if errors.Is(err, pgx.ErrNoRows) {
			// No hay choque: se reparenta. `subclasificacion` no participa de ningún FK
			// compuesto, así que sus hijos siguen apuntando al mismo id.
			if _, err := tx.Exec(ctx,
				`UPDATE subclasificacion SET clasificacion_id = $1::uuid WHERE id = $2::uuid`, destino, s.id); err != nil {
				return fmt.Errorf("bancos: reparentar subclasificación: %w", err)
			}
			res.Subclasificaciones++
			continue
		}
		if err != nil {
			return fmt.Errorf("bancos: buscar subclasificación gemela: %w", err)
		}
		// Choque de nombre: los hijos pasan a la gemela y la del origen se borra.
		for _, q := range []string{
			`UPDATE documento_cxp SET subclasificacion_id = $1::uuid WHERE empresa_id = $3::uuid AND subclasificacion_id = $2::uuid`,
			`UPDATE caja_chica_vale SET subclasificacion_id = $1::uuid WHERE empresa_id = $3::uuid AND subclasificacion_id = $2::uuid`,
			`UPDATE proveedor SET gasto_subclasificacion_id = $1::uuid WHERE empresa_id = $3::uuid AND gasto_subclasificacion_id = $2::uuid`,
		} {
			if _, err := tx.Exec(ctx, q, gemela, s.id, empresaID); err != nil {
				return fmt.Errorf("bancos: mover hijos de subclasificación: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM proveedor_gasto o
			WHERE o.empresa_id = $3::uuid AND o.subclasificacion_id = $2::uuid
			  AND EXISTS (
				SELECT 1 FROM proveedor_gasto d
				WHERE d.proveedor_id = o.proveedor_id AND d.concepto_id = o.concepto_id
				  AND d.clasificacion_id = o.clasificacion_id AND d.subclasificacion_id = $1::uuid
			  )`, gemela, s.id, empresaID); err != nil {
			return fmt.Errorf("bancos: quitar gastos duplicados por subclasificación: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE proveedor_gasto SET subclasificacion_id = $1::uuid WHERE empresa_id = $3::uuid AND subclasificacion_id = $2::uuid`,
			gemela, s.id, empresaID); err != nil {
			return fmt.Errorf("bancos: mover gastos por subclasificación: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM subclasificacion WHERE id = $1::uuid`, s.id); err != nil {
			return fmt.Errorf("bancos: borrar subclasificación duplicada: %w", err)
		}
		res.Subclasificaciones++
	}
	return nil
}

// moverConceptoSuelto reapunta lo que cuelga del concepto sin clasificación de por medio.
func moverConceptoSuelto(ctx context.Context, tx pgx.Tx, empresaID, origen, destino string) (ResumenFusion, error) {
	var res ResumenFusion
	pasos := []struct {
		nombre  string
		sql     string
		destino *int
	}{
		{"movimiento_bancario", `UPDATE movimiento_bancario SET concepto_id = $2::uuid
			WHERE empresa_id = $1::uuid AND concepto_id = $3::uuid AND clasificacion_id IS NULL`, &res.Movimientos},
		{"documento_cxp", `UPDATE documento_cxp SET concepto_id = $2::uuid
			WHERE empresa_id = $1::uuid AND concepto_id = $3::uuid AND clasificacion_id IS NULL`, &res.DocumentosCxP},
		{"caja_chica_vale", `UPDATE caja_chica_vale SET concepto_id = $2::uuid
			WHERE empresa_id = $1::uuid AND concepto_id = $3::uuid AND clasificacion_id IS NULL`, &res.ValesCajaChica},
		{"proveedor", `UPDATE proveedor SET gasto_concepto_id = $2::uuid
			WHERE empresa_id = $1::uuid AND gasto_concepto_id = $3::uuid AND gasto_clasificacion_id IS NULL`, &res.Proveedores},
		{"proveedor_gasto", `UPDATE proveedor_gasto SET concepto_id = $2::uuid
			WHERE empresa_id = $1::uuid AND concepto_id = $3::uuid AND clasificacion_id IS NULL`, &res.GastosProveedor},
	}
	for _, p := range pasos {
		tag, err := tx.Exec(ctx, p.sql, empresaID, destino, origen)
		if err != nil {
			return ResumenFusion{}, fmt.Errorf("bancos: mover %s suelto: %w", p.nombre, err)
		}
		*p.destino += int(tag.RowsAffected())
	}
	return res, nil
}
