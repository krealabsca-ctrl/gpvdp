import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * La pantalla de consultas lentas (mig 0120).
 *
 * Lo que se protege es que los TRES vacíos se distingan, porque significan cosas opuestas y el
 * arreglo de cada uno es distinto:
 *
 *   - El contador APAGADO: el servidor arrancó sin la extensión. Hay que encenderla.
 *   - El contador ENCENDIDO y sin datos todavía: hay que usar el sistema y volver.
 *   - Un fallo de verdad.
 *
 * Mostrar los tres como una tabla vacía haría que «no hay consultas lentas» —que es lo que se
 * lee— fuera mentira en dos de los tres casos, justo cuando alguien está buscando por qué el
 * sistema va lento.
 */

vi.mock("@/api/sistema", () => ({
  sistemaApi: { consultasLentas: vi.fn(), reiniciarMedicion: vi.fn() },
}));
vi.mock("@/features/bancos/useEmpresaId", () => ({ useEmpresaId: () => "e1" }));

import { sistemaApi } from "@/api/sistema";
import { ToastProvider } from "@/components/ui";
import { ConsultasLentasPage } from "@/features/config/pages/ConsultasLentasPage";

// MemoryRouter porque `PageHeader` lleva el botón de ayuda, que navega.
function envolver(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <MemoryRouter>{children}</MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>
  );
}

const UNA_CONSULTA = {
  consulta: "SELECT COALESCE(SUM(d.total_crc), $1) FROM documento_cxp d WHERE d.empresa_id = $2",
  llamadas: 22,
  ms_total: 504,
  ms_media: 23,
  ms_max: 85,
  porcentaje_del_total: 26.7,
  filas_por_llamada: 1,
};

describe("ConsultasLentasPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("explica cómo encender el contador cuando está apagado", async () => {
    vi.mocked(sistemaApi.consultasLentas).mockResolvedValue({
      disponible: false,
      motivo:
        "El contador de consultas no está activo en este servidor. Se enciende arrancando PostgreSQL con shared_preload_libraries=pg_stat_statements.",
      consultas: [],
      midiendo_desde: "",
    });

    render(envolver(<ConsultasLentasPage />));

    // El TÍTULO dice «El contador no está activo»; el motivo dice «El contador de consultas no
    // está activo…». Se busca el título para no empatar con los dos.
    expect(await screen.findByText(/^El contador no está activo/i)).toBeInTheDocument();
    // Y dice QUÉ hacer: quien abre esta pantalla es quien puede arreglarlo.
    expect(screen.getByText(/shared_preload_libraries/)).toBeInTheDocument();
    // NO se ofrece reiniciar una medición que no existe.
    expect(screen.queryByRole("button", { name: /reiniciar/i })).not.toBeInTheDocument();
  });

  it("distingue «encendido pero sin datos» de «apagado»", async () => {
    vi.mocked(sistemaApi.consultasLentas).mockResolvedValue({
      disponible: true,
      motivo: "",
      consultas: [],
      midiendo_desde: "2026-11-11 08:00",
    });

    render(envolver(<ConsultasLentasPage />));

    expect(await screen.findByText(/todavía no hay consultas medidas/i)).toBeInTheDocument();
    expect(screen.queryByText(/^El contador no está activo/i)).not.toBeInTheDocument();
  });

  it("muestra la consulta con su tiempo y desde cuándo mide", async () => {
    vi.mocked(sistemaApi.consultasLentas).mockResolvedValue({
      disponible: true,
      motivo: "",
      consultas: [UNA_CONSULTA],
      midiendo_desde: "2026-11-11 08:00",
    });

    render(envolver(<ConsultasLentasPage />));

    expect(await screen.findByText(/FROM documento_cxp/)).toBeInTheDocument();
    expect(screen.getByText("504 ms")).toBeInTheDocument();
    expect(screen.getByText("26.7 %")).toBeInTheDocument();
    // La fecha de inicio es lo que hace interpretable el total: 504 ms en una hora o en tres
    // semanas no significan lo mismo.
    expect(screen.getByText("2026-11-11 08:00")).toBeInTheDocument();
  });

  it("convierte los milisegundos a una unidad legible", async () => {
    vi.mocked(sistemaApi.consultasLentas).mockResolvedValue({
      disponible: true,
      motivo: "",
      consultas: [
        { ...UNA_CONSULTA, ms_total: 95_000, ms_media: 2_400, ms_max: 8_000 },
      ],
      midiendo_desde: "",
    });

    render(envolver(<ConsultasLentasPage />));

    // 95.000 ms no se lee; «1,6 min» sí.
    expect(await screen.findByText("1.6 min")).toBeInTheDocument();
    expect(screen.getByText("2.4 s")).toBeInTheDocument();
    expect(screen.getByText("8.0 s")).toBeInTheDocument();
  });
});
