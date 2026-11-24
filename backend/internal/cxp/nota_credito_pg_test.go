package cxp

// La nota de crédito del proveedor, contra PostgreSQL de verdad (21-oct-2026, hallazgo 3.6).
//
// El lector de XML de Hacienda reconoce la nota de crédito desde siempre y la EXCLUÍA con este
// comentario, textual: «la nota de crédito RESTA: entraría sumando». O sea, se sabía que leerla
// como una factura más inflaría la deuda y se eligió no leerla hasta tener dónde ponerla.
//
// Lo que estas pruebas fijan es que ahora resta de verdad, y que no se puede aplicar donde no
// corresponde — que es lo que convertiría la rebaja en un pago de menos al proveedor equivocado.

import (
	"context"
	"errors"
	"testing"
)

// notaDeCredito siembra una nota del proveedor de las pruebas.
func notaDeCredito(t *testing.T, repo *pgRepository, nombre, total string) string {
	t.Helper()
	return facturaMoneda(t, repo, nombre, TipoNotaCredito, "CRC", total, total)
}

// LA NOTA RESTA DEL NETO. Es la afirmación central del hallazgo.
//
// Falsificación: quitarle a `netoAPagarSQL` la resta de las notas hace que el neto siga en 100.000
// y el test falla diciendo que se le pagaría al proveedor lo que él mismo ya rebajó.
func TestLaNotaDeCreditoRestaDelNeto(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	svc := NewService(repo, nil, nil)

	fact := facturaMoneda(t, repo, "F-CONNOTA", "CXP", "CRC", "100000", "100000")
	// Solo se netea antes de aprobar, así que la factura arranca en REVISADO.
	if _, err := repo.pool.Exec(ctx, `UPDATE documento_cxp SET estado='REVISADO' WHERE id=$1::uuid`, fact); err != nil {
		t.Fatalf("estado: %v", err)
	}
	nota := notaDeCredito(t, repo, "NC-1", "30000")

	if _, err := svc.AplicarNotaCredito(ctx, empresaLote, fact, nota, "30000", usuarioLote); err != nil {
		t.Fatalf("aplicar nota: %v", err)
	}
	neto, err := repo.NetoAPagar(ctx, empresaLote, fact)
	if err != nil {
		t.Fatalf("neto: %v", err)
	}
	if neto == "100000" || neto == "100000.00" {
		t.Fatal("la nota NO se restó: se le pagaría al proveedor los 100.000 completos, incluyendo los 30.000 que él mismo rebajó")
	}
	if neto != "70000" && neto != "70000.00" {
		t.Errorf("neto = %q, esperaba 70000 (100000 − 30000)", neto)
	}
}

// No se puede aplicar más de lo que la nota vale.
func TestLaNotaNoSePuedeAplicarMasDeLoQueVale(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	svc := NewService(repo, nil, nil)

	unaF := facturaMoneda(t, repo, "F-N1", "CXP", "CRC", "100000", "100000")
	otraF := facturaMoneda(t, repo, "F-N2", "CXP", "CRC", "100000", "100000")
	if _, err := repo.pool.Exec(ctx,
		`UPDATE documento_cxp SET estado='REVISADO' WHERE id = ANY(ARRAY[$1,$2]::uuid[])`, unaF, otraF); err != nil {
		t.Fatalf("estado: %v", err)
	}
	nota := notaDeCredito(t, repo, "NC-REPARTO", "50000")

	// Reparto legítimo entre dos facturas del mismo proveedor.
	if _, err := svc.AplicarNotaCredito(ctx, empresaLote, unaF, nota, "30000", usuarioLote); err != nil {
		t.Fatalf("primera aplicación: %v", err)
	}
	// Quedan 20.000. Pedir 25.000 tiene que fallar.
	_, err := svc.AplicarNotaCredito(ctx, empresaLote, otraF, nota, "25000", usuarioLote)
	if !errors.Is(err, ErrNotaExcedeSuSaldo) {
		t.Errorf("se repartieron 55.000 de una nota de 50.000: se le pagaría de menos al proveedor (%v)", err)
	}
	if _, err := svc.AplicarNotaCredito(ctx, empresaLote, otraF, nota, "20000", usuarioLote); err != nil {
		t.Errorf("no se pudo aplicar el resto legítimo de la nota: %v", err)
	}
}

// Una nota NO rebaja la factura de otro proveedor.
//
// Cruzarlas le pagaría de menos a uno y de más al otro, y las dos facturas quedarían con un neto
// que no corresponde a ningún documento.
func TestLaNotaNoCruzaProveedores(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	svc := NewService(repo, nil, nil)

	// Un segundo proveedor con su propia factura.
	const otroProv = "66666666-6666-4666-8666-666666666666"
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO proveedor (id, empresa_id, nombre, identificacion, iban)
		VALUES ($1::uuid, $2::uuid, 'Proveedor Dos', '3102', 'CR0000000000000000002')`,
		otroProv, empresaLote); err != nil {
		t.Fatalf("proveedor: %v", err)
	}
	var ajena string
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO documento_cxp (empresa_id, proveedor_id, clave, consecutivo, fecha_emision,
		                           estado, tipo, total, total_crc, retencion, moneda)
		VALUES ($1::uuid, $2::uuid, 'F-AJENA', 'F-AJENA', '2026-08-01'::date, 'REVISADO', 'CXP',
		        100000, 100000, 0, 'CRC') RETURNING id::text`,
		empresaLote, otroProv).Scan(&ajena); err != nil {
		t.Fatalf("factura ajena: %v", err)
	}
	nota := notaDeCredito(t, repo, "NC-CRUZADA", "10000") // del proveedor UNO

	_, err := svc.AplicarNotaCredito(ctx, empresaLote, ajena, nota, "10000", usuarioLote)
	if !errors.Is(err, ErrNotaProveedorDistinto) {
		t.Errorf("una nota de un proveedor rebajó la factura de OTRO (%v)", err)
	}
}

// Después de aprobar, el neto ya no se mueve.
//
// Lo que se aprueba ES el neto, y es la cifra con la que se arma el archivo de pago. Si cambiara
// después de la firma, la aprobación sería una formalidad sobre un número que ya no existe. El
// mensaje nombra el camino que sí queda: registrar el pago parcial.
func TestLaNotaNoSeAplicaDespuesDeAprobar(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	svc := NewService(repo, nil, nil)

	fact := facturaMoneda(t, repo, "F-APROBADA", "CXP", "CRC", "100000", "100000")
	if _, err := repo.pool.Exec(ctx, `UPDATE documento_cxp SET estado='APROBADO' WHERE id=$1::uuid`, fact); err != nil {
		t.Fatalf("estado: %v", err)
	}
	nota := notaDeCredito(t, repo, "NC-TARDE", "10000")

	_, err := svc.AplicarNotaCredito(ctx, empresaLote, fact, nota, "10000", usuarioLote)
	if !errors.Is(err, ErrNotaFacturaNoNeteable) {
		t.Errorf("se movió el neto de una factura YA APROBADA: la firma quedaría sobre una cifra que no existe (%v)", err)
	}
}

// Reversar devuelve la deuda: aplicar a la factura equivocada no es un camino de ida.
func TestReversarLaNotaDevuelveLaDeuda(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()
	svc := NewService(repo, nil, nil)

	fact := facturaMoneda(t, repo, "F-REVNOTA", "CXP", "CRC", "100000", "100000")
	if _, err := repo.pool.Exec(ctx, `UPDATE documento_cxp SET estado='REVISADO' WHERE id=$1::uuid`, fact); err != nil {
		t.Fatalf("estado: %v", err)
	}
	nota := notaDeCredito(t, repo, "NC-REV", "40000")
	if _, err := svc.AplicarNotaCredito(ctx, empresaLote, fact, nota, "40000", usuarioLote); err != nil {
		t.Fatalf("aplicar: %v", err)
	}

	aplicadas, err := svc.NotasAplicadasAFactura(ctx, empresaLote, fact)
	if err != nil || len(aplicadas) != 1 {
		t.Fatalf("esperaba 1 aplicación viva, obtuve %d (%v)", len(aplicadas), err)
	}
	if err := svc.ReversarNotaCredito(ctx, empresaLote, aplicadas[0].ID, usuarioLote); err != nil {
		t.Fatalf("reversar: %v", err)
	}
	neto, err := repo.NetoAPagar(ctx, empresaLote, fact)
	if err != nil {
		t.Fatalf("neto: %v", err)
	}
	if neto != "100000" && neto != "100000.00" {
		t.Errorf("neto = %q tras reversar, esperaba 100000: la rebaja tiene que desaparecer", neto)
	}
}

// LA TABLA DE CxP NO ES LA DE CxC, Y EL NOMBRE LO TIENE QUE DECIR.
//
// La primera versión de la migración 0106 llamó a su tabla `nota_credito_aplicacion` a secas, y
// ese nombre ya estaba tomado desde la 0054 por las notas de crédito de CxC —con `cargo_id` donde
// acá va `factura_id`—. El `CREATE TABLE IF NOT EXISTS` no falló: encontró la tabla ajena y no
// hizo nada, en silencio. Lo que reventó fue el índice siguiente, y la migración quedó sucia.
//
// Si el índice no hubiera mencionado una columna inexistente, CxP habría empezado a escribir en la
// tabla de CxC. Esta prueba fija que las dos existen por separado.
func TestLaTablaDeNotasDeCxPNoEsLaDeCxC(t *testing.T) {
	repo := baseDeLotes(t)
	ctx := context.Background()

	var columnaDeCxP string
	if err := repo.pool.QueryRow(ctx, `
		SELECT COALESCE(string_agg(column_name, ','), '') FROM information_schema.columns
		WHERE table_name = 'cxp_nota_credito_aplicacion' AND column_name IN ('factura_id','nota_id')`).
		Scan(&columnaDeCxP); err != nil {
		t.Fatalf("columnas de CxP: %v", err)
	}
	if !contieneSub(columnaDeCxP, "factura_id") {
		t.Error("la tabla de notas de CxP no tiene `factura_id`: o no se creó, o se está usando la de CxC")
	}

	var cargoEnCxC bool
	if err := repo.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.columns
		               WHERE table_name = 'nota_credito_aplicacion' AND column_name = 'cargo_id')`).
		Scan(&cargoEnCxC); err != nil {
		t.Fatalf("columnas de CxC: %v", err)
	}
	if !cargoEnCxC {
		t.Error("la tabla de notas de CxC perdió `cargo_id`: la migración de CxP le pasó por encima")
	}
}
