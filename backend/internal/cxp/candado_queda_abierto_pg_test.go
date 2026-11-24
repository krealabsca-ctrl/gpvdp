package cxp

// Que liberar no haya que repetirlo todos los meses (4-nov-2026), contra PostgreSQL de verdad.
//
// EL PEDIDO, en palabras del Director Financiero: «cuando una factura ya fue liberada, debería ya
// sea por un botón o guardar el comportamiento de que las próximas deben venir liberadas por el
// ajuste brindado. Ya que de lo contrario siempre hay que irlas a liberar y ya es algo de la
// cotidianidad del proveedor».
//
// Liberar arreglaba lo que ya estaba trancado y nada más: la factura siguiente del mismo proveedor
// nacía igual, porque el candado se decide en el INSERT mirando la FICHA. La prueba de que el
// arreglo funciona no es que la ficha haya cambiado —eso lo diría un UPDATE— sino que la SIGUIENTE
// factura nazca libre, y eso solo lo puede demostrar la base.

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

const provCotidiano = "d1d1d1d1-d1d1-4d1d-9d1d-d1d1d1d1d1d1"

// unProveedorDeContado siembra la ficha tal como están 572 de las 573 reales: contado, sin plazo y
// creyéndole al comprobante.
func unProveedorDeContado(t *testing.T) *pgRepository {
	t.Helper()
	repo := baseDeLotes(t)
	if _, err := repo.pool.Exec(context.Background(),
		`INSERT INTO proveedor (id, empresa_id, nombre, condicion_pago, plazo_credito_dias, convenio_manda)
		 VALUES ($1::uuid, $2::uuid, 'ESTACION DE SERVICIO', 'CONTADO', 0, 'XML')`,
		provCotidiano, empresaLote); err != nil {
		t.Fatalf("sembrar: %v", err)
	}
	return repo
}

// contadoConVencimiento crea la factura como la deja el importador cuando el comprobante dice
// «contado»: pide bloquearla, y trae (o no) el vencimiento que declaró el emisor.
func contadoConVencimiento(t *testing.T, repo *pgRepository, clave, vencimientoXML string) Documento {
	t.Helper()
	total := decimal.RequireFromString("48500")
	doc, err := repo.CrearDocumento(context.Background(), empresaLote, DocumentoInput{
		ProveedorID:       provCotidiano,
		Clave:             clave,
		Consecutivo:       clave,
		FechaEmision:      "2026-11-04",
		Moneda:            "CRC",
		Subtotal:          total,
		Total:             total,
		Vencimiento:       vencimientoXML,
		BloqueadoParaPago: true,
		BloqueoMotivo:     "contado: confirmar si ya se pagó (caja chica, tarjeta o efectivo)",
	}, total, nil, usuarioLote)
	if err != nil {
		t.Fatalf("crear documento: %v", err)
	}
	return doc
}

// EL PEDIDO, DE PUNTA A PUNTA: después de configurar, la próxima nace libre.
func TestDespuesDeConfigurarLaSiguienteFacturaNaceLibre(t *testing.T) {
	repo := unProveedorDeContado(t)
	ctx := context.Background()

	// La de hoy nace trancada: es el estado del que parte el pedido.
	antes := contadoConVencimiento(t, repo, "50601000000000000000000000000000000000000000000201", "")
	if !antes.BloqueadoParaPago {
		t.Fatal("la primera no nació bloqueada: la prueba no estaría probando nada")
	}

	cambio, err := repo.ConfigurarProveedorSinCandado(ctx, empresaLote, provCotidiano)
	if err != nil {
		t.Fatalf("configurar: %v", err)
	}
	if !cambio {
		t.Fatal("dijo que no cambió nada sobre una ficha que estaba en XML/CONTADO")
	}

	// La del mes que viene ya no.
	despues := contadoConVencimiento(t, repo, "50601000000000000000000000000000000000000000000202", "")
	if despues.BloqueadoParaPago {
		t.Error("la siguiente volvió a nacer bloqueada: el ajuste no se guardó donde se decide el candado")
	}
	if despues.BloqueoMotivo != "" {
		t.Errorf("motivo de bloqueo = %q sobre una factura libre", despues.BloqueoMotivo)
	}
}

// CONFIGURAR NO PUEDE PISAR EL VENCIMIENTO DEL COMPROBANTE.
//
// Es el efecto lateral que casi se cuela: dejar la ficha en CRÉDITO para quitar el candado también
// la mete en la rama «manda el convenio», y con plazo 0 el vencimiento quedaba en el día de la
// emisión, tirando el que traía el comprobante. Muerde justo donde duele: de las 3.092 facturas de
// proveedores de contado, 45 vencen DESPUÉS de la emisión (15,6 días en promedio) y son
// exactamente las que alguien libera. Nacerían vencidas el mismo día.
func TestConfigurarNoPisaElVencimientoDelComprobante(t *testing.T) {
	repo := unProveedorDeContado(t)

	if _, err := repo.ConfigurarProveedorSinCandado(context.Background(), empresaLote, provCotidiano); err != nil {
		t.Fatalf("configurar: %v", err)
	}

	doc := contadoConVencimiento(t, repo, "50601000000000000000000000000000000000000000000203", "2026-11-20")
	if doc.FechaVencimiento == nil {
		t.Fatal("quedó sin vencimiento habiendo uno en el comprobante")
	}
	if *doc.FechaVencimiento != "2026-11-20" {
		t.Errorf("vencimiento = %q, quería el del comprobante (2026-11-20); sin plazo pactado en la "+
			"ficha no hay convenio que mande, y emisión + 0 la haría nacer vencida", *doc.FechaVencimiento)
	}
}

// Configurar dos veces no es un cambio dos veces: el llamador no debe anunciar lo que no pasó.
func TestConfigurarDosVecesNoVuelveAAnunciarCambio(t *testing.T) {
	repo := unProveedorDeContado(t)
	ctx := context.Background()

	if cambio, err := repo.ConfigurarProveedorSinCandado(ctx, empresaLote, provCotidiano); err != nil || !cambio {
		t.Fatalf("primera: cambio=%v err=%v", cambio, err)
	}
	cambio, err := repo.ConfigurarProveedorSinCandado(ctx, empresaLote, provCotidiano)
	if err != nil {
		t.Fatalf("segunda: %v", err)
	}
	if cambio {
		t.Error("la segunda dijo que cambió la ficha: el aviso diría que se configuró algo que ya estaba")
	}
}

// NO ALCANZA A OTRA EMPRESA. La ficha se toca por id, y sin el filtro de empresa el id de un
// proveedor de otra empresa quedaría configurado desde acá.
func TestConfigurarNoAlcanzaAOtraEmpresa(t *testing.T) {
	repo := unProveedorDeContado(t)
	const otraEmpresa = "ee000000-0000-4000-a000-000000000099"
	ctx := context.Background()

	// El proveedor existe y está sembrado; lo único distinto es la empresa desde la que se pide.
	cambio, err := repo.ConfigurarProveedorSinCandado(ctx, otraEmpresa, provCotidiano)
	if err != nil {
		t.Fatalf("configurar: %v", err)
	}
	if cambio {
		t.Fatal("configuró desde otra empresa la ficha de un proveedor ajeno")
	}
	var manda string
	if err := repo.pool.QueryRow(ctx,
		`SELECT convenio_manda FROM proveedor WHERE id = $1::uuid`, provCotidiano).Scan(&manda); err != nil {
		t.Fatal(err)
	}
	if manda != "XML" {
		t.Errorf("la ficha quedó en %q: se tocó desde otra empresa", manda)
	}
}
