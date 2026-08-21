package bancos

// Lo que se prueba acá no es que el archivo se lea, sino que NO se escriba nada cuando no se puede
// saber qué escribir: el movimiento que no está cargado, la partida que no existe, el nombre de
// clasificación que vive en dos conceptos, y los movimientos idénticos que nadie puede distinguir.

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// grilla arma una Grid a partir de filas de texto.
func grilla(filas ...[]string) Grid {
	g := make(Grid, 0, len(filas))
	for _, f := range filas {
		g = append(g, f)
	}
	return g
}

func encClasif() []string {
	return []string{"Fecha", "Cuenta", "Documento", "Débito", "Crédito", "Concepto", "Clasificación"}
}

func TestLeerClasifExcelEntiendeLasColumnasYLasFechas(t *testing.T) {
	t.Parallel()
	g := grilla(
		[]string{"Reporte de movimientos", "", "", "", "", "", ""}, // basura antes del encabezado
		encClasif(),
		[]string{"03/07/2026", "BN Valle de Paz Colones", "ABC1", "1 234,56", "", "Gastos", "Combustible"},
		[]string{"2026-07-04", "BN Valle de Paz Colones", "", "", "500", "Ingresos", "Datafonos"},
		[]string{"", "", "", "", "", "", ""}, // fila en blanco: se salta
		[]string{"no es fecha", "BN Valle de Paz Colones", "", "10", "", "Gastos", "Combustible"},
	)
	filas, err := LeerClasifExcel(g)
	if err != nil {
		t.Fatalf("LeerClasifExcel: %v", err)
	}
	if len(filas) != 3 {
		t.Fatalf("filas = %d, se esperaban 3 (la vacía se salta)", len(filas))
	}
	if filas[0].Fecha != "2026-07-03" {
		t.Errorf("fecha dd/mm/aaaa = %s, se esperaba 2026-07-03", filas[0].Fecha)
	}
	if filas[0].Debito != "1234.56" {
		t.Errorf("débito = %s, se esperaba 1234.56 (el separador de miles no debe estorbar)", filas[0].Debito)
	}
	if filas[1].Fecha != "2026-07-04" || filas[1].Credito != "500.00" {
		t.Errorf("fila 2 = %s / %s", filas[1].Fecha, filas[1].Credito)
	}
	if filas[2].Estado != ClasifExcelFilaInvalida {
		t.Errorf("una fecha ilegible debe quedar inválida, quedó %q", filas[2].Estado)
	}
}

func TestLeerClasifExcelExigeEncabezadoReconocible(t *testing.T) {
	t.Parallel()
	// Solo «Fecha» no alcanza: cualquier tabla tiene una columna Fecha y leerla como si fuera esta
	// hoja desalinea todo el archivo en silencio.
	g := grilla([]string{"Fecha", "Monto", "Saldo"}, []string{"03/07/2026", "100", "900"})
	if _, err := LeerClasifExcel(g); err != ErrClasifExcelSinEncabezado {
		t.Fatalf("err = %v, se esperaba ErrClasifExcelSinEncabezado", err)
	}
}

// escenario arma un servicio con una cuenta, un catálogo y los movimientos que la base "tiene".
func escenario(t *testing.T, movs []MovimientoCalzado) (*Service, *fakeRepo) {
	t.Helper()
	repo := &fakeRepo{
		cuentasLista: []CuentaListItem{
			{ID: "cta-1", Alias: "BN Valle de Paz Colones", Banco: "BN", Moneda: "CRC"},
			{ID: "cta-2", Alias: "Davivienda Colones", IBAN: "CR76010409142215626710", Moneda: "CRC"},
		},
		clasifsCat: []ClasificacionItem{
			{ID: "cl-comb", ConceptoID: "co-gas", Concepto: "Gastos", Nombre: "Combustible"},
			{ID: "cl-plan", ConceptoID: "co-gas", Concepto: "Gastos", Nombre: "Planilla"},
			// El mismo nombre en dos conceptos: pasa de verdad (3 casos en Coopeprofa, 1 en Memorial Pets).
			{ID: "cl-com-g", ConceptoID: "co-gas", Concepto: "Gastos", Nombre: "Comisiones"},
			{ID: "cl-com-i", ConceptoID: "co-ing", Concepto: "Ingresos", Nombre: "Comisiones"},
		},
		movsCalzados: movs,
	}
	return NewService(repo, nil, zap.NewNop(), true), repo
}

// fila arma una fila ya parseada, como si viniera del archivo.
func filaClasif(linea int, cuenta, fecha, doc, deb, cred, concepto, clasif string) FilaClasifExcel {
	g := grilla(encClasif(), []string{fecha, cuenta, doc, deb, cred, concepto, clasif})
	filas, err := LeerClasifExcel(g)
	if err != nil {
		panic(err)
	}
	f := filas[0]
	f.Linea = linea
	return f
}

func calzado(clave, id, desc, concepto, clasif, clasifID string) MovimientoCalzado {
	return MovimientoCalzado{
		Clave: clave, ID: id, Descripcion: desc,
		Concepto: concepto, Clasificacion: clasif, ClasifID: clasifID,
		Estado: "NO_IDENTIFICADO",
	}
}

func TestClasifExcelClasificaLoQueEstaSinClasificar(t *testing.T) {
	t.Parallel()
	svc, repo := escenario(t, []MovimientoCalzado{
		calzado("cta-1|2026-07-03|1234.56|0.00|ABC1", "mov-1", "COMPRA GASOLINA", "", "", ""),
	})
	filas := []FilaClasifExcel{
		filaClasif(2, "BN Valle de Paz Colones", "03/07/2026", "ABC1", "1234.56", "", "Gastos", "Combustible"),
	}
	plan, err := svc.planClasifExcel(context.Background(), "emp", filas, "", false, true, "u1")
	if err != nil {
		t.Fatalf("planClasifExcel: %v", err)
	}
	if plan.Clasifica != 1 || plan.Clasificados != 1 {
		t.Fatalf("plan = %+v; se esperaba 1 clasificada y 1 escrita", plan)
	}
	if len(repo.asignados) != 1 || repo.asignados[0].ClasificacionID != "cl-comb" || repo.asignados[0].ConceptoID != "co-gas" {
		t.Fatalf("asignaciones = %+v", repo.asignados)
	}
	// La descripción del movimiento hallado tiene que volver: es la prueba de que calzó con el correcto.
	if plan.Detalle[0].Descripcion != "COMPRA GASOLINA" {
		t.Errorf("descripción = %q, se esperaba la del movimiento hallado", plan.Detalle[0].Descripcion)
	}
}

func TestClasifExcelNoPisaLoYaClasificadoSalvoQueSePida(t *testing.T) {
	t.Parallel()
	movs := []MovimientoCalzado{
		calzado("cta-1|2026-07-03|1234.56|0.00|ABC1", "mov-1", "COMPRA GASOLINA", "Gastos", "Planilla", "cl-plan"),
	}
	filas := []FilaClasifExcel{
		filaClasif(2, "BN Valle de Paz Colones", "03/07/2026", "ABC1", "1234.56", "", "Gastos", "Combustible"),
	}

	// Por defecto: no se toca, y se dice qué tenía.
	svc, repo := escenario(t, movs)
	plan, err := svc.planClasifExcel(context.Background(), "emp", filas, "", false, true, "u1")
	if err != nil {
		t.Fatalf("planClasifExcel: %v", err)
	}
	if plan.Protegidas != 1 || plan.Clasificados != 0 || len(repo.asignados) != 0 {
		t.Fatalf("sin pedir reemplazo no se debe escribir nada; plan = %+v", plan)
	}
	if !strings.Contains(plan.Detalle[0].PartidaActual, "Planilla") {
		t.Errorf("partida actual = %q, se esperaba la que tenía", plan.Detalle[0].PartidaActual)
	}

	// Pidiéndolo explícitamente: sí se reemplaza.
	svc2, repo2 := escenario(t, movs)
	plan2, err := svc2.planClasifExcel(context.Background(), "emp", filas, "", true, true, "u1")
	if err != nil {
		t.Fatalf("planClasifExcel con reemplazo: %v", err)
	}
	if plan2.Reclasifica != 1 || len(repo2.asignados) != 1 {
		t.Fatalf("con reemplazo se esperaba 1 reclasificada; plan = %+v", plan2)
	}
}

func TestClasifExcelNoInventaCuandoNoPuedeSaber(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre  string
		movs    []MovimientoCalzado
		filas   []FilaClasifExcel
		estado  string
		enTexto string
	}{
		{
			nombre: "el movimiento no está cargado",
			movs:   nil,
			filas: []FilaClasifExcel{
				filaClasif(2, "BN Valle de Paz Colones", "03/07/2026", "ABC1", "100", "", "Gastos", "Combustible"),
			},
			estado:  ClasifExcelSinMovim,
			enTexto: "importá el estado de cuenta",
		},
		{
			nombre: "la partida no existe en el catálogo",
			movs: []MovimientoCalzado{
				calzado("cta-1|2026-07-03|100.00|0.00|ABC1", "mov-1", "X", "", "", ""),
			},
			filas: []FilaClasifExcel{
				filaClasif(2, "BN Valle de Paz Colones", "03/07/2026", "ABC1", "100", "", "Gastos", "Pólvora"),
			},
			estado:  ClasifExcelSinPartida,
			enTexto: "no existe",
		},
		{
			nombre: "el nombre de la clasificación vive en dos conceptos y falta el concepto",
			movs: []MovimientoCalzado{
				calzado("cta-1|2026-07-03|100.00|0.00|ABC1", "mov-1", "X", "", "", ""),
			},
			filas: []FilaClasifExcel{
				filaClasif(2, "BN Valle de Paz Colones", "03/07/2026", "ABC1", "100", "", "", "Comisiones"),
			},
			estado:  ClasifExcelSinPartida,
			enTexto: "agregá la columna Concepto",
		},
		{
			nombre: "la cuenta del archivo no existe",
			movs:   nil,
			filas: []FilaClasifExcel{
				filaClasif(2, "Banco Que No Existe", "03/07/2026", "ABC1", "100", "", "Gastos", "Combustible"),
			},
			estado:  ClasifExcelSinCuenta,
			enTexto: "no hay una cuenta",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			svc, repo := escenario(t, c.movs)
			plan, err := svc.planClasifExcel(context.Background(), "emp", c.filas, "cta-1", false, true, "u1")
			if err != nil {
				t.Fatalf("planClasifExcel: %v", err)
			}
			if len(repo.asignados) != 0 {
				t.Fatalf("no se debe escribir nada; se escribieron %d", len(repo.asignados))
			}
			if plan.Detalle[0].Estado != c.estado {
				t.Fatalf("estado = %q, se esperaba %q (detalle: %q)", plan.Detalle[0].Estado, c.estado, plan.Detalle[0].Detalle)
			}
			if !strings.Contains(plan.Detalle[0].Detalle, c.enTexto) {
				t.Errorf("el detalle debería decir %q y dice %q", c.enTexto, plan.Detalle[0].Detalle)
			}
			if plan.Aviso == "" {
				t.Error("con filas que no se pudieron resolver, el plan debe avisarlo arriba")
			}
		})
	}
}

func TestClasifExcelMovimientosIdenticosConPartidasDistintasQuedanAmbiguos(t *testing.T) {
	t.Parallel()
	// Dos movimientos exactamente iguales (los duplicados legítimos que el importador conserva) y dos
	// filas del archivo que les dan partidas DISTINTAS: nadie puede saber cuál es cuál, y elegir al
	// azar dejaría la plata en la partida equivocada.
	clave := "cta-1|2026-07-03|50.00|0.00|DUP"
	movs := []MovimientoCalzado{
		calzado(clave, "mov-1", "PAGO", "", "", ""),
		calzado(clave, "mov-2", "PAGO", "", "", ""),
	}
	filas := []FilaClasifExcel{
		filaClasif(2, "BN Valle de Paz Colones", "03/07/2026", "DUP", "50", "", "Gastos", "Combustible"),
		filaClasif(3, "BN Valle de Paz Colones", "03/07/2026", "DUP", "50", "", "Gastos", "Planilla"),
	}
	svc, repo := escenario(t, movs)
	plan, err := svc.planClasifExcel(context.Background(), "emp", filas, "", false, true, "u1")
	if err != nil {
		t.Fatalf("planClasifExcel: %v", err)
	}
	if plan.Ambiguas != 2 || len(repo.asignados) != 0 {
		t.Fatalf("se esperaban 2 ambiguas y 0 escrituras; plan = %+v, escritas = %d", plan, len(repo.asignados))
	}

	// Con la MISMA partida en las dos filas sí se puede: el orden deja de importar.
	filasIguales := []FilaClasifExcel{
		filaClasif(2, "BN Valle de Paz Colones", "03/07/2026", "DUP", "50", "", "Gastos", "Combustible"),
		filaClasif(3, "BN Valle de Paz Colones", "03/07/2026", "DUP", "50", "", "Gastos", "Combustible"),
	}
	svc2, repo2 := escenario(t, movs)
	plan2, err := svc2.planClasifExcel(context.Background(), "emp", filasIguales, "", false, true, "u1")
	if err != nil {
		t.Fatalf("planClasifExcel (misma partida): %v", err)
	}
	if plan2.Clasifica != 2 || len(repo2.asignados) != 2 {
		t.Fatalf("con la misma partida se esperaban 2 clasificadas; plan = %+v", plan2)
	}
}

func TestClasifExcelPrevisualizarNoEscribe(t *testing.T) {
	t.Parallel()
	svc, repo := escenario(t, []MovimientoCalzado{
		calzado("cta-1|2026-07-03|100.00|0.00|ABC1", "mov-1", "X", "", "", ""),
	})
	filas := []FilaClasifExcel{
		filaClasif(2, "BN Valle de Paz Colones", "03/07/2026", "ABC1", "100", "", "Gastos", "Combustible"),
	}
	plan, err := svc.planClasifExcel(context.Background(), "emp", filas, "", false, false, "u1")
	if err != nil {
		t.Fatalf("planClasifExcel: %v", err)
	}
	if plan.Aplicado {
		t.Error("previsualizar no puede reportarse como aplicado")
	}
	if plan.Clasifica != 1 {
		t.Errorf("el plan debe decir que clasificaría 1; dijo %d", plan.Clasifica)
	}
	if plan.Clasificados != 0 || len(repo.asignados) != 0 {
		t.Fatalf("previsualizar no debe escribir nada; escribió %d", len(repo.asignados))
	}
}

func TestClasifExcelResuelveLaCuentaPorIBAN(t *testing.T) {
	t.Parallel()
	// El archivo puede traer el IBAN con espacios, como lo pega la gente desde el banco.
	svc, repo := escenario(t, []MovimientoCalzado{
		calzado("cta-2|2026-07-03|100.00|0.00|ABC1", "mov-9", "X", "", "", ""),
	})
	filas := []FilaClasifExcel{
		filaClasif(2, "CR76 0104 0914 2215 6267 10", "03/07/2026", "ABC1", "100", "", "Gastos", "Combustible"),
	}
	plan, err := svc.planClasifExcel(context.Background(), "emp", filas, "", false, true, "u1")
	if err != nil {
		t.Fatalf("planClasifExcel: %v", err)
	}
	if plan.Clasifica != 1 || len(repo.asignados) != 1 || repo.asignados[0].MovimientoID != "mov-9" {
		t.Fatalf("el IBAN con espacios debe resolver la cuenta; plan = %+v, asignados = %+v", plan, repo.asignados)
	}
}

func TestDetalleClasifExcelPonePrimeroLosProblemas(t *testing.T) {
	t.Parallel()
	// Un archivo grande: la tabla se recorta, y lo que sobrevive tiene que ser lo que hay que
	// arreglar. Mandar las primeras 400 filas correctas esconderia justo lo que falló.
	filas := make([]FilaClasifExcel, 0, maxDetalleClasifExcel+10)
	for i := 0; i < maxDetalleClasifExcel+5; i++ {
		filas = append(filas, FilaClasifExcel{Linea: i + 2, Estado: ClasifExcelSinCambio})
	}
	filas = append(filas, FilaClasifExcel{Linea: 9001, Estado: ClasifExcelSinPartida})
	filas = append(filas, FilaClasifExcel{Linea: 9002, Estado: ClasifExcelAmbiguo})

	detalle, truncado := detalleClasifExcel(filas)
	if !truncado {
		t.Fatal("con más filas que el tope, la respuesta debe declarar que se recortó")
	}
	if len(detalle) != maxDetalleClasifExcel {
		t.Fatalf("detalle = %d filas, se esperaba el tope %d", len(detalle), maxDetalleClasifExcel)
	}
	if detalle[0].Linea != 9001 || detalle[1].Linea != 9002 {
		t.Errorf("los problemas deben ir primero; llegaron las líneas %d y %d", detalle[0].Linea, detalle[1].Linea)
	}
}

func TestParseMontoToleranteEntiendeLosDosFormatos(t *testing.T) {
	t.Parallel()
	casos := []struct{ texto, quiero string }{
		{"1234.56", "1234.56"},         // plano
		{"1,234.56", "1234.56"},        // inglés (como exportan los bancos)
		{"1.234,56", "1234.56"},        // español
		{"1 234,56", "1234.56"},        // español con separador de miles en espacio
		{"₡1 234,56", "1234.56"},       // con símbolo
		{"CRC 1,234.56", "1234.56"},    // con prefijo de moneda
		{"1,234", "1234"},              // tres cifras después del separador = miles
		{"12,34", "12.34"},             // dos cifras = decimal
		{"1.234.567,89", "1234567.89"}, // dos separadores de miles
		{"", "0"},                      // celda vacía
		{"-", "0"},                     // el guion que usan algunos bancos para el cero
		{"0.00", "0"},                  //
	}
	for _, c := range casos {
		got, err := parseMontoTolerante(c.texto)
		if err != nil {
			t.Errorf("%q: %v", c.texto, err)
			continue
		}
		if got.String() != c.quiero {
			t.Errorf("%q → %s, se esperaba %s", c.texto, got.String(), c.quiero)
		}
	}
	if _, err := parseMontoTolerante("mil pesos"); err == nil {
		t.Error("un texto que no es número debe fallar, no valer cero")
	}
}

func TestLeerClasifExcelNoLlamaIlegibleAUnaFilaEnBlanco(t *testing.T) {
	t.Parallel()
	// El flujo normal: se bajan 5.000 movimientos y se llenan unos pocos. Las que quedan en blanco NO
	// son un error del archivo, y contarlas como «no se pudieron leer» acusa al usuario de algo falso.
	filas := [][]string{encClasif()}
	for i := 0; i < 20; i++ {
		filas = append(filas, []string{"03/07/2026", "BN Valle de Paz Colones", "D" + itoa(i), "100", "", "", ""})
	}
	filas = append(filas, []string{"03/07/2026", "BN Valle de Paz Colones", "LLENA", "100", "", "Gastos", "Combustible"})
	leidas, err := LeerClasifExcel(grilla(filas...))
	if err != nil {
		t.Fatalf("LeerClasifExcel: %v", err)
	}
	enBlanco, invalidas := 0, 0
	for _, f := range leidas {
		switch f.Estado {
		case ClasifExcelSinLlenar:
			enBlanco++
		case ClasifExcelFilaInvalida:
			invalidas++
		}
	}
	if enBlanco != 20 {
		t.Errorf("filas en blanco = %d, se esperaban 20", enBlanco)
	}
	if invalidas != 0 {
		t.Errorf("una fila en blanco NO es ilegible; se contaron %d ilegibles", invalidas)
	}

	// Y con el concepto puesto pero la clasificación vacía sí es un error, porque la partida está a
	// medias: eso no se puede asignar y no es «reposo».
	aMedias, err := LeerClasifExcel(grilla(encClasif(),
		[]string{"03/07/2026", "BN Valle de Paz Colones", "X", "100", "", "Gastos", ""}))
	if err != nil {
		t.Fatalf("LeerClasifExcel a medias: %v", err)
	}
	if aMedias[0].Estado != ClasifExcelFilaInvalida {
		t.Errorf("con concepto y sin clasificación se esperaba inválida, quedó %q", aMedias[0].Estado)
	}
}

func TestDetalleClasifExcelMandaLasEnBlancoAlFinal(t *testing.T) {
	t.Parallel()
	// Con un archivo grande, la tabla se recorta. Las filas en blanco son mayoría y no aportan nada:
	// si van primero, expulsan los pocos problemas que hay que ver. Es el defecto que la revisión
	// adversarial reprodujo con 4.940 filas en blanco copando las 400 del detalle.
	filas := make([]FilaClasifExcel, 0, maxDetalleClasifExcel+10)
	for i := 0; i < maxDetalleClasifExcel+5; i++ {
		filas = append(filas, FilaClasifExcel{Linea: i + 2, Estado: ClasifExcelSinLlenar})
	}
	filas = append(filas,
		FilaClasifExcel{Linea: 9001, Estado: ClasifExcelSinPartida},
		FilaClasifExcel{Linea: 9002, Estado: ClasifExcelAmbiguo},
		FilaClasifExcel{Linea: 9003, Estado: ClasifExcelClasifica},
	)
	detalle, truncado := detalleClasifExcel(filas)
	if !truncado {
		t.Fatal("se esperaba que declarara el recorte")
	}
	if detalle[0].Linea != 9001 || detalle[1].Linea != 9002 {
		t.Fatalf("los problemas deben ir primero; llegaron %d y %d", detalle[0].Linea, detalle[1].Linea)
	}
	// La fila que SÍ se va a clasificar también tiene que sobrevivir al recorte.
	var hayClasifica bool
	for _, f := range detalle {
		if f.Estado == ClasifExcelClasifica {
			hayClasifica = true
		}
	}
	if !hayClasifica {
		t.Error("la fila que se va a clasificar no debería quedar fuera del detalle")
	}
}

func TestClasifExcelAvisaCuandoElLibroTieneVariasHojas(t *testing.T) {
	t.Parallel()
	// Se lee UNA hoja. Callarlo esconde trabajo: alguien pone una hoja por cuenta y el resumen se ve
	// igual de exitoso habiendo leído solo la primera.
	p := PlanClasifExcel{Hoja: "Movimientos", Hojas: []string{"Resumen", "Movimientos", "BAC"}}
	aviso := avisoClasifExcel(p)
	if !strings.Contains(aviso, "3 hojas") || !strings.Contains(aviso, "Movimientos") ||
		!strings.Contains(aviso, "Resumen") || !strings.Contains(aviso, "BAC") {
		t.Errorf("el aviso debe nombrar la hoja leída y las que quedaron afuera; dice %q", aviso)
	}
	if avisoClasifExcel(PlanClasifExcel{Hoja: "Movimientos", Hojas: []string{"Movimientos"}}) != "" {
		t.Error("con una sola hoja no hay nada que advertir")
	}
}

func TestClasifExcelCuentaAmbiguaNoEscribeNada(t *testing.T) {
	t.Parallel()
	// La base garantiza alias único por empresa, pero acá se compara sin tildes ni mayúsculas y eso
	// puede empatar dos alias que la base considera distintos.
	repo := &fakeRepo{
		cuentasLista: []CuentaListItem{
			{ID: "cta-1", Alias: "BAC Religiosa", Banco: "BAC", Moneda: "CRC"},
			{ID: "cta-2", Alias: "bac religiosa", Banco: "BAC", Moneda: "CRC"},
		},
		clasifsCat: []ClasificacionItem{
			{ID: "cl-comb", ConceptoID: "co-gas", Concepto: "Gastos", Nombre: "Combustible"},
		},
	}
	svc := NewService(repo, nil, zap.NewNop(), true)
	filas := []FilaClasifExcel{
		filaClasif(2, "BAC Religiosa", "03/07/2026", "ABC1", "100", "", "Gastos", "Combustible"),
	}
	plan, err := svc.planClasifExcel(context.Background(), "emp", filas, "", false, true, "u1")
	if err != nil {
		t.Fatalf("planClasifExcel: %v", err)
	}
	if plan.SinCuenta != 1 || len(repo.asignados) != 0 {
		t.Fatalf("con dos cuentas del mismo nombre no se debe elegir una; plan = %+v", plan)
	}
	if !strings.Contains(plan.Detalle[0].Detalle, "más de una cuenta") {
		t.Errorf("el detalle debe explicar la ambigüedad; dice %q", plan.Detalle[0].Detalle)
	}
}
