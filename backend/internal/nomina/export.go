package nomina

// Exportaciones .xlsx de la corrida (pantalla "Reportes y Pagos SINPE" de la maqueta):
//   - Archivo de pago: cuadra 1:1 con el neto de la corrida (cada línea nace de una
//     colilla; no hay exportador ad-hoc que evada las deducciones de ley) MÁS los
//     finiquitos del mes. Consecutivo por empresa con bitácora y huella por línea
//     (NOM- salario, FIN- cese) para conciliar en Bancos.
//   - Planilla CCSS: bases y cargas obrero/patronal del mes, una fila por persona,
//     sumando las vacaciones pagadas en un finiquito del mes (son salario).

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

var (
	// ErrCorridaNoCongelada exige APROBADA o PAGADA para exportar (el borrador cambia).
	ErrCorridaNoCongelada = errors.New("nomina: el archivo se genera de una corrida aprobada o pagada")
	// ErrArchivoSinRegistros indica que ninguna colilla tiene IBAN para el pago.
	ErrArchivoSinRegistros = errors.New("nomina: ningún empleado de la corrida tiene IBAN registrado")
	// ErrPlanillaSoloLiquidacion: la planilla CCSS sale de la liquidación (mes completo).
	ErrPlanillaSoloLiquidacion = errors.New("nomina: la planilla CCSS se genera de la corrida de liquidación")
)

// tipoIDSINPE mapea el tipo de identificación al código del layout de carga masiva.
func tipoIDSINPE(tipo string) string {
	switch tipo {
	case "DIMEX":
		return "03"
	case "PASAPORTE":
		return "04"
	default: // CEDULA
		return "01"
	}
}

// ArchivoPagoXLSX genera el archivo de pago de la corrida (APROBADA/PAGADA): asigna el
// consecutivo por empresa (bitácora auditable) y arma el .xlsx con la huella por línea.
// En la LIQUIDACIÓN incluye además los finiquitos congelados con salida en el mes.
func (s *Service) ArchivoPagoXLSX(ctx context.Context, empresaID, corridaID, usuarioID string) ([]byte, string, error) {
	c, err := s.repo.CorridaPorID(ctx, empresaID, corridaID)
	if err != nil {
		return nil, "", err
	}
	if c.Estado != EstAprobada && c.Estado != EstPagada {
		return nil, "", ErrCorridaNoCongelada
	}
	lineas, _, err := s.repo.LineasParaArchivo(ctx, empresaID, corridaID)
	if err != nil {
		return nil, "", err
	}
	// El cese también se paga por SINPE: los finiquitos congelados con salida en el mes se
	// suman al archivo de la LIQUIDACIÓN (una sola vez al mes — el adelanto del día 15 no
	// los lleva, y un mes tiene una sola liquidación viva, así que no se puede duplicar).
	var finiquitos []FiniquitoDelMes
	if c.Tipo == CorridaLiquidacion {
		delMes, err := s.repo.FiniquitosDelMes(ctx, empresaID, c.Anio, c.Mes)
		if err != nil {
			return nil, "", err
		}
		for _, f := range delMes {
			monto, err := decimal.NewFromString(f.Total)
			if err != nil {
				return nil, "", fmt.Errorf("nomina: total de finiquito corrupto: %w", err)
			}
			if f.IBAN == "" || !monto.IsPositive() {
				continue // sin cuenta o sin saldo a favor: se paga fuera del archivo
			}
			finiquitos = append(finiquitos, f)
		}
	}
	if len(lineas)+len(finiquitos) == 0 {
		return nil, "", ErrArchivoSinRegistros
	}
	total := decimal.Zero
	for _, l := range lineas {
		monto, err := decimal.NewFromString(l.Neto)
		if err != nil {
			return nil, "", fmt.Errorf("nomina: neto corrupto en archivo: %w", err)
		}
		total = total.Add(monto)
	}
	for _, f := range finiquitos {
		monto, _ := decimal.NewFromString(f.Total) // ya validado arriba
		total = total.Add(monto)
	}
	registros := len(lineas) + len(finiquitos)
	consecutivo, err := s.repo.RegistrarArchivoPago(ctx, empresaID, corridaID, registros, total, usuarioID)
	if err != nil {
		return nil, "", err
	}

	headers := []any{"Tipo id", "Identificación", "Nombre", "IBAN destino", "Monto neto", "Detalle (huella)", "Consecutivo"}
	rows := make([][]any, 0, registros+1)
	for _, l := range lineas {
		huella := fmt.Sprintf("NOM-%d-%s", consecutivo, l.Identificacion)
		rows = append(rows, []any{tipoIDSINPE(l.TipoIdentificacion), l.Identificacion, l.Nombre,
			l.IBAN, montoCelda(l.Neto), huella, consecutivo})
	}
	for _, f := range finiquitos {
		// Huella FIN-: en Bancos se distingue del salario del mes al conciliar.
		huella := fmt.Sprintf("FIN-%d-%s", consecutivo, f.Identificacion)
		rows = append(rows, []any{tipoIDSINPE(f.TipoIdentificacion), f.Identificacion,
			f.Nombre + " (finiquito)", f.IBAN, montoCelda(f.Total), huella, consecutivo})
	}
	rows = append(rows, []any{"", "", "TOTAL DEL ARCHIVO", "", montoCelda(total.StringFixed(2)), "", consecutivo})

	buf, err := construirXLSXNomina("Pago SINPE", headers, rows)
	if err != nil {
		return nil, "", err
	}
	s.auditar(ctx, empresaID, "corrida_nomina", corridaID, "ARCHIVO_PAGO", usuarioID,
		map[string]any{"consecutivo": consecutivo, "registros": registros,
			"finiquitos": len(finiquitos), "total": total.StringFixed(2)})
	nombre := fmt.Sprintf("nomina-sinpe-%d-%02d-%s-%d.xlsx", c.Anio, c.Mes, c.Tipo, consecutivo)
	return buf, nombre, nil
}

// PlanillaCCSSXLSX genera el resumen de bases y cargas de la liquidación (APROBADA/PAGADA)
// para preparar la planilla SICERE: la base reportada es la base CCSS ÍNTEGRA de cada
// colilla (guardarraíl: lo salarial jamás se reporta reducido).
func (s *Service) PlanillaCCSSXLSX(ctx context.Context, empresaID, corridaID, usuarioID string) ([]byte, string, error) {
	c, err := s.repo.CorridaPorID(ctx, empresaID, corridaID)
	if err != nil {
		return nil, "", err
	}
	if c.Tipo != CorridaLiquidacion {
		return nil, "", ErrPlanillaSoloLiquidacion
	}
	if c.Estado != EstAprobada && c.Estado != EstPagada {
		return nil, "", ErrCorridaNoCongelada
	}
	// La planilla es del MES, no de una sola corrida: en jornada quincenal el salario se
	// paga en dos partes y la CCSS debe recibir la base mensual íntegra. Se suman todas
	// las colillas congeladas del mes por empleado.
	lineas, err := s.repo.LineasPlanillaDelMes(ctx, empresaID, c.Anio, c.Mes)
	if err != nil {
		return nil, "", err
	}
	finiquitos, err := s.repo.FiniquitosDelMes(ctx, empresaID, c.Anio, c.Mes)
	if err != nil {
		return nil, "", err
	}
	filas, err := fusionarPlanilla(lineas, finiquitos)
	if err != nil {
		return nil, "", err
	}
	headers := []any{"Nombre", "Identificación", "Salario reportado (base CCSS del mes)",
		"CCSS obrero", "CCSS patronal + otras cargas", "Total cargas", "Incluye"}
	rows := make([][]any, 0, len(filas)+1)
	totBase, totObrero, totPatronal := decimal.Zero, decimal.Zero, decimal.Zero
	for _, f := range filas {
		totBase, totObrero, totPatronal = totBase.Add(f.Base), totObrero.Add(f.Obrero), totPatronal.Add(f.Patronal)
		rows = append(rows, []any{f.Nombre, f.Identificacion, montoCelda(f.Base.StringFixed(2)),
			montoCelda(f.Obrero.StringFixed(2)), montoCelda(f.Patronal.StringFixed(2)),
			montoCelda(f.Obrero.Add(f.Patronal).StringFixed(2)), f.Origen})
	}
	rows = append(rows, []any{"TOTAL", "", montoCelda(totBase.StringFixed(2)),
		montoCelda(totObrero.StringFixed(2)), montoCelda(totPatronal.StringFixed(2)),
		montoCelda(totObrero.Add(totPatronal).StringFixed(2)), ""})

	buf, err := construirXLSXNomina("Planilla CCSS", headers, rows)
	if err != nil {
		return nil, "", err
	}
	conFiniquito := 0
	for _, f := range filas {
		if f.ConFiniquito {
			conFiniquito++
		}
	}
	s.auditar(ctx, empresaID, "corrida_nomina", corridaID, "PLANILLA_CCSS", usuarioID,
		map[string]any{"registros": len(filas), "con_finiquito": conFiniquito})
	nombre := fmt.Sprintf("planilla-ccss-%d-%02d.xlsx", c.Anio, c.Mes)
	return buf, nombre, nil
}

// filaPlanilla es una persona en la planilla del mes: UNA fila por trabajador, como la
// reporta el SICERE, aunque el salario se haya pagado en dos quincenas y aunque además se
// le haya liquidado el cese.
type filaPlanilla struct {
	Nombre         string
	Identificacion string
	Base           decimal.Decimal
	Obrero         decimal.Decimal
	Patronal       decimal.Decimal
	Origen         string
	ConFiniquito   bool
}

// fusionarPlanilla suma a las colillas del mes la porción AFECTA de los finiquitos con
// salida en ese mes (las vacaciones pagadas al cese son salario: cotizan obrero y patronal).
// Un finiquito sin base afecta —cesantía y aguinaldo son exentos— no agrega fila.
func fusionarPlanilla(lineas []LineaCorrida, finiquitos []FiniquitoDelMes) ([]filaPlanilla, error) {
	filas := make([]filaPlanilla, 0, len(lineas)+len(finiquitos))
	pos := make(map[string]int, len(lineas)+len(finiquitos))
	for _, l := range lineas {
		base, err := decimal.NewFromString(l.BaseCCSS)
		if err != nil {
			return nil, fmt.Errorf("nomina: base CCSS corrupta en planilla: %w", err)
		}
		obrero, err := decimal.NewFromString(l.CCSSObrero)
		if err != nil {
			return nil, fmt.Errorf("nomina: CCSS obrero corrupta en planilla: %w", err)
		}
		patronal, err := decimal.NewFromString(l.Patronal)
		if err != nil {
			return nil, fmt.Errorf("nomina: carga patronal corrupta en planilla: %w", err)
		}
		pos[l.Identificacion] = len(filas)
		filas = append(filas, filaPlanilla{Nombre: l.Nombre, Identificacion: l.Identificacion,
			Base: base, Obrero: obrero, Patronal: patronal, Origen: "Salario del mes"})
	}
	for _, f := range finiquitos {
		base, err := decimal.NewFromString(f.BaseCCSS)
		if err != nil {
			return nil, fmt.Errorf("nomina: base CCSS corrupta en finiquito: %w", err)
		}
		if !base.IsPositive() {
			continue // finiquito sin vacaciones pendientes: nada afecto que reportar
		}
		obrero, err := decimal.NewFromString(f.CCSSObrero)
		if err != nil {
			return nil, fmt.Errorf("nomina: CCSS obrero corrupta en finiquito: %w", err)
		}
		patronal, err := decimal.NewFromString(f.Patronal)
		if err != nil {
			return nil, fmt.Errorf("nomina: carga patronal corrupta en finiquito: %w", err)
		}
		if i, ok := pos[f.Identificacion]; ok {
			filas[i].Base = filas[i].Base.Add(base)
			filas[i].Obrero = filas[i].Obrero.Add(obrero)
			filas[i].Patronal = filas[i].Patronal.Add(patronal)
			filas[i].Origen = "Salario del mes + vacaciones del finiquito"
			filas[i].ConFiniquito = true
			continue
		}
		pos[f.Identificacion] = len(filas)
		filas = append(filas, filaPlanilla{Nombre: f.Nombre, Identificacion: f.Identificacion,
			Base: base, Obrero: obrero, Patronal: patronal,
			Origen: "Vacaciones del finiquito (cese)", ConFiniquito: true})
	}
	return filas, nil
}

// montoCelda convierte el decimal-string a float64 SOLO para la celda de Excel (display:
// que el equipo pueda sumar/ordenar). El dato autoritativo sigue siendo el decimal en DB.
func montoCelda(s string) float64 {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return 0
	}
	f, _ := d.Float64()
	return f
}

// construirXLSXNomina genera el .xlsx (encabezado en negrita, fila congelada, autofiltro).
func construirXLSXNomina(hoja string, headers []any, rows [][]any) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", hoja); err != nil {
		return nil, fmt.Errorf("nomina: nombrar hoja: %w", err)
	}
	if err := f.SetSheetRow(hoja, "A1", &headers); err != nil {
		return nil, fmt.Errorf("nomina: encabezado xlsx: %w", err)
	}
	for i, r := range rows {
		row := r
		if err := f.SetSheetRow(hoja, fmt.Sprintf("A%d", i+2), &row); err != nil {
			return nil, fmt.Errorf("nomina: fila xlsx: %w", err)
		}
	}
	if style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err == nil {
		ultimaCol, _ := excelize.ColumnNumberToName(len(headers))
		_ = f.SetCellStyle(hoja, "A1", ultimaCol+"1", style)
		_ = f.SetPanes(hoja, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
		_ = f.AutoFilter(hoja, fmt.Sprintf("A1:%s1", ultimaCol), []excelize.AutoFilterOptions{})
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("nomina: serializar xlsx: %w", err)
	}
	return buf.Bytes(), nil
}
