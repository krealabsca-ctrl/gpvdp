/**
 * Rehacer los cargos cuando la cartera se corrigió.
 *
 * POR QUÉ HACE FALTA. Generar cargos es idempotente —correrlo dos veces no duplica—, y por eso
 * mismo NO corrige uno que ya existe y quedó mal. Cuando la cartera cambia, los cargos viejos
 * siguen ahí con el monto de antes. El caso que lo motivó (13-oct-2026): al empezar a guardar la
 * fecha de próximo pago, 18.450 contratos pasaron a deber menos — ₡1.042.683.725 en total — y no
 * había forma de bajar esa cifra sin borrar cargos a mano en producción.
 *
 * LO QUE PROTEGE, y es la razón de que esto sea una pantalla y no un comando: el servidor solo
 * borra cargos SIN pagos aplicados, sin notas de crédito y sin factura electrónica. Los demás los
 * deja y los cuenta aparte, porque la tabla de aplicaciones referencia al cargo EN CASCADA: un
 * borrado de más no daría error, borraría el registro del pago en silencio.
 *
 * Y pide confirmar el número EXACTO del plan. Si entre mirar y apretar alguien registró un cobro,
 * el servidor contesta 409 y no borra nada: el número dejó de ser el mismo, así que hay que volver
 * a mirar.
 */
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { Button, ConfirmDialog, useToast } from "@/components/ui";
import { cxcApi } from "@/api/cxc";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";
import { formatMoneda } from "@/lib/format";

export function RehacerCargos({ desde, hasta }: { desde: string; hasta: string }) {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();
  const [confirmando, setConfirmando] = useState(false);

  const plan = useQuery({
    queryKey: ["cxc", empresaId, "regeneracion-plan"],
    queryFn: () => cxcApi.planRegeneracion(),
    enabled: !!empresaId,
  });

  const rehacer = useMutation({
    mutationFn: () => cxcApi.regenerarCargos(desde, hasta, plan.data?.borrables ?? -1),
    onSuccess: (r) => {
      toast.success(
        `${r.borrados.toLocaleString("es-CR")} cargos borrados · ${r.creados.toLocaleString("es-CR")} creados`,
      );
      void qc.invalidateQueries({ queryKey: ["cxc", empresaId] });
      setConfirmando(false);
    },
    onError: (e) => {
      toast.error(mensajeError(e));
      setConfirmando(false);
      // El 409 llega cuando el plan cambió mientras se confirmaba: se vuelve a pedir para que
      // el número de la pantalla sea el de ahora y no el que ya no vale.
      void plan.refetch();
    },
  });

  const p = plan.data;
  // Sin cargos no hay nada que rehacer, y el panel no se pinta: un bloque con ceros en una
  // pantalla que ya tiene tres pasos es ruido.
  if (!p || p.total === 0) return null;

  return (
    <div className="mt-4 rounded-lg border border-border bg-surface-raised px-3 py-3">
      <p className="text-sm font-medium">¿La cartera cambió después de generar?</p>
      <p className="mt-0.5 text-xs text-content-muted">
        Generar no corrige los cargos que ya existen. Si volviste a importar la cartera —por
        ejemplo para que entrara la fecha de próximo pago— hay que rehacerlos con el dato nuevo.
      </p>

      <div className="mt-2 text-xs">
        Hoy hay <b>{p.total.toLocaleString("es-CR")}</b> cargos.{" "}
        <b>{p.borrables.toLocaleString("es-CR")}</b> se pueden rehacer (
        {formatMoneda(p.monto_borrable)}).
        {p.se_conservan > 0 && (
          <>
            {" "}
            <span className="text-pendiente">
              {p.se_conservan.toLocaleString("es-CR")} se conservan porque tienen pagos aplicados,
              nota de crédito o factura — ésos no se tocan.
            </span>
          </>
        )}
      </div>

      <Button
        className="mt-3"
        variant="secondary"
        disabled={p.borrables === 0}
        loading={rehacer.isPending}
        onClick={() => setConfirmando(true)}
      >
        Rehacer {p.borrables.toLocaleString("es-CR")} cargos
      </Button>

      {confirmando && (
        <ConfirmDialog
          titulo={`Rehacer ${p.borrables.toLocaleString("es-CR")} cargos`}
          descripcion={
            `Se borran y se vuelven a crear con la cartera de hoy, del ${desde} al ${hasta}. ` +
            "Esto no se puede deshacer."
          }
          // El impacto va en puntos y no enterrado en el párrafo: es lo que hay que leer antes de
          // apretar, y el más importante es el segundo — qué NO se toca.
          impacto={[
            `Se borran ${p.borrables.toLocaleString("es-CR")} cargos por ${formatMoneda(p.monto_borrable)}`,
            p.se_conservan > 0
              ? `Se conservan ${p.se_conservan.toLocaleString("es-CR")} con pagos, nota de crédito o factura`
              : "Ninguno tiene pagos aplicados: no se pierde ningún cobro",
          ]}
          tono="peligro"
          textoConfirmar="Rehacer"
          pendiente={rehacer.isPending}
          onConfirmar={() => rehacer.mutate()}
          onCancelar={() => setConfirmando(false)}
        />
      )}
    </div>
  );
}
