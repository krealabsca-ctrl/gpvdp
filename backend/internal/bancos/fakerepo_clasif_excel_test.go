package bancos

import "context"

// Stubs de fakeRepo para la clasificación en bloque desde Excel.

// BuscarMovimientosPorTupla devuelve lo sembrado, filtrado por las claves realmente pedidas: así el
// test comprueba también que el servicio pide lo que dice pedir.
func (f *fakeRepo) BuscarMovimientosPorTupla(_ context.Context, _ string, cuentas, fechas, debitos, creditos, documentos []string) ([]MovimientoCalzado, error) {
	pedidas := map[string]bool{}
	for i := range cuentas {
		pedidas[cuentas[i]+"|"+fechas[i]+"|"+debitos[i]+"|"+creditos[i]+"|"+documentos[i]] = true
	}
	out := []MovimientoCalzado{}
	for _, m := range f.movsCalzados {
		if pedidas[m.Clave] {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeRepo) AplicarClasificacionesEnBloque(_ context.Context, _ string, asigs []AsignacionClasif) (int, error) {
	f.asignados = append(f.asignados, asigs...)
	return len(asigs), nil
}

func (f *fakeRepo) MovimientosPlantillaClasif(context.Context, string, string, string, bool, int) ([]MovimientosParaPlantilla, error) {
	return f.plantillaMovs, nil
}
