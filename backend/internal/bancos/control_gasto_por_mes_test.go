package bancos

// «En qué gastó» tiene que decir también CUÁNDO (6-oct-2026).
//
// El Director miraba Control y presupuesto con el rango Julio–Setiembre y el desglose por partida le
// daba un solo número por partida: *«si quiero ver qué partida gastó más no me dice, y en qué mes»*.
// El orden por gasto ya existía, pero el total de tres meses esconde la forma: en sus datos reales
// el gas venía cayendo (1,68 M → 1,12 M → 294 K) y la joyería subiendo (153 K → 247 K → 313 K), y
// las dos filas se veían igual de planas.
//
// Contra Postgres de verdad porque lo que cambió es el SQL: el agregado pasó a hacerse en dos
// niveles (por mes y después el total). Las otras pruebas de dimensiones corren contra un doble, así
// que esta consulta no se ejecutaba en ninguna — el mismo descuido que ya costó un 500 con MIN(uuid).

import (
	"context"
	"testing"
)

const (
	cgDepto   = "c1000000-0000-4000-8000-000000000001"
	cgConcept = "c1000000-0000-4000-8000-000000000002"
	cgGas     = "c1000000-0000-4000-8000-000000000003"
	cgJoyeria = "c1000000-0000-4000-8000-000000000004"
)

// baseControlGasto siembra un departamento con dos partidas y su gasto repartido en tres meses,
// con la misma forma que tenían los datos reales del incidente.
func baseControlGasto(t *testing.T) (*pgRepository, *Service) {
	t.Helper()
	repo, svc, _ := baseMiPartida(t)
	ctx := context.Background()

	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO departamento (id, empresa_id, nombre) VALUES ($1::uuid, $2::uuid, 'Logística')`,
			[]any{cgDepto, empresaPrueba}},
		{`INSERT INTO concepto (id, empresa_id, nombre, naturaleza) VALUES ($1::uuid, $2::uuid, 'Gastos', 'GASTO')`,
			[]any{cgConcept, empresaPrueba}},
		{`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre, departamento_id)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'Gas', $4::uuid)`,
			[]any{cgGas, empresaPrueba, cgConcept, cgDepto}},
		{`INSERT INTO clasificacion (id, empresa_id, concepto_id, nombre, departamento_id)
		  VALUES ($1::uuid, $2::uuid, $3::uuid, 'Joyeria', $4::uuid)`,
			[]any{cgJoyeria, empresaPrueba, cgConcept, cgDepto}},
	} {
		if _, err := repo.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("sembrar catálogo: %v", err)
		}
	}

	// El gas cae y la joyería sube: el total de los tres meses es casi el mismo y la forma, opuesta.
	gastos := []struct {
		clasif, fecha, monto string
	}{
		{cgGas, "2026-07-10", "1687962.45"},
		{cgGas, "2026-08-11", "1126581.20"},
		{cgGas, "2026-09-12", "294722.00"},
		{cgJoyeria, "2026-07-13", "153000.00"},
		{cgJoyeria, "2026-08-14", "247000.00"},
		{cgJoyeria, "2026-09-15", "313000.00"},
	}
	for i, g := range gastos {
		if _, err := repo.pool.Exec(ctx, `
			INSERT INTO movimiento_bancario
				(empresa_id, cuenta_bancaria_id, fecha, documento, descripcion, debito, credito,
				 moneda_original, monto_original, monto_crc, concepto_id, clasificacion_id,
				 estado_clasificacion, natural_key, incluido, es_traslado)
			VALUES ($1::uuid, $2::uuid, $3::date, $4, 'COMPRA', $5::numeric, 0,
			        'CRC', $5::numeric, $5::numeric, $6::uuid, $7::uuid, 'REVISADO', $4, true, false)`,
			empresaPrueba, cuentaBuena, g.fecha, "cg-"+string(rune('a'+i)), g.monto, cgConcept, g.clasif); err != nil {
			t.Fatalf("sembrar gasto %d: %v", i, err)
		}
	}
	return repo, svc
}

// porNombre indexa las filas devueltas para poder leerlas sin depender del orden.
func porNombre(filas []GastoPartidaDimension) map[string]GastoPartidaDimension {
	out := map[string]GastoPartidaDimension{}
	for _, f := range filas {
		out[f.Clasificacion] = f
	}
	return out
}

func TestElDesgloseDeUnaPartidaTraeSuGastoMesAMes(t *testing.T) {
	_, svc := baseControlGasto(t)

	filas, err := svc.PartidasDeDimension(context.Background(), empresaPrueba,
		"2026-07", "2026-09", AgruparPorDepartamento, cgDepto, "")
	if err != nil {
		t.Fatalf("partidas de la dimensión: %v", err)
	}
	m := porNombre(filas)

	gas, ok := m["Gas"]
	if !ok {
		t.Fatalf("no vino la partida Gas; vinieron %d filas", len(filas))
	}
	// El total del rango no cambia: lo nuevo se suma, no reemplaza.
	if gas.Gasto != "3109265.65" {
		t.Errorf("total de Gas = %q, quiere 3109265.65", gas.Gasto)
	}
	if gas.Movs != 3 {
		t.Errorf("Gas tiene %d movimientos, se sembraron 3", gas.Movs)
	}
	// Y ahora se ve la FORMA, que es lo que el total escondía.
	if len(gas.Meses) != 3 {
		t.Fatalf("Gas trajo %d meses, se esperaban 3: %+v", len(gas.Meses), gas.Meses)
	}
	quiere := []GastoMes{
		{Periodo: "2026-07", Movs: 1, Gasto: "1687962.45"},
		{Periodo: "2026-08", Movs: 1, Gasto: "1126581.20"},
		{Periodo: "2026-09", Movs: 1, Gasto: "294722.00"},
	}
	for i, q := range quiere {
		if gas.Meses[i] != q {
			t.Errorf("mes %d de Gas = %+v, quiere %+v", i, gas.Meses[i], q)
		}
	}
}

func TestElMesPicoSeñalaDondeSeConcentroElGasto(t *testing.T) {
	_, svc := baseControlGasto(t)

	filas, err := svc.PartidasDeDimension(context.Background(), empresaPrueba,
		"2026-07", "2026-09", AgruparPorDepartamento, cgDepto, "")
	if err != nil {
		t.Fatalf("partidas de la dimensión: %v", err)
	}
	m := porNombre(filas)

	// Las dos partidas gastan en los mismos tres meses y su pico es OPUESTO. Si el pico se
	// calculara mal (el primero, el último, el de más movimientos) las dos darían igual.
	if m["Gas"].MesPico != "2026-07" {
		t.Errorf("el pico de Gas es %q y gastó más en julio", m["Gas"].MesPico)
	}
	if m["Joyeria"].MesPico != "2026-09" {
		t.Errorf("el pico de Joyeria es %q y gastó más en setiembre", m["Joyeria"].MesPico)
	}
}

func TestConUnSoloMesNoSeHablaDeMesPico(t *testing.T) {
	_, svc := baseControlGasto(t)

	filas, err := svc.PartidasDeDimension(context.Background(), empresaPrueba,
		"2026-08", "2026-08", AgruparPorDepartamento, cgDepto, "")
	if err != nil {
		t.Fatalf("partidas de la dimensión: %v", err)
	}
	for _, f := range filas {
		// «El mes más alto es agosto» cuando solo se pidió agosto no informa nada.
		if f.MesPico != "" {
			t.Errorf("%s: con un solo mes en el rango se nombró un pico (%q)", f.Clasificacion, f.MesPico)
		}
		if len(f.Meses) != 1 {
			t.Errorf("%s: trajo %d meses pidiendo uno", f.Clasificacion, len(f.Meses))
		}
	}
}

// El recorte del rango tiene que llegar también al desglose mensual: un mes de afuera no se asoma.
func TestElDesgloseMensualRespetaElRango(t *testing.T) {
	_, svc := baseControlGasto(t)

	filas, err := svc.PartidasDeDimension(context.Background(), empresaPrueba,
		"2026-08", "2026-09", AgruparPorDepartamento, cgDepto, "")
	if err != nil {
		t.Fatalf("partidas de la dimensión: %v", err)
	}
	gas := porNombre(filas)["Gas"]
	for _, mes := range gas.Meses {
		if mes.Periodo < "2026-08" || mes.Periodo > "2026-09" {
			t.Errorf("se coló el mes %s, fuera del rango pedido", mes.Periodo)
		}
	}
	// Y el total tiene que ser el de los DOS meses, no el de los tres.
	if gas.Gasto != "1421303.20" {
		t.Errorf("total de Gas en ago-set = %q, quiere 1421303.20 (sin julio)", gas.Gasto)
	}
}
