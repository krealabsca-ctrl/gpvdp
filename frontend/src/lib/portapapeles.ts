/**
 * Copiar al portapapeles, con la caída que hace falta de verdad.
 *
 * `navigator.clipboard` solo existe en un contexto SEGURO: https, o localhost. El ERP se usa
 * también por la red de la oficina contra una IP por http, y ahí `navigator.clipboard` es
 * `undefined` — el botón no haría nada y nadie sabría por qué. Por eso la caída al textarea con
 * `execCommand`, que está deprecado pero es lo único que funciona en ese caso.
 *
 * Devuelve si se copió, para que quien llama pueda decirlo en vez de fingir que sí.
 */
export async function copiarAlPortapapeles(texto: string): Promise<boolean> {
  const t = String(texto ?? "");
  if (t === "") return false;

  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(t);
      return true;
    } catch {
      // Permiso denegado o documento sin foco: se sigue con la caída de abajo.
    }
  }

  try {
    const ta = document.createElement("textarea");
    ta.value = t;
    // Fuera de la vista pero NO con display:none ni visibility:hidden: un elemento oculto así no se
    // puede seleccionar y la copia falla en silencio.
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    ta.remove();
    return ok;
  } catch {
    return false;
  }
}
