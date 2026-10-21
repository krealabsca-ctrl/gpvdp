package cxp

// La partida conciliatoria del tablero (16-oct-2026).
//
// El tablero leía PAGADO y CONCILIADO como lo mismo —los dos «ya salieron de la cartera»— y con eso
// desaparecía la cifra que un arqueo pide primero: cuánto declaramos haber pagado que el banco
// todavía no confirmó. Medido sobre los datos reales al escribir esto: 66 facturas, ₡3.023.476,84,
// y la más vieja con 96 días.
//
// Las dos lecturas de un saldo así son incompatibles —el pago no se ejecutó nunca, o se ejecutó y
// el reconocimiento por huella falló— y las dos exigen que alguien mire. Mientras la cifra no
// existió, ninguna de las dos se podía ver.
//
// Va contra Postgres de verdad porque lo que se protege ES el SQL.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var tablasPorConfirmar = []string{
	"empresa", "usuario", "proveedor", "concepto", "clasificacion", "subclasificacion",
	"departamento", "lote_pago", "remesa_pago", "documento_cxp", "anticipo_aplicacion",
	"comprobante_pago", "cxp_recepcion", "cxp_linea",
	"movimiento_bancario", "cuenta_bancaria", "banco",
	// El vale del fondo fijo (mig 0102): `documentoCols` lo consulta para el medio de pago.
	"caja_chica_fondo", "caja_chica_vale",
	// La antigüedad del pago sale del EVENTO de auditoría, no de `actualizado_en`.
	"auditoria_evento",
}

const (
	empPC  = "a1a1a1a1-b1b1-4c1c-8d1d-e1e1e1e1e1e1"
	usrPC  = "a2a2a2a2-b2b2-4c2c-8d2d-e2e2e2e2e2e2"
	provPC = "a3a3a3a3-b3b3-4c3c-8d3d-e3e3e3e3e3e3"
)

func baseDePorConfirmar(t *testing.T) *pgRepository {
	t.Helper()
	dsn := os.Getenv("GPVDP_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://gpvdp:localdev@127.0.0.1:5432/gpvdp?sslmode=disable"
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("sin base de datos (%v)", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("sin base de datos (%v)", err)
	}
	esquema := fmt.Sprintf("prueba_pconf_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+esquema); err != nil {
		admin.Close()
		t.Fatalf("crear esquema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+esquema+` CASCADE`); err != nil {
			t.Errorf("borrar esquema: %v", err)
		}
		admin.Close()
	})
	for _, tabla := range tablasPorConfirmar {
		if _, err := admin.Exec(ctx,
			fmt.Sprintf(`CREATE TABLE %s.%s (LIKE public.%s INCLUDING ALL)`, esquema, tabla, tabla)); err != nil {
			t.Fatalf("copiar %s: %v", tabla, err)
		}
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	// El pool ve SOLO el esquema temporal: una tabla olvidada falla en vez de leer la real.
	cfg.ConnConfig.RuntimeParams["search_path"] = esquema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	repo := &pgRepository{pool: pool}
	for _, q := range []string{
		`INSERT INTO empresa (id, nombre) VALUES ('` + empPC + `', 'Valle de Paz')`,
		`INSERT INTO usuario (id, nombre, email, password_hash)
		 VALUES ('` + usrPC + `', 'Tesorería', 'teso.pc@prueba.local', 'x')`,
		`INSERT INTO proveedor (id, empresa_id, nombre)
		 VALUES ('` + provPC + `', '` + empPC + `', 'VOLCANO 506 S.A')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("sembrar catálogo: %v", err)
		}
	}
	return repo
}

// sembrarFacturaPC crea una factura en el estado dado y, si diasAtras >= 0, le pone el evento de
// pago con esa antigüedad. diasAtras < 0 = sin evento (carga histórica).
func sembrarFacturaPC(t *testing.T, r *pgRepository, consecutivo, estado string, total float64, diasAtras int) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
			 total, total_crc, retencion)
		VALUES ($1::uuid,$2::uuid,$3,$3,'2026-07-01',$4,'CRC',$5::numeric,$5::numeric,0)
		RETURNING id::text`, empPC, provPC, consecutivo, estado, total).Scan(&id); err != nil {
		t.Fatalf("sembrar factura: %v", err)
	}
	if diasAtras >= 0 {
		if _, err := r.pool.Exec(ctx, `
			INSERT INTO auditoria_evento (empresa_id, entidad, entidad_id, accion, usuario_id, ts)
			VALUES ($1::uuid,'documento_cxp',$2::uuid,'PAGAR_DOCUMENTO',$3::uuid,
			        now() - make_interval(days => $4))`,
			empPC, id, usrPC, diasAtras); err != nil {
			t.Fatalf("sembrar evento de pago: %v", err)
		}
	}
	return id
}

func TestElTableroSeparaLoPagadoDeLoConfirmadoPorElBanco(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()

	// Declarada pagada hace 96 días y nunca confirmada: el caso que motivó esto.
	sembrarFacturaPC(t, repo, "001", EstPagado, 1_000_000, 96)
	// Pagada ayer: normal, el estado de cuenta todavía no llegó.
	sembrarFacturaPC(t, repo, "002", EstPagado, 500_000, 1)
	// Conciliada: el banco la confirmó. NO cuenta — es la mitad que sí cerró.
	sembrarFacturaPC(t, repo, "003", EstConciliado, 9_000_000, 30)
	// Abierta: no se declaró pagada, es cartera.
	sembrarFacturaPC(t, repo, "004", EstAprobado, 7_000_000, -1)

	var c CarteraCxP
	if err := repo.porConfirmarCxP(ctx, empPC, nil, &c); err != nil {
		t.Fatalf("porConfirmarCxP: %v", err)
	}

	if c.PorConfirmar.Cantidad != 2 {
		t.Errorf("cantidad = %d, se esperaba 2 (solo las PAGADO)", c.PorConfirmar.Cantidad)
	}
	// 1.000.000 + 500.000. Si contara las CONCILIADAS daría 10.500.000: la diferencia entre
	// «pagamos» y «el banco debitó» es justo lo que el tablero tiene que poder mostrar.
	if c.PorConfirmar.Monto != "1500000.00" {
		t.Errorf("monto = %q, se esperaba 1500000.00", c.PorConfirmar.Monto)
	}
	// La antigüedad es la del MÁS VIEJO: es lo que distingue «se pagó ayer» de un hallazgo.
	if c.DiasSinConfirmar != 96 {
		t.Errorf("días sin confirmar = %d, se esperaba 96", c.DiasSinConfirmar)
	}
}

// Una factura conciliada deja de contar. Es la prueba de que la cifra BAJA cuando el banco
// confirma: si no bajara, no mediría lo que dice medir y nadie podría usarla para cuadrar.
func TestConciliarUnaFacturaLaSacaDeLoPorConfirmar(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()
	id := sembrarFacturaPC(t, repo, "001", EstPagado, 1_000_000, 96)

	var antes CarteraCxP
	if err := repo.porConfirmarCxP(ctx, empPC, nil, &antes); err != nil {
		t.Fatal(err)
	}
	if antes.PorConfirmar.Cantidad != 1 {
		t.Fatalf("antes = %d, se esperaba 1", antes.PorConfirmar.Cantidad)
	}

	if _, err := repo.pool.Exec(ctx,
		`UPDATE documento_cxp SET estado = $2 WHERE id = $1::uuid`, id, EstConciliado); err != nil {
		t.Fatal(err)
	}

	var despues CarteraCxP
	if err := repo.porConfirmarCxP(ctx, empPC, nil, &despues); err != nil {
		t.Fatal(err)
	}
	if despues.PorConfirmar.Cantidad != 0 {
		t.Errorf("después de conciliar quedaron %d; tenía que quedar en cero", despues.PorConfirmar.Cantidad)
	}
	if despues.DiasSinConfirmar != 0 {
		t.Errorf("días = %d, tenía que volver a cero", despues.DiasSinConfirmar)
	}
}

// Una factura PAGADA sin evento (carga histórica) suma al monto pero no aporta antigüedad: no se
// le puede inventar una fecha de pago, y `actualizado_en` se reescribe con cualquier edición.
func TestUnaPagadaSinEventoSumaAlMontoPeroNoEnvejece(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()
	sembrarFacturaPC(t, repo, "001", EstPagado, 250_000, -1) // sin evento
	sembrarFacturaPC(t, repo, "002", EstPagado, 250_000, 10)

	var c CarteraCxP
	if err := repo.porConfirmarCxP(ctx, empPC, nil, &c); err != nil {
		t.Fatal(err)
	}
	if c.PorConfirmar.Cantidad != 2 || c.PorConfirmar.Monto != "500000.00" {
		t.Errorf("la importada tiene que sumar igual: %d por %q",
			c.PorConfirmar.Cantidad, c.PorConfirmar.Monto)
	}
	if c.DiasSinConfirmar != 10 {
		t.Errorf("días = %d, se esperaba 10 (la sin evento no aporta antigüedad)", c.DiasSinConfirmar)
	}
}

// Aislamiento por empresa: la partida conciliatoria de una empresa no puede incluir la de otra.
func TestLoPorConfirmarNoCruzaEmpresas(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()
	sembrarFacturaPC(t, repo, "001", EstPagado, 1_000_000, 96)

	otra := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	var c CarteraCxP
	if err := repo.porConfirmarCxP(ctx, otra, nil, &c); err != nil {
		t.Fatal(err)
	}
	if c.PorConfirmar.Cantidad != 0 {
		t.Errorf("desde otra empresa se vieron %d facturas", c.PorConfirmar.Cantidad)
	}
}
