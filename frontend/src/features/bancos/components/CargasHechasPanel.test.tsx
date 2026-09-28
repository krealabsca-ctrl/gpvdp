import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

/**
 * «CARGAS HECHAS» dibujada con datos.
 *
 * Los hooks de datos se reemplazan por respuestas fijas con la forma EXACTA del contrato del
 * backend (los montos son decimal-strings SIN escala fija: ₡10.000.000 viaja como "10000000").
 * Acá no se prueba la red: se prueba que la pantalla diga lo que el Director pidió poder hacer
 * —revertir la carga del 21 sin malograr la del 20— y que no ofrezca lo que va a fallar.
 *
 * Las cuatro cosas que esta pantalla no puede equivocarse:
 *  · mostrar lo que manda el servidor, con la plata formateada;
 *  · no dibujar el botón si el rol no puede revertir;
 *  · si hay algo apoyado en esa carga, el botón va apagado CON la razón leíble (no fallando al
 *    apretarlo);
 *  · confirmar con los números exactos y no dejar mandar sin motivo.
 */

vi.mock("@/features/bancos/hooks", () => ({
  useImportaciones: vi.fn(),
  useRevertirImportacion: vi.fn(),
  useDeshacerReversaImportacion: vi.fn(),
}));
vi.mock("@/features/auth/permisos", () => ({ useTienePermiso: vi.fn() }));

import {
  useDeshacerReversaImportacion,
  useImportaciones,
  useRevertirImportacion,
} from "@/features/bancos/hooks";
import { useTienePermiso } from "@/features/auth/permisos";
import { ToastProvider } from "@/components/ui";
import { CargasHechasPanel } from "@/features/bancos/components/CargasHechasPanel";
import type {
  BloqueosReversa,
  CuentaBancaria,
  ImportacionItem,
  ListaImportaciones,
} from "@/api/bancos";

const SIN_BLOQUEOS: BloqueosReversa = {
  cobros_cxc: 0,
  planillas_cxc: 0,
  avisos_sin_resolver: 0,
  responsabilidades: 0,
  facturas_cxp: 0,
  traslados_emparejados: 0,
  periodos_cerrados: [],
  actas_firmadas: [],
};

function carga(p: Partial<ImportacionItem> & { id: string }): ImportacionItem {
  return {
    cuenta_bancaria_id: "cta-colinas",
    banco: "Promerica",
    cuenta_alias: "Promerica Colinas",
    moneda: "CRC",
    nombre_archivo: "VALLE DE PAZ PROMERICA COLONES.xlsx",
    estado: "CONFIRMADA",
    creado_por: "u-admin",
    creado_por_nombre: "Administrador GPVDP",
    creado_en: "2026-09-23T21:03:25Z",
    movimientos: 7,
    excluidos: 0,
    clasificados: 0,
    total_debitos: "10000000",
    total_creditos: "7042727.09",
    fecha_desde: "2026-09-02",
    fecha_hasta: "2026-09-14",
    revertida: false,
    revertida_en: "",
    revertida_por: "",
    revertida_por_nombre: "",
    motivo_reversa: "",
    bloqueos: SIN_BLOQUEOS,
    puede_revertir: true,
    puede_deshacer_reversa: false,
    razon_no_revertir: "",
    ...p,
  };
}

/** El incidente real: la carga del 21 (mal) y la del 20 (bien), en la MISMA cuenta. */
const LA_MALA = carga({ id: "imp-mala" });
const LA_BUENA = carga({
  id: "imp-buena",
  nombre_archivo: "COLINAS PROMERICA COLONES.xlsx",
  creado_en: "2026-09-20T15:10:00Z",
  movimientos: 5,
  clasificados: 2,
  total_debitos: "702466",
  total_creditos: "1051233",
  fecha_desde: "2026-07-01",
  fecha_hasta: "2026-07-31",
});

const CUENTAS: CuentaBancaria[] = [
  { id: "cta-colinas", alias: "Promerica Colinas", banco: "Promerica", iban: "CR1", moneda: "CRC", activo: true },
  { id: "cta-vdp", alias: "Promerica VDP", banco: "Promerica", iban: "CR2", moneda: "CRC", activo: true },
];

const revertir = vi.fn();
const deshacer = vi.fn();

function lista(items: ImportacionItem[], total = items.length): ListaImportaciones {
  return { items, total, page: 1, page_size: 50 };
}

function montar(opciones: { datos?: ListaImportaciones; permiso?: boolean } = {}) {
  vi.mocked(useTienePermiso).mockReturnValue(() => opciones.permiso ?? true);
  vi.mocked(useImportaciones).mockImplementation((() => ({
    data: opciones.datos ?? lista([LA_MALA, LA_BUENA]),
    isPending: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  })) as unknown as typeof useImportaciones);
  vi.mocked(useRevertirImportacion).mockImplementation((() => ({
    mutate: revertir,
    isPending: false,
  })) as unknown as typeof useRevertirImportacion);
  vi.mocked(useDeshacerReversaImportacion).mockImplementation((() => ({
    mutate: deshacer,
    isPending: false,
  })) as unknown as typeof useDeshacerReversaImportacion);
  return render(
    <ToastProvider>
      <CargasHechasPanel cuentas={CUENTAS} />
    </ToastProvider>,
  );
}

/** La fila de una carga, buscada por su archivo + rango (dos cargas comparten el archivo). */
function filaDe(texto: string): HTMLElement {
  return screen.getByText(texto).closest("tr") as HTMLElement;
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CargasHechasPanel", () => {
  it("muestra lo que manda el servidor, con la plata formateada", () => {
    montar();
    const mala = filaDe("VALLE DE PAZ PROMERICA COLONES.xlsx");
    const f = within(mala);
    expect(f.getByText("Promerica · Promerica Colinas")).toBeInTheDocument();
    expect(f.getByText("Administrador GPVDP")).toBeInTheDocument();
    expect(f.getByText("7")).toBeInTheDocument();
    expect(f.getByText("02/09/2026 – 14/09/2026")).toBeInTheDocument();
    // Los decimal-strings del backend ("10000000") se muestran como plata, no como texto crudo.
    expect(f.getByText(/₡10\s?000\s?000,00/)).toBeInTheDocument();
    expect(f.getByText(/₡7\s?042\s?727,09/)).toBeInTheDocument();
    expect(f.getByText("En los libros")).toBeInTheDocument();

    // Y la carga que está BIEN sigue ahí con sus propios números: revertir una no es tocar la otra.
    const buena = within(filaDe("COLINAS PROMERICA COLONES.xlsx"));
    expect(buena.getByText(/₡702\s?466,00/)).toBeInTheDocument();
    expect(buena.getByText("01/07/2026 – 31/07/2026")).toBeInTheDocument();
  });

  it("el paginador dice el total REAL de la empresa, no el de la página", () => {
    montar({ datos: lista([LA_MALA, LA_BUENA], 239) });
    expect(screen.getByText(/Mostrando 1–2 de 239 cargas/)).toBeInTheDocument();
  });

  it("filtrar por cuenta vuelve a pedir la lista de esa cuenta, desde la página 1", async () => {
    const user = userEvent.setup();
    montar();
    await user.selectOptions(screen.getByLabelText("Ver las cargas de"), "cta-vdp");
    const llamadas = vi.mocked(useImportaciones).mock.calls;
    expect(llamadas[llamadas.length - 1]?.[0]).toEqual({
      cuenta_bancaria_id: "cta-vdp",
      page: 1,
      page_size: 50,
    });
  });

  it("sin el permiso no se ve el botón, pero el historial sí", () => {
    montar({ permiso: false });
    expect(screen.getByText("VALLE DE PAZ PROMERICA COLONES.xlsx")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Revertir esta carga" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Deshacer la reversa" })).toBeNull();
  });

  it("con bloqueos el botón está apagado y la razón se lee en la fila", () => {
    const bloqueada = carga({
      id: "imp-bloqueada",
      puede_revertir: false,
      razon_no_revertir:
        "3 cobro(s) de CxC identificados contra estos movimientos; el período 2026-09 ya está cerrado",
      bloqueos: { ...SIN_BLOQUEOS, cobros_cxc: 3, periodos_cerrados: ["2026-09"] },
    });
    montar({ datos: lista([bloqueada]) });

    const boton = screen.getByRole("button", { name: "Revertir esta carga" });
    expect(boton).toBeDisabled();
    // La razón la redacta el servidor y se muestra TAL CUAL: un botón apagado sin explicación es
    // una pared (¿me falta permiso?, ¿está roto?, ¿qué destrabo?).
    expect(
      screen.getByText(
        "3 cobro(s) de CxC identificados contra estos movimientos; el período 2026-09 ya está cerrado",
      ),
    ).toBeInTheDocument();
  });

  it("la confirmación dice los números exactos, promete que lo otro no se toca y exige motivo", async () => {
    const user = userEvent.setup();
    montar();
    await user.click(screen.getAllByRole("button", { name: "Revertir esta carga" })[0]!);

    expect(screen.getByRole("heading", { name: "Revertir esta carga" })).toBeInTheDocument();
    expect(
      screen.getByText(
        /Vas a excluir 7 movimientos por ₡10\s?000\s?000,00 en débitos y ₡7\s?042\s?727,09 en créditos, del 02\/09\/2026 al 14\/09\/2026 en Promerica · Promerica Colinas/,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("Las otras cargas de esa cuenta no se tocan.")).toBeInTheDocument();
    // Y que NO se borra: la reversa es «dejan de sumar», no un DELETE.
    expect(screen.getByText(/Los movimientos no se borran/)).toBeInTheDocument();

    const enviar = screen.getByRole("button", { name: "Revertir la carga" });
    expect(enviar).toBeDisabled(); // sin motivo no se manda: el 400 no debería llegar a pasar nunca
    await user.type(
      screen.getByLabelText("¿Por qué se revierte? *"),
      "el archivo era de Promerica VDP y se importó en Promerica Colinas",
    );
    expect(enviar).toBeEnabled();
    await user.click(enviar);
    expect(revertir).toHaveBeenCalledWith(
      {
        importacionId: "imp-mala",
        motivo: "el archivo era de Promerica VDP y se importó en Promerica Colinas",
      },
      expect.anything(),
    );
  });

  it("una carga revertida muestra quién, cuándo y por qué, y ofrece deshacer", async () => {
    const user = userEvent.setup();
    const revertida = carga({
      id: "imp-revertida",
      estado: "REVERTIDA",
      excluidos: 7,
      revertida: true,
      revertida_en: "2026-09-24T14:05:00Z",
      revertida_por: "u-admin",
      revertida_por_nombre: "Administrador GPVDP",
      motivo_reversa: "se importó en la cuenta equivocada",
      puede_revertir: false,
      puede_deshacer_reversa: true,
      razon_no_revertir: "esta carga ya está revertida",
    });
    montar({ datos: lista([revertida]) });

    // No se esconde: quien corrigió tiene que poder verificar qué marcó.
    const fila = within(filaDe("VALLE DE PAZ PROMERICA COLONES.xlsx"));
    expect(fila.getByText("Revertida")).toBeInTheDocument();
    expect(fila.getByText("7 no suman")).toBeInTheDocument();
    expect(fila.getByText(/por Administrador GPVDP/)).toBeInTheDocument();
    expect(fila.getByText(/«se importó en la cuenta equivocada»/)).toBeInTheDocument();
    expect(fila.getByText(/24\/09\/2026/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Revertir esta carga" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Deshacer la reversa" }));
    const dialogo = within(screen.getByRole("dialog"));
    expect(dialogo.getByRole("heading", { name: "Deshacer la reversa" })).toBeInTheDocument();
    expect(
      dialogo.getByText(/Vuelven a los libros los movimientos que esta reversa excluyó/),
    ).toBeInTheDocument();
    // El motivo acá es opcional (así lo definió el backend): se manda vacío sin trabar nada.
    await user.click(dialogo.getByRole("button", { name: "Deshacer la reversa" }));
    expect(deshacer).toHaveBeenCalledWith(
      { importacionId: "imp-revertida", motivo: "" },
      expect.anything(),
    );
  });

  it("si el mes se cerró después de la reversa, deshacer está apagado con su razón", () => {
    const revertidaYCerrada = carga({
      id: "imp-trabada",
      estado: "REVERTIDA",
      excluidos: 7,
      revertida: true,
      revertida_en: "2026-09-24T14:05:00Z",
      revertida_por_nombre: "Administrador GPVDP",
      motivo_reversa: "se importó en la cuenta equivocada",
      bloqueos: { ...SIN_BLOQUEOS, periodos_cerrados: ["2026-09"] },
      puede_revertir: false,
      puede_deshacer_reversa: false,
      razon_no_revertir: "no se puede deshacer la reversa porque hay el período 2026-09 ya está cerrado",
    });
    montar({ datos: lista([revertidaYCerrada]) });

    expect(screen.getByRole("button", { name: "Deshacer la reversa" })).toBeDisabled();
    expect(
      screen.getByText(/no se puede deshacer la reversa porque hay el período 2026-09 ya está cerrado/),
    ).toBeInTheDocument();
  });

  it("sin cargas lo dice, no queda en blanco", () => {
    montar({ datos: lista([], 0) });
    expect(
      screen.getByText("Todavía no se subió ningún archivo de banco en esta empresa."),
    ).toBeInTheDocument();
  });
});
