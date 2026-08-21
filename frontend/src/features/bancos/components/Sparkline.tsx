/**
 * Sparkline — la forma de una serie, del tamaño de una celda.
 *
 * SVG a mano y no un gráfico de la librería: en esta pantalla hay una por fila y pueden ser más de
 * cien. Montar cien gráficos con ejes, tooltip y responsive para dibujar seis puntos cuesta más que
 * la pantalla entera.
 *
 * Lo que comunica es la FORMA, no el valor: la escala es propia de cada serie (su mínimo y su
 * máximo), así que dos sparklines lado a lado no son comparables entre sí y no se les pone eje.
 * Los valores exactos viven en las columnas de al lado.
 */

interface SparklineProps {
  valores: number[];
  /** Índices que no cuentan (meses a medio clasificar): se dibujan como hueco. */
  huecos?: boolean[];
  ancho?: number;
  alto?: number;
  className?: string;
  /** Texto para lectores de pantalla: la serie no se «ve» sin él. */
  titulo?: string;
}

export function Sparkline({
  valores,
  huecos,
  ancho = 84,
  alto = 22,
  className,
  titulo,
}: SparklineProps) {
  if (valores.length < 2) return <span className="text-xs text-content-muted">—</span>;

  const min = Math.min(...valores, 0);
  const max = Math.max(...valores, 0);
  const rango = max - min || 1;
  const paso = valores.length > 1 ? ancho / (valores.length - 1) : ancho;
  const y = (v: number) => alto - 1 - ((v - min) / rango) * (alto - 2);

  const puntos = valores.map((v, i) => `${(i * paso).toFixed(1)},${y(v).toFixed(1)}`);
  const ultimoX = (valores.length - 1) * paso;
  const ultimoY = y(valores[valores.length - 1] ?? 0);
  // La línea del cero solo se dibuja cuando la serie la cruza: si todo es positivo, no aporta.
  const cruzaCero = min < 0 && max > 0;

  return (
    <svg
      width={ancho}
      height={alto}
      viewBox={`0 0 ${ancho} ${alto}`}
      className={className}
      role="img"
      aria-label={titulo ?? "tendencia"}
    >
      {cruzaCero && (
        <line
          x1={0}
          x2={ancho}
          y1={y(0)}
          y2={y(0)}
          stroke="currentColor"
          strokeWidth={0.5}
          className="text-border"
        />
      )}
      <polyline
        points={puntos.join(" ")}
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
        className="text-accent"
      />
      {/* Los meses que no cuentan se marcan con un círculo hueco: la línea los cruza, pero se ve
          que ese punto no es dato firme. */}
      {huecos?.map((esHueco, i) =>
        esHueco ? (
          <circle
            key={i}
            cx={i * paso}
            cy={y(valores[i] ?? 0)}
            r={2}
            fill="var(--color-surface, #fff)"
            stroke="currentColor"
            strokeWidth={1}
            className="text-content-muted"
          />
        ) : null,
      )}
      <circle cx={ultimoX} cy={ultimoY} r={2.2} fill="currentColor" className="text-accent" />
    </svg>
  );
}
