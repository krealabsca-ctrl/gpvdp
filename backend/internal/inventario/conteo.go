package inventario

// El conteo cíclico.
//
// ── Por qué cíclico y no un inventario general ───────────────────────────────
//
// Parar la operación para contar todo una vez al año produce un número exacto un día del año y
// ninguno los otros 364. La alternativa que usa la industria es contar por partes, y **lo caro más
// seguido que lo barato**: los artículos que concentran el capital se cuentan varias veces al año,
// los de la cola larga una vez.
//
// La clase (A/B/C) se DERIVA del capital, no se configura: pedirle al usuario que clasifique 40
// artículos a mano es pedirle que mantenga una tabla que el sistema puede calcular, y que va a
// quedar desactualizada el primer mes.

import (
	"errors"
	"fmt"
)

// Estados de una hoja de conteo.
const (
	ConteoAbierto = "ABIERTO"
	ConteoCerrado = "CERRADO"
	ConteoAnulado = "ANULADO"
)

// Clases del conteo cíclico y cada cuántos días toca contarlas.
//
// Los cortes (80 % / 95 % del capital acumulado) y las frecuencias son los estándar de la práctica
// de inventarios, ajustados a la escala de este negocio: con 150 unidades, contar la clase A cada
// mes es media hora de trabajo, no un operativo.
const (
	ClaseA = "A"
	ClaseB = "B"
	ClaseC = "C"

	diasClaseA = 30
	diasClaseB = 90
	diasClaseC = 365

	// corteA y corteB son el porcentaje ACUMULADO de capital que define cada clase.
	corteA = 80.0
	corteB = 95.0
)

// DiasEntreConteos es cada cuánto toca contar una clase.
func DiasEntreConteos(clase string) int {
	switch clase {
	case ClaseA:
		return diasClaseA
	case ClaseB:
		return diasClaseB
	default:
		return diasClaseC
	}
}

// EtiquetaClase explica la clase en palabras, porque «A» no le dice nada a nadie.
func EtiquetaClase(clase string) string {
	switch clase {
	case ClaseA:
		return "concentra el capital · contar cada mes"
	case ClaseB:
		return "valor medio · contar cada trimestre"
	default:
		return "cola larga · contar una vez al año"
	}
}

// Errores propios del conteo.
var (
	// ErrConteoNoEncontrado indica que la hoja no existe o no es de la empresa.
	ErrConteoNoEncontrado = errors.New("inventario: conteo no encontrado")
	// ErrConteoYaCerrado indica que se quiso tocar una hoja cerrada.
	ErrConteoYaCerrado = errors.New("inventario: ese conteo ya está cerrado")
	// ErrConteoAbiertoEnLaSede indica que ya hay una hoja abierta en esa sede.
	ErrConteoAbiertoEnLaSede = errors.New("inventario: ya hay un conteo abierto en esa sede; hay que cerrarlo o anularlo antes de abrir otro")
	// ErrConteoSinLineas indica que no había nada que contar.
	ErrConteoSinLineas = errors.New("inventario: no hay existencias que contar con ese filtro")
	// ErrLineaNoEncontrada indica que la línea no pertenece a la hoja.
	ErrLineaNoEncontrada = errors.New("inventario: esa línea no es de este conteo")
	// ErrMotivoAnularRequerido: una hoja anulada sin explicación deja la duda de si se contó y no
	// cuadró, o si nunca se contó.
	ErrMotivoAnularRequerido = errors.New("inventario: anular un conteo necesita motivo")
)

// DiferenciasSinExplicarError impide cerrar una hoja con diferencias mudas.
//
// No es burocracia: una diferencia sin motivo es exactamente el ajuste que después nadie puede
// auditar. El mensaje nombra los artículos, porque «hay diferencias sin explicar» obliga a ir a
// buscarlas.
type DiferenciasSinExplicarError struct {
	Cuantas   int
	Articulos []string
}

func (e *DiferenciasSinExplicarError) Error() string {
	lista := ""
	for i, a := range e.Articulos {
		if i == 3 {
			lista += fmt.Sprintf(" y %d más", len(e.Articulos)-3)
			break
		}
		if i > 0 {
			lista += ", "
		}
		lista += a
	}
	return fmt.Sprintf("no se puede cerrar: %d diferencia(s) sin explicar (%s). Cada una necesita un motivo: se rompió, se usó sin registrar, o se contó mal",
		e.Cuantas, lista)
}

// SinContarError impide cerrar una hoja a medio contar.
//
// Cerrar con líneas sin contar daría por bueno lo que el sistema dice de artículos que nadie miró, y
// eso es peor que no haber contado: queda registrado como verificado.
type SinContarError struct {
	Cuantas int
	Total   int
}

func (e *SinContarError) Error() string {
	return fmt.Sprintf("no se puede cerrar: quedan %d de %d líneas sin contar. Dejarlas afuera daría por verificado lo que nadie miró",
		e.Cuantas, e.Total)
}

// ── Tipos que viajan al cliente ─────────────────────────────────────────────

// Conteo es una hoja de conteo.
type Conteo struct {
	ID          string `json:"id"`
	Numero      string `json:"numero"`
	SedeID      string `json:"sede_id"`
	Sede        string `json:"sede"`
	CategoriaID string `json:"categoria_id"`
	Categoria   string `json:"categoria"`
	Estado      string `json:"estado"`
	AbiertoEn   string `json:"abierto_en"`
	CerradoEn   string `json:"cerrado_en"`
	AbiertoPor  string `json:"abierto_por"`
	CerradoPor  string `json:"cerrado_por"`
	Nota        string `json:"nota"`
	// El avance y los hallazgos, resumidos: es lo que decide si se puede cerrar.
	Lineas        int `json:"lineas"`
	Contadas      int `json:"contadas"`
	ConDiferencia int `json:"con_diferencia"`
	SinExplicar   int `json:"sin_explicar"`
	// PuedeCerrarse dice si el cierre va a pasar, para no ofrecer un botón que va a fallar.
	PuedeCerrarse bool          `json:"puede_cerrarse"`
	Filas         []ConteoLinea `json:"filas"`
}

// ConteoLinea es un artículo (o una unidad) de la hoja.
type ConteoLinea struct {
	ID          string `json:"id"`
	ArticuloID  string `json:"articulo_id"`
	Codigo      string `json:"codigo"`
	Articulo    string `json:"articulo"`
	Categoria   string `json:"categoria"`
	ModoControl string `json:"modo_control"`
	// UnidadNumero: en modo UNIDAD, qué ficha se busca.
	UnidadID     string `json:"unidad_id"`
	UnidadNumero string `json:"unidad_numero"`
	// CantidadSistema es la foto congelada al abrir la hoja.
	CantidadSistema int `json:"cantidad_sistema"`
	// CantidadContada nula = sin contar. El cliente recibe -1 para poder distinguirlo de un cero.
	CantidadContada int    `json:"cantidad_contada"`
	Diferencia      int    `json:"diferencia"`
	Motivo          string `json:"motivo"`
	// Estado: SIN_CONTAR | CUADRA | EXPLICADA | SIN_EXPLICAR.
	Estado           string `json:"estado"`
	CostoUnitarioCRC string `json:"costo_unitario_crc"`
	// ImpactoCRC es cuánto vale la diferencia: es lo que vuelve accionable un faltante.
	ImpactoCRC string `json:"impacto_crc"`
}

// Estados de una línea de conteo.
const (
	LineaSinContar   = "SIN_CONTAR"
	LineaCuadra      = "CUADRA"
	LineaExplicada   = "EXPLICADA"
	LineaSinExplicar = "SIN_EXPLICAR"
)

// estadoDeLinea resuelve el estado de una línea. En un solo lugar, para que el resumen de la hoja y
// cada fila no puedan contradecirse.
func estadoDeLinea(contada *int, sistema int, motivo string) string {
	if contada == nil {
		return LineaSinContar
	}
	if *contada == sistema {
		return LineaCuadra
	}
	if motivo == "" {
		return LineaSinExplicar
	}
	return LineaExplicada
}

// PlanConteo es una línea del plan: qué toca contar y con qué urgencia.
type PlanConteo struct {
	SedeID     string `json:"sede_id"`
	Sede       string `json:"sede"`
	Clase      string `json:"clase"`
	ClaseTexto string `json:"clase_texto"`
	Articulos  int    `json:"articulos"`
	CapitalCRC string `json:"capital_crc"`
	// UltimoConteo vacío = nunca se contó.
	UltimoConteo    string `json:"ultimo_conteo"`
	DiasDesde       int    `json:"dias_desde"`
	CadaCuantosDias int    `json:"cada_cuantos_dias"`
	// Estado: NUNCA_CONTADO | AL_DIA | POR_VENCER | ATRASADO.
	Estado string `json:"estado"`
}

// Estados del plan de conteo.
const (
	PlanNuncaContado = "NUNCA_CONTADO"
	PlanAlDia        = "AL_DIA"
	PlanPorVencer    = "POR_VENCER"
	PlanAtrasado     = "ATRASADO"
)

// estadoDelPlan traduce «hace cuánto se contó» a una urgencia.
//
// «Por vencer» empieza al 80 % del plazo: avisar el día que se vence no deja tiempo de organizar a
// nadie para ir a la bodega.
func estadoDelPlan(nuncaContado bool, diasDesde, cada int) string {
	if nuncaContado {
		return PlanNuncaContado
	}
	switch {
	case diasDesde > cada:
		return PlanAtrasado
	case float64(diasDesde) >= float64(cada)*0.8:
		return PlanPorVencer
	default:
		return PlanAlDia
	}
}
