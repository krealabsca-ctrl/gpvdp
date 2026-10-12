package bancos

// Dividir un movimiento entre partidas, contra PostgreSQL de verdad (9-oct-2026).
//
// «Del recibo de luz este mes se pagó 1.400.000, pero de luz normal eran 1.000.000, y esos 400.000
// corresponden a un proyecto especial de ese mes. Del movimiento principal se quita lo del
// proyecto, pero la sumatoria de ambas suman el movimiento real, y se conserva la integridad del
// consecutivo, fecha, descripción.»
//
// TODO lo que se prueba acá gira alrededor de UNA invariante:
//
//	padre + hijos = lo que trajo el banco, SIEMPRE
//
// Es una propiedad de lo que queda ESCRITO, así que un doble no la puede ni violar ni sostener: o
// se prueba contra la base, o no se prueba.

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

// recibosDeLuz siembra el caso exacto: ₡1.400.000 de electricidad en un solo movimiento.
func reciboDeLuz(t *testing.T, r *pgRepository) string {
	t.Helper()
	return sembrarMovDebito(t, r, "2026-09-30", "4410077", "PAGO RECIBO ELECTRICIDAD CNFL", 1400000)
}

// sembrarMovDebito crea un DÉBITO (una salida), que es la forma de un recibo pagado.
func sembrarMovDebito(t *testing.T, r *pgRepository, fecha, documento, descripcion string, debito float64) string {
	t.Helper()
	var id string
	llave := fecha + "|" + documento + "|deb"
	err := r.pool.QueryRow(context.Background(), `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, importacion_id, fecha, documento, descripcion,
			 debito, credito, moneda_original, monto_original, monto_crc, incluido, natural_key)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4::date,$5,$6,$7::numeric,0,'CRC',$7::numeric,
		        $7::numeric,true,$8)
		RETURNING id::text`,
		empExc, cuentaExc, impExc, fecha, documento, descripcion, debito, llave).Scan(&id)
	if err != nil {
		t.Fatalf("sembrar movimiento: %v", err)
	}
	return id
}

func montoDe(t *testing.T, r *pgRepository, id string) decimal.Decimal {
	t.Helper()
	var s string
	if err := r.pool.QueryRow(context.Background(),
		`SELECT (COALESCE(debito,0) + COALESCE(credito,0))::text FROM movimiento_bancario WHERE id = $1::uuid`,
		id).Scan(&s); err != nil {
		t.Fatalf("leer monto: %v", err)
	}
	return decimal.RequireFromString(s)
}

// clasificacionDePrueba crea una partida para mandar ahí la parte separada.
func clasificacionDePrueba(t *testing.T, r *pgRepository, nombre string) (conceptoID, clasifID string) {
	t.Helper()
	ctx := context.Background()
	if err := r.pool.QueryRow(ctx,
		`INSERT INTO concepto (empresa_id, nombre, naturaleza) VALUES ($1::uuid,$2,'GASTO')
		 ON CONFLICT DO NOTHING RETURNING id::text`, empExc, "Gastos "+nombre).Scan(&conceptoID); err != nil {
		// Ya existía: lo busco.
		if err2 := r.pool.QueryRow(ctx,
			`SELECT id::text FROM concepto WHERE empresa_id=$1::uuid AND nombre=$2`,
			empExc, "Gastos "+nombre).Scan(&conceptoID); err2 != nil {
			t.Fatalf("concepto: %v / %v", err, err2)
		}
	}
	if err := r.pool.QueryRow(ctx,
		`INSERT INTO clasificacion (empresa_id, concepto_id, nombre) VALUES ($1::uuid,$2::uuid,$3)
		 RETURNING id::text`, empExc, conceptoID, nombre).Scan(&clasifID); err != nil {
		t.Fatalf("clasificacion: %v", err)
	}
	return conceptoID, clasifID
}

// EL CASO: 1.400.000 de luz, de los cuales 400.000 son un proyecto especial.
func TestElReciboDeLuzSeDivideYLaSumaCuadra(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	mov := reciboDeLuz(t, repo)
	conceptoID, clasifID := clasificacionDePrueba(t, repo, "Proyecto especial setiembre")

	d, err := repo.DividirMovimiento(ctx, empExc, mov, DivisionInput{
		Monto:           decimal.RequireFromString("400000"),
		MontoCRC:        decimal.RequireFromString("400000"),
		ConceptoID:      conceptoID,
		ClasificacionID: clasifID,
		Motivo:          "obra del horno, no es consumo normal",
	}, usrExc)
	if err != nil {
		t.Fatalf("dividir: %v", err)
	}
	if d == "" {
		t.Fatal("no devolvió el id de la parte")
	}

	// 1. Al principal le quedó lo suyo.
	if got := montoDe(t, repo, mov); !got.Equal(decimal.RequireFromString("1000000")) {
		t.Errorf("al movimiento principal le quedaron %s, se esperaban 1000000", got)
	}
	// 2. La parte tiene lo que se apartó.
	if got := montoDe(t, repo, d); !got.Equal(decimal.RequireFromString("400000")) {
		t.Errorf("la parte tiene %s, se esperaban 400000", got)
	}

	// 3. LA INVARIANTE: las partes suman lo que trajo el banco.
	rep, err := repo.PartesDeMovimiento(ctx, empExc, mov)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Cuadra() {
		t.Errorf("el reparto no cuadra: antes %s, suma de las partes %s",
			rep.MontoAntesDeDividir, rep.SumaDeLasPartes)
	}
	if rep.SumaDeLasPartes != "1400000.00" {
		t.Errorf("las partes suman %s, se esperaba 1400000.00", rep.SumaDeLasPartes)
	}
	if len(rep.Partes) != 2 {
		t.Fatalf("el reparto tiene %d partes, se esperaban 2", len(rep.Partes))
	}

	// 4. EL HECHO BANCARIO NO SE TOCÓ: consecutivo, fecha y descripción se conservan.
	if rep.Documento != "4410077" {
		t.Errorf("el consecutivo cambió: %q", rep.Documento)
	}
	if rep.Fecha != "2026-09-30" {
		t.Errorf("la fecha cambió: %q", rep.Fecha)
	}
	if rep.Descripcion != "PAGO RECIBO ELECTRICIDAD CNFL" {
		t.Errorf("la descripción cambió: %q", rep.Descripcion)
	}
	// Y la parte lo hereda: quien la busque por el consecutivo del banco la encuentra.
	var docParte string
	if err := repo.pool.QueryRow(ctx,
		`SELECT documento FROM movimiento_bancario WHERE id=$1::uuid`, d).Scan(&docParte); err != nil {
		t.Fatal(err)
	}
	if docParte != "4410077" {
		t.Errorf("la parte no heredó el consecutivo del banco (%q)", docParte)
	}
}

// LO QUE MÁS IMPORTA: el total del mes NO cambia al dividir.
//
// Si cambiara, dividir un recibo movería los libros, y el reparto de la atribución pasaría a ser
// una operación financiera. Esto es lo que sostiene que el cuadre, el EBITDA y el tablero sigan
// dando lo mismo sin tocar ni una de sus 43 consultas.
func TestDividirNoMueveElTotalDelMes(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	// Tres movimientos del mes, con consecutivos distintos: el anti-duplicado (`natural_key` es
	// única por empresa) rechaza dos recibos con el mismo número, y hace bien.
	reciboDeLuz(t, repo)
	sembrarMovDebito(t, repo, "2026-09-15", "9900111", "OTRO GASTO", 250000)
	mov := sembrarMovDebito(t, repo, "2026-08-31", "4410076", "PAGO RECIBO ELECTRICIDAD CNFL", 1400000)

	total := func() decimal.Decimal {
		t.Helper()
		var s string
		if err := repo.pool.QueryRow(ctx,
			`SELECT COALESCE(SUM(COALESCE(debito,0)+COALESCE(credito,0)),0)::text
			 FROM movimiento_bancario WHERE empresa_id=$1::uuid AND incluido`, empExc).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return decimal.RequireFromString(s)
	}
	antes := total()

	conceptoID, clasifID := clasificacionDePrueba(t, repo, "Proyecto especial")
	if _, err := repo.DividirMovimiento(ctx, empExc, mov, DivisionInput{
		Monto:           decimal.RequireFromString("400000"),
		MontoCRC:        decimal.RequireFromString("400000"),
		ConceptoID:      conceptoID,
		ClasificacionID: clasifID,
	}, usrExc); err != nil {
		t.Fatalf("dividir: %v", err)
	}

	if despues := total(); !antes.Equal(despues) {
		t.Fatalf("el total de los libros cambió al dividir: antes %s, después %s. Dividir reparte "+
			"la atribución, no mueve plata", antes, despues)
	}
}

// Deshacer devuelve el monto al principal y saca la parte: la invariante se sostiene en los dos
// sentidos.
func TestDeshacerLaDivisionDevuelveElMonto(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	mov := reciboDeLuz(t, repo)
	conceptoID, clasifID := clasificacionDePrueba(t, repo, "Proyecto especial")

	parte, err := repo.DividirMovimiento(ctx, empExc, mov, DivisionInput{
		Monto:           decimal.RequireFromString("400000"),
		MontoCRC:        decimal.RequireFromString("400000"),
		ConceptoID:      conceptoID,
		ClasificacionID: clasifID,
	}, usrExc)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DeshacerDivision(ctx, empExc, parte); err != nil {
		t.Fatalf("deshacer: %v", err)
	}
	if got := montoDe(t, repo, mov); !got.Equal(decimal.RequireFromString("1400000")) {
		t.Errorf("al deshacer, el principal quedó en %s y se esperaban 1400000", got)
	}
	if estaIncluido(t, repo, parte) {
		t.Error("la parte sigue en los libros después de deshacer: la plata está contada dos veces")
	}
	// Y el principal deja de figurar como dividido.
	var antes *string
	if err := repo.pool.QueryRow(ctx,
		`SELECT monto_antes_de_dividir::text FROM movimiento_bancario WHERE id=$1::uuid`, mov).Scan(&antes); err != nil {
		t.Fatal(err)
	}
	if antes != nil {
		t.Errorf("el principal sigue marcado como dividido (%s)", *antes)
	}
}

// No se puede apartar TODO el monto: eso no es dividir, es reclasificar.
func TestNoSePuedeApartarTodoElMonto(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	mov := reciboDeLuz(t, repo)
	conceptoID, clasifID := clasificacionDePrueba(t, repo, "Proyecto especial")

	for _, monto := range []string{"1400000", "1500000"} {
		_, err := repo.DividirMovimiento(ctx, empExc, mov, DivisionInput{
			Monto:           decimal.RequireFromString(monto),
			MontoCRC:        decimal.RequireFromString(monto),
			ConceptoID:      conceptoID,
			ClasificacionID: clasifID,
		}, usrExc)
		if err != ErrMovimientoNoDivisible {
			t.Errorf("apartar %s devolvió %v; dejaría el movimiento en cero o en negativo", monto, err)
		}
	}
	// Y el movimiento quedó intacto.
	if got := montoDe(t, repo, mov); !got.Equal(decimal.RequireFromString("1400000")) {
		t.Errorf("el movimiento quedó en %s tras los intentos fallidos", got)
	}
}

// Una parte no se vuelve a dividir: con partes de partes, «padre + hijos» deja de ser una suma.
func TestUnaParteNoSeVuelveADividir(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	mov := reciboDeLuz(t, repo)
	conceptoID, clasifID := clasificacionDePrueba(t, repo, "Proyecto especial")

	parte, err := repo.DividirMovimiento(ctx, empExc, mov, DivisionInput{
		Monto:           decimal.RequireFromString("400000"),
		MontoCRC:        decimal.RequireFromString("400000"),
		ConceptoID:      conceptoID,
		ClasificacionID: clasifID,
	}, usrExc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.DividirMovimiento(ctx, empExc, parte, DivisionInput{
		Monto:           decimal.RequireFromString("100000"),
		MontoCRC:        decimal.RequireFromString("100000"),
		ConceptoID:      conceptoID,
		ClasificacionID: clasifID,
	}, usrExc)
	if err != ErrMovimientoNoDivisible {
		t.Fatalf("se dividió una parte (err=%v)", err)
	}
}

// Un movimiento se puede repartir en TRES: se divide el principal dos veces.
func TestSePuedeRepartirEnTresPartes(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	mov := reciboDeLuz(t, repo)
	conceptoID, c1 := clasificacionDePrueba(t, repo, "Proyecto A")
	_, c2 := clasificacionDePrueba(t, repo, "Proyecto B")

	for _, p := range []struct {
		monto  string
		clasif string
	}{{"400000", c1}, {"150000", c2}} {
		if _, err := repo.DividirMovimiento(ctx, empExc, mov, DivisionInput{
			Monto:           decimal.RequireFromString(p.monto),
			MontoCRC:        decimal.RequireFromString(p.monto),
			ConceptoID:      conceptoID,
			ClasificacionID: p.clasif,
		}, usrExc); err != nil {
			t.Fatalf("dividir %s: %v", p.monto, err)
		}
	}
	rep, err := repo.PartesDeMovimiento(ctx, empExc, mov)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Partes) != 3 {
		t.Fatalf("el reparto tiene %d partes, se esperaban 3", len(rep.Partes))
	}
	if !rep.Cuadra() || rep.SumaDeLasPartes != "1400000.00" {
		t.Errorf("tras dos divisiones la suma es %s y se esperaba 1400000.00 (antes: %s)",
			rep.SumaDeLasPartes, rep.MontoAntesDeDividir)
	}
	// Al principal le queda el resto.
	if got := montoDe(t, repo, mov); !got.Equal(decimal.RequireFromString("850000")) {
		t.Errorf("al principal le quedaron %s, se esperaban 850000", got)
	}
}

// Desde una PARTE se ve el reparto entero: quien la abre quiere saber de dónde salió.
func TestDesdeUnaParteSeVeElRepartoCompleto(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	mov := reciboDeLuz(t, repo)
	conceptoID, clasifID := clasificacionDePrueba(t, repo, "Proyecto especial")
	parte, err := repo.DividirMovimiento(ctx, empExc, mov, DivisionInput{
		Monto:           decimal.RequireFromString("400000"),
		MontoCRC:        decimal.RequireFromString("400000"),
		ConceptoID:      conceptoID,
		ClasificacionID: clasifID,
	}, usrExc)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := repo.PartesDeMovimiento(ctx, empExc, parte)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Partes) != 2 || rep.MovimientoID != mov {
		t.Fatalf("desde la parte se vieron %d partes y la raíz fue %q", len(rep.Partes), rep.MovimientoID)
	}
}

// El aislamiento por empresa: no se divide un movimiento ajeno.
func TestNoSeDivideUnMovimientoDeOtraEmpresa(t *testing.T) {
	repo := baseDeExclusion(t)
	ctx := context.Background()
	mov := reciboDeLuz(t, repo)
	conceptoID, clasifID := clasificacionDePrueba(t, repo, "Proyecto especial")
	otra := "99999999-9999-4999-9999-999999999999"

	_, err := repo.DividirMovimiento(ctx, otra, mov, DivisionInput{
		Monto:           decimal.RequireFromString("400000"),
		MontoCRC:        decimal.RequireFromString("400000"),
		ConceptoID:      conceptoID,
		ClasificacionID: clasifID,
	}, usrExc)
	if err != ErrMovimientoNoDivisible {
		t.Fatalf("se dividió un movimiento desde otra empresa (err=%v)", err)
	}
	if got := montoDe(t, repo, mov); !got.Equal(decimal.RequireFromString("1400000")) {
		t.Fatal("el movimiento cambió desde otra empresa")
	}
}
