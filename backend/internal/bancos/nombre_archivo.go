package bancos

// Cómo se llaman los archivos que se descargan.
//
// Nomenclatura del usuario (2026-08-17): la sigla de la empresa + la fecha, «VDP 17082026». Es la
// que ya usa para archivar, así que los reportes salen listos para guardar sin renombrarlos.

import (
	"context"
	"strings"
	"time"
	"unicode"
)

// siglasEmpresa son las que usa el grupo para nombrar sus archivos. La clave se compara en
// minúsculas y por CONTENIDO, para que aguante «Valle de Paz S.A.» o «Valle De Paz».
var siglasEmpresa = []struct{ contiene, sigla string }{
	{"valle", "VDP"},
	{"coopeprofa", "CPF"},
	{"memorial", "MPTS"},
}

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

// NombreArchivoReporte arma «VDP 17082026.xlsx», o «VDP 17082026 corrido.xlsx» con detalle.
//
// El detalle existe para que dos descargas del mismo día no se pisen en la carpeta: el listado
// corrido, el agrupado y el cuadre son archivos distintos y hay que poder distinguirlos.
func NombreArchivoReporte(empresa string, en time.Time, detalle string) string {
	nombre := SiglaEmpresa(empresa) + " " + en.Format("02012006")
	if d := strings.TrimSpace(detalle); d != "" {
		nombre += " " + d
	}
	return nombre + ".xlsx"
}

// NombreArchivo resuelve el nombre de descarga de un reporte de esta empresa.
//
// Si no se pudiera leer el nombre de la empresa, el archivo sale con una sigla genérica en lugar
// de fallar: quedarse sin la descarga por no poder armar el nombre sería peor que el nombre feo.
func (s *Service) NombreArchivo(ctx context.Context, empresaID, usuarioID, detalle string) string {
	empresa, _, _, err := s.repo.EncabezadoReporte(ctx, empresaID, usuarioID)
	if err != nil || strings.TrimSpace(empresa) == "" {
		return NombreArchivoReporte("EMPRESA", AhoraCR(), detalle)
	}
	return NombreArchivoReporte(empresa, AhoraCR(), detalle)
}
