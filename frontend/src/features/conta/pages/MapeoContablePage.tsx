/**
 * Contabilidad → Mapeo contable (/conta/mapeo). FASE 2.
 *
 * SIGUE SIN CONTABILIZAR NADA. Acá se construye la equivalencia entre las partidas que la
 * operación ya usa a diario y las cuentas del plan, y se mide cuánto falta. El valor de terminar
 * esta pantalla es que, al hacerlo, un número dice si la contabilidad se puede generar o no.
 *
 * ── POR QUÉ TRABAJA EN LOTE ──────────────────────────────────────────────────────────────────
 *
 * Son 332 clasificaciones con movimientos en las tres empresas. El trabajo viene en FAMILIAS: las
 * 23 «Caja Chica - X» van todas a cuentas de caja chica, las de combustible a combustible.
 * Resolverlas de a una es lo que hace que un mapeo no se termine nunca. Por eso se filtra, se
 * marcan varias y se decide una sola vez.
 *
 * ── POR QUÉ EL AVISO DE CRUCE AVISA Y NO BLOQUEA ─────────────────────────────────────────────
 *
 * Las 23 de caja chica cuelgan del concepto GASTOS en el ERP, y en el plan la caja chica es
 * ACTIVO. Las dos cosas están bien: la clasificación responde «¿a dónde se fue la plata?» y la
 * cuenta responde «¿qué tengo?». Lo que no puede pasar es que nadie lo mire.
 */

import { useMemo, useState } from "react";

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
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useCuentasDeMovimiento,
  useDecidirMapeo,
  useDeshacerMapeo,
  useMapeoContable,
} from "@/features/conta/hooks";
import {
  porcentajeResuelto,
  type CoberturaMapeo,
  type DecisionDeMapeo,
  type FilaDelMapeo,
} from "@/api/conta";

/** El mínimo que el servidor exige para un motivo. Se repite acá para avisar antes de mandar. */
const MOTIVO_MINIMO = 15;

export function MapeoContablePage() {
  const toast = useToast();
  const puedeMapear = useTienePermiso()("conta.mapeo");

  const [soloPendientes, setSoloPendientes] = useState(true);
  const [soloConMovimientos, setSoloConMovimientos] = useState(true);
  const [q, setQ] = useState("");
  const [marcadas, setMarcadas] = useState<Set<string>>(new Set());
  const [cuentaElegida, setCuentaElegida] = useState<string[]>([]);
  const [motivo, setMotivo] = useState("");

  const mapeoQ = useMapeoContable(soloPendientes, soloConMovimientos);
  const cuentasQ = useCuentasDeMovimiento();
  const decidir = useDecidirMapeo();
  const deshacer = useDeshacerMapeo();

  const filas = mapeoQ.data?.items ?? [];
  const cobertura = mapeoQ.data?.cobertura;

  const visibles = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return filas;
    return filas.filter(
      (f) =>
        f.clasificacion.toLowerCase().includes(needle) ||
        f.concepto.toLowerCase().includes(needle) ||
        f.cuenta_nombre.toLowerCase().includes(needle) ||
        f.cuenta_codigo.toLowerCase().includes(needle),
    );
  }, [filas, q]);

  const opcionesCuenta = useMemo(
    () =>
      (cuentasQ.data ?? []).map((c) => ({
        value: c.id,
        label: `${c.codigo} · ${c.nombre}`,
        grupo: c.clase,
      })),
    [cuentasQ.data],
  );

  // Lo marcado que ya no está a la vista no se decide: si alguien filtra «caja chica», marca 23 y
  // después busca otra cosa, aplicar sobre las que ya no ve sería mapear a ciegas.
  const aDecidir = useMemo(
    () => visibles.filter((f) => marcadas.has(f.clasificacion_id)),
    [visibles, marcadas],
  );

  function alternar(id: string) {
    setMarcadas((prev) => {
      const s = new Set(prev);
      if (s.has(id)) s.delete(id);
      else s.add(id);
      return s;
    });
  }

  function marcarTodasLasVisibles() {
    setMarcadas((prev) => {
      const s = new Set(prev);
      const faltan = visibles.some((f) => !s.has(f.clasificacion_id));
      for (const f of visibles) {
        if (faltan) s.add(f.clasificacion_id);
        else s.delete(f.clasificacion_id);
      }
      return s;
    });
  }

  function aplicar(decisiones: DecisionDeMapeo[]) {
    decidir.mutate(decisiones, {
      onSuccess: (r) => {
        const fallos = Object.keys(r.no_se_pudo).length;
        if (fallos === 0) {
          toast.success(`${r.guardadas} partida(s) resuelta(s).`);
        } else {
          // El primer motivo, textual: un «fallaron 3» sin el porqué deja probando a ciegas.
          const primero = Object.values(r.no_se_pudo)[0] ?? "";
          toast.error(
            `${r.guardadas} guardada(s), ${fallos} no: ${primero}` +
              (fallos > 1 ? ` (y ${fallos - 1} más)` : ""),
          );
        }
        setMarcadas(new Set());
        setCuentaElegida([]);
        setMotivo("");
      },
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  function mapearMarcadas() {
    const cuenta = cuentaElegida[0];
    if (!cuenta || aDecidir.length === 0) return;
    aplicar(
      aDecidir.map((f) => ({
        clasificacion_id: f.clasificacion_id,
        estado: "MAPEADA" as const,
        cuenta_id: cuenta,
      })),
    );
  }

  function declararNoContabilizables() {
    if (motivo.trim().length < MOTIVO_MINIMO || aDecidir.length === 0) return;
    aplicar(
      aDecidir.map((f) => ({
        clasificacion_id: f.clasificacion_id,
        estado: "NO_CONTABILIZABLE" as const,
        motivo: motivo.trim(),
      })),
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Mapeo contable"
        description="A qué cuenta del plan corresponde cada partida que ya usás a diario."
      />

      <div className="rounded-lg border border-accent/40 bg-accent/5 px-4 py-3 text-sm text-content">
        <b>Esto todavía no genera asientos.</b> Es el paso 2 de 6: decir a qué cuenta va cada
        partida. Cuando el avance llegue al 100 %, la contabilidad se puede generar; mientras
        tanto, el número de abajo dice exactamente cuánto falta.
      </div>

      {cobertura && <Avance c={cobertura} />}

      <Card>
        <CardContent className="flex flex-wrap items-end gap-4 pt-5">
          <Input
            label="Buscar"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Caja chica, combustible, 6-01…"
          />
          <label className="flex items-center gap-2 pb-2 text-sm text-content">
            <input
              type="checkbox"
              checked={soloPendientes}
              onChange={(e) => {
                setSoloPendientes(e.target.checked);
                setMarcadas(new Set());
              }}
            />
            Solo las que faltan
          </label>
          <label className="flex items-center gap-2 pb-2 text-sm text-content">
            <input
              type="checkbox"
              checked={soloConMovimientos}
              onChange={(e) => {
                setSoloConMovimientos(e.target.checked);
                setMarcadas(new Set());
              }}
            />
            Solo las que se usan
          </label>
          <p className="ml-auto pb-2 text-sm text-content-muted">
            {visibles.length} partida(s) a la vista
          </p>
        </CardContent>
      </Card>

      {puedeMapear && aDecidir.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>{aDecidir.length} partida(s) marcada(s)</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            <div className="flex flex-col gap-2">
              <BuscadorMultiple
                label="Mandarlas todas a esta cuenta"
                opciones={opcionesCuenta}
                seleccion={cuentaElegida}
                // Una sola cuenta por decisión: se queda la última elegida.
                onChange={(v) => setCuentaElegida(v.slice(-1))}
                placeholder="Escribí el código o el nombre de la cuenta…"
                leyendaVacio="solo cuentas de movimiento"
              />
              <div>
                <Button
                  onClick={mapearMarcadas}
                  disabled={cuentaElegida.length === 0 || decidir.isPending}
                >
                  Mapear las {aDecidir.length}
                </Button>
              </div>
            </div>

            <div className="flex flex-col gap-2 border-t border-border pt-4">
              <Input
                label="…o declararlas no contabilizables, diciendo por qué"
                value={motivo}
                onChange={(e) => setMotivo(e.target.value)}
                placeholder="Es un traslado entre cuentas propias de la empresa"
              />
              <p className="text-xs text-content-muted">
                El motivo es la respuesta que un auditor va a pedir dentro de seis meses. Mínimo{" "}
                {MOTIVO_MINIMO} caracteres.
              </p>
              <div>
                <Button
                  variant="secondary"
                  onClick={declararNoContabilizables}
                  disabled={motivo.trim().length < MOTIVO_MINIMO || decidir.isPending}
                >
                  No se contabilizan
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {mapeoQ.isPending ? (
        <LoadingState label="Cargando el mapeo" />
      ) : mapeoQ.isError ? (
        <ErrorState message={mensajeError(mapeoQ.error)} onRetry={() => mapeoQ.refetch()} />
      ) : visibles.length === 0 ? (
        <EmptyState
          message={
            soloPendientes
              ? "No queda ninguna partida sin decidir con estos filtros."
              : "No hay partidas que coincidan."
          }
        />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH className="w-10">
                  {puedeMapear && (
                    <input
                      type="checkbox"
                      aria-label="Marcar todas las visibles"
                      checked={visibles.length > 0 && visibles.every((f) => marcadas.has(f.clasificacion_id))}
                      onChange={marcarTodasLasVisibles}
                    />
                  )}
                </TH>
                <TH>Partida</TH>
                <TH className="text-right">Movimientos</TH>
                <TH>Naturaleza en el ERP</TH>
                <TH>Cuenta contable</TH>
                <TH />
              </TR>
            </THead>
            <TBody>
              {visibles.map((f) => (
                <Fila
                  key={f.clasificacion_id}
                  f={f}
                  marcada={marcadas.has(f.clasificacion_id)}
                  puedeMapear={puedeMapear}
                  onMarcar={() => alternar(f.clasificacion_id)}
                  onDeshacer={() =>
                    deshacer.mutate(f.clasificacion_id, {
                      onError: (e) => toast.error(mensajeError(e)),
                    })
                  }
                />
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}

/** El semáforo: cuánto falta sobre lo que de verdad bloquea. */
function Avance({ c }: { c: CoberturaMapeo }) {
  const pct = porcentajeResuelto(c);
  return (
    <Card>
      <CardContent className="flex flex-col gap-4 pt-5">
        <div className="flex flex-wrap items-end gap-6">
          <div>
            <p className="text-3xl font-semibold tabular-nums text-content">{pct}%</p>
            <p className="text-xs text-content-muted">
              resuelto de las {c.con_movimientos} partidas que se usan
            </p>
          </div>
          <Dato n={c.mapeadas} rotulo="mapeadas" />
          <Dato n={c.no_contabilizables} rotulo="no se contabilizan" />
          <Dato n={c.sin_decidir} rotulo="faltan" tono={c.sin_decidir > 0 ? "pendiente" : undefined} />
          {/* Este NO comparte denominador con los tres de la izquierda: se cuenta sobre todas las
              mapeadas, se usen o no. Sin decirlo, «2 cruzadas» al lado de «1 mapeada» parece un
              error de la pantalla. */}
          <Dato
            n={c.cruzadas}
            rotulo="cruzadas, de todas las mapeadas"
            tono={c.cruzadas > 0 ? "negativo" : undefined}
          />
          <div className="ml-auto text-right">
            <p className="text-sm tabular-nums text-content-muted">
              {c.total} clasificaciones en total
            </p>
            {c.sin_naturaleza > 0 && (
              <p className="text-xs text-content-muted">
                {c.sin_naturaleza} sin declarar si es ingreso o gasto
              </p>
            )}
          </div>
        </div>
        <div className="h-2 w-full overflow-hidden rounded-full bg-surface-muted">
          <div className="h-full rounded-full bg-accent" style={{ width: `${pct}%` }} />
        </div>
      </CardContent>
    </Card>
  );
}

function Dato({ n, rotulo, tono }: { n: number; rotulo: string; tono?: "pendiente" | "negativo" }) {
  const color =
    tono === "negativo" ? "text-danger" : tono === "pendiente" ? "text-warning" : "text-content";
  return (
    <div>
      <p className={`text-xl font-semibold tabular-nums ${color}`}>{n}</p>
      <p className="text-xs text-content-muted">{rotulo}</p>
    </div>
  );
}

function Fila({
  f,
  marcada,
  puedeMapear,
  onMarcar,
  onDeshacer,
}: {
  f: FilaDelMapeo;
  marcada: boolean;
  puedeMapear: boolean;
  onMarcar: () => void;
  onDeshacer: () => void;
}) {
  return (
    <TR className={marcada ? "bg-accent/5" : undefined}>
      <TD>
        {puedeMapear && (
          <input
            type="checkbox"
            aria-label={`Marcar ${f.clasificacion}`}
            checked={marcada}
            onChange={onMarcar}
          />
        )}
      </TD>
      <TD>
        <p className="font-medium text-content">{f.clasificacion}</p>
        <p className="text-xs text-content-muted">{f.concepto}</p>
      </TD>
      <TD className="text-right tabular-nums">{f.movimientos}</TD>
      <TD>
        {f.naturaleza_declarada ? (
          <Badge tone="neutral">{f.naturaleza_concepto}</Badge>
        ) : (
          // «NEUTRO» sin declarar NO es una decisión: es el default de la columna. Decirlo evita
          // que alguien mapee creyendo que el ERP ya opinó.
          <span className="text-xs text-content-muted">sin declarar</span>
        )}
      </TD>
      {/* `max-w-md` acota la celda, y el envolvido va en los hijos: `cn` concatena sin resolver
          conflictos de Tailwind, así que un `whitespace-normal` puesto ACÁ pierde contra el
          `whitespace-nowrap` que TD trae de fábrica. En el hijo gana, porque ahí el nowrap solo
          llega heredado. Sin esto el aviso se corta en «…o si es mover plata d», justo donde
          empieza lo accionable, y un aviso truncado es un aviso que nadie lee. */}
      <TD className="max-w-md">
        {f.estado === "MAPEADA" ? (
          <div>
            <p className="tabular-nums text-content">{f.cuenta_codigo}</p>
            <p className="text-xs text-content-muted">
              {f.cuenta_nombre} · {f.cuenta_clase}
            </p>
            {f.cruzada && (
              <p className="mt-1 whitespace-normal break-words text-xs text-danger">
                ⚠ {f.aviso_cruce}
              </p>
            )}
          </div>
        ) : f.estado === "NO_CONTABILIZABLE" ? (
          <div>
            <Badge tone="neutral">No se contabiliza</Badge>
            {/* El motivo es lo que un auditor va a leer: se muestra entero, por lo mismo. */}
            <p className="mt-1 whitespace-normal break-words text-xs text-content-muted">
              {f.motivo}
            </p>
          </div>
        ) : (
          <span className="text-sm text-content-muted">falta decidir</span>
        )}
      </TD>
      <TD className="text-right">
        {puedeMapear && f.estado !== "" && (
          <Button variant="ghost" size="sm" onClick={onDeshacer}>
            Deshacer
          </Button>
        )}
      </TD>
    </TR>
  );
}
