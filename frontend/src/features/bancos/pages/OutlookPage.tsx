/**
 * Outlook del año (/outlook): la tercera columna.
 *
 * ── QUÉ CONTESTA ────────────────────────────────────────────────────────────────────────────
 *
 * «¿Cómo vamos a cerrar el año?». Con Presupuesto y Real solo se sabe dónde se está; con el
 * outlook, dónde se va a terminar.
 *
 * ── POR QUÉ DOS ESCENARIOS LADO A LADO ──────────────────────────────────────────────────────
 *
 * Porque «lo que falta del año» admite dos respuestas legítimas y opuestas: que los meses
 * restantes salgan como se presupuestaron (PLAN) o como vienen saliendo (RITMO). Mostrar uno solo
 * elegiría por el usuario; mostrar los dos convierte la brecha en el dato — si el plan promete
 * mucho más que el ritmo, el plan ya no describe el año.
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
import { getOutlook, type EscenarioOutlook, type MesDelAnio } from "@/api/bancos";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";

const MES_CORTO = [
  "", "Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul", "Ago", "Sep", "Oct", "Nov", "Dic",
];

/** "2026-03" → "Mar". */
function mesDe(periodo: string): string {
  return MES_CORTO[Number(periodo.slice(5))] ?? periodo;
}

export function OutlookPage() {
  const empresaId = useEmpresaId();
  const anioActual = new Date().getFullYear();
  const [anio, setAnio] = useState(anioActual);

  const q = useQuery({
    queryKey: queryKeys.bancos.outlook(empresaId, anio),
    queryFn: () => getOutlook(anio),
  });
  const o = q.data;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Cómo cierra el año"
        description="Lo que va, lo que se presupuestó, y dónde se va a terminar."
        actions={
          <Select
            label="Año"
            id="outlook-anio"
            value={String(anio)}
            onChange={(e) => setAnio(Number(e.target.value))}
            options={[anioActual + 1, anioActual, anioActual - 1, anioActual - 2].map((a) => ({
              value: String(a),
              label: String(a),
            }))}
          />
        }
      />

      {q.isPending ? (
        <LoadingState label="Proyectando el cierre del año" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : !o ? null : (
        <>
          {o.avisos.length > 0 && (
            <div className="flex flex-col gap-2 rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
              {o.avisos.map((a, i) => (
                <p key={i} className="text-sm text-content-muted">
                  {a}
                </p>
              ))}
            </div>
          )}

          {/* LAS TRES COLUMNAS. El presupuesto es lo declarado; lo que va, lo medido; y los dos
              escenarios, las dos formas de completar el año. */}
          <div className="grid gap-4 lg:grid-cols-4">
            <Columna
              titulo="Presupuesto del año"
              subtitulo="lo que se declaró"
              ingreso={o.ingreso_presupuesto}
              gasto={o.gasto_presupuesto}
              resultado={o.resultado_presupuesto}
            />
            <Columna
              titulo="Lo que va"
              subtitulo={`${o.meses_cerrados} mes(es) cerrados${o.mes_en_curso ? " + lo que lleva " + mesDe(o.mes_en_curso) : ""}`}
              ingreso={o.ingreso_ytd}
              gasto={o.gasto_ytd}
              resultado={o.resultado_ytd}
            />
            <ColumnaEscenario titulo="Si se cumple el plan" e={o.plan} />
            <ColumnaEscenario titulo="Si seguimos como vamos" e={o.ritmo} destacada />
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Mes a mes</CardTitle>
            </CardHeader>
            <CardContent>
              <TablaMeses meses={o.meses} />
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}

/** Una columna del encabezado: ingreso, gasto y resultado. */
function Columna({
  titulo,
  subtitulo,
  ingreso,
  gasto,
  resultado,
  tenue,
}: {
  titulo: string;
  subtitulo: string;
  ingreso: string;
  gasto: string;
  resultado: string;
  tenue?: boolean;
}) {
  return (
    <Card>
      <CardContent className="flex flex-col gap-3 pt-5">
        <div>
          <p className="text-sm font-medium text-content">{titulo}</p>
          <p className="text-xs text-content-muted">{subtitulo}</p>
        </div>
        <div className={cn("flex flex-col gap-1 text-sm", tenue && "opacity-60")}>
          <Renglon etiqueta="Ingreso" valor={ingreso} />
          <Renglon etiqueta="Gasto" valor={gasto} />
          <div className="mt-1 border-t border-border pt-1">
            <p className="text-xs text-content-muted">Resultado</p>
            <p
              className={cn(
                "text-lg font-semibold tabular-nums",
                resultado.startsWith("-") ? "text-negativo" : "text-content",
              )}
            >
              {formatMoneda(resultado)}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function Renglon({ etiqueta, valor }: { etiqueta: string; valor: string }) {
  return (
    <div className="flex justify-between gap-2">
      <span className="text-content-muted">{etiqueta}</span>
      <span className="tabular-nums text-content">{formatMonto(valor)}</span>
    </div>
  );
}

/** Un escenario, con su base a la vista o su motivo de no poder calcularse. */
function ColumnaEscenario({
  titulo,
  e,
  destacada,
}: {
  titulo: string;
  e: EscenarioOutlook;
  destacada?: boolean;
}) {
  if (!e.calculable) {
    return (
      <Card>
        <CardContent className="flex flex-col gap-2 pt-5">
          <p className="text-sm font-medium text-content">{titulo}</p>
          {/* NO SE MUESTRA EL NÚMERO cuando no se puede calcular: un cierre de año en cero se lee
              como un cierre de año, no como la falta de un dato. */}
          <p className="text-sm text-content-muted">{e.aviso}</p>
        </CardContent>
      </Card>
    );
  }
  return (
    <Card className={cn(destacada && "border-accent/50")}>
      <CardContent className="flex flex-col gap-3 pt-5">
        <div>
          <p className="text-sm font-medium text-content">{titulo}</p>
          <p className="text-xs text-content-muted">{e.base}</p>
        </div>
        <div className="flex flex-col gap-1 text-sm">
          <Renglon etiqueta="Ingreso" valor={e.ingreso} />
          <Renglon etiqueta="Gasto" valor={e.gasto} />
          <div className="mt-1 border-t border-border pt-1">
            <p className="text-xs text-content-muted">Cierra en</p>
            <p
              className={cn(
                "text-lg font-semibold tabular-nums",
                e.resultado.startsWith("-") ? "text-negativo" : "text-positivo",
              )}
            >
              {formatMoneda(e.resultado)}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

/** El año mes a mes, marcando qué es hecho y qué es proyección. */
function TablaMeses({ meses }: { meses: MesDelAnio[] }) {
  return (
    <TableContainer>
      <Table>
        <THead>
          <TR>
            <TH>Mes</TH>
            <TH className="text-right">Ingreso</TH>
            <TH className="text-right">Meta</TH>
            <TH className="text-right">Gasto</TH>
            <TH className="text-right">Presupuesto</TH>
            <TH className="text-right">Resultado</TH>
          </TR>
        </THead>
        <TBody>
          {meses.map((m) => {
            const futuro = m.estado === "FUTURO";
            const res = Number(m.ingreso_real) - Number(m.gasto_real);
            return (
              <TR key={m.periodo} className={cn(futuro && "opacity-50")}>
                <TD>
                  <div className="flex items-center gap-2">
                    <span className="text-content">{mesDe(m.periodo)}</span>
                    {m.estado === "EN_CURSO" && <Badge tone="pendiente">en curso</Badge>}
                    {futuro && <span className="text-xs text-content-muted">por venir</span>}
                  </div>
                </TD>
                <TD className="text-right tabular-nums text-content">
                  {futuro ? "—" : formatMonto(m.ingreso_real)}
                  {/* El mes en curso muestra además a cuánto llegaría completo: es el único
                      renglón del año que no es ni hecho ni presupuesto. */}
                  {m.ingreso_proyectado && (
                    <span className="block text-xs text-content-muted">
                      → {formatMonto(m.ingreso_proyectado)} al cierre
                    </span>
                  )}
                </TD>
                <TD className="text-right tabular-nums text-content-muted">
                  {Number(m.ingreso_meta) ? formatMonto(m.ingreso_meta) : "—"}
                </TD>
                <TD className="text-right tabular-nums text-content">
                  {futuro ? "—" : formatMonto(m.gasto_real)}
                  {m.gasto_proyectado && (
                    <span className="block text-xs text-content-muted">
                      → {formatMonto(m.gasto_proyectado)} al cierre
                    </span>
                  )}
                </TD>
                <TD className="text-right tabular-nums text-content-muted">
                  {Number(m.gasto_meta) ? formatMonto(m.gasto_meta) : "—"}
                </TD>
                <TD
                  className={cn(
                    "text-right font-medium tabular-nums",
                    futuro ? "text-content-muted" : res < 0 ? "text-negativo" : "text-content",
                  )}
                >
                  {futuro ? "—" : formatMonto(String(res))}
                </TD>
              </TR>
            );
          })}
        </TBody>
      </Table>
    </TableContainer>
  );
}
