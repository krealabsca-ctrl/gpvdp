// Aplica el tema guardado ANTES de pintar, para evitar el parpadeo (FOUC).
//
// Vive en un archivo externo (no inline en index.html) a propósito: así la política de
// seguridad de contenido (CSP) puede exigir `script-src 'self'` SIN `'unsafe-inline'`, que
// es lo que de verdad frena la ejecución de scripts inyectados (XSS). Un script inline en el
// HTML obligaría a abrir esa puerta. Se sirve desde el mismo origen, es diminuto y bloquea el
// primer pintado igual que el inline, así que no reintroduce el parpadeo.
(function () {
  try {
    var t = localStorage.getItem("gpvdp.theme");
    var prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    if (t === "dark" || (!t && prefersDark)) {
      document.documentElement.classList.add("dark");
    }
  } catch (e) {
    /* no-op */
  }
})();
