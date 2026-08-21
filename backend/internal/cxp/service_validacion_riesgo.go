package cxp

// Validación de área por RIESGO: qué factura necesita que el área confirme la conformidad.
//
// Decisión del negocio (2026-08-13). Antes: «todo se valida salvo lo marcado» — 4.533 facturas
// esperando confirmación humana, de las cuales el 82 % eran de ₡100.000 o menos y sumaban el 8 %
// del dinero. Ahora: nada se valida salvo que dispare un criterio de riesgo (monto, proveedor
// nuevo, desvío contra su propio histórico). Medido: 969 facturas (21,7 %) cubriendo el 88,9 % del
// monto.
//
// Es el mismo principio del three-way match de los ERP grandes: lo que calza contra un compromiso
// previo se paga sin intervención, y solo la excepción llega a una persona. Acá no hay órdenes de
// compra, así que el compromiso se aproxima con el historial del propio proveedor.

import (
	"context"
	"strconv"
	"strings"
)

// ParametroCxP es un umbral configurable de CxP.
type ParametroCxP struct {
	Clave       string `json:"clave"`
	Valor       string `json:"valor"`
	Descripcion string `json:"descripcion"`
}

// EfectoMotivo es cuántas facturas —y cuánto dinero— trajo cada criterio.
type EfectoMotivo struct {
	Motivo   string `json:"motivo"`
	Etiqueta string `json:"etiqueta"`
	Cantidad int    `json:"cantidad"`
	Monto    string `json:"monto"`
}

// EfectoValidacion es a cuánto gasto le pide confirmación la regla vigente, medido sobre las
// facturas ya evaluadas. Es el dato que convierte la pantalla de umbrales en una decisión.
type EfectoValidacion struct {
	Total          int            `json:"total"`
	TotalMonto     string         `json:"total_monto"`
	Requieren      int            `json:"requieren"`
	RequierenMonto string         `json:"requieren_monto"`
	PorMotivo      []EfectoMotivo `json:"por_motivo"`
}

// Claves de los parámetros de validación (las define la migración 0061/0062).
var clavesValidacion = map[string]bool{
	"VALIDACION_UMBRAL_MONTO":        true,
	"VALIDACION_PROVEEDOR_NUEVO_MAX": true,
	"VALIDACION_DESVIO_PCT":          true,
	"VALIDACION_DESVIO_PISO_MONTO":   true,
}

// ParametrosValidacion devuelve los umbrales vigentes.
func (s *Service) ParametrosValidacion(ctx context.Context, empresaID string) ([]ParametroCxP, error) {
	return s.repo.ParametrosValidacion(ctx, empresaID)
}

// EfectoValidacion mide a cuánto gasto le está pidiendo confirmación la regla vigente.
func (s *Service) EfectoValidacion(ctx context.Context, empresaID string) (EfectoValidacion, error) {
	return s.repo.EfectoValidacion(ctx, empresaID)
}

// GuardarParametroValidacion cambia un umbral.
//
// Cambiarlo mueve cuánto gasto se paga sin revisión humana, así que queda en auditoría con el valor
// nuevo. NO recalcula las facturas ya evaluadas: el veredicto de cada una es un hecho del momento
// en que se revisó, y reescribir el pasado borraría por qué esa factura pasó (o no) por el área.
func (s *Service) GuardarParametroValidacion(ctx context.Context, empresaID, clave, valor, usuarioID string) error {
	clave = strings.TrimSpace(strings.ToUpper(clave))
	if !clavesValidacion[clave] {
		return ErrParametroInvalido
	}
	v := strings.TrimSpace(valor)
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n < 0 {
		return ErrParametroInvalido
	}
	filas, err := s.repo.GuardarParametroValidacion(ctx, empresaID, clave, v, usuarioID)
	if err != nil {
		return err
	}
	if filas == 0 {
		return ErrParametroInvalido
	}
	s.auditarEntidad(ctx, empresaID, "cxp_parametro", empresaID, "CAMBIAR_PARAMETRO_VALIDACION", usuarioID)
	return nil
}

// evaluarValidacion calcula y guarda si la factura necesita validación de área.
//
// Se llama al REVISAR, que es cuando Contabilidad ya le puso concepto y departamento: antes de eso
// no hay con qué decidir. El error no interrumpe la revisión —la factura queda con el veredicto en
// NULL y la cola la trata como pendiente de evaluar— porque bloquear la revisión por no poder
// calcular un umbral sería peor que el problema.
func (s *Service) evaluarValidacion(ctx context.Context, empresaID, docID string) {
	motivo, err := s.repo.EvaluarValidacion(ctx, empresaID, docID)
	if err != nil {
		if s.log != nil {
			s.log.Warn("cxp: no se pudo evaluar la validación por riesgo")
		}
		return
	}
	if motivo == "" {
		return // no requiere validación: sigue al circuito de aprobación
	}
	s.auditarDocNota(ctx, empresaID, docID, "REQUIERE_VALIDACION_AREA", "",
		"a validación de área: "+EtiquetaMotivoValidacion(motivo))
}
