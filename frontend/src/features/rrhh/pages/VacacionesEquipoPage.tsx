/**
 * «Vacaciones del equipo»: la jefatura aprueba o rechaza, y ve de un vistazo quién falta cuándo.
 *
 * LO QUE NO HAY EN ESTA PANTALLA es la razón por la que existe: ni un salario, ni una deducción,
 * ni un finiquito, ni una incapacidad. Antes, para que una jefatura pudiera ver el saldo de días de
 * su gente había que darle `rrhh.ver`, que abre las quince rutas de RRHH. Con
 * `rrhh.vacaciones.equipo` no se abre ninguna.
 *
 * El alcance tampoco se elige acá: el servidor resuelve de qué departamentos sos jefatura. Quien
 * tenga el permiso y no esté cargado como jefatura de ninguno no ve a nadie —un alcance vacío
 * cierra—, y el id de una solicitud ajena tampoco sirve: el recorte por departamento viaja hasta la
 * sentencia que escribe.
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
import { rrhhApi, type SolicitudVacacion } from "@/api/rrhh";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";
import { formatFecha } from "@/lib/format";
import { ESTADO_VACACION } from "./MisVacacionesPage";

export function VacacionesEquipoPage() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();
  const [motivos, setMotivos] = useState<Record<string, string>>({});

  const q = useQuery({
    queryKey: ["rrhh", empresaId, "vacaciones-equipo"],
    queryFn: () => rrhhApi.vacacionesDelEquipo(),
    retry: false,
  });

  const resolver = useMutation({
    mutationFn: (v: { id: string; aprobar: boolean; motivo: string }) =>
      rrhhApi.resolverSolicitud(v.id, v.aprobar, v.motivo),
    onSuccess: (_d, v) => {
      toast.success(v.aprobar ? "Aprobada" : "Rechazada");
      void qc.invalidateQueries({ queryKey: ["rrhh", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  if (q.isLoading) return <LoadingState label="Cargando las vacaciones del equipo" />;
  // Sin jefatura asignada no es un fallo: el permiso abre la puerta, la jefatura dice de quién.
  if (q.isError) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Vacaciones del equipo</h1>
        <ErrorState message={mensajeError(q.error)} />
      </div>
    );
  }

  const { pendientes, calendario, saldos } = q.data!;

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">Vacaciones del equipo</h1>
        <p className="text-sm text-content-muted">
          Lo que espera tu decisión y los días disponibles de tu gente.
        </p>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>
            Esperando tu decisión
            {pendientes.length > 0 && (
              <span className="ml-2 text-base font-normal text-pendiente">{pendientes.length}</span>
            )}
          </CardTitle>
          <CardDescription>
            Mientras no resolvás, los días quedan apartados del saldo de la persona.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {pendientes.length === 0 ? (
            <p className="py-6 text-center text-sm text-positivo">No hay nada pendiente.</p>
          ) : (
            <div className="space-y-3">
              {pendientes.map((s) => (
                <div key={s.id} className="rounded-lg border p-4">
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <div>
                      <span className="font-medium">{s.empleado_nombre}</span>
                      {s.departamento && (
                        <span className="ml-2 text-xs text-content-muted">{s.departamento}</span>
                      )}
                    </div>
                    <div className="text-sm">
                      {formatFecha(s.fecha_inicio)} → {formatFecha(s.fecha_fin)}
                      <span className="ml-2 font-medium tabular-nums">{s.dias} días</span>
                    </div>
                  </div>
                  {s.observaciones && (
                    <p className="mt-1 text-sm text-content-muted">«{s.observaciones}»</p>
                  )}
                  <div className="mt-3 flex flex-wrap items-center gap-2">
                    <Button
                      size="sm"
                      disabled={resolver.isPending}
                      onClick={() => resolver.mutate({ id: s.id, aprobar: true, motivo: "" })}
                    >
                      Aprobar
                    </Button>
                    <Input
                      className="w-64"
                      placeholder="Motivo, si la rechazás"
                      value={motivos[s.id] ?? ""}
                      onChange={(e) => setMotivos((m) => ({ ...m, [s.id]: e.target.value }))}
                    />
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={resolver.isPending || !(motivos[s.id] ?? "").trim()}
                      onClick={() =>
                        resolver.mutate({ id: s.id, aprobar: false, motivo: motivos[s.id] ?? "" })
                      }
                    >
                      Rechazar
                    </Button>
                    {/* El motivo es obligatorio al rechazar: sin él, la persona no sabe si puede
                        volver a pedir las mismas fechas. El servidor lo exige igual. */}
                    {!(motivos[s.id] ?? "").trim() && (
                      <span className="text-xs text-content-muted">
                        Para rechazar, escribí por qué.
                      </span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Días disponibles</CardTitle>
          <CardDescription>
            Cuántos días tiene cada persona. Sin ningún dato de planilla.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Persona</TH>
                  <TH className="text-right">Puede pedir</TH>
                  <TH className="text-right">Ya tomó</TH>
                  <TH className="text-right">Apartados</TH>
                </TR>
              </THead>
              <TBody>
                {saldos.map((s) => (
                  <TR key={s.empleado_id}>
                    <TD>{s.nombre}</TD>
                    <TD className="text-right font-medium tabular-nums">{s.pendiente}</TD>
                    <TD className="text-right tabular-nums text-content-muted">{s.disfrutado}</TD>
                    <TD className="text-right tabular-nums text-content-muted">{s.reservado}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Calendario del equipo</CardTitle>
          <CardDescription>
            Todo lo pedido, aprobado y disfrutado: sirve para ver si podés dar más días esa semana.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {calendario.length === 0 ? (
            <p className="py-6 text-center text-sm text-content-muted">Sin movimientos todavía.</p>
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Persona</TH>
                    <TH>Fechas</TH>
                    <TH className="text-right">Días</TH>
                    <TH>Estado</TH>
                  </TR>
                </THead>
                <TBody>
                  {calendario.map((s) => (
                    <FilaCalendario key={s.id} s={s} />
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

function FilaCalendario({ s }: { s: SolicitudVacacion }) {
  const e = ESTADO_VACACION[s.estado] ?? ESTADO_VACACION.SOLICITADA;
  return (
    <TR className={s.anulada ? "opacity-50" : undefined}>
      <TD>{s.empleado_nombre}</TD>
      <TD>
        {formatFecha(s.fecha_inicio)} → {formatFecha(s.fecha_fin)}
      </TD>
      <TD className="text-right tabular-nums">{s.dias}</TD>
      <TD>
        {s.anulada ? <Badge tone="neutral">retirada</Badge> : <Badge tone={e.tono}>{e.texto}</Badge>}
      </TD>
    </TR>
  );
}
