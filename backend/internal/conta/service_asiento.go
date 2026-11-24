package conta

// La lógica del asiento y del enlace bancario (mig 0111, Fase 3A).

import (
	"context"
	"strings"
)

// Bancos lista las cuentas bancarias con su enlace contable y con la SUGERENCIA por IBAN de las
// que todavía no lo tienen.
//
// La sugerencia se calcula al leer y no se guarda. Guardarla la volvería indistinguible de una
// decisión tomada, y son cosas distintas: una la propuso el sistema comparando texto, la otra la
// firmó una persona.
func (s *Service) Bancos(ctx context.Context, empresaID string) ([]FilaBancoContable, CoberturaBancos, error) {
	filas, err := s.repo.FilasBanco(ctx, empresaID)
	if err != nil {
		return nil, CoberturaBancos{}, err
	}
	cuentas, err := s.repo.Cuentas(ctx, empresaID, true)
	if err != nil {
		return nil, CoberturaBancos{}, err
	}

	var cob CoberturaBancos
	for i := range filas {
		f := &filas[i]
		cob.Cuentas++
		cob.MovimientosTotales += f.Movimientos
		if f.CuentaID != nil {
			cob.Enlazadas++
			cob.MovimientosCubiertos += f.Movimientos
			continue
		}
		cob.Faltan++
		if c, ok := SugerirPorIBAN(f.IBAN, cuentas); ok {
			f.SugeridaID, f.SugeridaCodigo, f.SugeridaNombre = c.ID, c.Codigo, c.Nombre
			cob.ConSugerencia++
		}
	}
	return filas, cob, nil
}

// EnlazarBanco guarda qué cuenta contable representa una cuenta bancaria.
func (s *Service) EnlazarBanco(ctx context.Context, empresaID, cuentaBancariaID, cuentaID, usuarioID string) error {
	cuentaBancariaID = strings.TrimSpace(cuentaBancariaID)
	cuentaID = strings.TrimSpace(cuentaID)
	if cuentaID == "" {
		return ErrCuentaRequerida
	}
	if err := s.repo.GuardarEnlaceBanco(ctx, empresaID, cuentaBancariaID, cuentaID, usuarioID); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, cuentaBancariaID, "ENLAZAR_BANCO", usuarioID,
		map[string]any{"cuenta_id": cuentaID})
	return nil
}

// AceptarSugerencias enlaza de una vez todas las cuentas que tienen sugerencia por IBAN.
//
// EXISTE PORQUE ES EL 79 % DEL TRABAJO. Medido en Valle de Paz: 5 de 16 cuentas se proponen solas
// y esas 5 cubren 15.873 de 20.082 movimientos. Hacer cinco clics idénticos para aceptar lo que
// el sistema ya dedujo no agrega control: el control está en que se ve qué va a enlazar antes de
// apretar, y en que cada una se puede deshacer.
func (s *Service) AceptarSugerencias(ctx context.Context, empresaID, usuarioID string) (int, error) {
	filas, _, err := s.Bancos(ctx, empresaID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range filas {
		if f.CuentaID != nil || f.SugeridaID == "" {
			continue
		}
		if err := s.EnlazarBanco(ctx, empresaID, f.CuentaBancariaID, f.SugeridaID, usuarioID); err != nil {
			// Igual que el lote del mapeo: una que falle no tumba a las demás. Lo que no entró
			// sigue visible en la lista como «falta».
			continue
		}
		n++
	}
	return n, nil
}

// DesenlazarBanco deshace el enlace.
func (s *Service) DesenlazarBanco(ctx context.Context, empresaID, cuentaBancariaID, usuarioID string) error {
	if err := s.repo.BorrarEnlaceBanco(ctx, empresaID, cuentaBancariaID); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, cuentaBancariaID, "DESENLAZAR_BANCO", usuarioID, nil)
	return nil
}

// ── El asiento ──────────────────────────────────────────────────────────────────────────────

// CrearAsientoManual escribe un asiento a mano.
//
// Nace ASENTADO y no en borrador: un asiento que cuadra y que alguien escribió a propósito ya es
// parte del libro. El borrador existe en el modelo para cuando 3B genere en lote y haga falta
// revisar antes de asentar; mientras tanto, un estado intermedio sin pantalla que lo saque de ahí
// sería una trampa.
func (s *Service) CrearAsientoManual(ctx context.Context, empresaID string, n NuevoAsiento, usuarioID string) (Asiento, error) {
	lineas, err := n.Validar()
	if err != nil {
		return Asiento{}, err
	}
	a := Asiento{
		EmpresaID:  empresaID,
		Fecha:      n.Fecha,
		Glosa:      n.Glosa,
		OrigenTipo: OrigenManual,
		Estado:     EstadoAsentado,
	}
	creado, err := s.repo.CrearAsiento(ctx, a, lineas, usuarioID)
	if err != nil {
		return Asiento{}, err
	}
	s.auditar(ctx, empresaID, creado.ID, "CREAR_ASIENTO", usuarioID, map[string]any{
		"numero": creado.Numero, "glosa": creado.Glosa, "lineas": len(lineas),
	})
	// Se relee en vez de devolver lo que se armó en memoria. Lo armado no tiene los totales ni
	// el `creado_en` —los pone la base—, y devolverlo manda al cliente un asiento con el monto
	// en blanco y la fecha en el año 1.
	completo, err := s.repo.Asiento(ctx, empresaID, creado.ID)
	if err != nil {
		return creado, nil
	}
	return completo, nil
}

// AnularAsiento lo deja sin efecto, con su motivo.
func (s *Service) AnularAsiento(ctx context.Context, empresaID, id, motivo, usuarioID string) error {
	motivo = strings.TrimSpace(motivo)
	// Mismo mínimo que el resto del sistema: el «ok» reflejo no es una explicación, y acá menos
	// que en ningún lado —anular un asiento cambia los estados financieros—.
	if len(motivo) < motivoMinimo {
		return ErrMotivoAnulacionRequerido
	}
	if err := s.repo.AnularAsiento(ctx, empresaID, id, motivo, usuarioID); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, id, "ANULAR_ASIENTO", usuarioID, map[string]any{"motivo": motivo})
	return nil
}

// Diario lista los asientos del período.
func (s *Service) Diario(ctx context.Context, empresaID string, f FiltroDiario) ([]Asiento, error) {
	if err := validarRango(f.Desde, f.Hasta); err != nil {
		return nil, err
	}
	if f.Limite <= 0 || f.Limite > topeDiario {
		f.Limite = topeDiario
	}
	return s.repo.Diario(ctx, empresaID, f)
}

// topeDiario acota la página. No es una regla de negocio: es que un libro diario de un año son
// miles de asientos y mandarlos todos de una vez no ayuda a nadie.
const topeDiario = 200

// Asiento devuelve uno con sus líneas.
func (s *Service) Asiento(ctx context.Context, empresaID, id string) (Asiento, error) {
	return s.repo.Asiento(ctx, empresaID, id)
}

// Mayor arma el movimiento de una cuenta.
func (s *Service) Mayor(ctx context.Context, empresaID string, f FiltroMayor) (Mayor, error) {
	if strings.TrimSpace(f.CuentaID) == "" {
		return Mayor{}, ErrCuentaRequerida
	}
	if err := validarRango(f.Desde, f.Hasta); err != nil {
		return Mayor{}, err
	}
	return s.repo.Mayor(ctx, empresaID, f)
}

// validarRango frena las fechas mal escritas ANTES de que lleguen a Postgres.
//
// Sin esto, `?desde=hola` revienta en la base y sale como 500 «error interno»: el usuario no
// entiende qué hizo mal y el sistema se registra un incidente a sí mismo. Pasó una vez ya.
func validarRango(desde, hasta string) error {
	for _, f := range []string{desde, hasta} {
		if f == "" {
			continue
		}
		if !esFechaISO(f) {
			return ErrFechaInvalida
		}
	}
	return nil
}

func esFechaISO(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, r := range s {
		if i == 4 || i == 7 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
