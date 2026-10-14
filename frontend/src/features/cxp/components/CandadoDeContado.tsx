/**
 * Las facturas que quedaron trancadas por el candado del contado, en la ficha del proveedor.
 *
 * EL PEDIDO (Director Financiero, 12-oct-2026): «necesitamos generar una huella para ciertas
 * facturas que el proveedor lo pone como contado pero no lo es, porque de lo contrario quedaría
 * sucio el sistema y facturas sin pagar cuando deben pagarse».
 *
 * POR QUÉ ACÁ Y NO EN UNA PANTALLA APARTE. Marcar «manda el convenio» arregla las facturas que
 * entren de ahora en adelante, pero no toca las que ya están: el candado está escrito en cada
 * una. Si el aviso no aparece en el mismo lugar donde se tomó esa decisión, hay que acordarse de
 * ir después a otra pantalla — y nadie se acuerda. Acá se ve el problema, se ve la lista y se
 * resuelve de un gesto.
 *
 * LO QUE NO HACE: no libera solo. Liberar es afirmar «esta factura NO se pagó por caja chica», y
 * esa es la única barrera que queda contra el doble pago. Por eso se muestran las facturas con
 * su monto ANTES de liberarlas —firmar sobre una lista que no se vio es firmar en blanco— y por
 * eso el motivo es obligatorio.
 */
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { Badge, Button, Input, useToast } from "@/components/ui";
import { candadoApi, type FacturaDeContado } from "@/api/cxp";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { useTienePermiso } from "@/features/auth/permisos";
import { mensajeError } from "@/lib/apiError";
import { formatMoneda } from "@/lib/format";

/** Mínimo del motivo. Igual que en el servidor: descarta el «ok» reflejo, no exige un ensayo. */
const MOTIVO_MINIMO = 20;

export function CandadoDeContado({ proveedorId }: { proveedorId: string }) {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();
  const tienePermiso = useTienePermiso();
  const puedeLiberar = tienePermiso("cxp.liberar_pago");

  const [abierto, setAbierto] = useState(false);
  const [motivo, setMotivo] = useState("");
  const [excluidas, setExcluidas] = useState<Set<string>>(new Set());

  const q = useQuery({
    queryKey: ["cxp", empresaId, "bloqueadas", proveedorId],
    queryFn: () => candadoApi.bloqueadas(proveedorId),
    enabled: !!empresaId && !!proveedorId,
  });

  const facturas = q.data?.facturas ?? [];
  const elegidas = facturas.filter((f) => !excluidas.has(f.documento_id));

  const liberar = useMutation({
    mutationFn: () => candadoApi.liberar(proveedorId, elegidas.map((f) => f.documento_id), motivo),
    onSuccess: (r) => {
      const fallaron = Object.keys(r.no_se_pudo).length;
      if (r.liberadas.length > 0) {
        toast.success(
          `${r.liberadas.length} factura(s) liberada(s)` + (fallaron > 0 ? ` · ${fallaron} no se pudo` : ""),
        );
      } else {
        toast.info("No se liberó ninguna. Volvé a abrir la lista.");
      }
      setAbierto(false);
      setMotivo("");
      setExcluidas(new Set());
      void qc.invalidateQueries({ queryKey: ["cxp", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  // Sin facturas trancadas no se pinta nada: un panel vacío en cada uno de los 649 proveedores
  // sería ruido permanente.
  if (facturas.length === 0) return null;

  const total = facturas.reduce((s, f) => s + Number(f.total || 0), 0);

  return (
    <div className="rounded-lg border border-pendiente/40 bg-pendiente/5 p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <p className="text-sm font-medium text-pendiente">
            🔒 {facturas.length} factura(s) de este proveedor no se pueden pagar por banco
          </p>
          <p className="mt-0.5 text-xs text-content-muted">
            El comprobante las declaró de contado, así que nacieron bloqueadas para no pagarlas dos
            veces (pudieron salir ya por caja chica). Marcar «manda el convenio» arriba arregla las
            próximas, pero no éstas.
          </p>
        </div>
        <Badge tone="pendiente">{formatMoneda(String(total), "CRC")}</Badge>
      </div>

      {!abierto ? (
        puedeLiberar ? (
          <Button size="sm" variant="secondary" className="mt-2" onClick={() => setAbierto(true)}>
            Revisarlas y liberarlas
          </Button>
        ) : (
          // El permiso lo decide el servidor; acá solo se explica por qué no hay botón. Un panel
          // que dice el problema y no dice quién lo resuelve manda a preguntar a ciegas.
          <p className="mt-2 text-xs text-content-muted">
            Liberarlas requiere el permiso «Liberar una factura de contado para pagarla por banco».
          </p>
        )
      ) : (
        <div className="mt-3 space-y-3">
          <ul className="max-h-56 space-y-1 overflow-auto rounded border border-border bg-surface p-2 text-xs">
            {facturas.map((f) => (
              <FilaFactura
                key={f.documento_id}
                f={f}
                elegida={!excluidas.has(f.documento_id)}
                onTogglear={() =>
                  setExcluidas((prev) => {
                    const s = new Set(prev);
                    if (s.has(f.documento_id)) s.delete(f.documento_id);
                    else s.add(f.documento_id);
                    return s;
                  })
                }
              />
            ))}
          </ul>

          <Input
            label="Por qué estas facturas sí se pagan por banco"
            value={motivo}
            onChange={(e) => setMotivo(e.target.value)}
            placeholder="Ej.: el emisor declara contado por costumbre; el convenio firmado es a 8 días."
          />
          <p className="-mt-2 text-xs text-content-muted">
            Queda guardado en cada factura con tu nombre y la fecha. Es lo que permite reconstruir
            después quién autorizó el pago, así que escribí la razón de verdad.
          </p>

          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              loading={liberar.isPending}
              disabled={elegidas.length === 0 || motivo.trim().length < MOTIVO_MINIMO}
              onClick={() => liberar.mutate()}
            >
              Liberar {elegidas.length} factura(s)
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setAbierto(false)}>
              Cancelar
            </Button>
            {motivo.trim().length > 0 && motivo.trim().length < MOTIVO_MINIMO && (
              <span className="text-xs text-pendiente">
                Faltan {MOTIVO_MINIMO - motivo.trim().length} caracteres del motivo.
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function FilaFactura(props: { f: FacturaDeContado; elegida: boolean; onTogglear: () => void }) {
  const { f } = props;
  return (
    <li className="flex items-center gap-2">
      <input
        type="checkbox"
        checked={props.elegida}
        onChange={props.onTogglear}
        aria-label={`Liberar ${f.consecutivo || f.documento_id}`}
      />
      <span className="font-mono">{f.consecutivo || "(sin consecutivo)"}</span>
      <span className="text-content-muted">{f.fecha_emision}</span>
      <span className="ml-auto tabular-nums">{formatMoneda(f.total, f.moneda === "USD" ? "USD" : "CRC")}</span>
    </li>
  );
}
