/**
 * Drivers del negocio (/drivers): la cartera por lo que la mueve.
 *
 * ── POR QUÉ ─────────────────────────────────────────────────────────────────────────────────
 *
 * Todo el análisis del sistema es por partida contable: cuánto entró por Datáfonos, cuánto por
 * Asociaciones. Eso dice CÓMO entró la plata, no POR QUÉ. Sin drivers, presupuestar es «el año
 * pasado más un diez por ciento», porque no hay con qué explicar de dónde saldría ese diez.
 *
 * ── LA DISTINCIÓN QUE SOSTIENE LA PANTALLA ──────────────────────────────────────────────────
 *
 * CONTRATADO no es FACTURADO. La suma de las cuotas es lo que debería entrar al mes; lo que
 * generó cargo es lo que se puso a cobrar. Mostrar solo el primero daría un recurrente que no
 * corresponde a nada y se vería perfectamente razonable.
 */

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";

import {
  Badge,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ErrorState,
  LoadingState,
  PageHeader,
  Select,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { mensajeError } from "@/lib/apiError";
import { formatMoneda, formatMonto } from "@/lib/format";
import { queryKeys } from "@/api/queryKeys";
import { getDrivers, type DimensionDriver, type FilaDriver } from "@/api/ratios";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

const DIMENSIONES: { value: DimensionDriver; label: string }[] = [
  { value: "tipo_servicio", label: "Tipo de servicio" },
  { value: "modalidad", label: "Modalidad" },
  { value: "sede", label: "Sede" },
];

export function DriversPage() {
  const empresaId = useEmpresaId();
  const [dimension, setDimension] = useState<DimensionDriver>("tipo_servicio");
  const [meses, setMeses] = useState(12);

  const q = useQuery({
    queryKey: queryKeys.ratios.drivers(empresaId, dimension, meses),
    queryFn: () => getDrivers(dimension, meses),
  });
  const d = q.data;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Drivers del negocio"
        description="La cartera por lo que la mueve: cuántos contratos, de qué tipo y con qué cuota."
        actions={
          <div className="flex flex-wrap items-end gap-2">
            <Select
              label="Abrir por"
              id="drivers-dimension"
              value={dimension}
              onChange={(e) => setDimension(e.target.value as DimensionDriver)}
              options={DIMENSIONES}
            />
            <Select
              label="Altas de los últimos"
              id="drivers-meses"
              value={String(meses)}
              onChange={(e) => setMeses(Number(e.target.value))}
              options={[
                { value: "6", label: "6 meses" },
                { value: "12", label: "12 meses" },
                { value: "24", label: "24 meses" },
              ]}
            />
          </div>
        }
      />

      {q.isPending ? (
        <LoadingState label="Midiendo la cartera" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : !d ? null : (
        <>
          {d.avisos.length > 0 && (
            <div className="flex flex-col gap-2 rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
              {d.avisos.map((a, i) => (
                <p key={i} className="text-sm text-content-muted">
                  {a}
                </p>
              ))}
            </div>
          )}

          {/* LOS DOS NÚMEROS, SIEMPRE JUNTOS. Separarlos es lo que haría que el contratado se
              lea como el ingreso del mes. */}
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Dato
              titulo="Contratos activos"
              valor={d.contratos_activos.toLocaleString("es-CR")}
              pie={`ticket recurrente ${formatMoneda(d.cuota_promedio)}`}
            />
            <Dato
              titulo="Cuota contratada"
              valor={formatMoneda(d.cuota_contratada)}
              pie="lo que debería entrar al mes"
            />
            <Dato
              titulo={`Facturado en ${d.periodo_facturado}`}
              valor={formatMoneda(d.cuota_facturada)}
              pie={`${d.contratos_facturados.toLocaleString("es-CR")} contrato(s) con cargo`}
            />
            <div>
              <p className="text-xs text-content-muted">Cobertura</p>
              <p
                className={cn(
                  "text-xl font-semibold tabular-nums",
                  !d.cobertura_pct
                    ? "text-content-muted"
                    : Number(d.cobertura_pct) < 90
                      ? "text-negativo"
                      : "text-positivo",
                )}
              >
                {d.cobertura_pct ? `${d.cobertura_pct} %` : "—"}
              </p>
              <p className="text-xs text-content-muted">de los activos generó cargo</p>
            </div>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>
                El mix · por {DIMENSIONES.find((x) => x.value === dimension)?.label.toLowerCase()}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <TablaMix filas={d.filas} />
            </CardContent>
          </Card>

          {d.altas.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>Altas por mes</CardTitle>
              </CardHeader>
              <CardContent>
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Mes</TH>
                        <TH className="text-right">Contratos nuevos</TH>
                        <TH className="text-right">Ticket</TH>
                        <TH className="text-right">Recurrente que sumaron</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {d.altas.map((a) => (
                        <TR key={a.periodo}>
                          <TD className="tabular-nums text-content">{a.periodo}</TD>
                          <TD className="text-right tabular-nums text-content">{a.altas}</TD>
                          <TD className="text-right tabular-nums text-content-muted">
                            {formatMonto(a.cuota_promedio)}
                          </TD>
                          <TD className="text-right tabular-nums text-content">
                            {formatMonto(a.cuota_incorporada)}
                          </TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              </CardContent>
            </Card>
          )}
        </>
      )}
    </div>
  );
}

function Dato({ titulo, valor, pie }: { titulo: string; valor: string; pie: string }) {
  return (
    <div>
      <p className="text-xs text-content-muted">{titulo}</p>
      <p className="text-xl font-semibold tabular-nums text-content">{valor}</p>
      <p className="text-xs text-content-muted">{pie}</p>
    </div>
  );
}

/** El mix: cada corte con su peso, su ticket y su cobertura. */
function TablaMix({ filas }: { filas: FilaDriver[] }) {
  return (
    <TableContainer>
      <Table>
        <THead>
          <TR>
            <TH>Corte</TH>
            <TH className="text-right">Contratos</TH>
            <TH className="text-right">Ticket</TH>
            <TH className="text-right">Cuota contratada</TH>
            <TH className="text-right">Peso</TH>
            <TH className="text-right">Cobertura</TH>
          </TR>
        </THead>
        <TBody>
          {filas.map((f) => (
            <TR key={f.clave || f.nombre}>
              <TD className="text-content">{f.nombre}</TD>
              <TD className="text-right tabular-nums text-content">
                {f.contratos.toLocaleString("es-CR")}
              </TD>
              <TD className="text-right tabular-nums text-content-muted">
                {formatMonto(f.cuota_promedio)}
              </TD>
              <TD className="text-right tabular-nums text-content">
                {formatMonto(f.cuota_contratada)}
              </TD>
              <TD className="text-right tabular-nums text-content-muted">
                {f.aporte_pct ? `${f.aporte_pct} %` : "—"}
              </TD>
              <TD className="text-right">
                {f.cobertura_pct ? (
                  <Badge tone={Number(f.cobertura_pct) >= 90 ? "positivo" : "pendiente"}>
                    {f.cobertura_pct} %
                  </Badge>
                ) : (
                  <span className="text-xs text-content-muted">—</span>
                )}
              </TD>
            </TR>
          ))}
        </TBody>
      </Table>
    </TableContainer>
  );
}
