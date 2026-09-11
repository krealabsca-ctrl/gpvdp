package bancos

// Visibilidad para CxP a nivel de CLASIFICACIÓN (mig 0080).
//
// Antes vivía solo en el concepto y la clasificación la heredaba: en Valle de Paz, 4 conceptos
// visibles exponían 124 clasificaciones a Contabilidad, y entre ellas salió gasto confidencial.

import (
	"context"

	"testing"

	"go.uber.org/zap"
)

func servicioVisibilidad(repo *fakeRepo) *Service {
	return NewService(repo, nil, zap.NewNop(), true)
}

func TestOcultarUnaClasificacionParaCxP(t *testing.T) {
	repo := &fakeRepo{}
	svc := servicioVisibilidad(repo)

	// Lo que importa es que el FALSE llegue: es el caso que oculta el rubro, y con un bool en el
	// cuerpo del pedido «false» y «no vino» serían indistinguibles.
	if err := svc.CambiarVisibilidadCxPClasificacion(context.Background(), "emp-1", "clasif-1", false, "usr-1"); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if repo.visibleClasifGuardada != [2]string{"clasif-1", "false"} {
		t.Fatalf("se guardó %v, quería ocultar clasif-1", repo.visibleClasifGuardada)
	}

	// Y que se pueda volver a mostrar.
	if err := svc.CambiarVisibilidadCxPClasificacion(context.Background(), "emp-1", "clasif-1", true, "usr-1"); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if repo.visibleClasifGuardada != [2]string{"clasif-1", "true"} {
		t.Fatalf("se guardó %v, quería mostrar clasif-1", repo.visibleClasifGuardada)
	}
}

// La guarda de la puerta de Contabilidad (`cxp.catalogo`, mig 0075) tiene que respetar el corte
// fino: si no, Contabilidad podría renombrar por esa puerta un rubro que la pantalla ya no le
// muestra. El fake devuelve un único valor para las dos guardas, así que se comprueban juntas.
func TestElAlcanceDeContabilidadRespetaLaVisibilidad(t *testing.T) {
	oculto := &fakeRepo{visibleCxP: false}
	svc := servicioVisibilidad(oculto)

	visible, err := svc.ClasificacionEsVisibleCxP(context.Background(), "emp-1", "clasif-oculta")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if visible {
		t.Error("una clasificación oculta no puede estar dentro del alcance de cxp.catalogo")
	}
}
