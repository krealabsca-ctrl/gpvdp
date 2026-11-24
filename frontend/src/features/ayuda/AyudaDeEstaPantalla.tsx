/**
 * El botón «?» del encabezado y su panel.
 *
 * VIVE EN `PageHeader`, Y ESA ES LA DECISIÓN QUE IMPORTA. Las 68 pantallas del sistema ya usan
 * ese componente, así que la ayuda aparece en todas sin editar ninguna: no hay que acordarse de
 * agregarla, no hay pantallas que queden sin ella por olvido, y el día que se escriba la ayuda de
 * una pantalla nueva aparece sola.
 *
 * El botón se dibuja SOLO donde hay ayuda escrita (ver pantallas.ts). Un «?» que abre «no hay
 * ayuda para esta pantalla» enseña a no hacer clic, y entonces tampoco se usa donde sí la hay.
 */

import { useState } from "react";
import { Link, useLocation } from "react-router-dom";

import { ayudaDe } from "@/features/ayuda/pantallas";

export function AyudaDeEstaPantalla() {
  const { pathname } = useLocation();
  const [abierto, setAbierto] = useState(false);

  // La ruta exacta: una pantalla de detalle («/cxp/documentos/abc») no hereda la ayuda del
  // listado, porque casi nunca es la misma explicación.
  const ayuda = ayudaDe(pathname);
  if (!ayuda) return null;

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setAbierto((v) => !v)}
        aria-label="Qué se hace en esta pantalla"
        aria-expanded={abierto}
        className="flex h-7 w-7 items-center justify-center rounded-full border border-border text-sm font-semibold text-content-muted transition-colors hover:border-accent hover:text-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        title="Qué se hace en esta pantalla"
      >
        ?
      </button>

      {abierto && (
        <>
          {/* Cerrar al tocar afuera. Un panel de ayuda que hay que cerrar con una X se queda
              abierto tapando justo la pantalla que explica. */}
          <button
            type="button"
            aria-label="Cerrar la ayuda"
            className="fixed inset-0 z-40 cursor-default"
            onClick={() => setAbierto(false)}
          />
          {/* Se despliega hacia la DERECHA (`left-0`) y no hacia la izquierda. El botón vive
              pegado al título, o sea en el borde izquierdo del área de contenido: con `right-0`
              el panel se metía debajo del menú lateral y el texto quedaba cortado. */}
          <div className="absolute left-0 top-9 z-50 w-[min(26rem,calc(100vw-2rem))] rounded-xl border border-border bg-surface-raised p-4 shadow-lifted">
            <p className="text-sm text-content">{ayuda.queEs}</p>
            <ul className="mt-3 flex flex-col gap-2">
              {ayuda.puntos.map((p, i) => (
                <li key={i} className="flex gap-2 text-xs leading-relaxed text-content-muted">
                  <span className="mt-1.5 h-1 w-1 shrink-0 rounded-full bg-accent" />
                  <span>{p}</span>
                </li>
              ))}
            </ul>
            {ayuda.guia && (
              <Link
                to={`/ayuda?guia=${ayuda.guia}`}
                onClick={() => setAbierto(false)}
                className="mt-3 inline-block text-xs font-medium text-accent underline hover:text-accent"
              >
                Ver la guía completa →
              </Link>
            )}
          </div>
        </>
      )}
    </div>
  );
}
