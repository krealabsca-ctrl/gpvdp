import { beforeEach, describe, expect, it } from "vitest";

import { abrirReporteLoteImprimible } from "./ReporteLoteImprimible";
import type { ReporteLote } from "@/api/cxp";

/**
 * El documento que se imprime, PARSEADO como lo parsea un navegador.
 *
 * Existe porque un `</style>` que faltaba dejó el documento **completamente en blanco** —el parser
 * no cerraba el head y se tragaba el cuerpo entero— y nada lo detectó: la función no lanzaba, el
 * TypeScript compilaba, las pruebas del monto en letras seguían verdes. Solo se vio abriendo el
 * documento en el navegador. Un HTML armado por concatenación se rompe en silencio, así que acá se
 * arma de verdad y se revisa el DOM resultante, no el texto.
 */

/** Genera el documento y devuelve el DOM que produciría el navegador. */
function documentoDe(r: ReporteLote, empresa = "Valle de Paz"): Document {
  let html = "";
  const original = window.open;
  // @ts-expect-error doble mínimo: solo interesa lo que la función escribiría en la ventana.
  window.open = () => ({ document: { write: (h: string) => { html = h; }, close: () => {} } });
  const ok = abrirReporteLoteImprimible(r, empresa);
  window.open = original;
  expect(ok, "la función dijo que el navegador bloqueó la ventana").toBe(true);
  return new DOMParser().parseFromString(html, "text/html");
}

const UNA_LINEA = {
  consecutivo: "00300001010000336700",
  gasto: "Gastos · Agua",
  fecha_emision: "2026-01-13",
  fecha_vencimiento: "2026-02-13",
  total_crc: "628007.00",
  anticipos_crc: "0",
  neto_crc: "628007.00",
};

function reporte(extra: Partial<ReporteLote> = {}): ReporteLote {
  return {
    lote_id: "l1",
    lote_numero: 36,
    fecha_corte: "2026-10-09",
    modo_pago: "PROVEEDOR",
    facturas: 2,
    transferencias: 1,
    total_crc: "903007.00",
    sin_iban: 0,
    pagos: [
      {
        referencia: "REM 0036-01",
        huella: "CXPR-C8D2EE9B80F4",
        lote_numero: 36,
        fecha_corte: "2026-10-09",
        proveedor: "INVERSIONES DEL PACIFICO JSM, S.A.",
        proveedor_id: "p1",
        iban: "cr12345678987456321023",
        total_crc: "903007.00",
        lineas: [UNA_LINEA, { ...UNA_LINEA, consecutivo: "00300001010000340610", anticipos_crc: "20000.00", neto_crc: "275000.00", total_crc: "295000.00" }],
      },
    ],
    ...extra,
  } as unknown as ReporteLote;
}

describe("el documento de la relación de pagos", () => {
  beforeEach(() => {
    // Cada prueba instala su propio doble; esto solo garantiza que exista algo que reemplazar.
    if (!window.open) window.open = (() => null) as typeof window.open;
  });

  // LA prueba: el cuerpo tiene que existir. Es la que habría atrapado el `</style>` perdido.
  it("el cuerpo del documento no queda vacío", () => {
    const doc = documentoDe(reporte());
    expect(doc.body.textContent?.trim().length, "el <body> salió vacío: el head se tragó el documento").toBeGreaterThan(200);
    expect(doc.head.querySelector("style"), "el documento perdió sus estilos").not.toBeNull();
  });

  it("lleva las partes que lo hacen un documento y no una pantalla", () => {
    const doc = documentoDe(reporte());
    // Identificación: empresa y folio del lote.
    expect(doc.querySelector(".banda .emp")?.textContent).toBe("Valle de Paz");
    expect(doc.querySelector(".folio .v")?.textContent).toContain("36");
    // El total, en números y en letras.
    expect(doc.querySelector(".granTotal .v")?.textContent).toContain("903");
    expect(doc.querySelector(".granTotal .letras")?.textContent?.toLowerCase()).toContain("colones");
    // Las tres firmas, en orden.
    expect([...doc.querySelectorAll(".firma .rol")].map((e) => e.textContent)).toEqual([
      "Preparado por",
      "Revisado por",
      "Autorizado por",
    ]);
    // El sello de emisión: un lote abierto cambia, y sin esto dos impresiones no se pueden ordenar.
    expect(doc.querySelector(".datos")?.textContent).toContain("Emitido");
  });

  it("una transferencia muestra su referencia, su código y el IBAN en mayúsculas", () => {
    const doc = documentoDe(reporte());
    const cab = doc.querySelector(".pago .cab")?.textContent ?? "";
    expect(cab).toContain("REM 0036-01");
    expect(cab).toContain("CXPR-C8D2EE9B80F4");
    // Cargado en minúscula, impreso en mayúscula: es contra este papel que se coteja la cuenta.
    expect(cab).toContain("CR12345678987456321023");
    expect(cab).not.toContain("cr12345678987456321023");
  });

  it("el subtotal aparece solo cuando el pago cubre más de una factura", () => {
    const dos = documentoDe(reporte());
    expect(dos.querySelectorAll("tr.sub")).toHaveLength(1);

    const una = reporte();
    una.pagos[0]!.lineas = [UNA_LINEA];
    // Con una sola factura, repetir el mismo número abajo no agrega nada.
    expect(documentoDe(una).querySelectorAll("tr.sub")).toHaveLength(0);
  });

  it("avisa de las transferencias sin IBAN y las marca una por una", () => {
    const sinIban = reporte({ sin_iban: 1 });
    sinIban.pagos[0]!.iban = "";
    const doc = documentoDe(sinIban);
    expect(doc.querySelector(".aviso")?.textContent).toContain("1 transferencia(s) sin cuenta IBAN");
    expect(doc.querySelectorAll(".chip.mal")).toHaveLength(1);
    expect(doc.querySelector(".pago .cab")?.textContent).toContain("cuenta no registrada");
  });

  it("sin problemas de IBAN no aparece ninguna alerta: un aviso que sale siempre deja de leerse", () => {
    const doc = documentoDe(reporte());
    expect(doc.querySelector(".aviso")).toBeNull();
    expect(doc.querySelectorAll(".chip.mal")).toHaveLength(0);
  });

  it("dice cómo se cortó el lote, que cambia cuánta plata sale por transferencia", () => {
    expect(documentoDe(reporte()).querySelector(".banda .sub")?.textContent).toContain("agrupado por proveedor");
    expect(
      documentoDe(reporte({ modo_pago: "FACTURA" as ReporteLote["modo_pago"] })).querySelector(".banda .sub")?.textContent,
    ).toContain("una transferencia por factura");
  });

  it("un corte sin transferencias lo dice en vez de mostrar una hoja muda", () => {
    const vacio = reporte({ pagos: [], facturas: 0, transferencias: 0, total_crc: "0" });
    const doc = documentoDe(vacio);
    expect(doc.querySelectorAll(".pago")).toHaveLength(0);
    expect(doc.body.textContent).toContain("no tiene ninguna transferencia");
  });

  it("escapa el texto que viene de la base: un nombre no puede inyectar marcado", () => {
    const raro = reporte();
    raro.pagos[0]!.proveedor = '<script>alert(1)</script> & "COMILLAS" S.A.';
    const doc = documentoDe(raro);
    expect(doc.querySelectorAll("script")).toHaveLength(0);
    expect(doc.querySelector(".pago .prov")?.textContent).toContain('& "COMILLAS" S.A.');
  });
});
