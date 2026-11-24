package conta

// El plan de cuentas contra Postgres (mig 0109).
//
// LO QUE SE PRUEBA ACÁ es lo que un doble no puede: la regla del árbol la hace cumplir un
// DISPARADOR, no el código de Go. Probarla contra un `fakeRepo` demostraría que mi función tiene
// un `if`, no que la base rechaza la fila. Y en un plan de cuentas eso es la diferencia entre un
// balance que cuadra y uno que suma el mismo colón dos veces.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	empUno    = "11111111-1111-4111-8111-111111111111"
	empDos    = "22222222-2222-4222-8222-222222222222"
	usrPrueba = "33333333-3333-4333-8333-333333333333"
)

var tablasDelPlan = []string{"empresa", "usuario", "cuenta_contable"}

// disparadorDelArbol: `INCLUDING ALL` NO copia los disparadores, y la regla del árbol vive en uno.
// Sin esto las pruebas pasarían contra una tabla que no la hace cumplir: verde en falso.
const disparadorDelArbol = `
	CREATE TRIGGER trg_cuenta_sin_hijas
	BEFORE INSERT OR UPDATE ON %s.cuenta_contable
	FOR EACH ROW EXECUTE FUNCTION public.fn_cuenta_sin_hijas()`

func basePlan(t *testing.T) *pgRepository {
	t.Helper()
	return baseConta(t, tablasDelPlan, []string{disparadorDelArbol})
}

// baseConta arma un esquema temporal con las tablas y los disparadores que la prueba necesita.
// `disparadores` son plantillas con un `%s` donde va el esquema.
func baseConta(t *testing.T, tablas, disparadores []string) *pgRepository {
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

	esquema := fmt.Sprintf("prueba_plan_%d", time.Now().UnixNano())
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
	for _, tabla := range tablas {
		if _, err := admin.Exec(ctx,
			fmt.Sprintf(`CREATE TABLE %s.%s (LIKE public.%s INCLUDING ALL)`, esquema, tabla, tabla)); err != nil {
			t.Fatalf("copiar %s: %v", tabla, err)
		}
	}
	for _, d := range disparadores {
		if _, err := admin.Exec(ctx, fmt.Sprintf(d, esquema)); err != nil {
			t.Fatalf("copiar disparador: %v", err)
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

	for _, q := range []string{
		`INSERT INTO empresa (id, nombre) VALUES ('` + empUno + `', 'Valle de Paz')`,
		`INSERT INTO empresa (id, nombre) VALUES ('` + empDos + `', 'Coopeprofa')`,
		`INSERT INTO usuario (id, nombre, email, password_hash)
		 VALUES ('` + usrPrueba + `', 'Contador', 'conta@prueba.local', 'x')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("sembrar: %v", err)
		}
	}
	return &pgRepository{pool: pool}
}

func cuenta(codigo, nombre string, empresa *string, acepta bool, padre string) Cuenta {
	clase, _ := claseDelCodigo(codigo)
	c := Cuenta{
		Codigo: codigo, Nombre: nombre, EmpresaID: empresa, Clase: clase,
		Naturaleza: naturalezaDeLaClase(clase), Nivel: nivelDelCodigo(codigo),
		AceptaMovimiento: acepta, Activo: true,
	}
	if padre != "" {
		p := padre
		c.PadreID = &p
	}
	return c
}

func idDe(s string) *string { return &s }

// LA REGLA DEL PLAN DE CUENTAS, hecha cumplir por la base y no por buena voluntad.
//
// Si una cuenta con hijas aceptara movimiento, al totalizar el árbol el mismo colón se contaría
// dos veces —una en la hija, otra en la madre— y ningún balance cuadraría. Es el defecto que no
// se nota hasta el cierre.
func TestLaBaseImpideQueUnaCuentaConHijasAcepteMovimiento(t *testing.T) {
	repo := basePlan(t)
	ctx := context.Background()

	madre, err := repo.CrearCuenta(ctx, cuenta("1-01-00-000-000", "ACTIVOS CORRIENTES", idDe(empUno), false, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear madre: %v", err)
	}
	if _, err := repo.CrearCuenta(ctx, cuenta("1-01-01-000-000", "Cajas Chicas", idDe(empUno), true, madre.Codigo), usrPrueba); err != nil {
		t.Fatalf("crear hija: %v", err)
	}

	t.Run("marcar movimiento en una cuenta que ya tiene hijas", func(t *testing.T) {
		madre.AceptaMovimiento = true
		_, err := repo.ActualizarCuenta(ctx, empUno, madre)
		if err == nil {
			t.Fatal("la base ACEPTÓ marcar como de movimiento una cuenta con hijas")
		}
		if !strings.Contains(err.Error(), "dos veces") {
			t.Errorf("el error no explica el porqué: %v", err)
		}
	})

	t.Run("colgarle una hija a una cuenta que acepta movimiento", func(t *testing.T) {
		// El mismo error por el otro extremo. Sin esta mitad, el árbol se rompe igual y nadie lo
		// nota hasta que el balance no cuadra.
		hoja, err := repo.CrearCuenta(ctx, cuenta("2-01-00-000-000", "PASIVOS CORRIENTES", idDe(empUno), true, ""), usrPrueba)
		if err != nil {
			t.Fatalf("crear hoja: %v", err)
		}
		_, err = repo.CrearCuenta(ctx, cuenta("2-01-01-000-000", "Proveedores", idDe(empUno), true, hoja.Codigo), usrPrueba)
		if err == nil {
			t.Fatal("la base ACEPTÓ colgarle una hija a una cuenta de movimiento")
		}
		if !strings.Contains(err.Error(), "dos veces") {
			t.Errorf("el error no explica el porqué: %v", err)
		}
	})
}

// Una cuenta de OTRA empresa no se ve ni se toca. Una del GRUPO sí: para eso existe.
func TestElRecorteVeLasPropiasYLasDelGrupoPeroNoLasDeOtraEmpresa(t *testing.T) {
	repo := basePlan(t)
	ctx := context.Background()

	if _, err := repo.CrearCuenta(ctx, cuenta("1-00-00-000-000", "ACTIVOS (del grupo)", nil, true, ""), usrPrueba); err != nil {
		t.Fatalf("crear la del grupo: %v", err)
	}
	if _, err := repo.CrearCuenta(ctx, cuenta("2-00-00-000-000", "PASIVOS (mía)", idDe(empUno), true, ""), usrPrueba); err != nil {
		t.Fatalf("crear la propia: %v", err)
	}
	ajena, err := repo.CrearCuenta(ctx, cuenta("3-00-00-000-000", "PATRIMONIO (ajena)", idDe(empDos), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear la ajena: %v", err)
	}

	vistas, err := repo.Cuentas(ctx, empUno, false)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	nombres := map[string]bool{}
	for _, c := range vistas {
		nombres[c.Nombre] = true
	}
	if !nombres["ACTIVOS (del grupo)"] {
		t.Error("no se ve la cuenta del GRUPO; para eso existe el plan común")
	}
	if !nombres["PASIVOS (mía)"] {
		t.Error("no se ve la cuenta propia")
	}
	if nombres["PATRIMONIO (ajena)"] {
		t.Error("SE VE la cuenta de OTRA empresa")
	}

	// Ni se edita…
	ajena.Nombre = "Secuestrada"
	if _, err := repo.ActualizarCuenta(ctx, empUno, ajena); !errors.Is(err, ErrCuentaNoEncontrada) {
		t.Errorf("se pudo editar la cuenta de otra empresa: %v", err)
	}
	// …ni se borra.
	if err := repo.BorrarCuenta(ctx, empUno, ajena.ID); !errors.Is(err, ErrCuentaNoEncontrada) {
		t.Errorf("se pudo borrar la cuenta de otra empresa: %v", err)
	}
}

// Dos cuentas con el mismo código en la misma empresa no se pueden distinguir: el código ES la
// identidad de la cuenta, y es lo que va a viajar en cada asiento.
func TestElCodigoEsUnicoPorEmpresaYEntreLasDelGrupo(t *testing.T) {
	repo := basePlan(t)
	ctx := context.Background()

	if _, err := repo.CrearCuenta(ctx, cuenta("1-00-00-000-000", "ACTIVOS", idDe(empUno), true, ""), usrPrueba); err != nil {
		t.Fatalf("crear: %v", err)
	}
	if _, err := repo.CrearCuenta(ctx, cuenta("1-00-00-000-000", "ACTIVOS otra vez", idDe(empUno), true, ""), usrPrueba); !errors.Is(err, ErrCodigoDuplicado) {
		t.Errorf("se repitió el código en la misma empresa: %v", err)
	}
	// Otra empresa SÍ puede usar el mismo código: son planes distintos.
	if _, err := repo.CrearCuenta(ctx, cuenta("1-00-00-000-000", "ACTIVOS de la otra", idDe(empDos), true, ""), usrPrueba); err != nil {
		t.Errorf("la otra empresa no pudo usar el mismo código: %v", err)
	}
	// Y entre las del grupo el código también es único. Esto necesita un índice PARCIAL: un
	// UNIQUE normal trata cada NULL como distinto y dejaría pasar dos cuentas del grupo iguales.
	if _, err := repo.CrearCuenta(ctx, cuenta("9-00-00-000-000", "ORDEN", nil, true, ""), usrPrueba); err != nil {
		// 9 no es una clase válida; se usa 7.
		_ = err
	}
	if _, err := repo.CrearCuenta(ctx, cuenta("7-00-00-000-000", "ORDEN", nil, true, ""), usrPrueba); err != nil {
		t.Fatalf("crear del grupo: %v", err)
	}
	if _, err := repo.CrearCuenta(ctx, cuenta("7-00-00-000-000", "ORDEN repetida", nil, true, ""), usrPrueba); !errors.Is(err, ErrCodigoDuplicado) {
		t.Errorf("se repitió el código entre las cuentas del grupo: %v", err)
	}
}

// Importar es TODO O NADA. Un plan de cuentas a medias es peor que ninguno: faltarían ramas y
// nadie sabría cuáles sin comparar contra el Excel a mano.
func TestImportarEsTodoONada(t *testing.T) {
	repo := basePlan(t)
	ctx := context.Background()

	// La tercera cuenta cuelga de una madre que no existe: la importación entera debe fallar.
	plan := []Cuenta{
		cuenta("1-00-00-000-000", "ACTIVOS", nil, false, ""),
		cuenta("1-01-00-000-000", "ACTIVOS CORRIENTES", nil, true, "1-00-00-000-000"),
		cuenta("1-02-00-000-000", "ACTIVOS NO CORRIENTES", nil, true, "9-99-99-999-999"),
	}
	if _, err := repo.ImportarCuentas(ctx, idDe(empUno), plan, usrPrueba); err == nil {
		t.Fatal("la importación con una madre inexistente no falló")
	}
	quedaron, err := repo.Cuentas(ctx, empUno, false)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(quedaron) != 0 {
		t.Errorf("quedaron %d cuentas de una importación que falló: tiene que ser todo o nada", len(quedaron))
	}
}

// Reimportar el mismo archivo actualiza en vez de reventar. Es lo que hace falta mientras el
// contador corrige el catálogo y lo vuelve a subir.
func TestReimportarActualizaYNoDuplica(t *testing.T) {
	repo := basePlan(t)
	ctx := context.Background()

	plan := []Cuenta{
		cuenta("1-00-00-000-000", "ACTIVOS", nil, false, ""),
		cuenta("1-01-00-000-000", "ACTIVOS CORRIENTES", nil, true, "1-00-00-000-000"),
	}
	res, err := repo.ImportarCuentas(ctx, idDe(empUno), plan, usrPrueba)
	if err != nil {
		t.Fatalf("primera importación: %v", err)
	}
	if res.Creadas != 2 || res.Actualizadas != 0 {
		t.Fatalf("primera: creadas %d, actualizadas %d", res.Creadas, res.Actualizadas)
	}

	plan[1].Nombre = "ACTIVOS CORRIENTES (corregido)"
	res2, err := repo.ImportarCuentas(ctx, idDe(empUno), plan, usrPrueba)
	if err != nil {
		t.Fatalf("segunda importación: %v", err)
	}
	if res2.Creadas != 0 || res2.Actualizadas != 2 {
		t.Errorf("segunda: creadas %d, actualizadas %d; se esperaba 0 y 2", res2.Creadas, res2.Actualizadas)
	}
	c, err := repo.CuentaPorCodigo(ctx, empUno, "1-01-00-000-000")
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if c.Nombre != "ACTIVOS CORRIENTES (corregido)" {
		t.Errorf("el nombre no se actualizó: %q", c.Nombre)
	}
}
