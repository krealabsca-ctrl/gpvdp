/**
 * «Cargar histórico» — meter años de bancos de un solo archivo (mig 0087).
 *
 * El pedido del Director Financiero, textual: «lo que no se puede hacer es buscar todos los bancos
 * de estos años e ir de uno en uno cargándolo». Tiene 2025 y 2026 en Excel y ya segmentados; sube UN
 * archivo con varias cuentas y varios meses, y esto CREA los movimientos con su partida puesta.
 *
 * Su primer intento falló porque usó «Traer la clasificación desde Excel», que solo PINTA la partida
 * sobre movimientos ya cargados: como no había nada anterior a julio de 2026, todas sus filas salían
 * «ese movimiento no está cargado». Por eso el encabezado de acá dice en una línea qué hace esta
 * sección y qué la distingue de la otra: confundirlas fue lo que le costó el tiempo.
 *
 * Dos pasos a propósito. Subir NO escribe: devuelve el plan. Lo que hay que poder mirar antes de
 * apretar es cuánta plata entra POR CUENTA, y eso no es lo mismo que cuánta trae el archivo —en una
 * re-subida, casi todo ya está—. Por eso la tabla muestra las dos cosas y el botón habla de lo nuevo.
 *
 * Lo que el archivo nombra y el sistema no tiene se muestra SEPARADO y con su nombre exacto, porque
 * se arregla distinto: una cuenta que falta hay que crearla y volver a subir (sus filas no entran);
 * una partida que falta no frena nada (el movimiento entra sin partida y se clasifica después). El
 * adorno del reporte —bandas, subtotales, el pie— se cuenta aparte y sin alarma: si saliera como
 * error, cada archivo exportado por el propio sistema mostraría errores que no existen.
 */

import { useRef, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
} from "@/components/ui";
import { formatFecha, formatMoneda } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useConfirmarHistorico, useSubirHistorico } from "@/features/bancos/hooks";
import type { PlanHistorico } from "@/api/bancos";

const nf = new Intl.NumberFormat("es-CR");

export function CargarHistoricoPanel() {
  const toast = useToast();
  const subir = useSubirHistorico();
  const confirmar = useConfirmarHistorico();
  const inputRef = useRef<HTMLInputElement>(null);
  const [plan, setPlan] = useState<PlanHistorico | null>(null);

  function elegir(f: File | null) {
    setPlan(null);
    if (!f) return;
    if (!f.name.toLowerCase().endsWith(".xlsx")) {
      toast.error("El archivo debe ser un Excel .xlsx");
      if (inputRef.current) inputRef.current.value = "";
      return;
    }
    // Elegir el archivo ya previsualiza: un paso menos, y no se escribe nada.
    subir.mutate(f, {
      onSuccess: setPlan,
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  function confirmarCarga() {
    if (!plan) return;
    confirmar.mutate(plan.carga_id, {
      onSuccess: (res) => {
        setPlan(res);
        if (inputRef.current) inputRef.current.value = "";
        toast.success(
          `Se cargaron ${nf.format(res.insertados)} movimientos en ${nf.format(res.cuentas.length)} cuenta(s).`,
        );
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  const t = plan?.totales;
  // Lo que se puede cargar. Con cero, el botón no tiene nada que hacer y la pantalla dice por qué.
  const nuevas = t?.nuevas ?? 0;
  const yaAplicado = plan?.aplicado === true;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Cargar histórico</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <p className="max-w-prose text-sm text-content-muted">
          Subí un Excel con <strong className="text-content">varias cuentas y varios meses</strong> y
          esto <strong className="text-content">crea los movimientos</strong> con la partida que ya
          trae el archivo. Es para meter los años que no están en el sistema, sin ir banco por banco.{" "}
          El archivo que exporta Bancos sirve tal cual; si es tuyo, necesita Fecha, Documento (o
          Consecutivo), Débito, Crédito y la Cuenta, y opcionalmente Concepto y Clasificación.
        </p>
        <p className="max-w-prose text-xs text-content-muted">
          No confundir con «Traer la clasificación desde Excel», acá arriba: esa solo le pone la
          partida a movimientos que YA están cargados, y no crea ninguno.
        </p>

        <div className="flex flex-wrap items-center gap-3">
          <input
            ref={inputRef}
            type="file"
            accept=".xlsx"
            aria-label="Archivo con el histórico de bancos"
            onChange={(e) => elegir(e.target.files?.[0] ?? null)}
            className="text-sm text-content-muted file:mr-3 file:cursor-pointer file:rounded-lg file:border file:border-border file:bg-surface file:px-3 file:py-2 file:text-sm file:text-content hover:file:bg-surface-raised"
          />
          {subir.isPending && <span className="text-sm text-content-muted">Leyendo el archivo…</span>}
        </div>

        {plan && t && (
          <>
            {/* El resumen del archivo entero. Las líneas de formato van acá y no entre los errores. */}
            <div className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
              <p className="text-content">
                {plan.nombre_archivo} · hoja «{plan.hoja}» ·{" "}
                <strong>{nf.format(t.filas)}</strong> movimiento(s)
                {t.fecha_desde && (
                  <>
                    {" "}
                    del {formatFecha(t.fecha_desde)} al {formatFecha(t.fecha_hasta)}
                  </>
                )}
              </p>
              <p className="mt-0.5 text-content-muted">
                {nf.format(t.nuevas)} para cargar · {nf.format(t.ya_existen)} ya estaban ·{" "}
                {nf.format(t.errores)} con error · {nf.format(t.lineas_de_formato)} líneas de formato
                del reporte (no son errores)
              </p>
              {plan.hojas.length > 1 && (
                <p className="mt-0.5 text-content-muted">
                  El libro tiene {plan.hojas.length} hojas y se leyó solo «{plan.hoja}»:{" "}
                  {plan.hojas.filter((h) => h !== plan.hoja).join(", ")} quedó sin leer.
                </p>
              )}
            </div>

            {plan.aviso && <p className="text-sm text-pendiente">{plan.aviso}</p>}

            {plan.cuentas.length > 0 && (
              <TableContainer>
                <Table>
                  <THead>
                    <TR>
                      <TH>Cuenta</TH>
                      <TH>Período</TH>
                      <TH className="text-right">Filas</TH>
                      <TH className="text-right">{yaAplicado ? "Cargados" : "Entran"}</TH>
                      <TH className="text-right">Ya estaban</TH>
                      <TH className="text-right">Débitos que entran</TH>
                      <TH className="text-right">Créditos que entran</TH>
                      <TH>Partida</TH>
                    </TR>
                  </THead>
                  <TBody>
                    {plan.cuentas.map((c) => (
                      <TR key={c.cuenta_bancaria_id}>
                        <TD>
                          <span className="text-sm text-content">
                            {c.banco} · {c.cuenta}
                          </span>
                          {/* Con qué nombre lo escribió el archivo: es lo que hay que corregir si
                              resolvió a una cuenta que no era la esperada. */}
                          {c.nombres_en_archivo.length > 0 && (
                            <span className="block text-[11px] text-content-muted">
                              en el archivo: {c.nombres_en_archivo.join(" · ")}
                            </span>
                          )}
                        </TD>
                        <TD className="text-sm whitespace-nowrap text-content-muted">
                          {c.fecha_desde ? (
                            <>
                              {formatFecha(c.fecha_desde)} – {formatFecha(c.fecha_hasta)}
                              <span className="block text-[11px]">
                                {nf.format(c.meses.length)} mes(es)
                              </span>
                            </>
                          ) : (
                            "—"
                          )}
                        </TD>
                        <TD className="text-right tabular-nums">{nf.format(c.filas)}</TD>
                        <TD className="text-right font-medium tabular-nums">
                          {nf.format(yaAplicado ? c.insertados : c.nuevas)}
                        </TD>
                        <TD className="text-right tabular-nums text-content-muted">
                          {nf.format(c.ya_existen)}
                        </TD>
                        <TD className="text-right tabular-nums">
                          {formatMoneda(c.debitos_nuevos, c.moneda)}
                        </TD>
                        <TD className="text-right tabular-nums">
                          {formatMoneda(c.creditos_nuevos, c.moneda)}
                        </TD>
                        <TD className="text-sm">
                          {c.sin_partida > 0 && (
                            <Badge tone="neutral">{nf.format(c.sin_partida)} sin partida</Badge>
                          )}
                          {c.partida_desconocida > 0 && (
                            <Badge tone="pendiente" className="ml-1">
                              {nf.format(c.partida_desconocida)} no existe
                            </Badge>
                          )}
                          {c.sin_tipo_cambio > 0 && (
                            <Badge tone="pendiente" className="ml-1">
                              {nf.format(c.sin_tipo_cambio)} sin tipo de cambio
                            </Badge>
                          )}
                        </TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              </TableContainer>
            )}

            {/* Lo que falta crear. Va SEPARADO de los errores porque no se arregla igual. */}
            {plan.cuentas_no_resueltas.length > 0 && (
              <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-3 py-2 text-sm">
                <p className="font-medium text-content">
                  {plan.cuentas_no_resueltas.length} cuenta(s) del archivo no existen acá. Sus{" "}
                  {nf.format(t.sin_cuenta)} fila(s) NO se van a cargar.
                </p>
                <ul className="mt-1 flex flex-col gap-0.5 text-content-muted">
                  {plan.cuentas_no_resueltas.map((c) => (
                    <li key={c.nombre_en_archivo}>
                      «{c.nombre_en_archivo}» — {c.motivo} · {nf.format(c.filas)} fila(s), la primera
                      en la línea {c.primera_linea}
                    </li>
                  ))}
                </ul>
                <p className="mt-1 text-content-muted">
                  Creá esas cuentas en el catálogo y volvé a subir el archivo: lo que ya entró no se
                  duplica.
                </p>
              </div>
            )}

            {plan.partidas_faltantes.length > 0 && (
              <div className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
                <p className="text-content">
                  {plan.partidas_faltantes.length} partida(s) del archivo no están en el catálogo.
                  Esos movimientos <strong>sí se cargan</strong>, sin partida, y se clasifican después.
                </p>
                <p className="mt-0.5 text-content-muted">{plan.partidas_faltantes.join(" · ")}</p>
              </div>
            )}

            {plan.errores.length > 0 && (
              <div className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
                <p className="font-medium text-content">
                  {nf.format(t.errores)} fila(s) no se pudieron leer y no se van a cargar.
                </p>
                <ul className="mt-1 flex flex-col gap-0.5 text-content-muted">
                  {plan.errores.map((e) => (
                    <li key={e.linea}>
                      Línea {e.linea}: {e.motivo}
                      {e.texto && <> — «{e.texto}»</>}
                    </li>
                  ))}
                </ul>
                {plan.errores_truncados && (
                  <p className="mt-1 text-content-muted">
                    La lista se recortó; el total de arriba es el real.
                  </p>
                )}
              </div>
            )}

            {yaAplicado ? (
              <div className="rounded-lg border border-accent/40 bg-accent/5 px-3 py-2 text-sm">
                <p className="font-medium text-content">
                  Listo: entraron {nf.format(plan.insertados)} movimientos en{" "}
                  {nf.format(plan.cuentas.length)} cuenta(s).
                </p>
                <p className="mt-0.5 text-content-muted">
                  Cada cuenta quedó como una carga propia en «Cargas hechas», acá abajo: si alguna
                  salió mal, se revierte sola sin tocar las otras.
                </p>
              </div>
            ) : (
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-surface-raised px-4 py-3">
                <p className="text-sm text-content-muted">
                  {nuevas > 0 ? (
                    <>
                      Se van a cargar <span className="font-medium text-content">{nf.format(nuevas)}</span>{" "}
                      movimientos en {nf.format(plan.cuentas.length)} cuenta(s), por{" "}
                      {formatMoneda(t.total_debitos)} en débitos y {formatMoneda(t.total_creditos)} en
                      créditos.
                    </>
                  ) : (
                    <>
                      No hay nada que cargar:{" "}
                      {t.ya_existen > 0
                        ? "todas las filas del archivo ya están en el sistema."
                        : "ninguna fila del archivo se pudo leer o resolver a una cuenta."}
                    </>
                  )}
                </p>
                <Button
                  onClick={confirmarCarga}
                  loading={confirmar.isPending}
                  disabled={nuevas === 0}
                >
                  Cargar histórico
                </Button>
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
