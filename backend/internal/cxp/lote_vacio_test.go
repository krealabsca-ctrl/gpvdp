package cxp

// Un lote de pago NUNCA se crea vacío, y lo que no entró se dice (25-set-2026).
//
// El Director cortó un lote, la pantalla le dejó seleccionar las facturas, y el lote #17 salió con
// CERO y la macro se bajó vacía. La causa eran dos filtros encadenados que no decían nada:
// `ProgramarAprobados` solo toca lo APROBADO y no bloqueado, y el UPDATE que asigna el lote solo
// toca lo PROGRAMADO y sin lote. Lo que no calzaba desaparecía, el lote se creaba igual y el .txt
// salía en blanco — que es el peor final posible, porque un archivo vacío se sube al banco lo mismo
// que uno bueno y no paga a nadie.
//
// Estas pruebas corren contra Postgres de verdad en un esquema temporal (nunca escriben en public):
// lo que hay que proteger es el SQL, y un doble del repositorio no probaría nada.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Las tablas que tocan CrearLote, motivosFueraDelLote y ListarDocumentos (ver documentoFrom),
// en orden de creación.
var tablasDelLote = []string{
	"empresa", "usuario", "proveedor", "lote_pago", "concepto", "clasificacion",
	"subclasificacion", "departamento", "documento_cxp", "anticipo_aplicacion",
	"comprobante_pago", "cxp_recepcion",
}

const (
	empresaLote = "11111111-1111-4111-8111-111111111111"
	otraEmpLote = "22222222-2222-4222-8222-222222222222"
	usuarioLote = "33333333-3333-4333-8333-333333333333"
	provLote    = "44444444-4444-4444-8444-444444444444"
)

func baseDeLotes(t *testing.T) *pgRepository {
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

	esquema := fmt.Sprintf("prueba_lote_%d", time.Now().UnixNano())
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
	for _, tabla := range tablasDelLote {
		if _, err := admin.Exec(ctx,
			fmt.Sprintf(`CREATE TABLE %s.%s (LIKE public.%s INCLUDING ALL)`, esquema, tabla, tabla)); err != nil {
			t.Fatalf("copiar %s: %v", tabla, err)
		}
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	// El pool ve SOLO el esquema temporal: si faltara una tabla, la consulta falla en vez de leer
	// los datos reales sin que nadie se entere.
	cfg.ConnConfig.RuntimeParams["search_path"] = esquema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	repo := &pgRepository{pool: pool}
	for _, q := range []string{
		`INSERT INTO empresa (id, nombre) VALUES ('` + empresaLote + `', 'Valle de Paz')`,
		`INSERT INTO empresa (id, nombre) VALUES ('` + otraEmpLote + `', 'Coopeprofa')`,
		`INSERT INTO usuario (id, nombre, email, password_hash)
		 VALUES ('` + usuarioLote + `', 'Tesorería', 'teso@prueba.local', 'x')`,
		`INSERT INTO proveedor (id, empresa_id, nombre, identificacion, iban)
		 VALUES ('` + provLote + `', '` + empresaLote + `', 'Proveedor Uno', '3101', 'CR0000000000000000001')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("sembrar catálogo: %v", err)
		}
	}
	return repo
}

// factura siembra un documento y devuelve su id.
func factura(t *testing.T, repo *pgRepository, nombre, estado string, total string, bloqueada bool, empresa string) string {
	t.Helper()
	var id string
	err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, total, total_crc,
			 moneda, bloqueado_para_pago, bloqueo_motivo)
		VALUES ($1::uuid, $2::uuid, $3, $3, '2026-08-01'::date, $4, $5, $5, 'CRC', $6, $7)
		RETURNING id::text`,
		empresa, provLote, nombre, estado, total, bloqueada,
		map[bool]string{true: "duplicada de la 4411", false: ""}[bloqueada]).Scan(&id)
	if err != nil {
		t.Fatalf("sembrar factura %s: %v", nombre, err)
	}
	return id
}

func TestNoSeCreaUnLoteVacio(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	// El escenario del lote #17: se seleccionan facturas que NINGUNA puede cortarse.
	bloqueada := factura(t, repo, "F-BLOQ", "APROBADO", "50000", true, empresaLote)
	enRevision := factura(t, repo, "F-REV", "REVISADO", "60000", false, empresaLote)
	ajena := factura(t, repo, "F-AJENA", "APROBADO", "70000", false, otraEmpLote)

	_, err := repo.CrearLote(ctx, empresaLote, "2026-08-07",
		[]string{bloqueada, enRevision, ajena}, usuarioLote)

	var vacio *LoteVacioError
	if !asLoteVacio(err, &vacio) {
		t.Fatalf("esperaba LoteVacioError, obtuve %v", err)
	}

	// Lo que de verdad importa: NO quedó un lote fantasma en la base.
	var lotes int
	if err := repo.pool.QueryRow(ctx, `SELECT count(*) FROM lote_pago`).Scan(&lotes); err != nil {
		t.Fatalf("contar lotes: %v", err)
	}
	if lotes != 0 {
		t.Fatalf("quedaron %d lote(s): el INSERT tenía que revertirse con la transacción", lotes)
	}

	// Y cada factura trae SU razón, que es lo que la pantalla necesita para poder actuar.
	porID := map[string]string{}
	for _, f := range vacio.Fuera {
		porID[f.DocumentoID] = f.Motivo
	}
	casos := []struct{ id, contiene, porque string }{
		{bloqueada, "bloqueada para pago", "hay que desbloquearla o sacarla de la selección"},
		{enRevision, "está en REVISADO", "todavía no se aprobó"},
		{ajena, "no existe en esta empresa", "para esta empresa ese id no existe: nunca se nombra la otra"},
	}
	for _, c := range casos {
		if !strings.Contains(porID[c.id], c.contiene) {
			t.Errorf("motivo = %q, se esperaba que dijera %q (%s)", porID[c.id], c.contiene, c.porque)
		}
	}
	// El motivo del bloqueo viaja: sin él, «está bloqueada» no dice qué hacer.
	if !strings.Contains(porID[bloqueada], "duplicada de la 4411") {
		t.Errorf("el motivo del bloqueo no llegó: %q", porID[bloqueada])
	}
}

func TestLoteParcialDiceQueQuedoAfuera(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	buena := factura(t, repo, "F-BUENA", "APROBADO", "100000", false, empresaLote)
	bloqueada := factura(t, repo, "F-BLOQ", "APROBADO", "50000", true, empresaLote)

	// El servicio programa las aprobadas antes de cortar; acá se hace lo mismo que él.
	if _, err := repo.ProgramarAprobados(ctx, empresaLote, []string{buena, bloqueada}, "2026-08-07"); err != nil {
		t.Fatalf("programar: %v", err)
	}
	lote, err := repo.CrearLote(ctx, empresaLote, "2026-08-07", []string{buena, bloqueada}, usuarioLote)
	if err != nil {
		t.Fatalf("no esperaba error con una factura buena: %v", err)
	}
	if lote.Cantidad != 1 {
		t.Fatalf("cantidad = %d, se esperaba 1: la bloqueada no entra y la buena sí", lote.Cantidad)
	}
	if len(lote.Fuera) != 1 || lote.Fuera[0].DocumentoID != bloqueada {
		t.Fatalf("fuera = %+v, se esperaba solo la bloqueada", lote.Fuera)
	}
	if !strings.Contains(lote.Fuera[0].Motivo, "bloqueada para pago") {
		t.Errorf("motivo = %q", lote.Fuera[0].Motivo)
	}
	// Control: la que entró NO figura como afuera.
	for _, f := range lote.Fuera {
		if f.DocumentoID == buena {
			t.Error("la factura que entró al lote se reportó como afuera")
		}
	}

	t.Run("cortar dos veces la misma factura dice en qué lote está", func(t *testing.T) {
		otra := factura(t, repo, "F-OTRA", "APROBADO", "80000", false, empresaLote)
		if _, err := repo.ProgramarAprobados(ctx, empresaLote, []string{otra}, "2026-08-08"); err != nil {
			t.Fatalf("programar: %v", err)
		}
		// `buena` ya está en el lote de arriba: se pide junto a una nueva.
		l2, err := repo.CrearLote(ctx, empresaLote, "2026-08-08", []string{buena, otra}, usuarioLote)
		if err != nil {
			t.Fatalf("no esperaba error: %v", err)
		}
		if l2.Cantidad != 1 || len(l2.Fuera) != 1 {
			t.Fatalf("cantidad = %d, fuera = %+v", l2.Cantidad, l2.Fuera)
		}
		esperado := fmt.Sprintf("ya está en el lote #%d", lote.Numero)
		if l2.Fuera[0].Motivo != esperado {
			t.Errorf("motivo = %q, se esperaba %q: sin el número no se sabe dónde buscarla",
				l2.Fuera[0].Motivo, esperado)
		}
	})
}

func TestUnLoteNormalNoReportaNadaAfuera(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	a := factura(t, repo, "F-A", "APROBADO", "100000", false, empresaLote)
	b := factura(t, repo, "F-B", "APROBADO", "250000", false, empresaLote)
	if _, err := repo.ProgramarAprobados(ctx, empresaLote, []string{a, b}, "2026-08-07"); err != nil {
		t.Fatalf("programar: %v", err)
	}

	lote, err := repo.CrearLote(ctx, empresaLote, "2026-08-07", []string{a, b}, usuarioLote)
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if lote.Cantidad != 2 || len(lote.Fuera) != 0 {
		t.Fatalf("cantidad = %d, fuera = %+v: el camino feliz no puede reportar nada", lote.Cantidad, lote.Fuera)
	}
	if lote.TotalCRC != "350000" && lote.TotalCRC != "350000.00" {
		t.Errorf("total = %q, se esperaban 350000", lote.TotalCRC)
	}

	t.Run("y la macro trae las dos líneas", func(t *testing.T) {
		rows, err := repo.DocumentosParaPagoPorLote(ctx, empresaLote, lote.ID)
		if err != nil {
			t.Fatalf("macro: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("líneas = %d, se esperaban 2", len(rows))
		}
	})
}

// asLoteVacio evita importar "errors" solo para esto en un archivo de pruebas.
func asLoteVacio(err error, dst **LoteVacioError) bool {
	for err != nil {
		if e, ok := err.(*LoteVacioError); ok {
			*dst = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
