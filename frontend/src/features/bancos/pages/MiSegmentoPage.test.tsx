import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

/**
 * «MI PARTIDA EN BANCOS» dibujada con datos.
 *
 * Los hooks de datos se reemplazan por respuestas fijas con la forma EXACTA del contrato del
 * backend: acá no se prueba la red, se prueba que la pantalla diga lo que el Director pidió —la
 * respuesta del aviso donde se ve, «Mis avisos» y la cuenta más atrasada nombrada— y que el pie ya
 * no prometa lo falso.
 *
 * Y lo del 23-set-2026: que NO estén la pestaña «Todavía sin partida» ni el desplegable «hasta
 * cuándo está cargada cada una de tus cuentas». «Esto no debe ser visible por ningún motivo a los
 * consultores»: las pruebas de abajo lo miran desde la pantalla, y las del backend cierran el
 * endpoint.
 */

vi.mock("@/features/bancos/hooks", () => ({
  useMiSegmento: vi.fn(),
  useMisAvisos: vi.fn(),
  useBuscarFaltante: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useReportarFaltante: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useReportarSegmentacion: vi.fn(),
}));
vi.mock("@/app/PeriodoProvider", () => ({
  usePeriodoActivo: () => ({ periodo: "2026-09", setPeriodo: vi.fn() }),
}));
vi.mock("@/features/auth/AuthContext", () => ({
  useAuth: () => ({
    user: { id: "u1", nombre: "Prueba", email: "prueba@valledepazcr.com" },
    empresaActiva: { id: "e1", nombre: "Valle de Paz" },
  }),
}));

import {
  useBuscarFaltante,
  useMiSegmento,
  useMisAvisos,
  useReportarFaltante,
  useReportarSegmentacion,
} from "@/features/bancos/hooks";
import { ToastProvider } from "@/components/ui";
import { MiSegmentoPage } from "@/features/bancos/pages/MiSegmentoPage";
import type {
  ListaMisAvisos,
  MiSegmento,
  MovimientoRow,
  ResultadoFaltante,
  TotalesMovimientos,
} from "@/api/bancos";

function fila(p: Partial<MovimientoRow> & { id: string }): MovimientoRow {
  return {
    fecha: "2026-09-05",
    documento: "",
    descripcion: "",
    // Lo deriva el servidor y solo existe en Davivienda: el default vacío es el caso de las demás.
    consecutivo_largo: "",
    banco: "BN",
    cuenta: "BN Privado de Cartago",
    debito: "0",
    credito: "0",
    moneda: "CRC",
    monto_crc: "0",
    concepto_id: null,
    concepto: "",
    clasificacion_id: null,
    clasificacion: "",
    estado_clasificacion: "NO_IDENTIFICADO",
    confianza: null,
    es_traslado: false,
    incluido: true,
    ...p,
  };
}

function totales(creditos: string): TotalesMovimientos {
  return {
    total_debitos: "0",
    total_creditos: creditos,
    diferencia: creditos,
    sin_tipo_cambio: 0,
    monto_sin_convertir: "0",
    excluidos: 0,
  };
}

const BN_CARTAGO = { id: "c-bn", banco: "BN", cuenta: "BN Privado de Cartago", cargado_hasta: "2026-09-09" };
const DAVI = { id: "c-davi", banco: "Davivienda", cuenta: "Davivienda Colones", cargado_hasta: "2026-09-11" };

const CABECERA = {
  partidas: [{ clasificacion_id: "cl1", clasificacion: "Deposito de Clientes", concepto: "Ingresos" }],
  cuentas: [
    { id: "c-bn", banco: "BN", cuenta: "BN Privado de Cartago" },
    { id: "c-davi", banco: "Davivienda", cuenta: "Davivienda Colones" },
  ],
  cargado_hasta: "2026-09-09",
  cargado_hasta_cuenta: BN_CARTAGO,
  carga_por_cuenta: [BN_CARTAGO, DAVI],
  sin_alcance: false,
};

const PARTIDA: MiSegmento = {
  ...CABECERA,
  movimientos: {
    totales: totales("3900.00"),
    items: [
      fila({
        id: "m-resuelto",
        documento: "55501",
        descripcion: "DEPOSITO VENTANILLA",
        credito: "1400.00",
        clasificacion_id: "cl1",
        clasificacion: "Deposito de Clientes",
        estado_clasificacion: "REVISADO",
        aviso_resuelto: {
          motivo: "esto es de Emergencias",
          resolucion: "SIN_CAMBIO",
          respuesta: "Es un depósito de cliente, la partida está bien",
          resuelto_en: "2026-09-20T15:30:00Z",
        },
      }),
      fila({
        id: "m-abierto",
        documento: "55502",
        credito: "2500.00",
        clasificacion_id: "cl1",
        clasificacion: "Deposito de Clientes",
        estado_clasificacion: "AUTO",
        reporte_abierto: "no es nuestro",
        reporte_abierto_propio: true,
      }),
      // Aviso abierto de OTRA persona (o un faltante enganchado): el servidor manda el texto
      // genérico y la marca en false; el motivo ajeno no viaja.
      fila({
        id: "m-abierto-ajeno",
        documento: "55503",
        descripcion: "DEPOSITO CAJERO",
        // Davivienda: el servidor deriva la referencia larga del SINPE desde la descripción.
        banco: "Davivienda",
        cuenta: "Davivienda Colones",
        consecutivo_largo: "2026080115283000117909629",
        credito: "900.00",
        clasificacion_id: "cl1",
        clasificacion: "Deposito de Clientes",
        estado_clasificacion: "AUTO",
        reporte_abierto: "Ya hay un aviso abierto sobre este movimiento.",
        reporte_abierto_propio: false,
      }),
    ],
    total: 3,
    page: 1,
    page_size: 200,
  },
};

const MIS_AVISOS: ListaMisAvisos = {
  items: [
    // El aviso real de la base local (2336d66a): un FALTANTE enganchado a un movimiento que hoy es
    // de otra partida. Se ve lo que la persona escribió, nada del movimiento enganchado.
    {
      id: "2336d66a",
      es_faltante: true,
      motivo: "depósito de la planilla",
      creado_en: "2026-09-15T16:00:00Z",
      fecha: "2026-08-13",
      documento: "",
      monto: "805000.00",
      moneda: "",
      banco: "",
      cuenta: "",
      referencia: "10403957",
      estado: "EN_REVISION",
      resolucion: "",
      respuesta: "",
      resuelto_en: "",
    },
    // Reclasificado a otra partida: ya no está en «Mi partida», y la respuesta tiene que leerse bien.
    {
      id: "av-movido",
      es_faltante: false,
      motivo: "esto es de Asociaciones",
      creado_en: "2026-09-10T14:00:00Z",
      fecha: "2026-09-08",
      documento: "44401",
      monto: "12000.00",
      moneda: "CRC",
      banco: "BN",
      cuenta: "BN Privado de Cartago",
      referencia: "",
      estado: "RESUELTO",
      resolucion: "RECLASIFICADO",
      respuesta: "Lo pasé a Asociaciones",
      resuelto_en: "2026-09-21T18:00:00Z",
    },
  ],
  total: 2,
  abiertos: 1,
  resueltos: 1,
  page: 1,
  page_size: 50,
  sin_alcance: false,
};

const reportar = vi.fn();

function consulta<T>(data: T) {
  return { data, isPending: false, isError: false, error: null, refetch: vi.fn() };
}

function montar(opciones: { partida?: MiSegmento } = {}) {
  vi.mocked(useMiSegmento).mockImplementation((() =>
    consulta(opciones.partida ?? PARTIDA)) as unknown as typeof useMiSegmento);
  vi.mocked(useMisAvisos).mockImplementation((() => consulta(MIS_AVISOS)) as unknown as typeof useMisAvisos);
  vi.mocked(useReportarSegmentacion).mockImplementation((() => ({
    mutate: reportar,
    isPending: false,
  })) as unknown as typeof useReportarSegmentacion);
  return render(
    <ToastProvider>
      <MiSegmentoPage />
    </ToastProvider>,
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.clearAllMocks();
});

describe("MiSegmentoPage", () => {
  it("hay DOS pestañas, con su conteo real y la respuesta nueva", () => {
    montar();
    expect(screen.getByRole("tab", { name: /Mi partida/ })).toHaveTextContent("3");
    expect(screen.getByRole("tab", { name: /Mis avisos/ })).toHaveTextContent("1 respuesta nueva");
    expect(screen.getAllByRole("tab")).toHaveLength(2);

    const llamadas = vi.mocked(useMiSegmento).mock.calls.map((c) => c[0]);
    expect(llamadas).toContainEqual(
      expect.objectContaining({ page: 1, page_size: 200, periodo: "2026-09" }),
    );
  });

  // La corrección del 23-set-2026, mirada desde donde el Director la señaló: la pestaña y el
  // desplegable. Se comprueban las dos cosas juntas porque vienen de la misma frase suya.
  it("no existe «Todavía sin partida» ni el desplegable de fechas por cuenta", () => {
    montar();
    expect(screen.queryByRole("tab", { name: /Todavía sin partida/ })).toBeNull();
    expect(screen.queryByText(/Todavía sin partida/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Es de mi partida" })).toBeNull();
    // La pantalla no pide esa vista por ningún lado (el servidor la responde 400).
    const llamadas = vi.mocked(useMiSegmento).mock.calls.map((c) => c[0] as Record<string, unknown>);
    for (const l of llamadas) expect(l).not.toHaveProperty("vista");

    // El desplegable: ni el título, ni la fecha de la cuenta que NO es la más atrasada.
    expect(screen.queryByText(/Hasta cuándo está cargada cada una de tus cuentas/)).toBeNull();
    expect(screen.queryByText(/Davivienda Colones: /)).toBeNull();
    expect(screen.queryByText(/11\/09\/2026/)).toBeNull();
  });

  // La leyenda que el Director mandó quitar el 23-set-2026, entera: el mes, la fecha de carga del
  // banco y el total en colones de la partida. Lo que queda arriba de la tabla es el paginador.
  it("no hay leyenda de mes, ni fecha de carga, ni total en colones", () => {
    montar();
    expect(screen.queryByText(/El último movimiento de/)).toBeNull();
    expect(screen.queryByText(/BN Privado de Cartago es del/)).toBeNull();
    expect(screen.queryByText(/puede no estar importado todavía/)).toBeNull();
    expect(screen.queryByText(/^Mostrando Septiembre 2026/)).toBeNull();
    expect(screen.queryByText(/Los créditos que ya están en tu partida/)).toBeNull();
    // El total en colones tampoco por otro lado: 3.900,00 son los créditos de PARTIDA.
    expect(screen.queryByText(/3\s?900,00/)).toBeNull();
    // Y el paginador sigue diciendo cuántos son, que es lo que impide conciliar viendo una página.
    expect(screen.getByText(/3 movimientos/)).toBeInTheDocument();
  });

  // Los dos avisos del rango NO son leyenda: solo salen cuando lo que se muestra no es lo que el
  // usuario pidió. El primero existe porque una fecha a medio teclear devolvía otra lista en
  // silencio; si se hubiera ido con la leyenda, volvía ese defecto.
  it("el aviso del rango mal escrito sigue saliendo", async () => {
    const user = userEvent.setup();
    montar();
    expect(screen.queryByText(/todavía no apliqué el rango de fechas/)).toBeNull();
    fireEvent.change(screen.getByLabelText("Desde"), { target: { value: "0202-09-01" } });
    expect(await screen.findByText(/todavía no apliqué el rango de fechas/)).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Desde"), { target: { value: "2026-09-01" } });
    expect(
      await screen.findByText(/Mostrando el rango de fechas que pediste, no el mes activo/),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Volver a Septiembre 2026/ }));
    expect(screen.queryByText(/Mostrando el rango de fechas/)).toBeNull();
  });

  it("la fila muestra el consecutivo largo, y un «—» donde el banco no lo publica", () => {
    montar();
    // Davivienda sí lo trae (lo deriva el servidor de la descripción).
    const conDavi = screen.getByText("DEPOSITO CAJERO").closest("tr") as HTMLElement;
    expect(within(conDavi).getByText("2026080115283000117909629")).toBeInTheDocument();
    // El BN no: la celda dice «—», no queda vacía.
    const sinLargo = screen.getByText("DEPOSITO VENTANILLA").closest("tr") as HTMLElement;
    expect(within(sinLargo).getByText("55501")).toBeInTheDocument();
    expect(within(sinLargo).getAllByText("—").length).toBeGreaterThan(0);
    // Y el encabezado explica el blanco, para que no se lea como un dato perdido.
    expect(
      screen.getByRole("columnheader", { name: /Consecutivo largo/ }),
    ).toHaveAttribute("title", expect.stringContaining("Davivienda"));
  });

  it("el pie ya no promete lo falso", () => {
    montar();
    expect(screen.queryByText(/en esta misma pantalla/)).toBeNull();
    // El pie: la respuesta queda SIEMPRE en «Mis avisos»; en la fila, solo mientras el movimiento
    // siga acá. Ya no afirma que queda «en la fila» cuando el movimiento deja de aparecer.
    expect(screen.queryByText(/deja de aparecer acá/)).toBeNull();
    expect(screen.getByText(/La respuesta a cada aviso tuyo queda siempre en «Mis avisos»/)).toBeInTheDocument();
    expect(screen.getByText(/mientras siga en esta lista y no haya otro aviso abierto sobre él/)).toBeInTheDocument();
    expect(screen.getByText(/si esperás uno que no ves, buscalo con «Falta un movimiento»/)).toBeInTheDocument();
  });

  it("«Está mal segmentado» manda el movimiento y el motivo", async () => {
    const user = userEvent.setup();
    montar();
    await user.click(screen.getByRole("button", { name: "Está mal segmentado" }));

    expect(screen.getByRole("heading", { name: "Avisar que está mal segmentado" })).toBeInTheDocument();
    await user.type(screen.getByLabelText("¿Qué está mal? *"), "es de Emergencias");
    await user.click(screen.getByRole("button", { name: "Enviar el aviso" }));
    expect(reportar).toHaveBeenCalledWith(
      { movimientoId: "m-resuelto", motivo: "es de Emergencias" },
      expect.anything(),
    );
  });

  it("un aviso abierto ajeno o de faltante dice que existe, sin citar a nadie", () => {
    montar();
    const ajena = screen.getByText("Ya hay un aviso abierto sobre este movimiento.").closest("tr") as HTMLElement;
    expect(within(ajena).getByText("DEPOSITO CAJERO")).toBeInTheDocument();
    // Sigue en revisión y sin botón: avisar otra vez daría 409.
    expect(within(ajena).getByText("En revisión")).toBeInTheDocument();
    expect(within(ajena).queryByRole("button")).toBeNull();
    expect(screen.queryByText("«Ya hay un aviso abierto sobre este movimiento.»")).toBeNull();
    // El motivo PROPIO sí va citado.
    expect(screen.getByText("«no es nuestro»")).toBeInTheDocument();
  });

  it("la fila con un aviso resuelto muestra la respuesta y cuándo, con el botón al lado", () => {
    montar();
    const filaResuelta = screen.getByText("DEPOSITO VENTANILLA").closest("tr");
    expect(filaResuelta).not.toBeNull();
    const f = within(filaResuelta as HTMLElement);
    expect(f.getByText("Revisado, sin cambio")).toBeInTheDocument();
    expect(f.getByText("«Es un depósito de cliente, la partida está bien»")).toBeInTheDocument();
    expect(f.getByText(/2026/, { selector: "span.tabular-nums" })).toBeInTheDocument();
    expect(f.getByRole("button", { name: "Está mal segmentado" })).toBeInTheDocument();

    // La fila con el aviso ABIERTO sigue en revisión, sin botón y sin respuesta.
    const filaAbierta = screen.getByText("«no es nuestro»").closest("tr") as HTMLElement;
    expect(within(filaAbierta).getByText("En revisión")).toBeInTheDocument();
    expect(within(filaAbierta).queryByRole("button")).toBeNull();
  });

  it("«Mis avisos»: el faltante huérfano y el movido de partida se leen bien, lo nuevo se nota", async () => {
    const user = userEvent.setup();
    const { unmount } = montar();
    await user.click(screen.getByRole("tab", { name: /Mis avisos/ }));

    // El faltante: lo que la persona escribió, en revisión.
    const faltante = screen.getByText(/esperabas el 13\/08\/2026/).closest("tr") as HTMLElement;
    expect(within(faltante).getByText("ref. 10403957")).toBeInTheDocument();
    expect(within(faltante).getByText("En revisión")).toBeInTheDocument();
    expect(within(faltante).getByText("«depósito de la planilla»")).toBeInTheDocument();

    // El movido de partida: la respuesta y la explicación, no un error.
    const movido = screen.getByText("«Lo pasé a Asociaciones»").closest("tr") as HTMLElement;
    expect(within(movido).getByText("Corregido")).toBeInTheDocument();
    expect(within(movido).getByText("Respuesta nueva")).toBeInTheDocument();
    expect(within(movido).getByText(/no se perdió/)).toBeInTheDocument();

    // Al mostrarla quedó vista: la próxima vez la pestaña ya no la anuncia.
    unmount();
    montar();
    expect(screen.getByRole("tab", { name: /Mis avisos/ })).not.toHaveTextContent("respuesta nueva");
  });

  it("sin alcance explica qué falta y no muestra pestañas ni tabla", () => {
    const vacia: MiSegmento = {
      partidas: [],
      cuentas: [],
      movimientos: { totales: totales("0"), items: [], total: 0, page: 1, page_size: 0 },
      cargado_hasta: "",
      cargado_hasta_cuenta: null,
      carga_por_cuenta: [],
      sin_alcance: true,
      aviso: "Tu rol todavía no tiene partidas asignadas para consulta.",
    };
    montar({ partida: vacia });
    expect(screen.getByText("Todavía no tenés ninguna partida asignada")).toBeInTheDocument();
    expect(screen.queryByRole("tab")).toBeNull();
  });

  describe("«Falta un movimiento»", () => {
    /** La búsqueda contesta `r` al instante (el hook real es una mutación). */
    function buscarDevuelve(r: ResultadoFaltante) {
      vi.mocked(useBuscarFaltante).mockImplementation((() => ({
        mutate: (_vars: unknown, opts?: { onSuccess?: (r: ResultadoFaltante) => void }) =>
          opts?.onSuccess?.(r),
        isPending: false,
      })) as unknown as typeof useBuscarFaltante);
    }

    async function buscar(user: ReturnType<typeof userEvent.setup>) {
      await user.click(screen.getByRole("button", { name: "Falta un movimiento" }));
      fireEvent.change(screen.getByLabelText("Fecha del movimiento *"), {
        target: { value: "2026-09-11" },
      });
      await user.type(screen.getByLabelText("Monto exacto *"), "15000");
      await user.click(screen.getByRole("button", { name: "Buscar en el banco" }));
    }

    it("el de mi partida se muestra completo: lo tapaba un filtro, no hay nada que avisar", async () => {
      const user = userEvent.setup();
      buscarDevuelve({
        veredicto: "EN_MI_PARTIDA",
        movimientos: [
          fila({ id: "m-encontrado", documento: "88801", descripcion: "SINPE ROJAS", credito: "15000.00" }),
        ],
        cargado_hasta: "",
        cargado_hasta_cuenta: null,
      });
      montar();
      await buscar(user);

      expect(screen.getByText("Sí está, y es de tu partida.")).toBeInTheDocument();
      expect(screen.getByText(/SINPE ROJAS/)).toBeInTheDocument();
      expect(screen.queryByLabelText("¿Qué esperabas? *")).toBeNull();
    });

    it.each(["FUERA_DE_MI_PARTIDA", "NO_EXISTE"] as const)(
      "%s: el formulario del faltante sigue funcionando",
      async (veredicto) => {
        const user = userEvent.setup();
        const avisar = vi.fn();
        vi.mocked(useReportarFaltante).mockImplementation((() => ({
          mutate: avisar,
          isPending: false,
        })) as unknown as typeof useReportarFaltante);
        buscarDevuelve({ veredicto, movimientos: [], cargado_hasta: "", cargado_hasta_cuenta: null });
        montar();
        await buscar(user);

        if (veredicto === "FUERA_DE_MI_PARTIDA") {
          // Desde el 23-set-2026 acá cae TAMBIÉN lo que nadie clasificó: se dice que existe y nada
          // más, sin nombrar cuenta, referencia ni descripción.
          expect(screen.getByText("Sí entró ese monto ese día, pero no en tu partida.")).toBeInTheDocument();
          expect(screen.getByText(/otra partida, o todavía sin partida/)).toBeInTheDocument();
        }
        expect(screen.queryByRole("button", { name: "Es de mi partida" })).toBeNull();
        await user.type(screen.getByLabelText("¿Qué esperabas? *"), "depósito de ventanilla");
        await user.type(screen.getByLabelText("Referencia (opcional)"), "REC-1");
        await user.click(screen.getByRole("button", { name: "Enviar el aviso" }));
        expect(avisar).toHaveBeenCalledWith(
          { fecha: "2026-09-11", monto: "15000", referencia: "REC-1", motivo: "depósito de ventanilla" },
          expect.anything(),
        );
      },
    );
  });

  it("la partida vacía lo explica, no queda en blanco", () => {
    montar({
      partida: {
        ...PARTIDA,
        movimientos: { totales: totales("0"), items: [], total: 0, page: 1, page_size: 200 },
      },
    });
    expect(
      screen.getByText("Todavía no entró nada de tu partida en Septiembre 2026."),
    ).toBeInTheDocument();
  });
});
