package conta

// La lógica del mapeo contable (mig 0110, Fase 2).

import (
	"context"
	"fmt"
)

// Mapeo lista las clasificaciones con su decisión.
func (s *Service) Mapeo(ctx context.Context, empresaID string, soloPendientes, soloConMovimientos bool) ([]FilaDelMapeo, error) {
	return s.repo.FilasDelMapeo(ctx, empresaID, soloPendientes, soloConMovimientos)
}

// Cobertura es el reporte que dice cuánto falta para poder contabilizar.
func (s *Service) Cobertura(ctx context.Context, empresaID string) (Cobertura, error) {
	return s.repo.Cobertura(ctx, empresaID)
}

// Decidir guarda la decisión sobre una clasificación.
func (s *Service) Decidir(ctx context.Context, empresaID string, d DecisionDeMapeo, usuarioID string) error {
	if err := d.Validar(); err != nil {
		return err
	}
	if err := s.repo.GuardarMapeo(ctx, empresaID, d, usuarioID); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, d.ClasificacionID, "MAPEAR", usuarioID, map[string]any{
		"estado": d.Estado, "cuenta_id": d.CuentaID, "motivo": d.Motivo,
	})
	return nil
}

// DecidirVarias guarda varias decisiones de una vez.
//
// EXISTE PORQUE EL TRABAJO VIENE EN FAMILIAS. Las 23 clasificaciones «Caja Chica - X» van todas a
// cuentas de caja chica; las de combustible, todas a combustible. Resolverlas de a una, con 332
// por delante, es lo que hace que el mapeo no se termine nunca.
//
// NO es transaccional a propósito: cada decisión es independiente y vale por sí misma. Si una
// falla —una cuenta de agrupación, un motivo corto—, las demás se guardan igual y se devuelve
// cuáles no entraron. Lo contrario obligaría a rehacer veinte decisiones buenas por una mala.
func (s *Service) DecidirVarias(ctx context.Context, empresaID string, ds []DecisionDeMapeo, usuarioID string) (ResultadoDecisiones, error) {
	res := ResultadoDecisiones{NoSePudo: map[string]string{}}
	if len(ds) > topeDecisiones {
		return res, fmt.Errorf("conta: no se pueden decidir más de %d partidas de una vez", topeDecisiones)
	}
	for _, d := range ds {
		if err := s.Decidir(ctx, empresaID, d, usuarioID); err != nil {
			res.NoSePudo[d.ClasificacionID] = sinPrefijo(err)
			continue
		}
		res.Guardadas++
	}
	return res, nil
}

// topeDecisiones acota el lote. No es una regla de negocio: es que una pantalla no manda mil
// decisiones, y sin tope un pedido armado a mano puede dejar la base ocupada largo rato.
const topeDecisiones = 500

// ResultadoDecisiones dice cuántas entraron y cuáles no.
type ResultadoDecisiones struct {
	Guardadas int `json:"guardadas"`
	// NoSePudo lleva el motivo POR CLASIFICACIÓN. Un conteo de fallos sin el porqué deja a quien
	// mapea probando a ciegas sobre un lote de veinte.
	NoSePudo map[string]string `json:"no_se_pudo"`
}

// Deshacer quita la decisión: la clasificación vuelve a «falta decidir».
func (s *Service) Deshacer(ctx context.Context, empresaID, clasificacionID, usuarioID string) error {
	if err := s.repo.BorrarMapeo(ctx, empresaID, clasificacionID); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, clasificacionID, "DESHACER_MAPEO", usuarioID, nil)
	return nil
}
