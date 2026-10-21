package cxp

// Un pago por proveedor, no uno por factura (1-oct-2026).
//
// Medido sobre los 28 lotes reales de Valle de Paz: 9 tenían más de una factura del mismo
// proveedor. El lote #3 salió con 11 transferencias para 8 proveedores. Cada transferencia de más
// es una comisión y una línea más que cuadrar.
//
// Lo que estas pruebas protegen es la parte que no se ve hasta que falla: que el monto de la remesa
// sea EXACTAMENTE la suma de los netos que cubre (si no, el banco debita una cifra y la
// conciliación espera otra, y el pago queda colgado), y que cada remesa tenga su propio código.

import (
	"context"
	"strings"
	"testing"
)

const (
	provUno = "55555555-5555-4555-8555-555555555555"
	provDos = "66666666-6666-4666-8666-666666666666"
)

// proveedorExtra siembra un proveedor más, para poder probar la agrupación.
func proveedorExtra(t *testing.T, repo *pgRepository, id, nombre string) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(),
		`INSERT INTO proveedor (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, $3)`,
		id, empresaLote, nombre); err != nil {
		t.Fatalf("sembrar proveedor %s: %v", nombre, err)
	}
}

// facturaDe es como `factura` pero eligiendo el proveedor.
func facturaDe(t *testing.T, repo *pgRepository, proveedorID, nombre, total string) string {
	t.Helper()
	var id string
	err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, total, total_crc, moneda)
		VALUES ($1::uuid, $2::uuid, $3, $3, '2026-08-01'::date, 'PROGRAMADO', $4, $4, 'CRC')
		RETURNING id::text`,
		empresaLote, proveedorID, nombre, total).Scan(&id)
	if err != nil {
		t.Fatalf("sembrar factura %s: %v", nombre, err)
	}
	return id
}

func TestUnPagoPorProveedorAgrupaSusFacturas(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	proveedorExtra(t, repo, provUno, "ALFA SOCIEDAD ANONIMA")
	proveedorExtra(t, repo, provDos, "ZETA LIMITADA")

	// Tres facturas de ALFA y una de ZETA: el caso real del lote #3.
	a1 := facturaDe(t, repo, provUno, "F-A1", "100000")
	a2 := facturaDe(t, repo, provUno, "F-A2", "250000.50")
	a3 := facturaDe(t, repo, provUno, "F-A3", "49999.50")
	z1 := facturaDe(t, repo, provDos, "F-Z1", "777000")

	lote, err := repo.CrearLote(ctx, empresaLote, "2026-10-02", ModoProveedor,
		[]string{a1, a2, a3, z1}, usuarioLote)
	if err != nil {
		t.Fatalf("crear lote por proveedor: %v", err)
	}
	if lote.Cantidad != 4 {
		t.Fatalf("entraron %d facturas, se esperaban 4", lote.Cantidad)
	}
	if lote.ModoPago != ModoProveedor {
		t.Errorf("modo_pago = %q, quiere %q: de esto depende cómo se arma la macro", lote.ModoPago, ModoProveedor)
	}

	// Cuatro facturas, DOS transferencias.
	type fila struct {
		proveedor string
		numero    int
		huella    string
		monto     string
		facturas  int
	}
	rows, err := repo.pool.Query(ctx, `
		SELECT p.nombre, r.numero, r.huella, r.monto_crc::text,
		       (SELECT COUNT(*) FROM documento_cxp d WHERE d.remesa_id = r.id)
		FROM remesa_pago r JOIN proveedor p ON p.id = r.proveedor_id
		WHERE r.lote_id = $1::uuid ORDER BY r.numero`, lote.ID)
	if err != nil {
		t.Fatalf("leer remesas: %v", err)
	}
	defer rows.Close()
	var got []fila
	for rows.Next() {
		var f fila
		if err := rows.Scan(&f.proveedor, &f.numero, &f.huella, &f.monto, &f.facturas); err != nil {
			t.Fatalf("scan remesa: %v", err)
		}
		got = append(got, f)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterar remesas: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("se crearon %d remesas, se esperaban 2 (una por proveedor): %+v", len(got), got)
	}

	// Orden por nombre de proveedor: ALFA es la 1 y ZETA la 2. El número es la mitad legible de la
	// referencia que va al banco, así que no puede bailar entre corridas.
	quiere := []fila{
		{proveedor: "ALFA SOCIEDAD ANONIMA", numero: 1, monto: "400000.00", facturas: 3},
		{proveedor: "ZETA LIMITADA", numero: 2, monto: "777000.00", facturas: 1},
	}
	for i, q := range quiere {
		g := got[i]
		if g.proveedor != q.proveedor || g.numero != q.numero {
			t.Errorf("remesa %d = %s nº%d, quiere %s nº%d", i, g.proveedor, g.numero, q.proveedor, q.numero)
		}
		// LA cifra que importa: es lo que el banco va a debitar. 100000 + 250000,50 + 49999,50.
		if g.monto != q.monto {
			t.Errorf("%s: monto %s, quiere %s — el banco debitaría una cifra y la conciliación esperaría otra",
				q.proveedor, g.monto, q.monto)
		}
		if g.facturas != q.facturas {
			t.Errorf("%s cubre %d factura(s), quiere %d", q.proveedor, g.facturas, q.facturas)
		}
		if !strings.HasPrefix(g.huella, "CXPR-") || len(g.huella) != len("CXPR-")+12 {
			t.Errorf("%s: código %q — tiene que ser CXPR- y 12 más, o el lector no lo distingue de una factura",
				q.proveedor, g.huella)
		}
	}
	if got[0].huella == got[1].huella {
		t.Error("las dos remesas comparten código: el pago de una cerraría las facturas de la otra")
	}

	t.Run("el total del lote sigue siendo la suma de todo", func(t *testing.T) {
		if lote.TotalCRC != "1177000.00" {
			t.Errorf("total del lote = %s, quiere 1177000.00 (400000 + 777000)", lote.TotalCRC)
		}
	})
}

// La macro es lo que de verdad mueve la plata: una línea = una transferencia. Si el lote dice «un
// pago por proveedor» y la macro sigue sacando una línea por factura, el modo no sirvió para nada
// y el banco cobra las mismas comisiones.
func TestLaMacroDeUnLotePorProveedorTraeUnaLineaPorProveedor(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	proveedorExtra(t, repo, provUno, "ALFA SOCIEDAD ANONIMA")
	proveedorExtra(t, repo, provDos, "ZETA LIMITADA")

	a1 := facturaDe(t, repo, provUno, "F-A1", "100000")
	a2 := facturaDe(t, repo, provUno, "F-A2", "250000.50")
	a3 := facturaDe(t, repo, provUno, "F-A3", "49999.50")
	z1 := facturaDe(t, repo, provDos, "F-Z1", "777000")

	lote, err := repo.CrearLote(ctx, empresaLote, "2026-10-02", ModoProveedor,
		[]string{a1, a2, a3, z1}, usuarioLote)
	if err != nil {
		t.Fatalf("crear lote: %v", err)
	}

	filas, err := repo.DocumentosParaPagoPorLote(ctx, empresaLote, lote.ID)
	if err != nil {
		t.Fatalf("macro del lote: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("la macro trae %d línea(s), se esperaban 2 (una por proveedor): %+v", len(filas), filas)
	}

	// ALFA primero (orden por número de remesa, que sale del nombre).
	alfa, zeta := filas[0], filas[1]
	if alfa.Nombre != "ALFA SOCIEDAD ANONIMA" || zeta.Nombre != "ZETA LIMITADA" {
		t.Fatalf("orden inesperado: %q, %q", alfa.Nombre, zeta.Nombre)
	}
	// LA cifra: tres facturas en un solo débito.
	if alfa.MontoNeto != "400000.00" {
		t.Errorf("ALFA cobra %s en una transferencia, se esperaba 400000.00", alfa.MontoNeto)
	}
	if zeta.MontoNeto != "777000.00" {
		t.Errorf("ZETA cobra %s, se esperaba 777000.00", zeta.MontoNeto)
	}

	// Y lo que viaja escrito al banco. La referencia legible va primero y el código al final: así,
	// si el banco truncara el campo, se pierde el automatismo pero no el sentido del pago.
	desc := descripcionDePago(alfa)
	if !strings.HasPrefix(desc, "REM ") {
		t.Errorf("la descripción %q no arranca con la referencia legible de la remesa", desc)
	}
	// Termina con el código interno tal como VIAJA (sin guion: el banco lo borra igual), no con
	// la forma canónica que guarda la base.
	if !strings.HasSuffix(desc, ParaElBanco(alfa.Descripcion)) {
		t.Errorf("la descripción %q no termina con el código interno %q", desc, ParaElBanco(alfa.Descripcion))
	}
	// Y tiene que poder volver a leerse, que es de lo que depende la conciliación.
	h, ok := extraerHuella(desc)
	if !ok || h != alfa.Descripcion || !EsRemesa(h) {
		t.Errorf("extraerHuella(%q) = (%q, %v, remesa=%v): el pago no se podría conciliar",
			desc, h, ok, EsRemesa(h))
	}
}

// Una factura bloqueada para pago puede terminar dentro de un lote (la asignación filtra por estado,
// no por el bloqueo) y la macro NO la paga. Si la remesa la hubiera sumado, el banco debitaría
// menos de lo que la remesa declara y la conciliación marcaría «monto distinto» para siempre.
func TestLaRemesaNoSumaLoQueLaMacroNoPaga(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	proveedorExtra(t, repo, provUno, "ALFA SOCIEDAD ANONIMA")

	buena := facturaDe(t, repo, provUno, "F-OK", "100000")
	bloqueada := facturaDe(t, repo, provUno, "F-BLOQ", "999999")
	if _, err := repo.pool.Exec(ctx,
		`UPDATE documento_cxp SET bloqueado_para_pago = true, bloqueo_motivo = 'viático' WHERE id = $1::uuid`,
		bloqueada); err != nil {
		t.Fatalf("bloquear: %v", err)
	}

	lote, err := repo.CrearLote(ctx, empresaLote, "2026-10-02", ModoProveedor,
		[]string{buena, bloqueada}, usuarioLote)
	if err != nil {
		t.Fatalf("crear lote: %v", err)
	}

	var monto string
	if err := repo.pool.QueryRow(ctx,
		`SELECT monto_crc::text FROM remesa_pago WHERE lote_id = $1::uuid`, lote.ID).Scan(&monto); err != nil {
		t.Fatalf("leer remesa: %v", err)
	}
	if monto != "100000.00" {
		t.Errorf("la remesa declara %s y la macro solo paga 100000.00: el banco y el sistema dirían cosas distintas", monto)
	}

	filas, err := repo.DocumentosParaPagoPorLote(ctx, empresaLote, lote.ID)
	if err != nil {
		t.Fatalf("macro: %v", err)
	}
	if len(filas) != 1 || filas[0].MontoNeto != monto {
		t.Fatalf("la macro y la remesa tienen que decir lo mismo: macro %+v, remesa %s", filas, monto)
	}

	// Y la bloqueada no queda marcada como cubierta por una remesa que no la pagó.
	var conRemesa bool
	if err := repo.pool.QueryRow(ctx,
		`SELECT remesa_id IS NOT NULL FROM documento_cxp WHERE id = $1::uuid`, bloqueada).Scan(&conRemesa); err != nil {
		t.Fatalf("leer la bloqueada: %v", err)
	}
	if conRemesa {
		t.Error("la factura bloqueada quedó apuntando a una remesa que no la sumó: diría que se pagó")
	}
}

func TestElModoPorFacturaNoCreaRemesas(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	proveedorExtra(t, repo, provUno, "ALFA SOCIEDAD ANONIMA")

	// Dos facturas del MISMO proveedor, pero cortadas de la forma de siempre.
	a1 := facturaDe(t, repo, provUno, "F-A1", "100000")
	a2 := facturaDe(t, repo, provUno, "F-A2", "200000")

	lote, err := repo.CrearLote(ctx, empresaLote, "2026-10-02", ModoFactura, []string{a1, a2}, usuarioLote)
	if err != nil {
		t.Fatalf("crear lote: %v", err)
	}
	if lote.ModoPago != ModoFactura {
		t.Errorf("modo_pago = %q, quiere %q", lote.ModoPago, ModoFactura)
	}
	var remesas, conRemesa int
	if err := repo.pool.QueryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM remesa_pago WHERE lote_id = $1::uuid),
		        (SELECT COUNT(*) FROM documento_cxp WHERE lote_id = $1::uuid AND remesa_id IS NOT NULL)`,
		lote.ID).Scan(&remesas, &conRemesa); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if remesas != 0 || conRemesa != 0 {
		t.Errorf("el modo por factura creó %d remesa(s) y marcó %d factura(s): tiene que pagar una por una",
			remesas, conRemesa)
	}
}

func TestUnModoDesconocidoNoSeTomaComoElDeSiempre(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	proveedorExtra(t, repo, provUno, "ALFA SOCIEDAD ANONIMA")
	a1 := facturaDe(t, repo, provUno, "F-A1", "100000")

	// Un modo que no se entiende NO puede caer en el default silencioso: decide cuánta plata sale
	// en cada transferencia. Si «PROVEDOR» (mal escrito) se tratara como «FACTURA», el corte
	// saldría partido en N pagos cuando se pidió uno solo, y nadie lo sabría hasta ver el banco.
	if _, err := repo.CrearLote(ctx, empresaLote, "2026-10-02", "PROVEDOR", []string{a1}, usuarioLote); err == nil {
		t.Fatal("un modo mal escrito fue aceptado")
	}
	var lotes int
	if err := repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM lote_pago`).Scan(&lotes); err != nil {
		t.Fatalf("contar lotes: %v", err)
	}
	if lotes != 0 {
		t.Errorf("quedaron %d lote(s): el rechazo tiene que ser ANTES de escribir nada", lotes)
	}
}
