package bancos

// Previsualizar y confirmar la carga del reporte de pagos aplicados del sistema anterior.
//
// Dos pasos y no uno, como el resto de los importadores del proyecto: se sube, se ve qué va a pasar
// y RECIÉN AHÍ se confirma. Marcar 3.000 movimientos como resueltos sin que nadie mire antes es el
// tipo de acción que no se puede deshacer leyendo la pantalla.

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// SelloPagoExterno es lo que se escribe para UN movimiento.
type SelloPagoExterno struct {
	MovimientoID   string
	Referencia     string
	Documento      string
	Tercero        string
	FechaAplicado  string // YYYY-MM-DD o "" (va como NULL)
	MontoReportado string // decimal o "" (va como NULL)
	Nota           string
}

// SelloVisible es el sello que la lista de movimientos muestra.
type SelloVisible struct {
	Referencia     string `json:"referencia"`
	Documento      string `json:"documento"`
	Tercero        string `json:"tercero"`
	FechaAplicado  string `json:"fecha_aplicado"`
	MontoReportado string `json:"monto_reportado"`
}

// ResumenPagosExternos es la foto de cuánto hay sellado y cuánto queda sin explicar.
type ResumenPagosExternos struct {
	Sellados     int    `json:"sellados"`
	MontoSellado string `json:"monto_sellado"`
	Cargas       int    `json:"cargas"`
	// Pendientes: los débitos que no tienen ni enlace al ERP ni sello. Es el número que importa.
	Pendientes     int    `json:"pendientes"`
	MontoPendiente string `json:"monto_pendiente"`
}

// PrevisualizacionPagosExternos es lo que se muestra ANTES de confirmar.
type PrevisualizacionPagosExternos struct {
	Filas int `json:"filas"`
	// Los cuatro veredictos, contados.
	Unicos        int `json:"unicos"`
	Varios        int `json:"varios"`
	SinMovimiento int `json:"sin_movimiento"`
	Ilegibles     int `json:"ilegibles"`
	// ConDiferencia: calzaron por referencia pero el monto del reporte no es el del banco. Se
	// sellan igual —es un hallazgo, no un rechazo— y se cuentan aparte para poder mirarlos.
	ConDiferencia int `json:"con_diferencia"`
	// MontoAMarcar: cuánto se va a sellar si se confirma.
	MontoAMarcar string `json:"monto_a_marcar"`
	// Detalle acotado: lo que NO calzó va primero, que es lo único accionable.
	Detalle []ResultadoCruce `json:"detalle"`
	// Antes: la foto del antes, para poder comparar con el después.
	Antes ResumenPagosExternos `json:"antes"`
}

// maxDetallePrevisualizacion acota lo que viaja a la pantalla. El resumen cuenta todo; el detalle
// es para mirar, y nadie mira veinte mil filas.
const maxDetallePrevisualizacion = 500

// PrevisualizarPagosExternos lee el reporte, lo cruza y dice qué pasaría. NO escribe nada.
func (s *Service) PrevisualizarPagosExternos(ctx context.Context, empresaID string, archivo []byte) (PrevisualizacionPagosExternos, []ResultadoCruce, error) {
	g, err := GridDelReporte(archivo)
	if err != nil {
		return PrevisualizacionPagosExternos{}, nil, err
	}
	filas, err := LeerReportePagos(g)
	if err != nil {
		return PrevisualizacionPagosExternos{}, nil, err
	}
	candidatos, err := s.repo.DebitosParaCruzar(ctx, empresaID)
	if err != nil {
		return PrevisualizacionPagosExternos{}, nil, err
	}
	antes, err := s.repo.ResumenSellos(ctx, empresaID)
	if err != nil {
		return PrevisualizacionPagosExternos{}, nil, err
	}

	cruce := CruzarReporte(filas, candidatos)
	rep := resumirCruce(cruce)
	rep.Antes = antes
	return rep, cruce, nil
}

// resumirCruce cuenta los veredictos y arma el detalle que se muestra.
func resumirCruce(cruce []ResultadoCruce) PrevisualizacionPagosExternos {
	rep := PrevisualizacionPagosExternos{Filas: len(cruce)}
	monto := decimal.Zero
	for _, r := range cruce {
		switch r.Veredicto {
		case CruceUnico:
			rep.Unicos++
			monto = monto.Add(r.Fila.Monto)
			if r.DiferenciaMonto != "" {
				rep.ConDiferencia++
			}
		case CruceVarios:
			rep.Varios++
		case CruceSinMovimiento:
			rep.SinMovimiento++
		default:
			rep.Ilegibles++
		}
	}
	rep.MontoAMarcar = monto.StringFixed(2)

	// Lo que NO calzó va primero: es lo único sobre lo que alguien puede hacer algo. Si el detalle
	// empezara por lo que sí calzó, en una corrida de miles las excepciones nunca se verían.
	rep.Detalle = make([]ResultadoCruce, 0, maxDetallePrevisualizacion)
	for _, orden := range []string{CruceVarios, CruceIlegible, CruceSinMovimiento, CruceUnico} {
		for _, r := range cruce {
			if len(rep.Detalle) >= maxDetallePrevisualizacion {
				return rep
			}
			if r.Veredicto == orden {
				rep.Detalle = append(rep.Detalle, r)
			}
		}
	}
	return rep
}

// ConfirmarPagosExternos vuelve a leer y cruzar el MISMO archivo y escribe los sellos.
//
// Se recruza en vez de confiar en lo que la pantalla devuelve: la fuente de verdad es el archivo, y
// entre la previsualización y la confirmación el banco pudo cambiar (alguien enlazó un movimiento a
// una factura, o revirtió una carga). Es el mismo criterio que usa el importador de cartera.
func (s *Service) ConfirmarPagosExternos(ctx context.Context, empresaID, usuarioID string, archivo []byte) (PrevisualizacionPagosExternos, string, int, error) {
	rep, cruce, err := s.PrevisualizarPagosExternos(ctx, empresaID, archivo)
	if err != nil {
		return rep, "", 0, err
	}

	sellos := make([]SelloPagoExterno, 0, rep.Unicos)
	for _, r := range cruce {
		if r.Veredicto != CruceUnico {
			continue
		}
		sellos = append(sellos, SelloPagoExterno{
			MovimientoID:   r.MovimientoID,
			Referencia:     r.Fila.Referencia,
			Documento:      r.Fila.Documento,
			Tercero:        r.Fila.Tercero,
			FechaAplicado:  r.Fila.Fecha,
			MontoReportado: r.Fila.MontoTexto,
			Nota:           r.Fila.Nota,
		})
	}
	if len(sellos) == 0 {
		// No es un error: el archivo se entendió y no hubo nada que marcar. Decirlo como error
		// haría pensar que el archivo está mal.
		return rep, "", 0, nil
	}

	carga, n, err := s.repo.SellarPagosExternos(ctx, empresaID, usuarioID, sellos)
	if err != nil {
		return rep, "", 0, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "pago_aplicado_externo", EntidadID: &carga,
		Accion: "SELLAR_PAGOS_EXTERNOS", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"carga_id": carga, "sellados": n, "filas_del_reporte": rep.Filas,
			"sin_movimiento": rep.SinMovimiento, "varios": rep.Varios,
		},
	})
	return rep, carga, n, nil
}

// RevertirCargaDeSellos quita las marcas de una carga entera.
func (s *Service) RevertirCargaDeSellos(ctx context.Context, empresaID, usuarioID, cargaID string) (int, error) {
	n, err := s.repo.RevertirCargaDeSellos(ctx, empresaID, cargaID)
	if err != nil {
		return 0, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "pago_aplicado_externo", EntidadID: &cargaID,
		Accion: "REVERTIR_PAGOS_EXTERNOS", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{"carga_id": cargaID, "quitados": n},
	})
	return n, nil
}

// ResumenPagosExternos es la foto para el encabezado de la pantalla.
func (s *Service) ResumenPagosExternos(ctx context.Context, empresaID string) (ResumenPagosExternos, error) {
	return s.repo.ResumenSellos(ctx, empresaID)
}
