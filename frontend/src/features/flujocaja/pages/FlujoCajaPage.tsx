/**
 * Flujo de caja a 13 semanas (/flujo-caja).
 *
 * Contesta dos preguntas que ninguna otra pantalla contestaba: «¿cuánto voy a tener en el banco
 * el viernes 14?» y «¿en qué semana me quedo corto?».
 *
 * ── POR QUÉ LAS CAPAS VAN SEPARADAS ─────────────────────────────────────────────────────────
 *
 * COMPROMETIDO tiene documento y fecha; ESPERADO es probable; REPRESA es la cartera atrasada que
 * se desagota al ritmo que fija la política. Apiladas en un solo número se vería el saldo;
 * separadas se ve CUÁL de las tres hay que mover para que la semana 7 no quede en rojo. Esa es
 * la diferencia entre un gráfico y una herramienta.
 */

import { useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ErrorState,
  Input,
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
  useToast,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { mensajeError } from "@/lib/apiError";
import { formatMoneda, formatMonto } from "@/lib/format";
import { useTienePermiso } from "@/features/auth/permisos";
import { CurvaCaja } from "@/features/flujocaja/components/CurvaCaja";
import { useFlujoCaja, useGuardarPolitica } from "@/features/flujocaja/hooks";
import type { AporteOrigen, FlujoSemanal, ModoPolitica, PoliticaCaja } from "@/api/flujocaja";

/** Nombre legible de cada fuente. El código no se le muestra a nadie. */
const NOMBRE_ORIGEN: Record<string, string> = {
  PLANILLA: "Planilla",
  CXP_PROGRAMADA: "Proveedores (programado)",
  CXP_ATRASO: "Cartera atrasada",
  CXC_CARGO: "Cobros con cargo emitido",
  CXC_CUOTA: "Cuotas de contratos",
};

export function FlujoCajaPage() {
  const [moneda, setMoneda] = useState("CRC");
  const flujoQ = useFlujoCaja(moneda);
  const puedeDecidir = useTienePermiso()("bancos.politica_caja");
  const f = flujoQ.data;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Flujo de caja · 13 semanas"
        description="Cuánto vas a tener en el banco cada semana, y en cuál te quedás corto."
        actions={
          <Select
            label="Moneda"
            id="flujo-moneda"
            value={moneda}
            onChange={(e) => setMoneda(e.target.value)}
            options={[
              { value: "CRC", label: "Colones" },
              { value: "USD", label: "Dólares" },
            ]}
          />
        }
      />

      {flujoQ.isPending ? (
        <LoadingState label="Proyectando las trece semanas" />
      ) : flujoQ.isError ? (
        <ErrorState message={mensajeError(flujoQ.error)} onRetry={() => flujoQ.refetch()} />
      ) : !f ? null : (
        <>
          <Titular flujo={f} />

          {f.avisos.length > 0 && (
            <div className="flex flex-col gap-2 rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
              {f.avisos.map((a, i) => (
                <p key={i} className="text-sm text-content-muted">
                  {a}
                </p>
              ))}
            </div>
          )}

          <Card>
            <CardHeader>
              <CardTitle>
                Saldo proyectado · {f.desde} al {f.hasta}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <CurvaCaja flujo={f} />
            </CardContent>
          </Card>

          <PoliticaPanel flujo={f} moneda={moneda} puedeDecidir={puedeDecidir} />
          <TablaSemanas flujo={f} />
        </>
      )}
    </div>
  );
}

/** El titular: lo que se mira primero y, muchas veces, lo único que se mira. */
function Titular({ flujo: f }: { flujo: FlujoSemanal }) {
  const rojo = f.primera_semana_en_rojo;
  const bajo = f.primera_semana_bajo_minimo;
  const semanaCritica = f.semanas.find((s) => s.numero === (rojo || bajo));

  return (
    <Card>
      <CardContent className="flex flex-wrap items-start gap-x-10 gap-y-4 pt-5">
        <div>
          {/* SIN SALDO DE PARTIDA NO SE AFIRMA NADA. La curva arranca en cero, así que «aguanta»
              es verdad por casualidad: mide el movimiento, no el saldo. Un titular en verde al
              lado de un aviso que dice que falta el dato se lee como el veredicto, y el aviso
              como letra chica. */}
          {f.fuente_saldo === "SIN_DATOS" ? (
            <>
              <p className="text-sm font-medium text-pendiente">
                Todavía no se puede decir si la caja aguanta
              </p>
              <p className="max-w-md text-xs text-content-muted">
                Falta el saldo de partida, así que esto proyecta el movimiento de las trece
                semanas, no el saldo del banco.
              </p>
            </>
          ) : rojo > 0 ? (
            <>
              <p className="text-sm font-medium text-negativo">
                La caja queda en rojo en la semana {rojo}
              </p>
              <p className="text-xs text-content-muted">
                {semanaCritica ? `del ${semanaCritica.desde} · ` : ""}
                saldo {formatMoneda(semanaCritica?.saldo_final ?? "0")}
              </p>
            </>
          ) : bajo > 0 ? (
            <>
              <p className="text-sm font-medium text-pendiente">
                La caja baja del mínimo operativo en la semana {bajo}
              </p>
              <p className="text-xs text-content-muted">
                {semanaCritica ? `del ${semanaCritica.desde} · ` : ""}
                saldo {formatMoneda(semanaCritica?.saldo_final ?? "0")}
              </p>
            </>
          ) : (
            <>
              <p className="text-sm font-medium text-positivo">
                La caja aguanta las trece semanas
              </p>
              <p className="text-xs text-content-muted">
                sin cruzar el mínimo operativo ni quedar en negativo
              </p>
            </>
          )}
        </div>

        <Dato
          titulo="Punto más bajo"
          valor={formatMoneda(f.punto_mas_bajo)}
          pie={`semana ${f.semana_punto_mas_bajo}`}
        />
        <Dato
          titulo="Saldo al cierre"
          valor={formatMoneda(f.saldo_final_horizonte)}
          pie={`arranca en ${formatMoneda(f.saldo_inicial)}`}
        />
        <Dato
          titulo="Cartera atrasada"
          valor={formatMoneda(f.represa_total)}
          pie={
            f.semanas_para_vaciar < 0
              ? "no baja al ritmo actual"
              : f.semanas_para_vaciar === 0
                ? "sin atrasos"
                : `${f.semanas_para_vaciar} semanas para salir`
          }
        />
      </CardContent>
    </Card>
  );
}

function Dato({ titulo, valor, pie }: { titulo: string; valor: string; pie: string }) {
  return (
    <div>
      <p className="text-xs text-content-muted">{titulo}</p>
      <p className="text-lg font-semibold tabular-nums text-content">{valor}</p>
      <p className="text-xs text-content-muted">{pie}</p>
    </div>
  );
}

/** Declarar la política. Es lo que convierte la cartera atrasada en una curva. */
function PoliticaPanel({
  flujo: f,
  moneda,
  puedeDecidir,
}: {
  flujo: FlujoSemanal;
  moneda: string;
  puedeDecidir: boolean;
}) {
  const toast = useToast();
  const guardar = useGuardarPolitica();
  const [editando, setEditando] = useState(false);
  const [borrador, setBorrador] = useState<PoliticaCaja>(f.politica);

  function abrir() {
    setBorrador({ ...f.politica, moneda });
    setEditando(true);
  }

  if (!editando) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Política de caja</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center gap-x-10 gap-y-3">
          {f.politica.definida ? (
            <>
              <Dato
                titulo="Para pagar atrasos"
                valor={
                  f.politica.modo === "PCT_INGRESOS"
                    ? `${formatMonto(f.politica.tope_pct)} % de lo que entra`
                    : `${formatMoneda(f.politica.tope_semanal)} por semana`
                }
                pie={`se desagotan ${formatMoneda(f.represa_desagotada)} en 13 semanas`}
              />
              <Dato
                titulo="Saldo mínimo operativo"
                valor={formatMoneda(f.politica.minimo_operativo)}
                pie="por debajo de esto la operación no camina"
              />
              {f.politica.nota && (
                <p className="max-w-sm text-xs text-content-muted">{f.politica.nota}</p>
              )}
            </>
          ) : (
            <p className="max-w-2xl text-sm text-content-muted">
              Sin política declarada, la cartera atrasada no entra en la proyección y el flujo se
              ve mejor de lo que va a ser. Declarar cuánto se destina por semana a pagar atrasos
              es lo que convierte {formatMoneda(f.represa_total)} de deuda vencida en un plan.
            </p>
          )}
          {puedeDecidir && (
            <Button variant="secondary" className="ml-auto" onClick={abrir}>
              {f.politica.definida ? "Cambiar" : "Declarar la política"}
            </Button>
          )}
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Política de caja</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-4">
          <Select
            label="Cómo se paga el atraso"
            id="politica-modo"
            value={borrador.modo}
            onChange={(e) => setBorrador({ ...borrador, modo: e.target.value as ModoPolitica })}
            options={[
              { value: "MONTO_FIJO", label: "Un monto fijo por semana" },
              { value: "PCT_INGRESOS", label: "Un % de lo que entra esa semana" },
            ]}
          />

          {borrador.modo === "MONTO_FIJO" ? (
            <Input
              label="Tope semanal"
              id="politica-tope"
              inputMode="decimal"
              value={borrador.tope_semanal}
              onChange={(e) => setBorrador({ ...borrador, tope_semanal: e.target.value })}
            />
          ) : (
            <Input
              label="% de lo que entra"
              id="politica-pct"
              inputMode="decimal"
              value={borrador.tope_pct}
              onChange={(e) => setBorrador({ ...borrador, tope_pct: e.target.value })}
            />
          )}

          <Input
            label="Saldo mínimo operativo"
            id="politica-minimo"
            inputMode="decimal"
            value={borrador.minimo_operativo}
            onChange={(e) => setBorrador({ ...borrador, minimo_operativo: e.target.value })}
          />
          <Input
            label="Nota"
            id="politica-nota"
            value={borrador.nota}
            onChange={(e) => setBorrador({ ...borrador, nota: e.target.value })}
          />
        </div>

        <p className="text-xs text-content-muted">
          El tope decide la curva: con un monto bajo, la cartera atrasada no baja y el flujo lo
          dice. Queda registrado quién lo declaró y cuándo.
        </p>

        <div className="flex gap-2">
          <Button
            loading={guardar.isPending}
            onClick={() =>
              guardar.mutate(
                { ...borrador, moneda },
                {
                  onSuccess: () => {
                    toast.success("Política guardada; la curva se recalculó.");
                    setEditando(false);
                  },
                  onError: (e) => toast.error(mensajeError(e)),
                },
              )
            }
          >
            Guardar
          </Button>
          <Button variant="secondary" onClick={() => setEditando(false)}>
            Cancelar
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

/** Las trece semanas, con su desglose. */
function TablaSemanas({ flujo: f }: { flujo: FlujoSemanal }) {
  const [abierta, setAbierta] = useState<number | null>(null);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Semana por semana</CardTitle>
      </CardHeader>
      <CardContent>
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Semana</TH>
                <TH className="text-right">Entra</TH>
                <TH className="text-right">Sale</TH>
                <TH className="text-right">De eso, atrasos</TH>
                <TH className="text-right">Neto</TH>
                <TH className="text-right">Saldo al cierre</TH>
              </TR>
            </THead>
            <TBody>
              {f.semanas.map((s) => (
                <>
                  <TR
                    key={s.numero}
                    className="cursor-pointer"
                    onClick={() => setAbierta(abierta === s.numero ? null : s.numero)}
                  >
                    <TD>
                      <div className="flex items-center gap-2">
                        <span className="tabular-nums text-content">{s.numero}</span>
                        <span className="text-xs text-content-muted">{s.desde}</span>
                        {s.en_rojo ? (
                          <Badge tone="negativo">en rojo</Badge>
                        ) : s.bajo_minimo ? (
                          <Badge tone="pendiente">bajo el mínimo</Badge>
                        ) : null}
                      </div>
                    </TD>
                    <TD className="text-right tabular-nums text-content-muted">
                      {formatMonto(s.entradas)}
                    </TD>
                    <TD className="text-right tabular-nums text-content-muted">
                      {formatMonto(s.salidas)}
                    </TD>
                    <TD className="text-right tabular-nums text-content-muted">
                      {formatMonto(s.salidas_represa)}
                    </TD>
                    <TD
                      className={cn(
                        "text-right tabular-nums",
                        s.neto.startsWith("-") ? "text-negativo" : "text-content",
                      )}
                    >
                      {formatMonto(s.neto)}
                    </TD>
                    <TD
                      className={cn(
                        "text-right font-medium tabular-nums",
                        s.en_rojo ? "text-negativo" : s.bajo_minimo ? "text-pendiente" : "text-content",
                      )}
                    >
                      {formatMonto(s.saldo_final)}
                    </TD>
                  </TR>
                  {abierta === s.numero && (
                    <TR key={`${s.numero}-detalle`}>
                      <TD colSpan={6} className="bg-surface-muted/40">
                        <Desglose aportes={s.por_origen} />
                      </TD>
                    </TR>
                  )}
                </>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      </CardContent>
    </Card>
  );
}

/** De dónde sale cada colón de la semana. Es lo que permite actuar sobre el número. */
function Desglose({ aportes }: { aportes: AporteOrigen[] }) {
  if (!aportes?.length) {
    return <p className="py-2 text-sm text-content-muted">Esta semana no tiene movimiento previsto.</p>;
  }
  return (
    <div className="flex flex-col gap-1 py-1">
      {aportes.map((a, i) => (
        <div key={i} className="flex items-center gap-3 text-sm">
          <span className={cn("w-4 text-center", a.entrada ? "text-positivo" : "text-negativo")}>
            {a.entrada ? "+" : "−"}
          </span>
          <span className="text-content">{NOMBRE_ORIGEN[a.origen] ?? a.origen}</span>
          <Badge tone={a.capa === "COMPROMETIDO" ? "neutral" : "pendiente"}>
            {a.capa === "COMPROMETIDO"
              ? "comprometido"
              : a.capa === "REPRESA"
                ? "atraso"
                : "esperado"}
          </Badge>
          <span className="ml-auto tabular-nums text-content">{formatMonto(a.monto)}</span>
        </div>
      ))}
    </div>
  );
}
