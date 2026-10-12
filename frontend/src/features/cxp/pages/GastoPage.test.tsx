import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * La pantalla de gasto, dibujada con datos.
 *
 * LO QUE SE PRUEBA ACÁ ES EL ÚLTIMO TRAMO, que es el que falla en este proyecto: que el usuario
 * LEA la advertencia, no que la función la calcule. Ya pasó antes —los centinelas nuevos salían
 * como 500 y nadie los veía— y acá el riesgo es peor: una lista de gasto armada con el 2 % de las
 * facturas se lee idéntica a una armada con el 100 %. Si la cobertura no se pinta, la pantalla
 * miente sin equivocarse en un solo número.
 */

vi.mock("@/features/bancos/useEmpresaId", () => ({ useEmpresaId: () => "e1" }));

import { gastoApi, type AnalisisGasto } from "@/api/cxp";
import { ToastProvider } from "@/components/ui";
import { GastoPage, VEREDICTO } from "@/features/cxp/pages/GastoPage";

vi.mock("@/api/cxp", async (original) => {
  const mod = await original<typeof import("@/api/cxp")>();
  return {
    ...mod,
    gastoApi: { analisis: vi.fn(), compras: vi.fn(), lineasDeDocumento: vi.fn(), cargarXML: vi.fn() },
  };
});

function pintar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <GastoPage />
      </ToastProvider>
    </QueryClientProvider>,
  );
}

const SIN_DETALLE: AnalisisGasto = {
  cobertura: {
    facturas: 4538,
    facturas_con_detalle: 0,
    monto_total: "1001703533.55",
    monto_con_detalle: "0",
    porcentaje_monto: "0",
    lineas_sin_cabys: 0,
    lineas_sin_convertir: 0,
  },
  items: [],
  total: "0",
  items_total: 0,
};

const PARCIAL: AnalisisGasto = {
  cobertura: {
    facturas: 4538,
    facturas_con_detalle: 90,
    monto_total: "1001703533.55",
    monto_con_detalle: "20000000",
    porcentaje_monto: "2.0",
    lineas_sin_cabys: 4,
    lineas_sin_convertir: 3,
  },
  items: [
    {
      cabys: "2399001",
      descripcion: "CEMENTO GRIS 50KG",
      descripciones: 1,
      gasto_crc: "170000",
      lineas: 3,
      facturas: 3,
      proveedores: 2,
      unidad: "Sacos",
      unidades: 1,
      cantidad: "30",
      precio_min: "5000",
      precio_max: "6500",
      precio_ultimo: "5500",
      variacion_pct: "30.0",
      ultima_compra: "2026-09-15",
      sin_convertir: 0,
    },
    {
      cabys: "2399002",
      descripcion: "ARENA",
      descripciones: 1,
      gasto_crc: "9000",
      lineas: 2,
      facturas: 1,
      proveedores: 1,
      unidad: "Sacos",
      unidades: 2,
      cantidad: "",
      precio_min: "",
      precio_max: "",
      precio_ultimo: "",
      variacion_pct: "",
      ultima_compra: "2026-04-01",
      sin_convertir: 0,
    },
  ],
  total: "179000",
  items_total: 2,
};

describe("GastoPage", () => {
  // SIN XML CARGADO, LA PANTALLA EXPLICA POR QUÉ ESTÁ VACÍA.
  //
  // Hoy `cxp_recepcion` tiene cero filas: las 4.533 facturas entraron por Excel y no hay un solo
  // XML en el sistema. Una pantalla en blanco se leería como «no gastamos nada», que es lo
  // contrario de la verdad. Tiene que decir qué falta y cómo traerlo.
  it("sin detalle cargado, dice qué falta en vez de quedar en blanco", async () => {
    vi.mocked(gastoApi.analisis).mockResolvedValue(SIN_DETALLE);
    pintar();

    expect(await screen.findByText(VEREDICTO.sinDetalle)).toBeInTheDocument();
    // Y el camino para arreglarlo está en ESTA pantalla, no escondido en otra.
    expect(screen.getByRole("button", { name: /cargar xml/i })).toBeInTheDocument();
    // Pero NO el aviso de «cubre una parte del gasto»: con cero facturas cargadas no cubre una
    // parte, no cubre nada, y dos mensajes sobre lo mismo confunden en vez de advertir.
    expect(screen.queryByText(VEREDICTO.coberturaBaja)).not.toBeInTheDocument();
  });

  // CON COBERTURA BAJA, SE AVISA ANTES DE MOSTRAR UN SOLO NÚMERO.
  it("avisa que el análisis cubre solo una parte del gasto", async () => {
    vi.mocked(gastoApi.analisis).mockResolvedValue(PARCIAL);
    pintar();

    expect(await screen.findByText(VEREDICTO.coberturaBaja)).toBeInTheDocument();
    expect(screen.getByText(/2 %/)).toBeInTheDocument();
    expect(screen.getByText(/90 de 4538 factura/i)).toBeInTheDocument();
  });

  // LAS LÍNEAS QUE NO SE PUDIERON CONVERTIR SE DICEN, NO SE CALLAN.
  it("avisa de las líneas sin tipo de cambio y sin CABYS", async () => {
    vi.mocked(gastoApi.analisis).mockResolvedValue(PARCIAL);
    pintar();

    expect(await screen.findByText(/3 línea\(s\) de facturas en otra moneda SIN tipo de cambio/i))
      .toBeInTheDocument();
    expect(screen.getByText(/4 línea\(s\) sin código CABYS/i)).toBeInTheDocument();
  });

  // EL HALLAZGO SE VE: el mismo ítem con 30 % de diferencia entre el precio mínimo y el máximo.
  it("muestra la variación de precio del mismo ítem", async () => {
    vi.mocked(gastoApi.analisis).mockResolvedValue(PARCIAL);
    pintar();

    expect(await screen.findByText("CEMENTO GRIS 50KG")).toBeInTheDocument();
    expect(screen.getByText("30 %")).toBeInTheDocument();
    // Dos proveedores para el mismo ítem: es poder de negociación sin usar.
    expect(screen.getByText("CABYS 2399001", { exact: false })).toBeInTheDocument();
  });

  // Y LO QUE NO SE PUEDE COMPARAR, NO SE COMPARA.
  //
  // La arena vino por saco y por metro cúbico. El backend manda los precios vacíos; la pantalla
  // tiene que decir por qué en vez de pintar un guion mudo, que se lee como «no hay dato».
  it("explica por qué un ítem no muestra precio", async () => {
    vi.mocked(gastoApi.analisis).mockResolvedValue(PARCIAL);
    pintar();

    expect(await screen.findByText("ARENA")).toBeInTheDocument();
    expect(screen.getByText(/unidades distintas/i)).toBeInTheDocument();
    expect(screen.getByText(/2 unidades de medida/i)).toBeInTheDocument();
  });
});
