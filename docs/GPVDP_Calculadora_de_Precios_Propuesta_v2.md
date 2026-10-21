# Calculadora de Precios — Propuesta v2

**Para:** Gerencia General · Gerencia Financiera · Gerencia de Costos
**De:** Coordinación de Gestiones Financieras
**Fecha:** 14 de octubre de 2026
**Reemplaza a:** `GPVDP_Calculadora_de_Precios_Propuesta_v1.md`

---

## Qué cambió desde la v1

La v1 se escribió leyendo el **costeo de 2023**. Después apareció el archivo **«Precios Calculadora
Abril 2025»**, que es el vigente, y con él siete respuestas de negocio. Eso cambió el diagnóstico de
raíz:

| | v1 (leyendo el costeo 2023) | v2 (leyendo el archivo vigente) |
|---|---|---|
| Qué le pasa al costeo | «Tiene huecos que hay que llenar» | **El 97,4 % del catálogo no tiene costo** |
| El margen | «Va de 24 % a 54 % sin regla» | **El 80 % que declara la hoja es 47,6 %** |
| La lista de precios | No se había examinado | **230 de 230 líneas son valores pegados a mano** |
| Los planes de suscripción | «Les falta el modelo actuarial» | **Quedaron fuera de la actualización de 2025** |
| La base de contratos | 31.000 | **72.000** |

Todas las cifras de este documento están **medidas**, no estimadas, salvo las que se marcan
explícitamente como estimación.

---

## 0 · Resumen para quien lee una sola página

**El problema no es que falten datos en el costeo. Es que el costeo dejó de estar anclado al costo.**

Tres mediciones sobre el archivo vigente:

1. **De 693 líneas en la tabla de costos, 675 no tienen costo.** En 665 de ellas el «margen» se
   calcula desde el precio y el precio se calcula desde el precio. Es circular: no hay un costo en
   ninguna parte de la cadena.
2. **El margen declarado no es el margen real.** La hoja dice 80 %. De las 18 líneas que sí tienen
   costo, el margen real promedia **47,6 %** y ninguna llega al 80 %. Son **32 puntos** de
   diferencia; quien autorice un descuento creyendo que tiene 80 % de colchón lo está dando contra
   la mitad.
3. **La lista con la que se cotiza no tiene una sola fórmula.** Son 230 valores pegados a mano. Si
   sube el costo de un cofre, la lista sigue mostrando el precio de siempre y nadie se entera.

Y un cuarto, que es el que más plata mueve: **los cuatro planes de suscripción no están en la
calculadora de 2025**. Los 72.000 contratos —la cartera recurrente— siguen costeados con el archivo
de 2023.

**Lo que se propone** es un módulo que vuelva a anclar el precio al costo: el costo sale de lo que
CxP pagó de verdad, los indirectos se reparten por lo que los causa, los planes de suscripción se
tarifan con un modelo actuarial alimentado por los 72.000 contratos de CxC, y un simulador permite
mover una variable y ver el efecto antes de decidir.

---

## 1 · El diagnóstico, medido sobre el archivo vigente

Archivo: `Precios Calculadora Abril 2025.xlsx`. Cuatro hojas: `Tabla de costos` (694 filas),
`Precios calculadora` (702), `Final` (236), `Lista Final Calculadora 2025` (231).

### 1.1 · El 97,4 % del catálogo no tiene costo

| | Líneas |
|---|---|
| Con costo real en la columna C | **18** |
| Sin costo | **675** |
| Con «margen» calculado desde el precio (circular) | **665** |

La diferencia se ve comparando dos filas de la misma hoja:

```
Cofre Standard      C = 79.000          ← costo real del proveedor
                    E = C × 55%         ← el margen sale del COSTO     ✔

Cremación Adulto    C = (vacía)         ← no hay costo
                    F = H ÷ 1,13        ← el precio sale del precio
                    E = F × 55%         ← el margen sale del PRECIO    ✘
```

En la segunda no existe un costo en ningún punto de la cadena. La nota «averiguar costos» que
arrastra el archivo de 2023 sigue sin responderse; lo que cambió es que ahora está disimulada
detrás de una fórmula que aparenta margen.

### 1.2 · El margen declarado está 32 puntos arriba del real

De las 18 líneas con costo verificable:

| | Margen sobre precio |
|---|---|
| Mínimo | 44,2 % |
| **Promedio** | **47,6 %** |
| Máximo | 48,7 % |
| Líneas que alcanzan el 80 % declarado | **0 de 18** |
| Líneas por debajo del 50 % | **18 de 18** |

La causa es la fórmula del precio de abril 2025:

```
K = J + E
    J = H × 80%   ← margen nuevo, calculado sobre el PRECIO ANTERIOR
    E = C × 55%   ← margen viejo, calculado sobre el costo
```

Suma dos márgenes de bases distintas y nunca regresa al costo. El cofre de ₡79.000 termina en
₡153.850: eso es 48,7 % sobre precio, no 80 %.

**El IVA no explica esa brecha.** Aun suponiendo el caso más favorable —que los costos del archivo
estén con IVA incluido y sobre un 13 %— el margen real subiría a 53,6 %: sigue **26 puntos** por
debajo del declarado.

### 1.3 · La lista con la que se cotiza está desconectada del costo

La hoja `Lista Final Calculadora 2025`, que es la que se usa:

| | |
|---|---|
| Líneas | 230 |
| Con fórmula | **0** |
| Valores pegados a mano | **230** |

Nada en esa hoja depende de la tabla de costos. Es una foto.

### 1.4 · «Margen» significa tres cosas distintas en el mismo archivo

| Hoja | Etiqueta | Qué es en realidad | Equivalente sobre precio |
|---|---|---|---|
| Tabla de costos | 55 %, 80 % | Mezcla de bases | **47,6 %** |
| Lista Final | 145 % (225 líneas), 155 %, 100 %, 212 % | Markup sobre costo | **59,2 %** |

La misma palabra con dos significados: los números de una hoja no se pueden comparar con los de la
otra, y las dos se llaman «margen».

### 1.5 · Los planes de suscripción quedaron fuera

Busqué Gardenia, Olivo, Tulipán y Cedro en las cuatro hojas: **cero coincidencias**.

La actualización de precios de 2025 cubrió el catálogo de venta directa —flores, cofres, cremación,
salas— y dejó afuera los planes. Los **72.000 contratos** de cartera recurrente siguen costeados con
el archivo de 2023.

No es que los planes tengan un costeo dudoso: **no tienen costeo vigente**.

---

## 2 · Lo que ya sabíamos del costeo 2023, que sigue en pie

### 2.1 · Los planes directos

| Plan | Costo | Precio | Margen | % |
|---|---|---|---|---|
| Jade | ₡377.541 | ₡500.000 | ₡122.459 | 24 % |
| Bronce | ₡419.811 | ₡650.000 | ₡230.189 | 35 % |
| Plata | ₡559.236 | ₡850.000 | ₡290.764 | 34 % |
| Oro | ₡936.225 | ₡1.350.000 | ₡413.775 | 31 % |
| Diamante | ₡1.254.607 | ₡2.750.000 | ₡1.495.393 | 54 % |
| Otoño | ₡350.674 | ₡650.000 | ₡299.326 | 46 % |
| Primavera | ₡899.344 | ₡1.250.000 | ₡350.656 | 28 % |

El margen no escala con el precio: Jade, el de entrada, rinde 24 % y Diamante 54 %. **Confirmado por
Gerencia que Jade responde a variables de venta**, o sea que el escalonamiento es comercial y no un
accidente de costeo. Queda igual la pregunta de cuál debería ser el margen objetivo de cada familia.

> **Y estos costos están subestimados**, por lo que se explica en §4.3: la comisión del vendedor
> entra sin sus cargas sociales.

### 2.2 · Los indirectos: dos reglas que no se hablan

| | `Indirectos Servicios` | `Indirectos Planes` |
|---|---|---|
| Gasto mensual promedio | ₡122.879.039 | ₡157.569.148 |
| A servicios | 30 % | 45 % |
| A planes | 70 % | 85 % |
| Divisor | 360 servicios · 31.000 contratos | 110 planes |

Tres problemas que siguen vigentes:

1. **45 % + 85 % = 130 %.** Se absorbe más gasto del que existe, o las bases son distintas y el
   archivo no lo dice.
2. **El mismo concepto recibe dos valores**: ₡102.399 por servicio y ₡1.217.580 por plan, mezclados
   en la misma fila.
3. **El divisor fijo es una trampa conocida**: dividir entre «110 planes» hace que *si baja la
   venta, suba el costo unitario*, y el modelo pide subir el precio justo cuando el mercado se
   enfría.

---

## 3 · El efecto de los 72.000 contratos

El dato de 72.000 contratos activos (contra los 31.000 del archivo 2023) corta en dos direcciones.

### 3.1 · El indirecto por contrato cae a la mitad

```
Gasto indirecto a planes = 70% × ₡122.879.039 = ₡86.015.327 / mes

  ÷ 31.000 contratos (2023)  →  ₡2.775 por contrato-mes
  ÷ 72.000 contratos (hoy)   →  ₡1.195 por contrato-mes
```

El doble de contratos reparte el mismo gasto fijo. Eso deja más margen por contrato para cubrir el
servicio.

### 3.2 · Pero descubre un límite que no se estaba mirando

Con ese margen, cada plan aguanta una **frecuencia de uso máxima** antes de perder plata:

| Plan | Cuota | Disponible/año | Severidad | **Frecuencia que aguanta** |
|---|---|---|---|---|
| Gardenia | ₡4.950 | ₡45.060 | ₡1.779.566 | **2,53 %** |
| Olivo | ₡4.950 | ₡45.060 | ₡1.785.566 | **2,52 %** |
| Tulipán | ₡7.800 | ₡79.260 | ₡1.878.365 | **4,22 %** |
| Cedro | ₡10.000 | ₡105.660 | ₡1.877.865 | **5,63 %** |

### 3.3 · La frecuencia estimada está por encima de dos de ellos

> **Lo que sigue es una ESTIMACIÓN, no una medición.** Se apoya en un supuesto que no se puede
> verificar con los datos actuales: cuántos de los servicios mensuales vienen de planes y cuántos
> de venta directa.

```
360 servicios/mes × 12                     = 4.320 al año
menos ~57/mes de venta directa             ≈ 3.636 de planes
sobre 72.000 contratos                     = 5,05 % anual
```

Como contraste independiente: la mortalidad nacional de Costa Rica (~5 por mil) sobre 648.000
beneficiarios da una cifra del mismo orden.

**Si la frecuencia real se acerca al 5 %, Gardenia y Olivo están bajo el punto de equilibrio y
Tulipán al filo.** Si está cerca del 0,47 % declarado, los cuatro planes son cómodamente rentables.

La diferencia entre las dos lecturas son cientos de millones al año, y depende de **una definición
que está pendiente**: si el 0,47 % histórico se midió por contrato o por beneficiario. Con nueve
beneficiarios por contrato, la segunda lectura da ~4,2 %.

**Por eso la propuesta pone medir la frecuencia antes de tocar cualquier precio.**

---

## 4 · Las decisiones tomadas, y lo que implican

### 4.1 · El IVA soportado es crédito fiscal

**Implicación:** el costo correcto para costear es el **subtotal**, no el total. El IVA de las
compras se recupera contra el IVA cobrado: no es costo, es dinero en tránsito.

Medido en CxP sobre 2026: **₡102.052.937 de IVA acreditable** sobre ₡893,9 millones de subtotal
(4.513 facturas).

**Ventaja operativa:** CxP ya guarda `subtotal` e `iva` en columnas separadas, así que el módulo
toma el costo correcto solo, sin que nadie decida factura por factura.

**Pendiente:** el costeo actual **no declara en qué base están sus costos**. Un «₡79.000» sin
etiqueta de con/sin IVA es ambiguo y la diferencia es 13 %. No se puede resolver mirando el archivo,
y tampoco deduciéndolo de las facturas, porque las compras traen varios modelos mezclados y el
detalle por línea está vacío (`cxp_linea` = 0 filas). **El catálogo debe guardar la base de forma
explícita.**

### 4.2 · La inflación es la del país

**Implicación:** IPC de Costa Rica. El módulo de Bancos ya tiene conexión al BCCR, así que el
sistema puede traerlo solo en vez de que alguien lo teclee y se desactualice.

### 4.3 · Las comisiones son por plan y por vendedor, y son variables

**Decisión:** un espacio configurable, no un número fijo en el costeo.

**Los datos confirman que es lo correcto.** El gasto de comisiones que sale por banco es irregular:

| Mes | Comisiones a colaboradores |
|---|---|
| ago-2026 | **₡28.620.309** (130 movimientos) |
| jul-2026 y hacia atrás | ₡150.000 (1 movimiento por mes) |

Un solo mes concentra el 87 % del total. No hay un porcentaje estable que derivar de ahí.

**Lo que sí es un dato firme**, y sirve de control para el módulo: **₡33.050.757** pagados entre
abril de 2025 y septiembre de 2026.

| Empresa | Comisiones | Movimientos |
|---|---|---|
| Valle de Paz | ₡23.528.689 | 90 |
| Coopeprofa | ₡5.795.220 | 45 |
| Memorial Pets | ₡3.726.848 | 33 |

#### El hallazgo: la comisión cuesta más de lo que recibe el vendedor

En nómina, el concepto **«Comisiones» está marcado como afecta CCSS, afecta renta y afecta
aguinaldo** en las tres empresas. Está bien puesto: es lo que manda la ley, las comisiones habituales
son salario.

Pero el costeo no lo refleja. El plan Oro lleva ₡135.000 de comisión; el costo real es:

```
₡135.000 × 1,2683 (cargas patronales CR 2026) = ₡171.221
```

**₡36.221 más por cada venta, que hoy no están en el costo.** Y esto aplica a los planes directos,
no solo a los de suscripción —donde el problema es el inverso: la comisión figura en cero.

#### Diseño del espacio configurable

| Campo | Para qué |
|---|---|
| **Plan** | Un plan específico o «todos» |
| **Vendedor** | La persona, un canal/grupo, o «todos» |
| **Tipo** | Porcentaje o monto fijo |
| **Base** | Sobre precio, sobre margen, o sobre la primera cuota |
| **Valor** | El número |
| **Vigencia** | Desde–hasta |

Resolución de lo específico a lo general: política de *este vendedor con este plan* → política del
plan → política general. Es el mismo patrón con que el sistema ya resuelve la clasificación por
defecto.

Dos cosas que lo separan de una tabla suelta en Excel:

- **La carga social se aplica sola.** Se configura «₡135.000 al vendedor» y el sistema costea
  ₡171.221, tomando el porcentaje de los parámetros de nómina, que ya están versionados por año.
- **La política queda fechada.** Si en marzo cambia la comisión de Gardenia, lo costeado en febrero
  sigue mostrando lo que costó en febrero. Sin eso, cambiar una política reescribe la historia.

### 4.4 · El costo de cremación sale de la tabla de costos

**Verificado:** la línea existe («Cremación de un Adulto», por sede), pero **con la casilla de costo
vacía** y la fórmula circular descrita en §1.1. Lo que hay es precio de venta (₡300.000), no costo.

**Implicación:** hay que capturar el costo real. Si el insumo no existe en el catálogo, la pantalla
1 debe permitir agregarlo y usarlo como referencia.

### 4.5 · Jade responde a variables de venta

**Implicación:** su 24 % contra el 54 % de Diamante es decisión comercial, no defecto de costeo. El
módulo debe **mostrar** la diferencia sin corregirla: el costo pone el piso, la estrategia pone el
precio.

### 4.6 · La siniestralidad es revisable y modificable

**Implicación aceptada:** el parámetro debe ser editable.

**Propuesta adicional:** que además se **mida**. Con 72.000 contratos y su historia de pagos en CxC,
cruzados contra el Registro Civil, la frecuencia deja de ser un supuesto que alguien ajusta y pasa a
ser un número medido. Un parámetro editable sigue siendo una opinión.

### 4.7 · El margen objetivo por familia: por definir

**Recomendación:** definirlo **después** de que el costeo esté anclado. Hoy la hoja muestra 80 %
cuando el real es 47,6 %; fijar un objetivo contra ese número sería fijarlo contra una ficción.

---

## 5 · La propuesta: módulo «Calculadora de Precios»

### 5.1 · Principio de diseño

**El precio se deriva del costo, y el costo se deriva de lo que se pagó.** Ningún número que el
módulo muestre puede existir sin una cadena que lo conecte a una factura, una planilla o un
parámetro declarado. Lo que no tenga esa cadena se muestra marcado como incompleto, nunca estimado
en silencio.

### 5.2 · Las seis pantallas

| # | Pantalla | Qué resuelve | Se alimenta de |
|---|---|---|---|
| **1** | **Catálogo de insumos** | El costo de cada insumo, con su base (con/sin IVA) declarada, contrastado contra la última compra real | CxP |
| **2** | **Receta del plan (BOM)** | La hoja `Costos` viva y **versionada por fecha**: «¿qué costaba Oro en enero?» | Pantalla 1 |
| **3** | **Reparto de indirectos (ABC)** | Reemplaza el 85 % por drivers defendibles; suma 100 % por construcción | Bancos |
| **4** | **Modelo actuarial** | Frecuencia, severidad, permanencia y cancelación de los planes de suscripción | CxC + Registro Civil |
| **5** | **Simulador** | Mover una variable y ver el efecto antes de decidir | Todas |
| **6** | **Tablero de rentabilidad** | La foto por plan y familia para Gerencia | Todas |

### 5.3 · El simulador, en detalle

Es el corazón del módulo. Controles sobre cuota, frecuencia, permanencia, inflación y composición
del plan; salida inmediata en cuatro indicadores:

- **Prima técnica** — la cuota que *debería* cobrarse
- **Cuota actual** y su diferencia
- **Resultado por contrato** a 36 meses
- **Frecuencia de equilibrio** — el techo que aguanta ese plan

Con escenarios comparables lado a lado. Preguntas que contesta y que hoy nadie puede responder:

- Si el cofre sube 12 %, ¿qué pasa con el margen de Oro?
- ¿A qué cuota empata Gardenia con inflación del 5 %?
- Si Jade sube a ₡550.000 y se pierde 8 % de volumen, ¿se gana o se pierde?
- ¿Cuántos beneficiarios aguanta cada cuota?

### 5.4 · La validación contra fuentes oficiales

| Fuente | Qué confirma | Efecto |
|---|---|---|
| **Registro Civil (TSE)** | Defunción del titular y de los 9 beneficiarios | Frecuencia **medida**. Y detecta el contrato que sigue pagando por alguien que ya falleció |
| **Padrón electoral (TSE)** | Edad y vigencia de cédula | Permite tarifar **por tramo de edad**: hoy un titular de 70 y uno de 35 pagan igual |
| **CCSS** | Condición del asegurado | Afina la segmentación de riesgo |

> **La ventaja que nadie más tiene:** 72.000 contratos con su historia de pagos. La permanencia y la
> cancelación se pueden **medir** con eso. Ninguna consultora externa puede calcularlo mejor.

### 5.5 · Lo que cada gerencia obtiene

| | |
|---|---|
| **Gerencia General** | Tablero: qué plan gana, cuál pierde, cuál está al filo |
| **Gerencia Financiera** | Margen real por plan y el efecto de cada decisión de precio en el resultado |
| **Gerencia de Costos** | Catálogo anclado a CxP y alertas cuando el modelo se despega de la factura |
| **Gerencia de Mercadeo** | El piso de precio de cada plan: hasta dónde se puede descontar sin perder |

### 5.6 · Sinergia con lo que ya existe

El módulo no parte de cero: **cuatro de sus cinco fuentes ya están construidas**.

| Necesita | Ya existe en |
|---|---|
| Costo real de insumos | CxP · gasto por ítem (4.542 facturas) |
| IVA separado del costo | CxP · columnas `subtotal` / `iva` |
| Gasto indirecto clasificado | Bancos · catálogo y clasificación |
| Cargas sociales versionadas | Nómina · parámetros por año |
| Historia de 72.000 contratos | CxC |
| Tipo de cambio e IPC | Bancos · conexión BCCR |

---

## 6 · Cómo lo hacen las empresas grandes

Cinco capas, de abajo hacia arriba. Las tres primeras son costeo; las dos últimas, decisión.

| Capa | Qué es | Dónde estamos |
|---|---|---|
| **1 · BOM** | Lista de materiales con costo unitario trazable al proveedor | Existe en papel; **sin anclaje al costo real** |
| **2 · ABC** | Indirectos repartidos por el driver que los causa | Hoy es un porcentaje plano que suma 130 % |
| **3 · Contribución marginal** | Separar variable de fijo para conocer el piso de precio | **No existe** |
| **4 · Modelo actuarial** | Frecuencia × severidad = prima pura (solo suscripción) | Piezas sueltas al pie de una hoja |
| **5 · Precio estratégico** | Elasticidad, good–better–best, LTV/CAC | Implícito en el criterio comercial |

**Lo que las grandes nunca hacen**, y conviene nombrarlo porque son exactamente los defectos
medidos en §1:

- Costear con un margen calculado desde el precio de venta
- Mantener la lista de cotización desconectada del costo
- Usar la palabra «margen» con dos significados en el mismo archivo
- Repartir indirectos con un divisor de volumen fijo
- Tarifar un producto de suscripción sin medir su frecuencia de uso

---

## 7 · Cómo construirlo, por etapas

Cada etapa entrega algo usable por sí sola.

| Etapa | Qué entrega | Por qué en ese orden |
|---|---|---|
| **1 · Catálogo de insumos** | Costo real por insumo, con base IVA declarada, alimentado desde CxP | Hoy el 97,4 % del catálogo no tiene costo: sin esto, lo demás es decoración |
| **2 · Receta + contribución marginal** | El BOM vivo y el **piso de precio** de cada plan | Responde la pregunta que más plata mueve a diario: hasta dónde se puede descontar |
| **3 · Comisiones** | Políticas por plan × vendedor, con carga social automática | Corrige un costo subestimado en todos los planes directos |
| **4 · ABC** | Indirectos por driver | Reemplaza el 85 % por algo defendible ante auditoría |
| **5 · Actuarial** | Prima técnica contra cuota cobrada | Cierra el hueco de los 72.000 contratos |
| **6 · Simulador + Tablero** | Escenarios y la foto para Gerencia | Vale cuando las capas de abajo son confiables |

**Recomendación de arranque: etapas 1 y 2 en paralelo.** La etapa 1 destraba todo lo demás, y la 2
se puede calcular con lo que ya está en el Excel para las 18 líneas que sí tienen costo, de modo que
entrega valor antes de que el catálogo esté completo.

**La etapa 5 no debe arrancar antes de resolver la definición de siniestralidad** (§3.3). Construir
el modelo actuarial sobre una frecuencia ambigua produciría números con aspecto de precisión y sin
sustento.

---

## 8 · Lo que queda abierto

| | Quién decide | Bloquea |
|---|---|---|
| **¿La siniestralidad 0,47 % es por contrato o por beneficiario?** | Gerencia + quien hizo el cálculo original | Etapa 5 |
| **El costo real de «Cremación de Adulto»** | Gerencia de Costos | El costeo de 6 planes |
| **Margen objetivo por familia** | Gerencia General | Nada: se define después de medir |
| **La base IVA de los costos actuales** | Gerencia de Costos | Precisión del 13 % en el catálogo |

---

## 9 · Lo que este documento NO hace

- **No recalcula los precios.** Para eso hace falta el catálogo con costo real, que es la etapa 1.
- **No afirma que Gardenia y Olivo pierdan plata.** Afirma que **con la frecuencia estimada en
  5,05 % estarían bajo el punto de equilibrio**, que esa frecuencia es una estimación con supuestos
  declarados, y que la medición pendiente es lo que convierte eso en un hecho o lo descarta.
- **No reemplaza el criterio comercial.** El costo pone el piso; el precio lo ponen el mercado y la
  estrategia. Lo que el módulo aporta es que el piso deje de ser una adivinanza.
- **No es un dictamen fiscal.** El tratamiento del IVA como crédito fiscal se toma de la decisión de
  Gerencia Financiera registrada en §4.1.
