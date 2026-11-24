package cxp

// El cronograma de pago: lo que se manda ANTES de pagar (mig 0122).
//
// «Los cronogramas de pago se envían por correo con lo que se pretende pagar y los bancos de donde
// salen los fondos.»
//
// LA CASCADA ES EL DOCUMENTO. El cuadro que esto reemplaza no es una lista: es una lista Y cuatro
// restas encadenadas, y lo que se comunica es el último número. Verificado contra el archivo real:
//
//	  Colones + Dólares colonizados  =  52.110.734,73   «TOTAL COLONIZADO»
//	  − Reserva                      =  51.888.184,73   «TOTAL FLUJO NORMAL»
//	  − Solicitudes por fuera        =  10.932.828,47   «CRONOGRAMA TOTAL FLUJO NORMAL»
//	  − Dólares                      =  10.604.337,47   «CRONOGRAMA COLONES FLUJO NORMAL»
//
// Cada paso cierra al céntimo. Por eso la cascada se calcula acá, en Go y con `decimal`, y no en
// la consulta: los cuatro escalones tienen que poder probarse uno por uno sin una base delante, y
// un redondeo intermedio distinto haría que el último número —el único que se comunica— no cuadre.

import (
	"errors"

	"github.com/shopspring/decimal"
)

var (
	// ErrTCCronogramaRequerido: hay dólares en el corte y nadie dijo a cuánto colonizarlos.
	//
	// No se inventa con el TC del mes: el del archivo real es 450,0000 exacto, un número que
	// elige quien arma el cuadro para proyectar. Usar otro cambiaría el total que se comunica.
	ErrTCCronogramaRequerido = errors.New("cxp: este corte tiene pagos en dólares; escribí el tipo de cambio con el que se colonizan")
	// ErrTCCronogramaInvalido: el valor no es un número positivo.
	ErrTCCronogramaInvalido = errors.New("cxp: el tipo de cambio tiene que ser un número mayor que cero")
)

// LineaCronograma es un proveedor con lo que se le va a pagar, y de dónde sale.
type LineaCronograma struct {
	ProveedorID string `json:"proveedor_id"`
	Proveedor   string `json:"proveedor"`
	Moneda      string `json:"moneda"`
	Monto       string `json:"monto"`
	// MontoColonizado es el monto llevado a colones con el TC del cronograma. En colones es el
	// mismo número; en dólares es lo que de verdad suma a los totales del cuadro.
	MontoColonizado string `json:"monto_colonizado"`
	// Cuenta es de dónde sale. Vacía si todavía no se decidió.
	Cuenta string `json:"cuenta"`
	Banco  string `json:"banco"`
	// EsReserva: sale de una cuenta marcada como reserva, así que no es flujo normal.
	EsReserva bool `json:"es_reserva"`
	// PorFuera: se paga igual, pero no entra en el cronograma que se comunica.
	PorFuera bool `json:"por_fuera"`
}

// TotalesCronograma son las cuatro restas, en el mismo orden en que las lee quien recibe el cuadro.
//
// Van como STRING y no como número: es dinero (ver CLAUDE.md), y un float en el camino al
// navegador convierte 52.110.734,73 en 52.110.734,729999997.
type TotalesCronograma struct {
	// Las dos puntas de arriba, antes de cualquier resta.
	TotalColones       string `json:"total_colones"`
	TotalDolares       string `json:"total_dolares"`
	DolaresColonizados string `json:"dolares_colonizados"`
	TotalColonizado    string `json:"total_colonizado"`
	// La cascada.
	Reserva             string `json:"reserva"`
	FlujoNormal         string `json:"flujo_normal"`
	PorFuera            string `json:"por_fuera"`
	CronogramaTotal     string `json:"cronograma_total"`
	CronogramaEnColones string `json:"cronograma_en_colones"`
	// TipoCambio es el que se usó, para que el cuadro diga con qué número se colonizó.
	TipoCambio string `json:"tipo_cambio"`
}

// Cronograma es el cuadro completo de un corte.
type Cronograma struct {
	LoteID     string            `json:"lote_id"`
	LoteNumero int64             `json:"lote_numero"`
	FechaCorte string            `json:"fecha_corte"`
	Lineas     []LineaCronograma `json:"lineas"`
	Totales    TotalesCronograma `json:"totales"`
	// SinCuenta cuenta las líneas a las que nadie les asignó cuenta todavía. Van al flujo normal,
	// y por eso hay que decirlo: un cronograma armado con la mitad sin asignar da un número que
	// parece definitivo y no lo es.
	SinCuenta int `json:"sin_cuenta"`
}

// FilaCronogramaCruda es una línea tal como sale de la base, antes de colonizar y sumar.
type FilaCronogramaCruda struct {
	ProveedorID string
	Proveedor   string
	Moneda      string
	Monto       decimal.Decimal
	Cuenta      string
	Banco       string
	EsReserva   bool
	PorFuera    bool
	TieneCuenta bool
}

// ConstruirCronograma arma las líneas y la cascada.
//
// `tc` es el tipo de cambio que escribió tesorería. Si hay dólares y no viene, es un error: sin él
// no se puede sumar el cuadro, y poner cero haría desaparecer esos pagos del total sin avisar.
func ConstruirCronograma(filas []FilaCronogramaCruda, tc decimal.Decimal) (Cronograma, error) {
	hayDolares := false
	for _, f := range filas {
		if normalizarMoneda(f.Moneda) == "USD" {
			hayDolares = true
			break
		}
	}
	if hayDolares && !tc.IsPositive() {
		return Cronograma{}, ErrTCCronogramaRequerido
	}

	var (
		totalCRC, totalUSD, dolaresColonizados decimal.Decimal
		reserva, porFuera                      decimal.Decimal
		lineas                                 = make([]LineaCronograma, 0, len(filas))
		sinCuenta                              int
	)

	for _, f := range filas {
		moneda := normalizarMoneda(f.Moneda)
		// Colonizar: en colones el monto ya está; en dólares se multiplica por el TC del cuadro.
		// Se redondea a dos decimales AQUÍ, una sola vez por línea, para que la suma de la columna
		// que se ve en el papel sea exactamente el total de abajo. Redondear al final daría un
		// total que no cuadra con lo que alguien suma a mano.
		colonizado := f.Monto
		if moneda == "USD" {
			colonizado = f.Monto.Mul(tc).Round(2)
			totalUSD = totalUSD.Add(f.Monto)
			dolaresColonizados = dolaresColonizados.Add(colonizado)
		} else {
			totalCRC = totalCRC.Add(f.Monto)
		}

		// La reserva se mide ANTES que «por fuera» y las dos se restan por separado: una línea
		// puede ser las dos cosas, y contarla dos veces rompería la cascada. Por eso `por_fuera`
		// solo acumula lo que NO es reserva — la reserva ya salió del flujo normal un paso antes.
		switch {
		case f.EsReserva:
			reserva = reserva.Add(colonizado)
		case f.PorFuera:
			porFuera = porFuera.Add(colonizado)
		}

		if !f.TieneCuenta {
			sinCuenta++
		}

		lineas = append(lineas, LineaCronograma{
			ProveedorID: f.ProveedorID, Proveedor: f.Proveedor, Moneda: moneda,
			Monto: f.Monto.StringFixed(2), MontoColonizado: colonizado.StringFixed(2),
			Cuenta: f.Cuenta, Banco: f.Banco, EsReserva: f.EsReserva, PorFuera: f.PorFuera,
		})
	}

	// La cascada, en el mismo orden del papel.
	totalColonizado := totalCRC.Add(dolaresColonizados)
	flujoNormal := totalColonizado.Sub(reserva)
	cronogramaTotal := flujoNormal.Sub(porFuera)
	// El último escalón saca los dólares que QUEDARON en el cronograma, no todos: los que se
	// fueron con la reserva o con lo de por fuera ya no están en `cronogramaTotal`, y restarlos
	// otra vez dejaría el número de abajo más chico de lo que es.
	dolaresEnCronograma := decimal.Zero
	for i, f := range filas {
		if normalizarMoneda(f.Moneda) == "USD" && !f.EsReserva && !f.PorFuera {
			d, _ := decimal.NewFromString(lineas[i].MontoColonizado)
			dolaresEnCronograma = dolaresEnCronograma.Add(d)
		}
	}

	return Cronograma{
		Lineas:    lineas,
		SinCuenta: sinCuenta,
		Totales: TotalesCronograma{
			TotalColones:        totalCRC.StringFixed(2),
			TotalDolares:        totalUSD.StringFixed(2),
			DolaresColonizados:  dolaresColonizados.StringFixed(2),
			TotalColonizado:     totalColonizado.StringFixed(2),
			Reserva:             reserva.StringFixed(2),
			FlujoNormal:         flujoNormal.StringFixed(2),
			PorFuera:            porFuera.StringFixed(2),
			CronogramaTotal:     cronogramaTotal.StringFixed(2),
			CronogramaEnColones: cronogramaTotal.Sub(dolaresEnCronograma).StringFixed(2),
			TipoCambio:          tc.StringFixed(4),
		},
	}, nil
}
