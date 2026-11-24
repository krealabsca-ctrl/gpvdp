/**
 * Configuración → Errores del sistema (/incidentes).
 *
 * EL PEDIDO (revisión comercial, punto 4): «nadie se entera si se cae». Sin Sentry, sin
 * OpenTelemetry, sin Prometheus: solo logs en el servidor. Con un cliente propio es aceptable; con
 * tres, te enterás de los problemas por teléfono.
 *
 * LO QUE HACE ESTA PANTALLA ES UN CANJE. Desde el 12-oct, cuando algo falla el usuario ve «Error
 * interno del sistema. Pasá esta referencia a soporte: 3F8A2C». Esa referencia quedaba escrita
 * ÚNICAMENTE en el log del contenedor —rotado, y accesible solo por SSH—, así que era un código
 * que no se podía canjear por nada. Acá se cambia por la causa exacta.
 *
 * POR QUÉ EL BUSCADOR VA PRIMERO Y NO LA LISTA. Quien abre esto casi siempre llega con un código
 * en la mano porque alguien lo llamó. La lista de «qué viene pasando» es la segunda pregunta, no
 * la primera, y ponerla arriba obliga a buscar entre ella lo que ya se sabe cómo nombrar.
 */

import { useState } from "react";

import {
  Badge,
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
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { formatFechaHora } from "@/lib/format";
import { useIncidentes, useResumenIncidentes } from "@/features/config/hooks";
import type { Incidente } from "@/api/sistema";

export function IncidentesPage() {
  // `ref` es lo que se teclea; `buscado` es lo que se consultó. Separados a propósito: con una
  // sola variable, cada letra de una referencia de seis dispararía su consulta —y las cinco
  // primeras devolverían «no es una referencia», que se lee como un error mientras se escribe.
  const [ref, setRef] = useState("");
  const [buscado, setBuscado] = useState("");
  const [soloPanico, setSoloPanico] = useState(false);

  const resumenQ = useResumenIncidentes();
  const listaQ = useIncidentes({ ref: buscado || undefined, panico: soloPanico, limite: 50 });

  const resumen = resumenQ.data;
  const items = listaQ.data?.items ?? [];

  function buscar() {
    setBuscado(ref.trim().toUpperCase());
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Errores del sistema"
        description="Cuando alguien dice «me dio error», acá se canjea la referencia que vio en pantalla por la causa exacta."
      />

      {/* El canje: lo primero, porque es con lo que se llega. */}
      <Card>
        <CardHeader>
          <CardTitle>Buscar por referencia</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <div className="flex flex-wrap items-end gap-2">
            <Input
              label="Referencia"
              value={ref}
              onChange={(e) => setRef(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && buscar()}
              placeholder="3F8A2C"
              className="font-mono uppercase"
            />
            <Button onClick={buscar}>Buscar</Button>
            {buscado && (
              <Button
                variant="ghost"
                onClick={() => {
                  setRef("");
                  setBuscado("");
                }}
              >
                Ver todos
              </Button>
            )}
            <label className="ml-auto flex items-center gap-2 text-xs text-content-muted">
              <input
                type="checkbox"
                checked={soloPanico}
                onChange={(e) => setSoloPanico(e.target.checked)}
                className="h-4 w-4 rounded border-border accent-accent"
              />
              Solo caídas del servidor
            </label>
          </div>
          <p className="text-xs text-content-muted">
            Son los seis caracteres del final del mensaje que el usuario tiene en pantalla. Da igual
            si los dicta en minúscula.
          </p>
        </CardContent>
      </Card>

      {/* «¿Está pasando ahora?» es la otra pregunta, y es la que un contador sin fecha no contesta. */}
      {resumen && (
        <Card>
          <CardContent className="flex flex-wrap items-center gap-6 pt-5">
            <Dato valor={resumen.ultima_hora} etiqueta="en la última hora" alarma={resumen.ultima_hora > 0} />
            <Dato valor={resumen.ultimo_dia} etiqueta="en el último día" />
            <Dato valor={resumen.ultima_semana} etiqueta="en la última semana" />
            <Dato
              valor={resumen.panicos}
              etiqueta="caídas del servidor (7 días)"
              alarma={resumen.panicos > 0}
            />
            <div className="ml-auto text-right">
              <p className="text-xs text-content-muted">Último error</p>
              <p className="text-sm text-content">
                {resumen.ultimo ? formatFechaHora(resumen.ultimo) : "ninguno registrado"}
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      {listaQ.isPending ? (
        <LoadingState label="Buscando" />
      ) : listaQ.isError ? (
        <ErrorState message={mensajeError(listaQ.error)} onRetry={() => listaQ.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          message={
            buscado
              ? `No hay ningún error con la referencia ${buscado}. Revisá que sean los seis caracteres completos del mensaje.`
              : "No hay errores registrados. Es la respuesta que se quiere."
          }
        />
      ) : (
        <div className="flex flex-col gap-3">
          {items.map((i) => (
            <FilaIncidente key={i.id} i={i} />
          ))}
          {items.length >= 50 && (
            // El tope se CONFIESA. Una lista cortada en silencio se lee como «esto es todo lo que
            // pasó», que es la conclusión contraria a la verdadera.
            <p className="text-xs text-content-muted">
              Se muestran los 50 más recientes. Buscá por referencia para encontrar uno puntual.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function Dato(props: { valor: number; etiqueta: string; alarma?: boolean }) {
  return (
    <div>
      <p className={`text-2xl font-semibold tabular-nums ${props.alarma ? "text-negativo" : "text-content"}`}>
        {props.valor}
      </p>
      <p className="text-xs text-content-muted">{props.etiqueta}</p>
    </div>
  );
}

function FilaIncidente({ i }: { i: Incidente }) {
  const [abierto, setAbierto] = useState(false);
  return (
    <Card>
      <CardContent className="flex flex-col gap-2 pt-5">
        <div className="flex flex-wrap items-center gap-2">
          <span className="rounded bg-surface-raised px-2 py-0.5 font-mono text-sm font-semibold text-content">
            {i.referencia}
          </span>
          {/* El panic se distingue a la vista: no es «algo falló al guardar», es que el servidor
              se cayó en medio de la respuesta. Son dos problemas de gravedad distinta. */}
          {i.panico ? (
            <Badge tone="negativo">se cayó el servidor</Badge>
          ) : (
            <Badge tone="pendiente">error {i.estado}</Badge>
          )}
          <span className="font-mono text-xs text-content-muted">
            {i.metodo} {i.ruta}
          </span>
          <span className="ml-auto text-xs text-content-muted">{formatFechaHora(i.creado_en)}</span>
        </div>

        <p className="break-words font-mono text-xs text-content">{i.detalle || "(sin detalle)"}</p>

        <div className="flex flex-wrap items-center gap-3 text-xs text-content-muted">
          {i.operacion && <span>al {i.operacion}</span>}
          {/* A quién le pasó: es con quien hay que volver a hablar para saber qué estaba haciendo. */}
          {i.usuario && <span>· le pasó a {i.usuario}</span>}
          {/* Un error sin empresa ocurrió ANTES de entrar. Decirlo evita que alguien lo busque en
              los datos de una empresa donde nunca estuvo. */}
          {!i.empresa_id && <span>· antes de elegir empresa</span>}
          {i.traza && (
            <button
              type="button"
              className="underline hover:text-accent"
              onClick={() => setAbierto((v) => !v)}
            >
              {abierto ? "ocultar el detalle técnico" : "ver el detalle técnico"}
            </button>
          )}
        </div>

        {abierto && i.traza && (
          <pre className="max-h-72 overflow-auto rounded border border-border bg-surface p-2 text-[11px] leading-relaxed text-content-muted">
            {i.traza}
          </pre>
        )}
      </CardContent>
    </Card>
  );
}
