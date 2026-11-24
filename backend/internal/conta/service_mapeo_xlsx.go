package conta

// Exportar e importar el mapeo contable como .xlsx. El porqué está en mapeo_xlsx.go.

import (
	"context"
	"fmt"
	"strings"
)

// ResultadoImportacionMapeo es lo que se le muestra a quien subió el archivo.
//
// No es un booleano: con cientos de filas, «no se pudo» sin decir CUÁLES obliga a revisar la hoja
// entera a ojo. Cada rechazo viaja con su número de fila y su motivo.
type ResultadoImportacionMapeo struct {
	Aplicadas int                `json:"aplicadas"`
	Rechazos  []RechazoDeFila    `json:"rechazos"`
	Resumen   ResumenImportacion `json:"resumen"`
}

// RechazoDeFila nombra la fila del Excel, no el id interno: es lo que la persona tiene delante.
type RechazoDeFila struct {
	Fila          int    `json:"fila"`
	Concepto      string `json:"concepto"`
	Clasificacion string `json:"clasificacion"`
	Motivo        string `json:"motivo"`
}

// ResumenImportacion separa lo que se leyó de lo que se aplicó: si el archivo trae 300 filas y se
// aplican 12, el número que importa es la diferencia.
type ResumenImportacion struct {
	FilasLeidas int `json:"filas_leidas"`
	Mapeadas    int `json:"mapeadas"`
	NoContables int `json:"no_contables"`
}

// ExportarMapeoXLSX arma la hoja de trabajo con el plan al lado.
//
// Trae TODAS las partidas con movimientos, decididas o no: el archivo sirve para completar lo que
// falta y también para revisar lo ya hecho, que es como se corrige un mapeo que se hizo rápido.
func (s *Service) ExportarMapeoXLSX(ctx context.Context, empresaID string) ([]byte, error) {
	filas, err := s.repo.FilasDelMapeo(ctx, empresaID, false, true)
	if err != nil {
		return nil, err
	}
	cuentas, err := s.repo.Cuentas(ctx, empresaID, true)
	if err != nil {
		return nil, err
	}
	return ConstruirMapeoXLSX(filas, cuentas)
}

// ImportarMapeoXLSX aplica el archivo.
//
// ── POR QUÉ RESUELVE LOS NOMBRES CONTRA LA BASE Y NO CONFÍA EN EL ARCHIVO ────────────────────
//
// El archivo trae nombres que alguien pudo editar, y códigos de cuenta tecleados. Las dos cosas se
// buscan acá: si el nombre no existe o el código no está en el plan, la fila se rechaza CON SU
// NÚMERO en vez de aplicarse a otra partida parecida. Un mapeo puesto en la partida equivocada no
// da ningún error y sale a la luz meses después, en un asiento que nadie entiende.
//
// ── POR QUÉ NO ES TRANSACCIONAL ──────────────────────────────────────────────────────────────
//
// Mismo criterio que `DecidirVarias`, y por la misma razón: una fila mala no puede tumbar a las
// 200 buenas. Quien sube una hoja de 300 filas con dos errores de tipeo quiere que entren 298 y
// que le digan cuáles dos arreglar, no empezar de nuevo.
func (s *Service) ImportarMapeoXLSX(ctx context.Context, empresaID string, archivo []byte, usuarioID string) (ResultadoImportacionMapeo, error) {
	leidas, err := LeerMapeoDesdeExcel(archivo)
	if err != nil {
		return ResultadoImportacionMapeo{}, err
	}

	// Las partidas de la empresa, indexadas por (concepto, partida) normalizado. Se trae una vez:
	// buscar por fila serían 300 consultas.
	filas, err := s.repo.FilasDelMapeo(ctx, empresaID, false, false)
	if err != nil {
		return ResultadoImportacionMapeo{}, err
	}
	porNombre := make(map[string]string, len(filas))
	for _, f := range filas {
		porNombre[llaveDePartida(f.Concepto, f.Clasificacion)] = f.ClasificacionID
	}

	cuentas, err := s.repo.Cuentas(ctx, empresaID, true)
	if err != nil {
		return ResultadoImportacionMapeo{}, err
	}
	porCodigo := make(map[string]Cuenta, len(cuentas))
	for _, c := range cuentas {
		porCodigo[normalizarCodigo(c.Codigo)] = c
	}

	res := ResultadoImportacionMapeo{Resumen: ResumenImportacion{FilasLeidas: len(leidas)}}
	decisiones := make([]DecisionDeMapeo, 0, len(leidas))
	origen := make([]FilaMapeoLeida, 0, len(leidas))

	for _, l := range leidas {
		clasifID, ok := porNombre[llaveDePartida(l.Concepto, l.Clasificacion)]
		if !ok {
			res.Rechazos = append(res.Rechazos, rechazo(l,
				"no existe una partida con ese concepto y ese nombre en esta empresa"))
			continue
		}
		// La cuenta manda sobre el motivo: si vinieron las dos, lo que se quiso decir es «va a
		// esta cuenta», y el motivo queda como la nota que es.
		if l.CuentaCodigo != "" {
			cuenta, ok := porCodigo[normalizarCodigo(l.CuentaCodigo)]
			if !ok {
				res.Rechazos = append(res.Rechazos, rechazo(l,
					fmt.Sprintf("la cuenta %q no está en el plan de esta empresa", l.CuentaCodigo)))
				continue
			}
			if !cuenta.AceptaMovimiento {
				res.Rechazos = append(res.Rechazos, rechazo(l, fmt.Sprintf(
					"la cuenta %s (%s) agrupa a otras, así que no admite asientos: elegí una de movimiento",
					cuenta.Codigo, cuenta.Nombre)))
				continue
			}
			decisiones = append(decisiones, DecisionDeMapeo{
				ClasificacionID: clasifID, Estado: EstadoMapeada, CuentaID: cuenta.ID,
			})
			origen = append(origen, l)
			continue
		}
		decisiones = append(decisiones, DecisionDeMapeo{
			ClasificacionID: clasifID, Estado: EstadoNoContabilizable, Motivo: l.Motivo,
		})
		origen = append(origen, l)
	}

	if len(decisiones) == 0 {
		return res, nil
	}

	aplicado, err := s.DecidirVarias(ctx, empresaID, decisiones, usuarioID)
	if err != nil {
		return ResultadoImportacionMapeo{}, err
	}
	res.Aplicadas = aplicado.Guardadas

	// Los rechazos del servicio vienen indexados por clasificación; acá se traducen al número de
	// FILA DEL EXCEL, que es lo único que la persona puede ir a corregir. Se recorre `decisiones`
	// y no el mapa para no perder el orden del archivo en el informe.
	for i, d := range decisiones {
		if motivo, fallo := aplicado.NoSePudo[d.ClasificacionID]; fallo {
			res.Rechazos = append(res.Rechazos, rechazo(origen[i], motivo))
			continue
		}
		if d.Estado == EstadoMapeada {
			res.Resumen.Mapeadas++
		} else {
			res.Resumen.NoContables++
		}
	}
	return res, nil
}

func rechazo(l FilaMapeoLeida, motivo string) RechazoDeFila {
	return RechazoDeFila{
		Fila: l.Fila, Concepto: l.Concepto, Clasificacion: l.Clasificacion, Motivo: motivo,
	}
}

// llaveDePartida arma la llave (concepto, partida) comparable.
//
// Es segura porque la base la garantiza única: `concepto_empresa_id_nombre_key` sobre
// (empresa, nombre) y `clasificacion_empresa_id_concepto_id_nombre_key` sobre
// (empresa, concepto_id, nombre). Se normaliza para que un espacio de más o una mayúscula
// distinta no manden la fila al rechazo.
func llaveDePartida(concepto, clasificacion string) string {
	return normalizarTitulo(concepto) + "\x00" + normalizarTitulo(clasificacion)
}

// normalizarCodigo quita lo que no es dígito ni guion: Excel convierte algunos códigos en texto
// con espacios o apóstrofo inicial, y «1-01-02 » no debería fallar contra «1-01-02».
func normalizarCodigo(c string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c), "'"))
}
