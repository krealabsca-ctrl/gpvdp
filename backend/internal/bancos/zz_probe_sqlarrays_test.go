package bancos

// PROBE TEMPORAL — borrar. Solo mide si pgx puede codificar []string en date[]/numeric[]
// y si la clave de to_char calza con la de Go.

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProbeArraysReales(t *testing.T) {
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

	const q = `
		WITH pedidos AS (
			SELECT * FROM unnest($2::uuid[], $3::date[], $4::numeric[], $5::numeric[], $6::text[])
			         AS t(cuenta_id, fecha, debito, credito, documento)
		)
		SELECT p.cuenta_id::text, to_char(p.fecha, 'YYYY-MM-DD'),
		       to_char(p.debito, 'FM9999999999999990.00'), to_char(p.credito, 'FM9999999999999990.00'),
		       p.documento,
		       m.id::text, COALESCE(m.descripcion, ''),
		       COALESCE(co.nombre, ''), COALESCE(cl.nombre, ''),
		       COALESCE(m.clasificacion_id::text, ''), m.estado_clasificacion
		FROM pedidos p
		JOIN movimiento_bancario m
		  ON m.empresa_id = $1::uuid
		 AND m.cuenta_bancaria_id = p.cuenta_id
		 AND m.fecha = p.fecha
		 AND m.debito = p.debito
		 AND m.credito = p.credito
		 AND COALESCE(m.documento, '') = p.documento
		 AND m.incluido
		LEFT JOIN concepto co ON co.id = m.concepto_id
		LEFT JOIN clasificacion cl ON cl.id = m.clasificacion_id
		ORDER BY 1, 2, 3, 4, 5, m.id`

	// 1) ¿codifica []string en date[] y numeric[]?
	rows, err := pool.Query(ctx, q,
		"00000000-0000-0000-0000-000000000000",
		[]string{"00000000-0000-0000-0000-000000000001"},
		[]string{"2026-08-01"},
		[]string{"1234.56"},
		[]string{"0.00"},
		[]string{"DOC1"})
	if err != nil {
		fmt.Printf("PROBE encode ERROR: %v\n", err)
	} else {
		n := 0
		for rows.Next() {
			n++
		}
		fmt.Printf("PROBE encode OK, filas=%d err=%v\n", n, rows.Err())
		rows.Close()
	}

	// 2) contra un movimiento REAL: ¿calza la clave?
	var emp, cta, fecha, deb, cre, doc string
	err = pool.QueryRow(ctx, `
		SELECT empresa_id::text, cuenta_bancaria_id::text, to_char(fecha,'YYYY-MM-DD'),
		       to_char(debito,'FM9999999999999990.00'), to_char(credito,'FM9999999999999990.00'),
		       COALESCE(documento,'')
		FROM movimiento_bancario WHERE incluido LIMIT 1`).Scan(&emp, &cta, &fecha, &deb, &cre, &doc)
	if err != nil {
		fmt.Printf("PROBE sin movimientos: %v\n", err)
		return
	}
	fmt.Printf("PROBE mov real: cta=%s fecha=%s deb=%s cre=%s doc=%q\n", cta, fecha, deb, cre, doc)
	rows2, err := pool.Query(ctx, q, emp, []string{cta}, []string{fecha}, []string{deb}, []string{cre}, []string{doc})
	if err != nil {
		fmt.Printf("PROBE real ERROR: %v\n", err)
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var c, f, d, cr, dc, id, desc, co, cl, clid, est string
		if err := rows2.Scan(&c, &f, &d, &cr, &dc, &id, &desc, &co, &cl, &clid, &est); err != nil {
			fmt.Printf("PROBE scan: %v\n", err)
			return
		}
		fmt.Printf("PROBE clave devuelta = %q\n", c+"|"+f+"|"+d+"|"+cr+"|"+dc)
	}
	fmt.Printf("PROBE real err=%v\n", rows2.Err())

	// 3) ¿una cuenta con UUID en MAYUSCULAS devuelve la clave normalizada?
	up := ""
	for _, r := range cta {
		if r >= 'a' && r <= 'f' {
			up += string(r - 32)
		} else {
			up += string(r)
		}
	}
	rows3, err := pool.Query(ctx, q, emp, []string{up}, []string{fecha}, []string{deb}, []string{cre}, []string{doc})
	if err != nil {
		fmt.Printf("PROBE mayus ERROR: %v\n", err)
		return
	}
	defer rows3.Close()
	for rows3.Next() {
		var c, f, d, cr, dc, id, desc, co, cl, clid, est string
		_ = rows3.Scan(&c, &f, &d, &cr, &dc, &id, &desc, &co, &cl, &clid, &est)
		fmt.Printf("PROBE mayus: pedido=%q devuelto=%q -> %v\n", up, c, up == c)
	}
}
