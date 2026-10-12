/**
 * Cargar el reporte de pagos que el sistema ANTERIOR ya tenía aplicados.
 *
 * El ERP no puede distinguir entre «este pago nunca se registró» y «este ya está aplicado en el
 * otro sistema»: los trata a todos como pendientes y la cifra asusta sin significar nada. Qué se
 * aplicó allá no se deduce de ningún dato de acá — hay que traerlo del reporte.
 *
 * Dos pasos, como el resto de los importadores del proyecto: se sube, se ve QUÉ VA A PASAR y recién
 * ahí se confirma. Marcar miles de movimientos como resueltos sin que nadie mire antes no se puede
 * deshacer leyendo la pantalla.
 *
 * Lo que esto NO hace: enlazar el movimiento a una factura. El sello dice «esto ya está resuelto
 * afuera», que es más débil y más honesto que «esto pagó la factura X».
 */
import { useState } from "react";
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
import { bancosApi, type PrevisualizacionPagosExternos, type ResultadoCruce } from "@/api/bancos";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";
import { formatFecha, formatMonto } from "@/lib/format";
import { cn } from "@/lib/cn";

/** Qué significa cada veredicto, en palabras de quien va a leer la pantalla. */
const VEREDICTO: Record<string, { texto: string; tono: "positivo" | "pendiente" | "negativo" | "neutral"; ayuda: string }> = {
  UNICO: {
    texto: "calza",
    tono: "positivo",
    ayuda: "Se encontró exactamente un movimiento. Es lo único que se va a marcar.",
  },
  VARIOS: {
    texto: "varios candidatos",
    tono: "pendiente",
    ayuda: "Calzó con más de un movimiento. No se marca ninguno: elegir sería adivinar.",
  },
  SIN_MOVIMIENTO: {
    texto: "sin movimiento",
    tono: "neutral",
    ayuda: "No se encontró el movimiento: puede ser de una cuenta que no se cargó, de otra empresa o de un período que el ERP todavía no tiene.",
  },
  ILEGIBLE: {
    texto: "no se pudo leer",
    tono: "negativo",
    ayuda: "La fila no trae con qué cruzarse, o un dato no se entendió.",
  },
};

export function PagosExternosPage() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();

  const [archivo, setArchivo] = useState<File | null>(null);
  const [previa, setPrevia] = useState<PrevisualizacionPagosExternos | null>(null);
  const [ultimaCarga, setUltimaCarga] = useState("");

  const resumen = useQuery({
    queryKey: ["bancos", empresaId, "pagos-externos"],
    queryFn: () => bancosApi.resumenPagosExternos(),
  });

  const previsualizar = useMutation({
    mutationFn: (f: File) => bancosApi.previsualizarPagosExternos(f),
    onSuccess: setPrevia,
    onError: (e) => {
      setPrevia(null);
      toast.error(mensajeError(e));
    },
  });

  const confirmar = useMutation({
    mutationFn: (f: File) => bancosApi.confirmarPagosExternos(f),
    onSuccess: (r) => {
      setUltimaCarga(r.carga_id);
      setPrevia(null);
      setArchivo(null);
      void qc.invalidateQueries({ queryKey: ["bancos", empresaId] });
      toast.success(
        r.sellados === 0
          ? "No había nada que marcar: ninguna fila calzó con un movimiento."
          : `Se marcaron ${r.sellados} movimiento(s) como aplicados en el sistema anterior.`,
      );
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const revertir = useMutation({
    mutationFn: (carga: string) => bancosApi.revertirPagosExternos(carga),
    onSuccess: (r) => {
      setUltimaCarga("");
      void qc.invalidateQueries({ queryKey: ["bancos", empresaId] });
      toast.success(`Se quitaron ${r.quitados} marca(s): esos movimientos vuelven a estar pendientes.`);
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  function elegir(f: File | null) {
    setArchivo(f);
    setPrevia(null);
    setUltimaCarga("");
    if (f) previsualizar.mutate(f);
  }

  const r = resumen.data;

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-xl font-semibold text-content">Pagos aplicados en el sistema anterior</h1>
        <p className="text-sm text-content-muted">
          El ERP no sabe cuáles de sus salidas ya estaban resueltas en el otro sistema, y por eso las
          cuenta todas como pendientes. Este reporte lo dice.
        </p>
      </div>

      {/* El estado actual, con el número que importa a la derecha. */}
      {resumen.isPending ? (
        <LoadingState label="Leyendo el estado" />
      ) : resumen.isError ? (
        <ErrorState message={mensajeError(resumen.error)} onRetry={() => void resumen.refetch()} />
      ) : (
        r && (
          <div className="grid gap-3 md:grid-cols-3">
            <Dato
              k="Aplicados en el sistema anterior"
              v={r.sellados.toLocaleString("es-CR")}
              sub={`₡${formatMonto(r.monto_sellado)} · ${r.cargas} carga(s)`}
            />
            <Dato
              k="Pendientes de explicar"
              v={r.pendientes.toLocaleString("es-CR")}
              sub={`₡${formatMonto(r.monto_pendiente)}`}
              alerta={r.pendientes > 0}
            />
            <div className="rounded-lg border border-border bg-surface-raised px-4 py-3">
              <p className="text-xs uppercase tracking-wider text-content-muted">Qué queda pendiente</p>
              <p className="mt-1 text-sm text-content-muted">
                Débitos que no tienen factura del ERP detrás <b className="text-content">ni</b> sello
                del sistema anterior. Es el número que este reporte hace bajar.
              </p>
            </div>
          </div>
        )
      )}

      <Card>
        <CardHeader>
          <CardTitle>Subir el reporte</CardTitle>
          <CardDescription>
            Excel o CSV, como salga del otro sistema. Las columnas se resuelven por encabezado: hace
            falta una de <b>referencia</b>, o una de <b>fecha</b> y otra de <b>monto</b>. Nada se
            escribe hasta que confirmés.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <input
            type="file"
            accept=".csv,.xlsx,.xls,text/csv"
            onChange={(e) => elegir(e.target.files?.[0] ?? null)}
            className="block w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm file:mr-3 file:rounded-md file:border-0 file:bg-accent file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-white"
          />
          {previsualizar.isPending && <p className="text-sm text-content-muted">Cruzando contra el banco…</p>}

          {previa && (
            <>
              <div className="grid gap-3 md:grid-cols-4">
                <Dato k="Filas del reporte" v={previa.filas.toLocaleString("es-CR")} />
                <Dato
                  k="Van a marcarse"
                  v={previa.unicos.toLocaleString("es-CR")}
                  sub={`₡${formatMonto(previa.monto_a_marcar)}`}
                />
                <Dato k="Varios candidatos" v={previa.varios.toLocaleString("es-CR")} alerta={previa.varios > 0} />
                <Dato
                  k="Sin movimiento"
                  v={(previa.sin_movimiento + previa.ilegibles).toLocaleString("es-CR")}
                  alerta={previa.sin_movimiento + previa.ilegibles > 0}
                />
              </div>

              {previa.con_diferencia > 0 && (
                <p className="rounded-lg border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-sm text-content">
                  ⚠ {previa.con_diferencia} fila(s) calzaron por referencia pero el monto del reporte
                  no es el que debitó el banco. <b>Se marcan igual</b> —una comisión o un redondeo no
                  invalidan el pago— y quedan listadas abajo para que las mires.
                </p>
              )}

              {previa.unicos === 0 ? (
                <p className="text-sm text-content-muted">
                  Ninguna fila calzó con un movimiento: no hay nada que marcar.
                </p>
              ) : (
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    onClick={() => archivo && confirmar.mutate(archivo)}
                    loading={confirmar.isPending}
                    disabled={!archivo}
                  >
                    Marcar {previa.unicos.toLocaleString("es-CR")} movimiento(s) como aplicados
                  </Button>
                  <span className="text-xs text-content-muted">
                    Solo se marcan los que calzaron con UNO. Lo demás queda igual.
                  </span>
                </div>
              )}

              <DetalleDelCruce detalle={previa.detalle} total={previa.filas} />
            </>
          )}

          {ultimaCarga && (
            <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-surface-muted px-3 py-2">
              <span className="text-sm text-content-muted">
                ¿Era el archivo equivocado? Se puede deshacer la carga entera.
              </span>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => revertir.mutate(ultimaCarga)}
                loading={revertir.isPending}
              >
                Deshacer esta carga
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function Dato({ k, v, sub, alerta }: { k: string; v: string; sub?: string; alerta?: boolean }) {
  return (
    <div className="rounded-lg border border-border bg-surface-raised px-4 py-3">
      <p className="text-xs uppercase tracking-wider text-content-muted">{k}</p>
      <p className={cn("text-2xl font-semibold tabular-nums", alerta ? "text-negativo" : "text-content")}>{v}</p>
      {sub && <p className="text-xs text-content-muted">{sub}</p>}
    </div>
  );
}

/**
 * El detalle del cruce. Lo que NO calzó va primero —lo pone así el servidor— porque es lo único
 * sobre lo que alguien puede hacer algo: en una corrida de miles, las excepciones nunca se verían
 * si estuvieran al final.
 */
function DetalleDelCruce({ detalle, total }: { detalle: ResultadoCruce[]; total: number }) {
  if (detalle.length === 0) return null;
  return (
    <div className="flex flex-col gap-1">
      <p className="text-xs text-content-muted">
        Detalle fila por fila{detalle.length < total && <> (las primeras {detalle.length} de {total})</>}. Lo
        que no calzó va primero.
      </p>
      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH className="text-right">Línea</TH>
              <TH>Referencia</TH>
              <TH>Tercero</TH>
              <TH>Fecha</TH>
              <TH className="text-right">Monto</TH>
              <TH>Resultado</TH>
            </TR>
          </THead>
          <TBody>
            {detalle.map((d, i) => {
              const v = VEREDICTO[d.veredicto] ?? VEREDICTO.ILEGIBLE!;
              return (
                <TR key={`${d.fila.linea}-${i}`}>
                  <TD className="text-right tabular-nums text-content-muted">{d.fila.linea}</TD>
                  <TD className="font-mono text-[11px]">{d.fila.referencia || "—"}</TD>
                  <TD className="max-w-48 truncate">{d.fila.tercero || "—"}</TD>
                  <TD className="whitespace-nowrap tabular-nums">
                    {d.fila.fecha ? formatFecha(d.fila.fecha) : "—"}
                  </TD>
                  <TD className="text-right tabular-nums">{d.fila.monto ? formatMonto(d.fila.monto) : "—"}</TD>
                  <TD>
                    <span title={v.ayuda}>
                      <Badge tone={v.tono}>{v.texto}</Badge>
                    </span>
                    {d.veredicto === "VARIOS" && (
                      <span className="ml-1 text-[11px] text-content-muted">{d.candidatos} movimientos</span>
                    )}
                    {d.diferencia_monto && (
                      <span className="ml-1 text-[11px] text-pendiente">
                        difiere {formatMonto(d.diferencia_monto)}
                      </span>
                    )}
                    {d.fila.motivo && (
                      <span className="block text-[11px] text-content-muted">{d.fila.motivo}</span>
                    )}
                  </TD>
                </TR>
              );
            })}
          </TBody>
        </Table>
      </TableContainer>
    </div>
  );
}
