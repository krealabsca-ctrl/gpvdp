package bancos

// Servicio de las dimensiones del gasto (departamento y sede) y del presupuesto por departamento.
//
// La pregunta que contesta esta pantalla es «¿este departamento se está gastando de más?», y para
// que la respuesta sea confiable hacen falta dos cosas que acá se hacen explícitas:
//
//  1. **El gasto sin dueño se muestra, no se reparte.** Lo que no tiene departamento asignado va en
//     su propia fila «(sin asignar)». Repartirlo o esconderlo haría que los departamentos con dueño
//     parecieran más chicos de lo que son.
//  2. **Un mes a medio clasificar consume menos presupuesto del que consumió.** Es el mismo
//     guardarraíl del análisis de partidas: si el mes está al 40 % clasificado, el «consumido» está
//     subestimado y presentarlo como verdad invita a gastar sobre un presupuesto ya agotado. Se dice
//     en la misma respuesta, no en una nota al pie.

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

var (
	// ErrSedeNoEncontrada indica que la sede no existe o no es de la empresa.
	ErrSedeNoEncontrada = errors.New("bancos: sede no encontrada")
	// ErrPresupuestoNoEncontrado indica que no había presupuesto para ese departamento y mes.
	ErrPresupuestoNoEncontrado = errors.New("bancos: no hay presupuesto para ese departamento en ese mes")
	// ErrDimensionInvalida indica que se pidió agrupar por algo que no es una dimensión.
	ErrDimensionInvalida = errors.New("bancos: solo se puede agrupar por departamento o por sede")
	// ErrPresupuestoNegativo indica un monto imposible.
	ErrPresupuestoNegativo = errors.New("bancos: el presupuesto no puede ser negativo")
	// ErrNombreRequerido indica que falta el nombre de una entrada de catálogo.
	ErrNombreRequerido = errors.New("bancos: el nombre es obligatorio")
	// ErrDepartamentoNoEncontrado indica que el departamento no existe o no es de la empresa.
	ErrDepartamentoNoEncontrado = errors.New("bancos: departamento no encontrado")
)

// Cómo se puede agrupar el gasto.
const (
	AgruparPorDepartamento = "departamento"
	AgruparPorSede         = "sede"
)

// Sede es un lugar físico del negocio (sucursal, camposanto, plaza).
type Sede struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Codigo string `json:"codigo"`
	Activo bool   `json:"activo"`
}

// Departamento es un área de la empresa. La tabla es UNA (migración 0026, llegó con CxP) y se
// administra desde los dos módulos: no hay dos catálogos que puedan contradecirse, solo dos puertas.
type Departamento struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Codigo string `json:"codigo"`
	Activo bool   `json:"activo"`
}

// GastoDimension es el gasto de un departamento (o sede) en el rango.
type GastoDimension struct {
	// ID vacío = la fila «(sin asignar)».
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Movs   int    `json:"movs"`
	Gasto  string `json:"gasto"`
}

// GastoPartidaDimension es en qué partida gastó un departamento, y de dónde salió esa atribución.
type GastoPartidaDimension struct {
	// ClasificacionID vacío = el gasto sin clasificar. Viaja para poder asignarle dimensiones o
	// subpresupuesto sin tener que adivinar la partida por su nombre.
	ClasificacionID string `json:"clasificacion_id"`
	Concepto        string `json:"concepto"`
	Clasificacion   string `json:"clasificacion"`
	Movs            int    `json:"movs"`
	Gasto           string `json:"gasto"`
	// Origen: MOVIMIENTO | FACTURA | PARTIDA | SIN_ASIGNAR. Sin esto no se sabe dónde corregir.
	Origen        string `json:"origen"`
	OrigenLegible string `json:"origen_legible"`
	// Subpresupuesto autorizado para esta partida en el rango. "0.00" = no se definió ninguno.
	Subpresupuesto string `json:"subpresupuesto"`
	// Disponible y ConsumidoPct quedan vacíos si no hay subpresupuesto: sin monto autorizado, un
	// porcentaje de consumo no significa nada.
	Disponible   string `json:"disponible"`
	ConsumidoPct string `json:"consumido_pct"`
	// Estado: SIN_PRESUPUESTO | EN_RANGO | ALERTA | EXCEDIDO (los mismos del departamento).
	Estado string `json:"estado"`

	// ── El default ACTUAL de la partida, para poder corregirlo ─────────────
	//
	// No es lo mismo que el departamento de la fila: la fila muestra el departamento EFECTIVO
	// (que puede venir del movimiento o de la factura), y esto es lo que tiene escrito la
	// clasificación. Viaja porque la pantalla necesita los DOS para reasignar: el UPDATE escribe
	// `departamento_id` y `sede_id` juntos, así que mandar el departamento con la sede vacía le
	// BORRARÍA la sede a la partida sin que nadie lo pidiera.
	PartidaDepartamentoID string `json:"partida_departamento_id"`
	PartidaSedeID         string `json:"partida_sede_id"`
}

// PresupuestoLinea es un monto autorizado en un mes: del departamento completo o de una de sus
// partidas.
type PresupuestoLinea struct {
	DepartamentoID string `json:"departamento_id"`
	Departamento   string `json:"departamento"`
	Periodo        string `json:"periodo"`
	Monto          string `json:"monto"`
	Nota           string `json:"nota"`
	// ClasificacionID vacío = es el TOTAL del departamento. Con valor = subpresupuesto de esa partida.
	ClasificacionID string `json:"clasificacion_id"`
	Clasificacion   string `json:"clasificacion"`
	Concepto        string `json:"concepto"`
}

// EsTotalDepartamento distingue la línea del total de un subpresupuesto. Sumar las dos clases
// contaría el mismo dinero dos veces, así que quien suma tiene que elegir.
func (l PresupuestoLinea) EsTotalDepartamento() bool { return l.ClasificacionID == "" }

// UsoDepartamento cuenta de qué cuelga una entrada del catálogo. Es lo que decide si se puede
// borrar de verdad o solo desactivar.
type UsoDepartamento struct {
	Facturas          int `json:"facturas"`
	Empleados         int `json:"empleados"`
	Fondos            int `json:"fondos"`
	Validadores       int `json:"validadores"`
	Partidas          int `json:"partidas"`
	Movimientos       int `json:"movimientos"`
	LineasPresupuesto int `json:"lineas_presupuesto"`
}

// Total es cuántas cosas cuelgan en total. Cero = se puede borrar físicamente.
func (u UsoDepartamento) Total() int {
	return u.Facturas + u.Empleados + u.Fondos + u.Validadores +
		u.Partidas + u.Movimientos + u.LineasPresupuesto
}

// Detalle arma la frase que explica por qué no se puede borrar, nombrando solo lo que sí tiene.
func (u UsoDepartamento) Detalle() string {
	var partes []string
	agregar := func(n int, singular, plural string) {
		if n == 1 {
			partes = append(partes, fmt.Sprintf("1 %s", singular))
		} else if n > 1 {
			partes = append(partes, fmt.Sprintf("%d %s", n, plural))
		}
	}
	agregar(u.Facturas, "factura", "facturas")
	agregar(u.Empleados, "empleado", "empleados")
	agregar(u.Fondos, "fondo de caja chica", "fondos de caja chica")
	agregar(u.Validadores, "validador asignado", "validadores asignados")
	agregar(u.Partidas, "partida", "partidas")
	agregar(u.Movimientos, "movimiento bancario", "movimientos bancarios")
	agregar(u.LineasPresupuesto, "línea de presupuesto", "líneas de presupuesto")
	if len(partes) == 0 {
		return ""
	}
	return joinConComas(partes)
}

func joinConComas(partes []string) string {
	switch len(partes) {
	case 1:
		return partes[0]
	default:
		out := ""
		for i, p := range partes {
			switch {
			case i == 0:
				out = p
			case i == len(partes)-1:
				out += " y " + p
			default:
				out += ", " + p
			}
		}
		return out
	}
}

// FilaControl es un departamento con su presupuesto y su gasto real, ya comparados.
type FilaControl struct {
	DepartamentoID string `json:"departamento_id"`
	Departamento   string `json:"departamento"`
	// Presupuesto del rango (la suma de los meses pedidos). Vacío si no se definió ninguno.
	Presupuesto string `json:"presupuesto"`
	// MesesConPresupuesto: sobre cuántos meses del rango hay monto definido. Con 0, el «disponible»
	// no significa nada y la pantalla no debe pintar un semáforo.
	MesesConPresupuesto int    `json:"meses_con_presupuesto"`
	Gasto               string `json:"gasto"`
	Movs                int    `json:"movs"`
	// Disponible = presupuesto − gasto. Negativo = se pasó.
	Disponible string `json:"disponible"`
	// ConsumidoPct sobre el presupuesto (vacío si no hay presupuesto).
	ConsumidoPct string `json:"consumido_pct"`
	// Estado: SIN_PRESUPUESTO | EN_RANGO | ALERTA | EXCEDIDO.
	Estado string `json:"estado"`
	// Subpresupuestos: cuántas partidas de este departamento tienen monto propio definido.
	Subpresupuestos int `json:"subpresupuestos"`
	// SumaSubpresupuestos es lo repartido por partida. Vacío si no hay ninguna.
	SumaSubpresupuestos string `json:"suma_subpresupuestos"`
	// SinRepartir = presupuesto del departamento − suma de sus subpresupuestos. NEGATIVO significa
	// que el desglose reparte MÁS de lo autorizado, que es el error que hay que ver.
	SinRepartir string `json:"sin_repartir"`
}

// Estados de una fila de control presupuestario.
const (
	CtrlSinPresupuesto = "SIN_PRESUPUESTO"
	CtrlEnRango        = "EN_RANGO"
	CtrlAlerta         = "ALERTA"
	CtrlExcedido       = "EXCEDIDO"
)

// ControlPresupuestario es la respuesta completa de la pantalla.
type ControlPresupuestario struct {
	Desde      string `json:"desde"`
	Hasta      string `json:"hasta"`
	AgruparPor string `json:"agrupar_por"`
	// UmbralAlertaPct: a partir de qué % de consumo se marca ALERTA. Lo elige el usuario.
	UmbralAlertaPct string        `json:"umbral_alerta_pct"`
	Filas           []FilaControl `json:"filas"`
	// Totales del rango.
	TotalPresupuesto string `json:"total_presupuesto"`
	TotalGasto       string `json:"total_gasto"`
	// SinAsignar: el gasto que no tiene departamento. Va aparte y NO se reparte.
	SinAsignar     string `json:"sin_asignar"`
	SinAsignarMovs int    `json:"sin_asignar_movs"`
	// Meses del rango con su salud de clasificación: un mes a medio clasificar consume menos
	// presupuesto del que consumió.
	Meses []SaludMes `json:"meses"`
	// Aviso explica en una frase por qué el control puede no ser confiable (vacío = está bien).
	Aviso string `json:"aviso"`
}

// umbralAlertaPorDefecto es el consumo a partir del cual se avisa. Es un DEFAULT de pantalla, no una
// regla: el usuario lo cambia y se ve cuál está aplicado (mismo criterio que el análisis de
// partidas, donde un porcentaje escondido en un `if` decidía qué era «descontrolado»).
const umbralAlertaPorDefecto = 90

// dimensionValida deja pasar solo las dos dimensiones reales. El valor llega del cliente y se
// concatena en el SQL: validarlo acá es lo que impide que se pueda inyectar algo por ese camino.
func dimensionValida(v string) bool {
	return v == AgruparPorDepartamento || v == AgruparPorSede
}

// Sedes devuelve el catálogo de sedes de la empresa.
func (s *Service) Sedes(ctx context.Context, empresaID string, incluirInactivas bool) ([]Sede, error) {
	return s.repo.ListarSedes(ctx, empresaID, incluirInactivas)
}

// DepartamentosDeLaEmpresa devuelve el catálogo de departamentos (administrado en CxP).
func (s *Service) DepartamentosDeLaEmpresa(ctx context.Context, empresaID string) ([]Departamento, error) {
	return s.repo.DepartamentosActivos(ctx, empresaID)
}

// CrearSede agrega una sede al catálogo.
func (s *Service) CrearSede(ctx context.Context, empresaID, nombre, codigo, usuarioID string) (Sede, error) {
	if nombre == "" {
		return Sede{}, ErrNombreRequerido
	}
	sd, err := s.repo.CrearSede(ctx, empresaID, nombre, codigo)
	if err != nil {
		return Sede{}, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "sede", EntidadID: &sd.ID,
		Accion: "CREAR_SEDE", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"nombre": nombre, "codigo": codigo},
	})
	return sd, nil
}

// ActualizarSede renombra una sede.
func (s *Service) ActualizarSede(ctx context.Context, empresaID, sedeID, nombre, codigo, usuarioID string) error {
	if nombre == "" {
		return ErrNombreRequerido
	}
	if err := s.repo.ActualizarSede(ctx, empresaID, sedeID, nombre, codigo); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "sede", EntidadID: &sedeID,
		Accion: "ACTUALIZAR_SEDE", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"nombre": nombre, "codigo": codigo},
	})
	return nil
}

// CambiarActivoSede da de baja o revive una sede.
func (s *Service) CambiarActivoSede(ctx context.Context, empresaID, sedeID string, activo bool, usuarioID string) error {
	if err := s.repo.CambiarActivoSede(ctx, empresaID, sedeID, activo); err != nil {
		return err
	}
	accion := "DESACTIVAR_SEDE"
	if activo {
		accion = "ACTIVAR_SEDE"
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "sede", EntidadID: &sedeID,
		Accion: accion, UsuarioID: &usuarioID,
	})
	return nil
}

// AsignarDimensionesClasificacion define el departamento y la sede POR DEFECTO de una partida, y
// devuelve cuántos movimientos quedan atribuidos por ese solo cambio.
//
// Queda en auditoría porque mueve la atribución de toda la historia de esa partida: si mañana un
// departamento aparece gastando el doble, este evento explica por qué.
func (s *Service) AsignarDimensionesClasificacion(ctx context.Context, empresaID, clasifID, deptoID, sedeID, usuarioID string) (int, error) {
	n, err := s.repo.AsignarDimensionesClasificacion(ctx, empresaID, clasifID, deptoID, sedeID)
	if err != nil {
		return 0, err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "clasificacion", EntidadID: &clasifID,
		Accion: "ASIGNAR_DIMENSIONES_PARTIDA", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"departamento_id": deptoID, "sede_id": sedeID, "movimientos_afectados": n,
		},
	})
	return n, nil
}

// AsignarDimensionesMovimiento escribe la excepción de un movimiento puntual.
func (s *Service) AsignarDimensionesMovimiento(ctx context.Context, empresaID, movID, deptoID, sedeID, usuarioID string) error {
	if err := s.repo.AsignarDimensionesMovimiento(ctx, empresaID, movID, deptoID, sedeID); err != nil {
		return err
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "movimiento_bancario", EntidadID: &movID,
		Accion: "ASIGNAR_DIMENSIONES_MOVIMIENTO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"departamento_id": deptoID, "sede_id": sedeID},
	})
	return nil
}

// PartidasDeDimension dice en qué partidas gastó un departamento (o sede). `dimID` vacío pide
// justamente el gasto SIN dimensión asignada, que es donde arranca el trabajo de atribuirlo.
func (s *Service) PartidasDeDimension(ctx context.Context, empresaID, desde, hasta, agruparPor, dimID, umbral string) ([]GastoPartidaDimension, error) {
	if !dimensionValida(agruparPor) {
		return nil, ErrDimensionInvalida
	}
	filas, err := s.repo.PartidasDeDimension(ctx, empresaID, desde, hasta, agruparPor, dimID)
	if err != nil {
		return nil, err
	}
	umbralPct := umbralDeAlerta(umbral)
	for i := range filas {
		filas[i].OrigenLegible = EtiquetaOrigenDimension(filas[i].Origen)
		filas[i].Estado = CtrlSinPresupuesto

		// Vacío se lee como «no hay subpresupuesto», igual que el cero: es lo mismo para el usuario y
		// no vale un 500. El SQL manda "0.00" por COALESCE, pero la ausencia no puede depender de eso.
		sub := decimal.Zero
		if filas[i].Subpresupuesto != "" {
			var err error
			if sub, err = decimal.NewFromString(filas[i].Subpresupuesto); err != nil {
				return nil, fmt.Errorf("bancos: subpresupuesto de %s: %w", filas[i].Clasificacion, err)
			}
		}
		gasto, err := decimal.NewFromString(filas[i].Gasto)
		if err != nil {
			return nil, fmt.Errorf("bancos: gasto de %s: %w", filas[i].Clasificacion, err)
		}
		if !sub.IsPositive() {
			// Sin monto autorizado no hay semáforo: el "0.00" que llega del SQL no es un presupuesto
			// de cero, es la ausencia de uno.
			filas[i].Subpresupuesto = ""
			continue
		}
		filas[i].Subpresupuesto = sub.StringFixed(2)
		filas[i].Disponible = sub.Sub(gasto).StringFixed(2)
		consumido := gasto.Div(sub).Mul(decimal.NewFromInt(100))
		filas[i].ConsumidoPct = consumido.StringFixed(1)
		filas[i].Estado = estadoDeConsumo(gasto, sub, consumido, umbralPct)
	}
	return filas, nil
}

// umbralDeAlerta resuelve el % de consumo que dispara la ALERTA. Se comparte entre el control por
// departamento y el detalle por partida: dos umbrales distintos harían que la misma plata estuviera
// «en rango» en una pantalla y «en alerta» en la otra.
func umbralDeAlerta(umbral string) decimal.Decimal {
	if umbral != "" {
		if u, err := decimal.NewFromString(umbral); err == nil && u.IsPositive() {
			return u
		}
	}
	return decimal.NewFromInt(umbralAlertaPorDefecto)
}

// estadoDeConsumo es el semáforo, en un solo lugar.
func estadoDeConsumo(gasto, presupuesto, consumidoPct, umbralPct decimal.Decimal) string {
	switch {
	case gasto.GreaterThan(presupuesto):
		return CtrlExcedido
	case consumidoPct.GreaterThanOrEqual(umbralPct):
		return CtrlAlerta
	default:
		return CtrlEnRango
	}
}

// GuardarPresupuesto fija el monto autorizado en un mes. Con `clasifID` vacío es el TOTAL del
// departamento; con valor, el subpresupuesto de esa partida dentro del departamento.
//
// El cuadre entre el total y la suma de sus partidas NO se fuerza acá: bloquear a mitad de la carga
// obligaría a cargar el desglose en un orden concreto (o a subir el total antes de poder repartirlo).
// Se calcula y se muestra en el control, con el número exacto de lo que sobra o falta por repartir.
func (s *Service) GuardarPresupuesto(ctx context.Context, empresaID, deptoID, clasifID, periodo, monto, nota, usuarioID string) error {
	m, err := decimal.NewFromString(monto)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrPresupuestoNegativo, monto)
	}
	if m.IsNegative() {
		return ErrPresupuestoNegativo
	}
	if err := s.repo.GuardarPresupuesto(ctx, empresaID, deptoID, clasifID, periodo, m.StringFixed(2), nota, usuarioID); err != nil {
		return err
	}
	accion := "GUARDAR_PRESUPUESTO"
	if clasifID != "" {
		accion = "GUARDAR_SUBPRESUPUESTO"
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "presupuesto_departamento", EntidadID: &deptoID,
		Accion: accion, UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{
			"periodo": periodo, "monto": m.StringFixed(2), "nota": nota, "clasificacion_id": clasifID,
		},
	})
	return nil
}

// BorrarPresupuesto quita un monto autorizado. Con `clasifID` vacío borra el total del departamento;
// los subpresupuestos de sus partidas quedan, y el control los muestra como «repartido sin total».
func (s *Service) BorrarPresupuesto(ctx context.Context, empresaID, deptoID, clasifID, periodo, usuarioID string) error {
	if err := s.repo.BorrarPresupuesto(ctx, empresaID, deptoID, clasifID, periodo); err != nil {
		return err
	}
	accion := "BORRAR_PRESUPUESTO"
	if clasifID != "" {
		accion = "BORRAR_SUBPRESUPUESTO"
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "presupuesto_departamento", EntidadID: &deptoID,
		Accion: accion, UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"periodo": periodo, "clasificacion_id": clasifID},
	})
	return nil
}

// PresupuestoDelRango devuelve las líneas de presupuesto cargadas en el rango.
func (s *Service) PresupuestoDelRango(ctx context.Context, empresaID, desde, hasta string) ([]PresupuestoLinea, error) {
	return s.repo.PresupuestoDelRango(ctx, empresaID, desde, hasta)
}

// ControlPresupuestario cruza el gasto real con el presupuesto del rango.
//
// El presupuesto solo existe por DEPARTAMENTO (decisión del usuario: se empieza por ahí). Cuando se
// agrupa por sede, las columnas de presupuesto quedan vacías y la pantalla lo dice: es mejor que
// inventar un presupuesto por sede que nadie autorizó.
func (s *Service) ControlPresupuestario(ctx context.Context, empresaID, desde, hasta, agruparPor, umbral string) (ControlPresupuestario, error) {
	if !dimensionValida(agruparPor) {
		return ControlPresupuestario{}, ErrDimensionInvalida
	}
	umbralPct := umbralDeAlerta(umbral)

	gasto, err := s.repo.GastoPorDimension(ctx, empresaID, desde, hasta, agruparPor)
	if err != nil {
		return ControlPresupuestario{}, err
	}
	salud, err := s.repo.SaludMeses(ctx, empresaID, desde, hasta)
	if err != nil {
		return ControlPresupuestario{}, err
	}
	for i := range salud {
		pct, _ := decimal.NewFromString(salud[i].PctClasificado)
		salud[i].Comparable = salud[i].Movs > 0 &&
			pct.GreaterThanOrEqual(decimal.NewFromFloat(umbralClasificadoComparable))
	}

	// El presupuesto solo aplica agrupando por departamento.
	//
	// Las líneas vienen de dos clases y se acumulan SEPARADAS: el total del departamento
	// (clasificacion_id vacío) y el desglose por partida. Sumarlas juntas duplicaría el mismo dinero y
	// haría aparecer presupuesto que nadie autorizó.
	presu := map[string]decimal.Decimal{}
	meses := map[string]int{}
	sub := map[string]decimal.Decimal{}
	subN := map[string]int{}
	var lineas []PresupuestoLinea
	if agruparPor == AgruparPorDepartamento {
		var err error
		lineas, err = s.repo.PresupuestoDelRango(ctx, empresaID, desde, hasta)
		if err != nil {
			return ControlPresupuestario{}, err
		}
		partidasVistas := map[string]bool{}
		for _, l := range lineas {
			m, err := decimal.NewFromString(l.Monto)
			if err != nil {
				return ControlPresupuestario{}, fmt.Errorf("bancos: monto de presupuesto de %s en %s: %w", l.Departamento, l.Periodo, err)
			}
			if l.EsTotalDepartamento() {
				presu[l.DepartamentoID] = presu[l.DepartamentoID].Add(m)
				meses[l.DepartamentoID]++
				continue
			}
			sub[l.DepartamentoID] = sub[l.DepartamentoID].Add(m)
			// Se cuentan PARTIDAS distintas, no líneas: la misma partida con monto en tres meses del
			// rango es un subpresupuesto, no tres.
			if k := l.DepartamentoID + "|" + l.ClasificacionID; !partidasVistas[k] {
				partidasVistas[k] = true
				subN[l.DepartamentoID]++
			}
		}
	}

	// Filas arranca como slice VACÍO y no nil: un slice nil de Go se serializa como `null`, y en el
	// navegador `null.map()` rompe la pantalla entera. Pasó de verdad al abrir esta pantalla con cero
	// departamentos atribuidos, que es justamente el estado inicial de cualquier empresa.
	out := ControlPresupuestario{
		Desde: desde, Hasta: hasta, AgruparPor: agruparPor,
		UmbralAlertaPct: umbralPct.StringFixed(0), Meses: salud,
		Filas: []FilaControl{},
	}
	totalPresu, totalGasto := decimal.Zero, decimal.Zero

	for _, g := range gasto {
		gastoDec, err := decimal.NewFromString(g.Gasto)
		if err != nil {
			return ControlPresupuestario{}, fmt.Errorf("bancos: gasto de %s: %w", g.Nombre, err)
		}
		totalGasto = totalGasto.Add(gastoDec)

		// El gasto sin dueño va aparte y NO se reparte entre los departamentos.
		if g.ID == "" {
			out.SinAsignar = gastoDec.StringFixed(2)
			out.SinAsignarMovs = g.Movs
			continue
		}

		fila := FilaControl{
			DepartamentoID: g.ID, Departamento: g.Nombre,
			Gasto: gastoDec.StringFixed(2), Movs: g.Movs,
			MesesConPresupuesto: meses[g.ID],
			Estado:              CtrlSinPresupuesto,
			Presupuesto:         "",
			Disponible:          "",
			ConsumidoPct:        "",
		}
		if p, hay := presu[g.ID]; hay && p.IsPositive() {
			totalPresu = totalPresu.Add(p)
			fila.Presupuesto = p.StringFixed(2)
			fila.Disponible = p.Sub(gastoDec).StringFixed(2)
			consumido := gastoDec.Div(p).Mul(decimal.NewFromInt(100))
			fila.ConsumidoPct = consumido.StringFixed(1)
			fila.Estado = estadoDeConsumo(gastoDec, p, consumido, umbralPct)
		}
		ponerSubpresupuesto(&fila, presu[g.ID], sub[g.ID], subN[g.ID])
		out.Filas = append(out.Filas, fila)
	}

	// Los departamentos con presupuesto y CERO gasto no aparecen en el gasto real, y son justamente
	// los que hay que ver: tienen plata autorizada sin usar. Se agregan con gasto en cero.
	if agruparPor == AgruparPorDepartamento {
		vistos := map[string]bool{}
		for _, f := range out.Filas {
			vistos[f.DepartamentoID] = true
		}
		agregados := map[string]bool{}
		for _, l := range lineas {
			if vistos[l.DepartamentoID] || agregados[l.DepartamentoID] {
				continue
			}
			p := presu[l.DepartamentoID]
			if !p.IsPositive() {
				continue
			}
			totalPresu = totalPresu.Add(p)
			agregados[l.DepartamentoID] = true
			fila := FilaControl{
				DepartamentoID: l.DepartamentoID, Departamento: l.Departamento,
				Presupuesto: p.StringFixed(2), MesesConPresupuesto: meses[l.DepartamentoID],
				Gasto: "0.00", Movs: 0,
				Disponible: p.StringFixed(2), ConsumidoPct: "0.0", Estado: CtrlEnRango,
			}
			ponerSubpresupuesto(&fila, p, sub[l.DepartamentoID], subN[l.DepartamentoID])
			out.Filas = append(out.Filas, fila)
		}
	}

	out.TotalPresupuesto = totalPresu.StringFixed(2)
	out.TotalGasto = totalGasto.StringFixed(2)
	out.Aviso = avisoControl(out, salud)
	return out, nil
}

// ponerSubpresupuesto llena el cuadre del desglose de una fila.
//
// `SinRepartir` negativo es el hallazgo que justifica todo esto: el desglose por partida reparte MÁS
// de lo que el departamento tiene autorizado, así que alguna partida va a quedar sin plata aunque el
// semáforo del departamento diga «en rango».
func ponerSubpresupuesto(fila *FilaControl, total, repartido decimal.Decimal, n int) {
	if n == 0 {
		return
	}
	fila.Subpresupuestos = n
	fila.SumaSubpresupuestos = repartido.StringFixed(2)
	fila.SinRepartir = total.Sub(repartido).StringFixed(2)
}

// avisoControl dice en una frase por qué el control puede no ser confiable.
func avisoControl(c ControlPresupuestario, salud []SaludMes) string {
	var partes []string

	// Lo más grave primero: un mes a medio clasificar consume MENOS presupuesto del que consumió, y
	// eso invita a seguir gastando sobre un presupuesto que en realidad ya está agotado.
	flojos := 0
	for _, m := range salud {
		if m.Movs > 0 && !m.Comparable {
			flojos++
		}
	}
	if flojos > 0 {
		partes = append(partes, fmt.Sprintf(
			"%d mes(es) del rango están a medio clasificar: el gasto que se ve es MENOR que el real, así que el consumo del presupuesto está subestimado",
			flojos))
	}
	// El MONTO no se escribe en la frase: el backend no formatea moneda y un decimal crudo dentro de
	// una oración («141172906.75 en 720 movimientos») se lee peor que el número ya formateado que la
	// pantalla muestra al lado. Acá va el hecho; el monto es un campo aparte.
	if sa, _ := decimal.NewFromString(c.SinAsignar); sa.IsPositive() {
		partes = append(partes, fmt.Sprintf(
			"%d movimiento(s) de gasto no tienen departamento asignado: esa plata no se le cobra a nadie",
			c.SinAsignarMovs))
	}
	if c.AgruparPor == AgruparPorSede {
		partes = append(partes, "el presupuesto se define por departamento, no por sede: acá solo se ve el gasto real")
	}
	// El desglose que reparte más de lo autorizado se dice acá aunque el semáforo del departamento
	// esté en verde: es un problema del presupuesto, no del gasto, y no se ve mirando el consumo.
	sobregirados := 0
	for _, f := range c.Filas {
		if f.SinRepartir == "" {
			continue
		}
		if sr, err := decimal.NewFromString(f.SinRepartir); err == nil && sr.IsNegative() {
			sobregirados++
		}
	}
	if sobregirados > 0 {
		partes = append(partes, fmt.Sprintf(
			"en %d departamento(s) los subpresupuestos por partida reparten MÁS de lo autorizado al departamento",
			sobregirados))
	}
	if len(partes) == 0 {
		return ""
	}
	return joinConPunto(partes)
}

func joinConPunto(partes []string) string {
	out := ""
	for i, p := range partes {
		if i > 0 {
			out += " · "
		}
		out += p
	}
	return out + "."
}
