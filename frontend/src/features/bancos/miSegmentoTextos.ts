/**
 * Los textos de «Mi partida en bancos» que AFIRMAN algo sobre el dinero (22-set-2026).
 *
 * Viven aparte de la página por la misma razón que rangoFechas.ts: cada frase de acá es una
 * afirmación que el equipo usa para decidir si avisa o espera, y se puede probar sin navegador.
 *
 * Una sola regla, y lo que queda de ella:
 *
 * «Cargado hasta» es la fecha de la cuenta del segmento MÁS ATRASADA, y se la NOMBRA. Decir «el
 * banco está cargado hasta el 11» cuando el BN va al 9 hace que quien espera un depósito del 10 en
 * el BN lea «no entró» cuando lo que pasa es que el BN no se importó. Esa MISMA fecha decide
 * «conviene esperar antes de avisar» (convieneEsperar): si fueran dos cálculos, se contradirían.
 *
 * El 23-set-2026 el Director Financiero sacó ese dato del ENCABEZADO de la pantalla —primero el
 * desplegable por cuenta, después la línea entera— porque no es tema del consultor. Sigue vivo
 * donde sí se necesita y no estorba: pegado a la fecha en «Falta un movimiento» y en la lista
 * vacía, o sea justo cuando el equipo está por decidir si avisa o espera. Por eso
 * `carga_por_cuenta` se sigue recibiendo: es lo que decide si la frase nombra una cuenta o habla
 * de «tus 8 cuentas».
 */

import type { CuentaCargadaHasta } from "@/api/bancos";
import { formatFecha } from "@/lib/format";

const nf = new Intl.NumberFormat("es-CR");

/** Lo que hace falta para hablar de hasta cuándo está cargado el segmento. */
export interface CargaDelSegmento {
  /** YYYY-MM-DD de la cuenta más atrasada; "" si el segmento no tiene cuentas. */
  cargado_hasta: string;
  cargado_hasta_cuenta: CuentaCargadaHasta | null;
  /**
   * Todas las cuentas del segmento. NO se listan en la pantalla: sirven para saber si van todas
   * parejas, que es lo que decide si la frase nombra una cuenta o habla de «tus 8 cuentas».
   */
  carga_por_cuenta: CuentaCargadaHasta[];
}

/**
 * El nombre de una cuenta para ponerlo en una frase.
 *
 * El alias casi siempre ya trae el banco («BN Privado de Cartago», «Davivienda Colones»): repetirlo
 * daría «BN BN Privado de Cartago». Si el alias NO lo trae, se antepone, para que «Colones» a secas
 * no deje adivinar de qué banco es.
 */
export function nombreCuenta(c: { banco: string; cuenta: string }): string {
  const banco = c.banco.trim();
  const cuenta = c.cuenta.trim();
  if (!cuenta) return banco || "La cuenta";
  if (!banco || cuenta.toLowerCase().includes(banco.toLowerCase())) return cuenta;
  return `${banco} · ${cuenta}`;
}

/**
 * La regla ÚNICA de «conviene esperar»: la fecha buscada es posterior a lo que está cargado en la
 * cuenta más atrasada del segmento, así que en esa cuenta ese día todavía no se importó.
 * Las fechas son ISO (YYYY-MM-DD): compararlas como texto es compararlas en el calendario.
 */
export function convieneEsperar(fecha: string | undefined, cargadoHasta: string | undefined): boolean {
  return Boolean(fecha && cargadoHasta && fecha > cargadoHasta);
}

/** Cuántas cuentas tienen fecha, y si van todas al mismo día que la más atrasada. */
function parejas(carga: CargaDelSegmento): { conFecha: number; todasIgual: boolean } {
  const conFecha = (carga.carga_por_cuenta ?? []).filter((c) => c.cargado_hasta);
  return {
    conFecha: conFecha.length,
    todasIgual: conFecha.length > 1 && conFecha.every((c) => c.cargado_hasta === carga.cargado_hasta),
  };
}

/**
 * «BN Privado de Cartago está cargada hasta el 09/09/2026».
 *
 * Si TODAS las cuentas van al mismo día, nombrar una como «la más atrasada» sería inventar una
 * diferencia: se dice «Tus 8 cuentas están cargadas hasta…». "" si no hay fecha.
 */
export function fraseCargadaHasta(carga: CargaDelSegmento): string {
  if (!carga.cargado_hasta) return "";
  const f = formatFecha(carga.cargado_hasta);
  const { conFecha, todasIgual } = parejas(carga);
  if (todasIgual) return `Tus ${nf.format(conFecha)} cuentas están cargadas hasta el ${f}`;
  if (carga.cargado_hasta_cuenta) {
    return `${nombreCuenta(carga.cargado_hasta_cuenta)} está cargada hasta el ${f}`;
  }
  return `Tus cuentas están cargadas hasta el ${f}`;
}

/**
 * La carga que vale para el veredicto de una búsqueda: la fecha y la cuenta que mandó el servidor
 * con NO_EXISTE (es el MISMO cálculo que el encabezado, hecho en el momento de buscar), y si no
 * vinieron, las del encabezado. Nunca mezcla la fecha de uno con la cuenta del otro.
 */
export function cargaDeLaBusqueda(
  r: { cargado_hasta: string; cargado_hasta_cuenta?: CuentaCargadaHasta | null },
  encabezado: CargaDelSegmento,
): CargaDelSegmento {
  if (!r.cargado_hasta) return encabezado;
  return {
    cargado_hasta: r.cargado_hasta,
    cargado_hasta_cuenta: r.cargado_hasta_cuenta ?? null,
    carga_por_cuenta: encabezado.carga_por_cuenta,
  };
}

/**
 * El consejo del diálogo «Falta un movimiento» cuando el veredicto es NO_EXISTE.
 * Usa convieneEsperar: la misma regla y la misma fecha que el encabezado.
 */
export function consejoNoExiste(fecha: string, carga: CargaDelSegmento): string {
  if (convieneEsperar(fecha, carga.cargado_hasta)) {
    const { todasIgual } = parejas(carga);
    const cola = todasIgual || !carga.cargado_hasta_cuenta
      ? "así que ese día todavía no se importó"
      : "así que ese día puede no estar importado todavía en esa cuenta";
    return `${fraseCargadaHasta(carga)}, ${cola}: conviene esperar antes de avisar.`;
  }
  return "Puede que no se haya depositado, o que el monto o la fecha sean otros. Revisá el recibo antes de avisar.";
}

/**
 * El aviso ANTES de buscar, pegado a la fecha del diálogo: con la misma regla, para que no haga
 * falta gastar una búsqueda (que queda en auditoría) en un día que todavía no se cargó.
 * "" si no aplica.
 */
export function avisoFechaSinCargar(fecha: string, carga: CargaDelSegmento): string {
  if (!convieneEsperar(fecha, carga.cargado_hasta)) return "";
  return `${fraseCargadaHasta(carga)}: ese día puede no estar importado todavía.`;
}

/** Lo que dice la lista vacía, según los filtros. */
export function mensajeVacio(p: {
  hayFiltro: boolean;
  mesActivo: string;
  /** El «desde» YA validado (el de revisarRango), o undefined. */
  desde?: string;
  carga: CargaDelSegmento;
}): string {
  // Una fecha posterior a lo importado no es «no entró nada»: es «todavía no se cargó». Misma
  // regla que el diálogo de faltantes.
  if (p.desde && convieneEsperar(p.desde, p.carga.cargado_hasta)) {
    const { todasIgual } = parejas(p.carga);
    const cola = todasIgual || !p.carga.cargado_hasta_cuenta
      ? "eso todavía no se importó"
      : "lo de esa cuenta todavía no se importó";
    return `${fraseCargadaHasta(p.carga)} y pediste desde el ${formatFecha(p.desde)}: ${cola}.`;
  }
  return p.hayFiltro
    ? "Ningún movimiento de tu partida coincide con lo que filtraste. Probá quitando los filtros antes de dar por perdido el movimiento."
    : `Todavía no entró nada de tu partida en ${p.mesActivo}.`;
}

