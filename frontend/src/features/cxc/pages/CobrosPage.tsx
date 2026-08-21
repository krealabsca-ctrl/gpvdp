/**
 * CxC — Cobros (/cxc/cobros). El dinero que entró y a qué se aplicó.
 *
 * Dos pestañas que son dos trabajos distintos:
 *   · TODOS: la lista de cobros con el resumen de lo filtrado. Cada fila dice a qué
 *     períodos fue («2026-05 + 2026-06»), que es lo que el sistema viejo escondía en un
 *     campo de texto.
 *   · SIN IDENTIFICAR: la bandeja del dinero que llegó y no se sabe de quién. No se
 *     descarta ni se adivina: se resuelve acá.
 *
 * El rango de fechas filtra por la fecha BANCARIA, la que concilia contra Bancos.
 */

import { useEffect, useState } from "react";
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
import { cn } from "@/lib/cn";
import { formatFecha, formatMoneda, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import { useCatalogosCxc, useCobros, useIdentificarCobro, useReversarCobro } from "@/features/cxc/hooks";
import type { CobroFila } from "@/api/cxc";

const PAGE_SIZE = 50;

const TONO_ESTADO: Record<CobroFila["estado"], BadgeTone> = {
  APLICADO: "positivo",
  SIN_IDENTIFICAR: "pendiente",
  REVERSADO: "negativo",
};
const ETIQUETA_ESTADO: Record<CobroFila["estado"], string> = {
  APLICADO: "Aplicado",
  SIN_IDENTIFICAR: "Sin identificar",
  REVERSADO: "Reversado",
};

export function CobrosPage() {
  const toast = useToast();
  const tiene = useTienePermiso();
  const puedeAplicar = tiene("cxc.aplicar");
  const catalogos = useCatalogosCxc();
  const reversar = useReversarCobro();
  const identificar = useIdentificarCobro();

  const [tab, setTab] = useState<"todos" | "sin_identificar">("todos");
  const [qInput, setQInput] = useState("");
  const [q, setQ] = useState("");
  const [asociacionId, setAsociacionId] = useState("");
  const [estado, setEstado] = useState("");
  const [desde, setDesde] = useState("");
  const [hasta, setHasta] = useState("");
  const [page, setPage] = useState(1);

  // La reversa pide confirmación (deshace plata ya aplicada). La identificación va EN LA
  // FILA: el operador resuelve muchos depósitos seguidos y un diálogo por cada uno lo
  // frenaría sin agregar seguridad.
  const [aReversar, setAReversar] = useState<CobroFila | null>(null);
  const [destinos, setDestinos] = useState<Record<string, string>>({});

  useEffect(() => {
    const t = setTimeout(() => setQ(qInput.trim()), 300);
    return () => clearTimeout(t);
  }, [qInput]);
  useEffect(() => {
    setPage(1);
  }, [q, asociacionId, estado, desde, hasta, tab]);

  const filtros = {
    ...(q ? { q } : {}),
    ...(asociacionId ? { asociacion_id: asociacionId } : {}),
    ...(tab === "todos" && estado ? { estado } : {}),
    ...(desde ? { desde } : {}),
    ...(hasta ? { hasta } : {}),
    ...(tab === "sin_identificar" ? { sin_identificar: true } : {}),
  };
  const cobrosQ = useCobros({ ...filtros, page, page_size: PAGE_SIZE });

  const items = cobrosQ.data?.items ?? [];
  const total = cobrosQ.data?.total ?? 0;
  const r = cobrosQ.data?.resumen;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  function confirmarReversa(nota: string) {
    if (!aReversar) return;
    reversar.mutate(
      { id: aReversar.id, motivo: nota.trim() || "sin motivo indicado" },
      {
        onSuccess: () => {
          toast.success("Cobro reversado: sus cargos volvieron a abrirse con su antigüedad original.");
          setAReversar(null);
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  function aplicarA(cobro: CobroFila) {
    const contrato = (destinos[cobro.id] ?? "").trim();
    if (!contrato) return;
    identificar.mutate(
      { id: cobro.id, contrato },
      {
        onSuccess: (res) => {
          // Se dice a QUÉ períodos fue: es la única forma de saber que se aplicó bien.
          const periodos = res.aplicaciones.map((a) => a.periodo).join(" + ");
          const favor = toNumber(res.saldo_a_favor) > 0 ? ` · a favor ${formatMoneda(res.saldo_a_favor)}` : "";
          toast.success(
            periodos
              ? `Aplicado a ${res.contrato}: ${periodos}${favor}.`
              : `${res.contrato} no tenía cargos abiertos: todo quedó a favor del cliente.`,
          );
          setDestinos((d) => {
            const n = { ...d };
            delete n[cobro.id];
            return n;
          });
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Cobros"
        description="El dinero que entró y a qué cargos se aplicó. El rango de fechas usa la fecha bancaria: la que concilia contra Bancos."
        actions={
          <Link to="/cxc/importar" className="text-sm font-medium text-accent underline">
            Importar pagos
          </Link>
        }
      />

      {/* Resumen de LO FILTRADO */}
      <div className="flex flex-wrap items-start gap-x-8 gap-y-3 rounded-xl border border-border bg-surface-raised px-4 py-3 shadow-card">
        <Cifra etiqueta="Cobros" valor={r ? r.cobros.toLocaleString("es-CR") : "—"} nota="en el filtro" />
        <Cifra etiqueta="Monto" valor={r ? formatMoneda(r.monto) : "—"} nota="lo que entró" />
        <Cifra etiqueta="Aplicado a cargos" valor={r ? formatMoneda(r.aplicado) : "—"} tono="positivo" nota="bajó deuda" />
        <Cifra
          etiqueta="Saldo a favor"
          valor={r ? formatMoneda(r.saldo_a_favor) : "—"}
          tono={r && toNumber(r.saldo_a_favor) > 0 ? "pendiente" : undefined}
          nota="sin aplicar todavía"
        />
        <Cifra
          etiqueta="Sin identificar"
          valor={r ? r.sin_identificar.toLocaleString("es-CR") : "—"}
          tono={r && r.sin_identificar > 0 ? "pendiente" : undefined}
          nota="dinero sin dueño"
        />
        <Cifra etiqueta="Reversados" valor={r ? r.reversados.toLocaleString("es-CR") : "—"} nota="no entraron" />
        {cobrosQ.isFetching && <span className="ml-auto self-center text-xs text-accent">actualizando…</span>}
      </div>

      {/* Pestañas */}
      <div className="flex flex-wrap items-center gap-2 border-b border-border">
        {(
          [
            ["todos", "Todos"],
            ["sin_identificar", "Sin identificar"],
          ] as const
        ).map(([id, label]) => (
          <button
            key={id}
            role="tab"
            aria-selected={tab === id}
            onClick={() => setTab(id)}
            className={cn(
              "-mb-px flex items-center gap-1.5 border-b-2 px-4 py-2 text-sm font-medium transition-colors",
              tab === id ? "border-accent text-accent" : "border-transparent text-content-muted hover:text-content",
            )}
          >
            {label}
            {id === "sin_identificar" && r && r.sin_identificar > 0 && (
              <span className="rounded-full bg-pendiente/15 px-1.5 text-[11px] font-semibold tabular-nums text-pendiente">
                {r.sin_identificar}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* Filtros */}
      <div className="flex flex-wrap items-end gap-3">
        <Input
          label="Buscar"
          value={qInput}
          onChange={(e) => setQInput(e.target.value)}
          placeholder="Recibo, referencia, contrato o cliente…"
          className="min-w-60"
        />
        <Select
          label="Asociación"
          value={asociacionId}
          onChange={(e) => setAsociacionId(e.target.value)}
          options={[
            { value: "", label: "Todas" },
            ...(catalogos.data?.asociaciones ?? []).map((a) => ({ value: a.id, label: a.nombre })),
          ]}
          className="min-w-44"
        />
        {tab === "todos" && (
          <Select
            label="Estado"
            value={estado}
            onChange={(e) => setEstado(e.target.value)}
            options={[
              { value: "", label: "Todos" },
              { value: "APLICADO", label: "Aplicado" },
              { value: "SIN_IDENTIFICAR", label: "Sin identificar" },
              { value: "REVERSADO", label: "Reversado" },
            ]}
            className="min-w-40"
          />
        )}
        <Input label="Desde (bancaria)" type="date" value={desde} onChange={(e) => setDesde(e.target.value)} className="w-40" />
        <Input label="Hasta (bancaria)" type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} className="w-40" />
      </div>

      {cobrosQ.isPending ? (
        <LoadingState label="Cargando los cobros" />
      ) : cobrosQ.isError ? (
        <ErrorState message={mensajeError(cobrosQ.error)} onRetry={() => cobrosQ.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          message={
            tab === "sin_identificar"
              ? "No hay dinero sin identificar. 👌"
              : total === 0
                ? "Todavía no hay cobros. Empezá por «Importar pagos»."
                : "Ningún cobro coincide con estos filtros."
          }
        />
      ) : (
        <>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Fecha bancaria</TH>
                  <TH>Recibo</TH>
                  <TH>Contrato · cliente</TH>
                  <TH className="text-right">Monto</TH>
                  <TH className="text-right">Aplicado</TH>
                  <TH className="text-right">A favor</TH>
                  <TH>Períodos que pagó</TH>
                  <TH>Canal</TH>
                  <TH>Estado</TH>
                  <TH className="text-right">Acción</TH>
                </TR>
              </THead>
              <TBody>
                {items.map((k) => (
                  <TR key={k.id}>
                    <TD className="whitespace-nowrap tabular-nums">
                      {k.fecha_bancaria ? formatFecha(k.fecha_bancaria) : "—"}
                      {/* La fecha del período es otra cosa y se muestra debajo: en los datos
                          reales el origen pone el día 1 del mes que se está pagando. */}
                      <span className="block text-[11px] text-content-muted">
                        período {formatFecha(k.fecha_pago)}
                      </span>
                    </TD>
                    <TD className="tabular-nums">{k.consecutivo || "—"}</TD>
                    <TD className="max-w-[16rem]">
                      {k.contrato ? (
                        <Link to={`/cxc/contratos/${encodeURIComponent(k.contrato)}`} className="font-medium text-accent underline">
                          {k.contrato}
                        </Link>
                      ) : k.contrato_origen ? (
                        // El archivo traía un número que NO está en la cartera: se muestra
                        // porque es la pista para identificarlo (o para darse cuenta de que
                        // falta cargar ese contrato).
                        <span className="text-pendiente">
                          {k.contrato_origen}
                          <span className="block text-[10.5px]">no está en la cartera</span>
                        </span>
                      ) : (
                        <span className="text-content-muted">sin contrato</span>
                      )}
                      <span className="block truncate text-[11px] text-content-muted">
                        {k.cliente || k.referencia || "—"}
                      </span>
                    </TD>
                    <TD className="text-right font-medium tabular-nums">{formatMoneda(k.monto)}</TD>
                    <TD className="text-right tabular-nums text-positivo">
                      {toNumber(k.aplicado) > 0 ? formatMoneda(k.aplicado) : "—"}
                    </TD>
                    <TD className={cn("text-right tabular-nums", toNumber(k.saldo_a_favor) > 0 && "text-pendiente")}>
                      {toNumber(k.saldo_a_favor) > 0 ? formatMoneda(k.saldo_a_favor) : "—"}
                    </TD>
                    <TD className="max-w-[12rem] text-xs">
                      {k.periodos || <span className="text-content-muted">—</span>}
                    </TD>
                    <TD className="max-w-[11rem] text-xs">
                      {k.forma_pago || "—"}
                      {k.asociacion && <span className="block text-[11px] text-content-muted">{k.asociacion}</span>}
                    </TD>
                    <TD>
                      <Badge tone={TONO_ESTADO[k.estado]}>{ETIQUETA_ESTADO[k.estado]}</Badge>
                    </TD>
                    {/* Identificar y reversar exigen cxc.aplicar. Sin el permiso no se
                        muestran: la regla del ERP es «si no tengo permiso, no se ve», no
                        mostrar un botón que devuelve 403 al tocarlo. */}
                    <TD className="text-right">
                      {!puedeAplicar ? (
                        k.estado === "SIN_IDENTIFICAR" && (
                          <span className="text-[11px] text-content-muted">sin permiso para aplicar</span>
                        )
                      ) : (
                        <>
                          {k.estado === "SIN_IDENTIFICAR" && (
                            <div className="flex items-center justify-end gap-2">
                              <input
                                value={destinos[k.id] ?? ""}
                                onChange={(e) => setDestinos((d) => ({ ...d, [k.id]: e.target.value }))}
                                onKeyDown={(e) => {
                                  if (e.key === "Enter") aplicarA(k);
                                }}
                                placeholder="N.º de contrato"
                                className="w-36 rounded-lg border border-border bg-surface px-2 py-1 text-xs focus:border-accent focus:outline-none"
                              />
                              <Button
                                size="sm"
                                disabled={!(destinos[k.id] ?? "").trim()}
                                loading={identificar.isPending}
                                onClick={() => aplicarA(k)}
                              >
                                Aplicar
                              </Button>
                            </div>
                          )}
                          {k.estado === "APLICADO" && (
                            <Button size="sm" variant="secondary" onClick={() => setAReversar(k)}>
                              Reversar
                            </Button>
                          )}
                        </>
                      )}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>

          <div className="flex items-center justify-between text-sm text-content-muted">
            <span>
              {items.length} de {total.toLocaleString("es-CR")} cobros
            </span>
            {totalPages > 1 && (
              <div className="flex items-center gap-2">
                <Button size="sm" variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
                  Anterior
                </Button>
                <span className="tabular-nums">
                  {page} / {totalPages}
                </span>
                <Button size="sm" variant="secondary" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
                  Siguiente
                </Button>
              </div>
            )}
          </div>
        </>
      )}

      {aReversar && (
        <ConfirmDialog
          titulo={`Reversar el cobro de ${formatMoneda(aReversar.monto)}`}
          descripcion={`Recibo ${aReversar.consecutivo || "sin número"} del contrato ${aReversar.contrato || "—"}.`}
          impacto={[
            "Los cargos que pagó vuelven a abrirse CON SU ANTIGÜEDAD ORIGINAL: la mora no se limpia.",
            aReversar.periodos ? `Se desaplica de: ${aReversar.periodos}` : "No tenía aplicaciones.",
            "El cobro no se borra: queda marcado como reversado con su motivo.",
          ]}
          textoConfirmar="Reversar"
          tono="peligro"
          pendiente={reversar.isPending}
          pedirNota
          notaPlaceholder="Motivo: cheque devuelto, débito rechazado, contracargo…"
          onConfirmar={confirmarReversa}
          onCancelar={() => setAReversar(null)}
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
  tono?: "positivo" | "pendiente";
}) {
  return (
    <div className="min-w-[8.5rem]">
      <p className="text-[10.5px] font-semibold uppercase tracking-wide text-content-muted">{etiqueta}</p>
      <p
        className={cn(
          "text-lg font-semibold tabular-nums",
          tono === "positivo" ? "text-positivo" : tono === "pendiente" ? "text-pendiente" : "text-content",
        )}
      >
        {valor}
      </p>
      {nota && <p className="text-[10.5px] text-content-muted">{nota}</p>}
    </div>
  );
}
