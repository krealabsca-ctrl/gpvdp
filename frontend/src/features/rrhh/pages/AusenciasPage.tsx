/**
 * RRHH — Incapacidades y vacaciones (/rrhh/ausencias). Pantalla de la maqueta aprobada:
 * el ausentismo del período y los saldos por empleado. Lo que se registra acá entra
 * automáticamente a la corrida del mes.
 *
 * Política de subsidios (confirmada por el DF): la empresa paga lo que la ley le obliga
 * — CCSS: los 3 primeros días al 50%; INS: el día del accidente completo — y el resto lo
 * cubre la entidad directo al trabajador.
 */

import { useState, type FormEvent } from "react";
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
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatFecha, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useAnularIncapacidad,
  useAnularVacacion,
  useAvisarVacaciones,
  useEmpleados,
  useIncapacidades,
  useRegistrarIncapacidad,
  useRegistrarVacacion,
  useSaldosVacaciones,
  useVacaciones,
} from "@/features/rrhh/hooks";
import type { EntidadIncapacidad, IncapacidadInput, VacacionInput } from "@/api/rrhh";

const MESES = [
  "Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
  "Julio", "Agosto", "Septiembre", "Octubre", "Noviembre", "Diciembre",
];

export function AusenciasPage() {
  const tiene = useTienePermiso();
  const puedeRegistrar = tiene("rrhh.ausencias");
  const hoy = new Date();
  const [anio, setAnio] = useState(hoy.getFullYear());
  const [mes, setMes] = useState(hoy.getMonth() + 1);

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Incapacidades y vacaciones"
        description="El ausentismo del período y los saldos por empleado. Lo que se registra acá entra automáticamente a la corrida del mes."
        actions={
          <div className="flex items-center gap-2">
            <Select
              value={String(mes)}
              onChange={(e) => setMes(Number(e.target.value))}
              options={MESES.map((m, i) => ({ value: String(i + 1), label: m }))}
            />
            <Select
              value={String(anio)}
              onChange={(e) => setAnio(Number(e.target.value))}
              options={[anio - 1, anio, anio + 1].map((a) => ({ value: String(a), label: String(a) }))}
            />
          </div>
        }
      />

      <IncapacidadesCard anio={anio} mes={mes} puedeRegistrar={puedeRegistrar} />
      <VacacionesCard anio={anio} puedeRegistrar={puedeRegistrar} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Incapacidades del período
// ---------------------------------------------------------------------------

function IncapacidadesCard({ anio, mes, puedeRegistrar }: { anio: number; mes: number; puedeRegistrar: boolean }) {
  const toast = useToast();
  const incapacidadesQ = useIncapacidades(anio, mes);
  const empleadosQ = useEmpleados({ estado: "activo" });
  const registrar = useRegistrarIncapacidad();
  const anular = useAnularIncapacidad();

  const [empleado, setEmpleado] = useState("");
  const [entidad, setEntidad] = useState<EntidadIncapacidad>("CCSS");
  const [fecha, setFecha] = useState("");
  const [dias, setDias] = useState("");
  const [boleta, setBoleta] = useState("");
  const [quitar, setQuitar] = useState<string | null>(null);

  const items = incapacidadesQ.data ?? [];

  function onRegistrar(e: FormEvent) {
    e.preventDefault();
    const n = Number(dias);
    if (!empleado) return toast.error("Elegí al empleado incapacitado.");
    if (!fecha) return toast.error("Indicá el primer día de la incapacidad.");
    if (!Number.isInteger(n) || n < 1) return toast.error("Los días deben ser un número entero mayor a cero.");
    // El listado muestra el mes seleccionado: si la boleta arranca en otro mes, el registro
    // quedaría fuera de la vista y es fácil volver a digitarlo (doble descuento).
    const partes = fecha.split("-").map(Number);
    const anioF = partes[0] ?? 0;
    const mesF = partes[1] ?? 0;
    if (anioF !== anio || mesF !== mes) {
      return toast.error(
        `Esa fecha es de ${MESES[mesF - 1] ?? "otro mes"} ${anioF} y estás viendo ${MESES[mes - 1]} ${anio}. ` +
          "Cambiá el mes de arriba para registrarla y verla en la lista.",
      );
    }
    const input: IncapacidadInput = {
      empleado_id: empleado,
      entidad,
      fecha_inicio: fecha,
      dias: n,
      boleta: boleta.trim() || undefined,
    };
    registrar.mutate(input, {
      onSuccess: (inc) => {
        toast.success(inc.subsidio);
        setEmpleado("");
        setFecha("");
        setDias("");
        setBoleta("");
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-3">
        <div>
          <h3 className="text-sm font-semibold text-content">Incapacidades de {MESES[mes - 1]} {anio}</h3>
          <p className="text-xs text-content-muted">
            Se muestran las que tocan el mes, aunque hayan empezado antes. Los días que cubre la CCSS o el INS
            no los paga la empresa y por eso no cotizan: la colilla lo deja por escrito.
          </p>
        </div>

        {puedeRegistrar && (
          <form onSubmit={onRegistrar} className="grid grid-cols-1 items-end gap-3 sm:grid-cols-6">
            <Select
              label="Empleado *"
              placeholder={empleadosQ.isPending ? "Cargando…" : "Seleccioná…"}
              value={empleado}
              onChange={(e) => setEmpleado(e.target.value)}
              options={(empleadosQ.data ?? []).map((emp) => ({ value: emp.id, label: emp.nombre }))}
            />
            <Select
              label="Entidad"
              value={entidad}
              onChange={(e) => setEntidad(e.target.value as EntidadIncapacidad)}
              options={[
                { value: "CCSS", label: "CCSS · enfermedad" },
                { value: "INS", label: "INS · riesgo del trabajo" },
              ]}
            />
            <Input label="Primer día *" type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} />
            <Input label="Días *" value={dias} onChange={(e) => setDias(e.target.value)} inputMode="numeric" placeholder="5" className="text-right tabular-nums" />
            <Input label="Boleta" value={boleta} onChange={(e) => setBoleta(e.target.value)} placeholder="N.º de boleta" />
            <Button type="submit" loading={registrar.isPending}>
              Registrar
            </Button>
          </form>
        )}

        <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-xs text-content-muted">
          ⚖️ <b>CCSS (enfermedad):</b> la empresa paga el 50% del salario los 3 primeros días; del cuarto en
          adelante la CCSS gira su subsidio directo al trabajador. <b>INS (riesgo del trabajo):</b> la empresa
          paga completo el día del accidente y desde el día siguiente el subsidio lo paga el INS.
        </p>

        {incapacidadesQ.isPending ? (
          <LoadingState label="Cargando incapacidades" />
        ) : incapacidadesQ.isError ? (
          <ErrorState message={mensajeError(incapacidadesQ.error)} onRetry={() => incapacidadesQ.refetch()} />
        ) : items.length === 0 ? (
          <EmptyState message={`Sin incapacidades en ${MESES[mes - 1]} de ${anio}.`} />
        ) : (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Empleado</TH>
                  <TH>Entidad</TH>
                  <TH>Período</TH>
                  <TH className="text-center">Días</TH>
                  <TH>Quién paga qué</TH>
                  <TH>Boleta</TH>
                  {puedeRegistrar && <TH className="text-right">Acción</TH>}
                </TR>
              </THead>
              <TBody>
                {items.map((i) => (
                  <TR key={i.id} className={cn(i.anulada && "opacity-50")}>
                    <TD className="font-medium">{i.empleado_nombre}</TD>
                    <TD>
                      <Badge tone={i.entidad === "CCSS" ? "accent" : "pendiente"}>{i.entidad}</Badge>
                    </TD>
                    <TD className="text-xs">
                      {formatFecha(i.fecha_inicio)} → {formatFecha(i.fecha_fin)}
                    </TD>
                    <TD className="text-center tabular-nums">{i.dias}</TD>
                    <TD className="max-w-80 text-xs text-content-muted">{i.subsidio}</TD>
                    <TD className="text-xs">{i.boleta || "—"}</TD>
                    {puedeRegistrar && (
                      <TD className="text-right">
                        {!i.anulada && (
                          <Button size="sm" variant="ghost" className="text-negativo" onClick={() => setQuitar(i.id)}>
                            Anular
                          </Button>
                        )}
                      </TD>
                    )}
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>

      {quitar && (
        <ConfirmDialog
          titulo="Anular incapacidad"
          descripcion="La incapacidad deja de afectar la corrida del período. El registro se conserva en el histórico."
          textoConfirmar="Anular"
          tono="peligro"
          pendiente={anular.isPending}
          onConfirmar={() => {
            anular.mutate(quitar, {
              onSuccess: () => toast.success("Incapacidad anulada."),
              onError: (err) => toast.error(mensajeError(err)),
            });
            setQuitar(null);
          }}
          onCancelar={() => setQuitar(null)}
        />
      )}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Saldos de vacaciones y registro de disfrute
// ---------------------------------------------------------------------------

function VacacionesCard({ anio, puedeRegistrar }: { anio: number; puedeRegistrar: boolean }) {
  const toast = useToast();
  const saldosQ = useSaldosVacaciones(anio);
  const vacacionesQ = useVacaciones();
  const registrar = useRegistrarVacacion();
  const anular = useAnularVacacion();
  const avisar = useAvisarVacaciones();

  const [empleado, setEmpleado] = useState("");
  const [fecha, setFecha] = useState("");
  const [dias, setDias] = useState("");
  const [quitar, setQuitar] = useState<string | null>(null);
  // Cuál fila está enviando su aviso (para poner el spinner solo en esa).
  const [avisando, setAvisando] = useState<string | null>(null);
  const [verHistorial, setVerHistorial] = useState(false);

  const saldos = saldosQ.data ?? [];
  const historial = vacacionesQ.data ?? [];
  const saldoElegido = saldos.find((s) => s.empleado_id === empleado);

  function onRegistrar(e: FormEvent) {
    e.preventDefault();
    if (!empleado) return toast.error("Elegí al empleado.");
    if (!fecha) return toast.error("Indicá el primer día de vacaciones.");
    const n = Number(dias.replace(",", "."));
    if (!(n > 0)) return toast.error("Indicá cuántos días toma.");
    const input: VacacionInput = { empleado_id: empleado, fecha_inicio: fecha, dias: String(n) };
    registrar.mutate(input, {
      onSuccess: () => {
        toast.success("Vacaciones registradas; el saldo quedó actualizado.");
        setEmpleado("");
        setFecha("");
        setDias("");
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-3">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <h3 className="text-sm font-semibold text-content">Saldo de vacaciones</h3>
            <p className="text-xs text-content-muted">
              Se acumula 1 día por mes trabajado (art. 153 del Código de Trabajo). El saldo pendiente es el que
              el finiquito paga si la persona sale sin haberlas tomado.
            </p>
          </div>
          <Button size="sm" variant="secondary" onClick={() => setVerHistorial((v) => !v)}>
            {verHistorial ? "Ver saldos" : `Ver historial (${historial.length})`}
          </Button>
        </div>

        {puedeRegistrar && !verHistorial && (
          <form onSubmit={onRegistrar} className="grid grid-cols-1 items-end gap-3 sm:grid-cols-4">
            <Select
              label="Empleado *"
              placeholder={saldosQ.isPending ? "Cargando…" : "Seleccioná…"}
              value={empleado}
              onChange={(e) => setEmpleado(e.target.value)}
              options={saldos.map((s) => ({
                value: s.empleado_id,
                label: `${s.nombre} — ${toNumber(s.pendiente)} días disponibles`,
              }))}
            />
            <Input label="Primer día *" type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} />
            <Input
              label={saldoElegido ? `Días (disponibles: ${toNumber(saldoElegido.pendiente)})` : "Días *"}
              value={dias}
              onChange={(e) => setDias(e.target.value)}
              inputMode="decimal"
              placeholder="5"
              className="text-right tabular-nums"
            />
            <Button type="submit" loading={registrar.isPending}>
              Registrar disfrute
            </Button>
          </form>
        )}

        {verHistorial ? (
          vacacionesQ.isPending ? (
            <LoadingState label="Cargando historial" />
          ) : vacacionesQ.isError ? (
            <ErrorState message={mensajeError(vacacionesQ.error)} onRetry={() => vacacionesQ.refetch()} />
          ) : historial.length === 0 ? (
            <EmptyState message="Todavía no hay vacaciones registradas." />
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Empleado</TH>
                    <TH>Desde</TH>
                    <TH className="text-center">Días</TH>
                    <TH>Estado</TH>
                    {puedeRegistrar && <TH className="text-right">Acción</TH>}
                  </TR>
                </THead>
                <TBody>
                  {historial.map((v) => (
                    <TR key={v.id} className={cn(v.anulada && "opacity-50")}>
                      <TD className="font-medium">{v.empleado_nombre}</TD>
                      <TD className="text-xs">{formatFecha(v.fecha_inicio)}</TD>
                      <TD className="text-center tabular-nums">{toNumber(v.dias)}</TD>
                      <TD>
                        <Badge tone={v.anulada ? "neutral" : "positivo"}>{v.anulada ? "Anulada" : "Vigente"}</Badge>
                      </TD>
                      {puedeRegistrar && (
                        <TD className="text-right">
                          {!v.anulada && (
                            <>
                              {/* El aviso va al correo de la ficha del colaborador; el texto se
                                  configura en Configuración → Notificaciones. */}
                              <Button
                                size="sm"
                                variant="ghost"
                                loading={avisar.isPending && avisando === v.id}
                                onClick={() => {
                                  setAvisando(v.id);
                                  avisar.mutate(v.id, {
                                    onSuccess: () =>
                                      toast.success(`Aviso enviado a ${v.empleado_nombre}.`),
                                    onError: (err) => toast.error(mensajeError(err)),
                                    onSettled: () => setAvisando(null),
                                  });
                                }}
                              >
                                Avisar al colaborador
                              </Button>
                              <Button size="sm" variant="ghost" className="text-negativo" onClick={() => setQuitar(v.id)}>
                                Anular
                              </Button>
                            </>
                          )}
                        </TD>
                      )}
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          )
        ) : saldosQ.isPending ? (
          <LoadingState label="Cargando saldos" />
        ) : saldosQ.isError ? (
          <ErrorState message={mensajeError(saldosQ.error)} onRetry={() => saldosQ.refetch()} />
        ) : saldos.length === 0 ? (
          <EmptyState message="No hay empleados con saldo de vacaciones." />
        ) : (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Empleado</TH>
                  <TH>Ingreso</TH>
                  <TH className="text-center">Meses</TH>
                  <TH className="text-right">Acumulado</TH>
                  <TH className="text-right">Disfrutado</TH>
                  <TH className="text-right">Pendiente</TH>
                </TR>
              </THead>
              <TBody>
                {saldos.map((s) => (
                  <TR key={s.empleado_id}>
                    <TD>
                      <div className="font-medium">{s.nombre}</div>
                      <div className="text-xs text-content-muted">{s.identificacion}</div>
                    </TD>
                    <TD className="text-xs">{formatFecha(s.fecha_ingreso)}</TD>
                    <TD className="text-center tabular-nums">{s.meses_servicio}</TD>
                    <TD className="text-right tabular-nums">{toNumber(s.acumulado)} d</TD>
                    <TD className="text-right tabular-nums text-content-muted">{toNumber(s.disfrutado)} d</TD>
                    <TD className="text-right font-semibold tabular-nums">{toNumber(s.pendiente)} d</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>

      {quitar && (
        <ConfirmDialog
          titulo="Anular vacaciones"
          descripcion="Los días vuelven al saldo del empleado. El registro se conserva en el histórico."
          textoConfirmar="Anular"
          tono="peligro"
          pendiente={anular.isPending}
          onConfirmar={() => {
            anular.mutate(quitar, {
              onSuccess: () => toast.success("Vacaciones anuladas; los días volvieron al saldo."),
              onError: (err) => toast.error(mensajeError(err)),
            });
            setQuitar(null);
          }}
          onCancelar={() => setQuitar(null)}
        />
      )}
    </Card>
  );
}
