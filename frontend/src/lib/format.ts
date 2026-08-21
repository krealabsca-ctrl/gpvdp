/**
 * Formateadores de dominio (montos, fechas, períodos) para el módulo Bancos.
 *
 * Los montos del backend llegan como STRING decimal (nunca float, ver CLAUDE.md).
 * Aquí se convierten a número SOLO para presentación; no se hace aritmética de
 * negocio en el cliente. Para alinear columnas usar siempre `tabular-nums`.
 */

const CRC = new Intl.NumberFormat("es-CR", {
  style: "currency",
  currency: "CRC",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

const USD = new Intl.NumberFormat("es-CR", {
  style: "currency",
  currency: "USD",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

const PLAIN = new Intl.NumberFormat("es-CR", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

export type Moneda = "CRC" | "USD";

/** Convierte un decimal-string del backend a número; "" / null -> 0. */
export function toNumber(value: string | null | undefined): number {
  if (value === null || value === undefined || value === "") return 0;
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

/** Formatea un monto (string decimal) con símbolo de moneda. */
export function formatMoneda(value: string | null | undefined, moneda: Moneda = "CRC"): string {
  const n = toNumber(value);
  return moneda === "USD" ? USD.format(n) : CRC.format(n);
}

/** Formatea un monto sin símbolo (para columnas de tabla con cabecera de moneda). */
export function formatMonto(value: string | null | undefined): string {
  return PLAIN.format(toNumber(value));
}

/**
 * Montos que ESCRIBE una persona (campos de formulario). Se trabaja en céntimos enteros para
 * que no aparezca el arrastre del punto flotante, y se separa el valor legible (con separador
 * de miles) del valor que viaja a la API (decimal plano, como espera el backend).
 */

/**
 * Parsea un monto tecleado a céntimos. Tolera los formatos que una persona escribe de verdad:
 * "1 234,56" (es-CR, incluido el espacio duro que produce Intl), "1.234,56", "1234.56", "450000".
 * El último separador cuenta como decimal SOLO si le siguen 1 o 2 dígitos; si no, es de miles
 * (así "480.000" son cuatrocientos ochenta mil, no 480).
 */
export function montoACentimos(texto: string | null | undefined): number {
  const limpio = String(texto ?? "").replace(/[^\d.,]/g, "");
  if (!limpio) return 0;
  const sep = Math.max(limpio.lastIndexOf(","), limpio.lastIndexOf("."));
  let entero = limpio;
  let decimales = "";
  if (sep >= 0) {
    const cola = limpio.slice(sep + 1);
    if (/^\d{1,2}$/.test(cola)) {
      entero = limpio.slice(0, sep);
      decimales = cola;
    }
  }
  entero = entero.replace(/[.,]/g, "");
  const n = Number(`${entero || "0"}.${(decimales || "0").padEnd(2, "0")}`);
  return Number.isFinite(n) ? Math.round(n * 100) : 0;
}

/** Céntimos → decimal plano para la API ("39823009" → "398230.09"). Nunca localizado. */
export function centimosAPlano(centimos: number): string {
  return (centimos / 100).toFixed(2);
}

/** Céntimos → texto legible para mostrar en el campo ("39823009" → "398 230,09"). */
export function centimosALegible(centimos: number): string {
  return PLAIN.format(centimos / 100);
}

/** Normaliza lo que escribió la persona al decimal plano que espera la API ("" → ""). */
export function montoParaApi(texto: string | null | undefined): string {
  const t = String(texto ?? "").trim();
  return t === "" ? "" : centimosAPlano(montoACentimos(t));
}

/** Reformatea lo que escribió la persona a texto legible; deja vacío si estaba vacío. */
export function montoLegible(texto: string | null | undefined): string {
  const t = String(texto ?? "").trim();
  return t === "" ? "" : centimosALegible(montoACentimos(t));
}

/** Formatea una fecha ISO (YYYY-MM-DD) a formato local corto, sin zona horaria. */
export function formatFecha(iso: string | null | undefined): string {
  if (!iso) return "—";
  // Parseo manual para evitar corrimientos por zona horaria (YYYY-MM-DD).
  const [y, m, d] = iso.split("T")[0]!.split("-");
  if (!y || !m || !d) return iso;
  return `${d}/${m}/${y}`;
}

/** Formatea un porcentaje (0.87 -> "87%"). Acepta string o número. */
export function formatPct(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === "") return "—";
  const n = typeof value === "string" ? Number(value) : value;
  if (!Number.isFinite(n)) return "—";
  // Confianza suele venir en 0..1; si viene 0..100 se respeta el número tal cual > 1.
  const pct = n <= 1 ? n * 100 : n;
  return `${Math.round(pct)}%`;
}

/**
 * Día de operación de Costa Rica en formato YYYY-MM-DD.
 *
 * El negocio opera en CR (UTC−6, sin horario de verano), así que el día hábil NO es el del
 * navegador ni el UTC: después de las 18:00 CR el UTC ya cambió de día y un vencimiento se
 * leería con un día de más. El backend usa la misma referencia
 * (`(now() AT TIME ZONE 'America/Costa_Rica')::date`), y así las dos capas coinciden.
 */
export function hoyCR(): string {
  return new Date(Date.now() - 6 * 60 * 60 * 1000).toISOString().slice(0, 10);
}

/** Período actual en formato YYYY-MM (mes en curso). */
export function periodoActual(): string {
  const now = new Date();
  const y = now.getFullYear();
  const m = String(now.getMonth() + 1).padStart(2, "0");
  return `${y}-${m}`;
}

/** Descompone "YYYY-MM" en { anio, mes } numéricos. */
export function partesPeriodo(periodo: string): { anio: number; mes: number } {
  const [y, m] = periodo.split("-");
  return { anio: Number(y), mes: Number(m) };
}

/** Compone { anio, mes } a "YYYY-MM". */
export function componerPeriodo(anio: number, mes: number): string {
  return `${anio}-${String(mes).padStart(2, "0")}`;
}

/** Genera una lista de períodos YYYY-MM hacia atrás desde el actual (para selectores). */
export function periodosRecientes(cantidad = 18): string[] {
  const out: string[] = [];
  const now = new Date();
  for (let i = 0; i < cantidad; i++) {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
    out.push(componerPeriodo(d.getFullYear(), d.getMonth() + 1));
  }
  return out;
}

/** Etiqueta legible de un período: "2026-07" -> "Julio 2026". */
const MESES = [
  "Enero",
  "Febrero",
  "Marzo",
  "Abril",
  "Mayo",
  "Junio",
  "Julio",
  "Agosto",
  "Septiembre",
  "Octubre",
  "Noviembre",
  "Diciembre",
];
export function etiquetaPeriodo(periodo: string): string {
  const { anio, mes } = partesPeriodo(periodo);
  const nombre = MESES[mes - 1] ?? periodo;
  return `${nombre} ${anio}`;
}

/**
 * Normaliza texto para buscar y comparar: minúsculas y sin tildes ("Depósito" ≡ "deposito").
 * Lo usan los filtros del catálogo y de la bandeja: nadie escribe las tildes al buscar.
 */
export function sinTildes(s: string): string {
  return s.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "");
}
