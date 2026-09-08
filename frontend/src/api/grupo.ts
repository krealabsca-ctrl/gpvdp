/**
 * Cliente de la vista consolidada del grupo.
 *
 * Es la ÚNICA superficie del ERP que cruza empresas, y solo lee. El alcance no se pide: lo resuelve
 * el servidor con las membresías del usuario, así que acá no hay —ni puede haber— un parámetro de
 * empresas. Lo único que viaja es el período.
 *
 * Los montos son strings decimales (nunca number: un total en colones no admite el redondeo del
 * punto flotante).
 */

import { apiFetch } from "@/api/client";

/** Naturaleza declarada en el concepto. Es lo que decide si algo es ingreso o gasto, no el signo. */
export type NaturalezaGrupo = "INGRESO" | "GASTO" | "NEUTRO" | "SIN_CLASIFICAR";

/** Aporte de una empresa al consolidado. */
export interface FilaEmpresaGrupo {
  empresa_id: string;
  empresa: string;
  ingresos_crc: string;
  gastos_crc: string;
  ebitda_crc: string;
  /** Traslados, ahorro y reservas: no son ingreso ni gasto, y en este grupo es el número mayor. */
  neutro_crc: string;
  movimientos: number;
  sin_clasificar: number;
  sin_clasificar_crc: string;
  /** Clasificado por MONTO (no por cantidad de movimientos). */
  pct_clasificado: string;
  /** Llega al 90 %, que es el umbral con que el proyecto da un número por bueno. */
  confiable: boolean;
  /**
   * Ni un movimiento en el período. Es distinto de «todo clasificado»: el porcentaje de un conjunto
   * vacío da 100 %, así que sin esta bandera un mes en el que nadie cargó nada se pinta de verde.
   */
  sin_datos: boolean;
}

/** Movimiento cuya partida nombra a otra empresa del grupo. Se informa, no se elimina del total. */
export interface OperacionInternaGrupo {
  empresa_id: string;
  empresa: string;
  contraparte: string;
  partida: string;
  naturaleza: NaturalezaGrupo;
  movimientos: number;
  monto_crc: string;
  /** Solo lo que la naturaleza cuenta como ingreso o gasto distorsiona el resultado. */
  afecta_ebitda: boolean;
}

export interface AporteEmpresaGrupo {
  empresa: string;
  monto_crc: string;
}

/** Partida sumada en todas las empresas visibles, con el desglose de quién la genera. */
export interface PartidaGrupo {
  partida: string;
  concepto: string;
  naturaleza: NaturalezaGrupo;
  monto_crc: string;
  movimientos: number;
  por_empresa: AporteEmpresaGrupo[];
}

export interface ResumenGrupo {
  periodo: string;
  empresas: FilaEmpresaGrupo[];
  ingresos_crc: string;
  gastos_crc: string;
  ebitda_crc: string;
  neutro_crc: string;
  movimientos: number;
  sin_clasificar_crc: string;
  entre_empresas_crc: string;
  entre_empresas_ebitda_crc: string;
  entre_empresas: OperacionInternaGrupo[];
  partidas: PartidaGrupo[];
  /** Cuántas empresas hay en el sistema: es el denominador que permite confesar las excluidas. */
  empresas_del_sistema: number;
  excluidas: string[];
  /**
   * Una frase que explica por qué el número puede no ser el que se espera. "" si no hay nada.
   *
   * No trae montos a propósito: los formatea el cliente, con las mismas reglas que el resto del ERP.
   */
  aviso: string;
  /** Todas las empresas del sistema incluidas y todas al 90 %. */
  completo: boolean;
}

export const grupoApi = {
  /** GET /v1/grupo/resumen — el consolidado del período. `periodo` en formato AAAA-MM. */
  resumen(periodo: string): Promise<ResumenGrupo> {
    return apiFetch<ResumenGrupo>(`/grupo/resumen?periodo=${encodeURIComponent(periodo)}`, {
      method: "GET",
    });
  },
};
