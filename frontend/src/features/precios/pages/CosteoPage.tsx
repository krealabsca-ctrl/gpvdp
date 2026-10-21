/**
 * Pantalla — Costeo del plan (/precios/costeo).
 *
 * La receta de un plan y lo que resulta: costo variable, costo fijo, margen y —la cifra que más
 * se usa a diario— el PISO DE PRECIO, que es hasta dónde se puede descontar sin perder plata.
 *
 * ── Lo que esta pantalla se niega a hacer ───────────────────────────────────────────────────
 *
 * Si falta el costo de un solo insumo, NO muestra margen. Muestra el costo parcial, dice cuántos
 * faltan y los nombra. La razón es que un margen calculado sobre un costo incompleto sale más
 * alto que el real y empuja a vender barato: es el mismo defecto por el que el archivo de costeo
 * actual declara 80 % cuando su margen real es 47,6 %.
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
  useAgregarALaReceta,
  useCambiarCantidad,
  useCosteo,
  useInsumos,
  usePlanes,
  useQuitarDeLaReceta,
} from "../hooks";

const HOY = new Date().toISOString().slice(0, 10);

function money(v: string | null | undefined): string {
  if (v === null || v === undefined) return "—";
  const n = Number(v);
  if (Number.isNaN(n)) return v;
  return `₡${n.toLocaleString("es-CR", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

export function CosteoPage() {
  const tienePermiso = useTienePermiso();
  const puedeEditar = tienePermiso("precios.editar");
  const toast = useToast();

  const [planId, setPlanId] = useState<string | null>(null);
  const [fecha, setFecha] = useState(HOY);
  const [nuevaLinea, setNuevaLinea] = useState({ insumoId: "", cantidad: "1" });

  const planes = usePlanes();
  const costeo = useCosteo(planId, fecha);
  const insumos = useInsumos({ fecha });
  const agregar = useAgregarALaReceta();
  const cambiar = useCambiarCantidad();
  const quitar = useQuitarDeLaReceta();

  const c = costeo.data;

  async function onAgregar() {
    if (!planId || !nuevaLinea.insumoId) return;
    try {
      await agregar.mutateAsync({
        planId,
        insumoId: nuevaLinea.insumoId,
        cantidad: nuevaLinea.cantidad,
      });
      setNuevaLinea({ insumoId: "", cantidad: "1" });
    } catch (e) {
      toast.error(`No se pudo agregar: `);
    }
  }

  async function onCambiar(lineaId: string, cantidad: string) {
    try {
      await cambiar.mutateAsync({ lineaId, cantidad });
    } catch (e) {
      toast.error(`No se pudo cambiar: `);
    }
  }

  async function onQuitar(lineaId: string) {
    try {
      await quitar.mutateAsync(lineaId);
    } catch (e) {
      toast.error(`No se pudo quitar: `);
    }
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title="Costeo del plan"
        description="Qué lleva cada plan, cuánto cuesta y hasta dónde se puede descontar."
      />

      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 py-4">
          <Select
            label="Plan"
            value={planId ?? ""}
            onChange={(e) => setPlanId(e.target.value || null)}
            options={[
              { value: "", label: "Elegí un plan…" },
              ...(planes.data?.planes ?? []).map((p) => ({
                value: p.id,
                label: `${p.nombre} · ${p.familia}`,
              })),
            ]}
          />
          <Input
            label="Costeado al"
            type="date"
            value={fecha}
            onChange={(e) => setFecha(e.target.value)}
          />
          <div className="pb-2 text-xs text-content-muted">
            Cambiar la fecha recalcula con los costos que regían ese día.
          </div>
        </CardContent>
      </Card>

      {planes.isLoading && <LoadingState />}
      {planes.data && planes.data.planes.length === 0 && (
        <EmptyState message="Todavía no hay planes — Creá un plan para poder armar su receta y costearlo." />
      )}

      {!planId && planes.data && planes.data.planes.length > 0 && (
        <EmptyState message="Elegí un plan — Arriba, para ver su receta y su costeo." />
      )}

      {costeo.isLoading && planId && <LoadingState />}
      {costeo.isError && <ErrorState message={mensajeError(costeo.error)} />}

      {c && (
        <>
          {/* El aviso de cobertura va ANTES de los números: ver la cabecera del archivo. */}
          {c.lineas_sin_costo > 0 && (
            <div className="rounded-lg border border-negativo/40 bg-negativo/5 p-3 text-sm">
              <div className="font-semibold text-negativo">
                Este costeo está incompleto: {c.lineas_sin_costo} de {c.lineas_total} insumos no
                tienen costo.
              </div>
              <div className="mt-1">
                Por eso <b>no se muestra el margen</b>. El costo que ves abajo es solo de las{" "}
                {c.lineas_con_costo} líneas que sí tienen costo, así que el real es mayor.
              </div>
              <div className="mt-2 text-xs text-content-muted">
                Falta capturar: {c.insumos_sin_costo.join(" · ")}
              </div>
            </div>
          )}

          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">
                  Costo variable
                </div>
                <div className="text-xl font-bold tabular-nums">{money(c.costo_variable)}</div>
                <div className="text-xs text-content-muted">se consume con cada servicio</div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">Costo fijo</div>
                <div className="text-xl font-bold tabular-nums">{money(c.costo_fijo)}</div>
                <div className="text-xs text-content-muted">absorbido por reparto</div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">
                  Piso de precio
                </div>
                <div className="text-xl font-bold tabular-nums text-accent">
                  {money(c.piso_de_precio)}
                </div>
                <div className="text-xs text-content-muted">
                  por debajo de esto, cada venta pierde
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">Margen</div>
                {c.margen === null ? (
                  <>
                    <div className="text-xl font-bold text-content-muted">sin publicar</div>
                    <div className="text-xs text-negativo">falta costo de {c.lineas_sin_costo}</div>
                  </>
                ) : (
                  <>
                    <div
                      className={`text-xl font-bold tabular-nums ${Number(c.margen) < 0 ? "text-negativo" : "text-positivo"}`}
                    >
                      {money(c.margen)}
                    </div>
                    <div className="text-xs text-content-muted">
                      {c.margen_pct}% sobre {money(c.precio_referencia)}
                    </div>
                  </>
                )}
              </CardContent>
            </Card>
          </div>

          {c.plan.modalidad === "SUSCRIPCION" && (
            <div className="rounded-lg border border-border bg-surface-muted p-3 text-sm">
              <b>{c.plan.nombre}</b> es de suscripción, así que el precio de referencia no es la
              cuota: es <b>cuota × permanencia</b> ({money(c.plan.cuota_mensual)} ×{" "}
              {c.plan.permanencia_meses ?? "—"} meses = {money(c.precio_referencia)}). Comparar la
              cuota contra el costo de un servicio completo diría que el plan pierde millones cada
              mes, que no significa nada.
            </div>
          )}

          <Card>
            <CardHeader>
              <CardTitle>
                Receta · {c.plan.nombre}{" "}
                <span className="text-sm font-normal text-content-muted">
                  {c.lineas_con_costo}/{c.lineas_total} con costo ({c.cobertura_pct}%)
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {puedeEditar && (
                <div className="flex flex-wrap items-end gap-3">
                  <Select
                    label="Agregar insumo"
                    value={nuevaLinea.insumoId}
                    onChange={(e) => setNuevaLinea({ ...nuevaLinea, insumoId: e.target.value })}
                    options={[
                      { value: "", label: "Elegí…" },
                      ...(insumos.data?.insumos ?? []).map((i) => ({
                        value: i.id,
                        label: `${i.nombre}${i.costo_vigente === null ? " (sin costo)" : ""}`,
                      })),
                    ]}
                  />
                  <Input
                    label="Cantidad"
                    value={nuevaLinea.cantidad}
                    onChange={(e) => setNuevaLinea({ ...nuevaLinea, cantidad: e.target.value })}
                  />
                  <Button onClick={onAgregar} disabled={agregar.isPending || !nuevaLinea.insumoId}>
                    Agregar
                  </Button>
                </div>
              )}

              {c.lineas.length === 0 ? (
                <EmptyState message="La receta está vacía — Agregá los insumos que lleva este plan." />
              ) : (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Insumo</TH>
                        <TH>Tipo</TH>
                        <TH className="text-right">Cantidad</TH>
                        <TH className="text-right">Costo unit.</TH>
                        <TH className="text-right">Extendido</TH>
                        {puedeEditar && <TH></TH>}
                      </TR>
                    </THead>
                    <TBody>
                      {c.lineas.map((l) => (
                        <TR key={l.id}>
                          <TD>
                            <div className="font-medium">{l.nombre}</div>
                            <div className="font-mono text-xs text-content-muted">{l.codigo}</div>
                          </TD>
                          <TD>
                            <Badge tone={l.tipo_costo === "FIJO" ? "pendiente" : "neutral"}>
                              {l.tipo_costo === "FIJO" ? "Fijo" : "Variable"}
                            </Badge>
                          </TD>
                          <TD className="text-right">
                            {puedeEditar ? (
                              <input
                                className="w-20 rounded border border-border bg-transparent px-2 py-1 text-right tabular-nums"
                                defaultValue={l.cantidad}
                                onBlur={(e) => {
                                  if (e.target.value !== l.cantidad) {
                                    void onCambiar(l.id, e.target.value);
                                  }
                                }}
                              />
                            ) : (
                              <span className="tabular-nums">{l.cantidad}</span>
                            )}
                          </TD>
                          <TD className="text-right tabular-nums">
                            {l.costo_unitario === null ? (
                              <Badge tone="negativo">sin costo</Badge>
                            ) : (
                              money(l.costo_unitario)
                            )}
                          </TD>
                          <TD className="text-right tabular-nums font-medium">
                            {money(l.extendido)}
                          </TD>
                          {puedeEditar && (
                            <TD>
                              <Button variant="ghost" size="sm" onClick={() => onQuitar(l.id)}>
                                Quitar
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
        </>
      )}
    </div>
  );
}
