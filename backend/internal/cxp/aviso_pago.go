package cxp

// EL AVISO DE PAGO: el cuadro que se manda por correo cuando se paga (4-nov-2026).
//
// ── EL PEDIDO ───────────────────────────────────────────────────────────────────────────────
//
// «Cada vez que se manda a pagar algo se envía un correo con un cuadro así para los que se van a
// pagar, ¿podés crear el formato en Excel para que salga así?» — Director Financiero. Hoy ese
// cuadro se arma a mano, corte tras corte, con siete columnas: Proveedor · Moneda · Concepto ·
// Documentos · Monto · Cuenta Bancaria · Identificación.
//
// ── DE DÓNDE SALE CADA COSA, Y POR QUÉ IMPORTA ──────────────────────────────────────────────
//
// El aviso se construye desde las MISMAS filas que van al banco (`PagoRow`): cédula, nombre,
// IBAN, moneda y monto neto. No es comodidad, es lo que hace imposible que el papel que recibe el
// proveedor diga algo distinto de lo que salió de la cuenta. Una consulta propia para el aviso se
// desincroniza de la macro en cuanto una de las dos cambie; ésta no puede, porque es la misma.
//
// Lo único que NO está en esas filas es el texto: el concepto que el proveedor reconoce y la
// glosa de los documentos. Eso se propone (del catálogo de gasto y de los consecutivos) y se
// puede corregir; ver la migración 0117.
//
// ── UNA FILA POR PROVEEDOR Y MONEDA, NO POR TRANSFERENCIA ───────────────────────────────────
//
// El cuadro que se manda hoy tiene una fila por proveedor con todos sus documentos juntos, y así
// se arma acá en los dos modos de corte. En el modo POR FACTURA eso significa que una fila puede
// corresponder a varias transferencias, y por eso cada fila lleva su cuenta: el pie del aviso
// dice cuántas transferencias salen en total, que es el número que hay que cuadrar contra el
// banco. Sin ese dato, un aviso de 4 filas sobre 12 transferencias se leería como 4 movimientos.
//
// La moneda parte las filas además del proveedor: un proveedor al que se le paga en colones y en
// dólares son dos líneas en dos archivos distintos, y sumarlas daría un número que no es plata.

import (
	"errors"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	// ErrTextoDeAvisoLargo: el texto no cabe en la celda ni en el correo. Los topes son los
	// mismos de la tabla (mig 0117); se comprueban antes para contestar 422 con una explicación
	// en vez del 500 de una violación de CHECK.
	ErrTextoDeAvisoLargo = errors.New("cxp: el texto del aviso es demasiado largo (concepto 120, documentos 600)")
	// ErrProveedorNoEstaEnElCorte: se intentó corregir el texto de un proveedor que no tiene
	// facturas vivas en ese corte. Decirlo evita guardar un texto que no se vería en ninguna
	// pantalla y aparecería solo mucho después.
	ErrProveedorNoEstaEnElCorte = errors.New("cxp: ese proveedor no tiene facturas en este corte")
)

// TextoAviso es lo que alguien escribió para ese proveedor en ese corte. Los punteros distinguen
// «no se tocó» (nil → usar lo propuesto) de «se borró a propósito» (vacío).
type TextoAviso struct {
	Concepto   *string
	Documentos *string
}

// DatosDelAviso es lo que hay que ir a buscar aparte de las filas del banco: el concepto que
// propone el catálogo y los consecutivos de las facturas cubiertas.
type DatosDelAviso struct {
	// ConceptoPropuesto es la hoja del catálogo de gasto («Suministro de Gas»), no la ruta
	// completa: el cuadro lo lee un proveedor, no un contador.
	ConceptoPropuesto string
	// Consecutivos son los números de las facturas que cubre el pago, en orden.
	Consecutivos []string
}

// FilaAviso es una línea del cuadro.
type FilaAviso struct {
	ProveedorID    string `json:"proveedor_id"`
	Proveedor      string `json:"proveedor"`
	Identificacion string `json:"identificacion"`
	Moneda         string `json:"moneda"`
	Concepto       string `json:"concepto"`
	Documentos     string `json:"documentos"`
	// Monto va en LA MONEDA DE LA FILA, no colonizado: es lo que el banco va a transferir.
	Monto  string `json:"monto"`
	Cuenta string `json:"cuenta"`
	// Editado dice si el texto lo escribió una persona. La pantalla lo usa para no pisar con una
	// propuesta nueva lo que alguien ya corrigió.
	Editado bool `json:"editado"`
	// Transferencias: cuántos movimientos del banco cubre esta fila (>1 solo en modo FACTURA).
	Transferencias int `json:"transferencias"`
	// SinCuenta marca al proveedor sin IBAN. El banco rechaza esa línea, así que el aviso tiene
	// que delatarla ANTES de que alguien lo mande como si estuviera todo bien.
	SinCuenta bool `json:"sin_cuenta"`
}

// AvisoDeMoneda es el cuadro de UNA moneda: el que se manda, y el que corresponde a un archivo.
type AvisoDeMoneda struct {
	Moneda string      `json:"moneda"`
	Filas  []FilaAviso `json:"filas"`
	// Total es la suma de los montos, EN SU MONEDA. No hay total general del corte a propósito:
	// sumar colones con dólares da un número que no significa nada. Mismo criterio que la macro.
	Total string `json:"total"`
	// Transferencias es cuántos movimientos salen al banco en esta moneda: lo que hay que cuadrar
	// contra el estado de cuenta, y que no coincide con la cantidad de filas en el modo FACTURA.
	Transferencias int `json:"transferencias"`
	// SinCuenta cuenta las filas que el banco rechazaría. Va arriba porque es lo único del aviso
	// que hay que arreglar ANTES de mandarlo, no después.
	SinCuenta int `json:"sin_cuenta"`
}

// Firma es quién hizo un paso del corte, según lo que REGISTRÓ EL SISTEMA.
//
// No se teclea: sale de la auditoría y de la ficha del corte. Esa es toda la diferencia con el
// Excel que este papel reemplaza, donde los cuatro nombres se escribían a mano y podían decir
// cualquier cosa. Acá el papel no puede afirmar que alguien aprobó algo que no aprobó.
//
// `Nombre` vacío significa QUE ESE PASO NO SE HIZO EN EL SISTEMA, y así se muestra: en blanco. Es
// decisión del Director Financiero —«si un paso no se hizo en el sistema, la línea sale vacía en
// vez de inventar»— y es lo que convierte un hueco en información en vez de en un olvido.
type Firma struct {
	// Rol es la etiqueta del papel: «Solicitado por», «Revisado por»…
	Rol string `json:"rol"`
	// Nombre puede traer VARIOS separados por « / »: un corte de cien facturas suele pasar por más
	// de un revisor, y poner solo uno diría que el resto no participó.
	Nombre string `json:"nombre"`
	// Fecha es la del paso; la MÁS RECIENTE cuando hubo varias personas, porque es cuando el paso
	// quedó completo para el corte entero.
	Fecha string `json:"fecha"`
}

// Aviso es el cuadro completo de un corte, partido por moneda.
type Aviso struct {
	LoteID     string          `json:"lote_id"`
	LoteNumero int64           `json:"lote_numero"`
	FechaCorte string          `json:"fecha_corte"`
	Monedas    []AvisoDeMoneda `json:"monedas"`
	// Firmas son los cuatro pasos del corte, SIEMPRE los cuatro y en orden, aunque alguno esté
	// vacío: un papel al que le falta la línea de «Aprobado por» se lee distinto de uno donde esa
	// línea está en blanco. El primero dice que nadie pensó en la aprobación; el segundo, que no
	// se hizo.
	Firmas []Firma `json:"firmas"`
	// FechaPago es cuándo salió el dinero (el cierre del corte). Vacía mientras no se haya cerrado.
	FechaPago string `json:"fecha_pago"`
}

// Los cuatro pasos del papel, en el orden en que ocurren.
const (
	RolSolicitado = "Solicitado por"
	RolRevisado   = "Revisado por"
	RolAprobado   = "Aprobado por"
	RolPagado     = "Pagado por"
)

// claveAviso identifica una fila del cuadro: el mismo proveedor en dos monedas son dos filas.
type claveAviso struct{ proveedorID, moneda string }

// ConstruirAviso arma el cuadro desde las filas que van al banco.
//
// `datos` y `textos` se indexan por proveedor; lo que falte simplemente no aporta texto, porque un
// aviso sin glosa sigue siendo un aviso correcto y uno que no se puede emitir no sirve de nada.
func ConstruirAviso(rows []PagoRow, datos map[string]DatosDelAviso, textos map[string]TextoAviso) []AvisoDeMoneda {
	if len(rows) == 0 {
		return nil
	}
	// El orden de llegada se conserva para que el cuadro no baile entre descargas: se agrupa en
	// un mapa pero se recuerda cuándo apareció cada clave.
	orden := []claveAviso{}
	acum := map[claveAviso]*FilaAviso{}
	montos := map[claveAviso]decimal.Decimal{}
	consecutivos := map[claveAviso][]string{}

	for _, r := range rows {
		moneda := normalizarMoneda(r.Moneda)
		k := claveAviso{proveedorID: r.ProveedorID, moneda: moneda}
		f, ok := acum[k]
		if !ok {
			f = &FilaAviso{
				ProveedorID:    k.proveedorID,
				Proveedor:      strings.TrimSpace(r.Nombre),
				Identificacion: strings.TrimSpace(r.Cedula),
				Moneda:         moneda,
				Cuenta:         strings.TrimSpace(r.IBAN),
			}
			f.SinCuenta = f.Cuenta == ""
			acum[k] = f
			orden = append(orden, k)
		}
		f.Transferencias++
		if v, err := decimal.NewFromString(strings.TrimSpace(r.MontoNeto)); err == nil {
			montos[k] = montos[k].Add(v)
		}
		if c := strings.TrimSpace(r.Consecutivo); c != "" {
			consecutivos[k] = append(consecutivos[k], c)
		}
	}

	porMoneda := map[string][]FilaAviso{}
	for _, k := range orden {
		f := acum[k]
		f.Monto = montos[k].StringFixed(2)
		d := datos[k.proveedorID]
		f.Concepto = d.ConceptoPropuesto
		f.Documentos = documentosPropuestos(d.Consecutivos, consecutivos[k])
		if t, ok := textos[k.proveedorID]; ok {
			if t.Concepto != nil {
				f.Concepto, f.Editado = strings.TrimSpace(*t.Concepto), true
			}
			if t.Documentos != nil {
				f.Documentos, f.Editado = strings.TrimSpace(*t.Documentos), true
			}
		}
		porMoneda[k.moneda] = append(porMoneda[k.moneda], *f)
	}

	out := make([]AvisoDeMoneda, 0, len(porMoneda))
	for moneda, filas := range porMoneda {
		total := decimal.Zero
		am := AvisoDeMoneda{Moneda: moneda, Filas: filas}
		for _, f := range filas {
			if v, err := decimal.NewFromString(f.Monto); err == nil {
				total = total.Add(v)
			}
			am.Transferencias += f.Transferencias
			if f.SinCuenta {
				am.SinCuenta++
			}
		}
		am.Total = total.StringFixed(2)
		out = append(out, am)
	}
	// Colones primero, igual que la macro: es la moneda de casi toda la operación y quien baja
	// los dos archivos espera encontrarla arriba.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Moneda == "CRC" {
			return true
		}
		if out[j].Moneda == "CRC" {
			return false
		}
		return out[i].Moneda < out[j].Moneda
	})
	return out
}

// normalizarMoneda trata la moneda vacía como colones, por la misma razón que la macro: así
// nacieron las 4.542 facturas que existen, y dejarlas como moneda desconocida las sacaría del
// cuadro sin que nadie lo note.
func normalizarMoneda(m string) string {
	m = strings.ToUpper(strings.TrimSpace(m))
	if m == "" {
		return "CRC"
	}
	return m
}

// documentosPropuestos arma «FC 0012 0013 0014» con los consecutivos del pago.
//
// Prefiere los que vienen del detalle de las facturas (`deLasFacturas`) sobre los de las filas del
// banco: en el corte agrupado la fila del banco lleva la referencia de la remesa, no los
// consecutivos, y el cuadro tiene que nombrar las facturas que el proveedor emitió.
func documentosPropuestos(deLasFacturas, deLasFilas []string) string {
	fuente := deLasFacturas
	if len(fuente) == 0 {
		fuente = deLasFilas
	}
	vistos := map[string]bool{}
	limpios := make([]string, 0, len(fuente))
	for _, c := range fuente {
		c = strings.TrimSpace(c)
		if c == "" || vistos[c] {
			continue
		}
		vistos[c] = true
		limpios = append(limpios, c)
	}
	if len(limpios) == 0 {
		return ""
	}
	return "FC " + strings.Join(limpios, " ")
}
