package bancos

// Enlazar dos movimientos reemitidos, del lado del negocio. El porqué está en reemitido.go.

import (
	"context"
	"fmt"
	"strings"
)

// diasMaxCandidatosReemision acota la sugerencia a tres días.
//
// EMPEZÓ EN SIETE Y SE BAJÓ CON LA LISTA DELANTE. Con siete días, los datos reales devolvían ocho
// candidatos y SIETE eran el mismo ahorro de ₡1.000.000 que el BCR debita cada semana con el
// consecutivo 1521673: la sugerencia quedaba enterrada en su propio ruido, y una lista así se
// aprende a ignorar —o peor, alguien enlaza dos cuotas reales y desaparece un millón de los libros—.
//
// Tres días deja pasar la reemisión después de un fin de semana (viernes → lunes) y corta justo
// antes de la recurrencia más corta que existe acá, la semanal. Medido: con este tope queda un
// solo candidato en toda la base. Las mensuales caen a 28, 30 o 35 días y nunca estuvieron cerca.
const diasMaxCandidatosReemision = 3

// CandidatosReemision propone pares para que una persona decida. NO enlaza nada.
func (s *Service) CandidatosReemision(ctx context.Context, empresaID string) ([]CandidatoReemision, error) {
	return s.repo.CandidatosReemision(ctx, empresaID, diasMaxCandidatosReemision)
}

// Reemisiones lista los enlaces ya hechos.
func (s *Service) Reemisiones(ctx context.Context, empresaID string) ([]Reemision, error) {
	return s.repo.Reemisiones(ctx, empresaID)
}

// EnlazarReemision declara que dos movimientos son el mismo hecho: el `vigenteID` se queda en los
// libros y el `salienteID` sale.
//
// CUÁL SE QUEDA LO DECIDE QUIEN LLAMA, no el sistema. Lo normal es que valga el más reciente —es el
// que el banco va a reportar en su saldo— pero no siempre: si el nuevo entró con menos información
// o cae en un mes que no corresponde, el que sirve es el viejo. Decidir por el usuario obligaría a
// deshacer el enlace para corregirlo.
func (s *Service) EnlazarReemision(ctx context.Context, empresaID, vigenteID, salienteID, motivo, usuarioID string) error {
	motivo = strings.TrimSpace(motivo)
	if !MotivoReemisionValido(motivo) {
		return ErrMotivoReemisionRequerido
	}
	if vigenteID == salienteID {
		return ErrReemisionMismoMovimiento
	}

	vigente, saliente, err := s.repo.DatosParaReemision(ctx, empresaID, vigenteID, salienteID)
	if err != nil {
		return err
	}
	// Las dos filas tienen que estar en los libros. Enlazar algo que ya salió no cambia ningún
	// número y deja un enlace que miente sobre por qué salió.
	if !vigente.Incluido || !saliente.Incluido {
		return ErrReemisionFueraDeLibros
	}
	if vigente.YaEnlazado || vigente.YaReemplazado || saliente.YaEnlazado || saliente.YaReemplazado {
		return ErrReemisionYaEnlazada
	}
	// El banco no reemite de una cuenta a otra: eso sería un traslado, y un traslado conserva sus
	// dos patas.
	if vigente.Cuenta != saliente.Cuenta {
		return ErrReemisionDistintaCuenta
	}
	// El mismo hecho no cambia de monto ni de lado al reemitirse.
	if vigente.Debito != saliente.Debito || vigente.Credito != saliente.Credito {
		return ErrReemisionDistintoMonto
	}

	// El que sale no puede estar en un mes ya cerrado: cerrar es declarar definitivos esos números.
	cerrado, err := s.repo.PeriodoCerrado(ctx, empresaID, saliente.Anio, saliente.Mes)
	if err != nil {
		return err
	}
	if cerrado {
		return ErrReemisionPeriodoCerrado
	}

	// Y no puede tener nada colgando. El usuario pidió que el sistema FRENE y nombre qué deshacer:
	// un cobro de CxC apuntando a plata que ya no cuenta es un descuadre que no se ve hasta el cierre.
	deps, err := s.repo.DependenciasDelMovimiento(ctx, empresaID, salienteID)
	if err != nil {
		return err
	}
	if len(deps) > 0 {
		return fmt.Errorf("%w: %s", ErrReemisionConDependencias, listaDeDependencias(deps))
	}

	// Hereda solo si el vigente NO está clasificado: pisar una clasificación puesta a mano
	// destruiría trabajo. El caso que motivó esto es el contrario —el reemitido entra en blanco y
	// el viejo ya estaba clasificado y revisado—.
	heredar := vigente.ClasificacionID == nil && vigente.ConceptoID == nil &&
		(saliente.ClasificacionID != nil || saliente.ConceptoID != nil)

	// El motivo que queda en «fuera de los libros» nombra la fecha del que SÍ cuenta, para que esa
	// lista se lea sola: «Reemplazado por el movimiento del 2026-11-05: …».
	motivoExclusion := "Reemplazado por el movimiento del " + vigente.Fecha + ": " + motivo

	heredo, err := s.repo.EnlazarReemision(ctx, empresaID, vigenteID, salienteID, motivo,
		motivoExclusion, usuarioID, heredar)
	if err != nil {
		return err
	}

	// La bitácora guarda QUÉ se enlazó, no solo que se enlazó: las dos fechas, el monto y si hubo
	// herencia. Con eso quien audite el mes puede reconstruir la decisión sin abrir la pantalla.
	s.auditarSegmento(ctx, empresaID, "movimiento_bancario", vigenteID, "ENLAZAR_REEMISION", usuarioID,
		map[string]any{
			"vigente": vigente.Fecha, "reemplazado": saliente.Fecha,
			"monto_debito": vigente.Debito, "monto_credito": vigente.Credito,
			"heredo_clasificacion": heredo, "motivo": motivo,
		})
	return nil
}

// DeshacerReemision rompe el enlace y devuelve a los libros el movimiento que había salido.
func (s *Service) DeshacerReemision(ctx context.Context, empresaID, vigenteID, usuarioID string) error {
	if err := s.repo.DeshacerReemision(ctx, empresaID, vigenteID); err != nil {
		return err
	}
	// Deshacer se audita igual que enlazar: devolver plata a los libros mueve los mismos números
	// que sacarla, y una bitácora que registra una sola dirección no cuadra.
	s.auditarSegmento(ctx, empresaID, "movimiento_bancario", vigenteID, "DESHACER_REEMISION", usuarioID,
		map[string]any{"movimiento": vigenteID})
	return nil
}

// listaDeDependencias arma «un cobro de CxC (1), un abono parcial a una factura (2)» para que el
// mensaje diga qué hay que deshacer y no solo que hay algo.
func listaDeDependencias(deps []DependenciaDeMovimiento) string {
	partes := make([]string, 0, len(deps))
	for _, d := range deps {
		partes = append(partes, fmt.Sprintf("%s (%d)", d.Que, d.Cantidad))
	}
	return strings.Join(partes, ", ")
}
