package bancos

import "context"

// Stubs de fakeRepo para el barrido de huellas Bancos↔CxP.

func (f *fakeRepo) MovimientosConHuella(_ context.Context, _, _, _ string) ([]MovimientoConHuella, error) {
	return f.movsConHuella, nil
}

func (f *fakeRepo) EnlazarPagoCxP(_ context.Context, _, movimientoID, documentoID, conceptoID, _ string) (bool, bool, error) {
	if f.yaEnlazados == nil {
		f.yaEnlazados = map[string]string{}
	}
	if _, existe := f.yaEnlazados[movimientoID]; existe {
		return false, false, nil // carrera: otro barrido lo tomó primero
	}
	f.yaEnlazados[movimientoID] = documentoID
	return conceptoID != "", true, nil
}
