import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    host: true, // escuchar en 0.0.0.0 dentro del contenedor
    port: 5173,
    // La API se sirve por el MISMO origen que la página: el navegador pide /v1/... a quien le
    // sirvió el HTML y este proxy lo reenvía al backend. Con eso el sistema funciona igual por
    // localhost, por IP de la red o por nombre de máquina, y deja de romperse cuando el router
    // le cambia la IP a esta computadora (pasó: 192.168.1.115 → .171 y nadie pudo entrar).
    // Además desaparece el CORS, que es la otra mitad del mismo problema.
    // En producción hace lo mismo el reverse proxy; la arquitectura es la misma en los dos lados.
    proxy: {
      "/v1": {
        target: "http://backend:8080", // nombre del servicio en la red de Docker
        changeOrigin: true,
      },
    },
  },
  // @ts-expect-error — 'test' lo añade Vitest al config de Vite en tiempo de ejecución
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    css: false,
  },
});
