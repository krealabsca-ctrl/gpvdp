package bancos

// Revertir una CARGA entera (mig 0085).
//
// La unidad que se revierte es la IMPORTACIÓN, no la fecha ni la cuenta. Viene de la regla textual
// del Director Financiero: «del día 20 hacia atrás todo está bien y el 21 agregué mal los bancos,
// lo que debo corregir es lo cargado el 21 no lo que ya estaba cargado el 20». Un recorte por fecha
// se llevaría puesto lo que otra carga dejó bien en la misma cuenta y el mismo mes; revertir la
// carga no toca ni una fila que no haya venido en ese archivo.
//
// Revertir = `incluido = false` en sus movimientos + estado REVERTIDA + evento de auditoría.
// NUNCA un DELETE: es la regla del proyecto para tablas financieras, y además la clasificación
// hecha sobre esos movimientos sigue sirviendo si mañana el archivo se importa en la cuenta buena.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// EstadoImportacionRevertida es el estado de una carga sacada de los libros (mig 0085).
const EstadoImportacionRevertida = "REVERTIDA"

var (
	// ErrImportacionYaRevertida indica que la carga ya está fuera de los libros.
	ErrImportacionYaRevertida = errors.New("bancos: esa carga ya fue revertida")
	// ErrImportacionNoRevertida indica que no hay reversa que deshacer.
	ErrImportacionNoRevertida = errors.New("bancos: esa carga no está revertida; no hay nada que deshacer")
	// ErrMotivoReversaRequerido: sin motivo, dentro de tres meses nadie sabe por qué falta la plata.
	ErrMotivoReversaRequerido = errors.New("bancos: hace falta escribir por qué se revierte la carga")
	// ErrImportacionRevertidaNoSeConfirma frena el re-confirm de una carga ya revertida.
	//
	// El archivo sigue guardado en `importacion.archivo` y la previsualización sigue funcionando,
	// así que apretar «Confirmar» de nuevo sobre una carga revertida es un clic de distancia. Sin
	// este freno, el UPDATE de estado choca contra el CHECK `importacion_reversa_coherente` y sale
	// un 500 sin explicación. El camino correcto es deshacer la reversa, que es lo que dice acá.
	ErrImportacionRevertidaNoSeConfirma = errors.New(
		"bancos: esa carga está revertida; deshacé la reversa en vez de volver a confirmarla")
)

// BloqueosReversa enumera lo que YA se apoya en los movimientos de una carga.
//
// Existe porque excluir un movimiento que sostiene una planilla de cobro, una obligación del
// calendario o un acta firmada rompería «lo que está bien», que es lo único que el Director pidió
// que no pasara. Con cualquiera de estos en el medio la reversa se RECHAZA (422) y el mensaje dice
// qué y cuántos, para que se pueda ir a deshacer eso primero.
//
// Viaja también en el listado: así el botón sale deshabilitado con la razón a la vista, en vez de
// fallar recién al apretarlo.
type BloqueosReversa struct {
	// CobrosCxC: cobros de Cuentas por Cobrar identificados contra un movimiento de esta carga.
	CobrosCxC int `json:"cobros_cxc"`
	// PlanillasCxC: movimientos vinculados a una planilla de asociación de CxC.
	PlanillasCxC int `json:"planillas_cxc"`
	// AvisosSinResolver: avisos de «esto no es de mi partida» todavía en revisión.
	AvisosSinResolver int `json:"avisos_sin_resolver"`
	// Responsabilidades: obligaciones del calendario de CxP que se dan por cumplidas con uno de
	// estos movimientos.
	Responsabilidades int `json:"responsabilidades"`
	// FacturasCxP: movimientos que son el PAGO de una factura de CxP (la huella `CXP-xxxx` los
	// emparejó al confirmar la carga y CxP pasó la factura a CONCILIADO por eso). Excluirlos deja a
	// la factura conciliada contra plata que ya no está en los libros, y el barrido de huellas no
	// puede volver a mirarla porque filtra por `incluido`: el enlace queda huérfano para siempre.
	FacturasCxP int `json:"facturas_cxp"`
	// TrasladosEmparejados: movimientos que ya forman par de traslado/overnight (apuntan a otro, o
	// alguien los apunta).
	TrasladosEmparejados int `json:"traslados_emparejados"`
	// PeriodosCerrados son los meses «YYYY-MM» ya cerrados donde cae algún movimiento.
	PeriodosCerrados []string `json:"periodos_cerrados"`
	// ActasFirmadas son los meses «YYYY-MM» con acta de conciliación FIRMADA de esa cuenta.
	ActasFirmadas []string `json:"actas_firmadas"`
	// LineasReimportadas es la OTRA MITAD de «la reversa libera la línea» (mig 0086).
	//
	// Desde que una carga revertida deja libre su `natural_key`, el archivo corregido se puede
	// volver a subir a la misma cuenta. Si después alguien deshace aquella reversa, la MISMA plata
	// queda incluida dos veces: la fila vieja vuelve a `incluido = true` y la nueva ya estaba. Por
	// eso deshacer se bloquea cuando alguna de sus líneas ya volvió a entrar, y el mensaje dice
	// cuántas y de qué carga nueva son, para poder ir a revertir ESA si era la equivocada.
	//
	// Solo pesa al DESHACER: una carga que no está revertida no tiene filas marcadas y este número
	// es cero (ver DelDeshacer).
	LineasReimportadas int `json:"lineas_reimportadas"`
	// CargasQueLasReimportaron nombra los archivos de las cargas nuevas que ocupan esas huellas.
	CargasQueLasReimportaron []string `json:"cargas_que_las_reimportaron"`
}

// Hay indica si algo impide revertir.
func (b BloqueosReversa) Hay() bool {
	return b.CobrosCxC > 0 || b.PlanillasCxC > 0 || b.AvisosSinResolver > 0 ||
		b.Responsabilidades > 0 || b.FacturasCxP > 0 || b.TrasladosEmparejados > 0 ||
		b.LineasReimportadas > 0 ||
		len(b.PeriodosCerrados) > 0 || len(b.ActasFirmadas) > 0
}

// Detalle redacta qué está en el medio y cuántos. El texto va a pantalla tal cual: tiene que decir
// qué hay que deshacer primero, no «no se puede».
func (b BloqueosReversa) Detalle() string {
	var partes []string
	if b.CobrosCxC > 0 {
		partes = append(partes, fmt.Sprintf("%d cobro(s) de CxC identificados contra estos movimientos", b.CobrosCxC))
	}
	if b.PlanillasCxC > 0 {
		partes = append(partes, fmt.Sprintf("%d movimiento(s) vinculados a una planilla de CxC", b.PlanillasCxC))
	}
	if b.AvisosSinResolver > 0 {
		partes = append(partes, fmt.Sprintf("%d aviso(s) de segmentación sin resolver", b.AvisosSinResolver))
	}
	if b.Responsabilidades > 0 {
		partes = append(partes, fmt.Sprintf("%d obligación(es) del calendario de CxP que se dan por cumplidas con estos movimientos", b.Responsabilidades))
	}
	if b.FacturasCxP > 0 {
		partes = append(partes, fmt.Sprintf("%d factura(s) de CxP conciliadas con estos movimientos", b.FacturasCxP))
	}
	if b.TrasladosEmparejados > 0 {
		partes = append(partes, fmt.Sprintf("%d movimiento(s) ya emparejados como traslado", b.TrasladosEmparejados))
	}
	if len(b.PeriodosCerrados) > 0 {
		partes = append(partes, "el período "+strings.Join(b.PeriodosCerrados, ", ")+" ya está cerrado")
	}
	if len(b.ActasFirmadas) > 0 {
		partes = append(partes, "el acta de conciliación de "+strings.Join(b.ActasFirmadas, ", ")+" ya está firmada")
	}
	if b.LineasReimportadas > 0 {
		detalle := fmt.Sprintf("%d línea(s) de esta carga ya se volvieron a importar", b.LineasReimportadas)
		if len(b.CargasQueLasReimportaron) > 0 {
			detalle += " en " + strings.Join(b.CargasQueLasReimportaron, ", ")
		}
		partes = append(partes, detalle)
	}
	return strings.Join(partes, "; ")
}

// DelDeshacer se queda con los bloqueos que aplican al DESHACER y descarta los que no.
//
// Quedan dos familias. La del CALENDARIO (mes cerrado, acta firmada): meter plata dentro de un mes
// ya cerrado o con acta firmada lo rompe igual en los dos sentidos. Y la de las LÍNEAS YA
// REIMPORTADAS (mig 0086): si el archivo corregido ya volvió a entrar, deshacer dejaría la misma
// plata contada dos veces.
//
// Se descartan los que miran a otro registro apoyado en el movimiento (un cobro de CxC, una
// planilla, una obligación del calendario de CxP, una factura pagada, un traslado emparejado):
// romperlos es sacar el movimiento de los libros, no devolverlo. Deshacer los RESTAURA, así que
// bloquear por ellos dejaría la reversa sin salida.
func (b BloqueosReversa) DelDeshacer() BloqueosReversa {
	return BloqueosReversa{
		PeriodosCerrados:         b.PeriodosCerrados,
		ActasFirmadas:            b.ActasFirmadas,
		LineasReimportadas:       b.LineasReimportadas,
		CargasQueLasReimportaron: b.CargasQueLasReimportaron,
	}
}

// ReversaBloqueadaError indica que la carga tiene dependencias vivas. Es un error TIPADO y no un
// centinela porque el detalle (qué y cuántos) es la mitad útil del mensaje.
type ReversaBloqueadaError struct {
	Bloqueos BloqueosReversa
	// Accion es el verbo del mensaje. Vacío = «revertir», que es el caso original.
	Accion string
}

func (e *ReversaBloqueadaError) Error() string {
	accion := e.Accion
	if accion == "" {
		accion = "revertir"
	}
	return "bancos: no se puede " + accion + " esta carga porque hay " + e.Bloqueos.Detalle() +
		". Deshacé eso primero y volvé a intentar."
}

// FiltrosImportaciones son los filtros del listado de cargas.
type FiltrosImportaciones struct {
	// CuentaBancariaID: vacío = todas las cuentas de la empresa.
	CuentaBancariaID string
	Page             int
	PageSize         int
}

// ImportacionItem es una carga en el listado, con TODO lo que hace falta para decidir si se
// revierte sin abrir otra pantalla.
type ImportacionItem struct {
	ID               string `json:"id"`
	CuentaBancariaID string `json:"cuenta_bancaria_id"`
	Banco            string `json:"banco"`
	CuentaAlias      string `json:"cuenta_alias"`
	// Moneda de la CUENTA: los totales de abajo están en ella (una carga es de una sola cuenta).
	Moneda        string `json:"moneda"`
	NombreArchivo string `json:"nombre_archivo"`
	Estado        string `json:"estado"`
	CreadoPor     string `json:"creado_por"`
	// CreadoPorNombre puede venir vacío: `importacion.creado_por` es nulable.
	CreadoPorNombre string `json:"creado_por_nombre"`
	CreadoEn        string `json:"creado_en"`
	// Movimientos es cuántas filas trajo la carga. Cero también se lista: una importación que no
	// trajo nada es justo la que confunde («¿la subí o no?»).
	Movimientos int `json:"movimientos"`
	// Excluidos son las que hoy no suman (por esta reversa o por cualquier otra corrección).
	Excluidos int `json:"excluidos"`
	// Clasificados son las que ya tienen partida (AUTO o REVISADO): mide el trabajo encima.
	Clasificados  int    `json:"clasificados"`
	TotalDebitos  string `json:"total_debitos"`
	TotalCreditos string `json:"total_creditos"`
	// FechaDesde/FechaHasta son el rango de fechas de sus movimientos («» si no trajo ninguno).
	FechaDesde string `json:"fecha_desde"`
	FechaHasta string `json:"fecha_hasta"`

	Revertida          bool   `json:"revertida"`
	RevertidaEn        string `json:"revertida_en"`
	RevertidaPor       string `json:"revertida_por"`
	RevertidaPorNombre string `json:"revertida_por_nombre"`
	MotivoReversa      string `json:"motivo_reversa"`

	Bloqueos BloqueosReversa `json:"bloqueos"`
	// PuedeRevertir / RazonNoRevertir: el botón y su porqué, calculados acá y no en la pantalla,
	// para que el servidor y el cliente no puedan discrepar sobre quién puede qué.
	PuedeRevertir        bool   `json:"puede_revertir"`
	PuedeDeshacerReversa bool   `json:"puede_deshacer_reversa"`
	RazonNoRevertir      string `json:"razon_no_revertir"`
}

// ListaImportaciones es la página del listado, con el total REAL (no el de la página).
type ListaImportaciones struct {
	Items    []ImportacionItem `json:"items"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// EstadoDeReversa es lo que el servicio necesita saber ANTES de decidir.
type EstadoDeReversa struct {
	Estado   string
	Moneda   string
	Bloqueos BloqueosReversa
}

// CambioDeReversa es lo que la transacción efectivamente movió.
type CambioDeReversa struct {
	// Movimientos es cuántas filas cambiaron de `incluido`. ESE número es la prueba de cuánta
	// plata entró o salió de los libros, junto con las dos sumas.
	Movimientos int
	Debitos     decimal.Decimal
	Creditos    decimal.Decimal
	// Estado resultante de la carga.
	Estado string
	// RevertidaEn: la marca de tiempo que quedó en la fila (vacío al deshacer).
	RevertidaEn string
}

// ResultadoReversa es la respuesta de revertir.
type ResultadoReversa struct {
	ImportacionID string `json:"importacion_id"`
	Estado        string `json:"estado"`
	Excluidos     int    `json:"excluidos"`
	TotalDebitos  string `json:"total_debitos"`
	TotalCreditos string `json:"total_creditos"`
	Moneda        string `json:"moneda"`
	Motivo        string `json:"motivo"`
	RevertidaEn   string `json:"revertida_en"`
}

// ResultadoDeshacerReversa es la respuesta de deshacer la reversa.
type ResultadoDeshacerReversa struct {
	ImportacionID string `json:"importacion_id"`
	Estado        string `json:"estado"`
	Reincluidos   int    `json:"reincluidos"`
	TotalDebitos  string `json:"total_debitos"`
	TotalCreditos string `json:"total_creditos"`
	Moneda        string `json:"moneda"`
}

// normalizarPaginaImportaciones aplica la regla del proyecto para un paginado fuera de rango:
// normalizar, no rechazar con 400 (ver paginaMaxima en repository_clasif.go).
func normalizarPaginaImportaciones(page, pageSize int) (int, int) {
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	if page <= 0 || page > paginaMaxima {
		page = 1
	}
	return page, pageSize
}

// Importaciones lista las cargas de la empresa, la más reciente primero.
func (s *Service) Importaciones(ctx context.Context, empresaID string, f FiltrosImportaciones) (ListaImportaciones, error) {
	f.Page, f.PageSize = normalizarPaginaImportaciones(f.Page, f.PageSize)
	lista, err := s.repo.ListarImportaciones(ctx, empresaID, f)
	if err != nil {
		return ListaImportaciones{}, err
	}
	if lista.Items == nil {
		lista.Items = []ImportacionItem{}
	}
	for i := range lista.Items {
		decidirReversa(&lista.Items[i])
	}
	return lista, nil
}

// decidirReversa resuelve el botón de cada fila. Es la MISMA regla que aplica RevertirImportacion:
// si acá dijera que sí y allá que no, el usuario apretaría un botón habilitado para recibir un 422.
func decidirReversa(it *ImportacionItem) {
	it.Revertida = it.Estado == EstadoImportacionRevertida
	it.PuedeDeshacerReversa = it.Revertida
	switch {
	case it.Revertida:
		it.PuedeRevertir = false
		it.RazonNoRevertir = "esta carga ya está revertida"
		// En una fila ya revertida el único botón vivo es «deshacer», así que la razón que se
		// muestra tiene que ser la SUYA: si el mes se cerró, se firmó el acta o las líneas ya se
		// volvieron a importar después de la reversa, deshacer está bloqueado y el usuario tiene
		// que leer por qué acá, no descubrirlo con un 422.
		if b := it.Bloqueos.DelDeshacer(); b.Hay() {
			it.PuedeDeshacerReversa = false
			it.RazonNoRevertir = "no se puede deshacer la reversa porque hay " + b.Detalle()
		}
	case it.Bloqueos.Hay():
		it.PuedeRevertir = false
		it.RazonNoRevertir = it.Bloqueos.Detalle()
	default:
		it.PuedeRevertir = true
		it.RazonNoRevertir = ""
	}
}

// RevertirImportacion saca de los libros TODOS los movimientos de una carga.
//
// El motivo es obligatorio y los bloqueos rechazan con 422: es lo conservador. Excluir un
// movimiento que sostiene una planilla de cobro o un acta firmada rompería lo que está bien.
func (s *Service) RevertirImportacion(ctx context.Context, empresaID, importacionID, motivo, usuarioID string) (ResultadoReversa, error) {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return ResultadoReversa{}, ErrMotivoReversaRequerido
	}
	est, err := s.repo.EstadoDeReversa(ctx, empresaID, importacionID)
	if err != nil {
		return ResultadoReversa{}, err
	}
	if est.Estado == EstadoImportacionRevertida {
		return ResultadoReversa{}, ErrImportacionYaRevertida
	}
	if est.Bloqueos.Hay() {
		return ResultadoReversa{}, &ReversaBloqueadaError{Bloqueos: est.Bloqueos}
	}

	cambio, err := s.repo.RevertirImportacion(ctx, empresaID, importacionID, usuarioID, motivo)
	if err != nil {
		return ResultadoReversa{}, err
	}

	// El evento lleva las dos sumas además del conteo: ese es el registro de cuánta plata salió de
	// los libros y el único lugar donde sobrevive si mañana se deshace la reversa.
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "importacion", EntidadID: &importacionID,
		Accion: "REVERTIR_IMPORTACION", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"excluidos":      cambio.Movimientos,
			"total_debitos":  cambio.Debitos.String(),
			"total_creditos": cambio.Creditos.String(),
			"moneda":         est.Moneda,
			"motivo":         motivo,
		},
	})

	return ResultadoReversa{
		ImportacionID: importacionID,
		Estado:        cambio.Estado,
		Excluidos:     cambio.Movimientos,
		TotalDebitos:  cambio.Debitos.String(),
		TotalCreditos: cambio.Creditos.String(),
		Moneda:        est.Moneda,
		Motivo:        motivo,
		RevertidaEn:   cambio.RevertidaEn,
	}, nil
}

// DeshacerReversaImportacion vuelve a poner en los libros lo que ESTA reversa excluyó.
//
// Existe porque revertir la carga equivocada por error es exactamente el mismo dedazo que la
// originó, y sin esto la única salida sería volver a importar el archivo.
func (s *Service) DeshacerReversaImportacion(ctx context.Context, empresaID, importacionID, motivo, usuarioID string) (ResultadoDeshacerReversa, error) {
	est, err := s.repo.EstadoDeReversa(ctx, empresaID, importacionID)
	if err != nil {
		return ResultadoDeshacerReversa{}, err
	}
	if est.Estado != EstadoImportacionRevertida {
		return ResultadoDeshacerReversa{}, ErrImportacionNoRevertida
	}
	// Deshacer también mueve plata: mete de vuelta a los libros lo que la reversa había sacado. Si
	// entremedio se cerró el mes o se firmó el acta de esa cuenta —que se pudo hacer JUSTAMENTE
	// porque esos movimientos estaban afuera—, devolverlos rompe el cierre y el acta firmada. Es el
	// mismo daño que el 422 de revertir evita en el otro sentido. Y desde la mig 0086 se suma el
	// caso nuevo: si el archivo corregido ya volvió a entrar, deshacer duplicaría esa plata.
	if bloqueos := est.Bloqueos.DelDeshacer(); bloqueos.Hay() {
		return ResultadoDeshacerReversa{}, &ReversaBloqueadaError{Bloqueos: bloqueos, Accion: "deshacer la reversa de"}
	}

	cambio, err := s.repo.DeshacerReversaImportacion(ctx, empresaID, importacionID)
	if err != nil {
		return ResultadoDeshacerReversa{}, err
	}

	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "importacion", EntidadID: &importacionID,
		Accion: "DESHACER_REVERSA_IMPORTACION", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"reincluidos":    cambio.Movimientos,
			"total_debitos":  cambio.Debitos.String(),
			"total_creditos": cambio.Creditos.String(),
			"moneda":         est.Moneda,
			"motivo":         strings.TrimSpace(motivo),
		},
	})

	return ResultadoDeshacerReversa{
		ImportacionID: importacionID,
		Estado:        cambio.Estado,
		Reincluidos:   cambio.Movimientos,
		TotalDebitos:  cambio.Debitos.String(),
		TotalCreditos: cambio.Creditos.String(),
		Moneda:        est.Moneda,
	}, nil
}
