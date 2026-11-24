package cxp

// El cronograma contra PostgreSQL de verdad (mig 0122, 11-nov-2026).
//
// `cronograma_test.go` ya prueba la ARITMÉTICA de la cascada contra el archivo real del Director
// Financiero. Lo que no puede probar es de dónde salen los números, y ahí están los dos defectos
// que importan, porque los dos producen un cuadro que se ve correcto:
//
//  1. QUE EL MONTO SEA EL NETO Y NO EL TOTAL. El cronograma dice lo que va a SALIR DEL BANCO. Una
//     factura de ₡100.000 con ₡2.000 de retención y ₡30.000 de nota de crédito mueve ₡68.000:
//     anunciar ₡100.000 pide ₡32.000 de fondos que nadie va a girar. El error no se ve en la
//     pantalla —la cifra está bien formada— y recién aparece al cuadrar contra el banco.
//
//  2. QUE LA RESERVA VIAJE DESDE LA CUENTA. La marca vive en `cuenta_bancaria` y tiene que llegar
//     hasta la línea para que la cascada la reste. Si se pierde en el camino, el cuadro suma al
//     flujo normal plata que salió de fondos apartados, que es exactamente el número que el
//     cronograma existe para comunicar.
//
// Y el de seguridad: asignar una cuenta de OTRA empresa diría que el dinero sale de un banco que
// no es de esta empresa.

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

// cuentaDePrueba siembra un banco con una cuenta y devuelve el id de la cuenta.
func cuentaDePrueba(t *testing.T, r *pgRepository, empresa, alias string, reserva bool) string {
	t.Helper()
	ctx := context.Background()
	var bancoID string
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO banco (empresa_id, nombre) VALUES ($1::uuid, $2) RETURNING id::text`,
		empresa, "Banco "+alias).Scan(&bancoID); err != nil {
		t.Fatalf("sembrar banco: %v", err)
	}
	var cuentaID string
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO cuenta_bancaria (empresa_id, banco_id, alias, moneda, es_reserva)
		VALUES ($1::uuid, $2::uuid, $3, 'CRC', $4) RETURNING id::text`,
		empresa, bancoID, alias, reserva).Scan(&cuentaID); err != nil {
		t.Fatalf("sembrar cuenta: %v", err)
	}
	return cuentaID
}

// lineaDe busca la línea de un proveedor en el cronograma.
func lineaDe(t *testing.T, c Cronograma, proveedor string) LineaCronograma {
	t.Helper()
	for _, l := range c.Lineas {
		if l.Proveedor == proveedor {
			return l
		}
	}
	t.Fatalf("el cronograma no trae a %q; trae %d línea(s)", proveedor, len(c.Lineas))
	return LineaCronograma{}
}

// El cronograma anuncia lo que SALE DEL BANCO, no el total facturado.
func TestElCronogramaAnunciaElNetoNoElTotal(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	loteID, docID := sembrarCorteConFactura(t, repo)
	// La factura nace en ₡100.000 sin descuentos (ver sembrarCorteConFactura). Le ponemos dos de
	// clases distintas —una que vive en la propia factura y otra en una tabla aparte— para que
	// ninguna de las dos formas de perder un descuento pase desapercibida.
	if _, err := repo.pool.Exec(ctx,
		`UPDATE documento_cxp SET retencion = 2000 WHERE id = $1::uuid`, docID); err != nil {
		t.Fatalf("poner retención: %v", err)
	}
	// La nota de crédito del proveedor es, ella misma, un documento.
	_, notaID := sembrarCorteConFactura(t, repo)
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO cxp_nota_credito_aplicacion (empresa_id, nota_id, factura_id, monto, aplicado_por)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 30000, $4::uuid)`,
		empresaLote, notaID, docID, usuarioLote); err != nil {
		t.Fatalf("sembrar nota de crédito: %v", err)
	}

	filas, err := repo.LineasDelCronograma(ctx, empresaLote, loteID)
	if err != nil {
		t.Fatalf("líneas: %v", err)
	}
	cr, err := ConstruirCronograma(filas, decimal.Zero)
	if err != nil {
		t.Fatalf("construir: %v", err)
	}

	// 100.000 − 2.000 de retención − 30.000 de nota de crédito = 68.000.
	l := lineaDe(t, cr, "Proveedor Uno")
	if l.Monto != "68000.00" {
		t.Errorf("monto = %s, se esperaba 68000 (el total es 100000: el cronograma estaría pidiendo "+
			"fondos que no se van a girar)", l.Monto)
	}
	if cr.Totales.CronogramaTotal != "68000.00" {
		t.Errorf("cronograma total = %s, se esperaba 68000", cr.Totales.CronogramaTotal)
	}
}

// La marca de la cuenta tiene que llegar hasta la cascada.
func TestLaReservaDeLaCuentaSaleDelFlujoNormal(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	loteID, _ := sembrarCorteConFactura(t, repo)
	normal := cuentaDePrueba(t, repo, empresaLote, "Operativa", false)
	reserva := cuentaDePrueba(t, repo, empresaLote, "Apartada", true)

	// Primero desde la cuenta operativa: la factura es flujo normal.
	if _, err := repo.AsignarCuentaDePago(ctx, empresaLote, loteID, provLote, normal); err != nil {
		t.Fatalf("asignar cuenta operativa: %v", err)
	}
	filas, err := repo.LineasDelCronograma(ctx, empresaLote, loteID)
	if err != nil {
		t.Fatalf("líneas: %v", err)
	}
	cr, err := ConstruirCronograma(filas, decimal.Zero)
	if err != nil {
		t.Fatalf("construir: %v", err)
	}
	if cr.Totales.Reserva != "0.00" {
		t.Errorf("reserva = %s desde una cuenta operativa", cr.Totales.Reserva)
	}
	if cr.Totales.CronogramaTotal != "100000.00" {
		t.Errorf("cronograma total = %s, se esperaba 100000", cr.Totales.CronogramaTotal)
	}

	// Ahora desde la cuenta de fondos apartados: la MISMA factura sale del flujo normal.
	if _, err := repo.AsignarCuentaDePago(ctx, empresaLote, loteID, provLote, reserva); err != nil {
		t.Fatalf("asignar cuenta de reserva: %v", err)
	}
	filas, err = repo.LineasDelCronograma(ctx, empresaLote, loteID)
	if err != nil {
		t.Fatalf("líneas: %v", err)
	}
	cr, err = ConstruirCronograma(filas, decimal.Zero)
	if err != nil {
		t.Fatalf("construir: %v", err)
	}
	if !lineaDe(t, cr, "Proveedor Uno").EsReserva {
		t.Error("la línea no quedó marcada como reserva: la marca de la cuenta no llegó")
	}
	if cr.Totales.Reserva != "100000.00" {
		t.Errorf("reserva = %s, se esperaba 100000", cr.Totales.Reserva)
	}
	if cr.Totales.CronogramaTotal != "0.00" {
		t.Errorf("cronograma total = %s: sigue contando plata de fondos apartados",
			cr.Totales.CronogramaTotal)
	}
}

// Una cuenta de otra empresa no puede terminar en el cronograma de esta.
func TestNoSePuedePagarDesdeUnaCuentaDeOtraEmpresa(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	loteID, _ := sembrarCorteConFactura(t, repo)
	ajena := cuentaDePrueba(t, repo, otraEmpLote, "De Coopeprofa", false)

	filas, err := repo.AsignarCuentaDePago(ctx, empresaLote, loteID, provLote, ajena)
	if err != nil {
		t.Fatalf("asignar: %v", err)
	}
	if filas != 0 {
		t.Fatalf("se asignaron %d factura(s) a una cuenta de otra empresa", filas)
	}

	cr, err := repoCronograma(t, repo, loteID)
	if err != nil {
		t.Fatalf("cronograma: %v", err)
	}
	if l := lineaDe(t, cr, "Proveedor Uno"); l.Cuenta != "" {
		t.Errorf("la línea dice que el dinero sale de %q, una cuenta de otra empresa", l.Cuenta)
	}
}

// Lo que se saca del cronograma en un corte no puede moverle las cifras a otro.
func TestSacarDelCronogramaNoTocaOtroCorte(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	loteA, _ := sembrarCorteConFactura(t, repo)
	loteB, _ := sembrarCorteConFactura(t, repo)

	if _, err := repo.MarcarPorFuera(ctx, empresaLote, loteB, provLote, true, usuarioLote); err != nil {
		t.Fatalf("marcar por fuera: %v", err)
	}

	crA, err := repoCronograma(t, repo, loteA)
	if err != nil {
		t.Fatalf("cronograma A: %v", err)
	}
	if crA.Totales.PorFuera != "0.00" {
		t.Errorf("el corte A quedó con %s por fuera por una decisión tomada sobre el corte B",
			crA.Totales.PorFuera)
	}
	if crA.Totales.CronogramaTotal != "100000.00" {
		t.Errorf("cronograma total del corte A = %s, se esperaba 100000",
			crA.Totales.CronogramaTotal)
	}

	crB, err := repoCronograma(t, repo, loteB)
	if err != nil {
		t.Fatalf("cronograma B: %v", err)
	}
	if crB.Totales.PorFuera != "100000.00" {
		t.Errorf("el corte B tiene %s por fuera, se esperaba 100000", crB.Totales.PorFuera)
	}
}

// repoCronograma arma el cronograma de un corte desde la base, sin tipo de cambio.
func repoCronograma(t *testing.T, r *pgRepository, loteID string) (Cronograma, error) {
	t.Helper()
	filas, err := r.LineasDelCronograma(context.Background(), empresaLote, loteID)
	if err != nil {
		return Cronograma{}, err
	}
	return ConstruirCronograma(filas, decimal.Zero)
}
