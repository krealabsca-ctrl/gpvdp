package bancos

// PROBE TEMPORAL — borrar. ¿La clave de Go y la del repositorio divergen cuando el
// cuenta_bancaria_id del formulario NO viene canónico?

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProbeClaveCuentaNoCanonica(t *testing.T) {
	dsn := os.Getenv("PROBE_DSN")
	if dsn == "" {
		t.Skip("sin PROBE_DSN")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	repo := &pgRepository{pool: pool}

	var emp, cta, fecha, deb, cre, doc string
	if err := pool.QueryRow(ctx, `
		SELECT empresa_id::text, cuenta_bancaria_id::text, to_char(fecha,'YYYY-MM-DD'),
		       to_char(debito,'FM9999999999999990.00'), to_char(credito,'FM9999999999999990.00'),
		       COALESCE(documento,'')
		FROM movimiento_bancario WHERE incluido LIMIT 1`).Scan(&emp, &cta, &fecha, &deb, &cre, &doc); err != nil {
		t.Fatalf("sin movimientos: %v", err)
	}
	up := strings.ToUpper(cta)
	sinGuiones := strings.ReplaceAll(cta, "-", "")

	// (a) el id en MAYUSCULAS: ¿encuentra el movimiento? ¿con qué clave lo devuelve?
	for _, id := range []string{cta, up, sinGuiones} {
		got, err := repo.BuscarMovimientosPorTupla(ctx, emp, []string{id}, []string{fecha}, []string{deb}, []string{cre}, []string{doc})
		if err != nil {
			fmt.Printf("PROBE id=%q -> ERROR %v\n", id, err)
			continue
		}
		claveGo := id + "|" + fecha + "|" + deb + "|" + cre + "|" + strings.TrimSpace(doc)
		fmt.Printf("PROBE id=%q -> %d movimiento(s)\n", id, len(got))
		for _, m := range got {
			fmt.Printf("        clave repo = %q\n        clave Go   = %q  -> CALZA: %v\n", m.Clave, claveGo, m.Clave == claveGo)
		}
	}

	// (b) DOS pedidos que castean al MISMO uuid: ¿devuelve el movimiento dos veces?
	got, err := repo.BuscarMovimientosPorTupla(ctx, emp,
		[]string{cta, up}, []string{fecha, fecha}, []string{deb, deb}, []string{cre, cre}, []string{doc, doc})
	if err != nil {
		fmt.Printf("PROBE (b) ERROR %v\n", err)
	} else {
		fmt.Printf("PROBE (b) dos pedidos (minus + MAYUS) -> %d fila(s):\n", len(got))
		for _, m := range got {
			fmt.Printf("        id=%s clave=%q\n", m.ID, m.Clave)
		}
	}

	// (c) un id que no es uuid
	_, err = repo.BuscarMovimientosPorTupla(ctx, emp, []string{"abc"}, []string{fecha}, []string{deb}, []string{cre}, []string{doc})
	fmt.Printf("PROBE (c) id=\"abc\" -> err=%v\n", err)
}
