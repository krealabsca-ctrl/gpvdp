/**
 * Pantalla — Proyecciones (/proyecciones), Fase C (CU-10).
 * El backend calcula el cierre de mes por método (semánticas aprobadas 2026-07-16):
 *  RITMO (run-rate confirmado 2026-07-06, respaldo sin histórico) · HISTORICO
 *  (mismo mes año anterior) · PROMEDIO (días restantes de los meses del año) ·
 *  COINCIDENCIA (mes gemelo por forma de senda). Meta % sobre el mes anterior.
 * Senda de cierre (real + proyección + meta), desglose por línea de ingreso,
 * escenarios guardados y precisión proyectado-vs-real en meses cerrados.
 */

import { useEffect, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { etiquetaPeriodo, formatMoneda, periodoActual, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { useEscenarios, useGuardarEscenario, useProyeccion } from "@/features/bancos/hooks";
import { SendaChart } from "@/features/bancos/components/SendaChart";
import type { MetodoProyeccion, ProyeccionResultado } from "@/api/bancos";

const METODOS: { id: MetodoProyeccion; nombre: string; desc: string }[] = [
  {
    id: "RITMO",
    nombre: "Ritmo del mes",
    desc: "Media diaria de los días con actividad de este mes × días activos restantes (domingos fuera).",
  },
  {
    id: "HISTORICO",
    nombre: "Histórico",
    desc: "El mismo mes del año pasado: tu acumulado se escala con el avance que ese mes llevaba a este día.",
  },
  {
    id: "PROMEDIO",
    nombre: "Promedio del año",
    desc: "A tu acumulado se le suma lo que en promedio entró en los días restantes en los meses de este año.",
  },
  {
    id: "COINCIDENCIA",
    nombre: "Coincidencia",
    desc: "Busca el mes cuya senda más se parece a la tuya y proyecta el cierre con la forma de ese mes.",
  },
];

const NOMBRE_METODO: Record<string, string> = Object.fromEntries(METODOS.map((m) => [m.id, m.nombre]));

export function ProyeccionesPage() {
  const toast = useToast();
  const { periodo } = usePeriodoActivo();
  const [metodo, setMetodo] = useState<MetodoProyeccion>("RITMO");
  const [metaInput, setMetaInput] = useState("6");
  const [metaPct, setMetaPct] = useState("6");

  useEffect(() => {
    const t = setTimeout(() => setMetaPct(metaInput.trim() || "0"), 400);
    return () => clearTimeout(t);
  }, [metaInput]);

  const proyQ = useProyeccion(periodo, metodo, metaPct);
  const escenariosQ = useEscenarios();
  const guardar = useGuardarEscenario();

  const p = proyQ.data;
  const disponibles = new Set(p?.metodos_disponibles ?? ["RITMO"]);

  function guardarEscenario() {
    guardar.mutate(
      { periodo, metodo, metaPct },
      {
        onSuccess: () => toast.success("Escenario guardado; se comparará contra el cierre real."),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Proyecciones"
        description={`Cierre estimado de ${etiquetaPeriodo(periodo)} por líneas de ingreso, con meta de crecimiento.`}
      />

      {/* Configuración del escenario */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[2fr_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>Método de proyección</CardTitle>
            <p className="mt-0.5 text-xs text-content-muted">
              Los métodos con histórico se habilitan solos conforme se acumulen meses.
            </p>
          </CardHeader>
          <CardContent className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            {METODOS.map((m) => {
              const habilitado = disponibles.has(m.id);
              const activo = metodo === m.id;
              return (
                <button
                  key={m.id}
                  type="button"
                  role="radio"
                  aria-checked={activo}
                  disabled={!habilitado}
                  onClick={() => setMetodo(m.id)}
                  className={cn(
                    "rounded-xl border-[1.5px] px-3.5 py-2.5 text-left transition-colors",
                    activo ? "border-accent bg-accent/5" : "border-border",
                    habilitado ? "hover:border-accent" : "cursor-not-allowed opacity-50",
                  )}
                >
                  <span className="block text-[12.5px] font-bold">{m.nombre}</span>
                  <span className="mt-0.5 block text-[11px] leading-snug text-content-muted">{m.desc}</span>
                  {!habilitado && (
                    <span className="mt-1.5 inline-block rounded-full border border-dashed border-brand-gold px-2 py-px text-[9.5px] font-extrabold tracking-wider text-brand-gold">
                      NECESITA HISTÓRICO
                    </span>
                  )}
                </button>
              );
            })}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Meta de crecimiento</CardTitle>
            <p className="mt-0.5 text-xs text-content-muted">Sobre el cierre del mes anterior</p>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-end gap-3">
              <Input
                label="Meta %"
                type="number"
                step="0.5"
                value={metaInput}
                onChange={(e) => setMetaInput(e.target.value)}
                className="w-28"
              />
              <p className="pb-2 text-sm font-semibold tabular-nums">
                {p && toNumber(p.meta_monto) > 0 ? `→ ${formatMoneda(p.meta_monto)}` : ""}
              </p>
            </div>
            {p && toNumber(p.meta_monto) === 0 && !p.sin_datos && (
              <p className="text-xs text-content-muted">
                Sin cierre del mes anterior todavía — la meta se activa cuando haya un mes previo con ingresos.
              </p>
            )}
            <Button onClick={guardarEscenario} loading={guardar.isPending} disabled={!p || p.sin_datos}>
              Guardar escenario
            </Button>
          </CardContent>
        </Card>
      </div>

      {proyQ.isPending ? (
        <LoadingState label="Calculando proyección" />
      ) : proyQ.isError ? (
        <ErrorState message={mensajeError(proyQ.error)} onRetry={() => proyQ.refetch()} />
      ) : p && p.sin_datos ? (
        <EmptyState message={`No hay ingresos en ${etiquetaPeriodo(periodo)} — importá o esperá movimientos para proyectar.`} />
      ) : p ? (
        <ResultadoView p={p} />
      ) : null}

      {/* Escenarios guardados + precisión */}
      <Card>
        <CardHeader>
          <CardTitle>Escenarios guardados</CardTitle>
          <p className="mt-0.5 text-xs text-content-muted">
            Cada escenario congela método, meta y proyección al día del cálculo; cuando el mes cierra, se mide
            la precisión contra el real.
          </p>
        </CardHeader>
        <CardContent>
          {escenariosQ.isPending ? (
            <LoadingState label="Cargando escenarios" />
          ) : escenariosQ.isError ? (
            <ErrorState message={mensajeError(escenariosQ.error)} onRetry={() => escenariosQ.refetch()} />
          ) : (escenariosQ.data ?? []).length === 0 ? (
            <EmptyState message="Aún no hay escenarios guardados. Configurá método y meta, y guardá el primero." />
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Guardado</TH>
                    <TH>Período</TH>
                    <TH>Método</TH>
                    <TH className="text-right">Meta</TH>
                    <TH className="text-right">Día</TH>
                    <TH className="text-right">Cierre proyectado</TH>
                    <TH className="text-right">Real del período</TH>
                    <TH className="text-right">Precisión</TH>
                  </TR>
                </THead>
                <TBody>
                  {(escenariosQ.data ?? []).map((e) => {
                    const cerrado = e.periodo < periodoActual();
                    const real = toNumber(e.real_cierre);
                    const proy = toNumber(e.cierre_proyectado);
                    const err = cerrado && real > 0 ? ((proy - real) / real) * 100 : null;
                    return (
                      <TR key={e.id}>
                        <TD className="whitespace-nowrap tabular-nums">{e.creado_en}</TD>
                        <TD>{etiquetaPeriodo(e.periodo)}</TD>
                        <TD>
                          {NOMBRE_METODO[e.metodo_efectivo] ?? e.metodo_efectivo}
                          {e.metodo !== e.metodo_efectivo && (
                            <span className="block text-[10px] text-content-muted">
                              pedido: {NOMBRE_METODO[e.metodo] ?? e.metodo}
                            </span>
                          )}
                        </TD>
                        <TD className="text-right tabular-nums">+{e.meta_pct}%</TD>
                        <TD className="text-right tabular-nums">{e.dia_calculo}</TD>
                        <TD className="text-right font-medium tabular-nums">{formatMoneda(e.cierre_proyectado)}</TD>
                        <TD className="text-right tabular-nums">{cerrado ? formatMoneda(e.real_cierre) : "en curso"}</TD>
                        <TD className="text-right">
                          {err === null ? (
                            <span className="text-content-muted">—</span>
                          ) : (
                            <Badge tone={Math.abs(err) <= 5 ? "positivo" : "pendiente"}>
                              {err >= 0 ? "+" : ""}
                              {err.toFixed(1)}%
                            </Badge>
                          )}
                        </TD>
                      </TR>
                    );
                  })}
                </TBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function ResultadoView({ p }: { p: ProyeccionResultado }) {
  const meta = toNumber(p.meta_monto);
  const brecha = toNumber(p.brecha);
  const restantes = p.dias_mes - p.dia_calculo;

  return (
    <>
      {/* KPIs */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Kpi
          label="Cierre proyectado"
          valor={formatMoneda(p.cierre_proyectado)}
          detalle={`método: ${NOMBRE_METODO[p.metodo_efectivo] ?? p.metodo_efectivo}${
            p.metodo !== p.metodo_efectivo ? " (respaldo)" : ""
          }${p.mes_gemelo ? ` · gemelo: ${etiquetaPeriodo(p.mes_gemelo)}` : ""}`}
          tono="accent"
        />
        <Kpi
          label={`Meta (+${p.meta_pct}%)`}
          valor={meta > 0 ? formatMoneda(p.meta_monto) : "—"}
          detalle={meta > 0 ? "sobre el mes anterior" : "sin mes anterior aún"}
          tono="gold"
        />
        <Kpi
          label="Brecha vs meta"
          valor={meta > 0 ? `${brecha >= 0 ? "+" : "−"}${formatMoneda(String(Math.abs(brecha)))}` : "—"}
          detalle={meta > 0 ? (brecha >= 0 ? "por encima de la meta" : "falta para la meta") : ""}
          tono={meta === 0 ? "neutral" : brecha >= 0 ? "positivo" : "negativo"}
        />
        <Kpi
          label={`Real al día ${p.dia_calculo}`}
          valor={formatMoneda(p.real_acumulado)}
          detalle={`${restantes} día(s) del mes por delante`}
          tono="neutral"
        />
      </div>

      {/* Senda de cierre */}
      <Card>
        <CardHeader>
          <CardTitle>Senda de cierre — ingresos acumulados</CardTitle>
          <p className="mt-0.5 text-xs text-content-muted">
            Sólida: real (día 1–{p.dia_calculo}) · punteada: proyección al cierre
            {meta > 0 ? " · dorada: senda hacia la meta" : ""}
          </p>
        </CardHeader>
        <CardContent>
          <SendaChart resultado={p} />
        </CardContent>
      </Card>

      {/* Por línea de ingreso */}
      <Card>
        <CardHeader>
          <CardTitle>Cierre proyectado por línea de ingreso</CardTitle>
          <p className="mt-0.5 text-xs text-content-muted">
            La participación de cada línea en el mes se aplica al cierre y a la meta. Los movimientos sin
            clasificar aparecen como su propia línea — clasificalos en la Bandeja para afinar el desglose.
          </p>
        </CardHeader>
        <CardContent>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Línea de ingreso</TH>
                  <TH className="text-right">Real (día {p.dia_calculo})</TH>
                  <TH className="text-right">Cierre proyectado</TH>
                  <TH className="text-right">Meta</TH>
                  <TH className="text-right">Brecha</TH>
                  <TH>Estado</TH>
                </TR>
              </THead>
              <TBody>
                {p.por_linea.map((l) => {
                  const metaL = toNumber(l.meta);
                  const brechaL = toNumber(l.brecha);
                  const chip =
                    metaL === 0
                      ? null
                      : brechaL >= 0
                        ? { tone: "positivo" as const, label: "En meta" }
                        : brechaL > -metaL * 0.05
                          ? { tone: "pendiente" as const, label: "Cerca" }
                          : { tone: "negativo" as const, label: "En riesgo" };
                  return (
                    <TR key={l.clasificacion_id || l.nombre}>
                      <TD className="font-medium">{l.nombre}</TD>
                      <TD className="text-right tabular-nums">{formatMoneda(l.real)}</TD>
                      <TD className="text-right font-medium tabular-nums">{formatMoneda(l.cierre)}</TD>
                      <TD className="text-right tabular-nums">{metaL > 0 ? formatMoneda(l.meta) : "—"}</TD>
                      <TD
                        className={cn(
                          "text-right tabular-nums",
                          metaL > 0 && (brechaL >= 0 ? "text-positivo" : "text-negativo"),
                        )}
                      >
                        {metaL > 0 ? `${brechaL >= 0 ? "+" : "−"}${formatMoneda(String(Math.abs(brechaL)))}` : "—"}
                      </TD>
                      <TD>{chip ? <Badge tone={chip.tone}>{chip.label}</Badge> : <span className="text-content-muted">—</span>}</TD>
                    </TR>
                  );
                })}
              </TBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>
    </>
  );
}

function Kpi({
  label,
  valor,
  detalle,
  tono,
}: {
  label: string;
  valor: string;
  detalle: string;
  tono: "accent" | "gold" | "positivo" | "negativo" | "neutral";
}) {
  const color =
    tono === "accent"
      ? "text-accent"
      : tono === "gold"
        ? "text-brand-gold"
        : tono === "positivo"
          ? "text-positivo"
          : tono === "negativo"
            ? "text-negativo"
            : "text-content";
  return (
    <Card>
      <CardContent className="py-4">
        <p className="text-xs uppercase tracking-wide text-content-muted">{label}</p>
        <p className={cn("mt-1 text-xl font-semibold tabular-nums", color)}>{valor}</p>
        <p className="mt-1 text-xs text-content-muted">{detalle}</p>
      </CardContent>
    </Card>
  );
}
