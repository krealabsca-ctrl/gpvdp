package bancos

// Dividir un movimiento, del lado del negocio.

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"
)

// DividirMovimiento aparta parte de un movimiento a otra partida.
//
// Las validaciones van ANTES de tocar la base y cada una con su propio error, para que la pantalla
// pueda decir qué arreglar. El repositorio igual las vuelve a comprobar dentro del UPDATE: entre
// que alguien mira la pantalla y confirma pueden pasar minutos, y la última palabra tiene que
// tenerla la sentencia que escribe.
func (s *Service) DividirMovimiento(ctx context.Context, empresaID, movID string, in DivisionInput, usuarioID string) (Division, error) {
	if strings.TrimSpace(in.ClasificacionID) == "" {
		return Division{}, ErrDivisionSinClasificacion
	}
	if in.Monto.LessThanOrEqual(decimal.Zero) {
		return Division{}, ErrMontoDivisionInvalido
	}
	m, err := s.repo.MovimientoParaDividir(ctx, empresaID, movID)
	if err != nil {
		return Division{}, err
	}
	if err := m.PuedeDividirse(); err != nil {
		return Division{}, err
	}
	// Menor ESTRICTO: apartar el monto completo no es dividir, es reclasificar.
	total, err := decimal.NewFromString(m.Monto)
	if err != nil {
		return Division{}, ErrMontoDivisionInvalido
	}
	if in.Monto.GreaterThanOrEqual(total) {
		return Division{}, ErrMontoDivisionExcede
	}
	// El equivalente en colones se reparte en la MISMA proporción que el monto, y se calcula acá
	// una sola vez: si la pantalla lo mandara, dos redondeos distintos podrían hacer que la suma
	// de las partes no dé el total en colones aunque sí dé en la moneda de la cuenta.
	in.MontoCRC = proporcionCRC(m.Monto, m.MontoCRC, in.Monto)

	if _, err := s.repo.DividirMovimiento(ctx, empresaID, movID, in, usuarioID); err != nil {
		return Division{}, err
	}
	s.auditarSegmento(ctx, empresaID, "movimiento_bancario", movID, "DIVIDIR_MOVIMIENTO", usuarioID,
		map[string]any{
			"descripcion": recorte(m.Descripcion, 80),
			"monto_total": m.Monto, "monto_separado": in.Monto.String(),
			"motivo": in.Motivo,
		})
	return s.repo.PartesDeMovimiento(ctx, empresaID, movID)
}

// DeshacerDivision devuelve una parte a su movimiento principal.
func (s *Service) DeshacerDivision(ctx context.Context, empresaID, parteID, usuarioID string) error {
	if err := s.repo.DeshacerDivision(ctx, empresaID, parteID); err != nil {
		return err
	}
	s.auditarSegmento(ctx, empresaID, "movimiento_bancario", parteID, "DESHACER_DIVISION", usuarioID, nil)
	return nil
}

// PartesDeMovimiento muestra el reparto completo, con la invariante a la vista.
func (s *Service) PartesDeMovimiento(ctx context.Context, empresaID, movID string) (Division, error) {
	return s.repo.PartesDeMovimiento(ctx, empresaID, movID)
}

// proporcionCRC reparte el equivalente en colones en la misma proporción que el monto.
//
// Para una cuenta en colones total y totalCRC son iguales y esto devuelve `parte` sin tocarla. Para
// una en dólares, mantiene el mismo tipo de cambio efectivo que ya tenía el movimiento, en vez de
// aplicar el del mes: si el movimiento entró con un TC y la parte se calculara con otro, la suma
// de las partes dejaría de dar el total en colones.
func proporcionCRC(total, totalCRC string, parte decimal.Decimal) decimal.Decimal {
	t, err1 := decimal.NewFromString(total)
	tc, err2 := decimal.NewFromString(totalCRC)
	if err1 != nil || err2 != nil || t.IsZero() {
		return parte
	}
	if t.Equal(tc) {
		return parte
	}
	return parte.Mul(tc).Div(t).Round(2)
}
