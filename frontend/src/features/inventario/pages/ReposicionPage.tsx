/**
 * Pantalla — Reposición (/inventario/reposicion).
 *
 * Qué hay que pedir, **agrupado por proveedor** porque se pide por proveedor.
 *
 * La columna «de dónde sale» es la razón de ser de la pantalla: una cantidad sugerida sin la frase
 * que la explica es una orden que nadie puede discutir. Y cuando todavía no hay historia suficiente
 * de servicios, la frase lo dice en vez de inventar un consumo.
 */

import { useMemo, useState } from "react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  EmptyState,
  ErrorState,
  Input,
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
} from "@/components/ui";
import { formatMoneda, formatMonto } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useSedes } from "@/features/bancos/hooks";
import { useReposicion } from "@/features/inventario/hooks";
import type { SugerenciaPedido } from "@/api/inventario";

export function ReposicionPage() {
  const sedes = useSedes();
  const [sedeID, setSedeID] = useState("");
  const [semanas, setSemanas] = useState("8");

  const nSemanas = Math.max(1, Math.min(52, Number(semanas) || 8));
  const q = useReposicion(sedeID, nSemanas);
  const filas = q.data ?? [];

  // Agrupado por proveedor: es el criterio con el que se arma un pedido.
  const porProveedor = useMemo(() => {
    const mapa = new Map<string, SugerenciaPedido[]>();
    for (const f of filas) {
      const clave = f.proveedor || "(sin proveedor asignado)";
      mapa.set(clave, [...(mapa.get(clave) ?? []), f]);
    }
    return [...mapa.entries()].sort((a, b) => a[0].localeCompare(b[0], "es"));
  }, [filas]);

  const totalCosto = filas.reduce((a, f) => a + Number(f.costo_estimado_crc || 0), 0);
  const totalPiezas = filas.reduce((a, f) => a + f.sugerido, 0);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Reposición"
        description="Qué pedir esta semana, agrupado por proveedor."
      />

      <Card>
        <CardContent className="flex flex-wrap items-start gap-4 pt-6">
          <div className="w-52">
            <Select
              label="Sede"
              value={sedeID}
              onChange={(e) => setSedeID(e.target.value)}
              options={[
                { value: "", label: "Todas las sedes" },
                ...(sedes.data ?? []).map((s) => ({ value: s.id, label: s.nombre })),
              ]}
            />
          </div>
          <div className="w-44">
            <Input
              label="Semanas a mirar"
              value={semanas}
              onChange={(e) => setSemanas(e.target.value)}
              inputMode="numeric"
            />
          </div>
        </CardContent>
      </Card>

      {q.isLoading && <LoadingState label="Viendo qué falta…" />}
      {q.isError && <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />}

      {q.data && filas.length === 0 && (
        <EmptyState message="Nada está en o por debajo de su mínimo. Si esperabas ver algo acá, revisá que los mínimos estén fijados en el catálogo." />
      )}

      {filas.length > 0 && (
        <>
          <div className="grid gap-4 sm:grid-cols-3">
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Líneas a pedir</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {filas.length}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  en {porProveedor.length} proveedor(es)
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Piezas sugeridas</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {totalPiezas}
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Costo estimado</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {formatMoneda(String(totalCosto))}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">al costo promedio conocido</p>
              </CardContent>
            </Card>
          </div>

          {porProveedor.map(([proveedor, lineas]) => (
            <Card key={proveedor}>
              <CardHeader>
                <CardTitle>{proveedor}</CardTitle>
                <p className="mt-1 text-xs text-content-muted">
                  {lineas.length} línea(s) ·{" "}
                  {formatMoneda(
                    String(lineas.reduce((a, l) => a + Number(l.costo_estimado_crc || 0), 0)),
                  )}
                </p>
              </CardHeader>
              <CardContent>
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Artículo</TH>
                        <TH>Sede</TH>
                        <TH className="text-right">Hay</TH>
                        <TH className="text-right">Mín</TH>
                        <TH className="text-right">Máx</TH>
                        <TH className="text-right">Consumo</TH>
                        <TH className="text-right">Sugerido</TH>
                        <TH>De dónde sale</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {lineas.map((l) => (
                        <TR key={`${l.articulo_id}-${l.sede_id}`}>
                          <TD>
                            <span className="font-medium text-content">{l.articulo}</span>
                            <span className="block text-xs text-content-muted">{l.codigo}</span>
                          </TD>
                          <TD className="text-content-muted">{l.sede}</TD>
                          <TD className="text-right font-medium tabular-nums">{l.hay}</TD>
                          <TD className="text-right tabular-nums text-content-muted">{l.minimo}</TD>
                          <TD className="text-right tabular-nums text-content-muted">
                            {l.maximo === 0 ? "—" : l.maximo}
                          </TD>
                          <TD className="text-right tabular-nums text-content-muted">
                            {l.consumo_semanal === ""
                              ? "sin datos"
                              : `${Number(l.consumo_semanal).toFixed(1)} / sem`}
                          </TD>
                          {/* El costo se muestra como cantidad × costo unitario: así se ve que los
                              dos números de la fila hablan de lo mismo. */}
                          <TD className="text-right font-medium tabular-nums">
                            {l.sugerido}
                            <span className="block text-xs font-normal text-content-muted">
                              × {formatMonto(l.costo_unitario_crc)} ={" "}
                              {formatMonto(l.costo_estimado_crc)}
                            </span>
                          </TD>
                          <TD className="max-w-sm whitespace-normal text-xs text-content-muted">
                            {l.de_donde_sale}
                          </TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              </CardContent>
            </Card>
          ))}

          <p className="text-xs text-content-muted">
            El sistema propone; la decisión de pedir es de una persona. Acá no se envía nada al
            proveedor: eso sigue por el canal de siempre.
          </p>
        </>
      )}
    </div>
  );
}
