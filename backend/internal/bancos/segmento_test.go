package bancos

// Tests de la consulta por segmento (mig 0077).
//
// El primero es el que importa: un rol sin partidas asignadas NO puede ver la empresa completa. Es
// el error clásico de este tipo de recorte —el filtro se arma con `if len(ids) > 0`, la lista queda
// vacía, la condición no se agrega y el WHERE se abre— y no falla ruidosamente: entrega datos.

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

func servicioSegmento(repo *fakeRepo) *Service {
	return NewService(repo, nil, zap.NewNop(), true)
}

func TestMiSegmentoSinAlcanceNoDevuelveNada(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		alcance: nil, // al rol no le asignaron ninguna partida
		// Si el servicio llegara a consultar, el fake devolvería estas filas: son la trampa.
		listaMovs: ListaMovimientos{Items: []MovimientoRow{{ID: "mov-de-otra-partida"}}, Total: 1},
	}
	svc := servicioSegmento(repo)

	res, err := svc.MiSegmento(context.Background(), "emp-1", "usr-1", FiltrosMovimientos{})

	if !errors.Is(err, ErrSinAlcance) {
		t.Fatalf("esperaba ErrSinAlcance, obtuve %v", err)
	}
	if len(res.Movimientos.Items) != 0 {
		t.Fatalf("con alcance vacío no puede devolver movimientos, devolvió %d", len(res.Movimientos.Items))
	}
	if repo.filtroMovs.Alcance != nil {
		t.Fatal("no debería ni consultar la tabla de movimientos sin alcance")
	}
}

func TestMiSegmentoFuerzaAlcanceYSoloCreditos(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{alcance: []string{"clasif-asociaciones"}}
	svc := servicioSegmento(repo)

	// El cliente intenta mirar otra partida y también los débitos.
	_, err := svc.MiSegmento(context.Background(), "emp-1", "usr-1", FiltrosMovimientos{
		ClasificacionIDs: []string{"clasif-ajena"},
		Tipo:             "DEBITO",
	})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}

	if len(repo.filtroMovs.Alcance) != 1 || repo.filtroMovs.Alcance[0] != "clasif-asociaciones" {
		t.Fatalf("el alcance tiene que salir del rol, llegó %v", repo.filtroMovs.Alcance)
	}
	if repo.filtroMovs.Tipo != "CREDITO" {
		t.Fatalf("la pantalla es de ingresos: esperaba CREDITO, llegó %q", repo.filtroMovs.Tipo)
	}
	// Lo que pidió el cliente sigue viajando: se INTERSECA con el alcance en el WHERE (todas las
	// condiciones se suman con AND), así que afinar sí se puede y ensanchar no.
	if len(repo.filtroMovs.ClasificacionIDs) != 1 || repo.filtroMovs.ClasificacionIDs[0] != "clasif-ajena" {
		t.Fatalf("el filtro del cliente no debería desaparecer, llegó %v", repo.filtroMovs.ClasificacionIDs)
	}
}

func TestMiSegmentoMarcaLosYaReportados(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		alcance: []string{"clasif-1"},
		listaMovs: ListaMovimientos{Items: []MovimientoRow{
			{ID: "mov-1"}, {ID: "mov-2"},
		}},
		reportesAbiertos: map[string]string{"mov-2": "esto es de Emergencias"},
	}
	svc := servicioSegmento(repo)

	res, err := svc.MiSegmento(context.Background(), "emp-1", "usr-1", FiltrosMovimientos{})
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if res.Movimientos.Items[0].ReporteAbierto != "" {
		t.Fatal("mov-1 no está reportado")
	}
	if res.Movimientos.Items[1].ReporteAbierto != "esto es de Emergencias" {
		t.Fatalf("mov-2 debía traer el motivo del aviso, trajo %q", res.Movimientos.Items[1].ReporteAbierto)
	}
}

func TestReportarSegmentacion(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre     string
		alcance    []string
		enAlcance  bool
		motivo     string
		esperaErr  error
		esperaAlta bool
	}{
		{
			nombre:     "avisa sobre un movimiento de su partida",
			alcance:    []string{"clasif-1"},
			enAlcance:  true,
			motivo:     "esto es de Emergencias",
			esperaAlta: true,
		},
		{
			// La guarda que importa: sin ella, mandando ids a mano se podría reportar —y por lo
			// tanto descubrir— cualquier movimiento de la empresa.
			nombre:    "un movimiento fuera de su alcance no existe para él",
			alcance:   []string{"clasif-1"},
			enAlcance: false,
			motivo:    "curioseando",
			esperaErr: ErrFueraDeAlcance,
		},
		{
			nombre:    "sin motivo no se puede corregir nada",
			alcance:   []string{"clasif-1"},
			enAlcance: true,
			motivo:    "   ",
			esperaErr: ErrMotivoRequerido,
		},
		{
			nombre:    "un rol sin partidas no puede avisar",
			alcance:   nil,
			enAlcance: true, // aunque el repo dijera que sí: el corte es antes
			motivo:    "algo",
			esperaErr: ErrSinAlcance,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{alcance: c.alcance, movEnAlcance: c.enAlcance}
			svc := servicioSegmento(repo)

			err := svc.ReportarSegmentacion(context.Background(), "emp-1", "usr-1", "mov-1", c.motivo)

			if c.esperaErr != nil {
				if !errors.Is(err, c.esperaErr) {
					t.Fatalf("esperaba %v, obtuve %v", c.esperaErr, err)
				}
				if repo.reporteCreado[0] != "" {
					t.Fatal("no debía crear el aviso")
				}
				return
			}
			if err != nil {
				t.Fatalf("no esperaba error: %v", err)
			}
			if !c.esperaAlta {
				return
			}
			if repo.reporteCreado != [3]string{"mov-1", "usr-1", c.motivo} {
				t.Fatalf("el aviso quedó mal registrado: %v", repo.reporteCreado)
			}
		})
	}
}

func TestResolverReporte(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre     string
		resolucion string
		respuesta  string
		esperaErr  error
	}{
		{nombre: "reclasificado no necesita explicación", resolucion: "RECLASIFICADO"},
		{
			// Cerrar «estaba bien» sin decir por qué deja al equipo con la misma duda, y el mismo
			// movimiento vuelve a reportarse el mes siguiente.
			nombre:     "sin cambio exige explicarle al equipo",
			resolucion: "SIN_CAMBIO",
			esperaErr:  ErrRespuestaRequerida,
		},
		{
			nombre:     "sin cambio con explicación cierra",
			resolucion: "sin_cambio",
			respuesta:  "esa planilla la paga la asociación, no Emergencias",
		},
		{nombre: "cualquier otra resolución no existe", resolucion: "ARCHIVADO", esperaErr: ErrResolucionInvalida},
		{nombre: "vacía tampoco", resolucion: "", esperaErr: ErrResolucionInvalida},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{}
			svc := servicioSegmento(repo)

			err := svc.ResolverReporte(context.Background(), "emp-1", "rep-1", "usr-1", c.resolucion, c.respuesta)

			if c.esperaErr != nil {
				if !errors.Is(err, c.esperaErr) {
					t.Fatalf("esperaba %v, obtuve %v", c.esperaErr, err)
				}
				if repo.reporteResuelto[0] != "" {
					t.Fatal("no debía resolver")
				}
				return
			}
			if err != nil {
				t.Fatalf("no esperaba error: %v", err)
			}
			// La resolución se normaliza a mayúsculas: «sin_cambio» y «SIN_CAMBIO» son lo mismo, y
			// la columna tiene un CHECK que solo acepta la forma de arriba.
			if repo.reporteResuelto[2] != ResolucionSinCambio && repo.reporteResuelto[2] != ResolucionReclasificado {
				t.Fatalf("resolución sin normalizar: %q", repo.reporteResuelto[2])
			}
		})
	}
}

// ── Buscar un movimiento que no aparece (mig 0078) ──────────────────────────

func TestBuscarFaltanteNoFiltraDatosAjenos(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		alcance: []string{"clasif-1"},
		// Existe uno igual, pero está en otra partida: el repositorio lo cuenta y no lo trae.
		busquedaMios:  nil,
		busquedaFuera: 1,
	}
	svc := servicioSegmento(repo)

	res, err := svc.BuscarFaltante(context.Background(), "emp-1", "usr-1", "2026-09-03", "4950")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if res.Veredicto != FaltanteFueraDeMiPartida {
		t.Fatalf("veredicto = %q, quería FUERA_DE_MI_PARTIDA", res.Veredicto)
	}
	// LO QUE IMPORTA: el veredicto dice que existe, y nada más. Ni descripción, ni cuenta, ni
	// partida, ni id. Si algún día alguien agrega el movimiento a la respuesta «para que se vea
	// mejor», este test se pone rojo.
	if len(res.Movimientos) != 0 {
		t.Fatalf("no puede devolver ni un dato del movimiento ajeno, devolvió %d", len(res.Movimientos))
	}
}

func TestBuscarFaltante(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre    string
		alcance   []string
		mios      []MovimientoRow
		fuera     int
		fecha     string
		monto     string
		veredicto string
		esperaErr error
	}{
		{
			nombre:    "es suyo: lo tapaba un filtro, se devuelve completo",
			alcance:   []string{"clasif-1"},
			mios:      []MovimientoRow{{ID: "mov-1", Credito: "4950"}},
			fecha:     "2026-09-03",
			monto:     "4950",
			veredicto: FaltanteEnMiPartida,
		},
		{
			nombre:    "no existe en toda la empresa",
			alcance:   []string{"clasif-1"},
			fecha:     "2026-09-03",
			monto:     "4950",
			veredicto: FaltanteNoExiste,
		},
		{
			// Los montos se escriben como se escriben en Costa Rica. Si hubiera que adivinar el
			// formato para poder buscar el propio depósito, la pantalla no sirve.
			nombre:    "acepta el monto con separadores de miles y coma decimal",
			alcance:   []string{"clasif-1"},
			mios:      []MovimientoRow{{ID: "mov-1"}},
			fecha:     "2026-09-03",
			monto:     "4.950,50",
			veredicto: FaltanteEnMiPartida,
		},
		{nombre: "fecha inválida", alcance: []string{"c"}, fecha: "03/09/2026", monto: "1", esperaErr: ErrFechaInvalida},
		{nombre: "monto en cero", alcance: []string{"c"}, fecha: "2026-09-03", monto: "0", esperaErr: ErrMontoInvalido},
		{nombre: "monto que no es número", alcance: []string{"c"}, fecha: "2026-09-03", monto: "como sea", esperaErr: ErrMontoInvalido},
		{nombre: "sin alcance no se puede buscar", alcance: nil, fecha: "2026-09-03", monto: "1", esperaErr: ErrSinAlcance},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{alcance: c.alcance, busquedaMios: c.mios, busquedaFuera: c.fuera}
			svc := servicioSegmento(repo)

			res, err := svc.BuscarFaltante(context.Background(), "emp-1", "usr-1", c.fecha, c.monto)

			if c.esperaErr != nil {
				if !errors.Is(err, c.esperaErr) {
					t.Fatalf("esperaba %v, obtuve %v", c.esperaErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("no esperaba error: %v", err)
			}
			if res.Veredicto != c.veredicto {
				t.Fatalf("veredicto = %q, quería %q", res.Veredicto, c.veredicto)
			}
		})
	}
}

func TestBuscarFaltanteInexistenteDiceHastaCuandoEstaCargado(t *testing.T) {
	t.Parallel()
	// Sin esa fecha, «no hay ninguno» no distingue «no entró» de «no lo han importado», que es la
	// mitad de la pregunta que la pantalla existe para contestar.
	repo := &fakeRepo{alcance: []string{"clasif-1"}, cargadoHasta: "2026-09-07"}
	svc := servicioSegmento(repo)

	res, err := svc.BuscarFaltante(context.Background(), "emp-1", "usr-1", "2026-09-09", "4950")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if res.Veredicto != FaltanteNoExiste || res.CargadoHasta != "2026-09-07" {
		t.Fatalf("esperaba NO_EXISTE con cargado_hasta, obtuve %q / %q", res.Veredicto, res.CargadoHasta)
	}
}

func TestReportarFaltanteEnganchaSinPasarElIDPorElCliente(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{alcance: []string{"clasif-1"}, enganche: "mov-ajeno-7"}
	svc := servicioSegmento(repo)

	err := svc.ReportarFaltante(context.Background(), "emp-1", "usr-1",
		"2026-09-03", "4 950,00", "depósito de ventanilla", "no me aparece el depósito")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	got := repo.faltanteCreado
	if got[0] != "usr-1" || got[1] != "2026-09-03" || got[2] != "4950" {
		t.Fatalf("el aviso quedó mal registrado: %v", got)
	}
	// El movimiento lo engancha el SERVIDOR: el cliente nunca manda ni recibe ese id, porque el
	// equipo no puede ver ese movimiento.
	if got[5] != "mov-ajeno-7" {
		t.Fatalf("esperaba el movimiento enganchado, obtuve %q", got[5])
	}
}

func TestReportarFaltanteExigeDecirQueEsperaba(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{alcance: []string{"clasif-1"}}
	svc := servicioSegmento(repo)

	err := svc.ReportarFaltante(context.Background(), "emp-1", "usr-1", "2026-09-03", "4950", "", "  ")
	if !errors.Is(err, ErrMotivoRequerido) {
		t.Fatalf("esperaba ErrMotivoRequerido, obtuve %v", err)
	}
	if repo.faltanteCreado[0] != "" {
		t.Fatal("no debía crear el aviso")
	}
}

func TestGuardarConsultaDePartidaLimpiaLaLista(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := servicioSegmento(repo)

	// Un rol repetido y uno vacío: la pantalla puede mandarlos y no pueden convertirse en un error
	// de conflicto que se lea como «el rol no existe».
	err := svc.GuardarConsultaDePartida(context.Background(), "emp-1", "clasif-1",
		[]string{"rol-a", " rol-b ", "rol-a", ""}, "usr-1")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	got := repo.consultaGuardada.rolIDs
	if len(got) != 2 || got[0] != "rol-a" || got[1] != "rol-b" {
		t.Fatalf("esperaba [rol-a rol-b] sin repetidos ni espacios, obtuve %v", got)
	}
}

func TestGuardarConsultaDePartidaAceptaListaVacia(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := servicioSegmento(repo)

	// «Esta partida no la consulta ningún equipo» es el estado normal de casi todas: de las 170
	// clasificaciones de Valle de Paz se asignan las tres o cuatro que tienen equipo detrás.
	if err := svc.GuardarConsultaDePartida(context.Background(), "emp-1", "clasif-1", nil, "usr-1"); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if !repo.consultaGuardada.llamado {
		t.Fatal("quitarle el último rol a una partida tiene que llegar a la base")
	}
	if len(repo.consultaGuardada.rolIDs) != 0 {
		t.Fatalf("esperaba lista vacía, obtuve %v", repo.consultaGuardada.rolIDs)
	}
}
