/**
 * Cliente de los formatos de banco configurables (mig 0108).
 *
 * Enseñarle al sistema a leer el estado de cuenta de un banco que no viene de fábrica. Los seis
 * que ya existen —BN, BCR, BAC, Davivienda, Promerica y Banco Popular— viven en el código y no se
 * tocan: lo que se configura acá se prueba ANTES que ellos y, si no calza, ellos siguen ahí.
 */

import { apiFetch } from "@/api/client";

/** Una condición sobre la fila de encabezado: «la columna N contiene este texto». */
export interface SenalEncabezado {
  columna: number;
  texto: string;
}

export interface FormatoBanco {
  id: string;
  nombre: string;
  activo: boolean;
  /** Texto que tiene que aparecer en alguna parte de las primeras filas. Opcional. */
  texto_identificador: string;
  col_fecha: number;
  /** -1 cuando el banco no publica ninguna referencia. */
  col_documento: number;
  col_descripcion: number;
  col_debito: number;
  col_credito: number;
  /** Clave de la lista cerrada que devuelve el servidor (nunca un layout de Go). */
  formato_fecha: string;
  /** Todas tienen que cumplirse: se unen con Y. */
  senales: SenalEncabezado[];
  creado_en?: string;
}

export interface OpcionFormatoFecha {
  clave: string;
  ejemplo: string;
}

export interface ListaFormatos {
  items: FormatoBanco[];
  formatos_fecha: OpcionFormatoFecha[];
  /** Los seis de fábrica. Se muestran para que nadie configure uno que ya existe. */
  bancos_del_codigo: string[];
}

export interface MovimientoDeMuestra {
  fecha: string;
  documento: string;
  descripcion: string;
  debito: string;
  credito: string;
}

/**
 * Una señal que no se cumple, con lo que de verdad hay en esa columna de los títulos.
 *
 * Las señales se unen con Y: una sola que no calce tumba el formato entero. Esto es lo que
 * convierte «no se reconoció el archivo» en algo que se corrige en diez segundos.
 */
export interface SenalQueNoCalza {
  columna: number;
  pedido: string;
  encontrado: string;
  /** Contra qué fila se comparó. Sin esto, «en los títulos dice X» no se puede verificar. */
  fila_titulos: number;
}

/** El resultado de correr un formato contra un archivo, sin guardar nada. */
export interface PruebaDeFormato {
  /** false con `filas` llenas es la respuesta útil: «no lo reconocí, mirá lo que hay». */
  detectado: boolean;
  movimientos: number;
  iban: string;
  moneda: string;
  muestra: MovimientoDeMuestra[];
  /** Las primeras filas del archivo, para contar las columnas contra algo real. */
  filas: string[][];
  /** Se reconoció pero no se pudo leer (fechas ilegibles, montos rotos). */
  problema: string;
  /** Cuando NO se detectó: qué señales no se cumplen. Vacío si el problema es otro. */
  senales_que_fallan: SenalQueNoCalza[];
}

export const formatosBancoApi = {
  listar(): Promise<ListaFormatos> {
    return apiFetch<ListaFormatos>("/bancos/formatos", { method: "GET" });
  },
  guardar(f: Omit<FormatoBanco, "id"> & { id?: string }): Promise<FormatoBanco> {
    return apiFetch<FormatoBanco>("/bancos/formatos", { method: "POST", json: f });
  },
  borrar(id: string): Promise<void> {
    return apiFetch<void>(`/bancos/formatos/${id}`, { method: "DELETE" });
  },
  /** Corre el formato contra un archivo de verdad. NO guarda nada. */
  probar(f: Partial<FormatoBanco>, archivo: File): Promise<PruebaDeFormato> {
    const fd = new FormData();
    fd.append("formato", JSON.stringify(f));
    fd.append("archivo", archivo);
    return apiFetch<PruebaDeFormato>("/bancos/formatos/probar", { method: "POST", raw: fd });
  },
};
