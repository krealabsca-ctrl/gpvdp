package cxp

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// Carga del detalle: se sueltan los XML de una carpeta y cada uno busca su factura por la clave.
//
// POR QUÉ POR CARPETA Y NO DE A UNO. Son 4.533 facturas. Adjuntar el XML de a una es el mismo
// trabajo manual que esta función viene a eliminar, y ya hay precedente en el proyecto: la función
// de adjuntar comprobantes de pago uno por uno existía desde 2015 y se usó SIETE veces en toda su
// vida, todas con archivos de prueba. Lo que no se puede hacer en bloque, no se hace.

// ArchivoXML es un archivo tal como lo subió el usuario.
type ArchivoXML struct {
	Nombre string
	Datos  []byte
}

// XMLSinDestino es un archivo que no terminó en ninguna factura, CON el motivo en palabras.
//
// Esta lista es la parte más informativa del resultado, no el descarte: un XML cuya clave no está
// en el sistema es gasto REAL que nunca entró a CxP. Enterrarlo en un contador de «fallidos»
// escondería justamente el hallazgo.
type XMLSinDestino struct {
	Nombre string `json:"nombre"`
	Clave  string `json:"clave,omitempty"`
	Motivo string `json:"motivo"`
}

// FacturaConDetalle es una factura que recibió su detalle.
type FacturaConDetalle struct {
	DocumentoID string `json:"documento_id"`
	Consecutivo string `json:"consecutivo"`
	Proveedor   string `json:"proveedor"`
	Clave       string `json:"clave"`
	Lineas      int    `json:"lineas"`
	// Reemplazo: la factura ya tenía detalle y éste lo sustituye.
	Reemplazo bool `json:"reemplazo"`
}

// ResultadoDetalleCarga es el parte de lo que pasó con la carpeta.
type ResultadoDetalleCarga struct {
	Archivos     int                 `json:"archivos"`
	Comprobantes int                 `json:"comprobantes"`
	Facturas     int                 `json:"facturas"`
	Lineas       int                 `json:"lineas"`
	Reemplazos   int                 `json:"reemplazos"`
	Cargadas     []FacturaConDetalle `json:"cargadas"`
	SinFactura   []XMLSinDestino     `json:"sin_factura"`
	Problemas    []XMLSinDestino     `json:"problemas"`
}

// CargarDetalleXML recorre los archivos, empareja cada comprobante con su factura por la clave y
// guarda las líneas.
//
// NO CREA FACTURAS. Si la clave no está en el sistema, el archivo se reporta y se sigue: dar de
// alta una cuenta por pagar desde acá crearía pasivo a partir de una carpeta que alguien arrastró,
// sin revisión ni aprobación de nadie. Ese camino ya existe y pasa por el importador.
func (s *Service) CargarDetalleXML(ctx context.Context, empresaID string, archivos []ArchivoXML, usuarioID string) (ResultadoDetalleCarga, error) {
	res := ResultadoDetalleCarga{
		Archivos:   len(archivos),
		Cargadas:   []FacturaConDetalle{},
		SinFactura: []XMLSinDestino{},
		Problemas:  []XMLSinDestino{},
	}

	for _, a := range archivos {
		comps, err := leerComprobantes(a.Datos)
		if err != nil || len(comps) == 0 {
			res.Problemas = append(res.Problemas, XMLSinDestino{
				Nombre: a.Nombre,
				Motivo: "No se pudo leer como comprobante electrónico.",
			})
			continue
		}

		// SE PROCESAN TODOS LOS COMPROBANTES DEL ARCHIVO, no solo el primero. Un ZIP del portal de
		// Hacienda trae un mes entero; quedarse con el primero cargaría 1 de 300 y el resultado
		// diría «1 archivo, 1 factura», que suena a éxito.
		for _, c := range comps {
			res.Comprobantes++
			if nombreDeTipo[c.XMLName.Local] == "" {
				res.Problemas = append(res.Problemas, XMLSinDestino{
					Nombre: a.Nombre,
					Motivo: avisoDeRaizDesconocida(c.XMLName.Local),
				})
				continue
			}

			d := DetalleDeXML(c)
			if d.Clave == "" {
				res.Problemas = append(res.Problemas, XMLSinDestino{
					Nombre: a.Nombre,
					Motivo: "El comprobante no trae clave: no hay con qué emparejarlo.",
				})
				continue
			}
			if len(d.Lineas) == 0 {
				res.Problemas = append(res.Problemas, XMLSinDestino{
					Nombre: a.Nombre, Clave: d.Clave,
					Motivo: "El comprobante no trae líneas de detalle.",
				})
				continue
			}

			doc, err := s.repo.DocumentoDeClave(ctx, empresaID, d.Clave)
			if errors.Is(err, ErrDocumentoNoEncontrado) {
				res.SinFactura = append(res.SinFactura, XMLSinDestino{
					Nombre: a.Nombre, Clave: d.Clave,
					Motivo: "No hay ninguna factura con esta clave en el sistema.",
				})
				continue
			}
			if err != nil {
				return ResultadoDetalleCarga{}, err
			}

			if err := s.repo.GuardarDetalle(ctx, empresaID, doc.ID, d.Lineas); err != nil {
				return ResultadoDetalleCarga{}, err
			}

			res.Facturas++
			res.Lineas += len(d.Lineas)
			if doc.Lineas > 0 {
				res.Reemplazos++
			}
			res.Cargadas = append(res.Cargadas, FacturaConDetalle{
				DocumentoID: doc.ID,
				Consecutivo: doc.Consecutivo,
				Proveedor:   doc.Proveedor,
				Clave:       d.Clave,
				Lineas:      len(d.Lineas),
				Reemplazo:   doc.Lineas > 0,
			})
			s.auditarDocNota(ctx, empresaID, doc.ID, "CARGAR_DETALLE", usuarioID,
				fmt.Sprintf("Detalle del XML cargado: %d línea(s)", len(d.Lineas)))
		}
	}

	// Lo cargado se ordena por proveedor para que la lista se pueda leer; lo que NO entró se deja
	// en el orden en que venía, que es el de la carpeta, para poder ir a buscar el archivo.
	sort.SliceStable(res.Cargadas, func(i, j int) bool {
		return res.Cargadas[i].Proveedor < res.Cargadas[j].Proveedor
	})
	return res, nil
}

// LineasDeDocumento devuelve el detalle guardado de una factura.
func (s *Service) LineasDeDocumento(ctx context.Context, empresaID, documentoID string) ([]LineaGuardada, error) {
	return s.repo.LineasDeDocumento(ctx, empresaID, documentoID)
}

// topeRellenoPorCorrida acota cuántas facturas se rellenan por llamada.
//
// No es una limitación técnica: es para que la pantalla conteste rápido y el usuario vea avanzar
// el contador en vez de esperar sin señal frente a mil facturas. Si quedan más, el botón sigue
// ahí diciendo cuántas faltan.
const topeRellenoPorCorrida = 300

// ResultadoRelleno es el parte de una corrida de relleno.
type ResultadoRelleno struct {
	Procesadas int `json:"procesadas"`
	Facturas   int `json:"facturas"`
	Lineas     int `json:"lineas"`
	// Ilegibles: el XML está guardado pero no se pudo leer. Se cuentan aparte porque son un
	// problema distinto —un archivo roto— y no «todavía no se procesó».
	Ilegibles int `json:"ilegibles"`
	// SinLineas: el comprobante se leyó pero no traía detalle (pasa con algunos tiquetes).
	SinLineas int `json:"sin_lineas"`
	// Faltan: cuántas quedan para la próxima corrida. 0 = terminó.
	Faltan int `json:"faltan"`
}

// PendientesDeDetalle dice cuántas facturas que YA entraron por el buzón pueden rellenarse con el
// XML que el sistema ya tiene guardado.
func (s *Service) PendientesDeDetalle(ctx context.Context, empresaID string) (int, error) {
	return s.repo.ContarRecepcionesSinDetalle(ctx, empresaID)
}

// RellenarDetalleDesdeRecepcion extrae el detalle de los XML que ya están guardados en la base.
//
// EL CAMINO REAL DE LAS FACTURAS ES EL BUZÓN, no la carga manual: «esto se alimenta de la
// recepción, ya que el importador no lo usamos» (Director Financiero, 13-oct-2026). Todo lo que
// entró por ahí antes de que se guardara el detalle tiene su `xml_crudo` en la base y la tabla de
// líneas vacía. No hay nada que subir: el archivo ya está acá.
//
// Una factura cuyo XML no se pueda leer NO corta la corrida: se cuenta y se sigue. Frenar todo por
// un comprobante roto dejaría sin detalle a las otras mil.
func (s *Service) RellenarDetalleDesdeRecepcion(ctx context.Context, empresaID string) (ResultadoRelleno, error) {
	pendientes, err := s.repo.RecepcionesSinDetalle(ctx, empresaID, topeRellenoPorCorrida)
	if err != nil {
		return ResultadoRelleno{}, err
	}

	var res ResultadoRelleno
	for _, p := range pendientes {
		res.Procesadas++
		comps, err := leerComprobantes(p.XML)
		if err != nil || len(comps) == 0 {
			res.Ilegibles++
			continue
		}
		d := DetalleDeXML(comps[0])
		if len(d.Lineas) == 0 {
			res.SinLineas++
			continue
		}
		if err := s.repo.GuardarDetalle(ctx, empresaID, p.DocumentoID, d.Lineas); err != nil {
			return ResultadoRelleno{}, err
		}
		res.Facturas++
		res.Lineas += len(d.Lineas)
	}

	// Lo que queda DESPUÉS de esta corrida. Las ilegibles y las sin líneas siguen contando como
	// pendientes —su `detalle_lineas` sigue en 0— y eso es correcto: el botón seguiría ofreciendo
	// reintentarlas. Por eso el parte las nombra aparte, para que se vea que no son «faltan por
	// procesar» sino «no hay nada que sacarles».
	faltan, err := s.repo.ContarRecepcionesSinDetalle(ctx, empresaID)
	if err != nil {
		return ResultadoRelleno{}, err
	}
	res.Faltan = faltan
	return res, nil
}
