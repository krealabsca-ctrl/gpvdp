import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * El expediente del pago: los dos papeles de una factura (mig 0103 + 16-nov-2026).
 *
 * LO QUE SE PROTEGE ES QUE «NO ESTÁ EL ARCHIVO» NO SE LEA COMO «NO ESTÁ LA FACTURA». En las 4.542
 * facturas del sistema el chip decía «+» —«no hay, adjuntala»— aunque 4.526 tienen su clave de
 * Hacienda con proveedor, montos y retención. El Director Financiero lo leyó como que el sistema
 * había perdido la factura, y es exactamente lo que el chip comunicaba.
 *
 * Un expediente con los datos completos no puede verse igual que uno sin nada, y el clic tiene
 * que llevar a donde están esos datos en vez de pedir un archivo que nadie tiene.
 */

vi.mock("@/features/cxp/hooks", () => ({ useAdjuntarComprobante: vi.fn() }));
vi.mock("@/api/cxp", () => ({ cxpApi: { descargarComprobante: vi.fn() } }));

import { useAdjuntarComprobante } from "@/features/cxp/hooks";
import type { Documento } from "@/api/cxp";
import { ToastProvider } from "@/components/ui";
import { ExpedienteDelPago } from "@/features/cxp/components/ExpedienteDelPago";

function doc(over: Partial<Documento> = {}): Documento {
  return {
    id: "d1",
    clave: "50601011500310112345600100001010000000671199999999",
    consecutivo: "00100001010000000671",
    proveedor: "GOS Servicios Integrales",
    tiene_factura: false,
    tiene_comprobante: false,
    ...over,
  } as Documento;
}

function envolver(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <MemoryRouter initialEntries={["/cxp/bandeja"]}>
          <Routes>
            <Route path="/cxp/bandeja" element={children} />
            <Route path="/cxp/documentos/:id" element={<p>DETALLE DE LA FACTURA</p>} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>
  );
}

/** El chip de la factura, por su texto accesible. */
function chipFactura() {
  return screen.getAllByRole("button").find((b) => b.textContent?.includes("Factura"))!;
}

describe("ExpedienteDelPago", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useAdjuntarComprobante).mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    } as unknown as ReturnType<typeof useAdjuntarComprobante>);
  });

  it("con la clave y sin archivo NO dice «+»: los datos de la factura están", () => {
    render(envolver(<ExpedienteDelPago doc={doc()} puedeAdjuntar compacto />));
    const chip = chipFactura();
    expect(chip.textContent).toContain("·");
    expect(chip.textContent).not.toContain("+");
    expect(chip.getAttribute("title")).toMatch(/Ver los datos de la factura/);
  });

  it("y el clic lleva al detalle, donde están esos datos", async () => {
    render(envolver(<ExpedienteDelPago doc={doc()} puedeAdjuntar compacto />));
    await userEvent.click(chipFactura());
    expect(await screen.findByText("DETALLE DE LA FACTURA")).toBeInTheDocument();
  });

  it("sin clave Y sin archivo sí dice «+»: ahí no hay nada", () => {
    render(envolver(<ExpedienteDelPago doc={doc({ clave: "" })} puedeAdjuntar compacto />));
    const chip = chipFactura();
    expect(chip.textContent).toContain("+");
    expect(chip.textContent).not.toContain("·");
  });

  it("con el archivo adjunto dice «✓», que es otra cosa", () => {
    render(envolver(<ExpedienteDelPago doc={doc({ tiene_factura: true })} puedeAdjuntar compacto />));
    const chip = chipFactura();
    expect(chip.textContent).toContain("✓");
    expect(chip.getAttribute("title")).toMatch(/Abrir factura/i);
  });

  it("en el DETALLE el chip de datos no navega: ofrece adjuntar el archivo", () => {
    // `compacto` ausente = la tarjeta del detalle. Ir al detalle desde el detalle no lleva a
    // ningún lado, así que lo único que falta hacer ahí es subir el archivo.
    render(envolver(<ExpedienteDelPago doc={doc()} puedeAdjuntar />));
    expect(chipFactura().getAttribute("title")).toMatch(/falta adjuntar el archivo/);
  });

  it("el comprobante del banco no tiene «datos»: o está el documento o no hay nada", () => {
    render(envolver(<ExpedienteDelPago doc={doc()} puedeAdjuntar compacto />));
    const comprobante = screen.getAllByRole("button").find((b) => b.textContent?.includes("Comprobante"))!;
    expect(comprobante.textContent).toContain("+");
    expect(comprobante.textContent).not.toContain("·");
  });
});
