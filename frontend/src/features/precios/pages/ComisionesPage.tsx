/**
 * Pantalla — Comisiones (/precios/comisiones).
 *
 * ── LO QUE ESTA PANTALLA EXISTE PARA MOSTRAR ────────────────────────────────────────────────
 *
 * La comisión NO cuesta lo que recibe el vendedor. En nómina el concepto «Comisiones» está
 * marcado como afecta CCSS, afecta renta y afecta aguinaldo en las tres empresas —correcto,
 * porque en Costa Rica las comisiones habituales son salario—, así que el costo real lleva las
 * cargas patronales: ₡135.000 en el plan Oro le cuestan a la empresa ₡171.221.
 *
 * Son ₡36.221 por venta que el costeo actual no tiene, y el error va en todos los planes
 * directos. En los de suscripción el problema es el inverso: la comisión figura en CERO.
 *
 * Por eso el probador de abajo muestra las tres cifras separadas —bruto, carga y total— en vez
 * de un solo número: la diferencia entre ellas ES el hallazgo.
 */

import { useState } from "react";

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
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";

import {
  useComisionDelPlan,
  useCrearPolitica,
  useEliminarPolitica,
  usePlanes,
  usePoliticas,
  useVendedores,
} from "../hooks";

const HOY = new Date().toISOString().slice(0, 10);

function money(v: string | null | undefined): string {
  if (v === null || v === undefined) return "—";
  const n = Number(v);
  if (Number.isNaN(n)) return v;
  return `₡${n.toLocaleString("es-CR", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

const ETIQUETA_BASE: Record<string, string> = {
  PRECIO: "sobre el precio",
  MARGEN: "sobre el margen",
  PRIMERA_CUOTA: "sobre la primera cuota",
};

export function ComisionesPage() {
  const tienePermiso = useTienePermiso();
  const puedeEditar = tienePermiso("precios.editar");
  const toast = useToast();

  const politicas = usePoliticas();
  const planes = usePlanes();
  const vendedores = useVendedores();
  const crear = useCrearPolitica();
  const eliminar = useEliminarPolitica();

  const [nueva, setNueva] = useState({
    plan_id: "",
    vendedor_id: "",
    tipo: "PORCENTAJE",
    base: "PRECIO",
    valor: "",
    vigente_desde: HOY,
    notas: "",
  });

  // El probador: qué política aplica a esta combinación, y cuánto cuesta de verdad.
  const [prueba, setPrueba] = useState({ planId: "", vendedorId: "" });
  const comision = useComisionDelPlan(prueba.planId || null, prueba.vendedorId, HOY);

  async function onCrear() {
    try {
      await crear.mutateAsync({
        plan_id: nueva.plan_id || null,
        vendedor_id: nueva.vendedor_id || null,
        tipo: nueva.tipo,
        base: nueva.base,
        valor: nueva.valor,
        vigente_desde: nueva.vigente_desde,
        notas: nueva.notas,
      });
      setNueva({ ...nueva, plan_id: "", vendedor_id: "", valor: "", notas: "" });
      toast.success("Política guardada.");
    } catch (e) {
      toast.error(mensajeError(e));
    }
  }

  const lista = politicas.data?.politicas ?? [];
  // De la más específica a la más general: es el orden en que el servidor las resuelve, y verlas
  // así es lo que deja entender cuál va a ganar.
  const ordenadas = [...lista].sort((a, b) => b.especificidad - a.especificidad);

  function alcance(p: (typeof lista)[number]) {
    const partes: string[] = [];
    if (p.plan) partes.push(p.plan);
    if (p.vendedor) partes.push(p.vendedor);
    if (p.canal) partes.push(p.canal);
    if (partes.length === 0) return <Badge tone="neutral">todos los planes y vendedores</Badge>;
    return <span className="text-sm">{partes.join(" · ")}</span>;
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title="Comisiones"
        description="Cuánto se le paga a quién por vender qué, y cuánto le cuesta eso a la empresa."
      />

      {/* El hallazgo va arriba porque es la razón de la pantalla. */}
      <div className="rounded-lg border border-pendiente/40 bg-pendiente/5 p-3 text-sm">
        <b>La comisión cuesta más de lo que recibe el vendedor.</b> En nómina, «Comisiones» afecta
        CCSS, renta y aguinaldo —es lo que manda la ley—, así que el costo real lleva las cargas
        patronales. El sistema las toma de los parámetros de nómina; no hay que teclearlas ni
        recordarlas.
      </div>

      {politicas.isLoading && <LoadingState />}
      {politicas.isError && <ErrorState message={mensajeError(politicas.error)} />}

      {/* ── El probador ── */}
      <Card>
        <CardHeader>
          <CardTitle>Qué comisión aplica</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm text-content-muted">
            Gana la política <b>más específica</b>: la de este vendedor con este plan, si no la del
            plan, si no la del vendedor, si no la general.
          </p>
          <div className="flex flex-wrap items-end gap-3">
            <Select
              label="Plan"
              value={prueba.planId}
              onChange={(e) => setPrueba({ ...prueba, planId: e.target.value })}
              options={[
                { value: "", label: "Elegí un plan…" },
                ...(planes.data?.planes ?? []).map((p) => ({ value: p.id, label: p.nombre })),
              ]}
            />
            <Select
              label="Vendedor (opcional)"
              value={prueba.vendedorId}
              onChange={(e) => setPrueba({ ...prueba, vendedorId: e.target.value })}
              options={[
                { value: "", label: "Sin vendedor — usa la del plan" },
                ...(vendedores.data?.vendedores ?? []).map((v) => ({ value: v.id, label: v.nombre })),
              ]}
            />
          </div>

          {prueba.planId && comision.isLoading && <LoadingState />}

          {prueba.planId && comision.data && comision.data.comision === null && (
            <div className="rounded-lg border border-negativo/40 bg-negativo/5 p-3 text-sm">
              <b>No hay política para este plan.</b> {comision.data.motivo}
              <div className="mt-1 text-xs text-content-muted">
                No se asume comisión cero: eso diría que la venta no paga comisión y el margen
                saldría más alto que el real.
              </div>
            </div>
          )}

          {prueba.planId && comision.data?.comision && (
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <Card>
                <CardContent className="py-3">
                  <div className="text-xs uppercase tracking-wide text-content-muted">Base</div>
                  <div className="text-base font-bold tabular-nums lg:text-lg">
                    {money(comision.data.comision.monto_base)}
                  </div>
                  <div className="text-xs text-content-muted">
                    {ETIQUETA_BASE[comision.data.comision.base] ?? comision.data.comision.base}
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardContent className="py-3">
                  <div className="text-xs uppercase tracking-wide text-content-muted">
                    Recibe el vendedor
                  </div>
                  <div className="text-base font-bold tabular-nums lg:text-lg">
                    {money(comision.data.comision.bruto)}
                  </div>
                  <div className="text-xs text-content-muted">
                    {comision.data.comision.tipo === "PORCENTAJE"
                      ? `${comision.data.comision.valor}%`
                      : "monto fijo"}
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardContent className="py-3">
                  <div className="text-xs uppercase tracking-wide text-content-muted">
                    Carga patronal
                  </div>
                  <div className="text-base font-bold tabular-nums text-pendiente lg:text-lg">
                    {money(comision.data.comision.carga_patronal)}
                  </div>
                  <div className="text-xs text-content-muted">
                    {comision.data.comision.factor_carga
                      ? `${comision.data.comision.factor_carga}% · de nómina`
                      : "—"}
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardContent className="py-3">
                  <div className="text-xs uppercase tracking-wide text-content-muted">
                    Le cuesta a la empresa
                  </div>
                  {comision.data.comision.costo_total === null ? (
                    <>
                      <div className="text-base font-bold text-content-muted lg:text-lg">
                        incompleto
                      </div>
                      <div className="text-xs text-negativo">{comision.data.comision.motivo}</div>
                    </>
                  ) : (
                    <>
                      <div className="text-base font-bold tabular-nums text-accent lg:text-lg">
                        {money(comision.data.comision.costo_total)}
                      </div>
                      <div className="text-xs text-content-muted">bruto + carga</div>
                    </>
                  )}
                </CardContent>
              </Card>
            </div>
          )}
        </CardContent>
      </Card>

      {/* ── Crear ── */}
      {puedeEditar && (
        <Card>
          <CardHeader>
            <CardTitle>Agregar una política</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-wrap items-end gap-3">
            <Select
              label="Plan"
              value={nueva.plan_id}
              onChange={(e) => setNueva({ ...nueva, plan_id: e.target.value })}
              options={[
                { value: "", label: "Todos los planes" },
                ...(planes.data?.planes ?? []).map((p) => ({ value: p.id, label: p.nombre })),
              ]}
            />
            <Select
              label="Vendedor"
              value={nueva.vendedor_id}
              onChange={(e) => setNueva({ ...nueva, vendedor_id: e.target.value })}
              options={[
                { value: "", label: "Todos los vendedores" },
                ...(vendedores.data?.vendedores ?? []).map((v) => ({ value: v.id, label: v.nombre })),
              ]}
            />
            <Select
              label="Tipo"
              value={nueva.tipo}
              onChange={(e) => setNueva({ ...nueva, tipo: e.target.value })}
              options={[
                { value: "PORCENTAJE", label: "Porcentaje" },
                { value: "MONTO_FIJO", label: "Monto fijo" },
              ]}
            />
            <Select
              label="Calculada sobre"
              value={nueva.base}
              onChange={(e) => setNueva({ ...nueva, base: e.target.value })}
              options={[
                { value: "PRECIO", label: "El precio de venta" },
                { value: "MARGEN", label: "El margen — alinea con la rentabilidad" },
                { value: "PRIMERA_CUOTA", label: "La primera cuota — para suscripción" },
              ]}
            />
            <Input
              label={nueva.tipo === "PORCENTAJE" ? "Porcentaje" : "Monto"}
              value={nueva.valor}
              onChange={(e) => setNueva({ ...nueva, valor: e.target.value })}
            />
            <Input
              label="Rige desde"
              type="date"
              value={nueva.vigente_desde}
              onChange={(e) => setNueva({ ...nueva, vigente_desde: e.target.value })}
            />
            <Button onClick={onCrear} disabled={crear.isPending || !nueva.valor}>
              Guardar
            </Button>
            <div className="pb-2 text-xs text-content-muted">
              Cambiar una política no reescribe la historia: lo costeado antes sigue con la que
              regía entonces.
            </div>
          </CardContent>
        </Card>
      )}

      {/* ── Las políticas ── */}
      <Card>
        <CardHeader>
          <CardTitle>
            Políticas{" "}
            <span className="text-sm font-normal text-content-muted">
              de la más específica a la más general
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {ordenadas.length === 0 ? (
            <EmptyState message="Todavía no hay políticas de comisión — sin ellas, el costeo de los planes no la incluye." />
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Aplica a</TH>
                    <TH className="text-right">Valor</TH>
                    <TH>Calculada sobre</TH>
                    <TH>Vigencia</TH>
                    <TH>Nota</TH>
                    {puedeEditar && <TH></TH>}
                  </TR>
                </THead>
                <TBody>
                  {ordenadas.map((p) => (
                    <TR key={p.id}>
                      <TD>{alcance(p)}</TD>
                      <TD className="text-right tabular-nums font-medium">
                        {p.tipo === "PORCENTAJE" ? `${p.valor}%` : money(p.valor)}
                      </TD>
                      <TD className="text-sm">{ETIQUETA_BASE[p.base] ?? p.base}</TD>
                      <TD className="text-xs text-content-muted">
                        desde {p.vigente_desde}
                        {p.vigente_hasta ? ` hasta ${p.vigente_hasta}` : ""}
                      </TD>
                      <TD className="text-xs text-content-muted">{p.notas || "—"}</TD>
                      {puedeEditar && (
                        <TD>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() =>
                              void eliminar
                                .mutateAsync(p.id)
                                .catch((e) => toast.error(mensajeError(e)))
                            }
                          >
                            Eliminar
                          </Button>
                        </TD>
                      )}
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
