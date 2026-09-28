import { describe, expect, it } from "vitest";
import {
  avisoFechaSinCargar,
  cargaDeLaBusqueda,
  consejoNoExiste,
  convieneEsperar,
  fraseCargadaHasta,
  mensajeVacio,
  nombreCuenta,
  type CargaDelSegmento,
} from "@/features/bancos/miSegmentoTextos";
import type { CuentaCargadaHasta } from "@/api/bancos";

/**
 * «MI PARTIDA EN BANCOS» — lo que la pantalla AFIRMA cuando el equipo decide si avisa o espera.
 *
 * Los datos imitan la base local de setiembre: 8 cuentas del segmento, la más atrasada es BN Privado
 * de Cartago (al 09/09), cuatro al 10 y tres al 11. El «cargado hasta» viejo (toda la empresa)
 * decía 11/09 y el equipo que esperaba un depósito del 10 en el BN leía «no entró».
 *
 * Esa frase ya NO está en el encabezado de la pantalla (el Director la sacó el 23-set-2026): vive
 * donde se usa para decidir —«Falta un movimiento» y la lista vacía—, y es ahí donde se prueba.
 */

function cuenta(banco: string, alias: string, fecha: string): CuentaCargadaHasta {
  return { id: `${banco}-${alias}`, banco, cuenta: alias, cargado_hasta: fecha };
}

const BN_CARTAGO = cuenta("BN", "BN Privado de Cartago", "2026-09-09");

const SETIEMBRE: CargaDelSegmento = {
  cargado_hasta: "2026-09-09",
  cargado_hasta_cuenta: BN_CARTAGO,
  carga_por_cuenta: [
    BN_CARTAGO,
    cuenta("BAC", "BAC Religiosa", "2026-09-10"),
    cuenta("Banco Popular", "BP Valle de Paz Colones", "2026-09-10"),
    cuenta("BCR", "BCR Religiosa", "2026-09-10"),
    cuenta("Davivienda", "Davivienda Comisiones COPENAE", "2026-09-10"),
    cuenta("BAC", "BAC Valle de Paz Colones", "2026-09-11"),
    cuenta("BN", "BN Valle de Paz Colones", "2026-09-11"),
    cuenta("Davivienda", "Davivienda Colones", "2026-09-11"),
  ],
};

const PAREJAS: CargaDelSegmento = {
  cargado_hasta: "2026-09-11",
  cargado_hasta_cuenta: cuenta("BAC", "BAC Religiosa", "2026-09-11"),
  carga_por_cuenta: [
    cuenta("BAC", "BAC Religiosa", "2026-09-11"),
    cuenta("BN", "BN Valle de Paz Colones", "2026-09-11"),
    cuenta("Davivienda", "Davivienda Colones", "2026-09-11"),
  ],
};

const UNA: CargaDelSegmento = {
  cargado_hasta: "2026-09-11",
  cargado_hasta_cuenta: cuenta("Davivienda", "Davivienda Colones", "2026-09-11"),
  carga_por_cuenta: [cuenta("Davivienda", "Davivienda Colones", "2026-09-11")],
};

const SIN_CUENTAS: CargaDelSegmento = {
  cargado_hasta: "",
  cargado_hasta_cuenta: null,
  carga_por_cuenta: [],
};

describe("nombreCuenta", () => {
  it("usa el alias cuando ya trae el banco (no «BN BN Privado de Cartago»)", () => {
    expect(nombreCuenta({ banco: "BN", cuenta: "BN Privado de Cartago" })).toBe("BN Privado de Cartago");
    expect(nombreCuenta({ banco: "Davivienda", cuenta: "Davivienda Colones" })).toBe("Davivienda Colones");
  });

  it("antepone el banco cuando el alias no lo dice", () => {
    expect(nombreCuenta({ banco: "Banco Popular", cuenta: "BP Valle de Paz Colones" })).toBe(
      "Banco Popular · BP Valle de Paz Colones",
    );
    expect(nombreCuenta({ banco: "BN", cuenta: "Colones" })).toBe("BN · Colones");
  });

  it("sin alias queda el banco", () => {
    expect(nombreCuenta({ banco: "BCR", cuenta: "" })).toBe("BCR");
  });
});

describe("convieneEsperar — la regla ÚNICA", () => {
  it("es posterior a la cuenta más atrasada: conviene esperar", () => {
    expect(convieneEsperar("2026-09-10", "2026-09-09")).toBe(true);
  });

  it("el mismo día o antes ya está cargado en todas", () => {
    expect(convieneEsperar("2026-09-09", "2026-09-09")).toBe(false);
    expect(convieneEsperar("2026-08-31", "2026-09-09")).toBe(false);
  });

  // Sin fecha de carga no hay nada que afirmar: no se inventa un «esperá».
  it("sin fecha de carga o sin fecha buscada no afirma nada", () => {
    expect(convieneEsperar("2026-09-10", "")).toBe(false);
    expect(convieneEsperar("", "2026-09-09")).toBe(false);
    expect(convieneEsperar(undefined, "2026-09-09")).toBe(false);
  });
});


// fraseCargadaHasta es el ladrillo de los dos avisos que SÍ quedaron (el del diálogo y el de la
// lista vacía). Se prueba aparte porque su redacción cambia según cuántas cuentas hay y si van
// parejas, y esa decisión es la que hace que no se invente «la más atrasada» cuando no la hay.
describe("fraseCargadaHasta", () => {
  it("con todas parejas habla del conjunto; con una sola, la nombra", () => {
    expect(fraseCargadaHasta(PAREJAS)).toBe("Tus 3 cuentas están cargadas hasta el 11/09/2026");
    expect(fraseCargadaHasta(UNA)).toBe("Davivienda Colones está cargada hasta el 11/09/2026");
  });

  it("señala la más atrasada cuando hay diferencia", () => {
    expect(fraseCargadaHasta(SETIEMBRE)).toBe(
      "BN Privado de Cartago está cargada hasta el 09/09/2026",
    );
  });

  it("sin cuentas no inventa una fecha", () => {
    expect(fraseCargadaHasta(SIN_CUENTAS)).toBe("");
  });
});

describe("«Falta un movimiento» usa la MISMA fecha que la regla de esperar", () => {
  it("después de la más atrasada: conviene esperar, nombrando la cuenta", () => {
    const t = consejoNoExiste("2026-09-10", SETIEMBRE);
    expect(t).toBe(
      "BN Privado de Cartago está cargada hasta el 09/09/2026, así que ese día puede no estar importado todavía en esa cuenta: conviene esperar antes de avisar.",
    );
  });

  it("con las cuentas parejas dice que ese día no se importó", () => {
    expect(consejoNoExiste("2026-09-12", PAREJAS)).toBe(
      "Tus 3 cuentas están cargadas hasta el 11/09/2026, así que ese día todavía no se importó: conviene esperar antes de avisar.",
    );
  });

  it("un día ya cargado no manda a esperar", () => {
    expect(consejoNoExiste("2026-09-09", SETIEMBRE)).toBe(
      "Puede que no se haya depositado, o que el monto o la fecha sean otros. Revisá el recibo antes de avisar.",
    );
  });

  // Las tres superficies (encabezado, lista vacía, diálogo) no se pueden contradecir: para cada
  // fecha, las tres dicen «esperá» exactamente cuando convieneEsperar lo dice.
  it("encabezado, lista vacía y diálogo nunca se contradicen", () => {
    for (const fecha of ["2026-09-01", "2026-09-09", "2026-09-10", "2026-09-11", "2026-09-12"]) {
      const esperar = convieneEsperar(fecha, SETIEMBRE.cargado_hasta);
      expect(consejoNoExiste(fecha, SETIEMBRE).includes("conviene esperar")).toBe(esperar);
      expect(avisoFechaSinCargar(fecha, SETIEMBRE) !== "").toBe(esperar);
      const vacio = mensajeVacio({
        hayFiltro: true,
        mesActivo: "Septiembre 2026",
        desde: fecha,
        carga: SETIEMBRE,
      });
      expect(vacio.includes("todavía no se importó")).toBe(esperar);
    }
  });

  it("el aviso previo a buscar nombra la misma cuenta", () => {
    expect(avisoFechaSinCargar("2026-09-10", SETIEMBRE)).toBe(
      "BN Privado de Cartago está cargada hasta el 09/09/2026: ese día puede no estar importado todavía.",
    );
    expect(avisoFechaSinCargar("", SETIEMBRE)).toBe("");
  });
});

describe("cargaDeLaBusqueda", () => {
  it("usa la fecha y la cuenta que mandó el servidor con NO_EXISTE", () => {
    const otra = cuenta("BCR", "BCR Religiosa", "2026-09-10");
    const c = cargaDeLaBusqueda({ cargado_hasta: "2026-09-10", cargado_hasta_cuenta: otra }, SETIEMBRE);
    expect(c.cargado_hasta).toBe("2026-09-10");
    expect(c.cargado_hasta_cuenta).toBe(otra);
  });

  it("sin fecha en la respuesta vale la del encabezado", () => {
    expect(cargaDeLaBusqueda({ cargado_hasta: "" }, SETIEMBRE)).toBe(SETIEMBRE);
  });

  // Nunca la fecha de uno con el nombre del otro: sería afirmar algo que ninguno de los dos dijo.
  it("no mezcla la fecha de la respuesta con la cuenta del encabezado", () => {
    const c = cargaDeLaBusqueda({ cargado_hasta: "2026-09-10", cargado_hasta_cuenta: null }, SETIEMBRE);
    expect(c.cargado_hasta_cuenta).toBeNull();
  });
});

describe("mensajeVacio", () => {
  it("la partida vacía dice el mes, o que fue el filtro", () => {
    expect(mensajeVacio({ hayFiltro: false, mesActivo: "Septiembre 2026", carga: SETIEMBRE })).toBe(
      "Todavía no entró nada de tu partida en Septiembre 2026.",
    );
    expect(mensajeVacio({ hayFiltro: true, mesActivo: "Septiembre 2026", carga: SETIEMBRE })).toMatch(
      /^Ningún movimiento de tu partida coincide/,
    );
  });

  // Y nunca habla de lo que nadie clasificó: esa lista se quitó de la pantalla el 23-set-2026.
  it("no menciona créditos sin partida", () => {
    for (const hayFiltro of [true, false]) {
      const t = mensajeVacio({ hayFiltro, mesActivo: "Septiembre 2026", carga: SETIEMBRE });
      expect(t).not.toMatch(/sin partida|nadie clasificó/i);
    }
  });

  it("pedir desde una fecha sin cargar dice que no se importó", () => {
    expect(
      mensajeVacio({ hayFiltro: true, mesActivo: "x", desde: "2026-09-10", carga: SETIEMBRE }),
    ).toBe(
      "BN Privado de Cartago está cargada hasta el 09/09/2026 y pediste desde el 10/09/2026: lo de esa cuenta todavía no se importó.",
    );
  });
});
