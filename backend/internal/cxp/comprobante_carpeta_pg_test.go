package cxp

// La carpeta de comprobantes contra PostgreSQL de verdad (7-oct-2026).
//
// «Por procesos nosotros almacenamos los pagos en una carpeta… para identificar cada pago es lento,
// hay que buscar uno a uno y validar si es el correcto. Si tengo 100 pagos y 100 recibos, ¿cómo
// automatizo?» El criterio del cruce se prueba sin base (es puro, ver comprobante_carpeta_test.go);
// lo que se prueba acá es el SQL, que es lo que no se puede probar detrás de un doble: `MIN(uuid)`
// —que en PostgreSQL no existe— pasó catorce suites y reventó con un 500 contra la base.

import (
	"context"
	"testing"
)

const (
	provCarpeta   = "91919191-9191-4191-9191-919191919191"
	loteCarpeta   = "92929292-9292-4292-9292-929292929292"
	remCarpeta    = "93939393-9393-4393-9393-939393939393"
	otroLote      = "94949494-9494-4494-9494-949494949494"
	huellaRemCarp = "CXPR-AAAA1111BBBB"
	huellaSuelta  = "CXP-5555CCCC6666"
	huellaOtroLot = "CXP-7777DDDD8888"
	// La que existe pero todavía no está pagada: el recibo no puede pegarse ahí.
	huellaSinPagar = "CXP-9999EEEE0000"
	// La misma huella, en OTRA empresa. Nunca puede aparecer.
	huellaAjena = "CXP-5555CCCC6666"
)

// carpetaSembrada arma una corrida realista: una remesa de tres facturas, una factura suelta, una
// programada (sin pagar), una de otro lote y una homónima de otra empresa.
func carpetaSembrada(t *testing.T) *pgRepository {
	t.Helper()
	repo := baseDeLotes(t)
	ctx := context.Background()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := repo.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	exec(`INSERT INTO proveedor (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'DISTRIBUIDORA X')`,
		provCarpeta, empresaLote)
	exec(`INSERT INTO lote_pago (id, empresa_id, numero, fecha_corte, modo_pago)
	      VALUES ($1::uuid, $2::uuid, 43, '2026-10-05'::date, 'PROVEEDOR')`, loteCarpeta, empresaLote)
	exec(`INSERT INTO lote_pago (id, empresa_id, numero, fecha_corte, modo_pago)
	      VALUES ($1::uuid, $2::uuid, 44, '2026-10-06'::date, 'FACTURA')`, otroLote, empresaLote)
	exec(`INSERT INTO remesa_pago (id, empresa_id, lote_id, proveedor_id, numero, huella, monto_crc)
	      VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 7, $5, 300000)`,
		remCarpeta, empresaLote, loteCarpeta, provCarpeta, huellaRemCarp)

	doc := func(consec, estado, huella string, lote, remesa any, total int) {
		t.Helper()
		exec(`INSERT INTO documento_cxp
			  (empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
			   total, total_crc, lote_id, remesa_id, huella)
		      VALUES ($1::uuid, $2::uuid, $3, $3, '2026-09-15'::date, $4, 'CRC',
		              $7, $7, $5::uuid, $6::uuid, $8)`,
			empresaLote, provCarpeta, consec, estado, lote, remesa, total, huella)
	}
	// Las tres de la remesa: comparten el código del pago agrupado y cada una el suyo propio.
	doc("00100001010000000001", "PAGADO", "CXP-R00000000001", loteCarpeta, remCarpeta, 100000)
	doc("00100001010000000002", "PAGADO", "CXP-R00000000002", loteCarpeta, remCarpeta, 100000)
	doc("00100001010000000003", "CONCILIADO", "CXP-R00000000003", loteCarpeta, remCarpeta, 100000)
	// Suelta y pagada, en el mismo lote.
	doc("00100001010000000487", "PAGADO", huellaSuelta, loteCarpeta, nil, 50000)
	// Programada: existe, no admite recibo todavía.
	doc("00100001010000000999", "PROGRAMADO", huellaSinPagar, loteCarpeta, nil, 25000)
	// Otro lote, para probar el filtro.
	doc("00100001010000000777", "PAGADO", huellaOtroLot, otroLote, nil, 10000)

	// LA TRAMPA: otra empresa, misma huella, también pagada.
	exec(`INSERT INTO proveedor (id, empresa_id, nombre)
	      VALUES ('95959595-9595-4595-9595-959595959595'::uuid, $1::uuid, 'AJENA')`, otraEmpLote)
	exec(`INSERT INTO documento_cxp
		  (empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
		   total, total_crc, huella)
	      VALUES ($1::uuid, '95959595-9595-4595-9595-959595959595'::uuid, 'AJENA-1', 'AJENA-1',
	              '2026-09-15'::date, 'PAGADO', 'CRC', 999999, 999999, $2)`, otraEmpLote, huellaAjena)
	return repo
}

func TestDestinosPorHuella(t *testing.T) {
	repo := carpetaSembrada(t)
	ctx := context.Background()

	idx, err := repo.DestinosPorHuella(ctx, empresaLote,
		[]string{huellaRemCarp, huellaSuelta, huellaSinPagar, "CXP-NOEXISTE0000"})
	if err != nil {
		t.Fatalf("DestinosPorHuella: %v", err)
	}

	// La remesa trae sus TRES facturas, no una. Es lo que hace que un recibo cubra el pago entero.
	rem := idx[huellaRemCarp]
	if !rem.EsRemesa {
		t.Error("el código de remesa no vino marcado como remesa; el recibo se pegaría a una sola factura")
	}
	if len(rem.Pagadas) != 3 {
		t.Errorf("la remesa trajo %d facturas, quería 3", len(rem.Pagadas))
	}

	// CONCILIADO también admite recibo: una factura ya cruzada contra el banco sigue necesitando
	// su comprobante. Si solo se aceptara PAGADO, las conciliadas quedarían fuera para siempre.
	hayConciliada := false
	for _, d := range rem.Pagadas {
		if d.Consecutivo == "00100001010000000003" {
			hayConciliada = true
		}
	}
	if !hayConciliada {
		t.Error("la factura CONCILIADA quedó fuera; una factura ya cruzada contra el banco sigue necesitando su recibo")
	}

	if n := len(idx[huellaSuelta].Pagadas); n != 1 {
		t.Errorf("la factura suelta trajo %d destinos, quería 1", n)
	}
	if idx[huellaSuelta].EsRemesa {
		t.Error("una factura suelta vino marcada como remesa")
	}

	// «No existe» y «no está pagada» tienen que llegar distintos a la pantalla.
	sp := idx[huellaSinPagar]
	if len(sp.Pagadas) != 0 || sp.SinPagar != 1 {
		t.Errorf("la PROGRAMADA vino como %d pagadas / %d sin pagar, quería 0/1", len(sp.Pagadas), sp.SinPagar)
	}
	if _, hay := idx["CXP-NOEXISTE0000"]; hay {
		t.Error("un código inexistente devolvió una entrada")
	}
}

// El aislamiento por empresa, con la misma huella de las dos empresas. Un recibo de Valle de Paz
// pegado a una factura de Coopeprofa sería un dato de una empresa visible desde otra.
func TestDestinosPorHuellaNoCruzaEmpresas(t *testing.T) {
	repo := carpetaSembrada(t)
	idx, err := repo.DestinosPorHuella(context.Background(), empresaLote, []string{huellaAjena})
	if err != nil {
		t.Fatalf("DestinosPorHuella: %v", err)
	}
	for _, d := range idx[huellaAjena].Pagadas {
		if d.Proveedor == "AJENA" {
			t.Fatalf("una factura de otra empresa salió como destino (%s); el recibo se pegaría "+
				"a la cartera equivocada", d.Consecutivo)
		}
	}
	if n := len(idx[huellaAjena].Pagadas); n != 1 {
		t.Errorf("trajo %d destinos para la huella compartida, quería solo la propia", n)
	}
}

func TestGuardarComprobantesCarpeta(t *testing.T) {
	repo := carpetaSembrada(t)
	ctx := context.Background()

	idx, err := repo.DestinosPorHuella(ctx, empresaLote, []string{huellaRemCarp, huellaSuelta})
	if err != nil {
		t.Fatalf("destinos: %v", err)
	}
	docsRemesa := make([]string, 0, 3)
	for _, d := range idx[huellaRemCarp].Pagadas {
		docsRemesa = append(docsRemesa, d.DocumentoID)
	}
	docSuelta := idx[huellaSuelta].Pagadas[0].DocumentoID

	// La suelta ya fue enviada al proveedor: al reemplazar su comprobante, esa marca tiene que
	// caer. Si no, la pantalla diría «enviado» sobre un PDF que el proveedor nunca recibió.
	if _, err := repo.pool.Exec(ctx,
		`UPDATE documento_cxp SET comprobante_enviado_en = now() WHERE id = $1::uuid`, docSuelta); err != nil {
		t.Fatalf("marcar enviado: %v", err)
	}

	res, err := repo.GuardarComprobantesCarpeta(ctx, empresaLote, []ItemCarpeta{
		{Nombre: "REM 0043-07 CXPR-AAAA1111BBBB.pdf", Mime: "application/pdf",
			Contenido: []byte("%PDF-1.4 remesa"), DocumentoID: docsRemesa},
		{Nombre: "FC 000487 CXP-5555CCCC6666.pdf", Mime: "application/pdf",
			Contenido: []byte("%PDF-1.4 suelta"), DocumentoID: []string{docSuelta}},
	}, usuarioLote)
	if err != nil {
		t.Fatalf("GuardarComprobantesCarpeta: %v", err)
	}
	if res.Archivos != 2 {
		t.Errorf("archivos escritos %d, quería 2", res.Archivos)
	}
	// DOS archivos, CUATRO facturas: el recibo de la remesa queda en sus tres.
	if res.Facturas != 4 {
		t.Errorf("facturas tocadas %d, quería 4 (la remesa cubre 3)", res.Facturas)
	}
	if len(res.Rechazados) != 0 {
		t.Errorf("rechazó %v sin motivo", res.Rechazados)
	}

	var n int
	if err := repo.pool.QueryRow(ctx,
		`SELECT count(*) FROM comprobante_pago WHERE documento_id = ANY($1::uuid[])`, docsRemesa).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 3 {
		t.Errorf("el recibo de la remesa quedó en %d facturas, quería 3", n)
	}

	var enviado *string
	if err := repo.pool.QueryRow(ctx,
		`SELECT comprobante_enviado_en::text FROM documento_cxp WHERE id = $1::uuid`, docSuelta).Scan(&enviado); err != nil {
		t.Fatalf("leer marca: %v", err)
	}
	if enviado != nil {
		t.Errorf("la marca de enviado sobrevivió al reemplazo (%s): la pantalla diría «enviado» "+
			"sobre un PDF que el proveedor nunca recibió", *enviado)
	}

	// Volver a subir reemplaza y no duplica.
	if _, err := repo.GuardarComprobantesCarpeta(ctx, empresaLote, []ItemCarpeta{
		{Nombre: "otro.pdf", Mime: "application/pdf", Contenido: []byte("%PDF nuevo"),
			DocumentoID: []string{docSuelta}},
	}, usuarioLote); err != nil {
		t.Fatalf("resubir: %v", err)
	}
	var filas int
	var contenido []byte
	if err := repo.pool.QueryRow(ctx,
		`SELECT count(*) FROM comprobante_pago WHERE documento_id = $1::uuid`, docSuelta).Scan(&filas); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if filas != 1 {
		t.Errorf("quedaron %d comprobantes para una factura, quería 1", filas)
	}
	if err := repo.pool.QueryRow(ctx,
		`SELECT contenido FROM comprobante_pago WHERE documento_id = $1::uuid`, docSuelta).Scan(&contenido); err != nil {
		t.Fatalf("leer: %v", err)
	}
	if string(contenido) != "%PDF nuevo" {
		t.Errorf("el contenido no se reemplazó: %q", contenido)
	}
}

// El INSERT revalida el estado. Entre previsualizar y confirmar pasan minutos y la factura pudo
// anularse: sin la revalidación quedaría un recibo colgado de una factura que ya no se pagó.
func TestNoSeAdjuntaAUnaFacturaQueNoEstaPagada(t *testing.T) {
	repo := carpetaSembrada(t)
	ctx := context.Background()

	var docID string
	if err := repo.pool.QueryRow(ctx,
		`SELECT id::text FROM documento_cxp WHERE empresa_id = $1::uuid AND huella = $2`,
		empresaLote, huellaSinPagar).Scan(&docID); err != nil {
		t.Fatalf("buscar la programada: %v", err)
	}
	res, err := repo.GuardarComprobantesCarpeta(ctx, empresaLote, []ItemCarpeta{
		{Nombre: "FC 000999 CXP-9999EEEE0000.pdf", Mime: "application/pdf",
			Contenido: []byte("%PDF"), DocumentoID: []string{docID}},
	}, usuarioLote)
	if err != nil {
		t.Fatalf("GuardarComprobantesCarpeta: %v", err)
	}
	if res.Facturas != 0 {
		t.Errorf("adjuntó %d a una factura PROGRAMADA", res.Facturas)
	}
	if len(res.Rechazados) != 1 {
		t.Errorf("rechazados %v; callarlo mostraría «1 adjuntado» cuando fueron 0", res.Rechazados)
	}
	var n int
	if err := repo.pool.QueryRow(ctx,
		`SELECT count(*) FROM comprobante_pago WHERE documento_id = $1::uuid`, docID).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 0 {
		t.Error("quedó un comprobante pegado a una factura que todavía no se pagó")
	}
}

// Una factura de OTRA empresa no se puede escribir aunque el id llegue en la petición.
func TestGuardarCarpetaNoEscribeEnOtraEmpresa(t *testing.T) {
	repo := carpetaSembrada(t)
	ctx := context.Background()

	var ajeno string
	if err := repo.pool.QueryRow(ctx,
		`SELECT id::text FROM documento_cxp WHERE empresa_id = $1::uuid`, otraEmpLote).Scan(&ajeno); err != nil {
		t.Fatalf("buscar la ajena: %v", err)
	}
	res, err := repo.GuardarComprobantesCarpeta(ctx, empresaLote, []ItemCarpeta{
		{Nombre: "robado.pdf", Mime: "application/pdf", Contenido: []byte("%PDF"),
			DocumentoID: []string{ajeno}},
	}, usuarioLote)
	if err != nil {
		t.Fatalf("GuardarComprobantesCarpeta: %v", err)
	}
	if res.Facturas != 0 {
		t.Fatal("escribió un comprobante en la cartera de otra empresa")
	}
}

func TestPagosSinComprobante(t *testing.T) {
	repo := carpetaSembrada(t)
	ctx := context.Background()

	todos, err := repo.PagosSinComprobante(ctx, empresaLote, "", false)
	if err != nil {
		t.Fatalf("PagosSinComprobante: %v", err)
	}
	// UNA FILA POR TRANSFERENCIA: la remesa (3 facturas) cuenta una sola vez. Contar por factura
	// daría «de 100 pagos me faltan 140», que no se puede comparar con la carpeta del banco.
	// Esperadas: la remesa, la suelta del lote 43 y la del lote 44. La PROGRAMADA no, porque
	// todavía no es un pago.
	if len(todos) != 3 {
		for _, p := range todos {
			t.Logf("  %s · %s · %d facturas", p.Referencia, p.Codigo, p.Facturas)
		}
		t.Fatalf("devolvió %d transferencias, quería 3", len(todos))
	}
	var rem *PagoConRecibo
	for i := range todos {
		if todos[i].Codigo == huellaRemCarp {
			rem = &todos[i]
		}
	}
	if rem == nil {
		t.Fatal("la remesa no apareció")
	}
	if rem.Facturas != 3 {
		t.Errorf("la remesa dice %d facturas, quería 3", rem.Facturas)
	}
	if rem.Referencia != "REM 004307" {
		t.Errorf("referencia %q, quería «REM 004307» — es lo que el banco devuelve, sin el guion", rem.Referencia)
	}
	if rem.MontoCRC != "300000.00" {
		t.Errorf("monto %q, quería «300000.00» (la suma de las tres)", rem.MontoCRC)
	}
	// El NOMBRE DE ARCHIVO conserva el guion del código: no viaja al banco, y acá sí ayuda a
	// ver dónde empieza el código. Lo que cambió es la REFERENCIA, que sí viaja.
	if rem.NombreSugerido != "REM 004307 CXPR-AAAA1111BBBB.pdf" {
		t.Errorf("nombre sugerido %q", rem.NombreSugerido)
	}

	// La factura suelta se nombra como la macro: «FC 000487».
	var suelta *PagoConRecibo
	for i := range todos {
		if todos[i].Codigo == huellaSuelta {
			suelta = &todos[i]
		}
	}
	if suelta == nil {
		t.Fatal("la factura suelta no apareció")
	}
	if suelta.Referencia != "FC 000487" {
		t.Errorf("referencia de la suelta %q, quería «FC 000487»", suelta.Referencia)
	}
	if suelta.EsRemesa {
		t.Error("la suelta vino marcada como remesa")
	}

	// Todavía nadie adjuntó nada.
	for _, p := range todos {
		if p.ConRecibo != 0 {
			t.Errorf("%s dice %d con recibo y no se adjuntó ninguno", p.Referencia, p.ConRecibo)
		}
	}

	// El filtro por lote recorta a la corrida.
	delLote, err := repo.PagosSinComprobante(ctx, empresaLote, loteCarpeta, false)
	if err != nil {
		t.Fatalf("por lote: %v", err)
	}
	if len(delLote) != 2 {
		t.Errorf("el lote 43 trajo %d transferencias, quería 2", len(delLote))
	}
}

// `con_recibo` cuenta en vez de responder sí/no porque existe el caso intermedio: el recibo de una
// remesa adjunto a UNA sola de sus facturas. Un booleano tendría que mentir en una de las dos
// direcciones.
func TestLaRemesaMedioDocumentadaSigueFaltando(t *testing.T) {
	repo := carpetaSembrada(t)
	ctx := context.Background()

	idx, err := repo.DestinosPorHuella(ctx, empresaLote, []string{huellaRemCarp, huellaSuelta})
	if err != nil {
		t.Fatalf("destinos: %v", err)
	}
	unaSola := idx[huellaRemCarp].Pagadas[0].DocumentoID
	if _, err := repo.GuardarComprobantesCarpeta(ctx, empresaLote, []ItemCarpeta{
		{Nombre: "suelto.pdf", Mime: "application/pdf", Contenido: []byte("%PDF"),
			DocumentoID: []string{unaSola}},
	}, usuarioLote); err != nil {
		t.Fatalf("adjuntar una: %v", err)
	}

	faltantes, err := repo.PagosSinComprobante(ctx, empresaLote, "", true)
	if err != nil {
		t.Fatalf("faltantes: %v", err)
	}
	hayRemesa := false
	for _, p := range faltantes {
		if p.Codigo == huellaRemCarp {
			hayRemesa = true
			if p.ConRecibo != 1 || p.Facturas != 3 {
				t.Errorf("la remesa dice %d de %d con recibo, quería 1 de 3", p.ConRecibo, p.Facturas)
			}
		}
	}
	if !hayRemesa {
		t.Error("la remesa con una sola factura documentada desapareció de los faltantes: " +
			"el pago quedaría medio documentado para siempre y nadie volvería a mirarlo")
	}

	// Y al completarla, sale de la lista.
	docs := make([]string, 0, 3)
	for _, d := range idx[huellaRemCarp].Pagadas {
		docs = append(docs, d.DocumentoID)
	}
	if _, err := repo.GuardarComprobantesCarpeta(ctx, empresaLote, []ItemCarpeta{
		{Nombre: "REM 0043-07 CXPR-AAAA1111BBBB.pdf", Mime: "application/pdf",
			Contenido: []byte("%PDF"), DocumentoID: docs},
	}, usuarioLote); err != nil {
		t.Fatalf("completar: %v", err)
	}
	faltantes, err = repo.PagosSinComprobante(ctx, empresaLote, "", true)
	if err != nil {
		t.Fatalf("faltantes 2: %v", err)
	}
	for _, p := range faltantes {
		if p.Codigo == huellaRemCarp {
			t.Errorf("la remesa completa sigue en los faltantes (%d de %d)", p.ConRecibo, p.Facturas)
		}
	}
}
