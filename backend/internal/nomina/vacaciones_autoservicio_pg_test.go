package nomina

// El autoservicio de vacaciones contra PostgreSQL de verdad (8-oct-2026).
//
// «Debe haber un permiso que le permita a la jefatura o bien al colaborador solicitar las
// vacaciones, programarlas, pero que no pueda ver nada más de los empleados o data confidencial.»
//
// Lo que hay que proteger acá es el SQL, y por eso estas pruebas no pueden correr detrás de un
// doble. Un fake diría que sí a todo: que el alcance cierra, que el recorte por departamento viaja
// hasta el UPDATE y que la lista de columnas no deja escapar un salario. Ninguna de las tres cosas
// la demuestra un fake, y las tres son la función entera.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

const (
	empVac    = "a1a1a1a1-a1a1-4a1a-a1a1-a1a1a1a1a1a1"
	otraEmpV  = "a2a2a2a2-a2a2-4a2a-a2a2-a2a2a2a2a2a2"
	deptoVent = "b1b1b1b1-b1b1-4b1b-b1b1-b1b1b1b1b1b1"
	deptoOper = "b2b2b2b2-b2b2-4b2b-b2b2-b2b2b2b2b2b2"
	usrAna    = "c1c1c1c1-c1c1-4c1c-c1c1-c1c1c1c1c1c1" // colaboradora de Ventas
	usrJefeV  = "c2c2c2c2-c2c2-4c2c-c2c2-c2c2c2c2c2c2" // jefatura de Ventas
	usrJefeO  = "c3c3c3c3-c3c3-4c3c-c3c3-c3c3c3c3c3c3" // jefatura de Operaciones
	usrSuelto = "c4c4c4c4-c4c4-4c4c-c4c4-c4c4c4c4c4c4" // entra al ERP y no está en planilla
	empAna    = "d1d1d1d1-d1d1-4d1d-d1d1-d1d1d1d1d1d1"
	empBeto   = "d2d2d2d2-d2d2-4d2d-d2d2-d2d2d2d2d2d2" // de Operaciones
)

var tablasVacaciones = []string{
	"empresa", "usuario", "departamento", "empleado", "vacacion", "rrhh_jefatura",
}

// baseDeVacaciones levanta un esquema temporal con las tablas que toca el autoservicio.
//
// El pool ve SOLO ese esquema: si faltara una tabla la consulta falla, en vez de leer la de
// producción sin que nadie se entere.
func baseDeVacaciones(t *testing.T) *pgRepository {
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
	esquema := fmt.Sprintf("prueba_vac_%d", time.Now().UnixNano())
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
	for _, tabla := range tablasVacaciones {
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

	repo := &pgRepository{pool: pool}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	exec(`INSERT INTO empresa (id, nombre) VALUES ($1::uuid,'Valle de Paz'), ($2::uuid,'Coopeprofa')`, empVac, otraEmpV)
	for _, u := range []struct{ id, nombre, email string }{
		{usrAna, "Ana", "ana@vdp.local"},
		{usrJefeV, "Jefe de Ventas", "jefe.ventas@vdp.local"},
		{usrJefeO, "Jefe de Operaciones", "jefe.oper@vdp.local"},
		{usrSuelto, "Contadora externa", "externa@vdp.local"},
	} {
		exec(`INSERT INTO usuario (id, nombre, email, password_hash) VALUES ($1::uuid,$2,$3,'x')`,
			u.id, u.nombre, u.email)
	}
	exec(`INSERT INTO departamento (id, empresa_id, nombre) VALUES ($1::uuid,$3::uuid,'Ventas'), ($2::uuid,$3::uuid,'Operaciones')`,
		deptoVent, deptoOper, empVac)

	// Ana lleva 24 meses: con 1 día por mes, 24 días acumulados.
	exec(`INSERT INTO empleado (id, empresa_id, nombre, identificacion, fecha_ingreso, salario_base,
	        departamento_id, usuario_id, activo)
	      VALUES ($1::uuid,$2::uuid,'Ana Jiménez','112340567', CURRENT_DATE - INTERVAL '24 months',
	              850000, $3::uuid, $4::uuid, true)`, empAna, empVac, deptoVent, usrAna)
	// Beto es de OTRO departamento: el control de que la jefatura de Ventas no lo alcanza.
	exec(`INSERT INTO empleado (id, empresa_id, nombre, identificacion, fecha_ingreso, salario_base,
	        departamento_id, activo)
	      VALUES ($1::uuid,$2::uuid,'Beto Rojas','203450678', CURRENT_DATE - INTERVAL '36 months',
	              1250000, $3::uuid, true)`, empBeto, empVac, deptoOper)

	exec(`INSERT INTO rrhh_jefatura (empresa_id, departamento_id, usuario_id, rol)
	      VALUES ($1::uuid,$2::uuid,$3::uuid,'TITULAR')`, empVac, deptoVent, usrJefeV)
	exec(`INSERT INTO rrhh_jefatura (empresa_id, departamento_id, usuario_id, rol)
	      VALUES ($1::uuid,$2::uuid,$3::uuid,'TITULAR')`, empVac, deptoOper, usrJefeO)
	return repo
}

var unDiaPorMes = decimal.RequireFromString("1")

// Sin ficha ligada no hay «mis vacaciones». Es el caso de quien entra al ERP y no está en planilla,
// y tiene que distinguirse de un fallo: la pantalla le dice qué pedirle a RRHH.
func TestSinFichaLigadaNoHayAlcance(t *testing.T) {
	repo := baseDeVacaciones(t)
	_, err := repo.EmpleadoDeUsuario(context.Background(), empVac, usrSuelto)
	if err != ErrSinEmpleadoLigado {
		t.Fatalf("un usuario sin ficha devolvió %v, se esperaba ErrSinEmpleadoLigado", err)
	}
	// Y el que sí la tiene, la encuentra.
	id, err := repo.EmpleadoDeUsuario(context.Background(), empVac, usrAna)
	if err != nil || id != empAna {
		t.Fatalf("Ana resolvió a %q / %v", id, err)
	}
}

// El alcance de la jefatura es su departamento, y una lista vacía CIERRA en vez de abrir.
func TestElAlcanceDeLaJefaturaEsSuDepartamento(t *testing.T) {
	repo := baseDeVacaciones(t)
	ctx := context.Background()

	deptos, err := repo.DepartamentosDeJefe(ctx, empVac, usrJefeV)
	if err != nil {
		t.Fatal(err)
	}
	if len(deptos) != 1 || deptos[0] != deptoVent {
		t.Fatalf("la jefatura de Ventas tiene a cargo %v", deptos)
	}
	// Quien no es jefatura de nada no alcanza a nadie.
	deptos, err = repo.DepartamentosDeJefe(ctx, empVac, usrAna)
	if err != nil {
		t.Fatal(err)
	}
	if len(deptos) != 0 {
		t.Fatalf("una colaboradora resultó jefatura de %v", deptos)
	}
	// Y con alcance vacío NO se ve la empresa entera, que es lo que haría un filtro mal escrito.
	saldos, err := repo.SaldosDeDepartamentos(ctx, empVac, nil, unDiaPorMes)
	if err != nil {
		t.Fatal(err)
	}
	if len(saldos) != 0 {
		t.Fatalf("un alcance vacío devolvió %d empleados; un alcance vacío CIERRA", len(saldos))
	}
	sols, err := repo.VacacionesDeDepartamentos(ctx, empVac, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(sols) != 0 {
		t.Fatalf("un alcance vacío devolvió %d solicitudes", len(sols))
	}
}

// Lo que ve la jefatura NO puede traer un salario. Es la razón de ser de toda la función: el
// permiso existe para no tener que dar `rrhh.ver`, que abre salarios, deducciones y finiquitos.
func TestLaJefaturaNoVeNingunDatoDePlanilla(t *testing.T) {
	repo := baseDeVacaciones(t)
	ctx := context.Background()
	if _, err := repo.SolicitarVacacion(ctx, empVac,
		VacacionInput{EmpleadoID: empAna, FechaInicio: "2026-11-02", Dias: "5"},
		unDiaPorMes, usrAna); err != nil {
		t.Fatalf("solicitar: %v", err)
	}
	sols, err := repo.VacacionesDeDepartamentos(ctx, empVac, []string{deptoVent}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(sols) != 1 {
		t.Fatalf("la jefatura vio %d solicitudes, se esperaba 1", len(sols))
	}
	s := sols[0]
	// El salario de Ana es 850000 y su cédula 112340567: ninguno de los dos puede aparecer en
	// NINGÚN campo de lo que viaja.
	for campo, valor := range map[string]string{
		"empleado": s.EmpleadoNombre, "departamento": s.Departamento,
		"observaciones": s.Observaciones, "estado": s.Estado, "dias": s.Dias,
		"solicitada_por": s.SolicitadaPor, "motivo_rechazo": s.MotivoRechazo,
	} {
		for _, prohibido := range []string{"850000", "112340567"} {
			if strings.Contains(valor, prohibido) {
				t.Errorf("el campo %q trae %q, que es dato de planilla", campo, prohibido)
			}
		}
	}
	// Y el saldo del equipo tampoco: trae días, no dinero.
	saldos, err := repo.SaldosDeDepartamentos(ctx, empVac, []string{deptoVent}, unDiaPorMes)
	if err != nil {
		t.Fatal(err)
	}
	if len(saldos) != 1 || saldos[0].Nombre != "Ana Jiménez" {
		t.Fatalf("saldos del equipo: %+v", saldos)
	}
	if saldos[0].Acumulado != "24.00" {
		t.Errorf("acumulado de Ana = %q, se esperaban 24.00 (24 meses × 1 día)", saldos[0].Acumulado)
	}
}

// Una jefatura NO puede aprobar las vacaciones de otro departamento, aunque mande el id correcto.
// El recorte viaja hasta el UPDATE justamente por esto: el id lo elige el cliente.
func TestUnaJefaturaNoApruebaFueraDeSuDepartamento(t *testing.T) {
	repo := baseDeVacaciones(t)
	ctx := context.Background()
	v, err := repo.SolicitarVacacion(ctx, empVac,
		VacacionInput{EmpleadoID: empAna, FechaInicio: "2026-11-02", Dias: "5"},
		unDiaPorMes, usrAna)
	if err != nil {
		t.Fatalf("solicitar: %v", err)
	}
	// La jefatura de OPERACIONES intenta aprobar una solicitud de VENTAS.
	err = repo.CambiarEstadoVacacion(ctx, empVac, v.ID, VacacionSolicitada, VacacionAprobada,
		usrJefeO, "", []string{deptoOper})
	if err != ErrTransicionVacacionInvalida {
		t.Fatalf("la jefatura ajena aprobó (err=%v): el recorte por departamento no llegó al UPDATE", err)
	}
	// La suya sí.
	if err := repo.CambiarEstadoVacacion(ctx, empVac, v.ID, VacacionSolicitada, VacacionAprobada,
		usrJefeV, "", []string{deptoVent}); err != nil {
		t.Fatalf("la jefatura propia no pudo aprobar: %v", err)
	}
}

// El saldo distingue lo GOZADO de lo RESERVADO, y las dos cuentas hacen falta.
func TestElSaldoDistingueGozadoDeReservado(t *testing.T) {
	repo := baseDeVacaciones(t)
	ctx := context.Background()

	// Un disfrute ya confirmado, como el que registra RRHH.
	if _, err := repo.CrearVacacion(ctx, empVac,
		VacacionInput{EmpleadoID: empAna, FechaInicio: "2026-03-02", Dias: "4"},
		unDiaPorMes, usrAna); err != nil {
		t.Fatalf("crear confirmada: %v", err)
	}
	// Y una solicitud sin aprobar.
	if _, err := repo.SolicitarVacacion(ctx, empVac,
		VacacionInput{EmpleadoID: empAna, FechaInicio: "2026-12-01", Dias: "6"},
		unDiaPorMes, usrAna); err != nil {
		t.Fatalf("solicitar: %v", err)
	}
	s, err := repo.SaldoDeEmpleado(ctx, empVac, empAna, unDiaPorMes)
	if err != nil {
		t.Fatal(err)
	}
	if s.Disfrutado != "4.00" {
		t.Errorf("disfrutado = %q, se esperaba 4.00: una solicitud sin aprobar NO es un día tomado",
			s.Disfrutado)
	}
	if s.Reservado != "6.00" {
		t.Errorf("reservado = %q, se esperaba 6.00", s.Reservado)
	}
	// 24 acumulados − 4 gozados − 6 reservados = 14 que puede pedir hoy.
	if s.Pendiente != "14.00" {
		t.Errorf("pendiente = %q, se esperaba 14.00 (24 − 4 − 6)", s.Pendiente)
	}
}

// No se pueden pedir dos veces los mismos días: la solicitud pendiente ya aparta el saldo.
func TestUnaSolicitudPendienteApartaElSaldo(t *testing.T) {
	repo := baseDeVacaciones(t)
	ctx := context.Background()
	// Ana tiene 24 días. Pide 20.
	if _, err := repo.SolicitarVacacion(ctx, empVac,
		VacacionInput{EmpleadoID: empAna, FechaInicio: "2026-11-02", Dias: "20"},
		unDiaPorMes, usrAna); err != nil {
		t.Fatalf("primera solicitud: %v", err)
	}
	// Y vuelve a pedir 20: solo le quedan 4.
	_, err := repo.SolicitarVacacion(ctx, empVac,
		VacacionInput{EmpleadoID: empAna, FechaInicio: "2026-12-01", Dias: "20"},
		unDiaPorMes, usrAna)
	if err != ErrSinSaldoVacaciones {
		t.Fatalf("la segunda solicitud de 20 días pasó (err=%v); con solo lo confirmado "+
			"descontándose, cualquiera pide sus vacaciones dos veces", err)
	}
}

// Una solicitud RECHAZADA devuelve los días: no reserva nada.
func TestLoRechazadoNoReservaSaldo(t *testing.T) {
	repo := baseDeVacaciones(t)
	ctx := context.Background()
	v, err := repo.SolicitarVacacion(ctx, empVac,
		VacacionInput{EmpleadoID: empAna, FechaInicio: "2026-11-02", Dias: "20"},
		unDiaPorMes, usrAna)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CambiarEstadoVacacion(ctx, empVac, v.ID, VacacionSolicitada, VacacionRechazada,
		usrJefeV, "esa semana cierra el mes", []string{deptoVent}); err != nil {
		t.Fatalf("rechazar: %v", err)
	}
	s, err := repo.SaldoDeEmpleado(ctx, empVac, empAna, unDiaPorMes)
	if err != nil {
		t.Fatal(err)
	}
	if s.Pendiente != "24.00" {
		t.Errorf("pendiente = %q tras un rechazo, se esperaban 24.00: los días rechazados "+
			"quedarían apartados para siempre", s.Pendiente)
	}
}

// El camino entero: pedir → aprobar → confirmar, y recién ahí cuenta como disfrutado.
func TestElCaminoCompletoHastaElDisfrute(t *testing.T) {
	repo := baseDeVacaciones(t)
	ctx := context.Background()
	v, err := repo.SolicitarVacacion(ctx, empVac,
		VacacionInput{EmpleadoID: empAna, FechaInicio: "2026-11-02", Dias: "5"},
		unDiaPorMes, usrAna)
	if err != nil {
		t.Fatal(err)
	}
	if v.Estado != VacacionSolicitada {
		t.Fatalf("nació %q, se esperaba SOLICITADA", v.Estado)
	}
	if err := repo.CambiarEstadoVacacion(ctx, empVac, v.ID, VacacionSolicitada, VacacionAprobada,
		usrJefeV, "", []string{deptoVent}); err != nil {
		t.Fatalf("aprobar: %v", err)
	}
	s, _ := repo.SaldoDeEmpleado(ctx, empVac, empAna, unDiaPorMes)
	if s.Disfrutado != "0.00" {
		t.Errorf("aprobada y ya figura como disfrutada (%q): aprobar no es haberse tomado los días",
			s.Disfrutado)
	}
	// RRHH confirma: sin recorte por departamento (nil).
	if err := repo.CambiarEstadoVacacion(ctx, empVac, v.ID, VacacionAprobada, VacacionConfirmada,
		usrJefeV, "", nil); err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	s, _ = repo.SaldoDeEmpleado(ctx, empVac, empAna, unDiaPorMes)
	if s.Disfrutado != "5.00" {
		t.Errorf("confirmada y el disfrutado es %q, se esperaba 5.00", s.Disfrutado)
	}
	// Y no se puede volver a confirmar lo ya confirmado.
	if err := repo.CambiarEstadoVacacion(ctx, empVac, v.ID, VacacionAprobada, VacacionConfirmada,
		usrJefeV, "", nil); err != ErrTransicionVacacionInvalida {
		t.Errorf("se confirmó dos veces (err=%v)", err)
	}
}

// El aislamiento por empresa, que en este módulo significa que la jefatura de una empresa no
// alcanza a los empleados de otra.
func TestLaJefaturaNoCruzaEmpresas(t *testing.T) {
	repo := baseDeVacaciones(t)
	ctx := context.Background()
	deptos, err := repo.DepartamentosDeJefe(ctx, otraEmpV, usrJefeV)
	if err != nil {
		t.Fatal(err)
	}
	if len(deptos) != 0 {
		t.Fatalf("la jefatura de Valle de Paz tiene %v a cargo en Coopeprofa", deptos)
	}
	// Y preguntando por la empresa ajena con el departamento propio, tampoco.
	saldos, err := repo.SaldosDeDepartamentos(ctx, otraEmpV, []string{deptoVent}, unDiaPorMes)
	if err != nil {
		t.Fatal(err)
	}
	if len(saldos) != 0 {
		t.Fatalf("se vieron %d empleados de otra empresa", len(saldos))
	}
}
