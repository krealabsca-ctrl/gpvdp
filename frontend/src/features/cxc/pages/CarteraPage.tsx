/**
 * CxC — Cartera (/cxc/cartera). La pantalla de trabajo del módulo.
 *
 * Dos cosas heredadas de lo que ya funciona en el ERP:
 *   · El encabezado mide LO FILTRADO (contratos · saldo · vencido · por vencer), no el
 *     universo: es el patrón que aprobamos en la hoja de trabajo de Bancos.
 *   · Los filtros se resuelven en el SERVIDOR. Con 70 000 contratos, filtrar en el
 *     navegador no es una opción.
 *
 * El saldo, los días de mora y el tramo no son columnas guardadas: los deriva el
 * servidor de los cargos abiertos de cada contrato.
 */

import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Badge,
  Button,
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
} from "@/components/ui";
import type { BadgeTone } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatFecha, formatMoneda, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useCartera, useCatalogosCxc } from "@/features/cxc/hooks";
import type { ContratoCxc } from "@/api/cxc";

const PAGE_SIZE = 50;

/** Tono del tramo según cuánto pesa la mora. El código del tramo lo manda el servidor. */
function tonoTramo(dias: number, cargos: number): BadgeTone {
  if (cargos === 0) return "neutral";
  if (dias <= 0) return "positivo";
  if (dias <= 30) return "pendiente";
  return "negativo";
}

function etiquetaMora(c: ContratoCxc): string {
  if (c.cargos_abiertos === 0) return "sin saldo";
  if (c.dias_mora_max <= 0) return "al día";
  return `${c.dias_mora_max} días`;
}

export function CarteraPage() {
  const navigate = useNavigate();
  const catalogos = useCatalogosCxc();

  const [qInput, setQInput] = useState("");
  const [q, setQ] = useState("");
  const [sedeId, setSedeId] = useState("");
  const [modalidadId, setModalidadId] = useState("");
  const [formaPagoId, setFormaPagoId] = useState("");
  const [asociacionId, setAsociacionId] = useState("");
  const [conSaldo, setConSaldo] = useState(false);
  const [enRevision, setEnRevision] = useState(false);
  const [orden, setOrden] = useState("");
  const [page, setPage] = useState(1);

  useEffect(() => {
    const t = setTimeout(() => setQ(qInput.trim()), 300);
    return () => clearTimeout(t);
  }, [qInput]);
  useEffect(() => {
    setPage(1);
  }, [q, sedeId, modalidadId, formaPagoId, asociacionId, conSaldo, enRevision, orden]);

  // UN solo objeto de filtros: el mismo que mide el encabezado y el que trae las filas.
  const filtros = {
    ...(q ? { q } : {}),
    ...(sedeId ? { sede_id: sedeId } : {}),
    ...(modalidadId ? { modalidad_id: modalidadId } : {}),
    ...(formaPagoId ? { forma_pago_id: formaPagoId } : {}),
    ...(asociacionId ? { asociacion_id: asociacionId } : {}),
    ...(conSaldo ? { con_saldo: true } : {}),
    ...(enRevision ? { en_revision: true } : {}),
    ...(orden ? { orden } : {}),
  };
  const carteraQ = useCartera({ ...filtros, page, page_size: PAGE_SIZE });

  const cat = catalogos.data;
  const items = carteraQ.data?.items ?? [];
  const total = carteraQ.data?.total ?? 0;
  const r = carteraQ.data?.resumen;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const opciones = (lista: { id: string; nombre: string; contratos: number }[] | undefined, todos: string) => [
    { value: "", label: todos },
    ...(lista ?? []).map((x) => ({
      value: x.id,
      label: x.contratos > 0 ? `${x.nombre} (${x.contratos.toLocaleString("es-CR")})` : x.nombre,
    })),
  ];

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Cartera por cobrar"
        description="Cada período de cada contrato es un cargo con su vencimiento. El saldo y la mora se derivan de los cargos abiertos."
        actions={
          <Button variant="secondary" onClick={() => navigate("/cxc/importar")}>
            Importar cartera
          </Button>
        }
      />

      {/* Resumen de LO FILTRADO: si cambiás un filtro, estas cifras se mueven con la lista. */}
      <div className="flex flex-wrap items-start gap-x-8 gap-y-3 rounded-xl border border-border bg-surface-raised px-4 py-3 shadow-card">
        <Cifra etiqueta="Contratos" valor={r ? r.contratos.toLocaleString("es-CR") : "—"} nota="en el filtro" />
        <Cifra
          etiqueta="Con saldo"
          valor={r ? r.con_saldo.toLocaleString("es-CR") : "—"}
          nota={r && r.contratos > 0 ? `${Math.round((r.con_saldo / r.contratos) * 100)} % de la selección` : undefined}
        />
        <Cifra etiqueta="Saldo" valor={r ? formatMoneda(r.saldo) : "—"} tono="negativo" nota="todo lo que deben" />
        <Cifra etiqueta="Vencido" valor={r ? formatMoneda(r.vencido) : "—"} tono="negativo" nota="ya exigible" />
        <Cifra etiqueta="Por vencer" valor={r ? formatMoneda(r.por_vencer) : "—"} nota="todavía no vence" />
        <Cifra etiqueta="Cargos abiertos" valor={r ? r.cargos_abiertos.toLocaleString("es-CR") : "—"} nota="partidas sin saldar" />
        {carteraQ.isFetching && <span className="ml-auto self-center text-xs text-accent">actualizando…</span>}
      </div>

      {/* Filtros */}
      <div className="flex flex-wrap items-end gap-3">
        <Input
          label="Buscar"
          value={qInput}
          onChange={(e) => setQInput(e.target.value)}
          placeholder="Contrato, cédula o cliente…"
          className="min-w-56"
        />
        <Select label="Sede" value={sedeId} onChange={(e) => setSedeId(e.target.value)} options={opciones(cat?.sedes, "Todas las sedes")} className="min-w-48" />
        <Select label="Modalidad" value={modalidadId} onChange={(e) => setModalidadId(e.target.value)} options={opciones(cat?.modalidades, "Todas")} className="min-w-40" />
        <Select label="Forma de pago" value={formaPagoId} onChange={(e) => setFormaPagoId(e.target.value)} options={opciones(cat?.formas_pago, "Todas")} className="min-w-48" />
        <Select label="Asociación" value={asociacionId} onChange={(e) => setAsociacionId(e.target.value)} options={opciones(cat?.asociaciones, "Todas")} className="min-w-40" />
        <Select
          label="Ordenar por"
          value={orden}
          onChange={(e) => setOrden(e.target.value)}
          options={[
            { value: "", label: "Saldo: mayor a menor" },
            { value: "mora_desc", label: "Mora: más viejo primero" },
            { value: "numero", label: "Número de contrato" },
            { value: "cliente", label: "Cliente" },
          ]}
          className="min-w-48"
        />
        <label className="flex items-center gap-2 pb-2 text-xs text-content-muted">
          <input type="checkbox" checked={conSaldo} onChange={(e) => setConSaldo(e.target.checked)} className="h-3.5 w-3.5 rounded border-border accent-accent" />
          Solo con saldo
        </label>
        <label className="flex items-center gap-2 pb-2 text-xs text-content-muted">
          <input type="checkbox" checked={enRevision} onChange={(e) => setEnRevision(e.target.checked)} className="h-3.5 w-3.5 rounded border-border accent-accent" />
          Solo en revisión
        </label>
      </div>

      {carteraQ.isPending ? (
        <LoadingState label="Cargando la cartera" />
      ) : carteraQ.isError ? (
        <ErrorState message={mensajeError(carteraQ.error)} onRetry={() => carteraQ.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          message={
            total === 0 && !q && !conSaldo && !enRevision
              ? "Todavía no hay cartera cargada. Empezá por «Importar cartera»."
              : "Ningún contrato coincide con estos filtros."
          }
        />
      ) : (
        <>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Contrato</TH>
                  <TH>Cliente</TH>
                  <TH>Sede</TH>
                  <TH>Modalidad · forma de pago</TH>
                  <TH className="text-right">Cuota</TH>
                  <TH className="text-right">Cargos</TH>
                  <TH className="text-right">Saldo</TH>
                  <TH>Mora</TH>
                  <TH className="text-right">Origen</TH>
                </TR>
              </THead>
              <TBody>
                {items.map((c) => (
                  <TR
                    key={c.id}
                    className="cursor-pointer"
                    onClick={() => navigate(`/cxc/contratos/${encodeURIComponent(c.numero)}`)}
                  >
                    <TD>
                      <span className="font-medium">{c.numero}</span>
                      {c.revision_pendiente && (
                        <Badge tone="pendiente" className="ml-2">
                          revisar
                        </Badge>
                      )}
                      {c.fecha_primer_cobro && (
                        <span className="block text-[11px] text-content-muted">
                          primer cobro {formatFecha(c.fecha_primer_cobro)}
                        </span>
                      )}
                    </TD>
                    <TD className="max-w-[15rem]">
                      <span className="block truncate">{c.cliente_nombre || "—"}</span>
                      <span className="block text-[11px] text-content-muted">{c.documento}</span>
                    </TD>
                    <TD className="max-w-[11rem] truncate text-xs">{c.sede || "—"}</TD>
                    <TD className="text-xs">
                      {c.modalidad || "—"}
                      <span className="block text-[11px] text-content-muted">
                        {c.forma_pago || "—"}
                        {c.asociacion && ` · ${c.asociacion}`}
                      </span>
                    </TD>
                    <TD className="text-right tabular-nums">{formatMoneda(c.cuota_vigente)}</TD>
                    <TD className="text-right tabular-nums">{c.cargos_abiertos || "—"}</TD>
                    <TD className={cn("text-right font-medium tabular-nums", toNumber(c.saldo) > 0 && "text-negativo")}>
                      {toNumber(c.saldo) > 0 ? formatMoneda(c.saldo) : "—"}
                    </TD>
                    <TD>
                      <Badge tone={tonoTramo(c.dias_mora_max, c.cargos_abiertos)}>{etiquetaMora(c)}</Badge>
                    </TD>
                    {/* El dato del sistema viejo, para poder comparar durante la corrida en
                        paralelo. No manda: el tramo lo calcula el ERP con la mora real. */}
                    <TD className="text-right text-[11px] text-content-muted">
                      {c.dias_vencidos_origen !== null ? `${c.dias_vencidos_origen} d` : "—"}
                      {c.score_origen !== null && <span className="block">score {c.score_origen}</span>}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>

          <div className="flex items-center justify-between text-sm text-content-muted">
            <span>
              {items.length} de {total.toLocaleString("es-CR")} contratos
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
  tono?: "negativo" | "positivo";
}) {
  return (
    <div className="min-w-[8.5rem]">
      <p className="text-[10.5px] font-semibold uppercase tracking-wide text-content-muted">{etiqueta}</p>
      <p
        className={cn(
          "text-lg font-semibold tabular-nums",
          tono === "negativo" ? "text-negativo" : tono === "positivo" ? "text-positivo" : "text-content",
        )}
      >
        {valor}
      </p>
      {nota && <p className="text-[10.5px] text-content-muted">{nota}</p>}
    </div>
  );
}
