package bancos

// Regresión del incidente del 16-set-2026: el mismo estado de cuenta importado en DOS cuentas.
// La corrección marca los movimientos de la cuenta equivocada con `incluido = false`, y esa marca
// es la compuerta que el resto del sistema ya respeta. Estas pruebas cubren las consultas de
// Clasificar / motor / segmento que NO la miraban y por eso seguían contando la plata duplicada.
//
// Se corren contra Postgres de verdad porque lo que cambió es el SQL: un doble contra el
// repositorio probaría el doble, no la consulta. Para no tocar ni un dato de la base local, cada
// corrida crea un ESQUEMA temporal con copias (`LIKE ... INCLUDING ALL`) de las tablas que las
// consultas tocan —columnas, tipos y checks reales, sin llaves foráneas a lo de afuera—, siembra
// el incidente ahí y lo borra al terminar. Las consultas bajo prueba son todas de LECTURA.
//
// Sin base de datos la prueba se OMITE (no falla): así corre en esta máquina y no rompe CI.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Las tablas que tocan las consultas bajo prueba, en orden de creación.
var tablasDelIncidente = []string{
	"banco", "cuenta_bancaria", "concepto", "clasificacion",
	"movimiento_bancario", "usuario", "movimiento_reporte_segmentacion",
}

// baseDelIncidente levanta el esquema temporal y devuelve el repositorio que apunta a él.
func baseDelIncidente(t *testing.T) *pgRepository {
	t.Helper()
	dsn := os.Getenv("GPVDP_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://gpvdp:localdev@127.0.0.1:5432/gpvdp?sslmode=disable"
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("sin base de datos para la prueba de regresión (%v)", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("sin base de datos para la prueba de regresión (%v)", err)
	}

	esquema := fmt.Sprintf("prueba_excluido_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+esquema); err != nil {
		admin.Close()
		t.Fatalf("crear esquema de prueba: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+esquema+` CASCADE`); err != nil {
			t.Errorf("borrar esquema de prueba: %v", err)
		}
		admin.Close()
	})
	for _, tabla := range tablasDelIncidente {
		if _, err := admin.Exec(ctx,
			fmt.Sprintf(`CREATE TABLE %s.%s (LIKE public.%s INCLUDING ALL)`, esquema, tabla, tabla)); err != nil {
			t.Fatalf("copiar tabla %s: %v", tabla, err)
		}
	}

	// El pool de las consultas ve ÚNICAMENTE el esquema temporal: si alguna tabla faltara, la
	// consulta falla en vez de leer los datos reales sin que nadie se entere.
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parsear dsn: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = esquema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool del esquema de prueba: %v", err)
	}
	t.Cleanup(pool.Close)
	return &pgRepository{pool: pool}
}

// movSembrado es un movimiento del incidente reproducido.
type movSembrado struct {
	nombre       string
	cuenta       string // id de cuenta_bancaria
	fecha        string
	debito       string
	credito      string
	montoCRC     string
	moneda       string // "CRC" o "USD"
	montoOrig    string
	tc           any // nil = sin tipo de cambio aplicado
	clasificacio string
	concepto     string
	estado       string
	descripcion  string
	incluido     bool
}

const (
	empresaPrueba = "11111111-1111-4111-8111-111111111111"
	bancoPrueba   = "22222222-2222-4222-8222-222222222222"
	cuentaBuena   = "33333333-3333-4333-8333-333333333333"
	cuentaMala    = "44444444-4444-4444-8444-444444444444"
	conceptoID    = "55555555-5555-4555-8555-555555555555"
	partidaMia    = "66666666-6666-4666-8666-666666666666"
	usuarioPrueba = "77777777-7777-4777-8777-777777777777"
)

// sembrarIncidente deja en la base el caso real: dos cuentas Promerica con el MISMO estado de
// cuenta, y la copia de la cuenta equivocada ya marcada `incluido = false`.
// Devuelve el id de cada movimiento por su nombre.
func sembrarIncidente(t *testing.T, repo *pgRepository) map[string]string {
	t.Helper()
	ctx := context.Background()

	catalogo := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Promerica')`,
			[]any{bancoPrueba, empresaPrueba}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'Promerica VDP')`,
			[]any{cuentaBuena, empresaPrueba, bancoPrueba}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'Promerica Colinas')`,
			[]any{cuentaMala, empresaPrueba, bancoPrueba}},
		{`INSERT INTO concepto (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Ingresos')`,
			[]any{conceptoID, empresaPrueba}},
		{`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'Depósito de Clientes')`,
			[]any{partidaMia, empresaPrueba, conceptoID}},
		{`INSERT INTO usuario (id, nombre, email, password_hash)
		  VALUES ($1::uuid, 'Tesorería', 'tesoreria@prueba.local', 'x')`,
			[]any{usuarioPrueba}},
	}
	for _, c := range catalogo {
		if _, err := repo.pool.Exec(ctx, c.q, c.args...); err != nil {
			t.Fatalf("sembrar catálogo: %v", err)
		}
	}

	movs := []movSembrado{
		// El depósito de verdad y su copia en la cuenta equivocada (ya corregida).
		{nombre: "deposito_bueno", cuenta: cuentaBuena, fecha: "2026-09-08", credito: "259230.50",
			montoCRC: "259230.50", clasificacio: partidaMia, concepto: conceptoID,
			estado: "REVISADO", descripcion: "DEPOSITO CLIENTE", incluido: true},
		{nombre: "deposito_duplicado", cuenta: cuentaMala, fecha: "2026-09-08", credito: "259230.50",
			montoCRC: "259230.50", clasificacio: partidaMia, concepto: conceptoID,
			estado: "REVISADO", descripcion: "DEPOSITO CLIENTE", incluido: false},
		// Un débito sin clasificar y su duplicado: alimentan al motor y al banner de la regla.
		{nombre: "pago_bueno", cuenta: cuentaBuena, fecha: "2026-09-09", debito: "1000000.00",
			montoCRC: "1000000.00", estado: "NO_IDENTIFICADO", descripcion: "PAGO LIQ PROVEEDOR", incluido: true},
		{nombre: "pago_duplicado", cuenta: cuentaMala, fecha: "2026-09-09", debito: "1000000.00",
			montoCRC: "1000000.00", estado: "NO_IDENTIFICADO", descripcion: "PAGO LIQ PROVEEDOR", incluido: false},
		// El movimiento MÁS NUEVO de la empresa es uno excluido: decide «cargado hasta».
		{nombre: "credito_del_30", cuenta: cuentaMala, fecha: "2026-09-30", credito: "500000.00",
			montoCRC: "500000.00", estado: "NO_IDENTIFICADO", descripcion: "TRANSFERENCIA", incluido: false},
		// Un excluido en dólares SIN tipo de cambio: hoy dispara la alerta «tu total está corto».
		{nombre: "usd_sin_tc", cuenta: cuentaMala, fecha: "2026-09-10", credito: "2000.00",
			montoCRC: "0.00", moneda: "USD", montoOrig: "2000.00", tc: nil,
			estado: "NO_IDENTIFICADO", descripcion: "WIRE", incluido: false},
		// Un crédito que SOLO existe excluido: es el que hace mentir a «buscá mi depósito».
		{nombre: "solo_excluido", cuenta: cuentaMala, fecha: "2026-09-12", credito: "777777.00",
			montoCRC: "777777.00", clasificacio: partidaMia, concepto: conceptoID,
			estado: "REVISADO", descripcion: "DEPOSITO CLIENTE", incluido: false},
	}

	const q = `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, tc_aplicado,
			 concepto_id, clasificacion_id, estado_clasificacion, natural_key, incluido)
		VALUES ($1::uuid, $2::uuid, $3::date, $4, $5, $6, $7, $8, $9, $10,
		        NULLIF($11,'')::uuid, NULLIF($12,'')::uuid, $13, $14, $15)
		RETURNING id::text`
	ids := map[string]string{}
	for _, m := range movs {
		moneda := m.moneda
		if moneda == "" {
			moneda = "CRC"
		}
		montoOrig := m.montoOrig
		if montoOrig == "" {
			montoOrig = m.montoCRC
		}
		deb, cred := m.debito, m.credito
		if deb == "" {
			deb = "0"
		}
		if cred == "" {
			cred = "0"
		}
		var id string
		if err := repo.pool.QueryRow(ctx, q,
			empresaPrueba, m.cuenta, m.fecha, m.descripcion, deb, cred,
			moneda, montoOrig, m.montoCRC, m.tc,
			m.concepto, m.clasificacio, m.estado, m.nombre, m.incluido).Scan(&id); err != nil {
			t.Fatalf("sembrar movimiento %s: %v", m.nombre, err)
		}
		ids[m.nombre] = id
	}
	return ids
}

// igualDecimal compara montos por VALOR y no por texto: "1000000" y "1000000.00" son el mismo
// dinero, y una prueba que falle por la escala solo enseña a cambiarla sin mirar.
func igualDecimal(t *testing.T, got, esperado string) bool {
	t.Helper()
	g, err := decimal.NewFromString(got)
	if err != nil {
		t.Fatalf("monto ilegible %q: %v", got, err)
	}
	return g.Equal(decimal.RequireFromString(esperado))
}

func TestExcluidoNoSuma(t *testing.T) {
	repo := baseDelIncidente(t)
	ids := sembrarIncidente(t, repo)
	ctx := context.Background()
	alcance := []string{partidaMia}
	periodo := FiltrosMovimientos{Periodo: "2026-09"}

	// Bancos › Clasificar, encabezado. El dinero NO cuenta lo excluido; el conteo de filas SÍ,
	// porque la tabla de abajo las sigue mostrando y encabezado y tabla no pueden contradecirse.
	t.Run("el encabezado de la selección no suma los excluidos pero los cuenta aparte", func(t *testing.T) {
		svc := NewService(repo, nil, nil, false)
		got, err := svc.ResumenFiltro(ctx, empresaPrueba, periodo, AgruparConcepto)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		// Sin el arreglo: crédito 1796238.00 (entran los 4 créditos duplicados) y débito 2000000.00.
		if !igualDecimal(t, got.TotalCredito, "259230.50") {
			t.Errorf("crédito = %s, se esperaba 259230.50 (solo el depósito bueno)", got.TotalCredito)
		}
		if !igualDecimal(t, got.TotalDebito, "1000000.00") {
			t.Errorf("débito = %s, se esperaba 1000000.00 (solo el pago bueno)", got.TotalDebito)
		}
		if !igualDecimal(t, got.Neto, "-740769.50") {
			t.Errorf("neto = %s, se esperaba -740769.50 (crédito − débito, sin excluidos)", got.Neto)
		}
		if got.Movs != 7 {
			t.Errorf("movs = %d, se esperaban 7: el encabezado cuenta las filas que la lista muestra", got.Movs)
		}
		if got.Excluidos != 5 {
			t.Errorf("excluidos = %d, se esperaban 5: sin este número la resta del encabezado no se explica", got.Excluidos)
		}
	})

	// Hoja de trabajo (y «Mi segmento», que reusa esta misma consulta).
	t.Run("los totales de la hoja de trabajo no suman los excluidos y la fila viaja marcada", func(t *testing.T) {
		got, err := repo.ListarMovimientos(ctx, empresaPrueba, periodo)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if !igualDecimal(t, got.Totales.TotalCreditos, "259230.50") {
			t.Errorf("créditos = %s, se esperaba 259230.50", got.Totales.TotalCreditos)
		}
		if !igualDecimal(t, got.Totales.TotalDebitos, "1000000.00") {
			t.Errorf("débitos = %s, se esperaba 1000000.00", got.Totales.TotalDebitos)
		}
		if got.Total != 7 {
			t.Errorf("total de filas = %d, se esperaban 7 (la lista los sigue mostrando)", got.Total)
		}
		if got.Totales.Excluidos != 5 {
			t.Errorf("excluidos = %d, se esperaban 5", got.Totales.Excluidos)
		}
		// El USD sin tipo de cambio que está excluido no puede disparar la alerta «tu total está
		// corto»: ese dinero no debía entrar al total.
		if got.Totales.SinTipoCambio != 0 {
			t.Errorf("sin tipo de cambio = %d, se esperaba 0 (el único es un excluido)", got.Totales.SinTipoCambio)
		}
		if !igualDecimal(t, got.Totales.MontoSinConvertir, "0") {
			t.Errorf("monto sin convertir = %s, se esperaba 0", got.Totales.MontoSinConvertir)
		}
		// Mostrar sin marcar sería lo peor de todo: la fila se leería como plata buena.
		var visto, marcados int
		for _, it := range got.Items {
			if it.ID == ids["deposito_duplicado"] {
				visto++
				if it.Incluido {
					t.Error("el duplicado llega con incluido = true: la pantalla lo pintaría como plata buena")
				}
			}
			if !it.Incluido {
				marcados++
			}
		}
		if visto != 1 {
			t.Error("el duplicado desapareció de la lista: quien corrigió no puede verificar qué marcó")
		}
		if marcados != 5 {
			t.Errorf("filas marcadas como excluidas = %d, se esperaban 5", marcados)
		}
	})

	// El motor ESCRIBE: clasificar un excluido le suma un acierto falso a la regla.
	t.Run("el motor no recibe los excluidos como entrada", func(t *testing.T) {
		got, err := repo.MovimientosNoIdentificados(ctx, empresaPrueba)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		// Sin el arreglo son 4: el pago duplicado, el crédito del 30 y el USD sin TC entran también.
		if len(got) != 1 || got[0].ID != ids["pago_bueno"] {
			t.Errorf("movimientos a clasificar = %d (%+v), se esperaba solo el pago bueno", len(got), got)
		}
	})

	t.Run("el banner de la regla promete lo que el motor va a clasificar", func(t *testing.T) {
		n, err := repo.ContarNoIdentificadosConPalabra(ctx, empresaPrueba, "LIQ", "DEBITO")
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if n != 1 {
			t.Errorf("banner = %d, se esperaba 1: con 2 prometería un movimiento que el motor no va a tocar", n)
		}
	})

	t.Run("el KPI de auto-clasificación no cuenta los excluidos", func(t *testing.T) {
		got, err := repo.ResumenClasificacion(ctx, empresaPrueba, "2026-09")
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		// Sin el arreglo: total 7 y 4 sin clasificar, mientras el bloqueo del cierre —que ya mira
		// la marca— diría 1. Dos números distintos para la misma frase en dos pantallas.
		if got.Total != 2 {
			t.Errorf("total = %d, se esperaban 2: el denominador del porcentaje no puede traer plata duplicada", got.Total)
		}
		if got.NoIdentificados != 1 {
			t.Errorf("sin clasificar = %d, se esperaba 1 (lo mismo que dice el cierre de período)", got.NoIdentificados)
		}
		if got.Revisados != 1 {
			t.Errorf("revisados = %d, se esperaba 1", got.Revisados)
		}
	})

	// «Buscá mi depósito»: el veredicto se calcula sobre la plata que cuenta.
	t.Run("buscar un depósito no encuentra el duplicado", func(t *testing.T) {
		mios, fuera, err := repo.BuscarPorFechaYMonto(ctx, empresaPrueba, "2026-09-08", dec("259230.50"), alcance)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if len(mios) != 1 || mios[0].ID != ids["deposito_bueno"] {
			t.Errorf("coincidencias = %d (%+v), se esperaba solo el depósito bueno", len(mios), mios)
		}
		if fuera != 0 {
			t.Errorf("fuera del alcance = %d, se esperaba 0", fuera)
		}
		// La fila que sí se devuelve tiene que llegar marcada como incluida: si el SELECT no
		// trajera la columna, Go la llenaría con `false` y la pantalla pintaría TODO como excluido.
		if len(mios) > 0 && !mios[0].Incluido {
			t.Error("el movimiento bueno llega con incluido = false: el SELECT no está trayendo la marca")
		}
	})

	t.Run("si la única coincidencia está excluida, el depósito NO existe", func(t *testing.T) {
		mios, fuera, err := repo.BuscarPorFechaYMonto(ctx, empresaPrueba, "2026-09-12", dec("777777.00"), alcance)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		// Sin el arreglo el equipo lee «existe, está en tu partida», deja de buscar, y el depósito
		// de verdad puede seguir faltando.
		if len(mios) != 0 || fuera != 0 {
			t.Errorf("coincidencias = %d, fuera = %d; esa plata no entró a los libros", len(mios), fuera)
		}
	})

	t.Run("el enganche del aviso no se empata contra el duplicado", func(t *testing.T) {
		got, err := repo.EngancharFaltante(ctx, empresaPrueba, "2026-09-08", dec("259230.50"))
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		// Sin el arreglo hay 2 candidatos, la función devuelve "" a propósito y el aviso viaja sin
		// movimiento: exactamente el trabajo manual que existe para evitar.
		if got != ids["deposito_bueno"] {
			t.Errorf("enganche = %q, se esperaba el depósito bueno %q", got, ids["deposito_bueno"])
		}
	})

	t.Run("el desplegable no ofrece una cuenta que aporta cero", func(t *testing.T) {
		got, err := repo.CuentasDelAlcance(ctx, empresaPrueba, alcance)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if len(got) != 1 || got[0].Cuenta != "Promerica VDP" {
			t.Errorf("cuentas = %+v, se esperaba solo «Promerica VDP»", got)
		}
	})

	t.Run("«cargado hasta» no se apoya en una importación excluida", func(t *testing.T) {
		got, err := repo.UltimaFechaCargada(ctx, empresaPrueba)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		// Sin el arreglo diría 2026-09-30 —la fecha del crédito excluido— y «no está y estamos al
		// día» frenaría un aviso de faltante legítimo.
		if got != "2026-09-09" {
			t.Errorf("cargado hasta = %q, se esperaba 2026-09-09", got)
		}
	})

	// La cola de avisos MUESTRA lo excluido: ahí «se excluyó por duplicado» suele ser la respuesta.
	t.Run("la cola de avisos sigue mostrando el excluido, marcado", func(t *testing.T) {
		for _, mov := range []string{"deposito_bueno", "deposito_duplicado"} {
			if _, err := repo.CrearReporteSegmentacion(ctx, empresaPrueba, ids[mov], usuarioPrueba,
				"no es de mi partida"); err != nil {
				t.Fatalf("crear aviso sobre %s: %v", mov, err)
			}
		}
		got, err := repo.ListarReportesSegmentacion(ctx, empresaPrueba, true)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("avisos = %d, se esperaban 2: filtrar acá dejaría el aviso mudo, no lo sacaría", len(got))
		}
		porMov := map[string]ReporteSegmentacion{}
		for _, r := range got {
			porMov[r.MovimientoID] = r
		}
		if r, ok := porMov[ids["deposito_duplicado"]]; !ok || !r.MovExcluido {
			t.Errorf("el aviso del duplicado no llega marcado (%+v): quien resuelve no ve la respuesta", r)
		}
		if r, ok := porMov[ids["deposito_bueno"]]; !ok || r.MovExcluido {
			t.Errorf("el aviso del movimiento bueno llega marcado como excluido (%+v)", r)
		}
	})
}
