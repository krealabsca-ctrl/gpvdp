/**
 * Pantalla — Catálogo de insumos (/precios/insumos).
 *
 * Lo primero que se pinta es CUÁNTOS insumos no tienen costo, y no es decoración: en el archivo
 * de costeo actual 675 de 693 líneas están en ese estado, y un catálogo así no sirve para costear
 * nada. La cifra tiene que estar antes que la tabla, porque una lista de insumos sin costo se ve
 * igual de completa que una con costo.
 *
 * El costo NO se edita en la fila: se CAPTURA con su fecha de vigencia. Editar un número en su
 * lugar borraría la historia, y con ella la respuesta a «¿qué costaba esto en enero?».
 */

import { useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Select,
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";

import {
  useCapturarCosto,
  useCrearInsumo,
  useHistorialDeCosto,
  useInsumos,
} from "../hooks";

const HOY = new Date().toISOString().slice(0, 10);

function money(v: string | null): string {
  if (v === null) return "—";
  const n = Number(v);
  if (Number.isNaN(n)) return v;
  return `₡${n.toLocaleString("es-CR", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

/** Diferencia porcentual entre el costo del modelo y la última compra real. */
function desvio(modelo: string | null, compra: string | null): number | null {
  if (modelo === null || compra === null) return null;
  const m = Number(modelo);
  const c = Number(compra);
  if (!m || Number.isNaN(m) || Number.isNaN(c)) return null;
  return ((c - m) / m) * 100;
}

export function InsumosPage() {
  const tienePermiso = useTienePermiso();
  const puedeEditar = tienePermiso("precios.editar");
  const toast = useToast();

  const [busca, setBusca] = useState("");
  const [tipo, setTipo] = useState("");
  const [soloSinCosto, setSoloSinCosto] = useState(false);
  const [fecha, setFecha] = useState(HOY);
  const [abierto, setAbierto] = useState<string | null>(null);

  const insumos = useInsumos({ fecha, busca, tipo, sinCosto: soloSinCosto });
  const historial = useHistorialDeCosto(abierto);
  const capturar = useCapturarCosto();
  const crear = useCrearInsumo();

  const [nuevo, setNuevo] = useState({ codigo: "", nombre: "", unidad: "UNIDAD", tipo_costo: "VARIABLE", base_iva: "SIN_IVA" });
  const [captura, setCaptura] = useState({ costo: "", vigenteDesde: HOY, nota: "" });

  const datos = insumos.data;
  const cobertura = datos && datos.total > 0
    ? ((datos.total - datos.sin_costo) / datos.total) * 100
    : 0;

  async function onCrear() {
    try {
      await crear.mutateAsync(nuevo as never);
      setNuevo({ codigo: "", nombre: "", unidad: "UNIDAD", tipo_costo: "VARIABLE", base_iva: "SIN_IVA" });
      toast.success("Insumo creado. Ahora capturale un costo para que pueda entrar a un costeo.");
    } catch (e) {
      toast.error(`No se pudo crear: `);
    }
  }

  async function onCapturar(insumoId: string) {
    try {
      await capturar.mutateAsync({
        insumoId,
        costo: captura.costo,
        vigenteDesde: captura.vigenteDesde,
        nota: captura.nota,
      });
      setCaptura({ costo: "", vigenteDesde: HOY, nota: "" });
      toast.success("Costo capturado. Se recalculó el costeo de todos los planes que lo usan.");
    } catch (e) {
      toast.error(`No se pudo guardar: `);
    }
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title="Catálogo de insumos"
        description="El costo de cada cosa que entra en un plan, contrastado con lo que CxP pagó de verdad."
      />

      {/* La cobertura va primero: sin ella, un catálogo a medias se lee como uno completo. */}
      {datos && (
        <Card>
          <CardContent className="flex flex-wrap items-center gap-6 py-4">
            <div>
              <div className="text-xs uppercase tracking-wide text-content-muted">Insumos</div>
              <div className="text-2xl font-bold tabular-nums">{datos.total}</div>
            </div>
            <div>
              <div className="text-xs uppercase tracking-wide text-content-muted">Con costo</div>
              <div className="text-2xl font-bold tabular-nums text-positivo">
                {datos.total - datos.sin_costo}
              </div>
            </div>
            <div>
              <div className="text-xs uppercase tracking-wide text-content-muted">Sin costo</div>
              <div
                className={`text-2xl font-bold tabular-nums ${datos.sin_costo > 0 ? "text-negativo" : ""}`}
              >
                {datos.sin_costo}
              </div>
            </div>
            <div className="min-w-[180px] flex-1">
              <div className="text-xs uppercase tracking-wide text-content-muted">
                Cobertura del catálogo
              </div>
              <div className="mt-1 flex items-center gap-2">
                <div className="h-2 flex-1 overflow-hidden rounded-full bg-surface-muted">
                  <div
                    className={`h-full rounded-full ${cobertura >= 90 ? "bg-positivo" : cobertura >= 50 ? "bg-pendiente" : "bg-negativo"}`}
                    style={{ width: `${Math.max(cobertura, 2)}%` }}
                  />
                </div>
                <span className="text-sm font-bold tabular-nums">{cobertura.toFixed(1)}%</span>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {datos && datos.sin_costo > 0 && (
        <div className="rounded-lg border border-negativo/40 bg-negativo/5 p-3 text-sm">
          <b>{datos.sin_costo}</b> de {datos.total} insumos no tienen costo capturado. Todo plan que
          los use queda con el costo <b>incompleto</b>, y por eso su margen no se publica: un margen
          calculado sobre un costo parcial sale más alto que el real.
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Filtros</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap items-end gap-3">
          <Input
            label="Buscar"
            placeholder="nombre o código…"
            value={busca}
            onChange={(e) => setBusca(e.target.value)}
          />
          <Select
            label="Tipo"
            value={tipo}
            onChange={(e) => setTipo(e.target.value)}
            options={[
              { value: "", label: "Todos" },
              { value: "VARIABLE", label: "Variable" },
              { value: "FIJO", label: "Fijo" },
            ]}
          />
          <Input
            label="Costo vigente al"
            type="date"
            value={fecha}
            onChange={(e) => setFecha(e.target.value)}
          />
          <label className="flex items-center gap-2 pb-2 text-sm">
            <input
              type="checkbox"
              checked={soloSinCosto}
              onChange={(e) => setSoloSinCosto(e.target.checked)}
            />
            Solo los que no tienen costo
          </label>
        </CardContent>
      </Card>

      {puedeEditar && (
        <Card>
          <CardHeader>
            <CardTitle>Agregar un insumo</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-wrap items-end gap-3">
            <Input
              label="Código *"
              value={nuevo.codigo}
              onChange={(e) => setNuevo({ ...nuevo, codigo: e.target.value })}
            />
            <Input
              label="Nombre *"
              value={nuevo.nombre}
              onChange={(e) => setNuevo({ ...nuevo, nombre: e.target.value })}
            />
            <Input
              label="Unidad"
              value={nuevo.unidad}
              onChange={(e) => setNuevo({ ...nuevo, unidad: e.target.value })}
            />
            <Select
              label="Tipo de costo"
              value={nuevo.tipo_costo}
              onChange={(e) => setNuevo({ ...nuevo, tipo_costo: e.target.value })}
              options={[
                { value: "VARIABLE", label: "Variable — se consume con cada servicio" },
                { value: "FIJO", label: "Fijo — se absorbe por reparto" },
              ]}
            />
            <Select
              label="El costo se captura"
              value={nuevo.base_iva}
              onChange={(e) => setNuevo({ ...nuevo, base_iva: e.target.value })}
              options={[
                { value: "SIN_IVA", label: "Sin IVA" },
                { value: "CON_IVA", label: "Con IVA (se descuenta al guardar)" },
              ]}
            />
            <Button onClick={onCrear} disabled={crear.isPending || !nuevo.codigo || !nuevo.nombre}>
              Agregar
            </Button>
          </CardContent>
        </Card>
      )}

      {insumos.isLoading && <LoadingState />}
      {insumos.isError && <ErrorState message={mensajeError(insumos.error)} />}

      {datos && datos.insumos.length === 0 && (
        <EmptyState message="No hay insumos — Agregá el primero arriba, o quitá los filtros." />
      )}

      {datos && datos.insumos.length > 0 && (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Código</TH>
                <TH>Insumo</TH>
                <TH>Tipo</TH>
                <TH className="text-right">Costo del modelo</TH>
                <TH className="text-right">Última compra (CxP)</TH>
                <TH className="text-right">Δ</TH>
                <TH>Estado</TH>
                {puedeEditar && <TH></TH>}
              </TR>
            </THead>
            <TBody>
              {datos.insumos.map((i) => {
                const d = desvio(i.costo_vigente, i.ultima_compra_cxp);
                return (
                  <TR key={i.id}>
                    <TD className="font-mono text-xs">{i.codigo}</TD>
                    <TD>
                      <div className="font-medium">{i.nombre}</div>
                      {i.proveedor && (
                        <div className="text-xs text-content-muted">{i.proveedor}</div>
                      )}
                    </TD>
                    <TD>
                      <Badge tone={i.tipo_costo === "FIJO" ? "pendiente" : "neutral"}>
                        {i.tipo_costo === "FIJO" ? "Fijo" : "Variable"}
                      </Badge>
                    </TD>
                    <TD className="text-right tabular-nums">
                      {money(i.costo_vigente)}
                      {i.vigente_desde && (
                        <div className="text-xs text-content-muted">desde {i.vigente_desde}</div>
                      )}
                    </TD>
                    <TD className="text-right tabular-nums">
                      {money(i.ultima_compra_cxp)}
                      {i.ultima_compra_en && (
                        <div className="text-xs text-content-muted">{i.ultima_compra_en}</div>
                      )}
                    </TD>
                    <TD className="text-right tabular-nums">
                      {d === null ? (
                        "—"
                      ) : (
                        <span className={Math.abs(d) >= 5 ? "text-negativo" : "text-content-muted"}>
                          {d > 0 ? "+" : ""}
                          {d.toFixed(1)}%
                        </span>
                      )}
                    </TD>
                    <TD>
                      {i.costo_vigente === null ? (
                        <Badge tone="negativo">Sin costo</Badge>
                      ) : d !== null && Math.abs(d) >= 5 ? (
                        <Badge tone="pendiente">Desactualizado</Badge>
                      ) : (
                        <Badge tone="positivo">Al día</Badge>
                      )}
                    </TD>
                    {puedeEditar && (
                      <TD>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setAbierto(abierto === i.id ? null : i.id)}
                        >
                          {abierto === i.id ? "Cerrar" : "Capturar costo"}
                        </Button>
                      </TD>
                    )}
                  </TR>
                );
              })}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {abierto && puedeEditar && (
        <Card>
          <CardHeader>
            <CardTitle>Capturar un costo</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm text-content-muted">
              El costo nuevo <b>no reemplaza</b> al anterior: se agrega con su fecha de vigencia, y
              el costeo de cada plan toma el que regía en la fecha que se consulte.
            </p>
            <div className="flex flex-wrap items-end gap-3">
              <Input
                label="Costo"
                value={captura.costo}
                onChange={(e) => setCaptura({ ...captura, costo: e.target.value })}
              />
              <Input
                label="Rige desde"
                type="date"
                value={captura.vigenteDesde}
                onChange={(e) => setCaptura({ ...captura, vigenteDesde: e.target.value })}
              />
              <Input
                label="Nota"
                placeholder="de dónde salió…"
                value={captura.nota}
                onChange={(e) => setCaptura({ ...captura, nota: e.target.value })}
              />
              <Button
                onClick={() => onCapturar(abierto)}
                disabled={capturar.isPending || !captura.costo}
              >
                Guardar
              </Button>
            </div>

            {historial.data && historial.data.capturas.length > 0 && (
              <TableContainer>
                <Table>
                  <THead>
                    <TR>
                      <TH>Rige desde</TH>
                      <TH className="text-right">Costo</TH>
                      <TH>Origen</TH>
                      <TH>Nota</TH>
                      <TH>Capturó</TH>
                    </TR>
                  </THead>
                  <TBody>
                    {historial.data.capturas.map((c) => (
                      <TR key={`${c.vigente_desde}-${c.costo}`}>
                        <TD>{c.vigente_desde}</TD>
                        <TD className="text-right tabular-nums">{money(c.costo)}</TD>
                        <TD>{c.origen}</TD>
                        <TD className="text-content-muted">{c.nota || "—"}</TD>
                        <TD className="text-content-muted">{c.capturado_por || "—"}</TD>
                      </TR>
                    ))}
                  </TBody>
                </Table>
              </TableContainer>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
