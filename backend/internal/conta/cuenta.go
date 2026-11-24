// Package conta implementa la contabilidad del ERP (mig 0109, Fase 1: el plan de cuentas).
//
// LO QUE ESTE PAQUETE NO HACE, Y ES A PROPÓSITO: no toca ningún módulo existente. Bancos, CxP,
// CxC y nómina no lo conocen ni lo importan. La contabilidad LEE lo que esos módulos producen y
// escribe en sus propias tablas; si este paquete se quitara, el ERP seguiría funcionando igual.
//
// Fase 1 es solo el plan de cuentas: una tabla, su árbol y su importador. Todavía no se
// contabiliza nada.
package conta

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Clases contables. El primer segmento del código decide a cuál pertenece la cuenta.
const (
	ClaseActivo     = "ACTIVO"
	ClasePasivo     = "PASIVO"
	ClasePatrimonio = "PATRIMONIO"
	ClaseIngreso    = "INGRESO"
	ClaseCosto      = "COSTO"
	ClaseGasto      = "GASTO"
	ClaseOrden      = "ORDEN"
)

// Naturalezas. Deudora aumenta por el DEBE; acreedora, por el HABER.
const (
	Deudora   = "DEUDORA"
	Acreedora = "ACREEDORA"
)

// Cuenta es una cuenta del plan.
//
// `EmpresaID` nulo significa que la cuenta es del GRUPO: la comparten las tres empresas. Con
// empresa, es propia de ella. Las dos formas conviven a propósito —ver la migración 0109—
// porque la decisión «plan único o uno por empresa» todavía no está tomada y este modelo no la
// fuerza.
type Cuenta struct {
	ID         string  `json:"id"`
	EmpresaID  *string `json:"empresa_id"`
	Codigo     string  `json:"codigo"`
	Nombre     string  `json:"nombre"`
	PadreID    *string `json:"padre_id"`
	Clase      string  `json:"clase"`
	Naturaleza string  `json:"naturaleza"`
	// AceptaMovimiento: solo estas admiten líneas de asiento. Una cuenta con hijas no puede.
	AceptaMovimiento bool   `json:"acepta_movimiento"`
	Nivel            int    `json:"nivel"`
	CodigoSoftland   string `json:"codigo_softland"`
	Activo           bool   `json:"activo"`
	// Hijas lo llena el servicio al armar el árbol; no es una columna.
	Hijas    []*Cuenta `json:"hijas,omitempty"`
	CreadoEn time.Time `json:"creado_en"`
}

// DelGrupo dice si la cuenta la comparten todas las empresas.
func (c Cuenta) DelGrupo() bool { return c.EmpresaID == nil }

var (
	// ErrCodigoRequerido: el código es la identidad de la cuenta.
	ErrCodigoRequerido = errors.New("conta: la cuenta necesita un código")
	// ErrNombreRequerido: sin nombre, el plan es ilegible.
	ErrNombreRequerido = errors.New("conta: la cuenta necesita un nombre")
	// ErrClaseDesconocida: el primer segmento no corresponde a ninguna clase contable.
	ErrClaseDesconocida = errors.New("conta: el código tiene que empezar por 1 a 7, que es la clase contable")
	// ErrCodigoDuplicado: ya existe una cuenta con ese código.
	ErrCodigoDuplicado = errors.New("conta: ya hay una cuenta con ese código")
	// ErrCuentaNoEncontrada: no existe, o es de otra empresa.
	ErrCuentaNoEncontrada = errors.New("conta: esa cuenta no existe")
	// ErrPadreAceptaMovimiento: no se le pueden colgar hijas a una cuenta de movimiento.
	ErrPadreAceptaMovimiento = errors.New("conta: la cuenta madre acepta movimiento, así que no puede tener hijas: el mismo dinero se sumaría dos veces")
	// ErrTieneHijas: una cuenta con hijas no puede aceptar movimiento, por lo mismo.
	ErrTieneHijas = errors.New("conta: la cuenta tiene hijas, así que no puede aceptar movimiento: el mismo dinero se sumaría dos veces")
	// ErrCuentaConMovimientos: no se borra lo que ya tiene contabilidad encima.
	ErrCuentaConMovimientos = errors.New("conta: la cuenta ya tiene movimientos contables; apagala en vez de borrarla")
)

// claseDelCodigo deriva la clase del primer segmento.
//
// La correspondencia sale del catálogo real del Director Financiero: 1 activo, 2 pasivo,
// 3 patrimonio, 4 ingresos, 5 compras (costo), 6 gastos. El 7 se admite como cuentas de orden
// porque es la convención usual y no cuesta nada dejarlo previsto.
func claseDelCodigo(codigo string) (string, bool) {
	if codigo == "" {
		return "", false
	}
	switch codigo[0] {
	case '1':
		return ClaseActivo, true
	case '2':
		return ClasePasivo, true
	case '3':
		return ClasePatrimonio, true
	case '4':
		return ClaseIngreso, true
	case '5':
		return ClaseCosto, true
	case '6':
		return ClaseGasto, true
	case '7':
		return ClaseOrden, true
	}
	return "", false
}

// naturalezaDeLaClase es el default al importar.
//
// Activo, costo y gasto aumentan por el debe; pasivo, patrimonio e ingreso por el haber. Se
// guarda como columna editable porque hay excepciones legítimas —la depreciación acumulada es
// activo y acreedora— y forzarla desde la clase las haría imposibles de representar.
func naturalezaDeLaClase(clase string) string {
	switch clase {
	case ClaseActivo, ClaseCosto, ClaseGasto:
		return Deudora
	default:
		return Acreedora
	}
}

// segmentos parte «1-01-01-001-000» en sus pedazos.
func segmentos(codigo string) []string { return strings.Split(codigo, "-") }

// nivelDelCodigo cuenta cuántos segmentos del código son significativos.
//
// «1-00-00-000-000» es nivel 1 (ACTIVOS); «1-01-00-000-000» nivel 2; «1-01-01-001-002» nivel 5.
// Un segmento en ceros es relleno de la máscara, no un escalón del árbol.
func nivelDelCodigo(codigo string) int {
	n := 0
	for _, s := range segmentos(codigo) {
		if strings.Trim(s, "0") == "" {
			continue
		}
		n++
	}
	if n == 0 {
		return 1
	}
	return n
}

// codigoDelPadre devuelve el código de la cuenta madre, o "" si es raíz.
//
// Se obtiene poniendo en ceros el último segmento significativo: la madre de «1-01-01-001-002»
// es «1-01-01-001-000», y la de esa, «1-01-01-000-000». Respeta el ancho original de cada
// segmento, porque «00» y «000» no son lo mismo en la máscara del catálogo.
func codigoDelPadre(codigo string) string {
	segs := segmentos(codigo)
	for i := len(segs) - 1; i >= 1; i-- {
		if strings.Trim(segs[i], "0") == "" {
			continue
		}
		copia := append([]string(nil), segs...)
		copia[i] = strings.Repeat("0", len(segs[i]))
		return strings.Join(copia, "-")
	}
	return ""
}

// Repository es lo que el servicio necesita de la base.
type Repository interface {
	// Cuentas devuelve las visibles desde `empresaID`: las suyas y las del grupo.
	Cuentas(ctx context.Context, empresaID string, soloActivas bool) ([]Cuenta, error)
	CuentaPorCodigo(ctx context.Context, empresaID, codigo string) (Cuenta, error)
	CrearCuenta(ctx context.Context, c Cuenta, usuarioID string) (Cuenta, error)
	ActualizarCuenta(ctx context.Context, empresaID string, c Cuenta) (Cuenta, error)
	BorrarCuenta(ctx context.Context, empresaID, id string) error
	// ImportarCuentas reemplaza o completa el plan en UNA transacción. Ver service_importar.go.
	ImportarCuentas(ctx context.Context, destino *string, cuentas []Cuenta, usuarioID string) (ResultadoImportacion, error)

	// El mapeo contable (mig 0110, Fase 2): qué cuenta corresponde a cada partida del ERP.
	FilasDelMapeo(ctx context.Context, empresaID string, soloPendientes, soloConMovimientos bool) ([]FilaDelMapeo, error)
	GuardarMapeo(ctx context.Context, empresaID string, d DecisionDeMapeo, usuarioID string) error
	BorrarMapeo(ctx context.Context, empresaID, clasificacionID string) error
	Cobertura(ctx context.Context, empresaID string) (Cobertura, error)

	// El asiento y la contrapartida bancaria (mig 0111, Fase 3A).
	FilasBanco(ctx context.Context, empresaID string) ([]FilaBancoContable, error)
	GuardarEnlaceBanco(ctx context.Context, empresaID, cuentaBancariaID, cuentaID, usuarioID string) error
	BorrarEnlaceBanco(ctx context.Context, empresaID, cuentaBancariaID string) error

	CrearAsiento(ctx context.Context, a Asiento, lineas []LineaValidada, usuarioID string) (Asiento, error)
	AnularAsiento(ctx context.Context, empresaID, id, motivo, usuarioID string) error
	Diario(ctx context.Context, empresaID string, f FiltroDiario) ([]Asiento, error)
	Asiento(ctx context.Context, empresaID, id string) (Asiento, error)
	Mayor(ctx context.Context, empresaID string, f FiltroMayor) (Mayor, error)

	// El generador por caja (Fase 3B). `MovimientosDelPeriodo` devuelve además, por cada
	// movimiento, si YA tiene un asiento vivo: eso lo sabe la consulta y recalcularlo después
	// costaría otra vuelta a la base.
	MovimientosDelPeriodo(ctx context.Context, empresaID, periodo string) ([]MovimientoAContabilizar, []bool, error)
	GenerarAsientos(ctx context.Context, empresaID string, movs []MovimientoAContabilizar, usuarioID string) (ResultadoGeneracion, error)
	RevertirGeneracion(ctx context.Context, empresaID, periodo, motivo, usuarioID string) (int, error)

	// Apertura, balance y cierre (mig 0113, Fase 4).
	Balance(ctx context.Context, empresaID, desde, hasta string) ([]RenglonBalance, error)
	HayApertura(ctx context.Context, empresaID string) (bool, error)
	Periodos(ctx context.Context, empresaID string) ([]PeriodoContable, error)
	TotalesDelMes(ctx context.Context, empresaID string, anio, mes int) (decimal.Decimal, decimal.Decimal, int, error)
	CerrarPeriodo(ctx context.Context, empresaID string, anio, mes int, debe, haber decimal.Decimal, asientos int, usuarioID string) error
	ReabrirPeriodo(ctx context.Context, empresaID string, anio, mes int, motivo, usuarioID string) error
}
