import { describe, expect, it } from "vitest";
import {
  adivinarColumnas,
  filaDeEncabezadoProbable,
  proponerSenales,
} from "@/features/bancos/formatoAdivinado";

/**
 * Adivinar las columnas, contra los encabezados REALES de los seis bancos.
 *
 * No son ejemplos inventados: son las filas que traen los archivos de verdad (están en
 * backend/internal/bancos/bancos_test.go). Si la adivinanza falla justo en los bancos que ya
 * conocemos, no hay razón para creerle en uno nuevo.
 *
 * Lo que está en juego: configurar un formato con el débito y el crédito cambiados lee el archivo
 * entero al revés, y eso no se nota hasta que el cuadre del mes no cierra.
 */

describe("adivinarColumnas", () => {
  const casos: { banco: string; encabezado: string[]; espera: Record<string, number> }[] = [
    {
      banco: "Promerica",
      encabezado: ["Fecha", "Documento", "Descripción", "Débitos", "Créditos", "Saldo"],
      espera: { fecha: 0, documento: 1, descripcion: 2, debito: 3, credito: 4 },
    },
    {
      banco: "BN",
      encabezado: ["oficina", "fechaMovimiento", "numeroDocumento", "debito", "credito", "descripcion"],
      espera: { fecha: 1, documento: 2, descripcion: 5, debito: 3, credito: 4 },
    },
    {
      banco: "BAC",
      encabezado: ["Fecha", "Referencia", "", "Código", "Descripción", "", "", "Débitos", "Créditos", "Balance*"],
      espera: { fecha: 0, documento: 1, descripcion: 4, debito: 7, credito: 8 },
    },
    {
      banco: "BCR",
      encabezado: ["Fecha Contable", "Fecha de Registro", "Hora", "Número Documento", "Descripción", "Oficina", "Débitos", "Créditos"],
      espera: { fecha: 0, documento: 3, descripcion: 4, debito: 6, credito: 7 },
    },
    {
      banco: "Banco Popular",
      encabezado: ["Fecha y Hora", "Descripcion", "Documento", "Debito", "Creditos", "Saldo"],
      espera: { fecha: 0, documento: 2, descripcion: 1, debito: 3, credito: 4 },
    },
    {
      // El que usa vocabulario contable en vez de bancario. Es el caso que justifica los sinónimos.
      banco: "uno con debe/haber",
      encabezado: ["Fecha mov.", "Concepto", "Comprobante", "Haber", "Debe", "Saldo"],
      espera: { fecha: 0, documento: 2, descripcion: 1, debito: 4, credito: 3 },
    },
  ];

  for (const c of casos) {
    it(`acierta las columnas de ${c.banco}`, () => {
      const got = adivinarColumnas(c.encabezado);
      expect(got).toMatchObject(c.espera);
    });
  }

  // «Fecha Contable» y «Fecha de Registro» son dos columnas distintas y el movimiento se fecha por
  // la primera. Sin el orden de los sinónimos, gana la que venga antes en el recorrido.
  it("prefiere «Fecha Contable» sobre «Fecha de Registro»", () => {
    const got = adivinarColumnas(["Fecha de Registro", "Fecha Contable", "Debe", "Haber"]);
    expect(got.fecha).toBe(1);
  });

  // Una columna no puede cumplir dos papeles: si «Descripción» ya se tomó, el siguiente papel
  // tiene que buscar en otra parte.
  it("no asigna la misma columna a dos papeles", () => {
    const got = adivinarColumnas(["Fecha", "Detalle", "Debito", "Credito"]);
    const usadas = [got.fecha, got.descripcion, got.debito, got.credito].filter((n) => n >= 0);
    expect(new Set(usadas).size).toBe(usadas.length);
  });

  // Lo que no está, no se inventa: el -1 es lo que la pantalla marca para que alguien lo complete.
  it("devuelve -1 para el papel que no encuentra", () => {
    const got = adivinarColumnas(["Fecha", "Monto", "Saldo"]);
    expect(got.debito).toBe(-1);
    expect(got.credito).toBe(-1);
  });
});

describe("filaDeEncabezadoProbable", () => {
  it("encuentra el encabezado entre los títulos del archivo", () => {
    const filas = [
      ["TRANSACCIONES POR FECHA"],
      [],
      ["Banco Promerica Costa Rica"],
      ["Número de cuenta: 30000002738132"],
      ["Fecha", "Documento", "Descripción", "Débitos", "Créditos", "Saldo"],
      ["06-16-26", "17130407", "TRASLADO", "", "361,600.00", "497,292.69"],
    ];
    expect(filaDeEncabezadoProbable(filas)).toBe(4);
  });

  // Sin encabezado no se adivina uno: proponer una fila cualquiera haría que la persona configure
  // columnas contra datos, y el formato quedaría mal sin que nada lo avise.
  it("no propone nada cuando no hay encabezado", () => {
    expect(filaDeEncabezadoProbable([["hola"], ["mundo", "otra cosa"]])).toBe(-1);
  });
});

// EL CASO SCOTIABANK (23-oct-2026). Es el que demuestra por qué la adivinanza NO puede pisar una
// configuración guardada: hay bancos donde la configuración correcta va a propósito contra ella.
//
// En el estado de cuenta real, los títulos están corridos respecto de los datos: la columna
// titulada «Descripcion» trae el NÚMERO DE DOCUMENTO y la titulada «Documento» trae la
// descripción. La adivinanza sigue los títulos —no puede hacer otra cosa, no mira los datos— y
// quien configura tiene que invertirlas a mano.
describe("cuando los títulos del banco están corridos respecto de los datos", () => {
  const titulos = ["Fecha y Hora", "Descripcion", "Documento", "Debito", "Creditos", "Saldo"];

  it("la adivinanza sigue los TÍTULOS, que es lo único que puede mirar", () => {
    const got = adivinarColumnas(titulos);
    expect(got.descripcion).toBe(1);
    expect(got.documento).toBe(2);
    // Lo que sí acierta siempre: la fecha y las dos columnas de dinero.
    expect(got.fecha).toBe(0);
    expect(got.debito).toBe(3);
    expect(got.credito).toBe(4);
  });

  // La configuración correcta para este banco es la INVERSA de lo que adivina. Por eso subir el
  // archivo para probar un formato ya guardado no puede volver a adivinar: borraría la corrección
  // en silencio, y nadie se enteraría hasta ver descripciones que son números.
  it("las señales propuestas usan los títulos, nunca los datos", () => {
    const senales = proponerSenales(titulos, adivinarColumnas(titulos));
    for (const s of senales) {
      const titulo = titulos[s.columna]!.toLowerCase();
      expect(
        titulo.includes(s.texto),
        `la señal de la columna ${s.columna} pide «${s.texto}» y ahí dice «${titulos[s.columna]}»`,
      ).toBe(true);
    }
  });
});

describe("proponerSenales", () => {
  it("propone dos señales, que es el mínimo que el servidor exige", () => {
    const enc = ["Fecha", "Documento", "Descripción", "Débitos (DR)", "Créditos", "Saldo"];
    const senales = proponerSenales(enc, adivinarColumnas(enc));
    expect(senales).toHaveLength(2);
    expect(senales[0]).toEqual({ columna: 0, texto: "fecha" });
    // Se corta en el paréntesis: «Débitos (DR)» → «debitos». Lo de adentro es una aclaración del
    // banco y cambia más seguido que el nombre de la columna.
    expect(senales[1]).toEqual({ columna: 3, texto: "debitos" });
  });
});
