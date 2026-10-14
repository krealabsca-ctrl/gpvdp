package cxc

// Rehacer los cargos NO puede borrar un peso cobrado (13-oct-2026).
//
// POR QUÉ ESTA PRUEBA ES LA QUE IMPORTA. `cobro_aplicacion` referencia al cargo con ON DELETE
// CASCADE: un DELETE de más no da error, borra la aplicación del cobro EN SILENCIO y el pago
// queda registrado contra nada. No hay forma de notarlo después salvo cuadrando a mano.
//
// Va contra Postgres de verdad y con `INCLUDING ALL` porque lo que se prueba es el WHERE del
// DELETE y el CASCADE de la FK. Un doble no tiene ninguna de las dos cosas.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var tablasRegenerar = []string{
	"contrato_cxc", "cargo_cxc", "cobro_cxc", "cobro_aplicacion",
	"nota_credito_cxc", "nota_credito_aplicacion",
}

func baseDeRegeneracion(t *testing.T) (*pgRepository, string) {
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
	esquema := fmt.Sprintf("prueba_regen_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+esquema); err != nil {
		admin.Close()
		t.Fatalf("crear esquema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+esquema+" CASCADE")
		admin.Close()
	})
	for _, tabla := range tablasRegenerar {
		if _, err := admin.Exec(ctx, fmt.Sprintf(
			"CREATE TABLE %s.%s (LIKE public.%s INCLUDING ALL)", esquema, tabla, tabla)); err != nil {
			t.Fatalf("copiar %s: %v", tabla, err)
		}
	}
	// La FK con CASCADE no la copia LIKE, y es justo lo que esta prueba vigila: se recrea a mano
	// para que el escenario sea el de producción y no uno más benigno.
	if _, err := admin.Exec(ctx, fmt.Sprintf(
		`ALTER TABLE %s.cobro_aplicacion
		   ADD CONSTRAINT cobro_aplicacion_cargo_fk FOREIGN KEY (cargo_id)
		   REFERENCES %s.cargo_cxc(id) ON DELETE CASCADE`, esquema, esquema)); err != nil {
		t.Fatalf("recrear la FK en cascada: %v", err)
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

// sembrarCargo crea un contrato con un cargo y devuelve el id del cargo.
func sembrarCargo(t *testing.T, r *pgRepository, empresaID, numero, periodo, origen string, aplicado string, conClave bool) string {
	t.Helper()
	ctx := context.Background()
	var contratoID string
	if err := r.pool.QueryRow(ctx,
		`INSERT INTO contrato_cxc (empresa_id, numero, cliente_nombre, cuota_vigente)
		 VALUES ($1::uuid,$2,'Cliente',25000) RETURNING id::text`, empresaID, numero).Scan(&contratoID); err != nil {
		t.Fatalf("sembrar contrato: %v", err)
	}
	clave := any(nil)
	if conClave {
		clave = "50601011500010000000100100001010000000011999999999"
	}
	var cargoID string
	if err := r.pool.QueryRow(ctx,
		`INSERT INTO cargo_cxc (empresa_id, contrato_id, periodo, vence_en, monto, monto_aplicado, origen, clave_hacienda)
		 VALUES ($1::uuid,$2::uuid,$3,'2026-10-15',25000,$4::numeric,$5,$6) RETURNING id::text`,
		empresaID, contratoID, periodo, aplicado, origen, clave).Scan(&cargoID); err != nil {
		t.Fatalf("sembrar cargo: %v", err)
	}
	return cargoID
}

// EL CARGO CON UN PAGO APLICADO NO SE BORRA, y la aplicación del cobro sobrevive.
func TestRehacerNoBorraUnCargoConPagoAplicado(t *testing.T) {
	repo, empresaID := baseDeRegeneracion(t)
	ctx := context.Background()

	libre := sembrarCargo(t, repo, empresaID, "CO000001", "2026-10", "GENERADO", "0", false)
	pagado := sembrarCargo(t, repo, empresaID, "CO000002", "2026-10", "GENERADO", "25000", false)

	// El cobro y su aplicación contra el cargo pagado: esto es lo que el CASCADE se llevaría.
	var cobroID string
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO cobro_cxc (empresa_id, consecutivo, fecha_pago, monto, origen)
		 VALUES ($1::uuid,'R-001','2026-10-10',25000,'ARCHIVO') RETURNING id::text`, empresaID).Scan(&cobroID); err != nil {
		t.Fatalf("sembrar cobro: %v", err)
	}
	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO cobro_aplicacion (empresa_id, cobro_id, cargo_id, monto)
		 VALUES ($1::uuid,$2::uuid,$3::uuid,25000)`, empresaID, cobroID, pagado); err != nil {
		t.Fatalf("sembrar aplicación: %v", err)
	}

	plan, err := repo.PlanDeRegeneracion(ctx, empresaID)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Borrables != 1 || plan.SeConservan != 1 {
		t.Fatalf("plan: borrables=%d conservan=%d, quería 1 y 1", plan.Borrables, plan.SeConservan)
	}

	n, err := repo.BorrarCargosRegenerables(ctx, empresaID)
	if err != nil {
		t.Fatalf("borrar: %v", err)
	}
	if n != 1 {
		t.Errorf("borrados = %d, quería 1", n)
	}

	// El cargo pagado sigue, y su aplicación también: el CASCADE no se la llevó.
	var quedan, aplicaciones int
	_ = repo.pool.QueryRow(ctx, `SELECT count(*) FROM cargo_cxc WHERE id = $1::uuid`, pagado).Scan(&quedan)
	_ = repo.pool.QueryRow(ctx, `SELECT count(*) FROM cobro_aplicacion WHERE cargo_id = $1::uuid`, pagado).Scan(&aplicaciones)
	if quedan != 1 {
		t.Error("se borró el cargo que tenía un pago aplicado")
	}
	if aplicaciones != 1 {
		t.Error("desapareció la aplicación del cobro: el pago quedó registrado contra nada")
	}
	// Y el libre sí salió.
	var libreQueda int
	_ = repo.pool.QueryRow(ctx, `SELECT count(*) FROM cargo_cxc WHERE id = $1::uuid`, libre).Scan(&libreQueda)
	if libreQueda != 0 {
		t.Error("el cargo sin pagos no se borró: la corrección no se haría")
	}
}

// Lo que no creó la generación no le toca borrarlo, y lo facturado tampoco.
func TestRehacerRespetaElOrigenYLaFactura(t *testing.T) {
	repo, empresaID := baseDeRegeneracion(t)
	ctx := context.Background()

	sembrarCargo(t, repo, empresaID, "CO000010", "2026-10", "GENERADO", "0", false)      // sí
	sembrarCargo(t, repo, empresaID, "CO000011", "2026-10", "SALDO_INICIAL", "0", false) // no: otro origen
	sembrarCargo(t, repo, empresaID, "CO000012", "2026-10", "GENERADO", "0", true)       // no: facturado

	plan, err := repo.PlanDeRegeneracion(ctx, empresaID)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Borrables != 1 {
		t.Errorf("borrables = %d, quería 1 (solo el GENERADO sin factura)", plan.Borrables)
	}
	if plan.SeConservan != 2 {
		t.Errorf("se conservan = %d, quería 2", plan.SeConservan)
	}

	n, _ := repo.BorrarCargosRegenerables(ctx, empresaID)
	if n != 1 {
		t.Errorf("borrados = %d, quería 1", n)
	}
}

// El plan y el DELETE miran lo MISMO.
//
// Si se separaran, el plan prometería un número y la operación haría otro — y en una pantalla que
// pide confirmar la cantidad exacta, eso es un bloqueo permanente o un borrado de más.
func TestElPlanYElBorradoCuentanIgual(t *testing.T) {
	repo, empresaID := baseDeRegeneracion(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		sembrarCargo(t, repo, empresaID, fmt.Sprintf("CO0001%02d", i), "2026-10", "GENERADO", "0", false)
	}
	sembrarCargo(t, repo, empresaID, "CO000199", "2026-10", "GENERADO", "500", false) // parcial: no

	plan, err := repo.PlanDeRegeneracion(ctx, empresaID)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	n, err := repo.BorrarCargosRegenerables(ctx, empresaID)
	if err != nil {
		t.Fatalf("borrar: %v", err)
	}
	if n != plan.Borrables {
		t.Errorf("el plan dijo %d y se borraron %d", plan.Borrables, n)
	}
}
