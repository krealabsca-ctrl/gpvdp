/**
 * Bandeja de CLASIFICACIÓN bancaria — LA superficie de trabajo del módulo Bancos
 * (reemplaza Movimientos + Revisar; patrón Bandeja CxP, maqueta aprobada).
 *
 * Pestañas: Por clasificar / Traslados / Reglas / Clasificados.
 * Motor que aprende: al clasificar a mano, el sistema propone crear una regla con
 * la palabra clave del movimiento ("clasificaría N similares") — 1 clic y retro-aplica.
 * Regla del negocio (decisión DF): el motor asigna solo con exactitud ≥90%
 * (match exacto = 100); todo lo demás queda "Por clasificar". Sin nivel intermedio.
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
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
  type BadgeTone,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { etiquetaPeriodo, formatFecha, formatMonto, sinTildes, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import {
  bancosApi,
  type AgruparResumen,
  type AplicaA,
  type FiltrosMovimientos,
  type MovimientoRow,
  type Regla,
  type SugerenciaRegla,
  type Tipo,
} from "@/api/bancos";
import {
  useActualizarRegla,
  useBancosCatalogo,
  useClasificarMasivo,
  useClasificarMovimiento,
  useClasificaciones,
  useConceptos,
  useCrearRegla,
  useCuentas,
  useDesemparejarTraslado,
  useEliminarRegla,
  useEmparejarTraslado,
  useMovimientos,
  usePropuestasTraslado,
  usePatrones,
  useReglas,
  useResumenClasificacion,
} from "@/features/bancos/hooks";
import { ClasifCombobox, type ClasifElegida } from "@/features/bancos/components/ClasifCombobox";
import { PatronesTab } from "@/features/bancos/components/PatronesTab";
import { ResumenSeleccion } from "@/features/bancos/components/ResumenSeleccion";

type Tab = "pendientes" | "patrones" | "traslados" | "reglas" | "clasificados";

/** Lo que una pestaña le dice al encabezado sobre lo que está mostrando. */
interface VistaResumen {
  filtros: FiltrosMovimientos;
  agrupar: AgruparResumen;
  descripcion: string;
}

const PAGE_SIZE = 50;
const META_AUTO = 90; // % de auto-clasificación al que apunta el motor

export function ClasificarPage() {
  const { periodo } = usePeriodoActivo();
  const [tab, setTab] = useState<Tab>("pendientes");
  const [todosPeriodos, setTodosPeriodos] = useState(true);
  const periodoFiltro = todosPeriodos ? undefined : periodo;

  const resumenQ = useResumenClasificacion(periodoFiltro);
  const reglasQ = useReglas();
  const patronesQ = usePatrones(periodoFiltro);

  /**
   * Qué está mirando la pestaña activa, para que el resumen viva ARRIBA (jerarquía: primero
   * lo que estás trabajando, después el avance del motor).
   *
   * El estado de los filtros se queda en cada pestaña —es suya— y la pestaña montada
   * reporta acá una copia para pintar el encabezado. Al cambiar de pestaña se limpia,
   * porque Patrones y Reglas no trabajan sobre movimientos y un resumen viejo mentiría.
   */
  const [vista, setVista] = useState<VistaResumen | null>(null);
  function cambiarTab(t: Tab) {
    setVista(null);
    setTab(t);
  }

  // Banner de aprendizaje: propuesta de regla tras una clasificación manual.
  const [sugerencia, setSugerencia] = useState<SugerenciaRegla | null>(null);
  // Palabra clave para pre-llenar el formulario de Nueva regla (desde la búsqueda).
  const [reglaPrefill, setReglaPrefill] = useState<string | null>(null);

  function proponerRegla(palabra: string) {
    setReglaPrefill(palabra);
    setTab("reglas");
  }

  const r = resumenQ.data;
  const clasificados = (r?.auto ?? 0) + (r?.revisados ?? 0);
  const pctAuto = r && r.total > 0 ? Math.round((r.auto / r.total) * 100) : 0;
  const pctClasificado = r && r.total > 0 ? Math.round((clasificados / r.total) * 100) : 0;

  const tabs: { id: Tab; label: string; count?: number }[] = [
    { id: "pendientes", label: "Por clasificar", count: r?.no_identificados },
    // Patrones va segundo a propósito: es por donde conviene empezar cuando hay miles
    // pendientes (una regla resuelve el grupo entero).
    { id: "patrones", label: "Patrones", count: patronesQ.data?.length },
    { id: "traslados", label: "Traslados", count: r?.traslados },
    { id: "reglas", label: "Reglas", count: reglasQ.data?.length },
    { id: "clasificados", label: "Clasificados", count: clasificados },
  ];

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Clasificación bancaria"
        description="Clasificá una vez; el motor aprende y hace el resto. Meta: ≥90% automático."
      />

      {/* Lo que estás trabajando AHORA: manda sobre el avance global del motor. */}
      {vista && (
        <ResumenSeleccion
          filtros={vista.filtros}
          agrupar={vista.agrupar}
          descripcion={vista.descripcion}
        />
      )}

      {/* Avance del motor. Las tarjetas «Por clasificar» y «Clasificados» que estaban acá se
          quitaron: repetían el número del badge de su pestaña y el del resumen de arriba. */}
      <div className="grid grid-cols-1 gap-3">
        <div className="rounded-xl border border-border bg-surface-raised px-5 py-4 shadow-card">
          <div className="flex items-baseline justify-between">
            <p className="text-xs uppercase tracking-wide text-content-muted">Auto-clasificado por el motor</p>
            <p className="text-xs text-content-muted">
              meta <span className="font-semibold text-brand-gold">{META_AUTO}%</span>
            </p>
          </div>
          <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
            {pctAuto}%{" "}
            <span className="text-sm font-normal text-content-muted">
              · clasificado total {pctClasificado}%
            </span>
          </p>
          <div className="relative mt-2 h-2 overflow-hidden rounded-full bg-surface-muted">
            <div
              className="absolute inset-y-0 left-0 rounded-full bg-content-muted/40"
              style={{ width: `${pctClasificado}%` }}
              title={`Clasificado total ${pctClasificado}%`}
            />
            <div
              className="absolute inset-y-0 left-0 rounded-full bg-accent"
              style={{ width: `${pctAuto}%` }}
              title={`Auto ${pctAuto}%`}
            />
            <div
              className="absolute inset-y-0 w-0.5 bg-brand-gold"
              style={{ left: `${META_AUTO}%` }}
              title={`Meta ${META_AUTO}%`}
            />
          </div>
        </div>
      </div>

      {/* Banner del motor que aprende */}
      {sugerencia?.sugerible && (
        <LearnBanner
          sugerencia={sugerencia}
          onCerrar={() => setSugerencia(null)}
        />
      )}

      {/* Pestañas + alcance de período */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border">
        <div role="tablist" aria-label="Fases de clasificación" className="flex gap-1">
          {tabs.map((t) => (
            <button
              key={t.id}
              role="tab"
              aria-selected={tab === t.id}
              onClick={() => cambiarTab(t.id)}
              className={cn(
                "-mb-px flex items-center gap-1.5 border-b-2 px-4 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent",
                tab === t.id
                  ? "border-accent text-accent"
                  : "border-transparent text-content-muted hover:text-content",
              )}
            >
              {t.label}
              {t.count !== undefined && (
                <span
                  className={cn(
                    "rounded-full px-1.5 text-[11px] font-semibold tabular-nums",
                    tab === t.id ? "bg-accent/15 text-accent" : "bg-surface-muted text-content-muted",
                  )}
                >
                  {t.count}
                </span>
              )}
            </button>
          ))}
        </div>
        <label className="flex items-center gap-2 pb-1 text-xs text-content-muted">
          <input
            type="checkbox"
            checked={todosPeriodos}
            onChange={(e) => setTodosPeriodos(e.target.checked)}
            className="h-3.5 w-3.5 rounded border-border accent-accent"
          />
          Todos los períodos (histórico completo)
        </label>
      </div>

      {tab === "pendientes" && (
        <MovsTab
          estado="NO_IDENTIFICADO"
          periodo={periodoFiltro}
          onSugerencia={setSugerencia}
          onProponerRegla={proponerRegla}
          onVista={setVista}
        />
      )}
      {tab === "patrones" && <PatronesTab periodo={periodoFiltro} />}
      {tab === "traslados" && <TrasladosTab periodo={periodo} periodoLista={periodoFiltro} onVista={setVista} />}
      {tab === "reglas" && (
        <ReglasTab prefill={reglaPrefill} onPrefillUsado={() => setReglaPrefill(null)} />
      )}
      {tab === "clasificados" && (
        <MovsTab
          estado="CLASIFICADO"
          periodo={periodoFiltro}
          onSugerencia={setSugerencia}
          onProponerRegla={proponerRegla}
          onVista={setVista}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Banner del motor que aprende
// ---------------------------------------------------------------------------

function LearnBanner({ sugerencia, onCerrar }: { sugerencia: SugerenciaRegla; onCerrar: () => void }) {
  const toast = useToast();
  const crearRegla = useCrearRegla();

  function crear() {
    crearRegla.mutate(
      {
        nombre: sugerencia.nombre_sugerido,
        aplica_a: sugerencia.aplica_a,
        concepto_id: sugerencia.concepto_id,
        clasificacion_id: sugerencia.clasificacion_id,
        prioridad: 100,
        palabras_clave: [sugerencia.palabra_clave],
      },
      {
        onSuccess: (res) => {
          toast.success(
            res.clasificados > 0
              ? `Regla creada: ${res.clasificados} movimiento(s) similares se clasificaron solos.`
              : "Regla creada: los próximos movimientos con esa palabra se clasificarán solos.",
          );
          onCerrar();
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-3 rounded-xl border border-accent/40 bg-accent/5 px-4 py-3">
      <span aria-hidden className="text-xl">🧠</span>
      <p className="min-w-52 flex-1 text-sm text-content">
        ¿Crear la regla{" "}
        <strong className="text-accent">«{sugerencia.palabra_clave}»</strong> →{" "}
        <strong>{sugerencia.concepto} › {sugerencia.clasificacion}</strong>?{" "}
        {sugerencia.similares > 0 ? (
          <>
            Clasificaría <strong className="text-accent">{sugerencia.similares}</strong> movimiento(s)
            similares ahora mismo, y los futuros solos.
          </>
        ) : (
          <>Los próximos movimientos con esa palabra se clasificarán solos.</>
        )}
      </p>
      <div className="flex gap-2">
        <Button size="sm" onClick={crear} loading={crearRegla.isPending}>
          Crear regla y aplicar
        </Button>
        <Button size="sm" variant="ghost" onClick={onCerrar}>
          Ahora no
        </Button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pestañas "Por clasificar" y "Clasificados" (misma tabla, distinto estado)
// ---------------------------------------------------------------------------

function MovsTab({
  estado,
  periodo,
  onSugerencia,
  onProponerRegla,
  onVista,
}: {
  estado: "NO_IDENTIFICADO" | "CLASIFICADO";
  periodo: string | undefined;
  onSugerencia: (s: SugerenciaRegla | null) => void;
  onProponerRegla: (palabra: string) => void;
  /** Reporta al encabezado qué está mostrando esta pestaña (el resumen vive arriba). */
  onVista: (v: VistaResumen) => void;
}) {
  const toast = useToast();
  const clasificar = useClasificarMovimiento();
  const masivo = useClasificarMasivo();
  const conceptosQ = useConceptos();
  const clasifsQ = useClasificaciones();
  const cuentasQ = useCuentas();
  const bancosQ = useBancosCatalogo();

  const [q, setQ] = useState("");
  const [qDebounced, setQDebounced] = useState("");
  const [tipo, setTipo] = useState<"" | Tipo>("");
  const [desde, setDesde] = useState("");
  const [hasta, setHasta] = useState("");
  const [orden, setOrden] = useState("");
  const [conceptoId, setConceptoId] = useState("");
  const [clasificacionId, setClasificacionId] = useState("");
  const [bancoId, setBancoId] = useState("");
  const [cuentaId, setCuentaId] = useState("");
  const [page, setPage] = useState(1);
  const [seleccion, setSeleccion] = useState<Set<string>>(new Set());

  useEffect(() => {
    const t = setTimeout(() => setQDebounced(q.trim()), 300);
    return () => clearTimeout(t);
  }, [q]);
  useEffect(() => {
    setPage(1);
    setSeleccion(new Set());
  }, [qDebounced, tipo, desde, hasta, orden, conceptoId, clasificacionId, bancoId, cuentaId, periodo, estado]);

  // Si se cambia de banco, la cuenta elegida deja de pertenecerle: se limpia en vez de
  // devolver cero filas sin explicación.
  useEffect(() => {
    setCuentaId("");
  }, [bancoId]);

  // UN solo objeto de filtros para la lista y para su resumen: así el encabezado mide
  // exactamente las filas que se están viendo (page/page_size no aplican al resumen).
  const filtros = {
    estado_clasificacion: estado,
    ...(periodo ? { periodo } : {}),
    ...(qDebounced ? { q: qDebounced } : {}),
    ...(tipo ? { tipo } : {}),
    ...(desde ? { desde } : {}),
    ...(hasta ? { hasta } : {}),
    ...(orden ? { orden } : {}),
    ...(conceptoId ? { concepto_id: conceptoId } : {}),
    ...(clasificacionId ? { clasificacion_id: clasificacionId } : {}),
    ...(bancoId ? { banco_id: bancoId } : {}),
    ...(cuentaId ? { cuenta_bancaria_id: cuentaId } : {}),
  } satisfies FiltrosMovimientos;
  const movsQ = useMovimientos({ ...filtros, page, page_size: PAGE_SIZE });

  // Banco · Cuenta. El banco filtra su grupo completo; la cuenta afina dentro de él. Las
  // cuentas se acotan al banco elegido comparando por NOMBRE porque el listado de cuentas
  // expone el banco así (no su id); los dos nombres salen del mismo catálogo.
  const bancoElegido = (bancosQ.data ?? []).find((b) => b.id === bancoId);
  const cuentasVisibles = (cuentasQ.data ?? []).filter(
    (c) => !bancoElegido || c.banco === bancoElegido.nombre,
  );
  const bancoOptions = [
    { value: "", label: "Todos los bancos" },
    ...(bancosQ.data ?? []).map((b) => ({ value: b.id, label: b.nombre })),
  ];
  const cuentaOptions = [
    { value: "", label: bancoElegido ? `Todas las de ${bancoElegido.nombre}` : "Todas las cuentas" },
    ...cuentasVisibles.map((c) => ({ value: c.id, label: `${c.alias} · ${c.moneda}` })),
  ];

  const conceptoOptions = [
    { value: "", label: "Todos los conceptos" },
    ...(conceptosQ.data ?? []).map((c) => ({ value: c.id, label: c.nombre })),
  ];
  // El concepto va como GRUPO del desplegable, no dentro de la etiqueta. Con «Gastos › Gas» como
  // texto, buscar por teclado era imposible: las 112 clasificaciones de Gastos empiezan igual, así
  // que escribir «gas» saltaba a la primera de la lista y nunca a la clasificación «Gas».
  const clasifOptions = [
    { value: "", label: "Todas las clasificaciones" },
    ...(clasifsQ.data ?? [])
      .filter((c) => !conceptoId || c.concepto_id === conceptoId)
      .map((c) => ({
        value: c.id,
        label: c.nombre,
        // Con un concepto ya elegido el grupo sería siempre el mismo: sobra.
        ...(conceptoId ? {} : { grupo: c.concepto }),
      })),
  ];

  // Qué está midiendo el resumen, en palabras: el usuario ve «1398 movimientos» y necesita
  // saber de qué. Sale de los filtros activos, no de un texto fijo.
  const descripcionSeleccion = [
    conceptoId ? (conceptosQ.data ?? []).find((c) => c.id === conceptoId)?.nombre : null,
    clasificacionId ? (clasifsQ.data ?? []).find((c) => c.id === clasificacionId)?.nombre : null,
    tipo === "DEBITO" ? "solo débitos" : tipo === "CREDITO" ? "solo créditos" : null,
    qDebounced ? `«${qDebounced}»` : null,
  ]
    .filter(Boolean)
    .join(" · ") || (estado === "NO_IDENTIFICADO" ? "sin clasificar" : "ya clasificados");

  const agrupar: AgruparResumen = estado === "NO_IDENTIFICADO" ? "cuenta" : "concepto";
  const filtrosJSON = JSON.stringify(filtros);
  useEffect(() => {
    onVista({ filtros: JSON.parse(filtrosJSON) as FiltrosMovimientos, agrupar, descripcion: descripcionSeleccion });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- filtrosJSON representa a filtros
  }, [filtrosJSON, agrupar, descripcionSeleccion]);

  const items = movsQ.data?.items ?? [];
  const total = movsQ.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / (movsQ.data?.page_size ?? PAGE_SIZE)));

  /** Clasifica UNA fila y consulta al motor si hay regla que proponer (aprendizaje). */
  function clasificarFila(m: MovimientoRow, c: ClasifElegida) {
    clasificar.mutate(
      { movimientoId: m.id, conceptoId: c.conceptoId, clasificacionId: c.clasificacionId },
      {
        onSuccess: () => {
          toast.success(`Clasificado: ${c.ruta}`);
          onSugerencia(null);
          bancosApi
            .sugerenciaRegla(m.id)
            .then((s) => {
              if (s.sugerible) onSugerencia(s);
            })
            .catch(() => undefined); // la sugerencia es opcional; no bloquea el flujo
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function aplicarMasivo(c: ClasifElegida) {
    const ids = [...seleccion];
    masivo.mutate(
      { movimientoIds: ids, conceptoId: c.conceptoId, clasificacionId: c.clasificacionId },
      {
        onSuccess: (res) => {
          toast.success(`${res.clasificados} movimiento(s) clasificados como ${c.ruta}.`);
          setSeleccion(new Set());
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function toggleFila(id: string) {
    setSeleccion((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }
  const todasSel = items.length > 0 && seleccion.size === items.length;

  return (
    <div className="flex flex-col gap-3">
      {/* Filtros */}
      <div className="flex flex-wrap items-end gap-3">
        <Input
          label="Buscar"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Descripción o documento…"
          className="min-w-56"
        />
        <Select
          label="Tipo"
          value={tipo}
          onChange={(e) => setTipo(e.target.value as "" | Tipo)}
          options={[
            { value: "", label: "Débitos y créditos" },
            { value: "DEBITO", label: "Solo débitos" },
            { value: "CREDITO", label: "Solo créditos" },
          ]}
          className="min-w-40"
        />
        {/* Banco y cuenta: la tabla muestra «Banco · Cuenta» y antes no se podía filtrar por ahí,
            así que revisar una sola cuenta obligaba a leer las 15. */}
        <Select
          label="Banco"
          value={bancoId}
          onChange={(e) => setBancoId(e.target.value)}
          options={bancoOptions}
          className="min-w-40"
        />
        <Select
          label="Cuenta"
          value={cuentaId}
          onChange={(e) => setCuentaId(e.target.value)}
          options={cuentaOptions}
          className="min-w-48"
        />
        {estado === "CLASIFICADO" && (
          <>
            <Select
              label="Concepto"
              value={conceptoId}
              onChange={(e) => {
                setConceptoId(e.target.value);
                setClasificacionId("");
              }}
              options={conceptoOptions}
              className="min-w-44"
            />
            <Select
              label="Clasificación"
              value={clasificacionId}
              onChange={(e) => setClasificacionId(e.target.value)}
              options={clasifOptions}
              className="min-w-48"
            />
          </>
        )}
        <Input
          label="Desde"
          type="date"
          value={desde}
          onChange={(e) => setDesde(e.target.value)}
          className="w-36"
        />
        <Input
          label="Hasta"
          type="date"
          value={hasta}
          onChange={(e) => setHasta(e.target.value)}
          className="w-36"
        />
        <Select
          label="Ordenar por"
          value={orden}
          onChange={(e) => setOrden(e.target.value)}
          options={[
            { value: "", label: "Más recientes" },
            { value: "fecha_asc", label: "Más antiguos" },
            { value: "monto_desc", label: "Monto: mayor a menor" },
            { value: "monto_asc", label: "Monto: menor a mayor" },
          ]}
          className="min-w-44"
        />
        <div className="ml-auto flex items-end gap-3 pb-0.5">
          {qDebounced && estado === "NO_IDENTIFICADO" && (
            <Button
              size="sm"
              variant="secondary"
              onClick={() => onProponerRegla(qDebounced)}
              title={`Crear una regla del motor con la palabra clave «${qDebounced}»`}
            >
              ⚡ Regla con «{qDebounced}»
            </Button>
          )}
        </div>
      </div>


      {/* Los montos del encabezado están en COLONES. Un movimiento en dólares sin tipo de
          cambio entra al total como cero, así que la falta se dice en vez de callarse: si no,
          el total se queda corto y nadie sabe por qué. */}
      {(movsQ.data?.totales.sin_tipo_cambio ?? 0) > 0 && (
        <div className="rounded-xl border border-pendiente/40 bg-pendiente/5 px-4 py-2.5 text-sm">
          <b>
            {movsQ.data!.totales.sin_tipo_cambio} movimiento
            {movsQ.data!.totales.sin_tipo_cambio === 1 ? "" : "s"} en dólares sin tipo de cambio
          </b>{" "}
          (USD {formatMonto(movsQ.data!.totales.monto_sin_convertir)}). Se muestran en su moneda, pero{" "}
          <b>no suman a los totales en colones</b> hasta que se registre el tipo de cambio del mes en{" "}
          <Link to="/tipo-cambio" className="text-accent underline">
            Tipo de cambio
          </Link>
          .
        </div>
      )}

      {movsQ.isPending ? (
        <LoadingState label="Cargando movimientos" />
      ) : movsQ.isError ? (
        <ErrorState message={mensajeError(movsQ.error)} onRetry={() => movsQ.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          message={
            estado === "NO_IDENTIFICADO"
              ? "🎉 No hay movimientos por clasificar con estos filtros."
              : "Aún no hay movimientos clasificados con estos filtros."
          }
        />
      ) : (
        <>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH className="w-10">
                    <input
                      type="checkbox"
                      checked={todasSel}
                      onChange={() =>
                        setSeleccion(todasSel ? new Set() : new Set(items.map((m) => m.id)))
                      }
                      aria-label="Seleccionar todas"
                      className="h-4 w-4 rounded border-border accent-accent"
                    />
                  </TH>
                  <TH>Fecha</TH>
                  <TH>Banco · Cuenta</TH>
                  {/* El consecutivo del estado de cuenta: es lo único con lo que se puede
                      señalar una fila concreta ante el banco o ante una auditoría. */}
                  <TH>Documento</TH>
                  <TH>Descripción</TH>
                  <TH className="text-right">Débito</TH>
                  <TH className="text-right">Crédito</TH>
                  <TH>Clasificación</TH>
                  {estado === "CLASIFICADO" && <TH>Estado</TH>}
                </TR>
              </THead>
              <TBody>
                {items.map((m) => {
                  const esDebito = toNumber(m.debito) > 0;
                  const sel = seleccion.has(m.id);
                  const ruta = m.concepto && m.clasificacion ? `${m.concepto} › ${m.clasificacion}` : "";
                  return (
                    <TR key={m.id} className={sel ? "bg-accent/5" : undefined}>
                      <TD>
                        <input
                          type="checkbox"
                          checked={sel}
                          onChange={() => toggleFila(m.id)}
                          aria-label={`Seleccionar ${m.descripcion}`}
                          className="h-4 w-4 rounded border-border accent-accent"
                        />
                      </TD>
                      <TD className="whitespace-nowrap tabular-nums">{formatFecha(m.fecha)}</TD>
                      <TD className="max-w-[10rem]">
                        <div className="truncate" title={`${m.banco}${m.cuenta ? " · " + m.cuenta : ""}`}>
                          <span>{m.banco || "—"}</span>
                          {m.cuenta && (
                            <span className="block truncate text-xs text-content-muted">{m.cuenta}</span>
                          )}
                        </div>
                      </TD>
                      <TD className="max-w-[9rem] whitespace-nowrap font-mono text-[11px] text-content-muted">
                        <span className="block truncate" title={m.documento}>
                          {m.documento || "—"}
                        </span>
                      </TD>
                      <TD className="min-w-[20rem] max-w-2xl">
                        {/* Descripción COMPLETA y visible: es la clave para identificar el movimiento. */}
                        <span className="block whitespace-normal break-words text-[13px] leading-snug">
                          {m.descripcion}
                        </span>
                        {m.es_traslado && <Badge tone="accent">Traslado</Badge>}
                      </TD>
                      <TD className="text-right tabular-nums text-negativo">
                        {esDebito ? <Importe m={m} /> : "—"}
                      </TD>
                      <TD className="text-right tabular-nums text-positivo">
                        {!esDebito ? <Importe m={m} /> : "—"}
                      </TD>
                      <TD>
                        <ClasifCombobox
                          actual={ruta}
                          auto={m.estado_clasificacion === "AUTO"}
                          esDebito={esDebito}
                          onElegir={(c) => clasificarFila(m, c)}
                        />
                      </TD>
                      {estado === "CLASIFICADO" && (
                        <TD>
                          <Badge tone={m.estado_clasificacion === "AUTO" ? "accent" : "positivo"}>
                            {m.estado_clasificacion === "AUTO" ? "Auto" : "Revisado"}
                          </Badge>
                        </TD>
                      )}
                    </TR>
                  );
                })}
              </TBody>
            </Table>
          </TableContainer>

          {/* Paginación */}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
              Anterior
            </Button>
            <span className="text-sm tabular-nums text-content-muted">
              Página {page} de {totalPages}
            </span>
            <Button
              variant="secondary"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => p + 1)}
            >
              Siguiente
            </Button>
          </div>

          {/* Barra de clasificación masiva */}
          {seleccion.size > 0 && (() => {
            // Flujo del bloque: solo se infiere el concepto si TODOS los seleccionados
            // (y visibles) tienen el mismo signo. Si es mixto o hay seleccionados fuera de
            // la página, queda undefined y el combobox exigirá el concepto explícito.
            const selDocs = items.filter((m) => seleccion.has(m.id));
            const conocidos = selDocs.length === seleccion.size && selDocs.length > 0;
            const esDebitoMasivo = !conocidos
              ? undefined
              : selDocs.every((m) => toNumber(m.debito) > 0)
                ? true
                : selDocs.every((m) => toNumber(m.debito) === 0)
                  ? false
                  : undefined;
            return (
              <div className="sticky bottom-3 z-10 flex flex-wrap items-center gap-3 rounded-xl border border-accent/40 bg-surface-raised px-4 py-3 shadow-lifted">
                <p className="text-sm font-medium text-content">
                  {seleccion.size} seleccionado{seleccion.size === 1 ? "" : "s"}
                </p>
                <ClasifCombobox
                  actual=""
                  placeholder="Elegir clasificación para el bloque…"
                  esDebito={esDebitoMasivo}
                  onElegir={aplicarMasivo}
                  disabled={masivo.isPending}
                />
                <Button size="sm" variant="ghost" onClick={() => setSeleccion(new Set())}>
                  Limpiar selección
                </Button>
              </div>
            );
          })()}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pestaña Traslados: propuestas del motor + emparejados del período
// ---------------------------------------------------------------------------

/** Cómo se lee cada veredicto del motor de traslados (espejo de PuntuarTraslado). */
const ETIQUETA_VEREDICTO: Record<string, string> = {
  PROBABLE: "Probable",
  REVISAR: "Revisar",
  AMBIGUO: "Varias parejas",
};

const TONO_VEREDICTO: Record<string, BadgeTone> = {
  PROBABLE: "positivo",
  REVISAR: "pendiente",
  AMBIGUO: "negativo",
};

/**
 * El importe de un movimiento, dicho sin mentir.
 *
 * La tabla mostraba SIEMPRE `monto_crc`, y en las cuentas en dólares ese campo vale 0 hasta que
 * se registra el tipo de cambio del mes (la conversión es un paso aparte del importador). El
 * resultado era «0,00» en la columna de débito de un movimiento de USD 52,81: no es un dato
 * faltante, es un dato FALSO — se lee como «esta transacción fue de cero colones».
 *
 * Ahora manda la moneda de la cuenta: en dólares se muestra el monto en dólares, que es lo que
 * de verdad se movió, y los colones aparecen debajo solo si hay tipo de cambio. Si no lo hay, lo
 * dice.
 */
function Importe({ m }: { m: MovimientoRow }) {
  const original = toNumber(m.debito) > 0 ? m.debito : m.credito;
  if (m.moneda === "CRC") {
    return <>{formatMonto(m.monto_crc)}</>;
  }
  const crc = toNumber(m.monto_crc);
  return (
    <>
      <span className="whitespace-nowrap">$ {formatMonto(original)}</span>
      {crc > 0 ? (
        <span className="block whitespace-nowrap text-[11px] font-normal text-content-muted">
          ₡ {formatMonto(m.monto_crc)}
        </span>
      ) : (
        <span
          className="block whitespace-nowrap text-[11px] font-normal text-pendiente"
          title="La cuenta es en dólares y el mes no tiene tipo de cambio registrado. Registralo en Tipo de cambio para que el equivalente en colones se calcule."
        >
          sin tipo de cambio
        </span>
      )}
    </>
  );
}

function TrasladosTab({
  periodo,
  /**
   * Alcance de los traslados YA marcados: sigue la casilla «Todos los períodos», igual
   * que el contador de la pestaña. Las PROPUESTAS siguen siendo del período activo,
   * porque emparejar compara patas dentro del mismo mes.
   */
  periodoLista,
  onVista,
}: {
  periodo: string;
  periodoLista: string | undefined;
  /** Reporta al encabezado qué está mostrando esta pestaña (el resumen vive arriba). */
  onVista: (v: VistaResumen) => void;
}) {
  const toast = useToast();
  const propuestasQ = usePropuestasTraslado(periodo);
  const emparejar = useEmparejarTraslado();
  const desemparejar = useDesemparejarTraslado();
  const clasificar = useClasificarMovimiento();
  // Traslados ya marcados del período (para revisarlos/desemparejar). El mismo filtro
  // alimenta el resumen de arriba, con traslado: "si" para que el servidor sea el que
  // acota (antes se traían 500 y se filtraban acá a ojo).
  const filtrosTraslados = {
    ...(periodoLista ? { periodo: periodoLista } : {}),
    estado_clasificacion: "CLASIFICADO",
    traslado: "si",
  } satisfies FiltrosMovimientos;
  const trasladosQ = useMovimientos({ ...filtrosTraslados, page_size: 500 });
  const emparejados = trasladosQ.data?.items ?? [];
  // Filtros: con decenas de propuestas, buscar la cuenta o el monto a ojo no escala.
  const [veredicto, setVeredicto] = useState("");
  const [busqueda, setBusqueda] = useState("");

  const filtrosTrasJSON = JSON.stringify(filtrosTraslados);
  useEffect(() => {
    onVista({
      filtros: JSON.parse(filtrosTrasJSON) as FiltrosMovimientos,
      agrupar: "concepto",
      descripcion: "solo traslados emparejados",
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- filtrosTrasJSON representa al filtro
  }, [filtrosTrasJSON]);

  const todas = propuestasQ.data ?? [];
  const propuestas = todas.filter((p) => {
    if (veredicto && p.veredicto !== veredicto) return false;
    if (!busqueda.trim()) return true;
    const t = sinTildes(busqueda);
    return [p.cuenta_debito, p.cuenta_credito, p.descripcion_debito, p.descripcion_credito, p.monto_debito]
      .some((v) => sinTildes(String(v ?? "")).includes(t));
  });

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h3 className="text-sm font-semibold text-content">
          Propuestas de emparejamiento <span className="text-content-muted">({etiquetaPeriodo(periodo)})</span>
        </h3>
        <p className="text-xs text-content-muted">
          Pares débito/crédito entre cuentas propias. Se ordenan por confianza y cada uno explica en
          qué se basa: la descripción manda, y los cobros a clientes o los montos que se repiten
          decenas de veces en el mes ya no se ofrecen. <b>Emparejar excluye el par del EBITDA.</b>
        </p>
      </div>

      {todas.length > 0 && (
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-[180px]">
            <Select
              label="Confianza"
              placeholder="Todas"
              value={veredicto}
              onChange={(e) => setVeredicto(e.target.value)}
              options={[
                { value: "PROBABLE", label: "Probable" },
                { value: "REVISAR", label: "Revisar" },
                { value: "AMBIGUO", label: "Ambiguo" },
              ]}
            />
          </div>
          <div className="min-w-[240px] flex-1">
            <Input
              label="Buscar"
              value={busqueda}
              onChange={(e) => setBusqueda(e.target.value)}
              placeholder="Cuenta, descripción o monto…"
            />
          </div>
          <p className="pb-2 text-xs text-content-muted">
            {propuestas.length === todas.length
              ? `${todas.length} propuestas`
              : `${propuestas.length} de ${todas.length}`}
          </p>
          {(veredicto || busqueda) && (
            <Button
              variant="ghost"
              onClick={() => {
                setVeredicto("");
                setBusqueda("");
              }}
            >
              Limpiar
            </Button>
          )}
        </div>
      )}

      {propuestasQ.isPending ? (
        <LoadingState label="Buscando pares de traslado" />
      ) : propuestasQ.isError ? (
        <ErrorState message={mensajeError(propuestasQ.error)} onRetry={() => propuestasQ.refetch()} />
      ) : todas.length === 0 ? (
        <EmptyState message="No hay propuestas de traslado pendientes en el período." />
      ) : propuestas.length === 0 ? (
        <EmptyState message="Ninguna propuesta coincide con el filtro." />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Confianza</TH>
                <TH>Sale de (débito)</TH>
                <TH>Entra a (crédito)</TH>
                <TH className="text-right">Monto salida</TH>
                <TH className="text-right">Monto entrada</TH>
                <TH className="text-right">Acción</TH>
              </TR>
            </THead>
            <TBody>
              {propuestas.map((p) => (
                <TR key={p.debito_id + p.credito_id}>
                  <TD className="max-w-[13rem] align-top">
                    <Badge tone={TONO_VEREDICTO[p.veredicto] ?? "neutral"}>
                      {ETIQUETA_VEREDICTO[p.veredicto] ?? p.veredicto}
                    </Badge>
                    {/* Las razones son el antídoto contra emparejar a ciegas: si el par no se
                        sostiene, acá se ve por qué antes de tocar el botón. */}
                    <span className="mt-1 block whitespace-normal text-[11px] leading-snug text-content-muted">
                      {(p.razones ?? []).join(" · ")}
                    </span>
                  </TD>
                  <TD className="max-w-[18rem]">
                    <span className="font-medium">{p.cuenta_debito}</span>
                    <span className="block text-xs tabular-nums text-content-muted">
                      {formatFecha(p.fecha_debito)}
                    </span>
                    <span
                      className="block whitespace-normal break-words text-[11.5px] leading-snug text-content-muted"
                      title={p.descripcion_debito}
                    >
                      {p.descripcion_debito}
                    </span>
                  </TD>
                  <TD className="max-w-[18rem]">
                    <span className="font-medium">{p.cuenta_credito}</span>
                    <span className="block text-xs tabular-nums text-content-muted">
                      {formatFecha(p.fecha_credito)}
                    </span>
                    <span
                      className="block whitespace-normal break-words text-[11.5px] leading-snug text-content-muted"
                      title={p.descripcion_credito}
                    >
                      {p.descripcion_credito}
                    </span>
                  </TD>
                  <TD className="text-right tabular-nums text-negativo">{formatMonto(p.monto_debito)}</TD>
                  <TD className="text-right tabular-nums text-positivo">{formatMonto(p.monto_credito)}</TD>
                  <TD className="text-right align-top">
                    <Button
                      size="sm"
                      variant={p.veredicto === "PROBABLE" ? "primary" : "secondary"}
                      loading={emparejar.isPending}
                      onClick={() =>
                        emparejar.mutate(
                          { debitoId: p.debito_id, creditoId: p.credito_id },
                          {
                            onSuccess: () => toast.success("Traslado emparejado."),
                            onError: (err) => toast.error(mensajeError(err)),
                          },
                        )
                      }
                    >
                      {p.veredicto === "PROBABLE" ? "Emparejar" : "Emparejar igual"}
                    </Button>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}

      <div>
        <h3 className="text-sm font-semibold text-content">
          Traslados marcados{" "}
          <span className="text-content-muted">
            ({periodoLista ? etiquetaPeriodo(periodoLista) : "histórico completo"})
          </span>
        </h3>
        <p className="text-xs text-content-muted">
          Movimientos marcados como traslado/overnight. Asignales su concepto (p. ej. «Overnight › Overnight»
          o «Traslados de Fondos › …») sin que se rompa el emparejamiento; si uno no corresponde, desemparejalo.
        </p>
      </div>
      {trasladosQ.isPending ? (
        <LoadingState label="Cargando traslados" />
      ) : emparejados.length === 0 ? (
        <EmptyState message="No hay traslados marcados con este alcance." />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Fecha</TH>
                <TH>Cuenta</TH>
                <TH>Descripción</TH>
                <TH className="text-right">Monto</TH>
                <TH>Clasificación</TH>
                <TH className="text-right">Acción</TH>
              </TR>
            </THead>
            <TBody>
              {emparejados.map((m) => {
                const ruta = m.concepto && m.clasificacion ? `${m.concepto} › ${m.clasificacion}` : "";
                return (
                  <TR key={m.id}>
                    <TD className="whitespace-nowrap tabular-nums">{formatFecha(m.fecha)}</TD>
                    <TD className="max-w-[11rem] truncate">{m.cuenta || m.banco || "—"}</TD>
                    <TD className="min-w-[14rem] max-w-md">
                      <span className="block whitespace-normal break-words text-[12.5px] leading-snug">
                        {m.descripcion}
                      </span>
                    </TD>
                    <TD className="text-right tabular-nums">{formatMonto(m.monto_crc)}</TD>
                    <TD>
                      <ClasifCombobox
                        actual={ruta}
                        auto={m.estado_clasificacion === "AUTO"}
                        esDebito={toNumber(m.debito) > 0}
                        onElegir={(c) =>
                          clasificar.mutate(
                            { movimientoId: m.id, conceptoId: c.conceptoId, clasificacionId: c.clasificacionId },
                            {
                              onSuccess: () => toast.success(`Traslado clasificado: ${c.ruta}`),
                              onError: (err) => toast.error(mensajeError(err)),
                            },
                          )
                        }
                      />
                    </TD>
                    <TD className="text-right">
                      <Button
                        size="sm"
                        variant="secondary"
                        loading={desemparejar.isPending}
                        onClick={() =>
                          desemparejar.mutate(m.id, {
                            onSuccess: () => toast.success("Traslado desemparejado."),
                            onError: (err) => toast.error(mensajeError(err)),
                          })
                        }
                      >
                        Desemparejar
                      </Button>
                    </TD>
                  </TR>
                );
              })}
            </TBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pestaña Reglas: el motor, editable (prioridad, palabras, pausar, eliminar)
// ---------------------------------------------------------------------------

const APLICA_LABEL: Record<string, string> = { DEBITO: "Débito", CREDITO: "Crédito", MIXTO: "Mixto" };

function ReglasTab({
  prefill,
  onPrefillUsado,
}: {
  prefill: string | null;
  onPrefillUsado: () => void;
}) {
  const reglasQ = useReglas();
  const [formAbierto, setFormAbierto] = useState(Boolean(prefill));
  // Filtros: cuando el diccionario mete decenas de reglas, hay que poder encontrar una.
  const [busqueda, setBusqueda] = useState("");
  const [estado, setEstado] = useState("");
  const [aplicaA, setAplicaA] = useState("");

  if (reglasQ.isPending) return <LoadingState label="Cargando reglas" />;
  if (reglasQ.isError)
    return <ErrorState message={mensajeError(reglasQ.error)} onRetry={() => reglasQ.refetch()} />;
  const todas = reglasQ.data ?? [];
  const reglas = todas.filter((r) => {
    if (estado === "activas" && !r.activo) return false;
    if (estado === "pausadas" && r.activo) return false;
    if (aplicaA && r.aplica_a !== aplicaA) return false;
    if (!busqueda.trim()) return true;
    const t = sinTildes(busqueda);
    return (
      sinTildes(r.nombre).includes(t) ||
      sinTildes(r.concepto ?? "").includes(t) ||
      sinTildes(r.clasificacion ?? "").includes(t) ||
      (r.palabras_clave ?? []).some((p) => sinTildes(p).includes(t))
    );
  });

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-3xl text-sm text-content-muted">
          Cada movimiento toma la <strong className="text-content">primera regla que coincide</strong>,
          evaluadas por <strong className="text-content">prioridad (mayor primero)</strong>. Nacen del banner
          de aprendizaje al clasificar o se crean aquí; luego se afinan: prioridad, palabras, pausar, eliminar.
        </p>
        {!formAbierto && (
          <Button size="sm" onClick={() => setFormAbierto(true)}>
            + Nueva regla
          </Button>
        )}
      </div>
      {formAbierto && (
        <NuevaReglaForm
          prefill={prefill}
          onListo={() => {
            setFormAbierto(false);
            onPrefillUsado();
          }}
          onCancelar={() => {
            setFormAbierto(false);
            onPrefillUsado();
          }}
        />
      )}
      {todas.length > 0 && (
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-[240px] flex-1">
            <Input
              label="Buscar"
              value={busqueda}
              onChange={(e) => setBusqueda(e.target.value)}
              placeholder="Palabra clave, concepto o clasificación…"
            />
          </div>
          <div className="min-w-[150px]">
            <Select
              label="Estado"
              placeholder="Todas"
              value={estado}
              onChange={(e) => setEstado(e.target.value)}
              options={[
                { value: "activas", label: "Activas" },
                { value: "pausadas", label: "Pausadas" },
              ]}
            />
          </div>
          <div className="min-w-[150px]">
            <Select
              label="Aplica a"
              placeholder="Todos"
              value={aplicaA}
              onChange={(e) => setAplicaA(e.target.value)}
              options={[
                { value: "DEBITO", label: "Débitos" },
                { value: "CREDITO", label: "Créditos" },
                { value: "MIXTO", label: "Mixto" },
              ]}
            />
          </div>
          <p className="pb-2 text-xs text-content-muted">
            {reglas.length === todas.length ? `${todas.length} reglas` : `${reglas.length} de ${todas.length}`}
          </p>
          {(busqueda || estado || aplicaA) && (
            <Button
              variant="ghost"
              onClick={() => {
                setBusqueda("");
                setEstado("");
                setAplicaA("");
              }}
            >
              Limpiar
            </Button>
          )}
        </div>
      )}

      {todas.length === 0 ? (
        <EmptyState message="Todavía no hay reglas. Creá una con «+ Nueva regla», o clasificá un movimiento y aceptá la sugerencia del motor." />
      ) : reglas.length === 0 ? (
        <EmptyState message="Ninguna regla coincide con el filtro." />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH className="w-24 text-center">Prioridad</TH>
                <TH>Regla</TH>
                <TH>Aplica a</TH>
                <TH>Clasifica como</TH>
                <TH>Palabras clave</TH>
                <TH className="text-right">Aciertos</TH>
                <TH className="text-center">Activa</TH>
                <TH className="text-right">Acción</TH>
              </TR>
            </THead>
            <TBody>
              {reglas.map((r) => (
                <ReglaRow key={r.id} regla={r} />
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}

/** Formulario de regla manual: palabras clave → clasificación, con retro-aplicación. */
function NuevaReglaForm({
  prefill,
  onListo,
  onCancelar,
}: {
  prefill: string | null;
  onListo: () => void;
  onCancelar: () => void;
}) {
  const toast = useToast();
  const crear = useCrearRegla();
  const [palabras, setPalabras] = useState(prefill ?? "");
  const [aplicaA, setAplicaA] = useState<AplicaA>("MIXTO");
  const [prioridad, setPrioridad] = useState("100");
  const [destino, setDestino] = useState<ClasifElegida | null>(null);

  function crearRegla() {
    const lista = palabras
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    if (lista.length === 0) {
      toast.error("Escribí al menos una palabra clave.");
      return;
    }
    if (!destino) {
      toast.error("Elegí la clasificación que asigna la regla.");
      return;
    }
    crear.mutate(
      {
        nombre: `${lista[0]} → ${destino.ruta.split(" › ").pop() ?? destino.ruta}`,
        aplica_a: aplicaA,
        concepto_id: destino.conceptoId,
        clasificacion_id: destino.clasificacionId,
        prioridad: Number(prioridad) || 100,
        palabras_clave: lista,
      },
      {
        onSuccess: (res) => {
          toast.success(
            res.clasificados > 0
              ? `Regla creada: clasificó ${res.clasificados} movimiento(s) pendientes al instante.`
              : "Regla creada: aplicará a los próximos movimientos que coincidan.",
          );
          onListo();
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex flex-wrap items-end gap-3 rounded-xl border border-accent/40 bg-accent/5 px-4 py-3">
      <Input
        label="Palabras clave (separadas por coma)"
        value={palabras}
        onChange={(e) => setPalabras(e.target.value)}
        placeholder="Ej. OVERNIGHT, DESINV, OVN"
        className="min-w-72"
        autoFocus
      />
      <Select
        label="Aplica a"
        value={aplicaA}
        onChange={(e) => setAplicaA(e.target.value as AplicaA)}
        options={[
          { value: "MIXTO", label: "Débitos y créditos" },
          { value: "DEBITO", label: "Solo débitos" },
          { value: "CREDITO", label: "Solo créditos" },
        ]}
        className="min-w-44"
      />
      <div className="flex flex-col gap-1">
        <span className="text-xs font-medium text-content-muted">Clasifica como</span>
        <ClasifCombobox
          actual={destino?.ruta ?? ""}
          placeholder="Elegir clasificación…"
          esDebito={aplicaA === "DEBITO" ? true : aplicaA === "CREDITO" ? false : undefined}
          onElegir={setDestino}
        />
      </div>
      <Input
        label="Prioridad"
        type="number"
        value={prioridad}
        onChange={(e) => setPrioridad(e.target.value)}
        className="w-24"
      />
      <div className="flex gap-2">
        <Button size="sm" onClick={crearRegla} loading={crear.isPending}>
          Crear regla y aplicar
        </Button>
        <Button size="sm" variant="ghost" onClick={onCancelar}>
          Cancelar
        </Button>
      </div>
      <p className="w-full text-xs text-content-muted">
        La regla clasifica al instante los movimientos pendientes que coincidan y los que lleguen en futuras
        importaciones. Coincidencia exacta (≥90%) o el movimiento queda sin identificar.
      </p>
    </div>
  );
}

function ReglaRow({ regla }: { regla: Regla }) {
  const toast = useToast();
  const actualizar = useActualizarRegla();
  const eliminar = useEliminarRegla();
  const [nuevaPalabra, setNuevaPalabra] = useState("");
  const [agregando, setAgregando] = useState(false);
  const [confirmando, setConfirmando] = useState(false);

  function patch(input: Parameters<typeof actualizar.mutate>[0]["input"], okMsg?: string) {
    actualizar.mutate(
      { id: regla.id, input },
      {
        onSuccess: () => okMsg && toast.success(okMsg),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function agregarPalabra() {
    const p = nuevaPalabra.trim();
    if (!p) return;
    patch({ agregar_palabras: [p] }, `Palabra «${p}» agregada.`);
    setNuevaPalabra("");
    setAgregando(false);
  }

  return (
    <TR className={!regla.activo ? "opacity-55" : undefined}>
      <TD>
        <div className="flex items-center justify-center gap-1">
          <button
            type="button"
            onClick={() => patch({ prioridad: regla.prioridad - 10 })}
            disabled={actualizar.isPending}
            className="rounded px-1 text-content-muted hover:bg-surface-muted hover:text-content"
            title="Bajar prioridad"
            aria-label={`Bajar prioridad de ${regla.nombre}`}
          >
            ▼
          </button>
          <span className="w-10 text-center font-medium tabular-nums">{regla.prioridad}</span>
          <button
            type="button"
            onClick={() => patch({ prioridad: regla.prioridad + 10 })}
            disabled={actualizar.isPending}
            className="rounded px-1 text-content-muted hover:bg-surface-muted hover:text-content"
            title="Subir prioridad"
            aria-label={`Subir prioridad de ${regla.nombre}`}
          >
            ▲
          </button>
        </div>
      </TD>
      <TD className="max-w-[14rem] truncate font-medium" title={regla.nombre}>
        {regla.nombre}
      </TD>
      <TD>
        <Badge tone="neutral">{APLICA_LABEL[regla.aplica_a] ?? regla.aplica_a}</Badge>
      </TD>
      <TD className="max-w-[14rem]">
        <span className="block truncate" title={`${regla.concepto} › ${regla.clasificacion}`}>
          {regla.concepto} <span className="text-content-muted">› {regla.clasificacion}</span>
        </span>
      </TD>
      <TD>
        <div className="flex max-w-xs flex-wrap items-center gap-1">
          {regla.palabras_clave.map((p) => (
            <span
              key={p}
              className="inline-flex items-center gap-1 rounded bg-accent/10 px-1.5 py-0.5 text-xs font-medium text-accent"
            >
              {p}
              {regla.palabras_clave.length > 1 && (
                <button
                  type="button"
                  onClick={() => patch({ quitar_palabras: [p] }, `Palabra «${p}» quitada.`)}
                  className="text-accent/60 hover:text-negativo"
                  title={`Quitar «${p}»`}
                  aria-label={`Quitar palabra ${p}`}
                >
                  ×
                </button>
              )}
            </span>
          ))}
          {agregando ? (
            <input
              autoFocus
              value={nuevaPalabra}
              onChange={(e) => setNuevaPalabra(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") agregarPalabra();
                if (e.key === "Escape") setAgregando(false);
              }}
              onBlur={() => setAgregando(false)}
              placeholder="palabra…"
              className="w-24 rounded border border-border bg-transparent px-1.5 py-0.5 text-xs focus:outline-none focus:ring-1 focus:ring-accent"
            />
          ) : (
            <button
              type="button"
              onClick={() => setAgregando(true)}
              className="rounded border border-dashed border-border px-1.5 py-0.5 text-xs text-content-muted hover:text-accent"
              title="Agregar palabra clave"
            >
              + palabra
            </button>
          )}
        </div>
      </TD>
      <TD className="text-right font-medium tabular-nums">{regla.aciertos}</TD>
      <TD className="text-center">
        <button
          type="button"
          role="switch"
          aria-checked={regla.activo}
          onClick={() =>
            patch({ activo: !regla.activo }, regla.activo ? "Regla pausada." : "Regla activada.")
          }
          disabled={actualizar.isPending}
          className={cn(
            "relative inline-flex h-5 w-9 items-center rounded-full transition-colors",
            regla.activo ? "bg-accent" : "bg-surface-muted border border-border",
          )}
          title={regla.activo ? "Pausar regla" : "Activar regla"}
        >
          <span
            className="inline-block h-3.5 w-3.5 rounded-full bg-surface-raised shadow transition-transform"
            style={{ transform: regla.activo ? "translateX(18px)" : "translateX(2px)" }}
          />
        </button>
      </TD>
      <TD className="text-right">
        {confirmando ? (
          <div className="flex items-center justify-end gap-1.5">
            <span className="text-xs text-content-muted">¿Eliminar?</span>
            <Button
              size="sm"
              variant="secondary"
              className="text-negativo"
              loading={eliminar.isPending}
              onClick={() =>
                eliminar.mutate(regla.id, {
                  onSuccess: () => toast.success("Regla eliminada. Lo ya clasificado no se toca."),
                  onError: (err) => toast.error(mensajeError(err)),
                })
              }
            >
              Sí
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setConfirmando(false)}>
              No
            </Button>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setConfirmando(true)}
            className="rounded px-1.5 py-0.5 text-xs text-content-muted hover:text-negativo"
            title="Eliminar regla"
          >
            Eliminar
          </button>
        )}
      </TD>
    </TR>
  );
}
