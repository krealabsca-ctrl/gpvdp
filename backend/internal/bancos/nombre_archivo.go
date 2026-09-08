package bancos

// Cómo se llaman los archivos que se descargan.
//
// Nomenclatura del usuario (2026-09-03), la que ya usa para archivar:
//
//	VDP Asoc + Dep Agosto 03092026.xlsx
//	└┬┘ └────┬────┘ └──┬─┘ └───┬────┘
//	 │       │         │       └── la fecha en que se descarga (ddmmaaaa)
//	 │       │         └────────── el mes de los DATOS que se están bajando
//	 │       └──────────────────── las clasificaciones filtradas, abreviadas
//	 └──────────────────────────── la sigla de la empresa
//
// Sale listo para guardar sin renombrarlo, que es el punto.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// mesesEnEspanol son los nombres que usa el sistema. «Setiembre» y no «Septiembre»: es la forma
// corriente en Costa Rica y la que ya usaba el encabezado de los reportes.
var mesesEnEspanol = [...]string{"", "Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
	"Julio", "Agosto", "Setiembre", "Octubre", "Noviembre", "Diciembre"}

// siglasEmpresa son las que usa el grupo para nombrar sus archivos. La clave se compara en
// minúsculas y por CONTENIDO, para que aguante «Valle de Paz S.A.» o «Valle De Paz».
var siglasEmpresa = []struct{ contiene, sigla string }{
	{"valle", "VDP"},
	{"coopeprofa", "CPF"},
	{"memorial", "MPTS"},
}

// abreviaturasClasificacion son las abreviaturas que el usuario ya escribe a mano.
//
// Es una tabla y no una regla porque no hay regla: «Asociaciones» se abrevia con 4 letras (Asoc),
// «Deposito de Clientes» con 3 (Dep) y «Servicios» con 4 (Serv). Se compara por CONTENIDO sobre el
// nombre normalizado, así que «Asociaciones / Cooperativas» y «Asociaciones» caen en la misma.
//
// Para agregar una, se pone acá arriba. Lo que no esté cae en la regla de respaldo.
var abreviaturasClasificacion = []struct{ contiene, abrev string }{
	{"asociacion", "Asoc"},
	{"deposito", "Dep"},
	{"servicio", "Serv"},
}

// topeClasificacionesEnNombre: hasta cuántas abreviaturas entran antes de resumir con «+N más».
// Windows corta los nombres en 255 caracteres, y con 302 clasificaciones un filtro amplio armaría
// un nombre que el sistema operativo no puede guardar.
const topeClasificacionesEnNombre = 3

// SiglaEmpresa devuelve la sigla de archivo de una empresa.
//
// Para una empresa que no esté en la lista arma la sigla con las iniciales de sus palabras (hasta
// 4 letras), en vez de inventar un nombre: así una empresa nueva sale con algo razonable y
// reconocible, y si el grupo define su sigla oficial se agrega arriba.
func SiglaEmpresa(nombre string) string {
	n := strings.ToLower(nombre)
	for _, s := range siglasEmpresa {
		if strings.Contains(n, s.contiene) {
			return s.sigla
		}
	}
	var iniciales []rune
	for _, palabra := range strings.Fields(nombre) {
		for _, r := range palabra {
			if unicode.IsLetter(r) {
				iniciales = append(iniciales, unicode.ToUpper(r))
				break
			}
		}
		if len(iniciales) == 4 {
			break
		}
	}
	if len(iniciales) == 0 {
		return "EMPRESA"
	}
	return string(iniciales)
}

// AbreviarClasificacion abrevia UNA clasificación para el nombre del archivo.
//
// Regla de respaldo (decisión del usuario): las primeras 4 letras de la primera palabra. Da algo
// corto y reconocible —«Datafonos» → «Data», «Planilla» → «Plan»— sin obligar a mantener a mano
// una tabla de 302 entradas.
func AbreviarClasificacion(nombre string) string {
	n := norm(nombre)
	for _, a := range abreviaturasClasificacion {
		if strings.Contains(n, a.contiene) {
			return a.abrev
		}
	}
	// Primera palabra con letras; se corta a 4 RUNAS (no bytes: «Ñ» y las tildes ocupan dos).
	for _, palabra := range strings.Fields(nombre) {
		letras := []rune{}
		for _, r := range palabra {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				letras = append(letras, r)
			}
		}
		if len(letras) == 0 {
			continue
		}
		if len(letras) > 4 {
			letras = letras[:4]
		}
		return strings.ToUpper(string(letras[:1])) + strings.ToLower(string(letras[1:]))
	}
	return ""
}

// EtiquetaClasificaciones es la parte del nombre que dice QUÉ se está bajando.
//
// Sin filtro es «Completo»: el archivo trae todo, y decirlo evita confundirlo con una descarga
// parcial del mismo día. Con más de `topeClasificacionesEnNombre` se resume, para no armar un nombre
// que el sistema operativo no pueda guardar.
func EtiquetaClasificaciones(nombres []string) string {
	// Se abrevia y se quitan repetidas conservando el orden de aparición: dos clasificaciones
	// distintas pueden compartir abreviatura («Servicios de Emergencias» y «Servicios Funerarios»
	// son las dos «Serv»), y repetirla en el nombre no aporta nada.
	vistas := map[string]bool{}
	abrevs := make([]string, 0, len(nombres))
	for _, n := range nombres {
		a := AbreviarClasificacion(n)
		if a == "" || vistas[a] {
			continue
		}
		vistas[a] = true
		abrevs = append(abrevs, a)
	}
	if len(abrevs) == 0 {
		return "Completo"
	}
	if len(abrevs) <= topeClasificacionesEnNombre {
		return strings.Join(abrevs, " + ")
	}
	resto := len(abrevs) - topeClasificacionesEnNombre
	return strings.Join(abrevs[:topeClasificacionesEnNombre], " + ") + fmt.Sprintf(" +%d más", resto)
}

// MesDelReporte es el mes de los DATOS que se descargan, no el de hoy.
//
// El año se agrega SOLO cuando no es el año en curso. Así el caso normal sale como el usuario lo
// escribe («Agosto») y bajar un mes de otro año no produce dos archivos llamados igual —la fecha de
// descarga que va al final lleva el año de HOY, no el de los datos—.
func MesDelReporte(f FiltrosMovimientos, hoy time.Time) string {
	meses := mesesPedidos(f)
	if len(meses) == 0 {
		return "Historico"
	}
	etiqueta := func(t time.Time) string {
		nombre := mesesEnEspanol[int(t.Month())]
		if t.Year() != hoy.Year() {
			return fmt.Sprintf("%s %d", nombre, t.Year())
		}
		return nombre
	}
	if len(meses) == 1 {
		return etiqueta(meses[0])
	}
	// Varios meses: el primero y el último. «Junio a Agosto» se entiende de una y no crece con el
	// rango, que es lo que importa para un nombre de archivo.
	return etiqueta(meses[0]) + " a " + etiqueta(meses[len(meses)-1])
}

// mesesPedidos resuelve los meses que abarca el filtro, ordenados y sin repetir. Acepta las dos
// formas de pedir un período (periodo/periodos y desde/hasta), igual que el resto del módulo.
func mesesPedidos(f FiltrosMovimientos) []time.Time {
	vistos := map[string]bool{}
	out := []time.Time{}
	agregar := func(t time.Time) {
		clave := t.Format("2006-01")
		if vistos[clave] {
			return
		}
		vistos[clave] = true
		out = append(out, time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC))
	}

	for _, p := range append(append([]string{}, f.Periodos...), f.Periodo) {
		if t, err := time.Parse("2006-01", strings.TrimSpace(p)); err == nil {
			agregar(t)
		}
	}
	if len(out) == 0 {
		// Rango de fechas: se recorren los meses que toca, con un tope por si llegan fechas
		// absurdas (un rango de años no puede colgar la descarga).
		desde, errD := time.Parse("2006-01-02", f.Desde)
		hasta, errH := time.Parse("2006-01-02", f.Hasta)
		switch {
		case errD == nil && errH == nil && !hasta.Before(desde):
			for t := desde; !t.After(hasta) && len(out) < 24; t = t.AddDate(0, 1, 0) {
				agregar(t)
			}
			agregar(hasta) // el último mes, si el salto de a un mes se lo pasó
		case errD == nil:
			agregar(desde)
		case errH == nil:
			agregar(hasta)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// NombreArchivoReporte arma «VDP Asoc + Dep Agosto 03092026.xlsx».
//
// `detalle` queda para los reportes que NO son el detalle de movimientos (el cuadre): dos descargas
// del mismo día con el mismo filtro se pisarían en la carpeta si no se distinguen. El detalle de
// movimientos ya no lleva nada —el usuario pidió sacar la palabra «corrido»—, así que bajar la vista
// agrupada y la corrida el mismo día produce el mismo nombre a propósito: para él son el mismo
// archivo con otra presentación.
func NombreArchivoReporte(empresa, clasificaciones, mes string, en time.Time, detalle string) string {
	partes := []string{SiglaEmpresa(empresa)}
	for _, p := range []string{clasificaciones, mes, strings.TrimSpace(detalle)} {
		if p != "" {
			partes = append(partes, p)
		}
	}
	partes = append(partes, en.Format("02012006"))
	return limpiarNombreArchivo(strings.Join(partes, " ")) + ".xlsx"
}

// limpiarNombreArchivo saca lo que un sistema de archivos no acepta.
//
// Hace falta porque los nombres del catálogo son libres: «Asociaciones / Cooperativas» trae una
// barra, y una barra en el Content-Disposition parte el nombre en dos y la descarga sale con un
// nombre que no es el que se armó.
func limpiarNombreArchivo(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '+' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			// Se descarta en vez de reemplazar por «_»: el nombre se lee, y «Asoc _ Coop» ensucia
			// más que «Asoc Coop».
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// NombreArchivo resuelve el nombre de descarga de un reporte de esta empresa.
//
// Si no se pudiera leer el nombre de la empresa, el archivo sale con una sigla genérica en lugar
// de fallar: quedarse sin la descarga por no poder armar el nombre sería peor que el nombre feo.
func (s *Service) NombreArchivo(ctx context.Context, empresaID, usuarioID, detalle string) string {
	return s.nombreArchivoCon(ctx, empresaID, usuarioID, "", "", detalle)
}

func (s *Service) nombreArchivoCon(ctx context.Context, empresaID, usuarioID, clasificaciones, mes, detalle string) string {
	empresa, _, _, err := s.repo.EncabezadoReporte(ctx, empresaID, usuarioID)
	if err != nil || strings.TrimSpace(empresa) == "" {
		empresa = "EMPRESA"
	}
	return NombreArchivoReporte(empresa, clasificaciones, mes, AhoraCR(), detalle)
}
