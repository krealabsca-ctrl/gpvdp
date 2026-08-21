/**
 * Diálogo de confirmación reutilizable para acciones sensibles/irreversibles
 * (cerrar período, congelar TC…). Generaliza el patrón de MotivoDialog de CxP:
 * portal + overlay con cierre por click-fuera + [Cancelar]/[Confirmar] con loading.
 * Opcional: lista de impacto y campo de motivo/nota.
 */

import { useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Button } from "@/components/ui/Button";

export interface ConfirmDialogProps {
  titulo: string;
  descripcion?: ReactNode;
  /** Puntos de impacto que se muestran destacados antes de confirmar. */
  impacto?: string[];
  textoConfirmar?: string;
  /** "peligro" pinta el botón de confirmar en tono negativo. */
  tono?: "accent" | "peligro";
  pendiente?: boolean;
  /** Si true, muestra un textarea y pasa la nota a onConfirmar. */
  pedirNota?: boolean;
  notaPlaceholder?: string;
  onConfirmar: (nota: string) => void;
  onCancelar: () => void;
}

export function ConfirmDialog({
  titulo,
  descripcion,
  impacto,
  textoConfirmar = "Confirmar",
  tono = "accent",
  pendiente = false,
  pedirNota = false,
  notaPlaceholder,
  onConfirmar,
  onCancelar,
}: ConfirmDialogProps) {
  const [nota, setNota] = useState("");
  return createPortal(
    <div
      className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && onCancelar()}
      role="dialog"
      aria-modal="true"
    >
      <div className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="mb-1 text-base font-semibold text-content">{titulo}</h2>
        {descripcion && <div className="mb-3 text-sm text-content-muted">{descripcion}</div>}
        {impacto && impacto.length > 0 && (
          <ul className="mb-3 flex flex-col gap-1 rounded-lg border border-brand-gold/40 bg-brand-gold/10 px-3 py-2 text-xs text-content">
            {impacto.map((i, k) => (
              <li key={k} className="flex gap-1.5">
                <span aria-hidden className="text-brand-gold">•</span>
                <span>{i}</span>
              </li>
            ))}
          </ul>
        )}
        {pedirNota && (
          <textarea
            value={nota}
            onChange={(e) => setNota(e.target.value)}
            autoFocus
            placeholder={notaPlaceholder ?? "Motivo / detalle…"}
            className="mb-3 min-h-20 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-content placeholder:text-content-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
          />
        )}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onCancelar} disabled={pendiente}>
            Cancelar
          </Button>
          <Button
            onClick={() => onConfirmar(nota.trim())}
            loading={pendiente}
            className={tono === "peligro" ? "!bg-negativo !text-white hover:!bg-negativo/90" : undefined}
          >
            {textoConfirmar}
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
