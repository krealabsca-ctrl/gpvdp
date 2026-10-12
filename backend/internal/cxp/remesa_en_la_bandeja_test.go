package cxp

// La Bandeja muestra el código que de verdad viajó al banco (2-oct-2026).
//
// El Director corrió un lote por proveedor, abrió la macro y leyó «REM 0035-01 CXPR-219F9FF4F7AE».
// Abrió la Bandeja y la misma factura decía «CXP-B397CFFF40AD». Dos códigos distintos para un solo
// pago, y ninguna forma de cruzarlos: «no tiene comunicación la macro con lo que se expone pagado».
//
// Eran tres cortes del mismo hueco, y los tres se prueban acá:
//
//  1. La fila mostraba SIEMPRE la huella de la factura. En un pago agrupado esa huella no viaja a
//     ningún lado —lo que sale es la de la remesa— así que la pantalla ofrecía un código que el
//     estado de cuenta jamás iba a traer, y hasta lo recomendaba para buscar.
//  2. El enlace al banco se resolvía solo por `documento_cxp_id`. Un pago agrupado se enlaza por
//     `remesa_id` (mig 0089), de modo que una factura pagada dentro de una remesa iba a decir «sin
//     enlazar al banco» para siempre, aunque el débito estuviera conciliado.
//  3. El buscador no miraba NINGÚN código de pago. Pegar lo copiado del estado de cuenta no
//     devolvía nada, respuesta indistinguible de «esa factura no existe».
//
// Contra Postgres de verdad en esquema temporal: lo que hay que proteger es el SQL.

import (
	"context"
	"strings"
	"testing"
)

const (
	provBandeja   = "81818181-8181-4181-8181-818181818181"
	loteBandeja   = "82828282-8282-4282-8282-828282828282"
	remBandeja    = "83838383-8383-4383-8383-838383838383"
	bancoBandeja  = "84848484-8484-4484-8484-848484848484"
	cuentaBandeja = "85858585-8585-4585-8585-858585858585"
	// La factura que se paga sola, fuera de la remesa: el control de que no se inventa un pago
	// agrupado donde no lo hay.
	provSuelto = "86868686-8686-4686-8686-868686868686"
)

// Los códigos de la corrida real que destapó el problema.
const (
	huellaRemesaReal  = "CXPR-219F9FF4F7AE"
	huellaFacturaReal = "CXP-B397CFFF40AD"
)

// bandejaConRemesa siembra un lote cortado POR PROVEEDOR: tres facturas de un proveedor dentro de
// una remesa, y una cuarta de otro proveedor que se paga sola.
func bandejaConRemesa(t *testing.T) *pgRepository {
	t.Helper()
	repo := baseDeLotes(t)
	ctx := context.Background()

	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO proveedor (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'INVERSIONES DEL PACIFICO')`,
			[]any{provBandeja, empresaLote}},
		{`INSERT INTO proveedor (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'FRANCELA')`,
			[]any{provSuelto, empresaLote}},
		{`INSERT INTO banco (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'BN')`,
			[]any{bancoBandeja, empresaLote}},
		{`INSERT INTO cuenta_bancaria (id, empresa_id, banco_id, moneda, alias)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CRC', 'BN Jardines Colones')`,
			[]any{cuentaBandeja, empresaLote, bancoBandeja}},
		// Lote 35, como el real: el número importa porque viaja en la referencia.
		{`INSERT INTO lote_pago (id, empresa_id, numero, fecha_corte, modo_pago)
		  VALUES ($1::uuid, $2::uuid, 35, '2026-10-02'::date, 'PROVEEDOR')`,
			[]any{loteBandeja, empresaLote}},
		{`INSERT INTO remesa_pago (id, empresa_id, lote_id, proveedor_id, numero, huella, monto_crc)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 1, $5, 79545)`,
			[]any{remBandeja, empresaLote, loteBandeja, provBandeja, huellaRemesaReal}},
	} {
		if _, err := repo.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("sembrar: %v", err)
		}
	}

	// Las tres de la remesa. La primera lleva la huella real de la captura.
	huellas := []string{huellaFacturaReal, "CXP-000000000002", "CXP-000000000003"}
	for i, h := range huellas {
		if _, err := repo.pool.Exec(ctx, `
			INSERT INTO documento_cxp
				(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
				 total, total_crc, lote_id, remesa_id, huella)
			VALUES ($1::uuid, $2::uuid, $3, $3, '2026-09-01'::date, 'PAGADO', 'CRC',
			        26515, 26515, $4::uuid, $5::uuid, $6)`,
			empresaLote, provBandeja, "0030000101000033769"+string(rune('0'+i)),
			loteBandeja, remBandeja, h); err != nil {
			t.Fatalf("sembrar factura en remesa %d: %v", i, err)
		}
	}
	// La que se paga sola: mismo lote, sin remesa.
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
			 total, total_crc, lote_id, huella)
		VALUES ($1::uuid, $2::uuid, 'SUELTA-1', 'SUELTA-1', '2026-09-01'::date, 'PAGADO', 'CRC',
		        50000, 50000, $3::uuid, 'CXP-5555555555AA')`,
		empresaLote, provSuelto, loteBandeja); err != nil {
		t.Fatalf("sembrar factura suelta: %v", err)
	}
	return repo
}

// unaFactura devuelve la única factura que calza con la búsqueda, o falla.
func unaFactura(t *testing.T, repo *pgRepository, f FiltrosDocumentos) Documento {
	t.Helper()
	lista, err := repo.ListarDocumentos(context.Background(), empresaLote, f)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(lista.Items) != 1 {
		t.Fatalf("trajo %d facturas, se esperaba 1 (filtro %+v)", len(lista.Items), f)
	}
	return lista.Items[0]
}

func TestLaFilaDeUnPagoAgrupadoMuestraElCodigoQueViajoAlBanco(t *testing.T) {
	repo := bandejaConRemesa(t)

	d := unaFactura(t, repo, FiltrosDocumentos{Q: "0030000101000033769" + "0"})
	if d.Remesa == nil {
		t.Fatal("la factura va dentro de una remesa y la fila no la trae: la pantalla seguiría mostrando solo su huella, que el banco nunca va a devolver")
	}
	if d.Remesa.Huella != huellaRemesaReal {
		t.Errorf("huella de remesa = %q, quiere %q", d.Remesa.Huella, huellaRemesaReal)
	}
	// La referencia tiene que salir IGUAL que en la macro: «REM 0035-01».
	if quiere := ReferenciaRemesa(35, 1); d.Remesa.Referencia != quiere {
		t.Errorf("referencia = %q, quiere %q — es lo que la persona lee en el estado de cuenta", d.Remesa.Referencia, quiere)
	}
	if d.Remesa.Facturas != 3 {
		t.Errorf("la transferencia cubre %d facturas, se sembraron 3", d.Remesa.Facturas)
	}
	// Y la huella propia NO se pierde: sigue siendo la identidad del documento.
	if d.Huella != huellaFacturaReal {
		t.Errorf("huella de la factura = %q, quiere %q", d.Huella, huellaFacturaReal)
	}
}

func TestUnaFacturaQueSePagaSolaNoInventaRemesa(t *testing.T) {
	repo := bandejaConRemesa(t)

	d := unaFactura(t, repo, FiltrosDocumentos{Q: "SUELTA-1"})
	if d.Remesa != nil {
		t.Errorf("la factura se paga sola y la fila trae remesa %+v: mostraría un pago agrupado que no existe", d.Remesa)
	}
	if d.Huella != "CXP-5555555555AA" {
		t.Errorf("huella = %q", d.Huella)
	}
}

// El corte 2, el que no se veía: el débito está conciliado y la factura decía «sin enlazar».
func TestElEnlaceAlBancoLlegaPorElCaminoDeLaRemesa(t *testing.T) {
	repo := bandejaConRemesa(t)
	ctx := context.Background()

	// UN movimiento que paga la remesa entera (mig 0089: apunta a remesa_id, no a la factura).
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, natural_key, remesa_id)
		VALUES ($1::uuid, $2::uuid, '2026-10-02'::date, $3, 79545, 0,
		        'CRC', 79545, -79545, 'nk-remesa-1', $4::uuid)`,
		empresaLote, cuentaBandeja, "REM 0035-01 CXPR219F9FF4F7AE", remBandeja); err != nil {
		t.Fatalf("sembrar movimiento: %v", err)
	}

	// Las TRES facturas de la remesa quedan enlazadas al mismo débito.
	lista, err := repo.ListarDocumentos(ctx, empresaLote, FiltrosDocumentos{ProveedorID: provBandeja})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(lista.Items) != 3 {
		t.Fatalf("trajo %d facturas, se sembraron 3", len(lista.Items))
	}
	for _, d := range lista.Items {
		if d.PagoBanco == nil {
			t.Fatalf("factura %s: el débito de su remesa está conciliado y la fila dice «sin enlazar al banco»", d.Consecutivo)
		}
		if d.PagoBanco.Cuenta != "BN Jardines Colones" {
			t.Errorf("factura %s: cuenta = %q", d.Consecutivo, d.PagoBanco.Cuenta)
		}
		// El débito es el de la TRANSFERENCIA (las tres juntas), no el de la factura: por eso no se
		// escribe como si fuera el pago individual de cada una.
		if d.PagoBanco.Debito != "79545.00" {
			t.Errorf("factura %s: débito = %q, quiere 79545.00 (el de la transferencia)", d.Consecutivo, d.PagoBanco.Debito)
		}
	}
}

// Dos caminos a la vez no pueden duplicar la fila: el listado paginado contaría de más y la misma
// factura aparecería dos veces en la Bandeja, en silencio.
func TestLosDosCaminosDeEnlaceNoDuplicanLaFila(t *testing.T) {
	repo := bandejaConRemesa(t)
	ctx := context.Background()

	var facturaID string
	if err := repo.pool.QueryRow(ctx,
		`SELECT id::text FROM documento_cxp WHERE huella = $1`, huellaFacturaReal).Scan(&facturaID); err != nil {
		t.Fatalf("buscar factura: %v", err)
	}
	// El CHECK de la mig 0089 impide las dos columnas en UN movimiento, así que se siembran dos:
	// uno enlazado a la remesa y otro a la factura. Es lo que pasaría si una factura se hubiera
	// pagado suelta antes de entrar al grupo.
	for _, q := range []struct{ nk, col, val string }{
		{"nk-dup-remesa", "remesa_id", remBandeja},
		{"nk-dup-factura", "documento_cxp_id", facturaID},
	} {
		if _, err := repo.pool.Exec(ctx, `
			INSERT INTO movimiento_bancario
				(empresa_id, cuenta_bancaria_id, fecha, descripcion, debito, credito,
				 moneda_original, monto_original, monto_crc, natural_key, `+q.col+`)
			VALUES ($1::uuid, $2::uuid, '2026-10-02'::date, 'PAGO', 1000, 0,
			        'CRC', 1000, -1000, $3, $4::uuid)`,
			empresaLote, cuentaBandeja, q.nk, q.val); err != nil {
			t.Fatalf("sembrar %s: %v", q.nk, err)
		}
	}

	lista, err := repo.ListarDocumentos(ctx, empresaLote, FiltrosDocumentos{ProveedorID: provBandeja})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(lista.Items) != 3 || lista.Total != 3 {
		t.Fatalf("trajo %d filas (total %d), se sembraron 3 facturas: el JOIN está multiplicando",
			len(lista.Items), lista.Total)
	}
}

// El corte 3: se ve un débito en el banco, se copia el código y se pega acá.
func TestSeBuscaUnaFacturaPorElCodigoQueTraeElEstadoDeCuenta(t *testing.T) {
	repo := bandejaConRemesa(t)

	casos := []struct {
		nombre, q string
		quiere    int
	}{
		// El banco BORRA el guion: lo que la persona copia nunca lo trae.
		{"la huella de la remesa sin guion, como vuelve del banco", "CXPR219F9FF4F7AE", 3},
		{"la huella de la remesa con guion, como se emitió", huellaRemesaReal, 3},
		{"la referencia legible", "REM 0035-01", 3},
		{"la huella de una factura suelta, sin guion", "CXP5555555555AA", 1},
		{"la huella de una factura suelta, con guion", "CXP-5555555555AA", 1},
		// Una factura DENTRO de la remesa conserva su huella y se la puede buscar igual.
		{"la huella propia de una factura agrupada", huellaFacturaReal, 1},
		{"un código que no existe no trae nada", "CXPR-000000000000", 0},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			lista, err := repo.ListarDocumentos(context.Background(), empresaLote, FiltrosDocumentos{Q: c.q})
			if err != nil {
				t.Fatalf("listar: %v", err)
			}
			if len(lista.Items) != c.quiere {
				t.Errorf("buscar %q trajo %d, quiere %d", c.q, len(lista.Items), c.quiere)
			}
		})
	}
}

// Las dos escrituras del mismo formato tienen que coincidir carácter por carácter: la macro la
// arma en Go y la Bandeja en SQL. Si se separan, el usuario vuelve a ver dos códigos distintos —
// que es exactamente el problema que esto vino a arreglar.
func TestLaReferenciaEnSQLYLaDeGoSonLaMisma(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	casos := []struct {
		lote   int64
		remesa int
	}{
		{35, 1},    // la corrida real
		{1, 1},     // el mínimo
		{9999, 99}, // el máximo que cabe en el formato
		{12345, 7}, // un lote que se pasa del ancho: FM no trunca, y las dos tienen que crecer igual
		{29, 10},   // dos dígitos en la remesa
	}
	for _, c := range casos {
		var enSQL string
		if err := repo.pool.QueryRow(ctx,
			`SELECT `+referenciaRemesaSQL+` FROM (SELECT $1::bigint AS numero) lrem, (SELECT $2::int AS numero) rcxp`,
			c.lote, c.remesa).Scan(&enSQL); err != nil {
			t.Fatalf("evaluar SQL (%d,%d): %v", c.lote, c.remesa, err)
		}
		if enGo := ReferenciaRemesa(c.lote, c.remesa); enSQL != enGo {
			t.Errorf("lote %d remesa %d: SQL dice %q y Go dice %q", c.lote, c.remesa, enSQL, enGo)
		}
	}
}

// La otra mitad de la nomenclatura: «FC 000487», la referencia del pago POR FACTURA. Tenía el mismo
// hueco que la remesa —se lee en el estado de cuenta y no se podía pegar en la Bandeja— porque el
// «FC » lo agrega la macro al salir y no vive en ningún campo de la factura.
func TestSeBuscaUnaFacturaPorSuReferenciaFC(t *testing.T) {
	repo := bandejaConRemesa(t)
	ctx := context.Background()

	// Una factura con consecutivo CORTO: el relleno con ceros de la macro no está en el dato, así
	// que un LIKE sobre el consecutivo nunca calzaría con «FC 000651».
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda, total, total_crc)
		VALUES ($1::uuid, $2::uuid, 'K-651', '651', '2026-09-01'::date, 'APROBADO', 'CRC', 1000, 1000)`,
		empresaLote, provSuelto); err != nil {
		t.Fatalf("sembrar factura corta: %v", err)
	}

	casos := []struct {
		nombre, q string
		quiere    int
	}{
		// Los últimos 6 del consecutivo largo 00300001010000337690 son «337690».
		{"la referencia tal cual sale en el banco", "FC 337690", 1},
		{"sin el espacio", "FC337690", 1},
		{"en minúscula", "fc 337690", 1},
		// Acá está la gracia: los ceros los pone la macro, no el consecutivo.
		{"una factura corta, con el relleno de ceros", "FC 000651", 1},
		{"la misma, escrita sin los ceros", "FC 651", 1},
		{"un número que nadie tiene", "FC 999999", 0},
		// Y no se confunde con texto que empieza igual.
		{"«FACTURA 12» no es un código de pago", "FACTURA 12", 0},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			lista, err := repo.ListarDocumentos(ctx, empresaLote, FiltrosDocumentos{Q: c.q})
			if err != nil {
				t.Fatalf("listar: %v", err)
			}
			if len(lista.Items) != c.quiere {
				t.Errorf("buscar %q trajo %d, quiere %d", c.q, len(lista.Items), c.quiere)
			}
		})
	}
}

// Lo que la macro escribe tiene que poder leerse de vuelta. Si las dos se separan, se repite el
// problema de origen: un código impreso en el estado de cuenta que el sistema no reconoce.
func TestLoQueEscribeLaMacroSeLeeDeVuelta(t *testing.T) {
	for _, consecutivo := range []string{"00300001010000337698", "651", "0000000000000000487", "12"} {
		emitido := macroDescripcion(consecutivo, "CXP-AAAAAAAAAAAA")
		// La macro emite «FC nnnnnn <huella>»; del estado de cuenta se copia la parte legible.
		partes := strings.Fields(emitido)
		if len(partes) < 2 {
			t.Fatalf("consecutivo %q: la macro emitió %q", consecutivo, emitido)
		}
		referencia := partes[0] + " " + partes[1]
		if leido := clavePagoFC(referencia); leido != partes[1] {
			t.Errorf("la macro emite %q y al leer %q se obtiene %q", emitido, referencia, leido)
		}
	}
}

// El recorte por empresa llega también a lo nuevo: una remesa ajena no se asoma por la fila.
func TestLaRemesaDeOtraEmpresaNoSeVe(t *testing.T) {
	repo := bandejaConRemesa(t)

	lista, err := repo.ListarDocumentos(context.Background(), otraEmpLote, FiltrosDocumentos{Q: "CXPR219F9FF4F7AE"})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(lista.Items) != 0 {
		t.Errorf("otra empresa vio %d facturas de la remesa", len(lista.Items))
	}
}
