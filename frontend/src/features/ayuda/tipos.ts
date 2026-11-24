/**
 * El manual, como datos.
 *
 * EL PEDIDO (punto 3 de la revisión comercial): «No hay documentación para el usuario final. Lo
 * que existe es técnico, para quien instala o programa. No hay un manual de "cómo cierro el mes".
 * Eso es capacitación que hoy das vos en persona, y es lo que no escala».
 *
 * POR QUÉ ES TS Y NO UN ARCHIVO MARKDOWN. Dos razones. La primera es que el proyecto no puede
 * instalar dependencias nuevas, y renderizar markdown necesita una; escribir un parser propio
 * sería mantener un intérprete de texto para mostrar un manual. La segunda, la que importa: como
 * dato, cada guía puede DECLARAR a qué pantallas corresponde, y entonces la ayuda aparece sola en
 * el lugar donde alguien tiene la duda. Un manual que hay que ir a buscar se lee una vez.
 *
 * Y porque así hay UNA sola fuente. El `.md` de antes era un segundo texto que mantener, y el
 * segundo texto siempre es el que queda viejo.
 */

/** Un bloque de contenido. Son los que el manual necesita de verdad, no un lenguaje de marcado. */
export type Bloque =
  | { tipo: "texto"; texto: string }
  /** Lo que conviene saber ANTES de que pase. Se dibuja destacado. */
  | { tipo: "aviso"; texto: string }
  | { tipo: "lista"; items: string[] }
  /** Pasos numerados, cada uno con la pantalla donde se hace. */
  | { tipo: "pasos"; items: { titulo: string; donde?: string; texto: string }[] }
  | { tipo: "tabla"; encabezados: string[]; filas: string[][] }
  /** Las fases de un flujo: Recibidas → Por validar → … */
  | { tipo: "flujo"; etapas: string[] }
  /** Una resta con alineación, como el neto de una factura. */
  | { tipo: "formula"; lineas: string[] };

/** Una guía: algo que alguien quiere HACER, no un módulo del sistema. */
export interface Guia {
  id: string;
  titulo: string;
  /** Una frase. Es lo que se lee en la lista y lo que el buscador muestra. */
  resumen: string;
  bloques: Bloque[];
  /**
   * Las pantallas que esta guía cubre.
   *
   * Es lo que hace que la ayuda aparezca sola donde se trabaja. Las rutas se comprueban contra el
   * router en una prueba: una ruta que se renombre dejaría la ayuda invisible en silencio, que es
   * el modo de falla conocido de este proyecto.
   */
  rutas: string[];
}

/**
 * La ayuda de UNA pantalla. Contesta «¿qué hago acá?», que es otra pregunta que «¿cómo cierro el
 * mes?»: la primera se hace parado frente a la pantalla y la segunda antes de empezar.
 */
export interface AyudaDePantalla {
  ruta: string;
  /** Qué es esta pantalla, en una frase. */
  queEs: string;
  /** Lo que hay que saber. Dos a cuatro: más que eso no se lee. */
  puntos: string[];
  /** La guía completa, si la hay. */
  guia?: string;
}
