package conta

// El mapeo contable contra Postgres (mig 0110).
//
// LO QUE SE PRUEBA ACÁ es lo que un doble no puede: las dos invariantes las hace cumplir la BASE
// —un CHECK y un disparador—, no el código de Go. Las llamadas de abajo entran por `GuardarMapeo`
// a propósito, SALTÁNDOSE `Validar()`: si la prueba pasara por el servicio, demostraría que mi
// función tiene un `if`, no que la fila mala es imposible.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"
)

var tablasDelMapeo = []string{
	"empresa", "usuario", "cuenta_contable",
	"concepto", "clasificacion", "movimiento_bancario", "mapeo_contable",
}

const disparadorDelMapeo = `
	CREATE TRIGGER trg_mapeo_cuenta_de_movimiento
	BEFORE INSERT OR UPDATE ON %s.mapeo_contable
	FOR EACH ROW EXECUTE FUNCTION public.fn_mapeo_cuenta_de_movimiento()`

func baseMapeo(t *testing.T) *pgRepository {
	t.Helper()
	return baseConta(t, tablasDelMapeo, []string{disparadorDelArbol, disparadorDelMapeo})
}

// sembrarPartida crea un concepto con su naturaleza y una clasificación colgando, y devuelve el id
// de la clasificación. `movimientos` dice cuántos movimientos vivos la usan: es lo que decide si
// entra en el denominador de la cobertura.
func sembrarPartida(t *testing.T, r *pgRepository, empresa, concepto, naturaleza string, declarada bool, clasificacion string, movimientos int) string {
	t.Helper()
	ctx := context.Background()
	var conceptoID string
	if err := r.pool.QueryRow(ctx,
		`INSERT INTO concepto (empresa_id, nombre, naturaleza, naturaleza_declarada)
		 VALUES ($1::uuid,$2,$3,$4) RETURNING id::text`,
		empresa, concepto, naturaleza, declarada).Scan(&conceptoID); err != nil {
		t.Fatalf("sembrar concepto: %v", err)
	}
	var clasifID string
	if err := r.pool.QueryRow(ctx,
		`INSERT INTO clasificacion (empresa_id, concepto_id, nombre)
		 VALUES ($1::uuid,$2::uuid,$3) RETURNING id::text`,
		empresa, conceptoID, clasificacion).Scan(&clasifID); err != nil {
		t.Fatalf("sembrar clasificación: %v", err)
	}
	for i := 0; i < movimientos; i++ {
		if _, err := r.pool.Exec(ctx,
			`INSERT INTO movimiento_bancario (empresa_id, cuenta_bancaria_id, fecha, natural_key, clasificacion_id)
			 VALUES ($1::uuid, gen_random_uuid(), '2026-10-01', $2, $3::uuid)`,
			empresa, fmt.Sprintf("%s-%d", clasifID, i), clasifID); err != nil {
			t.Fatalf("sembrar movimiento: %v", err)
		}
	}
	return clasifID
}

// INVARIANTE 1 · O CUENTA, O MOTIVO. Nunca las dos, nunca ninguna.
//
// Es lo que convierte el mapeo en una lista que se puede TERMINAR: cada clasificación o está
// resuelta con su cuenta, o está resuelta con su explicación. Sin esto, «motivo vacío» y «falta
// decidir» serían la misma fila y la cobertura mentiría.
func TestLaBaseExigeCuentaOMotivo(t *testing.T) {
	repo := baseMapeo(t)
	ctx := context.Background()

	cta, err := repo.CrearCuenta(ctx, cuenta("6-01-00-000-000", "Gastos generales", idDe(empUno), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear cuenta: %v", err)
	}
	clasif := sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Papelería", 0)

	t.Run("mapeada sin cuenta", func(t *testing.T) {
		err := repo.GuardarMapeo(ctx, empUno,
			DecisionDeMapeo{ClasificacionID: clasif, Estado: EstadoMapeada}, usrPrueba)
		if err == nil {
			t.Fatal("la base ACEPTÓ una partida mapeada sin cuenta")
		}
	})
	t.Run("no contabilizable sin motivo", func(t *testing.T) {
		err := repo.GuardarMapeo(ctx, empUno,
			DecisionDeMapeo{ClasificacionID: clasif, Estado: EstadoNoContabilizable}, usrPrueba)
		if err == nil {
			t.Fatal("la base ACEPTÓ declarar que algo no se contabiliza sin decir por qué")
		}
	})
	t.Run("no contabilizable con cuenta", func(t *testing.T) {
		err := repo.GuardarMapeo(ctx, empUno, DecisionDeMapeo{
			ClasificacionID: clasif, Estado: EstadoNoContabilizable,
			Motivo: "es un traslado entre cuentas propias", CuentaID: cta.ID,
		}, usrPrueba)
		if err == nil {
			t.Fatal("la base ACEPTÓ una decisión contradictoria: no se contabiliza, pero con cuenta")
		}
	})
	t.Run("mapeada con cuenta sí entra", func(t *testing.T) {
		if err := repo.GuardarMapeo(ctx, empUno,
			DecisionDeMapeo{ClasificacionID: clasif, Estado: EstadoMapeada, CuentaID: cta.ID}, usrPrueba); err != nil {
			t.Fatalf("la decisión buena no entró: %v", err)
		}
	})
}

// INVARIANTE 2 · SOLO CUENTAS DE MOVIMIENTO, y de esta empresa.
//
// Mapear contra una cuenta que agrupa a otras produciría asientos que rompen el árbol: el mismo
// colón sumaría dos veces y el error aparecería recién al cuadrar el balance, meses después.
func TestLaBaseImpideMapearAUnaCuentaQueNoAdmiteAsientos(t *testing.T) {
	repo := baseMapeo(t)
	ctx := context.Background()

	madre, err := repo.CrearCuenta(ctx, cuenta("6-00-00-000-000", "GASTOS", idDe(empUno), false, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear madre: %v", err)
	}
	if _, err := repo.CrearCuenta(ctx, cuenta("6-01-00-000-000", "Gastos generales", idDe(empUno), true, madre.Codigo), usrPrueba); err != nil {
		t.Fatalf("crear hija: %v", err)
	}
	ajena, err := repo.CrearCuenta(ctx, cuenta("6-00-00-000-000", "GASTOS (de la otra)", idDe(empDos), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear la ajena: %v", err)
	}
	clasif := sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Papelería", 0)

	t.Run("cuenta de agrupación", func(t *testing.T) {
		err := repo.GuardarMapeo(ctx, empUno,
			DecisionDeMapeo{ClasificacionID: clasif, Estado: EstadoMapeada, CuentaID: madre.ID}, usrPrueba)
		if !errors.Is(err, ErrCuentaDeAgrupacion) {
			t.Fatalf("la base aceptó mapear a una cuenta de agrupación, o el error no se tradujo: %v", err)
		}
		// Y además dice CUÁL cuenta. En un lote de veinte, un mensaje genérico obliga a probar
		// una por una para encontrar la que falló.
		if !strings.Contains(err.Error(), madre.Codigo) {
			t.Errorf("el error no nombra la cuenta que falló: %v", err)
		}
	})
	t.Run("cuenta de otra empresa", func(t *testing.T) {
		err := repo.GuardarMapeo(ctx, empUno,
			DecisionDeMapeo{ClasificacionID: clasif, Estado: EstadoMapeada, CuentaID: ajena.ID}, usrPrueba)
		if err == nil {
			t.Fatal("se mapeó contra la cuenta de OTRA empresa: eso cruza la contabilidad de dos compañías")
		}
		if !strings.Contains(err.Error(), "otra empresa") {
			t.Errorf("el error no dice qué pasó: %v", err)
		}
	})
	t.Run("una cuenta del grupo sí sirve", func(t *testing.T) {
		// Las cuentas del grupo (empresa nula) existen para esto: las tres empresas las comparten.
		grupo, err := repo.CrearCuenta(ctx, cuenta("6-02-00-000-000", "Servicios profesionales", nil, true, ""), usrPrueba)
		if err != nil {
			t.Fatalf("crear la del grupo: %v", err)
		}
		if err := repo.GuardarMapeo(ctx, empUno,
			DecisionDeMapeo{ClasificacionID: clasif, Estado: EstadoMapeada, CuentaID: grupo.ID}, usrPrueba); err != nil {
			t.Fatalf("no se pudo mapear a una cuenta del GRUPO: %v", err)
		}
	})
}

// El caso real, de punta a punta: la lista marca el cruce y dice por qué.
func TestLaListaMarcaElCruceDeLaCajaChica(t *testing.T) {
	repo := baseMapeo(t)
	ctx := context.Background()

	caja, err := repo.CrearCuenta(ctx, cuenta("1-01-01-001-000", "Caja Chica Administración", idDe(empUno), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear cuenta: %v", err)
	}
	clasif := sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Caja Chica - Administración", 3)
	if err := repo.GuardarMapeo(ctx, empUno,
		DecisionDeMapeo{ClasificacionID: clasif, Estado: EstadoMapeada, CuentaID: caja.ID}, usrPrueba); err != nil {
		t.Fatalf("mapear: %v", err)
	}

	filas, err := repo.FilasDelMapeo(ctx, empUno, false, false)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("vinieron %d filas, se esperaba 1", len(filas))
	}
	f := filas[0]
	if !f.Cruzada {
		t.Error("la fila no quedó marcada como cruzada: el error se vería recién cuando el balance no cuadre")
	}
	if f.Movimientos != 3 {
		t.Errorf("movimientos=%d, se sembraron 3", f.Movimientos)
	}
	if f.CuentaCodigo != "1-01-01-001-000" || f.CuentaClase != ClaseActivo {
		t.Errorf("la fila no trae la cuenta: %q / %q", f.CuentaCodigo, f.CuentaClase)
	}

	cob, err := repo.Cobertura(ctx, empUno)
	if err != nil {
		t.Fatalf("cobertura: %v", err)
	}
	if cob.Cruzadas != 1 {
		t.Errorf("la cobertura no cuenta el cruce: %d", cob.Cruzadas)
	}
}

// LA COBERTURA MIDE LO QUE BLOQUEA. Una clasificación que nadie usó no impide contabilizar, y
// meterla en el denominador haría que el porcentaje nunca llegue al 100 %.
func TestLaCoberturaCuentaSoloLasQueTienenMovimientos(t *testing.T) {
	repo := baseMapeo(t)
	ctx := context.Background()

	cta, err := repo.CrearCuenta(ctx, cuenta("6-01-00-000-000", "Gastos generales", idDe(empUno), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear cuenta: %v", err)
	}
	usada := sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Papelería", 5)
	sembrarPartida(t, repo, empUno, "Gastos varios", NatGasto, true, "Sin decidir", 2)
	// «Sin declarar» NO es naturaleza vacía: la columna solo admite INGRESO/GASTO/NEUTRO, y el
	// default es NEUTRO con la bandera en falso. Así están 58 de los 64 conceptos de la base real
	// (medido el 26-oct-2026), y por eso el detector de cruce calla cuando no está declarada.
	sembrarPartida(t, repo, empUno, "Inventada", NatNeutro, false, "Nadie la usó", 0)

	if err := repo.GuardarMapeo(ctx, empUno,
		DecisionDeMapeo{ClasificacionID: usada, Estado: EstadoMapeada, CuentaID: cta.ID}, usrPrueba); err != nil {
		t.Fatalf("mapear: %v", err)
	}

	cob, err := repo.Cobertura(ctx, empUno)
	if err != nil {
		t.Fatalf("cobertura: %v", err)
	}
	if cob.Total != 3 {
		t.Errorf("total=%d, se sembraron 3 clasificaciones", cob.Total)
	}
	if cob.ConMovimientos != 2 {
		t.Errorf("con movimientos=%d, se esperaba 2: la tercera no la usó nadie", cob.ConMovimientos)
	}
	if cob.Mapeadas != 1 || cob.SinDecidir != 1 {
		t.Errorf("mapeadas=%d, sin decidir=%d; se esperaba 1 y 1", cob.Mapeadas, cob.SinDecidir)
	}
	if cob.SinNaturaleza != 1 {
		t.Errorf("sin naturaleza=%d, se esperaba 1", cob.SinNaturaleza)
	}
	if got := cob.PorcentajeResuelto(); got != 50 {
		t.Errorf("porcentaje=%d, se esperaba 50", got)
	}

	// `pendientes=1` es lo que hace usable una lista de 170: deja fuera lo ya resuelto.
	pendientes, err := repo.FilasDelMapeo(ctx, empUno, true, true)
	if err != nil {
		t.Fatalf("listar pendientes: %v", err)
	}
	if len(pendientes) != 1 || pendientes[0].Clasificacion != "Sin decidir" {
		t.Errorf("el filtro de pendientes con movimientos devolvió %d filas: %+v", len(pendientes), pendientes)
	}
}

// El mapeo de otra empresa no se ve ni se deshace.
func TestElMapeoNoCruzaEmpresas(t *testing.T) {
	repo := baseMapeo(t)
	ctx := context.Background()

	ctaDos, err := repo.CrearCuenta(ctx, cuenta("6-01-00-000-000", "Gastos generales", idDe(empDos), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear cuenta: %v", err)
	}
	ajena := sembrarPartida(t, repo, empDos, "Gastos", NatGasto, true, "Papelería de la otra", 1)
	if err := repo.GuardarMapeo(ctx, empDos,
		DecisionDeMapeo{ClasificacionID: ajena, Estado: EstadoMapeada, CuentaID: ctaDos.ID}, usrPrueba); err != nil {
		t.Fatalf("mapear en la otra empresa: %v", err)
	}

	filas, err := repo.FilasDelMapeo(ctx, empUno, false, false)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(filas) != 0 {
		t.Errorf("se vieron %d clasificaciones de OTRA empresa", len(filas))
	}
	if err := repo.BorrarMapeo(ctx, empUno, ajena); !errors.Is(err, ErrMapeoNoEncontrado) {
		t.Errorf("se pudo deshacer el mapeo de otra empresa: %v", err)
	}
	// Y sigue puesto donde corresponde.
	suyas, err := repo.FilasDelMapeo(ctx, empDos, false, false)
	if err != nil {
		t.Fatalf("listar la otra: %v", err)
	}
	if len(suyas) != 1 || suyas[0].Estado != EstadoMapeada {
		t.Errorf("el mapeo de la otra empresa se perdió: %+v", suyas)
	}
}

// UNA DECISIÓN MALA NO TUMBA A LAS BUENAS. El lote es lo que hace que un mapeo de 332 partidas se
// termine; obligar a rehacer veinte decisiones correctas por una mala lo volvería inservible.
func TestDecidirVariasGuardaLasBuenasYDiceCualFallo(t *testing.T) {
	repo := baseMapeo(t)
	ctx := context.Background()
	// Sin auditoría: `auditoria_evento` no está en el esquema temporal y `Registrar` tolera el
	// nil a propósito. Lo que se prueba acá es qué decisiones entran, no que se auditen.
	svc := NewService(repo, nil, zap.NewNop())

	cta, err := repo.CrearCuenta(ctx, cuenta("6-01-00-000-000", "Gastos generales", idDe(empUno), true, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear cuenta: %v", err)
	}
	madre, err := repo.CrearCuenta(ctx, cuenta("1-00-00-000-000", "ACTIVOS", idDe(empUno), false, ""), usrPrueba)
	if err != nil {
		t.Fatalf("crear madre: %v", err)
	}
	if _, err := repo.CrearCuenta(ctx, cuenta("1-01-00-000-000", "Caja", idDe(empUno), true, madre.Codigo), usrPrueba); err != nil {
		t.Fatalf("crear hija: %v", err)
	}

	buena := sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Papelería", 1)
	otraBuena := sembrarPartida(t, repo, empUno, "Gastos varios", NatGasto, true, "Combustible", 1)
	sinMotivo := sembrarPartida(t, repo, empUno, "Control", NatNeutro, true, "Partida de control", 1)
	aAgrupacion := sembrarPartida(t, repo, empUno, "Otros", NatGasto, true, "Mal mapeada", 1)

	res, err := svc.DecidirVarias(ctx, empUno, []DecisionDeMapeo{
		{ClasificacionID: buena, Estado: EstadoMapeada, CuentaID: cta.ID},
		{ClasificacionID: sinMotivo, Estado: EstadoNoContabilizable, Motivo: "ok"},
		{ClasificacionID: otraBuena, Estado: EstadoMapeada, CuentaID: cta.ID},
		{ClasificacionID: aAgrupacion, Estado: EstadoMapeada, CuentaID: madre.ID},
	}, usrPrueba)
	if err != nil {
		t.Fatalf("decidir varias: %v", err)
	}
	if res.Guardadas != 2 {
		t.Errorf("guardadas=%d, se esperaba 2", res.Guardadas)
	}
	if len(res.NoSePudo) != 2 {
		t.Fatalf("no se pudo=%v, se esperaban 2", res.NoSePudo)
	}
	// El motivo POR CLASIFICACIÓN: un conteo de fallos sin el porqué deja probando a ciegas.
	if !strings.Contains(res.NoSePudo[sinMotivo], "por qué") {
		t.Errorf("el motivo del rechazo no explica qué falta: %q", res.NoSePudo[sinMotivo])
	}
	if !strings.Contains(res.NoSePudo[aAgrupacion], "agrupa") {
		t.Errorf("el motivo del rechazo no explica qué falta: %q", res.NoSePudo[aAgrupacion])
	}

	// Deshacer devuelve la partida a «falta decidir», y repetirlo avisa en vez de callar.
	if err := svc.Deshacer(ctx, empUno, buena, usrPrueba); err != nil {
		t.Fatalf("deshacer: %v", err)
	}
	if err := svc.Deshacer(ctx, empUno, buena, usrPrueba); !errors.Is(err, ErrMapeoNoEncontrado) {
		t.Errorf("deshacer dos veces no avisó: %v", err)
	}
	cob, err := svc.Cobertura(ctx, empUno)
	if err != nil {
		t.Fatalf("cobertura: %v", err)
	}
	if cob.Mapeadas != 1 || cob.SinDecidir != 3 {
		t.Errorf("mapeadas=%d, sin decidir=%d; se esperaba 1 y 3", cob.Mapeadas, cob.SinDecidir)
	}
}
