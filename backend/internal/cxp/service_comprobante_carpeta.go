package cxp

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// ErrCarpetaVacia: no llegó ningún archivo ni ningún nombre que mirar.
var ErrCarpetaVacia = errors.New("cxp: no llegó ningún archivo")

// ErrNadaQueAdjuntar: llegaron archivos pero ninguno se puede repartir. Es 422 y no 400: la
// petición está bien formada, lo que no sirve es la carpeta, y el detalle de por qué viaja en la
// previsualización.
var ErrNadaQueAdjuntar = errors.New("cxp: ninguno de los archivos se pudo asociar a un pago")

// PreviewCarpeta es lo que contesta la previsualización.
type PreviewCarpeta struct {
	Resumen  ResumenCarpeta   `json:"resumen"`
	Archivos []ArchivoCruzado `json:"archivos"`
}

// PrevisualizarCarpeta cruza una lista de NOMBRES contra la cartera, sin subir los archivos.
//
// Solo los nombres, a propósito: el código viaja en el nombre, así que para decidir no hace falta
// el contenido. Una corrida de cien recibos son unos pocos kilobytes de texto en vez de treinta
// megas, y así el operador puede corregir cómo bautizó los archivos ANTES de esperar la subida. Los
// archivos se mandan recién al confirmar, y solo los que sirven.
func (s *Service) PrevisualizarCarpeta(ctx context.Context, empresaID string, nombres []string) (PreviewCarpeta, error) {
	if len(nombres) == 0 {
		return PreviewCarpeta{}, ErrCarpetaVacia
	}
	cods := codigosDeLaCarpeta(nombres)
	idx, err := s.repo.DestinosPorHuella(ctx, empresaID, cods)
	if err != nil {
		return PreviewCarpeta{}, err
	}
	as := CruzarComprobantes(nombres, idx)
	return PreviewCarpeta{Resumen: ResumirCarpeta(as), Archivos: as}, nil
}

// AdjuntarCarpeta reparte los archivos de una corrida entre sus facturas.
//
// VUELVE A CRUZAR, no confía en lo que la pantalla decidió: el cliente manda archivos, el servidor
// decide a qué factura va cada uno. Si el reparto viniera en la petición, bastaría con cambiarlo
// para pegarle a una factura cualquiera el recibo de otra —y la Bandeja mostraría un pago
// documentado que nadie pagó—. Es el mismo principio que el alcance: lo pone el servidor.
func (s *Service) AdjuntarCarpeta(ctx context.Context, empresaID string, archivos []ArchivoSubido, usuarioID string) (ResultadoCarpeta, PreviewCarpeta, error) {
	if len(archivos) == 0 {
		return ResultadoCarpeta{}, PreviewCarpeta{}, ErrCarpetaVacia
	}
	nombres := make([]string, 0, len(archivos))
	for _, a := range archivos {
		nombres = append(nombres, a.Nombre)
	}
	prev, err := s.PrevisualizarCarpeta(ctx, empresaID, nombres)
	if err != nil {
		return ResultadoCarpeta{}, PreviewCarpeta{}, err
	}

	// `prev.Archivos` sale de `CruzarComprobantes(nombres, …)` y conserva el orden de `nombres`,
	// que se armó recorriendo `archivos`. Los índices se corresponden uno a uno; la comprobación
	// está igual porque de esa correspondencia depende que cada contenido vaya a SU factura, y una
	// equivocación ahí no se ve: pega el PDF correcto a la factura equivocada.
	if len(prev.Archivos) != len(archivos) {
		return ResultadoCarpeta{}, prev, fmt.Errorf("cxp: el cruce devolvió %d veredictos para %d archivos",
			len(prev.Archivos), len(archivos))
	}
	items := make([]ItemCarpeta, 0, len(archivos))
	for i, a := range prev.Archivos {
		if !a.SeAdjunta() {
			continue
		}
		if a.Nombre != archivos[i].Nombre {
			return ResultadoCarpeta{}, prev, fmt.Errorf("cxp: el veredicto %d es de %q y el archivo es %q",
				i, a.Nombre, archivos[i].Nombre)
		}
		docs := make([]string, 0, len(a.Destinos))
		for _, d := range a.Destinos {
			docs = append(docs, d.DocumentoID)
		}
		items = append(items, ItemCarpeta{
			Nombre:      archivos[i].Nombre,
			Mime:        archivos[i].Mime,
			Contenido:   archivos[i].Contenido,
			DocumentoID: docs,
		})
	}
	if len(items) == 0 {
		return ResultadoCarpeta{}, prev, ErrNadaQueAdjuntar
	}
	res, err := s.repo.GuardarComprobantesCarpeta(ctx, empresaID, items, usuarioID)
	if err != nil {
		return ResultadoCarpeta{}, prev, err
	}
	s.log.Info("cxp: comprobantes adjuntados en bloque",
		zap.Int("archivos", res.Archivos), zap.Int("facturas", res.Facturas),
		zap.Int("rechazados", len(res.Rechazados)))
	return res, prev, nil
}

// PagosConRecibo contesta el control inverso: de lo que ya se pagó, qué tiene recibo y qué no.
func (s *Service) PagosConRecibo(ctx context.Context, empresaID, loteID string, soloFaltantes bool) ([]PagoConRecibo, error) {
	return s.repo.PagosSinComprobante(ctx, empresaID, loteID, soloFaltantes)
}

// ArchivoSubido es un archivo tal como llegó por multipart.
type ArchivoSubido struct {
	Nombre    string
	Mime      string
	Contenido []byte
}

// codigosDeLaCarpeta junta los códigos de todos los nombres, sin repetir, para preguntarlos de un
// solo viaje a la base en vez de uno por archivo.
func codigosDeLaCarpeta(nombres []string) []string {
	vistos := make(map[string]bool, len(nombres))
	out := make([]string, 0, len(nombres))
	for _, n := range nombres {
		for _, c := range CodigosDelNombre(n) {
			if !vistos[c] {
				vistos[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}
