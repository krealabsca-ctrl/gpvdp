/**
 * Documento imprimible del finiquito: se abre en una ventana y el usuario lo guarda como
 * PDF con el diálogo del navegador (Ctrl+P → «Guardar como PDF»). Sin dependencias nuevas
 * ni generación en el servidor: el documento sale con el formato y los montos ya calculados.
 */

import { formatFecha, formatMoneda, toNumber } from "@/lib/format";
import type { Finiquito, MotivoCese } from "@/api/rrhh";

const ETIQUETA_MOTIVO: Record<MotivoCese, string> = {
  DESPIDO_RESPONSABILIDAD: "Despido con responsabilidad patronal",
  RENUNCIA: "Renuncia",
  FIN_CONTRATO: "Fin de contrato",
  MUTUO_ACUERDO: "Mutuo acuerdo",
};

const ESCAPES: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};

/** Escapa el texto para incrustarlo en el HTML del documento. */
function esc(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ESCAPES[c] ?? c);
}

/**
 * Abre el finiquito en una ventana lista para imprimir o guardar como PDF.
 * Devuelve false si el navegador bloqueó la ventana emergente.
 */
export function abrirFiniquitoImprimible(f: Finiquito, empresa: string): boolean {
  const ventana = window.open("", "_blank", "width=860,height=1000");
  if (!ventana) return false;

  const fila = (etiqueta: string, monto: string, resta = false) => `
    <tr>
      <td>${esc(etiqueta)}</td>
      <td class="r${resta ? " neg" : ""}">${resta ? "−" : ""}${formatMoneda(monto, "CRC")}</td>
    </tr>`;

  const rubros = f.detalle
    .map((d) => fila(d.nombre, d.monto, d.tipo !== "INGRESO"))
    .join("");

  const afecta = toNumber(f.base_ccss) > 0
    ? `<p class="nota">
         Porción afecta a cargas sociales: ${formatMoneda(f.base_ccss, "CRC")} (vacaciones pendientes).
         CCSS del trabajador retenido: ${formatMoneda(f.ccss_obrero, "CRC")}. Impuesto al salario:
         ${formatMoneda(f.renta, "CRC")}. El preaviso, el auxilio de cesantía y el aguinaldo proporcional
         son exentos de cargas y de renta.
       </p>`
    : "";

  ventana.document.write(`<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<title>Finiquito — ${esc(f.empleado_nombre)}</title>
<style>
  @page { size: letter; margin: 18mm 16mm; }
  * { box-sizing: border-box; }
  body { font: 12px/1.55 ui-sans-serif, system-ui, "Segoe UI", Roboto, sans-serif; color: #1E2A24; margin: 0; }
  h1 { font-size: 17px; margin: 0 0 2px; letter-spacing: -.01em; }
  h2 { font-size: 12px; margin: 22px 0 6px; text-transform: uppercase; letter-spacing: .07em; color: #55645C; }
  .enc { display: flex; justify-content: space-between; align-items: flex-start; gap: 20px;
         border-bottom: 2px solid #0F6E4E; padding-bottom: 10px; }
  .emp { font-weight: 700; color: #0F6E4E; font-size: 13px; }
  .meta { font-size: 11px; color: #55645C; text-align: right; }
  table { width: 100%; border-collapse: collapse; margin-top: 4px; }
  td, th { padding: 6px 8px; border-bottom: 1px solid #DCE2D9; text-align: left; vertical-align: top; }
  th { font-size: 10px; text-transform: uppercase; letter-spacing: .05em; color: #55645C; background: #EDF0EA; }
  .r { text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .neg { color: #B3542E; }
  .total td { border-top: 2px solid #1E2A24; border-bottom: 0; font-weight: 700; font-size: 14px; padding-top: 10px; }
  .datos { display: grid; grid-template-columns: repeat(2, 1fr); gap: 4px 24px; margin-top: 4px; }
  .datos div { display: flex; justify-content: space-between; gap: 12px; border-bottom: 1px dashed #DCE2D9; padding: 3px 0; }
  .datos span:first-child { color: #55645C; }
  .nota { font-size: 10.5px; color: #55645C; background: #F7EEDF; border: 1px solid #E4D3B0;
          border-radius: 8px; padding: 9px 11px; margin-top: 14px; line-height: 1.5; }
  .firmas { display: grid; grid-template-columns: 1fr 1fr; gap: 44px; margin-top: 52px; }
  .firma { border-top: 1px solid #1E2A24; padding-top: 6px; font-size: 11px; text-align: center; }
  .pie { margin-top: 28px; font-size: 9.5px; color: #8A958D; text-align: center; }
  @media print { .noprint { display: none; } }
  .noprint { margin-top: 22px; text-align: center; }
  .noprint button { font: inherit; font-weight: 700; padding: 9px 18px; border: 0; border-radius: 9px;
                    background: #0F6E4E; color: #fff; cursor: pointer; }
</style>
</head>
<body>
  <div class="enc">
    <div>
      <div class="emp">${esc(empresa)}</div>
      <h1>Liquidación de prestaciones — finiquito</h1>
      <div class="meta" style="text-align:left">Conforme al Código de Trabajo de Costa Rica</div>
    </div>
    <div class="meta">
      Estado: <b>${esc(f.estado)}</b><br>
      ${f.pagado_en ? "Pagado: " + formatFecha(f.pagado_en.slice(0, 10)) : "Emitido: " + formatFecha(f.creado_en.slice(0, 10))}
    </div>
  </div>

  <h2>Datos del trabajador</h2>
  <div class="datos">
    <div><span>Nombre</span><b>${esc(f.empleado_nombre)}</b></div>
    <div><span>Identificación</span><b>${esc(f.identificacion)}</b></div>
    <div><span>Fecha de ingreso</span><b>${formatFecha(f.fecha_ingreso)}</b></div>
    <div><span>Fecha de salida</span><b>${formatFecha(f.fecha_salida)}</b></div>
    <div><span>Antigüedad</span><b>${f.anios_servicio} año${f.anios_servicio === 1 ? "" : "s"}</b></div>
    <div><span>Motivo de cese</span><b>${esc(ETIQUETA_MOTIVO[f.motivo] ?? f.motivo)}</b></div>
    <div><span>Salario promedio mensual</span><b>${formatMoneda(f.salario_promedio, "CRC")}</b></div>
    <div><span>Salario diario</span><b>${formatMoneda(f.salario_diario, "CRC")}</b></div>
  </div>

  <h2>Detalle de la liquidación</h2>
  <table>
    <thead><tr><th>Rubro</th><th class="r">Monto</th></tr></thead>
    <tbody>
      ${rubros}
      <tr class="total"><td>Total a pagar</td><td class="r">${formatMoneda(f.total, "CRC")}</td></tr>
    </tbody>
  </table>
  ${afecta}

  <p class="nota">
    El cálculo se hizo sobre el salario promedio real del trabajador, que incluye comisiones y bonificaciones.
    El auxilio de cesantía se computa con un tope de ocho años de antigüedad. Las vacaciones pendientes son
    salario y se reportan en la planilla de la Caja Costarricense de Seguro Social.
  </p>

  <div class="firmas">
    <div class="firma">Firma del trabajador</div>
    <div class="firma">Firma del patrono</div>
  </div>

  <div class="pie">
    Documento generado por el ERP de ${esc(empresa)} · Recibí conforme la suma indicada por concepto de
    prestaciones legales.
  </div>

  <div class="noprint">
    <button onclick="window.print()">Imprimir o guardar como PDF</button>
  </div>
</body>
</html>`);
  ventana.document.close();
  return true;
}
