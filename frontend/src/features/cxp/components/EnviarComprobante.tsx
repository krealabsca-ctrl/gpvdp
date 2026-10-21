/**
 * Las dos vías de mandar el comprobante, en UN control.
 *
 * Antes eran dos botones sueltos —«Reenviar al proveedor» y «Enviar por WhatsApp»— peleando por el
 * ancho de la fila con otros tres. Son la misma decisión («mandarle el comprobante al proveedor»)
 * tomada por canales distintos, así que van juntas: el botón hace lo habitual y el desplegable
 * ofrece el otro camino.
 *
 * El botón primario es el CORREO porque es el que deja rastro solo: queda en la bitácora del
 * documento sin que nadie haga nada. Por WhatsApp el envío lo termina una persona a mano, y eso no
 * se puede registrar como enviado.
 */

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatFechaHora } from "@/lib/format";
import type { Documento } from "@/api/cxp";

export function EnviarComprobante({
  doc,
  pendiente,
  onCorreo,
  onWhatsApp,
  size = "sm",
}: {
  doc: Documento;
  pendiente: boolean;
  onCorreo: () => void;
  onWhatsApp: () => void;
  size?: "sm" | "md";
}) {
  const [abierto, setAbierto] = useState(false);
  const caja = useRef<HTMLDivElement>(null);
  const yaSalio = Boolean(doc.comprobante_enviado_en);

  // Se cierra al hacer clic afuera o con Escape: un menú que se queda abierto tapa la fila de
  // abajo, y en una tabla eso esconde justo lo que se estaba por mirar.
  useEffect(() => {
    if (!abierto) return;
    function fuera(e: MouseEvent) {
      if (caja.current && !caja.current.contains(e.target as Node)) setAbierto(false);
    }
    function esc(e: KeyboardEvent) {
      if (e.key === "Escape") setAbierto(false);
    }
    document.addEventListener("mousedown", fuera);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", fuera);
      document.removeEventListener("keydown", esc);
    };
  }, [abierto]);

  return (
    <div ref={caja} className="relative inline-flex flex-col items-end">
      <div className="inline-flex">
        <Button
          size={size}
          variant={yaSalio ? "secondary" : "primary"}
          loading={pendiente}
          onClick={onCorreo}
          className="rounded-r-none"
        >
          ✉ {yaSalio ? "Reenviar" : "Enviar"}
        </Button>
        <Button
          size={size}
          variant={yaSalio ? "secondary" : "primary"}
          onClick={() => setAbierto((v) => !v)}
          aria-label="Otras formas de enviar"
          aria-expanded={abierto}
          className="rounded-l-none border-l border-black/10 px-2"
        >
          ▾
        </Button>
      </div>
      {/* La hora del último envío vive acá y no en otra columna: al reenviar, lo que decide es si
          el anterior fue hace un rato o la semana pasada. */}
      {yaSalio && (
        <span className="mt-0.5 text-[10px] text-content-muted">
          {formatFechaHora(doc.comprobante_enviado_en)}
        </span>
      )}
      {abierto && (
        <div
          className={cn(
            "absolute right-0 top-full z-30 mt-1 w-56 overflow-hidden rounded-lg border border-border",
            "bg-surface-raised shadow-lifted",
          )}
        >
          <button
            type="button"
            className="block w-full px-3 py-2 text-left text-sm text-content hover:bg-surface-muted"
            onClick={() => {
              setAbierto(false);
              onCorreo();
            }}
          >
            ✉ Por correo…
            <span className="block text-[11px] text-content-muted">Se puede cambiar el destinatario</span>
          </button>
          <button
            type="button"
            className="block w-full border-t border-border px-3 py-2 text-left text-sm text-content hover:bg-surface-muted"
            onClick={() => {
              setAbierto(false);
              onWhatsApp();
            }}
          >
            💬 Por WhatsApp…
            <span className="block text-[11px] text-content-muted">El PDF se adjunta a mano</span>
          </button>
        </div>
      )}
    </div>
  );
}
