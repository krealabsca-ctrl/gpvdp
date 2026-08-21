// Package plantillas son las plantillas de correo de las notificaciones del ERP: el texto con
// el que se le avisa a un proveedor que se le pagó, o a un colaborador que su boleta está lista.
//
// Cada empresa del grupo se comunica distinto, así que el texto es CONFIGURACIÓN y no código. El
// catálogo de TIPOS (qué notificaciones existen, qué variables tiene cada una y cuál es el texto
// por defecto) sí vive acá: es lo que el sistema sabe llenar.
//
// Las variables se escriben entre corchetes: [NOMBRE_PROVEEDOR]. Se eligieron corchetes y no
// llaves porque el cuerpo de un correo lleva llaves con más frecuencia que corchetes.
package plantillas

import (
	"errors"
	"regexp"
	"sort"
	"strings"
)

var (
	// ErrTipoDesconocido indica una clave de plantilla que no está en el catálogo.
	ErrTipoDesconocido = errors.New("plantillas: tipo de notificación desconocido")
	// ErrAsuntoVacio y ErrCuerpoVacio: una plantilla sin texto no se guarda.
	ErrAsuntoVacio = errors.New("plantillas: el asunto no puede quedar vacío")
	ErrCuerpoVacio = errors.New("plantillas: el cuerpo no puede quedar vacío")
	// ErrVariablesDesconocidas: el texto usa variables que el sistema no sabe llenar. Se
	// rechaza al guardar en vez de mandarle «[FOO]» a un proveedor.
	ErrVariablesDesconocidas = errors.New("plantillas: la plantilla usa variables que no existen")
)

// Claves de los tipos de notificación.
const (
	// ClaveCxPComprobante: se le avisa al proveedor que su factura fue pagada (con el
	// comprobante adjunto).
	ClaveCxPComprobante = "CXP_COMPROBANTE"
	// ClaveNominaBoleta: la boleta de pago del colaborador.
	ClaveNominaBoleta = "NOMINA_BOLETA"
	// ClaveNominaVacaciones: el aviso de vacaciones aprobadas.
	ClaveNominaVacaciones = "NOMINA_VACACIONES"
)

// reVariable reconoce [NOMBRE_DE_VARIABLE] (mayúsculas, dígitos y guion bajo).
var reVariable = regexp.MustCompile(`\[([A-Z0-9_]+)\]`)

// Variable es un dato que el sistema sabe poner en el texto.
type Variable struct {
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion"`
	// Ejemplo se usa en la vista previa, para que se vea cómo queda antes de enviarlo.
	Ejemplo string `json:"ejemplo"`
}

// Tipo es una notificación del sistema: qué es, qué variables tiene y su texto por defecto.
type Tipo struct {
	Clave       string     `json:"clave"`
	Nombre      string     `json:"nombre"`
	Descripcion string     `json:"descripcion"`
	Modulo      string     `json:"modulo"`
	Variables   []Variable `json:"variables"`
	// Texto de fábrica: rige mientras la empresa no guarde el suyo.
	AsuntoDefault string `json:"asunto_default"`
	CuerpoDefault string `json:"cuerpo_default"`
}

// variablesEmpresa son las que traen TODOS los tipos: quién manda el correo.
var variablesEmpresa = []Variable{
	{"NOMBRE_EMPRESA", "Nombre de la empresa que envía", "Valle de Paz Servicios Funerarios"},
	{"ANIO", "Año actual", "2026"},
}

// Catalogo son los tipos de notificación que el sistema sabe enviar. Agregar uno acá lo hace
// editable en la pantalla de plantillas sin tocar nada más.
var Catalogo = []Tipo{
	{
		Clave:       ClaveCxPComprobante,
		Nombre:      "Comprobante de pago al proveedor",
		Descripcion: "Se envía cuando Tesorería manda el comprobante de una factura pagada. El comprobante va adjunto.",
		Modulo:      "Cuentas por pagar",
		Variables: append([]Variable{
			{"NOMBRE_PROVEEDOR", "Nombre del proveedor", "Gas Tomza de Costa Rica"},
			{"CONSECUTIVO", "Consecutivo de la factura", "00100001010000025786"},
			{"MONTO", "Monto pagado, con su símbolo de moneda", "₡137 450,00"},
			{"MONEDA", "Moneda de la factura", "CRC"},
			{"REFERENCIA", "Referencia del pago en el banco (la huella)", "CXP-A1B2C3D4E5F6"},
			{"DESCRIPCION_FACTURA", "Descripción de la factura", "Gas para crematorio"},
		}, variablesEmpresa...),
		AsuntoDefault: "Comprobante de pago — factura [CONSECUTIVO]",
		CuerpoDefault: `Estimado proveedor [NOMBRE_PROVEEDOR],

Le informamos que su factura [CONSECUTIVO] fue pagada por un monto de [MONTO].

Adjuntamos el comprobante del pago para sus registros. La referencia con la que aparece en el banco es [REFERENCIA].

Gracias por su servicio.

[NOMBRE_EMPRESA]`,
	},
	{
		Clave:       ClaveNominaBoleta,
		Nombre:      "Boleta de pago al colaborador",
		Descripcion: "Se envía al correo del colaborador con el detalle de su pago del período. Solo llega a quien tenga correo registrado en su ficha.",
		Modulo:      "RRHH / Nómina",
		Variables: append([]Variable{
			{"NOMBRE_EMPLEADO", "Nombre del colaborador", "María Fernández Rojas"},
			{"IDENTIFICACION", "Cédula del colaborador", "1-1234-5678"},
			{"PUESTO", "Puesto", "Asesora de servicios"},
			{"PERIODO", "Período de la corrida", "Julio 2026"},
			{"FECHA_PAGO", "Fecha de pago", "31/07/2026"},
			{"SALARIO_BRUTO", "Salario bruto del período", "₡650 000,00"},
			{"CCSS_OBRERO", "Deducción CCSS del trabajador", "₡70 395,00"},
			{"RENTA", "Impuesto al salario retenido", "₡0,00"},
			{"OTRAS_DEDUCCIONES", "Otras deducciones del período", "₡25 000,00"},
			{"ADELANTO", "Adelanto ya pagado en el período", "₡0,00"},
			{"NETO", "Neto que recibe", "₡554 605,00"},
		}, variablesEmpresa...),
		AsuntoDefault: "Boleta de pago — [PERIODO]",
		CuerpoDefault: `Estimado/a [NOMBRE_EMPLEADO],

Le compartimos el detalle de su pago correspondiente a [PERIODO].

Puesto: [PUESTO]
Cédula: [IDENTIFICACION]
Fecha de pago: [FECHA_PAGO]

Salario bruto: [SALARIO_BRUTO]
CCSS (trabajador): -[CCSS_OBRERO]
Impuesto al salario: -[RENTA]
Otras deducciones: -[OTRAS_DEDUCCIONES]
Adelanto ya recibido: -[ADELANTO]
NETO A RECIBIR: [NETO]

Si encuentra alguna diferencia, comuníquese con Recursos Humanos.

[NOMBRE_EMPRESA]`,
	},
	{
		Clave:       ClaveNominaVacaciones,
		Nombre:      "Aviso de vacaciones",
		Descripcion: "Se envía al colaborador cuando se le registra un disfrute de vacaciones, con su saldo actualizado.",
		Modulo:      "RRHH / Nómina",
		Variables: append([]Variable{
			{"NOMBRE_EMPLEADO", "Nombre del colaborador", "María Fernández Rojas"},
			{"DIAS", "Días de vacaciones registrados", "5"},
			{"FECHA_INICIO", "Primer día de vacaciones", "11/08/2026"},
			{"FECHA_FIN", "Último día de vacaciones", "15/08/2026"},
			{"SALDO_DIAS", "Saldo de días que le queda", "7,5"},
			{"OBSERVACIONES", "Observaciones del registro", "Solicitadas por el colaborador"},
		}, variablesEmpresa...),
		AsuntoDefault: "Vacaciones registradas — [FECHA_INICIO] a [FECHA_FIN]",
		CuerpoDefault: `Estimado/a [NOMBRE_EMPLEADO],

Le confirmamos el registro de sus vacaciones:

Días: [DIAS]
Del [FECHA_INICIO] al [FECHA_FIN]
Saldo de días disponible tras este disfrute: [SALDO_DIAS]

[OBSERVACIONES]

Recursos Humanos
[NOMBRE_EMPRESA]`,
	},
}

// TipoPorClave busca un tipo del catálogo.
func TipoPorClave(clave string) (Tipo, bool) {
	for _, t := range Catalogo {
		if t.Clave == clave {
			return t, true
		}
	}
	return Tipo{}, false
}

// Plantilla es el texto vigente de un tipo para una empresa.
type Plantilla struct {
	Clave  string `json:"clave"`
	Asunto string `json:"asunto"`
	Cuerpo string `json:"cuerpo"`
	// Personalizada: false = rige el texto de fábrica (la empresa no lo ha cambiado).
	Personalizada  bool   `json:"personalizada"`
	ActualizadoEn  string `json:"actualizado_en"`
	ActualizadoPor string `json:"actualizado_por"`
}

// TipoConPlantilla es lo que necesita la pantalla de edición: el tipo con su texto vigente.
type TipoConPlantilla struct {
	Tipo
	Vigente Plantilla `json:"vigente"`
}

// VariablesUsadas devuelve, sin repetir y en orden alfabético, las variables que aparecen en un
// texto.
func VariablesUsadas(textos ...string) []string {
	vistas := map[string]bool{}
	for _, t := range textos {
		for _, m := range reVariable.FindAllStringSubmatch(t, -1) {
			vistas[m[1]] = true
		}
	}
	out := make([]string, 0, len(vistas))
	for v := range vistas {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// Desconocidas devuelve las variables del texto que el tipo NO sabe llenar. Vacío = todo bien.
func (t Tipo) Desconocidas(textos ...string) []string {
	permitidas := map[string]bool{}
	for _, v := range t.Variables {
		permitidas[v.Nombre] = true
	}
	out := []string{}
	for _, v := range VariablesUsadas(textos...) {
		if !permitidas[v] {
			out = append(out, v)
		}
	}
	return out
}

// Render reemplaza las variables por sus valores. Una variable sin valor queda VACÍA, nunca
// como «[VARIABLE]»: al proveedor no le llega un marcador crudo.
func Render(texto string, valores map[string]string) string {
	return reVariable.ReplaceAllStringFunc(texto, func(m string) string {
		nombre := strings.Trim(m, "[]")
		return valores[nombre]
	})
}

// Ejemplos arma los valores de muestra del tipo, para la vista previa.
func (t Tipo) Ejemplos() map[string]string {
	out := make(map[string]string, len(t.Variables))
	for _, v := range t.Variables {
		out[v.Nombre] = v.Ejemplo
	}
	return out
}
