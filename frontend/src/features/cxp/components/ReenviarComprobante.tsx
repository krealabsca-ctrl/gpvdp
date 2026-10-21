/**
 * Reenviar el comprobante, pudiendo cambiar el destinatario.
 *
 * EL PEDIDO (Director Financiero, 19-oct-2026): «el botón de reenviar debe ver algo como el TO, se
 * puede modificar para enviar a otro destino diferente al enviado».
 *
 * Antes era una confirmación seca: volvía a mandar al correo de la ficha y punto. El contacto de
 * Contabilidad del proveedor casi nunca es el de la ficha, así que la única forma de mandárselo a
 * otra persona era editar la ficha, enviar, y volver a dejarla como estaba.
 *
 * El correo escrito QUEDA GUARDADO en la ficha si estaba vacía —misma decisión que el teléfono de
 * WhatsApp— y si ya tenía uno NO se pisa: un envío puntual no debería reemplazar el contacto
 * oficial del proveedor sin que nadie lo decida. A dónde se mandó queda en la bitácora igual que
 * siempre.
 */

import { useState } from "react";
import { createPortal } from "react-dom";
import { Button } from "@/components/ui";
import { formatFechaHora } from "@/lib/format";
import type { Documento } from "@/api/cxp";

export function ReenviarComprobante({
  doc,
  pendiente,
  onEnviar,
  onCerrar,
}: {
  doc: Documento;
  pendiente: boolean;
  /** `destino` vacío = al correo de la ficha. */
  onEnviar: (destino: string) => void;
  onCerrar: () => void;
}) {
  // Precargado con el correo de la ficha: el caso normal es reenviar ahí, y escribirlo de nuevo
  // cada vez sería castigar el camino habitual para habilitar la excepción.
  const [destino, setDestino] = useState(doc.proveedor_email ?? "");
  const limpio = destino.trim();
  // Validación mínima acá —la de verdad la hace el servidor con net/mail— solo para no mandar un
  // viaje al servidor por un campo obviamente vacío o sin arroba.
  const sirve = limpio.includes("@") && !limpio.includes(" ");

  return createPortal(
    <div
      className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && onCerrar()}
    >
      <div className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="mb-1 text-base font-semibold text-content">Reenviar el comprobante</h2>
        <p className="mb-4 text-xs text-content-muted">
          {doc.proveedor} · {doc.consecutivo || "sin consecutivo"}
        </p>

        <label
          htmlFor="reenviar-para"
          className="mb-1 block text-xs font-medium uppercase tracking-wide text-content-muted"
        >
          Para
        </label>
        <input
          id="reenviar-para"
          type="email"
          value={destino}
          onChange={(e) => setDestino(e.target.value)}
          autoFocus
          placeholder="correo@proveedor.com"
          className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-content placeholder:text-content-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        />
        <p className="mb-4 mt-1 text-[11px] text-content-muted">
          {doc.proveedor_email
            ? "Podés cambiarlo para mandárselo a otra persona; la ficha no se toca."
            : "Este proveedor no tiene correo en la ficha: el que escribas queda guardado ahí."}
        </p>

        <div className="mb-4 rounded-lg border border-border bg-surface-muted px-3 py-2 text-xs text-content-muted">
          Va el mismo PDF, con copia oculta a quien aprobó el pago.
          {doc.comprobante_enviado_en && (
            <>
              {" "}
              Último envío: <b className="text-content">{formatFechaHora(doc.comprobante_enviado_en)}</b>.
            </>
          )}
        </div>

        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onCerrar}>
            Cancelar
          </Button>
          <Button disabled={!sirve} loading={pendiente} onClick={() => onEnviar(limpio)}>
            Reenviar
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
