// Package rbac implementa el control de acceso por permiso × rol × empresa ([T3]).
// El token sigue llevando solo el rol; la autorización resuelve rol→permisos por
// request con caché corto, de modo que editar la matriz surte efecto casi en vivo.
package rbac

// RolAdmin es el superusuario: SIEMPRE tiene todos los permisos (bypass de la matriz),
// para que nadie se auto-bloquee al editar. Decisión del usuario (2026-07-16).
const RolAdmin = "ADMIN"

// PermisoDef es una entrada del catálogo de permisos.
type PermisoDef struct {
	Codigo      string
	Modulo      string
	Nombre      string
	Descripcion string
	Critico     bool
}

// Catalogo es el catálogo COMPLETO de permisos, derivado de los endpoints reales.
// El orden define el orden de despliegue en la matriz.
var Catalogo = []PermisoDef{
	// Bancos
	{"bancos.ver", "Bancos", "Ver Bancos", "Dashboard, movimientos, cuadre, análisis, proyecciones y catálogo (lectura)", false},
	{"bancos.importar", "Bancos", "Importar estados de cuenta", "Subir, previsualizar y confirmar archivos del banco", false},
	{"bancos.clasificar", "Bancos", "Clasificar movimientos", "Clasificar y reclasificar (individual y masivo)", false},
	{"bancos.reglas", "Bancos", "Gestionar reglas del motor", "Crear, editar, pausar y eliminar reglas", false},
	{"bancos.catalogo", "Bancos", "Gestionar catálogo", "Conceptos, clasificaciones, bancos, cuentas y visibilidad CxP", false},
	{"bancos.tc_registrar", "Bancos", "Registrar / sincronizar TC", "Cargar cotización manual y disparar sync BCCR", false},
	{"bancos.tc_congelar", "Bancos", "Congelar tipo de cambio", "Fija el TC del mes (inmutable)", true},
	{"bancos.traslados", "Bancos", "Emparejar traslados", "Emparejar y desemparejar traslados y overnight", false},
	{"bancos.saldos", "Bancos", "Capturar saldos diarios", "Registrar el saldo de cada cuenta bancaria del día (tesorería)", false},
	{"bancos.conciliar", "Bancos", "Conciliar cuentas bancarias", "Registrar partidas en tránsito y FIRMAR el acta de conciliación del mes (habilita el cierre)", true},
	{"bancos.saldos_revisar", "Bancos", "Revisar y congelar saldos del día", "Dirección Financiera aprueba el saldo capturado; congelado, nadie lo edita sin descongelarlo", true},
	{"bancos.exportar", "Bancos", "Exportar reportes", "Descargar .xlsx de movimientos y cuadre", false},
	{"bancos.cerrar_periodo", "Bancos", "Cerrar período", "Cierra el mes contable", true},
	{"bancos.ajustes", "Bancos", "Editar ajustes de la empresa", "Tolerancia de traslado y parámetros", false},
	// Cuentas por pagar
	{"cxp.ver", "Cuentas por pagar", "Ver CxP", "Bandeja y proveedores (lectura)", false},
	{"cxp.ver_todo", "Cuentas por pagar", "Ver todas las áreas", "Ve las facturas de TODOS los departamentos; sin este permiso el usuario solo ve las de su(s) área(s) asignada(s)", false},
	{"cxp.dashboard", "Cuentas por pagar", "Ver dashboard de CxP", "KPIs y tablero del proceso de cuentas por pagar", false},
	{"cxp.cartera_abierta", "Cuentas por pagar", "Ver la cartera abierta completa", "TODO lo que la empresa debe, sin importar la fase ni el departamento (dato sensible: es la deuda total)", true},
	{"cxp.clasificar", "Cuentas por pagar", "Clasificar / priorizar facturas", "Segmentar gasto, tipo y prioridad", false},
	{"cxp.revisar", "Cuentas por pagar", "Revisar facturas", "Marcar como revisadas", false},
	{"cxp.aprobar", "Cuentas por pagar", "Aprobar facturas", "Aprobación según matriz de montos", true},
	{"cxp.tesoreria", "Cuentas por pagar", "Programar y pagar", "Programar, generar lote/macro, pagar y conciliar", true},
	{"cxp.comprobante", "Cuentas por pagar", "Gestionar comprobantes", "Adjuntar y enviar el comprobante al proveedor", false},
	{"cxp.proveedores", "Cuentas por pagar", "Gestionar proveedores", "Crear, editar y desactivar proveedores", false},
	{"cxp.importar", "Cuentas por pagar", "Importar facturación", "Cargar el Excel de facturación electrónica", false},
	{"cxp.anticipos", "Cuentas por pagar", "Aplicar anticipos", "Netear anticipos del proveedor contra la factura (y reversar antes del pago)", false},
	{"cxp.caja_ver", "Cuentas por pagar", "Ver caja chica", "Fondos de caja chica y sus vales (el custodio sin ver-todo solo ve su fondo)", false},
	{"cxp.caja_vale", "Cuentas por pagar", "Registrar vales de caja chica", "Registrar y anular vales del fondo (custodio o Contabilidad)", false},
	{"cxp.caja_reponer", "Cuentas por pagar", "Generar reposición de caja chica", "Agrupar los vales pendientes en el documento de reposición al custodio", false},
	{"cxp.caja_administrar", "Cuentas por pagar", "Administrar fondos de caja chica", "Constituir/editar fondos: montos, umbral, límite por vale y custodio", true},
	{"cxp.validar_depto", "Cuentas por pagar", "Validar facturas de su departamento", "El área confirma la conformidad de sus facturas antes del pago", false},
	{"cxp.validar_escalado", "Cuentas por pagar", "Validar por escalamiento", "Validar en lugar del área cuando no hay validador o la factura está vencida (queda como escalamiento)", true},
	{"cxp.aprobar_contabilidad", "Cuentas por pagar", "Aprobar facturas de Contabilidad", "Aprobar las facturas marcadas como de Contabilidad sin que pasen por la validación de área (la matriz de firmas por monto se sigue aplicando)", true},
	{"cxp.marcar_contabilidad", "Cuentas por pagar", "Marcar facturas como de Contabilidad", "Marcar (o desmarcar) una factura, un proveedor o un rubro como «de Contabilidad»: sus facturas no requieren validación de área", false},
	{"cxp.departamentos", "Cuentas por pagar", "Administrar departamentos", "Crear/editar departamentos y asignar validadores", false},
	{"cxp.parametros", "Cuentas por pagar", "Configurar los umbrales de validación", "Define desde qué monto y en qué casos una factura requiere que el área confirme la conformidad (cambia cuánto gasto se paga sin revisión humana)", true},
	// Cuentas por cobrar — la cartera de 70 000+ contratos. El alcance de datos del
	// operador es su SEDE: sin cxc.ver_todas_sedes solo ve la cartera que le asignaron.
	{"cxc.ver", "Cuentas por cobrar", "Ver CxC", "Cola de cobro, contratos y cargos (lectura)", false},
	{"cxc.ver_todas_sedes", "Cuentas por cobrar", "Ver todas las sedes", "Ve la cartera de TODAS las sedes; sin este permiso solo la de su(s) sede(s) asignada(s)", false},
	{"cxc.gestionar", "Cuentas por cobrar", "Registrar gestión de cobro", "Llamadas, mensajes, resultados y promesas de pago", false},
	{"cxc.cobros", "Cuentas por cobrar", "Registrar cobros", "Capturar cobros y subir planillas de asociación", true},
	{"cxc.aplicar", "Cuentas por cobrar", "Aplicar y reversar cobros", "Aplicar a cargos por excepción, identificar depósitos y reversar (cheque devuelto, débito rechazado)", true},
	{"cxc.notas_credito", "Cuentas por cobrar", "Emitir notas de crédito", "Condonar, descontar o corregir un cargo (no edita el original: emite un documento)", true},
	{"cxc.arreglos", "Cuentas por cobrar", "Autorizar arreglos de pago", "Pactar plazos fuera de los estándar (1-3-6-9), quebrar un arreglo incumplido y anular uno mal pactado", false},
	{"cxc.preventivo", "Cuentas por cobrar", "Contacto preventivo", "Ver y gestionar la lista de contratos al día cuya cuota está por vencer (recordatorio antes del vencimiento)", false},
	{"cxc.importar", "Cuentas por cobrar", "Importar cartera y cobros", "Cargar los archivos del sistema de origen y confirmarlos", true},
	{"cxc.suspender", "Cuentas por cobrar", "Suspender el servicio por mora", "Cortar el servicio a un contrato que llegó al tope de cuotas vencidas, y reactivarlo", true},
	{"cxc.parametros", "Cuentas por cobrar", "Configurar parámetros de cobro", "Tramos, probabilidades, factores por forma de pago y sedes", true},
	// RRHH / Nómina — dato sensible (salarios): por defecto solo el DF; el resto se
	// concede rol por rol desde la matriz.
	{"rrhh.ver", "RRHH / Nómina", "Ver RRHH / Nómina", "Empleados, deducciones, parámetros y conceptos (lectura)", false},
	{"rrhh.empleados", "RRHH / Nómina", "Gestionar empleados", "Crear/editar fichas, salarios y deducciones recurrentes", false},
	{"rrhh.parametros", "RRHH / Nómina", "Configurar parámetros de nómina", "Cargas sociales, tramos de renta y conceptos (versionado por año)", true},
	{"rrhh.corrida", "RRHH / Nómina", "Correr y aprobar nómina", "Calcular, aprobar y pagar la corrida quincenal; generar el archivo de pago", true},
	{"rrhh.finiquito", "RRHH / Nómina", "Liquidar cese (finiquito)", "Calcular, aprobar y pagar liquidaciones de cese conforme al Código de Trabajo", true},
	{"rrhh.ausencias", "RRHH / Nómina", "Registrar incapacidades y vacaciones", "Anotar incapacidades CCSS/INS y el disfrute de vacaciones que alimentan la corrida", false},
	// Administración
	{"admin.roles", "Administración", "Roles y permisos", "Configurar esta matriz y asignar roles a usuarios", true},
	{"admin.plantillas", "Administración", "Plantillas de notificaciones", "Editar el texto de los correos que el sistema envía (comprobante al proveedor, boleta de pago, vacaciones)", false},
}

// codigos devuelve todos los códigos de permiso del catálogo.
func codigos() []string {
	out := make([]string, len(Catalogo))
	for i, p := range Catalogo {
		out[i] = p.Codigo
	}
	return out
}

// MatrizDefault son las concesiones por defecto de los roles base no-ADMIN.
// ADMIN no aparece: tiene bypass (todo). Validado con el usuario a partir de la maqueta.
//
// Todo rol que deba existir en TODAS las empresas tiene que estar acá: esta matriz es la que
// siembra una empresa nueva y la que recorre `aplicar-faltantes`. Un rol creado solo en una
// migración queda congelado con los permisos de ese día.
var MatrizDefault = map[string][]string{
	// Director Financiero: acceso total (incluye administrar la matriz).
	"DIRECTOR_FINANCIERO": codigos(),
	// Supervisor: operación de Bancos y CxP, sin congelar TC, ajustes ni aprobar/pagar.
	"SUPERVISOR_FINANCIERO": {
		"bancos.ver", "bancos.importar", "bancos.clasificar", "bancos.reglas", "bancos.catalogo",
		"bancos.tc_registrar", "bancos.traslados", "bancos.saldos", "bancos.conciliar", "bancos.exportar", "bancos.cerrar_periodo",
		"cxp.ver", "cxp.ver_todo", "cxp.dashboard", "cxp.cartera_abierta", "cxp.clasificar", "cxp.revisar", "cxp.validar_depto", "cxp.tesoreria", "cxp.comprobante", "cxp.importar", "cxp.anticipos",
		// Facturas de Contabilidad: el Supervisor las marca y las aprueba. Sigue SIN "cxp.aprobar"
		// general —no firma el gasto de las áreas— pero sí resuelve el gasto que no tiene área que
		// lo valide (honorarios contables, timbres, comisiones bancarias, Hacienda).
		"cxp.marcar_contabilidad", "cxp.aprobar_contabilidad",
		"cxp.caja_ver", "cxp.caja_vale", "cxp.caja_reponer",
		"cxc.ver", "cxc.ver_todas_sedes", "cxc.gestionar", "cxc.cobros", "cxc.aplicar", "cxc.arreglos", "cxc.preventivo", "cxc.suspender", "cxc.importar",
	},
	// Auxiliar: captura y clasificación; sin aprobaciones, congelamiento ni cierre.
	// Incluye el saldo diario: es trabajo de captura de tesorería, no una decisión.
	"AUXILIAR_FINANCIERO": {
		"bancos.ver", "bancos.importar", "bancos.clasificar", "bancos.tc_registrar", "bancos.saldos",
		// Revisa: marcar una factura como revisada es el trabajo diario del auxiliar de
		// contabilidad —clasificarla y darla por buena para que siga—. No firma ni paga:
		// aprobar y tesorería siguen fuera de este rol. Decisión del usuario (2026-08-14).
		"cxp.ver", "cxp.ver_todo", "cxp.dashboard", "cxp.clasificar", "cxp.revisar", "cxp.comprobante", "cxp.proveedores", "cxp.importar", "cxp.anticipos",
		// Marca pero NO aprueba: marcar es parte de segmentar la factura, aprobar es firmar.
		"cxp.marcar_contabilidad",
		"cxp.caja_ver", "cxp.caja_vale", "cxp.caja_reponer",
		"cxc.ver", "cxc.gestionar", "cxc.cobros", "cxc.preventivo",
	},
	// Gerencia General: lectura + validación de área + escalamiento + aprobación (firma mancomunada).
	"GERENCIA_GENERAL": {"bancos.ver", "cxp.ver", "cxp.ver_todo", "cxp.dashboard", "cxp.validar_depto", "cxp.validar_escalado", "cxp.aprobar", "cxp.aprobar_contabilidad", "cxp.parametros", "cxp.caja_ver", "cxc.ver", "cxc.ver_todas_sedes"},
	// Auditor Interno: solo lectura.
	"AUDITOR_INTERNO": {"bancos.ver", "cxp.ver", "cxp.ver_todo", "cxp.dashboard", "cxp.caja_ver", "cxc.ver", "cxc.ver_todas_sedes"},
	// Supervisor de Piso: la autoridad de la operación de cobro. Es un cargo real que el
	// negocio nombró como quien autoriza las excepciones — notas de crédito sin tope, arreglos
	// a plazo largo y el corte del servicio por mora.
	//
	// Estaba SOLO en la migración 0054 y no acá, y eso era un hueco: una empresa nueva nacía
	// con el rol pero sin ningún permiso, y `aplicar-faltantes` nunca se lo iba a dar porque
	// solo recorre esta matriz. Ahora se mantiene por el mismo camino que los demás.
	// Sin `cxc.ver_todas_sedes` a propósito: «supervisor de PISO» nombra a la autoridad de una
	// plaza, así que su alcance de datos son la(s) sede(s) que le asignen. Si el negocio quiere
	// que vea toda la cartera, se marca desde la pantalla; no es el default.
	"SUPERVISOR_PISO": {
		"cxc.ver", "cxc.gestionar", "cxc.preventivo",
		"cxc.notas_credito", "cxc.arreglos", "cxc.suspender",
	},
}

// PermisosRolNuevo son los permisos con que nace un rol a medida (lo mínimo: ver).
var PermisosRolNuevo = []string{"bancos.ver", "cxp.ver"}
