package bancos

import "testing"

// El ORDEN y los NOMBRES de las columnas del detalle los fija el negocio, no el código.
//
// Se escriben acá porque ya cambiaron dos veces (2026-09-02 y 2026-09-03) y porque quien recibe el
// .xlsx lo pega en sus propias hojas: mover una columna le rompe las fórmulas del otro lado. Si este
// test se pone rojo, el orden cambió — y tiene que ser porque alguien lo decidió, no de rebote.
func TestOrdenDeColumnasDelDetalle(t *testing.T) {
	t.Parallel()
	quiere := []string{
		"Fecha",
		"Consecutivo",
		"Débito",
		"Crédito",
		"Equivalencia",
		"Descripción",
		"Banco Cuenta",
		"Concepto",
		"Clasificación",
		"Consecutivo largo",
	}
	cols := columnasDetalle()
	if len(cols) != len(quiere) {
		t.Fatalf("el detalle tiene %d columnas, el negocio fijó %d", len(cols), len(quiere))
	}
	for i, q := range quiere {
		if cols[i].Titulo != q {
			t.Errorf("columna %d = %q, quiere %q", i, cols[i].Titulo, q)
		}
	}
}

// La fila tiene que tener tantos valores como columnas hay. Si se agrega una columna y se olvida el
// valor (o al revés), el .xlsx sale con los montos corridos una posición: un archivo que se ve bien
// y miente. Es el peor de los dos mundos, así que se vigila.
func TestFilaDetalleTieneUnValorPorColumna(t *testing.T) {
	t.Parallel()
	m := MovimientoExport{
		Fecha: "2026-08-01", Documento: "1001", Descripcion: "PAGO",
		Banco: "Davivienda", Cuenta: "Colones",
		Debito: "1000.00", Credito: "0", MontoCRC: "1000.00",
	}
	fila := filaDetalle(m, "Gastos", "Planilla")
	if len(fila) != len(columnasDetalle()) {
		t.Fatalf("la fila trae %d valores y hay %d columnas", len(fila), len(columnasDetalle()))
	}
	// Y cada valor tiene que caer bajo su propio encabezado.
	if fila[1] != "1001" {
		t.Errorf("Consecutivo = %v, quiere el documento del banco", fila[1])
	}
	if fila[5] != "PAGO" {
		t.Errorf("Descripción = %v; quedó en otra posición", fila[5])
	}
	if fila[6] != "Davivienda · Colones" {
		t.Errorf("Banco Cuenta = %v, quiere «Davivienda · Colones»", fila[6])
	}
}

// «Banco Cuenta» identifica la cuenta en una sola columna, sin repetir el banco ni dejar
// separadores sueltos.
//
// Los casos con nombres REALES de la empresa: los alias ya nombran el banco («Davivienda Colones»),
// así que concatenar a ciegas daba «Davivienda · Davivienda Colones» en cada fila — un dato repetido
// que se lee como un error del sistema.
func TestBancoCuentaNoRepiteNiDejaSeparadorSuelto(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre, banco, cuenta, quiere string
	}{
		{"el alias ya trae el banco", "Davivienda", "Davivienda Colones", "Davivienda Colones"},
		{"con tilde en el alias", "Davivienda", "Davivienda Dólares", "Davivienda Dólares"},
		{"BN abrevia igual que el alias", "BN", "BN Jardines Colones", "BN Jardines Colones"},
		{"el alias NO nombra el banco", "Banco Popular", "BP Negocios", "Banco Popular · BP Negocios"},
		{"alias corto sin el banco", "Davivienda", "Colones", "Davivienda · Colones"},
		{"cuenta sin alias", "Davivienda", "", "Davivienda"},
		{"sin banco", "", "Colones", "Colones"},
		{"sin nada", "", "", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := bancoCuenta(MovimientoExport{Banco: c.banco, Cuenta: c.cuenta})
			if got != c.quiere {
				t.Errorf("bancoCuenta(%q, %q) = %q, quiere %q", c.banco, c.cuenta, got, c.quiere)
			}
		})
	}
}
