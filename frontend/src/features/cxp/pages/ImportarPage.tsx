/**
 * CxP — Importador de facturación (/cxp/importar).
 *
 * Flujo: subir el .xlsx de facturación electrónica -> Previsualizar (marca cada
 * fila NUEVA/DUPLICADA por clave, y si su proveedor por cédula ya existe) ->
 * Confirmar -> crea los documentos nuevos y da de alta los proveedores faltantes.
 *
 * El backend re-parsea el archivo en ambos pasos y deduplica por clave (50 díg.),
 * así que reenviar el mismo archivo en "Confirmar" es seguro (no duplica).
 *
 * Facturas en USD: no se importan (el Excel no trae tipo de cambio). Se cuentan
 * aparte y quedan para carga manual con su TC.
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

  function seleccionarArchivo(f: File | undefined | null) {
    if (!f) return;
    if (!f.name.toLowerCase().endsWith(".xlsx")) {
      toast.error("El archivo debe ser un Excel .xlsx");
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
      toast.error("Seleccioná el archivo .xlsx de facturación.");
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
        description="Subí el Excel de facturación electrónica. El sistema detecta duplicados por clave y da de alta los proveedores que falten."
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
            <p className="text-sm text-content-muted">Arrastrá el archivo .xlsx aquí, o</p>
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
              accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
              onChange={onFileChange}
              className="sr-only"
              aria-label="Archivo Excel de facturación"
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

        {usd > 0 && (
          <p className="rounded-md border border-pendiente/30 bg-pendiente/5 px-3 py-2 text-sm text-content">
            {usd} factura(s) en USD no se importan automáticamente (el Excel no trae tipo de cambio).
            Cargalas manualmente con su TC desde “Nuevo documento”.
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
                <TH className="text-right">Total</TH>
              </TR>
            </THead>
            <TBody>
              {filas.map((f, i) => (
                <FilaPreview key={`${f.clave}-${i}`} fila={f} />
              ))}
            </TBody>
          </Table>
        </TableContainer>
      </CardContent>
    </Card>
  );
}

function FilaPreview({ fila }: { fila: FilaImportada }) {
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
      <TD className="text-right tabular-nums">
        {formatMoneda(fila.total, (fila.moneda as Moneda) || "CRC")}
        {fila.moneda === "USD" && (
          <Badge tone="pendiente" className="ml-2">
            USD
          </Badge>
        )}
      </TD>
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
