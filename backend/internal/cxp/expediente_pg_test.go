package cxp

// Los DOS documentos del expediente de un pago (mig 0103, 19-oct-2026).
//
// EL PEDIDO: «afuera en cada factura debería verse la factura original, el comprobante de pago,
// enviar el comprobante por correo o whatsapp, y demás».
//
// LA MITAD NO EXISTÍA. Medido al escribir esto: 4.542 facturas, 0 con su XML guardado, y
// `cxp_recepcion` —el buzón que lo conserva— con 0 filas. Todas entraron por Excel. Validar un
// pago es cruzar lo que el proveedor facturó contra lo que el banco debitó; con un solo papel, la
// pantalla muestra el pago pero no deja verificarlo.
//
// Va contra Postgres de verdad porque lo que se protege es el índice: la unicidad pasó de
// (documento) a (documento, tipo), y si quedara en la vieja, adjuntar la factura BORRARÍA el
// recibo del banco.

import (
	"context"
	"testing"
)

func docConEstado(t *testing.T, r *pgRepository, consecutivo, estado string) string {
	t.Helper()
	var id string
	if err := r.pool.QueryRow(context.Background(), `
		INSERT INTO documento_cxp
			(empresa_id, proveedor_id, clave, consecutivo, fecha_emision, estado, moneda,
			 total, total_crc, retencion)
		VALUES ($1::uuid,$2::uuid,$3,$3,'2026-09-01',$4,'CRC',50000,50000,0)
		RETURNING id::text`, empPC, provPC, consecutivo, estado).Scan(&id); err != nil {
		t.Fatalf("sembrar documento: %v", err)
	}
	return id
}

// LO CENTRAL: los dos conviven. Con el UNIQUE viejo, el segundo adjunto pisaba al primero.
func TestLaFacturaYElComprobanteConvivenEnLaMismaFactura(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()
	doc := docConEstado(t, repo, "001", EstPagado)

	if err := repo.GuardarComprobante(ctx, empPC, doc, AdjuntoComprobante,
		"recibo-banco.pdf", "application/pdf", []byte("%PDF recibo"), usrPC); err != nil {
		t.Fatalf("adjuntar comprobante: %v", err)
	}
	if err := repo.GuardarComprobante(ctx, empPC, doc, AdjuntoFactura,
		"factura-proveedor.pdf", "application/pdf", []byte("%PDF factura"), usrPC); err != nil {
		t.Fatalf("adjuntar factura: %v", err)
	}

	comp, err := repo.ObtenerComprobante(ctx, empPC, doc, AdjuntoComprobante)
	if err != nil {
		t.Fatalf("leer comprobante: %v", err)
	}
	if comp.Filename != "recibo-banco.pdf" {
		t.Errorf("el comprobante se perdió al adjuntar la factura: %q", comp.Filename)
	}
	fac, err := repo.ObtenerComprobante(ctx, empPC, doc, AdjuntoFactura)
	if err != nil {
		t.Fatalf("leer factura: %v", err)
	}
	if fac.Filename != "factura-proveedor.pdf" {
		t.Errorf("la factura = %q", fac.Filename)
	}
	// Y el contenido no se cruzó: cada uno trae SU archivo.
	if string(comp.Contenido) == string(fac.Contenido) {
		t.Error("los dos adjuntos devuelven el mismo contenido")
	}
}

// Reemplazar uno NO toca al otro.
func TestReemplazarUnDocumentoNoBorraElOtro(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()
	doc := docConEstado(t, repo, "001", EstPagado)

	_ = repo.GuardarComprobante(ctx, empPC, doc, AdjuntoComprobante, "v1.pdf", "application/pdf", []byte("a"), usrPC)
	_ = repo.GuardarComprobante(ctx, empPC, doc, AdjuntoFactura, "factura.pdf", "application/pdf", []byte("b"), usrPC)
	// Llega un recibo corregido.
	if err := repo.GuardarComprobante(ctx, empPC, doc, AdjuntoComprobante,
		"v2.pdf", "application/pdf", []byte("c"), usrPC); err != nil {
		t.Fatalf("reemplazar: %v", err)
	}

	comp, _ := repo.ObtenerComprobante(ctx, empPC, doc, AdjuntoComprobante)
	if comp.Filename != "v2.pdf" {
		t.Errorf("el comprobante no se reemplazó: %q", comp.Filename)
	}
	fac, err := repo.ObtenerComprobante(ctx, empPC, doc, AdjuntoFactura)
	if err != nil || fac.Filename != "factura.pdf" {
		t.Errorf("la factura se perdió al reemplazar el comprobante: %q (%v)", fac.Filename, err)
	}
}

// LA FACTURA SE ADJUNTA EN CUALQUIER ESTADO; el comprobante solo si ya se pagó.
//
// No es una excepción: son papeles de momentos distintos. La factura llega al principio y es lo
// que se revisa para decidir el pago — exigir que ya esté pagada deja sin respaldo justo el tramo
// donde se aprueba. El recibo del banco no existe antes de que el dinero salga.
func TestLaFacturaSeAdjuntaAntesDePagar(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()
	doc := docConEstado(t, repo, "001", EstRecibido)

	if err := repo.GuardarComprobante(ctx, empPC, doc, AdjuntoFactura,
		"factura.pdf", "application/pdf", []byte("x"), usrPC); err != nil {
		t.Fatalf("la factura tenía que poder adjuntarse sin estar pagada: %v", err)
	}
	// El recibo del banco, no: todavía no salió el dinero.
	if err := repo.GuardarComprobante(ctx, empPC, doc, AdjuntoComprobante,
		"recibo.pdf", "application/pdf", []byte("y"), usrPC); err != ErrDocNoPagado {
		t.Errorf("se aceptó un recibo de un pago que no ocurrió: %v", err)
	}
}

// Adjuntar la FACTURA no desmarca el comprobante como enviado.
//
// Esa marca significa «el proveedor ya tiene el recibo que está adjunto ahora». Adjuntar la
// factura no la vuelve mentira, y borrarla ahí haría aparecer como no enviado un comprobante que
// sí salió — y alguien lo reenviaría de más.
func TestAdjuntarLaFacturaNoDesmarcaElEnvio(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()
	doc := docConEstado(t, repo, "001", EstPagado)
	_ = repo.GuardarComprobante(ctx, empPC, doc, AdjuntoComprobante, "r.pdf", "application/pdf", []byte("a"), usrPC)
	if _, err := repo.pool.Exec(ctx,
		`UPDATE documento_cxp SET comprobante_enviado_en = now() WHERE id = $1::uuid`, doc); err != nil {
		t.Fatal(err)
	}

	if err := repo.GuardarComprobante(ctx, empPC, doc, AdjuntoFactura,
		"f.pdf", "application/pdf", []byte("b"), usrPC); err != nil {
		t.Fatalf("adjuntar factura: %v", err)
	}
	var enviado *string
	if err := repo.pool.QueryRow(ctx,
		`SELECT comprobante_enviado_en::text FROM documento_cxp WHERE id = $1::uuid`, doc).Scan(&enviado); err != nil {
		t.Fatal(err)
	}
	if enviado == nil {
		t.Error("adjuntar la factura borró la marca de enviado del COMPROBANTE")
	}

	// Pero reemplazar el COMPROBANTE sí la borra: ese PDF el proveedor no lo tiene.
	if err := repo.GuardarComprobante(ctx, empPC, doc, AdjuntoComprobante,
		"r2.pdf", "application/pdf", []byte("c"), usrPC); err != nil {
		t.Fatal(err)
	}
	if err := repo.pool.QueryRow(ctx,
		`SELECT comprobante_enviado_en::text FROM documento_cxp WHERE id = $1::uuid`, doc).Scan(&enviado); err != nil {
		t.Fatal(err)
	}
	if enviado != nil {
		t.Error("reemplazar el comprobante dejó la marca de enviado sobre un PDF que nadie recibió")
	}
}

// El listado NO duplica la factura que tiene los dos documentos.
//
// `documentoFrom` hace LEFT JOIN a `comprobante_pago`; desde que una factura puede tener dos
// filas ahí, un join sin filtrar por tipo devolvería la misma factura dos veces en la Bandeja.
func TestUnaFacturaConLosDosDocumentosNoSeDuplicaEnElListado(t *testing.T) {
	repo := baseDePorConfirmar(t)
	ctx := context.Background()
	doc := docConEstado(t, repo, "001", EstPagado)
	_ = repo.GuardarComprobante(ctx, empPC, doc, AdjuntoComprobante, "r.pdf", "application/pdf", []byte("a"), usrPC)
	_ = repo.GuardarComprobante(ctx, empPC, doc, AdjuntoFactura, "f.pdf", "application/pdf", []byte("b"), usrPC)

	lista, err := repo.ListarDocumentos(ctx, empPC, FiltrosDocumentos{})
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	vistas := 0
	for _, d := range lista.Items {
		if d.ID == doc {
			vistas++
		}
	}
	if vistas != 1 {
		t.Errorf("la factura aparece %d veces en el listado, quiere 1", vistas)
	}
	// Y el listado dice que tiene los dos.
	for _, d := range lista.Items {
		if d.ID != doc {
			continue
		}
		if !d.TieneComprobante || !d.TieneFactura {
			t.Errorf("el expediente no reporta los dos documentos: comprobante=%v factura=%v",
				d.TieneComprobante, d.TieneFactura)
		}
	}
}
