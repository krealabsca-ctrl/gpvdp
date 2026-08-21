/**
 * Pantalla — Exportar (/exportar).
 *
 * Es un ARMADOR de reportes, no un botón de descarga. La regla del negocio sobre qué se puede
 * pedir: «puede ser periodo o periodos, ambos puede ser; puede ser un concepto o varios, o todos.
 * Esas son las variables para identificar algo específico».
 *
 * Dos decisiones que hacen la diferencia:
 *  · Los filtros son LOS MISMOS de la hoja de trabajo y viajan al mismo WHERE del servidor, así
 *    que lo que se exporta es exactamente lo que se ve en Clasificar con esos filtros.
 *  · Antes de descargar se muestra CUÁNTOS movimientos va a traer. Bajar un Excel para descubrir
 *    que salió vacío (o con 10 000 filas) es perder el viaje.
 */

import { useMemo, useState } from "react";
import {
  BuscadorMultiple,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  PageHeader,
  Select,
  useToast,
} from "@/components/ui";
import { etiquetaPeriodo, formatMonto, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { construirCSV, descargarBlob, descargarCSV } from "@/lib/csv";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import {
  useBancosCatalogo,
  useClasificaciones,
  useConceptos,
  useCuadre,
  useCuentas,
  useMovimientos,
} from "@/features/bancos/hooks";
import { bancosApi, type AgruparReporte, type FiltrosMovimientos } from "@/api/bancos";

const MESES_CORTOS = ["Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul", "Ago", "Sep", "Oct", "Nov", "Dic"];

/** «2026-08» → «Ago». El año se muestra una sola vez, al inicio de su fila. */
function mesCorto(periodo: string): string {
  const mes = Number(periodo.split("-")[1]);
  return MESES_CORTOS[mes - 1] ?? periodo;
}

/**
 * Agrupa los períodos por año, del más reciente al más viejo, y dentro de cada año deja los meses
 * en orden natural (enero a diciembre). La lista llega al revés —del mes activo hacia atrás—, y
 * mostrarla así es lo que hacía difícil ubicar un mes.
 */
function porAnio(periodos: string[]): [string, string[]][] {
  const mapa = new Map<string, string[]>();
  for (const p of periodos) {
    const anio = p.split("-")[0] ?? p;
    const arr = mapa.get(anio);
    if (arr) arr.push(p);
    else mapa.set(anio, [p]);
  }
  return [...mapa.entries()]
    .sort((a, b) => b[0].localeCompare(a[0]))
    .map(([anio, ms]) => [anio, [...ms].sort()] as [string, string[]]);
}

/** Los últimos N meses hacia atrás desde el período activo, para elegir uno o varios. */
function mesesDisponibles(desde: string, cuantos = 18): string[] {
  const [a, m] = desde.split("-").map(Number);
  if (!a || !m) return [desde];
  const out: string[] = [];
  const d = new Date(Date.UTC(a, m - 1, 1));
  for (let i = 0; i < cuantos; i++) {
    out.push(`${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`);
    d.setUTCMonth(d.getUTCMonth() - 1);
  }
  return out;
}

export function ExportarPage() {
  const toast = useToast();
  const { periodo } = usePeriodoActivo();
  const [descMov, setDescMov] = useState(false);
  const [descCua, setDescCua] = useState(false);

  // Selección del reporte. Arranca en el período activo: el caso más común es «este mes».
  const [periodos, setPeriodos] = useState<string[]>([periodo]);
  const [conceptos, setConceptos] = useState<string[]>([]);
  const [clasificaciones, setClasificaciones] = useState<string[]>([]);
  const [bancoId, setBancoId] = useState("");
  const [cuentaId, setCuentaId] = useState("");
  const [tipo, setTipo] = useState("");
  // Presentación del detalle: agrupado con subtotales o listado corrido. Las dos se piden en la
  // práctica —una para analizar, la otra para trabajar el dato en Excel— así que las dos están.
  const [agrupar, setAgrupar] = useState<AgruparReporte>("partida");

  const conceptosQ = useConceptos();
  const clasifQ = useClasificaciones();
  const cuentasQ = useCuentas();
  const bancosQ = useBancosCatalogo();
  const cuadreQuery = useCuadre(periodo);
  const cuadre = cuadreQuery.data ?? [];

  const meses = useMemo(() => mesesDisponibles(periodo), [periodo]);

  const opcConceptos = useMemo(
    () => (conceptosQ.data ?? []).map((c) => ({ value: c.id, label: c.nombre })),
    [conceptosQ.data],
  );
  // Cuando ya hay conceptos elegidos, la lista de clasificaciones se acota a ESOS conceptos: es el
  // recorrido natural (primero el rubro, después el detalle) y baja de cientos a decenas.
  const opcClasificaciones = useMemo(() => {
    const todas = clasifQ.data ?? [];
    const acotadas = conceptos.length > 0 ? todas.filter((c) => conceptos.includes(c.concepto_id)) : todas;
    return acotadas.map((c) => ({ value: c.id, label: c.nombre, grupo: c.concepto }));
  }, [clasifQ.data, conceptos]);

  const bancoElegido = (bancosQ.data ?? []).find((b) => b.id === bancoId);
  const cuentasVisibles = (cuentasQ.data ?? []).filter(
    (c) => !bancoElegido || c.banco === bancoElegido.nombre,
  );

  // El MISMO objeto de filtros que usa la hoja de trabajo. La vista previa y la descarga leen de
  // acá, así que no pueden discrepar.
  const filtros: FiltrosMovimientos = {
    ...(periodos.length === 1 ? { periodo: periodos[0] } : {}),
    ...(periodos.length > 1 ? { periodos } : {}),
    ...(conceptos.length > 0 ? { conceptos } : {}),
    ...(clasificaciones.length > 0 ? { clasificaciones } : {}),
    ...(bancoId ? { banco_id: bancoId } : {}),
    ...(cuentaId ? { cuenta_bancaria_id: cuentaId } : {}),
    ...(tipo ? { tipo: tipo as "DEBITO" | "CREDITO" } : {}),
  };
  // Vista previa: solo interesa el conteo y los totales, no las filas.
  const previa = useMovimientos({ ...filtros, page: 1, page_size: 1 });
  const cuantos = previa.data?.total ?? 0;
  const nadaSeleccionado = periodos.length === 0;

  function alternar(lista: string[], valor: string, set: (v: string[]) => void) {
    set(lista.includes(valor) ? lista.filter((x) => x !== valor) : [...lista, valor]);
  }

  async function exportarMovimientosXLSX() {
    setDescMov(true);
    try {
      const descarga = await bancosApi.exportarMovimientosXLSX(filtros, agrupar);
      // El nombre autoritativo es el del servidor: sabe si los meses son contiguos o sueltos.
      // El del cliente queda solo de respaldo por si la cabecera no llegara.
      const sufijo = agrupar === "ninguno" ? "-corrido" : "";
      const respaldo = `movimientos-${nombreArchivo(periodos)}${sufijo}.xlsx`;
      descargarBlob(descarga.filename || respaldo, descarga.blob);
      toast.success(`${cuantos.toLocaleString("es-CR")} movimientos exportados.`);
    } catch (err) {
      toast.error(mensajeError(err));
    } finally {
      setDescMov(false);
    }
  }

  async function exportarCuadreXLSX() {
    setDescCua(true);
    try {
      const blob = await bancosApi.exportarCuadreXLSX(periodo);
      descargarBlob(`cuadre-${periodo}.xlsx`, blob);
      toast.success("Cuadre exportado en Excel.");
    } catch (err) {
      toast.error(mensajeError(err));
    } finally {
      setDescCua(false);
    }
  }

  function exportarCuadreCSV() {
    if (cuadre.length === 0) {
      toast.error("No hay cuadre para exportar en este período.");
      return;
    }
    const filas = cuadre.map((c) => {
      const neto = toNumber(c.total_creditos) - toNumber(c.total_debitos);
      return [c.concepto, c.total_creditos, c.total_debitos, neto.toFixed(2)];
    });
    descargarCSV(`cuadre-${periodo}.csv`, construirCSV(["Concepto", "Creditos", "Debitos", "Neto"], filas));
    toast.success("Cuadre exportado (CSV).");
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Exportar"
        description="Armá el reporte: uno o varios períodos, y la partida hasta donde haga falta — uno o varios conceptos, una o varias clasificaciones, o todo. El Excel sale con el formato del reporte financiero."
      />

      <Card>
        <CardHeader>
          <CardTitle>Detalle de movimientos</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          {/* PERÍODOS: uno, varios o todos los que se quieran. */}
          <div>
            <div className="flex items-center justify-between">
              <p className="text-xs font-semibold uppercase tracking-wide text-content-muted">
                Períodos
              </p>
              <div className="flex gap-3 text-xs">
                <button type="button" className="text-accent underline" onClick={() => setPeriodos(meses)}>
                  Todos
                </button>
                <button type="button" className="text-accent underline" onClick={() => setPeriodos([periodo])}>
                  Solo {etiquetaPeriodo(periodo)}
                </button>
              </div>
            </div>
            {/* Una fila por AÑO, con los meses abreviados y todos del mismo ancho.
                Antes eran 18 recuadros de ancho variable («Septiembre 2025» junto a «Julio 2026»)
                repartidos en dos renglones sin orden aparente: costaba encontrar un mes. Con el
                año a la izquierda y los meses parejos, se lee como un calendario. */}
            <div className="mt-2 flex flex-col gap-1.5">
              {porAnio(meses).map(([anio, delAnio]) => (
                <div key={anio} className="flex items-center gap-2">
                  <span className="w-10 shrink-0 text-xs font-semibold tabular-nums text-content-muted">
                    {anio}
                  </span>
                  <div className="flex flex-wrap gap-1">
                    {delAnio.map((m) => {
                      const sel = periodos.includes(m);
                      return (
                        <button
                          key={m}
                          type="button"
                          title={etiquetaPeriodo(m)}
                          onClick={() => alternar(periodos, m, setPeriodos)}
                          className={
                            sel
                              ? "w-12 rounded-md border border-accent bg-accent/10 py-1 text-xs font-medium text-accent"
                              : "w-12 rounded-md border border-border py-1 text-xs text-content-muted hover:bg-surface-muted"
                          }
                        >
                          {mesCorto(m)}
                        </button>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* PARTIDA: concepto (el rubro) y clasificación (el detalle). Nada elegido = todo.
              Los dos se eligen ESCRIBIENDO: la parrilla de chips servía con 22 conceptos pero hay
              168 clasificaciones vivas y van a ser cientos. */}
          <div className="grid gap-5 md:grid-cols-2">
            <BuscadorMultiple
              label="Conceptos"
              leyendaVacio="todos"
              placeholder="Buscar concepto…"
              opciones={opcConceptos}
              seleccion={conceptos}
              onChange={(vs) => {
                setConceptos(vs);
                // Si se acota el concepto, las clasificaciones que ya no pertenecen se caen: si
                // quedaran, el filtro pediría algo imposible y el reporte saldría vacío.
                if (vs.length > 0) {
                  const validas = new Set(
                    (clasifQ.data ?? []).filter((c) => vs.includes(c.concepto_id)).map((c) => c.id),
                  );
                  setClasificaciones((prev) => prev.filter((id) => validas.has(id)));
                }
              }}
            />
            <BuscadorMultiple
              label="Clasificaciones"
              leyendaVacio={conceptos.length > 0 ? "todas las de esos conceptos" : "todas"}
              placeholder="Buscar clasificación…"
              opciones={opcClasificaciones}
              seleccion={clasificaciones}
              onChange={setClasificaciones}
            />
          </div>

          {/* El resto de las variables, con los mismos nombres que en Clasificar. */}
          <div className="flex flex-wrap items-end gap-3">
            <Select
              label="Banco"
              value={bancoId}
              onChange={(e) => {
                setBancoId(e.target.value);
                setCuentaId("");
              }}
              options={[
                { value: "", label: "Todos los bancos" },
                ...(bancosQ.data ?? []).map((b) => ({ value: b.id, label: b.nombre })),
              ]}
              className="min-w-40"
            />
            <Select
              label="Cuenta"
              value={cuentaId}
              onChange={(e) => setCuentaId(e.target.value)}
              options={[
                { value: "", label: bancoElegido ? `Todas las de ${bancoElegido.nombre}` : "Todas las cuentas" },
                ...cuentasVisibles.map((c) => ({ value: c.id, label: `${c.alias} · ${c.moneda}` })),
              ]}
              className="min-w-48"
            />
            <Select
              label="Tipo"
              value={tipo}
              onChange={(e) => setTipo(e.target.value)}
              options={[
                { value: "", label: "Débitos y créditos" },
                { value: "CREDITO", label: "Solo créditos (ingresos)" },
                { value: "DEBITO", label: "Solo débitos (egresos)" },
              ]}
              className="min-w-48"
            />
          </div>

          {/* PRESENTACIÓN del detalle. No es un filtro: no cambia qué movimientos entran ni los
              totales, solo cómo se ven. Las dos formas se piden en la práctica. */}
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-content-muted">Presentación</p>
            <div className="mt-2 grid gap-2 sm:grid-cols-2">
              <OpcionPresentacion
                activa={agrupar === "partida"}
                onClick={() => setAgrupar("partida")}
                titulo="Agrupado por partida"
                detalle="Bandas Concepto › Clasificación con el subtotal de cada una. Para analizar y presentar."
              />
              <OpcionPresentacion
                activa={agrupar === "ninguno"}
                onClick={() => setAgrupar("ninguno")}
                titulo="Listado corrido"
                detalle="Una fila por movimiento en orden de fecha, sin subtotales. Suma columnas de Concepto y Clasificación, y autofiltro."
              />
            </div>
          </div>

          {/* Qué va a traer, antes de bajarlo. */}
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-surface-muted px-4 py-3">
            <div className="text-sm">
              {nadaSeleccionado ? (
                <span className="text-negativo">Elegí al menos un período.</span>
              ) : previa.isPending ? (
                <span className="text-content-muted">Contando…</span>
              ) : (
                <>
                  <b>{cuantos.toLocaleString("es-CR")}</b> movimiento{cuantos === 1 ? "" : "s"} ·{" "}
                  <span className="text-content-muted">
                    débitos ₡{formatMonto(previa.data?.totales.total_debitos)} · créditos ₡
                    {formatMonto(previa.data?.totales.total_creditos)}
                  </span>
                  {(previa.data?.totales.sin_tipo_cambio ?? 0) > 0 && (
                    <span className="block text-xs text-pendiente">
                      {previa.data!.totales.sin_tipo_cambio} en dólares sin tipo de cambio: no suman al total
                      en colones (queda advertido en el reporte).
                    </span>
                  )}
                </>
              )}
            </div>
            <Button onClick={exportarMovimientosXLSX} loading={descMov} disabled={nadaSeleccionado || cuantos === 0}>
              {agrupar === "partida" ? "Exportar agrupado" : "Exportar corrido"}
            </Button>
          </div>

          <p className="text-xs text-content-muted">
            El libro trae tres hojas: <b>Movimientos</b> en la presentación que elijás,{" "}
            <b>Resumen por partida</b> y <b>Resumen por cuenta</b> — las dos de resumen son iguales en ambas
            presentaciones, y el gran total también: elegir cómo se ve no cambia los números. Sin cuadrícula,
            fechas dd/mm/aaaa y montos en formato contable. La columna <b>Consecutivo largo</b> se completa
            en las cuentas de Davivienda.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Cuadre por concepto</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-sm text-content-muted">
            Créditos, débitos y neto por concepto — {etiquetaPeriodo(periodo)}.
            {cuadreQuery.data && ` (${cuadre.length} concepto(s))`}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button onClick={exportarCuadreXLSX} loading={descCua}>
              Exportar cuadre (Excel)
            </Button>
            <Button
              variant="secondary"
              onClick={exportarCuadreCSV}
              disabled={cuadreQuery.isPending || cuadre.length === 0}
            >
              CSV
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

/** Tarjeta de opción de presentación: el título y lo que cambia, para no elegir a ciegas. */
function OpcionPresentacion({
  activa,
  onClick,
  titulo,
  detalle,
}: {
  activa: boolean;
  onClick: () => void;
  titulo: string;
  detalle: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={activa}
      className={
        activa
          ? "rounded-xl border border-accent bg-accent/10 px-3 py-2 text-left"
          : "rounded-xl border border-border px-3 py-2 text-left hover:bg-surface-muted"
      }
    >
      <span className={activa ? "block text-sm font-semibold text-accent" : "block text-sm font-medium text-content"}>
        {titulo}
      </span>
      <span className="mt-0.5 block text-xs text-content-muted">{detalle}</span>
    </button>
  );
}

/**
 * Nombre de RESPALDO, solo si el servidor no mandó Content-Disposition. Dice el rango que abarca
 * y aclara cuando los meses son sueltos: «2026-01_a_2026-08» con huecos anunciaría ocho meses
 * trayendo dos.
 */
function nombreArchivo(periodos: string[]): string {
  const orden = [...periodos].sort();
  const primero = orden[0] ?? "seleccion";
  const ultimo = orden[orden.length - 1] ?? primero;
  if (orden.length <= 1) return primero;
  return mesesContiguos(orden) ? `${primero}_a_${ultimo}` : `seleccion-${orden.length}-periodos`;
}

/** Meses YYYY-MM consecutivos sin huecos (mismo criterio que `periodosContiguos` del backend). */
function mesesContiguos(ordenados: string[]): boolean {
  for (let i = 1; i < ordenados.length; i++) {
    const previo = ordenados[i - 1];
    const actual = ordenados[i];
    if (!previo || !actual) return false;
    const [a1, m1] = previo.split("-").map(Number);
    const [a2, m2] = actual.split("-").map(Number);
    if (!a1 || !m1 || !a2 || !m2) return false;
    if (a1 * 12 + m1 + 1 !== a2 * 12 + m2) return false;
  }
  return true;
}
