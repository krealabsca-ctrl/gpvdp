package bancos

import "context"

// Stubs de fakeRepo para las reemisiones del banco (mig 0118).
//
// Son stubs y no una imitación de la base a propósito: lo que hay que proteger acá es el SQL de la
// transacción —sacar uno de los libros Y enlazar el otro, o ninguna de las dos— y eso solo se puede
// probar contra Postgres de verdad (ver reemitido_pg_test.go). Un doble en memoria diría que todo
// funciona sin haber ejecutado una sola de esas sentencias.
//
// Los campos dejan que una prueba arme el caso que necesita sin tocar la base.

func (f *fakeRepo) CandidatosReemision(context.Context, string, int) ([]CandidatoReemision, error) {
	return f.candidatosReemision, nil
}

func (f *fakeRepo) Reemisiones(context.Context, string) ([]Reemision, error) {
	return f.reemisiones, nil
}

func (f *fakeRepo) DatosParaReemision(_ context.Context, _, _, _ string) (datosDeMovimiento, datosDeMovimiento, error) {
	if f.errDatosReemision != nil {
		return datosDeMovimiento{}, datosDeMovimiento{}, f.errDatosReemision
	}
	return f.vigenteReemision, f.salienteReemision, nil
}

func (f *fakeRepo) DependenciasDelMovimiento(context.Context, string, string) ([]DependenciaDeMovimiento, error) {
	return f.dependenciasMovimiento, nil
}

func (f *fakeRepo) EnlazarReemision(_ context.Context, _, vigenteID, salienteID, motivo, motivoExclusion, _ string, heredar bool) (bool, error) {
	f.enlazoVigente, f.enlazoSaliente = vigenteID, salienteID
	f.enlazoMotivo, f.enlazoMotivoExclusion = motivo, motivoExclusion
	f.enlazoHeredar = heredar
	return heredar, nil
}

func (f *fakeRepo) DeshacerReemision(_ context.Context, _, vigenteID string) error {
	f.deshizoReemision = vigenteID
	return nil
}
