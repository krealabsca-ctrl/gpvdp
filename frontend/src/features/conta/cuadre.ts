/**
 * El cuadre del asiento, calculado en la pantalla mientras se escribe.
 *
 * ── POR QUÉ EXISTE SI LA BASE YA LO VERIFICA ────────────────────────────────────────────────
 *
 * La base es la que manda: su disparador rechaza todo asiento descuadrado, venga de donde venga.
 * Esto es otra cosa: que quien está cargando el asiento vea la diferencia MIENTRAS la causa, en
 * vez de enterarse al apretar guardar. Con cuatro líneas y un error de un dígito, el servidor
 * responde «no cuadra por 100» y hay que volver a sumar a mano para encontrar cuál es.
 *
 * ── POR QUÉ EN CÉNTIMOS ─────────────────────────────────────────────────────────────────────
 *
 * Porque 0.1 + 0.2 no da 0.3 en coma flotante, y un asiento de centavos marcaría descuadre sin
 * estarlo. Es la misma razón por la que el backend usa decimal y nunca float64.
 */

import { centimosAPlano, montoACentimos } from "@/lib/format";
import type { NuevaLineaAsiento } from "@/api/conta";

export interface Cuadre {
  /** Totales en céntimos, para comparar sin errores de redondeo. */
  debe: number;
  haber: number;
  /** Debe menos haber. Positivo = sobra al debe. */
  diferencia: number;
  cuadra: boolean;
  /** Líneas que tienen algún monto. Un asiento necesita al menos dos. */
  conMonto: number;
  /** Los mismos totales ya formateados, para mostrarlos. */
  debeTexto: string;
  haberTexto: string;
  diferenciaTexto: string;
}

export function calcularCuadre(lineas: NuevaLineaAsiento[]): Cuadre {
  let debe = 0;
  let haber = 0;
  let conMonto = 0;
  for (const l of lineas) {
    const d = montoACentimos(l.debe);
    const h = montoACentimos(l.haber);
    debe += d;
    haber += h;
    if (d !== 0 || h !== 0) conMonto++;
  }
  const diferencia = debe - haber;
  return {
    debe,
    haber,
    diferencia,
    // Un asiento vacío no «cuadra»: cuadra en cero, que no es lo mismo. Sin esta condición el
    // botón de guardar se habilitaría sobre un formulario en blanco.
    cuadra: diferencia === 0 && conMonto >= 2,
    conMonto,
    debeTexto: centimosAPlano(debe),
    haberTexto: centimosAPlano(haber),
    diferenciaTexto: centimosAPlano(Math.abs(diferencia)),
  };
}

/** Una línea vacía, para arrancar el formulario y para el botón de «agregar línea». */
export function lineaVacia(): NuevaLineaAsiento {
  return { cuenta_id: "", debe: "", haber: "", detalle: "" };
}

/**
 * Qué le falta al asiento para poder guardarse, en una frase.
 *
 * Devuelve "" cuando está listo. El orden importa: se dice UNA cosa a la vez, la primera que
 * falta, porque una lista de cinco problemas sobre un formulario a medio llenar no se lee.
 */
export function queLeFalta(
  fecha: string,
  glosa: string,
  lineas: NuevaLineaAsiento[],
  c: Cuadre,
): string {
  if (!fecha) return "Falta la fecha.";
  if (!glosa.trim()) return "Falta la glosa: es lo que alguien va a leer en el diario.";
  if (lineas.some((l) => (montoACentimos(l.debe) !== 0 || montoACentimos(l.haber) !== 0) && !l.cuenta_id)) {
    return "Hay una línea con monto y sin cuenta.";
  }
  if (lineas.some((l) => montoACentimos(l.debe) !== 0 && montoACentimos(l.haber) !== 0)) {
    return "Una línea va al debe o al haber, no a los dos.";
  }
  if (c.conMonto < 2) return "Un asiento necesita al menos una línea al debe y otra al haber.";
  if (c.diferencia !== 0) {
    const lado = c.diferencia > 0 ? "al debe" : "al haber";
    return `No cuadra: sobran ₡${c.diferenciaTexto} ${lado}.`;
  }
  return "";
}
