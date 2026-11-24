/**
 * «Consultas lentas»: qué se lleva el tiempo del servidor (mig 0120).
 *
 * POR QUÉ EXISTE. Cuando alguien dice «el sistema está lento», hasta hoy no había forma de saber
 * QUÉ está lento. El diagnóstico de noviembre de 2026 encontró que UNA consulta del tablero de CxP
 * se llevaba el 92 % del tiempo de esa pantalla, pero para verlo hubo que entrar a la base por SSH
 * y encender el contador a mano. En el servidor del cliente eso no estaba disponible: el único
 * camino era adivinar.
 *
 * ORDENADA POR TIEMPO TOTAL, no por la más lenta. Una consulta de 900 ms que corre una vez al día
 * importa menos que una de 20 ms que corre cuarenta veces por pantalla: lo que se siente es la
 * suma. Fue exactamente la forma de aquel problema —un cálculo repetido siete veces por carga— y
 * ordenar por el promedio lo habría escondido.
 *
 * NO MUESTRA DATOS DE NADIE. PostgreSQL normaliza las consultas antes de contarlas: los valores
 * vienen como $1, $2. Se ve la forma de la consulta, nunca el monto ni la cédula que llevaba.
 */
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
  LoadingState,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  PageHeader,
  useToast,
} from "@/components/ui";
import { sistemaApi, type ConsultaLenta } from "@/api/sistema";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";

/** Milisegundos legibles: 45 ms, 1,2 s, 3,5 min. Un total en ms crudo no se lee. */
function duracion(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
  return `${(ms / 60_000).toFixed(1)} min`;
}

function miles(n: number): string {
  return new Intl.NumberFormat("es-CR").format(Math.round(n));
}

function Fila({ c, puesto }: { c: ConsultaLenta; puesto: number }) {
  return (
    <TR>
      <TD className="text-content-muted">{puesto}</TD>
      <TD>
        {/* La consulta en monoespaciada y con saltos: es SQL, y leerlo en proporcional cuesta. */}
        <code className="block max-w-[46rem] whitespace-pre-wrap break-words text-xs text-content-muted">
          {c.consulta}
        </code>
      </TD>
      <TD className="text-right tabular-nums font-medium">{duracion(c.ms_total)}</TD>
      <TD className="text-right tabular-nums">
        {/* El porcentaje es lo que distingue «la más lenta de una base tranquila» de «la que hay
            que arreglar hoy». Se resalta a partir de 20 %. */}
        {c.porcentaje_del_total >= 20 ? (
          <Badge tone="pendiente">{c.porcentaje_del_total.toFixed(1)} %</Badge>
        ) : (
          <span className="text-content-muted">{c.porcentaje_del_total.toFixed(1)} %</span>
        )}
      </TD>
      <TD className="text-right tabular-nums">{miles(c.llamadas)}</TD>
      <TD className="text-right tabular-nums">{duracion(c.ms_media)}</TD>
      <TD className="text-right tabular-nums">{duracion(c.ms_max)}</TD>
      <TD className="text-right tabular-nums text-content-muted">{miles(c.filas_por_llamada)}</TD>
    </TR>
  );
}

export function ConsultasLentasPage() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();

  const q = useQuery({
    queryKey: ["sistema", empresaId, "consultas-lentas"],
    queryFn: () => sistemaApi.consultasLentas(20),
    // Sin refresco automático: esto se mira cuando alguien reporta lentitud, no en continuo, y
    // una pantalla que se recarga sola mientras se lee una consulta larga es inusable.
    staleTime: 30_000,
  });

  const reiniciar = useMutation({
    mutationFn: () => sistemaApi.reiniciarMedicion(),
    onSuccess: () => {
      toast.success("Medición reiniciada: los tiempos vuelven a contar desde ahora");
      void qc.invalidateQueries({ queryKey: ["sistema", empresaId, "consultas-lentas"] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const datos = q.data;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Consultas lentas"
        description="Qué se lleva el tiempo del servidor. Se mira cuando alguien reporta que el sistema va lento."
      />

      <Card>
        <CardHeader>
          <CardTitle>Las que más tiempo acumulan</CardTitle>
          <CardDescription>
            Ordenadas por <strong>tiempo total</strong>, no por la más lenta: una consulta de 20 ms
            que corre cuarenta veces por pantalla pesa más que una de 900 ms que corre una vez al
            día. Acá se ve la forma de cada consulta, nunca los datos que llevaba.
            {datos?.midiendo_desde && (
              <> Midiendo desde el <strong>{datos.midiendo_desde}</strong>.</>
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {q.isPending && <LoadingState label="Leyendo el contador del servidor" />}
          {q.isError && <ErrorState message={mensajeError(q.error)} />}

          {/* El contador apagado NO es un error: es una herramienta opcional que este servidor no
              encendió. Mostrarlo como fallo mandaría a buscar algo roto que no existe. */}
          {datos && !datos.disponible && (
            <div className="rounded-md border border-border bg-surface-muted p-4">
              <p className="text-sm font-medium">El contador no está activo en este servidor</p>
              <p className="mt-1 text-sm text-content-muted">{datos.motivo}</p>
            </div>
          )}

          {datos?.disponible && datos.consultas.length === 0 && (
            <p className="py-6 text-center text-sm text-content-muted">
              Todavía no hay consultas medidas. Usá el sistema un rato y volvé.
            </p>
          )}

          {datos?.disponible && datos.consultas.length > 0 && (
            <>
              <TableContainer>
                <Table>
                  <THead>
                    <TR>
                      <TH>#</TH>
                      <TH>Consulta</TH>
                      <TH className="text-right">Tiempo total</TH>
                      <TH className="text-right">Del total</TH>
                      <TH className="text-right">Veces</TH>
                      <TH className="text-right">Promedio</TH>
                      <TH className="text-right">La peor</TH>
                      <TH className="text-right">Filas por vez</TH>
                    </TR>
                  </THead>
                  <TBody>
                    {datos.consultas.map((c, i) => (
                      <Fila key={`${i}-${c.consulta.slice(0, 40)}`} c={c} puesto={i + 1} />
                    ))}
                  </TBody>
                </Table>
              </TableContainer>

              <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-4">
                <p className="text-xs text-content-muted">
                  «Filas por vez» delata los escaneos completos: una consulta que devuelve una fila
                  tras leer doscientas mil no se nota en los tiempos hasta que la tabla crece.
                </p>
                <Button
                  variant="ghost"
                  onClick={() => reiniciar.mutate()}
                  disabled={reiniciar.isPending}
                >
                  Reiniciar medición
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
