/**
 * Pantalla — Notificaciones (/notificaciones).
 *
 * El texto de los correos que el sistema envía es CONFIGURACIÓN de cada empresa: el comprobante
 * de pago al proveedor, la boleta al colaborador y el aviso de vacaciones. Acá se edita, se ve
 * cómo queda con datos de ejemplo y se puede volver al texto de fábrica.
 *
 * Los campos entre corchetes son variables que el sistema reemplaza al enviar. Solo se aceptan
 * las que declara cada notificación: si el texto usa una inventada, el backend no lo guarda (es
 * mejor un error acá que un «[FOO]» en el correo de un proveedor).
 */

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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
  useToast,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { mensajeError } from "@/lib/apiError";
import { useAuth } from "@/features/auth/AuthContext";
import { plantillasApi, type TipoNotificacion, type VistaPreviaCorreo } from "@/api/plantillas";

export function NotificacionesPage() {
  const { empresaActiva } = useAuth();
  const empresaId = empresaActiva?.id ?? "none";
  const q = useQuery({
    queryKey: ["plantillas", empresaId],
    queryFn: () => plantillasApi.listar(),
    staleTime: 60_000,
  });
  const [abierta, setAbierta] = useState<string | null>(null);

  // Al llegar los tipos se abre el primero: la pantalla nunca se ve vacía.
  useEffect(() => {
    const primera = q.data?.[0];
    if (!abierta && primera) setAbierta(primera.clave);
  }, [q.data, abierta]);

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Notificaciones"
        description="El texto de los correos que envía el sistema. Cada empresa se comunica a su manera."
      />

      {q.isPending ? (
        <LoadingState label="Cargando las plantillas…" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />
      ) : (
        <div className="flex flex-col gap-4">
          {(q.data ?? []).map((tipo) => (
            <EditorPlantilla
              key={tipo.clave}
              tipo={tipo}
              abierta={abierta === tipo.clave}
              onAbrir={() => setAbierta(abierta === tipo.clave ? null : tipo.clave)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function EditorPlantilla({
  tipo,
  abierta,
  onAbrir,
}: {
  tipo: TipoNotificacion;
  abierta: boolean;
  onAbrir: () => void;
}) {
  const toast = useToast();
  const qc = useQueryClient();
  const { empresaActiva } = useAuth();
  const empresaId = empresaActiva?.id ?? "none";

  const [asunto, setAsunto] = useState(tipo.vigente.asunto);
  const [cuerpo, setCuerpo] = useState(tipo.vigente.cuerpo);
  const [previa, setPrevia] = useState<VistaPreviaCorreo | null>(null);
  const [confirmandoReset, setConfirmandoReset] = useState(false);

  // Si el servidor trae otro texto (se guardó o se restableció), el editor se sincroniza.
  useEffect(() => {
    setAsunto(tipo.vigente.asunto);
    setCuerpo(tipo.vigente.cuerpo);
    setPrevia(null);
  }, [tipo.vigente.asunto, tipo.vigente.cuerpo]);

  const invalidar = () => qc.invalidateQueries({ queryKey: ["plantillas", empresaId] });

  const guardar = useMutation({
    mutationFn: () => plantillasApi.guardar(tipo.clave, asunto, cuerpo),
    onSuccess: () => {
      toast.success(`«${tipo.nombre}» guardada. Los próximos correos usan este texto.`);
      void invalidar();
    },
    onError: (err) => toast.error(mensajeError(err)),
  });

  const restablecer = useMutation({
    mutationFn: () => plantillasApi.restablecer(tipo.clave),
    onSuccess: () => {
      toast.success("Volvió al texto de fábrica.");
      setConfirmandoReset(false);
      void invalidar();
    },
    onError: (err) => {
      toast.error(mensajeError(err));
      setConfirmandoReset(false);
    },
  });

  const verPrevia = useMutation({
    mutationFn: () => plantillasApi.vistaPrevia(tipo.clave, asunto, cuerpo),
    onSuccess: setPrevia,
    onError: (err) => toast.error(mensajeError(err)),
  });

  // Variables del texto que no existen: se avisa ANTES de intentar guardar.
  const desconocidas = useMemo(() => {
    const permitidas = new Set(tipo.variables.map((v) => v.nombre));
    const usadas = new Set<string>();
    for (const m of `${asunto}\n${cuerpo}`.matchAll(/\[([A-Z0-9_]+)\]/g)) {
      if (m[1]) usadas.add(m[1]);
    }
    return [...usadas].filter((v) => !permitidas.has(v)).sort();
  }, [asunto, cuerpo, tipo.variables]);

  const cambiado = asunto !== tipo.vigente.asunto || cuerpo !== tipo.vigente.cuerpo;

  /** Inserta la variable al final del cuerpo (o en el cursor si el textarea está enfocado). */
  function insertar(nombre: string) {
    const marca = `[${nombre}]`;
    const ta = document.getElementById(`cuerpo-${tipo.clave}`) as HTMLTextAreaElement | null;
    if (ta && document.activeElement === ta) {
      const { selectionStart: ini, selectionEnd: fin } = ta;
      setCuerpo(cuerpo.slice(0, ini) + marca + cuerpo.slice(fin));
      // Deja el cursor después de lo insertado.
      requestAnimationFrame(() => {
        ta.focus();
        ta.setSelectionRange(ini + marca.length, ini + marca.length);
      });
      return;
    }
    setCuerpo(cuerpo + marca);
  }

  return (
    <Card>
      <CardHeader className="flex flex-wrap items-start justify-between gap-3">
        <button type="button" onClick={onAbrir} className="min-w-0 flex-1 text-left" aria-expanded={abierta}>
          <div className="flex flex-wrap items-center gap-2">
            <CardTitle>{tipo.nombre}</CardTitle>
            <Badge tone="neutral">{tipo.modulo}</Badge>
            {tipo.vigente.personalizada ? (
              <Badge tone="accent">Personalizada</Badge>
            ) : (
              <Badge tone="neutral">Texto de fábrica</Badge>
            )}
          </div>
          <p className="mt-0.5 text-xs text-content-muted">{tipo.descripcion}</p>
          {tipo.vigente.personalizada && tipo.vigente.actualizado_en && (
            <p className="mt-0.5 text-xs text-content-muted">
              Última edición: {tipo.vigente.actualizado_en.slice(0, 10)}
              {tipo.vigente.actualizado_por && ` · ${tipo.vigente.actualizado_por}`}
            </p>
          )}
        </button>
        <Button variant="ghost" onClick={onAbrir}>
          {abierta ? "Cerrar" : "Editar"}
        </Button>
      </CardHeader>

      {abierta && (
        <CardContent className="flex flex-col gap-4 border-t border-border pt-4">
          {/* Variables disponibles */}
          <div>
            <p className="text-xs text-content-muted">
              Variables disponibles — hacé clic para insertarlas donde tengas el cursor:
            </p>
            <div className="mt-2 grid grid-cols-1 gap-1.5 sm:grid-cols-2">
              {tipo.variables.map((v) => (
                <button
                  key={v.nombre}
                  type="button"
                  onClick={() => insertar(v.nombre)}
                  title={`Ejemplo: ${v.ejemplo}`}
                  className="flex items-baseline gap-2 rounded-lg border border-border px-2.5 py-1.5 text-left transition-colors hover:bg-surface-muted"
                >
                  <code className="shrink-0 rounded bg-surface-muted px-1.5 py-0.5 font-mono text-xs text-accent">
                    [{v.nombre}]
                  </code>
                  <span className="truncate text-xs text-content-muted">{v.descripcion}</span>
                </button>
              ))}
            </div>
          </div>

          <Input
            label="Asunto del correo"
            value={asunto}
            onChange={(e) => setAsunto(e.target.value)}
            className="font-mono text-sm"
          />

          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-content">Contenido del correo</span>
            <textarea
              id={`cuerpo-${tipo.clave}`}
              value={cuerpo}
              onChange={(e) => setCuerpo(e.target.value)}
              rows={14}
              className={cn(
                "w-full rounded-lg border border-border bg-surface-raised px-3 py-2 font-mono text-sm text-content shadow-sm",
                "transition-colors hover:border-content-muted/50 focus-visible:outline-none focus-visible:ring-2",
                "focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-surface",
              )}
            />
          </label>

          {desconocidas.length > 0 && (
            <p className="rounded-lg border border-negativo/40 bg-negativo/10 px-3 py-2 text-xs text-content">
              Estas variables no existen y el sistema no las puede llenar:{" "}
              <span className="font-mono">[{desconocidas.join("], [")}]</span>. Quitalas o usá una de
              la lista.
            </p>
          )}

          <div className="flex flex-wrap items-center gap-3">
            <Button variant="secondary" onClick={() => verPrevia.mutate()} loading={verPrevia.isPending}>
              Vista previa del correo
            </Button>
            <Button
              onClick={() => guardar.mutate()}
              loading={guardar.isPending}
              disabled={desconocidas.length > 0 || !cambiado || !asunto.trim() || !cuerpo.trim()}
            >
              {cambiado ? "Guardar plantilla" : "Sin cambios"}
            </Button>
            {tipo.vigente.personalizada && (
              <Button variant="ghost" onClick={() => setConfirmandoReset(true)}>
                Volver al texto de fábrica
              </Button>
            )}
            {cambiado && (
              <Button
                variant="ghost"
                onClick={() => {
                  setAsunto(tipo.vigente.asunto);
                  setCuerpo(tipo.vigente.cuerpo);
                  setPrevia(null);
                }}
              >
                Descartar cambios
              </Button>
            )}
          </div>

          {previa && (
            <div className="rounded-xl border border-border bg-surface-muted p-4">
              <p className="text-xs uppercase tracking-wide text-content-muted">
                Vista previa con datos de ejemplo
              </p>
              <p className="mt-2 text-sm">
                <span className="text-content-muted">Asunto: </span>
                <span className="font-medium text-content">{previa.asunto}</span>
              </p>
              <pre className="mt-3 whitespace-pre-wrap break-words border-t border-border pt-3 text-sm text-content">
                {previa.cuerpo}
              </pre>
            </div>
          )}
        </CardContent>
      )}

      {confirmandoReset && (
        <ConfirmDialog
          titulo={`Volver al texto de fábrica: ${tipo.nombre}`}
          descripcion="Se borra la versión personalizada de esta empresa y los próximos correos saldrán con el texto que trae el sistema."
          textoConfirmar="Restablecer"
          tono="peligro"
          pendiente={restablecer.isPending}
          onConfirmar={() => restablecer.mutate()}
          onCancelar={() => setConfirmandoReset(false)}
        />
      )}
    </Card>
  );
}
