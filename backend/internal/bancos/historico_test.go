package bancos

// CARGAR HISTÓRICO, de punta a punta y contra Postgres de verdad.
//
// Se corre contra la base porque lo que decide si la plata entra bien es el SQL: el anti-duplicado,
// el índice único PARCIAL de la mig 0086 y la transacción que crea una importación por cuenta. Un
// doble del repositorio probaría el doble. Igual que excluido_no_suma_test.go, reversa_test.go y
// segmento_sin_partida_test.go: cada corrida crea un ESQUEMA temporal con copias
// (`LIKE ... INCLUDING ALL`) de las tablas que se tocan, siembra ahí y lo borra al terminar. En
// `public` no se escribe NUNCA. Sin base de datos la prueba se OMITE (no falla).
//
// El escenario es el del Director Financiero: UN archivo con TRES cuentas y CUATRO meses, con la
// forma exacta del reporte que el sistema exporta (dos filas de título, encabezado en la 3, bandas
// de partida, subtotales, total y pie), una cuenta en dólares y otra nombrada por su IBAN.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/shared"
)

// Las tablas que toca la carga histórica y, de yapa, la reversa (que es la otra mitad del trabajo).
var tablasDelHistorico = []string{
	"banco", "cuenta_bancaria", "concepto", "clasificacion", "usuario",
	"carga_historica", "importacion", "movimiento_bancario",
	"movimiento_reporte_segmentacion", "cobro_cxc", "cxc_planilla_movimiento",
	"responsabilidad_periodo", "periodo_cierre", "acta_conciliacion", "auditoria_evento",
}

const (
	hBancoProm   = "aa000000-0000-4000-8000-000000000001"
	hBancoBP     = "aa000000-0000-4000-8000-000000000002"
	hBancoDavi   = "aa000000-0000-4000-8000-000000000003"
	hBancoAjeno  = "aa000000-0000-4000-8000-000000000004"
	hCtaProm     = "bb000000-0000-4000-8000-000000000001" // alias «Promerica VDP», CRC
	hCtaBP       = "bb000000-0000-4000-8000-000000000002" // alias «BP Negocios», CRC (el export le antepone el banco)
	hCtaDavi     = "bb000000-0000-4000-8000-000000000003" // alias «Davivienda Dólares», USD, con IBAN
	hCtaApagada  = "bb000000-0000-4000-8000-000000000004" // desactivada: se nombra distinto que «no existe»
	hCtaOtra     = "bb000000-0000-4000-8000-000000000005" // de OTRA empresa
	hConceptoIng = "cc000000-0000-4000-8000-000000000001"
	hConceptoGas = "cc000000-0000-4000-8000-000000000002"
	hPartidaDep  = "dd000000-0000-4000-8000-000000000001" // Ingresos › Depósito de Clientes
	hPartidaServ = "dd000000-0000-4000-8000-000000000002" // Gastos › Servicios Públicos
	hIBANDavi    = "CR73015201001026284066"
)

// baseDelHistorico levanta el esquema temporal y devuelve el repositorio que apunta a él.
func baseDelHistorico(t *testing.T) *pgRepository {
	t.Helper()
	dsn := os.Getenv("GPVDP_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://gpvdp:localdev@127.0.0.1:5432/gpvdp?sslmode=disable"
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("sin base de datos para la prueba de carga histórica (%v)", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("sin base de datos para la prueba de carga histórica (%v)", err)
	}

	esquema := fmt.Sprintf("prueba_historico_%d", time.Now().UnixNano())
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
	for _, tabla := range tablasDelHistorico {
		if _, err := admin.Exec(ctx,
			fmt.Sprintf(`CREATE TABLE %s.%s (LIKE public.%s INCLUDING ALL)`, esquema, tabla, tabla)); err != nil {
			t.Fatalf("copiar tabla %s: %v", tabla, err)
		}
	}

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

// sembrarCuentasYCatalogo deja las cuatro cuentas de la empresa (una apagada), la de la otra
// empresa y dos partidas del catálogo.
func sembrarCuentasYCatalogo(t *testing.T, repo *pgRepository) {
	t.Helper()
	ctx := context.Background()
	pasos := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Promerica')`,
			[]any{hBancoProm, empresaPrueba}},
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Banco Popular')`,
			[]any{hBancoBP, empresaPrueba}},
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Davivienda')`,
			[]any{hBancoDavi, empresaPrueba}},
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'BAC')`,
			[]any{hBancoAjeno, otraEmpresa}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'Promerica VDP')`,
			[]any{hCtaProm, empresaPrueba, hBancoProm}},
		// El alias NO nombra al banco: es el caso en que el export escribe «Banco Popular · BP Negocios».
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'BP Negocios')`,
			[]any{hCtaBP, empresaPrueba, hBancoBP}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias, iban)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'USD', 'Davivienda Dólares', $4)`,
			[]any{hCtaDavi, empresaPrueba, hBancoDavi, hIBANDavi}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias, activo)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'Promerica Vieja', false)`,
			[]any{hCtaApagada, empresaPrueba, hBancoProm}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'BAC de la otra empresa')`,
			[]any{hCtaOtra, otraEmpresa, hBancoAjeno}},
		{`INSERT INTO concepto (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Ingresos')`,
			[]any{hConceptoIng, empresaPrueba}},
		{`INSERT INTO concepto (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Gastos')`,
			[]any{hConceptoGas, empresaPrueba}},
		{`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'Depósito de Clientes')`,
			[]any{hPartidaDep, empresaPrueba, hConceptoIng}},
		{`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'Servicios Públicos')`,
			[]any{hPartidaServ, empresaPrueba, hConceptoGas}},
		{`INSERT INTO usuario (id, nombre, email, password_hash)
		  VALUES ($1::uuid, 'Dirección Financiera', 'df@prueba.local', 'x')`,
			[]any{usuarioPrueba}},
	}
	for _, p := range pasos {
		if _, err := repo.pool.Exec(ctx, p.q, p.args...); err != nil {
			t.Fatalf("sembrar cuentas y catálogo: %v", err)
		}
	}
}

// routerDeHistorico monta las rutas del histórico Y las de la reversa: la otra mitad del trabajo es
// poder re-subir un archivo corregido después de revertir, y eso solo se ve con las dos juntas.
func routerDeHistorico(repo *pgRepository, empresa string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := NewService(repo, shared.NewAudit(repo.pool, zap.NewNop()), zap.NewNop(), false)
	h := NewHandler(svc, zap.NewNop())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		auth.SetClaims(c, &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: usuarioPrueba},
			EmpresaID:        empresa,
		})
		c.Next()
	})
	r.POST("/v1/bancos/importaciones/historico", h.SubirHistorico)
	r.GET("/v1/bancos/importaciones/historico/:id", h.PreviewHistorico)
	r.POST("/v1/bancos/importaciones/historico/:id/confirmar", h.ConfirmarHistorico)
	r.GET("/v1/bancos/importaciones", h.Importaciones)
	r.POST("/v1/bancos/importaciones/:id/revertir", h.RevertirImportacion)
	r.POST("/v1/bancos/importaciones/:id/deshacer-reversa", h.DeshacerReversaImportacion)
	return r
}

// subirArchivo manda el .xlsx como multipart, igual que el navegador.
func subirArchivo(t *testing.T, r *gin.Engine, nombre string, contenido []byte) *httptest.ResponseRecorder {
	t.Helper()
	var cuerpo bytes.Buffer
	w := multipart.NewWriter(&cuerpo)
	parte, err := w.CreateFormFile("archivo", nombre)
	if err != nil {
		t.Fatalf("armar multipart: %v", err)
	}
	if _, err := parte.Write(contenido); err != nil {
		t.Fatalf("escribir el archivo: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("cerrar multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/bancos/importaciones/historico", &cuerpo)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func planDe(t *testing.T, rec *httptest.ResponseRecorder, esperado int) PlanHistorico {
	t.Helper()
	if rec.Code != esperado {
		t.Fatalf("status = %d, se esperaba %d — %s", rec.Code, esperado, rec.Body.String())
	}
	var plan PlanHistorico
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatalf("cuerpo ilegible: %v — %s", err, rec.Body.String())
	}
	return plan
}

func mensajeDe(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var cuerpo struct{ Code, Message string }
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo ilegible: %v — %s", err, rec.Body.String())
	}
	if cuerpo.Code == "ERROR_INTERNO" {
		t.Fatalf("salió como error interno (el centinela no está en el switch): %s", rec.Body.String())
	}
	return cuerpo.Message
}

// archivoDeTresCuentas arma el archivo del Director: tres cuentas, cuatro meses, con TODA la
// decoración del reporte del sistema y una cuenta nombrada por su IBAN.
//
// Las fechas van mezcladas a propósito: unas como FECHA de Excel de verdad (el caso que lo rompió),
// otras escritas a mano con día de un dígito, otras en ISO.
func archivoDeTresCuentas() libroPrueba {
	filas := [][]any{
		{"Movimientos"},
		{"Valle de Paz · del 01/01/2025 al 30/04/2025"},
		encabezadoExport,
		{"Ingresos › Depósito de Clientes"},
	}
	// Promerica VDP (colones), por el alias pelado — así lo escribe el export en 13 de 15 cuentas.
	fechasProm := []any{
		time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), // FECHA de Excel de verdad
		"7/2/2025",   // día y mes de un dígito
		"2025-03-04", // ISO
		"25/04/2025", // dd/mm/aaaa
	}
	for i, f := range fechasProm {
		filas = append(filas, filaExport(f, fmt.Sprintf("PROM-%d", i+1), nil, 100000.0+float64(i),
			"DEPOSITO CLIENTE", "Promerica VDP", "Ingresos", "Depósito de Clientes"))
	}
	filas = append(filas,
		[]any{"Subtotal Ingresos › Depósito de Clientes", nil, 0.0, 400006.0, 400006.0},
		[]any{},
		[]any{"Gastos › Servicios Públicos"})
	// BP Negocios: el export le antepone el banco porque el alias no lo nombra.
	for i, mes := range []int{1, 2, 3, 4} {
		filas = append(filas, filaExport(
			time.Date(2025, time.Month(mes), 10, 0, 0, 0, 0, time.UTC),
			fmt.Sprintf("BP-%d", i+1), 50000.0+float64(i), nil,
			"PAGO SERVICIO", "Banco Popular · BP Negocios", "Gastos", "Servicios Públicos"))
	}
	filas = append(filas,
		[]any{"Subtotal Gastos › Servicios Públicos", nil, 200006.0, 0.0, 200006.0},
		[]any{},
		[]any{"Sin clasificar"})
	// Davivienda Dólares: nombrada por el IBAN, con espacios (como se pega del banco).
	ibanConEspacios := "CR73 0152 0100 1026 2840 66"
	for i, mes := range []int{1, 2, 3, 4} {
		filas = append(filas, filaExport(
			time.Date(2025, time.Month(mes), 20, 0, 0, 0, 0, time.UTC),
			fmt.Sprintf("DAV-%d", i+1), nil, 1000.0+float64(i),
			"WIRE TRANSFER", ibanConEspacios, "Sin clasificar", "Sin clasificar"))
	}
	filas = append(filas,
		[]any{"Subtotal Sin clasificar", nil, 0.0, 4006.0, 2000000.0},
		[]any{},
		[]any{"TOTAL · 12 movimiento(s)", nil, 200006.0, 404012.0, 2600012.0},
		[]any{},
		[]any{"Generado por GPVDP ERP · Valle de Paz · 24/09/2026 10:12"})

	return libroPrueba{numFmt: 14, filas: filas}
}

// El caso completo: UN archivo, TRES cuentas, CUATRO meses, cada movimiento en su cuenta.
func TestUnArchivoConTresCuentasYCuatroMesesEntraCompleto(t *testing.T) {
	repo := baseDelHistorico(t)
	sembrarCuentasYCatalogo(t, repo)
	r := routerDeHistorico(repo, empresaPrueba)
	ctx := context.Background()

	plan := planDe(t, subirArchivo(t, r, "HISTORICO 2025.xlsx", archivoDeTresCuentas().bytes(t)), http.StatusCreated)

	// ── La previsualización: qué entendió, ANTES de escribir nada ───────────
	if plan.Totales.Filas != 12 {
		t.Errorf("filas de movimiento = %d, se esperaban 12 — %+v", plan.Totales.Filas, plan.Errores)
	}
	// 3 bandas + 3 subtotales + el total + el pie.
	if plan.Totales.LineasDeFormato != 8 {
		t.Errorf("líneas de formato = %d, se esperaban 8", plan.Totales.LineasDeFormato)
	}
	if plan.Totales.Errores != 0 {
		t.Fatalf("errores = %d: %+v", plan.Totales.Errores, plan.Errores)
	}
	if len(plan.CuentasNoResueltas) != 0 {
		t.Fatalf("cuentas sin resolver: %+v", plan.CuentasNoResueltas)
	}
	if len(plan.Cuentas) != 3 {
		t.Fatalf("cuentas = %d, se esperaban 3 — %+v", len(plan.Cuentas), plan.Cuentas)
	}
	porID := map[string]ResumenCuentaHistorico{}
	for _, c := range plan.Cuentas {
		porID[c.CuentaBancariaID] = c
	}
	for _, id := range []string{hCtaProm, hCtaBP, hCtaDavi} {
		c, ok := porID[id]
		if !ok {
			t.Fatalf("falta la cuenta %s en el resumen: %+v", id, plan.Cuentas)
		}
		if c.Filas != 4 || c.Nuevas != 4 {
			t.Errorf("%s: filas=%d nuevas=%d, se esperaban 4 y 4", c.Cuenta, c.Filas, c.Nuevas)
		}
		if len(c.Meses) != 4 {
			t.Errorf("%s: meses=%v, se esperaban los cuatro", c.Cuenta, c.Meses)
		}
		if c.FechaDesde == "" || c.FechaHasta == "" || c.FechaDesde >= c.FechaHasta {
			t.Errorf("%s: rango de fechas raro (%s..%s)", c.Cuenta, c.FechaDesde, c.FechaHasta)
		}
	}
	// La de dólares: sin partida en el archivo y el monto en colones queda provisional.
	if d := porID[hCtaDavi]; d.SinPartida != 4 || d.SinTipoCambio != 4 || d.Moneda != "USD" {
		t.Errorf("Davivienda: sin_partida=%d sin_tipo_cambio=%d moneda=%s — se esperaba 4, 4, USD",
			d.SinPartida, d.SinTipoCambio, d.Moneda)
	}
	// Y la nombrada por IBAN con espacios calzó igual.
	if d := porID[hCtaDavi]; len(d.NombresEnArchivo) != 1 || !strings.HasPrefix(d.NombresEnArchivo[0], "CR73") {
		t.Errorf("Davivienda: nombres en el archivo = %v", d.NombresEnArchivo)
	}
	if plan.Aplicado {
		t.Error("la previsualización dice que ya se aplicó: no escribió nada todavía")
	}
	var antes int
	if err := repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM movimiento_bancario`).Scan(&antes); err != nil {
		t.Fatalf("contar movimientos: %v", err)
	}
	if antes != 0 {
		t.Fatalf("previsualizar escribió %d movimiento(s)", antes)
	}

	// ── Confirmar ───────────────────────────────────────────────────────────
	rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/historico/"+plan.CargaID+"/confirmar", "")
	aplicado := planDe(t, rec, http.StatusOK)
	if aplicado.Insertados != 12 {
		t.Fatalf("insertados = %d, se esperaban 12 — %s", aplicado.Insertados, rec.Body.String())
	}

	// UNA importación POR CUENTA, con el mismo nombre de archivo: así «Cargas hechas» las muestra
	// juntas y cada una se puede revertir sola.
	rows, err := repo.pool.Query(ctx,
		`SELECT cuenta_bancaria_id::text, nombre_archivo, source_file_hash, estado, banco FROM importacion`)
	if err != nil {
		t.Fatalf("leer importaciones: %v", err)
	}
	defer rows.Close()
	hashes, archivos, cuentas := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var cuenta, archivo, hash, estado, banco string
		if err := rows.Scan(&cuenta, &archivo, &hash, &estado, &banco); err != nil {
			t.Fatalf("scan importación: %v", err)
		}
		if estado != "CONFIRMADA" {
			t.Errorf("la importación de %s quedó en %q", cuenta, estado)
		}
		if banco == "" {
			t.Errorf("la importación de %s no dice de qué banco es", cuenta)
		}
		hashes[hash], archivos[archivo], cuentas[cuenta] = true, true, true
	}
	if len(cuentas) != 3 {
		t.Errorf("importaciones creadas = %d, se esperaban 3 (una por cuenta)", len(cuentas))
	}
	if len(hashes) != 1 || len(archivos) != 1 {
		t.Errorf("las tres cargas deberían compartir nombre y huella del archivo: %v %v", archivos, hashes)
	}

	// Cada movimiento en SU cuenta, con la partida del archivo y marcado como histórico.
	comprobar := []struct {
		cuenta   string
		partida  any
		estado   string
		cantidad int
	}{
		{hCtaProm, hPartidaDep, "REVISADO", 4},
		{hCtaBP, hPartidaServ, "REVISADO", 4},
		{hCtaDavi, nil, "NO_IDENTIFICADO", 4},
	}
	for _, c := range comprobar {
		var n int
		q := `SELECT COUNT(*) FROM movimiento_bancario
		       WHERE cuenta_bancaria_id = $1::uuid AND estado_clasificacion = $2
		         AND origen_historico AND incluido
		         AND clasificacion_id IS NOT DISTINCT FROM $3::uuid`
		if err := repo.pool.QueryRow(ctx, q, c.cuenta, c.estado, c.partida).Scan(&n); err != nil {
			t.Fatalf("contar movimientos de %s: %v", c.cuenta, err)
		}
		if n != c.cantidad {
			t.Errorf("cuenta %s: %d movimientos con la partida y el estado esperados, se esperaban %d",
				c.cuenta, n, c.cantidad)
		}
	}
	// `origen_historico` es la columna que estaba muerta desde la mig 0004: si nadie la escribe,
	// mañana no hay forma de distinguir esto de un estado de cuenta del banco.
	var historicos int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM movimiento_bancario WHERE origen_historico`).Scan(&historicos); err != nil {
		t.Fatalf("contar históricos: %v", err)
	}
	if historicos != 12 {
		t.Errorf("movimientos marcados como históricos = %d, se esperaban 12", historicos)
	}

	// La fecha que Excel guardaba como fecha de verdad quedó BIEN (no «07-03-26» ni invertida).
	var enero int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM movimiento_bancario WHERE fecha = DATE '2025-01-15' AND cuenta_bancaria_id = $1::uuid`,
		hCtaProm).Scan(&enero); err != nil {
		t.Fatalf("buscar el 15 de enero: %v", err)
	}
	if enero != 1 {
		t.Errorf("el movimiento del 15/01/2025 no quedó con esa fecha (n=%d)", enero)
	}

	// ── Subir el MISMO archivo otra vez no duplica ni un movimiento ─────────
	plan2 := planDe(t, subirArchivo(t, r, "HISTORICO 2025.xlsx", archivoDeTresCuentas().bytes(t)), http.StatusCreated)
	if plan2.Totales.YaExisten != 12 || plan2.Totales.Nuevas != 0 {
		t.Errorf("segunda subida: ya_existen=%d nuevas=%d, se esperaban 12 y 0",
			plan2.Totales.YaExisten, plan2.Totales.Nuevas)
	}
	rec2 := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/historico/"+plan2.CargaID+"/confirmar", "")
	if rec2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("confirmar la segunda subida devolvió %d — %s", rec2.Code, rec2.Body.String())
	}
	if msg := mensajeDe(t, rec2); !strings.Contains(msg, "ninguna fila para cargar") {
		t.Errorf("el mensaje debería explicar que no quedó nada por cargar; dice %q", msg)
	}
	var despues int
	if err := repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM movimiento_bancario`).Scan(&despues); err != nil {
		t.Fatalf("contar movimientos: %v", err)
	}
	if despues != 12 {
		t.Fatalf("después de subir dos veces hay %d movimientos: se duplicó la plata", despues)
	}
}

// Una cuenta que no existe se REPORTA con el nombre exacto que traía, y NO se crea.
//
// El mensaje tiene que nombrar lo que leyó: el otro importador tiene heredado un «la fila no dice
// de qué cuenta es» que sale incluso cuando la fila sí lo dice, y eso manda a buscar el problema
// donde no está.
func TestUnaCuentaQueNoExisteSeReportaConSuNombreYNoSeCrea(t *testing.T) {
	repo := baseDelHistorico(t)
	sembrarCuentasYCatalogo(t, repo)
	r := routerDeHistorico(repo, empresaPrueba)
	ctx := context.Background()

	libro := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("15/01/2025", "A-1", nil, "100000.00", "DEPOSITO", "Promerica VDP", "Ingresos", "Depósito de Clientes"),
		filaExport("16/01/2025", "A-2", nil, "200000.00", "DEPOSITO", "Promerica Colinas", "Ingresos", "Depósito de Clientes"),
		filaExport("17/01/2025", "A-3", nil, "300000.00", "DEPOSITO", "Promerica Colinas", "Ingresos", "Depósito de Clientes"),
		// Una cuenta que EXISTE pero está desactivada: no es lo mismo y no se arregla igual.
		filaExport("18/01/2025", "A-4", nil, "400000.00", "DEPOSITO", "Promerica Vieja", "Ingresos", "Depósito de Clientes"),
		// Y una fila que de verdad no dice de qué cuenta es.
		filaExport("19/01/2025", "A-5", nil, "500000.00", "DEPOSITO", "", "Ingresos", "Depósito de Clientes"),
	}}
	plan := planDe(t, subirArchivo(t, r, "MEZCLA.xlsx", libro.bytes(t)), http.StatusCreated)

	if plan.Totales.SinCuenta != 4 {
		t.Errorf("filas sin cuenta = %d, se esperaban 4", plan.Totales.SinCuenta)
	}
	porNombre := map[string]CuentaNoResuelta{}
	for _, c := range plan.CuentasNoResueltas {
		porNombre[c.NombreEnArchivo] = c
	}
	colinas, ok := porNombre["Promerica Colinas"]
	if !ok {
		t.Fatalf("no se reportó «Promerica Colinas» con su nombre exacto: %+v", plan.CuentasNoResueltas)
	}
	if colinas.Filas != 2 {
		t.Errorf("«Promerica Colinas»: filas = %d, se esperaban 2", colinas.Filas)
	}
	if !strings.Contains(colinas.Motivo, "Promerica Colinas") || !strings.Contains(colinas.Motivo, "creala") {
		t.Errorf("el motivo tiene que nombrar lo que leyó y decir qué hacer; dice %q", colinas.Motivo)
	}
	if colinas.PrimeraLinea != 3 {
		t.Errorf("primera línea = %d, se esperaba 3", colinas.PrimeraLinea)
	}
	if vieja := porNombre["Promerica Vieja"]; !strings.Contains(vieja.Motivo, "desactivada") {
		t.Errorf("una cuenta desactivada tiene que decirse así, no «no existe»; dice %q", vieja.Motivo)
	}
	if vacia := porNombre[""]; !strings.Contains(vacia.Motivo, "no dice de qué cuenta es") {
		t.Errorf("la fila sin cuenta debería decirlo; dice %q", vacia.Motivo)
	}
	// El aviso de una línea tiene que nombrar lo que falta crear.
	if !strings.Contains(plan.Aviso, "Promerica Colinas") {
		t.Errorf("el aviso no nombra la cuenta que falta: %q", plan.Aviso)
	}

	// Confirmar carga SOLO lo que se pudo resolver, y NO crea ninguna cuenta.
	rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/historico/"+plan.CargaID+"/confirmar", "")
	aplicado := planDe(t, rec, http.StatusOK)
	if aplicado.Insertados != 1 {
		t.Errorf("insertados = %d, se esperaba 1 (solo la fila de Promerica VDP)", aplicado.Insertados)
	}
	var cuentas int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM cuenta_bancaria WHERE empresa_id = $1::uuid`, empresaPrueba).Scan(&cuentas); err != nil {
		t.Fatalf("contar cuentas: %v", err)
	}
	if cuentas != 4 {
		t.Errorf("cuentas de la empresa = %d, se esperaban 4: la carga inventó una cuenta", cuentas)
	}
}

// Una partida que no está en el catálogo NO tira la plata: el movimiento se carga SIN partida y se
// reporta cuántos y qué nombres faltaron.
//
// Perder la plata por no encontrar una etiqueta sería peor que cargarla sin etiqueta: lo segundo se
// arregla después desde la pantalla de clasificar, lo primero no se nota hasta que no cuadra el año.
func TestUnaPartidaQueNoExisteCargaElMovimientoSinPartidaYLoReporta(t *testing.T) {
	repo := baseDelHistorico(t)
	sembrarCuentasYCatalogo(t, repo)
	r := routerDeHistorico(repo, empresaPrueba)
	ctx := context.Background()

	libro := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("15/01/2025", "B-1", nil, "100000.00", "DEPOSITO", "Promerica VDP", "Ingresos", "Depósito de Clientes"),
		filaExport("16/01/2025", "B-2", "250000.00", nil, "COMPRA", "Promerica VDP", "Gastos", "Papelería y Útiles"),
		filaExport("17/01/2025", "B-3", "180000.00", nil, "COMPRA", "Promerica VDP", "", "Papelería y Útiles"),
	}}
	plan := planDe(t, subirArchivo(t, r, "PARTIDAS.xlsx", libro.bytes(t)), http.StatusCreated)

	if plan.Totales.PartidaDesconocida != 2 {
		t.Errorf("partidas desconocidas = %d, se esperaban 2", plan.Totales.PartidaDesconocida)
	}
	if len(plan.PartidasFaltantes) == 0 {
		t.Fatal("no se reportó QUÉ partida falta: sin el nombre no se puede crear")
	}
	juntas := strings.Join(plan.PartidasFaltantes, " | ")
	if !strings.Contains(juntas, "Papelería y Útiles") {
		t.Errorf("las partidas faltantes no nombran la que falta: %q", juntas)
	}

	rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/historico/"+plan.CargaID+"/confirmar", "")
	aplicado := planDe(t, rec, http.StatusOK)
	// LAS TRES entran: la plata no se pierde por una etiqueta que falta.
	if aplicado.Insertados != 3 {
		t.Fatalf("insertados = %d, se esperaban 3: la plata no se tira por una etiqueta", aplicado.Insertados)
	}
	var sinPartida int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM movimiento_bancario
		  WHERE clasificacion_id IS NULL AND estado_clasificacion = 'NO_IDENTIFICADO'`).Scan(&sinPartida); err != nil {
		t.Fatalf("contar sin partida: %v", err)
	}
	if sinPartida != 2 {
		t.Errorf("movimientos sin partida = %d, se esperaban 2", sinPartida)
	}
	var conPartida int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM movimiento_bancario WHERE clasificacion_id = $1::uuid AND estado_clasificacion = 'REVISADO'`,
		hPartidaDep).Scan(&conPartida); err != nil {
		t.Fatalf("contar con partida: %v", err)
	}
	if conPartida != 1 {
		t.Errorf("movimientos con la partida del archivo = %d, se esperaba 1", conPartida)
	}
}

// Un mes CERRADO rechaza la carga, y el rechazo dice QUÉ mes.
func TestUnMesCerradoRechazaLaCargaHistorica(t *testing.T) {
	repo := baseDelHistorico(t)
	sembrarCuentasYCatalogo(t, repo)
	r := routerDeHistorico(repo, empresaPrueba)
	ctx := context.Background()

	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO periodo_cierre (empresa_id, anio, mes) VALUES ($1::uuid, 2025, 2)`, empresaPrueba); err != nil {
		t.Fatalf("cerrar febrero: %v", err)
	}

	libro := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("15/01/2025", "C-1", nil, "100000.00", "DEPOSITO", "Promerica VDP", "Ingresos", "Depósito de Clientes"),
		filaExport("10/02/2025", "C-2", nil, "200000.00", "DEPOSITO", "Promerica VDP", "Ingresos", "Depósito de Clientes"),
	}}
	plan := planDe(t, subirArchivo(t, r, "CERRADO.xlsx", libro.bytes(t)), http.StatusCreated)

	rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/historico/"+plan.CargaID+"/confirmar", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, se esperaba 422 — %s", rec.Code, rec.Body.String())
	}
	msg := mensajeDe(t, rec)
	if !strings.Contains(msg, "2025-02") {
		t.Errorf("el rechazo tiene que decir QUÉ mes estorba; dice %q", msg)
	}
	// Ni una fila escrita: rechazar a mitad de camino sería peor que no empezar.
	var n int
	if err := repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM movimiento_bancario`).Scan(&n); err != nil {
		t.Fatalf("contar movimientos: %v", err)
	}
	if n != 0 {
		t.Errorf("se escribieron %d movimiento(s) pese al rechazo", n)
	}

	// Lo mismo con un acta de conciliación FIRMADA de esa cuenta.
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO acta_conciliacion (empresa_id, cuenta_bancaria_id, anio, mes, saldo_banco, saldo_libros,
		                               ajuste_partidas, firmado_en, firmado_por)
		VALUES ($1::uuid, $2::uuid, 2025, 1, 0, 0, 0, now(), $3::uuid)`,
		empresaPrueba, hCtaProm, usuarioPrueba); err != nil {
		t.Fatalf("firmar el acta: %v", err)
	}
	soloEnero := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("15/01/2025", "C-1", nil, "100000.00", "DEPOSITO", "Promerica VDP", "Ingresos", "Depósito de Clientes"),
	}}
	plan2 := planDe(t, subirArchivo(t, r, "ACTA.xlsx", soloEnero.bytes(t)), http.StatusCreated)
	rec2 := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/historico/"+plan2.CargaID+"/confirmar", "")
	if rec2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("con acta firmada: status = %d, se esperaba 422 — %s", rec2.Code, rec2.Body.String())
	}
	if msg := mensajeDe(t, rec2); !strings.Contains(msg, "Promerica VDP 2025-01") {
		t.Errorf("el rechazo tiene que nombrar la cuenta y el mes del acta; dice %q", msg)
	}
}

// AISLAMIENTO. Nada de lo que haga una empresa puede tocar la cuenta de otra, y una carga histórica
// de otra empresa no existe para esta (404, nunca 403: un 403 confirmaría que existe).
func TestNoSePuedeCargarHistoricoContraLaCuentaDeOtraEmpresa(t *testing.T) {
	repo := baseDelHistorico(t)
	sembrarCuentasYCatalogo(t, repo)
	r := routerDeHistorico(repo, empresaPrueba)
	ctx := context.Background()

	libro := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("15/01/2025", "D-1", nil, "999999.99", "DEPOSITO", "BAC de la otra empresa", "Ingresos", "Depósito de Clientes"),
	}}
	plan := planDe(t, subirArchivo(t, r, "AJENA.xlsx", libro.bytes(t)), http.StatusCreated)

	if len(plan.Cuentas) != 0 {
		t.Fatalf("resolvió una cuenta de otra empresa: %+v", plan.Cuentas)
	}
	if len(plan.CuentasNoResueltas) != 1 || plan.CuentasNoResueltas[0].NombreEnArchivo != "BAC de la otra empresa" {
		t.Fatalf("no se reportó como cuenta desconocida: %+v", plan.CuentasNoResueltas)
	}
	rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/historico/"+plan.CargaID+"/confirmar", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, se esperaba 422 — %s", rec.Code, rec.Body.String())
	}
	var n int
	if err := repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM movimiento_bancario`).Scan(&n); err != nil {
		t.Fatalf("contar movimientos: %v", err)
	}
	if n != 0 {
		t.Fatalf("entraron %d movimiento(s) en la cuenta de otra empresa", n)
	}

	// Y la carga de ESTA empresa no se ve desde la otra.
	rOtra := routerDeHistorico(repo, otraEmpresa)
	recOtra := llamar(t, rOtra, http.MethodPost,
		"/v1/bancos/importaciones/historico/"+plan.CargaID+"/confirmar", "")
	if recOtra.Code != http.StatusNotFound {
		t.Fatalf("una carga de otra empresa devolvió %d, se esperaba 404 — %s", recOtra.Code, recOtra.Body.String())
	}
}

// PARTE 1, de punta a punta: la reversa LIBERA la línea, y a cambio deshacerla se bloquea.
//
// Hasta la mig 0086 una fila revertida seguía ocupando su `natural_key`, así que corregir el
// archivo y volver a subirlo A LA MISMA CUENTA insertaba CERO y la pantalla no tenía cómo
// explicarlo. La otra mitad de la decisión del Director es que, una vez re-importadas esas líneas,
// «Deshacer la reversa» quede bloqueado: si no, la misma plata queda contada dos veces.
func TestLaReversaLiberaLaLineaYDespuesElDeshacerSeBloquea(t *testing.T) {
	repo := baseDelHistorico(t)
	sembrarCuentasYCatalogo(t, repo)
	r := routerDeHistorico(repo, empresaPrueba)
	ctx := context.Background()

	archivo := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("15/01/2025", "E-1", nil, "100000.00", "DEPOSITO", "Promerica VDP", "Ingresos", "Depósito de Clientes"),
		filaExport("16/01/2025", "E-2", nil, "200000.00", "DEPOSITO", "Promerica VDP", "Ingresos", "Depósito de Clientes"),
		filaExport("17/01/2025", "E-3", "300000.00", nil, "PAGO", "Promerica VDP", "Gastos", "Servicios Públicos"),
	}}

	// 1. Cargar.
	plan := planDe(t, subirArchivo(t, r, "ENERO.xlsx", archivo.bytes(t)), http.StatusCreated)
	aplicado := planDe(t, llamar(t, r, http.MethodPost,
		"/v1/bancos/importaciones/historico/"+plan.CargaID+"/confirmar", ""), http.StatusOK)
	if aplicado.Insertados != 3 {
		t.Fatalf("insertados = %d, se esperaban 3", aplicado.Insertados)
	}
	impID := aplicado.Cuentas[0].ImportacionID

	// 2. Revertir esa carga (se cargó en la cuenta equivocada, digamos).
	rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impID+"/revertir",
		`{"motivo":"me equivoqué de cuenta"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("la reversa no pasó: %d — %s", rec.Code, rec.Body.String())
	}

	// 3. Volver a subir el MISMO archivo a la MISMA cuenta: ANTES de la mig 0086 esto insertaba 0.
	plan2 := planDe(t, subirArchivo(t, r, "ENERO.xlsx", archivo.bytes(t)), http.StatusCreated)
	if plan2.Totales.Nuevas != 3 || plan2.Totales.YaExisten != 0 {
		t.Fatalf("re-subida: nuevas=%d ya_existen=%d — la reversa no liberó las líneas",
			plan2.Totales.Nuevas, plan2.Totales.YaExisten)
	}
	rec2 := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/historico/"+plan2.CargaID+"/confirmar", "")
	aplicado2 := planDe(t, rec2, http.StatusOK)
	if aplicado2.Insertados != 3 {
		t.Fatalf("re-subida: insertados = %d, se esperaban 3 — %s", aplicado2.Insertados, rec2.Body.String())
	}

	// Los libros cuentan TRES movimientos, no seis: las viejas siguen ahí pero excluidas.
	var incluidos, total int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE incluido), COUNT(*) FROM movimiento_bancario`).
		Scan(&incluidos, &total); err != nil {
		t.Fatalf("contar movimientos: %v", err)
	}
	if incluidos != 3 || total != 6 {
		t.Fatalf("incluidos=%d total=%d, se esperaban 3 y 6 (nada se borra, pero la plata cuenta una vez)",
			incluidos, total)
	}

	// 4. Y ahora deshacer la reversa está BLOQUEADO, con cuántas líneas y de qué carga.
	lista := llamar(t, r, http.MethodGet, "/v1/bancos/importaciones", "")
	if lista.Code != http.StatusOK {
		t.Fatalf("listar cargas: %d — %s", lista.Code, lista.Body.String())
	}
	var pagina ListaImportaciones
	if err := json.Unmarshal(lista.Body.Bytes(), &pagina); err != nil {
		t.Fatalf("cuerpo ilegible: %v — %s", err, lista.Body.String())
	}
	var revertida *ImportacionItem
	for i := range pagina.Items {
		if pagina.Items[i].ID == impID {
			revertida = &pagina.Items[i]
		}
	}
	if revertida == nil {
		t.Fatalf("la carga revertida no aparece en el listado: %+v", pagina.Items)
	}
	if revertida.PuedeDeshacerReversa {
		t.Error("el botón «deshacer» sale habilitado: al apretarlo duplicaría la plata")
	}
	if revertida.Bloqueos.LineasReimportadas != 3 {
		t.Errorf("líneas reimportadas = %d, se esperaban 3", revertida.Bloqueos.LineasReimportadas)
	}
	if !strings.Contains(revertida.RazonNoRevertir, "3 línea(s)") {
		t.Errorf("la razón tiene que decir CUÁNTAS líneas; dice %q", revertida.RazonNoRevertir)
	}
	if !strings.Contains(revertida.RazonNoRevertir, "ENERO.xlsx") {
		t.Errorf("la razón tiene que decir de qué carga nueva son; dice %q", revertida.RazonNoRevertir)
	}

	// Y el servidor dice lo mismo que la pantalla: apretar igual devuelve 422, no un 500 ni un OK.
	recDeshacer := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impID+"/deshacer-reversa", `{}`)
	if recDeshacer.Code != http.StatusUnprocessableEntity {
		t.Fatalf("deshacer devolvió %d, se esperaba 422 — %s", recDeshacer.Code, recDeshacer.Body.String())
	}
	if msg := mensajeDe(t, recDeshacer); !strings.Contains(msg, "3 línea(s)") {
		t.Errorf("el 422 tiene que decir cuántas líneas; dice %q", msg)
	}
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE incluido) FROM movimiento_bancario`).Scan(&incluidos); err != nil {
		t.Fatalf("contar incluidos: %v", err)
	}
	if incluidos != 3 {
		t.Errorf("después del intento de deshacer hay %d incluidos: la plata se duplicó", incluidos)
	}
}

// El índice de cuentas no se puede envenenar solo.
//
// Para aceptar «Banco Popular · BP Negocios» el índice inventa grafías compuestas. Con un solo mapa
// esa tolerancia se vuelve en contra: banco «BN» + alias «Jardines Colones» da la MISMA cadena que
// el alias real «BN Jardines Colones» de otra cuenta, y la colisión marcaría ambigua una clave que
// estaba perfecta. La cuenta buena dejaría de resolverse por una grafía que nadie escribió.
func TestElAliasGanaSobreLaGrafiaCompuestaQueInventaElIndice(t *testing.T) {
	cuentas := []CuentaHistorica{
		{ID: "cuenta-alias", Alias: "BN Jardines Colones", Banco: "Banco Nacional", Moneda: "CRC", Activo: true},
		{ID: "cuenta-compuesta", Alias: "Jardines Colones", Banco: "BN", Moneda: "CRC", Activo: true},
		{ID: "cuenta-bp", Alias: "BP Negocios", Banco: "Banco Popular", Moneda: "CRC", Activo: true},
		{ID: "cuenta-iban", Alias: "Davivienda Dólares", Banco: "Davivienda", Moneda: "USD",
			IBAN: hIBANDavi, Activo: true},
	}
	idx := indiceDeCuentas(cuentas)

	casos := []struct{ texto, esperado string }{
		{"BN Jardines Colones", "cuenta-alias"},      // el alias pelado gana
		{"Jardines Colones", "cuenta-compuesta"},     // el otro alias, intacto
		{"Banco Popular · BP Negocios", "cuenta-bp"}, // lo que escribe el export
		{"BP Negocios", "cuenta-bp"},
		{"CR73 0152 0100 1026 2840 66", "cuenta-iban"}, // IBAN con espacios, como se pega del banco
	}
	for _, c := range casos {
		got, motivo := idx.resolver(c.texto)
		if motivo != "" {
			t.Errorf("%q no se resolvió: %s", c.texto, motivo)
			continue
		}
		if got.ID != c.esperado {
			t.Errorf("%q resolvió a %s, se esperaba %s", c.texto, got.ID, c.esperado)
		}
	}

	// Y la ambigüedad de verdad —dos cuentas con el MISMO alias normalizado— se sigue detectando:
	// cargar en la cuenta equivocada es el incidente que originó la reversa.
	ambiguo := indiceDeCuentas([]CuentaHistorica{
		{ID: "a", Alias: "BAC Religiosa", Banco: "BAC", Moneda: "CRC", Activo: true},
		{ID: "b", Alias: "bac religiosa", Banco: "BAC", Moneda: "CRC", Activo: true},
	})
	if _, motivo := ambiguo.resolver("BAC Religiosa"); !strings.Contains(motivo, "más de una cuenta") {
		t.Errorf("dos cuentas con el mismo alias no se detectaron como ambiguas; motivo = %q", motivo)
	}
}

// El borde HTTP de los centinelas NUEVOS. El modo de falla de este paquete es que uno que el switch
// no conoce sale como 500 «error interno» y el usuario no puede distinguir un archivo rechazado —con
// instrucciones de qué hacer— de una caída del servidor.
func TestResponderErrorTraduceLosCentinelasDelHistorico(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, zap.NewNop())

	casos := []struct {
		nombre  string
		err     error
		status  int
		enTexto string
	}{
		{"sin encabezado", ErrHistoricoSinEncabezado, http.StatusUnprocessableEntity, "encabezado"},
		{"archivo vacío", ErrHistoricoVacio, http.StatusUnprocessableEntity, "ninguna fila"},
		{"demasiadas filas", ErrHistoricoDemasiadasFilas, http.StatusUnprocessableEntity, "partilo por año o por cuenta"},
		{"nada que cargar", ErrHistoricoSinNadaQueCargar, http.StatusUnprocessableEntity, "ninguna fila para cargar"},
		{"carga inexistente", ErrCargaHistoricaNoEncontrada, http.StatusNotFound, "no existe"},
		{"ya confirmada", ErrCargaHistoricaYaConfirmada, http.StatusConflict, "ya se confirmó"},
		{
			"mes cerrado",
			&HistoricoBloqueadoError{PeriodosCerrados: []string{"2025-02"}},
			http.StatusUnprocessableEntity, "2025-02",
		},
		{
			"acta firmada",
			&HistoricoBloqueadoError{ActasFirmadas: []string{"Promerica VDP 2025-01"}},
			http.StatusUnprocessableEntity, "Promerica VDP 2025-01",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/bancos/importaciones/historico", nil)

			h.responderError(ctx, c.err, "test")

			if rec.Code != c.status {
				t.Fatalf("status = %d, se esperaba %d — %s", rec.Code, c.status, rec.Body.String())
			}
			var cuerpo struct{ Code, Message string }
			if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("cuerpo ilegible: %v — %s", err, rec.Body.String())
			}
			if cuerpo.Code == "ERROR_INTERNO" {
				t.Fatalf("el centinela salió como error interno: %s", rec.Body.String())
			}
			if !strings.Contains(cuerpo.Message, c.enTexto) {
				t.Errorf("el mensaje debería decir %q; dice %q", c.enTexto, cuerpo.Message)
			}
			// El prefijo del paquete es útil en los logs y ruido en la pantalla.
			if strings.HasPrefix(cuerpo.Message, "bancos: ") {
				t.Errorf("el mensaje llega con el prefijo del paquete: %q", cuerpo.Message)
			}
		})
	}
}

// LO QUE PROMETE EL PREVISUALIZAR ES LO QUE QUEDA EN LA BASE.
//
// Es la única propiedad que de verdad importa acá: este importador CREA movimientos, y si el
// resumen que el Director aprueba dice una cifra y la tabla termina con otra, nadie se entera hasta
// que no cuadra el año.
//
// Se rompía por dos lados y los dos se miden contra Postgres de verdad, no contra el resumen:
//
//	· Montos con más de dos decimales. Los libros son `numeric(16,2)` y Postgres redondeaba AL
//	  INSERTAR, después de que el plan ya había sumado con todos los decimales: tres filas de 1,005
//	  daban «3,02» en la previsualización y 3,03 en la tabla. Y una fila de 0,004 —que redondeada
//	  es cero— entraba como un movimiento de ¢0,00 sin decir nada.
//	· Un monto con ceros de más. No entraba en `numeric(16,2)`, el INSERT fallaba, la transacción
//	  se iba ENTERA y el usuario recibía «error interno» sin una sola línea que mirar: las filas
//	  buenas tampoco se cargaban.
func TestLaSumaDelPreviewEsLaQueQuedaEnLaBase(t *testing.T) {
	repo := baseDelHistorico(t)
	sembrarCuentasYCatalogo(t, repo)
	r := routerDeHistorico(repo, empresaPrueba)
	ctx := context.Background()

	libro := libroPrueba{filas: [][]any{
		encabezadoExport,
		filaExport("10/01/2025", "DEC-001", 1.005, nil, "TRES DECIMALES", "Promerica VDP", "", ""),
		filaExport("10/01/2025", "DEC-002", 1.005, nil, "TRES DECIMALES", "Promerica VDP", "", ""),
		filaExport("10/01/2025", "DEC-003", 1.005, nil, "TRES DECIMALES", "Promerica VDP", "", ""),
		filaExport("10/01/2025", "DEC-004", 0.004, nil, "SE REDONDEA A CERO", "Promerica VDP", "", ""),
		filaExport("11/01/2025", "BIG-001", "99999999999999999.99", nil, "CEROS DE MAS", "Promerica VDP", "", ""),
		filaExport("12/01/2025", "OK-001", nil, 250.25, "NORMAL", "Promerica VDP", "", ""),
	}}

	plan := planDe(t, subirArchivo(t, r, "DECIMALES.xlsx", libro.bytes(t)), http.StatusCreated)

	// Las dos filas imposibles salen como ERRORES con su línea, no calladas y no tirando el archivo.
	if plan.Totales.Errores != 2 {
		t.Fatalf("errores = %d, se esperaban 2 (la de ¢0 y la de ceros de más): %+v",
			plan.Totales.Errores, plan.Errores)
	}
	motivos := plan.Errores[0].Motivo + " · " + plan.Errores[1].Motivo
	if !strings.Contains(motivos, "no trae monto") || !strings.Contains(motivos, "no entra en los libros") {
		t.Errorf("motivos = %q, se esperaban los dos: «no trae monto» y «no entra en los libros»", motivos)
	}
	if len(plan.Cuentas) != 1 {
		t.Fatalf("cuentas = %+v, se esperaba solo Promerica VDP", plan.Cuentas)
	}
	prometidoDeb := plan.Cuentas[0].DebitosNuevos
	prometidoCre := plan.Cuentas[0].CreditosNuevos
	if prometidoDeb != "3.03" {
		t.Errorf("débitos prometidos = %s, se esperaba 3.03 (1,01 × 3, que es como quedan en la base)", prometidoDeb)
	}

	// ── Confirmar y LEER LA TABLA, no el resumen ────────────────────────────
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/v1/bancos/importaciones/historico/"+plan.CargaID+"/confirmar", nil))
	aplicado := planDe(t, rec, http.StatusOK)
	if aplicado.Insertados != 4 {
		t.Fatalf("insertados = %d, se esperaban 4", aplicado.Insertados)
	}

	var enBaseDeb, enBaseCre string
	var filas int
	if err := repo.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(debito), 0)::text, COALESCE(SUM(credito), 0)::text
		  FROM movimiento_bancario WHERE empresa_id = $1::uuid AND origen_historico`,
		empresaPrueba).Scan(&filas, &enBaseDeb, &enBaseCre); err != nil {
		t.Fatalf("sumar lo cargado: %v", err)
	}
	if filas != 4 {
		t.Fatalf("filas en la base = %d, se esperaban 4", filas)
	}
	if enBaseDeb != prometidoDeb || enBaseCre != prometidoCre {
		t.Fatalf("la base tiene %s/%s y el previsualizar prometió %s/%s: son dos números distintos",
			enBaseDeb, enBaseCre, prometidoDeb, prometidoCre)
	}

	// Y ni un movimiento de cero: una fila que redondeada no es plata no es un movimiento.
	var enCero int
	if err := repo.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM movimiento_bancario
		 WHERE empresa_id = $1::uuid AND origen_historico AND debito = 0 AND credito = 0`,
		empresaPrueba).Scan(&enCero); err != nil {
		t.Fatalf("contar movimientos en cero: %v", err)
	}
	if enCero != 0 {
		t.Errorf("quedaron %d movimiento(s) de ¢0,00 en los libros", enCero)
	}
}
