package conta

// La lógica del generador de asientos (Fase 3B).

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// Previsualizar dice qué pasaría al generar el período, sin escribir nada.
//
// ES EL MISMO CRITERIO QUE EL IMPORTADOR DE BANCOS Y EL PLAN DE CUENTAS —primero se ve, después
// se confirma— y acá pesa más que en ninguno: generar toca miles de movimientos de una vez, y lo
// que se escribe mal hay que anularlo asiento por asiento.
func (s *Service) PrevisualizarGeneracion(ctx context.Context, empresaID, periodo string) (ResumenGeneracion, error) {
	if !esPeriodo(periodo) {
		return ResumenGeneracion{}, ErrPeriodoInvalido
	}
	movs, yaTiene, err := s.repo.MovimientosDelPeriodo(ctx, empresaID, periodo)
	if err != nil {
		return ResumenGeneracion{}, err
	}

	// PorMotivo arranca como slice VACÍO y no nil: un nil de Go serializa a `null` en JSON, y la
	// pantalla que hace `d.por_motivo.length` se cae en un período sin movimientos. Ya pasó con
	// las filas del previsualizador de Excel; el contrato es que una lista vacía es `[]`.
	res := ResumenGeneracion{
		Periodo: periodo, Total: len(movs),
		PorMotivo: []MotivoExcluido{},
	}
	debitos, creditos := decimal.Zero, decimal.Zero
	porMotivo := map[string]*MotivoExcluido{}
	montoPorMotivo := map[string]decimal.Decimal{}

	for i, m := range movs {
		if yaTiene[i] {
			res.YaGenerados++
		}
		monto, err := decimal.NewFromString(m.MontoCRC)
		if err != nil {
			monto = decimal.Zero
		}
		if m.SePuede() {
			res.Contabilizables++
			if !yaTiene[i] {
				res.Nuevos++
			}
			if m.EsDebito {
				debitos = debitos.Add(monto)
			} else {
				creditos = creditos.Add(monto)
			}
			continue
		}
		if porMotivo[m.Motivo] == nil {
			etiqueta, donde := etiquetaMotivo(m.Motivo)
			porMotivo[m.Motivo] = &MotivoExcluido{
				Motivo: m.Motivo, Etiqueta: etiqueta, DondeSeArregla: donde,
			}
			montoPorMotivo[m.Motivo] = decimal.Zero
		}
		porMotivo[m.Motivo].Movimientos++
		montoPorMotivo[m.Motivo] = montoPorMotivo[m.Motivo].Add(monto)
	}

	res.MontoDebitos = debitos.StringFixed(2)
	res.MontoCreditos = creditos.StringFixed(2)
	for k, v := range porMotivo {
		v.MontoCRC = montoPorMotivo[k].StringFixed(2)
		res.PorMotivo = append(res.PorMotivo, *v)
	}
	// Lo que más movimientos bloquea, primero: es lo que conviene arreglar antes.
	sort.Slice(res.PorMotivo, func(i, j int) bool {
		return res.PorMotivo[i].Movimientos > res.PorMotivo[j].Movimientos
	})

	res.Aviso = avisoDeGeneracion(res)
	return res, nil
}

// avisoDeGeneracion arma la frase que dice qué hacer antes de generar.
func avisoDeGeneracion(r ResumenGeneracion) string {
	if r.Total == 0 {
		return "No hay movimientos vivos en ese período."
	}
	if r.Contabilizables == 0 {
		// El motivo que más bloquea es el que hay que atacar: decirlo ahorra abrir la tabla.
		if len(r.PorMotivo) > 0 {
			return fmt.Sprintf(
				"Todavía no se puede generar nada: lo que más bloquea es «%s» (%d movimientos). Se arregla en %s.",
				r.PorMotivo[0].Etiqueta, r.PorMotivo[0].Movimientos, r.PorMotivo[0].DondeSeArregla)
		}
		return "Todavía no se puede generar nada de ese período."
	}
	// Si todo lo contabilizable ya tiene asiento, no hay nada que generar y decir que «se van a
	// generar 6.340» sería falso aunque el número sea correcto.
	if r.Nuevos == 0 {
		return fmt.Sprintf(
			"Este período ya está generado: sus %d movimientos contabilizables tienen asiento. Volver a generar no cambia nada.",
			r.Contabilizables)
	}
	pct := r.Contabilizables * 100 / r.Total
	msg := fmt.Sprintf("Se van a generar %d asiento(s) nuevos. En total, %d de %d movimientos se pueden contabilizar (%d %%).",
		r.Nuevos, r.Contabilizables, r.Total, pct)
	if r.YaGenerados > 0 {
		msg += fmt.Sprintf(" Los %d que ya tienen asiento no se duplican.", r.YaGenerados)
	}
	if pct < 100 {
		msg += " Lo que queda fuera no se pierde: se puede generar después, cuando se complete."
	}
	return msg
}

// Generar escribe los asientos del período.
//
// Vuelve a leer los movimientos en vez de recibirlos de la pantalla: entre la previsualización y
// la confirmación alguien pudo clasificar, mapear o excluir, y generar sobre la foto vieja
// escribiría asientos que ya no corresponden.
func (s *Service) Generar(ctx context.Context, empresaID, periodo, usuarioID string) (ResultadoGeneracion, error) {
	if !esPeriodo(periodo) {
		return ResultadoGeneracion{}, ErrPeriodoInvalido
	}
	movs, yaTiene, err := s.repo.MovimientosDelPeriodo(ctx, empresaID, periodo)
	if err != nil {
		return ResultadoGeneracion{}, err
	}

	// LOS QUE YA TIENEN ASIENTO SE DESCARTAN ACÁ, antes de reservar la numeración.
	//
	// El consecutivo se reserva en bloque por rendimiento, y reservar para movimientos que la
	// base va a rechazar deja un HUECO del tamaño del lote: correr la generación dos veces sobre
	// el mismo período saltaba 6.340 números. Un libro diario con huecos es una observación de
	// auditoría, y nadie lo nota hasta que la hacen. Lo encontró el E2E.
	//
	// El `ON CONFLICT DO NOTHING` sigue siendo la red para lo que entre entre esta lectura y la
	// escritura; lo que cambia es que el caso normal ya no desperdicia numeración.
	listos := make([]MovimientoAContabilizar, 0, len(movs))
	omitidos, yaExistian := 0, 0
	for i, m := range movs {
		if yaTiene[i] {
			yaExistian++
			continue
		}
		if m.SePuede() {
			listos = append(listos, m)
			continue
		}
		omitidos++
	}
	if len(listos) == 0 {
		// Si lo único que había ya estaba generado, eso no es un error: es que no hay nada nuevo.
		if yaExistian > 0 {
			return ResultadoGeneracion{
				Periodo: periodo, Omitidos: omitidos, YaExistian: yaExistian,
			}, nil
		}
		return ResultadoGeneracion{}, ErrNadaQueGenerar
	}

	res, err := s.repo.GenerarAsientos(ctx, empresaID, listos, usuarioID)
	if err != nil {
		return ResultadoGeneracion{}, err
	}
	res.Periodo = periodo
	res.Omitidos = omitidos
	res.YaExistian += yaExistian

	s.auditar(ctx, empresaID, periodo, "GENERAR_ASIENTOS", usuarioID, map[string]any{
		"periodo": periodo, "creados": res.Creados,
		"ya_existian": res.YaExistian, "omitidos": omitidos,
	})
	return res, nil
}

// Revertir anula los asientos generados desde el banco en un período.
func (s *Service) Revertir(ctx context.Context, empresaID, periodo, motivo, usuarioID string) (ResultadoReversa, error) {
	if !esPeriodo(periodo) {
		return ResultadoReversa{}, ErrPeriodoInvalido
	}
	motivo = strings.TrimSpace(motivo)
	// Anular un período entero cambia los estados financieros de ese mes: el «ok» reflejo no
	// alcanza, igual que en el resto del sistema.
	if len(motivo) < motivoMinimo {
		return ResultadoReversa{}, ErrMotivoReversaRequerido
	}
	n, err := s.repo.RevertirGeneracion(ctx, empresaID, periodo, motivo, usuarioID)
	if err != nil {
		return ResultadoReversa{}, err
	}
	s.auditar(ctx, empresaID, periodo, "REVERTIR_GENERACION", usuarioID, map[string]any{
		"periodo": periodo, "anulados": n, "motivo": motivo,
	})
	return ResultadoReversa{Periodo: periodo, Anulados: n}, nil
}
