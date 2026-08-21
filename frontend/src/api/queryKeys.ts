/**
 * Fábrica central de query keys para TanStack Query.
 *
 * Regla del proyecto (gpvdp-data-layer): las keys de datos de empresa van
 * SIEMPRE namespaced por empresa activa, para que cambiar de empresa segregue
 * la caché y nunca se mezclen datos entre empresas.
 *
 * `me` y `empresas` son a nivel de usuario (no de empresa), así que no llevan
 * empresaId.
 */
export const queryKeys = {
  // Nivel usuario.
  me: () => ["me"] as const,
  empresas: () => ["empresas"] as const,

  // Nivel empresa — módulo Bancos (Fase 1). Todas namespaced por empresaId.
  bancos: {
    /** Raíz para invalidación amplia (invalida todo el módulo de la empresa). */
    raiz: (empresaId: string) => ["bancos", empresaId] as const,

    cuentas: (empresaId: string, incluirInactivas = false) =>
      ["bancos", "cuentas", empresaId, incluirInactivas] as const,
    conceptos: (empresaId: string, ambito?: string) =>
      ["bancos", "conceptos", empresaId, ambito ?? "todos"] as const,
    conceptosRaiz: (empresaId: string) => ["bancos", "conceptos", empresaId] as const,
    clasificaciones: (empresaId: string, ambito?: string) =>
      ["bancos", "clasificaciones", empresaId, ambito ?? "todos"] as const,
    clasificacionesRaiz: (empresaId: string) =>
      ["bancos", "clasificaciones", empresaId] as const,
    reglas: (empresaId: string) => ["bancos", "reglas", empresaId] as const,
    bancosCatalogo: (empresaId: string, incluirInactivos = false) =>
      ["bancos", "catalogo-bancos", empresaId, incluirInactivos] as const,

    movimientos: (empresaId: string, filtros?: unknown) =>
      ["bancos", "movimientos", empresaId, filtros ?? null] as const,
    /**
     * Resumen de la selección: cuelga del MISMO prefijo que los movimientos a propósito,
     * para que cualquier invalidación de la hoja de trabajo (clasificar, reclasificar,
     * emparejar) refresque también el encabezado sin tener que acordarse de hacerlo.
     */
    resumenSeleccion: (empresaId: string, agrupar: string, filtros?: unknown) =>
      ["bancos", "movimientos", empresaId, "resumen", agrupar, filtros ?? null] as const,
    /** Prefijo para invalidar TODOS los listados de movimientos sin importar filtros. */
    movimientosRaiz: (empresaId: string) =>
      ["bancos", "movimientos", empresaId] as const,

    cuadre: (empresaId: string, periodo: string) =>
      ["bancos", "cuadre", empresaId, periodo] as const,
    // Comparte el prefijo ["bancos","cuadre",empresaId] para que las invalidaciones
    // del cuadre lo refresquen sin cambios extra.
    cuadreArbol: (empresaId: string, periodo: string) =>
      ["bancos", "cuadre", empresaId, "arbol", periodo] as const,
    dashboard: (empresaId: string, periodo: string) =>
      ["bancos", "dashboard", empresaId, periodo] as const,

    propuestas: (empresaId: string, periodo: string) =>
      ["bancos", "propuestas", empresaId, periodo] as const,

    resumenClasif: (empresaId: string, periodo?: string) =>
      ["bancos", "resumen-clasif", empresaId, periodo ?? "todo"] as const,

    // Patrones: comparten prefijo con el resumen de clasificación para invalidarse juntos
    // cuando se crea una regla (crear una regla cambia lo que queda sin clasificar).
    patrones: (empresaId: string, periodo?: string) =>
      ["bancos", "resumen-clasif", empresaId, "patrones", periodo ?? "todo"] as const,

    // Tesorería (Tanda 1): saldo diario, checklist de carga y conciliación mensual.
    tesoreria: (empresaId: string, fecha: string) =>
      ["bancos", "tesoreria", empresaId, fecha] as const,
    /** Prefijo para invalidar la tesorería de cualquier fecha tras capturar o congelar. */
    tesoreriaRaiz: (empresaId: string) => ["bancos", "tesoreria", empresaId] as const,
    carga: (empresaId: string, periodo: string) =>
      ["bancos", "carga", empresaId, periodo] as const,
    conciliacion: (empresaId: string, periodo: string) =>
      ["bancos", "conciliacion", empresaId, periodo] as const,
    conciliacionRaiz: (empresaId: string) => ["bancos", "conciliacion", empresaId] as const,

    // Proyecciones (Fase C).
    proyeccion: (empresaId: string, periodo: string, metodo: string, metaPct: string) =>
      ["bancos", "proyeccion", empresaId, periodo, metodo, metaPct] as const,
    escenarios: (empresaId: string, periodo?: string) =>
      ["bancos", "escenarios", empresaId, periodo ?? "todos"] as const,
    escenariosRaiz: (empresaId: string) => ["bancos", "escenarios", empresaId] as const,

    // Análisis visual (Fase B) — comparten prefijo con dashboard para invalidación.
    serieMensual: (empresaId: string, hasta: string) =>
      ["bancos", "dashboard", empresaId, "serie", hasta] as const,
    calendario: (empresaId: string, periodo: string) =>
      ["bancos", "dashboard", empresaId, "calendario", periodo] as const,
    /** Análisis de partidas en el tiempo: cambia con el rango. */
    analisisPartidas: (empresaId: string, desde: string, hasta: string) =>
      ["bancos", "dashboard", empresaId, "partidas", desde, hasta] as const,
    cuentasResumen: (empresaId: string, periodo: string) =>
      ["bancos", "dashboard", empresaId, "cuentas", periodo] as const,
    /** Prefijo para invalidar el resumen sin importar el período. */
    resumenClasifRaiz: (empresaId: string) =>
      ["bancos", "resumen-clasif", empresaId] as const,

    tipoCambio: (empresaId: string, anio: number, mes: number) =>
      ["bancos", "tipo-cambio", empresaId, anio, mes] as const,

    periodo: (empresaId: string, anio: number, mes: number) =>
      ["bancos", "periodo", empresaId, anio, mes] as const,

    preview: (empresaId: string, importacionId: string) =>
      ["bancos", "preview", empresaId, importacionId] as const,

    // Fase D
    parametros: (empresaId: string) => ["bancos", "parametros", empresaId] as const,
    ultimoSync: (empresaId: string) => ["bancos", "ultimo-sync", empresaId] as const,
  },

  // Nivel empresa — módulo CxP (Fase 2). Todas namespaced por empresaId.
  cxc: {
    /**
     * La empresa va SEGUNDA en todas las claves de CxC, no tercera: TanStack Query calza
     * por PREFIJO, así que solo con este orden `raiz` invalida de verdad el módulo entero.
     * Con la empresa en tercer lugar, `["cxc", empresaId]` no calzaba con
     * `["cxc","cobros",empresaId,…]` y las invalidaciones no hacían nada — el saldo seguía
     * mostrando la deuda vieja después de aplicar un cobro.
     */
    raiz: (empresaId: string) => ["cxc", empresaId] as const,

    catalogos: (empresaId: string) => ["cxc", empresaId, "catalogos"] as const,
    contratos: (empresaId: string, filtros?: unknown) =>
      ["cxc", empresaId, "contratos", filtros ?? null] as const,
    contrato: (empresaId: string, numero: string, soloAbiertos: boolean) =>
      ["cxc", empresaId, "contrato", numero, soloAbiertos] as const,
    planCargos: (empresaId: string, desde: string, hasta: string) =>
      ["cxc", empresaId, "plan-cargos", desde, hasta] as const,
    cobros: (empresaId: string, filtros?: unknown) =>
      ["cxc", empresaId, "cobros", filtros ?? null] as const,
    asociaciones: (empresaId: string, periodo: string) =>
      ["cxc", empresaId, "asociaciones", periodo] as const,
    cola: (empresaId: string, filtros?: unknown) =>
      ["cxc", empresaId, "cola", filtros ?? null] as const,
    catalogosGestion: (empresaId: string) => ["cxc", empresaId, "catalogos-gestion"] as const,
    gestiones: (empresaId: string, numero: string) => ["cxc", empresaId, "gestiones", numero] as const,
    config: (empresaId: string) => ["cxc", empresaId, "config"] as const,
    planilla: (empresaId: string, asociacionId: string, periodo: string) =>
      ["cxc", empresaId, "planilla", asociacionId, periodo] as const,
    candidatos: (empresaId: string, planillaId: string) =>
      ["cxc", empresaId, "planilla-candidatos", planillaId] as const,
    notas: (empresaId: string, filtros?: unknown) =>
      ["cxc", empresaId, "notas-credito", filtros ?? null] as const,
    suspension: (empresaId: string, numero: string) =>
      ["cxc", empresaId, "suspension", numero] as const,
    arreglos: (empresaId: string, filtros?: unknown) =>
      ["cxc", empresaId, "arreglos", filtros ?? null] as const,
    preventivo: (empresaId: string, filtros?: unknown) =>
      ["cxc", empresaId, "preventivo", filtros ?? null] as const,
  },

  cxp: {
    /** Raíz para invalidación amplia del módulo CxP de la empresa. */
    raiz: (empresaId: string) => ["cxp", empresaId] as const,

    proveedores: (empresaId: string, filtros?: unknown) =>
      ["cxp", "proveedores", empresaId, filtros ?? null] as const,
    /** Prefijo para invalidar TODOS los listados paginados de proveedores. */
    proveedoresRaiz: (empresaId: string) => ["cxp", "proveedores", empresaId] as const,
    /** Lista completa de proveedores (para selects). */
    proveedoresTodos: (empresaId: string) => ["cxp", "proveedores-todos", empresaId] as const,
    proveedor: (empresaId: string, id: string) =>
      ["cxp", "proveedor", empresaId, id] as const,
    departamentos: (empresaId: string, soloActivos: boolean) =>
      ["cxp", "departamentos", empresaId, soloActivos] as const,

    documentos: (empresaId: string, filtros?: unknown) =>
      ["cxp", "documentos", empresaId, filtros ?? null] as const,
    /** Prefijo para invalidar TODOS los listados de documentos (cualquier filtro). */
    documentosRaiz: (empresaId: string) => ["cxp", "documentos", empresaId] as const,
    documento: (empresaId: string, id: string) =>
      ["cxp", "documento", empresaId, id] as const,
    historial: (empresaId: string, id: string) =>
      ["cxp", "historial", empresaId, id] as const,
    /** El período va en la clave: cambiar el selector global refetchea el tablero. */
    dashboard: (empresaId: string, periodo: string) =>
      ["cxp", "dashboard", empresaId, periodo] as const,
    /** Prefijo para invalidar el tablero de CUALQUIER período tras una mutación. */
    dashboardRaiz: (empresaId: string) => ["cxp", "dashboard", empresaId] as const,
    bandeja: (empresaId: string) => ["cxp", "bandeja", empresaId] as const,
    subclasificaciones: (empresaId: string, clasificacionId: string) =>
      ["cxp", "subclasificaciones", empresaId, clasificacionId] as const,
    subclasificacionesRaiz: (empresaId: string) => ["cxp", "subclasificaciones", empresaId] as const,
    lotes: (empresaId: string) => ["cxp", "lotes", empresaId] as const,
    /** Cuadro de lo marcado como «de Contabilidad» (proveedores, conceptos, clasificaciones). */
    marcasContabilidad: (empresaId: string) => ["cxp", "marcas-contabilidad", empresaId] as const,
    /** Umbrales de la validación por riesgo (desde qué monto/desvío el área tiene que confirmar). */
    /** Proveedores sin cuenta IBAN: la lista de quién no se puede pagar todavía. */
    sinIBAN: (empresaId: string) => ["cxp", "sin-iban", empresaId] as const,
    parametrosValidacion: (empresaId: string) => ["cxp", "parametros-validacion", empresaId] as const,
  },

  // Nivel empresa — módulo RRHH / Nómina (Fase 3).
  rrhh: {
    /** Raíz para invalidación amplia del módulo RRHH de la empresa. */
    raiz: (empresaId: string) => ["rrhh", empresaId] as const,

    dashboard: (empresaId: string, anio: number, mes: number) =>
      ["rrhh", "dashboard", empresaId, anio, mes] as const,
    empleados: (empresaId: string, filtros?: unknown) =>
      ["rrhh", "empleados", empresaId, filtros ?? null] as const,
    /** Prefijo para invalidar TODOS los listados de empleados (cualquier filtro). */
    empleadosRaiz: (empresaId: string) => ["rrhh", "empleados", empresaId] as const,
    empleado: (empresaId: string, id: string) => ["rrhh", "empleado", empresaId, id] as const,
    deducciones: (empresaId: string, empleadoId: string) =>
      ["rrhh", "deducciones", empresaId, empleadoId] as const,
    parametros: (empresaId: string, anio: number) =>
      ["rrhh", "parametros", empresaId, anio] as const,
    conceptos: (empresaId: string) => ["rrhh", "conceptos", empresaId] as const,
    corridas: (empresaId: string, anio: number) => ["rrhh", "corridas", empresaId, anio] as const,
    /** Prefijo para invalidar los listados de corridas de todos los años. */
    corridasRaiz: (empresaId: string) => ["rrhh", "corridas", empresaId] as const,
    corrida: (empresaId: string, id: string) => ["rrhh", "corrida", empresaId, id] as const,
    finiquitos: (empresaId: string) => ["rrhh", "finiquitos", empresaId] as const,
    finiquito: (empresaId: string, id: string) => ["rrhh", "finiquito", empresaId, id] as const,
    provisiones: (empresaId: string, anio: number) => ["rrhh", "provisiones", empresaId, anio] as const,
    incapacidades: (empresaId: string, anio: number, mes: number) =>
      ["rrhh", "incapacidades", empresaId, anio, mes] as const,
    /** Prefijo para invalidar los listados de incapacidades de cualquier período. */
    incapacidadesRaiz: (empresaId: string) => ["rrhh", "incapacidades", empresaId] as const,
    saldosVacaciones: (empresaId: string, anio: number) =>
      ["rrhh", "saldos-vacaciones", empresaId, anio] as const,
    saldosVacacionesRaiz: (empresaId: string) => ["rrhh", "saldos-vacaciones", empresaId] as const,
    vacaciones: (empresaId: string, empleadoId?: string) =>
      ["rrhh", "vacaciones", empresaId, empleadoId ?? "todas"] as const,
    vacacionesRaiz: (empresaId: string) => ["rrhh", "vacaciones", empresaId] as const,
  },
} as const;
