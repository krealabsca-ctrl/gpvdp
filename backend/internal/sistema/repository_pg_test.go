package sistema

// El registro de errores contra Postgres (mig 0107).
//
// LA PIEZA QUE SE PRUEBA ACÁ ES EL RECORTE POR EMPRESA. Esta tabla guarda rutas internas, nombres
// de columnas y fragmentos de SQL: es, de lejos, lo más revelador que el sistema almacena. Que la
// pantalla de una empresa muestre los errores de otra sería a la vez una fuga de datos y una
// descripción del sistema por dentro.
//
// Y el caso contrario, que es el que se escapa al escribir la consulta: un error en el LOGIN
// ocurre ANTES de que exista empresa. Si el filtro fuera `empresa_id = $1` a secas, esos errores
// no se verían NUNCA desde ninguna empresa — y son justo los que impiden entrar al sistema.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const (
	empresaUno = "11111111-1111-4111-8111-111111111111"
	empresaDos = "22222222-2222-4222-8222-222222222222"
	usuarioUno = "33333333-3333-4333-8333-333333333333"
)

var tablasDeIncidentes = []string{"empresa", "usuario", "incidente"}

// baseDeIncidentes levanta un esquema temporal con las tablas copiadas de `public`.
func baseDeIncidentes(t *testing.T) *pgRepository {
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

	esquema := fmt.Sprintf("prueba_incidente_%d", time.Now().UnixNano())
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
	for _, tabla := range tablasDeIncidentes {
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
		`INSERT INTO empresa (id, nombre) VALUES ('` + empresaUno + `', 'Valle de Paz')`,
		`INSERT INTO empresa (id, nombre) VALUES ('` + empresaDos + `', 'Coopeprofa')`,
		`INSERT INTO usuario (id, nombre, email, password_hash)
		 VALUES ('` + usuarioUno + `', 'Tesorería', 'teso@prueba.local', 'x')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("sembrar: %v", err)
		}
	}
	return repo
}

func guardar(t *testing.T, repo *pgRepository, in Incidente) {
	t.Helper()
	if err := repo.Registrar(context.Background(), in); err != nil {
		t.Fatalf("registrar: %v", err)
	}
}

func idDe(s string) *string { return &s }

// Lo que una empresa ve, y lo que no.
func TestCadaEmpresaVeSusErroresYLosQueNoTienenEmpresa(t *testing.T) {
	repo := baseDeIncidentes(t)
	ctx := context.Background()

	guardar(t, repo, Incidente{Referencia: "AAA111", EmpresaID: idDe(empresaUno),
		Metodo: "POST", Ruta: "/v1/cxp/documentos", Estado: 500, Detalle: "mio"})
	guardar(t, repo, Incidente{Referencia: "BBB222", EmpresaID: idDe(empresaDos),
		Metodo: "GET", Ruta: "/v1/bancos/movimientos", Estado: 500, Detalle: "de la otra empresa"})
	// El error del login: ocurre antes de que exista empresa.
	guardar(t, repo, Incidente{Referencia: "CCC333", EmpresaID: nil,
		Metodo: "POST", Ruta: "/v1/auth/login", Estado: 500, Detalle: "sin empresa todavía"})

	items, err := repo.Listar(ctx, empresaUno, FiltrosIncidentes{Limite: 50})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	vistas := map[string]bool{}
	for _, it := range items {
		vistas[it.Referencia] = true
	}
	if !vistas["AAA111"] {
		t.Error("no se ve el error de la propia empresa")
	}
	if vistas["BBB222"] {
		t.Error("SE VE el error de OTRA empresa: fuga de datos y descripción del sistema por dentro")
	}
	if !vistas["CCC333"] {
		t.Error("no se ve el error sin empresa: es el del login, justo el que impide entrar")
	}
}

// La búsqueda por referencia es para lo que existe la tabla: el usuario dicta seis caracteres.
func TestSeEncuentraPorLaReferenciaQueElUsuarioDicta(t *testing.T) {
	repo := baseDeIncidentes(t)
	svc := NewService(repo, zap.NewNop())
	ctx := context.Background()

	guardar(t, repo, Incidente{Referencia: "3F8A2C", EmpresaID: idDe(empresaUno), UsuarioID: idDe(usuarioUno),
		Metodo: "PUT", Ruta: "/v1/cxp/proveedores/:id", Estado: 500,
		Operacion: "cxp actualizar-proveedor", Detalle: "expected 17 arguments, got 16"})
	guardar(t, repo, Incidente{Referencia: "999999", EmpresaID: idDe(empresaUno),
		Metodo: "GET", Ruta: "/v1/otra", Estado: 500})

	// Tal como lo teclea quien atiende: en minúscula y con espacios de más.
	items, err := svc.Listar(ctx, empresaUno, FiltrosIncidentes{Referencia: "  3f8a2c "})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("trajo %d, esperaba 1: la referencia se dicta y se teclea como venga", len(items))
	}
	if items[0].Detalle != "expected 17 arguments, got 16" {
		t.Errorf("detalle = %q: el canje tiene que devolver la causa exacta", items[0].Detalle)
	}
	// Y con el nombre de quien lo sufrió, para poder volver a preguntarle qué estaba haciendo.
	if items[0].Usuario != "Tesorería" {
		t.Errorf("usuario = %q, esperaba el nombre", items[0].Usuario)
	}
}

// Lo que NO es una referencia se responde como tal. Devolver una lista vacía se lee como «ese
// código no existe», y el código puede existir perfectamente: lo que está mal es lo que se tecleó.
func TestUnaReferenciaConOtraFormaSeRechaza(t *testing.T) {
	repo := baseDeIncidentes(t)
	svc := NewService(repo, zap.NewNop())

	for _, malo := range []string{"3F8A2", "3F8A2CC", "ZZZZZZ", "3F 8A2C"} {
		if _, err := svc.Listar(context.Background(), empresaUno, FiltrosIncidentes{Referencia: malo}); err == nil {
			t.Errorf("se aceptó %q como referencia", malo)
		}
	}
}

// Y la fecha mal escrita tampoco llega a Postgres.
//
// ESTO NO ES COSMÉTICA. Se descubrió provocándolo a propósito contra el sistema corriendo:
// `?desde=hola` reventaba en la base (22007), salía como 500 «error interno» y SE REGISTRABA como
// incidente. Es decir, la pantalla que existe para ver los errores del sistema se llenaba de
// errores propios cada vez que alguien tecleaba mal una fecha en su propio filtro.
func TestUnaFechaMalEscritaNoLlegaALaBase(t *testing.T) {
	repo := baseDeIncidentes(t)
	svc := NewService(repo, zap.NewNop())
	ctx := context.Background()

	for _, malo := range []string{"hola", "2026-13-01", "22/10/2026", "2026-10"} {
		if _, err := svc.Listar(ctx, empresaUno, FiltrosIncidentes{Desde: malo}); !errors.Is(err, ErrFechaInvalida) {
			t.Errorf("desde=%q devolvió %v, esperaba ErrFechaInvalida", malo, err)
		}
		if _, err := svc.Listar(ctx, empresaUno, FiltrosIncidentes{Hasta: malo}); !errors.Is(err, ErrFechaInvalida) {
			t.Errorf("hasta=%q devolvió %v, esperaba ErrFechaInvalida", malo, err)
		}
	}
	// Y la bien escrita sigue pasando.
	if _, err := svc.Listar(ctx, empresaUno, FiltrosIncidentes{Desde: "2026-10-01", Hasta: "2026-10-31"}); err != nil {
		t.Errorf("una fecha válida fue rechazada: %v", err)
	}
}

// El resumen cuenta sobre el MISMO conjunto que la lista. Si contara de más, la pantalla diría
// «3 errores» y mostraría uno, y nadie sabría cuál de los dos números creer.
func TestElResumenCuentaLoMismoQueSeVe(t *testing.T) {
	repo := baseDeIncidentes(t)
	ctx := context.Background()

	guardar(t, repo, Incidente{Referencia: "AAA111", EmpresaID: idDe(empresaUno), Metodo: "GET", Ruta: "/a", Estado: 500})
	guardar(t, repo, Incidente{Referencia: "BBB222", EmpresaID: idDe(empresaDos), Metodo: "GET", Ruta: "/b", Estado: 500})
	guardar(t, repo, Incidente{Referencia: "CCC333", EmpresaID: nil, Metodo: "GET", Ruta: "/c", Estado: 500, Panico: true})

	res, err := repo.Resumen(ctx, empresaUno)
	if err != nil {
		t.Fatalf("resumen: %v", err)
	}
	if res.UltimaHora != 2 {
		t.Errorf("última hora = %d, esperaba 2 (el propio y el sin empresa; NO el de la otra)", res.UltimaHora)
	}
	if res.Panicos != 1 {
		t.Errorf("panicos = %d, esperaba 1", res.Panicos)
	}
	if res.Ultimo == nil {
		t.Error("sin fecha del último, el contador no dice si esto está pasando ahora o fue la semana pasada")
	}
}

// El texto enorme se recorta ANTES de la base, y por runas: cortar a la mitad de una «ó» deja un
// byte suelto que Postgres rechaza, y el incidente que se quería guardar se perdería.
func TestElDetalleEnormeSeRecortaSinRomperElTexto(t *testing.T) {
	repo := baseDeIncidentes(t)
	svc := NewService(repo, zap.NewNop())
	ctx := context.Background()

	largo := ""
	for i := 0; i < 5000; i++ {
		largo += "ó"
	}
	svc.Registrar(ctx, Incidente{Referencia: "DDD444", EmpresaID: idDe(empresaUno),
		Metodo: "GET", Ruta: "/largo", Estado: 500, Detalle: largo})

	items, err := repo.Listar(ctx, empresaUno, FiltrosIncidentes{Limite: 10})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("el incidente con detalle enorme no se guardó (%d filas)", len(items))
	}
	if n := len([]rune(items[0].Detalle)); n > 4100 {
		t.Errorf("detalle de %d runas: no se recortó", n)
	}
}
