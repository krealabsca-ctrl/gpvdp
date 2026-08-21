/**
 * Pantalla 7 — Tipo de cambio (/tipo-cambio).
 * Registrar cotización (fecha/valor/fuente); ver estado del mes
 * (PROVISIONAL/CONGELADO) con sus cotizaciones y valor congelado; botón
 * Congelar (POST). El backend exige rol autorizado + MFA: si responde 403 se
 * muestra el aviso de permiso; el botón se deshabilita si ya está congelado.
 */

import { useState, type FormEvent } from "react";
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
import { componerPeriodo, etiquetaPeriodo, formatFecha, partesPeriodo } from "@/lib/format";
import { mensajeError, esStatus } from "@/lib/apiError";
import { usePeriodoActivo } from "@/app/PeriodoProvider";
import {
  useCongelarTipoCambio,
  useRegistrarCotizacion,
  useSincronizarBCCR,
  useTipoCambio,
  useUltimoSyncBCCR,
} from "@/features/bancos/hooks";
import type { FuenteTC } from "@/api/bancos";

const FUENTE_OPTIONS: { value: FuenteTC; label: string }[] = [
  { value: "BCCR", label: "BCCR" },
  { value: "MANUAL", label: "Manual" },
];

export function TipoCambioPage() {
  const toast = useToast();
  const { periodo } = usePeriodoActivo();
  const { anio, mes } = partesPeriodo(periodo);

  const tcQuery = useTipoCambio(periodo);
  const registrar = useRegistrarCotizacion(periodo);
  const congelar = useCongelarTipoCambio(periodo);
  const sync = useSincronizarBCCR(periodo);
  const ultimoSync = useUltimoSyncBCCR();

  // Fecha por defecto: día 1 del período elegido.
  const [fecha, setFecha] = useState(`${componerPeriodo(anio, mes)}-01`);
  const [valor, setValor] = useState("");
  const [fuente, setFuente] = useState<FuenteTC>("MANUAL");
  const [confirmarCongelar, setConfirmarCongelar] = useState(false);

  const estado = tcQuery.data?.estado;
  const congelado = estado === "CONGELADO";

  function sincronizarBCCR() {
    sync.mutate(fecha, {
      onSuccess: (r) => {
        if (r.exito && r.omitido) toast.success(r.mensaje);
        else if (r.exito) toast.success(`BCCR: ${r.valor} (${r.fecha}).`);
        else toast.error(`BCCR no disponible: ${r.mensaje}`);
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  function registrarCotizacion(e: FormEvent) {
    e.preventDefault();
    if (!fecha) return toast.error("Ingresá una fecha.");
    if (!valor.trim() || Number.isNaN(Number(valor))) return toast.error("Ingresá un valor numérico.");

    registrar.mutate(
      { fecha, valor: valor.trim(), fuente },
      {
        onSuccess: () => {
          toast.success("Cotización registrada.");
          setValor("");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function congelarMes() {
    congelar.mutate(undefined, {
      onSuccess: () => {
        toast.success(`Tipo de cambio de ${etiquetaPeriodo(periodo)} congelado.`);
        setConfirmarCongelar(false);
      },
      onError: (err) => {
        if (esStatus(err, 403)) {
          toast.error("No tenés permiso (o falta MFA) para congelar el tipo de cambio.");
        } else {
          toast.error(mensajeError(err));
        }
        setConfirmarCongelar(false);
      },
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Tipo de cambio"
        description="Registrá cotizaciones del mes y congelá el TC (promedio de día 1, 15 y último)."
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Registrar cotización */}
        <Card>
          <CardHeader>
            <CardTitle>Registrar cotización</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="mb-3 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-surface-muted/40 px-3 py-2">
              <Button size="sm" variant="secondary" onClick={sincronizarBCCR} loading={sync.isPending} disabled={congelado}>
                Sincronizar con BCCR
              </Button>
              <span className="text-xs text-content-muted">
                {ultimoSync.data?.sincronizado && ultimoSync.data.log
                  ? `Último: ${ultimoSync.data.log.exito ? "✓" : "✗"} ${ultimoSync.data.log.fecha}` +
                    (ultimoSync.data.log.exito && ultimoSync.data.log.valor ? ` · ₡${ultimoSync.data.log.valor}` : ` · ${ultimoSync.data.log.mensaje}`)
                  : "Trae la cotización de la fecha de abajo; si el BCCR no está configurado, cargá el valor a mano."}
              </span>
            </div>
            <form onSubmit={registrarCotizacion} className="flex flex-col gap-3">
              <Input
                type="date"
                label="Fecha"
                value={fecha}
                onChange={(e) => setFecha(e.target.value)}
              />
              <Input
                label="Valor (CRC por USD)"
                value={valor}
                onChange={(e) => setValor(e.target.value)}
                placeholder="Ej. 512.35"
                inputMode="decimal"
                className="tabular-nums"
              />
              <Select
                label="Fuente"
                value={fuente}
                onChange={(e) => setFuente(e.target.value as FuenteTC)}
                options={FUENTE_OPTIONS}
              />
              <div className="flex justify-end">
                <Button type="submit" loading={registrar.isPending} disabled={congelado}>
                  Registrar
                </Button>
              </div>
              {congelado && (
                <p className="text-xs text-content-muted">
                  El mes está congelado: no se aceptan nuevas cotizaciones.
                </p>
              )}
            </form>
          </CardContent>
        </Card>

        {/* Estado del mes */}
        <Card>
          <CardHeader>
            <CardTitle>Estado — {etiquetaPeriodo(periodo)}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {tcQuery.isPending ? (
              <LoadingState label="Cargando tipo de cambio" />
            ) : tcQuery.isError ? (
              <ErrorState message={mensajeError(tcQuery.error)} onRetry={() => tcQuery.refetch()} />
            ) : (
              <>
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-xs uppercase tracking-wide text-content-muted">Estado</p>
                    <Badge tone={congelado ? "positivo" : "pendiente"}>
                      {congelado ? "Congelado" : "Provisional"}
                    </Badge>
                  </div>
                  <div className="text-right">
                    <p className="text-xs uppercase tracking-wide text-content-muted">
                      Valor congelado
                    </p>
                    <p className="text-lg font-semibold tabular-nums text-content">
                      {tcQuery.data?.valor_congelado ?? "—"}
                    </p>
                  </div>
                </div>

                <Button
                  onClick={() => setConfirmarCongelar(true)}
                  disabled={congelado}
                >
                  {congelado ? "Ya congelado" : "Congelar mes"}
                </Button>
              </>
            )}
          </CardContent>
        </Card>
      </div>

      {confirmarCongelar && (
        <ConfirmDialog
          titulo={`Congelar el TC de ${etiquetaPeriodo(periodo)}`}
          descripcion="El tipo de cambio del mes se fija en el promedio de las cotizaciones del día 1, 15 y último."
          impacto={[
            "El valor congelado es INMUTABLE para este mes (no se puede recalcular).",
            "Se aplica retroactivamente a TODOS los movimientos en USD del mes.",
            "Solo un rol autorizado puede congelar.",
          ]}
          textoConfirmar="Congelar"
          tono="peligro"
          pendiente={congelar.isPending}
          onConfirmar={congelarMes}
          onCancelar={() => setConfirmarCongelar(false)}
        />
      )}

      {/* Cotizaciones del mes */}
      <Card>
        <CardHeader>
          <CardTitle>Cotizaciones registradas</CardTitle>
        </CardHeader>
        <CardContent>
          {tcQuery.isPending ? (
            <LoadingState label="Cargando cotizaciones" />
          ) : (tcQuery.data?.cotizaciones ?? []).length === 0 ? (
            <EmptyState message="Sin cotizaciones registradas en este mes." />
          ) : (
            <TableContainer>
              <Table>
                <THead>
                  <TR>
                    <TH>Fecha</TH>
                    <TH className="text-right">Valor</TH>
                    <TH>Fuente</TH>
                  </TR>
                </THead>
                <TBody>
                  {tcQuery.data!.cotizaciones.map((c) => (
                    <TR key={`${c.fecha}-${c.fuente}`}>
                      <TD className="tabular-nums">{formatFecha(c.fecha)}</TD>
                      <TD className="text-right tabular-nums">{c.valor}</TD>
                      <TD>
                        <Badge tone={c.fuente === "BCCR" ? "accent" : "neutral"}>{c.fuente}</Badge>
                      </TD>
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
