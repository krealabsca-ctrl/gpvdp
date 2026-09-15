/**
 * CxP — Responsabilidades (/cxp/responsabilidades).
 *
 * Lo que se paga todos los meses: alquiler, pólizas, servicios públicos, operaciones bancarias y
 * los acuerdos donde nunca va a haber factura.
 *
 * POR QUÉ EXISTE, MEDIDO: de 14 partidas de obligación con débito en el banco, 12 tienen CERO
 * facturas en Cuentas por pagar — cerca de ₡28 millones por mes que salen sin que nadie los vea
 * venir. La causa es estructural: los 4.542 documentos de CxP traen clave de Hacienda, sin una
 * sola excepción, así que un acuerdo de palabra HOY NO PUEDE EXISTIR en el sistema.
 *
 * Dos pestañas: «El mes» (qué vence, qué se cumplió, qué falta) y «Los acuerdos» (la lista
 * declarada, que se escribe una vez y dura años).
 */

import { useMemo, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
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
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatFecha, formatMoneda } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useAbrirMes,
  useCambiarEstadoResponsabilidad,
  useCerrarPeriodo,
  useCrearResponsabilidad,
  useActualizarResponsabilidad,
  useMesDeResponsabilidades,
  usePlanDelMes,
  useReabrirPeriodo,
  useResponsabilidades,
  useDepartamentos,
} from "@/features/cxp/hooks";
import { useClasificaciones } from "@/features/bancos/hooks";
import {
  ETIQUETA_PERIODICIDAD,
  ETIQUETA_RESPALDO,
  MESES,
  SEMAFORO,
  correrPeriodo,
  etiquetaPeriodo,
  tonoRespaldo,
} from "@/features/cxp/dominioResponsabilidad";
import type {
  PeriodoResponsabilidad,
  Periodicidad,
  Responsabilidad,
  ResponsabilidadInput,
  RespaldoTipo,
} from "@/api/cxp";

/** El período actual en el calendario de Costa Rica, sin depender del huso del navegador. */
function periodoActualCR(): string {
  const d = new Date(Date.now() - 6 * 60 * 60 * 1000);
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}


/**
 * Panel modal de la pantalla. No se usa ConfirmDialog porque ese componente es para CONFIRMAR
 * (título + lista de impacto en texto + sí/no), y acá hay formularios con catorce campos y
 * validación en vivo. Forzarlo habría significado meter JSX donde el componente espera strings.
 */
function Panel(props: {
  titulo: string;
  descripcion?: ReactNode;
  children: ReactNode;
  textoConfirmar: string;
  confirmarDeshabilitado?: boolean;
  pendiente?: boolean;
  tono?: "accent" | "peligro";
  onConfirmar: () => void;
  onCancelar: () => void;
}) {
  return createPortal(
    <div
      className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && props.onCancelar()}
      role="dialog"
      aria-modal="true"
    >
      <div className="flex w-full max-w-2xl flex-col gap-4 rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <div>
          <h2 className="text-lg font-semibold text-content">{props.titulo}</h2>
          {props.descripcion && <div className="mt-1 text-sm text-content-muted">{props.descripcion}</div>}
        </div>
        {props.children}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={props.onCancelar}>
            Cancelar
          </Button>
          <Button
            onClick={props.onConfirmar}
            loading={props.pendiente}
            disabled={props.confirmarDeshabilitado}
            className={props.tono === "peligro" ? "bg-negativo hover:bg-negativo/90" : undefined}
          >
            {props.textoConfirmar}
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

type Pestana = "mes" | "acuerdos";

export function ResponsabilidadesPage() {
  const tiene = useTienePermiso();
  const [pestana, setPestana] = useState<Pestana>("mes");
  const [periodo, setPeriodo] = useState(periodoActualCR());

  const puedeDeclarar = tiene("cxp.responsabilidades.declarar");
  const puedeAbrirMes = tiene("cxp.responsabilidades.abrir_mes");
  const puedeCerrar = tiene("cxp.responsabilidades.cerrar");

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Responsabilidades"
        description="Lo que se paga todos los meses: alquiler, pólizas, servicios, banca y los acuerdos sin factura."
      />

      <div className="flex gap-1 border-b border-border">
        {([
          ["mes", "El mes"],
          ["acuerdos", "Los acuerdos"],
        ] as const).map(([k, label]) => (
          <button
            key={k}
            onClick={() => setPestana(k)}
            className={cn(
              "-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors",
              pestana === k
                ? "border-accent text-accent"
                : "border-transparent text-content-muted hover:text-content",
            )}
          >
            {label}
          </button>
        ))}
      </div>

      {pestana === "mes" ? (
        <TabMes
          periodo={periodo}
          setPeriodo={setPeriodo}
          puedeAbrirMes={puedeAbrirMes}
          puedeCerrar={puedeCerrar}
        />
      ) : (
        <TabAcuerdos puedeDeclarar={puedeDeclarar} />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pestaña: El mes
// ---------------------------------------------------------------------------

function TabMes(props: {
  periodo: string;
  setPeriodo: (p: string) => void;
  puedeAbrirMes: boolean;
  puedeCerrar: boolean;
}) {
  const { periodo, setPeriodo } = props;
  const toast = useToast();
  const mesQ = useMesDeResponsabilidades(periodo);
  const [abriendo, setAbriendo] = useState(false);
  const [cerrando, setCerrando] = useState<PeriodoResponsabilidad | null>(null);
  const [reabriendo, setReabriendo] = useState<PeriodoResponsabilidad | null>(null);
  const reabrir = useReabrirPeriodo();
  const [motivoReapertura, setMotivoReapertura] = useState("");

  const resumen = mesQ.data?.resumen;
  const filas = mesQ.data?.filas ?? [];
  const sinAbrir = mesQ.data?.sin_abrir ?? [];

  return (
    <div className="flex flex-col gap-4">
      {/* Navegación del mes */}
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="secondary" onClick={() => setPeriodo(correrPeriodo(periodo, -1))}>
          ‹ Mes anterior
        </Button>
        <span className="min-w-44 text-center text-base font-semibold tabular-nums text-content">
          {etiquetaPeriodo(periodo)}
        </span>
        <Button size="sm" variant="secondary" onClick={() => setPeriodo(correrPeriodo(periodo, 1))}>
          Mes siguiente ›
        </Button>
        {periodo !== periodoActualCR() && (
          <Button size="sm" variant="ghost" onClick={() => setPeriodo(periodoActualCR())}>
            Volver a hoy
          </Button>
        )}
        {props.puedeAbrirMes && (
          <Button className="ml-auto" onClick={() => setAbriendo(true)}>
            Abrir el mes
          </Button>
        )}
      </div>

      {/* Encabezado con los conteos */}
      {resumen && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
          <Tarjeta titulo="Sin abrir" valor={resumen.sin_abrir} tono={resumen.sin_abrir > 0 ? "negativo" : "neutral"} />
          <Tarjeta titulo="Vencidas" valor={resumen.vencida} tono={resumen.vencida > 0 ? "negativo" : "neutral"} />
          <Tarjeta titulo="Por vencer" valor={resumen.por_vencer} tono="pendiente" />
          <Tarjeta titulo="Sin dato" valor={resumen.sin_dato} tono="neutral" />
          <Tarjeta titulo="Cumplidas" valor={resumen.cumplida} tono="positivo" />
          <Tarjeta
            titulo="Esperado del mes"
            valor={formatMoneda(resumen.monto_esperado, "CRC")}
            tono="accent"
          />
        </div>
      )}

      {/* La honestidad sobre los datos del banco. Sin esto, un «SIN DATO» se lee como un error del
          sistema en vez de como una carga pendiente. */}
      {resumen && resumen.sin_dato > 0 && (
        <p className="rounded-lg border border-border bg-surface-raised px-3 py-2 text-sm text-content-muted">
          ⓘ{" "}
          {resumen.sin_dato === 1
            ? "Una responsabilidad venció y el sistema todavía no puede decir si se pagó"
            : `${resumen.sin_dato} responsabilidades vencieron y el sistema todavía no puede decir si se pagaron`}
          :{" "}
          {resumen.banco_hasta
            ? `los movimientos del banco están cargados hasta el ${formatFecha(resumen.banco_hasta)}.`
            : "no hay movimientos del banco importados."}{" "}
          Se dice «sin dato» a propósito, en vez de acusar un vencimiento que podría no existir.
        </p>
      )}

      {/* Lo que nadie abrió: el olvido del olvido. */}
      {sinAbrir.length > 0 && (
        <div className="rounded-lg border-2 border-negativo/40 bg-negativo/5 px-4 py-3">
          <p className="text-sm font-semibold text-negativo">
            {sinAbrir.length} {sinAbrir.length === 1 ? "responsabilidad no tiene" : "responsabilidades no tienen"} su fila
            de {etiquetaPeriodo(periodo).toLowerCase()}
          </p>
          <p className="mt-1 text-xs text-content-muted">
            Existen y les toca este mes, pero nadie abrió el mes todavía. Mientras no se abra, no van a figurar como
            vencidas aunque lo estén — y el mes se vería tranquilo sin serlo.
          </p>
          <p className="mt-2 text-sm text-content">{sinAbrir.map((r) => r.nombre).join(" · ")}</p>
        </div>
      )}

      {mesQ.isPending ? (
        <LoadingState label="Cargando el mes" />
      ) : mesQ.isError ? (
        <ErrorState message={mensajeError(mesQ.error)} onRetry={() => mesQ.refetch()} />
      ) : filas.length === 0 ? (
        <EmptyState
          message={
            sinAbrir.length > 0
              ? "El mes no está abierto todavía."
              : "No hay responsabilidades para este mes."
          }
        />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Responsabilidad</TH>
                <TH>Contraparte</TH>
                <TH>Respaldo</TH>
                <TH>Vence</TH>
                <TH className="text-right">Monto esperado</TH>
                <TH>Estado</TH>
                <TH>Responsable</TH>
                <TH className="text-right">Acción</TH>
              </TR>
            </THead>
            <TBody>
              {filas.map((f) => {
                const sem = SEMAFORO[f.semaforo];
                return (
                  <TR key={f.id}>
                    <TD>
                      <span className="font-medium text-content">{f.nombre}</span>
                      {!f.deducible && (
                        <Badge tone="negativo" className="ml-2">
                          No deducible
                        </Badge>
                      )}
                    </TD>
                    <TD className="text-content-muted">{f.contraparte}</TD>
                    <TD>
                      <Badge tone={tonoRespaldo(f.respaldo_tipo)}>
                        {ETIQUETA_RESPALDO[f.respaldo_tipo ?? "NINGUNO"]}
                      </Badge>
                    </TD>
                    <TD className="tabular-nums">
                      {formatFecha(f.vence_en)}
                      {f.estado === "PENDIENTE" && f.dias_de_atraso > 0 && (
                        <span className="ml-1 text-xs text-negativo">+{f.dias_de_atraso}d</span>
                      )}
                    </TD>
                    <TD className="text-right tabular-nums">
                      {formatMoneda(f.monto_esperado, f.moneda)}
                      {f.monto_tipo === "VARIABLE" && (
                        <span className="ml-1 text-xs text-content-muted" title="Monto de consumo: es una referencia, no autoriza un pago">
                          aprox.
                        </span>
                      )}
                    </TD>
                    <TD>
                      <Badge tone={sem.tono} title={sem.ayuda}>
                        {sem.etiqueta}
                      </Badge>
                      {f.estado === "NO_APLICA" && f.motivo && (
                        <p className="mt-1 max-w-48 text-xs text-content-muted">{f.motivo}</p>
                      )}
                    </TD>
                    <TD className="text-content-muted">
                      {f.titular_nombre || <span className="text-negativo">sin dueño</span>}
                    </TD>
                    <TD className="text-right">
                      {props.puedeCerrar &&
                        (f.estado === "PENDIENTE" ? (
                          <Button size="sm" variant="secondary" onClick={() => setCerrando(f)}>
                            Resolver
                          </Button>
                        ) : (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              setMotivoReapertura("");
                              setReabriendo(f);
                            }}
                          >
                            Reabrir
                          </Button>
                        ))}
                    </TD>
                  </TR>
                );
              })}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {abriendo && <DialogoAbrirMes periodo={periodo} onCerrar={() => setAbriendo(false)} />}
      {cerrando && <DialogoResolver fila={cerrando} onCerrar={() => setCerrando(null)} />}
      {reabriendo && (
        <Panel
          titulo={`Reabrir ${reabriendo.nombre}`}
          descripcion="Vuelve a quedar pendiente y se borra la prueba que tenía. Queda registrado quién lo reabrió y por qué."
          children={
            <Input
              label="¿Por qué se reabre?"
              value={motivoReapertura}
              onChange={(e) => setMotivoReapertura(e.target.value)}
              placeholder="el acuse era de otro mes"
            />
          }
          textoConfirmar="Reabrir"
          onConfirmar={() => {
            const fila = reabriendo;
            reabrir.mutate(
              { id: fila.id, motivo: motivoReapertura },
              {
                onSuccess: () => {
                  toast.success(`${fila.nombre} volvió a pendiente`);
                  setReabriendo(null);
                },
                onError: (e) => toast.error(mensajeError(e)),
              },
            );
          }}
          onCancelar={() => setReabriendo(null)}
        />
      )}
    </div>
  );
}

function Tarjeta(props: { titulo: string; valor: number | string; tono: "neutral" | "negativo" | "pendiente" | "positivo" | "accent" }) {
  const color = {
    neutral: "text-content",
    negativo: "text-negativo",
    pendiente: "text-pendiente",
    positivo: "text-positivo",
    accent: "text-accent",
  }[props.tono];
  return (
    <div className="rounded-xl border border-border bg-surface-raised px-4 py-3">
      <p className="text-[11px] font-bold uppercase tracking-wider text-content-muted">{props.titulo}</p>
      <p className={cn("mt-1 text-xl font-semibold tabular-nums", color)}>{props.valor}</p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Abrir el mes: primero el plan, después la confirmación
// ---------------------------------------------------------------------------

function DialogoAbrirMes(props: { periodo: string; onCerrar: () => void }) {
  const toast = useToast();
  const planQ = usePlanDelMes(props.periodo, true);
  const abrir = useAbrirMes();
  const plan = planQ.data;

  return (
    <Panel
      titulo={`Abrir ${etiquetaPeriodo(props.periodo).toLowerCase()}`}
      descripcion="Crea la fila de este mes para cada responsabilidad que corresponda. Abrirlo dos veces no duplica nada."
      children={
        planQ.isPending ? (
          <LoadingState label="Calculando el plan" />
        ) : planQ.isError ? (
          <ErrorState message={mensajeError(planQ.error)} onRetry={() => planQ.refetch()} />
        ) : plan ? (
          <div className="flex flex-col gap-3 text-sm">
            <p className="text-content">
              Va a crear <b className="tabular-nums">{plan.va_a_crear}</b>{" "}
              {plan.va_a_crear === 1 ? "responsabilidad" : "responsabilidades"} por{" "}
              <b className="tabular-nums">{formatMoneda(plan.monto_esperado, "CRC")}</b>.
              {plan.ya_estaban > 0 && (
                <span className="text-content-muted"> ({plan.ya_estaban} ya estaban abiertas.)</span>
              )}
            </p>
            {/* Lo que queda afuera, explicado por razón: «38 de 41» sin decir qué pasó con las 3
                es la clase de silencio que hace que nadie confíe en el número. */}
            {plan.afuera.length > 0 && (
              <div className="rounded-lg border border-border bg-surface-muted px-3 py-2">
                <p className="text-xs font-semibold text-content">Queda afuera:</p>
                <ul className="mt-1 flex flex-col gap-1">
                  {plan.afuera.map((a) => (
                    <li key={a.razon} className="text-xs text-content-muted">
                      <b className="text-content">{a.cuantas}</b> — {a.razon}:{" "}
                      {a.nombres.slice(0, 4).join(", ")}
                      {a.nombres.length > 4 && ` y ${a.nombres.length - 4} más`}
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {plan.va_a_crear === 0 && (
              <p className="text-content-muted">No hay nada nuevo que abrir en este mes.</p>
            )}
          </div>
        ) : null
      }
      textoConfirmar={plan && plan.va_a_crear > 0 ? `Abrir ${plan.va_a_crear}` : "Abrir"}
      onConfirmar={() => {
        if (!plan) return;
        // Se manda el total que la persona VIO: si alguien declaró algo en el medio, el servidor
        // se detiene. Uno confirma un total, no una intención.
        abrir.mutate(
          { periodo: props.periodo, esperadas: plan.va_a_crear },
          {
            onSuccess: (r) => {
              toast.success(
                r.va_a_crear === 0
                  ? "El mes ya estaba abierto: no se creó nada."
                  : r.va_a_crear === 1
                    ? "Se abrió 1 responsabilidad."
                    : `Se abrieron ${r.va_a_crear} responsabilidades.`,
              );
              props.onCerrar();
            },
            onError: (e) => toast.error(mensajeError(e)),
          },
        );
      }}
      onCancelar={props.onCerrar}
    />
  );
}

// ---------------------------------------------------------------------------
// Resolver un mes
// ---------------------------------------------------------------------------

function DialogoResolver(props: { fila: PeriodoResponsabilidad; onCerrar: () => void }) {
  const toast = useToast();
  const cerrar = useCerrarPeriodo();
  const [modo, setModo] = useState<"ACUSE" | "NO_APLICA">("ACUSE");
  const [acuse, setAcuse] = useState("");
  const [motivo, setMotivo] = useState("");

  return (
    <Panel
      titulo={`Resolver ${props.fila.nombre}`}
      descripcion={`${etiquetaPeriodo(props.fila.periodo)} · vence el ${formatFecha(props.fila.vence_en)}`}
      children={
        <div className="flex flex-col gap-3">
          <Select
            label="¿Qué pasó con este mes?"
            value={modo}
            onChange={(e) => setModo(e.target.value as "ACUSE" | "NO_APLICA")}
            options={[
              { value: "ACUSE", label: "Se cumplió — adjunto el comprobante" },
              { value: "NO_APLICA", label: "Este mes no aplicaba" },
            ]}
          />
          {modo === "ACUSE" ? (
            <Input
              label="Comprobante"
              value={acuse}
              onChange={(e) => setAcuse(e.target.value)}
              placeholder="transferencia-setiembre.pdf"
            />
          ) : (
            <Input
              label="¿Por qué no aplicaba?"
              value={motivo}
              onChange={(e) => setMotivo(e.target.value)}
              placeholder="el local estuvo cerrado todo el mes"
            />
          )}
          {/* El freno explicado, no solo aplicado. */}
          <p className="text-xs text-content-muted">
            {modo === "NO_APLICA"
              ? "El motivo es obligatorio: sin él, «no aplica» se vuelve el botón de tapar el olvido."
              : "Enlazar la factura o el débito del banco que lo prueba llega en la próxima etapa; por ahora se adjunta el comprobante."}
          </p>
        </div>
      }
      textoConfirmar="Guardar"
      onConfirmar={() => {
        cerrar.mutate(
          {
            id: props.fila.id,
            cierre:
              modo === "NO_APLICA"
                ? { estado: "NO_APLICA", motivo }
                : { estado: "CUMPLIDA", cumplida_con: "ACUSE", acuse_archivo: acuse },
          },
          {
            onSuccess: () => {
              toast.success(`${props.fila.nombre}: ${modo === "NO_APLICA" ? "marcada como no aplicable" : "cumplida"}`);
              props.onCerrar();
            },
            onError: (e) => toast.error(mensajeError(e)),
          },
        );
      }}
      onCancelar={props.onCerrar}
    />
  );
}

// ---------------------------------------------------------------------------
// Pestaña: Los acuerdos
// ---------------------------------------------------------------------------

const VACIO: ResponsabilidadInput = {
  nombre: "",
  contraparte: "",
  dia_vencimiento: 1,
  periodicidad: "MENSUAL",
  moneda: "CRC",
  monto_esperado: "0",
  monto_tipo: "FIJO",
  respaldo_tipo: "NINGUNO",
  espera_factura: true,
  deducible: false,
};

function TabAcuerdos(props: { puedeDeclarar: boolean }) {
  const toast = useToast();
  const [q, setQ] = useState("");
  const [estadoFiltro, setEstadoFiltro] = useState("ACTIVA");
  const lista = useResponsabilidades({
    q: q || undefined,
    estado: (estadoFiltro || undefined) as "ACTIVA" | "SUSPENDIDA" | "FINALIZADA" | undefined,
  });
  const [form, setForm] = useState<ResponsabilidadInput | null>(null);
  const [editId, setEditId] = useState<string | null>(null);
  const [suspendiendo, setSuspendiendo] = useState<Responsabilidad | null>(null);
  const [motivoSuspension, setMotivoSuspension] = useState("");

  const crear = useCrearResponsabilidad();
  const actualizar = useActualizarResponsabilidad();
  const cambiarEstado = useCambiarEstadoResponsabilidad();

  const items = lista.data ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end gap-3">
        <Input label="Buscar" value={q} onChange={(e) => setQ(e.target.value)} placeholder="nombre o contraparte" className="min-w-56" />
        <Select
          label="Estado"
          value={estadoFiltro}
          onChange={(e) => setEstadoFiltro(e.target.value)}
          options={[
            { value: "ACTIVA", label: "Activas" },
            { value: "SUSPENDIDA", label: "Suspendidas" },
            { value: "FINALIZADA", label: "Finalizadas" },
            { value: "", label: "Todas" },
          ]}
          className="min-w-40"
        />
        {props.puedeDeclarar && (
          <Button
            className="ml-auto"
            onClick={() => {
              setEditId(null);
              setForm({ ...VACIO });
            }}
          >
            Declarar una responsabilidad
          </Button>
        )}
      </div>

      {lista.isPending ? (
        <LoadingState label="Cargando los acuerdos" />
      ) : lista.isError ? (
        <ErrorState message={mensajeError(lista.error)} onRetry={() => lista.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState message="Todavía no hay responsabilidades declaradas.">
          <p className="max-w-lg text-sm text-content-muted">
            Acá se escribe una sola vez lo que la empresa paga todos los meses. Mientras la lista esté vacía, el sistema
            no puede avisar de nada: solo conoce las facturas que llegaron.
          </p>
        </EmptyState>
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Responsabilidad</TH>
                <TH>Contraparte</TH>
                <TH>Cuándo</TH>
                <TH className="text-right">Monto</TH>
                <TH>Respaldo</TH>
                <TH>Partida</TH>
                <TH>Titular</TH>
                <TH className="text-right">Acción</TH>
              </TR>
            </THead>
            <TBody>
              {items.map((r) => (
                <TR key={r.id}>
                  <TD>
                    <span className="font-medium text-content">{r.nombre}</span>
                    {r.estado !== "ACTIVA" && (
                      <Badge tone="neutral" className="ml-2">
                        {r.estado === "SUSPENDIDA" ? "Suspendida" : "Finalizada"}
                      </Badge>
                    )}
                    {r.tipo === "TRAMITE" && (
                      <Badge tone="accent" className="ml-2">
                        Trámite
                      </Badge>
                    )}
                  </TD>
                  <TD className="text-content-muted">
                    {r.contraparte}
                    {!r.espera_factura && (
                      <span className="ml-1 text-xs text-content-muted" title="Se declaró que no va a llegar comprobante">
                        (sin factura)
                      </span>
                    )}
                  </TD>
                  <TD className="text-content-muted">
                    {ETIQUETA_PERIODICIDAD[r.periodicidad]} · día {r.dia_vencimiento}
                  </TD>
                  <TD className="text-right tabular-nums">
                    {formatMoneda(r.monto_esperado, r.moneda)}
                    {r.monto_tipo === "VARIABLE" && <span className="ml-1 text-xs text-content-muted">aprox.</span>}
                  </TD>
                  <TD>
                    <Badge tone={tonoRespaldo(r.respaldo_tipo)}>{ETIQUETA_RESPALDO[r.respaldo_tipo]}</Badge>
                    {!r.deducible && (
                      <Badge tone="negativo" className="ml-1">
                        No deducible
                      </Badge>
                    )}
                  </TD>
                  <TD className="text-content-muted">{r.clasificacion_nombre || "—"}</TD>
                  <TD className="text-content-muted">
                    {r.titular_nombre || <span className="text-negativo">sin dueño</span>}
                    {r.suplente_nombre && <span className="text-xs"> / {r.suplente_nombre}</span>}
                  </TD>
                  <TD className="text-right">
                    {props.puedeDeclarar && (
                      <div className="flex justify-end gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => {
                            setEditId(r.id);
                            setForm({
                              nombre: r.nombre,
                              contraparte: r.contraparte,
                              proveedor_id: r.proveedor_id,
                              tipo: r.tipo,
                              periodicidad: r.periodicidad,
                              dia_vencimiento: r.dia_vencimiento,
                              mes_ancla: r.mes_ancla,
                              moneda: r.moneda,
                              monto_esperado: r.monto_esperado,
                              monto_tipo: r.monto_tipo,
                              respaldo_tipo: r.respaldo_tipo,
                              respaldo_archivo: r.respaldo_archivo,
                              espera_factura: r.espera_factura,
                              deducible: r.deducible,
                              clasificacion_id: r.clasificacion_id,
                              departamento_id: r.departamento_id,
                              notas: r.notas,
                              titular_id: r.titular_id,
                              suplente_id: r.suplente_id,
                            });
                          }}
                        >
                          Editar
                        </Button>
                        {r.estado === "ACTIVA" ? (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              setMotivoSuspension("");
                              setSuspendiendo(r);
                            }}
                          >
                            Suspender
                          </Button>
                        ) : (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() =>
                              cambiarEstado.mutate(
                                { id: r.id, estado: "ACTIVA", motivo: "" },
                                {
                                  onSuccess: () => toast.success(`${r.nombre} vuelve al calendario`),
                                  onError: (e) => toast.error(mensajeError(e)),
                                },
                              )
                            }
                          >
                            Reactivar
                          </Button>
                        )}
                      </div>
                    )}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {form && (
        <FormularioResponsabilidad
          valor={form}
          editando={!!editId}
          onCambiar={setForm}
          guardando={crear.isPending || actualizar.isPending}
          onGuardar={() => {
            const cb = {
              onSuccess: () => {
                toast.success(editId ? "Responsabilidad actualizada" : "Responsabilidad declarada");
                setForm(null);
                setEditId(null);
              },
              onError: (e: unknown) => toast.error(mensajeError(e)),
            };
            if (editId) actualizar.mutate({ id: editId, input: form }, cb);
            else crear.mutate(form, cb);
          }}
          onCancelar={() => {
            setForm(null);
            setEditId(null);
          }}
        />
      )}

      {suspendiendo && (
        <Panel
          titulo={`Suspender ${suspendiendo.nombre}`}
          descripcion="Deja de aparecer en el calendario a partir del próximo mes que se abra."
          children={
            <div className="flex flex-col gap-2">
              {/* Esto se dice explícitamente porque es el riesgo real del permiso: suspender
                  permite TAPAR el olvido en vez de cumplirlo. */}
              <p className="text-sm text-content-muted">
                Suspender saca la responsabilidad del calendario. Queda registrado quién lo hizo y por qué.
              </p>
              <Input
                label="¿Por qué se suspende?"
                value={motivoSuspension}
                onChange={(e) => setMotivoSuspension(e.target.value)}
                placeholder="se cerró esa sede"
              />
            </div>
          }
          textoConfirmar="Suspender"
          tono="peligro"
          onConfirmar={() => {
            const r = suspendiendo;
            cambiarEstado.mutate(
              { id: r.id, estado: "SUSPENDIDA", motivo: motivoSuspension },
              {
                onSuccess: () => {
                  toast.success(`${r.nombre} quedó suspendida`);
                  setSuspendiendo(null);
                },
                onError: (e) => toast.error(mensajeError(e)),
              },
            );
          }}
          onCancelar={() => setSuspendiendo(null)}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Formulario de un acuerdo
// ---------------------------------------------------------------------------

function FormularioResponsabilidad(props: {
  valor: ResponsabilidadInput;
  editando: boolean;
  guardando: boolean;
  onCambiar: (v: ResponsabilidadInput) => void;
  onGuardar: () => void;
  onCancelar: () => void;
}) {
  const { valor: v } = props;
  const set = (parcial: Partial<ResponsabilidadInput>) => props.onCambiar({ ...v, ...parcial });
  const clasifQ = useClasificaciones();
  const deptoQ = useDepartamentos(true);

  const sinComprobante = v.respaldo_tipo === "VERBAL" || v.respaldo_tipo === "NINGUNO";
  const necesitaArchivo = v.respaldo_tipo === "CONTRATO" || v.respaldo_tipo === "ACTA" || v.respaldo_tipo === "CORREO";

  // Los frenos se explican ANTES de apretar el botón, no como un error después.
  const problema = useMemo(() => {
    if (!v.nombre.trim()) return "Falta el nombre.";
    if (!v.contraparte.trim()) return "Falta la contraparte: a quién se le paga.";
    if (v.periodicidad !== "MENSUAL" && !v.mes_ancla) return "Una responsabilidad que no es mensual necesita su mes de arranque.";
    if (necesitaArchivo && !v.respaldo_archivo?.trim()) return "Si hay contrato, acta o correo, hay que adjuntarlo.";
    return "";
  }, [v, necesitaArchivo]);

  return (
    <Panel
      titulo={props.editando ? "Editar la responsabilidad" : "Declarar una responsabilidad"}
      descripcion="Se escribe una vez y rige todos los meses. El monto nuevo aplica de acá en adelante: los meses ya abiertos conservan el suyo."
      children={
        <div className="grid max-h-[60vh] grid-cols-1 gap-3 overflow-y-auto pr-1 sm:grid-cols-2">
          <Input label="Nombre" value={v.nombre} onChange={(e) => set({ nombre: e.target.value })} placeholder="Alquiler sede Alajuela" />
          <Input
            label="Contraparte"
            value={v.contraparte}
            onChange={(e) => set({ contraparte: e.target.value })}
            placeholder="a quién se le paga"
          />

          <Select
            label="Tipo"
            value={v.tipo ?? "PAGO"}
            onChange={(e) => set({ tipo: e.target.value as "PAGO" | "TRAMITE" })}
            options={[
              { value: "PAGO", label: "Pago — sale plata" },
              { value: "TRAMITE", label: "Trámite — hay que presentar o renovar algo" },
            ]}
          />
          <Select
            label="Cada cuánto"
            value={v.periodicidad ?? "MENSUAL"}
            onChange={(e) => set({ periodicidad: e.target.value as Periodicidad })}
            options={(Object.keys(ETIQUETA_PERIODICIDAD) as Periodicidad[]).map((k) => ({
              value: k,
              label: ETIQUETA_PERIODICIDAD[k],
            }))}
          />

          <Input
            label="Día del mes en que vence"
            type="number"
            min={1}
            max={31}
            value={String(v.dia_vencimiento)}
            onChange={(e) => set({ dia_vencimiento: Number(e.target.value) })}
          />
          {v.periodicidad !== "MENSUAL" ? (
            <Select
              label="Mes de arranque del ciclo"
              value={String(v.mes_ancla ?? "")}
              onChange={(e) => set({ mes_ancla: Number(e.target.value) })}
              options={[{ value: "", label: "Elegí un mes" }, ...MESES.map((m, i) => ({ value: String(i + 1), label: m }))]}
            />
          ) : (
            <div className="flex items-end pb-2 text-xs text-content-muted">
              El día 31 en un mes que no lo tiene cae automáticamente al último día.
            </div>
          )}

          <Input
            label="Monto esperado"
            value={v.monto_esperado ?? ""}
            onChange={(e) => set({ monto_esperado: e.target.value })}
            inputMode="decimal"
            placeholder="735000.00"
          />
          <Select
            label="¿El monto es fijo o varía?"
            value={v.monto_tipo ?? "FIJO"}
            onChange={(e) => set({ monto_tipo: e.target.value as "FIJO" | "VARIABLE" })}
            options={[
              { value: "FIJO", label: "Fijo — alquiler, póliza, internet" },
              { value: "VARIABLE", label: "Varía — agua, luz, combustible" },
            ]}
          />

          <Select
            label="Respaldo"
            value={v.respaldo_tipo ?? "NINGUNO"}
            onChange={(e) => {
              const t = e.target.value as RespaldoTipo;
              // La regla fiscal se aplica al elegir, no como un error después: sin comprobante el
              // gasto no es deducible, y decirlo tarde obliga a rehacer el formulario.
              set({ respaldo_tipo: t, deducible: t === "VERBAL" || t === "NINGUNO" ? false : v.deducible });
            }}
            options={(Object.keys(ETIQUETA_RESPALDO) as RespaldoTipo[]).map((k) => ({
              value: k,
              label: ETIQUETA_RESPALDO[k],
            }))}
          />
          <Input
            label={necesitaArchivo ? "Archivo del respaldo (obligatorio)" : "Archivo del respaldo"}
            value={v.respaldo_archivo ?? ""}
            onChange={(e) => set({ respaldo_archivo: e.target.value })}
            placeholder="contrato-alquiler-2026.pdf"
            disabled={sinComprobante}
          />

          <Select
            label="Partida (gasto)"
            value={v.clasificacion_id ?? ""}
            onChange={(e) => set({ clasificacion_id: e.target.value })}
            options={[
              { value: "", label: "Sin partida" },
              ...(clasifQ.data ?? []).map((c) => ({ value: c.id, label: c.nombre, grupo: c.concepto })),
            ]}
          />
          <Select
            label="Departamento"
            value={v.departamento_id ?? ""}
            onChange={(e) => set({ departamento_id: e.target.value })}
            options={[
              { value: "", label: "Sin departamento" },
              ...(deptoQ.data ?? []).map((d) => ({ value: d.id, label: d.nombre })),
            ]}
          />

          <div className="sm:col-span-2 flex flex-col gap-2 rounded-lg border border-border bg-surface-muted px-3 py-2">
            <label className="flex items-center gap-2 text-sm text-content">
              <input
                type="checkbox"
                checked={v.espera_factura ?? true}
                onChange={(e) => set({ espera_factura: e.target.checked })}
              />
              Va a llegar una factura por esto
            </label>
            <label className={cn("flex items-center gap-2 text-sm", sinComprobante ? "text-content-muted" : "text-content")}>
              <input
                type="checkbox"
                checked={v.deducible ?? false}
                disabled={sinComprobante}
                onChange={(e) => set({ deducible: e.target.checked })}
              />
              Es un gasto deducible
            </label>
            {sinComprobante && (
              <p className="text-xs text-content-muted">
                Sin comprobante el gasto <b>no es deducible</b>: en Costa Rica Hacienda no lo acepta. Por eso la casilla
                queda desactivada mientras el respaldo sea «de palabra» o «sin respaldo».
              </p>
            )}
          </div>

          <Input
            label="Notas"
            value={v.notas ?? ""}
            onChange={(e) => set({ notas: e.target.value })}
            className="sm:col-span-2"
            placeholder="número de medidor, número de póliza, referencia del contrato…"
          />

          {problema && <p className="sm:col-span-2 text-sm text-negativo">{problema}</p>}
        </div>
      }
      textoConfirmar={props.editando ? "Guardar los cambios" : "Declarar"}
      confirmarDeshabilitado={!!problema || props.guardando}
      onConfirmar={props.onGuardar}
      onCancelar={props.onCancelar}
    />
  );
}
