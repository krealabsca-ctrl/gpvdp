/**
 * Pantalla — Saldos diarios (/saldos-diarios), Tanda 1 (maqueta aprobada 2026-07-30).
 *
 * El flujo real de la empresa: la tesorera captura TODOS LOS DÍAS el saldo que le muestra el
 * banco, para saber con cuánto se cuenta. El sistema calcula el saldo que espera según los
 * movimientos ya cargados y compara: si no cuadra, o faltan movimientos o hubo un error de
 * digitación. Ese es el control de completitud que el módulo no tenía.
 *
 * Tres vistas: captura del día · posición (disponible por moneda y banco) · checklist de carga.
 * Dirección Financiera congela el día revisado; congelado, la captura queda en lectura.
 */

import { useEffect, useMemo, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
  type BadgeTone,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { etiquetaPeriodo, formatMoneda, hoyCR, periodoActual, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useCargaPeriodo,
  useGuardarSaldos,
  useRevisarSaldos,
  useTesoreria,
} from "@/features/bancos/hooksTesoreria";
import type { Cuadre, EstadoCarga, SaldoDelDia, SaldoInput } from "@/api/bancos";

type Vista = "captura" | "posicion" | "carga";

const VISTAS: { id: Vista; label: string; desc: string }[] = [
  { id: "captura", label: "Saldos del día", desc: "Capturar lo que dice el banco" },
  { id: "posicion", label: "Posición", desc: "Con cuánto contamos hoy" },
  { id: "carga", label: "Checklist de carga", desc: "¿El mes está completo?" },
];

const TONO_CUADRE: Record<Cuadre, BadgeTone> = {
  CUADRA: "positivo",
  DIFIERE: "negativo",
  SIN_CAPTURA: "neutral",
  SIN_ANTERIOR: "accent",
};

const ETIQUETA_CUADRE: Record<Cuadre, string> = {
  CUADRA: "Cuadra",
  DIFIERE: "Difiere",
  SIN_CAPTURA: "Sin capturar",
  SIN_ANTERIOR: "Primer día",
};

const TONO_CARGA: Record<EstadoCarga, BadgeTone> = {
  AL_DIA: "positivo",
  ATRASADA: "pendiente",
  REZAGADA: "negativo",
  SIN_CARGA: "neutral",
};

const ETIQUETA_CARGA: Record<EstadoCarga, string> = {
  AL_DIA: "Al día",
  ATRASADA: "Atrasada",
  REZAGADA: "Rezagada",
  SIN_CARGA: "Sin carga",
};

/** Un saldo se escribe con punto decimal; se limpian espacios, comas y símbolos. */
function normalizarMonto(texto: string): string {
  return texto.replace(/[^\d.,-]/g, "").replace(/,/g, "");
}

export function SaldosDiariosPage() {
  const toast = useToast();
  const tienePermiso = useTienePermiso();
  const puedeCapturar = tienePermiso("bancos.saldos");
  const puedeRevisar = tienePermiso("bancos.saldos_revisar");

  const [vista, setVista] = useState<Vista>("captura");
  const [fecha, setFecha] = useState(hoyCR());
  const [periodo, setPeriodo] = useState(periodoActual());
  // Borrador de la captura: cuenta_id -> texto tal como lo escribe la tesorera.
  const [borrador, setBorrador] = useState<Record<string, string>>({});
  const [notas, setNotas] = useState<Record<string, string>>({});
  const [confirmar, setConfirmar] = useState<null | "congelar" | "descongelar">(null);

  const tesQ = useTesoreria(fecha);
  const cargaQ = useCargaPeriodo(periodo);
  const guardar = useGuardarSaldos();
  const revisar = useRevisarSaldos();

  const t = tesQ.data;

  // Al cambiar de fecha (o llegar datos nuevos) el borrador arranca de lo capturado.
  useEffect(() => {
    if (!t) return;
    const inicial: Record<string, string> = {};
    const notasIniciales: Record<string, string> = {};
    for (const s of t.saldos) {
      inicial[s.cuenta_id] = s.saldo;
      notasIniciales[s.cuenta_id] = s.nota;
    }
    setBorrador(inicial);
    setNotas(notasIniciales);
  }, [t]);

  const cambiados = useMemo(() => {
    if (!t) return [] as SaldoInput[];
    const out: SaldoInput[] = [];
    for (const s of t.saldos) {
      if (s.congelado) continue; // un saldo revisado no se reescribe
      const valor = normalizarMonto(borrador[s.cuenta_id] ?? "");
      const nota = notas[s.cuenta_id] ?? "";
      if (valor === "" ) continue;
      if (valor !== s.saldo || nota !== s.nota) out.push({ cuenta_id: s.cuenta_id, saldo: valor, nota });
    }
    return out;
  }, [t, borrador, notas]);

  function guardarCaptura() {
    if (cambiados.length === 0) return;
    guardar.mutate(
      { fecha, saldos: cambiados },
      {
        onSuccess: (r) =>
          toast.success(
            r.guardados === 1 ? "Se guardó 1 saldo." : `Se guardaron ${r.guardados} saldos del día.`,
          ),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function aplicarRevision(congelar: boolean, motivo: string) {
    revisar.mutate(
      { fecha, congelar, motivo: motivo.trim() || (congelar ? "Revisión de Dirección Financiera" : "") },
      {
        onSuccess: (r) =>
          toast.success(
            congelar
              ? `Día congelado: ${r.cuentas} saldo(s) quedan en firme.`
              : `Día descongelado: ${r.cuentas} saldo(s) vuelven a ser editables (queda en auditoría).`,
          ),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
    setConfirmar(null);
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Saldos diarios"
        description="El saldo que dice el banco, contra el que dicen los movimientos cargados. Si no cuadra, algo falta."
      />

      {/* Selector de vista + fecha del día */}
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-wrap gap-2" role="tablist" aria-label="Vistas de tesorería">
          {VISTAS.map((v) => (
            <button
              key={v.id}
              type="button"
              role="tab"
              aria-selected={vista === v.id}
              onClick={() => setVista(v.id)}
              className={cn(
                "rounded-lg border px-3 py-2 text-left transition",
                vista === v.id
                  ? "border-accent bg-accent/10 text-content"
                  : "border-border bg-surface text-content-muted hover:bg-surface-muted",
              )}
            >
              <span className="block text-sm font-medium">{v.label}</span>
              <span className="block text-xs text-content-muted">{v.desc}</span>
            </button>
          ))}
        </div>
        {vista === "carga" ? (
          <label className="flex flex-col gap-1 text-xs text-content-muted">
            Período
            <Input type="month" value={periodo} onChange={(e) => setPeriodo(e.target.value)} className="w-40" />
          </label>
        ) : (
          <div className="flex items-end gap-2">
            <label className="flex flex-col gap-1 text-xs text-content-muted">
              Día
              <Input type="date" value={fecha} onChange={(e) => setFecha(e.target.value)} className="w-40" />
            </label>
            {t && fecha !== t.hoy && (
              <Button variant="secondary" onClick={() => setFecha(t.hoy)}>
                Ir a hoy
              </Button>
            )}
          </div>
        )}
      </div>

      {vista === "carga" ? (
        <CargaDelMes periodo={periodo} query={cargaQ} />
      ) : tesQ.isPending ? (
        <LoadingState label="Cargando la posición del día…" />
      ) : tesQ.isError || !t ? (
        <ErrorState message={mensajeError(tesQ.error)} onRetry={() => void tesQ.refetch()} />
      ) : vista === "captura" ? (
        <>
          <ResumenDia t={t} />
          <Card>
            <CardHeader className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <CardTitle>Saldos del {fecha}</CardTitle>
                <p className="mt-0.5 text-xs text-content-muted">
                  Escribí el saldo que muestra el banco. El esperado lo calcula el sistema con los
                  movimientos ya cargados; la diferencia señala lo que falta.
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {t.dia_revisado && <Badge tone="accent">Día revisado y congelado</Badge>}
                {puedeRevisar && t.congeladas < t.cuentas - t.sin_capturar && (
                  <Button variant="secondary" onClick={() => setConfirmar("congelar")} disabled={revisar.isPending}>
                    Congelar el día
                  </Button>
                )}
                {puedeRevisar && t.congeladas > 0 && (
                  <Button variant="secondary" onClick={() => setConfirmar("descongelar")} disabled={revisar.isPending}>
                    Descongelar
                  </Button>
                )}
                {puedeCapturar && (
                  <Button onClick={guardarCaptura} disabled={cambiados.length === 0 || guardar.isPending}>
                    {cambiados.length === 0
                      ? "Sin cambios"
                      : `Guardar ${cambiados.length} saldo${cambiados.length === 1 ? "" : "s"}`}
                  </Button>
                )}
              </div>
            </CardHeader>
            <CardContent>
              <TablaCaptura
                saldos={t.saldos}
                borrador={borrador}
                notas={notas}
                editable={puedeCapturar}
                onSaldo={(id, v) => setBorrador((b) => ({ ...b, [id]: v }))}
                onNota={(id, v) => setNotas((n) => ({ ...n, [id]: v }))}
              />
            </CardContent>
          </Card>
        </>
      ) : (
        <Posicion t={t} />
      )}

      {confirmar !== null && (
        <ConfirmDialog
          titulo={confirmar === "congelar" ? "Congelar los saldos del día" : "Descongelar los saldos del día"}
          descripcion={
            confirmar === "congelar"
              ? `Los saldos capturados del ${fecha} quedan en firme: nadie los podrá editar sin descongelarlos primero.`
              : `Los saldos del ${fecha} volverán a ser editables.`
          }
          impacto={
            confirmar === "congelar"
              ? ["Se registra quién revisó el día", "La captura queda en modo lectura"]
              : ["Queda en la auditoría con tu usuario y el motivo"]
          }
          textoConfirmar={confirmar === "congelar" ? "Congelar" : "Descongelar"}
          tono={confirmar === "congelar" ? "accent" : "peligro"}
          pendiente={revisar.isPending}
          pedirNota={confirmar === "descongelar"}
          notaPlaceholder="¿Por qué hay que corregir un saldo ya revisado?"
          onConfirmar={(nota) => aplicarRevision(confirmar === "congelar", nota)}
          onCancelar={() => setConfirmar(null)}
        />
      )}
    </div>
  );
}

/** Tarjetas del día: disponible, cuántas faltan y cuántas no cuadran. */
function ResumenDia({ t }: { t: NonNullable<ReturnType<typeof useTesoreria>["data"]> }) {
  const crc = t.totales.find((m) => m.moneda === "CRC");
  const usd = t.totales.find((m) => m.moneda === "USD");
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <Kpi
        titulo="Disponible en colones"
        valor={formatMoneda(crc?.monto ?? "0", "CRC")}
        nota={`${crc?.capturadas ?? 0} de ${crc?.cuentas ?? 0} cuentas capturadas`}
      />
      <Kpi
        titulo="Disponible en dólares"
        valor={formatMoneda(usd?.monto ?? "0", "USD")}
        nota={`${usd?.capturadas ?? 0} de ${usd?.cuentas ?? 0} cuentas capturadas`}
      />
      <Kpi
        titulo="Faltan por capturar"
        valor={String(t.sin_capturar)}
        nota={t.sin_capturar === 0 ? "El día está completo" : `de ${t.cuentas} cuentas activas`}
        tono={t.sin_capturar > 0 ? "pendiente" : "positivo"}
      />
      <Kpi
        titulo="No cuadran"
        valor={String(t.no_cuadran)}
        nota={t.no_cuadran === 0 ? "Todo conciliado con los movimientos" : "Faltan movimientos o hay error de captura"}
        tono={t.no_cuadran > 0 ? "negativo" : "positivo"}
      />
    </div>
  );
}

function Kpi({
  titulo,
  valor,
  nota,
  tono,
}: {
  titulo: string;
  valor: string;
  nota: string;
  tono?: "positivo" | "negativo" | "pendiente";
}) {
  return (
    <Card>
      <CardContent className="py-4">
        <p className="text-xs uppercase tracking-wide text-content-muted">{titulo}</p>
        <p
          className={cn(
            "mt-1 text-2xl font-semibold tabular-nums",
            tono === "negativo" && "text-negativo",
            tono === "positivo" && "text-positivo",
            tono === "pendiente" && "text-pendiente",
          )}
        >
          {valor}
        </p>
        <p className="mt-1 text-xs text-content-muted">{nota}</p>
      </CardContent>
    </Card>
  );
}

function TablaCaptura({
  saldos,
  borrador,
  notas,
  editable,
  onSaldo,
  onNota,
}: {
  saldos: SaldoDelDia[];
  borrador: Record<string, string>;
  notas: Record<string, string>;
  editable: boolean;
  onSaldo: (cuentaId: string, valor: string) => void;
  onNota: (cuentaId: string, valor: string) => void;
}) {
  return (
    <TableContainer>
      <Table>
        <THead>
          <TR>
            <TH>Cuenta</TH>
            <TH className="text-right">Saldo anterior</TH>
            <TH className="text-right">Movimientos del día</TH>
            <TH className="text-right">Esperado</TH>
            <TH className="text-right">Saldo del banco</TH>
            <TH className="text-right">Diferencia</TH>
            <TH>Cuadre</TH>
            <TH>Nota</TH>
          </TR>
        </THead>
        <TBody>
          {saldos.map((s) => {
            const moneda = s.moneda === "USD" ? "USD" : "CRC";
            const bloqueado = !editable || s.congelado;
            return (
              <TR key={s.cuenta_id}>
                <TD>
                  <span className="block font-medium">{s.alias}</span>
                  <span className="block text-xs text-content-muted">
                    {s.banco} · {s.moneda}
                    {s.dias_sin_cargar > 7 && (
                      <> · movimientos hasta {s.ultimo_movimiento || "—"} ({s.dias_sin_cargar} d)</>
                    )}
                  </span>
                </TD>
                <TD className="text-right tabular-nums">
                  {s.saldo_anterior ? (
                    <>
                      {formatMoneda(s.saldo_anterior, moneda)}
                      <span className="block text-xs text-content-muted">{s.fecha_anterior}</span>
                    </>
                  ) : (
                    <span className="text-content-muted">—</span>
                  )}
                </TD>
                <TD className="text-right tabular-nums text-xs">
                  <span className="text-positivo">+{formatMoneda(s.entradas_dia, moneda)}</span>
                  <span className="block text-negativo">−{formatMoneda(s.salidas_dia, moneda)}</span>
                </TD>
                <TD className="text-right tabular-nums">
                  {s.saldo_esperado ? formatMoneda(s.saldo_esperado, moneda) : <span className="text-content-muted">—</span>}
                </TD>
                <TD className="text-right">
                  <Input
                    inputMode="decimal"
                    value={borrador[s.cuenta_id] ?? ""}
                    onChange={(e) => onSaldo(s.cuenta_id, e.target.value)}
                    disabled={bloqueado}
                    placeholder="0.00"
                    className="w-36 text-right tabular-nums"
                    aria-label={`Saldo de ${s.alias}`}
                  />
                  {s.congelado && (
                    <span className="mt-1 block text-xs text-content-muted">
                      Congelado {s.revisado_en.slice(0, 10)}
                    </span>
                  )}
                </TD>
                <TD
                  className={cn(
                    "text-right tabular-nums",
                    s.cuadre === "DIFIERE" && "font-semibold text-negativo",
                  )}
                >
                  {s.diferencia ? formatMoneda(s.diferencia, moneda) : <span className="text-content-muted">—</span>}
                </TD>
                <TD>
                  <Badge tone={TONO_CUADRE[s.cuadre]}>{ETIQUETA_CUADRE[s.cuadre]}</Badge>
                </TD>
                <TD>
                  <Input
                    value={notas[s.cuenta_id] ?? ""}
                    onChange={(e) => onNota(s.cuenta_id, e.target.value)}
                    disabled={bloqueado}
                    placeholder="Opcional"
                    className="w-44"
                    aria-label={`Nota de ${s.alias}`}
                  />
                </TD>
              </TR>
            );
          })}
        </TBody>
      </Table>
    </TableContainer>
  );
}

/** Posición: disponible por moneda, concentración por banco y la serie de 7 días. */
function Posicion({ t }: { t: NonNullable<ReturnType<typeof useTesoreria>["data"]> }) {
  const maxSerie = Math.max(...t.serie.map((p) => toNumber(p.monto_crc)), 1);
  return (
    <div className="flex flex-col gap-4">
      <ResumenDia t={t} />

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Concentración por banco</CardTitle>
            <p className="mt-0.5 text-xs text-content-muted">
              Cuánto del disponible está en cada banco. Las monedas no se suman entre sí.
            </p>
          </CardHeader>
          <CardContent>
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Banco</TH>
                    <TH className="text-right">Colones</TH>
                    <TH className="text-right">Dólares</TH>
                    <TH className="text-right">Cuentas</TH>
                  </TR>
                </THead>
                <TBody>
                  {t.bancos.map((b) => (
                    <TR key={b.banco}>
                      <TD className="font-medium">{b.banco}</TD>
                      <TD className="text-right tabular-nums">{formatMoneda(b.monto_crc, "CRC")}</TD>
                      <TD className="text-right tabular-nums">
                        {toNumber(b.monto_usd) === 0 ? "—" : formatMoneda(b.monto_usd, "USD")}
                      </TD>
                      <TD className="text-right tabular-nums">
                        {b.cuentas}
                        {b.sin_capturar > 0 && (
                          <span className="block text-xs text-pendiente">{b.sin_capturar} sin capturar</span>
                        )}
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </TableContainer>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Disponible en colones, últimos 7 días</CardTitle>
            <p className="mt-0.5 text-xs text-content-muted">
              Solo días con captura: la serie no inventa saldos.
            </p>
          </CardHeader>
          <CardContent>
            <ul className="flex flex-col gap-2">
              {t.serie.map((p) => {
                const monto = toNumber(p.monto_crc);
                return (
                  <li key={p.fecha} className="flex items-center gap-3 text-sm">
                    <span className={cn("w-24 shrink-0 tabular-nums", p.es_hoy && "font-semibold")}>{p.fecha}</span>
                    <span className="h-2 flex-1 overflow-hidden rounded-full bg-surface-muted">
                      <span
                        className={cn("block h-full rounded-full", p.es_hoy ? "bg-accent" : "bg-accent/50")}
                        style={{ width: `${Math.max((monto / maxSerie) * 100, monto > 0 ? 2 : 0)}%` }}
                      />
                    </span>
                    <span className="w-36 shrink-0 text-right tabular-nums">
                      {p.capturadas === 0 ? (
                        <span className="text-content-muted">sin captura</span>
                      ) : (
                        formatMoneda(p.monto_crc, "CRC")
                      )}
                    </span>
                  </li>
                );
              })}
            </ul>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

/** Checklist de carga: qué cuenta tiene el mes completo y cuál quedó rezagada. */
function CargaDelMes({
  periodo,
  query,
}: {
  periodo: string;
  query: ReturnType<typeof useCargaPeriodo>;
}) {
  if (query.isPending) return <LoadingState label="Revisando la carga del mes…" />;
  if (query.isError || !query.data) {
    return <ErrorState message={mensajeError(query.error)} onRetry={() => void query.refetch()} />;
  }
  const filas = query.data;
  const pendientes = filas.filter((c) => c.estado !== "AL_DIA").length;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Carga de estados de cuenta · {etiquetaPeriodo(periodo)}</CardTitle>
        <p className="mt-0.5 text-xs text-content-muted">
          {pendientes === 0
            ? "Todas las cuentas están al día."
            : `${pendientes} de ${filas.length} cuentas no están al día: el mes todavía no está completo.`}
        </p>
      </CardHeader>
      <CardContent>
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Cuenta</TH>
                <TH className="text-right">Movimientos del mes</TH>
                <TH>Último movimiento</TH>
                <TH className="text-right">Días sin cargar</TH>
                <TH>Estado</TH>
              </TR>
            </THead>
            <TBody>
              {filas.map((c) => (
                <TR key={c.cuenta_id}>
                  <TD>
                    <span className="block font-medium">{c.alias}</span>
                    <span className="block text-xs text-content-muted">
                      {c.banco} · {c.moneda}
                    </span>
                  </TD>
                  <TD className="text-right tabular-nums">{c.movimientos}</TD>
                  <TD>{c.ultimo_movimiento || <span className="text-content-muted">nunca</span>}</TD>
                  <TD className="text-right tabular-nums">{c.movimientos === 0 ? "—" : c.dias_sin_cargar}</TD>
                  <TD>
                    <Badge tone={TONO_CARGA[c.estado]}>{ETIQUETA_CARGA[c.estado]}</Badge>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      </CardContent>
    </Card>
  );
}
