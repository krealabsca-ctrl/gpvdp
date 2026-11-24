import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

/**
 * El marcado del convenio en lote, mirado desde la pantalla.
 *
 * Que el servidor devuelva la lista de los que quedaron a medias NO prueba que alguien la lea.
 * Acá se prueba exactamente eso: que el aviso llegue a la pantalla, que el atajo cuente a los que
 * de verdad corresponde, y que la cifra de lo que se va a liberar se vea ANTES de la firma.
 *
 * Los hooks se reemplazan por respuestas fijas con la forma del contrato del backend: no se prueba
 * la red, se prueba lo que la pantalla dice.
 */

vi.mock("@/features/cxp/hooks", () => ({
  useCandidatosConvenio: vi.fn(),
  useMarcarConvenioMasivo: vi.fn(),
  useBloqueadasDeProveedores: vi.fn(),
  useLiberarDeProveedores: vi.fn(),
}));
vi.mock("@/features/auth/permisos", () => ({
  useTienePermiso: () => () => true,
}));

import {
  useBloqueadasDeProveedores,
  useCandidatosConvenio,
  useLiberarDeProveedores,
  useMarcarConvenioMasivo,
} from "@/features/cxp/hooks";
import { ToastProvider } from "@/components/ui";
import { ConvenioEnLotePanel } from "@/features/cxp/components/ConvenioEnLotePanel";
import type { CandidatoConvenio } from "@/api/cxp";

function candidato(p: Partial<CandidatoConvenio> & { proveedor_id: string; nombre: string }): CandidatoConvenio {
  return {
    condicion_pago: "CONTADO",
    convenio_manda: "XML",
    facturas_contado: 1,
    bloqueadas: 0,
    monto_bloqueado: "0",
    ya_listo: false,
    falta_credito: false,
    ...p,
  };
}

/** Deja los cuatro hooks listos y devuelve los espías de las dos mutaciones que escriben. */
function montar(opts: {
  candidatos: CandidatoConvenio[];
  sinCredito?: string[];
  bloqueadas?: { bloqueadas: number; monto_crc: string };
}) {
  // El doble responde como el servidor: con `pasarACredito` ya no queda nadie a medias, y sin él
  // vuelve el aviso de siempre. Si respondiera igual en los dos casos, las pruebas de abajo no
  // distinguirían haber mandado la decisión de no haberla mandado.
  const marcarMutate = vi.fn((_vars, o) =>
    o?.onSuccess?.({
      actualizados: _vars.ids.length,
      sin_credito: _vars.pasarACredito ? [] : (opts.sinCredito ?? []),
      pasados_a_credito: _vars.pasarACredito ? (opts.sinCredito ?? []) : [],
    }),
  );
  const contarMutate = vi.fn((_ids, o) =>
    o?.onSuccess?.(opts.bloqueadas ?? { bloqueadas: 0, monto_crc: "0" }),
  );
  const liberarMutate = vi.fn();

  vi.mocked(useCandidatosConvenio).mockReturnValue({
    data: opts.candidatos,
    isPending: false,
    isError: false,
    error: null,
  } as unknown as ReturnType<typeof useCandidatosConvenio>);
  vi.mocked(useMarcarConvenioMasivo).mockReturnValue({
    mutate: marcarMutate,
    isPending: false,
  } as unknown as ReturnType<typeof useMarcarConvenioMasivo>);
  vi.mocked(useBloqueadasDeProveedores).mockReturnValue({
    mutate: contarMutate,
    isPending: false,
  } as unknown as ReturnType<typeof useBloqueadasDeProveedores>);
  vi.mocked(useLiberarDeProveedores).mockReturnValue({
    mutate: liberarMutate,
    isPending: false,
  } as unknown as ReturnType<typeof useLiberarDeProveedores>);

  render(
    <ToastProvider>
      <ConvenioEnLotePanel />
    </ToastProvider>,
  );
  return { marcarMutate, contarMutate, liberarMutate };
}

describe("ConvenioEnLotePanel", () => {
  // El atajo del pedido: los que la ficha YA tiene en crédito y siguen declarando contado. Para
  // ellos marcar no mueve ningún plazo, solo levanta el candado. Si el atajo arrastra a los de
  // ficha en CONTADO, marca en masa proveedores que quedan a medias.
  it("el atajo cuenta solo a los que ya están en crédito sin el convenio puesto", async () => {
    montar({
      candidatos: [
        candidato({ proveedor_id: "1", nombre: "EnCredito", condicion_pago: "CREDITO" }),
        candidato({ proveedor_id: "2", nombre: "EnContado" }),
        candidato({
          proveedor_id: "3",
          nombre: "YaResuelto",
          condicion_pago: "CREDITO",
          convenio_manda: "PROVEEDOR",
          ya_listo: true,
        }),
      ],
    });
    await userEvent.click(screen.getByRole("button", { name: /ver a cuáles les pasa/i }));

    expect(screen.getByRole("button", { name: /elegir los 1 que ya están en crédito/i })).toBeTruthy();
  });

  // EL PEDIDO DE «IGUALALO»: el botón tiene que dejar el trabajo TERMINADO. Marcar solo el
  // convenio sobre una ficha en CONTADO no desactiva nada, y 573 de 649 están en CONTADO: el
  // camino dominante terminaba mandando a corregir a mano, ficha por ficha.
  it("por defecto completa la segunda condición y lo dice", async () => {
    const { marcarMutate } = montar({
      candidatos: [candidato({ proveedor_id: "2", nombre: "EnContado" })],
      sinCredito: ["EnContado"],
    });
    await userEvent.click(screen.getByRole("button", { name: /ver a cuáles les pasa/i }));
    await userEvent.click(screen.getByLabelText("Elegir EnContado"));

    // La casilla viene marcada y dice a cuántos les cambiaría la condición ANTES de apretar.
    const casilla = screen.getByRole("checkbox", { name: /pasar a «crédito» 1 de los elegidos/i });
    expect((casilla as HTMLInputElement).checked).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: /creerle a la ficha/i }));

    expect(marcarMutate.mock.calls[0]?.[0].pasarACredito).toBe(true);
    // Y el cambio de condición se anuncia: no es «a quién se le cree», es lo que el proveedor ES.
    expect(screen.getByText(/pasaron de «Contado» a «Crédito»/i)).toBeTruthy();
    expect(screen.queryByText(/quedaron a medias/i)).toBeNull();
  });

  // DESMARCAR TIENE QUE SIGNIFICAR ALGO. Hay proveedores que de verdad se pagan en el mostrador y
  // ahí el candado corresponde; para ésos vuelve el aviso de que quedaron a medias.
  it("si se desmarca, no cambia la condición y avisa que quedaron a medias", async () => {
    const { marcarMutate } = montar({
      candidatos: [candidato({ proveedor_id: "2", nombre: "EnContado" })],
      sinCredito: ["EnContado"],
    });
    await userEvent.click(screen.getByRole("button", { name: /ver a cuáles les pasa/i }));
    await userEvent.click(screen.getByLabelText("Elegir EnContado"));
    await userEvent.click(screen.getByRole("checkbox", { name: /pasar a «crédito» 1 de los elegidos/i }));
    await userEvent.click(screen.getByRole("button", { name: /creerle a la ficha/i }));

    expect(marcarMutate.mock.calls[0]?.[0].pasarACredito).toBe(false);
    expect(screen.getByText(/quedaron a medias/i).textContent).toContain("EnContado");
  });

  // La casilla no aparece cuando no le cambiaría la condición a nadie: ofrecer una decisión que no
  // hace nada es ruido, y encima sugiere que algo falta.
  it("no ofrece pasar a crédito si todos los elegidos ya lo están", async () => {
    montar({
      candidatos: [candidato({ proveedor_id: "1", nombre: "EnCredito", condicion_pago: "CREDITO" })],
    });
    await userEvent.click(screen.getByRole("button", { name: /ver a cuáles les pasa/i }));
    await userEvent.click(screen.getByLabelText("Elegir EnCredito"));

    expect(screen.queryByRole("checkbox", { name: /pasar a «crédito»/i })).toBeNull();
  });

  // Liberar es quitar la última barrera contra el doble pago: la cantidad y el monto se ven antes
  // de la firma, y la firma no se acepta en blanco.
  it("muestra cuántas y por cuánto antes de liberar, y exige el motivo", async () => {
    const { liberarMutate } = montar({
      candidatos: [candidato({ proveedor_id: "1", nombre: "EnCredito", condicion_pago: "CREDITO", bloqueadas: 2 })],
      bloqueadas: { bloqueadas: 2, monto_crc: "3500" },
    });
    await userEvent.click(screen.getByRole("button", { name: /ver a cuáles les pasa/i }));
    await userEvent.click(screen.getByLabelText("Elegir EnCredito"));
    await userEvent.click(screen.getByRole("button", { name: /creerle a la ficha/i }));

    // El separador de miles que produce Intl es un espacio DURO: se normaliza para que la
    // afirmación hable del número que se lee, no del code point que lo separa.
    const aviso = screen.getByText(/quedan 2 factura/i);
    expect(aviso.textContent?.replace(/ /g, " ")).toContain("₡3 500,00");

    const boton = screen.getByRole("button", { name: /liberar las 2/i });
    expect((boton as HTMLButtonElement).disabled).toBe(true);

    await userEvent.type(
      screen.getByLabelText(/por qué estas facturas sí se pagan por banco/i),
      "el convenio firmado es a 8 días",
    );
    expect((screen.getByRole("button", { name: /liberar las 2/i }) as HTMLButtonElement).disabled).toBe(false);
    await userEvent.click(screen.getByRole("button", { name: /liberar las 2/i }));
    expect(liberarMutate).toHaveBeenCalledTimes(1);
  });
});
