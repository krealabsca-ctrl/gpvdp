/**
 * Cliente de los errores del sistema (mig 0107).
 *
 * Lo que esta pantalla hace es un CANJE: el usuario dicta por teléfono los seis caracteres que vio
 * en el mensaje de error, y acá se cambian por la causa exacta. Antes esa referencia solo existía
 * en el log del contenedor, así que era un código que no se podía canjear por nada.
 *
 * La empresa no viaja: la pone el servidor desde el token. Una pantalla que muestra rutas
 * internas, nombres de tablas y fragmentos de SQL no puede tener un parámetro de empresa.
 */

import { apiFetch } from "@/api/client";

/** Un error interno tal como quedó registrado. */
export interface Incidente {
  id: string;
  /** Los seis caracteres que el usuario tiene en pantalla. */
  referencia: string;
  /** Nulo cuando el error ocurrió antes de elegir empresa (login, refresh). */
  empresa_id: string | null;
  usuario_id: string | null;
  /** Nombre de quien lo sufrió, para poder volver a preguntarle qué estaba haciendo. */
  usuario: string;
  metodo: string;
  /** El PATRÓN de la ruta («/v1/cxp/documentos/:id»), no la URL con el identificador adentro. */
  ruta: string;
  estado: number;
  /** La operación que se le pasó a AbortInterno («cxp actualizar-proveedor»). */
  operacion: string;
  detalle: string;
  /** El servidor se cayó en medio de la respuesta. Antes esto daba 500 sin cuerpo. */
  panico: boolean;
  traza: string;
  creado_en: string;
}

/** Lo que se ve de un vistazo, antes de abrir nada. */
export interface ResumenIncidentes {
  ultima_hora: number;
  ultimo_dia: number;
  ultima_semana: number;
  panicos: number;
  /** Contesta la única pregunta que importa al entrar: ¿está pasando AHORA? */
  ultimo: string | null;
}

// --- Consultas lentas (mig 0120) ---

/**
 * Una consulta con lo que se lleva del servidor.
 *
 * `consulta` es el texto NORMALIZADO por PostgreSQL: los valores ya vienen reemplazados por $1,
 * $2, así que acá no hay montos, cédulas ni nombres de nadie — solo la forma de la consulta.
 */
export interface ConsultaLenta {
  consulta: string;
  llamadas: number;
  /** El criterio de orden: la SUMA de todo lo que esta consulta se llevó. */
  ms_total: number;
  ms_media: number;
  ms_max: number;
  /** Cuánto pesa contra todo lo que el motor ejecutó. */
  porcentaje_del_total: number;
  /** Delata los escaneos completos: 1 fila devuelta tras leer 200.000 no se ve en los tiempos. */
  filas_por_llamada: number;
}

export interface RendimientoConsultas {
  /** false = el motor arrancó sin el contador. NO es un error; `motivo` dice cómo encenderlo. */
  disponible: boolean;
  motivo: string;
  consultas: ConsultaLenta[];
  /** Desde cuándo se acumulan estos números. Sin esto, un total de 40 s no se puede interpretar. */
  midiendo_desde: string;
}

export interface FiltrosIncidentes {
  ref?: string;
  desde?: string;
  hasta?: string;
  panico?: boolean;
  limite?: number;
}

export const sistemaApi = {
  /** GET /v1/sistema/incidentes — los errores que esta empresa puede ver. */
  incidentes(f: FiltrosIncidentes = {}): Promise<{ items: Incidente[] }> {
    return apiFetch<{ items: Incidente[] }>("/sistema/incidentes", {
      method: "GET",
      query: {
        ref: f.ref || undefined,
        desde: f.desde || undefined,
        hasta: f.hasta || undefined,
        panico: f.panico ? "1" : undefined,
        limite: f.limite ? String(f.limite) : undefined,
      },
    });
  },
  /** GET /v1/sistema/incidentes/resumen — cuántos y cuándo fue el último. */
  resumen(): Promise<ResumenIncidentes> {
    return apiFetch<ResumenIncidentes>("/sistema/incidentes/resumen", { method: "GET" });
  },

  // --- Consultas lentas (mig 0120) ---

  /**
   * GET /v1/sistema/rendimiento/consultas — qué se lleva el tiempo del servidor.
   *
   * `disponible: false` NO es un error: es un servidor cuyo motor arrancó sin el contador. Viene
   * con el motivo y la pantalla lo muestra en vez de una lista vacía.
   */
  consultasLentas(limite?: number): Promise<RendimientoConsultas> {
    return apiFetch<RendimientoConsultas>("/sistema/rendimiento/consultas", {
      method: "GET",
      query: { limite: limite ? String(limite) : undefined },
    });
  },
  /** POST /v1/sistema/rendimiento/reiniciar — pone la medición en cero. */
  reiniciarMedicion(): Promise<{ ok: boolean }> {
    return apiFetch<{ ok: boolean }>("/sistema/rendimiento/reiniciar", { method: "POST" });
  },
};
