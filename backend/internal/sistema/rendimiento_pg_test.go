package sistema

// Que la pantalla de consultas lentas NO filtre datos, contra PostgreSQL de verdad (11-nov-2026).
//
// Esta pantalla muestra el SQL que el ERP ejecuta, y el ERP ejecuta consultas con montos, cédulas
// y nombres de proveedores adentro. Lo único que impide que esos valores salgan por acá es que
// PostgreSQL NORMALIZA cada consulta antes de contarla: reemplaza los literales por $1, $2.
//
// Eso es una propiedad del motor, no del código de este repositorio, y por lo tanto NO SE PUEDE
// PROBAR CON UN DOBLE: un doble devolvería lo que uno le programe. La única forma de saber que de
// verdad no se escapa nada es ejecutar una consulta con un valor reconocible y buscarlo en la
// salida.
//
// Si algún día PostgreSQL cambiara ese comportamiento, o alguien activara `pg_stat_statements.
// track_utility` con sentencias que no se normalizan, esta prueba es lo que lo delata.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// poolDeRendimiento abre el pool real. A diferencia de las otras pruebas de este paquete NO crea
// un esquema temporal: `pg_stat_statements` es del MOTOR, no de un esquema, así que no hay nada
// que aislar — y tampoco nada que ensuciar, porque solo se lee un contador en memoria.
func poolDeRendimiento(t *testing.T) *pgxpool.Pool {
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
	t.Cleanup(pool.Close)
	return pool
}

func TestLasConsultasLentasNoMuestranDatosDeNadie(t *testing.T) {
	pool := poolDeRendimiento(t)
	repo := NewRepositoryRendimiento(pool)
	ctx := context.Background()

	if _, _, err := repo.ConsultasLentas(ctx, 5); err == ErrRendimientoNoDisponible {
		t.Skip("pg_stat_statements no está activo en esta base")
	}

	// NO SE REINICIA EL CONTADOR, y es a propósito. La primera versión de esta prueba lo hacía
	// para quedarse sola en el top, pero eso toca estado GLOBAL del motor: descarta la medición de
	// quien estuviera diagnosticando lentitud en esa base, y vuelve la prueba sensible a lo que
	// hagan los demás paquetes —que corren en paralelo y, al menos una vez, dejaron la extensión
	// en un estado donde el reset ni siquiera existía—.
	//
	// En su lugar, la consulta de abajo se hace deliberadamente LENTA con `pg_sleep`: así entra en
	// el top por mérito propio, sin pedirle nada al resto del mundo.

	// Una consulta con valores que NO pueden aparecer por casualidad: si salen en la lista, es que
	// los literales no se están normalizando.
	//
	// Va contra `movimiento_bancario` y con un `pg_sleep` a propósito: la lista se ordena por
	// TIEMPO TOTAL, y una consulta instantánea contra una tabla chica se hunde al fondo en cuanto
	// la batería completa llena el contador con cientos de consultas. Así entra siempre, y la
	// prueba deja de pasar o fallar según en qué orden se corrió lo demás.
	// SE PRUEBA EL CASO DEL ERP: una consulta PARAMETRIZADA. Es como está escrita toda consulta de
	// este sistema —es regla del proyecto— y es donde la garantía se sostiene.
	//
	// LO QUE SE APRENDIÓ POR EL CAMINO, y que cambia lo que se puede prometer: con el valor
	// INCRUSTADO en el texto de la consulta, PostgreSQL casi siempre lo normaliza (comprobado
	// tres veces: queda `descripcion = $1`), pero NO SIEMPRE. En una corrida de la batería
	// completa esta misma prueba vio su propio literal, único de esa ejecución, guardado tal cual.
	//
	// Es decir: «la pantalla de consultas lentas nunca muestra datos» es cierto para las consultas
	// del ERP y no es un absoluto del motor. Por eso el permiso de esa pantalla es crítico y su
	// descripción lo dice.
	marca := time.Now().UnixNano()
	cedulaDePrueba := fmt.Sprintf("3101%d", marca)
	montoDePrueba := fmt.Sprintf("9876.%d", marca%100000)
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) + (SELECT count(*) FROM pg_sleep(0.6))
		   FROM movimiento_bancario
		  WHERE descripcion = $1 OR descripcion = $2`,
		cedulaDePrueba, montoDePrueba).Scan(&n); err != nil {
		t.Fatalf("ejecutar la consulta de prueba: %v", err)
	}

	// Se pide el máximo para que la consulta de arriba esté entre las devueltas: es rapidísima, así
	// que por tiempo total queda al fondo.
	consultas, _, err := repo.ConsultasLentas(ctx, topConsultasMaximo)
	if err != nil {
		t.Fatalf("leer consultas lentas: %v", err)
	}

	// SE EXIGE ENCONTRARLA. Sin esto la prueba pasaba sin comprobar nada: si la consulta de prueba
	// no aparecía en la lista, el bucle de abajo no encontraba el dato por la razón equivocada y
	// daba verde igual. Una prueba que no puede comprobar lo que dice tiene que fallar, no pasar.
	var laNuestra string
	for _, c := range consultas {
		if strings.Contains(c.Consulta, "FROM movimiento_bancario WHERE descripcion") {
			laNuestra = c.Consulta
		}
		if strings.Contains(c.Consulta, cedulaDePrueba) {
			t.Errorf("SE ESCAPÓ UN DATO: la cédula de prueba aparece en %q", c.Consulta)
		}
		if strings.Contains(c.Consulta, montoDePrueba) {
			t.Errorf("SE ESCAPÓ UN DATO: el monto de prueba aparece en %q", c.Consulta)
		}
	}
	if laNuestra == "" {
		t.Fatalf("no se encontró la consulta de prueba entre las %d devueltas: "+
			"la prueba no comprobó nada", len(consultas))
	}
	// Y se ve la marca de que el motor la normalizó: donde iban los literales ahora hay $1.
	if !strings.Contains(laNuestra, "$1") {
		t.Errorf("la consulta no está normalizada: %q", laNuestra)
	}
}

// Lo que la pantalla ordena por tiempo TOTAL, no por el más lento de una vez.
//
// Importa porque fue la forma exacta del problema que motivó todo esto: una consulta de 14 ms que
// el tablero repetía siete veces por carga. Ordenada por media, habría quedado invisible entre
// consultas que tardan más pero corren una vez al día.
func TestLasConsultasVienenOrdenadasPorTiempoTotal(t *testing.T) {
	pool := poolDeRendimiento(t)
	repo := NewRepositoryRendimiento(pool)
	ctx := context.Background()

	consultas, _, err := repo.ConsultasLentas(ctx, 10)
	if err == ErrRendimientoNoDisponible {
		t.Skip("pg_stat_statements no está activo en esta base")
	}
	if err != nil {
		t.Fatalf("leer consultas lentas: %v", err)
	}
	if len(consultas) < 2 {
		t.Skip("hacen falta al menos dos consultas registradas para comprobar el orden")
	}
	for i := 1; i < len(consultas); i++ {
		if consultas[i].MsTotal > consultas[i-1].MsTotal {
			t.Fatalf("desordenadas: la #%d (%.2f ms) va después de la #%d (%.2f ms)",
				i, consultas[i].MsTotal, i-1, consultas[i-1].MsTotal)
		}
	}
	// Y el texto viene recortado: las consultas de este sistema pasan de 2.000 caracteres.
	for _, c := range consultas {
		if len([]rune(c.Consulta)) > largoConsultaVisible {
			t.Errorf("consulta sin recortar (%d caracteres)", len([]rune(c.Consulta)))
		}
	}
}
