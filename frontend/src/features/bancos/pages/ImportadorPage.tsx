/**
 * Pantalla 1 — Importador (/importador).
 * Flujo: elegir cuenta -> subir .xlsx -> ver PreviewResult (banda de resumen +
 * tabla con chips por estado_duplicado) -> marcar exclusiones (DUPLICADO_REAL
 * por defecto) -> Confirmar -> toast con `insertados`.
 * 422 = formato no reconocido -> se muestra el mensaje del backend.
 *
 * Abajo va «Cargas hechas»: el historial de lo que se subió y la salida cuando una carga entró en
 * la cuenta equivocada (revertirla entera, mig 0085). Vive en esta pantalla porque es la misma
 * pregunta que trae acá —«¿qué subí?»— y porque quien acaba de confirmar mal lo corrige sin irse.
 */

import { useMemo, useRef, useState, type ChangeEvent, type DragEvent } from "react";
import { Link } from "react-router-dom";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
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
import { formatMonto, formatFecha } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { chipEstadoDuplicado } from "@/features/bancos/chips";
import { useConfirmarImportacion, useCuentas, useImportar } from "@/features/bancos/hooks";
import { ClasificarDesdeExcelPanel } from "@/features/bancos/components/ClasificarDesdeExcelPanel";
import { CargarHistoricoPanel } from "@/features/bancos/components/CargarHistoricoPanel";
import { CargasHechasPanel } from "@/features/bancos/components/CargasHechasPanel";
import type { PreviewResult } from "@/api/bancos";

export function ImportadorPage() {
  const toast = useToast();
  const cuentasQuery = useCuentas();
  const importar = useImportar();
  const confirmar = useConfirmarImportacion();

  const [cuentaId, setCuentaId] = useState("");
  const [archivo, setArchivo] = useState<File | null>(null);
  const [preview, setPreview] = useState<PreviewResult | null>(null);
  // natural_key marcadas para EXCLUIR de la confirmación.
  const [excluidas, setExcluidas] = useState<Set<string>>(new Set());
  const [dragOver, setDragOver] = useState(false);
  // El rechazo del archivo se queda EN LA PANTALLA, no en un toast (22-oct-2026).
  //
  // El toast mide 384 px y dura cinco segundos. Desde que el importador explica por qué no
  // reconoció un archivo —qué hoja leyó, qué filas encontró, qué columnas entendió y dónde se
  // agrega un banco nuevo—, el mensaje pasa de seis palabras a varias líneas: en un toast no se
  // alcanza a leer, y menos a copiar de ahí los nombres de las columnas para configurar el
  // formato. Que es justamente para lo que sirve.
  const [rechazo, setRechazo] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);

  const cuentaOptions = useMemo(
    () =>
      (cuentasQuery.data ?? []).map((c) => ({
        value: c.id,
        label: `${c.alias} · ${c.banco} (${c.moneda})`,
      })),
    [cuentasQuery.data],
  );

  function seleccionarArchivo(f: File | undefined | null) {
    if (!f) return;
    if (!f.name.toLowerCase().endsWith(".xlsx")) {
      toast.error("El archivo debe ser un Excel .xlsx");
      return;
    }
    setArchivo(f);
    setPreview(null);
    setExcluidas(new Set());
    // El rechazo anterior se va al elegir otro archivo: si se quedara, describiría un archivo que
    // ya no es el que está cargado.
    setRechazo("");
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
    if (!cuentaId) {
      toast.error("Elegí una cuenta bancaria primero.");
      return;
    }
    if (!archivo) {
      toast.error("Seleccioná un archivo .xlsx.");
      return;
    }
    setRechazo("");
    importar.mutate(
      { cuentaId, archivo },
      {
        onSuccess: (raw) => {
          // Blindaje: un slice vacío puede llegar como null desde el backend; normalizamos
          // `advertencias` a [] (archivo y por línea) para que ningún .length/.filter reviente.
          const res: PreviewResult = {
            ...raw,
            advertencias: raw.advertencias ?? [],
            movimientos: (raw.movimientos ?? []).map((m) => ({ ...m, advertencias: m.advertencias ?? [] })),
          };
          setPreview(res);
          // Preseleccionar los DUPLICADO_REAL para excluir (el caso normal).
          const preExcluir = new Set(
            res.movimientos
              .filter((m) => m.estado_duplicado === "DUPLICADO_REAL")
              .map((m) => m.natural_key),
          );
          setExcluidas(preExcluir);
          toast.info(
            `Preview de ${res.banco}: ${res.resumen.nuevas} nuevas, ${res.resumen.duplicados_reales} duplicados.`,
          );
        },
        // Al bloque de la pantalla, no al toast: ver el comentario de `rechazo`. El toast queda
        // como el aviso de que algo pasó —una línea— y el detalle se lee abajo con calma.
        onError: (err) => {
          setRechazo(mensajeError(err));
          toast.error("No se pudo leer el archivo. El detalle está abajo.");
        },
      },
    );
  }

  function toggleExcluir(naturalKey: string) {
    setExcluidas((prev) => {
      const next = new Set(prev);
      if (next.has(naturalKey)) next.delete(naturalKey);
      else next.add(naturalKey);
      return next;
    });
  }

  function confirmarImportacion() {
    if (!preview) return;
    confirmar.mutate(
      { importacionId: preview.importacion_id, excluir: [...excluidas] },
      {
        onSuccess: (res) => {
          toast.success(`Importación confirmada: ${res.insertados} movimientos insertados.`);
          // Reset para una nueva importación.
          setPreview(null);
          setArchivo(null);
          setExcluidas(new Set());
          if (fileInputRef.current) fileInputRef.current.value = "";
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      <PageHeader
        title="Importador"
        description="Subí un estado de cuenta en Excel (.xlsx) y confirmá los movimientos nuevos."
      />

      <Card>
        <CardHeader>
          <CardTitle>1. Cuenta y archivo</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {cuentasQuery.isPending ? (
            <LoadingState label="Cargando cuentas" />
          ) : cuentasQuery.isError ? (
            <ErrorState message={mensajeError(cuentasQuery.error)} onRetry={() => cuentasQuery.refetch()} />
          ) : cuentaOptions.length === 0 ? (
            <EmptyState message="No hay cuentas bancarias registradas para esta empresa." />
          ) : (
            <Select
              label="Cuenta bancaria"
              placeholder="Elegí una cuenta"
              value={cuentaId}
              onChange={(e) => setCuentaId(e.target.value)}
              options={cuentaOptions}
            />
          )}

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
            <p className="text-sm text-content-muted">
              Arrastrá el archivo .xlsx aquí, o
            </p>
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
              aria-label="Archivo Excel del estado de cuenta"
            />
            {archivo && (
              <p className="mt-1 text-sm font-medium text-content">
                {archivo.name}{" "}
                <span className="text-content-muted">
                  ({(archivo.size / 1024).toFixed(0)} KB)
                </span>
              </p>
            )}
          </div>

          <div className="flex justify-end">
            <Button
              onClick={subir}
              loading={importar.isPending}
              disabled={!cuentaId || !archivo}
            >
              Previsualizar
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* POR QUÉ NO SE PUDO LEER EL ARCHIVO, en la pantalla y no en un toast.
          Va JUSTO DEBAJO del botón «Previsualizar», que es donde está mirando quien acaba de
          hacer clic. El mensaje trae qué hoja se leyó, qué filas encontró y dónde se agrega un
          banco nuevo: con eso se corrige el archivo o se configura el formato, y para las dos
          cosas hay que poder leerlo con calma. */}
      {rechazo && (
        <div className="rounded-lg border border-negativo/40 bg-negativo/5 p-4">
          <p className="text-sm font-medium text-negativo">No se pudo leer este archivo</p>
          <p className="mt-1 whitespace-pre-line text-sm leading-relaxed text-content">{rechazo}</p>
          <div className="mt-3 flex flex-wrap gap-3 text-xs">
            <Link to="/formatos" className="font-medium text-accent underline">
              Ir a Formatos de banco →
            </Link>
            <button
              type="button"
              className="text-content-muted underline hover:text-content"
              onClick={() => setRechazo("")}
            >
              Ocultar
            </button>
          </div>
        </div>
      )}

      {preview && <PreviewBlock preview={preview} excluidas={excluidas} onToggle={toggleExcluir} />}

      {preview && (
        <div className="flex items-center justify-between rounded-lg border border-border bg-surface-raised px-5 py-4">
          {(() => {
            const invalidas = preview.movimientos.filter((m) => m.advertencias.length > 0).length;
            const insertables = preview.movimientos.filter(
              (m) => m.advertencias.length === 0 && !excluidas.has(m.natural_key),
            ).length;
            return (
              <p className="text-sm text-content-muted">
                Se insertarán <span className="font-medium text-content">{insertables}</span> de{" "}
                {preview.movimientos.length} líneas ({excluidas.size} excluidas
                {invalidas > 0 ? `, ${invalidas} inválidas` : ""}).
              </p>
            );
          })()}
          <Button onClick={confirmarImportacion} loading={confirmar.isPending}>
            Confirmar importación
          </Button>
        </div>
      )}

      {/* Segunda vía de entrada: no cargar movimientos sino la CLASIFICACIÓN que ya se hizo en Excel.
          Va en esta pantalla porque es donde alguien busca «cómo meto lo que tengo en un archivo». */}
      <ClasificarDesdeExcelPanel cuentas={cuentasQuery.data ?? []} />

      {/* Tercera vía: el HISTÓRICO. Crea movimientos de varias cuentas y varios meses de un solo
          archivo. Va después de la otra a propósito: son las dos que se confunden, y así se leen
          seguidas con la diferencia escrita al lado. */}
      <CargarHistoricoPanel />

      {/* La salida del error de carga: qué se subió y cómo se revierte una carga entera (mig 0085).
          Va acá abajo porque es la misma pregunta que trae a esta pantalla —«¿qué subí?»— y porque
          quien acaba de confirmar una carga equivocada la ve aparecer y la corrige sin irse. */}
      <CargasHechasPanel cuentas={cuentasQuery.data ?? []} />
    </div>
  );
}

interface PreviewBlockProps {
  preview: PreviewResult;
  excluidas: Set<string>;
  onToggle: (naturalKey: string) => void;
}

function PreviewBlock({ preview, excluidas, onToggle }: PreviewBlockProps) {
  const { resumen } = preview;
  const stats = [
    { label: "Leídas", value: resumen.leidas, tone: "neutral" as const },
    { label: "Nuevas", value: resumen.nuevas, tone: "positivo" as const },
    { label: "Duplicados reales", value: resumen.duplicados_reales, tone: "negativo" as const },
    { label: "Reimportación", value: resumen.reimportacion, tone: "accent" as const },
    ...(resumen.invalidas > 0
      ? [{ label: "Inválidas (§19)", value: resumen.invalidas, tone: "negativo" as const }]
      : []),
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle>2. Previsualización</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {/* Banda de resumen */}
        <div className="flex flex-wrap items-center gap-4">
          <div className="text-sm">
            <span className="text-content-muted">Banco:</span>{" "}
            <span className="font-medium text-content">{preview.banco}</span>
          </div>
          <div className="text-sm">
            <span className="text-content-muted">IBAN archivo:</span>{" "}
            <span className="font-mono text-content">{preview.iban_archivo || "—"}</span>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {stats.map((s) => (
            <div key={s.label} className="rounded-md border border-border bg-surface px-3 py-2">
              <p className="text-xs uppercase tracking-wide text-content-muted">{s.label}</p>
              <p className="mt-0.5 text-xl font-semibold tabular-nums text-content">{s.value}</p>
            </div>
          ))}
        </div>

        {/* Advertencias de integridad a nivel de archivo (§19). */}
        {preview.advertencias.length > 0 && (
          <ul className="flex flex-col gap-1 rounded-lg border border-brand-gold/50 bg-brand-gold/10 px-4 py-2.5 text-sm">
            {preview.advertencias.map((a, i) => (
              <li key={i} className="flex gap-2">
                <span aria-hidden>⚠️</span>
                <span>{a}</span>
              </li>
            ))}
          </ul>
        )}

        {/* Tabla de movimientos */}
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH className="w-10">Incluir</TH>
                <TH>Fecha</TH>
                <TH>Documento</TH>
                <TH>Descripción</TH>
                <TH className="text-right">Débito</TH>
                <TH className="text-right">Crédito</TH>
                <TH>Moneda</TH>
                <TH>Estado</TH>
              </TR>
            </THead>
            <TBody>
              {preview.movimientos.map((m) => {
                const chip = chipEstadoDuplicado(m.estado_duplicado);
                const invalida = m.advertencias.length > 0;
                const excluido = excluidas.has(m.natural_key);
                return (
                  <TR
                    key={`${m.natural_key}-${m.indice_ocurrencia}`}
                    className={cn((excluido || invalida) && "opacity-50")}
                  >
                    <TD>
                      <input
                        type="checkbox"
                        checked={!excluido && !invalida}
                        disabled={invalida}
                        onChange={() => onToggle(m.natural_key)}
                        aria-label={`Incluir movimiento ${m.descripcion}`}
                        className="h-4 w-4 rounded border-border accent-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
                        title={invalida ? "Línea inválida: no se importa (§19)" : undefined}
                      />
                    </TD>
                    <TD className="tabular-nums">{formatFecha(m.fecha)}</TD>
                    <TD className="font-mono text-xs">{m.documento || "—"}</TD>
                    <TD className="max-w-xs truncate" title={m.descripcion}>
                      {m.descripcion}
                      {invalida && (
                        <span className="mt-0.5 block text-[11px] font-medium text-negativo" title={m.advertencias.join(" · ")}>
                          ⚠️ {m.advertencias.join(" · ")}
                        </span>
                      )}
                    </TD>
                    <TD className="text-right tabular-nums">{formatMonto(m.debito)}</TD>
                    <TD className="text-right tabular-nums">{formatMonto(m.credito)}</TD>
                    <TD>{m.moneda}</TD>
                    <TD>
                      <Badge tone={chip.tone}>{chip.label}</Badge>
                    </TD>
                  </TR>
                );
              })}
            </TBody>
          </Table>
        </TableContainer>
        <p className="text-xs text-content-muted">
          Los duplicados reales vienen preseleccionados para excluir. Desmarcá una fila para
          NO insertarla.
        </p>
      </CardContent>
    </Card>
  );
}
