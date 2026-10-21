/**
 * Catálogo de nomenclaturas de pago: qué código identifica a cada rubro en el estado de cuenta
 * («A8» comisiones, «A9» planilla).
 *
 * El código se carga en el CONCEPTO y sus clasificaciones lo heredan. Esa es la razón de ser del
 * diseño: el catálogo de Valle de Paz tiene 8 conceptos y 124 clasificaciones solo bajo «Gastos»;
 * cargar 124 códigos a mano no lo hace nadie, y lo que no se carga queda sin código.
 *
 * Por eso la pantalla abre mostrando los conceptos —que es donde rinde el trabajo— y permite bajar
 * a las clasificaciones solo cuando una necesita distinguirse de su concepto.
 */
import { useMemo, useState } from "react";

import { Badge, Button, ErrorState, Input, LoadingState } from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { sinTildes } from "@/lib/format";
import { cn } from "@/lib/cn";
import { useAsignarNomenclatura, useNomenclaturas } from "@/features/cxp/hooks";
import type { RubroConCodigo } from "@/api/cxp";

export function NomenclaturasPanel({ onCerrar, puedeEditar }: { onCerrar: () => void; puedeEditar: boolean }) {
  const q = useNomenclaturas();
  const asignar = useAsignarNomenclatura();
  const [verClasificaciones, setVerClasificaciones] = useState(false);
  const [buscar, setBuscar] = useState("");
  const [editando, setEditando] = useState<string | null>(null);
  const [valor, setValor] = useState("");
  const [error, setError] = useState("");

  const rubros = useMemo(() => q.data ?? [], [q.data]);
  const conceptos = rubros.filter((r) => r.nivel === "CONCEPTO");
  const clasifs = rubros.filter((r) => r.nivel === "CLASIFICACION");
  // Por defecto, solo las que tienen código PROPIO o movimiento: las 170 completas no se revisan a
  // ojo, y abrir con todas desplegadas convierte la pantalla en una lista que nadie lee.
  const clasifsRelevantes = clasifs.filter((r) => (!r.heredado && r.codigo) || r.facturas > 0);

  // …pero ese recorte dejaba 155 de 170 INALCANZABLES: una clasificación sin facturas todavía no
  // aparecía en ningún lado, así que no se le podía poner código, y cuando llegara su primera
  // factura el pago saldría sin él. El pez mordiéndose la cola, y justo al revés de para qué sirve
  // esta pantalla, que es cargar los que faltan.
  //
  // Se resuelve como el resto del proyecto resolvió lo mismo con 168 clasificaciones: ESCRIBIENDO.
  // Sin texto manda el recorte; con texto se busca entre TODAS.
  const hayBusqueda = buscar.trim() !== "";
  const clasifsVisibles = useMemo(() => {
    if (!hayBusqueda) return clasifsRelevantes;
    const t = sinTildes(buscar.trim());
    return clasifs.filter((r) => sinTildes(r.nombre).includes(t) || sinTildes(r.concepto).includes(t));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [buscar, hayBusqueda, clasifs, clasifsRelevantes]);

  const conCodigo = rubros.filter((r) => r.codigo).length;

  function guardar(r: RubroConCodigo, codigo: string) {
    setError("");
    asignar.mutate(
      { nivel: r.nivel, id: r.id, codigo },
      {
        onSuccess: () => {
          setEditando(null);
          setValor("");
        },
        // El error se muestra EN EL PANEL y no en un toast que se va: el caso típico es «ese
        // código ya está usado», y hay que poder leerlo mientras se escribe otro.
        onError: (err) => setError(mensajeError(err)),
      },
    );
  }

  function Fila({ r }: { r: RubroConCodigo }) {
    const clave = `${r.nivel}:${r.id}`;
    const enEdicion = editando === clave;
    return (
      <tr className="border-t border-border/60">
        <td className="px-3 py-1.5">
          <span className={cn("text-sm", r.nivel === "CONCEPTO" && "font-semibold text-content")}>{r.nombre}</span>
          {r.concepto && <span className="block text-[11px] text-content-muted">en {r.concepto}</span>}
        </td>
        <td className="px-3 py-1.5 text-right text-xs tabular-nums text-content-muted">
          {r.facturas.toLocaleString("es-CR")}
        </td>
        <td className="px-3 py-1.5">
          {enEdicion ? (
            <div className="flex items-center gap-1">
              <Input
                value={valor}
                onChange={(e) => setValor(e.target.value.toUpperCase())}
                placeholder="A8"
                maxLength={4}
                className="w-20 font-mono"
                autoFocus
              />
              <Button size="sm" loading={asignar.isPending} onClick={() => guardar(r, valor)}>
                Guardar
              </Button>
              <Button size="sm" variant="ghost" onClick={() => { setEditando(null); setError(""); }}>
                Cancelar
              </Button>
            </div>
          ) : (
            <div className="flex items-center gap-2">
              {r.codigo ? (
                <Badge tone={r.heredado ? "pendiente" : "accent"} className="font-mono">
                  {r.codigo}
                </Badge>
              ) : (
                <span className="text-xs text-content-muted">sin código</span>
              )}
              {/* Heredado se dice explícitamente: borrar acá un código que viene del concepto no
                  haría nada, y sin la marca eso se lee como que el sistema ignoró el cambio. */}
              {r.heredado && <span className="text-[11px] text-content-muted">heredado del rubro</span>}
              {puedeEditar && (
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setEditando(clave);
                    // Un código heredado arranca vacío: lo que se va a escribir es el PROPIO.
                    setValor(r.heredado ? "" : r.codigo);
                    setError("");
                  }}
                >
                  {r.codigo && !r.heredado ? "Cambiar" : "Poner código"}
                </Button>
              )}
              {puedeEditar && r.codigo && !r.heredado && (
                <Button size="sm" variant="ghost" onClick={() => guardar(r, "")}>
                  Quitar
                </Button>
              )}
            </div>
          )}
        </td>
      </tr>
    );
  }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/40 p-4">
      <div className="mt-8 w-full max-w-3xl rounded-xl border border-border bg-surface-raised shadow-lifted">
        <div className="flex items-center justify-between gap-4 border-b border-border px-5 py-3">
          <div>
            <h2 className="text-base font-semibold text-content">Nomenclaturas de pago</h2>
            <p className="text-sm text-content-muted">
              El código con el que reconocés cada débito en el estado de cuenta.{" "}
              <b className="text-content">{conCodigo}</b> de {rubros.length} rubros tienen uno.
            </p>
          </div>
          <Button size="sm" variant="ghost" onClick={onCerrar}>
            Cerrar
          </Button>
        </div>

        <div className="max-h-[70vh] overflow-y-auto px-5 py-4">
          {q.isPending ? (
            <LoadingState label="Leyendo el catálogo…" />
          ) : q.isError ? (
            <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />
          ) : (
            <div className="flex flex-col gap-3">
              <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm text-content-muted">
                El código viaja <b className="text-content">adelante</b> en la descripción del pago:{" "}
                <span className="font-mono text-xs">A8 FC 000487 CXP54A4281AE49A</span>. Se carga en
                el rubro y lo heredan sus clasificaciones. Un rubro sin código se paga igual — esto
                sirve para leer más rápido, no es un requisito.
                {/* El ejemplo va SIN guion desde el 19-oct-2026, porque así es como vuelve del
                    banco. Mientras mostró «CXP-54A4281AE49A», quien lo usaba de referencia para
                    buscar en el estado de cuenta buscaba una cadena que el banco nunca trae. */}
              </p>
              {error && <p className="text-sm text-negativo">{error}</p>}

              <table className="w-full">
                <thead>
                  <tr className="text-left text-[11px] uppercase tracking-wider text-content-muted">
                    <th className="px-3 py-1.5 font-semibold">Rubro</th>
                    <th className="px-3 py-1.5 text-right font-semibold">Facturas</th>
                    <th className="px-3 py-1.5 font-semibold">Código</th>
                  </tr>
                </thead>
                <tbody>
                  {conceptos.map((r) => (
                    <Fila key={`C${r.id}`} r={r} />
                  ))}
                </tbody>
              </table>

              <div>
                <Button size="sm" variant="ghost" onClick={() => setVerClasificaciones((v) => !v)}>
                  {verClasificaciones ? "Ocultar" : "Ver"} las clasificaciones ({clasifs.length})
                </Button>
                <p className="px-3 text-[11px] text-content-muted">
                  Solo hace falta bajar acá cuando una clasificación tiene que llevar un código
                  distinto al de su rubro.
                </p>
              </div>

              {verClasificaciones && (
                <div className="flex flex-col gap-2">
                  <Input
                    label="Buscar una clasificación"
                    value={buscar}
                    onChange={(e) => setBuscar(e.target.value)}
                    placeholder="Combustible, alquiler, seguros…"
                    className="max-w-sm"
                  />
                  {/* Decir qué se está viendo y qué NO. Sin esta línea, la lista corta se lee como
                      «estas son todas» y nadie buscaría las otras 155. */}
                  <p className="text-[11px] text-content-muted">
                    {hayBusqueda ? (
                      <>
                        {clasifsVisibles.length} de {clasifs.length} coinciden con «{buscar.trim()}».
                      </>
                    ) : (
                      <>
                        Mostrando las <b className="text-content">{clasifsRelevantes.length}</b> con
                        código propio o con facturas.{" "}
                        {clasifs.length > clasifsRelevantes.length && (
                          <>
                            Las otras {clasifs.length - clasifsRelevantes.length} todavía no tienen
                            movimiento: buscá por nombre para ponerles código desde ya.
                          </>
                        )}
                      </>
                    )}
                  </p>
                  {clasifsVisibles.length === 0 ? (
                    <p className="px-3 py-2 text-sm text-content-muted">
                      Ninguna clasificación se llama así.
                    </p>
                  ) : (
                    <table className="w-full">
                      <tbody>
                        {clasifsVisibles.map((r) => (
                          <Fila key={`L${r.id}`} r={r} />
                        ))}
                      </tbody>
                    </table>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
