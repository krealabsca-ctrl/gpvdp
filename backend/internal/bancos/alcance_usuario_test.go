package bancos

// El alcance por persona, desde la ficha del usuario (16-nov-2026).
//
// LO QUE NO PUEDE FALLAR: `departamento_validador` NO tiene `empresa_id` —cuelga del
// departamento—, así que lo único que impide darle un área de una empresa a alguien de OTRA es
// que el servicio compruebe la pertenencia antes de escribir. Y esas áreas no son solo de lectura:
// son la misma tabla con la que CxP decide quién valida las facturas del departamento. Un fallo
// acá no muestra datos de más: reparte poder de aprobación de pagos.

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

// AISLAMIENTO: a alguien de otra empresa no se le asigna nada, y NO se escribe.
func TestNoSeLeDaAlcanceAUnUsuarioDeOtraEmpresa(t *testing.T) {
	// El doble dice que la persona NO es de esta empresa.
	repo := &fakeRepo{usuarioDeLaEmpresa: false}
	svc := NewService(repo, nil, zap.NewNop(), false)

	err := svc.GuardarAreasDeUnUsuario(context.Background(), "empresa-a", "usuario-de-empresa-b",
		[]AreaAsignada{{DepartamentoID: "d-mercadeo", Rol: AreaTitular}})

	if err != ErrUsuarioFueraDeLaEmpresa {
		t.Fatalf("err = %v; se esperaba ErrUsuarioFueraDeLaEmpresa", err)
	}
	// Y lo que importa de verdad: no llegó una escritura a la base.
	if repo.areasGuardadas != nil {
		t.Errorf("se escribió igual: %+v. Esa tabla decide quién valida facturas en CxP",
			repo.areasGuardadas)
	}
}

// Lo mismo para las partidas de ingreso.
func TestNoSeLeDanPartidasAUnUsuarioDeOtraEmpresa(t *testing.T) {
	repo := &fakeRepo{usuarioDeLaEmpresa: false}
	svc := NewService(repo, nil, zap.NewNop(), false)

	err := svc.GuardarPartidasDeIngresoDeUnUsuario(context.Background(), "empresa-a", "ajeno",
		[]string{"c-datafonos"})

	if err != ErrUsuarioFueraDeLaEmpresa {
		t.Fatalf("err = %v", err)
	}
	if repo.partidasGuardadas != nil {
		t.Errorf("se escribió igual: %+v", repo.partidasGuardadas)
	}
}

// Y la ficha tampoco se puede LEER de alguien de otra empresa: saber qué áreas maneja una persona
// en otra empresa ya es información de esa otra empresa.
func TestNoSeLeeElAlcanceDeUnUsuarioDeOtraEmpresa(t *testing.T) {
	svc := NewService(&fakeRepo{usuarioDeLaEmpresa: false}, nil, zap.NewNop(), false)

	_, err := svc.AlcanceDeUnUsuario(context.Background(), "empresa-a", "ajeno")
	if err != ErrUsuarioFueraDeLaEmpresa {
		t.Fatalf("err = %v", err)
	}
}

// LA COMPROBACIÓN SE HACE CON LA EMPRESA DEL TOKEN, no con otra cosa.
//
// Sin esta prueba, comprobar contra una empresa y escribir en otra pasaría las de arriba.
func TestLaPertenenciaSeComprueabaConLaEmpresaDelToken(t *testing.T) {
	repo := &fakeRepo{usuarioDeLaEmpresa: true}
	svc := NewService(repo, nil, zap.NewNop(), false)

	if err := svc.GuardarAreasDeUnUsuario(context.Background(), "empresa-a", "u1", nil); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(repo.pidioUsuarioDeEmpresa) == 0 || repo.pidioUsuarioDeEmpresa[0] != "empresa-a/u1" {
		t.Errorf("se comprobó %v; se esperaba empresa-a/u1", repo.pidioUsuarioDeEmpresa)
	}
}

// UNA LISTA VACÍA QUITA TODO, y es una operación legítima: alguien que cambia de puesto.
func TestGuardarSinAreasQuitaLasQueTenia(t *testing.T) {
	repo := &fakeRepo{usuarioDeLaEmpresa: true}
	svc := NewService(repo, nil, zap.NewNop(), false)

	if err := svc.GuardarAreasDeUnUsuario(context.Background(), "e1", "u1", []AreaAsignada{}); err != nil {
		t.Fatalf("err = %v; vaciar el alcance no es un error", err)
	}
	if len(repo.areasGuardadas) != 0 {
		t.Errorf("quedaron áreas: %+v", repo.areasGuardadas)
	}
}

// SOLO PARTIDAS DE INGRESO.
//
// Una de gasto se aceptaría sin ruido y nunca produciría una línea —el control de ingreso solo
// mira los conceptos de naturaleza INGRESO—, así que la persona vería su partida nombrada arriba
// y la tabla vacía debajo, sin nada que explique por qué.
func TestNoSeAsignaUnaPartidaDeGastoAlAlcanceDeIngreso(t *testing.T) {
	repo := &fakeRepo{
		usuarioDeLaEmpresa: true,
		naturalezas:        map[string]string{"c-alquiler": "GASTO"},
	}
	svc := NewService(repo, nil, zap.NewNop(), false)

	err := svc.GuardarPartidasDeIngresoDeUnUsuario(context.Background(), "e1", "u1", []string{"c-alquiler"})
	if err != ErrPartidaNoEsDeIngreso {
		t.Fatalf("err = %v; se esperaba ErrPartidaNoEsDeIngreso", err)
	}
	if repo.partidasGuardadas != nil {
		t.Errorf("se guardó igual: %+v", repo.partidasGuardadas)
	}
}

// Una partida NEUTRA tampoco: es la naturaleza de 58 de los 64 conceptos, así que es el caso que
// más se va a intentar por error.
func TestNoSeAsignaUnaPartidaNeutraAlAlcanceDeIngreso(t *testing.T) {
	repo := &fakeRepo{usuarioDeLaEmpresa: true, naturalezas: map[string]string{"c-traslado": "NEUTRO"}}
	svc := NewService(repo, nil, zap.NewNop(), false)

	if err := svc.GuardarPartidasDeIngresoDeUnUsuario(context.Background(), "e1", "u1",
		[]string{"c-traslado"}); err != ErrPartidaNoEsDeIngreso {
		t.Fatalf("err = %v", err)
	}
}

// UNA PARTIDA QUE NO ES DE LA EMPRESA no aparece en el mapa de naturalezas —la consulta filtra por
// empresa— y eso tiene que ser un rechazo, no un «no sé, lo guardo igual».
func TestUnaPartidaDeOtraEmpresaSeRechaza(t *testing.T) {
	repo := &fakeRepo{usuarioDeLaEmpresa: true, naturalezas: map[string]string{}}
	svc := NewService(repo, nil, zap.NewNop(), false)

	if err := svc.GuardarPartidasDeIngresoDeUnUsuario(context.Background(), "e1", "u1",
		[]string{"c-de-otra-empresa"}); err != ErrPartidaDesconocida {
		t.Fatalf("err = %v; una partida que la empresa no tiene se rechaza", err)
	}
	if repo.partidasGuardadas != nil {
		t.Errorf("se guardó igual: %+v", repo.partidasGuardadas)
	}
}

// Una partida de ingreso SÍ pasa.
func TestSeAsignaUnaPartidaDeIngreso(t *testing.T) {
	repo := &fakeRepo{usuarioDeLaEmpresa: true, naturalezas: map[string]string{"c-datafonos": "INGRESO"}}
	svc := NewService(repo, nil, zap.NewNop(), false)

	if err := svc.GuardarPartidasDeIngresoDeUnUsuario(context.Background(), "e1", "u1",
		[]string{"c-datafonos"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(repo.partidasGuardadas) != 1 || repo.partidasGuardadas[0] != "c-datafonos" {
		t.Errorf("se guardó %+v", repo.partidasGuardadas)
	}
}

// ── La normalización ─────────────────────────────────────────────────────────────────────────

// SIN ROL ES TITULAR: es el caso normal y obligar a elegir en cada fila sería ruido.
func TestElAreaSinRolQuedaComoTitular(t *testing.T) {
	limpias, err := NormalizarAreas([]AreaAsignada{{DepartamentoID: "d1"}})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if limpias[0].Rol != AreaTitular {
		t.Errorf("rol = %q; se esperaba TITULAR", limpias[0].Rol)
	}
}

// UN ROL INVENTADO SE RECHAZA ACÁ y no en la base, donde el error no se entiende.
func TestUnRolDeAreaInventadoSeRechaza(t *testing.T) {
	if _, err := NormalizarAreas([]AreaAsignada{{DepartamentoID: "d1", Rol: "JEFE"}}); err != ErrRolDeAreaInvalido {
		t.Fatalf("err = %v", err)
	}
}

// LA MISMA ÁREA DOS VECES rompería el INSERT por clave duplicada: se deja una.
func TestElAreaRepetidaNoSeDuplica(t *testing.T) {
	limpias, err := NormalizarAreas([]AreaAsignada{
		{DepartamentoID: "d1", Rol: AreaTitular},
		{DepartamentoID: "d1", Rol: AreaSuplente},
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(limpias) != 1 {
		t.Fatalf("quedaron %d áreas; se esperaba 1", len(limpias))
	}
	// Gana la primera: es la que el usuario ve arriba en la lista.
	if limpias[0].Rol != AreaTitular {
		t.Errorf("rol = %q", limpias[0].Rol)
	}
}

// Y la misma partida dos veces tampoco.
func TestLaPartidaRepetidaNoSeDuplica(t *testing.T) {
	ids := SinRepetidos([]string{"c1", "c2", "c1", "", "c2"})
	if len(ids) != 2 || ids[0] != "c1" || ids[1] != "c2" {
		t.Errorf("ids = %v; se esperaba [c1 c2]", ids)
	}
}

// LA FICHA DICE SI LA PERSONA PUEDE ABRIR LA PANTALLA.
//
// Es el error que se comete la primera vez: asignar el alcance y olvidar el permiso. Sin este
// dato, la persona queda mirando un 403 y quien se lo asignó cree que lo dejó listo.
func TestLaFichaAvisaSiLeFaltaElPermiso(t *testing.T) {
	repo := &fakeRepo{
		usuarioDeLaEmpresa:  true,
		areasDelUsuario:     []AreaDelUsuario{{DepartamentoID: "d1", Departamento: "Mercadeo", Rol: AreaTitular}},
		puedeVerPresupuesto: false, // le asignaron el área pero NO el permiso
	}
	svc := NewService(repo, nil, zap.NewNop(), false)

	a, err := svc.AlcanceDeUnUsuario(context.Background(), "e1", "u1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(a.Areas) != 1 {
		t.Fatalf("áreas = %d", len(a.Areas))
	}
	if a.PuedeVerPresupuesto {
		t.Errorf("dice que puede ver el presupuesto y no tiene el permiso")
	}
}
