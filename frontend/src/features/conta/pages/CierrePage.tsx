/**
 * Contabilidad → Balance y cierre (/conta/cierre). FASE 4.
 *
 * Tres cosas que van juntas porque se miran juntas:
 *
 *  · BALANCE DE COMPROBACIÓN — la prueba de que el libro está sano. Es lo que se revisa ANTES de
 *    emitir cualquier estado financiero, y lo que decide si un mes se puede cerrar.
 *  · PERÍODOS — qué meses están cerrados. Un mes cerrado no recibe asientos: ni nuevos, ni
 *    modificados, ni anulados.
 *  · SALDOS INICIALES — con qué arranca la contabilidad. Sin ellos el balance muestra solo lo
 *    que se registró en el sistema, y el activo no es el activo.
 */

import { useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  EmptyState,
  ErrorState,
  LoadingState,
  PageHeader,
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
import { etiquetaPeriodo, finDeMes, formatMoneda, formatMonto } from "@/lib/format";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { useTienePermiso } from "@/features/auth/permisos";
import { PeriodoSelector } from "@/features/bancos/components/PeriodoSelector";
import {
  useBalance,
  useCargarApertura,
  useCerrarPeriodo,
  usePeriodosContables,
  usePrevisualizarApertura,
  useReabrirPeriodo,
} from "@/features/conta/hooks";
import type { PrevisualizacionApertura } from "@/api/conta";

const NOMBRE_MES = [
  "Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
  "Julio", "Agosto", "Setiembre", "Octubre", "Noviembre", "Diciembre",
];

export function CierrePage() {
  const toast = useToast();
  const { periodo: periodoActivo } = usePeriodoActivo();
  const [periodo, setPeriodo] = useState(periodoActivo);
  const puedeCerrar = useTienePermiso()("conta.periodos");

  const desde = `${periodo}-01`;
  const hasta = finDeMes(periodo);

  const balanceQ = useBalance(desde, hasta);
  const periodosQ = usePeriodosContables();
  const cerrar = useCerrarPeriodo();
  const reabrir = useReabrirPeriodo();

  const b = balanceQ.data;
  const periodos = periodosQ.data?.items ?? [];
  const [anio, mes] = periodo.split("-").map(Number);
  const actual = periodos.find((p) => p.anio === anio && p.mes === mes);

  // Las dos acciones preguntan antes, con el diálogo del sistema —el mismo que el resto del ERP—
  // y no con el del navegador. Cerrar se deshace reabriendo, pero la reapertura queda registrada
  // para siempre: un clic por error no se borra, se explica. Y el `window.prompt` del reabrir
  // recortaba el motivo a una línea sin formato, en la decisión que más se audita de la pantalla.
  const [porCerrar, setPorCerrar] = useState<string | null>(null);
  const [porReabrir, setPorReabrir] = useState<string | null>(null);

  function cerrarPeriodo(p: string) {
    cerrar.mutate(p, {
      onSuccess: () => {
        toast.success(`${etiquetaPeriodo(p)} quedó cerrado.`);
        setPorCerrar(null);
      },
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  function reabrirPeriodo(p: string, motivo: string) {
    reabrir.mutate(
      { periodo: p, motivo },
      {
        onSuccess: () => {
          toast.success(`${etiquetaPeriodo(p)} quedó abierto.`);
          setPorReabrir(null);
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Balance y cierre"
        description="La prueba de que el libro cuadra, y el candado que congela un mes ya emitido."
        actions={<PeriodoSelector label="Período" value={periodo} onChange={setPeriodo} id="cierre-periodo" />}
      />

      <AperturaPanel
        periodo={periodo}
        puedeCargar={puedeCerrar}
        yaCargada={b ? !b.sin_apertura : undefined}
      />

      {/* ── Balance de comprobación ───────────────────────────────────────────────────── */}
      {balanceQ.isPending ? (
        <LoadingState label="Armando el balance" />
      ) : balanceQ.isError ? (
        <ErrorState message={mensajeError(balanceQ.error)} onRetry={() => balanceQ.refetch()} />
      ) : !b ? null : (
        <>
          {/* El cuadre es EL dato del reporte, y va arriba: si no cuadra, nada de lo de abajo
              sirve y leerlo primero ahorra el tiempo de mirarlo. */}
          <Card>
            <CardContent className="flex flex-wrap items-center gap-6 pt-5">
              {b.cuadra ? (
                <div className="flex items-center gap-3">
                  <span className="text-2xl text-positivo">✓</span>
                  <div>
                    <p className="font-medium text-content">El balance cuadra</p>
                    {/* «con movimiento» era falso: un mes sin asientos igual lista las cuentas
                        que arrastran saldo de antes. Y el número de asientos es lo que decide si
                        el mes se puede cerrar, así que va acá y no escondido en la tabla de
                        abajo. */}
                    <p className="text-xs text-content-muted">
                      {etiquetaPeriodo(periodo)} · {actual?.asientos ?? 0} asiento(s) ·{" "}
                      {b.renglones.length} cuenta(s) con saldo
                    </p>
                  </div>
                </div>
              ) : (
                <div className="flex items-center gap-3">
                  <span className="text-2xl text-negativo">✗</span>
                  <div>
                    <p className="font-medium text-negativo">
                      No cuadra por {formatMoneda(b.diferencia ?? "0")}
                    </p>
                    <p className="text-xs text-content-muted">
                      Mientras no cuadre, ningún estado financiero que salga de acá sirve.
                    </p>
                  </div>
                </div>
              )}

              {puedeCerrar && (
                <div className="ml-auto flex gap-2">
                  {actual?.estado === "CERRADO" ? (
                    <>
                      <Badge tone="neutral">Período cerrado</Badge>
                      <Button variant="secondary" onClick={() => setPorReabrir(periodo)}>
                        Reabrir
                      </Button>
                    </>
                  ) : (
                    /* Un botón apagado sin motivo se lee como una falla del sistema: el mes más
                       común de encontrar apagado es el que todavía no tiene asientos, y eso no
                       se deduce mirando el botón. */
                    <div className="flex items-center gap-3">
                      {(actual?.asientos ?? 0) === 0 && (
                        <span className="text-xs text-content-muted">
                          Nada que cerrar: este mes no tiene asientos.
                        </span>
                      )}
                      <Button
                        onClick={() => setPorCerrar(periodo)}
                        disabled={!b.cuadra || (actual?.asientos ?? 0) === 0 || cerrar.isPending}
                      >
                        Cerrar {etiquetaPeriodo(periodo)}
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </CardContent>
          </Card>

          {b.aviso && (
            <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
              <p className="text-sm text-content-muted">{b.aviso}</p>
            </div>
          )}

          {b.renglones.length === 0 ? (
            <EmptyState message="No hay movimiento contable hasta esa fecha." />
          ) : (
            <Card>
              <CardHeader>
                <CardTitle>
                  Balance de comprobación · al {hasta}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Cuenta</TH>
                        <TH className="text-right">Saldo anterior</TH>
                        <TH className="text-right" />
                        <TH className="text-right">Débitos</TH>
                        <TH className="text-right">Créditos</TH>
                        <TH className="text-right">Saldo final</TH>
                        <TH className="text-right" />
                      </TR>
                    </THead>
                    <TBody>
                      {b.renglones.map((g) => (
                        <TR key={g.cuenta_id}>
                          <TD className="max-w-sm whitespace-normal">
                            <span className="tabular-nums text-content">{g.codigo}</span>
                            <span className="block text-xs text-content-muted">{g.nombre}</span>
                          </TD>
                          <Monto v={g.saldo_anterior_debe} />
                          <Monto v={g.saldo_anterior_haber} tenue />
                          <Monto v={g.debe} />
                          <Monto v={g.haber} />
                          <Monto v={g.saldo_final_debe} fuerte />
                          <Monto v={g.saldo_final_haber} fuerte />
                        </TR>
                      ))}
                      <TR>
                        <TD className="font-medium text-content">Totales</TD>
                        <Monto v={b.total_anterior_debe} fuerte />
                        <Monto v={b.total_anterior_haber} fuerte />
                        <Monto v={b.total_debe} fuerte />
                        <Monto v={b.total_haber} fuerte />
                        <Monto v={b.total_final_debe} fuerte />
                        <Monto v={b.total_final_haber} fuerte />
                      </TR>
                    </TBody>
                  </Table>
                </TableContainer>
              </CardContent>
            </Card>
          )}
        </>
      )}

      {/* ── Los períodos ──────────────────────────────────────────────────────────────── */}
      {periodos.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Meses del libro</CardTitle>
          </CardHeader>
          <CardContent>
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Mes</TH>
                    <TH className="text-right">Asientos</TH>
                    <TH className="text-right">Monto</TH>
                    <TH>Estado</TH>
                    <TH />
                  </TR>
                </THead>
                <TBody>
                  {periodos.map((p) => (
                    <TR key={`${p.anio}-${p.mes}`}>
                      <TD className="whitespace-nowrap">
                        {NOMBRE_MES[p.mes - 1]} {p.anio}
                      </TD>
                      <TD className="text-right tabular-nums">
                        {p.asientos.toLocaleString("es-CR")}
                      </TD>
                      <TD className="text-right tabular-nums">{formatMonto(p.debe)}</TD>
                      <TD className="max-w-md whitespace-normal">
                        {p.estado === "CERRADO" ? (
                          <>
                            <Badge tone="neutral">Cerrado</Badge>
                            {p.cerrado_por_nombre && (
                              <span className="ml-2 text-xs text-content-muted">
                                por {p.cerrado_por_nombre}
                              </span>
                            )}
                          </>
                        ) : (
                          <>
                            <Badge tone={p.cuadra ? "positivo" : "negativo"}>
                              {p.cuadra ? "Abierto" : "Abierto · no cuadra"}
                            </Badge>
                            {/* La reapertura queda a la vista con su motivo: es la primera
                                pregunta que hace un auditor al ver un mes reabierto. */}
                            {p.reabierto_motivo && (
                              <span className="mt-1 block text-xs text-pendiente">
                                Se reabrió: {p.reabierto_motivo}
                              </span>
                            )}
                          </>
                        )}
                      </TD>
                      <TD className="text-right">
                        {puedeCerrar && p.estado === "CERRADO" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setPorReabrir(`${p.anio}-${String(p.mes).padStart(2, "0")}`)}
                          >
                            Reabrir
                          </Button>
                        )}
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          </CardContent>
        </Card>
      )}

      {porCerrar && (
        <ConfirmDialog
          titulo={`Cerrar ${etiquetaPeriodo(porCerrar)}`}
          descripcion="El mes queda congelado con los números que tiene hoy."
          impacto={[
            `Se cierra con ${actual?.asientos ?? 0} asiento(s) por ${formatMoneda(actual?.debe ?? "0")}.`,
            "Deja de recibir asientos: ni nuevos, ni modificados, ni anulados.",
            "Para volver a tocarlo hay que reabrirlo, y la reapertura queda registrada.",
          ]}
          textoConfirmar="Cerrar el período"
          pendiente={cerrar.isPending}
          onConfirmar={() => cerrarPeriodo(porCerrar)}
          onCancelar={() => setPorCerrar(null)}
        />
      )}

      {porReabrir && (
        <ConfirmDialog
          titulo={`Reabrir ${etiquetaPeriodo(porReabrir)}`}
          descripcion="Mientras esté abierto vuelven a entrar asientos con fecha de ese mes, así que los estados financieros ya emitidos pueden cambiar."
          impacto={["El motivo queda guardado en el expediente del período."]}
          textoConfirmar="Reabrir"
          tono="peligro"
          pedirNota
          notaPlaceholder="¿Por qué se reabre? (mínimo 15 caracteres)"
          pendiente={reabrir.isPending}
          onConfirmar={(motivo) => reabrirPeriodo(porReabrir, motivo)}
          onCancelar={() => setPorReabrir(null)}
        />
      )}
    </div>
  );
}

/** Una celda de monto. El cero se muestra vacío: en un balance, las columnas llenas de ceros
 *  esconden las cifras que sí importan. */
function Monto({ v, fuerte, tenue }: { v: string; fuerte?: boolean; tenue?: boolean }) {
  const cero = !v || v === "0.00";
  return (
    <TD
      className={cn(
        "text-right tabular-nums",
        fuerte && "font-medium",
        tenue && "text-content-muted",
      )}
    >
      {cero ? "" : formatMonto(v)}
    </TD>
  );
}

/** Carga de saldos iniciales desde el balance del sistema anterior. */
function AperturaPanel({
  periodo,
  puedeCargar,
  yaCargada,
}: {
  periodo: string;
  puedeCargar: boolean;
  /** undefined mientras el balance no responde: ver abajo por qué no se dibuja nada todavía. */
  yaCargada: boolean | undefined;
}) {
  const toast = useToast();
  const [archivo, setArchivo] = useState<File | null>(null);
  const [prev, setPrev] = useState<PrevisualizacionApertura | null>(null);
  const previsualizar = usePrevisualizarApertura();
  const cargar = useCargarApertura();

  if (!puedeCargar) return null;
  // Sin saber si ya hay apertura, el panel solo puede decir algo que quizá sea falso. Esperar
  // medio segundo es preferible a invitar a cargar un archivo que va a ser rechazado.
  if (yaCargada === undefined) return null;

  // YA CARGADA: el backend lo sabía, pero solo lo decía DESPUÉS de subir el archivo. Alguien
  // tenía que ir a buscar el balance del sistema anterior, esperar la lectura y recién ahí
  // enterarse de que no hacía falta.
  if (yaCargada) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Saldos iniciales</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-content-muted">
            Ya están cargados: el libro arranca de ellos. Para reemplazarlos hay que anular el
            asiento de apertura desde <b className="text-content">Libros</b>; si no, quedarían dos
            puntos de partida y el balance contaría dos veces el mismo saldo.
          </p>
        </CardContent>
      </Card>
    );
  }

  function elegir(f: File) {
    setArchivo(f);
    setPrev(null);
    previsualizar.mutate(
      { periodo, archivo: f },
      { onSuccess: setPrev, onError: (e) => toast.error(mensajeError(e)) },
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Saldos iniciales</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <p className="text-sm text-content-muted">
          El balance de comprobación del sistema anterior, en Excel. Hace falta una columna con el{" "}
          <b className="text-content">código de cuenta</b> y los montos: <b className="text-content">
          Debe y Haber</b>, o una sola columna de <b className="text-content">Saldo</b> donde el
          signo decide el lado. Se carga una sola vez, con fecha del último día de{" "}
          {etiquetaPeriodo(periodo)}.
        </p>
        <input
          type="file"
          accept=".xlsx,.xls"
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) elegir(f);
          }}
          className="text-sm text-content file:mr-3 file:rounded-lg file:border-0 file:bg-accent file:px-3 file:py-1.5 file:text-sm file:text-white"
        />
        {previsualizar.isPending && <p className="text-sm text-content-muted">Leyendo el archivo…</p>}

        {prev && (
          <div className="flex flex-col gap-3 rounded-lg border border-border bg-surface-muted/40 p-4">
            <div className="flex flex-wrap items-end gap-6">
              <div>
                <p className="text-xl font-semibold tabular-nums text-content">{prev.cuentas}</p>
                <p className="text-xs text-content-muted">cuenta(s) con saldo</p>
              </div>
              <div>
                <p className="text-lg tabular-nums text-content">{formatMonto(prev.total_debe)}</p>
                <p className="text-xs text-content-muted">total debe</p>
              </div>
              <div>
                <p className="text-lg tabular-nums text-content">{formatMonto(prev.total_haber)}</p>
                <p className="text-xs text-content-muted">total haber</p>
              </div>
              <div className="ml-auto">
                {prev.cuadra ? (
                  <span className="text-sm text-positivo">Cuadra</span>
                ) : (
                  <span className="text-sm text-negativo">
                    No cuadra por {formatMoneda((prev.diferencia ?? "0").replace("-", ""))}
                  </span>
                )}
              </div>
            </div>

            {prev.aviso && <p className="text-sm text-pendiente">{prev.aviso}</p>}

            {prev.con_problema > 0 && (
              <TableContainer>
                <Table>
                  <THead>
                    <TR>
                      <TH className="text-right">Fila</TH>
                      <TH>Código</TH>
                      <TH>Qué pasa</TH>
                    </TR>
                  </THead>
                  <TBody>
                    {prev.filas
                      .filter((f) => f.problema)
                      .slice(0, 20)
                      .map((f) => (
                        <TR key={f.fila}>
                          <TD className="text-right tabular-nums">{f.fila}</TD>
                          <TD className="tabular-nums">{f.codigo}</TD>
                          <TD className="max-w-md whitespace-normal text-content-muted">
                            {f.problema}
                          </TD>
                        </TR>
                      ))}
                  </TBody>
                </Table>
              </TableContainer>
            )}

            <div>
              <Button
                disabled={!prev.cuadra || prev.ya_existe || prev.cuentas === 0 || cargar.isPending}
                onClick={() =>
                  archivo &&
                  cargar.mutate(
                    { periodo, archivo },
                    {
                      onSuccess: (a) => {
                        toast.success(`Saldos iniciales cargados (asiento ${a.numero}).`);
                        setArchivo(null);
                        setPrev(null);
                      },
                      onError: (e) => toast.error(mensajeError(e)),
                    },
                  )
                }
              >
                Cargar los saldos iniciales
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
