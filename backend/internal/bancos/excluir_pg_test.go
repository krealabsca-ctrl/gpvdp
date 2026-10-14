package bancos

// Sacar UN movimiento de los libros, contra PostgreSQL de verdad (9-oct-2026).
//
// «El BAC por alguna razón me cambió el consecutivo, pero veo que los últimos 4 dígitos conservan
// la base aunque la descripción no, pero el monto y la fecha sí. El detalle es que cuando se cargó
// ya no puedo excluir el movimiento y se malogró la data, ya no es fidedigna.»
//
// Dos cosas hay que proteger acá, y ninguna la puede demostrar un doble:
//
//  1. Que la exclusión a mano y la reversa de una carga NO se pisen. Si deshacer una reversa
//     devolviera a los libros un duplicado que alguien sacó a mano, la plata vuelve a estar
//     contada dos veces y nadie se entera.
//  2. Que el detector no proponga movimientos legítimos. Probada la regla «mismos cuatro dígitos»
//     a secas sobre los datos reales, devolvió 84 pares que eran comisiones y SINPE repetidos el
//     mismo día: excluirlos habría sacado plata real.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	empExc    = "d1d1d1d1-e1e1-4f1f-a1a1-b1b1b1b1b1b1"
	bancoExc  = "d2d2d2d2-e2e2-4f2f-a2a2-b2b2b2b2b2b2"
	cuentaExc = "d3d3d3d3-e3e3-4f3f-a3a3-b3b3b3b3b3b3"
	impExc    = "d4d4d4d4-e4e4-4f4f-a4a4-b4b4b4b4b4b4"
	usrExc    = "d5d5d5d5-e5e5-4f5f-a5a5-b5b5b5b5b5b5"
)

var tablasExcluir = []string{
	"empresa", "usuario", "banco", "cuenta_bancaria", "importacion", "movimiento_bancario",
	// La división manda la parte apartada a otra partida, así que hace falta el catálogo.
	"concepto", "clasificacion",
	// La huella CxP→Bancos trae el gasto de lo que se pagó (13-oct-2026), así que la lista de
	// movimientos toca las tablas de CxP. Sin ellas el esquema temporal da «relation does not
	// exist», que es lo correcto: el pool solo ve este esquema.
	"proveedor", "lote_pago", "remesa_pago", "documento_cxp",
}

func baseDeExclusion(t *testing.T) *pgRepository {
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
	esquema := fmt.Sprintf("prueba_exc_%d", time.Now().UnixNano())
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
	for _, tabla := range tablasExcluir {
		if _, err := admin.Exec(ctx,
			fmt.Sprintf(`CREATE TABLE %s.%s (LIKE public.%s INCLUDING ALL)`, esquema, tabla, tabla)); err != nil {
			t.Fatalf("copiar %s: %v", tabla, err)
		}
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

	repo := &pgRepository{pool: pool}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	exec(`INSERT INTO empresa (id, nombre) VALUES ($1::uuid,'Valle de Paz')`, empExc)
	exec(`INSERT INTO usuario (id, nombre, email, password_hash) VALUES ($1::uuid,'Tesorería','teso.exc@vdp.local','x')`, usrExc)
	exec(`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid,$2::uuid,'BAC')`, bancoExc, empExc)
	exec(`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
	      VALUES ($1::uuid,$2::uuid,$3::uuid,'CRC','BAC Religiosa')`, cuentaExc, empExc, bancoExc)
	exec(`INSERT INTO importacion (id, empresa_id, cuenta_bancaria_id, source_file_hash,
	        nombre_archivo, estado)
	      VALUES ($1::uuid,$2::uuid,$3::uuid,'hash-de-prueba','bac_setiembre.xlsx','CONFIRMADA')`,
		impExc, empExc, cuentaExc)
	return repo
}

// sembrarMov devuelve el id del movimiento creado.
//
// `natural_key` se arma con el consecutivo, igual que el importador: es la llave anti-duplicado, y
// que el BAC le cambiara el formato al consecutivo es exactamente por lo que el mismo depósito
// pudo entrar dos veces sin que nada lo frenara.
func sembrarMov(t *testing.T, r *pgRepository, fecha, documento, descripcion string, credito float64) string {
	t.Helper()
	var id string
	llave := fmt.Sprintf("%s|%s|%.2f", fecha, documento, credito)
	err := r.pool.QueryRow(context.Background(), `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, importacion_id, fecha, documento, descripcion,
			 debito, credito, monto_crc, incluido, natural_key)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4::date,$5,$6,0,$7::numeric,$7::numeric,true,$8)
		RETURNING id::text`,
		empExc, cuentaExc, impExc, fecha, documento, descripcion, credito, llave).Scan(&id)
	if err != nil {
		t.Fatalf("sembrar movimiento: %v", err)
	}
	return id
}

func estaIncluido(t *testing.T, r *pgRepository, id string) bool {
	t.Helper()
	var inc bool
	if err := r.pool.QueryRow(context.Background(),
		`SELECT incluido FROM movimiento_bancario WHERE id = $1::uuid`, id).Scan(&inc); err != nil {
		t.Fatalf("leer incluido: %v", err)
	}
	return inc
}

// El camino entero: sacar, verlo en la lista con su rastro, y devolverlo.
func TestExcluirYVolverAIncluirUnMovimiento(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	id := sembrarMov(t, repo, "2026-09-18", "220007293", "DEP_ATM_207710067 ALA", 70000)

	if err := repo.ExcluirMovimiento(ctx, empExc, id, "duplicado: el BAC lo cargó dos veces", usrExc); err != nil {
		t.Fatalf("excluir: %v", err)
	}
	if estaIncluido(t, repo, id) {
		t.Fatal("el movimiento sigue en los libros después de excluirlo")
	}

	// La lista tiene que poder mostrar QUIÉN y POR QUÉ: plata fuera de los libros sin rastro es
	// indistinguible de un error del sistema.
	lista, err := repo.ExcluidosAMano(ctx, empExc)
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) != 1 {
		t.Fatalf("la lista de excluidos trae %d, se esperaba 1", len(lista))
	}
	if lista[0].Motivo == "" {
		t.Error("el excluido no conserva el motivo")
	}
	if lista[0].ExcluidoPor != "Tesorería" {
		t.Errorf("no dice quién lo sacó (%q)", lista[0].ExcluidoPor)
	}
	if lista[0].Banco != "BAC" || lista[0].Cuenta != "BAC Religiosa" {
		t.Errorf("no dice de qué cuenta salió (%q · %q)", lista[0].Banco, lista[0].Cuenta)
	}

	// Sacarlo dos veces no corresponde: ya no está.
	if err := repo.ExcluirMovimiento(ctx, empExc, id, "otra vez", usrExc); err != ErrMovimientoNoExcluible {
		t.Errorf("excluir dos veces devolvió %v", err)
	}

	if err := repo.IncluirMovimiento(ctx, empExc, id); err != nil {
		t.Fatalf("incluir: %v", err)
	}
	if !estaIncluido(t, repo, id) {
		t.Fatal("el movimiento no volvió a los libros")
	}
	// Y al volver, el rastro se limpia: un movimiento en los libros no puede quedar con un motivo
	// de exclusión colgando, o la lista lo mostraría para siempre.
	lista, _ = repo.ExcluidosAMano(ctx, empExc)
	if len(lista) != 0 {
		t.Errorf("sigue apareciendo como excluido (%d)", len(lista))
	}
}

// LO QUE MÁS IMPORTA: la exclusión a mano y la reversa de una carga no se pisan.
//
// Si deshacer una reversa devolviera un duplicado sacado a mano, la plata vuelve a contarse dos
// veces y nadie se entera: el movimiento estaría en los libros y nadie lo sacó.
func TestLaReversaNoDevuelveLoQueSeSacoAMano(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	duplicado := sembrarMov(t, repo, "2026-09-18", "220007293", "DEP_ATM_207710067 ALA", 70000)
	normal := sembrarMov(t, repo, "2026-09-19", "220007400", "DEP_ATM otro", 15000)

	// Alguien saca el duplicado a mano.
	if err := repo.ExcluirMovimiento(ctx, empExc, duplicado, "duplicado del BAC", usrExc); err != nil {
		t.Fatal(err)
	}
	// Después se revierte la carga entera… y se deshace.
	if _, err := repo.RevertirImportacion(ctx, empExc, impExc, usrExc, "se cargó el mes equivocado"); err != nil {
		t.Fatalf("revertir: %v", err)
	}
	if _, err := repo.DeshacerReversaImportacion(ctx, empExc, impExc); err != nil {
		t.Fatalf("deshacer reversa: %v", err)
	}

	if estaIncluido(t, repo, duplicado) {
		t.Error("deshacer la reversa devolvió a los libros un duplicado que alguien había sacado " +
			"a mano: la plata vuelve a estar contada dos veces y nadie se entera")
	}
	if !estaIncluido(t, repo, normal) {
		t.Error("deshacer la reversa no devolvió el movimiento normal")
	}
}

// Lo que salió con la reversa NO se devuelve desde acá: volvería a los libros mientras su
// importación sigue marcada como revertida, y el libro tendría una fila sin respaldo.
func TestNoSeIncluyeAManoLoQueSacoLaReversa(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	id := sembrarMov(t, repo, "2026-09-18", "220007293", "DEP_ATM", 70000)
	if _, err := repo.RevertirImportacion(ctx, empExc, impExc, usrExc, "carga equivocada"); err != nil {
		t.Fatal(err)
	}
	if err := repo.IncluirMovimiento(ctx, empExc, id); err != ErrMovimientoNoIncluible {
		t.Fatalf("se incluyó a mano un movimiento que sacó la reversa (err=%v)", err)
	}
	// Y tampoco figura en la lista de excluidos A MANO: su motivo vive en la importación.
	lista, _ := repo.ExcluidosAMano(ctx, empExc)
	if len(lista) != 0 {
		t.Errorf("un excluido por reversa aparece en la lista de exclusiones a mano (%d)", len(lista))
	}
}

// El caso real del BAC: mismo día, mismo monto, mismos cuatro dígitos finales, pero el consecutivo
// cambió de largo y la descripción es otra. Las TRES señales.
func TestElDuplicadoDelBacSaleConLasTresSenales(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	sembrarMov(t, repo, "2026-09-18", "82007293", "DEPOSITO CAJERO AUTOMATICO CRC", 70000)
	sembrarMov(t, repo, "2026-09-18", "220007293", "DEP_ATM_207710067 ALA", 70000)

	pares, err := repo.DuplicadosSospechosos(ctx, empExc, "2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(pares) != 1 {
		t.Fatalf("el detector encontró %d pares, se esperaba 1", len(pares))
	}
	p := pares[0]
	if !p.MismosUltimos4 {
		t.Error("no detectó que los últimos cuatro dígitos coinciden (7293)")
	}
	if !p.LargoDistinto {
		t.Error("no detectó que el consecutivo cambió de largo (8 vs 9 dígitos)")
	}
	if !p.DescripcionDistinta {
		t.Error("no detectó que la descripción cambió")
	}
	ContarSenales(&p)
	if p.Senales != 3 {
		t.Errorf("el par tiene %d señales, se esperaban 3", p.Senales)
	}
	if p.Monto != "70000.00" {
		t.Errorf("monto %q", p.Monto)
	}
}

// Y LO CONTRARIO, que es lo que evita sacar plata real: dos comisiones legítimas del mismo día,
// mismo monto y con los mismos cuatro dígitos por casualidad NO pueden parecer un duplicado.
//
// Está medido sobre los datos reales: la regla de los cuatro dígitos a secas devolvía 84 pares así.
func TestDosComisionesLegitimasNoParecenDuplicado(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	// Mismo largo de consecutivo y MISMA descripción: dos cobros reales del mismo día.
	sembrarMov(t, repo, "2026-09-04", "425519637", "COMISION POR TRANSACCION SINPE", 229.50)
	sembrarMov(t, repo, "2026-09-04", "591009637", "COMISION POR TRANSACCION SINPE", 229.50)

	pares, err := repo.DuplicadosSospechosos(ctx, empExc, "2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(pares) != 1 {
		t.Fatalf("el detector devolvió %d pares", len(pares))
	}
	p := pares[0]
	ContarSenales(&p)
	if p.Senales == 3 {
		t.Fatal("dos comisiones legítimas salieron con las tres señales: excluirlas sacaría " +
			"plata real de los libros")
	}
	// La única señal que cumplen es la de los cuatro dígitos, y por eso sola no alcanza.
	if !p.MismosUltimos4 || p.LargoDistinto || p.DescripcionDistinta {
		t.Errorf("señales inesperadas: 4dig=%v largo=%v desc=%v",
			p.MismosUltimos4, p.LargoDistinto, p.DescripcionDistinta)
	}
}

// Un movimiento ya excluido no vuelve a proponerse: el par se resuelve de una vez.
func TestLoYaExcluidoNoSeVuelveAProponer(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	sembrarMov(t, repo, "2026-09-18", "82007293", "DEPOSITO CAJERO AUTOMATICO CRC", 70000)
	dup := sembrarMov(t, repo, "2026-09-18", "220007293", "DEP_ATM_207710067 ALA", 70000)

	if err := repo.ExcluirMovimiento(ctx, empExc, dup, "duplicado del BAC", usrExc); err != nil {
		t.Fatal(err)
	}
	pares, err := repo.DuplicadosSospechosos(ctx, empExc, "2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(pares) != 0 {
		t.Errorf("el par resuelto se sigue proponiendo (%d)", len(pares))
	}
}

// El aislamiento por empresa: no se puede sacar de los libros un movimiento ajeno.
func TestNoSeExcluyeUnMovimientoDeOtraEmpresa(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	id := sembrarMov(t, repo, "2026-09-18", "220007293", "DEP_ATM", 70000)
	otra := "99999999-9999-4999-9999-999999999999"
	if err := repo.ExcluirMovimiento(ctx, otra, id, "intento de otra empresa", usrExc); err != ErrMovimientoNoExcluible {
		t.Fatalf("se excluyó un movimiento desde otra empresa (err=%v)", err)
	}
	if !estaIncluido(t, repo, id) {
		t.Fatal("el movimiento salió de los libros desde otra empresa")
	}
}
