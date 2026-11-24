import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * «El banco lo mandó dos veces»: la pantalla que enlaza una reemisión (mig 0118).
 *
 * Lo que se protege acá es LA DIRECCIÓN DEL ENLACE. El servidor recibe dos ids —el que se queda en
 * los libros y el que sale— y si la pantalla los manda al revés, saca de los libros exactamente el
 * movimiento que el usuario quería conservar. Las dos filas se ven casi idénticas (mismo monto,
 * mismo consecutivo), así que nadie lo notaría mirando: el error se descubriría al cuadrar el mes.
 *
 * Por eso la prueba comprueba el caso por defecto Y el invertido.
 */

vi.mock("@/api/bancos", () => ({
  bancosApi: {
    candidatosReemision: vi.fn(),
    reemisiones: vi.fn(),
    enlazarReemision: vi.fn(),
    deshacerReemision: vi.fn(),
  },
}));
vi.mock("@/features/bancos/useEmpresaId", () => ({ useEmpresaId: () => "e1" }));

import { bancosApi, type CandidatoReemision } from "@/api/bancos";
import { ToastProvider } from "@/components/ui";
import { ReemisionesPanel } from "@/features/bancos/components/ReemisionesPanel";

// El caso real: ₡880.000 el 4 de noviembre, ya clasificado, y el mismo consecutivo el 5 sin
// clasificar.
const candidato: CandidatoReemision = {
  monto: "880000.00",
  es_debito: false,
  cuenta: "BAC Religiosa",
  banco: "BAC",
  dias_entre: 1,
  anterior: {
    id: "viejo-4nov",
    fecha: "2026-11-04",
    documento: "406802824",
    descripcion: "TEF A : 903825354",
    clasificacion: "Ingresos › Servicios de Emergencias",
    estado_clasificacion: "REVISADO",
    incluido: true,
  },
  posterior: {
    id: "nuevo-5nov",
    fecha: "2026-11-05",
    documento: "406802824",
    descripcion: "TEF DE:HILARY AILEEN PINTO CUB",
    clasificacion: "",
    estado_clasificacion: "NO_IDENTIFICADO",
    incluido: true,
  },
};

// El panel avisa con toasts al enlazar y al deshacer, así que necesita su proveedor igual que en
// la aplicación real: sin él, `useToast` lanza y la prueba fallaría por el envoltorio, no por el
// componente.
function envolver(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={qc}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}

describe("ReemisionesPanel", () => {
  beforeEach(() => {
    // Los mocks acumulan sus llamadas entre pruebas: sin esto, `mock.calls[0]` de la segunda
    // prueba sigue siendo la llamada de la primera, y la aserción mide la prueba anterior.
    vi.clearAllMocks();
    vi.mocked(bancosApi.candidatosReemision).mockResolvedValue({
      items: [candidato],
      total: 1,
      dias_max: 7,
    });
    vi.mocked(bancosApi.reemisiones).mockResolvedValue({ items: [], total: 0 });
    vi.mocked(bancosApi.enlazarReemision).mockResolvedValue({ ok: true });
  });

  it("enlaza dejando el MÁS RECIENTE en los libros por defecto", async () => {
    const user = userEvent.setup();
    render(envolver(<ReemisionesPanel />));

    await screen.findByText("TEF DE:HILARY AILEEN PINTO CUB");
    await user.type(
      screen.getByLabelText("¿Por qué son el mismo movimiento?"),
      "El BAC reemitió el mismo crédito al día siguiente",
    );
    await user.click(screen.getByRole("button", { name: "Es el mismo movimiento" }));

    await waitFor(() => expect(bancosApi.enlazarReemision).toHaveBeenCalled());
    // Primer argumento: el que SE QUEDA. Segundo: el que SALE.
    expect(vi.mocked(bancosApi.enlazarReemision).mock.calls[0]?.[0]).toBe("nuevo-5nov");
    expect(vi.mocked(bancosApi.enlazarReemision).mock.calls[0]?.[1]).toBe("viejo-4nov");
  });

  it("invierte el enlace si se elige dejar el anterior", async () => {
    const user = userEvent.setup();
    render(envolver(<ReemisionesPanel />));

    await screen.findByText("TEF DE:HILARY AILEEN PINTO CUB");
    await user.type(
      screen.getByLabelText("¿Por qué son el mismo movimiento?"),
      "Me quedo con el primero, el nuevo entró sin referencia",
    );
    // Elegir explícitamente el del 4: es el caso en que el reemitido trae menos información.
    // La selección va DESPUÉS del motivo a propósito: es el orden en que trabaja una persona —lee
    // las dos filas, escribe por qué, y recién entonces decide cuál deja—, y es el orden en que un
    // re-render intermedio podría perder la elección sin que se note.
    const radioViejo = screen.getByRole("radio", {
      name: "Dejar en los libros el movimiento del 2026-11-04",
    });
    await user.click(radioViejo);
    expect(radioViejo).toBeChecked();

    await user.click(screen.getByRole("button", { name: "Es el mismo movimiento" }));

    await waitFor(() => expect(bancosApi.enlazarReemision).toHaveBeenCalled());
    expect(vi.mocked(bancosApi.enlazarReemision).mock.calls[0]?.[0]).toBe("viejo-4nov");
    expect(vi.mocked(bancosApi.enlazarReemision).mock.calls[0]?.[1]).toBe("nuevo-5nov");
  });

  it("no deja enlazar sin un motivo con sustancia", async () => {
    const user = userEvent.setup();
    render(envolver(<ReemisionesPanel />));

    await screen.findByText("TEF DE:HILARY AILEEN PINTO CUB");
    const boton = screen.getByRole("button", { name: "Es el mismo movimiento" });
    // Sin motivo: deshabilitado. El servidor también lo rechaza, pero descubrirlo después de
    // apretar es peor que no poder apretar.
    expect(boton).toBeDisabled();

    await user.type(screen.getByLabelText("¿Por qué son el mismo movimiento?"), "corto");
    expect(boton).toBeDisabled();

    await user.type(
      screen.getByLabelText("¿Por qué son el mismo movimiento?"),
      " pero ahora ya es suficientemente largo",
    );
    expect(boton).toBeEnabled();
  });

  it("avisa del riesgo de los pagos recurrentes cuando hay candidatos", async () => {
    render(envolver(<ReemisionesPanel />));
    // El aviso tiene que estar ARRIBA y siempre: el riesgo real no es dejar de enlazar algo, es
    // enlazar una cuota mensual y hacer desaparecer un pago real de los libros.
    expect(
      await screen.findByText(/varios bancos reusan el consecutivo/i),
    ).toBeInTheDocument();
  });

  it("dice que no hay nada cuando la lista viene vacía", async () => {
    vi.mocked(bancosApi.candidatosReemision).mockResolvedValue({
      items: [],
      total: 0,
      dias_max: 7,
    });
    render(envolver(<ReemisionesPanel />));
    expect(
      await screen.findByText(/no hay movimientos que parezcan reemitidos/i),
    ).toBeInTheDocument();
  });
});
