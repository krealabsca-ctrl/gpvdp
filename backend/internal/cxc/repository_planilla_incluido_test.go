package cxc

// La marca `movimiento_bancario.incluido` en las planillas de asociación.
//
// Incidente real (16-set-2026): la tesorera importó el mismo estado de cuenta en dos cuentas
// Promerica. La corrección marca los movimientos de la cuenta equivocada con `incluido = false`.
// Estas consultas de CxC nunca miraban esa marca: un depósito duplicado seguía sumando en
// «Depositado» y dejaba la planilla en CONCILIADA con plata que no existe.
//
// Por qué la prueba va contra Postgres de verdad y no contra un doble: lo que falla o no falla
// es el SQL. Un test con un repositorio falso no puede distinguir un `AND m.incluido` puesto de
// uno olvidado, ni la diferencia entre ponerlo en el ON y ponerlo en el WHERE (que borra la
// asociación del panorama). Toda la prueba corre en un ESQUEMA temporal propio, con las tablas
// copiadas de las reales (LIKE) y vacías: no lee ni toca un solo dato de la empresa.
//
// Sin base disponible la prueba se salta con un motivo, no falla.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Las tablas que tocan las tres consultas. Se copian vacías al esquema de prueba para que el
// `search_path` no pueda caer nunca en las reales.
var tablasDelEscenario = []string{
	"banco", "cuenta_bancaria", "clasificacion", "movimiento_bancario",
	"cxc_asociacion", "cxc_planilla", "cxc_planilla_movimiento",
	"contrato_cxc", "cargo_cxc", "cobro_cxc",
}

// escenarioIncluido es el caso del incidente, en chiquito:
//
//	planilla 1  registrado ₡10.000.000  ·  dos depósitos vinculados: ₡4.000.000 (vivo) y
//	            ₡6.000.000 (el duplicado, excluido después de vincularlo)
//	planilla 2  registrado ₡3.000.000   ·  su ÚNICO depósito quedó excluido
//	sueltos     un crédito de ₡6.000.000 incluido (calza con lo que de verdad falta) y otro
//	            de ₡1.051.233 excluido (el que hoy se ofrece y no debería)
type escenarioIncluido struct {
	repo         *pgRepository
	empresaID    string
	asociacionID string
	planillaID   string
	movVivo      string // vinculado, incluido
	movDuplicado string // vinculado, EXCLUIDO
	candCalza    string // suelto, incluido, ₡6.000.000
	candExcluido string // suelto, EXCLUIDO
	asociacion2  string
	planilla2    string
	mov2Excluido string
}

func nuevoEscenarioIncluido(t *testing.T) escenarioIncluido {
	t.Helper()
	ctx := context.Background()

	dsn := os.Getenv("GPVDP_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://gpvdp:localdev@127.0.0.1:5432/gpvdp?sslmode=disable"
	}
	conexion, cancelar := context.WithTimeout(ctx, 5*time.Second)
	defer cancelar()
	admin, err := pgx.Connect(conexion, dsn)
	if err != nil {
		t.Skipf("sin base local (%v): levantá `docker compose up -d db` o poné GPVDP_TEST_DSN", err)
	}

	esquema := fmt.Sprintf("cxc_incluido_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+esquema); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("crear esquema de prueba: %v", err)
	}
	// El borrado queda registrado ANTES que el cierre del pool: t.Cleanup corre al revés, así
	// que el pool se cierra primero y el DROP no se queda esperando conexiones.
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+esquema+" CASCADE")
		_ = admin.Close(ctx)
	})
	for _, tabla := range tablasDelEscenario {
		if _, err := admin.Exec(ctx,
			fmt.Sprintf("CREATE TABLE %s.%s (LIKE public.%s INCLUDING DEFAULTS)", esquema, tabla, tabla)); err != nil {
			t.Fatalf("copiar la tabla %s: %v", tabla, err)
		}
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	// Solo el esquema de prueba: si una consulta nombrara una tabla que no copié, la prueba
	// grita en vez de leer callada los datos reales.
	cfg.ConnConfig.RuntimeParams["search_path"] = esquema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	e := escenarioIncluido{repo: &pgRepository{pool: pool}}
	uno := func(sql string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("insertar (%s): %v", sql, err)
		}
		return id
	}
	e.empresaID = uno(`SELECT gen_random_uuid()::text`)
	bancoID := uno(`INSERT INTO banco (empresa_id, nombre) VALUES ($1::uuid, 'Promerica') RETURNING id::text`, e.empresaID)
	cuentaID := uno(`INSERT INTO cuenta_bancaria (empresa_id, banco_id, moneda, alias)
		VALUES ($1::uuid, $2::uuid, 'CRC', 'Promerica VDP') RETURNING id::text`, e.empresaID, bancoID)

	mov := func(fecha, monto string, incluido bool) string {
		t.Helper()
		return uno(`INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, credito, monto_crc, monto_original, natural_key, descripcion, incluido)
			VALUES ($1::uuid, $2::uuid, $3::date, $4::numeric, $4::numeric, $4::numeric, $5, 'TEF DE:ASOCIACION SOLIDARISTA', $6)
			RETURNING id::text`,
			e.empresaID, cuentaID, fecha, monto, fmt.Sprintf("nk-%s-%s-%d", fecha, monto, time.Now().UnixNano()), incluido)
	}
	asociacion := func(nombre string) string {
		t.Helper()
		return uno(`INSERT INTO cxc_asociacion (empresa_id, nombre) VALUES ($1::uuid, $2) RETURNING id::text`, e.empresaID, nombre)
	}
	planilla := func(asocID string) string {
		t.Helper()
		return uno(`INSERT INTO cxc_planilla (empresa_id, asociacion_id, periodo, referencia)
			VALUES ($1::uuid, $2::uuid, '2026-07', 'COMP-1') RETURNING id::text`, e.empresaID, asocID)
	}
	vincular := func(planillaID, movID string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO cxc_planilla_movimiento (empresa_id, planilla_id, movimiento_bancario_id)
			VALUES ($1::uuid, $2::uuid, $3::uuid)`, e.empresaID, planillaID, movID); err != nil {
			t.Fatalf("vincular depósito de prueba: %v", err)
		}
	}
	cobro := func(asocID, monto string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO cobro_cxc (empresa_id, asociacion_id, fecha_pago, fecha_bancaria, monto)
			VALUES ($1::uuid, $2::uuid, '2026-07-15'::date, '2026-07-15'::date, $3::numeric)`,
			e.empresaID, asocID, monto); err != nil {
			t.Fatalf("cobro de prueba: %v", err)
		}
	}

	e.asociacionID = asociacion("ASEPRUEBA UNO")
	e.planillaID = planilla(e.asociacionID)
	e.movVivo = mov("2026-07-10", "4000000", true)
	e.movDuplicado = mov("2026-07-11", "6000000", false)
	vincular(e.planillaID, e.movVivo)
	vincular(e.planillaID, e.movDuplicado)
	cobro(e.asociacionID, "10000000")

	e.candCalza = mov("2026-07-14", "6000000", true)
	e.candExcluido = mov("2026-07-12", "1051233", false)

	e.asociacion2 = asociacion("ASEPRUEBA DOS")
	e.planilla2 = planilla(e.asociacion2)
	e.mov2Excluido = mov("2026-07-13", "3000000", false)
	vincular(e.planilla2, e.mov2Excluido)
	cobro(e.asociacion2, "3000000")

	return e
}

func igualMonto(t *testing.T, etiqueta, got, quiero string) {
	t.Helper()
	g, err := decimal.NewFromString(got)
	if err != nil {
		t.Fatalf("%s: monto ilegible %q: %v", etiqueta, got, err)
	}
	if !g.Equal(d(quiero)) {
		t.Errorf("%s = %s, se esperaba %s", etiqueta, got, quiero)
	}
}

// La ficha de la planilla: el DUPLICADO no suma en «Depositado», pero SIGUE en la lista.
//
// Sin el arreglo, «Depositado» dice ₡10.000.000 —los ₡6.000.000 fantasma incluidos— y como
// eso calza exacto con lo registrado, estadoPlanilla la declara CONCILIADA: la corrección de
// la tesorera no se ve por ningún lado.
func TestPlanillaDepositadoNoSumaElMovimientoExcluido(t *testing.T) {
	e := nuevoEscenarioIncluido(t)

	det, err := e.repo.PlanillaDeAsociacion(context.Background(), e.empresaID, e.asociacionID, "2026-07", decimal.Zero)
	if err != nil {
		t.Fatalf("PlanillaDeAsociacion: %v", err)
	}
	igualMonto(t, "registrado", det.Registrado, "10000000")
	igualMonto(t, "depositado", det.Depositado, "4000000")
	if det.Estado != "CON_DIFERENCIA" {
		t.Errorf("estado = %s, se esperaba CON_DIFERENCIA: la planilla tiene que CAER de estado cuando el duplicado deja de contar", det.Estado)
	}

	// MOSTRAR Y MARCAR: el vínculo lo hizo una persona. Si la fila desapareciera, el operador
	// vería bajar el total sin explicación y volvería a vincular otro crédito para «arreglarlo».
	if len(det.Movimientos) != 2 {
		t.Fatalf("movimientos = %d, se esperaban 2: el excluido se sigue mostrando, solo deja de sumar", len(det.Movimientos))
	}
	marcas := map[string]bool{}
	for _, m := range det.Movimientos {
		marcas[m.ID] = m.Incluido
	}
	if !marcas[e.movVivo] {
		t.Errorf("el depósito vivo salió marcado como excluido")
	}
	if marcas[e.movDuplicado] {
		t.Errorf("el duplicado tiene que venir con incluido = false para que la pantalla pueda pintarlo y explicar por qué bajó el total")
	}
}

// Los candidatos: no se ofrece un crédito excluido, y lo que «falta» se calcula sin él.
//
// Sin el arreglo pasan las dos cosas malas a la vez: el crédito excluido aparece en la lista
// para que alguien lo vincule, y `falta` (registrado − depositado) da 0 en vez de ₡6.000.000,
// así que el sistema le pone «calza monto» al crédito equivocado y lo ordena de primero.
func TestCandidatosDepositoIgnoranLoExcluido(t *testing.T) {
	e := nuevoEscenarioIncluido(t)

	cands, err := e.repo.CandidatosDeposito(context.Background(), e.empresaID, e.planillaID, 20)
	if err != nil {
		t.Fatalf("CandidatosDeposito: %v", err)
	}
	porID := map[string]CandidatoDeposito{}
	for _, c := range cands {
		if !c.Incluido {
			t.Errorf("candidato %s viene con incluido = false: un crédito excluido no se ofrece", c.ID)
		}
		porID[c.ID] = c
	}
	if _, hay := porID[e.candExcluido]; hay {
		t.Errorf("se ofreció como depósito un crédito EXCLUIDO del cuadre (%s)", e.candExcluido)
	}
	calza, hay := porID[e.candCalza]
	if !hay {
		t.Fatalf("el crédito vivo de ₡6.000.000 tiene que seguir apareciendo; candidatos = %d", len(cands))
	}
	// Lo que de verdad falta es 10.000.000 − 4.000.000; con el duplicado sumando sería 0.
	if !calza.CalzaMonto {
		t.Errorf("CalzaMonto = false: `falta` se calculó con el duplicado adentro (₡6.000.000 de menos)")
	}
	igualMonto(t, "diferencia del candidato que calza", calza.Diferencia, "0")
}

// El guardarraíl de escritura: un crédito excluido no puede DARSE por depositado.
//
// Es la única corrección preventiva del paquete: impide que el duplicado entre a cobranza, en
// vez de descontarlo después.
func TestVincularDepositoRechazaElExcluido(t *testing.T) {
	e := nuevoEscenarioIncluido(t)
	ctx := context.Background()

	err := e.repo.VincularDeposito(ctx, e.empresaID, e.planillaID, e.candExcluido, "")
	if !errors.Is(err, ErrMovimientoExcluido) {
		t.Fatalf("err = %v, se esperaba ErrMovimientoExcluido", err)
	}
	det, err := e.repo.PlanillaDeAsociacion(ctx, e.empresaID, e.asociacionID, "2026-07", decimal.Zero)
	if err != nil {
		t.Fatalf("PlanillaDeAsociacion: %v", err)
	}
	if len(det.Movimientos) != 2 {
		t.Errorf("movimientos = %d: el rechazo no puede haber dejado el vínculo hecho", len(det.Movimientos))
	}

	// Control: el crédito sano se sigue vinculando. El guardarraíl no puede haber cerrado la
	// operación normal.
	if err := e.repo.VincularDeposito(ctx, e.empresaID, e.planillaID, e.candCalza, ""); err != nil {
		t.Fatalf("vincular un crédito incluido falló: %v", err)
	}
	det, err = e.repo.PlanillaDeAsociacion(ctx, e.empresaID, e.asociacionID, "2026-07", decimal.Zero)
	if err != nil {
		t.Fatalf("PlanillaDeAsociacion: %v", err)
	}
	igualMonto(t, "depositado después de vincular el crédito sano", det.Depositado, "10000000")
	if det.Estado != "CONCILIADA" {
		t.Errorf("estado = %s, se esperaba CONCILIADA una vez vinculado el depósito de verdad", det.Estado)
	}
}

// La reparación NO puede quedar bloqueada por la marca del daño.
//
// DesvincularDeposito se dejó a propósito sin filtro: si exigiera `incluido`, el operador se
// quedaría sin poder soltar justamente el movimiento excluido, que es el que más urge soltar.
// Esta prueba existe para que nadie «complete» el arreglo agregándole el filtro.
func TestDesvincularDepositoFuncionaSobreUnExcluido(t *testing.T) {
	e := nuevoEscenarioIncluido(t)
	ctx := context.Background()

	if err := e.repo.DesvincularDeposito(ctx, e.empresaID, e.planillaID, e.movDuplicado); err != nil {
		t.Fatalf("desvincular el movimiento excluido falló: %v", err)
	}
	det, err := e.repo.PlanillaDeAsociacion(ctx, e.empresaID, e.asociacionID, "2026-07", decimal.Zero)
	if err != nil {
		t.Fatalf("PlanillaDeAsociacion: %v", err)
	}
	if len(det.Movimientos) != 1 {
		t.Errorf("movimientos = %d, se esperaba 1 después de soltar el duplicado", len(det.Movimientos))
	}
	igualMonto(t, "depositado", det.Depositado, "4000000")
}

// El panorama del canal: los totales no cuentan lo excluido y NINGUNA asociación desaparece.
//
// Las dos mitades de la prueba son distintas:
//   - sin `AND m.incluido`, el canal suma ₡6.000.000 que no existen y declara CONCILIADA una
//     asociación que no lo está;
//   - con el filtro puesto en el WHERE en vez de en el ON, la planilla cuyo único depósito
//     quedó excluido se cae del resultado y la asociación con el problema sería la única
//     invisible de la pantalla.
func TestPanoramaAsociacionesNoSumaExcluidosNiEscondeLaFila(t *testing.T) {
	e := nuevoEscenarioIncluido(t)

	pan, err := e.repo.PanoramaAsociaciones(context.Background(), e.empresaID, "2026-07", decimal.Zero)
	if err != nil {
		t.Fatalf("PanoramaAsociaciones: %v", err)
	}
	igualMonto(t, "depositado del canal", pan.Depositado, "4000000")
	if pan.Conciliadas != 0 {
		t.Errorf("conciliadas = %d, se esperaba 0: ninguna de las dos cuadra sin el duplicado", pan.Conciliadas)
	}
	filas := map[string]FilaAsociacion{}
	for _, f := range pan.Filas {
		filas[f.AsociacionID] = f
	}
	if len(filas) != 2 {
		t.Fatalf("filas = %d, se esperaban 2 asociaciones", len(filas))
	}

	uno := filas[e.asociacionID]
	igualMonto(t, "depositado de la asociación 1", uno.Depositado, "4000000")
	if uno.Depositos != 1 {
		t.Errorf("depósitos = %d, se esperaba 1: el conteo no puede contradecir al monto", uno.Depositos)
	}
	if uno.Estado != "CON_DIFERENCIA" {
		t.Errorf("estado de la asociación 1 = %s, se esperaba CON_DIFERENCIA", uno.Estado)
	}

	dos := filas[e.asociacion2]
	if dos.PlanillaID == "" {
		t.Fatalf("la asociación cuyo ÚNICO depósito quedó excluido desapareció del panorama: el filtro quedó en el WHERE y no en el ON")
	}
	igualMonto(t, "depositado de la asociación 2", dos.Depositado, "0")
	if dos.Depositos != 0 {
		t.Errorf("depósitos de la asociación 2 = %d, se esperaba 0", dos.Depositos)
	}
	if dos.Estado != "SIN_DEPOSITO" {
		t.Errorf("estado de la asociación 2 = %s, se esperaba SIN_DEPOSITO: esa es la señal para ir a buscar el depósito de verdad", dos.Estado)
	}
}
