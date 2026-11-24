package cxp

// El aviso de pago, armado. El porqué del diseño está en aviso_pago.go.

import (
	"context"
	"strings"
)

// largoMaximoConcepto y largoMaximoDocumentos son los mismos topes que la tabla (mig 0117). Se
// comprueban acá para devolver 422 con una explicación, en vez del 500 de una violación de CHECK.
const (
	largoMaximoConcepto   = 120
	largoMaximoDocumentos = 600
)

// EncabezadoDelAviso da los dos nombres de la línea de contexto del papel.
//
// Si la consulta falla devuelve vacíos y NO un error: el aviso se baja igual. Quedarse sin poder
// emitir el papel porque no se pudo leer un nombre sería cambiar un problema chico por uno grande.
func (s *Service) EncabezadoDelAviso(ctx context.Context, empresaID, usuarioID string) (string, string) {
	empresa, usuario, err := s.repo.EncabezadoDelAviso(ctx, empresaID, usuarioID)
	if err != nil {
		return "", ""
	}
	return empresa, usuario
}

// AvisoDeLote arma el cuadro de pago de un corte, partido por moneda.
func (s *Service) AvisoDeLote(ctx context.Context, empresaID, loteID string) (Aviso, error) {
	lote, err := s.repo.LotePorID(ctx, empresaID, loteID)
	if err != nil {
		return Aviso{}, err
	}
	// Las MISMAS filas que van al banco. Ver aviso_pago.go: es lo que hace imposible que el aviso
	// y la transferencia digan cosas distintas.
	rows, err := s.repo.DocumentosParaPagoPorLote(ctx, empresaID, loteID)
	if err != nil {
		return Aviso{}, err
	}
	datos, err := s.repo.DatosDeAvisoDelLote(ctx, empresaID, loteID)
	if err != nil {
		return Aviso{}, err
	}
	textos, err := s.repo.TextosDeAvisoDelLote(ctx, empresaID, loteID)
	if err != nil {
		return Aviso{}, err
	}
	// Las firmas salen de lo que el sistema registró (ver FirmasDelLote). Si fallan NO se cae el
	// aviso: quedarse sin poder emitir el papel porque no se pudo leer quién lo revisó sería
	// cambiar un problema chico por uno grande, igual que con la línea de contexto.
	firmas, fechaPago, err := s.repo.FirmasDelLote(ctx, empresaID, loteID)
	if err != nil {
		firmas = []Firma{
			{Rol: RolSolicitado}, {Rol: RolRevisado}, {Rol: RolAprobado}, {Rol: RolPagado},
		}
	}
	return Aviso{
		LoteID:     lote.ID,
		LoteNumero: lote.Numero,
		FechaCorte: lote.FechaCorte,
		Monedas:    ConstruirAviso(rows, datos, textos),
		Firmas:     firmas,
		FechaPago:  fechaPago,
	}, nil
}

// GuardarTextoDeAviso corrige el concepto o la glosa de un proveedor en un corte.
//
// Los punteros nil se respetan: mandar solo el concepto no borra la glosa. Es lo que permite que
// la pantalla guarde campo por campo sin tener que reenviar lo que no se tocó.
func (s *Service) GuardarTextoDeAviso(ctx context.Context, empresaID, loteID, proveedorID string, t TextoAviso, usuarioID string) error {
	if t.Concepto != nil {
		v := strings.TrimSpace(*t.Concepto)
		if len([]rune(v)) > largoMaximoConcepto {
			return ErrTextoDeAvisoLargo
		}
		t.Concepto = &v
	}
	if t.Documentos != nil {
		v := strings.TrimSpace(*t.Documentos)
		if len([]rune(v)) > largoMaximoDocumentos {
			return ErrTextoDeAvisoLargo
		}
		t.Documentos = &v
	}
	// Se leen los textos de antes para no pisar con NULL el campo que no venía en el pedido: el
	// INSERT ... ON CONFLICT escribe las dos columnas siempre, así que el que falte hay que
	// reponerlo con lo que ya estaba.
	previos, err := s.repo.TextosDeAvisoDelLote(ctx, empresaID, loteID)
	if err != nil {
		return err
	}
	if p, ok := previos[proveedorID]; ok {
		if t.Concepto == nil {
			t.Concepto = p.Concepto
		}
		if t.Documentos == nil {
			t.Documentos = p.Documentos
		}
	}
	ok, err := s.repo.GuardarTextoDeAviso(ctx, empresaID, loteID, proveedorID, t, usuarioID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrProveedorNoEstaEnElCorte
	}
	s.auditarProveedor(ctx, empresaID, proveedorID, "EDITAR_AVISO_PAGO", usuarioID,
		"Se corrigió el texto del aviso de pago de este proveedor en el corte.")
	return nil
}
