/**
 * «Mis vacaciones»: el colaborador ve su saldo y pide días. Nada más.
 *
 * Esta pantalla existe porque hasta ahora consultar el saldo propio exigía `rrhh.ver`, que abre
 * quince rutas: los salarios de todos, sus deducciones, las corridas, los finiquitos y las
 * incapacidades —datos de salud—. Pedir vacaciones no puede costar eso.
 *
 * Con `rrhh.vacaciones.mias` no se abre ninguna de esas quince. Y el servidor no acepta «pedir
 * vacaciones PARA el empleado X»: resuelve quién sos desde tu usuario, así que el permiso se puede
 * dar a toda la empresa sin que nadie alcance la ficha de otro.
 */
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  ErrorState,
  Input,
  LoadingState,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import { rrhhApi, type EstadoVacacion, type SolicitudVacacion } from "@/api/rrhh";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";
import { formatFecha } from "@/lib/format";

/**
 * Cómo se nombra cada estado para quien pidió los días.
 *
 * «Aprobada» y «confirmada» se distinguen a propósito: para el colaborador son dos cosas
 * distintas —una es el permiso de faltar, la otra el registro de que faltó— y mezclarlas haría que
 * no entienda por qué su saldo todavía no bajó.
 */
export const ESTADO_VACACION: Record<
  EstadoVacacion,
  { texto: string; tono: "neutral" | "accent" | "positivo" | "negativo" | "pendiente"; ayuda: string }
> = {
  SOLICITADA: {
    texto: "esperando a tu jefatura",
    tono: "pendiente",
    ayuda: "Todavía podés retirarla. Los días ya están apartados de tu saldo.",
  },
  APROBADA: {
    texto: "aprobada",
    tono: "accent",
    ayuda: "Tu jefatura dio el visto. Recursos Humanos la confirma cuando la disfrutes.",
  },
  CONFIRMADA: {
    texto: "disfrutada",
    tono: "positivo",
    ayuda: "Recursos Humanos ya la registró: estos días cuentan como tomados.",
  },
  RECHAZADA: {
    texto: "rechazada",
    tono: "negativo",
    ayuda: "No se dio. Los días volvieron a tu saldo y podés pedir otras fechas.",
  },
};

export function MisVacacionesPage() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();

  const [fecha, setFecha] = useState("");
  const [dias, setDias] = useState("");
  const [nota, setNota] = useState("");

  const q = useQuery({
    queryKey: ["rrhh", empresaId, "mis-vacaciones"],
    queryFn: () => rrhhApi.misVacaciones(),
    retry: false,
  });

  const pedir = useMutation({
    mutationFn: () => rrhhApi.pedirVacaciones({ fecha_inicio: fecha, dias, observaciones: nota }),
    onSuccess: () => {
      toast.success("Solicitud enviada a tu jefatura");
      setFecha("");
      setDias("");
      setNota("");
      void qc.invalidateQueries({ queryKey: ["rrhh", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const cancelar = useMutation({
    mutationFn: (id: string) => rrhhApi.cancelarMiSolicitud(id),
    onSuccess: () => {
      toast.success("Solicitud retirada");
      void qc.invalidateQueries({ queryKey: ["rrhh", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  if (q.isLoading) return <LoadingState label="Cargando tus vacaciones" />;

  // Sin ficha ligada NO es un fallo: es alguien que entra al ERP y no está en planilla, o una
  // ficha a la que todavía no le asignaron su usuario. El mensaje del servidor dice qué pedir.
  if (q.isError) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Mis vacaciones</h1>
        <ErrorState message={mensajeError(q.error)} />
      </div>
    );
  }

  const saldo = q.data!.saldo;
  const sols = q.data!.solicitudes;

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">Mis vacaciones</h1>
        <p className="text-sm text-content-muted">
          Tu saldo y tus solicitudes. Nadie más ve esta pantalla.
        </p>
      </header>

      <div className="grid gap-4 sm:grid-cols-3">
        <Dato n={saldo.pendiente} label="días podés pedir hoy" grande />
        <Dato n={saldo.disfrutado} label="ya disfrutados" />
        <Dato n={saldo.reservado} label="pedidos o aprobados sin tomar" />
      </div>
      <p className="text-xs text-content-muted">
        Acumulás {saldo.acumulado} días por {saldo.meses_servicio} meses de servicio desde el{" "}
        {formatFecha(saldo.fecha_ingreso)}.
      </p>

      <Card>
        <CardHeader>
          <CardTitle>Pedir días</CardTitle>
          <CardDescription>
            Se envía a tu jefatura. Mientras espera respuesta, los días quedan apartados de tu saldo.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              pedir.mutate();
            }}
          >
            <label className="text-sm">
              <span className="mb-1 block text-content-muted">Desde</span>
              <Input type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} required />
            </label>
            <label className="text-sm">
              <span className="mb-1 block text-content-muted">Días</span>
              <Input
                type="number"
                min="0.5"
                step="0.5"
                className="w-24"
                value={dias}
                onChange={(e) => setDias(e.target.value)}
                required
              />
            </label>
            <label className="min-w-[14rem] flex-1 text-sm">
              <span className="mb-1 block text-content-muted">Motivo (opcional)</span>
              <Input value={nota} onChange={(e) => setNota(e.target.value)} />
            </label>
            <Button type="submit" disabled={pedir.isPending || !fecha || !dias}>
              {pedir.isPending ? "Enviando…" : "Pedir"}
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Lo que pedí</CardTitle>
        </CardHeader>
        <CardContent>
          {sols.length === 0 ? (
            <p className="py-6 text-center text-sm text-content-muted">Todavía no pediste vacaciones.</p>
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Fechas</TH>
                    <TH>Días</TH>
                    <TH>Estado</TH>
                    <TH>Detalle</TH>
                    <TH />
                  </TR>
                </THead>
                <TBody>
                  {sols.map((s) => (
                    <FilaSolicitud
                      key={s.id}
                      s={s}
                      onCancelar={() => cancelar.mutate(s.id)}
                      cancelando={cancelar.isPending}
                    />
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function FilaSolicitud(props: { s: SolicitudVacacion; onCancelar: () => void; cancelando: boolean }) {
  const { s } = props;
  const e = ESTADO_VACACION[s.estado] ?? ESTADO_VACACION.SOLICITADA;
  return (
    <TR className={s.anulada ? "opacity-50" : undefined}>
      <TD>
        {formatFecha(s.fecha_inicio)} → {formatFecha(s.fecha_fin)}
      </TD>
      <TD className="tabular-nums">{s.dias}</TD>
      <TD>
        {s.anulada ? <Badge tone="neutral">retirada</Badge> : <Badge tone={e.tono}>{e.texto}</Badge>}
      </TD>
      <TD className="text-xs text-content-muted">
        {s.estado === "RECHAZADA" && s.motivo_rechazo ? s.motivo_rechazo : e.ayuda}
        {s.aprobada_por && <div className="mt-0.5">Resolvió: {s.aprobada_por}</div>}
      </TD>
      <TD className="text-right">
        {s.estado === "SOLICITADA" && !s.anulada && (
          <Button variant="ghost" size="sm" onClick={props.onCancelar} disabled={props.cancelando}>
            Retirar
          </Button>
        )}
      </TD>
    </TR>
  );
}

function Dato(props: { n: string; label: string; grande?: boolean }) {
  return (
    <div className="rounded-lg border p-4">
      <div className={props.grande ? "text-3xl font-semibold text-accent" : "text-2xl font-semibold"}>
        {props.n}
      </div>
      <div className="text-sm text-content-muted">{props.label}</div>
    </div>
  );
}
