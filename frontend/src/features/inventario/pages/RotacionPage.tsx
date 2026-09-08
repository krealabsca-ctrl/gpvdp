/**
 * Pantalla — Rotación y capital detenido (/inventario/rotacion).
 *
 * En este negocio hay que tener surtido, y el surtido inmoviliza plata. La pantalla dice cuánta y
 * dónde, para poder decidir qué modelo dejar de reponer.
 *
 * «Días de stock» pesa más que la rotación: **«tres años de stock» se entiende y «0,3×» no**. Y las
 * unidades quietas van con nombre y apellido, porque el capital detenido no se ve en un total.
 */

import { useState } from "react";
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
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatMoneda, formatMonto } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useRotacion, useUnidades } from "@/features/inventario/hooks";
import { MarcaRotacion } from "@/features/inventario/componentes";

/** Días sin movimiento a partir de los cuales una unidad se considera detenida. */
const DIAS_QUIETA = 90;

function haceUnAnio(): string {
  const d = new Date();
  d.setFullYear(d.getFullYear() - 1);
  return d.toISOString().slice(0, 10);
}
function hoyCR(): string {
  const d = new Date();
  const cr = new Date(d.getTime() - (d.getTimezoneOffset() + 360) * 60_000);
  return cr.toISOString().slice(0, 10);
}

export function RotacionPage() {
  const [desde, setDesde] = useState(haceUnAnio());
  const [hasta, setHasta] = useState(hoyCR());

  const q = useRotacion(desde, hasta);
  const quietas = useUnidades({ quietas_desde_dias: DIAS_QUIETA, limite: 100 });

  const filas = q.data?.filas ?? [];
  const detenidas = quietas.data ?? [];
  const capitalTotal = filas.reduce((a, f) => a + Number(f.capital_crc || 0), 0);
  const capitalQuieto = detenidas.reduce((a, u) => a + Number(u.costo_crc || 0), 0);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Rotación y capital detenido"
        description="Qué se mueve, qué está parado y cuánta plata hay quieta."
        actions={
          <div className="flex flex-wrap items-start gap-3">
            <div className="w-40">
              <Input
                label="Desde"
                type="date"
                value={desde}
                onChange={(e) => setDesde(e.target.value)}
              />
            </div>
            <div className="w-40">
              <Input
                label="Hasta"
                type="date"
                value={hasta}
                onChange={(e) => setHasta(e.target.value)}
              />
            </div>
          </div>
        }
      />

      {q.isLoading && <LoadingState label="Midiendo la rotación…" />}
      {q.isError && <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />}

      {q.data && (
        <>
          <div className="grid gap-4 sm:grid-cols-3">
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Capital detenido</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {formatMoneda(String(capitalTotal))}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">al costo, en todas las sedes</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Parado más de {DIAS_QUIETA} días</p>
                <p
                  className={cn(
                    "mt-1 text-2xl font-semibold tabular-nums",
                    capitalQuieto > 0 ? "text-negativo" : "text-content",
                  )}
                >
                  {formatMoneda(String(capitalQuieto))}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  {detenidas.length} unidad(es) sin movimiento
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <p className="text-sm text-content-muted">Artículos con existencia</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-content">
                  {filas.length}
                </p>
                <p className="mt-0.5 text-xs text-content-muted">
                  {filas.filter((f) => f.lectura === "DETENIDO" || f.lectura === "SIN_SALIDAS").length}{" "}
                  sin movimiento en el rango
                </p>
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Por artículo</CardTitle>
              <p className="mt-1 text-xs text-content-muted">
                La rotación está anualizada, para que rangos de distinto largo se puedan comparar.
              </p>
            </CardHeader>
            <CardContent>
              {filas.length === 0 ? (
                <EmptyState message="No hay existencias ni salidas en el rango elegido." />
              ) : (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Artículo</TH>
                        <TH className="text-right">En stock</TH>
                        <TH className="text-right">Salidas</TH>
                        <TH className="text-right">Rotación</TH>
                        <TH className="text-right">Días de stock</TH>
                        <TH className="text-right">Capital</TH>
                        <TH>Lectura</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {filas.map((f) => (
                        <TR key={f.articulo_id}>
                          <TD>
                            <span className="font-medium text-content">{f.articulo}</span>
                            <span className="block text-xs text-content-muted">
                              {f.codigo} · {f.categoria}
                            </span>
                          </TD>
                          <TD className="text-right tabular-nums">{f.en_stock}</TD>
                          <TD className="text-right tabular-nums text-content-muted">
                            {f.salidas}
                          </TD>
                          <TD className="text-right tabular-nums">
                            {f.rotacion === "" ? "—" : `${f.rotacion}×`}
                          </TD>
                          <TD className="text-right tabular-nums text-content-muted">
                            {f.dias_de_stock === "" ? "—" : f.dias_de_stock}
                          </TD>
                          <TD className="text-right font-medium tabular-nums">
                            {formatMonto(f.capital_crc)}
                          </TD>
                          <TD>
                            <MarcaRotacion lectura={f.lectura} />
                          </TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Unidades sin movimiento hace más de {DIAS_QUIETA} días</CardTitle>
              <p className="mt-1 text-xs text-content-muted">
                Cada una con su número: es lo que permite decidir si se traslada a una sede donde sí
                se piden, se pasa a exhibición o se deja de reponer el modelo.
              </p>
            </CardHeader>
            <CardContent>
              {quietas.isLoading && <LoadingState label="Buscando lo que está quieto…" />}
              {quietas.data && detenidas.length === 0 && (
                <EmptyState message="Nada lleva más de 90 días quieto." />
              )}
              {detenidas.length > 0 && (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Unidad</TH>
                        <TH>Artículo</TH>
                        <TH>Sede</TH>
                        <TH className="text-right">Días quieta</TH>
                        <TH className="text-right">Costo</TH>
                        <TH>Ingresó</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {detenidas.map((u) => (
                        <TR key={u.id}>
                          <TD className="font-medium tabular-nums text-content">{u.numero}</TD>
                          <TD className="text-content-muted">{u.articulo}</TD>
                          <TD className="text-content-muted">{u.sede || "—"}</TD>
                          <TD className="text-right font-medium tabular-nums text-negativo">
                            {u.dias_quieta}
                          </TD>
                          <TD className="text-right tabular-nums">{formatMonto(u.costo_crc)}</TD>
                          <TD className="tabular-nums text-content-muted">{u.ingresada_en}</TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
