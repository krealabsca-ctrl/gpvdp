package conta

// El ORDEN del mapeo contable, contra PostgreSQL de verdad (12-nov-2026).
//
// LO QUE SE PROTEGE ES POR DÓNDE SE EMPIEZA. La lista ordenaba por cantidad de movimientos, con el
// argumento razonable de que «lo que más se usa es donde mapear rinde más». Medido en Valle de Paz
// sobre ₡2.956 M, es falso y por mucho: «Traslado de Fondos» mueve ₡1.115 M en 239 movimientos y
// quedaba OCTAVO, detrás de «Depósito de Clientes», que mueve doce veces menos plata en 7.960
// movimientos. «Utilidades» mueve ₡63 M en 6 movimientos y caía en el puesto 54.
//
// Con el orden por plata, SIETE partidas de 131 cubren el 80 % del dinero. Volver al orden por
// cantidad no rompería ninguna pantalla ni daría ningún error: simplemente haría que alguien
// dedique la tarde a mapear lo que no mueve plata. Por eso hay una prueba.

import (
	"context"
	"testing"
)

// conMonto le pone plata a los movimientos de una clasificación ya sembrada.
//
// `sembrarPartida` (en repository_mapeo_test.go) inserta los movimientos SIN monto, porque a la
// cobertura solo le importa cuántos son. Acá importa cuánto pesan, así que se completa después en
// vez de cambiar un helper que seis pruebas ya usan.
func conMonto(t *testing.T, r *pgRepository, clasifID, monto string) {
	t.Helper()
	if _, err := r.pool.Exec(context.Background(),
		`UPDATE movimiento_bancario SET debito = $2::numeric, monto_crc = $2::numeric, incluido = true
		  WHERE clasificacion_id = $1::uuid`, clasifID, monto); err != nil {
		t.Fatalf("poner monto: %v", err)
	}
}

// LA PARTIDA QUE MUEVE MÁS PLATA VA PRIMERO, aunque tenga menos movimientos.
func TestElMapeoOrdenaPorPlataYNoPorCantidad(t *testing.T) {
	repo := baseMapeo(t)
	ctx := context.Background()

	// El caso real, en chiquito y con la misma forma: una partida de muchos movimientos chicos y
	// otra de pocos movimientos grandes. 60 × ₡1.000 = ₡60.000 contra 3 × ₡500.000 = ₡1.500.000.
	deposito := sembrarPartida(t, repo, empUno, "Ingresos", NatIngreso, true, "Deposito de Clientes", 60)
	conMonto(t, repo, deposito, "1000")
	traslado := sembrarPartida(t, repo, empUno, "Control", NatNeutro, true, "Traslado de Fondos", 3)
	conMonto(t, repo, traslado, "500000")

	filas, err := repo.FilasDelMapeo(ctx, empUno, false, false)
	if err != nil {
		t.Fatalf("filas del mapeo: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("filas = %d, se esperaban 2", len(filas))
	}

	if filas[0].Clasificacion != "Traslado de Fondos" {
		t.Errorf("primera fila = %q (%d movs, %s colones); se esperaba «Traslado de Fondos». "+
			"La lista volvió a ordenar por cantidad: manda a mapear lo que no mueve plata",
			filas[0].Clasificacion, filas[0].Movimientos, filas[0].MontoCRC)
	}

	// Y el monto tiene que ser el de verdad: es el número con el que se decide por dónde empezar.
	if filas[0].MontoCRC != "1500000.00" {
		t.Errorf("monto de la primera = %q, se esperaba 1500000", filas[0].MontoCRC)
	}
	if filas[0].Movimientos != 3 {
		t.Errorf("movimientos de la primera = %d, se esperaban 3", filas[0].Movimientos)
	}
	if filas[1].MontoCRC != "60000.00" {
		t.Errorf("monto de la segunda = %q, se esperaba 60000", filas[1].MontoCRC)
	}
}

// Una partida que nadie usó no desaparece ni rompe: va al final, en cero.
func TestLaPartidaSinMovimientosVaAlFinalEnCero(t *testing.T) {
	repo := baseMapeo(t)
	ctx := context.Background()

	usada := sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Con plata", 2)
	conMonto(t, repo, usada, "7500")
	sembrarPartida(t, repo, empUno, "Otros", NatGasto, true, "Nadie la usó", 0)

	filas, err := repo.FilasDelMapeo(ctx, empUno, false, false)
	if err != nil {
		t.Fatalf("filas del mapeo: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("filas = %d, se esperaban 2", len(filas))
	}
	if filas[1].Clasificacion != "Nadie la usó" {
		t.Errorf("la última es %q; la que nadie usó tiene que ir al final", filas[1].Clasificacion)
	}
	if filas[1].MontoCRC != "0.00" || filas[1].Movimientos != 0 {
		t.Errorf("la no usada trae monto %q y %d movimientos; se esperaba 0,00 y 0",
			filas[1].MontoCRC, filas[1].Movimientos)
	}

	// Y el filtro de «solo las que se usan» la deja fuera, que es lo que permite que la cobertura
	// llegue al 100 %.
	usadas, err := repo.FilasDelMapeo(ctx, empUno, false, true)
	if err != nil {
		t.Fatalf("filas con movimientos: %v", err)
	}
	if len(usadas) != 1 || usadas[0].Clasificacion != "Con plata" {
		t.Errorf("con el filtro de usadas salieron %d filas; se esperaba solo «Con plata»", len(usadas))
	}
}

// El peso de OTRA empresa no puede sumar acá: sería plata ajena decidiendo el orden de trabajo.
func TestElPesoDelMapeoNoSeMezclaEntreEmpresas(t *testing.T) {
	repo := baseMapeo(t)
	ctx := context.Background()

	clasifID := sembrarPartida(t, repo, empUno, "Gastos", NatGasto, true, "Combustible", 2)
	conMonto(t, repo, clasifID, "1000")

	// La MISMA clasificación, con un movimiento enorme cargado a la OTRA empresa.
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO movimiento_bancario
			(empresa_id, cuenta_bancaria_id, fecha, natural_key, clasificacion_id,
			 debito, monto_crc, incluido)
		VALUES ($1::uuid, gen_random_uuid(), '2026-07-01', 'ajeno', $2::uuid,
		        99000000, 99000000, true)`, empDos, clasifID); err != nil {
		t.Fatalf("sembrar movimiento ajeno: %v", err)
	}

	filas, err := repo.FilasDelMapeo(ctx, empUno, false, false)
	if err != nil {
		t.Fatalf("filas del mapeo: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("filas = %d, se esperaba 1", len(filas))
	}
	if filas[0].MontoCRC != "2000.00" {
		t.Errorf("monto = %q; se coló la plata de la otra empresa (se esperaba 2000,00)", filas[0].MontoCRC)
	}
	if filas[0].Movimientos != 2 {
		t.Errorf("movimientos = %d; se colaron los de la otra empresa", filas[0].Movimientos)
	}
}
