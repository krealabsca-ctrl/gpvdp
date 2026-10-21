/**
 * Mandar el comprobante de pago por WhatsApp.
 *
 * EL TEXTO ES EL DEL CORREO. Lo arma el servidor con la misma plantilla: dos textos para el mismo
 * hecho terminan diciendo cosas distintas, y el proveedor que recibe los dos no sabe cuál vale.
 *
 * EL PDF NO SE PUEDE ADJUNTAR, Y NO ES UNA LIMITACIÓN NUESTRA. El enlace `wa.me/<número>?text=…`
 * abre el chat con el mensaje escrito y nada más: WhatsApp no admite adjuntar un archivo por URL.
 * Por eso este diálogo DESCARGA el comprobante antes de abrir el chat y lo dice con todas las
 * letras — un botón que prometiera mandar el PDF estaría mintiendo, y quien lo usara creería que
 * el proveedor recibió algo que no recibió.
 *
 * EL TELÉFONO SE ESCRIBE ACÁ (decisión del Director Financiero, 19-oct-2026): los 649 proveedores
 * tienen el campo vacío. El número que se escriba queda guardado en la ficha, así que es la
 * primera vez y no todas.
 */

import { useState } from "react";
import { createPortal } from "react-dom";
import { Button, useToast } from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { cxpApi } from "@/api/cxp";

export function WhatsAppComprobante({
  documentoId,
  proveedor,
  /** Teléfono de la ficha, si lo tiene. Precarga el campo: escribirlo de nuevo cada vez sería
      castigar el caso normal para habilitar la excepción. */
  telefonoFicha = "",
  onCerrar,
}: {
  documentoId: string;
  proveedor: string;
  telefonoFicha?: string;
  onCerrar: () => void;
}) {
  const toast = useToast();
  const [telefono, setTelefono] = useState(telefonoFicha);
  const [pendiente, setPendiente] = useState(false);

  async function abrir() {
    setPendiente(true);
    try {
      // El servidor arma el texto y normaliza el número: si no sirve, contesta con la frase que
      // dice qué escribir en vez de abrir un chat vacío.
      const msj = await cxpApi.mensajeWhatsApp(documentoId, telefono);

      // El PDF primero: si el navegador bloqueara la pestaña de WhatsApp, al menos el archivo ya
      // está descargado y el envío se puede terminar a mano.
      const blob = await cxpApi.descargarComprobante(documentoId);
      const urlArchivo = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = urlArchivo;
      a.download = msj.archivo || "comprobante.pdf";
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(urlArchivo);

      const urlChat = `https://wa.me/${msj.telefono}?text=${encodeURIComponent(msj.texto)}`;
      window.open(urlChat, "_blank", "noopener,noreferrer");
      toast.success(`Se descargó ${msj.archivo}: arrastralo al chat para adjuntarlo.`);
      onCerrar();
    } catch (e) {
      toast.error(mensajeError(e));
    } finally {
      setPendiente(false);
    }
  }

  return createPortal(
    <div
      className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && onCerrar()}
    >
      <div className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="mb-1 text-base font-semibold text-content">Enviar por WhatsApp</h2>
        <p className="mb-4 text-xs text-content-muted">{proveedor}</p>

        <label className="mb-1 block text-xs font-medium uppercase tracking-wide text-content-muted">
          Número de WhatsApp
        </label>
        <input
          value={telefono}
          onChange={(e) => setTelefono(e.target.value)}
          autoFocus
          placeholder="8888-8888"
          className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-content placeholder:text-content-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        />
        <p className="mb-4 mt-1 text-[11px] text-content-muted">
          {telefonoFicha
            ? "Es el número de la ficha. Podés cambiarlo para este envío; la ficha no se toca."
            : "Este proveedor no tiene número en la ficha: el que escribas queda guardado ahí."}
        </p>

        {/* Esto NO es una advertencia de relleno: es el único paso que el sistema no puede hacer. */}
        <div className="mb-4 rounded-lg border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-xs text-content">
          WhatsApp no deja adjuntar archivos desde un enlace. Al continuar se{" "}
          <b>descarga el comprobante</b> y se abre el chat con el mensaje ya escrito:{" "}
          <b>arrastrá el PDF al chat</b> antes de enviarlo.
        </div>

        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onCerrar}>
            Cancelar
          </Button>
          <Button onClick={() => void abrir()} loading={pendiente} disabled={!telefono.trim()}>
            Abrir WhatsApp
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
