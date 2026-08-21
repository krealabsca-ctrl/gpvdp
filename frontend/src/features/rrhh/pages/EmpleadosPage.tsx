/**
 * RRHH — Empleados (/rrhh/empleados). Etapa 1 de Nómina (maqueta aprobada):
 * ficha básica para calcular y pagar nómina (salario base, departamento, IBAN
 * para el archivo SINPE) + deducciones recurrentes con cuota, saldo y prioridad.
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
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatFecha, formatMoneda, montoLegible, montoParaApi } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import { useDepartamentos } from "@/features/cxp/hooks";
import {
  useActualizarEmpleado,
  useConceptosNomina,
  useCrearDeduccion,
  useCrearEmpleado,
  useDeducciones,
  useDesactivarDeduccion,
  useDesactivarEmpleado,
  useEmpleados,
} from "@/features/rrhh/hooks";
import { ETIQUETA_FRECUENCIA } from "@/api/rrhh";
import type { DeduccionInput, Empleado, EmpleadoInput, FrecuenciaDeduccion } from "@/api/rrhh";

const JORNADAS = [
  { value: "MENSUAL", label: "Mensual" },
  { value: "QUINCENAL", label: "Quincenal" },
  { value: "SEMANAL", label: "Semanal" },
  { value: "HORAS", label: "Por horas" },
];

export function EmpleadosPage() {
  const tiene = useTienePermiso();
  const puedeGestionar = tiene("rrhh.empleados");

  const [q, setQ] = useState("");
  const [estado, setEstado] = useState<"activo" | "inactivo" | "">("activo");
  const empleadosQ = useEmpleados({ q: q || undefined, estado: estado || undefined });

  const [selId, setSelId] = useState<string | null>(null);
  const [editar, setEditar] = useState<Empleado | "nuevo" | null>(null);

  const empleados = empleadosQ.data ?? [];
  const sel = empleados.find((e) => e.id === selId) ?? null;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Empleados"
        description="Ficha de nómina: salario base, departamento e IBAN de pago. Las comisiones y bonos habituales son salario (base CCSS calculada conforme a la ley)."
        actions={puedeGestionar ? <Button onClick={() => setEditar("nuevo")}>Nuevo empleado</Button> : undefined}
      />

      <div className="flex flex-wrap items-end gap-3">
        <Input
          label="Buscar"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Nombre o identificación…"
          className="w-64"
        />
        <Select
          label="Estado"
          value={estado}
          onChange={(e) => setEstado(e.target.value as typeof estado)}
          options={[
            { value: "activo", label: "Activos" },
            { value: "inactivo", label: "Inactivos" },
            { value: "", label: "Todos" },
          ]}
        />
      </div>

      {empleadosQ.isPending ? (
        <LoadingState label="Cargando empleados" />
      ) : empleadosQ.isError ? (
        <ErrorState message={mensajeError(empleadosQ.error)} onRetry={() => empleadosQ.refetch()} />
      ) : empleados.length === 0 ? (
        <EmptyState
          message={
            puedeGestionar
              ? "No hay empleados con ese filtro. Usá «Nuevo empleado» para registrar la primera ficha."
              : "No hay empleados con ese filtro."
          }
        />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Empleado</TH>
                <TH>Puesto</TH>
                <TH>Departamento</TH>
                <TH>Jornada</TH>
                <TH>Ingreso</TH>
                <TH className="text-right">Salario base</TH>
                <TH className="text-center">Deducciones</TH>
                <TH className="text-right">Acción</TH>
              </TR>
            </THead>
            <TBody>
              {empleados.map((e) => (
                <TR key={e.id} className={cn(!e.activo && "opacity-50", selId === e.id && "bg-accent/5")}>
                  <TD>
                    <div className="font-medium">{e.nombre}</div>
                    <div className="text-xs text-content-muted">
                      {e.identificacion}
                      {!e.activo && e.fecha_salida && ` · salida ${formatFecha(e.fecha_salida)}`}
                    </div>
                  </TD>
                  <TD>{e.puesto || "—"}</TD>
                  <TD>{e.departamento_nombre || "—"}</TD>
                  <TD className="text-xs">{JORNADAS.find((j) => j.value === e.jornada)?.label ?? e.jornada}</TD>
                  <TD className="text-xs">{formatFecha(e.fecha_ingreso)}</TD>
                  <TD className="text-right font-semibold tabular-nums">{formatMoneda(e.salario_base, "CRC")}</TD>
                  <TD className="text-center">
                    {e.deducciones_activas > 0 ? <Badge tone="neutral">{e.deducciones_activas}</Badge> : "—"}
                  </TD>
                  <TD className="text-right">
                    <div className="flex justify-end gap-2">
                      <Button size="sm" variant="secondary" onClick={() => setSelId(selId === e.id ? null : e.id)}>
                        {selId === e.id ? "Ocultar" : "Deducciones"}
                      </Button>
                      {puedeGestionar && (
                        <Button size="sm" variant="ghost" onClick={() => setEditar(e)}>
                          Editar
                        </Button>
                      )}
                    </div>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {sel && <DeduccionesCard empleado={sel} puedeGestionar={puedeGestionar} />}

      {editar && (
        <EmpleadoDialog empleado={editar === "nuevo" ? null : editar} onCerrar={() => setEditar(null)} />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Deducciones recurrentes del empleado (préstamos, ahorro, pensión…)
// ---------------------------------------------------------------------------

function DeduccionesCard({ empleado, puedeGestionar }: { empleado: Empleado; puedeGestionar: boolean }) {
  const toast = useToast();
  const deduccionesQ = useDeducciones(empleado.id);
  const conceptosQ = useConceptosNomina();
  const crear = useCrearDeduccion();
  const desactivar = useDesactivarDeduccion();

  const [concepto, setConcepto] = useState("");
  const [etiqueta, setEtiqueta] = useState("");
  const [cuota, setCuota] = useState("");
  const [saldo, setSaldo] = useState("");
  const [frecuencia, setFrecuencia] = useState<FrecuenciaDeduccion>("MENSUAL");
  const [quitar, setQuitar] = useState<string | null>(null);

  const conceptosDeduccion = (conceptosQ.data ?? []).filter((c) => c.tipo === "DEDUCCION" && c.activo);
  const items = deduccionesQ.data ?? [];

  function onAgregar(e: FormEvent) {
    e.preventDefault();
    const c = montoParaApi(cuota);
    if (!concepto) return toast.error("Elegí el concepto de la deducción.");
    if (!etiqueta.trim()) return toast.error("Poné una etiqueta (ej. «Préstamo Asociación»).");
    if (!c || Number(c) <= 0) return toast.error("Indicá la cuota por quincena/mes.");
    const input: DeduccionInput = {
      concepto_id: concepto,
      etiqueta: etiqueta.trim(),
      cuota: c,
      saldo_total: montoParaApi(saldo) || undefined,
      frecuencia,
    };
    crear.mutate(
      { empleadoId: empleado.id, input },
      {
        onSuccess: () => {
          toast.success("Deducción registrada.");
          setConcepto("");
          setEtiqueta("");
          setCuota("");
          setSaldo("");
          setFrecuencia("MENSUAL");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold text-content">
            Deducciones recurrentes — {empleado.nombre}
          </h3>
          <span className="text-xs text-content-muted">
            La frecuencia decide en qué quincena se cobra. Con saldo, la deducción se corta sola al llegar a
            cero, y la prelación legal (pensión, embargo) se respeta siempre.
          </span>
        </div>

        {puedeGestionar && (
          <form onSubmit={onAgregar} className="grid grid-cols-1 items-end gap-3 sm:grid-cols-6">
            <Select
              label="Concepto *"
              placeholder={conceptosQ.isPending ? "Cargando…" : "Seleccioná…"}
              value={concepto}
              onChange={(e) => setConcepto(e.target.value)}
              options={conceptosDeduccion.map((c) => ({ value: c.id, label: c.nombre }))}
            />
            <Input label="Etiqueta *" value={etiqueta} onChange={(e) => setEtiqueta(e.target.value)} placeholder="Préstamo Asociación" />
            <Input label="Cuota *" value={cuota} onChange={(e) => setCuota(e.target.value)} onBlur={() => setCuota(montoLegible(cuota))} inputMode="decimal" placeholder="45 000,00" className="text-right tabular-nums" />
            <Select
              label="¿Cuándo se cobra?"
              value={frecuencia}
              onChange={(e) => setFrecuencia(e.target.value as FrecuenciaDeduccion)}
              options={(Object.keys(ETIQUETA_FRECUENCIA) as FrecuenciaDeduccion[]).map((f) => ({
                value: f,
                label: ETIQUETA_FRECUENCIA[f],
              }))}
            />
            <Input label="Saldo total" value={saldo} onChange={(e) => setSaldo(e.target.value)} onBlur={() => setSaldo(montoLegible(saldo))} inputMode="decimal" placeholder="Vacío = sin tope" className="text-right tabular-nums" />
            <Button type="submit" loading={crear.isPending}>
              Agregar
            </Button>
          </form>
        )}

        {deduccionesQ.isPending ? (
          <LoadingState label="Cargando deducciones" />
        ) : items.length === 0 ? (
          <EmptyState message="Este empleado no tiene deducciones recurrentes." />
        ) : (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Concepto</TH>
                  <TH>Etiqueta</TH>
                  <TH className="text-right">Cuota</TH>
                  <TH>Se cobra</TH>
                  <TH className="text-right">Saldo restante</TH>
                  <TH>Estado</TH>
                  {puedeGestionar && <TH className="text-right">Acción</TH>}
                </TR>
              </THead>
              <TBody>
                {items.map((d) => (
                  <TR key={d.id} className={cn(!d.activo && "opacity-50")}>
                    <TD>{d.concepto_nombre}</TD>
                    <TD className="font-medium">{d.etiqueta}</TD>
                    <TD className="text-right tabular-nums">{formatMoneda(d.cuota, "CRC")}</TD>
                    <TD className="text-xs">{ETIQUETA_FRECUENCIA[d.frecuencia] ?? d.frecuencia}</TD>
                    <TD className="text-right tabular-nums">
                      {d.saldo_restante ? (
                        <>
                          {formatMoneda(d.saldo_restante, "CRC")}
                          {d.saldo_total && (
                            <span className="text-xs text-content-muted"> / {formatMoneda(d.saldo_total, "CRC")}</span>
                          )}
                        </>
                      ) : (
                        <span className="text-content-muted">sin tope</span>
                      )}
                    </TD>
                    <TD>
                      <Badge tone={d.activo ? "positivo" : "neutral"}>{d.activo ? "Vigente" : "Inactiva"}</Badge>
                    </TD>
                    {puedeGestionar && (
                      <TD className="text-right">
                        {d.activo && (
                          <Button size="sm" variant="ghost" className="text-negativo" onClick={() => setQuitar(d.id)}>
                            Desactivar
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
          titulo="Desactivar deducción"
          descripcion="La deducción deja de aplicarse en las próximas corridas. El histórico se conserva."
          textoConfirmar="Desactivar"
          tono="peligro"
          pendiente={desactivar.isPending}
          onConfirmar={() => {
            desactivar.mutate(
              { empleadoId: empleado.id, id: quitar },
              {
                onSuccess: () => toast.success("Deducción desactivada."),
                onError: (err) => toast.error(mensajeError(err)),
              },
            );
            setQuitar(null);
          }}
          onCancelar={() => setQuitar(null)}
        />
      )}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Crear / editar ficha (rrhh.empleados)
// ---------------------------------------------------------------------------

function EmpleadoDialog({ empleado, onCerrar }: { empleado: Empleado | null; onCerrar: () => void }) {
  const toast = useToast();
  const crear = useCrearEmpleado();
  const actualizar = useActualizarEmpleado();
  const desactivar = useDesactivarEmpleado();
  const departamentosQ = useDepartamentos(true);

  const [nombre, setNombre] = useState(empleado?.nombre ?? "");
  const [tipoId, setTipoId] = useState(empleado?.tipo_identificacion ?? "CEDULA");
  const [identificacion, setIdentificacion] = useState(empleado?.identificacion ?? "");
  const [email, setEmail] = useState(empleado?.email ?? "");
  const [telefono, setTelefono] = useState(empleado?.telefono ?? "");
  const [iban, setIban] = useState(empleado?.iban ?? "");
  const [departamento, setDepartamento] = useState(empleado?.departamento_id ?? "");
  const [puesto, setPuesto] = useState(empleado?.puesto ?? "");
  const [fechaIngreso, setFechaIngreso] = useState(empleado?.fecha_ingreso ?? "");
  const [salario, setSalario] = useState(empleado ? montoLegible(empleado.salario_base) : "");
  const [jornada, setJornada] = useState(empleado?.jornada ?? "MENSUAL");
  const [hijos, setHijos] = useState(String(empleado?.hijos ?? 0));
  const [conyuge, setConyuge] = useState(empleado?.conyuge ?? false);
  const [confirmarBaja, setConfirmarBaja] = useState(false);

  const pendiente = crear.isPending || actualizar.isPending;

  function onGuardar(e: FormEvent) {
    e.preventDefault();
    const s = montoParaApi(salario);
    if (!nombre.trim()) return toast.error("Indicá el nombre completo.");
    if (!identificacion.trim()) return toast.error("Indicá la identificación.");
    if (!fechaIngreso) return toast.error("Indicá la fecha de ingreso.");
    if (!s || Number(s) <= 0) return toast.error("Indicá el salario base (mayor a cero).");
    const input: EmpleadoInput = {
      nombre: nombre.trim(),
      tipo_identificacion: tipoId,
      identificacion: identificacion.trim(),
      email: email.trim() || undefined,
      telefono: telefono.trim() || undefined,
      iban: iban.trim() || undefined,
      departamento_id: departamento || undefined,
      puesto: puesto.trim() || undefined,
      fecha_ingreso: fechaIngreso,
      salario_base: s,
      jornada,
      hijos: Math.max(0, Number(hijos) || 0),
      conyuge,
    };
    const done = {
      onSuccess: () => {
        toast.success(empleado ? "Ficha actualizada." : "Empleado registrado.");
        onCerrar();
      },
      onError: (err: unknown) => toast.error(mensajeError(err)),
    };
    if (empleado) actualizar.mutate({ id: empleado.id, input }, done);
    else crear.mutate(input, done);
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onCerrar}>
      <div
        className="max-h-[90vh] w-full max-w-2xl overflow-y-auto rounded-xl border border-border bg-surface-raised p-5 shadow-lifted"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-base font-semibold text-content">
          {empleado ? `Editar ficha — ${empleado.nombre}` : "Nuevo empleado"}
        </h2>
        <form onSubmit={onGuardar} className="mt-4 flex flex-col gap-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Input label="Nombre completo *" value={nombre} onChange={(e) => setNombre(e.target.value)} className="sm:col-span-2" />
            <Input label="Puesto" value={puesto} onChange={(e) => setPuesto(e.target.value)} placeholder="Ej. Tanatopractor" />
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Select
              label="Tipo de identificación"
              value={tipoId}
              onChange={(e) => setTipoId(e.target.value as typeof tipoId)}
              options={[
                { value: "CEDULA", label: "Cédula" },
                { value: "DIMEX", label: "DIMEX" },
                { value: "PASAPORTE", label: "Pasaporte" },
              ]}
            />
            <Input label="Identificación *" value={identificacion} onChange={(e) => setIdentificacion(e.target.value)} placeholder="1-1420-0356" />
            <Select
              label="Departamento"
              placeholder="Seleccioná…"
              value={departamento}
              onChange={(e) => setDepartamento(e.target.value)}
              options={(departamentosQ.data ?? []).map((d) => ({ value: d.id, label: d.nombre }))}
            />
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Input label="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
            <Input label="Teléfono" value={telefono} onChange={(e) => setTelefono(e.target.value)} />
            <Input label="IBAN (pago SINPE)" value={iban} onChange={(e) => setIban(e.target.value)} placeholder="CR05 0102 0000 …" />
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Input label="Fecha de ingreso *" type="date" value={fechaIngreso} onChange={(e) => setFechaIngreso(e.target.value)} />
            <Input label="Salario base *" value={salario} onChange={(e) => setSalario(e.target.value)} onBlur={() => setSalario(montoLegible(salario))} inputMode="decimal" placeholder="480 000,00" className="text-right tabular-nums" />
            <Select label="Jornada" value={jornada} onChange={(e) => setJornada(e.target.value as typeof jornada)} options={JORNADAS} />
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Input label="Hijos (crédito fiscal)" value={hijos} onChange={(e) => setHijos(e.target.value)} inputMode="numeric" className="text-right tabular-nums" />
            <Select
              label="Cónyuge (crédito fiscal)"
              value={conyuge ? "si" : "no"}
              onChange={(e) => setConyuge(e.target.value === "si")}
              options={[
                { value: "no", label: "No" },
                { value: "si", label: "Sí" },
              ]}
            />
          </div>
          <p className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-xs text-content-muted">
            El salario base es la referencia de la ficha; las comisiones, extras y bonos se agregan por corrida y
            <b> siempre</b> forman parte de la base CCSS (conceptos bloqueados por ley).
          </p>
          <div className="flex justify-between gap-2 pt-1">
            <div>
              {empleado && empleado.activo && (
                <Button type="button" variant="ghost" className="text-negativo" onClick={() => setConfirmarBaja(true)}>
                  Dar de baja
                </Button>
              )}
            </div>
            <div className="flex gap-2">
              <Button type="button" variant="ghost" onClick={onCerrar}>
                Cancelar
              </Button>
              <Button type="submit" loading={pendiente}>
                {empleado ? "Guardar" : "Registrar empleado"}
              </Button>
            </div>
          </div>
        </form>

        {confirmarBaja && (
          <ConfirmDialog
            titulo="Dar de baja al empleado"
            descripcion={`${nombre || "El empleado"} queda inactivo con fecha de salida hoy. La ficha y su histórico se conservan (la liquidación se calcula en la Etapa de corridas).`}
            textoConfirmar="Dar de baja"
            tono="peligro"
            pendiente={desactivar.isPending}
            onConfirmar={() => {
              setConfirmarBaja(false);
              if (!empleado) return;
              desactivar.mutate(
                { id: empleado.id },
                {
                  onSuccess: () => {
                    toast.success("Empleado dado de baja.");
                    onCerrar();
                  },
                  onError: (err) => toast.error(mensajeError(err)),
                },
              );
            }}
            onCancelar={() => setConfirmarBaja(false)}
          />
        )}
      </div>
    </div>,
    document.body,
  );
}
