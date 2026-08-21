/**
 * CxP — Validación por riesgo (/cxp/validacion).
 *
 * Los cuatro umbrales que deciden QUÉ factura necesita que el área confirme la conformidad y qué
 * gasto sigue derecho a aprobación. Mover uno de estos números cambia cuánto dinero se paga sin
 * revisión humana, así que la pantalla nunca muestra solo el formulario: arriba va el EFECTO
 * medido —cuántas facturas y cuánto monto le está pidiendo confirmación la regla vigente—, porque
 * subir el umbral de ₡250.000 a ₡2.000.000 se ve igual de inofensivo en un campo de texto y no lo es.
 *
 * Guardar NO recalcula las facturas ya revisadas: el veredicto de cada una es un hecho del momento
 * en que se revisó. Reescribirlo borraría por qué esa factura pasó (o no) por el área.
 */

import { useEffect, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
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
import { formatMoneda } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import { useGuardarParametroValidacion, useParametrosValidacion } from "@/features/cxp/hooks";
import type { ParametroCxP } from "@/api/cxp";

/**
 * Orden y presentación de cada umbral. El orden no es alfabético: es el orden en que se evalúan
 * los criterios, para que se lea como la regla que realmente corre.
 */
const UMBRALES: { clave: string; titulo: string; unidad: string; ayuda: string }[] = [
  {
    clave: "VALIDACION_UMBRAL_MONTO",
    titulo: "Monto desde el que se valida",
    unidad: "CRC",
    ayuda:
      "Es el criterio principal. Toda factura por encima de este monto va a la cola del área, sin importar el proveedor.",
  },
  {
    clave: "VALIDACION_PROVEEDOR_NUEVO_MAX",
    titulo: "Facturas históricas para considerar «nuevo»",
    unidad: "facturas",
    ayuda:
      "Un proveedor con esta cantidad de facturas o menos es nuevo o esporádico: no hay historial contra el que comparar, así que alguien lo mira. Con 0 se desactiva el criterio.",
  },
  {
    clave: "VALIDACION_DESVIO_PCT",
    titulo: "Desvío contra el histórico del proveedor",
    unidad: "%",
    ayuda:
      "Si la factura se aparta más de este porcentaje del promedio de ese mismo proveedor, va al área. Es el criterio que atrapa el cobro raro de un proveedor conocido. Con 0 se desactiva.",
  },
  {
    clave: "VALIDACION_DESVIO_PISO_MONTO",
    titulo: "Piso del criterio de desvío",
    unidad: "CRC",
    ayuda:
      "El desvío solo se evalúa sobre facturas que superen este monto: una anomalía porcentual en una factura chica no representa riesgo, y sin este piso el criterio trae miles de facturas de poco dinero.",
  },
];

export function ValidacionRiesgoPage() {
  const q = useParametrosValidacion();
  const tiene = useTienePermiso();
  const puedeEditar = tiene("cxp.parametros");

  if (q.isLoading) return <LoadingState label="Cargando umbrales" />;
  if (q.isError) return <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />;

  const parametros = q.data?.parametros ?? [];
  const efecto = q.data?.efecto;

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Validación por riesgo"
        description="Qué factura necesita que el área confirme la conformidad. Todo lo demás sigue derecho a aprobación."
      />

      <div className="flex items-start gap-2 rounded-lg border border-border bg-surface-raised px-3 py-2 text-sm text-content-muted">
        🎯{" "}
        <span>
          La regla es <b className="text-content">por excepción</b>: una factura solo espera al área
          si dispara uno de estos criterios. Es como opera el <i>three-way match</i> en los ERP
          grandes —lo que calza contra un compromiso previo se paga sin intervención—, salvo que acá
          el compromiso se aproxima con el historial del propio proveedor, porque no hay órdenes de
          compra.
        </span>
      </div>

      {efecto && <TarjetaEfecto efecto={efecto} />}

      <Card>
        <CardHeader>
          <CardTitle>Umbrales</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {!puedeEditar && (
            <p className="text-xs text-content-muted">
              Estás viendo los umbrales en modo lectura. Cambiarlos requiere el permiso{" "}
              <span className="font-mono">cxp.parametros</span>.
            </p>
          )}
          <p className="text-xs text-content-muted">
            Un cambio aplica <b className="text-content">de aquí en adelante</b>: las facturas ya
            revisadas conservan el veredicto con el que pasaron, para que el expediente siga
            explicando por qué cada una fue (o no) al área.
          </p>
          {UMBRALES.map((u) => {
            const p = parametros.find((x) => x.clave === u.clave);
            if (!p) return null;
            return <FilaUmbral key={u.clave} def={u} param={p} puedeEditar={puedeEditar} />;
          })}
        </CardContent>
      </Card>
    </div>
  );
}

/** Cuánto gasto le pide confirmación la regla vigente, sobre las facturas ya evaluadas. */
function TarjetaEfecto({
  efecto,
}: {
  efecto: NonNullable<ReturnType<typeof useParametrosValidacion>["data"]>["efecto"];
}) {
  if (!efecto || efecto.total === 0) {
    return (
      <Card>
        <CardContent className="py-4 text-sm text-content-muted">
          Todavía no hay facturas evaluadas con esta regla: el veredicto se calcula al revisar, así
          que el efecto aparece cuando empiece a pasar facturación por la Bandeja.
        </CardContent>
      </Card>
    );
  }
  const pctFacturas = (efecto.requieren / efecto.total) * 100;
  const totalMonto = Number(efecto.total_monto);
  const pctMonto = totalMonto > 0 ? (Number(efecto.requieren_monto) / totalMonto) * 100 : 0;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Efecto de la regla vigente</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="rounded-lg border border-border bg-surface-muted px-3 py-2">
            <p className="text-xs text-content-muted">Facturas que esperan al área</p>
            <p className="text-lg font-semibold text-content">
              {efecto.requieren.toLocaleString("es-CR")}{" "}
              <span className="text-sm font-normal text-content-muted">
                de {efecto.total.toLocaleString("es-CR")} · {pctFacturas.toFixed(1)} %
              </span>
            </p>
          </div>
          <div className="rounded-lg border border-border bg-surface-muted px-3 py-2">
            <p className="text-xs text-content-muted">Monto que cubren</p>
            <p className="text-lg font-semibold text-content">
              {formatMoneda(efecto.requieren_monto)}{" "}
              <span className="text-sm font-normal text-content-muted">
                de {formatMoneda(efecto.total_monto)} · {pctMonto.toFixed(1)} %
              </span>
            </p>
          </div>
        </div>
        <p className="text-xs text-content-muted">
          Eso es lo que se busca: revisar pocas facturas y cubrir la mayor parte del dinero. Si el
          porcentaje de facturas se acerca al de monto, la regla está trayendo gasto chico que nadie
          necesita mirar.
        </p>
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Criterio</TH>
                <TH className="text-right">Facturas</TH>
                <TH className="text-right">Monto</TH>
              </TR>
            </THead>
            <TBody>
              {efecto.por_motivo.map((m) => (
                <TR key={m.motivo}>
                  <TD className="text-sm">
                    <Badge tone="pendiente">{m.etiqueta || m.motivo}</Badge>
                  </TD>
                  <TD className="text-right text-sm">{m.cantidad.toLocaleString("es-CR")}</TD>
                  <TD className="text-right text-sm">{formatMoneda(m.monto)}</TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      </CardContent>
    </Card>
  );
}

/** Un umbral: valor editable + qué hace. Se guarda de a uno, para no mover cuatro cosas a la vez. */
function FilaUmbral({
  def,
  param,
  puedeEditar,
}: {
  def: { clave: string; titulo: string; unidad: string; ayuda: string };
  param: ParametroCxP;
  puedeEditar: boolean;
}) {
  const toast = useToast();
  const guardar = useGuardarParametroValidacion();
  const [valor, setValor] = useState(param.valor);

  // Si otro usuario (u otra empresa activa) cambió el umbral, el campo tiene que reflejarlo.
  useEffect(() => setValor(param.valor), [param.valor]);

  const sucio = valor.trim() !== param.valor;
  function onGuardar() {
    guardar.mutate(
      { clave: def.clave, valor: valor.trim() },
      {
        onSuccess: () => toast.success(`${def.titulo}: guardado en ${valor.trim()}`),
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border px-3 py-3 sm:flex-row sm:items-start sm:justify-between">
      <div className="sm:max-w-2xl">
        <p className="text-sm font-medium text-content">{def.titulo}</p>
        <p className="mt-0.5 text-xs text-content-muted">{def.ayuda}</p>
      </div>
      <div className="flex items-end gap-2">
        <Input
          label={def.unidad}
          value={valor}
          onChange={(e) => setValor(e.target.value)}
          disabled={!puedeEditar || guardar.isPending}
          inputMode="decimal"
          className="max-w-32"
        />
        {puedeEditar && (
          <Button size="sm" disabled={!sucio || guardar.isPending} onClick={onGuardar}>
            Guardar
          </Button>
        )}
      </div>
    </div>
  );
}
