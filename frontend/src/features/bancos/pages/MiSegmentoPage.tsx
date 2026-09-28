/**
 * Bancos — Mi partida (/mi-segmento).
 *
 * La pantalla del equipo de una partida: Asociaciones, Depósitos, Emergencias. Contesta una sola
 * pregunta —«¿entró el dinero?»— y no abre nada más del módulo.
 *
 * Lo que el equipo puede hacer acá es mirar y AVISAR. Corregir la partida sigue siendo de quien
 * clasifica: el aviso cae en la pestaña «Reportados» de Clasificar. Esa separación es a propósito
 * (la segmentación tiene que quedar bien en Bancos, y no la arregla quien la señala).
 *
 * El recorte por alcance lo aplica el servidor en su propio endpoint. Esta pantalla no filtra por
 * partida ni podría: no recibe las que no le tocan.
 *
 * Dos apartados:
 *  · «Mi partida» — los créditos de sus partidas, y NADA más.
 *  · «Mis avisos» — lo que avisó esta persona y la respuesta, aunque la corrección haya sacado el
 *    movimiento de su partida (que es lo más común: justo cuando hay respuesta, la fila desaparece).
 *
 * Hubo un tercero, «Todavía sin partida», con los créditos que nadie había clasificado en las
 * cuentas del segmento. El Director Financiero lo mandó quitar el 23-set-2026: «esto no debe ser
 * visible por ningún motivo a los consultores». No está escondido —el servidor responde 400 a esa
 * vista y no deja avisar sobre esos créditos—: quien espera un depósito y no lo ve usa «Falta un
 * movimiento», que avisa sin mostrar nada de lo que no le toca.
 *
 * Por la misma decisión se quitó el desplegable «hasta cuándo está cargada cada una de tus cuentas»:
 * queda la línea de arriba de la tabla, que es la que decide si conviene esperar antes de avisar.
 */

import { useCallback, useEffect, useState, type ReactNode } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Paginador,
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
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { useAuth } from "@/features/auth/AuthContext";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { cn } from "@/lib/cn";
import {
  etiquetaPeriodo,
  formatFecha,
  formatFechaHora,
  formatMoneda,
  formatMonto,
} from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import {
  useBuscarFaltante,
  useMiSegmento,
  useMisAvisos,
  useReportarFaltante,
  useReportarSegmentacion,
} from "@/features/bancos/hooks";
import { revisarRango } from "@/features/bancos/rangoFechas";
import {
  avisoFechaSinCargar,
  cargaDeLaBusqueda,
  consejoNoExiste,
  mensajeVacio,
  type CargaDelSegmento,
} from "@/features/bancos/miSegmentoTextos";
import {
  almacenamientoDelNavegador,
  claveVistos,
  escribirVistos,
  estadoDeAviso,
  idsResueltos,
  leerVistos,
  respuestasNuevas,
  tituloAvisoResuelto,
  unirVistos,
} from "@/features/bancos/misAvisos";
import type { AvisoResuelto, MiAviso, MovimientoRow, ResultadoFaltante } from "@/api/bancos";

/** Página de «Mis avisos» que también alimenta el número de la pestaña (misma consulta). */
const AVISOS_POR_PAGINA = 50;

const nf = new Intl.NumberFormat("es-CR");

const SIN_CARGA: CargaDelSegmento = {
  cargado_hasta: "",
  cargado_hasta_cuenta: null,
  carga_por_cuenta: [],
};

export function MiSegmentoPage() {
  const { periodo } = usePeriodoActivo();
  const { user } = useAuth();
  const empresaId = useEmpresaId();
  // «Mis avisos» es un apartado aparte y NO toca la lista: al volver, sigue donde estaba.
  const [verAvisos, setVerAvisos] = useState(false);
  const [q, setQ] = useState("");
  const [buscado, setBuscado] = useState("");
  // Filtros propios de la pantalla, los mismos que la hoja de trabajo: cuenta, rango de fechas y
  // orden. NO hay filtro de partida: el alcance ya la fija y ofrecer un selector con una sola
  // opción es ruido.
  const [cuentaId, setCuentaId] = useState("");
  const [desde, setDesde] = useState("");
  const [hasta, setHasta] = useState("");
  const [orden, setOrden] = useState("");
  // Paginado: antes la pantalla pedía 200 filas fijas y no lo decía. En setiembre de 2026 hay 2.108
  // créditos en el alcance de un rol de consulta, o sea que se veía el 9,5% y el pie afirmaba
  // «Mostrando Septiembre 2026». Quien concilia con esa lista da por FALTANTE plata que sí entró.
  // El backend ya paginaba y ya devolvía el total: el recorte estaba solo acá.
  const [pagina, setPagina] = useState(1);
  const [porPagina, setPorPagina] = useState(200);

  // Las fechas se revisan ANTES de salir. El selector de fecha del navegador dispara onChange en
  // cada tecla del año y rellena con ceros, así que tecleando «2026» esta pantalla llegaba a pedir
  // «0002-09-01»: 500 «error interno» en un caso y el histórico completo en silencio en el otro.
  // Con el rango mal escrito NO se consulta: se muestra el mes activo y el error va pegado al campo.
  const rango = revisarRango(desde, hasta);

  // El período global manda salvo que se escriba un rango de fechas VÁLIDO: con «desde/hasta»
  // puestos, el mes activo dejaría afuera lo que se está buscando (y buscar un depósito viejo es
  // justo para lo que sirve el rango).
  const usaRango = Boolean(rango.desde || rango.hasta);
  const filtros = {
    periodo: usaRango ? undefined : periodo,
    desde: rango.desde,
    hasta: rango.hasta,
    cuenta_bancaria_id: cuentaId || undefined,
    orden: orden || undefined,
    q: buscado || undefined,
  };
  const query = useMiSegmento({ ...filtros, page: pagina, page_size: porPagina });
  // La primera página de «Mis avisos» (la misma consulta que abre el apartado): de acá salen el
  // número de la pestaña y cuántas respuestas son nuevas.
  const avisosResumen = useMisAvisos(1, AVISOS_POR_PAGINA);
  const [reportando, setReportando] = useState<MovimientoRow | null>(null);
  const [buscandoFaltante, setBuscandoFaltante] = useState(false);

  // «Respuesta nueva»: lo que ya se mostró en «Mis avisos» en este navegador (ver misAvisos.ts).
  // La memoria va atada a SU clave (empresa + usuario): si cambia la empresa activa sin desmontar la
  // pantalla, se relee la de la nueva en vez de escribirle encima la de la anterior.
  const [almacen] = useState(almacenamientoDelNavegador);
  const clave = user ? claveVistos(empresaId, user.id) : null;
  const [memoria, setMemoria] = useState(() => ({ clave, vistos: leerVistos(almacen, clave) }));
  if (memoria.clave !== clave) setMemoria({ clave, vistos: leerVistos(almacen, clave) });
  const vistos = memoria.clave === clave ? memoria.vistos : null;
  useEffect(() => {
    escribirVistos(almacen, memoria.clave, memoria.vistos);
  }, [almacen, memoria]);
  // Estable a propósito: el apartado la llama desde un efecto cada vez que llega una página.
  const marcarVistos = useCallback((ids: string[]) => {
    setMemoria((m) => {
      const unidos = unirVistos(m.vistos, ids);
      return unidos === m.vistos ? m : { clave: m.clave, vistos: unidos };
    });
  }, []);

  // Al cambiar cualquier filtro se vuelve a la primera página. Quedarse en la página 8 de un filtro
  // que ahora tiene dos páginas muestra una tabla vacía, que acá se lee como «no entró nada tuyo».
  useEffect(() => {
    setPagina(1);
  }, [buscado, cuentaId, rango.desde, rango.hasta, orden, periodo, porPagina]);

  const mesActivo = etiquetaPeriodo(periodo);

  const cabecera = query.data;
  if (query.isPending && !cabecera) return <LoadingState label="Cargando tu partida" />;

  const data = query.data;
  const items = data?.movimientos.items ?? [];
  const hayFiltro = Boolean(buscado || cuentaId || desde || hasta);
  // Página fuera de rango: no llegaron filas pero SÍ hay movimientos. Lo explica el paginador
  // («la página N quedó vacía: hay 2.108 movimientos»); un «no entró nada tuyo» acá lo desmentiría.
  const paginaVacia = Boolean(data && data.movimientos.total > 0 && items.length === 0);
  const carga: CargaDelSegmento = cabecera
    ? {
        cargado_hasta: cabecera.cargado_hasta,
        cargado_hasta_cuenta: cabecera.cargado_hasta_cuenta ?? null,
        carga_por_cuenta: cabecera.carga_por_cuenta ?? [],
      }
    : SIN_CARGA;

  const nuevas = respuestasNuevas(avisosResumen.data?.items ?? [], vistos).size;

  function limpiarFiltros() {
    setQ("");
    setBuscado("");
    setCuentaId("");
    setDesde("");
    setHasta("");
    setOrden("");
  }

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Mi partida en bancos"
        description="Lo que entró al banco en las partidas que consultás. Solo ingresos, y solo los tuyos."
        actions={
          <div className="flex items-center gap-2">
            {(cabecera?.partidas ?? []).map((p) => (
              <Badge key={p.clasificacion_id} tone="accent">
                {p.clasificacion}
              </Badge>
            ))}
            {/* Fuera de la tabla a propósito: es el aviso de algo que NO tiene fila. */}
            {!cabecera?.sin_alcance && (
              <Button variant="secondary" onClick={() => setBuscandoFaltante(true)}>
                Falta un movimiento
              </Button>
            )}
          </div>
        }
      />

      {/* Un rol sin partidas asignadas NO es un error: es el estado inicial. Se dice qué falta y
          quién lo hace, en vez de mostrar una tabla vacía que parece una falla del sistema. */}
      {cabecera?.sin_alcance ? (
        <Card>
          <CardContent className="flex flex-col gap-2 py-8 text-center">
            <p className="text-base font-medium text-content">
              Todavía no tenés ninguna partida asignada
            </p>
            <p className="mx-auto max-w-prose text-sm text-content-muted">
              {cabecera.aviso ??
                "Pedile a Dirección Financiera que marque tus partidas en el catálogo de Bancos."}
            </p>
          </CardContent>
        </Card>
      ) : (
        <>
          {/* Los dos apartados. El número de «Mi partida» es el total REAL con los filtros de
              abajo. «Mis avisos» avisa cuando hay respuestas que no viste. */}
          <div
            role="tablist"
            aria-label="Qué mirar"
            className="flex flex-wrap gap-1 border-b border-border"
          >
            <Pestana
              activa={!verAvisos}
              onClick={() => setVerAvisos(false)}
              conteo={data?.movimientos.total}
            >
              Mi partida
            </Pestana>
            <Pestana
              activa={verAvisos}
              onClick={() => setVerAvisos(true)}
              conteo={avisosResumen.data?.total}
              destacado={
                nuevas > 0
                  ? `${nf.format(nuevas)} ${nuevas === 1 ? "respuesta nueva" : "respuestas nuevas"}`
                  : undefined
              }
            >
              Mis avisos
            </Pestana>
          </div>

          {verAvisos ? (
            <MisAvisosPanel vistosAlAbrir={vistos} onVistos={marcarVistos} />
          ) : (
            <>
              <form
                className="flex flex-wrap items-end gap-3"
                onSubmit={(e) => {
                  e.preventDefault();
                  setBuscado(q.trim());
                }}
              >
                <Input
                  label="Buscar en el detalle del banco"
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                  placeholder="Ej. SOLIDARIS, COOPENAE, SINPE…"
                  className="min-w-56"
                />
                {/* Las cuentas salen de SU segmento, no del catálogo: este rol no tiene acceso al
                    catálogo de cuentas y el desplegable le respondería 403. */}
                <Select
                  label="Cuenta"
                  value={cuentaId}
                  onChange={(e) => setCuentaId(e.target.value)}
                  options={[
                    { value: "", label: "Todas mis cuentas" },
                    ...(cabecera?.cuentas ?? []).map((c) => ({
                      value: c.id,
                      label: `${c.banco} · ${c.cuenta}`,
                    })),
                  ]}
                  className="min-w-48"
                />
                {/* El error va PEGADO al campo y la consulta no sale: una fecha a medio teclear no
                    puede terminar en un «error interno» que se lleve puesta la pantalla. */}
                <Input
                  label="Desde"
                  type="date"
                  value={desde}
                  onChange={(e) => setDesde(e.target.value)}
                  error={rango.errorDesde || undefined}
                  className="w-36"
                />
                <Input
                  label="Hasta"
                  type="date"
                  value={hasta}
                  onChange={(e) => setHasta(e.target.value)}
                  error={rango.errorHasta || undefined}
                  className="w-36"
                />
                <Select
                  label="Ordenar por"
                  value={orden}
                  onChange={(e) => setOrden(e.target.value)}
                  options={[
                    { value: "", label: "Más recientes" },
                    { value: "fecha_asc", label: "Más antiguos" },
                    { value: "monto_desc", label: "Monto: mayor a menor" },
                    { value: "monto_asc", label: "Monto: menor a mayor" },
                  ]}
                  className="min-w-44"
                />
                <div className="flex items-end gap-2 pb-0.5">
                  <Button type="submit" variant="secondary">
                    Buscar
                  </Button>
                  {hayFiltro && (
                    <Button type="button" variant="ghost" onClick={limpiarFiltros}>
                      Quitar filtros
                    </Button>
                  )}
                </div>
              </form>

              {/* Acá vivía la leyenda del mes y la fecha de carga del banco, y abajo el total de la
                  partida en colones. Las dos se quitaron el 23-set-2026 a pedido del Director
                  Financiero. Lo ÚNICO que sobrevive de ese bloque son los dos avisos de abajo, y
                  no son leyenda: aparecen solo cuando el rango de fechas que el usuario escribió
                  no es el que se está aplicando. El de «todavía no apliqué el rango» existe porque
                  una fecha a medio teclear devolvía otra lista sin decirlo; quitarlo sería
                  reponer ese defecto. El mes activo se sigue viendo en el selector de período de
                  la aplicación, y el conteo, en el paginador de acá abajo. */}
              {(rango.hayError || usaRango) && (
                <p className="text-xs text-content-muted">
                  {rango.hayError ? (
                    <>
                      Mostrando {mesActivo}: todavía no apliqué el rango de fechas, revisá lo que
                      está marcado arriba.
                    </>
                  ) : (
                    <>
                      Mostrando el rango de fechas que pediste, no el mes activo.{" "}
                      <button
                        type="button"
                        className="underline hover:text-content"
                        onClick={() => {
                          setDesde("");
                          setHasta("");
                        }}
                      >
                        Volver a {mesActivo}
                      </button>
                    </>
                  )}
                </p>
              )}

              {/* Paginado ARRIBA de la tabla, como en la bandeja de CxP: el total tiene que verse
                  sin tener que bajar 200 filas. «Mostrando 1–200 de 2.108» es el dato que impide
                  que alguien concilie creyendo que vio todo. */}
              {data && data.movimientos.total > 0 && (
                <div className="flex flex-col gap-1">
                  <Paginador
                    total={data.movimientos.total}
                    pagina={pagina}
                    porPagina={porPagina}
                    enPantalla={items.length}
                    onPagina={setPagina}
                    onPorPagina={setPorPagina}
                    etiqueta={data.movimientos.total === 1 ? "movimiento" : "movimientos"}
                  />
                  {/* El conteo del paginador cuenta los excluidos y la suma de dinero no. Si no se
                      dice, los dos números se contradicen en silencio después de revertir una
                      importación. */}
                  {data.movimientos.totales.excluidos > 0 && (
                    <p className="text-xs text-content-muted">
                      {data.movimientos.totales.excluidos} de esos movimientos vienen de una
                      importación revertida: se muestran, pero no suman.
                    </p>
                  )}
                </div>
              )}

              {query.isError ? (
                // El error ocupa el lugar de la TABLA, no el de la pantalla: con el formulario a la
                // vista se puede corregir o quitar el filtro que falló sin recargar.
                <ErrorState message={mensajeError(query.error)} onRetry={() => query.refetch()} />
              ) : !data ? (
                <LoadingState label="Cargando tu partida" />
              ) : items.length > 0 ? (
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Fecha</TH>
                        {/* Las dos referencias del banco: con esto el equipo cruza contra su recibo.
                            La larga es la del SINPE y solo existe en Davivienda; en las demás
                            cuentas la celda va con un «—» y el encabezado lo explica, para que el
                            blanco no se lea como un dato perdido. */}
                        <TH>Referencia</TH>
                        <TH title="La referencia larga del SINPE. Solo la publica Davivienda; en las otras cuentas no existe.">
                          Consecutivo largo
                        </TH>
                        <TH>Cuenta</TH>
                        <TH>Detalle del banco</TH>
                        <TH className="text-right">Entró</TH>
                        <TH className="text-right">Segmento</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {items.map((m) => (
                        <TR key={m.id} className={m.reporte_abierto ? "bg-pendiente/5" : undefined}>
                          <TD className="tabular-nums whitespace-nowrap">{formatFecha(m.fecha)}</TD>
                          <TD className="font-mono text-xs tabular-nums whitespace-nowrap text-content-muted">
                            {m.documento || "—"}
                          </TD>
                          <TD className="font-mono text-xs tabular-nums whitespace-nowrap text-content-muted">
                            {m.consecutivo_largo || "—"}
                          </TD>
                          <TD className="text-sm text-content-muted whitespace-nowrap">
                            {m.banco} · {m.cuenta}
                          </TD>
                          {/* La descripción se acota y se parte en varias líneas, como en la hoja
                              de trabajo: una línea SINPE de 120 caracteres empuja la columna del
                              segmento fuera de la pantalla, y el segmento es justo lo que hay que
                              mirar. */}
                          <TD className="max-w-xl">
                            <span className="block whitespace-normal break-words text-sm">
                              {m.descripcion || "—"}
                            </span>
                          </TD>
                          {/* Un movimiento excluido (importación duplicada revertida) se muestra
                              pero no suma. Tachado y con la etiqueta, en la fila misma: el aviso
                              general de arriba dice cuántos son, no cuáles, y sin esto el monto se
                              lee como plata que entró. */}
                          <TD
                            className={
                              m.incluido
                                ? "text-right tabular-nums whitespace-nowrap"
                                : "text-right tabular-nums whitespace-nowrap text-content-muted"
                            }
                          >
                            <span className={m.incluido ? undefined : "line-through"}>
                              {formatMonto(m.credito)}
                            </span>
                            {!m.incluido && (
                              <Badge tone="pendiente" className="ml-2">
                                no suma
                              </Badge>
                            )}
                          </TD>
                          {/* La celda MUESTRA la partida contra la que quedó marcado el movimiento, y
                              avisar pasa a ser un botón al lado. Antes solo estaba la acción: con dos
                              partidas en el alcance era imposible ver cuál de las dos le tocó a la
                              fila, que es exactamente lo que hay que mirar para detectar el error. */}
                          <TD>
                            <div className="flex items-center justify-end gap-2">
                              <div className="flex flex-col items-end gap-0.5">
                                <Badge tone={m.clasificacion ? "accent" : "neutral"}>
                                  {m.clasificacion || "Sin partida"}
                                </Badge>
                                {/* El motivo va entre comillas SOLO si es el que escribió esta
                                    persona. Si el aviso es de otra o es un faltante, el servidor
                                    manda un texto genérico —el motivo ajeno no viaja— y no se cita
                                    como si alguien lo hubiera escrito. */}
                                {m.reporte_abierto && (
                                  <span className="text-[11px] text-content-muted">
                                    {m.reporte_abierto_propio
                                      ? `«${m.reporte_abierto}»`
                                      : m.reporte_abierto}
                                  </span>
                                )}
                                {/* Un aviso SUYO ya resuelto: la respuesta y cuándo, AL LADO del
                                    botón. El botón vuelve a salir porque puede seguir mal; lo que
                                    ya no pasa es que la respuesta no se vea en ningún lado. */}
                                {!m.reporte_abierto && m.aviso_resuelto && (
                                  <RespuestaEnFila aviso={m.aviso_resuelto} />
                                )}
                              </div>
                              {m.reporte_abierto ? (
                                // Ya avisado: se muestra que está en revisión en vez de ofrecer
                                // avisar otra vez. Es lo que evita tres avisos idénticos del mismo
                                // movimiento (y el servidor igual responde 409).
                                <Badge tone="pendiente">En revisión</Badge>
                              ) : (
                                <Button
                                  size="sm"
                                  variant="ghost"
                                  className="whitespace-nowrap"
                                  onClick={() => setReportando(m)}
                                >
                                  Está mal segmentado
                                </Button>
                              )}
                            </div>
                          </TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              ) : paginaVacia ? null : ( // lo explica el paginador, no hace falta contradecirlo acá
                <EmptyState
                  message={mensajeVacio({ hayFiltro, mesActivo, desde: rango.desde, carga })}
                />
              )}
            </>
          )}

          {/* El pie dice lo que de verdad pasa con un aviso. Antes prometía «te queda la respuesta
              en esta misma pantalla», y no quedaba en ningún lado; después decía que quedaba «en la
              fila» también cuando el movimiento deja de aparecer acá, que es justo cuando no hay
              fila. La fila muestra solo la respuesta a un aviso PROPIO sobre ese movimiento.

              Acá vivía el desplegable «hasta cuándo está cargada cada una de tus cuentas». Se quitó
              el 23-set-2026: «no es un tema de interés a los consultores». Lo que sí sigue es la
              línea de arriba de la tabla, que es la que contesta «¿conviene esperar antes de
              avisar?». */}
          <div className="text-xs text-content-muted">
            <p>
              Si un movimiento de esta lista no es tuyo, avisalo desde su fila; si esperás uno que no
              ves, buscalo con «Falta un movimiento». Lo corrige quien clasifica en Bancos. La
              respuesta a cada aviso tuyo queda siempre en «Mis avisos»; la de «Está mal segmentado»
              también se ve en la fila del movimiento, mientras siga en esta lista y no haya otro
              aviso abierto sobre él.
            </p>
          </div>
        </>
      )}

      {reportando && (
        <ReportarDialog movimiento={reportando} onCerrar={() => setReportando(null)} />
      )}
      {buscandoFaltante && (
        <FaltanteDialog carga={carga} onCerrar={() => setBuscandoFaltante(false)} />
      )}
    </div>
  );
}

/** Una pestaña de la pantalla, con su número real (si ya llegó) y una marca opcional. */
function Pestana({
  activa,
  onClick,
  conteo,
  destacado,
  children,
}: {
  activa: boolean;
  onClick: () => void;
  /** undefined = todavía no llegó: no se inventa un cero. */
  conteo: number | undefined;
  destacado?: string;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={activa}
      onClick={onClick}
      className={cn(
        "-mb-px flex items-center gap-1.5 border-b-2 px-4 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent",
        activa
          ? "border-accent text-accent"
          : "border-transparent text-content-muted hover:text-content",
      )}
    >
      {children}
      {conteo !== undefined && (
        <span
          className={cn(
            "rounded-full px-1.5 text-[11px] font-semibold tabular-nums",
            activa ? "bg-accent/15 text-accent" : "bg-surface-muted text-content-muted",
          )}
        >
          {nf.format(conteo)}
        </span>
      )}
      {destacado && <Badge tone="accent">{destacado}</Badge>}
    </button>
  );
}

/** La respuesta de un aviso resuelto, dentro de la celda del segmento. */
function RespuestaEnFila({ aviso }: { aviso: AvisoResuelto }) {
  const cuando = formatFechaHora(aviso.resuelto_en);
  const respuesta = aviso.respuesta.trim();
  return (
    <div
      className="max-w-xs whitespace-normal break-words text-right text-[11px] leading-snug"
      title={aviso.motivo ? `Lo que avisaste: «${aviso.motivo}»` : undefined}
    >
      <span className="font-medium text-content">{tituloAvisoResuelto(aviso)}</span>
      {cuando && <span className="tabular-nums text-content-muted"> · {cuando}</span>}
      {respuesta && <span className="block text-content-muted">«{respuesta}»</span>}
    </div>
  );
}

/**
 * «Mis avisos»: lo que avisó ESTA persona en esta empresa, abierto y resuelto.
 *
 * Es el lugar donde la respuesta queda SIEMPRE, incluso cuando la corrección sacó el movimiento de
 * su partida (lo más común). Solo muestra lo que la persona vio al avisar: nada de la partida
 * actual ni del detalle del movimiento, que el servidor tampoco manda.
 */
function MisAvisosPanel({
  vistosAlAbrir,
  onVistos,
}: {
  vistosAlAbrir: Set<string> | null;
  onVistos: (ids: string[]) => void;
}) {
  const [pagina, setPagina] = useState(1);
  const [porPagina, setPorPagina] = useState(AVISOS_POR_PAGINA);
  const query = useMisAvisos(pagina, porPagina);
  // Lo visto ANTES de abrir el apartado. Con eso se resalta lo nuevo durante esta visita, aunque al
  // mostrarse ya quede anotado como visto (y la marca de la pestaña se apague).
  const [antes] = useState(vistosAlAbrir);
  const datos = query.data;

  useEffect(() => {
    const ids = idsResueltos(datos?.items ?? []);
    if (ids.length > 0) onVistos(ids);
  }, [datos, onVistos]);

  if (query.isPending) return <LoadingState label="Cargando tus avisos" />;
  if (query.isError) {
    return <ErrorState message={mensajeError(query.error)} onRetry={() => query.refetch()} />;
  }
  if (query.data.sin_alcance) {
    return (
      <EmptyState message="Tu rol todavía no tiene partidas asignadas, así que no hay avisos que mostrar." />
    );
  }
  const d = query.data;
  if (d.total === 0) {
    return (
      <EmptyState message="Todavía no hiciste ningún aviso. Cuando avises que un movimiento está mal segmentado, que uno sin partida es tuyo o que falta alguno, lo vas a ver acá con la respuesta de quien clasifica." />
    );
  }
  const items = d.items;
  const nuevas = respuestasNuevas(items, antes);

  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-content-muted">
        Los avisos que hiciste vos, de todos los meses. Siguen acá aunque la corrección haya pasado el
        movimiento a otra partida y ya no lo veas en «Mi partida».{" "}
        <span className="tabular-nums text-content">
          {nf.format(d.abiertos)} en revisión · {nf.format(d.resueltos)}{" "}
          {d.resueltos === 1 ? "respondido" : "respondidos"}
        </span>
        {nuevas.size > 0 && (
          <>
            {" · "}
            <b className="text-content">
              {nf.format(nuevas.size)} {nuevas.size === 1 ? "respuesta nueva" : "respuestas nuevas"}
            </b>
          </>
        )}
        .
      </p>
      <Paginador
        total={d.total}
        pagina={pagina}
        porPagina={porPagina}
        enPantalla={items.length}
        onPagina={setPagina}
        onPorPagina={setPorPagina}
        etiqueta={d.total === 1 ? "aviso" : "avisos"}
      />
      {items.length > 0 && (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Avisaste</TH>
                <TH>Movimiento</TH>
                <TH className="text-right">Monto</TH>
                <TH>Tu aviso</TH>
                <TH>Estado</TH>
                <TH>Respuesta</TH>
              </TR>
            </THead>
            <TBody>
              {items.map((a) => (
                <FilaMiAviso key={a.id} aviso={a} nueva={nuevas.has(a.id)} />
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}

function FilaMiAviso({ aviso: a, nueva }: { aviso: MiAviso; nueva: boolean }) {
  const estado = estadoDeAviso(a);
  const moneda = a.moneda === "USD" || a.moneda === "CRC" ? a.moneda : null;
  const respuesta = a.respuesta.trim();
  return (
    <TR className={nueva ? "bg-accent/5" : undefined}>
      <TD className="text-xs tabular-nums text-content-muted">
        {formatFechaHora(a.creado_en) || "—"}
      </TD>
      <TD className="text-sm">
        {a.es_faltante ? (
          // Un faltante muestra lo que la persona ESCRIBIÓ, no el movimiento que el servidor haya
          // enganchado: ese movimiento ella nunca lo vio.
          <div className="flex flex-col items-start gap-0.5">
            <span className="flex items-center gap-1.5">
              <Badge tone="neutral">Faltante</Badge>
              <span className="tabular-nums">esperabas el {formatFecha(a.fecha)}</span>
            </span>
            {a.referencia && (
              <span className="font-mono text-xs text-content-muted">ref. {a.referencia}</span>
            )}
          </div>
        ) : (
          <div className="flex flex-col gap-0.5">
            <span className="tabular-nums">
              {formatFecha(a.fecha)}
              {a.documento && (
                <span className="font-mono text-xs text-content-muted"> · ref. {a.documento}</span>
              )}
            </span>
            <span className="text-xs text-content-muted">
              {a.banco} · {a.cuenta}
            </span>
          </div>
        )}
      </TD>
      <TD className="text-right tabular-nums">
        {a.monto ? (moneda ? formatMoneda(a.monto, moneda) : formatMonto(a.monto)) : "—"}
      </TD>
      <TD className="max-w-xs">
        <span className="block whitespace-normal break-words text-sm">«{a.motivo}»</span>
      </TD>
      <TD>
        <div className="flex flex-col items-start gap-1">
          <div className="flex flex-wrap items-center gap-1">
            <Badge tone={estado.tono}>{estado.etiqueta}</Badge>
            {nueva && <Badge tone="accent">Respuesta nueva</Badge>}
          </div>
          {a.resuelto_en && (
            <span className="text-[11px] tabular-nums text-content-muted">
              {formatFechaHora(a.resuelto_en)}
            </span>
          )}
        </div>
      </TD>
      <TD className="max-w-md">
        <div className="flex flex-col gap-0.5 whitespace-normal break-words">
          {respuesta && <span className="text-sm text-content">«{respuesta}»</span>}
          {estado.explicacion && (
            <span className="text-xs text-content-muted">{estado.explicacion}</span>
          )}
        </div>
      </TD>
    </TR>
  );
}

/**
 * «Falta un movimiento»: buscar primero, avisar después.
 *
 * El orden importa. Avisar a ciegas llena la cola de quien clasifica con plata que todavía no
 * entró al banco; buscar primero separa las razones por las que un movimiento no aparece:
 *
 *   · está y es suyo    → lo tapaba un filtro o el mes. Se muestra y no hay nada que avisar.
 *   · existe, no es suyo → otra partida, o todavía sin partida. Se dice que existe y NADA más; el
 *                          aviso de faltante es lo que lo corrige.
 *   · no está           → o no se depositó, o no se importó ese día. La fecha de carga lo dice.
 *
 * Fecha y monto EXACTOS a propósito: sin rangos no se puede tantear la existencia de movimientos
 * ajenos, que es lo que hace aceptable que el sistema conteste.
 *
 * Hasta el 22-set-2026 el crédito que nadie había clasificado en una cuenta del segmento se mostraba
 * completo acá, porque el usuario lo veía en «Todavía sin partida». Quitada esa pestaña, mostrarlo
 * sería la misma divulgación por otra puerta: hoy cae en «existe y no es tuyo».
 *
 * `carga` es la del encabezado: «conviene esperar» sale de la misma fecha en los dos lados.
 */
function FaltanteDialog({
  carga,
  onCerrar,
}: {
  carga: CargaDelSegmento;
  onCerrar: () => void;
}) {
  const toast = useToast();
  const buscar = useBuscarFaltante();
  const avisar = useReportarFaltante();

  const [fecha, setFecha] = useState("");
  const [monto, setMonto] = useState("");
  const [referencia, setReferencia] = useState("");
  const [motivo, setMotivo] = useState("");
  const [resultado, setResultado] = useState<ResultadoFaltante | null>(null);

  const listoParaBuscar = Boolean(fecha && monto.trim());

  function hacerBusqueda() {
    buscar.mutate(
      { fecha, monto },
      {
        onSuccess: (r) => setResultado(r),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function enviarAviso() {
    avisar.mutate(
      { fecha, monto, referencia, motivo },
      {
        onSuccess: () => {
          toast.success(
            "Aviso enviado. Queda en revisión de quien clasifica; la respuesta la vas a ver en «Mis avisos».",
          );
          onCerrar();
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div
      className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && onCerrar()}
    >
      <div className="flex max-h-[90vh] w-full max-w-lg flex-col gap-3 overflow-y-auto rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <div>
          <h2 className="text-base font-semibold text-content">Falta un movimiento</h2>
          <p className="mt-1 text-sm text-content-muted">
            Poné la fecha y el monto exactos de lo que esperás. Primero lo busco en el banco.
          </p>
        </div>

        <div className="flex flex-wrap gap-3">
          <Input
            label="Fecha del movimiento *"
            type="date"
            value={fecha}
            onChange={(e) => {
              setFecha(e.target.value);
              setResultado(null);
            }}
            className="w-40"
          />
          <div className="min-w-40 flex-1">
            <Input
              label="Monto exacto *"
              value={monto}
              onChange={(e) => {
                setMonto(e.target.value);
                setResultado(null);
              }}
              placeholder="4 950,00"
              hint="Como está en el recibo, hasta los céntimos."
            />
          </div>
        </div>
        {/* Antes de buscar: si ese día todavía no se cargó en alguna de sus cuentas, se dice ya
            (misma regla que el encabezado), sin gastar una búsqueda que queda en auditoría. */}
        {!resultado && avisoFechaSinCargar(fecha, carga) && (
          <p className="text-sm text-pendiente">{avisoFechaSinCargar(fecha, carga)}</p>
        )}

        {!resultado ? (
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={onCerrar}>
              Cancelar
            </Button>
            <Button onClick={hacerBusqueda} loading={buscar.isPending} disabled={!listoParaBuscar}>
              Buscar en el banco
            </Button>
          </div>
        ) : (
          <>
            {resultado.veredicto === "EN_MI_PARTIDA" && (
              <div className="rounded-lg border border-accent/40 bg-accent/5 px-3 py-2 text-sm">
                <p className="font-medium text-content">Sí está, y es de tu partida.</p>
                <p className="mt-0.5 text-content-muted">
                  No aparecía por el mes o los filtros que tenías puestos. No hay nada que avisar.
                </p>
                <ul className="mt-2 flex flex-col gap-1">
                  {resultado.movimientos.map((m) => (
                    <li key={m.id} className="text-content">
                      <span className="tabular-nums">{formatFecha(m.fecha)}</span> · {m.banco} ·{" "}
                      {m.cuenta} — {m.descripcion || "sin detalle"} ·{" "}
                      <b className="tabular-nums">{formatMoneda(m.credito)}</b>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {resultado.veredicto === "FUERA_DE_MI_PARTIDA" && (
              // Se dice que existe y nada más. El detalle no se muestra porque no es algo que el
              // usuario vea: eso es exactamente lo que hay que corregir, y lo corrige quien clasifica.
              <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-3 py-2 text-sm">
                <p className="font-medium text-content">
                  Sí entró ese monto ese día, pero no en tu partida.
                </p>
                <p className="mt-0.5 text-content-muted">
                  Está clasificado en otra partida, o todavía sin partida: por eso no te aparece.
                  Avisá y quien clasifica lo revisa; si es el único de ese monto ese día, el aviso ya
                  lo lleva identificado.
                </p>
              </div>
            )}

            {resultado.veredicto === "NO_EXISTE" && (
              <div className="rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
                <p className="font-medium text-content">No hay ningún ingreso de ese monto ese día.</p>
                {/* La MISMA regla y la MISMA fecha que el encabezado de la pantalla (la cuenta del
                    segmento más atrasada, nombrada): si fueran dos cálculos, se contradirían. */}
                <p className="mt-0.5 text-content-muted">
                  {consejoNoExiste(fecha, cargaDeLaBusqueda(resultado, carga))}
                </p>
              </div>
            )}

            {/* El aviso de FALTANTE solo cuando el movimiento no es algo que el usuario ve: con
                EN_MI_PARTIDA no hay nada que avisar, porque ya lo está mirando. */}
            {(resultado.veredicto === "FUERA_DE_MI_PARTIDA" ||
              resultado.veredicto === "NO_EXISTE") && (
              <div className="flex flex-col gap-3 border-t border-border pt-3">
                <Input
                  label="¿Qué esperabas? *"
                  value={motivo}
                  onChange={(e) => setMotivo(e.target.value)}
                  placeholder="Ej. depósito de ventanilla del cliente Rojas"
                  hint="Lo lee quien clasifica en Bancos."
                />
                <Input
                  label="Referencia (opcional)"
                  value={referencia}
                  onChange={(e) => setReferencia(e.target.value)}
                  placeholder="Nº de recibo, comprobante, SINPE…"
                />
                <div className="flex justify-end gap-2">
                  <Button variant="secondary" onClick={onCerrar} disabled={avisar.isPending}>
                    Cancelar
                  </Button>
                  <Button onClick={enviarAviso} loading={avisar.isPending} disabled={!motivo.trim()}>
                    Enviar el aviso
                  </Button>
                </div>
              </div>
            )}

            {resultado.veredicto === "EN_MI_PARTIDA" && (
              <div className="flex justify-end">
                <Button onClick={onCerrar}>Listo</Button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

/**
 * El aviso «este movimiento no es de mi partida», sobre una fila que el usuario está viendo.
 *
 * El motivo es obligatorio y el servidor también lo exige: sin él no se puede corregir nada, y un
 * aviso vacío obliga a quien clasifica a adivinar o a llamar por teléfono.
 *
 * Tuvo un segundo modo, «Es de mi partida», para los créditos sin clasificar de la pestaña «Todavía
 * sin partida». Se fue con la pestaña el 23-set-2026, y el servidor tampoco acepta ya un aviso sobre
 * un movimiento que esta pantalla no muestra.
 */
function ReportarDialog({
  movimiento,
  onCerrar,
}: {
  movimiento: MovimientoRow;
  onCerrar: () => void;
}) {
  const toast = useToast();
  const reportar = useReportarSegmentacion();
  const [motivo, setMotivo] = useState("");

  function enviar() {
    reportar.mutate(
      { movimientoId: movimiento.id, motivo },
      {
        onSuccess: () => {
          toast.success(
            "Aviso enviado. Queda en revisión de quien clasifica; la respuesta la vas a ver en «Mis avisos».",
          );
          onCerrar();
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div
      className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && onCerrar()}
    >
      <div className="w-full max-w-md rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="text-base font-semibold text-content">Avisar que está mal segmentado</h2>
        <div className="mt-3 rounded-lg border border-border bg-surface-muted px-3 py-2 text-sm">
          <p className="tabular-nums text-content-muted">
            {formatFecha(movimiento.fecha)} · {movimiento.banco} · {movimiento.cuenta}
            {/* La referencia del banco, para que quien avisa cite el mismo número que quien
                clasifica va a buscar. */}
            {movimiento.documento && <> · ref. {movimiento.documento}</>}
          </p>
          <p className="mt-0.5 text-content">{movimiento.descripcion || "—"}</p>
          <p className="mt-0.5 font-semibold tabular-nums text-content">
            {formatMoneda(movimiento.credito)}
          </p>
        </div>
        <div className="mt-3">
          <Input
            label="¿Qué está mal? *"
            value={motivo}
            onChange={(e) => setMotivo(e.target.value)}
            placeholder="Ej. esto es de Emergencias, no de Asociaciones"
            hint="Lo lee quien clasifica en Bancos. Con el detalle correcto se corrige de una."
          />
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="secondary" onClick={onCerrar} disabled={reportar.isPending}>
            Cancelar
          </Button>
          <Button onClick={enviar} loading={reportar.isPending} disabled={!motivo.trim()}>
            Enviar el aviso
          </Button>
        </div>
      </div>
    </div>
  );
}
