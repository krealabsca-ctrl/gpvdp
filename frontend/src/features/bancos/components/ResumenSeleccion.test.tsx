import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

/**
 * EL DEFECTO (Director Financiero, 16-oct-2026): filtrando por un documento, el encabezado decía
 * «2 movimientos · créditos ₡0,00» sobre una lista de DOS filas de ₡75 000 cada una. Y al cargar
 * otra vez: «3 movimientos · ₡75 000» sobre TRES filas de ₡75 000.
 *
 * Ninguno de los dos números estaba mal. El conteo cuenta todo lo que la lista muestra; los
 * totales de plata no suman lo que está fuera de los libros. Lo que faltaba era DECIRLO: el
 * backend ya devolvía `excluidos` y el comentario de ese campo advertía, con todas las letras,
 * que «sin mostrarlo, los dos números se contradicen en silencio».
 *
 * La pantalla nunca lo leyó. Así se ve una reversa bien hecha cuando nadie la explica: como si el
 * sistema hubiera perdido la plata.
 */

vi.mock("@/features/bancos/hooks", () => ({ useResumenSeleccion: vi.fn() }));

import { useResumenSeleccion } from "@/features/bancos/hooks";
import { ResumenSeleccion } from "./ResumenSeleccion";
import type { ResumenSeleccion as Resumen } from "@/api/bancos";

function consulta(data: Resumen) {
  return { data, isPending: false, isError: false, isFetching: false, error: null };
}

function resumen(p: Partial<Resumen> = {}): Resumen {
  return {
    agrupar: "concepto",
    movs: 3,
    total_debito: "0",
    total_credito: "75000",
    neto: "75000",
    conceptos: [],
    excluidos: 0,
    ...p,
  };
}

function montar(r: Resumen) {
  vi.mocked(useResumenSeleccion).mockImplementation((() =>
    consulta(r)) as unknown as typeof useResumenSeleccion);
  return render(<ResumenSeleccion filtros={{}} agrupar="concepto" />);
}

describe("ResumenSeleccion · lo que no suma", () => {
  it("avisa cuántos movimientos están fuera de los libros", () => {
    // El caso real: 3 filas visibles, 2 revertidas, y un total que solo cuenta la que queda.
    montar(resumen({ movs: 3, excluidos: 2, total_credito: "75000" }));
    expect(screen.getByText(/2 fuera de los libros/)).toBeInTheDocument();
    // Y explica la consecuencia, que es lo que resuelve la contradicción aparente.
    expect(screen.getByText(/no suman/)).toBeInTheDocument();
  });

  it("el caso extremo: TODO está revertido y los totales quedan en cero", () => {
    // Es la primera captura del reporte: «2 movimientos · créditos ₡0,00» con dos filas visibles.
    montar(resumen({ movs: 2, excluidos: 2, total_credito: "0", neto: "0" }));
    expect(screen.getByText(/2 fuera de los libros/)).toBeInTheDocument();
  });

  it("sin excluidos no aparece el aviso ni se pierde la descripción", () => {
    // Un aviso que sale siempre deja de leerse. Y la nota normal tiene que seguir ahí.
    montar(resumen({ movs: 3, excluidos: 0 }));
    expect(screen.queryByText(/fuera de los libros/)).toBeNull();
  });

  it("el conteo sigue mostrando TODO lo que la lista trae", () => {
    // El número no se corrige restando los excluidos: la lista los muestra, y si el encabezado
    // dijera 1 sobre 3 filas visibles, la contradicción sería la contraria.
    montar(resumen({ movs: 3, excluidos: 2 }));
    expect(screen.getByText("3")).toBeInTheDocument();
  });
});
