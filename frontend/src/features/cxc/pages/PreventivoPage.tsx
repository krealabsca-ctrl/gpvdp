/**
 * CxC — Contacto preventivo (/cxc/preventivo).
 *
 * Es el universo que la cola de cobro deja FUERA a propósito: contratos que están al día y cuya
 * cuota todavía no vence. La cola no los trae porque a nadie se le cobra lo que no debe —el
 * catálogo de tramos lo dice: ADELANTADO y AL_DÍA traen canal sugerido «Ninguno»— pero llamar
 * antes del vencimiento no es cobrar: es evitar que la cuota se venza.
 *
 * Va como pantalla y permiso aparte (cxc.preventivo) porque el negocio lo pidió así, y tiene
 * razón: es otra actividad, con otro guion y otro indicador de éxito. Aquí el objetivo no es
 * recuperar, es que la cartera no envejezca.
 *
 * Los dos universos son DISJUNTOS: ningún contrato sale en las dos listas, así que nadie recibe
 * el mismo día una llamada de recordatorio y otra de cobro.
 */

import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
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
import { formatFecha, formatMoneda } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useCatalogosCxc, usePreventivo } from "@/features/cxc/hooks";

const PAGE_SIZE = 50;

export function PreventivoPage() {
  const catalogos = useCatalogosCxc();
  const [qInput, setQInput] = useState("");
  const [q, setQ] = useState("");
  const [sedeId, setSedeId] = useState("");
  const [motivo, setMotivo] = useState("");
  const [page, setPage] = useState(1);

  useEffect(() => {
    const t = setTimeout(() => setQ(qInput.trim()), 300);
    return () => clearTimeout(t);
  }, [qInput]);
  useEffect(() => {
    setPage(1);
  }, [q, sedeId, motivo]);

  const listaQ = usePreventivo({
    ...(q ? { q } : {}),
    ...(sedeId ? { sede_id: sedeId } : {}),
    ...(motivo ? { motivo } : {}),
    page,
    page_size: PAGE_SIZE,
  });

  const cat = catalogos.data;
  const items = listaQ.data?.items ?? [];
  const total = listaQ.data?.total ?? 0;
  const r = listaQ.data?.resumen;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Contacto preventivo"
        description="Contratos al día cuya cuota está por vencer. Avisar antes cuesta una llamada; cobrar después cuesta el mes."
        actions={
          <Link to="/cxc/cola" className="text-sm font-medium text-accent underline">
            Ir a la cola de cobro
          </Link>
        }
      />

      {/* El encabezado mide LO FILTRADO, como en el resto del ERP. */}
      <div className="flex flex-wrap items-start gap-x-8 gap-y-3 rounded-xl border border-border bg-surface-raised px-4 py-3 shadow-card">
        <Cifra
          etiqueta="Por avisar"
          valor={r ? r.contratos.toLocaleString("es-CR") : "—"}
          nota={r ? `ventana de ${r.dias} días` : undefined}
        />
        <Cifra etiqueta="Monto por vencer" valor={r ? formatMoneda(r.monto) : "—"} nota="todavía no es deuda" />
        <Cifra
          etiqueta="Cuota por vencer"
          valor={r ? r.por_vencer.toLocaleString("es-CR") : "—"}
          nota="recordatorio de pago"
        />
        <Cifra
          etiqueta="Tarjeta por vencer"
          valor={r ? r.tarjetas.toLocaleString("es-CR") : "—"}
          tono={r && r.tarjetas > 0 ? "pendiente" : undefined}
          nota="renovar antes del rechazo"
        />
        <Cifra
          etiqueta="Sin canal"
          valor={r ? r.sin_contactar.toLocaleString("es-CR") : "—"}
          tono={r && r.sin_contactar > 0 ? "negativo" : undefined}
          nota="ni teléfono ni correo"
        />
        {listaQ.isFetching && <span className="ml-auto self-center text-xs text-accent">actualizando…</span>}
      </div>

      <p className="text-xs text-content-muted">
        La ventana ({r?.dias ?? "—"} días) se configura en{" "}
        <Link to="/cxc/parametros" className="text-accent underline">
          Parámetros
        </Link>{" "}
        con <b>DIAS_CONTACTO_PREVENTIVO</b>. Esta lista y la cola de cobro son conjuntos disjuntos: en cuanto
        una cuota se vence, el contrato sale de aquí y entra allá.
      </p>

      <div className="flex flex-wrap items-end gap-3">
        <Input
          label="Buscar"
          value={qInput}
          onChange={(e) => setQInput(e.target.value)}
          placeholder="Contrato, cédula o cliente…"
          className="min-w-56"
        />
        <Select
          label="Sede"
          value={sedeId}
          onChange={(e) => setSedeId(e.target.value)}
          options={[
            { value: "", label: "Todas las sedes" },
            ...(cat?.sedes ?? []).map((s) => ({ value: s.id, label: s.nombre })),
          ]}
          className="min-w-44"
        />
        <Select
          label="Motivo del aviso"
          value={motivo}
          onChange={(e) => setMotivo(e.target.value)}
          options={[
            { value: "", label: "Los dos motivos" },
            { value: "POR_VENCER", label: "Cuota por vencer" },
            { value: "TARJETA", label: "Tarjeta por vencer" },
          ]}
          className="min-w-48"
        />
      </div>

      {listaQ.isPending ? (
        <LoadingState label="Armando la lista preventiva" />
      ) : listaQ.isError ? (
        <ErrorState message={mensajeError(listaQ.error)} onRetry={() => listaQ.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          message={
            total === 0 && !q && !motivo
              ? "Nadie por avisar en esta ventana. Todo lo que debe algo está en la cola de cobro."
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
                  <TH>Contacto</TH>
                  <TH>Forma de pago</TH>
                  <TH>Vence</TH>
                  <TH className="text-right">Cuota</TH>
                  <TH>Qué decirle</TH>
                  <TH>Última gestión</TH>
                </TR>
              </THead>
              <TBody>
                {items.map((f) => (
                  <TR key={f.contrato_id}>
                    <TD>
                      <Link
                        to={`/cxc/contratos/${encodeURIComponent(f.numero)}`}
                        className="font-medium hover:text-accent"
                      >
                        {f.numero}
                      </Link>
                      <span className="block max-w-[13rem] truncate text-[11px] text-content-muted">
                        {f.cliente || "—"}
                      </span>
                    </TD>
                    <TD className="text-xs">
                      {f.telefonos || f.correos ? (
                        <>
                          {f.telefonos && <span className="block">{f.telefonos}</span>}
                          {f.correos && (
                            <span className="block max-w-[14rem] truncate text-[11px] text-content-muted">
                              {f.correos}
                            </span>
                          )}
                        </>
                      ) : (
                        <Badge tone="negativo">sin canal para avisar</Badge>
                      )}
                      {f.sede && <span className="block truncate text-[11px] text-content-muted">{f.sede}</span>}
                    </TD>
                    <TD className="max-w-[12rem] text-xs">
                      <span className="block truncate">{f.forma_pago || "—"}</span>
                      {f.asociacion && (
                        <span className="block truncate text-[11px] text-content-muted">{f.asociacion}</span>
                      )}
                    </TD>
                    <TD className="tabular-nums">
                      {formatFecha(f.proxima_cuota)}
                      <span className="block text-[11px] text-content-muted">
                        {f.dias_para_vencer <= 0 ? "hoy" : `en ${f.dias_para_vencer} días`}
                      </span>
                    </TD>
                    <TD className="text-right font-medium tabular-nums">
                      {formatMoneda(f.proximo_monto)}
                      <span className="block text-[11px] font-normal text-content-muted">
                        saldo {formatMoneda(f.saldo)}
                      </span>
                    </TD>
                    <TD className="text-xs">
                      {/* A un domiciliado no se le pide que pague: se le pide que tenga fondos. */}
                      {f.motivo === "TARJETA" ? (
                        <>
                          <Badge tone="pendiente">renovar tarjeta</Badge>
                          <span className="block text-[11px] text-content-muted">
                            caduca {formatFecha(f.tarjeta_vence)}
                          </span>
                        </>
                      ) : f.domiciliado ? (
                        <>
                          <Badge tone="accent">avisar del débito</Badge>
                          <span className="block text-[11px] text-content-muted">que tenga fondos ese día</span>
                        </>
                      ) : (
                        <>
                          <Badge tone="accent">recordar el pago</Badge>
                          <span className="block text-[11px] text-content-muted">{f.modalidad}</span>
                        </>
                      )}
                    </TD>
                    <TD className="text-xs">
                      {f.ultima_gestion ? (
                        <>
                          {formatFecha(f.ultima_gestion)}
                          {f.gestiones > 1 && (
                            <span className="block text-[11px] text-content-muted">{f.gestiones} gestiones</span>
                          )}
                        </>
                      ) : (
                        <span className="text-content-muted">nunca contactado</span>
                      )}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>

          <div className="flex items-center justify-between text-sm text-content-muted">
            <span>
              {items.length} de {total.toLocaleString("es-CR")} contratos por avisar
            </span>
            {totalPages > 1 && (
              <div className="flex items-center gap-2">
                <Button size="sm" variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
                  Anterior
                </Button>
                <span className="tabular-nums">
                  {page} / {totalPages}
                </span>
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => p + 1)}
                >
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
