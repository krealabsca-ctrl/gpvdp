/**
 * Pantalla — Catálogo del inventario (/inventario/catalogo).
 *
 * Categorías, artículos y los mínimos por sede. Dos cosas que la pantalla explica en lugar de dejar
 * que se descubran a los golpes:
 *
 *  · **Cómo se cuenta el artículo se elige al crearlo y no se cambia después.** Pasar de cantidad a
 *    unidad exigiría inventar una ficha por objeto ya contado, y al revés habría que destruir fichas
 *    con historia. Si está mal, se crea el artículo correcto y se da de baja el otro.
 *  · **El mínimo es por sede.** El de Cartago no tiene nada que ver con el de Sabana, y sin mínimo
 *    no hay semáforo: la existencia se muestra sin juzgarla.
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
import {
  useActualizarCategoria,
  useArticulos,
  useCategorias,
  useCrearArticulo,
  useCrearCategoria,
  useFijarNivel,
  useNiveles,
} from "@/features/inventario/hooks";
import { MarcaModo } from "@/features/inventario/componentes";
import type { ModoControl } from "@/api/inventario";

export function CatalogoInvPage() {
  const toast = useToast();
  const categorias = useCategorias(true);
  const articulos = useArticulos({ incluir_inactivos: true });
  const crearCat = useCrearCategoria();
  const cambiarCat = useActualizarCategoria();
  const crearArt = useCrearArticulo();

  const [nombreCat, setNombreCat] = useState("");
  const [padreCat, setPadreCat] = useState("");
  const [art, setArt] = useState({
    codigo: "",
    nombre: "",
    categoria_id: "",
    modo_control: "UNIDAD" as ModoControl,
  });
  const [verNivelesDe, setVerNivelesDe] = useState<{ id: string; nombre: string } | null>(null);

  const cats = categorias.data ?? [];
  const arts = articulos.data ?? [];

  function agregarCategoria() {
    if (nombreCat.trim() === "") {
      toast.error("Escribí el nombre de la categoría.");
      return;
    }
    crearCat.mutate(
      { nombre: nombreCat.trim(), padre_id: padreCat },
      {
        onSuccess: () => {
          toast.success("Categoría agregada.");
          setNombreCat("");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function agregarArticulo() {
    if (art.codigo.trim() === "" || art.nombre.trim() === "" || art.categoria_id === "") {
      toast.error("Hacen falta el código, el nombre y la categoría.");
      return;
    }
    crearArt.mutate(
      { ...art, codigo: art.codigo.trim().toUpperCase(), nombre: art.nombre.trim(), activo: true },
      {
        onSuccess: () => {
          toast.success(`«${art.nombre.trim()}» quedó en el catálogo.`);
          setArt({ codigo: "", nombre: "", categoria_id: "", modo_control: "UNIDAD" });
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Catálogo del inventario"
        description="Categorías, artículos y el mínimo de cada uno en cada sede."
      />

      <Card>
        <CardHeader>
          <CardTitle>Categorías</CardTitle>
          <p className="mt-1 text-xs text-content-muted">
            Un nivel de subcategorías: «Cofres › Línea alta». Alcanza para agrupar y no obliga a
            mantener un árbol.
          </p>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-wrap items-start gap-3">
            <div className="w-56">
              <Input
                label="Categoría nueva"
                value={nombreCat}
                onChange={(e) => setNombreCat(e.target.value)}
                placeholder="Cofres"
              />
            </div>
            <div className="w-52">
              <Select
                label="Dentro de (opcional)"
                value={padreCat}
                onChange={(e) => setPadreCat(e.target.value)}
                options={[
                  { value: "", label: "— es de primer nivel —" },
                  ...cats.filter((c) => c.padre_id === "").map((c) => ({ value: c.id, label: c.nombre })),
                ]}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <span className="text-sm font-medium text-transparent" aria-hidden="true">
                &nbsp;
              </span>
              <Button onClick={agregarCategoria} loading={crearCat.isPending}>
                Agregar
              </Button>
            </div>
          </div>

          {categorias.isLoading && <LoadingState label="Cargando categorías…" />}
          {cats.length === 0 && categorias.data && (
            <EmptyState message="Todavía no hay categorías. Empezá creando «Cofres» y «Urnas»." />
          )}
          {cats.length > 0 && (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Categoría</TH>
                    <TH className="text-right">Artículos</TH>
                    <TH>Estado</TH>
                    <TH className="text-right">Acción</TH>
                  </TR>
                </THead>
                <TBody>
                  {cats.map((c) => (
                    <TR key={c.id} className={c.activo ? undefined : "opacity-60"}>
                      <TD className="font-medium text-content">
                        {c.padre ? `${c.padre} › ${c.nombre}` : c.nombre}
                      </TD>
                      <TD className="text-right tabular-nums text-content-muted">{c.articulos}</TD>
                      <TD>
                        <Badge tone={c.activo ? "positivo" : "neutral"}>
                          {c.activo ? "activa" : "inactiva"}
                        </Badge>
                      </TD>
                      <TD className="text-right">
                        <Button
                          size="sm"
                          variant="ghost"
                          loading={cambiarCat.isPending}
                          onClick={() =>
                            cambiarCat.mutate(
                              { id: c.id, nombre: c.nombre, activo: !c.activo },
                              {
                                onSuccess: () =>
                                  toast.success(
                                    c.activo
                                      ? `«${c.nombre}» ya no se ofrece para elegir.`
                                      : `«${c.nombre}» vuelve a estar disponible.`,
                                  ),
                                onError: (err) => toast.error(mensajeError(err)),
                              },
                            )
                          }
                        >
                          {c.activo ? "Desactivar" : "Reactivar"}
                        </Button>
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Artículos</CardTitle>
          <p className="mt-1 text-xs text-content-muted">
            <strong>Cómo se cuenta se elige acá y no se cambia después</strong>: un cofre se controla
            por unidad porque hay que saber cuál se usó en cuál servicio; una urna por cantidad
            porque se pide cada semana y no tiene identidad propia.
          </p>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
            <Input
              label="Código *"
              value={art.codigo}
              onChange={(e) => setArt({ ...art, codigo: e.target.value })}
              placeholder="COF-ROBLE"
            />
            <Input
              label="Nombre *"
              value={art.nombre}
              onChange={(e) => setArt({ ...art, nombre: e.target.value })}
              placeholder="Roble tallado"
            />
            <Select
              label="Categoría *"
              value={art.categoria_id}
              onChange={(e) => setArt({ ...art, categoria_id: e.target.value })}
              options={[
                { value: "", label: "— elegir —" },
                ...cats
                  .filter((c) => c.activo)
                  .map((c) => ({
                    value: c.id,
                    label: c.padre ? `${c.padre} › ${c.nombre}` : c.nombre,
                  })),
              ]}
            />
            <Select
              label="Cómo se cuenta *"
              value={art.modo_control}
              onChange={(e) => setArt({ ...art, modo_control: e.target.value as ModoControl })}
              options={[
                { value: "UNIDAD", label: "Por unidad (cofres)" },
                { value: "CANTIDAD", label: "Por cantidad (urnas)" },
              ]}
            />
            <div className="flex flex-col gap-1.5">
              <span className="text-sm font-medium text-transparent" aria-hidden="true">
                &nbsp;
              </span>
              <Button onClick={agregarArticulo} loading={crearArt.isPending}>
                Agregar
              </Button>
            </div>
          </div>

          {articulos.isLoading && <LoadingState label="Cargando el catálogo…" />}
          {articulos.isError && (
            <ErrorState message={mensajeError(articulos.error)} onRetry={() => articulos.refetch()} />
          )}
          {arts.length === 0 && articulos.data && (
            <EmptyState message="Todavía no hay artículos." />
          )}
          {arts.length > 0 && (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Artículo</TH>
                    <TH>Categoría</TH>
                    <TH>Cómo se cuenta</TH>
                    <TH>Estado</TH>
                    <TH className="text-right">Mínimos por sede</TH>
                  </TR>
                </THead>
                <TBody>
                  {arts.map((a) => (
                    <TR key={a.id} className={a.activo ? undefined : "opacity-60"}>
                      <TD>
                        <span className="font-medium text-content">{a.nombre}</span>
                        <span className="block text-xs text-content-muted">{a.codigo}</span>
                      </TD>
                      <TD className="text-content-muted">{a.categoria}</TD>
                      <TD>
                        <MarcaModo modo={a.modo_control} />
                      </TD>
                      <TD>
                        <Badge tone={a.activo ? "positivo" : "neutral"}>
                          {a.activo ? "activo" : "inactivo"}
                        </Badge>
                      </TD>
                      <TD className="text-right">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() =>
                            setVerNivelesDe(
                              verNivelesDe?.id === a.id ? null : { id: a.id, nombre: a.nombre },
                            )
                          }
                        >
                          {verNivelesDe?.id === a.id ? "Cerrar" : "Fijar mínimos"}
                        </Button>
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>

      {verNivelesDe && <NivelesDeArticulo id={verNivelesDe.id} nombre={verNivelesDe.nombre} />}
    </div>
  );
}

/** Los mínimos y máximos de un artículo, sede por sede. */
function NivelesDeArticulo({ id, nombre }: { id: string; nombre: string }) {
  const toast = useToast();
  const q = useNiveles(id);
  const fijar = useFijarNivel();
  const [edicion, setEdicion] = useState<Record<string, { minimo: string; maximo: string }>>({});

  const niveles = q.data ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Mínimos de «{nombre}»</CardTitle>
        <p className="mt-1 text-xs text-content-muted">
          Sin mínimo (0) no hay semáforo ni sugerencia de pedido: la existencia se muestra sin
          juzgarla, que es lo honesto cuando nadie definió cuánto debería haber.
        </p>
      </CardHeader>
      <CardContent>
        {q.isLoading && <LoadingState label="Cargando las sedes…" />}
        {q.data && niveles.length === 0 && (
          <EmptyState message="No hay sedes cargadas. El inventario se organiza por sede: cargalas primero en el catálogo de Bancos." />
        )}
        {niveles.length > 0 && (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Sede</TH>
                  <TH className="text-right">Mínimo</TH>
                  <TH className="text-right">Máximo</TH>
                  <TH className="text-right">Guardar</TH>
                </TR>
              </THead>
              <TBody>
                {niveles.map((n) => {
                  const e = edicion[n.sede_id] ?? {
                    minimo: String(n.minimo),
                    maximo: String(n.maximo),
                  };
                  return (
                    <TR key={n.sede_id}>
                      <TD className="font-medium text-content">{n.sede}</TD>
                      <TD className="text-right">
                        <div className="ml-auto w-24">
                          <Input
                            aria-label={`Mínimo en ${n.sede}`}
                            value={e.minimo}
                            onChange={(ev) =>
                              setEdicion({
                                ...edicion,
                                [n.sede_id]: { ...e, minimo: ev.target.value },
                              })
                            }
                            inputMode="numeric"
                          />
                        </div>
                      </TD>
                      <TD className="text-right">
                        <div className="ml-auto w-24">
                          <Input
                            aria-label={`Máximo en ${n.sede}`}
                            value={e.maximo}
                            onChange={(ev) =>
                              setEdicion({
                                ...edicion,
                                [n.sede_id]: { ...e, maximo: ev.target.value },
                              })
                            }
                            inputMode="numeric"
                          />
                        </div>
                      </TD>
                      <TD className="text-right">
                        <Button
                          size="sm"
                          loading={fijar.isPending}
                          onClick={() =>
                            fijar.mutate(
                              {
                                articuloId: id,
                                sedeId: n.sede_id,
                                minimo: Number(e.minimo) || 0,
                                maximo: Number(e.maximo) || 0,
                              },
                              {
                                onSuccess: () => toast.success(`Mínimo de ${n.sede} guardado.`),
                                onError: (err) => toast.error(mensajeError(err)),
                              },
                            )
                          }
                        >
                          Guardar
                        </Button>
                      </TD>
                    </TR>
                  );
                })}
              </TBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>
    </Card>
  );
}
