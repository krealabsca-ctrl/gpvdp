/**
 * Relación de pagos de un corte, lista para imprimir o guardar como PDF.
 *
 * Mismo patrón que el acta de conciliación y el finiquito: se abre en una ventana y el usuario la
 * guarda como PDF con el diálogo del navegador. Sin dependencias nuevas y sin un generador de PDF
 * del lado del servidor, que para un documento de una o dos páginas no se paga.
 *
 * ── QUÉ LO DISTINGUE DEL PANEL DE PANTALLA ─────────────────────────────────────────────────────
 *
 * El panel sirve para REVISAR antes de subir el archivo; esto sirve para AUTORIZAR. Son dos usos
 * distintos y por eso el documento lleva tres cosas que la pantalla no necesita:
 *
 *   · El pie de firmas (preparó / revisó / autorizó). Gerencia General firma el papel, no la
 *     pantalla, y un documento sin dónde firmar vuelve al escritorio de quien lo mandó.
 *   · El sello de cuándo se imprimió. Un corte vive: se le mueven facturas, se le anulan. Sin la
 *     marca de tiempo, dos impresiones del mismo lote con montos distintos no se pueden ordenar.
 *   · El total en letras. Es lo que vuelve el documento verificable de un vistazo contra el archivo
 *     del banco, y lo que impide que un cero de más pase sin que nadie lo note.
 *
 * ── IMPRESIÓN ──────────────────────────────────────────────────────────────────────────────────
 *
 * El detalle se corta por donde caiga: con 20 transferencias, obligar a que cada bloque quede
 * entero deja páginas medio vacías y el documento crece sin motivo. Lo que SÍ se mantiene junto es
 * el encabezado de cada transferencia con su primera fila (`break-after: avoid`), para que nunca
 * quede un proveedor solo al pie de una hoja; y `thead` se repite en cada página.
 */

import { formatFecha, formatMoneda, toNumber } from "@/lib/format";
import type { ReporteLote } from "@/api/cxp";

const ESCAPES: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};

/** Escapa el texto para incrustarlo en el HTML del documento. */
function esc(s: string): string {
  return String(s ?? "").replace(/[&<>"]/g, (c) => ESCAPES[c] ?? c);
}

const UNIDADES = [
  "", "un", "dos", "tres", "cuatro", "cinco", "seis", "siete", "ocho", "nueve", "diez",
  "once", "doce", "trece", "catorce", "quince", "dieciséis", "diecisiete", "dieciocho", "diecinueve",
];
const DECENAS = ["", "", "veinte", "treinta", "cuarenta", "cincuenta", "sesenta", "setenta", "ochenta", "noventa"];
const CENTENAS = [
  "", "ciento", "doscientos", "trescientos", "cuatrocientos", "quinientos",
  "seiscientos", "setecientos", "ochocientos", "novecientos",
];

// Los veintis se escriben juntos y con tilde («veintidós», «veintiséis»), no como «veinte y dos».
// Van en tabla y no derivados: las tres tildes y el «veintiún» son justo lo que una regla genérica
// escribe mal, y acá se imprimen en un papel que alguien firma.
const VEINTIS = [
  "veinte", "veintiún", "veintidós", "veintitrés", "veinticuatro",
  "veinticinco", "veintiséis", "veintisiete", "veintiocho", "veintinueve",
];

/** Convierte 0–999 a letras. */
function hasta999(n: number): string {
  if (n === 0) return "";
  if (n === 100) return "cien";
  const c = Math.floor(n / 100);
  const resto = n % 100;
  const partes: string[] = [];
  if (c > 0) partes.push(CENTENAS[c]!);
  if (resto > 0) {
    if (resto < 20) partes.push(resto === 1 ? "un" : UNIDADES[resto]!);
    else if (resto < 30) partes.push(VEINTIS[resto - 20]!);
    else {
      const d = Math.floor(resto / 10);
      const u = resto % 10;
      partes.push(u === 0 ? DECENAS[d]! : `${DECENAS[d]} y ${UNIDADES[u]}`);
    }
  }
  return partes.join(" ");
}

/**
 * Monto en letras para el documento firmable («CIENTO VEINTE MIL TRESCIENTOS COLONES CON 50/100»).
 *
 * Los céntimos van en quebrado y no en letras, que es como se escribe en un documento de pago en
 * Costa Rica. Sobre mil millones devuelve "" y el documento omite la línea: antes que arriesgar una
 * cifra mal escrita en un papel que alguien firma, es preferible que no esté.
 */
export function montoEnLetras(valor: string): string {
  // Se parsea acá y no con `toNumber`, que colapsa el vacío y lo ilegible a CERO. Esa conversión es
  // la correcta para mostrar un saldo en pantalla, pero no acá: un total ausente se imprimiría como
  // «cero colones», que en un documento firmado no es un dato faltante sino la AFIRMACIÓN de que no
  // se paga nada. Un «0» explícito sí se escribe.
  const texto = String(valor ?? "").trim();
  if (texto === "") return "";
  const n = Number(texto);
  if (!Number.isFinite(n) || n < 0 || n >= 1_000_000_000) return "";
  const entero = Math.floor(n);
  const centimos = Math.round((n - entero) * 100);
  const quebrado = `${String(centimos).padStart(2, "0")}/100`;
  if (entero === 0) return `cero colones con ${quebrado}`;

  const millones = Math.floor(entero / 1_000_000);
  const miles = Math.floor((entero % 1_000_000) / 1000);
  const resto = entero % 1000;

  const partes: string[] = [];
  if (millones === 1) partes.push("un millón");
  else if (millones > 1) partes.push(`${hasta999(millones)} millones`);
  if (miles === 1) partes.push("mil");
  else if (miles > 1) partes.push(`${hasta999(miles)} mil`);
  if (resto > 0) partes.push(hasta999(resto));

  return `${partes.join(" ")} ${entero === 1 ? "colón" : "colones"} con ${quebrado}`;
}

/**
 * Abre la relación de pagos lista para imprimir o guardar como PDF.
 * Devuelve false si el navegador bloqueó la ventana emergente.
 */
export function abrirReporteLoteImprimible(r: ReporteLote, empresa: string): boolean {
  const ventana = window.open("", "_blank", "width=900,height=1000");
  if (!ventana) return false;

  const m = (v: string) => formatMoneda(v, "CRC");
  const agrupado = r.modo_pago === "PROVEEDOR";
  const impreso = new Date().toLocaleString("es-CR", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });

  const bloques = r.pagos
    .map((p, i) => {
      const filas = p.lineas
        .map(
          (l) => `
        <tr>
          <td class="mono">${esc(l.consecutivo || "—")}</td>
          <td>${esc(l.gasto || "—")}</td>
          <td class="c">${l.fecha_emision ? formatFecha(l.fecha_emision) : "—"}</td>
          <td class="c">${l.fecha_vencimiento ? formatFecha(l.fecha_vencimiento) : "—"}</td>
          <td class="r">${m(l.total_crc)}</td>
          <td class="r${toNumber(l.anticipos_crc) > 0 ? " neg" : ""}">${
            toNumber(l.anticipos_crc) > 0 ? `−${m(l.anticipos_crc)}` : "—"
          }</td>
          <td class="r b">${m(l.neto_crc)}</td>
        </tr>`,
        )
        .join("");

      // El subtotal solo cuando el pago cubre más de una factura: con una sola, repetir el mismo
      // número abajo no agrega nada y alarga el documento.
      const subtotal =
        p.lineas.length > 1
          ? `<tr class="sub"><td colspan="6">Subtotal de la transferencia (${p.lineas.length} facturas)</td><td class="r b">${m(
              p.total_crc,
            )}</td></tr>`
          : "";

      return `
    <div class="pago">
      <div class="cab">
        <div>
          <span class="n">${i + 1}.</span>
          <span class="prov">${esc(p.proveedor)}</span>
          <span class="ref mono">${esc(p.referencia)}</span>
          ${p.lineas.length > 1 ? `<span class="chip">${p.lineas.length} facturas en un pago</span>` : ""}
          ${p.iban ? "" : `<span class="chip mal">SIN IBAN — el banco rechaza esta línea</span>`}
          <!-- El IBAN en MAYÚSCULAS aunque se haya cargado en minúscula: así se escribe una cuenta
               bancaria, y es contra este papel que alguien la va a cotejar con la del proveedor. -->
          <div class="iban mono">${
            p.iban ? esc(p.iban.toUpperCase()) : "cuenta no registrada"
          } · cód. ${esc(p.huella || "—")}</div>
        </div>
        <div class="monto">${m(p.total_crc)}</div>
      </div>
      <table>
        <thead>
          <tr>
            <th>Factura</th><th>Gasto</th><th class="c">Emisión</th><th class="c">Vence</th>
            <th class="r">Total</th><th class="r">Anticipos</th><th class="r">Neto a pagar</th>
          </tr>
        </thead>
        <tbody>${filas}${subtotal}</tbody>
      </table>
    </div>`;
    })
    .join("");

  const letras = montoEnLetras(r.total_crc);
  const alerta =
    r.sin_iban > 0
      ? `<p class="aviso">⚠ <b>${r.sin_iban} transferencia(s) sin cuenta IBAN.</b> El banco rechaza esas líneas:
         hay que registrar la cuenta de esos proveedores antes de subir el archivo. El total de abajo
         las incluye.</p>`
      : "";

  ventana.document.write(`<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<title>Relación de pagos — Lote ${r.lote_numero} — ${esc(empresa)}</title>
<style>
  @page { size: letter; margin: 0 0 14mm; }
  * { box-sizing: border-box; }
  /* Papel BLANCO y declarado, pase lo que pase.
     Un documento es papel: no tiene tema claro ni oscuro. Sin estas dos líneas el fondo lo pone el
     sistema, y en un equipo con tema oscuro la ventana sale negra con el texto gris encima —ilegible
     justo en el momento en que alguien la revisa antes de mandarla a firmar—. El color-scheme
     además evita que el navegador reinterprete los colores por su cuenta. */
  :root { color-scheme: light; --verde: #0F6E4E; --oro: #B0894A; --tinta: #16211D; --gris: #5B6A63; --linea: #DFE6E2; }
  html, body { background: #FFFFFF; }
  body { font: 10.5px/1.55 ui-sans-serif, system-ui, "Segoe UI", Roboto, sans-serif;
         color: var(--tinta); margin: 0; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
  .hoja { padding: 0 14mm; }
  .mono { font-family: ui-monospace, "Cascadia Mono", Consolas, monospace; letter-spacing: -.01em; }
  .r { text-align: right; } .c { text-align: center; } .b { font-weight: 600; }
  .neg { color: #A3321E; }

  /* ── Cabecera: banda de marca a sangre. Es lo que hace que el papel se reconozca de lejos como
     un documento de la empresa y no como una impresión de pantalla. ───────────────────────────── */
  .banda { background: var(--verde); color: #fff; padding: 11mm 14mm 7mm; }
  .banda .fila { display: flex; justify-content: space-between; align-items: flex-end; gap: 20px; }
  .banda .emp { font-size: 10px; letter-spacing: .16em; text-transform: uppercase; opacity: .85; }
  .banda h1 { font-size: 21px; margin: 2px 0 0; font-weight: 700; letter-spacing: -.015em; }
  .banda .sub { font-size: 10.5px; opacity: .9; margin-top: 3px; }
  .banda .folio { text-align: right; white-space: nowrap; }
  .banda .folio .k { font-size: 8.5px; letter-spacing: .14em; text-transform: uppercase; opacity: .8; }
  .banda .folio .v { font-size: 25px; font-weight: 700; line-height: 1.05; font-variant-numeric: tabular-nums; }
  .oro { height: 3px; background: var(--oro); }

  /* ── Franja de datos del corte ───────────────────────────────────────────────────────────────── */
  .datos { display: flex; gap: 0; border-bottom: 1px solid var(--linea); }
  .datos .d { padding: 7px 0; padding-right: 22px; }
  .datos .k { font-size: 8px; letter-spacing: .12em; text-transform: uppercase; color: var(--gris); }
  .datos .v { font-size: 11px; font-weight: 600; margin-top: 1px; }

  /* ── Resumen: el TOTAL manda. Es el número que se autoriza; los demás son contexto. ──────────── */
  .resumen { display: flex; align-items: stretch; gap: 14px; margin-top: 13px; }
  .cifras { display: flex; gap: 22px; flex: 1; }
  .cifra .k { font-size: 8px; letter-spacing: .12em; text-transform: uppercase; color: var(--gris); }
  .cifra .v { font-size: 17px; font-weight: 700; line-height: 1.2; font-variant-numeric: tabular-nums; }
  .granTotal { border: 1.5px solid var(--verde); border-radius: 5px; padding: 8px 16px; min-width: 42%;
               background: #F4F9F6; }
  .granTotal .k { font-size: 8px; letter-spacing: .12em; text-transform: uppercase; color: var(--verde); }
  .granTotal .v { font-size: 27px; font-weight: 700; line-height: 1.1; color: var(--verde);
                  font-variant-numeric: tabular-nums; letter-spacing: -.02em; }
  .granTotal .letras { font-size: 9px; color: var(--gris); margin-top: 2px; line-height: 1.35; }

  .aviso { border-left: 3px solid #A3321E; background: #FDF4F2; color: #8E2B1A;
           padding: 7px 11px; margin: 12px 0 0; font-size: 10px; }

  h2 { font-size: 9px; margin: 20px 0 0; text-transform: uppercase; letter-spacing: .14em;
       color: var(--gris); border-bottom: 1px solid var(--linea); padding-bottom: 5px; }

  /* ── Un bloque por transferencia ─────────────────────────────────────────────────────────────── */
  .pago { margin-top: 11px; }
  .cab { display: flex; justify-content: space-between; align-items: flex-start; gap: 14px;
         border-left: 3px solid var(--verde); padding: 1px 0 1px 9px;
         break-after: avoid; page-break-after: avoid; }
  .cab .n { display: inline-block; min-width: 15px; color: var(--gris); font-weight: 600; }
  .cab .prov { font-weight: 700; font-size: 11.5px; }
  .cab .ref { margin-left: 8px; color: var(--verde); font-weight: 700; font-size: 10.5px; }
  .cab .iban { font-size: 9px; color: var(--gris); margin-top: 1px; padding-left: 15px; }
  .cab .monto { font-weight: 700; font-size: 14px; white-space: nowrap; font-variant-numeric: tabular-nums; }
  .chip { display: inline-block; margin-left: 7px; padding: 1px 7px; border-radius: 3px;
          background: #E8F2EC; color: var(--verde); font-size: 8.5px; font-weight: 700;
          text-transform: uppercase; letter-spacing: .05em; vertical-align: 1.5px; }
  .chip.mal { background: #FBE7E2; color: #A3321E; }

  table { width: 100%; border-collapse: collapse; font-variant-numeric: tabular-nums; margin-top: 5px; }
  thead { display: table-header-group; }
  th { font-size: 8px; text-transform: uppercase; letter-spacing: .09em; color: var(--gris);
       text-align: left; padding: 4px 8px; border-bottom: 1px solid var(--linea); font-weight: 700; }
  td { padding: 4.5px 8px; border-bottom: 1px solid #F1F5F3; vertical-align: top; }
  tbody tr:nth-child(even) td { background: #FAFCFB; }
  tr.sub td { background: #F4F9F6 !important; border-top: 1px solid var(--linea);
              border-bottom: 0; font-size: 9.5px; color: var(--verde); font-weight: 600; }

  /* ── Firmas: líneas largas y fecha. Un espacio corto se lee como casilla y no se firma. ──────── */
  .firmas { display: flex; gap: 24px; margin-top: 30px; break-inside: avoid; page-break-inside: avoid; }
  .firma { flex: 1; }
  .firma .linea { border-bottom: 1px solid var(--tinta); height: 34px; }
  .firma .rol { font-size: 9.5px; font-weight: 700; margin-top: 5px; }
  .firma .quien { font-size: 9px; color: var(--gris); }
  .firma .fecha { font-size: 8.5px; color: var(--gris); margin-top: 6px; }

  .pie { margin-top: 20px; border-top: 1px solid var(--linea); padding-top: 7px;
         font-size: 8.5px; color: var(--gris); line-height: 1.6; }
  .pie b { color: var(--tinta); }

  .noprint { margin: 20px 0 10px; text-align: center; }
  .noprint button { font: inherit; padding: 10px 22px; border-radius: 6px; cursor: pointer;
                    border: 0; background: var(--verde); color: #fff; font-weight: 700; font-size: 12px; }
  @media print { .noprint { display: none; } }
</style>
</head>
<body>
  <!-- Banda de marca a sangre, con el folio del lote como identificador del documento. El número de
       lote es el folio natural: ya es único, ya se usa al hablar («el corte 32») y ya viaja en la
       referencia de cada transferencia («REM 003201»). Inventarle otro consecutivo sería darle dos
       nombres a la misma cosa. -->
  <div class="banda">
    <div class="fila">
      <div>
        <div class="emp">${esc(empresa)}</div>
        <h1>Relación de pagos a proveedores</h1>
        <div class="sub">${agrupado ? "Corte agrupado por proveedor" : "Corte con una transferencia por factura"}</div>
      </div>
      <div class="folio">
        <div class="k">Lote de pago</div>
        <div class="v">N.º ${r.lote_numero}</div>
      </div>
    </div>
  </div>
  <div class="oro"></div>

  <div class="hoja">
    <div class="datos">
      <div class="d"><div class="k">Fecha de corte</div><div class="v">${formatFecha(r.fecha_corte)}</div></div>
      <div class="d"><div class="k">Moneda</div><div class="v">Colones (CRC)</div></div>
      <div class="d"><div class="k">Emitido</div><div class="v">${esc(impreso)}</div></div>
    </div>

    <div class="resumen">
      <div class="cifras">
        <div class="cifra"><div class="k">Facturas</div><div class="v">${r.facturas}</div></div>
        <div class="cifra"><div class="k">Transferencias</div><div class="v">${r.transferencias}</div></div>
        <div class="cifra"><div class="k">Sin IBAN</div><div class="v${
          r.sin_iban > 0 ? " neg" : ""
        }">${r.sin_iban}</div></div>
      </div>
      <!-- El total, dominante: es LO QUE SE AUTORIZA. Lo demás es contexto, y una caja que les dé a
           todos el mismo peso obliga a buscar el dato que importa. -->
      <div class="granTotal">
        <div class="k">Total a pagar</div>
        <div class="v">${m(r.total_crc)}</div>
        ${letras ? `<div class="letras">${esc(letras.charAt(0).toUpperCase() + letras.slice(1))}</div>` : ""}
      </div>
    </div>

    ${alerta}

    <h2>Detalle por transferencia</h2>
    ${bloques || '<p style="color:#5B6A63">Este corte no tiene ninguna transferencia.</p>'}

    <div class="firmas">
      <div class="firma">
        <div class="linea"></div>
        <div class="rol">Preparado por</div>
        <div class="quien">Tesorería</div>
        <div class="fecha">Fecha: ____ / ____ / ________</div>
      </div>
      <div class="firma">
        <div class="linea"></div>
        <div class="rol">Revisado por</div>
        <div class="quien">Dirección Financiera</div>
        <div class="fecha">Fecha: ____ / ____ / ________</div>
      </div>
      <div class="firma">
        <div class="linea"></div>
        <div class="rol">Autorizado por</div>
        <div class="quien">Gerencia General</div>
        <div class="fecha">Fecha: ____ / ____ / ________</div>
      </div>
    </div>

    <div class="pie">
      <b>GPVDP ERP</b> · Documento generado del registro del sistema en la fecha de emisión indicada
      arriba. Un lote de pago se puede modificar mientras está abierto, de modo que esa fecha y hora
      son parte del documento. Los códigos internos —<b>CXP-…</b> para un pago suelto,
      <b>CXPR-…</b> para uno agrupado— viajan en el detalle de cada transferencia y permiten cruzarla
      contra el estado de cuenta; el banco los devuelve sin el guion.
    </div>

    <div class="noprint">
      <button onclick="window.print()">Imprimir o guardar como PDF</button>
    </div>
  </div>
</body>
</html>`);
  ventana.document.close();
  return true;
}
