package bancos

import (
	"errors"
	"strconv"
)

func itoa(n int) string     { return strconv.Itoa(n) }
func quote(s string) string { return strconv.Quote(s) }

var (
	// ErrCuentaNoEncontrada indica que la cuenta no existe o no pertenece a la empresa.
	ErrCuentaNoEncontrada = errors.New("bancos: cuenta no encontrada")
	// ErrImportacionNoEncontrada indica que la importación no existe o no pertenece a la empresa.
	ErrImportacionNoEncontrada = errors.New("bancos: importación no encontrada")
	// ErrMovimientoNoEncontrado indica que el movimiento no existe o no pertenece a la empresa.
	ErrMovimientoNoEncontrado = errors.New("bancos: movimiento no encontrado")
	// ErrClasificacionInvalida indica que la clasificación no pertenece al concepto (o a la empresa).
	ErrClasificacionInvalida = errors.New("bancos: la clasificación no corresponde al concepto")
	// ErrTCYaCongelado indica que el TC del mes ya fue congelado (es inmutable, RN-13).
	ErrTCYaCongelado = errors.New("bancos: el tipo de cambio del mes ya está congelado")
	// ErrCotizacionesIncompletas indica que faltan cotizaciones (día 1/15/último) para congelar.
	ErrCotizacionesIncompletas = errors.New("bancos: faltan cotizaciones del mes (día 1, 15 y último) para congelar")
	// ErrTrasladoInvalido indica que el par no es un traslado válido (misma cuenta, no opuestos, o fuera de tolerancia).
	ErrTrasladoInvalido = errors.New("bancos: par de traslado inválido")
	// ErrPeriodoYaCerrado indica que el período ya fue cerrado.
	ErrPeriodoYaCerrado = errors.New("bancos: el período ya está cerrado")
	// ErrPeriodoConNoIdentificados indica que no se puede cerrar con movimientos No identificado (RN-22).
	ErrPeriodoConNoIdentificados = errors.New("bancos: hay movimientos No identificado pendientes; no se puede cerrar el período")
	// ErrConceptoNoEncontrado indica que el concepto no existe o no pertenece a la empresa.
	ErrConceptoNoEncontrado = errors.New("bancos: concepto no encontrado")
	// ErrNaturalezaInvalida: la naturaleza del concepto debe ser INGRESO, GASTO o NEUTRO.
	ErrNaturalezaInvalida = errors.New("bancos: la naturaleza debe ser INGRESO, GASTO o NEUTRO")
	// ErrCatalogoDuplicado indica que ya existe un concepto/clasificación con ese nombre.
	ErrCatalogoDuplicado = errors.New("bancos: ya existe una entrada de catálogo con ese nombre")
	// ErrIBANNoCoincide indica que el IBAN del archivo no coincide con el de la cuenta elegida
	// (evita importar el estado de cuenta en la cuenta equivocada y duplicar/mezclar datos).
	ErrIBANNoCoincide = errors.New("bancos: el IBAN del archivo no coincide con la cuenta seleccionada")
	// ErrBancoNoEncontrado indica que el banco no existe o no pertenece a la empresa.
	ErrBancoNoEncontrado = errors.New("bancos: banco no encontrado")
	// ErrReglaNoEncontrada indica que la regla no existe o no pertenece a la empresa.
	ErrReglaNoEncontrada = errors.New("bancos: regla no encontrada")
	// ErrReglaSinPalabras indica que la edición dejaría a la regla sin palabras clave (no clasificaría nada).
	ErrReglaSinPalabras = errors.New("bancos: la regla debe conservar al menos una palabra clave")
	// ErrClasificacionNoEncontrada indica que la clasificación no existe o no pertenece a la empresa.
	ErrClasificacionNoEncontrada = errors.New("bancos: clasificación no encontrada")
	// ErrEmpresaNoEncontrada indica que la empresa no existe.
	ErrEmpresaNoEncontrada = errors.New("bancos: empresa no encontrada")
	// ErrToleranciaFueraDeRango indica una tolerancia de traslado fuera de 0–5%.
	ErrToleranciaFueraDeRango = errors.New("bancos: la tolerancia de traslado debe estar entre 0% y 5%")
	// ErrBCCRNoConfigurado indica que falta la configuración/credenciales del web service del BCCR.
	ErrBCCRNoConfigurado = errors.New("bancos: la sincronización con el BCCR no está configurada (correo/token)")
	// ErrMonedaNoCoincide indica que la moneda del archivo no coincide con la de la cuenta seleccionada.
	ErrMonedaNoCoincide = errors.New("bancos: la moneda del archivo no coincide con la de la cuenta seleccionada")
	// ErrFechaInvalida indica una fecha con formato inválido (se espera YYYY-MM-DD).
	ErrFechaInvalida = errors.New("bancos: fecha inválida (se espera YYYY-MM-DD)")
	// ErrExportacionVacia indica que el filtro de exportación no arrojó filas (§33).
	ErrExportacionVacia = errors.New("bancos: no hay datos para exportar con ese filtro")
)

// FechasIlegiblesError indica que el archivo SÍ se reconoció (banco y encabezado correctos)
// pero no se pudo leer la fecha de NINGUNA de sus filas de movimiento.
//
// Existe porque el silencio costó caro: el adaptador descarta las filas sin fecha a propósito
// (saldo inicial, subtotales, pie de página), y cuando el Banco Popular cambió los meses a
// inglés esa misma regla se tragó el archivo entero. El importador reportó «0 movimientos»,
// que es exactamente lo que se ve cuando el banco no mandó nada, y nadie podía distinguir un
// mes sin actividad de un formato que el ERP dejó de entender.
//
// La diferencia entre las dos cosas la sabe el parser y nadie más, así que la tiene que decir.
type FechasIlegiblesError struct {
	Banco Banco
	// Filas es cuántas filas se descartaron por no poder leerles la fecha.
	Filas int
	// Muestra es el texto crudo de la primera celda de fecha que no se entendió: es el dato
	// con el que se arregla el adaptador sin tener que pedir el archivo.
	Muestra string
}

func (e *FechasIlegiblesError) Error() string {
	return "bancos: el archivo se reconoció como " + string(e.Banco) +
		" pero no se pudo leer la fecha de ninguna de sus " + itoa(e.Filas) +
		" filas de movimiento (la primera dice " + quote(e.Muestra) + "). " +
		"Es un cambio de formato del banco, no un mes sin movimientos: hay que ajustar el adaptador."
}

// CatalogoEnUsoError indica que un concepto/clasificación tiene referencias y no puede
// eliminarse; Detalle enumera qué lo usa para que el usuario reclasifique primero.
type CatalogoEnUsoError struct{ Detalle string }

func (e *CatalogoEnUsoError) Error() string {
	return "bancos: la entrada del catálogo está en uso: " + e.Detalle
}

// CambioNoPermitidoError indica que el dato existe pero NO se puede cambiar por lo que ya
// hay encima (p. ej. la moneda de una cuenta que ya tiene movimientos importados). El
// mensaje va completo y sin prefijo: dice qué pasa y qué hacer en su lugar.
type CambioNoPermitidoError struct{ Motivo string }

func (e *CambioNoPermitidoError) Error() string { return "bancos: " + e.Motivo }
