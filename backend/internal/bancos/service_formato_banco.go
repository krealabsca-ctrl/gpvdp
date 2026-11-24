package bancos

// La lógica de los formatos de banco configurables (mig 0108).

import (
	"bytes"
	"context"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// auditarFormato deja el evento en `auditoria_evento`.
//
// Propio y no el de `service_segmento.go`: ese se llama `auditarSegmento` y usarlo desde acá
// dejaría los eventos de los formatos de banco colgando de un nombre que habla de otra cosa.
func (s *Service) auditarFormato(ctx context.Context, empresaID, id, accion, usuarioID string, detalle map[string]any) {
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "formato_banco", EntidadID: &id,
		Accion: accion, UsuarioID: &usuarioID, ValorNuevo: detalle,
	})
}

// FormatosDeBanco lista los formatos configurados de la empresa.
func (s *Service) FormatosDeBanco(ctx context.Context, empresaID string, soloActivos bool) ([]FormatoBanco, error) {
	return s.repo.FormatosDeBanco(ctx, empresaID, soloActivos)
}

// GuardarFormatoBanco valida y persiste.
//
// La validación vive en el tipo (`Validar`) y no acá: la pantalla de prueba la necesita antes de
// guardar nada, y tener la regla escrita dos veces es la forma de que una quede atrás.
func (s *Service) GuardarFormatoBanco(ctx context.Context, empresaID string, f FormatoBanco, usuarioID string) (FormatoBanco, error) {
	if err := f.Validar(); err != nil {
		return FormatoBanco{}, err
	}
	guardado, err := s.repo.GuardarFormatoBanco(ctx, empresaID, f, usuarioID)
	if err != nil {
		return FormatoBanco{}, err
	}
	s.auditarFormato(ctx, empresaID, guardado.ID, "GUARDAR_FORMATO", usuarioID, map[string]any{
		"nombre": guardado.Nombre, "activo": guardado.Activo, "formato_fecha": guardado.FormatoFecha,
	})
	return guardado, nil
}

// BorrarFormatoBanco lo elimina.
func (s *Service) BorrarFormatoBanco(ctx context.Context, empresaID, id, usuarioID string) error {
	if err := s.repo.BorrarFormatoBanco(ctx, empresaID, id); err != nil {
		return err
	}
	s.auditarFormato(ctx, empresaID, id, "BORRAR_FORMATO", usuarioID, nil)
	return nil
}

// ProbarFormato corre el formato contra un archivo de verdad, SIN guardar nada.
//
// POR QUÉ EXISTE. Configurar columnas por número es adivinar: la columna 3 de un Excel es la D, y
// quien mira el archivo cuenta desde 1. Un formato con el débito y el crédito cambiados lee todo
// el estado de cuenta al revés y nadie lo nota hasta que el cuadre no cierra, semanas después.
// Esto muestra lo que el sistema ENTENDIÓ antes de que el formato exista.
//
// UN FORMATO A MEDIAS NO ES UN ERROR ACÁ, y esto se descubrió usando la pantalla: el primer paso
// para configurar un banco es subir el archivo PARA PODER VER las columnas, así que en ese
// momento el formato está necesariamente vacío. Si la validación cortara, nadie llegaría nunca a
// tener un formato completo —habría que adivinar los números para poder verlos—.
//
// Por eso lo único que falla acá es un archivo ilegible. Lo demás vuelve en la respuesta: las
// filas siempre, y el motivo en `Problema`.
func (s *Service) ProbarFormato(ctx context.Context, f FormatoBanco, archivo []byte) (PruebaDeFormato, error) {
	g, err := CargarGrid(bytes.NewReader(archivo))
	if err != nil {
		return PruebaDeFormato{}, err
	}
	// Las primeras filas del archivo, tal cual: sin esto, quien configura no tiene contra qué
	// contar las columnas y vuelve a adivinar.
	prueba := PruebaDeFormato{Filas: primerasFilas(g, filasDeMuestra)}

	if err := f.Validar(); err != nil {
		prueba.Problema = sinPrefijoPaquete(err)
		return prueba, nil
	}
	a, err := f.Adaptador()
	if err != nil {
		prueba.Problema = sinPrefijoPaquete(err)
		return prueba, nil
	}

	if !a.Detecta(g) {
		// No es un error: es LA respuesta. Un 500 acá dejaría a quien configura sin saber si el
		// problema es el archivo o lo que acaba de escribir.
		//
		// Y se dice CUÁL señal falla. Las señales se unen con Y, así que una sola que no se cumpla
		// tumba todo el formato; el sistema ya sabía cuál era y antes no lo decía (23-oct-2026).
		prueba.Detectado = false
		prueba.SenalesQueFallan = SenalesQueNoCalzan(f, g)
		return prueba, nil
	}
	prueba.Detectado = true

	res, err := a.Parsea(g)
	if err != nil {
		prueba.Problema = err.Error()
		return prueba, nil
	}
	prueba.Movimientos = len(res.Movimientos)
	prueba.IBAN = res.IBAN
	prueba.Moneda = res.MonedaArchivo
	for i, m := range res.Movimientos {
		if i >= movimientosDeMuestra {
			break
		}
		prueba.Muestra = append(prueba.Muestra, MovimientoDeMuestra{
			Fecha:       m.Fecha.Format("2006-01-02"),
			Documento:   m.Documento,
			Descripcion: m.Descripcion,
			Debito:      m.Debito.String(),
			Credito:     m.Credito.String(),
		})
	}
	return prueba, nil
}

// adaptadoresConfigurados carga los formatos activos de la empresa, listos para detectar.
//
// NUNCA devuelve error: si la consulta falla, el importador sigue con los seis del código. Lo que
// está en juego es que alguien pueda cargar su estado de cuenta, y una tabla de configuración que
// no responde no puede impedírselo. Se deja en el log.
func (s *Service) adaptadoresConfigurados(ctx context.Context, empresaID string) []Adapter {
	formatos, err := s.repo.FormatosDeBanco(ctx, empresaID, true)
	if err != nil {
		if s.log != nil {
			s.log.Error("bancos: no se pudieron leer los formatos configurados; se usan los del código",
				zap.String("empresa", empresaID), zap.Error(err))
		}
		return nil
	}
	adaptadores, rotos := AdaptadoresDe(formatos)
	if rotos > 0 && s.log != nil {
		s.log.Warn("bancos: formatos configurados que no se pudieron construir",
			zap.String("empresa", empresaID), zap.Int("rotos", rotos))
	}
	return adaptadores
}

// filasDeMuestra / movimientosDeMuestra: lo justo para verificar, no un volcado del archivo.
const (
	filasDeMuestra       = 12
	movimientosDeMuestra = 5
)

// PruebaDeFormato es lo que se ve al probar un formato contra un archivo, antes de guardarlo.
type PruebaDeFormato struct {
	// Detectado false con Filas llenas es la respuesta útil: «no lo reconocí, y esto es lo que
	// hay en el archivo para que corrijas las señales».
	Detectado   bool                  `json:"detectado"`
	Movimientos int                   `json:"movimientos"`
	IBAN        string                `json:"iban"`
	Moneda      string                `json:"moneda"`
	Muestra     []MovimientoDeMuestra `json:"muestra"`
	Filas       [][]string            `json:"filas"`
	// Las señales que no se cumplen, cuando no se reconoció el archivo. Vacío si el problema no
	// son las señales (no hay fila de títulos reconocible, o el formato sí detectó).
	SenalesQueFallan []SenalQueNoCalza `json:"senales_que_fallan"`
	// Problema trae el error de parseo en texto cuando el formato se reconoció pero el archivo no
	// se pudo leer (fechas ilegibles, montos rotos). Va como texto y no como error HTTP porque es
	// parte del resultado de la prueba, no una falla de la petición.
	Problema string `json:"problema"`
}

// MovimientoDeMuestra son los montos como texto, igual que viajan en todo el ERP.
type MovimientoDeMuestra struct {
	Fecha       string `json:"fecha"`
	Documento   string `json:"documento"`
	Descripcion string `json:"descripcion"`
	Debito      string `json:"debito"`
	Credito     string `json:"credito"`
}

// primerasFilas devuelve las primeras `n` filas del archivo, sin ninguna nula.
//
// LO DE LA FILA VACÍA NO ES UN DETALLE. Un estado de cuenta real trae filas en blanco entre el
// título y el encabezado, y excelize las devuelve como un slice nil. Un nil se serializa como
// `null` en JSON, y la pantalla —que cuenta el ancho de cada fila para dibujar la grilla— se cae
// entera con «Cannot read properties of null». Se descubrió abriendo la pantalla con un archivo
// de verdad; con los grids de prueba, escritos a mano, nunca apareció.
func primerasFilas(g Grid, n int) [][]string {
	if len(g) < n {
		n = len(g)
	}
	out := make([][]string, 0, n)
	for i := 0; i < n; i++ {
		if g[i] == nil {
			out = append(out, []string{})
			continue
		}
		out = append(out, g[i])
	}
	return out
}
