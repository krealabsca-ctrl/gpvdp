/**
 * CxC — Parámetros de cobro (/cxc/parametros). La pantalla que faltaba de la maqueta.
 *
 * Lo que gobierna desde acá:
 *   · los TRAMOS de mora con su probabilidad de recuperación,
 *   · el FACTOR por forma de pago,
 *   · los parámetros clave/valor del módulo,
 *   · las SEDES y qué cartera ve cada usuario (la frontera de datos).
 *
 * Los dos primeros son los multiplicadores del valor esperado: cambiarlos REORDENA la cola
 * de cobro. La pantalla lo dice, y cada tramo muestra cuántos contratos caen hoy en él —
 * cambiar una probabilidad sin ese número es a ciegas.
 *
 * Un parámetro que el motor todavía no lee se muestra BLOQUEADO con el motivo. Es a
 * propósito: dejar que alguien lo cambie y creer que el sistema cambió sería peor que no
 * tener la pantalla.
 */

import { useEffect, useState, type FormEvent } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Select,
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
import { formatMoneda } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useActualizarFormaPago,
  useActualizarSede,
  useActualizarTramo,
  useAsignarSedes,
  useConfigCxc,
  useCrearSede,
  useGuardarParametrosCxc,
} from "@/features/cxc/hooks";
import type { ParametroCxc, SedeConfig, TramoConfig, UsuarioSedes } from "@/api/cxc";

type Tab = "tramos" | "parametros" | "sedes";

const TABS: { id: Tab; label: string }[] = [
  { id: "tramos", label: "Tramos y factores" },
  { id: "parametros", label: "Parámetros" },
  { id: "sedes", label: "Sedes y accesos" },
];

export function ParametrosCxcPage() {
  const [tab, setTab] = useState<Tab>("tramos");
  const q = useConfigCxc();
  const tiene = useTienePermiso();
  const puedeEditar = tiene("cxc.parametros");

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Parámetros de cobro"
        description="La probabilidad del tramo y el factor de la forma de pago son los dos multiplicadores del valor esperado: cambiarlos reordena la cola."
      />

      {!puedeEditar && (
        <p className="text-xs text-content-muted">
          Estás viendo la configuración en modo lectura. Cambiarla requiere el permiso{" "}
          <span className="font-mono">cxc.parametros</span>.
        </p>
      )}

      <div role="tablist" aria-label="Secciones de parámetros" className="flex gap-1 border-b border-border">
        {TABS.map((t) => (
          <button
            key={t.id}
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => setTab(t.id)}
            className={cn(
              "-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors",
              tab === t.id ? "border-accent text-accent" : "border-transparent text-content-muted hover:text-content",
            )}
          >
            {t.label}
          </button>
        ))}
      </div>

      {q.isPending ? (
        <LoadingState label="Cargando la configuración" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : !q.data ? (
        <EmptyState message="No se pudo leer la configuración." />
      ) : (
        <>
          {tab === "tramos" && <SeccionTramos config={q.data} puedeEditar={puedeEditar} />}
          {tab === "parametros" && <SeccionParametros parametros={q.data.parametros} puedeEditar={puedeEditar} />}
          {tab === "sedes" && <SeccionSedes config={q.data} puedeEditar={puedeEditar} />}
        </>
      )}
    </div>
  );
}

/** Tramos de mora + factor por forma de pago: los dos insumos del valor esperado. */
function SeccionTramos({
  config,
  puedeEditar,
}: {
  config: { tramos: TramoConfig[]; formas_pago: import("@/api/cxc").FormaPagoConfig[] };
  puedeEditar: boolean;
}) {
  const toast = useToast();
  const actualizar = useActualizarTramo();
  const actualizarForma = useActualizarFormaPago();
  const [editando, setEditando] = useState<string | null>(null);
  const [prob, setProb] = useState("");
  const [estrategia, setEstrategia] = useState("");
  const [canal, setCanal] = useState("");
  const [editandoForma, setEditandoForma] = useState<string | null>(null);
  const [factor, setFactor] = useState("");

  const totalVencido = config.tramos.reduce((a, t) => a + Number(t.vencido), 0);

  function guardar(t: TramoConfig) {
    actualizar.mutate(
      {
        codigo: t.codigo,
        cambio: {
          ...(prob !== t.prob_recuperacion ? { prob_recuperacion: prob } : {}),
          ...(estrategia !== t.estrategia ? { estrategia } : {}),
          ...(canal !== t.canal_sugerido ? { canal_sugerido: canal } : {}),
        },
      },
      {
        onSuccess: () => {
          toast.success(`Tramo «${t.etiqueta}» actualizado. La cola se reordenó.`);
          setEditando(null);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardContent className="flex flex-col gap-3 py-4">
          <div>
            <h3 className="text-sm font-semibold text-content">Tramos de mora</h3>
            <p className="text-xs text-content-muted">
              La probabilidad multiplica lo vencido para calcular el valor esperado. Las columnas de la
              derecha dicen cuántos contratos y cuánta plata caen HOY en cada tramo: son la evidencia
              para decidir si el número tiene sentido o sigue siendo el supuesto de arranque.
            </p>
          </div>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Tramo</TH>
                  <TH>Días</TH>
                  <TH className="text-right">Probabilidad</TH>
                  <TH>Estrategia</TH>
                  <TH>Canal sugerido</TH>
                  <TH className="text-right">Contratos hoy</TH>
                  <TH className="text-right">Vencido</TH>
                  {puedeEditar && <TH />}
                </TR>
              </THead>
              <TBody>
                {config.tramos.map((t) => (
                  <TR key={t.codigo}>
                    <TD className="font-medium">{t.etiqueta}</TD>
                    <TD className="whitespace-nowrap text-xs text-content-muted">
                      {t.dias_min <= -9999 ? "antes de vencer" : t.dias_min} …{" "}
                      {t.dias_max >= 999999 ? "∞" : t.dias_max}
                    </TD>
                    <TD className="text-right tabular-nums">
                      {editando === t.codigo ? (
                        <Input
                          value={prob}
                          onChange={(e) => setProb(e.target.value)}
                          className="max-w-[5rem] text-right"
                          aria-label="Probabilidad de recuperación"
                        />
                      ) : (
                        <span className="font-semibold">{t.prob_recuperacion}</span>
                      )}
                    </TD>
                    <TD className="max-w-[16rem] text-xs">
                      {editando === t.codigo ? (
                        <Input value={estrategia} onChange={(e) => setEstrategia(e.target.value)} aria-label="Estrategia" />
                      ) : (
                        t.estrategia || "—"
                      )}
                    </TD>
                    <TD className="max-w-[11rem] text-xs">
                      {editando === t.codigo ? (
                        <Input value={canal} onChange={(e) => setCanal(e.target.value)} aria-label="Canal sugerido" />
                      ) : (
                        t.canal_sugerido || "—"
                      )}
                    </TD>
                    <TD className="text-right tabular-nums">{t.contratos.toLocaleString("es-CR")}</TD>
                    <TD className="text-right tabular-nums">
                      {Number(t.vencido) > 0 ? formatMoneda(t.vencido) : "—"}
                      {Number(t.vencido) > 0 && totalVencido > 0 && (
                        <span className="block text-[11px] text-content-muted">
                          {Math.round((Number(t.vencido) / totalVencido) * 100)} % del vencido
                        </span>
                      )}
                    </TD>
                    {puedeEditar && (
                      <TD className="text-right">
                        {editando === t.codigo ? (
                          <div className="flex justify-end gap-2">
                            <Button size="sm" onClick={() => guardar(t)} loading={actualizar.isPending}>
                              Guardar
                            </Button>
                            <Button size="sm" variant="ghost" onClick={() => setEditando(null)}>
                              Cancelar
                            </Button>
                          </div>
                        ) : (
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => {
                              setEditando(t.codigo);
                              setProb(t.prob_recuperacion);
                              setEstrategia(t.estrategia);
                              setCanal(t.canal_sugerido);
                            }}
                          >
                            Editar
                          </Button>
                        )}
                      </TD>
                    )}
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
          <p className="text-xs text-content-muted">
            Los rangos de días no pueden traslaparse: lo impide la base de datos, porque si dos tramos
            se traslaparan un mismo saldo tendría dos probabilidades y la cola sería irreproducible.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="flex flex-col gap-3 py-4">
          <div>
            <h3 className="text-sm font-semibold text-content">Factor por forma de pago</h3>
            <p className="text-xs text-content-muted">
              Cuánto más (o menos) se recupera según por dónde llega la plata. El débito automático sale
              solo; un cobrador en la calle rinde menos. Rango permitido: 0,10 a 2,00.
            </p>
          </div>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Forma de pago</TH>
                  <TH className="text-right">Factor</TH>
                  <TH>Tipo</TH>
                  <TH className="text-right">Contratos</TH>
                  {puedeEditar && <TH />}
                </TR>
              </THead>
              <TBody>
                {config.formas_pago.map((f) => (
                  <TR key={f.id} className={cn(!f.activa && "opacity-60")}>
                    <TD className="font-medium">
                      {f.nombre}
                      {!f.activa && (
                        <Badge tone="neutral" className="ml-2">
                          inactiva
                        </Badge>
                      )}
                    </TD>
                    <TD className="text-right tabular-nums">
                      {editandoForma === f.id ? (
                        <Input
                          value={factor}
                          onChange={(e) => setFactor(e.target.value)}
                          className="max-w-[5rem] text-right"
                          aria-label="Factor de recuperación"
                        />
                      ) : (
                        <span className="font-semibold">{f.factor_recuperacion}</span>
                      )}
                    </TD>
                    <TD className="text-xs">
                      {f.es_asociacion && <Badge tone="accent">asociación</Badge>}
                      {f.es_domiciliado && <Badge tone="accent">domiciliado</Badge>}
                      {!f.es_asociacion && !f.es_domiciliado && <span className="text-content-muted">—</span>}
                    </TD>
                    <TD className="text-right tabular-nums">{f.contratos.toLocaleString("es-CR")}</TD>
                    {puedeEditar && (
                      <TD className="text-right">
                        {editandoForma === f.id ? (
                          <div className="flex justify-end gap-2">
                            <Button
                              size="sm"
                              loading={actualizarForma.isPending}
                              onClick={() =>
                                actualizarForma.mutate(
                                  { id: f.id, factor },
                                  {
                                    onSuccess: () => {
                                      toast.success(`Factor de «${f.nombre}» actualizado.`);
                                      setEditandoForma(null);
                                    },
                                    onError: (err) => toast.error(mensajeError(err)),
                                  },
                                )
                              }
                            >
                              Guardar
                            </Button>
                            <Button size="sm" variant="ghost" onClick={() => setEditandoForma(null)}>
                              Cancelar
                            </Button>
                          </div>
                        ) : (
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => {
                              setEditandoForma(f.id);
                              setFactor(f.factor_recuperacion);
                            }}
                          >
                            Editar
                          </Button>
                        )}
                      </TD>
                    )}
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>
    </div>
  );
}

/** Parámetros clave/valor. Los que el motor no lee salen bloqueados con su motivo. */
function SeccionParametros({ parametros, puedeEditar }: { parametros: ParametroCxc[]; puedeEditar: boolean }) {
  const toast = useToast();
  const guardar = useGuardarParametrosCxc();
  const [valores, setValores] = useState<Record<string, string>>({});

  // Al recargar la configuración se descartan los borradores: lo que se ve es lo guardado.
  useEffect(() => {
    setValores({});
  }, [parametros]);

  const editables = parametros.filter((p) => p.editable);
  const bloqueados = parametros.filter((p) => !p.editable);
  const sucios = Object.entries(valores).filter(([k, v]) => {
    const p = parametros.find((x) => x.clave === k);
    return p !== undefined && v !== p.valor;
  });

  function onGuardar(e: FormEvent) {
    e.preventDefault();
    if (sucios.length === 0) return;
    guardar.mutate(Object.fromEntries(sucios), {
      onSuccess: (res) => {
        toast.success(
          res.cambiados === 0 ? "No había nada que cambiar." : `${res.cambiados} parámetro(s) actualizado(s).`,
        );
        setValores({});
      },
      // Si una clave del lote está mal, el servidor rechaza TODO: nada queda a medias.
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardContent className="py-4">
          <form onSubmit={onGuardar} className="flex flex-col gap-4">
            <div>
              <h3 className="text-sm font-semibold text-content">Parámetros del módulo</h3>
              <p className="text-xs text-content-muted">
                Cada uno dice dónde lo usa el motor. Si una clave del lote está mal, no se guarda
                ninguna: media configuración es peor que ninguna.
              </p>
            </div>
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Parámetro</TH>
                    <TH>Valor</TH>
                    <TH>Lo usa</TH>
                    <TH>Último cambio</TH>
                  </TR>
                </THead>
                <TBody>
                  {editables.map((p) => (
                    <TR key={p.clave}>
                      <TD>
                        <span className="font-mono text-xs font-medium">{p.clave}</span>
                        <span className="block text-[11px] text-content-muted">{p.descripcion}</span>
                      </TD>
                      <TD>
                        {p.tipo === "opciones" && p.opciones ? (
                          <Select
                            value={valores[p.clave] ?? p.valor}
                            onChange={(e) => setValores((v) => ({ ...v, [p.clave]: e.target.value }))}
                            options={p.opciones.map((o) => ({ value: o, label: o }))}
                            disabled={!puedeEditar}
                            className="max-w-[10rem]"
                            aria-label={p.clave}
                          />
                        ) : (
                          <Input
                            value={valores[p.clave] ?? p.valor}
                            onChange={(e) => setValores((v) => ({ ...v, [p.clave]: e.target.value }))}
                            disabled={!puedeEditar}
                            type={p.tipo === "fecha_opcional" ? "date" : "text"}
                            className="max-w-[11rem]"
                            aria-label={p.clave}
                          />
                        )}
                      </TD>
                      <TD className="max-w-[20rem] text-xs text-content-muted">{p.leido_por}</TD>
                      <TD className="whitespace-nowrap text-[11px] text-content-muted">
                        {p.actualizado_en}
                        {p.actualizado_por && <span className="block">{p.actualizado_por}</span>}
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
            {puedeEditar && (
              <div className="flex items-center gap-3">
                <Button type="submit" loading={guardar.isPending} disabled={sucios.length === 0}>
                  Guardar {sucios.length > 0 && `(${sucios.length})`}
                </Button>
                {sucios.length > 0 && (
                  <Button variant="ghost" onClick={() => setValores({})}>
                    Descartar
                  </Button>
                )}
              </div>
            )}
          </form>
        </CardContent>
      </Card>

      {bloqueados.length > 0 && (
        <Card>
          <CardContent className="flex flex-col gap-3 py-4">
            <div>
              <h3 className="text-sm font-semibold text-content">Todavía no se pueden cambiar</h3>
              <p className="text-xs text-content-muted">
                Estos parámetros existen en la base pero <b>el motor no los lee</b>. Se muestran
                bloqueados a propósito: dejarte cambiarlos y que el sistema siguiera igual sería peor
                que no mostrarlos.
              </p>
            </div>
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Parámetro</TH>
                    <TH>Valor actual</TH>
                    <TH>Qué falta</TH>
                  </TR>
                </THead>
                <TBody>
                  {bloqueados.map((p) => (
                    <TR key={p.clave}>
                      <TD>
                        <span className="font-mono text-xs font-medium">{p.clave}</span>
                        <Badge tone="pendiente" className="ml-2">
                          bloqueado
                        </Badge>
                      </TD>
                      <TD className="font-mono text-xs">{p.valor || "—"}</TD>
                      <TD className="max-w-[34rem] text-xs text-content-muted">{p.nota}</TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

/** Sedes operativas y la frontera de datos: qué cartera ve cada usuario. */
function SeccionSedes({
  config,
  puedeEditar,
}: {
  config: { sedes: SedeConfig[]; usuarios: UsuarioSedes[] };
  puedeEditar: boolean;
}) {
  const toast = useToast();
  const crear = useCrearSede();
  const actualizar = useActualizarSede();
  const asignar = useAsignarSedes();
  const [nombre, setNombre] = useState("");
  const [plaza, setPlaza] = useState("");
  const [editando, setEditando] = useState<string | null>(null);
  const [nuevoNombre, setNuevoNombre] = useState("");

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardContent className="flex flex-col gap-4 py-4">
          <div>
            <h3 className="text-sm font-semibold text-content">Sedes operativas</h3>
            <p className="text-xs text-content-muted">
              La sede es la dimensión que decide quién ve qué cartera. El importador crea las que vienen
              en el archivo; acá se corrigen y se agregan las que falten.
            </p>
          </div>
          {puedeEditar && (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (!nombre.trim()) return;
                crear.mutate(
                  { nombre: nombre.trim(), plaza: plaza.trim() || undefined },
                  {
                    onSuccess: () => {
                      toast.success("Sede creada.");
                      setNombre("");
                      setPlaza("");
                    },
                    onError: (err) => toast.error(mensajeError(err)),
                  },
                )
              }}
              className="flex flex-wrap items-end gap-3"
            >
              <Input
                label="Nueva sede"
                value={nombre}
                onChange={(e) => setNombre(e.target.value)}
                placeholder="Ej. Cartago"
                className="min-w-56"
              />
              <Input
                label="Plaza (opcional)"
                value={plaza}
                onChange={(e) => setPlaza(e.target.value)}
                className="min-w-40"
              />
              <Button type="submit" loading={crear.isPending} disabled={!nombre.trim()}>
                Crear sede
              </Button>
            </form>
          )}
          {config.sedes.length === 0 ? (
            <EmptyState message="Todavía no hay sedes. Se crean al importar la cartera, o desde acá." />
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Sede</TH>
                    <TH>Plaza</TH>
                    <TH className="text-right">Contratos</TH>
                    <TH className="text-right">Usuarios</TH>
                    {puedeEditar && <TH />}
                  </TR>
                </THead>
                <TBody>
                  {config.sedes.map((s) => (
                    <TR key={s.id} className={cn(!s.activa && "opacity-60")}>
                      <TD className="max-w-[22rem] font-medium">
                        {editando === s.id ? (
                          <Input
                            value={nuevoNombre}
                            onChange={(e) => setNuevoNombre(e.target.value)}
                            aria-label="Nuevo nombre de la sede"
                          />
                        ) : (
                          <>
                            <span className="block truncate">{s.nombre}</span>
                            {!s.activa && (
                              <Badge tone="neutral" className="mt-1">
                                inactiva
                              </Badge>
                            )}
                          </>
                        )}
                      </TD>
                      <TD className="text-xs text-content-muted">{s.plaza || "—"}</TD>
                      <TD className="text-right tabular-nums">{s.contratos.toLocaleString("es-CR")}</TD>
                      <TD className="text-right tabular-nums">{s.usuarios}</TD>
                      {puedeEditar && (
                        <TD className="text-right">
                          {editando === s.id ? (
                            <div className="flex justify-end gap-2">
                              <Button
                                size="sm"
                                loading={actualizar.isPending}
                                disabled={!nuevoNombre.trim()}
                                onClick={() =>
                                  actualizar.mutate(
                                    { id: s.id, cambio: { nombre: nuevoNombre.trim() } },
                                    {
                                      onSuccess: () => {
                                        toast.success("Sede actualizada.");
                                        setEditando(null);
                                      },
                                      onError: (err) => toast.error(mensajeError(err)),
                                    },
                                  )
                                }
                              >
                                Guardar
                              </Button>
                              <Button size="sm" variant="ghost" onClick={() => setEditando(null)}>
                                Cancelar
                              </Button>
                            </div>
                          ) : (
                            <div className="flex justify-end gap-2">
                              <Button
                                size="sm"
                                variant="secondary"
                                onClick={() => {
                                  setEditando(s.id);
                                  setNuevoNombre(s.nombre);
                                }}
                              >
                                Renombrar
                              </Button>
                              <Button
                                size="sm"
                                variant="ghost"
                                loading={actualizar.isPending}
                                onClick={() =>
                                  actualizar.mutate(
                                    { id: s.id, cambio: { activa: !s.activa } },
                                    {
                                      onSuccess: () =>
                                        toast.success(s.activa ? "Sede desactivada." : "Sede reactivada."),
                                      onError: (err) => toast.error(mensajeError(err)),
                                    },
                                  )
                                }
                              >
                                {s.activa ? "Desactivar" : "Reactivar"}
                              </Button>
                            </div>
                          )}
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

      <Card>
        <CardContent className="flex flex-col gap-3 py-4">
          <div>
            <h3 className="text-sm font-semibold text-content">Quién ve qué cartera</h3>
            <p className="text-xs text-content-muted">
              Sin ninguna sede marcada, un usuario que no tenga el permiso «ver todas las sedes» abre la
              cola <b>vacía</b>. Marcar y desmarcar es la lista completa: si le quitás la casilla, pierde
              el acceso.
            </p>
          </div>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Usuario</TH>
                  <TH>Rol</TH>
                  <TH>Sedes que ve</TH>
                </TR>
              </THead>
              <TBody>
                {config.usuarios.map((u) => (
                  <TR key={u.usuario_id}>
                    <TD>
                      <span className="font-medium">{u.nombre}</span>
                      <span className="block text-[11px] text-content-muted">{u.email}</span>
                    </TD>
                    <TD className="text-xs">{u.rol}</TD>
                    <TD>
                      {!u.puede_ver_cxc ? (
                        <span className="text-xs text-content-muted">
                          su rol no tiene <span className="font-mono">cxc.ver</span>: no entra al módulo
                        </span>
                      ) : u.ve_todas_sedes ? (
                        <Badge tone="positivo">todas — no hace falta asignarle nada</Badge>
                      ) : (
                        <FilaSedesDeUsuario
                          usuario={u}
                          sedes={config.sedes.filter((s) => s.activa)}
                          puedeEditar={puedeEditar}
                          onGuardar={(sedeIds) =>
                            asignar.mutate(
                              { usuarioId: u.usuario_id, sedeIds },
                              {
                                onSuccess: (res) =>
                                  toast.success(
                                    res.sedes === 0
                                      ? `${u.nombre} ya no ve ninguna sede.`
                                      : `${u.nombre} ve ${res.sedes} sede(s).`,
                                  ),
                                onError: (err) => toast.error(mensajeError(err)),
                              },
                            )
                          }
                          pendiente={asignar.isPending}
                        />
                      )}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>
    </div>
  );
}

/** Las casillas de sedes de un usuario, con «Guardar» explícito. */
function FilaSedesDeUsuario({
  usuario,
  sedes,
  puedeEditar,
  onGuardar,
  pendiente,
}: {
  usuario: UsuarioSedes;
  sedes: SedeConfig[];
  puedeEditar: boolean;
  onGuardar: (sedeIds: string[]) => void;
  pendiente: boolean;
}) {
  const [seleccion, setSeleccion] = useState<string[]>(usuario.sede_ids);
  // Si el servidor devuelve otra asignación (la guardó otro supervisor), gana la del servidor.
  useEffect(() => {
    setSeleccion(usuario.sede_ids);
  }, [usuario.sede_ids]);

  const cambio =
    seleccion.length !== usuario.sede_ids.length || seleccion.some((s) => !usuario.sede_ids.includes(s));

  if (sedes.length === 0) {
    return <span className="text-xs text-content-muted">no hay sedes activas para asignar</span>;
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap gap-x-4 gap-y-1">
        {sedes.map((s) => (
          <label key={s.id} className="flex items-center gap-2 text-xs">
            <input
              type="checkbox"
              checked={seleccion.includes(s.id)}
              disabled={!puedeEditar}
              onChange={(e) =>
                setSeleccion((prev) => (e.target.checked ? [...prev, s.id] : prev.filter((x) => x !== s.id)))
              }
              className="h-3.5 w-3.5 rounded border-border accent-accent"
            />
            <span className="max-w-[16rem] truncate">{s.nombre}</span>
          </label>
        ))}
      </div>
      {puedeEditar && cambio && (
        <div className="flex gap-2">
          <Button size="sm" loading={pendiente} onClick={() => onGuardar(seleccion)}>
            Guardar acceso
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setSeleccion(usuario.sede_ids)}>
            Cancelar
          </Button>
        </div>
      )}
      {seleccion.length === 0 && !cambio && (
        <span className="text-[11px] text-negativo">sin sedes: abre la cola vacía</span>
      )}
    </div>
  );
}
