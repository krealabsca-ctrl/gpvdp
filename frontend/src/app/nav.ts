/**
 * Registro de navegación del ERP — FUENTE ÚNICA de módulos y páginas.
 *
 * Arquitectura de información: Empresa (contexto) → Módulo → Página. El riel de
 * módulos y la sidebar contextual se generan desde aquí. Para agregar un módulo
 * nuevo, solo se añade una entrada (con `disponible: true` y sus páginas). Los
 * módulos futuros se listan con `disponible: false` para mostrar el mapa completo.
 */

import type { LucideIcon } from "lucide-react";
import {
  Banknote,
  BarChart3,
  BellRing,
  BookOpen,
  Boxes,
  Building,
  Building2,
  Calculator,
  CalendarDays,
  ClipboardCheck,
  Coins,
  Download,
  FileSearch,
  Gauge,
  GitCompare,
  HandCoins,
  Handshake,
  Inbox,
  Landmark,
  LayoutDashboard,
  LineChart,
  Mail,
  PhoneCall,
  PiggyBank,
  Receipt,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  ShoppingBag,
  ShoppingCart,
  TrendingUp,
  Upload,
  Users,
  Wallet,
  Waves,
} from "lucide-react";

export interface NavPage {
  label: string;
  descripcion: string;
  to: string;
  icon: LucideIcon;
  /** Activo solo con coincidencia exacta (rutas índice como "/"). */
  end?: boolean;
  /** Si se define, la página solo se muestra a quien tenga este permiso (RBAC). */
  permiso?: string;
}

export interface NavModule {
  id: string;
  label: string;
  icon: LucideIcon;
  /** Página por defecto al entrar al módulo. */
  to: string;
  disponible: boolean;
  /** Permiso base de lectura del módulo; las páginas sin `permiso` propio lo heredan. */
  permiso?: string;
  pages: NavPage[];
}

export const MODULES: NavModule[] = [
  {
    id: "bancos",
    label: "Bancos",
    icon: Landmark,
    to: "/",
    disponible: true,
    permiso: "bancos.ver",
    pages: [
      { label: "Dashboard", descripcion: "KPIs del período", to: "/", end: true, icon: LayoutDashboard },
      { label: "Clasificar", descripcion: "Bandeja: motor, traslados y reglas", to: "/clasificar", icon: ClipboardCheck },
      // Nombres de PÁGINA, distintos de los módulos futuros «Tesorería» y «Conciliación
      // bancaria» del riel: acá se captura el saldo del día y se firma el acta del mes.
      { label: "Saldos diarios", descripcion: "Saldo del día y disponible", to: "/saldos-diarios", icon: Banknote },
      { label: "Actas de conciliación", descripcion: "Una por cuenta y mes", to: "/conciliacion", icon: GitCompare },
      { label: "Importador", descripcion: "Subir estados de cuenta", to: "/importador", icon: Upload },
      { label: "Catálogo", descripcion: "Conceptos y clasificaciones", to: "/catalogo", icon: BookOpen },
      { label: "Tipo de cambio", descripcion: "Cotizaciones", to: "/tipo-cambio", icon: Coins },
      { label: "Exportar", descripcion: "Salidas y reportes", to: "/exportar", icon: Download, permiso: "bancos.exportar" },
      {
        label: "Análisis y tendencias",
        descripcion: "Cada partida contra su historia",
        to: "/analisis",
        icon: LineChart,
      },
      { label: "Proyecciones", descripcion: "Cierre de mes", to: "/proyecciones", icon: TrendingUp },
      { label: "Ajustes", descripcion: "Parámetros de la empresa", to: "/ajustes", icon: Settings, permiso: "bancos.ajustes" },
    ],
  },
  {
    id: "cxp",
    label: "Cuentas por pagar",
    icon: Wallet,
    to: "/cxp/bandeja",
    disponible: true,
    permiso: "cxp.ver",
    pages: [
      { label: "Bandeja", descripcion: "El trabajo, por fases", to: "/cxp/bandeja", icon: Inbox },
      { label: "Dashboard", descripcion: "Cartera, vencimientos y cola", to: "/cxp/dashboard", icon: LayoutDashboard, permiso: "cxp.dashboard" },
      { label: "Importar", descripcion: "Facturación (Excel)", to: "/cxp/importar", icon: Upload, permiso: "cxp.importar" },
      { label: "Proveedores", descripcion: "Maestro", to: "/cxp/proveedores", icon: Building2, permiso: "cxp.ver_todo" },
      { label: "Anticipos", descripcion: "Saldos a favor del proveedor", to: "/cxp/anticipos", icon: Coins, permiso: "cxp.anticipos" },
      { label: "Caja chica", descripcion: "Fondos fijos y vales", to: "/cxp/cajas", icon: PiggyBank, permiso: "cxp.caja_ver" },
      { label: "Departamentos", descripcion: "Áreas / centros de costo", to: "/cxp/departamentos", icon: Building, permiso: "cxp.departamentos" },
      // Las excepciones de validación de área tienen que estar a la vista en una pantalla: si solo
      // se pueden consultar abriendo proveedor por proveedor, nadie las audita.
      { label: "De Contabilidad", descripcion: "Gasto sin validación de área", to: "/cxp/contabilidad", icon: Receipt, permiso: "cxp.marcar_contabilidad" },
      // Los umbrales deciden cuánto gasto se paga sin revisión humana: permiso propio y a la vista.
      { label: "Validación por riesgo", descripcion: "Desde qué monto valida el área", to: "/cxp/validacion", icon: ShieldCheck, permiso: "cxp.parametros" },
    ],
  },
  {
    // Cuentas por cobrar. Va junto a Cuentas por pagar y no al final: el riel sigue el flujo
    // del dinero (el banco, lo que se paga, lo que se cobra, la planilla) y agrupa TODO lo
    // disponible antes del roadmap.
    //
    // El alcance de datos del operador es su SEDE: sin cxc.ver_todas_sedes solo ve la cartera
    // que le asignaron, y eso lo verifica el servidor, no esta pantalla.
    id: "cxc",
    label: "Cuentas por cobrar",
    icon: HandCoins,
    to: "/cxc/cola",
    disponible: true,
    permiso: "cxc.ver",
    pages: [
      // La cola va PRIMERA: es donde se trabaja todos los días. La cartera es la consulta.
      { label: "Cola de cobro", descripcion: "A quién llamar hoy, por valor esperado", to: "/cxc/cola", icon: PhoneCall },
      // Lista aparte y con permiso propio: llamar a quien todavía no debe nada es otra
      // actividad que cobrar, y el negocio lo pidió separado.
      {
        label: "Contacto preventivo",
        descripcion: "Avisar antes de que la cuota se venza",
        to: "/cxc/preventivo",
        icon: BellRing,
        permiso: "cxc.preventivo",
      },
      { label: "Cartera", descripcion: "Contratos, cargos y saldo", to: "/cxc/cartera", icon: HandCoins },
      { label: "Cobros", descripcion: "Lo que entró y a qué se aplicó", to: "/cxc/cobros", icon: Coins },
      { label: "Arreglos de pago", descripcion: "Planes pactados y cartera morosa", to: "/cxc/arreglos", icon: Handshake, permiso: "cxc.arreglos" },
      { label: "Asociaciones", descripcion: "Planillas solidaristas: esperado vs cobrado", to: "/cxc/asociaciones", icon: Building2 },
      { label: "Importar", descripcion: "Cartera y pagos del origen", to: "/cxc/importar", icon: Upload, permiso: "cxc.importar" },
      { label: "Parámetros", descripcion: "Tramos, factores, sedes y accesos", to: "/cxc/parametros", icon: SlidersHorizontal },
    ],
  },
  {
    // RRHH / Nómina (Fase 3 — Etapa 1). Dato sensible (salarios): gate rrhh.ver.
    id: "rrhh",
    label: "Recursos Humanos",
    icon: Users,
    to: "/rrhh",
    disponible: true,
    permiso: "rrhh.ver",
    pages: [
      { label: "Dashboard", descripcion: "Costo real del mes y ciclo", to: "/rrhh", icon: LayoutDashboard, end: true },
      { label: "Empleados", descripcion: "Fichas, salarios y deducciones", to: "/rrhh/empleados", icon: Users },
      { label: "Corridas", descripcion: "Pago del período + horas extra y bonos", to: "/rrhh/corridas", icon: Calculator },
      { label: "Vacaciones e incapacidades", descripcion: "Registrar días disfrutados y boletas de la CCSS", to: "/rrhh/ausencias", icon: CalendarDays },
      { label: "Finiquitos", descripcion: "Cese conforme al CT + provisiones", to: "/rrhh/finiquitos", icon: HandCoins },
      { label: "Parámetros", descripcion: "Cargas, renta y conceptos", to: "/rrhh/parametros", icon: SlidersHorizontal },
    ],
  },
  {
    // Administración: usuarios y roles. Solo para quien administra (admin.roles).
    id: "config",
    label: "Configuración",
    icon: Settings,
    to: "/usuarios",
    disponible: true,
    permiso: "admin.roles",
    pages: [
      { label: "Usuarios", descripcion: "Personas y accesos", to: "/usuarios", icon: Users },
      { label: "Seguridad", descripcion: "Roles y permisos", to: "/seguridad", icon: ShieldCheck },
      {
        label: "Notificaciones",
        descripcion: "Texto de los correos que envía el sistema",
        to: "/notificaciones",
        icon: Mail,
        permiso: "admin.plantillas",
      },
    ],
  },
  // ── Roadmap: módulos aún sin construir. Se muestran atenuados y DESPUÉS de todo lo
  //    disponible, nunca intercalados. Un módulo terminado en medio de los grises se lee
  //    como si tampoco funcionara (le pasó a Cuentas por cobrar).
  { id: "tesoreria", label: "Tesorería", icon: Banknote, to: "#", disponible: false, pages: [] },
  { id: "conciliacion", label: "Conciliación bancaria", icon: GitCompare, to: "#", disponible: false, pages: [] },
  { id: "contabilidad", label: "Contabilidad", icon: Calculator, to: "#", disponible: false, pages: [] },
  { id: "presupuestos", label: "Presupuestos", icon: PiggyBank, to: "#", disponible: false, pages: [] },
  { id: "flujo", label: "Flujo de caja", icon: Waves, to: "#", disponible: false, pages: [] },
  { id: "compras", label: "Compras", icon: ShoppingCart, to: "#", disponible: false, pages: [] },
  { id: "ventas", label: "Ventas", icon: ShoppingBag, to: "#", disponible: false, pages: [] },
  { id: "facturacion", label: "Facturación", icon: Receipt, to: "#", disponible: false, pages: [] },
  { id: "inventarios", label: "Inventarios", icon: Boxes, to: "#", disponible: false, pages: [] },
  { id: "activos", label: "Activos", icon: Building, to: "#", disponible: false, pages: [] },
  { id: "bi", label: "Reportes BI", icon: BarChart3, to: "#", disponible: false, pages: [] },
  { id: "ejecutivo", label: "Dashboard Ejecutivo", icon: Gauge, to: "#", disponible: false, pages: [] },
  { id: "auditoria", label: "Auditoría", icon: FileSearch, to: "#", disponible: false, pages: [] },
];

/** Permiso efectivo de una página: el propio, o el del módulo si no tiene uno más específico. */
export function permisoDePagina(m: NavModule, p: NavPage): string | undefined {
  return p.permiso ?? m.permiso;
}

/** Módulo dueño de una ruta (para resaltar el riel y poblar la sidebar contextual). */
export function moduloActivo(pathname: string): NavModule {
  const byId = (id: string) => MODULES.find((m) => m.id === id)!;
  if (pathname.startsWith("/cxp")) return byId("cxp");
  if (pathname.startsWith("/cxc")) return byId("cxc");
  if (pathname.startsWith("/rrhh")) return byId("rrhh");
  // El resto se resuelve contra el REGISTRO de páginas, no con una lista a mano: agregar una
  // página a Configuración no debe exigir tocar esta función. (Así se rompió /notificaciones,
  // que abría la sidebar de Bancos porque nadie la agregó acá.)
  const dueño = MODULES.find(
    (m) =>
      m.disponible &&
      m.id !== "bancos" &&
      m.pages.some((p) => p.to !== "/" && (pathname === p.to || pathname.startsWith(p.to + "/"))),
  );
  // Bancos es el módulo raíz: se queda con "/" y con lo que no reclame nadie.
  return dueño ?? byId("bancos");
}

/** Permiso requerido para ver una ruta (según su módulo/página). undefined = sin gate. */
export function permisoDeRuta(pathname: string): string | undefined {
  const m = moduloActivo(pathname);
  const page = m.pages.find((p) => (p.end ? pathname === p.to : pathname.startsWith(p.to)));
  return page ? permisoDePagina(m, page) : m.permiso;
}

/** Primera ruta que el usuario SÍ puede ver (para aterrizar a quien no tiene acceso al índice). */
export function primeraRutaAccesible(tienePermiso: (permiso: string) => boolean): string {
  for (const m of MODULES.filter((x) => x.disponible)) {
    for (const p of m.pages) {
      const perm = permisoDePagina(m, p);
      if (!perm || tienePermiso(perm)) return p.to;
    }
  }
  return "/";
}

/** Todas las páginas de módulos disponibles (para el command palette). */
export function todasLasPaginas(): Array<NavPage & { modulo: string; permisoEfectivo?: string }> {
  return MODULES.filter((m) => m.disponible).flatMap((m) =>
    m.pages.map((p) => ({ ...p, modulo: m.label, permisoEfectivo: permisoDePagina(m, p) })),
  );
}
