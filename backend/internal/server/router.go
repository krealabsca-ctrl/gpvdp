// Package server arma el router HTTP (Gin) y su middleware.
package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/bancos"
	"github.com/gpvdp/erp/internal/config"
	"github.com/gpvdp/erp/internal/cxc"
	"github.com/gpvdp/erp/internal/cxp"
	"github.com/gpvdp/erp/internal/nomina"
	"github.com/gpvdp/erp/internal/plantillas"
	"github.com/gpvdp/erp/internal/rbac"
	"github.com/gpvdp/erp/internal/tenant"
)

// NewRouter construye el motor Gin. `perms` es el checker RBAC (permiso × rol × empresa).
func NewRouter(cfg config.Config, log *zap.Logger, authH *auth.Handler, bancosH *bancos.Handler, cxpH *cxp.Handler, cxcH *cxc.Handler, nominaH *nomina.Handler, rbacH *rbac.Handler, plantillasH *plantillas.Handler, perms tenant.PermisoChecker) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery(), requestLogger(log), cors(cfg.CORSOrigins))

	// P(permiso) exige ese permiso vía la matriz RBAC (deny-by-default; ADMIN bypass).
	P := func(permiso string) gin.HandlerFunc { return tenant.RequirePermiso(perms, permiso) }

	v1 := r.Group("/v1")
	v1.GET("/healthz", health)

	// Públicos
	v1.POST("/auth/login", authH.Login)
	v1.POST("/auth/refresh", authH.Refresh)

	// Requieren access token
	authed := v1.Group("")
	authed.Use(tenant.RequireAuth(cfg.JWTSecret))
	{
		authed.POST("/auth/select-empresa", authH.SelectEmpresa)
		authed.POST("/auth/cambiar-password", authH.CambiarPassword)
		authed.GET("/me", authH.Me)
		authed.GET("/empresas", authH.Empresas)

		// Requieren empresa seleccionada (aislamiento por tenant).
		scoped := authed.Group("")
		scoped.Use(tenant.RequireEmpresa())
		{
			scoped.GET("/empresas/actual", authH.EmpresaActual)

			// ── Módulo Bancos ──
			// Lectura (bancos.ver)
			scoped.GET("/bancos/cuentas", P("bancos.ver"), bancosH.Cuentas)
			scoped.GET("/bancos/movimientos", P("bancos.ver"), bancosH.Movimientos)
			// Resumen de la selección activa: mismos filtros que la lista, agregados.
			scoped.GET("/bancos/movimientos/resumen", P("bancos.ver"), bancosH.ResumenSeleccion)
			scoped.GET("/bancos/reglas", P("bancos.ver"), bancosH.Reglas)
			scoped.GET("/bancos/reglas/sugerencia", P("bancos.ver"), bancosH.SugerenciaRegla)
			scoped.GET("/bancos/clasificacion/resumen", P("bancos.ver"), bancosH.ResumenClasificacion)
			scoped.GET("/bancos/catalogo/conceptos", P("bancos.ver"), bancosH.Conceptos)
			scoped.GET("/bancos/catalogo/clasificaciones", P("bancos.ver"), bancosH.Clasificaciones)
			scoped.GET("/bancos/catalogo/bancos", P("bancos.ver"), bancosH.Bancos)
			scoped.GET("/bancos/tipo-cambio/:anio/:mes", P("bancos.ver"), bancosH.EstadoTC)
			scoped.GET("/bancos/tipo-cambio/ultimo-sync", P("bancos.ver"), bancosH.UltimoSyncBCCR)
			scoped.GET("/bancos/parametros", P("bancos.ver"), bancosH.Parametros)
			scoped.GET("/bancos/cuadre", P("bancos.ver"), bancosH.Cuadre)
			scoped.GET("/bancos/cuadre/arbol", P("bancos.ver"), bancosH.CuadreArbol)
			scoped.GET("/bancos/dashboard", P("bancos.ver"), bancosH.Dashboard)
			scoped.GET("/bancos/analisis/serie-mensual", P("bancos.ver"), bancosH.SerieMensual)
			scoped.GET("/bancos/analisis/calendario", P("bancos.ver"), bancosH.CalendarioDiario)
			scoped.GET("/bancos/analisis/cuentas", P("bancos.ver"), bancosH.ResumenPorCuenta)
			scoped.GET("/bancos/analisis/partidas", P("bancos.ver"), bancosH.AnalisisPartidas)
			scoped.GET("/bancos/proyecciones", P("bancos.ver"), bancosH.Proyeccion)
			scoped.POST("/bancos/proyecciones", P("bancos.ver"), bancosH.GuardarEscenario)
			scoped.GET("/bancos/proyecciones/escenarios", P("bancos.ver"), bancosH.Escenarios)
			scoped.GET("/bancos/traslados/propuestas", P("bancos.ver"), bancosH.PropuestasTraslados)
			// Tesorería: saldo diario por cuenta y checklist de carga del mes
			scoped.GET("/bancos/tesoreria", P("bancos.ver"), bancosH.Tesoreria)
			scoped.PUT("/bancos/saldos", P("bancos.saldos"), bancosH.GuardarSaldos)
			scoped.GET("/bancos/carga", P("bancos.ver"), bancosH.CargaDelPeriodo)
			// Conciliación bancaria mensual: acta por cuenta, partidas en tránsito y firma.
			// Quien captura el saldo no firma el acta ni congela el día (segregación).
			// Patrones: agrupa lo que quedó sin clasificar y propone la regla de cada grupo.
			scoped.GET("/bancos/patrones", P("bancos.ver"), bancosH.Patrones)
			// Huella Bancos↔CxP: empareja los pagos del banco con su factura. Corre solo al
			// importar; el endpoint sirve para repetirlo sobre lo ya cargado.
			scoped.POST("/bancos/conciliacion-cxp", P("cxp.tesoreria"), bancosH.ConciliarCxP)
			scoped.GET("/bancos/conciliacion", P("bancos.ver"), bancosH.Conciliacion)
			scoped.POST("/bancos/conciliacion/partidas", P("bancos.conciliar"), bancosH.RegistrarPartida)
			scoped.DELETE("/bancos/conciliacion/partidas/:id", P("bancos.conciliar"), bancosH.AnularPartida)
			scoped.POST("/bancos/conciliacion/firmar", P("bancos.conciliar"), bancosH.FirmarActa)
			scoped.POST("/bancos/saldos/revisar", P("bancos.saldos_revisar"), bancosH.RevisarSaldos)
			scoped.GET("/bancos/periodos/:anio/:mes", P("bancos.ver"), bancosH.EstadoPeriodo)
			// Importar
			scoped.POST("/bancos/importaciones", P("bancos.importar"), bancosH.Subir)
			scoped.GET("/bancos/importaciones/:id/preview", P("bancos.importar"), bancosH.Preview)
			scoped.POST("/bancos/importaciones/:id/confirmar", P("bancos.importar"), bancosH.Confirmar)
			// Clasificar
			scoped.PATCH("/bancos/movimientos/:id/clasificacion", P("bancos.clasificar"), bancosH.Reclasificar)
			scoped.POST("/bancos/movimientos/clasificar-masivo", P("bancos.clasificar"), bancosH.ClasificarMasivo)
			// Traer a bloque la clasificación hecha en Excel: la plantilla se baja, se llena y se sube.
			scoped.GET("/bancos/movimientos/plantilla-clasificacion", P("bancos.exportar"), bancosH.PlantillaClasificacion)
			scoped.POST("/bancos/movimientos/clasificar-excel", P("bancos.clasificar"), bancosH.ImportarClasificacionExcel)
			// Reglas del motor
			scoped.POST("/bancos/reglas", P("bancos.reglas"), bancosH.CrearRegla)
			scoped.PATCH("/bancos/reglas/:id", P("bancos.reglas"), bancosH.ActualizarRegla)
			scoped.DELETE("/bancos/reglas/:id", P("bancos.reglas"), bancosH.EliminarRegla)
			// Catálogo
			// Diccionario del catálogo: Concepto › Clasificación + palabras clave, portable.
			// Importar exige bancos.reglas porque una fila con palabras clave crea una regla.
			scoped.GET("/bancos/catalogo/diccionario", P("bancos.exportar"), bancosH.ExportarDiccionario)
			scoped.POST("/bancos/catalogo/diccionario", P("bancos.reglas"), bancosH.ImportarDiccionario)
			scoped.POST("/bancos/catalogo/conceptos", P("bancos.catalogo"), bancosH.CrearConcepto)
			scoped.PATCH("/bancos/catalogo/conceptos/:id", P("bancos.catalogo"), bancosH.RenombrarConcepto)
			scoped.DELETE("/bancos/catalogo/conceptos/:id", P("bancos.catalogo"), bancosH.EliminarConcepto)
			scoped.POST("/bancos/catalogo/clasificaciones", P("bancos.catalogo"), bancosH.CrearClasificacion)
			scoped.PATCH("/bancos/catalogo/clasificaciones/:id", P("bancos.catalogo"), bancosH.RenombrarClasificacion)
			scoped.DELETE("/bancos/catalogo/clasificaciones/:id", P("bancos.catalogo"), bancosH.EliminarClasificacion)
			scoped.POST("/bancos/catalogo/bancos", P("bancos.catalogo"), bancosH.CrearBanco)
			scoped.PATCH("/bancos/catalogo/bancos/:id", P("bancos.catalogo"), bancosH.RenombrarBanco)
			scoped.POST("/bancos/catalogo/cuentas", P("bancos.catalogo"), bancosH.CrearCuenta)
			scoped.PATCH("/bancos/catalogo/cuentas/:id", P("bancos.catalogo"), bancosH.ActualizarCuenta)
			// Corregir lo que se creó mal: eliminar si no tiene nada colgando, desactivar si
			// sí, y fusionar dos entradas del catálogo moviéndoles todo lo que las usa.
			scoped.DELETE("/bancos/catalogo/bancos/:id", P("bancos.catalogo"), bancosH.EliminarBanco)
			scoped.POST("/bancos/catalogo/bancos/:id/activo", P("bancos.catalogo"), bancosH.CambiarActivoBanco)
			scoped.GET("/bancos/catalogo/cuentas/:id/uso", P("bancos.ver"), bancosH.UsoDeCuenta)
			scoped.DELETE("/bancos/catalogo/cuentas/:id", P("bancos.catalogo"), bancosH.EliminarCuenta)
			scoped.POST("/bancos/catalogo/cuentas/:id/activo", P("bancos.catalogo"), bancosH.CambiarActivoCuenta)
			scoped.POST("/bancos/catalogo/conceptos/:id/fusionar", P("bancos.catalogo"), bancosH.FusionarConcepto)
			scoped.POST("/bancos/catalogo/clasificaciones/:id/fusionar", P("bancos.catalogo"), bancosH.FusionarClasificacion)
			// Tipo de cambio
			scoped.POST("/bancos/cotizaciones", P("bancos.tc_registrar"), bancosH.RegistrarCotizacion)
			scoped.POST("/bancos/tipo-cambio/sync", P("bancos.tc_registrar"), bancosH.SincronizarBCCR)
			scoped.POST("/bancos/tipo-cambio/:anio/:mes/congelar", P("bancos.tc_congelar"), bancosH.CongelarTC)
			// Ajustes
			scoped.PATCH("/bancos/parametros/tolerancia", P("bancos.ajustes"), bancosH.ActualizarTolerancia)
			// Exportar
			scoped.GET("/bancos/exportaciones/movimientos", P("bancos.exportar"), bancosH.ExportarMovimientos)
			scoped.GET("/bancos/exportaciones/cuadre", P("bancos.exportar"), bancosH.ExportarCuadre)
			// Traslados
			scoped.POST("/bancos/traslados/emparejar", P("bancos.traslados"), bancosH.EmparejarTraslado)
			scoped.POST("/bancos/traslados/desemparejar", P("bancos.traslados"), bancosH.DesemparejarTraslado)
			// Cerrar período
			scoped.POST("/bancos/periodos/:anio/:mes/cerrar", P("bancos.cerrar_periodo"), bancosH.CerrarPeriodo)

			// ── Módulo CxP ──
			// Lectura (cxp.ver)
			scoped.GET("/cxp/dashboard", P("cxp.dashboard"), cxpH.Dashboard)
			scoped.GET("/cxp/bandeja", P("cxp.ver"), cxpH.Bandeja)
			scoped.GET("/cxp/catalogo/subclasificaciones", P("cxp.ver"), cxpH.ListarSubclasificaciones)
			scoped.GET("/cxp/departamentos", P("cxp.ver"), cxpH.ListarDepartamentos)
			scoped.GET("/cxp/departamentos/:id/validadores", P("cxp.ver"), cxpH.ListarValidadores)
			scoped.GET("/cxp/usuarios", P("cxp.departamentos"), cxpH.ListarUsuarios)
			scoped.GET("/cxp/proveedores", P("cxp.ver"), cxpH.ListarProveedores)
			scoped.GET("/cxp/proveedores/:id", P("cxp.ver"), cxpH.ProveedorPorID)
			scoped.GET("/cxp/proveedores/:id/gastos", P("cxp.ver"), cxpH.GastosDeProveedor)
			scoped.GET("/cxp/documentos", P("cxp.ver"), cxpH.ListarDocumentos)
			scoped.GET("/cxp/documentos/:id", P("cxp.ver"), cxpH.DocumentoPorID)
			scoped.GET("/cxp/documentos/:id/historial", P("cxp.ver"), cxpH.HistorialDocumento)
			scoped.GET("/cxp/documentos/:id/comprobante", P("cxp.ver"), cxpH.DescargarComprobante)
			scoped.GET("/cxp/documentos/:id/anticipos", P("cxp.ver"), cxpH.AplicacionesDocumento)
			scoped.GET("/cxp/anticipos/disponibles", P("cxp.ver"), cxpH.AnticiposDisponibles)
			scoped.GET("/cxp/anticipos", P("cxp.ver"), cxpH.AnticiposEmpresa)
			scoped.GET("/cxp/lotes", P("cxp.ver"), cxpH.ListarLotes)
			// Clasificar / priorizar
			scoped.POST("/cxp/catalogo/subclasificaciones", P("cxp.clasificar"), cxpH.CrearSubclasificacion)
			scoped.PATCH("/cxp/documentos/:id/clasificacion", P("cxp.clasificar"), cxpH.ClasificarDocumento)
			scoped.POST("/cxp/documentos/clasificar-masivo", P("cxp.clasificar"), cxpH.ClasificarMasivo)
			scoped.POST("/cxp/documentos/tipo-masivo", P("cxp.clasificar"), cxpH.TipoMasivo)
			scoped.POST("/cxp/documentos/prioridad-masiva", P("cxp.clasificar"), cxpH.PrioridadMasiva)
			// Revisar (incluye transiciones de gestión: denegar/anular/liquidar/rebotar)
			scoped.POST("/cxp/documentos/:id/revisar", P("cxp.revisar"), cxpH.RevisarDocumento)
			scoped.POST("/cxp/documentos/transicion-masiva", P("cxp.revisar"), cxpH.TransicionMasiva)
			scoped.PATCH("/cxp/documentos/:id/departamento", P("cxp.clasificar"), cxpH.AsignarDepartamentoDocumento)
			// Validación por departamento (control operativo de área, previo a la aprobación financiera)
			scoped.POST("/cxp/documentos/:id/validar-depto", P("cxp.validar_depto"), cxpH.ValidarDeptoDocumento)
			scoped.POST("/cxp/documentos/:id/validar-escalado", P("cxp.validar_escalado"), cxpH.ValidarEscaladoDocumento)
			scoped.POST("/cxp/documentos/:id/devolver", P("cxp.validar_depto"), cxpH.DevolverDocumento)
			// Anticipos (netting): aplicar / reversar contra la factura (antes del pago)
			scoped.POST("/cxp/documentos/:id/anticipos", P("cxp.anticipos"), cxpH.AplicarAnticipoDocumento)
			scoped.POST("/cxp/documentos/:id/anticipos/lote", P("cxp.anticipos"), cxpH.AplicarAnticiposLoteDocumento)
			scoped.DELETE("/cxp/documentos/:id/anticipos/:aplicacionId", P("cxp.anticipos"), cxpH.ReversarAnticipoDocumento)
			// Aprobar
			scoped.POST("/cxp/documentos/:id/aprobar", P("cxp.aprobar"), cxpH.AprobarDocumento)
			// Facturas «de Contabilidad»: las que no tienen área operativa que las valide
			// (honorarios contables, timbres, comisiones bancarias, Hacienda, auditoría). Vía
			// propia porque el Supervisor Financiero no tiene `cxp.aprobar` y el middleware lo
			// cortaría antes de llegar a la regla. El servicio verifica que la factura ESTÉ
			// marcada, así que este permiso no sirve para aprobar cualquier otra.
			scoped.POST("/cxp/documentos/:id/aprobar-contabilidad", P("cxp.aprobar_contabilidad"), cxpH.AprobarDocumentoContabilidad)
			scoped.PATCH("/cxp/documentos/:id/contabilidad", P("cxp.marcar_contabilidad"), cxpH.MarcarDocumentoContabilidad)
			scoped.PATCH("/cxp/proveedores/:id/contabilidad", P("cxp.marcar_contabilidad"), cxpH.MarcarProveedorContabilidad)
			scoped.PATCH("/cxp/contabilidad/conceptos/:id", P("cxp.marcar_contabilidad"), cxpH.MarcarConceptoContabilidad)
			scoped.PATCH("/cxp/contabilidad/clasificaciones/:id", P("cxp.marcar_contabilidad"), cxpH.MarcarClasificacionContabilidad)
			scoped.GET("/cxp/contabilidad/marcas", P("cxp.ver"), cxpH.MarcasContabilidad)
			// Umbrales de la validación por riesgo: mover uno cambia cuánto gasto se paga sin
			// revisión humana, así que va detrás de su propio permiso crítico.
			scoped.GET("/cxp/parametros", P("cxp.ver"), cxpH.ParametrosValidacion)
			scoped.PUT("/cxp/parametros/:clave", P("cxp.parametros"), cxpH.GuardarParametroValidacion)
			// Tesorería: programar / pagar / conciliar / archivo / lotes
			scoped.POST("/cxp/documentos/:id/programar", P("cxp.tesoreria"), cxpH.ProgramarDocumento)
			scoped.POST("/cxp/documentos/:id/pagar", P("cxp.tesoreria"), cxpH.PagarDocumento)
			scoped.POST("/cxp/documentos/:id/conciliar", P("cxp.tesoreria"), cxpH.ConciliarDocumento)
			scoped.GET("/cxp/pagos/archivo", P("cxp.tesoreria"), cxpH.ArchivoPago)
			scoped.POST("/cxp/pagos/archivo", P("cxp.tesoreria"), cxpH.ArchivoPagoLote)
			scoped.POST("/cxp/conciliacion/match", P("cxp.tesoreria"), cxpH.ConciliarMatch)
			scoped.POST("/cxp/lotes", P("cxp.tesoreria"), cxpH.CrearLote)
			scoped.GET("/cxp/lotes/:id/macro", P("cxp.tesoreria"), cxpH.MacroLote)
			// Comprobantes
			scoped.POST("/cxp/documentos/:id/comprobante", P("cxp.comprobante"), cxpH.AdjuntarComprobante)
			scoped.POST("/cxp/documentos/:id/comprobante/enviar", P("cxp.comprobante"), cxpH.EnviarComprobante)
			// Proveedores
			scoped.POST("/cxp/proveedores", P("cxp.proveedores"), cxpH.CrearProveedor)
			scoped.PATCH("/cxp/proveedores/:id", P("cxp.proveedores"), cxpH.ActualizarProveedor)
			scoped.POST("/cxp/proveedores/:id/desactivar", P("cxp.proveedores"), cxpH.DesactivarProveedor)
			// Departamentos (catálogo administrable)
			scoped.POST("/cxp/departamentos", P("cxp.departamentos"), cxpH.CrearDepartamento)
			scoped.PATCH("/cxp/departamentos/:id", P("cxp.departamentos"), cxpH.ActualizarDepartamento)
			scoped.POST("/cxp/departamentos/:id/desactivar", P("cxp.departamentos"), cxpH.DesactivarDepartamento)
			scoped.POST("/cxp/departamentos/:id/validadores", P("cxp.departamentos"), cxpH.AsignarValidador)
			scoped.DELETE("/cxp/departamentos/:id/validadores/:usuarioId", P("cxp.departamentos"), cxpH.QuitarValidador)
			// Caja chica (fondo fijo): fondos, vales y reposición
			scoped.GET("/cxp/cajas", P("cxp.caja_ver"), cxpH.ListarFondos)
			scoped.POST("/cxp/cajas", P("cxp.caja_administrar"), cxpH.CrearFondo)
			scoped.PATCH("/cxp/cajas/:id", P("cxp.caja_administrar"), cxpH.ActualizarFondo)
			scoped.POST("/cxp/cajas/:id/desactivar", P("cxp.caja_administrar"), cxpH.DesactivarFondo)
			scoped.GET("/cxp/cajas/:id/vales", P("cxp.caja_ver"), cxpH.ListarVales)
			scoped.POST("/cxp/cajas/:id/vales", P("cxp.caja_vale"), cxpH.CrearVale)
			scoped.POST("/cxp/cajas/:id/vales/:valeId/anular", P("cxp.caja_vale"), cxpH.AnularVale)
			scoped.POST("/cxp/cajas/:id/reposicion", P("cxp.caja_reponer"), cxpH.GenerarReposicion)
			// Importar facturación (+ alta manual de documento)
			scoped.POST("/cxp/documentos", P("cxp.importar"), cxpH.CrearDocumento)
			// Carga masiva de IBAN: sin cuenta destino el banco rechaza la línea de la macro.
			scoped.GET("/cxp/proveedores/sin-iban", P("cxp.ver"), cxpH.ProveedoresSinIBAN)
			scoped.POST("/cxp/proveedores/iban/preview", P("cxp.proveedores"), cxpH.PrevisualizarIBAN)
			scoped.POST("/cxp/proveedores/iban", P("cxp.proveedores"), cxpH.CargarIBAN)
			scoped.POST("/cxp/importaciones", P("cxp.importar"), cxpH.SubirImportacion)
			scoped.POST("/cxp/importaciones/confirmar", P("cxp.importar"), cxpH.ConfirmarImportacion)

			// ── Módulo RRHH / Nómina ──
			// Dashboard del mes (costo real, ciclo, alertas)
			scoped.GET("/rrhh/dashboard", P("rrhh.ver"), nominaH.DashboardRRHH)
			// Empleados y deducciones recurrentes
			scoped.GET("/rrhh/empleados", P("rrhh.ver"), nominaH.ListarEmpleados)
			scoped.GET("/rrhh/empleados/:id", P("rrhh.ver"), nominaH.EmpleadoPorID)
			scoped.POST("/rrhh/empleados", P("rrhh.empleados"), nominaH.CrearEmpleado)
			scoped.PATCH("/rrhh/empleados/:id", P("rrhh.empleados"), nominaH.ActualizarEmpleado)
			scoped.POST("/rrhh/empleados/:id/desactivar", P("rrhh.empleados"), nominaH.DesactivarEmpleado)
			scoped.GET("/rrhh/empleados/:id/deducciones", P("rrhh.ver"), nominaH.ListarDeducciones)
			scoped.POST("/rrhh/empleados/:id/deducciones", P("rrhh.empleados"), nominaH.CrearDeduccion)
			scoped.PATCH("/rrhh/empleados/:id/deducciones/:dedId", P("rrhh.empleados"), nominaH.ActualizarDeduccion)
			scoped.POST("/rrhh/empleados/:id/deducciones/:dedId/desactivar", P("rrhh.empleados"), nominaH.DesactivarDeduccion)
			// Corrida quincenal (Etapa 2): adelanto día 15 + liquidación día 30
			scoped.GET("/rrhh/corridas", P("rrhh.ver"), nominaH.ListarCorridas)
			scoped.GET("/rrhh/corridas/:id", P("rrhh.ver"), nominaH.CorridaPorID)
			scoped.POST("/rrhh/corridas", P("rrhh.corrida"), nominaH.CrearCorrida)
			scoped.PUT("/rrhh/corridas/:id/novedades", P("rrhh.corrida"), nominaH.GuardarNovedades)
			scoped.POST("/rrhh/corridas/:id/recalcular", P("rrhh.corrida"), nominaH.RecalcularCorrida)
			scoped.POST("/rrhh/corridas/:id/aprobar", P("rrhh.corrida"), nominaH.AprobarCorrida)
			scoped.POST("/rrhh/corridas/:id/pagar", P("rrhh.corrida"), nominaH.PagarCorrida)
			scoped.POST("/rrhh/corridas/:id/anular", P("rrhh.corrida"), nominaH.AnularCorrida)
			// Notificaciones a los colaboradores (texto editable en Configuración → Notificaciones).
			scoped.POST("/rrhh/corridas/:id/boletas", P("rrhh.corrida"), nominaH.EnviarBoletas)
			scoped.POST("/rrhh/vacaciones/:id/aviso", P("rrhh.ausencias"), nominaH.EnviarAvisoVacaciones)
			// Exportaciones de la corrida (archivo SINPE con consecutivo; planilla CCSS)
			scoped.GET("/rrhh/corridas/:id/archivo-pago", P("rrhh.corrida"), nominaH.ArchivoPago)
			scoped.GET("/rrhh/corridas/:id/planilla-ccss", P("rrhh.ver"), nominaH.PlanillaCCSS)
			// Finiquito / liquidación de cese (Etapa 3, permiso crítico propio)
			scoped.GET("/rrhh/finiquitos", P("rrhh.ver"), nominaH.ListarFiniquitos)
			scoped.GET("/rrhh/finiquitos/:id", P("rrhh.ver"), nominaH.FiniquitoPorID)
			scoped.POST("/rrhh/finiquitos", P("rrhh.finiquito"), nominaH.CrearFiniquito)
			scoped.PATCH("/rrhh/finiquitos/:id", P("rrhh.finiquito"), nominaH.ActualizarFiniquito)
			scoped.POST("/rrhh/finiquitos/:id/aprobar", P("rrhh.finiquito"), nominaH.AprobarFiniquito)
			scoped.POST("/rrhh/finiquitos/:id/pagar", P("rrhh.finiquito"), nominaH.PagarFiniquito)
			scoped.POST("/rrhh/finiquitos/:id/anular", P("rrhh.finiquito"), nominaH.AnularFiniquito)
			// Incapacidades y vacaciones (Etapa 4)
			scoped.GET("/rrhh/incapacidades", P("rrhh.ver"), nominaH.ListarIncapacidades)
			scoped.POST("/rrhh/incapacidades", P("rrhh.ausencias"), nominaH.RegistrarIncapacidad)
			scoped.POST("/rrhh/incapacidades/:id/anular", P("rrhh.ausencias"), nominaH.AnularIncapacidad)
			scoped.GET("/rrhh/vacaciones/saldos", P("rrhh.ver"), nominaH.SaldosVacaciones)
			scoped.GET("/rrhh/vacaciones", P("rrhh.ver"), nominaH.ListarVacaciones)
			scoped.POST("/rrhh/vacaciones", P("rrhh.ausencias"), nominaH.RegistrarVacacion)
			scoped.POST("/rrhh/vacaciones/:id/anular", P("rrhh.ausencias"), nominaH.AnularVacacion)
			// Reportes
			scoped.GET("/rrhh/reportes/provisiones", P("rrhh.ver"), nominaH.Provisiones)
			// Parámetros del año y conceptos (guardarraíl CCSS en el servicio)
			scoped.GET("/rrhh/parametros/:anio", P("rrhh.ver"), nominaH.Parametros)
			scoped.PUT("/rrhh/parametros/:anio", P("rrhh.parametros"), nominaH.GuardarParametros)
			scoped.GET("/rrhh/conceptos", P("rrhh.ver"), nominaH.ListarConceptos)
			scoped.POST("/rrhh/conceptos", P("rrhh.parametros"), nominaH.CrearConcepto)
			scoped.PATCH("/rrhh/conceptos/:id", P("rrhh.parametros"), nominaH.ActualizarConcepto)
			scoped.POST("/rrhh/conceptos/:id/desactivar", P("rrhh.parametros"), nominaH.DesactivarConcepto)

			// Permisos efectivos del propio usuario (sin gate: cada quien ve su alcance).
			// Plantillas de correo de las notificaciones (CxP y RRHH). El texto es configuración.
			scoped.GET("/plantillas", P("admin.plantillas"), plantillasH.Listar)
			scoped.PUT("/plantillas/:clave", P("admin.plantillas"), plantillasH.Guardar)
			scoped.DELETE("/plantillas/:clave", P("admin.plantillas"), plantillasH.Restablecer)
			scoped.POST("/plantillas/:clave/vista-previa", P("admin.plantillas"), plantillasH.VistaPrevia)
			scoped.GET("/rbac/mis-permisos", rbacH.MisPermisos)

			// ── Administración RBAC (permiso admin.roles) ──
			scoped.GET("/rbac/permisos", P("admin.roles"), rbacH.Permisos)
			scoped.GET("/rbac/roles", P("admin.roles"), rbacH.Roles)
			scoped.GET("/rbac/matriz", P("admin.roles"), rbacH.Matriz)
			scoped.PUT("/rbac/roles/:codigo/permisos", P("admin.roles"), rbacH.SetPermisos)
			scoped.POST("/rbac/roles", P("admin.roles"), rbacH.CrearRol)
			// Usuarios (Administración) — gestión por empresa activa.
			scoped.GET("/rbac/usuarios", P("admin.roles"), rbacH.ListarUsuarios)
			scoped.POST("/rbac/usuarios", P("admin.roles"), rbacH.CrearUsuario)
			scoped.PATCH("/rbac/usuarios/:id", P("admin.roles"), rbacH.ActualizarUsuario)
			scoped.POST("/rbac/usuarios/:id/reset-password", P("admin.roles"), rbacH.ResetPassword)
			scoped.DELETE("/rbac/usuarios/:id/acceso", P("admin.roles"), rbacH.QuitarAcceso)
			scoped.POST("/rbac/permisos/aplicar-faltantes", P("admin.roles"), rbacH.AplicarPermisosFaltantes)
			// ── Cuentas por cobrar (fase 1: cartera y cargos) ──
			// El alcance por SEDE lo aplica el servicio: sin cxc.ver_todas_sedes, el
			// operador solo ve la cartera que le asignaron, aunque arme la URL a mano.
			scoped.GET("/cxc/catalogos", P("cxc.ver"), cxcH.Catalogos)
			scoped.GET("/cxc/contratos", P("cxc.ver"), cxcH.Contratos)
			scoped.GET("/cxc/contratos/:numero", P("cxc.ver"), cxcH.Contrato)
			scoped.GET("/cxc/cargos/plan", P("cxc.ver"), cxcH.PlanCargos)
			scoped.POST("/cxc/cargos/generar", P("cxc.importar"), cxcH.GenerarCargos)
			scoped.POST("/cxc/importaciones/contratos/previsualizar", P("cxc.importar"), cxcH.PrevisualizarImportacion)
			scoped.POST("/cxc/importaciones/contratos/confirmar", P("cxc.importar"), cxcH.ConfirmarImportacion)
			// Cobros (fase 2): archivo, API idempotente, reversa e identificación.
			scoped.GET("/cxc/cobros", P("cxc.ver"), cxcH.Cobros)
			scoped.GET("/cxc/asociaciones/panorama", P("cxc.ver"), cxcH.PanoramaAsociaciones)
			scoped.POST("/cxc/cobros", P("cxc.cobros"), cxcH.RegistrarCobro)
			scoped.POST("/cxc/cobros/:id/reversar", P("cxc.aplicar"), cxcH.ReversarCobro)
			scoped.POST("/cxc/cobros/:id/identificar", P("cxc.aplicar"), cxcH.IdentificarCobro)
			scoped.POST("/cxc/importaciones/cobros/previsualizar", P("cxc.importar"), cxcH.PrevisualizarCobros)
			scoped.POST("/cxc/importaciones/cobros/confirmar", P("cxc.cobros"), cxcH.ConfirmarCobros)
			// Gestión de cobro (fase 3): la cola por valor esperado y lo que se hizo con
			// cada contrato. Ver la cola es cxc.ver; anotar una gestión es cxc.gestionar.
			scoped.GET("/cxc/cola", P("cxc.ver"), cxcH.Cola)
			scoped.GET("/cxc/gestiones/catalogos", P("cxc.ver"), cxcH.CatalogosGestion)
			scoped.GET("/cxc/contratos/:numero/gestiones", P("cxc.ver"), cxcH.Gestiones)
			scoped.POST("/cxc/gestiones", P("cxc.gestionar"), cxcH.RegistrarGestion)
			// Configuración del módulo. Ver requiere cxc.ver (el operador necesita entender
			// por qué su cola está ordenada así); cambiar requiere cxc.parametros.
			scoped.GET("/cxc/parametros", P("cxc.ver"), cxcH.Config)
			scoped.PUT("/cxc/parametros", P("cxc.parametros"), cxcH.GuardarParametros)
			scoped.PATCH("/cxc/tramos/:codigo", P("cxc.parametros"), cxcH.ActualizarTramo)
			scoped.PATCH("/cxc/formas-pago/:id", P("cxc.parametros"), cxcH.ActualizarFormaPago)
			scoped.POST("/cxc/sedes", P("cxc.parametros"), cxcH.CrearSede)
			scoped.PATCH("/cxc/sedes/:id", P("cxc.parametros"), cxcH.ActualizarSede)
			// La frontera de datos: qué sede ve cada usuario. Sin esto, un operador sin
			// cxc.ver_todas_sedes veía la cola vacía y nadie podía arreglarlo.
			scoped.PUT("/cxc/usuarios/:id/sedes", P("cxc.parametros"), cxcH.AsignarSedes)
			// Planillas de asociación: el tercer contraste contra el depósito que ya está en
			// Bancos. Vincular es conciliación de dinero, así que va con cxc.aplicar.
			scoped.GET("/cxc/asociaciones/:id/planilla", P("cxc.ver"), cxcH.PlanillaAsociacion)
			scoped.POST("/cxc/asociaciones/:id/planilla", P("cxc.cobros"), cxcH.AbrirPlanilla)
			scoped.GET("/cxc/planillas/:id/candidatos", P("cxc.ver"), cxcH.CandidatosDeposito)
			scoped.POST("/cxc/planillas/:id/depositos", P("cxc.aplicar"), cxcH.VincularDeposito)
			scoped.DELETE("/cxc/planillas/:id/depositos/:movimiento", P("cxc.aplicar"), cxcH.DesvincularDeposito)
			// Notas de crédito: bajar deuda sin que entre plata. Las autoriza el supervisor de
			// piso (rol SUPERVISOR_PISO) y no tienen tope; el control es el motivo y el rastro.
			scoped.GET("/cxc/notas-credito", P("cxc.ver"), cxcH.Notas)
			scoped.POST("/cxc/notas-credito", P("cxc.notas_credito"), cxcH.EmitirNota)
			scoped.POST("/cxc/notas-credito/:id/anular", P("cxc.notas_credito"), cxcH.AnularNota)
			// Suspensión por mora: 18 MESES de mora, o su equivalencia en cuotas según la
			// modalidad. El sistema dice cuándo se puede; cortar el servicio lo decide una
			// persona con cxc.suspender.
			scoped.GET("/cxc/contratos/:numero/suspension", P("cxc.ver"), cxcH.EstadoSuspension)
			scoped.POST("/cxc/contratos/:numero/suspender", P("cxc.suspender"), cxcH.Suspender)
			scoped.POST("/cxc/contratos/:numero/reactivar", P("cxc.suspender"), cxcH.Reactivar)
			// Arreglos de pago. Pactar un plazo ESTÁNDAR (1-3-6-9) es parte de gestionar; los
			// plazos de excepción los autoriza el supervisor de piso y eso lo valida el
			// servicio con cxc.arreglos, porque es una regla de negocio y no una ruta.
			// Quebrar o anular sí exige cxc.arreglos: son decisiones del supervisor.
			scoped.GET("/cxc/arreglos", P("cxc.ver"), cxcH.Arreglos)
			scoped.GET("/cxc/arreglos/:id", P("cxc.ver"), cxcH.Arreglo)
			scoped.POST("/cxc/arreglos", P("cxc.gestionar"), cxcH.PactarArreglo)
			scoped.POST("/cxc/arreglos/:id/quebrar", P("cxc.arreglos"), cxcH.QuebrarArreglo)
			scoped.POST("/cxc/arreglos/:id/anular", P("cxc.arreglos"), cxcH.AnularArreglo)
			// Contacto preventivo: el universo que la cola excluye a propósito. Permiso propio,
			// como pidió el negocio: llamar a quien todavía no debe es otra actividad.
			scoped.GET("/cxc/preventivo", P("cxc.preventivo"), cxcH.Preventivo)
		}
	}

	return r
}

func health(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) }

// cors habilita CORS solo para los orígenes permitidos (dev: el frontend Vite).
func cors(origins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		allowed[o] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			// Sin esto el navegador OCULTA Content-Disposition al JavaScript (no es una cabecera
			// de respuesta «simple»), y toda descarga que nombra el archivo en el servidor —los
			// reportes de Bancos, el archivo SINPE y la planilla CCSS, que llevan el consecutivo
			// de bitácora— caía al nombre de respaldo del cliente sin avisar.
			c.Header("Access-Control-Expose-Headers", "Content-Disposition")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// requestLogger registra cada petición con zap.
func requestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("dur", time.Since(start)),
		)
	}
}
