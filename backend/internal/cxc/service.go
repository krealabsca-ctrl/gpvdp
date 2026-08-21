package cxc

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// Conciliacion es el reporte que se muestra ANTES de confirmar una importación. Es el
// contrato con el usuario: nada entra en silencio y nada se pierde.
type Conciliacion struct {
	Filas        int `json:"filas"`
	Nuevos       int `json:"nuevos"`
	Actualizados int `json:"actualizados"`
	Duplicados   int `json:"duplicados"`
	Cuarentena   int `json:"cuarentena"`
	SinSede      int `json:"sin_sede"`

	Resolucion Resolucion `json:"resolucion"`
	// Muestra: las primeras filas ya interpretadas, para que el usuario compruebe que el
	// mapeo de columnas entendió bien antes de confirmar 70 000.
	Muestra []FilaContrato `json:"muestra"`
	// Problemas: TODAS las filas con motivo, para descargarlas y corregirlas.
	Problemas []FilaContrato `json:"problemas"`
}

// PrevisualizarContratos lee el archivo y devuelve el reporte de conciliación sin
// escribir nada en la cartera.
func (s *Service) PrevisualizarContratos(ctx context.Context, empresaID string, archivo []byte, nombre, usuarioID string) (string, Conciliacion, error) {
	g, err := CargarGrid(archivo)
	if err != nil {
		return "", Conciliacion{}, err
	}
	reglas, err := s.reglasDe(ctx, empresaID)
	if err != nil {
		return "", Conciliacion{}, err
	}
	filas, err := LeerContratos(g, reglas)
	if err != nil {
		return "", Conciliacion{}, err
	}
	c, err := s.conciliar(ctx, empresaID, filas)
	if err != nil {
		return "", Conciliacion{}, err
	}
	id, err := s.repo.CrearImportacion(ctx, empresaID, "CONTRATOS", nombre, usuarioID, c)
	if err != nil {
		return "", Conciliacion{}, err
	}
	return id, c, nil
}

// ConfirmarContratos vuelve a leer el archivo y lo aplica. Se relee a propósito: guardar
// 70 000 filas interpretadas en la sesión para confirmarlas después es la clase de estado
// que se corrompe. El archivo es la fuente y el reporte se recalcula igual.
func (s *Service) ConfirmarContratos(ctx context.Context, empresaID, importacionID string, archivo []byte, usuarioID string) (Conciliacion, Aplicado, error) {
	g, err := CargarGrid(archivo)
	if err != nil {
		return Conciliacion{}, Aplicado{}, err
	}
	reglas, err := s.reglasDe(ctx, empresaID)
	if err != nil {
		return Conciliacion{}, Aplicado{}, err
	}
	filas, err := LeerContratos(g, reglas)
	if err != nil {
		return Conciliacion{}, Aplicado{}, err
	}
	c, err := s.conciliar(ctx, empresaID, filas)
	if err != nil {
		return Conciliacion{}, Aplicado{}, err
	}
	// Las filas duplicadas DENTRO del archivo se colapsan quedándose con la última: el
	// upsert las escribiría dos veces y la segunda ganaría igual, pero así el conteo que
	// se le muestra al usuario es el real.
	aplicar := sinDuplicados(filas)
	ap, err := s.repo.GuardarContratos(ctx, empresaID, aplicar, c.Resolucion)
	if err != nil {
		return Conciliacion{}, Aplicado{}, err
	}
	if importacionID != "" {
		if err := s.repo.ConfirmarImportacion(ctx, empresaID, importacionID); err != nil {
			return Conciliacion{}, Aplicado{}, err
		}
	}
	s.auditar(ctx, empresaID, "IMPORTAR_CARTERA_CXC", usuarioID, map[string]any{
		"filas": c.Filas, "nuevos": ap.Nuevos, "actualizados": ap.Actualizados, "cuarentena": c.Cuarentena,
	})
	return c, ap, nil
}

func (s *Service) conciliar(ctx context.Context, empresaID string, filas []FilaContrato) (Conciliacion, error) {
	res, err := s.repo.ResolverCatalogo(ctx, empresaID, filas)
	if err != nil {
		return Conciliacion{}, err
	}
	c := Conciliacion{Filas: len(filas), Resolucion: res, Muestra: []FilaContrato{}, Problemas: []FilaContrato{}}
	vistos := map[string]bool{}
	for _, f := range filas {
		if vistos[f.Numero] {
			c.Duplicados++
			continue
		}
		vistos[f.Numero] = true
		if f.EnCuarentena() {
			c.Cuarentena++
			if len(c.Problemas) < 500 {
				c.Problemas = append(c.Problemas, f)
			}
		}
		if len(c.Muestra) < 10 {
			c.Muestra = append(c.Muestra, f)
		}
		if f.SedeCruda == "" {
			c.SinSede++
		}
	}
	// Nuevos vs actualizados: se pregunta a la base qué números ya existen. Es el dato
	// que hace la diferencia entre «carga inicial» y «actualización diaria».
	existentes, err := s.repo.NumerosExistentes(ctx, empresaID, claves(vistos))
	if err != nil {
		return Conciliacion{}, err
	}
	for n := range vistos {
		if existentes[n] {
			c.Actualizados++
		} else {
			c.Nuevos++
		}
	}
	return c, nil
}

func sinDuplicados(filas []FilaContrato) []FilaContrato {
	ultima := map[string]FilaContrato{}
	orden := []string{}
	for _, f := range filas {
		if _, ya := ultima[f.Numero]; !ya {
			orden = append(orden, f.Numero)
		}
		ultima[f.Numero] = f
	}
	out := make([]FilaContrato, 0, len(orden))
	for _, n := range orden {
		out = append(out, ultima[n])
	}
	return out
}

func (s *Service) reglasDe(ctx context.Context, empresaID string) (ReglasImportacion, error) {
	p, err := s.repo.Parametros(ctx, empresaID)
	if err != nil {
		return ReglasImportacion{}, err
	}
	r := ReglasImportacion{}
	if v, ok := p["CUOTA_MAXIMA_RAZONABLE"]; ok && v != "" {
		if m, err := decimal.NewFromString(v); err == nil {
			r.CuotaMaxima = m
		}
	}
	return r, nil
}

// ---- Generador de cargos ----

// PlanCargos es la previsualización del generador: cuántos cargos crearía y de qué
// contratos, ANTES de escribir nada.
type PlanCargos struct {
	Desde     string `json:"desde"`
	Hasta     string `json:"hasta"`
	Contratos int    `json:"contratos"`
	// Cargos: el total que se crearía (los que ya existen no cuentan: el insert los
	// ignora, pero el número que se muestra es el del plan completo).
	Cargos int `json:"cargos"`
	// Excluidos: contratos que no pueden generar y por qué. Sin esto, un contrato que
	// nunca cobra pasaría inadvertido para siempre.
	Excluidos map[string]int `json:"excluidos"`
	// SobreElTope avisa que el volumen exige confirmación explícita.
	SobreElTope bool `json:"sobre_el_tope"`
	Tope        int  `json:"tope"`
}

// PrevisualizarCargos calcula el plan sin escribir. `desde` es OBLIGATORIO: generar desde
// el primer cobro de los contratos más viejos son millones de filas, y esa no es una
// decisión que el sistema deba tomar solo.
func (s *Service) PrevisualizarCargos(ctx context.Context, empresaID, rol, usuarioID, desde, hasta string) (PlanCargos, error) {
	// Si no se indica desde cuándo, se usa el parámetro CARGOS_DESDE de la empresa. Si ese
	// también está vacío, el generador sigue negándose (ErrSinDesde): generar desde el
	// primer cobro de los contratos viejos son millones de filas.
	if desde == "" {
		if p, err := s.repo.Parametros(ctx, empresaID); err == nil {
			desde = strings.TrimSpace(p["CARGOS_DESDE"])
		}
	}
	d, h, err := rangoDe(desde, hasta)
	if err != nil {
		return PlanCargos{}, err
	}
	sedes, err := s.sedesVisibles(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return PlanCargos{}, err
	}
	contratos, err := s.repo.ContratosParaGenerar(ctx, empresaID, sedes)
	if err != nil {
		return PlanCargos{}, err
	}
	plan := PlanCargos{
		Desde: d.Format("2006-01-02"), Hasta: h.Format("2006-01-02"),
		Excluidos: map[string]int{}, Tope: TopeCargosPorCorrida,
	}
	for _, c := range contratos {
		cargos, motivo := planDe(c, d, h)
		if motivo != "" {
			plan.Excluidos[motivo]++
			continue
		}
		if len(cargos) == 0 {
			continue
		}
		plan.Contratos++
		plan.Cargos += len(cargos)
	}
	plan.SobreElTope = plan.Cargos > TopeCargosPorCorrida
	return plan, nil
}

// GenerarCargos escribe los cargos del plan. `confirmarTotal` es el número que el usuario
// vio en la previsualización: si no coincide con lo que el plan calcula ahora, se aborta.
// Es el mismo cuidado que en un pago por lote: uno confirma un total, no una intención.
func (s *Service) GenerarCargos(ctx context.Context, empresaID, rol, usuarioID, desde, hasta string, confirmarTotal int) (PlanCargos, int, error) {
	plan, err := s.PrevisualizarCargos(ctx, empresaID, rol, usuarioID, desde, hasta)
	if err != nil {
		return PlanCargos{}, 0, err
	}
	if plan.SobreElTope && confirmarTotal != plan.Cargos {
		return plan, 0, fmt.Errorf("%w: el plan son %d cargos y se confirmaron %d",
			ErrRangoDemasiadoAmplio, plan.Cargos, confirmarTotal)
	}
	d, h, err := rangoDe(desde, hasta)
	if err != nil {
		return PlanCargos{}, 0, err
	}
	sedes, err := s.sedesVisibles(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return PlanCargos{}, 0, err
	}
	contratos, err := s.repo.ContratosParaGenerar(ctx, empresaID, sedes)
	if err != nil {
		return PlanCargos{}, 0, err
	}
	aInsertar := make([]CargoAInsertar, 0, plan.Cargos)
	for _, c := range contratos {
		cargos, motivo := planDe(c, d, h)
		if motivo != "" {
			continue
		}
		for _, g := range cargos {
			aInsertar = append(aInsertar, CargoAInsertar{
				ContratoID: c.ID, Periodo: g.Periodo, VenceEn: g.VenceEn, Monto: g.Monto, Origen: "GENERADO",
			})
		}
	}
	n, err := s.repo.InsertarCargos(ctx, empresaID, aInsertar)
	if err != nil {
		return plan, 0, err
	}
	s.auditar(ctx, empresaID, "GENERAR_CARGOS_CXC", usuarioID, map[string]any{
		"desde": plan.Desde, "hasta": plan.Hasta, "planeados": plan.Cargos, "creados": n,
	})
	return plan, n, nil
}

// planDe traduce un contrato de la base al plan de cargos. Devuelve el motivo de
// exclusión en vez de un error para poder AGRUPARLOS en el reporte: con 70 000 contratos,
// «12 sin fecha de primer cobro» es útil y 12 errores sueltos no.
func planDe(c ContratoGenerable, desde, hasta time.Time) ([]CargoPlan, string) {
	primer, err := time.Parse("2006-01-02", c.PrimerCobro)
	if err != nil {
		return nil, "fecha de primer cobro ilegible"
	}
	cargos, err := PlanDeCargos(
		ContratoParaGenerar{Numero: c.Numero, FechaPrimerCobro: primer, DiaPago: c.DiaPago, Cuota: c.Cuota},
		ModalidadCiclo{MesesCiclo: c.MesesCiclo, Quincenal: c.Quincenal},
		desde, hasta,
	)
	switch {
	case err == nil:
		return cargos, ""
	default:
		return nil, err.Error()
	}
}

func rangoDe(desde, hasta string) (time.Time, time.Time, error) {
	if desde == "" {
		return time.Time{}, time.Time{}, ErrSinDesde
	}
	d, err := time.Parse("2006-01-02", desde)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %q no es una fecha", ErrSinDesde, desde)
	}
	h := hoyCR()
	if hasta != "" {
		if v, err := time.Parse("2006-01-02", hasta); err == nil {
			h = v
		}
	}
	if h.Before(d) {
		return time.Time{}, time.Time{}, ErrRangoInvalido
	}
	return d, h, nil
}

// hoyCR es el día de operación de Costa Rica (UTC−6). El mismo criterio que usan Bancos y
// Nómina: a las 6 p. m. de un 31 en Costa Rica ya es día 1 en UTC.
func hoyCR() time.Time {
	t := time.Now().UTC().Add(-6 * time.Hour)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ---- Consultas ----

// ListarContratos devuelve la cartera filtrada, con el alcance por sede ya aplicado.
func (s *Service) ListarContratos(ctx context.Context, empresaID, rol, usuarioID string, f FiltrosContratos) (ListaContratos, error) {
	sedes, err := s.sedesVisibles(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return ListaContratos{}, err
	}
	f.SedeIDs = sedes
	return s.repo.ListarContratos(ctx, empresaID, f)
}

// Contrato360 es la ficha: el contrato y sus cargos.
type Contrato360 struct {
	Contrato Contrato `json:"contrato"`
	Cargos   []Cargo  `json:"cargos"`
}

func (s *Service) Contrato360(ctx context.Context, empresaID, numero string, soloAbiertos bool) (Contrato360, error) {
	c, err := s.repo.ContratoPorNumero(ctx, empresaID, numero)
	if err != nil {
		return Contrato360{}, err
	}
	cargos, err := s.repo.CargosDeContrato(ctx, empresaID, c.ID, soloAbiertos)
	if err != nil {
		return Contrato360{}, err
	}
	return Contrato360{Contrato: c, Cargos: cargos}, nil
}

func (s *Service) Catalogos(ctx context.Context, empresaID string) (Catalogos, error) {
	return s.repo.Catalogos(ctx, empresaID)
}

func (s *Service) auditar(ctx context.Context, empresaID, accion, usuarioID string, detalle map[string]any) {
	if s.audit == nil {
		return
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "cxc", Accion: accion,
		UsuarioID: &usuarioID, ValorNuevo: detalle,
	})
}

var _ = sort.Strings
