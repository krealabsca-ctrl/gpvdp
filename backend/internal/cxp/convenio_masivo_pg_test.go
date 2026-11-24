package cxp

// «A este proveedor le creemos a la ficha, no al comprobante» (21-oct-2026).
//
// EL PEDIDO: «hay proveedores que son así y es tedioso cada factura ejecutar el mismo proceso de
// limpia; lo mejor es que exista un botón del proveedor que a pesar de que dice contado es a
// crédito».
//
// LA REGLA YA EXISTÍA Y NADIE LA TENÍA PUESTA. Está en el INSERT del documento desde la migración
// 0097, y al medirlo: 0 de 649 proveedores. No porque esté mal hecha, sino porque el campo se
// llama «El vencimiento lo manda» y no dice que también gobierna el candado del contado.
//
// ACÁ NO SE PRUEBA LA REGLA, A PROPÓSITO. Que la factura de contado no nazca bloqueada con el
// convenio puesto ya está probado contra la base en `convenio_manda_pg_test.go`, con sus tres
// casos (con convenio, sin convenio, y la ficha que también dice contado). Repetirlo acá sería
// una segunda copia que se va a desincronizar de la primera.
//
// Lo que se prueba acá es lo que SÍ es nuevo: el marcado en lote, el aviso de lo que queda a
// medias, y que se vea cuánto se liberaría antes de liberarlo.

import (
	"context"
	"errors"
	"testing"
)

// proveedorCon siembra un proveedor con su condición y su convenio.
func proveedorCon(t *testing.T, repo *pgRepository, id, nombre, condicion, convenio string) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO proveedor (id, empresa_id, nombre, identificacion, iban, condicion_pago, convenio_manda)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)`,
		id, empresaLote, nombre, "3101"+nombre, "CR"+nombre, condicion, convenio); err != nil {
		t.Fatalf("sembrar proveedor %s: %v", nombre, err)
	}
}

// LA TRAMPA: marcar el convenio con la ficha en CONTADO no activa nada, y hay que decirlo.
//
// Sin este aviso, quien marca el proveedor cree que lo resolvió y lo descubre dentro de un mes con
// la factura trancada. Es el camino de quien DESMARCA «pasarlos a crédito»: ahí el trabajo queda a
// medias a propósito, y el aviso sigue siendo lo correcto.
func TestMarcarElConvenioAvisaSiLaFichaDiceContado(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	const bueno = "77777777-7777-4777-8777-777777777777"
	const aMedias = "88888888-8888-4888-8888-888888888888"
	proveedorCon(t, repo, bueno, "EnCredito", "CREDITO", "XML")
	proveedorCon(t, repo, aMedias, "EnContado", "CONTADO", "XML")

	res, err := repo.MarcarConvenioMasivo(ctx, empresaLote, []string{bueno, aMedias}, "PROVEEDOR", false)
	if err != nil {
		t.Fatalf("marcar: %v", err)
	}
	if res.Actualizados != 2 {
		t.Errorf("actualizados = %d, esperaba 2", res.Actualizados)
	}
	if len(res.SinCredito) != 1 || res.SinCredito[0] != "EnContado" {
		t.Errorf("no se avisó del proveedor cuya ficha dice CONTADO (la regla no se le activa): %v", res.SinCredito)
	}
	if len(res.PasadosACredito) != 0 {
		t.Errorf("pasó a crédito sin que se lo pidieran: %v", res.PasadosACredito)
	}
	// Y la ficha NO se tocó: desmarcar tiene que significar algo.
	var cond string
	if err := repo.pool.QueryRow(ctx,
		`SELECT condicion_pago FROM proveedor WHERE id = $1::uuid`, aMedias).Scan(&cond); err != nil {
		t.Fatal(err)
	}
	if cond != "CONTADO" {
		t.Errorf("la ficha quedó en %q sin haberlo pedido", cond)
	}
}

// EL PEDIDO DE «IGUALALO» (4-nov-2026): el botón del lote tiene que dejar el trabajo TERMINADO.
//
// Marcar solo el convenio no desactiva el candado —hacen falta las dos condiciones— y 573 de 649
// proveedores están en CONTADO: el camino dominante terminaba con un aviso de «pasalos a crédito»
// que había que ir a hacer a mano, ficha por ficha. Es el mismo trabajo tedioso que el panel venía
// a eliminar, un nivel más arriba.
func TestMarcarEnLoteDejaLasDosCondicionesPuestas(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	const yaEstaba = "77777777-7777-4777-8777-777777777777"
	const enContado = "88888888-8888-4888-8888-888888888888"
	proveedorCon(t, repo, yaEstaba, "EnCredito", "CREDITO", "XML")
	proveedorCon(t, repo, enContado, "EnContado", "CONTADO", "XML")

	res, err := repo.MarcarConvenioMasivo(ctx, empresaLote, []string{yaEstaba, enContado}, "PROVEEDOR", true)
	if err != nil {
		t.Fatalf("marcar: %v", err)
	}
	if res.Actualizados != 2 {
		t.Errorf("actualizados = %d, esperaba 2", res.Actualizados)
	}
	// Ya nadie queda a medias: ése era el defecto.
	if len(res.SinCredito) != 0 {
		t.Errorf("quedaron a medias habiendo pedido pasarlos a crédito: %v", res.SinCredito)
	}
	// Se nombra SOLO al que cambió: anunciar un cambio en la condición comercial de quien ya
	// estaba en crédito sería informar algo que no pasó.
	if len(res.PasadosACredito) != 1 || res.PasadosACredito[0] != "EnContado" {
		t.Errorf("pasados a crédito = %v, esperaba solo EnContado", res.PasadosACredito)
	}
	if len(res.PasadosACreditoIDs) != 1 || res.PasadosACreditoIDs[0] != enContado {
		t.Errorf("ids pasados a crédito = %v: sin el id no se puede dejar la huella en su ficha", res.PasadosACreditoIDs)
	}

	// LO QUE DE VERDAD IMPORTA: las dos condiciones puestas en la base.
	var cond, manda string
	if err := repo.pool.QueryRow(ctx,
		`SELECT condicion_pago, convenio_manda FROM proveedor WHERE id = $1::uuid`, enContado).
		Scan(&cond, &manda); err != nil {
		t.Fatal(err)
	}
	if cond != "CREDITO" || manda != "PROVEEDOR" {
		t.Errorf("la ficha quedó en %q/%q: con eso el candado sigue poniéndose", manda, cond)
	}
}

// CON «XML» NO SE TOCA LA CONDICIÓN, aunque se pida pasar a crédito.
//
// Elegir que mande el comprobante es lo contrario de este arreglo: ahí la condición de la ficha no
// interviene en el candado, y cambiarla sería alterar la condición comercial del proveedor por el
// camino de un campo que no habla de eso.
func TestConXMLNoSeCambiaLaCondicionDePago(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	const prov = "88888888-8888-4888-8888-888888888888"
	proveedorCon(t, repo, prov, "EnContado", "CONTADO", "PROVEEDOR")

	res, err := repo.MarcarConvenioMasivo(ctx, empresaLote, []string{prov}, "XML", true)
	if err != nil {
		t.Fatalf("marcar: %v", err)
	}
	if len(res.PasadosACredito) != 0 {
		t.Errorf("pasó a crédito volviendo al comprobante: %v", res.PasadosACredito)
	}
	var cond string
	if err := repo.pool.QueryRow(ctx,
		`SELECT condicion_pago FROM proveedor WHERE id = $1::uuid`, prov).Scan(&cond); err != nil {
		t.Fatal(err)
	}
	if cond != "CONTADO" {
		t.Errorf("la condición quedó en %q al elegir que mande el comprobante", cond)
	}
}

// NO ALCANZA A OTRA EMPRESA: el lote recibe ids, y sin el filtro un id ajeno quedaría reescrito.
func TestMarcarEnLoteNoAlcanzaAOtraEmpresa(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	const prov = "88888888-8888-4888-8888-888888888888"
	proveedorCon(t, repo, prov, "EnContado", "CONTADO", "XML")

	res, err := repo.MarcarConvenioMasivo(ctx, "ee000000-0000-4000-a000-000000000099",
		[]string{prov}, "PROVEEDOR", true)
	if err != nil {
		t.Fatalf("marcar: %v", err)
	}
	if res.Actualizados != 0 {
		t.Fatalf("actualizados = %d desde otra empresa", res.Actualizados)
	}
	var cond, manda string
	if err := repo.pool.QueryRow(ctx,
		`SELECT condicion_pago, convenio_manda FROM proveedor WHERE id = $1::uuid`, prov).
		Scan(&cond, &manda); err != nil {
		t.Fatal(err)
	}
	if cond != "CONTADO" || manda != "XML" {
		t.Errorf("la ficha quedó en %q/%q: se tocó desde otra empresa", manda, cond)
	}
}

// Liberar en lote exige el MISMO motivo que liberar de a una: hacerlo muchas veces no lo vuelve
// una afirmación más liviana.
func TestLiberarEnLoteExigeMotivo(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	svc := NewService(repo, nil, nil)

	const prov = "77777777-7777-4777-8777-777777777777"
	proveedorCon(t, repo, prov, "Alguno", "CREDITO", "XML")

	if _, err := svc.LiberarDeProveedores(ctx, empresaLote, []string{prov}, "ok", usuarioLote); !errors.Is(err, ErrMotivoLiberacionRequerido) {
		t.Errorf("se liberó en lote con un motivo de dos letras: %v", err)
	}
	if _, err := svc.LiberarDeProveedores(ctx, empresaLote, nil, "el convenio con estos proveedores es a crédito", usuarioLote); !errors.Is(err, ErrSinProveedores) {
		t.Errorf("se aceptó una liberación sin proveedores: %v", err)
	}
}

// El conteo previo dice cuántas y por cuánto: liberar es quitar la última barrera contra el doble
// pago, así que el monto se ve ANTES de decidir.
func TestSeVeCuantoSeLiberariaAntesDeHacerlo(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	const prov = "77777777-7777-4777-8777-777777777777"
	proveedorCon(t, repo, prov, "ConTrancadas", "CREDITO", "XML")
	for _, m := range []string{"1000", "2500"} {
		if _, err := repo.pool.Exec(ctx, `
			INSERT INTO documento_cxp (empresa_id, proveedor_id, clave, consecutivo, fecha_emision,
			                           estado, tipo, total, total_crc, retencion, moneda, bloqueado_para_pago)
			VALUES ($1::uuid, $2::uuid, $3, $3, '2026-08-01'::date, 'RECIBIDO', 'CXP', $4, $4, 0, 'CRC', true)`,
			empresaLote, prov, "F-"+m, m); err != nil {
			t.Fatalf("sembrar bloqueada: %v", err)
		}
	}

	n, monto, err := repo.BloqueadasDeProveedores(ctx, empresaLote, []string{prov})
	if err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 2 {
		t.Errorf("bloqueadas = %d, esperaba 2", n)
	}
	if monto != "3500" && monto != "3500.00" {
		t.Errorf("monto = %q, esperaba 3500: sin la cifra, liberar es una decisión a ciegas", monto)
	}
}
