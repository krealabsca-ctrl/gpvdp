import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * El calendario de pagos, en pantalla (16-nov-2026).
 *
 * LO QUE SE PROTEGE ES QUE «VENCIDO» Y «SE PUEDE PAGAR» NO SE LEAN COMO LO MISMO. Con los datos
 * reales de Valle de Paz son ₡998 M contra ₡3,9 M: si la pantalla mostrara un solo número, diría
 * que hay casi mil millones por pagar cuando nadie puede pagarlos, y escondería que lo accionable
 * es mover 4.301 facturas por el flujo.
 *
 * Y que cada grupo diga la ACCIÓN. «4.301 en RECIBIDO» no le dice a nadie qué hacer.
 */

vi.mock("@/features/cxp/hooks", () => ({ useCalendarioDePagos: vi.fn() }));

import { useCalendarioDePagos } from "@/features/cxp/hooks";
import type { CalendarioPagos } from "@/api/cxp";
import { CalendarioDePagos } from "@/features/cxp/components/CalendarioDePagos";

/** Las cifras reales medidas el 16-nov-2026. */
const CAL: CalendarioPagos = {
  hoy: "2026-11-16",
  programado: [
    { fecha: "2026-11-13", dia: "viernes", cuando: "atrasado", facturas: 6, total_crc: "412189.00", atrasado: true },
    { fecha: "2026-11-20", dia: "viernes", cuando: "esta semana", facturas: 4, total_crc: "100000.00", atrasado: false },
    { fecha: "2026-11-27", dia: "viernes", cuando: "la próxima semana", facturas: 2, total_crc: "50000.00", atrasado: false },
  ],
  vencido: [
    { orden: 1, situacion: "listo para pagar", que_falta: "solo falta incluirlas en un corte", estados: ["APROBADO", "PROGRAMADO"], facturas: 41, total_crc: "3857299.00" },
    { orden: 2, situacion: "validado por el área", que_falta: "falta aprobarlas", estados: ["VALIDADO_DEPTO"], facturas: 3, total_crc: "2597335.00" },
    { orden: 3, situacion: "revisado", que_falta: "falta la validación del área", estados: ["REVISADO"], facturas: 118, total_crc: "7893301.00" },
    { orden: 4, situacion: "en recepción", que_falta: "falta revisarlas", estados: ["RECIBIDO"], facturas: 4301, total_crc: "984212798.00" },
  ],
  total_programado: "562189.00",
  total_vencido: "998560733.00",
  listo_para_pagar: "3857299.00",
};

function envolver(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

/** Quita separadores de miles para comparar el número y no el formato del Intl de turno. */
function cifras(texto: string) {
  return texto.replace(/[\s.  ]/g, "");
}

describe("CalendarioDePagos", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useCalendarioDePagos).mockReturnValue({
      data: CAL,
      isPending: false,
      isError: false,
    } as unknown as ReturnType<typeof useCalendarioDePagos>);
  });

  it("separa lo que se puede pagar de todo lo vencido", () => {
    render(envolver(<CalendarioDePagos />));
    // El número destacado es lo PAGABLE (₡3,9 M), no el total (₡998 M). Se mira el PÁRRAFO del
    // aviso y no su contenedor: la sección entera sí trae el total, y subir un nivel haría que la
    // prueba pase aunque el aviso mostrara la cifra equivocada.
    const aviso = screen.getByText(/está listo para pagarse/);
    expect(cifras(aviso.textContent ?? "")).toContain("3857299,00");
    expect(cifras(aviso.textContent ?? "")).not.toContain("998560733,00");
  });

  it("muestra los dos totales por separado, no uno solo", () => {
    render(envolver(<CalendarioDePagos />));
    const cuerpo = cifras(document.body.textContent ?? "");
    expect(cuerpo).toContain("562189,00"); // con fecha de pago
    expect(cuerpo).toContain("998560733,00"); // vencido en total
    expect(cuerpo).toContain("3857299,00"); // de eso, lo pagable
  });

  it("cada grupo dice la acción pendiente, no el estado", () => {
    render(envolver(<CalendarioDePagos />));
    expect(screen.getByText("falta revisarlas")).toBeInTheDocument();
    expect(screen.getByText("falta aprobarlas")).toBeInTheDocument();
    expect(screen.getByText("falta la validación del área")).toBeInTheDocument();
    // Y NO muestra los códigos internos del flujo.
    expect(screen.queryByText("RECIBIDO")).not.toBeInTheDocument();
    expect(screen.queryByText("VALIDADO_DEPTO")).not.toBeInTheDocument();
  });

  it("ubica cada fecha de pago: atrasado, esta semana, la próxima", () => {
    render(envolver(<CalendarioDePagos />));
    expect(screen.getByText("atrasado")).toBeInTheDocument();
    expect(screen.getByText("esta semana")).toBeInTheDocument();
    expect(screen.getByText("la próxima semana")).toBeInTheDocument();
    // Los viernes se nombran: es el día en que se paga.
    expect(screen.getAllByText(/viernes/).length).toBe(3);
  });

  it("sin fechas programadas lo dice, en vez de mostrar un bloque vacío", () => {
    vi.mocked(useCalendarioDePagos).mockReturnValue({
      data: { ...CAL, programado: [], total_programado: "0.00" },
      isPending: false,
      isError: false,
    } as unknown as ReturnType<typeof useCalendarioDePagos>);
    render(envolver(<CalendarioDePagos />));
    expect(screen.getByText(/Todavía no hay facturas con fecha de pago/)).toBeInTheDocument();
  });
});
