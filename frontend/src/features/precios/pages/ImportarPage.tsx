/**
 * Pantalla — Importar el catálogo (/precios/importar).
 *
 * ── LA DECISIÓN QUE ESTA PANTALLA EXISTE PARA QUE ALGUIEN TOME ──────────────────────────────
 *
 * El archivo repite cada ítem por sede («Cremación de un Adulto-Limon», «-Heredia»…). Eso puede
 * significar que el costo es el MISMO en todas y la repetición es ruido del formato, o que el
 * costo CAMBIA por sede. El archivo no lo dice y el importador no lo adivina: mide las dos
 * lecturas, muestra en cuántos ítems las sedes coinciden y en cuántos difieren, y deja decidir.
 *
 * ── TRES PASOS, Y LOS DOS PRIMEROS NO ESCRIBEN NADA ─────────────────────────────────────────
 *
 *   1. Leer el archivo y medir.
 *   2. Ver la lista exacta que se escribiría.
 *   3. Confirmar con el número de insumos del paso 2.
 *
 * El número de confirmación no es burocracia: entre que la pantalla pinta el plan y alguien
 * aprieta el botón, el archivo pudo cambiar, y escribir cientos de filas no tiene deshacer.
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
import {
  analizarImportacion,
  importarCatalogo,
  planearImportacion,
  type AnalisisImportacion,
  type PlanImportacion,
  type ResultadoImportacion,
} from "@/api/precios";
import { useTienePermiso } from "@/features/auth/permisos";

const HOY = new Date().toISOString().slice(0, 10);

function money(v: string | null): string {
  if (v === null) return "—";
  const n = Number(v);
  if (Number.isNaN(n)) return v;
  return `₡${n.toLocaleString("es-CR", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

export function ImportarPreciosPage() {
  const tienePermiso = useTienePermiso();
  const puedeEditar = tienePermiso("precios.editar");
  const toast = useToast();

  const [archivo, setArchivo] = useState<File | null>(null);
  const [hoja, setHoja] = useState("");
  const [colNombre, setColNombre] = useState("");
  const [colCosto, setColCosto] = useState("");
  const [unificar, setUnificar] = useState(true);
  const [vigenteDesde, setVigenteDesde] = useState(HOY);
  const [confirmacion, setConfirmacion] = useState("");

  const [analisis, setAnalisis] = useState<AnalisisImportacion | null>(null);
  // Los sufijos que el usuario confirma como sede. Arrancan todos marcados: la detección propone
  // y él descarta, que es menos trabajo que marcarlos de cero.
  const [sedesElegidas, setSedesElegidas] = useState<string[]>([]);
  const [plan, setPlan] = useState<PlanImportacion | null>(null);
  const [resultado, setResultado] = useState<ResultadoImportacion | null>(null);
  const [cargando, setCargando] = useState<"" | "analizando" | "planeando" | "importando">("");

  function reiniciar() {
    setAnalisis(null);
    setSedesElegidas([]);
    setPlan(null);
    setResultado(null);
    setConfirmacion("");
  }

  async function onAnalizar(h?: string) {
    if (!archivo) return;
    setCargando("analizando");
    try {
      const a = await analizarImportacion(archivo, {
        hoja: h ?? hoja,
        columnaNombre: colNombre,
        columnaCosto: colCosto,
      });
      setAnalisis(a);
      setHoja(a.hoja);
      setSedesElegidas(a.sedes_detectadas);
      setPlan(null);
      setResultado(null);
      setConfirmacion("");
    } catch (e) {
      toast.error(mensajeError(e));
      setAnalisis(null);
    } finally {
      setCargando("");
    }
  }

  async function onPlanear() {
    if (!archivo) return;
    setCargando("planeando");
    try {
      const p = await planearImportacion(archivo, {
        hoja,
        columnaNombre: colNombre,
        columnaCosto: colCosto,
        unificarSedes: unificar,
        sedes: sedesElegidas,
      });
      setPlan(p);
      setConfirmacion("");
      setResultado(null);
    } catch (e) {
      toast.error(mensajeError(e));
    } finally {
      setCargando("");
    }
  }

  async function onImportar() {
    if (!archivo || !plan) return;
    setCargando("importando");
    try {
      const r = await importarCatalogo(archivo, {
        hoja,
        columnaNombre: colNombre,
        columnaCosto: colCosto,
        unificarSedes: unificar,
        sedes: sedesElegidas,
        vigenteDesde,
        confirmacion: Number(confirmacion),
      });
      setResultado(r);
      toast.success(
        `Catálogo importado: ${r.insumos_creados} insumos nuevos, ${r.costos_capturados} costos.`,
      );
    } catch (e) {
      toast.error(mensajeError(e));
    } finally {
      setCargando("");
    }
  }

  if (!puedeEditar) {
    return (
      <div className="space-y-4">
        <PageHeader title="Importar el catálogo" description="Cargar insumos y costos desde el archivo de costeo." />
        <EmptyState message="No tenés permiso para importar el catálogo. Pedí «Editar insumos, costos y recetas»." />
      </div>
    );
  }

  const confirmacionOk = plan !== null && Number(confirmacion) === plan.confirmar_con;

  return (
    <div className="space-y-4">
      <PageHeader
        title="Importar el catálogo"
        description="Leer el archivo de costeo y cargar los insumos con su costo. Nada se escribe hasta el último paso."
      />

      {/* ── Paso 1 ── */}
      <Card>
        <CardHeader>
          <CardTitle>1 · El archivo</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap items-end gap-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-content-muted">
                Archivo de costeo (.xlsx)
              </label>
              <input
                type="file"
                accept=".xlsx"
                onChange={(e) => {
                  setArchivo(e.target.files?.[0] ?? null);
                  setHoja("");
                  reiniciar();
                }}
                className="text-sm file:mr-3 file:rounded-lg file:border-0 file:bg-accent/10 file:px-3 file:py-2 file:text-sm file:font-medium file:text-accent"
              />
            </div>
            <Button onClick={() => onAnalizar()} disabled={!archivo || cargando !== ""}>
              {cargando === "analizando" ? "Leyendo…" : "Leer el archivo"}
            </Button>
          </div>

          {analisis && (
            <div className="flex flex-wrap items-end gap-3 border-t border-border pt-3">
              <Select
                label="Hoja"
                value={hoja}
                onChange={(e) => {
                  setHoja(e.target.value);
                  void onAnalizar(e.target.value);
                }}
                options={analisis.hojas_del_libro.map((h) => ({ value: h, label: h }))}
              />
              <Input
                label="Columna del nombre"
                placeholder="auto"
                value={colNombre}
                onChange={(e) => setColNombre(e.target.value.toUpperCase())}
              />
              <Input
                label="Columna del costo"
                placeholder="auto"
                value={colCosto}
                onChange={(e) => setColCosto(e.target.value.toUpperCase())}
              />
              <Button variant="ghost" onClick={() => onAnalizar()} disabled={cargando !== ""}>
                Volver a leer
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {cargando === "analizando" && <LoadingState />}

      {/* ── Paso 2: la decisión ── */}
      {analisis && (
        <>
          <Card>
            <CardHeader>
              <CardTitle>
                2 · Qué trae «{analisis.hoja}»{" "}
                <span className="text-sm font-normal text-content-muted">
                  columnas {analisis.columna_nombre} y {analisis.columna_costo || "—"}
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-3 sm:grid-cols-3">
                <div>
                  <div className="text-xs uppercase tracking-wide text-content-muted">Líneas</div>
                  <div className="text-2xl font-bold tabular-nums">{analisis.lineas_totales}</div>
                </div>
                <div>
                  <div className="text-xs uppercase tracking-wide text-content-muted">Con costo</div>
                  <div className="text-2xl font-bold tabular-nums text-positivo">
                    {analisis.lineas_con_costo}
                  </div>
                </div>
                <div>
                  <div className="text-xs uppercase tracking-wide text-content-muted">Sin costo</div>
                  <div className="text-2xl font-bold tabular-nums text-negativo">
                    {analisis.lineas_sin_costo}
                  </div>
                </div>
              </div>

              {analisis.sedes_detectadas.length > 0 ? (
                <div className="space-y-3 rounded-lg border border-border bg-surface-muted p-3">
                  <div className="text-sm">
                    El archivo repite los ítems en{" "}
                    <b>{sedesElegidas.length} de {analisis.sedes_detectadas.length} sufijos</b>{" "}
                    que parecen sedes.
                  </div>
                  {/* Los sufijos se confirman uno por uno porque la detección se equivoca: contra
                      el archivo real marcó «URNA DE ALUMINIO» y «Sabana 12 horas» junto a las
                      plazas verdaderas. Un modelo de urna partido como si fuera sede rompe el
                      nombre del insumo. */}
                  <div>
                    <div className="mb-1 text-xs text-content-muted">
                      Desmarcá lo que NO sea una plaza (modelos, duraciones, variantes):
                    </div>
                    <div className="flex flex-wrap gap-1.5">
                      {analisis.sedes_detectadas.map((s) => {
                        const on = sedesElegidas.includes(s);
                        return (
                          <button
                            key={s}
                            type="button"
                            onClick={() => {
                              setSedesElegidas((prev) =>
                                prev.includes(s) ? prev.filter((x) => x !== s) : [...prev, s],
                              );
                              setPlan(null);
                            }}
                            className={`rounded-full border px-2.5 py-1 text-xs transition-colors ${
                              on
                                ? "border-accent bg-accent/10 text-accent"
                                : "border-border text-content-muted line-through"
                            }`}
                          >
                            {s}
                          </button>
                        );
                      })}
                    </div>
                  </div>

                  <div className="text-sm">
                    <b>Hay que decidir si el costo es el mismo en todas las sedes.</b> Los números
                    de tu archivo:
                  </div>

                  <div className="grid gap-2 sm:grid-cols-2">
                    <label
                      className={`flex cursor-pointer gap-3 rounded-lg border p-3 ${unificar ? "border-accent bg-accent/5" : "border-border"}`}
                    >
                      <input
                        type="radio"
                        checked={unificar}
                        onChange={() => {
                          setUnificar(true);
                          setPlan(null);
                        }}
                        className="mt-1"
                      />
                      <span className="text-sm">
                        <b>Un insumo por producto</b> — {analisis.insumos_si_se_unifica} insumos
                        <div className="mt-1 text-xs text-content-muted">
                          El costo es el mismo en todas las sedes. Se toma el <b>mínimo</b> de cada
                          grupo.
                        </div>
                      </span>
                    </label>
                    <label
                      className={`flex cursor-pointer gap-3 rounded-lg border p-3 ${!unificar ? "border-accent bg-accent/5" : "border-border"}`}
                    >
                      <input
                        type="radio"
                        checked={!unificar}
                        onChange={() => {
                          setUnificar(false);
                          setPlan(null);
                        }}
                        className="mt-1"
                      />
                      <span className="text-sm">
                        <b>Uno por sede</b> — {analisis.insumos_si_no_se_unifica} insumos
                        <div className="mt-1 text-xs text-content-muted">
                          El costo cambia según la plaza. Cada sede lleva su propio costo.
                        </div>
                      </span>
                    </label>
                  </div>

                  <div className="text-sm">
                    De los {analisis.insumos_si_se_unifica} productos,{" "}
                    <b className="text-positivo">{analisis.grupos_con_costo_igual}</b> cuestan igual
                    en todas sus sedes y{" "}
                    <b className="text-negativo">{analisis.grupos_con_costo_distinto}</b> tienen
                    costos distintos.
                    {analisis.grupos_con_costo_distinto > 0 && (
                      <span className="text-content-muted">
                        {" "}
                        Unificar esos haría perder la diferencia entre plazas.
                      </span>
                    )}
                  </div>
                </div>
              ) : (
                <div className="rounded-lg border border-border bg-surface-muted p-3 text-sm">
                  No se detectaron sedes repetidas: cada línea es un insumo distinto.
                </div>
              )}

              {analisis.conflictos.length > 0 && (
                <div>
                  <div className="mb-2 text-sm font-medium">
                    Productos cuyo costo cambia entre sedes
                  </div>
                  <TableContainer>
                    <Table>
                      <THead>
                        <TR>
                          <TH>Producto</TH>
                          <TH className="text-right">Sedes</TH>
                          <TH className="text-right">Costos distintos</TH>
                          <TH className="text-right">Mínimo</TH>
                          <TH className="text-right">Máximo</TH>
                        </TR>
                      </THead>
                      <TBody>
                        {analisis.conflictos.map((g) => (
                          <TR key={g.nombre_base}>
                            <TD>{g.nombre_base}</TD>
                            <TD className="text-right tabular-nums">{g.sedes.length}</TD>
                            <TD className="text-right tabular-nums font-medium text-negativo">
                              {g.costos_distintos}
                            </TD>
                            <TD className="text-right tabular-nums">{money(g.costo_minimo)}</TD>
                            <TD className="text-right tabular-nums">{money(g.costo_maximo)}</TD>
                          </TR>
                        ))}
                      </TBody>
                    </Table>
                  </TableContainer>
                </div>
              )}

              <Button onClick={onPlanear} disabled={cargando !== ""}>
                {cargando === "planeando" ? "Calculando…" : "Ver qué se va a cargar"}
              </Button>
            </CardContent>
          </Card>
        </>
      )}

      {/* ── Paso 3: confirmar ── */}
      {plan && (
        <Card>
          <CardHeader>
            <CardTitle>3 · Confirmar</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="rounded-lg border border-accent/40 bg-accent/5 p-3 text-sm">
              Se van a cargar <b>{plan.total} insumos</b>: {plan.con_costo} con costo y{" "}
              {plan.sin_costo} sin costo.
              <div className="mt-1 text-xs text-content-muted">
                Los que ya existan con el mismo código se <b>actualizan</b>, no se duplican. Los que
                queden sin costo entran al catálogo y hay que capturárselo después.
              </div>
            </div>

            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Código</TH>
                    <TH>Insumo</TH>
                    <TH>Sede</TH>
                    <TH className="text-right">Costo</TH>
                  </TR>
                </THead>
                <TBody>
                  {plan.muestra.map((i) => (
                    <TR key={i.codigo}>
                      <TD className="font-mono text-xs">{i.codigo}</TD>
                      <TD>{i.nombre}</TD>
                      <TD className="text-content-muted">{i.sede || "—"}</TD>
                      <TD className="text-right tabular-nums">
                        {i.costo === null ? <Badge tone="negativo">sin costo</Badge> : money(i.costo)}
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
            {plan.total > plan.muestra.length && (
              <div className="text-xs text-content-muted">
                Se muestran {plan.muestra.length} de {plan.total}.
              </div>
            )}

            <div className="flex flex-wrap items-end gap-3 border-t border-border pt-3">
              <Input
                label="Los costos rigen desde"
                type="date"
                value={vigenteDesde}
                onChange={(e) => setVigenteDesde(e.target.value)}
              />
              <Input
                label={`Escribí ${plan.confirmar_con} para confirmar`}
                value={confirmacion}
                onChange={(e) => setConfirmacion(e.target.value)}
                placeholder={String(plan.confirmar_con)}
              />
              <Button onClick={onImportar} disabled={!confirmacionOk || cargando !== ""}>
                {cargando === "importando" ? "Importando…" : "Importar"}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {resultado && (
        <Card>
          <CardHeader>
            <CardTitle>Listo</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid gap-3 sm:grid-cols-4">
              <div>
                <div className="text-xs uppercase tracking-wide text-content-muted">Creados</div>
                <div className="text-2xl font-bold tabular-nums text-positivo">
                  {resultado.insumos_creados}
                </div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-content-muted">
                  Actualizados
                </div>
                <div className="text-2xl font-bold tabular-nums">
                  {resultado.insumos_actualizados}
                </div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-content-muted">Costos</div>
                <div className="text-2xl font-bold tabular-nums">{resultado.costos_capturados}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-wide text-content-muted">Sin costo</div>
                <div className="text-2xl font-bold tabular-nums text-negativo">
                  {resultado.sin_costo}
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
