package cxc

// Una cuota negativa no puede tumbar la importación entera (incidente 87CA1C, 13-oct-2026).
//
// LO QUE PASÓ. Al confirmar la cartera, el contrato CO139774 traía una cuota negativa.
// `contrato_cxc.cuota_vigente` tiene CHECK (>= 0) —y con razón: una cuota negativa generaría
// cargos negativos—. Como el upsert corre DENTRO de una transacción, el rechazo de esa fila
// abortó la importación completa y el usuario vio «Error interno del sistema. Pasá esta
// referencia a soporte: 87CA1C». El lector ya había marcado la fila con «cuota en cero o
// negativa»; lo que faltaba era que el repositorio le hiciera caso.
//
// POR QUÉ ESTA PRUEBA VA CONTRA POSTGRES DE VERDAD. Lo que falla es el CHECK de la tabla. Un
// doble no lo tiene, así que no puede distinguir una cuota acotada de una que no lo está.
//
// Y POR QUÉ `INCLUDING ALL` Y NO `INCLUDING DEFAULTS`: el otro helper PG de este paquete copia
// las tablas con DEFAULTS, que NO trae los CHECK. Con ese molde esta prueba pasaría en verde con
// el defecto puesto —el INSERT entraría con la cuota negativa— y no probaría nada.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

func baseDeContratos(t *testing.T) (*pgRepository, string) {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("GPVDP_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://gpvdp:localdev@127.0.0.1:5432/gpvdp?sslmode=disable"
	}
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("sin base de datos (%v)", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("sin base de datos (%v)", err)
	}
	esquema := fmt.Sprintf("prueba_cuota_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+esquema); err != nil {
		admin.Close()
		t.Fatalf("crear esquema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+esquema+" CASCADE")
		admin.Close()
	})
	// INCLUDING ALL: es lo que trae los CHECK, que es justamente lo que se está probando.
	if _, err := admin.Exec(ctx,
		fmt.Sprintf("CREATE TABLE %s.contrato_cxc (LIKE public.contrato_cxc INCLUDING ALL)", esquema)); err != nil {
		t.Fatalf("copiar contrato_cxc: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = esquema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	var empresaID string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&empresaID); err != nil {
		t.Fatalf("empresa: %v", err)
	}
	return &pgRepository{pool: pool}, empresaID
}

// Antes del arreglo, este archivo entero no entraba: la fila mala abortaba la transacción y las
// buenas se perdían con ella.
func TestUnaCuotaNegativaNoTumbaLaImportacion(t *testing.T) {
	repo, empresaID := baseDeContratos(t)

	filas := []FilaContrato{
		{Numero: "CO000001", Cliente: "Cliente bueno", Cuota: decimal.RequireFromString("25000")},
		{
			Numero: "CO139774", Cliente: "Cliente con cuota negativa",
			Cuota:   decimal.RequireFromString("-12500"),
			Motivos: []string{"cuota en cero o negativa"},
		},
		{Numero: "CO000003", Cliente: "Otro bueno", Cuota: decimal.RequireFromString("30000")},
	}

	out, err := repo.GuardarContratos(context.Background(), empresaID, filas, Resolucion{
		Sedes: map[string]string{}, Modalidades: map[string]string{},
		FormasPago: map[string]string{}, Asociaciones: map[string]string{},
	})
	if err != nil {
		t.Fatalf("la importación se cayó entera por una cuota negativa: %v", err)
	}
	if out.Nuevos != 3 {
		t.Errorf("entraron %d contratos, quería 3: la fila mala no puede llevarse las buenas", out.Nuevos)
	}
}

// El contrato malo ENTRA, en cero y marcado — no se descarta.
//
// Hacerlo desaparecer sería peor: el contrato existe y su deuda existe. Cero significa «sin cuota
// válida», el motivo conserva el valor original para poder corregirlo, y `revision_pendiente`
// impide que alguien le genere cargos sin mirarlo.
func TestLaCuotaNegativaEntraEnCeroYMarcada(t *testing.T) {
	repo, empresaID := baseDeContratos(t)
	ctx := context.Background()

	_, err := repo.GuardarContratos(ctx, empresaID, []FilaContrato{{
		Numero: "CO139774", Cliente: "Cliente con cuota negativa",
		Cuota:   decimal.RequireFromString("-12500"),
		Motivos: []string{"cuota en cero o negativa"},
	}}, Resolucion{
		Sedes: map[string]string{}, Modalidades: map[string]string{},
		FormasPago: map[string]string{}, Asociaciones: map[string]string{},
	})
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	var cuota string
	var pendiente bool
	var motivo string
	if err := repo.pool.QueryRow(ctx,
		`SELECT cuota_vigente::text, revision_pendiente, COALESCE(revision_motivo,'')
		   FROM contrato_cxc WHERE empresa_id = $1::uuid AND numero = 'CO139774'`,
		empresaID).Scan(&cuota, &pendiente, &motivo); err != nil {
		t.Fatalf("leer el contrato: %v", err)
	}

	if decimal.RequireFromString(cuota).Sign() != 0 {
		t.Errorf("cuota guardada = %s, quería 0", cuota)
	}
	if !pendiente {
		t.Error("el contrato no quedó marcado para revisión: alguien podría generarle cargos a ciegas")
	}
	// El valor original tiene que estar en el motivo, o la corrección se hace a ciegas.
	if !contieneTexto(motivo, "-12500") {
		t.Errorf("el motivo no conserva la cuota original: %q", motivo)
	}
}

// Una cuota válida no se toca.
func TestLaCuotaBuenaSeGuardaTalCual(t *testing.T) {
	repo, empresaID := baseDeContratos(t)
	ctx := context.Background()

	_, err := repo.GuardarContratos(ctx, empresaID, []FilaContrato{{
		Numero: "CO000009", Cliente: "Cliente normal", Cuota: decimal.RequireFromString("18750.50"),
	}}, Resolucion{
		Sedes: map[string]string{}, Modalidades: map[string]string{},
		FormasPago: map[string]string{}, Asociaciones: map[string]string{},
	})
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	var cuota string
	var pendiente bool
	if err := repo.pool.QueryRow(ctx,
		`SELECT cuota_vigente::text, revision_pendiente FROM contrato_cxc
		  WHERE empresa_id = $1::uuid AND numero = 'CO000009'`, empresaID).Scan(&cuota, &pendiente); err != nil {
		t.Fatalf("leer: %v", err)
	}
	if decimal.RequireFromString(cuota).Cmp(decimal.RequireFromString("18750.50")) != 0 {
		t.Errorf("cuota = %s, quería 18750.50", cuota)
	}
	if pendiente {
		t.Error("un contrato sano quedó marcado para revisión")
	}
}

func contieneTexto(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
