/**
 * Adivinar las columnas de un estado de cuenta a partir de su fila de encabezado.
 *
 * POR QUÉ EXISTE. Configurar un formato es decir «el débito viene en la columna 4». La columna 4
 * de un Excel es la E, y quien mira el archivo cuenta desde 1. Pedirle eso a alguien es garantizar
 * que tarde o temprano se configure un formato con el débito y el crédito cambiados —y ese
 * archivo se lee entero al revés, sin que nadie lo note hasta que el cuadre del mes no cierre—.
 *
 * Esto es AYUDA, no una regla: propone, la persona corrige, y nada se guarda sin pasar por
 * «Probar», que muestra los movimientos ya leídos. Por eso vive en el cliente y no en el servidor:
 * no decide nada, solo ahorra la parte que se equivoca.
 *
 * Los sinónimos salen de los seis bancos reales (ver backend/internal/bancos/specs.go) más las
 * variantes de contabilidad que aparecen en los demás: «debe/haber» en vez de «débito/crédito».
 */

/** Los papeles que una columna puede tener. */
export type PapelColumna = "fecha" | "documento" | "descripcion" | "debito" | "credito";

/**
 * Sinónimos por papel, en orden de especificidad.
 *
 * EL ORDEN IMPORTA Y NO ES COSMÉTICO: «debito» aparece antes que «debe» porque un encabezado que
 * diga «Débitos» contiene las dos, y «fecha contable» antes que «fecha» por la misma razón. La
 * primera coincidencia gana.
 */
const SINONIMOS: Record<PapelColumna, string[]> = {
  fecha: ["fecha mov", "fecha contable", "fecha y hora", "fechamovimiento", "fecha"],
  documento: ["numero documento", "numerodocumento", "comprobante", "referencia", "documento"],
  descripcion: ["descripcion", "concepto", "detalle", "glosa"],
  // «debitos (dr)» de Davivienda entra por «debito»; «debe» es la variante contable.
  debito: ["debitos", "debito", "cargos", "debe", "salidas"],
  credito: ["creditos", "credito", "abonos", "haber", "entradas"],
};

/** Normaliza igual que el backend: minúsculas, sin acentos, sin espacios extremos. */
export function normalizar(s: string): string {
  return s
    .toLowerCase()
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .trim();
}

/**
 * Adivina qué columna cumple cada papel en una fila de encabezado.
 *
 * Devuelve -1 para el papel que no encuentra. El -1 es correcto para el documento (hay bancos que
 * no publican ninguna referencia) y es lo que la pantalla marca en rojo para los otros cuatro.
 */
export function adivinarColumnas(encabezado: string[]): Record<PapelColumna, number> {
  const celdas = encabezado.map(normalizar);
  const resultado: Record<PapelColumna, number> = {
    fecha: -1, documento: -1, descripcion: -1, debito: -1, credito: -1,
  };
  const tomadas = new Set<number>();

  // Dos vueltas por papel y no una por celda: así el sinónimo MÁS específico de cada papel se
  // busca en todo el encabezado antes de probar el siguiente. Recorriendo celdas, un «Fecha» en
  // la columna 0 le ganaría a «Fecha contable» en la 2 solo por venir antes.
  for (const papel of Object.keys(SINONIMOS) as PapelColumna[]) {
    for (const aguja of SINONIMOS[papel]) {
      const i = celdas.findIndex((c, idx) => !tomadas.has(idx) && c.includes(aguja));
      if (i >= 0) {
        resultado[papel] = i;
        tomadas.add(i);
        break;
      }
    }
  }
  return resultado;
}

/**
 * Propone las señales que identifican esa fila como el encabezado.
 *
 * Toma las columnas de fecha y débito cuando se reconocieron: son las dos que distinguen un
 * estado de cuenta de cualquier otra tabla, y son dos —el mínimo que el servidor exige para que
 * un formato no reclame los archivos de otros bancos—.
 *
 * Usa la PALABRA del encabezado, no la celda entera: «Débitos (DR)» se compara por contenido, y
 * guardar el texto completo haría que el formato dejara de calzar el día que el banco le agregue
 * un espacio.
 */
export function proponerSenales(
  encabezado: string[],
  cols: Record<PapelColumna, number>,
): { columna: number; texto: string }[] {
  const out: { columna: number; texto: string }[] = [];
  for (const papel of ["fecha", "debito", "credito", "descripcion"] as PapelColumna[]) {
    const i = cols[papel];
    if (i < 0 || !encabezado[i]) continue;
    out.push({ columna: i, texto: primeraPalabra(encabezado[i]) });
    if (out.length === 2) break;
  }
  return out;
}

/** La primera palabra significativa de una celda de encabezado, normalizada. */
function primeraPalabra(celda: string): string {
  const n = normalizar(celda);
  // Se corta en el primer paréntesis o barra: «Débitos (DR)» → «debitos». Lo que viene después
  // suele ser una aclaración del banco y cambia más seguido que el nombre.
  const limpio = (n.split(/[([/]/)[0] ?? "").trim();
  return limpio || n;
}

/**
 * La fila que MÁS parece un encabezado, entre las primeras del archivo.
 *
 * Se propone sola para que la persona no tenga que buscarla: en los seis bancos reales está entre
 * la fila 1 y la 7, precedida de títulos, el nombre del titular y el rango de fechas.
 */
export function filaDeEncabezadoProbable(filas: string[][]): number {
  let mejor = -1;
  let mejorPuntaje = 0;
  filas.forEach((fila, i) => {
    const cols = adivinarColumnas(fila);
    // Cuántos papeles se reconocieron. Un encabezado de verdad trae al menos fecha, débito y
    // crédito; una fila de datos no trae ninguno.
    const puntaje = (Object.keys(cols) as PapelColumna[]).filter((p) => cols[p] >= 0).length;
    if (puntaje > mejorPuntaje) {
      mejorPuntaje = puntaje;
      mejor = i;
    }
  });
  // Menos de tres papeles no es un encabezado: es una fila cualquiera con la palabra «fecha».
  return mejorPuntaje >= 3 ? mejor : -1;
}
