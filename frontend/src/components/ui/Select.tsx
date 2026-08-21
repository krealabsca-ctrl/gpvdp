import { forwardRef, useId, type SelectHTMLAttributes } from "react";
import { cn } from "@/lib/cn";

export interface SelectOption {
  value: string;
  label: string;
  /**
   * Opcional: agrupa la opción bajo un encabezado (`<optgroup>`).
   *
   * Sirve para no meter el nombre del grupo DENTRO de la etiqueta. Un desplegable nativo busca
   * por teclado comparando el principio del texto, así que 112 opciones que empiezan con
   * «Gastos › » vuelven imposible llegar a una escribiendo su nombre: todas calzan igual. Con el
   * grupo aparte, la etiqueta es «Gas» y escribir «gas» va directo.
   */
  grupo?: string;
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  error?: string;
  options: SelectOption[];
  placeholder?: string;
}

/**
 * Junta las opciones consecutivas del mismo grupo, conservando el orden recibido. Las que no
 * traen grupo quedan sueltas (clave ""), así que un Select sin grupos se dibuja igual que antes.
 */
function agrupar(options: SelectOption[]): [string, SelectOption[]][] {
  const salida: [string, SelectOption[]][] = [];
  for (const opt of options) {
    const g = opt.grupo ?? "";
    const ultimo = salida[salida.length - 1];
    if (ultimo && ultimo[0] === g) ultimo[1].push(opt);
    else salida.push([g, [opt]]);
  }
  return salida;
}

/** Select nativo estilizado (accesible por defecto, navegable por teclado). */
export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
  { className, label, error, options, placeholder, id, ...props },
  ref,
) {
  const autoId = useId();
  const selectId = id ?? autoId;

  return (
    <div className="flex flex-col gap-1.5">
      {label && (
        <label htmlFor={selectId} className="text-sm font-medium text-content">
          {label}
        </label>
      )}
      <select
        ref={ref}
        id={selectId}
        aria-invalid={error ? true : undefined}
        className={cn(
          "h-10 w-full rounded-lg border bg-surface-raised px-3 text-sm text-content shadow-sm",
          "transition-colors hover:border-content-muted/50 focus-visible:outline-none focus-visible:ring-2",
          "focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-surface",
          error ? "border-negativo" : "border-border",
          className,
        )}
        {...props}
      >
        {placeholder && (
          <option value="" disabled>
            {placeholder}
          </option>
        )}
        {agrupar(options).map(([grupo, opts]) =>
          grupo ? (
            <optgroup key={grupo} label={grupo}>
              {opts.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </optgroup>
          ) : (
            opts.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))
          ),
        )}
      </select>
      {error && <p className="text-sm text-negativo">{error}</p>}
    </div>
  );
});
