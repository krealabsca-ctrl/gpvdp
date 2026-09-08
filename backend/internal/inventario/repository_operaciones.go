package inventario

// Las operaciones que MUEVEN el inventario.
//
// Todas van en una transacción, y no por prolijidad: una entrada escribe el movimiento Y crea las
// fichas de las unidades; un servicio escribe la salida Y marca la unidad como usada; un traslado
// escribe la salida Y deja la unidad en tránsito. Si cualquiera de esos pares se partiera a la
// mitad, quedarían dos verdades sobre el mismo cofre —y ninguna forma de saber cuál es la buena—.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// siguienteConsecutivo reserva el próximo número de un ámbito ("SERVICIO", "TRASLADO", o el
// artículo cuyas unidades se numeran). El UPDATE ... RETURNING lo hace atómico: dos entradas
// simultáneas no pueden llevarse el mismo número.
func siguienteConsecutivo(ctx context.Context, tx pgx.Tx, empresaID, ambito string) (int, error) {
	// La fila guarda el PRÓXIMO número libre, así que se devuelve el anterior al resultante: tanto
	// en el primer uso (inserta 2 → devuelve 1) como en los siguientes (incrementa → devuelve el
	// valor que quedó tomado). Un solo statement, así que dos entradas simultáneas no se pueden
	// llevar el mismo número.
	const q = `
		INSERT INTO inv_consecutivo (empresa_id, ambito, siguiente) VALUES ($1::uuid, $2, 2)
		ON CONFLICT (empresa_id, ambito) DO UPDATE SET siguiente = inv_consecutivo.siguiente + 1
		RETURNING siguiente`
	var n int
	if err := tx.QueryRow(ctx, q, empresaID, ambito).Scan(&n); err != nil {
		return 0, fmt.Errorf("inventario: consecutivo de %s: %w", ambito, err)
	}
	return n - 1, nil
}

// datosArticulo trae lo mínimo que necesitan las operaciones: su modo y su nombre para los mensajes.
func datosArticulo(ctx context.Context, tx pgx.Tx, empresaID, articuloID string) (modo, nombre, codigo string, err error) {
	err = tx.QueryRow(ctx,
		`SELECT modo_control, nombre, codigo FROM inv_articulo WHERE id = $1::uuid AND empresa_id = $2::uuid`,
		articuloID, empresaID).Scan(&modo, &nombre, &codigo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrArticuloNoEncontrado
	}
	if err != nil {
		return "", "", "", fmt.Errorf("inventario: datos del artículo: %w", err)
	}
	return modo, nombre, codigo, nil
}

func nombreSede(ctx context.Context, tx pgx.Tx, empresaID, sedeID string) (string, error) {
	var n string
	err := tx.QueryRow(ctx, `SELECT nombre FROM sede WHERE id = $1::uuid AND empresa_id = $2::uuid`,
		sedeID, empresaID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: la sede no existe o no es de esta empresa", ErrSedeRequerida)
	}
	if err != nil {
		return "", fmt.Errorf("inventario: nombre de sede: %w", err)
	}
	return n, nil
}

// costoPromedio es el costo unitario de un artículo controlado por cantidad: el promedio ponderado
// de sus entradas. La fórmula es la misma constante que usan las consultas de existencia
// (sqlPromedioPonderado), para que el costo con el que sale del inventario sea exactamente el mismo
// con el que se valoriza lo que queda.
func costoPromedio(ctx context.Context, tx pgx.Tx, empresaID, articuloID string) (string, error) {
	// Redondeado a dos decimales porque es el valor que se va a GUARDAR (la columna es numeric(14,2)):
	// si Go se quedara con el promedio largo, el costo que muestra la respuesta y el que queda en el
	// libro diferirían en céntimos, y el total del servicio dejaría de cuadrar al releerlo.
	const q = `
		SELECT COALESCE((SELECT ROUND(` + sqlPromedioPonderado + `, 2)::text
		                 FROM inv_movimiento e
		                 WHERE e.empresa_id = $1::uuid AND e.articulo_id = $2::uuid AND e.tipo = 'ENTRADA'), '0')`
	var c string
	if err := tx.QueryRow(ctx, q, empresaID, articuloID).Scan(&c); err != nil {
		return "", fmt.Errorf("inventario: costo promedio: %w", err)
	}
	return c, nil
}

// insertarMovimiento escribe una línea del libro. Es el único camino para escribir en
// inv_movimiento: si cada operación armara su propio INSERT, tarde o temprano una olvidaría un
// campo y el libro dejaría de poder explicar la existencia.
func insertarMovimiento(ctx context.Context, tx pgx.Tx, m movimientoNuevo) (string, error) {
	const q = `
		INSERT INTO inv_movimiento (empresa_id, articulo_id, unidad_id, tipo, cantidad, sede_id,
		                            sede_contra_id, costo_unitario_crc, fecha, servicio_id,
		                            proveedor_id, documento_cxp_id, traslado_id, motivo, creado_por)
		-- El costo va con COALESCE porque hay movimientos que legítimamente no lo llevan: un ajuste
		-- no tiene precio propio. Sin esto, una cadena vacía casteada a numeric revienta la
		-- operación entera con «invalid input syntax for type numeric».
		VALUES ($1::uuid, $2::uuid, NULLIF($3,'')::uuid, $4, $5, NULLIF($6,'')::uuid,
		        NULLIF($7,'')::uuid, COALESCE(NULLIF($8,'')::numeric, 0), $9::date, NULLIF($10,'')::uuid,
		        NULLIF($11,'')::uuid, NULLIF($12,'')::uuid, NULLIF($13,'')::uuid,
		        NULLIF($14,''), NULLIF($15,'')::uuid)
		RETURNING id::text`
	var id string
	err := tx.QueryRow(ctx, q, m.EmpresaID, m.ArticuloID, m.UnidadID, m.Tipo, m.Cantidad,
		m.SedeID, m.SedeContraID, m.Costo, m.Fecha, m.ServicioID, m.ProveedorID,
		m.DocumentoCxpID, m.TrasladoID, m.Motivo, m.UsuarioID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inventario: insertar movimiento %s: %w", m.Tipo, err)
	}
	return id, nil
}

type movimientoNuevo struct {
	EmpresaID, ArticuloID, UnidadID, Tipo string
	Cantidad                              int
	SedeID, SedeContraID, Costo, Fecha    string
	ServicioID, ProveedorID               string
	DocumentoCxpID, TrasladoID, Motivo    string
	UsuarioID                             string
}

// ── Entradas ────────────────────────────────────────────────────────────────

func (r *pgRepository) RegistrarEntrada(ctx context.Context, empresaID string, e EntradaNueva, usuarioID string) (EntradaHecha, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return EntradaHecha{}, fmt.Errorf("inventario: begin entrada: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	modo, _, codigo, err := datosArticulo(ctx, tx, empresaID, e.ArticuloID)
	if err != nil {
		return EntradaHecha{}, err
	}
	if _, err := nombreSede(ctx, tx, empresaID, e.SedeID); err != nil {
		return EntradaHecha{}, err
	}

	movID, err := insertarMovimiento(ctx, tx, movimientoNuevo{
		EmpresaID: empresaID, ArticuloID: e.ArticuloID, Tipo: MovEntrada, Cantidad: e.Cantidad,
		SedeID: e.SedeID, Costo: e.CostoUnitario, Fecha: e.Fecha,
		ProveedorID: e.ProveedorID, DocumentoCxpID: e.DocumentoCxpID,
		Motivo: e.Nota, UsuarioID: usuarioID,
	})
	if err != nil {
		return EntradaHecha{}, err
	}

	hecha := EntradaHecha{MovimientoID: movID, Cantidad: e.Cantidad, NumerosCreados: []string{}}

	if modo == ModoUnidad {
		// Los números se aceptan de las dos formas: si el proveedor ya los trae (placa, etiqueta) se
		// respetan; si no, se generan. Imponer una sola forma obligaría a inventar números donde ya
		// hay, o a no tener ninguno donde no.
		for i := 0; i < e.Cantidad; i++ {
			numero := ""
			if i < len(e.Numeros) {
				numero = e.Numeros[i]
			}
			if numero == "" {
				n, err := siguienteConsecutivo(ctx, tx, empresaID, "UNIDAD:"+codigo)
				if err != nil {
					return EntradaHecha{}, err
				}
				numero = fmt.Sprintf("%s-%04d", codigo, n)
			}
			var unidadID string
			err := tx.QueryRow(ctx, `
				INSERT INTO inv_unidad (empresa_id, articulo_id, numero, sede_id, estado, costo_crc,
				                        es_consignada, proveedor_id, documento_cxp_id, ingresada_en)
				VALUES ($1::uuid, $2::uuid, $3, $4::uuid, 'DISPONIBLE', $5::numeric, $6,
				        NULLIF($7,'')::uuid, NULLIF($8,'')::uuid, $9::date)
				RETURNING id::text`,
				empresaID, e.ArticuloID, numero, e.SedeID, e.CostoUnitario, e.EsConsignada,
				e.ProveedorID, e.DocumentoCxpID, e.Fecha).Scan(&unidadID)
			if esViolacionUnica(err) {
				return EntradaHecha{}, fmt.Errorf("%w: la unidad %s ya existe", ErrDuplicado, numero)
			}
			if err != nil {
				return EntradaHecha{}, fmt.Errorf("inventario: crear unidad %s: %w", numero, err)
			}
			hecha.NumerosCreados = append(hecha.NumerosCreados, numero)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return EntradaHecha{}, fmt.Errorf("inventario: commit entrada: %w", err)
	}
	return hecha, nil
}

// ── El servicio prestado ────────────────────────────────────────────────────

func (r *pgRepository) RegistrarServicio(ctx context.Context, empresaID string, sv ServicioNuevo, usuarioID string) (Servicio, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Servicio{}, fmt.Errorf("inventario: begin servicio: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sedeNombre, err := nombreSede(ctx, tx, empresaID, sv.SedeID)
	if err != nil {
		return Servicio{}, err
	}
	n, err := siguienteConsecutivo(ctx, tx, empresaID, "SERVICIO")
	if err != nil {
		return Servicio{}, err
	}
	numero := fmt.Sprintf("SV-%s-%04d", sv.Fecha[:4], n)

	var servicioID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO inv_servicio (empresa_id, numero, sede_id, fecha, a_nombre_de, nota, creado_por)
		VALUES ($1::uuid, $2, $3::uuid, $4::date, $5, NULLIF($6,''), NULLIF($7,'')::uuid)
		RETURNING id::text`,
		empresaID, numero, sv.SedeID, sv.Fecha, sv.ANombreDe, sv.Nota, usuarioID).Scan(&servicioID); err != nil {
		return Servicio{}, fmt.Errorf("inventario: crear servicio: %w", err)
	}

	out := Servicio{
		ID: servicioID, Numero: numero, SedeID: sv.SedeID, Sede: sedeNombre,
		Fecha: sv.Fecha, ANombreDe: sv.ANombreDe, Nota: sv.Nota,
		Consumos: []ConsumoLinea{},
	}

	for _, c := range sv.Consumos {
		modo, nombre, _, err := datosArticulo(ctx, tx, empresaID, c.ArticuloID)
		if err != nil {
			return Servicio{}, err
		}
		linea := ConsumoLinea{ArticuloID: c.ArticuloID, Articulo: nombre}

		if modo == ModoUnidad {
			if c.UnidadNumero == "" {
				return Servicio{}, fmt.Errorf("%w: «%s» se controla por unidad, hace falta decir cuál", ErrCantidadInvalida, nombre)
			}
			// La unidad tiene que estar en ESTA sede y en un estado que permita usarla. Sin esta
			// comprobación se podría consumir un cofre que está en otra plaza o que ya se usó.
			var unidadID, estado, costo string
			var sedeUnidad, sedeUnidadNombre *string
			var esConsignada bool
			var proveedorUnidad string
			err := tx.QueryRow(ctx, `
				SELECT u.id::text, u.estado, u.costo_crc::text, u.sede_id::text, s.nombre,
				       u.es_consignada, COALESCE(u.proveedor_id::text, '')
				FROM inv_unidad u LEFT JOIN sede s ON s.id = u.sede_id
				WHERE u.empresa_id = $1::uuid AND u.numero = $2 FOR UPDATE OF u`,
				empresaID, c.UnidadNumero).Scan(&unidadID, &estado, &costo, &sedeUnidad, &sedeUnidadNombre,
				&esConsignada, &proveedorUnidad)
			if errors.Is(err, pgx.ErrNoRows) {
				return Servicio{}, fmt.Errorf("%w: %s", ErrUnidadNoEncontrada, c.UnidadNumero)
			}
			if err != nil {
				return Servicio{}, fmt.Errorf("inventario: buscar unidad: %w", err)
			}
			if estado != EstadoDisponible && estado != EstadoReservada && estado != EstadoExhibicion {
				return Servicio{}, &UnidadNoDisponibleError{Numero: c.UnidadNumero, Estado: estado, Quiere: "usarla en un servicio"}
			}
			if sedeUnidad == nil || *sedeUnidad != sv.SedeID {
				return Servicio{}, &UnidadEnOtraSedeError{
					Numero: c.UnidadNumero, Esta: deref(sedeUnidadNombre), Pedida: sedeNombre,
				}
			}

			if _, err := tx.Exec(ctx,
				`UPDATE inv_unidad SET estado = 'USADA', actualizado_en = now() WHERE id = $1::uuid`,
				unidadID); err != nil {
				return Servicio{}, fmt.Errorf("inventario: marcar unidad usada: %w", err)
			}
			// El proveedor viaja en la SALIDA solo si la unidad es consignada, y ahí es un dato
			// operativo, no informativo: es el movimiento que dice a quién hay que pagarle. En una
			// unidad propia no se pone, porque a nadie se le debe nada por usarla.
			proveedorSalida := ""
			if esConsignada {
				proveedorSalida = proveedorUnidad
			}
			if _, err := insertarMovimiento(ctx, tx, movimientoNuevo{
				EmpresaID: empresaID, ArticuloID: c.ArticuloID, UnidadID: unidadID,
				Tipo: MovSalida, Cantidad: 1, SedeID: sv.SedeID, Costo: costo,
				Fecha: sv.Fecha, ServicioID: servicioID, ProveedorID: proveedorSalida,
				UsuarioID: usuarioID,
			}); err != nil {
				return Servicio{}, err
			}
			linea.UnidadNum = c.UnidadNumero
			linea.Cantidad = 1
			linea.CostoCRC = costo
			linea.EsConsignada = esConsignada
		} else {
			hay, err := existenciaDe(ctx, tx, empresaID, c.ArticuloID, sv.SedeID)
			if err != nil {
				return Servicio{}, err
			}
			if hay < c.Cantidad {
				return Servicio{}, &SinExistenciaError{Articulo: nombre, Sede: sedeNombre, Hay: hay, Pedido: c.Cantidad}
			}
			costo, err := costoPromedio(ctx, tx, empresaID, c.ArticuloID)
			if err != nil {
				return Servicio{}, err
			}
			if _, err := insertarMovimiento(ctx, tx, movimientoNuevo{
				EmpresaID: empresaID, ArticuloID: c.ArticuloID, Tipo: MovSalida,
				Cantidad: c.Cantidad, SedeID: sv.SedeID, Costo: costo,
				Fecha: sv.Fecha, ServicioID: servicioID, UsuarioID: usuarioID,
			}); err != nil {
				return Servicio{}, err
			}
			linea.Cantidad = c.Cantidad
			linea.CostoCRC = multiplicarDecimal(costo, c.Cantidad)
		}
		out.Consumos = append(out.Consumos, linea)
	}

	// El costo total se RELEE del libro con la misma expresión que usa la pantalla de servicios, en
	// vez de acumularlo en Go. Sumarlo por acá dejaba dos fórmulas para el mismo número —la del POST
	// y la del listado— y bastaba un redondeo distinto para que el servicio valiera dos cosas según
	// dónde se lo mirara.
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE((SELECT (`+sqlSumaSalidasDelServicio+`)::text FROM inv_movimiento m
		                 WHERE m.servicio_id = $1::uuid AND m.tipo = 'SALIDA'), '0')`,
		servicioID).Scan(&out.CostoProductoCRC); err != nil {
		return Servicio{}, fmt.Errorf("inventario: costo del servicio: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Servicio{}, fmt.Errorf("inventario: commit servicio: %w", err)
	}
	return out, nil
}

func (r *pgRepository) ListarServicios(ctx context.Context, empresaID, desde, hasta string) ([]Servicio, error) {
	q := `
		SELECT sv.id::text, sv.numero, COALESCE(sv.sede_id::text, ''), COALESCE(s.nombre, ''),
		       to_char(sv.fecha, 'YYYY-MM-DD'), sv.a_nombre_de, COALESCE(sv.nota, ''),
		       COALESCE((SELECT (` + sqlSumaSalidasDelServicio + `)::text FROM inv_movimiento m
		                 WHERE m.servicio_id = sv.id AND m.tipo = 'SALIDA'), '0')
		FROM inv_servicio sv
		LEFT JOIN sede s ON s.id = sv.sede_id
		WHERE sv.empresa_id = $1::uuid`
	args := []any{empresaID}
	if desde != "" {
		args = append(args, desde)
		q += fmt.Sprintf(` AND sv.fecha >= $%d::date`, len(args))
	}
	if hasta != "" {
		args = append(args, hasta)
		q += fmt.Sprintf(` AND sv.fecha <= $%d::date`, len(args))
	}
	q += ` ORDER BY sv.fecha DESC, sv.numero DESC LIMIT 300`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("inventario: listar servicios: %w", err)
	}
	defer rows.Close()
	out := []Servicio{}
	for rows.Next() {
		var s Servicio
		if err := rows.Scan(&s.ID, &s.Numero, &s.SedeID, &s.Sede, &s.Fecha,
			&s.ANombreDe, &s.Nota, &s.CostoProductoCRC); err != nil {
			return nil, fmt.Errorf("inventario: scan servicio: %w", err)
		}
		s.Consumos = []ConsumoLinea{}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *pgRepository) ServicioPorID(ctx context.Context, empresaID, id string) (Servicio, error) {
	ss, err := r.ListarServicios(ctx, empresaID, "", "")
	if err != nil {
		return Servicio{}, err
	}
	for _, s := range ss {
		if s.ID != id {
			continue
		}
		rows, err := r.pool.Query(ctx, `
			SELECT a.id::text, a.nombre, COALESCE(u.numero, ''), m.cantidad,
			       (m.cantidad * m.costo_unitario_crc)::text
			FROM inv_movimiento m
			JOIN inv_articulo a ON a.id = m.articulo_id
			LEFT JOIN inv_unidad u ON u.id = m.unidad_id
			WHERE m.empresa_id = $1::uuid AND m.servicio_id = $2::uuid AND m.tipo = 'SALIDA'
			ORDER BY a.nombre`, empresaID, id)
		if err != nil {
			return Servicio{}, fmt.Errorf("inventario: consumos del servicio: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var c ConsumoLinea
			if err := rows.Scan(&c.ArticuloID, &c.Articulo, &c.UnidadNum, &c.Cantidad, &c.CostoCRC); err != nil {
				return Servicio{}, fmt.Errorf("inventario: scan consumo: %w", err)
			}
			s.Consumos = append(s.Consumos, c)
		}
		return s, rows.Err()
	}
	return Servicio{}, ErrServicioNoEncontrado
}

// ── Ajustes y bajas ─────────────────────────────────────────────────────────

func (r *pgRepository) RegistrarAjuste(ctx context.Context, empresaID string, a AjusteNuevo, usuarioID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inventario: begin ajuste: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if a.UnidadNumero != "" {
		// Ajuste de una unidad: cambia su estado. Es una baja, una devolución o volver a poner
		// disponible algo que se había marcado por error.
		var unidadID, estado, costo, articuloID string
		var sedeUnidad *string
		err := tx.QueryRow(ctx, `
			SELECT id::text, estado, costo_crc::text, articulo_id::text, sede_id::text
			FROM inv_unidad WHERE empresa_id = $1::uuid AND numero = $2 FOR UPDATE`,
			empresaID, a.UnidadNumero).Scan(&unidadID, &estado, &costo, &articuloID, &sedeUnidad)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrUnidadNoEncontrada, a.UnidadNumero)
		}
		if err != nil {
			return fmt.Errorf("inventario: buscar unidad: %w", err)
		}
		if estado == EstadoEnTransito {
			return &UnidadNoDisponibleError{Numero: a.UnidadNumero, Estado: estado, Quiere: "ajustarla; primero hay que recibir el traslado"}
		}
		if _, err := tx.Exec(ctx,
			`UPDATE inv_unidad SET estado = $2, actualizado_en = now() WHERE id = $1::uuid`,
			unidadID, a.NuevoEstado); err != nil {
			return fmt.Errorf("inventario: cambiar estado de unidad: %w", err)
		}
		// El tipo del movimiento sale del estado nuevo: una baja y una devolución no son lo mismo,
		// y la que vuelve a estar disponible es un ajuste que suma.
		tipo := MovAjusteMenos
		switch a.NuevoEstado {
		case EstadoDanada:
			tipo = MovBaja
		case EstadoDevuelta:
			tipo = MovDevolucion
		case EstadoDisponible, EstadoExhibicion, EstadoReservada:
			tipo = MovAjusteMas
		}
		sede := ""
		if sedeUnidad != nil {
			sede = *sedeUnidad
		}
		if _, err := insertarMovimiento(ctx, tx, movimientoNuevo{
			EmpresaID: empresaID, ArticuloID: articuloID, UnidadID: unidadID, Tipo: tipo,
			Cantidad: 1, SedeID: sede, Costo: costo, Fecha: a.Fecha,
			Motivo: a.Motivo, UsuarioID: usuarioID,
		}); err != nil {
			return err
		}
	} else {
		// Ajuste de cantidad: el signo de la diferencia elige el tipo.
		modo, nombre, _, err := datosArticulo(ctx, tx, empresaID, a.ArticuloID)
		if err != nil {
			return err
		}
		if modo == ModoUnidad {
			return fmt.Errorf("%w: «%s» se controla por unidad, así que el ajuste va sobre una unidad concreta", ErrCantidadInvalida, nombre)
		}
		sedeNom, err := nombreSede(ctx, tx, empresaID, a.SedeID)
		if err != nil {
			return err
		}
		tipo, cantidad := MovAjusteMas, a.Diferencia
		if a.Diferencia < 0 {
			tipo, cantidad = MovAjusteMenos, -a.Diferencia
			// Un ajuste no puede dejar la existencia en negativo: eso no es una corrección, es un
			// dato imposible que después nadie puede explicar.
			hay, err := existenciaDe(ctx, tx, empresaID, a.ArticuloID, a.SedeID)
			if err != nil {
				return err
			}
			if hay < cantidad {
				return &SinExistenciaError{Articulo: nombre, Sede: sedeNom, Hay: hay, Pedido: cantidad}
			}
		}
		if _, err := insertarMovimiento(ctx, tx, movimientoNuevo{
			EmpresaID: empresaID, ArticuloID: a.ArticuloID, Tipo: tipo, Cantidad: cantidad,
			SedeID: a.SedeID, Fecha: a.Fecha, Motivo: a.Motivo, UsuarioID: usuarioID,
		}); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("inventario: commit ajuste: %w", err)
	}
	return nil
}
