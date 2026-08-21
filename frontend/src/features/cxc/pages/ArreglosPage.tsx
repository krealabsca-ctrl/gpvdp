/**
 * CxC — Arreglos de pago (/cxc/arreglos). La vista del supervisor de piso.
 *
 * Por qué existe como pantalla propia y no solo como panel en la ficha del contrato: los plazos
 * de excepción los autoriza el supervisor **sin tope**, y un permiso sin límite se controla
 * mirando el acumulado. Desde la ficha se ve un arreglo; acá se ve cuántos se pactaron fuera de
 * los plazos estándar, cuántos se rompieron y cuánta plata depende de que estos planes se
 * cumplan. Esa pregunta no se puede contestar abriendo contratos de uno en uno.
 *
 * El encabezado mide LO FILTRADO, como en el resto del ERP, y los contadores de estado sirven de
 * navegación: el resumen se calcula sobre todo el filtro y el estado se aplica después.
 */

import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Select,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import type { BadgeTone } from "@/components/ui";
import { formatFecha, formatMoneda, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import { useAnularArreglo, useArreglos, useQuebrarArreglo } from "@/features/cxc/hooks";
import type { Arreglo } from "@/api/cxc";

const TONO: Record<string, BadgeTone> = {
  AL_DIA: "positivo",
  EN_MORA: "negativo",
  CUMPLIDO: "positivo",
  QUEBRADO: "negativo",
  ANULADO: "neutral",
};

const ETIQUETA: Record<string, string> = {
  AL_DIA: "Al día",
  EN_MORA: "En mora del plan",
  CUMPLIDO: "Cumplido",
  QUEBRADO: "Quebrado",
  ANULADO: "Anulado",
};

export function ArreglosPage() {
  const tiene = useTienePermiso();
  const puedeCerrar = tiene("cxc.arreglos");
  const [estado, setEstado] = useState("");
  const [soloExcepciones, setSoloExcepciones] = useState(false);
  const [contrato, setContrato] = useState("");
  const [cerrar, setCerrar] = useState<{ a: Arreglo; quebrar: boolean } | null>(null);

  const q = useArreglos({
    ...(estado ? { estado } : {}),
    ...(soloExcepciones ? { excepciones: true } : {}),
    ...(contrato.trim() ? { contrato: contrato.trim() } : {}),
  });
  const quebrar = useQuebrarArreglo();
  const anular = useAnularArreglo();
  const toast = useToast();

  const items = q.data?.items ?? [];
  const r = q.data?.resumen;
  const plazos = q.data?.plazos;

  function cerrarArreglo(motivo: string) {
    if (!cerrar) return;
    const mut = cerrar.quebrar ? quebrar : anular;
    mut.mutate(
      { id: cerrar.a.id, motivo },
      {
        onSuccess: () => {
          toast.success(
            cerrar.quebrar
              ? `Arreglo ${cerrar.a.consecutivo} quebrado: el contrato pasa a cartera morosa`
              : `Arreglo ${cerrar.a.consecutivo} anulado`,
          );
          setCerrar(null);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Arreglos de pago"
        description="Los planes pactados sobre deuda vencida. El arreglo no reescribe los cargos: la mora y la antigüedad siguen corriendo."
        actions={
          <Link to="/cxc/cola?morosa=true" className="text-sm font-medium text-accent underline">
            Ver la cartera morosa
          </Link>
        }
      />

      {/* El encabezado mide LO FILTRADO. */}
      <div className="flex flex-wrap items-start gap-x-8 gap-y-3 rounded-xl border border-border bg-surface-raised px-4 py-3 shadow-card">
        <Cifra etiqueta="Arreglos" valor={r ? r.arreglos.toLocaleString("es-CR") : "—"} nota="en el filtro" />
        <Cifra etiqueta="Pactado" valor={r ? formatMoneda(r.pactado) : "—"} nota="lo que se comprometieron a pagar" />
        <Cifra
          etiqueta="Pagado"
          valor={r ? formatMoneda(r.pagado) : "—"}
          tono="positivo"
          nota={
            r && toNumber(r.pactado) > 0
              ? `${Math.round((toNumber(r.pagado) / toNumber(r.pactado)) * 100)} % de lo pactado`
              : undefined
          }
        />
        <Cifra
          etiqueta="Atraso del plan"
          valor={r ? formatMoneda(r.atraso_total) : "—"}
          tono={r && toNumber(r.atraso_total) > 0 ? "negativo" : "positivo"}
          nota="lo que debían haber pagado a hoy"
        />
        <Cifra
          etiqueta="Al día / en mora"
          valor={r ? `${r.al_dia} / ${r.en_mora}` : "—"}
          tono={r && r.en_mora > 0 ? "negativo" : undefined}
          nota="planes vivos"
        />
        <Cifra
          etiqueta="Quebrados"
          valor={r ? r.quebrados.toLocaleString("es-CR") : "—"}
          tono={r && r.quebrados > 0 ? "negativo" : undefined}
          nota="pasaron a cartera morosa"
        />
        {/* Con autorización SIN TOPE, este número ES el control. */}
        <Cifra
          etiqueta="Excepciones"
          valor={r ? r.excepciones.toLocaleString("es-CR") : "—"}
          tono={r && r.excepciones > 0 ? "pendiente" : undefined}
          nota="fuera de los plazos estándar"
        />
        {q.isFetching && <span className="ml-auto self-center text-xs text-accent">actualizando…</span>}
      </div>

      <p className="text-xs text-content-muted">
        Plazos estándar <b>{(plazos?.estandar ?? []).join("-")} cuotas</b>: los pacta cualquier gestor. Cualquier
        otro plazo es <b>excepción</b> y lo autoriza el supervisor de piso, que además es quien puede{" "}
        <b>quebrar</b> un arreglo incumplido. Como esa autorización no tiene tope, el contador de excepciones de
        arriba es el control: no hay un límite que avise, hay un acumulado que se mira.
      </p>

      <div className="flex flex-wrap items-end gap-3">
        <Input
          label="Contrato"
          value={contrato}
          onChange={(e) => setContrato(e.target.value)}
          placeholder="Número exacto…"
          className="min-w-48"
        />
        <Select
          label="Estado"
          value={estado}
          onChange={(e) => setEstado(e.target.value)}
          options={[
            { value: "", label: "Todos los estados" },
            { value: "VIVOS", label: "Vivos (al día y en mora)" },
            { value: "AL_DIA", label: "Al día" },
            { value: "EN_MORA", label: "En mora del plan" },
            { value: "CUMPLIDO", label: "Cumplidos" },
            { value: "QUEBRADO", label: "Quebrados" },
            { value: "ANULADO", label: "Anulados" },
          ]}
          className="min-w-56"
        />
        <label className="flex items-center gap-2 pb-2 text-xs text-content-muted">
          <input
            type="checkbox"
            checked={soloExcepciones}
            onChange={(e) => setSoloExcepciones(e.target.checked)}
            className="h-3.5 w-3.5 rounded border-border accent-accent"
          />
          Solo excepciones
        </label>
      </div>

      {q.isPending ? (
        <LoadingState label="Cargando los arreglos" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          message={
            estado || soloExcepciones || contrato
              ? "Ningún arreglo coincide con estos filtros."
              : "Todavía no se ha pactado ningún arreglo. Se pactan desde la ficha del contrato, sobre su deuda vencida."
          }
        />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Arreglo</TH>
                <TH>Contrato</TH>
                <TH>Plan</TH>
                <TH className="text-right">Pagado</TH>
                <TH className="text-right">Atraso</TH>
                <TH>Próxima cuota</TH>
                <TH>Estado</TH>
                <TH />
              </TR>
            </THead>
            <TBody>
              {items.map((a) => (
                <TR key={a.id}>
                  <TD className="font-medium tabular-nums">
                    {a.consecutivo}
                    <span className="block text-[11px] font-normal text-content-muted">{a.creado_en}</span>
                  </TD>
                  <TD>
                    <Link
                      to={`/cxc/contratos/${encodeURIComponent(a.contrato)}`}
                      className="font-medium hover:text-accent"
                    >
                      {a.contrato}
                    </Link>
                    <span className="block max-w-[12rem] truncate text-[11px] text-content-muted">
                      {a.cliente || "—"}
                      {a.sede && ` · ${a.sede}`}
                    </span>
                  </TD>
                  <TD className="text-xs">
                    {formatMoneda(a.monto_arreglo)} en {a.plazo_cuotas}{" "}
                    {a.plazo_cuotas === 1 ? "cuota" : "cuotas"}
                    {toNumber(a.prima) > 0 && (
                      <span className="block text-[11px] text-content-muted">prima {formatMoneda(a.prima)}</span>
                    )}
                    {a.es_excepcion && (
                      <Badge tone="pendiente" className="mt-1">
                        excepción
                      </Badge>
                    )}
                  </TD>
                  <TD className="text-right tabular-nums">
                    {formatMoneda(a.pagado)}
                    <span className="block text-[11px] text-content-muted">
                      {a.cuotas_cubiertas} de {a.cuotas.length} cuotas
                    </span>
                  </TD>
                  <TD
                    className={
                      toNumber(a.atraso) > 0
                        ? "text-right font-medium tabular-nums text-negativo"
                        : "text-right tabular-nums text-content-muted"
                    }
                  >
                    {formatMoneda(a.atraso)}
                  </TD>
                  <TD className="text-xs tabular-nums">
                    {a.proxima_cuota ? (
                      <>
                        {formatFecha(a.proxima_cuota)}
                        <span className="block text-[11px] text-content-muted">
                          {formatMoneda(a.proximo_monto)}
                        </span>
                      </>
                    ) : (
                      <span className="text-content-muted">—</span>
                    )}
                  </TD>
                  <TD>
                    <Badge tone={TONO[a.estado] ?? "neutral"}>{ETIQUETA[a.estado] ?? a.estado}</Badge>
                    {a.quebranto_motivo && (
                      <span className="mt-1 block max-w-[14rem] truncate text-[11px] text-content-muted">
                        {a.quebrado_por}: {a.quebranto_motivo}
                      </span>
                    )}
                  </TD>
                  <TD className="text-right">
                    {puedeCerrar && (a.estado === "AL_DIA" || a.estado === "EN_MORA") && (
                      <div className="flex justify-end gap-2">
                        <Button size="sm" variant="secondary" onClick={() => setCerrar({ a, quebrar: true })}>
                          Quebrar
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => setCerrar({ a, quebrar: false })}>
                          Anular
                        </Button>
                      </div>
                    )}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {cerrar && (
        <ConfirmDialog
          titulo={
            cerrar.quebrar
              ? `¿Quebrar el arreglo ${cerrar.a.consecutivo} de ${cerrar.a.contrato}?`
              : `¿Anular el arreglo ${cerrar.a.consecutivo} de ${cerrar.a.contrato}?`
          }
          descripcion={
            cerrar.quebrar
              ? "Quebrar declara el INCUMPLIMIENTO del cliente. Es distinto de anular."
              : "Anular es para el arreglo que no debió existir: se capturó mal, el cliente no firmó. No marca incumplimiento."
          }
          impacto={
            cerrar.quebrar
              ? [
                  "El contrato pasa a CARTERA MOROSA",
                  "La regla de los 18 meses sigue corriendo sobre los cargos originales",
                  `Llevaba pagado ${formatMoneda(cerrar.a.pagado)} de ${formatMoneda(cerrar.a.monto_arreglo)}`,
                ]
              : ["NO manda a cartera morosa", "Se puede pactar otro arreglo en su lugar"]
          }
          textoConfirmar={cerrar.quebrar ? "Quebrar" : "Anular"}
          tono={cerrar.quebrar ? "peligro" : "accent"}
          pendiente={quebrar.isPending || anular.isPending}
          pedirNota
          notaPlaceholder="Por qué se cierra este arreglo (obligatorio)"
          onConfirmar={cerrarArreglo}
          onCancelar={() => setCerrar(null)}
        />
      )}
    </div>
  );
}

function Cifra({
  etiqueta,
  valor,
  nota,
  tono,
}: {
  etiqueta: string;
  valor: string;
  nota?: string;
  tono?: "negativo" | "positivo" | "pendiente";
}) {
  return (
    <div>
      <p className="text-[11px] uppercase tracking-wide text-content-muted">{etiqueta}</p>
      <p
        className={
          tono === "negativo"
            ? "text-lg font-semibold tabular-nums text-negativo"
            : tono === "positivo"
              ? "text-lg font-semibold tabular-nums text-positivo"
              : tono === "pendiente"
                ? "text-lg font-semibold tabular-nums text-pendiente"
                : "text-lg font-semibold tabular-nums text-content"
        }
      >
        {valor}
      </p>
      {nota && <p className="text-[11px] text-content-muted">{nota}</p>}
    </div>
  );
}
