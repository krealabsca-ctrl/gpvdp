package bancos

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

// fakeRepo implementa Repository en memoria para probar el servicio.
type fakeRepo struct {
	cuenta   Cuenta
	existing map[string]bool
	// Tipo de cambio: en cero se comportan como los stubs originales.
	cotizaciones     []Cotizacion
	tcEstado         string
	tcValorCongelado *string
	tcAplicado       []tcAplicacion
	// Dashboard: totales por período (ingreso/gasto según la naturaleza del concepto) y cuántos
	// conceptos en uso siguen sin declararla.
	totales                map[string]TotalesEbitda
	conceptosSinNaturaleza int
	naturalezaActual       string
	// Clasificación en bloque desde Excel
	cuentasLista  []CuentaListItem
	movsCalzados  []MovimientoCalzado
	asignados     []AsignacionClasif
	plantillaMovs []MovimientosParaPlantilla
	// Análisis de partidas en el tiempo (tendencia y desvío contra su propio promedio)
	saludMeses    []SaludMes
	seriePartidas []TendenciaPartida
	// Resumen de la selección activa (hoja de trabajo)
	resumenFiltro []ResumenFiltroRow
	inserted      []MovimientoParaInsertar
	// Tesorería (saldos diarios y checklist de carga)
	saldosDia       []SaldoDelDia
	serieSaldos     []PuntoSaldo
	carga           []CargaCuenta
	hoyCR           string
	saldosGuardados []SaldoInput
	fechaGuardada   string
	// Conciliación bancaria mensual
	actas          []ActaConciliacion
	partidas       []PartidaConciliacion
	partidaCreada  PartidaInput
	signoUsado     int
	partidaAnulada string
	actaFirmada    string
	snapshotFirma  [3]string
	fechaRevisada  string
	congelo        bool
	cerrado        bool
	// Descubridor de patrones
	lineasSinClasif []LineaSinClasificar
	descripciones   []string
	// Huella Bancos↔CxP
	movsConHuella []MovimientoConHuella
	yaEnlazados   map[string]string
	// Diccionario del catálogo: foto actual y contadores de lo que se escribió.
	conceptosCat     []Concepto
	clasifsCat       []ClasificacionItem
	reglasCat        []Regla
	conceptosCreados int
	clasifsCreadas   int
	reglasCreadas    int
	ultimaRegla      NuevaRegla
}

func (f *fakeRepo) ListarCuentas(context.Context, string, bool) ([]CuentaListItem, error) {
	return f.cuentasLista, nil
}
func (f *fakeRepo) CuentaByID(context.Context, string, string) (Cuenta, error) { return f.cuenta, nil }
func (f *fakeRepo) CrearImportacion(context.Context, string, string, string, string, Banco, []byte, string) (string, error) {
	return "imp-1", nil
}
func (f *fakeRepo) ImportacionArchivo(context.Context, string, string) (string, []byte, error) {
	return f.cuenta.ID, nil, nil
}
func (f *fakeRepo) NaturalKeysExistentes(_ context.Context, _ string, keys []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, k := range keys {
		if f.existing[k] {
			out[k] = true
		}
	}
	return out, nil
}
func (f *fakeRepo) ConfirmarConMovimientos(_ context.Context, _, _, _, _ string, movs []MovimientoParaInsertar) (int, error) {
	f.inserted = movs
	return len(movs), nil
}
func (f *fakeRepo) SetCuentaIBANSiVacio(context.Context, string, string, string) error { return nil }

func mp(fecha, doc, deb, cred string, indice int) MovimientoParsed {
	t, _ := time.Parse("2006-01-02", fecha)
	return MovimientoParsed{
		Fecha: t, Documento: doc, Debito: dec(deb), Credito: dec(cred), IndiceOcurrencia: indice,
	}
}

func TestClasificar(t *testing.T) {
	const cuentaID = "cta-1"
	movA := mp("2026-06-01", "1", "0", "100.00", 1) // nuevo
	movB := mp("2026-06-02", "2", "50.00", "0", 1)  // dup real (misma tupla que C)
	movC := mp("2026-06-02", "2", "50.00", "0", 2)  // dup real
	movD := mp("2026-06-03", "3", "0", "200.00", 1) // reimportación
	res := ParseResult{Banco: BancoBN, Movimientos: []MovimientoParsed{movA, movB, movC, movD}}

	repo := &fakeRepo{existing: map[string]bool{naturalKey(cuentaID, movD): true}}
	svc := NewService(repo, nil, zap.NewNop(), true)

	resumen, movs, err := svc.clasificar(context.Background(), "emp", cuentaID, "CRC", res)
	if err != nil {
		t.Fatalf("clasificar: %v", err)
	}
	if resumen.Leidas != 4 || resumen.Nuevas != 1 || resumen.DuplicadosReales != 2 || resumen.Reimportacion != 1 {
		t.Fatalf("resumen = %+v", resumen)
	}
	if movs[0].EstadoDuplicado != DupNuevo {
		t.Errorf("mov A = %s, quería NUEVO", movs[0].EstadoDuplicado)
	}
	if movs[1].EstadoDuplicado != DupReal || movs[2].EstadoDuplicado != DupReal {
		t.Errorf("mov B/C = %s/%s, quería DUPLICADO_REAL", movs[1].EstadoDuplicado, movs[2].EstadoDuplicado)
	}
	if movs[3].EstadoDuplicado != DupReimport {
		t.Errorf("mov D = %s, quería REIMPORTACION", movs[3].EstadoDuplicado)
	}
}

func TestSeleccionarParaInsertar(t *testing.T) {
	const cuentaID = "cta-1"
	normal := mp("2026-06-01", "1", "0", "100.00", 1)
	reimport := mp("2026-06-02", "2", "50.00", "0", 1)
	excluido := mp("2026-06-03", "3", "0", "300.00", 1)
	res := ParseResult{Movimientos: []MovimientoParsed{normal, reimport, excluido}}

	existing := map[string]bool{naturalKey(cuentaID, reimport): true}
	excluir := map[string]bool{naturalKey(cuentaID, excluido): true}

	got := seleccionarParaInsertar(cuentaID, "CRC", res, existing, excluir)
	if len(got) != 1 {
		t.Fatalf("seleccionados = %d, quería 1 (solo el nuevo)", len(got))
	}
	if !got[0].MontoOriginal.Equal(dec("100.00")) || !got[0].MontoCRC.Equal(dec("100.00")) {
		t.Errorf("CRC: monto_original=%s monto_crc=%s, quería 100/100", got[0].MontoOriginal, got[0].MontoCRC)
	}

	// USD: monto_crc queda en 0 (pendiente del motor de TC).
	usd := seleccionarParaInsertar(cuentaID, "USD", ParseResult{Movimientos: []MovimientoParsed{normal}}, nil, nil)
	if !usd[0].MontoOriginal.Equal(dec("100.00")) || !usd[0].MontoCRC.Equal(dec("0")) {
		t.Errorf("USD: monto_original=%s monto_crc=%s, quería 100/0", usd[0].MontoOriginal, usd[0].MontoCRC)
	}
}
