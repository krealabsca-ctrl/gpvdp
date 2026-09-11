package rbac

import (
	"strings"
	"testing"
)

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
	// El total va fijo a propósito: agregar un permiso obliga a pasar por acá, y eso es lo que
	// recuerda que también hay que sumarlo a MatrizDefault (si no, el permiso queda congelado y
	// ninguna empresa nueva lo recibe). 55 → 61 con Inventario (Fase 1), → 62 con el conteo cíclico,
	// → 63 con la consignación (facturar al proveedor lo que se usó de su mercadería),
	// → 64 con la vista consolidada del grupo (la única que cruza empresas),
	// → 72 al partir la lectura de Bancos en una por pantalla (8 nuevas),
	// → 73 con `cxp.catalogo` (Contabilidad abre sus propios rubros de gasto),
	// → 74 con `bancos.ver_mi_segmento` (el equipo de una partida consulta SUS créditos).
	//
	// `bancos.ver_mi_segmento` es el primero que a propósito NO va a ningún rol de la matriz: es la
	// única pantalla de los roles de consulta que el negocio crea a mano (Cobros Asociaciones,
	// Servicio / Sala, Emergencias), y los roles financieros ya leen el módulo por su propio
	// permiso. DIRECTOR_FINANCIERO lo recibe igual porque su fila es `codigos()`, y ahí es
	// inofensivo: con el alcance vacío la pantalla no muestra ni una fila.
	// → 76 al pasar las acciones en lote de CxP de código de rol a permiso (mig 0079): `cxp.anular`
	// y `cxp.resultado_pago` existen porque su recorte vivía SOLO en una lista de roles escrita a
	// mano, y por eso ningún rol a medida podía denegar, anular ni rebotar.
	// → 78 con la recepción de facturas por buzón de correo (mig 0081): `cxp.recepcion` abre la
	// bandeja de lo que llega por correo y la cola de errores, y `cxp.fuentes` da de alta los
	// buzones. `cxp.fuentes` es SENSIBLE porque crear una fuente crea una credencial y decide de
	// qué empresa son las facturas que entran.
	if len(Catalogo) != 78 {
		t.Errorf("catálogo tiene %d permisos, se esperaban 78", len(Catalogo))
	}
}

// Partir `bancos.ver` no puede quitarle acceso a nadie: todo rol que hoy lee Bancos tiene que
// seguir leyéndolo completo. Lo nuevo es poder DESTILDAR pantallas, no perderlas de golpe.
func TestRolesQueLeianBancosSiguenLeyendoTodo(t *testing.T) {
	for rol, permisos := range MatrizDefault {
		tiene := map[string]bool{}
		for _, p := range permisos {
			tiene[p] = true
		}
		if !tiene["bancos.ver"] {
			continue // no leía Bancos; no hay nada que preservar
		}
		for _, p := range LecturaBancosCompleta {
			if !tiene[p] {
				t.Errorf("%s tenía bancos.ver pero le falta %q: el cambio le quita acceso que hoy tiene", rol, p)
			}
		}
	}
}

// Los ocho permisos por pantalla tienen que estar en el catálogo, o la migración 0074 los inserta
// en la base y el código nunca los pide.
func TestLecturaBancosCompletaEstaEnElCatalogo(t *testing.T) {
	valido := map[string]bool{}
	for _, p := range Catalogo {
		valido[p.Codigo] = true
	}
	for _, p := range LecturaBancosCompleta {
		if !valido[p] {
			t.Errorf("LecturaBancosCompleta referencia %q, que no está en el catálogo", p)
		}
	}
	if len(LecturaBancosCompleta) != 9 {
		t.Errorf("LecturaBancosCompleta tiene %d permisos, se esperaban 9 (la base + 8 pantallas)",
			len(LecturaBancosCompleta))
	}
}

// `bancos.ver` ya no puede describirse como «ve todo el módulo»: es la entrada a los catálogos.
// Si alguien revierte la descripción, la matriz vuelve a mentirle a quien reparte permisos.
func TestBancosVerSeDescribeComoEntrada(t *testing.T) {
	for _, p := range Catalogo {
		if p.Codigo != "bancos.ver" {
			continue
		}
		if strings.Contains(p.Descripcion, "Dashboard") {
			t.Errorf("la descripción de bancos.ver sigue prometiendo el dashboard: %q", p.Descripcion)
		}
		return
	}
	t.Fatal("bancos.ver no está en el catálogo")
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
