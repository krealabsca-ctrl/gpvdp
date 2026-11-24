package cxp

// El arqueo de excepción y el corte del lote, contra PostgreSQL de verdad (20-oct-2026).
//
// Hallazgos 3.1 y 3.3 de la auditoría Bancos↔CxP. Nada de esto se puede probar con un doble: lo
// que se está afirmando es que CIERTAS consultas distinguen correctamente «el banco lo debitó» de
// «el banco no lo debitó», y un doble solo repetiría lo que yo creo que hacen.
//
// Sobre los datos reales el reporte devuelve 62 facturas por ₡2.757.710,84 con el estado de cuenta
// cargado hasta el 14 de setiembre. Acá se fijan las reglas que producen ese número.

import (
	"context"
	"errors"
	"testing"
)

// fechaDePago le pone a una factura su fecha de pago programada.
func fechaDePago(t *testing.T, repo *pgRepository, docID, fecha string) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(),
		`UPDATE documento_cxp SET fecha_pago_programada = $2::date WHERE id = $1::uuid`, docID, fecha); err != nil {
		t.Fatalf("fecha de pago: %v", err)
	}
}

// cuentaYMovimiento siembra un débito bancario. `docID` y `remesaID` son opcionales: el que venga
// no vacío es la columna de enlace.
func movimiento(t *testing.T, repo *pgRepository, fecha, monto, docID, remesaID string, incluido bool) string {
	t.Helper()
	ctx := context.Background()
	var cuentaID string
	err := repo.pool.QueryRow(ctx, `SELECT id::text FROM cuenta_bancaria LIMIT 1`).Scan(&cuentaID)
	if err != nil {
		if _, e := repo.pool.Exec(ctx,
			`INSERT INTO banco (id, empresa_id, nombre) VALUES ('55555555-5555-4555-8555-555555555555','`+empresaLote+`','BN')`); e != nil {
			t.Fatalf("banco: %v", e)
		}
		if e := repo.pool.QueryRow(ctx, `
			INSERT INTO cuenta_bancaria (empresa_id, banco_id, iban, moneda, alias)
			VALUES ('`+empresaLote+`','55555555-5555-4555-8555-555555555555','CR99','CRC','BN Colones')
			RETURNING id::text`).Scan(&cuentaID); e != nil {
			t.Fatalf("cuenta: %v", e)
		}
	}
	var id string
	if e := repo.pool.QueryRow(ctx, `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, descripcion, debito, credito, monto_crc,
			 moneda_original, monto_original, estado_clasificacion, natural_key, incluido,
			 documento_cxp_id, remesa_id)
		VALUES ($1::uuid, $2::uuid, $3::date, 'PAGO PROVEEDOR', $4, 0, $4, 'CRC', $4,
		        'NO_IDENTIFICADO', $5, $6, NULLIF($7,'')::uuid, NULLIF($8,'')::uuid)
		RETURNING id::text`,
		empresaLote, cuentaID, fecha, monto, "nk-"+fecha+"-"+monto+docID+remesaID, incluido, docID, remesaID).Scan(&id); e != nil {
		t.Fatalf("movimiento: %v", e)
	}
	return id
}

// EL ENLACE TIENE DOS CAMINOS Y HAY QUE MIRAR LOS DOS.
//
// Un pago individual deja el movimiento apuntando a la FACTURA; uno agrupado apunta a la REMESA, y
// la factura cuelga de esa remesa. Si la consulta mirara solo el primero, TODA factura pagada en
// grupo aparecería como «el banco nunca la debitó» — y pagar agrupado es justamente la forma en
// que se le paga a un proveedor con varias facturas.
//
// Falsificación: quitarle a `sinRespaldoBase` la condición de la remesa hace que la factura
// agrupada aparezca en el reporte, y el test falla diciendo exactamente eso.
func TestElPagoAgrupadoCuentaComoRespaldo(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	// Tres facturas declaradas pagadas, todas con fecha anterior al corte del banco.
	sinNada := factura(t, repo, "F-SIN", EstPagado, "10000", false, empresaLote)
	porFactura := factura(t, repo, "F-DIRECTA", EstPagado, "20000", false, empresaLote)
	porRemesa := factura(t, repo, "F-AGRUPADA", EstPagado, "30000", false, empresaLote)
	for _, id := range []string{sinNada, porFactura, porRemesa} {
		fechaDePago(t, repo, id, "2026-08-10")
	}

	// La segunda tiene su débito directo.
	movimiento(t, repo, "2026-08-11", "20000", porFactura, "", true)

	// La tercera se pagó dentro de una remesa, y el débito apunta a la REMESA. La remesa cuelga de
	// un lote (`lote_id` es NOT NULL): una remesa es siempre parte de un corte.
	var loteID string
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO lote_pago (empresa_id, numero, fecha_corte, creado_por, modo_pago)
		VALUES ($1::uuid, 99, '2026-08-07'::date, $2::uuid, 'PROVEEDOR') RETURNING id::text`,
		empresaLote, usuarioLote).Scan(&loteID); err != nil {
		t.Fatalf("lote de la remesa: %v", err)
	}
	var remesaID string
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO remesa_pago (empresa_id, lote_id, proveedor_id, numero, huella, monto_crc)
		VALUES ($1::uuid, $3::uuid, $2::uuid, 1, 'REM 000101', '30000') RETURNING id::text`,
		empresaLote, provLote, loteID).Scan(&remesaID); err != nil {
		t.Fatalf("remesa: %v", err)
	}
	if _, err := repo.pool.Exec(ctx,
		`UPDATE documento_cxp SET remesa_id = $2::uuid WHERE id = $1::uuid`, porRemesa, remesaID); err != nil {
		t.Fatalf("ligar remesa: %v", err)
	}
	movimiento(t, repo, "2026-08-11", "30000", "", remesaID, true)

	fs, err := repo.FacturasSinRespaldo(ctx, empresaLote, "2026-09-14", "")
	if err != nil {
		t.Fatalf("facturas sin respaldo: %v", err)
	}
	enReporte := map[string]bool{}
	for _, f := range fs {
		enReporte[f.DocumentoID] = true
	}
	if !enReporte[sinNada] {
		t.Error("la factura SIN ningún débito no salió en el arqueo: es justamente la que se busca")
	}
	if enReporte[porFactura] {
		t.Error("una factura con su débito enlazado salió como «sin respaldo»")
	}
	if enReporte[porRemesa] {
		t.Error("la factura pagada POR REMESA salió como «sin respaldo»: el enlace por remesa no se está mirando")
	}
}

// Un movimiento revertido o excluido NO respalda nada.
//
// `incluido = false` significa que el movimiento ya no está en los libros —se revirtió la carga, o
// alguien lo excluyó a mano—. Contarlo como respaldo dejaría la factura fuera del arqueo apoyada
// en plata que el sistema ya no reconoce.
func TestUnMovimientoExcluidoNoRespalda(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	doc := factura(t, repo, "F-EXCL", EstPagado, "15000", false, empresaLote)
	fechaDePago(t, repo, doc, "2026-08-10")
	movimiento(t, repo, "2026-08-11", "15000", doc, "", false) // excluido

	fs, err := repo.FacturasSinRespaldo(ctx, empresaLote, "2026-09-14", "")
	if err != nil {
		t.Fatalf("facturas sin respaldo: %v", err)
	}
	hallada := false
	for _, f := range fs {
		if f.DocumentoID == doc {
			hallada = true
		}
	}
	if !hallada {
		t.Error("un movimiento EXCLUIDO se está contando como respaldo: la factura quedó fuera del arqueo apoyada en plata que no está en los libros")
	}
}

// Solo entra lo que el banco YA PUDO haber debitado.
//
// Una factura programada para el 30 de setiembre cuando el estado de cuenta llega al 14 no es una
// excepción: es un estado de cuenta que falta cargar. Mezclarlas vuelve el reporte una lista que
// nadie termina de revisar, que es la forma segura de que deje de mirarse.
func TestLoQueElBancoTodaviaNoPudoDebitarNoEsExcepcion(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	vieja := factura(t, repo, "F-VIEJA", EstPagado, "11000", false, empresaLote)
	futura := factura(t, repo, "F-FUTURA", EstPagado, "12000", false, empresaLote)
	fechaDePago(t, repo, vieja, "2026-08-10")
	fechaDePago(t, repo, futura, "2026-09-30") // después del corte

	fs, err := repo.FacturasSinRespaldo(ctx, empresaLote, "2026-09-14", "")
	if err != nil {
		t.Fatalf("sin respaldo: %v", err)
	}
	for _, f := range fs {
		if f.DocumentoID == futura {
			t.Error("entró una factura cuya fecha de pago el banco todavía no alcanzó: eso no es una excepción, es banco sin cargar")
		}
	}
	if len(fs) != 1 || fs[0].DocumentoID != vieja {
		t.Errorf("esperaba solo la factura vencida en el arqueo, obtuve %d", len(fs))
	}

	// Y la futura tiene que contarse aparte, o «1 excepción» se leería como «lo demás cuadra».
	n, err := repo.ContarAunNoVencidas(ctx, empresaLote, "2026-09-14")
	if err != nil {
		t.Fatalf("aún no vencidas: %v", err)
	}
	if n != 1 {
		t.Errorf("aún no vencidas = %d, esperaba 1: sin ese número el reporte se lee como si todo lo demás cuadrara", n)
	}
}

// Cerrar el lote congela su contenido: una factura no puede salir de un corte ya dado.
//
// Falsificación: quitarle a `Reintentar` la condición del lote cerrado hace que la factura salga
// igual y el test falla.
func TestUnLoteCerradoNoDejaSalirSusFacturas(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	doc := factura(t, repo, "F-LOTE", "PROGRAMADO", "25000", false, empresaLote)
	lote, err := repo.CrearLote(ctx, empresaLote, "2026-08-07", ModoFactura, []string{doc}, usuarioLote)
	if err != nil {
		t.Fatalf("crear lote: %v", err)
	}
	if _, err := repo.pool.Exec(ctx,
		`UPDATE documento_cxp SET estado = 'REBOTADA' WHERE id = $1::uuid`, doc); err != nil {
		t.Fatalf("rebotar: %v", err)
	}

	// Con el lote ABIERTO, la factura sale (comportamiento de siempre: no se rompe nada).
	n, err := repo.Reintentar(ctx, empresaLote, doc)
	if err != nil || n != 1 {
		t.Fatalf("con el lote abierto el reintento tiene que funcionar como siempre (n=%d, err=%v)", n, err)
	}

	// Se vuelve a meter en el lote y ahora se cierra.
	if _, err := repo.pool.Exec(ctx,
		`UPDATE documento_cxp SET estado='REBOTADA', lote_id=$2::uuid WHERE id=$1::uuid`, doc, lote.ID); err != nil {
		t.Fatalf("reponer en el lote: %v", err)
	}
	if n, err := repo.CerrarLote(ctx, empresaLote, lote.ID, usuarioLote); err != nil || n != 1 {
		t.Fatalf("cerrar lote (n=%d, err=%v)", n, err)
	}

	n, err = repo.Reintentar(ctx, empresaLote, doc)
	if err != nil {
		t.Fatalf("reintentar: %v", err)
	}
	if n != 0 {
		t.Error("la factura salió de un lote CERRADO: el corte no congela nada y la macro emitida deja de cuadrar con el lote")
	}

	// Y el sistema tiene que poder explicar por qué no se movió.
	cerrado, err := repo.LoteDeDocumentoEstaCerrado(ctx, empresaLote, doc)
	if err != nil {
		t.Fatalf("explicación: %v", err)
	}
	if !cerrado {
		t.Error("no se puede explicar el rechazo: «0 filas» se vería igual que «la factura no está rebotada», y son dos arreglos distintos")
	}
}

// Cerrar es idempotente hacia afuera, pero no pisa el cierre bueno; y reabrir deja el motivo.
func TestCerrarYReabrirDejanSuHuella(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	doc := factura(t, repo, "F-HUELLA", "PROGRAMADO", "9000", false, empresaLote)
	lote, err := repo.CrearLote(ctx, empresaLote, "2026-08-07", ModoFactura, []string{doc}, usuarioLote)
	if err != nil {
		t.Fatalf("crear lote: %v", err)
	}

	if n, _ := repo.CerrarLote(ctx, empresaLote, lote.ID, usuarioLote); n != 1 {
		t.Fatal("el primer cierre tiene que mover una fila")
	}
	if n, _ := repo.CerrarLote(ctx, empresaLote, lote.ID, usuarioLote); n != 0 {
		t.Error("cerrar dos veces movió filas: pisaría la fecha y el autor del cierre bueno")
	}
	existe, cerrado, err := repo.LoteEstaCerrado(ctx, empresaLote, lote.ID)
	if err != nil || !existe || !cerrado {
		t.Fatalf("el lote tiene que quedar cerrado (existe=%v cerrado=%v err=%v)", existe, cerrado, err)
	}

	if n, _ := repo.ReabrirLote(ctx, empresaLote, lote.ID, usuarioLote, "el banco rechazó dos transferencias del corte"); n != 1 {
		t.Fatal("reabrir tiene que mover una fila")
	}
	var motivo, cerradoEn string
	if err := repo.pool.QueryRow(ctx, `
		SELECT COALESCE(reapertura_motivo,''), COALESCE(cerrado_en::text,'')
		FROM lote_pago WHERE id = $1::uuid`, lote.ID).Scan(&motivo, &cerradoEn); err != nil {
		t.Fatalf("leer lote: %v", err)
	}
	if motivo == "" {
		t.Error("se reabrió sin dejar el motivo: es la única huella de que un corte terminado se volvió a mover")
	}
	if cerradoEn == "" {
		t.Error("reabrir borró la fecha del cierre: que el lote estuvo cerrado alguna vez es parte de lo que hay que poder reconstruir")
	}
}

// Deshacer una conciliación la devuelve a PAGADO, NUNCA a PROGRAMADO.
//
// La diferencia no es cosmética: PROGRAMADO es la cola de pago. Una factura que vuelve ahí puede
// entrar a un lote nuevo y salir al banco por segunda vez, con el primer pago ya hecho. Lo que el
// desenlace retira es la confirmación del banco, no la declaración de que el pago se hizo.
//
// Falsificación: cambiar el destino a PROGRAMADO en `AbrirRemesa` o en el servicio hace que este
// test falle diciendo exactamente eso.
func TestDeshacerLaConciliacionNoDevuelveLaFacturaALaColaDePago(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	svc := NewService(repo, nil, nil)

	doc := factura(t, repo, "F-VUELTA", EstConciliado, "40000", false, empresaLote)

	n, err := svc.Desconciliar(ctx, empresaLote, doc, "", usuarioLote,
		"se enlazó contra el débito equivocado del mismo proveedor")
	if err != nil {
		t.Fatalf("desconciliar: %v", err)
	}
	if n != 1 {
		t.Fatalf("volvieron %d facturas, esperaba 1", n)
	}

	var estado string
	if err := repo.pool.QueryRow(ctx,
		`SELECT estado FROM documento_cxp WHERE id = $1::uuid`, doc).Scan(&estado); err != nil {
		t.Fatalf("leer estado: %v", err)
	}
	if estado == "PROGRAMADO" {
		t.Fatal("la factura volvió a la COLA DE PAGO: puede entrar a un lote nuevo y pagarse dos veces")
	}
	if estado != EstPagado {
		t.Errorf("estado = %q, esperaba PAGADO: lo que se retira es la confirmación del banco, no el pago declarado", estado)
	}
}

// Sin motivo no se deshace, y el mínimo es el MISMO de liberar y reabrir.
func TestDeshacerLaConciliacionExigeMotivo(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	svc := NewService(repo, nil, nil)

	doc := factura(t, repo, "F-SINMOT", EstConciliado, "40000", false, empresaLote)
	if _, err := svc.Desconciliar(ctx, empresaLote, doc, "", usuarioLote, "ok"); !errors.Is(err, ErrMotivoDesenlaceRequerido) {
		t.Errorf("un motivo de dos letras pasó: el expediente quedaría sin nada que leer (%v)", err)
	}

	// Y la factura no se movió: una validación que rechaza pero igual escribe no es una validación.
	var estado string
	if err := repo.pool.QueryRow(ctx,
		`SELECT estado FROM documento_cxp WHERE id = $1::uuid`, doc).Scan(&estado); err != nil {
		t.Fatalf("leer estado: %v", err)
	}
	if estado != EstConciliado {
		t.Errorf("la factura cambió a %q pese a que el motivo se rechazó", estado)
	}
}
