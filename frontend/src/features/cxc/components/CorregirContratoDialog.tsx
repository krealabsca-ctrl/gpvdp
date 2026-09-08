/**
 * Corregir un contrato que la carga dejó incompleto.
 *
 * Existe porque los contratos apartados estaban en un callejón sin salida: se veían con el filtro
 * «Solo en revisión», quedaban fuera de la cola de cobro y del preventivo, y no había forma de
 * arreglarlos sin corregir el archivo del origen y volver a importar. En Coopeprofa eso era el 20 %
 * de la cartera (2.451 de 12.231).
 *
 * Dos cosas que el diálogo dice en voz alta, porque son las que la gente pregunta después:
 *   · la marca de revisión NO se quita a mano: cae sola cuando el dato queda completo;
 *   · corregir la cuota afecta los cargos que se generen DE AQUÍ EN ADELANTE, no los ya emitidos.
 */

import { useState } from "react";
import { Button, Input, Select, useToast } from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { useCorregirContrato } from "@/features/cxc/hooks";
import type { CatalogosCxc, ContratoCxc } from "@/api/cxc";

export function CorregirContratoDialog({
  contrato,
  catalogos,
  onCerrar,
}: {
  contrato: ContratoCxc;
  catalogos?: CatalogosCxc;
  onCerrar: () => void;
}) {
  const toast = useToast();
  const corregir = useCorregirContrato();

  const [cuota, setCuota] = useState(
    // Una cuota en cero se muestra vacía: precargar «0» invita a guardarlo igual, y es justo el
    // valor que no resuelve nada.
    contrato.cuota_vigente && Number(contrato.cuota_vigente) > 0 ? contrato.cuota_vigente : "",
  );
  const [diaPago, setDiaPago] = useState(contrato.dia_pago ? String(contrato.dia_pago) : "");
  const [modalidadId, setModalidadId] = useState("");
  const [nota, setNota] = useState("");

  const guardar = () => {
    // Viaja lo que la persona escribió, sin comparar contra el valor actual: quien juzga si el dato
    // sirve es el servidor. Al compararlo acá, escribir «0» en un contrato que ya tenía 0 salía como
    // «no cambiaste nada» en vez de explicar que una cuota en cero no resuelve la revisión —el
    // cliente contestando algo que no le toca decidir—.
    const cambios: { cuota?: string; dia_pago?: number; modalidad_id?: string; nota?: string } = {};
    if (cuota.trim() !== "") cambios.cuota = cuota.trim();
    if (diaPago.trim() !== "") cambios.dia_pago = Number(diaPago);
    if (modalidadId) cambios.modalidad_id = modalidadId;
    if (nota.trim()) cambios.nota = nota.trim();

    // Lo único que sí se resuelve acá es no mandar un pedido vacío.
    if (cambios.cuota === undefined && cambios.dia_pago === undefined && !cambios.modalidad_id) {
      toast.error("Escribí al menos la cuota, el día de pago o la modalidad.");
      return;
    }

    corregir.mutate(
      { numero: contrato.numero, ...cambios },
      {
        onSuccess: (c) => {
          toast.success(
            c.revision_pendiente
              ? `Guardado, pero ${c.numero} sigue apartado: ${c.revision_motivo}`
              : `${c.numero} quedó completo y ya entra a la generación de cargos.`,
          );
          onCerrar();
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-lg rounded-lg border border-border bg-surface-raised p-5 shadow-xl">
        <h2 className="text-base font-semibold text-content">Corregir {contrato.numero}</h2>
        <p className="mt-1 text-xs text-content-muted">
          {contrato.cliente_nombre} · {contrato.sede || "sin sede"}
        </p>

        {contrato.revision_pendiente && (
          <div className="mt-3 rounded-md border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-xs">
            Está apartado porque <b>{contrato.revision_motivo || "el dato del origen quedó incompleto"}</b>.
            No genera cargos y no aparece en la cola de cobro.
          </div>
        )}

        <div className="mt-4 flex flex-col gap-3">
          <Input
            label="Cuota"
            hint="Tiene que ser mayor que cero: es lo que se va a cobrar cada período."
            value={cuota}
            onChange={(e) => setCuota(e.target.value)}
            placeholder="4700"
            inputMode="decimal"
          />
          <Input
            label="Día de pago"
            hint="Del 1 al 31. Si el mes no llega a ese día, el cargo se topa al último."
            value={diaPago}
            onChange={(e) => setDiaPago(e.target.value)}
            placeholder="30"
            inputMode="numeric"
          />
          <div>
            <Select
              label="Modalidad"
              value={modalidadId}
              onChange={(e) => setModalidadId(e.target.value)}
              options={[
                { value: "", label: contrato.modalidad ? "Sin cambios" : "Elegí una modalidad" },
                ...(catalogos?.modalidades ?? []).map((m) => ({ value: m.id, label: m.nombre })),
              ]}
            />
            <p className="mt-1 text-[11px] text-content-muted">
              {contrato.modalidad
                ? `Hoy es «${contrato.modalidad}». Dejalo sin tocar para no cambiarla.`
                : "Este contrato no tiene modalidad, así que no puede generar cargos."}
            </p>
          </div>
          <Input
            label="Nota (queda en la bitácora)"
            hint="Con qué respaldo se corrigió. Se guarda junto con el antes y el después."
            value={nota}
            onChange={(e) => setNota(e.target.value)}
            placeholder="cuota confirmada con el expediente físico"
          />
        </div>

        <p className="mt-3 text-[11px] text-content-muted">
          La marca de «revisar» no se quita a mano: cae sola cuando el dato queda completo. Y la cuota
          nueva rige para los cargos que se generen de aquí en adelante — los ya emitidos no se tocan.
        </p>

        <div className="mt-4 flex justify-end gap-2">
          <Button variant="secondary" onClick={onCerrar} disabled={corregir.isPending}>
            Cancelar
          </Button>
          <Button onClick={guardar} loading={corregir.isPending}>
            Guardar corrección
          </Button>
        </div>
      </div>
    </div>
  );
}
