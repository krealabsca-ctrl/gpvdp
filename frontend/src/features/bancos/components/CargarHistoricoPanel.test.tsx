import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

/**
 * «Cargar histórico» dibujado con datos.
 *
 * Lo que estas pruebas cuidan es lo que el Director tiene que poder decidir ANTES de escribir nada:
 * cuánta plata entra por cuenta (que no es lo que trae el archivo si ya subió parte), qué le falta
 * crear, y que el adorno del reporte no se lea como si algo estuviera roto.
 */

vi.mock("@/features/bancos/hooks", () => ({
  useSubirHistorico: vi.fn(),
  useConfirmarHistorico: vi.fn(),
}));

import { ToastProvider } from "@/components/ui";
import { CargarHistoricoPanel } from "@/features/bancos/components/CargarHistoricoPanel";
import { useConfirmarHistorico, useSubirHistorico } from "@/features/bancos/hooks";
import type { PlanHistorico } from "@/api/bancos";

const PLAN: PlanHistorico = {
  carga_id: "carga-1",
  nombre_archivo: "bancos-2025.xlsx",
  hoja: "Movimientos",
  hojas: ["Movimientos", "Resumen por partida"],
  totales: {
    filas: 1200,
    lineas_de_formato: 6,
    nuevas: 1150,
    ya_existen: 40,
    sin_partida: 12,
    partida_desconocida: 3,
    sin_cuenta: 8,
    errores: 2,
    total_debitos: "5000000.00",
    total_creditos: "7500000.50",
    fecha_desde: "2025-01-03",
    fecha_hasta: "2025-12-29",
  },
  cuentas: [
    {
      cuenta_bancaria_id: "c1",
      cuenta: "Davivienda Colones",
      banco: "Davivienda",
      moneda: "CRC",
      nombres_en_archivo: ["Davivienda Colones"],
      filas: 900,
      fecha_desde: "2025-01-03",
      fecha_hasta: "2025-12-29",
      meses: ["2025-01", "2025-02"],
      total_debitos: "4000000.00",
      total_creditos: "6000000.00",
      debitos_nuevos: "3900000.00",
      creditos_nuevos: "5900000.00",
      nuevas: 880,
      ya_existen: 20,
      sin_partida: 10,
      partida_desconocida: 3,
      sin_tipo_cambio: 0,
      importacion_id: "",
      insertados: 0,
    },
    {
      cuenta_bancaria_id: "c2",
      cuenta: "BP Negocios",
      banco: "Banco Popular",
      moneda: "CRC",
      nombres_en_archivo: ["Banco Popular · BP Negocios"],
      filas: 300,
      fecha_desde: "2025-02-01",
      fecha_hasta: "2025-11-30",
      meses: ["2025-02"],
      total_debitos: "1000000.00",
      total_creditos: "1500000.50",
      debitos_nuevos: "1000000.00",
      creditos_nuevos: "1500000.50",
      nuevas: 270,
      ya_existen: 20,
      sin_partida: 2,
      partida_desconocida: 0,
      sin_tipo_cambio: 5,
      importacion_id: "",
      insertados: 0,
    },
  ],
  cuentas_no_resueltas: [
    { nombre_en_archivo: "BAC Religiosa Vieja", filas: 8, primera_linea: 412, motivo: "no existe" },
  ],
  partidas_faltantes: ["Ventas históricas 2025"],
  errores: [{ linea: 57, motivo: "no se entiende la fecha", texto: "15-ene-2025" }],
  errores_truncados: false,
  aplicado: false,
  insertados: 0,
  aviso: "Revisá las cuentas que no se resolvieron antes de cargar.",
};

const subir = vi.fn();
const confirmar = vi.fn();

function montar(plan: PlanHistorico | null = PLAN) {
  vi.mocked(useSubirHistorico).mockImplementation((() => ({
    mutate: (f: File, opts?: { onSuccess?: (p: PlanHistorico) => void }) => {
      subir(f);
      if (plan) opts?.onSuccess?.(plan);
    },
    isPending: false,
  })) as unknown as typeof useSubirHistorico);
  vi.mocked(useConfirmarHistorico).mockImplementation((() => ({
    mutate: (id: string, opts?: { onSuccess?: (p: PlanHistorico) => void }) => {
      confirmar(id);
      opts?.onSuccess?.({
        ...PLAN,
        aplicado: true,
        insertados: 1150,
        cuentas: PLAN.cuentas.map((c) => ({ ...c, insertados: c.nuevas, importacion_id: `imp-${c.cuenta_bancaria_id}` })),
      });
    },
    isPending: false,
  })) as unknown as typeof useConfirmarHistorico);
  return render(
    <ToastProvider>
      <CargarHistoricoPanel />
    </ToastProvider>,
  );
}

/** Elegir el archivo ya previsualiza: es el único camino a la tabla. */
async function subirArchivo(nombre = "bancos-2025.xlsx") {
  const user = userEvent.setup();
  const input = screen.getByLabelText("Archivo con el histórico de bancos");
  await user.upload(input, new File(["x"], nombre, { type: "application/vnd.ms-excel" }));
  return user;
}

beforeEach(() => vi.clearAllMocks());

describe("CargarHistoricoPanel", () => {
  it("dice en qué se diferencia del otro importador", () => {
    montar();
    expect(screen.getByText(/crea los movimientos/i)).toBeInTheDocument();
    expect(screen.getByText(/No confundir con «Traer la clasificación desde Excel»/)).toBeInTheDocument();
  });

  it("el resumen por cuenta muestra lo que ENTRA, no lo que trae el archivo", async () => {
    montar();
    await subirArchivo();

    const davi = (await screen.findByText("Davivienda · Davivienda Colones")).closest("tr") as HTMLElement;
    // Entran 880 de 900: las otras 20 ya estaban.
    expect(within(davi).getByText("880")).toBeInTheDocument();
    expect(within(davi).getByText("20")).toBeInTheDocument();
    // Y el dinero es el de lo NUEVO (₡3.900.000), no el del archivo (₡4.000.000).
    expect(within(davi).getByText(/3\D?900\D?000/)).toBeInTheDocument();
    expect(within(davi).queryByText(/^₡\s?4\D?000\D?000/)).toBeNull();
    // El nombre con que el archivo la llamó, para poder detectar que resolvió a otra cuenta.
    const bp = screen.getByText("Banco Popular · BP Negocios").closest("tr") as HTMLElement;
    expect(within(bp).getByText(/en el archivo: Banco Popular · BP Negocios/)).toBeInTheDocument();
  });

  it("separa lo que hay que crear de lo que no frena nada, y el adorno no es error", async () => {
    montar();
    await subirArchivo();

    // Cuenta que falta: sus filas NO entran, y se dice el nombre exacto y la línea.
    const bloqueo = (await screen.findByText(/no existen acá/)).closest("div") as HTMLElement;
    expect(within(bloqueo).getByText(/«BAC Religiosa Vieja»/)).toBeInTheDocument();
    expect(within(bloqueo).getByText(/línea 412/)).toBeInTheDocument();

    // Partida que falta: el movimiento SÍ entra.
    expect(screen.getByText(/sí se cargan/)).toBeInTheDocument();
    expect(screen.getByText("Ventas históricas 2025")).toBeInTheDocument();

    // El adorno del reporte se cuenta aparte y dice que no son errores.
    expect(screen.getByText(/6 líneas de formato del reporte \(no son errores\)/)).toBeInTheDocument();
    // Y el error de verdad trae la línea y el texto original.
    expect(screen.getByText(/Línea 57: no se entiende la fecha — «15-ene-2025»/)).toBeInTheDocument();
  });

  it("avisa que el libro tenía otra hoja sin leer", async () => {
    montar();
    await subirArchivo();
    expect(await screen.findByText(/Resumen por partida quedó sin leer/)).toBeInTheDocument();
  });

  it("confirmar manda la carga y después muestra lo que entró", async () => {
    montar();
    const user = await subirArchivo();

    const boton = await screen.findByRole("button", { name: "Cargar histórico" });
    expect(boton).toBeEnabled();
    await user.click(boton);

    expect(confirmar).toHaveBeenCalledWith("carga-1");
    expect(await screen.findByText(/entraron 1.150 movimientos en 2 cuenta\(s\)/)).toBeInTheDocument();
    // Y explica dónde se corrige si una cuenta salió mal.
    expect(screen.getByText(/se revierte sola sin tocar las otras/)).toBeInTheDocument();
    // Ya aplicado: no se puede volver a confirmar el mismo plan.
    expect(screen.queryByRole("button", { name: "Cargar histórico" })).toBeNull();
  });

  it("sin nada que cargar el botón queda deshabilitado y se lee por qué", async () => {
    montar({
      ...PLAN,
      cuentas_no_resueltas: [],
      partidas_faltantes: [],
      errores: [],
      totales: { ...PLAN.totales, nuevas: 0, ya_existen: 1200, errores: 0, sin_cuenta: 0 },
    });
    await subirArchivo();

    expect(await screen.findByText(/todas las filas del archivo ya están en el sistema/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cargar histórico" })).toBeDisabled();
  });

  // El `accept=".xlsx"` del input hace que el selector del sistema ni ofrezca el archivo, así que
  // `userEvent.upload` no lo dejaría pasar y la prueba no probaría nada. Esta guarda existe para el
  // otro camino —arrastrar el archivo, o elegir «todos los archivos» en el diálogo—, y ese es el que
  // se reproduce disparando el change directo.
  it("un archivo que no es .xlsx ni se manda al servidor", async () => {
    montar();
    const input = screen.getByLabelText("Archivo con el histórico de bancos") as HTMLInputElement;
    const csv = new File(["x"], "bancos-2025.csv", { type: "text/csv" });
    Object.defineProperty(input, "files", { value: [csv], configurable: true });
    fireEvent.change(input);

    await waitFor(() =>
      expect(screen.getByText("El archivo debe ser un Excel .xlsx")).toBeInTheDocument(),
    );
    expect(subir).not.toHaveBeenCalled();
  });
});
