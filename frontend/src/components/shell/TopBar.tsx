import { useLocation } from "react-router-dom";
import { Bell, ChevronRight, Search } from "lucide-react";
import { useAuth } from "@/features/auth/AuthContext";
import { Button, Select, useToast } from "@/components/ui";
import { ThemeToggle } from "@/components/ThemeToggle";
import { EmpresaSwitcher } from "@/components/shell/EmpresaSwitcher";
import { BrandLogo } from "@/components/shell/BrandLogo";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { etiquetaPeriodo, periodosRecientes } from "@/lib/format";

/** Migaja de pan por ruta (módulo / pantalla). */
function breadcrumb(path: string): { group: string; page: string } {
  if (path === "/") return { group: "Bancos", page: "Dashboard" };
  if (path.startsWith("/importador")) return { group: "Bancos", page: "Importador" };
  if (path.startsWith("/clasificar")) return { group: "Bancos", page: "Clasificar" };
  if (path.startsWith("/catalogo")) return { group: "Bancos", page: "Catálogo" };
  if (path.startsWith("/tipo-cambio")) return { group: "Bancos", page: "Tipo de cambio" };
  if (path.startsWith("/exportar")) return { group: "Bancos", page: "Exportar" };
  if (path.startsWith("/proyecciones")) return { group: "Bancos", page: "Proyecciones" };
  if (path.startsWith("/saldos-diarios")) return { group: "Bancos", page: "Saldos diarios" };
  if (path.startsWith("/conciliacion")) return { group: "Bancos", page: "Actas de conciliación" };
  if (path.startsWith("/cxp/proveedores")) return { group: "Cuentas por pagar", page: "Proveedores" };
  if (path.startsWith("/cxp/documentos/nuevo")) return { group: "Cuentas por pagar", page: "Nuevo documento" };
  if (path.startsWith("/cxp/documentos")) return { group: "Cuentas por pagar", page: "Documentos" };
  if (path.startsWith("/cxp/pagos")) return { group: "Cuentas por pagar", page: "Pagos y conciliación" };
  return { group: "", page: "" };
}

/** Barra superior del shell autenticado: marca, breadcrumb, buscador, contexto (período + empresa) y perfil. */
export function TopBar() {
  const { user, logout } = useAuth();
  const toast = useToast();
  const { periodo, setPeriodo } = usePeriodoActivo();
  const { pathname } = useLocation();
  const crumb = breadcrumb(pathname);

  const periodoOptions = periodosRecientes().map((p) => ({ value: p, label: etiquetaPeriodo(p) }));

  return (
    <header className="flex h-16 items-center gap-3 border-b border-border bg-surface-raised px-4 md:px-6">
      <div className="flex min-w-0 items-center gap-4">
        <BrandLogo size={28} />
        {crumb.page && (
          <div className="hidden items-center gap-1.5 border-l border-border pl-4 text-sm lg:flex">
            <span className="text-content-muted">{crumb.group}</span>
            <ChevronRight className="h-4 w-4 text-content-muted" aria-hidden />
            <span className="font-medium text-content">{crumb.page}</span>
          </div>
        )}
      </div>

      <button
        type="button"
        onClick={() => window.dispatchEvent(new Event("gpvdp:open-command"))}
        className="mx-auto hidden h-9 w-full max-w-xs items-center gap-2 rounded-lg border border-border bg-surface px-3 text-sm text-content-muted transition-colors hover:border-content-muted/50 md:flex"
        aria-label="Buscar (Ctrl+K)"
      >
        <Search className="h-4 w-4" aria-hidden />
        <span className="flex-1 text-left">Buscar…</span>
        <kbd className="rounded border border-border px-1.5 py-0.5 text-[11px] font-medium text-content-muted">
          Ctrl K
        </kbd>
      </button>

      <div className="flex items-center gap-2 sm:gap-3">
        <Select
          aria-label="Período activo"
          value={periodo}
          onChange={(e) => setPeriodo(e.target.value)}
          options={periodoOptions}
          className="hidden h-9 w-40 lg:block"
        />
        <EmpresaSwitcher />
        <button
          type="button"
          onClick={() => toast.info("No tenés notificaciones nuevas.")}
          aria-label="Notificaciones"
          title="Notificaciones"
          className="inline-flex h-9 w-9 items-center justify-center rounded-lg text-content-muted transition-colors hover:bg-surface-muted hover:text-content focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          <Bell className="h-[18px] w-[18px]" aria-hidden />
        </button>
        <ThemeToggle />
        <span
          className="hidden border-l border-border pl-3 text-sm text-content-muted md:inline"
          aria-label="Usuario"
        >
          {user?.nombre}
        </span>
        <Button variant="secondary" size="sm" onClick={logout}>
          Salir
        </Button>
      </div>
    </header>
  );
}
