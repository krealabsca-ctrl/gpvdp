package bancos

import "context"

// Stubs de fakeRepo para la conciliación bancaria mensual.

func (f *fakeRepo) ActasDelMes(_ context.Context, _ string, _, _ int) ([]ActaConciliacion, error) {
	return f.actas, nil
}

func (f *fakeRepo) PartidasDelMes(_ context.Context, _, cuentaID string, _, _ int) ([]PartidaConciliacion, error) {
	if cuentaID == "" {
		return f.partidas, nil
	}
	out := []PartidaConciliacion{}
	for _, p := range f.partidas {
		if p.CuentaID == cuentaID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeRepo) CrearPartida(_ context.Context, _ string, in PartidaInput, signo int, _ string) (string, error) {
	f.partidaCreada = in
	f.signoUsado = signo
	return "p-nueva", nil
}

func (f *fakeRepo) AnularPartida(_ context.Context, _, partidaID, _ string) error {
	f.partidaAnulada = partidaID
	return nil
}

func (f *fakeRepo) FirmarActa(_ context.Context, _, cuentaID string, _, _ int, banco, libros, ajuste, _ string) error {
	f.actaFirmada = cuentaID
	f.snapshotFirma = [3]string{banco, libros, ajuste}
	return nil
}

func (f *fakeRepo) RevisarSaldos(_ context.Context, _, fecha, _ string, congelar bool) (int, error) {
	f.fechaRevisada = fecha
	f.congelo = congelar
	return len(f.saldosDia), nil
}

// Stubs del descubridor de patrones.

func (f *fakeRepo) LineasSinClasificar(_ context.Context, _, _ string) ([]LineaSinClasificar, error) {
	return f.lineasSinClasif, nil
}

func (f *fakeRepo) DescripcionesEmpresa(_ context.Context, _ string) ([]string, error) {
	return f.descripciones, nil
}
