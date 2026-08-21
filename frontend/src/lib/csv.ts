/**
 * Utilidades de exportación CSV (cliente). Genera CSV con separador coma,
 * escapando comillas/comas/saltos, y dispara la descarga con BOM UTF-8 para que
 * Excel abra bien los acentos (₡, tildes).
 */

type Celda = string | number | null | undefined;

function celda(v: Celda): string {
  const s = v === null || v === undefined ? "" : String(v);
  return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

/** Construye el texto CSV a partir de encabezados + filas. */
export function construirCSV(headers: string[], filas: Celda[][]): string {
  const lineas = [headers.map(celda).join(",")];
  for (const fila of filas) lineas.push(fila.map(celda).join(","));
  return lineas.join("\r\n");
}

/** Dispara la descarga de un CSV en el navegador (BOM UTF-8 para Excel). */
export function descargarCSV(nombreArchivo: string, csv: string): void {
  const blob = new Blob(["﻿" + csv], { type: "text/csv;charset=utf-8" });
  descargarBlob(nombreArchivo, blob);
}

/** Dispara la descarga de un Blob cualquiera (p. ej. .xlsx del backend). */
export function descargarBlob(nombreArchivo: string, blob: Blob): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = nombreArchivo;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
