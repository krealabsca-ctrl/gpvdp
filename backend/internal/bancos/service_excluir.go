package bancos

// Excluir un movimiento suelto, del lado del negocio.

import (
	"context"
	"sort"
	"strings"
)

// ExcluirMovimiento saca un movimiento de los libros.
//
// El motivo es obligatorio y se audita. Sacar plata de los libros sin decir por qué deja un
// faltante indistinguible de un error del sistema, y el que cierra el mes no tiene cómo saber si
// perdió una carga o si alguien corrigió un duplicado.
func (s *Service) ExcluirMovimiento(ctx context.Context, empresaID, movID, motivo, usuarioID string) error {
	motivo = strings.TrimSpace(motivo)
	if !MotivoExclusionValido(motivo) {
		return ErrMotivoExclusionRequerido
	}
	// Se lee ANTES para poder auditar qué se sacó: después del UPDATE el movimiento ya no está en
	// los libros y la bitácora diría «se excluyó algo» sin decir qué.
	m, err := s.repo.MovimientoParaExcluir(ctx, empresaID, movID)
	if err != nil {
		return err
	}
	if err := s.repo.ExcluirMovimiento(ctx, empresaID, movID, motivo, usuarioID); err != nil {
		return err
	}
	// La bitácora guarda QUÉ se sacó, no solo que se sacó: fecha, cuenta, consecutivo, monto y las
	// primeras palabras de la descripción. Con eso, quien audite el mes puede reconstruir el
	// movimiento sin ir a buscarlo, que es justamente lo que no va a poder hacer si algún día se
	// purga la fila.
	s.auditarSegmento(ctx, empresaID, "movimiento_bancario", movID, "EXCLUIR_MOVIMIENTO", usuarioID,
		map[string]any{
			"fecha": m.Fecha, "cuenta": m.Cuenta, "banco": m.Banco,
			"documento": m.Documento, "monto": montoDeLaFila(m),
			"descripcion": recorte(m.Descripcion, 80), "motivo": motivo,
		})
	return nil
}

// IncluirMovimiento lo devuelve a los libros.
func (s *Service) IncluirMovimiento(ctx context.Context, empresaID, movID, usuarioID string) error {
	m, err := s.repo.MovimientoParaExcluir(ctx, empresaID, movID)
	if err != nil {
		return err
	}
	if err := s.repo.IncluirMovimiento(ctx, empresaID, movID); err != nil {
		return err
	}
	// Volver a incluir también se audita: devolver plata a los libros mueve los mismos números que
	// sacarla, y una bitácora que solo registra una de las dos direcciones no cuadra.
	s.auditarSegmento(ctx, empresaID, "movimiento_bancario", movID, "INCLUIR_MOVIMIENTO", usuarioID,
		map[string]any{
			"fecha": m.Fecha, "cuenta": m.Cuenta, "banco": m.Banco,
			"documento": m.Documento, "monto": montoDeLaFila(m),
			"descripcion":   recorte(m.Descripcion, 80),
			"motivo_previo": m.Motivo,
		})
	return nil
}

// ExcluidosAMano lista lo que está fuera de los libros por decisión de alguien.
func (s *Service) ExcluidosAMano(ctx context.Context, empresaID string) ([]MovimientoExcluido, error) {
	return s.repo.ExcluidosAMano(ctx, empresaID)
}

// DuplicadosSospechosos propone pares que podrían ser el mismo hecho cargado dos veces.
//
// Los ordena por cuántas señales tienen: lo que cumple las tres va primero, porque es lo único
// que vale la pena mirar de un archivo de cien pares. NO excluye nada: el sistema propone y una
// persona decide, por la misma razón que el cruce de pagos del sistema anterior no marca cuando
// hay varios candidatos.
func (s *Service) DuplicadosSospechosos(ctx context.Context, empresaID, desde, hasta string) ([]ParDuplicado, error) {
	if strings.TrimSpace(desde) == "" || strings.TrimSpace(hasta) == "" {
		return nil, ErrRangoDuplicadosRequerido
	}
	pares, err := s.repo.DuplicadosSospechosos(ctx, empresaID, desde, hasta)
	if err != nil {
		return nil, err
	}
	for i := range pares {
		ContarSenales(&pares[i])
	}
	sort.SliceStable(pares, func(i, j int) bool { return pares[i].Senales > pares[j].Senales })
	return pares, nil
}

// montoDeLaFila arma «₡12.500,00 débito» para la bitácora: sin el lado, un monto suelto no dice si
// la corrección sumó o restó a los libros.
func montoDeLaFila(m MovimientoExcluido) string {
	if montoNum(m.Debito) > 0 {
		return m.Debito + " débito"
	}
	return m.Credito + " crédito"
}

func recorte(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
