import { Outlet, useLocation } from "react-router-dom";
import { motion } from "framer-motion";
import { PeriodoProvider } from "@/app/PeriodoProvider";
import { TopBar } from "@/components/shell/TopBar";
import { ModuleRail } from "@/components/shell/ModuleRail";
import { Sidebar } from "@/components/shell/Sidebar";
import { useSidebarColapsada } from "@/components/shell/useSidebarColapsada";
import { CommandPalette } from "@/components/shell/CommandPalette";
import { PermisoGate } from "@/routes/PermisoRoute";

/**
 * Layout autenticado: navbar + riel de módulos + sidebar contextual + contenido.
 * El período activo es contexto global (PeriodoProvider) compartido por la navbar
 * y todas las pantallas. Command palette (⌘/Ctrl+K) montado a nivel de shell.
 */
export function AppShell() {
  const location = useLocation();
  // El estado vive acá y no dentro de la sidebar: el atajo Ctrl+B tiene que funcionar también
  // cuando está colapsada, y entonces el componente que lo escucha no puede ser el que desaparece.
  const sidebar = useSidebarColapsada();
  return (
    <PeriodoProvider>
      <div className="flex min-h-screen flex-col bg-surface">
        <TopBar />
        <div className="flex flex-1 overflow-hidden">
          <ModuleRail />
          <Sidebar estado={sidebar} />
          <main className="flex-1 overflow-auto p-6 lg:p-8">
            <motion.div
              key={location.pathname}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.22, ease: "easeOut" }}
            >
              <PermisoGate>
                <Outlet />
              </PermisoGate>
            </motion.div>
          </main>
        </div>
        <CommandPalette />
      </div>
    </PeriodoProvider>
  );
}
