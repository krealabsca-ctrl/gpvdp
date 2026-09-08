/**
 * Pantalla — Servicio prestado (/inventario/servicios).
 *
 * **Es la pantalla que hace funcionar al módulo.** El stock baja acá: si los funerales no se anotan,
 * las existencias solo suben y en la bodega falta la mitad sin que ninguna pantalla lo muestre.
 *
 * Antes de este módulo el servicio prestado no se registraba en ninguna parte del sistema (Cuentas
 * por Cobrar lleva la cartera de asociados, que es otra cosa), así que esta pantalla no reemplaza
 * nada: agrega el hecho que faltaba.
 */

import { useState } from "react";
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
  useToast,
} from "@/components/ui";
import { formatMonto } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useSedes } from "@/features/bancos/hooks";
import {
  useArticulos,
  useRegistrarServicio,
  useServicios,
  useUnidades,
} from "@/features/inventario/hooks";

function hoyCR(): string {
  const d = new Date();
  const cr = new Date(d.getTime() - (d.getTimezoneOffset() + 360) * 60_000);
  return cr.toISOString().slice(0, 10);
}

/** Una línea de consumo en el formulario. */
interface LineaBorrador {
  articuloID: string;
  unidadNumero: string;
  cantidad: string;
}

export function ServiciosPage() {
  const toast = useToast();
  const sedes = useSedes();
  const articulos = useArticulos();
  const registrar = useRegistrarServicio();
  const historial = useServicios();

  const [sedeID, setSedeID] = useState("");
  const [fecha, setFecha] = useState(hoyCR());
  const [aNombreDe, setANombreDe] = useState("");
  const [nota, setNota] = useState("");
  const [lineas, setLineas] = useState<LineaBorrador[]>([
    { articuloID: "", unidadNumero: "", cantidad: "1" },
  ]);

  const lista = articulos.data ?? [];
  const modoDe = (id: string) => lista.find((a) => a.id === id)?.modo_control;

  function cambiar(i: number, campo: keyof LineaBorrador, valor: string) {
    setLineas((prev) => prev.map((l, k) => (k === i ? { ...l, [campo]: valor } : l)));
  }

  function enviar() {
    if (!sedeID) {
      toast.error("Elegí la sede donde se prestó el servicio.");
      return;
    }
    const consumos = lineas
      .filter((l) => l.articuloID !== "")
      .map((l) =>
        modoDe(l.articuloID) === "UNIDAD"
          ? { articulo_id: l.articuloID, unidad_numero: l.unidadNumero.trim() }
          : { articulo_id: l.articuloID, cantidad: Number(l.cantidad) || 0 },
      );
    if (consumos.length === 0) {
      toast.error("Agregá al menos un artículo consumido.");
      return;
    }
    registrar.mutate(
      { sede_id: sedeID, fecha, a_nombre_de: aNombreDe.trim(), nota: nota.trim(), consumos },
      {
        onSuccess: (sv) => {
          toast.success(
            `Servicio ${sv.numero} registrado. Costo de producto: ${formatMonto(sv.costo_producto_crc)}. Las existencias ya bajaron.`,
          );
          setANombreDe("");
          setNota("");
          setLineas([{ articuloID: "", unidadNumero: "", cantidad: "1" }]);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  const servicios = historial.data ?? [];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Servicio prestado"
        description="El funeral realizado y qué consumió. Es lo que descarga las existencias."
      />

      <Card>
        <CardHeader>
          <CardTitle>Registrar el servicio</CardTitle>
          <p className="mt-1 text-xs text-content-muted">
            El número lo pone el sistema. Si no consumió nada de bodega, no hace falta registrarlo
            acá.
          </p>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Select
              label="Sede *"
              value={sedeID}
              onChange={(e) => setSedeID(e.target.value)}
              options={[
                { value: "", label: "— elegir —" },
                ...(sedes.data ?? []).map((s) => ({ value: s.id, label: s.nombre })),
              ]}
            />
            <Input label="Fecha *" type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} />
            <Input
              label="A nombre de"
              value={aNombreDe}
              onChange={(e) => setANombreDe(e.target.value)}
              placeholder="Familia Solano Vargas"
            />
            <Input label="Nota" value={nota} onChange={(e) => setNota(e.target.value)} />
          </div>

          <div className="flex flex-col gap-3">
            <p className="text-sm font-medium text-content">Qué se usó</p>
            {lineas.map((l, i) => (
              <LineaConsumo
                key={i}
                linea={l}
                articulos={lista}
                sedeID={sedeID}
                onCambiar={(campo, valor) => cambiar(i, campo, valor)}
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
            <Button onClick={enviar} loading={registrar.isPending}>
              Registrar el servicio
            </Button>
            <span className="text-xs text-content-muted">
              Al registrarlo, las existencias bajan y las unidades usadas quedan marcadas.
            </span>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Servicios registrados</CardTitle>
        </CardHeader>
        <CardContent>
          {historial.isLoading && <LoadingState label="Buscando los servicios…" />}
          {historial.isError && (
            <ErrorState
              message={mensajeError(historial.error)}
              onRetry={() => historial.refetch()}
            />
          )}
          {historial.data && servicios.length === 0 && (
            <EmptyState message="Todavía no se registró ningún servicio. Mientras no se registren, las existencias solo suben." />
          )}
          {servicios.length > 0 && (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Número</TH>
                    <TH>Fecha</TH>
                    <TH>Sede</TH>
                    <TH>A nombre de</TH>
                    <TH className="text-right">Costo de producto</TH>
                  </TR>
                </THead>
                <TBody>
                  {servicios.map((s) => (
                    <TR key={s.id}>
                      <TD className="font-medium tabular-nums text-content">{s.numero}</TD>
                      <TD className="tabular-nums text-content-muted">{s.fecha}</TD>
                      <TD className="text-content-muted">{s.sede}</TD>
                      <TD className="text-content-muted">{s.a_nombre_de || "—"}</TD>
                      <TD className="text-right font-medium tabular-nums">
                        {formatMonto(s.costo_producto_crc)}
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

/**
 * Una línea de consumo.
 *
 * Para un artículo por unidad, el selector ofrece **solo las unidades disponibles en esa sede**: es
 * lo que evita el error más común —elegir un cofre que está en otra plaza— antes de que ocurra, en
 * lugar de rechazarlo después.
 */
function LineaConsumo({
  linea,
  articulos,
  sedeID,
  onCambiar,
  onQuitar,
}: {
  linea: LineaBorrador;
  articulos: { id: string; nombre: string; categoria: string; modo_control: string }[];
  sedeID: string;
  onCambiar: (campo: keyof LineaBorrador, valor: string) => void;
  onQuitar?: () => void;
}) {
  const modo = articulos.find((a) => a.id === linea.articuloID)?.modo_control;
  const esUnidad = modo === "UNIDAD";
  const unidades = useUnidades(
    { articulo_id: linea.articuloID, sede_id: sedeID, estado: "DISPONIBLE", limite: 200 },
    esUnidad && linea.articuloID !== "" && sedeID !== "",
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
                label: sedeID === "" ? "elegí la sede primero" : "— elegir unidad —",
              },
              ...disponibles.map((u) => ({ value: u.numero, label: u.numero })),
            ]}
          />
          {sedeID !== "" && disponibles.length === 0 && !unidades.isLoading && (
            <p className="mt-1 text-xs text-negativo">
              No hay unidades disponibles de este artículo en esa sede.
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
