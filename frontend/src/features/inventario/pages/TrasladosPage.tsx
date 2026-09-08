/**
 * Pantalla — Traslados entre sedes (/inventario/traslados).
 *
 * Con 15 plazas, el traslado es donde se pierde el control. La pantalla se organiza alrededor de eso:
 * **lo que va en camino va arriba**, con los días que lleva, y la primera cosa que se ve es lo que
 * nadie recibió. Un traslado sin recibir no es un detalle administrativo: es mercadería que salió de
 * una bodega y no llegó a la otra.
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
import { cn } from "@/lib/cn";
import { mensajeError } from "@/lib/apiError";
import { useSedes } from "@/features/bancos/hooks";
import {
  useArticulos,
  useCrearTraslado,
  useRecibirTraslado,
  useTraslados,
  useUnidades,
} from "@/features/inventario/hooks";

/** Días sin recibir a partir de los cuales el traslado se marca en rojo. */
const DIAS_PREOCUPANTES = 3;

function hoyCR(): string {
  const d = new Date();
  const cr = new Date(d.getTime() - (d.getTimezoneOffset() + 360) * 60_000);
  return cr.toISOString().slice(0, 10);
}

interface LineaBorrador {
  articuloID: string;
  unidadNumero: string;
  cantidad: string;
}

export function TrasladosPage() {
  const toast = useToast();
  const sedes = useSedes();
  const articulos = useArticulos();
  const crear = useCrearTraslado();
  const recibir = useRecibirTraslado();
  const enCamino = useTraslados("EN_TRANSITO");
  const recibidos = useTraslados("RECIBIDO");

  const [origen, setOrigen] = useState("");
  const [destino, setDestino] = useState("");
  const [fecha, setFecha] = useState(hoyCR());
  const [nota, setNota] = useState("");
  const [lineas, setLineas] = useState<LineaBorrador[]>([
    { articuloID: "", unidadNumero: "", cantidad: "1" },
  ]);

  const lista = articulos.data ?? [];
  const modoDe = (id: string) => lista.find((a) => a.id === id)?.modo_control;

  function enviar() {
    if (!origen || !destino) {
      toast.error("Elegí de qué sede sale y a cuál va.");
      return;
    }
    if (origen === destino) {
      toast.error("El origen y el destino tienen que ser sedes distintas.");
      return;
    }
    const items = lineas
      .filter((l) => l.articuloID !== "")
      .map((l) =>
        modoDe(l.articuloID) === "UNIDAD"
          ? { articulo_id: l.articuloID, unidad_numero: l.unidadNumero.trim() }
          : { articulo_id: l.articuloID, cantidad: Number(l.cantidad) || 0 },
      );
    if (items.length === 0) {
      toast.error("Agregá al menos un artículo.");
      return;
    }
    crear.mutate(
      { sede_origen_id: origen, sede_destino_id: destino, fecha, nota: nota.trim(), lineas: items },
      {
        onSuccess: (t) => {
          toast.success(
            `Traslado ${t.numero} enviado. Lo que va en camino no cuenta en ninguna de las dos sedes hasta que alguien lo reciba.`,
          );
          setLineas([{ articuloID: "", unidadNumero: "", cantidad: "1" }]);
          setNota("");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  const camino = enCamino.data ?? [];
  const hechos = recibidos.data ?? [];
  const sinRecibir = camino.filter((t) => t.dias_en_camino >= DIAS_PREOCUPANTES).length;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Traslados entre sedes"
        description="Lo que sale de una bodega no llega a la otra hasta que alguien lo recibe."
      />

      {sinRecibir > 0 && (
        <div className="rounded-lg border border-negativo/40 bg-negativo/10 px-4 py-3">
          <p className="text-sm font-medium text-content">
            {sinRecibir} traslado(s) llevan {DIAS_PREOCUPANTES} días o más sin recibirse
          </p>
          <p className="mt-0.5 text-sm text-content-muted">
            Eso es mercadería que salió de una sede y nadie confirmó en la otra. Hay que averiguar
            dónde está antes de que se dé por perdida.
          </p>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>En camino ahora</CardTitle>
        </CardHeader>
        <CardContent>
          {enCamino.isLoading && <LoadingState label="Buscando lo que va en camino…" />}
          {enCamino.isError && (
            <ErrorState message={mensajeError(enCamino.error)} onRetry={() => enCamino.refetch()} />
          )}
          {enCamino.data && camino.length === 0 && (
            <EmptyState message="No hay nada en camino entre sedes." />
          )}
          {camino.length > 0 && (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Traslado</TH>
                    <TH>Qué va</TH>
                    <TH>Sale de</TH>
                    <TH>Va a</TH>
                    <TH>Salió</TH>
                    <TH className="text-right">Días</TH>
                    <TH className="text-right">Recibir</TH>
                  </TR>
                </THead>
                <TBody>
                  {camino.map((t) => (
                    <TR key={t.id}>
                      <TD className="font-medium tabular-nums text-content">
                        {t.numero}
                        {t.enviado_por && (
                          <span className="block text-xs font-normal text-content-muted">
                            lo envió {t.enviado_por}
                          </span>
                        )}
                      </TD>
                      <TD className="text-content-muted">
                        {(t.lineas ?? []).map((l, i) => (
                          <span key={i} className="block text-xs">
                            {l.unidad_numero
                              ? `${l.articulo} · ${l.unidad_numero}`
                              : `${l.articulo} × ${l.cantidad}`}
                          </span>
                        ))}
                      </TD>
                      <TD className="text-content-muted">{t.sede_origen}</TD>
                      <TD className="text-content-muted">{t.sede_destino}</TD>
                      <TD className="tabular-nums text-content-muted">{t.enviado_en}</TD>
                      <TD
                        className={cn(
                          "text-right tabular-nums",
                          t.dias_en_camino >= DIAS_PREOCUPANTES
                            ? "font-medium text-negativo"
                            : "text-content-muted",
                        )}
                      >
                        {t.dias_en_camino}
                      </TD>
                      <TD className="text-right">
                        <Button
                          size="sm"
                          loading={recibir.isPending}
                          onClick={() =>
                            recibir.mutate(
                              { id: t.id },
                              {
                                onSuccess: () =>
                                  toast.success(
                                    `${t.numero} recibido en ${t.sede_destino}: ya cuenta en sus existencias.`,
                                  ),
                                onError: (err) => toast.error(mensajeError(err)),
                              },
                            )
                          }
                        >
                          Recibir
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
          <CardTitle>Enviar a otra sede</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Select
              label="Sale de *"
              value={origen}
              onChange={(e) => setOrigen(e.target.value)}
              options={[
                { value: "", label: "— elegir —" },
                ...(sedes.data ?? []).map((s) => ({ value: s.id, label: s.nombre })),
              ]}
            />
            <Select
              label="Va a *"
              value={destino}
              onChange={(e) => setDestino(e.target.value)}
              options={[
                { value: "", label: "— elegir —" },
                ...(sedes.data ?? [])
                  .filter((s) => s.id !== origen)
                  .map((s) => ({ value: s.id, label: s.nombre })),
              ]}
            />
            <Input label="Fecha *" type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} />
            <Input label="Nota" value={nota} onChange={(e) => setNota(e.target.value)} />
          </div>

          <div className="flex flex-col gap-3">
            <p className="text-sm font-medium text-content">Qué se manda</p>
            {lineas.map((l, i) => (
              <LineaTraslado
                key={i}
                linea={l}
                articulos={lista}
                sedeOrigen={origen}
                onCambiar={(campo, valor) =>
                  setLineas((prev) => prev.map((x, k) => (k === i ? { ...x, [campo]: valor } : x)))
                }
                onQuitar={
                  lineas.length > 1
                    ? () => setLineas((prev) => prev.filter((_, k) => k !== i))
                    : undefined
                }
              />
            ))}
            <Button
              variant="secondary"
              className="w-fit"
              onClick={() =>
                setLineas((prev) => [...prev, { articuloID: "", unidadNumero: "", cantidad: "1" }])
              }
            >
              + Otro artículo
            </Button>
          </div>

          <div className="flex flex-wrap items-center gap-3 border-t border-border pt-4">
            <Button onClick={enviar} loading={crear.isPending}>
              Enviar
            </Button>
            <span className="text-xs text-content-muted">
              Sale del origen y queda en camino con su responsable, hasta que la sede destino lo
              reciba.
            </span>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Ya recibidos</CardTitle>
        </CardHeader>
        <CardContent>
          {hechos.length === 0 ? (
            <EmptyState message="Todavía no se recibió ningún traslado." />
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Traslado</TH>
                    <TH>Ruta</TH>
                    <TH>Salió</TH>
                    <TH>Llegó</TH>
                    <TH className="text-right">Tardó</TH>
                    <TH>Recibió</TH>
                  </TR>
                </THead>
                <TBody>
                  {hechos.map((t) => (
                    <TR key={t.id}>
                      <TD className="font-medium tabular-nums text-content">{t.numero}</TD>
                      <TD className="text-content-muted">
                        {t.sede_origen} → {t.sede_destino}
                      </TD>
                      <TD className="tabular-nums text-content-muted">{t.enviado_en}</TD>
                      <TD className="tabular-nums text-content-muted">{t.recibido_en}</TD>
                      <TD className="text-right tabular-nums text-content-muted">
                        {t.dias_en_camino} d
                      </TD>
                      <TD className="text-content-muted">
                        {t.recibido_por || <Badge tone="neutral">sin registrar</Badge>}
                      </TD>
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

/** Una línea del traslado. Solo ofrece unidades que están en la sede de ORIGEN. */
function LineaTraslado({
  linea,
  articulos,
  sedeOrigen,
  onCambiar,
  onQuitar,
}: {
  linea: LineaBorrador;
  articulos: { id: string; nombre: string; categoria: string; modo_control: string }[];
  sedeOrigen: string;
  onCambiar: (campo: keyof LineaBorrador, valor: string) => void;
  onQuitar?: () => void;
}) {
  const esUnidad = articulos.find((a) => a.id === linea.articuloID)?.modo_control === "UNIDAD";
  const unidades = useUnidades(
    { articulo_id: linea.articuloID, sede_id: sedeOrigen, estado: "DISPONIBLE", limite: 200 },
    esUnidad && linea.articuloID !== "" && sedeOrigen !== "",
  );
  const disponibles = unidades.data ?? [];

  return (
    <div className="flex flex-wrap items-start gap-3 rounded-lg border border-border bg-surface-raised p-3">
      <div className="min-w-56 flex-1">
        <Select
          label="Artículo"
          value={linea.articuloID}
          onChange={(e) => onCambiar("articuloID", e.target.value)}
          options={[
            { value: "", label: "— elegir —" },
            ...articulos.map((a) => ({ value: a.id, label: `${a.nombre} · ${a.categoria}` })),
          ]}
        />
      </div>
      {esUnidad ? (
        <div className="w-52">
          <Select
            label="Unidad"
            value={linea.unidadNumero}
            onChange={(e) => onCambiar("unidadNumero", e.target.value)}
            options={[
              {
                value: "",
                label: sedeOrigen === "" ? "elegí el origen primero" : "— elegir unidad —",
              },
              ...disponibles.map((u) => ({ value: u.numero, label: u.numero })),
            ]}
          />
          {sedeOrigen !== "" && disponibles.length === 0 && !unidades.isLoading && (
            <p className="mt-1 text-xs text-negativo">
              No hay unidades disponibles de este artículo en la sede de origen.
            </p>
          )}
        </div>
      ) : (
        <div className="w-28">
          <Input
            label="Cantidad"
            value={linea.cantidad}
            onChange={(e) => onCambiar("cantidad", e.target.value)}
            inputMode="numeric"
          />
        </div>
      )}
      {onQuitar && (
        <div className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-transparent" aria-hidden="true">
            &nbsp;
          </span>
          <Button variant="ghost" className="text-negativo" onClick={onQuitar}>
            Quitar
          </Button>
        </div>
      )}
    </div>
  );
}
