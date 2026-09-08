package grupo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgRepository struct{ pool *pgxpool.Pool }

// NewRepository construye el repositorio.
func NewRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool: pool} }

// EmpresasVisibles es la pieza de seguridad de este paquete.
//
// Devuelve las empresas donde el usuario tiene MEMBRESÍA y, en esa empresa, un rol con el permiso
// pedido. Las dos condiciones juntas: la membresía sola no alcanza (se puede pertenecer a una empresa
// con un rol que no ve bancos) y el permiso solo existe dentro de una empresa.
//
// El usuarioID sale del token, nunca del pedido. Y ADMIN es bypass, igual que en rbac.Tiene: si se
// resolviera distinto acá, un admin vería menos empresas en el consolidado que entrando de a una, y
// nadie entendería por qué.
func (r *pgRepository) EmpresasVisibles(ctx context.Context, usuarioID, permiso string, esAdmin bool) ([]EmpresaVisible, error) {
	q := `
		SELECT e.id::text, e.nombre, ro.codigo
		FROM usuario_empresa_rol uer
		JOIN empresa e ON e.id = uer.empresa_id
		JOIN rol ro ON ro.id = uer.rol_id
		WHERE uer.usuario_id = $1::uuid`
	if !esAdmin {
		// El rol de ESA empresa tiene que tener el permiso EN ESA empresa: rol_permiso lleva
		// empresa_id, así que un mismo rol puede tener permisos distintos en cada una.
		q += `
		  AND EXISTS (
		    SELECT 1 FROM rol_permiso rp
		    JOIN permiso p ON p.id = rp.permiso_id
		    WHERE rp.rol_id = uer.rol_id AND rp.empresa_id = uer.empresa_id AND p.codigo = $2
		  )`
	}
	q += ` ORDER BY e.nombre`

	var rows interface {
		Next() bool
		Scan(...any) error
		Close()
		Err() error
	}
	var err error
	if esAdmin {
		rows, err = r.pool.Query(ctx, q, usuarioID)
	} else {
		rows, err = r.pool.Query(ctx, q, usuarioID, permiso)
	}
	if err != nil {
		return nil, fmt.Errorf("grupo: empresas visibles: %w", err)
	}
	defer rows.Close()

	out := []EmpresaVisible{}
	for rows.Next() {
		var e EmpresaVisible
		if err := rows.Scan(&e.ID, &e.Nombre, &e.Rol); err != nil {
			return nil, fmt.Errorf("grupo: scan empresa visible: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ContarEmpresas cuenta las del sistema. No filtra por nada: es solo el denominador para poder decir
// «se incluyen 2 de 3».
func (r *pgRepository) ContarEmpresas(ctx context.Context) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*)::int FROM empresa`).Scan(&n); err != nil {
		return 0, fmt.Errorf("grupo: contar empresas: %w", err)
	}
	return n, nil
}

// sqlNaturaleza resuelve ingreso/gasto/neutro desde la NATURALEZA declarada en el concepto, no desde
// el signo del movimiento. Es la misma regla que usa el dashboard de Bancos: el ahorro, las reservas
// y los aportes entre empresas son NEUTRO, y antes de que existiera este campo inflaban los gastos
// de agosto en ₡35,3M.
const sqlNaturaleza = `COALESCE(co.naturaleza, 'SIN_CLASIFICAR')`

// ResumenPorEmpresa da el aporte de cada empresa al consolidado, para UN período.
//
// El pct_clasificado se calcula por MONTO y no por cantidad de movimientos: 155 movimientos chicos
// sin clasificar no dicen lo mismo que ₡32,6M sin clasificar, y lo que decide si el número sirve es
// la plata.
func (r *pgRepository) ResumenPorEmpresa(ctx context.Context, empresaIDs []string, periodo string) ([]FilaEmpresa, error) {
	const q = `
		SELECT e.id::text, e.nombre,
		  COALESCE(SUM(m.monto_crc) FILTER (WHERE ` + sqlNaturaleza + ` = 'INGRESO'), 0)::text,
		  COALESCE(SUM(m.monto_crc) FILTER (WHERE ` + sqlNaturaleza + ` = 'GASTO'), 0)::text,
		  COALESCE(SUM(m.monto_crc) FILTER (WHERE ` + sqlNaturaleza + ` = 'NEUTRO'), 0)::text,
		  count(m.id)::int,
		  count(m.id) FILTER (WHERE m.clasificacion_id IS NULL)::int,
		  COALESCE(SUM(m.monto_crc) FILTER (WHERE m.clasificacion_id IS NULL), 0)::text,
		  COALESCE(SUM(m.monto_crc), 0)::text
		FROM empresa e
		LEFT JOIN movimiento_bancario m ON m.empresa_id = e.id
		     AND to_char(m.fecha, 'YYYY-MM') = $2
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		LEFT JOIN concepto co ON co.id = cl.concepto_id
		WHERE e.id = ANY($1::uuid[])
		GROUP BY e.id, e.nombre
		ORDER BY e.nombre`

	rows, err := r.pool.Query(ctx, q, empresaIDs, periodo)
	if err != nil {
		return nil, fmt.Errorf("grupo: resumen por empresa: %w", err)
	}
	defer rows.Close()

	out := []FilaEmpresa{}
	for rows.Next() {
		var f FilaEmpresa
		var totalCRC string
		if err := rows.Scan(&f.EmpresaID, &f.Empresa, &f.IngresosCRC, &f.GastosCRC, &f.NeutroCRC,
			&f.Movimientos, &f.SinClasificar, &f.SinClasificarCRC, &totalCRC); err != nil {
			return nil, fmt.Errorf("grupo: scan fila empresa: %w", err)
		}
		f.PctClasificado = pctClasificado(totalCRC, f.SinClasificarCRC)
		out = append(out, f)
	}
	return out, rows.Err()
}

// sqlPartidaNombraEmpresa detecta las partidas que nombran a OTRA empresa del grupo.
//
// Es una heurística por nombre y hay que decirlo: hoy no existe una marca de «esto es intercompañía».
// Detecta lo que el catálogo ya nombra así —«Regalias Memorial Pets», «Coopeprofa»— y por eso la
// pantalla presenta el resultado como «lo que se pudo identificar», no como la cifra cerrada. El día
// que haya una marca formal, esta expresión se reemplaza por ella y la vista no cambia.
const sqlPartidaNombraEmpresa = `
	(SELECT o.nombre FROM empresa o
	 WHERE o.id <> m.empresa_id AND o.id = ANY($1::uuid[])
	   AND (cl.nombre ILIKE '%' || o.nombre || '%' OR co.nombre ILIKE '%' || o.nombre || '%')
	 LIMIT 1)`

// OperacionesEntreEmpresas son los movimientos cuya partida nombra a otra empresa del grupo.
func (r *pgRepository) OperacionesEntreEmpresas(ctx context.Context, empresaIDs []string, periodo string) ([]OperacionInterna, error) {
	const q = `
		WITH marcadas AS (
		  SELECT m.empresa_id, m.monto_crc, cl.nombre AS partida, co.naturaleza,
		         ` + sqlPartidaNombraEmpresa + ` AS contraparte
		  FROM movimiento_bancario m
		  JOIN clasificacion cl ON cl.id = m.clasificacion_id
		  JOIN concepto co ON co.id = cl.concepto_id
		  WHERE m.empresa_id = ANY($1::uuid[]) AND to_char(m.fecha, 'YYYY-MM') = $2
		)
		SELECT e.id::text, e.nombre, mk.contraparte, mk.partida, mk.naturaleza,
		       count(*)::int, SUM(mk.monto_crc)::text
		FROM marcadas mk
		JOIN empresa e ON e.id = mk.empresa_id
		WHERE mk.contraparte IS NOT NULL
		GROUP BY e.id, e.nombre, mk.contraparte, mk.partida, mk.naturaleza
		ORDER BY SUM(mk.monto_crc) DESC`

	rows, err := r.pool.Query(ctx, q, empresaIDs, periodo)
	if err != nil {
		return nil, fmt.Errorf("grupo: operaciones entre empresas: %w", err)
	}
	defer rows.Close()

	out := []OperacionInterna{}
	for rows.Next() {
		var o OperacionInterna
		if err := rows.Scan(&o.EmpresaID, &o.Empresa, &o.Contraparte, &o.Partida, &o.Naturaleza,
			&o.Movimientos, &o.MontoCRC); err != nil {
			return nil, fmt.Errorf("grupo: scan operación interna: %w", err)
		}
		// AfectaEbitda no se resuelve acá a propósito: lo deriva el servicio, que es quien suma. Si se
		// llenara en el repositorio, el total y la bandera serían dos cálculos distintos del mismo hecho.
		out = append(out, o)
	}
	return out, rows.Err()
}

// PartidasDelGrupo suma cada partida en todas las empresas visibles, con el desglose de quién la
// genera. Sin el desglose, una partida grande del grupo no dice si es un problema de todos o de uno.
func (r *pgRepository) PartidasDelGrupo(ctx context.Context, empresaIDs []string, periodo string, limite int) ([]PartidaGrupo, error) {
	const q = `
		WITH base AS (
		  SELECT cl.nombre AS partida, co.nombre AS concepto, co.naturaleza,
		         e.nombre AS empresa, m.monto_crc
		  FROM movimiento_bancario m
		  JOIN empresa e ON e.id = m.empresa_id
		  JOIN clasificacion cl ON cl.id = m.clasificacion_id
		  JOIN concepto co ON co.id = cl.concepto_id
		  WHERE m.empresa_id = ANY($1::uuid[]) AND to_char(m.fecha, 'YYYY-MM') = $2
		    -- Los NEUTRO quedan afuera: son traslados y ahorro, no el gasto ni el ingreso del grupo.
		    AND co.naturaleza IN ('INGRESO', 'GASTO')
		),
		totales AS (
		  SELECT partida, concepto, naturaleza, SUM(monto_crc) AS monto, count(*)::int AS movs
		  FROM base GROUP BY 1, 2, 3
		  ORDER BY SUM(monto_crc) DESC LIMIT $3
		)
		SELECT t.partida, t.concepto, t.naturaleza, t.monto::text, t.movs,
		       b.empresa, SUM(b.monto_crc)::text
		FROM totales t
		JOIN base b ON b.partida = t.partida AND b.concepto = t.concepto
		GROUP BY t.partida, t.concepto, t.naturaleza, t.monto, t.movs, b.empresa
		ORDER BY t.monto DESC, SUM(b.monto_crc) DESC`

	rows, err := r.pool.Query(ctx, q, empresaIDs, periodo, limite)
	if err != nil {
		return nil, fmt.Errorf("grupo: partidas del grupo: %w", err)
	}
	defer rows.Close()

	// Las filas vienen una por (partida, empresa): se agrupan acá conservando el orden del SQL.
	out := []PartidaGrupo{}
	indice := map[string]int{}
	for rows.Next() {
		var partida, concepto, naturaleza, monto, empresa, aporte string
		var movs int
		if err := rows.Scan(&partida, &concepto, &naturaleza, &monto, &movs, &empresa, &aporte); err != nil {
			return nil, fmt.Errorf("grupo: scan partida del grupo: %w", err)
		}
		clave := concepto + "›" + partida
		i, ok := indice[clave]
		if !ok {
			out = append(out, PartidaGrupo{
				Partida: partida, Concepto: concepto, Naturaleza: naturaleza,
				MontoCRC: monto, Movimientos: movs, PorEmpresa: []AporteEmpresa{},
			})
			i = len(out) - 1
			indice[clave] = i
		}
		out[i].PorEmpresa = append(out[i].PorEmpresa, AporteEmpresa{Empresa: empresa, MontoCRC: aporte})
	}
	return out, rows.Err()
}
