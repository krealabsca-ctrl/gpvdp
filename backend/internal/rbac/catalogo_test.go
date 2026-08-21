package rbac

import "testing"

// La matriz por defecto y los permisos del rol nuevo solo pueden referenciar
// códigos que existan en el catálogo (atrapa typos que romperían el seed).
func TestMatrizDefaultReferenciaCatalogoValido(t *testing.T) {
	valido := map[string]bool{}
	for _, p := range Catalogo {
		valido[p.Codigo] = true
	}
	for rol, permisos := range MatrizDefault {
		for _, code := range permisos {
			if !valido[code] {
				t.Errorf("MatrizDefault[%s] referencia permiso inexistente: %q", rol, code)
			}
		}
	}
	for _, code := range PermisosRolNuevo {
		if !valido[code] {
			t.Errorf("PermisosRolNuevo referencia permiso inexistente: %q", code)
		}
	}
}

func TestCatalogoSinDuplicados(t *testing.T) {
	visto := map[string]bool{}
	for _, p := range Catalogo {
		if visto[p.Codigo] {
			t.Errorf("código de permiso duplicado: %q", p.Codigo)
		}
		visto[p.Codigo] = true
	}
	if len(Catalogo) != 55 {
		t.Errorf("catálogo tiene %d permisos, se esperaban 55", len(Catalogo))
	}
}

func TestDirectorFinancieroTieneTodo(t *testing.T) {
	// El DF administra la matriz: su default debe incluir admin.roles y todo el resto.
	if len(MatrizDefault["DIRECTOR_FINANCIERO"]) != len(Catalogo) {
		t.Errorf("DIRECTOR_FINANCIERO debería tener los %d permisos por defecto", len(Catalogo))
	}
}

func TestCodigoDesdeNombre(t *testing.T) {
	casos := map[string]string{
		"Solo Bancos":  "CUSTOM_SOLO_BANCOS",
		"Tesorería":    "CUSTOM_TESORERA", // la í no-ASCII se descarta
		"  Auditor 2 ": "CUSTOM_AUDITOR_2",
	}
	for in, want := range casos {
		if got := codigoDesdeNombre(in); got != want {
			t.Errorf("codigoDesdeNombre(%q) = %q, quiere %q", in, got, want)
		}
	}
}
