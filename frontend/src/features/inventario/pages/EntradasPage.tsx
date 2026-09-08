/**
 * Pantalla — Entrada de mercadería (/inventario/entradas).
 *
 * Una entrada por remesa. Dos detalles que importan:
 *
 *  · Si el artículo se controla **por unidad**, el sistema crea una ficha por objeto. Los números se
 *    pueden escribir (si el proveedor los trae en placa o etiqueta) o dejar que los genere: hay
 *    funerarias que numeran y otras que no, y forzar una sola forma obliga a inventar datos.
 *  · **Consignada** cambia de quién es el capital. Se marca al entrar, no después: reclasificarlo más
 *    tarde obliga a decidir hacia atrás de quién era cada unidad.
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
import { formatMonto, montoParaApi } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useSedes } from "@/features/bancos/hooks";
import { useTodosProveedores } from "@/features/cxp/hooks";
import { useArticulos, useMovimientos, useRegistrarEntrada } from "@/features/inventario/hooks";

/** El día de hoy en Costa Rica, para que la fecha por defecto no venga corrida por la zona. */
function hoyCR(): string {
  const d = new Date();
  const cr = new Date(d.getTime() - (d.getTimezoneOffset() + 360) * 60_000);
  return cr.toISOString().slice(0, 10);
}

export function EntradasPage() {
  const toast = useToast();
  const sedes = useSedes();
  const articulos = useArticulos();
  const registrar = useRegistrarEntrada();
  const ultimas = useMovimientos({ tipo: "ENTRADA", limite: 25 });
  const proveedores = useTodosProveedores();

  const [articuloID, setArticuloID] = useState("");
  const [sedeID, setSedeID] = useState("");
  const [fecha, setFecha] = useState(hoyCR());
  const [cantidad, setCantidad] = useState("1");
  const [costo, setCosto] = useState("");
  const [consignada, setConsignada] = useState(false);
  const [proveedorID, setProveedorID] = useState("");
  const [numeros, setNumeros] = useState("");
  const [nota, setNota] = useState("");

  const articulo = (articulos.data ?? []).find((a) => a.id === articuloID);
  const esPorUnidad = articulo?.modo_control === "UNIDAD";
  const nCantidad = Number(cantidad) || 0;

  /**
   * Cambiar de artículo limpia lo que solo aplica a los de unidad.
   *
   * Sin esto quedaba pegado: se marcaba «consignada» en un cofre, se cambiaba a una urna por
   * cantidad —donde el checkbox ya no se dibuja—, y la entrada se mandaba igual con
   * es_consignada=true. El servidor la rechazaba con un 422 correcto, pero el usuario no tenía forma
   * de desmarcar lo que no veía: quedaba trabado sin entender por qué.
   */
  function elegirArticulo(id: string) {
    setArticuloID(id);
    const nuevo = (articulos.data ?? []).find((a) => a.id === id);
    if (nuevo?.modo_control !== "UNIDAD") {
      setConsignada(false);
      setProveedorID("");
      setNumeros("");
    }
  }

  function enviar() {
    if (!articuloID || !sedeID) {
      toast.error("Elegí el artículo y la sede que recibe.");
      return;
    }
    if (nCantidad <= 0) {
      toast.error("La cantidad tiene que ser mayor que cero.");
      return;
    }
    if (costo.trim() === "") {
      toast.error("Escribí el costo por unidad.");
      return;
    }
    // Se avisa acá y no después del 422: el backend igual lo rechaza, pero pedirlo antes de mandar
    // evita que el usuario pierda lo que ya escribió.
    if (consignada && proveedorID === "") {
      toast.error("Elegí de qué proveedor es la mercadería consignada: es a quien se le va a pagar.");
      return;
    }
    const lista = numeros
      .split(/[\s,;]+/)
      .map((n) => n.trim())
      .filter((n) => n !== "");
    registrar.mutate(
      {
        articulo_id: articuloID,
        sede_id: sedeID,
        fecha,
        cantidad: nCantidad,
        costo_unitario: montoParaApi(costo),
        es_consignada: consignada,
        ...(consignada ? { proveedor_id: proveedorID } : {}),
        numeros: lista,
        nota: nota.trim(),
      },
      {
        onSuccess: (r) => {
          toast.success(
            r.numeros_creados.length > 0
              ? `Entraron ${r.cantidad}: ${r.numeros_creados.join(", ")}.`
              : `Entraron ${r.cantidad} unidad(es).`,
          );
          setCantidad("1");
          setNumeros("");
          setNota("");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  const movs = ultimas.data ?? [];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Entrada de mercadería"
        description="Lo que llegó del proveedor, con su costo y a qué sede."
      />

      <Card>
        <CardHeader>
          <CardTitle>Registrar la entrada</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Select
              label="Artículo *"
              value={articuloID}
              onChange={(e) => elegirArticulo(e.target.value)}
              options={[
                { value: "", label: "— elegir —" },
                ...(articulos.data ?? []).map((a) => ({
                  value: a.id,
                  label: `${a.nombre} · ${a.categoria}`,
                })),
              ]}
            />
            <Select
              label="Sede que recibe *"
              value={sedeID}
              onChange={(e) => setSedeID(e.target.value)}
              options={[
                { value: "", label: "— elegir —" },
                ...(sedes.data ?? []).map((s) => ({ value: s.id, label: s.nombre })),
              ]}
            />
            <Input label="Fecha *" type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} />
            <Input
              label="Cantidad *"
              value={cantidad}
              onChange={(e) => setCantidad(e.target.value)}
              inputMode="numeric"
            />
            <Input
              label="Costo por unidad *"
              value={costo}
              onChange={(e) => setCosto(e.target.value)}
              inputMode="decimal"
              placeholder="423 750"
            />
            <Input
              label="Nota"
              value={nota}
              onChange={(e) => setNota(e.target.value)}
              placeholder="Factura, remisión…"
            />
          </div>

          {esPorUnidad && (
            <div className="rounded-lg border border-border bg-surface-muted px-4 py-3">
              <p className="text-sm font-medium text-content">
                Este artículo se controla por unidad
              </p>
              <p className="mt-0.5 text-sm text-content-muted">
                Se van a crear {nCantidad} ficha(s), una por objeto físico. Si el proveedor ya los
                trae numerados, escribilos separados por coma; los que falten los genera el sistema.
              </p>
              <div className="mt-3 max-w-xl">
                <Input
                  label="Números de las unidades"
                  value={numeros}
                  onChange={(e) => setNumeros(e.target.value)}
                  placeholder="CF-0412, CF-0413"
                />
              </div>
            </div>
          )}

          {/*
            La consignación se lleva por UNIDAD y solo se ofrece ahí. Un artículo por cantidad no
            crea fichas, así que no habría dónde guardar de quién es cada objeto: antes el checkbox
            aparecía siempre y el dato se descartaba en silencio, con el usuario creyendo que había
            quedado marcado.
          */}
          {esPorUnidad ? (
            <div className="flex flex-col gap-3">
              <label className="flex w-fit items-center gap-2 text-sm text-content">
                <input
                  type="checkbox"
                  checked={consignada}
                  onChange={(e) => setConsignada(e.target.checked)}
                  className="h-4 w-4 rounded border-border accent-accent"
                />
                Es mercadería consignada — el capital sigue siendo del proveedor hasta que se use
              </label>

              {consignada && (
                <div className="rounded-lg border border-accent/40 bg-accent/5 px-4 py-3">
                  <div className="w-full sm:w-96">
                    <Select
                      label="Proveedor dueño de la mercadería *"
                      value={proveedorID}
                      onChange={(e) => setProveedorID(e.target.value)}
                      options={[
                        { value: "", label: "— elegir —" },
                        ...(proveedores.data ?? []).map((p) => ({ value: p.id, label: p.nombre })),
                      ]}
                    />
                  </div>
                  <p className="mt-2 text-xs text-content-muted">
                    Es a quien se le va a facturar cuando la unidad se use en un servicio. Sin
                    proveedor la cuenta por pagar no tendría destinatario, así que es obligatorio.
                  </p>
                </div>
              )}
            </div>
          ) : (
            articuloID !== "" && (
              <p className="text-xs text-content-muted">
                Este artículo se lleva por cantidad, así que no puede marcarse como consignado: la
                consignación necesita una ficha por objeto para saber de quién es cada uno.
              </p>
            )
          )}

          <div className="flex flex-wrap items-center gap-3">
            <Button onClick={enviar} loading={registrar.isPending}>
              Registrar entrada
            </Button>
            <span className="text-xs text-content-muted">
              La entrada suma a las existencias de la sede al instante.
            </span>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Últimas entradas</CardTitle>
        </CardHeader>
        <CardContent>
          {ultimas.isLoading && <LoadingState label="Buscando las últimas entradas…" />}
          {ultimas.isError && (
            <ErrorState message={mensajeError(ultimas.error)} onRetry={() => ultimas.refetch()} />
          )}
          {ultimas.data && movs.length === 0 && (
            <EmptyState message="Todavía no se registró ninguna entrada." />
          )}
          {movs.length > 0 && (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Fecha</TH>
                    <TH>Artículo</TH>
                    <TH>Unidad</TH>
                    <TH>Sede</TH>
                    <TH className="text-right">Cant.</TH>
                    <TH className="text-right">Costo unit.</TH>
                    <TH>Quién</TH>
                  </TR>
                </THead>
                <TBody>
                  {movs.map((m) => (
                    <TR key={m.id}>
                      <TD className="tabular-nums text-content-muted">{m.fecha}</TD>
                      <TD className="font-medium text-content">{m.articulo}</TD>
                      <TD className="tabular-nums text-content-muted">{m.unidad_numero || "—"}</TD>
                      <TD className="text-content-muted">{m.sede}</TD>
                      <TD className="text-right tabular-nums">{m.cantidad}</TD>
                      <TD className="text-right tabular-nums">
                        {formatMonto(m.costo_unitario_crc)}
                      </TD>
                      <TD className="text-content-muted">{m.usuario || "—"}</TD>
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
