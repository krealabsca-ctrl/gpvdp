# GPVDP · Qué analiza hoy el sistema y qué le falta

**27 de octubre de 2026 · migración 111**
Alcance: módulo Bancos (análisis, proyecciones, control). Medido sobre **Memorial Pets**, que es
la única empresa con histórico largo.

---

## 1 · Por qué Memorial Pets

| Empresa | Movimientos | Período | Clasificado |
|---|---|---|---|
| **Memorial Pets** | **7.197** | **ene-2025 → sep-2026 (21 meses)** | **91,9 %** |
| Valle de Paz | 20.082 | jul-2026 → sep-2026 (3 meses) | 69,2 % |
| Coopeprofa | 3.909 | jul-2026 → sep-2026 (3 meses) | 66,7 % |

Memorial tiene **17 meses comparables** (clasificados al 90 % o más) y **dos años solapados**
(ene-ago de 2025 y de 2026). Es la única donde hoy se puede hablar de tendencia, estacionalidad o
comparación interanual. En las otras dos, cualquier análisis de serie es prematuro hasta que se
cargue histórico.

---

## 2 · Lo que ya existe

| Pantalla | Qué contesta | Grano | Período |
|---|---|---|---|
| **Dashboard** | Ingresos · Gastos · EBITDA del mes, serie de 12 meses, calendario de flujo, resumen por cuenta, cuadre por partida | Mes | Un mes |
| **Análisis y tendencias** | Cada partida contra su propio promedio; qué se salió de cauce | Partida × mes | 6 / 12 / 24 meses |
| **Proyecciones** | Cierre del mes con 4 métodos (ritmo, mismo mes del año anterior, promedio del año, mes gemelo), meta y brecha | Clasificación | Mes en curso |
| **Control** | Gasto contra presupuesto, por departamento y por partida | Departamento × mes | Mes / **Trimestre** / 6 / 12 |
| **Saldos diarios** | Saldo del banco contra el esperado, posición por moneda, checklist de carga | Día | Día |
| **Conciliación** | Bancaria y contra CxP | Movimiento | — |
| **Grupo** | Las tres empresas sumadas | Empresa | Mes |

Es más de lo que tiene la mayoría de las empresas de este tamaño. Lo que sigue no es una crítica
al diseño: es lo que se nota cuando hay 21 meses de datos y antes no había.

---

## 3 · Las variables de cálculo, y dónde se quedan cortas

Así se calcula hoy una anomalía en «Análisis y tendencias»:

| Variable | Valor actual | Dónde vive |
|---|---|---|
| Promedio | Media aritmética simple de los meses comparables **anteriores** (excluye el mes juzgado) | `analisis_partidas.go` |
| Mes comparable | ≥ **90 %** clasificado | `umbralClasificadoComparable` |
| Confiable | ≥ **2** meses previos comparables | `p.Confiable` |
| Desvío | `(último − promedio) / promedio × 100` | — |
| Umbral de alerta | **25 %**, editable por el usuario | pantalla |
| Rango máximo | **24 meses** | `maxMesesAnalisis` |

Las seis decisiones son correctas y están bien defendidas en el código. **El problema no es
ninguna de ellas por separado: es que el promedio simple no distingue tres cosas distintas.**

Medido sobre el gasto mensual de Memorial (17 meses comparables, en millones de colones):

```
2025-01  10,8   2025-07  23,4   2026-01  13,5   2026-07  16,8
2025-02   7,8   2025-09  14,1   2026-02  15,3   2026-08  10,4
2025-03   9,9   2025-10  14,7   2026-03  17,1
2025-04  13,6   2025-11  12,7   2026-04  13,8
2025-05  16,3   2025-12  17,5
2025-06  14,6
```

**El gasto va de ₡7,8 M a ₡23,4 M: tres veces.** Contra el promedio simple, los meses se desvían
entre **−45 % y +64 %**. Con el umbral por defecto de 25 %, **un tercio de los meses sale marcado
como anomalía sin que pase nada raro**.

Y comparando el mismo mes de un año contra otro:

| Mes | 2025 | 2026 | Variación |
|---|---|---|---|
| Febrero | 7,8 | 15,3 | **+96 %** |
| Marzo | 9,9 | 17,1 | **+72 %** |
| Abril | 13,6 | 13,8 | +1 % |
| Julio | 23,4 | 16,8 | **−28 %** |

Son lecturas **opuestas** a las que da el promedio móvil, y hoy el sistema solo sabe hacer la
segunda. Un promedio de 12 meses que mezcla 2025 con 2026 arrastra el nivel del año viejo, y
después cada mes de 2026 parece una anomalía al alza que en realidad es crecimiento.

---

## 4 · Lo que falta

Ordenado por valor sobre esfuerzo. La columna **Datos** dice si ya se puede hacer con lo cargado.

### Tanda 1 — barato y de alto impacto · **CONSTRUIDA el 28-oct-2026**

> Las cinco están hechas y verificadas sobre Memorial Pets. Al probarlas contra datos reales
> apareció un defecto que no estaba previsto en este diagnóstico y que valía por sí solo:
>
> **La columna «año anterior» no suma al total del año anterior.** La lista de partidas sale del
> rango actual, así que las que se usaban antes y ya no, no tienen fila donde aparecer. En Q1 de
> Memorial son **38 partidas nuevas por ₡40,2 M y 34 desaparecidas por ₡38,0 M**: sin aviso,
> cualquiera habría leído «el gasto se disparó» cuando buena parte es reclasificación. Ahora la
> pantalla lo dice antes que cualquier número.
>
> Y quedó a la vista por qué las dos lecturas conviven: Planilla en Q1-2026 sale **+24,3 %
> contra el año anterior** y **−13,6 % contra su propio promedio**. Las dos son ciertas.
>
> Concentración de ese mismo trimestre: **8 de 38 partidas explican el 80 %**.

| # | Qué falta | Por qué importa | Datos | Esfuerzo |
|---|---|---|---|---|
| **1** | **Trimestre y rango libre en Análisis** | Hoy solo hay 6 / 12 / 24 meses hacia atrás. No se puede pedir «Q1 2026» ni «mayo a agosto». El **Control ya tiene botón de Trimestre** y Análisis no: dos pantallas del mismo módulo con criterios distintos | ✅ | **Bajo.** El endpoint **ya acepta `desde` y `hasta` libres**; falta solo el control en pantalla |
| **2** | **Comparación interanual (mismo mes / mismo trimestre del año anterior)** | Es lo primero que pregunta un comité y la tabla de arriba muestra que da lecturas opuestas al promedio móvil | ✅ Memorial | **Bajo-medio.** La serie ya existe; es una segunda columna |
| **3** | **Acumulado del año (YTD)** | El número más usado en cualquier reunión de resultados. Hoy no existe en ninguna pantalla | ✅ | **Bajo** |
| **4** | **Exportar el análisis a Excel** | Hoy **no hay ningún botón de exportar en Análisis**: para llevarlo a un comité hay que capturar la pantalla | ✅ | **Bajo.** `ConstruirLibro` ya existe |
| **5** | **Concentración (Pareto)** | Medido en Memorial: **10 partidas de 116 explican el 73 % del gasto**, y Planilla sola el 31,4 %. La pantalla ordena 86 partidas por desvío, así que lo que mueve el dinero se pierde entre lo que se movió en porcentaje | ✅ | **Bajo** |

### Tanda 2 — cambia la calidad del número · **CONSTRUIDA el 28-oct-2026**

> Las cuatro están hechas. Lo que midieron sobre Memorial:
>
> · **La mediana cambia la conclusión, no solo el número.** El promedio está entre **+90 % y
>   +126 %** por encima de la mediana en varias partidas. El caso real es **Mercadeo: −79,4 %
>   contra el promedio y −7,3 % contra la mediana** — contra uno parece un desplome, contra la
>   otra está en su sitio. La pantalla deja elegir la base.
>
> · **Recurrencia**: 8 partidas fijas (47 % del dinero), 16 intermitentes y **93 esporádicas**
>   (19,6 %). Para una esporádica, «promedio mensual» no significa nada.
>
> · **Estacionalidad**: **solo 5 de los 12 meses tienen dos años cargados**, así que el resto
>   sale en gris con «sin patrón». Los cinco confiables sí muestran algo —enero-febrero en 82-86,
>   julio en 118—. Para verla hay que pedir 24 meses; con 12 ningún mes se repite y la pantalla
>   lo dice en vez de fingir un patrón.
>
> · **Puente de variación**: de ₡110,4 M a ₡158,1 M en 12 meses, y **Gas explica ₡8,0 M (17 %)**.

| # | Qué falta | Por qué importa | Datos | Esfuerzo |
|---|---|---|---|---|
| **6** | **Mediana o media recortada como alternativa al promedio** | Un pago extraordinario infla el promedio y después todo parece bajo. Con rangos de 3× como los de Memorial, la media simple es frágil | ✅ | **Bajo.** Es una opción al lado del promedio, no un reemplazo |
| **7** | **Índice estacional** | Separar «diciembre siempre es alto» de «diciembre se disparó». Hoy son indistinguibles | ✅ Memorial (2 años) | **Medio** |
| **8** | **Gasto recurrente vs. extraordinario** | Hoy van juntos y contaminan el promedio. La liquidación laboral de un mes no es gasto corriente | ✅ | **Medio** |
| **9** | **Variación explicada (puente / waterfall)** | «El EBITDA bajó ₡12 M: ₡8 M por planilla, ₡3 M por gas». Hoy se ve el total y se ve el detalle, pero nadie arma el puente entre los dos | ✅ | **Medio** |

### Tanda 3 — revisada el 28-oct-2026 contra los datos

> **Al medirla, cuatro de los cinco puntos resultaron distintos de como los planteé.** Esto es lo
> que dicen los datos, y lo que se hizo con cada uno:
>
> | # | Veredicto |
> |---|---|
> | **10 · Flujo de caja** | **CONSTRUIDO, pero no como estaba planteado.** Proyectar por vencimientos no funciona: de la cartera pendiente de Valle de Paz, **₡936,9 M ya venció y solo ₡61,6 M vence a futuro** (el 99,9 %). Pondría todo en «este mes» y nada después. Se construyó por **patrón histórico**, usando la mediana y la estacionalidad de la tanda 2. Verificado: Memorial proyecta +₡4,56 M netos/mes sobre 8 meses de historia. |
> | **11 · Comparar empresas** | **YA EXISTÍA. Mi diagnóstico estaba equivocado.** El resumen del grupo ya desglosa cada partida por empresa (`PartidaGrupo.PorEmpresa`): Planilla ₡81,1 M repartida entre las tres, Depósito de Clientes ₡68,2 M entre las tres. No se construyó nada. |
> | **12 · Presupuesto anual** | **Prematuro.** Hay **6 presupuestos mensuales cargados** en todo el sistema. Construir la apertura anual sobre eso es construir para nadie; primero habría que usar el mensual. |
> | **13 · Alertas que lleguen solas** | **Necesita infraestructura que no existe.** No hay ningún programador de tareas en el sistema —todo envío de correo lo dispara una acción de una persona—, hay **0 buzones configurados** y **0 plantillas**, y falta decidir a quién le llega. Es el mismo bloqueo que ya tiene pendiente el aviso de incidentes. |
> | **14 · Devengo** | Sigue bloqueado por la **Fase 3B** de contabilidad. |

### Lo que decía antes de medirlo

| # | Qué falta | Por qué importa | Datos | Esfuerzo |
|---|---|---|---|---|
| **10** | **Flujo de caja proyectado** | Las proyecciones de hoy solo proyectan **ingresos** del **mes en curso**. No hay proyección de gastos ni de saldo bancario futuro, que es lo que contesta «¿llego a fin de mes?» | ⚠ Necesita cruzar CxP (vencimientos) + nómina (quincenas) + recurrentes | **Alto** |
| **11** | **Comparación entre empresas** | `/grupo` suma las tres pero no las compara en las mismas partidas. Hoy no se puede ver si Memorial gasta más en gas que VDP | ⚠ Catálogos distintos por empresa; necesita el mapeo contable como puente | **Medio-alto** |
| **12** | **Presupuesto anual con apertura mensual** | El presupuesto es **por mes suelto** (`presupuesto_departamento.periodo`). No hay presupuesto anual, ni reproyección, ni comparación plan-real-plan revisado | ✅ estructura, falta modelo | **Medio** |
| **13** | **Alertas que lleguen solas** | Todo lo anterior exige **entrar a mirar**. Un comité quiere recibir «estas 5 partidas se salieron este mes» | ✅ | **Medio.** El correo saliente ya existe (mig 0084) |
| **14** | **Análisis por devengo, no por caja** | Todo lo que hay hoy mide **cuándo salió la plata del banco**. El gasto del mes es otra cosa | ❌ Depende de la **Fase 3B** de contabilidad | — |

---

## 5 · Lo que NO recomiendo agregar

Para que la lista de arriba no se lea como «hay que hacerlo todo»:

1. **Ratios financieros clásicos** (liquidez, endeudamiento, rotación). Necesitan balance, y el
   balance necesita la contabilidad terminada. Calcularlos desde el banco daría números con
   nombre correcto y contenido falso.
2. **Pronóstico estadístico** (regresión, series temporales). Con 17 meses comparables y un
   negocio que cambió de nivel entre 2025 y 2026, un modelo daría precisión aparente sobre muy
   pocos datos. Los 4 métodos actuales de proyección son honestos y suficientes.
3. **Dashboards nuevos.** El problema no es la falta de pantallas, es que a las que hay les falta
   el corte por período y la comparación contra el año anterior.

---

## 6 · Qué recomiendo

**La tanda 1 completa.** Son cinco cosas, todas de esfuerzo bajo, y tres de ellas
—rango libre, interanual y YTD— son exactamente lo que se le pide a un sistema cuando por fin
tiene dos años de datos. El punto 1 es casi gratis porque **el backend ya lo soporta**.

Después, con eso en uso, decidir la tanda 2 según lo que se vea.

La tanda 3 no debería arrancar hasta que: el flujo de caja tenga definido de dónde salen los
vencimientos, y el análisis por devengo tenga la Fase 3B.

---

*Documento de diagnóstico. No se construyó nada: es insumo para decidir qué se construye.*

---

## 7 · Revisión del 30-oct-2026 — «¿qué falta para estar a nivel internacional?»

Medido sobre las tres pantallas que existen hoy (Análisis y tendencias, Control y presupuesto,
Proyecciones). El diagnóstico en una frase:

> Hoy el módulo contesta **«qué pasó y cuánto me desvié del gasto presupuestado»**. Lo que
> pregunta una junta, un banco o un comprador es **«cuánto voy a tener en el banco en ocho
> semanas, cómo cierro el año, y cuántos días tardo en cobrar»**.

### Los ocho huecos

| # | Qué falta | Estado |
|---|---|---|
| **1** | **Saldo de caja proyectado, semanal (13 semanas)** | **CONSTRUIDO** el 30-oct-2026 (mig 0114, `/flujo-caja`) |
| **2** | DSO, DPO y ciclo de conversión de efectivo | **CONSTRUIDO** el 2-nov-2026 (mig 0115, `/ratios`) |
| **3** | Presupuesto de **ingresos** | **CONSTRUIDO** el 3-nov-2026 (mig 0116, en `/control`) |
| **4** | Outlook del año | **CONSTRUIDO** el 3-nov-2026 (`/outlook`, sin migración) |
| **5** | Deterioro de cartera / ECL (NIIF 9) | Pendiente. **Es requisito contable, no análisis** |
| **6** | Tipo de cambio constante | Pendiente. Sin él no se separa «crecí» de «se movió el dólar» |
| **7** | Drivers operativos y unit economics (servicios/mes, ticket, mix, margen por servicio) | Pendiente. Sin drivers, el presupuesto es «el año pasado +10 %» |
| **8** | Campo de explicación de la variación | Pendiente. Un −18 % sin el porqué escrito no es información en un comité |

**Dependencias:** 1, 2, 7 y 8 son independientes de todo. 3 y 4 dependen del presupuesto que se
cargue. **5 y 6 —y todo ratio de balance (corriente, prueba ácida, deuda/EBITDA, ROE)— dependen
de A2: los saldos iniciales reales.** Sin balance no hay denominador.

### Lo construido: flujo de caja a 13 semanas (mig 0114)

**El hallazgo que definió el diseño:** las fechas de vencimiento no sirven para proyectar caja en
esta empresa. Medido: **4.465 de 4.469 facturas abiertas de CxP ya vencieron y solo 32 tienen
fecha de pago programada**; en CxC los cargos se generan por período, así que los de las próximas
trece semanas todavía no existen como registro. Proyectar por vencimiento pondría ₡933 millones
en la semana 1 y una línea plana en cero hacia adelante.

Lo que se modela es **comportamiento**: cuánto se paga de atrasos por semana (la política que el
Director Financiero declara, decisión suya del 30-oct-2026) y cuánto se cobra de verdad (la tasa
medida contra el histórico). Tres capas separadas y visibles —comprometido, esperado y represa—
porque el reporte existe para decir **cuál de las tres mover** para que una semana no quede en
rojo.

**Las cuatro fuentes:** saldo declarado en Tesorería puesto al día con los movimientos
posteriores · CxP por `fecha_pago_programada` · `corrida_nomina` por `fecha_pago` (neto +
patronal) · `cargo_cxc` por `vence_en` más la cuota de los 35.375 contratos activos proyectada en
su día de pago.

**Lo que NO cubre, y lo dice en cada consulta:** el gasto que sale del banco sin pasar por CxP,
planilla o CxC. Incluirlo desde el patrón histórico duplicaría los pagos a proveedores, porque el
enlace Bancos↔CxP todavía no distingue cuáles movimientos ya están contados.

### Lo construido: ratios del ciclo de efectivo (mig 0115)

**El método: agotamiento, no promedio.** La fórmula de libro supone que las ventas se repartieron
parejo, y las compras de esta empresa van de ₡73 a ₡187 millones por mes. El countback descuenta
la cartera contra lo facturado de cada mes, del más reciente al más viejo. Se publican los dos: la
brecha entre ellos dice que los últimos meses no se parecen al promedio.

**Medido sobre Valle de Paz, ventana de 12 meses:**

| | Agotamiento | Fórmula simple | Cartera | Base mensual |
|---|---|---|---|---|
| **DSO** | 304,0 días | 304,0 días | ₡533.206.491 | ₡53.320.649 |
| **DPO** | **254,4 días** | 273,1 días | ₡998.446.854 | ₡111.283.945 |
| **Ciclo** | 49,6 días | | | |

El DSO dio idéntico por los dos métodos —los cargos de CxC son cuotas regulares— y el DPO se
separó 19 días. Esa asimetría es exactamente el argumento del countback, con datos.

**El hallazgo: DPO de 254 días.** Ocho meses y medio para pagar a proveedores. No es riesgo de
caja sino comercial: a ese atraso el proveedor sube el precio, pide contado o deja de vender. La
pantalla ahora lo marca — la regla original solo vigilaba el DPO bajo, porque un DPO alto parece
la posición cómoda y por eso nadie lo mira.

**El DSO todavía no es confiable, y lo dice.** Hay 70.592 cargos abiertos y ninguno con un cobro
aplicado: mientras los cobros no se registren contra el cargo, la cartera no baja y el ratio mide
el registro en vez de la cobranza.

**El DIO queda fuera:** `inv_nivel` guarda mínimos y máximos de reposición, no existencias, y no
hay un solo artículo cargado. El ciclo se publica como DSO − DPO declarando qué le falta.

### Lo construido: presupuesto de ingreso (mig 0116)

**El ingreso se mide al revés que el gasto.** Consumir el 95 % del presupuesto de gasto es una
alerta; llegar al 95 % de la meta de ingreso es casi un logro. Por eso los estados son propios
—CUMPLIDO / EN_CAMINO / CORTO / SIN_META, umbral 85 %— y no los heredados del gasto: un semáforo
copiado pinta de rojo un mes bueno y nadie lo nota, porque el monto de abajo está bien.

Y **sin meta declarada no es 0 % de cumplimiento**: un mes que nadie presupuestó no incumplió
nada.

**Se reutiliza el patrón del subpresupuesto**: meta total del mes (`clasificacion_id` nulo) más
desglose opcional por línea. Las 41 clasificaciones de ingreso serían unas 500 celdas al año si se
exigieran todas, y la pantalla quedaría vacía para siempre.

**Lo que nació con esto — el resultado presupuestado.** Verificado sobre Memorial Pets, sept-2026:

| | Meta | Real |
|---|---|---|
| Ingreso | ₡13.000.000 | ₡4.254.076 (32,7 % → CORTO) |
| Gasto | ₡1.000.000 | ₡1.597.912 |
| **Resultado** | **₡12.000.000** | **₡2.656.164** |
| **Desvío** | | **−₡9.343.836** |

Con media meta el número engaña, y el aviso dice hacia qué lado: sin meta de ingreso el resultado
sale artificialmente malo; sin presupuesto de gasto, artificialmente bueno.

**La ayuda que lo hace usable:** un botón propone la meta desde el mismo mes del año anterior con
el ajuste porcentual que se le indique. Propone, no guarda. Verificado: Datáfonos ₡5.596.849 →
₡6.044.597 con +8 %.

### Lo construido: outlook del año (sin migración)

**Son tres columnas, no dos.** Con Presupuesto y Real solo se sabe dónde se está; la tercera dice
dónde se va a terminar.

**Dos escenarios, porque la brecha es el dato.** «Lo que falta del año» admite dos respuestas
opuestas: que los meses restantes salgan como se presupuestaron (PLAN) o como vienen saliendo
(RITMO). Si el plan promete mucho más que el ritmo, el plan ya no describe el año.

**Las dos trampas del tiempo, resueltas:**

- *El mes en curso va a paridad de día*, en ambos escenarios. Metido a medias en un promedio,
  mediría los días transcurridos y no el desempeño. Los escenarios solo difieren en los meses
  futuros, así que la única variable entre ellos es la que se compara.
- *Un mes cerrado sin movimiento no promedia.* Lo destapó la pantalla: Memorial tenía nueve meses
  con movimiento y octubre cerrado en blanco; contarlo como mes de cero bajaba el cierre
  proyectado un 10 % con un número perfectamente plausible. Un mes sin movimiento casi nunca es un
  mes sin operación: es un estado de cuenta sin importar. Queda fuera del promedio y se avisa.

Verificado sobre Memorial 2026: YTD ₡147,3 M de ingreso y ₡114,2 M de gasto (10 meses cerrados, 1
sin datos, noviembre al 10 %), con el año cerrando en **₡36.751.721** al ritmo actual. El escenario
PLAN no se calcula por falta de meta de ingreso, y lo dice en vez de mostrar un cierre en cero.

### Lo construido: drivers del negocio (hueco 7, sin migración)

**Qué cambia.** Todo el análisis anterior es por partida contable: dice **cómo** entró la plata
(Datáfonos, Asociaciones), no **por qué**. Sin drivers, presupuestar es «el año pasado +10 %»,
porque no hay con qué explicar de dónde saldría ese diez. El driver de este negocio es la
**cartera**: 35.375 contratos de prepago, cada uno con su cuota, su tipo de servicio, su modalidad
y su sede.

**La distinción que sostiene la pantalla: contratado ≠ facturado.** La suma de las cuotas vigentes
es lo que *debería* entrar; lo que generó cargo este mes es lo que de verdad se puso a cobrar.
Medido: **₡227.207.030 contratados contra 10.468 de 35.375 contratos con cargo en el mes**.
Publicar solo el contratado daría un ingreso recurrente que no corresponde a nada y se vería
perfectamente razonable. Van los dos, y **la brecha es el dato**.

**Se abre por tres dimensiones** —tipo de servicio (el mix: CORPORATIVO 15.488, PLANES COOPEPROFA
11.119, …), modalidad y sede— con el ticket recurrente de cada corte, que es lo que distingue un
plan caro de uno barato y lo que un «+10 %» tendría que mover. Ticket promedio de la cartera:
**₡6.422,81** (227.207.030 / 35.375).

**Lo que NO puede medir, y se dice en vez de aproximarlo.** El servicio *prestado* —el funeral o la
cremación ejecutada—, su ticket y su mix. La tabla `inv_servicio` existe con la estructura correcta
y está **vacía**: nadie registra servicios. Sin eso hay unit economics de la **cartera**, no de la
**operación**. Aproximarlo con movimientos bancarios contaría depósitos, no servicios.

Ruta `/drivers`, paquete transversal de solo lectura `internal/ratios` (mismo precedente que
`internal/grupo` y `internal/flujocaja`).
