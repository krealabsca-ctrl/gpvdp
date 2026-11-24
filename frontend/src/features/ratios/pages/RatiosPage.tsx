/**
 * Ciclo de efectivo (/ratios): DSO, DPO y el ciclo.
 *
 * ── QUÉ CONTESTA ────────────────────────────────────────────────────────────────────────────
 *
 * Cuántos días tardás en cobrar, cuántos en pagar, y cuántos días de plata propia financiás la
 * operación. Es lo primero que pregunta un banco o un due diligence.
 *
 * ── POR QUÉ CADA RATIO MUESTRA SU CÁLCULO ───────────────────────────────────────────────────
 *
 * Un DSO de 304 días puede significar «cobramos tardísimo» o «los cobros no se registran», y las
 * dos cosas se ven idénticas mirando la cifra. Por eso cada tarjeta trae su cartera, su base
 * mensual y su advertencia: el número sin eso se lleva a una junta y se discute como si fuera un
 * diagnóstico del negocio.
 */

import { useState } from "react";

import {
  Badge,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ErrorState,
  LoadingState,
  PageHeader,
  Select,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { mensajeError } from "@/lib/apiError";
import { formatMoneda } from "@/lib/format";
import { useCicloDeEfectivo } from "@/features/ratios/hooks";
import type { Ratio } from "@/api/ratios";

export function RatiosPage() {
  const [meses, setMeses] = useState(12);
  const q = useCicloDeEfectivo(meses);
  const c = q.data;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Ciclo de efectivo"
        description="Cuántos días tardás en cobrar, cuántos en pagar, y cuántos financiás vos."
        actions={
          <Select
            label="Ventana"
            id="ratios-ventana"
            value={String(meses)}
            onChange={(e) => setMeses(Number(e.target.value))}
            options={[
              { value: "3", label: "Últimos 3 meses" },
              { value: "6", label: "Últimos 6 meses" },
              { value: "12", label: "Últimos 12 meses" },
              { value: "24", label: "Últimos 24 meses" },
            ]}
          />
        }
      />

      {q.isPending ? (
        <LoadingState label="Midiendo el ciclo" />
      ) : q.isError ? (
        <ErrorState message={mensajeError(q.error)} onRetry={() => q.refetch()} />
      ) : !c ? null : (
        <>
          {c.avisos.length > 0 && (
            <div className="flex flex-col gap-2 rounded-lg border border-pendiente/40 bg-pendiente/10 px-4 py-3">
              {c.avisos.map((a, i) => (
                <p key={i} className="text-sm text-content-muted">
                  {a}
                </p>
              ))}
            </div>
          )}

          <div className="grid gap-4 md:grid-cols-3">
            <TarjetaRatio
              titulo="DSO · días para cobrar"
              explica="Lo que te falta cobrar, medido en días de facturación real."
              etiquetaCartera="Cartera por cobrar"
              r={c.dso}
              malo="alto"
            />
            <TarjetaRatio
              titulo="DPO · días para pagar"
              explica="Lo que te falta pagar, medido en días de compras reales."
              etiquetaCartera="Cartera por pagar"
              r={c.dpo}
              malo="bajo"
            />
            <TarjetaCiclo ciclo={c.ciclo} calculable={c.ciclo_calculable} dio={c.dio} />
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Cómo se calculó</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-3 text-sm text-content-muted">
              <p>
                Ventana del <b className="text-content">{c.desde}</b> al{" "}
                <b className="text-content">{c.hasta}</b>.
              </p>
              <p>
                El número principal se calcula por <b className="text-content">agotamiento</b>: la
                cartera se va descontando contra lo facturado del mes más reciente, después del
                anterior, hasta agotarla. No promedia, y por eso resiste meses dispares — acá los
                hay, de ₡73 a ₡187 millones.
              </p>
              <p>
                Al lado va la fórmula de libro (cartera ÷ promedio × días). Cuando las dos se
                separan, está diciendo que{" "}
                <b className="text-content">los últimos meses no se parecen al promedio</b>, y eso
                es información sobre el negocio, no sobre el método.
              </p>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}

/**
 * Qué decir de un ratio según su valor, y de qué color.
 *
 * ── EL DPO SE VIGILA EN LOS DOS EXTREMOS, Y ESO NO ES SIMÉTRICO ─────────────────────────────
 *
 * Un DPO muy BAJO significa que pagás casi al contado: estás financiando vos la operación
 * pudiendo no hacerlo. Un DPO muy ALTO parece la posición cómoda —te financia el proveedor— y por
 * eso es el que nadie marca, pero a 8 meses de atraso el proveedor te sube el precio, te pide
 * contado o te deja de vender. Que el número «se vea bien» es justamente el riesgo.
 *
 * El DSO solo tiene un lado malo: cobrar tarde. Cobrar rápido nunca es un problema.
 *
 * Los umbrales son de criterio general, no de la empresa: 180 días para cobrar y 120 para pagar
 * son señales en cualquier industria. Si el negocio quiere los suyos, se parametrizan.
 */
function alertaDe(malo: "alto" | "bajo", dias: number): { clase: string; texto: string } | null {
  if (malo === "alto") {
    if (dias > 180) {
      return {
        clase: "text-negativo",
        texto: "Más de seis meses para cobrar: la cartera está financiando a los clientes.",
      };
    }
    return null;
  }
  if (dias < 15) {
    return {
      clase: "text-pendiente",
      texto: "Pagás casi al contado: estás financiando la operación pudiendo no hacerlo.",
    };
  }
  if (dias > 120) {
    return {
      clase: "text-negativo",
      texto:
        "Más de cuatro meses para pagar. No es gratis: a este atraso el proveedor sube el precio, pide contado o deja de vender.",
    };
  }
  return null;
}

/** Una tarjeta de ratio con su cálculo a la vista. */
function TarjetaRatio({
  titulo,
  explica,
  etiquetaCartera,
  r,
  malo,
}: {
  titulo: string;
  explica: string;
  etiquetaCartera: string;
  r: Ratio;
  /** Hacia qué lado el número es mala noticia: el DSO alto es malo, el DPO bajo es malo. */
  malo: "alto" | "bajo";
}) {
  if (!r.calculable) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{titulo}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-content-muted">{r.aviso || "No se puede calcular todavía."}</p>
        </CardContent>
      </Card>
    );
  }

  const dias = Number(r.dias);
  const simple = Number(r.dias_simple);
  // La brecha entre los dos métodos, que es el dato que nadie mira y debería.
  const brecha = Math.abs(dias - simple);
  const alerta = alertaDe(malo, dias);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{titulo}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div>
          <p
            className={cn(
              "text-3xl font-semibold tabular-nums",
              alerta ? alerta.clase : "text-content",
            )}
          >
            {r.dias} <span className="text-base font-normal text-content-muted">días</span>
          </p>
          <p className="text-xs text-content-muted">{explica}</p>
          {alerta && <p className={cn("mt-1 text-xs", alerta.clase)}>{alerta.texto}</p>}
        </div>

        <div className="flex flex-col gap-1 border-t border-border pt-3 text-xs text-content-muted">
          <div className="flex justify-between gap-2">
            <span>{etiquetaCartera}</span>
            <span className="tabular-nums text-content">{formatMoneda(r.cartera)}</span>
          </div>
          <div className="flex justify-between gap-2">
            <span>Base mensual</span>
            <span className="tabular-nums text-content">{formatMoneda(r.base_mensual)}</span>
          </div>
          <div className="flex justify-between gap-2">
            <span>Fórmula simple</span>
            <span className="tabular-nums text-content">
              {r.dias_simple} días
              {brecha >= 10 && (
                <Badge tone="pendiente" className="ml-2">
                  {brecha.toFixed(0)} de brecha
                </Badge>
              )}
            </span>
          </div>
          <div className="flex justify-between gap-2">
            <span>Meses que hizo falta agotar</span>
            <span className="tabular-nums text-content">{r.meses_usados}</span>
          </div>
        </div>

        {r.aviso && <p className="text-xs text-pendiente">{r.aviso}</p>}
      </CardContent>
    </Card>
  );
}

/** El ciclo, con su lectura: el signo es lo que importa. */
function TarjetaCiclo({
  ciclo,
  calculable,
  dio,
}: {
  ciclo: string;
  calculable: boolean;
  dio: Ratio;
}) {
  if (!calculable) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Ciclo de efectivo</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-content-muted">
            Hace falta el DSO y el DPO para calcularlo.
          </p>
        </CardContent>
      </Card>
    );
  }
  const n = Number(ciclo);
  return (
    <Card>
      <CardHeader>
        <CardTitle>Ciclo de efectivo</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div>
          <p
            className={cn(
              "text-3xl font-semibold tabular-nums",
              n > 0 ? "text-negativo" : "text-positivo",
            )}
          >
            {ciclo} <span className="text-base font-normal text-content-muted">días</span>
          </p>
          {/* EL SIGNO ES LA LECTURA, y sin traducirlo no significa nada para quien no lo usa a
              diario: positivo = ponés vos la plata; negativo = la ponen tus proveedores. */}
          <p className="text-xs text-content-muted">
            {n > 0
              ? "Financiás la operación con plata propia durante ese tiempo."
              : "Tus proveedores financian la operación: cobrás antes de pagar."}
          </p>
        </div>
        <p className="border-t border-border pt-3 text-xs text-content-muted">
          Es DSO − DPO. {dio.aviso || "Le falta el inventario (DIO) para ser el ciclo completo."}
        </p>
      </CardContent>
    </Card>
  );
}
