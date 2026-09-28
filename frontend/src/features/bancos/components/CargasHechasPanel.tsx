/**
 * «Cargas hechas» — el historial de importaciones y la reversa de una carga (mig 0085).
 *
 * Existe por un incidente real: el mismo archivo se importó en la cuenta equivocada y esa plata
 * quedó contada dos veces. La regla que dio el Director Financiero es textual: «debe permitirme
 * excluir todo lo que dupliqué sin malograr lo que está bien; del día 20 hacia atrás todo está bien
 * y el 21 agregué mal los bancos, lo que debo corregir es lo cargado el 21 no lo que ya estaba
 * cargado el 20». De ahí las dos decisiones de esta pantalla:
 *
 *  · La unidad que se revierte es LA CARGA, no la fecha ni la cuenta. Revertir la carga del 21 no
 *    toca ni una fila de la del 20, aunque las dos vivan en la misma cuenta y el mismo mes.
 *  · Y «la solución no debe ser por código»: se hace desde acá, sin SQL y sin programador.
 *
 * Lo que NO hace: borrar. Revertir marca los movimientos como no incluidos —dejan de sumar— y deja
 * el motivo y quién lo hizo a la vista. Por eso una carga revertida se sigue viendo en la lista:
 * esconderla le quitaría a quien corrigió la única forma de verificar qué marcó.
 *
 * El servidor decide si el botón va habilitado (`puede_revertir` / `puede_deshacer_reversa`) y
 * redacta el porqué (`razon_no_revertir`). Acá no se re-deriva nada de `bloqueos`: si la pantalla
 * opinara distinto, el usuario apretaría un botón vivo para recibir un 422.
 */

import { useState, type ReactNode } from "react";
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
  Paginador,
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
import { formatFecha, formatFechaHora, formatMoneda } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useDeshacerReversaImportacion,
  useImportaciones,
  useRevertirImportacion,
} from "@/features/bancos/hooks";
import type { CuentaBancaria, EstadoImportacion, ImportacionItem } from "@/api/bancos";

/**
 * Qué significa cada estado, dicho por su consecuencia y no por el nombre técnico: lo que importa
 * al mirar la lista es si esa carga está sumando plata o no.
 */
const ESTADO: Record<EstadoImportacion, { label: string; tone: BadgeTone }> = {
  CARGADA: { label: "Subida, sin confirmar", tone: "neutral" },
  PREVISUALIZADA: { label: "Vista previa, sin confirmar", tone: "neutral" },
  CONFIRMADA: { label: "En los libros", tone: "positivo" },
  CERRADA: { label: "Cerrada", tone: "accent" },
  REVERTIDA: { label: "Revertida", tone: "negativo" },
};

/** La cuenta como se nombra en toda la pantalla: «Promerica · Promerica Colinas». */
function nombreCuenta(it: ImportacionItem): string {
  return it.cuenta_alias ? `${it.banco} · ${it.cuenta_alias}` : it.banco;
}

/** El rango de fechas de los movimientos que trajo la carga («—» si no trajo ninguno). */
function rangoDeFechas(it: ImportacionItem): string {
  if (!it.fecha_desde) return "—";
  if (it.fecha_desde === it.fecha_hasta) return formatFecha(it.fecha_desde);
  return `${formatFecha(it.fecha_desde)} – ${formatFecha(it.fecha_hasta)}`;
}

/**
 * La frase que describe la plata de una carga. La usan los dos diálogos, y por eso está acá: el de
 * revertir y el de deshacer tienen que decir EXACTAMENTE los mismos números.
 */
function frasePlata(it: ImportacionItem): string {
  return (
    `${it.movimientos} ${it.movimientos === 1 ? "movimiento" : "movimientos"} por ` +
    `${formatMoneda(it.total_debitos, it.moneda)} en débitos y ` +
    `${formatMoneda(it.total_creditos, it.moneda)} en créditos, ` +
    `del ${formatFecha(it.fecha_desde)} al ${formatFecha(it.fecha_hasta)} en ${nombreCuenta(it)}`
  );
}

export interface CargasHechasPanelProps {
  /** Las cuentas de la empresa, para el filtro. Vienen de la página para no pedirlas dos veces. */
  cuentas: CuentaBancaria[];
}

export function CargasHechasPanel({ cuentas }: CargasHechasPanelProps) {
  const tienePermiso = useTienePermiso();
  // Gating como el resto del cliente: si el rol no puede revertir, el botón no se dibuja. El
  // historial sí lo ve quien importa, que es quien necesita verificar qué subió.
  const puedeRevertir = tienePermiso("bancos.revertir_importacion");

  const [cuentaId, setCuentaId] = useState("");
  const [pagina, setPagina] = useState(1);
  const [porPagina, setPorPagina] = useState(50);
  const [aRevertir, setARevertir] = useState<ImportacionItem | null>(null);
  const [aDeshacer, setADeshacer] = useState<ImportacionItem | null>(null);

  const query = useImportaciones({
    cuenta_bancaria_id: cuentaId || undefined,
    page: pagina,
    page_size: porPagina,
  });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  const opcionesCuenta = [
    { value: "", label: "Todas las cuentas" },
    ...cuentas.map((c) => ({ value: c.id, label: `${c.alias} · ${c.banco} (${c.moneda})` })),
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Cargas hechas</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <p className="text-sm text-content-muted">
          Cada archivo que se subió, la más reciente arriba. Si una carga entró en la cuenta
          equivocada o quedó duplicada, se revierte entera desde acá:{" "}
          <span className="font-medium text-content">
            se excluye solo lo que trajo ESE archivo
          </span>
          , las demás cargas de la cuenta no se tocan.
        </p>

        <div className="max-w-sm">
          {/* No se llama «Cuenta bancaria» a propósito: arriba, en el paso 1, ya hay un
              desplegable con ese nombre y es el que ELIGE dónde cargar. Este solo filtra la
              lista, y dos etiquetas iguales en la misma pantalla se confunden. */}
          <Select
            label="Ver las cargas de"
            value={cuentaId}
            onChange={(e) => {
              setCuentaId(e.target.value);
              setPagina(1); // cambiar el filtro con la página 5 puesta deja la lista en blanco
            }}
            options={opcionesCuenta}
          />
        </div>

        {query.isPending ? (
          <LoadingState label="Cargando las cargas hechas" />
        ) : query.isError ? (
          <ErrorState message={mensajeError(query.error)} onRetry={() => query.refetch()} />
        ) : items.length === 0 ? (
          <EmptyState
            message={
              cuentaId
                ? "Esta cuenta todavía no tiene ninguna carga."
                : "Todavía no se subió ningún archivo de banco en esta empresa."
            }
          />
        ) : (
          <>
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Cuándo y quién</TH>
                    <TH>Cuenta</TH>
                    <TH>Archivo</TH>
                    <TH className="text-right">Movimientos</TH>
                    <TH>Fechas del archivo</TH>
                    <TH className="text-right">Débitos</TH>
                    <TH className="text-right">Créditos</TH>
                    <TH>Estado</TH>
                    <TH />
                  </TR>
                </THead>
                <TBody>
                  {items.map((it) => (
                    <FilaCarga
                      key={it.id}
                      it={it}
                      puedeRevertir={puedeRevertir}
                      onRevertir={() => setARevertir(it)}
                      onDeshacer={() => setADeshacer(it)}
                    />
                  ))}
                </TBody>
              </Table>
            </TableContainer>

            <Paginador
              total={total}
              pagina={pagina}
              porPagina={porPagina}
              enPantalla={items.length}
              onPagina={setPagina}
              onPorPagina={setPorPagina}
              etiqueta="cargas"
            />
          </>
        )}
      </CardContent>

      {aRevertir && <RevertirDialog it={aRevertir} onCerrar={() => setARevertir(null)} />}
      {aDeshacer && <DeshacerDialog it={aDeshacer} onCerrar={() => setADeshacer(null)} />}
    </Card>
  );
}

interface FilaCargaProps {
  it: ImportacionItem;
  puedeRevertir: boolean;
  onRevertir: () => void;
  onDeshacer: () => void;
}

function FilaCarga({ it, puedeRevertir, onRevertir, onDeshacer }: FilaCargaProps) {
  const estado = ESTADO[it.estado] ?? { label: it.estado, tone: "neutral" as BadgeTone };

  return (
    // La revertida se ve DISTINTA, no escondida: quien corrigió tiene que poder verificar qué marcó.
    <TR className={cn(it.revertida && "bg-negativo/5")}>
      <TD className="whitespace-nowrap">
        <span className="tabular-nums">{formatFechaHora(it.creado_en) || "—"}</span>
        <span className="mt-0.5 block text-xs text-content-muted">
          {it.creado_por_nombre || "—"}
        </span>
      </TD>
      <TD>{nombreCuenta(it)}</TD>
      <TD className="max-w-xs truncate" title={it.nombre_archivo}>
        {it.nombre_archivo || "—"}
      </TD>
      <TD className="text-right tabular-nums">
        {it.movimientos}
        {it.excluidos > 0 && (
          <span
            className="mt-0.5 block text-xs font-medium text-negativo"
            title="Movimientos marcados como excluidos: siguen guardados pero no suman."
          >
            {it.excluidos} no suman
          </span>
        )}
      </TD>
      <TD className="whitespace-nowrap tabular-nums">{rangoDeFechas(it)}</TD>
      <TD className="text-right tabular-nums">{formatMoneda(it.total_debitos, it.moneda)}</TD>
      <TD className="text-right tabular-nums">{formatMoneda(it.total_creditos, it.moneda)}</TD>
      <TD>
        <Badge tone={estado.tone}>{estado.label}</Badge>
        {it.revertida && (
          <span className="mt-1 block text-xs text-content-muted">
            Revertida el{" "}
            <span className="tabular-nums">{formatFechaHora(it.revertida_en) || "—"}</span>
            {it.revertida_por_nombre ? ` por ${it.revertida_por_nombre}` : ""}
            {it.motivo_reversa ? ` · «${it.motivo_reversa}»` : ""}
          </span>
        )}
      </TD>
      <TD className="whitespace-normal">
        {/* Sin el permiso no hay botón (el backend igual deniega; esto es para no ofrecer lo
            que no se puede hacer). */}
        {puedeRevertir && (
          <AccionDeLaFila it={it} onRevertir={onRevertir} onDeshacer={onDeshacer} />
        )}
      </TD>
    </TR>
  );
}

/**
 * El botón de la fila y, cuando está bloqueado, la razón A LA VISTA.
 *
 * Un botón deshabilitado sin explicación es una pared: el usuario no sabe si le falta permiso, si
 * el sistema está roto o qué tiene que ir a destrabar. El texto lo redacta el servidor.
 */
function AccionDeLaFila({
  it,
  onRevertir,
  onDeshacer,
}: {
  it: ImportacionItem;
  onRevertir: () => void;
  onDeshacer: () => void;
}) {
  const bloqueado = it.revertida ? !it.puede_deshacer_reversa : !it.puede_revertir;
  return (
    <div className="flex max-w-[16rem] flex-col items-start gap-1">
      {it.revertida ? (
        <Button
          size="sm"
          variant="secondary"
          onClick={onDeshacer}
          disabled={!it.puede_deshacer_reversa}
        >
          Deshacer la reversa
        </Button>
      ) : (
        <Button size="sm" variant="secondary" onClick={onRevertir} disabled={!it.puede_revertir}>
          Revertir esta carga
        </Button>
      )}
      {bloqueado && it.razon_no_revertir && (
        <span className="text-xs text-content-muted">{it.razon_no_revertir}</span>
      )}
    </div>
  );
}

/** Marco común de los dos diálogos (mismo patrón que el de «Mi partida»). */
function Dialogo({
  titulo,
  onCerrar,
  children,
}: {
  titulo: string;
  onCerrar: () => void;
  children: ReactNode;
}) {
  return (
    <div
      className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && onCerrar()}
      role="dialog"
      aria-modal="true"
    >
      <div className="w-full max-w-lg rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="text-base font-semibold text-content">{titulo}</h2>
        {children}
      </div>
    </div>
  );
}

/**
 * La confirmación de revertir dice con NÚMEROS qué va a pasar, porque a la vista del usuario esto
 * es destructivo aunque no borre nada: la plata deja de estar en los libros.
 */
function RevertirDialog({ it, onCerrar }: { it: ImportacionItem; onCerrar: () => void }) {
  const toast = useToast();
  const revertir = useRevertirImportacion();
  const [motivo, setMotivo] = useState("");

  function enviar() {
    revertir.mutate(
      { importacionId: it.id, motivo: motivo.trim() },
      {
        onSuccess: (res) => {
          toast.success(
            `Carga revertida: ${res.excluidos} ${res.excluidos === 1 ? "movimiento salió" : "movimientos salieron"} ` +
              `de los libros (${formatMoneda(res.total_debitos, res.moneda)} en débitos y ` +
              `${formatMoneda(res.total_creditos, res.moneda)} en créditos).`,
          );
          onCerrar();
        },
        // Un 422 (algo se apoya en esos movimientos) o un 409 (ya estaba revertida) llegan con el
        // mensaje redactado del backend: se muestra tal cual, no «error desconocido».
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <Dialogo titulo="Revertir esta carga" onCerrar={onCerrar}>
      <div className="mt-3 rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
        <p className="font-medium text-content">{it.nombre_archivo || "—"}</p>
        <p className="mt-0.5 text-content-muted">
          Subida el <span className="tabular-nums">{formatFechaHora(it.creado_en) || "—"}</span>
          {it.creado_por_nombre ? ` por ${it.creado_por_nombre}` : ""}
        </p>
      </div>

      <p className="mt-3 text-sm text-content">
        {it.movimientos === 0 ? (
          <>
            Esta carga no trajo ningún movimiento: se marca como revertida y no cambia ninguna
            plata. Las otras cargas de {nombreCuenta(it)} no se tocan.
          </>
        ) : (
          <>
            Vas a excluir {frasePlata(it)}.{" "}
            <span className="font-medium">Las otras cargas de esa cuenta no se tocan.</span>
          </>
        )}
      </p>
      <p className="mt-2 text-sm text-content-muted">
        Los movimientos no se borran: quedan marcados y dejan de sumar en saldos, cuadre y reportes.
        Si te equivocaste de carga, se deshace desde esta misma lista.
      </p>

      <div className="mt-3">
        <Input
          label="¿Por qué se revierte? *"
          value={motivo}
          onChange={(e) => setMotivo(e.target.value)}
          placeholder="Ej. el archivo era de Promerica VDP y se importó en Promerica Colinas"
          hint="Queda guardado junto a la carga y en la auditoría. Dentro de tres meses es lo único que explica por qué falta esa plata."
        />
      </div>

      <div className="mt-5 flex justify-end gap-2">
        <Button variant="secondary" onClick={onCerrar} disabled={revertir.isPending}>
          Cancelar
        </Button>
        <Button
          onClick={enviar}
          loading={revertir.isPending}
          disabled={!motivo.trim()}
          className="!bg-negativo !text-white hover:!bg-negativo/90"
        >
          Revertir la carga
        </Button>
      </div>
    </Dialogo>
  );
}

/**
 * Deshacer mueve la MISMA plata en el sentido contrario, así que se confirma con los mismos
 * números. El motivo es opcional (así lo definió el backend), pero queda en la auditoría.
 */
function DeshacerDialog({ it, onCerrar }: { it: ImportacionItem; onCerrar: () => void }) {
  const toast = useToast();
  const deshacer = useDeshacerReversaImportacion();
  const [motivo, setMotivo] = useState("");

  function enviar() {
    deshacer.mutate(
      { importacionId: it.id, motivo: motivo.trim() },
      {
        onSuccess: (res) => {
          toast.success(
            `Reversa deshecha: ${res.reincluidos} ${res.reincluidos === 1 ? "movimiento volvió" : "movimientos volvieron"} ` +
              `a los libros (${formatMoneda(res.total_debitos, res.moneda)} en débitos y ` +
              `${formatMoneda(res.total_creditos, res.moneda)} en créditos).`,
          );
          onCerrar();
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <Dialogo titulo="Deshacer la reversa" onCerrar={onCerrar}>
      <div className="mt-3 rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
        <p className="font-medium text-content">{it.nombre_archivo || "—"}</p>
        {it.motivo_reversa && (
          <p className="mt-0.5 text-content-muted">Se revirtió por: «{it.motivo_reversa}»</p>
        )}
      </div>

      <p className="mt-3 text-sm text-content">
        Vuelven a los libros los movimientos que esta reversa excluyó: hasta {frasePlata(it)}.
      </p>
      <p className="mt-2 text-sm text-content-muted">
        Lo que ya estaba excluido por otra corrección antes de la reversa sigue afuera.
      </p>

      <div className="mt-3">
        <Input
          label="¿Por qué se deshace? (opcional)"
          value={motivo}
          onChange={(e) => setMotivo(e.target.value)}
          placeholder="Ej. me equivoqué de carga"
          hint="Queda en la auditoría."
        />
      </div>

      <div className="mt-5 flex justify-end gap-2">
        <Button variant="secondary" onClick={onCerrar} disabled={deshacer.isPending}>
          Cancelar
        </Button>
        <Button onClick={enviar} loading={deshacer.isPending}>
          Deshacer la reversa
        </Button>
      </div>
    </Dialogo>
  );
}
