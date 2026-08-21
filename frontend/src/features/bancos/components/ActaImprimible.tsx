/**
 * Documento imprimible del acta de conciliación bancaria. Mismo patrón que el finiquito: se
 * abre en una ventana y el usuario la guarda como PDF con el diálogo del navegador.
 *
 * El acta es un documento firmable: lleva las dos puntas (banco y libros), las partidas que
 * explican la diferencia, la declaración de conformidad y los espacios de firma de quien la
 * preparó y quien la revisó.
 */

import { etiquetaPeriodo, formatFecha, formatMoneda, toNumber } from "@/lib/format";
import type { ActaConciliacion, TipoPartida } from "@/api/bancos";

const ETIQUETA_TIPO: Record<TipoPartida, string> = {
  DEPOSITO_NO_ACREDITADO: "Depósito registrado sin acreditar en el banco",
  TRANSFERENCIA_NO_PRESENTADA: "Transferencia girada y no debitada por el banco",
  CARGO_BANCO_NO_REGISTRADO: "Cargo del banco no registrado en libros",
  ABONO_BANCO_NO_REGISTRADO: "Abono del banco no registrado en libros",
  OTRA: "Otra partida en tránsito",
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
 * Abre el acta lista para imprimir o guardar como PDF.
 * Devuelve false si el navegador bloqueó la ventana emergente.
 */
export function abrirActaImprimible(a: ActaConciliacion, empresa: string, periodo: string): boolean {
  const ventana = window.open("", "_blank", "width=860,height=1000");
  if (!ventana) return false;

  const moneda = a.moneda === "USD" ? "USD" : "CRC";
  const m = (v: string) => formatMoneda(v, moneda);

  const partidas = a.partidas.length
    ? a.partidas
        .map(
          (p) => `
      <tr>
        <td>${esc(ETIQUETA_TIPO[p.tipo])}<div class="sub">${esc(p.descripcion)}</div></td>
        <td class="r${p.signo < 0 ? " neg" : ""}">${p.signo < 0 ? "−" : "+"}${m(p.monto)}</td>
      </tr>`,
        )
        .join("")
    : `<tr><td colspan="2" class="vacio">Sin partidas en tránsito: el saldo del banco coincide con el de libros.</td></tr>`;

  const conforme = a.cuadra
    ? `<p class="nota ok">
         <b>Conciliación conforme.</b> La diferencia entre el saldo del estado de cuenta y el saldo
         según los registros del sistema queda explicada en su totalidad por las partidas en
         tránsito detalladas arriba. Diferencia sin explicar: ${m("0")}.
       </p>`
    : `<p class="nota alerta">
         <b>Conciliación NO conforme.</b> Queda una diferencia sin explicar de
         ${m(a.diferencia_sin_explicar)}. El período no puede cerrarse hasta identificar su origen.
       </p>`;

  ventana.document.write(`<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<title>Acta de conciliación — ${esc(a.alias)} — ${esc(periodo)}</title>
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
  .sub { font-size: 10.5px; color: #55645C; margin-top: 2px; }
  .vacio { color: #8A958D; font-style: italic; }
  .total td { border-top: 2px solid #1E2A24; border-bottom: 0; font-weight: 700; font-size: 13.5px; padding-top: 10px; }
  .datos { display: grid; grid-template-columns: repeat(2, 1fr); gap: 4px 24px; margin-top: 4px; }
  .datos div { display: flex; justify-content: space-between; gap: 12px; border-bottom: 1px dashed #DCE2D9; padding: 3px 0; }
  .datos span:first-child { color: #55645C; }
  .nota { font-size: 10.5px; border-radius: 8px; padding: 9px 11px; margin-top: 14px; line-height: 1.5; }
  .nota.ok { color: #1E4B36; background: #E8F2EC; border: 1px solid #B9D8C7; }
  .nota.alerta { color: #7A3A1E; background: #F7EEDF; border: 1px solid #E4D3B0; }
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
      <h1>Acta de conciliación bancaria</h1>
      <div class="meta" style="text-align:left">${esc(a.alias)} · ${esc(a.banco)} · ${esc(a.moneda)}</div>
    </div>
    <div class="meta">
      Período: <b>${esc(etiquetaPeriodo(periodo))}</b><br>
      ${
        a.firmado_en
          ? `Firmada: ${formatFecha(a.firmado_en.slice(0, 10))}<br>por ${esc(a.firmado_por)}`
          : "Estado: <b>sin firmar</b>"
      }
    </div>
  </div>

  <h2>Saldo según el estado de cuenta</h2>
  <div class="datos">
    <div><span>Saldo al cierre del período</span><span class="r"><b>${m(a.saldo_banco)}</b></span></div>
    <div><span>Fecha del saldo</span><span class="r">${esc(a.fecha_banco || "—")}</span></div>
  </div>

  <h2>Saldo según los registros del sistema</h2>
  <div class="datos">
    <div><span>Saldo inicial (${esc(a.fecha_inicial || "—")})</span><span class="r">${m(a.saldo_inicial)}</span></div>
    <div><span>Créditos del período</span><span class="r">+${m(a.entradas_mes)}</span></div>
    <div><span>Débitos del período</span><span class="r neg">−${m(a.salidas_mes)}</span></div>
    <div><span>Saldo según libros</span><span class="r"><b>${m(a.saldo_libros)}</b></span></div>
  </div>

  <h2>Partidas en tránsito que explican la diferencia</h2>
  <table>
    <thead><tr><th>Concepto</th><th class="r">Ajuste</th></tr></thead>
    <tbody>
      ${partidas}
      <tr class="total">
        <td>Total de ajustes</td>
        <td class="r">${toNumber(a.ajuste_partidas) < 0 ? "−" : "+"}${m(
          String(Math.abs(toNumber(a.ajuste_partidas))),
        )}</td>
      </tr>
    </tbody>
  </table>

  <h2>Cuadre</h2>
  <table>
    <tbody>
      <tr><td>Saldo del estado de cuenta</td><td class="r">${m(a.saldo_banco)}</td></tr>
      <tr><td>Más / menos partidas en tránsito</td><td class="r">${
        toNumber(a.ajuste_partidas) < 0 ? "−" : "+"
      }${m(String(Math.abs(toNumber(a.ajuste_partidas))))}</td></tr>
      <tr><td>Menos saldo según libros</td><td class="r neg">−${m(a.saldo_libros)}</td></tr>
      <tr class="total">
        <td>Diferencia sin explicar</td>
        <td class="r">${m(a.diferencia_sin_explicar)}</td>
      </tr>
    </tbody>
  </table>

  ${conforme}

  <div class="firmas">
    <div class="firma">Preparado por<br>Tesorería</div>
    <div class="firma">${
      a.firmado_en ? esc(a.firmado_por) : "Revisado y aprobado por"
    }<br>Dirección Financiera</div>
  </div>

  <div class="pie">
    GPVDP ERP · Acta generada del registro del sistema. El saldo del estado de cuenta y los
    movimientos del período son los capturados y cargados al momento de la firma.
  </div>

  <div class="noprint">
    <button onclick="window.print()">Imprimir o guardar como PDF</button>
  </div>
</body>
</html>`);
  ventana.document.close();
  return true;
}
