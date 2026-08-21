/**
 * Pantalla — Análisis y tendencias (/analisis).
 *
 * Contesta la pregunta que el usuario planteó el 2026-08-17: «¿esta partida se está descontrolando,
 * o es normal?» — no del mes, sino del semestre o del año. Existían las dos mitades por separado: el
 * Cuadre da el desglose por partida de UN mes y el Dashboard la serie de 12 meses de TOTALES. Acá se
 * cruzan: cada partida, mes a mes, contra su propio promedio.
 *
 * Tres decisiones que se ven en la pantalla:
 *
 *  1. El criterio de anomalía es **cada partida contra su propio promedio de meses anteriores**
 *     (elección del usuario). No hace falta presupuesto —que el sistema no tiene— y es el mismo
 *     criterio con el que CxP mide el desvío por proveedor.
 *
 *  2. **El umbral es del usuario, no del código.** Un porcentaje fijo escondido en un `if` decide
 *     qué se considera «descontrolado» sin que nadie lo haya acordado. Acá se escribe en pantalla y
 *     se ve cuál está aplicado.
 *
 *  3. **El semáforo de datos va primero.** Un mes a medio clasificar muestra menos gasto del que
 *     tuvo. Al construir esto, julio estaba al 30 % y aparentaba seis veces menos gasto que agosto:
 *     sin el semáforo, la primera conclusión de la pantalla habría sido «el gasto se multiplicó por
 *     seis», falsa y con toda la autoridad de un número.
 */

import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  Badge,
  BuscadorMultiple,
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
} from "@/components/ui";
import { cn } from "@/lib/cn";
import {
  componerPeriodo,
  etiquetaPeriodo,
  formatMoneda,
  formatMonto,
  partesPeriodo,
  toNumber,
} from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { useAnalisisPartidas } from "@/features/bancos/hooks";
import { PeriodoSelector } from "@/features/bancos/components/PeriodoSelector";
import { Sparkline } from "@/features/bancos/components/Sparkline";
import type { AnalisisPartidas, TendenciaPartida } from "@/api/bancos";

/** Cuántos meses hacia atrás mira el análisis. El tope de 24 es el del endpoint. */
const TRAMOS = [
  { meses: 6, label: "Semestre" },
  { meses: 12, label: "12 meses" },
  { meses: 24, label: "24 meses" },
];

/**
 * Los cuatro grupos de partidas, más «todas».
 *
 * «Otros rubros» y «Falta decidir» son dos cosas distintas que antes se mostraban juntas bajo un
 * rótulo que mentía: Utilidades, Proyecto Edificio y Ahorro son rubros legítimos del negocio que el
 * usuario decidió dejar fuera del EBITDA, no omisiones. La diferencia la da `naturaleza_declarada`
 * (migración 0064): sin esa bandera el sistema no podía distinguir la decisión del silencio.
 */
const GRUPOS = [
  { id: "GASTO", label: "Gastos" },
  { id: "INGRESO", label: "Ingresos" },
  { id: "OTROS", label: "Otros rubros" },
  { id: "PENDIENTE", label: "Falta decidir" },
  { id: "TODAS", label: "Todas" },
] as const;

type GrupoFiltro = (typeof GRUPOS)[number]["id"];

/** A qué grupo pertenece una partida. */
function grupoDe(p: TendenciaPartida): Exclude<GrupoFiltro, "TODAS"> {
  const nat = p.naturaleza || "NEUTRO";
  if (nat === "GASTO" || nat === "INGRESO") return nat;
  return p.naturaleza_declarada ? "OTROS" : "PENDIENTE";
}

/** Qué explica cada grupo, debajo de los filtros. */
const AYUDA_GRUPO: Record<GrupoFiltro, string> = {
  GASTO: "",
  INGRESO: "",
  OTROS:
    "Rubros que se declararon fuera del EBITDA a propósito: tesorería, ahorro, reservas, aportes entre empresas, proyectos. Se analizan igual que los gastos —el desvío contra su propio promedio es el mismo— pero no suman al resultado.",
  PENDIENTE:
    "Nadie declaró todavía si son ingreso o gasto, así que quedan fuera del EBITDA por omisión, no por decisión. Mientras estén acá, el resultado del período está incompleto.",
  TODAS: "Todas las partidas del rango, sin importar su naturaleza.",
};

/** Resta meses a un período YYYY-MM sin pasar por Date (que arrastra la zona horaria). */
function restarMeses(periodo: string, n: number): string {
  const { anio, mes } = partesPeriodo(periodo);
  const total = anio * 12 + (mes - 1) - n;
  return componerPeriodo(Math.floor(total / 12), (total % 12) + 1);
}

/** El mes de la serie en su forma corta: «ago 26». La tabla puede tener 24 columnas. */
function mesCorto(periodo: string): string {
  const { anio, mes } = partesPeriodo(periodo);
  const nombres = ["ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "set", "oct", "nov", "dic"];
  return `${nombres[mes - 1] ?? "?"} ${String(anio).slice(2)}`;
}

/** Lo que se apartó del promedio, en plata: es lo que decide si vale la pena mirarlo. */
function diferencia(p: TendenciaPartida): number {
  return toNumber(p.ultimo) - toNumber(p.promedio);
}

export function AnalisisPage() {
  const { periodo, setPeriodo } = usePeriodoActivo();
  const navigate = useNavigate();
  const [hasta, setHasta] = useState(periodo);
  const [meses, setMeses] = useState(6);
  const [umbral, setUmbral] = useState("25");
  const [grupo, setGrupo] = useState<GrupoFiltro>("GASTO");
  const [clasifs, setClasifs] = useState<string[]>([]);

  const desde = useMemo(() => restarMeses(hasta, meses - 1), [hasta, meses]);
  const q = useAnalisisPartidas(desde, hasta);
  const data: AnalisisPartidas | undefined = q.data;

  const umbralNum = Math.abs(toNumber(umbral.replace(",", ".")));

  // Meses del rango, en el orden en que vienen: son las columnas de la tabla.
  const periodos = useMemo(() => data?.meses.map((m) => m.periodo) ?? [], [data]);
  const noComparable = useMemo(
    () => new Set((data?.meses ?? []).filter((m) => !m.comparable).map((m) => m.periodo)),
    [data],
  );

  const delTipo = useMemo(
    () =>
      (data?.partidas ?? []).filter((p) => grupo === "TODAS" || grupoDe(p) === grupo),
    [data, grupo],
  );

  const opciones = useMemo(
    () =>
      delTipo.map((p) => ({
        value: p.clasificacion_id,
        label: p.clasificacion,
        grupo: p.concepto,
      })),
    [delTipo],
  );

  const filtradas = useMemo(
    () => (clasifs.length === 0 ? delTipo : delTipo.filter((p) => clasifs.includes(p.clasificacion_id))),
    [delTipo, clasifs],
  );

  // Fuera de cauce: se apartó más que el umbral Y hay historia con la que comparar. Se ordena por
  // PLATA, no por porcentaje: un +300 % sobre ₡2.000 no es noticia y taparía lo que sí importa.
  const fueraDeCauce = useMemo(
    () =>
      filtradas
        .filter((p) => p.confiable && Math.abs(toNumber(p.desvio_pct)) >= umbralNum)
        .sort((a, b) => Math.abs(diferencia(b)) - Math.abs(diferencia(a))),
    [filtradas, umbralNum],
  );

  // Lo que NO se puede juzgar todavía. Se muestra aparte en vez de desaparecer: una partida sin
  // historia no es una partida en orden.
  const sinHistoria = useMemo(() => filtradas.filter((p) => !p.confiable), [filtradas]);

  const totalRango = useMemo(
    () => filtradas.reduce((acc, p) => acc + toNumber(p.total), 0),
    [filtradas],
  );
  const totalUltimo = useMemo(
    () => filtradas.reduce((acc, p) => acc + toNumber(p.ultimo), 0),
    [filtradas],
  );

  /** Ir a clasificar el mes que está flojo: el semáforo señala el problema y también la acción. */
  function irAClasificar(p: string) {
    setPeriodo(p);
    navigate("/clasificar");
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Análisis y tendencias"
        description="Cada partida mes a mes contra su propio promedio: qué se movió, cuánto y desde cuándo."
        actions={
          <div className="flex flex-wrap items-end gap-2">
            <div className="flex items-end gap-1">
              {TRAMOS.map((t) => (
                <button
                  key={t.meses}
                  type="button"
                  onClick={() => setMeses(t.meses)}
                  className={cn(
                    "rounded-md border px-3 py-2 text-sm font-medium transition-colors",
                    meses === t.meses
                      ? "border-accent bg-accent text-white"
                      : "border-border bg-surface text-content-muted hover:text-content",
                  )}
                >
                  {t.label}
                </button>
              ))}
            </div>
            <PeriodoSelector label="Hasta" value={hasta} onChange={setHasta} id="analisis-hasta" />
          </div>
        }
      />

      {q.isLoading && <LoadingState label="Cruzando las partidas con su historia…" />}
      {q.isError && <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />}

      {data && (
        <>
          {/* El aviso no bloquea el análisis, pero se lee antes que cualquier número. */}
          {data.aviso && (
            <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
              <p className="text-sm font-medium text-content">Ojo con estos datos</p>
              <p className="mt-0.5 text-sm text-content-muted">{data.aviso}</p>
            </div>
          )}

          <Card>
            <CardHeader>
              <CardTitle>
                Datos del rango · {etiquetaPeriodo(desde)} a {etiquetaPeriodo(hasta)}
              </CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-2">
              <p className="text-sm text-content-muted">
                Un mes entra al promedio solo si está clasificado al 90 % o más. A medio clasificar
                muestra menos gasto del que tuvo, y compararlo contra él inventa anomalías.
              </p>
              <div className="flex flex-wrap gap-2">
                {data.meses.map((m) => {
                  const pct = toNumber(m.pct_clasificado);
                  const vacio = m.movs === 0;
                  return (
                    <div
                      key={m.periodo}
                      className={cn(
                        "flex min-w-32 flex-col gap-0.5 rounded-lg border px-3 py-2",
                        m.comparable
                          ? "border-positivo/40 bg-positivo/5"
                          : vacio
                            ? "border-border bg-surface-muted"
                            : "border-pendiente/50 bg-pendiente/10",
                      )}
                    >
                      <span className="text-sm font-medium text-content">
                        {etiquetaPeriodo(m.periodo)}
                      </span>
                      {vacio ? (
                        <>
                          <span className="text-xs text-content-muted">sin movimientos</span>
                          <Link
                            to="/importador"
                            className="text-xs font-medium text-accent underline"
                          >
                            importar
                          </Link>
                        </>
                      ) : (
                        <>
                          <span className="text-xs tabular-nums text-content-muted">
                            {m.movs.toLocaleString("es-CR")} movs · {pct.toFixed(1)} % clasificado
                          </span>
                          {m.comparable ? (
                            <Badge tone="positivo">comparable</Badge>
                          ) : (
                            <button
                              type="button"
                              onClick={() => irAClasificar(m.periodo)}
                              className="self-start text-xs font-medium text-accent underline"
                            >
                              clasificar este mes
                            </button>
                          )}
                        </>
                      )}
                    </div>
                  );
                })}
              </div>
            </CardContent>
          </Card>

          {/* Filtros: qué grupo se está mirando, cuáles partidas y con qué umbral. */}
          <Card>
            <CardContent className="flex flex-col gap-4 pt-6">
              <div className="flex flex-wrap items-end justify-between gap-4">
                <div className="flex flex-wrap items-end gap-1">
                  {GRUPOS.map((g) => {
                    const cuantas =
                      g.id === "TODAS"
                        ? (data.partidas ?? []).length
                        : (data.partidas ?? []).filter((p) => grupoDe(p) === g.id).length;
                    return (
                      <button
                        key={g.id}
                        type="button"
                        onClick={() => {
                          setGrupo(g.id);
                          setClasifs([]);
                        }}
                        className={cn(
                          "rounded-md border px-3 py-2 text-sm font-medium transition-colors",
                          grupo === g.id
                            ? "border-accent bg-accent text-white"
                            : "border-border bg-surface text-content-muted hover:text-content",
                        )}
                      >
                        {g.label} ({cuantas})
                      </button>
                    );
                  })}
                </div>
                <div className="w-44">
                  <Input
                    label="Avisar si se aparta más de"
                    value={umbral}
                    onChange={(e) => setUmbral(e.target.value)}
                    inputMode="decimal"
                    hint="% sobre su propio promedio"
                  />
                </div>
              </div>
              <BuscadorMultiple
                label="Partidas"
                opciones={opciones}
                seleccion={clasifs}
                onChange={setClasifs}
                placeholder="Escribí para filtrar (combustible, planilla…)"
                leyendaVacio="sin elegir = todas"
              />
              {AYUDA_GRUPO[grupo] && (
                <p className="text-sm text-content-muted">
                  {AYUDA_GRUPO[grupo]}
                  {grupo === "PENDIENTE" && (
                    <>
                      {" "}
                      <Link to="/catalogo" className="font-medium text-accent underline">
                        Declararlas en el Catálogo
                      </Link>
                      . Decir «no entra al EBITDA» también es declararlas: pasan a «Otros rubros» y
                      dejan de contar como pendientes.
                    </>
                  )}
                </p>
              )}
            </CardContent>
          </Card>

          <div className="grid gap-4 sm:grid-cols-3">
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Total del rango</p>
                {/* Con «Todas» no se muestra una sola cifra: sumar ingresos con gastos en un mismo
                    total no significa nada, y un número grande en colones se lee como si sí. */}
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {grupo === "TODAS" ? "—" : formatMoneda(String(totalRango))}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  {grupo === "TODAS"
                    ? `${filtradas.length} partida(s): elegí un grupo para ver el total`
                    : `${filtradas.length} partida(s) · ${meses} meses`}
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">{etiquetaPeriodo(hasta)}</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {grupo === "TODAS" ? "—" : formatMoneda(String(totalUltimo))}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  {grupo === "TODAS" ? "ingresos y gastos no se suman juntos" : "el mes que se está juzgando"}
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Fuera de cauce</p>
                {/* Con menos de 3 meses comparables, un «0» se leería como «todo en orden». No hay
                    nada en orden: no hay con qué comparar, y eso se dice con un guion. */}
                {/* La condición es la MISMA que decide si la tabla de abajo tiene filas: antes la
                    tarjeta decía «—» por meses_comparables mientras la tabla listaba partidas, porque
                    una partida puede tener dos meses previos comparables aunque el rango tenga solo
                    dos en total. Dos números del mismo hecho no pueden discrepar. */}
                {fueraDeCauce.length === 0 && data.meses_comparables < 3 ? (
                  <>
                    <p className="mt-1 text-2xl font-semibold text-content-muted">—</p>
                    <p className="mt-0.5 text-xs text-content-muted">
                      sin historia suficiente para comparar
                    </p>
                  </>
                ) : (
                  <>
                    <p
                      className={cn(
                        "mt-1 text-2xl font-semibold tabular-nums",
                        fueraDeCauce.length > 0 ? "text-negativo" : "text-content",
                      )}
                    >
                      {fueraDeCauce.length}
                    </p>
                    <p className="mt-0.5 text-xs text-content-muted">
                      se apartaron más de {umbralNum} % de su promedio
                    </p>
                  </>
                )}
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Lo que se salió de su cauce</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="mb-3 text-sm text-content-muted">
                Ordenado por plata, no por porcentaje: un +300 % sobre ₡2.000 no es noticia. El
                promedio es de los meses anteriores comparables, sin incluir{" "}
                {etiquetaPeriodo(hasta)}.
              </p>
              {fueraDeCauce.length === 0 ? (
                // Sin historia, «0 fuera de cauce» se leería como «todo en orden», que es lo
                // contrario de lo que pasa: todavía no hay con qué comparar. Se dice cuál es el
                // paso que falta, no un consejo genérico.
                <EmptyState
                  message={
                    data.meses_comparables < 3
                      ? `Todavía no se puede juzgar nada: hacen falta al menos 3 meses clasificados al 90 % y hay ${data.meses_comparables}. Clasificá los meses en amarillo o cargá los estados de cuenta que faltan y esta tabla se llena sola.`
                      : `Ninguna partida se apartó más de ${umbralNum} % de su propio promedio. Bajá el umbral para ver movimientos más pequeños.`
                  }
                />
              ) : (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Partida</TH>
                        <TH>Tendencia</TH>
                        <TH className="text-right">Promedio</TH>
                        <TH className="text-right">{etiquetaPeriodo(hasta)}</TH>
                        <TH className="text-right">Diferencia</TH>
                        <TH className="text-right">Desvío</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {fueraDeCauce.map((p) => {
                        const dif = diferencia(p);
                        const desvio = toNumber(p.desvio_pct);
                        return (
                          <TR key={`${p.concepto_id}-${p.clasificacion_id}`}>
                            <TD>
                              <span className="font-medium text-content">{p.clasificacion}</span>
                              <span className="block text-xs text-content-muted">{p.concepto}</span>
                            </TD>
                            <TD>
                              <Sparkline
                                valores={p.serie.map((s) => toNumber(s.monto))}
                                huecos={p.serie.map((s) => noComparable.has(s.periodo))}
                                titulo={`${p.clasificacion}: ${p.serie
                                  .map((s) => `${mesCorto(s.periodo)} ${formatMonto(s.monto)}`)
                                  .join(", ")}`}
                              />
                            </TD>
                            <TD className="text-right tabular-nums">
                              {formatMonto(p.promedio)}
                              <span className="block text-xs text-content-muted">
                                {p.meses_con_dato} mes(es)
                              </span>
                            </TD>
                            <TD className="text-right font-medium tabular-nums">
                              {formatMonto(p.ultimo)}
                            </TD>
                            <TD
                              className={cn(
                                "text-right tabular-nums",
                                dif > 0 ? "text-negativo" : "text-positivo",
                              )}
                            >
                              {dif > 0 ? "+" : "−"}
                              {formatMonto(String(Math.abs(dif)))}
                            </TD>
                            <TD className="text-right">
                              <Badge tone={desvio > 0 ? "negativo" : "positivo"}>
                                {desvio > 0 ? "+" : ""}
                                {desvio.toFixed(1)} %
                              </Badge>
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

          <Card>
            <CardHeader>
              <CardTitle>Mes a mes</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="mb-3 text-sm text-content-muted">
                Toda la parrilla, para ver el patrón con los ojos: qué es estacional, qué se paga una
                vez al año y qué viene subiendo despacio. Las columnas de meses a medio clasificar
                van en gris.
              </p>
              {filtradas.length === 0 ? (
                <EmptyState message="No hay movimientos clasificados con esta naturaleza en el rango elegido." />
              ) : (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Partida</TH>
                        {periodos.map((p) => (
                          <TH
                            key={p}
                            className={cn(
                              "text-right whitespace-nowrap",
                              noComparable.has(p) && "text-content-muted",
                            )}
                          >
                            {mesCorto(p)}
                          </TH>
                        ))}
                        <TH className="text-right">Total</TH>
                        <TH className="text-right">Promedio</TH>
                        <TH className="text-right">Desvío</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {filtradas
                        .slice()
                        .sort((a, b) => toNumber(b.total) - toNumber(a.total))
                        .map((p) => {
                          const desvio = toNumber(p.desvio_pct);
                          const alerta =
                            p.confiable && Math.abs(desvio) >= umbralNum;
                          return (
                            <TR key={`${p.concepto_id}-${p.clasificacion_id}`}>
                              <TD className="whitespace-nowrap">
                                <span className="font-medium text-content">{p.clasificacion}</span>
                                <span className="block text-xs text-content-muted">
                                  {p.concepto}
                                </span>
                              </TD>
                              {p.serie.map((s) => (
                                <TD
                                  key={s.periodo}
                                  className={cn(
                                    "text-right tabular-nums whitespace-nowrap",
                                    noComparable.has(s.periodo) && "text-content-muted",
                                  )}
                                >
                                  {toNumber(s.monto) === 0 ? "—" : formatMonto(s.monto)}
                                </TD>
                              ))}
                              <TD className="text-right font-medium tabular-nums">
                                {formatMonto(p.total)}
                              </TD>
                              <TD className="text-right tabular-nums text-content-muted">
                                {p.confiable ? formatMonto(p.promedio) : "—"}
                              </TD>
                              <TD className="text-right">
                                {p.confiable ? (
                                  <span
                                    className={cn(
                                      "tabular-nums",
                                      alerta
                                        ? desvio > 0
                                          ? "font-medium text-negativo"
                                          : "font-medium text-positivo"
                                        : "text-content-muted",
                                    )}
                                  >
                                    {desvio > 0 ? "+" : ""}
                                    {desvio.toFixed(1)} %
                                  </span>
                                ) : (
                                  <span className="text-xs text-content-muted">sin historia</span>
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

          {sinHistoria.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>Todavía no se pueden juzgar ({sinHistoria.length})</CardTitle>
              </CardHeader>
              <CardContent>
                <p className="mb-3 text-sm text-content-muted">
                  Estas partidas no tienen al menos dos meses anteriores comparables, así que un
                  «promedio» sería un solo dato y el desvío no significaría nada. Aparecen acá para
                  que no se lean como partidas en orden.
                </p>
                <div className="flex flex-wrap gap-2">
                  {sinHistoria
                    .slice()
                    .sort((a, b) => toNumber(b.total) - toNumber(a.total))
                    .map((p) => (
                      <span
                        key={`${p.concepto_id}-${p.clasificacion_id}`}
                        className="rounded-md border border-border bg-surface-muted px-2 py-1 text-xs text-content-muted"
                      >
                        {p.clasificacion} · {formatMonto(p.total)}
                      </span>
                    ))}
                </div>
              </CardContent>
            </Card>
          )}
        </>
      )}
    </div>
  );
}
