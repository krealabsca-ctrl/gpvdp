/**
 * Las guías del manual, ordenadas por LO QUE HAY QUE HACER.
 *
 * No por módulo: el cierre de mes primero, porque es lo que se repite, y después cada cosa cuando
 * aparece. Alguien que entra al sistema no busca «Bancos», busca «cómo cierro el mes».
 *
 * Todo lo que dice acá describe el sistema tal como está (migración 108). Cuando una pantalla
 * cambia, esto cambia con ella: es parte del producto, no un documento aparte.
 */

import type { Guia } from "@/features/ayuda/tipos";

export const GUIAS: Guia[] = [
  {
    id: "orientacion",
    titulo: "Cómo está organizado el sistema",
    resumen: "Los módulos, la empresa activa y por qué no ves todo.",
    rutas: [],
    bloques: [
      {
        tipo: "texto",
        texto:
          "A la izquierda está la barra de módulos. Solo se ven los módulos que tu rol alcanza: si no ves Recursos Humanos, no es que esté escondida, es que tu usuario no la abre.",
      },
      {
        tipo: "tabla",
        encabezados: ["Módulo", "Para qué"],
        filas: [
          ["Bancos", "Cargar los estados de cuenta y decir a qué partida va cada movimiento"],
          ["Cuentas por pagar", "Las facturas de proveedores, desde que llegan hasta que el banco las debita"],
          ["Cuentas por cobrar", "Los contratos de los clientes y lo que deben"],
          ["Calculadora de precios", "Qué cuesta cada plan y hasta dónde se puede descontar"],
          ["Inventario", "Existencias y consumo por sede"],
          ["Recursos Humanos", "Planilla, vacaciones, horas extra"],
          ["Grupo", "Las tres empresas sumadas. Solo lectura"],
          ["Configuración", "Usuarios, permisos, correo saliente, errores del sistema"],
        ],
      },
      {
        tipo: "aviso",
        texto:
          "Arriba a la derecha está la empresa activa. Todo lo que ves es de esa empresa y nada más: cambiarla cambia absolutamente todo lo de la pantalla.",
      },
    ],
  },

  {
    id: "cierre-de-mes",
    titulo: "El cierre de mes, en orden",
    resumen: "Los cinco pasos que se repiten todos los meses. El orden importa.",
    rutas: ["/importador", "/clasificar", "/saldos-diarios", "/cxp/arqueo"],
    bloques: [
      { tipo: "texto", texto: "Cada paso necesita el anterior. Salteárselos deja el mes sin cerrar." },
      {
        tipo: "pasos",
        items: [
          {
            titulo: "Cargar los estados de cuenta",
            donde: "Bancos › Importador",
            texto:
              "Un archivo por cuenta. Puede traer varios meses: no hay que subir mes por mes. Subir dos archivos que se solapan es seguro —el sistema reconoce lo que ya cargó y descarta lo repetido—, así que ante la duda, volvé a subir.",
          },
          {
            titulo: "Clasificar lo que entró",
            donde: "Bancos › Clasificar",
            texto:
              "Cada movimiento necesita su concepto y su partida. El motor propone lo que puede a partir de lo que ya clasificaste antes; vos confirmás o corregís. Clasificá una vez bien y la próxima ya viene propuesto.",
          },
          {
            titulo: "Revisar que el banco cuadre",
            donde: "Bancos › Saldos diarios",
            texto:
              "El saldo que calcula el sistema contra el que dice el estado de cuenta. Si no cuadran, falta cargar algo o hay un movimiento de más.",
          },
          {
            titulo: "El arqueo de pagos",
            donde: "Cuentas por pagar › Arqueo de pagos",
            texto:
              "Las facturas que el sistema da por pagadas y que el banco nunca debitó. Es la pantalla que más se pasa por alto y la que más sirve.",
          },
          {
            titulo: "Cerrar el período",
            donde: "Bancos › Ajustes",
            texto:
              "No se puede cerrar con movimientos sin clasificar. Es a propósito: un mes cerrado con movimientos «no identificados» es un mes que hay que volver a abrir.",
          },
        ],
      },
      {
        tipo: "aviso",
        texto:
          "Si el archivo del banco se rechaza con «formato de banco no reconocido», las causas habituales son tres: la hoja equivocada dentro del Excel, filas de título de más arriba, o que ese banco no esté entre los seis de fábrica. Las dos primeras se arreglan en el archivo; la tercera, en Bancos › Formatos de banco, sin programar nada.",
      },
    ],
  },

  {
    id: "pagar-proveedor",
    titulo: "Pagarle a un proveedor, de principio a fin",
    resumen: "Las seis fases de la Bandeja y qué hace falta en cada una.",
    rutas: ["/cxp/bandeja", "/cxp/dashboard"],
    bloques: [
      {
        tipo: "texto",
        texto:
          "Las facturas viven en Cuentas por pagar › Bandeja, y cada pestaña es una fase. Una factura avanza de izquierda a derecha.",
      },
      {
        tipo: "flujo",
        etapas: ["Recibidas", "Por validar (área)", "Por aprobar", "Por pagar", "En banco", "Pagadas"],
      },
      {
        tipo: "lista",
        items: [
          "Recibidas · Llegaron, falta decir a qué gasto corresponden.",
          "Por validar (área) · El departamento que pidió la compra confirma que la recibió. Solo pasan por acá las facturas que superan el umbral de monto: validar todo hace que no se valide nada.",
          "Por aprobar · La firma financiera. Cuántas firmas hacen falta depende del monto.",
          "Por pagar · Acá se arma el corte: se marcan las facturas, se crea el lote y se baja la macro, que es el archivo que se sube al banco.",
          "En banco · La macro ya se subió y se espera el débito. Al cargar el estado de cuenta, el sistema empareja solo los pagos con sus facturas por un código que viaja en la descripción.",
          "Pagadas · El banco confirmó. Desde acá se le manda el comprobante al proveedor.",
        ],
      },
    ],
  },

  {
    id: "cuando-algo-no-sale-derecho",
    titulo: "Cuando una factura no sale derecho",
    resumen: "Contado que era crédito, pagos por otro medio, rebotes, notas de crédito y anticipos.",
    rutas: ["/cxp/bandeja", "/cxp/arqueo", "/cxp/proveedores"],
    bloques: [
      {
        tipo: "tabla",
        encabezados: ["Situación", "Qué hacer"],
        filas: [
          [
            "La factura dice «contado» pero el convenio es a crédito",
            "Liberarla: queda registrado quién y por qué. Si ese proveedor siempre lo hace, marcalo una vez en su ficha («A quién le creemos sobre este proveedor») y sus facturas nuevas dejan de trancarse.",
          ],
          [
            "Ya se pagó por caja chica, viáticos o tarjeta",
            "«Pagada por otro medio». No la anules: anular dice que la factura no existió.",
          ],
          ["El banco rechazó el pago", "Marcarla rebotada y volver a incluirla en un corte nuevo."],
          [
            "El banco debitó menos",
            "Registrar el pago parcial desde el Arqueo. El saldo queda a la vista en la factura.",
          ],
          [
            "El proveedor emitió una nota de crédito",
            "Cargarla y aplicarla a la factura, antes de aprobarla.",
          ],
          ["Le entregaste un anticipo", "Aplicarlo a la factura antes de aprobar: lo que se paga es el neto."],
        ],
      },
      {
        tipo: "aviso",
        texto:
          "«Antes de aprobar» es literal. Lo que se aprueba es el neto, y es la cifra con la que se arma la macro. Si la nota de crédito llega tarde, el camino es registrar como pago parcial lo que el banco debitó de menos.",
      },
      {
        tipo: "texto",
        texto: "Para ver lo que se le debe de verdad a un proveedor, abrí la factura. Si tiene descuentos, aparece la resta completa:",
      },
      {
        tipo: "formula",
        lineas: [
          "Total de la factura",
          "− Retención            (se entera a Hacienda)",
          "− Anticipos            (ya entregados)",
          "− Notas de crédito     (las emitió el proveedor)",
          "− Ya debitado          (lo que el banco pagó)",
          "= Saldo a pagar",
        ],
      },
      { tipo: "texto", texto: "Si la factura no tiene nada de eso, el recuadro no aparece." },
    ],
  },

  {
    id: "banco-nuevo",
    titulo: "Cuando el sistema no reconoce el archivo de un banco",
    resumen: "Agregar un banco que no viene de fábrica, sin esperar a un programador.",
    rutas: ["/formatos", "/importador"],
    bloques: [
      {
        tipo: "texto",
        texto:
          "Vienen reconocidos seis: BN, BCR, BAC, Davivienda, Promerica y Banco Popular. Si tu banco no está —o cambió cómo exporta—, se configura en Bancos › Formatos de banco.",
      },
      {
        tipo: "pasos",
        items: [
          {
            titulo: "Subí un estado de cuenta de ese banco",
            texto: "No se importa nada: solo se lee para mostrarte las columnas con su número encima.",
          },
          {
            titulo: "Marcá cuál es la fila de los títulos",
            texto:
              "El sistema la reconoce solo casi siempre, y con ella completa las cinco columnas. Revisalas igual.",
          },
          {
            titulo: "Probá con el archivo",
            texto:
              "Muestra los movimientos ya leídos. Mirá que lo que ENTRÓ esté en crédito y lo que SALIÓ en débito.",
          },
          { titulo: "Guardá", texto: "Los próximos archivos de ese banco se reconocen solos." },
        ],
      },
      {
        tipo: "aviso",
        texto:
          "Si el débito y el crédito quedan cambiados, el archivo se lee entero al revés y no se nota hasta que el cuadre del mes no cierra. Por eso «Probar» existe: es un minuto que evita semanas de buscar.",
      },
    ],
  },

  {
    id: "buscar-y-ordenar",
    titulo: "Buscar y ordenar en cualquier lista",
    resumen: "La barra de arriba es la misma en todas, y el orden lo resuelve el servidor.",
    rutas: [],
    bloques: [
      { tipo: "texto", texto: "Toda lista grande tiene la misma barra arriba: buscador, filtros y «Ordenar por»." },
      {
        tipo: "texto",
        texto:
          "El orden se aplica sobre la lista completa, no sobre lo que ves en pantalla. Si ordenás por proveedor, todas sus facturas quedan juntas aunque estén en otra página.",
      },
    ],
  },

  {
    id: "quien-puede-que",
    titulo: "Quién puede qué, y qué hacer si no podés entrar",
    resumen: "Roles, permisos por empresa y el freno de los cinco intentos.",
    rutas: ["/usuarios", "/seguridad"],
    bloques: [
      {
        tipo: "aviso",
        texto:
          "Cada persona tiene su usuario propio. No se comparten: todo lo que pasa queda registrado con quién lo hizo, y una cuenta compartida borra esa trazabilidad justo cuando hace falta.",
      },
      {
        tipo: "tabla",
        encabezados: ["Rol", "Qué hace"],
        filas: [
          ["Auxiliar financiero", "Importa, clasifica y cuadra"],
          ["Supervisor financiero", "Valida la clasificación y aprueba reglas"],
          ["Director financiero", "Dashboard, proyecciones, congelar el tipo de cambio"],
          ["Gerencia general", "Vista consolidada; aprueba facturas"],
          ["Auditor interno", "Solo lectura y trazabilidad. No puede cambiar nada"],
        ],
      },
      {
        tipo: "texto",
        texto:
          "Los permisos se ajustan en Configuración › Seguridad, por rol y por empresa. Una misma persona puede tener roles distintos en empresas distintas.",
      },
      {
        tipo: "texto",
        texto:
          "Si no podés entrar: después de cinco intentos fallidos desde el mismo equipo, el sistema espera quince minutos y te dice cuántos faltan. No es que la cuenta esté bloqueada —desde otro equipo entrás normal—.",
      },
    ],
  },

  {
    id: "antes-de-que-pasen",
    titulo: "Las cosas que conviene saber antes de que pasen",
    resumen: "Tipo de cambio, dólares, cargas revertidas y respaldos.",
    rutas: ["/tipo-cambio"],
    bloques: [
      {
        tipo: "lista",
        items: [
          "El tipo de cambio arranca manual. Se escribe en Bancos › Tipo de cambio. Hay sincronización con el BCCR, pero viene apagada y hay que registrarse para activarla.",
          "Los montos en dólares no se convierten solos. Cuando dos documentos están en monedas distintas, el sistema lo dice y pide que lo resuelva una persona: con qué tipo de cambio convertir es una decisión del negocio.",
          "Lo revertido no desaparece. Si revertís una carga, los movimientos salen de los libros y dejan de sumar, pero quedan guardados: en Bancos › Clasificar se siguen viendo, marcados. Borrarlos dejaría un hueco que nadie puede explicar después.",
          "Los respaldos corren solos todos los días a las 2 de la mañana, en el mismo servidor. Eso protege contra un error humano, no contra perder el servidor: hay que bajarlos periódicamente.",
        ],
      },
    ],
  },

  {
    id: "cuando-algo-falla",
    titulo: "Cuando una pantalla da error",
    resumen: "Qué anotar, a quién llamar y cómo se encuentra la causa exacta.",
    rutas: ["/incidentes"],
    bloques: [
      {
        tipo: "aviso",
        texto:
          "Si una pantalla dice «Error interno del sistema», el mensaje termina en una referencia de seis caracteres. Anotala o sacale una foto: con ella se encuentra la causa exacta en Configuración › Errores del sistema. Sin ella hay que adivinar.",
      },
      {
        tipo: "tabla",
        encabezados: ["Pasa esto", "Es"],
        filas: [
          ["Una pantalla da «error interno»", "Pasá la referencia a quien administra el sistema"],
          [
            "El archivo del banco se rechaza",
            "Mirá el mensaje: dice qué encontró. Si el banco cambió cómo exporta, se arregla en Formatos de banco",
          ],
          ["Una factura no avanza de fase", "Falta una firma o una validación de área: mirá la columna Firmas"],
          ["Un pago no aparece conciliado", "Arqueo de pagos, que es exactamente la pantalla de eso"],
          ["Alguien no ve un módulo", "Permisos: Configuración › Seguridad"],
        ],
      },
    ],
  },
];

/** Busca guías por texto. Mira el título, el resumen y el contenido de los bloques. */
export function buscarGuias(q: string): Guia[] {
  const needle = q.trim().toLowerCase();
  if (!needle) return GUIAS;
  return GUIAS.filter((g) => textoDeGuia(g).includes(needle));
}

/** Todo el texto de una guía, en minúsculas. Es lo que el buscador recorre. */
function textoDeGuia(g: Guia): string {
  const partes: string[] = [g.titulo, g.resumen];
  for (const b of g.bloques) {
    switch (b.tipo) {
      case "texto":
      case "aviso":
        partes.push(b.texto);
        break;
      case "lista":
        partes.push(...b.items);
        break;
      case "pasos":
        for (const p of b.items) partes.push(p.titulo, p.donde ?? "", p.texto);
        break;
      case "tabla":
        partes.push(...b.encabezados, ...b.filas.flat());
        break;
      case "flujo":
        partes.push(...b.etapas);
        break;
      case "formula":
        partes.push(...b.lineas);
        break;
    }
  }
  return partes.join(" ").toLowerCase();
}
