/**
 * Contabilidad → Libros (/conta/libros). FASE 3A.
 *
 * Dos pestañas: el DIARIO —los asientos en orden, con su detalle— y el MAYOR —el movimiento de
 * una cuenta con su saldo corrido—. Son las dos formas de mirar lo mismo y las dos hacen falta:
 * el diario responde «¿qué se registró?» y el mayor «¿qué tiene esta cuenta?».
 *
 * HOY ESTÁN VACÍOS, y es correcto que lo estén: el generador de asientos es la Fase 3B y depende
 * de la matriz de reglas de contabilización, que todavía no está aprobada. Lo que sí se puede
 * hacer es escribir un asiento a mano, que es como se cargan los ajustes y las reclasificaciones.
 */

import { useState } from "react";

import {
  Badge,
  BuscadorMultiple,
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
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { formatMonto, hoyCR, montoParaApi } from "@/lib/format";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useAnularAsiento,
  useAsiento,
  useCrearAsiento,
  useCuentasDeMovimiento,
  useDiario,
  useMayor,
} from "@/features/conta/hooks";
import { calcularCuadre, lineaVacia, queLeFalta } from "@/features/conta/cuadre";
import type { Asiento, NuevaLineaAsiento } from "@/api/conta";

type Pestana = "diario" | "mayor";

export function LibrosPage() {
  const [pestana, setPestana] = useState<Pestana>("diario");
  const puedeEscribir = useTienePermiso()("conta.asientos");

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Libros"
        description="El diario y el mayor: lo que se registró, y qué tiene cada cuenta."
      />

      <div className="flex gap-2 border-b border-border">
        {(
          [
            ["diario", "Libro diario"],
            ["mayor", "Mayor por cuenta"],
          ] as const
        ).map(([id, rotulo]) => (
          <button
            key={id}
            type="button"
            onClick={() => setPestana(id)}
            className={
              pestana === id
                ? "border-b-2 border-accent px-3 py-2 text-sm font-medium text-content"
                : "px-3 py-2 text-sm text-content-muted hover:text-content"
            }
          >
            {rotulo}
          </button>
        ))}
      </div>

      {pestana === "diario" ? <Diario puedeEscribir={puedeEscribir} /> : <MayorPanel />}
    </div>
  );
}

// ── Libro diario ────────────────────────────────────────────────────────────────────────────

function Diario({ puedeEscribir }: { puedeEscribir: boolean }) {
  const toast = useToast();
  const [desde, setDesde] = useState("");
  const [hasta, setHasta] = useState("");
  const [abierto, setAbierto] = useState<string | null>(null);
  const [escribiendo, setEscribiendo] = useState(false);

  const diarioQ = useDiario({ desde, hasta });
  const anular = useAnularAsiento();
  const items = diarioQ.data?.items ?? [];

  function anularAsiento(a: Asiento) {
    const motivo = window.prompt(
      `Anular el asiento ${a.numero}. ¿Por qué? (mínimo 15 caracteres)\n\n` +
        "El asiento NO se borra: queda en el libro, sin efecto, con este motivo.",
    );
    if (motivo === null) return;
    anular.mutate(
      { id: a.id, motivo },
      {
        onSuccess: () => toast.success(`Asiento ${a.numero} anulado.`),
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-6">
      {puedeEscribir && !escribiendo && (
        <div>
          <Button onClick={() => setEscribiendo(true)}>Escribir un asiento</Button>
        </div>
      )}
      {escribiendo && <AsientoManual onCerrar={() => setEscribiendo(false)} />}

      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 pt-5">
          <Input label="Desde" type="date" value={desde} onChange={(e) => setDesde(e.target.value)} />
          <Input label="Hasta" type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} />
          <p className="ml-auto pb-2 text-sm text-content-muted">{items.length} asiento(s)</p>
        </CardContent>
      </Card>

      {diarioQ.isPending ? (
        <LoadingState label="Cargando el diario" />
      ) : diarioQ.isError ? (
        <ErrorState message={mensajeError(diarioQ.error)} onRetry={() => diarioQ.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          message={
            desde || hasta
              ? "No hay asientos en ese rango."
              : "Todavía no hay asientos. El generador automático es el paso siguiente; mientras tanto se pueden escribir a mano."
          }
        />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH className="text-right">Nº</TH>
                <TH>Fecha</TH>
                <TH>Glosa</TH>
                <TH>Origen</TH>
                <TH className="text-right">Monto</TH>
                <TH />
              </TR>
            </THead>
            <TBody>
              {items.map((a) => (
                <FilaAsiento
                  key={a.id}
                  a={a}
                  abierto={abierto === a.id}
                  puedeEscribir={puedeEscribir}
                  onAlternar={() => setAbierto(abierto === a.id ? null : a.id)}
                  onAnular={() => anularAsiento(a)}
                />
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}

function FilaAsiento({
  a,
  abierto,
  puedeEscribir,
  onAlternar,
  onAnular,
}: {
  a: Asiento;
  abierto: boolean;
  puedeEscribir: boolean;
  onAlternar: () => void;
  onAnular: () => void;
}) {
  const detalleQ = useAsiento(abierto ? a.id : null);
  const anulado = a.estado === "ANULADO";
  return (
    <>
      <TR className={anulado ? "opacity-60" : undefined}>
        <TD className="text-right tabular-nums">{a.numero}</TD>
        <TD className="tabular-nums">{a.fecha}</TD>
        <TD className="max-w-md whitespace-normal">
          <p className="text-content">{a.glosa}</p>
          {anulado && (
            // Un asiento anulado que se ve igual que uno vivo es una trampa: sigue en el libro
            // pero no mueve saldo, y nadie puede notar la diferencia sin abrirlo.
            <p className="mt-1 whitespace-normal break-words text-xs text-danger">
              ANULADO — {a.anulado_motivo}
            </p>
          )}
        </TD>
        <TD>
          <Badge tone={a.origen_tipo === "MANUAL" ? "pendiente" : "neutral"}>
            {a.origen_tipo === "MANUAL" ? "a mano" : a.origen_tipo.toLowerCase().replace(/_/g, " ")}
          </Badge>
        </TD>
        <TD className="text-right tabular-nums">{formatMonto(a.debe)}</TD>
        <TD className="text-right">
          <div className="flex justify-end gap-1">
            <Button variant="ghost" size="sm" onClick={onAlternar}>
              {abierto ? "Cerrar" : "Ver"}
            </Button>
            {puedeEscribir && !anulado && (
              <Button variant="ghost" size="sm" onClick={onAnular}>
                Anular
              </Button>
            )}
          </div>
        </TD>
      </TR>
      {abierto && (
        <TR>
          <TD colSpan={6} className="whitespace-normal bg-surface-muted/40">
            {detalleQ.isPending ? (
              <p className="py-2 text-sm text-content-muted">Cargando las líneas…</p>
            ) : (
              <table className="w-full text-sm">
                <tbody>
                  {(detalleQ.data?.lineas ?? []).map((l) => (
                    <tr key={l.id}>
                      <td className="py-1 pr-3 tabular-nums text-content-muted">{l.cuenta_codigo}</td>
                      <td className="py-1 pr-3 text-content">{l.cuenta_nombre}</td>
                      <td className="py-1 pr-3 text-content-muted">{l.detalle}</td>
                      <td className="py-1 pr-3 text-right tabular-nums text-content">
                        {l.debe !== "0.00" ? formatMonto(l.debe) : ""}
                      </td>
                      <td className="py-1 text-right tabular-nums text-content">
                        {l.haber !== "0.00" ? formatMonto(l.haber) : ""}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </TD>
        </TR>
      )}
    </>
  );
}

// ── Asiento a mano ──────────────────────────────────────────────────────────────────────────

function AsientoManual({ onCerrar }: { onCerrar: () => void }) {
  const toast = useToast();
  const crear = useCrearAsiento();
  const cuentasQ = useCuentasDeMovimiento();

  const [fecha, setFecha] = useState(hoyCR());
  const [glosa, setGlosa] = useState("");
  const [lineas, setLineas] = useState<NuevaLineaAsiento[]>([lineaVacia(), lineaVacia()]);

  const cuadre = calcularCuadre(lineas);
  const falta = queLeFalta(fecha, glosa, lineas, cuadre);

  const opciones = (cuentasQ.data ?? []).map((c) => ({
    value: c.id,
    label: `${c.codigo} · ${c.nombre}`,
    grupo: c.clase,
  }));

  function cambiar(i: number, campo: keyof NuevaLineaAsiento, valor: string) {
    setLineas((prev) => prev.map((l, j) => (i === j ? { ...l, [campo]: valor } : l)));
  }

  function guardar() {
    crear.mutate(
      {
        fecha,
        glosa,
        // Los montos van normalizados: lo que se teclea con separadores de miles no es lo que el
        // servidor espera recibir.
        lineas: lineas
          .filter((l) => l.debe || l.haber)
          .map((l) => ({
            ...l,
            debe: montoParaApi(l.debe),
            haber: montoParaApi(l.haber),
          })),
      },
      {
        onSuccess: (a) => {
          toast.success(`Asiento ${a.numero} registrado.`);
          onCerrar();
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Escribir un asiento</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap gap-3">
          <Input label="Fecha" type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} />
          <Input
            label="Glosa"
            className="min-w-[20rem] flex-1"
            value={glosa}
            onChange={(e) => setGlosa(e.target.value)}
            placeholder="Reclasificación de la caja chica de Cartago"
          />
        </div>

        <div className="flex flex-col gap-3">
          {lineas.map((l, i) => (
            <div key={i} className="flex flex-wrap items-end gap-2 border-t border-border pt-3">
              <BuscadorMultiple
                label={`Cuenta ${i + 1}`}
                className="min-w-[18rem] flex-1"
                opciones={opciones}
                seleccion={l.cuenta_id ? [l.cuenta_id] : []}
                onChange={(v) => cambiar(i, "cuenta_id", v.slice(-1)[0] ?? "")}
                placeholder="Código o nombre…"
                leyendaVacio="elegí una cuenta"
              />
              <Input
                label="Debe"
                className="w-32"
                value={l.debe}
                onChange={(e) => cambiar(i, "debe", e.target.value)}
                placeholder="0,00"
              />
              <Input
                label="Haber"
                className="w-32"
                value={l.haber}
                onChange={(e) => cambiar(i, "haber", e.target.value)}
                placeholder="0,00"
              />
              <Input
                label="Detalle"
                className="min-w-[12rem] flex-1"
                value={l.detalle}
                onChange={(e) => cambiar(i, "detalle", e.target.value)}
              />
              {lineas.length > 2 && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setLineas((prev) => prev.filter((_, j) => j !== i))}
                >
                  Quitar
                </Button>
              )}
            </div>
          ))}
          <div>
            <Button variant="ghost" size="sm" onClick={() => setLineas((p) => [...p, lineaVacia()])}>
              Agregar línea
            </Button>
          </div>
        </div>

        {/* El cuadre se ve MIENTRAS se escribe. El servidor lo rechazaría igual, pero entonces
            hay que volver a sumar a mano para encontrar cuál de las líneas está mal. */}
        <div className="flex flex-wrap items-center gap-6 rounded-lg border border-border bg-surface-muted/40 px-4 py-3">
          <div>
            <p className="text-lg font-semibold tabular-nums text-content">{formatMonto(cuadre.debeTexto)}</p>
            <p className="text-xs text-content-muted">debe</p>
          </div>
          <div>
            <p className="text-lg font-semibold tabular-nums text-content">{formatMonto(cuadre.haberTexto)}</p>
            <p className="text-xs text-content-muted">haber</p>
          </div>
          <div className="ml-auto text-right">
            {falta ? (
              <p className="text-sm text-warning">{falta}</p>
            ) : (
              <p className="text-sm text-positive">Cuadra.</p>
            )}
          </div>
        </div>

        <div className="flex gap-2">
          <Button onClick={guardar} disabled={!!falta || crear.isPending}>
            Registrar
          </Button>
          <Button variant="ghost" onClick={onCerrar}>
            Cancelar
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

// ── Mayor ───────────────────────────────────────────────────────────────────────────────────

function MayorPanel() {
  const [sel, setSel] = useState<string[]>([]);
  const [desde, setDesde] = useState("");
  const [hasta, setHasta] = useState("");
  const cuentasQ = useCuentasDeMovimiento();
  const mayorQ = useMayor(sel[0] ?? "", desde, hasta);

  const opciones = (cuentasQ.data ?? []).map((c) => ({
    value: c.id,
    label: `${c.codigo} · ${c.nombre}`,
    grupo: c.clase,
  }));
  const m = mayorQ.data;

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 pt-5">
          <BuscadorMultiple
            label="Cuenta"
            className="min-w-[22rem] flex-1"
            opciones={opciones}
            seleccion={sel}
            onChange={(v) => setSel(v.slice(-1))}
            placeholder="Código o nombre de la cuenta…"
            leyendaVacio="elegí una cuenta"
          />
          <Input label="Desde" type="date" value={desde} onChange={(e) => setDesde(e.target.value)} />
          <Input label="Hasta" type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} />
        </CardContent>
      </Card>

      {sel.length === 0 ? (
        <EmptyState message="Elegí una cuenta para ver su movimiento." />
      ) : mayorQ.isPending ? (
        <LoadingState label="Cargando el mayor" />
      ) : mayorQ.isError ? (
        <ErrorState message={mensajeError(mayorQ.error)} onRetry={() => mayorQ.refetch()} />
      ) : m ? (
        <>
          <Card>
            <CardContent className="flex flex-wrap items-end gap-8 pt-5">
              <div>
                <p className="tabular-nums text-content">{m.cuenta.codigo}</p>
                <p className="text-sm text-content-muted">
                  {m.cuenta.nombre} · {m.cuenta.clase} · {m.cuenta.naturaleza.toLowerCase()}
                </p>
              </div>
              <div className="ml-auto text-right">
                <p className="text-2xl font-semibold tabular-nums text-content">
                  {formatMonto(m.saldo_final)}
                </p>
                {/* Decirlo evita que alguien lea un saldo acreedor como si fuera deudor: el signo
                    sigue a la naturaleza de la cuenta, no a debe-menos-haber. */}
                <p className="text-xs text-content-muted">
                  saldo {m.cuenta.naturaleza === "ACREEDORA" ? "acreedor" : "deudor"}
                </p>
              </div>
            </CardContent>
          </Card>

          {m.renglones.length === 0 ? (
            <EmptyState message="Esta cuenta no tiene movimiento en el rango." />
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH className="text-right">Nº</TH>
                    <TH>Fecha</TH>
                    <TH>Glosa</TH>
                    <TH className="text-right">Debe</TH>
                    <TH className="text-right">Haber</TH>
                    <TH className="text-right">Saldo</TH>
                  </TR>
                </THead>
                <TBody>
                  <TR>
                    <TD />
                    <TD />
                    <TD className="text-content-muted">Saldo anterior</TD>
                    <TD />
                    <TD />
                    <TD className="text-right tabular-nums text-content-muted">
                      {formatMonto(m.saldo_inicial)}
                    </TD>
                  </TR>
                  {m.renglones.map((g, i) => (
                    <TR key={`${g.asiento_id}-${i}`}>
                      <TD className="text-right tabular-nums">{g.numero}</TD>
                      <TD className="tabular-nums">{g.fecha}</TD>
                      <TD className="max-w-md whitespace-normal">
                        {g.glosa}
                        {g.detalle && (
                          <span className="block text-xs text-content-muted">{g.detalle}</span>
                        )}
                      </TD>
                      <TD className="text-right tabular-nums">
                        {g.debe !== "0.00" ? formatMonto(g.debe) : ""}
                      </TD>
                      <TD className="text-right tabular-nums">
                        {g.haber !== "0.00" ? formatMonto(g.haber) : ""}
                      </TD>
                      <TD className="text-right tabular-nums font-medium">{formatMonto(g.saldo)}</TD>
                    </TR>
                  ))}
                  <TR>
                    <TD />
                    <TD />
                    <TD className="font-medium text-content">Totales</TD>
                    <TD className="text-right tabular-nums font-medium">{formatMonto(m.total_debe)}</TD>
                    <TD className="text-right tabular-nums font-medium">{formatMonto(m.total_haber)}</TD>
                    <TD className="text-right tabular-nums font-medium">{formatMonto(m.saldo_final)}</TD>
                  </TR>
                </TBody>
              </Table>
            </TableContainer>
          )}
        </>
      ) : null}
    </div>
  );
}
