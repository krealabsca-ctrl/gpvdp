package cxc

// El encabezado del sistema de origen viene en snake_case (8-oct-2026).
//
// El archivo real —«Cartera 17092026.xlsx», 23.144 contratos— exporta así: `fecha_primer_cobro`,
// `sede_contrato`, `modalidad_de_cobro`, `dia_de_pago`, `forma_pago`, `vencimiento_tarjeta`. El
// normalizador bajaba a minúsculas y quitaba tildes, pero trataba el guion bajo como una letra
// más, así que NINGUNO de esos encabezados calzaba con su alias.
//
// El efecto no era un error: era silencio. Las columnas entraban VACÍAS y las 23.144 filas caían
// en cuarentena con «sin fecha de primer cobro», «sin modalidad de cobro» y «sin sede» —por datos
// que el archivo traía puestos—. El usuario miraba su Excel al lado de la pantalla: «no me está
// cuadrando esto y sí tiene datos».
//
// Medido sobre el archivo real: cuarentena 23.144 → 35, sin sede 23.144 → 0. Las 35 que quedan
// son problemas de verdad (24 cuotas en cero, 10 por encima del máximo, 1 sin ninguna fecha).

import (
	"strconv"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestNormalizarEncabezadoParteElSnakeCase(t *testing.T) {
	casos := map[string]string{
		"fecha_primer_cobro":  "fecha primer cobro",
		"sede_contrato":       "sede contrato",
		"modalidad_de_cobro":  "modalidad de cobro",
		"dia_de_pago":         "dia de pago",
		"forma_pago":          "forma pago",
		"vencimiento_tarjeta": "vencimiento tarjeta",
		"tipo_servicio":       "tipo servicio",
		// El guion también separa, y los espacios de más se colapsan igual que antes.
		"Fecha-Primer-Cobro":     "fecha primer cobro",
		"  Cuota  ":              "cuota",
		"Días de pagos":          "dias de pagos",
		"CONTRATO FECHA INICIAL": "contrato fecha inicial",
	}
	for crudo, quiere := range casos {
		if got := normalizarEncabezado(crudo); got != quiere {
			t.Errorf("normalizarEncabezado(%q) = %q, se esperaba %q", crudo, got, quiere)
		}
	}
}

// TODO alias tiene que estar escrito como SE COMPARA, o no puede calzar nunca.
//
// Esta prueba existe porque ya pasó dos veces: diez alias con tilde en el lector de pagos de
// bancos, y acá «cédula», «teléfonos», «asociación» —el encabezado se normaliza sin tildes antes
// de comparar, así que esos alias eran letra muerta que parecía cubrir un caso—. Ahora el alias se
// normaliza con la misma función, y esto lo deja fijo: un alias que cambie al normalizarse
// significa que alguien lo escribió pensando en cómo se ve, no en cómo se compara.
func TestTodoAliasEstaEscritoComoSeCompara(t *testing.T) {
	for campo, lista := range aliasContrato {
		for _, a := range lista {
			if n := normalizarEncabezado(a); n != a {
				t.Errorf("cxc/contratos: el alias %q de %q se normaliza a %q; escribilo así para "+
					"que se lea igual que se compara", a, campo, n)
			}
		}
	}
	for campo, lista := range aliasCobro {
		for _, a := range lista {
			if n := normalizarEncabezado(a); n != a {
				t.Errorf("cxc/cobros: el alias %q de %q se normaliza a %q", a, campo, n)
			}
		}
	}
}

// encabezadoDelOrigen son las 31 columnas del archivo real, en su orden y con su ortografía
// —«asocaicion» incluido, que así viene—.
var encabezadoDelOrigen = []string{
	"documento", "nombre", "primer_apellido", "segundo_apellido", "Contrato",
	"contrato_codigo_id_fm", "fecha_servicio_creado", "fecha_inicio_contrato",
	"fecha_primer_cobro", "sede_contrato", "forma_pago", "asociacion_siglas", "asocaicion",
	"modalidad_de_cobro", "dia_de_pago", "vendedor", "empleado_usuario_creador", "ruta",
	"vencimiento_tarjeta", "contrato_estado", "tipo_servicio", "servicio",
	"impuesto_porcentaje", "impuesto_incluido_en_valor", " Cuota ",
	"contrato_servicio_estado", "Gestor", "Fecha del proximo pago", "Monto", "Estado Cobro",
	"Nueva Fecha de Proximo pago",
}

// Las columnas que el archivo real trae y que el importador TIENE que encontrar.
func TestElEncabezadoDelOrigenSeReconoceEntero(t *testing.T) {
	cols, fila := columnasContrato(Grid{encabezadoDelOrigen})
	if cols == nil {
		t.Fatal("no se reconoció el encabezado del archivo real")
	}
	if fila != 0 {
		t.Fatalf("encabezado detectado en la fila %d", fila)
	}
	quiere := map[string]int{
		"contrato":      4,
		"cliente":       1,
		"documento":     0,
		"fecha_inicial": 7,
		"primer_cobro":  8,
		"sede":          9,
		"forma_pago":    10,
		"asociacion":    11,
		"modalidad":     13,
		"dia_pago":      14,
		"tarjeta_vence": 18,
		"estado":        19,
		"tipo_servicio": 20,
		"servicio":      21,
		"cuota":         24,
	}
	for campo, i := range quiere {
		if cols[campo] != i {
			t.Errorf("la columna %q quedó en %d y está en %d (%q): entra vacía y la fila cae en "+
				"cuarentena por un dato que el archivo trae", campo, cols[campo], i, encabezadoDelOrigen[i])
		}
	}
}

// El camino completo con el encabezado del origen y una fecha de Excel de verdad: la fila NO puede
// quedar en cuarentena.
func TestUnaFilaDelArchivoRealNoCaeEnCuarentena(t *testing.T) {
	fila := make([]string, len(encabezadoDelOrigen))
	fila[4] = "CO127269"
	fila[1] = "RAUL"
	fila[0] = "113940337"
	fila[9] = "SAN JOSÉ - VALLE DE PAZ DE COSTA RICA SA"
	fila[10] = "Débito Automático"
	fila[13] = "Mensual"
	fila[14] = "4"
	fila[24] = "4950"
	crudas := make([]string, len(fila))
	copy(crudas, fila)
	// La fecha, como la guarda Excel: serie en el crudo y el render del formato en lo visible.
	fila[8], crudas[8] = "02-01-22", "44593"

	h := Hoja{
		Valores: Grid{encabezadoDelOrigen, fila},
		Crudos:  Grid{encabezadoDelOrigen, crudas},
	}
	filas, err := LeerContratos(h, ReglasImportacion{CuotaMaxima: decimal.NewFromInt(500000)})
	if err != nil {
		t.Fatalf("LeerContratos: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("se leyeron %d contratos, se esperaba 1", len(filas))
	}
	f := filas[0]
	if len(f.Motivos) > 0 {
		t.Errorf("la fila quedó en cuarentena por %v, y el archivo trae todos esos datos", f.Motivos)
	}
	if f.PrimerCobro != "2022-02-01" {
		t.Errorf("primer cobro = %q, se esperaba 2022-02-01", f.PrimerCobro)
	}
	if f.Modalidad != "Mensual" {
		t.Errorf("modalidad = %q, se esperaba «Mensual»", f.Modalidad)
	}
	if f.DiaPago != 4 {
		t.Errorf("día de pago = %d, se esperaba 4", f.DiaPago)
	}
	if f.SedeCruda == "" {
		t.Error("la sede entró vacía")
	}
	// La sede trae la razón social pegada y se separa por el sufijo societario.
	if f.RazonSocial != "VALLE DE PAZ DE COSTA RICA SA" || f.Plaza != "SAN JOSÉ" {
		t.Errorf("sede partida en razón social %q y plaza %q", f.RazonSocial, f.Plaza)
	}
}

// El vencimiento de la tarjeta vale hasta el ÚLTIMO día de su mes.
//
// El origen escribe «ago-23» y Excel lo guarda como el 1 de agosto. Quedarse con ese día le
// quita a la tarjeta el mes de vigencia que la regla del campo le da, y el cobro se cortaría
// treinta días antes. Los 12 contratos del archivo real que traen vencimiento caen los 12 en
// día 1, que es justamente la firma de un mes-año convertido.
func TestElVencimientoDeTarjetaLlegaAFinDeMes(t *testing.T) {
	// La serie se calcula, no se adivina: para cualquier fecha moderna es la cantidad de días
	// desde el 30 de diciembre de 1899 (el origen desplazado que arrastra Excel por el año
	// bisiesto de 1900 que nunca existió).
	origen := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	serieDe := func(a int, m time.Month, d int) string {
		dias := time.Date(a, m, d, 0, 0, 0, 0, time.UTC).Sub(origen).Hours() / 24
		return strconv.Itoa(int(dias))
	}
	// Control de la cuenta contra el caso ya medido sobre el archivo real.
	if s := serieDe(2022, 2, 1); s != "44593" {
		t.Fatalf("la serie del 1-feb-2022 dio %s y Excel guarda 44593: la cuenta de la prueba "+
			"está mal y lo que mida no significa nada", s)
	}
	// Un mes de 31, uno de 30 y un febrero bisiesto: que el último día no salga bien por
	// casualidad. En los tres, el origen escribió el MES y Excel le puso el día 1.
	casos := map[string]string{
		serieDe(2023, 8, 1):  "2023-08-31",
		serieDe(2023, 9, 1):  "2023-09-30",
		serieDe(2024, 2, 1):  "2024-02-29",
		serieDe(2026, 12, 1): "2026-12-31",
	}
	for serie, quiere := range casos {
		if got := finDeMesDeCelda("", serie); got != quiere {
			t.Errorf("serie %s → %q, se esperaba %q: una tarjeta que vence en ese mes sirve "+
				"todo el mes", serie, got, quiere)
		}
	}
	// El camino del TEXTO sigue como estaba: el mes-año se lleva a fin de mes…
	if got := finDeMesDeCelda("oct-28", ""); got != "2028-10-31" {
		t.Errorf("«oct-28» = %q, se esperaba 2028-10-31", got)
	}
	// …y una fecha completa escrita como texto se respeta tal cual, que es lo que ya hacía.
	if got := finDeMesDeCelda("15/8/2023", ""); got != "2023-08-15" {
		t.Errorf("«15/8/2023» = %q, se esperaba 2023-08-15 (el camino del texto no cambió)", got)
	}
}
