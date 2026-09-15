package cxp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Lógica de negocio de las responsabilidades mensuales (migración 0082).
//
// La regla que ordena todo este archivo: el sistema NUNCA inventa un cumplimiento. Propone,
// muestra y deja constancia; marcar que algo se cumplió es siempre un acto de una persona. Un
// cumplimiento inventado es peor que ninguno, porque apaga la alarma.

// Permisos de la superficie.
const (
	permisoRespVer      = "cxp.responsabilidades.ver"
	permisoRespVerMias  = "cxp.responsabilidades.ver_mias"
	permisoRespDeclarar = "cxp.responsabilidades.declarar"
)

// FechaHoyCR devuelve el día de hoy (YYYY-MM-DD) en el calendario de Costa Rica. Se usa `offsetCR`
// —el mismo que ya usa el tablero— para no depender de la base de husos del contenedor.
func FechaHoyCR() string {
	return time.Now().UTC().Add(offsetCR).Format("2006-01-02")
}

// resolverAlcance traduce los permisos de la persona a lo que puede ver.
//
// Fail-closed: si no tiene ninguno de los dos permisos, el alcance resultante no muestra nada. Y
// si no hay checker de permisos configurado (arranques sin RBAC), ve todo — el mismo criterio que
// ya aplica el resto del módulo, para que una instalación sin RBAC no quede inutilizable.
func (s *Service) resolverAlcance(ctx context.Context, empresaID, rol, usuarioID string) (Alcance, error) {
	if s.perms == nil {
		return Alcance{Todo: true}, nil
	}
	verTodas, err := s.perms.Tiene(ctx, empresaID, rol, permisoRespVer)
	if err != nil {
		return Alcance{}, fmt.Errorf("cxp: permisos de responsabilidades: %w", err)
	}
	if verTodas {
		return Alcance{Todo: true}, nil
	}
	verMias, err := s.perms.Tiene(ctx, empresaID, rol, permisoRespVerMias)
	if err != nil {
		return Alcance{}, fmt.Errorf("cxp: permisos de responsabilidades: %w", err)
	}
	if !verMias {
		// Sin permiso no ve nada. Devolver un alcance vacío (no `Todo`) es lo que hace que el
		// listado salga vacío en vez de completo.
		return Alcance{}, nil
	}
	partidas, err := s.repo.PartidasDelRol(ctx, empresaID, usuarioID)
	if err != nil {
		return Alcance{}, err
	}
	return Alcance{ClasificacionIDs: partidas, UsuarioID: usuarioID}, nil
}

// ListarResponsabilidades devuelve los acuerdos que la persona puede ver.
func (s *Service) ListarResponsabilidades(ctx context.Context, empresaID, rol, usuarioID string, f FiltrosResponsabilidad) ([]Responsabilidad, error) {
	alcance, err := s.resolverAlcance(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return nil, err
	}
	f.Alcance = alcance
	return s.repo.ListarResponsabilidades(ctx, empresaID, f)
}

// ResponsabilidadPorID lee un acuerdo verificando que quede dentro del alcance de la persona.
//
// No alcanza con filtrar el listado: si el detalle no comprobara el alcance, bastaría con adivinar
// un id para leer una responsabilidad que no le toca. El 404 es a propósito — decir «existe pero
// no podés verla» ya es decir que existe.
func (s *Service) ResponsabilidadPorID(ctx context.Context, empresaID, rol, usuarioID, id string) (Responsabilidad, error) {
	alcance, err := s.resolverAlcance(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return Responsabilidad{}, err
	}
	if !alcance.Todo {
		lista, err := s.repo.ListarResponsabilidades(ctx, empresaID, FiltrosResponsabilidad{Alcance: alcance})
		if err != nil {
			return Responsabilidad{}, err
		}
		visible := false
		for _, r := range lista {
			if r.ID == id {
				visible = true
				break
			}
		}
		if !visible {
			return Responsabilidad{}, ErrResponsabilidadNoEncontrada
		}
	}
	return s.repo.ResponsabilidadPorID(ctx, empresaID, id)
}

// normalizarInput aplica los valores de fábrica y las reglas que no dependen de la base.
func normalizarInput(in *ResponsabilidadInput) error {
	in.Nombre = strings.TrimSpace(in.Nombre)
	in.Contraparte = strings.TrimSpace(in.Contraparte)
	if in.Nombre == "" || in.Contraparte == "" {
		return errors.New("cxp: el nombre y la contraparte son obligatorios")
	}
	if in.Tipo == "" {
		in.Tipo = "PAGO"
	}
	if in.Periodicidad == "" {
		in.Periodicidad = "MENSUAL"
	}
	if in.Moneda == "" {
		in.Moneda = "CRC"
	}
	if in.MontoTipo == "" {
		in.MontoTipo = "FIJO"
	}
	if in.RespaldoTipo == "" {
		in.RespaldoTipo = "NINGUNO"
	}
	if in.MontoEsperado == "" {
		in.MontoEsperado = "0"
	}
	if _, err := decimal.NewFromString(in.MontoEsperado); err != nil {
		return fmt.Errorf("cxp: monto esperado inválido: %q", in.MontoEsperado)
	}
	if _, ok := pasoDePeriodicidad[in.Periodicidad]; !ok {
		return fmt.Errorf("%w: %q", ErrPeriodicidadInvalida, in.Periodicidad)
	}
	if in.Periodicidad != "MENSUAL" && (in.MesAncla < 1 || in.MesAncla > 12) {
		return fmt.Errorf("%w: una %s necesita mes de ancla", ErrPeriodicidadInvalida, in.Periodicidad)
	}
	if in.DiaVencimiento < 1 || in.DiaVencimiento > 31 {
		return errors.New("cxp: el día de vencimiento va del 1 al 31")
	}
	// La regla fiscal, antes de tocar la base, para que el mensaje lo escriba el dominio y no
	// Postgres. La base lo vuelve a frenar: es el último candado, no el primero.
	return ValidarRespaldo(in.RespaldoTipo, in.RespaldoArchivo, in.Deducible)
}

// CrearResponsabilidad declara un acuerdo nuevo.
func (s *Service) CrearResponsabilidad(ctx context.Context, empresaID string, in ResponsabilidadInput, usuarioID string) (Responsabilidad, error) {
	if err := normalizarInput(&in); err != nil {
		return Responsabilidad{}, err
	}
	r, err := s.repo.CrearResponsabilidad(ctx, empresaID, in, usuarioID)
	if err != nil {
		return Responsabilidad{}, err
	}
	s.auditarEntidad(ctx, empresaID, "responsabilidad_cxp", r.ID, "CREAR_RESPONSABILIDAD", usuarioID)
	return r, nil
}

// ActualizarResponsabilidad corrige un acuerdo. El monto nuevo rige de acá en adelante: los meses
// ya abiertos conservan el monto con el que se abrieron, porque un histórico que cambia hacia
// atrás deja de servir para comparar.
func (s *Service) ActualizarResponsabilidad(ctx context.Context, empresaID, id string, in ResponsabilidadInput, usuarioID string) (Responsabilidad, error) {
	if err := normalizarInput(&in); err != nil {
		return Responsabilidad{}, err
	}
	r, err := s.repo.ActualizarResponsabilidad(ctx, empresaID, id, in)
	if err != nil {
		return Responsabilidad{}, err
	}
	s.auditarEntidad(ctx, empresaID, "responsabilidad_cxp", id, "EDITAR_RESPONSABILIDAD", usuarioID)
	return r, nil
}

// CambiarEstadoResponsabilidad suspende, reactiva o finaliza un acuerdo.
//
// OJO CON ESTO: suspender saca la responsabilidad del calendario, o sea que permite TAPAR el
// olvido en vez de cumplirlo. Por eso exige motivo escrito y queda en la bitácora.
func (s *Service) CambiarEstadoResponsabilidad(ctx context.Context, empresaID, id, estado, motivo, usuarioID string) error {
	switch estado {
	case RespActiva, RespSuspendida, RespFinalizada:
	default:
		return fmt.Errorf("cxp: estado inválido: %q", estado)
	}
	motivo = strings.TrimSpace(motivo)
	if estado != RespActiva && motivo == "" {
		return errors.New("cxp: suspender o finalizar una responsabilidad exige escribir el motivo")
	}
	if err := s.repo.CambiarEstadoResponsabilidad(ctx, empresaID, id, estado, motivo); err != nil {
		return err
	}
	s.auditarEntidad(ctx, empresaID, "responsabilidad_cxp", id, "ESTADO_RESPONSABILIDAD_"+estado, usuarioID)
	return nil
}

// PlanDelMes es lo que «Abrir el mes» va a hacer, ANTES de hacerlo.
//
// El botón muestra esto y recién después confirma: uno confirma un total, no una intención. Y lo
// que queda afuera se dice agrupado por razón, porque «38 de 41» sin explicar las 3 que faltan es
// justo la clase de silencio que hace que nadie confíe en el número.
type PlanDelMes struct {
	Periodo       string `json:"periodo"`
	VaACrear      int    `json:"va_a_crear"`
	YaEstaban     int    `json:"ya_estaban"`
	MontoEsperado string `json:"monto_esperado"`
	// Afuera explica, por razón, qué responsabilidades activas NO entran en este mes.
	Afuera []MotivoAfuera `json:"afuera"`
}

// MotivoAfuera agrupa lo que queda fuera del mes con su razón y sus nombres.
type MotivoAfuera struct {
	Razon   string   `json:"razon"`
	Cuantas int      `json:"cuantas"`
	Nombres []string `json:"nombres"`
}

// PrevisualizarMes calcula el plan sin escribir nada.
func (s *Service) PrevisualizarMes(ctx context.Context, empresaID, periodo string) (PlanDelMes, error) {
	filas, plan, err := s.planDelMes(ctx, empresaID, periodo)
	_ = filas
	return plan, err
}

// planDelMes es el cálculo compartido por la previsualización y la apertura: las dos tienen que
// mirar exactamente lo mismo, o el total que confirma la persona no sería el que se ejecuta.
func (s *Service) planDelMes(ctx context.Context, empresaID, periodo string) ([]PeriodoNuevo, PlanDelMes, error) {
	if !periodoValido(periodo) {
		return nil, PlanDelMes{}, ErrPeriodoInvalido
	}
	activas, err := s.repo.ResponsabilidadesActivas(ctx, empresaID)
	if err != nil {
		return nil, PlanDelMes{}, err
	}
	yaTienen, err := s.repo.IDsConFilaEnElMes(ctx, empresaID, periodo)
	if err != nil {
		return nil, PlanDelMes{}, err
	}

	plan := PlanDelMes{Periodo: periodo}
	porRazon := map[string][]string{}
	var filas []PeriodoNuevo
	total := decimal.Zero

	for _, r := range activas {
		aplica, err := AplicaEnPeriodo(r.Periodicidad, r.MesAncla, periodo)
		if err != nil {
			porRazon["Periodicidad mal declarada"] = append(porRazon["Periodicidad mal declarada"], r.Nombre)
			continue
		}
		if !aplica {
			razon := fmt.Sprintf("No toca este mes (%s)", strings.ToLower(r.Periodicidad))
			porRazon[razon] = append(porRazon[razon], r.Nombre)
			continue
		}
		if yaTienen[r.ID] {
			plan.YaEstaban++
			continue
		}
		vence, err := FechaDeVencimiento(periodo, r.DiaVencimiento)
		if err != nil {
			porRazon["Sin día de vencimiento usable"] = append(porRazon["Sin día de vencimiento usable"], r.Nombre)
			continue
		}
		monto, err := decimal.NewFromString(r.MontoEsperado)
		if err != nil {
			monto = decimal.Zero
		}
		total = total.Add(monto)
		filas = append(filas, PeriodoNuevo{
			ResponsabilidadID: r.ID,
			Periodo:           periodo,
			VenceEn:           vence.Format("2006-01-02"),
			MontoEsperado:     monto.StringFixed(2),
			Moneda:            r.Moneda,
		})
	}

	plan.VaACrear = len(filas)
	plan.MontoEsperado = total.StringFixed(2)
	plan.Afuera = []MotivoAfuera{}
	for razon, nombres := range porRazon {
		plan.Afuera = append(plan.Afuera, MotivoAfuera{Razon: razon, Cuantas: len(nombres), Nombres: nombres})
	}
	// Orden estable: sin esto, dos previsualizaciones seguidas del mismo mes mostrarían las razones
	// en distinto orden y parecería que algo cambió.
	ordenarAfuera(plan.Afuera)
	return filas, plan, nil
}

func ordenarAfuera(xs []MotivoAfuera) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j].Razon < xs[j-1].Razon; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// AbrirMes crea los períodos del mes.
//
// Es idempotente por construcción (UNIQUE responsabilidad_id+periodo): apretarlo dos veces no
// duplica nada y la segunda vez informa honestamente que creó cero.
//
// `esperadas` es el número que la persona vio en la previsualización. Si al confirmar el plan
// cambió —alguien declaró una responsabilidad nueva en el medio—, se detiene: uno confirma un
// total, no una intención. Mandar -1 salta la comprobación (uso desde scripts).
func (s *Service) AbrirMes(ctx context.Context, empresaID, periodo string, esperadas int, usuarioID string) (PlanDelMes, error) {
	filas, plan, err := s.planDelMes(ctx, empresaID, periodo)
	if err != nil {
		return PlanDelMes{}, err
	}
	if esperadas >= 0 && esperadas != plan.VaACrear {
		return plan, fmt.Errorf("cxp: el plan cambió desde que lo viste (ibas a crear %d y ahora son %d); revisalo y confirmá de nuevo",
			esperadas, plan.VaACrear)
	}
	creadas, err := s.repo.AbrirMes(ctx, empresaID, filas)
	if err != nil {
		return PlanDelMes{}, err
	}
	plan.VaACrear = creadas
	s.auditarEntidad(ctx, empresaID, "responsabilidad_periodo", periodo, "ABRIR_MES_RESPONSABILIDADES", usuarioID)
	return plan, nil
}

// VistaDelMes es la pantalla «El mes»: el encabezado y las filas con su semáforo.
type VistaDelMes struct {
	Resumen ResumenDelMes            `json:"resumen"`
	Filas   []PeriodoResponsabilidad `json:"filas"`
	// SinAbrir lista las responsabilidades activas que corresponden a este mes y NO tienen fila.
	// Es lo que delata que nadie abrió el mes: sin esto, un mes sin abrir se ve idéntico a un mes
	// sin nada pendiente.
	SinAbrir []Responsabilidad `json:"sin_abrir"`
}

// MesDeResponsabilidades arma la pantalla del mes para la persona que consulta.
func (s *Service) MesDeResponsabilidades(ctx context.Context, empresaID, rol, usuarioID, periodo string) (VistaDelMes, error) {
	if periodo == "" {
		periodo = PeriodoActualCR()
	}
	if !periodoValido(periodo) {
		return VistaDelMes{}, ErrPeriodoInvalido
	}
	alcance, err := s.resolverAlcance(ctx, empresaID, rol, usuarioID)
	if err != nil {
		return VistaDelMes{}, err
	}
	filas, err := s.repo.ListarPeriodos(ctx, empresaID, FiltrosPeriodo{Periodo: periodo, Alcance: alcance})
	if err != nil {
		return VistaDelMes{}, err
	}
	// Hasta dónde alcanzan los datos del banco. Es lo que separa «esto está vencido» de «todavía
	// no puedo saberlo», y por eso se lee UNA vez y se aplica a todas las filas.
	bancoHasta, err := s.repo.UltimaFechaBanco(ctx, empresaID)
	if err != nil {
		return VistaDelMes{}, err
	}
	hoy := FechaHoyCR()
	for i := range filas {
		filas[i].Semaforo = Semaforo(filas[i].Estado, filas[i].VenceEn, hoy, bancoHasta)
		filas[i].DiasDeAtraso = DiasDeAtraso(filas[i].VenceEn, hoy)
	}

	sinAbrir, err := s.responsabilidadesSinFila(ctx, empresaID, periodo, alcance)
	if err != nil {
		return VistaDelMes{}, err
	}
	return VistaDelMes{
		Resumen:  ResumirMes(periodo, filas, len(sinAbrir), bancoHasta),
		Filas:    filas,
		SinAbrir: sinAbrir,
	}, nil
}

// responsabilidadesSinFila calcula el estado SIN ABRIR: activas que tocan este mes y no tienen su
// fila. Es el estado que atrapa el olvido del olvido.
func (s *Service) responsabilidadesSinFila(ctx context.Context, empresaID, periodo string, alcance Alcance) ([]Responsabilidad, error) {
	activas, err := s.repo.ListarResponsabilidades(ctx, empresaID, FiltrosResponsabilidad{
		Estado: RespActiva, Alcance: alcance,
	})
	if err != nil {
		return nil, err
	}
	yaTienen, err := s.repo.IDsConFilaEnElMes(ctx, empresaID, periodo)
	if err != nil {
		return nil, err
	}
	out := []Responsabilidad{}
	for _, r := range activas {
		if yaTienen[r.ID] {
			continue
		}
		aplica, err := AplicaEnPeriodo(r.Periodicidad, r.MesAncla, periodo)
		if err != nil || !aplica {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// CerrarPeriodo resuelve un mes: cumplida (con su prueba) o no aplica (con su motivo).
func (s *Service) CerrarPeriodo(ctx context.Context, empresaID, id string, c CierrePeriodo, usuarioID string) error {
	c.Motivo = strings.TrimSpace(c.Motivo)
	switch c.Estado {
	case PerNoAplica:
		if c.Motivo == "" {
			return ErrMotivoObligatorio
		}
		// Un «no aplica» no arrastra pruebas: si trajera un documento, la fila diría a la vez que
		// no correspondía y que se pagó.
		c.CumplidaCon, c.DocumentoID, c.MovimientoID, c.AcuseArchivo = "", "", "", ""
	case PerCumplida:
		switch c.CumplidaCon {
		case PruebaFactura:
			if c.DocumentoID == "" {
				return ErrPruebaObligatoria
			}
			c.MovimientoID, c.AcuseArchivo = "", ""
		case PruebaMovimiento:
			if c.MovimientoID == "" {
				return ErrPruebaObligatoria
			}
			c.DocumentoID, c.AcuseArchivo = "", ""
		case PruebaAcuse:
			if strings.TrimSpace(c.AcuseArchivo) == "" {
				return ErrPruebaObligatoria
			}
			c.DocumentoID, c.MovimientoID = "", ""
		default:
			return ErrPruebaObligatoria
		}
	default:
		return fmt.Errorf("cxp: estado de cierre inválido: %q", c.Estado)
	}

	if err := s.repo.CerrarPeriodo(ctx, empresaID, id, c, usuarioID); err != nil {
		return err
	}
	s.auditarEntidad(ctx, empresaID, "responsabilidad_periodo", id, "CERRAR_PERIODO_"+c.Estado, usuarioID)
	return nil
}

// ReabrirPeriodo corrige un cierre equivocado. No borra la fila: la devuelve a pendiente dejando
// escrito por qué, porque un mes que desaparece se lleva con él la explicación.
func (s *Service) ReabrirPeriodo(ctx context.Context, empresaID, id, motivo, usuarioID string) error {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return errors.New("cxp: reabrir un mes exige escribir por qué")
	}
	if err := s.repo.ReabrirPeriodo(ctx, empresaID, id, motivo); err != nil {
		return err
	}
	s.auditarEntidad(ctx, empresaID, "responsabilidad_periodo", id, "REABRIR_PERIODO", usuarioID)
	return nil
}

// MisResponsabilidades es la pantalla corta: solo las que la persona lleva, de este mes.
//
// Es la que decide si el módulo vive. La lista completa es de Contabilidad; dos o tres filas con
// nombre propio es lo único que alguien de otra área va a abrir.
func (s *Service) MisResponsabilidades(ctx context.Context, empresaID, usuarioID, periodo string) ([]PeriodoResponsabilidad, error) {
	if periodo == "" {
		periodo = PeriodoActualCR()
	}
	if !periodoValido(periodo) {
		return nil, ErrPeriodoInvalido
	}
	// Acá el alcance NO depende de permisos: son las suyas. Cualquiera que entre al módulo puede
	// ver aquello de lo que lo hicieron responsable — no poder verlo sería absurdo.
	filas, err := s.repo.ListarPeriodos(ctx, empresaID, FiltrosPeriodo{
		Periodo: periodo,
		Alcance: Alcance{UsuarioID: usuarioID},
	})
	if err != nil {
		return nil, err
	}
	bancoHasta, err := s.repo.UltimaFechaBanco(ctx, empresaID)
	if err != nil {
		return nil, err
	}
	hoy := FechaHoyCR()
	for i := range filas {
		filas[i].Semaforo = Semaforo(filas[i].Estado, filas[i].VenceEn, hoy, bancoHasta)
		filas[i].DiasDeAtraso = DiasDeAtraso(filas[i].VenceEn, hoy)
	}
	return filas, nil
}
