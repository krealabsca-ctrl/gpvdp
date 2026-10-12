/**
 * Qué se compró en ESTA factura, línea por línea.
 *
 * De dónde sale: del XML de Hacienda, que declara por línea el código CABYS, la descripción, la
 * cantidad, la unidad y el precio unitario. El resto de CxP trabaja con el TOTAL de la factura;
 * esto es lo único que dice qué hay adentro.
 *
 * NO SE PINTA NADA SI NO HAY DETALLE, a propósito. Casi todas las facturas del sistema entraron
 * por el importador de Excel y traen solo el encabezado: una tarjeta vacía en cada una de las
 * 4.533 fichas sería ruido permanente. Cuando el XML se carga, la tarjeta aparece sola.
 */
import { useQuery } from "@tanstack/react-query";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
} from "@/components/ui";
import { gastoApi } from "@/api/cxp";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { formatMonto } from "@/lib/format";

export function DetalleLineasCard({ documentoId }: { documentoId: string }) {
  const empresaId = useEmpresaId();
  const { data } = useQuery({
    queryKey: ["cxp", empresaId, "documento-lineas", documentoId],
    queryFn: () => gastoApi.lineasDeDocumento(documentoId),
    enabled: !!empresaId && !!documentoId,
  });

  const lineas = data?.lineas ?? [];
  if (lineas.length === 0) return null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Detalle de la factura</CardTitle>
        <CardDescription>
          {lineas.length} línea(s) del XML. Los montos van en la moneda de la factura, tal como el
          proveedor las declaró ante Hacienda.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH className="w-10 text-right">#</TH>
                <TH>Descripción</TH>
                <TH className="text-right">Cantidad</TH>
                <TH className="text-right">Precio unit.</TH>
                <TH className="text-right">Descuento</TH>
                <TH className="text-right">Impuesto</TH>
                <TH className="text-right">Total</TH>
              </TR>
            </THead>
            <TBody>
              {lineas.map((l) => (
                <TR key={l.linea}>
                  <TD className="text-right text-content-muted">{l.linea}</TD>
                  <TD>
                    <div>{l.detalle || "(sin descripción)"}</div>
                    <div className="text-xs text-content-muted">
                      {l.cabys ? `CABYS ${l.cabys}` : "sin CABYS"}
                      {l.codigo_comercial && <> · cód. {l.codigo_comercial}</>}
                    </div>
                  </TD>
                  <TD className="text-right tabular-nums">
                    {/* Cadena vacía = el comprobante NO trajo el dato; un 0 diría que compró cero. */}
                    {l.cantidad ? formatMonto(l.cantidad) : "—"}
                    {l.unidad_medida && (
                      <span className="text-xs text-content-muted"> {l.unidad_medida}</span>
                    )}
                  </TD>
                  <TD className="text-right tabular-nums">
                    {l.precio_unitario ? formatMonto(l.precio_unitario) : "—"}
                  </TD>
                  <TD className="text-right tabular-nums">
                    {l.descuento ? formatMonto(l.descuento) : "—"}
                  </TD>
                  <TD className="text-right tabular-nums">
                    {l.impuesto ? formatMonto(l.impuesto) : "—"}
                  </TD>
                  <TD className="text-right tabular-nums font-medium">
                    {l.total ? formatMonto(l.total) : "—"}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      </CardContent>
    </Card>
  );
}
