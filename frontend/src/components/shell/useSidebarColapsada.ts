/**
 * Estado de la sidebar contextual: abierta o colapsada.
 *
 * ── POR QUÉ LA MEMORIA ES LA MITAD DE LA FUNCIÓN ────────────────────────────────────────────
 *
 * La sidebar ocupa 240 px fijos —el 18 % de una pantalla de 1366— y el pedido fue que no estorbe
 * en la operativa diaria. Pero si hay que colapsarla otra vez en cada ingreso, nadie la usa: el
 * costo de dos clics diarios supera al del espacio. Por eso la decisión se guarda POR USUARIO y
 * sobrevive al cierre de sesión.
 *
 * ── POR QUÉ POR USUARIO Y NO GLOBAL ─────────────────────────────────────────────────────────
 *
 * En la misma computadora puede trabajar más de una persona —en las sedes pasa—, y la tesorera
 * que vive en Clasificar la quiere cerrada mientras que quien entra una vez por semana la quiere
 * abierta. Una preferencia compartida haría que cada uno le cambie la pantalla al otro.
 *
 * `null` significa «este usuario nunca decidió», y es distinto de `false`: solo cuando es `null`
 * la pantalla puede proponer un valor por defecto (ver el auto-colapso en Sidebar).
 */

import { useCallback, useEffect, useState } from "react";

import { useAuth } from "@/features/auth/AuthContext";

const PREFIJO = "gpvdp:sidebar-colapsada:";

/** Lee la preferencia guardada. Devuelve null si el usuario nunca decidió. */
function leer(clave: string): boolean | null {
  try {
    const v = window.localStorage.getItem(clave);
    if (v === null) return null;
    return v === "1";
  } catch {
    // Modo privado o almacenamiento bloqueado: se trabaja sin memoria, no se rompe.
    return null;
  }
}

function escribir(clave: string, valor: boolean): void {
  try {
    window.localStorage.setItem(clave, valor ? "1" : "0");
  } catch {
    // Sin almacenamiento la preferencia dura lo que la sesión. Aceptable.
  }
}

export interface EstadoSidebar {
  /** Si la sidebar está colapsada ahora mismo. */
  colapsada: boolean;
  /** true cuando el usuario NUNCA tomó una decisión: habilita el valor por defecto por pantalla. */
  sinDecidir: boolean;
  /** Alterna y RECUERDA la decisión. */
  alternar: () => void;
  /** Propone un valor por defecto. No hace nada si el usuario ya decidió, ni se recuerda. */
  proponer: (colapsada: boolean) => void;
}

export function useSidebarColapsada(): EstadoSidebar {
  const { user } = useAuth();
  const clave = `${PREFIJO}${user?.id ?? "anonimo"}`;

  const [guardada, setGuardada] = useState<boolean | null>(() => leer(clave));
  const [propuesta, setPropuesta] = useState<boolean | null>(null);

  // Al cambiar de usuario hay que releer: si no, el segundo hereda la preferencia del primero.
  useEffect(() => {
    setGuardada(leer(clave));
    setPropuesta(null);
  }, [clave]);

  const alternar = useCallback(() => {
    setGuardada((previo) => {
      const siguiente = !(previo ?? propuesta ?? false);
      escribir(clave, siguiente);
      return siguiente;
    });
    // Una decisión explícita deja sin efecto cualquier propuesta de la pantalla.
    setPropuesta(null);
  }, [clave, propuesta]);

  const proponer = useCallback((valor: boolean) => {
    setPropuesta((previo) => (previo === valor ? previo : valor));
  }, []);

  // Ctrl/⌘ + B. Se ignora mientras se escribe en un campo, porque en un formulario el atajo
  // del navegador o del sistema operativo gana y moverle la pantalla a alguien que está
  // tecleando es exactamente lo que no se quiere.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== "b") return;
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable)) return;
      e.preventDefault();
      alternar();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [alternar]);

  return {
    colapsada: guardada ?? propuesta ?? false,
    sinDecidir: guardada === null,
    alternar,
    proponer,
  };
}
