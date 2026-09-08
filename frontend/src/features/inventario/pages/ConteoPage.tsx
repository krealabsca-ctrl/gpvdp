/**
 * Pantalla — Conteo cíclico (/inventario/conteo).
 *
 * Contesta dos preguntas distintas, y por eso son dos bloques:
 *
 *  1. **¿Qué toca contar?** El plan, por sede y clase de capital. La clase se deriva del capital: no
 *     hay nada que configurar y no puede quedar desactualizada.
 *  2. **La hoja abierta**: contar, explicar las diferencias y cerrar.
 *
 * La regla que gobierna la pantalla: **la diferencia se muestra y se explica; nunca se ajusta en
 * silencio.** El botón de cerrar solo aparece habilitado cuando el backend dice que va a pasar, y
 * cuando no, la pantalla dice exactamente qué falta.
 */

import { useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ConfirmDialog,
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
import type { BadgeTone } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatMoneda, formatMonto, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useSedes } from "@/features/bancos/hooks";
import {
  useAbrirConteo,
  useAnularConteo,
  useCategorias,
  useCerrarConteo,
  useConteos,
  useGuardarLineaConteo,
  useHojaConteo,
  usePlanConteo,
} from "@/features/inventario/hooks";
import type {
  ConteoLinea,
  EstadoLineaConteo,
  EstadoPlanConteo,
} from "@/api/inventario";

const PLAN: Record<EstadoPlanConteo, { tono: BadgeTone; texto: string }> = {
  NUNCA_CONTADO: { tono: "pendiente", texto: "nunca se contó" },
  AL_DIA: { tono: "positivo", texto: "al día" },
  POR_VENCER: { tono: "pendiente", texto: "toca pronto" },
  ATRASADO: { tono: "negativo", texto: "atrasado" },
};

const LINEA: Record<EstadoLineaConteo, { tono: BadgeTone; texto: string }> = {
  SIN_CONTAR: { tono: "neutral", texto: "sin contar" },
  CUADRA: { tono: "positivo", texto: "cuadra" },
  EXPLICADA: { tono: "pendiente", texto: "explicada" },
  SIN_EXPLICAR: { tono: "negativo", texto: "falta explicar" },
};

function hoyCR(): string {
  const d = new Date();
  const cr = new Date(d.getTime() - (d.getTimezoneOffset() + 360) * 60_000);
  return cr.toISOString().slice(0, 10);
}

export function ConteoPage() {
  const toast = useToast();
  const sedes = useSedes();
  const categorias = useCategorias();
  const plan = usePlanConteo();
  const abiertos = useConteos("ABIERTO");
  const cerrados = useConteos("CERRADO");
  const abrir = useAbrirConteo();

  const [sedeID, setSedeID] = useState("");
  const [categoriaID, setCategoriaID] = useState("");
  const [fecha, setFecha] = useState(hoyCR());
  const [hojaAbierta, setHojaAbierta] = useState<string | null>(null);

  const filasPlan = plan.data ?? [];
  const hojasAbiertas = abiertos.data ?? [];
  const hojasCerradas = cerrados.data ?? [];
  const atrasados = filasPlan.filter((p) => p.estado === "ATRASADO" || p.estado === "NUNCA_CONTADO");

  function abrirHoja() {
    if (!sedeID) {
      toast.error("Elegí la sede que se va a contar.");
      return;
    }
    abrir.mutate(
      { sede_id: sedeID, categoria_id: categoriaID, fecha },
      {
        onSuccess: (h) => {
          toast.success(
            `Hoja ${h.numero} abierta con ${h.lineas} línea(s). Lo que el sistema dice quedó congelado: la hoja se compara contra esa foto.`,
          );
          setHojaAbierta(h.id);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Conteo cíclico"
        description="Contar por partes, no parar la operación. Lo caro más seguido que lo barato."
      />

      {atrasados.length > 0 && (
        <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
          <p className="text-sm font-medium text-content">
            {atrasados.length} grupo(s) esperan conteo
          </p>
          <p className="mt-0.5 text-sm text-content-muted">
            Un inventario que nunca se cuenta es una hoja de cálculo: dice lo que se registró, no lo
            que hay en la bodega.
          </p>
        </div>
      )}

      {/* ── Qué toca contar ─────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle>Qué toca contar</CardTitle>
          <p className="mt-1 text-xs text-content-muted">
            La clase se calcula con el capital de cada sede: <strong>A</strong> son los artículos que
            concentran el 80 % del valor y se cuentan cada mes; <strong>C</strong> es la cola larga,
            una vez al año. No hay nada que configurar.
          </p>
        </CardHeader>
        <CardContent>
          {plan.isLoading && <LoadingState label="Calculando el plan…" />}
          {plan.isError && <ErrorState message={mensajeError(plan.error)} onRetry={() => plan.refetch()} />}
          {plan.data && filasPlan.length === 0 && (
            <EmptyState message="Todavía no hay existencias que contar." />
          )}
          {filasPlan.length > 0 && (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Sede</TH>
                    <TH>Clase</TH>
                    <TH className="text-right">Artículos</TH>
                    <TH className="text-right">Capital</TH>
                    <TH>Cada</TH>
                    <TH>Último conteo</TH>
                    <TH>Estado</TH>
                  </TR>
                </THead>
                <TBody>
                  {filasPlan.map((p) => (
                    <TR key={`${p.sede_id}-${p.clase}`}>
                      <TD className="font-medium text-content">{p.sede}</TD>
                      <TD>
                        <span className="font-medium text-content">{p.clase}</span>
                        <span className="block text-xs text-content-muted">{p.clase_texto}</span>
                      </TD>
                      <TD className="text-right tabular-nums">{p.articulos}</TD>
                      <TD className="text-right font-medium tabular-nums">
                        {formatMonto(p.capital_crc)}
                      </TD>
                      <TD className="tabular-nums text-content-muted">{p.cada_cuantos_dias} d</TD>
                      <TD className="tabular-nums text-content-muted">
                        {p.ultimo_conteo || "—"}
                        {p.ultimo_conteo && (
                          <span className="block text-xs">hace {p.dias_desde} d</span>
                        )}
                      </TD>
                      <TD>
                        <Badge tone={PLAN[p.estado]?.tono ?? "neutral"}>
                          {PLAN[p.estado]?.texto ?? p.estado}
                        </Badge>
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>

      {/* ── Abrir una hoja ──────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle>Abrir una hoja de conteo</CardTitle>
          <p className="mt-1 text-xs text-content-muted">
            Al abrirla se <strong>congela</strong> lo que el sistema dice que hay. Si alguien registra
            un movimiento mientras se cuenta, la hoja no cambia: se compara contra la foto del
            momento en que se abrió.
          </p>
        </CardHeader>
        <CardContent className="flex flex-wrap items-start gap-4">
          <div className="w-52">
            <Select
              label="Sede a contar *"
              value={sedeID}
              onChange={(e) => setSedeID(e.target.value)}
              options={[
                { value: "", label: "— elegir —" },
                ...(sedes.data ?? []).map((s) => ({ value: s.id, label: s.nombre })),
              ]}
            />
          </div>
          <div className="w-56">
            <Select
              label="Categoría"
              value={categoriaID}
              onChange={(e) => setCategoriaID(e.target.value)}
              options={[
                { value: "", label: "Toda la sede" },
                ...(categorias.data ?? []).map((c) => ({
                  value: c.id,
                  label: c.padre ? `${c.padre} › ${c.nombre}` : c.nombre,
                })),
              ]}
            />
          </div>
          <div className="w-40">
            <Input label="Fecha" type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-transparent" aria-hidden="true">
              &nbsp;
            </span>
            <Button onClick={abrirHoja} loading={abrir.isPending}>
              Abrir hoja
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* ── Hojas abiertas ──────────────────────────────────────────────── */}
      {hojasAbiertas.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Hojas abiertas</CardTitle>
          </CardHeader>
          <CardContent>
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Hoja</TH>
                    <TH>Sede</TH>
                    <TH>Abierta</TH>
                    <TH className="text-right">Avance</TH>
                    <TH className="text-right">Diferencias</TH>
                    <TH>Estado</TH>
                    <TH className="text-right">Acción</TH>
                  </TR>
                </THead>
                <TBody>
                  {hojasAbiertas.map((h) => (
                    <TR key={h.id}>
                      <TD className="font-medium tabular-nums text-content">
                        {h.numero}
                        {h.categoria && (
                          <span className="block text-xs font-normal text-content-muted">
                            {h.categoria}
                          </span>
                        )}
                      </TD>
                      <TD className="text-content-muted">{h.sede}</TD>
                      <TD className="tabular-nums text-content-muted">
                        {h.abierto_en}
                        {h.abierto_por && (
                          <span className="block text-xs">por {h.abierto_por}</span>
                        )}
                      </TD>
                      <TD className="text-right tabular-nums">
                        {h.contadas} / {h.lineas}
                      </TD>
                      <TD className="text-right tabular-nums">
                        {h.con_diferencia}
                        {h.sin_explicar > 0 && (
                          <span className="block text-xs text-negativo">
                            {h.sin_explicar} sin explicar
                          </span>
                        )}
                      </TD>
                      <TD>
                        {h.puede_cerrarse ? (
                          <Badge tone="positivo">lista para cerrar</Badge>
                        ) : (
                          <Badge tone="pendiente">en proceso</Badge>
                        )}
                      </TD>
                      <TD className="text-right">
                        <Button
                          size="sm"
                          variant={hojaAbierta === h.id ? "secondary" : "primary"}
                          onClick={() => setHojaAbierta(hojaAbierta === h.id ? null : h.id)}
                        >
                          {hojaAbierta === h.id ? "Cerrar vista" : "Contar"}
                        </Button>
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          </CardContent>
        </Card>
      )}

      {hojaAbierta && <HojaDeConteo id={hojaAbierta} onCerrada={() => setHojaAbierta(null)} />}

      {/* ── Conteos cerrados ────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <CardTitle>Conteos cerrados</CardTitle>
        </CardHeader>
        <CardContent>
          {hojasCerradas.length === 0 ? (
            <EmptyState message="Todavía no se cerró ningún conteo." />
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Hoja</TH>
                    <TH>Sede</TH>
                    <TH>Contada</TH>
                    <TH className="text-right">Líneas</TH>
                    <TH className="text-right">Diferencias</TH>
                    <TH>Cerró</TH>
                  </TR>
                </THead>
                <TBody>
                  {hojasCerradas.map((h) => (
                    <TR key={h.id}>
                      <TD className="font-medium tabular-nums text-content">{h.numero}</TD>
                      <TD className="text-content-muted">{h.sede}</TD>
                      <TD className="tabular-nums text-content-muted">{h.cerrado_en}</TD>
                      <TD className="text-right tabular-nums">{h.lineas}</TD>
                      <TD className="text-right tabular-nums">
                        {h.con_diferencia === 0 ? (
                          <span className="text-positivo">cuadró todo</span>
                        ) : (
                          h.con_diferencia
                        )}
                      </TD>
                      <TD className="text-content-muted">{h.cerrado_por || "—"}</TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

/** La hoja: contar cada línea, explicar las diferencias y cerrar. */
function HojaDeConteo({ id, onCerrada }: { id: string; onCerrada: () => void }) {
  const toast = useToast();
  const q = useHojaConteo(id);
  const cerrar = useCerrarConteo();
  const anular = useAnularConteo();
  const [confirmar, setConfirmar] = useState<"cerrar" | "anular" | null>(null);

  const hoja = q.data;
  const filas = hoja?.filas ?? [];

  if (q.isLoading) return <LoadingState label="Cargando la hoja…" />;
  if (q.isError) return <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />;
  if (!hoja) return null;

  const impactoTotal = filas.reduce((a, f) => a + toNumber(f.impacto_crc || "0"), 0);

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          Hoja {hoja.numero} · {hoja.sede}
        </CardTitle>
        <p className="mt-1 text-xs text-content-muted">
          «Sistema» es lo que había al abrir la hoja. Para los cofres la pregunta es si la unidad
          está: 1 = apareció, 0 = no apareció.
        </p>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap gap-6">
          <div>
            <p className="text-xs uppercase tracking-wide text-content-muted">Avance</p>
            <p className="text-xl font-semibold tabular-nums text-content">
              {hoja.contadas} / {hoja.lineas}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-wide text-content-muted">Diferencias</p>
            <p className="text-xl font-semibold tabular-nums text-content">{hoja.con_diferencia}</p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-wide text-content-muted">Sin explicar</p>
            <p
              className={cn(
                "text-xl font-semibold tabular-nums",
                hoja.sin_explicar > 0 ? "text-negativo" : "text-content",
              )}
            >
              {hoja.sin_explicar}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-wide text-content-muted">Impacto</p>
            <p
              className={cn(
                "text-xl font-semibold tabular-nums",
                impactoTotal < 0 ? "text-negativo" : "text-content",
              )}
            >
              {formatMoneda(String(impactoTotal))}
            </p>
          </div>
        </div>

        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Artículo</TH>
                <TH className="text-right">Sistema</TH>
                <TH className="text-right">Contado</TH>
                <TH className="text-right">Diferencia</TH>
                <TH className="text-right">Impacto</TH>
                <TH>Explicación</TH>
                <TH>Estado</TH>
              </TR>
            </THead>
            <TBody>
              {filas.map((f) => (
                <FilaConteo key={f.id} conteoID={id} linea={f} bloqueada={hoja.estado !== "ABIERTO"} />
              ))}
            </TBody>
          </Table>
        </TableContainer>

        {hoja.estado === "ABIERTO" && (
          <div className="flex flex-wrap items-center gap-3 border-t border-border pt-4">
            <Button
              onClick={() => setConfirmar("cerrar")}
              disabled={!hoja.puede_cerrarse}
              loading={cerrar.isPending}
            >
              Cerrar el conteo
            </Button>
            <Button variant="ghost" className="text-negativo" onClick={() => setConfirmar("anular")}>
              Anular
            </Button>
            <span className="text-xs text-content-muted">
              {hoja.puede_cerrarse
                ? "Al cerrar, cada diferencia explicada genera su ajuste con el motivo del conteo."
                : hoja.contadas < hoja.lineas
                  ? `Faltan ${hoja.lineas - hoja.contadas} línea(s) por contar.`
                  : `Faltan ${hoja.sin_explicar} explicación(es).`}
            </span>
          </div>
        )}
      </CardContent>

      {confirmar === "cerrar" && (
        <ConfirmDialog
          titulo={`Cerrar el conteo ${hoja.numero}`}
          descripcion={
            <>
              Se van a generar los ajustes de las <strong>{hoja.con_diferencia} diferencia(s)</strong>{" "}
              explicadas, con el motivo de cada una. Las unidades que no aparecieron quedan dadas de
              baja. Después de cerrar, la hoja no se puede modificar.
            </>
          }
          impacto={[`Impacto en el valor del inventario: ${formatMoneda(String(impactoTotal))}`]}
          textoConfirmar="Cerrar y ajustar"
          tono="peligro"
          pendiente={cerrar.isPending}
          onConfirmar={() =>
            cerrar.mutate(
              { id },
              {
                onSuccess: (r) => {
                  toast.success(
                    `${r.numero} cerrado: ${r.ajustes_que_restan} ajuste(s) que restan, ${r.ajustes_que_suman} que suman, ${r.bajas} baja(s). Impacto ${formatMoneda(r.impacto_crc)}.`,
                  );
                  setConfirmar(null);
                  onCerrada();
                },
                onError: (err) => {
                  toast.error(mensajeError(err));
                  setConfirmar(null);
                },
              },
            )
          }
          onCancelar={() => setConfirmar(null)}
        />
      )}

      {confirmar === "anular" && (
        <ConfirmDialog
          titulo={`Anular el conteo ${hoja.numero}`}
          descripcion="La hoja se descarta y NO se genera ningún ajuste: el sistema queda diciendo lo mismo que antes. Escribí por qué se anula."
          textoConfirmar="Anular"
          tono="peligro"
          pedirNota
          notaPlaceholder="No se pudo terminar de contar…"
          pendiente={anular.isPending}
          onConfirmar={(nota) =>
            anular.mutate(
              { id, motivo: nota },
              {
                onSuccess: () => {
                  toast.success(`${hoja.numero} anulado. No se generó ningún ajuste.`);
                  setConfirmar(null);
                  onCerrada();
                },
                onError: (err) => toast.error(mensajeError(err)),
              },
            )
          }
          onCancelar={() => setConfirmar(null)}
        />
      )}
    </Card>
  );
}

/**
 * Una línea de la hoja.
 *
 * El campo de conteo arranca VACÍO y no en cero: prellenarlo con lo que dice el sistema invita a
 * confirmar sin mirar, que es la forma más rápida de tener un conteo que no cuenta nada.
 */
function FilaConteo({
  conteoID,
  linea,
  bloqueada,
}: {
  conteoID: string;
  linea: ConteoLinea;
  bloqueada: boolean;
}) {
  const toast = useToast();
  const guardar = useGuardarLineaConteo();
  const sinContar = linea.cantidad_contada < 0;
  const [contado, setContado] = useState(sinContar ? "" : String(linea.cantidad_contada));
  const [motivo, setMotivo] = useState(linea.motivo);

  const esUnidad = linea.modo_control === "UNIDAD";
  const marca = LINEA[linea.estado] ?? LINEA.SIN_CONTAR;

  function enviar() {
    if (contado.trim() === "") {
      toast.error("Escribí cuántos contaste (0 si no había ninguno).");
      return;
    }
    guardar.mutate(
      { conteoId: conteoID, lineaId: linea.id, cantidad: Number(contado), motivo },
      { onError: (err) => toast.error(mensajeError(err)) },
    );
  }

  return (
    <TR>
      <TD>
        <span className="font-medium text-content">{linea.articulo}</span>
        <span className="block text-xs text-content-muted">
          {linea.unidad_numero ? `${linea.unidad_numero} · ${linea.categoria}` : linea.categoria}
        </span>
      </TD>
      <TD className="text-right tabular-nums text-content-muted">{linea.cantidad_sistema}</TD>
      <TD className="text-right">
        {bloqueada ? (
          <span className="tabular-nums">{sinContar ? "—" : linea.cantidad_contada}</span>
        ) : (
          <div className="ml-auto w-24">
            <Input
              aria-label={`Contado de ${linea.unidad_numero || linea.articulo}`}
              value={contado}
              onChange={(e) => setContado(e.target.value)}
              onBlur={() => {
                if (contado.trim() !== "" && Number(contado) !== linea.cantidad_contada) enviar();
              }}
              inputMode="numeric"
              placeholder={esUnidad ? "1 ó 0" : "0"}
            />
          </div>
        )}
      </TD>
      <TD
        className={cn(
          "text-right tabular-nums",
          !sinContar && linea.diferencia !== 0 ? "font-medium text-negativo" : "text-content-muted",
        )}
      >
        {sinContar ? "—" : linea.diferencia > 0 ? `+${linea.diferencia}` : linea.diferencia}
      </TD>
      <TD className="text-right tabular-nums text-content-muted">
        {sinContar || linea.diferencia === 0 ? "—" : formatMonto(linea.impacto_crc)}
      </TD>
      <TD>
        {sinContar || linea.diferencia === 0 ? (
          <span className="text-xs text-content-muted">—</span>
        ) : bloqueada ? (
          <span className="text-xs text-content-muted">{linea.motivo}</span>
        ) : (
          <div className="min-w-48">
            <Input
              aria-label={`Explicación de ${linea.unidad_numero || linea.articulo}`}
              value={motivo}
              onChange={(e) => setMotivo(e.target.value)}
              onBlur={() => {
                if (motivo !== linea.motivo && contado.trim() !== "") enviar();
              }}
              placeholder="se rompió, se usó sin registrar…"
            />
          </div>
        )}
      </TD>
      <TD>
        <Badge tone={marca.tono}>{marca.texto}</Badge>
      </TD>
    </TR>
  );
}
