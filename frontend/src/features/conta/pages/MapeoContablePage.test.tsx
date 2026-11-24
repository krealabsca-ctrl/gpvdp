import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { ToastProvider } from "@/components/ui";

/**
 * El mapeo, mirado desde la pantalla.
 *
 * Que el servidor sepa guardar un lote no prueba que la pantalla mande el lote correcto. Acá se
 * verifica lo que solo existe en el cliente: que lo marcado y después escondido por un filtro NO
 * se decida a ciegas, que el aviso de cruce se vea, y que «NEUTRO sin declarar» no se muestre como
 * si el ERP hubiera opinado.
 */

vi.mock("@/features/conta/hooks", () => ({
  useMapeoContable: vi.fn(),
  useCuentasDeMovimiento: vi.fn(),
  useDecidirMapeo: vi.fn(),
  useDeshacerMapeo: vi.fn(),
}));
vi.mock("@/features/auth/permisos", () => ({ useTienePermiso: () => () => true }));

import {
  useCuentasDeMovimiento,
  useDecidirMapeo,
  useDeshacerMapeo,
  useMapeoContable,
} from "@/features/conta/hooks";
import { MapeoContablePage } from "@/features/conta/pages/MapeoContablePage";
import type { CoberturaMapeo, CuentaContable, DecisionDeMapeo, FilaDelMapeo } from "@/api/conta";

const decidir = vi.fn();

function fila(p: Partial<FilaDelMapeo> & { clasificacion_id: string; clasificacion: string }): FilaDelMapeo {
  return {
    concepto: "Gastos",
    naturaleza_concepto: "GASTO",
    naturaleza_declarada: true,
    movimientos: 4,
    estado: "",
    motivo: "",
    cuenta_id: null,
    cuenta_codigo: "",
    cuenta_nombre: "",
    cuenta_clase: "",
    cruzada: false,
    aviso_cruce: "",
    ...p,
  };
}

const cobertura: CoberturaMapeo = {
  con_movimientos: 4,
  mapeadas: 1,
  no_contabilizables: 0,
  sin_decidir: 3,
  cruzadas: 1,
  sin_naturaleza: 2,
  total: 9,
};

function montar(items: FilaDelMapeo[]) {
  vi.mocked(useMapeoContable).mockReturnValue({
    data: { items, cobertura },
    isPending: false,
    isError: false,
    error: null,
  } as unknown as ReturnType<typeof useMapeoContable>);
  vi.mocked(useCuentasDeMovimiento).mockReturnValue({
    data: [
      { id: "cta-caja", codigo: "1-01-01-001-000", nombre: "Caja Chica Administración", clase: "ACTIVO" },
      { id: "cta-gasto", codigo: "6-01-00-000-000", nombre: "Gastos generales", clase: "GASTO" },
    ] as CuentaContable[],
  } as unknown as ReturnType<typeof useCuentasDeMovimiento>);
  vi.mocked(useDecidirMapeo).mockReturnValue({
    mutate: decidir,
    isPending: false,
  } as unknown as ReturnType<typeof useDecidirMapeo>);
  vi.mocked(useDeshacerMapeo).mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
  } as unknown as ReturnType<typeof useDeshacerMapeo>);
  render(
    <ToastProvider>
      <MemoryRouter>
        <MapeoContablePage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("MapeoContablePage", () => {
  beforeEach(() => decidir.mockClear());

  // LA PRUEBA QUE IMPORTA. El trabajo viene en familias: se filtra «caja chica», se marcan todas y
  // se mandan de una vez. Si al cambiar el filtro siguieran contando las marcadas que ya no se
  // ven, se mapearía a ciegas partidas que la persona no está mirando.
  it("no decide sobre lo marcado que el filtro dejó de mostrar", async () => {
    montar([
      fila({ clasificacion_id: "c1", clasificacion: "Caja Chica - Administración" }),
      fila({ clasificacion_id: "c2", clasificacion: "Combustible" }),
    ]);

    await userEvent.click(screen.getByLabelText("Marcar Caja Chica - Administración"));
    await userEvent.click(screen.getByLabelText("Marcar Combustible"));
    expect(screen.getByText(/2 partida\(s\) marcada\(s\)/)).toBeTruthy();

    // Se busca otra cosa: Combustible desaparece de la vista.
    await userEvent.type(screen.getByLabelText(/buscar/i), "caja");
    expect(screen.getByText(/1 partida\(s\) marcada\(s\)/)).toBeTruthy();

    await userEvent.click(screen.getByPlaceholderText(/código o el nombre de la cuenta/i));
    await userEvent.click(screen.getByText(/Caja Chica Administración/));
    await userEvent.click(screen.getByRole("button", { name: /mapear las 1$/i }));

    expect(decidir).toHaveBeenCalledTimes(1);
    const enviado = decidir.mock.calls[0]?.[0] as DecisionDeMapeo[];
    expect(enviado).toHaveLength(1);
    expect(enviado[0]).toMatchObject({ clasificacion_id: "c1", estado: "MAPEADA", cuenta_id: "cta-caja" });
  });

  // El cruce de la caja chica, visible en la fila: el error no se nota hasta que el balance no
  // cuadra, meses después, y para entonces ya viajó a todos los asientos.
  it("muestra el aviso de naturaleza cruzada con su explicación", () => {
    montar([
      fila({
        clasificacion_id: "c1",
        clasificacion: "Caja Chica - Administración",
        estado: "MAPEADA",
        cuenta_id: "cta-caja",
        cuenta_codigo: "1-01-01-001-000",
        cuenta_nombre: "Caja Chica Administración",
        cuenta_clase: "ACTIVO",
        cruzada: true,
        aviso_cruce: "en el ERP es GASTO y la cuenta es de ACTIVO: revisá si es un gasto o si es mover plata",
      }),
    ]);
    expect(screen.getByText(/revisá si es un gasto o si es mover plata/)).toBeTruthy();
  });

  // «NEUTRO» es el default de la columna, no una decisión. Mostrarlo como si lo fuera haría que
  // alguien mapee creyendo que el ERP ya opinó sobre esa partida.
  it("distingue una naturaleza declarada de la que nadie declaró", () => {
    montar([
      fila({ clasificacion_id: "c1", clasificacion: "Papelería", naturaleza_concepto: "GASTO", naturaleza_declarada: true }),
      fila({ clasificacion_id: "c2", clasificacion: "Sin opinión", naturaleza_concepto: "NEUTRO", naturaleza_declarada: false }),
    ]);
    expect(screen.getByText("GASTO")).toBeTruthy();
    expect(screen.getByText("sin declarar")).toBeTruthy();
    expect(screen.queryByText("NEUTRO")).toBeNull();
  });

  // Declarar que algo no se contabiliza exige decir por qué: el botón no se habilita con un «ok».
  it("no deja declarar «no se contabiliza» sin una explicación de verdad", async () => {
    montar([fila({ clasificacion_id: "c1", clasificacion: "Partida de control" })]);
    await userEvent.click(screen.getByLabelText("Marcar Partida de control"));

    const boton = screen.getByRole("button", { name: /no se contabilizan/i });
    expect(boton.hasAttribute("disabled")).toBe(true);

    await userEvent.type(screen.getByLabelText(/no contabilizables/i), "ok");
    expect(boton.hasAttribute("disabled")).toBe(true);

    await userEvent.clear(screen.getByLabelText(/no contabilizables/i));
    await userEvent.type(screen.getByLabelText(/no contabilizables/i), "es un traslado entre cuentas propias");
    expect(boton.hasAttribute("disabled")).toBe(false);

    await userEvent.click(boton);
    const mandadas = decidir.mock.calls[0]?.[0] as DecisionDeMapeo[];
    expect(mandadas[0]).toMatchObject({
      clasificacion_id: "c1",
      estado: "NO_CONTABILIZABLE",
      motivo: "es un traslado entre cuentas propias",
    });
  });

  // El avance mide lo que BLOQUEA: 1 de 4 usadas, no 1 de 9 existentes.
  it("el avance se mide sobre las partidas que se usan", () => {
    montar([fila({ clasificacion_id: "c1", clasificacion: "Papelería" })]);
    expect(screen.getByText("25%")).toBeTruthy();
    expect(screen.getByText(/de las 4 partidas que se usan/)).toBeTruthy();
    expect(screen.getByText(/9 clasificaciones en total/)).toBeTruthy();
  });
});
