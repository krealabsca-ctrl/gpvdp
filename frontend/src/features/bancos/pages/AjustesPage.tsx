/**
 * Pantalla — Ajustes (/ajustes), Fase D.
 * Parámetros de negocio por empresa. Hoy: tolerancia de traslado (proporción del
 * monto que puede diferir entre las dos patas de un traslado/overnight para
 * emparejarlas). Editar requiere rol autorizado (el backend valida el rol y el
 * rango 0–5%). El cierre bloqueante se muestra de solo lectura (es global).
 */

import { useEffect, useState, type FormEvent } from "react";
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
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { useActualizarTolerancia, useParametros } from "@/features/bancos/hooks";

export function AjustesPage() {
  const toast = useToast();
  const q = useParametros();
  const actualizar = useActualizarTolerancia();
  const [pct, setPct] = useState("");

  // Prellena el campo cuando llegan los parámetros.
  useEffect(() => {
    if (q.data) setPct(q.data.tolerancia_traslado_pct);
  }, [q.data]);

  function guardar(e: FormEvent) {
    e.preventDefault();
    if (pct.trim() === "" || Number.isNaN(Number(pct))) {
      toast.error("Ingresá un porcentaje válido (ej. 1 o 1.5).");
      return;
    }
    actualizar.mutate(pct.trim(), {
      onSuccess: () => toast.success("Tolerancia de traslado actualizada."),
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Ajustes"
        description="Parámetros de negocio de la empresa activa."
      />

      {q.isPending ? (
        <LoadingState label="Cargando parámetros" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Tolerancia de traslado</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="mb-3 text-sm text-content-muted">
                Diferencia máxima permitida entre el débito y el crédito de un par de traslado/overnight
                para proponerlo y emparejarlo. Se aplica sobre el monto mayor de las dos patas (útil por el
                diferencial cambiario en dólares). Rango válido: 0% a 5%.
              </p>
              <form onSubmit={guardar} className="flex items-end gap-3">
                <Input
                  label="Tolerancia (%)"
                  value={pct}
                  onChange={(e) => setPct(e.target.value)}
                  inputMode="decimal"
                  className="w-32 tabular-nums"
                  placeholder="1"
                />
                <Button type="submit" loading={actualizar.isPending}>
                  Guardar
                </Button>
              </form>
              <p className="mt-2 text-xs text-content-muted">
                Editar la tolerancia requiere rol autorizado (Director Financiero o Admin).
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Cierre de período</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-2">
              <div className="flex items-center gap-2">
                <Badge tone={q.data!.cierre_bloqueante ? "positivo" : "pendiente"}>
                  {q.data!.cierre_bloqueante ? "Bloqueante" : "Solo advierte"}
                </Badge>
                <span className="text-sm text-content-muted">
                  {q.data!.cierre_bloqueante
                    ? "No se puede cerrar un período con movimientos «No identificado»."
                    : "El cierre avisa pero no bloquea."}
                </span>
              </div>
              <p className="text-xs text-content-muted">
                Este parámetro es global de la instalación (no por empresa) y se administra por configuración.
              </p>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}
