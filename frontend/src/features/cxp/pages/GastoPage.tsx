/**
 * En qué se nos va la plata, ítem por ítem.
 *
 * EL PEDIDO (Director Financiero, 9-oct-2026): «el xml trae el cabis y el detalle de cada factura,
 * por unidad, por costo, por descripción; podemos hacer en cxp un apartado para analizar y ver la
 * lista de todo lo que usamos, y como lo hacen las grandes empresas».
 *
 * CÓMO LO HACEN LAS GRANDES EMPRESAS. Se llama *spend analysis* y el método es el mismo en todas:
 * tomar el detalle de línea de las facturas, agruparlo por una taxonomía común y mirar el mismo
 * ítem a lo largo del tiempo y entre proveedores. La parte cara de montarlo es la taxonomía —hay
 * empresas que pasan meses clasificando su gasto a mano—. Acá no hace falta: en Costa Rica el
 * emisor está OBLIGADO a declarar el código CABYS de cada línea ante Hacienda, así que el gasto
 * ya viene clasificado por quien lo vendió.
 *
 * LAS TRES PREGUNTAS, EN ESE ORDEN:
 *
 *   1. ¿En qué se va la plata?  → la lista ordenada por gasto. El 20 % de los ítems suele explicar
 *      el 80 % del monto, y ahí es donde negociar rinde.
 *   2. ¿Cuesta siempre lo mismo? → el mínimo, el máximo y el ÚLTIMO precio pagado por el mismo
 *      código. La diferencia entre el mínimo y el máximo es la señal más barata que existe.
 *   3. ¿A cuántos le compramos lo mismo? → comprarle un ítem a seis proveedores es poder de
 *      negociación que se está dejando sobre la mesa.
 *
 * LA COBERTURA VA PRIMERO Y NO ES DECORACIÓN. El detalle sale del XML, y una factura sin XML
 * cargado no aporta una sola línea. Una lista armada con el 2 % de las facturas es verdadera y, al
 * mismo tiempo, engañosa si nadie dice que es el 2 %: «gastamos ₡3 M en papel» puede ser ₡30 M.
 * Por eso lo primero que se pinta es cuánto del gasto se está mirando, y por eso el botón de
 * cargar XML está en esta misma pantalla y no escondido en otra.
 */
import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import { gastoApi, type ItemGasto, type ResultadoDetalleCarga } from "@/api/cxp";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";
import { formatMoneda, formatMonto } from "@/lib/format";
import { cn } from "@/lib/cn";

/**
 * VEREDICTO es el texto que esta pantalla muestra cuando NO hay nada que analizar.
 *
 * Se exporta para que una prueba lo lea: en este proyecto ya pasó que una función existiera, se
 * probara entera y el usuario nunca viera el resultado. Una pantalla vacía sin explicación se lee
 * como «no gastamos nada», que es lo contrario de la verdad.
 */
export const VEREDICTO = {
  sinDetalle:
    "Todavía no hay detalle cargado. Las facturas del sistema entraron por Excel y traen solo el " +
    "encabezado; el detalle de qué se compró vive en el XML de Hacienda. Subí la carpeta de XML y " +
    "cada uno busca su factura solo, por la clave.",
  coberturaBaja:
    "Este análisis cubre una parte del gasto. Los ítems que se ven son reales, pero los montos son " +
    "los de las facturas que YA tienen su XML cargado.",
} as const;

/** Por debajo de esto, la pantalla avisa en grande antes de mostrar un solo número. */
const COBERTURA_MINIMA = 80;

export function GastoPage() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();

  const [desde, setDesde] = useState("");
  const [hasta, setHasta] = useState("");
  const [texto, setTexto] = useState("");
  const [buscado, setBuscado] = useState("");
  const [abierto, setAbierto] = useState<string | null>(null);
  const [carga, setCarga] = useState<ResultadoDetalleCarga | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const filtros = useMemo(() => ({ desde, hasta, q: buscado }), [desde, hasta, buscado]);

  const analisis = useQuery({
    queryKey: ["cxp", empresaId, "gasto", filtros],
    queryFn: () => gastoApi.analisis(filtros),
    enabled: !!empresaId,
  });

  const compras = useQuery({
    queryKey: ["cxp", empresaId, "gasto-compras", abierto, filtros],
    queryFn: () => gastoApi.compras(abierto as string, filtros),
    enabled: !!empresaId && !!abierto,
  });

  const subir = useMutation({
    mutationFn: (archivos: File[]) => gastoApi.cargarXML(archivos),
    onSuccess: (r) => {
      setCarga(r);
      // La clave raíz invalida por prefijo: con la empresa SEGUNDA, esto refresca todo CxP de
      // esta empresa y de ninguna otra.
      void qc.invalidateQueries({ queryKey: ["cxp", empresaId] });
      // Cero facturas emparejadas NO es un éxito callado: se avisa como información, porque el
      // parte de abajo es el que dice por qué (no hay factura con esa clave, archivo ilegible…).
      if (r.facturas > 0) {
        toast.success(`${r.facturas} factura(s) con detalle, ${r.lineas} línea(s)`);
      } else {
        toast.info("Ningún XML encontró su factura. Mirá el detalle de abajo.");
      }
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const cob = analisis.data?.cobertura;
  const pct = cob ? Number(cob.porcentaje_monto) : 0;
  const hayDetalle = !!cob && cob.facturas_con_detalle > 0;

  function elegirArchivos(e: React.ChangeEvent<HTMLInputElement>) {
    const fs = Array.from(e.target.files ?? []);
    if (fs.length === 0) return;
    setCarga(null);
    subir.mutate(fs);
    // Se limpia el input para que volver a elegir los MISMOS archivos dispare el evento otra vez.
    if (inputRef.current) inputRef.current.value = "";
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Análisis de gasto"
        description="Qué se compra, a qué precio y a cuántos proveedores — según el detalle del XML de cada factura"
        actions={
          <>
            <input
              ref={inputRef}
              type="file"
              multiple
              accept=".xml,.zip,text/xml,application/xml,application/zip"
              className="hidden"
              onChange={elegirArchivos}
            />
            <Button
              variant="secondary"
              onClick={() => inputRef.current?.click()}
              disabled={subir.isPending}
            >
              {subir.isPending ? "Cargando…" : "Cargar XML"}
            </Button>
          </>
        }
      />

      {carga && <ParteDeCarga r={carga} onCerrar={() => setCarga(null)} />}

      {analisis.isLoading && <LoadingState label="Leyendo el detalle de las facturas…" />}
      {analisis.isError && <ErrorState message={mensajeError(analisis.error)} />}

      {analisis.data && (
        <>
          <Cobertura
            facturas={analisis.data.cobertura.facturas}
            conDetalle={analisis.data.cobertura.facturas_con_detalle}
            montoTotal={analisis.data.cobertura.monto_total}
            montoConDetalle={analisis.data.cobertura.monto_con_detalle}
            pct={pct}
            sinCabys={analisis.data.cobertura.lineas_sin_cabys}
            sinConvertir={analisis.data.cobertura.lineas_sin_convertir}
          />

          {!hayDetalle ? (
            <EmptyState message={VEREDICTO.sinDetalle} />
          ) : (
            <>
              <Card>
                <CardContent className="flex flex-wrap items-end gap-3 pt-5">
                  <label className="flex flex-col gap-1 text-xs text-content-muted">
                    Desde
                    <Input type="date" value={desde} onChange={(e) => setDesde(e.target.value)} />
                  </label>
                  <label className="flex flex-col gap-1 text-xs text-content-muted">
                    Hasta
                    <Input type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} />
                  </label>
                  <label className="flex min-w-[16rem] flex-1 flex-col gap-1 text-xs text-content-muted">
                    Buscar un ítem (o pegar un código CABYS)
                    <Input
                      value={texto}
                      placeholder="papel, tornillo, 2399001…"
                      onChange={(e) => setTexto(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") setBuscado(texto);
                      }}
                    />
                  </label>
                  <Button variant="secondary" onClick={() => setBuscado(texto)}>
                    Buscar
                  </Button>
                  {(desde || hasta || buscado) && (
                    <Button
                      variant="ghost"
                      onClick={() => {
                        setDesde("");
                        setHasta("");
                        setTexto("");
                        setBuscado("");
                      }}
                    >
                      Limpiar
                    </Button>
                  )}
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Lo que compramos</CardTitle>
                  <CardDescription>
                    {analisis.data.items_total} ítem(s) · {formatMoneda(analisis.data.total, "CRC")} en
                    total
                    {analisis.data.items_total > analisis.data.items.length && (
                      <> · se muestran los {analisis.data.items.length} de mayor gasto</>
                    )}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  {analisis.data.items.length === 0 ? (
                    <EmptyState message="No hay líneas que calcen con ese filtro. Probá con otro texto o ampliá el rango de fechas." />
                  ) : (
                    <TableContainer>
                      <Table>
                        <THead>
                          <TR>
                            <TH>Ítem</TH>
                            <TH className="text-right">Gasto</TH>
                            <TH className="text-right">Facturas</TH>
                            <TH className="text-right">Prov.</TH>
                            <TH className="text-right">Último precio</TH>
                            <TH className="text-right">Variación</TH>
                            <TH />
                          </TR>
                        </THead>
                        <TBody>
                          {analisis.data.items.map((it) => (
                            <FilaItem
                              key={it.cabys}
                              it={it}
                              abierto={abierto === it.cabys}
                              onAbrir={() => setAbierto(abierto === it.cabys ? null : it.cabys)}
                              compras={abierto === it.cabys ? compras.data?.compras : undefined}
                              cargando={abierto === it.cabys && compras.isLoading}
                            />
                          ))}
                        </TBody>
                      </Table>
                    </TableContainer>
                  )}
                </CardContent>
              </Card>
            </>
          )}
        </>
      )}
    </div>
  );
}

/** La barra de cobertura: cuánto del gasto estoy mirando de verdad. */
function Cobertura(props: {
  facturas: number;
  conDetalle: number;
  montoTotal: string;
  montoConDetalle: string;
  pct: number;
  sinCabys: number;
  sinConvertir: number;
}) {
  const baja = props.pct < COBERTURA_MINIMA;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Sobre cuánto del gasto es este análisis</CardTitle>
        <CardDescription>
          {props.conDetalle} de {props.facturas} factura(s) tienen su detalle cargado ·{" "}
          {formatMoneda(props.montoConDetalle, "CRC")} de {formatMoneda(props.montoTotal, "CRC")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-center gap-3">
          <div className="h-2 flex-1 overflow-hidden rounded-full bg-surface-muted">
            <div
              className={cn("h-full rounded-full", baja ? "bg-pendiente" : "bg-positivo")}
              style={{ width: `${Math.min(100, Math.max(0, props.pct))}%` }}
            />
          </div>
          <span className={cn("text-sm font-semibold", baja ? "text-pendiente" : "text-positivo")}>
            {props.pct.toLocaleString("es-CR", { maximumFractionDigits: 1 })} %
          </span>
        </div>
        {/* Con CERO facturas cargadas este aviso sobraría: abajo ya está la explicación entera de
            qué falta y cómo traerlo, y leer dos mensajes sobre lo mismo confunde en vez de
            advertir. Esto es para la cobertura PARCIAL, que es el caso peligroso: hay números en
            pantalla y se leen como si fueran el total. */}
        {baja && props.conDetalle > 0 && (
          <p className="text-sm text-pendiente">{VEREDICTO.coberturaBaja}</p>
        )}

        {/* Lo que quedó incompleto DENTRO de lo que sí se cargó. Son dos problemas distintos de la
            cobertura por factura y por eso se dicen aparte. */}
        {props.sinCabys > 0 && (
          <p className="text-sm text-content-muted">
            {props.sinCabys} línea(s) sin código CABYS: el emisor no lo declaró, así que quedan
            agrupadas aparte bajo un código vacío.
          </p>
        )}
        {props.sinConvertir > 0 && (
          <p className="text-sm text-pendiente">
            {props.sinConvertir} línea(s) de facturas en otra moneda SIN tipo de cambio: no se
            pudieron convertir y NO están sumadas en los montos.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Una fila de la lista, con las compras desplegables debajo.
 *
 * La variación solo se pinta en rojo cuando hay con qué compararla. Con una sola compra no hay
 * variación posible y un «0 %» se leería como «el precio está firme», que es una afirmación que
 * nadie hizo.
 */
function FilaItem(props: {
  it: ItemGasto;
  abierto: boolean;
  onAbrir: () => void;
  compras?: { fecha: string; proveedor: string; consecutivo: string; cantidad: string; unidad: string; precio_unitario: string; total: string; moneda: string }[];
  cargando: boolean;
}) {
  const { it } = props;
  const variacion = it.variacion_pct ? Number(it.variacion_pct) : null;
  const alerta = variacion !== null && variacion >= 15;

  return (
    <>
      <TR
        className="cursor-pointer hover:bg-surface-muted/50"
        onClick={props.onAbrir}
        aria-expanded={props.abierto}
      >
        <TD>
          <div className="font-medium">{it.descripcion || "(sin descripción)"}</div>
          <div className="text-xs text-content-muted">
            {it.cabys ? `CABYS ${it.cabys}` : "sin código CABYS"}
            {it.descripciones > 1 && <> · {it.descripciones} descripciones distintas</>}
            {it.unidades > 1 && <> · {it.unidades} unidades de medida</>}
            {it.sin_convertir > 0 && (
              <span className="text-pendiente"> · {it.sin_convertir} sin convertir</span>
            )}
          </div>
        </TD>
        <TD className="text-right tabular-nums">{formatMonto(it.gasto_crc)}</TD>
        <TD className="text-right tabular-nums">{it.facturas}</TD>
        <TD className="text-right tabular-nums">
          {it.proveedores > 1 ? <Badge tone="accent">{it.proveedores}</Badge> : it.proveedores}
        </TD>
        <TD className="text-right tabular-nums">
          {it.precio_ultimo ? (
            <>
              {formatMonto(it.precio_ultimo)}
              {it.unidad && <span className="text-xs text-content-muted"> / {it.unidad}</span>}
            </>
          ) : (
            <span className="text-xs text-content-muted">
              {it.unidades > 1 ? "unidades distintas" : "—"}
            </span>
          )}
        </TD>
        <TD className="text-right tabular-nums">
          {variacion === null ? (
            <span className="text-content-muted">—</span>
          ) : (
            <span className={alerta ? "font-semibold text-negativo" : "text-content-muted"}>
              {variacion.toLocaleString("es-CR", { maximumFractionDigits: 1 })} %
            </span>
          )}
        </TD>
        <TD className="text-right text-xs text-accent">{props.abierto ? "Cerrar" : "Ver compras"}</TD>
      </TR>

      {props.abierto && (
        <TR>
          <TD colSpan={7} className="bg-surface-muted/30">
            {props.cargando ? (
              <LoadingState label="Buscando las compras…" />
            ) : !props.compras || props.compras.length === 0 ? (
              <p className="py-3 text-sm text-content-muted">Sin compras en el rango.</p>
            ) : (
              <div className="space-y-2 py-2">
                {variacion !== null && variacion > 0 && (
                  <p className="text-sm text-content-muted">
                    Entre {formatMonto(it.precio_min)} y {formatMonto(it.precio_max)} por{" "}
                    {it.unidad || "unidad"}. El precio de la última compra es{" "}
                    {formatMonto(it.precio_ultimo)}.
                  </p>
                )}
                <TableContainer>
                  <Table>
                    <THead>
                      <TR>
                        <TH>Fecha</TH>
                        <TH>Proveedor</TH>
                        <TH>Factura</TH>
                        <TH className="text-right">Cantidad</TH>
                        <TH className="text-right">Precio unit.</TH>
                        <TH className="text-right">Total</TH>
                      </TR>
                    </THead>
                    <TBody>
                      {props.compras.map((c, i) => (
                        <TR key={`${c.consecutivo}-${i}`}>
                          <TD className="whitespace-nowrap">{c.fecha}</TD>
                          <TD>{c.proveedor}</TD>
                          <TD className="font-mono text-xs">
                            {c.consecutivo}
                            {c.moneda !== "CRC" && (
                              <span className="ml-1 text-content-muted">({c.moneda})</span>
                            )}
                          </TD>
                          <TD className="text-right tabular-nums">
                            {formatMonto(c.cantidad)}
                            {c.unidad && (
                              <span className="text-xs text-content-muted"> {c.unidad}</span>
                            )}
                          </TD>
                          <TD className="text-right tabular-nums">
                            {/* Vacío = la factura era en otra moneda y no traía tipo de cambio.
                                Un cero acá diría que fue gratis. */}
                            {c.precio_unitario ? (
                              formatMonto(c.precio_unitario)
                            ) : (
                              <span className="text-xs text-pendiente">sin TC</span>
                            )}
                          </TD>
                          <TD className="text-right tabular-nums">
                            {c.total ? (
                              formatMonto(c.total)
                            ) : (
                              <span className="text-xs text-pendiente">sin TC</span>
                            )}
                          </TD>
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </TableContainer>
              </div>
            )}
          </TD>
        </TR>
      )}
    </>
  );
}

/**
 * El parte de la carga.
 *
 * Los XML que NO encontraron factura van PRIMERO y con nombre: son facturas que existen en
 * Hacienda y no en el sistema, o sea gasto que nunca entró a CxP. Enterrarlos en un contador de
 * «fallidos» escondería justamente el hallazgo.
 */
function ParteDeCarga({ r, onCerrar }: { r: ResultadoDetalleCarga; onCerrar: () => void }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Carga de XML</CardTitle>
        <CardDescription>
          {r.archivos} archivo(s) · {r.comprobantes} comprobante(s) · {r.facturas} factura(s) con
          detalle · {r.lineas} línea(s)
          {r.reemplazos > 0 && <> · {r.reemplazos} reemplazaron un detalle anterior</>}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {r.sin_factura.length > 0 && (
          <div className="space-y-1">
            <p className="text-sm font-semibold text-pendiente">
              {r.sin_factura.length} comprobante(s) sin factura en el sistema
            </p>
            <p className="text-xs text-content-muted">
              Existen en Hacienda pero no están cargados en CxP. Si corresponden a gasto de la
              empresa, hay que darlos de alta por Importar.
            </p>
            <ul className="max-h-48 space-y-1 overflow-auto text-xs">
              {r.sin_factura.map((x, i) => (
                <li key={`${x.nombre}-${i}`} className="font-mono text-content-muted">
                  {x.nombre}
                  {x.clave && <span className="ml-2">clave {x.clave}</span>}
                </li>
              ))}
            </ul>
          </div>
        )}

        {r.problemas.length > 0 && (
          <div className="space-y-1">
            <p className="text-sm font-semibold text-negativo">
              {r.problemas.length} archivo(s) que no se pudieron usar
            </p>
            <ul className="max-h-48 space-y-1 overflow-auto text-xs">
              {r.problemas.map((x, i) => (
                <li key={`${x.nombre}-${i}`} className="text-content-muted">
                  <span className="font-mono">{x.nombre}</span> — {x.motivo}
                </li>
              ))}
            </ul>
          </div>
        )}

        {r.cargadas.length > 0 && (
          <details>
            <summary className="cursor-pointer text-sm text-accent">
              Ver las {r.cargadas.length} factura(s) que recibieron detalle
            </summary>
            <ul className="mt-2 max-h-48 space-y-1 overflow-auto text-xs text-content-muted">
              {r.cargadas.map((f) => (
                <li key={f.documento_id}>
                  <span className="font-mono">{f.consecutivo}</span> · {f.proveedor} · {f.lineas}{" "}
                  línea(s)
                  {f.reemplazo && <span className="text-pendiente"> (reemplazó el anterior)</span>}
                </li>
              ))}
            </ul>
          </details>
        )}

        <Button variant="ghost" onClick={onCerrar}>
          Cerrar
        </Button>
      </CardContent>
    </Card>
  );
}
