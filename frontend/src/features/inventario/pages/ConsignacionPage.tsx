/**
 * Pantalla — Consignación (/inventario/consignacion).
 *
 * El proveedor deja el cofre en la funeraria y cobra cuando se usa. Mientras está en la bodega, ese
 * capital NO es de la empresa; el día que se usa, nace una cuenta por pagar.
 *
 * La pantalla contesta una sola pregunta y la pone arriba: **qué mercadería del proveedor ya salió
 * de la bodega y todavía no se le facturó.** Ese es el número que cuesta plata cuando nadie lo mira.
 *
 * Tres decisiones de diseño que vienen de reglas del negocio, no de gusto:
 *
 *  1. **Solo el uso ofrece el botón de facturar.** Si la unidad se dañó, se devolvió o el conteo no
 *     la encontró, la fila muestra el caso escrito y NO deja facturar de un clic: ¿le cobra el
 *     proveedor un cofre que se rompió en la bodega? Eso lo decide una persona.
 *  2. **Lo que se genera es una PROVISIÓN.** Cuando llega la factura electrónica real del proveedor,
 *     se concilia: se anula la provisión y la unidad queda enlazada a la de verdad. Sin ese paso
 *     habría dos cuentas por pagar por el mismo cofre.
 *  3. **El estado es derivado**, no un campo que alguien marca. Si mañana se anula esa factura en
 *     CxP, la unidad vuelve sola a la cola.
 */

import { useState } from "react";
import { Link } from "react-router-dom";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  EmptyState,
  ErrorState,
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
import { formatMoneda } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useCandidatasConciliacion,
  useConciliarConsignada,
  useConsignacion,
  useFacturarConsignada,
} from "@/features/inventario/hooks";
import type { ConsignadaSalida } from "@/api/inventario";

export function ConsignacionPage() {
  const tienePermiso = useTienePermiso();
  // Ver la cola es lectura; facturar CREA una cuenta por pagar. Sin este gate, el Auditor Interno
  // —que solo tiene inventario.ver— veía los botones y descubría que no podía apretarlos por el 403.
  const puedeOperar = tienePermiso("inventario.consignacion");

  const [situacion, setSituacion] = useState("");
  const [proveedorID, setProveedorID] = useState("");

  // La cola se pide SIN filtrar y se filtra en el cliente. Con 40-150 unidades el costo es nulo, y
  // resuelve un defecto real: cuando el filtro iba al servidor, las opciones del selector se
  // derivaban de la lista YA filtrada, así que al elegir un proveedor desaparecían los demás y no
  // había forma de volver atrás sin recargar.
  const cola = useConsignacion();
  const data = cola.data;
  const todas = data?.filas ?? [];
  const filas = todas.filter(
    (f) =>
      (situacion === "" || f.situacion === situacion) &&
      (proveedorID === "" || f.proveedor_id === proveedorID),
  );
  // Las opciones salen de TODAS las filas, no de las filtradas. Y de la cola y no del catálogo de
  // 649 proveedores: ofrecer proveedores sin ninguna consignada obliga a buscar entre ruido.
  const proveedores = Array.from(
    new Map(todas.filter((f) => f.proveedor_id).map((f) => [f.proveedor_id, f.proveedor])).entries(),
  ).sort((a, b) => a[1].localeCompare(b[1], "es"));

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Consignación"
        description="Mercadería del proveedor que está en la bodega. Se le paga cuando se usa."
      />

      {data && data.resumen.por_facturar > 0 && (
        <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
          <p className="text-sm font-medium text-content">
            {data.resumen.por_facturar} unidad(es) por {formatMoneda(data.resumen.por_facturar_crc)}{" "}
            salieron de la bodega y no se le facturaron al proveedor
          </p>
          <p className="mt-0.5 text-sm text-content-muted">
            Es plata que se le debe y todavía no está en ninguna cuenta por pagar: no aparece en la
            cartera ni en las proyecciones de tesorería.
          </p>
        </div>
      )}

      {!!data && !data.puede_facturar && (
        <div className="rounded-lg border border-negativo/40 bg-negativo/10 px-4 py-3">
          <p className="text-sm text-content">{data.resumen.aviso}</p>
        </div>
      )}

      {/* ── El resumen: de quién es lo que hay ──────────────────────────── */}
      {data && (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardContent className="pt-6">
              <p className="text-sm text-content-muted">En bodega, del proveedor</p>
              <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                {formatMoneda(data.resumen.en_bodega_crc)}
              </p>
              <p className="mt-0.5 text-xs text-content-muted">
                {data.resumen.en_bodega} unidad(es) · no se le debe nada todavía
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <p className="text-sm text-content-muted">Por facturar</p>
              <p
                className={cn(
                  "mt-1 text-2xl font-semibold tabular-nums",
                  data.resumen.por_facturar > 0 ? "text-negativo" : "text-content",
                )}
              >
                {formatMoneda(data.resumen.por_facturar_crc)}
              </p>
              <p className="mt-0.5 text-xs text-content-muted">
                {data.resumen.por_facturar} unidad(es) usadas o salidas sin pagar
              </p>
            </CardContent>
          </Card>
          {/*
            «A decidir» va SEPARADO de «por facturar» y no sumado. Lo que se dañó, se devolvió o el
            conteo no encontró no es deuda: es un caso que alguien tiene que resolver. Meterlos en el
            mismo total decía que se le debe al proveedor un cofre que se le devolvió.
          */}
          <Card>
            <CardContent className="pt-6">
              <p className="text-sm text-content-muted">A decidir</p>
              <p
                className={cn(
                  "mt-1 text-2xl font-semibold tabular-nums",
                  data.resumen.a_decidir > 0 ? "text-content" : "text-content-muted",
                )}
              >
                {formatMoneda(data.resumen.a_decidir_crc)}
              </p>
              <p className="mt-0.5 text-xs text-content-muted">
                {data.resumen.a_decidir} salieron sin ser un uso · no es deuda todavía
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <p className="text-sm text-content-muted">Ya facturado</p>
              <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                {formatMoneda(data.resumen.facturadas_crc)}
              </p>
              <p className="mt-0.5 text-xs text-content-muted">
                {data.resumen.facturadas} unidad(es) · {data.resumen.proveedores} proveedor(es) ·{" "}
                <Link to="/inventario" className="text-accent underline">
                  existencias
                </Link>
              </p>
            </CardContent>
          </Card>
        </div>
      )}

      {/* ── La cola ─────────────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle>Salió de la bodega</CardTitle>
          <p className="mt-1 text-xs text-content-muted">
            Lo consignado que se usó, se dañó, se devolvió o el conteo no encontró.{" "}
            <strong>Solo el uso</strong> ofrece el botón de facturar: los demás casos son decisiones
            y el sistema no las toma por nadie.
          </p>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-wrap items-start gap-4">
            <div className="w-48">
              <Select
                label="Situación"
                value={situacion}
                onChange={(e) => setSituacion(e.target.value)}
                options={[
                  { value: "", label: "Todo lo que salió" },
                  { value: "PENDIENTE", label: "Sin facturar" },
                  { value: "FACTURADA", label: "Ya facturado" },
                ]}
              />
            </div>
            <div className="w-64">
              <Select
                label="Proveedor"
                value={proveedorID}
                onChange={(e) => setProveedorID(e.target.value)}
                options={[
                  { value: "", label: "Todos" },
                  ...proveedores.map(([id, nombre]) => ({ value: id, label: nombre })),
                ]}
              />
            </div>
          </div>

          {cola.isLoading && <LoadingState label="Buscando lo consignado…" />}
          {cola.isError && (
            <ErrorState message={mensajeError(cola.error)} onRetry={() => cola.refetch()} />
          )}
          {data && filas.length === 0 && (
            <EmptyState message="Nada consignado salió de la bodega. Cuando se use un cofre del proveedor en un servicio, va a aparecer acá con lo que hay que pagarle." />
          )}

          {filas.length > 0 && (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Unidad</TH>
                    <TH>Proveedor</TH>
                    <TH>Salió</TH>
                    <TH>Por qué</TH>
                    <TH className="text-right">Costo</TH>
                    <TH>Cuenta por pagar</TH>
                    <TH>Acción</TH>
                  </TR>
                </THead>
                <TBody>
                  {filas.map((f) => (
                    <FilaConsignada key={f.unidad_id} fila={f} puedeOperar={puedeOperar && (data?.puede_facturar ?? false)} />
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          )}

          {filas.length > 0 && (
            <p className="text-xs text-content-muted">
              El monto de la provisión es el costo con el que la unidad entró a la bodega, sin IVA. El
              IVA lo trae la factura del proveedor cuando llega, y ahí se concilia.
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function FilaConsignada({ fila, puedeOperar }: { fila: ConsignadaSalida; puedeOperar: boolean }) {
  const toast = useToast();
  const facturar = useFacturarConsignada();
  const conciliar = useConciliarConsignada();
  const [dialogo, setDialogo] = useState<"facturar" | "aMano" | "conciliar" | null>(null);
  const [documentoReal, setDocumentoReal] = useState("");

  const yaFacturada = fila.situacion === "FACTURADA";

  // Las candidatas las decide el SERVIDOR, con las mismas cuatro guardas que valida al conciliar:
  // mismo proveedor, no anulada, no la propia provisión, y no usada por otra unidad. Antes la
  // pantalla ofrecía TODAS las facturas del proveedor —incluidas las provisiones que este módulo
  // había generado para otras unidades— y elegir una equivocada terminaba en un rechazo.
  const candidatas = useCandidatasConciliacion(fila.unidad_id, dialogo === "conciliar");
  const opcionesFactura = (candidatas.data?.filas ?? []).map((d) => ({
    value: d.id,
    label: `${d.consecutivo || d.clave.slice(0, 14)} · ${formatMoneda(d.total_crc)} · ${d.fecha}`,
  }));

  return (
    <>
      <TR>
        <TD>
          <span className="font-medium text-content">{fila.articulo}</span>
          <span className="block text-xs text-content-muted">
            {fila.unidad_numero} · {fila.categoria}
          </span>
        </TD>
        <TD>
          <span className="text-sm text-content">{fila.proveedor || "—"}</span>
          {fila.sede && <span className="block text-xs text-content-muted">{fila.sede}</span>}
        </TD>
        <TD>
          <span className="text-sm tabular-nums text-content">{fila.fecha_salida || "—"}</span>
          <span className="block text-xs text-content-muted">
            {fila.servicio_numero
              ? `${fila.servicio_numero}${fila.servicio_a_nombre_de ? ` · ${fila.servicio_a_nombre_de}` : ""}`
              : fila.estado_legible}
          </span>
        </TD>
        <TD>
          <Badge tone={fila.estado === "USADA" ? "neutral" : "negativo"}>{fila.estado_legible}</Badge>
          {fila.aviso && (
            <span className="mt-1 block max-w-xs text-xs text-content-muted">{fila.aviso}</span>
          )}
          {!fila.aviso && fila.motivo_salida && (
            <span className="mt-1 block max-w-xs text-xs text-content-muted">{fila.motivo_salida}</span>
          )}
        </TD>
        <TD className="text-right tabular-nums text-content">{formatMoneda(fila.costo_crc)}</TD>
        <TD>
          {yaFacturada ? (
            <>
              <Badge tone={fila.documento_estado === "ANULADO" ? "negativo" : "positivo"}>
                {fila.documento_consecutivo || fila.documento_estado}
              </Badge>
              <span className="block text-xs text-content-muted">
                {formatMoneda(fila.documento_total_crc)} · {fila.documento_estado.toLowerCase()}
              </span>
            </>
          ) : (
            <Badge tone="pendiente">sin facturar</Badge>
          )}
        </TD>
        <TD>
          {!yaFacturada && fila.puede_facturarse && puedeOperar && (
            <Button size="sm" onClick={() => setDialogo("facturar")} disabled={facturar.isPending}>
              Facturar
            </Button>
          )}
          {/*
            El «a mano» de los casos que no son un uso normal. Existe porque el aviso de la fila
            mandaba a «generá la factura a mano acá» y no había ninguna forma de hacerlo: o faltaba
            la acción o mentía el texto. Lo que el negocio decidió es que estos casos se deciden,
            así que la acción existe pero pide confirmación explícita. La unidad DEVUELTA no lo
            ofrece nunca: lo que volvió al proveedor no se paga.
          */}
          {!yaFacturada && !fila.puede_facturarse && puedeOperar && fila.estado !== "DEVUELTA" && (
            <Button
              size="sm"
              variant="secondary"
              onClick={() => setDialogo("aMano")}
              disabled={facturar.isPending}
            >
              Facturar a mano
            </Button>
          )}
          {fila.puede_conciliarse && puedeOperar && (
            <Button
              size="sm"
              variant="secondary"
              onClick={() => setDialogo("conciliar")}
              disabled={conciliar.isPending}
            >
              Conciliar con la real
            </Button>
          )}
          {yaFacturada && !fila.puede_conciliarse && (
            <span className="text-xs text-content-muted">conciliada</span>
          )}
          {!puedeOperar && <span className="text-xs text-content-muted">solo lectura</span>}
          {puedeOperar && !yaFacturada && fila.estado === "DEVUELTA" && (
            <span className="text-xs text-content-muted">no se paga</span>
          )}
        </TD>
      </TR>

      {dialogo === "facturar" && (
        <ConfirmDialog
          titulo={`Facturar ${fila.unidad_numero} a ${fila.proveedor}`}
          descripcion={
            <>
              Se va a crear una <strong>provisión</strong> de cuenta por pagar por el costo con el que
              esta unidad entró a la bodega, sin IVA. Cuando llegue la factura electrónica del
              proveedor hay que volver acá y conciliarla, para no terminar con dos documentos por el
              mismo cofre.
            </>
          }
          impacto={[`Se compromete: ${formatMoneda(fila.costo_crc)}`]}
          textoConfirmar="Crear la provisión"
          pendiente={facturar.isPending}
          onConfirmar={() =>
            facturar.mutate({ unidadId: fila.unidad_id }, {
              onSuccess: (u) => {
                toast.success(
                  `Provisión ${u.documento_consecutivo || ""} creada por ${formatMoneda(u.documento_total_crc)}. Queda en la bandeja de CxP para conciliar con la factura del proveedor.`,
                );
                setDialogo(null);
              },
              onError: (err) => {
                toast.error(mensajeError(err));
                setDialogo(null);
              },
            })
          }
          onCancelar={() => setDialogo(null)}
        />
      )}

      {dialogo === "aMano" && (
        <ConfirmDialog
          titulo={`Facturar a mano ${fila.unidad_numero}`}
          descripcion={
            <>
              Esta unidad <strong>no salió por un servicio</strong>: {fila.estado_legible}. El sistema
              no decide solo si al proveedor le corresponde cobrarla, así que lo estás confirmando
              vos. Si el proveedor asume el caso, no generés nada.
              {fila.motivo_salida && (
                <span className="mt-2 block text-content-muted">
                  Lo que se anotó al sacarla: «{fila.motivo_salida}»
                </span>
              )}
            </>
          }
          impacto={[`Se compromete: ${formatMoneda(fila.costo_crc)}`, "Queda registrado quién lo confirmó"]}
          textoConfirmar="Sí, corresponde pagarla"
          tono="peligro"
          pendiente={facturar.isPending}
          onConfirmar={() =>
            facturar.mutate(
              { unidadId: fila.unidad_id, confirmar: true },
              {
                onSuccess: (u) => {
                  toast.success(
                    `Provisión ${u.documento_consecutivo || ""} creada por ${formatMoneda(u.documento_total_crc)}.`,
                  );
                  setDialogo(null);
                },
                onError: (err) => {
                  toast.error(mensajeError(err));
                  setDialogo(null);
                },
              },
            )
          }
          onCancelar={() => setDialogo(null)}
        />
      )}

      {dialogo === "conciliar" && (
        <ConfirmDialog
          titulo={`Conciliar ${fila.unidad_numero} con la factura del proveedor`}
          descripcion={
            <>
              Se <strong>anula la provisión</strong> {fila.documento_consecutivo} y la unidad queda
              enlazada a la factura real. Elegí cuál de las facturas de{" "}
              <strong>{fila.proveedor}</strong> corresponde a esta unidad.
              <span className="mt-3 block">
                <Select
                  label="Factura del proveedor"
                  value={documentoReal}
                  onChange={(e) => setDocumentoReal(e.target.value)}
                  options={[
                    { value: "", label: candidatas.isLoading ? "Buscando…" : "— elegir —" },
                    ...opcionesFactura,
                  ]}
                />
              </span>
              {/* El aviso viene del servidor: dice si no hay ninguna, o cuántas quedaron afuera del
                  tope. Un tope que no avisa esconde justo la factura que se está buscando. */}
              {candidatas.data?.aviso && (
                <span className="mt-2 block text-xs text-content-muted">{candidatas.data.aviso}</span>
              )}
            </>
          }
          impacto={[
            `Se anula la provisión de ${formatMoneda(fila.documento_total_crc)}`,
            "El monto que se pague va a ser el de la factura real, no el de la provisión",
          ]}
          textoConfirmar="Anular la provisión y enlazar"
          tono="peligro"
          pendiente={conciliar.isPending}
          onConfirmar={() => {
            if (documentoReal.trim() === "") {
              toast.error("Pegá el identificador de la factura del proveedor.");
              return;
            }
            conciliar.mutate(
              { unidadId: fila.unidad_id, documentoId: documentoReal.trim() },
              {
                onSuccess: (u) => {
                  toast.success(
                    `La provisión quedó anulada y ${fila.unidad_numero} está enlazada a la factura real por ${formatMoneda(u.documento_total_crc)}.`,
                  );
                  setDialogo(null);
                  setDocumentoReal("");
                },
                onError: (err) => toast.error(mensajeError(err)),
              },
            );
          }}
          onCancelar={() => setDialogo(null)}
        />
      )}
    </>
  );
}
