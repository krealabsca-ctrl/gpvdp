package sistema

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

// Service aplica las reglas de la consulta y escribe los incidentes.
type Service struct {
	repo Repository
	log  *zap.Logger
}

// NewService construye el servicio.
func NewService(repo Repository, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Registrar escribe el incidente y NO devuelve error, a propósito.
//
// Esto corre en el camino de una petición que YA falló. Si la escritura también falla —y el caso
// más probable es que falle justo cuando más incidentes hay, porque la base se cayó—, propagar
// ese error solo lograría reemplazar el error original por otro peor. Se deja en el log, que es
// adonde iba antes de esta migración, y la respuesta al usuario sale igual con su referencia.
//
// Tolera un *Service nil por la misma razón que `shared.Audit`: en producción siempre se inyecta,
// pero un cableado incompleto no puede tumbar las respuestas.
func (s *Service) Registrar(ctx context.Context, in Incidente) {
	if s == nil || s.repo == nil {
		return
	}
	// El detalle y la traza se acotan acá y no en el SQL: un error de pgx con un INSERT de 200
	// columnas, o la traza de un panic profundo, llegan a decenas de miles de caracteres. Guardar
	// eso entero convierte la tabla en un depósito de texto y la pantalla en algo ilegible; lo que
	// hace falta para encontrar la causa está siempre al principio.
	in.Detalle = acotar(in.Detalle, 4000)
	in.Traza = acotar(in.Traza, 8000)
	if err := s.repo.Registrar(ctx, in); err != nil && s.log != nil {
		s.log.Error("sistema: no se pudo registrar el incidente",
			zap.String("ref", in.Referencia), zap.String("ruta", in.Ruta), zap.Error(err))
	}
}

// Listar devuelve lo que esta empresa puede ver.
//
// Normaliza la referencia (mayúsculas, sin espacios) porque el usuario la dicta y quien la escribe
// la teclea como venga: «3f8a2c» y « 3F8A2C » son la misma referencia, y devolver vacío por una
// minúscula haría que el código pareciera inventado.
func (s *Service) Listar(ctx context.Context, empresaID string, f FiltrosIncidentes) ([]Incidente, error) {
	f.Referencia = strings.ToUpper(strings.TrimSpace(f.Referencia))
	if f.Referencia != "" && !referenciaValida(f.Referencia) {
		return nil, ErrReferenciaInvalida
	}
	if !fechaValida(f.Desde) || !fechaValida(f.Hasta) {
		return nil, ErrFechaInvalida
	}
	if f.Limite <= 0 {
		f.Limite = limiteDefecto
	}
	if f.Limite > limiteMaximo {
		f.Limite = limiteMaximo
	}
	return s.repo.Listar(ctx, empresaID, f)
}

// Resumen cuenta lo mismo que Listar ve.
func (s *Service) Resumen(ctx context.Context, empresaID string) (ResumenIncidentes, error) {
	return s.repo.Resumen(ctx, empresaID)
}

// acotar recorta a `max` runas sin partir un carácter multibyte por la mitad.
//
// Se cuentan RUNAS y no bytes: un recorte a la mitad de una «ó» deja un byte suelto que Postgres
// rechaza como texto inválido, y el incidente no se guardaría —justo el que se quería guardar—.
func acotar(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "… (recortado)"
}
