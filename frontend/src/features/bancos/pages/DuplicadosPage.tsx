/**
 * «Duplicados y exclusiones»: sacar de los libros un movimiento que el banco cargó dos veces.
 *
 * El caso que la originó (9-oct-2026): «el BAC por alguna razón me cambió el consecutivo, pero veo
 * que los últimos 4 dígitos conservan la base aunque la descripción no, pero el monto y la fecha
 * sí. El detalle es que cuando se cargó ya no puedo excluir el movimiento y se malogró la data, ya
 * no es fidedigna.»
 *
 * Hasta ahora la única salida era revertir la carga ENTERA y volver a subirla sin el duplicado, lo
 * que en la práctica significaba no sacarlo nunca.
 *
 * LA PANTALLA PROPONE, NO DECIDE. Muestra los pares con sus tres señales y deja que una persona
 * mire. No es prudencia de más: probada la regla «mismos cuatro dígitos» a secas sobre los datos
 * reales, devolvió 84 pares que eran comisiones y SINPE legítimos repetidos el mismo día.
 * Excluirlos habría sacado plata real de los libros.
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
import { bancosApi, type ParDuplicado } from "@/api/bancos";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";
import { formatFecha, formatMonto } from "@/lib/format";

/** Primer y último día del mes en curso, que es con lo que se entra a revisar. */
function mesEnCurso(): { desde: string; hasta: string } {
  const hoy = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  const a = hoy.getFullYear();
  const m = hoy.getMonth() + 1;
  const ultimo = new Date(a, m, 0).getDate();
  return { desde: `${a}-${p(m)}-01`, hasta: `${a}-${p(m)}-${p(ultimo)}` };
}

export function DuplicadosPage() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();
  const inicial = mesEnCurso();
  const [desde, setDesde] = useState(inicial.desde);
  const [hasta, setHasta] = useState(inicial.hasta);
  const [rango, setRango] = useState(inicial);

  const dupsQ = useQuery({
    queryKey: ["bancos", empresaId, "duplicados", rango.desde, rango.hasta],
    queryFn: () => bancosApi.duplicadosSospechosos(rango.desde, rango.hasta),
  });
  const excQ = useQuery({
    queryKey: ["bancos", empresaId, "excluidos"],
    queryFn: () => bancosApi.movimientosExcluidos(),
  });

  const excluir = useMutation({
    mutationFn: (v: { id: string; motivo: string }) => bancosApi.excluirMovimiento(v.id, v.motivo),
    onSuccess: () => {
      toast.success("Movimiento fuera de los libros");
      void qc.invalidateQueries({ queryKey: ["bancos", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const incluir = useMutation({
    mutationFn: (id: string) => bancosApi.incluirMovimiento(id),
    onSuccess: () => {
      toast.success("Movimiento devuelto a los libros");
      void qc.invalidateQueries({ queryKey: ["bancos", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const excluidos = excQ.data?.items ?? [];

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">Duplicados y exclusiones</h1>
        <p className="text-sm text-content-muted">
          Movimientos que el banco pudo cargar dos veces, y lo que ya se sacó de los libros.
        </p>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>
            Posibles duplicados
            {dupsQ.data && dupsQ.data.con_las_tres > 0 && (
              <span className="ml-2 text-base font-normal text-pendiente">
                {dupsQ.data.con_las_tres} con las tres señales
              </span>
            )}
          </CardTitle>
          <CardDescription>
            Mismo día, mismo monto y misma cuenta, con consecutivos distintos. Las{" "}
            <strong>tres señales juntas</strong> son la huella de un cambio de formato del banco;
            una sola puede ser coincidencia —dos comisiones iguales el mismo día—, así que el
            sistema no excluye nada por su cuenta.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              setRango({ desde, hasta });
            }}
          >
            <label className="text-sm">
              <span className="mb-1 block text-content-muted">Desde</span>
              <Input type="date" value={desde} onChange={(e) => setDesde(e.target.value)} />
            </label>
            <label className="text-sm">
              <span className="mb-1 block text-content-muted">Hasta</span>
              <Input type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} />
            </label>
            <Button type="submit" variant="ghost">
              Buscar
            </Button>
          </form>

          {dupsQ.isLoading && <LoadingState label="Buscando duplicados" />}
          {dupsQ.isError && <ErrorState message={mensajeError(dupsQ.error)} />}
          {dupsQ.data && dupsQ.data.pares.length === 0 && (
            <p className="py-6 text-center text-sm text-positivo">
              No hay movimientos repetidos en ese rango.
            </p>
          )}
          {dupsQ.data?.pares.map((p) => (
            <ParSospechoso
              key={`${p.a.id}-${p.b.id}`}
              p={p}
              onExcluir={(id, motivo) => excluir.mutate({ id, motivo })}
              trabajando={excluir.isPending}
            />
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>
            Fuera de los libros
            {excluidos.length > 0 && (
              <span className="ml-2 text-base font-normal text-content-muted">
                {excluidos.length}
              </span>
            )}
          </CardTitle>
          <CardDescription>
            Lo que alguien sacó a mano, con quién y por qué. No se borró nada: todo se puede
            devolver.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {excQ.isLoading && <LoadingState label="Cargando" />}
          {excluidos.length === 0 && !excQ.isLoading && (
            <p className="py-6 text-center text-sm text-content-muted">
              No hay movimientos excluidos a mano.
            </p>
          )}
          {excluidos.length > 0 && (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Fecha</TH>
                    <TH>Consecutivo</TH>
                    <TH className="text-right">Monto</TH>
                    <TH>Cuenta</TH>
                    <TH>Motivo</TH>
                    <TH>Quién</TH>
                    <TH />
                  </TR>
                </THead>
                <TBody>
                  {excluidos.map((m) => (
                    <TR key={m.id}>
                      <TD>{formatFecha(m.fecha)}</TD>
                      <TD className="font-mono text-xs">{m.documento}</TD>
                      <TD className="text-right tabular-nums">
                        {formatMonto(Number(m.debito) > 0 ? m.debito : m.credito)}
                      </TD>
                      <TD className="text-xs">
                        {m.banco} · {m.cuenta}
                      </TD>
                      <TD className="max-w-[18rem] text-xs">{m.motivo}</TD>
                      <TD className="text-xs text-content-muted">{m.excluido_por}</TD>
                      <TD className="text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={incluir.isPending}
                          onClick={() => incluir.mutate(m.id)}
                        >
                          Devolver
                        </Button>
                      </TD>
                    </TR>
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

/**
 * Un par, con sus dos lados lado a lado y las señales a la vista.
 *
 * Los dos lados se muestran COMPLETOS —consecutivo y descripción de cada uno— porque la decisión
 * de cuál sacar depende de eso: el que se queda debería ser el que el banco sigue usando.
 */
function ParSospechoso(props: {
  p: ParDuplicado;
  onExcluir: (id: string, motivo: string) => void;
  trabajando: boolean;
}) {
  const { p } = props;
  const [motivo, setMotivo] = useState(
    `duplicado: el banco cargó el mismo movimiento con dos consecutivos (${p.a.documento} y ${p.b.documento})`,
  );
  const tono = p.senales === 3 ? "border-pendiente" : "border-border";

  return (
    <div className={`rounded-lg border-2 p-4 ${tono}`}>
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <div className="text-sm">
          <span className="font-medium">{formatFecha(p.fecha)}</span>
          <span className="ml-2 text-content-muted">
            {p.banco} · {p.cuenta}
          </span>
        </div>
        <div className="text-sm font-medium tabular-nums">
          {formatMonto(p.monto)} {p.es_debito ? "débito" : "crédito"}
        </div>
      </div>

      <div className="mt-2 flex flex-wrap gap-2 text-xs">
        <Senal activa={p.mismos_ultimos_4} texto="mismos 4 dígitos finales" />
        <Senal activa={p.largo_distinto} texto="el consecutivo cambió de largo" />
        <Senal activa={p.descripcion_distinta} texto="descripción distinta" />
        {p.senales < 3 && (
          <span className="text-content-muted">
            — con menos de tres señales, puede ser un movimiento legítimo repetido
          </span>
        )}
      </div>

      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <LadoMov lado={p.a} onExcluir={() => props.onExcluir(p.a.id, motivo)} trabajando={props.trabajando} />
        <LadoMov lado={p.b} onExcluir={() => props.onExcluir(p.b.id, motivo)} trabajando={props.trabajando} />
      </div>

      <label className="mt-3 block text-xs">
        <span className="mb-1 block text-content-muted">Motivo (queda en auditoría)</span>
        <Input value={motivo} onChange={(e) => setMotivo(e.target.value)} />
      </label>
    </div>
  );
}

function LadoMov(props: {
  lado: { id: string; documento: string; descripcion: string };
  onExcluir: () => void;
  trabajando: boolean;
}) {
  return (
    <div className="rounded-md border p-3">
      <div className="font-mono text-xs">{props.lado.documento}</div>
      <div className="mt-1 text-xs text-content-muted">{props.lado.descripcion}</div>
      <Button
        variant="ghost"
        size="sm"
        className="mt-2"
        disabled={props.trabajando}
        onClick={props.onExcluir}
      >
        Sacar este de los libros
      </Button>
    </div>
  );
}

function Senal({ activa, texto }: { activa: boolean; texto: string }) {
  if (!activa) return null;
  return <Badge tone="pendiente">{texto}</Badge>;
}
