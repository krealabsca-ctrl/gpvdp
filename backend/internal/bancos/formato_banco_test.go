package bancos

// Formatos de banco configurables (22-oct-2026, mig 0108).
//
// EL PEDIDO (punto 2 de la revisión comercial): los seis formatos viven en Go, así que cada banco
// nuevo es un ticket de desarrollo, un build y un despliegue. Es el techo de cuántos clientes se
// pueden atender sin crecer el equipo.
//
// LA RESTRICCIÓN DEL DIRECTOR FINANCIERO: «nada de lo que hagamos puede romper lo que funciona».
// Por eso la primera prueba de este archivo no es la funcionalidad nueva: es que los seis de
// siempre se sigan detectando y leyendo exactamente igual. Si esa falla, lo demás no importa.

import (
	"testing"
)

// gridLafise es un banco que NINGUNO de los seis adaptadores reconoce.
//
// Está armado a propósito con las columnas en otro orden —descripción antes que documento, y el
// crédito antes que el débito— para que una lectura con las columnas corridas se note: si el
// formato configurado leyera mal, el débito y el crédito saldrían cambiados.
var gridLafise = Grid{
	{"BANCO LAFISE COSTA RICA"},
	{"Estado de cuenta", "Cuenta", "CR12345678901234567890"},
	{},
	{"Fecha mov.", "Concepto", "Comprobante", "Haber", "Debe", "Saldo"},
	{"2026-10-01", "DEPOSITO EN EFECTIVO", "991001", "150,000.00", "", "150,000.00"},
	{"2026-10-02", "PAGO PROVEEDOR XYZ", "991002", "", "40,000.00", "110,000.00"},
	{"", "SALDO FINAL", "", "", "", "110,000.00"},
}

// formatoLafise es cómo se configuraría ese banco desde la pantalla.
func formatoLafise() FormatoBanco {
	return FormatoBanco{
		Nombre:             "Lafise",
		Activo:             true,
		TextoIdentificador: "banco lafise",
		ColFecha:           0,
		ColDescripcion:     1,
		ColDocumento:       2,
		// Haber = crédito, Debe = débito. Invertidas respecto del orden habitual a propósito.
		ColCredito:   3,
		ColDebito:    4,
		FormatoFecha: FechaISO,
		Senales: []SenalEncabezado{
			{Columna: 0, Texto: "fecha mov"},
			{Columna: 4, Texto: "debe"},
		},
	}
}

// LO PRIMERO: con la tabla de formatos vacía, el importador se comporta EXACTAMENTE como antes.
//
// Esta es la prueba que sostiene la restricción. Recorre las seis grillas reales y comprueba que
// `DetectarCon(g, nil)` elige el mismo adaptador que `Detectar(g)` elegía.
func TestSinFormatosConfiguradosLosSeisDeSiempreSeDetectanIgual(t *testing.T) {
	casos := []struct {
		nombre string
		grid   Grid
		quiere Banco
	}{
		{"Promerica", gridPromerica, BancoPromerica},
		{"BN", gridBN, BancoBN},
		{"BAC", gridBAC, BancoBAC},
		{"BCR", gridBCR, BancoBCR},
		{"Banco Popular", gridBP, BancoBP},
		{"Davivienda", gridDavivienda, BancoDavivienda},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			a, err := DetectarCon(c.grid, nil)
			if err != nil {
				t.Fatalf("ya no se reconoce: %v", err)
			}
			if a.Banco() != c.quiere {
				t.Errorf("detectó %q, se esperaba %q", a.Banco(), c.quiere)
			}
			// Y lee lo mismo: detectar el banco correcto y parsear mal sería peor que no detectarlo.
			res, err := a.Parsea(c.grid)
			if err != nil {
				t.Fatalf("ya no se parsea: %v", err)
			}
			if len(res.Movimientos) == 0 {
				t.Error("no salió ningún movimiento")
			}
		})
	}
}

// Y con formatos configurados TAMPOCO cambian: los suyos no calzan con estos archivos, así que
// cada uno sigue cayendo en su adaptador de siempre.
func TestUnFormatoConfiguradoNoLeRobaLosArchivosALosDeSiempre(t *testing.T) {
	extra, _ := AdaptadoresDe([]FormatoBanco{formatoLafise()})
	if len(extra) != 1 {
		t.Fatalf("el formato no se construyó (%d adaptadores)", len(extra))
	}
	casos := []struct {
		nombre string
		grid   Grid
		quiere Banco
	}{
		{"Promerica", gridPromerica, BancoPromerica},
		{"BN", gridBN, BancoBN},
		{"BAC", gridBAC, BancoBAC},
		{"BCR", gridBCR, BancoBCR},
		{"Banco Popular", gridBP, BancoBP},
		{"Davivienda", gridDavivienda, BancoDavivienda},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			a, err := DetectarCon(c.grid, extra)
			if err != nil {
				t.Fatalf("no se reconoció: %v", err)
			}
			if a.Banco() != c.quiere {
				t.Errorf("el formato configurado le robó el archivo a %s (detectó %q)", c.quiere, a.Banco())
			}
		})
	}
}

// LO QUE EL PUNTO 2 COMPRA: un banco que hoy no se puede importar, se importa sin tocar código.
func TestUnBancoNuevoSeLeeSinTocarCodigo(t *testing.T) {
	// Primero, la constancia de que hoy NO se puede: ningún adaptador del código lo reconoce.
	if _, err := Detectar(gridLafise); err == nil {
		t.Fatal("algún adaptador del código ya reconoce este archivo; la prueba no demuestra nada")
	}

	extra, rotos := AdaptadoresDe([]FormatoBanco{formatoLafise()})
	if rotos != 0 {
		t.Fatalf("el formato no se pudo construir (%d rotos)", rotos)
	}
	a, err := DetectarCon(gridLafise, extra)
	if err != nil {
		t.Fatalf("con el formato configurado sigue sin reconocerse: %v", err)
	}
	if a.Banco() != "Lafise" {
		t.Errorf("banco = %q, se esperaba Lafise", a.Banco())
	}

	res, err := a.Parsea(gridLafise)
	if err != nil {
		t.Fatalf("parsear: %v", err)
	}
	if len(res.Movimientos) != 2 {
		t.Fatalf("movimientos = %d, se esperaban 2 (la fila de saldo final se descarta sola)", len(res.Movimientos))
	}
	// EL DEPÓSITO ES CRÉDITO Y EL PAGO ES DÉBITO. Con las columnas corridas una sola posición,
	// este archivo se leería al revés y el cuadre del mes cerraría con el signo cambiado.
	dep := res.Movimientos[0]
	if dep.Credito.String() != "150000" || !dep.Debito.IsZero() {
		t.Errorf("el depósito quedó como débito %s / crédito %s", dep.Debito, dep.Credito)
	}
	if dep.Descripcion != "DEPOSITO EN EFECTIVO" || dep.Documento != "991001" {
		t.Errorf("descripción %q, documento %q: las columnas no son las configuradas", dep.Descripcion, dep.Documento)
	}
	pago := res.Movimientos[1]
	if pago.Debito.String() != "40000" || !pago.Credito.IsZero() {
		t.Errorf("el pago quedó como débito %s / crédito %s", pago.Debito, pago.Credito)
	}
	// El IBAN se extrae igual que en los seis del código: es el mismo `Parsea`.
	if res.IBAN != "CR12345678901234567890" {
		t.Errorf("IBAN = %q", res.IBAN)
	}
}

// El caso que justifica que los configurados vayan PRIMERO: un banco cambia su exportación y hay
// que poder ponerle un formato nuevo delante sin esperar un despliegue.
func TestUnFormatoConfiguradoPuedeTomarElLugarDeUnoDelCodigo(t *testing.T) {
	propio := FormatoBanco{
		Nombre: "BN corregido", Activo: true,
		ColFecha: 1, ColDocumento: 2, ColDescripcion: 5, ColDebito: 3, ColCredito: 4,
		FormatoFecha: FechaMMDDAA,
		Senales: []SenalEncabezado{
			{Columna: 0, Texto: "oficina"},
			{Columna: 1, Texto: "fechamovimiento"},
		},
	}
	extra, _ := AdaptadoresDe([]FormatoBanco{propio})
	a, err := DetectarCon(gridBN, extra)
	if err != nil {
		t.Fatalf("detectar: %v", err)
	}
	if a.Banco() != "BN corregido" {
		t.Errorf("ganó %q: un formato configurado tiene que poder reemplazar a uno del código, "+
			"o no sirve para arreglar un banco que cambió su export", a.Banco())
	}
}

// LA VALIDACIÓN QUE IMPIDE EL FORMATO COMODÍN.
//
// Como los configurados se prueban ANTES, uno con una sola señal genérica —«la columna 0 dice
// fecha», que lo cumplen casi todos— le robaría los archivos a los seis que hoy funcionan.
func TestUnFormatoDemasiadoGeneralNoSeGuarda(t *testing.T) {
	base := func() FormatoBanco {
		return FormatoBanco{
			Nombre: "Comodín", Activo: true, FormatoFecha: FechaISO,
			ColFecha: 0, ColDocumento: 1, ColDescripcion: 2, ColDebito: 3, ColCredito: 4,
		}
	}

	t.Run("una sola señal y sin texto identificador se rechaza", func(t *testing.T) {
		f := base()
		f.Senales = []SenalEncabezado{{Columna: 0, Texto: "fecha"}}
		if err := f.Validar(); err != ErrFormatoDebilParaDetectar {
			t.Errorf("se aceptó un formato que calza con cualquier archivo: %v", err)
		}
	})
	t.Run("sin ninguna señal se rechaza", func(t *testing.T) {
		f := base()
		if err := f.Validar(); err != ErrFormatoDebilParaDetectar {
			t.Errorf("se aceptó un formato sin señales: %v", err)
		}
	})
	t.Run("una señal MÁS el texto identificador alcanza", func(t *testing.T) {
		f := base()
		f.TextoIdentificador = "banco lafise"
		f.Senales = []SenalEncabezado{{Columna: 0, Texto: "fecha"}}
		if err := f.Validar(); err != nil {
			t.Errorf("se rechazó un formato específico: %v", err)
		}
	})
	t.Run("dos señales alcanzan", func(t *testing.T) {
		f := base()
		f.Senales = []SenalEncabezado{{Columna: 0, Texto: "fecha"}, {Columna: 3, Texto: "debito"}}
		if err := f.Validar(); err != nil {
			t.Errorf("se rechazó un formato con dos señales: %v", err)
		}
	})
}

// Lo demás de la validación: lo que falta se dice con nombre propio, porque quien configura está
// contando columnas a mano y un «datos inválidos» lo deja probando a ciegas.
func TestLoQueFaltaEnUnFormatoSeDiceConNombrePropio(t *testing.T) {
	ok := formatoLafise()

	casos := []struct {
		nombre string
		armar  func(FormatoBanco) FormatoBanco
		quiere error
	}{
		{"sin nombre", func(f FormatoBanco) FormatoBanco { f.Nombre = "  "; return f }, ErrFormatoSinNombre},
		{"fecha de otra lista", func(f FormatoBanco) FormatoBanco { f.FormatoFecha = "DD.MM.YY"; return f }, ErrFormatoFechaDesconocida},
		{"sin columna de débito", func(f FormatoBanco) FormatoBanco { f.ColDebito = -1; return f }, ErrFormatoColumnaInvalida},
		{"señal sin texto", func(f FormatoBanco) FormatoBanco {
			f.Senales = []SenalEncabezado{{Columna: 0, Texto: "fecha"}, {Columna: 1, Texto: "  "}}
			return f
		}, ErrFormatoSenalVacia},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := c.armar(ok)
			if err := f.Validar(); err != c.quiere {
				t.Errorf("devolvió %v, se esperaba %v", err, c.quiere)
			}
		})
	}

	// El documento SÍ admite -1: hay bancos que no publican ninguna referencia.
	t.Run("sin columna de documento se acepta", func(t *testing.T) {
		f := ok
		f.ColDocumento = -1
		if err := f.Validar(); err != nil {
			t.Errorf("se rechazó un banco sin referencia: %v", err)
		}
	})
}

// Un formato roto no puede impedir que alguien cargue su estado de cuenta: se salta y los seis de
// siempre siguen ahí.
func TestUnFormatoRotoSeSaltaYNoTumbaLaImportacion(t *testing.T) {
	roto := formatoLafise()
	roto.FormatoFecha = "ESTO NO EXISTE"
	apagado := formatoLafise()
	apagado.Nombre = "Apagado"
	apagado.Activo = false

	extra, rotos := AdaptadoresDe([]FormatoBanco{roto, apagado, formatoLafise()})
	if rotos != 1 {
		t.Errorf("rotos = %d, se esperaba 1", rotos)
	}
	if len(extra) != 1 {
		t.Fatalf("adaptadores = %d, se esperaba 1 (el roto se salta, el apagado no se usa)", len(extra))
	}
	// Y lo demás sigue funcionando con el formato roto en la lista.
	if _, err := DetectarCon(gridPromerica, extra); err != nil {
		t.Errorf("un formato roto tumbó la detección de Promerica: %v", err)
	}
}

// Los seis formatos de fecha de la lista se pueden traducir. Si uno no, la pantalla ofrecería una
// opción que el servidor no sabe leer y el formato se guardaría para fallar después.
func TestTodosLosFormatosDeFechaOfrecidosSeTraducen(t *testing.T) {
	for _, f := range FormatosDeFecha {
		parser, ok := parserDeFecha(f.Clave)
		if !ok {
			t.Errorf("%s se ofrece en la pantalla y no se puede traducir", f.Clave)
			continue
		}
		// Y el ejemplo que la pantalla muestra tiene que parsear con ese parser: si no, le está
		// enseñando a la gente a escribir una fecha que el sistema no entiende.
		if _, err := parser(f.Ejemplo); err != nil {
			t.Errorf("%s: el ejemplo %q no parsea con su propio parser: %v", f.Clave, f.Ejemplo, err)
		}
	}
}
