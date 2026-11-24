/**
 * «El banco lo mandó dos veces»: enlazar dos movimientos que son el mismo hecho (mig 0118).
 *
 * El caso que lo originó (10-nov-2026): «hay un banco que el 100% cambia una transacción y no hay
 * forma de mapearla… una vez que yo los identifique me permita relacionarlos como el mismo
 * movimiento de forma manual». Un crédito de ₡880.000 entra el 4 de noviembre y se clasifica; el 5
 * vuelve a entrar con el MISMO consecutivo y otra descripción, y cae en «por clasificar».
 *
 * NO SE SOBRESCRIBE NINGUNO. Los dos se quedan con su fecha y su descripción originales —son
 * evidencia del banco— y lo único que se decide es cuál CUENTA para los libros. El otro sale, con
 * su motivo y su autor, y se puede devolver.
 *
 * LA PANTALLA PROPONE, NO DECIDE, por la misma razón que los duplicados: medido sobre los datos
 * reales, la huella «mismo consecutivo, mismo monto, fechas distintas» devuelve sobre todo pagos
 * RECURRENTES legítimos —un ahorro de ₡1.000.000 que el BCR debita cada semana con el mismo
 * consecutivo—. Por eso cada par muestra a cuántos días está: una recurrencia cae a 7, 14 o 30
 * días; una reemisión del banco cae al día siguiente.
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
  useToast,
} from "@/components/ui";
import { bancosApi, type CandidatoReemision, type LadoReemision } from "@/api/bancos";
import { useEmpresaId } from "@/features/bancos/useEmpresaId";
import { mensajeError } from "@/lib/apiError";
import { formatFecha, formatMonto } from "@/lib/format";

/** Una fila del par: la que el usuario elige es la que se queda en los libros. */
function LadoElegible({
  lado,
  elegido,
  onElegir,
  idGrupo,
}: {
  lado: LadoReemision;
  elegido: boolean;
  onElegir: () => void;
  idGrupo: string;
}) {
  // El <label> va asociado por htmlFor en vez de envolver al input: con el input adentro, un clic
  // sobre el propio radio dispara el evento dos veces (el nativo y el que reenvía el label), y eso
  // es ruido innecesario justo en el control que decide cuál movimiento sale de los libros.
  const idInput = `${idGrupo}-${lado.id}`;
  return (
    <div
      className={`flex items-start gap-3 rounded-md border p-3 transition ${
        elegido ? "border-primary bg-primary/5" : "border-border hover:bg-surface-muted"
      }`}
    >
      <input
        id={idInput}
        type="radio"
        name={idGrupo}
        checked={elegido}
        onChange={onElegir}
        className="mt-1"
        aria-label={`Dejar en los libros el movimiento del ${lado.fecha}`}
      />
      <label htmlFor={idInput} className="min-w-0 flex-1 cursor-pointer">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{formatFecha(lado.fecha)}</span>
          <span className="text-xs text-content-muted">{lado.documento}</span>
          {elegido ? (
            <Badge tone="positivo">Se queda en los libros</Badge>
          ) : (
            <Badge tone="neutral">Sale de los libros</Badge>
          )}
        </div>
        <p className="mt-1 break-words text-sm text-content-muted">{lado.descripcion}</p>
        {/* La clasificación decide cuál conviene dejar: si uno ya está trabajado y el otro no,
            dejar el clasificado ahorra volver a hacerlo. Y si se deja el vacío, lo hereda. */}
        <p className="mt-1 text-xs">
          {lado.clasificacion ? (
            <span className="text-content-muted">
              {lado.clasificacion} · {lado.estado_clasificacion}
            </span>
          ) : (
            <span className="text-pendiente">Sin clasificar</span>
          )}
        </p>
      </label>
    </div>
  );
}

/** Una propuesta: dos movimientos que podrían ser el mismo. */
function FilaCandidato({
  c,
  onEnlazar,
  enviando,
}: {
  c: CandidatoReemision;
  onEnlazar: (vigenteId: string, reemplazaA: string, motivo: string) => void;
  enviando: boolean;
}) {
  // Por defecto se queda el POSTERIOR: es el que el banco va a reportar en su saldo. Pero es solo
  // el valor inicial — si el nuevo entró con menos información, quien mira elige el otro.
  const [vigenteId, setVigenteId] = useState(c.posterior.id);
  const [motivo, setMotivo] = useState("");
  const grupo = `reemision-${c.anterior.id}`;
  const reemplazaA = vigenteId === c.posterior.id ? c.anterior.id : c.posterior.id;
  const motivoCorto = motivo.trim().length < 10;

  return (
    <div className="space-y-3 rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-lg font-semibold">
            {formatMonto(c.monto)} {c.es_debito ? "débito" : "crédito"}
          </span>
          <span className="text-sm text-content-muted">
            {c.banco} · {c.cuenta}
          </span>
        </div>
        {/* Los días son LA señal. Uno o dos días es una reemisión; siete o más, casi siempre una
            recurrencia legítima que no hay que tocar. */}
        <Badge tone={c.dias_entre <= 2 ? "pendiente" : "neutral"}>
          {c.dias_entre === 1 ? "al día siguiente" : `a ${c.dias_entre} días`}
        </Badge>
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        <LadoElegible
          lado={c.anterior}
          elegido={vigenteId === c.anterior.id}
          onElegir={() => setVigenteId(c.anterior.id)}
          idGrupo={grupo}
        />
        <LadoElegible
          lado={c.posterior}
          elegido={vigenteId === c.posterior.id}
          onElegir={() => setVigenteId(c.posterior.id)}
          idGrupo={grupo}
        />
      </div>

      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-[16rem] flex-1">
          <label className="mb-1 block text-xs text-content-muted" htmlFor={`${grupo}-motivo`}>
            ¿Por qué son el mismo movimiento?
          </label>
          <Input
            id={`${grupo}-motivo`}
            value={motivo}
            onChange={(e) => setMotivo(e.target.value)}
            placeholder="Ej.: el BAC reemitió el mismo crédito al día siguiente"
          />
        </div>
        <Button
          onClick={() => onEnlazar(vigenteId, reemplazaA, motivo.trim())}
          disabled={motivoCorto || enviando}
        >
          Es el mismo movimiento
        </Button>
      </div>
      {motivoCorto && motivo.length > 0 && (
        <p className="text-xs text-pendiente">
          Escribí un poco más: quien revise el mes tiene que entender por qué esa plata no está.
        </p>
      )}
    </div>
  );
}

export function ReemisionesPanel() {
  const empresaId = useEmpresaId();
  const qc = useQueryClient();
  const toast = useToast();

  const candQ = useQuery({
    queryKey: ["bancos", empresaId, "reemisiones", "candidatos"],
    queryFn: () => bancosApi.candidatosReemision(),
  });
  const hechasQ = useQuery({
    queryKey: ["bancos", empresaId, "reemisiones"],
    queryFn: () => bancosApi.reemisiones(),
  });

  const enlazar = useMutation({
    mutationFn: (v: { vigenteId: string; reemplazaA: string; motivo: string }) =>
      bancosApi.enlazarReemision(v.vigenteId, v.reemplazaA, v.motivo),
    onSuccess: () => {
      toast.success("Enlazados: ahora el monto cuenta una sola vez");
      void qc.invalidateQueries({ queryKey: ["bancos", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const deshacer = useMutation({
    mutationFn: (vigenteId: string) => bancosApi.deshacerReemision(vigenteId),
    onSuccess: () => {
      toast.success("Enlace deshecho: el movimiento volvió a los libros");
      void qc.invalidateQueries({ queryKey: ["bancos", empresaId] });
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  const candidatos = candQ.data?.items ?? [];
  const hechas = hechasQ.data?.items ?? [];
  const diasMax = candQ.data?.dias_max ?? 7;

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          El banco lo mandó dos veces
          {candidatos.length > 0 && (
            <span className="ml-2 text-base font-normal text-pendiente">
              {candidatos.length} por revisar
            </span>
          )}
        </CardTitle>
        <CardDescription>
          El mismo consecutivo y el mismo monto, en días distintos y con otra descripción: el banco
          reemitió la transacción. Al enlazarlos, <strong>uno sale de los libros</strong> y el otro
          se queda con la clasificación de los dos. Ninguno se borra ni se modifica, y se puede
          deshacer.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {candQ.isPending && <LoadingState label="Buscando movimientos reemitidos" />}
        {candQ.isError && <ErrorState message={mensajeError(candQ.error)} />}

        {!candQ.isPending && !candQ.isError && candidatos.length === 0 && (
          <p className="py-6 text-center text-sm text-content-muted">
            No hay movimientos que parezcan reemitidos en los últimos {diasMax} días.
          </p>
        )}

        {candidatos.length > 0 && (
          <>
            {/* El aviso va ARRIBA de la lista y no en la ayuda: el riesgo real acá no es dejar de
                enlazar algo, es enlazar dos cobros reales y hacer desaparecer plata de los libros.
                El ejemplo de la referencia no es inventado: el único candidato de los datos reales
                es un par de consumos de flota con R:033855 y R:034610, que son dos compras. */}
            <p className="rounded-md bg-surface-muted p-3 text-sm text-content-muted">
              Mirá la descripción antes de enlazar. Varios bancos reusan el consecutivo, así que dos
              compras distintas de días seguidos pueden verse casi iguales: lo que las separa suele
              ser la referencia o el número de autorización.
            </p>
            <div className="space-y-4">
              {candidatos.map((c) => (
                <FilaCandidato
                  key={`${c.anterior.id}-${c.posterior.id}`}
                  c={c}
                  enviando={enlazar.isPending}
                  onEnlazar={(vigenteId, reemplazaA, motivo) =>
                    enlazar.mutate({ vigenteId, reemplazaA, motivo })
                  }
                />
              ))}
            </div>
          </>
        )}

        {hechas.length > 0 && (
          <div className="space-y-2 border-t border-border pt-4">
            <h3 className="text-sm font-medium">Ya enlazados</h3>
            {hechas.map((r) => (
              <div
                key={r.vigente.id}
                className="flex flex-wrap items-start justify-between gap-3 rounded-md border border-border p-3"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">
                      {formatMonto(r.monto)} {r.es_debito ? "débito" : "crédito"}
                    </span>
                    <span className="text-sm text-content-muted">
                      {r.banco} · {r.cuenta}
                    </span>
                    {r.misma_clasificacion && (
                      <Badge tone="neutral">Misma clasificación</Badge>
                    )}
                  </div>
                  <p className="mt-1 text-sm">
                    Cuenta el del <strong>{formatFecha(r.vigente.fecha)}</strong>; salió de los
                    libros el del {formatFecha(r.reemplazado.fecha)}.
                  </p>
                  <p className="mt-1 text-xs text-content-muted">
                    {r.motivo}
                    {r.enlazado_por && ` — ${r.enlazado_por}`}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  onClick={() => deshacer.mutate(r.vigente.id)}
                  disabled={deshacer.isPending}
                >
                  Deshacer
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
