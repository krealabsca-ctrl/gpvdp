package cxc

// Reimportar la cartera ACTUALIZA lo que viene y CONSERVA lo que no (13-oct-2026).
//
// EL PEDIDO del Director Financiero: «como siempre se actualizan los datos, y la fecha del
// próximo pago cambia según entran los pagos, ¿es posible que el importador, en lugar de
// sobreescribirse, solo escriba los cambios?».
//
// EL RIESGO QUE TENÍA. El upsert hacía `campo = EXCLUDED.campo` para casi todo. Un archivo que
// no trajera una columna la mandaba vacía y PISABA el dato bueno. Medido sobre la base real
// antes del arreglo: 12.231 teléfonos y 11.513 correos se habrían borrado con un recorte que no
// los incluyera, y una cuota ilegible —que el lector deja en 0— habría dejado al contrato sin
// generar cargos NUNCA MÁS. La deuda no se vería mal: se volvería invisible.
//
// Va contra Postgres porque lo que se prueba es el ON CONFLICT, que un doble no tiene.

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

func resolucionVacia() Resolucion {
	return Resolucion{
		Sedes: map[string]string{}, Modalidades: map[string]string{},
		FormasPago: map[string]string{}, Asociaciones: map[string]string{},
	}
}

// completo es el contrato tal como entra la primera vez, con todos sus datos.
func completo() FilaContrato {
	return FilaContrato{
		Numero: "CO500001", Cliente: "YORYINA", Documento: "155811942016",
		Telefonos: "8888-0000", Correos: "yoryina@correo.cr",
		Servicio: "Plan funerario", TipoServicio: "Individual",
		Cuota: decimal.RequireFromString("25000"),
		//

		PrimerCobro: "2024-01-15", ProximoPago: "2026-10-01",
	}
}

func TestReimportarParcialNoBorraLoQueNoTrae(t *testing.T) {
	repo, empresaID := baseDeContratos(t)
	ctx := context.Background()

	if _, err := repo.GuardarContratos(ctx, empresaID, []FilaContrato{completo()}, resolucionVacia()); err != nil {
		t.Fatalf("primera carga: %v", err)
	}

	// El recorte: solo el número y la cuota nueva. Todo lo demás llega vacío.
	recorte := FilaContrato{Numero: "CO500001", Cuota: decimal.RequireFromString("27000")}
	if _, err := repo.GuardarContratos(ctx, empresaID, []FilaContrato{recorte}, resolucionVacia()); err != nil {
		t.Fatalf("reimportación parcial: %v", err)
	}

	var cliente, doc, tels, correos, servicio, cuota, proximo string
	if err := repo.pool.QueryRow(ctx, `
		SELECT cliente_nombre, documento, telefonos, correos, servicio, cuota_vigente::text,
		       COALESCE(fecha_proximo_pago::text,'')
		  FROM contrato_cxc WHERE empresa_id = $1::uuid AND numero = 'CO500001'`,
		empresaID).Scan(&cliente, &doc, &tels, &correos, &servicio, &cuota, &proximo); err != nil {
		t.Fatalf("leer: %v", err)
	}

	// Lo que el recorte SÍ traía, cambió.
	if decimal.RequireFromString(cuota).Cmp(decimal.RequireFromString("27000")) != 0 {
		t.Errorf("cuota = %s, quería 27000: lo que el archivo trae tiene que actualizarse", cuota)
	}
	// Lo que NO traía, se conservó.
	for _, c := range []struct{ campo, got, want string }{
		{"cliente", cliente, "YORYINA"},
		{"documento", doc, "155811942016"},
		{"teléfonos", tels, "8888-0000"},
		{"correos", correos, "yoryina@correo.cr"},
		{"servicio", servicio, "Plan funerario"},
		{"próximo pago", proximo, "2026-10-01"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q tras la reimportación parcial, quería conservar %q", c.campo, c.got, c.want)
		}
	}
}

// UNA CUOTA ILEGIBLE NO APAGA EL CONTRATO.
//
// El lector deja la cuota en 0 cuando no la entiende. Si ese 0 llegara a la tabla, el contrato
// dejaría de generar cargos y su deuda desaparecería de la cartera sin que nada lo avise.
func TestUnaCuotaEnCeroNoPisaLaCuotaBuena(t *testing.T) {
	repo, empresaID := baseDeContratos(t)
	ctx := context.Background()

	if _, err := repo.GuardarContratos(ctx, empresaID, []FilaContrato{completo()}, resolucionVacia()); err != nil {
		t.Fatalf("primera carga: %v", err)
	}

	malo := completo()
	malo.Cuota = decimal.Zero
	malo.Motivos = []string{"cuota ilegible"}
	if _, err := repo.GuardarContratos(ctx, empresaID, []FilaContrato{malo}, resolucionVacia()); err != nil {
		t.Fatalf("reimportación con cuota ilegible: %v", err)
	}

	var cuota string
	var pendiente bool
	if err := repo.pool.QueryRow(ctx,
		`SELECT cuota_vigente::text, revision_pendiente FROM contrato_cxc
		  WHERE empresa_id = $1::uuid AND numero = 'CO500001'`, empresaID).Scan(&cuota, &pendiente); err != nil {
		t.Fatalf("leer: %v", err)
	}
	if decimal.RequireFromString(cuota).Sign() == 0 {
		t.Error("la cuota quedó en cero: el contrato dejaría de generar cargos y su deuda se volvería invisible")
	}
	if !pendiente {
		t.Error("tampoco quedó marcado para revisión: el problema del archivo no se ve por ningún lado")
	}
}

// LA FECHA DE PRÓXIMO PAGO SÍ AVANZA cuando el archivo la trae: es el caso normal, el cliente
// pagó y su próxima cuota se corrió un mes.
func TestLaFechaDeProximoPagoAvanzaConCadaArchivo(t *testing.T) {
	repo, empresaID := baseDeContratos(t)
	ctx := context.Background()

	if _, err := repo.GuardarContratos(ctx, empresaID, []FilaContrato{completo()}, resolucionVacia()); err != nil {
		t.Fatalf("primera carga: %v", err)
	}
	conPago := completo()
	conPago.ProximoPago = "2026-11-01" // pagó octubre
	if _, err := repo.GuardarContratos(ctx, empresaID, []FilaContrato{conPago}, resolucionVacia()); err != nil {
		t.Fatalf("segunda carga: %v", err)
	}

	var proximo string
	if err := repo.pool.QueryRow(ctx,
		`SELECT fecha_proximo_pago::text FROM contrato_cxc
		  WHERE empresa_id = $1::uuid AND numero = 'CO500001'`, empresaID).Scan(&proximo); err != nil {
		t.Fatalf("leer: %v", err)
	}
	if proximo != "2026-11-01" {
		t.Errorf("próximo pago = %s, quería 2026-11-01: la fecha avanza con cada pago", proximo)
	}
}
