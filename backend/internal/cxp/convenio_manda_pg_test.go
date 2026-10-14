package cxp

// El convenio del proveedor manda sobre lo que declare el comprobante (mig 0097), contra
// PostgreSQL de verdad.
//
// EL CASO, en palabras del Director Financiero (12-oct-2026): hay proveedores que ponen «contado»
// en el comprobante aunque el convenio real sea a crédito. La factura nace BLOQUEADA para pago
// —un candado contra el doble pago, porque una compra de contado suele estar ya pagada por caja
// chica— y queda trancada: «quedaría sucio el sistema y facturas sin pagar cuando deben pagarse».
//
// La decisión vive en el INSERT y no en el importador A PROPÓSITO: así vale igual para el XML y
// para el Excel, sin repetir la regla en dos caminos que después se desincronizan. Por eso se
// prueba acá, contra la base, y no con un doble del importador.

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

const (
	provConvenio = "c1c1c1c1-c1c1-4c1c-9c1c-c1c1c1c1c1c1"
	provSoloXML  = "c2c2c2c2-c2c2-4c2c-9c2c-c2c2c2c2c2c2"
)

// baseDeConvenio siembra dos proveedores que solo difieren en a quién se le cree.
func baseDeConvenio(t *testing.T) *pgRepository {
	t.Helper()
	repo := baseDeLotes(t)
	ctx := context.Background()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := repo.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar: %v", err)
		}
	}
	// El del convenio: su ficha dice CRÉDITO a 8 días y manda la ficha.
	exec(`INSERT INTO proveedor (id, empresa_id, nombre, condicion_pago, plazo_credito_dias, convenio_manda)
	      VALUES ($1::uuid, $2::uuid, 'FERRETERIA CON CONVENIO', 'CREDITO', 8, 'PROVEEDOR')`,
		provConvenio, empresaLote)
	// El de siempre: manda el comprobante (el default).
	exec(`INSERT INTO proveedor (id, empresa_id, nombre, condicion_pago, plazo_credito_dias, convenio_manda)
	      VALUES ($1::uuid, $2::uuid, 'PULPERIA DE LA ESQUINA', 'CONTADO', 0, 'XML')`,
		provSoloXML, empresaLote)
	return repo
}

// crearContado crea una factura tal como la deja el importador cuando el comprobante dice
// «contado»: bloqueada para pago, con su motivo.
func crearContado(t *testing.T, repo *pgRepository, provID, clave string) Documento {
	t.Helper()
	total := decimal.RequireFromString("57200")
	doc, err := repo.CrearDocumento(context.Background(), empresaLote, DocumentoInput{
		ProveedorID:       provID,
		Clave:             clave,
		Consecutivo:       clave,
		FechaEmision:      "2026-09-22",
		Moneda:            "CRC",
		Subtotal:          total,
		Total:             total,
		BloqueadoParaPago: true,
		BloqueoMotivo:     "contado: confirmar si ya se pagó (caja chica, tarjeta o efectivo)",
	}, total, nil, usuarioLote)
	if err != nil {
		t.Fatalf("crear documento: %v", err)
	}
	return doc
}

// CON EL CONVENIO, LA FACTURA NO NACE TRANCADA.
//
// El emisor declaró «contado» y el importador pide bloquearla; la ficha dice que con este
// proveedor se compra a crédito, así que no hay vale de caja chica con el que se pueda duplicar
// el pago y el candado no corresponde.
func TestConElConvenioLaFacturaDeContadoNoNaceBloqueada(t *testing.T) {
	repo := baseDeConvenio(t)
	doc := crearContado(t, repo, provConvenio, "50601000000000000000000000000000000000000000000101")

	if doc.BloqueadoParaPago {
		t.Error("nació bloqueada: con convenio_manda = PROVEEDOR y ficha a CRÉDITO gana la ficha")
	}
	if doc.BloqueoMotivo != "" {
		t.Errorf("motivo de bloqueo = %q, quería vacío: si no está trancada, no hay por qué", doc.BloqueoMotivo)
	}
	// Y el vencimiento sale del convenio, que es la otra mitad del mismo switch: 22-09 + 8 días.
	if doc.FechaVencimiento == nil || *doc.FechaVencimiento != "2026-09-30" {
		t.Errorf("vencimiento = %v, quería 2026-09-30 (emisión + 8 días del convenio)", doc.FechaVencimiento)
	}
}

// SIN EL CONVENIO, TODO SIGUE COMO SIEMPRE.
//
// Es la mitad que protege: el candado existe por una razón —el gasto saldría dos veces por la
// puerta del banco— y solo se levanta donde alguien lo pidió proveedor por proveedor. Un cambio
// que lo apagara para todos haría pagables 3.506 facturas de 573 proveedores de contado.
func TestSinConvenioLaFacturaDeContadoSigueNaciendoBloqueada(t *testing.T) {
	repo := baseDeConvenio(t)
	doc := crearContado(t, repo, provSoloXML, "50601000000000000000000000000000000000000000000102")

	if !doc.BloqueadoParaPago {
		t.Fatal("nació libre: con convenio_manda = XML (el default) el candado tiene que seguir poniéndose")
	}
	if doc.BloqueoMotivo == "" {
		t.Error("quedó bloqueada sin motivo: nadie podría saber por qué no se puede pagar")
	}
}

// Un proveedor marcado «manda el convenio» pero que de verdad es de CONTADO no abre nada.
//
// El switch dice a quién creerle, no «no bloquees nunca». Si la ficha también dice contado, el
// comprobante y el convenio coinciden y el candado corresponde igual.
func TestElConvenioQueTambienDiceContadoNoLevantaElCandado(t *testing.T) {
	repo := baseDeConvenio(t)
	ctx := context.Background()

	if _, err := repo.pool.Exec(ctx,
		`UPDATE proveedor SET condicion_pago = 'CONTADO', convenio_manda = 'PROVEEDOR' WHERE id = $1::uuid`,
		provConvenio); err != nil {
		t.Fatalf("ajustar proveedor: %v", err)
	}
	doc := crearContado(t, repo, provConvenio, "50601000000000000000000000000000000000000000000103")
	if !doc.BloqueadoParaPago {
		t.Error("se levantó el candado de un proveedor que SÍ es de contado")
	}
}

// ── La huella: liberar lo que ya estaba trancado ──────────────────────────────────────────

func TestLiberarDejaHuellaYNoSeRepite(t *testing.T) {
	repo := baseDeConvenio(t)
	ctx := context.Background()
	a := crearContado(t, repo, provSoloXML, "50601000000000000000000000000000000000000000000111")
	b := crearContado(t, repo, provSoloXML, "50601000000000000000000000000000000000000000000112")

	// Se ven ANTES de liberar: firmar sobre una lista que no se vio es firmar en blanco.
	pendientes, err := repo.FacturasDeContado(ctx, empresaLote, provSoloXML)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(pendientes) != 2 {
		t.Fatalf("bloqueadas = %d, quería 2", len(pendientes))
	}

	const motivo = "El emisor declara contado por costumbre; el convenio firmado es a 8 días."
	libres, err := repo.LiberarParaPago(ctx, empresaLote, provSoloXML, []string{a.ID, b.ID}, motivo, usuarioLote)
	if err != nil {
		t.Fatalf("liberar: %v", err)
	}
	if len(libres) != 2 {
		t.Fatalf("liberadas = %d, quería 2", len(libres))
	}

	// LA HUELLA: quién, cuándo y por qué, más el motivo ORIGINAL del bloqueo, que es lo que
	// explica de qué se la liberó.
	var quien, motivoGuardado, motivoOriginal string
	var cuando *string
	if err := repo.pool.QueryRow(ctx,
		`SELECT desbloqueado_por::text, COALESCE(desbloqueo_motivo,''), desbloqueado_en::text,
		        COALESCE(bloqueo_motivo,'')
		   FROM documento_cxp WHERE id = $1::uuid`, a.ID).
		Scan(&quien, &motivoGuardado, &cuando, &motivoOriginal); err != nil {
		t.Fatalf("leer huella: %v", err)
	}
	if quien != usuarioLote {
		t.Errorf("desbloqueado_por = %q", quien)
	}
	if motivoGuardado != motivo {
		t.Errorf("motivo guardado = %q", motivoGuardado)
	}
	if cuando == nil {
		t.Error("sin fecha de liberación")
	}
	if motivoOriginal == "" {
		t.Error("se borró el motivo del bloqueo: sin él la huella dice quién la liberó pero no de qué")
	}

	// Liberar de nuevo no vuelve a escribir la huella: la primera autorización es la que vale.
	otra, err := repo.LiberarParaPago(ctx, empresaLote, provSoloXML, []string{a.ID}, "otro motivo cualquiera largo", usuarioLote)
	if err != nil {
		t.Fatalf("segunda liberación: %v", err)
	}
	if len(otra) != 0 {
		t.Error("volvió a liberar una factura ya libre: pisaría el motivo y el autor originales")
	}
}

// NO SE LIBERAN FACTURAS DE OTRO PROVEEDOR, aunque se manden sus ids en el cuerpo.
//
// La ruta cuelga de UN proveedor. Sin esta condición, quien pueda liberar las facturas de un
// proveedor cualquiera podría liberar las de todos desde ahí.
func TestLiberarNoAlcanzaAOtroProveedor(t *testing.T) {
	repo := baseDeConvenio(t)
	ajeno := crearContado(t, repo, provSoloXML, "50601000000000000000000000000000000000000000000121")

	libres, err := repo.LiberarParaPago(context.Background(), empresaLote, provConvenio,
		[]string{ajeno.ID}, "motivo suficientemente largo para pasar", usuarioLote)
	if err != nil {
		t.Fatalf("liberar: %v", err)
	}
	if len(libres) != 0 {
		t.Error("liberó una factura de otro proveedor desde la ficha de éste")
	}
}

// El motivo no es opcional, y «ok» no es un motivo.
func TestLiberarExigeUnMotivoDeVerdad(t *testing.T) {
	casos := []struct {
		nombre string
		motivo string
		vale   bool
	}{
		{"vacío", "", false},
		{"la respuesta refleja", "ok", false},
		{"sigue siendo refleja", "ya se pagó", false},
		{"una explicación", "El emisor declara contado pero el convenio es a 8 días.", true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := LiberacionInput{DocumentoIDs: []string{"x"}, Motivo: c.motivo}.Validar()
			if c.vale && err != nil {
				t.Errorf("motivo %q rechazado: %v", c.motivo, err)
			}
			if !c.vale && err == nil {
				t.Errorf("motivo %q aceptado: sin explicación, un doble pago no se puede reconstruir", c.motivo)
			}
		})
	}
}
