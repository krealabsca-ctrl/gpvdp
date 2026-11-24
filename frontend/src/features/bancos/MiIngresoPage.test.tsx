import { describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";

/**
 * «Mi ingreso» (mig 0125).
 *
 * LO QUE ESTA PANTALLA SE COPIÓ DE «MI PRESUPUESTO» Y NO PUEDE HEREDAR: el signo. En el gasto,
 * pasarse es malo; acá superar la meta es el objetivo. Un copiar/pegar que arrastre la clase roja
 * pinta un logro como un problema y nadie lo nota, porque la cifra está bien.
 *
 * Y lo que sí tiene que heredar: nombrar SIEMPRE las partidas a cargo, aunque no hayan cobrado.
 */

vi.mock("@/api/bancos", () => ({
  bancosApi: { miIngreso: vi.fn(), miSerieDeIngreso: vi.fn() },
}));
vi.mock("@/features/bancos/useEmpresaId", () => ({ useEmpresaId: () => "e1" }));

import { bancosApi, type MiIngreso } from "@/api/bancos";
import { MiIngresoPage } from "@/features/bancos/pages/MiIngresoPage";

function envoltorio({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  // El router hace falta por el botón «?» del encabezado, que busca el artículo de ayuda de la
  // ruta en la que está.
  return (
    <MemoryRouter initialEntries={["/mi-ingreso"]}>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </MemoryRouter>
  );
}

function respuesta(p: Partial<MiIngreso> = {}): MiIngreso {
  return {
    desde: "2026-10",
    hasta: "2026-10",
    partidas: [
      { clasificacion_id: "c-dat", clasificacion: "Datafonos", concepto: "Ingresos" },
      { clasificacion_id: "c-cja", clasificacion: "Caja de Ingresos", concepto: "Ingresos" },
    ],
    lineas: [
      {
        clasificacion_id: "c-dat",
        clasificacion: "Datafonos",
        concepto: "Ingresos",
        meta: "80000000.00",
        real: "95855286.35",
        movs: 1397,
        diferencia: "15855286.35",
        cumplido_pct: "119.8",
        estado: "CUMPLIDO",
      },
    ],
    meta_total: "80000000.00",
    real_total: "95855286.35",
    diferencia: "15855286.35",
    sin_meta: 0,
    ...p,
  };
}

describe("MiIngresoPage", () => {
  // SUPERAR LA META VA EN VERDE. Al revés del gasto, de donde salió esta pantalla.
  it("pinta a favor la diferencia positiva", async () => {
    vi.mocked(bancosApi.miIngreso).mockResolvedValue(respuesta());
    render(<MiIngresoPage />, { wrapper: envoltorio });

    const celda = await screen.findByText(/^\+/);
    expect(celda.className).toContain("text-positivo");
    expect(celda.className).not.toContain("text-negativo");
  });

  // Y QUEDARSE CORTO VA EN ROJO.
  it("pinta en contra la diferencia negativa", async () => {
    vi.mocked(bancosApi.miIngreso).mockResolvedValue(
      respuesta({
        lineas: [
          {
            clasificacion_id: "c-dat",
            clasificacion: "Datafonos",
            concepto: "Ingresos",
            meta: "80000000.00",
            real: "60000000.00",
            movs: 900,
            diferencia: "-20000000.00",
            cumplido_pct: "75.0",
            estado: "CORTO",
          },
        ],
      }),
    );
    render(<MiIngresoPage />, { wrapper: envoltorio });

    // El separador de miles de es-CR es un espacio duro, no un punto: el regex no lo asume.
    const celda = await screen.findByText(/^-\D*20\D000\D000/);
    expect(celda.className).toContain("text-negativo");
    expect(celda.className).not.toContain("text-positivo");
  });

  // LAS PARTIDAS A CARGO SE NOMBRAN AUNQUE NO HAYAN COBRADO.
  // «Caja de Ingresos» no tiene línea en la respuesta y aun así tiene que aparecer: si no, su
  // jefe no puede distinguir «no me la asignaron» de «no entró nada».
  it("nombra las partidas que no registraron ingreso", async () => {
    vi.mocked(bancosApi.miIngreso).mockResolvedValue(respuesta());
    render(<MiIngresoPage />, { wrapper: envoltorio });

    await screen.findByText("Caja de Ingresos");
  });

  // SIN META NO HAY SEMÁFORO, y la pantalla lo dice en vez de mostrar un 0 % falso.
  it("no inventa diferencia cuando la partida no tiene meta", async () => {
    vi.mocked(bancosApi.miIngreso).mockResolvedValue(
      respuesta({
        lineas: [
          {
            clasificacion_id: "c-dat",
            clasificacion: "Datafonos",
            concepto: "Ingresos",
            meta: "",
            real: "95855286.35",
            movs: 1397,
            diferencia: "",
            cumplido_pct: "",
            estado: "SIN_META",
          },
        ],
        meta_total: "0.00",
        sin_meta: 1,
        aviso: "Ninguna de tus partidas tiene meta declarada en este rango.",
      }),
    );
    render(<MiIngresoPage />, { wrapper: envoltorio });

    await screen.findByText("sin meta");
    // Ni un 0 % ni un «+0,00»: la ausencia de meta se muestra como ausencia.
    expect(screen.queryByText(/%/)).toBeNull();
    expect(screen.queryByText(/^\+/)).toBeNull();
  });

  // EL AVISO DEL SERVIDOR SE MUESTRA TAL CUAL: es lo que explica por qué no hay semáforo.
  it("muestra el aviso", async () => {
    vi.mocked(bancosApi.miIngreso).mockResolvedValue(
      respuesta({ aviso: "Ninguna de tus partidas tiene meta declarada en este rango." }),
    );
    render(<MiIngresoPage />, { wrapper: envoltorio });

    await screen.findByText(/Ninguna de tus partidas tiene meta/);
  });

  // NO MUESTRA MOVIMIENTOS BANCARIOS: la persona está justamente afuera de ese módulo.
  //
  // Lo que se comprueba es que la pantalla solo tiene a mano los DOS agregados —el control y su
  // serie— y ninguna puerta al listado de movimientos, que es el módulo del que está afuera.
  it("pide el agregado, no la lista de movimientos", async () => {
    vi.mocked(bancosApi.miIngreso).mockResolvedValue(respuesta());
    render(<MiIngresoPage />, { wrapper: envoltorio });

    await waitFor(() => expect(bancosApi.miIngreso).toHaveBeenCalled());
    expect(Object.keys(bancosApi).sort()).toEqual(["miIngreso", "miSerieDeIngreso"]);
  });

  // EL MES A MES NO SE PIDE HASTA QUE SE ABRE UNA FILA.
  //
  // La serie reusa el cálculo de Análisis, que trae las 131 partidas de la empresa para recortar a
  // dos. Pedirla al entrar haría lenta la pantalla que más se mira, para un dato que casi nadie
  // abre en la primera visita.
  it("no pide el mes a mes al entrar", async () => {
    vi.mocked(bancosApi.miIngreso).mockResolvedValue(respuesta());
    render(<MiIngresoPage />, { wrapper: envoltorio });

    await screen.findByRole("button", { name: /Datafonos/ });
    expect(bancosApi.miSerieDeIngreso).not.toHaveBeenCalled();
  });

  // Y SÍ SE PIDE al abrir la partida.
  it("pide el mes a mes al abrir una partida", async () => {
    vi.mocked(bancosApi.miIngreso).mockResolvedValue(respuesta());
    vi.mocked(bancosApi.miSerieDeIngreso).mockResolvedValue({
      desde: "2026-08",
      hasta: "2026-10",
      partidas: [
        {
          concepto_id: "co-1",
          concepto: "Ingresos",
          clasificacion_id: "c-dat",
          clasificacion: "Datafonos",
          naturaleza: "INGRESO",
          naturaleza_declarada: true,
          serie: [
            { periodo: "2026-08", monto: "30000000.00", movs: 400 },
            { periodo: "2026-09", monto: "40000000.00", movs: 500 },
            { periodo: "2026-10", monto: "25855286.35", movs: 497 },
          ],
          total: "95855286.35",
          promedio: "31951762.12",
          ultimo: "25855286.35",
          desvio_pct: "-19.1",
          meses_con_dato: 2,
          confiable: true,
        },
      ],
    } as never);
    render(<MiIngresoPage />, { wrapper: envoltorio });

    const boton = await screen.findByRole("button", { name: /Datafonos/ });
    await userEvent.click(boton);

    await waitFor(() => expect(bancosApi.miSerieDeIngreso).toHaveBeenCalled());
    await screen.findByText("Ingreso mes a mes");
  });
});
