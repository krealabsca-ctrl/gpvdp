/**
 * CxP — Bandeja (/cxp/bandeja). LA superficie de trabajo del módulo (validada en maqueta).
 *
 * Una pantalla, seis fases (pestañas con conteo + monto). Todo se resuelve en la fila:
 *  · Recibidas: clasificar gasto en un campo (AUTO por memoria del proveedor), tipo, Revisar,
 *    ⋯ (viáticos→liquidar, denegar, anular).
 *  · Por aprobar: matriz por monto, aprobar individual o en lote.
 *  · Por pagar: agrupadas por vencimiento, pre-marcadas hasta el corte → Generar lote + macro.
 *  · En banco: por lote → Pagada ✓ / Rebotó ✗ / Reintentar.
 *  · Pagadas: adjuntar comprobante y enviarlo al proveedor.
 *  · Archivo: terminales (denegadas, anuladas, liquidadas).
 */

import { useEffect, useMemo, useRef, useState, type ChangeEvent } from "react";
import { createPortal } from "react-dom";
import { useNavigate, useSearchParams } from "react-router-dom";
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
import { formatFecha, formatMoneda, hoyCR, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import { ETIQUETA_ESTADO, ETIQUETA_TIPO, TONO_ESTADO, esViaExpresa, puedeAccion, textoRequisitoAprobacion } from "@/features/cxp/dominio";
import {
  useAdjuntarComprobante,
  useAsignarDepartamentoDoc,
  useBandeja,
  useClasificarDocumento,
  useClasificarMasivo,
  useCrearLote,
  useDepartamentos,
  useDevolverDoc,
  useDocumentos,
  useEnviarComprobante,
  useLotes,
  usePrioridadMasiva,
  useTipoMasivo,
  useTodosProveedores,
  useTransicionMasiva,
  useValidarDepto,
  useValidarEscalado,
  useAprobarContabilidad,
  useMarcarDocContabilidad,
} from "@/features/cxp/hooks";
import { useClasificaciones } from "@/features/bancos/hooks";
import { GastoCombobox, type GastoElegido } from "@/features/cxp/components/GastoCombobox";
import {
  cxpApi,
  etiquetaOrigenContabilidad,
  etiquetaMotivoValidacion,
  type AccionMasiva,
  type Documento,
  type LotePago,
  type TipoFactura,
} from "@/api/cxp";

type FaseKey = "rec" | "val" | "apr" | "cnt" | "pag" | "bco" | "pgd" | "arc" | "abi";

const FASE_KEYS: FaseKey[] = ["rec", "val", "apr", "cnt", "pag", "bco", "pgd", "arc", "abi"];

/** Valida la pestaña que llega por URL (?fase=) antes de usarla como estado. */
function esFaseKey(v: string | null): v is FaseKey {
  return !!v && (FASE_KEYS as string[]).includes(v);
}

/** ¿El usuario puede ver esta pestaña? Los permisos viven en FASES (fuente única). */
function puedeVerFase(k: FaseKey, tiene: (p: string) => boolean): boolean {
  const f = FASES.find((x) => x.k === k);
  return !f?.permisos || f.permisos.some((p) => tiene(p));
}

/** Tramos de vencimiento admitidos (espejo de condVencimiento del backend) y su etiqueta. */
const ETIQUETA_VENCIMIENTO: Record<string, string> = {
  vencido: "Solo vencidas",
  v90: "Vencidas +90 días",
  v61: "Vencidas 61 a 90 días",
  v31: "Vencidas 31 a 60 días",
  v1: "Vencidas 1 a 30 días",
  s7: "Vencen en 7 días",
  s30: "Vencen en 30 días",
  futuro: "Vencen en más de 30 días",
  sin_fecha: "Sin fecha de vencimiento",
};

/** Descarta cualquier valor que no sea un tramo conocido (?vencimiento=). */
function normalizarVencimiento(v: string | null): string {
  return v && ETIQUETA_VENCIMIENTO[v] ? v : "";
}

// `permisos`: la pestaña se muestra si el usuario tiene ALGUNO (undefined = siempre, para
// todo usuario con cxp.ver). Así un validador de área (cxp.ver + cxp.validar_depto) solo ve
// «Por validar», «Pagadas» y «Archivo» (estas dos, filtradas a su departamento en el backend);
// las fases de conta/tesorería/aprobación quedan ocultas. El backend reverifica igual.
const FASES: {
  k: FaseKey;
  label: string;
  estados: string;
  lote?: string;
  permisos?: string[];
  abierta?: boolean;
  contabilidad?: "si" | "no";
  /**
   * Las cuatro colas del ciclo de aprobación se piden por FASE, no por lista de estados: el
   * backend las resuelve con la misma expresión que cuenta el encabezado, así que el número de
   * la pestaña y las filas que se abren al hacerle clic son siempre los mismos documentos.
   * `estados` sigue describiendo qué estados puede contener la fase (alimenta el selector).
   */
  usaFase?: boolean;
}[] = [
  // Lo marcado como «de Contabilidad» NO espera validación de área, así que sale de estas colas y
  // forma la suya. Dejarlo acá llevaba al validador a abrir una factura que no le tocaba y chocar
  // con «no sos validador de este departamento».
  { k: "rec", label: "📥 Recibidas", estados: "RECIBIDO", usaFase: true, permisos: ["cxp.clasificar", "cxp.revisar"] },
  // La cola del área es la EXCEPCIÓN, no el default: solo llegan las facturas que dispararon un
  // criterio de riesgo al revisarse (monto, proveedor esporádico, desvío contra su histórico).
  { k: "val", label: "🏭 Por validar (área)", estados: "REVISADO", usaFase: true, permisos: ["cxp.validar_depto"] },
  // «Por aprobar» junta dos caminos: lo que el área ya validó (VALIDADO_DEPTO) y lo que nunca
  // necesitó pasar por el área (REVISADO sin riesgo). Por eso no se puede pedir por estado.
  { k: "apr", label: "✍️ Por aprobar", estados: "REVISADO,VALIDADO_DEPTO", usaFase: true, permisos: ["cxp.aprobar"] },
  // «De Contabilidad»: el gasto que NO tiene área operativa que lo valide (honorarios contables,
  // timbres, comisiones bancarias, Hacienda, auditoría). Es una cola de trabajo, no una fase:
  // cruza RECIBIDO/REVISADO/VALIDADO_DEPTO porque esas facturas no dependen del área para avanzar.
  // Sin esta pestaña quedarían perdidas entre miles y habría que buscarlas a ojo.
  {
    k: "cnt",
    label: "🧾 De Contabilidad",
    estados: "RECIBIDO,REVISADO,VALIDADO_DEPTO",
    usaFase: true,
    permisos: ["cxp.aprobar_contabilidad", "cxp.marcar_contabilidad"],
  },
  { k: "pag", label: "📅 Por pagar", estados: "APROBADO,PROGRAMADO", lote: "sin", permisos: ["cxp.tesoreria"] },
  { k: "bco", label: "🏦 En banco", estados: "PROGRAMADO,REBOTADA", lote: "con", permisos: ["cxp.tesoreria"] },
  { k: "pgd", label: "✅ Pagadas", estados: "PAGADO,CONCILIADO" },
  { k: "arc", label: "🗂 Archivo", estados: "DENEGADO,ANULADO,LIQUIDADA" },
  // Cartera abierta: NO es una fase del flujo, es la POBLACIÓN que cuenta el tablero (todo
  // lo que todavía se debe, en cualquier etapa). Existe para que al hacer clic en «+90 días»
  // salgan exactamente las facturas que el tablero contó, y no las de una sola etapa.
  // Permiso propio: es la DEUDA TOTAL de la empresa, dato sensible que no corresponde a todo
  // el que pueda leer CxP. El backend reverifica el filtro `abierta` con el mismo permiso.
  { k: "abi", label: "💰 Cartera abierta", estados: "", abierta: true, permisos: ["cxp.cartera_abierta"] },
];

// Día de operación de Costa Rica (no el UTC del navegador): es el mismo con el que el
// tablero calcula el vencimiento, así que las dos pantallas nunca discrepan sobre qué está
// vencido ni sobre cuál es el próximo corte de pago.
const HOY = hoyCR();
const TIPOS: TipoFactura[] = ["CXP", "ANTICIPO", "VIATICOS", "REINTEGRO"];

/** Próximo viernes (o el mismo día si ya es viernes) — política de pago semanal a proveedores. */
function proximoViernes(desde: string): string {
  const [y, m, d] = desde.split("-").map(Number);
  const dt = new Date(y!, m! - 1, d!);
  dt.setDate(dt.getDate() + ((5 - dt.getDay() + 7) % 7)); // 5 = viernes
  const mm = String(dt.getMonth() + 1).padStart(2, "0");
  const dd = String(dt.getDate()).padStart(2, "0");
  return `${dt.getFullYear()}-${mm}-${dd}`;
}
const VIERNES = proximoViernes(HOY);

function prioridad(doc: Documento, hoy: string = HOY): { label: string; cls: string } {
  if (!doc.fecha_vencimiento) return { label: "Sin fecha", cls: "text-content-muted" };
  const d = Math.round((Date.parse(doc.fecha_vencimiento) - Date.parse(hoy)) / 86_400_000);
  if (d < 0) return { label: `Vencida ${Math.abs(d)}d`, cls: "font-semibold text-negativo" };
  if (d === 0) return { label: "Vence hoy", cls: "font-semibold text-negativo" };
  if (d <= 7) return { label: `En ${d}d`, cls: "font-medium text-pendiente" };
  return { label: `En ${d}d`, cls: "text-content-muted" };
}
function gastoRuta(d: Documento): string {
  if (d.subclasificacion) return `${d.clasificacion} › ${d.subclasificacion}`;
  return d.clasificacion || d.concepto || "";
}
function limpiarError(msg: string): string {
  return msg.replace(/^cxp:\s*/i, "");
}

export function BandejaPage() {
  const navigate = useNavigate();
  const toast = useToast();
  const tiene = useTienePermiso();

  const bandeja = useBandeja();
  // La pestaña y el tramo de vencimiento se pueden fijar por URL: así el tablero enlaza a
  // «+90 días» o «por aprobar» y aterrizás en las facturas exactas de ese número.
  const [params, setParams] = useSearchParams();
  // Una fase pedida por URL solo se acepta si el usuario PUEDE verla: un enlace viejo (o
  // compartido) a una pestaña gateada no debe aterrizar en un 403, sino en la bandeja normal.
  const faseURLCruda = params.get("fase");
  const faseURL = esFaseKey(faseURLCruda) && puedeVerFase(faseURLCruda, tiene) ? faseURLCruda : null;
  const [fase, setFase] = useState<FaseKey>(faseURL ?? "rec");
  const [vencFiltro, setVencFiltro] = useState(() => normalizarVencimiento(params.get("vencimiento")));
  const [sel, setSel] = useState<Set<string>>(new Set());
  const preselRef = useRef(false);
  // Con la pestaña pedida por URL, el auto-enfoque ya está resuelto desde el primer render.
  const focoRef = useRef(faseURL !== null);

  // Si llega otra combinación desde el tablero con la Bandeja ya abierta, se aplica.
  // focoRef se marca para que el auto-enfoque de «primera fase con trabajo» no pise la
  // pestaña que pidió el enlace.
  useEffect(() => {
    const f = params.get("fase");
    if (esFaseKey(f) && puedeVerFase(f, tiene)) {
      setFase(f);
      focoRef.current = true;
    }
    setVencFiltro(normalizarVencimiento(params.get("vencimiento")));
  }, [params]);

  // Filtros (aplican a la pestaña activa): búsqueda libre, proveedor, gasto, monto y estado.
  const [qInput, setQInput] = useState("");
  const [q, setQ] = useState("");
  const [provFiltro, setProvFiltro] = useState("");
  const [gastoFiltro, setGastoFiltro] = useState(""); // clasificacion_id
  const [montoMinIn, setMontoMinIn] = useState("");
  const [montoMaxIn, setMontoMaxIn] = useState("");
  const [montoMin, setMontoMin] = useState("");
  const [montoMax, setMontoMax] = useState("");
  const [estadoFiltro, setEstadoFiltro] = useState("");
  const proveedoresQ = useTodosProveedores();
  const clasificacionesQ = useClasificaciones("cxp");
  useEffect(() => {
    const t = setTimeout(() => {
      setQ(qInput.trim());
      setMontoMin(montoMinIn.trim());
      setMontoMax(montoMaxIn.trim());
    }, 300);
    return () => clearTimeout(t);
  }, [qInput, montoMinIn, montoMaxIn]);
  const hayFiltros = !!(q || provFiltro || gastoFiltro || montoMin || montoMax || estadoFiltro || vencFiltro);
  function limpiarFiltros() {
    setQInput("");
    setProvFiltro("");
    setGastoFiltro("");
    setMontoMinIn("");
    setMontoMaxIn("");
    setEstadoFiltro("");
    quitarVencimiento();
  }

  /** Quita el tramo de vencimiento y lo saca de la URL (para que no reaparezca al recargar). */
  function quitarVencimiento() {
    setVencFiltro("");
    if (params.has("vencimiento")) {
      const p = new URLSearchParams(params);
      p.delete("vencimiento");
      setParams(p, { replace: true });
    }
  }

  /**
   * Cambio de pestaña a mano: escribe la fase en la URL (así lo que se ve y lo que dice la
   * dirección no se contradicen) y suelta el tramo de vencimiento, que pertenece al enlace
   * del tablero y no tiene sentido arrastrarlo a otra población.
   */
  function cambiarFase(k: FaseKey) {
    setFase(k);
    focoRef.current = true;
    setParams(new URLSearchParams({ fase: k }), { replace: true });
  }

  // Diálogo de motivo (contrapartida del archivo: denegar/anular/liquidar/rebotar).
  const [motivo, setMotivo] = useState<{ accion: AccionMasiva; ids: string[]; titulo: string; ok: string } | null>(null);
  // Diálogos de la validación por departamento.
  const [validar, setValidar] = useState<Documento | null>(null);
  const [escalar, setEscalar] = useState<Documento | null>(null);
  const [devolver, setDevolver] = useState<{ id: string; titulo: string } | null>(null);
  const departamentosQ = useDepartamentos(true);
  const asignarDepM = useAsignarDepartamentoDoc();
  const validarM = useValidarDepto();
  const escalarM = useValidarEscalado();
  const devolverM = useDevolverDoc();

  // Facturas «de Contabilidad»: aprobar por la vía propia (el permiso es otro) y quitar la marca.
  const aprobarContaM = useAprobarContabilidad();
  const quitarMarcaM = useMarcarDocContabilidad();
  const puedeAprobarConta = tiene("cxp.aprobar_contabilidad");
  const puedeMarcarConta = tiene("cxp.marcar_contabilidad");
  function aprobarDeContabilidad(id: string) {
    aprobarContaM.mutate(id, {
      onSuccess: (d) =>
        toast.success(
          d.estado === "APROBADO"
            ? "Aprobada sin validación de área → Por pagar"
            : "Firma registrada: falta(n) firma(s) para completar la matriz.",
        ),
      onError: (e) => toast.error(limpiarError(mensajeError(e))),
    });
  }

  const conf = FASES.find((f) => f.k === fase)!;
  const docsQ = useDocumentos({
    // Con `usaFase`, la cola la define el backend; `estados` solo se manda si el usuario eligió
    // uno a mano dentro de la pestaña (es un filtro ADICIONAL sobre la fase, no la fase misma).
    fase: conf.usaFase ? conf.k : undefined,
    estados: conf.usaFase ? estadoFiltro || undefined : estadoFiltro || conf.estados,
    abierta: conf.abierta,
    lote: conf.lote,
    contabilidad: conf.contabilidad,
    q: q || undefined,
    proveedor_id: provFiltro || undefined,
    clasificacion_id: gastoFiltro || undefined,
    monto_min: montoMin || undefined,
    monto_max: montoMax || undefined,
    vencimiento: vencFiltro || undefined,
    orden: "vencimiento",
    page: 1,
    page_size: 200,
  });
  const items = useMemo(() => docsQ.data?.items ?? [], [docsQ.data]);

  // Mutaciones compartidas
  const trans = useTransicionMasiva();
  const clasifDoc = useClasificarDocumento();
  const clasifMasiva = useClasificarMasivo();
  const tipoM = useTipoMasivo();
  const prioM = usePrioridadMasiva();
  const crearLote = useCrearLote();
  const adjuntar = useAdjuntarComprobante();
  const enviar = useEnviarComprobante();
  const lotesQ = useLotes();

  useEffect(() => {
    setSel(new Set());
    preselRef.current = false;
  }, [fase, q, provFiltro, gastoFiltro, montoMin, montoMax, estadoFiltro, vencFiltro]);
  // El filtro de estado es por pestaña: al cambiar de fase se limpia.
  useEffect(() => {
    setEstadoFiltro("");
  }, [fase]);

  // El resumen ya trae la fase «cnt»: el backend saca las de Contabilidad de las colas de área y
  // las cuenta aparte, con la MISMA expresión que usa el filtro de la lista. Así la pestaña y su
  // contador no pueden discrepar, y ningún documento se cuenta dos veces en el encabezado.
  const resumen = useMemo(() => {
    const m = new Map<string, { cantidad: number; monto: string }>();
    (bandeja.data?.fases ?? []).forEach((f) => m.set(f.fase, { cantidad: f.cantidad, monto: f.monto }));
    return m;
  }, [bandeja.data]);

  // Pestañas visibles según permisos (un validador de área ve solo un subconjunto).
  const fasesVisibles = FASES.filter((f) => !f.permisos || f.permisos.some((p) => tiene(p)));
  const visibleKeys = fasesVisibles.map((f) => f.k).join(",");

  // Si la pestaña activa dejó de ser visible (p. ej. tras cargar permisos), saltar a la primera visible.
  useEffect(() => {
    const primeraVisible = fasesVisibles[0];
    if (primeraVisible && !fasesVisibles.some((f) => f.k === fase)) {
      setFase(primeraVisible.k);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visibleKeys, fase]);

  // Al abrir la Bandeja, enfocar la primera fase VISIBLE con trabajo pendiente — salvo que
  // la pestaña venga pedida por URL (enlace del tablero), que manda.
  useEffect(() => {
    if (focoRef.current || !bandeja.data) return;
    const primera = fasesVisibles.find((f) => f.k !== "arc" && (resumen.get(f.k)?.cantidad ?? 0) > 0);
    if (primera) setFase(primera.k);
    focoRef.current = true;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bandeja.data, resumen]);

  // --- helpers de acciones ---
  function toggle(id: string) {
    setSel((p) => {
      const n = new Set(p);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  }
  function toggleTodos() {
    setSel((p) => (items.every((d) => p.has(d.id)) ? new Set() : new Set(items.map((d) => d.id))));
  }

  function accion(ids: string[], acc: AccionMasiva, ok: string, fecha?: string, nota?: string) {
    trans.mutate(
      { accion: acc, ids, fecha, nota },
      {
        onSuccess: (res) => {
          setSel(new Set());
          setMotivo(null);
          if (res.fallidos === 0) toast.success(ok.replace("{n}", String(res.exitosos)));
          else {
            const err = res.resultados.find((r) => !r.ok);
            toast.info(`${res.exitosos} ok · ${res.fallidos} con error: ${limpiarError(err?.error ?? "")}`);
          }
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  /** Acciones de archivo: piden motivo antes de ejecutar (queda como contrapartida). */
  function pedirMotivo(accionArc: AccionMasiva, ids: string[], titulo: string, ok: string) {
    setMotivo({ accion: accionArc, ids, titulo, ok });
  }

  function cambiarPrioridad(doc: Documento) {
    const orden: ("" | "A" | "AA")[] = ["", "A", "AA"];
    const sig = orden[(orden.indexOf(doc.prioridad) + 1) % orden.length]!;
    prioM.mutate(
      { ids: [doc.id], prioridad: sig },
      {
        onSuccess: () => toast.success(sig ? `Prioridad ${sig}${sig === "AA" ? " — se paga sí o sí" : ""}` : "Prioridad normal"),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function elegirGasto(doc: Documento, g: GastoElegido) {
    clasifDoc.mutate(
      { id: doc.id, conceptoId: g.conceptoId, clasificacionId: g.clasificacionId, subclasificacionId: g.subclasificacionId },
      {
        onSuccess: () => toast.success(`${g.ruta} — quedará como predeterminada de ${doc.proveedor.split(" ")[0]}`),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function revisar(doc: Documento) {
    // La reposición de caja chica (REINTEGRO) no exige clasificación: el gasto vive en los vales.
    if (!doc.concepto_id && doc.tipo !== "REINTEGRO") return toast.error("⚠ Primero clasificá el gasto");
    if (doc.tipo === "VIATICOS")
      return pedirMotivo("liquidar", [doc.id], `Liquidar viáticos — ${doc.proveedor}`, "Viáticos → Liquidada (sin pago)");
    accion([doc.id], "revisar", "Revisada → Por validar (área)");
  }

  function ciclarTipo(doc: Documento) {
    const sig = TIPOS[(TIPOS.indexOf(doc.tipo) + 1) % TIPOS.length]!;
    tipoM.mutate(
      { ids: [doc.id], tipo: sig },
      {
        onSuccess: () => {
          if (sig === "VIATICOS") toast.info("Viáticos: al revisar se liquida sin pago");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  const puedeRevisar = puedeAccion(tiene, "revisar");
  const puedeValidar = puedeAccion(tiene, "validar");
  const puedeEscalar = puedeAccion(tiene, "escalar");
  const puedeAprobar = puedeAccion(tiene, "aprobar");
  const puedePagar = puedeAccion(tiene, "pagar");
  const puedeRevision = puedeAccion(tiene, "denegar");
  const puedeAnticipos = tiene("cxp.anticipos");
  const irAAnticipos = (docId: string) => navigate(`/cxp/documentos/${docId}#anticipos`);

  // Guardia proactiva (estándar SAP/Oracle): si el proveedor tiene anticipos con saldo sin
  // aplicar, avisar ANTES de aprobar — aprobar sin aplicar deja la factura por su monto completo.
  const [guardAprobar, setGuardAprobar] = useState<{ ids: string[]; provs: string[] } | null>(null);
  function aprobarConGuardia(ids: string[]) {
    const ok = ids.length === 1 ? "Aprobada → Por pagar" : "{n} aprobadas → Por pagar";
    const conAnticipo = items.filter(
      (d) => ids.includes(d.id) && d.proveedor_anticipo_disponible && d.tipo !== "ANTICIPO",
    );
    if (conAnticipo.length) {
      setGuardAprobar({ ids, provs: [...new Set(conAnticipo.map((d) => d.proveedor))] });
    } else {
      accion(ids, "aprobar", ok);
    }
  }

  // --- validación por departamento ---
  function asignarDepto(doc: Documento, deptoId: string) {
    asignarDepM.mutate(
      { id: doc.id, departamentoId: deptoId },
      { onSuccess: () => toast.success("Departamento asignado"), onError: (e) => toast.error(mensajeError(e)) },
    );
  }
  function confirmarValidar(respaldo: string, nota: string) {
    if (!validar) return;
    validarM.mutate(
      { id: validar.id, respaldo, nota: nota || undefined },
      {
        onSuccess: () => {
          setValidar(null);
          toast.success("Validada → Por aprobar");
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }
  function confirmarEscalar(motivo: string, respaldo: string) {
    if (!escalar) return;
    escalarM.mutate(
      { id: escalar.id, motivo, respaldo: respaldo || undefined },
      {
        onSuccess: () => {
          setEscalar(null);
          toast.success("Validada por escalamiento → Por aprobar");
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }
  function confirmarDevolver(nota: string) {
    if (!devolver) return;
    devolverM.mutate(
      { id: devolver.id, nota: nota || undefined },
      {
        onSuccess: () => {
          setDevolver(null);
          toast.success("Devuelta a Contabilidad (Recibidas)");
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-5 pb-20">
      <PageHeader
        title="Cuentas por pagar — Bandeja"
        description="Cada pestaña es una fase del trabajo. Todo se resuelve en la fila."
        actions={
          tiene("cxp.importar") ? (
            <div className="flex gap-2">
              <Button variant="secondary" onClick={() => navigate("/cxp/importar")}>
                Importar facturación
              </Button>
              <Button variant="secondary" onClick={() => navigate("/cxp/documentos/nuevo")}>
                Nuevo documento
              </Button>
            </div>
          ) : undefined
        }
      />

      {/* Pestañas de fase */}
      <div className="flex flex-wrap gap-2">
        {fasesVisibles.map((f) => {
          const r = resumen.get(f.k);
          const on = fase === f.k;
          return (
            <button
              key={f.k}
              type="button"
              onClick={() => cambiarFase(f.k)}
              className={cn(
                "flex min-w-32 flex-col items-start gap-0.5 rounded-lg border px-3 py-2 text-left transition-colors",
                on ? "border-accent bg-accent/5" : "border-border bg-surface-raised hover:bg-surface-muted",
              )}
            >
              <span className={cn("text-xs font-semibold", on ? "text-accent" : "text-content-muted")}>{f.label}</span>
              {/* La cartera abierta cruza varias etapas: el resumen por fase no la cuenta, así
                  que va «—» y el total real se muestra sobre la tabla. Una fase del flujo sin
                  trabajo sí es un cero de verdad y se imprime como cero. */}
              <span className="text-lg font-semibold leading-none tabular-nums text-content">
                {f.abierta ? "—" : (r?.cantidad ?? 0)}
              </span>
              <span className="text-[10.5px] tabular-nums text-content-muted">
                {f.abierta ? "todo lo que se debe" : formatMoneda(r?.monto ?? "0", "CRC")}
              </span>
            </button>
          );
        })}
      </div>

      {/* Filtros (aplican a la pestaña activa) */}
      <div className="flex flex-wrap items-end gap-3">
        <Input
          label="Buscar"
          value={qInput}
          onChange={(e) => setQInput(e.target.value)}
          placeholder="Proveedor, consecutivo o clave…"
          className="min-w-56"
        />
        <Select
          label="Proveedor"
          value={provFiltro}
          onChange={(e) => setProvFiltro(e.target.value)}
          options={[{ value: "", label: "Todos" }, ...(proveedoresQ.data ?? []).map((p) => ({ value: p.id, label: p.nombre }))]}
          className="min-w-48"
        />
        <Select
          label="Gasto"
          value={gastoFiltro}
          onChange={(e) => setGastoFiltro(e.target.value)}
          options={[
            { value: "", label: "Todos" },
            ...(clasificacionesQ.data ?? []).map((c) => ({ value: c.id, label: `${c.concepto} › ${c.nombre}` })),
          ]}
          className="min-w-48"
        />
        <Input label="Monto ≥" value={montoMinIn} onChange={(e) => setMontoMinIn(e.target.value)} placeholder="0" inputMode="decimal" className="max-w-28" />
        <Input label="Monto ≤" value={montoMaxIn} onChange={(e) => setMontoMaxIn(e.target.value)} placeholder="—" inputMode="decimal" className="max-w-28" />
        {conf.estados.includes(",") && (
          <Select
            label="Estado"
            value={estadoFiltro}
            onChange={(e) => setEstadoFiltro(e.target.value)}
            options={[
              { value: "", label: "Todos" },
              ...conf.estados.split(",").map((e) => ({ value: e, label: ETIQUETA_ESTADO[e as Documento["estado"]] ?? e })),
            ]}
            className="min-w-40"
          />
        )}
        {vencFiltro && (
          <button
            type="button"
            onClick={quitarVencimiento}
            title="Quitar el filtro de vencimiento"
            className="mb-0.5 inline-flex items-center gap-1.5 rounded-full border border-accent/50 bg-accent/5 px-3 py-1.5 text-xs font-semibold text-accent hover:bg-accent/10"
          >
            {ETIQUETA_VENCIMIENTO[vencFiltro]}
            <span aria-hidden>×</span>
          </button>
        )}
        {hayFiltros && (
          <Button variant="ghost" size="sm" onClick={limpiarFiltros}>
            Limpiar filtros
          </Button>
        )}
      </div>

      {/* Diálogo de motivo (contrapartida del archivo) */}
      {motivo && (
        <MotivoDialog
          titulo={motivo.titulo}
          pendiente={trans.isPending}
          onCancelar={() => setMotivo(null)}
          onConfirmar={(nota) => accion(motivo.ids, motivo.accion, motivo.ok, undefined, nota)}
        />
      )}

      {/* Diálogo de validación de área (checklist de conformidad + respaldo obligatorio) */}
      {validar && (
        <ValidarDialog
          doc={validar}
          pendiente={validarM.isPending}
          onCancelar={() => setValidar(null)}
          onConfirmar={confirmarValidar}
        />
      )}

      {/* Diálogo de escalamiento (Dirección valida cuando el área está trancada) */}
      {escalar && (
        <EscalarDialog
          doc={escalar}
          pendiente={escalarM.isPending}
          onCancelar={() => setEscalar(null)}
          onConfirmar={confirmarEscalar}
        />
      )}

      {/* Diálogo de devolución a Contabilidad */}
      {devolver && (
        <MotivoDialog
          titulo={devolver.titulo}
          pendiente={devolverM.isPending}
          onCancelar={() => setDevolver(null)}
          onConfirmar={confirmarDevolver}
        />
      )}

      {/* Guardia proactiva: proveedor con anticipos sin aplicar al momento de aprobar */}
      {guardAprobar && (
        <ConfirmDialog
          titulo="Hay anticipos sin aplicar"
          descripcion={`${guardAprobar.provs.join(", ")} ${guardAprobar.provs.length === 1 ? "tiene" : "tienen"} anticipos con saldo disponible. Si aprobás sin aplicarlos, la factura sigue por su monto completo y el anticipo queda como saldo sin usar.`}
          impacto={["Para netear: abrí el expediente de la factura (⋯ → 🔗 Aplicar anticipo) y volvé a aprobar por el neto."]}
          textoConfirmar="Aprobar igual"
          tono="peligro"
          pendiente={trans.isPending}
          onConfirmar={() => {
            const ids = guardAprobar.ids;
            setGuardAprobar(null);
            accion(ids, "aprobar", ids.length === 1 ? "Aprobada → Por pagar" : "{n} aprobadas → Por pagar");
          }}
          onCancelar={() => setGuardAprobar(null)}
        />
      )}

      {/* Cuántas hay de verdad: la tabla trae hasta 200 y callarlo hacía que el número del
          tablero pareciera no cuadrar con la lista. */}
      {docsQ.data && docsQ.data.total > 0 && (
        <p className="text-xs tabular-nums text-content-muted">
          {docsQ.data.total > items.length
            ? `Mostrando ${items.length} de ${docsQ.data.total.toLocaleString("es-CR")} facturas — afiná los filtros para ver el resto.`
            : `${docsQ.data.total.toLocaleString("es-CR")} ${docsQ.data.total === 1 ? "factura" : "facturas"}`}
        </p>
      )}

      {docsQ.isPending ? (
        <LoadingState label="Cargando facturas" />
      ) : docsQ.isError ? (
        <ErrorState message={mensajeError(docsQ.error)} onRetry={() => docsQ.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState message={hayFiltros ? "Sin resultados con estos filtros." : fase === "rec" ? "Nada pendiente. Importá facturación para empezar." : "Nada pendiente en esta fase ✓"} />
      ) : fase === "rec" ? (
        <TabRecibidas
          items={items}
          sel={sel}
          onToggle={toggle}
          onToggleTodos={toggleTodos}
          onGasto={elegirGasto}
          onRevisar={revisar}
          onTipo={ciclarTipo}
          onBulkGasto={(g) => {
            const ids = [...sel];
            clasifMasiva.mutate(
              { ids, conceptoId: g.conceptoId, clasificacionId: g.clasificacionId, subclasificacionId: g.subclasificacionId },
              {
                onSuccess: (res) => {
                  setSel(new Set());
                  toast.success(`${res.exitosos} clasificadas: ${g.ruta}`);
                },
                onError: (err) => toast.error(mensajeError(err)),
              },
            );
          }}
          onBulkRevisar={() => {
            const docs = items.filter((d) => sel.has(d.id));
            const sinGasto = docs.filter((d) => !d.concepto_id).length;
            if (sinGasto) return toast.error(`⚠ ${sinGasto} sin gasto — clasificalas primero`);
            const viat = docs.filter((d) => d.tipo === "VIATICOS").map((d) => d.id);
            const norm = docs.filter((d) => d.tipo !== "VIATICOS").map((d) => d.id);
            if (viat.length) pedirMotivo("liquidar", viat, `Liquidar ${viat.length} viáticos`, "{n} viáticos liquidados");
            if (norm.length) accion(norm, "revisar", "{n} revisadas → Por aprobar");
          }}
          onAccionFila={(id, acc, ok) =>
            acc === "denegar" || acc === "anular" || acc === "liquidar"
              ? pedirMotivo(acc, [id], acc === "denegar" ? "Denegar factura" : acc === "anular" ? "Anular factura" : "Liquidar viáticos (sin pago)", ok)
              : accion([id], acc, ok)
          }
          onVer={(id) => navigate(`/cxp/documentos/${id}`)}
          puedeRevisar={puedeRevisar}
          puedeRevision={puedeRevision}
          puedeAnticipos={puedeAnticipos}
          onAplicarAnticipo={irAAnticipos}
          puedeAprobar={puedeAprobar}
          onAprobarAnticipo={(id) => accion([id], "aprobar", "Anticipo aprobado → Por pagar")}
          pendiente={trans.isPending || tipoM.isPending}
        />
      ) : fase === "val" ? (
        <TabValidar
          items={items}
          departamentos={departamentosQ.data ?? []}
          onAsignarDepto={asignarDepto}
          onValidar={(d) => setValidar(d)}
          onEscalar={(d) => setEscalar(d)}
          onDevolver={(d) => setDevolver({ id: d.id, titulo: `Devolver a Contabilidad — ${d.proveedor}` })}
          onVer={(id) => navigate(`/cxp/documentos/${id}`)}
          puedeValidar={puedeValidar}
          puedeEscalar={puedeEscalar}
          puedeAnticipos={puedeAnticipos}
          onAplicarAnticipo={irAAnticipos}
          puedeAprobar={puedeAprobar}
          onAprobarDirecto={(id) => accion([id], "aprobar", "Aprobada (documento interno) → Por pagar")}
          asignandoDepto={asignarDepM.isPending}
        />
      ) : fase === "apr" ? (
        <TabAprobar
          items={items}
          sel={sel}
          onToggle={toggle}
          onToggleTodos={toggleTodos}
          onAprobar={(id) => aprobarConGuardia([id])}
          onBulk={() => aprobarConGuardia([...sel])}
          onPrioridad={cambiarPrioridad}
          onAccionFila={(id, acc, ok) =>
            acc === "anular" ? pedirMotivo(acc, [id], "Anular factura", ok) : accion([id], acc, ok)
          }
          onVer={(id) => navigate(`/cxp/documentos/${id}`)}
          puedeAprobar={puedeAprobar}
          puedeRevision={puedeRevision}
          puedeAnticipos={puedeAnticipos}
          onAplicarAnticipo={irAAnticipos}
          pendiente={trans.isPending}
        />
      ) : fase === "cnt" ? (
        <TabContabilidad
          items={items}
          totalFiltro={docsQ.data?.total ?? items.length}
          montoTotal={resumen.get("cnt")?.monto ?? ""}
          onAprobar={aprobarDeContabilidad}
          onQuitarMarca={(d) => {
            // Si la marca es HEREDADA (proveedor o rubro), poner `null` la deja igual: seguiría
            // heredándola y el aviso de éxito mentiría. Para sacar ESTA factura hay que forzar
            // `false`. Solo cuando la marca es de la factura misma se vuelve a heredar.
            const heredada = d.contabilidad_origen !== "FACTURA";
            quitarMarcaM.mutate(
              { id: d.id, valor: heredada ? false : null },
              {
                onSuccess: () =>
                  toast.success(
                    heredada
                      ? "Esta factura la valida el área (el proveedor/rubro sigue marcado)."
                      : "Marca quitada: vuelve a heredar del proveedor/rubro.",
                  ),
                onError: (e) => toast.error(limpiarError(mensajeError(e))),
              },
            );
          }}
          onVer={(id) => navigate(`/cxp/documentos/${id}`)}
          puedeAprobar={puedeAprobarConta}
          puedeMarcar={puedeMarcarConta}
          pendiente={aprobarContaM.isPending || quitarMarcaM.isPending}
        />
      ) : fase === "pag" ? (
        <TabPagar
          items={items}
          sel={sel}
          preselRef={preselRef}
          setSel={setSel}
          onToggle={toggle}
          crearLote={(fecha) => {
            const ids = [...sel];
            crearLote.mutate(
              { fechaCorte: fecha, ids },
              {
                onSuccess: async (lote) => {
                  setSel(new Set());
                  try {
                    const blob = await cxpApi.descargarMacroLote(lote.id);
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement("a");
                    a.href = url;
                    a.download = `macro-lote-${lote.numero}.txt`;
                    document.body.appendChild(a);
                    a.click();
                    a.remove();
                    URL.revokeObjectURL(url);
                  } catch {
                    /* la macro se puede re-descargar desde En banco */
                  }
                  toast.success(`Lote #${lote.numero} creado (${lote.cantidad} pagos) · macro descargada — subila al banco`);
                  setFase("bco");
                },
                onError: (err) => toast.error(mensajeError(err)),
              },
            );
          }}
          onPrioridad={cambiarPrioridad}
          onAccionFila={(id, acc, ok) =>
            acc === "anular" ? pedirMotivo(acc, [id], "Anular factura", ok) : accion([id], acc, ok)
          }
          onVer={(id) => navigate(`/cxp/documentos/${id}`)}
          puedePagar={puedePagar}
          puedeRevision={puedeRevision}
          creando={crearLote.isPending}
        />
      ) : fase === "bco" ? (
        <TabBanco
          items={items}
          lotes={lotesQ.data ?? []}
          onPagada={(id) => accion([id], "pagar", "Pagada ✓ — adjuntá el comprobante en Pagadas")}
          onRebota={(id) => pedirMotivo("rebotar", [id], "Motivo del rebote (banco)", "Rebotada — podés reintentarla en otro corte")}
          onReintentar={(id) => accion([id], "reintentar", "De vuelta en Por pagar")}
          onMacro={async (loteId, numero) => {
            try {
              const blob = await cxpApi.descargarMacroLote(loteId);
              const url = URL.createObjectURL(blob);
              const a = document.createElement("a");
              a.href = url;
              a.download = `macro-lote-${numero}.txt`;
              document.body.appendChild(a);
              a.click();
              a.remove();
              URL.revokeObjectURL(url);
            } catch (err) {
              toast.error(mensajeError(err));
            }
          }}
          puedePagar={puedePagar}
          pendiente={trans.isPending}
        />
      ) : fase === "pgd" ? (
        <TabPagadas
          items={items}
          onAdjuntar={(id, archivo) =>
            adjuntar.mutate(
              { id, archivo },
              { onSuccess: () => toast.success("Comprobante adjuntado"), onError: (e) => toast.error(mensajeError(e)) },
            )
          }
          onEnviar={(id) =>
            enviar.mutate(id, {
              onSuccess: () => toast.success("Enviado al proveedor con el PDF adjunto"),
              onError: (e) => toast.error(mensajeError(e)),
            })
          }
          onVer={(id) => navigate(`/cxp/documentos/${id}`)}
          puede={puedePagar}
          pendiente={adjuntar.isPending || enviar.isPending}
        />
      ) : fase === "abi" ? (
        <TabCarteraAbierta items={items} hoy={HOY} onVer={(id) => navigate(`/cxp/documentos/${id}`)} />
      ) : (
        <TabArchivo items={items} onVer={(id) => navigate(`/cxp/documentos/${id}`)} />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Diálogo de motivo (contrapartida al denegar/anular/liquidar/rebotar)
// ---------------------------------------------------------------------------
function MotivoDialog({
  titulo,
  pendiente,
  onConfirmar,
  onCancelar,
}: {
  titulo: string;
  pendiente: boolean;
  onConfirmar: (nota: string) => void;
  onCancelar: () => void;
}) {
  const [nota, setNota] = useState("");
  return createPortal(
    <div className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4" onMouseDown={(e) => e.target === e.currentTarget && onCancelar()}>
      <div className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="mb-1 text-base font-semibold text-content">{titulo}</h2>
        <p className="mb-3 text-xs text-content-muted">
          El motivo queda en el expediente y en el Archivo (quién, cuándo y por qué).
        </p>
        <textarea
          value={nota}
          onChange={(e) => setNota(e.target.value)}
          autoFocus
          placeholder="Motivo / detalle… (ej. «almuerzo atención familia Rojas, pagado con caja chica»)"
          className="mb-3 min-h-24 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-content placeholder:text-content-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        />
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onCancelar}>
            Cancelar
          </Button>
          <Button onClick={() => onConfirmar(nota.trim())} loading={pendiente}>
            Confirmar
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

// Chip de prioridad interna: clic cicla normal → A → AA.
function PrioChip({ doc, onCiclar }: { doc: Documento; onCiclar?: (d: Documento) => void }) {
  const estilo =
    doc.prioridad === "AA"
      ? "bg-negativo/15 text-negativo"
      : doc.prioridad === "A"
        ? "bg-pendiente/15 text-pendiente"
        : "bg-surface-muted text-content-muted";
  const label = doc.prioridad || "—";
  if (!onCiclar) return <span className={cn("rounded-full px-2 py-0.5 text-[10px] font-bold", estilo)}>{label}</span>;
  return (
    <button
      type="button"
      onClick={() => onCiclar(doc)}
      title="Prioridad interna: AA se paga sí o sí · A puede esperar · — normal"
      className={cn("rounded-full border border-dashed border-transparent px-2 py-0.5 text-[10px] font-bold hover:border-border", estilo)}
    >
      {label}
    </button>
  );
}

// ---------------------------------------------------------------------------
// Menú ⋯ por fila (portal con posición fija: no lo recorta la tabla)
// ---------------------------------------------------------------------------
interface MenuItem {
  label: string;
  onClick: () => void;
  tono?: "neg";
}

function Dots({ items }: { items: MenuItem[] }) {
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null);
  useEffect(() => {
    if (!pos) return;
    const cerrar = () => setPos(null);
    document.addEventListener("mousedown", cerrar);
    document.addEventListener("scroll", cerrar, true);
    return () => {
      document.removeEventListener("mousedown", cerrar);
      document.removeEventListener("scroll", cerrar, true);
    };
  }, [pos]);
  return (
    <>
      <button
        type="button"
        title="Más acciones"
        onClick={(e) => {
          const r = e.currentTarget.getBoundingClientRect();
          setPos({ x: Math.min(r.right - 240, window.innerWidth - 250), y: r.bottom + 4 });
        }}
        className="rounded-md border border-transparent px-2 py-1 font-bold tracking-widest text-content-muted hover:border-border"
      >
        ⋯
      </button>
      {pos &&
        createPortal(
          <div
            style={{ position: "fixed", left: Math.max(8, pos.x), top: pos.y, zIndex: 90 }}
            className="w-60 overflow-hidden rounded-xl border border-border bg-surface-raised py-1 shadow-lifted"
            onMouseDown={(e) => e.stopPropagation()}
          >
            {items.map((it) => (
              <button
                key={it.label}
                type="button"
                onClick={() => {
                  setPos(null);
                  it.onClick();
                }}
                className={cn("block w-full px-3 py-2 text-left text-sm hover:bg-surface-muted", it.tono === "neg" && "text-negativo")}
              >
                {it.label}
              </button>
            ))}
          </div>,
          document.body,
        )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Celdas compartidas
// ---------------------------------------------------------------------------
function CeldaProveedor({
  d,
  onTipo,
  mostrarDepto,
}: {
  d: Documento;
  onTipo?: (d: Documento) => void;
  /** Muestra el departamento (centro de costo) bajo el proveedor — seguimiento tras la validación. */
  mostrarDepto?: boolean;
}) {
  const devuelta = d.estado === "RECIBIDO" && !!d.nota_revision;
  const anticipoDisp =
    d.proveedor_anticipo_disponible &&
    d.tipo !== "ANTICIPO" &&
    (d.estado === "RECIBIDO" || d.estado === "REVISADO" || d.estado === "VALIDADO_DEPTO");
  return (
    <TD>
      <div className="flex items-center gap-2">
        <span className="font-semibold text-content">{d.proveedor}</span>
        {onTipo ? (
          <button
            type="button"
            onClick={() => onTipo(d)}
            title="Clic: cambiar tipo"
            className={cn(
              "rounded-full border border-dashed border-transparent px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide hover:border-border",
              d.tipo === "CXP" ? "bg-surface-muted text-content-muted" : "bg-pendiente/15 text-pendiente",
            )}
          >
            {ETIQUETA_TIPO[d.tipo] ?? d.tipo}
          </button>
        ) : (
          d.tipo !== "CXP" && (
            <span className="rounded-full bg-surface-muted px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-content-muted">
              {ETIQUETA_TIPO[d.tipo] ?? d.tipo}
            </span>
          )
        )}
        {devuelta && (
          <span
            title={`Devuelta a Contabilidad: ${d.nota_revision}`}
            className="rounded-full bg-pendiente/15 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-pendiente"
          >
            ↩ Devuelta
          </span>
        )}
        {anticipoDisp && (
          <span
            title="Este proveedor tiene un anticipo con saldo. Abrí el expediente para aplicarlo."
            className="rounded-full bg-accent/15 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-accent"
          >
            🔗 anticipo
          </span>
        )}
      </div>
      <div className="font-mono text-[11px] text-content-muted">{d.consecutivo || d.clave.slice(0, 14) + "…"}</div>
      {mostrarDepto && d.departamento && (
        <div className="mt-0.5 text-[11px] text-content-muted">🏢 {d.departamento}</div>
      )}
    </TD>
  );
}
function CeldaVence({ d }: { d: Documento }) {
  const p = prioridad(d);
  return (
    <TD className="whitespace-nowrap tabular-nums">
      {d.fecha_vencimiento ? formatFecha(d.fecha_vencimiento) : "—"}
      <span className={cn("block text-[11px]", p.cls)}>{p.label}</span>
    </TD>
  );
}
// Monto: muestra el neto a pagar cuando hay anticipos aplicados (con el total original de contexto).
function CeldaMonto({ d }: { d: Documento }) {
  if (toNumber(d.anticipos_aplicados) > 0) {
    return (
      <TD className="text-right tabular-nums">
        <span className="block font-medium">{formatMoneda(d.neto_crc, "CRC")}</span>
        <span
          className="block text-[10.5px] text-content-muted"
          title={`Total ${formatMoneda(d.total_crc, "CRC")} − anticipos ${formatMoneda(d.anticipos_aplicados, "CRC")}`}
        >
          🔗 neto · de {formatMoneda(d.total_crc, "CRC")}
        </span>
      </TD>
    );
  }
  return <TD className="text-right tabular-nums">{formatMoneda(d.total_crc, "CRC")}</TD>;
}

// ---------------------------------------------------------------------------
// Pestaña: Recibidas
// ---------------------------------------------------------------------------
function TabRecibidas(props: {
  items: Documento[];
  sel: Set<string>;
  onToggle: (id: string) => void;
  onToggleTodos: () => void;
  onGasto: (d: Documento, g: GastoElegido) => void;
  onRevisar: (d: Documento) => void;
  onTipo: (d: Documento) => void;
  onBulkGasto: (g: GastoElegido) => void;
  onBulkRevisar: () => void;
  onAccionFila: (id: string, acc: AccionMasiva, ok: string) => void;
  onVer: (id: string) => void;
  puedeRevisar: boolean;
  puedeRevision: boolean;
  puedeAnticipos?: boolean;
  onAplicarAnticipo?: (id: string) => void;
  /** Vía expresa: el ANTICIPO se aprueba directo desde Recibidas (matriz de firmas). */
  puedeAprobar?: boolean;
  onAprobarAnticipo?: (id: string) => void;
  pendiente: boolean;
}) {
  const { items, sel } = props;
  const autos = items.filter((d) => d.clasif_auto).length;
  const todos = items.length > 0 && items.every((d) => sel.has(d.id));
  return (
    <div className="flex flex-col gap-3">
      {autos > 0 && (
        <div className="flex items-center gap-2 rounded-lg border border-pendiente/30 bg-pendiente/10 px-3 py-2 text-sm text-content">
          ✨ <b>{autos} pre-clasificadas (AUTO)</b> por memoria del proveedor — revisá y confirmá.
        </div>
      )}
      {sel.size > 0 && (
        <div className="sticky top-2 z-30 flex flex-wrap items-center gap-2 rounded-lg border border-accent bg-surface-raised px-3 py-2 shadow-card">
          <Badge tone="accent">{sel.size} seleccionadas</Badge>
          <GastoCombobox actual="" onElegir={props.onBulkGasto} />
          {props.puedeRevisar && (
            <Button size="sm" onClick={props.onBulkRevisar} loading={props.pendiente}>
              Revisar todas ✓
            </Button>
          )}
        </div>
      )}
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH className="w-9">
                <input type="checkbox" checked={todos} onChange={props.onToggleTodos} aria-label="Seleccionar todas" className="h-4 w-4 rounded accent-accent" />
              </TH>
              <TH>Proveedor</TH>
              <TH>Gasto (segmentación)</TH>
              <TH>Vence</TH>
              <TH className="text-right">Monto</TH>
              <TH className="text-right">Acción</TH>
            </TR>
          </THead>
          <TBody>
            {items.map((d) => (
              <TR key={d.id} className={cn(sel.has(d.id) && "bg-accent/5")}>
                <TD>
                  <input type="checkbox" checked={sel.has(d.id)} onChange={() => props.onToggle(d.id)} aria-label={`Seleccionar ${d.proveedor}`} className="h-4 w-4 rounded accent-accent" />
                </TD>
                <CeldaProveedor d={d} onTipo={props.onTipo} />
                <TD>
                  {d.tipo === "ANTICIPO" ? (
                    <span className="text-xs text-content-muted" title="El anticipo es saldo a favor (activo); el gasto se clasifica una sola vez, en la factura final.">
                      — se clasifica en la factura final
                    </span>
                  ) : (
                    <GastoCombobox actual={gastoRuta(d)} auto={d.clasif_auto} proveedorId={d.proveedor_id} onElegir={(g) => props.onGasto(d, g)} />
                  )}
                </TD>
                <CeldaVence d={d} />
                <CeldaMonto d={d} />
                <TD>
                  <div className="flex items-center justify-end gap-1.5">
                    {esViaExpresa(d.tipo) && props.puedeAprobar && props.onAprobarAnticipo ? (
                      <Button size="sm" onClick={() => props.onAprobarAnticipo!(d.id)} loading={props.pendiente}>
                        Aprobar ✓
                      </Button>
                    ) : props.puedeRevisar && !esViaExpresa(d.tipo) ? (
                      <Button size="sm" onClick={() => props.onRevisar(d)} loading={props.pendiente}>
                        {d.tipo === "VIATICOS" ? "Liquidar ✓" : "Revisar ✓"}
                      </Button>
                    ) : null}
                    <Dots
                      items={[
                        ...(props.puedeRevision
                          ? [
                              { label: "🍽 Marcar viáticos y liquidar", onClick: () => { props.onAccionFila(d.id, "liquidar", "Liquidada como viáticos (sin pago)"); } },
                              { label: "Denegar factura", tono: "neg" as const, onClick: () => props.onAccionFila(d.id, "denegar", "Denegada") },
                              { label: "Anular", tono: "neg" as const, onClick: () => props.onAccionFila(d.id, "anular", "Anulada") },
                            ]
                          : []),
                        ...(props.puedeAnticipos && d.proveedor_anticipo_disponible && props.onAplicarAnticipo
                          ? [{ label: "🔗 Aplicar anticipo", onClick: () => props.onAplicarAnticipo!(d.id) }]
                          : []),
                        // Opcional: si un reintegro/interno SÍ amerita validación del área, se enruta.
                        ...((d.tipo === "REINTEGRO" || d.tipo === "INTERNO") && props.puedeRevisar
                          ? [{ label: "🏭 Enviar a validación de área", onClick: () => props.onRevisar(d) }]
                          : []),
                        { label: "Ver expediente", onClick: () => props.onVer(d.id) },
                      ]}
                    />
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pestaña: Por validar (área) — el dueño del departamento valida la conformidad
// ---------------------------------------------------------------------------
function TabValidar(props: {
  items: Documento[];
  departamentos: { id: string; nombre: string }[];
  onAsignarDepto: (d: Documento, deptoId: string) => void;
  onValidar: (d: Documento) => void;
  onEscalar: (d: Documento) => void;
  onDevolver: (d: Documento) => void;
  onVer: (id: string) => void;
  puedeValidar: boolean;
  puedeEscalar: boolean;
  puedeAnticipos?: boolean;
  onAplicarAnticipo?: (id: string) => void;
  /** Vía expresa: Reintegro/Interno/Anticipo se aprueban directo (Conta), sin validación de área. */
  puedeAprobar?: boolean;
  onAprobarDirecto?: (id: string) => void;
  asignandoDepto: boolean;
}) {
  const enRiesgo = props.items.filter((d) => (d.fecha_vencimiento ?? "9999") <= VIERNES).length;
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-start gap-2 rounded-lg border border-brand-gold/40 bg-brand-gold/10 px-3 py-2 text-sm text-content">
        🏭 <span><b>Validación del área.</b> El dueño del departamento confirma que la factura corresponde a lo solicitado/recibido y adjunta el respaldo. Acá solo llega lo que <b>disparó un criterio de riesgo</b> —la columna «Por qué» dice cuál—; el resto del gasto sigue derecho a aprobación. Sin validación de área, estas facturas no pasan a aprobación ni al pago.</span>
      </div>
      <div className="flex items-start gap-2 rounded-lg border border-border bg-surface-raised px-3 py-2 text-sm text-content-muted">
        ⏱ <span>Próximo corte de pago: <b className="text-content">viernes {formatFecha(VIERNES)}</b>. {enRiesgo > 0 ? <><b className="text-pendiente">{enRiesgo} factura(s)</b> vencen para el corte y siguen sin validar — validalas a tiempo para que entren en la corrida.</> : "Todo lo que vence para el corte ya está validado ✓"}</span>
      </div>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Proveedor</TH>
              <TH>Gasto</TH>
              <TH>Por qué</TH>
              <TH>Departamento</TH>
              <TH>Vence</TH>
              <TH className="text-right">Monto</TH>
              <TH className="text-right">Acción</TH>
            </TR>
          </THead>
          <TBody>
            {props.items.map((d) => (
              <TR key={d.id}>
                <CeldaProveedor d={d} />
                <TD className="text-sm">{gastoRuta(d) || "—"}</TD>
                <CeldaMotivoValidacion d={d} />
                <TD>
                  <Select
                    aria-label="Departamento"
                    value={d.departamento_id}
                    onChange={(e) => props.onAsignarDepto(d, e.target.value)}
                    disabled={props.asignandoDepto}
                    options={[
                      { value: "", label: "— Asignar —" },
                      ...props.departamentos.map((x) => ({ value: x.id, label: x.nombre })),
                    ]}
                    className="min-w-40"
                  />
                </TD>
                <CeldaVence d={d} />
                <CeldaMonto d={d} />
                <TD>
                  <div className="flex items-center justify-end gap-1.5">
                    {esViaExpresa(d.tipo) && props.puedeAprobar && props.onAprobarDirecto && (
                      <Button
                        size="sm"
                        title="Documento interno de Contabilidad: no requiere validación de área"
                        onClick={() => props.onAprobarDirecto!(d.id)}
                      >
                        Aprobar ✓
                      </Button>
                    )}
                    {props.puedeValidar && (
                      <Button
                        size="sm"
                        variant={esViaExpresa(d.tipo) ? "secondary" : undefined}
                        disabled={!d.departamento_id}
                        onClick={() => props.onValidar(d)}
                      >
                        Validar ✓
                      </Button>
                    )}
                    <Dots
                      items={[
                        ...(props.puedeEscalar
                          ? [{ label: "⤴ Validar por escalamiento (Dirección)", onClick: () => props.onEscalar(d) }]
                          : []),
                        ...(props.puedeValidar
                          ? [{ label: "↩ Devolver a Contabilidad", tono: "neg" as const, onClick: () => props.onDevolver(d) }]
                          : []),
                        ...(props.puedeAnticipos && d.proveedor_anticipo_disponible && props.onAplicarAnticipo
                          ? [{ label: "🔗 Aplicar anticipo", onClick: () => props.onAplicarAnticipo!(d.id) }]
                          : []),
                        { label: "Ver expediente", onClick: () => props.onVer(d.id) },
                      ]}
                    />
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
    </div>
  );
}

// Diálogo de validación de área: checklist de conformidad (3 preguntas) + respaldo obligatorio.
function ValidarDialog({
  doc,
  pendiente,
  onConfirmar,
  onCancelar,
}: {
  doc: Documento;
  pendiente: boolean;
  onConfirmar: (respaldo: string, nota: string) => void;
  onCancelar: () => void;
}) {
  const [ck, setCk] = useState([false, false, false]);
  const [respaldo, setRespaldo] = useState("");
  const [nota, setNota] = useState("");
  const preguntas = [
    "¿Mi área lo solicitó / recibió?",
    "¿El precio y la cantidad corresponden a lo pactado?",
    "¿El bien / servicio quedó conforme?",
  ];
  const listo = ck.every(Boolean) && respaldo.trim().length > 0;
  return createPortal(
    <div className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4" onMouseDown={(e) => e.target === e.currentTarget && onCancelar()}>
      <div className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="mb-1 text-base font-semibold text-content">Validar — {doc.proveedor}</h2>
        <p className="mb-3 text-xs text-content-muted">
          {gastoRuta(doc) || "Gasto sin clasificar"} · {formatMoneda(doc.total_crc, "CRC")} · {doc.departamento || "sin departamento"}
        </p>
        <div className="mb-3 flex flex-col gap-1.5">
          {preguntas.map((q, i) => (
            <label key={i} className="flex items-start gap-2 rounded-lg border border-border px-3 py-2 text-sm text-content hover:bg-surface-muted">
              <input
                type="checkbox"
                checked={ck[i]}
                onChange={(e) => setCk((p) => p.map((v, j) => (j === i ? e.target.checked : v)))}
                className="mt-0.5 h-4 w-4 rounded accent-accent"
              />
              <span>{q}</span>
            </label>
          ))}
        </div>
        <label className="mb-1 block text-xs font-medium text-content">Respaldo (obligatorio)</label>
        <input
          value={respaldo}
          onChange={(e) => setRespaldo(e.target.value)}
          placeholder="Cotización, remisión firmada, correo/WhatsApp, foto de recepción…"
          className="mb-3 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-content placeholder:text-content-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        />
        <textarea
          value={nota}
          onChange={(e) => setNota(e.target.value)}
          placeholder="Nota (opcional)"
          className="mb-3 min-h-16 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-content placeholder:text-content-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        />
        <p className="mb-3 text-[11px] text-content-muted">
          Al validar quedás como responsable de la conformidad. No podrás además aprobar esta factura en Finanzas (segregación de funciones).
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onCancelar}>
            Cancelar
          </Button>
          <Button disabled={!listo} loading={pendiente} onClick={() => onConfirmar(respaldo.trim(), nota.trim())}>
            Validar y enviar a Finanzas
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

// Diálogo de escalamiento: Dirección/Gerencia valida cuando el área está trancada.
// Motivo obligatorio (queda en auditoría como escalamiento); respaldo opcional.
function EscalarDialog({
  doc,
  pendiente,
  onConfirmar,
  onCancelar,
}: {
  doc: Documento;
  pendiente: boolean;
  onConfirmar: (motivo: string, respaldo: string) => void;
  onCancelar: () => void;
}) {
  const [motivo, setMotivo] = useState("");
  const [respaldo, setRespaldo] = useState("");
  return createPortal(
    <div className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4" onMouseDown={(e) => e.target === e.currentTarget && onCancelar()}>
      <div className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="mb-1 text-base font-semibold text-content">Validar por escalamiento — {doc.proveedor}</h2>
        <p className="mb-3 text-xs text-content-muted">
          {gastoRuta(doc) || "Gasto sin clasificar"} · {formatMoneda(doc.total_crc, "CRC")} · {doc.departamento || "sin departamento"}
        </p>
        <div className="mb-3 rounded-lg border border-pendiente/40 bg-pendiente/10 px-3 py-2 text-[12.5px] text-content">
          ⚠ Solo procede si el área <b>no tiene validador asignado</b> o la factura <b>ya está vencida</b>. Queda registrado como <b>escalamiento</b> a tu nombre (no podrás además aprobarla).
        </div>
        <label className="mb-1 block text-xs font-medium text-content">Motivo del escalamiento (obligatorio)</label>
        <textarea
          value={motivo}
          onChange={(e) => setMotivo(e.target.value)}
          autoFocus
          placeholder="Ej. «Jefe de Logística de vacaciones sin suplente; factura vence hoy»"
          className="mb-3 min-h-20 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-content placeholder:text-content-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        />
        <label className="mb-1 block text-xs font-medium text-content">Respaldo (opcional)</label>
        <input
          value={respaldo}
          onChange={(e) => setRespaldo(e.target.value)}
          placeholder="Documento de respaldo si lo hay…"
          className="mb-4 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-content placeholder:text-content-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        />
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onCancelar}>
            Cancelar
          </Button>
          <Button disabled={!motivo.trim()} loading={pendiente} onClick={() => onConfirmar(motivo.trim(), respaldo.trim())}>
            Validar por escalamiento
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

// ---------------------------------------------------------------------------
// Pestaña: Por aprobar
// ---------------------------------------------------------------------------
/**
 * Cola «De Contabilidad»: el gasto que no tiene área operativa que lo valide (honorarios
 * contables, timbres, comisiones bancarias, Hacienda, auditoría).
 *
 * Cada fila dice POR QUÉ está acá —marcada a mano, por el proveedor o por el rubro—: sin eso la
 * excepción se vuelve invisible y nadie puede auditarla. Se aprueba desde cualquier estado previo,
 * pero la matriz de firmas por monto se sigue aplicando: lo que se salta es la validación de área.
 */
function TabContabilidad(props: {
  items: Documento[];
  /** Total del filtro completo (el servidor lo cuenta); `items` es solo la página cargada. */
  totalFiltro: number;
  /** Monto de TODA la cola, del resumen del servidor (no de la página cargada). */
  montoTotal: string;
  onAprobar: (id: string) => void;
  onQuitarMarca: (d: Documento) => void;
  onVer: (id: string) => void;
  puedeAprobar: boolean;
  puedeMarcar: boolean;
  pendiente: boolean;
}) {
  const { items, totalFiltro, montoTotal } = props;
  return (
    <div className="flex flex-col gap-3">
      <div className="rounded-xl border border-border bg-surface-muted px-4 py-3 text-sm">
        <b>{totalFiltro.toLocaleString("es-CR")}</b> factura{totalFiltro === 1 ? "" : "s"} de
        Contabilidad
        {montoTotal ? ` · ${formatMoneda(montoTotal, "CRC")}` : ""}
        {items.length < totalFiltro ? ` (se muestran ${items.length})` : ""} · se aprueban{" "}
        <b>sin validación de área</b>, con la misma matriz de firmas por monto.
      </div>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Proveedor</TH>
              <TH>Gasto</TH>
              <TH>Por qué es de Contabilidad</TH>
              <TH>Estado</TH>
              <TH>Vence</TH>
              <TH className="text-right">Monto</TH>
              <TH>Firmas</TH>
              <TH className="text-right">Acción</TH>
            </TR>
          </THead>
          <TBody>
            {items.map((d) => (
              <TR key={d.id}>
                <CeldaProveedor d={d} />
                <TD className="text-sm">{gastoRuta(d) || "—"}</TD>
                <TD className="text-xs">
                  <MarcaContabilidadChip d={d} />
                </TD>
                <TD className="text-xs text-content-muted">{d.estado}</TD>
                <CeldaVence d={d} />
                <CeldaMonto d={d} />
                {/* Las firmas se miden sobre el NETO (total − anticipos aplicados), que es lo que
                    evalúa el backend. Sobre el total bruto pediría más firmas de las que exige. */}
                <TD className="text-xs text-content-muted">{textoRequisitoAprobacion(toNumber(d.neto_crc))}</TD>
                <TD>
                  <div className="flex items-center justify-end gap-1.5">
                    {props.puedeAprobar && (
                      <Button size="sm" onClick={() => props.onAprobar(d.id)} loading={props.pendiente}>
                        Aprobar
                      </Button>
                    )}
                    <Dots
                      items={[
                        { label: "Ver factura", onClick: () => props.onVer(d.id) },
                        ...(props.puedeMarcar
                          ? [
                              {
                                label:
                                  d.contabilidad_origen === "FACTURA"
                                    ? "Quitar la marca"
                                    : "Que la valide el área",
                                onClick: () => props.onQuitarMarca(d),
                              },
                            ]
                          : []),
                      ]}
                    />
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
    </div>
  );
}

/**
 * Por qué esta factura llegó a la cola del área. Sin esto el validador recibe trabajo sin saber
 * qué se le está pidiendo mirar: el criterio que la trajo es justamente lo que tiene que revisar.
 * Un guion significa que se marcó antes de la regla y no quedó constancia del criterio.
 */
function CeldaMotivoValidacion({ d }: { d: Documento }) {
  const etiqueta = etiquetaMotivoValidacion(d.validacion_motivo);
  if (!etiqueta) return <TD className="text-sm text-content-muted">—</TD>;
  const corta =
    d.validacion_motivo === "MONTO"
      ? "monto alto"
      : d.validacion_motivo === "PROVEEDOR_NUEVO"
        ? "proveedor nuevo"
        : "fuera de su histórico";
  return (
    <TD className="text-sm">
      <span title={etiqueta}>
        <Badge tone="pendiente">{corta}</Badge>
      </span>
    </TD>
  );
}

/** Chip que explica de dónde sale la marca. El «por qué» es lo que hace auditable la excepción. */
function MarcaContabilidadChip({ d }: { d: Documento }) {
  if (!d.es_contabilidad) return <span className="text-content-muted">—</span>;
  const etiqueta =
    d.contabilidad_origen === "FACTURA"
      ? "marcada a mano"
      : d.contabilidad_origen === "PROVEEDOR"
        ? "por el proveedor"
        : d.contabilidad_origen === "CLASIFICACION"
          ? "por la clasificación"
          : "por el concepto";
  return (
    <span title={etiquetaOrigenContabilidad(d.contabilidad_origen)}>
      <Badge tone="accent">🧾 {etiqueta}</Badge>
      {d.contabilidad_motivo && (
        <span className="mt-0.5 block text-content-muted">{d.contabilidad_motivo}</span>
      )}
    </span>
  );
}

function TabAprobar(props: {
  items: Documento[];
  sel: Set<string>;
  onToggle: (id: string) => void;
  onToggleTodos: () => void;
  onAprobar: (id: string) => void;
  onBulk: () => void;
  onPrioridad: (d: Documento) => void;
  onAccionFila: (id: string, acc: AccionMasiva, ok: string) => void;
  onVer: (id: string) => void;
  puedeAprobar: boolean;
  puedeRevision: boolean;
  puedeAnticipos?: boolean;
  onAplicarAnticipo?: (id: string) => void;
  pendiente: boolean;
}) {
  const { items, sel } = props;
  const todos = items.length > 0 && items.every((d) => sel.has(d.id));
  return (
    <div className="flex flex-col gap-3">
      {sel.size > 0 && props.puedeAprobar && (
        <div className="sticky top-2 z-30 flex flex-wrap items-center gap-2 rounded-lg border border-accent bg-surface-raised px-3 py-2 shadow-card">
          <Badge tone="accent">
            {sel.size} · {formatMoneda(String(items.filter((d) => sel.has(d.id)).reduce((a, d) => a + toNumber(d.total_crc), 0)), "CRC")}
          </Badge>
          <Button size="sm" onClick={props.onBulk} loading={props.pendiente}>
            Aprobar todas ✓
          </Button>
        </div>
      )}
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH className="w-9">
                <input type="checkbox" checked={todos} onChange={props.onToggleTodos} aria-label="Seleccionar todas" className="h-4 w-4 rounded accent-accent" />
              </TH>
              <TH>Proveedor</TH>
              <TH>Gasto</TH>
              <TH>Área / validó</TH>
              <TH>Prio</TH>
              <TH>Vence</TH>
              <TH className="text-right">Monto</TH>
              <TH>Firmas</TH>
              <TH className="text-right">Acción</TH>
            </TR>
          </THead>
          <TBody>
            {items.map((d) => (
              <TR key={d.id} className={cn(sel.has(d.id) && "bg-accent/5")}>
                <TD>
                  <input type="checkbox" checked={sel.has(d.id)} onChange={() => props.onToggle(d.id)} aria-label={`Seleccionar ${d.proveedor}`} className="h-4 w-4 rounded accent-accent" />
                </TD>
                <CeldaProveedor d={d} />
                <TD className="text-sm">{gastoRuta(d) || "—"}</TD>
                <TD className="text-xs">
                  {d.departamento ? <div className="text-content">🏢 {d.departamento}</div> : <span className="text-content-muted">— sin área —</span>}
                  {d.validado_depto_por_nombre && (
                    <div className="text-content-muted">
                      ✓ {d.validado_depto_por_nombre}
                      {d.validado_depto_en ? ` · ${formatFecha(d.validado_depto_en)}` : ""}
                    </div>
                  )}
                </TD>
                <TD>
                  <PrioChip doc={d} onCiclar={props.onPrioridad} />
                </TD>
                <CeldaVence d={d} />
                <CeldaMonto d={d} />
                <TD className="text-xs text-content-muted">{textoRequisitoAprobacion(toNumber(d.total_crc))}</TD>
                <TD>
                  <div className="flex items-center justify-end gap-1.5">
                    {props.puedeAprobar && (
                      <Button size="sm" onClick={() => props.onAprobar(d.id)} loading={props.pendiente}>
                        Aprobar
                      </Button>
                    )}
                    <Dots
                      items={[
                        ...(props.puedeRevision ? [{ label: "Anular", tono: "neg" as const, onClick: () => props.onAccionFila(d.id, "anular", "Anulada") }] : []),
                        ...(props.puedeAnticipos && d.proveedor_anticipo_disponible && props.onAplicarAnticipo
                          ? [{ label: "🔗 Aplicar anticipo", onClick: () => props.onAplicarAnticipo!(d.id) }]
                          : []),
                        { label: "Ver expediente", onClick: () => props.onVer(d.id) },
                      ]}
                    />
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pestaña: Por pagar (corte + lote + macro)
// ---------------------------------------------------------------------------
function TabPagar(props: {
  items: Documento[];
  sel: Set<string>;
  preselRef: { current: boolean };
  setSel: (s: Set<string>) => void;
  onToggle: (id: string) => void;
  crearLote: (fecha: string) => void;
  onPrioridad: (d: Documento) => void;
  onAccionFila: (id: string, acc: AccionMasiva, ok: string) => void;
  onVer: (id: string) => void;
  puedePagar: boolean;
  puedeRevision: boolean;
  creando: boolean;
}) {
  const { items, sel } = props;
  const [corte, setCorte] = useState(VIERNES); // política: corrida semanal los viernes

  // Pre-marcar: prioridad AA SIEMPRE (se paga sí o sí) + lo que vence hasta el corte
  // + los ANTICIPOS (por naturaleza urgentes: el área espera el desembolso).
  useEffect(() => {
    if (props.preselRef.current || !items.length) return;
    props.setSel(
      new Set(
        items
          .filter((d) => d.prioridad === "AA" || d.tipo === "ANTICIPO" || (d.fecha_vencimiento ?? "9999") <= corte)
          .map((d) => d.id),
      ),
    );
    props.preselRef.current = true;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items]);

  const grupos: [string, Documento[]][] = [
    ["Vencidas", items.filter((d) => (d.fecha_vencimiento ?? "") < HOY)],
    ["Hasta el corte", items.filter((d) => (d.fecha_vencimiento ?? "") >= HOY && (d.fecha_vencimiento ?? "9999") <= corte)],
    ["Después del corte", items.filter((d) => (d.fecha_vencimiento ?? "9999") > corte)],
  ];
  const selDocs = items.filter((d) => sel.has(d.id));
  const total = selDocs.reduce((a, d) => a + toNumber(d.total_crc), 0);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2 rounded-lg border border-border bg-surface-raised px-3 py-2 text-sm text-content-muted">
        {/* Este texto decía «Para un anticipo o pago a solicitud…», y confundía: en el resto del
            módulo ANTICIPO es un TIPO de factura (con su propio circuito y su neteo), no «pagar
            antes». Acá se explica en dos pasos concretos y sin esa palabra. */}
        📅 <span>
          <b className="text-content">Pago semanal:</b> el dinero sale el viernes{" "}
          <b className="text-content">{formatFecha(VIERNES)}</b>. Ya están marcadas las facturas
          vencidas, las que vencen antes de ese viernes y las de prioridad AA — revisá la lista,
          destildá lo que no quieras pagar y generá el lote.
          <br />
          <b className="text-content">¿Necesitás pagar algo hoy, sin esperar al viernes?</b>{" "}
          Destildá todo, marcá solo esa factura, poné la fecha de hoy en «Fecha de corte» y generá
          el lote con ella sola.
        </span>
      </div>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH className="w-9"></TH>
              <TH>Proveedor</TH>
              <TH>Gasto</TH>
              <TH>Prio</TH>
              <TH>Vence</TH>
              <TH className="text-right">Monto</TH>
              <TH></TH>
            </TR>
          </THead>
          <TBody>
            {grupos.map(([titulo, g]) =>
              g.length === 0 ? null : (
                [
                  <TR key={titulo}>
                    <TD colSpan={7} className="bg-surface-muted/60 py-1.5 text-[11px] font-bold uppercase tracking-wider text-content-muted">
                      {titulo} ({g.length})
                    </TD>
                  </TR>,
                  ...g.map((d) => (
                    <TR key={d.id} className={cn(sel.has(d.id) && "bg-accent/5")}>
                      <TD>
                        <input type="checkbox" checked={sel.has(d.id)} onChange={() => props.onToggle(d.id)} aria-label={`Incluir ${d.proveedor}`} className="h-4 w-4 rounded accent-accent" />
                      </TD>
                      <CeldaProveedor d={d} mostrarDepto />
                      <TD className="text-sm">{gastoRuta(d) || "—"}</TD>
                      <TD>
                        <PrioChip doc={d} onCiclar={props.onPrioridad} />
                      </TD>
                      <CeldaVence d={d} />
                      <CeldaMonto d={d} />
                      <TD>
                        <div className="flex justify-end">
                          <Dots
                            items={[
                              ...(props.puedeRevision ? [{ label: "Anular", tono: "neg" as const, onClick: () => props.onAccionFila(d.id, "anular", "Anulada") }] : []),
                              { label: "Ver expediente", onClick: () => props.onVer(d.id) },
                            ]}
                          />
                        </div>
                      </TD>
                    </TR>
                  )),
                ]
              ),
            )}
          </TBody>
        </Table>
      </TableContainer>

      {/* Barra del corte */}
      <div className="sticky bottom-2 z-30 flex flex-wrap items-end gap-4 rounded-xl border-2 border-accent bg-surface-raised px-4 py-3 shadow-lifted">
        <div>
          <p className="text-[11px] font-bold uppercase tracking-wider text-content-muted">Corte de pago</p>
          <p className="text-lg font-semibold tabular-nums text-content">
            {sel.size} facturas · {formatMoneda(String(total), "CRC")}
          </p>
        </div>
        <Input label="Fecha de corte" type="date" value={corte} onChange={(e) => setCorte(e.target.value)} className="max-w-40" />
        {props.puedePagar ? (
          <Button className="ml-auto" onClick={() => props.crearLote(corte)} loading={props.creando} disabled={sel.size === 0 || !corte}>
            Generar lote + macro (.txt) →
          </Button>
        ) : (
          <span className="ml-auto self-center text-xs text-content-muted">El corte lo genera Tesorería/Dirección.</span>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pestaña: En banco
// ---------------------------------------------------------------------------
function TabBanco(props: {
  items: Documento[];
  lotes: LotePago[];
  onPagada: (id: string) => void;
  onRebota: (id: string) => void;
  onReintentar: (id: string) => void;
  onMacro: (loteId: string, numero: string) => void;
  puedePagar: boolean;
  pendiente: boolean;
}) {
  const lotes = useMemo(() => {
    const m = new Map<string, Documento[]>();
    props.items.forEach((d) => {
      const k = d.lote_numero || "—";
      m.set(k, [...(m.get(k) ?? []), d]);
    });
    return [...m.entries()].sort((a, b) => Number(b[0]) - Number(a[0]));
  }, [props.items]);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2 rounded-lg border border-border bg-surface-raised px-3 py-2 text-sm text-content-muted">
        🏦 Subiste la macro al banco. Cuando procese: marcá <b className="text-content">Pagada</b> o <b className="text-content">Rebotó</b> por línea.
      </div>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Proveedor</TH>
              <TH>Estado</TH>
              <TH className="text-right">Monto</TH>
              <TH className="text-right">Resultado del banco</TH>
            </TR>
          </THead>
          <TBody>
            {lotes.map(([numero, docs]) => [
              <TR key={"l" + numero}>
                <TD colSpan={4} className="bg-surface-muted/60 py-1.5">
                  <div className="flex items-center gap-3">
                    <span className="text-[11px] font-bold uppercase tracking-wider text-content-muted">
                      Lote #{numero} · {docs.length} pagos · {formatMoneda(String(docs.reduce((a, d) => a + toNumber(d.total_crc), 0)), "CRC")}
                    </span>
                    {docs[0] && (
                      <Button size="sm" variant="ghost" onClick={() => props.onMacro(docs[0]!.lote_id, numero)}>
                        ⬇ Macro
                      </Button>
                    )}
                  </div>
                </TD>
              </TR>,
              ...docs.map((d) => (
                <TR key={d.id}>
                  <CeldaProveedor d={d} mostrarDepto />
                  <TD>
                    <Badge tone={d.estado === "REBOTADA" ? "negativo" : "pendiente"}>
                      {d.estado === "REBOTADA" ? "Rebotada" : "En banco"}
                    </Badge>
                  </TD>
                  <CeldaMonto d={d} />
                  <TD>
                    <div className="flex items-center justify-end gap-1.5">
                      {props.puedePagar && d.estado !== "REBOTADA" && (
                        <>
                          <Button size="sm" onClick={() => props.onPagada(d.id)} loading={props.pendiente}>
                            Pagada ✓
                          </Button>
                          <Button size="sm" variant="secondary" className="text-negativo" onClick={() => props.onRebota(d.id)} loading={props.pendiente}>
                            Rebotó ✗
                          </Button>
                        </>
                      )}
                      {props.puedePagar && d.estado === "REBOTADA" && (
                        <Button size="sm" variant="secondary" onClick={() => props.onReintentar(d.id)} loading={props.pendiente}>
                          ↩ Reintentar en próximo corte
                        </Button>
                      )}
                    </div>
                  </TD>
                </TR>
              )),
            ])}
          </TBody>
        </Table>
      </TableContainer>

      {/* Histórico de cortes (auditoría, sin ensuciar el trabajo activo) */}
      {props.lotes.length > 0 && (
        <details className="rounded-lg border border-border bg-surface-raised px-4 py-3">
          <summary className="cursor-pointer text-sm font-semibold text-content-muted">
            🗄 Histórico de lotes ({props.lotes.length})
          </summary>
          <div className="mt-3 overflow-x-auto">
            <table className="w-full min-w-[560px] border-collapse text-sm">
              <thead>
                <tr className="text-left text-[10.5px] uppercase tracking-wider text-content-muted">
                  <th className="pb-1.5">Lote</th>
                  <th className="pb-1.5">Corte</th>
                  <th className="pb-1.5 text-right">Facturas</th>
                  <th className="pb-1.5 text-right">Monto</th>
                  <th className="pb-1.5">Resultado</th>
                  <th className="pb-1.5"></th>
                </tr>
              </thead>
              <tbody>
                {props.lotes.map((l) => (
                  <tr key={l.id} className="border-t border-border">
                    <td className="py-1.5 font-semibold tabular-nums">#{l.numero}</td>
                    <td className="py-1.5 tabular-nums">{formatFecha(l.fecha_corte)}</td>
                    <td className="py-1.5 text-right tabular-nums">{l.cantidad}</td>
                    <td className="py-1.5 text-right tabular-nums">{formatMoneda(l.total_crc, "CRC")}</td>
                    <td className="py-1.5">
                      <span className="text-positivo">{l.pagadas} pagadas</span>
                      {l.rebotadas > 0 && <span className="text-negativo"> · {l.rebotadas} rebotadas</span>}
                      {l.pendientes > 0 && <span className="text-pendiente"> · {l.pendientes} en banco</span>}
                    </td>
                    <td className="py-1.5 text-right">
                      <Button size="sm" variant="ghost" onClick={() => props.onMacro(l.id, String(l.numero))}>
                        ⬇ Macro
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pestaña: Pagadas (comprobante + envío)
// ---------------------------------------------------------------------------
function TabPagadas(props: {
  items: Documento[];
  onAdjuntar: (id: string, archivo: File) => void;
  onEnviar: (id: string) => void;
  onVer: (id: string) => void;
  puede: boolean;
  pendiente: boolean;
}) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [paraId, setParaId] = useState<string | null>(null);

  function onFile(e: ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0];
    if (f && paraId) props.onAdjuntar(paraId, f);
    if (fileRef.current) fileRef.current.value = "";
    setParaId(null);
  }

  return (
    <div className="flex flex-col gap-3">
      <input ref={fileRef} type="file" accept="application/pdf,.pdf" onChange={onFile} className="sr-only" aria-label="Comprobante de pago (PDF)" />
      <div className="flex items-center gap-2 rounded-lg border border-border bg-surface-raised px-3 py-2 text-sm text-content-muted">
        📎 Adjuntá el comprobante del banco y envialo — el proveedor recibe su respaldo por correo.
      </div>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Proveedor</TH>
              <TH>Gasto</TH>
              <TH className="text-right">Monto</TH>
              <TH>Comprobante</TH>
              <TH className="text-right">Acción</TH>
            </TR>
          </THead>
          <TBody>
            {props.items.map((d) => (
              <TR key={d.id}>
                <CeldaProveedor d={d} mostrarDepto />
                <TD className="text-sm">{gastoRuta(d) || "—"}</TD>
                <CeldaMonto d={d} />
                <TD>
                  {d.comprobante_enviado_en ? (
                    <Badge tone="positivo">Enviado ✓ {formatFecha(d.comprobante_enviado_en.slice(0, 10))}</Badge>
                  ) : d.tiene_comprobante ? (
                    <Badge tone="accent">Adjunto ✓</Badge>
                  ) : (
                    <Badge tone="pendiente">Pendiente</Badge>
                  )}
                </TD>
                <TD>
                  <div className="flex items-center justify-end gap-1.5">
                    {props.puede && (
                      <Button
                        size="sm"
                        variant="secondary"
                        loading={props.pendiente && paraId === d.id}
                        onClick={() => {
                          setParaId(d.id);
                          fileRef.current?.click();
                        }}
                      >
                        {d.tiene_comprobante ? "📎 Reemplazar" : "📎 Adjuntar"}
                      </Button>
                    )}
                    {props.puede && d.tiene_comprobante && !d.comprobante_enviado_en && (
                      <Button size="sm" onClick={() => props.onEnviar(d.id)} loading={props.pendiente}>
                        ✉ Enviar
                      </Button>
                    )}
                    <Button size="sm" variant="ghost" onClick={() => props.onVer(d.id)}>
                      Ver
                    </Button>
                  </div>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pestaña: Archivo
// ---------------------------------------------------------------------------
/**
 * Cartera abierta: la MISMA población que cuenta el tablero (todo lo que se debe, en
 * cualquier etapa). Es de solo lectura a propósito — las acciones viven en la fase que le
 * corresponde a cada documento; acá se viene a ver y a decidir qué pagar.
 */
function TabCarteraAbierta({
  items,
  hoy,
  onVer,
}: {
  items: Documento[];
  hoy: string;
  onVer: (id: string) => void;
}) {
  return (
    <TableContainer>
      <Table>
        <THead>
          <TR>
            <TH>Proveedor</TH>
            <TH>Etapa</TH>
            <TH>Vence</TH>
            <TH>Prioridad</TH>
            <TH className="text-right">Monto</TH>
            <TH className="text-right"></TH>
          </TR>
        </THead>
        <TBody>
          {items.map((d) => {
            const p = prioridad(d, hoy);
            return (
              <TR key={d.id}>
                <CeldaProveedor d={d} mostrarDepto />
                <TD>
                  <Badge tone={TONO_ESTADO[d.estado]}>{ETIQUETA_ESTADO[d.estado]}</Badge>
                </TD>
                <TD className="text-xs">
                  <span className={p.cls}>{p.label}</span>
                  <span className="block text-[10.5px] text-content-muted">
                    {d.fecha_vencimiento ? formatFecha(d.fecha_vencimiento) : "sin fecha"}
                  </span>
                </TD>
                <TD className="text-xs">
                  {d.prioridad ? (
                    <Badge tone={d.prioridad === "AA" ? "negativo" : "pendiente"}>{d.prioridad}</Badge>
                  ) : (
                    <span className="text-content-muted">—</span>
                  )}
                </TD>
                <CeldaMonto d={d} />
                <TD className="text-right">
                  <Button size="sm" variant="ghost" onClick={() => onVer(d.id)}>
                    Ver
                  </Button>
                </TD>
              </TR>
            );
          })}
        </TBody>
      </Table>
    </TableContainer>
  );
}

function TabArchivo({ items, onVer }: { items: Documento[]; onVer: (id: string) => void }) {
  return (
    <TableContainer>
      <Table>
        <THead>
          <TR>
            <TH>Proveedor</TH>
            <TH>Tipo</TH>
            <TH>Resultado</TH>
            <TH>Motivo</TH>
            <TH className="text-right">Monto</TH>
            <TH className="text-right"></TH>
          </TR>
        </THead>
        <TBody>
          {items.map((d) => (
            <TR key={d.id}>
              <CeldaProveedor d={d} mostrarDepto />
              <TD>
                <span className="rounded-full bg-surface-muted px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-content-muted">
                  {ETIQUETA_TIPO[d.tipo] ?? d.tipo}
                </span>
              </TD>
              <TD>
                <Badge tone={TONO_ESTADO[d.estado]}>{ETIQUETA_ESTADO[d.estado]}</Badge>
              </TD>
              <TD className="max-w-72 text-sm text-content-muted">
                <span className="line-clamp-2" title={d.nota_revision}>{d.nota_revision || "—"}</span>
              </TD>
              <CeldaMonto d={d} />
              <TD className="text-right">
                <Button size="sm" variant="ghost" onClick={() => onVer(d.id)}>
                  Ver
                </Button>
              </TD>
            </TR>
          ))}
        </TBody>
      </Table>
    </TableContainer>
  );
}
