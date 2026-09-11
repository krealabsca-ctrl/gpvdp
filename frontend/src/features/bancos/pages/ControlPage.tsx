/**
 * Pantalla — Control por departamento y presupuesto (/control).
 *
 * Contesta la pregunta del usuario (2026-08-20): «¿este departamento está gastando de más?».
 *
 * ── Las cuatro decisiones que se ven en la pantalla ──────────────────────────
 *
 *  1. **La atribución se hace en la PARTIDA, no movimiento por movimiento.** Desde la fila «sin
 *     asignar» se ve en qué partidas está ese gasto y se le pone el departamento ahí mismo: un
 *     selector cubre toda la historia de esa partida y todo su futuro. Es la diferencia entre
 *     configurar una vez y teclear 5.000 veces.
 *
 *  2. **El gasto sin dueño se muestra aparte y no se reparte.** Repartirlo entre los departamentos
 *     haría que cada uno pareciera más chico de lo que es, y esconderlo haría que los totales no
 *     cuadraran con el banco.
 *
 *  3. **Un mes a medio clasificar consume MENOS presupuesto del que consumió.** El semáforo va
 *     arriba: sin él, un departamento al 40 % de su presupuesto podría estar en realidad pasado, y
 *     la pantalla habría invitado a seguir gastando.
 *
 *  4. **El presupuesto del departamento y el desglose por partida son dos cosas distintas.** El
 *     total es lo aprobado; los subpresupuestos son cómo se piensa repartir. La pantalla muestra
 *     cuánto queda sin repartir y avisa cuando el desglose reparte MÁS de lo autorizado —un
 *     problema que el semáforo del consumo no puede ver, porque el gasto todavía no llegó—.
 *
 * El departamento dice QUIÉN gastó y la sede DÓNDE: son dos dimensiones independientes de la
 * partida, que dice QUÉ se gastó. Meter el lugar dentro del nombre de la partida —como pasó con las
 * 22 «Caja Chica - X»— multiplica el catálogo y hace imposible sumar por área.
 */

import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
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
  Select,
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
} from "@/components/ui";
import type { BadgeTone } from "@/components/ui";
import { cn } from "@/lib/cn";
import {
  componerPeriodo,
  etiquetaPeriodo,
  formatMoneda,
  formatMonto,
  montoParaApi,
  partesPeriodo,
  toNumber,
} from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import {
  useAsignarDimensionesPartida,
  useBorrarPresupuesto,
  useControl,
  useDepartamentosDeBancos,
  useGuardarPresupuesto,
  usePartidasDeDimension,
  usePresupuesto,
  useSedes,
} from "@/features/bancos/hooks";
import { PeriodoSelector } from "@/features/bancos/components/PeriodoSelector";
import { AdminDepartamentos } from "@/features/bancos/components/AdminDepartamentos";
import type {
  EstadoControl,
  FilaControl,
  GastoPartidaDimension,
  OrigenDimension,
} from "@/api/bancos";

const TRAMOS = [
  { meses: 1, label: "Este mes" },
  { meses: 3, label: "Trimestre" },
  { meses: 6, label: "Semestre" },
  { meses: 12, label: "12 meses" },
];

const TONO_ESTADO: Record<EstadoControl, BadgeTone> = {
  SIN_PRESUPUESTO: "neutral",
  EN_RANGO: "positivo",
  ALERTA: "pendiente",
  EXCEDIDO: "negativo",
};

const ETIQUETA_ESTADO: Record<EstadoControl, string> = {
  SIN_PRESUPUESTO: "sin presupuesto",
  EN_RANGO: "en rango",
  ALERTA: "cerca del tope",
  EXCEDIDO: "se pasó",
};

/** Los estados por los que se puede filtrar, con el nombre que usa el usuario. */
const FILTROS_ESTADO = [
  { id: "", label: "Todos" },
  { id: "EXCEDIDO", label: "Se pasaron" },
  { id: "ALERTA", label: "Cerca del tope" },
  { id: "EN_RANGO", label: "En rango" },
  { id: "SIN_PRESUPUESTO", label: "Sin presupuesto" },
] as const;

/** Resta meses a un período YYYY-MM sin pasar por Date (que arrastra la zona horaria). */
function restarMeses(periodo: string, n: number): string {
  const { anio, mes } = partesPeriodo(periodo);
  const total = anio * 12 + (mes - 1) - n;
  return componerPeriodo(Math.floor(total / 12), (total % 12) + 1);
}

/** Compara ignorando tildes y mayúsculas: se busca «logistica» y aparece «Logística». */
function normalizar(s: string): string {
  return s
    .toLowerCase()
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "");
}

export function ControlPage() {
  const { periodo } = usePeriodoActivo();
  const toast = useToast();

  const [hasta, setHasta] = useState(periodo);
  const [meses, setMeses] = useState(1);
  const [agruparPor, setAgruparPor] = useState<"departamento" | "sede">("departamento");
  const [umbral, setUmbral] = useState("90");
  const [abierta, setAbierta] = useState<string | null>(null);
  // Filtros de la tabla principal.
  const [busca, setBusca] = useState("");
  const [estado, setEstado] = useState<string>("");

  const desde = useMemo(() => restarMeses(hasta, meses - 1), [hasta, meses]);
  const q = useControl(desde, hasta, agruparPor, umbral);
  const deptos = useDepartamentosDeBancos();
  const sedes = useSedes();
  const data = q.data;

  // Las partidas de la fila abierta. `abierta === ""` es la fila «sin asignar», que es donde se
  // arranca a atribuir: por eso se pide igual que cualquier otra.
  const detalle = usePartidasDeDimension(desde, hasta, agruparPor, abierta ?? "", abierta !== null, umbral);

  // Blindaje: un arreglo vacío puede llegar como null desde el backend y `null.map()` rompe la
  // pantalla entera. El servidor ya manda `[]`, pero una pantalla no se cae por eso.
  const filas = data?.filas ?? [];
  const mesesSalud = data?.meses ?? [];

  const conPresupuesto = useMemo(() => filas.filter((f) => f.presupuesto !== "").length, [filas]);

  // El filtrado es del CLIENTE: son 14 departamentos, no 14.000, y filtrar en el servidor obligaría
  // a recargar el cruce con el presupuesto en cada tecla.
  const filasVisibles = useMemo(() => {
    const t = normalizar(busca.trim());
    return filas.filter(
      (f) =>
        (t === "" || normalizar(f.departamento).includes(t)) && (estado === "" || f.estado === estado),
    );
  }, [filas, busca, estado]);

  const filtroActivo = busca.trim() !== "" || estado !== "";

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Control por departamento"
        description="Quién gastó, dónde, y cuánto le queda de su presupuesto."
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
            <PeriodoSelector label="Hasta" value={hasta} onChange={setHasta} id="control-hasta" />
          </div>
        }
      />

      {q.isLoading && <LoadingState label="Cruzando el gasto con el presupuesto…" />}
      {q.isError && <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />}

      {data && (
        <>
          {data.aviso && (
            <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
              <p className="text-sm font-medium text-content">Ojo con estos números</p>
              <p className="mt-0.5 text-sm text-content-muted">{data.aviso}</p>
            </div>
          )}

          {/* Semáforo de datos: el mismo criterio del análisis de partidas. */}
          <Card>
            <CardHeader>
              <CardTitle>
                Datos del rango · {etiquetaPeriodo(desde)} a {etiquetaPeriodo(hasta)}
              </CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-2">
              <p className="text-sm text-content-muted">
                Un mes a medio clasificar muestra menos gasto del que tuvo, así que el presupuesto
                parece menos consumido de lo que está.
              </p>
              <div className="flex flex-wrap gap-2">
                {mesesSalud.map((m) => (
                  <div
                    key={m.periodo}
                    className={cn(
                      "flex min-w-32 flex-col gap-0.5 rounded-lg border px-3 py-2",
                      m.comparable
                        ? "border-positivo/40 bg-positivo/5"
                        : m.movs === 0
                          ? "border-border bg-surface-muted"
                          : "border-pendiente/50 bg-pendiente/10",
                    )}
                  >
                    <span className="text-sm font-medium text-content">
                      {etiquetaPeriodo(m.periodo)}
                    </span>
                    <span className="text-xs tabular-nums text-content-muted">
                      {m.movs === 0
                        ? "sin movimientos"
                        : `${m.movs.toLocaleString("es-CR")} movs · ${toNumber(m.pct_clasificado).toFixed(1)} % clasificado`}
                    </span>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          {/*
            Filtros.

            Alineados por ARRIBA (`items-start`) y con todos los labels de UNA línea: alinear por
            abajo hacía que un texto de ayuda de dos líneas empujara su campo hacia arriba y la
            barra quedara escalonada. Por lo mismo el grupo de botones lleva su propio label
            («Agrupar por»): sin él, era el único control sin encabezado y no calzaba con los otros.
            Los textos de ayuda que repetían lo obvio («por nombre», con el placeholder al lado) se
            fueron: la unidad del umbral vive en el label y el «%» se dibuja dentro del campo.
          */}
          <Card>
            <CardContent className="flex flex-wrap items-start gap-4 pt-6">
              <div className="flex flex-col gap-1.5">
                <span className="text-sm font-medium text-content">Agrupar por</span>
                <div className="flex h-10 items-stretch gap-1">
                  {(
                    [
                      { id: "departamento", label: "Departamento" },
                      { id: "sede", label: "Sede" },
                    ] as const
                  ).map((o) => (
                    <button
                      key={o.id}
                      type="button"
                      onClick={() => {
                        setAgruparPor(o.id);
                        setAbierta(null);
                      }}
                      className={cn(
                        "rounded-lg border px-3 text-sm font-medium shadow-sm transition-colors",
                        agruparPor === o.id
                          ? "border-accent bg-accent text-accent-fg"
                          : "border-border bg-surface-raised text-content-muted hover:text-content",
                      )}
                    >
                      {o.label}
                    </button>
                  ))}
                </div>
              </div>
              <div className="w-56">
                <Input
                  label={agruparPor === "departamento" ? "Buscar departamento" : "Buscar sede"}
                  value={busca}
                  onChange={(e) => setBusca(e.target.value)}
                  placeholder={agruparPor === "departamento" ? "Logística" : "Cartago"}
                />
              </div>
              <div className="w-48">
                <Select
                  label="Estado"
                  value={estado}
                  onChange={(e) => setEstado(e.target.value)}
                  options={FILTROS_ESTADO.map((f) => ({ value: f.id, label: f.label }))}
                />
              </div>
              <div className="w-40">
                <div className="relative">
                  <Input
                    label="Avisar al consumir"
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
            </CardContent>
          </Card>

          <div className="grid gap-4 sm:grid-cols-3">
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Presupuestado</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {conPresupuesto === 0 ? "—" : formatMoneda(data.total_presupuesto)}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  {conPresupuesto === 0
                    ? "todavía no se cargó ningún presupuesto"
                    : `${conPresupuesto} departamento(s) con monto definido`}
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Gasto del rango</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {formatMoneda(data.total_gasto)}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  todo el gasto, con dueño y sin dueño
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Sin asignar</p>
                <p
                  className={cn(
                    "mt-1 text-2xl font-semibold tabular-nums",
                    toNumber(data.sin_asignar) > 0 ? "text-negativo" : "text-content",
                  )}
                >
                  {formatMoneda(data.sin_asignar)}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  {data.sin_asignar_movs.toLocaleString("es-CR")} movimiento(s) que no se le cobran a
                  nadie
                </p>
              </CardContent>
            </Card>
          </div>

          {/* La tabla */}
          <Card>
            <CardHeader>
              <CardTitle>
                {agruparPor === "departamento" ? "Departamentos" : "Sedes"} · gasto contra
                presupuesto
              </CardTitle>
              {/* El «limpiar» vive acá y no en la barra de filtros: junto al aviso de que hay un
                  filtro puesto, y sin empujar los cuatro campos a una segunda línea. */}
              {filtroActivo && (
                <p className="mt-1 flex flex-wrap items-center gap-2 text-xs text-content-muted">
                  <span>
                    Mostrando {filasVisibles.length} de {filas.length}. Los totales de arriba son
                    del rango completo, no de lo filtrado.
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      setBusca("");
                      setEstado("");
                    }}
                    className="font-medium text-accent underline"
                  >
                    Quitar el filtro
                  </button>
                </p>
              )}
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              {agruparPor === "sede" && (
                <p className="text-sm text-content-muted">
                  El presupuesto se define por <strong>departamento</strong>, no por sede: acá se ve
                  el gasto real de cada lugar, sin comparación.
                </p>
              )}
              {filas.length === 0 && toNumber(data.sin_asignar) === 0 ? (
                <EmptyState message="No hay gasto en el rango elegido." />
              ) : filasVisibles.length === 0 && filtroActivo ? (
                <EmptyState message="Ningún departamento calza con el filtro." />
              ) : (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>{agruparPor === "departamento" ? "Departamento" : "Sede"}</TH>
                        <TH className="text-right">Movs</TH>
                        <TH className="text-right">Presupuesto</TH>
                        <TH className="text-right">Gasto</TH>
                        <TH className="text-right">Disponible</TH>
                        <TH className="text-right">Consumido</TH>
                        <TH>Estado</TH>
                        {agruparPor === "departamento" && <TH>Desglose por partida</TH>}
                      </TR>
                    </THead>
                    <TBody>
                      {filasVisibles.map((f) => (
                        <TR key={f.departamento_id}>
                          <TD>
                            <button
                              type="button"
                              onClick={() =>
                                setAbierta(abierta === f.departamento_id ? null : f.departamento_id)
                              }
                              className="text-left font-medium text-accent underline"
                            >
                              {f.departamento}
                            </button>
                          </TD>
                          <TD className="text-right tabular-nums text-content-muted">{f.movs}</TD>
                          <TD className="text-right tabular-nums">
                            {f.presupuesto === "" ? "—" : formatMonto(f.presupuesto)}
                          </TD>
                          <TD className="text-right font-medium tabular-nums">
                            {formatMonto(f.gasto)}
                          </TD>
                          <TD
                            className={cn(
                              "text-right tabular-nums",
                              f.disponible !== "" && toNumber(f.disponible) < 0
                                ? "font-medium text-negativo"
                                : "",
                            )}
                          >
                            {f.disponible === "" ? "—" : formatMonto(f.disponible)}
                          </TD>
                          <TD className="text-right tabular-nums">
                            {f.consumido_pct === "" ? "—" : `${f.consumido_pct} %`}
                          </TD>
                          <TD>
                            <Badge tone={TONO_ESTADO[f.estado] ?? "neutral"}>
                              {ETIQUETA_ESTADO[f.estado] ?? f.estado}
                            </Badge>
                          </TD>
                          {agruparPor === "departamento" && (
                            <TD>
                              <CeldaDesglose fila={f} />
                            </TD>
                          )}
                        </TR>
                      ))}
                      {toNumber(data.sin_asignar) > 0 && !filtroActivo && (
                        <TR className="bg-negativo/5">
                          <TD>
                            <button
                              type="button"
                              onClick={() => setAbierta(abierta === "" ? null : "")}
                              className="text-left font-medium text-accent underline"
                            >
                              (sin asignar)
                            </button>
                            <span className="block text-xs text-content-muted">
                              acá se empieza: abrilo y asigná el departamento de cada partida
                            </span>
                          </TD>
                          <TD className="text-right tabular-nums text-content-muted">
                            {data.sin_asignar_movs}
                          </TD>
                          <TD className="text-right">—</TD>
                          <TD className="text-right font-medium tabular-nums text-negativo">
                            {formatMonto(data.sin_asignar)}
                          </TD>
                          <TD className="text-right">—</TD>
                          <TD className="text-right">—</TD>
                          <TD>
                            <Badge tone="negativo">falta atribuir</Badge>
                          </TD>
                          {agruparPor === "departamento" && <TD>—</TD>}
                        </TR>
                      )}
                    </TBody>
                  </Table>
                </TableContainer>
              )}
            </CardContent>
          </Card>

          {/* El detalle de la fila abierta: en qué partidas, y el selector que atribuye */}
          {abierta !== null && (
            <DetallePartidas
              esSinAsignar={abierta === ""}
              titulo={
                abierta === ""
                  ? "Gasto sin departamento: en qué partidas está"
                  : `En qué gastó ${filas.find((f) => f.departamento_id === abierta)?.departamento ?? ""}`
              }
              departamentoID={abierta}
              periodoParaSubpresupuesto={hasta}
              partidas={detalle.data}
              cargando={detalle.isLoading}
              error={detalle.isError ? mensajeError(detalle.error) : ""}
              onReintentar={() => detalle.refetch()}
              departamentos={deptos.data ?? []}
              sedes={sedes.data ?? []}
              onHecho={(m) => toast.success(m)}
              onError={(m) => toast.error(m)}
            />
          )}

          <PanelPresupuesto desde={desde} hasta={hasta} />
          <AdminDepartamentos />
        </>
      )}
    </div>
  );
}

/**
 * La celda que muestra si el presupuesto del departamento está repartido por partida.
 *
 * `sin_repartir` negativo es lo que hay que ver: el desglose asigna más de lo que el departamento
 * tiene autorizado, y eso NO lo detecta el semáforo del consumo (el gasto todavía no llegó).
 */
function CeldaDesglose({ fila }: { fila: FilaControl }) {
  if (fila.subpresupuestos === 0) {
    return (
      <span className="text-xs text-content-muted">
        {fila.presupuesto === "" ? "—" : "sin desglosar"}
      </span>
    );
  }
  const sobra = toNumber(fila.sin_repartir);
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-xs text-content">
        {fila.subpresupuestos} partida(s) · {formatMonto(fila.suma_subpresupuestos)}
      </span>
      <span className={cn("text-xs tabular-nums", sobra < 0 ? "font-medium text-negativo" : "text-content-muted")}>
        {sobra < 0
          ? `reparte ${formatMonto(String(Math.abs(sobra)))} de más`
          : `queda ${formatMonto(fila.sin_repartir)} sin repartir`}
      </span>
    </div>
  );
}

/** El detalle de una fila: sus partidas, con filtro propio y el subpresupuesto de cada una. */
function DetallePartidas({
  esSinAsignar,
  titulo,
  departamentoID,
  periodoParaSubpresupuesto,
  partidas,
  cargando,
  error,
  onReintentar,
  departamentos,
  sedes,
  onHecho,
  onError,
}: {
  esSinAsignar: boolean;
  titulo: string;
  departamentoID: string;
  periodoParaSubpresupuesto: string;
  partidas: GastoPartidaDimension[] | undefined;
  cargando: boolean;
  error: string;
  onReintentar: () => void;
  departamentos: { id: string; nombre: string }[];
  sedes: { id: string; nombre: string }[];
  onHecho: (msg: string) => void;
  onError: (msg: string) => void;
}) {
  const [busca, setBusca] = useState("");
  const [estado, setEstado] = useState("");

  const lista = partidas ?? [];
  const visibles = useMemo(() => {
    const t = normalizar(busca.trim());
    return lista.filter(
      (p) =>
        (t === "" ||
          normalizar(p.clasificacion).includes(t) ||
          normalizar(p.concepto).includes(t)) &&
        (estado === "" || p.estado === estado),
    );
  }, [lista, busca, estado]);

  const filtroActivo = busca.trim() !== "" || estado !== "";

  return (
    <Card>
      <CardHeader>
        <CardTitle>{titulo}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="mb-3 text-sm text-content-muted">
          {esSinAsignar ? (
            <>
              Elegí el departamento (y la sede) de cada partida:{" "}
              <strong>queda atribuida toda su historia y todo su futuro</strong>, sin reclasificar ni
              volver a importar nada. Para la excepción de un movimiento puntual se usa la pantalla
              de{" "}
              <Link to="/clasificar" className="font-medium text-accent underline">
                Clasificar
              </Link>
              .
            </>
          ) : (
            "«De dónde salió» dice si la atribución la escribió alguien en el movimiento, si la heredó de la factura de CxP o si viene del default de la partida — que es donde hay que corregirla. En «Subpresupuesto» se le fija a cada partida cuánto puede gastar."
          )}
        </p>

        {lista.length > 8 && (
          <div className="mb-3 flex flex-wrap items-start gap-3">
            <div className="w-56">
              <Input
                label="Buscar partida"
                value={busca}
                onChange={(e) => setBusca(e.target.value)}
                placeholder="Combustible"
              />
            </div>
            {!esSinAsignar && (
              <div className="w-48">
                <Select
                  label="Estado"
                  value={estado}
                  onChange={(e) => setEstado(e.target.value)}
                  options={FILTROS_ESTADO.map((f) => ({ value: f.id, label: f.label }))}
                />
              </div>
            )}
            {/* El conteo se alinea con los campos por un label invisible: sin él quedaba pegado al
                borde de arriba, que es lo que hacía ver la barra torcida. El «quitar» va como enlace
                y no como botón para que no empuje los campos a una segunda línea. */}
            <div className="flex flex-col gap-1.5">
              <span className="text-sm font-medium text-transparent" aria-hidden="true">
                &nbsp;
              </span>
              <div className="flex h-10 items-center gap-2 text-sm text-content-muted">
                <span>
                  {filtroActivo
                    ? `${visibles.length} de ${lista.length}`
                    : `${lista.length} partidas`}
                </span>
                {filtroActivo && (
                  <button
                    type="button"
                    onClick={() => {
                      setBusca("");
                      setEstado("");
                    }}
                    className="font-medium text-accent underline"
                  >
                    quitar
                  </button>
                )}
              </div>
            </div>
          </div>
        )}

        {cargando && <LoadingState label="Buscando las partidas…" />}
        {error !== "" && <ErrorState message={error} onRetry={onReintentar} />}
        {partidas && lista.length === 0 && (
          <EmptyState message="No hay gasto en esta agrupación durante el rango." />
        )}
        {partidas && lista.length > 0 && visibles.length === 0 && (
          <EmptyState message="Ninguna partida calza con el filtro." />
        )}
        {visibles.length > 0 && (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Partida</TH>
                  <TH className="text-right">Movs</TH>
                  <TH className="text-right">Gasto</TH>
                  {esSinAsignar ? (
                    <TH>Asignar a</TH>
                  ) : (
                    <>
                      <TH className="text-right">Subpresupuesto</TH>
                      <TH className="text-right">Disponible</TH>
                      <TH>Estado</TH>
                      <TH>De dónde salió</TH>
                    </>
                  )}
                </TR>
              </THead>
              <TBody>
                {visibles.map((p) => (
                  <TR key={`${p.clasificacion_id}-${p.concepto}-${p.clasificacion}-${p.origen}`}>
                    <TD>
                      <span className="font-medium text-content">{p.clasificacion}</span>
                      <span className="block text-xs text-content-muted">{p.concepto}</span>
                    </TD>
                    <TD className="text-right tabular-nums">{p.movs}</TD>
                    <TD className="text-right font-medium tabular-nums">{formatMonto(p.gasto)}</TD>
                    {esSinAsignar ? (
                      <TD>
                        <AsignarPartida
                          clasificacionID={p.clasificacion_id}
                          clasificacion={p.clasificacion}
                          departamentos={departamentos}
                          sedes={sedes}
                          departamentoActual={p.partida_departamento_id}
                          sedeActual={p.partida_sede_id}
                          origen={p.origen}
                          origenLegible={p.origen_legible}
                          movs={p.movs}
                          abiertoDeEntrada
                          onHecho={onHecho}
                          onError={onError}
                        />
                      </TD>
                    ) : (
                      <>
                        <TD className="text-right">
                          <SubpresupuestoDePartida
                            departamentoID={departamentoID}
                            partida={p}
                            periodo={periodoParaSubpresupuesto}
                            onHecho={onHecho}
                            onError={onError}
                          />
                        </TD>
                        <TD
                          className={cn(
                            "text-right tabular-nums",
                            p.disponible !== "" && toNumber(p.disponible) < 0
                              ? "font-medium text-negativo"
                              : "",
                          )}
                        >
                          {p.disponible === "" ? "—" : formatMonto(p.disponible)}
                        </TD>
                        <TD>
                          <Badge tone={TONO_ESTADO[p.estado] ?? "neutral"}>
                            {ETIQUETA_ESTADO[p.estado] ?? p.estado}
                          </Badge>
                        </TD>
                        <TD>
                          {/* La atribución tiene que poder CORREGIRSE acá, no solo fijarse una
                              vez. La operación cambia —un rubro pasa de un área a otra— y hasta
                              ahora el selector solo existía en «sin asignar», así que una partida
                              ya atribuida quedaba congelada aunque el endpoint siempre permitió
                              cambiarla. */}
                          <AsignarPartida
                            clasificacionID={p.clasificacion_id}
                            clasificacion={p.clasificacion}
                            departamentos={departamentos}
                            sedes={sedes}
                            departamentoActual={p.partida_departamento_id}
                            sedeActual={p.partida_sede_id}
                            origen={p.origen}
                            origenLegible={p.origen_legible}
                            movs={p.movs}
                            onHecho={onHecho}
                            onError={onError}
                          />
                        </TD>
                      </>
                    )}
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * El monto que esta partida puede gastar dentro del departamento.
 *
 * Se fija en el MES elegido arriba («Hasta»), no en todo el rango: un presupuesto es de un mes, y
 * repartir un rango de seis meses en una sola línea no diría en cuál de los seis se puede gastar.
 * El monto que se muestra sí es el del rango completo, porque es contra eso que se compara el gasto.
 */
function SubpresupuestoDePartida({
  departamentoID,
  partida,
  periodo,
  onHecho,
  onError,
}: {
  departamentoID: string;
  partida: GastoPartidaDimension;
  periodo: string;
  onHecho: (msg: string) => void;
  onError: (msg: string) => void;
}) {
  const guardar = useGuardarPresupuesto();
  const borrar = useBorrarPresupuesto();
  const [editando, setEditando] = useState(false);
  const [monto, setMonto] = useState("");

  // Sin partida (movimientos sin clasificar) no hay a qué ponerle un subpresupuesto.
  if (partida.clasificacion_id === "") {
    return <span className="text-xs text-content-muted">hay que clasificarlo primero</span>;
  }

  if (!editando) {
    return (
      <button
        type="button"
        onClick={() => {
          setMonto(partida.subpresupuesto === "" ? "" : partida.subpresupuesto);
          setEditando(true);
        }}
        className={cn(
          "tabular-nums underline",
          partida.subpresupuesto === "" ? "text-xs text-content-muted" : "font-medium text-accent",
        )}
      >
        {partida.subpresupuesto === "" ? "fijar" : formatMonto(partida.subpresupuesto)}
      </button>
    );
  }

  function enviar() {
    if (monto.trim() === "") {
      onError("Escribí el monto o quitá el subpresupuesto.");
      return;
    }
    guardar.mutate(
      {
        departamento_id: departamentoID,
        clasificacion_id: partida.clasificacion_id,
        periodo,
        monto: montoParaApi(monto),
      },
      {
        onSuccess: () => {
          onHecho(`«${partida.clasificacion}» quedó con su tope en ${etiquetaPeriodo(periodo)}.`);
          setEditando(false);
        },
        onError: (err) => onError(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex items-center justify-end gap-1">
      <div className="w-28">
        <Input
          aria-label={`Subpresupuesto de ${partida.clasificacion}`}
          value={monto}
          onChange={(e) => setMonto(e.target.value)}
          inputMode="decimal"
          placeholder="500 000"
        />
      </div>
      <Button size="sm" onClick={enviar} loading={guardar.isPending}>
        OK
      </Button>
      {partida.subpresupuesto !== "" && (
        <Button
          size="sm"
          variant="ghost"
          loading={borrar.isPending}
          onClick={() =>
            borrar.mutate(
              {
                departamento_id: departamentoID,
                periodo,
                clasificacion_id: partida.clasificacion_id,
              },
              {
                onSuccess: () => {
                  onHecho(`«${partida.clasificacion}» quedó sin tope propio.`);
                  setEditando(false);
                },
                onError: (err) => onError(mensajeError(err)),
              },
            )
          }
        >
          Quitar
        </Button>
      )}
      <Button size="sm" variant="ghost" onClick={() => setEditando(false)}>
        Cancelar
      </Button>
    </div>
  );
}

/**
 * El selector que atribuye una partida.
 *
 * El id de la clasificación viene ahora en la respuesta del control. Antes se resolvía por
 * (concepto, nombre) contra el catálogo completo, que además fallaba con dos partidas homónimas en
 * conceptos distintos.
 */
function AsignarPartida({
  clasificacionID,
  clasificacion,
  departamentos,
  sedes,
  departamentoActual,
  sedeActual,
  origen,
  origenLegible,
  movs,
  abiertoDeEntrada = false,
  onHecho,
  onError,
}: {
  clasificacionID: string;
  clasificacion: string;
  departamentos: { id: string; nombre: string }[];
  sedes: { id: string; nombre: string }[];
  /** El default que HOY tiene la partida. Hace falta para precargar y para no borrar nada. */
  departamentoActual: string;
  sedeActual: string;
  origen: OrigenDimension;
  origenLegible: string;
  movs: number;
  /** En «sin asignar» el selector se muestra directo; en las demás tablas hay que pedirlo. */
  abiertoDeEntrada?: boolean;
  onHecho: (msg: string) => void;
  onError: (msg: string) => void;
}) {
  const asignar = useAsignarDimensionesPartida();
  const [editando, setEditando] = useState(abiertoDeEntrada);
  // Se precargan los DOS. El endpoint escribe `departamento_id` y `sede_id` juntos, así que
  // guardar con la sede vacía se la BORRARÍA a la partida sin que nadie lo haya pedido.
  const [deptoID, setDeptoID] = useState(departamentoActual);
  const [sedeID, setSedeID] = useState(sedeActual);

  // Sin partida no hay nada que atribuir: pasa con «(sin clasificar)», que es un movimiento sin
  // partida todavía. Ahí el trabajo es clasificarlo, no atribuirlo.
  if (clasificacionID === "") {
    return (
      <span className="text-xs text-content-muted">
        primero hay que clasificar este movimiento
      </span>
    );
  }

  if (!editando) {
    return (
      <div className="flex items-center gap-2">
        <span className="text-xs text-content-muted">{origenLegible}</span>
        <button
          type="button"
          className="text-xs text-accent underline"
          onClick={() => {
            setDeptoID(departamentoActual);
            setSedeID(sedeActual);
            setEditando(true);
          }}
        >
          cambiar
        </button>
      </div>
    );
  }

  function guardar(vaciar = false) {
    const depto = vaciar ? "" : deptoID;
    const sede = vaciar ? "" : sedeID;
    if (!vaciar && !depto && !sede) {
      onError("Elegí un departamento o una sede. Para dejarla sin atribuir, usá «Quitar».");
      return;
    }
    asignar.mutate(
      { clasificacionId: clasificacionID, departamento_id: depto, sede_id: sede },
      {
        onSuccess: (r) => {
          const n = r.movimientos_afectados.toLocaleString("es-CR");
          onHecho(
            vaciar
              ? `«${clasificacion}» quedó sin atribuir: ${n} movimiento(s) vuelven a «sin asignar».`
              : `«${clasificacion}» quedó atribuida: ${n} movimiento(s), incluida su historia.`,
          );
          if (!abiertoDeEntrada) setEditando(false);
        },
        onError: (err) => onError(mensajeError(err)),
      },
    );
  }

  const yaAtribuida = departamentoActual !== "" || sedeActual !== "";

  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-wrap items-end gap-2">
        <Select
          aria-label={`Departamento de ${clasificacion}`}
          value={deptoID}
          onChange={(e) => setDeptoID(e.target.value)}
          options={[
            { value: "", label: "— departamento —" },
            ...departamentos.map((d) => ({ value: d.id, label: d.nombre })),
          ]}
          className="min-w-40"
        />
        <Select
          aria-label={`Sede de ${clasificacion}`}
          value={sedeID}
          onChange={(e) => setSedeID(e.target.value)}
          options={[
            { value: "", label: "— sede —" },
            ...sedes.map((s) => ({ value: s.id, label: s.nombre })),
          ]}
          className="min-w-36"
        />
        <Button size="sm" onClick={() => guardar()} loading={asignar.isPending}>
          {yaAtribuida ? "Cambiar" : "Asignar"}
        </Button>
        {yaAtribuida && (
          <Button size="sm" variant="ghost" onClick={() => guardar(true)} loading={asignar.isPending}>
            Quitar
          </Button>
        )}
        {!abiertoDeEntrada && (
          <Button size="sm" variant="ghost" onClick={() => setEditando(false)}>
            Cancelar
          </Button>
        )}
      </div>

      {/* Cambiar el default de la PARTIDA no mueve los movimientos que tienen la atribución
          escrita encima. Decirlo acá evita el peor final: cambiar, ver que la fila sigue igual, y
          concluir que el sistema no guardó. */}
      {(origen === "MOVIMIENTO" || origen === "FACTURA") && (
        <span className="text-xs text-pendiente">
          {origen === "MOVIMIENTO"
            ? `Ojo: la atribución de estos ${movs} movimiento(s) está escrita en el movimiento, así que cambiar el default de la partida no los va a mover. Eso se corrige en Clasificar.`
            : `Ojo: estos ${movs} movimiento(s) heredan la atribución de su factura de CxP, así que cambiar el default de la partida no los va a mover.`}
        </span>
      )}
    </div>
  );
}

/** El panel para cargar el presupuesto del mes: el total del departamento y su desglose. */
function PanelPresupuesto({ desde, hasta }: { desde: string; hasta: string }) {
  const toast = useToast();
  const deptos = useDepartamentosDeBancos();
  const lineas = usePresupuesto(desde, hasta);
  const guardar = useGuardarPresupuesto();
  const borrar = useBorrarPresupuesto();

  const [deptoID, setDeptoID] = useState("");
  const [periodo, setPeriodo] = useState(hasta);
  const [monto, setMonto] = useState("");
  const [soloTotales, setSoloTotales] = useState(false);

  function enviar() {
    if (!deptoID || monto.trim() === "") {
      toast.error("Elegí el departamento y escribí el monto.");
      return;
    }
    guardar.mutate(
      { departamento_id: deptoID, periodo, monto: montoParaApi(monto) },
      {
        onSuccess: () => {
          toast.success("Presupuesto guardado.");
          setMonto("");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  const todas = lineas.data ?? [];
  const visibles = soloTotales ? todas.filter((l) => l.clasificacion_id === "") : todas;
  const cuantosSub = todas.filter((l) => l.clasificacion_id !== "").length;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Presupuesto por departamento</CardTitle>
        <p className="mt-1 text-xs text-content-muted">
          Un monto por departamento y por mes. El presupuesto no se descuenta ni se consume: se
          compara contra el gasto real que sale del banco. El <strong>desglose por partida</strong> se
          carga desde el detalle de cada departamento, arriba.
        </p>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-2">
          <Select
            label="Departamento"
            value={deptoID}
            onChange={(e) => setDeptoID(e.target.value)}
            options={[
              { value: "", label: "— elegir —" },
              ...(deptos.data ?? []).map((d) => ({ value: d.id, label: d.nombre })),
            ]}
            className="min-w-48"
          />
          <PeriodoSelector label="Mes" value={periodo} onChange={setPeriodo} id="presu-periodo" />
          <div className="w-40">
            <Input
              label="Monto del mes"
              value={monto}
              onChange={(e) => setMonto(e.target.value)}
              inputMode="decimal"
              placeholder="2 000 000"
            />
          </div>
          <Button onClick={enviar} loading={guardar.isPending}>
            Guardar
          </Button>
        </div>

        {cuantosSub > 0 && (
          <label className="flex w-fit items-center gap-2 text-sm text-content-muted">
            <input
              type="checkbox"
              checked={soloTotales}
              onChange={(e) => setSoloTotales(e.target.checked)}
              className="h-4 w-4 rounded border-border"
            />
            Ver solo los totales por departamento (esconder los {cuantosSub} desglose(s) por partida)
          </label>
        )}

        {visibles.length > 0 && (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Departamento</TH>
                  <TH>Partida</TH>
                  <TH>Mes</TH>
                  <TH className="text-right">Monto</TH>
                  <TH className="text-right">Quitar</TH>
                </TR>
              </THead>
              <TBody>
                {visibles.map((l) => (
                  <TR
                    key={`${l.departamento_id}-${l.periodo}-${l.clasificacion_id}`}
                    className={l.clasificacion_id !== "" ? "bg-surface-muted/50" : ""}
                  >
                    <TD className="font-medium text-content">{l.departamento}</TD>
                    <TD className="text-content-muted">
                      {l.clasificacion_id === "" ? (
                        <span className="text-xs uppercase tracking-wide">total del depto.</span>
                      ) : (
                        <>
                          {l.clasificacion}
                          <span className="block text-xs">{l.concepto}</span>
                        </>
                      )}
                    </TD>
                    <TD>{etiquetaPeriodo(l.periodo)}</TD>
                    <TD className="text-right font-medium tabular-nums">{formatMonto(l.monto)}</TD>
                    <TD className="text-right">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() =>
                          borrar.mutate(
                            {
                              departamento_id: l.departamento_id,
                              periodo: l.periodo,
                              clasificacion_id: l.clasificacion_id,
                            },
                            {
                              onSuccess: () => toast.success("Línea quitada."),
                              onError: (err) => toast.error(mensajeError(err)),
                            },
                          )
                        }
                      >
                        Quitar
                      </Button>
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}
        {lineas.data && lineas.data.length === 0 && (
          <p className="text-sm text-content-muted">
            Todavía no hay presupuesto cargado para este rango.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
