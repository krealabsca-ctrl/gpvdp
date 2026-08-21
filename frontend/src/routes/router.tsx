import { createBrowserRouter, Navigate } from "react-router-dom";
import { PublicOnlyRoute } from "@/routes/PublicOnlyRoute";
import { ProtectedRoute } from "@/routes/ProtectedRoute";
import { LoginPage } from "@/features/auth/LoginPage";
import { SelectEmpresaPage } from "@/features/auth/SelectEmpresaPage";
import { AppShell } from "@/components/shell/AppShell";
import { NotFoundPage } from "@/routes/NotFoundPage";
// Módulo Bancos (Fase 1 + Bandeja de clasificación Fase A)
import { DashboardPage } from "@/features/bancos/pages/DashboardPage";
import { ImportadorPage } from "@/features/bancos/pages/ImportadorPage";
import { ClasificarPage } from "@/features/bancos/pages/ClasificarPage";
import { CatalogoPage } from "@/features/bancos/pages/CatalogoPage";
import { TipoCambioPage } from "@/features/bancos/pages/TipoCambioPage";
import { ExportarPage } from "@/features/bancos/pages/ExportarPage";
import { ProyeccionesPage } from "@/features/bancos/pages/ProyeccionesPage";
import { AnalisisPage } from "@/features/bancos/pages/AnalisisPage";
import { SaldosDiariosPage } from "@/features/bancos/pages/SaldosDiariosPage";
import { ConciliacionPage } from "@/features/bancos/pages/ConciliacionPage";
import { AjustesPage } from "@/features/bancos/pages/AjustesPage";
import { SeguridadPage } from "@/features/bancos/pages/SeguridadPage";
import { UsuariosPage } from "@/features/bancos/pages/UsuariosPage";
import { NotificacionesPage } from "@/features/config/pages/NotificacionesPage";
import { ChangePasswordPage } from "@/features/auth/ChangePasswordPage";
// Módulo CxP (Fase 2) — la Bandeja es la superficie de trabajo única.
import { BandejaPage } from "@/features/cxp/pages/BandejaPage";
import { ProveedoresPage } from "@/features/cxp/pages/ProveedoresPage";
import { DepartamentosPage } from "@/features/cxp/pages/DepartamentosPage";
import { ContabilidadPage } from "@/features/cxp/pages/ContabilidadPage";
import { ValidacionRiesgoPage } from "@/features/cxp/pages/ValidacionRiesgoPage";
import { AnticiposPage } from "@/features/cxp/pages/AnticiposPage";
import { CajaChicaPage } from "@/features/cxp/pages/CajaChicaPage";
import { NuevoDocumentoPage } from "@/features/cxp/pages/NuevoDocumentoPage";
import { DocumentoDetailPage } from "@/features/cxp/pages/DocumentoDetailPage";
import { ImportarPage } from "@/features/cxp/pages/ImportarPage";
import { DashboardCxpPage } from "@/features/cxp/pages/DashboardCxpPage";
// Módulo CxC (fase 1) — cartera sobre partidas abiertas.
import { CarteraPage } from "@/features/cxc/pages/CarteraPage";
import { ColaPage } from "@/features/cxc/pages/ColaPage";
import { ParametrosCxcPage } from "@/features/cxc/pages/ParametrosCxcPage";
import { ContratoCxcPage } from "@/features/cxc/pages/ContratoCxcPage";
import { ImportarCxcPage } from "@/features/cxc/pages/ImportarCxcPage";
import { CobrosPage } from "@/features/cxc/pages/CobrosPage";
import { AsociacionesPage } from "@/features/cxc/pages/AsociacionesPage";
import { PreventivoPage } from "@/features/cxc/pages/PreventivoPage";
import { ArreglosPage } from "@/features/cxc/pages/ArreglosPage";
// Módulo RRHH / Nómina (Fase 3 — Etapas 1 y 2)
import { EmpleadosPage } from "@/features/rrhh/pages/EmpleadosPage";
import { ParametrosNominaPage } from "@/features/rrhh/pages/ParametrosNominaPage";
import { CorridasPage } from "@/features/rrhh/pages/CorridasPage";
import { FiniquitosPage } from "@/features/rrhh/pages/FiniquitosPage";
import { AusenciasPage } from "@/features/rrhh/pages/AusenciasPage";
import { DashboardRRHHPage } from "@/features/rrhh/pages/DashboardRRHHPage";

/**
 * Definición de rutas.
 *
 *  /login                -> solo NO autenticados.
 *  /seleccionar-empresa  -> autenticado; la propia página resuelve redirecciones.
 *  /                     -> shell protegido (requiere sesión + empresa activa).
 *                           Contiene el módulo Bancos (Fase 1).
 */
export const router = createBrowserRouter([
  {
    element: <PublicOnlyRoute />,
    children: [{ path: "/login", element: <LoginPage /> }],
  },
  {
    // La página gestiona internamente los estados (sin sesión / con empresa).
    path: "/seleccionar-empresa",
    element: <SelectEmpresaPage />,
  },
  {
    // Cambio de contraseña obligatorio (contraseña temporal / primer ingreso).
    path: "/cambiar-password",
    element: <ChangePasswordPage />,
  },
  {
    element: <ProtectedRoute />,
    children: [
      {
        element: <AppShell />,
        children: [
          { index: true, element: <DashboardPage /> },
          { path: "importador", element: <ImportadorPage /> },
          { path: "clasificar", element: <ClasificarPage /> },
          // Rutas viejas → Clasificar (Movimientos y Revisar se fundieron en la Bandeja).
          { path: "movimientos", element: <Navigate to="/clasificar" replace /> },
          { path: "revisar", element: <Navigate to="/clasificar" replace /> },
          { path: "catalogo", element: <CatalogoPage /> },
          { path: "tipo-cambio", element: <TipoCambioPage /> },
          { path: "exportar", element: <ExportarPage /> },
          { path: "analisis", element: <AnalisisPage /> },
          { path: "proyecciones", element: <ProyeccionesPage /> },
          { path: "saldos-diarios", element: <SaldosDiariosPage /> },
          // Ruta vieja (se llamó «tesoreria» un día): el nombre chocaba con el módulo futuro.
          { path: "tesoreria", element: <Navigate to="/saldos-diarios" replace /> },
          { path: "conciliacion", element: <ConciliacionPage /> },
          { path: "ajustes", element: <AjustesPage /> },
          { path: "seguridad", element: <SeguridadPage /> },
          { path: "usuarios", element: <UsuariosPage /> },
          { path: "notificaciones", element: <NotificacionesPage /> },
          // Módulo CxP (Fase 2) — Bandeja única + páginas de apoyo.
          { path: "cxp/bandeja", element: <BandejaPage /> },
          { path: "cxp/dashboard", element: <DashboardCxpPage /> },
          { path: "cxp/importar", element: <ImportarPage /> },
          { path: "cxp/proveedores", element: <ProveedoresPage /> },
          { path: "cxp/anticipos", element: <AnticiposPage /> },
          { path: "cxp/cajas", element: <CajaChicaPage /> },
          { path: "cxp/departamentos", element: <DepartamentosPage /> },
          { path: "cxp/contabilidad", element: <ContabilidadPage /> },
          { path: "cxp/validacion", element: <ValidacionRiesgoPage /> },
          { path: "cxp/documentos/nuevo", element: <NuevoDocumentoPage /> },
          { path: "cxp/documentos/:id", element: <DocumentoDetailPage /> },
          // Rutas viejas → Bandeja (Flujo/Documentos/Tesorería/Lotes/Pagos se fundieron en ella).
          { path: "cxp/flujo", element: <Navigate to="/cxp/bandeja" replace /> },
          { path: "cxp/documentos", element: <Navigate to="/cxp/bandeja" replace /> },
          { path: "cxp/tesoreria", element: <Navigate to="/cxp/bandeja" replace /> },
          { path: "cxp/lotes", element: <Navigate to="/cxp/bandeja" replace /> },
          { path: "cxp/pagos", element: <Navigate to="/cxp/bandeja" replace /> },
          // Módulo CxC (fase 1) — cartera y cargos.
          { path: "cxc/cola", element: <ColaPage /> },
          { path: "cxc/preventivo", element: <PreventivoPage /> },
          { path: "cxc/arreglos", element: <ArreglosPage /> },
          { path: "cxc/cartera", element: <CarteraPage /> },
          { path: "cxc/cobros", element: <CobrosPage /> },
          { path: "cxc/asociaciones", element: <AsociacionesPage /> },
          { path: "cxc/importar", element: <ImportarCxcPage /> },
          { path: "cxc/parametros", element: <ParametrosCxcPage /> },
          { path: "cxc/contratos/:numero", element: <ContratoCxcPage /> },
          { path: "cxc", element: <Navigate to="/cxc/cartera" replace /> },
          // Módulo RRHH / Nómina (Fase 3 — Etapas 1 y 2).
          { path: "rrhh", element: <DashboardRRHHPage /> },
          { path: "rrhh/empleados", element: <EmpleadosPage /> },
          { path: "rrhh/corridas", element: <CorridasPage /> },
          { path: "rrhh/ausencias", element: <AusenciasPage /> },
          { path: "rrhh/finiquitos", element: <FiniquitosPage /> },
          { path: "rrhh/parametros", element: <ParametrosNominaPage /> },
        ],
      },
    ],
  },
  { path: "/404", element: <NotFoundPage /> },
  { path: "*", element: <Navigate to="/404" replace /> },
]);
