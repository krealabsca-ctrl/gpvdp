package cxp

// Lo que se emite tiene que sobrevivir al canal del banco (19-oct-2026).
//
// EL REPORTE (Director Financiero): «el banco devuelve así el detalle REM 006101 CXPR1760CCFC2435,
// pero nosotros mandamos REM 0061-01 CXPR-1760CCFC2435; creo que los guiones no nos sirven ni
// caracteres especiales».
//
// ES LA SEGUNDA VEZ QUE SE MIDE LO MISMO. El 30 de setiembre, un débito SINPE real devolvió
// «CXP54A4281AE49A» por lo que se había emitido como «CXP-54A4281AE49A»; entonces se hizo opcional
// el guion EN LA LECTURA, que salvó la conciliación pero dejó intacta la emisión. Este reporte es
// la otra mitad del mismo hecho, ahora con la referencia de la remesa adentro.
//
// LA CONCILIACIÓN NUNCA ESTUVO ROTA —el patrón acepta las dos formas— pero sí la BÚSQUEDA: quien
// copiaba «REM 004307» del estado de cuenta y lo pegaba en el buscador no encontraba nada, porque
// el sistema la tenía guardada como «REM 0043-07». Se ve igual que «ese pago no existe».

import (
	"strings"
	"testing"
)

// El caso exacto del reporte, de punta a punta.
func TestLoQueSeEmiteEsLoQueElBancoDevuelve(t *testing.T) {
	t.Parallel()
	// Un pago agrupado del lote 61, remesa 1.
	ref := ReferenciaRemesa(61, 1)
	emitido := descripcionDePago(PagoRow{
		CodigoPago:  "A9",
		Referencia:  ref,
		Descripcion: "CXPR-1760CCFC2435",
	})

	// Esto es, carácter por carácter, lo que el estado de cuenta trajo —sin el código A9, que en
	// aquel pago no había—.
	const devueltoPorElBanco = "REM 006101 CXPR1760CCFC2435"
	if !strings.HasSuffix(emitido, devueltoPorElBanco) {
		t.Errorf("se emite %q; el banco devuelve %q — tienen que coincidir", emitido, devueltoPorElBanco)
	}
	// Y lo que se emite ya no lleva nada que el canal vaya a borrar.
	if emitido != ParaElBanco(emitido) {
		t.Errorf("la descripción lleva algo que no sobrevive: %q", emitido)
	}
	// La conciliación sigue leyendo el código, que es de lo que depende todo.
	if h, ok := extraerHuella(devueltoPorElBanco); !ok || h != "CXPR-1760CCFC2435" {
		t.Errorf("extraerHuella(%q) = (%q, %v)", devueltoPorElBanco, h, ok)
	}
}

func TestLaReferenciaDeLaRemesaYaNoLlevaGuion(t *testing.T) {
	t.Parallel()
	casos := []struct {
		lote   int64
		remesa int
		quiere string
	}{
		{61, 1, "REM 006101"},
		{43, 7, "REM 004307"},
		// Los anchos fijos son los que la dejan leer sin separador: 4 para el lote, 2 para la
		// remesa. «REM 004307» es lote 43, remesa 7.
		{1, 1, "REM 000101"},
		// Y crecen antes que truncar: un lote de 5 dígitos alarga el texto, no lo recorta. Un
		// número recortado apuntaría a otro lote.
		{12345, 7, "REM 1234507"},
	}
	for _, c := range casos {
		if got := ReferenciaRemesa(c.lote, c.remesa); got != c.quiere {
			t.Errorf("ReferenciaRemesa(%d, %d) = %q, quiere %q", c.lote, c.remesa, got, c.quiere)
		}
	}
}

func TestParaElBancoDejaSoloLoQueSobrevive(t *testing.T) {
	t.Parallel()
	casos := map[string]string{
		"CXP-54A4281AE49A":              "CXP54A4281AE49A",
		"REM 0061-01 CXPR-1760CCFC2435": "REM 006101 CXPR1760CCFC2435",
		// El espacio SÍ sobrevive —la captura del estado de cuenta lo prueba— así que no se toca:
		// pegar las palabras volvería la descripción ilegible.
		"A9 FC 000487": "A9 FC 000487",
		// Cualquier otro signo se va por el mismo motivo que el guion.
		"FC #487 (urgente)": "FC 487 urgente",
		"":                  "",
	}
	for entra, quiere := range casos {
		if got := ParaElBanco(entra); got != quiere {
			t.Errorf("ParaElBanco(%q) = %q, quiere %q", entra, got, quiere)
		}
	}
}

// La llave de búsqueda iguala TODAS las formas en que el mismo pago puede llegar escrito.
//
// Hace falta aunque ya no se emita el guion: los pagos enviados antes del 19-oct salieron con él y
// no se pueden reescribir. Sin esto, buscar un pago viejo por lo que trae el estado de cuenta
// seguiría devolviendo cero.
func TestLaClaveDeBusquedaIgualaLasFormasDelMismoPago(t *testing.T) {
	t.Parallel()
	mismas := []string{
		"REM 0043-07",  // como salía en las macros viejas
		"REM 004307",   // como vuelve del banco, y como sale ahora
		"rem 004307",   // como lo escribe alguien a mano
		"REM-004307",   // como lo deja un copiado torcido
		"  REM 004307", // con espacios de más al pegar
	}
	esperada := ClaveDeBusqueda(mismas[0])
	if esperada == "" {
		t.Fatal("la clave quedó vacía")
	}
	for _, forma := range mismas[1:] {
		if got := ClaveDeBusqueda(forma); got != esperada {
			t.Errorf("ClaveDeBusqueda(%q) = %q, quiere %q — es el mismo pago", forma, got, esperada)
		}
	}

	// Y NO iguala pagos distintos: una llave que colapse de más devolvería la remesa equivocada.
	if ClaveDeBusqueda("REM 004307") == ClaveDeBusqueda("REM 004308") {
		t.Error("dos remesas distintas dan la misma llave")
	}
	if ClaveDeBusqueda("CXP-A1B2C3D4E5F6") == ClaveDeBusqueda("CXPR-A1B2C3D4E5F6") {
		t.Error("una factura y una remesa dan la misma llave")
	}
}
