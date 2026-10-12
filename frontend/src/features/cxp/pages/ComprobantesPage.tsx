/**
 * Los recibos del banco de una corrida entera, de un solo viaje.
 *
 * El problema que resuelve, en palabras del Director Financiero (7-oct-2026): «para identificar
 * cada pago es lento, hay que buscar uno a uno y validar si es el correcto; si tengo 100 pagos y
 * 100 recibos, ¿cómo automatizo?». Adjuntar de a uno existía desde la migración 0015 y se usó
 * SIETE veces en toda la vida del sistema — con archivos de prueba. Una función que nadie puede
 * usar cien veces seguidas es una función que no existe.
 *
 * El cruce no lo hace una persona: el código de pago ya viaja en el nombre del archivo, porque es
 * la misma descripción que se le puso a la macro. Se leen los nombres, se resuelven contra la
 * cartera y cada recibo se pega solo a su factura.
 *
 * DOS PASOS, como el resto de los importadores: primero se ven los NOMBRES —que son unos
 * kilobytes— y recién al confirmar se suben los archivos. Así quien bautizó mal un archivo lo
 * corrige antes de esperar treinta megas de subida.
 *
 * Y la mitad que casi siempre falta en una función así: EL CONTROL INVERSO. «Adjunté 87» no
 * contesta la pregunta que importa, que es cuáles de los cien faltan.
 */
import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  ErrorState,
  LoadingState,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import {
  cxpApi,
  type ArchivoCruzado,
  type EstadoArchivoCarpeta,
  type PagoConRecibo,
  type PreviewCarpeta,
} from "@/api/cxp";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";
import { formatMonto } from "@/lib/format";
import { copiarAlPortapapeles } from "@/lib/portapapeles";
import { cn } from "@/lib/cn";

/**
 * Qué significa cada veredicto, en palabras de quien lee la pantalla.
 *
 * Cada uno dice QUÉ HACER, no solo qué pasó: un estado que describe el problema sin indicar la
 * salida deja al operador con cien archivos y ninguna acción.
 */
export const VEREDICTO: Record<
  EstadoArchivoCarpeta,
  { texto: string; tono: "positivo" | "pendiente" | "negativo" | "neutral"; queHacer: string }
> = {
  UNICO: {
    texto: "calza",
    tono: "positivo",
    queHacer: "Se va a adjuntar a esa factura.",
  },
  REMESA: {
    texto: "pago agrupado",
    tono: "positivo",
    queHacer: "Es un pago que cubre varias facturas del mismo proveedor: el recibo queda en todas.",
  },
  SIN_CODIGO: {
    texto: "sin código",
    tono: "neutral",
    queHacer: "Renombrá el archivo incluyendo el código del pago. Abajo está la lista con el nombre de cada uno.",
  },
  VARIOS_CODIGOS: {
    texto: "dos pagos en el nombre",
    tono: "pendiente",
    queHacer: "El nombre menciona más de un pago y no hay cómo saber de cuál es. Dejá un solo código.",
  },
  DUPLICADO: {
    texto: "repetido",
    tono: "pendiente",
    queHacer: "Hay dos archivos para el mismo pago. No se adjunta ninguno: borrá el que sobra y volvé a subir.",
  },
  NO_ENCONTRADO: {
    texto: "no existe acá",
    tono: "negativo",
    queHacer: "Ese código no corresponde a ninguna factura de esta empresa. ¿Es de otra empresa del grupo?",
  },
  NO_PAGADA: {
    texto: "todavía no pagada",
    tono: "pendiente",
    queHacer: "La factura existe pero no está marcada como pagada. Marcala primero y volvé a subir el recibo.",
  },
};

/**
 * El texto de un veredicto, con salida para el que no esté en el mapa.
 *
 * Sin esto, un estado que el servidor agregue y acá falte devuelve `undefined` y la página entera
 * revienta al leer `.tono` — cien archivos y una pantalla en blanco. El fallback muestra el estado
 * crudo, que es feo pero legible, y manda a mirar el archivo. Hay una prueba que compara esta lista
 * contra la del backend para que el fallback no llegue a usarse nunca.
 */
function veredicto(estado: EstadoArchivoCarpeta) {
  return (
    VEREDICTO[estado] ?? {
      texto: estado,
      tono: "pendiente" as const,
      queHacer: "No se pudo asociar. Revisá el archivo a mano.",
    }
  );
}

export function ComprobantesPage() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();

  const [archivos, setArchivos] = useState<File[]>([]);
  const [previa, setPrevia] = useState<PreviewCarpeta | null>(null);
  const [soloFaltantes, setSoloFaltantes] = useState(true);
  const input = useRef<HTMLInputElement>(null);

  const pagosQ = useQuery({
    queryKey: ["cxp", empresaId, "comprobantes-pagos", soloFaltantes],
    queryFn: () => cxpApi.pagosConRecibo({ soloFaltantes }),
  });

  const previsualizar = useMutation({
    mutationFn: (fs: File[]) => cxpApi.previsualizarCarpeta(fs.map((f) => f.name)),
    onSuccess: setPrevia,
    onError: (e) => toast.error(mensajeError(e)),
  });

  const adjuntar = useMutation({
    mutationFn: (fs: File[]) => cxpApi.adjuntarCarpeta(fs),
    onSuccess: (r) => {
      // El conteo que se canta es el de FACTURAS, no el de archivos: es el que se compara contra
      // lo que falta. Y los rechazados se nombran, porque un «87 listos» que fueron 85 deja dos
      // pagos sin documentar que nadie vuelve a mirar.
      toast.success(
        `${r.facturas} ${r.facturas === 1 ? "factura quedó documentada" : "facturas quedaron documentadas"}` +
          (r.rechazados.length > 0 ? ` · ${r.rechazados.length} no se pudieron` : ""),
      );
      setArchivos([]);
      setPrevia(null);
      if (input.current) input.current.value = "";
      void qc.invalidateQueries({ queryKey: ["cxp", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  function elegir(lista: FileList | null) {
    const fs = Array.from(lista ?? []);
    setArchivos(fs);
    setPrevia(null);
    if (fs.length > 0) previsualizar.mutate(fs);
  }

  /** Los archivos que de verdad se van a subir: los demás no tienen a dónde ir. */
  const subibles = useMemo(() => {
    if (!previa) return [];
    const ok = new Set(
      previa.archivos.filter((a) => a.estado === "UNICO" || a.estado === "REMESA").map((a) => a.nombre),
    );
    return archivos.filter((f) => ok.has(f.name));
  }, [previa, archivos]);

  const pagos = pagosQ.data;

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">Comprobantes de pago</h1>
        <p className="text-sm text-muted-foreground">
          Soltá acá la carpeta de recibos de una corrida. Cada uno se pega solo a su factura leyendo
          el código del nombre.
        </p>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>Subir la carpeta</CardTitle>
          <CardDescription>
            El archivo tiene que llevar el código del pago en el nombre —<code>CXP-…</code> o{" "}
            <code>CXPR-…</code>—, que es el mismo que le pusiste a la macro.{" "}
            <strong>Con guion o sin él, en mayúscula o minúscula, y no importa qué más diga el nombre.</strong>
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <input
            ref={input}
            type="file"
            multiple
            onChange={(e) => elegir(e.target.files)}
            className="block w-full text-sm file:mr-3 file:rounded-md file:border-0 file:bg-primary file:px-3 file:py-2 file:text-sm file:text-primary-foreground"
          />

          {previsualizar.isPending && <LoadingState label="Viendo a qué pago corresponde cada uno…" />}

          {previa && (
            <>
              <div className="flex flex-wrap gap-3 text-sm">
                <Resumen n={previa.resumen.facturas} label="facturas quedarían documentadas" tono="positivo" />
                <Resumen n={previa.resumen.se_adjuntan} label="archivos se usan" tono="neutral" />
                {previa.resumen.reemplazan > 0 && (
                  <Resumen
                    n={previa.resumen.reemplazan}
                    label="reemplazan uno que ya estaba"
                    tono="pendiente"
                  />
                )}
                {previa.resumen.problemas > 0 && (
                  <Resumen n={previa.resumen.problemas} label="quedan afuera" tono="negativo" />
                )}
              </div>

              {previa.resumen.reemplazan > 0 && (
                <p className="rounded-md bg-pendiente/10 px-3 py-2 text-sm text-pendiente">
                  Hay {previa.resumen.reemplazan} factura(s) que ya tenían recibo. Al reemplazarlo se
                  borra la marca de «comprobante enviado al proveedor», porque el que recibió ya no es
                  el que está adjunto.
                </p>
              )}

              <TablaCruce archivos={previa.archivos} />

              <div className="flex items-center gap-3">
                <Button
                  onClick={() => adjuntar.mutate(subibles)}
                  disabled={subibles.length === 0 || adjuntar.isPending}
                >
                  {adjuntar.isPending
                    ? "Subiendo…"
                    : `Adjuntar ${subibles.length} archivo${subibles.length === 1 ? "" : "s"}`}
                </Button>
                <Button
                  variant="ghost"
                  onClick={() => {
                    setArchivos([]);
                    setPrevia(null);
                    if (input.current) input.current.value = "";
                  }}
                >
                  Cancelar
                </Button>
                {subibles.length === 0 && (
                  <span className="text-sm text-muted-foreground">
                    Ninguno se puede asociar todavía. Mirá la columna «qué hacer».
                  </span>
                )}
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="flex items-start justify-between gap-4">
            <div>
              <CardTitle>
                {soloFaltantes ? "Pagos sin recibo" : "Todos los pagos"}
                {pagos && (
                  <span className="ml-2 text-base font-normal text-muted-foreground">
                    {soloFaltantes ? pagos.faltan : pagos.transferencias} de {pagos.transferencias}{" "}
                    transferencias
                  </span>
                )}
              </CardTitle>
              <CardDescription>
                Una fila por <strong>transferencia</strong>, que es lo que corresponde uno a uno con un
                archivo bajado del banco: un pago agrupado cuenta una sola vez aunque cubra cinco
                facturas.
              </CardDescription>
            </div>
            <Button variant="ghost" size="sm" onClick={() => setSoloFaltantes((v) => !v)}>
              {soloFaltantes ? "Ver todos" : "Ver solo los que faltan"}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {pagosQ.isLoading && <LoadingState label="Cargando los pagos…" />}
          {pagosQ.isError && <ErrorState message={mensajeError(pagosQ.error)} />}
          {pagos && pagos.pagos.length === 0 && (
            <p className="py-6 text-center text-sm text-positivo">
              {soloFaltantes
                ? "Todos los pagos tienen su recibo adjunto."
                : "Todavía no hay pagos registrados."}
            </p>
          )}
          {pagos && pagos.pagos.length > 0 && <TablaPagos pagos={pagos.pagos} />}
        </CardContent>
      </Card>
    </div>
  );
}

function Resumen(props: { n: number; label: string; tono: "positivo" | "pendiente" | "negativo" | "neutral" }) {
  const color = {
    positivo: "text-positivo",
    pendiente: "text-pendiente",
    negativo: "text-negativo",
    neutral: "text-foreground",
  }[props.tono];
  return (
    <span className="rounded-md border px-3 py-1.5">
      <strong className={cn("text-base", color)}>{props.n}</strong>{" "}
      <span className="text-muted-foreground">{props.label}</span>
    </span>
  );
}

function TablaCruce({ archivos }: { archivos: ArchivoCruzado[] }) {
  // Los problemas primero: son los que piden una acción. Lo que calza no hay que mirarlo.
  const ordenados = useMemo(() => {
    const peso = (a: ArchivoCruzado) => (a.estado === "UNICO" || a.estado === "REMESA" ? 1 : 0);
    return [...archivos].sort((a, b) => peso(a) - peso(b) || a.nombre.localeCompare(b.nombre));
  }, [archivos]);

  return (
    <TableContainer>
      <Table>
        <THead>
          <TR>
            <TH>Archivo</TH>
            <TH>Qué es</TH>
            <TH>A qué factura</TH>
            <TH>Qué hacer</TH>
          </TR>
        </THead>
        <TBody>
          {ordenados.map((a) => {
            const v = veredicto(a.estado);
            return (
              <TR key={a.nombre}>
                <TD className="max-w-[22rem] truncate font-mono text-xs" title={a.nombre}>
                  {a.nombre}
                </TD>
                <TD>
                  <Badge tone={v.tono}>{v.texto}</Badge>
                  {a.codigo && <div className="mt-1 font-mono text-xs text-muted-foreground">{a.codigo}</div>}
                </TD>
                <TD>
                  {a.destinos.length === 0 ? (
                    <span className="text-muted-foreground">—</span>
                  ) : (
                    <div className="space-y-0.5 text-xs">
                      {a.destinos.map((d) => (
                        <div key={d.documento_id}>
                          <span className="font-medium">{d.proveedor}</span>
                          {d.consecutivo && <span className="text-muted-foreground"> · {d.consecutivo}</span>}
                          {d.ya_tiene && <span className="ml-1 text-pendiente">(reemplaza)</span>}
                        </div>
                      ))}
                    </div>
                  )}
                </TD>
                <TD className="text-xs text-muted-foreground">{a.motivo ? `${a.motivo}. ` : ""}{v.queHacer}</TD>
              </TR>
            );
          })}
        </TBody>
      </Table>
    </TableContainer>
  );
}

function TablaPagos({ pagos }: { pagos: PagoConRecibo[] }) {
  const toast = useToast();

  async function copiarNombres() {
    const faltan = pagos.filter((p) => p.con_recibo < p.facturas);
    const ok = await copiarAlPortapapeles(faltan.map((p) => p.nombre_sugerido).join("\n"));
    toast[ok ? "success" : "error"](
      ok ? `${faltan.length} nombres copiados` : "No se pudo copiar",
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          El nombre sugerido es una ayuda, no un requisito: lo único que el lector necesita es que el
          código aparezca en alguna parte del nombre.
        </p>
        <Button variant="ghost" size="sm" onClick={() => void copiarNombres()}>
          Copiar los nombres que faltan
        </Button>
      </div>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Referencia</TH>
              <TH>Proveedor</TH>
              <TH className="text-right">Monto</TH>
              <TH>Recibo</TH>
              <TH>Nombre sugerido del archivo</TH>
            </TR>
          </THead>
          <TBody>
            {pagos.map((p) => (
              <TR key={p.codigo}>
                <TD>
                  <div className="font-medium">{p.referencia}</div>
                  <div className="font-mono text-xs text-muted-foreground">{p.codigo}</div>
                </TD>
                <TD>
                  {p.proveedor}
                  {p.es_remesa && (
                    <div className="text-xs text-muted-foreground">{p.facturas} facturas en un pago</div>
                  )}
                </TD>
                <TD className="text-right tabular-nums">{formatMonto(p.monto_crc)}</TD>
                <TD>
                  {p.con_recibo === 0 ? (
                    <Badge tone="negativo">falta</Badge>
                  ) : p.con_recibo < p.facturas ? (
                    <Badge tone="pendiente">
                      {p.con_recibo} de {p.facturas}
                    </Badge>
                  ) : (
                    <Badge tone="positivo">completo</Badge>
                  )}
                </TD>
                <TD>
                  <button
                    type="button"
                    onClick={() => {
                      void copiarAlPortapapeles(p.nombre_sugerido).then((ok) => {
                        if (ok) toast.success("Nombre copiado");
                      });
                    }}
                    className="font-mono text-xs text-muted-foreground underline-offset-2 hover:underline"
                    title="Copiar"
                  >
                    {p.nombre_sugerido}
                  </button>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
    </div>
  );
}
