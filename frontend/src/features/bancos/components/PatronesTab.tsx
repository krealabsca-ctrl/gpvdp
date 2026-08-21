/**
 * Pestaña «Patrones» de la Bandeja de clasificación.
 *
 * El problema que resuelve: los movimientos sin clasificar no son casos distintos, son unos
 * pocos hechos repetidos miles de veces. Nadie descubre eso escribiendo búsquedas a mano. Acá
 * el sistema los agrupa por forma, propone la palabra clave de cada grupo y con un clic crea la
 * regla — la misma `CrearRegla` de siempre, que retro-aplica y suma aciertos.
 *
 * No cambia el criterio del motor: sigue clasificando solo con coincidencia exacta (≥90%).
 * Cuando no hay una palabra segura NO se propone nada: se dice por qué y se deja el grupo para
 * revisión manual.
 */

import { useState } from "react";
import {
  Badge,
  Button,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  Select,
  useToast,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatMoneda, sinTildes } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useCrearRegla, usePatrones } from "@/features/bancos/hooks";
import { ClasifCombobox, type ClasifElegida } from "@/features/bancos/components/ClasifCombobox";
import type { AplicaA, MotivoPatron, PatronSugerido } from "@/api/bancos";

const ETIQUETA_MOTIVO: Record<Exclude<MotivoPatron, "">, string> = {
  SOLO_REFERENCIAS:
    "Lo único que comparten es el número de referencia, que no se repite. Una regla con eso clasificaría los de hoy y nunca más: hay que revisarlos en «Por clasificar».",
  SIN_PALABRA_SEGURA:
    "No hay una palabra que identifique este grupo sin alcanzar movimientos de otro tipo. Conviene revisarlos en «Por clasificar».",
};

const ETIQUETA_APLICA: Record<AplicaA, string> = {
  CREDITO: "Solo créditos",
  DEBITO: "Solo débitos",
  MIXTO: "Créditos y débitos",
};

export function PatronesTab({ periodo }: { periodo?: string }) {
  const patronesQ = usePatrones(periodo);
  const [busqueda, setBusqueda] = useState("");
  const [signo, setSigno] = useState("");
  const [soloProponibles, setSoloProponibles] = useState(false);

  if (patronesQ.isPending) return <LoadingState label="Buscando patrones en lo que falta clasificar…" />;
  if (patronesQ.isError) {
    return <ErrorState message={mensajeError(patronesQ.error)} onRetry={() => void patronesQ.refetch()} />;
  }
  const todos = patronesQ.data ?? [];
  if (todos.length === 0) {
    return (
      <EmptyState message="No hay patrones que proponer: o ya está todo clasificado, o lo que queda son casos sueltos que no repiten forma. Revisalos en «Por clasificar»." />
    );
  }

  const patrones = todos.filter((p) => {
    if (soloProponibles && !p.patron) return false;
    if (signo && p.aplica_a !== signo) return false;
    if (!busqueda.trim()) return true;
    const t = sinTildes(busqueda);
    return sinTildes(p.patron).includes(t) || p.ejemplos.some((e) => sinTildes(e).includes(t));
  });

  const conRegla = patrones.filter((p) => p.patron);
  const movsConRegla = conRegla.reduce((s, p) => s + p.movimientos, 0);
  const movsAgrupados = patrones.reduce((s, p) => s + p.movimientos, 0);

  return (
    <div className="flex flex-col gap-4">
      <div className="rounded-xl border border-accent/30 bg-accent/5 px-4 py-3 text-sm">
        <p className="font-medium text-content">
          {conRegla.length} regla{conRegla.length === 1 ? "" : "s"} clasificarían{" "}
          <span className="tabular-nums">{movsConRegla.toLocaleString("es-CR")}</span> movimientos.
        </p>
        <p className="mt-0.5 text-xs text-content-muted">
          {patrones.length} grupos con {movsAgrupados.toLocaleString("es-CR")} movimientos
          {patrones.length !== todos.length && ` (de ${todos.length} grupos en total)`}. Elegí el
          concepto de cada grupo y creá la regla: se aplica de una vez a todo lo que calce y queda para los
          movimientos que entren después.
        </p>
      </div>

      {/* Filtros: la lista puede traer 25 grupos y no todos se atienden en la misma sesión. */}
      <div className="flex flex-wrap items-end gap-3">
        <div className="min-w-[240px] flex-1">
          <Input
            label="Buscar"
            value={busqueda}
            onChange={(e) => setBusqueda(e.target.value)}
            placeholder="Palabra del patrón o de un ejemplo…"
          />
        </div>
        <div className="min-w-[160px]">
          <Select
            label="Signo"
            placeholder="Todos"
            value={signo}
            onChange={(e) => setSigno(e.target.value)}
            options={[
              { value: "CREDITO", label: "Solo créditos" },
              { value: "DEBITO", label: "Solo débitos" },
              { value: "MIXTO", label: "Mezclados" },
            ]}
          />
        </div>
        <label className="flex items-center gap-2 pb-2.5 text-xs text-content-muted">
          <input
            type="checkbox"
            checked={soloProponibles}
            onChange={(e) => setSoloProponibles(e.target.checked)}
            className="h-3.5 w-3.5 rounded border-border accent-accent"
          />
          Solo los que se pueden convertir en regla
        </label>
        {(busqueda || signo || soloProponibles) && (
          <Button
            variant="ghost"
            onClick={() => {
              setBusqueda("");
              setSigno("");
              setSoloProponibles(false);
            }}
          >
            Limpiar
          </Button>
        )}
      </div>

      {patrones.length === 0 ? (
        <EmptyState message="Ningún patrón coincide con el filtro." />
      ) : (
        /* La clave es el contenido del grupo, no el índice: al refrescar (crear una regla
           invalida la lista) el estado de cada tarjeta tiene que seguir siendo el suyo. */
        patrones.map((p) => <FilaPatron key={p.patron || p.ejemplos[0]} patron={p} />)
      )}
    </div>
  );
}

function FilaPatron({ patron }: { patron: PatronSugerido }) {
  const toast = useToast();
  const crear = useCrearRegla();
  const [palabra, setPalabra] = useState(patron.patron);
  const [elegida, setElegida] = useState<ClasifElegida | null>(null);
  const [listo, setListo] = useState(false);

  const proponible = patron.patron !== "";
  // El signo del grupo decide el concepto por defecto al crear una clasificación de 1 nivel:
  // un crédito nunca es un gasto. Mixto no adivina (igual que en el resto de la Bandeja).
  const esDebito = patron.aplica_a === "DEBITO" ? true : patron.aplica_a === "CREDITO" ? false : undefined;
  const yaClasificados = Math.max(patron.alcance - patron.movimientos - patron.ajenos, 0);

  function crearRegla() {
    if (!elegida || !palabra.trim()) return;
    crear.mutate(
      {
        nombre: palabra.trim().slice(0, 60),
        aplica_a: patron.aplica_a,
        concepto_id: elegida.conceptoId,
        clasificacion_id: elegida.clasificacionId,
        palabras_clave: [palabra.trim()],
      },
      {
        onSuccess: (r) => {
          setListo(true);
          toast.success(
            `Regla creada: clasificó ${r.clasificados.toLocaleString("es-CR")} movimientos como ${elegida.ruta}.`,
          );
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div
      className={cn(
        "rounded-xl border bg-surface-raised p-4 shadow-card",
        listo ? "border-positivo/40" : proponible ? "border-border" : "border-border bg-surface-muted",
      )}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-lg font-semibold tabular-nums text-content">
              {patron.movimientos.toLocaleString("es-CR")}
            </span>
            <span className="text-sm text-content-muted">movimientos</span>
            <Badge tone={patron.aplica_a === "MIXTO" ? "pendiente" : "neutral"}>
              {ETIQUETA_APLICA[patron.aplica_a]}
            </Badge>
            {patron.aplica_a === "MIXTO" && (
              <span className="text-xs text-content-muted">
                {patron.creditos} cr · {patron.debitos} db
              </span>
            )}
            {listo && <Badge tone="positivo">Regla creada</Badge>}
          </div>
          <p className="mt-1 text-sm tabular-nums text-content-muted">{formatMoneda(patron.monto, "CRC")}</p>
        </div>
      </div>

      {/* Ejemplos: lo que permite reconocer el hecho de un vistazo */}
      <ul className="mt-3 flex flex-col gap-1">
        {patron.ejemplos.map((e, i) => (
          <li key={i} className="break-words font-mono text-xs text-content-muted">
            {e}
          </li>
        ))}
      </ul>

      {!proponible ? (
        <p className="mt-3 rounded-lg border border-border bg-surface px-3 py-2 text-xs text-content-muted">
          {ETIQUETA_MOTIVO[patron.motivo as Exclude<MotivoPatron, "">]}
        </p>
      ) : listo ? null : (
        <div className="mt-3 flex flex-col gap-3 border-t border-border pt-3">
          <div className="grid grid-cols-1 gap-3 lg:grid-cols-[1.4fr_1.4fr_auto] lg:items-end">
            <label className="flex flex-col gap-1 text-xs text-content-muted">
              Palabra clave de la regla
              <Input
                value={palabra}
                onChange={(e) => setPalabra(e.target.value)}
                className="font-mono"
                aria-label="Palabra clave"
              />
            </label>
            <div className="flex flex-col gap-1 text-xs text-content-muted">
              Clasificar como
              <ClasifCombobox
                actual={elegida?.ruta ?? ""}
                esDebito={esDebito}
                onElegir={setElegida}
                placeholder="Concepto › Clasificación"
              />
            </div>
            <Button onClick={crearRegla} disabled={!elegida || !palabra.trim() || crear.isPending}>
              Crear regla
            </Button>
          </div>

          <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-content-muted">
            {yaClasificados > 0 && (
              <span>
                ✓ {yaClasificados.toLocaleString("es-CR")} movimientos iguales ya se clasificaron a mano — la
                palabra es la correcta.
              </span>
            )}
            {patron.ajenos > 0 && (
              <span className="text-pendiente">
                ⚠ también alcanzaría {patron.ajenos} movimientos de otro tipo.
              </span>
            )}
            {patron.aviso_anio && (
              <span className="text-pendiente">
                ⚠ la palabra incluye un año: dejará de calzar cuando cambie.
              </span>
            )}
            {patron.alterna && (
              <button
                type="button"
                onClick={() => setPalabra(patron.alterna)}
                className="text-accent underline decoration-dotted"
              >
                usar «{patron.alterna}»
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
