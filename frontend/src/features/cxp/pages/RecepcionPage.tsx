/**
 * CxP — Recepción de facturas (/cxp/recepcion).
 *
 * Lo que llega por correo, y la cola de errores.
 *
 * La pantalla existe para responder tres preguntas, en este orden:
 *
 *   1. ¿está entrando algo?            → el resumen y «última recepción»
 *   2. ¿hay algo trancado?             → el bloque de parqueadas, arriba y en rojo
 *   3. ¿por qué se trancó y qué hago?  → el motivo escrito en palabras + reintentar
 *
 * El motivo NO es un código: es la frase que el servidor escribió al parquear la recepción
 * («el receptor 3004275336 no corresponde a esta empresa»), porque es lo único que le dice a la
 * persona qué arreglar.
 */

import { useMemo, useState } from "react";
import { AlertTriangle, Download, RefreshCw } from "lucide-react";
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
  Select,
  LoadingState,
  PageHeader,
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
  type BadgeTone,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { aFechaHora, formatFechaHora, formatMonto } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useReintentarRecepcion, useRecepciones } from "@/features/cxp/hooks";
import { cxpApi, type EstadoRecepcion, type Recepcion } from "@/api/cxp";
import { VisorComprobante } from "@/features/cxp/components/VisorComprobante";

const TONO: Record<EstadoRecepcion, BadgeTone> = {
  PENDIENTE: "neutral",
  PROCESADA: "positivo",
  DUPLICADA: "accent",
  PARQUEADA: "negativo",
  DESCARTADA: "neutral",
};

/**
 * Los tipos de comprobante de Hacienda, dichos por lo que son y no por su sigla. Un tipo que no
 * esté acá se muestra con su código tal cual: es mejor un «TEE» crudo que esconder el filtro.
 */
const ETIQUETA_TIPO: Record<string, string> = {
  FE: "Factura electrónica",
  NC: "Nota de crédito",
  ND: "Nota de débito",
  TE: "Tiquete electrónico",
  FEC: "Factura de compra",
  FEE: "Factura de exportación",
};

/**
 * El nombre de cada estado dice QUÉ PASÓ, no cómo se llama en la base.
 * «Descartada» sonaría a que se perdió, y no se perdió: el XML se conserva.
 */
const ETIQUETA: Record<EstadoRecepcion, string> = {
  PENDIENTE: "Pendiente",
  PROCESADA: "Registrada",
  DUPLICADA: "Ya estaba",
  PARQUEADA: "Hay que revisarla",
  DESCARTADA: "No genera pago",
};

/**
 * Los 7 tipos que emite Hacienda, en castellano. La raíz del XML («NotaCreditoElectronica») es un
 * identificador técnico: mostrarlo tal cual obliga a la persona a traducirlo de cabeza.
 */
const NOMBRE_TIPO: Record<string, string> = {
  FacturaElectronica: "Factura",
  NotaCreditoElectronica: "Nota de crédito",
  NotaDebitoElectronica: "Nota de débito",
  TiqueteElectronico: "Tiquete",
  FacturaElectronicaCompra: "Factura de compra",
  FacturaElectronicaExportacion: "Factura de exportación",
  ReciboElectronicoPago: "Recibo de pago",
};

const FILTROS: { id: string; label: string }[] = [
  { id: "", label: "Todo" },
  { id: "PARQUEADA", label: "Por revisar" },
  { id: "PROCESADA", label: "Registradas" },
  { id: "DUPLICADA", label: "Ya estaban" },
  { id: "DESCARTADA", label: "Sin pago" },
];

export function RecepcionPage() {
  const toast = useToast();
  const [estado, setEstado] = useState("");
  // La recepción cuyo comprobante se está mirando. null = el visor está cerrado.
  const [viendo, setViendo] = useState<string | null>(null);
  const [q, setQ] = useState("");
  const [busqueda, setBusqueda] = useState("");
  // Las fechas son de LLEGADA: la del documento todavía no se leyó cuando algo no pudo entrar, que
  // es justo el caso que se viene a revisar acá. No se mandan a medio teclear (el selector del
  // navegador dispara en cada tecla del año y emitiría «0002-08-01»).
  const [desde, setDesde] = useState("");
  const [hasta, setHasta] = useState("");
  const [buzon, setBuzon] = useState("");
  const [tipo, setTipo] = useState("");
  const fechaLista = (v: string) => (/^[1-9]\d{3}-\d{2}-\d{2}$/.test(v) ? v : undefined);
  const query = useRecepciones({
    estado: estado || undefined,
    q: busqueda || undefined,
    desde: fechaLista(desde),
    hasta: fechaLista(hasta),
    buzon: buzon || undefined,
    tipo_documento: tipo || undefined,
  });
  const hayFiltros = !!(busqueda || desde || hasta || buzon || tipo);
  const reintentar = useReintentarRecepcion();

  const datos = query.data;
  const parqueadas = datos?.resumen.parqueadas ?? 0;
  const sinCedulas = (datos?.cedulas?.length ?? 0) === 0;

  function reintentarUna(r: Recepcion) {
    reintentar.mutate(r.id, {
      onSuccess: (res) => {
        if (res.estado === "PROCESADA") {
          toast.success(`Registrada: factura ${res.consecutivo || res.clave || ""}.`);
        } else {
          // Un reintento que vuelve a fallar NO es un éxito silencioso: se dice el motivo nuevo.
          toast.info(`Sigue sin entrar: ${res.motivo || "sin motivo"}`);
        }
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Recepción de facturas"
        description="Lo que llega directo del correo de la empresa. Si algo no pudo entrar, queda acá con el motivo."
      />

      {/* SIN CÉDULAS EL GUARDARRAÍL NO PUEDE FUNCIONAR, y eso hay que decirlo fuerte: es lo único
          que verifica que la factura sea de esta empresa y no de otra del grupo. */}
      {datos && sinCedulas && (
        <div className="flex items-start gap-3 rounded-lg border border-negativo/40 bg-negativo/5 px-4 py-3">
          <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-negativo" aria-hidden />
          <div className="text-sm text-content">
            <p className="font-medium">Esta empresa no tiene cédulas jurídicas configuradas.</p>
            <p className="mt-0.5 text-content-muted">
              Sin ellas no se puede verificar que una factura sea de esta empresa, así que la
              recepción por correo rechaza todo lo que llega. Hay que cargarlas antes de poner a
              andar el buzón.
            </p>
          </div>
        </div>
      )}

      {query.isPending && <LoadingState />}
      {query.isError && <ErrorState message={mensajeError(query.error)} onRetry={() => void query.refetch()} />}

      {datos && (
        <>
          <ResumenBloque datos={datos} />

          {/* Filtros. «Por revisar» primero y con el número, porque es lo único que pide acción. */}
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div className="flex flex-wrap gap-2">
              {FILTROS.map((f) => {
                const activo = estado === f.id;
                const cuantas = f.id === "PARQUEADA" ? parqueadas : null;
                return (
                  <button
                    key={f.id || "todo"}
                    type="button"
                    onClick={() => setEstado(f.id)}
                    className={cn(
                      "rounded-lg border px-3 py-1.5 text-sm transition",
                      activo
                        ? "border-accent bg-accent/10 text-content"
                        : "border-border bg-surface text-content-muted hover:bg-surface-muted",
                    )}
                  >
                    {f.label}
                    {cuantas !== null && cuantas > 0 && (
                      <span className="ml-1.5 rounded bg-negativo/15 px-1.5 text-xs font-semibold text-negativo">
                        {cuantas}
                      </span>
                    )}
                  </button>
                );
              })}
            </div>
            <form
              className="flex flex-wrap items-end gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                setBusqueda(q.trim());
              }}
            >
              <Input
                label="Desde"
                type="date"
                value={desde}
                onChange={(e) => setDesde(e.target.value)}
                hint="fecha de llegada"
                className="w-36"
              />
              <Input
                label="Hasta"
                type="date"
                value={hasta}
                onChange={(e) => setHasta(e.target.value)}
                className="w-36"
              />
              {/* Los dos selectores se llenan con lo que REALMENTE llegó: si solo hay un buzón, no
                  tiene sentido ofrecer una lista, y si nunca llegó una nota de crédito tampoco. */}
              {datos.resumen.buzones.length > 1 && (
                <Select
                  label="Buzón"
                  value={buzon}
                  onChange={(e) => setBuzon(e.target.value)}
                  options={[
                    { value: "", label: "Todos" },
                    ...datos.resumen.buzones.map((b) => ({ value: b, label: b })),
                  ]}
                  className="min-w-48"
                />
              )}
              {datos.resumen.tipos.length > 1 && (
                <Select
                  label="Tipo"
                  value={tipo}
                  onChange={(e) => setTipo(e.target.value)}
                  options={[
                    { value: "", label: "Todos" },
                    ...datos.resumen.tipos.map((t) => ({ value: t, label: ETIQUETA_TIPO[t] ?? t })),
                  ]}
                  className="min-w-44"
                />
              )}
              <label className="flex flex-col gap-1 text-xs text-content-muted">
                Buscar
                <Input
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                  placeholder="Clave, proveedor, asunto…"
                  className="w-60"
                />
              </label>
              <Button type="submit" variant="secondary" size="sm">
                Buscar
              </Button>
              {hayFiltros && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setQ("");
                    setBusqueda("");
                    setDesde("");
                    setHasta("");
                    setBuzon("");
                    setTipo("");
                  }}
                >
                  Quitar filtros
                </Button>
              )}
            </form>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>
                {datos.recepciones.length} recepción(es)
                {estado ? ` · ${FILTROS.find((f) => f.id === estado)?.label}` : ""}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {datos.recepciones.length === 0 ? (
                <EmptyState
                  message={
                    estado === "PARQUEADA"
                      ? "Nada trancado: todo lo que llegó se pudo procesar."
                      : "Todavía no ha llegado nada. Cuando el buzón de la empresa reciba una factura electrónica, va a aparecer acá."
                  }
                />
              ) : (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Estado</TH>
                        <TH>Recibido el</TH>
                        <TH>Comprobante</TH>
                        <TH>Proveedor</TH>
                        <TH>Moneda</TH>
                        <TH className="text-right">Total comprobante</TH>
                        <TH className="text-right">Total impuesto</TH>
                        <TH>Qué pasó</TH>
                        <TH>Archivos</TH>
                        <TH />
                      </TR>
                    </THead>
                    <TBody>
                      {datos.recepciones.map((r) => (
                        <Fila
                          key={r.id}
                          r={r}
                          onReintentar={() => reintentarUna(r)}
                          onVer={setViendo}
                          reintentando={reintentar.isPending && reintentar.variables === r.id}
                        />
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              )}
            </CardContent>
          </Card>
        </>
      )}

      {/* El visor: se monta solo cuando hay una recepción elegida, así no pide el XML de fondo. */}
      {viendo && (
        <VisorComprobante
          recepcionId={viendo}
          tienePDF={datos?.recepciones.find((x) => x.id === viendo)?.tiene_pdf}
          onCerrar={() => setViendo(null)}
        />
      )}
    </div>
  );
}

/** El resumen: primero si está entrando algo, después si hay algo trancado. */
function ResumenBloque({ datos }: { datos: NonNullable<ReturnType<typeof useRecepciones>["data"]> }) {
  const { resumen } = datos;
  const total =
    resumen.pendientes + resumen.procesadas + resumen.duplicadas + resumen.parqueadas + resumen.descartadas;

  // «Hace cuánto» y no la fecha cruda: la pregunta es si el buzón sigue vivo, no qué día es.
  const desde = useMemo(() => hace(resumen.ultima_en), [resumen.ultima_en]);

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <Tarjeta
        titulo="Última recepción"
        valor={desde ?? "nunca"}
        nota={total === 0 ? "el buzón todavía no ha entregado nada" : `${total} en total`}
        tono={desde === null ? "neutral" : "accent"}
      />
      <Tarjeta titulo="Registradas" valor={String(resumen.procesadas)} nota="se volvieron cuenta por pagar" tono="positivo" />
      <Tarjeta
        titulo="Por revisar"
        valor={String(resumen.parqueadas)}
        nota={resumen.parqueadas > 0 ? "no entraron: hay que resolverlas" : "nada trancado"}
        tono={resumen.parqueadas > 0 ? "negativo" : "neutral"}
      />
      <Tarjeta
        titulo="Sin pago"
        valor={String(resumen.descartadas + resumen.duplicadas)}
        nota="notas de crédito, recibos y repetidas"
        tono="neutral"
      />
    </div>
  );
}

function Tarjeta({
  titulo,
  valor,
  nota,
  tono,
}: {
  titulo: string;
  valor: string;
  nota: string;
  tono: BadgeTone;
}) {
  const borde =
    tono === "negativo"
      ? "border-negativo/40 bg-negativo/5"
      : tono === "positivo"
        ? "border-positivo/40 bg-positivo/5"
        : tono === "accent"
          ? "border-accent/40 bg-accent/5"
          : "border-border bg-surface";
  return (
    <div className={cn("rounded-lg border px-4 py-3", borde)}>
      <p className="text-xs uppercase tracking-wide text-content-muted">{titulo}</p>
      <p className="mt-0.5 text-2xl font-semibold tabular-nums text-content">{valor}</p>
      <p className="mt-0.5 text-xs text-content-muted">{nota}</p>
    </div>
  );
}

function Fila({
  r,
  onReintentar,
  reintentando,
  onVer,
}: {
  r: Recepcion;
  onReintentar: () => void;
  reintentando: boolean;
  onVer: (id: string) => void;
}) {
  const parqueada = r.estado === "PARQUEADA";
  return (
    <TR className={cn(parqueada && "bg-negativo/5")}>
      <TD>
        <Badge tone={TONO[r.estado]}>{ETIQUETA[r.estado]}</Badge>
        {r.intentos > 1 && (
          <span className="mt-1 block text-xs text-content-muted">{r.intentos} intentos</span>
        )}
      </TD>
      {/* «Recibido el»: pedido del Director. Va con formatFechaHora y NUNCA con new Date():
          el servidor emite el desfase sin minutos («+00») y new Date() devuelve NaN, que se ve
          como celda vacía y no como error. */}
      <TD className="whitespace-nowrap text-xs">
        {formatFechaHora(r.creado_en)}
        <span className="mt-0.5 block text-content-muted">{hace(r.creado_en) ?? ""}</span>
      </TD>
      <TD className="font-mono text-xs">
        {r.consecutivo || (r.clave ? r.clave.slice(-10) : "—")}
        <span className="mt-0.5 block text-content-muted">
          {/* Si aparece un tipo que no está en el mapa —una versión nueva del esquema— se muestra
              su nombre técnico en vez de esconderlo: es el dato con el que se entiende qué llegó. */}
          {r.tipo_documento ? NOMBRE_TIPO[r.tipo_documento] || r.tipo_documento : "—"}
          {r.version_schema ? ` · ${r.version_schema}` : ""}
        </span>
      </TD>
      <TD>
        <span className="block font-medium">{r.proveedor || r.remitente || "—"}</span>
        {r.asunto && <span className="block text-xs text-content-muted">{r.asunto}</span>}
      </TD>
      {/* Moneda en columna PROPIA: así un monto en una moneda que el formateador no conoce nunca
          se pinta con el símbolo de colones. Por eso el total va con formatMonto, sin símbolo. */}
      <TD className="text-xs">{r.moneda || "—"}</TD>
      <TD className="text-right tabular-nums">{r.total ? formatMonto(r.total) : "—"}</TD>
      <TD className="text-right tabular-nums">{r.total_impuesto ? formatMonto(r.total_impuesto) : "—"}</TD>
      <TD className="max-w-sm text-xs text-content">
        {/* El motivo tal como lo escribió el servidor: es lo que dice qué arreglar. */}
        {r.motivo || (r.estado === "PROCESADA" ? "entró sin problemas" : "—")}
      </TD>
      <TD className="text-xs">
        <div className="flex gap-2">
          {/* VER la factura. Solo si conserva el XML: se borra a propósito cuando el comprobante
              no era de esta empresa. */}
          {r.tiene_xml && (
            <Button variant="secondary" size="sm" onClick={() => onVer(r.id)}>
              Ver
            </Button>
          )}
          {r.tiene_xml && <BotonArchivo id={r.id} cual="xml" nombre={nombreArchivo(r)} />}
          {r.tiene_pdf && <BotonArchivo id={r.id} cual="pdf" nombre={nombreArchivo(r)} />}
          {!r.tiene_xml && !r.tiene_pdf && (
            // Pasa a propósito con las facturas de otra empresa: el contenido NO se guarda, para
            // que la cola de errores no se vuelva un repositorio de documentos ajenos.
            <span className="text-content-muted">no se conservan</span>
          )}
        </div>
      </TD>
      <TD>
        {parqueada && r.tiene_xml && (
          <Button variant="secondary" size="sm" onClick={onReintentar} disabled={reintentando}>
            <RefreshCw className={cn("mr-1 h-3.5 w-3.5", reintentando && "animate-spin")} aria-hidden />
            {reintentando ? "Reintentando…" : "Reintentar"}
          </Button>
        )}
      </TD>
    </TR>
  );
}

/**
 * Descarga el original. Se pide como blob y se guarda desde memoria: la ruta exige el Bearer de la
 * sesión, así que un enlace directo daría 401. Mismo patrón que la macro de pagos.
 */
function BotonArchivo({ id, cual, nombre }: { id: string; cual: "xml" | "pdf"; nombre: string }) {
  const toast = useToast();
  const [bajando, setBajando] = useState(false);
  return (
    <button
      type="button"
      disabled={bajando}
      className="inline-flex items-center gap-1 text-accent hover:underline disabled:opacity-50"
      onClick={async () => {
        setBajando(true);
        try {
          const blob = await cxpApi.descargarArchivoRecepcion(id, cual);
          const url = URL.createObjectURL(blob);
          const a = document.createElement("a");
          a.href = url;
          a.download = `${nombre}.${cual}`;
          a.click();
          URL.revokeObjectURL(url);
        } catch (err) {
          toast.error(mensajeError(err));
        } finally {
          setBajando(false);
        }
      }}
    >
      <Download className="h-3.5 w-3.5" aria-hidden />
      {cual.toUpperCase()}
    </button>
  );
}

/** El nombre con el que se guarda el archivo: la clave si la hay, y si no el id. */
function nombreArchivo(r: Recepcion): string {
  return r.clave || r.consecutivo || r.id;
}

/**
 * «hace 3 horas» en vez de la fecha: la pregunta que responde este dato es si el buzón sigue vivo.
 * Devuelve null si nunca hubo recepción.
 */
function hace(iso: string | undefined): string | null {
  // `aFechaHora` y no `new Date`: el backend emite el desfase sin minutos («+00») y eso da NaN,
  // que acá se vería como «nunca» aunque sí hubiera habido recepciones.
  const d = aFechaHora(iso);
  if (!d) return null;
  const min = Math.floor((Date.now() - d.getTime()) / 60000);
  if (min < 2) return "hace un momento";
  if (min < 60) return `hace ${min} min`;
  const h = Math.floor(min / 60);
  if (h < 24) return `hace ${h} h`;
  const dias = Math.floor(h / 24);
  return dias === 1 ? "hace 1 día" : `hace ${dias} días`;
}
