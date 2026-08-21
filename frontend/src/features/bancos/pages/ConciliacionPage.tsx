/**
 * Pantalla — Conciliación bancaria (/conciliacion), Tanda 1.
 *
 * Es a la vez pantalla de control y puerta del cierre (decisión del 2026-07-31: el acta es
 * documento imprimible/firmable Y control interno; el período no cierra con nada sin explicar).
 *
 * Por cada cuenta se enfrentan dos saldos: el del estado de cuenta (el capturado el último día
 * del mes) y el de libros (saldo inicial + movimientos cargados). La diferencia se explica con
 * partidas en tránsito; cuando llega a cero, el acta se puede firmar. El mes cierra solo cuando
 * TODAS las cuentas tienen su acta firmada.
 */

import { useMemo, useState } from "react";
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
  Select,
  useToast,
  type BadgeTone,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { etiquetaPeriodo, formatMoneda, montoParaApi, toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import { useAuth } from "@/features/auth/AuthContext";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useAnularPartida,
  useConciliacion,
  useFirmarActa,
  useRegistrarPartida,
} from "@/features/bancos/hooksTesoreria";
import { abrirActaImprimible } from "@/features/bancos/components/ActaImprimible";
import type { ActaConciliacion, TipoPartida } from "@/api/bancos";

/** Los cuatro tipos con signo fijo + «otra». El signo lo impone el backend. */
const TIPOS: { id: TipoPartida; label: string; efecto: string }[] = [
  {
    id: "DEPOSITO_NO_ACREDITADO",
    label: "Depósito sin acreditar",
    efecto: "Libros ya lo tiene y el banco todavía no · suma",
  },
  {
    id: "TRANSFERENCIA_NO_PRESENTADA",
    label: "Transferencia no debitada",
    efecto: "Se giró y el banco no la debitó · resta",
  },
  {
    id: "CARGO_BANCO_NO_REGISTRADO",
    label: "Cargo del banco no registrado",
    efecto: "Comisión o cargo que libros no registró · suma",
  },
  {
    id: "ABONO_BANCO_NO_REGISTRADO",
    label: "Abono del banco no registrado",
    efecto: "Intereses o abono que libros no registró · resta",
  },
  { id: "OTRA", label: "Otra", efecto: "Indicá si suma o resta" },
];

const ETIQUETA_TIPO: Record<TipoPartida, string> = Object.fromEntries(
  TIPOS.map((t) => [t.id, t.label]),
) as Record<TipoPartida, string>;

const ETIQUETA_IMPEDIMENTO: Record<string, string> = {
  SIN_SALDO_BANCO: "Falta capturar el saldo de cierre del mes en Saldos diarios",
  SIN_SALDO_INICIAL: "Falta el saldo de apertura: capturá el saldo del último día del mes anterior",
};

/** Estado visible de un acta, en el orden en que hay que atenderlas. */
type EstadoActa = "FIRMADA" | "CUADRA" | "DIFIERE" | "INCOMPLETA";

function estadoDeActa(a: ActaConciliacion): EstadoActa {
  if (a.firmado_en) return "FIRMADA";
  if (a.impedimento) return "INCOMPLETA";
  return a.cuadra ? "CUADRA" : "DIFIERE";
}

const TONO_ESTADO: Record<EstadoActa, BadgeTone> = {
  FIRMADA: "positivo",
  CUADRA: "accent",
  DIFIERE: "negativo",
  INCOMPLETA: "neutral",
};

const ETIQUETA_ESTADO: Record<EstadoActa, string> = {
  FIRMADA: "Firmada",
  CUADRA: "Lista para firmar",
  DIFIERE: "Con diferencia",
  INCOMPLETA: "Faltan datos",
};

const ORDEN: Record<EstadoActa, number> = { DIFIERE: 0, CUADRA: 1, INCOMPLETA: 2, FIRMADA: 3 };

export function ConciliacionPage() {
  const toast = useToast();
  const { periodo } = usePeriodoActivo();
  const { empresaActiva } = useAuth();
  const tienePermiso = useTienePermiso();
  const puedeConciliar = tienePermiso("bancos.conciliar");

  const [abierta, setAbierta] = useState<string | null>(null);
  const [porFirmar, setPorFirmar] = useState<ActaConciliacion | null>(null);

  const concQ = useConciliacion(periodo);
  const firmar = useFirmarActa();

  const c = concQ.data;
  const actas = useMemo(() => {
    if (!c) return [];
    return [...c.actas].sort((x, y) => {
      const d = ORDEN[estadoDeActa(x)] - ORDEN[estadoDeActa(y)];
      return d !== 0 ? d : x.alias.localeCompare(y.alias);
    });
  }, [c]);

  function imprimir(a: ActaConciliacion) {
    if (!abrirActaImprimible(a, empresaActiva?.nombre ?? "", periodo)) {
      toast.error("El navegador bloqueó la ventana; permitile abrir ventanas emergentes a este sitio.");
    }
  }

  function confirmarFirma() {
    if (!porFirmar) return;
    const a = porFirmar;
    setPorFirmar(null);
    firmar.mutate(
      { cuentaId: a.cuenta_id, periodo },
      {
        onSuccess: () => toast.success(`Acta de ${a.alias} firmada.`),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Actas de conciliación"
        description={`Lo que dice el banco contra lo que dicen los libros, cuenta por cuenta, en ${etiquetaPeriodo(periodo)}.`}
      />

      {concQ.isPending ? (
        <LoadingState label="Armando las actas del mes…" />
      ) : concQ.isError || !c ? (
        <ErrorState message={mensajeError(concQ.error)} onRetry={() => void concQ.refetch()} />
      ) : (
        <>
          {/* Semáforo del mes: qué falta para poder cerrar */}
          <Card>
            <CardContent className="flex flex-wrap items-center justify-between gap-4 py-4">
              <div className="flex flex-wrap gap-x-8 gap-y-3">
                <Cifra titulo="Cuentas" valor={c.cuentas} />
                <Cifra titulo="Firmadas" valor={c.firmadas} tono={c.firmadas === c.cuentas ? "positivo" : undefined} />
                <Cifra titulo="Listas para firmar" valor={c.cuadran} tono={c.cuadran > 0 ? "info" : undefined} />
                <Cifra titulo="Con diferencia" valor={c.con_diferencia} tono={c.con_diferencia > 0 ? "negativo" : undefined} />
                <Cifra titulo="Faltan datos" valor={c.incompletas} tono={c.incompletas > 0 ? "pendiente" : undefined} />
              </div>
              <div className="max-w-md text-sm">
                {c.cerrado ? (
                  <p className="text-content-muted">
                    El período está <b className="text-content">cerrado</b>: las actas son historia y no se modifican.
                  </p>
                ) : c.puede_cerrar ? (
                  <p className="text-positivo">
                    Todas las cuentas están conciliadas y firmadas: el período se puede cerrar.
                  </p>
                ) : (
                  <p className="text-content-muted">
                    Faltan <b className="text-content">{c.cuentas - c.firmadas}</b> acta(s) por firmar. Mientras haya
                    algo sin explicar, el período no cierra.
                  </p>
                )}
              </div>
            </CardContent>
          </Card>

          <div className="flex flex-col gap-3">
            {actas.map((a) => (
              <FilaActa
                key={a.cuenta_id}
                acta={a}
                periodo={periodo}
                abierta={abierta === a.cuenta_id}
                editable={puedeConciliar && !c.cerrado && !a.firmado_en}
                puedeFirmar={puedeConciliar && !c.cerrado}
                onAbrir={() => setAbierta(abierta === a.cuenta_id ? null : a.cuenta_id)}
                onImprimir={() => imprimir(a)}
                onFirmar={() => setPorFirmar(a)}
              />
            ))}
          </div>
        </>
      )}

      {porFirmar !== null && (
        <ConfirmDialog
          titulo={`Firmar el acta de ${porFirmar.alias}`}
          descripcion={
            "Al firmar declarás que la diferencia entre el estado de cuenta y los libros queda " +
            "explicada por completo."
          }
          impacto={[
            "El acta guarda las cifras del momento de la firma",
            "Ya no admite partidas nuevas ni anulaciones",
            "Habilita el cierre del período cuando todas estén firmadas",
          ]}
          textoConfirmar="Firmar el acta"
          pendiente={firmar.isPending}
          onConfirmar={confirmarFirma}
          onCancelar={() => setPorFirmar(null)}
        />
      )}
    </div>
  );
}

function Cifra({
  titulo,
  valor,
  tono,
}: {
  titulo: string;
  valor: number;
  tono?: "positivo" | "negativo" | "pendiente" | "info";
}) {
  return (
    <div>
      <p className="text-xs uppercase tracking-wide text-content-muted">{titulo}</p>
      <p
        className={cn(
          "text-2xl font-semibold tabular-nums",
          tono === "negativo" && "text-negativo",
          tono === "positivo" && "text-positivo",
          tono === "pendiente" && "text-pendiente",
          tono === "info" && "text-accent",
        )}
      >
        {valor}
      </p>
    </div>
  );
}

function FilaActa({
  acta,
  periodo,
  abierta,
  editable,
  puedeFirmar,
  onAbrir,
  onImprimir,
  onFirmar,
}: {
  acta: ActaConciliacion;
  periodo: string;
  abierta: boolean;
  editable: boolean;
  puedeFirmar: boolean;
  onAbrir: () => void;
  onImprimir: () => void;
  onFirmar: () => void;
}) {
  const estado = estadoDeActa(acta);
  const moneda = acta.moneda === "USD" ? "USD" : "CRC";
  const dif = toNumber(acta.diferencia_sin_explicar);

  return (
    <Card>
      <CardHeader className="flex flex-wrap items-center justify-between gap-3">
        <button type="button" onClick={onAbrir} className="min-w-0 flex-1 text-left" aria-expanded={abierta}>
          <div className="flex flex-wrap items-center gap-2">
            <CardTitle className="truncate">{acta.alias}</CardTitle>
            <Badge tone={TONO_ESTADO[estado]}>{ETIQUETA_ESTADO[estado]}</Badge>
            <span className="text-xs text-content-muted">
              {acta.banco} · {acta.moneda}
            </span>
          </div>
          <p className="mt-0.5 text-xs text-content-muted">
            {acta.impedimento ? (
              ETIQUETA_IMPEDIMENTO[acta.impedimento]
            ) : (
              <>
                Banco {formatMoneda(acta.saldo_banco, moneda)} · Libros{" "}
                {formatMoneda(acta.saldo_libros, moneda)} ·{" "}
                <span className={cn(dif !== 0 && "font-semibold text-negativo")}>
                  {dif === 0 ? "sin diferencia" : `diferencia ${formatMoneda(acta.diferencia_sin_explicar, moneda)}`}
                </span>
              </>
            )}
          </p>
        </button>
        <div className="flex flex-wrap items-center gap-2">
          {!acta.impedimento && (
            <Button variant="secondary" onClick={onImprimir}>
              Imprimir acta
            </Button>
          )}
          {puedeFirmar && !acta.firmado_en && acta.cuadra && !acta.impedimento && (
            <Button onClick={onFirmar}>Firmar</Button>
          )}
          <Button variant="ghost" onClick={onAbrir}>
            {abierta ? "Cerrar" : "Detalle"}
          </Button>
        </div>
      </CardHeader>

      {abierta && (
        <CardContent className="flex flex-col gap-4 border-t border-border pt-4">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div className="rounded-lg border border-border bg-surface-muted p-3">
              <p className="text-xs uppercase tracking-wide text-content-muted">Según el estado de cuenta</p>
              <p className="mt-1 text-xl font-semibold tabular-nums">{formatMoneda(acta.saldo_banco || "0", moneda)}</p>
              <p className="mt-1 text-xs text-content-muted">
                Saldo capturado el {acta.fecha_banco || "—"}
              </p>
            </div>
            <div className="rounded-lg border border-border bg-surface-muted p-3">
              <p className="text-xs uppercase tracking-wide text-content-muted">Según los libros</p>
              <p className="mt-1 text-xl font-semibold tabular-nums">{formatMoneda(acta.saldo_libros || "0", moneda)}</p>
              <p className="mt-1 text-xs text-content-muted">
                Apertura {formatMoneda(acta.saldo_inicial || "0", moneda)} ({acta.fecha_inicial || "—"}) +{" "}
                {formatMoneda(acta.entradas_mes, moneda)} − {formatMoneda(acta.salidas_mes, moneda)}
              </p>
            </div>
          </div>

          <Partidas acta={acta} periodo={periodo} moneda={moneda} editable={editable} />

          <div
            className={cn(
              "rounded-lg border p-3 text-sm",
              dif === 0
                ? "border-positivo/40 bg-positivo/10 text-content"
                : "border-negativo/40 bg-negativo/10 text-content",
            )}
          >
            <span className="font-medium">Diferencia sin explicar: </span>
            <span className="tabular-nums font-semibold">
              {formatMoneda(acta.diferencia_sin_explicar || "0", moneda)}
            </span>
            {dif === 0 ? (
              <p className="mt-1 text-xs text-content-muted">
                Banco + partidas = libros. El acta cuadra y se puede firmar.
              </p>
            ) : (
              <p className="mt-1 text-xs text-content-muted">
                Registrá las partidas que la explican. Si no hay ninguna, faltan movimientos por cargar o el
                saldo capturado está mal.
              </p>
            )}
          </div>

          {acta.firmado_en && (
            <p className="text-xs text-content-muted">
              Firmada el {acta.firmado_en.slice(0, 10)} por {acta.firmado_por || "—"}. Un acta firmada no admite
              cambios.
            </p>
          )}
        </CardContent>
      )}
    </Card>
  );
}

function Partidas({
  acta,
  periodo,
  moneda,
  editable,
}: {
  acta: ActaConciliacion;
  periodo: string;
  moneda: "CRC" | "USD";
  editable: boolean;
}) {
  const toast = useToast();
  const registrar = useRegistrarPartida();
  const anular = useAnularPartida();

  const [tipo, setTipo] = useState<TipoPartida>("DEPOSITO_NO_ACREDITADO");
  const [descripcion, setDescripcion] = useState("");
  const [monto, setMonto] = useState("");
  const [signo, setSigno] = useState("1");

  const dif = toNumber(acta.diferencia_sin_explicar);
  const efecto = TIPOS.find((t) => t.id === tipo)?.efecto ?? "";

  function agregar() {
    const montoApi = montoParaApi(monto);
    if (!descripcion.trim() || toNumber(montoApi) <= 0) {
      toast.error("La partida necesita una descripción y un monto mayor que cero.");
      return;
    }
    registrar.mutate(
      {
        cuenta_id: acta.cuenta_id,
        periodo,
        tipo,
        descripcion: descripcion.trim(),
        monto: montoApi,
        ...(tipo === "OTRA" ? { signo: Number(signo) } : {}),
      },
      {
        onSuccess: () => {
          setDescripcion("");
          setMonto("");
          toast.success("Partida registrada.");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">Partidas en tránsito</h3>
        <span className="text-xs text-content-muted">
          Ajuste total: <span className="tabular-nums">{formatMoneda(acta.ajuste_partidas || "0", moneda)}</span>
        </span>
      </div>

      {acta.partidas.length === 0 ? (
        <p className="text-xs text-content-muted">
          Sin partidas registradas.
          {dif !== 0 && " La diferencia todavía no está explicada."}
        </p>
      ) : (
        <ul className="flex flex-col divide-y divide-border rounded-lg border border-border">
          {acta.partidas.map((p) => (
            <li key={p.id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2 text-sm">
              <div className="min-w-0">
                <span className="block truncate">{p.descripcion}</span>
                <span className="block text-xs text-content-muted">
                  {ETIQUETA_TIPO[p.tipo]} · {p.registrado_por || "—"} · {p.registrado_en.slice(0, 10)}
                </span>
              </div>
              <div className="flex items-center gap-3">
                <span className={cn("tabular-nums", p.signo < 0 ? "text-negativo" : "text-positivo")}>
                  {p.signo < 0 ? "−" : "+"}
                  {formatMoneda(p.monto, moneda)}
                </span>
                {editable && (
                  <Button
                    variant="ghost"
                    onClick={() =>
                      anular.mutate(p.id, {
                        onSuccess: () => toast.success("Partida anulada; queda el rastro."),
                        onError: (err) => toast.error(mensajeError(err)),
                      })
                    }
                    disabled={anular.isPending}
                  >
                    Anular
                  </Button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}

      {editable && (
        <div className="rounded-lg border border-border bg-surface-muted p-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1.2fr_2fr_1fr_auto] sm:items-end">
            <Select
              label="Tipo"
              value={tipo}
              onChange={(e) => setTipo(e.target.value as TipoPartida)}
              options={TIPOS.map((t) => ({ value: t.id, label: t.label }))}
            />
            <Input
              label="Descripción"
              value={descripcion}
              onChange={(e) => setDescripcion(e.target.value)}
              placeholder="Ej.: depósito del 31 acreditado el 1º"
            />
            <Input
              label={`Monto (${moneda})`}
              inputMode="decimal"
              value={monto}
              onChange={(e) => setMonto(e.target.value)}
              placeholder="0.00"
              className="text-right tabular-nums"
            />
            <Button onClick={agregar} disabled={registrar.isPending}>
              Agregar
            </Button>
          </div>
          {tipo === "OTRA" ? (
            <div className="mt-3 max-w-xs">
              <Select
                label="¿Suma o resta al saldo del banco?"
                value={signo}
                onChange={(e) => setSigno(e.target.value)}
                options={[
                  { value: "1", label: "Suma (+)" },
                  { value: "-1", label: "Resta (−)" },
                ]}
              />
            </div>
          ) : (
            <p className="mt-2 text-xs text-content-muted">{efecto}</p>
          )}
        </div>
      )}
    </div>
  );
}
