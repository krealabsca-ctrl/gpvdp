/**
 * Bitácora de envíos del comprobante de pago (migración 0084).
 *
 * Contesta la pregunta que antes no tenía respuesta en ninguna pantalla: **a quién se le mandó el
 * comprobante, cuándo, con copia a quién y si salió**. Antes de la 0084 el sistema guardaba una
 * sola fecha (`comprobante_enviado_en`), que se pisaba en cada envío y no registraba los fallos:
 * un correo que rebotaba se veía exactamente igual que uno que llegó.
 *
 * Dos cosas que se leen acá y en ningún otro lado:
 *  · Los FALLOS quedan. Después de un 422 al enviar, la fila del error aparece en esta lista.
 *  · «El adjunto se reemplazó después» marca las filas cuyo PDF ya no es el que hay hoy en el
 *    expediente —el proveedor recibió otro documento—.
 *
 * Lista vacía = nunca se intentó. Los envíos anteriores a la migración no se rellenaron: decir a
 * qué dirección salieron sería inventarlo.
 */

import { AlertTriangle } from "lucide-react";
import { Badge, EmptyState, ErrorState, LoadingState } from "@/components/ui";
import { formatFechaHora } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useEnviosComprobante } from "@/features/cxp/hooks";
import {
  ETIQUETA_CATEGORIA_ENVIO,
  etiquetaIntento,
  frasePersonas,
  notaArchivo,
  tonoResultado,
} from "@/features/cxp/comprobanteDominio";

export function HistorialEnvios({ documentoId }: { documentoId: string }) {
  const q = useEnviosComprobante(documentoId);

  if (q.isPending) return <LoadingState label="Leyendo la bitácora de envíos" />;
  if (q.isError) {
    return <ErrorState message={mensajeError(q.error)} onRetry={() => void q.refetch()} />;
  }

  const envios = q.data ?? [];
  if (envios.length === 0) {
    return (
      <EmptyState message="Todavía no se intentó enviar este comprobante. Cuando se mande —salga o no— queda acá con el destinatario, la copia y la hora." />
    );
  }

  return (
    <ol className="flex flex-col gap-2">
      {envios.map((e) => (
        <li
          key={e.id}
          className="flex flex-col gap-1 rounded-lg border border-border bg-surface-raised px-3 py-2.5"
        >
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone={tonoResultado(e.resultado)}>
              {e.resultado === "OK" ? "Enviado" : "No salió"}
            </Badge>
            <span className="text-xs font-medium text-content">{etiquetaIntento(e.reenvio)}</span>
            <span className="text-xs text-content-muted">{formatFechaHora(e.enviado_en)}</span>
            {e.error_categoria && (
              <Badge tone="neutral">{ETIQUETA_CATEGORIA_ENVIO[e.error_categoria]}</Badge>
            )}
          </div>

          <p className="text-sm text-content">{frasePersonas(e.destinatario, e.copia)}</p>

          {/* La frase del backend, tal cual: ya está escrita para el operador de CxP y NO trae
              host, usuario ni código SMTP. El detalle técnico vive en Configuración › Correo
              saliente, que se lee con otro permiso. */}
          {e.error && (
            <div className="flex items-start gap-2 rounded border border-negativo/40 bg-negativo/5 px-2.5 py-1.5">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-negativo" aria-hidden />
              <p className="text-xs text-content">{e.error}</p>
            </div>
          )}

          <p className="text-xs text-content-muted">
            Desde {e.remitente} ({e.origen === "EMPRESA" ? "buzón de la empresa" : "buzón del grupo"})
            {" · "}
            <span className={e.mismo_archivo ? undefined : "text-pendiente"}>{notaArchivo(e)}</span>
            {e.enviado_por ? ` · lo mandó ${e.enviado_por}` : ""}
          </p>
        </li>
      ))}
    </ol>
  );
}
