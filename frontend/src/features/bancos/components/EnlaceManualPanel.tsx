/**
 * «Enlazarlos yo»: el mismo enlace de la mig 0118, pero sin pasar por el detector (12-nov-2026).
 *
 * EL HUECO QUE TAPA. Las dos cajas de arriba dependen de una huella: los duplicados exigen el MISMO
 * DÍA y las reemisiones el MISMO CONSECUTIVO. El banco que motivó todo esto cambia la transacción
 * entera —otra fecha Y otro consecutivo—, así que ese caso se escapa por el medio de las dos y no
 * había ninguna otra puerta: `excluirMovimiento` solo se ofrecía desde los duplicados detectados y
 * `enlazarReemision` solo desde los candidatos. Con el par delante y la certeza de quién lo mira,
 * no se podía hacer nada.
 *
 * POR QUÉ NO SE AMPLÍA EL DETECTOR EN VEZ DE ESTO. Medido sobre los datos reales: pares de mismo
 * monto y misma cuenta con consecutivo DISTINTO, a 7 días o menos, hay 1.646.427 — 1.642.038 solo
 * en Davivienda, que repite montos todo el tiempo por el overnight. Una lista así no se revisa: se
 * ignora, y el día que alguien la usa enlaza dos cobros reales y desaparece plata de los libros.
 * Por eso acá no hay sugerencia: el sistema no opina, solo ejecuta lo que la persona ya decidió.
 *
 * EL SERVIDOR SIGUE SIENDO EL QUE MANDA. El consecutivo y los tres días eran condiciones del
 * DETECTOR, nunca del enlace: `EnlazarReemision` exige misma cuenta, mismo monto y mismo lado, los
 * dos en los libros, ninguno ya enlazado, el que sale en un mes abierto y sin nada colgando. Esta
 * pantalla no relaja ninguna — solo deja elegir el par a mano.
 */
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  ErrorState,
  Input,
  LoadingState,
  Select,
  useToast,
} from "@/components/ui";
import { bancosApi, type MovimientoRow } from "@/api/bancos";
import { useCuentas } from "@/features/bancos/hooks";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";
import { formatFecha, formatMonto } from "@/lib/format";

/** El monto con su lado, como lo compara el servidor: débito y crédito son cosas distintas. */
function montoDe(m: MovimientoRow): { valor: string; esDebito: boolean } {
  const d = Number(m.debito) || 0;
  return d > 0 ? { valor: m.debito, esDebito: true } : { valor: m.credito, esDebito: false };
}

/** Dos movimientos son enlazables si el servidor los va a aceptar: misma cuenta, mismo monto y lado. */
function porQueNoSePuede(a: MovimientoRow, b: MovimientoRow): string {
  if (a.id === b.id) return "Elegí dos movimientos distintos.";
  if (a.cuenta !== b.cuenta)
    return `Son de cuentas distintas (${a.cuenta} y ${b.cuenta}). El banco no reemite de una cuenta a otra: eso sería un traslado, y un traslado conserva sus dos patas.`;
  const ma = montoDe(a);
  const mb = montoDe(b);
  if (ma.esDebito !== mb.esDebito)
    return "Uno es débito y el otro crédito. El mismo hecho no cambia de lado al reemitirse.";
  if (Number(ma.valor) !== Number(mb.valor))
    return `Los montos no son iguales (${formatMonto(ma.valor)} y ${formatMonto(mb.valor)}).`;
  return "";
}

/** Una fila del resultado, con sus dos botones de radio. */
function FilaResultado({
  m,
  esVigente,
  esSaliente,
  onVigente,
  onSaliente,
}: {
  m: MovimientoRow;
  esVigente: boolean;
  esSaliente: boolean;
  onVigente: () => void;
  onSaliente: () => void;
}) {
  const { valor, esDebito } = montoDe(m);
  const marcado = esVigente || esSaliente;
  return (
    <div
      className={`grid grid-cols-[auto_auto_1fr] items-start gap-3 rounded-md border p-3 transition ${
        marcado ? "border-primary bg-primary/5" : "border-border hover:bg-surface-muted"
      }`}
    >
      {/* Dos radios por fila y no uno: el usuario tiene que decir CUÁL se queda y CUÁL sale, y son
          decisiones distintas. Con un solo control habría que adivinar la segunda. */}
      <input
        type="radio"
        name="enlace-manual-vigente"
        checked={esVigente}
        onChange={onVigente}
        className="mt-1"
        aria-label={`Dejar en los libros el movimiento del ${m.fecha} por ${valor}`}
      />
      <input
        type="radio"
        name="enlace-manual-saliente"
        checked={esSaliente}
        onChange={onSaliente}
        className="mt-1"
        aria-label={`Sacar de los libros el movimiento del ${m.fecha} por ${valor}`}
      />
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{formatFecha(m.fecha)}</span>
          <span className="tabular-nums">
            {formatMonto(valor)} {esDebito ? "débito" : "crédito"}
          </span>
          <span className="text-xs text-content-muted">{m.documento}</span>
          {esVigente && <Badge tone="positivo">Se queda</Badge>}
          {esSaliente && <Badge tone="neutral">Sale</Badge>}
        </div>
        <p className="mt-1 break-words text-sm text-content-muted">{m.descripcion}</p>
        <p className="mt-1 text-xs">
          <span className="text-content-muted">{m.cuenta}</span>
          {m.clasificacion ? (
            <span className="text-content-muted"> · {m.clasificacion}</span>
          ) : (
            <span className="text-pendiente"> · Sin clasificar</span>
          )}
        </p>
      </div>
    </div>
  );
}

export function EnlaceManualPanel() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();
  const cuentasQ = useCuentas();

  const [cuenta, setCuenta] = useState("");
  const [desde, setDesde] = useState("");
  const [hasta, setHasta] = useState("");
  const [monto, setMonto] = useState("");
  const [buscado, setBuscado] = useState<null | {
    cuenta_bancaria_id: string;
    desde: string;
    hasta: string;
    monto_min?: string;
    monto_max?: string;
  }>(null);
  const [vigenteId, setVigenteId] = useState("");
  const [salienteId, setSalienteId] = useState("");
  const [motivo, setMotivo] = useState("");

  const resQ = useQuery({
    queryKey: ["bancos", empresaId, "enlace-manual", buscado],
    queryFn: () => bancosApi.movimientos({ ...buscado!, page_size: 200 }),
    enabled: buscado !== null,
  });

  const enlazar = useMutation({
    mutationFn: (v: { vigenteId: string; reemplazaA: string; motivo: string }) =>
      bancosApi.enlazarReemision(v.vigenteId, v.reemplazaA, v.motivo),
    onSuccess: () => {
      toast.success("Enlazados: ahora el monto cuenta una sola vez");
      setVigenteId("");
      setSalienteId("");
      setMotivo("");
      void qc.invalidateQueries({ queryKey: ["bancos", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const cuentas = cuentasQ.data ?? [];
  const items = resQ.data?.items ?? [];
  const vigente = items.find((m) => m.id === vigenteId);
  const saliente = items.find((m) => m.id === salienteId);
  const impedimento = vigente && saliente ? porQueNoSePuede(vigente, saliente) : "";
  const motivoCorto = motivo.trim().length < 10;
  const listo = Boolean(vigente && saliente) && impedimento === "" && !motivoCorto;

  function buscar() {
    setVigenteId("");
    setSalienteId("");
    // El monto va como rango cerrado sobre el mismo valor: el filtro del servidor es en COLONES y
    // en valor absoluto, así que un débito y un crédito del mismo tamaño caen los dos — que es lo
    // que quiere quien busca, porque todavía no sabe cuál de los dos está mirando.
    const m = monto.trim();
    setBuscado({
      cuenta_bancaria_id: cuenta,
      desde,
      hasta,
      ...(m !== "" ? { monto_min: m, monto_max: m } : {}),
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Enlazarlos yo</CardTitle>
        <CardDescription>
          Para cuando el sistema <strong>no</strong> los propone y vos ya sabés que son el mismo
          movimiento: el banco cambió la fecha y el consecutivo, así que no cae ni en duplicados ni
          en reemisiones. Acá los elegís a mano. Hace exactamente lo mismo que la caja de arriba —
          uno sale de los libros, ninguno se borra, y se puede deshacer.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* El aviso va ARRIBA y es más fuerte que el de la caja de arriba a propósito: allá hay una
            huella que respalda el par; acá la única garantía es el ojo de quien lo hace. */}
        <p className="rounded-md border border-pendiente/40 bg-pendiente/5 p-3 text-sm">
          Acá no hay detector que te respalde: lo que enlaces, se enlaza. Antes de confirmar, leé
          las dos descripciones completas y cruzá la referencia o el número de autorización contra
          el estado de cuenta. Dos cobros reales del mismo monto se ven casi iguales, y enlazarlos
          hace desaparecer plata de los libros.
        </p>

        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-56">
            <label className="mb-1 block text-xs text-content-muted" htmlFor="em-cuenta">
              Cuenta
            </label>
            <Select
              id="em-cuenta"
              value={cuenta}
              onChange={(e) => setCuenta(e.target.value)}
              options={[
                { value: "", label: cuentasQ.isPending ? "Cargando…" : "Elegí la cuenta" },
                ...cuentas.map((c) => ({ value: c.id, label: `${c.banco} · ${c.alias}` })),
              ]}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-content-muted" htmlFor="em-desde">
              Desde
            </label>
            <Input id="em-desde" type="date" value={desde} onChange={(e) => setDesde(e.target.value)} />
          </div>
          <div>
            <label className="mb-1 block text-xs text-content-muted" htmlFor="em-hasta">
              Hasta
            </label>
            <Input id="em-hasta" type="date" value={hasta} onChange={(e) => setHasta(e.target.value)} />
          </div>
          <div>
            <label className="mb-1 block text-xs text-content-muted" htmlFor="em-monto">
              Monto (opcional)
            </label>
            <Input
              id="em-monto"
              value={monto}
              onChange={(e) => setMonto(e.target.value)}
              placeholder="27001"
              className="w-32 text-right"
            />
          </div>
          <Button onClick={buscar} disabled={cuenta === "" || desde === "" || hasta === ""}>
            Buscar
          </Button>
        </div>
        {/* El monto filtra en colones aunque la cuenta sea en dólares: decirlo evita que alguien
            escriba el monto en dólares, no encuentre nada y concluya que el movimiento no está. */}
        <p className="text-xs text-content-muted">
          El monto se busca en colones y sin signo, así que trae el débito y el crédito de ese
          tamaño. En una cuenta en dólares, dejalo vacío y guiate por la fecha.
        </p>

        {resQ.isPending && buscado !== null && <LoadingState label="Buscando movimientos" />}
        {resQ.isError && <ErrorState message={mensajeError(resQ.error)} />}

        {buscado !== null && !resQ.isPending && !resQ.isError && items.length === 0 && (
          <p className="py-6 text-center text-sm text-content-muted">
            No hay movimientos en ese rango. Probá ampliando las fechas o sin el monto.
          </p>
        )}

        {items.length > 0 && (
          <>
            <div className="grid grid-cols-[auto_auto_1fr] gap-3 px-3 text-xs font-medium text-content-muted">
              <span className="w-4 text-center">Queda</span>
              <span className="w-4 text-center">Sale</span>
              <span>
                {items.length} movimiento(s)
                {resQ.data && resQ.data.total > items.length && ` de ${resQ.data.total}`}
              </span>
            </div>
            <div className="space-y-2">
              {items.map((m) => (
                <FilaResultado
                  key={m.id}
                  m={m}
                  esVigente={m.id === vigenteId}
                  esSaliente={m.id === salienteId}
                  onVigente={() => setVigenteId(m.id)}
                  onSaliente={() => setSalienteId(m.id)}
                />
              ))}
            </div>

            {/* El impedimento se dice ANTES de que el usuario escriba el motivo y apriete: el
                servidor lo rechazaría igual, pero hacerle escribir una justificación para después
                negarle la acción es hacerle perder el trabajo. */}
            {impedimento !== "" && (
              <p className="rounded-md border border-negativo/40 bg-negativo/5 p-3 text-sm">
                {impedimento}
              </p>
            )}

            {vigente && saliente && impedimento === "" && (
              <div className="space-y-3 rounded-lg border border-border p-4">
                <p className="text-sm">
                  Se queda el del <strong>{formatFecha(vigente.fecha)}</strong> y sale el del{" "}
                  <strong>{formatFecha(saliente.fecha)}</strong>, por{" "}
                  {formatMonto(montoDe(vigente).valor)} en {vigente.cuenta}.
                </p>
                <div className="flex flex-wrap items-end gap-2">
                  <div className="min-w-[16rem] flex-1">
                    <label className="mb-1 block text-xs text-content-muted" htmlFor="em-motivo">
                      ¿Por qué son el mismo movimiento?
                    </label>
                    <Input
                      id="em-motivo"
                      value={motivo}
                      onChange={(e) => setMotivo(e.target.value)}
                      placeholder="Ej.: el banco reemitió el mismo débito con otro consecutivo"
                    />
                  </div>
                  <Button
                    onClick={() =>
                      enlazar.mutate({
                        vigenteId,
                        reemplazaA: salienteId,
                        motivo: motivo.trim(),
                      })
                    }
                    disabled={!listo || enlazar.isPending}
                  >
                    Es el mismo movimiento
                  </Button>
                </div>
                {motivoCorto && motivo.length > 0 && (
                  <p className="text-xs text-pendiente">
                    Escribí un poco más: quien revise el mes tiene que entender por qué esa plata no
                    está, sin una lista de candidatos que lo explique.
                  </p>
                )}
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
