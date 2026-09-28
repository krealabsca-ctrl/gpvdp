/**
 * El rango «desde–hasta» revisado ANTES de consultar (21-set-2026).
 *
 * El Director Financiero reportó «seleccioné una fecha y dio error». El error salía del servidor:
 * el `<input type="date">` de Chrome dispara onChange en CADA tecla del año y rellena con ceros a
 * la izquierda, así que tecleando «2026» la pantalla llegó a pedir `desde=0002-09-01` y
 * `desde=0202-09-01`. El primero reventaba en Postgres (500 «error interno»); el segundo salía 200
 * y devolvía TODO el histórico, que es peor: el usuario cree que filtró y está mirando otra cosa.
 *
 * El servidor rechaza lo que no es una fecha ISO real y, con el mismo piso que acá (año de cuatro
 * cifras que no empieza en cero: `anioMinimo` en handler_clasif.go), también los años a medio
 * teclear — así el endpoint no queda expuesto por URL ni desde Clasificar, que comparte el
 * validador pero no esta función. Esta función es la otra mitad: que una
 * fecha a medio escribir NI SIQUIERA SALGA de la pantalla, y que lo que está mal se diga pegado al
 * campo en vez de reemplazar la pantalla por «error interno».
 *
 * Es una función pura y vive aparte de la página para poder probarla sin navegador.
 */

/** Lo que la pantalla debe hacer con lo que el usuario escribió en los dos campos. */
export interface RangoRevisado {
  /** El valor a mandar a la API, o `undefined` si no hay que mandarlo. */
  desde?: string;
  hasta?: string;
  /** Mensaje para pegar al campo «Desde» («" "» si está bien). */
  errorDesde: string;
  /** Mensaje para pegar al campo «Hasta». */
  errorHasta: string;
  /** true si algo está mal: con esto puesto NO se consulta con el rango. */
  hayError: boolean;
}

/**
 * Una fecha ISO que existe de verdad en el calendario.
 *
 * Las tres comprobaciones son tres defectos distintos:
 *   - la forma exacta aaaa-mm-dd descarta «01/09/2026» y «2026-09», que Postgres interpreta a su
 *     manera (con DateStyle=MDY, «01/09/2026» es el 9 de enero) y dejan el filtro midiendo otra cosa;
 *   - el año de cuatro dígitos que no empieza en cero descarta «0002-09-01» y «0202-09-01», que es
 *     lo que el navegador emite mientras se teclea el año;
 *   - el ida y vuelta por Date descarta el día que no existe («2026-02-31», «2026-13-01»).
 */
export function fechaISOValida(v: string): boolean {
  const m = /^([1-9]\d{3})-(\d{2})-(\d{2})$/.exec(v);
  if (!m) return false;
  const anio = Number(m[1]);
  const mes = Number(m[2]);
  const dia = Number(m[3]);
  if (mes < 1 || mes > 12 || dia < 1 || dia > 31) return false;
  const d = new Date(Date.UTC(anio, mes - 1, dia));
  return d.getUTCFullYear() === anio && d.getUTCMonth() === mes - 1 && d.getUTCDate() === dia;
}

/**
 * Revisa los dos campos juntos.
 *
 * Si algo está mal NO se manda NINGUNA de las dos fechas: aplicar media fecha devolvería una lista
 * que no es la que el usuario pidió, y en esta pantalla una lista es una afirmación sobre el dinero
 * que entró. Se muestra el mes activo y el mensaje dice qué corregir.
 */
export function revisarRango(desde: string, hasta: string): RangoRevisado {
  const errorDesde = motivo(desde);
  const errorHasta = motivo(hasta);

  // Rango invertido: los dos campos están bien escritos y aun así la consulta no puede devolver
  // nada. El servidor responde 400; acá se dice antes de salir, que es lo que pidió el Director.
  const invertido =
    !errorDesde && !errorHasta && desde !== "" && hasta !== "" && desde > hasta
      ? "«Hasta» es anterior a «Desde». Corregí una de las dos."
      : "";

  const hayError = Boolean(errorDesde || errorHasta || invertido);
  return {
    desde: hayError || desde === "" ? undefined : desde,
    hasta: hayError || hasta === "" ? undefined : hasta,
    errorDesde,
    errorHasta: errorHasta || invertido,
    hayError,
  };
}

function motivo(v: string): string {
  if (v === "") return "";
  if (fechaISOValida(v)) return "";
  if (/^0\d{3}-/.test(v)) return "El año quedó a medio escribir.";
  return "Fecha incompleta o inexistente (aaaa-mm-dd).";
}
