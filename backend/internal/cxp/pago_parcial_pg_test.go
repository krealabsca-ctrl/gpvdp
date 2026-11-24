package cxp

// El pago parcial contra PostgreSQL de verdad (20-oct-2026, hallazgo 3.5).
//
// Esto toca lo más delicado del módulo: CÓMO SE DECLARA PAGADA UNA FACTURA. Las tres invariantes
// son sumas entre filas —no caben en un CHECK— y las verifica el repositorio dentro de una
// transacción con el movimiento bloqueado. Un doble repetiría lo que yo creo que hace; solo la
// base puede demostrar que la guarda está donde tiene que estar.

import (
	"context"
	"errors"
	"testing"
)

const motivoParcial = "el banco rechazó una línea del corte y entró solo una parte"

// EL NETO SE ENCOGE, Y ESO ES LO QUE HACE QUE EL SEGUNDO DÉBITO CONCILIE.
//
// Es el corazón del diseño. El primer débito llega por 60.000 contra un neto de 100.000 y da
// MONTO_DIFERENTE; al registrarlo como parcial, el neto pasa a 40.000 y el débito que venga por
// 40.000 calza EXACTO, sin aflojar la comparación ni inventar una regla aparte para el resto.
//
// Falsificación: quitarle a `netoAPagarSQL` la resta de los parciales hace que el neto siga en
// 100.000 y el test falla diciendo exactamente eso.
func TestElNetoSeEncogeConElPagoParcial(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	doc := factura(t, repo, "F-PARCIAL", EstPagado, "100000", false, empresaLote)
	mov := movimiento(t, repo, "2026-08-11", "60000", "", "", true)

	antes, err := repo.NetoAPagar(ctx, empresaLote, doc)
	if err != nil {
		t.Fatalf("neto antes: %v", err)
	}
	if antes != "100000" && antes != "100000.00" {
		t.Fatalf("neto antes = %q, esperaba 100000", antes)
	}

	if _, err := repo.RegistrarPagoParcial(ctx, empresaLote, doc, mov, dec("60000"), "CRC", motivoParcial, usuarioLote); err != nil {
		t.Fatalf("registrar parcial: %v", err)
	}

	despues, err := repo.NetoAPagar(ctx, empresaLote, doc)
	if err != nil {
		t.Fatalf("neto después: %v", err)
	}
	if despues == "100000" || despues == "100000.00" {
		t.Fatal("el neto NO se encogió: el próximo corte le volvería a pagar al proveedor la factura entera, pagando dos veces los 60.000 que ya entraron")
	}
	if despues != "40000" && despues != "40000.00" {
		t.Errorf("neto después = %q, esperaba 40000 (100000 − 60000)", despues)
	}
}

// INVARIANTE 1: lo aplicado a una factura nunca supera lo que todavía debe.
func TestNoSePuedeAplicarMasDeLoQueLaFacturaDebe(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	doc := factura(t, repo, "F-EXCESO", EstPagado, "50000", false, empresaLote)
	mov := movimiento(t, repo, "2026-08-11", "80000", "", "", true)

	_, err := repo.RegistrarPagoParcial(ctx, empresaLote, doc, mov, dec("80000"), "CRC", motivoParcial, usuarioLote)
	if !errors.Is(err, ErrParcialExcedeFactura) {
		t.Errorf("se aplicaron 80.000 a una factura de 50.000: quedaría dada por pagada de más (%v)", err)
	}
}

// INVARIANTE 2: lo aplicado desde un movimiento nunca supera su débito.
//
// Sin esto, el mismo colón se usaría dos veces: dos facturas se darían por pagadas con un débito
// que solo alcanza para una, y el cuadre contra el banco dejaría de cerrar.
func TestUnDebitoNoSePuedeRepartirMasDeLoQueTrajo(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	unaFactura := factura(t, repo, "F-A", EstPagado, "70000", false, empresaLote)
	otraFactura := factura(t, repo, "F-B", EstPagado, "70000", false, empresaLote)
	mov := movimiento(t, repo, "2026-08-11", "50000", "", "", true)

	// El reparto legítimo: 30.000 a una.
	if _, err := repo.RegistrarPagoParcial(ctx, empresaLote, unaFactura, mov, dec("30000"), "CRC", motivoParcial, usuarioLote); err != nil {
		t.Fatalf("primer reparto: %v", err)
	}
	// Quedan 20.000 del débito. Pedir 25.000 para la otra tiene que fallar.
	_, err := repo.RegistrarPagoParcial(ctx, empresaLote, otraFactura, mov, dec("25000"), "CRC", motivoParcial, usuarioLote)
	if !errors.Is(err, ErrParcialExcedeMovimiento) {
		t.Errorf("se repartieron 55.000 de un débito de 50.000: el mismo colón se usó dos veces (%v)", err)
	}
	// Y los 20.000 que sí quedan tienen que poder aplicarse: el reparto entre varias facturas es
	// justamente lo que se pidió.
	if _, err := repo.RegistrarPagoParcial(ctx, empresaLote, otraFactura, mov, dec("20000"), "CRC", motivoParcial, usuarioLote); err != nil {
		t.Errorf("no se pudo repartir el resto legítimo del débito: %v", err)
	}
}

// INVARIANTE 3: un movimiento ya aplicado entero no admite parciales.
func TestUnMovimientoYaEnlazadoNoAdmiteParciales(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	pagada := factura(t, repo, "F-ENTERA", EstPagado, "50000", false, empresaLote)
	otra := factura(t, repo, "F-OTRA", EstPagado, "50000", false, empresaLote)
	mov := movimiento(t, repo, "2026-08-11", "50000", pagada, "", true) // enlazado entero

	_, err := repo.RegistrarPagoParcial(ctx, empresaLote, otra, mov, dec("10000"), "CRC", motivoParcial, usuarioLote)
	if !errors.Is(err, ErrMovimientoYaAplicado) {
		t.Errorf("un movimiento que ya paga una factura entera se repartió además en parciales: se contaría dos veces (%v)", err)
	}
}

// Cuando los parciales cubren el neto, la factura se concilia sola.
//
// No es comodidad: el propio registro demuestra que la plata entró completa, y exigir que alguien
// vaya aparte a declararla conciliada dejaría facturas saldadas figurando como pendientes en el
// arqueo.
func TestCuandoLosParcialesCubrenElNetoLaFacturaSeConcilia(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	svc := NewService(repo, nil, nil)

	doc := factura(t, repo, "F-COMPLETA", EstPagado, "50000", false, empresaLote)
	mov1 := movimiento(t, repo, "2026-08-11", "30000", "", "", true)
	mov2 := movimiento(t, repo, "2026-08-12", "20000", "", "", true)

	if _, err := svc.RegistrarPagoParcial(ctx, empresaLote, doc, mov1, "30000", motivoParcial, usuarioLote); err != nil {
		t.Fatalf("primer parcial: %v", err)
	}
	var estado string
	if err := repo.pool.QueryRow(ctx, `SELECT estado FROM documento_cxp WHERE id=$1::uuid`, doc).Scan(&estado); err != nil {
		t.Fatalf("estado intermedio: %v", err)
	}
	if estado == EstConciliado {
		t.Fatal("la factura se concilió con el pago incompleto: 30.000 de 50.000")
	}

	saldo, err := svc.RegistrarPagoParcial(ctx, empresaLote, doc, mov2, "20000", motivoParcial, usuarioLote)
	if err != nil {
		t.Fatalf("segundo parcial: %v", err)
	}
	if saldo.Saldo != "0" && saldo.Saldo != "0.00" {
		t.Errorf("saldo = %q, esperaba 0", saldo.Saldo)
	}
	if err := repo.pool.QueryRow(ctx, `SELECT estado FROM documento_cxp WHERE id=$1::uuid`, doc).Scan(&estado); err != nil {
		t.Fatalf("estado final: %v", err)
	}
	if estado != EstConciliado {
		t.Errorf("estado = %q: los parciales cubren el neto completo, la factura tiene que quedar conciliada", estado)
	}
}

// El saldo NO vuelve solo a la cola de pago.
//
// La diferencia puede ser una nota de crédito o una retención no aplicada, y en esos casos volver
// a cobrarla le paga de más al proveedor. Lo decide una persona, con motivo.
func TestElSaldoNoVuelveSoloALaColaDePago(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	svc := NewService(repo, nil, nil)

	doc := factura(t, repo, "F-SALDO", EstPagado, "100000", false, empresaLote)
	mov := movimiento(t, repo, "2026-08-11", "60000", "", "", true)
	if _, err := svc.RegistrarPagoParcial(ctx, empresaLote, doc, mov, "60000", motivoParcial, usuarioLote); err != nil {
		t.Fatalf("parcial: %v", err)
	}

	var estado string
	if err := repo.pool.QueryRow(ctx, `SELECT estado FROM documento_cxp WHERE id=$1::uuid`, doc).Scan(&estado); err != nil {
		t.Fatalf("estado: %v", err)
	}
	if estado == "PROGRAMADO" {
		t.Fatal("el saldo volvió SOLO a la cola de pago: si la diferencia era una nota de crédito, se le paga de más al proveedor")
	}

	// Y con la acción explícita sí vuelve, exigiendo motivo.
	if _, err := svc.MandarACobrarElSaldo(ctx, empresaLote, doc, "ok", usuarioLote); !errors.Is(err, ErrMotivoCobroRequerido) {
		t.Errorf("se mandó a cobrar sin motivo: %v", err)
	}
	if _, err := svc.MandarACobrarElSaldo(ctx, empresaLote, doc,
		"el banco rechazó la línea; el proveedor confirma que el resto se sigue debiendo", usuarioLote); err != nil {
		t.Fatalf("mandar a cobrar: %v", err)
	}
	if err := repo.pool.QueryRow(ctx, `SELECT estado FROM documento_cxp WHERE id=$1::uuid`, doc).Scan(&estado); err != nil {
		t.Fatalf("estado final: %v", err)
	}
	if estado != "PROGRAMADO" {
		t.Errorf("estado = %q, esperaba PROGRAMADO: con la acción explícita el saldo vuelve al circuito", estado)
	}

	// Y lo que sale al banco es el SALDO, no el total: es la misma expresión que arma la macro.
	neto, err := repo.NetoAPagar(ctx, empresaLote, doc)
	if err != nil {
		t.Fatalf("neto: %v", err)
	}
	if neto != "40000" && neto != "40000.00" {
		t.Errorf("al próximo corte iría %q en vez de 40000: se le pagaría de nuevo lo ya cobrado", neto)
	}
}
