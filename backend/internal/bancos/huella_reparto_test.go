package bancos

// La huella del reparto: lo que hace que una parte apartada deje de verse igual que un
// movimiento normal.
//
// Lo que se prueba acá es que la huella diga CUÁL de las dos caras es cada fila y que las dos
// compartan el mismo código. Invertir ese booleano, o derivar dos códigos distintos para el
// mismo reparto, da una pantalla que se ve bien y agrupa mal.

import "testing"

const idPrincipal = "a3f2b1c4-0000-4000-8000-000000000001"
const idParte = "99887766-0000-4000-8000-000000000002"

func TestCodigoDeReparto(t *testing.T) {
	t.Parallel()
	casos := []struct{ nombre, id, quiere string }{
		{"uuid normal", idPrincipal, "DIV-A3F2B1C4"},
		{"pasa a mayúscula", "deadbeef-1111-4111-8111-111111111111", "DIV-DEADBEEF"},
		{"ya en mayúscula", "DEADBEEF-1111-4111-8111-111111111111", "DIV-DEADBEEF"},
		{"se saltan los guiones", "ab-cdef12-3456", "DIV-ABCDEF12"},
		// Sin id no hay reparto que nombrar: devolver «DIV-» a secas sería un código que no
		// identifica nada y que al buscarlo traería todo o nada.
		{"vacío no inventa código", "", ""},
		{"demasiado corto", "abc", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := CodigoDeReparto(c.id); got != c.quiere {
				t.Errorf("CodigoDeReparto(%q) = %q, quiere %q", c.id, got, c.quiere)
			}
		})
	}
}

// LAS DOS CARAS COMPARTEN EL CÓDIGO. Es lo único que las agrupa: si el principal y su parte
// derivaran códigos distintos, la huella sería decorativa —diría «esto se dividió» sin permitir
// encontrar con qué—.
func TestLasDosCarasDelRepartoCompartenElCodigo(t *testing.T) {
	t.Parallel()
	principal := huellaDeReparto(idPrincipal, "", "1421500.00", 2)
	parte := huellaDeReparto(idParte, idPrincipal, "1421500.00", 2)

	if principal == nil || parte == nil {
		t.Fatalf("falta una huella: principal=%v parte=%v", principal, parte)
	}
	if principal.Codigo != parte.Codigo {
		t.Errorf("códigos distintos: principal %q, parte %q", principal.Codigo, parte.Codigo)
	}
	if principal.Codigo != "DIV-A3F2B1C4" {
		t.Errorf("código %q, quiere DIV-A3F2B1C4", principal.Codigo)
	}
	// Y las dos apuntan al MISMO principal, que es a donde lleva el clic.
	if principal.PrincipalID != idPrincipal || parte.PrincipalID != idPrincipal {
		t.Errorf("principal_id: %q y %q", principal.PrincipalID, parte.PrincipalID)
	}
}

// LA DIRECCIÓN NO SE INVIERTE. «A esta fila la apartaron de otra» y «a esta fila le apartaron
// algo» son frases opuestas, y el usuario lee una de las dos.
func TestLaHuellaDistingueElPrincipalDeLaParte(t *testing.T) {
	t.Parallel()
	principal := huellaDeReparto(idPrincipal, "", "1421500.00", 2)
	if principal.EsParte {
		t.Error("el movimiento original salió marcado como parte")
	}
	parte := huellaDeReparto(idParte, idPrincipal, "1421500.00", 2)
	if !parte.EsParte {
		t.Error("la parte apartada salió marcada como principal")
	}
}

// UN MOVIMIENTO NORMAL NO LLEVA HUELLA. Marcar de más es tan malo como no marcar: si casi todas
// las filas traen el objeto, el ojo deja de verlo y la marca pierde su sentido.
func TestUnMovimientoSinRepartoNoLlevaHuella(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre, divididoDe, montoOriginal string
		partes                            int
	}{
		{"nunca se dividió", "", "", 0},
		// El repositorio devuelve "0" cuando el LATERAL no encontró reparto: tratarlo como monto
		// válido marcaría TODOS los movimientos como divididos por ₡0.
		{"monto original en cero", "", "0", 0},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if h := huellaDeReparto(idPrincipal, c.divididoDe, c.montoOriginal, c.partes); h != nil {
				t.Errorf("salió huella donde no hay reparto: %+v", h)
			}
		})
	}
}

// UNA PARTE SIEMPRE LLEVA HUELLA, aunque su monto original venga raro: lo que la hace parte es
// tener padre, no el monto. Sin esto, una parte cuyo padre perdió el monto quedaría indistinguible
// de un movimiento normal, que es exactamente el defecto que la huella viene a cerrar.
func TestUnaParteLlevaHuellaAunqueElMontoVengaVacio(t *testing.T) {
	t.Parallel()
	h := huellaDeReparto(idParte, idPrincipal, "", 2)
	if h == nil {
		t.Fatal("una parte sin monto original quedó sin huella")
	}
	if !h.EsParte || h.PrincipalID != idPrincipal {
		t.Errorf("huella mal armada: %+v", h)
	}
}

// El monto original y la cantidad de partes viajan tal cual: son lo que permite cruzar la fila
// contra el estado de cuenta sin abrir nada.
func TestLaHuellaLlevaElMontoOriginalYCuantasPartes(t *testing.T) {
	t.Parallel()
	h := huellaDeReparto(idParte, idPrincipal, "1421500.00", 3)
	if h.MontoOriginal != "1421500.00" {
		t.Errorf("monto original %q", h.MontoOriginal)
	}
	if h.Partes != 3 {
		t.Errorf("partes %d, quiere 3", h.Partes)
	}
}

// El .xlsx y la pantalla tienen que decir el MISMO código, o no sirven para cruzar uno contra
// otro — que es justamente para lo que existe la columna.
func TestElExportUsaElMismoCodigoQueLaPantalla(t *testing.T) {
	t.Parallel()
	enPantalla := huellaDeReparto(idParte, idPrincipal, "1421500.00", 2).Codigo
	enExcel := CodigoDeReparto(idPrincipal)
	if enPantalla != enExcel {
		t.Errorf("pantalla %q vs .xlsx %q", enPantalla, enExcel)
	}
	// Y la fila del detalle lo pone bajo su propio encabezado, no corrido.
	m := MovimientoExport{Fecha: "2026-08-01", Documento: "1001", Reparto: enExcel}
	fila := filaDetalle(m, "Gastos", "Electricidad")
	cols := columnasDetalle()
	if cols[len(cols)-1].Titulo != "Reparto" {
		t.Fatalf("la última columna es %q", cols[len(cols)-1].Titulo)
	}
	if fila[len(fila)-1] != enExcel {
		t.Errorf("la columna Reparto trae %v, quiere %q", fila[len(fila)-1], enExcel)
	}
}
