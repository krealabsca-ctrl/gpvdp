package sistema

// Las consultas que más tiempo se llevan (mig 0120).
//
// POR QUÉ EXISTE. Cuando alguien dice «el sistema está lento», hasta hoy no había forma de saber
// QUÉ está lento. El diagnóstico de noviembre de 2026 encontró que UNA consulta del tablero de CxP
// se llevaba el 92 % del tiempo de esa pantalla, pero para verlo hubo que entrar a la base y
// encender `pg_stat_statements` a mano: en el servidor del cliente eso no está disponible, así que
// el único camino era adivinar y optimizar a ciegas.
//
// LO QUE NO MUESTRA. Ningún dato de ninguna empresa. PostgreSQL normaliza las consultas antes de
// contarlas —los valores se reemplazan por marcadores $1, $2— así que acá se ve la FORMA de la
// consulta y sus tiempos, nunca el monto, el proveedor ni la cédula que llevaba. Por eso esta
// pantalla, a diferencia de la de incidentes, no filtra por empresa: los tiempos del motor son del
// servidor entero y no se pueden repartir por empresa aunque se quisiera.
//
// ORDENADO POR TIEMPO TOTAL, no por el más lento. Una consulta de 900 ms que corre una vez al día
// importa menos que una de 20 ms que corre cuarenta veces por pantalla: lo que se siente es la
// suma. Esa fue exactamente la forma del problema de CxP —un cálculo repetido siete veces por
// carga— y ordenar por media lo habría escondido.

import (
	"context"
	"errors"
)

// ConsultaLenta es una consulta con lo que se lleva.
type ConsultaLenta struct {
	// Consulta es el texto NORMALIZADO, recortado para que la pantalla sea legible.
	Consulta string `json:"consulta"`
	Llamadas int64  `json:"llamadas"`
	// MsTotal es el criterio de orden: la suma de todo lo que esta consulta se llevó.
	MsTotal float64 `json:"ms_total"`
	MsMedia float64 `json:"ms_media"`
	MsMax   float64 `json:"ms_max"`
	// PorcentajeDelTotal sitúa a la consulta contra TODO lo que el motor ejecutó. Es lo que
	// distingue «la más lenta de una base tranquila» de «la que hay que arreglar hoy».
	PorcentajeDelTotal float64 `json:"porcentaje_del_total"`
	// FilasPorLlamada delata los escaneos completos: una consulta que devuelve 1 fila pero lee
	// 200.000 no se ve en los tiempos hasta que la tabla crece.
	FilasPorLlamada float64 `json:"filas_por_llamada"`
}

// RendimientoConsultas es lo que ve la pantalla.
//
// `Disponible` en false NO es un error: es el estado normal de un servidor cuyo motor arrancó sin
// la librería. La pantalla tiene que poder decir POR QUÉ no hay datos y cómo encenderlo, en vez de
// mostrar una lista vacía que se lee como «no hay consultas lentas».
type RendimientoConsultas struct {
	Disponible bool            `json:"disponible"`
	Motivo     string          `json:"motivo"`
	Consultas  []ConsultaLenta `json:"consultas"`
	// MidiendoDesde dice desde cuándo se acumulan estos números. Sin esa fecha, un total de
	// 40 segundos no se puede interpretar: no es lo mismo en una hora que en tres semanas.
	MidiendoDesde string `json:"midiendo_desde"`
}

// topConsultasDefecto es cuántas trae la pantalla. Veinte alcanzan: lo que importa siempre está en
// las primeras, y una lista larga de consultas SQL no se lee, se hojea.
const topConsultasDefecto = 20

// topConsultasMaximo acota lo que se puede pedir.
const topConsultasMaximo = 100

// largoConsultaVisible recorta el texto de la consulta.
//
// Las consultas de este sistema pasan de los 2.000 caracteres (el tablero de CxP es un CTE con
// veinte agregados). Mandarlas enteras a la pantalla la vuelve ilegible y pesa; con el principio
// alcanza para reconocer cuál es, que es para lo que se mira.
const largoConsultaVisible = 300

// ErrRendimientoNoDisponible: el motor arrancó sin `pg_stat_statements`.
//
// No se devuelve como error al cliente —la pantalla muestra el motivo y sigue— pero existe para
// que el servicio pueda distinguir «no está instalado» de «falló la consulta», que son dos cosas
// con arreglos distintos.
var ErrRendimientoNoDisponible = errors.New("sistema: el contador de consultas no está activo en este servidor")

// RepositoryRendimiento es lo que el servicio necesita de la base.
//
// Va en una interfaz aparte de `Repository` a propósito: los incidentes se guardan en una tabla de
// este sistema y las consultas lentas salen de una extensión del motor. Mezclarlas obligaría a que
// cualquier doble de pruebas de incidentes tuviera que fingir también el contador de PostgreSQL.
type RepositoryRendimiento interface {
	// ConsultasLentas devuelve el top por tiempo total. Si la extensión no está, devuelve
	// ErrRendimientoNoDisponible.
	ConsultasLentas(ctx context.Context, limite int) ([]ConsultaLenta, string, error)
	// ReiniciarContadores pone los acumulados en cero, para medir una ventana concreta.
	ReiniciarContadores(ctx context.Context) error
}

// acotarTop deja el límite entre 1 y topConsultasMaximo.
func acotarTop(n int) int {
	if n <= 0 {
		return topConsultasDefecto
	}
	if n > topConsultasMaximo {
		return topConsultasMaximo
	}
	return n
}
