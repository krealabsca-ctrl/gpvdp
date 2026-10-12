package bancos

// «¿A qué factura corresponde este pago?» — 30 de setiembre de 2026.
//
// El Director Financiero corrió dos pagos reales y no pudo relacionar ni uno con su factura. El
// enlace existía: `movimiento_bancario.documento_cxp_id` se escribe desde la migración 0043 cuando
// la conciliación por huella empareja un débito con su pago de CxP. Lo que no existía era una
// pantalla que lo mostrara — en Bancos se veía un débito con un código ilegible, y en CxP una
// factura «pagada» sin decir con cuál movimiento.
//
// Esta prueba fija el cableado: si el enlace está en la base, llega a la fila; y si no está, la
// fila no inventa uno. Lo segundo importa tanto como lo primero: un «pagado» de adorno en una fila
// sin enlazar es peor que el blanco, porque se cree.

import (
	"context"
	"testing"
)

const (
	provDelPago = "b0bb0000-b0bb-4bbb-8bbb-b0bb0000b0bb"
	docDelPago  = "fac00000-fac0-4ac0-8ac0-fac00000fac0"
)

func TestElMovimientoDiceQueFacturaPago(t *testing.T) {
	repo, _, _ := baseMiPartida(t)
	ctx := context.Background()

	// La factura y su proveedor, como quedarían después de un pago conciliado.
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO proveedor (id, empresa_id, nombre, identificacion)
		  VALUES ($1::uuid, $2::uuid, 'AR SISTEMAS SOCIEDAD ANONIMA', '3101123456')`,
			[]any{provDelPago, empresaPrueba}},
		{`INSERT INTO documento_cxp
			(id, empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado,
			 moneda, total, total_crc, huella)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'CLAVE-487', '00100001010000000487',
		          '2026-09-01'::date, 'CONCILIADO', 'CRC', 884614.85, 884614.85, 'CXP-54A4281AE49A')`,
			[]any{docDelPago, empresaPrueba, provDelPago}},
	} {
		if _, err := repo.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("sembrar la factura pagada: %v", err)
		}
	}

	// El débito, con la descripción TAL COMO vuelve del banco (sin el guion de la huella).
	const descBanco = "DEBITO MASIVO SINPE ( - 1060746795 Pagar Proveedores Nuevo FC 000487 " +
		"CXP54A4281AE49A Lote 888607 Linea 21 [FC 000487 CXP54A4281AE49A])"
	var idPago string
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
			 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
			 estado_clasificacion, natural_key, incluido, documento_cxp_id)
		VALUES ($1::uuid, $2::uuid, '2026-12-18'::date, 'DOC-pago', $3, 884614.85, 0,
		        'CRC', 884614.85, 884614.85, $4::uuid, $5::uuid, 'REVISADO', 'cl-pago', true, $6::uuid)
		RETURNING id::text`,
		empresaPrueba, cuentaBuena, descBanco, conceptoID, partidaMia, docDelPago).Scan(&idPago); err != nil {
		t.Fatalf("sembrar el pago: %v", err)
	}

	// La hoja de trabajo de Bancos, que es donde Tesorería mira los débitos. No «Mi partida»: esa
	// lista CRÉDITOS de un segmento, y un pago a proveedor es un débito.
	res, err := repo.ListarMovimientos(ctx, empresaPrueba, FiltrosMovimientos{PageSize: 200})
	if err != nil {
		t.Fatalf("listar movimientos: %v", err)
	}

	porID := map[string]MovimientoRow{}
	for _, it := range res.Items {
		porID[it.ID] = it
	}

	pago, ok := porID[idPago]
	if !ok {
		t.Fatal("control: el movimiento del pago no está en la lista")
	}
	if pago.PagoCxP == nil {
		t.Fatal("el movimiento está enlazado a una factura en la base y la fila no lo dice: " +
			"es justo el dato que faltaba para conciliar")
	}
	casos := []struct{ campo, got, quiere string }{
		{"documento_id", pago.PagoCxP.DocumentoID, docDelPago},
		{"consecutivo", pago.PagoCxP.Consecutivo, "00100001010000000487"},
		{"proveedor", pago.PagoCxP.Proveedor, "AR SISTEMAS SOCIEDAD ANONIMA"},
		// El código interno: el MISMO que viaja en la descripción del banco, con su guion.
		{"huella", pago.PagoCxP.Huella, "CXP-54A4281AE49A"},
	}
	for _, c := range casos {
		if c.got != c.quiere {
			t.Errorf("%s = %q, quiere %q", c.campo, c.got, c.quiere)
		}
	}

	t.Run("el pago de una sola factura se cuenta como una", func(t *testing.T) {
		if pago.PagoCxP.Facturas != 1 {
			t.Errorf("facturas = %d, quiere 1", pago.PagoCxP.Facturas)
		}
	})

	t.Run("un movimiento sin enlace no inventa una factura", func(t *testing.T) {
		sinEnlace := 0
		for _, it := range res.Items {
			if it.ID == idPago {
				continue
			}
			sinEnlace++
			if it.PagoCxP != nil {
				t.Errorf("%s dice haber pagado la factura %q y no está enlazado a ninguna",
					it.ID, it.PagoCxP.Consecutivo)
			}
		}
		if sinEnlace == 0 {
			t.Fatal("control: no había ninguna fila sin enlace con la que comparar")
		}
	})
}
