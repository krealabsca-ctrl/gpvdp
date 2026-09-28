package bancos

// CARGAR HISTÓRICO — el servicio: resolver contra la base, previsualizar y confirmar.
//
// Son DOS PASOS como el importador de siempre, y por la misma razón: el archivo trae quince cuentas
// y dos años, y nadie puede aprobar eso sin ver antes qué entendió el sistema. El paso 1 guarda el
// archivo y devuelve un RESUMEN POR CUENTA; el paso 2 escribe.
//
// Los dos pasos recorren EXACTAMENTE el mismo plan (`planHistorico`): lo único que cambia es si se
// escribe. Es el mismo contrato del diccionario del catálogo y del otro importador de Excel, y
// existe para que lo que el usuario aprueba sea lo que ocurre.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// CuentaNoResuelta es un nombre de cuenta que el archivo trae y que no existe en la empresa.
//
// La cuenta NO se crea sola: crear una cuenta bancaria por un texto de una celda es exactamente
// cómo se termina con «Promerica Colinas» y «promerica colinas» cobrando plata distinta. El
// Director dijo que él las crea antes; lo que el sistema le debe es el NOMBRE EXACTO que leyó.
type CuentaNoResuelta struct {
	// NombreEnArchivo es el texto tal cual venía en la celda.
	NombreEnArchivo string `json:"nombre_en_archivo"`
	Filas           int    `json:"filas"`
	// PrimeraLinea ubica el problema en el Excel sin contar filas a mano.
	PrimeraLinea int `json:"primera_linea"`
	// Motivo distingue «no existe» de «existe pero está desactivada» y de «hay dos que se llaman
	// igual»: las tres se arreglan distinto.
	Motivo string `json:"motivo"`
}

// ResumenCuentaHistorico es lo que trae el archivo para UNA cuenta.
type ResumenCuentaHistorico struct {
	CuentaBancariaID string `json:"cuenta_bancaria_id"`
	Cuenta           string `json:"cuenta"`
	Banco            string `json:"banco"`
	Moneda           string `json:"moneda"`
	// NombresEnArchivo son las grafías con que el archivo nombró esta cuenta (puede haber más de
	// una: «Davivienda Colones» y su IBAN caen en la misma).
	NombresEnArchivo []string `json:"nombres_en_archivo"`
	Filas            int      `json:"filas"`
	FechaDesde       string   `json:"fecha_desde"`
	FechaHasta       string   `json:"fecha_hasta"`
	// Meses «YYYY-MM» que toca esta cuenta, ordenados.
	Meses []string `json:"meses"`
	// TotalDebitos / TotalCreditos son de TODAS las filas del archivo para esta cuenta.
	TotalDebitos  string `json:"total_debitos"`
	TotalCreditos string `json:"total_creditos"`
	// DebitosNuevos / CreditosNuevos son solo de lo que VA A ENTRAR (descontando lo que ya está).
	//
	// Van aparte de los totales a propósito: en una re-subida los dos pares dicen cosas distintas, y
	// el número que hay que mirar antes de apretar «Confirmar» —y el que queda en la auditoría— es
	// cuánta plata entra, no cuánta traía el archivo.
	DebitosNuevos  string `json:"debitos_nuevos"`
	CreditosNuevos string `json:"creditos_nuevos"`
	// Nuevas / YaExisten: el anti-duplicado. `YaExisten` es lo que NO se va a insertar porque esa
	// misma huella ya cuenta plata en los libros.
	Nuevas    int `json:"nuevas"`
	YaExisten int `json:"ya_existen"`
	// SinPartida: el archivo no dice partida (celda vacía o «Sin clasificar»).
	SinPartida int `json:"sin_partida"`
	// PartidaDesconocida: el archivo nombra una partida que no está en el catálogo. Esos
	// movimientos SE CARGAN IGUAL, sin partida (ver el comentario de `resolverPartida`).
	PartidaDesconocida int `json:"partida_desconocida"`
	// SinTipoCambio: movimientos de una cuenta en dólares cuyo monto en colones queda provisional
	// hasta registrar o congelar el TC del mes. Es lo mismo que hace el importador de siempre.
	SinTipoCambio int `json:"sin_tipo_cambio"`

	// ── solo al confirmar ──
	ImportacionID string `json:"importacion_id"`
	Insertados    int    `json:"insertados"`
}

// TotalesHistorico son los números del archivo entero.
type TotalesHistorico struct {
	// Filas son las de MOVIMIENTO (la decoración del reporte no cuenta acá).
	Filas int `json:"filas"`
	// LineasDeFormato son las filas de adorno del reporte que se saltaron: banda de partida,
	// «Subtotal …», «TOTAL · N movimiento(s)» y el pie. No son errores.
	LineasDeFormato    int    `json:"lineas_de_formato"`
	Nuevas             int    `json:"nuevas"`
	YaExisten          int    `json:"ya_existen"`
	SinPartida         int    `json:"sin_partida"`
	PartidaDesconocida int    `json:"partida_desconocida"`
	SinCuenta          int    `json:"sin_cuenta"`
	Errores            int    `json:"errores"`
	TotalDebitos       string `json:"total_debitos"`
	TotalCreditos      string `json:"total_creditos"`
	FechaDesde         string `json:"fecha_desde"`
	FechaHasta         string `json:"fecha_hasta"`
}

// PlanHistorico es qué va a pasar (o qué pasó) con el archivo entero.
type PlanHistorico struct {
	CargaID       string `json:"carga_id"`
	NombreArchivo string `json:"nombre_archivo"`
	// Hoja leída y todas las hojas del libro: se lee UNA sola, y callarlo esconde trabajo.
	Hoja    string                   `json:"hoja"`
	Hojas   []string                 `json:"hojas"`
	Totales TotalesHistorico         `json:"totales"`
	Cuentas []ResumenCuentaHistorico `json:"cuentas"`
	// CuentasNoResueltas: lo que hay que crear antes de volver a subir.
	CuentasNoResueltas []CuentaNoResuelta `json:"cuentas_no_resueltas"`
	// PartidasFaltantes: los nombres de partida que el archivo trae y el catálogo no tiene.
	PartidasFaltantes []string `json:"partidas_faltantes"`
	// Errores son las filas que no se pudieron leer, con su número de línea y el texto original.
	Errores []ErrorFilaHistorico `json:"errores"`
	// ErroresTruncados avisa que la lista se recortó; el contador de Totales es del total.
	ErroresTruncados bool `json:"errores_truncados"`
	// Aplicado: false = fue una previsualización y no se escribió nada.
	Aplicado bool `json:"aplicado"`
	// Insertados: movimientos efectivamente escritos (solo al confirmar).
	Insertados int `json:"insertados"`
	// Aviso resume en una frase lo que hay que mirar antes de confirmar (vacío = nada que advertir).
	Aviso string `json:"aviso"`
}

// HistoricoBloqueadoError frena la carga contra un mes cerrado o un acta firmada.
//
// Es un error TIPADO y no un centinela porque el detalle —QUÉ mes y QUÉ cuenta— es la mitad útil
// del mensaje: «no se puede cargar» a secas obliga a adivinar cuál de los veinticuatro meses del
// archivo es el que estorba.
type HistoricoBloqueadoError struct {
	PeriodosCerrados []string
	ActasFirmadas    []string
}

func (e *HistoricoBloqueadoError) Error() string {
	var partes []string
	if len(e.PeriodosCerrados) > 0 {
		partes = append(partes, "el período "+strings.Join(e.PeriodosCerrados, ", ")+" ya está cerrado")
	}
	if len(e.ActasFirmadas) > 0 {
		partes = append(partes, "ya está firmada el acta de conciliación de "+strings.Join(e.ActasFirmadas, ", "))
	}
	return "bancos: no se puede cargar histórico porque " + strings.Join(partes, " y ") +
		". Sacá esos meses del archivo, o reabrí el período, y volvé a subirlo."
}

// CuentaHistorica es una cuenta de la empresa lista para resolver nombres del archivo contra ella.
type CuentaHistorica struct {
	ID     string
	Alias  string
	Banco  string
	IBAN   string
	Moneda string
	Activo bool
}

// MovimientoHistorico es una fila lista para persistir, con su partida ya resuelta.
type MovimientoHistorico struct {
	NaturalKey       string
	Fecha            time.Time
	Documento        string
	Descripcion      string
	Debito           decimal.Decimal
	Credito          decimal.Decimal
	MontoOriginal    decimal.Decimal
	MontoCRC         decimal.Decimal
	IndiceOcurrencia int
	// ConceptoID / ClasificacionID vacíos = se carga sin partida (ver `resolverPartida`).
	ConceptoID      string
	ClasificacionID string
}

// LoteHistoricoCuenta es todo lo que hay que escribir para UNA cuenta.
type LoteHistoricoCuenta struct {
	CuentaID string
	Banco    string
	Moneda   string
	Movs     []MovimientoHistorico
}

// ResultadoLoteHistorico es lo que la transacción efectivamente escribió para una cuenta.
type ResultadoLoteHistorico struct {
	CuentaID      string
	ImportacionID string
	Insertados    int
}

// SubirHistorico guarda el archivo y devuelve la previsualización. NO escribe ni un movimiento.
func (s *Service) SubirHistorico(ctx context.Context, empresaID, nombre string, archivo []byte, usuarioID string) (PlanHistorico, error) {
	plan, _, err := s.planHistorico(ctx, empresaID, archivo)
	if err != nil {
		return PlanHistorico{}, err
	}
	// El archivo se guarda DESPUÉS de que se entendió: una carga que ni siquiera tiene encabezado
	// no tiene por qué ocupar una fila que después alguien tenga que limpiar.
	cargaID, err := s.repo.CrearCargaHistorica(ctx, empresaID, nombre, sha256hex(archivo), archivo, usuarioID)
	if err != nil {
		return PlanHistorico{}, err
	}
	plan.CargaID, plan.NombreArchivo = cargaID, nombre
	return plan, nil
}

// PrevisualizarHistorico reconstruye la previsualización de una carga ya subida (re-lee el original).
func (s *Service) PrevisualizarHistorico(ctx context.Context, empresaID, cargaID string) (PlanHistorico, error) {
	carga, err := s.repo.CargaHistorica(ctx, empresaID, cargaID)
	if err != nil {
		return PlanHistorico{}, err
	}
	plan, _, err := s.planHistorico(ctx, empresaID, carga.Archivo)
	if err != nil {
		return PlanHistorico{}, err
	}
	plan.CargaID, plan.NombreArchivo = carga.ID, carga.NombreArchivo
	plan.Aplicado = carga.Estado == EstadoCargaHistoricaConfirmada
	return plan, nil
}

// ConfirmarHistorico escribe: una importación POR CUENTA y sus movimientos, con la partida puesta.
func (s *Service) ConfirmarHistorico(ctx context.Context, empresaID, cargaID, usuarioID string) (PlanHistorico, error) {
	carga, err := s.repo.CargaHistorica(ctx, empresaID, cargaID)
	if err != nil {
		return PlanHistorico{}, err
	}
	if carga.Estado == EstadoCargaHistoricaConfirmada {
		return PlanHistorico{}, ErrCargaHistoricaYaConfirmada
	}

	plan, lotes, err := s.planHistorico(ctx, empresaID, carga.Archivo)
	if err != nil {
		return PlanHistorico{}, err
	}
	plan.CargaID, plan.NombreArchivo = carga.ID, carga.NombreArchivo
	if len(lotes) == 0 {
		return PlanHistorico{}, ErrHistoricoSinNadaQueCargar
	}

	// BLOQUEO de calendario. Va ANTES de escribir y mirando TODO el archivo: rechazar a mitad de
	// camino dejaría ocho cuentas cargadas y siete no, que es peor que no empezar.
	if err := s.bloqueosDeCargaHistorica(ctx, empresaID, lotes); err != nil {
		return PlanHistorico{}, err
	}

	resultados, err := s.repo.ConfirmarCargaHistorica(ctx, empresaID, cargaID, carga.NombreArchivo,
		carga.SourceFileHash, usuarioID, lotes)
	if err != nil {
		return PlanHistorico{}, err
	}

	porCuenta := map[string]ResultadoLoteHistorico{}
	for _, r := range resultados {
		porCuenta[r.CuentaID] = r
	}
	for i := range plan.Cuentas {
		if r, ok := porCuenta[plan.Cuentas[i].CuentaBancariaID]; ok {
			plan.Cuentas[i].ImportacionID = r.ImportacionID
			plan.Cuentas[i].Insertados = r.Insertados
			plan.Insertados += r.Insertados
		}
	}
	plan.Aplicado = true

	// Conversión a colones de lo recién cargado (cuentas en otra moneda). Es lo MISMO que hace el
	// confirmar de siempre y a propósito: si el TC del mes ya existe, el movimiento no puede quedar
	// en cero por haber entrado después. Best-effort: no bloquea la carga.
	//
	// Lo que NO se corre acá es el MOTOR DE REGLAS. El archivo ya viene segmentado por el Director
	// Financiero —ese es el trabajo que esta pantalla viene a traer— y el motor le pisaría encima
	// su clasificación con la suya. Por eso los movimientos con partida entran como REVISADO.
	for _, lote := range lotes {
		if lote.Moneda == "CRC" {
			continue
		}
		paraTC := make([]MovimientoParaInsertar, 0, len(lote.Movs))
		for _, m := range lote.Movs {
			paraTC = append(paraTC, MovimientoParaInsertar{Fecha: m.Fecha})
		}
		s.AplicarTCImportado(ctx, empresaID, lote.Moneda, paraTC)
	}

	// Un evento POR CUENTA, con cuántos movimientos y las dos sumas: es el registro de cuánta plata
	// entró a los libros por esta carga y en qué cuenta.
	for i := range plan.Cuentas {
		c := plan.Cuentas[i]
		if c.ImportacionID == "" {
			continue
		}
		impID := c.ImportacionID
		s.audit.Registrar(ctx, shared.Evento{
			EmpresaID: &empresaID, Entidad: "importacion", EntidadID: &impID,
			Accion: "CARGAR_HISTORICO", UsuarioID: &usuarioID,
			// Las dos sumas del evento son las de lo que ENTRÓ, no las del archivo: el evento es el
			// registro de cuánta plata se metió a los libros, y en una re-subida parcial el total
			// del archivo diría de más.
			ValorNuevo: map[string]any{
				"carga_historica": cargaID,
				"nombre_archivo":  carga.NombreArchivo,
				"cuenta":          c.Cuenta,
				"movimientos":     c.Insertados,
				"total_debitos":   c.DebitosNuevos,
				"total_creditos":  c.CreditosNuevos,
				"moneda":          c.Moneda,
				"desde":           c.FechaDesde,
				"hasta":           c.FechaHasta,
			},
		})
	}

	plan.Aviso = avisoHistorico(plan)
	return plan, nil
}

// bloqueosDeCargaHistorica rechaza la carga si toca un mes CERRADO o una cuenta/mes con acta de
// conciliación FIRMADA.
//
// Meter movimientos dentro de un mes cerrado rompe el cierre —que se firmó diciendo que el 100 %
// estaba clasificado— y dentro de un mes con acta firmada rompe el cuadre que alguien ya firmó.
// Es el mismo daño que la reversa evita en el otro sentido.
func (s *Service) bloqueosDeCargaHistorica(ctx context.Context, empresaID string, lotes []LoteHistoricoCuenta) error {
	var cuentas []string
	var anios, meses []int
	vistos := map[string]bool{}
	for _, lote := range lotes {
		for _, m := range lote.Movs {
			k := fmt.Sprintf("%s|%04d-%02d", lote.CuentaID, m.Fecha.Year(), int(m.Fecha.Month()))
			if vistos[k] {
				continue
			}
			vistos[k] = true
			cuentas = append(cuentas, lote.CuentaID)
			anios = append(anios, m.Fecha.Year())
			meses = append(meses, int(m.Fecha.Month()))
		}
	}
	if len(cuentas) == 0 {
		return nil
	}
	periodos, actas, err := s.repo.BloqueosDeCargaHistorica(ctx, empresaID, cuentas, anios, meses)
	if err != nil {
		return err
	}
	if len(periodos) > 0 || len(actas) > 0 {
		return &HistoricoBloqueadoError{PeriodosCerrados: periodos, ActasFirmadas: actas}
	}
	return nil
}

// planHistorico lee el archivo, resuelve cuenta y partida de cada fila y arma el plan.
//
// Devuelve además los lotes listos para escribir, para que previsualizar y confirmar recorran el
// MISMO camino: si el plan que se muestra saliera de un cálculo y lo que se escribe de otro, la
// pantalla podría prometer algo distinto de lo que pasa.
func (s *Service) planHistorico(ctx context.Context, empresaID string, archivo []byte) (PlanHistorico, []LoteHistoricoCuenta, error) {
	formateada, cruda, hoja, hojas, date1904, err := gridDeArchivoHistorico(archivo, hojaMovimientos)
	if err != nil {
		return PlanHistorico{}, nil, err
	}
	lec, err := LeerHistorico(formateada, cruda, date1904)
	if err != nil {
		return PlanHistorico{}, nil, err
	}
	lec.Hoja, lec.Hojas = hoja, hojas

	cuentas, err := s.repo.CuentasHistorico(ctx, empresaID)
	if err != nil {
		return PlanHistorico{}, nil, err
	}
	clasifs, err := s.repo.ListarClasificaciones(ctx, empresaID, false)
	if err != nil {
		return PlanHistorico{}, nil, err
	}

	idx := indiceDeCuentas(cuentas)
	porPartida, porNombre := indiceDePartidas(clasifs)

	plan := PlanHistorico{
		Hoja: lec.Hoja, Hojas: lec.Hojas,
		Cuentas:            []ResumenCuentaHistorico{},
		CuentasNoResueltas: []CuentaNoResuelta{},
		PartidasFaltantes:  []string{},
		Errores:            []ErrorFilaHistorico{},
		Totales: TotalesHistorico{
			Filas: len(lec.Filas), LineasDeFormato: lec.LineasDeFormato, Errores: len(lec.Errores),
		},
	}
	plan.Errores = lec.Errores
	if len(plan.Errores) > maxErroresHistorico {
		plan.Errores = plan.Errores[:maxErroresHistorico]
		plan.ErroresTruncados = true
	}

	// ── Paso 1: repartir las filas por cuenta ───────────────────────────────
	type acumulado struct {
		cuenta   CuentaHistorica
		nombres  map[string]bool
		filas    []FilaHistorico
		partidas []string // concepto|clasificacion resuelto de cada fila, en paralelo
	}
	orden := []string{}
	porCuenta := map[string]*acumulado{}
	noResueltas := map[string]*CuentaNoResuelta{}
	faltantes := map[string]bool{}

	for _, f := range lec.Filas {
		c, motivo := idx.resolver(f.Cuenta)
		if motivo != "" {
			clave := norm(f.Cuenta)
			nr, ok := noResueltas[clave]
			if !ok {
				nr = &CuentaNoResuelta{NombreEnArchivo: f.Cuenta, PrimeraLinea: f.Linea, Motivo: motivo}
				noResueltas[clave] = nr
			}
			nr.Filas++
			plan.Totales.SinCuenta++
			continue
		}
		a, ok := porCuenta[c.ID]
		if !ok {
			a = &acumulado{cuenta: c, nombres: map[string]bool{}}
			porCuenta[c.ID] = a
			orden = append(orden, c.ID)
		}
		if t := strings.TrimSpace(f.Cuenta); t != "" {
			a.nombres[t] = true
		}
		a.filas = append(a.filas, f)
	}

	// ── Paso 2: por cuenta, resolver partida, huella y anti-duplicado ───────
	lotes := make([]LoteHistoricoCuenta, 0, len(orden))
	totalDeb, totalCre := decimal.Zero, decimal.Zero
	var desdeGlobal, hastaGlobal string

	for _, cuentaID := range orden {
		a := porCuenta[cuentaID]
		res := ResumenCuentaHistorico{
			CuentaBancariaID: a.cuenta.ID,
			Cuenta:           a.cuenta.Alias,
			Banco:            a.cuenta.Banco,
			Moneda:           a.cuenta.Moneda,
			NombresEnArchivo: clavesOrdenadas(a.nombres),
			Filas:            len(a.filas),
		}

		// El índice de ocurrencia distingue los duplicados LEGÍTIMOS dentro del archivo (el mismo
		// monto el mismo día con el mismo documento pasa de verdad). Se cuenta por ARCHIVO Y CUENTA,
		// igual que en el importador normal, así subir dos veces el mismo archivo da las mismas
		// huellas y el anti-duplicado las reconoce.
		ocurrencias := map[string]int{}
		movs := make([]MovimientoHistorico, 0, len(a.filas))
		claves := make([]string, 0, len(a.filas))
		meses := map[string]bool{}
		deb, cre := decimal.Zero, decimal.Zero

		for _, f := range a.filas {
			base := strings.Join([]string{
				f.Fecha.Format("2006-01-02"), f.Debito.String(), f.Credito.String(), f.Documento,
			}, "|")
			ocurrencias[base]++
			parsed := MovimientoParsed{
				Fecha: f.Fecha, Documento: f.Documento, Descripcion: f.Descripcion,
				Debito: f.Debito, Credito: f.Credito, IndiceOcurrencia: ocurrencias[base],
			}
			nk := naturalKey(a.cuenta.ID, parsed)

			conceptoID, clasifID, estadoPartida, faltante := resolverPartida(f, porPartida, porNombre)
			switch estadoPartida {
			case partidaSinDeclarar:
				res.SinPartida++
			case partidaDesconocida:
				res.PartidaDesconocida++
				faltantes[faltante] = true
			}

			monto := f.Debito.Add(f.Credito) // exactamente uno es > 0: lo garantiza el lector
			montoCRC := decimal.Zero
			if a.cuenta.Moneda == "CRC" {
				montoCRC = monto
			}
			// En una cuenta en otra moneda el monto en colones queda PROVISIONAL hasta registrar o
			// congelar el TC del mes: es exactamente lo que hace el confirmar de siempre. Cuántos
			// quedan así se cuenta más abajo, sobre los que de verdad van a entrar.

			movs = append(movs, MovimientoHistorico{
				NaturalKey: nk, Fecha: f.Fecha, Documento: f.Documento, Descripcion: f.Descripcion,
				Debito: f.Debito, Credito: f.Credito,
				MontoOriginal: monto, MontoCRC: montoCRC,
				IndiceOcurrencia: ocurrencias[base],
				ConceptoID:       conceptoID, ClasificacionID: clasifID,
			})
			claves = append(claves, nk)
			meses[f.Fecha.Format("2006-01")] = true
			deb, cre = deb.Add(f.Debito), cre.Add(f.Credito)
			res.FechaDesde = menorFecha(res.FechaDesde, f.Fecha.Format("2006-01-02"))
			res.FechaHasta = mayorFecha(res.FechaHasta, f.Fecha.Format("2006-01-02"))
		}

		existentes, err := s.repo.NaturalKeysExistentes(ctx, empresaID, claves)
		if err != nil {
			return PlanHistorico{}, nil, err
		}
		nuevos := make([]MovimientoHistorico, 0, len(movs))
		debNuevos, creNuevos := decimal.Zero, decimal.Zero
		for _, m := range movs {
			if existentes[m.NaturalKey] {
				res.YaExisten++
				continue
			}
			res.Nuevas++
			debNuevos, creNuevos = debNuevos.Add(m.Debito), creNuevos.Add(m.Credito)
			nuevos = append(nuevos, m)
		}
		if a.cuenta.Moneda != "CRC" {
			// Se cuenta sobre lo que VA A ENTRAR: lo que ya estaba cargado ya tiene (o no) su tipo
			// de cambio, y contarlo acá inflaría la advertencia en cada re-subida.
			res.SinTipoCambio = res.Nuevas
		}

		res.Meses = clavesOrdenadas(meses)
		res.TotalDebitos, res.TotalCreditos = deb.StringFixed(2), cre.StringFixed(2)
		res.DebitosNuevos, res.CreditosNuevos = debNuevos.StringFixed(2), creNuevos.StringFixed(2)
		plan.Cuentas = append(plan.Cuentas, res)
		plan.Totales.Nuevas += res.Nuevas
		plan.Totales.YaExisten += res.YaExisten
		plan.Totales.SinPartida += res.SinPartida
		plan.Totales.PartidaDesconocida += res.PartidaDesconocida
		totalDeb, totalCre = totalDeb.Add(deb), totalCre.Add(cre)
		desdeGlobal = menorFecha(desdeGlobal, res.FechaDesde)
		hastaGlobal = mayorFecha(hastaGlobal, res.FechaHasta)

		if len(nuevos) > 0 {
			lotes = append(lotes, LoteHistoricoCuenta{
				CuentaID: a.cuenta.ID, Banco: a.cuenta.Banco, Moneda: a.cuenta.Moneda, Movs: nuevos,
			})
		}
	}

	for _, nr := range noResueltas {
		plan.CuentasNoResueltas = append(plan.CuentasNoResueltas, *nr)
	}
	sort.Slice(plan.CuentasNoResueltas, func(i, j int) bool {
		return plan.CuentasNoResueltas[i].PrimeraLinea < plan.CuentasNoResueltas[j].PrimeraLinea
	})
	plan.PartidasFaltantes = clavesOrdenadas(faltantes)
	plan.Totales.TotalDebitos, plan.Totales.TotalCreditos = totalDeb.StringFixed(2), totalCre.StringFixed(2)
	plan.Totales.FechaDesde, plan.Totales.FechaHasta = desdeGlobal, hastaGlobal
	plan.Aviso = avisoHistorico(plan)
	return plan, lotes, nil
}

// Estados de la partida de una fila.
const (
	partidaResuelta    = iota // el archivo la trae y está en el catálogo
	partidaSinDeclarar        // el archivo no la trae (celda vacía o «Sin clasificar»)
	partidaDesconocida        // el archivo la nombra y el catálogo no la tiene
)

// resolverPartida busca el concepto y la clasificación del archivo en el catálogo.
//
// ── LA DECISIÓN ─────────────────────────────────────────────────────────────
//
// Si la partida NO existe, el movimiento SE CARGA IGUAL pero SIN partida, y se reporta cuántos y
// qué nombres faltaron. Perder la plata por no encontrar una etiqueta sería peor que cargarla sin
// etiqueta: lo segundo se arregla después desde la pantalla de clasificar de siempre, lo primero no
// se nota hasta que no cuadra el año.
//
// El caso del nombre repetido en dos conceptos se resuelve igual que en `planClasifExcel`: cuando
// el archivo no trae el concepto y el nombre existe bajo dos, NO se elige uno. Se carga sin partida
// y se reporta, porque el 25 % del dinero de una empresa puede depender de cuál era.
func resolverPartida(f FilaHistorico, porPartida map[string]ClasificacionItem, porNombre map[string][]ClasificacionItem) (conceptoID, clasifID string, estado int, faltante string) {
	if f.Clasificacion == "" && f.Concepto == "" {
		return "", "", partidaSinDeclarar, ""
	}
	if f.Clasificacion == "" {
		// Trae el concepto pero no la clasificación: la partida son las dos cosas.
		return "", "", partidaDesconocida, f.Concepto + " › (sin clasificación)"
	}
	if f.Concepto != "" {
		cl, ok := porPartida[claveClasif(f.Concepto, f.Clasificacion)]
		if !ok {
			return "", "", partidaDesconocida, f.Concepto + " › " + f.Clasificacion
		}
		return cl.ConceptoID, cl.ID, partidaResuelta, ""
	}
	candidatas := porNombre[norm(f.Clasificacion)]
	switch len(candidatas) {
	case 1:
		return candidatas[0].ConceptoID, candidatas[0].ID, partidaResuelta, ""
	case 0:
		return "", "", partidaDesconocida, f.Clasificacion
	default:
		nombres := make([]string, 0, len(candidatas))
		for _, c := range candidatas {
			nombres = append(nombres, c.Concepto)
		}
		return "", "", partidaDesconocida,
			fmt.Sprintf("%s (está en %d conceptos: %s — agregá la columna Concepto)",
				f.Clasificacion, len(candidatas), strings.Join(nombres, ", "))
	}
}

// indiceDePartidas arma los dos índices del catálogo: por (concepto, clasificación) y por nombre.
func indiceDePartidas(clasifs []ClasificacionItem) (map[string]ClasificacionItem, map[string][]ClasificacionItem) {
	porPartida := map[string]ClasificacionItem{}
	porNombre := map[string][]ClasificacionItem{}
	for _, cl := range clasifs {
		porPartida[claveClasif(cl.Concepto, cl.Nombre)] = cl
		n := norm(cl.Nombre)
		porNombre[n] = append(porNombre[n], cl)
	}
	return porPartida, porNombre
}

// indiceCuentas resuelve el texto de la celda «Banco Cuenta» contra las cuentas de la empresa.
//
// Son TRES mapas y no uno, EN ESTE ORDEN DE PRIORIDAD, y la separación no es cosmética:
//
//  1. porAlias — el alias pelado («Davivienda Colones»), que es lo que el export escribe para 13 de
//     las 15 cuentas, porque `bancoCuenta()` no antepone el banco cuando el alias ya lo menciona;
//  2. porCompuesto — «Banco · Alias» y «Banco Alias» («Banco Popular · BP Negocios»), que es lo que
//     el export escribe para las otras dos;
//  3. porIBAN — el IBAN sin espacios ni guiones, para quien pega la columna del banco.
//
// Con UN solo mapa, una clave compuesta puede chocar contra el ALIAS de otra cuenta —banco «BN» +
// alias «Jardines Colones» da la misma cadena que el alias «BN Jardines Colones»— y la colisión
// marcaría AMBIGUA una clave que estaba perfecta: una cuenta real dejaría de resolverse por culpa
// de una grafía inventada por el índice. Separados, la forma más específica gana y la tolerancia
// extra no puede romper nada.
type indiceCuentas struct {
	porAlias     map[string]string // clave normalizada → id de cuenta (o "?" si dos la comparten)
	porCompuesto map[string]string
	porIBAN      map[string]string
	porID        map[string]CuentaHistorica
}

const cuentaHistAmbigua = "?"

// indiceDeCuentas arma el índice con todas las formas en que el archivo puede nombrar una cuenta.
//
// Cuando dos cuentas empatan en una clave normalizada, la clave se marca AMBIGUA en vez de quedarse
// con la última: cargar los movimientos en la cuenta equivocada es exactamente el incidente que
// originó la reversa (mig 0085).
func indiceDeCuentas(cuentas []CuentaHistorica) indiceCuentas {
	idx := indiceCuentas{
		porAlias: map[string]string{}, porCompuesto: map[string]string{},
		porIBAN: map[string]string{}, porID: map[string]CuentaHistorica{},
	}
	agregar := func(m map[string]string, clave, id string) {
		clave = norm(clave)
		if clave == "" {
			return
		}
		if ya, hay := m[clave]; hay && ya != id {
			m[clave] = cuentaHistAmbigua
			return
		}
		m[clave] = id
	}
	for _, c := range cuentas {
		idx.porID[c.ID] = c
		agregar(idx.porAlias, c.Alias, c.ID)
		agregar(idx.porAlias, sinEspaciosNiGuiones(c.Alias), c.ID)
		agregar(idx.porCompuesto, c.Banco+" · "+c.Alias, c.ID)
		agregar(idx.porCompuesto, c.Banco+" "+c.Alias, c.ID)
		agregar(idx.porIBAN, sinEspaciosNiGuiones(c.IBAN), c.ID)
	}
	return idx
}

func sinEspaciosNiGuiones(s string) string {
	return strings.NewReplacer(" ", "", "-", "").Replace(s)
}

// resolver devuelve la cuenta, o el motivo por el que no se pudo.
//
// El motivo NOMBRA lo que se leyó. El otro importador tiene un mensaje engañoso heredado («la fila
// no dice de qué cuenta es») que sale incluso cuando la fila SÍ lo dice; acá eso no puede pasar.
func (idx indiceCuentas) resolver(texto string) (CuentaHistorica, string) {
	t := strings.TrimSpace(texto)
	if t == "" {
		return CuentaHistorica{}, "la fila no dice de qué cuenta es: la columna «Banco Cuenta» está vacía"
	}
	pegado := norm(sinEspaciosNiGuiones(t))
	id := ""
	for _, m := range []map[string]string{idx.porAlias, idx.porCompuesto, idx.porIBAN} {
		if v := m[norm(t)]; v != "" {
			id = v
			break
		}
		if v := m[pegado]; v != "" {
			id = v
			break
		}
	}
	switch {
	case id == "":
		return CuentaHistorica{}, fmt.Sprintf(
			"no hay ninguna cuenta que se llame %q ni con ese IBAN: creala en Bancos › Cuentas y volvé a subir el archivo", t)
	case id == cuentaHistAmbigua:
		return CuentaHistorica{}, fmt.Sprintf(
			"hay más de una cuenta que se llama %q: renombralas para distinguirlas", t)
	}
	c := idx.porID[id]
	if !c.Activo {
		return CuentaHistorica{}, fmt.Sprintf(
			"la cuenta %q está desactivada: activala en Bancos › Cuentas para poder cargarle histórico", t)
	}
	return c, ""
}

func clavesOrdenadas(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func menorFecha(actual, nueva string) string {
	if actual == "" || (nueva != "" && nueva < actual) {
		return nueva
	}
	return actual
}

func mayorFecha(actual, nueva string) string {
	if nueva > actual {
		return nueva
	}
	return actual
}

// avisoHistorico resume en una frase qué hay que mirar antes de confirmar.
func avisoHistorico(p PlanHistorico) string {
	var partes []string
	if len(p.CuentasNoResueltas) > 0 {
		filas := 0
		nombres := make([]string, 0, len(p.CuentasNoResueltas))
		for _, c := range p.CuentasNoResueltas {
			filas += c.Filas
			nombres = append(nombres, "«"+c.NombreEnArchivo+"»")
		}
		partes = append(partes, fmt.Sprintf(
			"%d fila(s) quedan afuera porque su cuenta no existe todavía: %s", filas, strings.Join(nombres, ", ")))
	}
	if p.Totales.Errores > 0 {
		partes = append(partes, fmt.Sprintf("%d fila(s) no se pudieron leer", p.Totales.Errores))
	}
	if p.Totales.PartidaDesconocida > 0 {
		partes = append(partes, fmt.Sprintf(
			"%d movimiento(s) nombran una partida que no está en el catálogo y se cargan SIN partida (%s)",
			p.Totales.PartidaDesconocida, strings.Join(p.PartidasFaltantes, ", ")))
	}
	if p.Totales.YaExisten > 0 {
		partes = append(partes, fmt.Sprintf(
			"%d movimiento(s) ya están cargados y no se repiten", p.Totales.YaExisten))
	}
	// Los movimientos en otra moneda entran con el monto en colones PROVISIONAL: vale cero hasta
	// que el mes tenga tipo de cambio registrado o congelado. Está en el desglose por cuenta, pero
	// el histórico son meses viejos para los que nadie fue a buscar el TC: si no se dice también
	// acá arriba, esas cuentas aparecen valiendo ¢0 en todo reporte en colones y nadie lo relaciona
	// con esta carga.
	sinTC, cuentasSinTC := 0, []string{}
	for _, c := range p.Cuentas {
		if c.SinTipoCambio > 0 {
			sinTC += c.SinTipoCambio
			cuentasSinTC = append(cuentasSinTC, c.Cuenta)
		}
	}
	if sinTC > 0 {
		partes = append(partes, fmt.Sprintf(
			"%d movimiento(s) en otra moneda (%s) entran con el monto en colones PROVISIONAL: queda "+
				"en ¢0 hasta que el mes tenga tipo de cambio registrado o congelado",
			sinTC, strings.Join(cuentasSinTC, ", ")))
	}
	if p.ErroresTruncados {
		partes = append(partes, fmt.Sprintf(
			"la lista de errores muestra los primeros %d de %d (el contador es del total)",
			maxErroresHistorico, p.Totales.Errores))
	}
	// Se lee UNA hoja. Si el libro traía más, hay que decirlo: alguien puede haber puesto una hoja
	// por cuenta y el resumen se vería igual de exitoso habiendo leído solo una.
	if len(p.Hojas) > 1 {
		otras := make([]string, 0, len(p.Hojas)-1)
		for _, h := range p.Hojas {
			if h != p.Hoja {
				otras = append(otras, h)
			}
		}
		partes = append(partes, fmt.Sprintf(
			"el libro tiene %d hojas y solo se leyó «%s»: %s quedó sin leer (subilas de a una)",
			len(p.Hojas), p.Hoja, strings.Join(otras, ", ")))
	}
	if len(partes) == 0 {
		return ""
	}
	return strings.Join(partes, " · ") + "."
}
