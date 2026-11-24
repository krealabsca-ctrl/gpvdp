/**
 * La ayuda de CADA pantalla.
 *
 * Es otra pregunta que la del manual. «¿Cómo cierro el mes?» se hace antes de empezar; «¿qué hago
 * acá?» se hace parado frente a la pantalla, y para contestarla nadie abre un documento. Por eso
 * esto aparece en el mismo encabezado, a un clic, y por eso cada entrada es corta: dos a cuatro
 * puntos. Más que eso no se lee.
 *
 * NO ESTÁN LAS 68 PANTALLAS, Y ES A PROPÓSITO. El botón de ayuda aparece SOLO donde hay algo
 * escrito. Un «?» que abre «no hay ayuda para esta pantalla» es peor que no tenerlo: enseña a no
 * hacer clic, y entonces tampoco se usa donde sí hay. Están las del flujo que se repite todos los
 * meses y las que la gente se traba, que son las que el manual ya identificaba.
 */

import type { AyudaDePantalla } from "@/features/ayuda/tipos";

export const AYUDA_POR_PANTALLA: AyudaDePantalla[] = [
  // ── Bancos ────────────────────────────────────────────────────────────────
  {
    ruta: "/importador",
    queEs: "Acá se cargan los estados de cuenta que baja cada banco.",
    puntos: [
      "Un archivo por cuenta, y puede traer varios meses: no hace falta subir mes por mes.",
      "Volver a subir un archivo que ya cargaste es seguro. El sistema reconoce lo repetido y lo descarta, así que ante la duda, subilo.",
      "Primero muestra lo que entendió y recién después confirmás. Nada entra a los libros antes de eso.",
      "Si dice «formato de banco no reconocido»: revisá que sea la hoja correcta del Excel y que no le falten filas de arriba. Si el banco no está entre los seis de fábrica, agregalo en «Formatos de banco».",
    ],
    guia: "cierre-de-mes",
  },
  {
    ruta: "/clasificar",
    queEs: "Decirle a cada movimiento del banco a qué partida pertenece.",
    puntos: [
      "El motor propone a partir de lo que ya clasificaste antes. Clasificá una vez bien y la próxima llega propuesto.",
      "Con muchos movimientos iguales, usá la pestaña «Patrones»: agrupa lo sin clasificar por la forma de la descripción y resolvés el grupo entero.",
      "Un movimiento que salió de los libros —una carga revertida, un duplicado corregido— se sigue viendo acá, marcado y sin sumar.",
    ],
    guia: "cierre-de-mes",
  },
  {
    ruta: "/mi-segmento",
    queEs: "Lo que entró al banco en las partidas que tu rol consulta. Solo ingresos.",
    puntos: [
      "El alcance lo pone tu rol, no se elige: ves tus partidas y ninguna otra.",
      "Si buscás un depósito y no aparece, fijate en el aviso de «también aparece en» antes de concluir que no entró: puede ser de otro mes.",
      "«Falta un movimiento» levanta el aviso a Finanzas con la fecha y el monto que esperabas.",
    ],
  },
  // Las dos de abajo son para gente de FUERA de Finanzas —jefes de área, gerencias—, que entra
  // a una sola pantalla y no conoce el resto del sistema. Son las que más necesitan el «?».
  {
    ruta: "/mi-presupuesto",
    queEs: "Cómo va el gasto de tu área este mes, contra lo que tiene presupuestado.",
    puntos: [
      "El alcance va por persona: ves las áreas de las que sos responsable y ninguna otra.",
      "Si no hay presupuesto cargado, igual ves lo gastado, pero sin semáforo: no hay contra qué compararlo. Lo carga quien administra el presupuesto.",
      "Un mes a medio clasificar consume menos de lo que consumió. Si el aviso dice que falta clasificar, el porcentaje todavía va a subir.",
      "No muestra movimientos del banco: es el total de tu área, no el detalle de cada pago.",
    ],
  },
  {
    ruta: "/mi-ingreso",
    queEs: "Cuánto entró por tus partidas de cobro este mes, contra la meta.",
    puntos: [
      "El alcance va por persona y por partida: ves tus canales de cobro y ninguno más.",
      "Acá superar la meta es lo bueno: la diferencia en verde es a favor, en rojo es lo que falta.",
      "Si no hay meta declarada, ves lo que entró sin nada contra qué compararlo. La declara Finanzas en el presupuesto de ingreso.",
      "Un depósito que nadie clasificó todavía no aparece en ninguna partida. Si esperabas uno y no está, puede estar sin clasificar.",
    ],
  },
  {
    ruta: "/analisis",
    queEs: "Cada partida mes a mes contra su propia historia: qué se movió, cuánto pesa y qué explica el cambio.",
    puntos: [
      "«Comparar contra» cambia la vara: el promedio se infla cuando un mes trae un pago grande, y a partir de ahí los meses normales parecen una caída. La mediana no se mueve. Si las dos dan lecturas muy distintas, la partida tiene pagos salteados.",
      "Una partida marcada «esporádico» casi no aparece en el mes: su promedio mensual no significa nada, porque no hay un ritmo del que apartarse.",
      "«Qué explica la variación» contesta en plata quién movió el total; la tabla contesta en porcentaje qué pasó con cada partida. Para una reunión sirve la primera.",
      "La estacionalidad necesita el mismo mes repetido en dos años. Los meses con uno solo salen en gris: ahí todavía no hay patrón, hay un dato.",
    ],
  },
  {
    ruta: "/saldos-diarios",
    queEs: "El saldo que calcula el sistema contra el que dice el banco.",
    puntos: [
      "Si no cuadran, falta cargar algo o hay un movimiento de más.",
      "Es el paso 3 del cierre: se hace después de clasificar y antes del arqueo.",
    ],
    guia: "cierre-de-mes",
  },
  {
    ruta: "/formatos",
    queEs: "Enseñarle al sistema a leer el estado de cuenta de un banco que no viene de fábrica.",
    puntos: [
      "Primero subí un archivo de ese banco: la grilla te muestra las columnas con su número, y el sistema completa lo que puede.",
      "«Probar» corre el formato contra el archivo y muestra los movimientos ya leídos. Nada se guarda hasta que vos lo digas.",
      "Mirá que lo que ENTRÓ esté en crédito y lo que SALIÓ en débito: si están cambiados, el archivo se lee al revés.",
      "Los seis bancos de fábrica siguen funcionando igual. Lo que configurés acá se prueba antes que ellos.",
    ],
    guia: "banco-nuevo",
  },
  {
    ruta: "/tipo-cambio",
    queEs: "El tipo de cambio con el que se convierten los montos en dólares.",
    puntos: [
      "Arranca manual. La sincronización con el BCCR existe pero viene apagada.",
      "Congelar el mes fija el tipo de cambio de todo lo de ese período: se hace cuando el mes está completo.",
    ],
    guia: "antes-de-que-pasen",
  },

  // ── Cuentas por pagar ─────────────────────────────────────────────────────
  {
    ruta: "/cxp/bandeja",
    queEs: "Todas las facturas de proveedores. Cada pestaña es una fase, y avanzan de izquierda a derecha.",
    puntos: [
      "Una factura no salta fases: si no avanza, falta una firma o la validación del área. Mirá la columna «Firmas».",
      "En «Por pagar» se arma el corte: se marcan las facturas, se crea el lote y se baja la macro para el banco.",
      "Las notas de crédito y los anticipos se aplican ANTES de aprobar: lo que se aprueba es el neto, y es la cifra con la que se arma la macro.",
    ],
    guia: "pagar-proveedor",
  },
  {
    ruta: "/cxp/arqueo",
    queEs: "Las facturas que el sistema da por pagadas y que el banco nunca debitó.",
    puntos: [
      "Para cada una ofrece los débitos que podrían ser ese pago. Si alguno calza exacto, se enlaza de un clic.",
      "Si el banco pagó de menos, se registra como pago parcial y la factura queda debiendo el resto.",
      "Arriba dice hasta qué fecha llega el estado de cuenta cargado: todo se lee contra esa fecha, y lo más nuevo se cuenta aparte porque el banco todavía no lo alcanzó.",
    ],
    guia: "cierre-de-mes",
  },
  {
    ruta: "/cxp/proveedores",
    queEs: "El maestro de proveedores: cuenta IBAN, retención, condición de pago y gasto predeterminado.",
    puntos: [
      "Sin cuenta IBAN el banco rechaza la línea de pago, así que esos proveedores no se pueden pagar por transferencia.",
      "Si un proveedor siempre declara «contado» en sus comprobantes pero el convenio es a crédito, marcalo una vez acá y sus facturas nuevas dejan de nacer trancadas.",
      "El gasto predeterminado hace que sus facturas lleguen ya clasificadas.",
    ],
    guia: "cuando-algo-no-sale-derecho",
  },
  {
    ruta: "/cxp/dashboard",
    queEs: "Cómo va la cartera de proveedores: lo vencido, lo que viene y dónde está trabada la plata.",
    puntos: [
      "El aging es navegable: desde cada tramo se llega a las facturas que lo componen.",
      "Lo que muestra es la cartera a hoy, no un cierre: cambia con cada factura que entra.",
    ],
    guia: "pagar-proveedor",
  },

  // ── Contabilidad ──────────────────────────────────────────────────────────
  {
    ruta: "/conta/plan",
    queEs: "El catálogo contable del grupo: dónde se registra cada colón.",
    puntos: [
      "Todavía no contabiliza nada. Es el paso 1: tener el plan dentro del sistema. Los asientos vienen después.",
      "Se carga desde el Excel del contador. Antes de escribir nada muestra qué cuentas entran y cuáles quedan fuera.",
      "Una cuenta que agrupa a otras no admite asientos: si los admitiera, el mismo monto se sumaría dos veces al totalizar.",
      "Si el archivo trae el código de Softland, se guarda: es el puente para traer los saldos iniciales.",
    ],
  },
  {
    ruta: "/conta/mapeo",
    queEs: "A qué cuenta del plan corresponde cada partida que ya usás a diario.",
    puntos: [
      "Tampoco genera asientos todavía. Es el paso 2, y el porcentaje de arriba dice cuánto falta para poder generarlos: se mide sobre las partidas que SE USAN, porque una que nadie usó no impide contabilizar.",
      "Trabajá en familias: filtrá «caja chica», marcá todas y mandalas de una vez a su cuenta. De a una no se termina nunca.",
      "Si una partida no produce asiento —un traslado entre cuentas propias, por ejemplo— declarala no contabilizable y escribí por qué. Eso también es terminarla.",
      "El aviso ⚠ de naturaleza cruzada no bloquea: avisa. La caja chica es «Gasto» en el sistema y «Activo» en el plan, y las dos cosas están bien; lo que no puede pasar es que nadie lo mire.",
    ],
  },
  {
    ruta: "/conta/bancos",
    queEs: "Qué cuenta contable representa cada cuenta del banco: la otra mitad de todo asiento.",
    puntos: [
      "Un asiento necesita DOS cuentas. En «Mapeo contable» se dice a qué cuenta va la partida; acá, cuál es la cuenta del banco.",
      "Varias se enlazan solas: el plan contable trae el IBAN dentro del nombre de la cuenta, así que el sistema encuentra a cuál corresponde. Mirá la sugerencia antes de aceptarla.",
      "Hay dos números de avance y miden cosas distintas: cuántas cuentas faltan, y cuánto movimiento cubren las que ya están. Pocas cuentas pueden cubrir casi todo el movimiento.",
      "El aviso ⚠ sale cuando dos cuentas del banco apuntan a la misma cuenta contable: sus movimientos quedarían mezclados en el mismo mayor.",
    ],
  },
  {
    ruta: "/conta/generar",
    queEs: "Convertir los movimientos del banco de un mes en asientos contables.",
    puntos: [
      "Contabiliza POR CAJA: el asiento nace cuando la plata se movió, no cuando llegó la factura. Es como opera la empresa hoy, pero no cumple NIIF.",
      "Primero mostrá qué va a pasar y después confirmá. Lo que queda fuera se explica por motivo, y cada motivo enlaza a la pantalla donde se arregla.",
      "Generar dos veces el mismo mes NO duplica nada: lo que ya tiene asiento se salta solo.",
      "Si algo salió mal, «Revertir» anula los asientos del mes con un motivo —no los borra— y después se puede volver a generar corregido.",
    ],
  },
  {
    ruta: "/outlook",
    queEs: "Cómo cierra el año: lo que se presupuestó, lo que va, y dónde se va a terminar.",
    puntos: [
      "Son TRES columnas, no dos. Con Presupuesto y Real solo se sabe dónde se está; la tercera dice dónde se va a terminar, que es lo que pregunta una junta.",
      "Hay dos escenarios porque «lo que falta del año» admite dos respuestas opuestas: que los meses restantes salgan como se presupuestaron, o como vienen saliendo. La BRECHA entre los dos es el dato: si el plan promete mucho más que el ritmo, el plan ya no describe el año.",
      "El mes en curso se lleva a mes completo con lo que lleva, en los dos escenarios. Meterlo a medias en un promedio mide los días que pasaron, no el desempeño.",
      "Un outlook armado en febrero es casi todo supuesto y se ve igual de firme que uno de noviembre: por eso la pantalla dice cuántos meses están cerrados y cuántos por venir.",
    ],
  },
  {
    ruta: "/drivers",
    queEs: "La cartera por lo que la mueve: cuántos contratos, de qué tipo y con qué cuota.",
    puntos: [
      "Todo lo demás del sistema analiza por PARTIDA CONTABLE —cuánto entró por Datáfonos, cuánto por Asociaciones—. Eso dice cómo entró la plata, no por qué. Sin drivers, presupuestar es «el año pasado más un diez por ciento».",
      "CONTRATADO no es FACTURADO: la suma de las cuotas es lo que debería entrar al mes; lo que generó cargo es lo que se puso a cobrar. La cobertura explica la brecha, y por eso los dos números van siempre juntos.",
      "El ticket recurrente de cada corte es lo que distingue un plan caro de uno barato, y lo que un «+10 %» tendría que mover.",
      "Mide la CARTERA, no la operación: cuántos funerales o cremaciones se ejecutaron, su ticket y su mix no se registran todavía.",
    ],
  },
  {
    ruta: "/ratios",
    queEs: "Cuántos días tardás en cobrar (DSO), cuántos en pagar (DPO), y cuántos financiás la operación con plata propia.",
    puntos: [
      "Es lo primero que pregunta un banco o un comprador. El DPO alto no es gratis: pagar tarde se negocia con precio, y a partir de cierto punto el proveedor deja de vender.",
      "El número principal se calcula por AGOTAMIENTO: descuenta la cartera contra lo facturado de cada mes, del más reciente al más viejo. No promedia, y por eso resiste meses dispares —acá los hay, de ₡73 a ₡187 millones—.",
      "Al lado va la fórmula de libro. Cuando las dos se separan mucho, está diciendo que los últimos meses no se parecen al promedio: eso es información del negocio, no del método.",
      "El DSO solo es confiable si los cobros se registran contra el cargo. Mientras no se registren, la cartera nunca baja y el ratio mide el registro en vez de la cobranza — la pantalla lo avisa cuando pasa.",
    ],
  },
  {
    ruta: "/flujo-caja",
    queEs: "Cuánto vas a tener en el banco cada una de las próximas 13 semanas, y en cuál te quedás corto.",
    puntos: [
      "Proyecta el SALDO, no el neto. Un mes puede cerrar positivo y aun así dejar la cuenta en rojo el día 12: lo que decide si se paga la planilla es el saldo en una fecha.",
      "No proyecta por fecha de vencimiento. Con casi toda la cartera vencida, eso pondría todo en la semana 1 y el reporte no serviría: la cartera atrasada se desagota al ritmo que declara la política de caja.",
      "Cada semana se abre por fuente, y dice si lo que mueve es comprometido (hay documento y fecha), esperado (probable) o atraso. Es lo que dice cuál de los tres mover para que una semana deje de estar en rojo.",
      "Cubre proveedores, planilla y cobros de cartera; lo que sale del banco sin pasar por esos tres circuitos no está. El punto de partida es el saldo que captura tesorería, y si no hay ninguno la curva arranca en cero y lo avisa.",
    ],
  },
  {
    ruta: "/conta/cierre",
    queEs: "La prueba de que el libro cuadra, y el candado que congela un mes ya emitido.",
    puntos: [
      "El balance de comprobación arrastra el saldo anterior: sin esa columna, el saldo final sería el del mes y no el de la cuenta.",
      "Si no cuadra, el mes no se puede cerrar. Cerrarlo así dejaría el error congelado y la única salida sería reabrir.",
      "Un mes cerrado no recibe NADA: ni asientos nuevos, ni líneas, ni anulaciones. Para corregirlo hay que reabrirlo, y eso queda registrado con su motivo.",
      "Los saldos iniciales se cargan una sola vez, desde el balance del sistema anterior en Excel. Mientras falten, el balance es correcto pero incompleto.",
    ],
  },
  {
    ruta: "/conta/libros",
    queEs: "El diario —lo que se registró— y el mayor —qué tiene cada cuenta—.",
    puntos: [
      "Van a estar vacíos hasta que se apruebe de qué documento sale cada asiento. Mientras tanto se pueden escribir a mano, que es como se cargan los ajustes y las reclasificaciones.",
      "Un asiento que no cuadra no se puede guardar: el sistema no lo acepta aunque se insista. Arriba del botón se ve la diferencia mientras escribís.",
      "Los asientos NO se borran: se anulan con un motivo. Quedan en el libro, visibles y sin efecto, porque borrarlos dejaría un hueco en la numeración.",
      "En el mayor, el saldo arranca de lo anterior al rango. El signo sigue a la naturaleza de la cuenta: un pasivo con saldo acreedor sale positivo.",
    ],
  },

  // ── Configuración ─────────────────────────────────────────────────────────
  {
    ruta: "/incidentes",
    queEs: "Donde se canjea la referencia de un error por su causa exacta.",
    puntos: [
      "Cuando alguien dice «me dio error», pedile los seis caracteres del final del mensaje y buscalos acá.",
      "Da igual si los dicta en minúscula.",
      "Lo que aparece son detalles técnicos del sistema por dentro: por eso esta pantalla tiene su propio permiso.",
    ],
    guia: "cuando-algo-falla",
  },
  {
    ruta: "/seguridad",
    queEs: "Qué puede hacer cada rol, por empresa.",
    puntos: [
      "Los permisos marcados como críticos no son de operación: tocan dinero, credenciales o el sistema por dentro.",
      "Una misma persona puede tener roles distintos en empresas distintas.",
    ],
    guia: "quien-puede-que",
  },
  {
    ruta: "/usuarios",
    queEs: "Las personas que entran al sistema y a qué empresas.",
    puntos: [
      "Cada persona con su usuario propio. Una cuenta compartida borra la trazabilidad justo cuando hace falta.",
      "Dar de alta en otra empresa es una membresía, no un permiso: se agrega acá, no en la matriz.",
    ],
    guia: "quien-puede-que",
  },
];

/** La ayuda de una ruta, si la hay. */
export function ayudaDe(ruta: string): AyudaDePantalla | undefined {
  return AYUDA_POR_PANTALLA.find((a) => a.ruta === ruta);
}
