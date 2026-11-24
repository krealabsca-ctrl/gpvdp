package bancos

// La lógica del presupuesto de INGRESO y del resultado presupuestado.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

var (
	// ErrMetaIngresoInvalida: la meta es un monto y no puede ser negativa. Cero sí: significa «no
	// se espera ingreso por acá este mes», que es una decisión.
	ErrMetaIngresoInvalida = errors.New("bancos: la meta de ingreso tiene que ser un monto de cero o más")
	// ErrPeriodoMetaInvalido: el período es YYYY-MM.
	ErrPeriodoMetaInvalido = errors.New("bancos: el período de la meta tiene que ser YYYY-MM")
	// ErrLineaNoEsDeIngreso: una meta de ingreso sobre una partida de gasto mezclaría las dos
	// mitades del presupuesto y haría que el resultado reste dos veces lo mismo.
	ErrLineaNoEsDeIngreso = errors.New("bancos: esa partida no es de ingreso: la naturaleza de su concepto tiene que ser INGRESO")
)

// auditarMeta registra quién movió una meta y cuándo.
//
// La meta de ingreso es una DECISIÓN de gestión, no un dato: cuando el cumplimiento cambie de un
// mes a otro, la primera pregunta es si cambió lo que entró o lo que se esperaba.
func (s *Service) auditarMeta(ctx context.Context, empresaID, usuarioID, accion, periodo, clasifID, monto string) {
	if s.audit == nil {
		return
	}
	entidadID := periodo
	if clasifID != "" {
		entidadID = periodo + "|" + clasifID
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "presupuesto_ingreso", EntidadID: &entidadID,
		Accion: accion, UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{"periodo": periodo, "clasificacion_id": clasifID, "monto": monto},
	})
}

// MetasDeIngreso lista lo cargado en el rango.
func (s *Service) MetasDeIngreso(ctx context.Context, empresaID, desde, hasta string) ([]MetaIngreso, error) {
	return s.repo.MetasDeIngreso(ctx, empresaID, desde, hasta)
}

// GuardarMetaIngreso valida y escribe la meta de un mes.
func (s *Service) GuardarMetaIngreso(ctx context.Context, empresaID, clasifID, periodo, monto, nota, usuarioID string) error {
	if !esPeriodoValido(periodo) {
		return ErrPeriodoMetaInvalido
	}
	m, err := decimal.NewFromString(strings.TrimSpace(monto))
	if err != nil || m.IsNegative() {
		return ErrMetaIngresoInvalida
	}
	// UNA META DE INGRESO SOLO VA SOBRE UNA PARTIDA DE INGRESO. Sin esto se podría poner la meta
	// de ventas sobre «Planilla», y el resultado presupuestado restaría ese monto dos veces: una
	// como gasto autorizado y otra como ingreso esperado.
	if clasifID != "" {
		nat, err := s.repo.NaturalezaDeClasificacion(ctx, empresaID, clasifID)
		if err != nil {
			return err
		}
		if nat != NaturalezaIngreso {
			return ErrLineaNoEsDeIngreso
		}
	}
	if err := s.repo.GuardarMetaIngreso(ctx, empresaID, clasifID, periodo, m.StringFixed(2), nota, usuarioID); err != nil {
		return err
	}
	s.auditarMeta(ctx, empresaID, usuarioID, "GUARDAR_META_INGRESO", periodo, clasifID, m.StringFixed(2))
	return nil
}

// BorrarMetaIngreso quita la meta de un mes.
func (s *Service) BorrarMetaIngreso(ctx context.Context, empresaID, clasifID, periodo, usuarioID string) error {
	if !esPeriodoValido(periodo) {
		return ErrPeriodoMetaInvalido
	}
	if err := s.repo.BorrarMetaIngreso(ctx, empresaID, clasifID, periodo); err != nil {
		return err
	}
	s.auditarMeta(ctx, empresaID, usuarioID, "BORRAR_META_INGRESO", periodo, clasifID, "")
	return nil
}

// ControlDeIngreso cruza la meta contra lo que de verdad entró.
func (s *Service) ControlDeIngreso(ctx context.Context, empresaID, desde, hasta string) (ControlIngreso, error) {
	metas, err := s.repo.MetasDeIngreso(ctx, empresaID, desde, hasta)
	if err != nil {
		return ControlIngreso{}, err
	}
	reales, err := s.repo.IngresoRealPorLinea(ctx, empresaID, desde, hasta)
	if err != nil {
		return ControlIngreso{}, err
	}

	c := ControlIngreso{Desde: desde, Hasta: hasta}

	// El total del rango es la suma de los TOTALES MENSUALES, no la de las líneas: son dos vistas
	// del mismo dinero y sumarlas contaría doble.
	metaTotal := decimal.Zero
	metaPorLinea := map[string]decimal.Decimal{}
	sumaLineas := decimal.Zero
	for _, m := range metas {
		monto, err := decimal.NewFromString(m.Monto)
		if err != nil {
			continue
		}
		if m.EsTotalDelMes() {
			metaTotal = metaTotal.Add(monto)
			c.MesesConMeta++
			continue
		}
		metaPorLinea[m.ClasificacionID] = metaPorLinea[m.ClasificacionID].Add(monto)
		sumaLineas = sumaLineas.Add(monto)
	}

	realTotal := decimal.Zero
	vistas := map[string]bool{}
	for i := range reales {
		f := &reales[i]
		real, err := decimal.NewFromString(f.Real)
		if err != nil {
			continue
		}
		realTotal = realTotal.Add(real)
		vistas[f.ClasificacionID] = true

		if meta, hay := metaPorLinea[f.ClasificacionID]; hay {
			f.Meta = meta.StringFixed(2)
			f.Diferencia = real.Sub(meta).StringFixed(2)
			f.Estado, f.CumplidoPct = estadoDeIngreso(meta, real)
		} else {
			// Entró plata por una línea sin meta. No es un error —no todo se presupuesta— pero sí
			// hay que contarlo: es lo que una meta por líneas deja afuera sin avisar.
			f.Estado = IngresoSinMeta
			c.LineasSinMeta++
		}
	}

	// Las líneas con meta y SIN movimiento tienen que aparecer igual, en cero: son las que
	// alguien esperaba y no entraron, y omitirlas haría desaparecer justo el incumplimiento.
	for _, m := range metas {
		if m.EsTotalDelMes() || vistas[m.ClasificacionID] {
			continue
		}
		meta := metaPorLinea[m.ClasificacionID]
		estado, pct := estadoDeIngreso(meta, decimal.Zero)
		reales = append(reales, FilaIngreso{
			ClasificacionID: m.ClasificacionID,
			Clasificacion:   m.Clasificacion,
			Concepto:        m.Concepto,
			Meta:            meta.StringFixed(2),
			Real:            "0.00",
			Diferencia:      meta.Neg().StringFixed(2),
			CumplidoPct:     pct,
			Estado:          estado,
		})
		vistas[m.ClasificacionID] = true
	}

	c.Lineas = reales
	c.MetaTotal = metaTotal.StringFixed(2)
	c.RealTotal = realTotal.StringFixed(2)
	c.Diferencia = realTotal.Sub(metaTotal).StringFixed(2)
	c.Estado, c.CumplidoPct = estadoDeIngreso(metaTotal, realTotal)
	c.SumaDeLineas = sumaLineas.StringFixed(2)
	c.SinRepartir = metaTotal.Sub(sumaLineas).StringFixed(2)
	c.Aviso = avisoDeIngreso(c, metaTotal, sumaLineas)
	return c, nil
}

func avisoDeIngreso(c ControlIngreso, metaTotal, sumaLineas decimal.Decimal) string {
	var partes []string
	if c.MesesConMeta == 0 {
		partes = append(partes,
			"No hay meta de ingreso declarada en este rango: lo que se ve es lo que entró, sin nada contra qué compararlo.")
	}
	// El desglose que promete MÁS que el total es el error que hay que ver: significa que las
	// líneas no caben en la meta del mes.
	if sumaLineas.GreaterThan(metaTotal) && metaTotal.IsPositive() {
		partes = append(partes, fmt.Sprintf(
			"El desglose por línea suma ₡%s MÁS que la meta del rango: una de las dos está mal.",
			sumaLineas.Sub(metaTotal).StringFixed(2)))
	}
	if c.LineasSinMeta > 0 {
		partes = append(partes, fmt.Sprintf(
			"%d línea(s) tuvieron ingreso sin meta propia: entran en el total pero no se juzgan.", c.LineasSinMeta))
	}
	return strings.Join(partes, " ")
}

// ResultadoPresupuestado cruza las dos mitades de la meta contra las dos mitades de lo real.
func (s *Service) ResultadoPresupuestado(ctx context.Context, empresaID, desde, hasta string) (ResultadoPresupuestado, error) {
	metas, err := s.repo.MetasDeIngreso(ctx, empresaID, desde, hasta)
	if err != nil {
		return ResultadoPresupuestado{}, err
	}
	ingresoMeta := decimal.Zero
	for _, m := range metas {
		if !m.EsTotalDelMes() {
			continue
		}
		if v, err := decimal.NewFromString(m.Monto); err == nil {
			ingresoMeta = ingresoMeta.Add(v)
		}
	}

	// El gasto presupuestado sale de los TOTALES por departamento, no de los subpresupuestos: son
	// las dos caras del mismo dinero y sumar ambas contaría doble.
	presup, err := s.repo.PresupuestoDelRango(ctx, empresaID, desde, hasta)
	if err != nil {
		return ResultadoPresupuestado{}, err
	}
	gastoMeta := decimal.Zero
	for _, p := range presup {
		if !p.EsTotalDepartamento() {
			continue
		}
		if v, err := decimal.NewFromString(p.Monto); err == nil {
			gastoMeta = gastoMeta.Add(v)
		}
	}

	ingresoReal, gastoReal, err := s.repo.IngresoYGastoReal(ctx, empresaID, desde, hasta)
	if err != nil {
		return ResultadoPresupuestado{}, err
	}

	r := CalcularResultado(ingresoMeta, ingresoReal, gastoMeta, gastoReal)
	r.Desde, r.Hasta = desde, hasta
	return r, nil
}

// PropuestaDeMeta sugiere una meta desde el mismo rango de un año atrás.
//
// SE PROPONE, NO SE GUARDA. Con 41 líneas de ingreso, una planilla vacía se queda vacía: esto
// convierte el trabajo en ajustar en vez de escribir. El número que queda es el que el usuario
// confirma, y por eso la respuesta dice de dónde salió cada cifra.
func (s *Service) PropuestaDeMeta(ctx context.Context, empresaID, desde, hasta string, ajustePct string) ([]FilaIngreso, error) {
	desdeAnt, hastaAnt := unAnioAtras(desde), unAnioAtras(hasta)
	base, err := s.repo.IngresoHistorico(ctx, empresaID, desdeAnt, hastaAnt)
	if err != nil {
		return nil, err
	}
	factor := decimal.NewFromInt(1)
	if p, err := decimal.NewFromString(strings.TrimSpace(ajustePct)); err == nil {
		factor = factor.Add(p.Div(decimal.NewFromInt(100)))
	}
	for i := range base {
		v, err := decimal.NewFromString(base[i].Real)
		if err != nil {
			continue
		}
		// El histórico queda en `Real` y la propuesta en `Meta`: así la pantalla puede mostrar las
		// dos y quien ajusta ve contra qué.
		base[i].Meta = v.Mul(factor).Round(0).StringFixed(2)
	}
	return base, nil
}

// unAnioAtras corre un período YYYY-MM doce meses hacia atrás.
func unAnioAtras(periodo string) string {
	if len(periodo) < 4 {
		return periodo
	}
	anio := periodo[:4]
	var n int
	if _, err := fmt.Sscanf(anio, "%d", &n); err != nil {
		return periodo
	}
	return fmt.Sprintf("%04d%s", n-1, periodo[4:])
}

// ── El outlook del año ──────────────────────────────────────────────────────────────────────

// ErrAnioInvalido: el outlook es de un año calendario.
var ErrAnioInvalido = errors.New("bancos: el año tiene que estar entre 2000 y 2100")

// Outlook arma el cierre proyectado del año.
//
// Es la TERCERA COLUMNA que faltaba: con Presupuesto y Real solo se sabe dónde se está; con esta,
// dónde se va a terminar.
func (s *Service) Outlook(ctx context.Context, empresaID string, anio int) (OutlookAnual, error) {
	if anio < 2000 || anio > 2100 {
		return OutlookAnual{}, ErrAnioInvalido
	}
	reales, err := s.repo.RealPorMesDelAnio(ctx, empresaID, anio)
	if err != nil {
		return OutlookAnual{}, err
	}
	metas, err := s.repo.MetasPorMesDelAnio(ctx, empresaID, anio)
	if err != nil {
		return OutlookAnual{}, err
	}
	// time.Now() directo y no un reloj inyectado: TODO el criterio del outlook vive en
	// ConstruirOutlook, que es pura y se prueba con la fecha fijada. Acá no queda nada que un
	// reloj falso pudiera ejercitar.
	return ConstruirOutlook(time.Now().UTC(), anio, reales, metas), nil
}
