import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * «Llenarlo en Excel»: el panel que saca y vuelve a entrar el mapeo (12-nov-2026).
 *
 * LO QUE SE PROTEGE SON LOS RECHAZOS. Una hoja de 300 filas con doce errores de tipeo es el caso
 * NORMAL, no la excepción. Si la pantalla dijera «12 no se pudieron aplicar» sin decir cuáles,
 * habría que revisar la hoja entera a ojo para encontrarlas —y el número de fila del Excel es lo
 * único que la persona puede ir a buscar directamente—.
 *
 * Y que un archivo donde NO entró nada no se lea como éxito: `aplicadas: 0` con rechazos tiene que
 * decirlo, porque el camino de salida (corregir y resubir) es distinto del de «ya está todo».
 */

vi.mock("@/features/conta/hooks", () => ({ useImportarMapeoXLSX: vi.fn() }));
vi.mock("@/api/conta", () => ({ contaApi: { exportarMapeoXLSX: vi.fn() } }));
vi.mock("@/lib/csv", () => ({ descargarBlob: vi.fn() }));

import { useImportarMapeoXLSX } from "@/features/conta/hooks";
import { contaApi, type ResultadoImportacionMapeo } from "@/api/conta";
import { descargarBlob } from "@/lib/csv";
import { ToastProvider } from "@/components/ui";
import { MapeoExcelPanel } from "@/features/conta/components/MapeoExcelPanel";

const mutate = vi.fn();

function envolver(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={qc}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}

/** Dispara la subida y entrega el resultado que el servidor habría devuelto. */
async function subirYResponder(res: ResultadoImportacionMapeo) {
  mutate.mockImplementation((_archivo: File, opts: { onSuccess: (r: unknown) => void }) => {
    opts.onSuccess(res);
  });
  render(envolver(<MapeoExcelPanel />));
  const input = screen.getByLabelText("Archivo del mapeo");
  await userEvent.upload(
    input,
    new File(["x"], "mapeo-contable.xlsx", {
      type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    }),
  );
}

describe("MapeoExcelPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useImportarMapeoXLSX).mockReturnValue({
      mutate,
      isPending: false,
    } as unknown as ReturnType<typeof useImportarMapeoXLSX>);
  });

  it("muestra cada rechazo con el número de fila del Excel", async () => {
    await subirYResponder({
      aplicadas: 2,
      rechazos: [
        {
          fila: 4,
          concepto: "Gastos",
          clasificacion: "Partida Que No Existe",
          motivo: "no existe una partida con ese concepto y ese nombre en esta empresa",
        },
        {
          fila: 17,
          concepto: "Gastos",
          clasificacion: "Combustible",
          motivo: 'la cuenta "9-99-99-999-999" no está en el plan de esta empresa',
        },
      ],
      resumen: { filas_leidas: 4, mapeadas: 1, no_contables: 1 },
    });

    expect(await screen.findByText("4")).toBeInTheDocument();
    expect(screen.getByText("17")).toBeInTheDocument();
    expect(screen.getByText(/Partida Que No Existe/)).toBeInTheDocument();
    expect(screen.getByText(/9-99-99-999-999/)).toBeInTheDocument();
    // Y dice que el resto SÍ entró: resubir la hoja corregida no empieza de cero.
    expect(screen.getByText(/2 aplicada/)).toBeInTheDocument();
    expect(screen.getByText(/no hace falta empezar de nuevo/)).toBeInTheDocument();
  });

  it("un archivo donde no entró nada no se lee como éxito", async () => {
    await subirYResponder({
      aplicadas: 0,
      rechazos: [
        { fila: 2, concepto: "Gastos", clasificacion: "Combustible", motivo: "la cuenta no existe" },
      ],
      resumen: { filas_leidas: 1, mapeadas: 0, no_contables: 0 },
    });
    expect(await screen.findByText(/0 aplicada/)).toBeInTheDocument();
    expect(screen.getByText(/1 fila\(s\) no entraron/)).toBeInTheDocument();
  });

  it("sin rechazos no muestra la tabla de errores", async () => {
    await subirYResponder({
      aplicadas: 12,
      rechazos: [],
      resumen: { filas_leidas: 12, mapeadas: 10, no_contables: 2 },
    });
    expect(await screen.findByText(/12 aplicada/)).toBeInTheDocument();
    expect(screen.queryByText(/no entraron/)).not.toBeInTheDocument();
    expect(screen.getByText(/10 con cuenta/)).toBeInTheDocument();
    expect(screen.getByText(/2 marcadas/)).toBeInTheDocument();
  });

  it("bajar la hoja pide el archivo y lo descarga", async () => {
    vi.mocked(contaApi.exportarMapeoXLSX).mockResolvedValue(new Blob(["x"]));
    render(envolver(<MapeoExcelPanel />));
    await userEvent.click(screen.getByRole("button", { name: /Bajar la hoja/ }));
    expect(vi.mocked(contaApi.exportarMapeoXLSX)).toHaveBeenCalled();
    expect(vi.mocked(descargarBlob).mock.calls[0]?.[0]).toBe("mapeo-contable.xlsx");
  });
});
