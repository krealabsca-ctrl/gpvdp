/**
 * Pantalla — Reparto de indirectos (/precios/reparto).
 *
 * ── EL ORDEN DE LA PANTALLA ES EL ARGUMENTO ─────────────────────────────────────────────────
 *
 * Primero lo que está MAL repartido, después lo que falta repartir, al final lo que ya funciona.
 * No es estético: un reparto cuyos porcentajes no suman 100 % se absorbe de más o de menos, y
 * eso no se nota mirando el total —los números son correctos, la suma de las partes no—. Si esa
 * lista queda abajo, nadie la ve.
 *
 * ── LO QUE ESTA PANTALLA REEMPLAZA ──────────────────────────────────────────────────────────
 *
 * En el archivo de costeo el indirecto es una línea llamada «Indirectos Global» con la casilla
 * de costo VACÍA: el monto se escribe directo en el bloque de resultados, ₡102.399 para los
 * planes directos y ₡1.217.580 para los de suscripción. Un mismo «insumo» con dos costos según
 * dónde se use no es un costo, es un ajuste que nadie puede auditar. Acá cada colón que un plan
 * absorbe se puede abrir: de qué gasto salió, por qué driver pasó y contra qué base se dividió.
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
  useCrearDriver,
  useDrivers,
  useEliminarDriver,
  useFijarBase,
  useFijarReparto,
  useQuitarReparto,
  useReparto,
  useRepartosDeGasto,
} from "../hooks";

/** Primer y último día del mes anterior: el período cerrado más reciente. */
function mesAnterior(): { desde: string; hasta: string } {
  const hoy = new Date();
  const fin = new Date(hoy.getFullYear(), hoy.getMonth(), 0);
  const ini = new Date(fin.getFullYear(), fin.getMonth(), 1);
  const f = (d: Date) => d.toISOString().slice(0, 10);
  return { desde: f(ini), hasta: f(fin) };
}

// Los KPIs llevan el monto SIN decimales: con cifras de ocho dígitos los céntimos no aportan
// nada y empujan el número fuera de la tarjeta, que es peor que redondear. El detalle con
// decimales queda en las tablas, donde sí hay ancho.
function moneyCorto(v: string | null | undefined): string {
  if (v === null || v === undefined) return "—";
  const n = Number(v);
  if (Number.isNaN(n)) return v;
  return `₡${n.toLocaleString("es-CR", { maximumFractionDigits: 0 })}`;
}

function money(v: string | null | undefined): string {
  if (v === null || v === undefined) return "—";
  const n = Number(v);
  if (Number.isNaN(n)) return v;
  return `₡${n.toLocaleString("es-CR", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function pct(parte: string, total: string): string {
  const p = Number(parte);
  const t = Number(total);
  if (!t || Number.isNaN(p) || Number.isNaN(t)) return "";
  return `${((p / t) * 100).toFixed(1)}%`;
}

export function RepartoPage() {
  const tienePermiso = useTienePermiso();
  const puedeEditar = tienePermiso("precios.editar");
  const toast = useToast();

  const inicial = mesAnterior();
  const [desde, setDesde] = useState(inicial.desde);
  const [hasta, setHasta] = useState(inicial.hasta);
  const [verTodas, setVerTodas] = useState(false);

  const reparto = useReparto(desde, hasta);
  const detalle = useRepartosDeGasto(desde, hasta);
  const drivers = useDrivers();

  const crearDriver = useCrearDriver();
  const eliminarDriver = useEliminarDriver();
  const fijarBase = useFijarBase();
  const fijarReparto = useFijarReparto();
  const quitarReparto = useQuitarReparto();

  const [nuevoDriver, setNuevoDriver] = useState({ codigo: "", nombre: "", unidad: "", descripcion: "" });
  const [base, setBase] = useState<{ driverId: string; cantidad: string }>({ driverId: "", cantidad: "" });
  const [asignar, setAsignar] = useState<{ clasifId: string; driverId: string; pct: string }>({
    clasifId: "",
    driverId: "",
    pct: "",
  });

  const r = reparto.data;
  const todos = detalle.data?.repartos ?? [];
  const malRepartidos = todos.filter((g) => !g.valido && g.lineas.length > 0);
  const sinRepartir = todos.filter((g) => g.lineas.length === 0 && Number(g.gasto_del_periodo) > 0);
  const repartidos = todos.filter((g) => g.valido);

  async function onCrearDriver() {
    try {
      await crearDriver.mutateAsync({
        codigo: nuevoDriver.codigo,
        nombre: nuevoDriver.nombre,
        unidad: nuevoDriver.unidad || "UNIDAD",
        descripcion: nuevoDriver.descripcion,
      });
      setNuevoDriver({ codigo: "", nombre: "", unidad: "", descripcion: "" });
      toast.success("Driver creado. Falta fijarle la base del período.");
    } catch (e) {
      toast.error(mensajeError(e));
    }
  }

  async function onFijarBase() {
    try {
      await fijarBase.mutateAsync({
        driverId: base.driverId,
        periodo: desde,
        cantidad: base.cantidad,
      });
      setBase({ driverId: "", cantidad: "" });
      toast.success("Base del período guardada.");
    } catch (e) {
      toast.error(mensajeError(e));
    }
  }

  async function onAsignar() {
    try {
      await fijarReparto.mutateAsync({
        clasificacion_id: asignar.clasifId,
        driver_id: asignar.driverId,
        porcentaje: asignar.pct,
        vigente_desde: desde,
      });
      setAsignar({ clasifId: "", driverId: "", pct: "" });
    } catch (e) {
      toast.error(mensajeError(e));
    }
  }

  function filaDeGasto(g: (typeof todos)[number]) {
    return (
      <TR key={g.clasificacion_id}>
        <TD>
          <div className="font-medium">{g.clasificacion}</div>
          <div className="text-xs text-content-muted">{g.concepto}</div>
        </TD>
        <TD className="text-right tabular-nums">{money(g.gasto_del_periodo)}</TD>
        <TD>
          {g.lineas.length === 0 ? (
            <span className="text-xs text-content-muted">sin repartir</span>
          ) : (
            <div className="flex flex-wrap gap-1">
              {g.lineas.map((l) => (
                <span
                  key={l.id}
                  className="inline-flex items-center gap-1 rounded-full bg-surface-muted px-2 py-0.5 text-xs"
                >
                  {l.driver} {l.porcentaje}%
                  {puedeEditar && (
                    <button
                      type="button"
                      onClick={() => void quitarReparto.mutateAsync(l.id)}
                      className="text-content-muted hover:text-negativo"
                      aria-label={`Quitar ${l.driver}`}
                    >
                      ×
                    </button>
                  )}
                </span>
              ))}
            </div>
          )}
        </TD>
        <TD className="text-right tabular-nums">
          {g.lineas.length === 0 ? (
            "—"
          ) : (
            <span className={g.valido ? "text-positivo" : "text-negativo"}>{g.suma_pct}%</span>
          )}
        </TD>
        <TD className="text-xs">
          {g.valido ? (
            <Badge tone="positivo">reparte</Badge>
          ) : (
            <span className="text-negativo">{g.motivo}</span>
          )}
        </TD>
      </TR>
    );
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title="Reparto de indirectos"
        description="Cada gasto se reparte por lo que lo causa. El monto sale de Bancos, no se teclea."
      />

      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 py-4">
          <Input label="Desde" type="date" value={desde} onChange={(e) => setDesde(e.target.value)} />
          <Input label="Hasta" type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} />
          <div className="pb-2 text-xs text-content-muted">
            El gasto del período sale de los movimientos clasificados en Bancos.
          </div>
        </CardContent>
      </Card>

      {(reparto.isLoading || detalle.isLoading) && <LoadingState />}
      {reparto.isError && <ErrorState message={mensajeError(reparto.error)} />}

      {r && (
        <>
          {/* 4 columnas solo cuando hay ancho de verdad: a 1024 px con la sidebar abierta cada
              tarjeta queda en 101 px y un monto de ocho dígitos se corta. Medido. */}
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">Gasto del período</div>
                <div className="text-base font-bold tabular-nums lg:text-xl">{moneyCorto(r.gasto_total)}</div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">Repartido</div>
                <div className="text-base font-bold tabular-nums lg:text-xl text-positivo">
                  {moneyCorto(r.gasto_repartido)}
                </div>
                <div className="text-xs text-content-muted">
                  {pct(r.gasto_repartido, r.gasto_total)} del gasto
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">Sin asignar</div>
                <div className="text-base font-bold tabular-nums lg:text-xl text-pendiente">
                  {moneyCorto(r.gasto_sin_asignar)}
                </div>
                <div className="text-xs text-content-muted">{sinRepartir.length} clasificaciones</div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">Mal repartido</div>
                <div
                  className={`text-base font-bold tabular-nums lg:text-xl ${Number(r.gasto_en_repartos_invalidos) > 0 ? "text-negativo" : ""}`}
                >
                  {moneyCorto(r.gasto_en_repartos_invalidos)}
                </div>
                <div className="text-xs text-content-muted">{malRepartidos.length} a corregir</div>
              </CardContent>
            </Card>
          </div>

          {/* Lo que está MAL va primero: ver la cabecera del archivo. */}
          {malRepartidos.length > 0 && (
            <Card className="border-negativo/40">
              <CardHeader>
                <CardTitle className="text-negativo">
                  {malRepartidos.length} gasto{malRepartidos.length > 1 ? "s" : ""} mal repartido
                  {malRepartidos.length > 1 ? "s" : ""}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <p className="text-sm">
                  Sus porcentajes no suman 100 %, así que <b>no se usan</b>: su gasto queda sin
                  absorber. Repartir de más infla el costo de todo lo que se vende; repartir de
                  menos lo desinfla, y ninguna de las dos cosas se nota mirando el total.
                </p>
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Gasto</TH>
                        <TH className="text-right">Monto</TH>
                        <TH>Reparto</TH>
                        <TH className="text-right">Suma</TH>
                        <TH>Qué pasa</TH>
                      </TR>
                    </THead>
                    <TBody>{malRepartidos.map(filaDeGasto)}</TBody>
                  </Table>
                </TableContainer>
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader>
              <CardTitle>Bolsas del período</CardTitle>
            </CardHeader>
            <CardContent>
              {r.bolsas.length === 0 ? (
                <EmptyState message="Todavía no hay gasto repartido — asigná abajo qué driver causa cada gasto." />
              ) : (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Driver</TH>
                        <TH className="text-right">Gasto acumulado</TH>
                        <TH className="text-right">Base del período</TH>
                        <TH className="text-right">Costo unitario</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {r.bolsas.map((b) => (
                        <TR key={b.driver_id}>
                          <TD>
                            <div className="font-medium">{b.driver}</div>
                            <div className="text-xs text-content-muted">por {b.unidad}</div>
                          </TD>
                          <TD className="text-right tabular-nums">{money(b.gasto)}</TD>
                          <TD className="text-right tabular-nums">{b.base ?? "—"}</TD>
                          <TD className="text-right tabular-nums font-medium">
                            {b.costo_unitario === null ? (
                              <span className="text-xs text-negativo">{b.motivo}</span>
                            ) : (
                              money(b.costo_unitario)
                            )}
                          </TD>
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

      {/* ── Drivers ── */}
      <Card>
        <CardHeader>
          <CardTitle>Drivers</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm text-content-muted">
            Un driver es <b>lo que causa</b> que un gasto exista. Nombrarlo es lo que permite
            defender el reparto ante auditoría, en vez de un porcentaje suelto.
          </p>

          {puedeEditar && (
            <div className="flex flex-wrap items-end gap-3 rounded-lg border border-border p-3">
              <Input
                label="Código *"
                value={nuevoDriver.codigo}
                onChange={(e) => setNuevoDriver({ ...nuevoDriver, codigo: e.target.value.toUpperCase() })}
              />
              <Input
                label="Nombre *"
                placeholder="Servicios atendidos"
                value={nuevoDriver.nombre}
                onChange={(e) => setNuevoDriver({ ...nuevoDriver, nombre: e.target.value })}
              />
              <Input
                label="Unidad"
                placeholder="servicio"
                value={nuevoDriver.unidad}
                onChange={(e) => setNuevoDriver({ ...nuevoDriver, unidad: e.target.value })}
              />
              <Input
                label="Qué lo causa"
                placeholder="sala, carroza y personal de velación"
                value={nuevoDriver.descripcion}
                onChange={(e) => setNuevoDriver({ ...nuevoDriver, descripcion: e.target.value })}
              />
              <Button
                onClick={onCrearDriver}
                disabled={crearDriver.isPending || !nuevoDriver.codigo || !nuevoDriver.nombre}
              >
                Agregar
              </Button>
            </div>
          )}

          {drivers.data && drivers.data.drivers.length > 0 && (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Driver</TH>
                    <TH>Qué lo causa</TH>
                    <TH className="text-right">Base del período</TH>
                    <TH className="text-right">En uso</TH>
                    {puedeEditar && <TH></TH>}
                  </TR>
                </THead>
                <TBody>
                  {drivers.data.drivers.map((d) => (
                    <TR key={d.id}>
                      <TD>
                        <div className="font-medium">{d.nombre}</div>
                        <div className="font-mono text-xs text-content-muted">
                          {d.codigo} · por {d.unidad}
                        </div>
                      </TD>
                      <TD className="text-xs text-content-muted">{d.descripcion || "—"}</TD>
                      <TD className="text-right tabular-nums">
                        {d.base_del_periodo ?? <Badge tone="negativo">falta</Badge>}
                      </TD>
                      <TD className="text-right text-xs text-content-muted">
                        {d.gastos} gasto{d.gastos === 1 ? "" : "s"} · {d.planes} plan
                        {d.planes === 1 ? "" : "es"}
                      </TD>
                      {puedeEditar && (
                        <TD>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => void eliminarDriver.mutateAsync(d.id).catch((e) => toast.error(mensajeError(e)))}
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

          {puedeEditar && drivers.data && drivers.data.drivers.length > 0 && (
            <div className="flex flex-wrap items-end gap-3 border-t border-border pt-3">
              <Select
                label="Fijar la base de"
                value={base.driverId}
                onChange={(e) => setBase({ ...base, driverId: e.target.value })}
                options={[
                  { value: "", label: "Elegí un driver…" },
                  ...drivers.data.drivers.map((d) => ({ value: d.id, label: `${d.nombre} (${d.unidad})` })),
                ]}
              />
              <Input
                label={`Cantidad del período (${desde.slice(0, 7)})`}
                value={base.cantidad}
                onChange={(e) => setBase({ ...base, cantidad: e.target.value })}
              />
              <Button onClick={onFijarBase} disabled={!base.driverId || !base.cantidad || fijarBase.isPending}>
                Guardar
              </Button>
              <div className="pb-2 text-xs text-content-muted">
                Se mide del período, no se presupuesta: con un divisor fijo, si baja la venta sube
                el costo unitario.
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* ── Asignar gasto a driver ── */}
      {puedeEditar && drivers.data && drivers.data.drivers.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Asignar un gasto a un driver</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-wrap items-end gap-3">
            <Select
              label="Gasto"
              value={asignar.clasifId}
              onChange={(e) => setAsignar({ ...asignar, clasifId: e.target.value })}
              options={[
                { value: "", label: "Elegí un gasto…" },
                ...todos.map((g) => ({
                  value: g.clasificacion_id,
                  label: `${g.clasificacion} — ${money(g.gasto_del_periodo)}`,
                })),
              ]}
            />
            <Select
              label="Driver"
              value={asignar.driverId}
              onChange={(e) => setAsignar({ ...asignar, driverId: e.target.value })}
              options={[
                { value: "", label: "Elegí…" },
                ...drivers.data.drivers.map((d) => ({ value: d.id, label: d.nombre })),
              ]}
            />
            <Input
              label="Porcentaje"
              placeholder="100"
              value={asignar.pct}
              onChange={(e) => setAsignar({ ...asignar, pct: e.target.value })}
            />
            <Button
              onClick={onAsignar}
              disabled={!asignar.clasifId || !asignar.driverId || !asignar.pct || fijarReparto.isPending}
            >
              Asignar
            </Button>
          </CardContent>
        </Card>
      )}

      {/* ── El resto del gasto ── */}
      {todos.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>
              Gasto del período{" "}
              <span className="text-sm font-normal text-content-muted">
                {repartidos.length} repartidos · {sinRepartir.length} sin asignar
              </span>
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Gasto</TH>
                    <TH className="text-right">Monto</TH>
                    <TH>Reparto</TH>
                    <TH className="text-right">Suma</TH>
                    <TH>Qué pasa</TH>
                  </TR>
                </THead>
                <TBody>
                  {repartidos.map(filaDeGasto)}
                  {(verTodas ? sinRepartir : sinRepartir.slice(0, 10)).map(filaDeGasto)}
                </TBody>
              </Table>
            </TableContainer>
            {!verTodas && sinRepartir.length > 10 && (
              <Button variant="ghost" onClick={() => setVerTodas(true)}>
                Ver las {sinRepartir.length - 10} restantes sin asignar
              </Button>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
