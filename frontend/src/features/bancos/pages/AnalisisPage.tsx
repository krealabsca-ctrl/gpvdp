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
import { descargarBlob } from "@/lib/csv";
import {
  etiquetaPeriodo,
  formatMoneda,
  formatMonto,
  partesPeriodo,
  toNumber,
} from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { useAnalisisPartidas, useAnalisisPartidasDiario } from "@/features/bancos/hooks";
import { PeriodoSelector } from "@/features/bancos/components/PeriodoSelector";
import { Sparkline } from "@/features/bancos/components/Sparkline";
import { DiarioPartidasChart } from "@/features/bancos/components/DiarioPartidasChart";
import {
  calcularConcentracion,
  calcularPuente,
  desdeDelTramo,
  mesesEntre,
  partidasQueExplican,
  problemaDelRango,
  restarMeses,
  type Tramo,
} from "@/features/bancos/analisisRango";
import { bancosApi, type AnalisisPartidas, type TendenciaPartida } from "@/api/bancos";

/**
 * Cuántos meses hacia atrás mira el análisis. El tope de 24 es el del endpoint.
 *
 * TRIMESTRE Y AÑO EN CURSO SE AGREGARON EL 27-oct-2026. Faltaban los dos cortes que más se usan
 * en una reunión de resultados, y la pantalla de Control —del mismo módulo— ya tenía el trimestre:
 * dos pantallas hermanas ofrecían cortes distintos sobre los mismos datos.
 */
const TRAMOS: { tramo: Tramo; label: string }[] = [
  { tramo: 3, label: "Trimestre" },
  { tramo: 6, label: "Semestre" },
  { tramo: 12, label: "12 meses" },
  { tramo: 24, label: "24 meses" },
  // El acumulado del año: de enero al mes elegido. Es el número que se lleva a un comité.
  { tramo: "YTD", label: "Año en curso" },
  { tramo: "LIBRE", label: "Personalizado" },
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

/** El mes de la serie en su forma corta: «ago 26». La tabla puede tener 24 columnas. */
function mesCorto(periodo: string): string {
  const { anio, mes } = partesPeriodo(periodo);
  const nombres = ["ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "set", "oct", "nov", "dic"];
  return `${nombres[mes - 1] ?? "?"} ${String(anio).slice(2)}`;
}

/** Contra qué se juzga el último mes. */
type BaseComparacion = "promedio" | "mediana";

/** El valor de referencia de la partida según la base elegida. */
function referenciaDe(p: TendenciaPartida, base: BaseComparacion): string {
  return base === "mediana" ? p.mediana : p.promedio;
}

/** El desvío del último mes contra esa referencia. */
function desvioDe(p: TendenciaPartida, base: BaseComparacion): string {
  return base === "mediana" ? p.desvio_mediana_pct : p.desvio_pct;
}

/** Lo que se apartó de la referencia, en plata: es lo que decide si vale la pena mirarlo. */
function diferencia(p: TendenciaPartida, base: BaseComparacion): number {
  return toNumber(p.ultimo) - toNumber(referenciaDe(p, base));
}

const NOMBRE_MES = [
  "Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
  "Julio", "Agosto", "Setiembre", "Octubre", "Noviembre", "Diciembre",
];

/** Cómo se dice cada grado de recurrencia en pantalla. */
const RECURRENCIA: Record<string, { label: string; tono: "neutral" | "accent" | "pendiente" }> = {
  FIJO: { label: "fijo", tono: "accent" },
  INTERMITENTE: { label: "intermitente", tono: "neutral" },
  ESPORADICO: { label: "esporádico", tono: "pendiente" },
};

export function AnalisisPage() {
  const { periodo, setPeriodo } = usePeriodoActivo();
  const navigate = useNavigate();
  const toast = useToast();
  const [hasta, setHasta] = useState(periodo);
  const [tramo, setTramo] = useState<Tramo>(6);
  const [desdeLibre, setDesdeLibre] = useState(() => restarMeses(periodo, 5));
  const [umbral, setUmbral] = useState("25");
  const [grupo, setGrupo] = useState<GrupoFiltro>("GASTO");
  const [clasifs, setClasifs] = useState<string[]>([]);
  const [comparar, setComparar] = useState(false);
  const [exportando, setExportando] = useState(false);
  // Contra qué se juzga el último mes. El promedio es lo de siempre; la mediana no se mueve
  // cuando un mes trae un pago extraordinario, que es cuando el promedio deja de servir.
  const [base, setBase] = useState<"promedio" | "mediana">("promedio");

  const desde = useMemo(() => desdeDelTramo(tramo, hasta, desdeLibre), [tramo, hasta, desdeLibre]);
  const mesesDelRango = useMemo(() => mesesEntre(desde, hasta), [desde, hasta]);
  const rangoInvalido = problemaDelRango(desde, hasta);

  const q = useAnalisisPartidas(desde, hasta, comparar, !rangoInvalido);
  const data: AnalisisPartidas | undefined = rangoInvalido ? undefined : q.data;

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
        .filter((p) => p.confiable && Math.abs(toNumber(desvioDe(p, base))) >= umbralNum)
        .sort((a, b) => Math.abs(diferencia(b, base)) - Math.abs(diferencia(a, base))),
    [filtradas, umbralNum, base],
  );

  const puente = useMemo(() => calcularPuente(filtradas), [filtradas]);

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

  const concentracion = useMemo(() => calcularConcentracion(filtradas), [filtradas]);
  const partidasPara80 = useMemo(() => partidasQueExplican(concentracion), [concentracion]);

  async function exportar() {
    setExportando(true);
    try {
      // El grupo viaja con el nombre que entiende el servidor: la pantalla usa OTROS/PENDIENTE y
      // el dominio NEUTRO/SIN_DECLARAR. Traducir acá evita que el archivo salga con otro corte.
      const grupoApi =
        grupo === "OTROS" ? "NEUTRO" : grupo === "PENDIENTE" ? "SIN_DECLARAR" : grupo;
      const { blob, filename } = await bancosApi.exportarAnalisisXLSX(desde, hasta, grupoApi, comparar);
      descargarBlob(filename || `Analisis ${desde} a ${hasta}.xlsx`, blob);
    } catch (e) {
      toast.error(mensajeError(e));
    } finally {
      setExportando(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Análisis y tendencias"
        description="Cada partida mes a mes contra su propio promedio: qué se movió, cuánto y desde cuándo."
        actions={
          <div className="flex flex-wrap items-end gap-2">
            <div className="flex flex-wrap items-end gap-1">
              {TRAMOS.map((t) => (
                <button
                  key={String(t.tramo)}
                  type="button"
                  onClick={() => {
                    // Al pasar a «Personalizado» se arranca desde el rango que ya se está
                    // mirando: empezar en blanco obligaría a reconstruirlo a mano.
                    if (t.tramo === "LIBRE") setDesdeLibre(desde);
                    setTramo(t.tramo);
                  }}
                  className={cn(
                    "rounded-md border px-3 py-2 text-sm font-medium transition-colors",
                    tramo === t.tramo
                      ? "border-accent bg-accent text-white"
                      : "border-border bg-surface text-content-muted hover:text-content",
                  )}
                >
                  {t.label}
                </button>
              ))}
            </div>
            {tramo === "LIBRE" && (
              <PeriodoSelector label="Desde" value={desdeLibre} onChange={setDesdeLibre} id="analisis-desde" />
            )}
            <PeriodoSelector label="Hasta" value={hasta} onChange={setHasta} id="analisis-hasta" />
          </div>
        }
      />

      {/* El rango imposible se dice acá y no se consulta: el servidor devolvería el mismo
          rechazo con peor redacción y después de ir y volver. */}
      {rangoInvalido && (
        <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
          <p className="text-sm font-medium text-content">Ese rango no se puede pedir</p>
          <p className="mt-0.5 text-sm text-content-muted">{rangoInvalido}</p>
        </div>
      )}

      {!rangoInvalido && q.isLoading && (
        <LoadingState label="Cruzando las partidas con su historia…" />
      )}
      {!rangoInvalido && q.isError && (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      )}

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
            <CardContent className="flex flex-wrap items-center gap-4 pt-5">
              <label className="flex items-center gap-2 text-sm text-content">
                <input
                  type="checkbox"
                  checked={comparar}
                  onChange={(e) => setComparar(e.target.checked)}
                />
                Comparar contra el mismo rango del año anterior
              </label>
              {data.compara_anual && data.desde_ant && (
                <span className="text-xs text-content-muted">
                  contra {etiquetaPeriodo(data.desde_ant)} a {etiquetaPeriodo(data.hasta_ant ?? "")}
                </span>
              )}
              <Button
                variant="secondary"
                className="ml-auto"
                disabled={exportando || filtradas.length === 0}
                onClick={exportar}
              >
                {exportando ? "Armando el archivo…" : "Exportar a Excel"}
              </Button>
            </CardContent>
          </Card>

          {/* El año anterior a medio clasificar hace que este año parezca un crecimiento que no
              ocurrió. Va aparte del otro aviso porque es un problema del OTRO rango. */}
          {data.aviso_anual && (
            <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
              <p className="text-sm font-medium text-content">Ojo con la comparación anual</p>
              <p className="mt-0.5 text-sm text-content-muted">{data.aviso_anual}</p>
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
              {/* Alineado por ARRIBA: con `items-end`, el texto de ayuda del umbral empujaba su
                  campo y los botones quedaban escalonados. La unidad vive en el label y el «%» se
                  dibuja dentro del campo. */}
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium text-content">Ver</span>
                  <div className="flex h-10 flex-wrap items-stretch gap-1">
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
                            "rounded-lg border px-3 text-sm font-medium shadow-sm transition-colors",
                            grupo === g.id
                              ? "border-accent bg-accent text-accent-fg"
                              : "border-border bg-surface-raised text-content-muted hover:text-content",
                          )}
                        >
                          {g.label} ({cuantas})
                        </button>
                      );
                    })}
                  </div>
                </div>
                {/* Label corto a propósito, por dos razones medidas: si no cabe en una línea se
                    parte en dos y baja su propio campo (el mismo escalón que causaba el texto de
                    ayuda), y con cinco botones al lado quedan ~240 px libres, así que un campo más
                    ancho se iba a una segunda fila. Contra qué se compara ya lo dice la tarjeta. */}
                <div className="flex items-start gap-4">
                  {/* Contra qué se juzga el último mes. Va al lado del umbral porque son los dos
                      parámetros del mismo juicio: cuánto es «mucho» y mucho respecto de qué. */}
                  <div className="flex flex-col gap-1.5">
                    <span className="text-sm font-medium text-content">Comparar contra</span>
                    <div className="flex h-10 items-stretch gap-1">
                      {(
                        [
                          ["promedio", "Promedio"],
                          ["mediana", "Mediana"],
                        ] as const
                      ).map(([id, label]) => (
                        <button
                          key={id}
                          type="button"
                          onClick={() => setBase(id)}
                          className={cn(
                            "rounded-lg border px-3 text-sm font-medium shadow-sm transition-colors",
                            base === id
                              ? "border-accent bg-accent text-white"
                              : "border-border bg-surface text-content-muted hover:text-content",
                          )}
                        >
                          {label}
                        </button>
                      ))}
                    </div>
                  </div>
                  <div className="w-48">
                    <div className="relative">
                      <Input
                        label="Umbral de desvío"
                        value={umbral}
                        onChange={(e) => setUmbral(e.target.value)}
                        inputMode="decimal"
                        className="pr-8"
                      />
                      <span className="pointer-events-none absolute bottom-0 right-3 flex h-10 items-center text-sm text-content-muted">
                        %
                      </span>
                    </div>
                  </div>
                </div>
              </div>
              {base === "mediana" && (
                <p className="text-sm text-content-muted">
                  La <b className="text-content">mediana</b> es el mes del medio: no se mueve
                  cuando un mes trae un pago extraordinario. El promedio sí, y a partir de ahí
                  todos los meses normales parecen una caída. Usala cuando la partida tenga pagos
                  grandes y salteados.
                </p>
              )}
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

          <DiaADia desde={desde} hasta={hasta} clasifs={clasifs} />

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
                    : `${filtradas.length} partida(s) · ${mesesDelRango} meses`}
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
                        const dif = diferencia(p, base);
                        const desvio = toNumber(desvioDe(p, base));
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
                              {formatMonto(referenciaDe(p, base))}
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

          {/* ── El puente: qué explica la variación ────────────────────────────────────────
              Solo con la comparación anual activa: sin un período contra el cual medir no hay
              variación que explicar. La tabla dice cuánto varió CADA partida en porcentaje; esto
              dice quién movió el TOTAL, en plata, que es lo que se lleva a una reunión. */}
          {data.compara_anual && puente.tramos.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>Qué explica la variación</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-3">
                <div className="flex flex-wrap items-baseline gap-2 text-sm text-content-muted">
                  <span>
                    De {formatMoneda(String(puente.totalAnterior))} a{" "}
                    {formatMoneda(String(puente.totalActual))}:
                  </span>
                  <b
                    className={cn(
                      "text-base",
                      puente.cambio > 0 ? "text-negativo" : "text-positivo",
                    )}
                  >
                    {puente.cambio > 0 ? "+" : "−"}
                    {formatMoneda(String(Math.abs(puente.cambio)))}
                  </b>
                </div>
                <div className="flex flex-col gap-2">
                  {puente.tramos.map((t) => (
                    <div key={t.clasificacionId} className="flex flex-wrap items-center gap-3">
                      <div className="w-56 shrink-0">
                        <span className="text-sm font-medium text-content">{t.clasificacion}</span>
                        <span className="block text-xs text-content-muted">{t.concepto}</span>
                      </div>
                      {/* La barra se dibuja contra el tramo MÁS GRANDE, no contra el cambio
                          total: si las subidas y las bajadas casi se cancelan, el cambio neto es
                          chico y todas las barras se saldrían de la escala. */}
                      <div className="flex h-5 flex-1 items-center">
                        <div className="relative h-3 w-full rounded-full bg-surface-muted">
                          <div
                            className={cn(
                              "absolute h-full rounded-full",
                              t.aporte > 0 ? "bg-negativo" : "bg-positivo",
                            )}
                            style={{
                              width: `${Math.min(
                                (Math.abs(t.aporte) * 100) /
                                  Math.max(...puente.tramos.map((x) => Math.abs(x.aporte))),
                                100,
                              )}%`,
                            }}
                          />
                        </div>
                      </div>
                      <span
                        className={cn(
                          "w-32 shrink-0 text-right tabular-nums text-sm",
                          t.aporte > 0 ? "text-negativo" : "text-positivo",
                        )}
                      >
                        {t.aporte > 0 ? "+" : "−"}
                        {formatMonto(String(Math.abs(t.aporte)))}
                      </span>
                      <span className="w-24 shrink-0 text-right text-xs text-content-muted">
                        {t.esNueva ? "partida nueva" : `${t.pctDelCambio.toFixed(0)} % del cambio`}
                      </span>
                    </div>
                  ))}
                </div>
                {puente.restoPartidas > 0 && (
                  <p className="text-xs text-content-muted">
                    Las otras {puente.restoPartidas} partidas aportan{" "}
                    {puente.resto > 0 ? "+" : "−"}
                    {formatMonto(String(Math.abs(puente.resto)))} entre todas.
                  </p>
                )}
                {data.aviso_anual && (
                  <p className="text-xs text-pendiente">
                    Ojo: parte de esta variación puede ser reclasificación. Ver el aviso de arriba.
                  </p>
                )}
              </CardContent>
            </Card>
          )}

          {/* ── Estacionalidad ─────────────────────────────────────────────────────────────
              Separa «diciembre siempre es alto» de «diciembre se disparó». Un mes con un solo
              año NO es estacionalidad: es el dato de ese mes, y se muestra en gris sin opinar. */}
          {(data.estacionalidad?.length ?? 0) > 1 && (
            <Card>
              <CardHeader>
                <CardTitle>Estacionalidad · qué meses pesan más</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-3">
                <p className="text-sm text-content-muted">
                  100 es un mes cualquiera del rango. Un mes en 141 gasta un 41 % más que el
                  típico, y ese 41 % no es una anomalía: es cómo es ese mes.{" "}
                  {(data.meses_con_dos_anios ?? 0) < 2 ? (
                    <b className="text-pendiente">
                      Todavía no hay ningún mes con dos años cargados, así que esto es el dato de
                      un año y no un patrón. Hace falta más histórico.
                    </b>
                  ) : (
                    <>
                      Solo <b className="text-content">{data.meses_con_dos_anios}</b> de los meses
                      tienen dos años o más; los demás van en gris porque todavía no son un patrón.
                    </>
                  )}
                </p>
                <div className="flex flex-col gap-1.5">
                  {data.estacionalidad?.map((e) => {
                    const idx = toNumber(e.indice);
                    return (
                      <div key={e.mes} className="flex items-center gap-3">
                        <span
                          className={cn(
                            "w-24 shrink-0 text-sm",
                            e.confiable ? "text-content" : "text-content-muted",
                          )}
                        >
                          {NOMBRE_MES[e.mes - 1]}
                        </span>
                        <div className="relative h-3 flex-1 rounded-full bg-surface-muted">
                          {/* La barra arranca en el centro: lo que importa es cuánto se aparta
                              de 100 y hacia qué lado, no el valor absoluto. */}
                          <div className="absolute left-1/2 h-full w-px bg-border" />
                          <div
                            className={cn(
                              "absolute h-full rounded-full",
                              !e.confiable
                                ? "bg-content-muted/40"
                                : idx >= 100
                                  ? "bg-negativo"
                                  : "bg-positivo",
                            )}
                            style={
                              idx >= 100
                                ? { left: "50%", width: `${Math.min((idx - 100) / 2, 50)}%` }
                                : { right: "50%", width: `${Math.min((100 - idx) / 2, 50)}%` }
                            }
                          />
                        </div>
                        <span
                          className={cn(
                            "w-16 shrink-0 text-right tabular-nums text-sm",
                            e.confiable ? "text-content" : "text-content-muted",
                          )}
                        >
                          {e.indice}
                        </span>
                        <span className="w-28 shrink-0 text-right text-xs text-content-muted">
                          {e.anios === 1 ? "1 año · sin patrón" : `${e.anios} años`}
                        </span>
                      </div>
                    );
                  })}
                </div>
              </CardContent>
            </Card>
          )}

          {/* ── Concentración ──────────────────────────────────────────────────────────────
              Dónde está el dinero, que no es la misma pregunta que qué se movió. La tabla de
              arriba ordena por DESVÍO, así que una partida chica que se duplicó aparece antes que
              la que se lleva un tercio del gasto. */}
          {concentracion.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>Concentración · dónde está el dinero</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-3">
                <p className="text-sm text-content-muted">
                  <b className="text-content">
                    {partidasPara80} de {concentracion.length} partidas
                  </b>{" "}
                  explican el 80 % del total del rango. Es otra lectura que la de arriba: acá no
                  importa cuánto se movió una partida, sino cuánto pesa.
                </p>
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH className="text-right">#</TH>
                        <TH>Partida</TH>
                        <TH className="text-right">Total del rango</TH>
                        <TH className="text-right">% del total</TH>
                        <TH className="text-right">Acumulado</TH>
                        <TH>Peso</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {/* Solo las que llegan al 80 %, más una: la cola larga es ruido y alargaría
                          la tabla con partidas de ₡2.000. La lista completa está en el .xlsx. */}
                      {concentracion.slice(0, Math.min(partidasPara80 + 1, concentracion.length)).map((c, i) => (
                        <TR key={`${c.p.concepto_id}-${c.p.clasificacion_id}`}>
                          <TD className="text-right tabular-nums text-content-muted">{i + 1}</TD>
                          <TD className="whitespace-nowrap">
                            <span className="font-medium text-content">{c.p.clasificacion}</span>
                            <span className="block text-xs text-content-muted">{c.p.concepto}</span>
                          </TD>
                          <TD className="text-right font-medium tabular-nums">
                            {formatMonto(c.p.total)}
                          </TD>
                          <TD className="text-right tabular-nums">{c.pct.toFixed(1)} %</TD>
                          <TD
                            className={cn(
                              "text-right tabular-nums",
                              c.pctAcum <= 80 ? "text-content" : "text-content-muted",
                            )}
                          >
                            {c.pctAcum.toFixed(1)} %
                          </TD>
                          <TD>
                            {/* La barra hace visible de un vistazo la caída entre la primera
                                partida y la quinta, que es el dato de la concentración. */}
                            <div className="h-2 w-28 overflow-hidden rounded-full bg-surface-muted">
                              <div
                                className="h-full rounded-full bg-accent"
                                style={{ width: `${Math.min(c.pct, 100)}%` }}
                              />
                            </div>
                          </TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
                {concentracion.length > partidasPara80 + 1 && (
                  <p className="text-xs text-content-muted">
                    Las otras {concentracion.length - partidasPara80 - 1} partidas se reparten el{" "}
                    {(100 - (concentracion[partidasPara80]?.pctAcum ?? 100)).toFixed(1)} % restante.
                    Están todas en el archivo de Excel.
                  </p>
                )}
              </CardContent>
            </Card>
          )}

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
                        {data.compara_anual && (
                          <>
                            <TH className="text-right whitespace-nowrap">Año anterior</TH>
                            <TH className="text-right whitespace-nowrap">Var. anual</TH>
                          </>
                        )}
                        <TH className="text-right">Promedio</TH>
                        <TH className="text-right">Desvío</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {filtradas
                        .slice()
                        .sort((a, b) => toNumber(b.total) - toNumber(a.total))
                        .map((p) => {
                          const desvio = toNumber(desvioDe(p, base));
                          const alerta =
                            p.confiable && Math.abs(desvio) >= umbralNum;
                          return (
                            <TR key={`${p.concepto_id}-${p.clasificacion_id}`}>
                              <TD className="whitespace-nowrap">
                                <span className="font-medium text-content">{p.clasificacion}</span>
                                <span className="block text-xs text-content-muted">
                                  {p.concepto}
                                  {/* La recurrencia va pegada a la partida: para una esporádica
                                      el promedio y el desvío de las columnas de la derecha no
                                      significan nada, y hay que poder verlo en la misma fila. */}
                                  {p.recurrencia && RECURRENCIA[p.recurrencia] && (
                                    <>
                                      {" · "}
                                      <span
                                        className={cn(
                                          p.recurrencia === "ESPORADICO" && "text-pendiente",
                                        )}
                                      >
                                        {RECURRENCIA[p.recurrencia]!.label}
                                      </span>
                                    </>
                                  )}
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
                              {data.compara_anual && (
                                <>
                                  <TD className="text-right tabular-nums text-content-muted">
                                    {p.sin_base_anterior ? "—" : formatMonto(p.total_anterior ?? "0")}
                                  </TD>
                                  <TD className="text-right">
                                    {p.sin_base_anterior ? (
                                      // «No existía» no es «no varió» ni «cayó 100 %»: son tres
                                      // lecturas distintas y solo una es cierta.
                                      <span className="text-xs text-content-muted">no existía</span>
                                    ) : (
                                      <span
                                        className={cn(
                                          "tabular-nums",
                                          toNumber(p.variacion_anual_pct ?? "0") > 0
                                            ? "text-negativo"
                                            : "text-positivo",
                                        )}
                                      >
                                        {toNumber(p.variacion_anual_pct ?? "0") > 0 ? "+" : ""}
                                        {p.variacion_anual_pct} %
                                      </span>
                                    )}
                                  </TD>
                                </>
                              )}
                              <TD className="text-right tabular-nums text-content-muted">
                                {p.confiable ? formatMonto(referenciaDe(p, base)) : "—"}
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

/**
 * Día a día de las partidas seleccionadas: CUÁNDO se movió la plata.
 *
 * Es una vista aparte y no una granularidad de la tabla mensual, y la razón es de fondo: el
 * análisis mensual JUZGA —compara cada partida contra su propio promedio y marca lo que se salió
 * de cauce—, y ese juicio no se traduce al día. El gasto diario es a saltos, así que «este día se
 * desvió 400 % del promedio diario» marcaría casi todos los días de pago. Acá no hay promedio ni
 * desvío: hay una serie.
 *
 * Sin partidas elegidas no se pide nada. El día a día de las 168 partidas no es una respuesta.
 */
function DiaADia({ desde, hasta, clasifs }: { desde: string; hasta: string; clasifs: string[] }) {
  const q = useAnalisisPartidasDiario(desde, hasta, clasifs);

  if (clasifs.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Día a día</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-content-muted">
            Elegí una o más partidas arriba para ver en qué días se movió la plata. Sirve para
            distinguir un gasto que fue <b className="text-content">de golpe</b> de uno{" "}
            <b className="text-content">repartido</b> en el mes, que en la tabla mensual se ven
            igual.
          </p>
        </CardContent>
      </Card>
    );
  }

  const data = q.data;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Día a día</CardTitle>
        {data && (
          <p className="mt-0.5 text-xs text-content-muted">
            {data.dias_con_movimiento} día(s) con movimiento entre {etiquetaPeriodo(desde)} y{" "}
            {etiquetaPeriodo(hasta)}. Los días sin movimiento no se dibujan: una barra en cero no
            es un dato.
          </p>
        )}
      </CardHeader>
      <CardContent>
        {q.isLoading && <LoadingState label="Buscando los movimientos día por día…" />}
        {q.isError && <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />}
        {data && (
          <>
            <DiarioPartidasChart partidas={data.partidas} />
            {data.partidas.length > 0 && (
              <TableContainer className="mt-4">
                <Table>
                  <THead>
                    <TR>
                      <TH>Partida</TH>
                      <TH className="text-right">Total del rango</TH>
                      <TH className="text-right">Movs</TH>
                      <TH className="text-right">Días con movimiento</TH>
                      <TH>El día más grande</TH>
                    </TR>
                  </THead>
                  <TBody>
                    {data.partidas.map((p) => {
                      // El día más grande es lo que contesta «¿fue de golpe?»: si concentra la
                      // mayor parte del total, la respuesta es sí y se ve sin leer la gráfica.
                      const mayor = p.dias.reduce(
                        (a, d) => (toNumber(d.monto) > toNumber(a.monto) ? d : a),
                        p.dias[0] ?? { fecha: "", monto: "0", movs: 0 },
                      );
                      const total = toNumber(p.total);
                      const pct = total !== 0 ? (toNumber(mayor.monto) / total) * 100 : 0;
                      return (
                        <TR key={p.clasificacion_id}>
                          <TD>
                            <span className="font-medium text-content">{p.clasificacion}</span>
                            <span className="block text-xs text-content-muted">{p.concepto}</span>
                          </TD>
                          <TD className="text-right font-medium tabular-nums">
                            {formatMoneda(p.total)}
                          </TD>
                          <TD className="text-right tabular-nums">{p.movs}</TD>
                          <TD className="text-right tabular-nums">{p.dias.length}</TD>
                          <TD className="text-xs">
                            {p.dias.length === 0 ? (
                              <span className="text-content-muted">sin movimiento</span>
                            ) : (
                              <>
                                <span className="text-content">{mayor.fecha}</span>{" "}
                                <span className="text-content-muted">
                                  {formatMoneda(mayor.monto)} ({Math.round(pct)} % del total)
                                </span>
                              </>
                            )}
                          </TD>
                        </TR>
                      );
                    })}
                  </TBody>
                </Table>
              </TableContainer>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
