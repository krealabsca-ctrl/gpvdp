import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * «Enlazarlos yo»: enlazar dos movimientos a mano, sin detector (12-nov-2026).
 *
 * LO QUE SE PROTEGE ES LA DIRECCIÓN DEL ENLACE, por la misma razón que en el panel hermano: el
 * servidor recibe dos ids —el que se queda y el que sale— y si la pantalla los manda al revés saca
 * de los libros exactamente el movimiento que el usuario quería conservar. Acá el riesgo es MAYOR,
 * porque no hay una huella que respalde el par: lo único que hay es lo que la persona marcó.
 *
 * Y que el impedimento se diga ANTES de pedir el motivo. El servidor rechaza igual un par de
 * cuentas o montos distintos, pero hacerle escribir una justificación para después negarle la
 * acción le hace perder el trabajo — y peor, enseña a ignorar el mensaje de error.
 */

vi.mock("@/api/bancos", () => ({
  bancosApi: {
    movimientos: vi.fn(),
    cuentas: vi.fn(),
    enlazarReemision: vi.fn(),
  },
}));
vi.mock("@/features/bancos/useEmpresaId", () => ({ useEmpresaId: () => "e1" }));

import { bancosApi } from "@/api/bancos";
import { ToastProvider } from "@/components/ui";
import { EnlaceManualPanel } from "@/features/bancos/components/EnlaceManualPanel";

const CUENTAS = [
  { id: "cta-1", alias: "BAC Valle de Paz Colones", banco: "BAC", iban: "CR1", moneda: "CRC" as const, activo: true, es_reserva: false },
  { id: "cta-2", alias: "BCR Valle de Paz", banco: "BCR", iban: "CR2", moneda: "CRC" as const, activo: true, es_reserva: false },
];

/** Un movimiento mínimo con lo que la pantalla usa. */
function mov(over: Record<string, unknown>) {
  return {
    id: "x",
    fecha: "2026-07-28",
    documento: "72700248",
    descripcion: "BAC FLOTA DE R:033855",
    consecutivo_largo: "",
    banco: "BAC",
    cuenta: "BAC Valle de Paz Colones",
    debito: "27001.00",
    credito: "0",
    moneda: "CRC",
    monto_crc: "27001.00",
    concepto_id: null,
    concepto: "",
    clasificacion_id: null,
    clasificacion: "",
    estado_clasificacion: "NO_IDENTIFICADO",
    confianza: null,
    es_traslado: false,
    ...over,
  };
}

// El caso real del usuario: el banco reemite con OTRA fecha y OTRO consecutivo, así que ningún
// detector los junta.
const VIEJO = mov({ id: "viejo-28jul", fecha: "2026-07-28", documento: "72700248" });
const NUEVO = mov({
  id: "nuevo-05ago",
  fecha: "2026-08-05",
  documento: "99900111",
  descripcion: "BAC FLOTA DE R:034610",
});

function lista(items: unknown[]) {
  return {
    totales: {},
    items,
    total: items.length,
    page: 1,
    page_size: 200,
  } as never;
}

function envolver(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={qc}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}

/**
 * Llena los filtros y aprieta Buscar.
 *
 * Espera la OPCIÓN de la cuenta antes de elegirla: el selector se llena con una consulta aparte y
 * sin esperarla la prueba corre contra un `<select>` que todavía dice «Cargando…».
 */
async function llenarFiltros() {
  await screen.findByRole("option", { name: /BAC Valle de Paz Colones/ });
  await userEvent.selectOptions(screen.getByLabelText("Cuenta"), "cta-1");
  await userEvent.type(screen.getByLabelText("Desde"), "2026-07-01");
  await userEvent.type(screen.getByLabelText("Hasta"), "2026-08-31");
  await userEvent.click(screen.getByRole("button", { name: "Buscar" }));
}

/** Llena los filtros, busca y devuelve cuando ya hay resultados en pantalla. */
async function buscar() {
  await llenarFiltros();
  await screen.findByText(/BAC FLOTA DE R:034610/);
}

describe("EnlaceManualPanel", () => {
  beforeEach(() => {
    // Los mocks acumulan sus llamadas entre pruebas: sin esto, `mock.calls[0]` de la segunda
    // prueba sigue siendo la llamada de la primera.
    vi.clearAllMocks();
    vi.mocked(bancosApi.cuentas).mockResolvedValue(CUENTAS as never);
    vi.mocked(bancosApi.enlazarReemision).mockResolvedValue({ ok: true } as never);
  });

  it("manda primero el que SE QUEDA y después el que SALE", async () => {
    vi.mocked(bancosApi.movimientos).mockResolvedValue(lista([VIEJO, NUEVO]));
    render(envolver(<EnlaceManualPanel />));
    await buscar();

    // Se queda el nuevo, sale el viejo.
    await userEvent.click(
      screen.getByLabelText("Dejar en los libros el movimiento del 2026-08-05 por 27001.00"),
    );
    await userEvent.click(
      screen.getByLabelText("Sacar de los libros el movimiento del 2026-07-28 por 27001.00"),
    );
    await userEvent.type(
      screen.getByLabelText("¿Por qué son el mismo movimiento?"),
      "el banco lo reemitió con otro consecutivo",
    );
    await userEvent.click(screen.getByRole("button", { name: "Es el mismo movimiento" }));

    await waitFor(() => expect(bancosApi.enlazarReemision).toHaveBeenCalled());
    const llamada = vi.mocked(bancosApi.enlazarReemision).mock.calls[0];
    expect(llamada?.[0]).toBe("nuevo-05ago"); // el que se queda
    expect(llamada?.[1]).toBe("viejo-28jul"); // el que sale
  });

  it("invertida la elección, invierte los dos ids", async () => {
    vi.mocked(bancosApi.movimientos).mockResolvedValue(lista([VIEJO, NUEVO]));
    render(envolver(<EnlaceManualPanel />));
    await buscar();

    // Ahora al revés: se queda el VIEJO (el que ya estaba clasificado) y sale el nuevo.
    await userEvent.click(
      screen.getByLabelText("Dejar en los libros el movimiento del 2026-07-28 por 27001.00"),
    );
    await userEvent.click(
      screen.getByLabelText("Sacar de los libros el movimiento del 2026-08-05 por 27001.00"),
    );
    await userEvent.type(
      screen.getByLabelText("¿Por qué son el mismo movimiento?"),
      "el nuevo entró sin la referencia completa",
    );
    await userEvent.click(screen.getByRole("button", { name: "Es el mismo movimiento" }));

    await waitFor(() => expect(bancosApi.enlazarReemision).toHaveBeenCalled());
    const llamada = vi.mocked(bancosApi.enlazarReemision).mock.calls[0];
    expect(llamada?.[0]).toBe("viejo-28jul");
    expect(llamada?.[1]).toBe("nuevo-05ago");
  });

  it("con montos distintos lo dice y ni siquiera pide el motivo", async () => {
    const otroMonto = mov({ id: "otro", fecha: "2026-08-05", debito: "15000.00", monto_crc: "15000.00" });
    vi.mocked(bancosApi.movimientos).mockResolvedValue(lista([VIEJO, otroMonto]));
    render(envolver(<EnlaceManualPanel />));
    await llenarFiltros();
    await screen.findByLabelText("Dejar en los libros el movimiento del 2026-07-28 por 27001.00");

    await userEvent.click(
      screen.getByLabelText("Dejar en los libros el movimiento del 2026-07-28 por 27001.00"),
    );
    await userEvent.click(
      screen.getByLabelText("Sacar de los libros el movimiento del 2026-08-05 por 15000.00"),
    );

    expect(await screen.findByText(/Los montos no son iguales/)).toBeInTheDocument();
    // El motivo no se pide: el par no se puede enlazar, así que escribirlo sería trabajo perdido.
    expect(screen.queryByLabelText("¿Por qué son el mismo movimiento?")).not.toBeInTheDocument();
  });

  it("con un débito y un crédito lo dice: el mismo hecho no cambia de lado", async () => {
    const credito = mov({ id: "cred", fecha: "2026-08-05", debito: "0", credito: "27001.00" });
    vi.mocked(bancosApi.movimientos).mockResolvedValue(lista([VIEJO, credito]));
    render(envolver(<EnlaceManualPanel />));
    await llenarFiltros();
    await screen.findByLabelText("Dejar en los libros el movimiento del 2026-07-28 por 27001.00");

    await userEvent.click(
      screen.getByLabelText("Dejar en los libros el movimiento del 2026-07-28 por 27001.00"),
    );
    await userEvent.click(
      screen.getByLabelText("Sacar de los libros el movimiento del 2026-08-05 por 27001.00"),
    );

    expect(await screen.findByText(/débito y el otro crédito/)).toBeInTheDocument();
    expect(screen.queryByLabelText("¿Por qué son el mismo movimiento?")).not.toBeInTheDocument();
  });

  it("no deja enlazar un movimiento consigo mismo", async () => {
    vi.mocked(bancosApi.movimientos).mockResolvedValue(lista([VIEJO, NUEVO]));
    render(envolver(<EnlaceManualPanel />));
    await buscar();

    await userEvent.click(
      screen.getByLabelText("Dejar en los libros el movimiento del 2026-07-28 por 27001.00"),
    );
    await userEvent.click(
      screen.getByLabelText("Sacar de los libros el movimiento del 2026-07-28 por 27001.00"),
    );

    expect(await screen.findByText(/dos movimientos distintos/)).toBeInTheDocument();
    expect(bancosApi.enlazarReemision).not.toHaveBeenCalled();
  });

  it("un motivo corto no alcanza: quien revise el mes tiene que entenderlo", async () => {
    vi.mocked(bancosApi.movimientos).mockResolvedValue(lista([VIEJO, NUEVO]));
    render(envolver(<EnlaceManualPanel />));
    await buscar();

    await userEvent.click(
      screen.getByLabelText("Dejar en los libros el movimiento del 2026-08-05 por 27001.00"),
    );
    await userEvent.click(
      screen.getByLabelText("Sacar de los libros el movimiento del 2026-07-28 por 27001.00"),
    );
    await userEvent.type(screen.getByLabelText("¿Por qué son el mismo movimiento?"), "igual");
    await userEvent.click(screen.getByRole("button", { name: "Es el mismo movimiento" }));

    expect(bancosApi.enlazarReemision).not.toHaveBeenCalled();
  });
});
