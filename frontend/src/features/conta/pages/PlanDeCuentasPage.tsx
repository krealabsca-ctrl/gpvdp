/**
 * Contabilidad → Plan de cuentas (/conta/plan). FASE 1.
 *
 * ESTA PANTALLA NO CONTABILIZA NADA, y lo dice en voz alta. Es el catálogo contable dentro del
 * sistema: se importa del Excel del contador, se navega y se mantiene. Decirlo importa porque
 * ver «Contabilidad» en el menú hace suponer que ya hay asientos, y no los hay: el mapeo es la
 * Fase 2 y los asientos la Fase 3.
 *
 * SE MUESTRA COMO ÁRBOL porque un plan de cuentas es un árbol. Una tabla plana de 258 filas
 * ordenadas por código se lee igual que el Excel del que salió, y entonces no agrega nada.
 */

import { useMemo, useState } from "react";

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
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useImportarPlan,
  usePlanDeCuentas,
  usePrevisualizarPlan,
} from "@/features/conta/hooks";
import type { ClaseContable, CuentaContable, PrevisualizacionPlan } from "@/api/conta";

/** Tono por clase. Activo y gasto crecen por el debe; pasivo, patrimonio e ingreso por el haber. */
const TONO: Record<ClaseContable, "accent" | "positivo" | "negativo" | "pendiente" | "neutral"> = {
  ACTIVO: "accent",
  PASIVO: "pendiente",
  PATRIMONIO: "neutral",
  INGRESO: "positivo",
  COSTO: "negativo",
  GASTO: "negativo",
  ORDEN: "neutral",
};

export function PlanDeCuentasPage() {
  const toast = useToast();
  const puedeEditar = useTienePermiso()("conta.plan_cuentas");

  const [q, setQ] = useState("");
  const [abiertas, setAbiertas] = useState<Set<string>>(new Set());
  const [archivo, setArchivo] = useState<File | null>(null);
  const [prev, setPrev] = useState<PrevisualizacionPlan | null>(null);

  const planQ = usePlanDeCuentas(false);
  const previsualizar = usePrevisualizarPlan();
  const importar = useImportarPlan();

  const arbol = planQ.data?.arbol ?? [];

  // Al buscar se aplana: con un filtro puesto lo que importa es QUÉ cuentas coinciden, no dónde
  // cuelgan. Navegar el árbol para encontrarlas es justamente lo que el buscador evita.
  const coincidencias = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return null;
    const out: CuentaContable[] = [];
    const visitar = (cs: CuentaContable[]) => {
      for (const c of cs) {
        if (c.codigo.toLowerCase().includes(needle) || c.nombre.toLowerCase().includes(needle)) {
          out.push(c);
        }
        visitar(c.hijas ?? []);
      }
    };
    visitar(arbol);
    return out;
  }, [q, arbol]);

  const totales = useMemo(() => {
    let n = 0;
    let mov = 0;
    const visitar = (cs: CuentaContable[]) => {
      for (const c of cs) {
        n++;
        if (c.acepta_movimiento) mov++;
        visitar(c.hijas ?? []);
      }
    };
    visitar(arbol);
    return { n, mov };
  }, [arbol]);

  function alternar(id: string) {
    setAbiertas((prev) => {
      const s = new Set(prev);
      if (s.has(id)) s.delete(id);
      else s.add(id);
      return s;
    });
  }

  function elegirArchivo(f: File) {
    setArchivo(f);
    setPrev(null);
    previsualizar.mutate(f, {
      onSuccess: (p) => setPrev(p),
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  function importarAhora(destino: "grupo" | "empresa") {
    if (!archivo) return;
    importar.mutate(
      { archivo, destino },
      {
        onSuccess: (r) => {
          toast.success(
            `Plan cargado: ${r.creadas} cuenta(s) nueva(s), ${r.actualizadas} actualizada(s), ` +
              `${r.de_movimiento} de movimiento.`,
          );
          setArchivo(null);
          setPrev(null);
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Plan de cuentas"
        description="El catálogo contable del grupo. Se carga desde el Excel del contador."
      />

      {/* Lo primero que hay que decir: todavía no se contabiliza nada. Sin esto, ver
          «Contabilidad» en el menú hace suponer que ya hay asientos. */}
      <div className="rounded-lg border border-accent/40 bg-accent/5 px-4 py-3 text-sm text-content">
        <b>Esta pantalla todavía no contabiliza nada.</b> Es el paso 1 de 6: tener el plan de
        cuentas dentro del sistema. Lo siguiente es relacionarlo con las partidas que ya usás a
        diario, y recién después se generan asientos.
      </div>

      {puedeEditar && (
        <Card>
          <CardHeader>
            <CardTitle>Cargar el plan desde Excel</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <p className="text-sm text-content-muted">
              El archivo necesita una columna con el <b className="text-content">código</b> de la
              cuenta y otra con su <b className="text-content">nombre</b>. Si trae una segunda
              columna de códigos —la de Softland—, se guarda: es el puente para los saldos
              iniciales.
            </p>
            <input
              type="file"
              accept=".xlsx,.xls"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) elegirArchivo(f);
              }}
              className="text-sm text-content file:mr-3 file:rounded-lg file:border-0 file:bg-accent file:px-3 file:py-1.5 file:text-sm file:text-white"
            />
            {previsualizar.isPending && <p className="text-sm text-content-muted">Leyendo el archivo…</p>}
            {prev && <Previsualizacion p={prev} onImportar={importarAhora} importando={importar.isPending} />}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 pt-5">
          <Input
            label="Buscar en el plan"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Código o nombre: 1-01, Caja Chica, Salarios…"
          />
          <div className="ml-auto flex gap-4 text-sm">
            <div>
              <p className="text-xl font-semibold tabular-nums text-content">{totales.n}</p>
              <p className="text-xs text-content-muted">cuentas</p>
            </div>
            <div>
              <p className="text-xl font-semibold tabular-nums text-content">{totales.mov}</p>
              <p className="text-xs text-content-muted">de movimiento</p>
            </div>
          </div>
        </CardContent>
      </Card>

      {planQ.isPending ? (
        <LoadingState label="Cargando el plan" />
      ) : planQ.isError ? (
        <ErrorState message={mensajeError(planQ.error)} onRetry={() => planQ.refetch()} />
      ) : arbol.length === 0 ? (
        <EmptyState message="Todavía no hay plan de cuentas. Cargalo desde el Excel del contador." />
      ) : coincidencias ? (
        coincidencias.length === 0 ? (
          <EmptyState message={`Ninguna cuenta coincide con «${q}».`} />
        ) : (
          <Card>
            <CardContent className="flex flex-col gap-1 pt-5">
              <p className="mb-2 text-xs text-content-muted">
                {coincidencias.length} coincidencia(s). Se muestran planas: al buscar, lo que
                importa es cuáles son, no dónde cuelgan.
              </p>
              {coincidencias.map((c) => (
                <Fila key={c.id} c={c} sangria={0} abierta={false} onAlternar={() => {}} hojaForzada />
              ))}
            </CardContent>
          </Card>
        )
      ) : (
        <Card>
          <CardContent className="flex flex-col gap-0.5 pt-5">
            {arbol.map((c) => (
              <Rama key={c.id} c={c} sangria={0} abiertas={abiertas} onAlternar={alternar} />
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function Rama({
  c,
  sangria,
  abiertas,
  onAlternar,
}: {
  c: CuentaContable;
  sangria: number;
  abiertas: Set<string>;
  onAlternar: (id: string) => void;
}) {
  const tieneHijas = (c.hijas?.length ?? 0) > 0;
  // Las dos primeras ramas nacen abiertas: ver solo «ACTIVOS / PASIVOS / …» al entrar no dice
  // nada, y abrir seis veces para llegar a una cuenta es trabajo que la pantalla puede ahorrar.
  const abierta = abiertas.has(c.id) || (sangria < 1 && !abiertas.has("cerrada:" + c.id));
  return (
    <>
      <Fila c={c} sangria={sangria} abierta={abierta} onAlternar={() => onAlternar(abierta ? "cerrada:" + c.id : c.id)} />
      {tieneHijas && abierta &&
        c.hijas!.map((h) => (
          <Rama key={h.id} c={h} sangria={sangria + 1} abiertas={abiertas} onAlternar={onAlternar} />
        ))}
    </>
  );
}

function Fila({
  c,
  sangria,
  abierta,
  onAlternar,
  hojaForzada,
}: {
  c: CuentaContable;
  sangria: number;
  abierta: boolean;
  onAlternar: () => void;
  hojaForzada?: boolean;
}) {
  const tieneHijas = !hojaForzada && (c.hijas?.length ?? 0) > 0;
  return (
    <div
      className="flex items-center gap-2 rounded px-2 py-1 text-sm hover:bg-surface-raised"
      style={{ paddingLeft: `${sangria * 1.25 + 0.5}rem` }}
    >
      {tieneHijas ? (
        <button
          type="button"
          onClick={onAlternar}
          aria-label={abierta ? `Cerrar ${c.nombre}` : `Abrir ${c.nombre}`}
          className="w-4 shrink-0 text-content-muted hover:text-accent"
        >
          {abierta ? "▾" : "▸"}
        </button>
      ) : (
        <span className="w-4 shrink-0" />
      )}
      <span className="w-36 shrink-0 font-mono text-xs text-content-muted">{c.codigo}</span>
      <span className={c.acepta_movimiento ? "text-content" : "font-medium text-content"}>
        {c.nombre}
      </span>
      {/* Solo se marca lo que NO es obvio. Una cuenta de movimiento es el caso normal —son 211
          de 258—, así que lo que se señala es la de agrupación: la que no admite asientos. */}
      {!c.acepta_movimiento && (
        <span title="Agrupa a sus cuentas hijas; no admite asientos">
          <Badge tone="neutral">agrupa</Badge>
        </span>
      )}
      {sangria === 0 && <Badge tone={TONO[c.clase]}>{c.clase}</Badge>}
      {!c.activo && <Badge tone="negativo">apagada</Badge>}
      {c.empresa_id === null && (
        <span title="Cuenta del grupo: la comparten las tres empresas">
          <Badge tone="accent">grupo</Badge>
        </span>
      )}
      {c.codigo_softland && c.codigo_softland !== c.codigo && (
        <span className="ml-auto font-mono text-[11px] text-content-muted" title="Código en Softland">
          Softland {c.codigo_softland}
        </span>
      )}
    </div>
  );
}

/** Lo que va a pasar si se confirma. Nadie carga un plan de cuentas a ciegas. */
function Previsualizacion({
  p,
  onImportar,
  importando,
}: {
  p: PrevisualizacionPlan;
  onImportar: (destino: "grupo" | "empresa") => void;
  importando: boolean;
}) {
  const conProblema = p.filas.filter((f) => f.problema);
  return (
    <div className="rounded-lg border border-border bg-surface p-3">
      <p className="text-sm font-medium text-content">
        {p.a_crear} cuenta(s) nueva(s) · {p.a_actualizar} que ya existen ·{" "}
        {p.de_movimiento} de movimiento
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        {Object.entries(p.por_clase).map(([clase, n]) => (
          <Badge key={clase} tone={TONO[clase as ClaseContable] ?? "neutral"}>
            {clase} {n}
          </Badge>
        ))}
      </div>

      {conProblema.length > 0 && (
        <div className="mt-3">
          <p className="text-xs text-pendiente">
            {conProblema.length} fila(s) quedan fuera. Casi siempre son títulos del archivo, y está
            bien que no entren:
          </p>
          <ul className="mt-1 max-h-32 overflow-auto text-xs text-content-muted">
            {conProblema.slice(0, 10).map((f) => (
              <li key={f.fila}>
                fila {f.fila}: «{f.codigo || f.nombre}» — {f.problema}
              </li>
            ))}
            {conProblema.length > 10 && <li>… y {conProblema.length - 10} más</li>}
          </ul>
        </div>
      )}

      {/* La decisión de a quién pertenece el plan. Se toma acá, al cargar, y cambiar de opinión
          es volver a importar: el modelo soporta las dos formas. */}
      <p className="mt-4 text-sm font-medium text-content">¿De quién es este plan?</p>
      <p className="mt-0.5 text-xs text-content-muted">
        Un plan del <b className="text-content">grupo</b> lo comparten las tres empresas. Uno de la{" "}
        <b className="text-content">empresa</b> es solo de la que tenés activa. Se puede cambiar
        después volviendo a importar.
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        <Button onClick={() => onImportar("empresa")} loading={importando}>
          Cargar para esta empresa
        </Button>
        <Button variant="secondary" onClick={() => onImportar("grupo")} loading={importando}>
          Cargar para todo el grupo
        </Button>
      </div>
    </div>
  );
}
