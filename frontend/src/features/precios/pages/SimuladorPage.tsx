/**
 * Pantalla — Simulador (/precios/simulador).
 *
 * ── QUÉ CONTESTA, Y POR QUÉ HOY NADIE PUEDE ─────────────────────────────────────────────────
 *
 *   · Si el cofre sube 12 %, ¿qué pasa con el margen de Oro?
 *   · ¿A qué cuota empata Gardenia?
 *   · ¿Cuánta siniestralidad aguanta un plan de suscripción antes de perder plata?
 *
 * ── LA CIFRA QUE MANDA EN SUSCRIPCIÓN ───────────────────────────────────────────────────────
 *
 * La FRECUENCIA DE EQUILIBRIO: cuánto uso aguanta el plan. Medido con el costeo actual, Gardenia
 * aguanta 2,53 % y la frecuencia que se desprende de los servicios observados ronda el 5 %. Esa
 * comparación es la que decide si el plan se sostiene, y no existe en el archivo de costeo.
 *
 * La frecuencia NO se fija acá: se escribe como supuesto y queda declarada al lado del resultado.
 * El número del que todo cuelga —si el 0,47 % histórico es por contrato o por beneficiario— es
 * una definición pendiente, y un simulador que la fijara solo escondería la pregunta.
 *
 * ── EL CÁLCULO ESTÁ EN EL SERVIDOR ──────────────────────────────────────────────────────────
 *
 * Esta pantalla manda supuestos y pinta lo que vuelve. No recalcula nada: las reglas del costeo
 * ya viven en el servicio y duplicarlas acá daría dos verdades.
 */

import { useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  EmptyState,
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
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import type { ResultadoSimulacion } from "@/api/precios";

import { usePlanes, useSimular } from "../hooks";

const HOY = new Date().toISOString().slice(0, 10);

function mesAnterior(): { desde: string; hasta: string } {
  const hoy = new Date();
  const fin = new Date(hoy.getFullYear(), hoy.getMonth(), 0);
  const ini = new Date(fin.getFullYear(), fin.getMonth(), 1);
  const f = (d: Date) => d.toISOString().slice(0, 10);
  return { desde: f(ini), hasta: f(fin) };
}

function money(v: string | null | undefined): string {
  if (v === null || v === undefined) return "—";
  const n = Number(v);
  if (Number.isNaN(n)) return v;
  return `₡${n.toLocaleString("es-CR", { maximumFractionDigits: 0 })}`;
}

interface Guardado {
  nombre: string;
  resultado: ResultadoSimulacion;
}

export function SimuladorPage() {
  const toast = useToast();
  const planes = usePlanes();
  const simular = useSimular();
  const periodo = mesAnterior();

  const [planId, setPlanId] = useState("");
  const [esc, setEsc] = useState({
    precio: "",
    cuota: "",
    permanencia: "",
    ajusteCostos: "",
    frecuencia: "",
    sinComision: false,
    sinIndirecto: false,
  });
  const [resultado, setResultado] = useState<ResultadoSimulacion | null>(null);
  // Los escenarios guardados se comparan lado a lado. Es lo que convierte «probar un número» en
  // «decidir entre opciones».
  const [guardados, setGuardados] = useState<Guardado[]>([]);

  const plan = (planes.data?.planes ?? []).find((p) => p.id === planId);
  const esSuscripcion = plan?.modalidad === "SUSCRIPCION";

  async function correr() {
    if (!planId) return;
    try {
      const r = await simular.mutateAsync({
        planId,
        body: {
          fecha: HOY,
          desde: periodo.desde,
          hasta: periodo.hasta,
          escenario: {
            precio_venta: esc.precio || null,
            cuota_mensual: esc.cuota || null,
            permanencia_meses: esc.permanencia ? Number(esc.permanencia) : null,
            ajuste_costos_pct: esc.ajusteCostos || null,
            frecuencia_anual_pct: esc.frecuencia || null,
            sin_comision: esc.sinComision,
            sin_indirecto: esc.sinIndirecto,
          },
        },
      });
      setResultado(r);
    } catch (e) {
      toast.error(mensajeError(e));
      setResultado(null);
    }
  }

  function guardar() {
    if (!resultado) return;
    const etiqueta =
      [
        esc.cuota && `cuota ${esc.cuota}`,
        esc.precio && `precio ${esc.precio}`,
        esc.ajusteCostos && `costos ${esc.ajusteCostos > "0" ? "+" : ""}${esc.ajusteCostos}%`,
        esc.frecuencia && `frec ${esc.frecuencia}%`,
      ]
        .filter(Boolean)
        .join(" · ") || "como está";
    setGuardados((g) => [...g, { nombre: etiqueta, resultado }]);
  }

  const negativo = resultado ? resultado.margen.startsWith("-") : false;

  return (
    <div className="space-y-4">
      <PageHeader
        title="Simulador"
        description="Mover una variable y ver el efecto antes de decidir. No guarda nada."
      />

      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 py-4">
          <Select
            label="Plan"
            value={planId}
            onChange={(e) => {
              setPlanId(e.target.value);
              setResultado(null);
            }}
            options={[
              { value: "", label: "Elegí un plan…" },
              ...(planes.data?.planes ?? []).map((p) => ({
                value: p.id,
                label: `${p.nombre} · ${p.familia}`,
              })),
            ]}
          />
          {plan && (
            <div className="pb-2 text-xs text-content-muted">
              {esSuscripcion
                ? `Cuota actual ${money(plan.cuota_mensual)} · ${plan.permanencia_meses ?? "—"} meses`
                : `Precio actual ${money(plan.precio_venta)}`}
            </div>
          )}
        </CardContent>
      </Card>

      {!planId && <EmptyState message="Elegí un plan arriba para simular sobre él." />}

      {planId && (
        <Card>
          <CardHeader>
            <CardTitle>Supuestos</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm text-content-muted">
              Lo que dejes en blanco sale del plan tal como está hoy.
            </p>
            <div className="flex flex-wrap items-end gap-3">
              {esSuscripcion ? (
                <>
                  <Input
                    label="Cuota mensual"
                    placeholder={plan?.cuota_mensual ?? ""}
                    value={esc.cuota}
                    onChange={(e) => setEsc({ ...esc, cuota: e.target.value })}
                  />
                  <Input
                    label="Permanencia (meses)"
                    placeholder={String(plan?.permanencia_meses ?? "")}
                    value={esc.permanencia}
                    onChange={(e) => setEsc({ ...esc, permanencia: e.target.value })}
                  />
                  <Input
                    label="Frecuencia anual de uso (%)"
                    placeholder="5.05"
                    value={esc.frecuencia}
                    onChange={(e) => setEsc({ ...esc, frecuencia: e.target.value })}
                  />
                </>
              ) : (
                <Input
                  label="Precio de venta"
                  placeholder={plan?.precio_venta ?? ""}
                  value={esc.precio}
                  onChange={(e) => setEsc({ ...esc, precio: e.target.value })}
                />
              )}
              <Input
                label="Los insumos suben (%)"
                placeholder="12"
                value={esc.ajusteCostos}
                onChange={(e) => setEsc({ ...esc, ajusteCostos: e.target.value })}
              />
              <label className="flex items-center gap-2 pb-2 text-sm">
                <input
                  type="checkbox"
                  checked={!esc.sinIndirecto}
                  onChange={(e) => setEsc({ ...esc, sinIndirecto: !e.target.checked })}
                />
                Incluir indirecto
              </label>
              <label className="flex items-center gap-2 pb-2 text-sm">
                <input
                  type="checkbox"
                  checked={!esc.sinComision}
                  onChange={(e) => setEsc({ ...esc, sinComision: !e.target.checked })}
                />
                Incluir comisión
              </label>
              <Button onClick={correr} disabled={simular.isPending}>
                {simular.isPending ? "Calculando…" : "Simular"}
              </Button>
            </div>
            {esSuscripcion && (
              <p className="text-xs text-content-muted">
                En suscripción el costo esperado es el del servicio <b>por la probabilidad de
                usarlo</b>: un contrato paga todos los meses y usa el servicio una vez, o ninguna.
                Sin frecuencia se asume el peor caso y el resultado queda marcado.
              </p>
            )}
          </CardContent>
        </Card>
      )}

      {simular.isPending && <LoadingState />}

      {resultado && (
        <>
          {!resultado.confiable && (
            <div className="rounded-lg border border-pendiente/40 bg-pendiente/5 p-3 text-sm">
              <b>Este resultado está incompleto.</b> El número se muestra igual, pero no se puede
              leer como definitivo:
              <ul className="ml-4 mt-1 list-disc text-xs text-content-muted">
                {resultado.faltantes.map((f) => (
                  <li key={f}>{f}</li>
                ))}
              </ul>
            </div>
          )}

          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">Ingreso</div>
                <div className="text-base font-bold tabular-nums lg:text-xl">
                  {money(resultado.ingreso)}
                </div>
                <div className="text-xs text-content-muted">{resultado.ingreso_detalle}</div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">
                  Costo esperado
                </div>
                <div className="text-base font-bold tabular-nums lg:text-xl">
                  {money(resultado.costo_esperado)}
                </div>
                <div className="text-xs text-content-muted">
                  {resultado.frecuencia_usada
                    ? `servicio ${money(resultado.costo_servicio)} × ${resultado.frecuencia_usada}%`
                    : "el servicio completo"}
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">
                  Resultado por contrato
                </div>
                <div
                  className={`text-base font-bold tabular-nums lg:text-xl ${negativo ? "text-negativo" : "text-positivo"}`}
                >
                  {money(resultado.margen)}
                </div>
                <div className="text-xs text-content-muted">{resultado.margen_pct}%</div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="py-4">
                <div className="text-xs uppercase tracking-wide text-content-muted">
                  {resultado.frecuencia_equilibrio ? "Frecuencia que aguanta" : "Costo total"}
                </div>
                {resultado.frecuencia_equilibrio ? (
                  <>
                    <div className="text-base font-bold tabular-nums text-accent lg:text-xl">
                      {resultado.frecuencia_equilibrio}%
                    </div>
                    <div className="text-xs text-content-muted">
                      por encima de esto, pierde plata
                    </div>
                  </>
                ) : (
                  <>
                    <div className="text-base font-bold tabular-nums lg:text-xl">
                      {money(resultado.costo_total)}
                    </div>
                    <div className="text-xs text-content-muted">esperado + indirecto + comisión</div>
                  </>
                )}
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>De dónde sale</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <TableContainer>
                <Table>
                  <THead>
                    <TR>
                      <TH>Concepto</TH>
                      <TH className="text-right">Monto</TH>
                    </TR>
                  </THead>
                  <TBody>
                    <TR>
                      <TD>Ingreso — {resultado.ingreso_detalle}</TD>
                      <TD className="text-right tabular-nums text-positivo">
                        {money(resultado.ingreso)}
                      </TD>
                    </TR>
                    <TR>
                      <TD>
                        Costo del servicio
                        {resultado.frecuencia_usada && (
                          <span className="text-xs text-content-muted">
                            {" "}
                            — entero {money(resultado.costo_servicio)}, esperado al{" "}
                            {resultado.frecuencia_usada}%
                          </span>
                        )}
                      </TD>
                      <TD className="text-right tabular-nums">−{money(resultado.costo_esperado)}</TD>
                    </TR>
                    <TR>
                      <TD>Indirecto absorbido</TD>
                      <TD className="text-right tabular-nums">−{money(resultado.indirecto)}</TD>
                    </TR>
                    <TR>
                      <TD>Comisión (con su carga social)</TD>
                      <TD className="text-right tabular-nums">−{money(resultado.comision)}</TD>
                    </TR>
                    <TR>
                      <TD className="font-medium">Resultado</TD>
                      <TD
                        className={`text-right tabular-nums font-bold ${negativo ? "text-negativo" : "text-positivo"}`}
                      >
                        {money(resultado.margen)}
                      </TD>
                    </TR>
                  </TBody>
                </Table>
              </TableContainer>
              <Button variant="ghost" onClick={guardar}>
                Guardar este escenario para comparar
              </Button>
            </CardContent>
          </Card>
        </>
      )}

      {guardados.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Escenarios comparados</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Escenario</TH>
                    <TH className="text-right">Ingreso</TH>
                    <TH className="text-right">Costo</TH>
                    <TH className="text-right">Resultado</TH>
                    <TH className="text-right">Aguanta</TH>
                    <TH></TH>
                  </TR>
                </THead>
                <TBody>
                  {guardados.map((g, i) => {
                    const neg = g.resultado.margen.startsWith("-");
                    return (
                      <TR key={`${g.nombre}-${i}`}>
                        <TD>
                          <div className="font-medium">{g.resultado.plan}</div>
                          <div className="text-xs text-content-muted">{g.nombre}</div>
                        </TD>
                        <TD className="text-right tabular-nums">{money(g.resultado.ingreso)}</TD>
                        <TD className="text-right tabular-nums">{money(g.resultado.costo_total)}</TD>
                        <TD
                          className={`text-right tabular-nums font-medium ${neg ? "text-negativo" : "text-positivo"}`}
                        >
                          {money(g.resultado.margen)}
                        </TD>
                        <TD className="text-right tabular-nums">
                          {g.resultado.frecuencia_equilibrio
                            ? `${g.resultado.frecuencia_equilibrio}%`
                            : "—"}
                        </TD>
                        <TD>
                          {!g.resultado.confiable && <Badge tone="pendiente">parcial</Badge>}
                        </TD>
                      </TR>
                    );
                  })}
                </TBody>
              </Table>
            </TableContainer>
            <Button variant="ghost" onClick={() => setGuardados([])}>
              Limpiar la comparación
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
