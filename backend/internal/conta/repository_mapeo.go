package conta

// Consultas del mapeo contable (mig 0110). El porqué está en mapeo.go.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// filasDelMapeoSQL es LA consulta de la Fase 2.
//
// Trae cada clasificación de la empresa con tres cosas que hoy viven separadas: lo que el ERP
// declara (concepto y naturaleza), cuántos movimientos la usan, y la decisión contable si ya se
// tomó. Juntarlas es lo que permite ver el cruce de naturaleza y ordenar el trabajo por peso.
//
// EL PESO SE AGRUPA UNA VEZ, NO UNA VEZ POR FILA (12-nov-2026). La primera versión contaba los
// movimientos con una subconsulta correlacionada, con el argumento de que agrupar 31.195
// movimientos para mostrar 170 filas sería más caro. Medido, es al revés y por mucho: la
// subconsulta se ejecuta una vez por clasificación y recorre el índice entero cada vez
// —239.366 bloques y 557 ms—, mientras que el GROUP BY recorre la tabla UNA vez: 1.414 bloques
// y 9 ms. Sesenta veces más rápido, y con lugar para traer el monto sin pagar nada.
// Es el mismo defecto que ya se corrigió en el tablero de CxP.
const filasDelMapeoSQL = `
	SELECT cl.id::text, co.nombre, cl.nombre,
	       COALESCE(co.naturaleza,''), co.naturaleza_declarada,
	       COALESCE(peso.movimientos, 0),
	       -- El cast a numeric(18,2) ANTES del texto: sin él, una partida con movimientos sale
	       -- «1500000.00» y una sin movimientos sale «0», porque el literal del COALESCE es entero.
	       -- Dos formatos distintos en la misma columna rompen cualquier lector del otro lado.
	       COALESCE(peso.monto, 0)::numeric(18,2)::text,
	       COALESCE(mp.estado,''), COALESCE(mp.motivo,''),
	       cc.id::text, COALESCE(cc.codigo,''), COALESCE(cc.nombre,''), COALESCE(cc.clase,'')
	FROM clasificacion cl
	JOIN concepto co ON co.id = cl.concepto_id
	LEFT JOIN (
	    SELECT m.clasificacion_id, COUNT(*)::int AS movimientos, SUM(abs(m.monto_crc)) AS monto
	      FROM movimiento_bancario m
	     WHERE m.empresa_id = $1::uuid AND m.incluido
	     GROUP BY m.clasificacion_id
	) peso ON peso.clasificacion_id = cl.id
	LEFT JOIN mapeo_contable mp ON mp.clasificacion_id = cl.id AND mp.empresa_id = $1::uuid
	LEFT JOIN cuenta_contable cc ON cc.id = mp.cuenta_id
	WHERE cl.empresa_id = $1::uuid AND cl.activo`

// FilasDelMapeo lista las clasificaciones con su decisión.
//
// `soloPendientes` deja fuera lo ya resuelto: con 170 clasificaciones, revisar la lista entera
// cada vez para encontrar las que faltan es el trabajo que esta pantalla existe para evitar.
func (r *pgRepository) FilasDelMapeo(ctx context.Context, empresaID string, soloPendientes, soloConMovimientos bool) ([]FilaDelMapeo, error) {
	q := filasDelMapeoSQL
	if soloPendientes {
		q += ` AND mp.id IS NULL`
	}
	if soloConMovimientos {
		q += ` AND peso.movimientos IS NOT NULL`
	}
	// LO QUE MÁS PLATA MUEVE, PRIMERO — no lo que más veces aparece.
	//
	// Antes ordenaba por cantidad de movimientos, y eso manda a trabajar en el orden equivocado.
	// Medido en Valle de Paz sobre ₡2.956 M: «Traslado de Fondos» mueve ₡1.115 M en 239
	// movimientos y quedaba OCTAVO, detrás de «Depósito de Clientes», que mueve doce veces menos
	// plata en 7.960 movimientos. «Utilidades» mueve ₡63 M en 6 movimientos y caía en el puesto 54.
	//
	// Con este orden, SIETE clasificaciones de 131 cubren el 80 % del dinero y 24 cubren el 95 %:
	// la diferencia entre una pantalla con 131 pendientes y una tarde de trabajo con sentido.
	q += ` ORDER BY COALESCE(peso.monto, 0) DESC, COALESCE(peso.movimientos, 0) DESC, co.nombre, cl.nombre`

	rows, err := r.pool.Query(ctx, q, empresaID)
	if err != nil {
		return nil, fmt.Errorf("conta: listar el mapeo: %w", err)
	}
	defer rows.Close()

	out := make([]FilaDelMapeo, 0)
	for rows.Next() {
		var f FilaDelMapeo
		if err := rows.Scan(&f.ClasificacionID, &f.Concepto, &f.Clasificacion,
			&f.NaturalezaConcepto, &f.NaturalezaDeclarada, &f.Movimientos, &f.MontoCRC,
			&f.Estado, &f.Motivo, &f.CuentaID, &f.CuentaCodigo, &f.CuentaNombre, &f.CuentaClase); err != nil {
			return nil, fmt.Errorf("conta: scan fila del mapeo: %w", err)
		}
		// El cruce se calcula acá, en Go, y no en SQL: la regla es de negocio, tiene matices por
		// clase y tiene que decir POR QUÉ. Un CASE en la consulta devolvería un booleano mudo.
		f.Cruzada, f.AvisoCruce = NaturalezaCruzada(f.NaturalezaConcepto, f.NaturalezaDeclarada, f.CuentaClase)
		out = append(out, f)
	}
	return out, rows.Err()
}

// GuardarMapeo crea o reemplaza la decisión sobre una clasificación.
func (r *pgRepository) GuardarMapeo(ctx context.Context, empresaID string, d DecisionDeMapeo, usuarioID string) error {
	var cuenta any
	if d.CuentaID != "" {
		cuenta = d.CuentaID
	}
	const q = `
		INSERT INTO mapeo_contable (empresa_id, clasificacion_id, cuenta_id, estado, motivo, decidido_por)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, NULLIF($6,'')::uuid)
		ON CONFLICT (empresa_id, clasificacion_id) DO UPDATE
		SET cuenta_id = EXCLUDED.cuenta_id, estado = EXCLUDED.estado, motivo = EXCLUDED.motivo,
		    decidido_por = EXCLUDED.decidido_por, decidido_en = now(), actualizado_en = now()`
	if _, err := r.pool.Exec(ctx, q, empresaID, d.ClasificacionID, cuenta, d.Estado, d.Motivo, usuarioID); err != nil {
		return traducirMapeo(err)
	}
	return nil
}

// BorrarMapeo deshace la decisión: la clasificación vuelve a «falta decidir».
func (r *pgRepository) BorrarMapeo(ctx context.Context, empresaID, clasificacionID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM mapeo_contable WHERE empresa_id = $1::uuid AND clasificacion_id = $2::uuid`,
		empresaID, clasificacionID)
	if err != nil {
		if idInvalido(err) {
			return ErrMapeoNoEncontrado
		}
		return fmt.Errorf("conta: borrar el mapeo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMapeoNoEncontrado
	}
	return nil
}

// Cobertura cuenta cuánto falta, sobre el conjunto que de verdad bloquea.
//
// Las clasificaciones SIN movimientos se cuentan aparte: nadie las usó, así que no impiden
// contabilizar, y meterlas en el denominador haría que el porcentaje nunca llegue al 100 % y que
// el reporte deje de servir como semáforo.
func (r *pgRepository) Cobertura(ctx context.Context, empresaID string) (Cobertura, error) {
	filas, err := r.FilasDelMapeo(ctx, empresaID, false, false)
	if err != nil {
		return Cobertura{}, err
	}
	var c Cobertura
	for _, f := range filas {
		c.Total++
		if !f.NaturalezaDeclarada {
			c.SinNaturaleza++
		}
		if f.Cruzada {
			c.Cruzadas++
		}
		if f.Movimientos == 0 {
			continue
		}
		c.ConMovimientos++
		switch f.Estado {
		case EstadoMapeada:
			c.Mapeadas++
		case EstadoNoContabilizable:
			c.NoContabilizables++
		default:
			c.SinDecidir++
		}
	}
	return c, nil
}

// traducirMapeo convierte el rechazo de la base en un error de dominio.
//
// EL ORDEN IMPORTA Y NO ES COSMÉTICO. `traducir()` también atiende los check_violation (23514) y
// devuelve el texto del disparador envuelto, sin identificarlo. Si se llamara primero, el caso de
// la cuenta de agrupación nunca llegaría a `ErrCuentaDeAgrupacion`: saldría como 422 genérico y el
// `errors.Is` de cualquier llamador fallaría en silencio. Pasó: lo encontró la prueba de la
// invariante 2.
func traducirMapeo(err error) error {
	// El disparador de la 0110 y el CHECK de la tabla hablan con su propio texto, que ya explica
	// el porqué. Se mira ANTES de delegar.
	msg := errorDePostgres(err)
	if strings.Contains(msg, "agrupa a otras") {
		// Se conserva el texto del disparador —nombra CUÁL cuenta: «la cuenta 1-00-00-000-000
		// agrupa a otras»— y a la vez se hace reconocible con errors.Is. En un lote de veinte,
		// un mensaje que no dice qué cuenta obliga a probar una por una.
		return deAgrupacion{msg}
	}
	if e := traducir(err); !errors.Is(e, err) {
		return e
	}
	if msg != "" {
		return fmt.Errorf("conta: %s", msg)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMapeoNoEncontrado
	}
	return fmt.Errorf("conta: guardar el mapeo: %w", err)
}

// deAgrupacion lleva el texto del disparador y responde que SÍ a ErrCuentaDeAgrupacion.
//
// Un error tiene dos lectores con necesidades distintas: la persona quiere saber qué cuenta falló
// y el handler quiere saber qué código HTTP devolver. Un centinela pelado sirve al segundo y le
// quita al primero el único dato accionable.
type deAgrupacion struct{ msg string }

func (e deAgrupacion) Error() string { return "conta: " + e.msg }
func (e deAgrupacion) Unwrap() error { return ErrCuentaDeAgrupacion }
