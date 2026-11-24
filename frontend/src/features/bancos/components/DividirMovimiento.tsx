/**
 * Repartir un movimiento entre partidas.
 *
 * «Del recibo de luz este mes se pagó 1.400.000, pero de luz normal eran 1.000.000, y esos 400.000
 * corresponden a un proyecto especial. Del movimiento principal se quita lo del proyecto, pero la
 * sumatoria de ambas suman el movimiento real, y se conserva la integridad del consecutivo, fecha,
 * descripción.»
 *
 * LA INVARIANTE SE MUESTRA, NO SE PROMETE. El panel siempre tiene a la vista lo que trajo el banco
 * y lo que suman las partes, y el `cuadra` lo calcula el servidor. Si alguna vez difieren, se ve —
 * que es la única forma de que un reparto mal hecho no pase inadvertido.
 */
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { Badge, Button, Input, LoadingState, useToast } from "@/components/ui";
import { bancosApi, type DivisionMovimiento } from "@/api/bancos";
import { ClasifCombobox, type ClasifElegida } from "@/features/bancos/components/ClasifCombobox";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";
import { formatMonto } from "@/lib/format";
import { copiarAlPortapapeles } from "@/lib/portapapeles";

export function DividirMovimiento(props: {
  movimientoId: string;
  /** Signo del movimiento: lo usa el buscador de partidas para proponer el concepto correcto. */
  esDebito?: boolean;
  /**
   * La factura que este débito pagó, si está enlazado a CxP.
   *
   * Dividir uno enlazado SÍ se puede —el enlace dice qué factura se pagó y el reparto a qué
   * partida va el gasto, que son cosas distintas— pero hay que decirlo: quien reparte tiene que
   * saber que no está cambiando lo que se le pagó al proveedor.
   */
  pagoCxP?: { proveedor: string; consecutivo: string; facturas: number };
  onCerrar?: () => void;
}) {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();

  const [monto, setMonto] = useState("");
  const [motivo, setMotivo] = useState("");
  // Se reusa el MISMO buscador de partidas que la pantalla de Clasificar: elegir a qué partida va
  // una parte es el mismo acto que clasificar, y dos buscadores distintos para lo mismo acaban
  // ofreciendo catálogos distintos.
  const [destino, setDestino] = useState<ClasifElegida | null>(null);

  const q = useQuery({
    queryKey: ["bancos", empresaId, "partes", props.movimientoId],
    queryFn: () => bancosApi.partesDeMovimiento(props.movimientoId),
  });

  const dividir = useMutation({
    mutationFn: () =>
      bancosApi.dividirMovimiento(props.movimientoId, {
        monto,
        concepto_id: destino?.conceptoId,
        clasificacion_id: destino?.clasificacionId ?? "",
        motivo,
      }),
    onSuccess: () => {
      toast.success("Movimiento repartido");
      setMonto("");
      setMotivo("");
      void qc.invalidateQueries({ queryKey: ["bancos", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const deshacer = useMutation({
    mutationFn: (parteId: string) => bancosApi.deshacerDivision(parteId),
    onSuccess: () => {
      toast.success("Parte devuelta al movimiento");
      void qc.invalidateQueries({ queryKey: ["bancos", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  if (q.isLoading) return <LoadingState label="Cargando el reparto" />;
  const d = q.data;
  // NUNCA EN BLANCO.
  //
  // Esto devolvía `null` cuando la consulta fallaba, así que el panel se abría vacío y el botón
  // parecía no hacer nada — reportado por el Director Financiero el 13-oct-2026. El caso real:
  // un movimiento EXCLUIDO de los libros, que el endpoint no devuelve porque no tiene partes que
  // mostrar. El problema no era ese 404 sino que no se viera: un panel mudo manda a dudar del
  // botón en vez de leer el motivo.
  if (!d) {
    return (
      <div className="rounded-lg border border-border bg-surface-raised px-3 py-2 text-sm text-content-muted">
        No se pudo abrir el reparto de este movimiento.{" "}
        {q.error ? mensajeError(q.error) : "Probá cerrando y abriendo de nuevo."}
      </div>
    );
  }

  const dividido = d.partes.length > 1;

  return (
    <div className="space-y-4 rounded-lg border p-4">
      {/* El hecho bancario, que es UNO y no se reparte. */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium">{d.descripcion}</div>
          <div className="text-xs text-content-muted">
            {d.fecha} · consecutivo {d.documento}
          </div>
        </div>
        {/* EL CÓDIGO DEL REPARTO, copiable. Es lo que nombra al grupo: sin él, las caras de un
            reparto solo se pueden señalar diciendo «las dos filas del 12 de agosto», y en el
            .xlsx no hay forma de agruparlas. Se copia con el botón y no a ojo: un código
            transcrito mal devuelve CERO resultados, igual que uno inexistente. */}
        {dividido && (
          <button
            type="button"
            className="rounded-md border border-brand-gold/40 bg-brand-gold/10 px-2 py-1 font-mono text-xs text-brand-gold hover:bg-brand-gold/20"
            title="Copiar el código del reparto. Es el mismo que sale en la columna «Reparto» del .xlsx."
            onClick={async () => {
              const ok = await copiarAlPortapapeles(d.codigo);
              toast[ok ? "success" : "error"](
                ok ? "Código del reparto copiado." : "No se pudo copiar; seleccionalo a mano.",
              );
            }}
          >
            {d.codigo} ⧉
          </button>
        )}
      </div>

      {dividido && <Invariante d={d} />}

      <div className="space-y-2">
        {d.partes.map((p) => (
          <div key={p.id} className="flex flex-wrap items-center justify-between gap-2 rounded-md border p-3">
            <div>
              <div className="text-sm">
                {p.clasificacion || "Sin clasificar"}
                {p.es_principal && (
                  <span className="ml-2 text-xs text-content-muted">(movimiento principal)</span>
                )}
              </div>
              {p.motivo && <div className="text-xs text-content-muted">{p.motivo}</div>}
            </div>
            <div className="flex items-center gap-3">
              <span className="font-medium tabular-nums">{formatMonto(p.monto)}</span>
              {/* Solo las partes se devuelven; el principal no es una parte de nada. */}
              {!p.es_principal && (
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={deshacer.isPending}
                  onClick={() => deshacer.mutate(p.id)}
                >
                  Devolver
                </Button>
              )}
            </div>
          </div>
        ))}
      </div>

      <form
        className="space-y-3 border-t pt-3"
        onSubmit={(e) => {
          e.preventDefault();
          dividir.mutate();
        }}
      >
        <div className="text-sm font-medium">Separar una parte</div>
        {/* EL AVISO DEL PAGO ENLAZADO. Repartir un débito que pagó una factura es legítimo —el
            gasto puede corresponder a otra empresa del grupo— pero hay que decir con todas las
            letras qué se mueve y qué no: lo que se le pagó al proveedor no cambia, y la factura
            sigue viéndose pagada por el total porque el expediente suma las caras. */}
        {props.pagoCxP && (
          <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-3 py-2 text-xs text-content-muted">
            Este débito pagó{" "}
            {props.pagoCxP.facturas > 1
              ? `${props.pagoCxP.facturas} facturas de ${props.pagoCxP.proveedor || "un proveedor"}`
              : `la factura ${props.pagoCxP.consecutivo || "de CxP"} de ${props.pagoCxP.proveedor || "un proveedor"}`}
            . Repartirlo cambia <b className="text-content">a qué partida va el gasto</b>, no lo
            que se le pagó: la factura sigue figurando pagada por el total, porque su expediente
            suma todas las partes.
          </div>
        )}
        <div className="flex flex-wrap items-end gap-3">
          <label className="text-sm">
            <span className="mb-1 block text-content-muted">Monto a separar</span>
            <Input
              type="number"
              min="0"
              step="0.01"
              className="w-40"
              value={monto}
              onChange={(e) => setMonto(e.target.value)}
              required
            />
          </label>
          <div className="min-w-[16rem] flex-1">
            <span className="mb-1 block text-sm text-content-muted">A qué partida va</span>
            <ClasifCombobox
              actual={destino?.ruta ?? ""}
              esDebito={props.esDebito}
              onElegir={setDestino}
              placeholder="Elegí la partida de la parte"
            />
          </div>
        </div>
        <label className="block text-sm">
          <span className="mb-1 block text-content-muted">Por qué (queda en auditoría)</span>
          <Input
            value={motivo}
            onChange={(e) => setMotivo(e.target.value)}
            placeholder="obra del horno, no es consumo normal"
          />
        </label>
        <Button type="submit" disabled={dividir.isPending || !monto || !destino}>
          {dividir.isPending ? "Separando…" : "Separar"}
        </Button>
        <p className="text-xs text-content-muted">
          Al movimiento principal se le resta lo que separes. La suma de las partes siempre da lo
          que trajo el banco, y el total de los libros no cambia.
        </p>
      </form>

      {props.onCerrar && (
        <Button variant="ghost" size="sm" onClick={props.onCerrar}>
          Cerrar
        </Button>
      )}
    </div>
  );
}

/**
 * La invariante a la vista: lo que trajo el banco contra lo que suman las partes.
 *
 * `cuadra` viene del servidor. Si alguna vez dijera que no, es un problema de datos que hay que
 * ver, no un detalle de presentación: por eso se muestra en rojo y con el número de la diferencia.
 */
function Invariante({ d }: { d: DivisionMovimiento }) {
  if (d.cuadra) {
    return (
      <div className="flex items-center gap-2 rounded-md bg-positivo/10 px-3 py-2 text-sm">
        <Badge tone="positivo">cuadra</Badge>
        <span>
          Las {d.partes.length} partes suman{" "}
          <strong className="tabular-nums">{formatMonto(d.suma_de_las_partes)}</strong>, que es lo
          que trajo el banco.
        </span>
      </div>
    );
  }
  return (
    <div className="rounded-md bg-negativo/10 px-3 py-2 text-sm text-negativo">
      <strong>El reparto no cuadra.</strong> El banco trajo{" "}
      {formatMonto(d.monto_antes_de_dividir)} y las partes suman{" "}
      {formatMonto(d.suma_de_las_partes)}. Avisá a Dirección Financiera: hay plata sin atribuir.
    </div>
  );
}
