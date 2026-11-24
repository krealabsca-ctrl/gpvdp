// Package sistema registra y consulta los errores internos del ERP (mig 0107).
//
// POR QUÉ EXISTE. La referencia de incidente ya se le mostraba al usuario desde el 12-oct
// (`httpx.AbortInterno`), pero quedaba escrita únicamente en el log del contenedor. El usuario
// terminaba con un código que no se podía canjear por nada: para buscarlo había que entrar por
// SSH al servidor, y si el contenedor se reinició, ya no estaba.
//
// LO QUE NO HACE. No manda nada afuera. La traza de un error de este sistema lleva montos,
// cédulas y nombres de proveedores; eso no sale del servidor del cliente.
package sistema

import (
	"context"
	"errors"
	"time"
)

// Incidente es un error interno tal como quedó registrado.
//
// `EmpresaID` y `UsuarioID` son punteros porque un 500 en el login ocurre ANTES de que exista
// cualquiera de los dos, y es justo el error que más importa ver.
type Incidente struct {
	ID         string    `json:"id"`
	Referencia string    `json:"referencia"`
	EmpresaID  *string   `json:"empresa_id"`
	UsuarioID  *string   `json:"usuario_id"`
	Usuario    string    `json:"usuario"`
	Metodo     string    `json:"metodo"`
	Ruta       string    `json:"ruta"`
	Estado     int       `json:"estado"`
	Operacion  string    `json:"operacion"`
	Detalle    string    `json:"detalle"`
	Panico     bool      `json:"panico"`
	Traza      string    `json:"traza"`
	CreadoEn   time.Time `json:"creado_en"`
}

// FiltrosIncidentes acota la consulta.
//
// `Referencia` es LA búsqueda: el usuario dicta seis caracteres por teléfono. Las demás son para
// mirar qué viene pasando.
type FiltrosIncidentes struct {
	Referencia string
	Desde      string
	Hasta      string
	SoloPanico bool
	Limite     int
}

// ResumenIncidentes es lo que se ve de un vistazo, sin abrir la lista.
//
// `Ultimo` existe para contestar la única pregunta que importa al entrar: ¿esto está pasando
// AHORA o es de la semana pasada? Un contador sin fecha no la contesta.
type ResumenIncidentes struct {
	UltimaHora   int        `json:"ultima_hora"`
	UltimoDia    int        `json:"ultimo_dia"`
	UltimaSemana int        `json:"ultima_semana"`
	Panicos      int        `json:"panicos"`
	Ultimo       *time.Time `json:"ultimo"`
}

// limiteMaximo acota lo que una consulta puede traer de una vez.
//
// No es una regla de negocio: es que la pantalla muestra una lista para leer, no un volcado. Con
// una base de errores grande, pedirlos todos cuelga el navegador justo cuando el sistema ya está
// teniendo problemas.
const limiteMaximo = 200

// limiteDefecto es lo que trae la pantalla sin pedir nada.
const limiteDefecto = 50

// ErrReferenciaInvalida: la referencia tiene una forma fija (seis caracteres de httpx).
//
// Se valida para no mandar a la base cualquier texto que alguien pegue en el buscador, y para
// poder decirle a quien busca que lo que escribió no es una referencia —en vez de devolverle una
// lista vacía, que se lee como «no existe» cuando en realidad es «eso no es un código».
var ErrReferenciaInvalida = errors.New("sistema: la referencia son 6 caracteres, como los que aparecen en el mensaje de error")

// ErrFechaInvalida: las fechas se validan ANTES de armar la consulta.
//
// Sin esto, «desde=hola» llega hasta Postgres, revienta con 22007 y sale como 500 «error
// interno» — y queda registrado como un incidente del sistema. Es decir: la pantalla que existe
// para ver los errores se llenaba de errores propios cada vez que alguien tecleaba mal una fecha,
// que es exactamente el ruido que vuelve inútil un registro de incidentes.
var ErrFechaInvalida = errors.New("sistema: la fecha va como AAAA-MM-DD")

// Repository es lo que el servicio necesita de la base.
type Repository interface {
	// Registrar escribe el incidente. Best-effort: ver Service.Registrar.
	Registrar(ctx context.Context, in Incidente) error
	// Listar devuelve los incidentes visibles desde `empresaID`: los suyos y los que no tienen
	// empresa. Nunca los de otra.
	Listar(ctx context.Context, empresaID string, f FiltrosIncidentes) ([]Incidente, error)
	// Resumen cuenta lo mismo que Listar ve, por ventanas de tiempo.
	Resumen(ctx context.Context, empresaID string) (ResumenIncidentes, error)
}

// fechaValida acepta vacío (sin filtro) o AAAA-MM-DD.
func fechaValida(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// referenciaValida comprueba la forma: 6 caracteres hex en mayúsculas, o el literal SINREF que
// httpx devuelve cuando ni el generador de aleatorios responde.
func referenciaValida(s string) bool {
	if s == "SINREF" {
		return true
	}
	if len(s) != 6 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}
