/**
 * Los DOS papeles de un pago, como estado y como acción a la vez.
 *
 * EL PEDIDO (Director Financiero, 19-oct-2026): «afuera en cada factura debería verse la factura
 * original, el comprobante de pago, enviar el comprobante por correo o whatsapp, y demás».
 *
 * Y el problema de forma que venía con él: la fila ya tenía cinco botones sueltos compitiendo
 * —adjuntar, enviar, comprobante, factura, ver— y cada cosa que se agregaba la volvía más difícil
 * de leer. Acá los dos documentos son UN control: el chip dice si está y, del mismo clic, lo abre
 * o lo pide.
 *
 * El que falta NO se esconde. Mostrar solo lo que existe obliga a deducir lo que falta leyendo
 * toda la columna; con el chip apagado, un expediente incompleto se ve de un vistazo.
 */

import { useRef, useState, type ChangeEvent } from "react";
import { cn } from "@/lib/cn";
import { mensajeError } from "@/lib/apiError";
import { useToast } from "@/components/ui";
import { useAdjuntarComprobante } from "@/features/cxp/hooks";
import { cxpApi, type Documento, type TipoAdjunto } from "@/api/cxp";

/** Lo que se muestra de cada papel. El orden es el del expediente: primero se factura, luego se paga. */
const PAPELES: { tipo: TipoAdjunto; icono: string; nombre: string; falta: string }[] = [
  {
    tipo: "FACTURA",
    icono: "📄",
    nombre: "Factura",
    falta: "Adjuntar la factura del proveedor",
  },
  {
    tipo: "COMPROBANTE",
    icono: "🧾",
    nombre: "Comprobante",
    falta: "Adjuntar el comprobante del banco",
  },
];

export function ExpedienteDelPago({
  doc,
  puedeAdjuntar,
  /** Compacto: en la fila de una tabla. Suelto: en la tarjeta del detalle. */
  compacto = false,
}: {
  doc: Documento;
  puedeAdjuntar: boolean;
  compacto?: boolean;
}) {
  const toast = useToast();
  const fileRef = useRef<HTMLInputElement>(null);
  const [subiendoTipo, setSubiendoTipo] = useState<TipoAdjunto | null>(null);
  const [abriendo, setAbriendo] = useState<TipoAdjunto | null>(null);
  // El hook invalida el documento solo: así el chip se enciende sin que cada pantalla tenga que
  // acordarse de refrescar, que es la clase de cableado que se olvida en la tercera.
  const adjuntar = useAdjuntarComprobante();

  function tiene(tipo: TipoAdjunto) {
    return tipo === "FACTURA" ? doc.tiene_factura : doc.tiene_comprobante;
  }

  /**
   * Se ABRE en una pestaña, no se descarga.
   *
   * Esto es para MIRAR: validar un pago es cruzar los dos papeles, y llenar la carpeta de
   * descargas de PDF que se leen una vez no ayuda. Quien lo quiera guardado tiene «Descargar».
   */
  async function abrir(tipo: TipoAdjunto) {
    setAbriendo(tipo);
    try {
      const blob = await cxpApi.descargarComprobante(doc.id, tipo);
      const url = URL.createObjectURL(blob);
      window.open(url, "_blank", "noopener,noreferrer");
      // Tarde: revocarlo al instante deja la pestaña nueva sin nada que mostrar.
      setTimeout(() => URL.revokeObjectURL(url), 60_000);
    } catch (e) {
      toast.error(mensajeError(e));
    } finally {
      setAbriendo(null);
    }
  }

  function onFile(e: ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0];
    const tipo = subiendoTipo;
    if (fileRef.current) fileRef.current.value = "";
    if (!f || !tipo) return setSubiendoTipo(null);
    adjuntar.mutate(
      { id: doc.id, archivo: f, tipo },
      {
        onSuccess: () =>
          toast.success(tipo === "FACTURA" ? "Factura adjuntada" : "Comprobante adjuntado"),
        onError: (err) => toast.error(mensajeError(err)),
        onSettled: () => setSubiendoTipo(null),
      },
    );
  }

  return (
    <div className={cn("flex items-center", compacto ? "gap-1" : "gap-2")}>
      <input
        ref={fileRef}
        type="file"
        accept="application/pdf,.pdf,.xml"
        onChange={(e) => void onFile(e)}
        className="sr-only"
        aria-label="Documento del expediente (PDF o XML)"
      />
      {PAPELES.map((p) => {
        const hay = tiene(p.tipo);
        const ocupado = abriendo === p.tipo || subiendoTipo === p.tipo;
        // Sin el papel y sin permiso para adjuntarlo, el chip solo informa: no es un botón.
        const accionable = hay || puedeAdjuntar;
        return (
          <button
            key={p.tipo}
            type="button"
            disabled={!accionable || ocupado}
            title={hay ? `Abrir ${p.nombre.toLowerCase()}` : puedeAdjuntar ? p.falta : `Sin ${p.nombre.toLowerCase()}`}
            onClick={() => {
              if (hay) return void abrir(p.tipo);
              setSubiendoTipo(p.tipo);
              fileRef.current?.click();
            }}
            className={cn(
              "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors",
              compacto ? "text-[11px]" : "text-xs",
              hay
                ? "border-positivo/40 bg-positivo/10 text-positivo hover:border-positivo"
                : accionable
                  ? "border-dashed border-border text-content-muted hover:border-accent hover:text-accent"
                  : "border-dashed border-border text-content-muted opacity-60",
              ocupado && "opacity-60",
            )}
          >
            <span aria-hidden>{p.icono}</span>
            {p.nombre}
            {/* El símbolo final es el estado: ✓ está, + se puede adjuntar, — no hay y no se puede. */}
            <span aria-hidden className="font-semibold">
              {hay ? "✓" : puedeAdjuntar ? "+" : "—"}
            </span>
          </button>
        );
      })}
    </div>
  );
}
