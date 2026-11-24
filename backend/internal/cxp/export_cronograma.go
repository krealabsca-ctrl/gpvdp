package cxp

// El cronograma de pago como .xlsx: lo que de verdad se manda por correo (mig 0122).
//
// «Los cronogramas de pago se envían por correo con lo que se pretende pagar y los bancos de donde
// salen los fondos.» La pantalla sirve para DECIDIR —de qué cuenta sale cada proveedor, qué se paga
// por fuera, a cuánto se colonizan los dólares—; esto es lo que sale del sistema cuando ya está
// decidido.
//
// LA CASCADA VA EN EL ARCHIVO, no solo en la pantalla. En el cuadro real que esto reemplaza, de
// ₡52,1 M que salen del banco solo ₡10,9 M son el cronograma: un archivo que trajera la lista y un
// total al pie comunicaría una cifra cinco veces mayor que la que se está anunciando. Los cuatro
// escalones son el documento.
//
// Reusa los estilos del aviso de pago (`export_aviso.go`) a propósito: los dos papeles salen del
// mismo corte y viajan al mismo destinatario, así que tienen que verse como el mismo juego de
// documentos. La deuda anotada allá sigue en pie —si aparece un tercero, el motor se saca a su
// propio paquete—.

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// NombreCronogramaXLSX nombra el archivo con el corte, para que dos cronogramas no se pisen en la
// carpeta de descargas de quien los recibe.
func NombreCronogramaXLSX(loteNumero int64) string {
	return fmt.Sprintf("cronograma-pago-corte-%04d.xlsx", loteNumero)
}

// escalon es una línea de la cascada tal como se imprime.
type escalon struct {
	rotulo string
	monto  string
	// destacado: los tres números que se leen (total colonizado, flujo normal y el cronograma).
	destacado bool
}

// escalonesDelCronograma arma la cascada en el orden del papel.
//
// Los dólares aparecen SOLO si los hay: en un corte sin dólares, dos líneas en cero invitan a
// buscar un error que no existe.
func escalonesDelCronograma(t TotalesCronograma) []escalon {
	e := []escalon{{rotulo: "Total colones", monto: t.TotalColones}}
	hayDolares := t.TotalDolares != "0.00"
	if hayDolares {
		e = append(e, escalon{
			rotulo: fmt.Sprintf("Total dólares colonizados (USD %s a %s)", t.TotalDolares, t.TipoCambio),
			monto:  t.DolaresColonizados,
		})
	}
	e = append(e,
		escalon{rotulo: "TOTAL COLONIZADO", monto: t.TotalColonizado, destacado: true},
		escalon{rotulo: "menos  Reserva", monto: t.Reserva},
		escalon{rotulo: "TOTAL FLUJO NORMAL", monto: t.FlujoNormal, destacado: true},
		escalon{rotulo: "menos  Solicitudes por fuera", monto: t.PorFuera},
		escalon{rotulo: "CRONOGRAMA TOTAL FLUJO NORMAL", monto: t.CronogramaTotal, destacado: true},
	)
	if hayDolares {
		e = append(e, escalon{
			rotulo: "Cronograma colones flujo normal", monto: t.CronogramaEnColones,
		})
	}
	return e
}

// estadoDeLinea dice en una palabra si la línea entra en el número que se comunica.
func estadoDeLinea(l LineaCronograma) string {
	switch {
	case l.EsReserva:
		return "Reserva"
	case l.PorFuera:
		return "Por fuera"
	default:
		return "Entra"
	}
}

// ConstruirCronogramaXLSX arma el cuadro completo de un corte.
func ConstruirCronogramaXLSX(cr Cronograma, empresa, generadoPor, generadoEn string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	hoja := "Cronograma"
	if err := f.SetSheetName("Sheet1", hoja); err != nil {
		return nil, fmt.Errorf("cxp: nombrar hoja: %w", err)
	}
	e, err := nuevosEstilosAviso(f)
	if err != nil {
		return nil, err
	}

	anchos := []struct {
		col string
		w   float64
	}{{"A", 36}, {"B", 10}, {"C", 16}, {"D", 18}, {"E", 20}, {"F", 26}, {"G", 14}}
	for _, a := range anchos {
		if err := f.SetColWidth(hoja, a.col, a.col, a.w); err != nil {
			return nil, fmt.Errorf("cxp: ancho de columna: %w", err)
		}
	}

	const filaTitulo, filaContexto, filaCabecera = 1, 2, 3

	celdaTitulo := fmt.Sprintf("A%d", filaTitulo)
	if err := mergeConEstiloAviso(f, hoja, celdaTitulo, fmt.Sprintf("G%d", filaTitulo), e.titulo); err != nil {
		return nil, err
	}
	if err := f.SetCellValue(hoja, celdaTitulo, "Cronograma de Pago"); err != nil {
		return nil, err
	}
	_ = f.SetRowHeight(hoja, filaTitulo, 24)

	contexto := fmt.Sprintf("%s  ·  Corte N° %04d  ·  Fecha de corte %s  ·  %d proveedor(es)",
		empresa, cr.LoteNumero, cr.FechaCorte, len(cr.Lineas))
	if generadoPor != "" {
		contexto += "  ·  Generado por " + generadoPor
	}
	if generadoEn != "" {
		contexto += " el " + generadoEn
	}
	celdaContexto := fmt.Sprintf("A%d", filaContexto)
	if err := mergeConEstiloAviso(f, hoja, celdaContexto, fmt.Sprintf("G%d", filaContexto), e.contexto); err != nil {
		return nil, err
	}
	if err := f.SetCellValue(hoja, celdaContexto, contexto); err != nil {
		return nil, err
	}

	cols := []string{"Proveedor", "Moneda", "Monto", "Monto colonizado", "Banco", "Cuenta", "En el cronograma"}
	for i, c := range cols {
		celda, err := excelize.CoordinatesToCellName(i+1, filaCabecera)
		if err != nil {
			return nil, fmt.Errorf("cxp: celda de cabecera: %w", err)
		}
		if err := f.SetCellValue(hoja, celda, c); err != nil {
			return nil, err
		}
		if err := f.SetCellStyle(hoja, celda, celda, e.cabecera); err != nil {
			return nil, err
		}
	}
	_ = f.SetRowHeight(hoja, filaCabecera, 22)

	fila := filaCabecera + 1
	for i, l := range cr.Lineas {
		txt, mon := e.texto, e.monto
		if i%2 == 1 {
			txt, mon = e.textoAlt, e.montoAlt
		}
		// Lo que NO entra en el número que se comunica va pintado. Quien recibe el cuadro tiene que
		// poder ver de un vistazo por qué la lista suma más que el cronograma: sin la marca, la
		// diferencia entre la suma de la columna y el total de abajo parece un error de cálculo.
		if l.EsReserva || l.PorFuera {
			txt, mon = e.textoAlerta, e.montoAlerta
		}
		valores := []any{
			l.Proveedor, nombreDeMoneda(l.Moneda), nil, nil,
			bancoOAviso(l), cuentaOAvisoCronograma(l), estadoDeLinea(l),
		}
		for j, v := range valores {
			celda, err := excelize.CoordinatesToCellName(j+1, fila)
			if err != nil {
				return nil, fmt.Errorf("cxp: celda de fila: %w", err)
			}
			estilo := txt
			switch j {
			case 2:
				// Los montos van como NÚMERO: en texto no suman ni ordenan, que es lo primero que
				// hace quien recibe el cuadro.
				estilo = mon
				if d, err := decimal.NewFromString(l.Monto); err == nil {
					v, _ = d.Float64()
				}
			case 3:
				estilo = mon
				if d, err := decimal.NewFromString(l.MontoColonizado); err == nil {
					v, _ = d.Float64()
				}
			}
			if v != nil {
				if err := f.SetCellValue(hoja, celda, v); err != nil {
					return nil, err
				}
			}
			if err := f.SetCellStyle(hoja, celda, celda, estilo); err != nil {
				return nil, err
			}
		}
		_ = f.SetRowHeight(hoja, fila, 22)
		fila++
	}

	// LA CASCADA. Va después de la lista y con una fila de aire: es el documento, no un pie.
	fila++
	for _, es := range escalonesDelCronograma(cr.Totales) {
		rot, val := e.texto, e.monto
		if es.destacado {
			rot, val = e.rotuloTotal, e.valorTotal
		}
		if err := mergeConEstiloAviso(f, hoja, fmt.Sprintf("A%d", fila), fmt.Sprintf("C%d", fila), rot); err != nil {
			return nil, err
		}
		if err := f.SetCellValue(hoja, fmt.Sprintf("A%d", fila), es.rotulo); err != nil {
			return nil, err
		}
		if d, err := decimal.NewFromString(es.monto); err == nil {
			v, _ := d.Float64()
			if err := f.SetCellValue(hoja, fmt.Sprintf("D%d", fila), v); err != nil {
				return nil, err
			}
		}
		if err := f.SetCellStyle(hoja, fmt.Sprintf("D%d", fila), fmt.Sprintf("D%d", fila), val); err != nil {
			return nil, err
		}
		_ = f.SetRowHeight(hoja, fila, 20)
		fila++
	}

	fila++
	pie := "Lo que sale de cuentas de reserva y lo marcado «por fuera» se paga igual, pero no entra " +
		"en el cronograma total de flujo normal."
	if cr.SinCuenta > 0 {
		pie += fmt.Sprintf(" ATENCIÓN: %d línea(s) todavía sin cuenta asignada; por ahora cuentan como "+
			"flujo normal, así que el total puede cambiar.", cr.SinCuenta)
	}
	if err := mergeConEstiloAviso(f, hoja, fmt.Sprintf("A%d", fila), fmt.Sprintf("G%d", fila), e.pie); err != nil {
		return nil, err
	}
	if err := f.SetCellValue(hoja, fmt.Sprintf("A%d", fila), pie); err != nil {
		return nil, err
	}

	if err := f.SetSheetView(hoja, 0, &excelize.ViewOptions{ShowGridLines: boolFalso()}); err != nil {
		return nil, fmt.Errorf("cxp: vista de la hoja: %w", err)
	}
	_ = f.SetPanes(hoja, &excelize.Panes{
		Freeze: true, YSplit: filaCabecera, TopLeftCell: fmt.Sprintf("A%d", filaCabecera+1),
		ActivePane: "bottomLeft",
	})
	_ = f.SetPageLayout(hoja, &excelize.PageLayoutOptions{Orientation: orientacionHorizontal()})
	_ = f.SetSheetProps(hoja, &excelize.SheetPropsOptions{FitToPage: boolVerdadero()})

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("cxp: serializar el cronograma: %w", err)
	}
	return buf.Bytes(), nil
}

// bancoOAviso y cuentaOAvisoCronograma dicen POR QUÉ está vacía la celda. Una celda en blanco se
// lee como descuido de quien armó el cuadro; «sin asignar» dice que la decisión está pendiente, que
// es justamente lo que mueve el total de abajo.
func bancoOAviso(l LineaCronograma) string {
	if strings.TrimSpace(l.Banco) == "" {
		return "SIN ASIGNAR"
	}
	return l.Banco
}

func cuentaOAvisoCronograma(l LineaCronograma) string {
	if strings.TrimSpace(l.Cuenta) == "" {
		return "—"
	}
	return l.Cuenta
}
