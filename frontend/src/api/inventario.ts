/**
 * Cliente del módulo de Inventario.
 *
 * Los montos viajan como strings decimales (nunca number: un costo en colones no admite errores de
 * redondeo del punto flotante) y las existencias como enteros, porque no hay medio cofre.
 */

import { apiFetch } from "@/api/client";

/** UNIDAD: cada objeto físico es una ficha. CANTIDAD: solo se cuenta. */
export type ModoControl = "UNIDAD" | "CANTIDAD";

/** Estados de una unidad física. EN_TRANSITO = salió de una sede y nadie la recibió todavía. */
export type EstadoUnidad =
  | "DISPONIBLE"
  | "RESERVADA"
  | "EN_TRANSITO"
  | "EXHIBICION"
  | "USADA"
  | "DANADA"
  | "DEVUELTA"
  | "NO_APARECIO";

/** Semáforo de existencia. SIN_NIVEL = nadie fijó el mínimo, así que no hay nada que comparar. */
export type NivelExistencia =
  | "SIN_NIVEL"
  | "EN_RANGO"
  | "CERCA_DEL_MINIMO"
  | "BAJO_MINIMO"
  | "SOBRE_MAXIMO";

export type LecturaRotacion = "SANO" | "LENTO" | "DETENIDO" | "SIN_SALIDAS";

export interface CategoriaInv {
  id: string;
  padre_id: string;
  padre: string;
  nombre: string;
  activo: boolean;
  articulos: number;
}

export interface ArticuloInv {
  id: string;
  codigo: string;
  nombre: string;
  categoria_id: string;
  categoria: string;
  modo_control: ModoControl;
  unidad_medida: string;
  proveedor_id: string;
  proveedor: string;
  clasificacion_id: string;
  clasificacion: string;
  activo: boolean;
  nota: string;
}

export interface ExistenciaSede {
  articulo_id: string;
  codigo: string;
  articulo: string;
  categoria: string;
  modo_control: ModoControl;
  sede_id: string;
  sede: string;
  cantidad: number;
  minimo: number;
  maximo: number;
  estado: NivelExistencia;
  valor_crc: string;
  costo_unitario_crc: string;
  /** De las unidades presentes, cuántas son del proveedor y no capital propio. */
  consignadas: number;
  /** Cuánto de valor_crc es del proveedor. Calculado con el costo real de cada unidad. */
  valor_consignado_crc: string;
}

export interface ResumenExistencias {
  filas: ExistenciaSede[];
  /** Se cuentan aparte: sumar cofres con urnas daría un número sin sentido. */
  unidades_totales: number;
  cantidades_totales: number;
  /** TODO lo que hay físicamente en bodega: propio y consignado. */
  valor_total_crc: string;
  /** La plata que la empresa tiene invertida: el total menos lo consignado. */
  valor_propio_crc: string;
  /** Lo que está en bodega pero es del proveedor. */
  consignadas_crc: string;
  unidades_consignadas: number;
  bajo_minimo: number;
  sedes_con_stock: number;
  /** Por qué el número puede no ser confiable. Vacío = está bien. */
  aviso: string;
}

export interface UnidadInv {
  id: string;
  numero: string;
  articulo_id: string;
  articulo: string;
  categoria: string;
  sede_id: string;
  sede: string;
  sede_destino_id: string;
  sede_destino: string;
  estado: EstadoUnidad;
  estado_legible: string;
  costo_crc: string;
  es_consignada: boolean;
  proveedor_id: string;
  proveedor: string;
  ingresada_en: string;
  servicio_numero: string;
  /** Días desde su último movimiento: es lo que delata el capital detenido. */
  dias_quieta: number;
}

export interface MovimientoInv {
  id: string;
  fecha: string;
  tipo: string;
  tipo_legible: string;
  articulo_id: string;
  articulo: string;
  unidad_id: string;
  unidad_numero: string;
  /** Siempre positiva; `signo` dice si sumó o restó. */
  cantidad: number;
  signo: number;
  sede: string;
  sede_contra: string;
  costo_unitario_crc: string;
  servicio: string;
  proveedor: string;
  motivo: string;
  usuario: string;
}

export interface ConsumoLinea {
  articulo_id: string;
  articulo: string;
  unidad_numero: string;
  cantidad: number;
  costo_crc: string;
  /** Lo usado era del proveedor: va a generar una cuenta por pagar. */
  es_consignada: boolean;
}

export interface ServicioInv {
  id: string;
  numero: string;
  sede_id: string;
  sede: string;
  fecha: string;
  a_nombre_de: string;
  nota: string;
  /** Lo que costó el producto que consumió: la base del margen del servicio. */
  costo_producto_crc: string;
  consumos: ConsumoLinea[];
}

export interface TrasladoLinea {
  articulo_id: string;
  articulo: string;
  unidad_id: string;
  unidad_numero: string;
  cantidad: number;
}

export interface TrasladoInv {
  id: string;
  numero: string;
  sede_origen_id: string;
  sede_origen: string;
  sede_destino_id: string;
  sede_destino: string;
  estado: "EN_TRANSITO" | "RECIBIDO" | "CANCELADO";
  enviado_en: string;
  recibido_en: string;
  enviado_por: string;
  recibido_por: string;
  nota: string;
  /** Lo que lleva sin recibirse: el número que delata lo extraviado. */
  dias_en_camino: number;
  lineas: TrasladoLinea[];
}

export interface SugerenciaPedido {
  articulo_id: string;
  codigo: string;
  articulo: string;
  sede_id: string;
  sede: string;
  proveedor_id: string;
  proveedor: string;
  hay: number;
  minimo: number;
  maximo: number;
  /** Vacío = todavía no hay historia suficiente para calcularlo. */
  consumo_semanal: string;
  sugerido: number;
  /** El costo promedio conocido del artículo, para poder mostrar cantidad × costo. */
  costo_unitario_crc: string;
  /** La frase que explica el número. Sin ella, una cantidad sugerida es una orden a ciegas. */
  de_donde_sale: string;
  costo_estimado_crc: string;
}

export interface RotacionArticulo {
  articulo_id: string;
  codigo: string;
  articulo: string;
  categoria: string;
  en_stock: number;
  salidas: number;
  rotacion: string;
  dias_de_stock: string;
  /** Capital PROPIO detenido. Lo consignado va aparte: no es plata de la empresa. */
  capital_crc: string;
  capital_consignado_crc: string;
  lectura: LecturaRotacion;
}

export interface NivelSede {
  sede_id: string;
  sede: string;
  minimo: number;
  maximo: number;
}

export interface ArticuloNuevo {
  codigo: string;
  nombre: string;
  categoria_id: string;
  modo_control: ModoControl;
  unidad_medida?: string;
  proveedor_id?: string;
  clasificacion_id?: string;
  activo?: boolean;
  nota?: string;
}

export interface EntradaNueva {
  articulo_id: string;
  sede_id: string;
  fecha: string;
  cantidad: number;
  costo_unitario: string;
  proveedor_id?: string;
  documento_cxp_id?: string;
  es_consignada?: boolean;
  /** Números de las unidades. Vacío = los genera el sistema. */
  numeros?: string[];
  nota?: string;
}

export interface ServicioNuevo {
  sede_id: string;
  fecha: string;
  a_nombre_de: string;
  nota?: string;
  consumos: { articulo_id: string; unidad_numero?: string; cantidad?: number }[];
}

export interface AjusteNuevo {
  articulo_id: string;
  sede_id?: string;
  unidad_numero?: string;
  fecha: string;
  /** Con signo: +2 sobraban dos, −3 faltaban tres. Para una unidad se usa `nuevo_estado`. */
  diferencia?: number;
  nuevo_estado?: EstadoUnidad;
  motivo: string;
}

export interface TrasladoNuevo {
  sede_origen_id: string;
  sede_destino_id: string;
  fecha: string;
  nota?: string;
  lineas: { articulo_id: string; unidad_numero?: string; cantidad?: number }[];
}

export const inventarioApi = {
  // ── Catálogo ──────────────────────────────────────────────────────────────
  categorias(incluirInactivas = false): Promise<CategoriaInv[]> {
    return apiFetch<CategoriaInv[]>("/inventario/categorias", {
      method: "GET",
      query: incluirInactivas ? { incluir_inactivas: "true" } : {},
    });
  },
  crearCategoria(nombre: string, padreId = ""): Promise<CategoriaInv> {
    return apiFetch<CategoriaInv>("/inventario/categorias", {
      method: "POST",
      json: { nombre, padre_id: padreId },
    });
  },
  actualizarCategoria(id: string, nombre: string, activo: boolean): Promise<void> {
    return apiFetch<void>(`/inventario/categorias/${id}`, {
      method: "PATCH",
      json: { nombre, activo },
    });
  },

  articulos(f: {
    categoria_id?: string;
    modo_control?: string;
    q?: string;
    incluir_inactivos?: boolean;
  } = {}): Promise<ArticuloInv[]> {
    return apiFetch<ArticuloInv[]>("/inventario/articulos", {
      method: "GET",
      query: {
        ...(f.categoria_id ? { categoria_id: f.categoria_id } : {}),
        ...(f.modo_control ? { modo_control: f.modo_control } : {}),
        ...(f.q ? { q: f.q } : {}),
        ...(f.incluir_inactivos ? { incluir_inactivos: "true" } : {}),
      },
    });
  },
  crearArticulo(a: ArticuloNuevo): Promise<{ id: string }> {
    return apiFetch<{ id: string }>("/inventario/articulos", { method: "POST", json: a });
  },
  actualizarArticulo(id: string, a: ArticuloNuevo): Promise<void> {
    return apiFetch<void>(`/inventario/articulos/${id}`, { method: "PATCH", json: a });
  },
  niveles(articuloId: string): Promise<NivelSede[]> {
    return apiFetch<NivelSede[]>(`/inventario/articulos/${articuloId}/niveles`, { method: "GET" });
  },
  fijarNivel(articuloId: string, sedeId: string, minimo: number, maximo: number): Promise<void> {
    return apiFetch<void>(`/inventario/articulos/${articuloId}/nivel`, {
      method: "PUT",
      json: { sede_id: sedeId, minimo, maximo },
    });
  },

  // ── Existencias ───────────────────────────────────────────────────────────
  existencias(f: {
    sede_id?: string;
    categoria_id?: string;
    modo_control?: string;
    q?: string;
    solo_bajo_minimo?: boolean;
    solo_consignadas?: boolean;
  } = {}): Promise<ResumenExistencias> {
    return apiFetch<ResumenExistencias>("/inventario/existencias", {
      method: "GET",
      query: {
        ...(f.sede_id ? { sede_id: f.sede_id } : {}),
        ...(f.categoria_id ? { categoria_id: f.categoria_id } : {}),
        ...(f.modo_control ? { modo_control: f.modo_control } : {}),
        ...(f.q ? { q: f.q } : {}),
        ...(f.solo_bajo_minimo ? { solo_bajo_minimo: "true" } : {}),
        ...(f.solo_consignadas ? { solo_consignadas: "true" } : {}),
      },
    });
  },
  unidades(f: {
    articulo_id?: string;
    sede_id?: string;
    estado?: string;
    q?: string;
    quietas_desde_dias?: number;
    limite?: number;
    /**
     * true = solo las del proveedor, false = solo las propias, undefined = todas.
     * Es opcional y no un boolean simple porque «no me importa» y «solo las propias» son preguntas
     * distintas: con un boolean, pedir las propias devolvería todo.
     */
    consignada?: boolean;
  } = {}): Promise<UnidadInv[]> {
    return apiFetch<UnidadInv[]>("/inventario/unidades", {
      method: "GET",
      query: {
        ...(f.articulo_id ? { articulo_id: f.articulo_id } : {}),
        ...(f.sede_id ? { sede_id: f.sede_id } : {}),
        ...(f.estado ? { estado: f.estado } : {}),
        ...(f.q ? { q: f.q } : {}),
        ...(f.quietas_desde_dias ? { quietas_desde_dias: String(f.quietas_desde_dias) } : {}),
        ...(f.limite ? { limite: String(f.limite) } : {}),
        ...(f.consignada === undefined ? {} : { consignada: String(f.consignada) }),
      },
    });
  },
  movimientos(f: {
    articulo_id?: string;
    unidad_id?: string;
    sede_id?: string;
    tipo?: string;
    desde?: string;
    hasta?: string;
    limite?: number;
  } = {}): Promise<MovimientoInv[]> {
    return apiFetch<MovimientoInv[]>("/inventario/movimientos", {
      method: "GET",
      query: {
        ...(f.articulo_id ? { articulo_id: f.articulo_id } : {}),
        ...(f.unidad_id ? { unidad_id: f.unidad_id } : {}),
        ...(f.sede_id ? { sede_id: f.sede_id } : {}),
        ...(f.tipo ? { tipo: f.tipo } : {}),
        ...(f.desde ? { desde: f.desde } : {}),
        ...(f.hasta ? { hasta: f.hasta } : {}),
        ...(f.limite ? { limite: String(f.limite) } : {}),
      },
    });
  },

  // ── Operaciones ───────────────────────────────────────────────────────────
  registrarEntrada(e: EntradaNueva): Promise<{
    movimiento_id: string;
    cantidad: number;
    numeros_creados: string[];
  }> {
    return apiFetch("/inventario/entradas", { method: "POST", json: e });
  },
  /** El hecho que descarga el inventario. */
  registrarServicio(s: ServicioNuevo): Promise<ServicioInv> {
    return apiFetch<ServicioInv>("/inventario/servicios", { method: "POST", json: s });
  },
  servicios(desde = "", hasta = ""): Promise<ServicioInv[]> {
    return apiFetch<ServicioInv[]>("/inventario/servicios", {
      method: "GET",
      query: { ...(desde ? { desde } : {}), ...(hasta ? { hasta } : {}) },
    });
  },
  registrarAjuste(a: AjusteNuevo): Promise<void> {
    return apiFetch<void>("/inventario/ajustes", { method: "POST", json: a });
  },

  // ── Traslados ─────────────────────────────────────────────────────────────
  traslados(estado = ""): Promise<TrasladoInv[]> {
    return apiFetch<TrasladoInv[]>("/inventario/traslados", {
      method: "GET",
      query: estado ? { estado } : {},
    });
  },
  crearTraslado(t: TrasladoNuevo): Promise<TrasladoInv> {
    return apiFetch<TrasladoInv>("/inventario/traslados", { method: "POST", json: t });
  },
  recibirTraslado(id: string, fecha = ""): Promise<void> {
    return apiFetch<void>(`/inventario/traslados/${id}/recibir`, {
      method: "POST",
      json: { fecha },
    });
  },

  // ── Análisis ──────────────────────────────────────────────────────────────
  reposicion(sedeId = "", semanas = 8): Promise<SugerenciaPedido[]> {
    return apiFetch<SugerenciaPedido[]>("/inventario/reposicion", {
      method: "GET",
      query: { ...(sedeId ? { sede_id: sedeId } : {}), semanas: String(semanas) },
    });
  },
  rotacion(desde = "", hasta = ""): Promise<{
    desde: string;
    hasta: string;
    filas: RotacionArticulo[];
  }> {
    return apiFetch("/inventario/rotacion", {
      method: "GET",
      query: { ...(desde ? { desde } : {}), ...(hasta ? { hasta } : {}) },
    });
  },
};

// ── Conteo cíclico (Fase 2) ─────────────────────────────────────────────────

/** Estado de una hoja de conteo. */
export type EstadoConteo = "ABIERTO" | "CERRADO" | "ANULADO";

/** Estado de una línea. SIN_CONTAR ≠ contó cero. */
export type EstadoLineaConteo = "SIN_CONTAR" | "CUADRA" | "EXPLICADA" | "SIN_EXPLICAR";

/** Urgencia del plan de conteo. */
export type EstadoPlanConteo = "NUNCA_CONTADO" | "AL_DIA" | "POR_VENCER" | "ATRASADO";

export interface ConteoLinea {
  id: string;
  articulo_id: string;
  codigo: string;
  articulo: string;
  categoria: string;
  modo_control: ModoControl;
  unidad_id: string;
  unidad_numero: string;
  /** La foto del sistema al abrir la hoja. No cambia después. */
  cantidad_sistema: number;
  /** −1 = todavía sin contar. Es distinto de haber contado cero. */
  cantidad_contada: number;
  diferencia: number;
  motivo: string;
  estado: EstadoLineaConteo;
  costo_unitario_crc: string;
  /** Cuánto vale la diferencia: es lo que vuelve accionable un faltante. */
  impacto_crc: string;
}

export interface ConteoInv {
  id: string;
  numero: string;
  sede_id: string;
  sede: string;
  categoria_id: string;
  categoria: string;
  estado: EstadoConteo;
  abierto_en: string;
  cerrado_en: string;
  abierto_por: string;
  cerrado_por: string;
  nota: string;
  lineas: number;
  contadas: number;
  con_diferencia: number;
  sin_explicar: number;
  /** Si el cierre va a pasar. Evita ofrecer un botón que va a fallar. */
  puede_cerrarse: boolean;
  filas: ConteoLinea[];
}

export interface CierreConteo {
  numero: string;
  lineas: number;
  ajustes_que_suman: number;
  ajustes_que_restan: number;
  bajas: number;
  /** Cuánto cambió el valor del inventario. Negativo = faltaba. */
  impacto_crc: string;
}

export interface PlanConteo {
  sede_id: string;
  sede: string;
  /** A = concentra el capital, C = cola larga. Se deriva del capital, no se configura. */
  clase: "A" | "B" | "C";
  clase_texto: string;
  articulos: number;
  capital_crc: string;
  /** Vacío = nunca se contó. */
  ultimo_conteo: string;
  dias_desde: number;
  cada_cuantos_dias: number;
  estado: EstadoPlanConteo;
}

export const conteoApi = {
  plan(): Promise<PlanConteo[]> {
    return apiFetch<PlanConteo[]>("/inventario/conteos/plan", { method: "GET" });
  },
  lista(estado = "", sedeId = ""): Promise<ConteoInv[]> {
    return apiFetch<ConteoInv[]>("/inventario/conteos", {
      method: "GET",
      query: { ...(estado ? { estado } : {}), ...(sedeId ? { sede_id: sedeId } : {}) },
    });
  },
  hoja(id: string): Promise<ConteoInv> {
    return apiFetch<ConteoInv>(`/inventario/conteos/${id}`, { method: "GET" });
  },
  /** Abrir congela lo que el sistema dice que hay: la hoja se compara contra esa foto. */
  abrir(v: { sede_id: string; categoria_id?: string; fecha?: string; nota?: string }): Promise<ConteoInv> {
    return apiFetch<ConteoInv>("/inventario/conteos", { method: "POST", json: v });
  },
  guardarLinea(conteoId: string, lineaId: string, cantidad: number, motivo = ""): Promise<void> {
    return apiFetch<void>(`/inventario/conteos/${conteoId}/lineas/${lineaId}`, {
      method: "PUT",
      json: { cantidad_contada: cantidad, motivo },
    });
  },
  /** Cerrar convierte cada diferencia explicada en su movimiento de ajuste. */
  cerrar(id: string, fecha = ""): Promise<CierreConteo> {
    return apiFetch<CierreConteo>(`/inventario/conteos/${id}/cerrar`, {
      method: "POST",
      json: { fecha },
    });
  },
  anular(id: string, motivo: string): Promise<void> {
    return apiFetch<void>(`/inventario/conteos/${id}/anular`, { method: "POST", json: { motivo } });
  },
};

// ── Consignación (Fase 3) ───────────────────────────────────────────────────
//
// El proveedor deja el cofre en la funeraria y cobra cuando se usa. La cola muestra qué mercadería
// suya ya salió de la bodega y todavía no tiene cuenta por pagar.

/** PENDIENTE = salió y nadie facturó · FACTURADA = ya tiene su cuenta por pagar. */
export type SituacionConsignada = "PENDIENTE" | "FACTURADA";

export interface ConsignadaSalida {
  unidad_id: string;
  unidad_numero: string;
  articulo: string;
  categoria: string;
  estado: EstadoUnidad;
  estado_legible: string;
  costo_crc: string;
  proveedor_id: string;
  proveedor: string;
  sede: string;
  fecha_salida: string;
  /** SALIDA = se usó en un servicio · BAJA = se dañó o el conteo no la encontró. */
  tipo_salida: string;
  motivo_salida: string;
  servicio_numero: string;
  servicio_a_nombre_de: string;
  /** Vacío = todavía no se le facturó al proveedor. */
  documento_id: string;
  documento_consecutivo: string;
  documento_estado: string;
  documento_total_crc: string;
  /** La factura con la que se COMPRÓ la unidad. No es lo mismo que documento_id. */
  documento_compra_id: string;
  /** Lo enlazado es la provisión que generó el sistema (true) o la factura real (false). */
  documento_es_provision: boolean;
  /** Si todavía se le debe al proveedor por esta unidad. Lo deriva el SQL, no la presencia del enlace. */
  deuda_vigente: boolean;
  situacion: SituacionConsignada;
  /** Solo el uso factura automáticamente; los demás casos los decide una persona. */
  puede_facturarse: boolean;
  /** Hay una provisión vigente esperando la factura real: todavía queda algo por conciliar. */
  puede_conciliarse: boolean;
  aviso: string;
}

export interface ResumenConsignacion {
  /** Lo del proveedor que sigue en bodega: no se le debe nada por eso. */
  en_bodega: number;
  en_bodega_crc: string;
  /** Lo que salió y nadie facturó: plata que se le debe y no está en ninguna cuenta por pagar. */
  por_facturar: number;
  por_facturar_crc: string;
  /** Lo que salió por otra razón (se dañó, se devolvió, no apareció): NO es deuda, es un caso a resolver. */
  a_decidir: number;
  a_decidir_crc: string;
  facturadas: number;
  facturadas_crc: string;
  proveedores: number;
  aviso: string;
}

export interface ColaConsignacion {
  resumen: ResumenConsignacion;
  filas: ConsignadaSalida[];
  /** Si el servidor no tiene CxP conectado, la cola se ve pero no se puede facturar. */
  puede_facturar: boolean;
}

export const consignacionApi = {
  cola(f: { proveedor_id?: string; estado?: string; situacion?: string } = {}): Promise<ColaConsignacion> {
    return apiFetch<ColaConsignacion>("/inventario/consignacion", {
      method: "GET",
      query: {
        ...(f.proveedor_id ? { proveedor_id: f.proveedor_id } : {}),
        ...(f.estado ? { estado: f.estado } : {}),
        ...(f.situacion ? { situacion: f.situacion } : {}),
      },
    });
  },
  /**
   * Genera la PROVISIÓN de la cuenta por pagar. Va sin cuerpo a propósito: el proveedor, el monto y
   * la fecha salen de la unidad, así el cliente no puede mandar un monto distinto del que dice el
   * inventario.
   */
  facturar(unidadId: string, confirmar = false): Promise<ConsignadaSalida> {
    return apiFetch<ConsignadaSalida>(`/inventario/consignacion/${unidadId}/facturar`, {
      method: "POST",
      query: confirmar ? { confirmar: "true" } : {},
      json: {},
    });
  },
  /** Reemplaza la provisión por la factura electrónica real del proveedor. */
  conciliar(unidadId: string, documentoId: string): Promise<ConsignadaSalida> {
    return apiFetch<ConsignadaSalida>(`/inventario/consignacion/${unidadId}/conciliar`, {
      method: "POST",
      json: { documento_id: documentoId },
    });
  },
  servicio(id: string): Promise<ServicioInv> {
    return apiFetch<ServicioInv>(`/inventario/servicios/${id}`, { method: "GET" });
  },
  unidad(numero: string): Promise<UnidadInv> {
    return apiFetch<UnidadInv>(`/inventario/unidades/${encodeURIComponent(numero)}`, { method: "GET" });
  },
};

/** Una factura del proveedor que SÍ sirve para conciliar una unidad consignada. */
export interface FacturaCandidata {
  id: string;
  consecutivo: string;
  clave: string;
  total_crc: string;
  fecha: string;
}

export interface CandidatasConciliacion {
  filas: FacturaCandidata[];
  /** Cuántas hay en total, aunque la lista venga cortada: un tope que no avisa esconde lo buscado. */
  total: number;
  aviso: string;
}

/**
 * Las facturas usables para conciliar. Es un endpoint aparte y no un filtro del listado de CxP
 * porque los cuatro descartes —mismo proveedor, no anulada, no la propia provisión, no usada por
 * otra unidad— son las mismas guardas que valida el servidor al conciliar: ofrecerlas desde acá evita
 * mostrar opciones que van a ser rechazadas.
 */
export function candidatasDeConciliacion(unidadId: string): Promise<CandidatasConciliacion> {
  return apiFetch<CandidatasConciliacion>(`/inventario/consignacion/${unidadId}/candidatas`, {
    method: "GET",
  });
}
