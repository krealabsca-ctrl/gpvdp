import { describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * Las claves y la caché de «Mi partida».
 *
 * La pantalla tiene UNA lista —los créditos de la partida—: la vista «sin_clasificar» se quitó el
 * 23-set-2026 y el cliente ni siquiera manda `vista`. Por eso conservar la respuesta anterior
 * mientras llega la nueva es seguro: siempre es la misma lista con otro recorte.
 */

vi.mock("@/api/bancos", () => ({
  bancosApi: { miSegmento: vi.fn(), misAvisos: vi.fn() },
}));
vi.mock("@/features/bancos/useEmpresaId", () => ({ useEmpresaId: () => "e1" }));

import { bancosApi, type FiltrosMovimientos, type MiSegmento } from "@/api/bancos";
import { queryKeys } from "@/api/queryKeys";
import { useMiSegmento, useMisAvisos } from "@/features/bancos/hooks";

function respuesta(total: number): MiSegmento {
  return {
    partidas: [],
    cuentas: [],
    movimientos: {
      totales: {
        total_debitos: "0",
        total_creditos: "0",
        diferencia: "0",
        sin_tipo_cambio: 0,
        monto_sin_convertir: "0",
        excluidos: 0,
      },
      items: [],
      total,
      page: 1,
      page_size: 100,
    },
    cargado_hasta: "",
    cargado_hasta_cuenta: null,
    carga_por_cuenta: [],
    sin_alcance: false,
  };
}

function envoltorio() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("useMiSegmento", () => {
  it("nunca pide otra vista que la de la partida", async () => {
    vi.mocked(bancosApi.miSegmento).mockResolvedValue(respuesta(2108));
    const { result } = renderHook(() => useMiSegmento({ page: 1, page_size: 200 }), {
      wrapper: envoltorio(),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    // Mandar `vista` sería pedirle al servidor algo que responde 400 desde que se quitó la pestaña.
    const llamadas = vi.mocked(bancosApi.miSegmento).mock.calls;
    expect(llamadas.length).toBeGreaterThan(0);
    for (const [pedido] of llamadas) {
      expect(pedido as Record<string, unknown>).not.toHaveProperty("vista");
    }
  });

  it("al paginar conserva la página anterior (sin parpadeo)", async () => {
    let soltarPagina2: (r: MiSegmento) => void = () => {};
    vi.mocked(bancosApi.miSegmento).mockImplementation((f: FiltrosMovimientos) =>
      f.page === 2
        ? new Promise<MiSegmento>((ok) => {
            soltarPagina2 = ok;
          })
        : Promise.resolve(respuesta(2108)),
    );
    const { result, rerender } = renderHook(
      ({ f }: { f: FiltrosMovimientos }) => useMiSegmento(f),
      { wrapper: envoltorio(), initialProps: { f: { page: 1 } } },
    );
    await waitFor(() => expect(result.current.data?.movimientos.total).toBe(2108));

    rerender({ f: { page: 2 } });
    expect(result.current.data?.movimientos.total).toBe(2108);
    expect(result.current.isPlaceholderData).toBe(true);
    soltarPagina2(respuesta(2108));
    await waitFor(() => expect(result.current.isPlaceholderData).toBe(false));
  });
});

describe("claves de «Mi partida»", () => {
  // TanStack Query calza por PREFIJO: todo tiene que colgar de miSegmentoRaiz (que ya lleva la
  // empresa), porque avisar invalida esa raíz y así se refrescan la lista y «Mis avisos».
  it("la lista, la página y «Mis avisos» cuelgan de la raíz de la pantalla", () => {
    const raiz = queryKeys.bancos.miSegmentoRaiz("e1");
    const lista = queryKeys.bancos.miSegmento("e1", { page: 3, page_size: 100 });
    const avisos = queryKeys.bancos.misAvisos("e1", 2, 50);
    expect(lista.slice(0, raiz.length)).toEqual([...raiz]);
    expect(avisos.slice(0, raiz.length)).toEqual([...raiz]);
    expect(avisos).toEqual(["bancos", "e1", "mi-segmento", "mis-avisos", 2, 50]);
    expect(lista[3]).toEqual({ page: 3, page_size: 100 });
  });

  // La regla del proyecto: la empresa va SEGUNDA, así la raíz del módulo de ESA empresa las calza.
  it("la empresa va segunda en las tres claves", () => {
    const modulo = queryKeys.bancos.raiz("e1");
    for (const clave of [
      queryKeys.bancos.miSegmentoRaiz("e1"),
      queryKeys.bancos.miSegmento("e1", { page: 1 }),
      queryKeys.bancos.misAvisos("e1", 1, 50),
    ]) {
      expect(clave[1]).toBe("e1");
      expect(clave.slice(0, modulo.length)).toEqual([...modulo]);
    }
    // Y otra empresa no calza: cambiar de empresa no reusa la caché de la anterior.
    expect(queryKeys.bancos.misAvisos("e2", 1, 50).slice(0, 2)).not.toEqual([...modulo]);
  });

  it("useMisAvisos pide la página y el tamaño que se le dan", async () => {
    vi.mocked(bancosApi.misAvisos).mockResolvedValue({
      items: [],
      total: 0,
      abiertos: 0,
      resueltos: 0,
      page: 2,
      page_size: 50,
      sin_alcance: false,
    });
    const { result } = renderHook(() => useMisAvisos(2, 50), { wrapper: envoltorio() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(bancosApi.misAvisos).toHaveBeenCalledWith(2, 50);
  });
});
