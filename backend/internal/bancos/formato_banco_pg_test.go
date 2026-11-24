package bancos

// Los formatos de banco contra Postgres (mig 0108).
//
// Lo que se prueba acá es lo que un doble no puede: que las señales sobrevivan al viaje por JSON
// —si se pierden, el formato queda inerte y nadie entiende por qué su banco dejó de reconocerse—
// y que un formato de otra empresa no se vea ni se pueda tocar.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

const (
	empFormatoUno = "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"
	empFormatoDos = "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb"
	usrFormato    = "cccccccc-3333-4333-8333-cccccccccccc"
)

var tablasDeFormatos = []string{"empresa", "usuario", "formato_banco"}

func baseDeFormatos(t *testing.T) *pgRepository {
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

	esquema := fmt.Sprintf("prueba_formato_%d", time.Now().UnixNano())
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
	for _, tabla := range tablasDeFormatos {
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

	for _, q := range []string{
		`INSERT INTO empresa (id, nombre) VALUES ('` + empFormatoUno + `', 'Valle de Paz')`,
		`INSERT INTO empresa (id, nombre) VALUES ('` + empFormatoDos + `', 'Coopeprofa')`,
		`INSERT INTO usuario (id, nombre, email, password_hash)
		 VALUES ('` + usrFormato + `', 'Admin', 'admin@prueba.local', 'x')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("sembrar: %v", err)
		}
	}
	return &pgRepository{pool: pool}
}

// Las señales son lo único que viaja como JSON, y son lo que decide si el archivo se reconoce.
// Si se pierden en el camino, el formato queda guardado y no detecta NADA.
func TestLasSenalesSobrevivenAlViajePorLaBase(t *testing.T) {
	repo := baseDeFormatos(t)
	ctx := context.Background()

	guardado, err := repo.GuardarFormatoBanco(ctx, empFormatoUno, formatoLafise(), usrFormato)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if guardado.ID == "" {
		t.Fatal("no devolvió el id")
	}

	leidos, err := repo.FormatosDeBanco(ctx, empFormatoUno, true)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(leidos) != 1 {
		t.Fatalf("trajo %d formatos, esperaba 1", len(leidos))
	}
	f := leidos[0]
	if len(f.Senales) != 2 {
		t.Fatalf("señales = %d, esperaba 2: sin ellas el formato no reconoce ningún archivo", len(f.Senales))
	}
	if f.Senales[0].Texto != "fecha mov" || f.Senales[1].Columna != 4 {
		t.Errorf("las señales volvieron cambiadas: %+v", f.Senales)
	}
	// Y la prueba de fondo: lo que volvió de la base reconoce el archivo igual que el original.
	a, err := f.Adaptador()
	if err != nil {
		t.Fatalf("construir desde lo leído: %v", err)
	}
	if !a.Detecta(gridLafise) {
		t.Error("el formato leído de la base ya no reconoce su propio archivo")
	}
}

// Un formato es de UNA empresa. Uno mal configurado lee los montos corridos, y eso no puede
// cruzar de una empresa a otra.
func TestUnFormatoDeOtraEmpresaNoSeVeNiSeToca(t *testing.T) {
	repo := baseDeFormatos(t)
	ctx := context.Background()

	ajeno, err := repo.GuardarFormatoBanco(ctx, empFormatoDos, formatoLafise(), usrFormato)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}

	míos, err := repo.FormatosDeBanco(ctx, empFormatoUno, false)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(míos) != 0 {
		t.Errorf("se ven %d formatos de la otra empresa", len(míos))
	}

	// Ni editarlo…
	editado := formatoLafise()
	editado.ID = ajeno.ID
	editado.Nombre = "Secuestrado"
	if _, err := repo.GuardarFormatoBanco(ctx, empFormatoUno, editado, usrFormato); !errors.Is(err, ErrFormatoNoEncontrado) {
		t.Errorf("se pudo editar el formato de otra empresa: %v", err)
	}
	// …ni borrarlo.
	if err := repo.BorrarFormatoBanco(ctx, empFormatoUno, ajeno.ID); !errors.Is(err, ErrFormatoNoEncontrado) {
		t.Errorf("se pudo borrar el formato de otra empresa: %v", err)
	}
	// Y sigue intacto para su dueña.
	suyos, err := repo.FormatosDeBanco(ctx, empFormatoDos, false)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(suyos) != 1 || suyos[0].Nombre != "Lafise" {
		t.Errorf("el formato de la otra empresa quedó alterado: %+v", suyos)
	}
}

// Dos formatos con el mismo nombre no se distinguen en la pantalla, y el nombre es lo que queda
// escrito en cada importación. Se compara sin may/min: «BAC» y «bac» son el mismo banco.
func TestNoSePuedenGuardarDosFormatosConElMismoNombre(t *testing.T) {
	repo := baseDeFormatos(t)
	ctx := context.Background()

	if _, err := repo.GuardarFormatoBanco(ctx, empFormatoUno, formatoLafise(), usrFormato); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	otro := formatoLafise()
	otro.Nombre = "LAFISE"
	if _, err := repo.GuardarFormatoBanco(ctx, empFormatoUno, otro, usrFormato); !errors.Is(err, ErrFormatoDuplicado) {
		t.Errorf("se guardó un segundo formato con el mismo nombre: %v", err)
	}
	// Pero la OTRA empresa sí puede tener el suyo con ese nombre.
	if _, err := repo.GuardarFormatoBanco(ctx, empFormatoDos, formatoLafise(), usrFormato); err != nil {
		t.Errorf("la otra empresa no pudo usar el mismo nombre: %v", err)
	}
}

// PROBAR UN FORMATO A MEDIAS TIENE QUE DEVOLVER LAS FILAS IGUAL.
//
// Se descubrió usando la pantalla, no leyendo el código: el PRIMER paso para configurar un banco
// es subir el archivo para poder VER en qué columna viene cada cosa, y en ese momento el formato
// está necesariamente vacío. Con la validación cortando, nadie podía llegar nunca a tener un
// formato completo: para configurar los números había que conocerlos de antemano.
func TestProbarUnFormatoAMediasDevuelveLasFilasIgual(t *testing.T) {
	svc := NewService(baseDeFormatos(t), nil, zap.NewNop(), true)
	ctx := context.Background()

	archivo, err := excelDePrueba([][]any{
		{"BANCO LAFISE COSTA RICA"},
		{"Fecha mov.", "Concepto", "Comprobante", "Haber", "Debe"},
		{"2026-10-01", "DEPOSITO", "991001", "150,000.00", ""},
	})
	if err != nil {
		t.Fatalf("armar excel: %v", err)
	}

	// Un formato completamente vacío: es con lo que se entra a la pantalla.
	prueba, err := svc.ProbarFormato(ctx, FormatoBanco{}, archivo)
	if err != nil {
		t.Fatalf("probar un formato vacío no puede fallar: %v", err)
	}
	if len(prueba.Filas) < 2 {
		t.Fatalf("filas = %d: sin ellas no hay con qué contar las columnas", len(prueba.Filas))
	}
	if prueba.Filas[1][0] != "Fecha mov." {
		t.Errorf("la grilla no trae el contenido real: %v", prueba.Filas[1])
	}
	if prueba.Detectado {
		t.Error("un formato vacío no puede decir que reconoció el archivo")
	}
	// Y dice POR QUÉ, en el mismo lugar donde se va a leer.
	if prueba.Problema == "" {
		t.Error("no se explicó qué le falta al formato")
	}

	// Lo único que sí falla es un archivo que no es un Excel.
	if _, err := svc.ProbarFormato(ctx, formatoLafise(), []byte("esto no es un excel")); err == nil {
		t.Error("se aceptó un archivo ilegible")
	}
}

// excelDePrueba arma un .xlsx en memoria con esas filas.
func excelDePrueba(filas [][]any) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for i, fila := range filas {
		cel, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			return nil, err
		}
		if err := f.SetSheetRow("Sheet1", cel, &fila); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Apagar un formato lo saca del importador sin perderlo: es la salida cuando algo se configuró
// mal y hay que volver a los seis de siempre mientras se revisa.
func TestApagarUnFormatoLoSacaDelImportadorSinBorrarlo(t *testing.T) {
	repo := baseDeFormatos(t)
	ctx := context.Background()

	guardado, err := repo.GuardarFormatoBanco(ctx, empFormatoUno, formatoLafise(), usrFormato)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	guardado.Activo = false
	if _, err := repo.GuardarFormatoBanco(ctx, empFormatoUno, guardado, usrFormato); err != nil {
		t.Fatalf("apagar: %v", err)
	}

	activos, err := repo.FormatosDeBanco(ctx, empFormatoUno, true)
	if err != nil {
		t.Fatalf("listar activos: %v", err)
	}
	if len(activos) != 0 {
		t.Error("el formato apagado sigue llegándole al importador")
	}
	todos, err := repo.FormatosDeBanco(ctx, empFormatoUno, false)
	if err != nil {
		t.Fatalf("listar todos: %v", err)
	}
	if len(todos) != 1 {
		t.Error("el formato apagado desapareció de la pantalla: apagar no es borrar")
	}
}
