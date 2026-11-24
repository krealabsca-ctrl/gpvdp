import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";

/**
 * El canje de la referencia, mirado desde la pantalla.
 *
 * Que el servidor sepa encontrar un incidente no prueba que la pantalla lo pida bien. El caso real
 * es una llamada: alguien dicta «tres efe ocho a dos ce» y quien atiende lo teclea como venga. Si
 * la pantalla manda eso tal cual, el servidor responde vacío y el código —que existe— parece
 * inventado.
 */

vi.mock("@/features/config/hooks", () => ({
  useIncidentes: vi.fn(),
  useResumenIncidentes: vi.fn(),
}));

import { useIncidentes, useResumenIncidentes } from "@/features/config/hooks";
import { IncidentesPage } from "@/features/config/pages/IncidentesPage";
import type { Incidente } from "@/api/sistema";

function incidente(p: Partial<Incidente> & { id: string; referencia: string }): Incidente {
  return {
    empresa_id: "e1",
    usuario_id: "u1",
    usuario: "Tesorería",
    metodo: "PUT",
    ruta: "/v1/cxp/proveedores/:id",
    estado: 500,
    operacion: "cxp actualizar-proveedor",
    detalle: "expected 17 arguments, got 16",
    panico: false,
    traza: "",
    creado_en: "2026-10-22T15:04:00Z",
    ...p,
  };
}

function montar(items: Incidente[]) {
  vi.mocked(useIncidentes).mockReturnValue({
    data: { items },
    isPending: false,
    isError: false,
    error: null,
  } as unknown as ReturnType<typeof useIncidentes>);
  vi.mocked(useResumenIncidentes).mockReturnValue({
    data: { ultima_hora: 1, ultimo_dia: 1, ultima_semana: 1, panicos: 0, ultimo: "2026-10-22T15:04:00Z" },
  } as unknown as ReturnType<typeof useResumenIncidentes>);
  render(
    <MemoryRouter>
      <IncidentesPage />
    </MemoryRouter>,
  );
}

describe("IncidentesPage", () => {
  // LA PRUEBA QUE IMPORTA: lo que se teclea se normaliza antes de consultar.
  it("busca en mayúsculas aunque la referencia se teclee como venga", async () => {
    montar([incidente({ id: "1", referencia: "3F8A2C" })]);

    await userEvent.type(screen.getByLabelText(/referencia/i), "  3f8a2c ");
    await userEvent.click(screen.getByRole("button", { name: /^buscar$/i }));

    const llamadas = vi.mocked(useIncidentes).mock.calls;
    const ultima = llamadas[llamadas.length - 1]?.[0];
    expect(ultima?.ref).toBe("3F8A2C");
  });

  // El canje devuelve la causa, que es para lo que existe la pantalla.
  it("muestra la causa exacta y a quién le pasó", () => {
    montar([incidente({ id: "1", referencia: "3F8A2C" })]);

    expect(screen.getByText("3F8A2C")).toBeTruthy();
    expect(screen.getByText(/expected 17 arguments, got 16/)).toBeTruthy();
    expect(screen.getByText(/le pasó a Tesorería/)).toBeTruthy();
  });

  // Una caída del servidor y un «falló al guardar» son problemas de gravedad distinta, y la
  // pantalla tiene que distinguirlos sin que haya que abrir nada.
  it("distingue la caída del servidor de un error cualquiera", () => {
    montar([
      incidente({ id: "1", referencia: "AAA111" }),
      incidente({ id: "2", referencia: "BBB222", panico: true, traza: "goroutine 1 [running]:" }),
    ]);

    expect(screen.getByText(/se cayó el servidor/i)).toBeTruthy();
    expect(screen.getByText(/error 500/i)).toBeTruthy();
  });

  // Un error sin empresa ocurrió ANTES de entrar. Si no se dice, alguien lo busca en los datos de
  // una empresa donde el usuario nunca estuvo.
  it("avisa cuando el error ocurrió antes de elegir empresa", () => {
    montar([incidente({ id: "1", referencia: "CCC333", empresa_id: null, ruta: "/v1/auth/login" })]);
    expect(screen.getByText(/antes de elegir empresa/i)).toBeTruthy();
  });

  // Sin resultados, el texto tiene que distinguir «no hay errores» de «esa referencia no apareció»:
  // son dos conclusiones opuestas sobre el mismo vacío.
  it("el vacío de una búsqueda no dice lo mismo que el vacío sin buscar", async () => {
    montar([]);
    expect(screen.getByText(/no hay errores registrados/i)).toBeTruthy();

    await userEvent.type(screen.getByLabelText(/referencia/i), "999999");
    await userEvent.click(screen.getByRole("button", { name: /^buscar$/i }));
    expect(screen.getByText(/no hay ningún error con la referencia 999999/i)).toBeTruthy();
  });
});
