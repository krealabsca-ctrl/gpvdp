/**
 * El papel de respaldo del pago: el cuadro que se manda cuando el corte se aprueba (mig 0117).
 *
 * «Cuando se aprueba se envía algo como esto para un control y respaldo de lo pagado.»
 *
 * Reemplaza un Excel que se armaba a mano por corte. Dos cosas lo hacen distinto de ese Excel:
 *
 *   1. SALE DE LAS MISMAS FILAS QUE EL ARCHIVO DEL BANCO. El papel y la transferencia no pueden
 *      decir cosas distintas porque son el mismo dato.
 *   2. LAS FIRMAS NO SE TECLEAN. Salen de quién hizo cada paso en el sistema; si un paso no se
 *      hizo, la línea sale en blanco en vez de inventar un nombre.
 *
 * El concepto y la glosa SÍ se corrigen acá, antes de descargar: el sistema propone lo que sabe
 * (el rubro del catálogo, los consecutivos de las facturas) y una persona lo afina. Bajar el
 * archivo, abrirlo, ver que el concepto está mal y volver al sistema es el viaje que esto evita.
 */
import { useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  ErrorState,
  Input,
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
import { cxpApi, type AvisoDeMonedaCorte, type FilaAvisoCorte } from "@/api/cxp";
import { useAvisoDeCorte, useGuardarTextoAviso } from "@/features/cxp/hooks";
import { mensajeError } from "@/lib/apiError";
import { descargarBlob } from "@/lib/csv";
import { formatFecha, formatMoneda } from "@/lib/format";

/** Una casilla que guarda al salir, solo si cambió. */
function CasillaTexto({
  valor,
  onGuardar,
  placeholder,
  etiqueta,
}: {
  valor: string;
  onGuardar: (v: string) => void;
  placeholder: string;
  etiqueta: string;
}) {
  const [texto, setTexto] = useState(valor);
  return (
    <Input
      value={texto}
      onChange={(e) => setTexto(e.target.value)}
      // Guarda al SALIR del campo y solo si cambió: guardar en cada tecla mandaría una petición
      // por letra, y guardar siempre al salir escribiría aunque nadie tocara nada.
      onBlur={() => {
        if (texto !== valor) onGuardar(texto);
      }}
      placeholder={placeholder}
      aria-label={etiqueta}
      className="min-w-40"
    />
  );
}

function TablaDeMoneda({
  m,
  loteId,
  onDescargar,
  descargando,
}: {
  m: AvisoDeMonedaCorte;
  loteId: string;
  onDescargar: (moneda: string) => void;
  descargando: boolean;
}) {
  const toast = useToast();
  const guardar = useGuardarTextoAviso(loteId);

  function guardarTexto(fila: FilaAvisoCorte, campo: "concepto" | "documentos", v: string) {
    guardar.mutate(
      { proveedorId: fila.proveedor_id, [campo]: v },
      { onError: (e) => toast.error(mensajeError(e)) },
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="text-sm font-medium">
            {m.moneda === "USD" ? "Pago Proveedores Dólares" : "Pago Proveedores Colones"}
          </h3>
          <span className="text-sm text-content-muted">
            {m.filas.length} proveedor(es) · {m.transferencias} transferencia(s)
          </span>
          {/* Lo único del papel que hay que arreglar ANTES de mandarlo: el banco rechaza esa
              línea. Por eso va arriba y no como una celda más de la tabla. */}
          {m.sin_cuenta > 0 && (
            <Badge tone="negativo">
              {m.sin_cuenta} sin cuenta — el banco las rechaza
            </Badge>
          )}
        </div>
        <Button onClick={() => onDescargar(m.moneda)} disabled={descargando}>
          Descargar {m.moneda}
        </Button>
      </div>

      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH>Proveedor</TH>
              <TH>Moneda</TH>
              <TH>Concepto</TH>
              <TH>Documentos</TH>
              <TH className="text-right">Monto</TH>
              <TH>Cuenta bancaria</TH>
              <TH>Identificación</TH>
            </TR>
          </THead>
          <TBody>
            {m.filas.map((fa) => (
              <TR key={`${fa.proveedor_id}-${fa.moneda}`}>
                <TD className="font-medium">{fa.proveedor}</TD>
                <TD>{fa.moneda === "USD" ? "Dólares" : "Colones"}</TD>
                <TD>
                  <CasillaTexto
                    valor={fa.concepto}
                    onGuardar={(v) => guardarTexto(fa, "concepto", v)}
                    placeholder="¿Qué se le paga?"
                    etiqueta={`Concepto de ${fa.proveedor}`}
                  />
                </TD>
                <TD>
                  <CasillaTexto
                    valor={fa.documentos}
                    onGuardar={(v) => guardarTexto(fa, "documentos", v)}
                    placeholder="FC 123 456"
                    etiqueta={`Documentos de ${fa.proveedor}`}
                  />
                </TD>
                <TD className="text-right tabular-nums">{formatMoneda(fa.monto, fa.moneda === "USD" ? "USD" : "CRC")}</TD>
                <TD>
                  {fa.sin_cuenta ? (
                    <Badge tone="negativo">Sin cuenta registrada</Badge>
                  ) : (
                    <span className="text-xs text-content-muted">{fa.cuenta}</span>
                  )}
                </TD>
                <TD className="text-content-muted">{fa.identificacion}</TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>

      {/* El total es lo que se compara contra el archivo que se sube al banco. */}
      <div className="flex justify-end gap-3 border-t border-border pt-2 text-sm">
        <span className="font-medium">
          Total a transferir en {m.moneda === "USD" ? "dólares" : "colones"}
        </span>
        <span className="tabular-nums font-semibold">{formatMoneda(m.total, m.moneda === "USD" ? "USD" : "CRC")}</span>
      </div>
    </div>
  );
}

export function AvisoDeCortePanel({ loteId }: { loteId: string }) {
  const toast = useToast();
  const q = useAvisoDeCorte(loteId);
  const [descargando, setDescargando] = useState(false);

  async function descargar(moneda: string) {
    setDescargando(true);
    try {
      const blob = await cxpApi.descargarAvisoXLSX(loteId, moneda);
      const num = String(q.data?.lote_numero ?? 0).padStart(4, "0");
      descargarBlob(`aviso-pago-corte-${num}-${moneda}.xlsx`, blob);
    } catch (e) {
      toast.error(mensajeError(e));
    } finally {
      setDescargando(false);
    }
  }

  const av = q.data;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Respaldo del pago</CardTitle>
        <CardDescription>
          El cuadro que se manda cuando el corte se aprueba. Sale de las{" "}
          <strong>mismas filas que el archivo del banco</strong>, así que el papel y la
          transferencia no pueden decir cosas distintas. El concepto y los documentos se corrigen
          acá antes de descargarlo.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {q.isPending && <LoadingState label="Armando el respaldo" />}
        {q.isError && <ErrorState message={mensajeError(q.error)} />}

        {av && av.monedas.length === 0 && (
          <p className="py-6 text-center text-sm text-content-muted">
            Este corte no tiene nada que pagar todavía.
          </p>
        )}

        {av && av.monedas.length > 0 && (
          <>
            {/* LAS FIRMAS VAN ARRIBA, como en el papel que se firma: es lo primero que mira quien
                lo recibe para saber por cuántas manos pasó este pago. */}
            <div className="rounded-md border border-border p-4">
              <div className="grid gap-2 sm:grid-cols-2">
                {av.firmas.map((fi) => (
                  <div key={fi.rol} className="flex gap-2 text-sm">
                    <span className="min-w-32 font-medium">{fi.rol}:</span>
                    {fi.nombre ? (
                      <span>
                        {fi.nombre}
                        {fi.fecha && (
                          <span className="ml-2 text-xs text-content-muted">
                            {formatFecha(fi.fecha)}
                          </span>
                        )}
                      </span>
                    ) : (
                      // En blanco con su aviso: un hueco sin explicar se lee como un olvido del
                      // sistema, cuando lo que dice es que ese paso no se hizo.
                      <span className="text-content-muted">— sin registrar en el sistema</span>
                    )}
                  </div>
                ))}
                <div className="flex gap-2 text-sm">
                  <span className="min-w-32 font-medium">Fecha de pago:</span>
                  {av.fecha_pago ? (
                    <span>{formatFecha(av.fecha_pago)}</span>
                  ) : (
                    <span className="text-content-muted">— el corte no se ha cerrado</span>
                  )}
                </div>
              </div>
              <p className="mt-3 text-xs text-content-muted">
                Estos nombres no se escriben: salen de quién hizo cada paso en el sistema.
              </p>
            </div>

            {av.monedas.map((m) => (
              <TablaDeMoneda
                key={m.moneda}
                m={m}
                loteId={loteId}
                onDescargar={descargar}
                descargando={descargando}
              />
            ))}
          </>
        )}
      </CardContent>
    </Card>
  );
}
