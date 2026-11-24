package conta

// El generador de asientos por caja (Fase 3B), contra Postgres.
//
// LO QUE SE PRUEBA ACÁ es la regla —la dirección del asiento la da el signo del movimiento— y las
// tres propiedades que hacen que generar en masa sea seguro: que no duplique, que no deje huecos
// en la numeración y que se pueda deshacer.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
)

var tablasDelGenerador = []string{
	"empresa", "usuario", "cuenta_contable",
	"banco", "cuenta_bancaria", "concepto", "clasificacion", "movimiento_bancario",
	"mapeo_contable", "mapeo_cuenta_bancaria",
	"conta_consecutivo", "asiento", "asiento_linea",
}

func baseGenerador(t *testing.T) *pgRepository {
	t.Helper()
	return baseConta(t, tablasDelGenerador, []string{
		disparadorDelArbol, disparadorMapeoCB, disparadorCuadraCab, disparadorCuadraLin,
		disparadorLineaCuenta,
	})
}

// escenario deja listo un período con N movimientos contabilizables.
type escenario struct {
	repo             *pgRepository
	svc              *Service
	gasto            Cuenta
	banco            Cuenta
	cuentaBancariaID string
	clasifID         string
}

func prepararEscenario(t *testing.T) *escenario {
	t.Helper()
	repo := baseGenerador(t)
	ctx := context.Background()
	gasto, banco := dosCuentas(t, repo)

	// Banco del ERP + su enlace contable.
	var bancoID, cuentaBancariaID string
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO banco (empresa_id, nombre) VALUES ($1::uuid, 'BN') RETURNING id::text`,
		empUno).Scan(&bancoID); err != nil {
		t.Fatalf("sembrar banco: %v", err)
	}
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO cuenta_bancaria (empresa_id, banco_id, iban, moneda, alias)
		VALUES ($1::uuid, $2::uuid, 'CR15015108710010019890', 'CRC', 'BN Colones')
		RETURNING id::text`, empUno, bancoID).Scan(&cuentaBancariaID); err != nil {
		t.Fatalf("sembrar cuenta bancaria: %v", err)
	}
	if err := repo.GuardarEnlaceBanco(ctx, empUno, cuentaBancariaID, banco.ID, usrPrueba); err != nil {
		t.Fatalf("enlazar banco: %v", err)
	}

	// Partida + su mapeo.
	clasifID := sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Papelería", 0)
	if err := repo.GuardarMapeo(ctx, empUno,
		DecisionDeMapeo{ClasificacionID: clasifID, Estado: EstadoMapeada, CuentaID: gasto.ID}, usrPrueba); err != nil {
		t.Fatalf("mapear: %v", err)
	}

	return &escenario{
		repo: repo, svc: NewService(repo, nil, zap.NewNop()),
		gasto: gasto, banco: banco,
		cuentaBancariaID: cuentaBancariaID, clasifID: clasifID,
	}
}

// movimiento siembra uno y devuelve su id.
func (e *escenario) movimiento(t *testing.T, fecha, monto string, esDebito bool, clasif string) string {
	t.Helper()
	debito, credito := "0", monto
	if esDebito {
		debito, credito = monto, "0"
	}
	var id string
	q := `INSERT INTO movimiento_bancario
	        (empresa_id, cuenta_bancaria_id, fecha, natural_key, clasificacion_id,
	         debito, credito, monto_crc, descripcion)
	      VALUES ($1::uuid, $2::uuid, $3::date, $4, NULLIF($5,'')::uuid, $6, $7, $8, 'compra')
	      RETURNING id::text`
	if err := e.repo.pool.QueryRow(context.Background(), q,
		empUno, e.cuentaBancariaID, fecha, fecha+monto+debito, clasif,
		debito, credito, monto).Scan(&id); err != nil {
		t.Fatalf("sembrar movimiento: %v", err)
	}
	return id
}

// LA REGLA: la dirección la da el SIGNO del movimiento, no un criterio.
//
//	débito  (salió plata) → Debe la partida, Haber el banco
//	crédito (entró plata) → Debe el banco,  Haber la partida
func TestLaDireccionDelAsientoLaDaElSignoDelMovimiento(t *testing.T) {
	e := prepararEscenario(t)
	ctx := context.Background()
	e.movimiento(t, "2026-08-05", "1000.00", true, e.clasifID)  // salió
	e.movimiento(t, "2026-08-06", "2500.00", false, e.clasifID) // entró

	res, err := e.svc.Generar(ctx, empUno, "2026-08", usrPrueba)
	if err != nil {
		t.Fatalf("generar: %v", err)
	}
	if res.Creados != 2 {
		t.Fatalf("creados %d, se esperaban 2", res.Creados)
	}

	asientos, err := e.repo.Diario(ctx, empUno, FiltroDiario{})
	if err != nil {
		t.Fatalf("diario: %v", err)
	}
	for _, a := range asientos {
		completo, err := e.repo.Asiento(ctx, empUno, a.ID)
		if err != nil {
			t.Fatalf("leer asiento: %v", err)
		}
		var debeEnGasto, haberEnGasto, debeEnBanco, haberEnBanco string
		for _, l := range completo.Lineas {
			switch l.CuentaID {
			case e.gasto.ID:
				debeEnGasto, haberEnGasto = l.Debe, l.Haber
			case e.banco.ID:
				debeEnBanco, haberEnBanco = l.Debe, l.Haber
			}
		}
		if a.Debe == "1000.00" { // el débito
			if debeEnGasto != "1000.00" || haberEnBanco != "1000.00" {
				t.Errorf("débito mal orientado: gasto debe %q, banco haber %q", debeEnGasto, haberEnBanco)
			}
		} else { // el crédito
			if debeEnBanco != "2500.00" || haberEnGasto != "2500.00" {
				t.Errorf("crédito mal orientado: banco debe %q, gasto haber %q", debeEnBanco, haberEnGasto)
			}
		}
	}
}

// NO DUPLICA, Y NO DEJA HUECOS EN LA NUMERACIÓN.
//
// Lo segundo lo encontró el E2E: el consecutivo se reserva en bloque por rendimiento, y reservar
// para movimientos que la base va a rechazar saltaba tantos números como el lote —6.340 en la
// prueba real—. Un libro diario con huecos es una observación de auditoría que nadie nota hasta
// que la hacen.
func TestRegenerarNoDuplicaNiDejaHuecosEnLaNumeracion(t *testing.T) {
	e := prepararEscenario(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		e.movimiento(t, "2026-08-0"+string(rune('1'+i)), "100.00", true, e.clasifID)
	}

	primera, err := e.svc.Generar(ctx, empUno, "2026-08", usrPrueba)
	if err != nil {
		t.Fatalf("primera generación: %v", err)
	}
	if primera.Creados != 5 || primera.DesdeNumero != 1 || primera.HastaNumero != 5 {
		t.Fatalf("primera: creados %d, números %d-%d", primera.Creados, primera.DesdeNumero, primera.HastaNumero)
	}

	segunda, err := e.svc.Generar(ctx, empUno, "2026-08", usrPrueba)
	if err != nil {
		t.Fatalf("segunda generación: %v", err)
	}
	if segunda.Creados != 0 || segunda.YaExistian != 5 {
		t.Errorf("segunda: creados %d, ya existían %d; se esperaba 0 y 5", segunda.Creados, segunda.YaExistian)
	}

	// Un movimiento nuevo tiene que tomar el número 6, no el 11: la corrida en vacío no puede
	// haber consumido numeración.
	e.movimiento(t, "2026-08-09", "700.00", true, e.clasifID)
	tercera, err := e.svc.Generar(ctx, empUno, "2026-08", usrPrueba)
	if err != nil {
		t.Fatalf("tercera generación: %v", err)
	}
	if tercera.DesdeNumero != 6 {
		t.Errorf("el asiento nuevo tomó el número %d; se esperaba 6 (la corrida en vacío dejó un hueco)",
			tercera.DesdeNumero)
	}
}

// EL RESUMEN DISTINGUE «SE PUEDE CONTABILIZAR» DE «FALTA GENERAR».
//
// En un período ya generado los dos números se separan, y la pantalla pone el primero en el
// botón: prometía «Generar 6.340» para después crear cero. Lo vio el navegador, no esta suite.
func TestElResumenSeparaLoContabilizableDeLoQueFaltaGenerar(t *testing.T) {
	e := prepararEscenario(t)
	ctx := context.Background()
	e.movimiento(t, "2026-08-05", "1000.00", true, e.clasifID)
	e.movimiento(t, "2026-08-06", "2000.00", true, e.clasifID)

	antes, err := e.svc.PrevisualizarGeneracion(ctx, empUno, "2026-08")
	if err != nil {
		t.Fatalf("previsualizar: %v", err)
	}
	if antes.Contabilizables != 2 || antes.Nuevos != 2 {
		t.Fatalf("antes: contabilizables %d, nuevos %d; se esperaba 2 y 2", antes.Contabilizables, antes.Nuevos)
	}

	if _, err := e.svc.Generar(ctx, empUno, "2026-08", usrPrueba); err != nil {
		t.Fatalf("generar: %v", err)
	}

	despues, err := e.svc.PrevisualizarGeneracion(ctx, empUno, "2026-08")
	if err != nil {
		t.Fatalf("previsualizar otra vez: %v", err)
	}
	// Siguen siendo contabilizables —no cambió nada en los datos— pero ya no hay nada nuevo.
	if despues.Contabilizables != 2 {
		t.Errorf("contabilizables %d, se esperaban 2", despues.Contabilizables)
	}
	if despues.Nuevos != 0 {
		t.Errorf("nuevos %d; después de generar no queda nada por generar", despues.Nuevos)
	}
	if !strings.Contains(despues.Aviso, "ya está generado") {
		t.Errorf("el aviso no dice que ya está generado: %q", despues.Aviso)
	}

	// Un movimiento más: vuelve a haber uno nuevo, y los contabilizables suben a tres.
	e.movimiento(t, "2026-08-07", "500.00", true, e.clasifID)
	conUnoMas, err := e.svc.PrevisualizarGeneracion(ctx, empUno, "2026-08")
	if err != nil {
		t.Fatalf("previsualizar con uno más: %v", err)
	}
	if conUnoMas.Contabilizables != 3 || conUnoMas.Nuevos != 1 {
		t.Errorf("con uno más: contabilizables %d, nuevos %d; se esperaba 3 y 1",
			conUnoMas.Contabilizables, conUnoMas.Nuevos)
	}
}

// TODO O NADA: una contabilidad a medias es peor que ninguna, porque nadie sabe dónde se cortó.
func TestLaGeneracionEsTodoONada(t *testing.T) {
	e := prepararEscenario(t)
	ctx := context.Background()
	e.movimiento(t, "2026-08-05", "1000.00", true, e.clasifID)
	e.movimiento(t, "2026-08-06", "2000.00", true, e.clasifID)

	// Se apaga la cuenta de gasto a una de agrupación: la base va a rechazar la línea.
	if _, err := e.repo.pool.Exec(ctx,
		`UPDATE cuenta_contable SET acepta_movimiento = false WHERE id = $1::uuid`, e.gasto.ID); err != nil {
		t.Fatalf("apagar la cuenta: %v", err)
	}

	if _, err := e.svc.Generar(ctx, empUno, "2026-08", usrPrueba); err == nil {
		t.Fatal("generó contra una cuenta que no admite asientos")
	}
	vivos, err := e.repo.Diario(ctx, empUno, FiltroDiario{})
	if err != nil {
		t.Fatalf("diario: %v", err)
	}
	if len(vivos) != 0 {
		t.Errorf("quedaron %d asientos de una generación que falló", len(vivos))
	}
}

// Lo que no se puede contabilizar se cuenta y se explica, en vez de desaparecer.
func TestPrevisualizarDiceQueFaltaYDonde(t *testing.T) {
	e := prepararEscenario(t)
	ctx := context.Background()

	e.movimiento(t, "2026-08-05", "1000.00", true, e.clasifID) // listo
	e.movimiento(t, "2026-08-06", "500.00", true, "")          // sin partida

	// Una partida sin mapear.
	otra := sembrarPartida(t, e.repo, empUno, "Otros", NatGasto, true, "Sin mapear", 0)
	e.movimiento(t, "2026-08-07", "300.00", true, otra)

	res, err := e.svc.PrevisualizarGeneracion(ctx, empUno, "2026-08")
	if err != nil {
		t.Fatalf("previsualizar: %v", err)
	}
	if res.Total != 3 || res.Contabilizables != 1 {
		t.Fatalf("total %d, contabilizables %d; se esperaba 3 y 1", res.Total, res.Contabilizables)
	}
	porMotivo := map[string]MotivoExcluido{}
	for _, m := range res.PorMotivo {
		porMotivo[m.Motivo] = m
	}
	if porMotivo[FaltaClasificacion].Movimientos != 1 {
		t.Errorf("sin clasificar: %+v", porMotivo[FaltaClasificacion])
	}
	if porMotivo[FaltaMapeo].Movimientos != 1 {
		t.Errorf("sin mapeo: %+v", porMotivo[FaltaMapeo])
	}
	// Cada motivo dice DÓNDE se arregla: sin eso, «1.843 sin mapeo» informa pero no se puede
	// accionar sin preguntarle a alguien.
	if !strings.Contains(porMotivo[FaltaMapeo].DondeSeArregla, "Mapeo") {
		t.Errorf("el motivo no dice dónde se arregla: %q", porMotivo[FaltaMapeo].DondeSeArregla)
	}
	if res.Aviso == "" {
		t.Error("no hay aviso")
	}
}

// UNA PARTIDA DECLARADA NO CONTABILIZABLE NO GENERA ASIENTO, y eso no es un error: es la decisión
// de la Fase 2 respetada.
func TestLaPartidaNoContabilizableNoGeneraAsiento(t *testing.T) {
	e := prepararEscenario(t)
	ctx := context.Background()

	otra := sembrarPartida(t, e.repo, empUno, "Control", NatNeutro, true, "Traslado interno", 0)
	if err := e.repo.GuardarMapeo(ctx, empUno, DecisionDeMapeo{
		ClasificacionID: otra, Estado: EstadoNoContabilizable,
		Motivo: "es un traslado entre cuentas propias",
	}, usrPrueba); err != nil {
		t.Fatalf("mapear como no contabilizable: %v", err)
	}
	e.movimiento(t, "2026-08-05", "1000.00", true, e.clasifID)
	e.movimiento(t, "2026-08-06", "9999.00", true, otra)

	res, err := e.svc.PrevisualizarGeneracion(ctx, empUno, "2026-08")
	if err != nil {
		t.Fatalf("previsualizar: %v", err)
	}
	if res.Contabilizables != 1 {
		t.Errorf("contabilizables %d, se esperaba 1", res.Contabilizables)
	}
	var hallado bool
	for _, m := range res.PorMotivo {
		if m.Motivo == NoContabilizable && m.Movimientos == 1 {
			hallado = true
		}
	}
	if !hallado {
		t.Errorf("la partida no contabilizable no se reportó: %+v", res.PorMotivo)
	}
}

// Revertir anula y permite regenerar; borrar dejaría un hueco en el libro.
func TestRevertirAnulaYPermiteRegenerar(t *testing.T) {
	e := prepararEscenario(t)
	ctx := context.Background()
	e.movimiento(t, "2026-08-05", "1000.00", true, e.clasifID)
	e.movimiento(t, "2026-08-06", "2000.00", true, e.clasifID)

	if _, err := e.svc.Generar(ctx, empUno, "2026-08", usrPrueba); err != nil {
		t.Fatalf("generar: %v", err)
	}

	// El motivo corto no alcanza: anular un período cambia los estados financieros del mes.
	if _, err := e.svc.Revertir(ctx, empUno, "2026-08", "ok", usrPrueba); !errors.Is(err, ErrMotivoReversaRequerido) {
		t.Errorf("aceptó revertir sin explicación: %v", err)
	}

	rev, err := e.svc.Revertir(ctx, empUno, "2026-08", "el mapeo estaba mal, se rehace", usrPrueba)
	if err != nil {
		t.Fatalf("revertir: %v", err)
	}
	if rev.Anulados != 2 {
		t.Errorf("anulados %d, se esperaban 2", rev.Anulados)
	}

	// Los asientos siguen en el libro, sin efecto.
	todos, err := e.repo.Diario(ctx, empUno, FiltroDiario{})
	if err != nil {
		t.Fatalf("diario: %v", err)
	}
	if len(todos) != 2 {
		t.Errorf("los asientos anulados desaparecieron del libro: %d", len(todos))
	}
	// Y no mueven saldo.
	m, err := e.repo.Mayor(ctx, empUno, FiltroMayor{CuentaID: e.gasto.ID})
	if err != nil {
		t.Fatalf("mayor: %v", err)
	}
	if m.SaldoFinal != "0.00" {
		t.Errorf("los anulados siguen moviendo saldo: %q", m.SaldoFinal)
	}

	// Y el período se puede volver a generar.
	otra, err := e.svc.Generar(ctx, empUno, "2026-08", usrPrueba)
	if err != nil {
		t.Fatalf("regenerar tras revertir: %v", err)
	}
	if otra.Creados != 2 {
		t.Errorf("tras revertir se crearon %d, se esperaban 2", otra.Creados)
	}
}

// UNA LISTA VACÍA VIAJA COMO `[]`, NO COMO `null`.
//
// Un slice nil de Go serializa a `null`, y la pantalla que hace `por_motivo.length` se cae en un
// período sin movimientos. Lo encontró el navegador, no esta suite: las pruebas miran la
// estructura de Go, donde nil y vacío se comportan igual. Ya había pasado con el previsualizador
// de Excel.
func TestElResumenNuncaDevuelveListasEnNulo(t *testing.T) {
	e := prepararEscenario(t)
	ctx := context.Background()

	// Un período sin un solo movimiento: el caso donde todo queda vacío.
	res, err := e.svc.PrevisualizarGeneracion(ctx, empUno, "2026-01")
	if err != nil {
		t.Fatalf("previsualizar: %v", err)
	}
	if res.PorMotivo == nil {
		t.Error("por_motivo es nil: en JSON sale como null y tumba la pantalla")
	}

	crudo, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("serializar: %v", err)
	}
	if strings.Contains(string(crudo), `"por_motivo":null`) {
		t.Errorf("el JSON lleva null en vez de []: %s", crudo)
	}
}

// Un período mal escrito no llega a la base: saldría como 500 y se registraría como incidente.
func TestPeriodoInvalido(t *testing.T) {
	e := prepararEscenario(t)
	ctx := context.Background()
	for _, p := range []string{"hola", "2026-13", "2026/08", "202608", ""} {
		if _, err := e.svc.PrevisualizarGeneracion(ctx, empUno, p); !errors.Is(err, ErrPeriodoInvalido) {
			t.Errorf("previsualizar(%q): %v", p, err)
		}
		if _, err := e.svc.Generar(ctx, empUno, p, usrPrueba); !errors.Is(err, ErrPeriodoInvalido) {
			t.Errorf("generar(%q): %v", p, err)
		}
	}
}

// El asiento apunta a SU movimiento: es lo que permite ir del estado financiero al documento.
func TestCadaAsientoApuntaASuMovimiento(t *testing.T) {
	e := prepararEscenario(t)
	ctx := context.Background()
	movID := e.movimiento(t, "2026-08-05", "1000.00", true, e.clasifID)

	if _, err := e.svc.Generar(ctx, empUno, "2026-08", usrPrueba); err != nil {
		t.Fatalf("generar: %v", err)
	}
	asientos, err := e.repo.Diario(ctx, empUno, FiltroDiario{})
	if err != nil {
		t.Fatalf("diario: %v", err)
	}
	if len(asientos) != 1 {
		t.Fatalf("asientos: %d", len(asientos))
	}
	a := asientos[0]
	if a.OrigenTipo != OrigenMovimiento {
		t.Errorf("origen %q, se esperaba %q", a.OrigenTipo, OrigenMovimiento)
	}
	if a.OrigenID == nil || *a.OrigenID != movID {
		t.Errorf("el asiento no apunta a su movimiento: %v", a.OrigenID)
	}
	// Y la glosa dice qué es, no un código del banco.
	if !strings.Contains(a.Glosa, "Papelería") {
		t.Errorf("la glosa no nombra la partida: %q", a.Glosa)
	}
}
