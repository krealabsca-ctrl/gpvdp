package bancos

// «Mi partida en bancos» contra Postgres de verdad (mismo esquema temporal que
// excluido_no_suma_test.go: no se escribe en public).
//
//   1. Lo que NADIE clasificó no sale por esta puerta: ni en la lista, ni sumado al dinero de la
//      partida, ni pidiendo la vista que se quitó, ni dejándose avisar (23-set-2026, «esto no debe
//      ser visible por ningún motivo a los consultores»). El escenario siembra 13 créditos sin
//      partida en las cuentas del segmento justamente para que una fuga se vea.
//   2. La respuesta de un aviso resuelto en la fila, y «Mis avisos» del propio usuario aunque el
//      movimiento ya no esté en su alcance.
//   3. «Cargado hasta» = la cuenta del segmento más atrasada, sin mirar lo excluido.
//
// En los tres, UN ALCANCE VACÍO CIERRA. Cada prueba siembra datos que el recorte tiene que dejar
// afuera: si el recorte se abriera, la prueba los vería.
//
// Aparte de las tablas del incidente hacen falta las dos del alcance, para correr el SERVICIO real
// (AlcanceDeUsuario lee del rol) y no solo el repositorio con un alcance escrito a mano.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"
)

const (
	bancoBN           = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	cuentaSegunda     = "99999999-9999-4999-8999-999999999999" // del segmento, la más atrasada
	cuentaAjena       = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" // solo recibe plata de otra partida
	usuarioOtro       = "cccccccc-cccc-4ccc-8ccc-cccccccccccc" // mismo equipo, otra persona
	usuarioSinAlcance = "dddddddd-dddd-4ddd-8ddd-dddddddddddd" // su rol no tiene partidas
	rolEquipo         = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	rolSinPartidas    = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	otraEmpresa       = "12121212-1212-4212-8212-121212121212"
)

// sembrarMiPartida deja el escenario de diciembre de 2026 (vacío en el incidente) y devuelve el id
// de cada movimiento por su nombre.
//
// Cuentas y por qué está cada una:
//   - Promerica VDP (cuentaBuena): del segmento (tiene el depósito bueno de la partida). Recibe 12
//     créditos sin partida y sus trampas: uno EXCLUIDO, un DÉBITO y uno de OTRA partida.
//   - BN Segunda: del segmento por un crédito de la partida del 03; su movimiento más nuevo (el 28)
//     está EXCLUIDO, así que su carga llega al 03 y es la más atrasada.
//   - Promerica Colinas (cuentaMala): NO es del segmento —lo único de la partida que tuvo es el
//     duplicado excluido—. Tiene un crédito sin partida del 01 que no puede aparecer.
//   - BN Ajena: NO es del segmento, solo recibe plata de otra partida. Otro sin partida del 01.
//
// Si la definición de «cuentas del segmento» se aflojara (contara lo excluido, o cualquier cuenta),
// las dos últimas entrarían: sus créditos aparecerían en el bloque y su fecha del 01 se volvería el
// «cargado hasta».
func sembrarMiPartida(t *testing.T, repo *pgRepository) map[string]string {
	t.Helper()
	ctx := context.Background()

	ddl := []string{
		`CREATE TABLE usuario_empresa_rol (LIKE public.usuario_empresa_rol INCLUDING ALL)`,
		`CREATE TABLE rol_clasificacion_consulta (LIKE public.rol_clasificacion_consulta INCLUDING ALL)`,
	}
	for _, q := range ddl {
		if _, err := repo.pool.Exec(ctx, q); err != nil {
			t.Fatalf("crear tabla del alcance: %v", err)
		}
	}

	catalogo := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'Asociaciones / Cooperativas')`,
			[]any{partidaAjena, empresaPrueba, conceptoID}},
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'BN')`,
			[]any{bancoBN, empresaPrueba}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'BN Segunda')`,
			[]any{cuentaSegunda, empresaPrueba, bancoBN}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'BN Ajena')`,
			[]any{cuentaAjena, empresaPrueba, bancoBN}},
		{`INSERT INTO usuario (id, nombre, email, password_hash)
		  VALUES ($1::uuid, 'Otra del equipo', 'otra@prueba.local', 'x')`, []any{usuarioOtro}},
		{`INSERT INTO usuario (id, nombre, email, password_hash)
		  VALUES ($1::uuid, 'Sin partidas', 'sinpartidas@prueba.local', 'x')`, []any{usuarioSinAlcance}},
		{`INSERT INTO usuario_empresa_rol (empresa_id, usuario_id, rol_id) VALUES ($1::uuid, $2::uuid, $3::uuid)`,
			[]any{empresaPrueba, usuarioPrueba, rolEquipo}},
		{`INSERT INTO usuario_empresa_rol (empresa_id, usuario_id, rol_id) VALUES ($1::uuid, $2::uuid, $3::uuid)`,
			[]any{empresaPrueba, usuarioOtro, rolEquipo}},
		{`INSERT INTO usuario_empresa_rol (empresa_id, usuario_id, rol_id) VALUES ($1::uuid, $2::uuid, $3::uuid)`,
			[]any{empresaPrueba, usuarioSinAlcance, rolSinPartidas}},
		// El equipo consulta UNA partida; el otro rol, ninguna.
		{`INSERT INTO rol_clasificacion_consulta (empresa_id, rol_id, clasificacion_id) VALUES ($1::uuid, $2::uuid, $3::uuid)`,
			[]any{empresaPrueba, rolEquipo, partidaMia}},
	}
	for _, c := range catalogo {
		if _, err := repo.pool.Exec(ctx, c.q, c.args...); err != nil {
			t.Fatalf("sembrar catálogo de «Mi partida»: %v", err)
		}
	}

	const q = `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
			 estado_clasificacion, natural_key, incluido)
		VALUES ($1::uuid, $2::uuid, $3::date, $4, $5, $6, $7, 'CRC', $8, $8,
		        NULLIF($9,'')::uuid, NULLIF($10,'')::uuid, $11, $12, $13)
		RETURNING id::text`
	ids := map[string]string{}
	mov := func(nombre, cuenta, fecha, debito, credito, partida string, incluido bool) {
		monto, concepto, estado := credito, "", "NO_IDENTIFICADO"
		if debito != "0" {
			monto = debito
		}
		if partida != "" {
			concepto, estado = conceptoID, "REVISADO"
		}
		var id string
		if err := repo.pool.QueryRow(ctx, q, empresaPrueba, cuenta, fecha, "DOC-"+nombre,
			"DEPOSITO "+strings.ToUpper(nombre), debito, credito, monto, concepto, partida, estado,
			"mp-"+nombre, incluido).Scan(&id); err != nil {
			t.Fatalf("sembrar %s: %v", nombre, err)
		}
		ids[nombre] = id
	}

	// Promerica VDP (del segmento): 12 créditos sin partida de ₡100.
	for dia := 1; dia <= 12; dia++ {
		mov(fmt.Sprintf("sp-%02d", dia), cuentaBuena, fmt.Sprintf("2026-12-%02d", dia), "0", "100.00", "", true)
	}
	mov("sp-excluido", cuentaBuena, "2026-12-13", "0", "50000.00", "", false) // sin partida, pero excluido
	mov("sp-debito", cuentaBuena, "2026-12-13", "70000.00", "0", "", true)    // sin partida, pero débito
	mov("ajeno-en-buena", cuentaBuena, "2026-12-14", "0", "30000.00", partidaAjena, true)
	mov("mio-1", cuentaBuena, "2026-12-14", "0", "1000.00", partidaMia, true)
	mov("mio-2", cuentaBuena, "2026-12-15", "0", "1000.00", partidaMia, true) // lo más nuevo de la cuenta

	// BN Segunda (del segmento): la partida el 03, un sin partida el 02, y lo del 28 excluido.
	mov("mio-segunda", cuentaSegunda, "2026-12-03", "0", "500.00", partidaMia, true)
	mov("sp-segunda", cuentaSegunda, "2026-12-02", "0", "200.00", "", true)
	mov("segunda-excluido", cuentaSegunda, "2026-12-28", "0", "8000.00", "", false)

	// Fuera del segmento.
	mov("mala-sin-partida", cuentaMala, "2026-12-01", "0", "999.00", "", true)
	mov("ajena-clasif", cuentaAjena, "2026-11-20", "0", "40000.00", partidaAjena, true)
	mov("ajena-sin-partida", cuentaAjena, "2026-12-01", "0", "555.00", "", true)
	return ids
}

// baseMiPartida levanta el esquema temporal con el incidente y el escenario de diciembre.
func baseMiPartida(t *testing.T) (*pgRepository, *Service, map[string]string) {
	t.Helper()
	repo := baseDelIncidente(t)
	sembrarIncidente(t, repo)
	ids := sembrarMiPartida(t, repo)
	return repo, NewService(repo, nil, zap.NewNop(), true), ids
}

// Del bloque: 12 × ₡100 en Promerica VDP + ₡200 en BN Segunda.
const (
	filasSinPartida  = 13
	dineroSinPartida = "1400.00"
)

// Lo que NADIE clasificó no sale por esta puerta (23-set-2026).
//
// El escenario tiene 13 créditos sin partida en las cuentas del segmento —los que la pestaña
// «Todavía sin partida» mostraba, ₡1.400— y esta prueba es la que dice que ya no se ven: ni en la
// lista, ni sumados al dinero de la partida, ni pidiendo la vista que se quitó.
func TestLoSinPartidaNoSaleDeMiPartida(t *testing.T) {
	_, svc, ids := baseMiPartida(t)
	ctx := context.Background()
	diciembre := FiltrosMovimientos{Periodo: "2026-12", PageSize: 100}

	partida, err := svc.MiSegmento(ctx, empresaPrueba, usuarioPrueba, VistaPartida, diciembre)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// mio-1 + mio-2 + mio-segunda. Con lo sin partida sumado serían ₡3.900: el total de la partida
	// afirmaría plata que nadie clasificó todavía como suya.
	if partida.Movimientos.Total != 3 {
		t.Errorf("filas de la partida = %d, se esperaban 3", partida.Movimientos.Total)
	}
	if !igualDecimal(t, partida.Movimientos.Totales.TotalCreditos, "2500.00") {
		t.Errorf("total de la partida = %s, se esperaba 2500.00 (sin los ₡1.400 sin partida)",
			partida.Movimientos.Totales.TotalCreditos)
	}
	for _, it := range partida.Movimientos.Items {
		if it.ClasificacionID == nil || *it.ClasificacionID != partidaMia {
			t.Errorf("apareció %s sin ser de la partida", it.ID)
		}
	}

	t.Run("ni uno solo de los 13 sin partida aparece, en ningún período", func(t *testing.T) {
		prohibidos := map[string]string{}
		for nombre, id := range ids {
			if strings.HasPrefix(nombre, "sp-") || strings.Contains(nombre, "sin-partida") {
				prohibidos[id] = nombre
			}
		}
		if len(prohibidos) < filasSinPartida {
			t.Fatalf("control: la lista de prohibidos tiene %d, se esperaban al menos %d", len(prohibidos), filasSinPartida)
		}
		// Sin período: todo el histórico, que es donde una fuga se escondería mejor.
		res, err := svc.MiSegmento(ctx, empresaPrueba, usuarioPrueba, VistaPartida, FiltrosMovimientos{PageSize: 500})
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		for _, it := range res.Movimientos.Items {
			if nombre, malo := prohibidos[it.ID]; malo {
				t.Errorf("«%s» se coló en «Mi partida»", nombre)
			}
		}
	})

	t.Run("la vista que los mostraba ya no se sirve", func(t *testing.T) {
		res, err := svc.MiSegmento(ctx, empresaPrueba, usuarioPrueba, "sin_clasificar", diciembre)
		if !errors.Is(err, ErrVistaInvalida) {
			t.Fatalf("esperaba ErrVistaInvalida, obtuve %v", err)
		}
		if len(res.Movimientos.Items) != 0 {
			t.Fatalf("devolvió %d filas con la vista que se quitó", len(res.Movimientos.Items))
		}
	})

	t.Run("el desplegable sigue ofreciendo las cuentas del segmento", func(t *testing.T) {
		// Quitar la pestaña no toca el filtro: las cuentas salen del alcance, no de lo sin clasificar.
		if len(partida.Cuentas) != 2 || partida.Cuentas[0].Cuenta != "BN Segunda" ||
			partida.Cuentas[1].Cuenta != "Promerica VDP" {
			t.Errorf("cuentas del segmento = %+v, se esperaban BN Segunda y Promerica VDP", partida.Cuentas)
		}
	})
}

func TestMiPartidaAlcanceVacioCierra(t *testing.T) {
	repo, svc, _ := baseMiPartida(t)
	ctx := context.Background()

	t.Run("por el servicio, un rol sin partidas no ve nada", func(t *testing.T) {
		res, err := svc.MiSegmento(ctx, empresaPrueba, usuarioSinAlcance, VistaPartida, FiltrosMovimientos{})
		if !errors.Is(err, ErrSinAlcance) {
			t.Fatalf("esperaba ErrSinAlcance, obtuve %v", err)
		}
		if len(res.Movimientos.Items) != 0 || len(res.Cuentas) != 0 {
			t.Fatalf("sin alcance devolvió %d filas y %d cuentas", len(res.Movimientos.Items), len(res.Cuentas))
		}
	})

	t.Run("las cuentas del segmento también salen vacías", func(t *testing.T) {
		got, err := repo.CuentasDelAlcance(ctx, empresaPrueba, []string{})
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("cuentas = %+v, se esperaban cero", got)
		}
	})
}

// La guarda del aviso, después del cierre del 23-set-2026: se puede avisar sobre lo que se VE, y
// sobre nada más. Hasta el 22 aceptaba además los créditos sin clasificar de las cuentas del
// segmento, porque la pestaña «Todavía sin partida» los mostraba; sin la pestaña, aceptarlos dejaría
// confirmar por id la existencia de un movimiento que la pantalla ya no enseña.
func TestSoloSePuedeAvisarSobreLoQueSeVe(t *testing.T) {
	repo, svc, ids := baseMiPartida(t)
	ctx := context.Background()
	alcance := []string{partidaMia}

	casos := []struct {
		mov     string
		visible bool
		porque  string
	}{
		{"mio-1", true, "crédito de la partida: es lo único que se ve"},
		{"mio-segunda", true, "crédito de la partida en la otra cuenta del segmento"},
		{"sp-01", false, "nadie lo clasificó: ya no se muestra, así que no se avisa"},
		{"sp-segunda", false, "que la cuenta sea del segmento no lo hace visible"},
		{"sp-excluido", false, "está excluido"},
		{"sp-debito", false, "es un débito"},
		{"ajeno-en-buena", false, "tiene OTRA partida"},
		{"mala-sin-partida", false, "su cuenta no es del segmento"},
		{"ajena-sin-partida", false, "su cuenta no es del segmento"},
	}
	for _, c := range casos {
		t.Run(c.mov, func(t *testing.T) {
			ok, err := repo.MovimientoEnAlcance(ctx, empresaPrueba, ids[c.mov], alcance)
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if ok != c.visible {
				t.Fatalf("MovimientoEnAlcance(%s) = %v, se esperaba %v: %s", c.mov, ok, c.visible, c.porque)
			}
		})
	}

	t.Run("con alcance vacío no se puede avisar sobre nada", func(t *testing.T) {
		ok, err := repo.MovimientoEnAlcance(ctx, empresaPrueba, ids["mio-1"], []string{})
		if err != nil || ok {
			t.Fatalf("MovimientoEnAlcance con alcance vacío = %v (%v)", ok, err)
		}
	})

	t.Run("por el servicio: se avisa sobre el de la partida y NO sobre lo sin clasificar", func(t *testing.T) {
		if err := svc.ReportarSegmentacion(ctx, empresaPrueba, usuarioPrueba, ids["mio-1"],
			"Esto es de Emergencias, no de Depósitos"); err != nil {
			t.Fatalf("avisar sobre un crédito de la partida: %v", err)
		}
		for _, mov := range []string{"sp-01", "ajena-sin-partida"} {
			err := svc.ReportarSegmentacion(ctx, empresaPrueba, usuarioPrueba, ids[mov], "es mío")
			if !errors.Is(err, ErrFueraDeAlcance) {
				t.Fatalf("avisar sobre %s: esperaba ErrFueraDeAlcance, obtuve %v", mov, err)
			}
		}
		// Y el único aviso quedó en la cola de quien clasifica, donde se corrige.
		cola, err := repo.ListarReportesSegmentacion(ctx, empresaPrueba, true)
		if err != nil {
			t.Fatalf("listar la cola: %v", err)
		}
		if len(cola) != 1 || cola[0].MovimientoID != ids["mio-1"] {
			t.Fatalf("cola = %+v, se esperaba el aviso sobre mio-1", cola)
		}
		// Y la fila lo muestra en revisión.
		res, err := svc.MiSegmento(ctx, empresaPrueba, usuarioPrueba, VistaPartida,
			FiltrosMovimientos{Periodo: "2026-12", PageSize: 100})
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		marcadas := 0
		for _, it := range res.Movimientos.Items {
			if it.ReporteAbierto != "" {
				marcadas++
				if it.ID != ids["mio-1"] {
					t.Errorf("quedó marcada en revisión una fila que nadie avisó: %s", it.ID)
				}
			}
		}
		if marcadas != 1 {
			t.Errorf("filas en revisión = %d, se esperaba 1", marcadas)
		}
	})

	t.Run("un rol sin partidas no puede avisar sobre nada", func(t *testing.T) {
		err := svc.ReportarSegmentacion(ctx, empresaPrueba, usuarioSinAlcance, ids["mio-2"], "es mío")
		if !errors.Is(err, ErrSinAlcance) {
			t.Fatalf("esperaba ErrSinAlcance, obtuve %v", err)
		}
	})
}

func TestAvisoResueltoSeVeEnLaFila(t *testing.T) {
	repo, svc, ids := baseMiPartida(t)
	ctx := context.Background()
	diciembre := FiltrosMovimientos{Periodo: "2026-12", PageSize: 100}

	filaDe := func(t *testing.T, usuario, vista, mov string) MovimientoRow {
		t.Helper()
		res, err := svc.MiSegmento(ctx, empresaPrueba, usuario, vista, diciembre)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		for _, it := range res.Movimientos.Items {
			if it.ID == ids[mov] {
				return it
			}
		}
		t.Fatalf("%s no está en la vista %s", mov, vista)
		return MovimientoRow{}
	}
	fila := func(t *testing.T, vista, mov string) MovimientoRow {
		t.Helper()
		return filaDe(t, usuarioPrueba, vista, mov)
	}
	avisarYResolver := func(t *testing.T, mov, usuario, motivo, resolucion, respuesta string) {
		t.Helper()
		repID, err := repo.CrearReporteSegmentacion(ctx, empresaPrueba, ids[mov], usuario, motivo)
		if err != nil {
			t.Fatalf("crear aviso: %v", err)
		}
		if err := repo.ResolverReporteSegmentacion(ctx, empresaPrueba, repID, usuarioPrueba, resolucion, respuesta); err != nil {
			t.Fatalf("resolver aviso: %v", err)
		}
	}

	// Lo avisó OTRA persona del equipo. Su motivo y la respuesta que recibió son de ella (decisión
	// conservadora del 22-set-2026, hasta que el Director Financiero decida si se abre): en MI fila no
	// aparece nada; en la de ella, sí.
	avisarYResolver(t, "mio-1", usuarioOtro, "esto no es de Depósitos", ResolucionSinCambio,
		"Es de ustedes: es la planilla de setiembre")

	t.Run("el resuelto de otra persona no aparece en mi fila", func(t *testing.T) {
		it := fila(t, VistaPartida, "mio-1")
		if it.ReporteAbierto != "" || it.AvisoResuelto != nil {
			t.Fatalf("mi fila trae el aviso de otra persona: reporte_abierto=%q aviso_resuelto=%+v",
				it.ReporteAbierto, it.AvisoResuelto)
		}
		// Control: a quien avisó sí le llega, en su fila.
		suya := filaDe(t, usuarioOtro, VistaPartida, "mio-1")
		if suya.AvisoResuelto == nil || suya.AvisoResuelto.Respuesta != "Es de ustedes: es la planilla de setiembre" {
			t.Fatalf("a quien avisó no le llega su respuesta: %+v", suya.AvisoResuelto)
		}
	})

	t.Run("resuelto y sin aviso abierto: la respuesta y cuándo", func(t *testing.T) {
		avisarYResolver(t, "mio-1", usuarioPrueba, "sigue sin cuadrarme", ResolucionSinCambio,
			"Lo revisé con el recibo: es de ustedes")
		it := fila(t, VistaPartida, "mio-1")
		if it.ReporteAbierto != "" {
			t.Fatalf("no hay aviso abierto y la fila dice %q", it.ReporteAbierto)
		}
		if it.AvisoResuelto == nil || it.AvisoResuelto.Respuesta != "Lo revisé con el recibo: es de ustedes" ||
			it.AvisoResuelto.Resolucion != ResolucionSinCambio || it.AvisoResuelto.ResueltoEn == "" {
			t.Fatalf("aviso resuelto = %+v", it.AvisoResuelto)
		}
	})

	t.Run("con dos resueltos míos se ve el último mío, aunque después responda a otro", func(t *testing.T) {
		avisarYResolver(t, "mio-1", usuarioPrueba, "tercera vez", ResolucionSinCambio,
			"Ya lo cruzamos con el banco: es de ustedes")
		// Resuelto DESPUÉS del mío: sin el recorte por usuario, DISTINCT ON se quedaría con este.
		avisarYResolver(t, "mio-1", usuarioOtro, "otra vez yo", ResolucionSinCambio, "Respuesta para la otra persona")
		it := fila(t, VistaPartida, "mio-1")
		if it.AvisoResuelto == nil || it.AvisoResuelto.Respuesta != "Ya lo cruzamos con el banco: es de ustedes" {
			t.Fatalf("aviso resuelto = %+v, se esperaba mi última respuesta", it.AvisoResuelto)
		}
	})

	t.Run("si se vuelve a avisar, manda el abierto", func(t *testing.T) {
		if _, err := repo.CrearReporteSegmentacion(ctx, empresaPrueba, ids["mio-1"], usuarioPrueba,
			"tercera vez: no es nuestro"); err != nil {
			t.Fatalf("crear aviso: %v", err)
		}
		it := fila(t, VistaPartida, "mio-1")
		if it.ReporteAbierto != "tercera vez: no es nuestro" || it.AvisoResuelto != nil {
			t.Fatalf("reporte_abierto = %q, aviso_resuelto = %+v", it.ReporteAbierto, it.AvisoResuelto)
		}
		if it.ReporteAbiertoPropio == nil || !*it.ReporteAbiertoPropio {
			t.Fatalf("el aviso abierto es mío y la fila no lo marca como propio: %s", valorPropio(it.ReporteAbiertoPropio))
		}
	})

	t.Run("también en la otra cuenta del segmento", func(t *testing.T) {
		avisarYResolver(t, "mio-segunda", usuarioPrueba, "esto no es de mi partida", ResolucionSinCambio,
			"Todavía no: el cliente no mandó el comprobante")
		it := fila(t, VistaPartida, "mio-segunda")
		if it.AvisoResuelto == nil || it.AvisoResuelto.Respuesta != "Todavía no: el cliente no mandó el comprobante" {
			t.Fatalf("aviso resuelto = %+v", it.AvisoResuelto)
		}
	})
}

// clavesDeMiAviso es TODO lo que «Mis avisos» puede decir: lo que el usuario vio al avisar, su
// motivo, el estado y la respuesta. Una clave nueva tiene que pasar por esta lista a propósito.
var clavesDeMiAviso = map[string]bool{
	"id": true, "es_faltante": true, "motivo": true, "creado_en": true, "fecha": true,
	"documento": true, "monto": true, "moneda": true, "banco": true, "cuenta": true,
	"referencia": true, "estado": true, "resolucion": true, "respuesta": true, "resuelto_en": true,
}

func TestMisAvisos(t *testing.T) {
	repo, svc, ids := baseMiPartida(t)
	ctx := context.Background()

	// (a) Un aviso abierto sobre un movimiento de la partida.
	abierto, err := repo.CrearReporteSegmentacion(ctx, empresaPrueba, ids["mio-1"], usuarioPrueba, "no es de Depósitos")
	if err != nil {
		t.Fatalf("aviso abierto: %v", err)
	}
	// (b) EL HUÉRFANO: se avisó, se reclasificó a OTRA partida y se resolvió. El movimiento salió del
	// alcance justo cuando llegó la respuesta.
	huerfano, err := repo.CrearReporteSegmentacion(ctx, empresaPrueba, ids["mio-2"], usuarioPrueba, "esto es de Asociaciones")
	if err != nil {
		t.Fatalf("aviso huérfano: %v", err)
	}
	if err := repo.ReclasificarMovimiento(ctx, empresaPrueba, ids["mio-2"], conceptoID, partidaAjena); err != nil {
		t.Fatalf("reclasificar: %v", err)
	}
	if err := repo.ResolverReporteSegmentacion(ctx, empresaPrueba, huerfano, usuarioOtro,
		ResolucionReclasificado, "Tenían razón: lo pasé a Asociaciones"); err != nil {
		t.Fatalf("resolver: %v", err)
	}
	// (c) Un faltante que el servidor enganchó a un movimiento de OTRA partida: el usuario nunca lo
	// vio (la búsqueda solo le dijo «existe, no es tuyo»), así que nada de ese movimiento puede viajar.
	faltante, err := repo.CrearReporteFaltante(ctx, empresaPrueba, usuarioPrueba, "2026-12-14", dec("30000"),
		"REC-77", "falta mi depósito de ventanilla", ids["ajeno-en-buena"])
	if err != nil {
		t.Fatalf("faltante: %v", err)
	}
	// Lo que NO puede aparecer: el aviso de otra persona del equipo, el de la misma persona en otra
	// empresa, y el de alguien cuyo rol se quedó sin partidas.
	delOtro, err := repo.CrearReporteSegmentacion(ctx, empresaPrueba, ids["mio-segunda"], usuarioOtro, "esto es mío")
	if err != nil {
		t.Fatalf("aviso de otro: %v", err)
	}
	deOtraEmpresa, err := repo.CrearReporteFaltante(ctx, otraEmpresa, usuarioPrueba, "2026-12-20", dec("1234"),
		"", "en la otra empresa", "")
	if err != nil {
		t.Fatalf("aviso de otra empresa: %v", err)
	}
	if _, err := repo.CrearReporteFaltante(ctx, empresaPrueba, usuarioSinAlcance, "2026-12-21", dec("4321"),
		"", "cuando todavía tenía partida", ""); err != nil {
		t.Fatalf("aviso del que se quedó sin partidas: %v", err)
	}

	res, err := svc.MisAvisos(ctx, empresaPrueba, usuarioPrueba, 1, 50)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	porID := map[string]MiAviso{}
	for _, a := range res.Items {
		porID[a.ID] = a
	}

	t.Run("solo los suyos y de esta empresa", func(t *testing.T) {
		if res.Total != 3 || res.Abiertos != 2 || res.Resueltos != 1 || len(res.Items) != 3 {
			t.Fatalf("total=%d abiertos=%d resueltos=%d items=%d; se esperaban 3/2/1/3",
				res.Total, res.Abiertos, res.Resueltos, len(res.Items))
		}
		for _, id := range []string{abierto, huerfano, faltante} {
			if _, ok := porID[id]; !ok {
				t.Errorf("falta el aviso %s", id)
			}
		}
		if _, ok := porID[delOtro]; ok {
			t.Error("apareció el aviso de OTRA persona")
		}
		if _, ok := porID[deOtraEmpresa]; ok {
			t.Error("apareció un aviso de OTRA empresa")
		}
		if res.SinAlcance {
			t.Error("tiene alcance y la respuesta dice sin_alcance")
		}
	})

	t.Run("el huérfano sigue apareciendo, con lo que vio al avisar y la respuesta", func(t *testing.T) {
		// Control: de verdad salió del alcance.
		partida, err := svc.MiSegmento(ctx, empresaPrueba, usuarioPrueba, VistaPartida,
			FiltrosMovimientos{Periodo: "2026-12", PageSize: 100})
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		for _, it := range partida.Movimientos.Items {
			if it.ID == ids["mio-2"] {
				t.Fatal("control: mio-2 sigue en la partida, el caso no es huérfano")
			}
		}
		a := porID[huerfano]
		if a.Estado != AvisoResueltoEstado || a.Resolucion != ResolucionReclasificado ||
			a.Respuesta != "Tenían razón: lo pasé a Asociaciones" || a.ResueltoEn == "" {
			t.Fatalf("estado/respuesta del huérfano = %+v", a)
		}
		if a.EsFaltante || a.Fecha != "2026-12-15" || a.Documento != "DOC-mio-2" ||
			!igualDecimal(t, a.Monto, "1000") || a.Moneda != "CRC" ||
			a.Banco != "Promerica" || a.Cuenta != "Promerica VDP" || a.Motivo != "esto es de Asociaciones" {
			t.Fatalf("lo que vio al avisar llegó mal: %+v", a)
		}
	})

	t.Run("el faltante enganchado NO trae nada del movimiento que nunca vio", func(t *testing.T) {
		a := porID[faltante]
		if !a.EsFaltante || a.Fecha != "2026-12-14" || !igualDecimal(t, a.Monto, "30000") ||
			a.Referencia != "REC-77" || a.Estado != AvisoEnRevision {
			t.Fatalf("faltante = %+v", a)
		}
		if a.Documento != "" || a.Banco != "" || a.Cuenta != "" || a.Moneda != "" {
			t.Fatalf("el faltante trae datos del movimiento enganchado, que es de otra partida: %+v", a)
		}
	})

	t.Run("no expone la partida actual ni otros campos del movimiento", func(t *testing.T) {
		b, err := json.Marshal(res.Items)
		if err != nil {
			t.Fatalf("serializar: %v", err)
		}
		var crudos []map[string]any
		if err := json.Unmarshal(b, &crudos); err != nil {
			t.Fatalf("deserializar: %v", err)
		}
		for _, c := range crudos {
			for k := range c {
				if !clavesDeMiAviso[k] {
					t.Errorf("«Mis avisos» expone la clave %q, que no es de lo que el usuario vio al avisar", k)
				}
			}
		}
		texto := string(b)
		// La partida a la que se reclasificó el huérfano, y el documento y la descripción del
		// movimiento que el servidor enganchó al faltante.
		for _, prohibido := range []string{"Cooperativas", "DOC-ajeno-en-buena", "AJENO-EN-BUENA", ids["mio-2"], ids["ajeno-en-buena"]} {
			if strings.Contains(texto, prohibido) {
				t.Errorf("la respuesta contiene %q", prohibido)
			}
		}
	})

	t.Run("primero lo que sigue en revisión", func(t *testing.T) {
		if res.Items[len(res.Items)-1].ID != huerfano {
			t.Fatalf("el resuelto tiene que ir después de los abiertos: %+v", res.Items)
		}
	})

	t.Run("paginado con total real", func(t *testing.T) {
		vistos := map[string]int{}
		for page := 1; page <= 2; page++ {
			pag, err := svc.MisAvisos(ctx, empresaPrueba, usuarioPrueba, page, 2)
			if err != nil {
				t.Fatalf("página %d: %v", page, err)
			}
			if pag.Total != 3 || pag.Page != page || pag.PageSize != 2 {
				t.Fatalf("página %d: total=%d page=%d page_size=%d", page, pag.Total, pag.Page, pag.PageSize)
			}
			for _, a := range pag.Items {
				vistos[a.ID]++
			}
		}
		if len(vistos) != 3 {
			t.Fatalf("avisos distintos recorriendo las páginas = %d, se esperaban 3", len(vistos))
		}
	})

	t.Run("la otra persona del equipo ve solo el suyo", func(t *testing.T) {
		otro, err := svc.MisAvisos(ctx, empresaPrueba, usuarioOtro, 1, 50)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if otro.Total != 1 || len(otro.Items) != 1 || otro.Items[0].ID != delOtro {
			t.Fatalf("avisos de la otra persona = %+v", otro)
		}
	})

	t.Run("la misma persona en otra empresa ve solo los de esa empresa", func(t *testing.T) {
		// Por el repositorio: en la otra empresa esta persona no tiene rol, así que el servicio ya
		// cerraría antes. Lo que se prueba acá es que la consulta filtra por la empresa del token.
		got, err := repo.MisAvisos(ctx, otraEmpresa, usuarioPrueba, 1, 50)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if got.Total != 1 || len(got.Items) != 1 || got.Items[0].ID != deOtraEmpresa {
			t.Fatalf("en la otra empresa = %+v", got)
		}
	})

	t.Run("alcance vacío cierra", func(t *testing.T) {
		// Control: el aviso existe y el repositorio lo devolvería.
		crudo, err := repo.MisAvisos(ctx, empresaPrueba, usuarioSinAlcance, 1, 50)
		if err != nil || crudo.Total != 1 {
			t.Fatalf("control: total = %d (%v)", crudo.Total, err)
		}
		got, err := svc.MisAvisos(ctx, empresaPrueba, usuarioSinAlcance, 1, 50)
		if !errors.Is(err, ErrSinAlcance) {
			t.Fatalf("esperaba ErrSinAlcance, obtuve %v", err)
		}
		if !got.SinAlcance || len(got.Items) != 0 || got.Total != 0 {
			t.Fatalf("un rol sin partidas vio %d avisos: %+v", len(got.Items), got)
		}
	})
}

func TestCargadoHastaEsLaCuentaDelSegmentoMasAtrasada(t *testing.T) {
	repo, svc, _ := baseMiPartida(t)
	ctx := context.Background()

	t.Run("por cuenta, sin lo excluido y solo las del segmento", func(t *testing.T) {
		got, err := repo.CargaDeCuentasDelSegmento(ctx, empresaPrueba, []string{partidaMia})
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		// BN Segunda al 03 (su 28 está excluido) y Promerica VDP al 15. Promerica Colinas y BN Ajena,
		// al 01, NO son del segmento: si entraran, el «cargado hasta» sería el 01.
		if len(got) != 2 {
			t.Fatalf("cuentas = %+v, se esperaban las 2 del segmento", got)
		}
		if got[0].Cuenta != "BN Segunda" || got[0].CargadoHasta != "2026-12-03" {
			t.Errorf("la primera = %+v, se esperaba BN Segunda al 2026-12-03 (el 28 está excluido)", got[0])
		}
		if got[1].Cuenta != "Promerica VDP" || got[1].CargadoHasta != "2026-12-15" {
			t.Errorf("la segunda = %+v, se esperaba Promerica VDP al 2026-12-15", got[1])
		}
	})

	t.Run("la pantalla nombra la más atrasada", func(t *testing.T) {
		res, err := svc.MiSegmento(ctx, empresaPrueba, usuarioPrueba, VistaPartida, FiltrosMovimientos{Periodo: "2026-12"})
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		// La empresa llega al 15 (Promerica VDP): el dato de antes habría dicho «cargado hasta el 15».
		if res.CargadoHasta != "2026-12-03" {
			t.Fatalf("cargado hasta = %q, se esperaba 2026-12-03", res.CargadoHasta)
		}
		if res.CuentaMasAtrasada == nil || res.CuentaMasAtrasada.ID != cuentaSegunda ||
			res.CuentaMasAtrasada.Banco != "BN" || res.CuentaMasAtrasada.Cuenta != "BN Segunda" {
			t.Fatalf("cuenta más atrasada = %+v, se esperaba BN · BN Segunda", res.CuentaMasAtrasada)
		}
		if len(res.CargaPorCuenta) != 2 {
			t.Fatalf("carga por cuenta = %+v", res.CargaPorCuenta)
		}

		// Y es la MISMA fecha con la que «Falta un movimiento» dice «conviene esperar».
		busq, err := svc.BuscarFaltante(ctx, empresaPrueba, usuarioPrueba, "2026-12-10", "123456")
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if busq.Veredicto != FaltanteNoExiste || busq.CargadoHasta != res.CargadoHasta ||
			busq.CargadoHastaCuenta == nil || busq.CargadoHastaCuenta.ID != cuentaSegunda {
			t.Fatalf("el diálogo dice %q / %+v y el encabezado %q: se contradicen",
				busq.CargadoHasta, busq.CargadoHastaCuenta, res.CargadoHasta)
		}
	})

	t.Run("alcance vacío cierra", func(t *testing.T) {
		got, err := repo.CargaDeCuentasDelSegmento(ctx, empresaPrueba, []string{})
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("con alcance vacío devolvió %+v", got)
		}
		res, err := svc.MiSegmento(ctx, empresaPrueba, usuarioSinAlcance, VistaPartida, FiltrosMovimientos{})
		if !errors.Is(err, ErrSinAlcance) {
			t.Fatalf("esperaba ErrSinAlcance, obtuve %v", err)
		}
		if res.CargadoHasta != "" || res.CuentaMasAtrasada != nil || len(res.CargaPorCuenta) != 0 {
			t.Fatalf("un rol sin partidas recibió una fecha de carga: %q / %+v", res.CargadoHasta, res.CuentaMasAtrasada)
		}
	})
}
