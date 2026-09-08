/**
 * Pantalla — Existencias (/inventario).
 *
 * La pregunta que contesta: **qué hay, en qué sede, cuánto vale y qué falta reponer.**
 *
 * Dos cosas que la pantalla tiene que dejar ver, y por eso están arriba y no escondidas:
 *
 *  1. **Los cofres y las urnas se cuentan distinto**, así que sus totales van separados. Sumar 47
 *     cofres con 134 urnas da un número que no significa nada.
 *  2. **Si nadie registra los servicios prestados, las existencias solo suben.** El aviso del
 *     backend lo dice con esas palabras: la pantalla se vería sana mientras en la bodega falta la
 *     mitad.
 */

import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
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
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatMoneda, formatMonto, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useSedes } from "@/features/bancos/hooks";
import { useCategorias, useExistencias, useUnidades } from "@/features/inventario/hooks";
import {
  AvisoInventario,
  BarraNivel,
  MarcaEstadoUnidad,
  MarcaNivel,
} from "@/features/inventario/componentes";

export function ExistenciasPage() {
  const navigate = useNavigate();
  const [sedeID, setSedeID] = useState("");
  const [categoriaID, setCategoriaID] = useState("");
  const [modo, setModo] = useState("");
  const [busca, setBusca] = useState("");
  const [soloBajo, setSoloBajo] = useState(false);
  const [verUnidadesDe, setVerUnidadesDe] = useState<{ id: string; nombre: string } | null>(null);

  const sedes = useSedes();
  const categorias = useCategorias();
  const filtro = useMemo(
    () => ({
      sede_id: sedeID,
      categoria_id: categoriaID,
      modo_control: modo,
      q: busca.trim(),
      solo_bajo_minimo: soloBajo,
    }),
    [sedeID, categoriaID, modo, busca, soloBajo],
  );
  const q = useExistencias(filtro);
  const data = q.data;

  // Blindaje: un arreglo vacío de Go llega como `null` y `null.map()` rompe la pantalla entera.
  const filas = data?.filas ?? [];
  const hayFiltro = sedeID !== "" || categoriaID !== "" || modo !== "" || busca.trim() !== "" || soloBajo;

  const opcionesSede = [
    { value: "", label: "Todas las sedes" },
    ...(sedes.data ?? []).map((s) => ({ value: s.id, label: s.nombre })),
  ];
  const opcionesCategoria = [
    { value: "", label: "Todas las categorías" },
    ...(categorias.data ?? []).map((c) => ({
      value: c.id,
      label: c.padre ? `${c.padre} › ${c.nombre}` : c.nombre,
    })),
  ];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Existencias"
        description="Qué hay, en qué sede está y cuánto vale."
        actions={
          <div className="flex flex-wrap gap-2">
            {/* Botones que navegan: el componente Button no acepta `asChild`, así que se usa
                useNavigate en vez de anidar un Link dentro de un <button>, que no es válido. */}
            <Button variant="secondary" onClick={() => navigate("/inventario/entradas")}>
              Registrar entrada
            </Button>
            <Button onClick={() => navigate("/inventario/servicios")}>Registrar servicio</Button>
          </div>
        }
      />

      {q.isLoading && <LoadingState label="Contando lo que hay…" />}
      {q.isError && <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />}

      {data && (
        <>
          <AvisoInventario texto={data.aviso} />

          {/* Los totales de unidades y cantidades van SEPARADOS: son cosas distintas. */}
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Cofres y otros por unidad</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {data.unidades_totales.toLocaleString("es-CR")}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  en {data.sedes_con_stock} sede(s)
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Urnas y suministros</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {data.cantidades_totales.toLocaleString("es-CR")}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">contados por cantidad</p>
              </CardContent>
            </Card>
            {/*
              Dos números, no uno con una nota al pie. Antes la tarjeta mostraba TODO lo que hay en
              bodega y aclaraba en letra chica cuánto era del proveedor, dejándole la resta al
              usuario. El capital propio —la plata que la empresa tiene invertida— es el número que
              se usa para decidir, así que va arriba.
            */}
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Capital propio detenido</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {formatMoneda(data.valor_propio_crc)}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  {toNumber(data.consignadas_crc) > 0 ? (
                    <>
                      + {formatMoneda(data.consignadas_crc)} del proveedor (
                      {data.unidades_consignadas} unidad(es)) ={" "}
                      {formatMoneda(data.valor_total_crc)} en bodega ·{" "}
                      <Link to="/inventario/consignacion" className="text-accent underline">
                        ver consignación
                      </Link>
                    </>
                  ) : (
                    "al costo · no hay mercadería consignada"
                  )}
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Bajo el mínimo</p>
                <p
                  className={cn(
                    "mt-1 text-2xl font-semibold tabular-nums",
                    data.bajo_minimo > 0 ? "text-negativo" : "text-content",
                  )}
                >
                  {data.bajo_minimo}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  {data.bajo_minimo > 0 ? (
                    <Link to="/inventario/reposicion" className="text-accent underline">
                      ver qué pedir
                    </Link>
                  ) : (
                    "nada por reponer"
                  )}
                </p>
              </CardContent>
            </Card>
          </div>

          {/* Filtros: alineados por arriba y con labels de una línea. */}
          <Card>
            <CardContent className="flex flex-wrap items-start gap-4 pt-6">
              <div className="w-48">
                <Select
                  label="Sede"
                  value={sedeID}
                  onChange={(e) => setSedeID(e.target.value)}
                  options={opcionesSede}
                />
              </div>
              <div className="w-56">
                <Select
                  label="Categoría"
                  value={categoriaID}
                  onChange={(e) => setCategoriaID(e.target.value)}
                  options={opcionesCategoria}
                />
              </div>
              <div className="w-44">
                <Select
                  label="Cómo se cuenta"
                  value={modo}
                  onChange={(e) => setModo(e.target.value)}
                  options={[
                    { value: "", label: "Todo" },
                    { value: "UNIDAD", label: "Por unidad" },
                    { value: "CANTIDAD", label: "Por cantidad" },
                  ]}
                />
              </div>
              <div className="w-48">
                <Input
                  label="Buscar artículo"
                  value={busca}
                  onChange={(e) => setBusca(e.target.value)}
                  placeholder="Roble, urna…"
                />
              </div>
              <div className="flex flex-col gap-1.5">
                <span className="text-sm font-medium text-transparent" aria-hidden="true">
                  &nbsp;
                </span>
                <label className="flex h-10 items-center gap-2 text-sm text-content">
                  <input
                    type="checkbox"
                    checked={soloBajo}
                    onChange={(e) => setSoloBajo(e.target.checked)}
                    className="h-4 w-4 rounded border-border accent-accent"
                  />
                  Solo lo que falta
                </label>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Por artículo y sede</CardTitle>
              {hayFiltro && (
                <p className="mt-1 flex flex-wrap items-center gap-2 text-xs text-content-muted">
                  <span>
                    Mostrando {filas.length} fila(s). Los totales de arriba son de lo filtrado.
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      setSedeID("");
                      setCategoriaID("");
                      setModo("");
                      setBusca("");
                      setSoloBajo(false);
                    }}
                    className="font-medium text-accent underline"
                  >
                    Quitar el filtro
                  </button>
                </p>
              )}
            </CardHeader>
            <CardContent>
              {filas.length === 0 ? (
                <EmptyState
                  message={
                    hayFiltro
                      ? "Nada calza con el filtro."
                      : "Todavía no hay existencias. Empezá por el catálogo y después registrá una entrada."
                  }
                />
              ) : (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Artículo</TH>
                        <TH>Sede</TH>
                        <TH className="text-right">Hay</TH>
                        <TH className="text-right">Mín</TH>
                        <TH className="text-right">Máx</TH>
                        <TH>Estado</TH>
                        <TH className="text-right">Costo unit.</TH>
                        <TH className="text-right">Valor</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {filas.map((f) => (
                        <TR key={`${f.articulo_id}-${f.sede_id}`}>
                          <TD>
                            {f.modo_control === "UNIDAD" ? (
                              <button
                                type="button"
                                onClick={() =>
                                  setVerUnidadesDe(
                                    verUnidadesDe?.id === f.articulo_id
                                      ? null
                                      : { id: f.articulo_id, nombre: f.articulo },
                                  )
                                }
                                className="text-left font-medium text-accent underline"
                              >
                                {f.articulo}
                              </button>
                            ) : (
                              <span className="font-medium text-content">{f.articulo}</span>
                            )}
                            <span className="block text-xs text-content-muted">
                              {f.codigo} · {f.categoria}
                              {f.consignadas > 0 && ` · ${f.consignadas} consignada(s)`}
                            </span>
                          </TD>
                          <TD className="text-content-muted">{f.sede}</TD>
                          <TD className="text-right font-medium tabular-nums">{f.cantidad}</TD>
                          <TD className="text-right tabular-nums text-content-muted">
                            {f.minimo === 0 ? "—" : f.minimo}
                          </TD>
                          <TD className="text-right tabular-nums text-content-muted">
                            {f.maximo === 0 ? "—" : f.maximo}
                          </TD>
                          <TD>
                            <MarcaNivel estado={f.estado} />
                            <BarraNivel
                              hay={f.cantidad}
                              minimo={f.minimo}
                              maximo={f.maximo}
                              estado={f.estado}
                            />
                          </TD>
                          <TD className="text-right tabular-nums text-content-muted">
                            {formatMonto(f.costo_unitario_crc)}
                          </TD>
                          <TD className="text-right font-medium tabular-nums">
                            {formatMonto(f.valor_crc)}
                          </TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              )}
            </CardContent>
          </Card>

          {verUnidadesDe && (
            <FichasDeUnidad
              articuloID={verUnidadesDe.id}
              nombre={verUnidadesDe.nombre}
              sedeID={sedeID}
            />
          )}
        </>
      )}
    </div>
  );
}

/**
 * Las fichas de un artículo controlado por unidad.
 *
 * `dias_quieta` es la columna que justifica la pantalla: una unidad de ₡423.750 parada 400 días es
 * capital detenido que no se ve mirando el total.
 */
function FichasDeUnidad({
  articuloID,
  nombre,
  sedeID,
}: {
  articuloID: string;
  nombre: string;
  sedeID: string;
}) {
  const q = useUnidades({ articulo_id: articuloID, sede_id: sedeID, limite: 200 });
  const unidades = q.data ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Unidades de «{nombre}»</CardTitle>
        <p className="mt-1 text-xs text-content-muted">
          Una fila por objeto físico. «Quieta» son los días desde su último movimiento.
        </p>
      </CardHeader>
      <CardContent>
        {q.isLoading && <LoadingState label="Buscando las fichas…" />}
        {q.isError && <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />}
        {q.data && unidades.length === 0 && (
          <EmptyState message="Este artículo no tiene unidades registradas." />
        )}
        {unidades.length > 0 && (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Unidad</TH>
                  <TH>Dónde está</TH>
                  <TH>Estado</TH>
                  <TH className="text-right">Costo</TH>
                  <TH className="text-right">Quieta</TH>
                  <TH>Ingresó</TH>
                  <TH>Servicio</TH>
                </TR>
              </THead>
              <TBody>
                {unidades.map((u) => (
                  <TR key={u.id}>
                    <TD className="font-medium tabular-nums text-content">
                      {u.numero}
                      {u.es_consignada && (
                        <span className="block text-xs text-content-muted">
                          consignada{u.proveedor ? ` · ${u.proveedor}` : ""}
                        </span>
                      )}
                    </TD>
                    <TD className="text-content-muted">
                      {u.estado === "EN_TRANSITO" ? (
                        <>
                          en camino
                          <span className="block text-xs">a {u.sede_destino}</span>
                        </>
                      ) : (
                        u.sede || "—"
                      )}
                    </TD>
                    <TD>
                      <MarcaEstadoUnidad estado={u.estado} legible={u.estado_legible} />
                    </TD>
                    <TD className="text-right tabular-nums">{formatMonto(u.costo_crc)}</TD>
                    <TD
                      className={cn(
                        "text-right tabular-nums",
                        u.dias_quieta >= 90 ? "font-medium text-negativo" : "text-content-muted",
                      )}
                    >
                      {u.dias_quieta === 0 ? "—" : `${u.dias_quieta} d`}
                    </TD>
                    <TD className="tabular-nums text-content-muted">{u.ingresada_en}</TD>
                    <TD className="tabular-nums text-content-muted">{u.servicio_numero || "—"}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>
    </Card>
  );
}
