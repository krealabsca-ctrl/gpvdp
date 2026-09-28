package grupo

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// La marca `movimiento_bancario.incluido` en el Consolidado del Grupo.
//
// Incidente real (16-set-2026): la tesorera importó el mismo estado de cuenta en dos cuentas
// Promerica. La corrección marca los movimientos de la cuenta equivocada con `incluido = false`.
// Las tres consultas de dinero de este paquete no miraban esa marca, así que el Consolidado —la
// pantalla que mira el Director— iba a seguir contando la plata duplicada después de corregir.
//
// Por qué va contra PostgreSQL de verdad: lo que falla o no falla es el SQL, y service_test.go
// corre contra un fakeRepo, así que estas consultas no se ejercitan en ningún otro lado. Un doble
// no distingue un `AND m.incluido` puesto de uno olvidado.
//
// Toda la prueba corre en un ESQUEMA temporal propio, con las tablas copiadas de las reales (LIKE)
// y vacías: no lee ni escribe un solo dato de ninguna empresa. La versión anterior insertaba dos
// empresas en `public` y las borraba con DELETE; con PROBE_DSN apuntando a producción eso escribía
// —y borraba físicamente— en producción. Es el mismo patrón de aislamiento que usa
// cxc/repository_planilla_incluido_test.go.
//
// Sin base disponible la prueba se salta con un motivo, no falla.

// Las tablas que tocan las tres consultas. Se copian vacías al esquema de prueba para que el
// `search_path` no pueda caer nunca en las reales.
var tablasDelGrupo = []string{
	"empresa", "banco", "cuenta_bancaria", "concepto", "clasificacion", "movimiento_bancario",
}

const (
	fixEmpresaA = "9b000000-0000-0000-0000-0000000000a1"
	fixEmpresaB = "9b000000-0000-0000-0000-0000000000b1"
	fixEmpresaC = "9b000000-0000-0000-0000-0000000000b2"
	fixBanco    = "9b000000-0000-0000-0000-0000000000c1"
	fixCuenta   = "9b000000-0000-0000-0000-0000000000c2"
	fixCuentaC  = "9b000000-0000-0000-0000-0000000000c3"
	fixConIngr  = "9b000000-0000-0000-0000-0000000000d1"
	fixConGasto = "9b000000-0000-0000-0000-0000000000d2"
	fixClaIngr  = "9b000000-0000-0000-0000-0000000000e1"
	fixClaGasto = "9b000000-0000-0000-0000-0000000000e2"
	fixClaInter = "9b000000-0000-0000-0000-0000000000e3"

	fixPeriodo   = "1999-01"
	fixNombreA   = "ZZ Prueba Incluido A"
	fixNombreB   = "ZZ Prueba Incluido B"
	fixNombreC   = "ZZ Prueba Incluido C"
	fixPartidaIn = "ZZ Venta de servicios"
	fixPartidaGa = "ZZ Proveedores"
	fixPartidaIE = "Regalias ZZ Prueba Incluido B"
)

// montarFixture deja, en el período 1999-01:
//   - empresa A con 8 movimientos, donde CADA uno tiene su gemelo excluido (la importación
//     duplicada ya corregida). El gemelo lleva un monto DISTINTO a propósito: si alguna consulta
//     lo contara, el número no sería «el doble» sino uno que no cuadra con nada, que es
//     exactamente lo que vio el Director;
//   - empresa B sin un solo movimiento, que es el caso que revienta si el filtro se pone en el
//     WHERE del LEFT JOIN en vez de dentro de cada FILTER;
//   - empresa C cuyo mes ENTERO quedó excluido: aporta cero pero sí tiene movimientos, y el aviso
//     no puede decirle «no tiene ningún movimiento» porque eso manda a re-importar el duplicado.
func montarFixture(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	dsn := os.Getenv("GPVDP_TEST_DSN")
	if dsn == "" {
		dsn = os.Getenv("PROBE_DSN")
	}
	if dsn == "" {
		dsn = "postgres://gpvdp:localdev@127.0.0.1:5432/gpvdp?sslmode=disable"
	}
	conexion, cancelar := context.WithTimeout(ctx, 5*time.Second)
	defer cancelar()
	admin, err := pgx.Connect(conexion, dsn)
	if err != nil {
		t.Skipf("sin base local (%v): levantá `docker compose up -d db` o poné GPVDP_TEST_DSN", err)
	}

	esquema := fmt.Sprintf("grupo_incluido_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+esquema); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("crear esquema de prueba: %v", err)
	}
	// El borrado queda registrado ANTES que el cierre del pool: t.Cleanup corre al revés, así que
	// el pool se cierra primero y el DROP no se queda esperando conexiones.
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+esquema+" CASCADE")
		_ = admin.Close(ctx)
	})
	for _, tabla := range tablasDelGrupo {
		if _, err := admin.Exec(ctx,
			fmt.Sprintf("CREATE TABLE %s.%s (LIKE public.%s INCLUDING DEFAULTS)", esquema, tabla, tabla)); err != nil {
			t.Fatalf("copiar la tabla %s: %v", tabla, err)
		}
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	// Solo el esquema de prueba: si una consulta nombrara una tabla que no copié, la prueba grita
	// en vez de leer callada los datos reales.
	cfg.ConnConfig.RuntimeParams["search_path"] = esquema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture: %v\nSQL: %s", err, sql)
		}
	}

	ejecutar(`INSERT INTO empresa (id, nombre) VALUES ($1::uuid, $2), ($3::uuid, $4), ($5::uuid, $6)`,
		fixEmpresaA, fixNombreA, fixEmpresaB, fixNombreB, fixEmpresaC, fixNombreC)
	ejecutar(`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'ZZ Banco')`,
		fixBanco, fixEmpresaA)
	ejecutar(`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
	          VALUES ($1::uuid, $2::uuid, $4::uuid, 'CRC', 'ZZ Cuenta'),
	                 ($3::uuid, $5::uuid, $4::uuid, 'CRC', 'ZZ Cuenta C')`,
		fixCuenta, fixEmpresaA, fixCuentaC, fixBanco, fixEmpresaC)
	ejecutar(`INSERT INTO concepto (id, empresa_id, nombre, naturaleza, naturaleza_declarada)
	          VALUES ($1::uuid, $3::uuid, 'ZZ Ingresos', 'INGRESO', true),
	                 ($2::uuid, $3::uuid, 'ZZ Gastos',   'GASTO',   true)`,
		fixConIngr, fixConGasto, fixEmpresaA)
	ejecutar(`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
	          VALUES ($1::uuid, $4::uuid, $5::uuid, $6),
	                 ($2::uuid, $4::uuid, $7::uuid, $8),
	                 ($3::uuid, $4::uuid, $7::uuid, $9)`,
		fixClaIngr, fixClaGasto, fixClaInter, fixEmpresaA,
		fixConIngr, fixPartidaIn, fixConGasto, fixPartidaGa, fixPartidaIE)

	mov := func(sufijo, empresa, cuenta, concepto, clasificacion, monto string, incluido bool) {
		t.Helper()
		estado := "REVISADO"
		var con, cla any = concepto, clasificacion
		if concepto == "" {
			estado, con, cla = "NO_IDENTIFICADO", nil, nil
		}
		ejecutar(`
			INSERT INTO movimiento_bancario
			  (id, empresa_id, cuenta_bancaria_id, fecha, descripcion, credito, monto_original,
			   monto_crc, concepto_id, clasificacion_id, estado_clasificacion, natural_key, incluido)
			VALUES ($1::uuid, $2::uuid, $3::uuid, DATE '1999-01-15', 'ZZ prueba incluido',
			        $4::numeric, $4::numeric, $4::numeric, $5::uuid, $6::uuid, $7, $8, $9)`,
			"9b000000-0000-0000-0000-0000000000f"+sufijo, empresa, cuenta, monto,
			con, cla, estado, "zz-incluido-"+sufijo, incluido)
	}

	a := func(sufijo, concepto, clasificacion, monto string, incluido bool) {
		mov(sufijo, fixEmpresaA, fixCuenta, concepto, clasificacion, monto, incluido)
	}
	a("1", fixConIngr, fixClaIngr, "1000.00", true)
	a("2", fixConIngr, fixClaIngr, "500.00", false) // duplicado corregido
	a("3", fixConGasto, fixClaGasto, "200.00", true)
	a("4", fixConGasto, fixClaGasto, "100.00", false) // duplicado corregido
	a("5", "", "", "300.00", true)                    // sin clasificar
	a("6", "", "", "700.00", false)                   // sin clasificar y duplicado corregido
	a("7", fixConGasto, fixClaInter, "50.00", true)   // entre empresas
	a("8", fixConGasto, fixClaInter, "25.00", false)  // entre empresas y duplicado corregido

	// Empresa C: su mes entero es la importación duplicada, ya revertida.
	mov("9", fixEmpresaC, fixCuentaC, fixConIngr, fixClaIngr, "4729798.13", false)

	return ctx, pool
}

func igualCRC(t *testing.T, que, got, want string) {
	t.Helper()
	g, err := decimal.NewFromString(got)
	if err != nil {
		t.Fatalf("%s: monto ilegible %q: %v", que, got, err)
	}
	w := decimal.RequireFromString(want)
	if !g.Equal(w) {
		t.Errorf("%s = %s, quiere %s (está contando movimientos excluidos)", que, got, want)
	}
}

// ResumenPorEmpresa es la tabla que mira el Director y la única fuente de los totales del grupo.
func TestResumenPorEmpresaNoCuentaLoExcluido(t *testing.T) {
	ctx, pool := montarFixture(t)

	filas, err := NewRepository(pool).ResumenPorEmpresa(ctx,
		[]string{fixEmpresaA, fixEmpresaB, fixEmpresaC}, fixPeriodo)
	if err != nil {
		t.Fatalf("ResumenPorEmpresa: %v", err)
	}
	if len(filas) != 3 {
		t.Fatalf("filas = %d, quiere 3 (A con datos, B vacía, C toda excluida)", len(filas))
	}

	a := filas[0] // ORDER BY e.nombre
	if a.EmpresaID != fixEmpresaA {
		t.Fatalf("la primera fila es %s, quiere %s", a.Empresa, fixNombreA)
	}
	igualCRC(t, "ingresos", a.IngresosCRC, "1000.00")           // sin el filtro: 1500.00
	igualCRC(t, "gastos", a.GastosCRC, "250.00")                // sin el filtro: 375.00
	igualCRC(t, "neutro", a.NeutroCRC, "0")                     //
	igualCRC(t, "sin clasificar", a.SinClasificarCRC, "300.00") // sin el filtro: 1000.00
	if a.SinClasificar != 1 {
		t.Errorf("sin clasificar (cantidad) = %d, quiere 1", a.SinClasificar)
	}
	// El denominador del % también se limpia: 1250 de 1550, no 1875 de 2875 (65,2 %).
	if a.PctClasificado != "80.6" {
		t.Errorf("pct clasificado = %s, quiere 80.6", a.PctClasificado)
	}

	// EL CONTEO NO ESCONDE: los 8 movimientos se cuentan y los 4 excluidos se publican aparte, igual
	// que en Bancos. Si el filtro se pusiera en el ON del LEFT JOIN, `Movimientos` bajaría a 4 y una
	// empresa cuyo mes entero quedó excluido pasaría por «no tiene ningún movimiento» (ver la fila C).
	if a.Movimientos != 8 {
		t.Errorf("movimientos = %d, quiere 8 (los excluidos se cuentan, no suman)", a.Movimientos)
	}
	if a.Excluidos != 4 {
		t.Errorf("excluidos = %d, quiere 4", a.Excluidos)
	}

	// LA GUARDA DEL LEFT JOIN: la empresa sin movimientos en el período tiene que seguir apareciendo
	// con 0. Si el filtro se pasa al WHERE, esta fila desaparece y el consolidado omite una empresa
	// entera en silencio, sin SinDatos y sin aviso.
	b := filas[1]
	if b.EmpresaID != fixEmpresaB {
		t.Fatalf("la empresa sin movimientos desapareció del consolidado: filas = %+v", filas)
	}
	if b.Movimientos != 0 || b.Excluidos != 0 {
		t.Errorf("%s: movimientos = %d y excluidos = %d, quiere 0 y 0", b.Empresa, b.Movimientos, b.Excluidos)
	}

	// LA GUARDA DEL AVISO: C tiene movimientos, todos excluidos. Aporta cero, pero no es lo mismo
	// que «no cargaron nada»: decírselo así manda a re-importar el archivo duplicado.
	c := filas[2]
	if c.EmpresaID != fixEmpresaC {
		t.Fatalf("la tercera fila es %s, quiere %s", c.Empresa, fixNombreC)
	}
	if c.Movimientos != 1 || c.Excluidos != 1 {
		t.Errorf("%s: movimientos = %d y excluidos = %d, quiere 1 y 1", c.Empresa, c.Movimientos, c.Excluidos)
	}
	igualCRC(t, "ingresos de "+c.Empresa, c.IngresosCRC, "0")
}

// OperacionesEntreEmpresas: el bloque que explica cuánto del total se movió entre bolsillos del
// mismo grupo. Si suma lo excluido, dice más colones de los que el total contiene.
func TestOperacionesEntreEmpresasNoCuentaLoExcluido(t *testing.T) {
	ctx, pool := montarFixture(t)

	ops, err := NewRepository(pool).OperacionesEntreEmpresas(ctx,
		[]string{fixEmpresaA, fixEmpresaB, fixEmpresaC}, fixPeriodo)
	if err != nil {
		t.Fatalf("OperacionesEntreEmpresas: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("operaciones = %d, quiere 1: %+v", len(ops), ops)
	}
	o := ops[0]
	if o.Contraparte != fixNombreB || o.Partida != fixPartidaIE {
		t.Fatalf("operación inesperada: %+v", o)
	}
	igualCRC(t, "monto entre empresas", o.MontoCRC, "50.00") // sin el filtro: 75.00
	if o.Movimientos != 1 {
		t.Errorf("movimientos entre empresas = %d, quiere 1", o.Movimientos)
	}
}

// PartidasDelGrupo: el top-15 que lee el Director. Acá la duplicación no solo miente sobre un
// número, también puede empujar una partida legítima fuera del corte.
func TestPartidasDelGrupoNoCuentanLoExcluido(t *testing.T) {
	ctx, pool := montarFixture(t)

	partidas, err := NewRepository(pool).PartidasDelGrupo(ctx,
		[]string{fixEmpresaA, fixEmpresaB, fixEmpresaC}, fixPeriodo, topePartidas)
	if err != nil {
		t.Fatalf("PartidasDelGrupo: %v", err)
	}
	porNombre := map[string]PartidaGrupo{}
	for _, p := range partidas {
		porNombre[p.Partida] = p
	}
	// Sin el filtro: 1500.00, 300.00 y 75.00.
	for _, c := range []struct{ partida, monto string }{
		{fixPartidaIn, "1000.00"},
		{fixPartidaGa, "200.00"},
		{fixPartidaIE, "50.00"},
	} {
		p, ok := porNombre[c.partida]
		if !ok {
			t.Errorf("falta la partida %q en el top: %+v", c.partida, partidas)
			continue
		}
		igualCRC(t, "partida "+c.partida, p.MontoCRC, c.monto)
		if p.Movimientos != 1 {
			t.Errorf("partida %q: movimientos = %d, quiere 1", c.partida, p.Movimientos)
		}
		if len(p.PorEmpresa) != 1 {
			t.Fatalf("partida %q: desglose = %+v, quiere una sola empresa", c.partida, p.PorEmpresa)
		}
		igualCRC(t, "aporte de "+p.PorEmpresa[0].Empresa+" a "+c.partida, p.PorEmpresa[0].MontoCRC, c.monto)
	}
}
