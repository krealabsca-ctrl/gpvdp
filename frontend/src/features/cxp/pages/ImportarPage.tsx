/**
 * CxP — Importador de facturación (/cxp/importar).
 *
 * Acepta DOS formatos, y el backend elige el parser por la FORMA del archivo:
 *
 *   · el .xlsx que sale del script de facturación (el camino de siempre);
 *   · el XML del comprobante electrónico de Hacienda —uno, varios concatenados,
 *     o un .zip de XML—, que es el dato original y trae lo que el Excel pierde:
 *     el tipo de cambio, la condición de venta declarada y el RECEPTOR (a quién
 *     se le facturó).
 *
 * Flujo: subir -> Previsualizar (marca cada fila NUEVA/DUPLICADA por clave, y si
 * su proveedor por cédula ya existe) -> Confirmar -> crea los documentos nuevos y
 * da de alta los proveedores faltantes.
 *
 * El backend re-parsea el archivo en ambos pasos y deduplica por clave (50 díg.),
 * así que reenviar el mismo archivo en "Confirmar" es seguro (no duplica).
 */

import { useMemo, useRef, useState, type ChangeEvent, type DragEvent } from "react";
import { useNavigate } from "react-router-dom";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  PageHeader,
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
import { formatFecha, formatMoneda, type Moneda } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useConfirmarImportacion, usePrevisualizarImportacion } from "@/features/cxp/hooks";
import type { FilaImportada, PreviewImportacion, ResultadoImportacion } from "@/api/cxp";

export function ImportarPage() {
  const toast = useToast();
  const navigate = useNavigate();
  const previsualizar = usePrevisualizarImportacion();
  const confirmar = useConfirmarImportacion();

  const [archivo, setArchivo] = useState<File | null>(null);
  const [preview, setPreview] = useState<PreviewImportacion | null>(null);
  const [resultado, setResultado] = useState<ResultadoImportacion | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  /** Extensiones aceptadas. El backend decide por la forma del archivo; esto solo evita el viaje. */
  const EXTENSIONES = [".xlsx", ".xml", ".zip"];

  function seleccionarArchivo(f: File | undefined | null) {
    if (!f) return;
    const nombre = f.name.toLowerCase();
    if (!EXTENSIONES.some((e) => nombre.endsWith(e))) {
      toast.error("El archivo debe ser el Excel de facturación (.xlsx), un XML de comprobante, o un .zip de XML");
      return;
    }
    setArchivo(f);
    setPreview(null);
    setResultado(null);
  }

  function onDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault();
    setDragOver(false);
    seleccionarArchivo(e.dataTransfer.files?.[0]);
  }

  function onFileChange(e: ChangeEvent<HTMLInputElement>) {
    seleccionarArchivo(e.target.files?.[0]);
  }

  function subir() {
    if (!archivo) {
      toast.error("Seleccioná el Excel de facturación, o los XML de los comprobantes.");
      return;
    }
    previsualizar.mutate(archivo, {
      onSuccess: (res) => {
        setPreview(res);
        setResultado(null);
        toast.info(
          `Preview: ${res.resumen.nuevas} nuevas, ${res.resumen.duplicadas} ya registradas.`,
        );
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  function confirmarImportacion() {
    if (!archivo) return;
    confirmar.mutate(archivo, {
      onSuccess: (res) => {
        setResultado(res);
        setPreview(null);
        toast.success(
          `Importación lista: ${res.creados} documento(s) creado(s)` +
            (res.proveedores_creados > 0 ? `, ${res.proveedores_creados} proveedor(es) nuevo(s).` : "."),
        );
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  function reiniciar() {
    setArchivo(null);
    setPreview(null);
    setResultado(null);
    if (fileInputRef.current) fileInputRef.current.value = "";
  }

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      <PageHeader
        title="Importar facturación"
        description="Subí el Excel de facturación, o directamente los XML de los comprobantes electrónicos (uno, varios, o un .zip). El sistema detecta duplicados por clave y da de alta los proveedores que falten."
      />

      <Card>
        <CardHeader>
          <CardTitle>1. Archivo de facturación</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div
            onDragOver={(e) => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={onDrop}
            className={cn(
              "flex flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed px-4 py-8 text-center transition-colors",
              dragOver ? "border-accent bg-accent/5" : "border-border",
            )}
          >
            <p className="text-sm text-content-muted">Arrastrá el .xlsx, los .xml o un .zip aquí, o</p>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => fileInputRef.current?.click()}
            >
              Elegir archivo
            </Button>
            <input
              ref={fileInputRef}
              type="file"
              accept=".xlsx,.xml,.zip,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/xml,application/xml,application/zip"
              onChange={onFileChange}
              className="sr-only"
              aria-label="Archivo de facturación: Excel, XML o zip de XML"
            />
            {archivo && (
              <p className="mt-1 text-sm font-medium text-content">
                {archivo.name}{" "}
                <span className="text-content-muted">({(archivo.size / 1024).toFixed(0)} KB)</span>
              </p>
            )}
          </div>

          <div className="flex justify-end">
            <Button onClick={subir} loading={previsualizar.isPending} disabled={!archivo}>
              Previsualizar
            </Button>
          </div>
        </CardContent>
      </Card>

      {preview && (
        <>
          <PreviewBlock preview={preview} />
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-surface-raised px-5 py-4">
            <p className="text-sm text-content-muted">
              Se crearán{" "}
              <span className="font-medium text-content">{preview.resumen.nuevas}</span> documento(s)
              nuevo(s). {preview.resumen.duplicadas > 0 && (
                <>Los {preview.resumen.duplicadas} ya registrados se omiten.</>
              )}
            </p>
            <div className="flex items-center gap-2">
              <Button variant="secondary" onClick={reiniciar}>
                Cancelar
              </Button>
              <Button
                onClick={confirmarImportacion}
                loading={confirmar.isPending}
                disabled={preview.resumen.nuevas === 0}
              >
                Confirmar importación
              </Button>
            </div>
          </div>
        </>
      )}

      {resultado && (
        <ResultadoBlock
          resultado={resultado}
          onVerDocumentos={() => navigate("/cxp/documentos")}
          onOtro={reiniciar}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------

function PreviewBlock({ preview }: { preview: PreviewImportacion }) {
  const { resumen, filas } = preview;
  const usd = useMemo(() => filas.filter((f) => f.moneda === "USD").length, [filas]);
  /** El backend solo informa las versiones del esquema cuando lo que se subió fueron XML. */
  const esXml = (resumen.versiones?.length ?? 0) > 0 || filas.some((f) => !!f.tipo_documento);

  const stats = [
    { label: "Leídas", value: resumen.leidas, tone: "neutral" as const },
    { label: "Nuevas", value: resumen.nuevas, tone: "positivo" as const },
    { label: "Ya registradas", value: resumen.duplicadas, tone: "pendiente" as const },
    { label: "Proveedores nuevos", value: resumen.proveedores_nuevos, tone: "accent" as const },
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle>2. Previsualización</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {stats.map((s) => (
            <div key={s.label} className="rounded-md border border-border bg-surface px-3 py-2">
              <p className="text-xs uppercase tracking-wide text-content-muted">{s.label}</p>
              <p className="mt-0.5 text-xl font-semibold tabular-nums text-content">{s.value}</p>
            </div>
          ))}
        </div>

        {/* De dónde salieron las filas. Un archivo del que se leen menos facturas de las que tiene
            era indistinguible de un archivo chico: las filas sin clave se descartaban en silencio.
            Ahora el faltante se puede explicar sin abrir el Excel. */}
        {esXml ? (
          <p className="text-xs text-content-muted">
            Leído de <b className="text-content">{resumen.hoja}</b>:{" "}
            <b className="text-content">{resumen.filas_hoja}</b> comprobante(s) en total, de los que
            se tomaron <b className="text-content">{resumen.leidas}</b> como factura por pagar.
            {resumen.sin_clave > 0 && (
              <>
                {" "}
                Se descartaron <b className="text-content">{resumen.sin_clave}</b> por no traer una
                clave de Hacienda válida (50 dígitos).
              </>
            )}
          </p>
        ) : (
          <p className="text-xs text-content-muted">
            Leído de la hoja <b className="text-content">«{resumen.hoja}»</b>
            {resumen.hojas.length > 1 && <> (el archivo trae {resumen.hojas.length} hojas)</>}: la
            hoja tiene <b className="text-content">{resumen.filas_hoja}</b> fila(s) de datos y se
            leyeron <b className="text-content">{resumen.leidas}</b>.
            {resumen.sin_clave > 0 && (
              <>
                {" "}
                Se descartaron <b className="text-content">{resumen.sin_clave}</b> por no traer clave
                — si esperabas más facturas, ahí está la diferencia.
              </>
            )}
          </p>
        )}

        {/* ── Lo que solo aparece cuando se subieron XML ─────────────────────── */}

        {resumen.versiones && resumen.versiones.length > 0 && (
          <p className="text-xs text-content-muted">
            Esquema de Hacienda leído: <b className="text-content">{resumen.versiones.join(" · ")}</b>
            {resumen.repetidas_en_archivo
              ? ` · ${resumen.repetidas_en_archivo} comprobante(s) venían repetidos en la misma entrega`
              : ""}
            {resumen.xml_ilegibles ? ` · ${resumen.xml_ilegibles} archivo(s) no se pudieron leer` : ""}
          </p>
        )}

        {/* Solo la factura electrónica genera deuda. Lo demás se informa para que nada parezca
            perdido: la nota de crédito RESTA y entraría sumando, y el recibo de pago documenta que
            YA se pagó. */}
        {resumen.descartados && Object.keys(resumen.descartados).length > 0 && (
          <div className="rounded-md border border-border bg-surface-muted px-3 py-2 text-sm text-content-muted">
            <p>
              Se leyeron comprobantes que <b className="text-content">no generan cuenta por pagar</b>{" "}
              y quedaron fuera:
            </p>
            <ul className="mt-1 list-disc pl-5">
              {Object.entries(resumen.descartados).map(([tipo, n]) => (
                <li key={tipo}>
                  <b className="text-content">{n}</b> {tipo}
                </li>
              ))}
            </ul>
          </div>
        )}

        {(resumen.descuadres ?? 0) > 0 && (
          <p className="rounded-md border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-sm text-content">
            En <b>{resumen.descuadres}</b> comprobante(s) la aritmética no cuadra (el subtotal más el
            impuesto no da el total). Se importan con el{" "}
            <b>total del comprobante</b>, que es lo que se le debe al proveedor, pero conviene
            mirarlos: el detalle está en la columna de la derecha de cada fila.
          </p>
        )}

        {(resumen.sin_receptor ?? 0) > 0 && (
          <p className="rounded-md border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-sm text-content">
            <b>{resumen.sin_receptor}</b> comprobante(s) no dicen a quién se les facturó (no traen
            receptor), así que no se puede verificar que sean de esta empresa. Revisalos antes de
            confirmar.
          </p>
        )}

        {resumen.sin_fecha > 0 && (
          <p className="rounded-md border border-negativo/30 bg-negativo/5 px-3 py-2 text-sm text-content">
            {resumen.sin_fecha} fila(s) traen la fecha de emisión ilegible y no se van a importar. Se
            aceptan <span className="font-mono">dd/mm/aaaa</span> y{" "}
            <span className="font-mono">aaaa-mm-dd</span>.
          </p>
        )}

        {/* La fecha se toma de la clave numérica, que la trae en un formato sin ambigüedad. Cuando
            la columna del Excel dice otra cosa, se avisa: el archivo viene mal y eso se arregla en
            el script que lo genera, no acá. */}
        {resumen.fecha_corregida > 0 && (
          <p className="rounded-md border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-sm text-content">
            En <b>{resumen.fecha_corregida}</b> fila(s) la fecha de la columna no coincide con la que
            trae la clave numérica de la factura. Se usó la de la clave, que es la de Hacienda y no
            admite dos lecturas. Conviene revisar el archivo de origen.
          </p>
        )}

        {usd > 0 && (
          <p className="rounded-md border border-border bg-surface-muted px-3 py-2 text-sm text-content-muted">
            {usd} factura(s) en USD: se importan con el{" "}
            <b className="text-content">tipo de cambio de la factura</b>
            {esXml ? " que declara el comprobante" : " (columna «Tipo Cambio» del archivo)"}. Si
            alguna no lo trae, se rechaza y lo dice.
          </p>
        )}

        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Estado</TH>
                <TH>Consecutivo</TH>
                <TH>Proveedor</TH>
                <TH>Cédula</TH>
                <TH>Emisión</TH>
                {esXml && <TH>Receptor</TH>}
                <TH className="text-right">Total</TH>
                {esXml && <TH>Aritmética</TH>}
              </TR>
            </THead>
            <TBody>
              {filas.map((f, i) => (
                <FilaPreview key={`${f.clave}-${i}`} fila={f} esXml={esXml} />
              ))}
            </TBody>
          </Table>
        </TableContainer>
      </CardContent>
    </Card>
  );
}

function FilaPreview({ fila, esXml }: { fila: FilaImportada; esXml: boolean }) {
  const dup = fila.estado === "DUPLICADO";
  return (
    <TR className={cn(dup && "opacity-60")}>
      <TD>
        <Badge tone={dup ? "pendiente" : "positivo"}>{dup ? "Ya registrada" : "Nueva"}</Badge>
      </TD>
      <TD className="font-mono text-xs">{fila.consecutivo || fila.clave.slice(0, 12) + "…"}</TD>
      <TD className="font-medium">
        {fila.proveedor || "—"}
        {fila.proveedor_nuevo && (
          <Badge tone="accent" className="ml-2">
            proveedor nuevo
          </Badge>
        )}
      </TD>
      <TD className="font-mono text-xs tabular-nums">{fila.cedula || "—"}</TD>
      <TD className="tabular-nums">{formatFecha(fila.fecha_emision)}</TD>
      {esXml && (
        <TD className="font-mono text-xs tabular-nums">
          {fila.receptor || <span className="text-negativo">sin receptor</span>}
        </TD>
      )}
      <TD className="text-right tabular-nums">
        {formatMoneda(fila.total, (fila.moneda as Moneda) || "CRC")}
        {fila.moneda === "USD" && (
          <Badge tone="pendiente" className="ml-2">
            USD
          </Badge>
        )}
      </TD>
      {/* La aritmética del comprobante. Existe porque un elemento del XML que no calza devuelve
          CERO sin error: sin esta columna, una factura con IVA 0 por un cambio de esquema pasaría
          desapercibida. Vacío = cuadra. */}
      {esXml && (
        <TD className="text-xs">
          {fila.descuadre ? (
            <span className="text-pendiente" title={fila.descuadre}>
              no cuadra
            </span>
          ) : (
            <span className="text-content-muted">✓</span>
          )}
        </TD>
      )}
    </TR>
  );
}

function ResultadoBlock({
  resultado,
  onVerDocumentos,
  onOtro,
}: {
  resultado: ResultadoImportacion;
  onVerDocumentos: () => void;
  onOtro: () => void;
}) {
  // El backend envía `errores: null` cuando no hubo ninguno (slice nil de Go) → normalizar a [].
  const errores = resultado.errores ?? [];
  const stats = [
    { label: "Documentos creados", value: resultado.creados, tone: "positivo" as const },
    { label: "Omitidos (duplicados)", value: resultado.omitidos_duplicados, tone: "pendiente" as const },
    { label: "Proveedores creados", value: resultado.proveedores_creados, tone: "accent" as const },
    { label: "Con error", value: errores.length, tone: "negativo" as const },
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Resultado de la importación</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {stats.map((s) => (
            <div key={s.label} className="rounded-md border border-border bg-surface px-3 py-2">
              <p className="text-xs uppercase tracking-wide text-content-muted">{s.label}</p>
              <p className="mt-0.5 text-xl font-semibold tabular-nums text-content">{s.value}</p>
            </div>
          ))}
        </div>

        {errores.length > 0 && (
          <div className="rounded-md border border-negativo/30 bg-negativo/5 px-3 py-2">
            <p className="mb-1 text-sm font-medium text-content">Filas no importadas:</p>
            <ul className="max-h-48 list-disc space-y-0.5 overflow-y-auto pl-5 text-xs text-content-muted">
              {errores.map((e, i) => (
                <li key={i} className="break-all">
                  {e}
                </li>
              ))}
            </ul>
          </div>
        )}

        <div className="flex items-center justify-end gap-2">
          <Button variant="secondary" onClick={onOtro}>
            Importar otro archivo
          </Button>
          <Button onClick={onVerDocumentos}>Ver documentos</Button>
        </div>
      </CardContent>
    </Card>
  );
}
