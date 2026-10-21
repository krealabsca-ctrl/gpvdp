package precios

// Orquestación del importador: analizar, planificar y escribir.
//
// La importación es de DOS PASOS a propósito. El primero no toca nada y contesta «¿qué pasaría?»;
// el segundo exige que el usuario repita el número de insumos que el primero prometió. Es la
// misma defensa que usa rehacer cargos en CxC, y por la misma razón: escribir cientos de filas no
// tiene deshacer, y entre que la pantalla pinta el plan y alguien aprieta el botón el archivo
// pudo cambiar.

import (
	"context"
	"time"
)

// AnalizarArchivo mide qué pasaría al importar, sin escribir nada.
func (s *Service) AnalizarArchivo(archivo []byte, hoja, colNombre, colCosto string) (Analisis, error) {
	return Analizar(archivo, hoja, colNombre, colCosto)
}

// PlanearImportacion arma la lista exacta de insumos que se escribirían.
func (s *Service) PlanearImportacion(archivo []byte, hoja, colNombre, colCosto string, unificarSedes bool, sedes []string) (PlanDeImportacion, error) {
	return Planificar(archivo, hoja, colNombre, colCosto, unificarSedes, sedes)
}

// OpcionesImportacion es lo que llega del cliente al confirmar.
type OpcionesImportacion struct {
	Hoja          string
	ColumnaNombre string
	ColumnaCosto  string
	UnificarSedes bool
	// Sedes confirmadas por el usuario. Vacío = usar las detectadas.
	Sedes        []string
	VigenteDesde string
	// Confirmacion es el número de insumos que el usuario vio en el plan. Tiene que coincidir.
	Confirmacion int
}

// Importar escribe el catálogo, previa verificación de que el plan sigue siendo el que el
// usuario aprobó.
func (s *Service) Importar(ctx context.Context, empresaID string, archivo []byte, op OpcionesImportacion, usuarioID string) (ResultadoImportacion, error) {
	plan, err := Planificar(archivo, op.Hoja, op.ColumnaNombre, op.ColumnaCosto, op.UnificarSedes, op.Sedes)
	if err != nil {
		return ResultadoImportacion{}, err
	}
	if op.Confirmacion != len(plan.Insumos) {
		return ResultadoImportacion{}, ErrConfirmacionNoCoincide
	}

	vigente := op.VigenteDesde
	if vigente == "" {
		vigente = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", vigente); err != nil {
		return ResultadoImportacion{}, ErrFechaVigenciaVacia
	}

	return s.repo.ImportarCatalogo(ctx, empresaID, plan, vigente, usuarioID)
}

// AnalizarRecetasDelArchivo mide qué planes y recetas traería la matriz de costeo.
func (s *Service) AnalizarRecetasDelArchivo(archivo []byte, hoja string) (AnalisisRecetas, error) {
	return AnalizarRecetas(archivo, hoja)
}

// OpcionesRecetas es lo que llega al confirmar la importación de recetas.
type OpcionesRecetas struct {
	Hoja         string
	VigenteDesde string
	// Modalidades confirmadas por el usuario, por código de plan. Manda sobre la inferida.
	Modalidades map[string]string
	// Confirmacion es la cantidad de líneas de receta que el usuario vio en el plan.
	Confirmacion int
}

// ImportarRecetas escribe los planes y sus recetas.
func (s *Service) ImportarRecetas(ctx context.Context, empresaID string, archivo []byte, op OpcionesRecetas, usuarioID string) (ResultadoRecetas, error) {
	plan, err := PlanificarRecetas(archivo, op.Hoja, op.Modalidades)
	if err != nil {
		return ResultadoRecetas{}, err
	}
	if op.Confirmacion != len(plan.Lineas) {
		return ResultadoRecetas{}, ErrConfirmacionNoCoincide
	}
	vigente := op.VigenteDesde
	if vigente == "" {
		vigente = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", vigente); err != nil {
		return ResultadoRecetas{}, ErrFechaVigenciaVacia
	}
	return s.repo.ImportarRecetas(ctx, empresaID, plan, vigente, usuarioID)
}
