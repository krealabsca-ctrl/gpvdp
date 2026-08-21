package plantillas

// Tests del motor de plantillas. Lo que importa: que a nadie le llegue un «[VARIABLE]» crudo, y
// que no se guarde una plantilla que el sistema no sepa llenar.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	valores := map[string]string{"NOMBRE_PROVEEDOR": "Gas Tomza", "MONTO": "₡137 450,00"}

	t.Run("reemplaza las variables", func(t *testing.T) {
		got := Render("Hola [NOMBRE_PROVEEDOR], van [MONTO].", valores)
		if got != "Hola Gas Tomza, van ₡137 450,00." {
			t.Errorf("got = %q", got)
		}
	})

	t.Run("una variable sin valor queda vacía, NUNCA como marcador", func(t *testing.T) {
		got := Render("Hola [NOMBRE_PROVEEDOR], ref [REFERENCIA].", valores)
		if strings.Contains(got, "[") {
			t.Errorf("al proveedor no le puede llegar un marcador: %q", got)
		}
		if got != "Hola Gas Tomza, ref ." {
			t.Errorf("got = %q", got)
		}
	})

	t.Run("no toca corchetes que no son variables", func(t *testing.T) {
		// Minúsculas o con espacios NO son variables: el cuerpo puede llevar corchetes.
		got := Render("Ver el detalle [ver aquí] y [minuscula]", valores)
		if got != "Ver el detalle [ver aquí] y [minuscula]" {
			t.Errorf("got = %q", got)
		}
	})
}

func TestVariablesUsadas(t *testing.T) {
	got := VariablesUsadas("[B] y [A]", "otra vez [A] y [C]")
	quiere := []string{"A", "B", "C"}
	if len(got) != 3 {
		t.Fatalf("got = %v", got)
	}
	for i := range quiere {
		if got[i] != quiere[i] {
			t.Errorf("got = %v, quiere %v (sin repetir y ordenadas)", got, quiere)
		}
	}
}

func TestDesconocidas(t *testing.T) {
	tipo, ok := TipoPorClave(ClaveCxPComprobante)
	if !ok {
		t.Fatal("falta el tipo del comprobante en el catálogo")
	}
	if d := tipo.Desconocidas(tipo.AsuntoDefault, tipo.CuerpoDefault); len(d) != 0 {
		t.Errorf("el texto de fábrica usa variables inexistentes: %v", d)
	}
	if d := tipo.Desconocidas("Hola [INVENTADA] y [OTRA_MAS]"); len(d) != 2 {
		t.Errorf("desconocidas = %v, quiere 2", d)
	}
}

func TestCatalogoCoherente(t *testing.T) {
	// Cada tipo tiene que poder renderizarse solo con SUS variables, y traer ejemplo para la
	// vista previa. Si alguien agrega un tipo mal, se cae acá y no en la cara de un proveedor.
	for _, tipo := range Catalogo {
		t.Run(tipo.Clave, func(t *testing.T) {
			if tipo.Nombre == "" || tipo.AsuntoDefault == "" || tipo.CuerpoDefault == "" {
				t.Error("el tipo debe traer nombre, asunto y cuerpo por defecto")
			}
			if d := tipo.Desconocidas(tipo.AsuntoDefault, tipo.CuerpoDefault); len(d) != 0 {
				t.Errorf("su texto por defecto usa variables que no declara: %v", d)
			}
			for _, v := range tipo.Variables {
				if v.Descripcion == "" || v.Ejemplo == "" {
					t.Errorf("la variable %s necesita descripción y ejemplo (se muestran en la UI)", v.Nombre)
				}
			}
			// Renderizado con los ejemplos: no debe quedar ningún marcador.
			cuerpo := Render(tipo.CuerpoDefault, tipo.Ejemplos())
			if strings.Contains(cuerpo, "[") {
				t.Errorf("quedó un marcador sin llenar en la vista previa: %q", cuerpo)
			}
		})
	}
}

// repoFake es el repositorio en memoria para probar el servicio.
type repoFake struct {
	guardadas map[string]Plantilla
	empresa   string
	borrada   string
	capAsunto string
	capCuerpo string
}

func (r *repoFake) Listar(context.Context, string) (map[string]Plantilla, error) {
	if r.guardadas == nil {
		return map[string]Plantilla{}, nil
	}
	return r.guardadas, nil
}
func (r *repoFake) Guardar(_ context.Context, _, clave, asunto, cuerpo, _ string) error {
	if r.guardadas == nil {
		r.guardadas = map[string]Plantilla{}
	}
	r.capAsunto, r.capCuerpo = asunto, cuerpo
	r.guardadas[clave] = Plantilla{Clave: clave, Asunto: asunto, Cuerpo: cuerpo, Personalizada: true}
	return nil
}
func (r *repoFake) Restablecer(_ context.Context, _, clave string) error {
	r.borrada = clave
	delete(r.guardadas, clave)
	return nil
}
func (r *repoFake) NombreEmpresa(context.Context, string) (string, error) {
	if r.empresa == "" {
		return "Valle de Paz", nil
	}
	return r.empresa, nil
}

func servicio(r *repoFake) *Service { return NewService(r, nil) }

func TestListarTraeElTextoVigente(t *testing.T) {
	ctx := context.Background()
	repo := &repoFake{guardadas: map[string]Plantilla{
		ClaveNominaBoleta: {Clave: ClaveNominaBoleta, Asunto: "Su boleta", Cuerpo: "Hola", Personalizada: true},
	}}
	items, err := servicio(repo).Listar(ctx, "e1")
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(items) != len(Catalogo) {
		t.Fatalf("tipos = %d, quiere %d", len(items), len(Catalogo))
	}
	for _, it := range items {
		switch it.Clave {
		case ClaveNominaBoleta:
			if !it.Vigente.Personalizada || it.Vigente.Asunto != "Su boleta" {
				t.Errorf("la personalizada debería regir: %+v", it.Vigente)
			}
		default:
			if it.Vigente.Personalizada || it.Vigente.Asunto != it.AsuntoDefault {
				t.Errorf("%s debería regir por defecto: %+v", it.Clave, it.Vigente)
			}
		}
	}
}

func TestGuardarValida(t *testing.T) {
	ctx := context.Background()
	svc := servicio(&repoFake{})

	if _, err := svc.Guardar(ctx, "e1", "NO_EXISTE", "a", "b", "u1"); !errors.Is(err, ErrTipoDesconocido) {
		t.Errorf("tipo inexistente: err = %v", err)
	}
	if _, err := svc.Guardar(ctx, "e1", ClaveNominaBoleta, "   ", "cuerpo", "u1"); !errors.Is(err, ErrAsuntoVacio) {
		t.Errorf("asunto vacío: err = %v", err)
	}
	if _, err := svc.Guardar(ctx, "e1", ClaveNominaBoleta, "asunto", "  ", "u1"); !errors.Is(err, ErrCuerpoVacio) {
		t.Errorf("cuerpo vacío: err = %v", err)
	}

	t.Run("rechaza variables que el sistema no sabe llenar", func(t *testing.T) {
		repo := &repoFake{}
		desc, err := servicio(repo).Guardar(ctx, "e1", ClaveNominaBoleta,
			"Boleta [PERIODO]", "Hola [NOMBRE_EMPLEADO], su bono es [BONO_INVENTADO]", "u1")
		if !errors.Is(err, ErrVariablesDesconocidas) {
			t.Fatalf("err = %v, quiere ErrVariablesDesconocidas", err)
		}
		if len(desc) != 1 || desc[0] != "BONO_INVENTADO" {
			t.Errorf("debería decir cuál sobra: %v", desc)
		}
		if len(repo.guardadas) != 0 {
			t.Error("no debería haber guardado nada")
		}
	})

	t.Run("guarda recortando espacios", func(t *testing.T) {
		repo := &repoFake{}
		if _, err := servicio(repo).Guardar(ctx, "e1", ClaveNominaBoleta,
			"  Boleta [PERIODO]  ", "  Hola [NOMBRE_EMPLEADO]  ", "u1"); err != nil {
			t.Fatalf("guardar: %v", err)
		}
		if repo.capAsunto != "Boleta [PERIODO]" || repo.capCuerpo != "Hola [NOMBRE_EMPLEADO]" {
			t.Errorf("asunto = %q, cuerpo = %q", repo.capAsunto, repo.capCuerpo)
		}
	})
}

func TestArmarUsaLaPlantillaYCompletaLoComun(t *testing.T) {
	ctx := context.Background()
	repo := &repoFake{
		empresa: "Memorial Pets",
		guardadas: map[string]Plantilla{
			ClaveCxPComprobante: {
				Clave:  ClaveCxPComprobante,
				Asunto: "Pago de [CONSECUTIVO]",
				Cuerpo: "Hola [NOMBRE_PROVEEDOR], van [MONTO]. — [NOMBRE_EMPRESA] [ANIO]",
			},
		},
	}
	asunto, cuerpo, err := servicio(repo).Armar(ctx, "e1", ClaveCxPComprobante, map[string]string{
		"NOMBRE_PROVEEDOR": "Gas Tomza", "CONSECUTIVO": "0001", "MONTO": "₡100,00",
	})
	if err != nil {
		t.Fatalf("armar: %v", err)
	}
	if asunto != "Pago de 0001" {
		t.Errorf("asunto = %q", asunto)
	}
	// El módulo no mandó NOMBRE_EMPRESA ni ANIO: el servicio los completa solo.
	if !strings.Contains(cuerpo, "Memorial Pets") {
		t.Errorf("debería completar el nombre de la empresa: %q", cuerpo)
	}
	if strings.Contains(cuerpo, "[") {
		t.Errorf("quedó un marcador sin llenar: %q", cuerpo)
	}
}

func TestVistaPreviaNoGuardaYUsaEjemplos(t *testing.T) {
	repo := &repoFake{empresa: "Coopeprofa"}
	asunto, cuerpo, desconocidas, err := servicio(repo).VistaPrevia(context.Background(), "e1",
		ClaveNominaBoleta, "", "")
	if err != nil {
		t.Fatalf("vista previa: %v", err)
	}
	if len(desconocidas) != 0 {
		t.Errorf("el texto de fábrica no debería tener desconocidas: %v", desconocidas)
	}
	if strings.Contains(asunto+cuerpo, "[") {
		t.Errorf("la vista previa debe salir sin marcadores: %q / %q", asunto, cuerpo)
	}
	if !strings.Contains(cuerpo, "Coopeprofa") {
		t.Errorf("debería usar el nombre real de la empresa activa: %q", cuerpo)
	}
	if len(repo.guardadas) != 0 {
		t.Error("la vista previa NO debe guardar")
	}
}

func TestRestablecer(t *testing.T) {
	repo := &repoFake{guardadas: map[string]Plantilla{
		ClaveNominaBoleta: {Clave: ClaveNominaBoleta, Asunto: "x", Cuerpo: "y", Personalizada: true},
	}}
	svc := servicio(repo)
	if err := svc.Restablecer(context.Background(), "e1", ClaveNominaBoleta, "u1"); err != nil {
		t.Fatalf("restablecer: %v", err)
	}
	if repo.borrada != ClaveNominaBoleta {
		t.Errorf("borrada = %q", repo.borrada)
	}
	items, _ := svc.Listar(context.Background(), "e1")
	for _, it := range items {
		if it.Clave == ClaveNominaBoleta && it.Vigente.Personalizada {
			t.Error("tras restablecer debería regir el texto de fábrica")
		}
	}
	if err := svc.Restablecer(context.Background(), "e1", "NO_EXISTE", "u1"); !errors.Is(err, ErrTipoDesconocido) {
		t.Errorf("tipo inexistente: err = %v", err)
	}
}
