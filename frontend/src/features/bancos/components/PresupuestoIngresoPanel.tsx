/**
 * Presupuesto de INGRESO y resultado presupuestado (mig 0116).
 *
 * ── POR QUÉ ESTO VA ARRIBA DEL CONTROL DE GASTO ─────────────────────────────────────────────
 *
 * Porque es la pregunta de la que cuelga todo lo demás: «¿vamos a cerrar como dijimos?». El
 * control de gasto contesta la mitad —cuánto llevamos gastado— y con media meta no se puede
 * afirmar que se va bien.
 *
 * ── EL INGRESO SE LEE AL REVÉS QUE EL GASTO ─────────────────────────────────────────────────
 *
 * Consumir el 95 % del presupuesto de gasto es una alerta; llegar al 95 % de la meta de ingreso
 * es casi un logro. Los colores y los textos de esta sección NO se copian de la de gasto: un
 * semáforo heredado pintaría de rojo un mes bueno.
 */

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ErrorState,
  Input,
  LoadingState,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { mensajeError } from "@/lib/apiError";
import { etiquetaPeriodo, formatMoneda, formatMonto } from "@/lib/format";
import { queryKeys } from "@/api/queryKeys";
import { presupuestoIngresoApi, type EstadoIngreso, type FilaIngreso } from "@/api/bancos";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { useTienePermiso } from "@/features/auth/permisos";

/** Cómo se pinta cada estado. Distinto del gasto a propósito: acá pasarse es bueno. */
const TONO: Record<EstadoIngreso, { clase: string; texto: string }> = {
  CUMPLIDO: { clase: "text-positivo", texto: "cumplida" },
  EN_CAMINO: { clase: "text-content", texto: "en camino" },
  CORTO: { clase: "text-negativo", texto: "corto" },
  SIN_META: { clase: "text-content-muted", texto: "sin meta" },
};

export function PresupuestoIngresoPanel({ desde, hasta }: { desde: string; hasta: string }) {
  const empresaId = useEmpresaId();
  const puedeEditar = useTienePermiso()("bancos.ajustes");
  const [abierto, setAbierto] = useState(false);

  const ctrlQ = useQuery({
    queryKey: queryKeys.bancos.controlIngreso(empresaId, desde, hasta),
    queryFn: () => presupuestoIngresoApi.control(desde, hasta),
  });
  const resQ = useQuery({
    queryKey: queryKeys.bancos.resultadoPresupuestado(empresaId, desde, hasta),
    queryFn: () => presupuestoIngresoApi.resultado(desde, hasta),
  });

  const ctrl = ctrlQ.data;
  const res = resQ.data;

  return (
    <div className="flex flex-col gap-4">
      {/* EL RESULTADO PRESUPUESTADO: lo que no se podía calcular con media meta. */}
      {resQ.isLoading ? (
        <LoadingState label="Cruzando las dos mitades de la meta…" />
      ) : resQ.isError ? (
        <ErrorState message={mensajeError(resQ.error)} onRetry={() => resQ.refetch()} />
      ) : res ? (
        <Card>
          <CardHeader>
            <CardTitle>
              Resultado presupuestado · {etiquetaPeriodo(desde)} a {etiquetaPeriodo(hasta)}
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <Cifra titulo="Ingreso" meta={res.ingreso_meta} real={res.ingreso_real} masEsMejor />
              <Cifra titulo="Gasto" meta={res.gasto_meta} real={res.gasto_real} />
              <Cifra
                titulo="Resultado"
                meta={res.resultado_meta}
                real={res.resultado_real}
                masEsMejor
                fuerte
              />
              {/* SIN META NO HAY DESVÍO QUE LEER. Con todo en cero, «se cerró mejor de lo
                  planeado» es una afirmación sobre un plan que no existe — y es justo la frase
                  que alguien repite en una reunión. */}
              <div>
                <p className="text-xs text-content-muted">Desvío</p>
                <p
                  className={cn(
                    "text-xl font-semibold tabular-nums",
                    !res.completo
                      ? "text-content-muted"
                      : res.desvio.startsWith("-")
                        ? "text-negativo"
                        : "text-positivo",
                  )}
                >
                  {res.completo ? formatMoneda(res.desvio) : "—"}
                </p>
                <p className="text-xs text-content-muted">
                  {!res.completo
                    ? "hace falta la meta de los dos lados"
                    : res.desvio.startsWith("-")
                      ? "se cerró peor de lo planeado"
                      : "se cerró mejor de lo planeado"}
                </p>
              </div>
            </div>
            {res.aviso && <p className="text-sm text-pendiente">{res.aviso}</p>}
          </CardContent>
        </Card>
      ) : null}

      {/* EL CONTROL DE INGRESO: meta contra real, por línea. */}
      {ctrlQ.isLoading ? null : ctrlQ.isError ? (
        <ErrorState message={mensajeError(ctrlQ.error)} onRetry={() => ctrlQ.refetch()} />
      ) : ctrl ? (
        <Card>
          <CardHeader>
            <CardTitle>Meta de ingreso</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex flex-wrap items-end gap-x-10 gap-y-3">
              <div>
                <p className="text-xs text-content-muted">Meta del rango</p>
                <p className="text-lg font-semibold tabular-nums text-content">
                  {formatMoneda(ctrl.meta_total)}
                </p>
                <p className="text-xs text-content-muted">
                  {ctrl.meses_con_meta} mes(es) con meta declarada
                </p>
              </div>
              <div>
                <p className="text-xs text-content-muted">Entró de verdad</p>
                <p className="text-lg font-semibold tabular-nums text-content">
                  {formatMoneda(ctrl.real_total)}
                </p>
              </div>
              {ctrl.cumplido_pct && (
                <div>
                  <p className="text-xs text-content-muted">Cumplimiento</p>
                  <p className={cn("text-lg font-semibold tabular-nums", TONO[ctrl.estado].clase)}>
                    {ctrl.cumplido_pct} %
                  </p>
                  <p className="text-xs text-content-muted">meta {TONO[ctrl.estado].texto}</p>
                </div>
              )}
              {puedeEditar && (
                <Button variant="secondary" className="ml-auto" onClick={() => setAbierto(!abierto)}>
                  {abierto ? "Cerrar" : "Cargar metas"}
                </Button>
              )}
            </div>

            {ctrl.aviso && <p className="text-sm text-pendiente">{ctrl.aviso}</p>}

            {abierto && puedeEditar && <CargarMetas hasta={hasta} />}

            {ctrl.lineas.length > 0 && <TablaLineas lineas={ctrl.lineas} />}
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}

/** Meta contra real de una cifra, con su lectura. */
function Cifra({
  titulo,
  meta,
  real,
  masEsMejor,
  fuerte,
}: {
  titulo: string;
  meta: string;
  real: string;
  /** En ingreso y resultado, quedarse corto es malo. En gasto, pasarse. */
  masEsMejor?: boolean;
  fuerte?: boolean;
}) {
  const m = Number(meta);
  const r = Number(real);
  // Sin meta no se juzga: nadie dijo qué se esperaba.
  const hayMeta = m !== 0;
  const bien = masEsMejor ? r >= m : r <= m;
  return (
    <div>
      <p className="text-xs text-content-muted">{titulo}</p>
      <p
        className={cn(
          "font-semibold tabular-nums",
          fuerte ? "text-xl" : "text-lg",
          hayMeta ? (bien ? "text-positivo" : "text-negativo") : "text-content",
        )}
      >
        {formatMoneda(real)}
      </p>
      <p className="text-xs text-content-muted">
        {hayMeta ? `meta ${formatMoneda(meta)}` : "sin meta declarada"}
      </p>
    </div>
  );
}

/** El desglose por línea. */
function TablaLineas({ lineas }: { lineas: FilaIngreso[] }) {
  return (
    <TableContainer>
      <Table>
        <THead>
          <TR>
            <TH>Línea de ingreso</TH>
            <TH className="text-right">Meta</TH>
            <TH className="text-right">Entró</TH>
            <TH className="text-right">Diferencia</TH>
            <TH className="text-right">Cumplido</TH>
          </TR>
        </THead>
        <TBody>
          {lineas.map((l) => (
            <TR key={l.clasificacion_id}>
              <TD>
                <span className="text-content">{l.clasificacion}</span>
                {l.movs > 0 && (
                  <span className="ml-2 text-xs text-content-muted">{l.movs} mov.</span>
                )}
              </TD>
              <TD className="text-right tabular-nums text-content-muted">
                {l.meta ? formatMonto(l.meta) : "—"}
              </TD>
              <TD className="text-right tabular-nums text-content">{formatMonto(l.real)}</TD>
              <TD
                className={cn(
                  "text-right tabular-nums",
                  !l.diferencia
                    ? "text-content-muted"
                    : l.diferencia.startsWith("-")
                      ? "text-negativo"
                      : "text-positivo",
                )}
              >
                {l.diferencia ? formatMonto(l.diferencia) : "—"}
              </TD>
              <TD className="text-right">
                {l.cumplido_pct ? (
                  <Badge tone={l.estado === "CUMPLIDO" ? "positivo" : l.estado === "CORTO" ? "negativo" : "neutral"}>
                    {l.cumplido_pct} %
                  </Badge>
                ) : (
                  <span className="text-xs text-content-muted">sin meta</span>
                )}
              </TD>
            </TR>
          ))}
        </TBody>
      </Table>
    </TableContainer>
  );
}

/**
 * Cargar la meta total y, si se quiere, proponerla desde el histórico.
 *
 * Toma solo `hasta` y no el rango: una meta es de UN mes. El rango sirve para mirar el acumulado,
 * pero declarar «la meta de estos seis meses» en una sola cifra haría imposible después saber qué
 * mes falló.
 */
function CargarMetas({ hasta }: { hasta: string }) {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();
  const [monto, setMonto] = useState("");
  const [nota, setNota] = useState("");
  const [ajuste, setAjuste] = useState("0");

  const guardar = useMutation({
    mutationFn: () => presupuestoIngresoApi.guardar({ periodo: hasta, monto, nota }),
    onSuccess: () => {
      toast.success(`Meta de ${etiquetaPeriodo(hasta)} guardada.`);
      setMonto("");
      void qc.invalidateQueries({ queryKey: queryKeys.bancos.raiz(empresaId) });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const propuesta = useMutation({
    mutationFn: () => presupuestoIngresoApi.propuesta(hasta, hasta, ajuste),
    onSuccess: (p) => {
      if (p.aviso) {
        toast.error(p.aviso);
        return;
      }
      // La propuesta es la SUMA de las líneas del mismo mes del año anterior, ajustada. Se carga
      // en el campo para que se revise, no se guarda sola: el número que queda es el que alguien
      // confirma.
      const total = p.items.reduce((acc, i) => acc + Number(i.meta || 0), 0);
      setMonto(String(Math.round(total)));
      setNota(`Propuesta desde ${p.desde_base} con ${ajuste} % de ajuste`);
      toast.success(`Propuesta cargada desde ${p.desde_base}. Revisala antes de guardar.`);
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-border bg-surface-muted/40 p-4">
      <p className="text-sm text-content-muted">
        La meta es del mes <b className="text-content">{etiquetaPeriodo(hasta)}</b>. Con 41 líneas
        de ingreso, cargar el total alcanza para controlar el resultado; el desglose por línea es
        opcional.
      </p>

      <div className="flex flex-wrap items-end gap-3">
        <Input
          label="Meta del mes"
          id="meta-ingreso-monto"
          inputMode="decimal"
          value={monto}
          onChange={(e) => setMonto(e.target.value)}
          className="w-44"
        />
        <Input
          label="Nota"
          id="meta-ingreso-nota"
          value={nota}
          onChange={(e) => setNota(e.target.value)}
        />
        <Button loading={guardar.isPending} disabled={!monto} onClick={() => guardar.mutate()}>
          Guardar
        </Button>
      </div>

      <div className="flex flex-wrap items-end gap-3 border-t border-border pt-3">
        <Input
          label="Ajuste sobre el año pasado (%)"
          id="meta-ingreso-ajuste"
          inputMode="decimal"
          value={ajuste}
          onChange={(e) => setAjuste(e.target.value)}
          className="w-56"
        />
        <Button
          variant="secondary"
          loading={propuesta.isPending}
          onClick={() => propuesta.mutate()}
        >
          Proponer desde el histórico
        </Button>
        <p className="text-xs text-content-muted">
          Toma lo que entró el mismo mes del año anterior y le aplica el ajuste. Llena el campo
          para que lo revises; no guarda nada.
        </p>
      </div>
    </div>
  );
}
