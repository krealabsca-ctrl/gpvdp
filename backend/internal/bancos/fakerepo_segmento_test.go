package bancos

import (
	"context"

	"github.com/shopspring/decimal"
)

// Stubs de fakeRepo para la consulta por segmento (mig 0077).

func (f *fakeRepo) AlcanceDeUsuario(context.Context, string, string) ([]string, error) {
	return f.alcance, nil
}

func (f *fakeRepo) PartidasDelAlcance(context.Context, string, []string) ([]PartidaDelSegmento, error) {
	return f.partidasAlcance, nil
}

func (f *fakeRepo) CuentasDelAlcance(context.Context, string, []string) ([]CuentaDelSegmento, error) {
	return f.cuentasAlcance, nil
}

func (f *fakeRepo) UltimaFechaCargada(context.Context, string) (string, error) {
	return f.cargadoHasta, nil
}

func (f *fakeRepo) RolesDeConsulta(context.Context, string) ([]RolDeConsulta, error) {
	return f.rolesConsulta, nil
}

func (f *fakeRepo) AsignacionesConsulta(context.Context, string) ([]AsignacionConsulta, error) {
	return f.asignConsulta, nil
}

func (f *fakeRepo) GuardarConsultaDePartida(_ context.Context, _, clasificacionID string, rolIDs []string) error {
	f.consultaGuardada.clasificacionID = clasificacionID
	f.consultaGuardada.rolIDs = rolIDs
	f.consultaGuardada.llamado = true
	return nil
}

func (f *fakeRepo) MovimientoEnAlcance(context.Context, string, string, []string) (bool, error) {
	return f.movEnAlcance, nil
}

func (f *fakeRepo) CrearReporteSegmentacion(_ context.Context, _, movID, usuarioID, motivo string) (string, error) {
	f.reporteCreado = [3]string{movID, usuarioID, motivo}
	return "rep-1", nil
}

func (f *fakeRepo) BuscarPorFechaYMonto(
	_ context.Context, _, _ string, _ decimal.Decimal, _ []string,
) ([]MovimientoRow, int, error) {
	return f.busquedaMios, f.busquedaFuera, nil
}

func (f *fakeRepo) EngancharFaltante(_ context.Context, _, _ string, _ decimal.Decimal) (string, error) {
	return f.enganche, nil
}

func (f *fakeRepo) CrearReporteFaltante(
	_ context.Context, _, usuarioID, fecha string, monto decimal.Decimal,
	referencia, motivo, movimientoID string,
) (string, error) {
	f.faltanteCreado = [6]string{usuarioID, fecha, monto.String(), referencia, motivo, movimientoID}
	return "rep-faltante-1", nil
}

func (f *fakeRepo) ListarReportesSegmentacion(context.Context, string, bool) ([]ReporteSegmentacion, error) {
	return f.reportes, nil
}

func (f *fakeRepo) ResolverReporteSegmentacion(_ context.Context, _, reporteID, usuarioID, resolucion, respuesta string) error {
	f.reporteResuelto = [4]string{reporteID, usuarioID, resolucion, respuesta}
	return nil
}

func (f *fakeRepo) ReportesDeMovimientos(context.Context, string, []string) (map[string]string, error) {
	if f.reportesAbiertos == nil {
		return map[string]string{}, nil
	}
	return f.reportesAbiertos, nil
}
