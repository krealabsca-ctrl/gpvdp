/**
 * Command palette (⌘/Ctrl+K): salto rápido a cualquier módulo/pantalla.
 * Se abre con el atajo o con el evento "gpvdp:open-command" (botón de la navbar).
 * Navegación por teclado (↑/↓/Enter/Esc). Solo lista páginas de módulos disponibles.
 */

import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { useNavigate } from "react-router-dom";
import { AnimatePresence, motion } from "framer-motion";
import { Search } from "lucide-react";
import { cn } from "@/lib/cn";
import { todasLasPaginas } from "@/app/nav";
import { useTienePermiso } from "@/features/auth/permisos";

export function CommandPalette() {
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const tienePermiso = useTienePermiso();

  // Solo ofrece saltos a páginas que el usuario puede ver (defensa en profundidad).
  const paginas = useMemo(
    () => todasLasPaginas().filter((p) => !p.permisoEfectivo || tienePermiso(p.permisoEfectivo)),
    [tienePermiso],
  );
  const filtradas = useMemo(() => {
    const t = q.trim().toLowerCase();
    if (!t) return paginas;
    return paginas.filter((p) => `${p.label} ${p.modulo} ${p.descripcion}`.toLowerCase().includes(t));
  }, [q, paginas]);

  useEffect(() => {
    function onKey(e: globalThis.KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((o) => !o);
      } else if (e.key === "Escape") {
        setOpen(false);
      }
    }
    function onOpen() {
      setOpen(true);
    }
    window.addEventListener("keydown", onKey);
    window.addEventListener("gpvdp:open-command", onOpen);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("gpvdp:open-command", onOpen);
    };
  }, []);

  useEffect(() => {
    if (!open) return undefined;
    setQ("");
    setSel(0);
    const id = window.setTimeout(() => inputRef.current?.focus(), 20);
    return () => window.clearTimeout(id);
  }, [open]);

  useEffect(() => {
    setSel(0);
  }, [q]);

  function irA(to: string) {
    setOpen(false);
    navigate(to);
  }

  function onInputKey(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSel((s) => Math.min(s + 1, filtradas.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSel((s) => Math.max(s - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const item = filtradas[sel];
      if (item) irA(item.to);
    }
  }

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          className="fixed inset-0 z-[60] flex items-start justify-center bg-black/40 p-4 pt-[12vh]"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.12 }}
          onMouseDown={() => setOpen(false)}
          role="dialog"
          aria-modal="true"
          aria-label="Buscar módulo o pantalla"
        >
          <motion.div
            className="w-full max-w-lg overflow-hidden rounded-xl border border-border bg-surface-raised shadow-lifted"
            initial={{ opacity: 0, y: -8, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -8, scale: 0.98 }}
            transition={{ duration: 0.14, ease: "easeOut" }}
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="flex items-center gap-2 border-b border-border px-3">
              <Search className="h-4 w-4 shrink-0 text-content-muted" aria-hidden />
              <input
                ref={inputRef}
                value={q}
                onChange={(e) => setQ(e.target.value)}
                onKeyDown={onInputKey}
                placeholder="Buscar módulo o pantalla…"
                className="h-12 w-full bg-transparent text-sm text-content placeholder:text-content-muted focus:outline-none"
                aria-label="Buscar"
              />
            </div>
            <ul className="max-h-80 overflow-y-auto p-2">
              {filtradas.length === 0 ? (
                <li className="px-3 py-6 text-center text-sm text-content-muted">Sin resultados.</li>
              ) : (
                filtradas.map((p, i) => {
                  const Icon = p.icon;
                  return (
                    <li key={`${p.to}-${p.label}`}>
                      <button
                        type="button"
                        onMouseEnter={() => setSel(i)}
                        onClick={() => irA(p.to)}
                        className={cn(
                          "flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left",
                          i === sel ? "bg-accent/10 text-accent" : "text-content hover:bg-surface-muted",
                        )}
                      >
                        <Icon className="h-4 w-4 shrink-0" aria-hidden />
                        <span className="flex min-w-0 flex-col">
                          <span className="truncate text-sm font-medium">{p.label}</span>
                          <span className="truncate text-xs text-content-muted">
                            {p.modulo} · {p.descripcion}
                          </span>
                        </span>
                      </button>
                    </li>
                  );
                })
              )}
            </ul>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
