/**
 * RRHH — Finiquitos (/rrhh/finiquitos). Maqueta "Liquidación / Prestaciones": cálculo de
 * cese conforme al Código de Trabajo sobre el salario promedio REAL (comisiones y bonos
 * incluidos): preaviso + cesantía (tope 8 años) según motivo, vacaciones pendientes,
 * aguinaldo proporcional, y el comparativo calculado-vs-provisionado. Incluye el reporte
 * de provisiones acumuladas del año (corridas pagadas).
 */

import { useState, type FormEvent } from "react";
import { createPortal } from "react-dom";
import {
  Badge,
  Button,
  Card,
  CardContent,
  ConfirmDialog,
  EmptyState,
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
  type BadgeTone,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatFecha, formatMoneda, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import { useAuth } from "@/features/auth/AuthContext";
import { abrirFiniquitoImprimible } from "@/features/rrhh/components/FiniquitoImprimible";
import {
  useAnularFiniquito,
  useAprobarFiniquito,
  useActualizarFiniquito,
  useCrearFiniquito,
  useEmpleados,
  useFiniquito,
  useFiniquitos,
  usePagarFiniquito,
  useProvisiones,
} from "@/features/rrhh/hooks";
import type { Finiquito, FiniquitoInput, MotivoCese } from "@/api/rrhh";

const MOTIVOS: Array<{ value: MotivoCese; label: string }> = [
  { value: "DESPIDO_RESPONSABILIDAD", label: "Despido con responsabilidad patronal" },
  { value: "RENUNCIA", label: "Renuncia" },
  { value: "FIN_CONTRATO", label: "Fin de contrato" },
  { value: "MUTUO_ACUERDO", label: "Mutuo acuerdo" },
];

const TONO_FIN: Record<string, BadgeTone> = {
  BORRADOR: "pendiente",
  APROBADO: "accent",
  PAGADO: "positivo",
  ANULADO: "neutral",
};

export function FiniquitosPage() {
  const tiene = useTienePermiso();
  const puedeLiquidar = tiene("rrhh.finiquito");
  const finiquitosQ = useFiniquitos();
  const [selId, setSelId] = useState<string | null>(null);
  const [nuevo, setNuevo] = useState(false);

  const items = finiquitosQ.data ?? [];

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Finiquitos"
        description="Liquidación de cese conforme al Código de Trabajo, sobre el salario promedio real (comisiones y bonos incluidos — nunca una base reducida)."
        actions={puedeLiquidar ? <Button onClick={() => setNuevo(true)}>Nuevo finiquito</Button> : undefined}
      />

      {finiquitosQ.isPending ? (
        <LoadingState label="Cargando finiquitos" />
      ) : finiquitosQ.isError ? (
        <ErrorState message={mensajeError(finiquitosQ.error)} onRetry={() => finiquitosQ.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState message="No hay finiquitos registrados." />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Empleado</TH>
                <TH>Motivo</TH>
                <TH>Salida</TH>
                <TH>Estado</TH>
                <TH className="text-right">Total</TH>
                <TH className="text-right">Acción</TH>
              </TR>
            </THead>
            <TBody>
              {items.map((f) => (
                <TR key={f.id} className={cn(f.estado === "ANULADO" && "opacity-50", selId === f.id && "bg-accent/5")}>
                  <TD>
                    <div className="font-medium">{f.empleado_nombre}</div>
                    <div className="text-xs text-content-muted">{f.identificacion} · {f.anios_servicio} años</div>
                  </TD>
                  <TD className="text-xs">{MOTIVOS.find((m) => m.value === f.motivo)?.label ?? f.motivo}</TD>
                  <TD className="text-xs">{formatFecha(f.fecha_salida)}</TD>
                  <TD>
                    <Badge tone={TONO_FIN[f.estado]}>{f.estado}</Badge>
                  </TD>
                  <TD className="text-right font-semibold tabular-nums">{formatMoneda(f.total, "CRC")}</TD>
                  <TD className="text-right">
                    <Button size="sm" variant="secondary" onClick={() => setSelId(selId === f.id ? null : f.id)}>
                      {selId === f.id ? "Ocultar" : "Abrir"}
                    </Button>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {selId && <FiniquitoCard key={selId} id={selId} puedeLiquidar={puedeLiquidar} onAnulado={() => setSelId(null)} />}

      <ProvisionesCard />

      {nuevo && (
        <FiniquitoDialog
          onCerrar={() => setNuevo(false)}
          onCreado={(id) => {
            setNuevo(false);
            setSelId(id);
          }}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Detalle del finiquito: rubros + calculado vs provisionado + acciones
// ---------------------------------------------------------------------------

function FiniquitoCard({ id, puedeLiquidar, onAnulado }: { id: string; puedeLiquidar: boolean; onAnulado: () => void }) {
  const toast = useToast();
  const { empresaActiva } = useAuth();
  const finiquitoQ = useFiniquito(id);
  const actualizar = useActualizarFiniquito();
  const aprobar = useAprobarFiniquito();
  const pagar = usePagarFiniquito();
  const anular = useAnularFiniquito();
  const [confirmar, setConfirmar] = useState<"aprobar" | "pagar" | "anular" | null>(null);
  const [editar, setEditar] = useState(false);

  if (finiquitoQ.isPending) return <LoadingState label="Cargando finiquito" />;
  if (finiquitoQ.isError)
    return <ErrorState message={mensajeError(finiquitoQ.error)} onRetry={() => finiquitoQ.refetch()} />;
  const f = finiquitoQ.data;
  const esBorrador = f.estado === "BORRADOR";
  const onError = (err: unknown) => toast.error(mensajeError(err));

  const provisionado = toNumber(f.provisionado);
  const total = Math.max(1, toNumber(f.total));
  const pctProv = Math.min(100, (provisionado / total) * 100);

  return (
    <Card>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-semibold text-content">Finiquito — {f.empleado_nombre}</h3>
            <Badge tone={TONO_FIN[f.estado]}>{f.estado}</Badge>
            <span className="text-xs text-content-muted">
              Ingreso {formatFecha(f.fecha_ingreso)} · salida {formatFecha(f.fecha_salida)} · {f.anios_servicio} años
            </span>
          </div>
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="secondary"
              onClick={() => {
                if (!abrirFiniquitoImprimible(f, empresaActiva?.nombre ?? "Valle de Paz")) {
                  toast.error("El navegador bloqueó la ventana. Permitile abrir ventanas emergentes a este sitio.");
                }
              }}
            >
              Imprimir / PDF
            </Button>
          </div>
          {puedeLiquidar && (
            <div className="flex gap-2">
              {esBorrador && (
                <>
                  <Button size="sm" variant="secondary" onClick={() => setEditar(true)}>
                    Editar datos
                  </Button>
                  <Button size="sm" onClick={() => setConfirmar("aprobar")}>
                    Aprobar ✓
                  </Button>
                </>
              )}
              {f.estado === "APROBADO" && (
                <Button size="sm" onClick={() => setConfirmar("pagar")}>
                  Marcar pagado
                </Button>
              )}
              {(esBorrador || f.estado === "APROBADO") && (
                <Button size="sm" variant="ghost" className="text-negativo" onClick={() => setConfirmar("anular")}>
                  Anular
                </Button>
              )}
            </div>
          )}
        </div>

        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Rubro</TH>
                  <TH className="text-right">Monto</TH>
                </TR>
              </THead>
              <TBody>
                {f.detalle.map((d, i) => (
                  <TR key={i}>
                    <TD className={cn("text-sm", d.tipo !== "INGRESO" && "text-content-muted")}>{d.nombre}</TD>
                    <TD className={cn("text-right font-medium tabular-nums", d.tipo !== "INGRESO" && "text-negativo")}>
                      {d.tipo !== "INGRESO" && "−"}
                      {formatMoneda(d.monto, "CRC")}
                    </TD>
                  </TR>
                ))}
                <TR>
                  <TD className="font-semibold">Total del finiquito</TD>
                  <TD className="text-right text-base font-bold tabular-nums text-accent">{formatMoneda(f.total, "CRC")}</TD>
                </TR>
              </TBody>
            </Table>
          </TableContainer>

          <div className="flex flex-col gap-3">
            <div className="rounded-lg border border-border p-4">
              <h4 className="text-sm font-semibold text-content">Calculado vs provisionado</h4>
              <p className="text-xs text-content-muted">Cuánto de esta liquidación ya estaba provisionado (corridas pagadas)</p>
              <div className="mt-3 h-3 overflow-hidden rounded-full bg-dorado/20">
                <div className="h-full rounded-full bg-accent" style={{ width: `${pctProv}%` }} />
              </div>
              <div className="mt-2 flex justify-between text-xs">
                <span className="text-accent">Provisionado {formatMoneda(f.provisionado, "CRC")}</span>
                <span className="text-content-muted">
                  Diferencia a desembolsar {formatMoneda(String(Math.max(0, toNumber(f.total) - provisionado)), "CRC")}
                </span>
              </div>
            </div>
            {toNumber(f.base_ccss) > 0 && (
              <div className="rounded-lg border border-border px-3 py-2 text-xs">
                <div className="font-semibold text-content">Porción afecta del finiquito</div>
                <div className="mt-1 flex justify-between text-content-muted">
                  <span>Base CCSS (vacaciones pendientes)</span>
                  <span className="tabular-nums">{formatMoneda(f.base_ccss, "CRC")}</span>
                </div>
                <div className="flex justify-between text-content-muted">
                  <span>CCSS obrero retenido</span>
                  <span className="tabular-nums">{formatMoneda(f.ccss_obrero, "CRC")}</span>
                </div>
                <div className="flex justify-between text-content-muted">
                  <span>Renta retenida</span>
                  <span className="tabular-nums">{formatMoneda(f.renta, "CRC")}</span>
                </div>
              </div>
            )}
            <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-xs text-content-muted">
              ⚖️ El cálculo usa el <b>salario promedio real</b> de las últimas liquidaciones pagadas
              ({formatMoneda(f.salario_promedio, "CRC")}; diario {formatMoneda(f.salario_diario, "CRC")}) —
              comisiones y bonos incluidos, nunca una base reducida. El preaviso, la cesantía (tope 8 años)
              y el aguinaldo son exentos; las <b>vacaciones pendientes son salario</b>: cotizan CCSS y renta
              y se reportan en planilla.
            </p>
          </div>
        </div>
      </CardContent>

      {editar && (
        <FiniquitoDialog
          finiquito={f}
          onCerrar={() => setEditar(false)}
          onCreado={() => setEditar(false)}
          onGuardar={(input) =>
            actualizar.mutate(
              { id: f.id, input },
              {
                onSuccess: () => {
                  toast.success("Finiquito recalculado.");
                  setEditar(false);
                },
                onError,
              },
            )
          }
          pendiente={actualizar.isPending}
        />
      )}

      {confirmar === "aprobar" && (
        <ConfirmDialog
          titulo="Aprobar finiquito"
          descripcion={`${f.empleado_nombre}: se recalcula con los datos vigentes y se congela.`}
          impacto={[`Total ${formatMoneda(f.total, "CRC")}`, "Tras aprobar ya no se edita"]}
          textoConfirmar="Aprobar finiquito"
          pendiente={aprobar.isPending}
          onConfirmar={() => {
            aprobar.mutate(f.id, { onSuccess: () => toast.success("Finiquito aprobado."), onError });
            setConfirmar(null);
          }}
          onCancelar={() => setConfirmar(null)}
        />
      )}
      {confirmar === "pagar" && (
        <ConfirmDialog
          titulo="Marcar finiquito pagado"
          descripcion={`${f.empleado_nombre}: confirmá cuando el depósito esté hecho.`}
          impacto={[
            `Total ${formatMoneda(f.total, "CRC")}`,
            "Se descuentan los saldos de préstamos aplicados",
            "La ficha queda de baja y sus deducciones se cierran",
            "Este paso no se puede deshacer",
          ]}
          textoConfirmar="Marcar pagado"
          pendiente={pagar.isPending}
          onConfirmar={() => {
            pagar.mutate(f.id, { onSuccess: () => toast.success("Finiquito pagado. Ficha dada de baja."), onError });
            setConfirmar(null);
          }}
          onCancelar={() => setConfirmar(null)}
        />
      )}
      {confirmar === "anular" && (
        <ConfirmDialog
          titulo="Anular finiquito"
          descripcion="El finiquito queda anulado (el histórico se conserva) y se puede rehacer."
          textoConfirmar="Anular"
          tono="peligro"
          pendiente={anular.isPending}
          onConfirmar={() => {
            anular.mutate(f.id, {
              onSuccess: () => {
                toast.success("Finiquito anulado.");
                onAnulado();
              },
              onError,
            });
            setConfirmar(null);
          }}
          onCancelar={() => setConfirmar(null)}
        />
      )}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Crear / editar finiquito
// ---------------------------------------------------------------------------

function FiniquitoDialog({
  finiquito,
  onCerrar,
  onCreado,
  onGuardar,
  pendiente,
}: {
  finiquito?: Finiquito;
  onCerrar: () => void;
  onCreado: (id: string) => void;
  onGuardar?: (input: FiniquitoInput) => void;
  pendiente?: boolean;
}) {
  const toast = useToast();
  const crear = useCrearFiniquito();
  const empleadosQ = useEmpleados({ estado: "" });

  const [empleado, setEmpleado] = useState(finiquito?.empleado_id ?? "");
  const [motivo, setMotivo] = useState<MotivoCese>(finiquito?.motivo ?? "RENUNCIA");
  const [fecha, setFecha] = useState(finiquito?.fecha_salida ?? "");
  // En blanco al crear: el backend precarga el saldo pendiente de vacaciones. Al editar se
  // muestra el valor guardado solo si lo digitó RRHH (si vino del saldo, se deja vacío para
  // que se recalcule).
  const [dias, setDias] = useState(
    finiquito && finiquito.dias_vacaciones_manual ? String(toNumber(finiquito.dias_vacaciones)) : "",
  );

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!finiquito && !empleado) return toast.error("Elegí al empleado.");
    if (!fecha) return toast.error("Indicá la fecha de salida.");
    const input: FiniquitoInput = {
      empleado_id: finiquito ? undefined : empleado,
      motivo,
      fecha_salida: fecha,
    };
    // Solo se envían los días si el usuario los escribió: en blanco, el backend usa el
    // SALDO PENDIENTE calculado (y lo recalcula al aprobar, por si disfruta días entre medio).
    const diasEscritos = dias.trim().replace(",", ".");
    if (diasEscritos !== "") input.dias_vacaciones = diasEscritos;
    if (onGuardar) {
      onGuardar(input);
      return;
    }
    crear.mutate(input, {
      onSuccess: (f) => {
        toast.success(`Finiquito calculado: ${formatMoneda(f.total, "CRC")}.`);
        onCreado(f.id);
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onCerrar}>
      <div
        className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold text-content">
          {finiquito ? `Editar finiquito — ${finiquito.empleado_nombre}` : "Nuevo finiquito"}
        </h2>
        <form onSubmit={onSubmit} className="mt-4 flex flex-col gap-3">
          {!finiquito && (
            <Select
              label="Empleado *"
              placeholder={empleadosQ.isPending ? "Cargando…" : "Seleccioná…"}
              value={empleado}
              onChange={(e) => setEmpleado(e.target.value)}
              options={(empleadosQ.data ?? []).map((emp) => ({
                value: emp.id,
                label: emp.nombre + (emp.activo ? "" : " (inactivo)"),
              }))}
            />
          )}
          <Select
            label="Motivo de cese"
            value={motivo}
            onChange={(e) => setMotivo(e.target.value as MotivoCese)}
            options={MOTIVOS}
          />
          <div className="grid grid-cols-2 gap-3">
            <Input label="Fecha de salida *" type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} />
            <Input
              label="Vacaciones pendientes (días)"
              value={dias}
              onChange={(e) => setDias(e.target.value)}
              inputMode="decimal"
              placeholder="Saldo automático"
              className="text-right tabular-nums"
            />
          </div>
          <p className="rounded-lg border border-pendiente/30 bg-pendiente/10 px-3 py-2 text-xs text-content">
            ℹ️ Con <b>despido con responsabilidad patronal</b> se paga preaviso + auxilio de cesantía (tope 8 años).
            En <b>renuncia</b>, fin de contrato o mutuo acuerdo solo vacaciones y aguinaldo proporcionales. El
            adelanto de quincena sin descontar y los saldos de préstamos se descuentan del total.
          </p>
          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="ghost" onClick={onCerrar}>
              Cancelar
            </Button>
            <Button type="submit" loading={pendiente ?? crear.isPending}>
              {finiquito ? "Recalcular" : "Calcular finiquito"}
            </Button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}

// ---------------------------------------------------------------------------
// Provisiones acumuladas del año (corridas pagadas)
// ---------------------------------------------------------------------------

function ProvisionesCard() {
  const [anio, setAnio] = useState(() => new Date().getFullYear());
  const provisionesQ = useProvisiones(anio);
  const items = provisionesQ.data ?? [];

  return (
    <Card>
      <CardContent className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-semibold text-content">Provisiones acumuladas {anio}</h3>
            <p className="text-xs text-content-muted">
              Aguinaldo, vacaciones y cesantía provisionados en las liquidaciones pagadas del año.
            </p>
          </div>
          <Select
            value={String(anio)}
            onChange={(e) => setAnio(Number(e.target.value))}
            options={[anio - 1, anio, anio + 1].map((a) => ({ value: String(a), label: String(a) }))}
          />
        </div>
        {provisionesQ.isPending ? (
          <LoadingState label="Cargando provisiones" />
        ) : provisionesQ.isError ? (
          <ErrorState message={mensajeError(provisionesQ.error)} onRetry={() => provisionesQ.refetch()} />
        ) : items.length === 0 ? (
          <EmptyState message="Sin liquidaciones pagadas en el año." />
        ) : (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Empleado</TH>
                  <TH className="text-center">Meses</TH>
                  <TH className="text-right">Aguinaldo</TH>
                  <TH className="text-right">Vacaciones</TH>
                  <TH className="text-right">Cesantía</TH>
                  <TH className="text-right">Total</TH>
                </TR>
              </THead>
              <TBody>
                {items.map((p) => (
                  <TR key={p.empleado_id}>
                    <TD>
                      <div className="font-medium">{p.nombre}</div>
                      <div className="text-xs text-content-muted">{p.identificacion}</div>
                    </TD>
                    <TD className="text-center">{p.meses}</TD>
                    <TD className="text-right tabular-nums">{formatMoneda(p.aguinaldo, "CRC")}</TD>
                    <TD className="text-right tabular-nums">{formatMoneda(p.vacaciones, "CRC")}</TD>
                    <TD className="text-right tabular-nums">{formatMoneda(p.cesantia, "CRC")}</TD>
                    <TD className="text-right font-semibold tabular-nums">{formatMoneda(p.total, "CRC")}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>
    </Card>
  );
}
