package cxp

// Marcado masivo del convenio y liberación de lo ya bloqueado. El porqué está en convenio_masivo.go.

import (
	"context"
	"strconv"
	"strings"
)

// CandidatosConvenio lista los proveedores que declaran contado en sus comprobantes.
func (s *Service) CandidatosConvenio(ctx context.Context, empresaID string, soloPendientes bool) ([]CandidatoConvenio, error) {
	return s.repo.CandidatosConvenio(ctx, empresaID, soloPendientes)
}

// BloqueadasDeProveedores dice cuántas facturas y por cuánto se liberarían.
//
// Va en su propio endpoint y no dentro del marcado: la pantalla lo pregunta ANTES, para poder
// mostrar el monto y que la persona decida con el número a la vista.
func (s *Service) BloqueadasDeProveedores(ctx context.Context, empresaID string, ids []string) (int, string, error) {
	if len(ids) == 0 {
		return 0, "0", nil
	}
	return s.repo.BloqueadasDeProveedores(ctx, empresaID, ids)
}

// MarcarConvenioMasivo pone el convenio a varios proveedores.
//
// NO libera nada: marcar el convenio cambia lo que pasa con las facturas que ENTREN de ahora en
// adelante, y lo ya bloqueado es una decisión aparte —con su propio permiso y su motivo— porque
// levantar ese candado es quitar la última barrera contra el doble pago.
func (s *Service) MarcarConvenioMasivo(ctx context.Context, empresaID string, in ConvenioMasivoInput, usuarioID string) (ResultadoConvenioMasivo, error) {
	if err := in.Validar(); err != nil {
		return ResultadoConvenioMasivo{}, err
	}
	res, err := s.repo.MarcarConvenioMasivo(ctx, empresaID, in.ProveedorIDs, in.Convenio, in.PasarACredito)
	if err != nil {
		return ResultadoConvenioMasivo{}, err
	}
	// Un evento por proveedor: la ficha de cada uno tiene que poder contar su propia historia, y
	// un solo evento con una lista adentro no aparece al mirar el proveedor.
	for _, id := range in.ProveedorIDs {
		s.auditar(ctx, empresaID, id, "MARCAR_CONVENIO_"+in.Convenio, usuarioID)
	}
	// El paso a CRÉDITO se audita aparte, y solo en los que de verdad cambiaron. Es un cambio en
	// la condición comercial del proveedor —no en a quién se le cree—, y dentro de seis meses la
	// pregunta «¿quién dijo que este proveedor era a crédito?» tiene que encontrar su respuesta en
	// la ficha, no deducirse de un evento que habla de otra cosa.
	for _, id := range res.PasadosACreditoIDs {
		s.auditarProveedor(ctx, empresaID, id, "CONFIGURAR_SIN_CANDADO", usuarioID,
			"Marcado en lote desde «Proveedores que declaran contado»: la ficha pasó de CONTADO a "+
				"CRÉDITO para que sus facturas no nazcan bloqueadas. El plazo de crédito no se tocó.")
	}
	return res, nil
}

// LiberarDeProveedores levanta el candado de todo lo bloqueado de esos proveedores.
func (s *Service) LiberarDeProveedores(ctx context.Context, empresaID string, ids []string, motivo, usuarioID string) (int64, error) {
	if len(ids) == 0 {
		return 0, ErrSinProveedores
	}
	if len(ids) > topeProveedoresMasivo {
		return 0, ErrDemasiadosProveedores
	}
	motivo = strings.TrimSpace(motivo)
	// El MISMO mínimo que liberar de a una: liberar en lote no es una afirmación más liviana, es
	// la misma hecha muchas veces.
	if len([]rune(motivo)) < motivoLiberacionMinimo {
		return 0, ErrMotivoLiberacionRequerido
	}
	n, err := s.repo.LiberarDeProveedores(ctx, empresaID, ids, motivo, usuarioID)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		s.auditar(ctx, empresaID, id, "LIBERAR_BLOQUEADAS_"+strconv.FormatInt(n, 10), usuarioID)
	}
	return n, nil
}
