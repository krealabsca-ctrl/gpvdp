import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * El cronograma de pago: el cuadro que se manda ANTES de pagar (mig 0122).
 *
 * LO QUE SE PROTEGE ES QUE LA CASCADA SE VEA. La lista suma una cosa y el cronograma comunica otra:
 * en el archivo real, de ₡52,1 M que salen del banco solo ₡10,9 M son el cronograma. Una pantalla
 * que muestre la lista con un total al pie da una cifra cinco veces equivocada, perfectamente bien
 * formada, que nadie puede detectar leyendo.
 *
 * Y que un cuadro INCOMPLETO no se lea como definitivo: con dólares sin colonizar la cascada no se
 * puede sumar, y con líneas sin cuenta el total todavía puede moverse.
 */

vi.mock("@/api/cxp", () => ({
  cxpApi: {
    cronograma: vi.fn(),
    guardarTCCronograma: vi.fn(),
    asignarCuentaDePago: vi.fn(),
    marcarPorFuera: vi.fn(),
    descargarCronogramaXLSX: vi.fn(),
  },
}));
vi.mock("@/api/bancos", () => ({ bancosApi: { cuentas: vi.fn() } }));
vi.mock("@/features/bancos/useEmpresaId", () => ({ useEmpresaId: () => "e1" }));
vi.mock("@/features/cxp/useEmpresaId", () => ({ useEmpresaId: () => "e1" }));

import { cxpApi } from "@/api/cxp";
import { bancosApi } from "@/api/bancos";
import { ToastProvider } from "@/components/ui";
import { CronogramaPanel } from "@/features/cxp/components/CronogramaPanel";

// Cuatro líneas: dos que entran, una de reserva y una por fuera. La lista suma ₡888.494,09 y el
// cronograma comunica ₡288.494,09 — la diferencia es exactamente el punto.
const CRONOGRAMA = {
  lote_id: "l1",
  lote_numero: 43,
  fecha_corte: "2026-11-04",
  sin_cuenta: 0,
  lineas: [
    {
      proveedor_id: "p1",
      proveedor: "Gas Tomza De Costa Rica SA",
      moneda: "CRC",
      monto: "216174.09",
      monto_colonizado: "216174.09",
      cuenta: "BP Valle de Paz Colones",
      banco: "Banco Popular",
      es_reserva: false,
      por_fuera: false,
    },
    {
      proveedor_id: "p2",
      proveedor: "Daniel Esteban Chaves Arguedas",
      moneda: "CRC",
      monto: "72320.00",
      monto_colonizado: "72320.00",
      cuenta: "BAC Valle de Paz Colones",
      banco: "BAC",
      es_reserva: false,
      por_fuera: false,
    },
    {
      proveedor_id: "p3",
      proveedor: "Fondo De Mantenimiento",
      moneda: "CRC",
      monto: "100000.00",
      monto_colonizado: "100000.00",
      cuenta: "BP Negocios",
      banco: "Banco Popular",
      es_reserva: true,
      por_fuera: false,
    },
    {
      proveedor_id: "p4",
      proveedor: "Municipalidad De Cartago",
      moneda: "CRC",
      monto: "500000.00",
      monto_colonizado: "500000.00",
      cuenta: "BCR Valle de Paz",
      banco: "BCR",
      es_reserva: false,
      por_fuera: true,
    },
  ],
  totales: {
    total_colones: "888494.09",
    total_dolares: "0.00",
    dolares_colonizados: "0.00",
    total_colonizado: "888494.09",
    reserva: "100000.00",
    flujo_normal: "788494.09",
    por_fuera: "500000.00",
    cronograma_total: "288494.09",
    cronograma_en_colones: "288494.09",
    tipo_cambio: "0.0000",
  },
};

function envolver(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={qc}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}

describe("CronogramaPanel", () => {
  beforeEach(() => {
    // Los mocks acumulan sus llamadas entre pruebas: sin esto, `mock.calls[0]` de la segunda
    // prueba sigue siendo la llamada de la primera.
    vi.clearAllMocks();
    vi.mocked(bancosApi.cuentas).mockResolvedValue([]);
  });

  it("muestra la cascada completa, no solo la lista", async () => {
    vi.mocked(cxpApi.cronograma).mockResolvedValue(CRONOGRAMA);
    render(envolver(<CronogramaPanel loteId="l1" />));

    // El número que se comunica tiene que estar en la pantalla y ser el del cronograma, NO la suma
    // de la lista: son ₡288.494,09 contra ₡888.494,09.
    await screen.findByText("CRONOGRAMA TOTAL FLUJO NORMAL");
    const total = screen.getByText("CRONOGRAMA TOTAL FLUJO NORMAL").parentElement;
    // El separador de miles depende del entorno (punto, espacio fino…): se quitan TODOS para que
    // la prueba mida el número y no el formato del Intl de turno.
    const cifra = (total?.textContent ?? "").replace(/[\s.  ]/g, "");
    expect(cifra).toContain("288494,09");
    expect(cifra).not.toContain("888494,09");

    // Y los escalones intermedios, o nadie puede explicar de dónde sale la diferencia. Se buscan
    // DENTRO del resumen: «Reserva» también es la etiqueta de la fila, y afuera serían dos.
    const cascada = within(screen.getByText("Resumen del pago programado").parentElement!);
    expect(cascada.getByText("Total colonizado")).toBeInTheDocument();
    expect(cascada.getByText("Reserva")).toBeInTheDocument();
    expect(cascada.getByText("Total flujo normal")).toBeInTheDocument();
    expect(cascada.getByText("Solicitudes por fuera")).toBeInTheDocument();
  });

  it("con dólares sin colonizar pide el tipo de cambio antes de mostrar el cuadro", async () => {
    vi.mocked(cxpApi.cronograma).mockResolvedValue({
      ...CRONOGRAMA,
      lineas: [],
      totales: { ...CRONOGRAMA.totales, total_dolares: "730.00", tipo_cambio: "0.0000" },
    });
    render(envolver(<CronogramaPanel loteId="l1" />));

    const campo = await screen.findByLabelText(/pagos en dólares/i);
    await userEvent.type(campo, "450");
    await userEvent.click(screen.getByRole("button", { name: "Guardar" }));

    await waitFor(() => {
      expect(vi.mocked(cxpApi.guardarTCCronograma).mock.calls[0]).toEqual(["l1", "450"]);
    });
  });

  it("avisa cuando hay líneas sin cuenta: el total todavía puede cambiar", async () => {
    vi.mocked(cxpApi.cronograma).mockResolvedValue({ ...CRONOGRAMA, sin_cuenta: 3 });
    render(envolver(<CronogramaPanel loteId="l1" />));
    expect(await screen.findByText(/3 línea\(s\) todavía sin cuenta/i)).toBeInTheDocument();
  });

  it("sacar un proveedor del cronograma manda por_fuera = true", async () => {
    vi.mocked(cxpApi.cronograma).mockResolvedValue(CRONOGRAMA);
    render(envolver(<CronogramaPanel loteId="l1" />));

    // La casilla está marcada cuando el proveedor ENTRA; desmarcarla es sacarlo.
    const casilla = await screen.findByLabelText("Gas Tomza De Costa Rica SA entra en el cronograma");
    await userEvent.click(casilla);

    await waitFor(() => {
      expect(vi.mocked(cxpApi.marcarPorFuera).mock.calls[0]).toEqual(["l1", "p1", true]);
    });
  });
});
