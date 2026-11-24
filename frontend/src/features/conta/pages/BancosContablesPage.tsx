/**
 * Contabilidad → Cuentas bancarias (/conta/bancos). FASE 3A.
 *
 * ── QUÉ RESUELVE ────────────────────────────────────────────────────────────────────────────
 *
 * Un asiento necesita DOS cuentas. La pantalla de Mapeo contable resolvió una —a qué cuenta va
 * cada partida—; acá se resuelve la otra: qué cuenta contable ES cada cuenta bancaria. Sin esto,
 * de un movimiento del banco sale media partida doble, que no es un asiento.
 *
 * ── POR QUÉ HAY UN BOTÓN QUE ENLAZA VARIAS DE UNA ───────────────────────────────────────────
 *
 * Porque el plan contable ya trae el IBAN escrito dentro del nombre de la cuenta, así que el
 * sistema puede deducir el enlace comparando texto. Medido el 26-oct-2026 en Valle de Paz: 5 de
 * 16 cuentas se deducen solas, y esas 5 cubren el 79 % de los movimientos. Hacer cinco clics
 * idénticos para aceptar lo que ya está deducido no agrega control: el control está en que se ve
 * qué va a enlazar ANTES de apretar, y en que cada una se deshace por separado.
 */

import { useMemo, useState } from "react";

import {
  Badge,
  BuscadorMultiple,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ErrorState,
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
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useAceptarSugerencias,
  useBancosContables,
  useCuentasDeMovimiento,
  useDesenlazarBanco,
  useEnlazarBanco,
} from "@/features/conta/hooks";
import type { CoberturaBancos, FilaBancoContable } from "@/api/conta";

export function BancosContablesPage() {
  const toast = useToast();
  const puedeEnlazar = useTienePermiso()("conta.bancos");

  const bancosQ = useBancosContables();
  const cuentasQ = useCuentasDeMovimiento();
  const enlazar = useEnlazarBanco();
  const desenlazar = useDesenlazarBanco();
  const aceptar = useAceptarSugerencias();

  const filas = bancosQ.data?.items ?? [];
  const cobertura = bancosQ.data?.cobertura;

  const opcionesCuenta = useMemo(
    () =>
      (cuentasQ.data ?? []).map((c) => ({
        value: c.id,
        label: `${c.codigo} · ${c.nombre}`,
        grupo: c.clase,
      })),
    [cuentasQ.data],
  );

  function aplicarSugerencias() {
    aceptar.mutate(undefined, {
      onSuccess: (r) =>
        toast.success(
          r.enlazadas === 0
            ? "No había ninguna sugerencia pendiente."
            : `${r.enlazadas} cuenta(s) enlazada(s) por IBAN.`,
        ),
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Cuentas bancarias"
        description="Qué cuenta contable representa cada cuenta del banco."
      />

      <div className="rounded-lg border border-accent/40 bg-accent/5 px-4 py-3 text-sm text-content">
        <b>Esta es la otra mitad del asiento.</b> En «Mapeo contable» se dice a qué cuenta va cada
        partida; acá, cuál es la cuenta del banco. Todo movimiento bancario necesita las dos: una
        sola no es un asiento.
      </div>

      {cobertura && <AvanceBancos c={cobertura} />}

      {puedeEnlazar && cobertura && cobertura.con_sugerencia > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>
              {cobertura.con_sugerencia} cuenta(s) se pueden enlazar solas
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <p className="text-sm text-content-muted">
              El plan contable trae el IBAN dentro del nombre de la cuenta, así que el sistema
              encontró a cuál corresponde. Abajo se ve cuál propone para cada una; si alguna no
              te convence, enlazala a mano en vez de aceptar todas.
            </p>
            <div>
              <Button onClick={aplicarSugerencias} disabled={aceptar.isPending}>
                Aceptar las {cobertura.con_sugerencia}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {bancosQ.isPending ? (
        <LoadingState label="Cargando las cuentas" />
      ) : bancosQ.isError ? (
        <ErrorState message={mensajeError(bancosQ.error)} onRetry={() => bancosQ.refetch()} />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Cuenta del banco</TH>
                <TH className="text-right">Movimientos</TH>
                <TH>Cuenta contable</TH>
                <TH />
              </TR>
            </THead>
            <TBody>
              {filas.map((f) => (
                <FilaBanco
                  key={f.cuenta_bancaria_id}
                  f={f}
                  opciones={opcionesCuenta}
                  puedeEnlazar={puedeEnlazar}
                  guardando={enlazar.isPending}
                  onEnlazar={(cuentaId) =>
                    enlazar.mutate(
                      { cuentaBancariaId: f.cuenta_bancaria_id, cuentaId },
                      { onError: (e) => toast.error(mensajeError(e)) },
                    )
                  }
                  onDesenlazar={() =>
                    desenlazar.mutate(f.cuenta_bancaria_id, {
                      onError: (e) => toast.error(mensajeError(e)),
                    })
                  }
                />
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}

/**
 * El avance se mide en DOS números distintos a propósito: cuántas cuentas faltan y cuánto
 * movimiento cubren las que ya están. No son lo mismo —5 de 16 cuentas cubren el 79 % del
 * movimiento— y quedarse solo con el primero haría parecer que falta casi todo.
 */
function AvanceBancos({ c }: { c: CoberturaBancos }) {
  const pctCuentas = c.cuentas === 0 ? 100 : Math.floor((c.enlazadas * 100) / c.cuentas);
  const pctMovs =
    c.movimientos_totales === 0
      ? 100
      : Math.floor((c.movimientos_cubiertos * 100) / c.movimientos_totales);
  return (
    <Card>
      <CardContent className="flex flex-wrap items-end gap-8 pt-5">
        <div>
          <p className="text-3xl font-semibold tabular-nums text-content">
            {c.enlazadas}/{c.cuentas}
          </p>
          <p className="text-xs text-content-muted">cuentas enlazadas ({pctCuentas} %)</p>
        </div>
        <div>
          <p className="text-3xl font-semibold tabular-nums text-content">{pctMovs}%</p>
          <p className="text-xs text-content-muted">
            del movimiento cubierto ({c.movimientos_cubiertos.toLocaleString("es-CR")} de{" "}
            {c.movimientos_totales.toLocaleString("es-CR")})
          </p>
        </div>
        {c.faltan > 0 && (
          <div>
            <p className="text-xl font-semibold tabular-nums text-warning">{c.faltan}</p>
            <p className="text-xs text-content-muted">
              faltan{c.con_sugerencia > 0 && `, ${c.con_sugerencia} con sugerencia`}
            </p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function FilaBanco({
  f,
  opciones,
  puedeEnlazar,
  guardando,
  onEnlazar,
  onDesenlazar,
}: {
  f: FilaBancoContable;
  opciones: { value: string; label: string; grupo?: string }[];
  puedeEnlazar: boolean;
  guardando: boolean;
  onEnlazar: (cuentaId: string) => void;
  onDesenlazar: () => void;
}) {
  const [eligiendo, setEligiendo] = useState(false);
  const [sel, setSel] = useState<string[]>([]);

  return (
    <TR>
      <TD className="max-w-xs whitespace-normal">
        <p className="font-medium text-content">{f.alias}</p>
        <p className="text-xs text-content-muted">
          {f.banco} · {f.moneda}
          {f.iban ? ` · ${f.iban}` : " · sin IBAN cargado"}
        </p>
      </TD>
      <TD className="text-right tabular-nums">{f.movimientos.toLocaleString("es-CR")}</TD>
      <TD className="max-w-md whitespace-normal">
        {f.cuenta_id ? (
          <div>
            <p className="tabular-nums text-content">{f.cuenta_codigo}</p>
            <p className="text-xs text-content-muted">{f.cuenta_nombre}</p>
            {f.compartida && (
              // Avisa y no lo impide: agrupar dos cuentas del mismo banco puede ser criterio del
              // contador, pero casi siempre es un error y mezclaría dos bancos en un mayor.
              <p className="mt-1 whitespace-normal break-words text-xs text-warning">
                ⚠ otra cuenta bancaria apunta a esta misma cuenta contable: sus movimientos van a
                quedar mezclados en el mismo mayor
              </p>
            )}
          </div>
        ) : eligiendo ? (
          <div className="flex flex-col gap-2">
            <BuscadorMultiple
              label="Cuenta contable"
              opciones={opciones}
              seleccion={sel}
              onChange={(v) => setSel(v.slice(-1))}
              placeholder="Código o nombre…"
              leyendaVacio="solo cuentas de movimiento"
            />
            <div className="flex gap-2">
              <Button
                size="sm"
                disabled={sel.length === 0 || guardando}
                onClick={() => {
                  if (sel[0]) onEnlazar(sel[0]);
                  setEligiendo(false);
                  setSel([]);
                }}
              >
                Enlazar
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setEligiendo(false)}>
                Cancelar
              </Button>
            </div>
          </div>
        ) : f.sugerida_id ? (
          <div>
            <Badge tone="accent">sugerida</Badge>
            <p className="mt-1 tabular-nums text-content">{f.sugerida_codigo}</p>
            <p className="text-xs text-content-muted">{f.sugerida_nombre}</p>
          </div>
        ) : (
          <span className="text-sm text-content-muted">falta enlazar</span>
        )}
      </TD>
      <TD className="text-right">
        {puedeEnlazar &&
          (f.cuenta_id ? (
            <Button variant="ghost" size="sm" onClick={onDesenlazar}>
              Cambiar
            </Button>
          ) : f.sugerida_id && !eligiendo ? (
            <div className="flex justify-end gap-1">
              <Button size="sm" disabled={guardando} onClick={() => onEnlazar(f.sugerida_id)}>
                Aceptar
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setEligiendo(true)}>
                Otra
              </Button>
            </div>
          ) : (
            !eligiendo && (
              <Button variant="ghost" size="sm" onClick={() => setEligiendo(true)}>
                Enlazar
              </Button>
            )
          ))}
      </TD>
    </TR>
  );
}
