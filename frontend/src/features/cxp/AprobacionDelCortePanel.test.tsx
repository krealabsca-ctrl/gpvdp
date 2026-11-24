import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * La aprobación del corte, en pantalla (mig 0123).
 *
 * LO QUE SE PROTEGE ES LA SEGREGACIÓN DE FUNCIONES. El sentido de este paso es que quien arma el
 * corte no sea quien lo aprueba; si la pantalla le mostrara «Aprobar» a tesorería, el control
 * desaparece sin que nadie lo note —el servidor igual rechazaría, pero recién después del clic, y
 * una pantalla que ofrece lo que no se puede hacer enseña a ignorar los errores—.
 *
 * Y que el estado decida qué se ofrece: aprobar un corte que nadie mandó a aprobar dejaría una
 * firma sobre algo que la gerencia nunca vio.
 */

vi.mock("@/features/cxp/hooks", () => ({
  useMandarLoteAAprobar: vi.fn(),
  useAprobarLote: vi.fn(),
  useDevolverLote: vi.fn(),
  useExcluidasDelLote: vi.fn(),
}));
vi.mock("@/features/auth/permisos", () => ({ useTienePermiso: vi.fn() }));

import {
  useAprobarLote,
  useDevolverLote,
  useExcluidasDelLote,
  useMandarLoteAAprobar,
} from "@/features/cxp/hooks";
import { useTienePermiso } from "@/features/auth/permisos";
import type { LotePago } from "@/api/cxp";
import { ToastProvider } from "@/components/ui";
import { AprobacionDelCortePanel } from "@/features/cxp/components/AprobacionDelCortePanel";

const mandar = vi.fn();
const aprobar = vi.fn();
const devolver = vi.fn();

function lote(over: Partial<LotePago> = {}): LotePago {
  return {
    id: "l1",
    numero: 41,
    fecha_corte: "2026-11-13",
    estado: "ABIERTO",
    cantidad: 12,
    total_crc: "4500000.00",
    creado_en: "2026-11-13T09:00:00-06:00",
    pagadas: 0,
    rebotadas: 0,
    pendientes: 12,
    modo_pago: "PROVEEDOR",
    excluidas: 0,
    ...over,
  } as LotePago;
}

/** Quién es quien mira la pantalla. */
function comoUsuarioCon(...permisos: string[]) {
  vi.mocked(useTienePermiso).mockReturnValue((p: string) => permisos.includes(p));
}

function envolver(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={qc}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}

describe("AprobacionDelCortePanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useMandarLoteAAprobar).mockReturnValue({
      mutate: mandar,
      isPending: false,
    } as unknown as ReturnType<typeof useMandarLoteAAprobar>);
    vi.mocked(useAprobarLote).mockReturnValue({
      mutate: aprobar,
      isPending: false,
    } as unknown as ReturnType<typeof useAprobarLote>);
    vi.mocked(useDevolverLote).mockReturnValue({
      mutate: devolver,
      isPending: false,
    } as unknown as ReturnType<typeof useDevolverLote>);
    vi.mocked(useExcluidasDelLote).mockReturnValue({
      data: { items: [] },
      isError: false,
    } as unknown as ReturnType<typeof useExcluidasDelLote>);
  });

  it("a quien arma el corte NO le ofrece aprobarlo", async () => {
    comoUsuarioCon("cxp.tesoreria");
    render(envolver(<AprobacionDelCortePanel lote={lote({ estado: "EN_APROBACION" })} />));

    // Esto es la segregación de funciones: tesorería manda a aprobar, no aprueba.
    expect(screen.queryByRole("button", { name: "Aprobar el corte" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Devolver" })).not.toBeInTheDocument();
  });

  it("a la gerencia le ofrece aprobar y devolver, pero no mandar a aprobar", async () => {
    comoUsuarioCon("cxp.aprobar_lote");
    render(envolver(<AprobacionDelCortePanel lote={lote({ estado: "EN_APROBACION" })} />));

    expect(screen.getByRole("button", { name: "Aprobar el corte" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Devolver" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Mandar a aprobar" })).not.toBeInTheDocument();
  });

  it("cada acción solo aparece en su estado", async () => {
    comoUsuarioCon("cxp.tesoreria", "cxp.aprobar_lote");

    const { unmount } = render(envolver(<AprobacionDelCortePanel lote={lote({ estado: "ABIERTO" })} />));
    expect(screen.getByRole("button", { name: "Mandar a aprobar" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Aprobar el corte" })).not.toBeInTheDocument();
    unmount();

    render(envolver(<AprobacionDelCortePanel lote={lote({ estado: "APROBADO" })} />));
    // Ya aprobado: no hay nada más que hacer acá.
    expect(screen.queryByRole("button", { name: "Mandar a aprobar" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Aprobar el corte" })).not.toBeInTheDocument();
  });

  it("dice que la macro está trancada mientras no se apruebe", async () => {
    comoUsuarioCon("cxp.tesoreria");
    const { unmount } = render(envolver(<AprobacionDelCortePanel lote={lote({ estado: "ABIERTO" })} />));
    expect(screen.getByText(/no se puede bajar hasta que la gerencia apruebe/)).toBeInTheDocument();
    unmount();

    // Aprobado: el aviso desaparece, porque ya se puede.
    render(envolver(<AprobacionDelCortePanel lote={lote({ estado: "APROBADO" })} />));
    expect(screen.queryByText(/no se puede bajar hasta/)).not.toBeInTheDocument();
  });

  it("devolver exige un motivo con sustancia", async () => {
    comoUsuarioCon("cxp.aprobar_lote");
    render(envolver(<AprobacionDelCortePanel lote={lote({ estado: "EN_APROBACION" })} />));

    await userEvent.click(screen.getByRole("button", { name: "Devolver" }));
    const campo = screen.getByLabelText(/Qué hay que corregir/);
    await userEvent.type(campo, "no sirve");
    await userEvent.click(screen.getByRole("button", { name: "Devolver el corte" }));
    expect(devolver).not.toHaveBeenCalled();

    await userEvent.clear(campo);
    await userEvent.type(campo, "la fecha de corte es del viernes pasado, rehacelo");
    await userEvent.click(screen.getByRole("button", { name: "Devolver el corte" }));
    await waitFor(() => expect(devolver).toHaveBeenCalled());
    expect(vi.mocked(devolver).mock.calls[0]?.[0]).toMatchObject({ loteId: "l1" });
  });

  it("muestra quién hizo cada paso y qué se sacó del corte", async () => {
    comoUsuarioCon("cxp.aprobar_lote");
    vi.mocked(useExcluidasDelLote).mockReturnValue({
      data: {
        items: [
          {
            documento_id: "d1",
            consecutivo: "00100001",
            proveedor: "GOS Servicios Integrales",
            monto_crc: "1068029.67",
            motivo: "el proveedor no entregó la nota de crédito acordada",
            excluido_por: "Gerencia Financiera",
            excluido_en: "2026-11-13 10:22",
            lote_numero: 41,
          },
        ],
      },
      isError: false,
    } as unknown as ReturnType<typeof useExcluidasDelLote>);

    render(
      envolver(
        <AprobacionDelCortePanel
          lote={lote({
            estado: "APROBADO",
            enviado_aprobacion_por: "Tesorería",
            enviado_aprobacion_en: "2026-11-13T09:30:00-06:00",
            aprobado_por: "Gerencia Financiera",
            aprobado_en: "2026-11-13T10:40:00-06:00",
            excluidas: 1,
          })}
        />,
      ),
    );

    expect(screen.getByText("Tesorería")).toBeInTheDocument();
    // «Gerencia Financiera» sale dos veces (aprobó y excluyó): lo que importa es que esté.
    expect(screen.getAllByText("Gerencia Financiera").length).toBeGreaterThan(0);
    expect(screen.getByText(/1 factura\(s\) salieron de este corte/)).toBeInTheDocument();
    expect(screen.getByText(/no entregó la nota de crédito/)).toBeInTheDocument();
    // Y dice que NO fueron denegadas: es la diferencia que evita el reproceso.
    expect(screen.getByText(/entran al próximo sin volver a tramitarse/)).toBeInTheDocument();
  });
});
