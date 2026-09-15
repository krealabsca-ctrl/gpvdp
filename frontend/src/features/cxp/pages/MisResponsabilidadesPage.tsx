/**
 * CxP — Mis responsabilidades (/cxp/mis-responsabilidades).
 *
 * La lista corta: dos o tres filas con nombre propio, no cuatrocientas.
 *
 * Es la pantalla que decide si el módulo vive. La lista completa es de Contabilidad y nadie más la
 * va a abrir; lo que una persona de otra área sí abre es «qué me toca a mí este mes». Por eso no
 * pide permiso de módulo: negarle a alguien la lista de lo que él mismo tiene que cumplir no
 * protege nada y garantiza que no entre nunca.
 */

import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Badge,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  PageHeader,
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
} from "@/components/ui";
import { formatFecha, formatMoneda } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import { useMisResponsabilidades } from "@/features/cxp/hooks";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  ETIQUETA_RESPALDO,
  SEMAFORO,
  correrPeriodo,
  etiquetaPeriodo,
  tonoRespaldo,
} from "@/features/cxp/dominioResponsabilidad";

function periodoActualCR(): string {
  const d = new Date(Date.now() - 6 * 60 * 60 * 1000);
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

/** El orden en que urge mirar cada cosa. Lo resuelto se va al fondo. */
const URGENCIA: Record<string, number> = {
  VENCIDA: 0,
  POR_VENCER: 1,
  SIN_DATO: 2,
  AL_DIA: 3,
  NO_APLICA: 4,
  CUMPLIDA: 5,
  SIN_ABRIR: 0,
};

export function MisResponsabilidadesPage() {
  const [periodo, setPeriodo] = useState(periodoActualCR());
  const q = useMisResponsabilidades(periodo);
  const tiene = useTienePermiso();
  const puedeVerMas = tiene("cxp.responsabilidades.ver") || tiene("cxp.responsabilidades.ver_mias");

  const filas = [...(q.data ?? [])].sort(
    (a, b) => (URGENCIA[a.semaforo] ?? 9) - (URGENCIA[b.semaforo] ?? 9) || a.vence_en.localeCompare(b.vence_en),
  );
  const pendientes = filas.filter((f) => f.estado === "PENDIENTE").length;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Mis responsabilidades"
        description={
          pendientes === 0
            ? "Lo que me toca cumplir este mes."
            : `Tenés ${pendientes} ${pendientes === 1 ? "cosa pendiente" : "cosas pendientes"} este mes.`
        }
      />

      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="secondary" onClick={() => setPeriodo(correrPeriodo(periodo, -1))}>
          ‹ Mes anterior
        </Button>
        <span className="min-w-44 text-center text-base font-semibold tabular-nums text-content">
          {etiquetaPeriodo(periodo)}
        </span>
        <Button size="sm" variant="secondary" onClick={() => setPeriodo(correrPeriodo(periodo, 1))}>
          Mes siguiente ›
        </Button>
        {periodo !== periodoActualCR() && (
          <Button size="sm" variant="ghost" onClick={() => setPeriodo(periodoActualCR())}>
            Volver a hoy
          </Button>
        )}
      </div>

      {q.isPending ? (
        <LoadingState label="Cargando lo mío" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : filas.length === 0 ? (
        <EmptyState message="No tenés ninguna responsabilidad asignada en este mes.">
          <p className="max-w-lg text-sm text-content-muted">
            Acá aparece lo que alguien puso a tu nombre como titular o suplente. Si esperabas ver algo, pedile a
            Contabilidad que te asigne en{" "}
            <Link to="/cxp/responsabilidades" className="text-accent underline">
              Responsabilidades
            </Link>
            .
          </p>
        </EmptyState>
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Qué</TH>
                <TH>A quién</TH>
                <TH>Vence</TH>
                <TH className="text-right">Monto</TH>
                <TH>Estado</TH>
                <TH>Respaldo</TH>
              </TR>
            </THead>
            <TBody>
              {filas.map((f) => {
                const sem = SEMAFORO[f.semaforo];
                return (
                  <TR key={f.id}>
                    <TD className="font-medium text-content">{f.nombre}</TD>
                    <TD className="text-content-muted">{f.contraparte}</TD>
                    <TD className="tabular-nums">
                      {formatFecha(f.vence_en)}
                      {f.estado === "PENDIENTE" && f.dias_de_atraso > 0 && (
                        <span className="ml-1 text-xs text-negativo">
                          hace {f.dias_de_atraso} {f.dias_de_atraso === 1 ? "día" : "días"}
                        </span>
                      )}
                    </TD>
                    <TD className="text-right tabular-nums">
                      {formatMoneda(f.monto_esperado, f.moneda)}
                      {f.monto_tipo === "VARIABLE" && (
                        <span
                          className="ml-1 text-xs text-content-muted"
                          title="Monto de consumo: es una referencia, no autoriza un pago"
                        >
                          aprox.
                        </span>
                      )}
                    </TD>
                    <TD>
                      <Badge tone={sem.tono} title={sem.ayuda}>
                        {sem.etiqueta}
                      </Badge>
                    </TD>
                    <TD>
                      <Badge tone={tonoRespaldo(f.respaldo_tipo)}>
                        {ETIQUETA_RESPALDO[f.respaldo_tipo ?? "NINGUNO"]}
                      </Badge>
                    </TD>
                  </TR>
                );
              })}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {/* Puerta a la pantalla completa para quien tenga alcance más allá de lo suyo. El menú
          admite UN permiso por página, así que el segundo camino entra por acá. */}
      {puedeVerMas && (
        <Link to="/cxp/responsabilidades" className="text-sm text-accent underline">
          Ver todas las responsabilidades a mi alcance →
        </Link>
      )}

      {/* La verdad sobre los avisos, dicha donde alguien la va a leer. Prometer que el sistema
          avisa cuando no lo hace garantiza que en dos meses alguien diga «pero si me tenía que
          avisar». */}
      <p className="text-xs text-content-muted">
        ⓘ Por ahora esta pantalla hay que abrirla: el sistema todavía <b>no manda avisos por correo</b>. Eso llega en una
        etapa aparte, cuando se configure el correo saliente.
      </p>
    </div>
  );
}
