package cxp

// De dónde sale el vencimiento: del XML o del convenio (mig 0095, 9-oct-2026).
//
// «Hay proveedores que en el XML su plazo por defecto es 90 días, pero el crédito real en las
// facturas o por convenio es otro, y eso nos está malogrando la data. Hay que validar y
// permitirnos configurar por proveedor si la fecha de vencimiento aplica la del XML o bien la que
// uno configuró en cada proveedor.»
//
// La regla vive en el INSERT de `CrearDocumento`, así que se prueba contra PostgreSQL: es un CASE
// dentro de un SQL y un doble no puede demostrar qué fecha queda escrita.

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

const (
	provXML  = "aa111111-1111-4111-a111-111111111111"
	provConv = "aa222222-2222-4222-a222-222222222222"
)

// dosProveedores siembra uno que respeta el XML y otro con el convenio por delante. Los dos
// declaran 30 días de crédito en su ficha.
func dosProveedores(t *testing.T) *pgRepository {
	t.Helper()
	repo := baseDeLotes(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := repo.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar: %v", err)
		}
	}
	exec(`INSERT INTO proveedor (id, empresa_id, nombre, condicion_pago, plazo_credito_dias, convenio_manda)
	      VALUES ($1::uuid, $2::uuid, 'PROVEEDOR QUE DECLARA 90', 'CREDITO', 30, 'XML')`, provXML, empresaLote)
	exec(`INSERT INTO proveedor (id, empresa_id, nombre, condicion_pago, plazo_credito_dias, convenio_manda)
	      VALUES ($1::uuid, $2::uuid, 'PROVEEDOR CON CONVENIO', 'CREDITO', 30, 'PROVEEDOR')`, provConv, empresaLote)
	return repo
}

// crearConVencimiento mete una factura con el vencimiento que trae el comprobante y devuelve el
// que quedó escrito.
func crearConVencimiento(t *testing.T, r *pgRepository, provID, emision, vencimientoXML, clave string) string {
	t.Helper()
	ctx := context.Background()
	in := DocumentoInput{
		ProveedorID:  provID,
		Clave:        clave,
		Consecutivo:  clave,
		FechaEmision: emision,
		Moneda:       "CRC",
		Subtotal:     decimal.NewFromInt(1000),
		IVA:          decimal.Zero,
		Retencion:    decimal.Zero,
		Total:        decimal.NewFromInt(1000),
		Vencimiento:  vencimientoXML,
	}
	if _, err := r.CrearDocumento(ctx, empresaLote, in, decimal.NewFromInt(1000), nil, usuarioLote); err != nil {
		t.Fatalf("crear documento: %v", err)
	}
	var venc string
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(fecha_vencimiento::text, '') FROM documento_cxp
		 WHERE empresa_id = $1::uuid AND clave = $2`, empresaLote, clave).Scan(&venc); err != nil {
		t.Fatalf("leer vencimiento: %v", err)
	}
	return venc
}

// Con 'XML' manda el comprobante, que es como funcionó siempre.
func TestConXMLMandaElComprobante(t *testing.T) {
	repo := dosProveedores(t)
	// El XML declara 90 días; la ficha dice 30.
	venc := crearConVencimiento(t, repo, provXML, "2026-09-01", "2026-11-30", "XML-90")
	if venc != "2026-11-30" {
		t.Errorf("el vencimiento quedó en %q y se esperaba el del XML (2026-11-30)", venc)
	}
}

// Con 'PROVEEDOR' manda el convenio AUNQUE el comprobante traiga otro. Es el caso del pedido.
func TestConProveedorMandaElConvenio(t *testing.T) {
	repo := dosProveedores(t)
	// Mismo XML a 90 días, pero este proveedor tiene el convenio por delante: 1-set + 30 días.
	venc := crearConVencimiento(t, repo, provConv, "2026-09-01", "2026-11-30", "CONV-30")
	if venc != "2026-10-01" {
		t.Errorf("el vencimiento quedó en %q y se esperaba el del convenio (2026-10-01, "+
			"emisión + 30 días); el plazo de 90 del comprobante volvió a ganar", venc)
	}
}

// Con 'XML' y SIN vencimiento en el comprobante, se cae al plazo de la ficha. Es el
// comportamiento de siempre y no puede perderse al agregar la opción.
func TestSinVencimientoEnElXMLSeUsaElPlazoDeLaFicha(t *testing.T) {
	repo := dosProveedores(t)
	venc := crearConVencimiento(t, repo, provXML, "2026-09-01", "", "XML-SIN")
	if venc != "2026-10-01" {
		t.Errorf("sin vencimiento en el XML quedó %q, se esperaba emisión + 30 (2026-10-01)", venc)
	}
}

// Un proveedor de CONTADO no tiene vencimiento, mande quien mande: no hay plazo que aplicar.
func TestUnProveedorDeContadoNoTieneVencimiento(t *testing.T) {
	repo := dosProveedores(t)
	ctx := context.Background()
	const provContado = "aa333333-3333-4333-a333-333333333333"
	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO proveedor (id, empresa_id, nombre, condicion_pago, plazo_credito_dias, convenio_manda)
		 VALUES ($1::uuid, $2::uuid, 'DE CONTADO', 'CONTADO', 0, 'PROVEEDOR')`,
		provContado, empresaLote); err != nil {
		t.Fatal(err)
	}
	if venc := crearConVencimiento(t, repo, provContado, "2026-09-01", "", "CONTADO-1"); venc != "" {
		t.Errorf("un proveedor de contado quedó con vencimiento %q", venc)
	}
}

// El default de la columna es 'XML': la migración no puede haber movido ningún vencimiento de los
// 4.542 documentos que ya existen.
func TestElDefaultEsXMLYNoMueveNada(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	const prov = "aa444444-4444-4444-a444-444444444444"
	// Se crea SIN decir convenio_manda: tiene que nacer en 'XML'.
	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO proveedor (id, empresa_id, nombre, condicion_pago, plazo_credito_dias)
		 VALUES ($1::uuid, $2::uuid, 'SIN CONFIGURAR', 'CREDITO', 30)`, prov, empresaLote); err != nil {
		t.Fatal(err)
	}
	var manda string
	if err := repo.pool.QueryRow(ctx,
		`SELECT convenio_manda FROM proveedor WHERE id = $1::uuid`, prov).Scan(&manda); err != nil {
		t.Fatal(err)
	}
	if manda != "XML" {
		t.Fatalf("un proveedor nuevo nació con convenio_manda = %q; el default tiene que ser XML para "+
			"que la migración no recalcule en silencio los vencimientos de toda la cartera", manda)
	}
	if venc := crearConVencimiento(t, repo, prov, "2026-09-01", "2026-11-30", "DEF-1"); venc != "2026-11-30" {
		t.Errorf("con el default, el vencimiento quedó en %q en vez del que trae el XML", venc)
	}
}
