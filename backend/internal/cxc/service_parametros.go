package cxc

// Configuración del módulo. Acá viven las REGLAS: qué valor es válido para cada parámetro y
// cuáles todavía no se pueden cambiar porque el motor no los lee.
//
// Esa última parte es deliberada y es la más importante de este archivo: dejar que alguien
// ponga «FECHA_COBRO_DEL_MES = PAGO» y creer que el sistema cambió sería peor que no tener
// la pantalla. Un parámetro que nadie lee se muestra bloqueado, con el motivo.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// reglaParametro describe qué se acepta en una clave y si hoy sirve de algo.
type reglaParametro struct {
	// tipo: "entero", "monto", "fecha_opcional" o "opciones".
	tipo     string
	min, max int
	opciones []string
	// leidoPor: dónde lo usa el motor. Vacío = NADIE lo lee todavía, y entonces no se puede
	// editar: la pantalla lo muestra bloqueado con la nota.
	leidoPor string
	// nota explica al usuario qué falta para que el parámetro sirva.
	nota string
}

// reglasParametros es la tabla de verdad de la configuración de CxC.
//
// Se actualiza cuando un parámetro empieza a ser leído: mover una línea de «pendiente» a
// «leidoPor» es lo que desbloquea su edición en la pantalla.
var reglasParametros = map[string]reglaParametro{
	"DIAS_SIN_GESTIONAR": {
		tipo: "entero", min: 1, max: 3650,
		leidoPor: "la cola de cobro, para marcar un contrato como desatendido",
	},
	"PROMESA_TOLERANCIA_DIAS": {
		tipo: "entero", min: 0, max: 60,
		leidoPor: "la cola y el historial, para juzgar si una promesa se cumplió",
	},
	"DIAS_ALERTA_TARJETA": {
		tipo: "entero", min: 0, max: 365,
		leidoPor: "la cola, para avisar de una tarjeta que está por vencer",
	},
	"COBRO_MAXIMO_RAZONABLE": {
		tipo: "monto", leidoPor: "el importador de pagos, para mandar a revisión un cobro absurdo",
	},
	"CUOTA_MAXIMA_RAZONABLE": {
		tipo: "monto", leidoPor: "el importador de cartera, para mandar a cuarentena una cuota absurda",
	},
	"CARGOS_DESDE": {
		tipo:     "fecha_opcional",
		leidoPor: "el generador de cargos, como fecha por omisión cuando no se indica una",
	},
	"MESES_PARA_SUSPENDER": {
		tipo: "entero", min: 1, max: 600,
		leidoPor: "la cola y la ficha del contrato, para marcar cuándo se puede suspender el servicio. " +
			"Son MESES de mora: cada modalidad los convierte a sus propias cuotas (un quincenal " +
			"necesita el doble de cuotas que un mensual para llegar al mismo atraso)",
	},
	"ARREGLO_PLAZOS_ESTANDAR": {
		tipo: "lista_enteros", min: 1, max: 600,
		leidoPor: "el pactado de arreglos, para saber qué plazos puede dar un gestor sin autorización; " +
			"cualquier otro plazo es excepción y lo aprueba el supervisor de piso",
	},
	"ARREGLO_PLAZO_MAXIMO": {
		tipo: "entero", min: 1, max: 600,
		leidoPor: "el pactado de arreglos, como tope duro de captura (no es una regla de negocio: " +
			"es para que un dedazo no genere un plan de 999 cuotas)",
	},
	"DIAS_CONTACTO_PREVENTIVO": {
		tipo: "entero", min: 1, max: 365,
		leidoPor: "la lista de contacto preventivo, para decidir cuántos días antes del vencimiento " +
			"entra un contrato al día",
	},
	"PLANILLA_TOLERANCIA": {
		// Cero es válido y es el valor de arranque: no se tolera nada hasta que el negocio
		// diga cuánto (p. ej. si las asociaciones depositan neto de comisión bancaria).
		tipo:     "monto_cero_ok",
		leidoPor: "la conciliación de planillas, para decidir si el depósito calza con el detalle",
	},
	// ── Los que todavía NO sirven ───────────────────────────────────────────────
	"APLICACION_COBROS": {
		tipo: "opciones", opciones: []string{"MAS_VIEJO"},
		nota: "hoy el motor aplica siempre el cargo más viejo primero (FIFO). Otras políticas " +
			"habría que construirlas antes de poder elegirlas acá.",
	},
	"FECHA_COBRO_DEL_MES": {
		tipo: "opciones", opciones: []string{"PAGO", "BANCARIA", "REGISTRO"},
		nota: "hoy todo el módulo usa la fecha BANCARIA, que es la que concilia contra Bancos. " +
			"Cambiarlo movería los números de Cobros y del panorama de asociaciones: es una " +
			"decisión, no un ajuste.",
	},
	"DIAS_PROMESA_VIGENTE": {
		tipo: "entero", min: 1, max: 365,
		nota: "la vigencia de una promesa hoy sale de su propia fecha más la tolerancia. " +
			"Falta definir qué debería hacer este número: ¿topar a cuántos días se puede " +
			"prometer, o vencer la promesa aunque su fecha no haya llegado?",
	},
}

// ErrParametroInvalido se devuelve con el motivo puntual del rechazo.
type ErrParametroInvalido struct {
	Clave  string
	Motivo string
}

func (e *ErrParametroInvalido) Error() string {
	return fmt.Sprintf("cxc: parámetro %s: %s", e.Clave, e.Motivo)
}

// Config es la configuración completa del módulo, con las notas de editabilidad ya puestas.
func (s *Service) Config(ctx context.Context, empresaID string) (ConfigCxC, error) {
	cfg, err := s.repo.ConfigCxC(ctx, empresaID)
	if err != nil {
		return ConfigCxC{}, err
	}
	for i := range cfg.Parametros {
		r, conocido := reglasParametros[cfg.Parametros[i].Clave]
		// Una clave desconocida (agregada a mano en la base) se muestra pero no se edita:
		// el ERP no sabe qué significa ni qué validar.
		cfg.Parametros[i].Editable = conocido && r.leidoPor != ""
		cfg.Parametros[i].LeidoPor = r.leidoPor
		cfg.Parametros[i].Nota = r.nota
		if !conocido {
			cfg.Parametros[i].Nota = "clave desconocida para el ERP: no se valida ni se usa."
		}
		cfg.Parametros[i].Opciones = r.opciones
		cfg.Parametros[i].Tipo = r.tipo
	}
	return cfg, nil
}

// GuardarParametros valida y guarda. Rechaza el LOTE completo si una sola clave está mal:
// guardar la mitad de una configuración deja al módulo en un estado que nadie pidió.
func (s *Service) GuardarParametros(ctx context.Context, empresaID string, valores map[string]string, usuarioID string) (int, error) {
	for clave, valor := range valores {
		if err := validarParametro(clave, valor); err != nil {
			return 0, err
		}
	}
	n, err := s.repo.GuardarParametros(ctx, empresaID, valores, usuarioID)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		s.auditar(ctx, empresaID, "GUARDAR_PARAMETROS_CXC", usuarioID, map[string]any{
			"cambiados": n, "valores": valores,
		})
	}
	return n, nil
}

func validarParametro(clave, valor string) error {
	r, ok := reglasParametros[clave]
	if !ok {
		return &ErrParametroInvalido{Clave: clave, Motivo: "no es un parámetro del módulo"}
	}
	if r.leidoPor == "" {
		return &ErrParametroInvalido{Clave: clave, Motivo: "todavía no se puede cambiar — " + r.nota}
	}
	switch r.tipo {
	case "entero":
		n, err := strconv.Atoi(strings.TrimSpace(valor))
		if err != nil {
			return &ErrParametroInvalido{Clave: clave, Motivo: "tiene que ser un número entero"}
		}
		if n < r.min || n > r.max {
			return &ErrParametroInvalido{Clave: clave, Motivo: fmt.Sprintf("tiene que estar entre %d y %d", r.min, r.max)}
		}
	case "monto":
		m, err := decimal.NewFromString(strings.TrimSpace(valor))
		if err != nil || m.Sign() <= 0 {
			return &ErrParametroInvalido{Clave: clave, Motivo: "tiene que ser un monto mayor que cero"}
		}
	case "monto_cero_ok":
		m, err := decimal.NewFromString(strings.TrimSpace(valor))
		if err != nil || m.Sign() < 0 {
			return &ErrParametroInvalido{Clave: clave, Motivo: "tiene que ser un monto de cero o más"}
		}
	case "fecha_opcional":
		if strings.TrimSpace(valor) == "" {
			return nil
		}
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(valor)); err != nil {
			return &ErrParametroInvalido{Clave: clave, Motivo: "tiene que ser una fecha AAAA-MM-DD (o quedar vacío)"}
		}
	case "opciones":
		if !contiene(r.opciones, strings.TrimSpace(valor)) {
			return &ErrParametroInvalido{Clave: clave, Motivo: "tiene que ser uno de: " + strings.Join(r.opciones, ", ")}
		}
	case "lista_enteros":
		// Lista separada por comas (los plazos estándar de un arreglo: «1,3,6,9»). Se valida
		// entera: una lista con un solo token malo no se guarda a medias.
		partes := strings.Split(strings.TrimSpace(valor), ",")
		if len(partes) == 0 || strings.TrimSpace(valor) == "" {
			return &ErrParametroInvalido{Clave: clave, Motivo: "tiene que traer al menos un número"}
		}
		for _, tok := range partes {
			n, err := strconv.Atoi(strings.TrimSpace(tok))
			if err != nil {
				return &ErrParametroInvalido{Clave: clave,
					Motivo: "tiene que ser una lista de números separados por comas, por ejemplo 1,3,6,9"}
			}
			if n < r.min || n > r.max {
				return &ErrParametroInvalido{Clave: clave,
					Motivo: fmt.Sprintf("cada número tiene que estar entre %d y %d", r.min, r.max)}
			}
		}
	}
	return nil
}

// ActualizarTramo cambia la probabilidad, la estrategia, el canal o el rango de días.
// La probabilidad es lo que multiplica el valor esperado: cambiarla reordena la cola.
func (s *Service) ActualizarTramo(ctx context.Context, empresaID, codigo string, c CambioTramo, usuarioID string) error {
	if c.Prob != nil {
		p, err := decimal.NewFromString(strings.TrimSpace(*c.Prob))
		if err != nil || p.LessThan(decimal.Zero) || p.GreaterThan(decimal.NewFromInt(1)) {
			return &ErrParametroInvalido{Clave: "prob_recuperacion", Motivo: "tiene que ser un número entre 0 y 1"}
		}
	}
	if c.DiasMin != nil && c.DiasMax != nil && *c.DiasMin > *c.DiasMax {
		return &ErrParametroInvalido{Clave: "dias", Motivo: "el día mínimo no puede ser mayor que el máximo"}
	}
	if err := s.repo.ActualizarTramo(ctx, empresaID, codigo, c); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "ACTUALIZAR_TRAMO_CXC", usuarioID, map[string]any{
		"codigo": codigo, "prob": valorOVacio(c.Prob), "estrategia": valorOVacio(c.Estrategia),
		"canal": valorOVacio(c.CanalSugerido),
	})
	return nil
}

// ActualizarFormaPago cambia el factor de recuperación, que también reordena la cola.
func (s *Service) ActualizarFormaPago(ctx context.Context, empresaID, id string, factor *string, activa *bool, usuarioID string) error {
	if factor != nil {
		f, err := decimal.NewFromString(strings.TrimSpace(*factor))
		// El mismo rango que el CHECK de la tabla, para que el error sea explicativo y no
		// una violación de restricción traducida a «error interno».
		if err != nil || f.LessThan(decimal.NewFromFloat(0.10)) || f.GreaterThan(decimal.NewFromInt(2)) {
			return &ErrParametroInvalido{Clave: "factor_recuperacion", Motivo: "tiene que estar entre 0,10 y 2,00"}
		}
	}
	if err := s.repo.ActualizarFormaPago(ctx, empresaID, id, factor, activa); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "ACTUALIZAR_FORMA_PAGO_CXC", usuarioID, map[string]any{
		"forma_pago": id, "factor": valorOVacio(factor),
	})
	return nil
}

// CrearSede registra una sede operativa (la dimensión que decide quién ve qué cartera).
func (s *Service) CrearSede(ctx context.Context, empresaID, nombre, razonSocial, plaza, usuarioID string) (SedeConfig, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return SedeConfig{}, &ErrParametroInvalido{Clave: "nombre", Motivo: "la sede necesita un nombre"}
	}
	sede, err := s.repo.CrearSede(ctx, empresaID, nombre, razonSocial, plaza)
	if err != nil {
		return SedeConfig{}, err
	}
	s.auditar(ctx, empresaID, "CREAR_SEDE_CXC", usuarioID, map[string]any{"sede": sede.ID, "nombre": nombre})
	return sede, nil
}

func (s *Service) ActualizarSede(ctx context.Context, empresaID, id string, nombre *string, activa *bool, usuarioID string) error {
	if nombre != nil && strings.TrimSpace(*nombre) == "" {
		return &ErrParametroInvalido{Clave: "nombre", Motivo: "la sede necesita un nombre"}
	}
	if err := s.repo.ActualizarSede(ctx, empresaID, id, nombre, activa); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "ACTUALIZAR_SEDE_CXC", usuarioID, map[string]any{
		"sede": id, "nombre": valorOVacio(nombre),
	})
	return nil
}

// AsignarSedes define qué cartera ve un usuario. Es la frontera de datos del módulo, así
// que el cambio queda auditado con la lista completa.
func (s *Service) AsignarSedes(ctx context.Context, empresaID, usuarioObjetivo string, sedeIDs []string, usuarioID string) error {
	if err := s.repo.AsignarSedes(ctx, empresaID, usuarioObjetivo, sedeIDs); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, "ASIGNAR_SEDES_CXC", usuarioID, map[string]any{
		"usuario": usuarioObjetivo, "sedes": sedeIDs, "cantidad": len(sedeIDs),
	})
	return nil
}

func valorOVacio(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
