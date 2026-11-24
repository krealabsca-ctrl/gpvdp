package seed

// Sembrar los roles base contra PostgreSQL de verdad (19-oct-2026).
//
// EL DEFECTO QUE ESTA PRUEBA FIJA. Al instalar el sistema en un servidor limpio, la siembra moría
// en el primer rol:
//
//	seed rol ADMIN: there is no unique or exclusion constraint matching the ON CONFLICT
//	specification (SQLSTATE 42P10)
//
// La migración 0067 reemplazó el único de `rol` por DOS índices PARCIALES, y un
// `ON CONFLICT (codigo)` sin el predicado no calza ninguno.
//
// POR QUÉ NADIE LO VIO EN 36 MIGRACIONES. El arranque registra el error y CONTINÚA. El backend
// queda arriba, responde healthz y pasa cualquier chequeo de «¿está vivo?» — pero la base quedó
// sin empresas, y como entrar exige elegir una, el sistema está instalado y es inusable. En las
// bases que ya tenían sus roles el `DO UPDATE` nunca se ejecutaba, así que el camino que falla
// solo aparece en una instalación nueva.
//
// POR QUÉ ESTO NO SE PUEDE PROBAR SIN SQL. El error vive en el calce entre una sentencia y la
// forma del índice: no hay doble que lo reproduzca, y leer el fuente solo diría que el texto está
// ahí. Por eso la tabla se construye acá tal como la deja la 0067, con los dos índices parciales.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// rolComoLaDeja0067 es la forma real de la tabla: dos únicos PARCIALES, no uno entero.
const rolComoLaDeja0067 = `
CREATE TABLE rol (
	id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	codigo      text NOT NULL,
	nombre      text NOT NULL,
	descripcion text,
	creado_en   timestamptz NOT NULL DEFAULT now(),
	empresa_id  uuid
);
CREATE UNIQUE INDEX uq_rol_codigo_base    ON rol (codigo)             WHERE empresa_id IS NULL;
CREATE UNIQUE INDEX uq_rol_codigo_empresa ON rol (empresa_id, codigo) WHERE empresa_id IS NOT NULL;`

func poolRolBase(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("GPVDP_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://gpvdp:localdev@127.0.0.1:5432/gpvdp?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("sin base de datos (%v)", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("sin base de datos (%v)", err)
	}
	esquema := fmt.Sprintf("prueba_rol_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+esquema); err != nil {
		pool.Close()
		t.Fatalf("crear esquema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA "+esquema+" CASCADE")
		pool.Close()
	})
	if _, err := pool.Exec(ctx, "SET search_path TO "+esquema); err != nil {
		t.Fatalf("search_path: %v", err)
	}
	// El pool reparte varias conexiones y el search_path es por conexión: fijarlo en la base de
	// datos evita que una consulta caiga en otra conexión y no encuentre la tabla.
	if _, err := pool.Exec(ctx, "ALTER ROLE CURRENT_USER SET search_path TO "+esquema); err != nil {
		t.Fatalf("fijar search_path: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "ALTER ROLE CURRENT_USER RESET search_path")
	})
	if _, err := pool.Exec(ctx, rolComoLaDeja0067); err != nil {
		t.Fatalf("crear rol: %v", err)
	}
	return pool, ctx
}

// La sentencia que corre en producción tiene que poder correr contra la tabla que existe.
//
// Falsificación: quitarle a SQLUpsertRolBase el "WHERE empresa_id IS NULL" hace que este test
// falle con 42P10, que es exactamente el error que tumbó la instalación limpia.
func TestSembrarRolBaseCorreContraLosIndicesParciales(t *testing.T) {
	pool, ctx := poolRolBase(t)

	var id string
	if err := pool.QueryRow(ctx, SQLUpsertRolBase, "ADMIN", "Administrador", "Acceso total").Scan(&id); err != nil {
		t.Fatalf("la siembra del rol base no corre contra la tabla real: %v", err)
	}
	if id == "" {
		t.Fatal("el upsert no devolvió el id del rol, y la siembra lo necesita para los permisos")
	}
}

// Sembrar dos veces no puede duplicar ni fallar: el instalador se puede volver a correr, y el
// backend siembra en cada arranque mientras SEED_ON_START esté encendido.
func TestSembrarRolBaseDosVecesNoDuplica(t *testing.T) {
	pool, ctx := poolRolBase(t)

	var primero, segundo string
	if err := pool.QueryRow(ctx, SQLUpsertRolBase, "ADMIN", "Administrador", "x").Scan(&primero); err != nil {
		t.Fatalf("primera siembra: %v", err)
	}
	if err := pool.QueryRow(ctx, SQLUpsertRolBase, "ADMIN", "Administrador", "x").Scan(&segundo); err != nil {
		t.Fatalf("segunda siembra: %v", err)
	}
	if primero != segundo {
		t.Errorf("la segunda siembra creó OTRO rol ADMIN (%s ≠ %s): los permisos quedarían repartidos entre dos", primero, segundo)
	}

	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM rol WHERE codigo = 'ADMIN'").Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 1 {
		t.Errorf("quedaron %d roles ADMIN, se esperaba 1", n)
	}
}

// Un rol BASE y uno propio de una empresa pueden compartir código — para eso la 0067 partió el
// índice en dos. Si alguien "arreglara" el 42P10 volviendo a un único entero, esto lo delata.
func TestRolDeEmpresaPuedeRepetirElCodigoDelBase(t *testing.T) {
	pool, ctx := poolRolBase(t)

	if _, err := pool.Exec(ctx, "INSERT INTO rol (codigo, nombre) VALUES ('ADMIN', 'Administrador')"); err != nil {
		t.Fatalf("rol base: %v", err)
	}
	_, err := pool.Exec(ctx,
		"INSERT INTO rol (codigo, nombre, empresa_id) VALUES ('ADMIN', 'Admin de la empresa', gen_random_uuid())")
	if err != nil {
		t.Fatalf("una empresa no pudo tener su propio rol ADMIN: %v", err)
	}

	// Y dos roles base con el mismo código siguen prohibidos, que es lo que el índice parcial cuida.
	_, err = pool.Exec(ctx, "INSERT INTO rol (codigo, nombre) VALUES ('ADMIN', 'Otro base')")
	if err == nil {
		t.Error("se crearon DOS roles base con código ADMIN: el único parcial dejó de cuidar")
	} else if !strings.Contains(err.Error(), "uq_rol_codigo_base") {
		t.Errorf("falló por otra razón, no por el único esperado: %v", err)
	}
}
