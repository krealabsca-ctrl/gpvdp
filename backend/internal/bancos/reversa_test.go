package bancos

// Revertir una CARGA entera (mig 0085), contra Postgres de verdad y por el BORDE HTTP.
//
// Se corre contra la base porque lo que se está probando es SQL —qué filas cambian de `incluido` y
// cuáles no— y un doble del repositorio probaría el doble. Igual que excluido_no_suma_test.go y
// segmento_sin_partida_test.go: cada corrida crea un ESQUEMA temporal con copias
// (`LIKE ... INCLUDING ALL`) de las tablas que se tocan, siembra ahí y lo borra al terminar. En
// `public` no se escribe NUNCA. Sin base de datos la prueba se OMITE (no falla).
//
// El escenario es el incidente real del 23-set-2026, con las trampas que lo hacen difícil:
//
//   - En la MISMA cuenta equivocada conviven dos cargas: la de julio (BIEN, con dos movimientos ya
//     clasificados) y la de setiembre (MAL). Revertir la segunda no puede tocar la primera. Esa es
//     la regla textual del Director: «del día 20 hacia atrás todo está bien».
//   - Dentro de la carga mala hay un movimiento que YA estaba excluido por otra corrección: deshacer
//     la reversa no puede re-incluirlo, o estaría metiendo de vuelta plata que nadie pidió.
//   - Hay una carga de 0 movimientos y una carga de OTRA empresa.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// Las tablas que tocan la reversa y sus bloqueos. Si faltara alguna, la consulta falla en vez de
// leer los datos reales sin que nadie se entere (el pool ve SOLO el esquema temporal).
var tablasDeLaReversa = []string{
	"banco", "cuenta_bancaria", "concepto", "clasificacion", "usuario",
	"importacion", "movimiento_bancario", "movimiento_reporte_segmentacion",
	"cobro_cxc", "cxc_planilla_movimiento", "responsabilidad_periodo",
	"periodo_cierre", "acta_conciliacion", "auditoria_evento",
}

const (
	impJulio     = "a1a1a1a1-a1a1-4a1a-8a1a-a1a1a1a1a1a1" // en la cuenta equivocada, pero BIEN
	impSetiembre = "b2b2b2b2-b2b2-4b2b-8b2b-b2b2b2b2b2b2" // la carga MAL: la que se revierte
	impVacia     = "c3c3c3c3-c3c3-4c3c-8c3c-c3c3c3c3c3c3" // 0 movimientos
	impAjena     = "d4d4d4d4-d4d4-4d4d-8d4d-d4d4d4d4d4d4" // de OTRA empresa
	bancoAjeno   = "e5e5e5e5-e5e5-4e5e-8e5e-e5e5e5e5e5e5"
	cuentaOtra   = "f6f6f6f6-f6f6-4f6f-8f6f-f6f6f6f6f6f6"
	partidaTras  = "07070707-0707-4707-8707-070707070707" // «Traslado de Fondos»
)

// baseDeLaReversa levanta el esquema temporal y devuelve el repositorio que apunta a él.
func baseDeLaReversa(t *testing.T) *pgRepository {
	t.Helper()
	dsn := os.Getenv("GPVDP_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://gpvdp:localdev@127.0.0.1:5432/gpvdp?sslmode=disable"
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("sin base de datos para la prueba de la reversa (%v)", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("sin base de datos para la prueba de la reversa (%v)", err)
	}

	esquema := fmt.Sprintf("prueba_reversa_%d", time.Now().UnixNano())
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
	for _, tabla := range tablasDeLaReversa {
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

// movDeCarga es un movimiento del escenario de la reversa.
type movDeCarga struct {
	nombre      string
	importacion string
	cuenta      string
	empresa     string
	fecha       string
	debito      string
	credito     string
	partida     string // "" = sin clasificar
	incluido    bool
}

// sembrarCargas deja en la base las cuatro importaciones del escenario y devuelve el id de cada
// movimiento por su nombre.
func sembrarCargas(t *testing.T, repo *pgRepository) map[string]string {
	t.Helper()
	ctx := context.Background()

	catalogo := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Promerica')`,
			[]any{bancoPrueba, empresaPrueba}},
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'BAC')`,
			[]any{bancoAjeno, otraEmpresa}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'Promerica VDP')`,
			[]any{cuentaBuena, empresaPrueba, bancoPrueba}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'Promerica Colinas')`,
			[]any{cuentaMala, empresaPrueba, bancoPrueba}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'BAC de la otra empresa')`,
			[]any{cuentaOtra, otraEmpresa, bancoAjeno}},
		{`INSERT INTO concepto (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Traslados')`,
			[]any{conceptoID, empresaPrueba}},
		{`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'Traslado de Fondos')`,
			[]any{partidaTras, empresaPrueba, conceptoID}},
		{`INSERT INTO usuario (id, nombre, email, password_hash)
		  VALUES ($1::uuid, 'Dirección Financiera', 'df@prueba.local', 'x')`,
			[]any{usuarioPrueba}},
	}
	for _, c := range catalogo {
		if _, err := repo.pool.Exec(ctx, c.q, c.args...); err != nil {
			t.Fatalf("sembrar catálogo de la reversa: %v", err)
		}
	}

	cargas := []struct {
		id, empresa, cuenta, archivo string
	}{
		{impJulio, empresaPrueba, cuentaMala, "COLINAS PROMERICA COLONES.xlsx"},
		{impSetiembre, empresaPrueba, cuentaMala, "VALLE DE PAZ PROMERICA COLONES.xlsx"},
		{impVacia, empresaPrueba, cuentaBuena, "VALLE DE PAZ BN DOLARES.xlsx"},
		{impAjena, otraEmpresa, cuentaOtra, "OTRA EMPRESA BAC.xlsx"},
	}
	for i, c := range cargas {
		if _, err := repo.pool.Exec(ctx, `
			INSERT INTO importacion (id, empresa_id, cuenta_bancaria_id, source_file_hash, nombre_archivo,
			                         estado, creado_por, creado_en)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, 'CONFIRMADA', $6::uuid, now() - make_interval(mins => $7))`,
			c.id, c.empresa, c.cuenta, "hash-"+c.id, c.archivo, usuarioPrueba, len(cargas)-i); err != nil {
			t.Fatalf("sembrar carga %s: %v", c.archivo, err)
		}
	}

	movs := []movDeCarga{
		// Julio, en la MISMA cuenta equivocada: está BIEN y dos ya están clasificados.
		{nombre: "julio_traslado", importacion: impJulio, cuenta: cuentaMala, empresa: empresaPrueba,
			fecha: "2026-07-03", debito: "3000000.00", partida: partidaTras, incluido: true},
		{nombre: "julio_traslado_vuelta", importacion: impJulio, cuenta: cuentaMala, empresa: empresaPrueba,
			fecha: "2026-07-03", credito: "3000000.00", partida: partidaTras, incluido: true},
		{nombre: "julio_sin_partida", importacion: impJulio, cuenta: cuentaMala, empresa: empresaPrueba,
			fecha: "2026-07-15", credito: "125000.00", incluido: true},

		// Setiembre: LA CARGA MAL. Tres cuentan plata hoy y una ya estaba excluida de antes.
		{nombre: "set_debito", importacion: impSetiembre, cuenta: cuentaMala, empresa: empresaPrueba,
			fecha: "2026-09-02", debito: "10000000.00", incluido: true},
		{nombre: "set_credito", importacion: impSetiembre, cuenta: cuentaMala, empresa: empresaPrueba,
			fecha: "2026-09-08", credito: "7042727.09", partida: partidaTras, incluido: true},
		{nombre: "set_credito_2", importacion: impSetiembre, cuenta: cuentaMala, empresa: empresaPrueba,
			fecha: "2026-09-14", credito: "500000.00", incluido: true},
		{nombre: "set_ya_excluido", importacion: impSetiembre, cuenta: cuentaMala, empresa: empresaPrueba,
			fecha: "2026-09-14", credito: "123456.78", incluido: false},

		// De OTRA empresa: nada de lo que se haga acá puede tocarlo.
		{nombre: "ajeno", importacion: impAjena, cuenta: cuentaOtra, empresa: otraEmpresa,
			fecha: "2026-09-09", credito: "999999.99", incluido: true},
	}

	const q = `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, importacion_id, fecha, documento, descripcion,
			 debito, credito, moneda_original, monto_original, monto_crc,
			 concepto_id, clasificacion_id, estado_clasificacion, natural_key, incluido)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::date, $5, $6, $7, $8, 'CRC', $9, $9,
		        NULLIF($10,'')::uuid, NULLIF($11,'')::uuid, $12, $13, $14)
		RETURNING id::text`
	ids := map[string]string{}
	for _, m := range movs {
		deb, cred := m.debito, m.credito
		if deb == "" {
			deb = "0"
		}
		if cred == "" {
			cred = "0"
		}
		monto := deb
		if deb == "0" {
			monto = cred
		}
		concepto, estado := "", "NO_IDENTIFICADO"
		if m.partida != "" {
			concepto, estado = conceptoID, "REVISADO"
		}
		var id string
		if err := repo.pool.QueryRow(ctx, q,
			m.empresa, m.cuenta, m.importacion, m.fecha, m.nombre, strings.ToUpper(m.nombre),
			deb, cred, monto, concepto, m.partida, estado, m.nombre, m.incluido).Scan(&id); err != nil {
			t.Fatalf("sembrar movimiento %s: %v", m.nombre, err)
		}
		ids[m.nombre] = id
	}
	return ids
}

// routerDeReversa monta las tres rutas con un usuario ya autenticado (lo que en producción pone el
// middleware), para probar el CÓDIGO HTTP exacto y no solo el error de Go: el modo de falla clásico
// de este paquete es un centinela nuevo saliendo como 500.
func routerDeReversa(repo *pgRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := NewService(repo, shared.NewAudit(repo.pool, zap.NewNop()), zap.NewNop(), false)
	h := NewHandler(svc, zap.NewNop())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		auth.SetClaims(c, &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: usuarioPrueba},
			EmpresaID:        empresaPrueba,
		})
		c.Next()
	})
	r.GET("/v1/bancos/importaciones", h.Importaciones)
	r.POST("/v1/bancos/importaciones/:id/revertir", h.RevertirImportacion)
	r.POST("/v1/bancos/importaciones/:id/deshacer-reversa", h.DeshacerReversaImportacion)
	return r
}

// llamar ejecuta una petición contra el router y devuelve el grabador (a diferencia de `pedir`, de
// segmento_revision_test.go, acá hace falta el cuerpo Y el código juntos en cada aserción).
func llamar(t *testing.T, r *gin.Engine, metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	if cuerpo != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// estadoDeMovimientos devuelve, por nombre de movimiento, si hoy está incluido.
func estadoDeMovimientos(t *testing.T, repo *pgRepository) map[string]bool {
	t.Helper()
	rows, err := repo.pool.Query(context.Background(),
		`SELECT documento, incluido FROM movimiento_bancario`)
	if err != nil {
		t.Fatalf("leer estado de los movimientos: %v", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var nombre string
		var incluido bool
		if err := rows.Scan(&nombre, &incluido); err != nil {
			t.Fatalf("scan estado: %v", err)
		}
		out[nombre] = incluido
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterar estado: %v", err)
	}
	return out
}

// totalesDeLaCuenta son los totales que la hoja de trabajo muestra para una cuenta y un mes.
func totalesDeLaCuenta(t *testing.T, repo *pgRepository, cuenta, periodo string) Totales {
	t.Helper()
	got, err := repo.ListarMovimientos(context.Background(), empresaPrueba,
		FiltrosMovimientos{Periodo: periodo, CuentaID: cuenta})
	if err != nil {
		t.Fatalf("totales de la cuenta: %v", err)
	}
	return got.Totales
}

func itemPorID(t *testing.T, lista ListaImportaciones, id string) ImportacionItem {
	t.Helper()
	for _, it := range lista.Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("la carga %s no está en el listado (%d filas)", id, len(lista.Items))
	return ImportacionItem{}
}

func leerLista(t *testing.T, rec *httptest.ResponseRecorder) ListaImportaciones {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("listado: status = %d, cuerpo = %s", rec.Code, rec.Body.String())
	}
	var lista ListaImportaciones
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatalf("listado ilegible: %v — %s", err, rec.Body.String())
	}
	return lista
}

// ── El caso del Director ─────────────────────────────────────────────────────

func TestRevertirUnaCargaNoTocaLaCargaBuena(t *testing.T) {
	repo := baseDeLaReversa(t)
	sembrarCargas(t, repo)
	r := routerDeReversa(repo)

	antesJulio := totalesDeLaCuenta(t, repo, cuentaMala, "2026-07")
	antesSetiembre := totalesDeLaCuenta(t, repo, cuentaMala, "2026-09")
	// La foto de partida: sin esto, una prueba que «pasa» podría estar mirando una cuenta vacía.
	if !igualDecimal(t, antesSetiembre.TotalDebitos, "10000000.00") ||
		!igualDecimal(t, antesSetiembre.TotalCreditos, "7542727.09") {
		t.Fatalf("la siembra no dejó el incidente: débitos=%s créditos=%s",
			antesSetiembre.TotalDebitos, antesSetiembre.TotalCreditos)
	}

	rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impSetiembre+"/revertir",
		`{"motivo":"se importó en Promerica Colinas y el archivo era de Promerica VDP"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("revertir: status = %d, cuerpo = %s", rec.Code, rec.Body.String())
	}
	var res ResultadoReversa
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("respuesta ilegible: %v — %s", err, rec.Body.String())
	}

	// La prueba de cuánta plata salió de los libros: el conteo y las dos sumas.
	if res.Excluidos != 3 {
		t.Errorf("excluidos = %d, se esperaban 3 (el cuarto ya estaba excluido de antes)", res.Excluidos)
	}
	if !igualDecimal(t, res.TotalDebitos, "10000000.00") || !igualDecimal(t, res.TotalCreditos, "7542727.09") {
		t.Errorf("sumas de la reversa = %s / %s, se esperaban 10000000.00 / 7542727.09",
			res.TotalDebitos, res.TotalCreditos)
	}
	if res.Estado != EstadoImportacionRevertida || res.RevertidaEn == "" || res.Moneda != "CRC" {
		t.Errorf("la respuesta no describe la reversa: %+v", res)
	}

	t.Run("las filas de la carga buena siguen contando, con su clasificación intacta", func(t *testing.T) {
		estado := estadoDeMovimientos(t, repo)
		for _, nombre := range []string{"julio_traslado", "julio_traslado_vuelta", "julio_sin_partida"} {
			if !estado[nombre] {
				t.Errorf("%s quedó excluido: la reversa se llevó puesto lo que estaba BIEN", nombre)
			}
		}
		// Lo de julio ya estaba clasificado como «Traslado de Fondos»: la reversa no borra trabajo.
		var clasificados int
		if err := repo.pool.QueryRow(context.Background(), `
			SELECT COUNT(*) FROM movimiento_bancario
			WHERE importacion_id = $1::uuid AND clasificacion_id = $2::uuid AND estado_clasificacion = 'REVISADO'`,
			impJulio, partidaTras).Scan(&clasificados); err != nil {
			t.Fatalf("contar clasificados de julio: %v", err)
		}
		if clasificados != 2 {
			t.Errorf("clasificados de julio = %d, se esperaban 2", clasificados)
		}
		// Y la clasificación de lo REVERTIDO tampoco se borra: si el archivo se vuelve a importar en
		// la cuenta correcta, ese trabajo sigue ahí para consultarlo.
		var clasifRevertido int
		if err := repo.pool.QueryRow(context.Background(), `
			SELECT COUNT(*) FROM movimiento_bancario
			WHERE importacion_id = $1::uuid AND clasificacion_id IS NOT NULL`, impSetiembre).Scan(&clasifRevertido); err != nil {
			t.Fatalf("contar clasificados revertidos: %v", err)
		}
		if clasifRevertido != 1 {
			t.Errorf("clasificados de la carga revertida = %d, se esperaba 1 (la reversa no borra la partida)", clasifRevertido)
		}
	})

	t.Run("la plata: los totales bajan EXACTAMENTE por la suma de la carga revertida", func(t *testing.T) {
		despuesSetiembre := totalesDeLaCuenta(t, repo, cuentaMala, "2026-09")
		if !igualDecimal(t, despuesSetiembre.TotalDebitos, "0") ||
			!igualDecimal(t, despuesSetiembre.TotalCreditos, "0") {
			t.Errorf("setiembre después = %s / %s, se esperaba 0 / 0",
				despuesSetiembre.TotalDebitos, despuesSetiembre.TotalCreditos)
		}
		// La resta exacta, dicha como resta: sin esto la prueba de arriba también pasaría si la
		// reversa hubiera excluido de más.
		bajoDebito := dec(antesSetiembre.TotalDebitos).Sub(dec(despuesSetiembre.TotalDebitos))
		bajoCredito := dec(antesSetiembre.TotalCreditos).Sub(dec(despuesSetiembre.TotalCreditos))
		if !bajoDebito.Equal(dec(res.TotalDebitos)) || !bajoCredito.Equal(dec(res.TotalCreditos)) {
			t.Errorf("la baja fue %s / %s y la reversa dijo %s / %s",
				bajoDebito, bajoCredito, res.TotalDebitos, res.TotalCreditos)
		}
		// Julio, en la MISMA cuenta, no se movió ni un colón.
		despuesJulio := totalesDeLaCuenta(t, repo, cuentaMala, "2026-07")
		if despuesJulio.TotalDebitos != antesJulio.TotalDebitos ||
			despuesJulio.TotalCreditos != antesJulio.TotalCreditos {
			t.Errorf("julio cambió: antes %s/%s, después %s/%s",
				antesJulio.TotalDebitos, antesJulio.TotalCreditos,
				despuesJulio.TotalDebitos, despuesJulio.TotalCreditos)
		}
	})

	t.Run("revertir dos veces da 409 y no vuelve a contar movimientos", func(t *testing.T) {
		rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impSetiembre+"/revertir",
			`{"motivo":"otra vez"}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, se esperaba 409 — cuerpo: %s", rec.Code, rec.Body.String())
		}
		var cuerpo struct{ Code, Message string }
		if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
			t.Fatalf("cuerpo ilegible: %v — %s", err, rec.Body.String())
		}
		if cuerpo.Code == "ERROR_INTERNO" {
			t.Fatalf("el centinela salió como error interno: %s", rec.Body.String())
		}
		// Y el motivo de la PRIMERA reversa no se pisó con el de la segunda.
		var motivo string
		if err := repo.pool.QueryRow(context.Background(),
			`SELECT motivo_reversa FROM importacion WHERE id = $1::uuid`, impSetiembre).Scan(&motivo); err != nil {
			t.Fatalf("leer motivo: %v", err)
		}
		if !strings.Contains(motivo, "Promerica Colinas") {
			t.Errorf("el motivo quedó en %q: la segunda reversa pisó la primera", motivo)
		}
	})

	t.Run("deshacer devuelve exactamente las mismas filas, ni una más", func(t *testing.T) {
		rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impSetiembre+"/deshacer-reversa",
			`{"motivo":"me equivoqué de carga"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, cuerpo = %s", rec.Code, rec.Body.String())
		}
		var des ResultadoDeshacerReversa
		if err := json.Unmarshal(rec.Body.Bytes(), &des); err != nil {
			t.Fatalf("respuesta ilegible: %v — %s", err, rec.Body.String())
		}
		if des.Reincluidos != 3 {
			t.Errorf("reincluidos = %d, se esperaban 3", des.Reincluidos)
		}
		if des.Estado != "CONFIRMADA" {
			t.Errorf("estado = %q, se esperaba CONFIRMADA", des.Estado)
		}
		estado := estadoDeMovimientos(t, repo)
		// La trampa: el movimiento que YA estaba excluido antes de la reversa tiene que seguir
		// excluido. Re-incluirlo sería volver a meter plata que nadie pidió que volviera.
		if estado["set_ya_excluido"] {
			t.Error("set_ya_excluido volvió a contar: deshacer re-incluyó una fila que la reversa no había tocado")
		}
		for _, nombre := range []string{"set_debito", "set_credito", "set_credito_2"} {
			if !estado[nombre] {
				t.Errorf("%s no volvió a los libros", nombre)
			}
		}
		// Los totales vuelven a ser los de antes de todo.
		vuelta := totalesDeLaCuenta(t, repo, cuentaMala, "2026-09")
		if !igualDecimal(t, vuelta.TotalDebitos, antesSetiembre.TotalDebitos) ||
			!igualDecimal(t, vuelta.TotalCreditos, antesSetiembre.TotalCreditos) {
			t.Errorf("los totales no volvieron: %s/%s vs %s/%s",
				vuelta.TotalDebitos, vuelta.TotalCreditos,
				antesSetiembre.TotalDebitos, antesSetiembre.TotalCreditos)
		}
	})

	t.Run("deshacer una carga que no estaba revertida da 409", func(t *testing.T) {
		rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impJulio+"/deshacer-reversa", `{}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, se esperaba 409 — cuerpo: %s", rec.Code, rec.Body.String())
		}
		if estadoDeMovimientos(t, repo)["julio_traslado"] != true {
			t.Error("deshacer sobre una carga no revertida tocó sus movimientos")
		}
	})

	t.Run("la auditoría deja la prueba de cuánta plata se movió", func(t *testing.T) {
		rows, err := repo.pool.Query(context.Background(),
			`SELECT accion, valor_nuevo::text FROM auditoria_evento WHERE entidad = 'importacion' ORDER BY ts`)
		if err != nil {
			t.Fatalf("leer auditoría: %v", err)
		}
		defer rows.Close()
		vistos := map[string]string{}
		for rows.Next() {
			var accion, valor string
			if err := rows.Scan(&accion, &valor); err != nil {
				t.Fatalf("scan auditoría: %v", err)
			}
			vistos[accion] = valor
		}
		rev, ok := vistos["REVERTIR_IMPORTACION"]
		if !ok {
			t.Fatalf("no quedó el evento de la reversa: %v", vistos)
		}
		// Los montos van como los serializa shopspring (sin ceros de relleno), igual que en el resto
		// del módulo: lo que importa es el VALOR, y el evento es la prueba de cuánta plata salió.
		for _, texto := range []string{`"total_debitos": "10000000"`, `"total_creditos": "7542727.09"`, "Promerica Colinas"} {
			if !strings.Contains(rev, texto) {
				t.Errorf("el evento de la reversa no dice %q: %s", texto, rev)
			}
		}
		if _, ok := vistos["DESHACER_REVERSA_IMPORTACION"]; !ok {
			t.Errorf("deshacer no dejó su propio evento: %v", vistos)
		}
	})
}

// ── Aislamiento entre empresas ───────────────────────────────────────────────

func TestReversaNoCruzaEmpresas(t *testing.T) {
	repo := baseDeLaReversa(t)
	sembrarCargas(t, repo)
	r := routerDeReversa(repo)

	for _, c := range []struct{ nombre, ruta, cuerpo string }{
		{"revertir", "/v1/bancos/importaciones/" + impAjena + "/revertir", `{"motivo":"no debería poder"}`},
		{"deshacer", "/v1/bancos/importaciones/" + impAjena + "/deshacer-reversa", `{}`},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			rec := llamar(t, r, http.MethodPost, c.ruta, c.cuerpo)
			// 404 y no 403: para esta empresa esa carga NO EXISTE, y un 403 confirmaría que sí.
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, se esperaba 404 — cuerpo: %s", rec.Code, rec.Body.String())
			}
		})
	}

	if !estadoDeMovimientos(t, repo)["ajeno"] {
		t.Error("el movimiento de la otra empresa quedó excluido")
	}
	var estado string
	if err := repo.pool.QueryRow(context.Background(),
		`SELECT estado FROM importacion WHERE id = $1::uuid`, impAjena).Scan(&estado); err != nil {
		t.Fatalf("leer estado de la carga ajena: %v", err)
	}
	if estado != "CONFIRMADA" {
		t.Errorf("la carga de la otra empresa quedó en %q", estado)
	}

	t.Run("el listado tampoco la muestra", func(t *testing.T) {
		lista := leerLista(t, llamar(t, r, http.MethodGet, "/v1/bancos/importaciones", ""))
		for _, it := range lista.Items {
			if it.ID == impAjena {
				t.Fatal("una carga de otra empresa apareció en el listado")
			}
		}
		if lista.Total != 3 {
			t.Errorf("total = %d, se esperaban 3 (las de esta empresa)", lista.Total)
		}
	})
}

// ── El motivo ────────────────────────────────────────────────────────────────

func TestRevertirSinMotivoNoEscribeNada(t *testing.T) {
	repo := baseDeLaReversa(t)
	sembrarCargas(t, repo)
	r := routerDeReversa(repo)

	for _, c := range []struct{ nombre, cuerpo string }{
		{"sin cuerpo", ""},
		{"cuerpo vacío", `{}`},
		{"motivo en blanco", `{"motivo":"   "}`},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impSetiembre+"/revertir", c.cuerpo)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba 400 — cuerpo: %s", rec.Code, rec.Body.String())
			}
			var cuerpo struct{ Code, Message string }
			if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("cuerpo ilegible: %v", err)
			}
			if cuerpo.Code == "ERROR_INTERNO" {
				t.Fatalf("el centinela salió como error interno: %s", rec.Body.String())
			}
			if !estadoDeMovimientos(t, repo)["set_debito"] {
				t.Error("se excluyó igual sin motivo")
			}
		})
	}
}

// ── Los bloqueos ─────────────────────────────────────────────────────────────

// bloqueo es un escenario de dependencia viva sobre un movimiento de la carga mala.
type bloqueo struct {
	nombre string
	// sembrar recibe el id del movimiento `set_credito` y deja la dependencia.
	sembrar func(t *testing.T, repo *pgRepository, movID string)
	// enTexto tiene que aparecer en el mensaje del 422: el usuario necesita saber qué deshacer.
	enTexto string
}

func TestCadaBloqueoRechazaLaReversaSinEscribirNada(t *testing.T) {
	casos := []bloqueo{
		{
			nombre: "un cobro de CxC identificado contra el movimiento",
			sembrar: func(t *testing.T, repo *pgRepository, movID string) {
				ejecutar(t, repo, `
					INSERT INTO cobro_cxc (empresa_id, fecha_pago, monto, movimiento_bancario_id)
					VALUES ($1::uuid, '2026-09-08', 7042727.09, $2::uuid)`, empresaPrueba, movID)
			},
			enTexto: "cobro(s) de CxC",
		},
		{
			nombre: "una planilla de asociación de CxC",
			sembrar: func(t *testing.T, repo *pgRepository, movID string) {
				ejecutar(t, repo, `
					INSERT INTO cxc_planilla_movimiento (empresa_id, planilla_id, movimiento_bancario_id)
					VALUES ($1::uuid, gen_random_uuid(), $2::uuid)`, empresaPrueba, movID)
			},
			enTexto: "planilla de CxC",
		},
		{
			nombre: "un aviso de segmentación sin resolver",
			sembrar: func(t *testing.T, repo *pgRepository, movID string) {
				ejecutar(t, repo, `
					INSERT INTO movimiento_reporte_segmentacion (empresa_id, movimiento_id, usuario_id, motivo)
					VALUES ($1::uuid, $2::uuid, $3::uuid, 'esto no es de mi partida')`,
					empresaPrueba, movID, usuarioPrueba)
			},
			enTexto: "aviso(s) de segmentación",
		},
		{
			nombre: "una obligación del calendario que se da por cumplida con él",
			sembrar: func(t *testing.T, repo *pgRepository, movID string) {
				ejecutar(t, repo, `
					INSERT INTO responsabilidad_periodo
						(empresa_id, responsabilidad_id, periodo, vence_en, monto_esperado,
						 estado, cumplida_con, movimiento_id)
					VALUES ($1::uuid, gen_random_uuid(), '2026-09', '2026-09-30', 7042727.09,
					        'CUMPLIDA', 'MOVIMIENTO', $2::uuid)`, empresaPrueba, movID)
			},
			enTexto: "calendario de CxP",
		},
		{
			nombre: "un traslado ya emparejado",
			sembrar: func(t *testing.T, repo *pgRepository, movID string) {
				// El par apunta desde la carga BUENA hacia el movimiento de la carga mala: el bloqueo
				// tiene que verse igual en ese sentido.
				ejecutar(t, repo, `
					UPDATE movimiento_bancario SET par_traslado_id = $1::uuid, es_traslado = true
					WHERE documento = 'julio_traslado'`, movID)
			},
			enTexto: "emparejados como traslado",
		},
		{
			// La huella Bancos↔CxP: este movimiento ES el pago de una factura y CxP ya la pasó a
			// CONCILIADO por eso. Excluirlo deja a la factura diciendo que está conciliada contra
			// plata que ya no está en los libros, y el barrido no puede volver a mirarla porque
			// filtra por `m.incluido`.
			nombre: "el movimiento es el pago de una factura de CxP",
			sembrar: func(t *testing.T, repo *pgRepository, movID string) {
				ejecutar(t, repo, `
					UPDATE movimiento_bancario SET documento_cxp_id = gen_random_uuid()
					WHERE id = $1::uuid`, movID)
			},
			enTexto: "factura(s) de CxP",
		},
		{
			nombre: "el período ya está cerrado",
			sembrar: func(t *testing.T, repo *pgRepository, _ string) {
				ejecutar(t, repo, `
					INSERT INTO periodo_cierre (empresa_id, anio, mes) VALUES ($1::uuid, 2026, 9)`, empresaPrueba)
			},
			enTexto: "2026-09 ya está cerrado",
		},
		{
			nombre: "el acta de conciliación de esa cuenta ya está firmada",
			sembrar: func(t *testing.T, repo *pgRepository, _ string) {
				ejecutar(t, repo, `
					INSERT INTO acta_conciliacion
						(empresa_id, cuenta_bancaria_id, anio, mes, saldo_banco, saldo_libros,
						 ajuste_partidas, firmado_por, firmado_en)
					VALUES ($1::uuid, $2::uuid, 2026, 9, 0, 0, 0, $3::uuid, now())`,
					empresaPrueba, cuentaMala, usuarioPrueba)
			},
			enTexto: "acta de conciliación de 2026-09",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo := baseDeLaReversa(t)
			ids := sembrarCargas(t, repo)
			c.sembrar(t, repo, ids["set_credito"])
			r := routerDeReversa(repo)

			rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impSetiembre+"/revertir",
				`{"motivo":"se cargó en la cuenta equivocada"}`)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, se esperaba 422 — cuerpo: %s", rec.Code, rec.Body.String())
			}
			var cuerpo struct{ Code, Message string }
			if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("cuerpo ilegible: %v — %s", err, rec.Body.String())
			}
			if cuerpo.Code == "ERROR_INTERNO" {
				t.Fatalf("el bloqueo salió como error interno: %s", rec.Body.String())
			}
			if !strings.Contains(cuerpo.Message, c.enTexto) {
				t.Errorf("el mensaje no dice qué está en el medio (%q): %q", c.enTexto, cuerpo.Message)
			}

			// Y —lo que de verdad importa— NO escribió nada.
			estado := estadoDeMovimientos(t, repo)
			for _, nombre := range []string{"set_debito", "set_credito", "set_credito_2"} {
				if !estado[nombre] {
					t.Errorf("%s quedó excluido pese al 422: la reversa escribió igual", nombre)
				}
			}
			var estadoCarga string
			if err := repo.pool.QueryRow(context.Background(),
				`SELECT estado FROM importacion WHERE id = $1::uuid`, impSetiembre).Scan(&estadoCarga); err != nil {
				t.Fatalf("leer estado de la carga: %v", err)
			}
			if estadoCarga != "CONFIRMADA" {
				t.Errorf("la carga quedó en %q pese al 422", estadoCarga)
			}

			// El listado tiene que traer el bloqueo para que el botón salga deshabilitado CON la
			// razón, en vez de fallar recién al apretarlo.
			lista := leerLista(t, llamar(t, r, http.MethodGet, "/v1/bancos/importaciones", ""))
			it := itemPorID(t, lista, impSetiembre)
			if it.PuedeRevertir {
				t.Error("el listado dice que se puede revertir y el servidor la rechaza: el botón miente")
			}
			if !strings.Contains(it.RazonNoRevertir, c.enTexto) {
				t.Errorf("razon_no_revertir = %q, se esperaba que dijera %q", it.RazonNoRevertir, c.enTexto)
			}
		})
	}
}

// ejecutar corre un INSERT/UPDATE de siembra y falla la prueba si no entra.
func ejecutar(t *testing.T, repo *pgRepository, q string, args ...any) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("sembrar el bloqueo: %v", err)
	}
}

// ── El listado ───────────────────────────────────────────────────────────────

func TestListadoDeCargas(t *testing.T) {
	repo := baseDeLaReversa(t)
	sembrarCargas(t, repo)
	r := routerDeReversa(repo)

	lista := leerLista(t, llamar(t, r, http.MethodGet, "/v1/bancos/importaciones", ""))
	if lista.Total != 3 || len(lista.Items) != 3 {
		t.Fatalf("total = %d, items = %d; se esperaban 3 y 3", lista.Total, len(lista.Items))
	}
	if lista.Page != 1 || lista.PageSize != 50 {
		t.Errorf("la respuesta no repite la página: page=%d page_size=%d", lista.Page, lista.PageSize)
	}
	// La más reciente primero: es el orden con el que se busca «la que acabo de subir mal».
	if lista.Items[0].ID != impVacia {
		t.Errorf("la primera fila es %s; se esperaba la más reciente", lista.Items[0].ID)
	}

	t.Run("la carga mal trae sus conteos, sus sumas y su rango de fechas", func(t *testing.T) {
		it := itemPorID(t, lista, impSetiembre)
		if it.Movimientos != 4 {
			t.Errorf("movimientos = %d, se esperaban 4", it.Movimientos)
		}
		if it.Excluidos != 1 {
			t.Errorf("excluidos = %d, se esperaba 1 (el que ya estaba corregido)", it.Excluidos)
		}
		if it.Clasificados != 1 {
			t.Errorf("clasificados = %d, se esperaba 1", it.Clasificados)
		}
		// Las sumas son de TODO lo que trajo el archivo (incluido lo ya excluido): el usuario está
		// mirando qué metió esta carga, no qué cuenta hoy.
		if !igualDecimal(t, it.TotalDebitos, "10000000.00") || !igualDecimal(t, it.TotalCreditos, "7666183.87") {
			t.Errorf("sumas = %s / %s, se esperaban 10000000.00 / 7666183.87", it.TotalDebitos, it.TotalCreditos)
		}
		if it.FechaDesde != "2026-09-02" || it.FechaHasta != "2026-09-14" {
			t.Errorf("rango = %s..%s, se esperaba 2026-09-02..2026-09-14", it.FechaDesde, it.FechaHasta)
		}
		if it.Banco != "Promerica" || it.CuentaAlias != "Promerica Colinas" || it.Moneda != "CRC" {
			t.Errorf("la fila no identifica la cuenta: %+v", it)
		}
		if it.CreadoPorNombre != "Dirección Financiera" || it.CreadoEn == "" {
			t.Errorf("la fila no dice quién la cargó ni cuándo: %+v", it)
		}
		if !it.PuedeRevertir || it.RazonNoRevertir != "" || it.PuedeDeshacerReversa {
			t.Errorf("esta carga se puede revertir y nada más: %+v", it)
		}
		if it.Bloqueos.Hay() {
			t.Errorf("bloqueos inventados: %+v", it.Bloqueos)
		}
	})

	t.Run("la carga de 0 movimientos también aparece", func(t *testing.T) {
		// Es justo la que confunde: sin esta fila, quien subió un archivo que no trajo nada no tiene
		// cómo saber si lo subió.
		it := itemPorID(t, lista, impVacia)
		if it.Movimientos != 0 || it.Excluidos != 0 || it.Clasificados != 0 {
			t.Errorf("conteos = %d/%d/%d, se esperaban 0", it.Movimientos, it.Excluidos, it.Clasificados)
		}
		if !igualDecimal(t, it.TotalDebitos, "0") || !igualDecimal(t, it.TotalCreditos, "0") {
			t.Errorf("sumas = %s / %s, se esperaba 0 / 0", it.TotalDebitos, it.TotalCreditos)
		}
		if it.FechaDesde != "" || it.FechaHasta != "" {
			t.Errorf("rango = %q..%q, se esperaba vacío", it.FechaDesde, it.FechaHasta)
		}
	})

	t.Run("el filtro por cuenta recorta", func(t *testing.T) {
		lista := leerLista(t, llamar(t, r, http.MethodGet,
			"/v1/bancos/importaciones?cuenta_bancaria_id="+cuentaMala, ""))
		if lista.Total != 2 || len(lista.Items) != 2 {
			t.Fatalf("total = %d, items = %d; se esperaban 2 (las de Promerica Colinas)", lista.Total, len(lista.Items))
		}
	})

	t.Run("una página fuera de rango vuelve al defecto y no revienta", func(t *testing.T) {
		lista := leerLista(t, llamar(t, r, http.MethodGet,
			"/v1/bancos/importaciones?page=200000000000000000&page_size=5000", ""))
		if lista.Page != 1 || lista.PageSize != 50 || lista.Total != 3 {
			t.Errorf("page=%d page_size=%d total=%d; se esperaba el defecto", lista.Page, lista.PageSize, lista.Total)
		}
	})

	t.Run("una vez revertida, la fila lo dice y ofrece deshacer", func(t *testing.T) {
		rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impSetiembre+"/revertir",
			`{"motivo":"cuenta equivocada"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("revertir: status = %d, cuerpo = %s", rec.Code, rec.Body.String())
		}
		lista := leerLista(t, llamar(t, r, http.MethodGet, "/v1/bancos/importaciones", ""))
		it := itemPorID(t, lista, impSetiembre)
		if !it.Revertida || it.Estado != EstadoImportacionRevertida {
			t.Errorf("la fila no se muestra revertida: %+v", it)
		}
		if it.MotivoReversa != "cuenta equivocada" || it.RevertidaEn == "" ||
			it.RevertidaPorNombre != "Dirección Financiera" {
			t.Errorf("la fila no dice cuándo, quién ni por qué: %+v", it)
		}
		if it.PuedeRevertir || !it.PuedeDeshacerReversa {
			t.Errorf("los botones están al revés: %+v", it)
		}
		if it.Excluidos != 4 {
			t.Errorf("excluidos = %d, se esperaban 4 (las 3 de la reversa más la ya corregida)", it.Excluidos)
		}
	})
}

// ── El borde HTTP de los centinelas nuevos ───────────────────────────────────

// Cada centinela nuevo tiene que estar en el switch de responderError. Si falta uno, sale como 500
// «error interno» y el usuario no puede distinguir un rechazo con explicación de una caída del
// servidor — es el modo de falla que ya nos pasó dos veces en este paquete.
func TestResponderErrorTraduceLosCentinelasDeLaReversa(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, zap.NewNop())

	casos := []struct {
		nombre  string
		err     error
		status  int
		enTexto string
		// sinPrefijo: los centinelas NUEVOS mandan el mensaje sin el «bancos: », que es ruido en
		// pantalla. `ErrImportacionNoEncontrada` es anterior y comparte rama con otros siete
		// centinelas que sí lo llevan; corregirlo es otro cambio, no éste.
		sinPrefijo bool
	}{
		{"motivo vacío", ErrMotivoReversaRequerido, http.StatusBadRequest, "por qué", true},
		{"ya revertida", ErrImportacionYaRevertida, http.StatusConflict, "ya fue revertida", true},
		{"no estaba revertida", ErrImportacionNoRevertida, http.StatusConflict, "no está revertida", true},
		{"carga inexistente", ErrImportacionNoEncontrada, http.StatusNotFound, "no encontrada", false},
		{
			"re-confirmar una revertida", ErrImportacionRevertidaNoSeConfirma,
			http.StatusConflict, "deshacé la reversa", true,
		},
		{
			"con bloqueos",
			&ReversaBloqueadaError{Bloqueos: BloqueosReversa{CobrosCxC: 3, PeriodosCerrados: []string{"2026-09"}}},
			http.StatusUnprocessableEntity,
			"3 cobro(s) de CxC",
			true,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/bancos/importaciones/x/revertir", nil)

			h.responderError(ctx, c.err, "test")

			if rec.Code != c.status {
				t.Fatalf("status = %d, se esperaba %d — cuerpo: %s", rec.Code, c.status, rec.Body.String())
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
			if c.sinPrefijo && strings.HasPrefix(cuerpo.Message, "bancos: ") {
				t.Errorf("el mensaje llega con el prefijo del paquete: %q", cuerpo.Message)
			}
		})
	}
}

// Una carga revertida NO se vuelve a confirmar.
//
// El archivo sigue guardado en `importacion.archivo` y la previsualización sigue respondiendo, así
// que volver a apretar «Confirmar» sobre una carga ya revertida es un clic de distancia. Sin el
// freno del servicio, el `UPDATE importacion SET estado='CONFIRMADA'` choca contra el CHECK
// `importacion_reversa_coherente` (la fila seguiría con `revertida_en`) y al usuario le sale un 500
// sin explicación, justo en la pantalla donde acaba de sacar plata de los libros.
//
// Esta prueba corre el camino de verdad: revierte por HTTP y después llama a Confirmar.
func TestUnaCargaRevertidaNoSeVuelveAConfirmar(t *testing.T) {
	repo := baseDeLaReversa(t)
	sembrarCargas(t, repo)
	r := routerDeReversa(repo)
	ctx := context.Background()

	rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impSetiembre+"/revertir",
		`{"motivo":"se cargó en la cuenta equivocada"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("la reversa no pasó: %d — %s", rec.Code, rec.Body.String())
	}

	svc := NewService(repo, shared.NewAudit(repo.pool, zap.NewNop()), zap.NewNop(), false)
	n, err := svc.Confirmar(ctx, empresaPrueba, impSetiembre, nil, usuarioPrueba)
	if !errors.Is(err, ErrImportacionRevertidaNoSeConfirma) {
		t.Fatalf("confirmar una carga revertida devolvió (%d, %v); se esperaba el centinela", n, err)
	}

	// Y lo que importa: la carga sigue revertida y su plata sigue afuera de los libros.
	var estado string
	var revertidaEn *time.Time
	if err := repo.pool.QueryRow(ctx,
		`SELECT estado, revertida_en FROM importacion WHERE id = $1::uuid`, impSetiembre).
		Scan(&estado, &revertidaEn); err != nil {
		t.Fatalf("leer la carga: %v", err)
	}
	if estado != EstadoImportacionRevertida || revertidaEn == nil {
		t.Errorf("la carga quedó en %q (revertida_en nulo: %v): el re-confirm la resucitó",
			estado, revertidaEn == nil)
	}
	for nombre, incluido := range estadoDeMovimientos(t, repo) {
		if strings.HasPrefix(nombre, "set_") && incluido {
			t.Errorf("%s volvió a contar plata después del re-confirm", nombre)
		}
	}

	// Y POR QUÉ hace falta el freno: sin él, ConfirmarConMovimientos corre este mismo UPDATE dentro
	// de su transacción y la base lo rechaza. Ese error sale del repositorio sin centinela y cae en
	// el `default` de responderError, o sea un 500. Si algún día el CHECK deja de rechazarlo, esta
	// línea avisa que el freno del servicio pasó a ser la ÚNICA defensa.
	_, err = repo.pool.Exec(ctx,
		`UPDATE importacion SET estado = 'CONFIRMADA' WHERE empresa_id = $1::uuid AND id = $2::uuid`,
		empresaPrueba, impSetiembre)
	if err == nil {
		t.Error("la base aceptó dejar la carga CONFIRMADA con revertida_en puesto: " +
			"el CHECK importacion_reversa_coherente ya no protege nada")
	} else if !strings.Contains(err.Error(), "importacion_reversa_coherente") {
		t.Errorf("el re-confirm falló por otra razón: %v", err)
	}
}

// Deshacer la reversa también mete plata a los libros, así que tiene que mirar lo mismo que mira
// revertir.
//
// El caso concreto: se revierte la carga de setiembre (bien), y DESPUÉS el equipo cierra setiembre
// —puede, porque los movimientos excluidos no estorban la regla de «100 % clasificado»— y firma el
// acta de esa cuenta. Si ahora alguien aprieta «deshacer», ₡17,5 millones vuelven a caer dentro de
// un mes cerrado y con acta firmada, sin clasificar y sin que nadie se entere.
//
// Revertir eso mismo estaría bloqueado con un 422. Deshacer es la otra mitad de la misma perilla.
func TestDeshacerNoMetePlataEnUnPeriodoCerrado(t *testing.T) {
	repo := baseDeLaReversa(t)
	sembrarCargas(t, repo)
	r := routerDeReversa(repo)
	ctx := context.Background()

	rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impSetiembre+"/revertir",
		`{"motivo":"se cargó en la cuenta equivocada"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("la reversa no pasó: %d — %s", rec.Code, rec.Body.String())
	}

	// Con la plata ya afuera, setiembre se cierra y se firma el acta de esa cuenta.
	ejecutar(t, repo, `INSERT INTO periodo_cierre (empresa_id, anio, mes) VALUES ($1::uuid, 2026, 9)`,
		empresaPrueba)
	ejecutar(t, repo, `
		INSERT INTO acta_conciliacion
			(empresa_id, cuenta_bancaria_id, anio, mes, saldo_banco, saldo_libros,
			 ajuste_partidas, firmado_por, firmado_en)
		VALUES ($1::uuid, $2::uuid, 2026, 9, 0, 0, 0, $3::uuid, now())`,
		empresaPrueba, cuentaMala, usuarioPrueba)

	rec = llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impSetiembre+"/deshacer-reversa",
		`{"motivo":"me equivoqué de carga"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("deshacer devolvió %d; se esperaba 422 porque setiembre está cerrado y con acta firmada — %s",
			rec.Code, rec.Body.String())
	}
	var cuerpo struct{ Code, Message string }
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo ilegible: %v — %s", err, rec.Body.String())
	}
	if cuerpo.Code == "ERROR_INTERNO" {
		t.Fatalf("el bloqueo de deshacer salió como error interno: %s", rec.Body.String())
	}
	for _, frase := range []string{"2026-09 ya está cerrado", "acta de conciliación de 2026-09"} {
		if !strings.Contains(cuerpo.Message, frase) {
			t.Errorf("el mensaje no dice %q: %q", frase, cuerpo.Message)
		}
	}

	// Y no escribió nada: la plata sigue afuera y la carga sigue revertida.
	for nombre, incluido := range estadoDeMovimientos(t, repo) {
		if strings.HasPrefix(nombre, "set_") && incluido {
			t.Errorf("%s volvió a los libros pese al 422", nombre)
		}
	}
	var estado string
	if err := repo.pool.QueryRow(ctx,
		`SELECT estado FROM importacion WHERE id = $1::uuid`, impSetiembre).Scan(&estado); err != nil {
		t.Fatalf("leer la carga: %v", err)
	}
	if estado != EstadoImportacionRevertida {
		t.Errorf("la carga quedó en %q pese al 422", estado)
	}

	// Y el listado tiene que decir lo mismo que el servidor: botón apagado, con la razón escrita.
	it := itemPorID(t, leerLista(t, llamar(t, r, http.MethodGet, "/v1/bancos/importaciones", "")), impSetiembre)
	if it.PuedeDeshacerReversa {
		t.Error("el listado ofrece deshacer y el servidor lo rechaza: el botón miente")
	}
	if !strings.Contains(it.RazonNoRevertir, "2026-09 ya está cerrado") {
		t.Errorf("la fila no explica por qué no se puede deshacer: %q", it.RazonNoRevertir)
	}
}

// La reversa es TODO o NADA.
//
// Son dos UPDATE: el estado de la carga y el `incluido` de sus movimientos. Si el segundo falla y el
// primero quedara escrito, la carga diría «REVERTIDA» con su plata todavía contando: el peor estado
// posible, porque la pantalla afirma que el duplicado ya salió y los totales siguen inflados.
//
// Para forzar la falla sin tocar el código de producción, se le pone al esquema de prueba un trigger
// que revienta cualquier UPDATE sobre movimiento_bancario.
func TestSiFallaElSegundoUpdateNoQuedaNadaAMedias(t *testing.T) {
	repo := baseDeLaReversa(t)
	sembrarCargas(t, repo)
	ctx := context.Background()

	ejecutar(t, repo, `
		CREATE FUNCTION revienta() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'falla inyectada en el UPDATE de movimientos'; END $$`)
	ejecutar(t, repo, `
		CREATE TRIGGER revienta_al_excluir BEFORE UPDATE ON movimiento_bancario
		FOR EACH ROW EXECUTE FUNCTION revienta()`)

	r := routerDeReversa(repo)
	rec := llamar(t, r, http.MethodPost, "/v1/bancos/importaciones/"+impSetiembre+"/revertir",
		`{"motivo":"se cargó en la cuenta equivocada"}`)
	if rec.Code < 500 {
		t.Fatalf("status = %d; con el segundo UPDATE reventando se esperaba un error del servidor — %s",
			rec.Code, rec.Body.String())
	}

	// LO QUE IMPORTA: el primer UPDATE no quedó escrito.
	var estado, motivo string
	var revertidaEn *time.Time
	if err := repo.pool.QueryRow(ctx,
		`SELECT estado, motivo_reversa, revertida_en FROM importacion WHERE id = $1::uuid`, impSetiembre).
		Scan(&estado, &motivo, &revertidaEn); err != nil {
		t.Fatalf("leer la carga: %v", err)
	}
	if estado != "CONFIRMADA" || motivo != "" || revertidaEn != nil {
		t.Errorf("la carga quedó a medias: estado=%q motivo=%q revertida_en=%v", estado, motivo, revertidaEn)
	}
	for nombre, incluido := range estadoDeMovimientos(t, repo) {
		if nombre == "set_ya_excluido" {
			continue // ese ya venía excluido de antes
		}
		if !incluido {
			t.Errorf("%s quedó excluido pese a que la transacción falló", nombre)
		}
	}
}
