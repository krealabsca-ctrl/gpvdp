/**
 * El visor de un comprobante electrónico.
 *
 * POR QUÉ EXISTE: hasta hoy, mirar una factura recibida significaba descargar el XML crudo y leerlo
 * a ojo. Y no había alternativa: `documento_cxp.descripcion` guarda 45 caracteres en 4.526 de 4.542
 * facturas (99,6 %), así que del prácticamente todo el sistema NO SE PODÍA SABER QUÉ SE COMPRÓ.
 *
 * LOS VARIOS IVAS NO SON UN CASO RARO: 414 de 3.455 facturas con IVA (12,0 %) tienen una tasa
 * efectiva que no existe en la ley —12,9 %, 11,2 %, 9,3 %— porque mezclan tarifas en sus líneas y
 * el ERP las aplasta en un solo campo. Acá se ven separadas.
 *
 * La factura la interpreta el SERVIDOR. Parsearla acá perdería el ISO-8859-1 en que emite Hacienda
 * (convirtiendo «SEÑOR» en basura sin avisar) y mostraría solo el primero cuando un archivo trae
 * varios comprobantes pegados.
 */

import { createPortal } from "react-dom";
import { Badge, Button, ErrorState, LoadingState, TBody, TD, TH, THead, Table, TableContainer, TR } from "@/components/ui";
import { formatFecha, formatMonto } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useComprobanteRecepcion } from "@/features/cxp/hooks";
import type { Comprobante, LineaComprobante } from "@/api/cxp";

interface Props {
  recepcionId: string;
  /** Para el pie: los botones de descarga del original conviven con el visor, no lo reemplazan. */
  onDescargarXML?: () => void;
  onDescargarPDF?: () => void;
  tienePDF?: boolean;
  onCerrar: () => void;
}

export function VisorComprobante(props: Props) {
  const q = useComprobanteRecepcion(props.recepcionId);

  return createPortal(
    <div
      className="fixed inset-0 z-[95] flex items-start justify-center overflow-y-auto bg-black/50 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && props.onCerrar()}
      role="dialog"
      aria-modal="true"
      aria-label="Comprobante electrónico"
    >
      <div className="my-4 flex w-full max-w-5xl flex-col gap-4 rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        {q.isPending ? (
          <LoadingState label="Leyendo el comprobante" />
        ) : q.isError ? (
          <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
        ) : (
          <>
            {/* Los avisos van ARRIBA y en palabras. Callar lo que no se pudo leer es peor que
                mostrarlo: quien mira creería que vio la factura entera. */}
            {q.data!.avisos.length > 0 && (
              <div className="rounded-lg border border-pendiente/40 bg-pendiente/5 px-4 py-3">
                {q.data!.avisos.map((a, i) => (
                  <p key={i} className="text-sm text-content">
                    ⚠ {a}
                  </p>
                ))}
              </div>
            )}

            {q.data!.comprobante ? (
              <CuerpoComprobante c={q.data!.comprobante} />
            ) : (
              <p className="py-6 text-center text-sm text-content-muted">
                No hay una factura que mostrar. El archivo original sigue disponible abajo.
              </p>
            )}
          </>
        )}

        <div className="flex flex-wrap items-center justify-end gap-2 border-t border-border pt-3">
          {/* El original NUNCA se quita: es la salida de emergencia el día que un proveedor nuevo
              traiga algo que el visor no entienda. */}
          {props.onDescargarXML && (
            <Button size="sm" variant="secondary" onClick={props.onDescargarXML}>
              Descargar XML
            </Button>
          )}
          {props.tienePDF && props.onDescargarPDF && (
            <Button size="sm" variant="secondary" onClick={props.onDescargarPDF}>
              Descargar PDF
            </Button>
          )}
          <Button size="sm" onClick={props.onCerrar}>
            Cerrar
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

function CuerpoComprobante({ c }: { c: Comprobante }) {
  return (
    <div className="flex flex-col gap-5">
      {/* ── Identidad ── */}
      <div>
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-lg font-semibold text-content">{c.tipo_nombre || c.tipo}</h2>
          {c.version && <Badge tone="neutral">{c.version}</Badge>}
          {c.descuadre === "" ? (
            <Badge tone="positivo">Los números cuadran</Badge>
          ) : (
            <Badge tone="negativo">No cuadra</Badge>
          )}
        </div>
        <p className="mt-1 font-mono text-xs text-content-muted">
          {c.consecutivo}
          {c.clave && (
            <>
              {" · "}
              <span className="break-all">{c.clave}</span>
            </>
          )}
        </p>
        {c.fecha_emision && (
          <p className="mt-1 text-sm text-content-muted">
            Emitida el {formatFecha(c.fecha_emision.slice(0, 10))}
          </p>
        )}
      </div>

      {/* ── Emisor y receptor ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Parte titulo="Emisor" nombre={c.emisor.nombre} id={c.emisor.identificacion} />
        {c.receptor ? (
          <Parte titulo="Receptor" nombre={c.receptor.nombre} id={c.receptor.identificacion} />
        ) : (
          <div className="rounded-lg border border-border bg-surface-muted px-3 py-2">
            <p className="text-[11px] font-bold uppercase tracking-wider text-content-muted">Receptor</p>
            <p className="mt-1 text-sm text-content-muted">
              El comprobante no trae receptor. Es lo normal en un tiquete electrónico.
            </p>
          </div>
        )}
      </div>

      {/* ── Condiciones ── */}
      <div className="flex flex-wrap gap-x-6 gap-y-2 rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
        <Dato
          rotulo="Condición de venta"
          valor={c.condicion_venta_etiqueta ? `${c.condicion_venta_etiqueta} (${c.condicion_venta})` : c.condicion_venta || "—"}
        />
        {c.plazo_credito !== "" && c.plazo_credito !== "0" && <Dato rotulo="Plazo" valor={`${c.plazo_credito} días`} />}
        <Dato rotulo="Moneda" valor={c.moneda || "no declarada"} />
        {/* El tipo de cambio solo importa cuando NO son colones. */}
        {c.moneda !== "CRC" && c.tipo_cambio !== "" && <Dato rotulo="Tipo de cambio" valor={c.tipo_cambio} />}
        {c.medios_pago.map((m, i) => (
          <Dato key={i} rotulo="Medio de pago" valor={`${m.tipo} · ${formatMonto(m.monto)}`} />
        ))}
      </div>

      {/* ── Las líneas: lo que se compró ── */}
      {c.lineas.length > 0 ? (
        <div>
          <h3 className="mb-2 text-sm font-semibold text-content">
            {c.lineas.length === 1 ? "1 línea" : `${c.lineas.length} líneas`}
          </h3>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH className="w-8">#</TH>
                  <TH>Detalle</TH>
                  <TH className="text-right">Cantidad</TH>
                  <TH className="text-right">Precio</TH>
                  <TH className="text-right">Descuento</TH>
                  <TH className="text-right">Subtotal</TH>
                  <TH>Impuesto</TH>
                  <TH className="text-right">Total línea</TH>
                </TR>
              </THead>
              <TBody>
                {c.lineas.map((l, i) => (
                  <Fila key={i} l={l} />
                ))}
              </TBody>
            </Table>
          </TableContainer>
        </div>
      ) : null}

      {/* ── El desglose por tarifa: LOS VARIOS IVAS ── */}
      {c.desglose.length > 0 && (
        <div>
          <h3 className="mb-2 text-sm font-semibold text-content">
            Impuestos{c.desglose.length > 1 && ` · ${c.desglose.length} tarifas distintas`}
          </h3>
          <div className="overflow-x-auto rounded-lg border border-border">
            <table className="w-full text-sm">
              <tbody>
                {c.desglose.map((d, i) => (
                  <tr key={i} className="border-b border-border last:border-0">
                    <td className="px-3 py-1.5 text-content">
                      {d.tarifa !== "" ? `IVA ${d.tarifa} %` : "Impuesto"}
                      <span className="ml-2 text-xs text-content-muted">
                        cód. {d.codigo}
                        {d.codigo_tarifa && ` · tarifa ${d.codigo_tarifa}`}
                      </span>
                    </td>
                    <td className="px-3 py-1.5 text-right tabular-nums text-content">{formatMonto(d.monto)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {c.desglose.length > 1 && (
            <p className="mt-1 text-xs text-content-muted">
              Esta factura mezcla tarifas. Por eso su porcentaje «promedio» no coincide con ninguna tarifa de la ley.
            </p>
          )}
        </div>
      )}

      {/* ── Totales ── */}
      <div className="flex justify-end">
        <div className="w-full max-w-xs">
          <Renglon rotulo="Total venta" valor={c.total_venta} />
          <Renglon rotulo="Descuentos" valor={c.total_descuentos} negativo />
          <Renglon rotulo="Venta neta" valor={c.total_venta_neta} />
          <Renglon rotulo="Impuesto" valor={c.total_impuesto} />
          <Renglon rotulo="Otros cargos" valor={c.total_otros_cargos} />
          <Renglon rotulo="IVA devuelto" valor={c.total_iva_devuelto} negativo />
          <div className="mt-1 flex items-baseline justify-between border-t border-border pt-2">
            <span className="text-sm font-semibold text-content">Total del comprobante</span>
            <span className="text-lg font-semibold tabular-nums text-content">
              {formatMonto(c.total_comprobante)} <span className="text-xs text-content-muted">{c.moneda}</span>
            </span>
          </div>
        </div>
      </div>

      {c.descuadre !== "" && (
        <p className="rounded-lg border border-negativo/40 bg-negativo/5 px-3 py-2 text-sm text-negativo">
          {c.descuadre}. Los montos se muestran tal como vinieron: el sistema no corrige la factura del proveedor.
        </p>
      )}
    </div>
  );
}

function Fila({ l }: { l: LineaComprobante }) {
  const descuento = l.descuentos.reduce((a, d) => a + Number(d.monto || 0), 0);
  return (
    <TR>
      <TD className="text-content-muted">{l.numero}</TD>
      <TD>
        <span className="text-content">{l.detalle}</span>
        {(l.cabys || l.codigo_comercial) && (
          <p className="mt-0.5 font-mono text-[11px] text-content-muted">
            {l.codigo_comercial && <>cód. {l.codigo_comercial}</>}
            {l.codigo_comercial && l.cabys && " · "}
            {l.cabys && <>CABYS {l.cabys}</>}
          </p>
        )}
      </TD>
      <TD className="text-right tabular-nums">
        {l.cantidad}
        {l.unidad_medida && <span className="ml-1 text-xs text-content-muted">{l.unidad_medida}</span>}
      </TD>
      <TD className="text-right tabular-nums">{formatMonto(l.precio_unitario)}</TD>
      <TD className="text-right tabular-nums">
        {descuento > 0 ? (
          <>
            {formatMonto(String(descuento))}
            {/* La naturaleza la escribió el emisor: no es una traducción nuestra. */}
            {l.descuentos[0]?.naturaleza && (
              <p className="text-[11px] text-content-muted">{l.descuentos[0].naturaleza}</p>
            )}
          </>
        ) : (
          <span className="text-content-muted">—</span>
        )}
      </TD>
      <TD className="text-right tabular-nums">{formatMonto(l.subtotal)}</TD>
      <TD>
        {l.impuestos.length === 0 ? (
          <span className="text-content-muted">—</span>
        ) : (
          l.impuestos.map((i, k) => (
            <p key={k} className="whitespace-nowrap text-xs tabular-nums text-content">
              {i.tarifa !== "" ? `${i.tarifa} %` : `cód. ${i.codigo}`}
              <span className="ml-1 text-content-muted">{formatMonto(i.monto)}</span>
            </p>
          ))
        )}
      </TD>
      <TD className="text-right font-medium tabular-nums">{formatMonto(l.monto_total_linea)}</TD>
    </TR>
  );
}

function Parte(props: { titulo: string; nombre: string; id: string }) {
  return (
    <div className="rounded-lg border border-border bg-surface-muted px-3 py-2">
      <p className="text-[11px] font-bold uppercase tracking-wider text-content-muted">{props.titulo}</p>
      <p className="mt-1 font-medium text-content">{props.nombre || "—"}</p>
      {props.id && <p className="font-mono text-xs text-content-muted">{props.id}</p>}
    </div>
  );
}

function Dato({ rotulo, valor }: { rotulo: string; valor: string }) {
  return (
    <span>
      <span className="text-content-muted">{rotulo}: </span>
      <span className="text-content">{valor}</span>
    </span>
  );
}

/**
 * Un renglón de totales. Se oculta cuando el elemento NO VINO (cadena vacía) o vale cero.
 *
 * Se compara el STRING, no el número: `toNumber("")` y `toNumber("0")` dan lo mismo, y entonces no
 * habría forma de distinguir «el comprobante no lo declara» de «vale cero».
 */
function Renglon({ rotulo, valor, negativo }: { rotulo: string; valor: string; negativo?: boolean }) {
  if (valor === "" || /^0+([.,]0+)?$/.test(valor.trim())) return null;
  return (
    <div className="flex items-baseline justify-between py-0.5 text-sm">
      <span className="text-content-muted">{rotulo}</span>
      <span className="tabular-nums text-content">
        {negativo && "−"}
        {formatMonto(valor)}
      </span>
    </div>
  );
}
